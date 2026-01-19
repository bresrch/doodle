# Doodle

A graph-aware query DSL that transpiles to PostgreSQL. Write SurrealDB-style graph traversals, get safe parameterized SQL.

## Installation

```bash
go get github.com/bresrch/doodle
```

## Quick Start

```go
package main

import (
    "context"
    "github.com/bresrch/doodle"
    _ "github.com/lib/pq"
)

func main() {
    // Define schema with fluent API
    schema := doodle.NewSchema()
    schema.AddEntity("user", "users", "id").WithTemporal()
    schema.AddEntity("group", "groups", "id").WithTemporal()

    schema.Relationship("member_of").
        From("user").To("group").
        Via("user_groups", "user_id", "group_id")

    // Create doodle instance
    db := doodle.New(schema)
    db.Connect("postgres://user:pass@localhost:5432/mydb?sslmode=disable")
    defer db.Close()

    // Execute graph query
    rows, _ := db.Query(context.Background(),
        "SELECT ->member_of->group FROM user:alice")
}
```

Or load schema from YAML:

```go
schema, _ := doodle.LoadSchemaFromFile("schema.yaml")
db := doodle.New(schema)
```

## Schema Definition

### Entities

Entities map to database tables:

```go
schema.AddEntity("user", "users", "id")
//                 │        │       └── primary key column
//                 │        └── database table name
//                 └── entity name (used in queries)
```

Optional field mappings for WHERE clause validation:

```go
schema.AddEntity("user", "users", "id").
    AddField("email", "email").
    AddField("status", "status").
    AddField("name", "display_name")  // query field → db column
```

### Temporal Entities

Enable temporal versioning for entities with history tracking:

```go
schema.AddEntity("user", "users", "id").
    WithTemporal().  // enables temporal queries
    AddField("email", "email").
    AddField("status", "status")
```

Temporal entities expect these columns in the database:
- `version` (INT) - version number, incremented on each change
- `valid_from` (TIMESTAMPTZ) - when this version became active
- `valid_to` (TIMESTAMPTZ) - when this version expired (`'infinity'` for current)
- `raw_data` (JSONB) - optional, full API response storage

### Relationships

Relationships define graph edges via junction tables. Use the fluent builder for readable definitions:

```go
schema.Relationship("member_of").
    From("user").To("group").
    Via("user_groups", "user_id", "group_id")
```

Or the positional API:

```go
schema.AddRelationship("member_of", "user", "group", "user_groups", "user_id", "group_id")
//                          │         │       │           │            │          └── to_key
//                          │         │       │           │            └── from_key
//                          │         │       │           └── junction table
//                          │         │       └── to entity
//                          │         └── from entity
//                          └── relationship name (query alias)
```

The relationship name (`member_of`) is a **logical alias** used in queries - it maps to the junction table, not a column within it.

### Real-World Identity Examples

**User → Device (enrolled devices, MFA tokens)**
```go
schema.Relationship("uses").
    From("user").To("device").
    Via("user_devices", "user_id", "device_id")
```
```sql
SELECT ->uses->device FROM user:alice              -- devices alice uses
SELECT <-uses<-user FROM device:yubikey_abc123     -- who uses this YubiKey
```

**User → Factor (authentication factors)**
```go
schema.Relationship("enrolled").
    From("user").To("factor").
    Via("user_factors", "user_id", "factor_id")
```
```sql
SELECT ->enrolled->factor FROM user:alice          -- alice's enrolled factors
```

**App → SignOnPolicy (app security requirements)**
```go
schema.Relationship("governed_by").
    From("app").To("policy").
    Via("app_policies", "app_id", "policy_id")
```
```sql
SELECT ->governed_by->policy FROM app:slack        -- policies for Slack
```

**Policy → FactorType (required factors)**
```go
schema.Relationship("requires").
    From("policy").To("factor_type").
    Via("policy_requirements", "policy_id", "factor_type_id")
```
```sql
-- What factor types are required to access Slack?
SELECT ->governed_by->policy->requires->factor_type FROM app:slack
```

### Multiple Relationship Types

If you need multiple relationship types between the same entities (e.g., users can be `member_of` OR `admin_of` groups), use separate junction tables:

```sql
CREATE TABLE user_group_members (
    user_id UUID REFERENCES users(id),
    group_id UUID REFERENCES groups(id),
    PRIMARY KEY (user_id, group_id)
);

CREATE TABLE user_group_admins (
    user_id UUID REFERENCES users(id),
    group_id UUID REFERENCES groups(id),
    PRIMARY KEY (user_id, group_id)
);
```

```go
schema.Relationship("member_of").
    From("user").To("group").
    Via("user_group_members", "user_id", "group_id")

schema.Relationship("admin_of").
    From("user").To("group").
    Via("user_group_admins", "user_id", "group_id")
```

Now you can query each relationship type separately:

```sql
SELECT ->member_of->group FROM user:alice   -- groups alice is a member of
SELECT ->admin_of->group FROM user:alice    -- groups alice is an admin of
```

### Database Schema Example

```sql
-- Users table with temporal versioning
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT NOT NULL,
    email TEXT,
    status TEXT DEFAULT 'ACTIVE',
    provider TEXT NOT NULL,

    -- Full API response storage
    raw_data JSONB,

    -- Temporal versioning columns
    version INT NOT NULL DEFAULT 1,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ NOT NULL DEFAULT 'infinity',

    UNIQUE(external_id, version)
);

CREATE TABLE groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    provider TEXT NOT NULL,

    raw_data JSONB,
    version INT NOT NULL DEFAULT 1,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ NOT NULL DEFAULT 'infinity',

    UNIQUE(external_id, version)
);

CREATE TABLE apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    provider TEXT NOT NULL,

    raw_data JSONB,
    version INT NOT NULL DEFAULT 1,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ NOT NULL DEFAULT 'infinity',

    UNIQUE(external_id, version)
);

-- Junction tables for relationships
CREATE TABLE user_groups (
    user_id UUID REFERENCES users(id),
    group_id UUID REFERENCES groups(id),
    PRIMARY KEY (user_id, group_id)
);

CREATE TABLE group_apps (
    group_id UUID REFERENCES groups(id),
    app_id UUID REFERENCES apps(id),
    PRIMARY KEY (group_id, app_id)
);

-- Recommended indexes for temporal queries
CREATE INDEX idx_users_current ON users(external_id) WHERE valid_to = 'infinity';
CREATE INDEX idx_groups_current ON groups(external_id) WHERE valid_to = 'infinity';
CREATE INDEX idx_apps_current ON apps(external_id) WHERE valid_to = 'infinity';
```

Corresponding schema definition:

```go
schema := doodle.NewSchema()

schema.AddEntity("user", "users", "id").
    WithTemporal().
    AddField("email", "email").
    AddField("status", "status").
    AddField("provider", "provider")

schema.AddEntity("group", "groups", "id").
    WithTemporal().
    AddField("name", "name")

schema.AddEntity("app", "apps", "id").
    WithTemporal().
    AddField("name", "name")

schema.Relationship("member_of").
    From("user").To("group").
    Via("user_groups", "user_id", "group_id")

schema.Relationship("has_access").
    From("group").To("app").
    Via("group_apps", "group_id", "app_id")
```

### Loading Schema from YAML

Define your schema declaratively:

```yaml
# schema.yaml
entities:
  user:
    table: users
    primary_key: id
    temporal: true
    fields:
      email: email
      status: status
      provider: provider

  device:
    table: devices
    primary_key: id
    temporal: true
    fields:
      name: device_name
      type: device_type

  factor:
    table: factors
    primary_key: id
    fields:
      type: factor_type
      status: status

relationships:
  uses:
    from: user
    to: device
    table: user_devices
    from_key: user_id
    to_key: device_id

  enrolled:
    from: user
    to: factor
    table: user_factors
    from_key: user_id
    to_key: factor_id
```

Load in Go:

```go
schema, err := doodle.LoadSchemaFromFile("schema.yaml")
if err != nil {
    log.Fatal(err)
}

db := doodle.New(schema)
```

### Loading Schema from JSON

```json
{
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
}
```

```go
schema, err := doodle.LoadSchemaFromJSON(jsonBytes)
// or auto-detect format:
schema, err := doodle.LoadSchema(data) // works with JSON or YAML
```

### Provider-Based Schema Registration

In plugin architectures (like Bresearch providers), each provider can register its own entities and relationships. This allows providers to extend the schema with domain-specific data models.

```go
// Core schema with base entities
schema := doodle.NewSchema()
schema.AddEntity("user", "users", "id").WithTemporal()
schema.AddEntity("group", "groups", "id").WithTemporal()
schema.AddEntity("app", "apps", "id").WithTemporal()

// Provider: Okta - registers identity relationships
func (p *OktaProvider) RegisterSchema(schema *doodle.Schema) {
    schema.Relationship("member_of").
        From("user").To("group").
        Via("okta_user_groups", "user_id", "group_id")

    schema.Relationship("assigned_to").
        From("user").To("app").
        Via("okta_user_apps", "user_id", "app_id")
}

// Provider: CrowdStrike - registers device/security relationships
func (p *CrowdStrikeProvider) RegisterSchema(schema *doodle.Schema) {
    schema.AddEntity("device", "devices", "id").
        WithTemporal().
        AddField("hostname", "hostname").
        AddField("os", "operating_system")

    schema.AddEntity("vulnerability", "vulnerabilities", "id").
        AddField("cve", "cve_id").
        AddField("severity", "severity")

    schema.Relationship("uses").
        From("user").To("device").
        Via("user_devices", "user_id", "device_id")

    schema.Relationship("has_vulnerability").
        From("device").To("vulnerability").
        Via("device_vulnerabilities", "device_id", "vulnerability_id")
}

// Provider: Okta (MFA) - registers factor relationships
func (p *OktaMFAProvider) RegisterSchema(schema *doodle.Schema) {
    schema.AddEntity("factor", "factors", "id").
        AddField("type", "factor_type").
        AddField("status", "status")

    schema.AddEntity("policy", "policies", "id").
        AddField("name", "name").
        AddField("type", "policy_type")

    schema.AddEntity("factor_type", "factor_types", "id").
        AddField("name", "name")

    schema.Relationship("enrolled").
        From("user").To("factor").
        Via("user_factors", "user_id", "factor_id")

    schema.Relationship("governed_by").
        From("app").To("policy").
        Via("app_policies", "app_id", "policy_id")

    schema.Relationship("requires").
        From("policy").To("factor_type").
        Via("policy_requirements", "policy_id", "factor_type_id")
}
```

After providers register their schemas, you can query across all providers:

```sql
-- Find all devices a user has with vulnerabilities
SELECT ->uses->device->has_vulnerability->vulnerability FROM user:alice

-- What MFA factors are required for Slack access?
SELECT ->governed_by->policy->requires->factor_type FROM app:slack

-- Users with enrolled YubiKeys who have vulnerable devices
SELECT <-enrolled<-user->uses->device->has_vulnerability->vulnerability
FROM factor WHERE type = 'hardware_token'
```

Each provider can also supply its schema via YAML embedded in the provider binary or configuration:

```go
//go:embed schema.yaml
var schemaYAML []byte

func (p *OktaProvider) RegisterSchema(schema *doodle.Schema) error {
    providerSchema, err := doodle.LoadSchemaFromYAML(schemaYAML)
    if err != nil {
        return err
    }

    // Merge provider schema into main schema
    for name, entity := range providerSchema.Entities {
        schema.Entities[name] = entity
    }
    for key, rel := range providerSchema.Relationships {
        schema.Relationships[key] = rel
    }
    return nil
}
```

## Query Syntax

### Basic Select

```sql
SELECT * FROM user:alice
```

### Field Selection

Select specific fields instead of all columns:

```sql
SELECT email, status FROM user:alice

-- With table prefix
SELECT user.email, user.status FROM user:alice

-- With aliases
SELECT email AS e, status AS s FROM user:alice
```

### DISTINCT

Remove duplicate rows from results:

```sql
-- Distinct rows
SELECT DISTINCT status FROM user:alice

-- Distinct with multiple fields
SELECT DISTINCT status, provider FROM user:alice

-- Count distinct values
SELECT COUNT(DISTINCT status) FROM user:alice
```

### Aggregations

Use aggregate functions for counting and calculations:

```sql
-- Count all records
SELECT COUNT(*) FROM user:alice

-- Count specific field
SELECT COUNT(email) FROM user:alice

-- Count traversal results (how many groups is alice in?)
SELECT COUNT(->member_of->group) FROM user:alice

-- Other aggregates
SELECT SUM(score) FROM user:alice
SELECT AVG(score) FROM user:alice
SELECT MIN(created_at) FROM user:alice
SELECT MAX(created_at) FROM user:alice
```

### GROUP BY and HAVING

Group results and filter groups:

```sql
-- Group by single field
SELECT status, COUNT(*) FROM user:alice GROUP BY status

-- Group by multiple fields
SELECT status, provider, COUNT(*) FROM user:alice GROUP BY status, provider

-- Filter groups with HAVING
SELECT status, COUNT(*) FROM user:alice GROUP BY status HAVING COUNT(*) > 5

-- HAVING with other aggregates
SELECT provider, AVG(score) FROM user:alice GROUP BY provider HAVING AVG(score) >= 80
```

### Outgoing Edges (->)

Traverse from source to target:

```sql
-- Get groups that alice is a member of
SELECT ->member_of->group FROM user:alice

-- Multi-hop: get apps alice can access through groups
SELECT ->member_of->group->has_access->app FROM user:alice
```

### Incoming Edges (<-)

Traverse in reverse direction:

```sql
-- Get users who are members of the admins group
SELECT <-member_of<-user FROM group:admins

-- Get all users who can access slack (through any group)
SELECT <-has_access<-group<-member_of<-user FROM app:slack
```

### Temporal Queries (VERSION)

Query historical state using timestamps or version numbers:

```sql
-- Query by timestamp (returns state at that point in time)
SELECT * FROM user:alice VERSION d'2024-01-01T00:00:00Z'

-- Query by version number
SELECT * FROM user:alice VERSION 3

-- Temporal traversals
SELECT ->member_of->group FROM user:alice VERSION d'2024-06-15T12:00:00Z'
```

Without VERSION clause, doodle automatically queries current state (`valid_to = 'infinity'`).

#### Temporal Query Behavior

| Query Type | Generated SQL |
|------------|---------------|
| No VERSION | `WHERE ... AND valid_to = 'infinity'` |
| `VERSION d'2024-01-01...'` | `WHERE ... AND valid_from <= $n AND valid_to > $n` |
| `VERSION 3` | `WHERE ... AND version = $n` |

### Filtering (WHERE)

```sql
SELECT * FROM user:alice WHERE status = 'ACTIVE'

SELECT ->member_of->group FROM user:alice WHERE status = 'ACTIVE' AND provider = 'okta'

-- IN operator
SELECT * FROM user:alice WHERE status IN ('ACTIVE', 'PENDING')

-- BETWEEN operator
SELECT * FROM user:alice WHERE score BETWEEN 10 AND 100

-- OR conditions
SELECT * FROM user:alice WHERE status = 'ACTIVE' OR status = 'PENDING'

-- Complex AND/OR (AND binds tighter than OR)
SELECT * FROM user:alice WHERE status = 'ACTIVE' AND provider = 'okta' OR status = 'PENDING'
-- Generates: ((status = 'ACTIVE' AND provider = 'okta') OR status = 'PENDING')
```

### Ordering Results

```sql
SELECT * FROM user:alice ORDER BY email DESC

-- Multiple fields
SELECT * FROM user:alice ORDER BY status ASC, email DESC

-- Default is ASC
SELECT * FROM user:alice ORDER BY created_at
```

### Pagination (LIMIT/OFFSET)

```sql
SELECT ->member_of->group FROM user:alice LIMIT 10

-- With offset for pagination
SELECT * FROM user:alice LIMIT 10 OFFSET 20
```

### Combined

```sql
-- Full query with all clauses
SELECT ->member_of->group->has_access->app
FROM user:alice
VERSION d'2024-01-01T00:00:00Z'
WHERE status = 'ACTIVE'
ORDER BY name ASC
LIMIT 100
OFFSET 0

-- Field selection with ordering
SELECT email, status FROM user:alice ORDER BY email DESC LIMIT 10

-- Aggregation with traversal
SELECT COUNT(->member_of->group) FROM user:alice WHERE status = 'ACTIVE'
```

### Subqueries

Use subqueries in FROM or WHERE clauses:

```sql
-- Subquery in FROM clause
SELECT * FROM (SELECT * FROM user:alice WHERE status = 'ACTIVE') AS active_users

-- Subquery in WHERE IN clause
SELECT * FROM user:alice WHERE id IN (SELECT user_id FROM active_sessions)

-- Subquery with traversal
SELECT * FROM user:alice WHERE id IN (SELECT <-member_of<-user FROM group:admins)
```

### Optional Traversals (`->?` / `<?-`)

Use optional traversals for LEFT JOIN semantics (returns NULL when no relationship exists):

```sql
-- Get all users and their groups (NULL if user has no groups)
SELECT ->?member_of->group FROM user

-- Incoming optional traversal
SELECT <?-member_of<-user FROM group:admins
```

| Traversal | SQL Join Type |
|-----------|---------------|
| `->` | INNER JOIN |
| `->?` | LEFT JOIN |

### Negative Paths (`->!` / `<!-`)

Find entities that do NOT have a specific relationship:

```sql
-- Find users who are NOT members of the admins group
SELECT * FROM user WHERE ->!member_of->group:admins

-- Find groups with no users
SELECT * FROM group WHERE <!-member_of<-user
```

Generated as `NOT EXISTS` subqueries for correctness.

### Path Field Access

Access fields on junction tables during traversal:

```sql
-- Get the role field from the junction table
SELECT ->member_of.role->group FROM user:alice

-- Multiple traversals with junction fields
SELECT ->member_of.role->group->has_access.permission->app FROM user:alice
```

### Variable-Length Paths (`->{n,m}->`)

Traverse relationships with variable hop counts using recursive CTEs. This is essential for hierarchical data like org structures, nested groups, or folder trees.

#### Syntax Variants

| Syntax | Meaning |
|--------|---------|
| `->rel{3}->` | Exactly 3 hops |
| `->rel{1,5}->` | Between 1 and 5 hops (inclusive) |
| `->rel{2,}->` | 2 or more hops (unbounded) |

#### Basic Examples

```sql
-- Find all managers 1-3 levels up
SELECT ->reports_to{1,3}->user FROM user:alice

-- Exact hop count (exactly 2 levels)
SELECT ->reports_to{2}->user FROM user:alice

-- Find all descendants up to 5 levels deep
SELECT <-reports_to{1,5}<-user FROM user:ceo
```

#### Group Hierarchy Example

For nested group structures (e.g., "Engineering Team" → "All Engineering" → "Company Wide"):

```go
// Define self-referential relationship for group nesting
schema.Relationship("child_of").
    From("group").To("group").
    Via("group_hierarchy", "child_group_id", "parent_group_id")
```

```sql
-- Find all ancestor groups up to 10 levels deep
SELECT ->child_of{1,10}->group FROM group:engineering_team

-- Mixed path: find all ancestor groups for groups a user belongs to
SELECT ->member_of->group->child_of{1,10}->group FROM user:alice
```

The mixed path query first traverses from user to their direct groups (non-recursive), then recursively climbs the group hierarchy.

#### Generated SQL

Variable-length paths generate PostgreSQL recursive CTEs:

```sql
-- Doodle: SELECT ->child_of{1,3}->group FROM group:engineering_team

WITH RECURSIVE path_cte AS (
    -- Base case: direct relationships (depth 1)
    SELECT t1.id, 1 AS depth
    FROM groups t0
    JOIN group_hierarchy j0 ON t0.id = j0.child_group_id
    JOIN groups t1 ON j0.parent_group_id = t1.id
    WHERE t0.external_id = $1 AND t0.valid_to = 'infinity'
      AND t1.valid_to = 'infinity'

    UNION ALL

    -- Recursive case: traverse deeper
    SELECT t1.id, p.depth + 1
    FROM path_cte p
    JOIN group_hierarchy j0 ON p.id = j0.child_group_id
    JOIN groups t1 ON j0.parent_group_id = t1.id
    WHERE p.depth < 3  -- maxHops constraint
      AND t1.valid_to = 'infinity'
)
SELECT DISTINCT t1.*
FROM path_cte p
JOIN groups t1 ON p.id = t1.id
WHERE p.depth >= 1  -- minHops constraint
```

#### Schema Requirements

Variable-length paths require a **self-referential relationship** where from and to entities are the same type:

```go
// User hierarchy (reports_to)
schema.AddRelationship("reports_to", "user", "user", "user_managers", "user_id", "manager_id")

// Group hierarchy (child_of)
schema.AddRelationship("child_of", "group", "group", "group_hierarchy", "child_group_id", "parent_group_id")

// Folder hierarchy (parent)
schema.AddRelationship("parent", "folder", "folder", "folder_tree", "folder_id", "parent_folder_id")
```

#### Performance Notes

- Recursive CTEs are powerful but can be expensive on deeply nested or wide hierarchies
- The `depth` tracking ensures queries terminate even with cycles
- Consider adding indexes on junction table foreign keys for large datasets
- `DISTINCT` in the final select prevents duplicate results from multiple paths to the same node

### Wildcard Relationship Traversal (`->*->`)

Use `*` as a wildcard to traverse any relationship without specifying a name. This is useful for exploring graph connectivity or when you don't know which relationships exist.

#### Single-Hop Wildcard

```sql
-- Find all entities connected to alice via any outgoing relationship
SELECT ->*->* FROM user:alice

-- Find all groups connected to alice via any relationship
SELECT ->*->group FROM user:alice

-- Find all users connected to admins via any incoming relationship
SELECT <-*<-user FROM group:admins
```

The wildcard generates a UNION of all matching relationships. Each result includes an `_relationship` column showing which relationship was traversed.

#### Recursive Wildcard

Combine wildcards with quantifiers to explore multi-hop paths through any relationships:

```sql
-- Find all users reachable within 3 hops via any relationship
SELECT ->*{1,3}->user FROM user:alice

-- Explore all connected entities up to 5 hops
SELECT ->*{1,5}->* FROM user:alice
```

This generates a recursive CTE that tracks `entity_type` as it traverses, allowing exploration of heterogeneous graphs.

#### Example: Who Can Reach This Resource?

```sql
-- Find all users who can reach the slack app within 3 relationship hops
SELECT <-*{1,3}<-user FROM app:slack
```

This query finds users connected to the app through any chain of relationships (direct assignment, group membership, nested groups, etc.).

#### Generated SQL

Single-hop wildcards generate UNION queries:

```sql
-- Doodle: SELECT ->*->group FROM user:alice

SELECT t1.*, 'member_of' AS _relationship
FROM users t0
JOIN user_groups j0 ON t0.id = j0.user_id
JOIN groups t1 ON j0.group_id = t1.id
WHERE t0.external_id = $1 AND t0.valid_to = 'infinity' AND t1.valid_to = 'infinity'
```

Recursive wildcards generate recursive CTEs that explore all paths:

```sql
-- Doodle: SELECT ->*{1,3}->user FROM user:alice

WITH RECURSIVE path_cte AS (
    -- Base case: all first-level relationships from user
    SELECT j.group_id AS id, 'group' AS entity_type, 1 AS depth
    FROM users t0 JOIN user_groups j ON t0.id = j.user_id ...
    UNION ALL
    SELECT j.manager_id AS id, 'user' AS entity_type, 1 AS depth ...

    UNION ALL

    -- Recursive case: follow all relationships from each entity type
    SELECT j.target_id AS id, 'target_type' AS entity_type, p.depth + 1
    FROM path_cte p
    JOIN entities e ON p.id = e.id AND p.entity_type = 'entity_type'
    JOIN junction j ON e.id = j.source_id
    WHERE p.depth < 3
    ...
)
SELECT DISTINCT t.* FROM path_cte p
JOIN users t ON p.id = t.id AND p.entity_type = 'user'
WHERE p.depth >= 1 AND t.valid_to = 'infinity'
```

### Cross-Path Joins

Cross-path joins allow you to traverse multiple independent paths and join them based on a condition. This is essential for queries like "find users and the policy rules that apply to their groups."

#### Basic Syntax

```sql
SELECT fields
FROM entity AS alias ->rel->entity AS alias
JOIN entity:id ->rel->entity AS alias ->rel->entity AS alias
ON left_alias.field = right_alias.field
```

#### Example: Users with Effective Policy Rules

Find all users, their groups, and the policy rules that apply to those groups:

```sql
SELECT
    u.email,
    g.name AS access_group,
    p.name AS policy_name,
    r.name AS rule_name
FROM user AS u
    ->member_of->group AS g
    ->has_access->app AS a
JOIN app:slack
    ->governed_by->policy AS p
    ->has_rule->rule AS r
    ->applies_to->group AS rg
ON g.id = rg.id
```

This query:
1. Traverses from users to their groups to the apps they can access
2. Separately traverses from a specific app to its policies, rules, and the groups those rules apply to
3. Joins where the user's group matches the rule's target group

#### Schema for Policy-Based Access

```go
schema := doodle.NewSchema()

// Entities
schema.AddEntity("user", "users", "id").WithTemporal()
schema.AddEntity("group", "groups", "id").WithTemporal()
schema.AddEntity("app", "apps", "id").WithTemporal()
schema.AddEntity("policy", "policies", "id").WithTemporal()
schema.AddEntity("rule", "rules", "id").WithTemporal()

// Access relationships
schema.Relationship("member_of").From("user").To("group").Via("user_groups", "user_id", "group_id")
schema.Relationship("has_access").From("group").To("app").Via("group_apps", "group_id", "app_id")

// Policy relationships
schema.Relationship("governed_by").From("app").To("policy").Via("app_policies", "app_id", "policy_id")
schema.Relationship("has_rule").From("policy").To("rule").Via("policy_rules", "policy_id", "rule_id")
schema.Relationship("applies_to").From("rule").To("group").Via("rule_groups", "rule_id", "group_id")
```

#### Multiple JOINs

You can have multiple JOIN clauses to bring in data from different paths:

```sql
SELECT u.email, g.name, p.name, r.name, ra.role
FROM user AS u ->member_of->group AS g
JOIN app:slack ->governed_by->policy AS p ON g.app_id = p.app_id
JOIN rule AS r ->assigned_to->user AS ra ON u.id = ra.id
```

#### Generated SQL

Cross-path joins generate SQL with multiple JOIN chains and a WHERE clause for the ON condition:

```sql
-- Doodle: FROM user AS u ->member_of->group AS g JOIN app:slack ->governed_by->policy AS p ON g.id = p.group_id

SELECT t0.*, t1.*, t3.*
FROM users t0
JOIN user_groups j0 ON t0.id = j0.user_id
JOIN groups t1 ON j0.group_id = t1.id
JOIN apps t2 ON t2.external_id = 'slack' AND t2.valid_to = 'infinity'
JOIN app_policies j1 ON t2.id = j1.app_id
JOIN policies t3 ON j1.policy_id = t3.id
WHERE t0.valid_to = 'infinity'
  AND t1.valid_to = 'infinity'
  AND t3.valid_to = 'infinity'
  AND t1.id = t3.group_id  -- ON condition
```

#### Use Cases

- **Effective Policy Rules**: Find which rules apply to a user based on their group memberships
- **Access Audit**: Show the full path from user to resource with all intermediate entities
- **Compliance Reporting**: Join user access with the policies that govern that access
- **Impact Analysis**: Find all users affected by a policy change

### NULL Operators

```sql
-- Check for NULL values
SELECT * FROM user WHERE email IS NULL

-- Check for non-NULL values
SELECT * FROM user WHERE email IS NOT NULL

-- Combine with other conditions
SELECT * FROM user WHERE status = 'ACTIVE' AND email IS NOT NULL
```

### NOT Operator

Negate any condition:

```sql
-- NOT with comparison
SELECT * FROM user WHERE NOT status = 'INACTIVE'

-- NOT with LIKE
SELECT * FROM user WHERE email NOT LIKE '%@test.com'

-- NOT with IN
SELECT * FROM user WHERE status NOT IN ('DELETED', 'SUSPENDED')

-- NOT EXISTS
SELECT * FROM user WHERE NOT EXISTS (SELECT * FROM user WHERE status = 'BANNED')
```

### EXISTS Subqueries

Check for existence of related records:

```sql
-- Users who are members of at least one group
SELECT * FROM user WHERE EXISTS (SELECT ->member_of->group FROM user)

-- Users who are NOT in any group
SELECT * FROM user WHERE NOT EXISTS (SELECT ->member_of->group FROM user)
```

### LIKE Pattern Matching

```sql
-- Starts with
SELECT * FROM user WHERE email LIKE 'admin%'

-- Ends with
SELECT * FROM user WHERE email LIKE '%@example.com'

-- Contains
SELECT * FROM user WHERE name LIKE '%smith%'

-- NOT LIKE
SELECT * FROM user WHERE email NOT LIKE '%@test.com'
```

### Set Operations

Combine multiple queries:

```sql
-- UNION (removes duplicates)
SELECT email FROM user:alice UNION SELECT email FROM user:bob

-- UNION ALL (keeps duplicates)
SELECT status FROM user:alice UNION ALL SELECT status FROM user:bob

-- INTERSECT (common rows only)
SELECT ->member_of->group FROM user:alice INTERSECT SELECT ->member_of->group FROM user:bob

-- EXCEPT (rows in first but not second)
SELECT ->member_of->group FROM user:alice EXCEPT SELECT ->member_of->group FROM user:bob

-- Multiple set operations
SELECT * FROM user:alice
UNION SELECT * FROM user:bob
UNION SELECT * FROM user:charlie
```

### CASE Expressions

Conditional logic in SELECT:

```sql
-- Simple CASE with ELSE
SELECT CASE WHEN status = 'ACTIVE' THEN 'Yes' ELSE 'No' END AS is_active FROM user

-- Multiple WHEN clauses
SELECT CASE
    WHEN status = 'ACTIVE' THEN 'Active User'
    WHEN status = 'PENDING' THEN 'Pending Approval'
    WHEN status = 'SUSPENDED' THEN 'Account Suspended'
    ELSE 'Unknown'
END AS status_label FROM user

-- CASE with NULL checks
SELECT CASE WHEN email IS NULL THEN 'No Email' ELSE email END AS email_display FROM user

-- CASE with comparisons
SELECT CASE WHEN score > 90 THEN 'A' WHEN score > 80 THEN 'B' ELSE 'C' END AS grade FROM user
```

### COALESCE

Return first non-NULL value:

```sql
-- Default value for NULL
SELECT COALESCE(email, 'no-email@example.com') AS email FROM user

-- Multiple fallbacks
SELECT COALESCE(nickname, display_name, email, 'Anonymous') AS name FROM user

-- With alias
SELECT COALESCE(phone, 'N/A') AS contact FROM user
```

### NULLIF

Return NULL if two values are equal:

```sql
-- Convert empty string to NULL
SELECT NULLIF(email, '') AS email FROM user

-- Avoid division by zero (returns NULL instead of error)
SELECT total / NULLIF(count, 0) AS average FROM stats

-- Compare fields
SELECT NULLIF(current_email, previous_email) AS changed_email FROM user
```

### Common Table Expressions (WITH)

Define reusable subqueries:

```sql
-- Single CTE
WITH active_users AS (
    SELECT * FROM user WHERE status = 'ACTIVE'
)
SELECT * FROM active_users

-- Multiple CTEs
WITH active_users AS (
    SELECT * FROM user WHERE status = 'ACTIVE'
), admin_groups AS (
    SELECT * FROM group WHERE name LIKE 'admin%'
)
SELECT * FROM active_users WHERE id IN (SELECT <-member_of<-user FROM admin_groups)

-- CTE with traversal
WITH user_groups AS (
    SELECT ->member_of->group FROM user:alice
)
SELECT * FROM user_groups WHERE name = 'Engineering'
```

### String Functions

```sql
-- UPPER / LOWER
SELECT UPPER(name) AS name_upper FROM user
SELECT LOWER(email) AS email_lower FROM user

-- CONCAT
SELECT CONCAT(first_name, ' ', last_name) AS full_name FROM user

-- TRIM
SELECT TRIM(name) AS clean_name FROM user

-- SUBSTRING (value, start, length)
SELECT SUBSTRING(email, 1, 10) AS email_prefix FROM user

-- LENGTH
SELECT LENGTH(name) AS name_length FROM user
```

### Date Functions

```sql
-- Current timestamp
SELECT NOW() AS current_time FROM user

-- Current date (no time component)
SELECT CURRENT_DATE() AS today FROM user

-- DATE_TRUNC (truncate to precision)
SELECT DATE_TRUNC('month', created_at) AS month FROM user
SELECT DATE_TRUNC('year', created_at) AS year FROM user

-- EXTRACT (get date/time component)
SELECT EXTRACT('year', created_at) AS year FROM user
SELECT EXTRACT('month', created_at) AS month FROM user
SELECT EXTRACT('day', created_at) AS day FROM user
```

### Date Arithmetic

Add or subtract intervals from date/time fields:

```sql
-- Add interval
SELECT created + INTERVAL '1 day' AS tomorrow FROM user
SELECT created + INTERVAL '2 hours' AS later FROM user
SELECT created + INTERVAL '30 minutes' AS soon FROM user

-- Subtract interval
SELECT created - INTERVAL '7 days' AS last_week FROM user
SELECT created - INTERVAL '1 year' AS last_year FROM user

-- With alias
SELECT created + INTERVAL '1 month' AS next_month FROM user

-- In WHERE clause (filter by relative time)
SELECT * FROM user WHERE created > NOW() - INTERVAL '24 hours'
```

### Math Functions

```sql
-- ABS (absolute value)
SELECT ABS(balance) AS abs_balance FROM account

-- ROUND
SELECT ROUND(score, 2) AS rounded_score FROM user

-- CEIL (round up)
SELECT CEIL(price) AS ceiling_price FROM product

-- FLOOR (round down)
SELECT FLOOR(price) AS floor_price FROM product
```

### Array and JSON Aggregates

Aggregate values into arrays or JSON:

```sql
-- ARRAY_AGG: collect values into array
SELECT ARRAY_AGG(email) FROM user WHERE status = 'ACTIVE'

-- ARRAY_AGG with DISTINCT
SELECT ARRAY_AGG(DISTINCT status) FROM user

-- STRING_AGG: concatenate strings with delimiter
SELECT STRING_AGG(name, ', ') FROM user
SELECT STRING_AGG(email, '; ') AS all_emails FROM user

-- STRING_AGG with DISTINCT
SELECT STRING_AGG(DISTINCT status, ', ') FROM user

-- JSON_AGG: collect values into JSON array
SELECT JSON_AGG(email) FROM user WHERE status = 'ACTIVE'
```

### JSON Functions

Access and manipulate JSONB columns:

```sql
-- JSON_GET: access JSON field (returns JSON)
SELECT JSON_GET(metadata, 'theme') AS theme FROM user

-- JSON_TEXT: access JSON field (returns text)
SELECT JSON_TEXT(metadata, 'email') AS email FROM user

-- JSON_PATH: access nested JSON path (returns JSON)
SELECT JSON_PATH(metadata, 'address', 'city') AS city FROM user

-- JSON_PATH_TEXT: access nested JSON path (returns text)
SELECT JSON_PATH_TEXT(metadata, 'config', 'timezone') AS tz FROM user

-- JSON_BUILD_OBJECT: create JSON object from key-value pairs
SELECT JSON_BUILD_OBJECT('name', name, 'email', email, 'status', status) AS user_json FROM user
```

**PostgreSQL operator mapping:**

| Doodle Function | PostgreSQL |
|-----------------|------------|
| `JSON_GET(col, 'key')` | `col->'key'` |
| `JSON_TEXT(col, 'key')` | `col->>'key'` |
| `JSON_PATH(col, 'a', 'b')` | `col#>'{a,b}'` |
| `JSON_PATH_TEXT(col, 'a', 'b')` | `col#>>'{a,b}'` |

## Mutation Operations

Doodle supports INSERT, UPDATE, and DELETE statements with graph-aware relationship mutations and temporal versioning.

### INSERT

Insert new records with VALUES or SELECT syntax:

```sql
-- Basic INSERT with VALUES
INSERT INTO user (email, status) VALUES ('alice@example.com', 'ACTIVE')

-- Multiple rows
INSERT INTO user (email, status) VALUES ('a@example.com', 'ACTIVE'), ('b@example.com', 'PENDING')

-- With external_id (entity identifier)
INSERT INTO user:alice (email, status) VALUES ('alice@example.com', 'ACTIVE')

-- INSERT...SELECT (copy from another query)
INSERT INTO user (email, status) SELECT email, status FROM user WHERE status = 'PENDING'

-- With RETURNING clause
INSERT INTO user (email) VALUES ('alice@example.com') RETURNING *
INSERT INTO user (email) VALUES ('alice@example.com') RETURNING id, email
```

**Temporal entities**: For entities with `WithTemporal()`, INSERT automatically adds:
- `version = 1` (initial version)
- `valid_from = NOW()` (current timestamp)
- `valid_to = 'infinity'` (no expiration)

### UPDATE

Update existing records:

```sql
-- Basic UPDATE with WHERE
UPDATE user SET status = 'INACTIVE' WHERE email = 'alice@example.com'

-- Update by ID (external_id)
UPDATE user:alice SET status = 'INACTIVE'

-- Multiple fields
UPDATE user:alice SET status = 'INACTIVE', email = 'new@example.com'

-- With RETURNING
UPDATE user:alice SET status = 'INACTIVE' RETURNING *
UPDATE user:alice SET status = 'INACTIVE' RETURNING id, email, status
```

**Temporal UPDATE behavior** (for entities with `WithTemporal()`):
- Without FORCE: Creates a **new version** (closes current record with `valid_to = NOW()`, inserts new record with `version + 1`)
- With FORCE: Direct update to current record (no versioning)

```sql
-- Temporal update (creates new version)
UPDATE user:alice SET status = 'INACTIVE'
-- Generated: UPDATE ... SET valid_to = NOW() WHERE ...; INSERT INTO ... (version + 1)

-- FORCE: bypass versioning (direct update)
UPDATE FORCE user:alice SET status = 'INACTIVE'
-- Generated: UPDATE ... SET status = $1 WHERE ... AND valid_to = 'infinity'
```

### DELETE

Delete records with soft or hard delete:

```sql
-- Delete by ID
DELETE FROM user:alice

-- Delete with WHERE clause
DELETE FROM user WHERE status = 'DELETED'

-- With RETURNING
DELETE FROM user:alice RETURNING *
```

**Temporal DELETE behavior** (for entities with `WithTemporal()`):
- Without FORCE: **Soft delete** (sets `valid_to = NOW()` to mark as expired)
- With FORCE: **Hard delete** (physically removes the record)

```sql
-- Temporal delete (soft delete)
DELETE FROM user:alice
-- Generated: UPDATE ... SET valid_to = NOW() WHERE external_id = $1 AND valid_to = 'infinity'

-- FORCE: hard delete (physically remove)
DELETE FORCE FROM user:alice
-- Generated: DELETE FROM ... WHERE external_id = $1 AND valid_to = 'infinity'
```

### Relationship Mutations

Create and delete relationships using graph path syntax:

```sql
-- Create relationship: add alice to the admins group
INSERT INTO user:alice->member_of->group:admins

-- Create relationship with junction table fields
INSERT INTO user:alice->member_of->group:admins (role) VALUES ('admin')

-- Delete relationship: remove alice from the admins group
DELETE FROM user:alice->member_of->group:admins

-- Delete all relationships of a type: remove alice from all groups
DELETE FROM user:alice->member_of->group
```

**Generated SQL for relationship mutations:**

```sql
-- INSERT INTO user:alice->member_of->group:admins
INSERT INTO user_groups (user_id, group_id)
SELECT s.id, t.id FROM users s, groups t
WHERE s.external_id = 'alice' AND s.valid_to = 'infinity'
  AND t.external_id = 'admins' AND t.valid_to = 'infinity'

-- DELETE FROM user:alice->member_of->group:admins
DELETE FROM user_groups
WHERE user_id = (SELECT id FROM users WHERE external_id = 'alice' AND valid_to = 'infinity')
  AND group_id = (SELECT id FROM groups WHERE external_id = 'admins' AND valid_to = 'infinity')
```

### RETURNING Clause

Get affected rows back from mutations:

```sql
-- Return all columns
INSERT INTO user (email) VALUES ('alice@example.com') RETURNING *
UPDATE user:alice SET status = 'INACTIVE' RETURNING *
DELETE FROM user:alice RETURNING *

-- Return specific columns
INSERT INTO user (email) VALUES ('alice@example.com') RETURNING id, email
UPDATE user:alice SET status = 'INACTIVE' RETURNING id, email, status
DELETE FROM user:alice RETURNING id
```

### ParseStatement API

Use `ParseStatement()` and `GenerateStatement()` for mutation statements:

```go
// Parse any statement (SELECT, INSERT, UPDATE, DELETE)
stmt, err := doodle.ParseStatement("INSERT INTO user (email) VALUES ('alice@example.com')")
if err != nil {
    log.Fatal(err)
}

// Generate SQL
gen := doodle.NewGenerator(schema)
result, err := gen.GenerateStatement(stmt)
if err != nil {
    log.Fatal(err)
}

fmt.Println(result.SQL)    // INSERT INTO users (email, version, valid_from, valid_to) VALUES ($1, 1, NOW(), 'infinity')
fmt.Println(result.Params) // [alice@example.com]
```

The existing `Parse()` and `Generate()` functions continue to work for SELECT queries for backward compatibility.

## Generated SQL

Doodle transpiles queries to safe, parameterized PostgreSQL:

### Basic Queries

| Doodle | PostgreSQL |
|--------|------------|
| `SELECT * FROM user:alice` | `SELECT t0.* FROM users t0 WHERE t0.external_id = $1 AND t0.valid_to = 'infinity'` |
| `SELECT email, status FROM user:alice` | `SELECT t0.email, t0.status FROM users t0 WHERE ...` |
| `SELECT email AS e FROM user:alice` | `SELECT t0.email AS e FROM users t0 WHERE ...` |
| `SELECT DISTINCT status FROM user:alice` | `SELECT DISTINCT t0.status FROM users t0 WHERE ...` |

### Aggregations

| Doodle | PostgreSQL |
|--------|------------|
| `SELECT COUNT(*) FROM user` | `SELECT COUNT(*) FROM users t0 WHERE ...` |
| `SELECT COUNT(DISTINCT status) FROM user` | `SELECT COUNT(DISTINCT t0.status) FROM users t0 WHERE ...` |
| `SELECT ARRAY_AGG(email) FROM user` | `SELECT ARRAY_AGG(t0.email) FROM users t0 WHERE ...` |
| `SELECT STRING_AGG(name, ', ') FROM user` | `SELECT STRING_AGG(t0.name, ', ') FROM users t0 WHERE ...` |
| `SELECT JSON_AGG(email) FROM user` | `SELECT JSON_AGG(t0.email) FROM users t0 WHERE ...` |

### Graph Traversals

| Doodle | PostgreSQL |
|--------|------------|
| `SELECT ->member_of->group FROM user:alice` | `SELECT t1.* FROM users t0 JOIN user_groups j0 ON t0.id = j0.user_id JOIN groups t1 ON j0.group_id = t1.id WHERE ...` |
| `SELECT <-member_of<-user FROM group:admins` | `SELECT t1.* FROM groups t0 JOIN user_groups j0 ON t0.id = j0.group_id JOIN users t1 ON j0.user_id = t1.id WHERE ...` |
| `SELECT ->?member_of->group FROM user` | `... LEFT JOIN user_groups j0 ... LEFT JOIN groups t1 ...` |
| `SELECT ->reports_to{1,3}->user FROM user:alice` | `WITH RECURSIVE path_cte AS (...) SELECT DISTINCT t1.* FROM path_cte p JOIN users t1 ON p.id = t1.id WHERE p.depth >= 1` |

### Conditions

| Doodle | PostgreSQL |
|--------|------------|
| `... WHERE a = 'x' OR b = 'y'` | `... AND (t0.a = $2 OR t0.b = $3)` |
| `... WHERE x BETWEEN 10 AND 100` | `... AND t0.x BETWEEN $2 AND $3` |
| `... WHERE email IS NULL` | `... AND t0.email IS NULL` |
| `... WHERE email IS NOT NULL` | `... AND t0.email IS NOT NULL` |
| `... WHERE NOT status = 'DELETED'` | `... AND NOT (t0.status = $2)` |
| `... WHERE email LIKE '%@test.com'` | `... AND t0.email LIKE $2` |
| `... WHERE EXISTS (SELECT ...)` | `... AND EXISTS (SELECT ...)` |

### Grouping & Ordering

| Doodle | PostgreSQL |
|--------|------------|
| `... GROUP BY status` | `... GROUP BY t0.status` |
| `... HAVING COUNT(*) > 5` | `... HAVING COUNT(*) > $n` |
| `... ORDER BY email DESC` | `... ORDER BY t0.email DESC` |
| `... LIMIT 10 OFFSET 20` | `... LIMIT 10 OFFSET 20` |

### Temporal Queries

| Doodle | PostgreSQL |
|--------|------------|
| `... VERSION d'2024-01-01...'` | `... AND t0.valid_from <= $2 AND t0.valid_to > $2` |
| `... VERSION 3` | `... AND t0.version = $2` |

### Set Operations

| Doodle | PostgreSQL |
|--------|------------|
| `SELECT ... UNION SELECT ...` | `(SELECT ...) UNION (SELECT ...)` |
| `SELECT ... UNION ALL SELECT ...` | `(SELECT ...) UNION ALL (SELECT ...)` |
| `SELECT ... INTERSECT SELECT ...` | `(SELECT ...) INTERSECT (SELECT ...)` |
| `SELECT ... EXCEPT SELECT ...` | `(SELECT ...) EXCEPT (SELECT ...)` |

### Expressions

| Doodle | PostgreSQL |
|--------|------------|
| `CASE WHEN x THEN y ELSE z END` | `CASE WHEN x THEN y ELSE z END` |
| `COALESCE(a, b, c)` | `COALESCE(a, b, c)` |
| `NULLIF(a, b)` | `NULLIF(a, b)` |
| `created + INTERVAL '1 day'` | `(t0.created + INTERVAL '1 day')` |

### Functions

| Doodle | PostgreSQL |
|--------|------------|
| `UPPER(name)` | `UPPER(t0.name)` |
| `CONCAT(a, ' ', b)` | `CONCAT(t0.a, ' ', t0.b)` |
| `DATE_TRUNC('month', created)` | `DATE_TRUNC('month', t0.created)` |
| `NOW()` | `NOW()` |
| `ABS(value)` | `ABS(t0.value)` |

### JSON Functions

| Doodle | PostgreSQL |
|--------|------------|
| `JSON_GET(col, 'key')` | `(t0.col->'key')` |
| `JSON_TEXT(col, 'key')` | `(t0.col->>'key')` |
| `JSON_PATH(col, 'a', 'b')` | `(t0.col#>'{a,b}')` |
| `JSON_PATH_TEXT(col, 'a', 'b')` | `(t0.col#>>'{a,b}')` |
| `JSON_BUILD_OBJECT('k', v)` | `JSON_BUILD_OBJECT('k', t0.v)` |

### Mutations (INSERT/UPDATE/DELETE)

| Doodle | PostgreSQL |
|--------|------------|
| `INSERT INTO user (email) VALUES ('x')` | `INSERT INTO users (email, version, valid_from, valid_to) VALUES ($1, 1, NOW(), 'infinity')` |
| `INSERT INTO user:alice (email) VALUES ('x')` | `INSERT INTO users (external_id, email, ...) VALUES ($1, $2, ...)` |
| `UPDATE user SET status = 'x' WHERE ...` | `UPDATE users SET status = $1 WHERE ...` |
| `UPDATE user:alice SET status = 'x'` | (temporal) `UPDATE ... SET valid_to = NOW() ...; INSERT INTO ... (version + 1) ...` |
| `UPDATE FORCE user:alice SET status = 'x'` | `UPDATE users SET status = $1 WHERE external_id = $2 AND valid_to = 'infinity'` |
| `DELETE FROM user:alice` | (temporal) `UPDATE users SET valid_to = NOW() WHERE external_id = $1 AND valid_to = 'infinity'` |
| `DELETE FORCE FROM user:alice` | `DELETE FROM users WHERE external_id = $1 AND valid_to = 'infinity'` |
| `... RETURNING *` | `... RETURNING *` |
| `... RETURNING id, email` | `... RETURNING id, email` |

### Relationship Mutations

| Doodle | PostgreSQL |
|--------|------------|
| `INSERT INTO user:alice->member_of->group` | `INSERT INTO user_groups (user_id, group_id) SELECT s.id, t.id FROM users s, groups t WHERE ...` |
| `DELETE FROM user:alice->member_of->group` | `DELETE FROM user_groups WHERE user_id = (SELECT id FROM users WHERE ...) AND ...` |

## API

### Compile Only

Get the SQL without executing:

```go
result, err := db.Compile("SELECT ->member_of->group FROM user:alice")
fmt.Println(result.SQL)    // SELECT t1.* FROM users t0 JOIN ...
fmt.Println(result.Params) // [alice]
```

### Query Execution

```go
// Multiple rows
rows, err := db.Query(ctx, "SELECT ->member_of->group FROM user:alice")

// Single row
row, err := db.QueryRow(ctx, "SELECT * FROM user:alice")

// Non-SELECT statements
result, err := db.Exec(ctx, "...")
```

### Using Existing Connection

```go
import "database/sql"

conn, _ := sql.Open("postgres", connStr)
db := doodle.New(schema).WithConnection(conn)
```

## Development

```bash
# Run unit tests
make test

# Start PostgreSQL and run integration tests
make test-integration

# Stop PostgreSQL
make docker-down
```

### Test Data

Load the test schema and seed data for manual testing:

```bash
# Load schema (creates tables)
psql -d mydb -f testdata/init.sql

# Load comprehensive seed data (users, groups, apps, relationships)
psql -d mydb -f testdata/seed.sql
```

The seed data includes:
- **14 users** with various statuses, scores, salaries, and JSONB metadata
- **6 groups** including an empty group for edge case testing
- **7 apps** with different types
- **Management hierarchy** (CEO → CTO/CFO → Managers → Engineers) for variable-length path testing
- **Group memberships** with roles (owner, admin, member) for path field access testing
- **App permissions** (admin, write, read) for multi-hop traversal testing
- **NULL values** for testing COALESCE, NULLIF, IS NULL operators

Example queries to try after loading:

```sql
-- Basic traversal
SELECT ->member_of->group FROM user:seed_eng_1

-- Multi-hop: find apps accessible by a user
SELECT ->member_of->group->has_access->app FROM user:seed_eng_1

-- Variable-length path: find all managers up to 3 levels
SELECT ->reports_to{1,3}->user FROM user:seed_eng_1

-- Path field access: get role from junction table
SELECT ->member_of.role->group FROM user:seed_eng_1

-- JSON extraction
SELECT JSON_PATH_TEXT(metadata, 'config', 'theme') AS theme FROM user

-- Aggregation with grouping
SELECT status, COUNT(*), AVG(score) FROM user GROUP BY status

-- Date arithmetic
SELECT email, created_at + INTERVAL '90 days' AS review_date FROM user

-- Complex query
SELECT CASE
    WHEN score > 80 THEN 'High Performer'
    WHEN score > 50 THEN 'Meets Expectations'
    ELSE 'Needs Improvement'
END AS rating,
COUNT(*)
FROM user
WHERE status = 'ACTIVE'
GROUP BY rating

-- Mutation examples
INSERT INTO user (email, status) VALUES ('newuser@example.com', 'PENDING') RETURNING *
UPDATE user:seed_eng_1 SET status = 'INACTIVE' RETURNING id, status
DELETE FROM user:seed_inactive RETURNING *

-- Relationship mutations
INSERT INTO user:seed_eng_1->member_of->group:engineering
DELETE FROM user:seed_eng_1->member_of->group:empty_group
```

## License

MIT
