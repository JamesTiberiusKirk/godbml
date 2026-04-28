package dbmledit

import (
	"strings"
	"testing"
)

func TestRewriteColumnNamePropagatesToTopLevelRef(t *testing.T) {
	src := `Table users {
  id integer [pk]
  name text
}

Table posts {
  id integer [pk]
  author_id integer
}

Ref: posts.author_id > users.id
`
	path := writeTempDBML(t, src)

	if _, err := RewriteColumnName(path, "posts", "author_id", "user_id"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got := mustReadFile(t, path)
	if !strings.Contains(got, "user_id integer") {
		t.Error("column declaration not renamed")
	}
	if strings.Contains(got, "author_id") {
		t.Errorf("old column name still present:\n%s", got)
	}
	if !strings.Contains(got, "Ref: posts.user_id > users.id") {
		t.Errorf("top-level ref not propagated:\n%s", got)
	}
}

func TestRewriteColumnNamePropagatesToInlineRef(t *testing.T) {
	src := `Table users {
  id integer [pk]
}

Table sessions {
  id integer [pk]
  user_id integer [ref: > users.id]
}
`
	path := writeTempDBML(t, src)

	if _, err := RewriteColumnName(path, "users", "id", "uid"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got := mustReadFile(t, path)
	if !strings.Contains(got, "uid integer") {
		t.Error("column declaration not renamed")
	}
	if !strings.Contains(got, "[ref: > users.uid]") {
		t.Errorf("inline ref not propagated:\n%s", got)
	}
}

func TestRewriteColumnNameInCompositeRef(t *testing.T) {
	src := `Table carts {
  id uuid [pk]
  mode mode_enum

  Indexes {
    (id, mode) [unique]
  }
}

Table cart_items {
  id uuid [pk]
  cart_id uuid
  mode mode_enum
}

Ref: cart_items.(cart_id, mode) > carts.(id, mode)
`
	path := writeTempDBML(t, src)

	if _, err := RewriteColumnName(path, "cart_items", "cart_id", "basket_id"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got := mustReadFile(t, path)
	if !strings.Contains(got, "basket_id uuid") {
		t.Error("column declaration not renamed in composite case")
	}
	if !strings.Contains(got, "cart_items.(basket_id,mode)") {
		t.Errorf("composite ref not propagated:\n%s", got)
	}
}

func TestRewriteColumnNameConflict(t *testing.T) {
	src := `Table users {
  id integer [pk]
  name text
}
`
	path := writeTempDBML(t, src)
	if _, err := RewriteColumnName(path, "users", "id", "name"); err == nil {
		t.Error("expected conflict error, got nil")
	}
}

func TestRewriteTableNameUpdatesEverywhere(t *testing.T) {
	src := `Table users {
  id integer [pk]
}

Table posts {
  id integer [pk]
  author_id integer [ref: > users.id]
}

Ref: posts.author_id > users.id
`
	path := writeTempDBML(t, src)

	if _, err := RewriteTableName(path, "users", "accounts"); err != nil {
		t.Fatalf("rename table: %v", err)
	}
	got := mustReadFile(t, path)
	if !strings.Contains(got, "Table accounts {") {
		t.Error("table declaration not renamed")
	}
	if strings.Contains(got, "users") {
		t.Errorf("old table name still present:\n%s", got)
	}
	if !strings.Contains(got, "[ref: > accounts.id]") {
		t.Errorf("inline ref not updated:\n%s", got)
	}
	if !strings.Contains(got, "Ref: posts.author_id > accounts.id") {
		t.Errorf("top-level ref not updated:\n%s", got)
	}
}
