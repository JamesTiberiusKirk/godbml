package ui

import "github.com/hajimehoshi/ebiten/v2"

type Camera struct {
	X, Y float64
	Zoom float64
}

const (
	minZoom = 0.1
	maxZoom = 8.0
)

func NewCamera() *Camera {
	return &Camera{Zoom: 1.0}
}

func (c *Camera) ScreenToWorld(sx, sy float64) (float64, float64) {
	return sx/c.Zoom + c.X, sy/c.Zoom + c.Y
}

func (c *Camera) WorldToScreen(wx, wy float64) (float64, float64) {
	return (wx - c.X) * c.Zoom, (wy - c.Y) * c.Zoom
}

func (c *Camera) Pan(dxScreen, dyScreen float64) {
	c.X -= dxScreen / c.Zoom
	c.Y -= dyScreen / c.Zoom
}

func (c *Camera) ZoomAt(sx, sy, factor float64) {
	wx, wy := c.ScreenToWorld(sx, sy)
	c.Zoom *= factor
	if c.Zoom < minZoom {
		c.Zoom = minZoom
	}
	if c.Zoom > maxZoom {
		c.Zoom = maxZoom
	}
	c.X = wx - sx/c.Zoom
	c.Y = wy - sy/c.Zoom
}

func (c *Camera) GeoM() ebiten.GeoM {
	var g ebiten.GeoM
	g.Translate(-c.X, -c.Y)
	g.Scale(c.Zoom, c.Zoom)
	return g
}
