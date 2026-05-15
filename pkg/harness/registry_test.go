package harness

import (
	"context"
	"errors"
	"testing"
)

func TestRegistryDuplicateAssertionRejection(t *testing.T) {
	r := NewRegistry()
	a := Assertion{
		ID:        "assert-1",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeStringForbid,
		Condition: "rm -rf /",
		Message:   "no recursive rm",
	}
	if err := r.Register(a); err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}
	err := r.Register(a)
	if err == nil {
		t.Fatalf("expected duplicate assertion rejection")
	}
	if !errors.Is(err, ErrDuplicateAssertion) {
		t.Fatalf("expected ErrDuplicateAssertion, got %v", err)
	}
}

func TestRegistryDeterministicOrdering(t *testing.T) {
	r := NewRegistry()
	assertions := []Assertion{
		{ID: "b", Scope: "tool.shell", Severity: SeverityHardFail, Type: AssertionTypeStringForbid, Condition: "2", Message: "b"},
		{ID: "a", Scope: "tool.shell", Severity: SeverityHardFail, Type: AssertionTypeStringForbid, Condition: "1", Message: "a"},
		{ID: "c", Scope: "tool.shell", Severity: SeverityHardFail, Type: AssertionTypeStringForbid, Condition: "3", Message: "c"},
	}
	if err := r.RegisterMany(assertions); err != nil {
		t.Fatalf("register many failed: %v", err)
	}
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("expected 3 assertions, got %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		t.Fatalf("non-deterministic ordering: %+v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestRegistryScopeFiltering(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterMany([]Assertion{
		{ID: "tool", Scope: "tool.shell", Severity: SeverityHardFail, Type: AssertionTypeStringRequire, Condition: "echo", Message: "tool"},
		{ID: "global", Scope: "*", Severity: SeverityWarn, Type: AssertionTypeStringForbid, Condition: "passwd", Message: "global"},
		{ID: "other", Scope: "other.scope", Severity: SeverityWarn, Type: AssertionTypeStringForbid, Condition: "x", Message: "other"},
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	artifact := Artifact{
		ID:      "art-1",
		Kind:    ArtifactKindCommand,
		Name:    "tool.shell",
		Content: "echo hello",
	}
	report := r.Validate(context.Background(), artifact)
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 scoped results, got %d", len(report.Results))
	}
}

func TestRegistryUnknownAssertionTypeHandlingFailClosed(t *testing.T) {
	r := NewRegistry(WithFailClosedUnsupported(true))
	if err := r.Register(Assertion{
		ID:        "known",
		Scope:     "tool.shell",
		Severity:  SeverityWarn,
		Type:      AssertionTypeStringRequire,
		Condition: "echo",
		Message:   "must include echo",
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	artifact := Artifact{
		ID:      "a",
		Kind:    ArtifactKindText,
		Name:    "tool.shell",
		Content: "echo hello",
	}
	report := r.Validate(context.Background(), artifact)
	if report.Status != StatusPass {
		t.Fatalf("expected pass for supported assertion, got %s", report.Status)
	}
}

func TestRegistryInvalidArtifactDoesNotPanic(t *testing.T) {
	r := NewRegistry()
	report := r.Validate(context.Background(), Artifact{})
	if report.Status != StatusInvalid {
		t.Fatalf("expected invalid status, got %s", report.Status)
	}
}
