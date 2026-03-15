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
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/internal/tui"
	"github.com/velariumai/gorkbot/pkg/a2a"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/billing"
	"github.com/velariumai/gorkbot/pkg/channels/telegram"
	"github.com/velariumai/gorkbot/pkg/collab"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/discovery"
	"github.com/velariumai/gorkbot/pkg/mcp"
	"github.com/velariumai/gorkbot/pkg/memory"
	"github.com/velariumai/gorkbot/pkg/persist"
	"github.com/velariumai/gorkbot/pkg/process"
	"github.com/velariumai/gorkbot/pkg/providers"
	"github.com/velariumai/gorkbot/pkg/registry"
	"github.com/velariumai/gorkbot/pkg/router"
	"github.com/velariumai/gorkbot/pkg/scheduler"
	"github.com/velariumai/gorkbot/pkg/security"
	"github.com/velariumai/gorkbot/pkg/skills"
	"github.com/velariumai/gorkbot/pkg/subagents"
	"github.com/velariumai/gorkbot/pkg/theme"
	"github.com/velariumai/gorkbot/pkg/tools"
	"github.com/velariumai/gorkbot/pkg/usercommands"
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
	promptFlag    := flag.String("p", "", "Execute a single prompt and exit")
	stdinFlag     := flag.Bool("stdin", false, "Read prompt from stdin (one-shot mode)")
	outputFlag    := flag.String("output", "", "Write one-shot response to file instead of stdout")
	allowToolsFlag := flag.String("allow-tools", "", "Comma-separated list of tools allowed in one-shot mode")
	denyToolsFlag  := flag.String("deny-tools", "", "Comma-separated list of tools denied in one-shot mode")
	timeoutFlag   := flag.Duration("timeout", 60*time.Second, "Timeout for the operation")
	verboseThoughts := flag.Bool("verbose-thoughts", false, "Enable verbose output of consultant thinking")
	watchdogFlag  := flag.Bool("watchdog", false, "Enable orchestrator watchdog for state debugging")
	traceFlag     := flag.Bool("trace", false, "Write a JSONL execution trace to ~/.gorkbot/traces/")
	shareFlag     := flag.Bool("share", false, "Start a relay server and share this session (prints observer URL)")
	joinFlag      := flag.String("join", "", "Observe a shared session at host:port (e.g. localhost:9090)")
	a2aEnabled    := flag.Bool("a2a", false, "Enable A2A HTTP gateway")
	a2aAddr       := flag.String("a2a-addr", "127.0.0.1:18890", "A2A gateway listen address")
	flag.Parse()

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

	// Initialize Base Providers (Factories)
	baseGrok := ai.NewGrokProvider(grokKey, primaryOverride)
	baseGemini := ai.NewGeminiProvider(geminiKey, consultantOverride, *verboseThoughts)

	// Initialize Registry
	reg := registry.NewModelRegistry(logger)
	startupCtx := context.Background()

	// Register Providers
	if grokKey != "" {
		if err := reg.RegisterProvider(startupCtx, baseGrok); err != nil {
			logger.Error("Failed to register Grok provider", "error", err)
		}
	}
	if geminiKey != "" {
		if err := reg.RegisterProvider(startupCtx, baseGemini); err != nil {
			logger.Error("Failed to register Gemini provider", "error", err)
		}
	}

	// Initialize Router
	r := router.NewRouter(reg, logger)

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

	if err := toolRegistry.RegisterDefaultTools(); err != nil {
		logger.Error("Failed to register default tools", "error", err)
		os.Exit(1)
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
	logger.Info("Enhanced systems initialized (v1.6.2+)")

	// 6.3 Intelligence layer (ARC Router + MEL Meta-Experience Learning)
	orch.InitIntelligence(env.ConfigDir)
	logger.Info("Intelligence layer initialized (ARC + MEL)")

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

	// 6.4 Trace logger (--trace flag)
	if *traceFlag {
		traceDir := filepath.Join(env.LogDir, "traces")
		tracer := engine.NewTraceLogger(traceDir, true)
		orch.SetTraceLogger(tracer)
		defer tracer.Close()
	}

	// 6.5 MCP server integration
	mcpMgr := mcp.NewManager(env.ConfigDir, logger)
	mcpCtx := context.Background()
	if n, err := mcpMgr.LoadAndStart(mcpCtx); err != nil {
		logger.Warn("MCP manager failed to start some servers", "error", err)
	} else if n > 0 {
		count := mcpMgr.RegisterTools(toolRegistry)
		logger.Info("MCP tools registered", "servers", n, "tools", count)
	}

	// 6.5 Theme manager
	themeMgr := theme.NewManager(env.ConfigDir)
	logger.Info("Theme loaded", "active", themeMgr.Active().Name)

	// 6.5a App state manager — persists model selections + tool group prefs across sessions.
	appState := config.NewAppStateManager(env.ConfigDir)
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

	// 6.5.0b Telegram bot.
	tgMgr := telegram.NewManager(env.ConfigDir, logger)
	if tgMgr.IsEnabled() {
		tgErr := tgMgr.Start(func(ctx context.Context, userID int64, username, text string) (string, error) {
			resp, err := orch.ExecuteTask(ctx, text)
			return resp, err
		})
		if tgErr != nil {
			logger.Warn("Telegram bot failed to start", "err", tgErr)
		} else {
			defer tgMgr.Stop()
		}
	}

	// 6.5.0c Scheduler start.
	if sched != nil {
		schedCtx, schedCancel := context.WithCancel(context.Background())
		defer schedCancel()
		sched.Start(schedCtx)
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
	}

	// 7. Execution Loop
	ctx := context.Background()

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
		runOneShotTask(ctx, orch, oneShotPrompt, *outputFlag)
	} else {
		runTUI(orch, processManager, reg, primaryModelName, consultantModelName, sysConfig, themeMgr, mcpMgr, appState,
			&tuiExtras{sched: sched, tgMgr: tgMgr, userCmdLoader: userCmdLoader, billing: billingMgr, persist: persistStore}, discMgr)
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
func runOneShotTask(ctx context.Context, orch *engine.Orchestrator, prompt string, outputFile string) {
	fmt.Printf("> %s\n", prompt)
	resp, err := orch.ExecuteTask(ctx, prompt)
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
	sched         *scheduler.Scheduler
	tgMgr         *telegram.Manager
	userCmdLoader *usercommands.Loader
	billing       *billing.BillingManager
	persist       *persist.Store
}

func runTUI(orch *engine.Orchestrator, pm *process.Manager, reg *registry.ModelRegistry, modelName, consultantName string, sysConfig *router.SystemConfiguration, themeMgr *theme.Manager, mcpMgr *mcp.Manager, appState *config.AppStateManager, extras *tuiExtras, discMgr ...*discovery.Manager) {
	model, err := tui.NewModel(orch, pm, modelName, consultantName, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating TUI: %v\n", err)
		os.Exit(1)
	}
	if len(discMgr) > 0 && discMgr[0] != nil {
		model.SetDiscoveryManager(discMgr[0])
	}

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

	// Wire orchestrator adapter into command registry so new commands work.
	if cmdReg := model.GetCommandRegistry(); cmdReg != nil {
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
				if extras != nil && extras.persist != nil {
					stats, err := extras.persist.ToolCallStats(context.Background())
					if err != nil {
						return "Stats unavailable: " + err.Error()
					}
					return stats
				}
				return "SQLite persistence not initialized."
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
		}
	}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		// tea.WithMouseCellMotion(), // DISABLED: Causes keyboard issues on Termux
	)

	model.SetProgram(p)

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

	// Primary API Key (xAI / any OpenAI-compatible endpoint)
	fmt.Print("Enter your primary API Key (xAI starts with xai-...): ")
	xaiKey, _ := reader.ReadString('\n')
	xaiKey = strings.TrimSpace(xaiKey)

	// Consultant API Key (Google Gemini)
	fmt.Print("Enter your Gemini API Key (starts with AIza...): ")
	geminiKey, _ := reader.ReadString('\n')
	geminiKey = strings.TrimSpace(geminiKey)

	// Encrypt Keys
	encXai, _ := km.Encrypt(xaiKey)
	encGemini, _ := km.Encrypt(geminiKey)

	content := fmt.Sprintf("XAI_API_KEY=ENC_%s\nGEMINI_API_KEY=ENC_%s\n", encXai, encGemini)
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		fmt.Printf("\n❌ Error saving .env file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Configuration saved to %s (Encrypted)\n", envPath)
	fmt.Println("You can now run 'gorkbot' to start the TUI.")
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

	if xaiKey != "" {
		masked := xaiKey
		if len(xaiKey) > 8 {
			masked = xaiKey[:4] + "..." + xaiKey[len(xaiKey)-4:]
		}
		fmt.Printf("Primary API Key: ✅ Set (%s)\n", masked)
	} else {
		fmt.Printf("Primary API Key: ❌ Not set (Required)\n")
	}

	if geminiKey != "" {
		masked := geminiKey
		if len(geminiKey) > 8 {
			masked = geminiKey[:4] + "..." + geminiKey[len(geminiKey)-4:]
		}
		fmt.Printf("Gemini API Key:  ✅ Set (%s)\n", masked)
	} else {
		fmt.Printf("Gemini API Key:  ❌ Not set (Consultant features disabled)\n")
	}

	fmt.Println()
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
