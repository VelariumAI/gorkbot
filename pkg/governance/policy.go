package governance

import (
	"path/filepath"
	"strings"

	"github.com/velariumai/gorkbot/pkg/selfmod"
)

// Policy contains fast deterministic local governance checks.
type Policy struct {
	WorkspaceRoot                      string
	Mode                               Mode
	ExternalSideEffectsRequireApproval bool
	DestructiveRequireApproval         bool
	SelfModificationRequireManifest    bool
	PrivilegedBridgeRequireApproval    bool
	AllowReadOnlyFastPath              bool
	AllowUnknownTools                  bool
}

// DefaultPolicy returns conservative defaults with governance disabled.
func DefaultPolicy() Policy {
	return Policy{
		Mode:                               GOVERNANCE_OFF,
		ExternalSideEffectsRequireApproval: true,
		DestructiveRequireApproval:         true,
		SelfModificationRequireManifest:    true,
		PrivilegedBridgeRequireApproval:    true,
		AllowReadOnlyFastPath:              true,
		AllowUnknownTools:                  false,
	}
}

// Evaluate applies local deterministic policy.
func (p Policy) Evaluate(action GovernedAction) GovernanceDecision {
	decision := GovernanceDecision{
		ActionID:    action.ID,
		Allowed:     true,
		Mode:        p.Mode,
		RiskClass:   action.RiskClass,
		ReasonCode:  REASON_POLICY_ALLOWED,
		FinalStatus: GOVERNANCE_ALLOWED,
	}

	if p.Mode == GOVERNANCE_OFF {
		decision.ApprovedAction = &action
		return decision
	}

	if action.RiskClass == RISK_READ_ONLY && p.AllowReadOnlyFastPath {
		decision.ReasonCode = REASON_FAST_PATH_READ_ONLY
		decision.FinalStatus = GOVERNANCE_ALLOWED
		decision.ApprovedAction = &action
		return decision
	}

	if action.RiskClass == RISK_UNKNOWN && !p.AllowUnknownTools {
		decision.Issues = append(decision.Issues, REASON_UNKNOWN_RISK)
		if p.Mode == GOVERNANCE_AUDIT {
			decision.Allowed = true
			decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
			decision.ReasonCode = REASON_AUDIT_MODE
			decision.ApprovedAction = &action
			return decision
		}
		decision.Allowed = false
		decision.FinalStatus = GOVERNANCE_BLOCKED
		decision.ReasonCode = REASON_UNKNOWN_RISK
		return decision
	}

	switch action.RiskClass {
	case RISK_EXTERNAL_SIDE_EFFECT:
		if p.ExternalSideEffectsRequireApproval {
			decision.RequiresHuman = true
			decision.Issues = append(decision.Issues, REASON_EXTERNAL_SIDE_EFFECT_REQUIRES_APPROVAL)
			if p.Mode == GOVERNANCE_AUDIT {
				decision.Allowed = true
				decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
				decision.ReasonCode = REASON_AUDIT_MODE
				decision.ApprovedAction = &action
				return decision
			}
			decision.Allowed = false
			decision.FinalStatus = GOVERNANCE_REQUIRES_HUMAN
			decision.ReasonCode = REASON_EXTERNAL_SIDE_EFFECT_REQUIRES_APPROVAL
			return decision
		}
	case RISK_DESTRUCTIVE:
		if p.DestructiveRequireApproval {
			decision.RequiresHuman = true
			decision.Issues = append(decision.Issues, REASON_DESTRUCTIVE_REQUIRES_APPROVAL)
			if p.Mode == GOVERNANCE_AUDIT {
				decision.Allowed = true
				decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
				decision.ReasonCode = REASON_AUDIT_MODE
				decision.ApprovedAction = &action
				return decision
			}
			decision.Allowed = false
			decision.FinalStatus = GOVERNANCE_REQUIRES_HUMAN
			decision.ReasonCode = REASON_DESTRUCTIVE_REQUIRES_APPROVAL
			return decision
		}
	case RISK_SELF_MODIFICATION:
		if p.SelfModificationRequireManifest {
			selfDecision := selfmod.ValidateDynamicProposal(selfmod.ValidateInput{
				OperationID: action.ID,
				ToolName:    action.ToolName,
				Mode:        string(p.Mode),
				Parameters:  action.Parameters,
			})
			decision.Issues = append(decision.Issues, selfDecision.IssuesCopy()...)

			if p.Mode == GOVERNANCE_AUDIT {
				decision.Allowed = true
				decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
				decision.ReasonCode = REASON_AUDIT_MODE
				decision.ApprovedAction = &action
				return decision
			}

			if selfDecision.HardBlock || !selfDecision.Allowed {
				decision.Allowed = false
				decision.RequiresHuman = false
				decision.FinalStatus = GOVERNANCE_BLOCKED
				decision.ReasonCode = selfDecision.ReasonCode
				return decision
			}

			if selfDecision.RequiresApproval {
				decision.Allowed = false
				decision.RequiresHuman = true
				decision.FinalStatus = GOVERNANCE_REQUIRES_HUMAN
				decision.ReasonCode = selfDecision.ReasonCode
				return decision
			}
		}
		decision.ApprovedAction = &action
		return decision
	case RISK_PRIVILEGED_BRIDGE:
		if p.PrivilegedBridgeRequireApproval {
			decision.RequiresHuman = true
			decision.Issues = append(decision.Issues, "privileged bridge requires approval")
			if p.Mode == GOVERNANCE_AUDIT {
				decision.Allowed = true
				decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
				decision.ReasonCode = REASON_AUDIT_MODE
				decision.ApprovedAction = &action
				return decision
			}
			decision.Allowed = false
			decision.FinalStatus = GOVERNANCE_REQUIRES_HUMAN
			decision.ReasonCode = REASON_POLICY_BLOCKED
			return decision
		}
	case RISK_LOCAL_MUTATION:
		if p.Mode == GOVERNANCE_CORRECTNESS {
			if len(action.ExpectedEffects) == 0 {
				decision.Issues = append(decision.Issues, "missing expected effects")
			}
			if action.Rollback == nil || !action.Rollback.Available {
				decision.Issues = append(decision.Issues, "missing rollback plan")
			}
			if len(decision.Issues) > 0 {
				decision.Allowed = false
				decision.FinalStatus = GOVERNANCE_BLOCKED
				decision.ReasonCode = REASON_POLICY_BLOCKED
				return decision
			}
		}
		if p.WorkspaceRoot != "" && action.Workspace != "" {
			if !isWithinRoot(action.Workspace, p.WorkspaceRoot) {
				decision.Issues = append(decision.Issues, "workspace outside allowed root")
				if p.Mode == GOVERNANCE_AUDIT {
					decision.Allowed = true
					decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
					decision.ReasonCode = REASON_AUDIT_MODE
					decision.ApprovedAction = &action
					return decision
				}
				decision.Allowed = false
				decision.FinalStatus = GOVERNANCE_BLOCKED
				decision.ReasonCode = REASON_POLICY_BLOCKED
				return decision
			}
		}
	}

	decision.ApprovedAction = &action
	return decision
}

func isWithinRoot(target, root string) bool {
	tClean := filepath.Clean(target)
	rClean := filepath.Clean(root)
	if tClean == rClean {
		return true
	}
	return strings.HasPrefix(tClean, rClean+string(filepath.Separator))
}

// HasSelfModificationManifest checks if self-mod manifest extraction succeeds.
func HasSelfModificationManifest(action GovernedAction) bool {
	if action.Parameters == nil {
		return false
	}
	_, err := selfmod.ExtractManifest(action.Parameters)
	return err == nil
}
