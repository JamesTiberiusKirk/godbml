package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

func SidecarPath(dbmlPath string) string {
	return dbmlPath + ".meta.json"
}

func Load(path string) (*Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var d Document
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if d.Version == 0 {
		d.Version = CurrentVersion
	}
	if d.Version > CurrentVersion {
		return nil, fmt.Errorf("sidecar version %d newer than supported %d", d.Version, CurrentVersion)
	}
	for _, v := range d.Views {
		if v.Tables == nil {
			v.Tables = map[string]*TablePlacement{}
		}
		if v.Relationships == nil {
			v.Relationships = map[string]*RelationshipStyle{}
		}
	}
	return &d, nil
}

func Save(path string, d *Document) error {
	d.Version = CurrentVersion
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func NewDocument() *Document {
	return &Document{
		Version: CurrentVersion,
		Views: []*View{
			{
				ID:            NewID(),
				Name:          DefaultViewName,
				Tables:        map[string]*TablePlacement{},
				Relationships: map[string]*RelationshipStyle{},
			},
		},
	}
}
