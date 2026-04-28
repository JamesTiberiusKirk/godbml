package dbmledit

import (
	"bytes"
	"fmt"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
)

// AddTable appends a new table block at the end of the file with a single
// `id integer [pk]` column. Returns an error if a table with that name already
// exists.
func AddTable(path, name string) (*Result, error) {
	if err := validateIdent(name); err != nil {
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
	for _, t := range schema.Tables {
		if t.Name == name {
			return nil, fmt.Errorf("table %s already exists", name)
		}
	}

	prefix := ""
	if len(src) > 0 && src[len(src)-1] != '\n' {
		prefix = "\n"
	}
	insert := prefix + "\nTable " + name + " {\n  id integer [pk]\n}\n"
	out := append(src, []byte(insert)...)

	if _, err := dbml.Parse(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("rewritten dbml fails to parse: %w", err)
	}
	return commit(path, out, mtime, size)
}

// AddColumn inserts a new column at the bottom of <tableName>'s body, after
// the last existing column (or just before the closing brace if the table is
// empty). Default type is `text`.
func AddColumn(path, tableName, name, colType string) (*Result, error) {
	if err := validateIdent(name); err != nil {
		return nil, err
	}
	if err := validateTypeToken(colType); err != nil {
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

	var t *dbml.Table
	for ti := range schema.Tables {
		if schema.Tables[ti].Name == tableName {
			t = &schema.Tables[ti]
			break
		}
	}
	if t == nil {
		return nil, fmt.Errorf("table %s not found", tableName)
	}
	if !t.Span.Valid() {
		return nil, fmt.Errorf("table %s missing span", tableName)
	}
	for _, c := range t.Columns {
		if c.Name == name {
			return nil, fmt.Errorf("column %s.%s already exists", tableName, name)
		}
	}

	closeBracePos := t.Span.End - 1 // points at `}`

	// Insert just after the last existing column line, or right before `}`.
	insertPos := closeBracePos
	indent := "  "
	if len(t.Columns) > 0 {
		last := t.Columns[len(t.Columns)-1]
		ls, le := lineSpan(src, last.NameSpan.Start)
		insertPos = le
		// inherit the indentation of the last column
		for i := ls; i < last.NameSpan.Start; i++ {
			if src[i] != ' ' && src[i] != '\t' {
				indent = string(src[ls:i])
				break
			}
		}
		if indent == "" {
			// last column had no leading whitespace (single-line table); fall
			// back to two spaces and add a leading newline so the new column
			// doesn't end up jammed onto the same line.
			indent = "  "
		}
	}
	insert := indent + name + " " + colType + "\n"

	// If the inserted text would land before content on the same line as the
	// closing brace (single-line table), prepend a newline.
	needsLeadingNewline := false
	if insertPos > 0 && src[insertPos-1] != '\n' {
		needsLeadingNewline = true
	}
	if needsLeadingNewline {
		insert = "\n" + insert
	}

	out := splice(src, insertPos, insertPos, []byte(insert))

	if _, err := dbml.Parse(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("rewritten dbml fails to parse: %w", err)
	}
	return commit(path, out, mtime, size)
}

// RemoveColumn deletes the column declaration line and every top-level `Ref:`
// statement that mentions the column. Aborts with an error if any inline
// `[ref:]` setting on another column targets this column — the user must
// resolve those manually first.
func RemoveColumn(path, tableName, columnName string) (*Result, error) {
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
		return nil, fmt.Errorf("column %s.%s not found", tableName, columnName)
	}
	if !col.NameSpan.Valid() {
		return nil, fmt.Errorf("column %s.%s missing name span", tableName, columnName)
	}

	for _, r := range schema.Relationships {
		if !r.Inline {
			continue
		}
		// inline ref ON some other column whose target is the one being removed
		if r.FromTable == tableName && containsName(r.FromColumns, columnName) {
			// inline on the removed column itself — ok, goes away with it
			continue
		}
		if r.ToTable == tableName && containsName(r.ToColumns, columnName) {
			return nil, fmt.Errorf("cannot remove %s.%s: it has inline ref dependents on other columns; remove those `[ref:]` settings first",
				tableName, columnName)
		}
	}

	var edits []edit

	colStart, colEnd := lineSpan(src, col.NameSpan.Start)
	edits = append(edits, edit{Start: colStart, End: colEnd, Repl: ""})

	for _, r := range schema.Relationships {
		if r.Inline {
			continue
		}
		affected := (r.FromTable == tableName && containsName(r.FromColumns, columnName)) ||
			(r.ToTable == tableName && containsName(r.ToColumns, columnName))
		if !affected || !r.FromSpan.Valid() {
			continue
		}
		ls, le := lineSpan(src, r.FromSpan.Start)
		edits = append(edits, edit{Start: ls, End: le, Repl: ""})
	}

	out := applyEdits(src, edits)
	if _, err := dbml.Parse(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("rewritten dbml fails to parse: %w", err)
	}
	return commit(path, out, mtime, size)
}

// RemoveTable deletes the entire `Table X { ... }` block and every top-level
// `Ref:` statement that mentions the table. Aborts if any other table has an
// inline `[ref: > X.col]` targeting this table.
func RemoveTable(path, tableName string) (*Result, error) {
	src, mtime, size, err := readWithStat(path)
	if err != nil {
		return nil, err
	}
	schema, err := dbml.Parse(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("parse current source: %w", err)
	}

	var t *dbml.Table
	for ti := range schema.Tables {
		if schema.Tables[ti].Name == tableName {
			t = &schema.Tables[ti]
			break
		}
	}
	if t == nil {
		return nil, fmt.Errorf("table %s not found", tableName)
	}
	if !t.Span.Valid() {
		return nil, fmt.Errorf("table %s missing span", tableName)
	}

	for _, r := range schema.Relationships {
		if !r.Inline {
			continue
		}
		// only inline refs on OTHER tables that target this one are blockers
		if r.FromTable == tableName {
			continue
		}
		if r.ToTable == tableName {
			return nil, fmt.Errorf("cannot remove %s: it has inline ref dependents on other tables; remove those `[ref:]` settings first", tableName)
		}
	}

	var edits []edit

	blockStart := t.Span.Start
	blockEnd := t.Span.End
	if blockEnd < len(src) && src[blockEnd] == '\n' {
		blockEnd++
	}
	// also consume one leading blank line if present (so we don't leave double blanks)
	if blockStart > 0 && src[blockStart-1] == '\n' && blockStart > 1 && src[blockStart-2] == '\n' {
		blockStart--
	}
	edits = append(edits, edit{Start: blockStart, End: blockEnd, Repl: ""})

	for _, r := range schema.Relationships {
		if r.Inline {
			continue
		}
		if r.FromTable != tableName && r.ToTable != tableName {
			continue
		}
		if !r.FromSpan.Valid() {
			continue
		}
		ls, le := lineSpan(src, r.FromSpan.Start)
		edits = append(edits, edit{Start: ls, End: le, Repl: ""})
	}

	out := applyEdits(src, edits)
	if _, err := dbml.Parse(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("rewritten dbml fails to parse: %w", err)
	}
	return commit(path, out, mtime, size)
}

// lineSpan returns [start, end) byte offsets of the line containing pos:
// from the byte after the previous '\n' (or 0) up to and including the next
// '\n' (or end of file).
func lineSpan(src []byte, pos int) (int, int) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(src) {
		pos = len(src)
	}
	start := pos
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	end := pos
	for end < len(src) && src[end] != '\n' {
		end++
	}
	if end < len(src) {
		end++
	}
	return start, end
}
