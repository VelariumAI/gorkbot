package profile

import "strings"

type AutomationMode string

const (
	AutomationDisabled         AutomationMode = "disabled"
	AutomationAuditOnly        AutomationMode = "audit_only"
	AutomationStaged           AutomationMode = "staged"
	AutomationApprovalRequired AutomationMode = "approval_required"
	AutomationAllowLowRisk     AutomationMode = "allow_low_risk"
	AutomationAllowConfigured  AutomationMode = "allow_configured"
	AutomationSessionLocal     AutomationMode = "session_local"
	AutomationUnknown          AutomationMode = "unknown"
)

func NormalizeAutomationMode(raw string) AutomationMode {
	m := AutomationMode(strings.ToLower(strings.TrimSpace(raw)))
	switch m {
	case AutomationDisabled, AutomationAuditOnly, AutomationStaged,
		AutomationApprovalRequired, AutomationAllowLowRisk, AutomationAllowConfigured,
		AutomationSessionLocal:
		return m
	default:
		return AutomationUnknown
	}
}

func normalizeAutomationValue(m AutomationMode) AutomationMode {
	return NormalizeAutomationMode(string(m))
}

type AutomationConfig struct {
	AutoPromotionMode AutomationMode `json:"auto_promotion_mode"`
	AutoRepairMode    AutomationMode `json:"auto_repair_mode"`
	AutoRetryMode     AutomationMode `json:"auto_retry_mode"`
	PlannerMutation   AutomationMode `json:"planner_mutation_mode"`
	MemoryWriteMode   AutomationMode `json:"memory_write_mode"`
	SkillWriteMode    AutomationMode `json:"skill_write_mode"`
	ToolInstallMode   AutomationMode `json:"tool_install_mode"`
	ReleaseMode       AutomationMode `json:"release_mode"`
}

func (c AutomationConfig) Normalized() AutomationConfig {
	out := c
	out.AutoPromotionMode = normalizeAutomationValue(out.AutoPromotionMode)
	out.AutoRepairMode = normalizeAutomationValue(out.AutoRepairMode)
	out.AutoRetryMode = normalizeAutomationValue(out.AutoRetryMode)
	out.PlannerMutation = normalizeAutomationValue(out.PlannerMutation)
	out.MemoryWriteMode = normalizeAutomationValue(out.MemoryWriteMode)
	out.SkillWriteMode = normalizeAutomationValue(out.SkillWriteMode)
	out.ToolInstallMode = normalizeAutomationValue(out.ToolInstallMode)
	out.ReleaseMode = normalizeAutomationValue(out.ReleaseMode)
	return out
}
