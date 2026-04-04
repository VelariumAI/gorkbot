package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
)

func TestCCIIntegration_NilGuards(t *testing.T) {
	orch := &Orchestrator{}

	if got := orch.BuildCCISystemContext("hello"); got != "" {
		t.Fatalf("expected empty CCI context when layer missing")
	}
	if got := orch.RunCCIDriftCheck(); got != "" {
		t.Fatalf("expected empty drift check when layer missing")
	}
	if got := orch.HandleCCIGap("orchestrator"); got != "" {
		t.Fatalf("expected empty gap message when layer missing")
	}
	if got := orch.GetCCIStatus(); !strings.Contains(strings.ToLower(got), "not initialized") {
		t.Fatalf("unexpected CCI status guard: %s", got)
	}

	ctx := context.Background()
	if gotCtx := orch.InjectCCIContextIntoRegistry(ctx); gotCtx != ctx {
		t.Fatalf("expected same context when CCI layer missing")
	}
	if got := orch.cciPrefixForSystemMessage("x"); got != "" {
		t.Fatalf("expected empty cci prefix when CCI missing")
	}
}

func TestCCIIntegration_InitAndCorePaths(t *testing.T) {
	cfg := t.TempDir()
	cwd := filepath.Join(cfg, "repo")
	if err := osMkdirAll(cwd); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	orch := &Orchestrator{
		Logger:              slog.Default(),
		ModeManager:         NewModeManager(),
		ConversationHistory: ai.NewConversationHistory(),
	}
	orch.InitCCI(cfg, cwd)
	if orch.CCI == nil {
		t.Fatalf("expected CCI layer after InitCCI")
	}

	status := orch.GetCCIStatus()
	if !strings.Contains(status, "CCI System Status") {
		t.Fatalf("unexpected CCI status output: %s", status)
	}

	ctxText := orch.BuildCCISystemContext("internal/tui update")
	if ctxText == "" {
		t.Fatalf("expected non-empty built CCI system context")
	}
	if !strings.Contains(ctxText, "CCI") && !strings.Contains(ctxText, "Hot") && !strings.Contains(ctxText, "Trigger") {
		t.Fatalf("expected CCI-ish context content, got: %s", ctxText)
	}

	drift := orch.RunCCIDriftCheck()
	// No strict assertion on content; just ensure it does not panic and is a valid string.
	_ = drift

	gapMsg := orch.HandleCCIGap("undocumented-subsystem")
	if !strings.Contains(gapMsg, "CCI Gap Detected") {
		t.Fatalf("expected gap message, got: %s", gapMsg)
	}
	if orch.ModeManager.Name() != "PLAN" {
		t.Fatalf("expected mode manager switched to PLAN, got %s", orch.ModeManager.Name())
	}
	msgs := orch.ConversationHistory.GetMessages()
	if len(msgs) == 0 || !strings.Contains(msgs[len(msgs)-1].Content, "PLAN mode activated") {
		t.Fatalf("expected CCI gap system message in history")
	}

	ctx := context.Background()
	if gotCtx := orch.InjectCCIContextIntoRegistry(ctx); gotCtx == ctx {
		t.Fatalf("expected context wrapping when CCI layer present")
	}

	prefix := orch.cciPrefixForSystemMessage("internal/engine orchestrator")
	if prefix == "" {
		t.Fatalf("expected non-empty cci prefix")
	}
}

// osMkdirAll keeps this test file self-contained and avoids extra imports in the assertion-heavy sections.
func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
