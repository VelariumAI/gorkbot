package evidence

import "strings"

type PolicyState string

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

func NormalizePolicyState(raw string) PolicyState {
	s := PolicyState(strings.ToLower(strings.TrimSpace(raw)))
	switch s {
	case PolicyOff, PolicyNotConfigured, PolicyUnavailable, PolicyNoMatch,
		PolicyInvalid, PolicyMatched, PolicyEnforced, PolicyAuditOnly:
		return s
	default:
		return PolicyInvalid
	}
}

func IsPolicyAbsent(state PolicyState) bool {
	s := NormalizePolicyState(string(state))
	switch s {
	case PolicyOff, PolicyNotConfigured, PolicyUnavailable, PolicyNoMatch, PolicyInvalid:
		return true
	default:
		return false
	}
}

func IsPolicyAuthoritative(state PolicyState) bool {
	s := NormalizePolicyState(string(state))
	return s == PolicyMatched || s == PolicyEnforced || s == PolicyAuditOnly
}

func AllowsSensitiveOperation(state PolicyState) bool {
	s := NormalizePolicyState(string(state))
	return s == PolicyMatched || s == PolicyEnforced
}

func RequiresApprovalForSensitive(state PolicyState) bool {
	s := NormalizePolicyState(string(state))
	if s == PolicyAuditOnly {
		return true
	}
	return IsPolicyAbsent(s)
}
