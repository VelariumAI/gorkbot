package engine

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	engproviders "github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/governance"
	"github.com/velariumai/gorkbot/pkg/registry"
)

func TestExecuteTaskWithToolsCorrectnessMissingClaimMapDowngrades(t *testing.T) {
	primary := &taskTestProvider{
		name: "test-primary",
		id:   registry.ProviderID("xai"),
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			return "The final answer.", nil
		},
	}
	coord := engproviders.NewProviderCoordinator(nil, primary, nil, nil, nil, slog.Default())
	p := governance.DefaultPolicy()
	p.Mode = governance.GOVERNANCE_CORRECTNESS
	orch := &Orchestrator{
		ProviderCoord:       coord,
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
		Governor:            &governance.Governor{Policy: p},
	}

	out, err := orch.ExecuteTaskWithTools(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("ExecuteTaskWithTools failed: %v", err)
	}
	if !strings.Contains(out, "[UNVERIFIED — renderer guard did not validate this answer]") {
		t.Fatalf("expected unverified prefix, got: %q", out)
	}
	if !strings.Contains(out, governance.MISSING_CLAIM_REFS) {
		t.Fatalf("expected missing claim refs reason, got: %q", out)
	}
}

func TestExecuteTaskWithToolsOffModeUnchanged(t *testing.T) {
	primary := &taskTestProvider{
		name: "test-primary",
		id:   registry.ProviderID("xai"),
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			return "The final answer.", nil
		},
	}
	coord := engproviders.NewProviderCoordinator(nil, primary, nil, nil, nil, slog.Default())
	p := governance.DefaultPolicy()
	p.Mode = governance.GOVERNANCE_OFF
	orch := &Orchestrator{
		ProviderCoord:       coord,
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
		Governor:            &governance.Governor{Policy: p},
	}

	out, err := orch.ExecuteTaskWithTools(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("ExecuteTaskWithTools failed: %v", err)
	}
	if out != "The final answer." {
		t.Fatalf("off mode should remain unchanged, got: %q", out)
	}
}
