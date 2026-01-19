package doodle

import (
	"testing"
)

func TestFluentRelationshipBuilder(t *testing.T) {
	schema := NewSchema()
	schema.AddEntity("user", "users", "id")
	schema.AddEntity("device", "devices", "id")

	// Fluent API
	rel := schema.Relationship("uses").
		From("user").
		To("device").
		Via("user_devices", "user_id", "device_id")

	if rel.Name != "uses" {
		t.Errorf("expected name 'uses', got %q", rel.Name)
	}
	if rel.FromEntity != "user" {
		t.Errorf("expected from 'user', got %q", rel.FromEntity)
	}
	if rel.ToEntity != "device" {
		t.Errorf("expected to 'device', got %q", rel.ToEntity)
	}
	if rel.JoinTable != "user_devices" {
		t.Errorf("expected table 'user_devices', got %q", rel.JoinTable)
	}
	if rel.FromKey != "user_id" {
		t.Errorf("expected from_key 'user_id', got %q", rel.FromKey)
	}
	if rel.ToKey != "device_id" {
		t.Errorf("expected to_key 'device_id', got %q", rel.ToKey)
	}

	// Verify relationship is registered in schema
	found, err := schema.FindRelationship("user", "uses", "->")
	if err != nil {
		t.Fatalf("relationship not found: %v", err)
	}
	if found.Name != "uses" {
		t.Errorf("expected found relationship 'uses', got %q", found.Name)
	}
}

func TestLoadSchemaFromJSON(t *testing.T) {
	jsonData := []byte(`{
		"entities": {
			"user": {
				"table": "users",
				"primary_key": "id",
				"temporal": true,
				"fields": {
					"email": "email",
					"status": "status"
				}
			},
			"device": {
				"table": "devices",
				"primary_key": "id",
				"fields": {
					"name": "device_name"
				}
			}
		},
		"relationships": {
			"uses": {
				"from": "user",
				"to": "device",
				"table": "user_devices",
				"from_key": "user_id",
				"to_key": "device_id"
			}
		}
	}`)

	schema, err := LoadSchemaFromJSON(jsonData)
	if err != nil {
		t.Fatalf("failed to load schema: %v", err)
	}

	// Verify entities
	user, err := schema.GetEntity("user")
	if err != nil {
		t.Fatalf("user entity not found: %v", err)
	}
	if user.Table != "users" {
		t.Errorf("expected table 'users', got %q", user.Table)
	}
	if user.Temporal == nil {
		t.Error("expected user to be temporal")
	}
	if user.Fields["email"] != "email" {
		t.Errorf("expected field mapping email->email, got %q", user.Fields["email"])
	}

	device, err := schema.GetEntity("device")
	if err != nil {
		t.Fatalf("device entity not found: %v", err)
	}
	if device.Temporal != nil {
		t.Error("expected device to not be temporal")
	}

	// Verify relationship
	rel, err := schema.FindRelationship("user", "uses", "->")
	if err != nil {
		t.Fatalf("relationship not found: %v", err)
	}
	if rel.JoinTable != "user_devices" {
		t.Errorf("expected table 'user_devices', got %q", rel.JoinTable)
	}
}

func TestLoadSchemaFromYAML(t *testing.T) {
	yamlData := []byte(`
entities:
  user:
    table: users
    primary_key: id
    temporal: true
    fields:
      email: email
      status: status
  factor:
    table: factors
    primary_key: id
    fields:
      type: factor_type

relationships:
  enrolled:
    from: user
    to: factor
    table: user_factors
    from_key: user_id
    to_key: factor_id
`)

	schema, err := LoadSchemaFromYAML(yamlData)
	if err != nil {
		t.Fatalf("failed to load schema: %v", err)
	}

	// Verify entities
	user, err := schema.GetEntity("user")
	if err != nil {
		t.Fatalf("user entity not found: %v", err)
	}
	if user.Temporal == nil {
		t.Error("expected user to be temporal")
	}

	factor, err := schema.GetEntity("factor")
	if err != nil {
		t.Fatalf("factor entity not found: %v", err)
	}
	if factor.Fields["type"] != "factor_type" {
		t.Errorf("expected field mapping type->factor_type, got %q", factor.Fields["type"])
	}

	// Verify relationship
	rel, err := schema.FindRelationship("user", "enrolled", "->")
	if err != nil {
		t.Fatalf("relationship not found: %v", err)
	}
	if rel.ToEntity != "factor" {
		t.Errorf("expected to entity 'factor', got %q", rel.ToEntity)
	}
}

func TestLoadSchemaValidation(t *testing.T) {
	// Invalid: relationship references non-existent entity
	jsonData := []byte(`{
		"entities": {
			"user": {
				"table": "users",
				"primary_key": "id"
			}
		},
		"relationships": {
			"uses": {
				"from": "user",
				"to": "device",
				"table": "user_devices",
				"from_key": "user_id",
				"to_key": "device_id"
			}
		}
	}`)

	_, err := LoadSchemaFromJSON(jsonData)
	if err == nil {
		t.Error("expected validation error for missing entity")
	}
}

func TestLoadSchemaAutoDetect(t *testing.T) {
	// JSON format
	jsonData := []byte(`{"entities": {"user": {"table": "users", "primary_key": "id"}}, "relationships": {}}`)
	schema, err := LoadSchema(jsonData)
	if err != nil {
		t.Fatalf("failed to auto-detect JSON: %v", err)
	}
	if _, err := schema.GetEntity("user"); err != nil {
		t.Error("user entity not found")
	}

	// YAML format
	yamlData := []byte(`
entities:
  group:
    table: groups
    primary_key: id
relationships: {}
`)
	schema, err = LoadSchema(yamlData)
	if err != nil {
		t.Fatalf("failed to auto-detect YAML: %v", err)
	}
	if _, err := schema.GetEntity("group"); err != nil {
		t.Error("group entity not found")
	}
}

func TestGetRelationshipByName(t *testing.T) {
	schema := NewSchema()
	schema.AddEntity("user", "users", "id")
	schema.AddEntity("group", "groups", "id")
	schema.AddEntity("app", "apps", "id")

	schema.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")
	schema.AddRelationship("assigned_to", "user", "app", "user_apps", "user_id", "app_id")
	schema.AddRelationship("has_access", "group", "app", "group_apps", "group_id", "app_id")

	tests := []struct {
		name       string
		relName    string
		wantFrom   string
		wantTo     string
		wantTable  string
		wantErr    bool
	}{
		{
			name:      "find member_of relationship",
			relName:   "member_of",
			wantFrom:  "user",
			wantTo:    "group",
			wantTable: "user_groups",
			wantErr:   false,
		},
		{
			name:      "find assigned_to relationship",
			relName:   "assigned_to",
			wantFrom:  "user",
			wantTo:    "app",
			wantTable: "user_apps",
			wantErr:   false,
		},
		{
			name:      "find has_access relationship",
			relName:   "has_access",
			wantFrom:  "group",
			wantTo:    "app",
			wantTable: "group_apps",
			wantErr:   false,
		},
		{
			name:    "non-existent relationship",
			relName: "unknown",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel, err := schema.GetRelationshipByName(tt.relName)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rel.FromEntity != tt.wantFrom {
				t.Errorf("FromEntity = %q, want %q", rel.FromEntity, tt.wantFrom)
			}
			if rel.ToEntity != tt.wantTo {
				t.Errorf("ToEntity = %q, want %q", rel.ToEntity, tt.wantTo)
			}
			if rel.JoinTable != tt.wantTable {
				t.Errorf("JoinTable = %q, want %q", rel.JoinTable, tt.wantTable)
			}
		})
	}
}

func TestGetRelationshipByNameWithTemporal(t *testing.T) {
	schema := NewSchema()
	schema.AddEntity("user", "users", "id").WithTemporal()
	schema.AddEntity("group", "groups", "id").WithTemporal()

	rel := schema.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")
	rel.WithTemporal()

	found, err := schema.GetRelationshipByName("member_of")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.Temporal == nil {
		t.Error("expected relationship to be temporal")
	}
	if found.Temporal.ValidFromColumn != "valid_from" {
		t.Errorf("ValidFromColumn = %q, want 'valid_from'", found.Temporal.ValidFromColumn)
	}
	if found.Temporal.ValidToColumn != "valid_to" {
		t.Errorf("ValidToColumn = %q, want 'valid_to'", found.Temporal.ValidToColumn)
	}
}

// DepartmentMembershipSchema creates a schema for testing department-based
// group membership queries with direct and indirect (nested) assignments.
//
// Entities:
//   - user: with metadata JSON field containing department (maps to 'meta' column)
//   - group: with type field (security, distribution, etc.)
//
// Relationships:
//   - member_of: user -> group (with role edge attribute)
//   - child_of: group -> group (for nested groups, e.g., "Engineering Team" child_of "All Engineering")
func DepartmentMembershipSchema() *Schema {
	schema := NewSchema()

	// User entity with metadata JSON field for department
	// Note: 'meta' is a keyword in Doodle, so we use 'metadata' as the field name
	schema.AddEntity("user", "users", "id").
		WithTemporal().
		AddField("email", "email").
		AddField("external_id", "external_id").
		AddField("metadata", "meta") // Doodle field 'metadata' maps to column 'meta'

	// Group entity
	schema.AddEntity("group", "groups", "id").
		WithTemporal().
		AddField("name", "name").
		AddField("external_id", "external_id").
		AddField("type", "type") // security, distribution, etc.

	// User -> Group membership (with role attribute on junction)
	rel := schema.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")
	rel.WithTemporal()

	// Group -> Group nesting (for indirect membership)
	// "child_of" means this group is a child of the target group
	childRel := schema.AddRelationship("child_of", "group", "group", "group_hierarchy", "child_group_id", "parent_group_id")
	childRel.WithTemporal()

	return schema
}

func TestDepartmentMembershipSchema(t *testing.T) {
	schema := DepartmentMembershipSchema()

	// Verify user entity has metadata field (maps to 'meta' column)
	user, err := schema.GetEntity("user")
	if err != nil {
		t.Fatalf("user entity not found: %v", err)
	}
	if user.Fields["metadata"] != "meta" {
		t.Errorf("expected metadata->meta field mapping, got %v", user.Fields)
	}
	if user.Temporal == nil {
		t.Error("expected user to be temporal")
	}

	// Verify group entity
	group, err := schema.GetEntity("group")
	if err != nil {
		t.Fatalf("group entity not found: %v", err)
	}
	if group.Fields["type"] != "type" {
		t.Errorf("expected type field, got %v", group.Fields)
	}

	// Verify member_of relationship
	memberOf, err := schema.FindRelationship("user", "member_of", "->")
	if err != nil {
		t.Fatalf("member_of relationship not found: %v", err)
	}
	if memberOf.Temporal == nil {
		t.Error("expected member_of to be temporal")
	}

	// Verify child_of relationship (group nesting)
	childOf, err := schema.FindRelationship("group", "child_of", "->")
	if err != nil {
		t.Fatalf("child_of relationship not found: %v", err)
	}
	if childOf.ToEntity != "group" {
		t.Errorf("expected child_of to point to group, got %s", childOf.ToEntity)
	}
}
