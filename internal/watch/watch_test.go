package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherEmitsDBMLEvent(t *testing.T) {
	dir := t.TempDir()
	dbmlPath := filepath.Join(dir, "schema.dbml")
	metaPath := filepath.Join(dir, "schema.dbml.meta.json")

	if err := os.WriteFile(dbmlPath, []byte("// initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := New(dbmlPath, metaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(20 * time.Millisecond)

	if err := os.WriteFile(dbmlPath, []byte("// updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != EventDBML {
			t.Errorf("event kind = %d, want EventDBML", ev.Kind)
		}
		if ev.Path != dbmlPath {
			t.Errorf("event path = %q, want %q", ev.Path, dbmlPath)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event within 2s")
	}
}

func TestWatcherDetectsAtomicRenameSave(t *testing.T) {
	dir := t.TempDir()
	dbmlPath := filepath.Join(dir, "schema.dbml")
	metaPath := filepath.Join(dir, "schema.dbml.meta.json")

	if err := os.WriteFile(dbmlPath, []byte("// v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := New(dbmlPath, metaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(20 * time.Millisecond)

	// Simulate an editor's atomic save: write to a temp file in the same dir,
	// then rename over the target. fsnotify fires events on the tmp path; we
	// must still detect the change at the target path.
	tmp := filepath.Join(dir, ".schema.dbml.tmp")
	if err := os.WriteFile(tmp, []byte("// v2 — much longer body now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, dbmlPath); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != EventDBML {
			t.Errorf("event kind = %d, want EventDBML", ev.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event after atomic-save rename within 2s")
	}
}

func TestWatcherIgnoresUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	dbmlPath := filepath.Join(dir, "schema.dbml")
	metaPath := filepath.Join(dir, "schema.dbml.meta.json")
	if err := os.WriteFile(dbmlPath, []byte("// x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := New(dbmlPath, metaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(20 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		t.Errorf("unexpected event: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}
