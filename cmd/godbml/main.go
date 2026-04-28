package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/JamesTiberiusKirk/godbml/internal/ui"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: godbml <schema.dbml>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	dbmlPath := flag.Arg(0)
	if _, err := os.Stat(dbmlPath); err != nil {
		log.Fatalf("dbml file: %v", err)
	}

	app, err := ui.NewApp(dbmlPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	defer app.Close()

	w, h := app.WindowSize()
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowTitle("godbml — " + dbmlPath)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(app); err != nil {
		log.Fatal(err)
	}
}
