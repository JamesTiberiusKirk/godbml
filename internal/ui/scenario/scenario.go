// Package scenario reads and executes YAML-defined visual test scenarios
// against the godbml UI, producing PNG snapshots for inspection.
package scenario

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Scenario describes one visual test: a schema, window dimensions, and a
// sequence of state-mutation + screenshot steps.
type Scenario struct {
	Schema string         `yaml:"schema"`
	Window Window         `yaml:"window"`
	OutDir string         `yaml:"out_dir"`
	Steps  []Step         `yaml:"steps"`
	Path   string         `yaml:"-"` // populated post-load: source file path
}

type Window struct {
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
}

// Step is one action in a scenario. Each YAML list entry is a single-key map
// where the key names the action (`shoot`, `hover_table`, etc.) and the value
// carries any arguments. Args is decoded lazily by the runner per-action.
type Step struct {
	Kind string
	Args yaml.Node
}

func (s *Step) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode || len(value.Content) != 2 {
		return fmt.Errorf("step must be a single-key map (got %d entries)", len(value.Content)/2)
	}
	s.Kind = value.Content[0].Value
	s.Args = *value.Content[1]
	return nil
}

// Decode populates `into` from the step's args. Use scalar types (string, int,
// bool) for short-form steps like `shoot: foo.png`, struct types for nested
// args like `camera: {x, y, zoom}`.
func (s *Step) Decode(into interface{}) error {
	return s.Args.Decode(into)
}

// Load reads and parses a scenario file. Defaults the window to 1280×800 and
// out_dir next to the scenario file if either is missing.
func Load(path string) (*Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc Scenario
	if err := yaml.Unmarshal(b, &sc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if sc.Window.Width <= 0 {
		sc.Window.Width = 1280
	}
	if sc.Window.Height <= 0 {
		sc.Window.Height = 800
	}
	sc.Path = path
	return &sc, nil
}

// Action constants. Kept as string consts so a typo at parse-time produces a
// clear "unknown action" error rather than a silent no-op.
const (
	ActionShoot              = "shoot"
	ActionCamera             = "camera"
	ActionFit                = "fit"
	ActionAdvanceFrames      = "advance_frames"
	ActionSetFrame           = "set_frame"
	ActionHoverTable         = "hover_table"
	ActionHoverGroup         = "hover_group"
	ActionSelectTables       = "select_tables"
	ActionSelectAnnotations  = "select_annotations"
	ActionClearSelection     = "clear_selection"
	ActionRenameTable        = "rename_table"
	ActionRenameColumn       = "rename_column"
	ActionChangeType         = "change_type"
	ActionAddTable           = "add_table"
	ActionAddColumn          = "add_column"
	ActionRemoveTable        = "remove_table"
	ActionRemoveColumn       = "remove_column"
	ActionUndo               = "undo"
	ActionRedo               = "redo"
)

// Argument decoders for the multi-arg actions. Single-arg actions decode
// directly into a scalar (string / int / bool).

type CameraArgs struct {
	X    float64 `yaml:"x"`
	Y    float64 `yaml:"y"`
	Zoom float64 `yaml:"zoom"`
	Fit  bool    `yaml:"fit"`
}

type RenameTableArgs struct {
	Old string `yaml:"old"`
	New string `yaml:"new"`
}

type RenameColumnArgs struct {
	Table  string `yaml:"table"`
	Column string `yaml:"column"`
	New    string `yaml:"new"`
}

type ChangeTypeArgs struct {
	Table  string `yaml:"table"`
	Column string `yaml:"column"`
	Type   string `yaml:"type"`
}

type AddColumnArgs struct {
	Table string `yaml:"table"`
	Name  string `yaml:"name"`
	Type  string `yaml:"type"`
}

type RemoveColumnArgs struct {
	Table  string `yaml:"table"`
	Column string `yaml:"column"`
}
