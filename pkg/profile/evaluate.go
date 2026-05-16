package profile

import (
	"errors"

	"github.com/velariumai/gorkbot/pkg/evidence"
)

type capabilityRule struct {
	surface      AuthoritySurface
	risk         evidence.Risk
	sensitive    evidence.SensitiveOperation
	explicitLow  bool
	needsRelease bool
	needsPromo   bool
	needsNetwork bool
	needsPrivate bool
	needsShell   bool
	needsSelfmod bool
	irreversible bool
}

var capabilityRules = map[Capability]capabilityRule{
	CapabilityToolExecute: {
		surface:     SurfaceToolAuthority,
		risk:        evidence.RiskMedium,
		explicitLow: false,
	},
	CapabilityFileRead: {
		surface:     SurfaceFileAuthority,
		risk:        evidence.RiskLow,
		explicitLow: true,
	},
	CapabilityFileMutate: {
		surface:      SurfaceFileAuthority,
		risk:         evidence.RiskSensitive,
		sensitive:    evidence.SensitiveFileMutation,
		irreversible: true,
	},
	CapabilityNetworkEgress: {
		surface:      SurfaceNetworkAuthority,
		risk:         evidence.RiskSensitive,
		sensitive:    evidence.SensitiveNetworkEgress,
		needsNetwork: true,
	},
	CapabilityPrivateNetworkEgress: {
		surface:      SurfacePrivateNetworkAuthority,
		risk:         evidence.RiskSensitive,
		sensitive:    evidence.SensitivePrivateNetwork,
		needsNetwork: true,
		needsPrivate: true,
	},
	CapabilityShellExecute: {
		surface:    SurfaceShellAuthority,
		risk:       evidence.RiskSensitive,
		sensitive:  evidence.SensitiveShellExecution,
		needsShell: true,
	},
	CapabilitySelfmodValidate: {
		surface:      SurfaceSelfmodAuthority,
		risk:         evidence.RiskMedium,
		explicitLow:  false,
		needsSelfmod: true,
	},
	CapabilitySelfmodPromote: {
		surface:      SurfacePromotionAuthority,
		risk:         evidence.RiskSensitive,
		sensitive:    evidence.SensitiveSelfmodPromotion,
		needsSelfmod: true,
		needsPromo:   true,
		irreversible: true,
	},
	CapabilitySkillStage: {
		surface:      SurfaceSelfmodAuthority,
		risk:         evidence.RiskMedium,
		explicitLow:  false,
		needsSelfmod: true,
	},
	CapabilitySkillPromote: {
		surface:      SurfacePromotionAuthority,
		risk:         evidence.RiskSensitive,
		sensitive:    evidence.SensitiveSelfmodPromotion,
		needsSelfmod: true,
		needsPromo:   true,
		irreversible: true,
	},
	CapabilityPlannerMutateSession: {
		surface:     SurfacePlannerMutationAuthority,
		risk:        evidence.RiskMedium,
		explicitLow: false,
	},
	CapabilityPlannerMutatePersistent: {
		surface:      SurfacePlannerMutationAuthority,
		risk:         evidence.RiskSensitive,
		sensitive:    evidence.SensitiveFileMutation,
		irreversible: true,
	},
	CapabilityReleasePublish: {
		surface:      SurfaceReleaseAuthority,
		risk:         evidence.RiskSensitive,
		sensitive:    evidence.SensitiveReleasePublish,
		needsRelease: true,
		irreversible: true,
	},
	CapabilityVectorRetrieve: {
		surface:     SurfaceVectorRetrievalAuthority,
		risk:        evidence.RiskLow,
		explicitLow: true,
	},
}

func EvaluateCapability(config Config, capability Capability) evidence.Assessment {
	cfg := config.Normalized()
	capValue := normalizeCapabilityValue(capability)

	if capValue == CapabilityVectorAssertTruth {
		return evidence.Assessment{
			PolicyState: evidence.PolicyEnforced,
			Risk:        evidence.RiskSensitive,
			Operation:   string(capValue),
			Authority:   evidence.AuthorityHardInvariant,
			Status:      evidence.StatusFail,
			Decision:    evidence.DecisionDenySensitive,
			ReasonCode:  "vector_assert_truth_forbidden",
			Metadata: map[string]string{
				"profile":    string(cfg.Profile),
				"capability": string(capValue),
				"invariant":  "no_vector_truth_authority",
			},
		}.Normalized()
	}

	rule, ok := capabilityRules[capValue]
	if !ok {
		return evidence.Assessment{
			PolicyState: evidence.PolicyInvalid,
			Risk:        evidence.RiskUnknown,
			Operation:   string(capValue),
			Authority:   evidence.AuthorityUnknown,
			Status:      evidence.StatusInvalid,
			Decision:    evidence.DecisionDenyInvalid,
			ReasonCode:  "unknown_capability",
			Metadata: map[string]string{
				"profile":    string(cfg.Profile),
				"capability": string(capability),
			},
		}.Normalized()
	}

	surfaceMode := normalizeAuthorityValue(cfg.Authority.SurfaceMode(rule.surface))
	explicitConfigured := surfaceMode == AuthorityAllowConfigured && cfg.AllowsConfigured(capValue)
	explicitAllowed := authorizesDirectly(surfaceMode) || explicitConfigured
	requiresApproval := requiresPrompt(surfaceMode) || approvalRequired(cfg, rule, explicitAllowed)

	policyState := evidence.PolicyNoMatch
	authority := evidence.AuthorityNone
	switch surfaceMode {
	case AuthorityAllow:
		policyState = evidence.PolicyMatched
		authority = evidence.AuthorityPolicyMatch
	case AuthorityAllowConfigured:
		if explicitConfigured {
			policyState = evidence.PolicyMatched
			authority = evidence.AuthorityPolicyMatch
		} else {
			policyState = evidence.PolicyNotConfigured
			authority = evidence.AuthorityNone
		}
	case AuthorityAuditOnly:
		policyState = evidence.PolicyAuditOnly
		authority = evidence.AuthorityAuditOnly
	case AuthorityPromptOnce, AuthorityPromptSession, AuthorityPromptAlways:
		policyState = evidence.PolicyNoMatch
		authority = evidence.AuthorityHumanApproval
	case AuthorityUnknown:
		policyState = evidence.PolicyInvalid
		authority = evidence.AuthorityUnknown
	case AuthorityDisabled, AuthorityDeny:
		policyState = evidence.PolicyNoMatch
		authority = evidence.AuthorityNone
	default:
		policyState = evidence.PolicyInvalid
		authority = evidence.AuthorityUnknown
	}

	assessment := evidence.Assessment{
		PolicyState:     policyState,
		Risk:            rule.risk,
		Operation:       string(capValue),
		SensitiveClass:  rule.sensitive,
		ExplicitLowRisk: rule.explicitLow,
		Authority:       authority,
		Status:          evidence.StatusUnknown,
		Decision:        evidence.DecisionInconclusive,
		ReasonCode:      "",
		Metadata: map[string]string{
			"profile":                string(cfg.Profile),
			"capability":             string(capValue),
			"surface":                string(rule.surface),
			"surface_mode":           string(surfaceMode),
			"allow_configured_match": boolString(explicitConfigured),
			"vector_candidate_only":  boolString(cfg.Evidence.VectorCandidateOnly),
		},
	}

	assessed := evidence.Evaluate(assessment)

	if capValue == CapabilityReleasePublish && !explicitAllowed {
		assessed.PolicyState = evidence.PolicyNotConfigured
		assessed.Authority = evidence.AuthorityNone
		assessed.Status = evidence.StatusFail
		assessed.Decision = evidence.DecisionDenySensitive
		assessed.ReasonCode = "release_authority_explicit_required"
	}

	if requiresApproval {
		if assessed.Risk == evidence.RiskSensitive {
			assessed.Status = evidence.StatusWarn
			assessed.Decision = evidence.DecisionRequireApproval
			if assessed.ReasonCode == "" || assessed.ReasonCode == evidence.ReasonSensitivePolicyBound {
				assessed.ReasonCode = "sensitive_requires_approval"
			}
		} else if assessed.Risk == evidence.RiskMedium {
			assessed.Status = evidence.StatusWarn
			assessed.Decision = evidence.DecisionRequireApproval
			assessed.ReasonCode = "medium_requires_approval"
		}
	}

	if (rule.needsPromo || rule.needsRelease || rule.irreversible) && !cfg.Evidence.RequireEvidenceReceipt {
		assessed.Status = evidence.StatusWarn
		assessed.Decision = evidence.DecisionRequireApproval
		assessed.ReasonCode = "evidence_receipt_required"
	}
	if (rule.needsPromo || rule.needsRelease || rule.irreversible) && !cfg.Evidence.RequireRollbackPlan {
		assessed.Status = evidence.StatusWarn
		assessed.Decision = evidence.DecisionRequireApproval
		assessed.ReasonCode = "rollback_plan_required"
	}
	if (rule.needsPromo || rule.needsRelease || rule.irreversible) && !cfg.Evidence.RequireDisablePath {
		assessed.Status = evidence.StatusWarn
		assessed.Decision = evidence.DecisionRequireApproval
		assessed.ReasonCode = "disable_path_required"
	}
	if capValue == CapabilityVectorRetrieve {
		assessed.Metadata["vector_role"] = "candidate_only"
	}

	return assessed.Normalized()
}

func EvaluateCapabilityStrict(config Config, capability Capability) (evidence.Assessment, error) {
	if normalizeCapabilityValue(capability) == CapabilityUnknown {
		return evidence.Assessment{}, ErrUnknownCapability
	}
	assessment := EvaluateCapability(config, capability)
	if err := assessment.Validate(); err != nil {
		return evidence.Assessment{}, err
	}
	if assessment.Decision == evidence.DecisionDenyInvalid {
		return assessment, errors.New("invalid capability assessment")
	}
	return assessment, nil
}

func approvalRequired(cfg Config, rule capabilityRule, explicitAllowed bool) bool {
	a := cfg.Approval
	if rule.risk == evidence.RiskSensitive && a.RequireHumanApprovalForSensitive {
		return true
	}
	if !explicitAllowed && a.RequireApprovalForPolicyAbsence {
		return true
	}
	if rule.needsRelease && a.RequireApprovalForRelease {
		return true
	}
	if rule.needsNetwork && a.RequireApprovalForNetwork {
		return true
	}
	if rule.needsPrivate && a.RequireApprovalForPrivateNetwork {
		return true
	}
	if rule.needsShell && a.RequireApprovalForShell {
		return true
	}
	if rule.needsSelfmod && a.RequireApprovalForSelfmod {
		return true
	}
	if rule.needsPromo && a.RequireApprovalForPromotion {
		return true
	}
	if rule.irreversible && a.RequireApprovalForIrreversibleMutat {
		return true
	}
	return false
}
