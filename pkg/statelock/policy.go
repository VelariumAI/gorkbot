package statelock

import "strings"

type PolicyState string

type Risk string

type SensitiveOperation string

const (
	PolicyOff           PolicyState = "policy_off"
	PolicyNotConfigured PolicyState = "policy_not_configured"
	PolicyUnavailable   PolicyState = "policy_unavailable"
	PolicyNoMatch       PolicyState = "policy_no_match"
	PolicyInvalid       PolicyState = "policy_invalid"
	PolicyMatched       PolicyState = "policy_matched"
	PolicyEnforced      PolicyState = "policy_enforced"
	PolicyAuditOnly     PolicyState = "policy_audit_only"
)

const (
	RiskLow       Risk = "low"
	RiskMedium    Risk = "medium"
	RiskSensitive Risk = "sensitive"
	RiskUnknown   Risk = "unknown"
)

const (
	SensitiveCredentialAccess SensitiveOperation = "credential_access"
	SensitiveNetworkEgress    SensitiveOperation = "network_egress"
	SensitivePrivateNetwork   SensitiveOperation = "private_network_access"
	SensitiveFileMutation     SensitiveOperation = "file_mutation"
	SensitiveSelfmodPromotion SensitiveOperation = "selfmod_promotion"
	SensitiveToolInstallation SensitiveOperation = "tool_installation"
	SensitiveShellExecution   SensitiveOperation = "shell_execution"
	SensitiveReleasePublish   SensitiveOperation = "release_publish"
	SensitiveHostBridge       SensitiveOperation = "host_bridge"
	SensitiveWorkspaceEscape  SensitiveOperation = "workspace_escape"
)

func normalizePolicyState(raw string) PolicyState {
	s := PolicyState(strings.ToLower(strings.TrimSpace(raw)))
	switch s {
	case PolicyOff, PolicyNotConfigured, PolicyUnavailable, PolicyNoMatch,
		PolicyInvalid, PolicyMatched, PolicyEnforced, PolicyAuditOnly:
		return s
	default:
		return PolicyInvalid
	}
}

func normalizeRisk(raw string) Risk {
	r := Risk(strings.ToLower(strings.TrimSpace(raw)))
	switch r {
	case RiskLow, RiskMedium, RiskSensitive:
		return r
	default:
		return RiskUnknown
	}
}

func IsPolicyAbsent(state PolicyState) bool {
	s := normalizePolicyState(string(state))
	switch s {
	case PolicyOff, PolicyNotConfigured, PolicyUnavailable, PolicyNoMatch, PolicyInvalid:
		return true
	default:
		return false
	}
}

func IsPolicyAuthoritative(state PolicyState) bool {
	s := normalizePolicyState(string(state))
	return s == PolicyMatched || s == PolicyEnforced || s == PolicyAuditOnly
}

func AllowsSensitiveOperation(state PolicyState) bool {
	s := normalizePolicyState(string(state))
	return s == PolicyMatched || s == PolicyEnforced
}

func RequiresApprovalForSensitive(state PolicyState) bool {
	s := normalizePolicyState(string(state))
	if s == PolicyAuditOnly {
		return true
	}
	return IsPolicyAbsent(s)
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

	return RiskUnknown
}

func isSensitiveOperation(operation string) bool {
	sensitive := map[string]struct{}{
		string(SensitiveCredentialAccess): {},
		string(SensitiveNetworkEgress):    {},
		string(SensitivePrivateNetwork):   {},
		string(SensitiveFileMutation):     {},
		string(SensitiveSelfmodPromotion): {},
		string(SensitiveToolInstallation): {},
		string(SensitiveShellExecution):   {},
		string(SensitiveReleasePublish):   {},
		string(SensitiveHostBridge):       {},
		string(SensitiveWorkspaceEscape):  {},
	}
	_, ok := sensitive[operation]
	return ok
}
