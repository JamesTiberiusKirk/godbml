package layout

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
)

func TestForceDirectedDeterministic(t *testing.T) {
	s := loadExample(t)

	a := ForceDirected(s, DefaultOptions())
	b := ForceDirected(s, DefaultOptions())

	for name, pa := range a {
		pb, ok := b[name]
		if !ok {
			t.Fatalf("table %q missing in second run", name)
		}
		if math.Abs(pa.X-pb.X) > 1e-9 || math.Abs(pa.Y-pb.Y) > 1e-9 {
			t.Errorf("non-deterministic position for %q: %+v vs %+v", name, pa, pb)
		}
	}
}

func TestForceDirectedSeparatesAllTables(t *testing.T) {
	s := loadExample(t)
	pos := ForceDirected(s, DefaultOptions())

	if len(pos) != len(s.Tables) {
		t.Fatalf("position count = %d, want %d", len(pos), len(s.Tables))
	}

	const minDist = 50.0
	names := tableNames(s)
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			pa, pb := pos[names[i]], pos[names[j]]
			d := math.Hypot(pa.X-pb.X, pa.Y-pb.Y)
			if d < minDist {
				t.Errorf("tables %q and %q too close: %.1f (min %.0f)", names[i], names[j], d, minDist)
			}
		}
	}
}

func TestPlaceAtEdge(t *testing.T) {
	existing := map[string]Position{
		"a": {X: 0, Y: 0},
		"b": {X: 100, Y: 50},
	}
	p := PlaceAtEdge(existing, "c")
	if p.X <= 100 {
		t.Errorf("PlaceAtEdge X = %.1f, want > 100", p.X)
	}
}

func TestPlaceAtEdgeEmpty(t *testing.T) {
	p := PlaceAtEdge(map[string]Position{}, "first")
	if p.X != 0 || p.Y != 0 {
		t.Errorf("PlaceAtEdge on empty = %+v, want origin", p)
	}
}

func loadExample(t *testing.T) *dbml.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "example.dbml")
	s, err := dbml.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return s
}
