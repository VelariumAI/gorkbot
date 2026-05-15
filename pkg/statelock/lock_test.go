package statelock

import (
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestLockValidation(t *testing.T) {
	lock := Lock{
		ID:          "l1",
		Scope:       ScopeWorkspace,
		Dimension:   DimensionArtifact,
		Subject:     "artifact:a",
		StateHash:   "hash1",
		Status:      StatusActive,
		PolicyState: PolicyMatched,
		CreatedAt:   time.Now().UTC(),
	}
	if err := lock.Validate(); err != nil {
		t.Fatalf("expected valid lock, got %v", err)
	}

	invalid := Lock{Scope: ScopeUnknown, Dimension: DimensionUnknown}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected invalid lock error")
	}
}

func TestLockMetadataBoundAndRedaction(t *testing.T) {
	lock := Lock{
		ID:          "l2",
		Scope:       ScopeWorkspace,
		Dimension:   DimensionDecision,
		Subject:     "decision:x",
		StateHash:   "abc",
		Status:      StatusActive,
		PolicyState: PolicyMatched,
		CreatedAt:   time.Now().UTC(),
		Metadata: map[string]string{
			"api_key": "SECRET",
			"safe":    "ok",
		},
	}
	n := lock.Normalized()
	if got := n.Metadata["api_key"]; got != "[REDACTED]" {
		t.Fatalf("expected redaction, got %q", got)
	}
	if got := n.Metadata["safe"]; got != "ok" {
		t.Fatalf("expected safe metadata retained, got %q", got)
	}
}

func TestHelperTraceRefs(t *testing.T) {
	lock := Lock{
		ID:             "l3",
		Scope:          ScopeWorkspace,
		Dimension:      DimensionValidationResult,
		Subject:        "test",
		StateHash:      "hash3",
		Status:         StatusActive,
		PolicyState:    PolicyMatched,
		CreatedAt:      time.Now().UTC(),
		ValidationRefs: []trace.Ref{trace.NewRef("v", "r", "h", 1)},
	}
	ref := LockRef(lock)
	if ref.Kind != "state_lock" || ref.Ref == "" {
		t.Fatalf("unexpected lock ref: %#v", ref)
	}

	report := ParadoxReport{Status: ParadoxPossible, Summary: "x", PolicyState: PolicyNoMatch, Risk: RiskSensitive}
	pref := ParadoxRef(report)
	if pref.Kind != "paradox_report" || pref.Ref == "" {
		t.Fatalf("unexpected paradox ref: %#v", pref)
	}
}

func TestLockFromHarnessReport(t *testing.T) {
	r := harness.NewReport("h1", "a1")
	r.Status = harness.StatusPass
	r.FinishedAt = time.Now().UTC()
	lock, err := LockFromHarnessReport(r, ScopeWorkspace, "artifact:a1", PolicyMatched)
	if err != nil {
		t.Fatalf("expected lock from harness report, got %v", err)
	}
	if lock.Dimension != DimensionValidationResult {
		t.Fatalf("unexpected dimension %s", lock.Dimension)
	}
	if len(lock.ValidationRefs) == 0 {
		t.Fatal("expected validation ref")
	}
}

func TestMalformedInputNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	_ = Lock{}.Normalized()
	_ = ProposedState{}.Normalized()
	_ = ParadoxReport{}.Normalized()
	_ = LockRef(Lock{})
}
