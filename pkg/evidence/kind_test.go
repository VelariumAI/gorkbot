package evidence

import "testing"

func TestNormalizeKind(t *testing.T) {
	if got := NormalizeKind("trace_event"); got != KindTraceEvent {
		t.Fatalf("expected trace_event, got %q", got)
	}
	if got := NormalizeKind("whatever"); got != KindUnknown {
		t.Fatalf("expected unknown for unrecognized kind, got %q", got)
	}
}

func TestKindValidate(t *testing.T) {
	if err := KindTraceEvent.Validate(); err != nil {
		t.Fatalf("trace_event should validate: %v", err)
	}
	if err := KindUnknown.Validate(); err == nil {
		t.Fatal("unknown kind should fail validation")
	}
}
