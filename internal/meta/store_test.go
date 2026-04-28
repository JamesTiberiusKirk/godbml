package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSidecarPath(t *testing.T) {
	got := SidecarPath("/foo/bar/schema.dbml")
	want := "/foo/bar/schema.dbml.meta.json"
	if got != want {
		t.Errorf("SidecarPath = %q, want %q", got, want)
	}
}

func TestLoadMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	doc, err := Load(filepath.Join(dir, "nope.dbml.meta.json"))
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if doc != nil {
		t.Errorf("Load missing returned doc=%+v, want nil", doc)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.dbml.meta.json")

	in := NewDocument()
	v := in.Views[0]
	v.Tables["users"] = &TablePlacement{X: 100, Y: 200, Color: "#00f0ff"}
	v.Annotations = []*Annotation{
		{ID: "a1", X: 0, Y: 0, W: 200, H: 60, Text: "hello", Color: "#f0ff00"},
	}
	v.Relationships["users.id->sessions.user_id"] = &RelationshipStyle{Color: "#ff2bd6"}
	v.Groups = []*Group{
		{ID: "g1", Name: "auth", Tables: []string{"users", "sessions"}},
	}

	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Version != CurrentVersion {
		t.Errorf("version = %d, want %d", out.Version, CurrentVersion)
	}
	if len(out.Views) != 1 {
		t.Fatalf("views = %d, want 1", len(out.Views))
	}
	got := out.Views[0]
	if got.Tables["users"].X != 100 || got.Tables["users"].Y != 200 {
		t.Errorf("users placement = %+v", got.Tables["users"])
	}
	if got.Tables["users"].Color != "#00f0ff" {
		t.Errorf("users color = %q", got.Tables["users"].Color)
	}
	if len(got.Annotations) != 1 || got.Annotations[0].Text != "hello" {
		t.Errorf("annotations = %+v", got.Annotations)
	}
	if got.Relationships["users.id->sessions.user_id"].Color != "#ff2bd6" {
		t.Errorf("rel color = %q", got.Relationships["users.id->sessions.user_id"].Color)
	}
	if len(got.Groups) != 1 || got.Groups[0].Name != "auth" {
		t.Errorf("groups = %+v", got.Groups)
	}
}

func TestLoadFutureVersionFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.dbml.meta.json")
	doc := NewDocument()
	doc.Version = CurrentVersion + 5
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load future version: want error, got nil")
	}
}
