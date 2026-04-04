package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	engproviders "github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

type taskTestProvider struct {
	name      string
	id        registry.ProviderID
	generateH func(ctx context.Context, history *ai.ConversationHistory) (string, error)
}

func (p *taskTestProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return p.GenerateWithHistory(ctx, ai.NewConversationHistory())
}
func (p *taskTestProvider) GenerateWithHistory(ctx context.Context, history *ai.ConversationHistory) (string, error) {
	if p.generateH != nil {
		return p.generateH(ctx, history)
	}
	return "ok", nil
}
func (p *taskTestProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	return nil
}
func (p *taskTestProvider) StreamWithHistory(ctx context.Context, history *ai.ConversationHistory, out io.Writer) error {
	return nil
}
func (p *taskTestProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{ID: string(p.id), ContextSize: 128000}
}
func (p *taskTestProvider) Name() string { return p.name }
func (p *taskTestProvider) ID() registry.ProviderID {
	if p.id == "" {
		return registry.ProviderID("test")
	}
	return p.id
}
func (p *taskTestProvider) Ping(ctx context.Context) error { return nil }
func (p *taskTestProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return []registry.ModelDefinition{{ID: "m1", Name: "m1"}}, nil
}
func (p *taskTestProvider) WithModel(model string) ai.AIProvider { return p }

func TestExecuteTaskWithTools_NoPrimaryProvider(t *testing.T) {
	orch := &Orchestrator{
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}

	_, err := orch.ExecuteTaskWithTools(context.Background(), "hello", nil)
	if err == nil {
		t.Fatal("expected error when no primary provider is configured")
	}
	if !strings.Contains(err.Error(), "no primary provider available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteTaskWithHistory_NilHistoryDelegates(t *testing.T) {
	orch := &Orchestrator{
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}
	_, err := orch.ExecuteTaskWithHistory(context.Background(), "hello", nil)
	if err == nil {
		t.Fatal("expected no-primary-provider error")
	}
	if !strings.Contains(err.Error(), "no primary provider available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteTaskWithHistory_RestoresOriginalHistory(t *testing.T) {
	primary := &taskTestProvider{
		name: "test-primary",
		id:   registry.ProviderID("xai"),
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			return "final answer", nil
		},
	}
	coord := engproviders.NewProviderCoordinator(nil, primary, nil, nil, nil, slog.Default())
	origHistory := ai.NewConversationHistory()
	sessionHistory := ai.NewConversationHistory()

	orch := &Orchestrator{
		ProviderCoord:       coord,
		ConversationHistory: origHistory,
		Logger:              slog.Default(),
	}

	out, err := orch.ExecuteTaskWithHistory(context.Background(), "hello", sessionHistory)
	if err != nil {
		t.Fatalf("ExecuteTaskWithHistory failed: %v", err)
	}
	if out != "final answer" {
		t.Fatalf("unexpected output: %q", out)
	}
	if orch.ConversationHistory != origHistory {
		t.Fatalf("orchestrator history was not restored")
	}
	if origHistory.Count() != 0 {
		t.Fatalf("original history should remain untouched, got %d", origHistory.Count())
	}
	if sessionHistory.Count() == 0 {
		t.Fatalf("session history should have been used and updated")
	}
}

func TestExecuteTaskWithTools_PropagatesProviderCancellation(t *testing.T) {
	primary := &taskTestProvider{
		name: "test-primary",
		id:   registry.ProviderID("xai"),
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	}
	coord := engproviders.NewProviderCoordinator(nil, primary, nil, nil, nil, slog.Default())
	orch := &Orchestrator{
		ProviderCoord:       coord,
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := orch.ExecuteTaskWithTools(ctx, "hello", nil)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}
