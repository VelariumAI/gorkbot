package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/trace"
)

type Config struct {
	Profile                 Profile             `json:"profile"`
	Description             string              `json:"description,omitempty"`
	Authority               AuthorityConfig     `json:"authority"`
	Automation              AutomationConfig    `json:"automation"`
	Approval                ApprovalConfig      `json:"approval"`
	Evidence                EvidenceConfig      `json:"evidence"`
	TraceMode               trace.Mode          `json:"trace_mode"`
	HarnessMode             harness.Mode        `json:"harness_mode"`
	StateLockMode           Mode                `json:"statelock_mode"`
	ReplayMode              Mode                `json:"replay_mode"`
	CustomProfileConfigured bool                `json:"custom_profile_configured"`
	ConfiguredCapabilities  map[Capability]bool `json:"configured_capabilities,omitempty"`
	Metadata                map[string]string   `json:"metadata,omitempty"`
}

func DefaultConfig(profile Profile) Config {
	norm := normalizeProfileValue(profile)
	if norm == ProfileUnknown {
		norm = ProfileBeginner
	}
	cfg := Config{
		Profile:       norm,
		Description:   defaultDescription(norm),
		Authority:     defaultAuthority(norm),
		Automation:    defaultAutomation(norm),
		Approval:      defaultApproval(norm),
		Evidence:      defaultEvidence(norm),
		TraceMode:     defaultTraceMode(norm),
		HarnessMode:   defaultHarnessMode(norm),
		StateLockMode: defaultStateLockMode(norm),
		ReplayMode:    defaultReplayMode(norm),
		Metadata:      map[string]string{"profile": string(norm)},
	}
	if profile == ProfileCustom {
		cfg.Profile = ProfileCustom
		cfg.Description = defaultDescription(ProfileCustom)
	}
	if profile == ProfileUnknown {
		cfg.Profile = ProfileUnknown
		cfg.Description = defaultDescription(ProfileUnknown)
		cfg.Metadata = map[string]string{"profile": string(ProfileUnknown), "fallback": string(ProfileBeginner)}
	}
	return cfg.Normalized()
}

func (c Config) Normalized() Config {
	out := c
	profile := normalizeProfileValue(out.Profile)
	if profile == ProfileUnknown {
		profile = ProfileBeginner
	}
	out.Profile = profile
	out.Description = trace.RedactString(strings.TrimSpace(out.Description), 256)
	if out.Description == "" {
		out.Description = defaultDescription(profile)
	}

	out.Authority = out.Authority.Normalized()
	out.Automation = out.Automation.Normalized()
	out.Approval = out.Approval.Normalized()
	out.Evidence = out.Evidence.Normalized()
	out.TraceMode = trace.ParseMode(string(out.TraceMode))
	out.HarnessMode = harness.ParseMode(string(out.HarnessMode))
	out.StateLockMode = conservativeMode(out.StateLockMode)
	out.ReplayMode = conservativeMode(out.ReplayMode)
	out.Metadata = boundMetadata(out.Metadata)
	out.ConfiguredCapabilities = normalizeConfiguredCapabilities(out.ConfiguredCapabilities)
	if out.Profile != ProfileCustom {
		out.CustomProfileConfigured = false
	}
	return out
}

func (c Config) Validate() error {
	n := c.Normalized()
	if n.Profile == ProfileCustom && !n.CustomProfileConfigured {
		return ErrCustomProfileNotMarked
	}
	if n.Description == "" {
		return fmt.Errorf("%w: description required", ErrInvalidConfig)
	}
	return nil
}

func (c Config) AllowsConfigured(cap Capability) bool {
	n := c.Normalized()
	_, ok := n.ConfiguredCapabilities[normalizeCapabilityValue(cap)]
	return ok
}

func defaultDescription(profile Profile) string {
	switch profile {
	case ProfileBeginner:
		return "Conservative approval-heavy profile with explicit visibility."
	case ProfileStandard:
		return "Balanced profile with conservative sensitive-operation boundaries."
	case ProfilePowerUser:
		return "Broader local authority with auditable controls and rollback expectations."
	case ProfileExpert:
		return "Explicit policy-pack driven advanced profile with auditability."
	case ProfileLab:
		return "High-automation lab profile requiring explicit receipts for sensitive actions."
	case ProfileEnterprise:
		return "Strict centrally managed profile prioritizing approval and compliance."
	case ProfileCustom:
		return "Custom explicit profile requiring an acknowledged configuration marker."
	default:
		return "Unknown profile fallback using conservative beginner posture."
	}
}

func defaultTraceMode(profile Profile) trace.Mode {
	switch profile {
	case ProfileBeginner:
		return trace.ModeMinimal
	case ProfileStandard:
		return trace.ModeAudit
	case ProfilePowerUser:
		return trace.ModeAudit
	case ProfileExpert:
		return trace.ModeDebug
	case ProfileLab:
		return trace.ModeReplay
	case ProfileEnterprise:
		return trace.ModeAudit
	case ProfileCustom:
		return trace.ModeAudit
	default:
		return trace.ModeMinimal
	}
}

func defaultHarnessMode(profile Profile) harness.Mode {
	switch profile {
	case ProfileBeginner, ProfileStandard, ProfilePowerUser,
		ProfileExpert, ProfileLab, ProfileEnterprise, ProfileCustom:
		return harness.ModeAudit
	default:
		return harness.ModeAudit
	}
}

func defaultStateLockMode(profile Profile) Mode {
	switch profile {
	case ProfileExpert, ProfileEnterprise:
		return ModeEnforce
	case ProfileLab:
		return ModeWarn
	case ProfilePowerUser:
		return ModeAllowConfigured
	default:
		return ModeApprovalRequired
	}
}

func defaultReplayMode(profile Profile) Mode {
	switch profile {
	case ProfileExpert, ProfileLab, ProfileEnterprise:
		return ModeAudit
	case ProfilePowerUser:
		return ModeWarn
	default:
		return ModeApprovalRequired
	}
}

func defaultAuthority(profile Profile) AuthorityConfig {
	base := AuthorityConfig{
		ToolAuthority:            AuthorityPromptAlways,
		FileAuthority:            AuthorityPromptAlways,
		NetworkAuthority:         AuthorityPromptAlways,
		PrivateNetworkAuthority:  AuthorityDeny,
		ShellAuthority:           AuthorityPromptAlways,
		SelfmodAuthority:         AuthorityPromptAlways,
		PromotionAuthority:       AuthorityPromptAlways,
		PlannerMutationAuthority: AuthorityDisabled,
		ReleaseAuthority:         AuthorityDeny,
		HostBridgeAuthority:      AuthorityDeny,
		WorkspaceEscapeAuthority: AuthorityDeny,
		VectorRetrievalAuthority: AuthorityPromptOnce,
	}
	switch profile {
	case ProfileBeginner:
		return base
	case ProfileStandard:
		base.ToolAuthority = AuthorityPromptSession
		base.FileAuthority = AuthorityPromptSession
		base.VectorRetrievalAuthority = AuthorityAllow
		return base
	case ProfilePowerUser:
		base.ToolAuthority = AuthorityAllowConfigured
		base.FileAuthority = AuthorityAllowConfigured
		base.NetworkAuthority = AuthorityPromptSession
		base.ShellAuthority = AuthorityPromptSession
		base.PlannerMutationAuthority = AuthorityPromptSession
		base.VectorRetrievalAuthority = AuthorityAllow
		return base
	case ProfileExpert:
		base.ToolAuthority = AuthorityAllowConfigured
		base.FileAuthority = AuthorityAllowConfigured
		base.NetworkAuthority = AuthorityAllowConfigured
		base.PrivateNetworkAuthority = AuthorityPromptAlways
		base.ShellAuthority = AuthorityAllowConfigured
		base.SelfmodAuthority = AuthorityAllowConfigured
		base.PromotionAuthority = AuthorityAllowConfigured
		base.PlannerMutationAuthority = AuthorityAllowConfigured
		base.ReleaseAuthority = AuthorityAllowConfigured
		base.VectorRetrievalAuthority = AuthorityAllow
		return base
	case ProfileLab:
		base.ToolAuthority = AuthorityAllowConfigured
		base.FileAuthority = AuthorityAllowConfigured
		base.NetworkAuthority = AuthorityAllowConfigured
		base.PrivateNetworkAuthority = AuthorityAllowConfigured
		base.ShellAuthority = AuthorityAllowConfigured
		base.SelfmodAuthority = AuthorityAllowConfigured
		base.PromotionAuthority = AuthorityAllowConfigured
		base.PlannerMutationAuthority = AuthorityAllowConfigured
		base.ReleaseAuthority = AuthorityAllowConfigured
		base.VectorRetrievalAuthority = AuthorityAllow
		return base
	case ProfileEnterprise:
		base.ToolAuthority = AuthorityPromptAlways
		base.FileAuthority = AuthorityPromptAlways
		base.NetworkAuthority = AuthorityPromptAlways
		base.PrivateNetworkAuthority = AuthorityDeny
		base.ShellAuthority = AuthorityPromptAlways
		base.SelfmodAuthority = AuthorityPromptAlways
		base.PromotionAuthority = AuthorityPromptAlways
		base.PlannerMutationAuthority = AuthorityDisabled
		base.ReleaseAuthority = AuthorityDeny
		base.VectorRetrievalAuthority = AuthorityAllow
		return base
	case ProfileCustom:
		base.ToolAuthority = AuthorityAllowConfigured
		base.FileAuthority = AuthorityAllowConfigured
		base.NetworkAuthority = AuthorityAllowConfigured
		base.PrivateNetworkAuthority = AuthorityAllowConfigured
		base.ShellAuthority = AuthorityAllowConfigured
		base.SelfmodAuthority = AuthorityAllowConfigured
		base.PromotionAuthority = AuthorityAllowConfigured
		base.PlannerMutationAuthority = AuthorityAllowConfigured
		base.ReleaseAuthority = AuthorityAllowConfigured
		base.VectorRetrievalAuthority = AuthorityAllowConfigured
		return base
	default:
		return base
	}
}

func defaultAutomation(profile Profile) AutomationConfig {
	base := AutomationConfig{
		AutoPromotionMode: AutomationDisabled,
		AutoRepairMode:    AutomationAuditOnly,
		AutoRetryMode:     AutomationAllowLowRisk,
		PlannerMutation:   AutomationDisabled,
		MemoryWriteMode:   AutomationApprovalRequired,
		SkillWriteMode:    AutomationApprovalRequired,
		ToolInstallMode:   AutomationDisabled,
		ReleaseMode:       AutomationDisabled,
	}
	switch profile {
	case ProfileBeginner, ProfileStandard:
		return base
	case ProfilePowerUser:
		base.AutoPromotionMode = AutomationStaged
		base.PlannerMutation = AutomationSessionLocal
		base.MemoryWriteMode = AutomationAllowLowRisk
		base.SkillWriteMode = AutomationApprovalRequired
		return base
	case ProfileExpert:
		base.AutoPromotionMode = AutomationAllowConfigured
		base.AutoRepairMode = AutomationAllowConfigured
		base.PlannerMutation = AutomationAllowConfigured
		base.MemoryWriteMode = AutomationAllowConfigured
		base.SkillWriteMode = AutomationAllowConfigured
		base.ToolInstallMode = AutomationApprovalRequired
		base.ReleaseMode = AutomationApprovalRequired
		return base
	case ProfileLab:
		base.AutoPromotionMode = AutomationAllowConfigured
		base.AutoRepairMode = AutomationAllowConfigured
		base.PlannerMutation = AutomationAllowConfigured
		base.MemoryWriteMode = AutomationAllowConfigured
		base.SkillWriteMode = AutomationAllowConfigured
		base.ToolInstallMode = AutomationAllowConfigured
		base.ReleaseMode = AutomationApprovalRequired
		return base
	case ProfileEnterprise:
		base.AutoPromotionMode = AutomationDisabled
		base.AutoRepairMode = AutomationAuditOnly
		base.AutoRetryMode = AutomationApprovalRequired
		base.PlannerMutation = AutomationDisabled
		base.MemoryWriteMode = AutomationApprovalRequired
		base.SkillWriteMode = AutomationApprovalRequired
		base.ToolInstallMode = AutomationDisabled
		base.ReleaseMode = AutomationDisabled
		return base
	case ProfileCustom:
		base.AutoPromotionMode = AutomationAllowConfigured
		base.AutoRepairMode = AutomationAllowConfigured
		base.AutoRetryMode = AutomationAllowConfigured
		base.PlannerMutation = AutomationAllowConfigured
		base.MemoryWriteMode = AutomationAllowConfigured
		base.SkillWriteMode = AutomationAllowConfigured
		base.ToolInstallMode = AutomationAllowConfigured
		base.ReleaseMode = AutomationAllowConfigured
		return base
	default:
		return base
	}
}

func defaultApproval(profile Profile) ApprovalConfig {
	base := ApprovalConfig{
		RequireHumanApprovalForSensitive:    true,
		RequireApprovalForPolicyAbsence:     true,
		RequireApprovalForRelease:           true,
		RequireApprovalForNetwork:           true,
		RequireApprovalForPrivateNetwork:    true,
		RequireApprovalForShell:             true,
		RequireApprovalForSelfmod:           true,
		RequireApprovalForPromotion:         true,
		RequireApprovalForIrreversibleMutat: true,
	}
	switch profile {
	case ProfilePowerUser:
		base.RequireApprovalForShell = false
		return base
	case ProfileExpert:
		base.RequireApprovalForShell = false
		base.RequireApprovalForNetwork = false
		return base
	case ProfileLab:
		base.RequireApprovalForNetwork = false
		base.RequireApprovalForShell = false
		base.RequireApprovalForPrivateNetwork = false
		return base
	case ProfileCustom:
		base.RequireApprovalForNetwork = false
		base.RequireApprovalForShell = false
		base.RequireApprovalForPrivateNetwork = false
		return base
	default:
		return base
	}
}

func defaultEvidence(profile Profile) EvidenceConfig {
	base := EvidenceConfig{
		RequireEvidenceReceipt:    true,
		RequireTraceRef:           true,
		RequireHarnessReport:      true,
		RequireReplayNoRegression: true,
		RequireStateLockCheck:     true,
		RequireParadoxCheck:       true,
		RequireRollbackPlan:       true,
		RequireDisablePath:        true,
		VectorCandidateOnly:       true,
	}
	switch profile {
	case ProfileBeginner, ProfileStandard, ProfilePowerUser,
		ProfileExpert, ProfileLab, ProfileEnterprise, ProfileCustom:
		return base
	default:
		return base
	}
}

func boundMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	bounded := trace.BoundMetadata(in)
	if len(bounded) == 0 {
		return nil
	}
	keys := make([]string, 0, len(bounded))
	for k := range bounded {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 16 {
		keys = keys[:16]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = bounded[k]
	}
	return out
}

func normalizeConfiguredCapabilities(in map[Capability]bool) map[Capability]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[Capability]bool, len(in))
	for capValue, allowed := range in {
		if !allowed {
			continue
		}
		norm := normalizeCapabilityValue(capValue)
		if norm == CapabilityUnknown {
			continue
		}
		out[norm] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
