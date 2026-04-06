package providers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

type mockAIProvider struct {
	pingErr error
}

func (m *mockAIProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return "", nil
}

func (m *mockAIProvider) GenerateWithHistory(ctx context.Context, history *ai.ConversationHistory) (string, error) {
	return "", nil
}

func (m *mockAIProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	return nil
}

func (m *mockAIProvider) StreamWithHistory(ctx context.Context, history *ai.ConversationHistory, out io.Writer) error {
	return nil
}

func (m *mockAIProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{}
}

func (m *mockAIProvider) Name() string {
	return "mock"
}

func (m *mockAIProvider) ID() registry.ProviderID {
	return "mock"
}

func (m *mockAIProvider) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockAIProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return nil, nil
}

func (m *mockAIProvider) WithModel(model string) ai.AIProvider {
	return m
}

func TestKeyStoreSeedFromEnvAndSetGet(t *testing.T) {
	t.Setenv("XAI_API_KEY", "xai-from-env")
	dir := t.TempDir()
	ks := NewKeyStore(dir)

	key, status := ks.Get(ProviderXAI)
	if key != "xai-from-env" {
		t.Fatalf("expected env-seeded key, got %q", key)
	}
	if status != KeyStatusUnverified {
		t.Fatalf("expected unverified status for env-seeded key, got %v", status)
	}

	if err := ks.Set(ProviderXAI, "xai-manual"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	key, status = ks.Get(ProviderXAI)
	if key != "xai-manual" || status != KeyStatusUnverified {
		t.Fatalf("unexpected key/status after Set: %q %v", key, status)
	}

	// Re-open from disk to verify persistence.
	ks2 := NewKeyStore(dir)
	key, status = ks2.Get(ProviderXAI)
	if key != "xai-manual" || status != KeyStatusUnverified {
		t.Fatalf("unexpected persisted key/status: %q %v", key, status)
	}
}

func TestKeyStoreEnvDoesNotOverridePersistedKey(t *testing.T) {
	dir := t.TempDir()
	ks := NewKeyStore(dir)
	if err := ks.Set(ProviderGoogle, "persisted-key"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	t.Setenv("GEMINI_API_KEY", "env-key")
	ks2 := NewKeyStore(dir)
	key, _ := ks2.Get(ProviderGoogle)
	if key != "persisted-key" {
		t.Fatalf("expected persisted key to win over env seed, got %q", key)
	}
}

func TestKeyStoreValidateUpdatesStatus(t *testing.T) {
	dir := t.TempDir()
	ks := NewKeyStore(dir)
	if err := ks.Set(ProviderOpenAI, "sk-test"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	okProv := &mockAIProvider{}
	if err := ks.Validate(context.Background(), ProviderOpenAI, okProv, slog.Default()); err != nil {
		t.Fatalf("Validate success path failed: %v", err)
	}
	_, status := ks.Get(ProviderOpenAI)
	if status != KeyStatusValid {
		t.Fatalf("expected valid status, got %v", status)
	}

	failProv := &mockAIProvider{pingErr: errors.New("bad key")}
	err := ks.Validate(context.Background(), ProviderOpenAI, failProv, slog.Default())
	if err == nil {
		t.Fatal("expected Validate to fail when provider ping fails")
	}
	_, status = ks.Get(ProviderOpenAI)
	if status != KeyStatusInvalid {
		t.Fatalf("expected invalid status, got %v", status)
	}
}

func TestKeyStoreSubscribeReceivesUpdates(t *testing.T) {
	dir := t.TempDir()
	ks := NewKeyStore(dir)
	ch := ks.Subscribe()

	if err := ks.Set(ProviderAnthropic, "sk-ant"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	select {
	case provider := <-ch:
		if provider != ProviderAnthropic {
			t.Fatalf("unexpected provider update: %q", provider)
		}
	default:
		t.Fatal("expected provider update notification")
	}
}

func TestAllProvidersReturnsCopy(t *testing.T) {
	got := AllProviders()
	if len(got) == 0 {
		t.Fatal("expected non-empty providers list")
	}
	origFirst := got[0]
	got[0] = "mutated"
	again := AllProviders()
	if len(again) == 0 || again[0] != origFirst {
		t.Fatal("AllProviders must return a copy, not mutable global backing")
	}
}

func TestDirAndPathPersistenceLocation(t *testing.T) {
	dir := t.TempDir()
	ks := NewKeyStore(dir)
	if ks.Dir() != dir {
		t.Fatalf("unexpected dir: %q", ks.Dir())
	}
	if err := ks.Set(ProviderMiniMax, "mm-key"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	expected := filepath.Join(dir, "api_keys.json")
	ks2 := NewKeyStore(dir)
	if _, status := ks2.Get(ProviderMiniMax); status == KeyStatusMissing {
		t.Fatalf("expected persisted key at %s", expected)
	}
}
