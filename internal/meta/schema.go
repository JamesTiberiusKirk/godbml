package meta

const CurrentVersion = 1

const DefaultViewName = "default"

type Document struct {
	Version int     `json:"version"`
	Views   []*View `json:"views"`
}

type View struct {
	ID            string                        `json:"id"`
	Name          string                        `json:"name"`
	Tables        map[string]*TablePlacement    `json:"tables"`
	Groups        []*Group                      `json:"groups,omitempty"`
	Annotations   []*Annotation                 `json:"annotations,omitempty"`
	Relationships map[string]*RelationshipStyle `json:"relationships,omitempty"`
}

type TablePlacement struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Hidden   bool    `json:"hidden,omitempty"`
	Color    string  `json:"color,omitempty"`
	Orphaned bool    `json:"orphaned,omitempty"`
}


type Group struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Tables []string `json:"tables"`
	Color  string   `json:"color,omitempty"`
}

type Annotation struct {
	ID    string  `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	Text  string  `json:"text"`
	Color string  `json:"color,omitempty"`
}

type RelationshipStyle struct {
	Hidden bool   `json:"hidden,omitempty"`
	Color  string `json:"color,omitempty"`
}

func (d *Document) DefaultView() *View {
	if d == nil {
		return nil
	}
	for _, v := range d.Views {
		if v.Name == DefaultViewName {
			return v
		}
	}
	if len(d.Views) > 0 {
		return d.Views[0]
	}
	return nil
}
