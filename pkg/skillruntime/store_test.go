package skillruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestMemoryStoreDeterministicListingAndDefensiveCloning(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	c1 := Candidate{Name: "b", Source: "s", Risk: evidence.RiskLow}
	c2 := Candidate{Name: "a", Source: "s", Risk: evidence.RiskLow}
	if err := store.SaveCandidate(ctx, c1); err != nil {
		t.Fatalf("save c1: %v", err)
	}
	if err := store.SaveCandidate(ctx, c2); err != nil {
		t.Fatalf("save c2: %v", err)
	}

	list, err := store.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(list))
	}
	if list[0].ID > list[1].ID {
		t.Fatalf("candidate list must be deterministic by id")
	}

	mut := list[0]
	mut.Metadata = map[string]string{"x": "y"}
	loaded, err := store.LoadCandidate(ctx, list[0].ID)
	if err != nil {
		t.Fatalf("load candidate: %v", err)
	}
	if loaded.Metadata["x"] != "" {
		t.Fatalf("load candidate should return defensive clone")
	}

	res := Result{
		Operation:   OperationPropose,
		CandidateID: loaded.ID,
		Status:      StatusAllowed,
		Decision:    evidence.DecisionAuditOnly,
		Assessment: evidence.Assessment{
			PolicyState: evidence.PolicyMatched,
			Risk:        evidence.RiskMedium,
			Operation:   "skill_stage",
			Authority:   evidence.AuthorityPolicyMatch,
		},
		Receipt: evidence.Receipt{
			Status: evidence.StatusWarn,
			Assessment: evidence.Assessment{
				PolicyState: evidence.PolicyMatched,
				Risk:        evidence.RiskMedium,
				Operation:   "skill_stage",
				Authority:   evidence.AuthorityPolicyMatch,
			},
		},
		ValidationRefs: []trace.Ref{trace.NewRef("v", "r", "h", 1)},
	}
	if err := store.SaveResult(ctx, res); err != nil {
		t.Fatalf("save result: %v", err)
	}
	loadedRes, err := store.LoadResult(ctx, res.Normalized().ID)
	if err != nil {
		t.Fatalf("load result: %v", err)
	}
	loadedRes.Warnings = append(loadedRes.Warnings, "mutated")
	loadedRes2, err := store.LoadResult(ctx, res.Normalized().ID)
	if err != nil {
		t.Fatalf("load result second: %v", err)
	}
	if len(loadedRes2.Warnings) != 0 {
		t.Fatalf("result clone expected, got warnings=%v", loadedRes2.Warnings)
	}
}

func TestMemoryStoreDisableAndNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	c := Candidate{Name: "x", Source: "s", Risk: evidence.RiskLow}
	if err := store.SaveCandidate(ctx, c); err != nil {
		t.Fatalf("save candidate: %v", err)
	}
	id := c.Normalized().ID

	if err := store.DisableCandidate(ctx, id); err != nil {
		t.Fatalf("disable candidate: %v", err)
	}
	out, err := store.LoadCandidate(ctx, id)
	if err != nil {
		t.Fatalf("load disabled candidate: %v", err)
	}
	if !out.Disabled {
		t.Fatalf("expected candidate disabled flag")
	}

	if _, err := store.LoadCandidate(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing candidate, got %v", err)
	}
	if _, err := store.LoadResult(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing result, got %v", err)
	}
	if err := store.DisableCandidate(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for disable missing candidate, got %v", err)
	}
}
