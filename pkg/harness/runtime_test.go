package harness

import (
	"context"
	"errors"
	"testing"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Mode
	}{
		{name: "default empty", in: "", want: ModeOff},
		{name: "off", in: "off", want: ModeOff},
		{name: "audit", in: "audit", want: ModeAudit},
		{name: "uppercase audit", in: "AUDIT", want: ModeAudit},
		{name: "unknown", in: "enforce", want: ModeOff},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseMode(tc.in); got != tc.want {
				t.Fatalf("ParseMode(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRuntimeOffIsNoop(t *testing.T) {
	rt := NewRuntime(ModeOff, nil)
	if rt.Enabled() {
		t.Fatalf("runtime should be disabled")
	}
	_, err := rt.Validate(context.Background(), Artifact{ID: "a1", Kind: ArtifactKindToolCall})
	if !errors.Is(err, ErrHarnessRuntimeDisabled) {
		t.Fatalf("expected ErrHarnessRuntimeDisabled, got %v", err)
	}
}

func TestRuntimeAuditValidation(t *testing.T) {
	reg := NewRegistry(WithFailClosedUnsupported(false))
	err := reg.Register(Assertion{
		ID:        "a1",
		Scope:     "tool_call",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeStringForbid,
		Condition: "rm -rf",
		Message:   "forbid destructive command",
	})
	if err != nil {
		t.Fatalf("register assertion: %v", err)
	}

	rt := NewRuntime(ModeAudit, reg)
	report, err := rt.Validate(context.Background(), Artifact{ID: "tool-1", Kind: ArtifactKindToolCall, Content: "echo safe"})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if report.Status != StatusPass {
		t.Fatalf("expected pass report, got %q", report.Status)
	}
}

func TestRuntimeAuditNilRegistry(t *testing.T) {
	rt := NewRuntime(ModeAudit, nil)
	report, err := rt.Validate(context.Background(), Artifact{ID: "a2", Kind: ArtifactKindToolCall})
	if !errors.Is(err, ErrHarnessAuditUnavailable) {
		t.Fatalf("expected ErrHarnessAuditUnavailable, got %v", err)
	}
	if report.Status != StatusUnsupported {
		t.Fatalf("expected unsupported status, got %q", report.Status)
	}
}
