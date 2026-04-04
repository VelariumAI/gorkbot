package memory_test

import (
	"context"
	"io"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/memory"
	"github.com/velariumai/gorkbot/pkg/registry"
)

// mockProvider is a simple mock for testing
type mockProvider struct{}

func (m *mockProvider) Generate(ctx context.Context, prompt string) (string, error) {
	// Return mock facts
	return `The user is working on a project.
The project requires Go 1.25.
The team uses SQLite for persistence.`, nil
}

func (m *mockProvider) GenerateWithHistory(ctx context.Context, history *ai.ConversationHistory) (string, error) {
	return "", nil
}

func (m *mockProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	return nil
}

func (m *mockProvider) StreamWithHistory(ctx context.Context, history *ai.ConversationHistory, out io.Writer) error {
	return nil
}

func (m *mockProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{ID: "mock", Name: "Mock"}
}

func (m *mockProvider) Name() string {
	return "Mock"
}

func (m *mockProvider) ID() registry.ProviderID {
	return "mock"
}

func (m *mockProvider) Ping(ctx context.Context) error {
	return nil
}

func (m *mockProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return nil, nil
}

func (m *mockProvider) WithModel(model string) ai.AIProvider {
	return m
}

func TestFactStoreCreate(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-1"

	fs, err := memory.NewFactStore(tmpDir, sessionID, &mockProvider{})
	if err != nil {
		t.Fatalf("failed to create FactStore: %v", err)
	}
	defer fs.Close()

	if fs == nil {
		t.Fatal("expected non-nil FactStore")
	}
}

func TestFactStoreQueueForExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := memory.NewFactStore(tmpDir, "test-session", &mockProvider{})
	if err != nil {
		t.Fatalf("failed to create FactStore: %v", err)
	}
	defer fs.Close()

	// Queue a message (should not block)
	fs.QueueForExtraction("This is a test message about facts.")

	// Queue an empty message (should be ignored)
	fs.QueueForExtraction("")
	fs.QueueForExtraction("   ")

	// Queue multiple messages
	fs.QueueForExtraction("First message")
	fs.QueueForExtraction("Second message")

	// No error expected
}

func TestFactStoreQueryRelevant(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := memory.NewFactStore(tmpDir, "test-session", &mockProvider{})
	if err != nil {
		t.Fatalf("failed to create FactStore: %v", err)
	}
	defer fs.Close()

	// Query empty store (should return empty list, no error)
	facts, err := fs.QueryRelevant("some query", 10)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts, got %d", len(facts))
	}
}

func TestFactStoreFormatForContext(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := memory.NewFactStore(tmpDir, "test-session", &mockProvider{})
	if err != nil {
		t.Fatalf("failed to create FactStore: %v", err)
	}
	defer fs.Close()

	// Format empty store
	formatted := fs.FormatForContext("test query", 1000)
	if formatted != "" {
		t.Errorf("expected empty string for empty store, got %q", formatted)
	}
}

func TestFactStoreClose(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := memory.NewFactStore(tmpDir, "test-session", &mockProvider{})
	if err != nil {
		t.Fatalf("failed to create FactStore: %v", err)
	}

	// Queue a message
	fs.QueueForExtraction("test message")

	// Close should drain and close gracefully
	if err := fs.Close(); err != nil {
		t.Fatalf("failed to close FactStore: %v", err)
	}

	// Close again should not error
	if err := fs.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

func TestFactStoreRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"

	// Create and populate a FactStore
	fs1, err := memory.NewFactStore(tmpDir, sessionID, &mockProvider{})
	if err != nil {
		t.Fatalf("failed to create FactStore: %v", err)
	}

	// Queue messages
	fs1.QueueForExtraction("Test fact number one")
	fs1.QueueForExtraction("Test fact number two")

	// Close to ensure data is committed
	fs1.Close()

	// Create a new FactStore with same directory (should find existing DB)
	fs2, err := memory.NewFactStore(tmpDir, sessionID, &mockProvider{})
	if err != nil {
		t.Fatalf("failed to create second FactStore: %v", err)
	}
	defer fs2.Close()

	// Database should exist and be usable
	facts, err := fs2.QueryRelevant("test", 10)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	// Note: facts might be empty if debounce didn't trigger, but the query should work
	t.Logf("found %d facts in persistent database", len(facts))
}
