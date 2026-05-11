package governance

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
		if p.SelfModificationRequireManifest && !HasSelfModificationManifest(action) {
			decision.Issues = append(decision.Issues, REASON_SELF_MODIFICATION_REQUIRES_MANIFEST)
			if p.Mode == GOVERNANCE_AUDIT {
				decision.Allowed = true
				decision.FinalStatus = GOVERNANCE_AUDIT_ONLY
				decision.ReasonCode = REASON_AUDIT_MODE
				decision.ApprovedAction = &action
				return decision
			}
			decision.Allowed = false
			decision.FinalStatus = GOVERNANCE_BLOCKED
			decision.ReasonCode = REASON_SELF_MODIFICATION_REQUIRES_MANIFEST
			return decision
		}
		decision.RequiresHuman = true
		decision.Issues = append(decision.Issues, "self modification requires approval")
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

// HasSelfModificationManifest checks for minimal manifest structure in action parameters.
func HasSelfModificationManifest(action GovernedAction) bool {
	for _, p := range action.Provenance {
		if strings.EqualFold(p.Kind, "manifest") && p.Ref != "" {
			return true
		}
	}
	if action.Parameters == nil {
		return false
	}

	for _, key := range []string{"manifest", "tool_manifest", "governance_manifest"} {
		v, ok := action.Parameters[key]
		if !ok || v == nil {
			continue
		}
		if manifest, ok := parseManifest(v); ok {
			if manifestHasRequiredFields(manifest) {
				return true
			}
		}
	}
	return false
}

func parseManifest(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case string:
		raw := strings.TrimSpace(t)
		if raw == "" {
			return nil, false
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

func manifestHasRequiredFields(manifest map[string]any) bool {
	if manifest == nil {
		return false
	}
	name, _ := manifest["name"].(string)
	riskClass, _ := manifest["risk_class"].(string)
	caps := manifest["capabilities"]
	if strings.TrimSpace(name) == "" || strings.TrimSpace(riskClass) == "" {
		return false
	}
	switch t := caps.(type) {
	case []any:
		return len(t) > 0
	case []string:
		return len(t) > 0
	default:
		return false
	}
}

func isWithinRoot(target, root string) bool {
	tClean := filepath.Clean(target)
	rClean := filepath.Clean(root)
	if tClean == rClean {
		return true
	}
	return strings.HasPrefix(tClean, rClean+string(filepath.Separator))
}
