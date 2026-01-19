package doodle

import (
	"fmt"
	"strings"
)

// AccessExplanationQueries contains all SQL queries needed to explain access
type AccessExplanationQueries struct {
	// SubjectQuery fetches the subject entity
	SubjectQuery *GeneratedQuery

	// TargetQuery fetches the target entity
	TargetQuery *GeneratedQuery

	// DirectAccessQuery checks for direct assignment
	DirectAccessQuery *GeneratedQuery

	// GroupAccessQuery finds access via group membership
	GroupAccessQuery *GeneratedQuery

	// PolicyQuery fetches the governing policy
	PolicyQuery *GeneratedQuery

	// RulesQuery fetches policy rules
	RulesQuery *GeneratedQuery

	// EffectiveRulesQuery finds rules that apply to the subject's groups
	EffectiveRulesQuery *GeneratedQuery

	// TemporalMode indicates the temporal query mode used
	TemporalMode string
}

// temporalContext holds temporal query configuration
type temporalContext struct {
	version  *VersionClause
	versions *VersionsClause
	params   []interface{}
}

// buildTemporalCondition generates temporal WHERE conditions for an entity
func (g *Generator) buildTemporalCondition(tc *TemporalConfig, alias string, tctx *temporalContext) (string, []interface{}) {
	if tc == nil {
		return "", nil
	}

	var conditions []string
	var params []interface{}

	if tctx.version != nil {
		if tctx.version.Timestamp != nil {
			// Query at point in time: valid_from <= ts AND valid_to > ts
			params = append(params, *tctx.version.Timestamp)
			paramNum := len(tctx.params) + len(params)
			conditions = append(conditions,
				fmt.Sprintf("%s.%s <= $%d", alias, tc.ValidFromColumn, paramNum),
				fmt.Sprintf("%s.%s > $%d", alias, tc.ValidToColumn, paramNum),
			)
		} else if tctx.version.Number != nil {
			// Query specific version number
			params = append(params, *tctx.version.Number)
			paramNum := len(tctx.params) + len(params)
			conditions = append(conditions,
				fmt.Sprintf("%s.%s = $%d", alias, tc.VersionColumn, paramNum),
			)
		}
	} else if tctx.versions != nil {
		if tctx.versions.All {
			// VERSIONS ALL - no temporal filter
		} else if tctx.versions.Last != nil {
			// VERSIONS LAST N - no filter, handled by ORDER BY/LIMIT
		} else if tctx.versions.Between != nil {
			if tctx.versions.Between.From != nil {
				params = append(params, *tctx.versions.Between.From)
				paramNum := len(tctx.params) + len(params)
				conditions = append(conditions,
					fmt.Sprintf("%s.%s >= $%d", alias, tc.ValidFromColumn, paramNum),
				)
			}
			if tctx.versions.Between.To != nil {
				params = append(params, *tctx.versions.Between.To)
				paramNum := len(tctx.params) + len(params)
				conditions = append(conditions,
					fmt.Sprintf("%s.%s <= $%d", alias, tc.ValidFromColumn, paramNum),
				)
			}
		}
	} else {
		// Default: current version
		conditions = append(conditions,
			fmt.Sprintf("%s.%s = 'infinity'", alias, tc.ValidToColumn),
		)
	}

	return strings.Join(conditions, " AND "), params
}

// getTemporalMode returns a description of the temporal mode
func getTemporalMode(stmt *ExplainAccessStmt) string {
	if stmt.Version != nil {
		if stmt.Version.Timestamp != nil {
			return fmt.Sprintf("point_in_time:%s", *stmt.Version.Timestamp)
		} else if stmt.Version.Number != nil {
			return fmt.Sprintf("version:%d", *stmt.Version.Number)
		}
	} else if stmt.Versions != nil {
		if stmt.Versions.All {
			return "all_versions"
		} else if stmt.Versions.Last != nil {
			return fmt.Sprintf("last_%d_versions", *stmt.Versions.Last)
		} else if stmt.Versions.Between != nil {
			return fmt.Sprintf("between:%s:%s", *stmt.Versions.Between.From, *stmt.Versions.Between.To)
		}
	}
	return "current"
}

// buildEntityTemporalFilter generates temporal WHERE clause for an entity in joins
func buildEntityTemporalFilter(tc *TemporalConfig, alias string, stmt *ExplainAccessStmt, params *[]interface{}) string {
	if tc == nil {
		return ""
	}

	if stmt.Version != nil {
		if stmt.Version.Timestamp != nil {
			*params = append(*params, *stmt.Version.Timestamp)
			paramNum := len(*params)
			return fmt.Sprintf("%s.%s <= $%d AND %s.%s > $%d",
				alias, tc.ValidFromColumn, paramNum,
				alias, tc.ValidToColumn, paramNum)
		} else if stmt.Version.Number != nil {
			*params = append(*params, *stmt.Version.Number)
			paramNum := len(*params)
			return fmt.Sprintf("%s.%s = $%d", alias, tc.VersionColumn, paramNum)
		}
	} else if stmt.Versions != nil {
		if stmt.Versions.All {
			// No filter for all versions
			return ""
		} else if stmt.Versions.Last != nil {
			// No filter, handled by ORDER BY/LIMIT at end
			return ""
		} else if stmt.Versions.Between != nil {
			var conditions []string
			if stmt.Versions.Between.From != nil {
				*params = append(*params, *stmt.Versions.Between.From)
				conditions = append(conditions,
					fmt.Sprintf("%s.%s >= $%d", alias, tc.ValidFromColumn, len(*params)))
			}
			if stmt.Versions.Between.To != nil {
				*params = append(*params, *stmt.Versions.Between.To)
				conditions = append(conditions,
					fmt.Sprintf("%s.%s <= $%d", alias, tc.ValidFromColumn, len(*params)))
			}
			return strings.Join(conditions, " AND ")
		}
	}

	// Default: current version
	return fmt.Sprintf("%s.%s = 'infinity'", alias, tc.ValidToColumn)
}

// buildRelationshipTemporalFilter generates temporal WHERE clause for a relationship in joins
func buildRelationshipTemporalFilter(tc *RelationshipTemporalConfig, alias string, stmt *ExplainAccessStmt, params *[]interface{}) string {
	if tc == nil {
		return ""
	}

	if stmt.Version != nil {
		if stmt.Version.Timestamp != nil {
			*params = append(*params, *stmt.Version.Timestamp)
			paramNum := len(*params)
			return fmt.Sprintf("%s.%s <= $%d AND %s.%s > $%d",
				alias, tc.ValidFromColumn, paramNum,
				alias, tc.ValidToColumn, paramNum)
		}
		// Note: VERSION N (version number) not supported for relationships - they don't have version column
		// Fall through to default
	} else if stmt.Versions != nil {
		if stmt.Versions.All {
			// No filter for all versions
			return ""
		} else if stmt.Versions.Last != nil {
			// No filter, handled by ORDER BY/LIMIT at end
			return ""
		} else if stmt.Versions.Between != nil {
			var conditions []string
			if stmt.Versions.Between.From != nil {
				*params = append(*params, *stmt.Versions.Between.From)
				conditions = append(conditions,
					fmt.Sprintf("%s.%s >= $%d", alias, tc.ValidFromColumn, len(*params)))
			}
			if stmt.Versions.Between.To != nil {
				*params = append(*params, *stmt.Versions.Between.To)
				conditions = append(conditions,
					fmt.Sprintf("%s.%s <= $%d", alias, tc.ValidFromColumn, len(*params)))
			}
			return strings.Join(conditions, " AND ")
		}
	}

	// Default: current version
	return fmt.Sprintf("%s.%s = 'infinity'", alias, tc.ValidToColumn)
}

// GenerateAccessExplanation generates SQL queries for EXPLAIN ACCESS
func (g *Generator) GenerateAccessExplanation(stmt *ExplainAccessStmt) (*AccessExplanationQueries, error) {
	result := &AccessExplanationQueries{
		TemporalMode: getTemporalMode(stmt),
	}

	// 1. Generate query to fetch the subject
	subjectQuery, err := g.generateSubjectQueryTemporal(stmt.FromEntity, stmt.FromID, stmt.Version, stmt.Versions)
	if err != nil {
		return nil, fmt.Errorf("generating subject query: %w", err)
	}
	result.SubjectQuery = subjectQuery

	// 2. Generate query to fetch the target
	targetQuery, err := g.generateTargetQueryTemporal(stmt.ToEntity, stmt.ToID, stmt.Version, stmt.Versions)
	if err != nil {
		return nil, fmt.Errorf("generating target query: %w", err)
	}
	result.TargetQuery = targetQuery

	// 3. Generate query to check direct access
	directQuery, err := g.generateDirectAccessQuery(stmt)
	if err != nil {
		// Direct access might not be defined, that's OK
		result.DirectAccessQuery = nil
	} else {
		result.DirectAccessQuery = directQuery
	}

	// 4. Generate query to find group-based access
	groupQuery, err := g.generateGroupAccessQuery(stmt)
	if err != nil {
		// Group access might not be defined
		result.GroupAccessQuery = nil
	} else {
		result.GroupAccessQuery = groupQuery
	}

	// 5. Generate query to find governing policy
	policyQuery, err := g.generatePolicyQuery(stmt)
	if err != nil {
		result.PolicyQuery = nil
	} else {
		result.PolicyQuery = policyQuery
	}

	// 6. Generate query to find policy rules
	rulesQuery, err := g.generateRulesQuery(stmt)
	if err != nil {
		result.RulesQuery = nil
	} else {
		result.RulesQuery = rulesQuery
	}

	// 7. Generate query to find effective rules (rules that apply to subject's groups)
	effectiveRulesQuery, err := g.generateEffectiveRulesQuery(stmt)
	if err != nil {
		result.EffectiveRulesQuery = nil
	} else {
		result.EffectiveRulesQuery = effectiveRulesQuery
	}

	return result, nil
}

// generateSubjectQuery creates a query to fetch the subject entity (current version)
func (g *Generator) generateSubjectQuery(entityType, entityID string) (*GeneratedQuery, error) {
	return g.generateSubjectQueryTemporal(entityType, entityID, nil, nil)
}

// generateSubjectQueryTemporal creates a query to fetch the subject entity with temporal support
func (g *Generator) generateSubjectQueryTemporal(entityType, entityID string, version *VersionClause, versions *VersionsClause) (*GeneratedQuery, error) {
	entity, err := g.schema.GetEntity(entityType)
	if err != nil {
		return nil, err
	}

	params := []interface{}{entityID}
	sql := fmt.Sprintf("SELECT * FROM %s WHERE external_id = $1", entity.Table)

	if entity.Temporal != nil {
		tctx := &temporalContext{version: version, versions: versions, params: params}
		condition, extraParams := g.buildTemporalCondition(entity.Temporal, entity.Table, tctx)
		if condition != "" {
			sql += " AND " + condition
			params = append(params, extraParams...)
		}
	}

	// Add ordering for VERSIONS LAST N
	if versions != nil && versions.Last != nil && entity.Temporal != nil {
		sql += fmt.Sprintf(" ORDER BY %s DESC LIMIT %d", entity.Temporal.VersionColumn, *versions.Last)
	}

	return &GeneratedQuery{
		SQL:    sql,
		Params: params,
	}, nil
}

// generateTargetQuery creates a query to fetch the target entity (current version)
func (g *Generator) generateTargetQuery(entityType, entityID string) (*GeneratedQuery, error) {
	return g.generateTargetQueryTemporal(entityType, entityID, nil, nil)
}

// generateTargetQueryTemporal creates a query to fetch the target entity with temporal support
func (g *Generator) generateTargetQueryTemporal(entityType, entityID string, version *VersionClause, versions *VersionsClause) (*GeneratedQuery, error) {
	entity, err := g.schema.GetEntity(entityType)
	if err != nil {
		return nil, err
	}

	params := []interface{}{entityID}
	sql := fmt.Sprintf("SELECT * FROM %s WHERE external_id = $1", entity.Table)

	if entity.Temporal != nil {
		tctx := &temporalContext{version: version, versions: versions, params: params}
		condition, extraParams := g.buildTemporalCondition(entity.Temporal, entity.Table, tctx)
		if condition != "" {
			sql += " AND " + condition
			params = append(params, extraParams...)
		}
	}

	// Add ordering for VERSIONS LAST N
	if versions != nil && versions.Last != nil && entity.Temporal != nil {
		sql += fmt.Sprintf(" ORDER BY %s DESC LIMIT %d", entity.Temporal.VersionColumn, *versions.Last)
	}

	return &GeneratedQuery{
		SQL:    sql,
		Params: params,
	}, nil
}

// generateDirectAccessQuery checks for direct assignment from subject to target
func (g *Generator) generateDirectAccessQuery(stmt *ExplainAccessStmt) (*GeneratedQuery, error) {
	// Look for a direct relationship like "assigned_to" from subject to target
	directRels := []string{"assigned_to", "has_access", "can_access", "direct_access"}

	for _, relName := range directRels {
		rel, err := g.schema.FindRelationship(stmt.FromEntity, relName, "->")
		if err != nil {
			continue
		}
		if rel.ToEntity != stmt.ToEntity {
			continue
		}

		// Found a direct relationship
		fromEntity, _ := g.schema.GetEntity(stmt.FromEntity)
		toEntity, _ := g.schema.GetEntity(stmt.ToEntity)

		params := []interface{}{stmt.FromID, stmt.ToID}

		var sql strings.Builder
		sql.WriteString(fmt.Sprintf(
			"SELECT s.*, t.*, j.* FROM %s s ",
			fromEntity.Table))
		sql.WriteString(fmt.Sprintf(
			"JOIN %s j ON s.%s = j.%s ",
			rel.JoinTable, fromEntity.PrimaryKey, rel.FromKey))
		sql.WriteString(fmt.Sprintf(
			"JOIN %s t ON j.%s = t.%s ",
			toEntity.Table, rel.ToKey, toEntity.PrimaryKey))
		sql.WriteString("WHERE s.external_id = $1 AND t.external_id = $2")

		// Add temporal filters
		if filter := buildEntityTemporalFilter(fromEntity.Temporal, "s", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}
		if filter := buildEntityTemporalFilter(toEntity.Temporal, "t", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}
		if filter := buildRelationshipTemporalFilter(rel.Temporal, "j", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}

		return &GeneratedQuery{
			SQL:    sql.String(),
			Params: params,
		}, nil
	}

	return nil, fmt.Errorf("no direct access relationship found")
}

// generateGroupAccessQuery finds access through group membership
func (g *Generator) generateGroupAccessQuery(stmt *ExplainAccessStmt) (*GeneratedQuery, error) {
	// Look for: subject -> member_of -> group -> has_access -> target
	memberRel, err := g.schema.FindRelationship(stmt.FromEntity, "member_of", "->")
	if err != nil {
		return nil, err
	}

	groupEntity := memberRel.ToEntity
	accessRels := []string{"has_access", "can_access", "assigned_to"}

	for _, relName := range accessRels {
		accessRel, err := g.schema.FindRelationship(groupEntity, relName, "->")
		if err != nil {
			continue
		}
		if accessRel.ToEntity != stmt.ToEntity {
			continue
		}

		// Found the path: subject -> group -> target
		fromEntity, _ := g.schema.GetEntity(stmt.FromEntity)
		grpEntity, _ := g.schema.GetEntity(groupEntity)
		toEntity, _ := g.schema.GetEntity(stmt.ToEntity)

		params := []interface{}{stmt.FromID, stmt.ToID}

		var sql strings.Builder
		sql.WriteString(fmt.Sprintf(
			"SELECT s.external_id AS subject_id, g.*, t.external_id AS target_id, "+
				"j1.* AS membership, j2.* AS access "+
				"FROM %s s ",
			fromEntity.Table))
		sql.WriteString(fmt.Sprintf(
			"JOIN %s j1 ON s.%s = j1.%s ",
			memberRel.JoinTable, fromEntity.PrimaryKey, memberRel.FromKey))
		sql.WriteString(fmt.Sprintf(
			"JOIN %s g ON j1.%s = g.%s ",
			grpEntity.Table, memberRel.ToKey, grpEntity.PrimaryKey))
		sql.WriteString(fmt.Sprintf(
			"JOIN %s j2 ON g.%s = j2.%s ",
			accessRel.JoinTable, grpEntity.PrimaryKey, accessRel.FromKey))
		sql.WriteString(fmt.Sprintf(
			"JOIN %s t ON j2.%s = t.%s ",
			toEntity.Table, accessRel.ToKey, toEntity.PrimaryKey))
		sql.WriteString("WHERE s.external_id = $1 AND t.external_id = $2")

		// Add temporal filters
		if filter := buildEntityTemporalFilter(fromEntity.Temporal, "s", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}
		if filter := buildEntityTemporalFilter(grpEntity.Temporal, "g", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}
		if filter := buildEntityTemporalFilter(toEntity.Temporal, "t", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}
		if filter := buildRelationshipTemporalFilter(memberRel.Temporal, "j1", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}
		if filter := buildRelationshipTemporalFilter(accessRel.Temporal, "j2", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}

		return &GeneratedQuery{
			SQL:    sql.String(),
			Params: params,
		}, nil
	}

	return nil, fmt.Errorf("no group access path found")
}

// generatePolicyQuery finds the policy governing the target
func (g *Generator) generatePolicyQuery(stmt *ExplainAccessStmt) (*GeneratedQuery, error) {
	// Look for: target -> governed_by -> policy
	policyRels := []string{"governed_by", "has_policy", "policy"}

	for _, relName := range policyRels {
		rel, err := g.schema.FindRelationship(stmt.ToEntity, relName, "->")
		if err != nil {
			continue
		}

		toEntity, _ := g.schema.GetEntity(stmt.ToEntity)
		policyEntity, _ := g.schema.GetEntity(rel.ToEntity)

		params := []interface{}{stmt.ToID}

		var sql strings.Builder
		sql.WriteString(fmt.Sprintf(
			"SELECT p.* FROM %s t ",
			toEntity.Table))
		sql.WriteString(fmt.Sprintf(
			"JOIN %s j ON t.%s = j.%s ",
			rel.JoinTable, toEntity.PrimaryKey, rel.FromKey))
		sql.WriteString(fmt.Sprintf(
			"JOIN %s p ON j.%s = p.%s ",
			policyEntity.Table, rel.ToKey, policyEntity.PrimaryKey))
		sql.WriteString("WHERE t.external_id = $1")

		// Add temporal filters
		if filter := buildEntityTemporalFilter(toEntity.Temporal, "t", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}
		if filter := buildEntityTemporalFilter(policyEntity.Temporal, "p", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}
		if filter := buildRelationshipTemporalFilter(rel.Temporal, "j", stmt, &params); filter != "" {
			sql.WriteString(" AND " + filter)
		}

		return &GeneratedQuery{
			SQL:    sql.String(),
			Params: params,
		}, nil
	}

	return nil, fmt.Errorf("no policy relationship found")
}

// generateRulesQuery finds all rules for the target's policy
func (g *Generator) generateRulesQuery(stmt *ExplainAccessStmt) (*GeneratedQuery, error) {
	// Look for: target -> governed_by -> policy -> has_rule -> rule
	policyRels := []string{"governed_by", "has_policy", "policy"}
	ruleRels := []string{"has_rule", "rules", "contains_rule"}

	for _, policyRelName := range policyRels {
		policyRel, err := g.schema.FindRelationship(stmt.ToEntity, policyRelName, "->")
		if err != nil {
			continue
		}

		policyEntityName := policyRel.ToEntity

		for _, ruleRelName := range ruleRels {
			ruleRel, err := g.schema.FindRelationship(policyEntityName, ruleRelName, "->")
			if err != nil {
				continue
			}

			toEntity, _ := g.schema.GetEntity(stmt.ToEntity)
			policyEntity, _ := g.schema.GetEntity(policyEntityName)
			ruleEntity, _ := g.schema.GetEntity(ruleRel.ToEntity)

			params := []interface{}{stmt.ToID}

			var sql strings.Builder
			sql.WriteString(fmt.Sprintf(
				"SELECT r.*, p.external_id AS policy_id FROM %s t ",
				toEntity.Table))
			sql.WriteString(fmt.Sprintf(
				"JOIN %s j1 ON t.%s = j1.%s ",
				policyRel.JoinTable, toEntity.PrimaryKey, policyRel.FromKey))
			sql.WriteString(fmt.Sprintf(
				"JOIN %s p ON j1.%s = p.%s ",
				policyEntity.Table, policyRel.ToKey, policyEntity.PrimaryKey))
			sql.WriteString(fmt.Sprintf(
				"JOIN %s j2 ON p.%s = j2.%s ",
				ruleRel.JoinTable, policyEntity.PrimaryKey, ruleRel.FromKey))
			sql.WriteString(fmt.Sprintf(
				"JOIN %s r ON j2.%s = r.%s ",
				ruleEntity.Table, ruleRel.ToKey, ruleEntity.PrimaryKey))
			sql.WriteString("WHERE t.external_id = $1")

			// Add temporal filters
			if filter := buildEntityTemporalFilter(toEntity.Temporal, "t", stmt, &params); filter != "" {
				sql.WriteString(" AND " + filter)
			}
			if filter := buildEntityTemporalFilter(policyEntity.Temporal, "p", stmt, &params); filter != "" {
				sql.WriteString(" AND " + filter)
			}
			if filter := buildEntityTemporalFilter(ruleEntity.Temporal, "r", stmt, &params); filter != "" {
				sql.WriteString(" AND " + filter)
			}

			sql.WriteString(" ORDER BY r.id")

			return &GeneratedQuery{
				SQL:    sql.String(),
				Params: params,
			}, nil
		}
	}

	return nil, fmt.Errorf("no rules relationship found")
}

// generateEffectiveRulesQuery finds rules that apply to the subject's groups
func (g *Generator) generateEffectiveRulesQuery(stmt *ExplainAccessStmt) (*GeneratedQuery, error) {
	// This is the cross-path join:
	// subject -> member_of -> group
	// target -> governed_by -> policy -> has_rule -> rule -> applies_to -> group
	// WHERE subject's group = rule's target group

	memberRel, err := g.schema.FindRelationship(stmt.FromEntity, "member_of", "->")
	if err != nil {
		return nil, err
	}

	// Find policy relationship
	policyRels := []string{"governed_by", "has_policy"}
	var policyRel *Relationship
	for _, name := range policyRels {
		policyRel, err = g.schema.FindRelationship(stmt.ToEntity, name, "->")
		if err == nil {
			break
		}
	}
	if policyRel == nil {
		return nil, fmt.Errorf("no policy relationship found")
	}

	// Find rule relationship
	ruleRels := []string{"has_rule", "rules"}
	var ruleRel *Relationship
	for _, name := range ruleRels {
		ruleRel, err = g.schema.FindRelationship(policyRel.ToEntity, name, "->")
		if err == nil {
			break
		}
	}
	if ruleRel == nil {
		return nil, fmt.Errorf("no rule relationship found")
	}

	// Find applies_to relationship
	appliesToRels := []string{"applies_to", "targets", "for_group"}
	var appliesToRel *Relationship
	for _, name := range appliesToRels {
		appliesToRel, err = g.schema.FindRelationship(ruleRel.ToEntity, name, "->")
		if err == nil {
			break
		}
	}
	if appliesToRel == nil {
		return nil, fmt.Errorf("no applies_to relationship found")
	}

	// Build the query
	fromEntity, _ := g.schema.GetEntity(stmt.FromEntity)
	groupEntity, _ := g.schema.GetEntity(memberRel.ToEntity)
	toEntity, _ := g.schema.GetEntity(stmt.ToEntity)
	policyEntity, _ := g.schema.GetEntity(policyRel.ToEntity)
	ruleEntity, _ := g.schema.GetEntity(ruleRel.ToEntity)

	params := []interface{}{stmt.FromID, stmt.ToID}

	var sql strings.Builder

	// Select rule info and the group it applies to
	sql.WriteString("SELECT DISTINCT ")
	sql.WriteString("r.*, ")
	sql.WriteString("g.external_id AS user_group_id, ")
	sql.WriteString("g.name AS user_group_name, ")
	sql.WriteString("rg.external_id AS rule_target_group_id, ")
	sql.WriteString("rg.name AS rule_target_group_name ")

	// From subject's groups
	sql.WriteString(fmt.Sprintf("FROM %s s ", fromEntity.Table))
	sql.WriteString(fmt.Sprintf(
		"JOIN %s jm ON s.%s = jm.%s ",
		memberRel.JoinTable, fromEntity.PrimaryKey, memberRel.FromKey))
	sql.WriteString(fmt.Sprintf(
		"JOIN %s g ON jm.%s = g.%s ",
		groupEntity.Table, memberRel.ToKey, groupEntity.PrimaryKey))

	// From target's policy rules
	sql.WriteString(fmt.Sprintf(
		"JOIN %s t ON t.external_id = $2 ",
		toEntity.Table))
	sql.WriteString(fmt.Sprintf(
		"JOIN %s jp ON t.%s = jp.%s ",
		policyRel.JoinTable, toEntity.PrimaryKey, policyRel.FromKey))
	sql.WriteString(fmt.Sprintf(
		"JOIN %s p ON jp.%s = p.%s ",
		policyEntity.Table, policyRel.ToKey, policyEntity.PrimaryKey))
	sql.WriteString(fmt.Sprintf(
		"JOIN %s jr ON p.%s = jr.%s ",
		ruleRel.JoinTable, policyEntity.PrimaryKey, ruleRel.FromKey))
	sql.WriteString(fmt.Sprintf(
		"JOIN %s r ON jr.%s = r.%s ",
		ruleEntity.Table, ruleRel.ToKey, ruleEntity.PrimaryKey))
	sql.WriteString(fmt.Sprintf(
		"JOIN %s ja ON r.%s = ja.%s ",
		appliesToRel.JoinTable, ruleEntity.PrimaryKey, appliesToRel.FromKey))
	sql.WriteString(fmt.Sprintf(
		"JOIN %s rg ON ja.%s = rg.%s ",
		groupEntity.Table, appliesToRel.ToKey, groupEntity.PrimaryKey))

	// The key join: user's group matches rule's target group
	sql.WriteString(fmt.Sprintf("WHERE s.external_id = $1 AND g.%s = rg.%s",
		groupEntity.PrimaryKey, groupEntity.PrimaryKey))

	// Temporal filters
	if filter := buildEntityTemporalFilter(fromEntity.Temporal, "s", stmt, &params); filter != "" {
		sql.WriteString(" AND " + filter)
	}
	if filter := buildEntityTemporalFilter(groupEntity.Temporal, "g", stmt, &params); filter != "" {
		sql.WriteString(" AND " + filter)
	}
	if filter := buildEntityTemporalFilter(groupEntity.Temporal, "rg", stmt, &params); filter != "" {
		sql.WriteString(" AND " + filter)
	}
	if filter := buildEntityTemporalFilter(toEntity.Temporal, "t", stmt, &params); filter != "" {
		sql.WriteString(" AND " + filter)
	}
	if filter := buildEntityTemporalFilter(policyEntity.Temporal, "p", stmt, &params); filter != "" {
		sql.WriteString(" AND " + filter)
	}
	if filter := buildEntityTemporalFilter(ruleEntity.Temporal, "r", stmt, &params); filter != "" {
		sql.WriteString(" AND " + filter)
	}

	sql.WriteString(" ORDER BY r.id")

	return &GeneratedQuery{
		SQL:    sql.String(),
		Params: params,
	}, nil
}

// GenerateAccessExplanationSQL returns a single comprehensive SQL query
// that can be used to explain access (for debugging/testing)
func (g *Generator) GenerateAccessExplanationSQL(stmt *ExplainAccessStmt) (string, error) {
	queries, err := g.GenerateAccessExplanation(stmt)
	if err != nil {
		return "", err
	}

	var parts []string

	// Include temporal mode header
	parts = append(parts, fmt.Sprintf("-- Temporal Mode: %s\n", queries.TemporalMode))

	parts = append(parts, fmt.Sprintf("-- Subject Query:\n%s\n", queries.SubjectQuery.SQL))
	parts = append(parts, fmt.Sprintf("-- Target Query:\n%s\n", queries.TargetQuery.SQL))

	if queries.DirectAccessQuery != nil {
		parts = append(parts, fmt.Sprintf("-- Direct Access Query:\n%s\n", queries.DirectAccessQuery.SQL))
	}
	if queries.GroupAccessQuery != nil {
		parts = append(parts, fmt.Sprintf("-- Group Access Query:\n%s\n", queries.GroupAccessQuery.SQL))
	}
	if queries.PolicyQuery != nil {
		parts = append(parts, fmt.Sprintf("-- Policy Query:\n%s\n", queries.PolicyQuery.SQL))
	}
	if queries.RulesQuery != nil {
		parts = append(parts, fmt.Sprintf("-- Rules Query:\n%s\n", queries.RulesQuery.SQL))
	}
	if queries.EffectiveRulesQuery != nil {
		parts = append(parts, fmt.Sprintf("-- Effective Rules Query:\n%s\n", queries.EffectiveRulesQuery.SQL))
	}

	return strings.Join(parts, "\n"), nil
}
