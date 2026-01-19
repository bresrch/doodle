package doodle

// AccessExplanation represents the full result of an EXPLAIN ACCESS query
type AccessExplanation struct {
	// Subject is the entity requesting access (e.g., user)
	Subject *AccessEntity `json:"subject"`

	// Target is the resource being accessed (e.g., app)
	Target *AccessEntity `json:"target"`

	// CanAccess indicates whether access is allowed
	CanAccess bool `json:"can_access"`

	// Decision is the access decision (ALLOW, DENY, NOT_APPLICABLE)
	Decision AccessDecision `json:"decision"`

	// AccessPaths shows how the subject can reach the target
	AccessPaths []*AccessPath `json:"access_paths"`

	// EffectivePolicy is the policy governing this access
	EffectivePolicy *PolicyInfo `json:"effective_policy,omitempty"`

	// EvaluatedRules shows all rules that were evaluated
	EvaluatedRules []*EvaluatedRule `json:"evaluated_rules"`

	// DenialReasons explains why access was denied (if applicable)
	DenialReasons []*DenialReason `json:"denial_reasons,omitempty"`

	// Recommendation suggests how to fix access issues
	Recommendation *AccessRecommendation `json:"recommendation,omitempty"`
}

// AccessDecision represents the final access decision
type AccessDecision string

const (
	AccessDecisionAllow         AccessDecision = "ALLOW"
	AccessDecisionDeny          AccessDecision = "DENY"
	AccessDecisionNotApplicable AccessDecision = "NOT_APPLICABLE"
)

// AccessEntity represents an entity in the access explanation
type AccessEntity struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id"`
	ExternalID string                 `json:"external_id"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// AccessPath represents a path from subject to target
type AccessPath struct {
	// Type is the kind of access path (direct, group_membership, etc.)
	Type AccessPathType `json:"type"`

	// Path is the sequence of entities and relationships
	Path []PathSegment `json:"path"`

	// ViaGroup is set if access is through group membership
	ViaGroup *AccessEntity `json:"via_group,omitempty"`

	// Assignment contains assignment details if applicable
	Assignment *AssignmentInfo `json:"assignment,omitempty"`
}

// AccessPathType indicates how access was granted
type AccessPathType string

const (
	AccessPathDirect          AccessPathType = "direct"
	AccessPathGroupMembership AccessPathType = "group_membership"
	AccessPathNested          AccessPathType = "nested_group"
)

// PathSegment represents one step in an access path
type PathSegment struct {
	Entity       *AccessEntity `json:"entity,omitempty"`
	Relationship string        `json:"relationship,omitempty"`
}

// AssignmentInfo contains details about an assignment
type AssignmentInfo struct {
	AssignedAt string                 `json:"assigned_at,omitempty"`
	AssignedBy string                 `json:"assigned_by,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// PolicyInfo represents a policy governing access
type PolicyInfo struct {
	ID          string                 `json:"id"`
	ExternalID  string                 `json:"external_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Priority    int                    `json:"priority,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
}

// EvaluatedRule represents a rule that was evaluated
type EvaluatedRule struct {
	// Rule is the rule information
	Rule *RuleInfo `json:"rule"`

	// AppliesToGroup is the group this rule targets
	AppliesToGroup *AccessEntity `json:"applies_to_group,omitempty"`

	// Matched indicates if this rule matched the subject
	Matched bool `json:"matched"`

	// Result is the evaluation result (PASS, FAIL, SKIP)
	Result RuleResult `json:"result"`

	// Reason explains why the rule matched or didn't match
	Reason string `json:"reason,omitempty"`

	// Conditions shows individual condition evaluations
	Conditions []*ConditionEvaluation `json:"conditions,omitempty"`

	// Actions are the actions this rule would take if matched
	Actions *RuleActions `json:"actions,omitempty"`
}

// RuleInfo represents a policy rule
type RuleInfo struct {
	ID          string                 `json:"id"`
	ExternalID  string                 `json:"external_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Priority    int                    `json:"priority"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
}

// RuleResult indicates the outcome of rule evaluation
type RuleResult string

const (
	RuleResultPass RuleResult = "PASS"
	RuleResultFail RuleResult = "FAIL"
	RuleResultSkip RuleResult = "SKIP"
)

// ConditionEvaluation represents the evaluation of a single condition
type ConditionEvaluation struct {
	// Type is the kind of condition (group_membership, mfa_required, etc.)
	Type string `json:"type"`

	// Description is a human-readable description
	Description string `json:"description,omitempty"`

	// Expected is the expected value
	Expected interface{} `json:"expected"`

	// Actual is the actual value
	Actual interface{} `json:"actual"`

	// Result is PASS or FAIL
	Result ConditionResult `json:"result"`

	// Message provides additional context
	Message string `json:"message,omitempty"`
}

// ConditionResult indicates whether a condition passed
type ConditionResult string

const (
	ConditionResultPass ConditionResult = "PASS"
	ConditionResultFail ConditionResult = "FAIL"
)

// RuleActions represents actions a rule takes
type RuleActions struct {
	AllowAccess     bool                   `json:"allow_access"`
	DenyAccess      bool                   `json:"deny_access,omitempty"`
	RequireMFA      bool                   `json:"require_mfa,omitempty"`
	SessionLifetime string                 `json:"session_lifetime,omitempty"`
	Custom          map[string]interface{} `json:"custom,omitempty"`
}

// DenialReason explains why access was denied
type DenialReason struct {
	// Type is the kind of denial (no_assignment, rule_denied, etc.)
	Type DenialType `json:"type"`

	// Message is a human-readable explanation
	Message string `json:"message"`

	// Rule is the rule that caused denial (if applicable)
	Rule *RuleInfo `json:"rule,omitempty"`

	// Condition is the failed condition (if applicable)
	Condition *ConditionEvaluation `json:"condition,omitempty"`
}

// DenialType indicates why access was denied
type DenialType string

const (
	DenialTypeNoAssignment DenialType = "no_assignment"
	DenialTypeNoGroup      DenialType = "no_group_membership"
	DenialTypeRuleDenied   DenialType = "rule_denied"
	DenialTypeCondition    DenialType = "condition_failed"
	DenialTypePolicyDenied DenialType = "policy_denied"
)

// AccessRecommendation suggests how to fix access issues
type AccessRecommendation struct {
	// Action is the recommended action
	Action RecommendationAction `json:"action"`

	// Description explains the recommendation
	Description string `json:"description"`

	// Target is the entity to modify (e.g., group to add user to)
	Target *AccessEntity `json:"target,omitempty"`

	// Details provides additional context
	Details map[string]interface{} `json:"details,omitempty"`
}

// RecommendationAction is the type of recommendation
type RecommendationAction string

const (
	RecommendationAddToGroup     RecommendationAction = "add_to_group"
	RecommendationAssignDirectly RecommendationAction = "assign_directly"
	RecommendationEnableMFA      RecommendationAction = "enable_mfa"
	RecommendationContactAdmin   RecommendationAction = "contact_admin"
)
