package harness

import (
	"context"
	"testing"
)

func TestEvaluatorRegexForbidPassFail(t *testing.T) {
	assertion := Assertion{
		ID:        "a1",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeRegexForbid,
		Condition: `rm\s+-rf\s+/`,
		Message:   "no destructive rm",
	}
	artifactPass := Artifact{ID: "art-pass", Kind: ArtifactKindText, Content: "rm -rf ./build"}
	artifactFail := Artifact{ID: "art-fail", Kind: ArtifactKindText, Content: "rm -rf /"}

	if got := evaluateAssertion(artifactPass, assertion); got.Status != StatusPass {
		t.Fatalf("expected pass, got %s", got.Status)
	}
	if got := evaluateAssertion(artifactFail, assertion); got.Status != StatusFail {
		t.Fatalf("expected fail, got %s", got.Status)
	}
}

func TestEvaluatorRegexRequirePassFail(t *testing.T) {
	assertion := Assertion{
		ID:        "a2",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeRegexRequire,
		Condition: `echo\s+\w+`,
		Message:   "must echo",
	}
	if got := evaluateAssertion(Artifact{ID: "x", Kind: ArtifactKindText, Content: "echo hello"}, assertion); got.Status != StatusPass {
		t.Fatalf("expected pass, got %s", got.Status)
	}
	if got := evaluateAssertion(Artifact{ID: "y", Kind: ArtifactKindText, Content: "pwd"}, assertion); got.Status != StatusFail {
		t.Fatalf("expected fail, got %s", got.Status)
	}
}

func TestEvaluatorStringForbidPassFail(t *testing.T) {
	assertion := Assertion{
		ID:        "a3",
		Scope:     "tool.shell",
		Severity:  SeverityWarn,
		Type:      AssertionTypeStringForbid,
		Condition: "password=",
		Message:   "no passwords",
	}
	if got := evaluateAssertion(Artifact{ID: "x", Kind: ArtifactKindText, Content: "echo hello"}, assertion); got.Status != StatusPass {
		t.Fatalf("expected pass, got %s", got.Status)
	}
	if got := evaluateAssertion(Artifact{ID: "y", Kind: ArtifactKindText, Content: "password=123"}, assertion); got.Status != StatusWarn {
		t.Fatalf("expected warn, got %s", got.Status)
	}
}

func TestEvaluatorStringRequirePassFail(t *testing.T) {
	assertion := Assertion{
		ID:        "a4",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeStringRequire,
		Condition: "go test",
		Message:   "must run tests",
	}
	if got := evaluateAssertion(Artifact{ID: "x", Kind: ArtifactKindText, Content: "go test ./..."}, assertion); got.Status != StatusPass {
		t.Fatalf("expected pass, got %s", got.Status)
	}
	if got := evaluateAssertion(Artifact{ID: "y", Kind: ArtifactKindText, Content: "go build ./..."}, assertion); got.Status != StatusFail {
		t.Fatalf("expected fail, got %s", got.Status)
	}
}

func TestEvaluatorMaxLengthPassFail(t *testing.T) {
	assertion := Assertion{
		ID:        "a5",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeMaxLength,
		Condition: "10",
		Message:   "length limit",
	}
	if got := evaluateAssertion(Artifact{ID: "x", Kind: ArtifactKindText, Content: "short"}, assertion); got.Status != StatusPass {
		t.Fatalf("expected pass, got %s", got.Status)
	}
	if got := evaluateAssertion(Artifact{ID: "y", Kind: ArtifactKindText, Content: "this-is-way-too-long"}, assertion); got.Status != StatusFail {
		t.Fatalf("expected fail, got %s", got.Status)
	}
}

func TestEvaluatorRequiredMetadataPassFail(t *testing.T) {
	assertion := Assertion{
		ID:        "a6",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeRequiredMetadata,
		Condition: "owner",
		Message:   "owner required",
	}
	if got := evaluateAssertion(Artifact{ID: "x", Kind: ArtifactKindText, Metadata: map[string]string{"owner": "sec"}}, assertion); got.Status != StatusPass {
		t.Fatalf("expected pass, got %s", got.Status)
	}
	if got := evaluateAssertion(Artifact{ID: "y", Kind: ArtifactKindText, Metadata: map[string]string{"team": "sec"}}, assertion); got.Status != StatusFail {
		t.Fatalf("expected fail, got %s", got.Status)
	}
}

func TestEvaluatorForbiddenMetadataKeyPassFail(t *testing.T) {
	assertion := Assertion{
		ID:        "a7",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeForbiddenMetadataKey,
		Condition: "api_key",
		Message:   "api key forbidden",
	}
	if got := evaluateAssertion(Artifact{ID: "x", Kind: ArtifactKindText, Metadata: map[string]string{"owner": "sec"}}, assertion); got.Status != StatusPass {
		t.Fatalf("expected pass, got %s", got.Status)
	}
	if got := evaluateAssertion(Artifact{ID: "y", Kind: ArtifactKindText, Metadata: map[string]string{"api-key": "abc"}}, assertion); got.Status != StatusFail {
		t.Fatalf("expected fail, got %s", got.Status)
	}
}

func TestEvaluatorInvalidRegexHandling(t *testing.T) {
	assertion := Assertion{
		ID:        "a8",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeRegexRequire,
		Condition: "(",
		Message:   "bad regex",
	}
	got := evaluateAssertion(Artifact{ID: "x", Kind: ArtifactKindText, Content: "echo"}, assertion)
	if got.Status != StatusInvalid {
		t.Fatalf("expected invalid status, got %s", got.Status)
	}
}

func TestUnknownAssertionTypeHandling(t *testing.T) {
	r := NewRegistry(WithFailClosedUnsupported(false))
	r.byID["u1"] = Assertion{
		ID:        "u1",
		Scope:     "tool.shell",
		Severity:  SeverityWarn,
		Type:      AssertionType("unknown_type"),
		Condition: "x",
		Message:   "unsupported",
	}
	r.rebuildSortedIDsLocked()

	report := r.Validate(context.Background(), Artifact{
		ID:      "a1",
		Kind:    ArtifactKindText,
		Name:    "tool.shell",
		Content: "echo",
	})
	if report.Status != StatusUnsupported {
		t.Fatalf("expected unsupported report status, got %s", report.Status)
	}

	rFailClosed := NewRegistry(WithFailClosedUnsupported(true))
	rFailClosed.byID["u2"] = Assertion{
		ID:        "u2",
		Scope:     "tool.shell",
		Severity:  SeverityWarn,
		Type:      AssertionType("unknown_type"),
		Condition: "x",
		Message:   "unsupported",
	}
	rFailClosed.rebuildSortedIDsLocked()
	reportFailClosed := rFailClosed.Validate(context.Background(), Artifact{
		ID:      "a2",
		Kind:    ArtifactKindText,
		Name:    "tool.shell",
		Content: "echo",
	})
	if reportFailClosed.Status != StatusFail {
		t.Fatalf("expected fail-closed status fail, got %s", reportFailClosed.Status)
	}
}

func TestGoldenPoisonedFixtures(t *testing.T) {
	assertion := Assertion{
		ID:        "fixture-check",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeRegexForbid,
		Condition: `rm\s+-rf\s+/`,
		Message:   "unsafe rm",
		Golden: []string{
			"rm -rf ./build/tmp",
		},
		Poisoned: []string{
			"rm -rf /",
		},
	}
	if err := ValidateAssertionFixtures(assertion); err != nil {
		t.Fatalf("expected fixtures to validate, got %v", err)
	}
}

func TestMalformedFixtureReturnsError(t *testing.T) {
	assertion := Assertion{
		ID:        "fixture-bad",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeRegexForbid,
		Condition: `rm\s+-rf\s+/`,
		Message:   "unsafe rm",
		Golden: []string{
			"rm -rf /",
		},
	}
	if err := ValidateAssertionFixtures(assertion); err == nil {
		t.Fatalf("expected malformed fixture behavior error")
	}
}
