package tools

import (
	"context"
	"testing"

	"github.com/velariumai/gorkbot/pkg/governance"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestRegistryHarnessOffBehaviorUnchanged(t *testing.T) {
	reg := NewRegistry(nil)
	if err := reg.Register(newTestExecTool("write_file", func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return &ToolResult{Success: true, Output: "ok"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	reg.SetGovernor(&fakeGovernor{decision: governance.GovernanceDecision{
		Allowed:     false,
		Mode:        governance.GOVERNANCE_ENFORCE,
		FinalStatus: governance.GOVERNANCE_BLOCKED,
		ReasonCode:  governance.REASON_POLICY_BLOCKED,
	}})
	reg.SetHarnessRuntime(harness.NewRuntime(harness.ModeOff, nil))

	res, err := reg.Execute(context.Background(), &ToolRequest{
		ToolName:   "write_file",
		Parameters: map[string]interface{}{"path": "a.txt", "content": "x"},
		RequestID:  "rh-off",
		AgentID:    "gorkbot",
	})
	if err == nil {
		t.Fatalf("expected governance block")
	}
	if res == nil || res.Success {
		t.Fatalf("expected blocked result, got %#v", res)
	}
}

func TestRegistryHarnessAuditPassDoesNotChangeBehavior(t *testing.T) {
	reg := NewRegistry(nil)
	if err := reg.Register(newTestExecTool("bash", func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return &ToolResult{Success: true, Output: "done"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	reg.SetGovernor(&fakeGovernor{decision: governance.GovernanceDecision{
		Allowed:     true,
		Mode:        governance.GOVERNANCE_ENFORCE,
		FinalStatus: governance.GOVERNANCE_ALLOWED,
		ReasonCode:  governance.REASON_POLICY_ALLOWED,
	}})

	auditReg := harness.NewRegistry(harness.WithFailClosedUnsupported(false))
	if err := auditReg.Register(harness.Assertion{
		ID:        "tool-required-surface",
		Scope:     "tool_registry.governance_decision",
		Severity:  harness.SeverityHardFail,
		Type:      harness.AssertionTypeRequiredMetadata,
		Condition: "surface",
	}); err != nil {
		t.Fatalf("register harness assertion: %v", err)
	}
	reg.SetHarnessRuntime(harness.NewRuntime(harness.ModeAudit, auditReg))

	res, err := reg.Execute(context.Background(), &ToolRequest{
		ToolName:   "bash",
		Parameters: map[string]interface{}{"command": "echo hi"},
		RequestID:  "rh-pass",
		AgentID:    "gorkbot",
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected successful tool execution, got %#v", res)
	}
}

func TestRegistryHarnessAuditFailDoesNotChangeBehaviorAndStaysRedacted(t *testing.T) {
	reg := NewRegistry(nil)
	if err := reg.Register(newTestExecTool("bash", func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return &ToolResult{Success: true, Output: "done"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	reg.SetGovernor(&fakeGovernor{decision: governance.GovernanceDecision{
		Allowed:     true,
		Mode:        governance.GOVERNANCE_ENFORCE,
		FinalStatus: governance.GOVERNANCE_ALLOWED,
		ReasonCode:  governance.REASON_POLICY_ALLOWED,
	}})

	auditReg := harness.NewRegistry(harness.WithFailClosedUnsupported(false))
	if err := auditReg.Register(harness.Assertion{
		ID:        "tool-force-fail",
		Scope:     "tool_registry.governance_decision",
		Severity:  harness.SeverityHardFail,
		Type:      harness.AssertionTypeForbiddenMetadataKey,
		Condition: "surface",
	}); err != nil {
		t.Fatalf("register harness assertion: %v", err)
	}
	reg.SetHarnessRuntime(harness.NewRuntime(harness.ModeAudit, auditReg))

	sink := &governanceCaptureSink{}
	reg.SetTraceSink(sink, trace.ModeAudit)

	res, err := reg.Execute(context.Background(), &ToolRequest{
		ToolName: "bash",
		Parameters: map[string]interface{}{
			"command": "echo hi",
			"token":   "super-secret-token",
		},
		RequestID: "rh-fail",
		AgentID:   "gorkbot",
	})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected successful execution despite harness failure, got %#v", res)
	}

	if len(sink.events) == 0 {
		t.Fatalf("expected governance trace event")
	}
	ev := sink.events[0]
	if ev.Metadata["harness_status"] == "" {
		t.Fatalf("expected harness metadata on governance trace: %#v", ev.Metadata)
	}
	if ev.Metadata["harness_status"] != string(harness.StatusFail) {
		t.Fatalf("expected harness fail status, got %q", ev.Metadata["harness_status"])
	}
	if ev.Metadata["token"] != "" {
		t.Fatalf("trace metadata must not include raw params: %#v", ev.Metadata)
	}
}
