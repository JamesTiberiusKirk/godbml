package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/JamesTiberiusKirk/godbml/internal/routing"
	"github.com/JamesTiberiusKirk/godbml/internal/theme"
)

// CardinalityKind selects which ER marker to draw at a relationship endpoint.
type CardinalityKind int

const (
	CardNone CardinalityKind = iota
	CardOne
	CardMany
)

const (
	LineWidth      = 3.0
	MarkerWidth    = 3.0
	PulseRadius    = 3.2
	PulseSpacing   = 90.0 // world-units between pulses; screen px once multiplied by zoom
	PulseSpeed     = 0.7  // world-units per frame
	PulseFadeDist  = 22.0 // world-units of fade at each endpoint
	GlowMultiplier = 2.0  // line glow halo width = LineWidth * this when active
	GlowAlpha      = 0x55 // halo opacity (0..255)
	ActiveLineMult = 1.4  // active line core thickness relative to LineWidth
)

// CullMargin is how many extra pixels around the screen we still draw, so
// strokes and pulse halos near the edge don't pop in/out at the seam.
const CullMargin = 64.0

func DrawRelationship(dst *ebiten.Image, fromBox, toBox TableBox, fromCard, toCard CardinalityKind, scale float64, frame int, clr color.Color, screenW, screenH int, pulse bool) {
	from := routing.Box{X: fromBox.X, Y: fromBox.Y, W: fromBox.W, H: fromBox.H}
	to := routing.Box{X: toBox.X, Y: toBox.Y, W: toBox.W, H: toBox.H}
	pts := routing.ZRoute(from, to)
	if len(pts) < 2 {
		return
	}
	if !pathIntersectsViewport(pts, screenW, screenH) {
		return
	}

	// When the relationship is "active" (its table is hovered or selected), we
	// promote the line to the saturated pulse colour and underlay each segment
	// with a translucent halo for a neon glow. Active stroke + halo widths
	// scale with the canvas zoom so the line stays proportional to the tables
	// it connects. The line is built as a single Path (not per-segment Line
	// calls) so corners join with rounded line-joins instead of leaving gaps.
	strokeColor := clr
	var glow color.NRGBA
	lineWidth := float32(LineWidth)
	glowWidth := float32(LineWidth * GlowMultiplier)
	if pulse {
		bright := pulseColor(clr)
		strokeColor = bright
		glow = bright
		glow.A = GlowAlpha
		lineWidth = float32(LineWidth * ActiveLineMult * scale)
		if lineWidth < 1.5 {
			lineWidth = 1.5
		}
		glowWidth = float32(LineWidth * GlowMultiplier * scale)
		if glowWidth < lineWidth {
			glowWidth = lineWidth
		}
	}

	var path vector.Path
	path.MoveTo(float32(pts[0].X), float32(pts[0].Y))
	for i := 1; i < len(pts); i++ {
		path.LineTo(float32(pts[i].X), float32(pts[i].Y))
	}

	if pulse {
		strokePath(dst, &path, glowWidth, glow)
	}
	strokePath(dst, &path, lineWidth, strokeColor)

	if pulse {
		drawPulses(dst, pts, frame, scale, strokeColor, screenW, screenH)
	}

	if inViewport(pts[0], screenW, screenH) {
		drawCardinalityMarker(dst, pts[0], pts[1], fromCard, scale, strokeColor)
	}
	if inViewport(pts[len(pts)-1], screenW, screenH) {
		drawCardinalityMarker(dst, pts[len(pts)-1], pts[len(pts)-2], toCard, scale, strokeColor)
	}
}

func inViewport(p routing.Point, screenW, screenH int) bool {
	return p.X >= -CullMargin && p.X <= float64(screenW)+CullMargin &&
		p.Y >= -CullMargin && p.Y <= float64(screenH)+CullMargin
}

func pathIntersectsViewport(pts []routing.Point, screenW, screenH int) bool {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return maxX >= -CullMargin && minX <= float64(screenW)+CullMargin &&
		maxY >= -CullMargin && minY <= float64(screenH)+CullMargin
}

func segmentIntersectsViewport(a, b routing.Point, screenW, screenH int) bool {
	minX, maxX := a.X, b.X
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := a.Y, b.Y
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return maxX >= -CullMargin && minX <= float64(screenW)+CullMargin &&
		maxY >= -CullMargin && minY <= float64(screenH)+CullMargin
}

// drawPulses places pulses at constant world-space spacing along the line,
// regardless of line length. Pulses scroll over time at constant per-frame
// speed. Both spacing and speed scale with zoom so visual density and motion
// stay consistent relative to the tables.
//
// Direction: to-side → from-side (parent → child for `>` refs), per dbdiagram.
func drawPulses(dst *ebiten.Image, pts []routing.Point, frame int, scale float64, lineColor color.Color, screenW, screenH int) {
	pathLen := routing.PathLength(pts)
	if pathLen < 1 {
		return
	}
	spacing := PulseSpacing * scale
	speed := PulseSpeed * scale
	fade := PulseFadeDist * scale
	if spacing <= 0 || speed <= 0 {
		return
	}

	scroll := math.Mod(float64(frame)*speed, spacing)
	if scroll < 0 {
		scroll += spacing
	}

	r := PulseRadius * scale
	if r < 2.0 {
		r = 2.0
	}
	base := pulseColor(lineColor)

	for d := scroll; d < pathLen; d += spacing {
		t := 1 - d/pathLen
		pos := routing.PointAtT(pts, t)
		if !inViewport(pos, screenW, screenH) {
			continue
		}

		alpha := 1.0
		switch {
		case d < fade:
			alpha = d / fade
		case d > pathLen-fade:
			alpha = (pathLen - d) / fade
		}
		if alpha <= 0 {
			continue
		}
		c := base
		c.A = uint8(float64(c.A) * alpha)
		halo := c
		halo.A = uint8(float64(c.A) * 0.35)
		vector.FillCircle(dst, float32(pos.X), float32(pos.Y), float32(r*1.9), halo, true)
		vector.FillCircle(dst, float32(pos.X), float32(pos.Y), float32(r), c, true)
	}
}

// strokePath draws an antialiased polyline with rounded joins and caps so
// orthogonal-routing bends meet smoothly instead of leaving gaps where
// per-segment StrokeLine ends would butt.
func strokePath(dst *ebiten.Image, path *vector.Path, width float32, clr color.Color) {
	stroke := &vector.StrokeOptions{
		Width:    width,
		LineCap:  vector.LineCapRound,
		LineJoin: vector.LineJoinRound,
	}
	dop := &vector.DrawPathOptions{AntiAlias: true}
	dop.ColorScale.ScaleWithColor(clr)
	vector.StrokePath(dst, path, stroke, dop)
}

// pulseColor returns the colour used for pulses. If the relationship has the
// default (muted) line colour, pulses use the theme accent; otherwise they
// inherit the user-chosen line colour at full saturation so per-line colouring
// stays visually coherent.
func pulseColor(lineColor color.Color) color.NRGBA {
	r, g, b, _ := lineColor.RGBA()
	dr, dg, db, _ := theme.ColorLine.RGBA()
	if r == dr && g == dg && b == db {
		return theme.ColorPulse
	}
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0xff}
}

// drawCardinalityMarker draws an ER cardinality marker at `at`, with the line
// continuing toward `toward`. Marker geometry scales with the canvas zoom so
// it grows/shrinks alongside the tables.
func drawCardinalityMarker(dst *ebiten.Image, at, toward routing.Point, kind CardinalityKind, scale float64, clr color.Color) {
	dx := toward.X - at.X
	dy := toward.Y - at.Y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	ux, uy := dx/d, dy/d
	px, py := -uy, ux

	switch kind {
	case CardOne:
		bar := 5.0 * scale
		inset := 7.0 * scale
		cx := at.X + ux*inset
		cy := at.Y + uy*inset
		vector.StrokeLine(dst,
			float32(cx+px*bar), float32(cy+py*bar),
			float32(cx-px*bar), float32(cy-py*bar),
			MarkerWidth, clr, true)
	case CardMany:
		fork := 6.0 * scale
		reach := 12.0 * scale
		baseX := at.X + ux*reach
		baseY := at.Y + uy*reach
		vector.StrokeLine(dst,
			float32(baseX), float32(baseY),
			float32(at.X+px*fork), float32(at.Y+py*fork),
			MarkerWidth, clr, true)
		vector.StrokeLine(dst,
			float32(baseX), float32(baseY),
			float32(at.X-px*fork), float32(at.Y-py*fork),
			MarkerWidth, clr, true)
	}
}
