package dbmledit

import (
	"strings"
	"testing"
)

func TestAddTable(t *testing.T) {
	src := `Table users {
  id integer [pk]
}
`
	path := writeTempDBML(t, src)
	if _, err := AddTable(path, "posts"); err != nil {
		t.Fatal(err)
	}
	got := mustReadFile(t, path)
	if !strings.Contains(got, "Table posts {") {
		t.Errorf("new table not inserted:\n%s", got)
	}
	if !strings.Contains(got, "Table users {") {
		t.Error("existing table clobbered")
	}
}

func TestAddTableConflict(t *testing.T) {
	src := `Table users { id int [pk] }`
	path := writeTempDBML(t, src)
	if _, err := AddTable(path, "users"); err == nil {
		t.Error("expected conflict error")
	}
}

func TestAddColumn(t *testing.T) {
	src := `Table users {
  id integer [pk]
  name text
}
`
	path := writeTempDBML(t, src)
	if _, err := AddColumn(path, "users", "email", "text"); err != nil {
		t.Fatal(err)
	}
	got := mustReadFile(t, path)
	if !strings.Contains(got, "  email text\n") {
		t.Errorf("new column missing or wrong indent:\n%s", got)
	}
	// new column should be after `name text`, before `}`
	idxName := strings.Index(got, "name text")
	idxEmail := strings.Index(got, "email text")
	idxClose := strings.Index(got, "}")
	if !(idxName < idxEmail && idxEmail < idxClose) {
		t.Errorf("ordering wrong: name=%d email=%d close=%d", idxName, idxEmail, idxClose)
	}
}

func TestAddColumnPreservesIndexesBlock(t *testing.T) {
	src := `Table users {
  id integer [pk]
  name text

  Indexes {
    name [unique]
  }
}
`
	path := writeTempDBML(t, src)
	if _, err := AddColumn(path, "users", "email", "text"); err != nil {
		t.Fatal(err)
	}
	got := mustReadFile(t, path)
	// new col should land right after name, NOT after the Indexes block
	idxName := strings.Index(got, "name text")
	idxEmail := strings.Index(got, "email text")
	idxIdx := strings.Index(got, "Indexes {")
	if !(idxName < idxEmail && idxEmail < idxIdx) {
		t.Errorf("new column should land before Indexes block, got:\n%s", got)
	}
}

func TestRemoveColumnAndTopLevelRef(t *testing.T) {
	src := `Table users {
  id integer [pk]
}

Table posts {
  id integer [pk]
  author_id integer
  title text
}

Ref: posts.author_id > users.id
`
	path := writeTempDBML(t, src)
	if _, err := RemoveColumn(path, "posts", "author_id"); err != nil {
		t.Fatal(err)
	}
	got := mustReadFile(t, path)
	if strings.Contains(got, "author_id") {
		t.Errorf("column still present:\n%s", got)
	}
	if strings.Contains(got, "Ref: posts.author_id") {
		t.Errorf("dependent ref not removed:\n%s", got)
	}
	if !strings.Contains(got, "title text") {
		t.Error("unrelated column lost")
	}
}

func TestRemoveColumnAbortsOnInlineRefDependent(t *testing.T) {
	src := `Table users {
  id integer [pk]
}

Table sessions {
  id integer [pk]
  user_id integer [ref: > users.id]
}
`
	path := writeTempDBML(t, src)
	if _, err := RemoveColumn(path, "users", "id"); err == nil {
		t.Error("expected abort due to inline ref dependent")
	}
	// file unchanged
	if got := mustReadFile(t, path); got != src {
		t.Errorf("file mutated despite abort:\n%s", got)
	}
}

func TestRemoveTable(t *testing.T) {
	src := `Table users {
  id integer [pk]
}

Table posts {
  id integer [pk]
  author_id integer
}

Ref: posts.author_id > users.id
`
	path := writeTempDBML(t, src)
	if _, err := RemoveTable(path, "posts"); err != nil {
		t.Fatal(err)
	}
	got := mustReadFile(t, path)
	if strings.Contains(got, "Table posts") {
		t.Error("table still present")
	}
	if strings.Contains(got, "Ref:") {
		t.Errorf("dependent ref not removed:\n%s", got)
	}
	if !strings.Contains(got, "Table users") {
		t.Error("unrelated table lost")
	}
}

func TestRemoveTableAbortsOnInlineRefDependent(t *testing.T) {
	src := `Table users {
  id integer [pk]
}

Table sessions {
  id integer [pk]
  user_id integer [ref: > users.id]
}
`
	path := writeTempDBML(t, src)
	if _, err := RemoveTable(path, "users"); err == nil {
		t.Error("expected abort due to inline ref dependent")
	}
}
