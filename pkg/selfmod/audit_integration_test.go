package selfmod

import (
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/harness"
)

func TestHarnessAuditPassDoesNotChangeSelfmodDecision(t *testing.T) {
	reg := harness.NewRegistry(harness.WithFailClosedUnsupported(false))
	if err := reg.Register(harness.Assertion{
		ID:        "selfmod-required-surface",
		Scope:     "selfmod.dynamic_validation",
		Severity:  harness.SeverityHardFail,
		Type:      harness.AssertionTypeRequiredMetadata,
		Condition: "surface",
		Message:   "surface metadata required",
	}); err != nil {
		t.Fatalf("register assertion: %v", err)
	}
	SetHarnessRuntime(harness.NewRuntime(harness.ModeAudit, reg))
	t.Cleanup(func() { SetHarnessRuntime(harness.NewRuntime(harness.ModeOff, nil)) })

	res := ValidateDynamicProposal(validSelfmodAuditInput())
	if !res.Allowed {
		t.Fatalf("expected selfmod allowed decision to remain allowed, got %#v", res)
	}
	if res.Receipt.AuditSummary == nil {
		t.Fatalf("expected audit summary")
	}
	if res.Receipt.AuditSummary.Mode != harness.ModeAudit {
		t.Fatalf("expected audit mode summary, got %#v", res.Receipt.AuditSummary)
	}
}

func TestHarnessAuditFailDoesNotChangeSelfmodDecision(t *testing.T) {
	reg := harness.NewRegistry(harness.WithFailClosedUnsupported(false))
	if err := reg.Register(harness.Assertion{
		ID:        "selfmod-forbid-surface",
		Scope:     "selfmod.dynamic_validation",
		Severity:  harness.SeverityHardFail,
		Type:      harness.AssertionTypeForbiddenMetadataKey,
		Condition: "surface",
		Message:   "force harness failure",
	}); err != nil {
		t.Fatalf("register assertion: %v", err)
	}
	SetHarnessRuntime(harness.NewRuntime(harness.ModeAudit, reg))
	t.Cleanup(func() { SetHarnessRuntime(harness.NewRuntime(harness.ModeOff, nil)) })

	res := ValidateDynamicProposal(validSelfmodAuditInput())
	if !res.Allowed {
		t.Fatalf("harness failure must not change selfmod allow decision, got %#v", res)
	}
	if res.Receipt.AuditSummary == nil {
		t.Fatalf("expected audit summary")
	}
	if res.Receipt.AuditSummary.FailedCount == 0 {
		t.Fatalf("expected failed harness summary, got %#v", res.Receipt.AuditSummary)
	}
}

func TestBuildHarnessArtifactDoesNotLeakSource(t *testing.T) {
	input := validSelfmodAuditInput()
	decision := DynamicValidationDecision{
		Allowed:          true,
		RequiresApproval: false,
		HardBlock:        false,
		ReasonCode:       "REASON_POLICY_ALLOWED",
		Receipt: DynamicValidationReceipt{
			OperationID:  "op-selfmod-audit",
			ArtifactKind: "dynamic_tool",
			ArtifactName: "safe_tool",
			ManifestHash: "manifest-hash",
			ArtifactHash: "artifact-hash",
			TargetPaths:  []string{"staging/generated/safe_tool.go"},
		},
	}
	artifact := buildHarnessArtifact(input, decision)
	if strings.Contains(artifact.Content, "TOP_SECRET_CODE") {
		t.Fatalf("raw source leaked into harness artifact content")
	}
	for k, v := range artifact.Metadata {
		if strings.Contains(k, "source") || strings.Contains(v, "TOP_SECRET_CODE") {
			t.Fatalf("raw source leaked into metadata: %q=%q", k, v)
		}
	}
}

func validSelfmodAuditInput() ValidateInput {
	return ValidateInput{
		OperationID: "op-selfmod-audit",
		ToolName:    "dynamic_tool_create",
		Mode:        "GOVERNANCE_ENFORCE",
		GeneratedGoSrc: `package generated

func Run() string {
	return "TOP_SECRET_CODE"
}
`,
		Parameters: map[string]any{
			"manifest": map[string]any{
				"name":             "safe_tool",
				"artifact_kind":    "dynamic_tool",
				"risk_class":       "low",
				"capabilities":     []any{"dynamic.skill.stage"},
				"target_paths":     []any{".gorkbot/staging/generated/safe_tool.go"},
				"expected_effects": []any{"adds read-only helper"},
				"rollback_plan":    "remove generated file",
			},
		},
	}
}
