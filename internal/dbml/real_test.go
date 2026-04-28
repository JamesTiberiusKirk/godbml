package dbml

import (
	"path/filepath"
	"testing"
)

func TestParseRealSchema(t *testing.T) {
	path := filepath.Join("..", "..", "example.dbml")
	s, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile real schema: %v", err)
	}
	t.Logf("parsed: tables=%d relationships=%d enums=%d groups=%d",
		len(s.Tables), len(s.Relationships), len(s.Enums), len(s.TableGroups))
}
