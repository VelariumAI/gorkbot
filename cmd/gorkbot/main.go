// Gorkbot — Multi-model AI orchestration CLI
//
// Lead Designer & Engineer: Todd Eddings
// Parent Entity:            Velarium AI
// Contact:                  velarium.ai@gmail.com
//
// Gorkbot is an independent open-source project and is NOT affiliated with
// xAI (makers of Grok), Google (makers of Gemini), or any other AI provider.
// It works with any OpenAI-compatible API endpoint, giving users freedom to
// harness multiple AI models from a single, unified TUI.
//
// All credit for concept, design, and engineering goes to Todd Eddings.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/internal/engine/consultation"
	"github.com/velariumai/gorkbot/internal/inline"
	"github.com/velariumai/gorkbot/internal/llm"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/internal/tui"
	"github.com/velariumai/gorkbot/orchestrator"
	"github.com/velariumai/gorkbot/pkg/a2a"
	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/billing"
	gorkenv "github.com/velariumai/gorkbot/pkg/env"
	"github.com/velariumai/gorkbot/pkg/channels/bridge"
	"github.com/velariumai/gorkbot/pkg/channels/discord"
	"github.com/velariumai/gorkbot/pkg/channels/telegram"
	"github.com/velariumai/gorkbot/pkg/collab"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/discovery"
	"github.com/velariumai/gorkbot/pkg/memory"
	"github.com/velariumai/gorkbot/pkg/persist"
	"github.com/velariumai/gorkbot/pkg/process"
	"github.com/velariumai/gorkbot/pkg/providers"
	"github.com/velariumai/gorkbot/pkg/registry"
	"github.com/velariumai/gorkbot/pkg/router"
	"github.com/velariumai/gorkbot/pkg/scheduler"
	"github.com/velariumai/gorkbot/pkg/schema"
	"github.com/velariumai/gorkbot/pkg/security"
	"github.com/velariumai/gorkbot/pkg/auth"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/skills"
	"github.com/velariumai/gorkbot/pkg/subagents"
	"github.com/velariumai/gorkbot/pkg/theme"
	"github.com/velariumai/gorkbot/pkg/tools"
	"github.com/velariumai/gorkbot/pkg/tui/hotkeys"
	"github.com/velariumai/gorkbot/pkg/usercommands"
	"github.com/velariumai/gorkbot/pkg/vectorstore"
	"github.com/velariumai/gorkbot/pkg/webhook"
)

// loadEnv loads environment variables from .env file, supporting encryption
func loadEnv(configDir string) {
	file, err := os.Open(".env")
	if err != nil {
		return // It's okay if .env doesn't exist
	}
	defer file.Close()

	km, _ := security.NewKeyManager(configDir)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Decrypt if it looks encrypted (starts with ENC_)
			if strings.HasPrefix(value, "ENC_") && km != nil {
				decrypted, err := km.Decrypt(strings.TrimPrefix(value, "ENC_"))
				if err == nil {
					value = decrypted
				}
			}

			// Only set if not already set (allow override)
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 1. Universal Environment Abstraction
	env, err := platform.GetEnvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Critical error detecting environment: %v\n", err)
		os.Exit(1)
	}

	// Load .env file if present, with decryption support
	loadEnv(env.ConfigDir)

	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup", "config", "configure":
			handleSetup(env.ConfigDir)
			return
		case "status":
			handleStatus()
			return
		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	// 2. Structured Logging (slog)
	logFile := filepath.Join(env.LogDir, "gorkbot.json")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	var handler slog.Handler
	if err == nil {
		handler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
		defer f.Close()
	} else {
		// Fallback to discarding logs if file creation fails to avoid breaking TUI
		handler = slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	logger.Info("Gorkbot initialized", "os", env.OS, "arch", env.Arch, "termux", env.IsTermux)

	// 3. CLI Configuration
	fs := flag.NewFlagSet("gorkbot", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fs.PrintDefaults()
	}

	promptFlag := fs.String("p", "", "Execute a single prompt and exit")
	stdinFlag := fs.Bool("stdin", false, "Read prompt from stdin (one-shot mode)")
	outputFlag := fs.String("output", "", "Write one-shot response to file instead of stdout")
	allowToolsFlag := fs.String("allow-tools", "", "Comma-separated list of tools allowed in one-shot mode")
	denyToolsFlag := fs.String("deny-tools", "", "Comma-separated list of tools denied in one-shot mode")
	timeoutFlag := fs.Duration("timeout", 60*time.Second, "Timeout for the operation")
	verboseThoughts := fs.Bool("verbose-thoughts", false, "Enable verbose output of consultant thinking")
	watchdogFlag := fs.Bool("watchdog", false, "Enable orchestrator watchdog for state debugging")
	traceFlag := fs.Bool("trace", false, "Write a JSONL execution trace to ~/.gorkbot/traces/")
	shareFlag := fs.Bool("share", false, "Start a relay server and share this session")
	joinFlag := fs.String("join", "", "Observe a shared session at host:port")
	a2aEnabled := fs.Bool("a2a", false, "Enable A2A HTTP gateway")
	a2aAddr := fs.String("a2a-addr", "127.0.0.1:18890", "A2A gateway listen address")
	describeFlag := fs.Bool("describe", false, "Output the machine-readable JSON schema of the CLI")
	outputFormatFlag := fs.String("output-format", "text", "Format for one-shot mode output (e.g. 'json' or 'text')")
	dryRunFlag := fs.Bool("dry-run", false, "Validate request and tools locally without executing mutations")
	inlineFlag := fs.Bool("inline", false, "Use the inline REPL instead of the TUI")
	irFlag := fs.Bool("ir", false, "Use the inline REPL instead of the TUI (alias for --inline)")

	// Pre-scan for --output-format=json to handle errors correctly
	isJSON := false
	for _, arg := range os.Args {
		if strings.Contains(arg, "output-format=json") || arg == "json" {
			isJSON = true
		}
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		if isJSON {
			outputErrorJSON(fmt.Sprintf("Invalid flags: %v", err))
			os.Exit(1)
		}
		// ContinueOnError returns err instead of exiting, so we exit manually if not JSON
		os.Exit(2)
	}

	// --join: observer-only mode — no orchestrator or TUI needed.
	if *joinFlag != "" {
		fmt.Printf("Connecting to session at %s ...\n", *joinFlag)
		err := collab.ObserveSession(*joinFlag, collab.ObserverCallbacks{
			OnToken: func(token string) { fmt.Print(token) },
			OnToolStart: func(toolName string) {
				fmt.Printf("\n[tool: %s ...]\n", toolName)
			},
			OnToolDone: func(toolName string) {
				fmt.Printf("[done: %s]\n", toolName)
			},
			OnComplete: func() { fmt.Println("\n--- turn complete ---") },
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Observer error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 4. Provider Setup & Dynamic Routing

	// 4.0 Initialize unified KeyStore + Provider Manager.
	// This replaces the previous per-provider env var reads and supports
	// all 5 providers: xAI, Google, Anthropic, OpenAI, MiniMax.
	keyStore := providers.NewKeyStore(env.ConfigDir)
	provMgr := providers.NewManager(keyStore, logger)
	provMgr.SetVerboseThoughts(*verboseThoughts)
	// Store as global singleton so orchestrator methods can access it.
	providers.SetGlobalProviderManager(provMgr)
	engine.SetProviderManager(provMgr)

	// Convenience vars for backward-compat with the router and discovery code below.
	grokKey, _ := keyStore.Get(providers.ProviderXAI)
	geminiKey, _ := keyStore.Get(providers.ProviderGoogle)

	// Read Model Overrides
	primaryOverride := os.Getenv("GORKBOT_PRIMARY_MODEL")
	consultantOverride := os.Getenv("GORKBOT_CONSULTANT_MODEL")

	if grokKey == "" {
		logger.Warn("XAI_API_KEY missing")
	}

	// Initialize Base Providers (Factories) via Manager (ensures they are WrappedProviders)
	baseGrok, _ := provMgr.GetBase(providers.ProviderXAI)
	if baseGrok == nil {
		baseGrok = ai.NewGrokProvider(grokKey, primaryOverride)
	}
	baseGemini, _ := provMgr.GetBase(providers.ProviderGoogle)
	if baseGemini == nil {
		baseGemini = ai.NewGeminiProvider(geminiKey, consultantOverride, *verboseThoughts)
	}

	// Initialize Registry
	reg := registry.NewModelRegistry(logger)
	startupCtx := context.Background()

	// Register Providers
	if grokKey != "" && baseGrok != nil {
		if err := reg.RegisterProvider(startupCtx, baseGrok); err != nil {
			logger.Error("Failed to register Grok provider", "error", err)
		}
	}
	if geminiKey != "" && baseGemini != nil {
		if err := reg.RegisterProvider(startupCtx, baseGemini); err != nil {
			logger.Error("Failed to register Gemini provider", "error", err)
		}
	}

	// Initialize Router
	r := router.NewRouter(reg, logger)

	// Load AppState early so provider bias can be applied before SelectSystemModels().
	appState := config.NewAppStateManager(env.ConfigDir)
	if init := appState.Get(); init.PrimaryProvider != "" {
		r.PrimaryBiasProvider = init.PrimaryProvider
	}
	if init := appState.Get(); init.SecondaryProvider != "" {
		r.ConsultantBiasProvider = init.SecondaryProvider
	}

	// Select System Models (Dynamic Configuration)
	var primary ai.AIProvider
	var consultant ai.AIProvider
	var sysConfig *router.SystemConfiguration // Declare here
	primaryModelName := "Primary AI (Default)"
	consultantModelName := ""

	// Check if user has explicit preferences via ENV vars first
	if primaryOverride != "" && consultantOverride != "" {
		logger.Info("Using explicit model overrides from environment",
			"primary", primaryOverride, "consultant", consultantOverride)

		primary = baseGrok.WithModel(primaryOverride)
		consultant = baseGemini.WithModel(consultantOverride)

		primaryModelName = primaryOverride
		consultantModelName = consultantOverride

		// Create a dummy sysConfig for display purposes if needed
		sysConfig = &router.SystemConfiguration{
			Reasoning: "Explicit environment variable override",
		}
	} else {
		// Attempt Dynamic Selection
		sysConfig, err = r.SelectSystemModels()
		if err == nil {
			logger.Info("Dynamic Model Selection Successful",
				"primary", sysConfig.PrimaryModel.ID,
				"specialist", sysConfig.SpecialistModel.ID,
				"reason", sysConfig.Reasoning)

			primaryModelName = sysConfig.PrimaryModel.Name
			consultantModelName = sysConfig.SpecialistModel.Name

			// Instantiate Primary
			if p, ok := reg.GetProvider(sysConfig.PrimaryModel.Provider); ok {
				if factory, ok := p.(ai.AIProvider); ok {
					primary = factory.WithModel(string(sysConfig.PrimaryModel.ID))
				}
			}

			// Instantiate Consultant
			if p, ok := reg.GetProvider(sysConfig.SpecialistModel.Provider); ok {
				if factory, ok := p.(ai.AIProvider); ok {
					consultant = factory.WithModel(string(sysConfig.SpecialistModel.ID))
				}
			}
		} else {
			logger.Warn("Dynamic Model Selection Failed - Falling back to defaults/overrides", "error", err)
		}
	}

	// Fallback / Default Initialization if dynamic failed or only partial overrides
	if primary == nil {
		if primaryOverride != "" {
			primary = baseGrok.WithModel(primaryOverride)
			primaryModelName = primaryOverride
		} else {
			primary = baseGrok
		}
	}
	if consultant == nil && geminiKey != "" {
		if consultantOverride != "" {
			consultant = baseGemini.WithModel(consultantOverride)
			consultantModelName = consultantOverride
		} else {
			consultant = baseGemini
			if consultantModelName == "" {
				consultantModelName = baseGemini.GetMetadata().Name
			}
		}
	}

	// 4.2 Initialize Process Manager (For Advanced Shell Execution)
	processManager := process.NewManager()

	// 4.5. Tool System Setup
	permissionMgr, err := tools.NewPermissionManager(env.ConfigDir)
	if err != nil {
		logger.Error("Failed to initialize permission manager", "error", err)
	}

	analytics, err := tools.NewAnalytics(env.ConfigDir)
	if err != nil {
		logger.Error("Failed to initialize analytics", "error", err)
	}

	// Structured SQLite audit log — records every tool execution asynchronously.
	// Stored at <configDir>/audit.db with schema defined in audit_db.go.
	auditPruneCtx, auditPruneCancel := context.WithCancel(context.Background())
	defer auditPruneCancel()
	auditDB, auditErr := tools.InitAuditDB(env.ConfigDir)
	if auditErr != nil {
		logger.Warn("Audit DB init failed — tool executions will not be logged to audit.db",
			"error", auditErr)
	}
	if auditDB != nil {
		// Start 12-hour background pruner; caps the DB at DefaultAuditMaxRecords rows.
		auditDB.StartPruner(auditPruneCtx, tools.DefaultAuditMaxRecords)
		defer auditDB.Close()
		logger.Info("Audit DB initialized", "path", filepath.Join(env.ConfigDir, "audit.db"))
	}

	toolRegistry := tools.NewRegistry(permissionMgr)
	toolRegistry.SetAnalytics(analytics)
	toolRegistry.SetAIProvider(primary)
	toolRegistry.SetConsultantProvider(consultant)
	toolRegistry.SetConfigDir(env.ConfigDir)
	if auditDB != nil {
		toolRegistry.SetAuditDB(auditDB)
	}

	// ── SENSE Stabilization Middleware ────────────────────────────────────────
	// Wire InputSanitizer: validates every tool call parameter before execution.
	// This enforces control-char rejection, path sandboxing, and resource-name
	// validation on ALL tool invocations — adversarial-by-default posture.
	senseInputSanitizer, sanitizerErr := sense.NewInputSanitizer()
	if sanitizerErr != nil {
		logger.Warn("SENSE input sanitizer init failed — proceeding without sanitization",
			"error", sanitizerErr)
	} else {
		toolRegistry.SetInputSanitizer(senseInputSanitizer)
		logger.Info("SENSE input sanitizer active", "cwd", senseInputSanitizer.CWD())
	}

	// ── SENSE Event Tracer ────────────────────────────────────────────────────
	// Wire SENSETracer: writes daily JSONL files to <configDir>/sense/traces/.
	// The trace analyzer (/self check) reads these files to classify failures.
	senseTraceDir := filepath.Join(env.ConfigDir, "sense", "traces")
	senseSessionID := fmt.Sprintf("%d", time.Now().Unix())
	senseTracer := sense.NewSENSETracer(senseTraceDir, senseSessionID)
	toolRegistry.SetSENSETracer(senseTracer)
	defer senseTracer.Close()
	logger.Info("SENSE tracer active", "trace_dir", senseTraceDir)
	// SENSETracer is also stored on the orchestrator (set after orch is created
	// below) so streaming can emit context-overflow and provider-error events.

	if err := toolRegistry.RegisterDefaultTools(); err != nil {
		logger.Error("Failed to register default tools", "error", err)
		os.Exit(1)
	}

	// Register PostNotifyTool (wired to Telegram/Discord backends later, after bots start).
	postNotifyTool := tools.NewPostNotifyTool(tools.NewNotificationRouter(nil, nil, "", 0))
	if err := toolRegistry.Register(postNotifyTool); err != nil {
		logger.Debug("post_notify already registered", "error", err)
	}

	// Register Process Tools
	if err := toolRegistry.Register(tools.NewStartManagedProcessTool(processManager)); err != nil {
		logger.Warn("Failed to register StartManagedProcessTool", "error", err)
	}
	if err := toolRegistry.Register(tools.NewListManagedProcessesTool(processManager)); err != nil {
		logger.Warn("Failed to register ListManagedProcessesTool", "error", err)
	}
	if err := toolRegistry.Register(tools.NewStopManagedProcessTool(processManager)); err != nil {
		logger.Warn("Failed to register StopManagedProcessTool", "error", err)
	}
	if err := toolRegistry.Register(tools.NewReadManagedProcessOutputTool(processManager)); err != nil {
		logger.Warn("Failed to register ReadManagedProcessOutputTool", "error", err)
	}

	// 4.6 Subagent System Setup
	subAgentManager := subagents.NewManager()

	// 4.6.1 Discovery Manager — polls all 5 providers for live model lists.
	discMgr := discovery.NewManagerWithKeys(keyStore, logger)
	discCtx := context.Background()
	discMgr.Start(discCtx)

	// Register subagent tools manually (they were moved out of default tools to break dependency cycle)
	if err := toolRegistry.Register(subagents.NewSpawnAgentTool(subAgentManager, toolRegistry)); err != nil {
		logger.Warn("Failed to register SpawnAgentTool", "error", err)
	}
	if err := toolRegistry.Register(subagents.NewCheckAgentStatusTool(subAgentManager)); err != nil {
		logger.Warn("Failed to register CheckAgentStatusTool", "error", err)
	}
	if err := toolRegistry.Register(subagents.NewListAgentsTool(subAgentManager)); err != nil {
		logger.Warn("Failed to register ListAgentsTool", "error", err)
	}
	if err := toolRegistry.Register(subagents.NewCollectAgentTool(subAgentManager)); err != nil {
		logger.Warn("Failed to register CollectAgentTool", "error", err)
	}
	// Discovery-aware recursive delegation tool.
	if err := toolRegistry.Register(subagents.NewSpawnSubAgentTool(subAgentManager, toolRegistry, discMgr)); err != nil {
		logger.Warn("Failed to register SpawnSubAgentTool", "error", err)
	}

	// Load any dynamic tools the agent previously created (no restart needed)
	if err := toolRegistry.LoadDynamicTools(env.ConfigDir); err != nil {
		logger.Warn("Failed to load dynamic tools", "error", err)
	}

	// Scheduler
	schedStore, err := scheduler.NewStore(env.ConfigDir)
	if err != nil {
		logger.Warn("Failed to init scheduler store", "err", err)
	}
	var sched *scheduler.Scheduler
	if schedStore != nil {
		sched = scheduler.NewScheduler(schedStore, nil, logger)
	}

	// User commands
	userCmdLoader, err := usercommands.NewLoader(env.ConfigDir)
	if err != nil {
		logger.Warn("Failed to init user command loader", "err", err)
		userCmdLoader = nil
	}

	// Wire scheduler and user command loader into tool registry
	if sched != nil {
		toolRegistry.SetScheduler(sched)
	}
	if userCmdLoader != nil {
		toolRegistry.SetUserCmdLoader(userCmdLoader)
	}

	logger.Info("Tool system initialized", "tool_count", len(toolRegistry.List()))

	if *describeFlag {
		fmt.Println(schema.GetSchema(toolRegistry.ListAll()))
		os.Exit(0)
	}

	// 5. Memory System Setup
	memMgr, err := memory.NewMemoryManager(env.ConfigDir, logger)
	if err != nil {
		logger.Error("Failed to initialize memory manager", "error", err)
	} else {
		if _, err := memMgr.LoadDefaultSession(); err != nil {
			logger.Error("Failed to load session", "error", err)
		}
	}

	// 6. Orchestration Engine
	orch := engine.NewOrchestrator(primary, consultant, toolRegistry, logger, *watchdogFlag)

	// Wire SENSE tracer onto orchestrator so streaming can emit
	// context-overflow and provider-error events (complementing tool-layer tracing).
	orch.SENSETracer = senseTracer

	// 6.0 Set config dir on orchestrator for daily memory logs.
	orch.SetConfigDir(env.ConfigDir)

	// 6.0b Wire context stats reporter so the context_stats tool can query live usage.
	if orch.ContextMgr != nil {
		toolRegistry.SetContextStatsReporter(orch.ContextMgr)
	}

	// 6.0c Wire introspection reporter so the query_* tools can surface intelligence state.
	toolRegistry.SetIntrospectionReporter(orch)

	// 6.0a Billing manager — per-model cost tracking persisted to usage_history.jsonl.
	billingMgr := billing.NewBillingManagerWithDir(env.ConfigDir)
	orch.Billing = billingMgr
	defer billingMgr.SaveDailyLog()

	// 6.1 SENSE AgeMem + Engram Store
	if err := orch.InitSENSEMemory(env.ConfigDir); err != nil {
		logger.Warn("SENSE AgeMem init failed", "error", err)
	} else {
		logger.Info("SENSE AgeMem initialised", "data_dir", env.ConfigDir)
	}

	// 6.2 Enhanced systems (P0/P1/P2): hooks, config, rules, checkpoints
	cwd, _ := os.Getwd()
	orch.InitEnhancements(env.ConfigDir, cwd)
	orch.StartConfigWatcher(ctx)
	logger.Info("Enhanced systems initialized", "version", platform.Version)

	// 6.3 Intelligence layer (ARC Router + MEL Meta-Experience Learning)
	orch.InitIntelligence(env.ConfigDir)
	logger.Info("Intelligence layer initialized (ARC + MEL)")

	// 6.3a-companion: Initialise companion augmentation systems
	// (CacheAdvisor, IngressFilter, IngressGuard, MELValidator).
	// geminiKey is already resolved above; geminiModel from consultant if set.
	companionGeminiModel := ""
	if baseGemini != nil {
		companionGeminiModel = baseGemini.GetMetadata().ID
	}
	orch.InitCompanions(env.ConfigDir, geminiKey, companionGeminiModel)
	logger.Info("Companion systems initialized (CacheAdvisor + IngressFilter + MELValidator)")

	// 6.3a XSKILL continual-learning system (Phase 1 + Phase 2)
	if orch.InitXSkill(env.ConfigDir) {
		logger.Info("XSKILL continual-learning system initialized", "config_dir", env.ConfigDir)
	} else {
		logger.Warn("XSKILL: disabled at startup — no embedder available yet (will upgrade in initEmbedder if local model loads)")
	}

	// 6.3b-SPARK: SPARK autonomous reasoning daemon
	if orch.InitSPARK(env.ConfigDir) {
		orch.StartSPARK(ctx)
		logger.Info("SPARK: autonomous reasoning daemon started", "config_dir", env.ConfigDir)
	}

	// 6.3b Goal Ledger (prospective cross-session memory)
	if gl, err := memory.NewGoalLedger(env.ConfigDir); err != nil {
		logger.Warn("Goal ledger init failed", "error", err)
	} else {
		orch.GoalLedger = gl
		toolRegistry.SetGoalLedger(gl)
		logger.Info("Goal ledger initialized", "config_dir", env.ConfigDir)
	}

	// 6.3b.1 Wire security context brief into tool registry so redteam subagents get shared state
	toolRegistry.SetSecurityBriefFn(func() string {
		if orch.SecurityCtx != nil {
			return orch.SecurityCtx.FormatBrief()
		}
		return ""
	})

	// 6.3b.2 Wire pipeline runner — allows run_pipeline tool to execute agents synchronously
	toolRegistry.SetPipelineRunner(func(ctx context.Context, agentType, task string) (string, error) {
		agentReg := subAgentManager.GetRegistry()
		agent, ok := agentReg.Get(subagents.AgentType(agentType))
		if !ok {
			return "", fmt.Errorf("unknown agent type: %s", agentType)
		}
		if orch.Primary == nil {
			return "", fmt.Errorf("no primary AI provider available")
		}
		return agent.Execute(ctx, task, orch.Primary, toolRegistry)
	})

	// 6.3c Unified Memory — wraps AgeMem + Engrams + MEL
	orch.UnifiedMem = memory.NewUnifiedMemory(orch.AgeMem, orch.Engrams, func() *adaptive.VectorStore {
		if orch.Intelligence != nil {
			return orch.Intelligence.Store
		}
		return nil
	}())

	// 6.3a Colony debate — wire orchestrator primary provider as the runner
	toolRegistry.SetColonyRunner(func(ctx context.Context, sys, prompt string) (string, error) {
		if orch.Primary == nil {
			return "", fmt.Errorf("no primary provider")
		}
		combined := prompt
		if sys != "" {
			combined = sys + "\n\n" + prompt
		}
		return orch.Primary.Generate(ctx, combined)
	})
	logger.Info("Colony debate tool registered")

	// 6.3d Dual-Model Consultation Mediator (five-stage airlock).
	// VectorStore is nil here — the embedder loads asynchronously in initEmbedder.
	// SetVectorStore is called there once the store is ready; the mediator runs
	// in gracefully-degraded mode (lexical-only context, no engram cache) until then.
	consultMediator := consultation.NewMediator(consultation.MediatorConfig{
		Secondary:   orch.Consultant,
		VectorStore: nil, // patched in initEmbedder
		Embedder:    nil, // patched in initEmbedder
		AgeMem:      orch.AgeMem,
		WorkDir:     cwd,
		HAL:         orch.HAL,
		Logger:      logger,
	})
	consultTool := consultation.NewConsultTool(consultMediator, orch.ConversationHistory)
	consultTool.TruthInjector = orch.ConversationHistory.UpsertSystemMessage
	if err := toolRegistry.Register(consultTool); err != nil {
		logger.Warn("Failed to register consult_secondary tool", "err", err)
	}
	if err := toolRegistry.Register(consultation.NewContextHashTool(orch.ConversationHistory)); err != nil {
		logger.Warn("Failed to register compute_context_hash tool", "err", err)
	}
	logger.Info("Consultation mediator registered (degraded mode until embedder loads)")

	// 6.4 Trace logger (--trace flag)
	if *traceFlag {
		traceDir := filepath.Join(env.LogDir, "traces")
		tracer := engine.NewTraceLogger(traceDir, true)
		orch.SetTraceLogger(tracer)
		defer tracer.Close()
	}

	// 6.5 MCP server integration (Hybrid Resilient Architecture)
	// First-run: silently install a platform-appropriate mcp.json if none exists.
	mcpCfgPath := filepath.Join(env.ConfigDir, "mcp.json")
	if _, err := os.Stat(mcpCfgPath); os.IsNotExist(err) {
		if installErr := installDefaultMCPConfig(env.ConfigDir); installErr != nil {
			logger.Warn("Could not auto-install default mcp.json", "err", installErr)
		} else {
			logger.Info("Auto-installed default mcp.json", "path", mcpCfgPath)
		}
	}
	mcpMgr := orchestrator.NewManagedManager(env.ConfigDir, toolRegistry, logger)

	// 6.5a Google OAuth client — enables Google Drive sources in NotebookLM.
	// Created unconditionally; auth only triggers on first Drive source request.
	googleAuthClient, err := auth.NewGoogleClient(env.ConfigDir, auth.NotebookLMScopes(), logger)
	if err != nil {
		logger.Warn("Google auth client init failed (non-fatal)", "err", err)
		googleAuthClient = nil
	}
	if googleAuthClient != nil {
		// Register so MCP manager injects GOOGLE_ACCESS_TOKEN into NotebookLM subprocess.
		mcpMgr.RegisterTokenProvider("google", googleAuthClient)
	}

	// ── Environment probe (Phase 1 of 2) ────────────────────────────────────
	// Run before MCP startup so runtimes, packages, and API keys are known
	// before any server launch attempts.
	envProbe := gorkenv.NewEnvProbe(logger)
	probeCtx, probeCancel := context.WithTimeout(ctx, 10*time.Second)
	envProbe.Probe(probeCtx)
	probeCancel()
	orch.EnvContext = envProbe.BuildSystemContext()

	// MCP startup runs fully asynchronously so slow npm/pip operations never
	// block the TUI from appearing. MCPContext and EnvContext are patched in
	// once servers are ready; the first conversation turn that needs MCP tools
	// will already have them available.
	go func() {
		// Ensure mcp Python package first (pip install if absent).
		ensurePythonMCPPackage(logger)

		if n, err := mcpMgr.LoadAndStart(ctx); err != nil {
			logger.Warn("MCP manager failed to start some servers", "error", err)
		} else if n > 0 {
			logger.Info("Hybrid MCP tools registered", "servers", n)
			orch.MCPContext = mcpMgr.GetSystemContext()
		}

		// Phase 2: rebuild EnvContext with real MCP statuses.
		srvStatuses := mcpMgr.GetServerStatuses()
		mcpStatuses := make([]gorkenv.MCPServerStatus, 0, len(srvStatuses))
		for _, s := range srvStatuses {
			mcpStatuses = append(mcpStatuses, gorkenv.MCPServerStatus{
				Name:      s.Name,
				Running:   s.Running,
				ToolCount: s.ToolCount,
				Error:     s.Error,
			})
		}
		envProbe.SetMCPStatus(mcpStatuses)
		orch.EnvContext = envProbe.BuildSystemContext()
	}()
	// Wire env probe into tool registry for capability pre-flight checks.
	// Tools implementing CapabilityRequirer get their binary/package deps
	// validated before execution.
	toolRegistry.SetEnvSnapshot(envProbe)

	// ── v4.6.0 wiring ────────────────────────────────────────────────────────

	// Wire InputSanitizer into orchestrator for brain/GORKBOT.md context scanning.
	if senseInputSanitizer != nil {
		orch.InputSanitizer = senseInputSanitizer
	}

	// Wire skills loader into orchestrator for <available_skills> index injection.
	{
		cwd2, _ := os.Getwd()
		orchSkillLoader := skills.NewLoader(env.ConfigDir+"/skills", cwd2+"/.gorkbot/skills")
		orch.SkillLoader = orchSkillLoader
		// Register skill CRUD tools so the AI can create/patch/delete/view skills.
		skillTools := tools.NewSkillTools(orchSkillLoader)
		skillTools.Register(toolRegistry)
	}

	// Create shared session budget (100 turns across parent + all subagents).
	sessionBudget := subagents.NewSessionBudget(100)
	ctx = subagents.WithBudget(ctx, sessionBudget)
	logger.Info("Session budget created", "total_turns", 100)

	// 6.5 Theme manager
	themeMgr := theme.NewManager(env.ConfigDir)
	logger.Info("Theme loaded", "active", themeMgr.Active().Name)

	// 6.5a Restore saved model selections + tool group prefs from AppState.
	if appState.HasSavedModel() {
		saved := appState.Get()
		logger.Info("Restoring saved model selection",
			"primary_provider", saved.PrimaryProvider,
			"primary_model", saved.PrimaryModel,
			"secondary_auto", saved.SecondaryAuto,
		)
		if pm := providers.GetGlobalProviderManager(); pm != nil {
			if saved.PrimaryProvider != "" && saved.PrimaryModel != "" {
				if p, err := pm.GetProviderForModel(saved.PrimaryProvider, saved.PrimaryModel); err == nil {
					primary = p
					primaryModelName = saved.PrimaryModel
				} else {
					logger.Warn("Saved primary model unavailable, using default", "error", err)
				}
			}
			if saved.SecondaryAuto {
				consultant = nil
			} else if saved.SecondaryProvider != "" && saved.SecondaryModel != "" {
				if p, err := pm.GetProviderForModel(saved.SecondaryProvider, saved.SecondaryModel); err == nil {
					consultant = p
					consultantModelName = saved.SecondaryModel
				} else {
					logger.Warn("Saved secondary model unavailable", "error", err)
				}
			}
		}
		// Restore disabled tool categories
		for _, cat := range saved.DisabledCategories {
			toolRegistry.SetCategoryEnabled(tools.ToolCategory(cat), false)
		}
		// Restore session-disabled providers (user persisted these).
		if len(saved.DisabledProviders) > 0 {
			if pm := providers.GetGlobalProviderManager(); pm != nil {
				for _, id := range saved.DisabledProviders {
					pm.DisableForSession(id)
				}
			}
		}

		// Propagate restored selections to the already-initialised orchestrator and
		// tool registry. NewOrchestrator (and toolRegistry.SetConsultantProvider) ran
		// before app state was loaded and therefore hold pre-restore values.
		orch.Primary = primary
		toolRegistry.SetAIProvider(primary)
		orch.Consultant = consultant
		toolRegistry.SetConsultantProvider(consultant)

		// Restore cascade order.
		if len(saved.CascadeOrder) > 0 {
			orch.CascadeOrder = saved.CascadeOrder
		}
		// Restore pinned compression provider.
		if saved.CompressionProvider != "" {
			if pm := providers.GetGlobalProviderManager(); pm != nil {
				if p, err := pm.GetProviderForModel(saved.CompressionProvider, ""); err == nil {
					orch.SetCompressorGenerator(p)
				}
			}
		}

		// Restore sandbox state: if user explicitly disabled it, bypass sanitizer.
		if !appState.IsSandboxEnabled() {
			orch.ToggleSandbox()
			logger.Warn("SENSE sandbox disabled (restored from AppState)")
		}
	}

	if memMgr != nil {
		logger.Info("Memory manager active", "sessions_dir", env.ConfigDir)
	}

	// 6.5.0a A2A HTTP gateway (--a2a flag).
	if *a2aEnabled {
		a2aRunner := a2a.TaskRunnerFunc(func(ctx context.Context, prompt string) (string, error) {
			return orch.ExecuteTask(ctx, prompt)
		})
		a2aSrv := a2a.NewServer(*a2aAddr, "", a2aRunner, logger)
		a2aCtx, a2aCancel := context.WithCancel(context.Background())
		defer a2aCancel()
		go func() {
			if err := a2aSrv.Start(a2aCtx); err != nil {
				logger.Error("A2A gateway error", "err", err)
			}
		}()
		logger.Info("A2A HTTP gateway started", "addr", *a2aAddr)
	}

	// 6.5.0a Cross-channel bridge — declare variables here; init after persistStore is ready.
	// The Discord/Telegram closures below capture these by reference so they see
	// the initialized values when the first message arrives.
	var bridgeRegistry *bridge.Registry
	var sessionRouter *bridge.SessionRouter

	// 6.5.0b Telegram bot.
	tgMgr := telegram.NewManager(env.ConfigDir, logger)
	if tgMgr.IsEnabled() {
		tgHandler := func(ctx context.Context, userID int64, username, text string) (string, error) {
			if sessionRouter != nil {
				h := sessionRouter.GetHistory("telegram", fmt.Sprintf("%d", userID))
				return orch.ExecuteTaskWithHistory(ctx, text, h)
			}
			return orch.ExecuteTask(ctx, text)
		}
		tgErr := tgMgr.Start(tgHandler)
		if tgErr != nil {
			logger.Warn("Telegram bot failed to start", "err", tgErr)
		} else {
			defer tgMgr.Stop()
			// Wire streaming.
			tgMgr.SetStreamHandler(func(ctx context.Context, userID int64, username, text string, cb telegram.StreamCallback) error {
				return orch.ExecuteTaskWithStreaming(ctx, text, func(token string) { cb(token) }, nil, nil, nil, nil)
			})
			if bridgeRegistry != nil {
				tgMgr.SetBridgeRegistry(bridgeRegistry)
			}
		}
	}

	// 6.5.0c Discord bot — bidirectional communication bus.
	// Token is read from DISCORD_BOT_TOKEN env var (never stored on disk).
	discordMgr := discord.NewManager(env.ConfigDir, logger)
	if discordMgr.IsEnabled() {
		discordHandler := func(ctx context.Context, userID, username, text string) (string, error) {
			if sessionRouter != nil {
				h := sessionRouter.GetHistory("discord", userID)
				resp, err := orch.ExecuteTaskWithHistory(ctx, text, h)
				if err != nil && isContextOverflow(err) {
					sessionRouter.ClearHistory("discord", userID)
					resp, err = orch.ExecuteTaskWithHistory(ctx, text, sessionRouter.GetHistory("discord", userID))
				}
				return resp, err
			}
			resp, err := orch.ExecuteTask(ctx, text)
			if err != nil && isContextOverflow(err) {
				orch.ClearHistory()
				resp, err = orch.ExecuteTask(ctx, text)
			}
			return resp, err
		}
		dErr := discordMgr.Start(discordHandler)
		if dErr != nil {
			logger.Warn("Discord bot failed to start", "err", dErr)
		} else {
			defer discordMgr.Stop()
			// Wire streaming.
			discordMgr.SetStreamHandler(func(ctx context.Context, userID, username, text string, cb discord.StreamCallback) error {
				return orch.ExecuteTaskWithStreaming(ctx, text, func(token string) { cb(token) }, nil, nil, nil, nil)
			})
			if bridgeRegistry != nil {
				discordMgr.SetBridgeRegistry(bridgeRegistry)
			}
		}
	}
	// Register discord_send regardless — gives the agent the tool even when the
	// bot is not running (it will return a clear "not configured" error).
	if err := toolRegistry.Register(tools.NewDiscordSendTool(discordMgr)); err != nil {
		logger.Warn("Failed to register discord_send tool", "err", err)
	}

	// Wire PostNotifyTool backends now that both bots have started.
	// Read optional default destinations from env vars.
	{
		var tgSender tools.TelegramSender
		if tgMgr.IsEnabled() {
			tgSender = tgMgr
		}
		var discSender tools.DiscordSender
		if discordMgr.IsEnabled() {
			discSender = discordMgr
		}
		defaultDiscordChan := os.Getenv("POST_NOTIFY_DISCORD_CHAN")
		var defaultTgChat int64
		if chatStr := os.Getenv("POST_NOTIFY_TELEGRAM_CHAT"); chatStr != "" {
			if chatID, err := strconv.ParseInt(chatStr, 10, 64); err == nil {
				defaultTgChat = chatID
			}
		}
		if pnt, ok := toolRegistry.Get("post_notify"); ok {
			if cast, ok := pnt.(*tools.PostNotifyTool); ok {
				cast.SetRouter(tools.NewNotificationRouter(discSender, tgSender, defaultDiscordChan, defaultTgChat))
				logger.Info("post_notify wired", "discord", discordMgr.IsEnabled(), "telegram", tgMgr.IsEnabled())
			}
		}
	}

	// 6.5.0d Scheduler start with result dispatcher.
	if sched != nil && schedStore != nil {
		schedNotify := func(subject, body string) {
			msg := fmt.Sprintf("**%s**\n%s", subject, body)
			if discordMgr.IsEnabled() {
				if ch := os.Getenv("SCHEDULER_NOTIFY_DISCORD"); ch != "" {
					_ = discordMgr.SendToChannel(ch, msg)
				}
			}
			if tgMgr.IsEnabled() {
				if chatStr := os.Getenv("SCHEDULER_NOTIFY_TELEGRAM"); chatStr != "" {
					if chatID, err := strconv.ParseInt(chatStr, 10, 64); err == nil {
						_ = tgMgr.SendMessage(chatID, msg)
					}
				}
			}
		}
		dispatcher := scheduler.NewResultDispatcher(schedStore, orch.ExecuteTask, schedNotify, logger)
		sched = scheduler.NewScheduler(schedStore, dispatcher.Dispatch, logger)
		if sched != nil {
			toolRegistry.SetScheduler(sched)
		}
		schedCtx, schedCancel := context.WithCancel(context.Background())
		defer schedCancel()
		sched.Start(schedCtx)
	}

	// 6.5.0e Webhook ingestion server.
	if port := os.Getenv("WEBHOOK_PORT"); port != "" {
		webhookNotify := func(result string) {
			if ch := os.Getenv("WEBHOOK_NOTIFY_DISCORD"); ch != "" && discordMgr.IsEnabled() {
				_ = discordMgr.SendToChannel(ch, result)
			}
			if chatStr := os.Getenv("WEBHOOK_NOTIFY_TELEGRAM"); chatStr != "" && tgMgr.IsEnabled() {
				if chatID, err := strconv.ParseInt(chatStr, 10, 64); err == nil {
					_ = tgMgr.SendMessage(chatID, result)
				}
			}
		}
		wh := webhook.NewWebhookServer(
			":"+port,
			os.Getenv("WEBHOOK_SECRET"),
			func(ctx context.Context, src, evt, prompt string) (string, error) {
				return orch.ExecuteTask(ctx, prompt)
			},
			webhookNotify,
			logger,
		)
		webhookCtx, webhookCancel := context.WithCancel(context.Background())
		defer webhookCancel()
		if err := wh.Start(webhookCtx); err != nil {
			logger.Warn("Webhook server failed to start", "err", err)
		} else {
			logger.Info("Webhook server started", "port", port)
		}
	}

	// 6.5.1 Remote Session Sharing (--share).
	if *shareFlag {
		relay := collab.NewRelay(0)
		url, err := relay.Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start relay: %v\n", err)
			os.Exit(1)
		}
		orch.Relay = relay
		defer relay.Stop()
		fmt.Printf("\n┌──────────────────────────────────────────────────┐\n")
		fmt.Printf("│  Session sharing active                          │\n")
		fmt.Printf("│  Observer URL: %-35s│\n", url)
		fmt.Printf("│  Connect with: gorkbot --join localhost:%-10d│\n", relay.Port())
		fmt.Printf("└──────────────────────────────────────────────────┘\n\n")
	}

	// 6.5.2 Feedback manager — persists adaptive routing outcomes to disk.
	orch.Feedback = router.NewFeedbackManager(env.ConfigDir, logger)
	defer orch.Feedback.Close()

	// 6.5.3 Wire discovery manager into orchestrator (used by spawn_sub_agent + Cloud Brains tab).
	orch.Discovery = discMgr

	// 6.5.4 SQLite persistence store — conversation history + tool call analytics.
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	persistStore, err := persist.NewStore(env.ConfigDir, sessionID)
	if err != nil {
		logger.Warn("SQLite persist store failed to init", "err", err)
		persistStore = nil
	}
	if persistStore != nil {
		defer persistStore.Close()
		orch.ToolCache.SetDB(persistStore.DB())
	}

	orch.PersistStore = persistStore
	orch.SessionID = sessionID

	// Wire persist store into tool registry now that it is initialized.
	if persistStore != nil {
		toolRegistry.SetPersistStore(persistStore)
	}

	// 6.5.4b  Cross-channel bridge (requires persistStore).
	if persistStore != nil {
		var bridgeErr error
		bridgeRegistry, bridgeErr = bridge.NewRegistry(persistStore.DB())
		if bridgeErr != nil {
			logger.Warn("channel bridge registry failed", "err", bridgeErr)
		} else {
			sessionRouter = bridge.NewSessionRouter(bridgeRegistry, persistStore)
			logger.Info("Cross-channel bridge initialized")
			// Wire registry into already-started bots (no-op if bots not running).
			discordMgr.SetBridgeRegistry(bridgeRegistry)
			tgMgr.SetBridgeRegistry(bridgeRegistry)
		}
	}

	// 6.5.4c  Context compression pipeline.
	if orch.Compressor != nil && persistStore != nil {
		orch.CompressionPipe = engine.NewCompressionPipe(orch.Compressor, persistStore, sessionID, logger)
		logger.Info("Context compression pipeline initialized")
	}

	// 6.5.4d  Budget guard (session/daily USD limits).
	{
		sessionLimit, _ := strconv.ParseFloat(os.Getenv("BUDGET_SESSION_USD"), 64)
		dailyLimit, _ := strconv.ParseFloat(os.Getenv("BUDGET_DAILY_USD"), 64)
		if sessionLimit > 0 || dailyLimit > 0 {
			orch.BudgetGuard = engine.NewBudgetGuard(orch.Billing, sessionLimit, dailyLimit)
			logger.Info("Budget guard active", "session_limit", sessionLimit, "daily_limit", dailyLimit)
		}
	}

	// 6.5.4e  Vector store schema init (embedder wired later in initEmbedder goroutine).
	if persistStore != nil {
		if err := vectorstore.InitSchema(persistStore.DB()); err != nil {
			logger.Warn("vector store schema init failed", "err", err)
		}
	}

	// 7. Execution Loop
	// ctx is the root cancellable context from main(); propagate it through.

	// 6.5.4a  Restore compressed context from the most recent prior session.
	// (Session IDs rotate each run, so we look up by recency, not by ID.)
	if persistStore != nil {
		if summary, ok, err := persistStore.GetLatestContext(ctx); err == nil && ok {
			orch.ConversationHistory.AddSystemMessage("## Restored Context (prior session)\n" + summary)
			logger.Info("Restored prior session context from SQLite")
		} else if err != nil {
			logger.Warn("persist: GetLatestContext", "err", err)
		}
		// Start background pruners for SQLite cache tables.
		orch.ToolCache.StartCachePruner(ctx)
		go func() {
			_ = persistStore.PruneExpiredContexts(ctx)
		}()
	}

	// Apply one-shot tool allow/deny lists
	if *allowToolsFlag != "" || *denyToolsFlag != "" {
		applyToolFilters(toolRegistry, *allowToolsFlag, *denyToolsFlag, logger)
	}

	// Determine one-shot prompt
	oneShotPrompt := *promptFlag
	if *stdinFlag && oneShotPrompt == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		oneShotPrompt = strings.TrimSpace(string(data))
	}

	if oneShotPrompt != "" {
		ctx, cancel := context.WithTimeout(ctx, *timeoutFlag)
		defer cancel()
		toolRegistry.DryRun = *dryRunFlag
		runOneShotTask(ctx, orch, oneShotPrompt, *outputFlag, *outputFormatFlag, toolRegistry)
	} else {
		// Load integration settings (BUDGET_*, WEBHOOK_*, SCHEDULER_*) from disk
		// and apply any stored values as env vars before starting the TUI.
		integCfg, _ := config.LoadIntegrations(env.ConfigDir)
		integCfg.Apply()

		// Initialize shared command registry
		cmdReg := commands.NewRegistry()

		extras := &tuiExtras{
			sched:           sched,
			tgMgr:           tgMgr,
			userCmdLoader:   userCmdLoader,
			billing:         billingMgr,
			persist:         persistStore,
			integrations:    integCfg,
			configDir:       env.ConfigDir,
			consultMediator: consultMediator,
			toolRegistry:    toolRegistry,
			dryRun:          *dryRunFlag,
			googleAuth:      googleAuthClient,
		}

		if *inlineFlag || *irFlag {
			// CRITICAL: Inline REPL mode bypasses TUI
			wireCommandRegistry(ctx, cmdReg, orch, extras, appState, themeMgr, mcpMgr, envProbe, googleAuthClient)
			inline.RunREPL(orch, cmdReg, env)
		} else {
			// Standard TUI mode (absolute default)
			runTUI(ctx, orch, processManager, reg, primaryModelName, consultantModelName, sysConfig, themeMgr, mcpMgr, appState,
				extras, cmdReg, envProbe, discMgr)
		}
	}

	// ── Post-session: handle any tools that need a Go rebuild ────────────────
	handlePendingRebuild(toolRegistry, env.ConfigDir)
}

// handlePendingRebuild checks whether any tools were created or modified in a
// way that requires recompiling Gorkbot.
func handlePendingRebuild(reg *tools.Registry, configDir string) {
	pending := reg.GetPendingRebuild()
	if len(pending) == 0 {
		return
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(os.Stderr, "  PENDING REBUILD — Tools requiring static integration")
	fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	for _, name := range pending {
		fmt.Fprintf(os.Stderr, "  • %s\n", name)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "These tools are ALREADY hot-loaded and will work on the")
	fmt.Fprintln(os.Stderr, "next startup via dynamic_tools.json — no action required")
	fmt.Fprintln(os.Stderr, "for normal use.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "To permanently compile them into the binary, run:")
	fmt.Fprintln(os.Stderr, "  go build -o gorkbot ./cmd/gorkbot/")
	fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Auto-rebuild if explicitly requested and 'go' is available.
	if os.Getenv("GORKBOT_AUTO_REBUILD") == "1" {
		fmt.Fprintln(os.Stderr, "\nGORKBOT_AUTO_REBUILD=1 detected — rebuilding now...")
		goBin, err := exec.LookPath("go")
		if err != nil {
			fmt.Fprintln(os.Stderr, "Auto-rebuild failed: 'go' not found on PATH.")
			return
		}
		projectRoot := "."
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				projectRoot = dir
			}
		}
		cmd := exec.Command(goBin, "build", "-o", filepath.Join(projectRoot, "gorkbot"), filepath.Join(projectRoot, "cmd/gorkbot"))
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Auto-rebuild failed: %v\n", err)
			return
		}
		fmt.Fprintln(os.Stderr, "Rebuild successful! Pending tools are now compiled in.")
		_ = reg.ClearPendingRebuild()
	}
}

// runOneShotTask executes a single prompt and writes output to stdout or a file.
func runOneShotTask(ctx context.Context, orch *engine.Orchestrator, prompt string, outputFile string, outputFormat string, reg *tools.Registry) {
	if err := security.ValidateInput(prompt); err != nil {
		if outputFormat == "json" {
			outputErrorJSON(err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Input validation error: %v\n", err)
		}
		os.Exit(1)
	}

	if outputFormat != "json" {
		fmt.Printf("> %s\n", prompt)
	}

	var toolCalls []map[string]interface{}
	toolCallback := func(name string, result *tools.ToolResult) {
		toolCalls = append(toolCalls, map[string]interface{}{
			"tool":    name,
			"success": result.Success,
			"output":  result.Output,
			"error":   result.Error,
		})
	}

	resp, err := orch.ExecuteTaskWithTools(ctx, prompt, toolCallback)

	if outputFormat == "json" {
		inTokens, outTokens := 0, 0
		if orch.ContextMgr != nil {
			inTokens, outTokens = orch.ContextMgr.TotalUsage()
		}

		outMap := map[string]interface{}{
			"response": resp,
			"tokens": map[string]int{
				"input":  inTokens,
				"output": outTokens,
			},
			"tools": toolCalls,
		}

		if err != nil {
			outMap["error"] = err.Error()
		}

		b, _ := json.MarshalIndent(outMap, "", "  ")
		if outputFile != "" {
			if wErr := os.WriteFile(outputFile, b, 0644); wErr != nil {
				fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", wErr)
			}
		} else {
			fmt.Println(string(b))
		}
		return
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, []byte(resp+"\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
		} else {
			fmt.Printf("Output written to: %s\n", outputFile)
		}
		return
	}
	fmt.Println(resp)
}

func outputErrorJSON(msg string) {
	out := map[string]interface{}{"error": msg}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}

// applyToolFilters configures the tool registry's session permissions based on
// --allow-tools and --deny-tools flags for one-shot mode.
func applyToolFilters(reg *tools.Registry, allowList, denyList string, logger *slog.Logger) {
	if denyList != "" {
		for _, name := range strings.Split(denyList, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			// Block by setting session perm to never via permission manager
			if pm := reg.GetPermissionManager(); pm != nil {
				if err := pm.SetPermission(name, tools.PermissionNever); err != nil {
					logger.Warn("Failed to deny tool", "tool", name, "error", err)
				} else {
					logger.Info("Tool denied (one-shot)", "tool", name)
				}
			}
		}
	}
	if allowList != "" {
		for _, name := range strings.Split(allowList, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if pm := reg.GetPermissionManager(); pm != nil {
				if err := pm.SetPermission(name, tools.PermissionAlways); err != nil {
					logger.Warn("Failed to allow tool", "tool", name, "error", err)
				} else {
					logger.Info("Tool allowed (one-shot)", "tool", name)
				}
			}
		}
	}
}

// tuiExtras carries optional subsystem pointers into runTUI without changing the variadic signature.
type tuiExtras struct {
	sched           *scheduler.Scheduler
	tgMgr           *telegram.Manager
	userCmdLoader   *usercommands.Loader
	billing         *billing.BillingManager
	persist         *persist.Store
	integrations    *config.IntegrationSettings
	configDir       string
	consultMediator *consultation.ConsultationMediator // upgraded in initEmbedder goroutine
	toolRegistry    *tools.Registry                    // for MCP reload re-registration
	dryRun          bool                               // global dry-run mode
	googleAuth      *auth.Client                       // Google OAuth for NotebookLM / Drive
}

// initEmbedder tries to load the local nomic embedding model and wire it into
// the Intelligence layer (ARC Router + MEL vector store) and the vector store
// for RAG retrieval. Non-fatal: if the build lacks llamacpp support or the
// model is absent, routing falls back to the heuristic classifier automatically.
func initEmbedder(orch *engine.Orchestrator, consultMed *consultation.ConsultationMediator) {
	modelsDir := filepath.Join(os.Getenv("HOME"), ".cache", "llama.cpp")
	modelPath, err := llm.EnsureEmbedModel(modelsDir)
	if err != nil {
		slog.Default().Warn("semantic embedder: model unavailable (using heuristic routing)", "error", err)
		return
	}
	slog.Default().Info("semantic embedder: loading", "path", modelPath)
	embedder, err := llm.NewLocalEmbedder(modelPath)
	if err != nil {
		slog.Default().Warn("semantic embedder: load failed (using heuristic routing)", "error", err)
		return
	}
	if orch.Intelligence != nil {
		// Use SetEmbedderWithProjection so both the ARC Router and MEL VectorStore
		// receive a RAM-aware projected embedder (128/256/full dims based on HAL).
		orch.Intelligence.SetEmbedderWithProjection(embedder, orch.HAL)
	}
	// Upgrade XSKILL to use the high-quality local model when available.
	orch.UpgradeXSkillEmbedder(embedder)
	// Wire embedder into the vector store and RAG injector.
	if orch.PersistStore != nil && orch.VectorStore == nil {
		vs := vectorstore.New(orch.PersistStore.DB(), embedder)
		if vs != nil {
			if initErr := vs.Init(orch.PersistStore.DB()); initErr == nil {
				orch.VectorStore = vs
				orch.RAGInjector = engine.NewRAGInjector(vs, 2000, slog.Default())
				slog.Default().Info("RAG injector initialized with semantic embedder")
				// Upgrade consultation mediator from degraded mode: inject live VectorStore
				// and Embedder so Stage 2 semantic search and Stage 3 engram cache activate.
				if consultMed != nil {
					consultMed.SetVectorStore(vs)
					consultMed.SetEmbedder(embedder)
					slog.Default().Info("consultation mediator: semantic capabilities activated")
				}
			}
		}
	}
	slog.Default().Info("semantic embedder: ready", "model", llm.EmbedModelFile)
}

func wireCommandRegistry(ctx context.Context, cmdReg *commands.Registry, orch *engine.Orchestrator, extras *tuiExtras, appState *config.AppStateManager, themeMgr *theme.Manager, mcpMgr *orchestrator.ManagedManager, envProbeInst *gorkenv.EnvProbe, googleAuth ...*auth.Client) {
	// Wire orchestrator adapter into command registry so new commands work.
	if cmdReg == nil {
		return
	}

	cmdReg.DryRun = extras.dryRun

	// Wire configDir so /self commands can locate trace and skills directories.
	if extras != nil && extras.configDir != "" {
		cmdReg.SetConfigDir(extras.configDir)
	}

	// Orchestrator adapter — bridges engine methods to command handlers.
	cmdReg.Orch = &commands.OrchestratorAdapter{
		GetContextReport: orch.GetContextReport,
		GetCostReport:    orch.GetCostReport,
		GetCheckpoints:   orch.GetCheckpointList,
		RewindTo:         orch.RewindTo,
		ExportConv:       orch.ExportConversation,
		CompactFocus: func(focus string) string {
			ctx := context.Background()
			return orch.CompactWithFocus(ctx, focus)
		},
		CycleMode:    orch.CycleMode,
		SetMode:      orch.SetMode,
		GetMode:      orch.GetMode,
		SaveSession:  orch.SaveSession,
		LoadSession:  orch.LoadSession,
		ListSessions: orch.ListSessions,
		RateResponse: func(score float64) string {
			if orch.Feedback == nil {
				return "Feedback manager not available."
			}
			modelID := ""
			if orch.Primary != nil {
				modelID = orch.Primary.GetMetadata().ID
			}
			// Normalize 1–5 → 0.0–1.0 and treat ≥3 as success.
			orch.Feedback.RecordOutcome(router.ClassifyQuery(""), modelID, (score-1)/4.0, score >= 3)
			return fmt.Sprintf("Response rated %.0f/5 — adaptive router updated.", score)
		},
		StartRelay: func() string {
			if orch.Relay != nil {
				return "" // already active
			}
			relay := collab.NewRelay(0)
			url, err := relay.Start()
			if err != nil {
				slog.Default().Error("Relay start failed", "error", err)
				return ""
			}
			orch.Relay = relay
			return url
		},
		StopRelay: func() {
			if orch.Relay != nil {
				orch.Relay.Stop()
				orch.Relay = nil
			}
		},
		ToggleDebug: func() bool {
			return orch.ToggleDebug()
		},
		SetPrimary: func(providerName, modelID string) string {
			ctx := context.Background()
			if err := orch.SetPrimary(ctx, providerName, modelID); err != nil {
				return fmt.Sprintf("Failed to switch primary: %v", err)
			}
			_ = appState.SetPrimary(providerName, modelID)
			return fmt.Sprintf("Primary switched to **%s** (%s)", modelID, providerName)
		},
		SetSecondary: func(providerName, modelID string) string {
			ctx := context.Background()
			if err := orch.SetSecondary(ctx, providerName, modelID); err != nil {
				return fmt.Sprintf("Failed to switch secondary: %v", err)
			}
			_ = appState.SetSecondary(providerName, modelID)
			return fmt.Sprintf("Secondary switched to **%s** (%s)", modelID, providerName)
		},
		SetAutoSecondary: func() string {
			orch.Consultant = nil
			// Clear registry cache so ResolveConsultantProvider auto-selects.
			if orch.Registry != nil {
				orch.Registry.SetConsultantProvider(nil)
			}
			_ = appState.SetSecondaryAuto()
			return "Secondary set to **Auto** — AI picks best model per task."
		},
		GetProviderStatus: func() string {
			return orch.GetProviderStatus()
		},
		SetProviderKey: func(providerName, key string) string {
			ctx := context.Background()
			return orch.SetProviderKey(ctx, providerName, key)
		},
		PersistDisabledCategories: func(cats []string) error {
			return appState.SetDisabledCategories(cats)
		},
		BillingGet: func() string {
			if extras != nil && extras.billing != nil {
				return extras.billing.GetSessionReport()
			}
			return "Billing not available."
		},
		BillingGetAllTime: func() string {
			if extras != nil && extras.billing != nil {
				return extras.billing.GetAllTimeReport()
			}
			return "No usage history."
		},
		GetToolCallStats: func() string {
			// Prefer persistent audit DB (all-time, accurate) over
			// the in-session tool_calls table which is rarely populated.
			if orch.Registry != nil {
				if adb := orch.Registry.GetAuditDB(); adb != nil {
					return adb.AuditSummary(25)
				}
			}
			// Fallback: SQLite persist store (session-scoped tool_calls).
			if extras != nil && extras.persist != nil {
				stats, err := extras.persist.ToolCallStats(context.Background())
				if err != nil {
					return "Stats unavailable: " + err.Error()
				}
				return stats
			}
			return "Audit DB not initialized."
		},
		GetDiagnosticReport: func() string {
			return orch.GetSystemState()
		},
		ToggleProvider: func(providerID string) (bool, string) {
			pm := engine.GetProviderManager()
			if pm == nil {
				return false, "provider manager unavailable"
			}
			if pm.IsSessionDisabled(providerID) {
				pm.EnableForSession(providerID)
				st := appState.Get()
				filtered := removeFromSlice(st.DisabledProviders, providerID)
				_ = appState.SetDisabledProviders(filtered)
				return true, providers.ProviderName(providerID) + " enabled"
			}
			pm.DisableForSession(providerID)
			st := appState.Get()
			_ = appState.SetDisabledProviders(append(st.DisabledProviders, providerID))
			return false, providers.ProviderName(providerID) + " disabled"
		},
		GetProviderEnabled: func() map[string]bool {
			pm := engine.GetProviderManager()
			result := map[string]bool{}
			for _, id := range providers.AllProviders() {
				result[id] = pm == nil || !pm.IsSessionDisabled(id)
			}
			return result
		},
		PersistDisabledProviders: func(ids []string) error {
			return appState.SetDisabledProviders(ids)
		},
		SetThinkingBudget: func(budget int) string {
			orch.ThinkingBudget = budget
			if budget == 0 {
				return "Extended thinking **disabled**."
			}
			return fmt.Sprintf("Extended thinking enabled — budget **%d tokens**.", budget)
		},
		ToggleSandbox: func() bool {
			enabled := orch.ToggleSandbox()
			_ = appState.SetSandboxEnabled(enabled)
			return enabled
		},
		SandboxEnabled: func() bool {
			return orch.SandboxEnabled()
		},
	}

	// Wire extras: scheduler, telegram, user-defined commands.
	if extras != nil {
		if extras.sched != nil {
			st := extras.sched
			cmdReg.Orch.GetScheduledTasks = func() string {
				tasks := st.Store().List()
				if len(tasks) == 0 {
					return "No scheduled tasks. Use the `schedule_task` tool to create one."
				}
				var sb strings.Builder
				sb.WriteString("# Scheduled Tasks\n\n")
				for _, t := range tasks {
					sb.WriteString(fmt.Sprintf("- **%s** (`%s`): %s — %s\n", t.Name, t.ID, t.ScheduleValue, string(t.Status)))
				}
				return sb.String()
			}
		}
		if extras.tgMgr != nil {
			tg := extras.tgMgr
			cmdReg.Orch.GetTelegramStatus = func() string {
				return fmt.Sprintf("**Telegram Bot Status**: %s", tg.Status())
			}
		}
		if extras.userCmdLoader != nil {
			ucl := extras.userCmdLoader
			cmdReg.UserCmdsGet = func(name, args string) (string, bool) {
				if name == "__list__" {
					list := ucl.List()
					if len(list) == 0 {
						return "", false
					}
					var sb strings.Builder
					sb.WriteString("# User-Defined Commands\n\n")
					for _, c := range list {
						sb.WriteString(fmt.Sprintf("- **/%s** — %s\n", c.Name, c.Description))
					}
					return sb.String(), true
				}
				return ucl.Get(name, args)
			}
		}
	}

	// Wire skills loader.
	cwd, _ := os.Getwd()
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = home + "/.config/gorkbot"
	}
	skillsLoader := skills.NewLoader(
		configDir+"/skills",
		cwd+"/.gorkbot/skills",
	)
	cmdReg.SkillsFormat = skillsLoader.Format
	cmdReg.SkillsGet = func(name, argsStr string) (string, bool) {
		def, ok := skillsLoader.Get(name)
		if !ok {
			return "", false
		}
		return def.Render(argsStr), true
	}

	// Wire rule engine into commands.
	if orch.RuleEngine != nil {
		cmdReg.RulesFormat = orch.RuleEngine.Format
		cmdReg.RulesAdd = func(decision, pattern, comment string) error {
			return orch.RuleEngine.AddRule(tools.RuleDecision(decision), pattern, comment)
		}
		cmdReg.RulesRemove = func(decision, pattern string) error {
			return orch.RuleEngine.RemoveRule(tools.RuleDecision(decision), pattern)
		}
	}

	// Wire theme manager into /theme command.
	if themeMgr != nil {
		cmdReg.ThemeList = themeMgr.Format
		cmdReg.ThemeSet = themeMgr.Set
		cmdReg.ThemeActive = func() string { return themeMgr.Active().Name }
	}

	// Wire MCP status into /mcp command.
	if mcpMgr != nil {
		cmdReg.MCPStatus = mcpMgr.Status
		cmdReg.MCPConfigPath = mcpMgr.ConfigPath
		if extras != nil && extras.toolRegistry != nil {
			tr := extras.toolRegistry
			cmdReg.MCPReload = func() (int, error) {
				return mcpMgr.Reload(ctx, tr)
			}
		}
	}

	// Wire environment probe into /env command.
	if envProbeInst != nil {
		cmdReg.EnvProbeSnapshot = envProbeInst.BuildSystemContext
		cmdReg.EnvProbeRefresh = func() { envProbeInst.RefreshAsync(ctx) }
	}

	// Wire Google OAuth callbacks into /auth notebooklm command.
	var gac *auth.Client
	if len(googleAuth) > 0 {
		gac = googleAuth[0]
	}
	if gac != nil {
		cmdReg.GoogleAuthStatus = gac.Status
		cmdReg.GoogleAuthSetup = func(clientID, clientSecret string) error {
			return gac.SaveClientConfig(auth.ClientConfig{
				ClientID:     clientID,
				ClientSecret: clientSecret,
			})
		}
		cmdReg.GoogleAuthLogin = func() (instructions string, deviceCode string, err error) {
			res, err := gac.EnsureToken(ctx)
			if err != nil {
				return "", "", err
			}
			if !res.AuthRequired {
				return "✅ Already authenticated with Google.", "", nil
			}
			return res.Instructions, res.DeviceCode, nil
		}
		cmdReg.GoogleAuthPoll = func(deviceCode string) error {
			pollCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			defer cancel()
			_, err := gac.CompleteDeviceFlow(pollCtx, deviceCode, 5*time.Second)
			return err
		}
		cmdReg.GoogleAuthLogout = func() error {
			return gac.RevokeToken(ctx)
		}
	}
}

func runTUI(ctx context.Context, orch *engine.Orchestrator, pm *process.Manager, reg *registry.ModelRegistry, modelName, consultantName string, sysConfig *router.SystemConfiguration, themeMgr *theme.Manager, mcpMgr *orchestrator.ManagedManager, appState *config.AppStateManager, extras *tuiExtras, cmdReg *commands.Registry, envProbeInst *gorkenv.EnvProbe, discMgr ...*discovery.Manager) {
	if mcpMgr != nil {
		defer mcpMgr.StopAll()
	}

	model, err := tui.NewModel(orch, pm, modelName, consultantName, cmdReg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating TUI: %v\n", err)
		os.Exit(1)
	}
	if len(discMgr) > 0 && discMgr[0] != nil {
		model.SetDiscoveryManager(discMgr[0])
	}

	// Wire integration settings (Integrations tab in /settings overlay).
	if extras != nil && extras.integrations != nil {
		intCfg := extras.integrations
		intDir := extras.configDir
		model.SetIntegrationCallbacks(
			func() map[string]string {
				out := make(map[string]string, len(intCfg.Values))
				for k, v := range intCfg.Values {
					out[k] = v
				}
				return out
			},
			func(key, value string) error {
				return intCfg.Set(intDir, key, value)
			},
		)
	}

	// Attempt to load nomic semantic embedder (non-fatal if unavailable).
	var consultMedPtr *consultation.ConsultationMediator
	if extras != nil {
		consultMedPtr = extras.consultMediator
	}
	go initEmbedder(orch, consultMedPtr)

	if reg != nil {
		available := buildCommandModelInfo(reg.ListActiveModels())
		var primaryInfo, consultantInfo commands.ModelInfo
		if sysConfig != nil {
			primaryInfo = commands.ModelInfo{
				ID:       string(sysConfig.PrimaryModel.ID),
				Name:     sysConfig.PrimaryModel.Name,
				Provider: string(sysConfig.PrimaryModel.Provider),
				Thinking: sysConfig.PrimaryModel.Capabilities.SupportsThinking,
			}
			consultantInfo = commands.ModelInfo{
				ID:       string(sysConfig.SpecialistModel.ID),
				Name:     sysConfig.SpecialistModel.Name,
				Provider: string(sysConfig.SpecialistModel.Provider),
				Thinking: sysConfig.SpecialistModel.Capabilities.SupportsThinking,
			}
		}
		model.SetModelInfo(reg, available, primaryInfo, consultantInfo)
	}

	var gac *auth.Client
	if extras != nil {
		gac = extras.googleAuth
	}
	wireCommandRegistry(ctx, cmdReg, orch, extras, appState, themeMgr, mcpMgr, envProbeInst, gac)

	pr, pw := io.Pipe()

	hm := hotkeys.NewManager(func() error {
		orch.SaveSession("")
		return nil
	}, pw)

	if err := hm.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start hotkey manager: %v\n", err)
	} else {
		defer hm.Stop()
	}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithInput(pr),
		// tea.WithMouseCellMotion(), // DISABLED: Causes keyboard issues on Termux
	)

	model.SetProgram(p)

	// Forward hotkey commands to Bubble Tea program
	go func() {
		for cmd := range hm.Commands {
			p.Send(cmd)
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func buildCommandModelInfo(models []registry.ModelDefinition) []commands.ModelInfo {
	result := make([]commands.ModelInfo, 0, len(models))
	for _, m := range models {
		result = append(result, commands.ModelInfo{
			ID:       string(m.ID),
			Name:     m.Name,
			Provider: string(m.Provider),
			Thinking: m.Capabilities.SupportsThinking,
		})
	}
	return result
}

// removeFromSlice returns a new slice with all occurrences of item removed.
func removeFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// handleSetup runs the interactive setup wizard
func handleSetup(configDir string) {
	reader := bufio.NewReader(os.Stdin)
	envPath := ".env"

	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║           Gorkbot Setup Wizard           ║")
	fmt.Println("║        by Todd Eddings / Velarium AI     ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Gorkbot works with any OpenAI-compatible API.")
	fmt.Println("This wizard configures your API keys.")
	fmt.Println("Keys will be saved to .env in the current directory.")
	fmt.Println("Keys will be ENCRYPTED using a local key.")
	fmt.Println()

	km, err := security.NewKeyManager(configDir)
	if err != nil {
		fmt.Printf("Error initializing security: %v\n", err)
		return
	}

	// 1. Primary AI Providers
	fmt.Print("1. Enter your Primary API Key (xAI starts with xai-...): ")
	xaiKey, _ := reader.ReadString('\n')
	xaiKey = strings.TrimSpace(xaiKey)

	fmt.Print("2. Enter your Google Gemini Key (starts with AIza...): ")
	geminiKey, _ := reader.ReadString('\n')
	geminiKey = strings.TrimSpace(geminiKey)

	fmt.Print("3. Enter your Anthropic API Key (starts with sk-ant-...): ")
	anthropicKey, _ := reader.ReadString('\n')
	anthropicKey = strings.TrimSpace(anthropicKey)

	fmt.Print("4. Enter your OpenAI API Key (starts with sk-...): ")
	openaiKey, _ := reader.ReadString('\n')
	openaiKey = strings.TrimSpace(openaiKey)

	fmt.Print("5. Enter your MiniMax API Key: ")
	minimaxKey, _ := reader.ReadString('\n')
	minimaxKey = strings.TrimSpace(minimaxKey)

	fmt.Println("\n--- MCP & Extension Credentials ---")
	
	fmt.Print("6. GitHub Personal Access Token (for mcp_github): ")
	githubToken, _ := reader.ReadString('\n')
	githubToken = strings.TrimSpace(githubToken)

	fmt.Print("7. Brave Search API Key (for mcp_brave-search): ")
	braveKey, _ := reader.ReadString('\n')
	braveKey = strings.TrimSpace(braveKey)

	// Encrypt Keys
	var sb strings.Builder
	writeKey := func(name, val string) {
		if val != "" {
			enc, err := km.Encrypt(val)
			if err == nil {
				sb.WriteString(fmt.Sprintf("%s=ENC_%s\n", name, enc))
			}
		}
	}

	writeKey("XAI_API_KEY", xaiKey)
	writeKey("GEMINI_API_KEY", geminiKey)
	writeKey("ANTHROPIC_API_KEY", anthropicKey)
	writeKey("OPENAI_API_KEY", openaiKey)
	writeKey("MINIMAX_API_KEY", minimaxKey)
	writeKey("GITHUB_PERSONAL_ACCESS_TOKEN", githubToken)
	writeKey("BRAVE_API_KEY", braveKey)

	if err := os.WriteFile(envPath, []byte(sb.String()), 0600); err != nil {
		fmt.Printf("\n❌ Error saving .env file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Configuration saved to %s (Encrypted)\n", envPath)

	// ── MCP config installation ────────────────────────────────────────────
	mcpPath := filepath.Join(configDir, "mcp.json")
	if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
		if err := installDefaultMCPConfig(configDir); err != nil {
			fmt.Printf("⚠️  Could not install MCP config: %v\n", err)
			fmt.Printf("   You can manually copy configs/mcp.json to %s\n", mcpPath)
		} else {
			fmt.Printf("✅ MCP config installed to %s\n", mcpPath)
		}
	} else {
		fmt.Printf("ℹ️  MCP config already exists at %s\n", mcpPath)
	}

	fmt.Println()
	fmt.Println("── Next Steps ──────────────────────────────────────────────────")
	fmt.Println("1. Run 'gorkbot' to start the TUI.")
	fmt.Println()
	fmt.Println("2. MCP Servers are pre-configured. To see what's running:")
	fmt.Println("   /mcp status")
	fmt.Println()
	fmt.Println("3. If you have a GitHub token, it will be used automatically by")
	fmt.Println("   the 'github' MCP server (mcp_github_* tools).")
	fmt.Println()
	fmt.Println("4. For Google NotebookLM with Google Drive sources, run:")
	fmt.Println("   /auth notebooklm login")
	fmt.Println("   (Gemini AI features work without this; only Drive sources need OAuth.)")
	fmt.Println()
	fmt.Println("5. To reload MCP servers after editing mcp.json:")
	fmt.Println("   /mcp reload")
	fmt.Println("────────────────────────────────────────────────────────────────")
}

// ensurePythonMCPPackage verifies that the Python 'mcp' package is importable
// and auto-installs it if absent. Python MCP servers (notebooklm, gorkbot-termux,
// gorkbot-introspect) all start with `from mcp.server.fastmcp import FastMCP`;
// without this package they crash immediately and the Go transport sees
// "transport closed" — a confusing error that obscures the real cause.
func ensurePythonMCPPackage(logger *slog.Logger) {
	python, err := exec.LookPath("python3")
	if err != nil {
		if p, err2 := exec.LookPath("python"); err2 == nil {
			python = p
		} else {
			logger.Warn("Python not found — Python MCP servers will not start")
			return
		}
	}

	// Check if 'mcp' is importable.
	checkCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	check := exec.CommandContext(checkCtx, python, "-c", "import mcp")
	if err := check.Run(); err == nil {
		return // already installed
	}

	logger.Warn("Python 'mcp' package not found — attempting auto-install (pip install mcp)")
	installCtx, installCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer installCancel()

	pip, pipErr := exec.LookPath("pip3")
	if pipErr != nil {
		pip, pipErr = exec.LookPath("pip")
	}
	if pipErr != nil {
		logger.Error("pip not found — cannot auto-install mcp package; Python MCP servers will fail",
			"fix", "pip install mcp")
		return
	}

	installCmd := exec.CommandContext(installCtx, pip, "install", "--quiet", "mcp")
	out, installErr := installCmd.CombinedOutput()
	if installErr != nil {
		logger.Error("Failed to auto-install Python mcp package — Python MCP servers will fail",
			"err", installErr, "output", strings.TrimSpace(string(out)),
			"fix", "run manually: pip install mcp")
		return
	}
	logger.Info("Python 'mcp' package installed successfully")
}

// installDefaultMCPConfig writes a platform-appropriate default mcp.json to
// configDir. It detects the project directory from the running executable and
// resolves Python MCP server paths automatically.
func installDefaultMCPConfig(configDir string) error {
	// Resolve project dir: executable lives at <projectDir>/bin/gorkbot (or similar)
	exePath, err := os.Executable()
	if err != nil {
		exePath = ""
	}
	projectDir := ""
	if exePath != "" {
		// Walk up: if exe is <dir>/bin/gorkbot, project = <dir>
		dir := filepath.Dir(exePath)
		if filepath.Base(dir) == "bin" {
			projectDir = filepath.Dir(dir)
		} else {
			projectDir = dir
		}
	}
	// Fallback to CWD if the binary is not in a bin/ subdir
	if projectDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectDir = cwd
		}
	}

	type serverEntry struct {
		Name         string            `json:"name"`
		Command      string            `json:"command"`
		Args         []string          `json:"args"`
		Env          map[string]string `json:"env,omitempty"`
		ShaperPath   string            `json:"shaper_path,omitempty"`
		AuthProvider string            `json:"auth_provider,omitempty"`
		Disabled     bool              `json:"disabled,omitempty"`
		Description  string            `json:"description"`
	}

	// Determine if we're on Windows for platform-specific entries
	goos := strings.ToLower(os.Getenv("GOOS"))
	if goos == "" {
		// Use build-time constant approach — check for a known Windows env var
		if _, ok := os.LookupEnv("SYSTEMROOT"); ok {
			goos = "windows"
		} else {
			goos = "linux"
		}
	}

	join := func(parts ...string) string { return filepath.Join(parts...) }

	servers := []serverEntry{
		{
			Name:        "sequential-thinking",
			Command:     "npx",
			Args:        []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
			Description: "Structured multi-step reasoning chains for complex tasks",
		},
		{
			Name:        "filesystem",
			Command:     "npx",
			Args:        []string{"-y", "@modelcontextprotocol/server-filesystem", projectDir, configDir},
			Description: "File I/O within project and config directories",
		},
		{
			Name:        "memory",
			Command:     "npx",
			Args:        []string{"-y", "@modelcontextprotocol/server-memory"},
			Description: "Persistent cross-session key-value entity graph",
		},
		{
			Name:        "time",
			Command:     "python3",
			Args:        []string{"-m", "mcp_server_time"},
			Description: "Current time and timezone lookups",
		},
		{
			Name:        "fetch",
			Command:     "python3",
			Args:        []string{"-m", "mcp_server_fetch"},
			Description: "HTTP fetch for any URL with raw body access",
		},
		{
			Name:        "github",
			Command:     "npx",
			Args:        []string{"-y", "@modelcontextprotocol/server-github"},
			Env:         map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_PERSONAL_ACCESS_TOKEN}"},
			Disabled:    os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN") == "",
			Description: "GitHub API — issues, PRs, repos, code search (needs GITHUB_PERSONAL_ACCESS_TOKEN)",
		},
		{
			Name:        "brave-search",
			Command:     "npx",
			Args:        []string{"-y", "@modelcontextprotocol/server-brave-search"},
			Env:         map[string]string{"BRAVE_API_KEY": "${BRAVE_API_KEY}"},
			Disabled:    os.Getenv("BRAVE_API_KEY") == "",
			Description: "Brave web search API (needs BRAVE_API_KEY)",
		},
		{
			Name:        "puppeteer",
			Command:     "npx",
			Args:        []string{"-y", "@modelcontextprotocol/server-puppeteer"},
			Disabled:    goos == "windows", // puppeteer on Windows often needs separate config
			Description: "Headless browser automation with JavaScript rendering",
		},
		{
			Name:         "notebooklm",
			Command:      "python3",
			Args:         []string{join(projectDir, "server", "notebooklm_mcp.py")},
			ShaperPath:   join(projectDir, "plugins", "notebooklm_shaper.py"),
			AuthProvider: "google",
			Description:  "Google NotebookLM — notebooks, sources, AI chat (needs GEMINI_API_KEY; Drive needs /auth notebooklm login)",
		},
	}

	// Platform-specific entries
	if goos != "windows" {
		servers = append(servers, serverEntry{
			Name:        "gorkbot-introspect",
			Command:     "python3",
			Args:        []string{join(projectDir, "mcp_servers", "gorkbot_introspect.py")},
			Description: "Gorkbot self-introspection — capabilities, tools, session state",
		})
		servers = append(servers, serverEntry{
			Name:        "gorkbot-termux",
			Command:     "python3",
			Args:        []string{join(projectDir, "mcp_servers", "gorkbot_termux.py")},
			Description: "Termux environment management and system APIs",
		})
	}

	type configDoc struct {
		Comment string        `json:"_comment"`
		Servers []serverEntry `json:"servers"`
	}

	doc := configDoc{
		Comment: "Gorkbot MCP Server Configuration — edit and run /mcp reload to apply",
		Servers: servers,
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	return os.WriteFile(filepath.Join(configDir, "mcp.json"), data, 0600)
}

// handleStatus shows the current configuration status
func handleStatus() {
	env, err := platform.GetEnvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Critical error detecting environment: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║             Gorkbot Status               ║")
	fmt.Println("║        by Todd Eddings / Velarium AI     ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	fmt.Printf("OS: %s (%s)\n", env.OS, env.Arch)
	if env.IsTermux {
		fmt.Println("Environment: Termux (Android)")
	}
	fmt.Printf("Config Dir: %s\n", env.ConfigDir)
	fmt.Printf("Log Dir:    %s\n", env.LogDir)
	fmt.Println()

	xaiKey := os.Getenv("XAI_API_KEY")
	geminiKey := os.Getenv("GEMINI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	openaiKey := os.Getenv("OPENAI_API_KEY")
	minimaxKey := os.Getenv("MINIMAX_API_KEY")
	githubKey := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	braveKey := os.Getenv("BRAVE_API_KEY")

	mask := func(key string) string {
		if key == "" {
			return "❌ Not set"
		}
		if len(key) > 8 {
			return "✅ Set (" + key[:4] + "..." + key[len(key)-4:] + ")"
		}
		return "✅ Set (***)"
	}

	fmt.Printf("Primary (xAI):   %s\n", mask(xaiKey))
	fmt.Printf("Gemini:          %s\n", mask(geminiKey))
	fmt.Printf("Anthropic:       %s\n", mask(anthropicKey))
	fmt.Printf("OpenAI:          %s\n", mask(openaiKey))
	fmt.Printf("MiniMax:         %s\n", mask(minimaxKey))
	fmt.Println()
	fmt.Printf("GitHub MCP:      %s\n", mask(githubKey))
	fmt.Printf("Brave Search:    %s\n", mask(braveKey))
	fmt.Printf("NotebookLM:      %s (shared with Gemini)\n", mask(geminiKey))

	fmt.Println()
}

// isContextOverflow returns true for API errors indicating the conversation
// history is too long to fit in the model's context window.
func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context window exceeded") ||
		strings.Contains(msg, "context_length_exceeded") ||
		strings.Contains(msg, "maximum context length") ||
		strings.Contains(msg, "reduce the length") ||
		strings.Contains(msg, "too many tokens")
}

// printHelp displays usage information
func printHelp() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║              Gorkbot — AI Task Orchestration               ║")
	fmt.Println("║          Lead Designer & Engineer: Todd Eddings            ║")
	fmt.Println("║          Parent Entity: Velarium AI                        ║")
	fmt.Println("║          Contact: velarium.ai@gmail.com                    ║")
	fmt.Println("║                                                            ║")
	fmt.Println("║  Works with any OpenAI-compatible API endpoint.            ║")
	fmt.Println("║  Not affiliated with xAI (Grok) or Google (Gemini).       ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gorkbot [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  setup              Run setup wizard to configure API keys")
	fmt.Println("  status             Show configuration status")
	fmt.Println("  help               Show this help message")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -p <prompt>        Execute a single prompt and exit")
	fmt.Println("  -timeout <duration> Timeout for the operation (default: 60s)")
	fmt.Println("  -verbose-thoughts  Enable verbose output of consultant thinking")
	fmt.Println("  -watchdog          Enable orchestrator watchdog for debugging")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  gorkbot setup                    # Configure API keys")
	fmt.Println("  gorkbot status                   # Check configuration")
	fmt.Println("  gorkbot                          # Start interactive TUI")
	fmt.Println("  gorkbot -p \"Hello, Gorkbot!\"    # One-shot prompt")
	fmt.Println()
	fmt.Println("First time setup:")
	fmt.Println("  1. Run: gorkbot setup")
	fmt.Println("  2. Get API keys from:")
	fmt.Println("     - xAI:    https://console.x.ai/")
	fmt.Println("     - Google: https://aistudio.google.com/apikey")
	fmt.Println("  3. Run: gorkbot")
	fmt.Println()
}
