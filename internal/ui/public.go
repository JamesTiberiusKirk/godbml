package ui

import (
	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
)

// Exported mutation API. Used by the scenario runner (cmd/godbml-test) to
// drive App state for visual snapshots without going through real input
// events. Calling these from outside the main goroutine is not supported.

// HoverTable sets the directly-hovered table; clears any group hover.
func (a *App) HoverTable(name string) {
	a.hoveredTable = name
	a.setHoveredGroup("")
}

// HoverGroup sets the hovered group. Pass either group ID or group name; the
// first matching group is used. Empty string clears.
func (a *App) HoverGroup(idOrName string) {
	if idOrName == "" {
		a.setHoveredGroup("")
		return
	}
	a.hoveredTable = ""
	for _, g := range a.currentView().Groups {
		if g.ID == idOrName || g.Name == idOrName {
			a.setHoveredGroup(g.ID)
			return
		}
	}
}

// SelectTables replaces the current table selection.
func (a *App) SelectTables(names []string) {
	if len(names) == 0 {
		a.selectedTables = nil
		return
	}
	sel := make(map[string]bool, len(names))
	for _, n := range names {
		sel[n] = true
	}
	a.selectedTables = sel
	a.selectedAnnos = nil
}

// SelectAnnotations replaces the current annotation selection.
func (a *App) SelectAnnotations(ids []string) {
	if len(ids) == 0 {
		a.selectedAnnos = nil
		return
	}
	sel := make(map[string]bool, len(ids))
	for _, id := range ids {
		sel[id] = true
	}
	a.selectedAnnos = sel
	a.selectedTables = nil
}

// ClearSelection clears both table and annotation selections.
func (a *App) ClearSelection() {
	a.selectedTables = nil
	a.selectedAnnos = nil
}

// SetCamera sets the camera position and (optionally) zoom. A zoom of <= 0
// leaves the existing zoom untouched.
func (a *App) SetCamera(x, y, zoom float64) {
	a.camera.X = x
	a.camera.Y = y
	if zoom > 0 {
		a.camera.Zoom = zoom
	}
}

// FitToTables recentres the camera to fit all visible tables.
func (a *App) FitToTables() { a.fitToTables() }

// AdvanceFrame increments the internal frame counter, advancing pulse
// animation phase. Used by the scenario runner to capture mid-cycle pulses.
func (a *App) AdvanceFrame() { a.frameCount++ }

// SetFrameCount sets the absolute frame counter. Useful for deterministic
// pulse-state snapshots.
func (a *App) SetFrameCount(n int) { a.frameCount = n }

// DBMLPath returns the path the app is editing/viewing.
func (a *App) DBMLPath() string { return a.dbmlPath }

// ApplyParsedSchema swaps the in-memory schema. Caller is responsible for
// having parsed it from the file on disk; used after the scenario runner
// performs a structural edit via dbmledit.
func (a *App) ApplyParsedSchema(s *dbml.Schema) {
	if s != nil {
		a.setSchema(s)
	}
}

// AbsorbDBMLEdit applies a successful dbmledit operation: refreshes the
// schema, updates the in-memory DBML byte mirror, persists the sidecar, and
// pushes an undo-history snapshot. Used by the scenario runner.
func (a *App) AbsorbDBMLEdit(newBytes []byte, schema *dbml.Schema) {
	if schema != nil {
		a.setSchema(schema)
	}
	if newBytes != nil {
		a.dbmlBytes = append([]byte(nil), newBytes...)
	}
	a.commit()
}

// ReloadFromDisk re-parses the on-disk DBML file and reconciles metadata.
// Equivalent to what the file watcher does on an external change.
func (a *App) ReloadFromDisk() error {
	return a.reloadDBML()
}
