package replay

import (
	"context"
	"testing"
)

func TestMemoryStoreSaveLoadList(t *testing.T) {
	store := NewMemoryStore()
	base := testTrajectory("success", "success", 100, 1000)

	c, err := NewCaseFromTrajectory("store", base, CandidateSpec{ID: "store-c1", Kind: CandidateKindPolicy}, Expectations{})
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}
	if err := store.SaveCase(context.Background(), c); err != nil {
		t.Fatalf("save case failed: %v", err)
	}

	loaded, err := store.LoadCase(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("load case failed: %v", err)
	}
	if loaded.ID != c.ID {
		t.Fatalf("expected case id %q, got %q", c.ID, loaded.ID)
	}

	list, err := store.ListCases(context.Background())
	if err != nil {
		t.Fatalf("list cases failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 case, got %d", len(list))
	}

	res := Result{CaseID: c.ID, CandidateID: c.Candidate.ID, Verdict: VerdictPass}
	if err := store.SaveResult(context.Background(), res); err != nil {
		t.Fatalf("save result failed: %v", err)
	}

	loadedRes, err := store.LoadResult(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("load result failed: %v", err)
	}
	if loadedRes.CaseID != c.ID {
		t.Fatalf("expected result case id %q, got %q", c.ID, loadedRes.CaseID)
	}
}

func TestMemoryStoreSaveResultTrimsKey(t *testing.T) {
	store := NewMemoryStore()
	res := Result{CaseID: " case-id ", CandidateID: "c1", Verdict: VerdictPass}

	if err := store.SaveResult(context.Background(), res); err != nil {
		t.Fatalf("save result failed: %v", err)
	}

	loaded, err := store.LoadResult(context.Background(), "case-id")
	if err != nil {
		t.Fatalf("load result failed: %v", err)
	}
	if loaded.CaseID != "case-id" {
		t.Fatalf("expected trimmed case id %q, got %q", "case-id", loaded.CaseID)
	}

	loaded2, err := store.LoadResult(context.Background(), " case-id ")
	if err != nil {
		t.Fatalf("load result with padded id failed: %v", err)
	}
	if loaded2.CaseID != "case-id" {
		t.Fatalf("expected trimmed case id %q, got %q", "case-id", loaded2.CaseID)
	}
}
