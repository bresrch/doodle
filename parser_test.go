package doodle

import (
	"strings"
	"testing"
)

func TestParseSimpleSelect(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "select star",
			input:   "SELECT * FROM user:abc123",
			wantErr: false,
		},
		{
			name:    "select with path",
			input:   "SELECT ->member_of->group FROM user:abc123",
			wantErr: false,
		},
		{
			name:    "multi-hop path",
			input:   "SELECT ->member_of->group->has_access->app FROM user:abc123",
			wantErr: false,
		},
		{
			name:    "with version",
			input:   "SELECT * FROM user:abc123 VERSION d'2024-01-01T00:00:00Z'",
			wantErr: false,
		},
		{
			name:    "with where clause",
			input:   "SELECT * FROM user:abc123 WHERE status = 'ACTIVE'",
			wantErr: false,
		},
		{
			name:    "with limit",
			input:   "SELECT * FROM user:abc123 LIMIT 10",
			wantErr: false,
		},
		{
			name:    "quoted id",
			input:   "SELECT * FROM user:'okta_user_001'",
			wantErr: false,
		},
		{
			name:    "case insensitive keywords",
			input:   "select * from user:abc123 where status = 'ACTIVE'",
			wantErr: false,
		},
		{
			name:    "incoming edge",
			input:   "SELECT <-member_of<-user FROM group:admins",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseFromClause(t *testing.T) {
	input := "SELECT * FROM user:okta_123"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.From.Entity != "user" {
		t.Errorf("From.Entity = %v, want user", q.From.Entity)
	}
	if q.From.ID != "okta_123" {
		t.Errorf("From.ID = %v, want okta_123", q.From.ID)
	}
}

func TestParsePath(t *testing.T) {
	input := "SELECT ->member_of->group->has_access->app FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Path == nil {
		t.Fatal("Select.Path is nil")
	}

	traversals := q.Select.Path.Traversals
	// Path: ->member_of ->group ->has_access ->app = 4 traversals
	// Pattern: relationship, entity, relationship, entity
	if len(traversals) != 4 {
		t.Fatalf("len(traversals) = %d, want 4", len(traversals))
	}

	expected := []struct {
		direction string
		target    string
	}{
		{"->", "member_of"},
		{"->", "group"},
		{"->", "has_access"},
		{"->", "app"},
	}

	for i, e := range expected {
		if traversals[i].Direction != e.direction {
			t.Errorf("traversals[%d].Direction = %v, want %v", i, traversals[i].Direction, e.direction)
		}
		if traversals[i].Target != e.target {
			t.Errorf("traversals[%d].Target = %v, want %v", i, traversals[i].Target, e.target)
		}
	}
}

func TestParseVersionTimestamp(t *testing.T) {
	input := "SELECT * FROM user:abc VERSION d'2024-01-15T10:30:00Z'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Version == nil {
		t.Fatal("Version is nil")
	}

	if q.Version.Timestamp == nil {
		t.Fatal("Version.Timestamp is nil")
	}

	want := "2024-01-15T10:30:00Z"
	if *q.Version.Timestamp != want {
		t.Errorf("Version.Timestamp = %v, want %v", *q.Version.Timestamp, want)
	}
}

func TestParseVersionNumber(t *testing.T) {
	input := "SELECT * FROM user:abc VERSION 3"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Version == nil {
		t.Fatal("Version is nil")
	}

	if q.Version.Number == nil {
		t.Fatal("Version.Number is nil")
	}

	if *q.Version.Number != 3 {
		t.Errorf("Version.Number = %v, want 3", *q.Version.Number)
	}
}

func TestParseWhere(t *testing.T) {
	input := "SELECT * FROM user:abc WHERE status = 'ACTIVE' AND provider = 'okta'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Where == nil {
		t.Fatal("Where is nil")
	}

	if len(q.Where.Or) != 1 {
		t.Fatalf("len(Or) = %d, want 1", len(q.Where.Or))
	}

	conditions := q.Where.Or[0].Conditions
	if len(conditions) != 2 {
		t.Fatalf("len(Conditions) = %d, want 2", len(conditions))
	}

	// First condition
	if conditions[0].Left.Field != "status" {
		t.Errorf("Conditions[0].Left.Field = %v, want status", conditions[0].Left.Field)
	}
	if conditions[0].Op != "=" {
		t.Errorf("Conditions[0].Op = %v, want =", conditions[0].Op)
	}
	if *conditions[0].Right.String != "ACTIVE" {
		t.Errorf("Conditions[0].Right.String = %v, want ACTIVE", *conditions[0].Right.String)
	}
}

func TestParseLimit(t *testing.T) {
	input := "SELECT * FROM user:abc LIMIT 25"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Limit == nil {
		t.Fatal("Limit is nil")
	}

	if *q.Limit != 25 {
		t.Errorf("Limit = %d, want 25", *q.Limit)
	}
}

func TestParseInOperator(t *testing.T) {
	input := "SELECT * FROM user:abc WHERE status IN ('ACTIVE', 'PENDING')"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Where == nil || len(q.Where.Or) == 0 || len(q.Where.Or[0].Conditions) == 0 {
		t.Fatal("Where clause not parsed")
	}

	cond := q.Where.Or[0].Conditions[0]
	if cond.Op != "IN" {
		t.Errorf("Op = %v, want IN", cond.Op)
	}

	if len(cond.Right.List) != 2 {
		t.Errorf("len(List) = %d, want 2", len(cond.Right.List))
	}
}

func TestParseOrderBy(t *testing.T) {
	input := "SELECT * FROM user:abc ORDER BY email DESC"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.OrderBy == nil {
		t.Fatal("OrderBy is nil")
	}

	if len(q.OrderBy.Fields) != 1 {
		t.Fatalf("len(Fields) = %d, want 1", len(q.OrderBy.Fields))
	}

	if q.OrderBy.Fields[0].Field.Name != "email" {
		t.Errorf("Field.Name = %v, want email", q.OrderBy.Fields[0].Field.Name)
	}

	if q.OrderBy.Fields[0].Direction != "DESC" {
		t.Errorf("Direction = %v, want DESC", q.OrderBy.Fields[0].Direction)
	}
}

func TestParseOrderByMultiple(t *testing.T) {
	input := "SELECT * FROM user:abc ORDER BY status ASC, email DESC"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.OrderBy == nil || len(q.OrderBy.Fields) != 2 {
		t.Fatal("Expected 2 ORDER BY fields")
	}

	if q.OrderBy.Fields[0].Field.Name != "status" {
		t.Errorf("First field = %v, want status", q.OrderBy.Fields[0].Field.Name)
	}
	if q.OrderBy.Fields[1].Field.Name != "email" {
		t.Errorf("Second field = %v, want email", q.OrderBy.Fields[1].Field.Name)
	}
}

func TestParseOffset(t *testing.T) {
	input := "SELECT * FROM user:abc LIMIT 10 OFFSET 20"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Limit == nil || *q.Limit != 10 {
		t.Errorf("Limit = %v, want 10", q.Limit)
	}

	if q.Offset == nil || *q.Offset != 20 {
		t.Errorf("Offset = %v, want 20", q.Offset)
	}
}

func TestParseFieldSelection(t *testing.T) {
	input := "SELECT email, status FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Star {
		t.Error("Star should be false")
	}

	if len(q.Select.Fields) != 2 {
		t.Fatalf("len(Fields) = %d, want 2", len(q.Select.Fields))
	}

	if q.Select.Fields[0].Field.Name != "email" {
		t.Errorf("Fields[0].Field.Name = %v, want email", q.Select.Fields[0].Field.Name)
	}
	if q.Select.Fields[1].Field.Name != "status" {
		t.Errorf("Fields[1].Field.Name = %v, want status", q.Select.Fields[1].Field.Name)
	}
}

func TestParseAggregate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		function string
	}{
		{"count star", "SELECT COUNT(*) FROM user:abc", "COUNT"},
		{"count field", "SELECT COUNT(email) FROM user:abc", "COUNT"},
		{"sum", "SELECT SUM(score) FROM user:abc", "SUM"},
		{"avg", "SELECT AVG(score) FROM user:abc", "AVG"},
		{"min", "SELECT MIN(created_at) FROM user:abc", "MIN"},
		{"max", "SELECT MAX(created_at) FROM user:abc", "MAX"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if q.Select.Aggregate == nil {
				t.Fatal("Aggregate is nil")
			}

			if q.Select.Aggregate.Function != tt.function {
				t.Errorf("Function = %v, want %v", q.Select.Aggregate.Function, tt.function)
			}
		})
	}
}

func TestParseCountPath(t *testing.T) {
	input := "SELECT COUNT(->member_of->group) FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Aggregate == nil {
		t.Fatal("Aggregate is nil")
	}

	if q.Select.Aggregate.Function != "COUNT" {
		t.Errorf("Function = %v, want COUNT", q.Select.Aggregate.Function)
	}

	if q.Select.Aggregate.Path == nil {
		t.Fatal("Aggregate.Path is nil")
	}

	if len(q.Select.Aggregate.Path.Traversals) != 2 {
		t.Errorf("len(Traversals) = %d, want 2", len(q.Select.Aggregate.Path.Traversals))
	}
}

func TestParseOrCondition(t *testing.T) {
	input := "SELECT * FROM user:abc WHERE status = 'ACTIVE' OR status = 'PENDING'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Where == nil {
		t.Fatal("Where is nil")
	}

	if len(q.Where.Or) != 2 {
		t.Fatalf("len(Or) = %d, want 2", len(q.Where.Or))
	}

	// First OR group
	if len(q.Where.Or[0].Conditions) != 1 {
		t.Errorf("len(Or[0].Conditions) = %d, want 1", len(q.Where.Or[0].Conditions))
	}
	if *q.Where.Or[0].Conditions[0].Right.String != "ACTIVE" {
		t.Errorf("Or[0] value = %v, want ACTIVE", *q.Where.Or[0].Conditions[0].Right.String)
	}

	// Second OR group
	if len(q.Where.Or[1].Conditions) != 1 {
		t.Errorf("len(Or[1].Conditions) = %d, want 1", len(q.Where.Or[1].Conditions))
	}
	if *q.Where.Or[1].Conditions[0].Right.String != "PENDING" {
		t.Errorf("Or[1] value = %v, want PENDING", *q.Where.Or[1].Conditions[0].Right.String)
	}
}

func TestParseComplexOrAnd(t *testing.T) {
	input := "SELECT * FROM user:abc WHERE status = 'ACTIVE' AND provider = 'okta' OR status = 'PENDING'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.Where.Or) != 2 {
		t.Fatalf("len(Or) = %d, want 2", len(q.Where.Or))
	}

	// First group has 2 AND conditions
	if len(q.Where.Or[0].Conditions) != 2 {
		t.Errorf("len(Or[0].Conditions) = %d, want 2", len(q.Where.Or[0].Conditions))
	}

	// Second group has 1 condition
	if len(q.Where.Or[1].Conditions) != 1 {
		t.Errorf("len(Or[1].Conditions) = %d, want 1", len(q.Where.Or[1].Conditions))
	}
}

func TestParseDistinct(t *testing.T) {
	input := "SELECT DISTINCT status FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if !q.Select.Distinct {
		t.Error("Distinct should be true")
	}

	if len(q.Select.Fields) != 1 {
		t.Fatalf("len(Fields) = %d, want 1", len(q.Select.Fields))
	}

	if q.Select.Fields[0].Field.Name != "status" {
		t.Errorf("Field.Name = %v, want status", q.Select.Fields[0].Field.Name)
	}
}

func TestParseDistinctStar(t *testing.T) {
	input := "SELECT DISTINCT * FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if !q.Select.Distinct {
		t.Error("Distinct should be true")
	}

	if !q.Select.Star {
		t.Error("Star should be true")
	}
}

func TestParseCountDistinct(t *testing.T) {
	input := "SELECT COUNT(DISTINCT status) FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Aggregate == nil {
		t.Fatal("Aggregate is nil")
	}

	if q.Select.Aggregate.Function != "COUNT" {
		t.Errorf("Function = %v, want COUNT", q.Select.Aggregate.Function)
	}

	if !q.Select.Aggregate.Distinct {
		t.Error("Aggregate.Distinct should be true")
	}

	if q.Select.Aggregate.Field == nil || q.Select.Aggregate.Field.Name != "status" {
		t.Error("Expected field 'status'")
	}
}

func TestParseBetween(t *testing.T) {
	input := "SELECT * FROM user:abc WHERE score BETWEEN 10 AND 100"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Where == nil || len(q.Where.Or) == 0 {
		t.Fatal("Where clause not parsed")
	}

	cond := q.Where.Or[0].Conditions[0]
	if cond.Op != "BETWEEN" {
		t.Errorf("Op = %v, want BETWEEN", cond.Op)
	}

	if cond.Between == nil {
		t.Fatal("Between is nil")
	}

	if cond.Between.Low.Int == nil || *cond.Between.Low.Int != 10 {
		t.Errorf("Low = %v, want 10", cond.Between.Low.Int)
	}

	if cond.Between.High.Int == nil || *cond.Between.High.Int != 100 {
		t.Errorf("High = %v, want 100", cond.Between.High.Int)
	}
}

func TestParseGroupBy(t *testing.T) {
	input := "SELECT status, COUNT(*) FROM user:abc GROUP BY status"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.GroupBy == nil {
		t.Fatal("GroupBy is nil")
	}

	if len(q.GroupBy.Fields) != 1 {
		t.Fatalf("len(Fields) = %d, want 1", len(q.GroupBy.Fields))
	}

	if q.GroupBy.Fields[0].Name != "status" {
		t.Errorf("Field.Name = %v, want status", q.GroupBy.Fields[0].Name)
	}
}

func TestParseGroupByMultiple(t *testing.T) {
	input := "SELECT status, provider, COUNT(*) FROM user:abc GROUP BY status, provider"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.GroupBy == nil || len(q.GroupBy.Fields) != 2 {
		t.Fatal("Expected 2 GROUP BY fields")
	}

	if q.GroupBy.Fields[0].Name != "status" {
		t.Errorf("Fields[0].Name = %v, want status", q.GroupBy.Fields[0].Name)
	}
	if q.GroupBy.Fields[1].Name != "provider" {
		t.Errorf("Fields[1].Name = %v, want provider", q.GroupBy.Fields[1].Name)
	}
}

func TestParseHaving(t *testing.T) {
	input := "SELECT status, COUNT(*) FROM user:abc GROUP BY status HAVING COUNT(*) > 5"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Having == nil {
		t.Fatal("Having is nil")
	}

	if len(q.Having.Or) != 1 {
		t.Fatalf("len(Or) = %d, want 1", len(q.Having.Or))
	}

	cond := q.Having.Or[0].Conditions[0]
	if cond.Aggregate == nil {
		t.Fatal("Aggregate is nil")
	}

	if cond.Aggregate.Function != "COUNT" {
		t.Errorf("Function = %v, want COUNT", cond.Aggregate.Function)
	}

	if cond.Op != ">" {
		t.Errorf("Op = %v, want >", cond.Op)
	}

	if cond.Right.Int == nil || *cond.Right.Int != 5 {
		t.Errorf("Right = %v, want 5", cond.Right.Int)
	}
}

func TestParseFieldAlias(t *testing.T) {
	input := "SELECT email AS e, status AS s FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.Select.Fields) != 2 {
		t.Fatalf("len(Fields) = %d, want 2", len(q.Select.Fields))
	}

	if q.Select.Fields[0].Field.Name != "email" || q.Select.Fields[0].Alias != "e" {
		t.Errorf("Fields[0] = %v/%v, want email/e", q.Select.Fields[0].Field.Name, q.Select.Fields[0].Alias)
	}

	if q.Select.Fields[1].Field.Name != "status" || q.Select.Fields[1].Alias != "s" {
		t.Errorf("Fields[1] = %v/%v, want status/s", q.Select.Fields[1].Field.Name, q.Select.Fields[1].Alias)
	}
}

func TestParseSubqueryInFrom(t *testing.T) {
	input := "SELECT * FROM (SELECT * FROM user:abc WHERE status = 'ACTIVE') AS active_users"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.From.Subquery == nil {
		t.Fatal("Subquery is nil")
	}

	if q.From.Alias != "active_users" {
		t.Errorf("Alias = %v, want active_users", q.From.Alias)
	}

	subq := q.From.Subquery
	if subq.From.Entity != "user" {
		t.Errorf("Subquery.From.Entity = %v, want user", subq.From.Entity)
	}

	if subq.Where == nil {
		t.Error("Subquery should have WHERE clause")
	}
}

func TestParseSubqueryInWhere(t *testing.T) {
	input := "SELECT * FROM user:abc WHERE id IN (SELECT user_id FROM active_users)"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Where == nil || len(q.Where.Or) == 0 {
		t.Fatal("Where clause not parsed")
	}

	cond := q.Where.Or[0].Conditions[0]
	if cond.Op != "IN" {
		t.Errorf("Op = %v, want IN", cond.Op)
	}

	if cond.Right.Subquery == nil {
		t.Fatal("Subquery is nil")
	}

	subq := cond.Right.Subquery
	if subq.From.Entity != "active_users" {
		t.Errorf("Subquery.From.Entity = %v, want active_users", subq.From.Entity)
	}
}

func TestParseFromAlias(t *testing.T) {
	input := "SELECT u.email FROM user:abc AS u"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.From.Alias != "u" {
		t.Errorf("From.Alias = %v, want u", q.From.Alias)
	}

	if len(q.Select.Fields) != 1 {
		t.Fatalf("len(Fields) = %d, want 1", len(q.Select.Fields))
	}

	if q.Select.Fields[0].Field.Entity != "u" {
		t.Errorf("Field.Entity = %v, want u", q.Select.Fields[0].Field.Entity)
	}
}

func TestParseOptionalTraversal(t *testing.T) {
	input := "SELECT ->?member_of->group FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Path == nil {
		t.Fatal("Path is nil")
	}

	if len(q.Select.Path.Traversals) != 2 {
		t.Fatalf("len(Traversals) = %d, want 2", len(q.Select.Path.Traversals))
	}

	// First traversal should be optional
	if q.Select.Path.Traversals[0].Direction != "->?" {
		t.Errorf("Traversals[0].Direction = %v, want ->?", q.Select.Path.Traversals[0].Direction)
	}
	if !q.Select.Path.Traversals[0].IsOptional() {
		t.Error("Traversals[0] should be optional")
	}
	if q.Select.Path.Traversals[0].BaseDirection() != "->" {
		t.Errorf("Traversals[0].BaseDirection() = %v, want ->", q.Select.Path.Traversals[0].BaseDirection())
	}

	// Second traversal should be regular
	if q.Select.Path.Traversals[1].Direction != "->" {
		t.Errorf("Traversals[1].Direction = %v, want ->", q.Select.Path.Traversals[1].Direction)
	}
	if q.Select.Path.Traversals[1].IsOptional() {
		t.Error("Traversals[1] should not be optional")
	}
}

func TestParseOptionalIncomingTraversal(t *testing.T) {
	input := "SELECT <?-member_of<?-user FROM group:admins"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Path == nil {
		t.Fatal("Path is nil")
	}

	if len(q.Select.Path.Traversals) != 2 {
		t.Fatalf("len(Traversals) = %d, want 2", len(q.Select.Path.Traversals))
	}

	// First traversal should be optional incoming
	if q.Select.Path.Traversals[0].Direction != "<?-" {
		t.Errorf("Traversals[0].Direction = %v, want <?-", q.Select.Path.Traversals[0].Direction)
	}
	if !q.Select.Path.Traversals[0].IsOptional() {
		t.Error("Traversals[0] should be optional")
	}
	if q.Select.Path.Traversals[0].BaseDirection() != "<-" {
		t.Errorf("Traversals[0].BaseDirection() = %v, want <-", q.Select.Path.Traversals[0].BaseDirection())
	}
}

func TestParsePathFieldAccess(t *testing.T) {
	input := "SELECT ->member_of.role->group FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Path == nil {
		t.Fatal("Path is nil")
	}

	if len(q.Select.Path.Traversals) != 2 {
		t.Fatalf("len(Traversals) = %d, want 2", len(q.Select.Path.Traversals))
	}

	// First traversal (relationship) should have field access
	if q.Select.Path.Traversals[0].Target != "member_of" {
		t.Errorf("Traversals[0].Target = %v, want member_of", q.Select.Path.Traversals[0].Target)
	}
	if q.Select.Path.Traversals[0].Field != "role" {
		t.Errorf("Traversals[0].Field = %v, want role", q.Select.Path.Traversals[0].Field)
	}

	// Second traversal (entity) should not have field access
	if q.Select.Path.Traversals[1].Target != "group" {
		t.Errorf("Traversals[1].Target = %v, want group", q.Select.Path.Traversals[1].Target)
	}
	if q.Select.Path.Traversals[1].Field != "" {
		t.Errorf("Traversals[1].Field = %v, want empty", q.Select.Path.Traversals[1].Field)
	}
}

func TestParsePathFieldAccessMultiple(t *testing.T) {
	input := "SELECT ->member_of.role->group->has_access.permission->app FROM user:abc"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Path == nil {
		t.Fatal("Path is nil")
	}

	if len(q.Select.Path.Traversals) != 4 {
		t.Fatalf("len(Traversals) = %d, want 4", len(q.Select.Path.Traversals))
	}

	// First hop: member_of relationship with role field
	if q.Select.Path.Traversals[0].Field != "role" {
		t.Errorf("Traversals[0].Field = %v, want role", q.Select.Path.Traversals[0].Field)
	}

	// Second hop: group entity (no field)
	if q.Select.Path.Traversals[1].Field != "" {
		t.Errorf("Traversals[1].Field = %v, want empty", q.Select.Path.Traversals[1].Field)
	}

	// Third hop: has_access relationship with permission field
	if q.Select.Path.Traversals[2].Field != "permission" {
		t.Errorf("Traversals[2].Field = %v, want permission", q.Select.Path.Traversals[2].Field)
	}

	// Fourth hop: app entity (no field)
	if q.Select.Path.Traversals[3].Field != "" {
		t.Errorf("Traversals[3].Field = %v, want empty", q.Select.Path.Traversals[3].Field)
	}
}

func TestParseNegatedPath(t *testing.T) {
	// The field on the last traversal (group.external_id) is parsed as Traversal.Field
	// This tests that the negated path with target field is parsed correctly
	input := "SELECT * FROM user:abc WHERE ->!member_of->group.external_id = 'admins'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Where == nil {
		t.Fatal("Where is nil")
	}

	if len(q.Where.Or) != 1 || len(q.Where.Or[0].Conditions) != 1 {
		t.Fatal("Expected single condition")
	}

	cond := q.Where.Or[0].Conditions[0]

	// Path should have negated first traversal
	if len(cond.Left.Path) != 2 {
		t.Fatalf("len(Path) = %d, want 2", len(cond.Left.Path))
	}

	if cond.Left.Path[0].Direction != "->!" {
		t.Errorf("Path[0].Direction = %v, want ->!", cond.Left.Path[0].Direction)
	}
	if !cond.Left.Path[0].IsNegated() {
		t.Error("Path[0] should be negated")
	}
	if cond.Left.Path[0].BaseDirection() != "->" {
		t.Errorf("Path[0].BaseDirection() = %v, want ->", cond.Left.Path[0].BaseDirection())
	}
	if cond.Left.Path[0].Target != "member_of" {
		t.Errorf("Path[0].Target = %v, want member_of", cond.Left.Path[0].Target)
	}

	if cond.Left.Path[1].Direction != "->" {
		t.Errorf("Path[1].Direction = %v, want ->", cond.Left.Path[1].Direction)
	}
	if cond.Left.Path[1].Target != "group" {
		t.Errorf("Path[1].Target = %v, want group", cond.Left.Path[1].Target)
	}
	// The field is attached to the last traversal, not ConditionField.Field
	if cond.Left.Path[1].Field != "external_id" {
		t.Errorf("Path[1].Field = %v, want external_id", cond.Left.Path[1].Field)
	}
}

func TestParseNegatedIncomingPath(t *testing.T) {
	input := "SELECT * FROM group:admins WHERE <!-member_of<!-user.status = 'ACTIVE'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Where == nil {
		t.Fatal("Where is nil")
	}

	cond := q.Where.Or[0].Conditions[0]

	if cond.Left.Path[0].Direction != "<!-" {
		t.Errorf("Path[0].Direction = %v, want <!-", cond.Left.Path[0].Direction)
	}
	if !cond.Left.Path[0].IsNegated() {
		t.Error("Path[0] should be negated")
	}
	if cond.Left.Path[0].BaseDirection() != "<-" {
		t.Errorf("Path[0].BaseDirection() = %v, want <-", cond.Left.Path[0].BaseDirection())
	}
	// The field is attached to the last traversal
	if cond.Left.Path[1].Field != "status" {
		t.Errorf("Path[1].Field = %v, want status", cond.Left.Path[1].Field)
	}
}

func TestParsePathQuantifier(t *testing.T) {
	input := "SELECT ->manages{1,3}->employee FROM user:alice"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Path == nil {
		t.Fatal("Path is nil")
	}

	if len(q.Select.Path.Traversals) != 2 {
		t.Fatalf("len(Traversals) = %d, want 2", len(q.Select.Path.Traversals))
	}

	// First traversal (relationship) should have quantifier
	relTrav := q.Select.Path.Traversals[0]
	if relTrav.Target != "manages" {
		t.Errorf("Traversals[0].Target = %v, want manages", relTrav.Target)
	}
	if !relTrav.HasQuantifier() {
		t.Error("Traversals[0] should have quantifier")
	}
	if relTrav.GetMinHops() != 1 {
		t.Errorf("GetMinHops() = %d, want 1", relTrav.GetMinHops())
	}
	if relTrav.GetMaxHops() != 3 {
		t.Errorf("GetMaxHops() = %d, want 3", relTrav.GetMaxHops())
	}

	// Second traversal (entity) should not have quantifier
	entTrav := q.Select.Path.Traversals[1]
	if entTrav.Target != "employee" {
		t.Errorf("Traversals[1].Target = %v, want employee", entTrav.Target)
	}
	if entTrav.HasQuantifier() {
		t.Error("Traversals[1] should not have quantifier")
	}
}

func TestParsePathQuantifierSingle(t *testing.T) {
	// Single number means exactly that many hops
	input := "SELECT ->reports_to{2}->manager FROM employee:john"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	relTrav := q.Select.Path.Traversals[0]
	if !relTrav.HasQuantifier() {
		t.Error("Traversals[0] should have quantifier")
	}
	if relTrav.GetMinHops() != 2 {
		t.Errorf("GetMinHops() = %d, want 2", relTrav.GetMinHops())
	}
	if relTrav.GetMaxHops() != 2 {
		t.Errorf("GetMaxHops() = %d, want 2 (same as min)", relTrav.GetMaxHops())
	}
}

func TestParseIsNull(t *testing.T) {
	input := "SELECT * FROM user WHERE email IS NULL"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Where == nil {
		t.Fatal("Where is nil")
	}

	cond := q.Where.Or[0].Conditions[0]
	if cond.Left.Field != "email" {
		t.Errorf("Field = %v, want email", cond.Left.Field)
	}
	// Op should be "IS NULL" (with spaces removed in comparison)
	op := strings.ToUpper(strings.ReplaceAll(cond.Op, " ", ""))
	if op != "ISNULL" {
		t.Errorf("Op = %v, want ISNULL", op)
	}
}

func TestParseIsNotNull(t *testing.T) {
	input := "SELECT * FROM user WHERE status IS NOT NULL"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	cond := q.Where.Or[0].Conditions[0]
	op := strings.ToUpper(strings.ReplaceAll(cond.Op, " ", ""))
	if op != "ISNOTNULL" {
		t.Errorf("Op = %v, want ISNOTNULL", op)
	}
}

func TestParseExists(t *testing.T) {
	input := "SELECT * FROM user WHERE EXISTS (SELECT * FROM group WHERE name = 'admins')"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	cond := q.Where.Or[0].Conditions[0]
	if cond.Exists == nil {
		t.Fatal("Exists subquery is nil")
	}
	if cond.Exists.From.Entity != "group" {
		t.Errorf("Exists.From.Entity = %v, want group", cond.Exists.From.Entity)
	}
}

func TestParseNotExists(t *testing.T) {
	input := "SELECT * FROM user WHERE NOT EXISTS (SELECT * FROM banned WHERE status = 'active')"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	cond := q.Where.Or[0].Conditions[0]
	if !cond.Not {
		t.Error("Not should be true")
	}
	if cond.Exists == nil {
		t.Fatal("Exists subquery is nil")
	}
}

func TestParseNotCondition(t *testing.T) {
	input := "SELECT * FROM user WHERE NOT status = 'BANNED'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	cond := q.Where.Or[0].Conditions[0]
	if !cond.Not {
		t.Error("Not should be true")
	}
	if cond.Left.Field != "status" {
		t.Errorf("Field = %v, want status", cond.Left.Field)
	}
}

func TestParseUnion(t *testing.T) {
	input := "SELECT * FROM user UNION SELECT * FROM group"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.From.Entity != "user" {
		t.Errorf("First query FROM = %v, want user", q.From.Entity)
	}

	if len(q.Compound) != 1 {
		t.Fatalf("Expected 1 compound part, got %d", len(q.Compound))
	}

	if q.Compound[0].Operator != "UNION" {
		t.Errorf("Operator = %v, want UNION", q.Compound[0].Operator)
	}

	if q.Compound[0].Query.From.Entity != "group" {
		t.Errorf("Second query FROM = %v, want group", q.Compound[0].Query.From.Entity)
	}
}

func TestParseUnionAll(t *testing.T) {
	input := "SELECT * FROM user UNION ALL SELECT * FROM group"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.Compound) != 1 {
		t.Fatalf("Expected 1 compound part, got %d", len(q.Compound))
	}

	if q.Compound[0].Operator != "UNIONALL" {
		t.Errorf("Operator = %v, want UNIONALL", q.Compound[0].Operator)
	}
}

func TestParseIntersect(t *testing.T) {
	input := "SELECT * FROM user WHERE status = 'ACTIVE' INTERSECT SELECT * FROM user WHERE provider = 'okta'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.Compound) != 1 {
		t.Fatalf("Expected 1 compound part, got %d", len(q.Compound))
	}

	if q.Compound[0].Operator != "INTERSECT" {
		t.Errorf("Operator = %v, want INTERSECT", q.Compound[0].Operator)
	}
}

func TestParseExcept(t *testing.T) {
	input := "SELECT * FROM user EXCEPT SELECT * FROM user WHERE status = 'INACTIVE'"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.Compound) != 1 {
		t.Fatalf("Expected 1 compound part, got %d", len(q.Compound))
	}

	if q.Compound[0].Operator != "EXCEPT" {
		t.Errorf("Operator = %v, want EXCEPT", q.Compound[0].Operator)
	}
}

func TestParseMultipleUnion(t *testing.T) {
	input := "SELECT * FROM user UNION SELECT * FROM group UNION SELECT * FROM app"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// First query has one compound part, which contains nested compound
	if len(q.Compound) != 1 {
		t.Fatalf("Expected 1 compound part at top level, got %d", len(q.Compound))
	}

	if q.Compound[0].Query.From.Entity != "group" {
		t.Errorf("Second query FROM = %v, want group", q.Compound[0].Query.From.Entity)
	}

	// Third query is nested inside the second query's compound
	if len(q.Compound[0].Query.Compound) != 1 {
		t.Fatalf("Expected 1 nested compound part, got %d", len(q.Compound[0].Query.Compound))
	}

	if q.Compound[0].Query.Compound[0].Query.From.Entity != "app" {
		t.Errorf("Third query FROM = %v, want app", q.Compound[0].Query.Compound[0].Query.From.Entity)
	}
}

func TestParseCaseExpression(t *testing.T) {
	input := "SELECT CASE WHEN status = 'ACTIVE' THEN 'yes' ELSE 'no' END AS is_active FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.Select.Fields) != 1 {
		t.Fatalf("Expected 1 field, got %d", len(q.Select.Fields))
	}

	if q.Select.Fields[0].Case == nil {
		t.Fatal("Expected CASE expression")
	}

	if len(q.Select.Fields[0].Case.Whens) != 1 {
		t.Errorf("Expected 1 WHEN clause, got %d", len(q.Select.Fields[0].Case.Whens))
	}

	if q.Select.Fields[0].Case.Else == nil {
		t.Error("Expected ELSE clause")
	}

	if q.Select.Fields[0].Alias != "is_active" {
		t.Errorf("Alias = %v, want is_active", q.Select.Fields[0].Alias)
	}
}

func TestParseCaseMultipleWhens(t *testing.T) {
	input := "SELECT CASE WHEN status = 'ACTIVE' THEN 1 WHEN status = 'PENDING' THEN 2 ELSE 0 END FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].Case == nil {
		t.Fatal("Expected CASE expression")
	}

	if len(q.Select.Fields[0].Case.Whens) != 2 {
		t.Errorf("Expected 2 WHEN clauses, got %d", len(q.Select.Fields[0].Case.Whens))
	}
}

func TestParseCoalesce(t *testing.T) {
	input := "SELECT COALESCE(email, 'unknown') AS email FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.Select.Fields) != 1 {
		t.Fatalf("Expected 1 field, got %d", len(q.Select.Fields))
	}

	if q.Select.Fields[0].Coalesce == nil {
		t.Fatal("Expected COALESCE expression")
	}

	if len(q.Select.Fields[0].Coalesce.Values) != 2 {
		t.Errorf("Expected 2 values in COALESCE, got %d", len(q.Select.Fields[0].Coalesce.Values))
	}

	if q.Select.Fields[0].Alias != "email" {
		t.Errorf("Alias = %v, want email", q.Select.Fields[0].Alias)
	}
}

func TestParseNullif(t *testing.T) {
	input := "SELECT NULLIF(status, 'DELETED') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].Nullif == nil {
		t.Fatal("Expected NULLIF expression")
	}

	if q.Select.Fields[0].Nullif.First == nil || q.Select.Fields[0].Nullif.Second == nil {
		t.Error("Expected two arguments to NULLIF")
	}
}

func TestParseCTE(t *testing.T) {
	input := "WITH active_users AS (SELECT * FROM user WHERE status = 'ACTIVE') SELECT * FROM active_users"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.With) != 1 {
		t.Fatalf("Expected 1 CTE, got %d", len(q.With))
	}

	if q.With[0].Name != "active_users" {
		t.Errorf("CTE name = %v, want active_users", q.With[0].Name)
	}

	if q.With[0].Query == nil {
		t.Fatal("CTE query is nil")
	}

	if q.From.Entity != "active_users" {
		t.Errorf("Main query FROM = %v, want active_users", q.From.Entity)
	}
}

func TestParseMultipleCTEs(t *testing.T) {
	input := "WITH active AS (SELECT * FROM user WHERE status = 'ACTIVE'), admins AS (SELECT * FROM group WHERE name = 'admins') SELECT * FROM active"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.With) != 2 {
		t.Fatalf("Expected 2 CTEs, got %d", len(q.With))
	}

	if q.With[0].Name != "active" {
		t.Errorf("First CTE name = %v, want active", q.With[0].Name)
	}

	if q.With[1].Name != "admins" {
		t.Errorf("Second CTE name = %v, want admins", q.With[1].Name)
	}
}

func TestParseUpperFunction(t *testing.T) {
	input := "SELECT UPPER(email) AS upper_email FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "UPPER" {
		t.Errorf("Function name = %v, want UPPER", q.Select.Fields[0].FuncCall.Name)
	}

	if len(q.Select.Fields[0].FuncCall.Args) != 1 {
		t.Errorf("Expected 1 arg, got %d", len(q.Select.Fields[0].FuncCall.Args))
	}
}

func TestParseConcatFunction(t *testing.T) {
	input := "SELECT CONCAT(email, '-suffix') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "CONCAT" {
		t.Errorf("Function name = %v, want CONCAT", q.Select.Fields[0].FuncCall.Name)
	}

	if len(q.Select.Fields[0].FuncCall.Args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(q.Select.Fields[0].FuncCall.Args))
	}
}

func TestParseAbsFunction(t *testing.T) {
	input := "SELECT ABS(-5) FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "ABS" {
		t.Errorf("Function name = %v, want ABS", q.Select.Fields[0].FuncCall.Name)
	}
}

func TestParseNowFunction(t *testing.T) {
	input := "SELECT NOW() FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "NOW" {
		t.Errorf("Function name = %v, want NOW", q.Select.Fields[0].FuncCall.Name)
	}

	// NOW() has no arguments
	if len(q.Select.Fields[0].FuncCall.Args) != 0 {
		t.Errorf("Expected 0 args for NOW(), got %d", len(q.Select.Fields[0].FuncCall.Args))
	}
}

func TestParseArrayAgg(t *testing.T) {
	input := "SELECT ARRAY_AGG(email) FROM user GROUP BY status"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Aggregate == nil {
		t.Fatal("Expected aggregate")
	}

	if q.Select.Aggregate.Function != "ARRAY_AGG" {
		t.Errorf("Function = %v, want ARRAY_AGG", q.Select.Aggregate.Function)
	}
}

func TestParseStringAgg(t *testing.T) {
	input := "SELECT STRING_AGG(email, ', ') FROM user GROUP BY status"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Aggregate == nil {
		t.Fatal("Expected aggregate")
	}

	if q.Select.Aggregate.Function != "STRING_AGG" {
		t.Errorf("Function = %v, want STRING_AGG", q.Select.Aggregate.Function)
	}

	if q.Select.Aggregate.Delimiter == nil || *q.Select.Aggregate.Delimiter != ", " {
		t.Errorf("Delimiter = %v, want ', '", q.Select.Aggregate.Delimiter)
	}
}

func TestParseJsonAgg(t *testing.T) {
	input := "SELECT JSON_AGG(email) FROM user GROUP BY status"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Aggregate == nil {
		t.Fatal("Expected aggregate")
	}

	if q.Select.Aggregate.Function != "JSON_AGG" {
		t.Errorf("Function = %v, want JSON_AGG", q.Select.Aggregate.Function)
	}
}

func TestParseArrayAggDistinct(t *testing.T) {
	input := "SELECT ARRAY_AGG(DISTINCT status) FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Aggregate == nil {
		t.Fatal("Expected aggregate")
	}

	if !q.Select.Aggregate.Distinct {
		t.Error("Expected DISTINCT to be true")
	}
}

func TestParseJsonGet(t *testing.T) {
	input := "SELECT JSON_GET(email, 'domain') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "JSON_GET" {
		t.Errorf("Function name = %v, want JSON_GET", q.Select.Fields[0].FuncCall.Name)
	}

	if len(q.Select.Fields[0].FuncCall.Args) != 2 {
		t.Errorf("Expected 2 args, got %d", len(q.Select.Fields[0].FuncCall.Args))
	}
}

func TestParseJsonText(t *testing.T) {
	input := "SELECT JSON_TEXT(email, 'domain') AS domain FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "JSON_TEXT" {
		t.Errorf("Function name = %v, want JSON_TEXT", q.Select.Fields[0].FuncCall.Name)
	}
}

func TestParseJsonPath(t *testing.T) {
	input := "SELECT JSON_PATH(email, 'address', 'city') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "JSON_PATH" {
		t.Errorf("Function name = %v, want JSON_PATH", q.Select.Fields[0].FuncCall.Name)
	}

	if len(q.Select.Fields[0].FuncCall.Args) != 3 {
		t.Errorf("Expected 3 args, got %d", len(q.Select.Fields[0].FuncCall.Args))
	}
}

func TestParseJsonBuildObject(t *testing.T) {
	input := "SELECT JSON_BUILD_OBJECT('name', email, 'status', status) FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "JSON_BUILD_OBJECT" {
		t.Errorf("Function name = %v, want JSON_BUILD_OBJECT", q.Select.Fields[0].FuncCall.Name)
	}

	if len(q.Select.Fields[0].FuncCall.Args) != 4 {
		t.Errorf("Expected 4 args, got %d", len(q.Select.Fields[0].FuncCall.Args))
	}
}

func TestParseJsonPathText(t *testing.T) {
	input := "SELECT JSON_PATH_TEXT(metadata, 'config', 'theme') AS theme FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].FuncCall == nil {
		t.Fatal("Expected function call")
	}

	if q.Select.Fields[0].FuncCall.Name != "JSON_PATH_TEXT" {
		t.Errorf("Function name = %v, want JSON_PATH_TEXT", q.Select.Fields[0].FuncCall.Name)
	}

	if len(q.Select.Fields[0].FuncCall.Args) != 3 {
		t.Errorf("Expected 3 args, got %d", len(q.Select.Fields[0].FuncCall.Args))
	}

	if q.Select.Fields[0].Alias != "theme" {
		t.Errorf("Alias = %v, want theme", q.Select.Fields[0].Alias)
	}
}

func TestParseStringAggDistinct(t *testing.T) {
	input := "SELECT STRING_AGG(DISTINCT name, ', ') FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Aggregate == nil {
		t.Fatal("Expected aggregate expression")
	}

	if q.Select.Aggregate.Function != "STRING_AGG" {
		t.Errorf("Function = %v, want STRING_AGG", q.Select.Aggregate.Function)
	}

	if !q.Select.Aggregate.Distinct {
		t.Error("Expected DISTINCT to be true")
	}

	if q.Select.Aggregate.Delimiter == nil || *q.Select.Aggregate.Delimiter != ", " {
		t.Errorf("Delimiter = %v, want ', '", q.Select.Aggregate.Delimiter)
	}
}

func TestParseDateArithmeticAdd(t *testing.T) {
	input := "SELECT created + INTERVAL '1 day' AS tomorrow FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(q.Select.Fields) != 1 {
		t.Fatalf("Expected 1 field, got %d", len(q.Select.Fields))
	}

	if q.Select.Fields[0].DateArith == nil {
		t.Fatal("Expected DateArithmetic expression")
	}

	if q.Select.Fields[0].DateArith.Operator != "+" {
		t.Errorf("Operator = %v, want +", q.Select.Fields[0].DateArith.Operator)
	}

	if q.Select.Fields[0].DateArith.Interval.Value != "1 day" {
		t.Errorf("Interval value = %v, want '1 day'", q.Select.Fields[0].DateArith.Interval.Value)
	}

	if q.Select.Fields[0].Alias != "tomorrow" {
		t.Errorf("Alias = %v, want tomorrow", q.Select.Fields[0].Alias)
	}
}

func TestParseDateArithmeticSubtract(t *testing.T) {
	input := "SELECT created - INTERVAL '30 days' AS past FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].DateArith == nil {
		t.Fatal("Expected DateArithmetic expression")
	}

	if q.Select.Fields[0].DateArith.Operator != "-" {
		t.Errorf("Operator = %v, want -", q.Select.Fields[0].DateArith.Operator)
	}

	if q.Select.Fields[0].DateArith.Interval.Value != "30 days" {
		t.Errorf("Interval value = %v, want '30 days'", q.Select.Fields[0].DateArith.Interval.Value)
	}
}

func TestParseDateArithmeticHours(t *testing.T) {
	input := "SELECT created + INTERVAL '2 hours' FROM user"
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if q.Select.Fields[0].DateArith == nil {
		t.Fatal("Expected DateArithmetic expression")
	}

	if q.Select.Fields[0].DateArith.Interval.Value != "2 hours" {
		t.Errorf("Interval value = %v, want '2 hours'", q.Select.Fields[0].DateArith.Interval.Value)
	}
}

// ============================================================================
// INSERT Statement Tests
// ============================================================================

func TestParseInsertValues(t *testing.T) {
	input := "INSERT INTO user (email, status) VALUES ('alice@example.com', 'ACTIVE')"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Insert == nil {
		t.Fatal("Expected INSERT statement")
	}

	if stmt.Insert.Entity != "user" {
		t.Errorf("Entity = %v, want user", stmt.Insert.Entity)
	}

	if len(stmt.Insert.Fields) != 2 {
		t.Fatalf("Expected 2 fields, got %d", len(stmt.Insert.Fields))
	}

	if stmt.Insert.Fields[0] != "email" || stmt.Insert.Fields[1] != "status" {
		t.Errorf("Fields = %v, want [email, status]", stmt.Insert.Fields)
	}

	if len(stmt.Insert.Values) != 1 {
		t.Fatalf("Expected 1 value row, got %d", len(stmt.Insert.Values))
	}

	if len(stmt.Insert.Values[0].Values) != 2 {
		t.Fatalf("Expected 2 values, got %d", len(stmt.Insert.Values[0].Values))
	}
}

func TestParseInsertWithID(t *testing.T) {
	input := "INSERT INTO user:alice (email) VALUES ('alice@example.com')"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Insert == nil {
		t.Fatal("Expected INSERT statement")
	}

	if stmt.Insert.Entity != "user" {
		t.Errorf("Entity = %v, want user", stmt.Insert.Entity)
	}

	if stmt.Insert.ID != "alice" {
		t.Errorf("ID = %v, want alice", stmt.Insert.ID)
	}
}

func TestParseInsertMultipleRows(t *testing.T) {
	input := "INSERT INTO user (email) VALUES ('a@example.com'), ('b@example.com')"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Insert == nil {
		t.Fatal("Expected INSERT statement")
	}

	if len(stmt.Insert.Values) != 2 {
		t.Fatalf("Expected 2 value rows, got %d", len(stmt.Insert.Values))
	}
}

func TestParseInsertReturning(t *testing.T) {
	input := "INSERT INTO user (email) VALUES ('alice@example.com') RETURNING *"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Insert == nil {
		t.Fatal("Expected INSERT statement")
	}

	if stmt.Insert.Returning == nil {
		t.Fatal("Expected RETURNING clause")
	}

	if !stmt.Insert.Returning.Star {
		t.Error("Expected RETURNING *")
	}
}

func TestParseInsertReturningFields(t *testing.T) {
	input := "INSERT INTO user (email) VALUES ('alice@example.com') RETURNING id, email"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Insert.Returning == nil {
		t.Fatal("Expected RETURNING clause")
	}

	if len(stmt.Insert.Returning.Fields) != 2 {
		t.Fatalf("Expected 2 returning fields, got %d", len(stmt.Insert.Returning.Fields))
	}
}

// ============================================================================
// UPDATE Statement Tests
// ============================================================================

func TestParseUpdateBasic(t *testing.T) {
	input := "UPDATE user SET status = 'INACTIVE' WHERE email = 'alice@example.com'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Update == nil {
		t.Fatal("Expected UPDATE statement")
	}

	if stmt.Update.Entity != "user" {
		t.Errorf("Entity = %v, want user", stmt.Update.Entity)
	}

	if len(stmt.Update.Set) != 1 {
		t.Fatalf("Expected 1 SET clause, got %d", len(stmt.Update.Set))
	}

	if stmt.Update.Set[0].Field != "status" {
		t.Errorf("Field = %v, want status", stmt.Update.Set[0].Field)
	}

	if stmt.Update.Where == nil {
		t.Fatal("Expected WHERE clause")
	}
}

func TestParseUpdateWithID(t *testing.T) {
	input := "UPDATE user:alice SET status = 'INACTIVE'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Update == nil {
		t.Fatal("Expected UPDATE statement")
	}

	if stmt.Update.ID != "alice" {
		t.Errorf("ID = %v, want alice", stmt.Update.ID)
	}
}

func TestParseUpdateForce(t *testing.T) {
	input := "UPDATE FORCE user:alice SET status = 'INACTIVE'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Update == nil {
		t.Fatal("Expected UPDATE statement")
	}

	if !stmt.Update.Force {
		t.Error("Expected FORCE to be true")
	}
}

func TestParseUpdateMultipleFields(t *testing.T) {
	input := "UPDATE user:alice SET status = 'INACTIVE', email = 'new@example.com'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if len(stmt.Update.Set) != 2 {
		t.Fatalf("Expected 2 SET clauses, got %d", len(stmt.Update.Set))
	}
}

func TestParseUpdateReturning(t *testing.T) {
	input := "UPDATE user:alice SET status = 'INACTIVE' RETURNING *"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Update.Returning == nil {
		t.Fatal("Expected RETURNING clause")
	}
}

// ============================================================================
// DELETE Statement Tests
// ============================================================================

func TestParseDeleteBasic(t *testing.T) {
	input := "DELETE FROM user WHERE status = 'DELETED'"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Delete == nil {
		t.Fatal("Expected DELETE statement")
	}

	if stmt.Delete.Entity != "user" {
		t.Errorf("Entity = %v, want user", stmt.Delete.Entity)
	}

	if stmt.Delete.Where == nil {
		t.Fatal("Expected WHERE clause")
	}
}

func TestParseDeleteWithID(t *testing.T) {
	input := "DELETE FROM user:alice"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Delete == nil {
		t.Fatal("Expected DELETE statement")
	}

	if stmt.Delete.ID != "alice" {
		t.Errorf("ID = %v, want alice", stmt.Delete.ID)
	}
}

func TestParseDeleteForce(t *testing.T) {
	input := "DELETE FORCE FROM user:alice"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Delete == nil {
		t.Fatal("Expected DELETE statement")
	}

	if !stmt.Delete.Force {
		t.Error("Expected FORCE to be true")
	}
}

func TestParseDeleteReturning(t *testing.T) {
	input := "DELETE FROM user:alice RETURNING *"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Delete.Returning == nil {
		t.Fatal("Expected RETURNING clause")
	}
}

// ============================================================================
// Statement SELECT passthrough
// ============================================================================

func TestParseStatementSelect(t *testing.T) {
	input := "SELECT * FROM user:alice"
	stmt, err := ParseStatement(input)
	if err != nil {
		t.Fatalf("ParseStatement() error = %v", err)
	}

	if stmt.Select == nil {
		t.Fatal("Expected SELECT statement")
	}

	if stmt.Select.From.Entity != "user" {
		t.Errorf("Entity = %v, want user", stmt.Select.From.Entity)
	}
}
