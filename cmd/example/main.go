package main

import (
	"fmt"
	"os"

	"github.com/bresrch/doodle"
)

func main() {
	// Set up schema
	schema := doodle.NewSchema()

	schema.AddEntity("user", "users", "id").
		AddField("email", "email").
		AddField("status", "status").
		AddField("provider", "provider")

	schema.AddEntity("group", "groups", "id").
		AddField("name", "name").
		AddField("description", "description")

	schema.AddEntity("app", "apps", "id").
		AddField("name", "name").
		AddField("app_type", "app_type")

	schema.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")
	schema.AddRelationship("has_access", "group", "app", "group_apps", "group_id", "app_id")

	db := doodle.New(schema)

	// Example queries
	queries := []string{
		// Outgoing edges
		"SELECT * FROM user:okta_123",
		"SELECT ->member_of->group FROM user:okta_123",
		"SELECT ->member_of->group->has_access->app FROM user:okta_123",

		// Incoming edges (reverse traversal)
		"SELECT <-member_of<-user FROM group:admins",
		"SELECT <-has_access<-group<-member_of<-user FROM app:slack",

		// With temporal, where, limit
		"SELECT * FROM user:okta_123 VERSION d'2024-01-01T00:00:00Z'",
		"SELECT * FROM user:okta_123 WHERE status = 'ACTIVE'",
		"SELECT ->member_of->group->has_access->app FROM user:okta_123 WHERE status = 'ACTIVE' LIMIT 10",
	}

	for _, q := range queries {
		fmt.Printf("Doodle: %s\n", q)

		result, err := db.Compile(q)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			continue
		}

		fmt.Printf("SQL:    %s\n", result.SQL)
		fmt.Printf("Params: %v\n\n", result.Params)
	}
}
