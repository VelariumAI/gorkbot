package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/governance"
)

// TestToolNormalization tests parameter normalization
func TestToolNormalization(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		params   map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "query to pattern",
			toolName: "grep_content",
			params:   map[string]interface{}{"query": "test"},
			expected: map[string]interface{}{"pattern": "test"},
		},
		{
			name:     "cmd to command",
			toolName: "bash",
			params:   map[string]interface{}{"cmd": "ls"},
			expected: map[string]interface{}{"command": "ls"},
		},
		{
			name:     "file to path",
			toolName: "read_file",
			params:   map[string]interface{}{"file": "/test/path"},
			expected: map[string]interface{}{"path": "/test/path"},
		},
		{
			name:     "no change for correct param",
			toolName: "bash",
			params:   map[string]interface{}{"command": "ls"},
			expected: map[string]interface{}{"command": "ls"},
		},
		{
			name:     "nil params returns nil",
			toolName: "bash",
			params:   nil,
			expected: nil,
		},
		{
			name:     "empty params returns empty",
			toolName: "bash",
			params:   map[string]interface{}{},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := NormalizeToolParams(tt.toolName, tt.params)
			if tt.expected == nil {
				if normalized != nil {
					t.Errorf("Expected nil, got %v", normalized)
				}
				return
			}
			for key, expectedVal := range tt.expected {
				if normalized[key] != expectedVal {
					t.Errorf("Expected %s=%v, got %v", key, expectedVal, normalized[key])
				}
			}
		})
	}
}

// TestToolResult tests ToolResult structure
func TestToolResult(t *testing.T) {
	result := &ToolResult{
		Success: true,
		Output:  "test output",
	}

	if !result.Success {
		t.Error("Expected success to be true")
	}

	if result.Output != "test output" {
		t.Errorf("Expected output 'test output', got '%s'", result.Output)
	}
}

type testExecTool struct {
	BaseTool
	exec func(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
}

func newTestExecTool(name string, exec func(ctx context.Context, params map[string]interface{}) (*ToolResult, error)) *testExecTool {
	return &testExecTool{
		BaseTool: NewBaseTool(name, "test tool", CategoryMeta, false, PermissionAlways),
		exec:     exec,
	}
}

func (t *testExecTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *testExecTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	return t.exec(ctx, params)
}

type fakeGovernor struct {
	decision governance.GovernanceDecision
	called   bool
	lastTool string
}

func (f *fakeGovernor) DecideAndApprove(ctx context.Context, action governance.GovernedAction) governance.GovernanceDecision {
	f.called = true
	f.lastTool = action.ToolName
	d := f.decision
	d.ActionID = action.ID
	d.RiskClass = action.RiskClass
	if d.Mode == "" {
		d.Mode = governance.GOVERNANCE_ENFORCE
	}
	return d
}

func TestRegistryExecuteWithoutGovernor(t *testing.T) {
	reg := NewRegistry(nil)
	if err := reg.Register(newTestExecTool("write_file", func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return &ToolResult{Success: true, Output: "ok"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	res, err := reg.Execute(context.Background(), &ToolRequest{
		ToolName:   "write_file",
		Parameters: map[string]interface{}{"path": "a.txt", "content": "x"},
		RequestID:  "r1",
		AgentID:    "gorkbot",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestRegistryExecuteAuditGovernorStillExecutesWhenDenied(t *testing.T) {
	reg := NewRegistry(nil)
	if err := reg.Register(newTestExecTool("bash", func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return &ToolResult{Success: true, Output: "done"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	g := &fakeGovernor{decision: governance.GovernanceDecision{
		Allowed:     false,
		Mode:        governance.GOVERNANCE_AUDIT,
		FinalStatus: governance.GOVERNANCE_AUDIT_ONLY,
		ReasonCode:  governance.REASON_AUDIT_MODE,
	}}
	reg.SetGovernor(g)

	res, err := reg.Execute(context.Background(), &ToolRequest{
		ToolName:   "bash",
		Parameters: map[string]interface{}{"command": "echo test"},
		RequestID:  "r2",
		AgentID:    "gorkbot",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("unexpected result: %#v", res)
	}
	if !g.called {
		t.Fatalf("expected governor to be called")
	}
}

func TestRegistryExecuteGovernorBlocksInEnforce(t *testing.T) {
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
		ReasonCode:  governance.REASON_HUMAN_APPROVAL_UNAVAILABLE,
	}})
	res, err := reg.Execute(context.Background(), &ToolRequest{
		ToolName:   "write_file",
		Parameters: map[string]interface{}{"path": "a.txt", "content": "x", "token": "super-secret"},
		RequestID:  "r3",
		AgentID:    "gorkbot",
	})
	if err == nil {
		t.Fatal("expected blocking error")
	}
	if res == nil || res.Success {
		t.Fatalf("expected blocked result, got %#v", res)
	}
	if strings.Contains(res.Error, "super-secret") {
		t.Fatalf("error should not leak secret: %q", res.Error)
	}
}

func TestRegistryExecuteGovernorAllowsInEnforce(t *testing.T) {
	reg := NewRegistry(nil)
	if err := reg.Register(newTestExecTool("bash", func(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
		return &ToolResult{Success: true, Output: "ok"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	reg.SetGovernor(&fakeGovernor{decision: governance.GovernanceDecision{
		Allowed:     true,
		Mode:        governance.GOVERNANCE_ENFORCE,
		FinalStatus: governance.GOVERNANCE_ALLOWED,
		ReasonCode:  governance.REASON_HUMAN_APPROVAL_GRANTED,
	}})
	res, err := reg.Execute(context.Background(), &ToolRequest{
		ToolName:   "bash",
		Parameters: map[string]interface{}{"command": "echo ok"},
		RequestID:  "r4",
		AgentID:    "gorkbot",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected success, got %#v", res)
	}
}

func TestPermissionHandlerAdapterMapping(t *testing.T) {
	tests := []struct {
		name         string
		level        PermissionLevel
		wantDecision governance.ApprovalDecision
		wantScope    governance.ApprovalScope
	}{
		{"always", PermissionAlways, governance.APPROVAL_GRANTED, governance.APPROVAL_ALWAYS},
		{"session", PermissionSession, governance.APPROVAL_GRANTED, governance.APPROVAL_SESSION},
		{"once", PermissionOnce, governance.APPROVAL_GRANTED, governance.APPROVAL_ONCE},
		{"never", PermissionNever, governance.APPROVAL_DENIED, governance.APPROVAL_NEVER},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(nil)
			reg.SetPermissionHandler(func(toolName string, params map[string]interface{}) PermissionLevel {
				return tt.level
			})
			res, err := reg.RequestApproval(context.Background(), governance.ApprovalRequest{
				ActionID: "a1",
				ToolName: "bash",
			})
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if res.Decision != tt.wantDecision || res.Scope != tt.wantScope {
				t.Fatalf("unexpected approval mapping: %#v", res)
			}
		})
	}
}

func TestPermissionHandlerAdapterUnavailable(t *testing.T) {
	reg := NewRegistry(nil)
	res, err := reg.RequestApproval(context.Background(), governance.ApprovalRequest{ActionID: "a1", ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != governance.APPROVAL_UNAVAILABLE {
		t.Fatalf("expected unavailable, got %#v", res)
	}
}

func TestRegistryRequestApprovalNoHandlerUnavailable(t *testing.T) {
	reg := NewRegistry(nil)
	res, err := reg.RequestApproval(context.Background(), governance.ApprovalRequest{ActionID: "a2", ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != governance.APPROVAL_UNAVAILABLE {
		t.Fatalf("expected unavailable decision, got %#v", res)
	}
}

func TestPermissionHandlerAdapterTimeout(t *testing.T) {
	reg := NewRegistry(nil)
	reg.SetPermissionHandler(func(toolName string, params map[string]interface{}) PermissionLevel {
		time.Sleep(100 * time.Millisecond)
		return PermissionOnce
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	res, err := reg.RequestApproval(ctx, governance.ApprovalRequest{ActionID: "a1", ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != governance.APPROVAL_TIMEOUT {
		t.Fatalf("expected timeout, got %#v", res)
	}
}

func TestRegistryRequestApprovalDoesNotHoldLockDuringCallback(t *testing.T) {
	reg := NewRegistry(nil)
	reg.SetPermissionHandler(func(toolName string, params map[string]interface{}) PermissionLevel {
		// If RequestApproval holds registry locks while calling the callback,
		// this re-entrant read can deadlock.
		_ = reg.IsSecurityModeEnabled()
		return PermissionOnce
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = reg.RequestApproval(context.Background(), governance.ApprovalRequest{
			ActionID: "lock-test",
			ToolName: "bash",
		})
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("RequestApproval appears to hold lock during callback (deadlock)")
	}
}

func TestRegistryApprovalMappingStillCorrect(t *testing.T) {
	cases := []struct {
		level    PermissionLevel
		decision governance.ApprovalDecision
		scope    governance.ApprovalScope
	}{
		{PermissionAlways, governance.APPROVAL_GRANTED, governance.APPROVAL_ALWAYS},
		{PermissionSession, governance.APPROVAL_GRANTED, governance.APPROVAL_SESSION},
		{PermissionOnce, governance.APPROVAL_GRANTED, governance.APPROVAL_ONCE},
		{PermissionNever, governance.APPROVAL_DENIED, governance.APPROVAL_NEVER},
	}
	for _, tc := range cases {
		reg := NewRegistry(nil)
		reg.SetPermissionHandler(func(toolName string, params map[string]interface{}) PermissionLevel {
			return tc.level
		})
		res, err := reg.RequestApproval(context.Background(), governance.ApprovalRequest{
			ActionID: "map-test",
			ToolName: "bash",
		})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.Decision != tc.decision || res.Scope != tc.scope {
			t.Fatalf("unexpected mapping for %v: %#v", tc.level, res)
		}
	}
}
