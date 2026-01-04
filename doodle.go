// Package doodle provides a graph-aware query DSL that transpiles to PostgreSQL.
//
// Doodle enables SurrealDB-like graph traversal syntax while targeting PostgreSQL
// with temporal table support.
//
// Example usage:
//
//	schema := doodle.NewSchema()
//	schema.AddEntity("user", "users", "id")
//	schema.AddEntity("group", "groups", "id")
//	schema.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")
//
//	db := doodle.New(schema)
//	result, err := db.Compile("SELECT ->member_of->group FROM user:okta_123")
//	// result.SQL: SELECT t1.* FROM users t0 JOIN user_groups j0 ON ...
//	// result.Params: ["okta_123"]
package doodle

import (
	"context"
	"database/sql"
)

// DB wraps a PostgreSQL connection with doodle query support
type DB struct {
	schema    *Schema
	generator *Generator
	conn      *sql.DB
}

// New creates a new doodle DB wrapper
func New(schema *Schema) *DB {
	return &DB{
		schema:    schema,
		generator: NewGenerator(schema),
	}
}

// Connect establishes connection to PostgreSQL
func (db *DB) Connect(connStr string) error {
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	db.conn = conn
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Compile parses and transpiles a doodle query without executing
func (db *DB) Compile(query string) (*GeneratedQuery, error) {
	ast, err := Parse(query)
	if err != nil {
		return nil, err
	}
	return db.generator.Generate(ast)
}

// Query executes a doodle query and returns rows
func (db *DB) Query(ctx context.Context, query string) (*sql.Rows, error) {
	compiled, err := db.Compile(query)
	if err != nil {
		return nil, err
	}
	return db.conn.QueryContext(ctx, compiled.SQL, compiled.Params...)
}

// QueryRow executes a doodle query and returns a single row
func (db *DB) QueryRow(ctx context.Context, query string) (*sql.Row, error) {
	compiled, err := db.Compile(query)
	if err != nil {
		return nil, err
	}
	return db.conn.QueryRowContext(ctx, compiled.SQL, compiled.Params...), nil
}

// Exec executes a doodle query that doesn't return rows
func (db *DB) Exec(ctx context.Context, query string) (sql.Result, error) {
	compiled, err := db.Compile(query)
	if err != nil {
		return nil, err
	}
	return db.conn.ExecContext(ctx, compiled.SQL, compiled.Params...)
}

// Schema returns the schema for modification
func (db *DB) Schema() *Schema {
	return db.schema
}

// WithConnection sets an existing database connection
func (db *DB) WithConnection(conn *sql.DB) *DB {
	db.conn = conn
	return db
}
