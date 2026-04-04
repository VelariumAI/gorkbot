package engine

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	engproviders "github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/sense"
)

func TestFormatKAndTruncate(t *testing.T) {
	if got := formatK(999); got != "999" {
		t.Fatalf("unexpected formatK output: %q", got)
	}
	if got := formatK(1500); got != "1.5k" {
		t.Fatalf("unexpected formatK compact output: %q", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("unexpected truncate no-op output: %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Fatalf("unexpected truncate output: %q", got)
	}
}

func TestOrchestrator_ClearAndGetHistory(t *testing.T) {
	h := ai.NewConversationHistory()
	h.AddUserMessage("hello")
	orch := &Orchestrator{ConversationHistory: h, Logger: slog.Default()}

	if orch.GetHistory() != h {
		t.Fatalf("expected GetHistory to return same history pointer")
	}
	orch.ClearHistory()
	if h.Count() != 0 {
		t.Fatalf("expected history to be cleared")
	}
}

func TestOrchestrator_CompactWithFocus_Guards(t *testing.T) {
	orch := &Orchestrator{}
	if got := orch.CompactWithFocus(context.Background(), "x"); !strings.Contains(got, "Compressor not available") {
		t.Fatalf("unexpected compact guard message: %s", got)
	}

	orch2 := &Orchestrator{Compressor: sense.NewCompressor(&compressorGenStub{})}
	if got := orch2.CompactWithFocus(context.Background(), "x"); !strings.Contains(got, "No conversation history") {
		t.Fatalf("unexpected nil-history guard message: %s", got)
	}
}

func TestOrchestrator_InitSENSEMemory(t *testing.T) {
	orch := &Orchestrator{}
	if err := orch.InitSENSEMemory(t.TempDir()); err != nil {
		t.Fatalf("InitSENSEMemory failed: %v", err)
	}
	if orch.AgeMem == nil || orch.Engrams == nil {
		t.Fatalf("expected AgeMem and Engrams to be initialized")
	}
}

func TestOrchestrator_SpawnCollectListFromTool(t *testing.T) {
	provider := &taskTestProvider{
		name: "primary",
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			return "bg-ok", nil
		},
	}
	coord := engproviders.NewProviderCoordinator(nil, provider, nil, nil, nil, nil)
	orch := &Orchestrator{
		ProviderCoord:    coord,
		BackgroundAgents: NewBackgroundAgentManager(1, "", nil),
	}

	id, err := orch.SpawnFromTool(context.Background(), "worker", "task", "")
	if err != nil {
		t.Fatalf("SpawnFromTool failed: %v", err)
	}
	if id == "" {
		t.Fatalf("expected non-empty background agent id")
	}

	out, err := orch.CollectFromTool(context.Background(), id, 2)
	if err != nil {
		t.Fatalf("CollectFromTool failed: %v", err)
	}
	if out != "bg-ok" {
		t.Fatalf("unexpected background output: %q", out)
	}

	info := orch.ListRunningFromTool()
	if len(info) == 0 {
		t.Fatalf("expected agent info from ListRunningFromTool")
	}
}

func TestOrchestrator_BackgroundToolErrors(t *testing.T) {
	orch := &Orchestrator{}
	if _, err := orch.SpawnFromTool(context.Background(), "x", "y", ""); err == nil {
		t.Fatalf("expected spawn error when manager missing")
	}
	if _, err := orch.CollectFromTool(context.Background(), "id", 1); err == nil {
		t.Fatalf("expected collect error when manager missing")
	}
	if got := orch.ListRunningFromTool(); got != nil {
		t.Fatalf("expected nil list when manager missing")
	}

	orch2 := &Orchestrator{BackgroundAgents: NewBackgroundAgentManager(1, "", nil)}
	if _, err := orch2.SpawnFromTool(context.Background(), "x", "y", ""); err == nil || !strings.Contains(err.Error(), "no primary") {
		t.Fatalf("expected no-primary error, got %v", err)
	}

	orch3 := &Orchestrator{BackgroundAgents: NewBackgroundAgentManager(1, "", nil)}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := orch3.CollectFromTool(ctx, "missing", 0); err == nil {
		t.Fatalf("expected collect error for missing id")
	}
}

func TestOrchestrator_SampleOnce(t *testing.T) {
	orch := &Orchestrator{}
	if _, err := orch.SampleOnce(context.Background(), "", "", "hi"); err == nil {
		t.Fatalf("expected error when no primary provider configured")
	}

	provider := &taskTestProvider{
		name: "primary",
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			msgs := history.GetMessages()
			if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
				t.Fatalf("unexpected history passed to SampleOnce: %+v", msgs)
			}
			return "sample-response", nil
		},
	}
	coord := engproviders.NewProviderCoordinator(nil, provider, nil, nil, nil, nil)
	orch.ProviderCoord = coord

	out, err := orch.SampleOnce(context.Background(), "model-x", "sys", "hi")
	if err != nil {
		t.Fatalf("SampleOnce failed: %v", err)
	}
	if out != "sample-response" {
		t.Fatalf("unexpected sample output: %q", out)
	}
}
