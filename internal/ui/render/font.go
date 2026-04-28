package render

import (
	"bytes"
	_ "embed"
	"log"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

//go:embed fonts/NotoSansMono-Regular.ttf
var notoSansMonoTTF []byte

const (
	bodyFontSize   = 13.0
	headerFontSize = 14.0
)

var (
	fontSource *text.GoTextFaceSource
	bodyFace   text.Face
	headerFace text.Face
)

func init() {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(notoSansMonoTTF))
	if err != nil {
		log.Fatalf("load embedded font: %v", err)
	}
	fontSource = src
	bodyFace = &text.GoTextFace{Source: src, Size: bodyFontSize}
	headerFace = &text.GoTextFace{Source: src, Size: headerFontSize}
}

func BodyFace() text.Face   { return bodyFace }
func HeaderFace() text.Face { return headerFace }

func TextWidth(s string) float64 {
	return text.Advance(s, bodyFace)
}

// LineHeight returns the body font's vertical advance.
func LineHeight() float64 {
	m := bodyFace.Metrics()
	return m.HAscent + m.HDescent
}
