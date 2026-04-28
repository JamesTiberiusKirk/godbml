// Package dbmledit applies surgical text edits to DBML source files,
// preserving comments, whitespace, and original formatting.
package dbmledit

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
)

// Result describes the outcome of a successful rewrite.
type Result struct {
	NewBytes []byte
	NewMtime time.Time
	Schema   *dbml.Schema
}

// readWithStat reads `path`, returning the contents alongside the mtime/size
// captured immediately after the read, used as a precondition for safe write.
func readWithStat(path string) ([]byte, time.Time, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, 0, err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, 0, err
	}
	return src, info.ModTime(), info.Size(), nil
}

// commit writes `out` to `path` atomically (temp file + rename) only if the
// file's mtime/size still match what we observed at read time. This guards
// against an external editor having clobbered the file mid-edit.
func commit(path string, out []byte, expectedMtime time.Time, expectedSize int64) (*Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.ModTime().Equal(expectedMtime) || info.Size() != expectedSize {
		return nil, fmt.Errorf("file changed externally during edit; aborted to avoid clobbering")
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}

	newInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	schema, err := dbml.Parse(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("internal: rewritten file parses but not via dbml.Parse: %w", err)
	}
	return &Result{NewBytes: out, NewMtime: newInfo.ModTime(), Schema: schema}, nil
}
