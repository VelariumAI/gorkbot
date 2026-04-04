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
	"github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/internal/events"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/internal/tui"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/memory"
	"github.com/velariumai/gorkbot/pkg/process"
	pkgprov "github.com/velariumai/gorkbot/pkg/providers"
	"github.com/velariumai/gorkbot/pkg/registry"
	"github.com/velariumai/gorkbot/pkg/research"
	"github.com/velariumai/gorkbot/pkg/router"
	"github.com/velariumai/gorkbot/pkg/security"
	"github.com/velariumai/gorkbot/pkg/sre"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// loadEnv loads environment variables from .env file, supporting encryption
func loadEnv(configDir string) {
	file, err := os.Open(".env")
	if err != nil {
		return // It's okay if .env doesn't exist
	}
	defer file.Close()

	// Check file permissions - warn if insecure
	fileInfo, err := file.Stat()
	if err == nil {
		perm := fileInfo.Mode().Perm()
		// Check if file is readable by others (group or world)
		if perm&0070 != 0 || perm&0007 != 0 {
			fmt.Fprintf(os.Stderr, "Warning: .env file has insecure permissions %o - consider running: chmod 600 .env\n", perm)
		}
	}

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
		// Ensure file is closed on exit, though main exit cleans up fds anyway.
		defer f.Close()
	} else {
		// Fallback to discarding logs if file creation fails to avoid breaking TUI
		handler = slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	logger.Info("Gorkbot initialized", "os", env.OS, "arch", env.Arch, "termux", env.IsTermux)

	// 3. CLI Configuration
	promptFlag := flag.String("p", "", "Execute a single prompt and exit")
	timeoutFlag := flag.Duration("timeout", 60*time.Second, "Timeout for the operation")
	verboseThoughts := flag.Bool("verbose-thoughts", false, "Enable verbose output of consultant thinking")
	watchdogFlag := flag.Bool("watchdog", false, "Enable orchestrator watchdog for state debugging")
	enableSRE := flag.Bool("sre", false, "Enable Step-wise Reasoning Engine")
	enableEnsemble := flag.Bool("ensemble", false, "Enable multi-trajectory ensemble")
	disableSRE := flag.Bool("no-sre", false, "Disable Step-wise Reasoning Engine")
	flag.Parse()

	// 4. Provider Setup & Dynamic Routing
	// In a real app, load from env or config file in env.ConfigDir
	// Initialize providers.Manager so WrappedProviders are used
	keyStore := pkgprov.NewKeyStore(env.ConfigDir)
	provMgr := pkgprov.NewManager(keyStore, logger)
	provMgr.SetVerboseThoughts(*verboseThoughts)
	pkgprov.SetGlobalProviderManager(provMgr)
	engine.SetProviderManager(provMgr)

	grokKey, _ := keyStore.Get(pkgprov.ProviderXAI)
	if grokKey == "" {
		grokKey = os.Getenv("XAI_API_KEY")
	}
	geminiKey, _ := keyStore.Get(pkgprov.ProviderGoogle)

	// Initialize Base Providers (Factories) via Manager (ensures they are WrappedProviders)
	baseGrok, _ := provMgr.GetBase(pkgprov.ProviderXAI)
	if baseGrok == nil {
		baseGrok = ai.NewGrokProvider(grokKey, "")
	}
	baseGemini, _ := provMgr.GetBase(pkgprov.ProviderGoogle)
	if baseGemini == nil {
		baseGemini = ai.NewGeminiProvider(geminiKey, "", *verboseThoughts)
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

	// Select System Models (Dynamic Configuration)
	var primary ai.AIProvider
	var consultant ai.AIProvider
	primaryModelName := "Grok-3 (Default)"
	consultantModelName := ""

	sysConfig, err := r.SelectSystemModels()
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
		logger.Warn("Dynamic Model Selection Failed - Falling back to hardcoded defaults", "error", err)
	}

	// Fallback / Default Initialization if dynamic failed
	if primary == nil {
		primary = baseGrok // Default to base Grok (hardcoded model in struct)
	}
	if consultant == nil && geminiKey != "" {
		consultant = baseGemini // Default to base Gemini
		if consultantModelName == "" {
			consultantModelName = baseGemini.GetMetadata().Name
		}
	}

	// 4.5. Tool System Setup
	permissionMgr, err := tools.NewPermissionManager(env.ConfigDir)
	if err != nil {
		logger.Error("Failed to initialize permission manager", "error", err)
		fmt.Fprintf(os.Stderr, "Warning: Permission manager failed to initialize: %v\n", err)
		fmt.Fprintf(os.Stderr, "Tools will require manual approval each time.\n")
	}

	analytics, err := tools.NewAnalytics(env.ConfigDir)
	if err != nil {
		logger.Error("Failed to initialize analytics", "error", err)
		fmt.Fprintf(os.Stderr, "Warning: Analytics failed to initialize: %v\n", err)
	}

	registry := tools.NewRegistry(permissionMgr)
	registry.SetAnalytics(analytics)
	registry.SetAIProvider(primary)            // Set AI provider for Task tool
	registry.SetConsultantProvider(consultant) // Set Consultant provider for Consult tool
	registry.SetConfigDir(env.ConfigDir)       // Enable dynamic tool persistence

	// Initialize Research Engine (browser_search, browser_open, browser_find)
	researchEngine := research.NewEngine(10, logger)
	registry.SetResearchEngine(researchEngine)
	logger.Info("Research engine initialized", "max_documents", 10)

	if err := registry.RegisterDefaultTools(); err != nil {
		logger.Error("Failed to register default tools", "error", err)
		fmt.Fprintf(os.Stderr, "Error: Failed to register tools: %v\n", err)
		os.Exit(1)
	}

	// Load any dynamic tools the agent previously created (no restart needed)
	if err := registry.LoadDynamicTools(env.ConfigDir); err != nil {
		logger.Warn("Failed to load dynamic tools", "error", err)
		fmt.Fprintf(os.Stderr, "Warning: Some custom tools could not be loaded: %v\n", err)
		fmt.Fprintf(os.Stderr, "You can recreate them using /tools or create_tool\n")
	}

	logger.Info("Tool system initialized", "tool_count", len(registry.List()))

	// 5. Memory System Setup
	memMgr, err := memory.NewMemoryManager(env.ConfigDir, logger)
	if err != nil {
		logger.Error("Failed to initialize memory manager", "error", err)
	} else {
		// Load default session
		if _, err := memMgr.LoadDefaultSession(); err != nil {
			logger.Error("Failed to load session", "error", err)
		}
	}

	// 6. Orchestration Engine — Initialize ProviderCoordinator
	provCoord := providers.NewProviderCoordinator(provMgr, primary, consultant, nil, events.NewBus(), logger)
	orch := engine.NewOrchestrator(provCoord, registry, logger, *watchdogFlag)

	// 6.0.5 Initialize message suppression (will be set with verbose mode after AppState loads)
	orch.MessageSuppressor = engine.NewMessageSuppressionMiddleware(false, logger)

	// 6.1 SENSE AgeMem + Engram Store
	if err := orch.InitSENSEMemory(env.ConfigDir); err != nil {
		logger.Warn("SENSE AgeMem init failed", "error", err)
	} else {
		logger.Info("SENSE AgeMem initialised", "data_dir", env.ConfigDir)
	}

	// 6.2 AppState — load saved preferences and apply CLI overrides
	appState := config.NewAppStateManager(env.ConfigDir)
	if *enableSRE {
		appState.SetSREEnabled(true)
	}
	if *disableSRE {
		appState.SetSREEnabled(false)
	}
	if *enableEnsemble {
		appState.SetEnsembleEnabled(true)
	}

	// Update message suppression with persisted verbose mode setting
	verboseMode := appState.IsVerboseMode()
	orch.MessageSuppressor.SetVerboseMode(verboseMode)

	// 6.3 Initialize enhancements
	orch.InitEnhancements(env.ConfigDir, ".")

	// 6.4 Initialize SRE
	appStateSnapshot := appState.Get()
	sreEnabled := appStateSnapshot.SREEnabled == nil || *appStateSnapshot.SREEnabled
	ensEnabled := appStateSnapshot.EnsembleEnabled != nil && *appStateSnapshot.EnsembleEnabled
	orch.InitSRE(sre.SREConfig{
		EnsembleEnabled:  ensEnabled,
		CoSEnabled:       sreEnabled,
		GroundingEnabled: sreEnabled,
		HypothesisTurns:  3,
		PruneTurns:       6,
		CorrectionThresh: 0.30,
	})

	// Sync memory session history into orchestrator if available.
	if memMgr != nil {
		logger.Info("Memory manager active", "sessions_dir", env.ConfigDir)
	}

	// 7. Execution Loop
	ctx := context.Background()

	if *promptFlag != "" {
		// One-shot execution with timeout
		ctx, cancel := context.WithTimeout(ctx, *timeoutFlag)
		defer cancel()
		runTask(ctx, orch, *promptFlag)
	} else {
		// TUI Mode (replaces old REPL)
		runTUI(orch, reg, primaryModelName, consultantModelName, sysConfig, env, appState)
	}

	// ── Post-session: handle any tools that need a Go rebuild ────────────────
	handlePendingRebuild(registry, env.ConfigDir)
}

// handlePendingRebuild checks whether any tools were created or modified in a
// way that requires recompiling Gorkbot. It either auto-rebuilds (when 'go' is
// on PATH and GORKBOT_AUTO_REBUILD=1 is set) or prints a clear notification.
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
		// Determine project root (directory of the running binary or CWD).
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

func runTask(ctx context.Context, orch *engine.Orchestrator, prompt string) {
	fmt.Printf("> %s\n", prompt)
	resp, err := orch.ExecuteTask(ctx, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	fmt.Println(resp)
}

func runTUI(orch *engine.Orchestrator, reg *registry.ModelRegistry, modelName, consultantName string, sysConfig *router.SystemConfiguration, env *platform.EnvConfig, appState *config.AppStateManager) {
	// Create TUI model
	pm := process.NewManager() // Need process manager
	cmdReg := commands.NewRegistry()
	model, err := tui.NewModel(orch, pm, modelName, consultantName, cmdReg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating TUI: %v\n", err)
		os.Exit(1)
	}

	// Wire HITL callback to the TUI model
	orch.HITLCallback = model.RequestHITLApproval

	// Wire verbose mode callbacks to the commands registry
	if cmdReg.Orch == nil {
		cmdReg.Orch = &commands.OrchestratorAdapter{}
	}
	cmdReg.Orch.GetVerboseMode = func() bool {
		if orch.MessageSuppressor == nil {
			return false
		}
		return orch.MessageSuppressor.IsVerbose()
	}
	cmdReg.Orch.SetVerboseMode = func(enabled bool) error {
		if orch.MessageSuppressor == nil {
			return nil
		}
		orch.MessageSuppressor.SetVerboseMode(enabled)
		// Persist to app state
		if appState != nil {
			return appState.SetVerboseMode(enabled)
		}
		return nil
	}

	// Wire up SRE and Ensemble settings callbacks if Orch adapter is available
	if cmdReg.Orch == nil {
		cmdReg.Orch = &commands.OrchestratorAdapter{}
	}
	if appState != nil {
		cmdReg.Orch.GetSREEnabled = func() bool {
			st := appState.Get()
			return st.SREEnabled == nil || *st.SREEnabled
		}
		cmdReg.Orch.SetSREEnabled = func(enabled bool) error {
			return appState.SetSREEnabled(enabled)
		}
		cmdReg.Orch.GetEnsembleEnabled = func() bool {
			st := appState.Get()
			return st.EnsembleEnabled != nil && *st.EnsembleEnabled
		}
		cmdReg.Orch.SetEnsembleEnabled = func(enabled bool) error {
			return appState.SetEnsembleEnabled(enabled)
		}
	}

	// Wire up live model registry so /model can display and switch real models.
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

	// Create Bubble Tea program
	// Collect program options
	programOptions := []tea.ProgramOption{
		tea.WithAltScreen(), // Use alternate screen buffer
	}

	// Enable mouse support on non-Termux platforms
	// On Termux, mouse events can interfere with keyboard input
	if !env.IsTermux {
		programOptions = append(programOptions, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(model, programOptions...)

	// Set program reference in model for streaming callbacks
	model.SetProgram(p)

	// Run the program
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// buildCommandModelInfo converts registry model definitions to the minimal
// commands.ModelInfo descriptors used by the /model command.
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

// handleSetup runs the interactive setup wizard
func handleSetup(configDir string) {
	reader := bufio.NewReader(os.Stdin)
	envPath := ".env"

	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║           Gorkbot Setup Wizard           ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("This wizard will help you configure your API keys.")
	fmt.Println("Keys will be saved to .env in the current directory.")
	fmt.Println("Keys will be ENCRYPTED using a local key.")
	fmt.Println()

	// Initialize Key Manager
	km, err := security.NewKeyManager(configDir)
	if err != nil {
		fmt.Printf("Error initializing security: %v\n", err)
		return
	}

	// xAI API Key
	fmt.Print("Enter your xAI API Key (starts with xai-...): ")
	xaiKey, _ := reader.ReadString('\n')
	xaiKey = strings.TrimSpace(xaiKey)

	// Gemini API Key
	fmt.Print("Enter your Gemini API Key (starts with AIza...): ")
	geminiKey, _ := reader.ReadString('\n')
	geminiKey = strings.TrimSpace(geminiKey)

	// Encrypt Keys
	encXai, _ := km.Encrypt(xaiKey)
	encGemini, _ := km.Encrypt(geminiKey)

	// Save to .env with ENC_ prefix
	content := fmt.Sprintf("XAI_API_KEY=ENC_%s\nGEMINI_API_KEY=ENC_%s\n", encXai, encGemini)
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		fmt.Printf("\n❌ Error saving .env file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Configuration saved to %s (Encrypted)\n", envPath)
	fmt.Println("You can now run 'gorkbot' to start using the tool.")
}

// handleStatus shows the current configuration status
func handleStatus() {
	// 1. Universal Environment Abstraction
	env, err := platform.GetEnvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Critical error detecting environment: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║             Gorkbot Status               ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	fmt.Printf("OS: %s (%s)\n", env.OS, env.Arch)
	if env.IsTermux {
		fmt.Println("Environment: Termux")
	}
	fmt.Printf("Config Dir: %s\n", env.ConfigDir)
	fmt.Printf("Log Dir:    %s\n", env.LogDir)
	fmt.Println()

	// Check API Keys
	xaiKey := os.Getenv("XAI_API_KEY")
	geminiKey := os.Getenv("GEMINI_API_KEY")

	if xaiKey != "" {
		masked := xaiKey
		if len(xaiKey) > 8 {
			masked = xaiKey[:4] + "..." + xaiKey[len(xaiKey)-4:]
		}
		fmt.Printf("xAI API Key:    ✅ Set (%s)\n", masked)
	} else {
		fmt.Printf("xAI API Key:    ❌ Not set (Required)\n")
	}

	if geminiKey != "" {
		masked := geminiKey
		if len(geminiKey) > 8 {
			masked = geminiKey[:4] + "..." + geminiKey[len(geminiKey)-4:]
		}
		fmt.Printf("Gemini API Key: ✅ Set (%s)\n", masked)
	} else {
		fmt.Printf("Gemini API Key: ❌ Not set (Consultant features disabled)\n")
	}

	fmt.Println()
}

// printHelp displays usage information
func printHelp() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║              Gorkbot — AI Task Orchestration               ║")
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
	fmt.Println("  gorkbot setup                     # Configure API keys")
	fmt.Println("  gorkbot status                    # Check configuration")
	fmt.Println("  gorkbot                           # Start interactive TUI")
	fmt.Println("  gorkbot -p \"Hello, Gorkbot!\"      # One-shot prompt")
	fmt.Println()
	fmt.Println("First time setup:")
	fmt.Println("  1. Run: grokster setup")
	fmt.Println("  2. Get API keys from:")
	fmt.Println("     - xAI: https://console.x.ai/")
	fmt.Println("     - Google: https://aistudio.google.com/apikey")
	fmt.Println("  3. Run: grokster")
	fmt.Println()
}
