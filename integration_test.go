//go:build integration

package doodle

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func getTestDB(t *testing.T) *sql.DB {
	connStr := os.Getenv("DOODLE_TEST_DB")
	if connStr == "" {
		connStr = "postgres://doodle:doodle@localhost:5439/doodle_test?sslmode=disable"
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// Wait for connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	return db
}

func integrationSchema() *Schema {
	s := NewSchema()

	s.AddEntity("user", "users", "id").
		WithTemporal().
		AddField("external_id", "external_id").
		AddField("email", "email").
		AddField("first_name", "first_name").
		AddField("last_name", "last_name").
		AddField("status", "status").
		AddField("provider", "provider")

	s.AddEntity("group", "groups", "id").
		WithTemporal().
		AddField("external_id", "external_id").
		AddField("name", "name").
		AddField("description", "description").
		AddField("provider", "provider")

	s.AddEntity("app", "apps", "id").
		WithTemporal().
		AddField("external_id", "external_id").
		AddField("name", "name").
		AddField("app_type", "app_type").
		AddField("provider", "provider")

	s.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id").
		WithTemporal()
	s.AddRelationship("has_access", "group", "app", "group_apps", "group_id", "app_id").
		WithTemporal()

	return s
}

func TestIntegrationSimpleQuery(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()
	rows, err := db.Query(ctx, "SELECT * FROM user:okta_user_001")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}

func TestIntegrationSingleHop(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Alice (okta_user_001) is in Administrators and Developers groups
	rows, err := db.Query(ctx, "SELECT ->member_of->group FROM user:okta_user_001")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 groups for Alice, got %d", count)
	}
}

func TestIntegrationMultiHop(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Alice -> groups -> apps (should have access to all 3 apps through admin group)
	rows, err := db.Query(ctx, "SELECT ->member_of->group->has_access->app FROM user:okta_user_001")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	apps := make(map[string]bool)
	for rows.Next() {
		var id, externalID, name, appType, provider string
		var rawData interface{}
		var version int
		var validFrom, validTo interface{}
		err := rows.Scan(&id, &externalID, &name, &appType, &provider, &rawData, &version, &validFrom, &validTo)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		apps[name] = true
	}

	// Alice should have access to all apps via Administrators + GitHub/Slack via Developers
	expectedApps := []string{"Slack", "GitHub", "Jira"}
	for _, app := range expectedApps {
		if !apps[app] {
			t.Errorf("Expected Alice to have access to %s", app)
		}
	}

	t.Logf("Alice has access to %d apps: %v", len(apps), apps)
}

func TestIntegrationWithWhere(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Get user with specific status
	compiled, err := db.Compile("SELECT * FROM user:okta_user_003 WHERE status = 'SUSPENDED'")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)
	t.Logf("Params: %v", compiled.Params)

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 suspended user, got %d", count)
	}
}

func TestIntegrationWithLimit(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Alice has access to multiple apps, limit to 1
	rows, err := db.Query(ctx, "SELECT ->member_of->group->has_access->app FROM user:okta_user_001 LIMIT 1")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 row with LIMIT 1, got %d", count)
	}
}

func TestIntegrationCompileOnly(t *testing.T) {
	db := New(integrationSchema())

	testCases := []struct {
		name  string
		query string
	}{
		{
			name:  "simple select",
			query: "SELECT * FROM user:abc123",
		},
		{
			name:  "single hop",
			query: "SELECT ->member_of->group FROM user:abc123",
		},
		{
			name:  "multi hop",
			query: "SELECT ->member_of->group->has_access->app FROM user:abc123",
		},
		{
			name:  "with version",
			query: "SELECT * FROM user:abc123 VERSION d'2024-01-01T00:00:00Z'",
		},
		{
			name:  "with where",
			query: "SELECT * FROM user:abc123 WHERE status = 'ACTIVE'",
		},
		{
			name:  "complex",
			query: "SELECT ->member_of->group->has_access->app FROM user:abc123 WHERE status = 'ACTIVE' LIMIT 10",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := db.Compile(tc.query)
			if err != nil {
				t.Errorf("Compile() error = %v", err)
				return
			}

			t.Logf("Query: %s", tc.query)
			t.Logf("SQL: %s", result.SQL)
			t.Logf("Params: %v", result.Params)
		})
	}
}

func TestIntegrationBobAccess(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Bob (okta_user_002) is only in Developers, should have Slack and GitHub access
	rows, err := db.Query(ctx, "SELECT ->member_of->group->has_access->app FROM user:okta_user_002")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	apps := make(map[string]bool)
	for rows.Next() {
		var id, externalID, name, appType, provider string
		var rawData interface{}
		var version int
		var validFrom, validTo interface{}
		err := rows.Scan(&id, &externalID, &name, &appType, &provider, &rawData, &version, &validFrom, &validTo)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		apps[name] = true
	}

	// Bob should have access to Slack and GitHub only
	if !apps["Slack"] {
		t.Error("Expected Bob to have access to Slack")
	}
	if !apps["GitHub"] {
		t.Error("Expected Bob to have access to GitHub")
	}
	if apps["Jira"] {
		t.Error("Expected Bob to NOT have access to Jira")
	}

	t.Logf("Bob has access to %d apps: %v", len(apps), apps)
}

func TestIntegrationCharlieAccess(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Charlie (okta_user_003) is only in Users group, should have Slack access only
	rows, err := db.Query(ctx, "SELECT ->member_of->group->has_access->app FROM user:okta_user_003")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	apps := make(map[string]bool)
	for rows.Next() {
		var id, externalID, name, appType, provider string
		var rawData interface{}
		var version int
		var validFrom, validTo interface{}
		err := rows.Scan(&id, &externalID, &name, &appType, &provider, &rawData, &version, &validFrom, &validTo)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		apps[name] = true
	}

	if len(apps) != 1 || !apps["Slack"] {
		t.Errorf("Expected Charlie to have access only to Slack, got: %v", apps)
	}

	t.Logf("Charlie has access to %d apps: %v", len(apps), apps)
}

func TestIntegrationIncomingEdge(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Get all users who are members of the Administrators group
	// group <- member_of <- user
	rows, err := db.Query(ctx, "SELECT <-member_of<-user FROM group:okta_group_admins")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	users := make(map[string]bool)
	for rows.Next() {
		var id, externalID, email, firstName, lastName, status, provider string
		var rawData interface{}
		var version int
		var validFrom, validTo interface{}
		err := rows.Scan(&id, &externalID, &email, &firstName, &lastName, &status, &provider, &rawData, &version, &validFrom, &validTo)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		users[email] = true
	}

	// Only Alice is in the Administrators group
	if len(users) != 1 || !users["alice@example.com"] {
		t.Errorf("Expected only Alice in Administrators, got: %v", users)
	}

	t.Logf("Users in Administrators group: %v", users)
}

func TestIntegrationIncomingEdgeMultiHop(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Get all users who have access to Jira (only admins have Jira access)
	// app <- has_access <- group <- member_of <- user
	rows, err := db.Query(ctx, "SELECT <-has_access<-group<-member_of<-user FROM app:okta_app_jira")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	users := make(map[string]bool)
	for rows.Next() {
		var id, externalID, email, firstName, lastName, status, provider string
		var rawData interface{}
		var version int
		var validFrom, validTo interface{}
		err := rows.Scan(&id, &externalID, &email, &firstName, &lastName, &status, &provider, &rawData, &version, &validFrom, &validTo)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		users[email] = true
	}

	// Only Alice has access to Jira (via Administrators group)
	if len(users) != 1 || !users["alice@example.com"] {
		t.Errorf("Expected only Alice to have Jira access, got: %v", users)
	}

	t.Logf("Users with Jira access: %v", users)
}

func TestIntegrationOrderBy(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Get all users ordered by email descending
	// We query from a specific user but with ORDER BY
	compiled, err := db.Compile("SELECT * FROM user:okta_user_001 ORDER BY email DESC")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	// Verify ORDER BY is in the SQL
	if !strings.Contains(compiled.SQL, "ORDER BY") {
		t.Errorf("SQL missing ORDER BY: %s", compiled.SQL)
	}
	if !strings.Contains(compiled.SQL, "DESC") {
		t.Errorf("SQL missing DESC: %s", compiled.SQL)
	}

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}

func TestIntegrationOrderByMultiple(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	compiled, err := db.Compile("SELECT * FROM user:okta_user_001 ORDER BY status ASC, email DESC")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	if !strings.Contains(compiled.SQL, "ORDER BY") {
		t.Errorf("SQL missing ORDER BY: %s", compiled.SQL)
	}
	if !strings.Contains(compiled.SQL, "ASC") {
		t.Errorf("SQL missing ASC: %s", compiled.SQL)
	}
	if !strings.Contains(compiled.SQL, "DESC") {
		t.Errorf("SQL missing DESC: %s", compiled.SQL)
	}
}

func TestIntegrationFieldSelection(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	compiled, err := db.Compile("SELECT email, status FROM user:okta_user_001")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var email, status string
		err := rows.Scan(&email, &status)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		if email != "alice@example.com" {
			t.Errorf("Expected alice@example.com, got %s", email)
		}
		if status != "ACTIVE" {
			t.Errorf("Expected ACTIVE, got %s", status)
		}
		t.Logf("Got email=%s, status=%s", email, status)
	}
}

func TestIntegrationCountStar(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	compiled, err := db.Compile("SELECT COUNT(*) FROM user:okta_user_001")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var count int
		err := rows.Scan(&count)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected count 1, got %d", count)
		}
		t.Logf("COUNT(*) = %d", count)
	}
}

func TestIntegrationCountPath(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Count how many groups Alice is in (should be 2: Administrators and Developers)
	compiled, err := db.Compile("SELECT COUNT(->member_of->group) FROM user:okta_user_001")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var count int
		err := rows.Scan(&count)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		if count != 2 {
			t.Errorf("Expected Alice in 2 groups, got %d", count)
		}
		t.Logf("Alice is in %d groups", count)
	}
}

func TestIntegrationCountApps(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Count how many apps Alice has access to through groups
	compiled, err := db.Compile("SELECT COUNT(->member_of->group->has_access->app) FROM user:okta_user_001")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var count int
		err := rows.Scan(&count)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		// Alice has access via Administrators (all 3 apps) + Developers (Slack, GitHub)
		// But with duplicates, could be 5 total records
		t.Logf("Alice has access to %d app records (may include duplicates)", count)
	}
}

func TestIntegrationOrCondition(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// This query looks for user okta_user_001 with ACTIVE or SUSPENDED status
	// Alice is ACTIVE, so should return 1 row
	compiled, err := db.Compile("SELECT * FROM user:okta_user_001 WHERE status = 'ACTIVE' OR status = 'SUSPENDED'")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)
	t.Logf("Params: %v", compiled.Params)

	if !strings.Contains(compiled.SQL, "OR") {
		t.Errorf("SQL missing OR: %s", compiled.SQL)
	}

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}

func TestIntegrationComplexOrAnd(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Complex query with AND and OR
	compiled, err := db.Compile("SELECT * FROM user:okta_user_001 WHERE status = 'ACTIVE' AND provider = 'okta' OR status = 'SUSPENDED'")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)
	t.Logf("Params: %v", compiled.Params)

	if !strings.Contains(compiled.SQL, "AND") {
		t.Errorf("SQL missing AND: %s", compiled.SQL)
	}
	if !strings.Contains(compiled.SQL, "OR") {
		t.Errorf("SQL missing OR: %s", compiled.SQL)
	}

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	// Alice is ACTIVE and provider is okta, so first condition matches
	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}

func TestIntegrationOffset(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Get apps Alice has access to, with LIMIT and OFFSET
	compiled, err := db.Compile("SELECT ->member_of->group->has_access->app FROM user:okta_user_001 LIMIT 2 OFFSET 1")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	if !strings.Contains(compiled.SQL, "LIMIT 2") {
		t.Errorf("SQL missing LIMIT: %s", compiled.SQL)
	}
	if !strings.Contains(compiled.SQL, "OFFSET 1") {
		t.Errorf("SQL missing OFFSET: %s", compiled.SQL)
	}

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	// With LIMIT 2 OFFSET 1, should get at most 2 rows
	if count > 2 {
		t.Errorf("Expected at most 2 rows with LIMIT 2, got %d", count)
	}
	t.Logf("Got %d rows with LIMIT 2 OFFSET 1", count)
}

func TestIntegrationPagination(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// First page
	compiled1, err := db.Compile("SELECT ->member_of->group->has_access->app FROM user:okta_user_001 LIMIT 2 OFFSET 0")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	rows1, err := conn.QueryContext(ctx, compiled1.SQL, compiled1.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}

	page1Count := 0
	for rows1.Next() {
		page1Count++
	}
	rows1.Close()

	// Second page
	compiled2, err := db.Compile("SELECT ->member_of->group->has_access->app FROM user:okta_user_001 LIMIT 2 OFFSET 2")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	rows2, err := conn.QueryContext(ctx, compiled2.SQL, compiled2.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}

	page2Count := 0
	for rows2.Next() {
		page2Count++
	}
	rows2.Close()

	t.Logf("Page 1: %d rows, Page 2: %d rows", page1Count, page2Count)
}

func TestIntegrationPathWithOrderByAndLimit(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// Traverse to groups with LIMIT (ORDER BY applies to starting entity)
	compiled, err := db.Compile("SELECT ->member_of->group FROM user:okta_user_001 LIMIT 1")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, externalID, name, description, provider string
		var rawData interface{}
		var version int
		var validFrom, validTo interface{}
		err := rows.Scan(&id, &externalID, &name, &description, &provider, &rawData, &version, &validFrom, &validTo)
		if err != nil {
			t.Fatalf("Scan error: %v", err)
		}
		t.Logf("Group: %s", name)
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 group with LIMIT 1, got %d", count)
	}
}

func TestIntegrationOrderByOnStartEntity(t *testing.T) {
	conn := getTestDB(t)
	defer conn.Close()

	db := New(integrationSchema()).WithConnection(conn)

	ctx := context.Background()

	// ORDER BY on the starting entity's field
	compiled, err := db.Compile("SELECT * FROM user:okta_user_001 ORDER BY email DESC")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Logf("SQL: %s", compiled.SQL)

	if !strings.Contains(compiled.SQL, "ORDER BY t0.email DESC") {
		t.Errorf("SQL missing proper ORDER BY: %s", compiled.SQL)
	}

	rows, err := conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}
