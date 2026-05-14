package trace

import "testing"

func TestOperatorValidation(t *testing.T) {
	if !OperatorExecute.Valid() {
		t.Fatalf("expected execute operator to be valid")
	}
	if NormalizeOperator("bad-op") != OperatorUnknown {
		t.Fatalf("expected unknown fallback")
	}
}

func TestBoundMetadataRedactsAndBounds(t *testing.T) {
	in := map[string]string{
		"api_key":       "super-secret",
		"ok":            "value",
		"":              "ignored",
		"verylong":      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"session_token": "token",
	}
	out := BoundMetadata(in)
	if got := out["api_key"]; got != "[REDACTED]" {
		t.Fatalf("expected redaction, got %q", got)
	}
	if got := out["session_token"]; got != "[REDACTED]" {
		t.Fatalf("expected token redaction, got %q", got)
	}
	if len(out) == 0 || len(out) > maxMetadataEntries {
		t.Fatalf("unexpected metadata size %d", len(out))
	}
	if len(out["verylong"]) > maxMetadataValueLen {
		t.Fatalf("expected metadata value truncation")
	}
}

func TestEventValidate(t *testing.T) {
	e := NewEvent("sense", "tool_success")
	e.Operator = OperatorExecute
	e = e.Normalized()
	if err := e.Validate(); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}

	e.Operator = Operator("broken")
	if err := e.Validate(); err == nil {
		t.Fatalf("expected invalid operator error")
	}
}
