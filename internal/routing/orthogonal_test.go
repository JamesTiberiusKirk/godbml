package routing

import "testing"

func TestZRouteHorizontalRight(t *testing.T) {
	from := Box{X: 0, Y: 0, W: 100, H: 50}
	to := Box{X: 300, Y: 0, W: 100, H: 50}
	pts := ZRoute(from, to)
	if len(pts) != 4 {
		t.Fatalf("waypoints = %d, want 4", len(pts))
	}
	if pts[0].X != 100 {
		t.Errorf("start X = %.1f, want 100 (right edge of from)", pts[0].X)
	}
	if pts[3].X != 300 {
		t.Errorf("end X = %.1f, want 300 (left edge of to)", pts[3].X)
	}
}

func TestZRouteHorizontalLeft(t *testing.T) {
	from := Box{X: 300, Y: 0, W: 100, H: 50}
	to := Box{X: 0, Y: 0, W: 100, H: 50}
	pts := ZRoute(from, to)
	if pts[0].X != 300 {
		t.Errorf("start X = %.1f, want 300 (left edge of from)", pts[0].X)
	}
	if pts[3].X != 100 {
		t.Errorf("end X = %.1f, want 100 (right edge of to)", pts[3].X)
	}
}

func TestZRouteVertical(t *testing.T) {
	from := Box{X: 0, Y: 0, W: 100, H: 50}
	to := Box{X: 10, Y: 300, W: 100, H: 50}
	pts := ZRoute(from, to)
	if pts[0].Y != 50 {
		t.Errorf("start Y = %.1f, want 50 (bottom of from)", pts[0].Y)
	}
	if pts[3].Y != 300 {
		t.Errorf("end Y = %.1f, want 300 (top of to)", pts[3].Y)
	}
}

func TestZRouteAllSegmentsAxisAligned(t *testing.T) {
	from := Box{X: 0, Y: 0, W: 100, H: 50}
	to := Box{X: 400, Y: 200, W: 100, H: 50}
	pts := ZRoute(from, to)
	for i := 0; i < len(pts)-1; i++ {
		a, b := pts[i], pts[i+1]
		if a.X != b.X && a.Y != b.Y {
			t.Errorf("segment %d not axis-aligned: %+v -> %+v", i, a, b)
		}
	}
}
