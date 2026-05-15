package selfmod

import (
	"context"
	"strings"

	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func applyHarnessAudit(input ValidateInput, decision DynamicValidationDecision) DynamicValidationDecision {
	runtime := harnessRuntime()
	if runtime == nil || runtime.Mode() == harness.ModeOff {
		return decision
	}

	artifact := buildHarnessArtifact(input, decision)
	report, _ := runtime.Validate(context.Background(), artifact)
	summary := harness.SummarizeReport(runtime.Mode(), report)
	decision.AuditSummary = &summary
	decision.Receipt.AuditSummary = &summary
	return decision
}

func buildHarnessArtifact(input ValidateInput, decision DynamicValidationDecision) harness.Artifact {
	receipt := decision.Receipt
	contentHash := strings.TrimSpace(receipt.ArtifactHash)
	if contentHash == "" {
		contentHash = strings.TrimSpace(receipt.ManifestHash)
	}
	artifactID := trace.StableHash(
		"selfmod-audit",
		strings.TrimSpace(input.OperationID),
		strings.TrimSpace(receipt.ManifestHash),
		strings.TrimSpace(receipt.ArtifactHash),
		strings.TrimSpace(decision.ReasonCode),
	)

	refs := []trace.Ref{
		trace.NewRef("operation_id", receipt.OperationID, "", 0),
		trace.NewRef("manifest_hash", receipt.ManifestHash, receipt.ManifestHash, 0),
		trace.NewRef("artifact_hash", receipt.ArtifactHash, receipt.ArtifactHash, 0),
	}
	for _, target := range receipt.TargetPaths {
		refs = append(refs, trace.NewRef("target_path", target, "", 0))
	}

	metadata := map[string]string{
		"surface":           "selfmod",
		"operation_id":      receipt.OperationID,
		"tool_name":         input.ToolName,
		"mode":              input.Mode,
		"reason_code":       decision.ReasonCode,
		"allowed":           boolString(decision.Allowed),
		"requires_approval": boolString(decision.RequiresApproval),
		"hard_block":        boolString(decision.HardBlock),
		"artifact_kind":     receipt.ArtifactKind,
		"risk_class":        receipt.RiskClass,
		"issues_count":      intString(receipt.IssuesCount),
		"target_count":      intString(len(receipt.TargetPaths)),
		"capability_count":  intString(len(receipt.Capabilities)),
		"manifest_hash":     receipt.ManifestHash,
		"artifact_hash":     receipt.ArtifactHash,
	}

	return harness.Artifact{
		ID:          artifactID,
		Kind:        harness.ArtifactKindSelfmodManifest,
		Name:        "selfmod.dynamic_validation",
		ContentHash: contentHash,
		Refs:        refs,
		Metadata:    metadata,
	}
}
