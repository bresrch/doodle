package doodle

import "fmt"

// Schema holds entity and relationship definitions
type Schema struct {
	Entities      map[string]*Entity
	Relationships map[string]*Relationship
}

// Entity represents a database entity/table
type Entity struct {
	Name       string
	Table      string
	PrimaryKey string
	Fields     map[string]string // field name -> column name

	// Temporal configuration
	Temporal *TemporalConfig
}

// TemporalConfig defines temporal versioning columns
type TemporalConfig struct {
	VersionColumn   string // e.g., "version"
	ValidFromColumn string // e.g., "valid_from"
	ValidToColumn   string // e.g., "valid_to"
	RawDataColumn   string // e.g., "raw_data" (JSONB)
}

// DefaultTemporalConfig returns standard temporal column names
func DefaultTemporalConfig() *TemporalConfig {
	return &TemporalConfig{
		VersionColumn:   "version",
		ValidFromColumn: "valid_from",
		ValidToColumn:   "valid_to",
		RawDataColumn:   "raw_data",
	}
}

// Relationship represents a graph edge between entities
type Relationship struct {
	Name       string
	FromEntity string
	ToEntity   string
	JoinTable  string
	FromKey    string
	ToKey      string
}

// NewSchema creates an empty schema
func NewSchema() *Schema {
	return &Schema{
		Entities:      make(map[string]*Entity),
		Relationships: make(map[string]*Relationship),
	}
}

// AddEntity registers an entity in the schema
func (s *Schema) AddEntity(name, table, primaryKey string) *Entity {
	e := &Entity{
		Name:       name,
		Table:      table,
		PrimaryKey: primaryKey,
		Fields:     make(map[string]string),
	}
	s.Entities[name] = e
	return e
}

// AddField adds a field mapping to an entity
func (e *Entity) AddField(name, column string) *Entity {
	e.Fields[name] = column
	return e
}

// WithTemporal enables temporal versioning with default column names
func (e *Entity) WithTemporal() *Entity {
	e.Temporal = DefaultTemporalConfig()
	return e
}

// WithTemporalConfig enables temporal versioning with custom column names
func (e *Entity) WithTemporalConfig(config *TemporalConfig) *Entity {
	e.Temporal = config
	return e
}

// AddRelationship registers a relationship between entities
func (s *Schema) AddRelationship(name, from, to, joinTable, fromKey, toKey string) *Relationship {
	r := &Relationship{
		Name:       name,
		FromEntity: from,
		ToEntity:   to,
		JoinTable:  joinTable,
		FromKey:    fromKey,
		ToKey:      toKey,
	}
	key := fmt.Sprintf("%s->%s->%s", from, name, to)
	s.Relationships[key] = r
	return r
}

// FindRelationship looks up a relationship by traversal
func (s *Schema) FindRelationship(fromEntity, relationName, direction string) (*Relationship, error) {
	if direction == "->" {
		// Outgoing: look for fromEntity->relationName->*
		for key, rel := range s.Relationships {
			if rel.FromEntity == fromEntity && rel.Name == relationName {
				return rel, nil
			}
			_ = key
		}
	} else {
		// Incoming: look for *->relationName->fromEntity
		for key, rel := range s.Relationships {
			if rel.ToEntity == fromEntity && rel.Name == relationName {
				return rel, nil
			}
			_ = key
		}
	}
	return nil, fmt.Errorf("relationship %s%s not found from entity %s", direction, relationName, fromEntity)
}

// GetEntity retrieves an entity by name
func (s *Schema) GetEntity(name string) (*Entity, error) {
	e, ok := s.Entities[name]
	if !ok {
		return nil, fmt.Errorf("entity %s not found", name)
	}
	return e, nil
}

// Validate checks schema consistency
func (s *Schema) Validate() error {
	for key, rel := range s.Relationships {
		if _, ok := s.Entities[rel.FromEntity]; !ok {
			return fmt.Errorf("relationship %s references unknown entity %s", key, rel.FromEntity)
		}
		if _, ok := s.Entities[rel.ToEntity]; !ok {
			return fmt.Errorf("relationship %s references unknown entity %s", key, rel.ToEntity)
		}
	}
	return nil
}
