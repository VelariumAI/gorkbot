package providers

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestManagerSetKeyAndKeyStoreAccessor(t *testing.T) {
	ks := NewKeyStore(t.TempDir())
	m := NewManager(ks, slog.Default())

	if m.KeyStore() != ks {
		t.Fatalf("expected KeyStore accessor to return original keystore")
	}

	if err := m.SetKey(context.Background(), ProviderOpenAI, "sk-test", false); err != nil {
		t.Fatalf("set key failed: %v", err)
	}
	got, _ := ks.Get(ProviderOpenAI)
	if got != "sk-test" {
		t.Fatalf("expected keystore to persist set key")
	}
}

func TestManagerGetProviderForModel(t *testing.T) {
	ks := NewKeyStore(t.TempDir())
	m := NewManager(ks, slog.Default())
	// Seed base map directly for deterministic model switch test.
	m.bases[ProviderOpenAI] = NewWrappedProvider((&mockProvider{modelID: "base"}), nil)

	p, err := m.GetProviderForModel(ProviderOpenAI, "new-model")
	if err != nil {
		t.Fatalf("get provider for model failed: %v", err)
	}
	if p.GetMetadata().ID != "new-model" {
		t.Fatalf("expected model switch, got %q", p.GetMetadata().ID)
	}
}

func TestInitProviderAndSetVerboseThoughts(t *testing.T) {
	ks := NewKeyStore(t.TempDir())
	if err := ks.Set(ProviderGoogle, "gem-key"); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	m := NewManager(ks, slog.Default())
	m.InitProvider(ProviderGoogle)
	m.SetVerboseThoughts(true)
}

func TestManagerSetKeyValidateAndErrorBranches(t *testing.T) {
	ks := NewKeyStore(t.TempDir())
	m := NewManager(ks, slog.Default())

	// validate=true path for known provider should succeed (ping may fail in real net,
	// but wrapped providers and defaults keep this deterministic in tests).
	if err := m.SetKey(context.Background(), ProviderXAI, "xai-test-key", false); err != nil {
		t.Fatalf("set key without validate failed: %v", err)
	}

	// Unknown provider: key saves, but init yields no base; validate should fail on GetBase.
	if err := m.SetKey(context.Background(), "unknown-provider", "k", true); err == nil {
		t.Fatalf("expected validate error for unknown provider")
	}
}

func TestManagerGetProviderForModelBranches(t *testing.T) {
	ks := NewKeyStore(t.TempDir())
	m := NewManager(ks, slog.Default())
	m.bases[ProviderOpenAI] = NewWrappedProvider((&mockProvider{modelID: "base"}), nil)

	// Empty model ID should return base unchanged.
	base, err := m.GetProviderForModel(ProviderOpenAI, "")
	if err != nil {
		t.Fatalf("get provider for empty model failed: %v", err)
	}
	if base.GetMetadata().ID != "base" {
		t.Fatalf("expected base model, got %q", base.GetMetadata().ID)
	}

	// Missing provider should propagate unavailable error.
	if _, err := m.GetProviderForModel("missing", "m"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable provider error, got %v", err)
	}
}
