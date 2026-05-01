package ui

import (
	"bytes"
	"encoding/json"

	"github.com/JamesTiberiusKirk/godbml/internal/meta"
)

// snapshot captures everything that would persist to disk: the DBML source
// bytes and a deep clone of the sidecar metadata document. UI state (camera,
// selection, edit-mode buffers, hover) is intentionally NOT included — undo
// restores document state, not screen state.
type snapshot struct {
	DBMLBytes []byte
	Document  *meta.Document
}

// history maintains a linear list of snapshots with a "current" pointer. Past
// the current pointer is the redo stack; before it is the undo stack.
//
//	states: [s0, s1, s2, s3, s4]
//	                    ^current   (= world state right now)
//	  undo → move pointer back, restore that state
//	  redo → move pointer forward, restore that state
//	  mutate → drop everything after current, append, advance
type history struct {
	states []snapshot
	cursor int // index of the snapshot representing the *current* world state
	cap    int
}

func newHistory(cap int) *history {
	if cap < 2 {
		cap = 2
	}
	return &history{cap: cap, cursor: -1}
}

func (h *history) clear() {
	h.states = h.states[:0]
	h.cursor = -1
}

// reset drops everything and seeds with a single snapshot representing the
// world's current state. Used at startup and after external file changes.
func (h *history) reset(s snapshot) {
	h.states = append(h.states[:0], s)
	h.cursor = 0
}

// push records a new snapshot of the current world state, dropping any
// pending redo entries past the current cursor.
func (h *history) push(s snapshot) {
	if h.cursor >= 0 && h.cursor < len(h.states)-1 {
		h.states = h.states[:h.cursor+1]
	}
	h.states = append(h.states, s)
	if len(h.states) > h.cap {
		drop := len(h.states) - h.cap
		h.states = h.states[drop:]
		h.cursor = len(h.states) - 1
		return
	}
	h.cursor = len(h.states) - 1
}

func (h *history) canUndo() bool { return h.cursor > 0 }
func (h *history) canRedo() bool { return h.cursor >= 0 && h.cursor < len(h.states)-1 }

// undo moves the cursor back one step and returns the snapshot the world
// should now be restored to. Returns nil if there's nothing to undo.
func (h *history) undo() *snapshot {
	if !h.canUndo() {
		return nil
	}
	h.cursor--
	s := h.states[h.cursor]
	return &s
}

func (h *history) redo() *snapshot {
	if !h.canRedo() {
		return nil
	}
	h.cursor++
	s := h.states[h.cursor]
	return &s
}

// cloneDocument deep-copies a meta.Document via JSON round-trip. Cheap for
// typical sidecar sizes (tens of KB) and avoids hand-maintained per-field
// clone code that would drift as the schema evolves.
func cloneDocument(d *meta.Document) *meta.Document {
	if d == nil {
		return nil
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil
	}
	var out meta.Document
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return &out
}

// dbmlBytesEqual reports whether two byte slices have the same content.
func dbmlBytesEqual(a, b []byte) bool { return bytes.Equal(a, b) }
