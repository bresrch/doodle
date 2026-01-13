package doodle

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Ref represents an entity reference in the format "type:id"
type Ref struct {
	Type string
	ID   string
}

// ParseRef parses a reference string like "user:alice" into a Ref
func ParseRef(s string) (Ref, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return Ref{}, fmt.Errorf("invalid reference format %q, expected type:id", s)
	}
	return Ref{Type: parts[0], ID: parts[1]}, nil
}

// String returns the reference as "type:id"
func (r Ref) String() string {
	return r.Type + ":" + r.ID
}

// TupleBuilder builds relationship tuples using the pattern:
// subject @relation #target
//
// Example:
//
//	db.Tuple("user:alice", "member_of", "group:admins").
//	    With("role", "admin").
//	    Create(ctx)
type TupleBuilder struct {
	db       *DB
	subject  Ref
	relation string
	target   Ref
	meta     map[string]any
	validAt  *time.Time // Point-in-time for temporal queries
	err      error
}

// At sets a point-in-time for temporal relationship queries.
// Use this for checking if a relationship existed at a specific time.
//
// Example:
//
//	exists, _ := db.Tuple("user:bob", "member_of", "group:admins").
//	    At(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)).
//	    Exists(ctx)
func (tb *TupleBuilder) At(t time.Time) *TupleBuilder {
	if tb.err != nil {
		return tb
	}
	tb.validAt = &t
	return tb
}

// Tuple creates a new relationship tuple builder.
// Pattern: subject @relation #target
//
// Example:
//
//	db.Tuple("user:alice", "member_of", "group:admins")
func (db *DB) Tuple(subject, relation, target string) *TupleBuilder {
	tb := &TupleBuilder{
		db:       db,
		relation: relation,
		meta:     make(map[string]any),
	}

	subRef, err := ParseRef(subject)
	if err != nil {
		tb.err = fmt.Errorf("invalid subject: %w", err)
		return tb
	}
	tb.subject = subRef

	tarRef, err := ParseRef(target)
	if err != nil {
		tb.err = fmt.Errorf("invalid target: %w", err)
		return tb
	}
	tb.target = tarRef

	return tb
}

// With adds metadata to the relationship tuple.
// Metadata is stored in the junction table.
//
// Example:
//
//	db.Tuple("user:alice", "member_of", "group:admins").
//	    With("role", "admin").
//	    With("created_at", time.Now())
func (tb *TupleBuilder) With(key string, value any) *TupleBuilder {
	if tb.err != nil {
		return tb
	}
	tb.meta[key] = value
	return tb
}

// Create inserts the relationship tuple into the database.
// For temporal relationships, sets valid_from to now() and valid_to to 'infinity'.
// Returns the number of rows affected and any error.
func (tb *TupleBuilder) Create(ctx context.Context) (sql.Result, error) {
	if tb.err != nil {
		return nil, tb.err
	}

	// Find the relationship in schema
	rel, err := tb.db.schema.FindRelationship(tb.subject.Type, tb.relation, "->")
	if err != nil {
		return nil, fmt.Errorf("relationship %s not found: %w", tb.relation, err)
	}

	// Validate target entity type
	if rel.ToEntity != tb.target.Type {
		return nil, fmt.Errorf("relationship %s targets %s, not %s", tb.relation, rel.ToEntity, tb.target.Type)
	}

	// Get entity definitions
	fromEntity, err := tb.db.schema.GetEntity(tb.subject.Type)
	if err != nil {
		return nil, err
	}
	toEntity, err := tb.db.schema.GetEntity(tb.target.Type)
	if err != nil {
		return nil, err
	}

	// Build the INSERT query
	// We need to look up the actual IDs from external_id
	columns := []string{rel.FromKey, rel.ToKey}
	values := []string{
		fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $1)", fromEntity.PrimaryKey, fromEntity.Table),
		fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $2)", toEntity.PrimaryKey, toEntity.Table),
	}
	params := []any{tb.subject.ID, tb.target.ID}

	// Add temporal filter if entities have temporal versioning
	if fromEntity.Temporal != nil {
		values[0] = fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $1 AND %s = 'infinity')",
			fromEntity.PrimaryKey, fromEntity.Table, fromEntity.Temporal.ValidToColumn)
	}
	if toEntity.Temporal != nil {
		values[1] = fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $2 AND %s = 'infinity')",
			toEntity.PrimaryKey, toEntity.Table, toEntity.Temporal.ValidToColumn)
	}

	// Add metadata columns
	paramIdx := 3
	for key, val := range tb.meta {
		columns = append(columns, key)
		values = append(values, fmt.Sprintf("$%d", paramIdx))
		params = append(params, val)
		paramIdx++
	}

	// Add temporal columns for temporal relationships (defaults handled by DB)
	// No explicit handling needed - valid_from defaults to now(), valid_to to 'infinity'

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		rel.JoinTable,
		strings.Join(columns, ", "),
		strings.Join(values, ", "))

	return tb.db.conn.ExecContext(ctx, query, params...)
}

// Delete removes the relationship tuple from the database.
// For temporal relationships, this soft-deletes by setting valid_to to now().
// For non-temporal relationships, this performs a hard delete.
func (tb *TupleBuilder) Delete(ctx context.Context) (sql.Result, error) {
	if tb.err != nil {
		return nil, tb.err
	}

	rel, err := tb.db.schema.FindRelationship(tb.subject.Type, tb.relation, "->")
	if err != nil {
		return nil, fmt.Errorf("relationship %s not found: %w", tb.relation, err)
	}

	fromEntity, err := tb.db.schema.GetEntity(tb.subject.Type)
	if err != nil {
		return nil, err
	}
	toEntity, err := tb.db.schema.GetEntity(tb.target.Type)
	if err != nil {
		return nil, err
	}

	// Build subquery for from entity
	fromSubquery := fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $1)",
		fromEntity.PrimaryKey, fromEntity.Table)
	if fromEntity.Temporal != nil {
		fromSubquery = fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $1 AND %s = 'infinity')",
			fromEntity.PrimaryKey, fromEntity.Table, fromEntity.Temporal.ValidToColumn)
	}

	// Build subquery for to entity
	toSubquery := fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $2)",
		toEntity.PrimaryKey, toEntity.Table)
	if toEntity.Temporal != nil {
		toSubquery = fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $2 AND %s = 'infinity')",
			toEntity.PrimaryKey, toEntity.Table, toEntity.Temporal.ValidToColumn)
	}

	var query string
	if rel.Temporal != nil {
		// Soft delete: set valid_to to now()
		query = fmt.Sprintf("UPDATE %s SET %s = now() WHERE %s = %s AND %s = %s AND %s = 'infinity'",
			rel.JoinTable, rel.Temporal.ValidToColumn,
			rel.FromKey, fromSubquery, rel.ToKey, toSubquery,
			rel.Temporal.ValidToColumn)
	} else {
		// Hard delete for non-temporal relationships
		query = fmt.Sprintf("DELETE FROM %s WHERE %s = %s AND %s = %s",
			rel.JoinTable, rel.FromKey, fromSubquery, rel.ToKey, toSubquery)
	}

	return tb.db.conn.ExecContext(ctx, query, tb.subject.ID, tb.target.ID)
}

// Exists checks if the relationship tuple exists.
// For temporal relationships, checks current state (valid_to = 'infinity') unless At() was called.
func (tb *TupleBuilder) Exists(ctx context.Context) (bool, error) {
	if tb.err != nil {
		return false, tb.err
	}

	rel, err := tb.db.schema.FindRelationship(tb.subject.Type, tb.relation, "->")
	if err != nil {
		return false, fmt.Errorf("relationship %s not found: %w", tb.relation, err)
	}

	fromEntity, err := tb.db.schema.GetEntity(tb.subject.Type)
	if err != nil {
		return false, err
	}
	toEntity, err := tb.db.schema.GetEntity(tb.target.Type)
	if err != nil {
		return false, err
	}

	params := []any{tb.subject.ID, tb.target.ID}

	// Build subquery for from entity - always use current version since entity IDs don't change
	fromSubquery := fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $1)",
		fromEntity.PrimaryKey, fromEntity.Table)
	if fromEntity.Temporal != nil {
		fromSubquery = fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $1 AND %s = 'infinity')",
			fromEntity.PrimaryKey, fromEntity.Table, fromEntity.Temporal.ValidToColumn)
	}

	// Build subquery for to entity - always use current version since entity IDs don't change
	toSubquery := fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $2)",
		toEntity.PrimaryKey, toEntity.Table)
	if toEntity.Temporal != nil {
		toSubquery = fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $2 AND %s = 'infinity')",
			toEntity.PrimaryKey, toEntity.Table, toEntity.Temporal.ValidToColumn)
	}

	// Build WHERE clause for the junction table
	whereClause := fmt.Sprintf("%s = %s AND %s = %s",
		rel.FromKey, fromSubquery, rel.ToKey, toSubquery)

	// Add temporal filter for the relationship itself
	if rel.Temporal != nil {
		if tb.validAt != nil {
			// Point-in-time query
			whereClause += fmt.Sprintf(" AND %s <= $3 AND %s > $3",
				rel.Temporal.ValidFromColumn, rel.Temporal.ValidToColumn)
			params = append(params, *tb.validAt)
		} else {
			// Current state
			whereClause += fmt.Sprintf(" AND %s = 'infinity'", rel.Temporal.ValidToColumn)
		}
	} else if tb.validAt != nil {
		params = append(params, *tb.validAt)
	}

	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s)",
		rel.JoinTable, whereClause)

	var exists bool
	err = tb.db.conn.QueryRowContext(ctx, query, params...).Scan(&exists)
	return exists, err
}

// LinkSpec defines a relationship to create along with the entity
type LinkSpec struct {
	Relation string
	Target   string
	Meta     map[string]any
}

// EntityBuilder builds entity records using the pattern:
// type:id with fields
//
// Example:
//
//	db.Entity("user:alice").
//	    Set("email", "alice@example.com").
//	    Set("status", "ACTIVE").
//	    Create(ctx)
//
// With chained links:
//
//	db.Entity("user:alice").
//	    Set("email", "alice@example.com").
//	    Link("member_of", "group:admins").WithMeta("role", "admin").
//	    Link("member_of", "group:devs").
//	    Create(ctx)
type EntityBuilder struct {
	db       *DB
	ref      Ref
	fields   map[string]any
	rawData  map[string]any // For JSONB metadata column
	links    []*LinkBuilder
	version  *int
	validAt  *time.Time
	err      error
}

// LinkBuilder builds a link specification for chaining with EntityBuilder
type LinkBuilder struct {
	parent   *EntityBuilder
	relation string
	target   string
	meta     map[string]any
}

// EntityResult is returned after creating an entity, allowing chained operations
type EntityResult struct {
	db     *DB
	ref    Ref
	result sql.Result
	err    error
}

// Link creates a relationship from the newly created entity
func (er *EntityResult) Link(relation, target string) *TupleBuilder {
	if er.err != nil {
		return &TupleBuilder{db: er.db, err: er.err}
	}
	return er.db.Tuple(er.ref.String(), relation, target)
}

// Result returns the underlying sql.Result
func (er *EntityResult) Result() (sql.Result, error) {
	return er.result, er.err
}

// Err returns any error from the creation
func (er *EntityResult) Err() error {
	return er.err
}

// Entity creates a new entity builder.
//
// Example:
//
//	db.Entity("user:alice")
func (db *DB) Entity(ref string) *EntityBuilder {
	eb := &EntityBuilder{
		db:      db,
		fields:  make(map[string]any),
		rawData: make(map[string]any),
		links:   []*LinkBuilder{},
	}

	r, err := ParseRef(ref)
	if err != nil {
		eb.err = err
		return eb
	}
	eb.ref = r

	return eb
}

// Set adds a field value to the entity.
//
// Example:
//
//	db.Entity("user:alice").
//	    Set("email", "alice@example.com").
//	    Set("first_name", "Alice")
func (eb *EntityBuilder) Set(field string, value any) *EntityBuilder {
	if eb.err != nil {
		return eb
	}
	eb.fields[field] = value
	return eb
}

// Version sets a specific version number for temporal entities.
func (eb *EntityBuilder) Version(v int) *EntityBuilder {
	if eb.err != nil {
		return eb
	}
	eb.version = &v
	return eb
}

// At sets a specific timestamp for temporal entities.
func (eb *EntityBuilder) At(t time.Time) *EntityBuilder {
	if eb.err != nil {
		return eb
	}
	eb.validAt = &t
	return eb
}

// Meta sets a key-value pair in the raw_data JSONB column.
// Use this for arbitrary metadata that doesn't have a dedicated column.
//
// Example:
//
//	db.Entity("user:alice").
//	    Set("email", "alice@example.com").
//	    Meta("preferences", map[string]any{"theme": "dark"}).
//	    Meta("tags", []string{"admin", "power-user"}).
//	    Create(ctx)
func (eb *EntityBuilder) Meta(key string, value any) *EntityBuilder {
	if eb.err != nil {
		return eb
	}
	eb.rawData[key] = value
	return eb
}

// RawData sets the entire raw_data JSONB column at once.
//
// Example:
//
//	db.Entity("user:alice").
//	    RawData(map[string]any{"source": "okta", "original_id": "123"}).
//	    Create(ctx)
func (eb *EntityBuilder) RawData(data map[string]any) *EntityBuilder {
	if eb.err != nil {
		return eb
	}
	eb.rawData = data
	return eb
}

// Link adds a relationship to be created along with the entity.
// Returns a LinkBuilder for adding metadata to the relationship.
//
// Example:
//
//	db.Entity("user:alice").
//	    Set("email", "alice@example.com").
//	    Link("member_of", "group:admins").WithMeta("role", "admin").
//	    Link("member_of", "group:devs").WithMeta("role", "member").
//	    Create(ctx)
func (eb *EntityBuilder) Link(relation, target string) *LinkBuilder {
	lb := &LinkBuilder{
		parent:   eb,
		relation: relation,
		target:   target,
		meta:     make(map[string]any),
	}
	eb.links = append(eb.links, lb)
	return lb
}

// WithMeta adds metadata to the link and returns the parent EntityBuilder.
func (lb *LinkBuilder) WithMeta(key string, value any) *LinkBuilder {
	lb.meta[key] = value
	return lb
}

// Link adds another relationship (convenience method to chain from LinkBuilder).
func (lb *LinkBuilder) Link(relation, target string) *LinkBuilder {
	return lb.parent.Link(relation, target)
}

// Set continues setting fields on the entity (chain back from LinkBuilder).
func (lb *LinkBuilder) Set(field string, value any) *EntityBuilder {
	return lb.parent.Set(field, value)
}

// Create creates the entity (chain from LinkBuilder).
func (lb *LinkBuilder) Create(ctx context.Context) *EntityResult {
	return lb.parent.Create(ctx)
}

// Create inserts the entity and any chained links into the database.
// Uses a transaction when links are present to ensure atomicity.
func (eb *EntityBuilder) Create(ctx context.Context) *EntityResult {
	result := &EntityResult{db: eb.db, ref: eb.ref}

	if eb.err != nil {
		result.err = eb.err
		return result
	}

	entity, err := eb.db.schema.GetEntity(eb.ref.Type)
	if err != nil {
		result.err = err
		return result
	}

	// Build column list - always include external_id
	columns := []string{"external_id"}
	placeholders := []string{"$1"}
	params := []any{eb.ref.ID}
	paramIdx := 2

	// Add user-provided fields
	for field, value := range eb.fields {
		colName := field
		if mapped, ok := entity.Fields[field]; ok {
			colName = mapped
		}
		columns = append(columns, colName)
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
		params = append(params, value)
		paramIdx++
	}

	// Add raw_data JSONB if present
	if len(eb.rawData) > 0 && entity.Temporal != nil {
		jsonData, err := json.Marshal(eb.rawData)
		if err != nil {
			result.err = fmt.Errorf("marshaling raw_data: %w", err)
			return result
		}
		columns = append(columns, entity.Temporal.RawDataColumn)
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
		params = append(params, string(jsonData))
		paramIdx++
	}

	// Add version if specified and entity is temporal
	if entity.Temporal != nil && eb.version != nil {
		columns = append(columns, entity.Temporal.VersionColumn)
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
		params = append(params, *eb.version)
		paramIdx++
	}

	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		entity.Table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	// If no links, simple insert
	if len(eb.links) == 0 {
		result.result, result.err = eb.db.conn.ExecContext(ctx, insertSQL, params...)
		return result
	}

	// Use transaction for entity + links
	tx, err := eb.db.conn.BeginTx(ctx, nil)
	if err != nil {
		result.err = fmt.Errorf("starting transaction: %w", err)
		return result
	}
	defer tx.Rollback()

	// Insert entity
	_, err = tx.ExecContext(ctx, insertSQL, params...)
	if err != nil {
		result.err = fmt.Errorf("inserting entity: %w", err)
		return result
	}

	// Create each link
	for _, link := range eb.links {
		tb := eb.db.Tuple(eb.ref.String(), link.relation, link.target)
		for k, v := range link.meta {
			tb.With(k, v)
		}

		// Build the tuple insert SQL (reuse logic from TupleBuilder)
		rel, err := eb.db.schema.FindRelationship(tb.subject.Type, tb.relation, "->")
		if err != nil {
			result.err = fmt.Errorf("relationship %s not found: %w", tb.relation, err)
			return result
		}

		fromEntity, _ := eb.db.schema.GetEntity(tb.subject.Type)
		toEntity, _ := eb.db.schema.GetEntity(tb.target.Type)

		linkCols := []string{rel.FromKey, rel.ToKey}
		linkVals := []string{
			fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $1 AND %s = 'infinity')",
				fromEntity.PrimaryKey, fromEntity.Table, fromEntity.Temporal.ValidToColumn),
			fmt.Sprintf("(SELECT %s FROM %s WHERE external_id = $2 AND %s = 'infinity')",
				toEntity.PrimaryKey, toEntity.Table, toEntity.Temporal.ValidToColumn),
		}
		linkParams := []any{tb.subject.ID, tb.target.ID}

		linkParamIdx := 3
		for key, val := range tb.meta {
			linkCols = append(linkCols, key)
			linkVals = append(linkVals, fmt.Sprintf("$%d", linkParamIdx))
			linkParams = append(linkParams, val)
			linkParamIdx++
		}

		linkSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			rel.JoinTable,
			strings.Join(linkCols, ", "),
			strings.Join(linkVals, ", "))

		_, err = tx.ExecContext(ctx, linkSQL, linkParams...)
		if err != nil {
			result.err = fmt.Errorf("creating link %s->%s: %w", link.relation, link.target, err)
			return result
		}
	}

	if err := tx.Commit(); err != nil {
		result.err = fmt.Errorf("committing transaction: %w", err)
		return result
	}

	return result
}

// Update modifies an existing entity.
func (eb *EntityBuilder) Update(ctx context.Context) (sql.Result, error) {
	if eb.err != nil {
		return nil, eb.err
	}

	entity, err := eb.db.schema.GetEntity(eb.ref.Type)
	if err != nil {
		return nil, err
	}

	if len(eb.fields) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Build SET clause
	setClauses := []string{}
	params := []any{}
	paramIdx := 1

	for field, value := range eb.fields {
		colName := field
		if mapped, ok := entity.Fields[field]; ok {
			colName = mapped
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", colName, paramIdx))
		params = append(params, value)
		paramIdx++
	}

	// Build WHERE clause
	whereClause := fmt.Sprintf("external_id = $%d", paramIdx)
	params = append(params, eb.ref.ID)

	// Add temporal filter for current version
	if entity.Temporal != nil {
		whereClause += fmt.Sprintf(" AND %s = 'infinity'", entity.Temporal.ValidToColumn)
	}

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		entity.Table,
		strings.Join(setClauses, ", "),
		whereClause)

	return eb.db.conn.ExecContext(ctx, sql, params...)
}

// Delete removes the entity from the database.
// For temporal entities, this sets valid_to to now() instead of hard delete.
func (eb *EntityBuilder) Delete(ctx context.Context) (sql.Result, error) {
	if eb.err != nil {
		return nil, eb.err
	}

	entity, err := eb.db.schema.GetEntity(eb.ref.Type)
	if err != nil {
		return nil, err
	}

	var sql string
	var params []any

	if entity.Temporal != nil {
		// Soft delete for temporal entities
		sql = fmt.Sprintf("UPDATE %s SET %s = now() WHERE external_id = $1 AND %s = 'infinity'",
			entity.Table, entity.Temporal.ValidToColumn, entity.Temporal.ValidToColumn)
		params = []any{eb.ref.ID}
	} else {
		// Hard delete for non-temporal entities
		sql = fmt.Sprintf("DELETE FROM %s WHERE external_id = $1", entity.Table)
		params = []any{eb.ref.ID}
	}

	return eb.db.conn.ExecContext(ctx, sql, params...)
}

// Exists checks if the entity exists.
func (eb *EntityBuilder) Exists(ctx context.Context) (bool, error) {
	if eb.err != nil {
		return false, eb.err
	}

	entity, err := eb.db.schema.GetEntity(eb.ref.Type)
	if err != nil {
		return false, err
	}

	whereClause := "external_id = $1"
	if entity.Temporal != nil {
		whereClause += fmt.Sprintf(" AND %s = 'infinity'", entity.Temporal.ValidToColumn)
	}

	sql := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s)", entity.Table, whereClause)

	var exists bool
	err = eb.db.conn.QueryRowContext(ctx, sql, eb.ref.ID).Scan(&exists)
	return exists, err
}
