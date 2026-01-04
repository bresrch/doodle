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
