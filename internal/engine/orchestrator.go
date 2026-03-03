package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/internal/arc"
	"github.com/velariumai/gorkbot/internal/mel"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/billing"
	"github.com/velariumai/gorkbot/pkg/cci"
	"github.com/velariumai/gorkbot/pkg/collab"
	"github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/discovery"
	"github.com/velariumai/gorkbot/pkg/hooks"
	"github.com/velariumai/gorkbot/pkg/memory"
	"github.com/velariumai/gorkbot/pkg/router"
	"github.com/velariumai/gorkbot/pkg/subagents"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/session"
	"github.com/velariumai/gorkbot/pkg/tools"
	"github.com/velariumai/gorkbot/pkg/tui"
)

// Orchestrator manages the interaction between the user and the AI providers.
type Orchestrator struct {
	Primary            ai.AIProvider
	Consultant         ai.AIProvider
	Registry           *tools.Registry
	Logger             *slog.Logger
	EnableWatchdog     bool
	ConversationHistory *ai.ConversationHistory
	Stylist             *tui.Stylist

	// ── SENSE components ────────────────────────────────────────────────────
	LIE        *sense.LIEEvaluator
	Stabilizer *sense.Stabilizer
	AgeMem     *sense.AgeMem
	Engrams    *sense.EngramStore
	Compressor *sense.Compressor
	HITLGuard  *HITLGuard
	HITLCallback HITLCallback
	HAL        platform.HALProfile

	// ── Enhanced systems (P0/P1/P2) ─────────────────────────────────────────
	// ContextMgr tracks token usage and triggers auto-compaction.
	ContextMgr *ContextManager
	// Billing tracks per-model token costs for the session.
	Billing *billing.BillingManager
	// ModeManager controls execution mode (Normal/Plan/AutoEdit).
	ModeManager *ModeManager
	// Checkpoints saves conversation state before each tool execution.
	Checkpoints *session.CheckpointManager
	// Hooks fires lifecycle event scripts.
	Hooks *hooks.Manager
	// ConfigLoader loads GORKBOT.md project instructions.
	ConfigLoader *config.Loader
	// RuleEngine evaluates fine-grained permission rules.
	RuleEngine *tools.RuleEngine
	// ToolCache provides TTL-based memoization for read-only tools.
	ToolCache *tools.ToolCache
	// Dispatcher runs independent tool batches concurrently.
	Dispatcher *tools.Dispatcher
	// Exporter handles conversation export.
	Exporter *session.Exporter

	// cancelMu guards cancelFunc.
	cancelMu   sync.Mutex
	cancelFunc context.CancelFunc // Set during generation; Interrupt() calls it.

	// primaryModelName is stored for cost/context reports.
	primaryModelName string

	// Tracer writes a newline-delimited JSON execution trace (--trace flag).
	Tracer *TraceLogger

	// Relay broadcasts streaming tokens + tool events to remote observers (--share flag).
	// nil when session sharing is disabled.
	Relay *collab.Relay

	// Feedback records routing outcomes and seeds the AdaptiveRouter.
	Feedback *router.FeedbackManager

	// Discovery polls xAI and Gemini for live model lists; used by the
	// Cloud Brains TUI tab and spawn_sub_agent depth-aware routing.
	Discovery *discovery.Manager

	// Intelligence bundles the ARC Router and MEL learning system.
	// Initialized after construction via InitIntelligence().
	Intelligence *IntelligenceLayer

	// DebugMode — when true, raw AI responses (including tool JSON blocks) are
	// returned to the TUI unstripped. Toggle via /debug command.
	DebugMode bool

	// ── New systems (oh-my-opencode / termui inspired) ──────────────────────

	// RalphLoop provides self-referential retry when the AI gets stuck.
	// Enabled by default; disable with RalphLoop.cfg.Enabled = false.
	RalphLoop *RalphLoop

	// ContextInjector auto-injects GORKBOT.md hierarchy + README + rules
	// into the conversation at session start (three-tier context injection).
	ContextInjector *ContextInjector

	// Workspace manages git-based rollbacks for file-modifying tools.
	Workspace *session.WorkspaceManager

	// BackgroundAgents manages parallel sub-agent execution.
	BackgroundAgents *BackgroundAgentManager

	// Crystallizer monitors and autonomously creates python tools.
	Crystallizer *Crystallizer

	// GoalLedger persists cross-session goals (prospective memory).
	GoalLedger *memory.GoalLedger

	// UnifiedMem wraps AgeMem+Engrams+MEL under one query interface.
	UnifiedMem *memory.UnifiedMemory

	// SecurityCtx is the session-scoped shared state for red team agents.
	// Nil when not in a security assessment session.
	SecurityCtx *subagents.SecurityContext

	// CCI is the Codified Context Infrastructure layer — three-tier persistent
	// project memory (Hot / Specialist / Cold) with Truth Sentry drift detection.
	// Initialized by InitEnhancements() → InitCCI().
	CCI *cci.CCILayer

	// configDir is set via SetConfigDir; used by writeMemoryLog.
	configDir string
}

// NewOrchestrator initializes the orchestration engine with SENSE components.
func NewOrchestrator(primary, consultant ai.AIProvider, registry *tools.Registry, logger *slog.Logger, enableWatchdog bool) *Orchestrator {
	o := &Orchestrator{
		Primary:             primary,
		Consultant:          consultant,
		Registry:            registry,
		Logger:              logger,
		EnableWatchdog:      enableWatchdog,
		ConversationHistory: ai.NewConversationHistory(),
		Stylist:             tui.NewStylist(),
		LIE:                 sense.NewLIEEvaluator(),
		HITLGuard:           NewHITLGuard(),
		HAL:                 platform.ProbeHAL(logger),
		// Enhanced systems
		ModeManager: NewModeManager(),
		Checkpoints: session.NewCheckpointManager(20, ""),
		ToolCache:   tools.NewToolCache(),
		Exporter:    session.NewExporter(),
	}

	// Stabilizer uses Consultant for task-alignment scoring when available.
	var stabGen sense.TextGenerator
	if consultant != nil {
		stabGen = consultant
	}
	o.Stabilizer = sense.NewStabilizer(stabGen)

	// Compressor uses Consultant to drive the 4-stage pipeline.
	if consultant != nil {
		o.Compressor = sense.NewCompressor(consultant)
	}

	// Dispatcher for concurrent tool execution.
	if registry != nil {
		o.Dispatcher = tools.NewDispatcher(registry, 8)
	}

	// Ralph Loop — self-referential retry (oh-my-opencode inspired).
	o.RalphLoop = NewRalphLoop(DefaultRalphConfig())

	// Three-tier context injector (oh-my-opencode inspired).
	o.ContextInjector = NewContextInjector()

	// Background agent manager (oh-my-opencode inspired, 4 max concurrent).
	o.BackgroundAgents = NewBackgroundAgentManager(4, "", nil)

	// Context manager — fires auto-compaction at 90% full.
	o.ContextMgr = NewContextManager(131072, func() {
		logger.Info("Context window 90%+ full — triggering auto-compaction")
		// Compaction runs asynchronously to avoid blocking.
		go func() {
			if o.Compressor != nil && o.ConversationHistory != nil {
				ctx := context.Background()
				msgs := o.ConversationHistory.GetMessages()
				senseMsgs := make([]sense.ConversationMessage, 0, len(msgs))
				for _, m := range msgs {
					senseMsgs = append(senseMsgs, sense.ConversationMessage{
						Role:    m.Role,
						Content: m.Content,
					})
				}
				if _, err := o.Compressor.Compress(ctx, senseMsgs); err != nil {
					logger.Warn("Auto-compaction failed", "error", err)
				} else {
					logger.Info("Auto-compaction complete")
				}
			}
		}()
	})

	cwd, _ := os.Getwd()
	o.Workspace = session.NewWorkspaceManager(cwd)
	
	o.Crystallizer = NewCrystallizer(o)

	if primary != nil {
		o.primaryModelName = primary.Name()
	}

	return o
}

// InitEnhancements wires up config-dir-dependent systems.
// Call from main.go after InitSENSEMemory.
func (o *Orchestrator) InitEnhancements(configDir, cwd string) {
	o.Hooks = hooks.NewManager(configDir, o.Logger)
	o.ConfigLoader = config.NewLoader(configDir, cwd)
	o.RuleEngine = tools.NewRuleEngine(configDir)
	// Store checkpoints in configDir/checkpoints/
	o.Checkpoints = session.NewCheckpointManager(20, configDir+"/checkpoints")

	// Initialize CCI — three-tier project memory (Hot/Specialist/Cold).
	o.InitCCI(configDir, cwd)

	o.Logger.Info("Enhanced systems initialized",
		"hooks_dir", o.Hooks.HooksDir(),
		"config_files", o.ConfigLoader.ActiveFiles())
}

// InitIntelligence initializes the ARC Router + MEL learning stack.
// Call from main.go after InitEnhancements so configDir is set.
// Fails silently if initialization fails — the system runs without it.
func (o *Orchestrator) InitIntelligence(configDir string) {
	il, err := NewIntelligenceLayer(o.HAL, configDir)
	if err != nil {
		o.Logger.Warn("Intelligence layer init failed (running without it)", "error", err)
		return
	}
	o.Intelligence = il
	o.Logger.Info("Intelligence layer ready",
		"platform", il.Router.PlatformName(),
		"mel_heuristics", il.Store.Len())
}

// SetTraceLogger attaches a trace logger (enabled via --trace flag).
func (o *Orchestrator) SetTraceLogger(tl *TraceLogger) {
	o.Tracer = tl
}

// SetConfigDir stores the config directory for daily memory log writes.
func (o *Orchestrator) SetConfigDir(dir string) {
	o.configDir = dir
}

// writeMemoryLog writes a daily compaction summary to ~/.config/gorkbot/memory/YYYY-MM-DD.md
func (o *Orchestrator) writeMemoryLog(summary string) {
	if o.configDir == "" {
		return
	}
	dir := filepath.Join(o.configDir, "memory")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	date := time.Now().Format("2006-01-02")
	path := filepath.Join(dir, date+".md")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "\n## Compaction at %s\n\n%s\n", time.Now().Format(time.RFC3339), summary)
}

// ToggleDebug flips the DebugMode flag and returns the new value.
// When DebugMode is true, raw AI responses (including tool JSON blocks) are
// sent to the TUI without stripping.
func (o *Orchestrator) ToggleDebug() bool {
	o.DebugMode = !o.DebugMode
	o.Logger.Info("Debug mode toggled", "debug_mode", o.DebugMode)
	return o.DebugMode
}

// Interrupt cancels the current in-progress generation without crashing the app.
func (o *Orchestrator) Interrupt() {
	o.cancelMu.Lock()
	fn := o.cancelFunc
	o.cancelMu.Unlock()
	if fn != nil {
		fn()
	}
}

// GetMode returns the current execution mode name (e.g. "NORMAL", "PLAN").
func (o *Orchestrator) GetMode() string {
	if o.ModeManager == nil {
		return "NORMAL"
	}
	return o.ModeManager.Name()
}

// CycleMode advances through Normal → Plan → AutoEdit → Normal and returns
// the new mode name.
func (o *Orchestrator) CycleMode() string {
	if o.ModeManager == nil {
		return "NORMAL"
	}
	next := o.ModeManager.Cycle()
	name := o.ModeManager.Name()
	if o.Hooks != nil {
		o.Hooks.FireAsync(context.Background(), hooks.EventModeChange, hooks.Payload{
			Mode: name,
		})
	}
	_ = next
	return name
}

// SetMode sets the execution mode by name ("normal", "plan", "auto").
func (o *Orchestrator) SetMode(name string) string {
	if o.ModeManager == nil {
		return "NORMAL"
	}
	switch strings.ToLower(name) {
	case "plan":
		o.ModeManager.Set(ModePlan)
	case "auto", "autoedit":
		o.ModeManager.Set(ModeAutoEdit)
	default:
		o.ModeManager.Set(ModeNormal)
	}
	return o.ModeManager.Name()
}

// ContextStatusBar returns a compact context/cost string for the status bar.
func (o *Orchestrator) ContextStatusBar() string {
	if o.ContextMgr == nil {
		return ""
	}
	return o.ContextMgr.StatusBar()
}

// GetContextReport returns a detailed /context breakdown report.
func (o *Orchestrator) GetContextReport() string {
	if o.ContextMgr == nil {
		return "Context tracking not initialized."
	}
	// Estimate breakdown from conversation history
	systemT, convT, toolT := 0, 0, 0
	if o.ConversationHistory != nil {
		for _, m := range o.ConversationHistory.GetMessages() {
			est := len(m.Content) / 4
			switch m.Role {
			case "system":
				systemT += est
			case "user":
				if strings.Contains(m.Content, "<tool_result") {
					toolT += est
				} else {
					convT += est
				}
			case "assistant":
				convT += est
			}
		}
	}
	return o.ContextMgr.ContextBreakdown(systemT, convT, toolT, 0)
}

// GetCostReport returns a /cost summary.
func (o *Orchestrator) GetCostReport() string {
	if o.ContextMgr == nil {
		return "Cost tracking not initialized."
	}
	consultant := ""
	if o.Consultant != nil {
		consultant = o.Consultant.Name()
	}
	return o.ContextMgr.CostReport(o.primaryModelName, consultant)
}

// GetCheckpointList returns formatted checkpoint list for /rewind.
func (o *Orchestrator) GetCheckpointList() string {
	if o.Checkpoints == nil {
		return "Checkpoint system not initialized."
	}
	return o.Checkpoints.Format()
}

// RewindTo restores conversation to checkpoint id (or "last").
// Returns a "REWIND_COMPLETE:<id>:<count>" signal on success (parsed by the TUI)
// or a plain error string on failure.
func (o *Orchestrator) RewindTo(id string) string {
	if o.Checkpoints == nil {
		return "Checkpoint system not initialized."
	}
	cp, err := o.Checkpoints.Rewind(id, o.ConversationHistory)
	if err != nil {
		return fmt.Sprintf("Rewind failed: %v", err)
	}

	if o.Workspace != nil && cp.WorkspaceHash != "" {
		if err := o.Workspace.RestoreCheckpoint(cp.WorkspaceHash); err != nil {
			o.Logger.Warn("Workspace rollback failed during rewind", "error", err)
			return fmt.Sprintf("REWIND_COMPLETE:%s:%d (Note: workspace rollback failed: %v)", cp.ID, len(cp.Messages), err)
		}
	}

	return fmt.Sprintf("REWIND_COMPLETE:%s:%d", cp.ID, len(cp.Messages))
}

// ExportConversation exports the conversation in the given format.
func (o *Orchestrator) ExportConversation(format, path string) string {
	if o.Exporter == nil || o.ConversationHistory == nil {
		return "Export system not initialized."
	}
	var ef session.ExportFormat
	switch strings.ToLower(format) {
	case "json":
		ef = session.ExportJSON
	case "plain", "text":
		ef = session.ExportPlain
	default:
		ef = session.ExportMarkdown
	}
	result, err := o.Exporter.Export(o.ConversationHistory, ef, path)
	if err != nil {
		return fmt.Sprintf("Export failed: %v", err)
	}
	return result
}

// SaveSession writes the conversation history to a named JSON file under configDir/sessions/.
// Returns a status message suitable for display in the TUI.
func (o *Orchestrator) SaveSession(name string) string {
	if o.ConversationHistory == nil {
		return "No conversation history to save."
	}
	env, err := platform.GetEnvConfig()
	if err != nil {
		return fmt.Sprintf("Failed to get environment config: %v", err)
	}
	if env == nil {
		return "Environment config is nil"
	}
	sessionDir := filepath.Join(env.ConfigDir, "sessions")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Sprintf("Cannot create sessions directory: %v", err)
	}
	path := filepath.Join(sessionDir, name+".json")
	msgs := o.ConversationHistory.GetMessages()
	if err := session.SaveSessionFile(path, name, msgs); err != nil {
		return fmt.Sprintf("Save failed: %v", err)
	}
	return fmt.Sprintf("Session '%s' saved (%d messages).", name, len(msgs))
}

// LoadSession imports a named session file and replaces the current conversation history.
// Returns "SESSION_LOADED:<name>:<count>" on success (parsed by the TUI), or an error string.
func (o *Orchestrator) LoadSession(name string) string {
	if o.ConversationHistory == nil {
		return "Conversation history not available."
	}
	env, err := platform.GetEnvConfig()
	if err != nil {
		return fmt.Sprintf("Failed to get environment config: %v", err)
	}
	if env == nil {
		return "Environment config is nil"
	}
	path := filepath.Join(env.ConfigDir, "sessions", name+".json")
	msgs, err := session.LoadSessionFile(path)
	if err != nil {
		return fmt.Sprintf("Load failed: %v", err)
	}
	o.ConversationHistory.Clear()
	for _, m := range msgs {
		o.ConversationHistory.AddMessage(m.Role, m.Content)
	}
	return fmt.Sprintf("SESSION_LOADED:%s:%d", name, len(msgs))
}

// ListSessions returns a formatted list of saved session names.
func (o *Orchestrator) ListSessions() string {
	env, err := platform.GetEnvConfig()
	if err != nil {
		return fmt.Sprintf("Failed to get environment config: %v", err)
	}
	if env == nil {
		return "Environment config is nil"
	}
	names := session.ListSessionFiles(filepath.Join(env.ConfigDir, "sessions"))
	if len(names) == 0 {
		return "No saved sessions."
	}
	return "Saved sessions:\n• " + strings.Join(names, "\n• ")
}

// CompactWithFocus runs SENSE compression with an optional focus hint.
func (o *Orchestrator) CompactWithFocus(ctx context.Context, focus string) string {
	if o.Compressor == nil {
		return "Compressor not available (Gemini API key required)."
	}
	if o.ConversationHistory == nil {
		return "No conversation history to compress."
	}
	beforeTokens := o.ConversationHistory.EstimateTokens()
	msgs := o.ConversationHistory.GetMessages()
	senseMsgs := make([]sense.ConversationMessage, 0, len(msgs))
	for _, m := range msgs {
		senseMsgs = append(senseMsgs, sense.ConversationMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	snapshot, err := o.Compressor.Compress(ctx, senseMsgs)
	if err != nil {
		return fmt.Sprintf("Compression failed: %v", err)
	}
	// Replace history with compressed snapshot
	o.ConversationHistory.Clear()
	summary := snapshot.Summary
	if focus != "" {
		summary = "[Focus: " + focus + "]\n" + summary
	}
	o.ConversationHistory.AddSystemMessage("## Compressed Context\n" + summary)
	afterTokens := o.ConversationHistory.EstimateTokens()
	pct := 0
	if beforeTokens > 0 {
		pct = (beforeTokens - afterTokens) * 100 / beforeTokens
	}
	result := fmt.Sprintf("Context compressed: %s → %s tokens (-%d%%)",
		formatK(beforeTokens), formatK(afterTokens), pct)
	o.writeMemoryLog(result + "\n\n" + summary)
	return result
}

func formatK(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// InitSENSEMemory attaches persistent AgeMem to the orchestrator.
// Called from main.go after the config directory is known.
func (o *Orchestrator) InitSENSEMemory(dataDir string) error {
	am, err := sense.NewAgeMem(dataDir, 8000)
	if err != nil {
		return fmt.Errorf("sense agemem: %w", err)
	}
	o.AgeMem = am
	o.Engrams = sense.NewEngramStore(am)
	return nil
}

// ClearHistory clears the conversation history
func (o *Orchestrator) ClearHistory() {
	if o.ConversationHistory != nil {
		o.ConversationHistory.Clear()
		o.Logger.Info("Conversation history cleared")
	}
}

// GetHistory returns the conversation history
func (o *Orchestrator) GetHistory() *ai.ConversationHistory {
	return o.ConversationHistory
}

func (o *Orchestrator) printWatchdogState(stage string, prompt string) {
	fmt.Fprintf(os.Stderr, "\n[WATCHDOG] Stage: %s\n", stage)
	fmt.Fprintf(os.Stderr, "[WATCHDOG] Primary Provider: %s\n", o.Primary.Name())
	if o.Consultant != nil {
		fmt.Fprintf(os.Stderr, "[WATCHDOG] Consultant Provider: %s\n", o.Consultant.Name())
	}
	fmt.Fprintf(os.Stderr, "[WATCHDOG] Prompt Length: %d\n", len(prompt))
	fmt.Fprintf(os.Stderr, "[WATCHDOG] Prompt Preview: %.50s...\n\n", prompt)
}

// AnalyzeAgency performs a layered reflection cycle that combines:
//  1. SENSE Stabilizer — scores response quality on four dimensions and
//     prescribes corrections when quality is low.
//  2. SENSE LIE — evaluates reasoning depth/diversity and injects feedback
//     when responses are too short or repetitive.
//  3. Legacy Consultant check — retained for backward compatibility and for
//     cases where the Stabilizer escalates to a full consultation.
func (o *Orchestrator) AnalyzeAgency(ctx context.Context, lastResponse string) (bool, error) {
	o.Logger.Info("Running SENSE Agency Reflection...")
	injected := false

	// ── 1. LIE Evaluation ────────────────────────────────────────────────────
	if o.LIE != nil {
		metrics, inject := o.LIE.Evaluate(lastResponse)
		o.Logger.Debug("LIE metrics",
			"reward", metrics.FinalReward,
			"redundancy", metrics.RedundancyPenalty,
			"length_score", metrics.LengthScore,
		)
		if inject {
			msg := o.LIE.FormatSystemMessage(metrics.Feedback)
			o.ConversationHistory.AddSystemMessage(msg)
			o.Logger.Info("LIE feedback injected", "feedback", metrics.Feedback)
			injected = true
		}
	}

	// ── 2. Stabilizer Quality Score ──────────────────────────────────────────
	if o.Stabilizer != nil {
		// Retrieve the most recent user prompt for alignment scoring.
		lastUserPrompt := ""
		msgs := o.ConversationHistory.GetMessages()
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" {
				lastUserPrompt = msgs[i].Content
				break
			}
		}
		qs := o.Stabilizer.Evaluate(ctx, lastUserPrompt, lastResponse)
		o.Logger.Debug("Stabilizer score",
			"overall", qs.Overall,
			"task_alignment", qs.TaskAlignment,
			"action", qs.Action,
		)
		switch qs.Action {
		case sense.ActionAdvise:
			msg := o.Stabilizer.FormatSystemMessage(qs)
			o.ConversationHistory.AddSystemMessage(msg)
			o.Logger.Info("Stabilizer advice injected", "advice", qs.Advice)
			return true, nil // Advice injected — no need for full Consultant call.
		case sense.ActionEscalate:
			o.Logger.Info("Stabilizer escalation triggered", "reason", qs.Advice)
			// Fall through to the Consultant check below.
		default: // sense.ActionNone — no intervention needed.
			return injected, nil
		}
	}

	// ── 3. Consultant Check (escalation path) ────────────────────────────────
	if o.Consultant == nil {
		return injected, nil
	}

	prompt := fmt.Sprintf(`
You are the Subconscious Supervisor for an AI Agent.
Review the Agent's last response below.

AGENT RESPONSE:
%s

**YOUR TASK:**
Determine if the agent:
1. Failed to solve the problem.
2. Needs to create a NEW tool to solve it (e.g., if it said "I can't do that").
3. Needs to consult for complex reasoning.
4. Is acting repetitively or getting stuck.

**OUTPUT:**
- If everything is fine, output: "NO_ACTION"
- If intervention is needed, output a concise SYSTEM INSTRUCTION to the agent.
  Example: "SYSTEM_ADVICE: You failed to read the file. Use create_tool to build a file reader."
`, lastResponse)

	advice, err := o.Consultant.Generate(ctx, prompt)
	if err != nil {
		o.Logger.Warn("Consultant agency check failed", "error", err)
		return injected, nil
	}

	advice = strings.TrimSpace(advice)
	if strings.Contains(advice, "NO_ACTION") {
		o.Logger.Info("Agency check: No action needed")
		return injected, nil
	}

	o.Logger.Info("Agency check triggered intervention", "advice", advice)
	if strings.HasPrefix(advice, "SYSTEM_ADVICE:") {
		advice = strings.TrimPrefix(advice, "SYSTEM_ADVICE:")
	}
	injectMsg := fmt.Sprintf("\n[SUBCONSCIOUS INSIGHT]: %s\n", strings.TrimSpace(advice))
	o.ConversationHistory.AddSystemMessage(injectMsg)

	return true, nil
}

// ExecuteTask handles a user prompt with multi-turn tool execution support
func (o *Orchestrator) ExecuteTask(ctx context.Context, prompt string) (string, error) {
	return o.ExecuteTaskWithTools(ctx, prompt, nil)
}

// ExecuteTaskWithTools handles a user prompt with tool chaining and callback support
func (o *Orchestrator) ExecuteTaskWithTools(ctx context.Context, prompt string, toolCallback func(string, *tools.ToolResult)) (string, error) {
	// ── Interrupt support: wrap context so Interrupt() can cancel ───────────
	ctx, cancelFn := context.WithCancel(ctx)
	o.cancelMu.Lock()
	o.cancelFunc = cancelFn
	o.cancelMu.Unlock()
	defer func() {
		cancelFn()
		o.cancelMu.Lock()
		o.cancelFunc = nil
		o.cancelMu.Unlock()
	}()

	// ── Fire session hooks ───────────────────────────────────────────────────
	if o.Hooks != nil {
		o.Hooks.FireAsync(ctx, hooks.EventSessionStart, hooks.Payload{
			Extra: map[string]interface{}{"prompt_length": len(prompt)},
		})
	}

	o.Logger.Info("Analyzing task complexity...", "prompt_length", len(prompt))

	// ── ARC: classify prompt and compute platform-aware resource budget ───────
	var arcDecision arc.RouteDecision
	if o.Intelligence != nil {
		arcDecision = o.Intelligence.Route(prompt)
		o.Logger.Info("ARC routing decision",
			"workflow", arcDecision.Classification.String(),
			"platform", o.Intelligence.Router.PlatformName(),
			"max_tool_calls", arcDecision.Budget.MaxToolCalls,
			"temperature", arcDecision.Budget.Temperature)

		// Auto-initialize SecurityCtx for SecurityCritical workflow tasks.
		if arcDecision.Classification == arc.WorkflowSecurityCritical && o.SecurityCtx == nil {
			o.SecurityCtx = subagents.NewSecurityContext("", fmt.Sprintf("session-%d", time.Now().Unix()))
			o.Logger.Info("SecurityCtx auto-initialized for SecurityCritical workflow")
		}
	}

	// Intelligent Trigger Logic
	needsConsult := false
	upperPrompt := strings.ToUpper(prompt)

	// Check keywords
	if strings.Contains(upperPrompt, "COMPLEX") || strings.Contains(upperPrompt, "REFRESH") {
		needsConsult = true
		o.Logger.Info("Complexity trigger detected", "trigger", "keyword_match")
	}

	// Check length threshold
	if len(prompt) > 1000 {
		needsConsult = true
		o.Logger.Info("Complexity trigger detected", "trigger", "length_threshold")
	}

	var consultationAdvice string
	if needsConsult && o.Consultant != nil {
		o.Logger.Info("Triggering Specialty Consult with Gemini...", "consultant", o.Consultant.Name())

		if o.EnableWatchdog {
			o.printWatchdogState("Consultation", prompt)
		}

		advice, err := o.Consultant.Generate(ctx, prompt)
		if err != nil {
			o.Logger.Error("Consultation failed", "error", err)
			consultationAdvice = ""
		} else if advice == "" {
			o.Logger.Warn("Consultant returned empty response")
			consultationAdvice = ""
		} else {
			consultationAdvice = advice
			o.Logger.Info("Consultation received", "length", len(advice))
		}
	}

	// Determine once whether native function calling is available.
	// Native path: xAI returns structured tool_calls → much more reliable than text parsing.
	// Fallback path: text-based JSON extraction from the AI response (original behaviour).
	// Detection must happen BEFORE history injection so the correct system prompt is used.
	var nativeCaller ai.NativeToolCaller
	var grokTools []ai.GrokToolSchema
	if ntc, ok := o.Primary.(ai.NativeToolCaller); ok {
		nativeCaller = ntc
		grokTools = o.buildGrokTools()
	}
	useNative := nativeCaller != nil && len(grokTools) > 0

	// Add system message with tool context (only on first message if history is empty)
	if o.ConversationHistory.Count() == 0 {
		toolContext := ""
		if o.Registry != nil {
			toolContext = o.getToolContextInternal(useNative)
		}
		// Inject plan mode instructions if active
		if o.ModeManager != nil {
			modeInject := o.ModeManager.SystemPromptInjection()
			if modeInject != "" {
				toolContext += modeInject
			}
		}
		if toolContext != "" {
			o.ConversationHistory.AddSystemMessage(toolContext)
		}
	}

	// ── Per-turn memory refresh ───────────────────────────────────────────────
	// Upsert a single pinned system message with all query-relevant memory
	// signals (AgeMem cross-tier, Engrams, MEL heuristics). This replaces
	// the previous version every turn so context stays fresh without bloating
	// history. The tag prefix [[MEMORY]] identifies the pinned slot.
	if memCtx := o.buildMemoryContext(prompt); memCtx != "" {
		o.ConversationHistory.UpsertSystemMessage("[[MEMORY]]", memCtx)
	}

	// Build user message with consultation advice if provided
	userMessage := prompt
	if consultationAdvice != "" {
		userMessage = fmt.Sprintf("EXPERT CONSULTANT ADVICE:\n%s\n\nUSER REQUEST:\n%s", consultationAdvice, prompt)
	}

	// Add user message to history
	o.ConversationHistory.AddUserMessage(userMessage)

	// Ensure history doesn't exceed context limit (use 80% of Grok's 128k context)
	maxContextTokens := 100000
	o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)

	// Multi-turn tool execution loop — capped by ARC budget when available.
	maxTurns := 10 // default: prevent infinite loops
	if o.Intelligence != nil && arcDecision.Budget.MaxToolCalls > 0 {
		maxTurns = arcDecision.Budget.MaxToolCalls
	}
	var finalResponse string

	// batchResult collects one tool execution outcome.
	type batchResult struct {
		req    tools.ToolRequest
		result *tools.ToolResult
		err    error
	}

	for turn := 0; turn < maxTurns; turn++ {
		o.Logger.Info("Executing AI turn", "turn", turn+1, "max_turns", maxTurns)

		if o.EnableWatchdog {
			o.printWatchdogState(fmt.Sprintf("Turn %d", turn+1), fmt.Sprintf("History messages: %d", o.ConversationHistory.Count()))
		}

		// updateContextMgr pulls the last-call usage from the provider (if supported)
		// and records the outcome for model confidence tracking.
		updateContextMgr := func(failed bool) {
			modelID := o.Primary.GetMetadata().ID
			// EWMA confidence tracking
			if globalProvMgr != nil {
				globalProvMgr.RecordOutcome(modelID, failed)
			}
			if ur, ok := o.Primary.(ai.UsageReporter); ok {
				u := ur.LastUsage()
				provID := string(o.Primary.ID())
				if o.ContextMgr != nil {
					o.ContextMgr.UpdateFromUsage(TokenUsage{
						InputTokens:  u.PromptTokens,
						OutputTokens: u.CompletionTokens,
						ProviderID:   provID,
						ModelID:      modelID,
					})
				}
				if o.Billing != nil {
					o.Billing.TrackTurn(provID, modelID, u.PromptTokens, u.CompletionTokens)
				}
			}
		}

		// ── AI Call ────────────────────────────────────────────────────────────
		var toolRequests []tools.ToolRequest

		if useNative {
			// ── Native function-calling path ─────────────────────────────────
			nativeResult, err := nativeCaller.GenerateWithTools(ctx, o.ConversationHistory, grokTools)
			if err != nil {
				updateContextMgr(true)
				o.Logger.Error("Native AI call failed", "error", err, "turn", turn+1)
				return "", err
			}
			updateContextMgr(nativeResult.Content == "" && len(nativeResult.ToolCalls) == 0)

			if len(nativeResult.ToolCalls) == 0 {
				// Final answer — no tools requested.
				finalResponse = nativeResult.Content
				o.ConversationHistory.AddAssistantMessage(nativeResult.Content)
				o.Logger.Info("Native call: final answer", "turn", turn+1)
				injected, err := o.AnalyzeAgency(ctx, finalResponse)
				if err != nil {
					o.Logger.Warn("Agency analysis error", "error", err)
				}
				if injected {
					o.Logger.Info("SENSE intervention triggered, continuing execution loop")
					continue
				}
				break
			}

			// Record tool-call intent in history (role:"assistant" with tool_calls).
			entries := make([]ai.ToolCallEntry, len(nativeResult.ToolCalls))
			for i, tc := range nativeResult.ToolCalls {
				entries[i] = ai.ToolCallEntry{
					ID:        tc.ID,
					ToolName:  tc.Function.Name,
					Arguments: tc.Function.Arguments,
				}
			}
			o.ConversationHistory.AddToolCallMessage(entries)

			// Convert native tool calls → ToolRequest slice for the shared executor.
			toolRequests = make([]tools.ToolRequest, len(nativeResult.ToolCalls))
			for i, tc := range nativeResult.ToolCalls {
				var params map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &params)
				if params == nil {
					params = map[string]interface{}{}
				}
				toolRequests[i] = tools.ToolRequest{
					ToolName:   tc.Function.Name,
					Parameters: params,
					RequestID:  tc.ID,
				}
			}

			if o.Stylist != nil {
				o.Stylist.StartActionBlock()
			}
			o.Logger.Info("Native tool calls", "count", len(toolRequests), "turn", turn+1)

		} else {
			// ── Text-based fallback path ──────────────────────────────────────
			response, err := o.Primary.GenerateWithHistory(ctx, o.ConversationHistory)
			if err != nil {
				updateContextMgr(true)
				o.Logger.Error("Primary execution failed", "error", err, "turn", turn+1)
				return "", err
			}
			updateContextMgr(response == "")

			o.ConversationHistory.AddAssistantMessage(response)
			// Strip raw JSON tool blocks from user-visible output unless debug mode is on.
			// ParseToolRequests always runs on the original full response.
			if o.DebugMode {
				finalResponse = response
			} else {
				finalResponse = tools.StripToolBlocks(response)
			}
			o.Logger.Debug("raw AI response (text path)", "response", response)

			toolRequests = tools.ParseToolRequests(response)
			if len(toolRequests) == 0 {
				o.Logger.Info("No tool requests found, task complete", "turn", turn+1)
				injected, err := o.AnalyzeAgency(ctx, finalResponse)
				if err != nil {
					o.Logger.Warn("Agency analysis error", "error", err)
				}
				if injected {
					o.Logger.Info("SENSE intervention triggered, continuing execution loop")
					continue
				}
				break
			}

			// Extract text before the first code block as reasoning for Stylist.
			if o.Stylist != nil {
				parts := strings.Split(response, "```")
				if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
					o.Stylist.PrintReasoning(parts[0])
				}
				o.Stylist.StartActionBlock()
			}
			o.Logger.Info("Found tool requests (text-parsed)", "count", len(toolRequests), "turn", turn+1)
		}

		// ── Concurrent tool execution (shared by both paths) ──────────────────
		batchResults := make([]batchResult, len(toolRequests))
		var stylistMu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4) // cap concurrency at 4

		for i, req := range toolRequests {
			wg.Add(1)
			go func(idx int, r tools.ToolRequest) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				stylistMu.Lock()
				if o.Stylist != nil {
					paramsBytes, _ := json.Marshal(r.Parameters)
					o.Stylist.LogToolExecution(r.ToolName, string(paramsBytes))
				}
				stylistMu.Unlock()

				res, execErr := o.ExecuteTool(ctx, r)
				batchResults[idx] = batchResult{req: r, result: res, err: execErr}

				stylistMu.Lock()
				if o.Stylist != nil && res != nil {
					out := res.Output
					if !res.Success {
						out = res.Error
					}
					o.Stylist.LogToolResult(res.Success, out)
				}
				stylistMu.Unlock()
			}(i, req)
		}
		wg.Wait()

		// ── Collect results and feed back to history ──────────────────────────
		toolResults := []string{} // used only by text-based path
		for i, br := range batchResults {
			result := br.result
			if br.err != nil {
				o.Logger.Error("Tool execution error", "tool", br.req.ToolName, "error", br.err)
				result = &tools.ToolResult{Success: false, Error: br.err.Error()}
			}

			if toolCallback != nil {
				toolCallback(br.req.ToolName, result)
			}

			if useNative {
				// Native path: individual role:"tool" messages keyed by call ID.
				content := result.Output
				if !result.Success {
					content = "Error: " + result.Error
				}
				o.ConversationHistory.AddToolResultMessage(
					toolRequests[i].RequestID,
					toolRequests[i].ToolName,
					content,
				)
			} else {
				// Text path: accumulate into a single user message.
				var resultStr string
				if result.Success {
					resultStr = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: true\nOutput:\n%s\n</tool_result>",
						br.req.ToolName, result.Output)
				} else {
					resultStr = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: false\nError: %s\n</tool_result>",
						br.req.ToolName, result.Error)
				}
				toolResults = append(toolResults, resultStr)
			}
		}

		if !useNative {
			msg := "Here are the results from the tools you requested:\n\n" +
				strings.Join(toolResults, "\n\n") +
				"\n\nPlease continue with the task based on these results. If you need more tools, request them. Otherwise, provide your final response to the user."
			o.ConversationHistory.AddUserMessage(msg)
		}

		o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)
	}

	return finalResponse, nil
}

// GetToolContext returns tool definitions formatted for AI context (text-based tool calling).
func (o *Orchestrator) GetToolContext() string {
	return o.getToolContextInternal(false)
}

// getToolContextInternal builds the system prompt.
// useNative=true → omit JSON-block format instructions (model uses structured tool schemas).
func (o *Orchestrator) getToolContextInternal(useNative bool) string {
	if o.Registry == nil {
		return ""
	}

	env, _ := platform.GetEnvConfig()
	osInfo := "Unknown"
	if env != nil {
		osInfo = fmt.Sprintf("%s (%s)", env.OS, env.Arch)
		if env.IsTermux {
			osInfo += " [Termux]"
		}
	}

	var sb strings.Builder
	sb.WriteString("SYSTEM INSTRUCTIONS:\n")
	sb.WriteString(fmt.Sprintf("You are Gorkbot v%s, an intelligent AI assistant designed by Todd Eddings (Velarium AI). ", platform.Version) +
		"Gorkbot is an independent project — not affiliated with xAI's Grok or Google's Gemini — " +
		"and works with any OpenAI-compatible API endpoint.\n")
	sb.WriteString(fmt.Sprintf("Environment: %s\n", osInfo))
	sb.WriteString("Your goal is to assist the user by executing tasks, answering questions, and providing insights.\n\n")

	sb.WriteString("### CRITICAL GUIDELINES:\n")
	sb.WriteString("1. **Reason First**: Before taking action, briefly explain your reasoning. Why are you choosing a specific tool?\n")
	sb.WriteString("2. **Consult Specialist**: For complex planning, reasoning, or if you are unsure, use the `consultation` tool to get expert advice.\n")
	sb.WriteString("3. **Verify Facts**: If asked for current events or specific data, use `web_search` or `web_fetch`. Do not hallucinate.\n")
	sb.WriteString("4. **Tool Usage**: Use tools only when necessary. If a simple answer suffices, just answer.\n")
	if useNative {
		sb.WriteString("5. **Tool Invocation**: Call tools using the structured function-calling interface provided by the API. Do NOT output raw JSON blocks in your response text.\n")
	} else {
		sb.WriteString("5. **Output Format**: When invoking a tool, always start with a brief plain-language explanation of what you are doing and why. Then include the JSON tool block. After receiving tool results, summarize the outcome in natural language — do NOT repeat the raw JSON or results verbatim in your reply.\n")
	}
	sb.WriteString("6. **Preview Destructive Actions**: Before running destructive shell commands or modifying production systems, use `code2world` to preview the action.\n")
	sb.WriteString("7. **Record Preferences**: When you learn a user preference or a reliable tool pattern, use `record_engram` to remember it for future sessions.\n\n")
	sb.WriteString("8. **Parallel Agent Orchestration**: For complex coding tasks, DO NOT attempt to do everything yourself sequentially. Instead, spawn specialized agents in parallel (e.g., 'plan', 'frontend-styling-expert', 'full-stack-developer', 'code-reviewer', 'test-engineer') to work efficiently. Use `spawn_agent` to launch them and `check_agent_status` to monitor their progress.\n\n")

	sb.WriteString("### TOOL USAGE SOP (STRICT ENFORCEMENT):\n")
	sb.WriteString("1. **CHECK AVAILABLE TOOLS FIRST**: Before even considering creating a new tool, you MUST exhaustively review the 'AVAILABLE TOOLS' list below.\n")
	sb.WriteString("2. **NO DUPLICATES**: Do NOT create a tool if a similar one exists. For example, do not create `read_file_content` if `read_file` exists. Do not create `run_bash` if `bash` exists.\n")
	sb.WriteString("3. **REUSE**: If an existing tool can perform the task (e.g., using `bash` to run a custom script), USE IT instead of creating a new dedicated tool.\n")
	sb.WriteString("4. **JUSTIFY CREATION**: You may ONLY create a new tool if strictly necessary and NO existing tool can accomplish the goal. In your reasoning, you must explicitly state: 'I checked the tool list and tool X is missing/insufficient because...'.\n\n")

	// NOTE: Per-session memory context (AgeMem, Engrams, MEL heuristics) is
	// injected dynamically via buildMemoryContext + UpsertSystemMessage on
	// every turn in ExecuteTaskWithTools — not here — so it stays query-relevant.

	// Inject Dynamic Brain System (SOUL, IDENTITY, USER, MEMORY)
	sb.WriteString(GetDynamicBrainContext())

	// Inject GORKBOT.md project instructions if configured.
	if o.ConfigLoader != nil {
		projectInstructions := o.ConfigLoader.LoadInstructions()
		if projectInstructions != "" {
			sb.WriteString(projectInstructions)
		}
	}

	if useNative {
		sb.WriteString(o.Registry.GetSystemPromptNative())
	} else {
		sb.WriteString(o.Registry.GetSystemPrompt())
	}

	return sb.String()
}

// buildGrokTools converts the tool registry into xAI native function-call schemas.
func (o *Orchestrator) buildGrokTools() []ai.GrokToolSchema {
	if o.Registry == nil {
		return nil
	}
	defs := o.Registry.GetDefinitions()
	schemas := make([]ai.GrokToolSchema, 0, len(defs))
	for _, def := range defs {
		schemas = append(schemas, ai.GrokToolSchema{
			Type: "function",
			Function: ai.GrokFunctionDef{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
			},
		})
	}
	return schemas
}

// buildMemoryContext aggregates ALL parametric memory signals relevant to the
// current prompt and formats them as a single pinned system message block.
//
// Sources (in injection order):
//  1. AgeMem FormatRelevant — cross-tier (STM hot + LTM cold) keyword-ranked facts
//  2. EngramStore FormatAsContext — learned tool preferences from past sessions
//  3. Intelligence HeuristicContext — MEL failure-derived guardrails
//
// Returns "" when all sources are empty so no empty system messages are injected.
func (o *Orchestrator) buildMemoryContext(prompt string) string {
	var parts []string

	// 1. AgeMem: cross-tier query-relevant facts. FormatRelevant searches both
	//    STM (in-session, hot/warm) and LTM (cross-session, cold/persistent)
	//    using keyword overlap × priority × recency scoring.
	if o.AgeMem != nil {
		if ctx := o.AgeMem.FormatRelevant(prompt, 600); ctx != "" {
			parts = append(parts, "### Remembered Context (AgeMem):\n"+ctx)
		}
	}

	// 2. Engrams: learned tool/behaviour preferences written by `record_engram`.
	//    Persisted to LTM unconditionally so they survive across sessions.
	if o.Engrams != nil {
		if ctx := o.Engrams.FormatAsContext(prompt); ctx != "" {
			parts = append(parts, ctx)
		}
	}

	// 3. MEL heuristics: failure→correction guardrails generated by the
	//    BifurcationAnalyzer. Scored by Jaccard × confidence × log(UseCount).
	if o.Intelligence != nil {
		if ctx := o.Intelligence.HeuristicContext(prompt); ctx != "" {
			parts = append(parts, ctx)
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return "[[MEMORY]]\n" + strings.Join(parts, "\n") + "\n[[/MEMORY]]"
}

// ExecuteTool executes a tool request with permission checking and SENSE HITL gating.
func (o *Orchestrator) ExecuteTool(ctx context.Context, req tools.ToolRequest) (*tools.ToolResult, error) {
	if o.Registry == nil {
		return &tools.ToolResult{
			Success: false,
			Error:   "tool registry not initialized",
		}, fmt.Errorf("tool registry not initialized")
	}

	// ── Plan mode block ───────────────────────────────────────────────────────
	if o.ModeManager != nil {
		allowed, _ := o.ModeManager.IsToolAllowed(req.ToolName)
		if !allowed {
			return &tools.ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Tool '%s' is blocked in PLAN mode. Switch to NORMAL mode with /mode normal to execute write/shell tools.", req.ToolName),
			}, nil
		}
	}

	// ── Fine-grained rule check ───────────────────────────────────────────────
	if o.RuleEngine != nil {
		decision, rule := o.RuleEngine.Evaluate(req.ToolName, req.Parameters)
		switch decision {
		case tools.RuleDeny:
			reason := fmt.Sprintf("Blocked by rule '%s'", rule.Pattern)
			if rule.Comment != "" {
				reason += ": " + rule.Comment
			}
			return &tools.ToolResult{Success: false, Error: reason}, nil
		case tools.RuleAllow:
			// Skip permission prompts — rule grants approval
			o.Logger.Debug("Rule-granted tool", "tool", req.ToolName, "pattern", rule.Pattern)
		case tools.RuleAsk:
			o.Logger.Debug("Rule requires prompt", "tool", req.ToolName, "pattern", rule.Pattern)
		}
	}

	// ── Tool cache check ─────────────────────────────────────────────────────
	if o.ToolCache != nil {
		if cached, ok := o.ToolCache.Get(req.ToolName, req.Parameters); ok {
			o.Logger.Debug("Cache hit", "tool", req.ToolName)
			return cached, nil
		}
	}

	// ── Pre-tool hook ────────────────────────────────────────────────────────
	if o.Hooks != nil {
		hookResult := o.Hooks.Fire(ctx, hooks.EventPreToolUse, hooks.Payload{
			Tool:   req.ToolName,
			Params: req.Parameters,
		})
		if hookResult.Blocked {
			reason := "Blocked by pre_tool_use hook"
			if hookResult.Reason != "" {
				reason += ": " + hookResult.Reason
			}
			return &tools.ToolResult{Success: false, Error: reason}, nil
		}
	}

	// ── Save checkpoint before execution ─────────────────────────────────────
	if o.Checkpoints != nil && o.ConversationHistory != nil {
		desc := fmt.Sprintf("Before: %s", req.ToolName)
		if len(req.Parameters) > 0 {
			// Add first param value to description
			for _, v := range req.Parameters {
				if s, ok := v.(string); ok && len(s) > 0 {
					if len(s) > 40 {
						s = s[:40] + "..."
					}
					desc = fmt.Sprintf("Before: %s(%s)", req.ToolName, s)
					break
				}
			}
		}
		
		// Create workspace checkpoint if it's a file modifying tool
		workspaceHash := ""
		if o.Workspace != nil && tools.IsFileModifier(req.ToolName) {
			if h, err := o.Workspace.CreateCheckpoint(); err == nil {
				workspaceHash = h
			}
		}
		
		o.Checkpoints.Save(desc, o.ConversationHistory, workspaceHash)
	}

	// ── SENSE HITL Gate ──────────────────────────────────────────────────────
	// Extract the most recent AI reasoning to pass to the plan builder.
	aiReasoning := ""
	msgs := o.ConversationHistory.GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			aiReasoning = msgs[i].Content
			if len(aiReasoning) > 300 {
				aiReasoning = aiReasoning[:300]
			}
			break
		}
	}

	proceed, notes := o.GateToolExecution(ctx, req, o.HITLGuard, o.HITLCallback, aiReasoning)
	if !proceed {
		reason := "Execution blocked by HITL guard"
		if notes != "" {
			reason += ": " + notes
		}
		return &tools.ToolResult{
			Success: false,
			Error:   reason,
		}, nil
	}
	if notes != "" {
		// Inject amendment notes into the context.
		o.ConversationHistory.AddSystemMessage("[HITL AMENDMENT]: " + notes)
	}
	// ─────────────────────────────────────────────────────────────────────────

	o.Logger.Info("Executing tool", "tool", req.ToolName, "request_id", req.RequestID)

	// ── Trace: log call ───────────────────────────────────────────────────────
	if o.Tracer != nil {
		o.Tracer.LogToolCall(req.ToolName, req.Parameters)
	}

	// Add orchestrator to context for tools that need to resolve consultants (e.g., consultation tool)
	ctxWithOrchestrator := context.WithValue(ctx, tools.OrchestratorContextKey(), o)
	// Inject CCI layer so mcp_context_* tools can access Tier 1/2/3 cold memory.
	ctxWithOrchestrator = o.InjectCCIContextIntoRegistry(ctxWithOrchestrator)
	// Inject SecurityCtx so report_finding tool can write to shared assessment state.
	if o.SecurityCtx != nil {
		ctxWithOrchestrator = context.WithValue(ctxWithOrchestrator, tools.SecurityContextKey, o.SecurityCtx)
	}

	toolStart := time.Now()
	result, err := o.Registry.Execute(ctxWithOrchestrator, &req)
	toolElapsed := time.Since(toolStart)

	// ── Enrich failed results with structured recovery hints ─────────────────
	if result != nil && !result.Success {
		tools.EnrichResult(result, req.ToolName)
	}

	// ── CCI Gap Detection ─────────────────────────────────────────────────────
	// If mcp_context_get_subsystem returned empty, activate PLAN mode and
	// inject a gap notification into the conversation so the AI understands
	// it must map the subsystem before proceeding with implementation.
	if req.ToolName == "mcp_context_get_subsystem" && result != nil && !result.Success {
		if subsys, ok := req.Parameters["name"].(string); ok && subsys != "" {
			gapMsg := o.HandleCCIGap(subsys)
			if gapMsg != "" {
				o.Logger.Warn("CCI gap handler activated", "subsystem", subsys)
				// Augment the tool result error with the gap notice.
				result.Error = gapMsg + "\n\n" + result.Error
			}
		}
	}

	// ── MEL: Observe tool result for bifurcation analysis ────────────────────
	if o.Intelligence != nil && result != nil {
		if !result.Success {
			o.Intelligence.ObserveFailed(req.ToolName, req.Parameters, result.Error)
		} else {
			o.Intelligence.ObserveSuccess(req.ToolName, req.Parameters)
		}
		// Phase 3.3: Feed analytics-derived reliability signal into MEL.
		// If a tool has low success rate after enough executions, generate a
		// reliability heuristic so the AI learns to be more careful with it.
		if o.Registry != nil && o.Registry.GetAnalytics() != nil {
			analytics := o.Registry.GetAnalytics()
			stats := analytics.GetStats(req.ToolName)
			if stats != nil && stats.ExecutionCount > 5 {
				successRate := analytics.GetSuccessRate(req.ToolName)
				if successRate < 0.5 {
					h := &mel.Heuristic{
						Context:     fmt.Sprintf("using %s tool", req.ToolName),
						Constraint:  "verify parameters carefully before execution",
						Error:       fmt.Sprintf("repeated failure (success rate: %.0f%%)", successRate*100),
						ContextTags: []string{req.ToolName, "reliability", "low_success"},
						Confidence:  0.5 + (0.5 - successRate), // higher confidence for lower rates
					}
					o.Intelligence.Store.Add(h)
				}
			}
		}
	}

	// ── Trace: log result ────────────────────────────────────────────────────
	if o.Tracer != nil {
		out := ""
		ok := false
		if result != nil {
			ok = result.Success
			if result.Success {
				out = result.Output
			} else {
				out = result.Error
			}
		}
		if err != nil {
			out = err.Error()
		}
		o.Tracer.LogToolResult(req.ToolName, out, ok, toolElapsed)
	}

	// ── Post-tool cache + hooks ───────────────────────────────────────────────
	if result != nil && result.Success && o.ToolCache != nil {
		o.ToolCache.Set(req.ToolName, req.Parameters, result)
		o.ToolCache.InvalidateOnMutation(req.ToolName)
	}
	if o.Hooks != nil {
		event := hooks.EventPostToolUse
		if err != nil || (result != nil && !result.Success) {
			event = hooks.EventPostToolFailure
		}
		payload := hooks.Payload{Tool: req.ToolName, Params: req.Parameters}
		if result != nil {
			payload.Result = &hooks.ResultInfo{
				Success: result.Success,
				Output:  truncate(result.Output, 200),
				Error:   result.Error,
			}
		}
		o.Hooks.FireAsync(ctx, event, payload)
	}

	if err != nil {
		o.Logger.Error("Tool execution failed", "tool", req.ToolName, "error", err)
		// Store failure in AgeMem so the AI sees recent failures for this tool
		// when building its next prompt (helps it self-correct on retries).
		if o.AgeMem != nil {
			key := fmt.Sprintf("tool_fail:%s", req.ToolName)
			o.AgeMem.Store(key,
				fmt.Sprintf("Tool %s FAILED: %s", req.ToolName, truncate(err.Error(), 120)),
				0.70, nil, false)
		}
	} else {
		o.Logger.Info("Tool executed successfully", "tool", req.ToolName, "success", result.Success)

		// Store tool-level failures (result.Success=false) in AgeMem too.
		if o.AgeMem != nil && !result.Success {
			key := fmt.Sprintf("tool_fail:%s", req.ToolName)
			o.AgeMem.Store(key,
				fmt.Sprintf("Tool %s FAILED: %s", req.ToolName, truncate(result.Error, 120)),
				0.70, nil, false)
		}

		// Record successful tool execution in AgeMem for preference learning.
		// Priority tiers:
		//   0.85 + persist=true  → bifurcation-corrected success (hard-won, LTM immediately)
		//   0.80 + persist=false → repeat success (will consolidate to LTM on STM prune)
		//   0.65 + persist=false → first-time success (stays in STM, warms up over time)
		if o.AgeMem != nil && result.Success {
			key := fmt.Sprintf("tool_success:%s", req.ToolName)
			summary := fmt.Sprintf("Tool %s succeeded with params: %s. Snippet: %s",
				req.ToolName, truncate(fmt.Sprintf("%v", req.Parameters), 80), truncate(result.Output, 100))

			// Check if this was a bifurcation-corrected run (prior failure existed).
			// The BifurcationAnalyzer already consumed the pending record in ObserveSuccess,
			// so we detect it by checking whether a MEL heuristic was just generated
			// (store.Len increased). Simpler: peek the Intelligence.Analyzer pending map
			// via a dedicated method — but to avoid coupling, use a flag approach:
			// store at 0.8 always (consolidates to LTM on next STM prune) and let
			// access frequency naturally promote important patterns.
			if existing, ok := o.AgeMem.Retrieve(key); ok && existing.AccessCount >= 2 {
				// Repeat success for this tool — promote priority so it consolidates.
				o.AgeMem.Store(key, summary, 0.82, nil, false)
			} else {
				o.AgeMem.Store(key, summary, 0.65, nil, false)
			}
		}

		// If the agent explicitly called record_engram, write into the Engram store.
		if req.ToolName == "record_engram" && o.Engrams != nil && result.Success {
			pref, _ := req.Parameters["preference"].(string)
			cond, _ := req.Parameters["condition"].(string)
			toolName, _ := req.Parameters["tool_name"].(string)
			conf := 0.7
			if v, ok := req.Parameters["confidence"].(float64); ok {
				conf = v
			}
			o.Engrams.Record(sense.Engram{
				ToolName:   toolName,
				Preference: pref,
				Condition:  cond,
				Confidence: conf,
			})
			o.Logger.Info("Engram recorded", "preference", pref, "tool", toolName)
		}
	}

	return result, err
}

// truncate returns the first n bytes of s (ASCII-safe).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
