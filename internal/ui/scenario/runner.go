package scenario

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
	"github.com/JamesTiberiusKirk/godbml/internal/dbmledit"
	"github.com/JamesTiberiusKirk/godbml/internal/ui"
)

// Runner executes a scenario against an App, advancing one step per Update
// frame and writing a PNG whenever a `shoot` step fires.
type Runner struct {
	app  *ui.App
	scen *Scenario

	cursor int
	width  int
	height int

	pendingShoot string

	// numerical exit code (0 = success, 1 = at least one step error).
	// Errors are logged but don't abort the scenario — we want a snapshot
	// per attempted step even if some fail.
	hadError bool
}

func NewRunner(app *ui.App, scen *Scenario) *Runner {
	return &Runner{
		app:    app,
		scen:   scen,
		width:  scen.Window.Width,
		height: scen.Window.Height,
	}
}

func (r *Runner) Failed() bool { return r.hadError }

func (r *Runner) Update() error {
	if r.cursor >= len(r.scen.Steps) {
		return ebiten.Termination
	}
	step := r.scen.Steps[r.cursor]
	r.cursor++
	if err := r.applyStep(step); err != nil {
		log.Printf("step %d (%s): %v", r.cursor, step.Kind, err)
		r.hadError = true
	}
	return nil
}

func (r *Runner) Draw(screen *ebiten.Image) {
	r.app.Draw(screen)
	if r.pendingShoot == "" {
		return
	}
	path := r.pendingShoot
	r.pendingShoot = ""
	if err := savePNG(screen, path); err != nil {
		log.Printf("write %s: %v", path, err)
		r.hadError = true
		return
	}
	log.Printf("wrote %s", path)
}

func (r *Runner) Layout(int, int) (int, int) { return r.width, r.height }

func (r *Runner) applyStep(step Step) error {
	switch step.Kind {
	case ActionShoot:
		var name string
		if err := step.Decode(&name); err != nil {
			return err
		}
		r.pendingShoot = filepath.Join(r.scen.OutDir, name)
		return nil

	case ActionCamera:
		var args CameraArgs
		if err := step.Decode(&args); err != nil {
			return err
		}
		if args.Fit {
			r.app.FitToTables()
			return nil
		}
		r.app.SetCamera(args.X, args.Y, args.Zoom)
		return nil

	case ActionFit:
		r.app.FitToTables()
		return nil

	case ActionAdvanceFrames:
		var n int
		if err := step.Decode(&n); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			r.app.AdvanceFrame()
		}
		return nil

	case ActionSetFrame:
		var n int
		if err := step.Decode(&n); err != nil {
			return err
		}
		r.app.SetFrameCount(n)
		return nil

	case ActionHoverTable:
		var name string
		if err := step.Decode(&name); err != nil {
			return err
		}
		r.app.HoverTable(name)
		return nil

	case ActionHoverGroup:
		var name string
		if err := step.Decode(&name); err != nil {
			return err
		}
		r.app.HoverGroup(name)
		return nil

	case ActionSelectTables:
		var names []string
		if err := step.Decode(&names); err != nil {
			return err
		}
		r.app.SelectTables(names)
		return nil

	case ActionSelectAnnotations:
		var ids []string
		if err := step.Decode(&ids); err != nil {
			return err
		}
		r.app.SelectAnnotations(ids)
		return nil

	case ActionClearSelection:
		r.app.ClearSelection()
		return nil

	case ActionRenameTable:
		var args RenameTableArgs
		if err := step.Decode(&args); err != nil {
			return err
		}
		return r.applyEditResult(dbmledit.RewriteTableName(r.app.DBMLPath(), args.Old, args.New))

	case ActionRenameColumn:
		var args RenameColumnArgs
		if err := step.Decode(&args); err != nil {
			return err
		}
		return r.applyEditResult(dbmledit.RewriteColumnName(r.app.DBMLPath(), args.Table, args.Column, args.New))

	case ActionChangeType:
		var args ChangeTypeArgs
		if err := step.Decode(&args); err != nil {
			return err
		}
		return r.applyEditResult(dbmledit.RewriteColumnType(r.app.DBMLPath(), args.Table, args.Column, args.Type))

	case ActionAddTable:
		var name string
		if err := step.Decode(&name); err != nil {
			return err
		}
		return r.applyEditResult(dbmledit.AddTable(r.app.DBMLPath(), name))

	case ActionAddColumn:
		var args AddColumnArgs
		if err := step.Decode(&args); err != nil {
			return err
		}
		return r.applyEditResult(dbmledit.AddColumn(r.app.DBMLPath(), args.Table, args.Name, args.Type))

	case ActionRemoveTable:
		var name string
		if err := step.Decode(&name); err != nil {
			return err
		}
		return r.applyEditResult(dbmledit.RemoveTable(r.app.DBMLPath(), name))

	case ActionRemoveColumn:
		var args RemoveColumnArgs
		if err := step.Decode(&args); err != nil {
			return err
		}
		return r.applyEditResult(dbmledit.RemoveColumn(r.app.DBMLPath(), args.Table, args.Column))

	case ActionUndo:
		r.app.Undo()
		return nil

	case ActionRedo:
		r.app.Redo()
		return nil

	default:
		return fmt.Errorf("unknown action %q", step.Kind)
	}
}

func (r *Runner) applyEditResult(res *dbmledit.Result, err error) error {
	if err != nil {
		return err
	}
	if res != nil {
		r.app.AbsorbDBMLEdit(res.NewBytes, res.Schema)
	}
	return nil
}

func savePNG(screen *ebiten.Image, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	bounds := screen.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	screen.ReadPixels(rgba.Pix)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return encodePNG(f, rgba)
}

func encodePNG(w io.Writer, img image.Image) error { return png.Encode(w, img) }

// Verify dbml import is used (silences unused-import warnings if Compile
// drops the schema sync). Real use is via the runner driving edits.
var _ = dbml.ParseFile
