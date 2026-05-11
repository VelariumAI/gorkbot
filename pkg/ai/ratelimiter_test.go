package ai

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/velariumai/gorkbot/pkg/registry"
)

type mockAIProvider struct{}

func (m *mockAIProvider) Generate(context.Context, string) (string, error) { return "ok", nil }
func (m *mockAIProvider) GenerateWithHistory(context.Context, *ConversationHistory) (string, error) {
	return "ok-h", nil
}
func (m *mockAIProvider) Stream(context.Context, string, io.Writer) error { return nil }
func (m *mockAIProvider) StreamWithHistory(context.Context, *ConversationHistory, io.Writer) error {
	return nil
}
func (m *mockAIProvider) GetMetadata() ProviderMetadata {
	return ProviderMetadata{ID: "m", ContextSize: 1}
}
func (m *mockAIProvider) Name() string               { return "mock" }
func (m *mockAIProvider) ID() registry.ProviderID    { return "mock" }
func (m *mockAIProvider) Ping(context.Context) error { return nil }
func (m *mockAIProvider) FetchModels(context.Context) ([]registry.ModelDefinition, error) {
	return []registry.ModelDefinition{{ID: "m"}}, nil
}
func (m *mockAIProvider) WithModel(string) AIProvider { return &mockAIProvider{} }

func TestRateLimitedProviderDelegation(t *testing.T) {
	rp := NewRateLimitedProvider(&mockAIProvider{}, 60000) // effectively non-blocking

	if _, err := rp.Generate(context.Background(), "p"); err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	h := NewConversationHistory()
	if _, err := rp.GenerateWithHistory(context.Background(), h); err != nil {
		t.Fatalf("generate with history failed: %v", err)
	}
	var out bytes.Buffer
	if err := rp.Stream(context.Background(), "p", &out); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if err := rp.StreamWithHistory(context.Background(), h, &out); err != nil {
		t.Fatalf("stream with history failed: %v", err)
	}
	if rp.GetMetadata().ID != "m" || rp.Name() != "mock" || rp.ID() != "mock" {
		t.Fatalf("unexpected metadata/name/id delegation")
	}
	if err := rp.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if models, err := rp.FetchModels(context.Background()); err != nil || len(models) != 1 {
		t.Fatalf("fetch models delegation failed, models=%d err=%v", len(models), err)
	}
	if _, ok := rp.WithModel("x").(*RateLimitedProvider); !ok {
		t.Fatalf("expected WithModel to return RateLimitedProvider")
	}
}

func TestNewFetchClientShape(t *testing.T) {
	c := NewFetchClient()
	if c.Timeout <= 0 {
		t.Fatalf("expected positive timeout")
	}
	if c.Transport == nil {
		t.Fatalf("expected transport configured")
	}
}
