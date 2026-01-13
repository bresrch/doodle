package doodle

import (
	"fmt"
	"strings"
)

// Generator transpiles doodle AST to PostgreSQL
type Generator struct {
	schema *Schema
}

// GeneratedQuery contains the SQL and parameters
type GeneratedQuery struct {
	SQL    string
	Params []interface{}
}

// NewGenerator creates a generator with the given schema
func NewGenerator(schema *Schema) *Generator {
	return &Generator{schema: schema}
}

// Generate transpiles a Query AST to PostgreSQL
func (g *Generator) Generate(q *Query) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: make([]interface{}, 0),
	}

	// Handle CTEs (WITH clause)
	var cteSQL string
	cteNames := make(map[string]bool)
	if len(q.With) > 0 {
		cteParts := []string{}
		for _, cte := range q.With {
			cteNames[cte.Name] = true
			cteResult, err := g.Generate(cte.Query)
			if err != nil {
				return nil, fmt.Errorf("CTE %s error: %w", cte.Name, err)
			}
			// Adjust parameter numbers
			adjustedSQL := g.adjustParamNumbers(cteResult.SQL, len(result.Params))
			result.Params = append(result.Params, cteResult.Params...)
			cteParts = append(cteParts, fmt.Sprintf("%s AS (%s)", cte.Name, adjustedSQL))
		}
		cteSQL = "WITH " + strings.Join(cteParts, ", ") + " "
	}

	// Handle subquery in FROM clause
	if q.From.Subquery != nil {
		subResult, err := g.generateWithSubquery(q)
		if err != nil {
			return nil, err
		}
		result.SQL = cteSQL + subResult.SQL
		result.Params = append(result.Params, subResult.Params...)
		return result, nil
	}

	// Check if FROM references a CTE (not a schema entity)
	if cteNames[q.From.Entity] {
		return g.generateFromCTE(q, cteSQL, result.Params)
	}

	// Resolve starting entity
	startEntity, err := g.schema.GetEntity(q.From.Entity)
	if err != nil {
		return nil, fmt.Errorf("unknown entity in FROM clause: %w", err)
	}

	// Track table aliases
	aliases := &aliasTracker{
		aliases: make(map[string]string),
		counter: 0,
	}
	startAlias := aliases.next(q.From.Entity)

	// Build the query parts
	var selectClause string
	var joins []string
	var currentEntity = q.From.Entity
	var finalAlias = startAlias
	var isAggregate bool
	var hasGroupBy = q.GroupBy != nil

	// Handle DISTINCT
	distinctPrefix := ""
	if q.Select.Distinct {
		distinctPrefix = "DISTINCT "
	}

	// Process traversals if present in SELECT path
	if q.Select.Path != nil && len(q.Select.Path.Traversals) > 0 {
		// Check for variable-length paths - use recursive CTE
		if hasVariableLengthPath(q.Select.Path.Traversals) {
			return g.generateWithRecursivePath(q, startEntity, startAlias, cteSQL, result.Params)
		}

		travResult, err := g.processTraversals(q.Select.Path.Traversals, currentEntity, startAlias, aliases, &joins)
		if err != nil {
			return nil, err
		}
		finalAlias = travResult.FinalAlias
		// Build SELECT with target entity and any junction fields
		selectParts := []string{fmt.Sprintf("%s.*", finalAlias)}
		selectParts = append(selectParts, travResult.JunctionFields...)
		selectClause = distinctPrefix + strings.Join(selectParts, ", ")
	} else if q.Select.Aggregate != nil {
		// Handle aggregate function
		isAggregate = true
		aggSQL, err := g.generateAggregate(q.Select.Aggregate, startAlias, currentEntity, aliases, &joins)
		if err != nil {
			return nil, err
		}
		selectClause = distinctPrefix + aggSQL
	} else if q.Select.Star {
		selectClause = fmt.Sprintf("%s%s.*", distinctPrefix, startAlias)
	} else if len(q.Select.Fields) > 0 {
		// Specific fields
		fields := make([]string, len(q.Select.Fields))
		for i, f := range q.Select.Fields {
			fieldStr, fieldIsAgg, err := g.generateFieldExprWithAggCheck(f, startAlias, currentEntity, aliases, &joins)
			if err != nil {
				return nil, err
			}
			if fieldIsAgg {
				isAggregate = true
			}
			fields[i] = fieldStr
		}
		selectClause = distinctPrefix + strings.Join(fields, ", ")
	} else {
		selectClause = fmt.Sprintf("%s%s.*", distinctPrefix, startAlias)
	}

	// Build base query
	sql := fmt.Sprintf("SELECT %s FROM %s %s",
		selectClause,
		startEntity.Table,
		startAlias,
	)

	// Add joins
	if len(joins) > 0 {
		sql += " " + strings.Join(joins, " ")
	}

	// Add WHERE clause
	whereClauses := []string{}

	// Starting entity ID filter (only if ID is provided)
	if q.From.ID != "" {
		result.Params = append(result.Params, cleanID(q.From.ID))
		whereClauses = append(whereClauses, fmt.Sprintf("%s.external_id = $%d", startAlias, len(result.Params)))
	}

	// Add temporal/version clause (only if entity has temporal config)
	if startEntity.Temporal != nil {
		tc := startEntity.Temporal
		if q.Version != nil {
			if q.Version.Timestamp != nil {
				// Query at point in time: valid_from <= ts AND valid_to > ts
				result.Params = append(result.Params, *q.Version.Timestamp)
				paramNum := len(result.Params)
				whereClauses = append(whereClauses,
					fmt.Sprintf("%s.%s <= $%d", startAlias, tc.ValidFromColumn, paramNum),
					fmt.Sprintf("%s.%s > $%d", startAlias, tc.ValidToColumn, paramNum),
				)
			} else if q.Version.Number != nil {
				// Query specific version number
				result.Params = append(result.Params, *q.Version.Number)
				whereClauses = append(whereClauses,
					fmt.Sprintf("%s.%s = $%d", startAlias, tc.VersionColumn, len(result.Params)),
				)
			}
		} else {
			// Default: current version (valid_to = infinity)
			whereClauses = append(whereClauses,
				fmt.Sprintf("%s.%s = 'infinity'", startAlias, tc.ValidToColumn),
			)
		}
	}

	// User-defined conditions with OR support
	if q.Where != nil {
		whereSQL, params, err := g.generateWhereClause(q.Where, aliases, startAlias, len(result.Params))
		if err != nil {
			return nil, err
		}
		result.Params = append(result.Params, params...)
		whereClauses = append(whereClauses, whereSQL)
	}

	if len(whereClauses) > 0 {
		sql += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Add GROUP BY
	if q.GroupBy != nil {
		groupBySQL := g.generateGroupBy(q.GroupBy, aliases, startAlias)
		sql += " GROUP BY " + groupBySQL
	}

	// Add HAVING
	if q.Having != nil {
		havingSQL, params, err := g.generateHavingClause(q.Having, aliases, startAlias, len(result.Params))
		if err != nil {
			return nil, err
		}
		result.Params = append(result.Params, params...)
		sql += " HAVING " + havingSQL
	}

	// Add ORDER BY (allowed with aggregates if GROUP BY is present)
	if q.OrderBy != nil && (!isAggregate || hasGroupBy) {
		orderBySQL, err := g.generateOrderBy(q.OrderBy, aliases, startAlias)
		if err != nil {
			return nil, err
		}
		sql += " ORDER BY " + orderBySQL
	}

	// Add LIMIT
	if q.Limit != nil {
		sql += fmt.Sprintf(" LIMIT %d", *q.Limit)
	}

	// Add OFFSET
	if q.Offset != nil {
		sql += fmt.Sprintf(" OFFSET %d", *q.Offset)
	}

	// Handle compound queries (UNION, INTERSECT, EXCEPT)
	for _, compound := range q.Compound {
		compoundResult, err := g.Generate(compound.Query)
		if err != nil {
			return nil, fmt.Errorf("compound query error: %w", err)
		}

		// Map operator to SQL
		op := compound.Operator
		if op == "UNIONALL" {
			op = "UNION ALL"
		}

		// Adjust parameter numbers in compound query
		paramOffset := len(result.Params)
		adjustedSQL := g.adjustParamNumbers(compoundResult.SQL, paramOffset)

		sql += " " + op + " " + adjustedSQL
		result.Params = append(result.Params, compoundResult.Params...)
	}

	result.SQL = cteSQL + sql
	return result, nil
}

// generateFromCTE generates a query that references a CTE
func (g *Generator) generateFromCTE(q *Query, cteSQL string, cteParams []interface{}) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: cteParams,
	}

	cteName := q.From.Entity
	alias := cteName
	if q.From.Alias != "" {
		alias = q.From.Alias
	}

	// Build SELECT clause
	var selectClause string
	if q.Select.Star {
		selectClause = alias + ".*"
	} else if len(q.Select.Fields) > 0 {
		fieldParts := []string{}
		for _, f := range q.Select.Fields {
			if f.Field != nil {
				fieldSQL := fmt.Sprintf("%s.%s", alias, f.Field.Name)
				if f.Alias != "" {
					fieldSQL += " AS " + f.Alias
				}
				fieldParts = append(fieldParts, fieldSQL)
			}
		}
		selectClause = strings.Join(fieldParts, ", ")
	} else {
		selectClause = alias + ".*"
	}

	if q.Select.Distinct {
		selectClause = "DISTINCT " + selectClause
	}

	sql := fmt.Sprintf("SELECT %s FROM %s %s", selectClause, cteName, alias)

	// Handle WHERE clause
	if q.Where != nil {
		whereSQL, whereParams, err := g.generateCTEWhereClause(q.Where, alias, len(result.Params))
		if err != nil {
			return nil, err
		}
		result.Params = append(result.Params, whereParams...)
		sql += " WHERE " + whereSQL
	}

	// Handle LIMIT/OFFSET
	if q.Limit != nil {
		sql += fmt.Sprintf(" LIMIT %d", *q.Limit)
	}
	if q.Offset != nil {
		sql += fmt.Sprintf(" OFFSET %d", *q.Offset)
	}

	result.SQL = cteSQL + sql
	return result, nil
}

// generateCTEWhereClause generates WHERE for CTE queries (simplified)
func (g *Generator) generateCTEWhereClause(where *WhereClause, alias string, paramOffset int) (string, []interface{}, error) {
	var params []interface{}
	var orParts []string

	for _, andGroup := range where.Or {
		andClauses := []string{}
		for _, cond := range andGroup.Conditions {
			if cond.Left != nil && cond.Left.Field != "" {
				fieldRef := fmt.Sprintf("%s.%s", alias, cond.Left.Field)
				op := cond.Op

				if cond.Right != nil {
					if cond.Right.String != nil {
						params = append(params, *cond.Right.String)
						andClauses = append(andClauses, fmt.Sprintf("%s %s $%d", fieldRef, op, paramOffset+len(params)))
					} else if cond.Right.Int != nil {
						params = append(params, *cond.Right.Int)
						andClauses = append(andClauses, fmt.Sprintf("%s %s $%d", fieldRef, op, paramOffset+len(params)))
					}
				}
			}
		}
		if len(andClauses) > 0 {
			orParts = append(orParts, "("+strings.Join(andClauses, " AND ")+")")
		}
	}

	if len(orParts) == 0 {
		return "", nil, nil
	}

	return strings.Join(orParts, " OR "), params, nil
}

// generateWithSubquery handles queries with subqueries in FROM clause
func (g *Generator) generateWithSubquery(q *Query) (*GeneratedQuery, error) {
	// Generate the subquery first
	subResult, err := g.Generate(q.From.Subquery)
	if err != nil {
		return nil, fmt.Errorf("subquery error: %w", err)
	}

	result := &GeneratedQuery{
		Params: subResult.Params,
	}

	subAlias := "sub"
	if q.From.Alias != "" {
		subAlias = q.From.Alias
	}

	// Build SELECT clause for outer query
	var selectClause string
	if q.Select.Distinct {
		selectClause = "DISTINCT "
	}
	if q.Select.Star {
		selectClause += subAlias + ".*"
	} else if len(q.Select.Fields) > 0 {
		fields := make([]string, len(q.Select.Fields))
		for i, f := range q.Select.Fields {
			if f.Field != nil {
				fields[i] = fmt.Sprintf("%s.%s", subAlias, f.Field.Name)
				if f.Alias != "" {
					fields[i] += " AS " + f.Alias
				}
			}
		}
		selectClause += strings.Join(fields, ", ")
	} else {
		selectClause += subAlias + ".*"
	}

	sql := fmt.Sprintf("SELECT %s FROM (%s) %s", selectClause, subResult.SQL, subAlias)

	// Add LIMIT/OFFSET for outer query
	if q.Limit != nil {
		sql += fmt.Sprintf(" LIMIT %d", *q.Limit)
	}
	if q.Offset != nil {
		sql += fmt.Sprintf(" OFFSET %d", *q.Offset)
	}

	// Handle compound queries (UNION, INTERSECT, EXCEPT)
	for _, compound := range q.Compound {
		compoundResult, err := g.Generate(compound.Query)
		if err != nil {
			return nil, fmt.Errorf("compound query error: %w", err)
		}

		// Map operator to SQL
		op := compound.Operator
		if op == "UNIONALL" {
			op = "UNION ALL"
		}

		// Adjust parameter numbers in compound query
		paramOffset := len(result.Params)
		adjustedSQL := g.adjustParamNumbers(compoundResult.SQL, paramOffset)

		sql += " " + op + " " + adjustedSQL
		result.Params = append(result.Params, compoundResult.Params...)
	}

	result.SQL = sql
	return result, nil
}

// TraversalResult contains the result of processing traversals
type TraversalResult struct {
	FinalAlias     string
	JunctionFields []string // Fields from junction tables (e.g., "j0.role")
}

// hasVariableLengthPath checks if any traversal has a quantifier requiring recursive CTE
func hasVariableLengthPath(travs []*Traversal) bool {
	for _, t := range travs {
		if t.HasQuantifier() && (t.GetMinHops() != 1 || t.GetMaxHops() != 1) {
			return true
		}
	}
	return false
}

// generateWithRecursivePath generates a recursive CTE for variable-length paths
func (g *Generator) generateWithRecursivePath(q *Query, startEntity *Entity, startAlias, cteSQL string, cteParams []interface{}) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: cteParams,
	}

	travs := q.Select.Path.Traversals
	if len(travs) < 2 {
		return nil, fmt.Errorf("variable-length path requires at least relationship and target entity")
	}

	// Find the traversal with the quantifier
	var relTrav, entTrav *Traversal
	var minHops, maxHops int

	for i := 0; i < len(travs); i += 2 {
		if i+1 >= len(travs) {
			return nil, fmt.Errorf("incomplete path: relationship without target entity")
		}
		if travs[i].HasQuantifier() {
			relTrav = travs[i]
			entTrav = travs[i+1]
			minHops = relTrav.GetMinHops()
			maxHops = relTrav.GetMaxHops()
			break
		}
	}

	if relTrav == nil {
		return nil, fmt.Errorf("no quantifier found in path")
	}

	// Find the relationship
	baseDirection := relTrav.BaseDirection()
	rel, err := g.schema.FindRelationship(startEntity.Name, relTrav.Target, baseDirection)
	if err != nil {
		return nil, err
	}

	// Get target entity
	targetEntity, err := g.schema.GetEntity(entTrav.Target)
	if err != nil {
		return nil, err
	}

	// Determine keys based on direction
	var sourceKey, targetKey string
	if baseDirection == "->" {
		sourceKey = rel.FromKey
		targetKey = rel.ToKey
	} else {
		sourceKey = rel.ToKey
		targetKey = rel.FromKey
	}

	// Build the recursive CTE
	// WITH RECURSIVE path_cte AS (
	//   -- Base case
	//   SELECT j.target_id AS id, 1 AS depth FROM junction j
	//   JOIN source s ON j.source_id = s.id WHERE s.external_id = $1
	//   UNION ALL
	//   -- Recursive case
	//   SELECT j.target_id AS id, p.depth + 1 FROM path_cte p
	//   JOIN junction j ON p.id = j.source_id WHERE p.depth < maxHops
	// )
	// SELECT DISTINCT t.* FROM path_cte p JOIN target t ON p.id = t.id
	// WHERE p.depth >= minHops AND t.valid_to = 'infinity'

	var sql strings.Builder

	// Add any existing CTEs
	if cteSQL != "" {
		sql.WriteString(cteSQL[:len(cteSQL)-1]) // Remove trailing space
		sql.WriteString(", ")
	} else {
		sql.WriteString("WITH ")
	}

	sql.WriteString("RECURSIVE path_cte AS (")

	// Base case: first hop from start entity
	sql.WriteString(fmt.Sprintf("SELECT j0.%s AS id, 1 AS depth FROM %s j0 ", targetKey, rel.JoinTable))
	sql.WriteString(fmt.Sprintf("JOIN %s %s ON j0.%s = %s.%s ", startEntity.Table, startAlias, sourceKey, startAlias, startEntity.PrimaryKey))

	// Add temporal filter for start entity
	if startEntity.Temporal != nil {
		sql.WriteString(fmt.Sprintf("WHERE %s.valid_to = 'infinity' ", startAlias))
	} else {
		sql.WriteString("WHERE 1=1 ")
	}

	// Add temporal filter for relationship (junction table)
	if rel.Temporal != nil {
		sql.WriteString(fmt.Sprintf("AND j0.%s = 'infinity' ", rel.Temporal.ValidToColumn))
	}

	// Add ID filter if present
	if q.From.ID != "" {
		result.Params = append(result.Params, q.From.ID)
		sql.WriteString(fmt.Sprintf("AND %s.external_id = $%d ", startAlias, len(result.Params)))
	}

	sql.WriteString("UNION ALL ")

	// Recursive case: follow relationship again (for self-referential relationships)
	sql.WriteString(fmt.Sprintf("SELECT j.%s AS id, p.depth + 1 FROM path_cte p ", targetKey))
	sql.WriteString(fmt.Sprintf("JOIN %s j ON p.id = j.%s ", rel.JoinTable, sourceKey))

	// Add temporal filter for relationship in recursive case
	if rel.Temporal != nil {
		sql.WriteString(fmt.Sprintf("AND j.%s = 'infinity' ", rel.Temporal.ValidToColumn))
	}

	sql.WriteString(fmt.Sprintf("WHERE p.depth < %d", maxHops))

	sql.WriteString(") ")

	// Main query: select from target entity using the path results
	sql.WriteString(fmt.Sprintf("SELECT DISTINCT t1.* FROM path_cte p "))
	sql.WriteString(fmt.Sprintf("JOIN %s t1 ON p.id = t1.%s ", targetEntity.Table, targetEntity.PrimaryKey))
	sql.WriteString(fmt.Sprintf("WHERE p.depth >= %d", minHops))

	// Add temporal filter for target entity
	if targetEntity.Temporal != nil {
		sql.WriteString(" AND t1.valid_to = 'infinity'")
	}

	result.SQL = sql.String()
	return result, nil
}

// processTraversals handles graph traversal path and returns the result
func (g *Generator) processTraversals(travs []*Traversal, currentEntity, prevAlias string, aliases *aliasTracker, joins *[]string) (*TraversalResult, error) {
	result := &TraversalResult{
		FinalAlias:     prevAlias,
		JunctionFields: []string{},
	}

	// Check for variable-length paths - these require recursive CTE handling
	if hasVariableLengthPath(travs) {
		return nil, fmt.Errorf("path quantifiers with variable length (e.g., {1,3}) require recursive CTE - use Generate method instead")
	}

	// Process in pairs: (relationship, entity)
	for i := 0; i < len(travs); i += 2 {
		if i+1 >= len(travs) {
			return nil, fmt.Errorf("incomplete path: relationship without target entity")
		}

		relTrav := travs[i]   // relationship name
		entTrav := travs[i+1] // target entity

		// Use base direction for relationship lookup (strip optional marker)
		baseDirection := relTrav.BaseDirection()
		rel, err := g.schema.FindRelationship(currentEntity, relTrav.Target, baseDirection)
		if err != nil {
			return nil, err
		}

		// Verify the relationship leads to expected entity
		expectedEntity := entTrav.Target
		var actualTarget string
		if baseDirection == "->" {
			actualTarget = rel.ToEntity
		} else {
			actualTarget = rel.FromEntity
		}
		if actualTarget != expectedEntity {
			return nil, fmt.Errorf("relationship %s does not connect to %s (connects to %s)",
				relTrav.Target, expectedEntity, actualTarget)
		}

		prevEntity, _ := g.schema.GetEntity(currentEntity)

		var targetKey string
		var sourceKey string

		if baseDirection == "->" {
			sourceKey = rel.FromKey
			targetKey = rel.ToKey
		} else {
			sourceKey = rel.ToKey
			targetKey = rel.FromKey
		}

		// Determine join type: LEFT JOIN for optional, JOIN for required
		joinType := "JOIN"
		if relTrav.IsOptional() {
			joinType = "LEFT JOIN"
		}

		// Junction table join
		joinAlias := aliases.nextJoin()
		joinCondition := fmt.Sprintf("%s.%s = %s.%s",
			result.FinalAlias, prevEntity.PrimaryKey,
			joinAlias, sourceKey)

		// Add temporal filter for temporal relationships (current state only)
		if rel.Temporal != nil {
			joinCondition += fmt.Sprintf(" AND %s.%s = 'infinity'", joinAlias, rel.Temporal.ValidToColumn)
		}

		*joins = append(*joins, fmt.Sprintf(
			"%s %s %s ON %s",
			joinType, rel.JoinTable, joinAlias,
			joinCondition,
		))

		// Collect junction field if specified on relationship traversal
		if relTrav.Field != "" {
			result.JunctionFields = append(result.JunctionFields, fmt.Sprintf("%s.%s", joinAlias, relTrav.Field))
		}

		// Target table join
		targetEntityDef, err := g.schema.GetEntity(expectedEntity)
		if err != nil {
			return nil, err
		}
		targetAlias := aliases.next(expectedEntity)
		*joins = append(*joins, fmt.Sprintf(
			"%s %s %s ON %s.%s = %s.%s",
			joinType, targetEntityDef.Table, targetAlias,
			joinAlias, targetKey,
			targetAlias, targetEntityDef.PrimaryKey,
		))

		currentEntity = expectedEntity
		result.FinalAlias = targetAlias
	}

	return result, nil
}

// generateAggregate generates SQL for an aggregate function
func (g *Generator) generateAggregate(agg *Aggregate, startAlias, currentEntity string, aliases *aliasTracker, joins *[]string) (string, error) {
	fn := strings.ToUpper(agg.Function)

	distinctStr := ""
	if agg.Distinct {
		distinctStr = "DISTINCT "
	}

	// Build delimiter suffix for STRING_AGG
	delimiterSuffix := ""
	if agg.Delimiter != nil {
		delimiterSuffix = fmt.Sprintf(", '%s'", strings.ReplaceAll(*agg.Delimiter, "'", "''"))
	}

	if agg.Star {
		return fmt.Sprintf("%s(%s*%s)", fn, distinctStr, delimiterSuffix), nil
	}

	if agg.Path != nil {
		// Aggregate over path - need to traverse and count
		travResult, err := g.processTraversals(agg.Path.Traversals, currentEntity, startAlias, aliases, joins)
		if err != nil {
			return "", err
		}
		// For COUNT on path, we count the final entity's id
		lastEntity := agg.Path.Traversals[len(agg.Path.Traversals)-1].Target
		entityDef, err := g.schema.GetEntity(lastEntity)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s(%s%s.%s%s)", fn, distinctStr, travResult.FinalAlias, entityDef.PrimaryKey, delimiterSuffix), nil
	}

	if agg.Field != nil {
		alias := startAlias
		if agg.Field.Entity != "" {
			if a := aliases.get(agg.Field.Entity); a != "" {
				alias = a
			}
		}
		return fmt.Sprintf("%s(%s%s.%s%s)", fn, distinctStr, alias, agg.Field.Name, delimiterSuffix), nil
	}

	return fmt.Sprintf("%s(%s*%s)", fn, distinctStr, delimiterSuffix), nil
}

// generateFieldExprWithAggCheck generates SQL for a field expression and reports if it's an aggregate
func (g *Generator) generateFieldExprWithAggCheck(f *FieldExpr, startAlias, currentEntity string, aliases *aliasTracker, joins *[]string) (string, bool, error) {
	var fieldSQL string
	isAgg := false

	if f.Case != nil {
		caseSQL, err := g.generateCaseExpr(f.Case, startAlias)
		if err != nil {
			return "", false, err
		}
		fieldSQL = caseSQL
	} else if f.Coalesce != nil {
		coalesceSQL, err := g.generateCoalesce(f.Coalesce, startAlias)
		if err != nil {
			return "", false, err
		}
		fieldSQL = coalesceSQL
	} else if f.Nullif != nil {
		nullifSQL, err := g.generateNullif(f.Nullif, startAlias)
		if err != nil {
			return "", false, err
		}
		fieldSQL = nullifSQL
	} else if f.DateArith != nil {
		dateSQL := g.generateDateArithmetic(f.DateArith, startAlias)
		fieldSQL = dateSQL
	} else if f.FuncCall != nil {
		funcSQL := g.generateFuncCall(f.FuncCall, startAlias)
		fieldSQL = funcSQL
	} else if f.Aggregate != nil {
		isAgg = true
		agg, err := g.generateAggregate(f.Aggregate, startAlias, currentEntity, aliases, joins)
		if err != nil {
			return "", false, err
		}
		fieldSQL = agg
	} else if f.Path != nil {
		// Path in field list - traverse to get final entity fields
		travResult, err := g.processTraversals(f.Path.Traversals, currentEntity, startAlias, aliases, joins)
		if err != nil {
			return "", false, err
		}
		// Include junction fields if any were collected during traversal
		if len(travResult.JunctionFields) > 0 {
			parts := []string{fmt.Sprintf("%s.*", travResult.FinalAlias)}
			parts = append(parts, travResult.JunctionFields...)
			fieldSQL = strings.Join(parts, ", ")
		} else {
			fieldSQL = fmt.Sprintf("%s.*", travResult.FinalAlias)
		}
	} else if f.Field != nil {
		alias := startAlias
		if f.Field.Entity != "" {
			if a := aliases.get(f.Field.Entity); a != "" {
				alias = a
			}
		}
		fieldSQL = fmt.Sprintf("%s.%s", alias, f.Field.Name)
	}

	if f.Alias != "" {
		fieldSQL += " AS " + f.Alias
	}

	return fieldSQL, isAgg, nil
}

// generateFieldExpr generates SQL for a field expression (backwards compatible)
func (g *Generator) generateFieldExpr(f *FieldExpr, startAlias, currentEntity string, aliases *aliasTracker, joins *[]string) (string, error) {
	sql, _, err := g.generateFieldExprWithAggCheck(f, startAlias, currentEntity, aliases, joins)
	return sql, err
}

// generateCaseExpr generates SQL for CASE expression
func (g *Generator) generateCaseExpr(c *CaseExpr, defaultAlias string) (string, error) {
	var parts []string
	parts = append(parts, "CASE")

	for _, when := range c.Whens {
		condSQL, err := g.generateSimpleCondition(when.Condition, defaultAlias)
		if err != nil {
			return "", err
		}
		thenSQL := g.generateCaseValue(when.Then, defaultAlias)
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condSQL, thenSQL))
	}

	if c.Else != nil {
		elseSQL := g.generateCaseValue(c.Else, defaultAlias)
		parts = append(parts, fmt.Sprintf("ELSE %s", elseSQL))
	}

	parts = append(parts, "END")
	return strings.Join(parts, " "), nil
}

// generateSimpleCondition generates SQL for condition in CASE WHEN
func (g *Generator) generateSimpleCondition(c *SimpleCondition, defaultAlias string) (string, error) {
	var left string
	if c.Left != nil && c.Left.Field != "" {
		left = fmt.Sprintf("%s.%s", defaultAlias, c.Left.Field)
	}

	op := strings.ToUpper(strings.ReplaceAll(c.Op, " ", ""))

	// Handle IS NULL / IS NOT NULL
	if op == "ISNULL" {
		return fmt.Sprintf("%s IS NULL", left), nil
	}
	if op == "ISNOTNULL" {
		return fmt.Sprintf("%s IS NOT NULL", left), nil
	}

	// For comparison operators
	right := ""
	if c.Right != nil {
		right = g.generateCaseValue(c.Right, defaultAlias)
	}

	return fmt.Sprintf("%s %s %s", left, c.Op, right), nil
}

// generateCaseValue generates SQL for a value in CASE expression
func (g *Generator) generateCaseValue(v *CaseValue, defaultAlias string) string {
	switch {
	case v.String != nil:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(*v.String, "'", "''"))
	case v.Float != nil:
		return fmt.Sprintf("%v", *v.Float)
	case v.Int != nil:
		return fmt.Sprintf("%d", *v.Int)
	case v.Bool != nil:
		if *v.Bool {
			return "TRUE"
		}
		return "FALSE"
	case v.Null:
		return "NULL"
	case v.Field != nil:
		if v.Field.Entity != "" {
			return fmt.Sprintf("%s.%s", v.Field.Entity, v.Field.Name)
		}
		return fmt.Sprintf("%s.%s", defaultAlias, v.Field.Name)
	}
	return ""
}

// generateCoalesce generates SQL for COALESCE expression
func (g *Generator) generateCoalesce(c *Coalesce, defaultAlias string) (string, error) {
	var values []string
	for _, v := range c.Values {
		values = append(values, g.generateCaseValue(v, defaultAlias))
	}
	return fmt.Sprintf("COALESCE(%s)", strings.Join(values, ", ")), nil
}

// generateNullif generates SQL for NULLIF expression
func (g *Generator) generateNullif(n *Nullif, defaultAlias string) (string, error) {
	first := g.generateCaseValue(n.First, defaultAlias)
	second := g.generateCaseValue(n.Second, defaultAlias)
	return fmt.Sprintf("NULLIF(%s, %s)", first, second), nil
}

// generateDateArithmetic generates SQL for date arithmetic expressions
func (g *Generator) generateDateArithmetic(d *DateArithmetic, defaultAlias string) string {
	var fieldRef string
	if d.Left.Entity != "" {
		fieldRef = fmt.Sprintf("%s.%s", d.Left.Entity, d.Left.Name)
	} else {
		fieldRef = fmt.Sprintf("%s.%s", defaultAlias, d.Left.Name)
	}
	return fmt.Sprintf("(%s %s INTERVAL '%s')", fieldRef, d.Operator, d.Interval.Value)
}

// generateFuncCall generates SQL for function calls
func (g *Generator) generateFuncCall(f *FuncCall, defaultAlias string) string {
	var args []string
	for _, arg := range f.Args {
		args = append(args, g.generateFuncArg(arg, defaultAlias))
	}

	fn := strings.ToUpper(f.Name)

	// Handle JSON functions - convert to PostgreSQL operators
	switch fn {
	case "JSON_GET":
		// JSON_GET(field, 'key') -> field->'key'
		if len(args) >= 2 {
			return fmt.Sprintf("(%s->%s)", args[0], args[1])
		}
	case "JSON_TEXT":
		// JSON_TEXT(field, 'key') -> field->>'key'
		if len(args) >= 2 {
			return fmt.Sprintf("(%s->>%s)", args[0], args[1])
		}
	case "JSON_PATH":
		// JSON_PATH(field, 'key1', 'key2') -> field#>'{key1,key2}'
		if len(args) >= 2 {
			pathParts := args[1:]
			return fmt.Sprintf("(%s#>'{%s}')", args[0], strings.Join(pathParts, ","))
		}
	case "JSON_PATH_TEXT":
		// JSON_PATH_TEXT(field, 'key1', 'key2') -> field#>>'{key1,key2}'
		if len(args) >= 2 {
			pathParts := args[1:]
			return fmt.Sprintf("(%s#>>'{%s}')", args[0], strings.Join(pathParts, ","))
		}
	case "JSON_BUILD_OBJECT":
		// JSON_BUILD_OBJECT('key1', val1, 'key2', val2) -> json_build_object(...)
		return fmt.Sprintf("json_build_object(%s)", strings.Join(args, ", "))
	}

	return fmt.Sprintf("%s(%s)", fn, strings.Join(args, ", "))
}

// generateFuncArg generates SQL for a function argument
func (g *Generator) generateFuncArg(arg *FuncArg, defaultAlias string) string {
	switch {
	case arg.String != nil:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(*arg.String, "'", "''"))
	case arg.Float != nil:
		return fmt.Sprintf("%v", *arg.Float)
	case arg.Int != nil:
		return fmt.Sprintf("%d", *arg.Int)
	case arg.Field != nil:
		if arg.Field.Entity != "" {
			return fmt.Sprintf("%s.%s", arg.Field.Entity, arg.Field.Name)
		}
		return fmt.Sprintf("%s.%s", defaultAlias, arg.Field.Name)
	}
	return ""
}

// generateWhereClause generates WHERE with OR support
func (g *Generator) generateWhereClause(where *WhereClause, aliases *aliasTracker, defaultAlias string, paramOffset int) (string, []interface{}, error) {
	var params []interface{}
	var orParts []string

	for _, andGroup := range where.Or {
		andClauses := []string{}
		for _, cond := range andGroup.Conditions {
			clause, condParams, err := g.generateCondition(cond, aliases, defaultAlias, paramOffset+len(params))
			if err != nil {
				return "", nil, err
			}
			params = append(params, condParams...)
			andClauses = append(andClauses, clause)
		}
		if len(andClauses) == 1 {
			orParts = append(orParts, andClauses[0])
		} else {
			orParts = append(orParts, "("+strings.Join(andClauses, " AND ")+")")
		}
	}

	if len(orParts) == 1 {
		return orParts[0], params, nil
	}
	return "(" + strings.Join(orParts, " OR ") + ")", params, nil
}

// generateCondition generates SQL for a single condition
func (g *Generator) generateCondition(cond *Condition, aliases *aliasTracker, defaultAlias string, paramOffset int) (string, []interface{}, error) {
	params := []interface{}{}
	notPrefix := ""
	if cond.Not {
		notPrefix = "NOT "
	}

	// Handle EXISTS subquery
	if cond.Exists != nil {
		subResult, err := g.Generate(cond.Exists)
		if err != nil {
			return "", nil, fmt.Errorf("EXISTS subquery error: %w", err)
		}
		adjustedSQL := g.adjustParamNumbers(subResult.SQL, paramOffset)
		params = append(params, subResult.Params...)
		return fmt.Sprintf("%sEXISTS (%s)", notPrefix, adjustedSQL), params, nil
	}

	// Check for negated path - generates NOT EXISTS subquery
	if cond.Left != nil && len(cond.Left.Path) > 0 && cond.Left.Path[0].IsNegated() {
		return g.generateNegatedPathCondition(cond, aliases, defaultAlias, paramOffset)
	}

	// Build field reference
	var fieldRef string
	if cond.Left != nil && len(cond.Left.Path) > 0 {
		// Path-based field reference - use the target entity alias
		lastTarget := cond.Left.Path[len(cond.Left.Path)-1].Target
		alias := aliases.get(lastTarget)
		if alias == "" {
			alias = defaultAlias
		}
		fieldRef = fmt.Sprintf("%s.%s", alias, cond.Left.Field)
	} else if cond.Left != nil {
		fieldRef = fmt.Sprintf("%s.%s", defaultAlias, cond.Left.Field)
	}

	op := strings.ToUpper(strings.ReplaceAll(cond.Op, " ", ""))

	// Handle IS NULL / IS NOT NULL
	if op == "ISNULL" {
		return fmt.Sprintf("%s%s IS NULL", notPrefix, fieldRef), params, nil
	}
	if op == "ISNOTNULL" {
		return fmt.Sprintf("%s%s IS NOT NULL", notPrefix, fieldRef), params, nil
	}

	// Handle BETWEEN operator
	if op == "BETWEEN" && cond.Between != nil {
		lowVal, lowParams, err := g.generateConditionValue(cond.Between.Low, paramOffset)
		if err != nil {
			return "", nil, err
		}
		params = append(params, lowParams...)

		highVal, highParams, err := g.generateConditionValue(cond.Between.High, paramOffset+len(params))
		if err != nil {
			return "", nil, err
		}
		params = append(params, highParams...)

		return fmt.Sprintf("%s%s BETWEEN %s AND %s", notPrefix, fieldRef, lowVal, highVal), params, nil
	}

	// Handle subquery in condition value
	if cond.Right != nil && cond.Right.Subquery != nil {
		subResult, err := g.Generate(cond.Right.Subquery)
		if err != nil {
			return "", nil, fmt.Errorf("subquery error: %w", err)
		}
		// Adjust parameter numbers in subquery
		adjustedSQL := g.adjustParamNumbers(subResult.SQL, paramOffset)
		params = append(params, subResult.Params...)

		opStr := cond.Op
		if op == "NOTIN" {
			opStr = "NOT IN"
		}
		return fmt.Sprintf("%s%s %s (%s)", notPrefix, fieldRef, opStr, adjustedSQL), params, nil
	}

	// Build value
	var valueRef string
	if cond.Right != nil {
		switch {
		case cond.Right.String != nil:
			params = append(params, *cond.Right.String)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Float != nil:
			params = append(params, *cond.Right.Float)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Int != nil:
			params = append(params, *cond.Right.Int)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Bool != nil:
			params = append(params, *cond.Right.Bool)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Null:
			if cond.Op == "=" {
				return fmt.Sprintf("%s%s IS NULL", notPrefix, fieldRef), params, nil
			}
			return fmt.Sprintf("%s%s IS NOT NULL", notPrefix, fieldRef), params, nil
		case len(cond.Right.List) > 0:
			placeholders := make([]string, len(cond.Right.List))
			for i, v := range cond.Right.List {
				if v.String != nil {
					params = append(params, *v.String)
				} else if v.Float != nil {
					params = append(params, *v.Float)
				} else if v.Int != nil {
					params = append(params, *v.Int)
				}
				placeholders[i] = fmt.Sprintf("$%d", paramOffset+len(params))
			}
			valueRef = fmt.Sprintf("(%s)", strings.Join(placeholders, ", "))
		}
	}

	opStr := cond.Op
	if op == "LIKE" {
		opStr = "LIKE"
	} else if op == "NOTLIKE" {
		opStr = "NOT LIKE"
	} else if op == "NOTIN" {
		opStr = "NOT IN"
	}

	return fmt.Sprintf("%s%s %s %s", notPrefix, fieldRef, opStr, valueRef), params, nil
}

// generateConditionValue generates SQL for a condition value
func (g *Generator) generateConditionValue(val *ConditionValue, paramOffset int) (string, []interface{}, error) {
	params := []interface{}{}

	switch {
	case val.String != nil:
		params = append(params, *val.String)
		return fmt.Sprintf("$%d", paramOffset+len(params)), params, nil
	case val.Float != nil:
		params = append(params, *val.Float)
		return fmt.Sprintf("$%d", paramOffset+len(params)), params, nil
	case val.Int != nil:
		params = append(params, *val.Int)
		return fmt.Sprintf("$%d", paramOffset+len(params)), params, nil
	}

	return "", nil, fmt.Errorf("unsupported value type in BETWEEN")
}

// generateNegatedPathCondition generates NOT EXISTS subquery for negated paths
func (g *Generator) generateNegatedPathCondition(cond *Condition, aliases *aliasTracker, defaultAlias string, paramOffset int) (string, []interface{}, error) {
	params := []interface{}{}
	path := cond.Left.Path

	// Get the starting entity from the schema using the default alias
	// The default alias is like "t0", we need to find which entity it represents
	startEntity := aliases.getEntityByAlias(defaultAlias)
	if startEntity == "" {
		return "", nil, fmt.Errorf("cannot determine entity for alias %s", defaultAlias)
	}

	startEntityDef, err := g.schema.GetEntity(startEntity)
	if err != nil {
		return "", nil, err
	}

	// Process path in pairs: (relationship, entity)
	// Use different prefix for subquery aliases to avoid conflicts with main query
	var subqueryJoins []string
	currentEntity := startEntity
	var lastAlias string
	subqTableCounter := 0
	subqJoinCounter := 0
	nextSubqTable := func() string {
		alias := fmt.Sprintf("s%d", subqTableCounter)
		subqTableCounter++
		return alias
	}
	nextSubqJoin := func() string {
		alias := fmt.Sprintf("sj%d", subqJoinCounter)
		subqJoinCounter++
		return alias
	}

	for i := 0; i < len(path); i += 2 {
		if i+1 >= len(path) {
			return "", nil, fmt.Errorf("incomplete negated path: relationship without target entity")
		}

		relTrav := path[i]
		entTrav := path[i+1]

		baseDirection := relTrav.BaseDirection()
		rel, err := g.schema.FindRelationship(currentEntity, relTrav.Target, baseDirection)
		if err != nil {
			return "", nil, err
		}

		expectedEntity := entTrav.Target
		var actualTarget string
		if baseDirection == "->" {
			actualTarget = rel.ToEntity
		} else {
			actualTarget = rel.FromEntity
		}
		if actualTarget != expectedEntity {
			return "", nil, fmt.Errorf("relationship %s does not connect to %s", relTrav.Target, expectedEntity)
		}

		var targetKey, sourceKey string
		if baseDirection == "->" {
			sourceKey = rel.FromKey
			targetKey = rel.ToKey
		} else {
			sourceKey = rel.ToKey
			targetKey = rel.FromKey
		}

		joinAlias := nextSubqJoin()

		// First junction table in subquery references back to main query
		if i == 0 {
			subqueryJoins = append(subqueryJoins, fmt.Sprintf(
				"%s %s",
				rel.JoinTable, joinAlias,
			))
		} else {
			prevEntityDef, _ := g.schema.GetEntity(currentEntity)
			subqueryJoins = append(subqueryJoins, fmt.Sprintf(
				"JOIN %s %s ON %s.%s = %s.%s",
				rel.JoinTable, joinAlias,
				lastAlias, prevEntityDef.PrimaryKey,
				joinAlias, sourceKey,
			))
		}

		targetEntityDef, err := g.schema.GetEntity(expectedEntity)
		if err != nil {
			return "", nil, err
		}
		targetAlias := nextSubqTable()
		subqueryJoins = append(subqueryJoins, fmt.Sprintf(
			"JOIN %s %s ON %s.%s = %s.%s",
			targetEntityDef.Table, targetAlias,
			joinAlias, targetKey,
			targetAlias, targetEntityDef.PrimaryKey,
		))

		currentEntity = expectedEntity
		lastAlias = targetAlias
	}

	// Build WHERE clause for subquery
	var subqWhere []string

	// Connect back to main query - junction table references main entity
	firstRel := path[0]
	baseDir := firstRel.BaseDirection()
	rel, _ := g.schema.FindRelationship(startEntity, firstRel.Target, baseDir)
	var sourceKey string
	if baseDir == "->" {
		sourceKey = rel.FromKey
	} else {
		sourceKey = rel.ToKey
	}
	subqWhere = append(subqWhere, fmt.Sprintf("sj0.%s = %s.%s", sourceKey, defaultAlias, startEntityDef.PrimaryKey))

	// Add temporal filter for the relationship (junction table)
	if rel.Temporal != nil {
		subqWhere = append(subqWhere, fmt.Sprintf("sj0.%s = 'infinity'", rel.Temporal.ValidToColumn))
	}

	// Get the target field - either from ConditionField.Field or from last Traversal.Field
	targetField := cond.Left.Field
	if targetField == "" && len(path) > 0 {
		// Field is on the last traversal (entity traversal)
		lastTrav := path[len(path)-1]
		targetField = lastTrav.Field
	}

	// Add condition on target field
	if targetField != "" && cond.Right != nil {
		var valueRef string
		switch {
		case cond.Right.String != nil:
			params = append(params, *cond.Right.String)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Int != nil:
			params = append(params, *cond.Right.Int)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Float != nil:
			params = append(params, *cond.Right.Float)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Bool != nil:
			params = append(params, *cond.Right.Bool)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		}

		op := cond.Op
		if strings.ToUpper(strings.ReplaceAll(op, " ", "")) == "NOTLIKE" {
			op = "NOT LIKE"
		} else if strings.ToUpper(strings.ReplaceAll(op, " ", "")) == "NOTIN" {
			op = "NOT IN"
		}

		subqWhere = append(subqWhere, fmt.Sprintf("%s.%s %s %s", lastAlias, targetField, op, valueRef))
	}

	subquery := fmt.Sprintf("SELECT 1 FROM %s WHERE %s",
		strings.Join(subqueryJoins, " "),
		strings.Join(subqWhere, " AND "))

	return fmt.Sprintf("NOT EXISTS (%s)", subquery), params, nil
}

// adjustParamNumbers adjusts $N placeholders in SQL by adding offset
func (g *Generator) adjustParamNumbers(sql string, offset int) string {
	if offset == 0 {
		return sql
	}
	// Simple approach: find $N patterns and increment them
	result := sql
	for i := 20; i >= 1; i-- {
		old := fmt.Sprintf("$%d", i)
		new := fmt.Sprintf("$%d", i+offset)
		result = strings.ReplaceAll(result, old, new)
	}
	return result
}

// generateGroupBy generates GROUP BY clause
func (g *Generator) generateGroupBy(groupBy *GroupByClause, aliases *aliasTracker, defaultAlias string) string {
	parts := make([]string, len(groupBy.Fields))
	for i, f := range groupBy.Fields {
		alias := defaultAlias
		if f.Entity != "" {
			if a := aliases.get(f.Entity); a != "" {
				alias = a
			}
		}
		parts[i] = fmt.Sprintf("%s.%s", alias, f.Name)
	}
	return strings.Join(parts, ", ")
}

// generateHavingClause generates HAVING clause
func (g *Generator) generateHavingClause(having *HavingClause, aliases *aliasTracker, defaultAlias string, paramOffset int) (string, []interface{}, error) {
	var params []interface{}
	var orParts []string

	for _, andGroup := range having.Or {
		andClauses := []string{}
		for _, cond := range andGroup.Conditions {
			clause, condParams, err := g.generateHavingCondition(cond, aliases, defaultAlias, paramOffset+len(params))
			if err != nil {
				return "", nil, err
			}
			params = append(params, condParams...)
			andClauses = append(andClauses, clause)
		}
		if len(andClauses) == 1 {
			orParts = append(orParts, andClauses[0])
		} else {
			orParts = append(orParts, "("+strings.Join(andClauses, " AND ")+")")
		}
	}

	if len(orParts) == 1 {
		return orParts[0], params, nil
	}
	return "(" + strings.Join(orParts, " OR ") + ")", params, nil
}

// generateHavingCondition generates a single HAVING condition
func (g *Generator) generateHavingCondition(cond *HavingCondition, aliases *aliasTracker, defaultAlias string, paramOffset int) (string, []interface{}, error) {
	params := []interface{}{}

	var leftRef string
	if cond.Aggregate != nil {
		var joins []string
		agg, err := g.generateAggregate(cond.Aggregate, defaultAlias, "", aliases, &joins)
		if err != nil {
			return "", nil, err
		}
		leftRef = agg
	} else if cond.Field != nil {
		alias := defaultAlias
		if len(cond.Field.Path) > 0 {
			lastTarget := cond.Field.Path[len(cond.Field.Path)-1].Target
			if a := aliases.get(lastTarget); a != "" {
				alias = a
			}
		}
		leftRef = fmt.Sprintf("%s.%s", alias, cond.Field.Field)
	}

	// Build value
	var valueRef string
	if cond.Right != nil {
		switch {
		case cond.Right.String != nil:
			params = append(params, *cond.Right.String)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Float != nil:
			params = append(params, *cond.Right.Float)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		case cond.Right.Int != nil:
			params = append(params, *cond.Right.Int)
			valueRef = fmt.Sprintf("$%d", paramOffset+len(params))
		}
	}

	return fmt.Sprintf("%s %s %s", leftRef, cond.Op, valueRef), params, nil
}

// generateOrderBy generates ORDER BY clause
func (g *Generator) generateOrderBy(orderBy *OrderByClause, aliases *aliasTracker, defaultAlias string) (string, error) {
	parts := make([]string, len(orderBy.Fields))

	for i, f := range orderBy.Fields {
		var fieldRef string
		if f.Path != nil {
			// Path reference
			lastTarget := f.Path.Traversals[len(f.Path.Traversals)-1].Target
			alias := aliases.get(lastTarget)
			if alias == "" {
				alias = defaultAlias
			}
			fieldRef = fmt.Sprintf("%s.*", alias)
		} else if f.Field != nil {
			alias := defaultAlias
			if f.Field.Entity != "" {
				if a := aliases.get(f.Field.Entity); a != "" {
					alias = a
				}
			}
			fieldRef = fmt.Sprintf("%s.%s", alias, f.Field.Name)
		}

		if f.Direction != "" {
			fieldRef += " " + strings.ToUpper(f.Direction)
		}
		parts[i] = fieldRef
	}

	return strings.Join(parts, ", "), nil
}

// aliasTracker manages table aliases
type aliasTracker struct {
	aliases     map[string]string
	counter     int
	joinCounter int
}

func (a *aliasTracker) next(entity string) string {
	alias := fmt.Sprintf("t%d", a.counter)
	a.counter++
	a.aliases[entity] = alias
	return alias
}

func (a *aliasTracker) nextJoin() string {
	alias := fmt.Sprintf("j%d", a.joinCounter)
	a.joinCounter++
	return alias
}

func (a *aliasTracker) get(entity string) string {
	return a.aliases[entity]
}

func (a *aliasTracker) getEntityByAlias(alias string) string {
	for entity, a2 := range a.aliases {
		if a2 == alias {
			return entity
		}
	}
	return ""
}

func cleanID(id string) string {
	// Remove quotes if present
	if len(id) >= 2 && id[0] == '\'' && id[len(id)-1] == '\'' {
		return id[1 : len(id)-1]
	}
	return id
}

// GenerateStatement transpiles any Statement (SELECT, INSERT, UPDATE, DELETE) to PostgreSQL
func (g *Generator) GenerateStatement(s *Statement) (*GeneratedQuery, error) {
	if s.Select != nil {
		return g.Generate(s.Select)
	}
	if s.Insert != nil {
		return g.GenerateInsert(s.Insert)
	}
	if s.Update != nil {
		return g.GenerateUpdate(s.Update)
	}
	if s.Delete != nil {
		return g.GenerateDelete(s.Delete)
	}
	return nil, fmt.Errorf("empty statement")
}

// GenerateInsert generates SQL for an INSERT statement
func (g *Generator) GenerateInsert(stmt *InsertStmt) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: make([]interface{}, 0),
	}

	// Check if this is a relationship insertion (has path)
	if len(stmt.Path) > 0 {
		return g.generateInsertRelationship(stmt)
	}

	// Get entity from schema
	entity, err := g.schema.GetEntity(stmt.Entity)
	if err != nil {
		return nil, fmt.Errorf("unknown entity %s: %w", stmt.Entity, err)
	}

	var sql strings.Builder

	// Build column list
	columns := make([]string, 0)
	if stmt.ID != "" {
		columns = append(columns, "external_id")
	}
	for _, field := range stmt.Fields {
		// Map field name to column name
		if col, ok := entity.Fields[field]; ok {
			columns = append(columns, col)
		} else {
			columns = append(columns, field)
		}
	}

	// Add temporal columns if entity is temporal
	if entity.Temporal != nil {
		columns = append(columns, entity.Temporal.VersionColumn)
		columns = append(columns, entity.Temporal.ValidFromColumn)
		columns = append(columns, entity.Temporal.ValidToColumn)
	}

	sql.WriteString(fmt.Sprintf("INSERT INTO %s ", entity.Table))

	// Handle INSERT...SELECT
	if stmt.Select != nil {
		if len(columns) > 0 {
			sql.WriteString(fmt.Sprintf("(%s) ", strings.Join(columns, ", ")))
		}
		selectResult, err := g.Generate(stmt.Select)
		if err != nil {
			return nil, fmt.Errorf("INSERT SELECT error: %w", err)
		}
		sql.WriteString(selectResult.SQL)
		result.Params = append(result.Params, selectResult.Params...)
	} else {
		// Handle INSERT...VALUES
		if len(columns) > 0 {
			sql.WriteString(fmt.Sprintf("(%s) ", strings.Join(columns, ", ")))
		}
		sql.WriteString("VALUES ")

		rowStrs := make([]string, 0, len(stmt.Values))
		for _, row := range stmt.Values {
			vals := make([]string, 0)

			// Add external_id if present
			if stmt.ID != "" {
				result.Params = append(result.Params, cleanID(stmt.ID))
				vals = append(vals, fmt.Sprintf("$%d", len(result.Params)))
			}

			// Add field values
			for _, val := range row.Values {
				result.Params = append(result.Params, g.insertValueToInterface(val))
				vals = append(vals, fmt.Sprintf("$%d", len(result.Params)))
			}

			// Add temporal values if applicable
			if entity.Temporal != nil {
				vals = append(vals, "1")           // version
				vals = append(vals, "NOW()")       // valid_from
				vals = append(vals, "'infinity'")  // valid_to
			}

			rowStrs = append(rowStrs, "("+strings.Join(vals, ", ")+")")
		}
		sql.WriteString(strings.Join(rowStrs, ", "))
	}

	// Add RETURNING clause
	if stmt.Returning != nil {
		sql.WriteString(" RETURNING ")
		sql.WriteString(g.generateReturning(stmt.Returning))
	}

	result.SQL = sql.String()
	return result, nil
}

// GenerateUpdate generates SQL for an UPDATE statement
func (g *Generator) GenerateUpdate(stmt *UpdateStmt) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: make([]interface{}, 0),
	}

	// Get entity from schema
	entity, err := g.schema.GetEntity(stmt.Entity)
	if err != nil {
		return nil, fmt.Errorf("unknown entity %s: %w", stmt.Entity, err)
	}

	// For temporal entities without FORCE, we need to create a new version
	if entity.Temporal != nil && !stmt.Force {
		return g.generateUpdateTemporal(stmt, entity)
	}

	// Direct update (non-temporal or FORCE)
	var sql strings.Builder
	sql.WriteString(fmt.Sprintf("UPDATE %s SET ", entity.Table))

	// Build SET clause
	setParts := make([]string, 0, len(stmt.Set))
	for _, set := range stmt.Set {
		// Map field name to column name
		colName := set.Field
		if col, ok := entity.Fields[set.Field]; ok {
			colName = col
		}
		result.Params = append(result.Params, g.insertValueToInterface(set.Value))
		setParts = append(setParts, fmt.Sprintf("%s = $%d", colName, len(result.Params)))
	}
	sql.WriteString(strings.Join(setParts, ", "))

	// Build WHERE clause
	whereParts := make([]string, 0)

	// Add ID filter if present
	if stmt.ID != "" {
		result.Params = append(result.Params, cleanID(stmt.ID))
		whereParts = append(whereParts, fmt.Sprintf("external_id = $%d", len(result.Params)))
	}

	// Add temporal filter for current version
	if entity.Temporal != nil {
		whereParts = append(whereParts, fmt.Sprintf("%s = 'infinity'", entity.Temporal.ValidToColumn))
	}

	// Add user WHERE clause
	if stmt.Where != nil {
		whereSQL, whereParams := g.generateWhereClauseSimple(stmt.Where, entity, len(result.Params))
		result.Params = append(result.Params, whereParams...)
		if whereSQL != "" {
			whereParts = append(whereParts, whereSQL)
		}
	}

	if len(whereParts) > 0 {
		sql.WriteString(" WHERE ")
		sql.WriteString(strings.Join(whereParts, " AND "))
	}

	// Add RETURNING clause
	if stmt.Returning != nil {
		sql.WriteString(" RETURNING ")
		sql.WriteString(g.generateReturning(stmt.Returning))
	}

	result.SQL = sql.String()
	return result, nil
}

// GenerateDelete generates SQL for a DELETE statement
func (g *Generator) GenerateDelete(stmt *DeleteStmt) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: make([]interface{}, 0),
	}

	// Check if this is a relationship deletion (has path)
	if len(stmt.Path) > 0 {
		return g.generateDeleteRelationship(stmt)
	}

	// Get entity from schema
	entity, err := g.schema.GetEntity(stmt.Entity)
	if err != nil {
		return nil, fmt.Errorf("unknown entity %s: %w", stmt.Entity, err)
	}

	// For temporal entities without FORCE, soft delete (set valid_to = NOW())
	if entity.Temporal != nil && !stmt.Force {
		return g.generateDeleteTemporal(stmt, entity)
	}

	// Hard delete (non-temporal or FORCE)
	var sql strings.Builder
	sql.WriteString(fmt.Sprintf("DELETE FROM %s", entity.Table))

	// Build WHERE clause
	whereParts := make([]string, 0)

	// Add ID filter if present
	if stmt.ID != "" {
		result.Params = append(result.Params, cleanID(stmt.ID))
		whereParts = append(whereParts, fmt.Sprintf("external_id = $%d", len(result.Params)))
	}

	// Add temporal filter for current version (even on hard delete, only delete current)
	if entity.Temporal != nil {
		whereParts = append(whereParts, fmt.Sprintf("%s = 'infinity'", entity.Temporal.ValidToColumn))
	}

	// Add user WHERE clause
	if stmt.Where != nil {
		whereSQL, whereParams := g.generateWhereClauseSimple(stmt.Where, entity, len(result.Params))
		result.Params = append(result.Params, whereParams...)
		if whereSQL != "" {
			whereParts = append(whereParts, whereSQL)
		}
	}

	if len(whereParts) > 0 {
		sql.WriteString(" WHERE ")
		sql.WriteString(strings.Join(whereParts, " AND "))
	}

	// Add RETURNING clause
	if stmt.Returning != nil {
		sql.WriteString(" RETURNING ")
		sql.WriteString(g.generateReturning(stmt.Returning))
	}

	result.SQL = sql.String()
	return result, nil
}

// generateUpdateTemporal generates SQL for temporal UPDATE (creates new version)
func (g *Generator) generateUpdateTemporal(stmt *UpdateStmt, entity *Entity) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: make([]interface{}, 0),
	}

	tc := entity.Temporal
	var sql strings.Builder

	// First, close the current version
	sql.WriteString(fmt.Sprintf("UPDATE %s SET %s = NOW() WHERE ", entity.Table, tc.ValidToColumn))

	whereParts := make([]string, 0)
	if stmt.ID != "" {
		result.Params = append(result.Params, cleanID(stmt.ID))
		whereParts = append(whereParts, fmt.Sprintf("external_id = $%d", len(result.Params)))
	}
	whereParts = append(whereParts, fmt.Sprintf("%s = 'infinity'", tc.ValidToColumn))

	if stmt.Where != nil {
		whereSQL, whereParams := g.generateWhereClauseSimple(stmt.Where, entity, len(result.Params))
		result.Params = append(result.Params, whereParams...)
		if whereSQL != "" {
			whereParts = append(whereParts, whereSQL)
		}
	}

	sql.WriteString(strings.Join(whereParts, " AND "))
	sql.WriteString("; ")

	// Then insert a new version with updated fields
	// Build list of all columns we need to copy or update
	sql.WriteString(fmt.Sprintf("INSERT INTO %s (", entity.Table))

	// Collect all field names from entity schema
	allColumns := []string{"external_id"}
	selectParts := []string{"external_id"}

	for fieldName, colName := range entity.Fields {
		allColumns = append(allColumns, colName)
		// Check if this field is being updated
		updated := false
		for _, set := range stmt.Set {
			if set.Field == fieldName || set.Field == colName {
				result.Params = append(result.Params, g.insertValueToInterface(set.Value))
				selectParts = append(selectParts, fmt.Sprintf("$%d", len(result.Params)))
				updated = true
				break
			}
		}
		if !updated {
			selectParts = append(selectParts, colName)
		}
	}

	// Add temporal columns
	allColumns = append(allColumns, tc.VersionColumn, tc.ValidFromColumn, tc.ValidToColumn)
	selectParts = append(selectParts, fmt.Sprintf("%s + 1", tc.VersionColumn), "NOW()", "'infinity'")

	sql.WriteString(strings.Join(allColumns, ", "))
	sql.WriteString(") SELECT ")
	sql.WriteString(strings.Join(selectParts, ", "))
	sql.WriteString(fmt.Sprintf(" FROM %s WHERE ", entity.Table))

	// Same WHERE as the update
	wherePartsSelect := make([]string, 0)
	if stmt.ID != "" {
		wherePartsSelect = append(wherePartsSelect, fmt.Sprintf("external_id = $1"))
	}
	wherePartsSelect = append(wherePartsSelect, fmt.Sprintf("%s = NOW()", tc.ValidToColumn))
	sql.WriteString(strings.Join(wherePartsSelect, " AND "))

	// Add RETURNING clause
	if stmt.Returning != nil {
		sql.WriteString(" RETURNING ")
		sql.WriteString(g.generateReturning(stmt.Returning))
	}

	result.SQL = sql.String()
	return result, nil
}

// generateDeleteTemporal generates SQL for temporal DELETE (soft delete)
func (g *Generator) generateDeleteTemporal(stmt *DeleteStmt, entity *Entity) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: make([]interface{}, 0),
	}

	tc := entity.Temporal
	var sql strings.Builder

	// Soft delete: just set valid_to = NOW()
	sql.WriteString(fmt.Sprintf("UPDATE %s SET %s = NOW()", entity.Table, tc.ValidToColumn))

	// Build WHERE clause
	whereParts := make([]string, 0)

	if stmt.ID != "" {
		result.Params = append(result.Params, cleanID(stmt.ID))
		whereParts = append(whereParts, fmt.Sprintf("external_id = $%d", len(result.Params)))
	}

	whereParts = append(whereParts, fmt.Sprintf("%s = 'infinity'", tc.ValidToColumn))

	if stmt.Where != nil {
		whereSQL, whereParams := g.generateWhereClauseSimple(stmt.Where, entity, len(result.Params))
		result.Params = append(result.Params, whereParams...)
		if whereSQL != "" {
			whereParts = append(whereParts, whereSQL)
		}
	}

	sql.WriteString(" WHERE ")
	sql.WriteString(strings.Join(whereParts, " AND "))

	// Add RETURNING clause
	if stmt.Returning != nil {
		sql.WriteString(" RETURNING ")
		sql.WriteString(g.generateReturning(stmt.Returning))
	}

	result.SQL = sql.String()
	return result, nil
}

// generateInsertRelationship generates SQL for inserting a relationship
func (g *Generator) generateInsertRelationship(stmt *InsertStmt) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: make([]interface{}, 0),
	}

	// Path should be: ->relationship->target_entity:id
	if len(stmt.Path) < 2 {
		return nil, fmt.Errorf("relationship insert requires path with relationship and target entity")
	}

	relTrav := stmt.Path[0]
	targetTrav := stmt.Path[1]

	// Get source entity
	sourceEntity, err := g.schema.GetEntity(stmt.Entity)
	if err != nil {
		return nil, fmt.Errorf("unknown source entity %s: %w", stmt.Entity, err)
	}

	// Find relationship
	rel, err := g.schema.FindRelationship(stmt.Entity, relTrav.Target, relTrav.BaseDirection())
	if err != nil {
		return nil, err
	}

	// Get target entity
	targetEntity, err := g.schema.GetEntity(targetTrav.Target)
	if err != nil {
		return nil, fmt.Errorf("unknown target entity %s: %w", targetTrav.Target, err)
	}

	var sql strings.Builder

	// Determine source and target keys based on direction
	var sourceKey, targetKey string
	if relTrav.BaseDirection() == "->" {
		sourceKey = rel.FromKey
		targetKey = rel.ToKey
	} else {
		sourceKey = rel.ToKey
		targetKey = rel.FromKey
	}

	// Build column list for junction table
	columns := []string{sourceKey, targetKey}

	// Add junction field values if provided
	for _, field := range stmt.Fields {
		columns = append(columns, field)
	}

	sql.WriteString(fmt.Sprintf("INSERT INTO %s (%s) ", rel.JoinTable, strings.Join(columns, ", ")))

	// If VALUES provided, use subquery + values approach
	if len(stmt.Values) > 0 && len(stmt.Values[0].Values) > 0 {
		sql.WriteString("SELECT s.id, t.id")
		for _, val := range stmt.Values[0].Values {
			result.Params = append(result.Params, g.insertValueToInterface(val))
			sql.WriteString(fmt.Sprintf(", $%d", len(result.Params)))
		}
		sql.WriteString(fmt.Sprintf(" FROM %s s, %s t WHERE ", sourceEntity.Table, targetEntity.Table))
	} else {
		sql.WriteString(fmt.Sprintf("SELECT s.id, t.id FROM %s s, %s t WHERE ", sourceEntity.Table, targetEntity.Table))
	}

	// Add source entity filter
	result.Params = append(result.Params, cleanID(stmt.ID))
	sql.WriteString(fmt.Sprintf("s.external_id = $%d", len(result.Params)))
	if sourceEntity.Temporal != nil {
		sql.WriteString(fmt.Sprintf(" AND s.%s = 'infinity'", sourceEntity.Temporal.ValidToColumn))
	}

	// Add target entity filter
	targetID := targetTrav.Field
	if targetID == "" {
		// Check if there's an ID in the traversal (e.g., ->group:admins)
		// The ID might be captured differently - let's check the target
		if len(stmt.Path) >= 2 {
			// For now, require target ID via path syntax like ->member_of->group:admins
			// The target ID would be in a different field
		}
	}
	if targetID != "" {
		result.Params = append(result.Params, cleanID(targetID))
		sql.WriteString(fmt.Sprintf(" AND t.external_id = $%d", len(result.Params)))
	}
	if targetEntity.Temporal != nil {
		sql.WriteString(fmt.Sprintf(" AND t.%s = 'infinity'", targetEntity.Temporal.ValidToColumn))
	}

	result.SQL = sql.String()
	return result, nil
}

// generateDeleteRelationship generates SQL for deleting a relationship
// For temporal relationships, performs soft delete (sets valid_to = NOW())
func (g *Generator) generateDeleteRelationship(stmt *DeleteStmt) (*GeneratedQuery, error) {
	result := &GeneratedQuery{
		Params: make([]interface{}, 0),
	}

	// Path should be: ->relationship->target_entity or ->relationship->target_entity:id
	if len(stmt.Path) < 2 {
		return nil, fmt.Errorf("relationship delete requires path with relationship and target entity")
	}

	relTrav := stmt.Path[0]
	targetTrav := stmt.Path[1]

	// Get source entity
	sourceEntity, err := g.schema.GetEntity(stmt.Entity)
	if err != nil {
		return nil, fmt.Errorf("unknown source entity %s: %w", stmt.Entity, err)
	}

	// Find relationship
	rel, err := g.schema.FindRelationship(stmt.Entity, relTrav.Target, relTrav.BaseDirection())
	if err != nil {
		return nil, err
	}

	// Get target entity
	targetEntity, err := g.schema.GetEntity(targetTrav.Target)
	if err != nil {
		return nil, fmt.Errorf("unknown target entity %s: %w", targetTrav.Target, err)
	}

	var sql strings.Builder

	// Determine source and target keys based on direction
	var sourceKey, targetKey string
	if relTrav.BaseDirection() == "->" {
		sourceKey = rel.FromKey
		targetKey = rel.ToKey
	} else {
		sourceKey = rel.ToKey
		targetKey = rel.FromKey
	}

	// For temporal relationships without FORCE, soft delete (set valid_to = NOW())
	if rel.Temporal != nil && !stmt.Force {
		sql.WriteString(fmt.Sprintf("UPDATE %s SET %s = NOW() WHERE ",
			rel.JoinTable, rel.Temporal.ValidToColumn))

		// Source ID subquery
		result.Params = append(result.Params, cleanID(stmt.ID))
		sql.WriteString(fmt.Sprintf("%s = (SELECT id FROM %s WHERE external_id = $%d",
			sourceKey, sourceEntity.Table, len(result.Params)))
		if sourceEntity.Temporal != nil {
			sql.WriteString(fmt.Sprintf(" AND %s = 'infinity'", sourceEntity.Temporal.ValidToColumn))
		}
		sql.WriteString(")")

		// Target ID subquery (if specified)
		targetID := targetTrav.Field
		if targetID != "" {
			result.Params = append(result.Params, cleanID(targetID))
			sql.WriteString(fmt.Sprintf(" AND %s = (SELECT id FROM %s WHERE external_id = $%d",
				targetKey, targetEntity.Table, len(result.Params)))
			if targetEntity.Temporal != nil {
				sql.WriteString(fmt.Sprintf(" AND %s = 'infinity'", targetEntity.Temporal.ValidToColumn))
			}
			sql.WriteString(")")
		}

		// Only update current (not already deleted) relationships
		sql.WriteString(fmt.Sprintf(" AND %s = 'infinity'", rel.Temporal.ValidToColumn))
	} else {
		// Hard delete (non-temporal or FORCE)
		sql.WriteString(fmt.Sprintf("DELETE FROM %s WHERE ", rel.JoinTable))

		// Source ID subquery
		result.Params = append(result.Params, cleanID(stmt.ID))
		sql.WriteString(fmt.Sprintf("%s = (SELECT id FROM %s WHERE external_id = $%d",
			sourceKey, sourceEntity.Table, len(result.Params)))
		if sourceEntity.Temporal != nil {
			sql.WriteString(fmt.Sprintf(" AND %s = 'infinity'", sourceEntity.Temporal.ValidToColumn))
		}
		sql.WriteString(")")

		// Target ID subquery (if specified)
		targetID := targetTrav.Field
		if targetID != "" {
			result.Params = append(result.Params, cleanID(targetID))
			sql.WriteString(fmt.Sprintf(" AND %s = (SELECT id FROM %s WHERE external_id = $%d",
				targetKey, targetEntity.Table, len(result.Params)))
			if targetEntity.Temporal != nil {
				sql.WriteString(fmt.Sprintf(" AND %s = 'infinity'", targetEntity.Temporal.ValidToColumn))
			}
			sql.WriteString(")")
		}
	}

	result.SQL = sql.String()
	return result, nil
}

// generateWhereClauseSimple generates a simplified WHERE clause for mutations
func (g *Generator) generateWhereClauseSimple(where *WhereClause, entity *Entity, paramOffset int) (string, []interface{}) {
	if where == nil || len(where.Or) == 0 {
		return "", nil
	}

	params := make([]interface{}, 0)
	orParts := make([]string, 0)

	for _, andGroup := range where.Or {
		andParts := make([]string, 0)
		for _, cond := range andGroup.Conditions {
			if cond.Left != nil && cond.Left.Field != "" {
				// Map field to column
				colName := cond.Left.Field
				if col, ok := entity.Fields[cond.Left.Field]; ok {
					colName = col
				}

				op := cond.Op
				if cond.Right != nil {
					if cond.Right.String != nil {
						params = append(params, *cond.Right.String)
						andParts = append(andParts, fmt.Sprintf("%s %s $%d", colName, op, paramOffset+len(params)))
					} else if cond.Right.Int != nil {
						params = append(params, *cond.Right.Int)
						andParts = append(andParts, fmt.Sprintf("%s %s $%d", colName, op, paramOffset+len(params)))
					} else if cond.Right.Float != nil {
						params = append(params, *cond.Right.Float)
						andParts = append(andParts, fmt.Sprintf("%s %s $%d", colName, op, paramOffset+len(params)))
					} else if cond.Right.Bool != nil {
						params = append(params, *cond.Right.Bool)
						andParts = append(andParts, fmt.Sprintf("%s %s $%d", colName, op, paramOffset+len(params)))
					} else if cond.Right.Null {
						if op == "=" {
							andParts = append(andParts, fmt.Sprintf("%s IS NULL", colName))
						} else if op == "!=" {
							andParts = append(andParts, fmt.Sprintf("%s IS NOT NULL", colName))
						}
					}
				} else if strings.Contains(op, "NULL") {
					andParts = append(andParts, fmt.Sprintf("%s %s", colName, op))
				}
			}
		}
		if len(andParts) > 0 {
			orParts = append(orParts, "("+strings.Join(andParts, " AND ")+")")
		}
	}

	if len(orParts) == 0 {
		return "", nil
	}

	if len(orParts) == 1 {
		// Remove outer parentheses for single condition group
		return orParts[0][1 : len(orParts[0])-1], params
	}

	return strings.Join(orParts, " OR "), params
}

// generateReturning generates the RETURNING clause
func (g *Generator) generateReturning(ret *ReturningClause) string {
	if ret.Star {
		return "*"
	}
	parts := make([]string, 0, len(ret.Fields))
	for _, f := range ret.Fields {
		if f.Entity != "" {
			parts = append(parts, fmt.Sprintf("%s.%s", f.Entity, f.Name))
		} else {
			parts = append(parts, f.Name)
		}
	}
	return strings.Join(parts, ", ")
}

// insertValueToInterface converts an InsertValue to an interface{}
func (g *Generator) insertValueToInterface(v *InsertValue) interface{} {
	if v.String != nil {
		return *v.String
	}
	if v.Int != nil {
		return *v.Int
	}
	if v.Float != nil {
		return *v.Float
	}
	if v.Bool != nil {
		return *v.Bool
	}
	if v.Null {
		return nil
	}
	return nil
}
