//go:build integration

package doodle

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		input   string
		want    Ref
		wantErr bool
	}{
		{"user:alice", Ref{Type: "user", ID: "alice"}, false},
		{"group:admins", Ref{Type: "group", ID: "admins"}, false},
		{"app:okta_app_slack", Ref{Type: "app", ID: "okta_app_slack"}, false},
		{"invalid", Ref{}, true},
		{"", Ref{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseRef(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRef(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseRef(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRefString(t *testing.T) {
	ref := Ref{Type: "user", ID: "alice"}
	if got := ref.String(); got != "user:alice" {
		t.Errorf("Ref.String() = %q, want %q", got, "user:alice")
	}
}

func TestTupleBuilderCreate(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Cleanup function
	cleanup := func() {
		conn.ExecContext(ctx, `DELETE FROM user_groups WHERE user_id IN (SELECT id FROM users WHERE external_id = 'tuple_test_user')`)
		conn.ExecContext(ctx, `DELETE FROM users WHERE external_id = 'tuple_test_user'`)
		conn.ExecContext(ctx, `DELETE FROM groups WHERE external_id = 'tuple_test_group'`)
	}
	cleanup()
	t.Cleanup(cleanup)

	// First, create a test user and group that we can relate
	_, err := conn.ExecContext(ctx, `
		INSERT INTO users (external_id, email, first_name, last_name, status, provider)
		VALUES ('tuple_test_user', 'tuple@test.com', 'Tuple', 'Test', 'ACTIVE', 'okta')
		ON CONFLICT (external_id, version) DO NOTHING
	`)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	_, err = conn.ExecContext(ctx, `
		INSERT INTO groups (external_id, name, description, provider)
		VALUES ('tuple_test_group', 'Tuple Test Group', 'For testing tuples', 'okta')
		ON CONFLICT (external_id, version) DO NOTHING
	`)
	if err != nil {
		t.Fatalf("Failed to create test group: %v", err)
	}

	// Create relationship using Tuple API
	result, err := db.Tuple("user:tuple_test_user", "member_of", "group:tuple_test_group").
		With("role", "tester").
		Create(ctx)

	if err != nil {
		t.Fatalf("Tuple.Create() error = %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows != 1 {
		t.Errorf("Expected 1 row affected, got %d", rows)
	}

	// Verify the relationship exists
	exists, err := db.Tuple("user:tuple_test_user", "member_of", "group:tuple_test_group").Exists(ctx)
	if err != nil {
		t.Fatalf("Tuple.Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected relationship to exist after Create()")
	}

	// Verify the role metadata
	var role string
	err = conn.QueryRowContext(ctx, `
		SELECT role FROM user_groups
		WHERE user_id = (SELECT id FROM users WHERE external_id = 'tuple_test_user' AND valid_to = 'infinity')
		AND group_id = (SELECT id FROM groups WHERE external_id = 'tuple_test_group' AND valid_to = 'infinity')
	`).Scan(&role)
	if err != nil {
		t.Fatalf("Failed to query role: %v", err)
	}
	if role != "tester" {
		t.Errorf("Expected role 'tester', got %q", role)
	}
}

func TestTupleBuilderDelete(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Use existing test data - Alice is in Administrators
	exists, err := db.Tuple("user:okta_user_001", "member_of", "group:okta_group_admins").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Skip("Test data not available - okta_user_001 not in okta_group_admins")
	}

	// Delete the relationship
	_, err = db.Tuple("user:okta_user_001", "member_of", "group:okta_group_admins").Delete(ctx)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	exists, err = db.Tuple("user:okta_user_001", "member_of", "group:okta_group_admins").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() after delete error = %v", err)
	}
	if exists {
		t.Error("Expected relationship to be deleted")
	}

	// Re-create for other tests
	_, err = db.Tuple("user:okta_user_001", "member_of", "group:okta_group_admins").
		With("role", "admin").
		Create(ctx)
	if err != nil {
		t.Logf("Warning: Failed to re-create relationship: %v", err)
	}
}

func TestTupleBuilderExists(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Test existing relationship (Alice in Developers - from init.sql)
	exists, err := db.Tuple("user:okta_user_001", "member_of", "group:okta_group_devs").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected Alice to be in Developers group")
	}

	// Test non-existing relationship
	exists, err = db.Tuple("user:okta_user_001", "member_of", "group:okta_group_users").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Expected Alice NOT to be in Users group")
	}
}

func TestTupleBuilderInvalidRef(t *testing.T) {
	db := New(integrationSchema())

	// Invalid subject
	tb := db.Tuple("invalid", "member_of", "group:admins")
	_, err := tb.Create(context.Background())
	if err == nil {
		t.Error("Expected error for invalid subject reference")
	}

	// Invalid target
	tb = db.Tuple("user:alice", "member_of", "invalid")
	_, err = tb.Create(context.Background())
	if err == nil {
		t.Error("Expected error for invalid target reference")
	}
}

func TestEntityBuilderCreate(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Cleanup
	cleanup := func() {
		conn.ExecContext(ctx, "DELETE FROM users WHERE external_id = 'entity_test_user'")
	}
	cleanup()
	t.Cleanup(cleanup)

	// Create entity using Entity API
	result := db.Entity("user:entity_test_user").
		Set("email", "entity@test.com").
		Set("first_name", "Entity").
		Set("last_name", "Test").
		Set("status", "ACTIVE").
		Set("provider", "okta").
		Create(ctx)

	if result.Err() != nil {
		t.Fatalf("Entity.Create() error = %v", result.Err())
	}

	// Verify entity exists
	exists, err := db.Entity("user:entity_test_user").Exists(ctx)
	if err != nil {
		t.Fatalf("Entity.Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected entity to exist after Create()")
	}

	// Verify field values
	var email, firstName string
	err = conn.QueryRowContext(ctx, `
		SELECT email, first_name FROM users
		WHERE external_id = 'entity_test_user' AND valid_to = 'infinity'
	`).Scan(&email, &firstName)
	if err != nil {
		t.Fatalf("Failed to query entity: %v", err)
	}
	if email != "entity@test.com" {
		t.Errorf("Expected email 'entity@test.com', got %q", email)
	}
	if firstName != "Entity" {
		t.Errorf("Expected first_name 'Entity', got %q", firstName)
	}
}

func TestEntityBuilderUpdate(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Update Alice's email
	_, err := db.Entity("user:okta_user_001").
		Set("email", "alice.updated@example.com").
		Update(ctx)

	if err != nil {
		t.Fatalf("Entity.Update() error = %v", err)
	}

	// Verify the update
	var email string
	err = conn.QueryRowContext(ctx, `
		SELECT email FROM users
		WHERE external_id = 'okta_user_001' AND valid_to = 'infinity'
	`).Scan(&email)
	if err != nil {
		t.Fatalf("Failed to query entity: %v", err)
	}
	if email != "alice.updated@example.com" {
		t.Errorf("Expected updated email, got %q", email)
	}

	// Restore original email
	db.Entity("user:okta_user_001").Set("email", "alice@example.com").Update(ctx)
}

func TestEntityBuilderExists(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Test existing entity
	exists, err := db.Entity("user:okta_user_001").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected okta_user_001 to exist")
	}

	// Test non-existing entity
	exists, err = db.Entity("user:nonexistent").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Expected nonexistent user to not exist")
	}
}

func TestEntityBuilderWithVersion(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Cleanup
	cleanup := func() {
		conn.ExecContext(ctx, "DELETE FROM users WHERE external_id = 'versioned_user'")
	}
	cleanup()
	t.Cleanup(cleanup)

	// Create entity with specific version
	result := db.Entity("user:versioned_user").
		Set("email", "versioned@test.com").
		Set("first_name", "Versioned").
		Set("last_name", "User").
		Set("status", "ACTIVE").
		Set("provider", "okta").
		Version(5).
		Create(ctx)

	if result.Err() != nil {
		t.Fatalf("Entity.Create() with version error = %v", result.Err())
	}

	// Verify version
	var version int
	err := conn.QueryRowContext(ctx, `
		SELECT version FROM users WHERE external_id = 'versioned_user'
	`).Scan(&version)
	if err != nil {
		t.Fatalf("Failed to query version: %v", err)
	}
	if version != 5 {
		t.Errorf("Expected version 5, got %d", version)
	}
}

func TestTupleTemporalValidFrom(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Cleanup
	cleanup := func() {
		conn.ExecContext(ctx, `DELETE FROM user_groups WHERE user_id IN (SELECT id FROM users WHERE external_id = 'ts_test_user')`)
		conn.ExecContext(ctx, `DELETE FROM users WHERE external_id = 'ts_test_user'`)
		conn.ExecContext(ctx, `DELETE FROM groups WHERE external_id = 'ts_test_group'`)
	}
	cleanup()
	t.Cleanup(cleanup)

	// First ensure test entities exist
	conn.ExecContext(ctx, `
		INSERT INTO users (external_id, email, first_name, last_name, status, provider)
		VALUES ('ts_test_user', 'ts@test.com', 'TS', 'Test', 'ACTIVE', 'okta')
		ON CONFLICT (external_id, version) DO NOTHING
	`)
	conn.ExecContext(ctx, `
		INSERT INTO groups (external_id, name, description, provider)
		VALUES ('ts_test_group', 'TS Test Group', 'For timestamp testing', 'okta')
		ON CONFLICT (external_id, version) DO NOTHING
	`)

	beforeCreate := time.Now()

	_, err := db.Tuple("user:ts_test_user", "member_of", "group:ts_test_group").
		With("role", "member").
		Create(ctx)

	if err != nil {
		t.Fatalf("Tuple.Create() error = %v", err)
	}

	afterCreate := time.Now()

	// Verify the temporal columns were set correctly
	var validFrom time.Time
	var validToStr string
	err = conn.QueryRowContext(ctx, `
		SELECT valid_from, valid_to::text FROM user_groups
		WHERE user_id = (SELECT id FROM users WHERE external_id = 'ts_test_user' AND valid_to = 'infinity')
		AND group_id = (SELECT id FROM groups WHERE external_id = 'ts_test_group' AND valid_to = 'infinity')
		AND valid_to = 'infinity'
	`).Scan(&validFrom, &validToStr)
	if err != nil {
		t.Fatalf("Failed to query temporal columns: %v", err)
	}

	// valid_from should be between beforeCreate and afterCreate
	if validFrom.Before(beforeCreate.Add(-time.Second)) || validFrom.After(afterCreate.Add(time.Second)) {
		t.Errorf("valid_from %v should be between %v and %v", validFrom, beforeCreate, afterCreate)
	}

	// valid_to should be infinity
	if validToStr != "infinity" {
		t.Errorf("Expected valid_to to be 'infinity', got %q", validToStr)
	}
}

func TestEntityWithChainedLinks(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Cleanup
	cleanup := func() {
		conn.ExecContext(ctx, "DELETE FROM user_groups WHERE user_id IN (SELECT id FROM users WHERE external_id = 'chained_user')")
		conn.ExecContext(ctx, "DELETE FROM users WHERE external_id = 'chained_user'")
	}
	cleanup()
	t.Cleanup(cleanup)

	// Create entity with chained links in one operation
	result := db.Entity("user:chained_user").
		Set("email", "chained@test.com").
		Set("first_name", "Chained").
		Set("last_name", "User").
		Set("status", "ACTIVE").
		Set("provider", "okta").
		Link("member_of", "group:okta_group_devs").WithMeta("role", "developer").
		Link("member_of", "group:okta_group_users").WithMeta("role", "member").
		Create(ctx)

	if result.Err() != nil {
		t.Fatalf("Entity.Create() with links error = %v", result.Err())
	}

	// Verify entity exists
	exists, err := db.Entity("user:chained_user").Exists(ctx)
	if err != nil {
		t.Fatalf("Entity.Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected entity to exist after Create()")
	}

	// Verify both links exist
	exists, err = db.Tuple("user:chained_user", "member_of", "group:okta_group_devs").Exists(ctx)
	if err != nil {
		t.Fatalf("Tuple.Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected link to okta_group_devs to exist")
	}

	exists, err = db.Tuple("user:chained_user", "member_of", "group:okta_group_users").Exists(ctx)
	if err != nil {
		t.Fatalf("Tuple.Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected link to okta_group_users to exist")
	}

	// Verify role metadata
	var role string
	err = conn.QueryRowContext(ctx, `
		SELECT role FROM user_groups
		WHERE user_id = (SELECT id FROM users WHERE external_id = 'chained_user' AND valid_to = 'infinity')
		AND group_id = (SELECT id FROM groups WHERE external_id = 'okta_group_devs' AND valid_to = 'infinity')
	`).Scan(&role)
	if err != nil {
		t.Fatalf("Failed to query role: %v", err)
	}
	if role != "developer" {
		t.Errorf("Expected role 'developer', got %q", role)
	}
}

func TestEntityWithMeta(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Cleanup
	cleanup := func() {
		conn.ExecContext(ctx, "DELETE FROM users WHERE external_id = 'meta_user'")
	}
	cleanup()
	t.Cleanup(cleanup)

	// Create entity with metadata
	result := db.Entity("user:meta_user").
		Set("email", "meta@test.com").
		Set("first_name", "Meta").
		Set("last_name", "User").
		Set("status", "ACTIVE").
		Set("provider", "okta").
		Meta("preferences", map[string]any{"theme": "dark", "language": "en"}).
		Meta("tags", []string{"admin", "power-user"}).
		Create(ctx)

	if result.Err() != nil {
		t.Fatalf("Entity.Create() with meta error = %v", result.Err())
	}

	// Verify raw_data was stored
	var rawData string
	err := conn.QueryRowContext(ctx, `
		SELECT raw_data FROM users WHERE external_id = 'meta_user' AND valid_to = 'infinity'
	`).Scan(&rawData)
	if err != nil {
		t.Fatalf("Failed to query raw_data: %v", err)
	}

	if rawData == "" {
		t.Error("Expected raw_data to be populated")
	}

	// Verify it contains our metadata
	if !strings.Contains(rawData, "dark") {
		t.Errorf("Expected raw_data to contain 'dark', got %s", rawData)
	}
	if !strings.Contains(rawData, "admin") {
		t.Errorf("Expected raw_data to contain 'admin', got %s", rawData)
	}
}

func TestEntityResultLink(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Clean up if exists
	conn.ExecContext(ctx, "DELETE FROM user_groups WHERE user_id IN (SELECT id FROM users WHERE external_id = 'result_link_user')")
	conn.ExecContext(ctx, "DELETE FROM users WHERE external_id = 'result_link_user'")

	// Ensure cleanup after test to not affect other tests
	t.Cleanup(func() {
		conn.ExecContext(ctx, "DELETE FROM user_groups WHERE user_id IN (SELECT id FROM users WHERE external_id = 'result_link_user')")
		conn.ExecContext(ctx, "DELETE FROM users WHERE external_id = 'result_link_user'")
	})

	// Create entity first, then chain Link() on result
	result := db.Entity("user:result_link_user").
		Set("email", "resultlink@test.com").
		Set("first_name", "Result").
		Set("last_name", "Link").
		Set("status", "ACTIVE").
		Set("provider", "okta").
		Create(ctx)

	if result.Err() != nil {
		t.Fatalf("Entity.Create() error = %v", result.Err())
	}

	// Now link using the result - use a different group to not affect other tests
	_, err := result.Link("member_of", "group:okta_group_devs").
		With("role", "developer").
		Create(ctx)

	if err != nil {
		t.Fatalf("Result.Link().Create() error = %v", err)
	}

	// Verify link exists
	exists, err := db.Tuple("user:result_link_user", "member_of", "group:okta_group_devs").Exists(ctx)
	if err != nil {
		t.Fatalf("Tuple.Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected link to exist after Result.Link()")
	}
}

func TestEntityRawData(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Cleanup
	cleanup := func() {
		conn.ExecContext(ctx, "DELETE FROM users WHERE external_id = 'rawdata_user'")
	}
	cleanup()
	t.Cleanup(cleanup)

	// Create entity with full raw_data
	originalData := map[string]any{
		"source":       "okta",
		"original_id":  "12345",
		"imported_at":  "2024-01-01",
		"extra_fields": map[string]any{"foo": "bar"},
	}

	result := db.Entity("user:rawdata_user").
		Set("email", "rawdata@test.com").
		Set("first_name", "RawData").
		Set("last_name", "User").
		Set("status", "ACTIVE").
		Set("provider", "okta").
		RawData(originalData).
		Create(ctx)

	if result.Err() != nil {
		t.Fatalf("Entity.Create() with RawData error = %v", result.Err())
	}

	// Verify raw_data was stored correctly
	var rawData string
	err := conn.QueryRowContext(ctx, `
		SELECT raw_data FROM users WHERE external_id = 'rawdata_user' AND valid_to = 'infinity'
	`).Scan(&rawData)
	if err != nil {
		t.Fatalf("Failed to query raw_data: %v", err)
	}

	if !strings.Contains(rawData, "okta") || !strings.Contains(rawData, "12345") {
		t.Errorf("Expected raw_data to contain original values, got %s", rawData)
	}
}

// TestTemporalRelationshipSoftDelete tests that deleting a temporal relationship
// performs a soft delete (sets valid_to to now()) instead of hard delete
func TestTemporalRelationshipSoftDelete(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// Cleanup
	cleanup := func() {
		conn.ExecContext(ctx, `DELETE FROM user_groups WHERE user_id IN (SELECT id FROM users WHERE external_id = 'temporal_rel_user')`)
		conn.ExecContext(ctx, `DELETE FROM users WHERE external_id = 'temporal_rel_user'`)
		conn.ExecContext(ctx, `DELETE FROM groups WHERE external_id = 'temporal_rel_group'`)
	}
	cleanup()
	t.Cleanup(cleanup)

	// Create test entities
	conn.ExecContext(ctx, `
		INSERT INTO users (external_id, email, first_name, last_name, status, provider)
		VALUES ('temporal_rel_user', 'temporal@test.com', 'Temporal', 'Test', 'ACTIVE', 'okta')
	`)
	conn.ExecContext(ctx, `
		INSERT INTO groups (external_id, name, description, provider)
		VALUES ('temporal_rel_group', 'Temporal Test Group', 'For temporal testing', 'okta')
	`)

	// Create a relationship
	_, err := db.Tuple("user:temporal_rel_user", "member_of", "group:temporal_rel_group").
		With("role", "member").
		Create(ctx)
	if err != nil {
		t.Fatalf("Create relationship error = %v", err)
	}

	// Verify it exists
	exists, err := db.Tuple("user:temporal_rel_user", "member_of", "group:temporal_rel_group").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Expected relationship to exist after Create()")
	}

	// Delete the relationship (should soft delete)
	_, err = db.Tuple("user:temporal_rel_user", "member_of", "group:temporal_rel_group").Delete(ctx)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it doesn't exist at current time
	exists, err = db.Tuple("user:temporal_rel_user", "member_of", "group:temporal_rel_group").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() after delete error = %v", err)
	}
	if exists {
		t.Error("Expected relationship to NOT exist at current time after soft delete")
	}

	// Verify the row still exists in the database (soft delete preserves history)
	var count int
	err = conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM user_groups
		WHERE user_id = (SELECT id FROM users WHERE external_id = 'temporal_rel_user' AND valid_to = 'infinity')
		AND group_id = (SELECT id FROM groups WHERE external_id = 'temporal_rel_group' AND valid_to = 'infinity')
	`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows: %v", err)
	}
	if count == 0 {
		t.Error("Expected soft-deleted row to still exist in database")
	}
}

// TestTemporalRelationshipPointInTime tests querying relationships at a specific point in time
func TestTemporalRelationshipPointInTime(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)
	ctx := context.Background()

	// The init.sql includes historical relationship data:
	// Bob (okta_user_002) was in admins group from 2024-01-01 to 2024-06-01
	// Let's test if we can query this historical relationship

	// At current time, Bob should NOT be in admins
	exists, err := db.Tuple("user:okta_user_002", "member_of", "group:okta_group_admins").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() at current time error = %v", err)
	}
	if exists {
		t.Error("Expected Bob to NOT be in admins group at current time")
	}

	// At March 2024, Bob SHOULD be in admins
	march2024 := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	exists, err = db.Tuple("user:okta_user_002", "member_of", "group:okta_group_admins").
		At(march2024).
		Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() at March 2024 error = %v", err)
	}
	if !exists {
		t.Error("Expected Bob to BE in admins group in March 2024")
	}

	// At December 2023 (before he joined), Bob should NOT be in admins
	dec2023 := time.Date(2023, 12, 15, 0, 0, 0, 0, time.UTC)
	exists, err = db.Tuple("user:okta_user_002", "member_of", "group:okta_group_admins").
		At(dec2023).
		Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() at Dec 2023 error = %v", err)
	}
	if exists {
		t.Error("Expected Bob to NOT be in admins group in December 2023")
	}

	// At July 2024 (after he left), Bob should NOT be in admins
	july2024 := time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC)
	exists, err = db.Tuple("user:okta_user_002", "member_of", "group:okta_group_admins").
		At(july2024).
		Exists(ctx)
	if err != nil {
		t.Fatalf("Exists() at July 2024 error = %v", err)
	}
	if exists {
		t.Error("Expected Bob to NOT be in admins group in July 2024")
	}
}
