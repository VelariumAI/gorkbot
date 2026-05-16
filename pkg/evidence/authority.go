package evidence

import "strings"

type Authority string

const (
	AuthorityNone           Authority = "none"
	AuthorityAuditOnly      Authority = "audit_only"
	AuthorityPolicyMatch    Authority = "policy_match"
	AuthorityPolicyEnforced Authority = "policy_enforced"
	AuthorityHumanApproval  Authority = "human_approval"
	AuthorityHardInvariant  Authority = "hard_invariant"
	AuthorityOverride       Authority = "override"
	AuthorityUnknown        Authority = "unknown"
)

func NormalizeAuthority(raw string) Authority {
	a := Authority(strings.ToLower(strings.TrimSpace(raw)))
	switch a {
	case AuthorityNone, AuthorityAuditOnly, AuthorityPolicyMatch,
		AuthorityPolicyEnforced, AuthorityHumanApproval, AuthorityHardInvariant,
		AuthorityOverride, AuthorityUnknown:
		return a
	case "":
		return AuthorityNone
	default:
		return AuthorityUnknown
	}
}

func AllowsSensitiveByAuthority(authority Authority) bool {
	a := NormalizeAuthority(string(authority))
	switch a {
	case AuthorityPolicyMatch, AuthorityPolicyEnforced, AuthorityHumanApproval, AuthorityOverride:
		return true
	default:
		return false
	}
}
