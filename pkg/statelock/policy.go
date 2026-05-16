package statelock

import (
	"strings"

	"github.com/velariumai/gorkbot/pkg/evidence"
)

type PolicyState = evidence.PolicyState
type Risk = evidence.Risk
type SensitiveOperation = evidence.SensitiveOperation

const (
	PolicyOff           PolicyState = evidence.PolicyOff
	PolicyNotConfigured PolicyState = evidence.PolicyNotConfigured
	PolicyUnavailable   PolicyState = evidence.PolicyUnavailable
	PolicyNoMatch       PolicyState = evidence.PolicyNoMatch
	PolicyInvalid       PolicyState = evidence.PolicyInvalid
	PolicyMatched       PolicyState = evidence.PolicyMatched
	PolicyEnforced      PolicyState = evidence.PolicyEnforced
	PolicyAuditOnly     PolicyState = evidence.PolicyAuditOnly
)

const (
	RiskLow       Risk = evidence.RiskLow
	RiskMedium    Risk = evidence.RiskMedium
	RiskSensitive Risk = evidence.RiskSensitive
	RiskUnknown   Risk = evidence.RiskUnknown
)

const (
	SensitiveCredentialAccess SensitiveOperation = evidence.SensitiveCredentialAccess
	SensitiveNetworkEgress    SensitiveOperation = evidence.SensitiveNetworkEgress
	SensitivePrivateNetwork   SensitiveOperation = evidence.SensitivePrivateNetwork
	SensitiveFileMutation     SensitiveOperation = evidence.SensitiveFileMutation
	SensitiveSelfmodPromotion SensitiveOperation = evidence.SensitiveSelfmodPromotion
	SensitiveToolInstallation SensitiveOperation = evidence.SensitiveToolInstallation
	SensitiveShellExecution   SensitiveOperation = evidence.SensitiveShellExecution
	SensitiveReleasePublish   SensitiveOperation = evidence.SensitiveReleasePublish
	SensitiveHostBridge       SensitiveOperation = evidence.SensitiveHostBridge
	SensitiveWorkspaceEscape  SensitiveOperation = evidence.SensitiveWorkspaceEscape
)

func normalizePolicyState(raw string) PolicyState {
	return evidence.NormalizePolicyState(raw)
}

func normalizeRisk(raw string) Risk {
	return evidence.NormalizeRisk(raw)
}

func IsPolicyAbsent(state PolicyState) bool {
	return evidence.IsPolicyAbsent(state)
}

func IsPolicyAuthoritative(state PolicyState) bool {
	return evidence.IsPolicyAuthoritative(state)
}

func AllowsSensitiveOperation(state PolicyState) bool {
	return evidence.AllowsSensitiveOperation(state)
}

func RequiresApprovalForSensitive(state PolicyState) bool {
	return evidence.RequiresApprovalForSensitive(state)
}

func ClassifyOperationRisk(operation string) Risk {
	op := strings.ToLower(strings.TrimSpace(operation))
	if op == "" {
		return RiskUnknown
	}
	if isSensitiveOperation(op) {
		return RiskSensitive
	}

	mediumOps := map[string]struct{}{
		"provider_selection": {}, "decision_change": {}, "policy_update": {}, "validation_override": {},
	}
	if _, ok := mediumOps[op]; ok {
		return RiskMedium
	}

	lowOps := map[string]struct{}{
		"read_metadata": {}, "read_trace": {}, "list_locks": {}, "replay_compare": {}, "harness_report_read": {},
	}
	if _, ok := lowOps[op]; ok {
		return RiskLow
	}

	return evidence.ClassifyOperationRisk(op)
}

func isSensitiveOperation(operation string) bool {
	return evidence.IsSensitiveOperation(operation)
}
