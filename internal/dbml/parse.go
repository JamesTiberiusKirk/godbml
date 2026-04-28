package dbml

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JamesTiberiusKirk/godbml/internal/dbmlparser/core"
	"github.com/JamesTiberiusKirk/godbml/internal/dbmlparser/parser"
	"github.com/JamesTiberiusKirk/godbml/internal/dbmlparser/scanner"
)

type Schema struct {
	Project       Project
	Tables        []Table
	Enums         []Enum
	Relationships []Relationship
	TableGroups   []TableGroup
}

type Project struct {
	Name         string
	Note         string
	DatabaseType string
}

type Span struct {
	Start, End int
}

func (s Span) Valid() bool { return s.End > s.Start }

type Table struct {
	Name     string
	NameSpan Span
	// Span covers the full `Table X { ... }` block, used when removing a
	// table or inserting columns just before its closing brace.
	Span    Span
	Alias   string
	Note    string
	Columns []Column
	Indexes []Index
}

type Column struct {
	Name      string
	NameSpan  Span
	Type      string
	TypeSpan  Span
	PK        bool
	Unique    bool
	NotNull   bool
	Increment bool
	Default   string
	Note      string
	// InlineRefSpan is the byte range of the `T.col` in `[ref: > T.col]` when
	// this column declares its FK inline. Zero if no inline ref.
	InlineRefSpan Span
}

type Index struct {
	Fields []string
	Name   string
	Type   string
	Unique bool
	PK     bool
	Note   string
}

type Enum struct {
	Name   string
	Values []EnumValue
}

type EnumValue struct {
	Name string
	Note string
}

type RelationshipKind int

const (
	RelOneToOne RelationshipKind = iota
	RelOneToMany
	RelManyToOne
)

type Relationship struct {
	FromTable   string
	FromColumns []string
	FromSpan    Span // byte range of the `T.col` (or `T.(c1,c2)`) on the from side
	ToTable     string
	ToColumns   []string
	ToSpan      Span // byte range of the `T.col` (or `T.(c1,c2)`) on the to side
	Kind        RelationshipKind
	Name        string
	// Inline marks the relationship as declared inline on a column
	// (`[ref: > T.col]`) rather than as a top-level `Ref:` block.
	Inline bool
}

func (r Relationship) Key() string {
	return refSideString(r.FromTable, r.FromColumns) + "->" + refSideString(r.ToTable, r.ToColumns)
}

func refSideString(table string, cols []string) string {
	if len(cols) <= 1 {
		col := ""
		if len(cols) == 1 {
			col = cols[0]
		}
		return table + "." + col
	}
	return table + ".(" + strings.Join(cols, ",") + ")"
}

type TableGroup struct {
	Name    string
	Members []string
}

func ParseFile(path string) (*Schema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

func Parse(r io.Reader) (*Schema, error) {
	p := parser.NewParser(scanner.NewScanner(r))
	raw, err := p.Parse()
	if err != nil {
		return nil, err
	}
	return convert(raw), nil
}

func convert(raw *core.DBML) *Schema {
	s := &Schema{
		Project: Project{
			Name:         raw.Project.Name,
			Note:         raw.Project.Note,
			DatabaseType: raw.Project.DatabaseType,
		},
	}

	for _, t := range raw.Tables {
		out := Table{
			Name:     t.Name,
			NameSpan: Span{Start: t.NameStart, End: t.NameEnd},
			Span:     Span{Start: t.SpanStart, End: t.SpanEnd},
			Alias:    t.As,
			Note:     t.Note,
		}
		for _, c := range t.Columns {
			col := Column{
				Name:      c.Name,
				NameSpan:  Span{Start: c.NameStart, End: c.NameEnd},
				Type:      c.Type,
				TypeSpan:  Span{Start: c.TypeStart, End: c.TypeEnd},
				PK:        c.Settings.PK,
				Unique:    c.Settings.Unique,
				NotNull:   !c.Settings.Null,
				Increment: c.Settings.Increment,
				Default:   c.Settings.Default,
				Note:      c.Settings.Note,
			}
			if c.Settings.Ref.To != "" {
				col.InlineRefSpan = Span{Start: c.Settings.Ref.ToStart, End: c.Settings.Ref.ToEnd}
				toTable, toCols := splitRefSide(c.Settings.Ref.To)
				s.Relationships = append(s.Relationships, Relationship{
					FromTable:   t.Name,
					FromColumns: []string{c.Name},
					ToTable:     toTable,
					ToColumns:   toCols,
					ToSpan:      col.InlineRefSpan,
					Kind:        relKind(c.Settings.Ref.Type),
					Inline:      true,
				})
			}
			out.Columns = append(out.Columns, col)
		}
		for _, idx := range t.Indexes {
			out.Indexes = append(out.Indexes, Index{
				Fields: append([]string(nil), idx.Fields...),
				Name:   idx.Settings.Name,
				Type:   idx.Settings.Type,
				Unique: idx.Settings.Unique,
				PK:     idx.Settings.PK,
				Note:   idx.Settings.Note,
			})
		}
		s.Tables = append(s.Tables, out)
	}

	for _, ref := range raw.Refs {
		for _, rel := range ref.Relationships {
			fromTable, fromCols := splitRefSide(rel.From)
			toTable, toCols := splitRefSide(rel.To)
			s.Relationships = append(s.Relationships, Relationship{
				FromTable:   fromTable,
				FromColumns: fromCols,
				FromSpan:    Span{Start: rel.FromStart, End: rel.FromEnd},
				ToTable:     toTable,
				ToColumns:   toCols,
				ToSpan:      Span{Start: rel.ToStart, End: rel.ToEnd},
				Kind:        relKind(rel.Type),
				Name:        ref.Name,
			})
		}
	}

	for _, e := range raw.Enums {
		eo := Enum{Name: e.Name}
		for _, v := range e.Values {
			eo.Values = append(eo.Values, EnumValue{Name: v.Name, Note: v.Note})
		}
		s.Enums = append(s.Enums, eo)
	}

	for _, g := range raw.TableGroups {
		s.TableGroups = append(s.TableGroups, TableGroup{
			Name:    g.Name,
			Members: append([]string(nil), g.Members...),
		})
	}

	return s
}

func splitRefSide(s string) (table string, columns []string) {
	dot := strings.Index(s, ".")
	if dot < 0 {
		return s, nil
	}
	table = s[:dot]
	rest := s[dot+1:]
	if strings.HasPrefix(rest, "(") && strings.HasSuffix(rest, ")") {
		inner := rest[1 : len(rest)-1]
		for _, c := range strings.Split(inner, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				columns = append(columns, c)
			}
		}
		return table, columns
	}
	if rest == "" {
		return table, nil
	}
	return table, []string{rest}
}

func relKind(t core.RelationshipType) RelationshipKind {
	switch t {
	case core.OneToOne:
		return RelOneToOne
	case core.OneToMany:
		return RelOneToMany
	case core.ManyToOne:
		return RelManyToOne
	}
	return RelManyToOne
}

func (s *Schema) String() string {
	return fmt.Sprintf("Schema{tables=%d relationships=%d enums=%d groups=%d}",
		len(s.Tables), len(s.Relationships), len(s.Enums), len(s.TableGroups))
}
