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
	// Group hierarchy - child_of means this group is a child of the target group
	rel := s.AddRelationship("child_of", "group", "group", "group_hierarchy", "child_group_id", "parent_group_id")
	rel.WithTemporal()

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

// Tests for FROM clause path traversal syntax (e.g., SELECT fields FROM entity:id->rel->target)
func TestGenerateFromClausePathTraversal(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Single hop traversal with field selection from target
	input := "SELECT name, description FROM user:okta_123->member_of->group"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should select from target entity (group)
	if !strings.Contains(result.SQL, "t1.name") {
		t.Errorf("SQL should select name from target entity: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "t1.description") {
		t.Errorf("SQL should select description from target entity: %s", result.SQL)
	}

	// Should have JOINs to traverse relationship
	if !strings.Contains(result.SQL, "JOIN user_groups") {
		t.Errorf("SQL missing junction table JOIN: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "JOIN groups") {
		t.Errorf("SQL missing target table JOIN: %s", result.SQL)
	}

	// Should start from users table
	if !strings.Contains(result.SQL, "FROM users t0") {
		t.Errorf("SQL should start FROM users: %s", result.SQL)
	}

	// Should filter by external_id
	if !strings.Contains(result.SQL, "external_id = $1") {
		t.Errorf("SQL missing external_id filter: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateFromClausePathTraversalAllFields(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Select all fields from target entity
	input := "SELECT * FROM user:okta_123->member_of->group"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should select all from final target
	if !strings.Contains(result.SQL, "SELECT t1.*") {
		t.Errorf("SQL should SELECT t1.* for all fields: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateFromClauseMultiHopPath(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Multi-hop: user -> group -> app
	input := "SELECT name, app_type FROM user:okta_123->member_of->group->has_access->app"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have all required JOINs
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

	// Final select should be from apps table (t2 after 2 hops)
	if !strings.Contains(result.SQL, "t2.name") || !strings.Contains(result.SQL, "t2.app_type") {
		t.Errorf("SQL should select from final target entity: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateFromClauseReverseTraversal(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Reverse traversal: group <- user (who belongs to this group)
	input := "SELECT email, status FROM group:grp_123<-member_of<-user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should start from groups
	if !strings.Contains(result.SQL, "FROM groups t0") {
		t.Errorf("SQL should start FROM groups: %s", result.SQL)
	}

	// Should select from users (target of reverse traversal)
	if !strings.Contains(result.SQL, "t1.email") {
		t.Errorf("SQL should select email from users: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateFromClausePathWithQuotedID(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// IDs starting with numbers need quotes
	input := "SELECT name FROM user:'00u24m2v4jOPwc106697'->member_of->group"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Parameter should have the ID without quotes
	if len(result.Params) == 0 {
		t.Fatal("expected at least one parameter")
	}
	if result.Params[0] != "00u24m2v4jOPwc106697" {
		t.Errorf("Params[0] = %q, want '00u24m2v4jOPwc106697'", result.Params[0])
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateFromClausePathWithWhere(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Path with WHERE clause filtering on target entity
	input := "SELECT name FROM user:okta_123->member_of->group WHERE name LIKE 'Admin%'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have WHERE clause for both external_id and name filter
	if !strings.Contains(result.SQL, "external_id = $1") {
		t.Errorf("SQL missing external_id filter: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "LIKE") {
		t.Errorf("SQL missing LIKE filter: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateRecursiveTraversalSimple(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Simple recursive traversal: find all ancestor groups of a group
	input := "SELECT ->child_of{1,3}->group FROM group:engineering"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have RECURSIVE CTE
	if !strings.Contains(result.SQL, "WITH RECURSIVE path_cte") {
		t.Errorf("SQL missing RECURSIVE CTE: %s", result.SQL)
	}

	// Should have base case and recursive case with UNION ALL
	if !strings.Contains(result.SQL, "UNION ALL") {
		t.Errorf("SQL missing UNION ALL: %s", result.SQL)
	}

	// Should have depth tracking
	if !strings.Contains(result.SQL, "depth") {
		t.Errorf("SQL missing depth tracking: %s", result.SQL)
	}

	// Should have depth constraint (< 3 for max 3 hops)
	if !strings.Contains(result.SQL, "p.depth < 3") {
		t.Errorf("SQL missing depth constraint: %s", result.SQL)
	}

	// Should have minimum depth filter (>= 1)
	if !strings.Contains(result.SQL, "p.depth >= 1") {
		t.Errorf("SQL missing min depth filter: %s", result.SQL)
	}

	// Should have temporal filter
	if !strings.Contains(result.SQL, "valid_to = 'infinity'") {
		t.Errorf("SQL missing temporal filter: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateRecursiveTraversalMixedPath(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Mixed path: first traverse user->member_of->group, then recursive group->child_of->group
	// "Find all ancestor groups of groups that user alice is a member of"
	input := "SELECT ->member_of->group->child_of{1,6}->group FROM user:alice"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have RECURSIVE CTE
	if !strings.Contains(result.SQL, "WITH RECURSIVE path_cte") {
		t.Errorf("SQL missing RECURSIVE CTE: %s", result.SQL)
	}

	// Should join through user_groups (prefix path)
	if !strings.Contains(result.SQL, "user_groups") {
		t.Errorf("SQL missing user_groups join: %s", result.SQL)
	}

	// Should join through group_hierarchy (recursive relationship)
	if !strings.Contains(result.SQL, "group_hierarchy") {
		t.Errorf("SQL missing group_hierarchy join: %s", result.SQL)
	}

	// Should have the user's external_id parameter
	if len(result.Params) < 1 {
		t.Errorf("Expected at least 1 param, got %d", len(result.Params))
	}
	if result.Params[0] != "alice" {
		t.Errorf("Params[0] = %v, want alice", result.Params[0])
	}

	// Should have depth constraint (< 6 for max 6 hops)
	if !strings.Contains(result.SQL, "p.depth < 6") {
		t.Errorf("SQL missing depth constraint: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
	t.Logf("Params: %v", result.Params)
}

func TestGenerateRecursiveTraversalExactDepth(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Exact depth: {3} means exactly 3 hops
	input := "SELECT ->child_of{3}->group FROM group:engineering"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have RECURSIVE CTE
	if !strings.Contains(result.SQL, "WITH RECURSIVE path_cte") {
		t.Errorf("SQL missing RECURSIVE CTE: %s", result.SQL)
	}

	// For exact depth {3}, should have depth < 3 (max) and depth >= 3 (min)
	if !strings.Contains(result.SQL, "p.depth < 3") {
		t.Errorf("SQL missing max depth constraint: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "p.depth >= 3") {
		t.Errorf("SQL missing min depth constraint: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateWildcardSingleHop(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Wildcard to specific entity type: find all relationships from user that lead to group
	input := "SELECT ->*->group FROM user:alice"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should include the member_of relationship (user -> group)
	if !strings.Contains(result.SQL, "user_groups") {
		t.Errorf("SQL should join through user_groups for member_of: %s", result.SQL)
	}

	// Should include _relationship column showing which relationship was used
	if !strings.Contains(result.SQL, "'member_of' AS _relationship") {
		t.Errorf("SQL should include relationship name: %s", result.SQL)
	}

	// Should have parameter for external_id
	if len(result.Params) != 1 || result.Params[0] != "alice" {
		t.Errorf("Expected params [alice], got %v", result.Params)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateWildcardMultipleRelationships(t *testing.T) {
	// Create schema with multiple relationships from user
	schema := NewSchema()
	schema.AddEntity("user", "users", "id").WithTemporal()
	schema.AddEntity("group", "groups", "id").WithTemporal()
	schema.AddEntity("app", "apps", "id").WithTemporal()

	schema.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")
	schema.AddRelationship("assigned_to", "user", "app", "user_apps", "user_id", "app_id")

	gen := NewGenerator(schema)

	// Wildcard to any entity: find all outgoing relationships from user
	input := "SELECT ->*->* FROM user:alice"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should include both relationships as UNION
	if !strings.Contains(result.SQL, "UNION ALL") {
		t.Errorf("SQL should use UNION ALL for multiple relationships: %s", result.SQL)
	}

	// Should include both junction tables
	if !strings.Contains(result.SQL, "user_groups") {
		t.Errorf("SQL should include user_groups: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "user_apps") {
		t.Errorf("SQL should include user_apps: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateWildcardRecursive(t *testing.T) {
	schema := testSchema()
	gen := NewGenerator(schema)

	// Recursive wildcard: explore any path up to 3 hops ending at user
	input := "SELECT ->*{1,3}->user FROM user:alice"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have RECURSIVE CTE
	if !strings.Contains(result.SQL, "WITH RECURSIVE path_cte") {
		t.Errorf("SQL missing RECURSIVE CTE: %s", result.SQL)
	}

	// Should track entity_type in CTE for heterogeneous traversal
	if !strings.Contains(result.SQL, "entity_type") {
		t.Errorf("SQL should track entity_type: %s", result.SQL)
	}

	// Should have depth constraints
	if !strings.Contains(result.SQL, "p.depth < 3") {
		t.Errorf("SQL missing max depth constraint: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "p.depth >= 1") {
		t.Errorf("SQL missing min depth constraint: %s", result.SQL)
	}

	// Final query should filter by target entity type
	if !strings.Contains(result.SQL, "p.entity_type = 'user'") {
		t.Errorf("SQL should filter by target entity type: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func testPolicySchema() *Schema {
	s := NewSchema()

	s.AddEntity("user", "users", "id").WithTemporal()
	s.AddEntity("group", "groups", "id").WithTemporal()
	s.AddEntity("app", "apps", "id").WithTemporal()
	s.AddEntity("policy", "policies", "id").WithTemporal()
	s.AddEntity("rule", "rules", "id").WithTemporal()

	s.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id").WithTemporal()
	s.AddRelationship("has_access", "group", "app", "group_apps", "group_id", "app_id").WithTemporal()
	s.AddRelationship("governed_by", "app", "policy", "app_policies", "app_id", "policy_id").WithTemporal()
	s.AddRelationship("has_rule", "policy", "rule", "policy_rules", "policy_id", "rule_id").WithTemporal()
	s.AddRelationship("applies_to", "rule", "group", "rule_groups", "rule_id", "group_id").WithTemporal()

	return s
}

func TestGenerateCrossPathJoin(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	// Query: Find users, their groups, and the policy rules that apply to those groups
	input := `SELECT u.email, g.name, r.name
		FROM user AS u ->member_of->group AS g
		JOIN app:slack ->governed_by->policy ->has_rule->rule AS r ->applies_to->group AS rg
		ON g.id = rg.id`

	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have JOINs for the main path
	if !strings.Contains(result.SQL, "user_groups") {
		t.Errorf("SQL missing user_groups join: %s", result.SQL)
	}

	// Should have JOINs for the policy path
	if !strings.Contains(result.SQL, "app_policies") {
		t.Errorf("SQL missing app_policies join: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "policy_rules") {
		t.Errorf("SQL missing policy_rules join: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "rule_groups") {
		t.Errorf("SQL missing rule_groups join: %s", result.SQL)
	}

	// Should have the ON condition joining the two paths
	// The ON condition g.id = rg.id should translate to alias comparisons
	if !strings.Contains(result.SQL, ".id =") || !strings.Contains(result.SQL, ".id") {
		t.Errorf("SQL missing ON condition: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateCrossPathJoinEffectiveRules(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	// Full query: Users -> Groups -> Apps with Policy -> Rules that apply to those groups
	input := `SELECT u.email, g.name AS access_group, p.name AS policy_name, r.name AS rule_name
		FROM user AS u ->member_of->group AS g ->has_access->app AS a
		JOIN app AS app_join ->governed_by->policy AS p ->has_rule->rule AS r ->applies_to->group AS rg
		ON g.id = rg.id`

	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := gen.Generate(q)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have all the necessary JOINs
	if !strings.Contains(result.SQL, "users") {
		t.Errorf("SQL missing users table: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "groups") {
		t.Errorf("SQL missing groups table: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "apps") {
		t.Errorf("SQL missing apps table: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "policies") {
		t.Errorf("SQL missing policies table: %s", result.SQL)
	}
	if !strings.Contains(result.SQL, "rules") {
		t.Errorf("SQL missing rules table: %s", result.SQL)
	}

	// Should have temporal filters
	if !strings.Contains(result.SQL, "valid_to = 'infinity'") {
		t.Errorf("SQL missing temporal filter: %s", result.SQL)
	}

	t.Logf("Generated SQL: %s", result.SQL)
}

func TestGenerateExplainAccess(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	stmt := &ExplainAccessStmt{
		FromEntity: "user",
		FromID:     "alice",
		ToEntity:   "app",
		ToID:       "slack",
	}

	queries, err := gen.GenerateAccessExplanation(stmt)
	if err != nil {
		t.Fatalf("GenerateAccessExplanation() error = %v", err)
	}

	// Subject query should exist and query users table
	if queries.SubjectQuery == nil {
		t.Error("SubjectQuery should not be nil")
	} else {
		if !strings.Contains(queries.SubjectQuery.SQL, "users") {
			t.Errorf("SubjectQuery should query users table: %s", queries.SubjectQuery.SQL)
		}
		if !strings.Contains(queries.SubjectQuery.SQL, "external_id = $1") {
			t.Errorf("SubjectQuery should filter by external_id: %s", queries.SubjectQuery.SQL)
		}
		if queries.SubjectQuery.Params[0] != "alice" {
			t.Errorf("SubjectQuery param should be alice, got %v", queries.SubjectQuery.Params[0])
		}
		t.Logf("Subject Query: %s", queries.SubjectQuery.SQL)
	}

	// Target query should exist and query apps table
	if queries.TargetQuery == nil {
		t.Error("TargetQuery should not be nil")
	} else {
		if !strings.Contains(queries.TargetQuery.SQL, "apps") {
			t.Errorf("TargetQuery should query apps table: %s", queries.TargetQuery.SQL)
		}
		if queries.TargetQuery.Params[0] != "slack" {
			t.Errorf("TargetQuery param should be slack, got %v", queries.TargetQuery.Params[0])
		}
		t.Logf("Target Query: %s", queries.TargetQuery.SQL)
	}

	// Group access query should join user -> group -> app
	if queries.GroupAccessQuery == nil {
		t.Log("GroupAccessQuery is nil (expected since has_access is on group->app)")
	} else {
		if !strings.Contains(queries.GroupAccessQuery.SQL, "user_groups") {
			t.Errorf("GroupAccessQuery should include user_groups: %s", queries.GroupAccessQuery.SQL)
		}
		if !strings.Contains(queries.GroupAccessQuery.SQL, "group_apps") {
			t.Errorf("GroupAccessQuery should include group_apps: %s", queries.GroupAccessQuery.SQL)
		}
		t.Logf("Group Access Query: %s", queries.GroupAccessQuery.SQL)
	}

	// Policy query should find app -> policy
	if queries.PolicyQuery == nil {
		t.Log("PolicyQuery is nil (expected if no direct policy)")
	} else {
		if !strings.Contains(queries.PolicyQuery.SQL, "app_policies") {
			t.Errorf("PolicyQuery should include app_policies: %s", queries.PolicyQuery.SQL)
		}
		if !strings.Contains(queries.PolicyQuery.SQL, "policies") {
			t.Errorf("PolicyQuery should include policies: %s", queries.PolicyQuery.SQL)
		}
		t.Logf("Policy Query: %s", queries.PolicyQuery.SQL)
	}

	// Rules query should find app -> policy -> rules
	if queries.RulesQuery == nil {
		t.Log("RulesQuery is nil (expected if no rules defined)")
	} else {
		if !strings.Contains(queries.RulesQuery.SQL, "policy_rules") {
			t.Errorf("RulesQuery should include policy_rules: %s", queries.RulesQuery.SQL)
		}
		if !strings.Contains(queries.RulesQuery.SQL, "rules") {
			t.Errorf("RulesQuery should include rules: %s", queries.RulesQuery.SQL)
		}
		t.Logf("Rules Query: %s", queries.RulesQuery.SQL)
	}

	// Effective rules query should find rules that apply to user's groups
	if queries.EffectiveRulesQuery == nil {
		t.Log("EffectiveRulesQuery is nil (expected if schema doesn't support)")
	} else {
		// Should join user -> groups
		if !strings.Contains(queries.EffectiveRulesQuery.SQL, "user_groups") {
			t.Errorf("EffectiveRulesQuery should include user_groups: %s", queries.EffectiveRulesQuery.SQL)
		}
		// Should join rule -> target groups
		if !strings.Contains(queries.EffectiveRulesQuery.SQL, "rule_groups") {
			t.Errorf("EffectiveRulesQuery should include rule_groups: %s", queries.EffectiveRulesQuery.SQL)
		}
		t.Logf("Effective Rules Query: %s", queries.EffectiveRulesQuery.SQL)
	}
}

func TestGenerateExplainAccessSQL(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	stmt := &ExplainAccessStmt{
		FromEntity: "user",
		FromID:     "alice",
		ToEntity:   "app",
		ToID:       "slack",
	}

	sql, err := gen.GenerateAccessExplanationSQL(stmt)
	if err != nil {
		t.Fatalf("GenerateAccessExplanationSQL() error = %v", err)
	}

	// Should have section headers
	if !strings.Contains(sql, "-- Subject Query:") {
		t.Error("SQL should contain Subject Query section")
	}
	if !strings.Contains(sql, "-- Target Query:") {
		t.Error("SQL should contain Target Query section")
	}

	// Should contain actual SQL
	if !strings.Contains(sql, "SELECT") {
		t.Error("SQL should contain SELECT statements")
	}
	if !strings.Contains(sql, "FROM users") {
		t.Error("SQL should contain FROM users")
	}
	if !strings.Contains(sql, "FROM apps") {
		t.Error("SQL should contain FROM apps")
	}

	t.Logf("Generated SQL:\n%s", sql)
}

func TestGenerateExplainAccessUnknownEntity(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	stmt := &ExplainAccessStmt{
		FromEntity: "unknown_entity",
		FromID:     "alice",
		ToEntity:   "app",
		ToID:       "slack",
	}

	_, err := gen.GenerateAccessExplanation(stmt)
	if err == nil {
		t.Error("GenerateAccessExplanation() should return error for unknown entity")
	}
}

func TestParseAndGenerateExplainAccess(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	input := "EXPLAIN ACCESS user:alice TO app:slack"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.ExplainAccess == nil {
		t.Fatal("Expected ExplainAccess statement")
	}

	queries, err := gen.GenerateAccessExplanation(stmt.ExplainAccess)
	if err != nil {
		t.Fatalf("GenerateAccessExplanation() error = %v", err)
	}

	// Verify both queries exist
	if queries.SubjectQuery == nil {
		t.Error("SubjectQuery should not be nil")
	}
	if queries.TargetQuery == nil {
		t.Error("TargetQuery should not be nil")
	}

	t.Logf("Parsed and generated EXPLAIN ACCESS successfully")
}

func TestParseExplainAccessWithVersion(t *testing.T) {
	input := "EXPLAIN ACCESS user:alice TO app:slack VERSION d'2024-01-01T00:00:00Z'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stmt.ExplainAccess == nil {
		t.Fatal("Expected ExplainAccess statement")
	}

	ea := stmt.ExplainAccess
	if ea.FromEntity != "user" {
		t.Errorf("FromEntity = %v, want user", ea.FromEntity)
	}
	if ea.Version == nil {
		t.Fatal("Expected Version clause")
	}
	if ea.Version.Timestamp == nil {
		t.Fatal("Expected Version.Timestamp")
	}
	if *ea.Version.Timestamp != "2024-01-01T00:00:00Z" {
		t.Errorf("Version.Timestamp = %v, want 2024-01-01T00:00:00Z", *ea.Version.Timestamp)
	}
}

func TestParseExplainAccessVersionsAll(t *testing.T) {
	input := "EXPLAIN ACCESS user:alice TO app:slack VERSIONS ALL"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stmt.ExplainAccess == nil {
		t.Fatal("Expected ExplainAccess statement")
	}

	ea := stmt.ExplainAccess
	if ea.Versions == nil {
		t.Fatal("Expected Versions clause")
	}
	if !ea.Versions.All {
		t.Error("Expected Versions.All = true")
	}
}

func TestParseExplainAccessVersionsLast(t *testing.T) {
	input := "EXPLAIN ACCESS user:alice TO app:slack VERSIONS LAST 5"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stmt.ExplainAccess == nil {
		t.Fatal("Expected ExplainAccess statement")
	}

	ea := stmt.ExplainAccess
	if ea.Versions == nil {
		t.Fatal("Expected Versions clause")
	}
	if ea.Versions.Last == nil {
		t.Fatal("Expected Versions.Last")
	}
	if *ea.Versions.Last != 5 {
		t.Errorf("Versions.Last = %v, want 5", *ea.Versions.Last)
	}
}

func TestParseExplainAccessVersionsBetween(t *testing.T) {
	input := "EXPLAIN ACCESS user:alice TO app:slack VERSIONS BETWEEN d'2024-01-01' AND d'2024-06-01'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stmt.ExplainAccess == nil {
		t.Fatal("Expected ExplainAccess statement")
	}

	ea := stmt.ExplainAccess
	if ea.Versions == nil {
		t.Fatal("Expected Versions clause")
	}
	if ea.Versions.Between == nil {
		t.Fatal("Expected Versions.Between")
	}
	if *ea.Versions.Between.From != "2024-01-01" {
		t.Errorf("Versions.Between.From = %v, want 2024-01-01", *ea.Versions.Between.From)
	}
	if *ea.Versions.Between.To != "2024-06-01" {
		t.Errorf("Versions.Between.To = %v, want 2024-06-01", *ea.Versions.Between.To)
	}
}

func TestGenerateExplainAccessTemporalVersion(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	timestamp := "2024-01-01T00:00:00Z"
	stmt := &ExplainAccessStmt{
		FromEntity: "user",
		FromID:     "alice",
		ToEntity:   "app",
		ToID:       "slack",
		Version: &VersionClause{
			Timestamp: &timestamp,
		},
	}

	queries, err := gen.GenerateAccessExplanation(stmt)
	if err != nil {
		t.Fatalf("GenerateAccessExplanation() error = %v", err)
	}

	// Check temporal mode
	if queries.TemporalMode != "point_in_time:2024-01-01T00:00:00Z" {
		t.Errorf("TemporalMode = %v, want point_in_time:2024-01-01T00:00:00Z", queries.TemporalMode)
	}

	// Subject query should have temporal range filter
	if !strings.Contains(queries.SubjectQuery.SQL, "valid_from") {
		t.Errorf("SubjectQuery should contain valid_from: %s", queries.SubjectQuery.SQL)
	}
	if !strings.Contains(queries.SubjectQuery.SQL, "valid_to") {
		t.Errorf("SubjectQuery should contain valid_to: %s", queries.SubjectQuery.SQL)
	}
	// Should NOT have 'infinity' for point-in-time query
	if strings.Contains(queries.SubjectQuery.SQL, "'infinity'") {
		t.Errorf("SubjectQuery should not contain 'infinity' for point-in-time: %s", queries.SubjectQuery.SQL)
	}

	t.Logf("Subject Query: %s", queries.SubjectQuery.SQL)
	t.Logf("Params: %v", queries.SubjectQuery.Params)
}

func TestGenerateExplainAccessVersionsAll(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	stmt := &ExplainAccessStmt{
		FromEntity: "user",
		FromID:     "alice",
		ToEntity:   "app",
		ToID:       "slack",
		Versions: &VersionsClause{
			All: true,
		},
	}

	queries, err := gen.GenerateAccessExplanation(stmt)
	if err != nil {
		t.Fatalf("GenerateAccessExplanation() error = %v", err)
	}

	// Check temporal mode
	if queries.TemporalMode != "all_versions" {
		t.Errorf("TemporalMode = %v, want all_versions", queries.TemporalMode)
	}

	// Subject query should NOT have valid_to = 'infinity' filter
	if strings.Contains(queries.SubjectQuery.SQL, "'infinity'") {
		t.Errorf("SubjectQuery should not contain 'infinity' for VERSIONS ALL: %s", queries.SubjectQuery.SQL)
	}

	t.Logf("Subject Query: %s", queries.SubjectQuery.SQL)
}

func TestGenerateExplainAccessVersionsLast(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	last := 5
	stmt := &ExplainAccessStmt{
		FromEntity: "user",
		FromID:     "alice",
		ToEntity:   "app",
		ToID:       "slack",
		Versions: &VersionsClause{
			Last: &last,
		},
	}

	queries, err := gen.GenerateAccessExplanation(stmt)
	if err != nil {
		t.Fatalf("GenerateAccessExplanation() error = %v", err)
	}

	// Check temporal mode
	if queries.TemporalMode != "last_5_versions" {
		t.Errorf("TemporalMode = %v, want last_5_versions", queries.TemporalMode)
	}

	// Subject query should have ORDER BY and LIMIT
	if !strings.Contains(queries.SubjectQuery.SQL, "ORDER BY") {
		t.Errorf("SubjectQuery should contain ORDER BY: %s", queries.SubjectQuery.SQL)
	}
	if !strings.Contains(queries.SubjectQuery.SQL, "LIMIT 5") {
		t.Errorf("SubjectQuery should contain LIMIT 5: %s", queries.SubjectQuery.SQL)
	}

	t.Logf("Subject Query: %s", queries.SubjectQuery.SQL)
}

func TestGenerateExplainAccessVersionsBetween(t *testing.T) {
	schema := testPolicySchema()
	gen := NewGenerator(schema)

	from := "2024-01-01"
	to := "2024-06-01"
	stmt := &ExplainAccessStmt{
		FromEntity: "user",
		FromID:     "alice",
		ToEntity:   "app",
		ToID:       "slack",
		Versions: &VersionsClause{
			Between: &VersionRange{
				From: &from,
				To:   &to,
			},
		},
	}

	queries, err := gen.GenerateAccessExplanation(stmt)
	if err != nil {
		t.Fatalf("GenerateAccessExplanation() error = %v", err)
	}

	// Check temporal mode
	expectedMode := "between:2024-01-01:2024-06-01"
	if queries.TemporalMode != expectedMode {
		t.Errorf("TemporalMode = %v, want %v", queries.TemporalMode, expectedMode)
	}

	// Subject query should have range filters
	if !strings.Contains(queries.SubjectQuery.SQL, "valid_from >=") {
		t.Errorf("SubjectQuery should contain valid_from >=: %s", queries.SubjectQuery.SQL)
	}
	if !strings.Contains(queries.SubjectQuery.SQL, "valid_from <=") {
		t.Errorf("SubjectQuery should contain valid_from <=: %s", queries.SubjectQuery.SQL)
	}

	t.Logf("Subject Query: %s", queries.SubjectQuery.SQL)
	t.Logf("Params: %v", queries.SubjectQuery.Params)
}
