package routing

import "math"

func PathLength(pts []Point) float64 {
	total := 0.0
	for i := 0; i < len(pts)-1; i++ {
		a, b := pts[i], pts[i+1]
		total += math.Hypot(b.X-a.X, b.Y-a.Y)
	}
	return total
}

// PointAtT returns the point at fractional distance t in [0,1] along the polyline.
func PointAtT(pts []Point, t float64) Point {
	if len(pts) == 0 {
		return Point{}
	}
	if len(pts) == 1 || t <= 0 {
		return pts[0]
	}
	if t >= 1 {
		return pts[len(pts)-1]
	}
	total := PathLength(pts)
	target := total * t
	accum := 0.0
	for i := 0; i < len(pts)-1; i++ {
		a, b := pts[i], pts[i+1]
		seg := math.Hypot(b.X-a.X, b.Y-a.Y)
		if accum+seg >= target {
			local := (target - accum) / seg
			return Point{
				X: a.X + local*(b.X-a.X),
				Y: a.Y + local*(b.Y-a.Y),
			}
		}
		accum += seg
	}
	return pts[len(pts)-1]
}

func DistanceToPolyline(px, py float64, pts []Point) float64 {
	if len(pts) < 2 {
		return math.Inf(1)
	}
	min := math.Inf(1)
	for i := 0; i < len(pts)-1; i++ {
		d := distancePointSegment(px, py, pts[i], pts[i+1])
		if d < min {
			min = d
		}
	}
	return min
}

func distancePointSegment(px, py float64, a, b Point) float64 {
	abx := b.X - a.X
	aby := b.Y - a.Y
	lenSq := abx*abx + aby*aby
	if lenSq == 0 {
		dx := px - a.X
		dy := py - a.Y
		return math.Sqrt(dx*dx + dy*dy)
	}
	t := ((px-a.X)*abx + (py-a.Y)*aby) / lenSq
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	cx := a.X + t*abx
	cy := a.Y + t*aby
	dx := px - cx
	dy := py - cy
	return math.Sqrt(dx*dx + dy*dy)
}

type Box struct {
	X, Y, W, H float64
}

type Point struct {
	X, Y float64
}

func ZRoute(from, to Box) []Point {
	fcx := from.X + from.W/2
	fcy := from.Y + from.H/2
	tcx := to.X + to.W/2
	tcy := to.Y + to.H/2

	dx := tcx - fcx
	dy := tcy - fcy

	if math.Abs(dx) >= math.Abs(dy) {
		return horizontalRoute(from, to, dx)
	}
	return verticalRoute(from, to, dy)
}

func horizontalRoute(from, to Box, dx float64) []Point {
	var sx, ex float64
	if dx > 0 {
		sx = from.X + from.W
		ex = to.X
	} else {
		sx = from.X
		ex = to.X + to.W
	}
	sy := from.Y + from.H/2
	ey := to.Y + to.H/2
	midX := (sx + ex) / 2
	return []Point{
		{sx, sy},
		{midX, sy},
		{midX, ey},
		{ex, ey},
	}
}

func verticalRoute(from, to Box, dy float64) []Point {
	var sy, ey float64
	if dy > 0 {
		sy = from.Y + from.H
		ey = to.Y
	} else {
		sy = from.Y
		ey = to.Y + to.H
	}
	sx := from.X + from.W/2
	ex := to.X + to.W/2
	midY := (sy + ey) / 2
	return []Point{
		{sx, sy},
		{sx, midY},
		{ex, midY},
		{ex, ey},
	}
}
