package meta

import (
	"path/filepath"
	"testing"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
	"github.com/JamesTiberiusKirk/godbml/internal/layout"
)

func loadExample(t *testing.T) *dbml.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "example.dbml")
	s, err := dbml.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return s
}

func TestBootstrapPlacesAllTables(t *testing.T) {
	s := loadExample(t)
	doc := NewDocument()
	BootstrapView(doc.DefaultView(), s, layout.DefaultOptions())

	v := doc.DefaultView()
	if len(v.Tables) != len(s.Tables) {
		t.Errorf("placements = %d, want %d", len(v.Tables), len(s.Tables))
	}
	for _, table := range s.Tables {
		if _, ok := v.Tables[table.Name]; !ok {
			t.Errorf("missing placement for %q", table.Name)
		}
	}

	if len(v.Groups) != len(s.TableGroups) {
		t.Errorf("groups seeded = %d, want %d", len(v.Groups), len(s.TableGroups))
	}
}

func TestReconcileAddsMissingTablesAtEdge(t *testing.T) {
	s := loadExample(t)
	doc := NewDocument()
	v := doc.DefaultView()
	v.Tables["users"] = &TablePlacement{X: 0, Y: 0}

	report := Reconcile(doc, s)

	if len(report.AddedTables) == 0 {
		t.Error("expected added tables, got 0")
	}
	for _, table := range s.Tables {
		if _, ok := v.Tables[table.Name]; !ok {
			t.Errorf("table %q not added during reconcile", table.Name)
		}
	}
}

func TestReconcileOrphansMissingTables(t *testing.T) {
	s := loadExample(t)
	doc := NewDocument()
	v := doc.DefaultView()
	BootstrapView(v, s, layout.DefaultOptions())
	v.Tables["ghost_table"] = &TablePlacement{X: 123, Y: 456}

	report := Reconcile(doc, s)

	got, ok := v.Tables["ghost_table"]
	if !ok {
		t.Fatal("ghost_table should be soft-preserved (Orphaned=true), not deleted")
	}
	if !got.Orphaned {
		t.Errorf("ghost_table.Orphaned = false, want true")
	}
	if got.X != 123 || got.Y != 456 {
		t.Errorf("ghost_table position lost: %+v, want X=123 Y=456", got)
	}
	if len(report.OrphanedTables) != 1 || report.OrphanedTables[0] != "ghost_table" {
		t.Errorf("OrphanedTables = %v, want [ghost_table]", report.OrphanedTables)
	}
}

func TestReconcileRestoresPreviouslyOrphanedTables(t *testing.T) {
	s := loadExample(t)
	doc := NewDocument()
	v := doc.DefaultView()
	BootstrapView(v, s, layout.DefaultOptions())

	// Mark a real schema table as orphaned, simulating prior reconcile state
	// (e.g. user renamed it away then renamed it back).
	v.Tables["users"].Orphaned = true
	originalX, originalY := v.Tables["users"].X, v.Tables["users"].Y

	report := Reconcile(doc, s)

	if v.Tables["users"].Orphaned {
		t.Error("users.Orphaned still true after reconcile; should be restored")
	}
	if v.Tables["users"].X != originalX || v.Tables["users"].Y != originalY {
		t.Errorf("users position changed during restore: got %+v, want X=%v Y=%v", v.Tables["users"], originalX, originalY)
	}
	if len(report.RestoredTables) != 1 || report.RestoredTables[0] != "users" {
		t.Errorf("RestoredTables = %v, want [users]", report.RestoredTables)
	}
}

func TestReconcilePreservesOrphanedRelationships(t *testing.T) {
	s := loadExample(t)
	doc := NewDocument()
	v := doc.DefaultView()
	BootstrapView(v, s, layout.DefaultOptions())
	v.Relationships["users.id->ghost.user_id"] = &RelationshipStyle{Color: "#fff"}

	Reconcile(doc, s)

	// Soft-preserve: orphan rel styles stick around so a rename-and-back keeps colour.
	if _, ok := v.Relationships["users.id->ghost.user_id"]; !ok {
		t.Error("orphan relationship style should be preserved (soft-delete), not pruned")
	}
}

func TestReconcilePreservesOrphanedGroups(t *testing.T) {
	s := loadExample(t)
	doc := NewDocument()
	v := doc.DefaultView()
	BootstrapView(v, s, layout.DefaultOptions())
	v.Groups = append(v.Groups, &Group{
		ID:     "ghost_only",
		Name:   "ghost_only",
		Tables: []string{"ghost_a", "ghost_b"},
	})

	Reconcile(doc, s)

	found := false
	for _, g := range v.Groups {
		if g.Name == "ghost_only" {
			found = true
		}
	}
	if !found {
		t.Error("group containing only orphaned tables should be preserved")
	}
}

