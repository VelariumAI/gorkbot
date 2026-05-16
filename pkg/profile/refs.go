package profile

import (
	"sort"

	"github.com/velariumai/gorkbot/pkg/trace"
)

func ConfigID(config Config) string {
	n := config.Normalized()
	parts := []string{
		string(n.Profile),
		n.Description,
		string(n.TraceMode),
		string(n.HarnessMode),
		string(n.StateLockMode),
		string(n.ReplayMode),
		boolString(n.CustomProfileConfigured),
		string(n.Authority.ToolAuthority),
		string(n.Authority.FileAuthority),
		string(n.Authority.NetworkAuthority),
		string(n.Authority.PrivateNetworkAuthority),
		string(n.Authority.ShellAuthority),
		string(n.Authority.SelfmodAuthority),
		string(n.Authority.PromotionAuthority),
		string(n.Authority.PlannerMutationAuthority),
		string(n.Authority.ReleaseAuthority),
		string(n.Authority.HostBridgeAuthority),
		string(n.Authority.WorkspaceEscapeAuthority),
		string(n.Authority.VectorRetrievalAuthority),
		string(n.Automation.AutoPromotionMode),
		string(n.Automation.AutoRepairMode),
		string(n.Automation.AutoRetryMode),
		string(n.Automation.PlannerMutation),
		string(n.Automation.MemoryWriteMode),
		string(n.Automation.SkillWriteMode),
		string(n.Automation.ToolInstallMode),
		string(n.Automation.ReleaseMode),
		boolString(n.Approval.RequireHumanApprovalForSensitive),
		boolString(n.Approval.RequireApprovalForPolicyAbsence),
		boolString(n.Approval.RequireApprovalForRelease),
		boolString(n.Approval.RequireApprovalForNetwork),
		boolString(n.Approval.RequireApprovalForPrivateNetwork),
		boolString(n.Approval.RequireApprovalForShell),
		boolString(n.Approval.RequireApprovalForSelfmod),
		boolString(n.Approval.RequireApprovalForPromotion),
		boolString(n.Approval.RequireApprovalForIrreversibleMutat),
		boolString(n.Evidence.RequireEvidenceReceipt),
		boolString(n.Evidence.RequireTraceRef),
		boolString(n.Evidence.RequireHarnessReport),
		boolString(n.Evidence.RequireReplayNoRegression),
		boolString(n.Evidence.RequireStateLockCheck),
		boolString(n.Evidence.RequireParadoxCheck),
		boolString(n.Evidence.RequireRollbackPlan),
		boolString(n.Evidence.RequireDisablePath),
		boolString(n.Evidence.VectorCandidateOnly),
	}
	if len(n.ConfiguredCapabilities) > 0 {
		caps := make([]string, 0, len(n.ConfiguredCapabilities))
		for capValue := range n.ConfiguredCapabilities {
			caps = append(caps, string(capValue))
		}
		sort.Strings(caps)
		parts = append(parts, caps...)
	}
	if len(n.Metadata) > 0 {
		keys := make([]string, 0, len(n.Metadata))
		for k := range n.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, k, n.Metadata[k])
		}
	}
	return "profile_" + trace.StableHash(parts...)
}

func ConfigRef(config Config) trace.Ref {
	n := config.Normalized()
	id := ConfigID(n)
	return trace.NewRef(
		"profile_config",
		id,
		trace.StableHash(id, string(n.Profile), string(n.TraceMode), string(n.HarnessMode)),
		int64(len(n.Metadata)+len(n.ConfiguredCapabilities)),
	)
}
