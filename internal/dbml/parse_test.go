package dbml

import (
	"path/filepath"
	"testing"
)

func TestParseExample(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "example.dbml")
	s, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if s.Project.Name != "godbml_example" {
		t.Errorf("project name = %q, want godbml_example", s.Project.Name)
	}
	if s.Project.DatabaseType != "PostgreSQL" {
		t.Errorf("database_type = %q, want PostgreSQL", s.Project.DatabaseType)
	}

	if got, want := len(s.Tables), 3; got != want {
		t.Fatalf("tables = %d, want %d", got, want)
	}

	users := findTable(t, s, "users")
	if got, want := len(users.Columns), 4; got != want {
		t.Errorf("users columns = %d, want %d", got, want)
	}
	idCol := findColumn(t, users, "id")
	if !idCol.PK {
		t.Errorf("users.id PK = false, want true")
	}
	if !idCol.Increment {
		t.Errorf("users.id increment = false, want true")
	}
	usernameCol := findColumn(t, users, "username")
	if !usernameCol.Unique {
		t.Errorf("users.username unique = false, want true")
	}
	if !usernameCol.NotNull {
		t.Errorf("users.username not_null = false, want true")
	}

	posts := findTable(t, s, "posts")
	if got, want := len(posts.Indexes), 2; got != want {
		t.Errorf("posts indexes = %d, want %d", got, want)
	}

	if got, want := len(s.Relationships), 2; got != want {
		t.Fatalf("relationships = %d, want %d (sessions.user_id inline + posts.author_id ref block)", got, want)
	}

	foundSessionsFK := false
	foundPostsFK := false
	for _, r := range s.Relationships {
		if r.Key() == "sessions.user_id->users.id" {
			foundSessionsFK = true
		}
		if r.Key() == "posts.author_id->users.id" {
			foundPostsFK = true
		}
	}
	if !foundSessionsFK {
		t.Error("missing inline ref sessions.user_id -> users.id")
	}
	if !foundPostsFK {
		t.Error("missing top-level ref posts.author_id -> users.id")
	}

	if got, want := len(s.Enums), 1; got != want {
		t.Fatalf("enums = %d, want %d", got, want)
	}
	if s.Enums[0].Name != "user_role" {
		t.Errorf("enum name = %q, want user_role", s.Enums[0].Name)
	}

	if got, want := len(s.TableGroups), 1; got != want {
		t.Fatalf("table_groups = %d, want %d", got, want)
	}
	g := s.TableGroups[0]
	if g.Name != "auth" || len(g.Members) != 2 {
		t.Errorf("table group = %+v, want auth with 2 members", g)
	}
}

func findTable(t *testing.T, s *Schema, name string) *Table {
	t.Helper()
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	t.Fatalf("table %q not found", name)
	return nil
}

func findColumn(t *testing.T, tbl *Table, name string) *Column {
	t.Helper()
	for i := range tbl.Columns {
		if tbl.Columns[i].Name == name {
			return &tbl.Columns[i]
		}
	}
	t.Fatalf("column %q not found in %q", name, tbl.Name)
	return nil
}
