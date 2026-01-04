package doodle

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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

// RelationshipBuilder provides fluent API for defining relationships
type RelationshipBuilder struct {
	schema     *Schema
	name       string
	fromEntity string
	toEntity   string
}

// Relationship starts building a new relationship with the given name
func (s *Schema) Relationship(name string) *RelationshipBuilder {
	return &RelationshipBuilder{
		schema: s,
		name:   name,
	}
}

// From sets the source entity
func (rb *RelationshipBuilder) From(entity string) *RelationshipBuilder {
	rb.fromEntity = entity
	return rb
}

// To sets the target entity
func (rb *RelationshipBuilder) To(entity string) *RelationshipBuilder {
	rb.toEntity = entity
	return rb
}

// Via completes the relationship with junction table details
func (rb *RelationshipBuilder) Via(table, fromKey, toKey string) *Relationship {
	return rb.schema.AddRelationship(rb.name, rb.fromEntity, rb.toEntity, table, fromKey, toKey)
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

// SchemaDefinition represents a declarative schema in JSON/YAML
type SchemaDefinition struct {
	Entities      map[string]EntityDefinition      `json:"entities" yaml:"entities"`
	Relationships map[string]RelationshipDefinition `json:"relationships" yaml:"relationships"`
}

// EntityDefinition defines an entity in JSON/YAML format
type EntityDefinition struct {
	Table      string            `json:"table" yaml:"table"`
	PrimaryKey string            `json:"primary_key" yaml:"primary_key"`
	Fields     map[string]string `json:"fields,omitempty" yaml:"fields,omitempty"`
	Temporal   bool              `json:"temporal,omitempty" yaml:"temporal,omitempty"`
}

// RelationshipDefinition defines a relationship in JSON/YAML format
type RelationshipDefinition struct {
	From    string `json:"from" yaml:"from"`
	To      string `json:"to" yaml:"to"`
	Table   string `json:"table" yaml:"table"`
	FromKey string `json:"from_key" yaml:"from_key"`
	ToKey   string `json:"to_key" yaml:"to_key"`
}

// LoadSchemaFromFile loads a schema from a JSON or YAML file
func LoadSchemaFromFile(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schema file: %w", err)
	}
	return LoadSchema(data)
}

// LoadSchema parses schema from JSON or YAML bytes
func LoadSchema(data []byte) (*Schema, error) {
	var def SchemaDefinition

	// Try JSON first, then YAML
	if err := json.Unmarshal(data, &def); err != nil {
		if err := yaml.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("parsing schema: %w", err)
		}
	}

	return buildSchemaFromDefinition(&def)
}

// LoadSchemaFromJSON parses schema from JSON bytes
func LoadSchemaFromJSON(data []byte) (*Schema, error) {
	var def SchemaDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parsing JSON schema: %w", err)
	}
	return buildSchemaFromDefinition(&def)
}

// LoadSchemaFromYAML parses schema from YAML bytes
func LoadSchemaFromYAML(data []byte) (*Schema, error) {
	var def SchemaDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parsing YAML schema: %w", err)
	}
	return buildSchemaFromDefinition(&def)
}

func buildSchemaFromDefinition(def *SchemaDefinition) (*Schema, error) {
	schema := NewSchema()

	// Add entities
	for name, entDef := range def.Entities {
		ent := schema.AddEntity(name, entDef.Table, entDef.PrimaryKey)
		for fieldName, colName := range entDef.Fields {
			ent.AddField(fieldName, colName)
		}
		if entDef.Temporal {
			ent.WithTemporal()
		}
	}

	// Add relationships
	for name, relDef := range def.Relationships {
		schema.AddRelationship(name, relDef.From, relDef.To, relDef.Table, relDef.FromKey, relDef.ToKey)
	}

	if err := schema.Validate(); err != nil {
		return nil, err
	}

	return schema, nil
}
