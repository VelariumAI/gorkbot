package governance

import "time"

// Condition represents a predicate that should hold before action execution.
type Condition struct {
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    any    `json:"value,omitempty"`
}

// ExpectedEffect represents an expected state change after execution.
type ExpectedEffect struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
	Value  any    `json:"value,omitempty"`
}

// RollbackPlan captures rollback strategy for mutation actions.
type RollbackPlan struct {
	Available   bool   `json:"available"`
	Strategy    string `json:"strategy,omitempty"`
	SnapshotRef string `json:"snapshot_ref,omitempty"`
}

// ProvenanceRef references supporting data for decisions.
type ProvenanceRef struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
	Hash string `json:"hash,omitempty"`
}

// GovernedAction is a normalized execution proposal.
type GovernedAction struct {
	ID              string           `json:"id"`
	MissionID       string           `json:"mission_id,omitempty"`
	Actor           string           `json:"actor"`
	Capability      string           `json:"capability"`
	ToolName        string           `json:"tool_name,omitempty"`
	Workspace       string           `json:"workspace,omitempty"`
	Parameters      map[string]any   `json:"parameters"`
	RiskClass       RiskClass        `json:"risk_class"`
	Preconditions   []Condition      `json:"preconditions,omitempty"`
	ExpectedEffects []ExpectedEffect `json:"expected_effects,omitempty"`
	Rollback        *RollbackPlan    `json:"rollback,omitempty"`
	Provenance      []ProvenanceRef  `json:"provenance,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
}

// GovernanceDecision captures policy + verifier result.
type GovernanceDecision struct {
	ActionID       string          `json:"action_id"`
	Allowed        bool            `json:"allowed"`
	Mode           Mode            `json:"mode"`
	FinalStatus    string          `json:"final_status"`
	ReasonCode     string          `json:"reason_code"`
	Issues         []string        `json:"issues,omitempty"`
	RiskClass      RiskClass       `json:"risk_class"`
	RequiresHuman  bool            `json:"requires_human"`
	ApprovedAction *GovernedAction `json:"approved_action,omitempty"`
	DurationMS     int64           `json:"duration_ms"`
}
