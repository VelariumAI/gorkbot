package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engproviders "github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/pkg/ai"
)

func TestRalphLoop_BasicFlow(t *testing.T) {
	cfg := DefaultRalphConfig()
	if !cfg.Enabled || cfg.MaxIterations != 3 || cfg.FailureThreshold != 2 {
		t.Fatalf("unexpected default ralph config: %+v", cfg)
	}

	r := NewRalphLoop(cfg)
	r.Begin()
	r.RecordFailure("bash", "exit 1")
	if r.ShouldTrigger() {
		t.Fatalf("should not trigger below threshold")
	}
	r.RecordFailure("write_file", "permission denied")
	if !r.ShouldTrigger() {
		t.Fatalf("expected trigger at threshold")
	}
	r.Commit()

	if r.IterationsUsed() != 1 || r.MaxIterations() != 3 {
		t.Fatalf("unexpected iteration counters")
	}
	prompt := r.BuildRetryPrompt("fix this")
	if !strings.Contains(prompt, "RALPH LOOP") || !strings.Contains(prompt, "ORIGINAL TASK") {
		t.Fatalf("unexpected retry prompt: %s", prompt)
	}

	summary := r.Summary()
	if !strings.Contains(summary, "ralph:iter=1/3") {
		t.Fatalf("unexpected summary: %s", summary)
	}

	r.Reset()
	if r.IterationsUsed() != 0 {
		t.Fatalf("expected reset to clear attempts")
	}
}

func TestRalphLoop_DisabledAndLimits(t *testing.T) {
	r := NewRalphLoop(RalphConfig{Enabled: false, MaxIterations: 1, FailureThreshold: 1})
	r.Begin()
	r.RecordFailure("bash", "err")
	if r.ShouldTrigger() {
		t.Fatalf("disabled ralph loop should never trigger")
	}
	if got := r.Summary(); got != "ralph:disabled" {
		t.Fatalf("unexpected disabled summary: %s", got)
	}

	r2 := NewRalphLoop(RalphConfig{Enabled: true, MaxIterations: 1, FailureThreshold: 1})
	r2.Begin()
	r2.RecordFailure("a", "e")
	r2.Commit()
	r2.Begin()
	r2.RecordFailure("b", "e")
	if r2.ShouldTrigger() {
		t.Fatalf("should not trigger once max iterations reached")
	}
}

func TestExecutePlanModeAndTrackTokens(t *testing.T) {
	if err := ExecutePlanMode(nil, nil, nil); err == nil {
		t.Fatalf("expected error for nil orchestrator")
	}

	orch := &Orchestrator{
		ContextMgr:          NewContextManager(1000, nil),
		ConversationHistory: ai.NewConversationHistory(),
	}
	var b strings.Builder
	b.WriteString(strings.Repeat("x", 20))

	err := ExecutePlanMode(orch, &b, func() error { return nil })
	if err != nil {
		t.Fatalf("unexpected ExecutePlanMode error: %v", err)
	}
	if b.Len() != 0 {
		t.Fatalf("expected planning buffer reset")
	}
	msgs := orch.ConversationHistory.GetMessages()
	if len(msgs) == 0 || !strings.Contains(msgs[len(msgs)-1].Content, "Planning phase completed") {
		t.Fatalf("expected planning completion system message")
	}

	var p strings.Builder
	p.WriteString("abc") // fewer than 4 chars => 1 token minimum
	used := orch.ContextMgr.TrackTokens(&p)
	if used != 1 {
		t.Fatalf("expected minimum token accounting of 1, got %d", used)
	}

	err = ExecutePlanMode(orch, &p, func() error { panic("boom") })
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("expected panic conversion to error, got %v", err)
	}
}

func TestModeManager_FullFlow(t *testing.T) {
	mm := NewModeManager()
	if mm.Current() != ModeNormal || mm.Name() != "NORMAL" {
		t.Fatalf("unexpected initial mode")
	}

	changed := false
	mm.SetOnChange(func(from, to ExecutionMode) {
		changed = true
	})
	mm.Set(ModePlan)
	if !changed || mm.Description() == "" {
		t.Fatalf("expected mode change callback")
	}

	allowed, auto := mm.IsToolAllowed("write_file")
	if allowed || auto {
		t.Fatalf("write_file should be blocked in plan mode")
	}
	if inj := mm.SystemPromptInjection(); !strings.Contains(inj, "PLAN") {
		t.Fatalf("expected plan mode system injection")
	}

	next := mm.Cycle() // PLAN -> AUTO
	if next != ModeAutoEdit {
		t.Fatalf("unexpected cycle result: %v", next)
	}
	allowed, auto = mm.IsToolAllowed("edit_file")
	if !allowed || !auto {
		t.Fatalf("edit_file should be auto-approved in auto mode")
	}
	if inj := mm.SystemPromptInjection(); !strings.Contains(inj, "AUTO-EDIT") {
		t.Fatalf("expected auto mode system injection")
	}

	mm.SetMode("NORMAL")
	if mm.Current() != ModeNormal {
		t.Fatalf("expected SetMode NORMAL")
	}
	if out := FormatModeChange(ModePlan, ModeNormal); !strings.Contains(out, "Mode:") {
		t.Fatalf("unexpected mode change format: %s", out)
	}
}

func TestCrystallizer_CheckAndForgeAndSave(t *testing.T) {
	tmp := t.TempDir()
	origWD, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp failed: %v", err)
	}
	defer os.Chdir(origWD)

	h := ai.NewConversationHistory()
	for i := 0; i < 6; i++ {
		h.AddAssistantMessage("bash run_bash echo test")
	}

	consultant := &taskTestProvider{
		name: "consultant",
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			return `{"name":"auto_tool","description":"auto forged","python_code":"print('ok')"}`, nil
		},
	}
	coord := engproviders.NewProviderCoordinator(nil, nil, consultant, nil, nil, nil)
	orch := &Orchestrator{ProviderCoord: coord, ConversationHistory: h, Logger: slog.Default()}
	cr := NewCrystallizer(orch)

	cr.CheckAndForge(context.Background())

	toolDir := filepath.Join(tmp, "plugins", "python", "auto_forged", "auto_tool")
	if _, err := os.Stat(filepath.Join(toolDir, "main.py")); err != nil {
		t.Fatalf("expected forged main.py to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(toolDir, "manifest.json")); err != nil {
		t.Fatalf("expected forged manifest.json to exist: %v", err)
	}

	msgs := orch.ConversationHistory.GetMessages()
	found := false
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "autonomously crystallized") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tool forge system message")
	}
}

func TestCrystallizer_GuardsAndInvalidJSON(t *testing.T) {
	orch := &Orchestrator{ConversationHistory: ai.NewConversationHistory(), Logger: slog.Default()}
	cr := NewCrystallizer(orch)
	cr.CheckAndForge(context.Background()) // no consultant, should no-op

	consultant := &taskTestProvider{
		name: "consultant",
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			return "not-json", nil
		},
	}
	coord := engproviders.NewProviderCoordinator(nil, nil, consultant, nil, nil, nil)
	orch2 := &Orchestrator{ProviderCoord: coord, ConversationHistory: ai.NewConversationHistory(), Logger: slog.Default()}
	cr2 := NewCrystallizer(orch2)
	cr2.ForgeNewTool(context.Background(), "bash echo hi") // should fail parse and no panic
}
