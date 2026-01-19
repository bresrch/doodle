package doodle

// Query represents a parsed doodle query
type Query struct {
	With     []*CTEDef       `parser:"('WITH' @@ (',' @@)*)?"`
	Select   *SelectClause   `parser:"'SELECT' @@"`
	From     *FromClause     `parser:"'FROM' @@"`
	Joins    []*JoinClause   `parser:"@@*"`
	Version  *VersionClause  `parser:"('VERSION' @@)?"`
	Versions *VersionsClause `parser:"('VERSIONS' @@)?"`
	Where    *WhereClause    `parser:"('WHERE' @@)?"`
	GroupBy  *GroupByClause  `parser:"('GROUP' 'BY' @@)?"`
	Having   *HavingClause   `parser:"('HAVING' @@)?"`
	OrderBy  *OrderByClause  `parser:"('ORDER' 'BY' @@)?"`
	Limit    *int            `parser:"('LIMIT' @Int)?"`
	Offset   *int            `parser:"('OFFSET' @Int)?"`
	Compound []*CompoundPart `parser:"@@*"`
}

// CTEDef represents a Common Table Expression definition
type CTEDef struct {
	Name  string `parser:"@Ident 'AS' '('"`
	Query *Query `parser:"@@ ')'"`
}

// CompoundPart represents a set operation with another query
type CompoundPart struct {
	Operator string `parser:"@('UNION' 'ALL' | 'UNION' | 'INTERSECT' | 'EXCEPT')"`
	Query    *Query `parser:"@@"`
}

// VersionClause represents temporal or version number queries
type VersionClause struct {
	Timestamp *string `parser:"( @DateTime"`
	Number    *int    `parser:"| @Int )"`
}

// VersionsClause represents multi-version queries (VERSIONS ALL, VERSIONS LAST N, etc.)
type VersionsClause struct {
	All     bool `parser:"( @'ALL'"`
	Last    *int `parser:"| 'LAST' @Int"`
	Between *VersionRange `parser:"| @@ )"`
}

// VersionRange represents a version range (BETWEEN x AND y)
type VersionRange struct {
	From *string `parser:"'BETWEEN' @DateTime 'AND'"`
	To   *string `parser:"@DateTime"`
}

// MetaFunction represents meta:: namespace functions for version navigation
type MetaFunction struct {
	Name string `parser:"'meta' ':' ':' @('version' | 'version_num' | 'operation' | 'valid_from' | 'valid_to' | 'previous' | 'first' | 'latest')"`
	Call bool   `parser:"@( '(' ')' )?"`
}

// SelectClause represents the SELECT portion of a query
type SelectClause struct {
	Distinct  bool         `parser:"@'DISTINCT'?"`
	Star      bool         `parser:"( @'*'"`
	Aggregate *Aggregate   `parser:"| @@"`
	Path      *Path        `parser:"| @@"`
	Fields    []*FieldExpr `parser:"| @@ (',' @@)* )"`
}

// Aggregate represents an aggregate function call
type Aggregate struct {
	Function  string  `parser:"@('COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'ARRAY_AGG' | 'STRING_AGG' | 'JSON_AGG')"`
	Distinct  bool    `parser:"'(' @'DISTINCT'?"`
	Star      bool    `parser:"( @'*'"`
	Path      *Path   `parser:"| @@"`
	Field     *Field  `parser:"| @@ )?"`
	Delimiter *string `parser:"(',' @String)?"`
	End       bool    `parser:"@')'"`
}

// Path represents a graph traversal path
type Path struct {
	Traversals []*Traversal `parser:"@@+"`
}

// Traversal represents a single edge traversal
type Traversal struct {
	Direction string `parser:"@('->' | '<-' | '<->' | '?->' | '?<-' | '!->' | '!<-' | '->?' | '<?-' | '->!' | '<!-')"`
	Wildcard  bool   `parser:"( @'*'"`
	Target    string `parser:"| @(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS') )"`
	MinHops   *int   `parser:"('{' @Int"`
	MaxHops   *int   `parser:"(',' @Int)? '}')?"`
	Alias     string `parser:"('AS' @Ident)?"`
	Field     string `parser:"('.' @(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS'))?"`
}

// IsOptional returns true if this traversal uses LEFT JOIN semantics
func (t *Traversal) IsOptional() bool {
	return t.Direction == "->?" || t.Direction == "<?-" || t.Direction == "?->" || t.Direction == "?<-"
}

// IsWildcard returns true if this traversal matches any relationship
func (t *Traversal) IsWildcard() bool {
	return t.Wildcard
}

// IsNegated returns true if this traversal uses NOT EXISTS semantics
func (t *Traversal) IsNegated() bool {
	return t.Direction == "->!" || t.Direction == "<!-" || t.Direction == "!->" || t.Direction == "!<-"
}

// IsBidirectional returns true if this traversal matches in either direction
func (t *Traversal) IsBidirectional() bool {
	return t.Direction == "<->"
}

// HasQuantifier returns true if this traversal has path length constraints
func (t *Traversal) HasQuantifier() bool {
	return t.MinHops != nil
}

// GetMinHops returns the minimum hops (default 1)
func (t *Traversal) GetMinHops() int {
	if t.MinHops != nil {
		return *t.MinHops
	}
	return 1
}

// GetMaxHops returns the maximum hops
// Returns the explicit max if set, otherwise returns min (for exact count like {3})
// For unbounded recursion, the generator will handle setting a safe maximum
func (t *Traversal) GetMaxHops() int {
	if t.MaxHops != nil {
		return *t.MaxHops
	}
	if t.MinHops != nil {
		return *t.MinHops
	}
	return 1
}

// IsUnbounded returns true if this traversal has unbounded recursion (e.g., {1,})
// This is indicated by MinHops being set but MaxHops being nil
func (t *Traversal) IsUnbounded() bool {
	return t.MinHops != nil && t.MaxHops == nil
}

// BaseDirection returns the base direction without optional/negated marker
func (t *Traversal) BaseDirection() string {
	switch t.Direction {
	case "->?", "->!", "?->", "!->":
		return "->"
	case "<?-", "<!-", "?<-", "!<-":
		return "<-"
	case "<->":
		return "<->"
	default:
		return t.Direction
	}
}

// FieldExpr represents a field in SELECT with optional alias
type FieldExpr struct {
	Case          *CaseExpr       `parser:"( @@"`
	Coalesce      *Coalesce       `parser:"| @@"`
	Nullif        *Nullif         `parser:"| @@"`
	DateArith     *DateArithmetic `parser:"| @@"`
	FuncCall      *FuncCall       `parser:"| @@"`
	Aggregate     *Aggregate      `parser:"| @@"`
	Path          *Path           `parser:"| @@"`
	Field         *Field          `parser:"| @@ )"`
	Alias         string          `parser:"('AS' @(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET'))?"`
}

// DateArithmetic represents date/time arithmetic like field + INTERVAL '1 day'
type DateArithmetic struct {
	Left     *Field    `parser:"@@"`
	Operator string    `parser:"@('+' | '-')"`
	Interval *Interval `parser:"@@"`
}

// Interval represents an INTERVAL expression
type Interval struct {
	Value string `parser:"'INTERVAL' @String"`
}

// FuncCall represents a function call (string, date, math, JSON functions)
type FuncCall struct {
	Name string       `parser:"@('UPPER' | 'LOWER' | 'CONCAT' | 'TRIM' | 'SUBSTRING' | 'LENGTH' | 'DATE_TRUNC' | 'EXTRACT' | 'NOW' | 'CURRENT_DATE' | 'ABS' | 'ROUND' | 'CEIL' | 'FLOOR' | 'JSON_GET' | 'JSON_TEXT' | 'JSON_PATH' | 'JSON_PATH_TEXT' | 'JSON_BUILD_OBJECT')"`
	Args []*FuncArg   `parser:"'(' (@@ (',' @@)*)? ')'"`
}

// FuncArg represents an argument to a function
type FuncArg struct {
	String *string  `parser:"( @String"`
	Float  *float64 `parser:"| @Float"`
	Int    *int64   `parser:"| @Int"`
	Field  *Field   `parser:"| @@ )"`
}

// CaseExpr represents a CASE WHEN ... THEN ... ELSE ... END expression
type CaseExpr struct {
	Whens []*WhenClause `parser:"'CASE' @@+"`
	Else  *CaseValue    `parser:"('ELSE' @@)?"`
	End   bool          `parser:"@'END'"`
}

// WhenClause represents a WHEN ... THEN ... part
type WhenClause struct {
	Condition *SimpleCondition `parser:"'WHEN' @@"`
	Then      *CaseValue       `parser:"'THEN' @@"`
}

// SimpleCondition represents a condition in CASE WHEN
type SimpleCondition struct {
	Left  *ConditionField `parser:"@@"`
	Op    string          `parser:"@('IS' 'NOT' 'NULL' | 'IS' 'NULL' | '=' | '!=' | '>' | '<' | '>=' | '<=' | 'LIKE')"`
	Right *CaseValue      `parser:"@@?"`
}

// CaseValue represents a value in CASE expressions
type CaseValue struct {
	String *string  `parser:"( @String"`
	Float  *float64 `parser:"| @Float"`
	Int    *int64   `parser:"| @Int"`
	Bool   *bool    `parser:"| @('true' | 'false')"`
	Null   bool     `parser:"| @'NULL'"`
	Field  *Field   `parser:"| @@ )"`
}

// Coalesce represents COALESCE(val1, val2, ...)
type Coalesce struct {
	Values []*CaseValue `parser:"'COALESCE' '(' @@ (',' @@)* ')'"`
}

// Nullif represents NULLIF(val1, val2)
type Nullif struct {
	First  *CaseValue `parser:"'NULLIF' '(' @@"`
	Second *CaseValue `parser:"',' @@ ')'"`
}

// Field represents a field selection
type Field struct {
	Entity string `parser:"(@(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS') '.')?"`
	Name   string `parser:"@(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS')"`
}

// FromClause represents the FROM portion
type FromClause struct {
	Subquery *Query       `parser:"( '(' @@ ')'"`
	Entity   string       `parser:"| @(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS') )"`
	ID       string       `parser:"( ':' @(Ident | String | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS') )?"`
	Alias    string       `parser:"('AS' @(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX'))?"`
	Path     []*Traversal `parser:"@@*"`
}

// JoinClause represents a JOIN with another path traversal
type JoinClause struct {
	Type   string         `parser:"@('LEFT' 'JOIN' | 'JOIN')"`
	Entity string         `parser:"@(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS')"`
	ID     string         `parser:"( ':' @(Ident | String | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS') )?"`
	Alias  string         `parser:"('AS' @(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX'))?"`
	Path   []*Traversal   `parser:"@@*"`
	On     *JoinCondition `parser:"'ON' @@"`
}

// JoinCondition represents the ON condition for a JOIN
type JoinCondition struct {
	Left  *JoinField `parser:"@@"`
	Op    string     `parser:"@('=' | '!=' | '>' | '<' | '>=' | '<=')"`
	Right *JoinField `parser:"@@"`
}

// JoinField represents a field reference in a JOIN condition (alias.field)
type JoinField struct {
	Alias string `parser:"@Ident"`
	Field string `parser:"'.' @Ident"`
}

// WhereClause represents filtering conditions with OR support
type WhereClause struct {
	Or []*AndConditions `parser:"@@ ('OR' @@)*"`
}

// AndConditions represents conditions joined by AND
type AndConditions struct {
	Conditions []*Condition `parser:"@@ (('AND' | ',') @@)*"`
}

// Condition represents a single filter condition
type Condition struct {
	Not     bool            `parser:"@'NOT'?"`
	Exists  *Query          `parser:"( 'EXISTS' '(' @@ ')'"`
	Left    *ConditionField `parser:"| ( @@"`
	Op      string          `parser:"@('IS' 'NOT' 'NULL' | 'IS' 'NULL' | '=' | '!=' | '>' | '<' | '>=' | '<=' | 'LIKE' | 'NOT' 'LIKE' | 'IN' | 'NOT' 'IN' | 'BETWEEN')"`
	Between *BetweenValue   `parser:"( @@"`
	Right   *ConditionValue `parser:"| @@? ) ) )"`
}

// BetweenValue represents the range for BETWEEN operator
type BetweenValue struct {
	Low  *ConditionValue `parser:"@@ 'AND'"`
	High *ConditionValue `parser:"@@"`
}

// ConditionField represents the left side of a condition
type ConditionField struct {
	Path  []*Traversal `parser:"@@*"`
	Field string       `parser:"('.'? @(Ident | 'GROUP' | 'ORDER' | 'VERSION' | 'LIMIT' | 'OFFSET' | 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'AS'))?"`
}

// ConditionValue represents the right side of a condition
type ConditionValue struct {
	Subquery *Query   `parser:"( '(' @@ ')'"`
	String   *string  `parser:"| @String"`
	Float    *float64 `parser:"| @Float"`
	Int      *int64   `parser:"| @Int"`
	Bool     *bool    `parser:"| @('true' | 'false')"`
	Null     bool     `parser:"| @'NULL'"`
	List     []*Value `parser:"| '(' @@ (',' @@)* ')' )"`
}

// Value represents a literal value
type Value struct {
	String *string  `parser:"( @String"`
	Float  *float64 `parser:"| @Float"`
	Int    *int64   `parser:"| @Int )"`
}

// GroupByClause represents GROUP BY with multiple fields
type GroupByClause struct {
	Fields []*Field `parser:"@@ (',' @@)*"`
}

// HavingClause represents HAVING conditions (same structure as WHERE)
type HavingClause struct {
	Or []*AndHavingConditions `parser:"@@ ('OR' @@)*"`
}

// AndHavingConditions represents conditions joined by AND in HAVING
type AndHavingConditions struct {
	Conditions []*HavingCondition `parser:"@@ (('AND' | ',') @@)*"`
}

// HavingCondition represents a single HAVING condition (can use aggregates)
type HavingCondition struct {
	Aggregate *Aggregate      `parser:"( @@"`
	Field     *ConditionField `parser:"| @@ )"`
	Op        string          `parser:"@('=' | '!=' | '>' | '<' | '>=' | '<=')"`
	Right     *ConditionValue `parser:"@@"`
}

// OrderByClause represents ORDER BY with multiple fields
type OrderByClause struct {
	Fields []*OrderByField `parser:"@@ (',' @@)*"`
}

// OrderByField represents a single ORDER BY field
type OrderByField struct {
	Path      *Path  `parser:"( @@"`
	Field     *Field `parser:"| @@ )"`
	Direction string `parser:"@('ASC' | 'DESC')?"`
}

// Statement represents any doodle statement (SELECT, INSERT, UPDATE, DELETE, EXPLAIN ACCESS)
type Statement struct {
	ExplainAccess *ExplainAccessStmt `parser:"  @@"`
	Insert        *InsertStmt        `parser:"| @@"`
	Update        *UpdateStmt        `parser:"| @@"`
	Delete        *DeleteStmt        `parser:"| @@"`
	Select        *Query             `parser:"| @@"`
}

// InsertStmt represents an INSERT statement
type InsertStmt struct {
	Entity    string           `parser:"'INSERT' 'INTO' @Ident"`
	ID        string           `parser:"(':' @(Ident | String))?"`
	Path      []*Traversal     `parser:"@@*"`
	Fields    []string         `parser:"('(' @Ident (',' @Ident)* ')')?"`
	Values    []*ValueRow      `parser:"( 'VALUES' @@ (',' @@)*"`
	Select    *Query           `parser:"| @@ )?"`
	Returning *ReturningClause `parser:"('RETURNING' @@)?"`
}

// ValueRow represents a row of values in INSERT
type ValueRow struct {
	Values []*InsertValue `parser:"'(' @@ (',' @@)* ')'"`
}

// InsertValue represents a value in INSERT VALUES
type InsertValue struct {
	String *string  `parser:"( @String"`
	Float  *float64 `parser:"| @Float"`
	Int    *int64   `parser:"| @Int"`
	Bool   *bool    `parser:"| @('true' | 'false')"`
	Null   bool     `parser:"| @'NULL' )"`
}

// UpdateStmt represents an UPDATE statement
type UpdateStmt struct {
	Force     bool             `parser:"'UPDATE' @'FORCE'?"`
	Entity    string           `parser:"@Ident"`
	ID        string           `parser:"(':' @(Ident | String))?"`
	Set       []*SetClause     `parser:"'SET' @@ (',' @@)*"`
	Where     *WhereClause     `parser:"('WHERE' @@)?"`
	Returning *ReturningClause `parser:"('RETURNING' @@)?"`
}

// SetClause represents a field=value assignment in UPDATE
type SetClause struct {
	Field string       `parser:"@Ident '='"`
	Value *InsertValue `parser:"@@"`
}

// DeleteStmt represents a DELETE statement
type DeleteStmt struct {
	Force     bool             `parser:"'DELETE' @'FORCE'?"`
	Entity    string           `parser:"'FROM' @Ident"`
	ID        string           `parser:"(':' @(Ident | String))?"`
	Path      []*Traversal     `parser:"@@*"`
	Where     *WhereClause     `parser:"('WHERE' @@)?"`
	Returning *ReturningClause `parser:"('RETURNING' @@)?"`
}

// ReturningClause represents a RETURNING clause
type ReturningClause struct {
	Star   bool     `parser:"( @'*'"`
	Fields []*Field `parser:"| @@ (',' @@)* )"`
}

// ExplainAccessStmt represents an EXPLAIN ACCESS statement
// Syntax: EXPLAIN ACCESS user:alice TO app:slack [VERSION d'2024-01-01' | VERSIONS ALL | VERSIONS LAST N | VERSIONS BETWEEN x AND y]
type ExplainAccessStmt struct {
	FromEntity string          `parser:"'EXPLAIN' 'ACCESS' @Ident"`
	FromID     string          `parser:"':' @(Ident | String)"`
	ToEntity   string          `parser:"'TO' @Ident"`
	ToID       string          `parser:"':' @(Ident | String)"`
	Version    *VersionClause  `parser:"('VERSION' @@)?"`
	Versions   *VersionsClause `parser:"('VERSIONS' @@)?"`
}
