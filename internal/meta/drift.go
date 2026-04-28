package meta

import (
	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
	"github.com/JamesTiberiusKirk/godbml/internal/layout"
)

type DriftReport struct {
	AddedTables    []string
	OrphanedTables []string
	RestoredTables []string
}

func (r DriftReport) Empty() bool {
	return len(r.AddedTables) == 0 &&
		len(r.OrphanedTables) == 0 &&
		len(r.RestoredTables) == 0
}

func Reconcile(doc *Document, schema *dbml.Schema) DriftReport {
	report := DriftReport{}
	if doc == nil || schema == nil {
		return report
	}

	tableSet := make(map[string]*dbml.Table, len(schema.Tables))
	for i := range schema.Tables {
		tableSet[schema.Tables[i].Name] = &schema.Tables[i]
	}

	validRelKeys := make(map[string]struct{})
	for _, r := range schema.Relationships {
		from, ok := tableSet[r.FromTable]
		if !ok {
			continue
		}
		to, ok := tableSet[r.ToTable]
		if !ok {
			continue
		}
		if !allColumnsExist(from, r.FromColumns) || !allColumnsExist(to, r.ToColumns) {
			continue
		}
		validRelKeys[r.Key()] = struct{}{}
	}

	for _, view := range doc.Views {
		if view.Tables == nil {
			view.Tables = map[string]*TablePlacement{}
		}
		if view.Relationships == nil {
			view.Relationships = map[string]*RelationshipStyle{}
		}

		// Soft-delete: tables that vanish from the schema keep their entries
		// (positions, colour, group memberships) so a rename-and-back, or any
		// momentary parse failure, restores everything. Render code already
		// filters by schema membership, so orphans are invisible until they
		// reappear in the schema.
		for name, p := range view.Tables {
			_, inSchema := tableSet[name]
			switch {
			case !inSchema && !p.Orphaned:
				p.Orphaned = true
				report.OrphanedTables = append(report.OrphanedTables, name)
			case inSchema && p.Orphaned:
				p.Orphaned = false
				report.RestoredTables = append(report.RestoredTables, name)
			}
		}

		viewPositions := positionsFromView(view)
		for name := range tableSet {
			if _, ok := view.Tables[name]; !ok {
				p := layout.PlaceAtEdge(viewPositions, name)
				view.Tables[name] = &TablePlacement{X: p.X, Y: p.Y}
				viewPositions[name] = p
				report.AddedTables = append(report.AddedTables, name)
			}
		}

		// Relationship styles are also soft-preserved: stored by FK signature,
		// so once a renamed table reverts and columns reappear, styling sticks
		// without us having to track anything per-rel here.
		_ = validRelKeys
	}

	return report
}

func BootstrapView(view *View, schema *dbml.Schema, opts layout.Options) {
	if view.Tables == nil {
		view.Tables = map[string]*TablePlacement{}
	}
	if view.Relationships == nil {
		view.Relationships = map[string]*RelationshipStyle{}
	}
	positions := layout.ForceDirected(schema, opts)
	for name, p := range positions {
		if _, exists := view.Tables[name]; exists {
			continue
		}
		view.Tables[name] = &TablePlacement{X: p.X, Y: p.Y}
	}
	for _, g := range schema.TableGroups {
		view.Groups = append(view.Groups, &Group{
			ID:     NewID(),
			Name:   g.Name,
			Tables: append([]string(nil), g.Members...),
		})
	}
}

func columnExists(t *dbml.Table, name string) bool {
	if name == "" {
		return true
	}
	for _, c := range t.Columns {
		if c.Name == name {
			return true
		}
	}
	return false
}

func allColumnsExist(t *dbml.Table, names []string) bool {
	if len(names) == 0 {
		return true
	}
	for _, n := range names {
		if !columnExists(t, n) {
			return false
		}
	}
	return true
}

func positionsFromView(v *View) map[string]layout.Position {
	out := make(map[string]layout.Position, len(v.Tables))
	for name, p := range v.Tables {
		out[name] = layout.Position{X: p.X, Y: p.Y}
	}
	return out
}
