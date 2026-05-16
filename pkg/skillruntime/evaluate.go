package skillruntime

import (
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/profile"
	"github.com/velariumai/gorkbot/pkg/replay"
	"github.com/velariumai/gorkbot/pkg/statelock"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func Evaluate(req Request) Result {
	n := req.Normalized()
	result := Result{
		Operation:   n.Operation,
		CandidateID: n.Candidate.ID,
		Status:      StatusUnknown,
		Decision:    evidence.DecisionInconclusive,
		Metadata:    boundMetadata(n.Metadata, maxMetadataEntries),
	}

	if err := n.Operation.Validate(); err != nil {
		return invalidResult(result, n, "invalid operation")
	}
	if err := n.Candidate.Validate(); err != nil {
		return invalidResult(result, n, "invalid candidate")
	}

	cfg := n.Config.Normalized()
	if profile.NormalizeProfile(string(cfg.Profile)) == profile.ProfileCustom && !cfg.CustomProfileConfigured {
		result.Status = StatusConfigRequired
		result.Decision = evidence.DecisionDenyInvalid
		result.Warnings = mergeWarningLists(result.Warnings, "custom profile requires explicit marker")
		result.Assessment = buildConfigRequiredAssessment(n.Operation)
		result.Receipt = buildReceipt(result, n, nil)
		return result.Normalized()
	}

	capability := mapCapability(n.Operation)
	assessment := profile.EvaluateCapability(cfg, capability)
	result.Assessment = assessment
	result.Decision = assessment.Decision
	result.Status = statusFromAssessment(assessment)
	if assessment.PolicyState == evidence.PolicyNotConfigured {
		result.Status = escalateStatus(result.Status, StatusConfigRequired)
	}

	validationRefs := make([]trace.Ref, 0, 8)
	validationRefs = append(validationRefs, n.EvidenceRefs...)
	artifactRefs := make([]trace.Ref, 0, 8)
	artifactRefs = append(artifactRefs, n.Candidate.ArtifactRefs...)

	warnings := make([]string, 0, 8)
	records := make([]evidence.Record, 0, 8)

	records = append(records, evidence.Record{
		Kind:         evidence.KindValidationReport,
		Status:       toEvidenceStatus(result.Status),
		Subject:      "skillruntime_" + string(n.Operation),
		Summary:      "profile capability evaluated",
		PolicyState:  assessment.PolicyState,
		Risk:         assessment.Risk,
		Authority:    assessment.Authority,
		EvidenceRefs: append([]trace.Ref(nil), n.EvidenceRefs...),
		Metadata: map[string]string{
			"capability": string(capability),
			"operation":  string(n.Operation),
		},
	}.Normalized())

	if n.Operation == OperationRetrieve {
		warnings = append(warnings,
			"vector retrieval is candidate-only",
			"vector retrieval is not truth",
		)
		result.Metadata = mergeMetadata(result.Metadata, map[string]string{
			"vector_role": "candidate_only",
		})
	}

	if needsEvidenceGates(n.Operation) {
		if !isPolicyAuthoritative(assessment.PolicyState) {
			if assessment.PolicyState == evidence.PolicyNotConfigured {
				result.Status = escalateStatus(result.Status, StatusConfigRequired)
			} else {
				result.Status = escalateStatus(result.Status, StatusApprovalRequired)
			}
			warnings = append(warnings, "policy absence is not permission for sensitive lifecycle operation")
			records = append(records, evidence.Record{
				Kind:        evidence.KindPolicyAbsence,
				Status:      evidence.StatusWarn,
				Subject:     "policy_absence",
				Summary:     "policy absence requires approval/config",
				PolicyState: assessment.PolicyState,
				Risk:        assessment.Risk,
				Authority:   assessment.Authority,
			}.Normalized())
		}

		if cfg.Evidence.RequireHarnessReport {
			if n.HarnessReport == nil {
				result.Status = escalateStatus(result.Status, StatusApprovalRequired)
				warnings = append(warnings, "missing harness report")
				records = append(records, evidence.Record{
					Kind:    evidence.KindHarnessReport,
					Status:  evidence.StatusWarn,
					Subject: "harness_report",
					Summary: "harness report required",
				}.Normalized())
			} else {
				nh := n.HarnessReport.Normalized()
				validationRefs = append(validationRefs, nh.ValidationRef())
				hs := mapHarnessStatus(nh.Status)
				records = append(records, evidence.Record{
					Kind:         evidence.KindHarnessReport,
					Status:       hs,
					Subject:      nh.ArtifactID,
					Summary:      "harness report consumed",
					EvidenceRefs: []trace.Ref{nh.ValidationRef()},
				}.Normalized())
				if hs == evidence.StatusFail {
					result.Status = escalateStatus(result.Status, StatusDenied)
					warnings = append(warnings, "harness report indicates failure")
				} else if hs == evidence.StatusWarn {
					result.Status = escalateStatus(result.Status, StatusApprovalRequired)
				}
			}
		}

		if cfg.Evidence.RequireReplayNoRegression {
			if n.ReplayResult == nil {
				result.Status = escalateStatus(result.Status, StatusApprovalRequired)
				warnings = append(warnings, "missing replay result")
				records = append(records, evidence.Record{
					Kind:    evidence.KindReplayResult,
					Status:  evidence.StatusWarn,
					Subject: "replay_result",
					Summary: "replay result required",
				}.Normalized())
			} else {
				v := normalizeReplayVerdict(n.ReplayResult.Verdict)
				rs := evidence.StatusPass
				summary := "replay result consumed"
				switch v {
				case replay.VerdictRegression, replay.VerdictFail, replay.VerdictInvalid:
					rs = evidence.StatusFail
					summary = "replay regression detected"
					if n.Operation == OperationPromote {
						result.Status = escalateStatus(result.Status, StatusDenied)
					} else {
						result.Status = escalateStatus(result.Status, StatusApprovalRequired)
					}
					warnings = append(warnings, "replay regression or invalid verdict")
				case replay.VerdictInconclusive:
					rs = evidence.StatusWarn
					summary = "replay inconclusive"
					result.Status = escalateStatus(result.Status, StatusApprovalRequired)
					warnings = append(warnings, "replay verdict inconclusive")
				}
				records = append(records, evidence.Record{
					Kind:    evidence.KindReplayResult,
					Status:  rs,
					Subject: n.ReplayResult.CaseID,
					Summary: summary,
					Metadata: map[string]string{
						"verdict": string(v),
					},
				}.Normalized())
			}
		}

		if cfg.Evidence.RequireStateLockCheck {
			if n.StateLockResult == nil {
				result.Status = escalateStatus(result.Status, StatusApprovalRequired)
				warnings = append(warnings, "missing state lock check")
				records = append(records, evidence.Record{
					Kind:    evidence.KindStateLock,
					Status:  evidence.StatusWarn,
					Subject: "statelock_result",
					Summary: "state lock result required",
				}.Normalized())
			} else {
				status, warning, deny := evaluateStateLock(n.StateLockResult, n.Operation)
				records = append(records, evidence.Record{
					Kind:    evidence.KindStateLock,
					Status:  status,
					Subject: string(n.StateLockResult.Status),
					Summary: "state lock check consumed",
				}.Normalized())
				if warning != "" {
					warnings = append(warnings, warning)
				}
				if deny {
					result.Status = escalateStatus(result.Status, StatusDenied)
				} else if status == evidence.StatusWarn {
					result.Status = escalateStatus(result.Status, StatusApprovalRequired)
				}

				if n.StateLockResult.Paradox != nil {
					np := n.StateLockResult.Paradox.Normalized()
					validationRefs = append(validationRefs, statelock.ParadoxRef(np))
					pStatus := evidence.StatusPass
					if np.Status == statelock.ParadoxConfirmed {
						pStatus = evidence.StatusFail
						result.Status = escalateStatus(result.Status, StatusDenied)
						warnings = append(warnings, "statelock paradox confirmed")
					} else if np.Status == statelock.ParadoxPossible || np.Status == statelock.ParadoxInconclusive {
						pStatus = evidence.StatusWarn
						result.Status = escalateStatus(result.Status, StatusApprovalRequired)
						warnings = append(warnings, "statelock paradox possible")
					}
					records = append(records, evidence.Record{
						Kind:         evidence.KindParadoxReport,
						Status:       pStatus,
						Subject:      np.ID,
						Summary:      np.Summary,
						EvidenceRefs: []trace.Ref{statelock.ParadoxRef(np)},
					}.Normalized())
				}
			}
		}
	}

	if n.Operation == OperationPromote {
		if !hasRollbackEvidence(n) {
			result.Status = escalateStatus(result.Status, StatusApprovalRequired)
			warnings = append(warnings, "promotion requires rollback evidence")
		}
		if !hasDisablePathEvidence(n) {
			result.Status = escalateStatus(result.Status, StatusApprovalRequired)
			warnings = append(warnings, "promotion requires disable path evidence")
		}

		auto := profile.NormalizeAutomationMode(string(cfg.Automation.AutoPromotionMode))
		if auto != profile.AutomationAllowConfigured || !cfg.AllowsConfigured(profile.CapabilitySkillPromote) {
			if auto == profile.AutomationAllowConfigured && !cfg.AllowsConfigured(profile.CapabilitySkillPromote) {
				result.Status = escalateStatus(result.Status, StatusConfigRequired)
			} else {
				result.Status = escalateStatus(result.Status, StatusApprovalRequired)
			}
		}
		if result.Status == StatusAllowed {
			result.Status = StatusStaged
		}
	}

	if n.Operation == OperationStage && result.Status == StatusAllowed {
		result.Status = StatusStaged
	}
	if n.Operation == OperationDisable {
		result.Status = StatusDisabled
		if result.Decision == evidence.DecisionInconclusive {
			result.Decision = evidence.DecisionAuditOnly
		}
	}

	result.Warnings = mergeWarningLists(result.Warnings, warnings...)
	result.ValidationRefs = mergeRefs(validationRefs, n.Candidate.EvidenceRefs)
	result.ArtifactRefs = mergeRefs(artifactRefs, n.Candidate.EvidenceRefs)
	result.Receipt = buildReceipt(result, n, records)
	return result.Normalized()
}

func buildReceipt(result Result, req Request, records []evidence.Record) evidence.Receipt {
	receipt := evidence.Receipt{
		Records:      append([]evidence.Record(nil), records...),
		Assessment:   result.Assessment,
		Status:       toEvidenceStatus(result.Status),
		Summary:      boundStringSingleLine(fmt.Sprintf("%s %s", result.Operation, result.Status), 120),
		EvidenceRefs: mergeRefs(result.ValidationRefs, req.EvidenceRefs),
		Metadata: mergeMetadata(req.Metadata, map[string]string{
			"candidate_id": result.CandidateID,
			"status":       string(result.Status),
		}),
	}
	return receipt.Normalized()
}

func invalidResult(seed Result, req Request, message string) Result {
	seed.Status = StatusInvalid
	seed.Decision = evidence.DecisionDenyInvalid
	seed.Assessment = evidence.Assessment{
		PolicyState: evidence.PolicyInvalid,
		Risk:        evidence.RiskUnknown,
		Operation:   string(req.Operation),
		Authority:   evidence.AuthorityUnknown,
		Status:      evidence.StatusInvalid,
		Decision:    evidence.DecisionDenyInvalid,
		ReasonCode:  "invalid_input",
		Metadata: map[string]string{
			"detail": message,
		},
	}
	seed.Warnings = mergeWarningLists(seed.Warnings, message)
	seed.Receipt = buildReceipt(seed, req, nil)
	return seed.Normalized()
}

func buildConfigRequiredAssessment(op Operation) evidence.Assessment {
	return evidence.Assessment{
		PolicyState: evidence.PolicyNotConfigured,
		Risk:        evidence.RiskSensitive,
		Operation:   string(op),
		Authority:   evidence.AuthorityNone,
		Status:      evidence.StatusWarn,
		Decision:    evidence.DecisionRequireApproval,
		ReasonCode:  "custom_profile_not_marked",
	}.Normalized()
}

func needsEvidenceGates(op Operation) bool {
	switch op {
	case OperationValidate, OperationStage, OperationPromote:
		return true
	default:
		return false
	}
}

func mapCapability(op Operation) profile.Capability {
	switch op {
	case OperationRetrieve:
		return profile.CapabilityVectorRetrieve
	case OperationPropose, OperationValidate, OperationStage, OperationDisable:
		return profile.CapabilitySkillStage
	case OperationPromote:
		return profile.CapabilitySkillPromote
	default:
		return profile.CapabilityUnknown
	}
}

func statusFromAssessment(a evidence.Assessment) Status {
	an := evidence.Evaluate(a)
	switch an.Decision {
	case evidence.DecisionDenyInvalid:
		return StatusInvalid
	case evidence.DecisionDenySensitive:
		return StatusDenied
	case evidence.DecisionRequireApproval:
		return StatusApprovalRequired
	case evidence.DecisionAuditOnly, evidence.DecisionAllowLowRisk:
		return StatusAllowed
	default:
		if an.Status == evidence.StatusFail {
			return StatusDenied
		}
		if an.Status == evidence.StatusWarn {
			return StatusApprovalRequired
		}
		if an.Status == evidence.StatusPass {
			return StatusAllowed
		}
		return StatusUnknown
	}
}

func toEvidenceStatus(status Status) evidence.Status {
	switch NormalizeStatus(string(status)) {
	case StatusAllowed, StatusStaged, StatusDisabled:
		return evidence.StatusPass
	case StatusApprovalRequired, StatusConfigRequired:
		return evidence.StatusWarn
	case StatusDenied, StatusInvalid:
		return evidence.StatusFail
	default:
		return evidence.StatusInconclusive
	}
}

func isPolicyAuthoritative(state evidence.PolicyState) bool {
	n := evidence.NormalizePolicyState(string(state))
	return n == evidence.PolicyMatched || n == evidence.PolicyEnforced || n == evidence.PolicyAuditOnly
}

func hasRollbackEvidence(req Request) bool {
	return hasAnyMetadata(req.Metadata, req.Candidate.Metadata,
		"rollback", "rollback_path", "rollback_plan", "rollback_ref")
}

func hasDisablePathEvidence(req Request) bool {
	return hasAnyMetadata(req.Metadata, req.Candidate.Metadata,
		"disable", "disable_path", "disable_plan", "disable_ref")
}

func hasAnyMetadata(primary map[string]string, secondary map[string]string, keys ...string) bool {
	if hasMetadataKeys(primary, keys...) {
		return true
	}
	return hasMetadataKeys(secondary, keys...)
}

func hasMetadataKeys(meta map[string]string, keys ...string) bool {
	if len(meta) == 0 {
		return false
	}
	for i := range keys {
		k := strings.ToLower(strings.TrimSpace(keys[i]))
		for mk, mv := range meta {
			nmk := strings.ToLower(strings.TrimSpace(mk))
			if nmk == k || strings.Contains(nmk, k) {
				if strings.TrimSpace(mv) != "" && strings.TrimSpace(mv) != "false" && strings.TrimSpace(mv) != "0" {
					return true
				}
			}
		}
	}
	return false
}

func mapHarnessStatus(status harness.Status) evidence.Status {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(harness.StatusPass):
		return evidence.StatusPass
	case string(harness.StatusFail):
		return evidence.StatusFail
	case string(harness.StatusWarn), string(harness.StatusInconclusive), string(harness.StatusUnsupported):
		return evidence.StatusWarn
	default:
		return evidence.StatusInconclusive
	}
}

func normalizeReplayVerdict(v replay.Verdict) replay.Verdict {
	s := strings.ToLower(strings.TrimSpace(string(v)))
	switch replay.Verdict(s) {
	case replay.VerdictPass, replay.VerdictFail, replay.VerdictRegression,
		replay.VerdictImprovement, replay.VerdictInconclusive, replay.VerdictInvalid:
		return replay.Verdict(s)
	default:
		return replay.VerdictInconclusive
	}
}

func evaluateStateLock(in *statelock.CheckResult, op Operation) (evidence.Status, string, bool) {
	if in == nil {
		return evidence.StatusWarn, "missing state lock check", false
	}
	s := strings.ToLower(strings.TrimSpace(string(in.Status)))
	switch s {
	case string(statelock.CheckStatusAllowed):
		return evidence.StatusPass, "", false
	case string(statelock.CheckStatusConflict):
		if op == OperationPromote {
			return evidence.StatusFail, "state lock conflict", true
		}
		return evidence.StatusWarn, "state lock conflict", false
	default:
		if op == OperationPromote {
			return evidence.StatusFail, "invalid state lock result", true
		}
		return evidence.StatusWarn, "invalid state lock result", false
	}
}

func mergeMetadata(base map[string]string, other map[string]string) map[string]string {
	out := cloneMetadata(base)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range other {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		out[k] = v
	}
	return boundMetadata(out, maxMetadataEntries)
}

func escalateStatus(current, incoming Status) Status {
	rank := map[Status]int{
		StatusAllowed:          1,
		StatusStaged:           2,
		StatusDisabled:         2,
		StatusApprovalRequired: 3,
		StatusConfigRequired:   4,
		StatusDenied:           5,
		StatusInvalid:          6,
	}
	c := NormalizeStatus(string(current))
	n := NormalizeStatus(string(incoming))
	if rank[n] > rank[c] {
		return n
	}
	if c == StatusUnknown {
		return n
	}
	return c
}
