package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const GroupPadding = 14.0
const GroupHeaderH = 18.0

func DrawGroup(dst *ebiten.Image, sx, sy, sw, sh float64, name string, clr color.NRGBA, scale float64) {
	x := float32(sx)
	y := float32(sy)
	w := float32(sw)
	h := float32(sh)

	fill := withAlpha(clr, 0x18)
	border := withAlpha(clr, 0x70)

	vector.FillRect(dst, x, y, w, h, fill, false)
	vector.StrokeRect(dst, x, y, w, h, 1, border, false)

	if name != "" {
		labelClr := withAlpha(clr, 0xff)
		drawText(dst, name, sx+8*scale, sy+4*scale, scale, labelClr)
	}
}
