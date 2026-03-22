package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/internal/xskill"
	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/cache"
	"github.com/velariumai/gorkbot/pkg/spark"
	"github.com/velariumai/gorkbot/pkg/sre"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/billing"
	"github.com/velariumai/gorkbot/pkg/collab"
	"github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/discovery"
	"github.com/velariumai/gorkbot/pkg/hooks"
	"github.com/velariumai/gorkbot/pkg/memory"
	"github.com/velariumai/gorkbot/pkg/persist"
	"github.com/velariumai/gorkbot/pkg/router"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/skills"
	"github.com/velariumai/gorkbot/pkg/session"
	"github.com/velariumai/gorkbot/pkg/subagents"
	"github.com/velariumai/gorkbot/pkg/tools"
	"github.com/velariumai/gorkbot/pkg/tui"
	"github.com/velariumai/gorkbot/pkg/vectorstore"
)

// Orchestrator manages the interaction between the user and the AI providers.
type Orchestrator struct {
	Primary             ai.AIProvider
	Consultant          ai.AIProvider
	Registry            *tools.Registry
	Logger              *slog.Logger
	EnableWatchdog      bool
	NativeLLMEnabled    bool
	ConversationHistory *ai.ConversationHistory
	Stylist             *tui.Stylist
	PersistStore        *persist.Store
	SessionID           string

	// ── SENSE components ────────────────────────────────────────────────────
	LIE          *sense.LIEEvaluator
	Stabilizer   *sense.Stabilizer
	AgeMem       *sense.AgeMem
	Engrams      *sense.EngramStore
	Compressor   *sense.Compressor
	HITLGuard    *HITLGuard
	HITLCallback HITLCallback
	HAL          platform.HALProfile
	// SENSETracer writes structured SENSE events to daily JSONL trace files.
	// Nil when tracing is not configured.  Used to record context overflows
	// and provider errors that occur during streaming (outside the tool layer).
	SENSETracer *sense.SENSETracer

	// MessageSuppressionMiddleware filters internal system messages based on verbose mode.
	// Nil = no suppression (all messages passed through).
	MessageSuppressor *MessageSuppressionMiddleware

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

	// ThinkingBudget, when > 0, enables extended thinking on providers that
	// support it (Anthropic claude-3-7+/claude-4, xAI grok-3-mini).
	// The value is the token budget for the thinking block.
	ThinkingBudget int

	// Lock-free atomic callbacks (set via SetThinkingCallback/SetStatusCallback)
	// thinkingCallback: func(token string), invoked via invokeThinking
	thinkingCallback atomic.Value

	// statusCallback: func(phase, desc string, tokens int, model string), invoked via invokeStatus
	statusCallback atomic.Value

	// ToolProgressCallback, when non-nil, is called for each tool execution
	// with elapsed time updates every ~250ms while running, and once on completion.
	// Signature: func(toolName string, elapsed time.Duration, done bool, success bool)
	// The TUI uses this to update the live tools panel with per-tool timings.
	ToolProgressCallback func(toolName string, elapsed time.Duration, done bool, success bool)

	// ExecutionStats tracks historical tool execution times for progress estimation.
	ExecutionStats *tools.ExecutionStats

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

	// ── Companion systems (augment, never replace) ────────────────────────────

	// CacheAdvisor computes provider-appropriate caching hints before each LLM
	// call. Initialized via InitCacheAdvisor(); nil = no caching augmentation.
	CacheAdvisor *cache.Advisor

	// IngressFilter prunes low-information content from prompts before they
	// reach the ARC classifier and the LLM. Nil = pass-through.
	IngressFilter *adaptive.IngressFilter

	// IngressGuard validates semantic preservation after IngressFilter pruning
	// to prevent ARC classifier evasion attacks. Nil = no guard.
	IngressGuard *adaptive.IngressGuard

	// MELValidator guards the VectorStore against poisoning attacks.
	// Checked inside ObserveSuccess before heuristic persistence.
	// Nil = no validation (degrades to old behaviour).
	MELValidator *adaptive.MELValidator

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
	CCI *adaptive.CCILayer

	// CompressionPipe compresses history when token count exceeds its threshold.
	// Wired in main.go after persistStore and Compressor are ready.
	CompressionPipe *CompressionPipe

	// TieredCompactor enforces context limits before every LLM call using a
	// two-stage strategy: truncate large tool results first, then SENSE-compress.
	// Initialized by InitEnhancements; nil means compaction is managed only by
	// CompressionPipe's async 95% trigger.
	TieredCompactor *TieredCompactor

	// ConfigWatcher polls GORKBOT.md files for live changes and re-injects
	// updated instructions into the conversation without restarting.
	// Initialized by InitEnhancements; nil when config watching is disabled.
	ConfigWatcher *config.ConfigWatcher

	// PromptBuilder assembles the system prompt from named layers (Identity,
	// Soul, Bootstrap, Runtime, ChannelHint). Nil means the legacy flat
	// system prompt construction is used.
	PromptBuilder *PromptBuilder

	// RoutingTable provides regex-based source-to-agent routing that sits in
	// front of the ARC Router as a zero-learning reliable fallback.
	RoutingTable *adaptive.RoutingTable

	// VectorStore indexes conversation turns for semantic search (RAG).
	VectorStore *vectorstore.VectorStore

	// RAGInjector retrieves semantically similar past messages per turn.
	RAGInjector *RAGInjector

	// BudgetGuard enforces per-session and per-day USD spending limits.
	BudgetGuard *BudgetGuard

	// EnvContext is injected into the system prompt before MCPContext.
	// It is a compact (~30 line) snapshot of the host environment: runtimes,
	// Python packages, CLI tools, API key presence, and MCP server status.
	// Built by pkg/env.EnvProbe.BuildSystemContext() and set by main.go.
	// Injected first so the AI sees its operating constraints before the tool
	// list and can avoid calling tools that are guaranteed to fail.
	EnvContext string

	// MCPContext is injected into the system prompt after LoadAndStart completes.
	// It lists which MCP servers are running, their purpose, usage decision rules,
	// and credential requirements so the AI knows when and how to call MCP tools.
	// Set by main.go via: orch.MCPContext = mcpMgr.GetSystemContext()
	MCPContext string

	// configDir is set via SetConfigDir; used by writeMemoryLog.
	configDir string

	// SkillLoader provides skill index for system prompt injection and semantic
	// pre-loading. Wired by main.go after skills loader is initialized.
	SkillLoader *skills.Loader

	// InputSanitizer scans context files (brain, GORKBOT.md) for prompt
	// injection before injecting them into the system prompt.
	// Wired by main.go via SetInputSanitizer().
	InputSanitizer *sense.InputSanitizer

	// ── XSKILL continual-learning system ─────────────────────────────────────
	// XSkillKB is the XSKILL Knowledge Base (Phase 1 accumulation target).
	// Nil until InitXSkill succeeds (requires an available embedder).
	XSkillKB *xskill.KnowledgeBase

	// XSkillEngine drives Phase 2 context enrichment before each LLM call.
	// Nil until InitXSkill succeeds.
	XSkillEngine *xskill.InferenceEngine

	// xskillProvider is the hot-swappable LLM+embed adapter.
	// Swap the embedder via UpgradeXSkillEmbedder().
	xskillProvider *mutableProvider

	// xskillMu guards the per-task trajectory fields below.
	xskillMu sync.Mutex

	// Structured concurrency: root context for all background goroutines
	rootCtx    context.Context
	rootCancel context.CancelCauseFunc
	eg         *errgroup.Group
	shutdown   sync.Once

	// xskillSteps accumulates TrajectoryStep records during a single task.
	xskillSteps []xskill.TrajectoryStep

	// xskillPrompt holds the task prompt for Phase 1 accumulation.
	xskillPrompt string

	// xskillSkillName is the classified domain label for the current task.
	xskillSkillName string

	// xskillTaskStart records when the current task execution began.
	xskillTaskStart time.Time

	// SPARK autonomous reasoning daemon.
	// Nil until InitSPARK succeeds.
	SPARK *spark.SPARK

	// SRE — Step-wise Reasoning Engine
	SRE *sre.Coordinator

	// CascadeOrder overrides the default provider failover sequence.
	// nil or empty means use the hardcoded providerPriority default in fallback.go.
	// Set from AppState.CascadeOrder in main.go after AppState restore.
	CascadeOrder []string

	// sandboxSanitizer stores the InputSanitizer when the sandbox is bypassed
	// so it can be restored by ToggleSandbox(). Nil when sandbox is active.
	sandboxSanitizer *sense.InputSanitizer
}

// NewOrchestrator initializes the orchestration engine with SENSE components.
func NewOrchestrator(primary, consultant ai.AIProvider, registry *tools.Registry, logger *slog.Logger, enableWatchdog bool) *Orchestrator {
	ctx, cancel := context.WithCancelCause(context.Background())

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
		ToolCache:   tools.NewToolCache(1000, nil), // Default bounded cache size, db can be wired later if needed.
		Exporter:    session.NewExporter(),
		// Structured concurrency
		rootCtx:    ctx,
		rootCancel: cancel,
		eg:         new(errgroup.Group),
		// Initialize atomic values for callbacks
		thinkingCallback: atomic.Value{},
		statusCallback:   atomic.Value{},
	}

	// Stabilizer uses Consultant for task-alignment scoring when available.
	var stabGen sense.TextGenerator
	if consultant != nil {
		stabGen = consultant
	}
	o.Stabilizer = sense.NewStabilizer(stabGen)

	// Compressor uses Primary as the generator so compression works even when
	// the Consultant is unavailable. SetCompressorGenerator() hot-swaps this
	// whenever the Primary is changed via SetPrimary().
	if primary != nil {
		o.Compressor = sense.NewCompressor(primary)
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

	// Context manager — tracks token usage; no async callback needed.
	// Compaction is handled synchronously by TieredCompactor.Check() before
	// every LLM call in streaming.go — no async goroutine required.
	o.ContextMgr = NewContextManager(131072, nil)

	cwd, _ := os.Getwd()
	o.Workspace = session.NewWorkspaceManager(cwd)

	o.Crystallizer = NewCrystallizer(o)

	if primary != nil {
		o.primaryModelName = primary.Name()
	}

	return o
}

// SetThinkingCallback sets the callback for thinking block tokens (lock-free).
func (o *Orchestrator) SetThinkingCallback(cb func(string)) {
	o.thinkingCallback.Store(cb)
}

// SetStatusCallback sets the callback for status updates (lock-free).
func (o *Orchestrator) SetStatusCallback(cb func(string, string, int, string)) {
	o.statusCallback.Store(cb)
}

// invokeThinking safely invokes the thinking callback if set (lock-free).
func (o *Orchestrator) invokeThinking(token string) {
	if cb, ok := o.thinkingCallback.Load().(func(string)); ok && cb != nil {
		cb(token)
	}
}

// invokeStatus safely invokes the status callback if set (lock-free).
func (o *Orchestrator) invokeStatus(phase, desc string, tokens int, model string) {
	if cb, ok := o.statusCallback.Load().(func(string, string, int, string)); ok && cb != nil {
		cb(phase, desc, tokens, model)
	}
}

// Shutdown gracefully shuts down the orchestrator and waits for background goroutines.
func (o *Orchestrator) Shutdown(ctx context.Context) error {
	o.shutdown.Do(func() {
		o.rootCancel(errors.New("orchestrator shutdown"))
	})
	return o.eg.Wait()
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

	// ── TieredCompactor: pre-send token enforcement ───────────────────────
	// CompressionPipe may be nil here (wired later by main.go); TieredCompactor
	// accepts nil and will skip Stage-2 until it is set.
	o.TieredCompactor = NewTieredCompactor(o.ContextMgr, o.CompressionPipe, o.Logger)

	// ── ConfigWatcher: hot-reload GORKBOT.md hierarchy ────────────────────
	o.ConfigWatcher = config.NewWatcher(o.ConfigLoader, 2*time.Second)
	o.ConfigWatcher.OnChange(func(path string, event config.ChangeEvent) {
		o.Logger.Info("config file changed — refreshing project instructions",
			"path", path, "event", event.String())
		newInstructions := o.ConfigLoader.LoadInstructions()
		if newInstructions != "" {
			tag := "__gorkbot_project_instructions__"
			o.ConversationHistory.UpsertSystemMessage(tag,
				"### PROJECT INSTRUCTIONS (hot-reloaded)\n\n"+newInstructions)
		}
	})

	// ── RoutingTable: pattern-based agent routing fallback ────────────────
	o.RoutingTable = adaptive.NewRoutingTable()

	// ── PromptBuilder: structured five-layer prompt assembly ─────────────
	o.PromptBuilder = NewPromptBuilder()

	o.Logger.Info("Enhanced systems initialized",
		"hooks_dir", o.Hooks.HooksDir(),
		"config_files", o.ConfigLoader.ActiveFiles())
}

// GetRoutingTable returns the RoutingTable used for source-to-agent dispatch.
// Returns nil when no routing table has been initialized.
func (o *Orchestrator) GetRoutingTable() *adaptive.RoutingTable { return o.RoutingTable }

// StartConfigWatcher begins polling GORKBOT.md files for changes and
// hot-reloading updated project instructions into the conversation.
// ctx should be the application-lifetime context (cancel to stop watching).
// Call after InitEnhancements; no-op if ConfigWatcher is nil.
func (o *Orchestrator) StartConfigWatcher(ctx context.Context) {
	if o.ConfigWatcher == nil {
		return
	}
	go o.ConfigWatcher.Start(ctx)
	o.Logger.Info("config watcher started (polling every 2s)")
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

// InitCompanions initialises all companion augmentation systems:
//   - CacheAdvisor  (prompt caching across all providers)
//   - IngressFilter (token pruning before ARC routing)
//   - IngressGuard  (ARC classifier evasion protection)
//   - MELValidator  (VectorStore poisoning protection)
//
// configDir is the gorkbot config directory. geminiKey and geminiModel are
// optional — Gemini explicit caching is skipped when either is empty.
func (o *Orchestrator) InitCompanions(configDir, geminiKey, geminiModel string) {
	// CacheAdvisor.
	advisor, err := cache.NewAdvisor(geminiKey, geminiModel, configDir)
	if err != nil {
		o.Logger.Warn("CacheAdvisor init failed (running without it)", "error", err)
	} else {
		o.CacheAdvisor = advisor

		// Wire Grok conv-id into the GrokProvider if it is the primary.
		if grokProv, ok := o.Primary.(interface{ SetConvID(string) }); ok {
			hints := advisor.Advise("grok", "", "", nil)
			if hints.GrokConvID != "" {
				grokProv.SetConvID(hints.GrokConvID)
			}
		}
		o.Logger.Info("CacheAdvisor ready")
	}

	// IngressFilter + IngressGuard.
	o.IngressFilter = adaptive.NewIngressFilter(0) // 0 = use default 32k rune cap
	o.IngressGuard = adaptive.NewIngressGuard()
	o.Logger.Info("IngressFilter + IngressGuard ready")

	// MELValidator.
	o.MELValidator = adaptive.NewMELValidator()
	o.Logger.Info("MELValidator ready")
}

// buildSystemPrompt returns the full composed system prompt for the current
// conversation by concatenating all "system" role messages in order.
// This is used by the CacheAdvisor to detect system-prompt changes between
// turns, and by any other component that needs the assembled system context.
func (o *Orchestrator) buildSystemPrompt() string {
	msgs := o.ConversationHistory.GetMessages()
	parts := make([]string, 0, 4)
	for _, m := range msgs {
		if m.Role == "system" && m.Content != "" {
			parts = append(parts, m.Content)
		}
	}
	return strings.Join(parts, "\n")
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

// SetCompressorGenerator hot-swaps the TextGenerator backing the Compressor
// and CompressionPipe. Called from SetPrimary() so compression always uses
// the active primary provider — no dependency on the Consultant being available.
func (o *Orchestrator) SetCompressorGenerator(gen sense.TextGenerator) {
	newComp := sense.NewCompressor(gen)
	o.Compressor = newComp
	if o.CompressionPipe != nil {
		o.CompressionPipe.SetCompressor(newComp)
	}
}

// SetCascadeOrder updates the provider failover order at runtime.
// Pass nil to restore the default hardcoded order.
func (o *Orchestrator) SetCascadeOrder(order []string) {
	o.CascadeOrder = order
}

// GetCascadeOrder returns the current effective cascade order.
func (o *Orchestrator) GetCascadeOrder() []string {
	return o.effectiveCascade()
}

// ToggleDebug flips the DebugMode flag and returns the new value.
// When DebugMode is true, raw AI responses (including tool JSON blocks) are
// sent to the TUI without stripping.
func (o *Orchestrator) ToggleDebug() bool {
	o.DebugMode = !o.DebugMode
	o.Logger.Info("Debug mode toggled", "debug_mode", o.DebugMode)
	return o.DebugMode
}

// ToggleSandbox enables or disables the SENSE input sanitizer in the tool registry.
// Returns true when the sandbox is now enabled, false when disabled.
func (o *Orchestrator) ToggleSandbox() bool {
	if o.Registry == nil {
		return true // no registry → treat as enabled (safe default)
	}
	if o.Registry.HasSanitizer() {
		// Sandbox is ON → turn it OFF: stash the sanitizer and bypass.
		if s, ok := o.Registry.GetSanitizer().(*sense.InputSanitizer); ok {
			o.sandboxSanitizer = s
		}
		o.Registry.BypassSanitizer()
		o.Logger.Warn("SENSE input sanitizer bypassed — sandbox disabled")
		return false
	}
	// Sandbox is OFF → turn it ON: restore stashed sanitizer (or no-op if none).
	if o.sandboxSanitizer != nil {
		o.Registry.RestoreSanitizer(o.sandboxSanitizer)
		o.Logger.Info("SENSE input sanitizer restored — sandbox enabled")
	}
	return true
}

// SandboxEnabled returns true when the SENSE input sanitizer is currently active.
func (o *Orchestrator) SandboxEnabled() bool {
	return o.Registry != nil && o.Registry.HasSanitizer()
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

// SaveSession writes the conversation history to a key-named JSON file under configDir/sessions/.
// When name is empty an auto-generated name is derived from the first user message + date.
// The on-disk filename is a truncated SHA-256 of the name so sessions are opaque on the
// filesystem; the human-readable name is stored inside the JSON for listing and lookup.
// Returns "SAVE_SESSION_OK:<name>" on success (parsed by the TUI), or an error string.
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
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return fmt.Sprintf("Cannot create sessions directory: %v", err)
	}

	// Auto-generate name from first user message when none supplied.
	if strings.TrimSpace(name) == "" {
		name = o.suggestSessionName()
	}

	msgs := o.ConversationHistory.GetMessages()
	if _, err := session.SaveSessionByName(sessionDir, name, msgs); err != nil {
		return fmt.Sprintf("Save failed: %v", err)
	}
	return fmt.Sprintf("SAVE_SESSION_OK:%s", name)
}

// suggestSessionName derives a kebab-case session name from the first user message + date.
func (o *Orchestrator) suggestSessionName() string {
	date := time.Now().Format("2006-01-02")
	if o.ConversationHistory == nil {
		return "session-" + date
	}
	for _, m := range o.ConversationHistory.GetMessages() {
		if m.Role != "user" {
			continue
		}
		// Take the first 40 chars of the first user message, kebab-case it.
		text := strings.ToLower(m.Content)
		text = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return '-'
		}, text)
		// Collapse repeated dashes and trim
		for strings.Contains(text, "--") {
			text = strings.ReplaceAll(text, "--", "-")
		}
		text = strings.Trim(text, "-")
		if len(text) > 40 {
			text = text[:40]
		}
		text = strings.TrimRight(text, "-")
		if text == "" {
			break
		}
		return text + "-" + date
	}
	return "session-" + date
}

// LoadSession imports a named session file and replaces the current conversation history.
// Looks up the session by its human-readable name (scans the sessions directory).
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
	sessionDir := filepath.Join(env.ConfigDir, "sessions")
	path := session.FindSessionFile(sessionDir, name)
	if path == "" {
		return fmt.Sprintf("Session '%s' not found. Use /resume to list available sessions.", name)
	}
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

// ListSessions returns a formatted table of saved sessions with human-readable names.
func (o *Orchestrator) ListSessions() string {
	env, err := platform.GetEnvConfig()
	if err != nil {
		return fmt.Sprintf("Failed to get environment config: %v", err)
	}
	if env == nil {
		return "Environment config is nil"
	}
	metas := session.ListSessionMetas(filepath.Join(env.ConfigDir, "sessions"))
	if len(metas) == 0 {
		return "No saved sessions. Use `/save` to save the current session."
	}
	var sb strings.Builder
	sb.WriteString("**Saved Sessions** (use `/resume <name>` to restore):\n\n")
	for _, m := range metas {
		date := m.SavedAt
		if t, err := time.Parse(time.RFC3339, m.SavedAt); err == nil {
			date = t.Format("2006-01-02 15:04")
		}
		sb.WriteString(fmt.Sprintf("• **%s** — %s\n", m.Name, date))
	}
	return sb.String()
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

	if o.PersistStore != nil && o.SessionID != "" {
		_ = o.PersistStore.SaveSessionContext(context.Background(), o.SessionID, summary, 3600)
	}

	result := fmt.Sprintf("🗜️  **Context Compression**\n\nCompressing conversation history...\n• Before: %s tokens\n• After: %s tokens\n• Saved: %d%%\n\n✅ Context compressed successfully",
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

// ClearHistory clears the conversation history and flushes the tool cache
// so stale read results don't bleed into the fresh session.
func (o *Orchestrator) ClearHistory() {
	if o.ConversationHistory != nil {
		o.ConversationHistory.Clear()
		o.Logger.Info("Conversation history cleared")
	}
	if o.ToolCache != nil {
		o.ToolCache.Flush()
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
func (o *Orchestrator) AnalyzeAgency(ctx context.Context, lastResponse string) (injected bool, err error) {
	// Recover from any panic so a quality-reflection bug never kills the process.
	defer func() {
		if r := recover(); r != nil {
			o.Logger.Error("AnalyzeAgency panic recovered — SENSE reflection skipped",
				"recover", r)
			injected = false
			err = fmt.Errorf("agency analysis panicked: %v", r)
		}
	}()

	o.Logger.Info("Running SENSE Agency Reflection...")

	// ── 0. Empty/Lazy Response Check ─────────────────────────────────────────
	trimmed := strings.TrimSpace(lastResponse)
	// Only enforce length if the history has more than 2 messages (not a simple greeting)
	// and if the response is actually empty or a single word.
	if len(trimmed) < 5 && o.ConversationHistory.Count() > 2 {
		o.Logger.Warn("Empty or near-empty response detected", "length", len(trimmed))
		advice := "Your last response was empty. If you are still working, please provide a status update to the user. If you are finished, provide the final answer."
		o.ConversationHistory.AddSystemMessage("\n[SENSE AUTO-INTERVENTION]: " + advice)
		return true, nil
	}

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

// ExecuteTaskWithHistory temporarily swaps the orchestrator's ConversationHistory
// with the provided one for the duration of the call, enabling per-user channel
// isolation (e.g. Discord / Telegram users each get their own history).
// Thread-safe: the swap is protected by a mutex so concurrent TUI sessions are
// not affected.
func (o *Orchestrator) ExecuteTaskWithHistory(ctx context.Context, prompt string, history *ai.ConversationHistory) (string, error) {
	if history == nil {
		return o.ExecuteTask(ctx, prompt)
	}
	// Swap history for this call.
	prev := o.ConversationHistory
	o.ConversationHistory = history
	result, err := o.ExecuteTask(ctx, prompt)
	o.ConversationHistory = prev
	return result, err
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
	var arcDecision adaptive.RouteDecision
	if o.Intelligence != nil {
		arcDecision = o.Intelligence.Route(prompt)
		o.Logger.Info("ARC routing decision",
			"workflow", arcDecision.Classification.String(),
			"platform", o.Intelligence.Router.PlatformName(),
			"max_tool_calls", arcDecision.Budget.MaxToolCalls,
			"temperature", arcDecision.Budget.Temperature)

		// Auto-initialize SecurityCtx for SecurityCritical workflow tasks.
		if arcDecision.Classification == adaptive.WorkflowSecurityCritical && o.SecurityCtx == nil {
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

		// ── CCI Tier 1 + Tier 2 + Truth Sentry (always prepended) ──────────
		// CCI hot memory, optional specialist, and drift warnings are injected
		// at the head of every new session's system prompt.
		if cciPrefix := o.cciPrefixForSystemMessage(prompt); cciPrefix != "" {
			if toolContext != "" {
				toolContext = cciPrefix + "\n\n" + toolContext
			} else {
				toolContext = cciPrefix
			}
			o.Logger.Info("CCI context injected",
				"tier1_len", len(cciPrefix))
		}

		// ── Three-tier context injection (oh-my-opencode inspired) ──────────
		// Auto-inject GORKBOT.md hierarchy + README.md + .gorkbot/rules/*.md
		// from the current working directory and its parents.
		if o.ContextInjector != nil {
			injected := o.ContextInjector.Collect("")
			if injected.SystemPromptPrefix != "" {
				o.Logger.Info("Context injection active",
					"sources", len(injected.Sources),
					"bytes", injected.TotalBytes,
				)
				if toolContext != "" {
					toolContext = injected.SystemPromptPrefix + "\n\n" + toolContext
				} else {
					toolContext = injected.SystemPromptPrefix
				}
			}
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

	// Auto-title the session from the first user message (fire-and-forget).
	go o.AutoTitleSession(ctx, prompt)

	// Ensure history doesn't exceed context limit (use 80% of Grok's 128k context)
	maxContextTokens := 100000
	o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)
	o.ConversationHistory.RepairOrphanedPairs()

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

		// ── Pre-send: tiered context enforcement ─────────────────────────────
		// Truncate oversized tool results (Stage 1) and, if still over the
		// hard threshold, synchronously SENSE-compress (Stage 2) before the
		// token budget is handed to the provider.
		if o.TieredCompactor != nil {
			// Keep CompressionPipe in sync in case it was wired after InitEnhancements.
			if o.TieredCompactor.pipe == nil && o.CompressionPipe != nil {
				o.TieredCompactor.pipe = o.CompressionPipe
			}
			if err := o.TieredCompactor.Check(ctx, o.ConversationHistory); err != nil {
				o.Logger.Warn("tiered compaction error (non-fatal)", "err", err)
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
		o.ConversationHistory.RepairOrphanedPairs()
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
	sb.WriteString("9. **Self-Knowledge Grounding (CRITICAL)**: You CANNOT know your own runtime state from training data. Before making ANY claim about the current date/time, your version, which providers or models are active, whether semantic embedding is running, build variant, or any other live system fact — you MUST call `gorkbot_status` first. Never assert these facts from memory or inference. Treat unverified self-knowledge as hallucination.\n\n")

	sb.WriteString("### TOOL USAGE SOP (STRICT ENFORCEMENT):\n")
	sb.WriteString("1. **CHECK AVAILABLE TOOLS FIRST**: Before even considering creating a new tool, you MUST exhaustively review the 'AVAILABLE TOOLS' list below.\n")
	sb.WriteString("2. **NO DUPLICATES**: Do NOT create a tool if a similar one exists. For example, do not create `read_file_content` if `read_file` exists. Do not create `run_bash` if `bash` exists.\n")
	sb.WriteString("3. **REUSE**: If an existing tool can perform the task (e.g., using `bash` to run a custom script), USE IT instead of creating a new dedicated tool.\n")
	sb.WriteString("4. **JUSTIFY CREATION**: You may ONLY create a new tool if strictly necessary and NO existing tool can accomplish the goal. In your reasoning, you must explicitly state: 'I checked the tool list and tool X is missing/insufficient because...'.\n\n")

	// Inject environment snapshot so the AI knows exactly what is installed
	// before it sees the tool list.  This prevents wasted tool calls against
	// missing binaries, missing API keys, or failed MCP servers.
	if o.EnvContext != "" {
		sb.WriteString(o.EnvContext)
		sb.WriteString("\n")
	}

	// Inject live MCP server awareness so the AI understands what MCP tools
	// are available, when to use them, and what credentials they need.
	// Populated by main.go after mcpMgr.LoadAndStart() completes.
	if o.MCPContext != "" {
		sb.WriteString(o.MCPContext)
		sb.WriteString("\n")
	}

	// NOTE: Per-session memory context (AgeMem, Engrams, MEL heuristics) is
	// injected dynamically via buildMemoryContext + UpsertSystemMessage on
	// every turn in ExecuteTaskWithTools — not here — so it stays query-relevant.

	// Inject Dynamic Brain System (SOUL, IDENTITY, USER, MEMORY)
	// Scan each brain file for prompt injection before injecting.
	{
		brainCtx := GetDynamicBrainContext()
		if brainCtx != "" && o.InputSanitizer != nil {
			clean, blocked, _ := o.InputSanitizer.ScanContextContent(brainCtx, "brain/")
			if !blocked {
				sb.WriteString(clean)
			}
			// If blocked (>25% redacted), omit entirely — protects against poisoned brain files.
		} else {
			sb.WriteString(brainCtx)
		}
	}

	// Inject GORKBOT.md project instructions if configured.
	if o.ConfigLoader != nil {
		projectInstructions := o.ConfigLoader.LoadInstructions()
		if projectInstructions != "" {
			if o.InputSanitizer != nil {
				clean, blocked, _ := o.InputSanitizer.ScanContextContent(projectInstructions, "GORKBOT.md")
				if !blocked {
					sb.WriteString(clean)
				}
			} else {
				sb.WriteString(projectInstructions)
			}
		}
	}

	// Inject available skills index (name + description only — ~50 chars/skill).
	// Full skill content is loaded on demand via skill_view tool.
	if o.SkillLoader != nil {
		skillDefs := o.SkillLoader.LoadAll()
		if len(skillDefs) > 0 {
			sb.WriteString("\n<available_skills>\n")
			for _, sd := range skillDefs {
				sb.WriteString("• ")
				sb.WriteString(sd.Name)
				if sd.Description != "" {
					sb.WriteString(": ")
					desc := sd.Description
					if len(desc) > 80 {
						desc = desc[:77] + "..."
					}
					sb.WriteString(desc)
				}
				sb.WriteString("\n")
			}
			sb.WriteString("</available_skills>\n")
		}
	}

	// ### SELF-IMPROVEMENT DIRECTIVES (mandatory)
	sb.WriteString(`
### SELF-IMPROVEMENT DIRECTIVES (mandatory):

**FACT RECORDING**: When you discover something non-obvious — a tool quirk, environment
detail, configuration fact, or solution to a problem that took effort — call ` + "`record_fact`" + `
immediately. Do NOT wait to be asked. Future sessions reload this. Prefer ` + "`record_fact`" + `
over repeating the same discovery.

**USER MODELLING**: When the user expresses a preference, corrects an assumption, reveals
their workflow, or you observe a repeating pattern — call ` + "`record_user_pref`" + `. Build the
model incrementally. Never ask the user to re-explain something you've been told before.

**SKILL CREATION**: After completing a multi-step task (5+ tool calls, a complex workflow,
a tricky fix) — call ` + "`skill_create`" + ` with the procedure. Skills are reused across ALL future
sessions. Prefer skills for reusable procedures; prefer ` + "`record_fact`" + ` for one-off facts.

**SKILL SCANNING**: Before starting any task, scan ` + "`<available_skills>`" + ` above. If a skill
clearly matches (even partially), call ` + "`skill_view(name)`" + ` to load it before proceeding.
Following a skill prevents repeating prior mistakes.

**PAST RECALL**: When the user references prior work, or you suspect a past session holds
relevant context, call ` + "`session_search(query)`" + ` BEFORE asking the user to repeat themselves.
Search proactively — do not make the user remind you of your own history.

**SENSE SELF-CHECK**: After a tool fails unexpectedly, call ` + "`sense_check`" + ` to see if a
learned correction heuristic already covers this failure class. If not, use ` + "`sense_evolve`" + `
to create one.

**PRIVILEGE-AWARE EXECUTION**: Never embed ` + "`sudo`" + ` or ` + "`su -c`" + ` inside a ` + "`bash`" + ` command.
Instead, use ` + "`privileged_execute`" + ` — it probes the environment (root/su/sudo) and routes
automatically. The ` + "`Privilege`" + ` line in GORKBOT ENVIRONMENT above tells you what is available.

**STRUCTURED OUTPUT**: When you will inspect, filter, compare, or chain a command's output,
use ` + "`structured_bash`" + ` instead of ` + "`bash`" + `. It auto-parses stdout into typed JSON
(` + "`data_type`" + `: json/tabular/keyvalue/raw) with a 5 MB memory cap. Use ` + "`bash`" + ` for
fire-and-forget commands; use ` + "`structured_bash`" + ` when the result feeds into reasoning.

**USE SENSE EFFICIENTLY**: Always call ` + "`sense_discovery`" + ` with ` + "`compact=true`" + ` for routine
tool lookup — this omits full JSON schemas and keeps output ~1.5k tokens. Use
` + "`compact=false, category=X`" + ` only when you need the full parameter schema for a specific
category. Calling ` + "`sense_discovery`" + ` without ` + "`compact=true`" + ` adds ~22,000 tokens to context unnecessarily.

`)

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
	// Inject BackgroundSpawner so spawn_agent/collect_agent/list_agents tools work.
	if o.BackgroundAgents != nil {
		ctxWithOrchestrator = tools.BackgroundSpawnerToContext(ctxWithOrchestrator, o)
	}

	toolStart := time.Now()
	result, err := o.Registry.Execute(ctxWithOrchestrator, &req)
	toolElapsed := time.Since(toolStart)

	// ── Record execution statistics for progress estimation ──────────────────
	if o.ExecutionStats != nil {
		o.ExecutionStats.RecordExecution(req.ToolName, toolElapsed)
	}

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
					h := &adaptive.Heuristic{
						Context:     fmt.Sprintf("using %s tool", req.ToolName),
						Constraint:  "verify parameters carefully before execution",
						Error:       fmt.Sprintf("repeated failure (success rate: %.0f%%)", successRate*100),
						ContextTags: []string{req.ToolName, "reliability", "low_success"},
						Confidence:  0.5 + (0.5 - successRate), // higher confidence for lower rates
					}
					// MELValidator: screen for contamination before persisting.
					safe := true
					if o.MELValidator != nil {
						if res := o.MELValidator.Validate(h.Text()); !res.OK {
							o.Logger.Warn("MEL heuristic blocked", "reason", res.Reason, "tool", req.ToolName)
							safe = false
						}
					}
					if safe {
						o.Intelligence.Store.Add(h)
					}
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
		o.ToolCache.InvalidateOnMutation(req.ToolName, req.Parameters)
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

// ── BackgroundSpawner interface implementation ────────────────────────────────
// These methods satisfy tools.BackgroundSpawner so the Orchestrator can be
// injected via tools.BackgroundSpawnerToContext in ExecuteTool.

// SpawnFromTool starts a background sub-agent with the given label, prompt, and
// optional model ID. Returns the agent ID on success.
func (o *Orchestrator) SpawnFromTool(ctx context.Context, label, prompt, model string) (string, error) {
	if o.BackgroundAgents == nil {
		return "", fmt.Errorf("background agent manager not initialised")
	}
	provider := o.Primary
	if provider == nil {
		return "", fmt.Errorf("no primary AI provider available")
	}
	spec := BackgroundAgentSpec{
		Label:  label,
		Prompt: prompt,
		Model:  model,
	}
	id := o.BackgroundAgents.Spawn(ctx, spec, provider)
	return id, nil
}

// CollectFromTool waits (up to timeoutSec seconds) for the agent with the given
// ID to finish and returns its output.
func (o *Orchestrator) CollectFromTool(ctx context.Context, agentID string, timeoutSec int) (string, error) {
	if o.BackgroundAgents == nil {
		return "", fmt.Errorf("background agent manager not initialised")
	}
	collectCtx := ctx
	if timeoutSec > 0 {
		var cancel context.CancelFunc
		collectCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
	}
	return o.BackgroundAgents.Collect(collectCtx, agentID)
}

// ListRunningFromTool returns a snapshot of all pending/running background agents.
func (o *Orchestrator) ListRunningFromTool() []tools.BackgroundAgentInfo {
	if o.BackgroundAgents == nil {
		return nil
	}
	running := o.BackgroundAgents.List()
	out := make([]tools.BackgroundAgentInfo, 0, len(running))
	for _, a := range running {
		out = append(out, tools.BackgroundAgentInfo{
			ID:      a.ID,
			Label:   a.Label,
			Status:  a.Status.String(),
			Elapsed: a.Elapsed(),
		})
	}
	return out
}

// truncate returns the first n bytes of s (ASCII-safe).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// SaveTrajectory persists the current session as a ShareGPT-compatible JSONL
// trajectory file for potential fine-tuning use.
// Only saved when GORKBOT_SAVE_TRAJECTORIES=1 env var is set (default off).
// Quality filter: minimum 3 turns, success indicators present.
func (o *Orchestrator) SaveTrajectory(ctx context.Context, success bool) {
	if os.Getenv("GORKBOT_SAVE_TRAJECTORIES") != "1" {
		return
	}
	if o.SessionID == "" {
		return
	}

	msgs := o.ConversationHistory.GetMessages()
	// Count actual conversation turns (exclude system messages)
	turnCount := 0
	for _, m := range msgs {
		if m.Role != "system" {
			turnCount++
		}
	}
	if turnCount < 3 {
		return // too short to be useful
	}

	// Build ShareGPT format
	type convTurn struct {
		From  string `json:"from"`
		Value string `json:"value"`
	}
	type trajectory struct {
		Conversations []convTurn `json:"conversations"`
		Timestamp     string     `json:"timestamp"`
		Model         string     `json:"model"`
		TurnCount     int        `json:"turn_count"`
		Completed     bool       `json:"completed"`
		SessionID     string     `json:"session_id"`
	}

	var convs []convTurn
	for _, m := range msgs {
		switch m.Role {
		case "user":
			convs = append(convs, convTurn{From: "human", Value: m.Content})
		case "assistant":
			if m.Content != "" {
				convs = append(convs, convTurn{From: "gpt", Value: m.Content})
			}
		}
	}

	traj := trajectory{
		Conversations: convs,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Model:         o.primaryModelName,
		TurnCount:     turnCount,
		Completed:     success,
		SessionID:     o.SessionID,
	}

	// Write to ~/.config/gorkbot/trajectories/YYYY-MM/
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".config", "gorkbot", "trajectories", time.Now().Format("2006-01"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}

	prefix := "success"
	if !success {
		prefix = "partial"
	}
	sessionPrefix := o.SessionID
	if len(sessionPrefix) > 8 {
		sessionPrefix = sessionPrefix[:8]
	}
	outPath := filepath.Join(dir, fmt.Sprintf("%s-%s.jsonl", prefix, sessionPrefix))

	data, err := json.Marshal(traj)
	if err != nil {
		return
	}
	// Fire-and-forget
	go func() {
		_ = os.WriteFile(outPath, append(data, '\n'), 0600)
	}()
}

// SampleOnce calls the Primary AI provider with a single-turn prompt and returns
// the response text. It is used by the MCP sampling protocol handler to allow
// MCP servers to request LLM inference without importing the engine package.
func (o *Orchestrator) SampleOnce(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	if o.Primary == nil {
		return "", fmt.Errorf("no primary provider configured")
	}
	history := ai.NewConversationHistory()
	if systemPrompt != "" {
		history.AddSystemMessage(systemPrompt)
	}
	history.AddUserMessage(userPrompt)
	return o.Primary.GenerateWithHistory(ctx, history)
}

// AutoTitleSession generates and persists a human-readable title for the session
// based on the first user message. Called once after the first complete turn.
// Uses a heuristic (no API call) for simple messages; falls back to a formatted
// truncation for longer inputs.
func (o *Orchestrator) AutoTitleSession(ctx context.Context, firstUserMessage string) {
	if o.PersistStore == nil || o.SessionID == "" {
		return
	}
	// Check if title already set (only title once per session).
	if existing, err := o.PersistStore.GetSessionTitle(ctx, o.SessionID); err == nil && existing != "" {
		return
	}

	stopWords := map[string]bool{
		"can": true, "you": true, "please": true, "help": true, "me": true,
		"i": true, "want": true, "need": true, "could": true, "would": true,
		"a": true, "an": true, "the": true, "to": true, "how": true, "do": true,
		"is": true, "are": true, "my": true, "with": true, "for": true, "and": true,
		"in": true, "of": true, "it": true, "be": true, "that": true, "this": true,
	}

	words := strings.Fields(strings.ToLower(firstUserMessage))
	var kept []string
	for _, w := range words {
		// Strip trailing punctuation
		w = strings.TrimRight(w, ".,!?;:")
		if !stopWords[w] && len(w) > 1 {
			// Title-case
			kept = append(kept, strings.ToUpper(w[:1])+w[1:])
		}
		if len(kept) >= 7 {
			break
		}
	}

	title := strings.Join(kept, " ")
	if len(title) == 0 {
		// Fallback: first 60 chars of message
		title = firstUserMessage
		if len(title) > 60 {
			title = title[:57] + "..."
		}
	}

	_ = o.PersistStore.SetSessionTitle(ctx, o.SessionID, title)
}
