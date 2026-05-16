package statelock

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreSaveLoadListDeterministic(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	l1 := Lock{ID: "b", Scope: ScopeWorkspace, Dimension: DimensionArtifact, Subject: "s2", StateHash: "h", Status: StatusActive, PolicyState: PolicyMatched, CreatedAt: time.Now().UTC()}
	l2 := Lock{ID: "a", Scope: ScopeWorkspace, Dimension: DimensionArtifact, Subject: "s1", StateHash: "h", Status: StatusActive, PolicyState: PolicyMatched, CreatedAt: time.Now().UTC()}
	if err := store.SaveLock(ctx, l1); err != nil {
		t.Fatalf("save lock1: %v", err)
	}
	if err := store.SaveLock(ctx, l2); err != nil {
		t.Fatalf("save lock2: %v", err)
	}
	list, err := store.ListLocks(ctx, Filter{Scope: ScopeWorkspace, Dimension: DimensionArtifact})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 locks, got %d", len(list))
	}
	if list[0].Subject != "s1" || list[1].Subject != "s2" {
		t.Fatalf("expected deterministic order by subject/id, got %#v", list)
	}

	loaded, err := store.LoadLock(ctx, "a")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ID != "a" {
		t.Fatalf("unexpected loaded lock: %#v", loaded)
	}
}

func TestMemoryStoreDefensiveCloning(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	lock := Lock{
		ID:          "x",
		Scope:       ScopeWorkspace,
		Dimension:   DimensionArtifact,
		Subject:     "s",
		StateHash:   "h",
		Status:      StatusActive,
		PolicyState: PolicyMatched,
		CreatedAt:   time.Now().UTC(),
		Metadata:    map[string]string{"safe": "ok"},
	}
	if err := store.SaveLock(ctx, lock); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := store.LoadLock(ctx, "x")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Metadata["safe"] = "changed"
	loadedAgain, err := store.LoadLock(ctx, "x")
	if err != nil {
		t.Fatalf("load again: %v", err)
	}
	if loadedAgain.Metadata["safe"] != "ok" {
		t.Fatal("expected defensive clone for metadata")
	}
}

func TestMemoryStoreParadoxSaveLoadNotFound(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	report := ParadoxReport{Status: ParadoxPossible, Summary: "s", PolicyState: PolicyMatched, Risk: RiskLow, StartedAt: time.Now().UTC()}
	if err := store.SaveParadox(ctx, report); err != nil {
		t.Fatalf("save paradox: %v", err)
	}
	_, err := store.LoadParadox(ctx, "missing")
	if err == nil {
		t.Fatal("expected not found")
	}
}
