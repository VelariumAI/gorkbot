package evidence

import "strings"

type Risk string

type SensitiveOperation string

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
	SensitiveUnknown          SensitiveOperation = "unknown"
)

var lowRiskOperations = map[string]struct{}{
	"help":                        {},
	"version":                     {},
	"status_metadata":             {},
	"list_non_secret_modes":       {},
	"validate_in_memory_artifact": {},
	"replay_fixture_compare":      {},
	"manifest_count":              {},
	"public_source_inventory":     {},
	"format_user_text":            {},
	"plan_only":                   {},
	"read_metadata":               {},
	"read_trace":                  {},
	"list_locks":                  {},
	"replay_compare":              {},
	"harness_report_read":         {},
}

var mediumRiskOperations = map[string]struct{}{
	"provider_selection":  {},
	"decision_change":     {},
	"policy_update":       {},
	"validation_override": {},
}

func NormalizeRisk(raw string) Risk {
	r := Risk(strings.ToLower(strings.TrimSpace(raw)))
	switch r {
	case RiskLow, RiskMedium, RiskSensitive, RiskUnknown:
		return r
	default:
		return RiskUnknown
	}
}

func NormalizeSensitiveOperation(raw string) SensitiveOperation {
	op := SensitiveOperation(strings.ToLower(strings.TrimSpace(raw)))
	switch op {
	case SensitiveCredentialAccess, SensitiveNetworkEgress, SensitivePrivateNetwork,
		SensitiveFileMutation, SensitiveSelfmodPromotion, SensitiveToolInstallation,
		SensitiveShellExecution, SensitiveReleasePublish, SensitiveHostBridge,
		SensitiveWorkspaceEscape:
		return op
	default:
		return SensitiveUnknown
	}
}

func IsSensitiveOperation(operation string) bool {
	return NormalizeSensitiveOperation(operation) != SensitiveUnknown
}

func IsExplicitLowRiskOperation(operation string) bool {
	_, ok := lowRiskOperations[strings.ToLower(strings.TrimSpace(operation))]
	return ok
}

func ClassifyOperationRisk(operation string) Risk {
	op := strings.ToLower(strings.TrimSpace(operation))
	if op == "" {
		return RiskUnknown
	}
	if IsSensitiveOperation(op) {
		return RiskSensitive
	}
	if _, ok := lowRiskOperations[op]; ok {
		return RiskLow
	}
	if _, ok := mediumRiskOperations[op]; ok {
		return RiskMedium
	}
	return RiskUnknown
}
