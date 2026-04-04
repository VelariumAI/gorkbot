package engine

import "testing"

type testLogger struct{}

func (l *testLogger) Debug(msg string, args ...interface{}) {}
func (l *testLogger) Error(msg string, args ...interface{}) {}

func TestMessageSuppression_VerbosePassThrough(t *testing.T) {
	m := NewMessageSuppressionMiddleware(true, &testLogger{})
	in := "I'm executing the tool now"
	if got := m.ProcessResponse(in); got != in {
		t.Fatalf("expected pass-through response, got %q", got)
	}
	if got := m.ProcessStreamingToken(in); got != in {
		t.Fatalf("expected pass-through token, got %q", got)
	}
}

func TestMessageSuppression_NonVerboseStreamingSuppression(t *testing.T) {
	m := NewMessageSuppressionMiddleware(false, &testLogger{})

	if got := m.ProcessStreamingToken("I'm executing the filesystem tool"); got != "" {
		t.Fatalf("expected narration token suppressed, got %q", got)
	}
	if got := m.ProcessStreamingToken("=== SYSTEM STATUS ==="); got != "" {
		t.Fatalf("expected status token suppressed, got %q", got)
	}
	if got := m.ProcessStreamingToken("Normal user-facing content"); got == "" {
		t.Fatalf("expected normal content to remain visible")
	}
}

func TestMessageSuppression_ModeSwitch(t *testing.T) {
	m := NewMessageSuppressionMiddleware(false, &testLogger{})
	if m.IsVerbose() {
		t.Fatalf("expected non-verbose at start")
	}

	m.SetVerboseMode(true)
	if !m.IsVerbose() {
		t.Fatalf("expected verbose after switch")
	}

	in := "Tool has completed successfully"
	if got := m.ProcessStreamingToken(in); got != in {
		t.Fatalf("expected pass-through token after verbose switch, got %q", got)
	}

	m.SetVerboseMode(false)
	if m.IsVerbose() {
		t.Fatalf("expected non-verbose after second switch")
	}
	if got := m.ProcessStreamingToken(in); got != "" {
		t.Fatalf("expected token suppressed in non-verbose mode, got %q", got)
	}
}
