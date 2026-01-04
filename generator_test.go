package doodle

import (
	"strings"
	"testing"
)

func testSchema() *Schema {
	s := NewSchema()

	s.AddEntity("user", "users", "id").
		WithTemporal().
		AddField("email", "email").
		AddField("status", "status").
		AddField("provider", "provider").
		AddField("created", "created_at")

	s.AddEntity("group", "groups", "id").
		WithTemporal().
		AddField("name", "name").
		AddField("description", "description")

	s.AddEntity("app", "apps", "id").
		WithTemporal().
		AddField("name", "name").
		AddField("app_type", "app_type")

	s.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")
	s.AddRelationship("has_access", "group", "app", "group_apps", "group_id", "app_id")
	// Self-referential relationship for testing recursive paths
	s.AddRelationship("reports_to", "user", "user", "user_managers", "user_id", "manager_id")

	return s
}

func testSchemaNoTemporal() *Schema {
	s := NewSchema()

	s.AddEntity("user", "users", "id").
		AddField("email", "email").
		AddField("status", "status")

	s.AddEntity("group", "groups", "id").
		AddField("name", "name")

	s.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")

	return s
}

func TestGenerateSimpleSelect(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have SELECT, FROM, WHERE
	if !strings.Contains(result.SQL, "SELECT t0.*") {
		t.Errorf("SQL missing SELECT clause: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "FROM users t0") {
		t.Errorf("SQL missing FROM clause: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "external_id = $1") {
		t.Errorf("SQL missing WHERE clause: %s", result.SQL)
	}

	if len(result.Params) != 1 {
		t.Errorf("len(Params) = %d, want 1", len(result.Params))
	}
	if result.Params[0] != "okta_123" {
		t.Errorf("Params[0] = %v, want okta_123", result.Params[0])
	}
}

func TestGenerateSingleHopPath(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT ->member_of->group FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have JOINs
	if !strings.Contains(result.SQL, "JOIN user_groups") {
		t.Errorf("SQL missing junction table JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN groups") {
		t.Errorf("SQL missing target table JOIN: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateMultiHopPath(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT ->member_of->group->has_access->app FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have all JOINs
	if !strings.Contains(result.SQL, "JOIN user_groups") {
		t.Errorf("SQL missing user_groups JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN groups") {
		t.Errorf("SQL missing groups JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN group_apps") {
		t.Errorf("SQL missing group_apps JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN apps") {
		t.Errorf("SQL missing apps JOIN: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateWithVersion(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 VERSION d'2024-01-15T00:00:00Z'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should use temporal WHERE clauses, not FOR SYSTEM_TIME
	if !strings.Contains(result.SQL, "valid_from <= $2") {
		t.Errorf("SQL missing valid_from clause: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "valid_to > $2") {
		t.Errorf("SQL missing valid_to clause: %s", result.SQL)
	}

	// Timestamp should be in params
	if len(result.Params) != 2 {
		t.Errorf("len(Params) = %d, want 2", len(result.Params))
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateWithVersionNumber(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 VERSION 3"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "version = $2") {
		t.Errorf("SQL missing version clause: %s", result.SQL)
	}

	if len(result.Params) != 2 {
		t.Errorf("len(Params) = %d, want 2", len(result.Params))
	}
	if result.Params[1] != 3 {
		t.Errorf("Params[1] = %v, want 3", result.Params[1])
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateWithWhere(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 WHERE status = 'ACTIVE'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "status = $2") {
		t.Errorf("SQL missing WHERE condition: %s", result.SQL)
	}

	if len(result.Params) != 2 {
		t.Errorf("len(Params) = %d, want 2", len(result.Params))
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateWithLimit(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 LIMIT 10"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "LIMIT 10") {
		t.Errorf("SQL missing LIMIT: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateInOperator(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 WHERE status IN ('ACTIVE', 'PENDING')"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "status IN") {
		t.Errorf("SQL missing IN clause: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateComplexQuery(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := `SELECT ->member_of->group->has_access->app
              FROM user:okta_user_001
              VERSION d'2024-01-01T00:00:00Z'
              WHERE status = 'ACTIVE'
              LIMIT 100`

	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify all components present
	checks := []string{
		"SELECT",
		"FROM users",
		"JOIN user_groups",
		"JOIN groups",
		"JOIN group_apps",
		"JOIN apps",
		"valid_from <=",
		"valid_to >",
		"WHERE",
		"external_id",
		"LIMIT 100",
	}

	for _, check := range checks {
		if !strings.Contains(result.SQL, check) {
			t.Errorf("SQL missing '%s': %s", check, result.SQL)
		}
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateUnknownEntity(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM unknown_entity:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, err = gen.Generate(q)
	if err == nil {
		t.Error("Generate() should error on unknown entity")
	}
}

func TestGenerateUnknownRelationship(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT ->unknown_rel->group FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, err = gen.Generate(q)
	if err == nil {
		t.Error("Generate() should error on unknown relationship")
	}
}

func TestSchemaValidation(t *testing.T) {
	s := NewSchema()
	s.AddEntity("user", "users", "id")

	// Add relationship to non-existent entity
	s.AddRelationship("broken", "user", "nonexistent", "broken_table", "user_id", "other_id")

	err := s.Validate()
	if err == nil {
		t.Error("Validate() should error on broken relationship")
	}
}

func TestGenerateIncomingEdge(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Get all users that are members of a group
	// group <- member_of <- user
	input := "SELECT <-member_of<-user FROM group:admins"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should join from groups -> user_groups -> users
	if !strings.Contains(result.SQL, "FROM groups t0") {
		t.Errorf("SQL should start from groups: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN user_groups") {
		t.Errorf("SQL missing junction table JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN users") {
		t.Errorf("SQL missing users JOIN: %s", result.SQL)
	}
	// Key direction should be reversed: groups.id = j.group_id, j.user_id = users.id
	if !strings.Contains(result.SQL, "t0.id = j0.group_id") {
		t.Errorf("SQL should join on group_id for incoming edge: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "j0.user_id = t1.id") {
		t.Errorf("SQL should join to users.id: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateIncomingEdgeMultiHop(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Get all users that have access to an app (through groups)
	// app <- has_access <- group <- member_of <- user
	input := "SELECT <-has_access<-group<-member_of<-user FROM app:slack"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have all tables
	if !strings.Contains(result.SQL, "FROM apps t0") {
		t.Errorf("SQL should start from apps: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN group_apps") {
		t.Errorf("SQL missing group_apps JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN groups") {
		t.Errorf("SQL missing groups JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN user_groups") {
		t.Errorf("SQL missing user_groups JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN users") {
		t.Errorf("SQL missing users JOIN: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateOrderBy(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 ORDER BY email DESC"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "ORDER BY t0.email DESC") {
		t.Errorf("SQL missing ORDER BY: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateOrderByMultiple(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 ORDER BY status ASC, email DESC"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "ORDER BY t0.status ASC, t0.email DESC") {
		t.Errorf("SQL missing ORDER BY: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateOffset(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 LIMIT 10 OFFSET 20"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "LIMIT 10") {
		t.Errorf("SQL missing LIMIT: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "OFFSET 20") {
		t.Errorf("SQL missing OFFSET: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateFieldSelection(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT email, status FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "SELECT t0.email, t0.status") {
		t.Errorf("SQL missing field selection: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateCountStar(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT COUNT(*) FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "SELECT COUNT(*)") {
		t.Errorf("SQL missing COUNT(*): %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateCountPath(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT COUNT(->member_of->group) FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have JOINs and COUNT on the group's id
	if !strings.Contains(result.SQL, "COUNT(t1.id)") {
		t.Errorf("SQL missing COUNT(t1.id): %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN user_groups") {
		t.Errorf("SQL missing user_groups JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN groups") {
		t.Errorf("SQL missing groups JOIN: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateOrCondition(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 WHERE status = 'ACTIVE' OR status = 'PENDING'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have OR in the WHERE clause
	if !strings.Contains(result.SQL, "OR") {
		t.Errorf("SQL missing OR: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "t0.status =") {
		t.Errorf("SQL missing status condition: %s", result.SQL)
	}

	// Should have 3 params: external_id, ACTIVE, PENDING
	if len(result.Params) != 3 {
		t.Errorf("len(Params) = %d, want 3", len(result.Params))
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateComplexOrAnd(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 WHERE status = 'ACTIVE' AND provider = 'okta' OR status = 'PENDING'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have both AND and OR
	if !strings.Contains(result.SQL, "AND") {
		t.Errorf("SQL missing AND: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "OR") {
		t.Errorf("SQL missing OR: %s", result.SQL)
	}

	// Should have proper grouping with parentheses
	if !strings.Contains(result.SQL, "(t0.status") {
		t.Errorf("SQL missing parentheses for grouping: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateDistinct(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT DISTINCT status FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "SELECT DISTINCT t0.status") {
		t.Errorf("SQL missing DISTINCT: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateDistinctStar(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT DISTINCT * FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "SELECT DISTINCT t0.*") {
		t.Errorf("SQL missing DISTINCT: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateCountDistinct(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT COUNT(DISTINCT status) FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "COUNT(DISTINCT t0.status)") {
		t.Errorf("SQL missing COUNT DISTINCT: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateBetween(t *testing.T) {
	schema := testSchemaWithScore()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user:okta_123 WHERE score BETWEEN 10 AND 100"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "score BETWEEN $2 AND $3") {
		t.Errorf("SQL missing BETWEEN: %s", result.SQL)
	}

	if len(result.Params) != 3 {
		t.Errorf("len(Params) = %d, want 3", len(result.Params))
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func testSchemaWithScore() *Schema {
	s := NewSchema()

	s.AddEntity("user", "users", "id").
		WithTemporal().
		AddField("email", "email").
		AddField("status", "status").
		AddField("score", "score")

	return s
}

func TestGenerateGroupBy(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT status, COUNT(*) FROM user:okta_123 GROUP BY status"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "GROUP BY t0.status") {
		t.Errorf("SQL missing GROUP BY: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateGroupByMultiple(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT status, provider, COUNT(*) FROM user:okta_123 GROUP BY status, provider"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "GROUP BY t0.status, t0.provider") {
		t.Errorf("SQL missing GROUP BY: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateHaving(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT status, COUNT(*) FROM user:okta_123 GROUP BY status HAVING COUNT(*) > 5"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "HAVING COUNT(*) > $2") {
		t.Errorf("SQL missing HAVING: %s", result.SQL)
	}

	if len(result.Params) != 2 {
		t.Errorf("len(Params) = %d, want 2", len(result.Params))
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateFieldAlias(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT email AS e, status AS s FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "t0.email AS e") {
		t.Errorf("SQL missing email alias: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "t0.status AS s") {
		t.Errorf("SQL missing status alias: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateFromAlias(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT u.email FROM user:okta_123 AS u"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Note: The generator uses t0 alias internally
	if !strings.Contains(result.SQL, "SELECT") || !strings.Contains(result.SQL, "email") {
		t.Errorf("SQL missing field selection: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateOptionalTraversal(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT ->?member_of->group FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have LEFT JOINs instead of regular JOINs
	if !strings.Contains(result.SQL, "LEFT JOIN user_groups") {
		t.Errorf("SQL missing LEFT JOIN user_groups: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "LEFT JOIN groups") {
		t.Errorf("SQL missing LEFT JOIN groups: %s", result.SQL)
	}

	// Should NOT have regular JOINs for the path
	if strings.Contains(result.SQL, "JOIN user_groups") && !strings.Contains(result.SQL, "LEFT JOIN user_groups") {
		t.Errorf("SQL should use LEFT JOIN not JOIN: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateOptionalIncomingTraversal(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT <?-member_of<?-user FROM group:admins"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have LEFT JOINs
	if !strings.Contains(result.SQL, "LEFT JOIN user_groups") {
		t.Errorf("SQL missing LEFT JOIN user_groups: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "LEFT JOIN users") {
		t.Errorf("SQL missing LEFT JOIN users: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateMixedOptionalTraversal(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// First hop is optional, second is required
	input := "SELECT ->?member_of->group->has_access->app FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// First hop should be LEFT JOIN
	if !strings.Contains(result.SQL, "LEFT JOIN user_groups") {
		t.Errorf("SQL missing LEFT JOIN user_groups: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "LEFT JOIN groups") {
		t.Errorf("SQL missing LEFT JOIN groups: %s", result.SQL)
	}

	// Second hop should be regular JOIN
	// Note: The order matters - group_apps and apps should be regular JOINs
	sql := result.SQL
	// Find the position of group_apps join - it should NOT be preceded by LEFT
	if strings.Contains(sql, "LEFT JOIN group_apps") {
		t.Errorf("Second hop should use regular JOIN, not LEFT JOIN: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGeneratePathFieldAccess(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Access junction table field during traversal
	input := "SELECT ->member_of.role->group FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should include junction table field in SELECT
	if !strings.Contains(result.SQL, "j0.role") {
		t.Errorf("SQL missing junction field j0.role: %s", result.SQL)
	}

	// Should still have target entity select
	if !strings.Contains(result.SQL, "t1.*") {
		t.Errorf("SQL missing target entity select t1.*: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGeneratePathFieldAccessMultiple(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Multiple junction fields across hops
	input := "SELECT ->member_of.role->group->has_access.permission->app FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should include both junction table fields
	if !strings.Contains(result.SQL, "j0.role") {
		t.Errorf("SQL missing junction field j0.role: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "j1.permission") {
		t.Errorf("SQL missing junction field j1.permission: %s", result.SQL)
	}

	// Should still have target entity select
	if !strings.Contains(result.SQL, "t2.*") {
		t.Errorf("SQL missing target entity select t2.*: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGeneratePathFieldAccessWithOptional(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Combine optional traversal with field access
	input := "SELECT ->?member_of.role->group FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should use LEFT JOIN
	if !strings.Contains(result.SQL, "LEFT JOIN user_groups") {
		t.Errorf("SQL missing LEFT JOIN user_groups: %s", result.SQL)
	}

	// Should include junction table field
	if !strings.Contains(result.SQL, "j0.role") {
		t.Errorf("SQL missing junction field j0.role: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateNegatedPath(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Find users who are NOT members of a specific group
	input := "SELECT * FROM user:okta_123 WHERE ->!member_of->group.external_id = 'admins'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should contain NOT EXISTS
	if !strings.Contains(result.SQL, "NOT EXISTS") {
		t.Errorf("SQL missing NOT EXISTS: %s", result.SQL)
	}

	// Should have subquery with junction and target tables (using s prefix for subquery)
	if !strings.Contains(result.SQL, "user_groups sj0") {
		t.Errorf("SQL missing user_groups sj0: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "groups s0") {
		t.Errorf("SQL missing groups s0 in subquery: %s", result.SQL)
	}

	// Should have condition connecting back to main query
	if !strings.Contains(result.SQL, "sj0.user_id = t0.id") {
		t.Errorf("SQL missing connection to main query: %s", result.SQL)
	}

	// Should have target field condition in subquery
	if !strings.Contains(result.SQL, "s0.external_id") {
		t.Errorf("SQL missing target field condition: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateNegatedPathNoFilter(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Find all users - just check that negated paths work in more complex queries
	input := "SELECT * FROM user WHERE status = 'ACTIVE' AND ->!member_of->group.external_id = 'disabled'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should contain NOT EXISTS
	if !strings.Contains(result.SQL, "NOT EXISTS") {
		t.Errorf("SQL missing NOT EXISTS: %s", result.SQL)
	}

	// Should have the regular condition too
	if !strings.Contains(result.SQL, "status") {
		t.Errorf("SQL missing regular condition: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGeneratePathQuantifierSingleHop(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// {1} quantifier should work the same as no quantifier
	input := "SELECT ->member_of{1}->group FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate normal JOINs
	if !strings.Contains(result.SQL, "JOIN user_groups") {
		t.Errorf("SQL missing JOIN user_groups: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN groups") {
		t.Errorf("SQL missing JOIN groups: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGeneratePathQuantifierVariableLength(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Variable-length quantifier with self-referential relationship
	input := "SELECT ->reports_to{1,3}->user FROM user:alice"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should contain WITH RECURSIVE
	if !strings.Contains(result.SQL, "WITH RECURSIVE") {
		t.Errorf("SQL missing WITH RECURSIVE: %s", result.SQL)
	}

	// Should contain path_cte
	if !strings.Contains(result.SQL, "path_cte") {
		t.Errorf("SQL missing path_cte: %s", result.SQL)
	}

	// Should contain UNION ALL for recursive query
	if !strings.Contains(result.SQL, "UNION ALL") {
		t.Errorf("SQL missing UNION ALL: %s", result.SQL)
	}

	// Should have depth constraints
	if !strings.Contains(result.SQL, "depth >= 1") {
		t.Errorf("SQL missing min depth constraint: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "depth < 3") {
		t.Errorf("SQL missing max depth constraint: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateIsNull(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user WHERE status IS NULL"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "IS NULL") {
		t.Errorf("SQL missing IS NULL: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "t0.status IS NULL") {
		t.Errorf("SQL should have t0.status IS NULL: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateIsNotNull(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user WHERE email IS NOT NULL"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "IS NOT NULL") {
		t.Errorf("SQL missing IS NOT NULL: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "t0.email IS NOT NULL") {
		t.Errorf("SQL should have t0.email IS NOT NULL: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateExists(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user WHERE EXISTS (SELECT * FROM group WHERE name = 'admins')"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "EXISTS") {
		t.Errorf("SQL missing EXISTS: %s", result.SQL)
	}
	// Should contain the subquery for groups
	if !strings.Contains(result.SQL, "groups") {
		t.Errorf("SQL missing subquery table: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateNotExists(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user WHERE NOT EXISTS (SELECT * FROM group WHERE name = 'banned')"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "NOT EXISTS") {
		t.Errorf("SQL missing NOT EXISTS: %s", result.SQL)
	}
	// Should contain the subquery for groups
	if !strings.Contains(result.SQL, "groups") {
		t.Errorf("SQL missing subquery table: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateNotCondition(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user WHERE NOT status = 'ACTIVE'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "NOT") {
		t.Errorf("SQL missing NOT: %s", result.SQL)
	}
	// Should be NOT t0.status = $1 or equivalent
	if !strings.Contains(result.SQL, "NOT t0.status") {
		t.Errorf("SQL should have NOT t0.status: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateUnion(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user WHERE status = 'ACTIVE' UNION SELECT * FROM user WHERE status = 'PENDING'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "UNION") {
		t.Errorf("SQL missing UNION: %s", result.SQL)
	}

	// Should have two separate SELECT statements
	if strings.Count(result.SQL, "SELECT") != 2 {
		t.Errorf("Expected 2 SELECT statements, got %d in: %s", strings.Count(result.SQL, "SELECT"), result.SQL)
	}

	// Should have two parameters (ACTIVE, PENDING)
	if len(result.Params) != 2 {
		t.Errorf("Expected 2 params, got %d: %v", len(result.Params), result.Params)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateUnionAll(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user UNION ALL SELECT * FROM user WHERE status = 'ACTIVE'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "UNION ALL") {
		t.Errorf("SQL missing UNION ALL: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateIntersect(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user WHERE status = 'ACTIVE' INTERSECT SELECT * FROM user WHERE provider = 'okta'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "INTERSECT") {
		t.Errorf("SQL missing INTERSECT: %s", result.SQL)
	}

	// Should have two parameters
	if len(result.Params) != 2 {
		t.Errorf("Expected 2 params, got %d: %v", len(result.Params), result.Params)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateExcept(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user EXCEPT SELECT * FROM user WHERE status = 'INACTIVE'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "EXCEPT") {
		t.Errorf("SQL missing EXCEPT: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateMultipleUnion(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT * FROM user WHERE status = 'A' UNION SELECT * FROM user WHERE status = 'B' UNION SELECT * FROM user WHERE status = 'C'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have three SELECT statements and two UNIONs
	if strings.Count(result.SQL, "SELECT") != 3 {
		t.Errorf("Expected 3 SELECT statements, got %d in: %s", strings.Count(result.SQL, "SELECT"), result.SQL)
	}

	if strings.Count(result.SQL, "UNION") != 2 {
		t.Errorf("Expected 2 UNION operators, got %d in: %s", strings.Count(result.SQL, "UNION"), result.SQL)
	}

	// Should have three parameters
	if len(result.Params) != 3 {
		t.Errorf("Expected 3 params, got %d: %v", len(result.Params), result.Params)
	}

	// Params should be $1, $2, $3
	if !strings.Contains(result.SQL, "$1") || !strings.Contains(result.SQL, "$2") || !strings.Contains(result.SQL, "$3") {
		t.Errorf("SQL should have $1, $2, $3: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateCaseExpression(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT CASE WHEN status = 'ACTIVE' THEN 'yes' ELSE 'no' END AS is_active FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "CASE") {
		t.Errorf("SQL missing CASE: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "WHEN") {
		t.Errorf("SQL missing WHEN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "THEN") {
		t.Errorf("SQL missing THEN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "ELSE") {
		t.Errorf("SQL missing ELSE: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "END") {
		t.Errorf("SQL missing END: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "AS is_active") {
		t.Errorf("SQL missing alias: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateCaseMultipleWhens(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT CASE WHEN status = 'ACTIVE' THEN 1 WHEN status = 'PENDING' THEN 2 ELSE 0 END AS status_code FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have two WHEN clauses
	if strings.Count(result.SQL, "WHEN") != 2 {
		t.Errorf("Expected 2 WHEN clauses, got %d in: %s", strings.Count(result.SQL, "WHEN"), result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateCoalesce(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT COALESCE(email, 'unknown') AS email FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "COALESCE") {
		t.Errorf("SQL missing COALESCE: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "t0.email") {
		t.Errorf("SQL missing field reference: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "'unknown'") {
		t.Errorf("SQL missing fallback value: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateNullif(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT NULLIF(status, 'DELETED') AS status FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "NULLIF") {
		t.Errorf("SQL missing NULLIF: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "t0.status") {
		t.Errorf("SQL missing field reference: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "'DELETED'") {
		t.Errorf("SQL missing comparison value: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateCTE(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "WITH active_users AS (SELECT * FROM user WHERE status = 'ACTIVE') SELECT * FROM active_users"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "WITH active_users AS") {
		t.Errorf("SQL missing WITH clause: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "SELECT active_users.*") {
		t.Errorf("SQL missing main query: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateMultipleCTEs(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "WITH active AS (SELECT * FROM user WHERE status = 'ACTIVE'), admins AS (SELECT * FROM group WHERE name = 'admins') SELECT * FROM active"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "WITH active AS") {
		t.Errorf("SQL missing first CTE: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "admins AS") {
		t.Errorf("SQL missing second CTE: %s", result.SQL)
	}

	// Should have two parameters
	if len(result.Params) != 2 {
		t.Errorf("Expected 2 params, got %d: %v", len(result.Params), result.Params)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateUpperFunction(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT UPPER(email) AS upper_email FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "UPPER(t0.email)") {
		t.Errorf("SQL missing UPPER function: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "AS upper_email") {
		t.Errorf("SQL missing alias: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateConcatFunction(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT CONCAT(email, '-suffix') AS combined FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "CONCAT(t0.email, '-suffix')") {
		t.Errorf("SQL missing CONCAT function: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateNowFunction(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT NOW() AS current_time FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "NOW()") {
		t.Errorf("SQL missing NOW function: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateMathFunctions(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	tests := []struct {
		input    string
		expected string
	}{
		{"SELECT ABS(-5) FROM user", "ABS(-5)"},
		{"SELECT ROUND(3.14) FROM user", "ROUND(3.14)"},
		{"SELECT CEIL(3.14) FROM user", "CEIL(3.14)"},
		{"SELECT FLOOR(3.14) FROM user", "FLOOR(3.14)"},
	}

	for _, tt := range tests {
		q, err := Parse(tt.input)
		if err != nil {
			t.Fatalf("Parse(%s) error = %v", tt.input, err)
		}

		result, err := gen.Generate(q)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if !strings.Contains(result.SQL, tt.expected) {
			t.Errorf("SQL for %s missing %s: %s", tt.input, tt.expected, result.SQL)
		}

		t.Logf("Input: %s -> SQL: %s", tt.input, result.SQL)
	}
}

func TestGenerateArrayAgg(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT ARRAY_AGG(email) FROM user GROUP BY status"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "ARRAY_AGG(t0.email)") {
		t.Errorf("SQL missing ARRAY_AGG: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateStringAgg(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT STRING_AGG(email, ', ') FROM user GROUP BY status"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "STRING_AGG(t0.email, ', ')") {
		t.Errorf("SQL missing STRING_AGG with delimiter: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateJsonAgg(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT JSON_AGG(email) FROM user GROUP BY status"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "JSON_AGG(t0.email)") {
		t.Errorf("SQL missing JSON_AGG: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateArrayAggDistinct(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT ARRAY_AGG(DISTINCT status) FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.SQL, "ARRAY_AGG(DISTINCT t0.status)") {
		t.Errorf("SQL missing ARRAY_AGG with DISTINCT: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateJsonGet(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT JSON_GET(email, 'domain') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate: (t0.email->'domain')
	if !strings.Contains(result.SQL, "(t0.email->'domain')") {
		t.Errorf("SQL missing JSON -> operator: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateJsonText(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT JSON_TEXT(email, 'domain') AS domain FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate: (t0.email->>'domain')
	if !strings.Contains(result.SQL, "(t0.email->>'domain')") {
		t.Errorf("SQL missing JSON ->> operator: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateJsonPath(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT JSON_PATH(email, 'address', 'city') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate: (t0.email#>'{'address','city'}')
	if !strings.Contains(result.SQL, "#>'{") {
		t.Errorf("SQL missing JSON #> operator: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateJsonPathText(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT JSON_PATH_TEXT(email, 'address', 'city') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate: (t0.email#>>'{'address','city'}')
	if !strings.Contains(result.SQL, "#>>'{") {
		t.Errorf("SQL missing JSON #>> operator: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateJsonBuildObject(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT JSON_BUILD_OBJECT('name', email, 'status', status) FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate: json_build_object('name', t0.email, 'status', t0.status)
	if !strings.Contains(result.SQL, "json_build_object(") {
		t.Errorf("SQL missing json_build_object: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateDateArithmeticAdd(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT created + INTERVAL '1 day' AS tomorrow FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate: (t0.created + INTERVAL '1 day')
	if !strings.Contains(result.SQL, "(t0.created + INTERVAL '1 day')") {
		t.Errorf("SQL missing date arithmetic: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateDateArithmeticSubtract(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT created - INTERVAL '30 days' AS past FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate: (t0.created - INTERVAL '30 days')
	if !strings.Contains(result.SQL, "(t0.created - INTERVAL '30 days')") {
		t.Errorf("SQL missing date arithmetic: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateDateArithmeticHours(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT created + INTERVAL '2 hours' AS future FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should generate: (t0.created + INTERVAL '2 hours')
	if !strings.Contains(result.SQL, "(t0.created + INTERVAL '2 hours')") {
		t.Errorf("SQL missing date arithmetic: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateStringAggDistinct(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "SELECT STRING_AGG(DISTINCT name, ', ') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should include DISTINCT in STRING_AGG
	if !strings.Contains(result.SQL, "STRING_AGG(DISTINCT") {
		t.Errorf("SQL missing DISTINCT in STRING_AGG: %s", result.SQL)
	}

	// Should include delimiter
	if !strings.Contains(result.SQL, "', '") {
		t.Errorf("SQL missing delimiter: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGeneratePathQuantifierWithDifferentBounds(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Test with {2,5} bounds
	input := "SELECT ->reports_to{2,5}->user FROM user:alice"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have recursive CTE with proper depth constraints
	if !strings.Contains(result.SQL, "WITH RECURSIVE path_cte") {
		t.Errorf("SQL missing recursive CTE: %s", result.SQL)
	}

	// Should check depth >= 2 (minHops)
	if !strings.Contains(result.SQL, "depth >= 2") {
		t.Errorf("SQL should have depth >= 2: %s", result.SQL)
	}

	// Should check depth < 5 (maxHops)
	if !strings.Contains(result.SQL, "depth < 5") {
		t.Errorf("SQL should have depth < 5: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGeneratePathQuantifierExactHops(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Test with exact hop count {3} - same as {3,3}
	input := "SELECT ->reports_to{3}->user FROM user:alice"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// For exact hops, depth should be exactly 3
	if !strings.Contains(result.SQL, "depth >= 3") {
		t.Errorf("SQL should have depth >= 3: %s", result.SQL)
	}

	if !strings.Contains(result.SQL, "depth < 3") {
		t.Errorf("SQL should have depth < 3: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

// ============================================
// INSERT/UPDATE/DELETE Generator Tests
// ============================================

func TestGenerateInsertValues(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "INSERT INTO user (email, status) VALUES ('alice@example.com', 'ACTIVE')"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "INSERT INTO users") {
		t.Errorf("SQL missing INSERT INTO users: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "(email, status)") {
		t.Errorf("SQL missing column list: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "VALUES") {
		t.Errorf("SQL missing VALUES: %s", result.SQL)
	}

	if len(result.Params) != 2 {
		t.Errorf("Expected 2 params, got %d: %v", len(result.Params), result.Params)
	}
	if result.Params[0] != "alice@example.com" {
		t.Errorf("Params[0] = %v, want alice@example.com", result.Params[0])
	}
	if result.Params[1] != "ACTIVE" {
		t.Errorf("Params[1] = %v, want ACTIVE", result.Params[1])
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateInsertWithID(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "INSERT INTO user:alice (email, status) VALUES ('alice@example.com', 'ACTIVE')"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "external_id") {
		t.Errorf("SQL missing external_id: %s", result.SQL)
	}

	// Should have 3 params: external_id, email, status
	if len(result.Params) != 3 {
		t.Errorf("Expected 3 params, got %d: %v", len(result.Params), result.Params)
	}
	if result.Params[0] != "alice" {
		t.Errorf("Params[0] = %v, want alice", result.Params[0])
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateInsertMultipleRows(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "INSERT INTO user (email, status) VALUES ('a@example.com', 'ACTIVE'), ('b@example.com', 'PENDING')"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// Should have 4 params: 2 rows x 2 columns
	if len(result.Params) != 4 {
		t.Errorf("Expected 4 params, got %d: %v", len(result.Params), result.Params)
	}

	// Should have two value groups
	if strings.Count(result.SQL, "(") < 2 {
		t.Errorf("SQL should have multiple value groups: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateInsertTemporal(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "INSERT INTO user (email, status) VALUES ('alice@example.com', 'ACTIVE')"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// Should include temporal columns
	if !strings.Contains(result.SQL, "version") {
		t.Errorf("SQL missing version column: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "valid_from") {
		t.Errorf("SQL missing valid_from column: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "valid_to") {
		t.Errorf("SQL missing valid_to column: %s", result.SQL)
	}

	// Should have temporal values in VALUES
	if !strings.Contains(result.SQL, "NOW()") {
		t.Errorf("SQL missing NOW() for valid_from: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "'infinity'") {
		t.Errorf("SQL missing 'infinity' for valid_to: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateInsertReturning(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "INSERT INTO user (email) VALUES ('alice@example.com') RETURNING *"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "RETURNING *") {
		t.Errorf("SQL missing RETURNING *: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateInsertReturningFields(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "INSERT INTO user (email) VALUES ('alice@example.com') RETURNING id, email"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "RETURNING id, email") {
		t.Errorf("SQL missing RETURNING id, email: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateUpdateBasic(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "UPDATE user SET status = 'INACTIVE' WHERE email = 'alice@example.com'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "UPDATE users SET") {
		t.Errorf("SQL missing UPDATE users SET: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "status =") {
		t.Errorf("SQL missing SET clause: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "WHERE") {
		t.Errorf("SQL missing WHERE: %s", result.SQL)
	}

	// Should have 2 params: status value and email value
	if len(result.Params) != 2 {
		t.Errorf("Expected 2 params, got %d: %v", len(result.Params), result.Params)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateUpdateWithID(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "UPDATE user:alice SET status = 'INACTIVE'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "external_id =") {
		t.Errorf("SQL missing external_id filter: %s", result.SQL)
	}

	// Should have 2 params: status value and external_id
	if len(result.Params) != 2 {
		t.Errorf("Expected 2 params, got %d: %v", len(result.Params), result.Params)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateUpdateMultipleFields(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "UPDATE user:alice SET status = 'INACTIVE', email = 'new@example.com'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// Should have multiple SET assignments
	if !strings.Contains(result.SQL, "status =") {
		t.Errorf("SQL missing status SET: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "email =") {
		t.Errorf("SQL missing email SET: %s", result.SQL)
	}

	// Should have 3 params: 2 set values + external_id
	if len(result.Params) != 3 {
		t.Errorf("Expected 3 params, got %d: %v", len(result.Params), result.Params)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateUpdateTemporal(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "UPDATE user:alice SET status = 'INACTIVE'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// Temporal update should have two statements
	if !strings.Contains(result.SQL, "UPDATE") {
		t.Errorf("SQL missing UPDATE: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "INSERT INTO") {
		t.Errorf("SQL missing INSERT INTO (new version): %s", result.SQL)
	}

	// Should set valid_to on old record
	if !strings.Contains(result.SQL, "valid_to = NOW()") {
		t.Errorf("SQL missing valid_to = NOW(): %s", result.SQL)
	}

	// New version should have version + 1
	if !strings.Contains(result.SQL, "version + 1") {
		t.Errorf("SQL missing version increment: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateUpdateForce(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "UPDATE FORCE user:alice SET status = 'INACTIVE'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// FORCE should do direct update without new version
	if !strings.Contains(result.SQL, "UPDATE users SET") {
		t.Errorf("SQL missing UPDATE users SET: %s", result.SQL)
	}

	// Should NOT have INSERT (no new version)
	if strings.Contains(result.SQL, "INSERT INTO") {
		t.Errorf("FORCE update should not create new version: %s", result.SQL)
	}

	// Should filter on valid_to = infinity
	if !strings.Contains(result.SQL, "valid_to = 'infinity'") {
		t.Errorf("SQL missing valid_to filter: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateUpdateReturning(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "UPDATE user:alice SET status = 'INACTIVE' RETURNING *"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "RETURNING *") {
		t.Errorf("SQL missing RETURNING *: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateDeleteBasic(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "DELETE FROM user WHERE status = 'DELETED'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "DELETE FROM users") {
		t.Errorf("SQL missing DELETE FROM users: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "WHERE") {
		t.Errorf("SQL missing WHERE: %s", result.SQL)
	}

	if len(result.Params) != 1 {
		t.Errorf("Expected 1 param, got %d: %v", len(result.Params), result.Params)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateDeleteWithID(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "DELETE FROM user:alice"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "DELETE FROM users") {
		t.Errorf("SQL missing DELETE FROM users: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "external_id =") {
		t.Errorf("SQL missing external_id filter: %s", result.SQL)
	}

	if len(result.Params) != 1 {
		t.Errorf("Expected 1 param (alice), got %d: %v", len(result.Params), result.Params)
	}
	if result.Params[0] != "alice" {
		t.Errorf("Params[0] = %v, want alice", result.Params[0])
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateDeleteTemporal(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "DELETE FROM user:alice"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// Temporal delete should be soft delete (UPDATE, not DELETE)
	if !strings.Contains(result.SQL, "UPDATE users SET valid_to = NOW()") {
		t.Errorf("SQL missing soft delete (UPDATE...valid_to = NOW()): %s", result.SQL)
	}

	// Should NOT be a hard DELETE
	if strings.Contains(result.SQL, "DELETE FROM") {
		t.Errorf("Temporal delete should not be hard DELETE: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateDeleteForce(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "DELETE FORCE FROM user:alice"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// FORCE should do hard delete
	if !strings.Contains(result.SQL, "DELETE FROM users") {
		t.Errorf("FORCE delete should be hard DELETE: %s", result.SQL)
	}

	// Should NOT be an UPDATE (soft delete)
	if strings.Contains(result.SQL, "UPDATE") && strings.Contains(result.SQL, "valid_to =") {
		t.Errorf("FORCE delete should not be soft delete: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateDeleteReturning(t *testing.T) {
	schema := testSchemaNoTemporal()
	gen := NewGenerator(schema)

	input := "DELETE FROM user:alice RETURNING *"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "RETURNING *") {
		t.Errorf("SQL missing RETURNING *: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateInsertRelationship(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "INSERT INTO user:alice->member_of->group"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// Should insert into junction table
	if !strings.Contains(result.SQL, "INSERT INTO user_groups") {
		t.Errorf("SQL missing INSERT INTO user_groups: %s", result.SQL)
	}

	// Should select IDs from source and target
	if !strings.Contains(result.SQL, "SELECT s.id, t.id FROM") {
		t.Errorf("SQL missing SELECT for IDs: %s", result.SQL)
	}

	// Should filter by source external_id
	if !strings.Contains(result.SQL, "s.external_id =") {
		t.Errorf("SQL missing source external_id filter: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateDeleteRelationship(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	input := "DELETE FROM user:alice->member_of->group"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	// Should delete from junction table
	if !strings.Contains(result.SQL, "DELETE FROM user_groups") {
		t.Errorf("SQL missing DELETE FROM user_groups: %s", result.SQL)
	}

	// Should use subquery for source ID
	if !strings.Contains(result.SQL, "SELECT id FROM users") {
		t.Errorf("SQL missing subquery for source ID: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateStatementSelect(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Test that GenerateStatement also works for SELECT
	input := "SELECT * FROM user:alice"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	result, err := gen.GenerateStatement(stmt)
	if err != nil {
		t.Fatalf("GenerateStatement() error = %v", err)
	}

	if !strings.Contains(result.SQL, "SELECT t0.* FROM users") {
		t.Errorf("SQL missing SELECT: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}
