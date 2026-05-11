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

func TestRenderGuardStreamingNoticeOffFastEnforceNoSuffixByDefault(t *testing.T) {
	modes := []governance.Mode{
		governance.GOVERNANCE_OFF,
		governance.GOVERNANCE_FAST,
		governance.GOVERNANCE_ENFORCE,
	}

	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			p := governance.DefaultPolicy()
			p.Mode = mode
			orch := &Orchestrator{
				Logger:   slog.Default(),
				Governor: &governance.Governor{Policy: p},
			}

			var streamed strings.Builder
			orch.renderGuardStreamingNotice(context.Background(), "The final answer.", nil, func(token string) {
				streamed.WriteString(token)
			})

			got := streamed.String()
			if got != "" {
				t.Fatalf("expected no streaming suffix for %s, got: %q", mode, got)
			}
			if strings.Contains(got, "[VERIFIED") || strings.Contains(got, "[UNVERIFIED") {
				t.Fatalf("unexpected guard marker for %s: %q", mode, got)
			}

			d := orch.LastRenderGuardDecision()
			if d == nil {
				t.Fatalf("expected decision to be recorded for %s", mode)
			}
			if d.ReasonCode != governance.RENDER_GUARD_SKIPPED_NOT_CORRECTNESS_MODE {
				t.Fatalf("expected skip reason for %s, got: %#v", mode, d)
			}
		})
	}
}

func TestRenderGuardStreamingNoticeCorrectnessMissingClaimMapAppendsUnverified(t *testing.T) {
	p := governance.DefaultPolicy()
	p.Mode = governance.GOVERNANCE_CORRECTNESS
	orch := &Orchestrator{
		Logger:   slog.Default(),
		Governor: &governance.Governor{Policy: p},
	}

	var streamed strings.Builder
	orch.renderGuardStreamingNotice(context.Background(), "The final answer.", nil, func(token string) {
		streamed.WriteString(token)
	})

	got := streamed.String()
	if !strings.Contains(got, "[UNVERIFIED") {
		t.Fatalf("expected unverified marker in streaming notice, got: %q", got)
	}
	if !strings.Contains(got, governance.MISSING_CLAIM_REFS) {
		t.Fatalf("expected missing-claim-refs reason in notice, got: %q", got)
	}
}
