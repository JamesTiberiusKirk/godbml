package widgets

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/JamesTiberiusKirk/godbml/internal/theme"
	"github.com/JamesTiberiusKirk/godbml/internal/ui/render"
)

const (
	MenuItemH    = 22.0
	MenuItemPadX = 12.0
	MenuMinWidth = 180.0
)

type MenuItem struct {
	Label    string
	Disabled bool
	Sep      bool
	Action   func()
}

type Menu struct {
	X, Y    float64
	Items   []MenuItem
	hovered int
}

func (m *Menu) Width() float64 {
	w := MenuMinWidth
	for _, it := range m.Items {
		tw := render.TextWidth(it.Label) + 2*MenuItemPadX
		if tw > w {
			w = tw
		}
	}
	return w
}

func (m *Menu) Height() float64 {
	h := 0.0
	for _, it := range m.Items {
		if it.Sep {
			h += 6
		} else {
			h += MenuItemH
		}
	}
	return h + 4
}

func (m *Menu) Bounds() (x, y, w, h float64) {
	return m.X, m.Y, m.Width(), m.Height()
}

func (m *Menu) Contains(px, py float64) bool {
	x, y, w, h := m.Bounds()
	return px >= x && px <= x+w && py >= y && py <= y+h
}

func (m *Menu) hitIndex(px, py float64) int {
	x, _, w, _ := m.Bounds()
	if px < x || px > x+w {
		return -1
	}
	cy := m.Y + 2
	for i, it := range m.Items {
		var rowH float64
		if it.Sep {
			rowH = 6
		} else {
			rowH = MenuItemH
		}
		if py >= cy && py < cy+rowH && !it.Sep && !it.Disabled {
			return i
		}
		cy += rowH
	}
	return -1
}

// Update returns the index of an activated item, or -1, and a flag indicating
// whether the menu should be dismissed.
func (m *Menu) Update(mx, my int, clicked bool) (activated int, dismiss bool) {
	m.hovered = m.hitIndex(float64(mx), float64(my))
	if !clicked {
		return -1, false
	}
	if !m.Contains(float64(mx), float64(my)) {
		return -1, true
	}
	if m.hovered >= 0 {
		return m.hovered, true
	}
	return -1, false
}

func (m *Menu) Draw(dst *ebiten.Image) {
	x, y, w, h := m.Bounds()
	bg := theme.ColorSurface
	bg.A = 0xee
	vector.FillRect(dst, float32(x), float32(y), float32(w), float32(h), bg, false)
	vector.StrokeRect(dst, float32(x), float32(y), float32(w), float32(h), 1, theme.ColorBorder, false)

	cy := y + 2
	for i, it := range m.Items {
		if it.Sep {
			vector.StrokeLine(dst, float32(x+6), float32(cy+3), float32(x+w-6), float32(cy+3), 1, theme.ColorBorder, false)
			cy += 6
			continue
		}
		if i == m.hovered && !it.Disabled {
			hov := theme.ColorAccent
			hov.A = 0x22
			vector.FillRect(dst, float32(x+1), float32(cy), float32(w-2), float32(MenuItemH), hov, false)
		}
		var clr color.Color = theme.ColorText
		if it.Disabled {
			clr = theme.ColorTextMuted
		}
		render.DrawText(dst, it.Label, x+MenuItemPadX, cy+(MenuItemH-13)/2, 1.0, clr)
		cy += MenuItemH
	}
}
