// Command godbml-test executes a YAML-described scenario against the godbml
// UI, writing one PNG per `shoot` step. Use it to inspect rendering changes
// without manually clicking through the live app.
//
//	go run ./cmd/godbml-test path/to/scenario.yaml
//
// Mutating scenarios (rename, add, remove) are run against a temp copy of
// the source schema; the original DBML file on disk is left untouched.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/JamesTiberiusKirk/godbml/internal/ui"
	"github.com/JamesTiberiusKirk/godbml/internal/ui/scenario"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: godbml-test <scenario.yaml>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	scenPath := flag.Arg(0)

	sc, err := scenario.Load(scenPath)
	if err != nil {
		log.Fatalf("load scenario: %v", err)
	}
	if sc.Schema == "" {
		log.Fatalf("scenario %s: missing required `schema` field", scenPath)
	}
	if sc.OutDir == "" {
		sc.OutDir = filepath.Join(filepath.Dir(scenPath), "shots-"+strings.TrimSuffix(filepath.Base(scenPath), filepath.Ext(scenPath)))
	}

	// Mutating scenarios get a throwaway copy of the schema so we never
	// clobber the user's source file.
	schemaPath := sc.Schema
	if mutates(sc) {
		tmp, err := copyToTemp(schemaPath)
		if err != nil {
			log.Fatalf("copy schema: %v", err)
		}
		defer os.RemoveAll(filepath.Dir(tmp))
		log.Printf("running against temp copy %s (mutating scenario)", tmp)
		schemaPath = tmp
	}

	if err := os.MkdirAll(sc.OutDir, 0o755); err != nil {
		log.Fatalf("mkdir out: %v", err)
	}
	log.Printf("schema=%s window=%dx%d out=%s steps=%d", schemaPath, sc.Window.Width, sc.Window.Height, sc.OutDir, len(sc.Steps))

	app, err := ui.NewApp(schemaPath)
	if err != nil {
		log.Fatalf("init app: %v", err)
	}
	defer app.Close()

	runner := scenario.NewRunner(app, sc)
	ebiten.SetWindowSize(sc.Window.Width, sc.Window.Height)
	ebiten.SetWindowTitle("godbml-test: " + filepath.Base(scenPath))
	if err := ebiten.RunGame(runner); err != nil {
		log.Fatalf("run scenario: %v", err)
	}
	if runner.Failed() {
		log.Print("scenario completed with errors")
		os.Exit(1)
	}
	log.Print("scenario completed")
}

func mutates(sc *scenario.Scenario) bool {
	for _, st := range sc.Steps {
		switch st.Kind {
		case scenario.ActionRenameTable, scenario.ActionRenameColumn,
			scenario.ActionChangeType, scenario.ActionAddTable,
			scenario.ActionAddColumn, scenario.ActionRemoveTable,
			scenario.ActionRemoveColumn:
			return true
		}
	}
	return false
}

func copyToTemp(src string) (string, error) {
	dir, err := os.MkdirTemp("", "godbml-test-")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(src))
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return dst, nil
}
