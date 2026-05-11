package governance

import (
	"context"
	"time"
)

// ApprovalDecision captures the human approval outcome.
type ApprovalDecision string

const (
	APPROVAL_GRANTED      ApprovalDecision = "APPROVAL_GRANTED"
	APPROVAL_DENIED       ApprovalDecision = "APPROVAL_DENIED"
	APPROVAL_TIMEOUT      ApprovalDecision = "APPROVAL_TIMEOUT"
	APPROVAL_CANCELLED    ApprovalDecision = "APPROVAL_CANCELLED"
	APPROVAL_NOT_REQUIRED ApprovalDecision = "APPROVAL_NOT_REQUIRED"
	APPROVAL_UNAVAILABLE  ApprovalDecision = "APPROVAL_UNAVAILABLE"
)

// ApprovalScope controls how long an approval decision is reused.
type ApprovalScope string

const (
	APPROVAL_ONCE    ApprovalScope = "APPROVAL_ONCE"
	APPROVAL_SESSION ApprovalScope = "APPROVAL_SESSION"
	APPROVAL_ALWAYS  ApprovalScope = "APPROVAL_ALWAYS"
	APPROVAL_NEVER   ApprovalScope = "APPROVAL_NEVER"
)

// ApprovalRequest is sent to a human approval handler.
type ApprovalRequest struct {
	ActionID        string           `json:"action_id"`
	ToolName        string           `json:"tool_name"`
	Capability      string           `json:"capability"`
	RiskClass       RiskClass        `json:"risk_class"`
	ReasonCode      string           `json:"reason_code"`
	Summary         string           `json:"summary"`
	Parameters      map[string]any   `json:"parameters,omitempty"`
	RedactedParams  map[string]any   `json:"redacted_params,omitempty"`
	ExpectedEffects []ExpectedEffect `json:"expected_effects,omitempty"`
	Rollback        *RollbackPlan    `json:"rollback,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	Timeout         time.Duration    `json:"-"`
}

// ApprovalResult is returned by the approval handler.
type ApprovalResult struct {
	ActionID   string           `json:"action_id"`
	Decision   ApprovalDecision `json:"decision"`
	Scope      ApprovalScope    `json:"scope"`
	Reason     string           `json:"reason,omitempty"`
	ApprovedBy string           `json:"approved_by,omitempty"`
	DurationMS int64            `json:"duration_ms"`
}

// ApprovalHandler asks a human approval channel for a decision.
type ApprovalHandler interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResult, error)
}

// ApprovalHandlerFunc adapts a function into an ApprovalHandler.
type ApprovalHandlerFunc func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error)

// RequestApproval implements ApprovalHandler.
func (f ApprovalHandlerFunc) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
	return f(ctx, req)
}

// ApprovalGranted returns true when approval is explicitly granted.
func ApprovalGranted(result ApprovalResult) bool {
	if result.Decision != APPROVAL_GRANTED {
		return false
	}
	switch result.Scope {
	case APPROVAL_ONCE, APPROVAL_SESSION, APPROVAL_ALWAYS:
		return true
	default:
		return false
	}
}
