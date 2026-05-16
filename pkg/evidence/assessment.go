package evidence

import (
	"fmt"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type Decision string

const (
	DecisionAllowLowRisk    Decision = "allow_low_risk"
	DecisionAuditOnly       Decision = "audit_only"
	DecisionRequireApproval Decision = "require_approval"
	DecisionDenySensitive   Decision = "deny_sensitive"
	DecisionDenyInvalid     Decision = "deny_invalid"
	DecisionInconclusive    Decision = "inconclusive"
)

const (
	ReasonLowRiskExplicitAbsent     = "low_risk_explicit_absent_policy"
	ReasonLowRiskNoExplicit         = "low_risk_not_explicit"
	ReasonLowRiskPolicyBound        = "low_risk_policy_bound"
	ReasonMediumPolicyAbsent        = "medium_policy_absent"
	ReasonMediumPolicyAuditOnly     = "medium_policy_audit_only"
	ReasonMediumPolicyBound         = "medium_policy_bound"
	ReasonPolicyAuditOnly           = "policy_audit_only"
	ReasonPolicyAbsentSensitive     = "policy_absent_sensitive"
	ReasonPolicyInvalidSensitive    = "policy_invalid_sensitive"
	ReasonUnknownRisk               = "unknown_risk"
	ReasonSensitiveMissingAuthority = "sensitive_missing_authority"
	ReasonSensitivePolicyBound      = "sensitive_policy_bound"
	ReasonHardInvariant             = "hard_invariant"
)

// Assessment records deterministic policy/risk classification output.
type Assessment struct {
	ID              string             `json:"id"`
	PolicyState     PolicyState        `json:"policy_state"`
	Risk            Risk               `json:"risk"`
	Operation       string             `json:"operation,omitempty"`
	SensitiveClass  SensitiveOperation `json:"sensitive_class,omitempty"`
	ExplicitLowRisk bool               `json:"explicit_low_risk"`
	Authority       Authority          `json:"authority"`
	Status          Status             `json:"status"`
	Decision        Decision           `json:"decision"`
	ReasonCode      string             `json:"reason_code,omitempty"`
	EvidenceRefs    []trace.Ref        `json:"evidence_refs,omitempty"`
	Metadata        map[string]string  `json:"metadata,omitempty"`
}

func NormalizeDecision(raw string) Decision {
	d := Decision(boundString(raw, maxReasonCodeLen))
	switch d {
	case DecisionAllowLowRisk, DecisionAuditOnly, DecisionRequireApproval,
		DecisionDenySensitive, DecisionDenyInvalid, DecisionInconclusive:
		return d
	default:
		return DecisionInconclusive
	}
}

func (a Assessment) Normalized() Assessment {
	out := a
	out.PolicyState = NormalizePolicyState(string(out.PolicyState))
	out.Risk = NormalizeRisk(string(out.Risk))
	out.Operation = boundString(out.Operation, maxOperationLen)
	out.SensitiveClass = NormalizeSensitiveOperation(string(out.SensitiveClass))
	if out.SensitiveClass == SensitiveUnknown && IsSensitiveOperation(out.Operation) {
		out.SensitiveClass = NormalizeSensitiveOperation(out.Operation)
	}
	out.Authority = NormalizeAuthority(string(out.Authority))
	out.Status = NormalizeStatus(string(out.Status))
	out.Decision = NormalizeDecision(string(out.Decision))
	out.ReasonCode = boundString(out.ReasonCode, maxReasonCodeLen)
	out.EvidenceRefs = boundRefs(out.EvidenceRefs, maxRefCount)
	out.Metadata = boundMetadata(out.Metadata, maxMetadataCount)
	if out.ID == "" {
		out.ID = "assessment_" + trace.StableHash(
			string(out.PolicyState),
			string(out.Risk),
			out.Operation,
			string(out.SensitiveClass),
			boolString(out.ExplicitLowRisk),
			string(out.Authority),
			string(out.Decision),
			out.ReasonCode,
			stableRefsHash(out.EvidenceRefs),
			stableMetadataHash(out.Metadata),
		)
	}
	return out
}

func (a Assessment) Validate() error {
	n := a.Normalized()
	if n.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidAssessment)
	}
	if n.Risk == RiskSensitive && n.Decision == DecisionAllowLowRisk {
		return fmt.Errorf("%w: sensitive risk cannot allow_low_risk", ErrInvalidAssessment)
	}
	if n.Risk == RiskMedium && n.Decision == DecisionAllowLowRisk {
		return fmt.Errorf("%w: medium risk cannot allow_low_risk", ErrInvalidAssessment)
	}
	if n.Risk == RiskUnknown && isAllowDecision(n.Decision) {
		return fmt.Errorf("%w: unknown risk cannot use allow-like decisions", ErrInvalidAssessment)
	}
	if n.PolicyState == PolicyInvalid && isAllowDecision(n.Decision) {
		return fmt.Errorf("%w: invalid policy cannot use allow-like decisions", ErrInvalidAssessment)
	}
	if n.Risk == RiskSensitive && isAllowDecision(n.Decision) &&
		(n.Authority == AuthorityNone || n.Authority == AuthorityUnknown) {
		return fmt.Errorf("%w: sensitive allow-like decision requires authoritative allow", ErrInvalidAssessment)
	}
	if n.Decision == DecisionDenyInvalid && n.Status == StatusPass {
		return fmt.Errorf("%w: deny_invalid cannot be pass", ErrInvalidAssessment)
	}
	return nil
}

func Evaluate(a Assessment) Assessment {
	n := a.Normalized()

	if n.Authority == AuthorityHardInvariant {
		n.Decision = DecisionDenySensitive
		n.Status = StatusFail
		n.ReasonCode = ReasonHardInvariant
		return n
	}

	if n.Risk == RiskUnknown {
		n.Decision = DecisionDenyInvalid
		n.Status = StatusInvalid
		n.ReasonCode = ReasonUnknownRisk
		return n
	}

	if n.Risk == RiskLow {
		if IsPolicyAbsent(n.PolicyState) {
			if n.ExplicitLowRisk {
				n.Decision = DecisionAllowLowRisk
				n.Status = StatusPass
				n.ReasonCode = ReasonLowRiskExplicitAbsent
			} else {
				n.Decision = DecisionAuditOnly
				n.Status = StatusWarn
				n.ReasonCode = ReasonLowRiskNoExplicit
			}
			return n
		}
		if n.PolicyState == PolicyAuditOnly {
			n.Decision = DecisionAuditOnly
			n.Status = StatusWarn
			n.ReasonCode = ReasonPolicyAuditOnly
			return n
		}
		n.Decision = DecisionAllowLowRisk
		n.Status = StatusPass
		n.ReasonCode = ReasonLowRiskPolicyBound
		return n
	}

	if n.Risk == RiskMedium {
		if IsPolicyAbsent(n.PolicyState) {
			n.Decision = DecisionAuditOnly
			n.Status = StatusWarn
			n.ReasonCode = ReasonMediumPolicyAbsent
			return n
		}
		if n.PolicyState == PolicyAuditOnly {
			n.Decision = DecisionAuditOnly
			n.Status = StatusWarn
			n.ReasonCode = ReasonMediumPolicyAuditOnly
			return n
		}
		n.Decision = DecisionAuditOnly
		n.Status = StatusWarn
		n.ReasonCode = ReasonMediumPolicyBound
		return n
	}

	if n.Risk == RiskSensitive {
		if n.PolicyState == PolicyInvalid {
			n.Decision = DecisionDenySensitive
			n.Status = StatusFail
			n.ReasonCode = ReasonPolicyInvalidSensitive
			return n
		}
		if IsPolicyAbsent(n.PolicyState) {
			n.Decision = DecisionRequireApproval
			n.Status = StatusWarn
			n.ReasonCode = ReasonPolicyAbsentSensitive
			return n
		}
		if n.PolicyState == PolicyAuditOnly {
			n.Decision = DecisionRequireApproval
			n.Status = StatusWarn
			n.ReasonCode = ReasonPolicyAuditOnly
			return n
		}
		if AllowsSensitiveOperation(n.PolicyState) && AllowsSensitiveByAuthority(n.Authority) {
			n.Decision = DecisionAuditOnly
			n.Status = StatusWarn
			n.ReasonCode = ReasonSensitivePolicyBound
			return n
		}
		n.Decision = DecisionRequireApproval
		n.Status = StatusWarn
		n.ReasonCode = ReasonSensitiveMissingAuthority
		return n
	}

	if n.PolicyState == PolicyAuditOnly {
		n.Decision = DecisionAuditOnly
		n.Status = StatusWarn
		n.ReasonCode = ReasonPolicyAuditOnly
		return n
	}
	if IsPolicyAbsent(n.PolicyState) {
		n.Decision = DecisionRequireApproval
		n.Status = StatusWarn
		n.ReasonCode = ReasonLowRiskNoExplicit
		return n
	}
	if n.Status == StatusUnknown || n.Status == StatusInvalid {
		n.Status = StatusInconclusive
	}
	if n.Decision == DecisionInconclusive {
		n.Decision = DecisionAuditOnly
	}
	if n.ReasonCode == "" {
		n.ReasonCode = ReasonPolicyAuditOnly
	}
	return n
}

func isAllowDecision(decision Decision) bool {
	return decision == DecisionAllowLowRisk || decision == DecisionAuditOnly
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
