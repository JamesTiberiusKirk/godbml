package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
	"github.com/JamesTiberiusKirk/godbml/internal/theme"
)

const (
	TablePadX     = 12.0
	TableHeaderH  = 28.0
	TableRowH     = 20.0
	TableMinWidth = 220.0
	TableMaxWidth = 380.0
)

type TableBox struct {
	X, Y, W, H float64
}

func (b TableBox) Contains(x, y float64) bool {
	return x >= b.X && x <= b.X+b.W && y >= b.Y && y <= b.Y+b.H
}

func MeasureTable(t *dbml.Table) TableBox {
	maxRow := TextWidth(t.Name)
	for _, c := range t.Columns {
		w := TextWidth(c.Name + "  " + c.Type)
		if c.PK {
			w += TextWidth(" PK")
		}
		if w > maxRow {
			maxRow = w
		}
	}
	w := maxRow + 2*TablePadX
	if w < TableMinWidth {
		w = TableMinWidth
	}
	if w > TableMaxWidth {
		w = TableMaxWidth
	}
	h := TableHeaderH + float64(len(t.Columns))*TableRowH
	return TableBox{W: w, H: h}
}

func DrawTable(dst *ebiten.Image, t *dbml.Table, sx, sy, scale float64, accent color.Color, selected bool) {
	box := MeasureTable(t)
	w := float32(box.W * scale)
	h := float32(box.H * scale)
	x := float32(sx)
	y := float32(sy)

	vector.FillRect(dst, x, y, w, h, theme.ColorSurface, false)

	headerH := float32(TableHeaderH * scale)
	var headerColor color.Color = withAlpha(theme.ColorAccent, 0x33)
	if c, ok := accent.(color.NRGBA); ok {
		headerColor = withAlpha(c, 0x55)
	} else if accent != nil {
		headerColor = accent
	}
	vector.FillRect(dst, x, y, w, headerH, headerColor, false)

	borderColor := theme.ColorBorder
	if selected {
		borderColor = theme.ColorAccent
	}
	stroke := float32(1)
	if selected {
		stroke = 2
	}
	vector.StrokeRect(dst, x, y, w, h, stroke, borderColor, false)
	vector.StrokeLine(dst, x, y+headerH, x+w, y+headerH, 1, theme.ColorBorder, false)

	headerOffset := (TableHeaderH - 14) / 2
	drawHeaderText(dst, t.Name, sx+TablePadX*scale, sy+headerOffset*scale, scale, theme.ColorText)

	for i, c := range t.Columns {
		rowY := sy + (TableHeaderH+float64(i)*TableRowH+(TableRowH-13)/2)*scale
		nameColor := color.Color(theme.ColorText)
		if c.PK {
			nameColor = theme.ColorAccent
		}
		drawText(dst, c.Name, sx+TablePadX*scale, rowY, scale, nameColor)

		typeStr := c.Type
		typeW := TextWidth(typeStr)
		typeX := sx + (box.W-TablePadX)*scale - typeW*scale
		drawText(dst, typeStr, typeX, rowY, scale, theme.ColorTextMuted)
	}
}

func drawText(dst *ebiten.Image, s string, sx, sy, scale float64, clr color.Color) {
	DrawText(dst, s, sx, sy, scale, clr)
}

func DrawText(dst *ebiten.Image, s string, sx, sy, scale float64, clr color.Color) {
	opts := &text.DrawOptions{}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate(sx, sy)
	opts.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, BodyFace(), opts)
}

func drawHeaderText(dst *ebiten.Image, s string, sx, sy, scale float64, clr color.Color) {
	opts := &text.DrawOptions{}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate(sx, sy)
	opts.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, HeaderFace(), opts)
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
