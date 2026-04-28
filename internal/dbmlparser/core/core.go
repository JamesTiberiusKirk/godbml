package core

import "github.com/JamesTiberiusKirk/godbml/internal/dbmlparser/token"

// DBML structure
type DBML struct {
	Project     Project
	Tables      []Table
	Enums       []Enum
	Refs        []Ref
	TableGroups []TableGroup
}

// Project ...
type Project struct {
	Name         string
	Note         string
	DatabaseType string
}

// Table ...
type Table struct {
	Name      string
	NameStart int
	NameEnd   int
	// SpanStart/SpanEnd cover the entire `Table X { ... }` declaration,
	// from the start of the `Table` keyword to the end of the closing `}`.
	SpanStart int
	SpanEnd   int
	As        string
	Note      string
	Columns   []Column
	Indexes   []Index
}

// Column ...
type Column struct {
	Name      string
	NameStart int
	NameEnd   int
	Type      string
	TypeStart int
	TypeEnd   int
	Settings  ColumnSetting
}

// ColumnSetting ...
type ColumnSetting struct {
	Note      string
	PK        bool
	Unique    bool
	Default   string
	Null      bool
	Increment bool
	Ref       struct {
		Type    RelationshipType
		To      string
		ToStart int
		ToEnd   int
	}
}

// Index ...
type Index struct {
	Fields   []string
	Settings IndexSetting
}

// IndexSetting ...
type IndexSetting struct {
	Type   string
	Name   string
	Unique bool
	PK     bool
	Note   string
}

// RelationshipType ...
type RelationshipType int

const (
	//None relationship
	None = iota
	//OneToOne 1 - 1
	OneToOne
	//OneToMany 1 - n
	OneToMany
	// ManyToOne n - 1
	ManyToOne
)

// Relationship ...
type Relationship struct {
	From      string
	FromStart int
	FromEnd   int
	To        string
	ToStart   int
	ToEnd     int
	Type      RelationshipType
}

//RelationshipMap ...
var RelationshipMap = map[token.Token]RelationshipType{
	token.GTR: ManyToOne,
	token.LSS: OneToMany,
	token.SUB: OneToOne,
}

// Ref ...
type Ref struct {
	// TODO:
	//		- handle Ref
	Name          string // optional
	Relationships []Relationship
}

// Enum ...
type Enum struct {
	Name   string
	Values []EnumValue
}

// EnumValue ...
type EnumValue struct {
	Name string
	Note string
}

// TableGroup ...
type TableGroup struct {
	// TODO:
	// --  handle for table group
	Name    string
	Members []string
}
