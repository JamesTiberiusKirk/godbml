package dbmledit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
)

func writeTempDBML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.dbml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRewriteColumnTypeSimple(t *testing.T) {
	src := `// my schema
Table users {
  id integer [pk]
  name varchar
  created_at timestamp
}
`
	path := writeTempDBML(t, src)

	res, err := RewriteColumnType(path, "users", "name", "text")
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	got := mustReadFile(t, path)
	want := strings.Replace(src, "name varchar", "name text", 1)
	if got != want {
		t.Errorf("file content:\nwant:\n%s\ngot:\n%s", want, got)
	}

	if res.Schema == nil {
		t.Fatal("result.Schema is nil")
	}
	col := findCol(res.Schema, "users", "name")
	if col == nil || col.Type != "text" {
		t.Errorf("re-parsed column type = %v, want text", col)
	}
}

func TestRewriteColumnTypePreservesComments(t *testing.T) {
	src := `// preserve me
Table accounts {
  // and this comment
  id uuid [pk]
  name text [not null] // trailing
  metadata jsonb [not null, default: ` + "`" + `'{}'::jsonb` + "`" + `]
}
`
	path := writeTempDBML(t, src)

	if _, err := RewriteColumnType(path, "accounts", "metadata", "json"); err != nil {
		t.Fatal(err)
	}

	got := mustReadFile(t, path)
	if !strings.Contains(got, "// preserve me") {
		t.Error("top comment not preserved")
	}
	if !strings.Contains(got, "// and this comment") {
		t.Error("inline comment not preserved")
	}
	if !strings.Contains(got, "// trailing") {
		t.Error("trailing comment not preserved")
	}
	if !strings.Contains(got, "metadata json [not null, default: ") {
		t.Errorf("type rewrite landed wrong:\n%s", got)
	}
	if strings.Contains(got, "metadata jsonb") {
		t.Error("old type still present")
	}
}

func TestRewriteColumnTypeParameterizedType(t *testing.T) {
	src := `Table sessions {
  id uuid [pk]
  token varchar(255) [unique]
}
`
	path := writeTempDBML(t, src)

	if _, err := RewriteColumnType(path, "sessions", "token", "text"); err != nil {
		t.Fatal(err)
	}
	got := mustReadFile(t, path)
	if !strings.Contains(got, "token text [unique]") {
		t.Errorf("varchar(255) → text rewrite:\n%s", got)
	}
}

func TestRewriteColumnTypeInvalidNewTypeFails(t *testing.T) {
	src := `Table t { id int [pk] }`
	path := writeTempDBML(t, src)

	// not-a-token should fail re-parse
	if _, err := RewriteColumnType(path, "t", "id", "int int int"); err == nil {
		t.Error("expected error for invalid new type, got nil")
	}
	// the file should be unchanged
	if got := mustReadFile(t, path); got != src {
		t.Errorf("file mutated despite failed rewrite:\n%s", got)
	}
}

func TestRewriteColumnTypeUnknownColumn(t *testing.T) {
	src := `Table t { id int [pk] }`
	path := writeTempDBML(t, src)
	if _, err := RewriteColumnType(path, "t", "ghost", "int"); err == nil {
		t.Error("expected error for unknown column, got nil")
	}
}

func TestRewriteColumnTypeAbortsOnExternalEdit(t *testing.T) {
	src := `Table t { id int [pk] }`
	path := writeTempDBML(t, src)

	srcBytes, mtime, size, err := readWithStat(path)
	_, _, _ = srcBytes, mtime, size
	if err != nil {
		t.Fatal(err)
	}

	// Simulate someone else editing the file between our read and our commit.
	// We can't easily inject this into RewriteColumnType from outside, so we
	// drive it through commit() directly.
	out := bytes.ReplaceAll(srcBytes, []byte("int"), []byte("uuid"))

	// Rewrite externally (changes mtime).
	if err := os.WriteFile(path, []byte(`Table t { id text [pk] }`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := commit(path, out, mtime, size); err == nil {
		t.Error("expected commit to abort due to external edit, got nil")
	}
}

func findCol(s *dbml.Schema, tn, cn string) *dbml.Column {
	for i := range s.Tables {
		if s.Tables[i].Name != tn {
			continue
		}
		for j := range s.Tables[i].Columns {
			if s.Tables[i].Columns[j].Name == cn {
				return &s.Tables[i].Columns[j]
			}
		}
	}
	return nil
}
