package layout

import (
	"math"
	"math/rand"
	"sort"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
)

type Position struct {
	X, Y float64
}

type Options struct {
	Iterations int
	Seed       int64
	Width      float64
	Height     float64
	Spacing    float64
	// Gravity is a per-iteration pull each node feels toward the centroid.
	// Force scales with distance² so it's effectively zero inside the natural
	// cluster (doesn't squeeze connected nodes) but ramps up sharply for far-
	// away outliers and disconnected components.
	// Typical values: 0.02–0.15. 0 disables it.
	Gravity float64
	// GravityRef is the reference distance (world units) used to normalise
	// the quadratic gravity. Roughly: at d == GravityRef, the gravity force
	// equals Gravity * GravityRef. Smaller values make gravity ramp faster.
	GravityRef float64
	// SizeOf returns each node's rendered size in world units. When provided,
	// repulsion is computed against the gap between nodes' bounding circles
	// (radius = max(w,h)/2) instead of treating nodes as points, so tall
	// tables don't overlap shorter neighbours. Optional — nil = point nodes.
	SizeOf func(name string) (w, h float64)
	// OverlapPadding is the minimum world-unit gap between any two table
	// bounding boxes after layout completes. A post-process resolver pushes
	// any pair tighter than this apart along the minimum-translation axis.
	// Requires SizeOf to be set; ignored otherwise. 0 disables.
	OverlapPadding float64
}

func DefaultOptions() Options {
	return Options{
		Iterations:     300,
		Seed:           42,
		Width:          1500,
		Height:         1000,
		Spacing:        300,
		Gravity:        0.06,
		GravityRef:     200,
		OverlapPadding: 28,
	}
}

func ForceDirected(schema *dbml.Schema, opts Options) map[string]Position {
	names := tableNames(schema)
	n := len(names)
	if n == 0 {
		return map[string]Position{}
	}

	rng := rand.New(rand.NewSource(opts.Seed))

	positions := make(map[string]Position, n)
	for _, name := range names {
		positions[name] = Position{
			X: rng.Float64() * opts.Width,
			Y: rng.Float64() * opts.Height,
		}
	}

	edges := dedupEdges(schema)

	k := opts.Spacing
	if k == 0 {
		k = math.Sqrt(opts.Width * opts.Height / float64(n))
	}

	temp := opts.Width / 10
	cool := temp / float64(opts.Iterations)

	disp := make(map[string]Position, n)

	for it := 0; it < opts.Iterations; it++ {
		for _, name := range names {
			disp[name] = Position{}
		}

		radii := make(map[string]float64, n)
		if opts.SizeOf != nil {
			for _, name := range names {
				w, h := opts.SizeOf(name)
				r := math.Max(w, h) / 2
				radii[name] = r
			}
		}

		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				a, b := names[i], names[j]
				pa, pb := positions[a], positions[b]
				dx := pa.X - pb.X
				dy := pa.Y - pb.Y
				d := math.Hypot(dx, dy)
				if d < 0.01 {
					dx = (rng.Float64() - 0.5)
					dy = (rng.Float64() - 0.5)
					d = math.Hypot(dx, dy)
					if d < 0.01 {
						d = 0.01
					}
				}
				gap := d - radii[a] - radii[b]
				if gap < 1 {
					gap = 1
				}
				fr := k * k / gap
				ux, uy := dx/d, dy/d
				da := disp[a]
				db := disp[b]
				disp[a] = Position{X: da.X + ux*fr, Y: da.Y + uy*fr}
				disp[b] = Position{X: db.X - ux*fr, Y: db.Y - uy*fr}
			}
		}

		for _, e := range edges {
			pa, pb := positions[e[0]], positions[e[1]]
			dx := pa.X - pb.X
			dy := pa.Y - pb.Y
			d := math.Hypot(dx, dy)
			if d < 0.01 {
				d = 0.01
			}
			fa := d * d / k
			ux, uy := dx/d, dy/d
			da := disp[e[0]]
			db := disp[e[1]]
			disp[e[0]] = Position{X: da.X - ux*fa, Y: da.Y - uy*fa}
			disp[e[1]] = Position{X: db.X + ux*fa, Y: db.Y + uy*fa}
		}

		// Quadratic gravity toward centroid: force ~ gravity * d² / ref.
		// Inside the natural cluster (small d) this is negligible; on outliers
		// and disconnected components (large d) it dominates and pulls them in
		// without crushing the main cluster.
		if opts.Gravity > 0 {
			ref := opts.GravityRef
			if ref <= 0 {
				ref = 200
			}
			var cx, cy float64
			for _, name := range names {
				cx += positions[name].X
				cy += positions[name].Y
			}
			cx /= float64(n)
			cy /= float64(n)
			for _, name := range names {
				p := positions[name]
				dxc := cx - p.X
				dyc := cy - p.Y
				d := math.Hypot(dxc, dyc)
				if d < 1 {
					continue
				}
				f := opts.Gravity * d * d / ref
				ux, uy := dxc/d, dyc/d
				da := disp[name]
				disp[name] = Position{X: da.X + ux*f, Y: da.Y + uy*f}
			}
		}

		for _, name := range names {
			p := positions[name]
			d := disp[name]
			mag := math.Hypot(d.X, d.Y)
			if mag > 0 {
				step := math.Min(mag, temp)
				positions[name] = Position{
					X: p.X + d.X/mag*step,
					Y: p.Y + d.Y/mag*step,
				}
			}
		}

		temp -= cool
		if temp < 0 {
			temp = 0
		}
	}

	if opts.SizeOf != nil && opts.OverlapPadding > 0 {
		resolveOverlaps(positions, names, opts.SizeOf, opts.OverlapPadding)
	}

	return centerAroundOrigin(positions, names)
}

// resolveOverlaps performs a deterministic AABB-overlap relaxation pass.
// For every pair of tables whose bounding boxes overlap (with `padding` worth
// of breathing room), push them apart along the axis of minimum translation.
// Repeats until no pair overlaps or maxIter is reached.
func resolveOverlaps(positions map[string]Position, names []string, sizeOf func(string) (float64, float64), padding float64) {
	const maxIter = 30
	n := len(names)
	for iter := 0; iter < maxIter; iter++ {
		moved := false
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				a, b := names[i], names[j]
				wa, ha := sizeOf(a)
				wb, hb := sizeOf(b)
				pa := positions[a]
				pb := positions[b]

				cax := pa.X + wa/2
				cay := pa.Y + ha/2
				cbx := pb.X + wb/2
				cby := pb.Y + hb/2

				dx := cbx - cax
				dy := cby - cay

				reqX := wa/2 + wb/2 + padding
				reqY := ha/2 + hb/2 + padding

				ox := reqX - math.Abs(dx)
				oy := reqY - math.Abs(dy)
				if ox <= 0 || oy <= 0 {
					continue
				}

				if ox < oy {
					push := ox / 2
					if dx >= 0 {
						pa.X -= push
						pb.X += push
					} else {
						pa.X += push
						pb.X -= push
					}
				} else {
					push := oy / 2
					if dy >= 0 {
						pa.Y -= push
						pb.Y += push
					} else {
						pa.Y += push
						pb.Y -= push
					}
				}
				positions[a] = pa
				positions[b] = pb
				moved = true
			}
		}
		if !moved {
			break
		}
	}
}

func PlaceAtEdge(existing map[string]Position, newName string) Position {
	if len(existing) == 0 {
		return Position{X: 0, Y: 0}
	}
	var maxX, sumY float64
	maxX = math.Inf(-1)
	for _, p := range existing {
		if p.X > maxX {
			maxX = p.X
		}
		sumY += p.Y
	}
	avgY := sumY / float64(len(existing))
	return Position{X: maxX + 320, Y: avgY}
}

func tableNames(s *dbml.Schema) []string {
	names := make([]string, 0, len(s.Tables))
	for _, t := range s.Tables {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names
}

func dedupEdges(s *dbml.Schema) [][2]string {
	seen := map[[2]string]bool{}
	var edges [][2]string
	for _, r := range s.Relationships {
		a, b := r.FromTable, r.ToTable
		if a == b || a == "" || b == "" {
			continue
		}
		if a > b {
			a, b = b, a
		}
		key := [2]string{a, b}
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, key)
	}
	return edges
}

func centerAroundOrigin(positions map[string]Position, names []string) map[string]Position {
	n := len(names)
	if n == 0 {
		return positions
	}
	var sx, sy float64
	for _, name := range names {
		sx += positions[name].X
		sy += positions[name].Y
	}
	cx := sx / float64(n)
	cy := sy / float64(n)
	for _, name := range names {
		p := positions[name]
		positions[name] = Position{X: p.X - cx, Y: p.Y - cy}
	}
	return positions
}
