package render

import (
	"bytes"
	_ "embed"
	"log"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Adwaita Sans is GNOME's modern UI font, purpose-built for screen text at
// small sizes — high x-height, open apertures, distinct shapes for c/e/o/a.
// We use it for everything (table names, column rows, widgets) so the diagram
// stays legible when zoomed far out. Right-aligning column types by measured
// width preserves the visual "name on left, type on right" layout without
// needing a monospace face.
//
//go:embed fonts/AdwaitaSans-Regular.ttf
var uiFontTTF []byte

const (
	bodyFontSize   = 13.0
	headerFontSize = 15.0
)

var (
	fontSource *text.GoTextFaceSource
	bodyFace   text.Face
	headerFace text.Face
)

func init() {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(uiFontTTF))
	if err != nil {
		log.Fatalf("load embedded font: %v", err)
	}
	fontSource = src
	bodyFace = &text.GoTextFace{Source: src, Size: bodyFontSize}
	headerFace = &text.GoTextFace{Source: src, Size: headerFontSize}
}

func BodyFace() text.Face   { return bodyFace }
func HeaderFace() text.Face { return headerFace }

func TextWidth(s string) float64       { return text.Advance(s, bodyFace) }
func HeaderTextWidth(s string) float64 { return text.Advance(s, headerFace) }

// LineHeight returns the body font's vertical advance.
func LineHeight() float64 {
	m := bodyFace.Metrics()
	return m.HAscent + m.HDescent
}
