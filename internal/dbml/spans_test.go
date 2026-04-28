package dbml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpansAlignWithSource verifies that every recorded byte span actually
// indexes the substring it claims to represent in the original source.
// This is the foundation for surgical edits.
func TestSpansAlignWithSource(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "example.dbml")
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)

	s, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	check := func(name string, span Span, want string) {
		t.Helper()
		if !span.Valid() {
			t.Errorf("%s: span %+v is invalid", name, span)
			return
		}
		if span.End > len(src) {
			t.Errorf("%s: span end %d > source length %d", name, span.End, len(src))
			return
		}
		got := src[span.Start:span.End]
		// Quoted forms keep the quote chars in the span; the literal we stored
		// is the unquoted text. Allow either form to match.
		if got == want {
			return
		}
		if got == `"`+want+`"` || got == `'`+want+`'` {
			return
		}
		t.Errorf("%s: source[%d:%d] = %q, want %q", name, span.Start, span.End, got, want)
	}

	for ti := range s.Tables {
		tb := &s.Tables[ti]
		check("table "+tb.Name, tb.NameSpan, tb.Name)
		for ci := range tb.Columns {
			c := &tb.Columns[ci]
			check(tb.Name+"."+c.Name+" name", c.NameSpan, c.Name)
			check(tb.Name+"."+c.Name+" type", c.TypeSpan, c.Type)
			if c.InlineRefSpan.Valid() {
				got := src[c.InlineRefSpan.Start:c.InlineRefSpan.End]
				if !strings.Contains(got, ".") {
					t.Errorf("%s.%s inline ref span = %q, expected something with a dot",
						tb.Name, c.Name, got)
				}
			}
		}
	}

	for _, r := range s.Relationships {
		if r.Inline {
			continue
		}
		got := src[r.FromSpan.Start:r.FromSpan.End]
		want := refSideString(r.FromTable, r.FromColumns)
		if got != want {
			t.Errorf("rel %s from span = %q, want %q", r.Key(), got, want)
		}
		got = src[r.ToSpan.Start:r.ToSpan.End]
		want = refSideString(r.ToTable, r.ToColumns)
		if got != want {
			t.Errorf("rel %s to span = %q, want %q", r.Key(), got, want)
		}
	}
}
