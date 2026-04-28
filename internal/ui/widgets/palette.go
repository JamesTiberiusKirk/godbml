package widgets

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/JamesTiberiusKirk/godbml/internal/theme"
)

const (
	SwatchSz   = 22.0
	SwatchPad  = 4.0
	PaletteCol = 6
)

type Palette struct {
	X, Y    float64
	OnPick  func(c color.NRGBA)
	OnClear func()
	hovered int
}

func (p *Palette) entryCount() int {
	return len(theme.Palette) + 1
}

func (p *Palette) Width() float64 {
	return float64(PaletteCol)*SwatchSz + float64(PaletteCol+1)*SwatchPad
}

func (p *Palette) Height() float64 {
	rows := (p.entryCount() + PaletteCol - 1) / PaletteCol
	return float64(rows)*SwatchSz + float64(rows+1)*SwatchPad
}

func (p *Palette) cellAt(idx int) (float64, float64) {
	row := idx / PaletteCol
	col := idx % PaletteCol
	x := p.X + SwatchPad + float64(col)*(SwatchSz+SwatchPad)
	y := p.Y + SwatchPad + float64(row)*(SwatchSz+SwatchPad)
	return x, y
}

func (p *Palette) hitIndex(mx, my int) int {
	x := float64(mx)
	y := float64(my)
	for i := 0; i < p.entryCount(); i++ {
		cx, cy := p.cellAt(i)
		if x >= cx && x <= cx+SwatchSz && y >= cy && y <= cy+SwatchSz {
			return i
		}
	}
	return -1
}

func (p *Palette) Contains(mx, my int) bool {
	x := float64(mx)
	y := float64(my)
	return x >= p.X && x <= p.X+p.Width() && y >= p.Y && y <= p.Y+p.Height()
}

// PaletteResult tells the caller what the user did.
type PaletteResult int

const (
	PaletteNoop PaletteResult = iota
	PalettePicked
	PaletteCleared
	PaletteDismissed
)

// Update reports the user's action and the picked colour (if any).
func (p *Palette) Update(mx, my int, clicked bool) (color.NRGBA, PaletteResult) {
	p.hovered = p.hitIndex(mx, my)
	if !clicked {
		return color.NRGBA{}, PaletteNoop
	}
	if !p.Contains(mx, my) {
		return color.NRGBA{}, PaletteDismissed
	}
	if p.hovered < 0 {
		return color.NRGBA{}, PaletteNoop
	}
	if p.hovered == len(theme.Palette) {
		return color.NRGBA{}, PaletteCleared
	}
	return theme.Palette[p.hovered], PalettePicked
}

func (p *Palette) Draw(dst *ebiten.Image) {
	bg := theme.ColorSurface
	bg.A = 0xee
	vector.FillRect(dst, float32(p.X), float32(p.Y), float32(p.Width()), float32(p.Height()), bg, false)
	vector.StrokeRect(dst, float32(p.X), float32(p.Y), float32(p.Width()), float32(p.Height()), 1, theme.ColorBorder, false)

	for i, swatch := range theme.Palette {
		x, y := p.cellAt(i)
		vector.FillRect(dst, float32(x), float32(y), float32(SwatchSz), float32(SwatchSz), swatch, false)
		if i == p.hovered {
			vector.StrokeRect(dst, float32(x-1), float32(y-1), float32(SwatchSz+2), float32(SwatchSz+2), 2, theme.ColorAccent, false)
		} else {
			vector.StrokeRect(dst, float32(x), float32(y), float32(SwatchSz), float32(SwatchSz), 1, theme.ColorBorder, false)
		}
	}
	// "clear" cell: no fill, with X
	cx, cy := p.cellAt(len(theme.Palette))
	vector.StrokeRect(dst, float32(cx), float32(cy), float32(SwatchSz), float32(SwatchSz), 1, theme.ColorTextMuted, false)
	vector.StrokeLine(dst, float32(cx+4), float32(cy+4), float32(cx+SwatchSz-4), float32(cy+SwatchSz-4), 1, theme.ColorTextMuted, false)
	vector.StrokeLine(dst, float32(cx+SwatchSz-4), float32(cy+4), float32(cx+4), float32(cy+SwatchSz-4), 1, theme.ColorTextMuted, false)
	if p.hovered == len(theme.Palette) {
		vector.StrokeRect(dst, float32(cx-1), float32(cy-1), float32(SwatchSz+2), float32(SwatchSz+2), 2, theme.ColorAccent, false)
	}
}
