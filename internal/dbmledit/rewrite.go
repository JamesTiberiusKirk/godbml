package dbmledit

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
)

// RewriteColumnType replaces the type of <tableName>.<columnName> with
// `newType` in the DBML source at `path`, preserving every other byte of the
// file. The new type must be a single token (e.g. `int`, `varchar`, `uuid`,
// `"text[]"`); structural changes are out of scope.
func RewriteColumnType(path, tableName, columnName, newType string) (*Result, error) {
	if err := validateTypeToken(newType); err != nil {
		return nil, err
	}

	src, mtime, size, err := readWithStat(path)
	if err != nil {
		return nil, err
	}

	schema, err := dbml.Parse(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("parse current source: %w", err)
	}

	col := findColumn(schema, tableName, columnName)
	if col == nil {
		return nil, fmt.Errorf("column %s.%s not found in current source", tableName, columnName)
	}
	if !col.TypeSpan.Valid() {
		return nil, fmt.Errorf("column %s.%s missing type span (parser bug)", tableName, columnName)
	}

	out := splice(src, col.TypeSpan.Start, col.TypeSpan.End, []byte(newType))

	// Validate by re-parsing the proposed new content. If it fails, the
	// rewrite produced invalid DBML and we abort.
	if _, err := dbml.Parse(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("rewritten dbml fails to parse: %w", err)
	}

	return commit(path, out, mtime, size)
}

func findColumn(s *dbml.Schema, tableName, columnName string) *dbml.Column {
	for ti := range s.Tables {
		t := &s.Tables[ti]
		if t.Name != tableName {
			continue
		}
		for ci := range t.Columns {
			if t.Columns[ci].Name == columnName {
				return &t.Columns[ci]
			}
		}
		return nil
	}
	return nil
}

// splice returns a new byte slice with src[start:end] replaced by repl.
// Preserves all bytes outside that range exactly.
func splice(src []byte, start, end int, repl []byte) []byte {
	out := make([]byte, 0, len(src)-(end-start)+len(repl))
	out = append(out, src[:start]...)
	out = append(out, repl...)
	out = append(out, src[end:]...)
	return out
}

// validateTypeToken rejects new-type values that aren't a single DBML type
// token. The parser will accept some surprising strings ("int int int" parses
// as two columns) so we pre-validate to fail fast with a clear message.
func validateTypeToken(s string) error {
	if s == "" {
		return fmt.Errorf("new type cannot be empty")
	}
	if strings.ContainsAny(s, " \t\n\r") {
		return fmt.Errorf("type must be a single token (no whitespace), got %q", s)
	}
	return nil
}

// validateIdent rejects names that aren't a single legal DBML identifier.
// Quoted forms not allowed for v1 — keep it simple.
func validateIdent(s string) error {
	if s == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.ContainsAny(s, " \t\n\r.,()[]{}<>:;\"'`") {
		return fmt.Errorf("name must be a single identifier, got %q", s)
	}
	return nil
}

// edit is a pending replacement of [Start, End) with Repl in the source.
type edit struct {
	Start, End int
	Repl       string
}

// applyEdits returns src with each edit applied. Edits must not overlap.
// They're applied in reverse-offset order so earlier offsets stay valid.
func applyEdits(src []byte, edits []edit) []byte {
	if len(edits) == 0 {
		return src
	}
	sorted := make([]edit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start > sorted[j].Start })
	out := make([]byte, len(src))
	copy(out, src)
	for _, e := range sorted {
		out = splice(out, e.Start, e.End, []byte(e.Repl))
	}
	return out
}

// RewriteColumnName renames <tableName>.<oldColumnName> to newColumnName,
// propagating the change to every top-level `Ref:` block and inline
// `[ref: > T.col]` setting in the file.
func RewriteColumnName(path, tableName, oldColumnName, newColumnName string) (*Result, error) {
	if err := validateIdent(newColumnName); err != nil {
		return nil, err
	}
	if oldColumnName == newColumnName {
		return nil, nil
	}

	src, mtime, size, err := readWithStat(path)
	if err != nil {
		return nil, err
	}
	schema, err := dbml.Parse(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("parse current source: %w", err)
	}

	col := findColumn(schema, tableName, oldColumnName)
	if col == nil {
		return nil, fmt.Errorf("column %s.%s not found", tableName, oldColumnName)
	}
	if !col.NameSpan.Valid() {
		return nil, fmt.Errorf("column %s.%s missing name span", tableName, oldColumnName)
	}

	// Conflict check: another column with the new name already in the table.
	for _, t := range schema.Tables {
		if t.Name != tableName {
			continue
		}
		for _, c := range t.Columns {
			if c.Name == newColumnName {
				return nil, fmt.Errorf("column %s.%s already exists", tableName, newColumnName)
			}
		}
		break
	}

	edits := []edit{
		{Start: col.NameSpan.Start, End: col.NameSpan.End, Repl: newColumnName},
	}

	// Propagate to every relationship that mentions this column on either side.
	for _, r := range schema.Relationships {
		if r.FromTable == tableName && containsName(r.FromColumns, oldColumnName) && r.FromSpan.Valid() {
			cols := replaceName(r.FromColumns, oldColumnName, newColumnName)
			edits = append(edits, edit{
				Start: r.FromSpan.Start, End: r.FromSpan.End,
				Repl: rebuildRefSide(r.FromTable, cols),
			})
		}
		if r.ToTable == tableName && containsName(r.ToColumns, oldColumnName) && r.ToSpan.Valid() {
			cols := replaceName(r.ToColumns, oldColumnName, newColumnName)
			edits = append(edits, edit{
				Start: r.ToSpan.Start, End: r.ToSpan.End,
				Repl: rebuildRefSide(r.ToTable, cols),
			})
		}
	}

	out := applyEdits(src, edits)

	if _, err := dbml.Parse(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("rewritten dbml fails to parse: %w", err)
	}
	return commit(path, out, mtime, size)
}

// RewriteTableName renames a table and updates every reference in:
//   - the `Table X {` declaration
//   - every top-level `Ref: X.col > ...` and `... > X.col`
//   - every inline `[ref: > X.col]`
//   - every `TableGroup` member list that lists X
func RewriteTableName(path, oldTableName, newTableName string) (*Result, error) {
	if err := validateIdent(newTableName); err != nil {
		return nil, err
	}
	if oldTableName == newTableName {
		return nil, nil
	}

	src, mtime, size, err := readWithStat(path)
	if err != nil {
		return nil, err
	}
	schema, err := dbml.Parse(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("parse current source: %w", err)
	}

	// conflict check
	for _, t := range schema.Tables {
		if t.Name == newTableName {
			return nil, fmt.Errorf("table %s already exists", newTableName)
		}
	}

	var target *dbml.Table
	for ti := range schema.Tables {
		if schema.Tables[ti].Name == oldTableName {
			target = &schema.Tables[ti]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("table %s not found", oldTableName)
	}
	if !target.NameSpan.Valid() {
		return nil, fmt.Errorf("table %s missing name span", oldTableName)
	}

	edits := []edit{
		{Start: target.NameSpan.Start, End: target.NameSpan.End, Repl: newTableName},
	}

	for _, r := range schema.Relationships {
		if r.FromTable == oldTableName && r.FromSpan.Valid() {
			edits = append(edits, edit{
				Start: r.FromSpan.Start, End: r.FromSpan.End,
				Repl: rebuildRefSide(newTableName, r.FromColumns),
			})
		}
		if r.ToTable == oldTableName && r.ToSpan.Valid() {
			edits = append(edits, edit{
				Start: r.ToSpan.Start, End: r.ToSpan.End,
				Repl: rebuildRefSide(newTableName, r.ToColumns),
			})
		}
	}

	out := applyEdits(src, edits)
	if _, err := dbml.Parse(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("rewritten dbml fails to parse: %w", err)
	}
	return commit(path, out, mtime, size)
}

func containsName(cols []string, name string) bool {
	for _, c := range cols {
		if c == name {
			return true
		}
	}
	return false
}

func replaceName(cols []string, oldName, newName string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		if c == oldName {
			out[i] = newName
		} else {
			out[i] = c
		}
	}
	return out
}

func rebuildRefSide(table string, cols []string) string {
	switch len(cols) {
	case 0:
		return table + "."
	case 1:
		return table + "." + cols[0]
	default:
		return table + ".(" + strings.Join(cols, ",") + ")"
	}
}
