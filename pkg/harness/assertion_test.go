package harness

import (
	"errors"
	"testing"
)

func TestAssertionValidation(t *testing.T) {
	a := Assertion{
		ID:        "a-1",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeStringForbid,
		Condition: "rm -rf /",
		Message:   "unsafe command",
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("expected valid assertion, got %v", err)
	}
}

func TestAssertionInvalidRegex(t *testing.T) {
	a := Assertion{
		ID:        "a-invalid-regex",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionTypeRegexForbid,
		Condition: "(",
		Message:   "bad regex",
	}
	err := a.Validate()
	if err == nil {
		t.Fatalf("expected invalid regex error")
	}
	if !errors.Is(err, ErrInvalidAssertion) {
		t.Fatalf("expected ErrInvalidAssertion, got %v", err)
	}
}

func TestAssertionUnknownType(t *testing.T) {
	a := Assertion{
		ID:        "a-unsupported",
		Scope:     "tool.shell",
		Severity:  SeverityHardFail,
		Type:      AssertionType("not_supported"),
		Condition: "x",
	}
	err := a.Validate()
	if err == nil {
		t.Fatalf("expected unsupported assertion error")
	}
	if !errors.Is(err, ErrUnsupportedAssertion) {
		t.Fatalf("expected ErrUnsupportedAssertion, got %v", err)
	}
}

func TestAssertionMalformedNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	_ = (Assertion{}).Normalized()
	_ = (Assertion{}).Validate()
}
