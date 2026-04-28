package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/JamesTiberiusKirk/godbml/internal/theme"
)

const (
	AnnotationPadX      = 8.0
	AnnotationPadY      = 6.0
	AnnotationHandleSz  = 10.0
	AnnotationMinWidth  = 80.0
	AnnotationMinHeight = 32.0
)

func DrawAnnotation(dst *ebiten.Image, sx, sy, sw, sh float64, scale float64, text string, accent color.NRGBA, selected, editing bool, caret string) {
	x := float32(sx)
	y := float32(sy)
	w := float32(sw)
	h := float32(sh)

	fill := withAlpha(accent, 0x22)
	border := withAlpha(accent, 0xa0)
	if selected || editing {
		border = withAlpha(accent, 0xff)
	}
	vector.FillRect(dst, x, y, w, h, fill, false)
	stroke := float32(1)
	if selected || editing {
		stroke = 2
	}
	vector.StrokeRect(dst, x, y, w, h, stroke, border, false)

	display := text
	if editing {
		display = text + caret
	}
	if display == "" {
		display = "(empty)"
		muted := theme.ColorTextMuted
		DrawText(dst, display, sx+AnnotationPadX*scale, sy+AnnotationPadY*scale, scale, muted)
	} else {
		DrawText(dst, display, sx+AnnotationPadX*scale, sy+AnnotationPadY*scale, scale, theme.ColorText)
	}

	hx := x + w - float32(AnnotationHandleSz*scale)
	hy := y + h - float32(AnnotationHandleSz*scale)
	hw := float32(AnnotationHandleSz * scale)
	vector.FillRect(dst, hx, hy, hw, hw, border, false)
}
