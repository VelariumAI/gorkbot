package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// CommandDefinition defines a slash command in the Gorkbot TUI
type CommandDefinition struct {
	Name        string
	Description string
	Usage       string
	Handler     func(args []string) (string, error)
	IsModifier  bool // If true, command modifies state or filesystem
}

// ModelInfo is a minimal model descriptor for use by the /model command.
// It mirrors registry.ModelDefinition but lives in the commands package to
// avoid an import cycle between internal/tui and pkg/registry.
type ModelInfo struct {
	ID       string // e.g. "grok-3-mini"
	Name     string // e.g. "Grok-3 Mini"
	Provider string // "xai" or "google"
	Thinking bool   // supports reasoning_effort / thinking_config
}

// OrchestratorAdapter provides command handlers access to orchestrator methods
// without creating an import cycle (engine imports tui, so tui cannot import engine).
type OrchestratorAdapter struct {
	GetContextReport func() string
	GetCostReport    func() string
	GetCheckpoints   func() string
	RewindTo         func(id string) string
	ExportConv       func(format, path string) string
	CompactFocus     func(focus string) string
	CycleMode        func() string
	SetMode          func(name string) string
	GetMode          func() string
	SaveSession      func(name string) string
	LoadSession      func(name string) string
	ListSessions     func() string
	// RateResponse records user satisfaction (1–5) with the last response.
	RateResponse func(score float64) string
	// StartRelay starts session sharing and returns the observer URL.
	StartRelay func() string
	// StopRelay stops the active session relay.
	StopRelay func()
	// ToggleDebug flips debug mode (raw AI output visible). Returns new state.
	ToggleDebug func() bool
	// SetPrimary hot-swaps the primary provider/model. Returns status string.
	SetPrimary func(provider, modelID string) string
	// SetSecondary hot-swaps the consultant provider/model. Returns status string.
	SetSecondary func(provider, modelID string) string
	// SetAutoSecondary enables auto-secondary selection (clears explicit consultant).
	SetAutoSecondary func() string
	// GetProviderStatus returns a formatted status summary of all providers.
	GetProviderStatus func() string
	// SetProviderKey saves a new API key for the given provider.
	SetProviderKey func(provider, key string) string
	// PersistDisabledCategories writes the list of disabled tool categories to disk.
	PersistDisabledCategories func(cats []string) error
	// GetScheduledTasks returns a formatted list of scheduled tasks.
	GetScheduledTasks func() string
	// GetTelegramStatus returns the current Telegram bot status.
	GetTelegramStatus func() string
	// BillingGet returns a formatted session usage report from the billing manager.
	BillingGet func() string
	// BillingGetAllTime returns a formatted all-time usage report from usage_history.jsonl.
	BillingGetAllTime func() string
	// GetToolCallStats returns a formatted table of per-tool call counts and success rates from SQLite.
	GetToolCallStats func() string
	// GetDiagnosticReport returns a full system diagnostic snapshot for display in settings.
	GetDiagnosticReport func() string

	// ToggleProvider toggles a provider's session-disabled state and persists the change.
	// Returns (nowEnabled bool, statusMsg string).
	ToggleProvider func(providerID string) (bool, string)

	// GetProviderEnabled returns a map of providerID → enabled for all known providers.
	GetProviderEnabled func() map[string]bool

	// PersistDisabledProviders writes the list of disabled provider IDs to disk.
	PersistDisabledProviders func(ids []string) error

	// SetThinkingBudget sets the extended-thinking token budget on the orchestrator.
	// A budget of 0 disables extended thinking.  Returns a status string.
	SetThinkingBudget func(budget int) string

	// Verbose mode helpers for message suppression
	GetVerboseMode func() bool
	SetVerboseMode func(enabled bool) error

	// GetCascadeOrder returns the current provider failover order as a string slice.
	GetCascadeOrder func() []string
	// SetCascadeOrder updates the cascade order and persists it to AppState.
	SetCascadeOrder func(order []string) string
	// ResetCascadeOrder restores the default cascade order and persists the change.
	ResetCascadeOrder func() string

	// ToggleSandbox toggles the SENSE input sanitizer. Returns true when now enabled.
	ToggleSandbox func() bool
	// SandboxEnabled returns true when the SENSE input sanitizer is active.
	SandboxEnabled func() bool

	// GetSREEnabled returns true when SRE is enabled.
	GetSREEnabled func() bool
	// SetSREEnabled enables or disables the Step-wise Reasoning Engine and persists the change.
	SetSREEnabled func(bool) error
	// GetEnsembleEnabled returns true when ensemble reasoning is enabled.
	GetEnsembleEnabled func() bool
	// SetEnsembleEnabled enables or disables ensemble reasoning and persists the change.
	SetEnsembleEnabled func(bool) error
}

// Registry holds all available commands
type Registry struct {
	commands          map[string]*CommandDefinition
	toolRegistry      *tools.Registry
	availableModels   []ModelInfo
	currentPrimary    ModelInfo
	currentConsultant ModelInfo
	// configDir is the application configuration directory.
	// Required by /self commands for locating trace and skills directories.
	configDir string
	// Orchestrator access (set after creation to avoid import cycles)
	Orch *OrchestratorAdapter
	// SkillsLoader for /skills command (optional)
	SkillsFormat func() string
	// SkillsGet looks up a skill by name and returns the rendered prompt (name, args → prompt, found).
	SkillsGet func(name, args string) (string, bool)
	// RulesFormat for /rules command (optional)
	RulesFormat func() string
	RulesAdd    func(decision, pattern, comment string) error
	RulesRemove func(decision, pattern string) error
	// Theme manager callbacks (optional)
	ThemeList   func() string
	ThemeSet    func(name string) error
	ThemeActive func() string
	// MCP manager callbacks (optional)
	MCPStatus     func() string
	MCPConfigPath func() string
	MCPReload     func() (int, error)
	// UserCmdsGet looks up a user-defined command (name, args → rendered prompt, found).
	UserCmdsGet func(name, args string) (string, bool)

	// Google OAuth callbacks — wired from pkg/auth.Client in main.go (optional).
	// GoogleAuthStatus returns a one-line status string ("✅ authenticated", etc.)
	GoogleAuthStatus func() string
	// GoogleAuthLogin initiates the Device Authorization Flow.
	// Returns (instructions, deviceCode, error).  The caller must display
	// instructions to the user, then call GoogleAuthPoll to complete the flow.
	GoogleAuthLogin func() (instructions string, deviceCode string, err error)
	// GoogleAuthPoll blocks until the user completes the Device Flow or ctx times out.
	// Returns "" on success; the caller should re-run whatever needed auth.
	GoogleAuthPoll func(deviceCode string) error
	// GoogleAuthLogout revokes and deletes the stored OAuth token.
	GoogleAuthLogout func() error
	// GoogleAuthSetup saves a client_id (and optional secret) from user input.
	GoogleAuthSetup func(clientID, clientSecret string) error

	// EnvProbeSnapshot returns the current environment snapshot as a formatted
	// string.  Wired from pkg/env.EnvProbe.BuildSystemContext in main.go.
	EnvProbeSnapshot func() string
	// EnvProbeRefresh triggers a background re-probe of the host environment.
	// Wired from pkg/env.EnvProbe.RefreshAsync in main.go.
	EnvProbeRefresh func()

	DryRun bool // If true, state-modifying commands only validate and mock execution
}

// NewRegistry creates and initializes the command registry
func NewRegistry() *Registry {
	r := &Registry{
		commands:     make(map[string]*CommandDefinition),
		toolRegistry: nil,
	}
	r.registerCommands()
	return r
}

// SetToolRegistry sets the tool registry for querying tools
func (r *Registry) SetToolRegistry(toolReg *tools.Registry) {
	r.toolRegistry = toolReg
}

// SetConfigDir sets the application configuration directory used by /self commands.
// Must be called before any /self subcommand is executed.
func (r *Registry) SetConfigDir(dir string) {
	r.configDir = dir
}

// SetModelInfo populates the /model command with live data from the registry.
// Call this once after dynamic model selection completes in main.go.
func (r *Registry) SetModelInfo(available []ModelInfo, primary, consultant ModelInfo) {
	r.availableModels = available
	r.currentPrimary = primary
	r.currentConsultant = consultant
}

// UpdateCurrentPrimary updates the displayed primary model after a live switch.
func (r *Registry) UpdateCurrentPrimary(info ModelInfo) {
	r.currentPrimary = info
}

// UpdateCurrentConsultant updates the displayed consultant model after a live switch.
func (r *Registry) UpdateCurrentConsultant(info ModelInfo) {
	r.currentConsultant = info
}

// Get retrieves a command by name
func (r *Registry) Get(name string) (*CommandDefinition, bool) {
	cmd, exists := r.commands[name]
	return cmd, exists
}

// All returns all registered commands
func (r *Registry) All() map[string]*CommandDefinition {
	return r.commands
}

// Execute runs a command with the given arguments
func (r *Registry) Execute(name string, args []string) (string, error) {
	cmd, exists := r.Get(name)
	if !exists {
		argsStr := strings.Join(args, " ")
		// Fall through to skills loader.
		if r.SkillsGet != nil {
			if prompt, ok := r.SkillsGet(name, argsStr); ok {
				return "SKILL_INVOKE:" + prompt, nil
			}
		}
		// Fall through to user-defined commands.
		if r.UserCmdsGet != nil {
			if prompt, ok := r.UserCmdsGet(name, argsStr); ok {
				return "SKILL_INVOKE:" + prompt, nil
			}
		}
		return "", fmt.Errorf("unknown command: /%s\n\nType `/help` for available commands, or `/skills` to list skill definitions.", name)
	}

	// ── Dry Run Protection ───────────────────────────────────────────────────
	if r.DryRun && cmd.IsModifier {
		return fmt.Sprintf("[DRY-RUN] Command '/%s' would modify state. Execution skipped.", name), nil
	}

	return cmd.Handler(args)
}

// registerCommands registers all available slash commands
func (r *Registry) registerCommands() {
	r.commands["clear"] = &CommandDefinition{
		Name:        "clear",
		Description: "Reset conversation context and screen",
		Usage:       "/clear",
		Handler:     r.handleClear,
	}

	r.commands["help"] = &CommandDefinition{
		Name:        "help",
		Description: "Display all available commands",
		Usage:       "/help",
		Handler:     r.handleHelp,
	}

	r.commands["about"] = &CommandDefinition{
		Name:        "about",
		Description: "Show system overview, intelligence stack, and platform info",
		Usage:       "/about",
		Handler:     r.handleAbout,
	}

	r.commands["chat"] = &CommandDefinition{
		Name:        "chat",
		Description: "Manage conversation history",
		Usage:       "/chat [save|load|list|delete] [name]",
		Handler:     r.handleChat,
		IsModifier:  true,
	}

	r.commands["model"] = &CommandDefinition{
		Name:        "model",
		Description: "View or switch primary/consultant model",
		Usage:       "/model [primary|consultant] [model-id]",
		Handler:     r.handleModel,
		IsModifier:  true,
	}

	r.commands["tools"] = &CommandDefinition{
		Name:        "tools",
		Description: "List active tools; /tools stats shows usage analytics; /tools audit shows persistent audit log; /tools errors shows recent failures",
		Usage:       "/tools [stats|audit|errors [tool_name]]",
		Handler:     r.handleTools,
	}

	r.commands["key"] = &CommandDefinition{
		Name:        "key",
		Description: "Set or validate an API key for a provider",
		Usage:       "/key <provider> <api-key>  |  /key status  |  /key validate <provider>",
		Handler:     r.handleKey,
		IsModifier:  true,
	}

	r.commands["auth"] = &CommandDefinition{
		Name:        "auth",
		Description: "Refresh API credentials",
		Usage:       "/auth [refresh|status]",
		Handler:     r.handleAuth,
		IsModifier:  true,
	}

	r.commands["settings"] = &CommandDefinition{
		Name:        "settings",
		Description: "View settings and configuration",
		Usage:       "/settings",
		Handler:     r.handleSettings,
	}

	r.commands["verbose"] = &CommandDefinition{
		Name:        "verbose",
		Description: "Toggle verbose mode (show/hide internal system messages)",
		Usage:       "/verbose [on|off|toggle]",
		Handler:     r.handleVerbose,
		IsModifier:  true,
	}

	r.commands["version"] = &CommandDefinition{
		Name:        "version",
		Description: "Show build version and system info",
		Usage:       "/version",
		Handler:     r.handleVersion,
	}

	r.commands["quit"] = &CommandDefinition{
		Name:        "quit",
		Description: "Exit the application gracefully",
		Usage:       "/quit",
		Handler:     r.handleQuit,
	}

	r.commands["bug"] = &CommandDefinition{
		Name:        "bug",
		Description: "Open GitHub issue template",
		Usage:       "/bug",
		Handler:     r.handleBug,
	}

	r.commands["theme"] = &CommandDefinition{
		Name:        "theme",
		Description: "Toggle light/dark mode",
		Usage:       "/theme [light|dark|auto]",
		Handler:     r.handleTheme,
		IsModifier:  true,
	}

	r.commands["compress"] = &CommandDefinition{
		Name:        "compress",
		Description: "Compress current context to save tokens",
		Usage:       "/compress",
		Handler:     r.handleCompress,
		IsModifier:  true,
	}

	r.commands["mouse"] = &CommandDefinition{
		Name:        "mouse",
		Description: "Toggle mouse support (hides keyboard)",
		Usage:       "/mouse",
		Handler:     r.handleMouse,
	}

	r.commands["permissions"] = &CommandDefinition{
		Name:        "permissions",
		Description: "View and manage tool permissions",
		Usage:       "/permissions [list|reset|reset <tool>]",
		Handler:     r.handlePermissions,
		IsModifier:  true,
	}

	r.commands["context"] = &CommandDefinition{
		Name:        "context",
		Description: "Show context window usage breakdown",
		Usage:       "/context",
		Handler:     r.handleContext,
	}

	r.commands["cost"] = &CommandDefinition{
		Name:        "cost",
		Description: "Show session API cost estimate",
		Usage:       "/cost",
		Handler:     r.handleCost,
	}

	r.commands["rewind"] = &CommandDefinition{
		Name:        "rewind",
		Description: "Restore conversation to a previous checkpoint",
		Usage:       "/rewind [last|<checkpoint-id>]",
		Handler:     r.handleRewind,
		IsModifier:  true,
	}

	r.commands["mode"] = &CommandDefinition{
		Name:        "mode",
		Description: "View or switch execution mode (normal/plan/auto)",
		Usage:       "/mode [normal|plan|auto]",
		Handler:     r.handleMode,
		IsModifier:  true,
	}

	r.commands["export"] = &CommandDefinition{
		Name:        "export",
		Description: "Export full session to .md, .txt, or .pdf (PDF requires pandoc)",
		Usage:       "/export [md|txt|pdf] [filename]",
		Handler:     r.handleExport,
	}

	r.commands["compact"] = &CommandDefinition{
		Name:        "compact",
		Description: "Compress context to save tokens",
		Usage:       "/compact [focus hint]",
		Handler:     r.handleCompactEnhanced,
		IsModifier:  true,
	}

	r.commands["skills"] = &CommandDefinition{
		Name:        "skills",
		Description: "List and invoke skill definitions",
		Usage:       "/skills [list|help]",
		Handler:     r.handleSkills,
	}

	r.commands["rules"] = &CommandDefinition{
		Name:        "rules",
		Description: "Manage fine-grained tool permission rules",
		Usage:       "/rules [list|add <allow|ask|deny> <pattern>|remove <allow|ask|deny> <pattern>]",
		Handler:     r.handleRules,
		IsModifier:  true,
	}

	r.commands["hooks"] = &CommandDefinition{
		Name:        "hooks",
		Description: "List installed lifecycle hook scripts",
		Usage:       "/hooks [list|dir]",
		Handler:     r.handleHooks,
	}

	r.commands["rename"] = &CommandDefinition{
		Name:        "rename",
		Description: "Rename the current session",
		Usage:       "/rename <name>",
		Handler:     r.handleRename,
		IsModifier:  true,
	}

	r.commands["save"] = &CommandDefinition{
		Name:        "save",
		Description: "Save current conversation (auto-names if no name given)",
		Usage:       "/save [name]",
		Handler:     r.handleSave,
		IsModifier:  true,
	}

	r.commands["resume"] = &CommandDefinition{
		Name:        "resume",
		Description: "Resume a previously saved session",
		Usage:       "/resume <name>  |  /resume list",
		Handler:     r.handleResume,
		IsModifier:  true,
	}

	r.commands["mcp"] = &CommandDefinition{
		Name:        "mcp",
		Description: "Show MCP server status, config, or reload servers",
		Usage:       "/mcp [status|config|reload]",
		Handler:     r.handleMCP,
		IsModifier:  true,
	}

	r.commands["rate"] = &CommandDefinition{
		Name:        "rate",
		Description: "Rate the last AI response (1–5). Feeds the adaptive model router.",
		Usage:       "/rate <1-5>",
		Handler:     r.handleRate,
		IsModifier:  true,
	}

	r.commands["share"] = &CommandDefinition{
		Name:        "share",
		Description: "Start or stop live session sharing over SSE.",
		Usage:       "/share [start|stop]",
		Handler:     r.handleShare,
		IsModifier:  true,
	}

	r.commands["debug"] = &CommandDefinition{
		Name:        "debug",
		Description: "Toggle debug mode: shows raw AI output including tool JSON blocks.",
		Usage:       "/debug",
		Handler:     r.handleDebug,
	}

	r.commands["schedule"] = &CommandDefinition{
		Name:        "schedule",
		Description: "List all scheduled tasks.",
		Usage:       "/schedule",
		Handler:     r.handleSchedule,
	}

	r.commands["telegram"] = &CommandDefinition{
		Name:        "telegram",
		Description: "Show Telegram bot integration status.",
		Usage:       "/telegram",
		Handler:     r.handleTelegram,
	}

	r.commands["a2a"] = &CommandDefinition{
		Name:        "a2a",
		Description: "Show A2A HTTP gateway status and usage.",
		Usage:       "/a2a",
		Handler:     r.handleA2A,
	}

	r.commands["commands"] = &CommandDefinition{
		Name:        "commands",
		Description: "List user-defined slash commands (created with the define_command tool).",
		Usage:       "/commands",
		Handler:     r.handleUserCommands,
	}

	r.commands["think"] = &CommandDefinition{
		Name:        "think",
		Description: "Toggle extended thinking mode (Anthropic models). /think <budget> sets token budget; /think 0 disables.",
		Usage:       "/think [budget]",
		Handler:     r.handleThink,
		IsModifier:  true,
	}

	r.commands["self"] = &CommandDefinition{
		Name:        "self",
		Description: "SENSE self-knowledge layer: inspect, analyse, and evolve Gorkbot's capabilities.",
		Usage:       "/self <schema|check|evolve|fix> [--dry-run] [--min-evidence=N]",
		Handler:     r.handleSelf,
		IsModifier:  true,
	}

	r.commands["env"] = &CommandDefinition{
		Name:        "env",
		Description: "Show the current host environment snapshot (runtimes, API keys, CLI tools, MCP status).",
		Usage:       "/env [refresh]",
		Handler:     r.handleEnv,
		IsModifier:  false,
	}

	r.commands["cascade"] = &CommandDefinition{
		Name:        "cascade",
		Description: "View or set the provider failover cascade order.",
		Usage:       "/cascade  |  /cascade set <p1,p2,...>  |  /cascade reset",
		Handler:     r.handleCascade,
		IsModifier:  true,
	}

	r.commands["sandbox"] = &CommandDefinition{
		Name:        "sandbox",
		Description: "Toggle SENSE input sanitizer (path sandboxing, injection checks)",
		Usage:       "/sandbox  |  /sandbox on  |  /sandbox off",
		Handler:     r.handleSandbox,
		IsModifier:  true,
	}

}

// Command Handlers

func (r *Registry) handleClear(args []string) (string, error) {
	return "CLEAR_SCREEN", nil // Special signal for TUI
}

func (r *Registry) handleHelp(args []string) (string, error) {
	// Groups define display order and category headers.
	// Any command not listed in a group falls into "Other".
	type group struct {
		header string
		names  []string
	}
	groups := []group{
		{"Interface", []string{"clear", "theme", "mouse", "debug", "quit"}},
		{"Session", []string{"save", "resume", "rename", "chat", "export", "rewind"}},
		{"AI & Models", []string{"model", "think", "mode", "cascade", "rate", "compress", "compact"}},
		{"Tools & Permissions", []string{"tools", "permissions", "rules", "key", "auth", "sandbox"}},
		{"Skills & Learning", []string{"skills", "self", "env", "context", "cost"}},
		{"Integrations", []string{"mcp", "a2a", "telegram", "share", "schedule"}},
		{"System", []string{"hooks", "commands", "settings", "version", "about", "bug", "help"}},
	}

	// Track which commands have been placed in a group.
	placed := make(map[string]bool)
	for _, g := range groups {
		for _, n := range g.names {
			placed[n] = true
		}
	}

	// Collect ungrouped commands into "Other".
	var ungrouped []string
	for name := range r.commands {
		if !placed[name] {
			ungrouped = append(ungrouped, name)
		}
	}
	sort.Strings(ungrouped)
	if len(ungrouped) > 0 {
		groups = append(groups, group{"Other", ungrouped})
	}

	var sb strings.Builder
	sb.WriteString("# Gorkbot Commands\n\n")

	for _, g := range groups {
		// Check if any commands in this group actually exist.
		var rows []string
		for _, name := range g.names {
			cmd, exists := r.commands[name]
			if !exists {
				continue
			}
			rows = append(rows, fmt.Sprintf("| `/%-14s` | %-52s | `%s` |",
				cmd.Name, cmd.Description, cmd.Usage))
		}
		if len(rows) == 0 {
			continue
		}
		sb.WriteString("## " + g.header + "\n\n")
		sb.WriteString("| Command | Description | Usage |\n")
		sb.WriteString("|---------|-------------|-------|\n")
		for _, row := range rows {
			sb.WriteString(row + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("**%d commands total** · `Alt+Enter` multi-line input · `Ctrl+R` history search · `Esc Esc` rewind menu\n",
		len(r.commands)))
	return sb.String(), nil
}

func (r *Registry) handleAbout(args []string) (string, error) {
	v := platform.Version
	return "# Gorkbot v" + v + "\n\n" +
		"> A Go-native agentic AI runtime — terminal-first, provider-agnostic, built for serious work.\n\n" +
		"---\n\n" +

		"## Orchestration\n\n" +
		"**Multi-Provider Engine** routes every turn through a priority cascade — if one provider\n" +
		"is unavailable the next fires automatically, with no user intervention required.\n\n" +
		"| Priority | Provider | Role |\n" +
		"|----------|----------|------|\n" +
		"| 1 | xAI Grok | Primary reasoning |\n" +
		"| 2 | Google Gemini | Consultant / fallback |\n" +
		"| 3 | Anthropic Claude | Fallback |\n" +
		"| 4 | MiniMax | Fallback |\n" +
		"| 5 | OpenAI | Fallback |\n" +
		"| 6 | OpenRouter | Last resort |\n\n" +
		"**A2A Protocol** — structured inter-agent communication with typed message kinds\n" +
		"(Query, Response, Notification, ToolRequest), full threading via ReplyTo references,\n" +
		"and a live HTTP task server for external integrations.\n\n" +
		"**Hardened Network Layer** — 15 s TCP keepalive, TLS handshake timeout,\n" +
		"response-header deadline, automatic retry on transient EOF / TLS-MAC / RST errors\n" +
		"with exponential backoff.\n\n" +
		"---\n\n" +

		"## Intelligence\n\n" +
		"**ARC Router** — classifies every prompt into a workflow tier (Direct or ReasonVerify)\n" +
		"and calibrates the tool-call budget to the platform RAM profile via **HALProfile**.\n" +
		"Intent category is stamped on each user message for audit.\n\n" +
		"**MEL — Meta-Experience Learning** — **BifurcationAnalyzer** watches tool\n" +
		"failure→correction cycles and crystallises them into persistent heuristics stored in a\n" +
		"BM25/TF-IDF **VectorStore**. Retrieved at inference time, they steer the model away\n" +
		"from previously observed failure modes.\n\n" +
		"**SENSE Memory Stack** *(cross-session, query-relevant)*\n" +
		"- **AgeMem STM/LTM** — two-tier store; hot facts live in-session, cold facts survive restart\n" +
		"- **Engrams** — explicit behaviour preferences recorded by the agent via **record_engram**,\n" +
		"  persisted to LTM and injected into every system prompt\n" +
		"- **Unified Memory** — single retrieval façade across AgeMem, Engrams, and the MEL VectorStore\n\n" +
		"**SENSE Guards**\n" +
		"- **LIE** — reasoning-depth controller\n" +
		"- **Stabilizer** — output quality gate\n" +
		"- **Code2World** — action preview (shows what a shell command will do before execution)\n" +
		"- **HITL Guard** — human-in-the-loop approval overlay; blocks destructive tool calls\n" +
		"  pending explicit user confirmation\n\n" +
		"**Planning Box** — suppresses internal monologue during generation and surfaces a single\n" +
		"clean intent line, reducing noise and hallucination risk.\n\n" +
		"---\n\n" +

		"## DAG Orchestrator Engine\n\n" +
		"A parallel task-execution runtime that runs dependency-resolved work concurrently in\n" +
		"native Go goroutines — no Python framework overhead.\n\n" +
		"- **Dependency Graph** — topological sort (Kahn's algorithm) with cycle detection;\n" +
		"  tasks only dispatch when all declared dependencies have completed\n" +
		"- **Parallel Execution** — semaphore-based goroutine pool (default: 4 concurrent workers)\n" +
		"- **Retry + RCA** — exponential backoff retries; on exhaustion, an **RCAProvider** call\n" +
		"  diagnoses the failure and embeds the root-cause analysis in the task record\n" +
		"- **Atomic Rollbacks** — **RollbackStore** snapshots files before any task modifies them;\n" +
		"  on failure originals are restored (temp → rename); new files are removed via sentinels\n" +
		"- **Context Pruning** — **Pruner** compresses verbose tool output before LLM injection:\n" +
		"  error/warning lines prioritised, then output tail, then head; hard character ceiling\n" +
		"- **State Persistence** — gob-encoded snapshots written atomically; interrupted graphs\n" +
		"  resume from the last checkpoint with in-flight tasks reset to pending\n" +
		"- **TUI Integration** — live DAG view with per-task progress bars, elapsed timers,\n" +
		"  dependency display, RCA panels, and keyboard navigation\n\n" +
		"---\n\n" +

		"## Tool System\n\n" +
		"**196 registered tools** across 20+ categories, plus unlimited user-defined custom tools.\n\n" +
		"| Category | Count | Highlights |\n" +
		"|----------|------:|------------|\n" +
		"| Shell | 1 | bash |\n" +
		"| File | 9 | read/write/edit/multi-edit/list/search/grep/info/delete |\n" +
		"| Hash-validated File | 3 | read_hashed, edit_hashed, ast_grep |\n" +
		"| Git | 6 | status, diff, log, commit, push, pull |\n" +
		"| Web | 5 | fetch, http_request, check_port, download, x_pull |\n" +
		"| System | 5 | processes, kill, env_var, system_info, disk_usage |\n" +
		"| Meta & Maintenance | 17 | create_tool, modify_tool, audit, context_stats, cron, backup, monitor… |\n" +
		"| Task Management | 3 | todo_write, todo_read, complete |\n" +
		"| Skills / Docs | 8 | consultation, web_search, web_reader, docx, xlsx, pdf, pptx, frontend_design |\n" +
		"| AI / ML | 3 | image_generate, summarize_audio, ml_model_run |\n" +
		"| Background Agents | 3 | spawn_agent, collect_agent, list_agents |\n" +
		"| Database | 2 | db_query, db_migrate |\n" +
		"| Network | 3 | network_scan, socket_connect, network_escape_proxy |\n" +
		"| Media & Content | 8 | image_process, media_convert, ffmpeg_pro, transcribe, tts, ocr_batch, video_summarize, meme |\n" +
		"| Android / Termux | 34 | adb_shell, screen_capture, ui_dump, vision, app_control, logcat, clipboard… |\n" +
		"| Worktree | 4 | create_worktree, list_worktrees, remove_worktree, integrate_worktree |\n" +
		"| DevOps & Cloud | 6 | docker, kubectl, aws_s3, git_blame, ngrok, ci_trigger |\n" +
		"| Security — Basic | 6 | nmap, packet_capture, wifi_analyzer, shodan, metasploit_rpc, ssl_validator |\n" +
		"| Security — Pentest Suite | 28 | masscan, dns_enum, nikto, gobuster, ffuf, sqlmap, hydra, hashcat, john, nuclei… |\n" +
		"| Data Science | 5 | csv_pivot, plot_generate, arxiv_search, web_archive, whois |\n" +
		"| Personal & Life | 4 | calendar, email, contacts, smart_home |\n" +
		"| SENSE | 2 | code2world, record_engram |\n" +
		"| Self-Introspection | 4 | query_routing_stats, query_heuristics, query_memory_state, query_system_state |\n" +
		"| Goal Ledger | 3 | add_goal, close_goal, list_goals |\n" +
		"| Vision | 8 | vision_capture, vision_analyze, vision_screen, vision_ocr, vision_find, vision_watch… |\n" +
		"| Scrapling | 5 | scrapling_fetch, scrapling_stealth, scrapling_dynamic, scrapling_extract, scrapling_search |\n" +
		"| Scheduler | 4 | schedule_task, list_scheduled, cancel_scheduled, pause_resume_scheduled |\n" +
		"| CCI Context | 6 | mcp_context_list_subsystems, get_subsystem, suggest_specialist, update_subsystem… |\n" +
		"| Misc | 4 | jupyter, doc_parser, code_exec, rebuild |\n\n" +
		"**Permission System** — four levels (always · session · once · never) with persistent\n" +
		"JSON storage, shell-safe escaping, and per-call execution timeouts.\n\n" +
		"**Tool Audit Log** — every execution written to a local SQLite database (**audit.db**);\n" +
		"records auto-pruned at 10,000 entries on a 12-hour cycle.\n\n" +
		"**MCP Integration** — connects to any Model Context Protocol server over stdio;\n" +
		"MCP-provided tools appear natively in the registry under the same permission system.\n\n" +
		"**Custom Tool Creator** (**create_tool**) — AI generates a complete Go tool file at\n" +
		"runtime with parameter templating, integrates it into the registry, and makes it\n" +
		"available immediately without a restart.\n\n" +
		"---\n\n" +

		"## Security Subagents\n\n" +
		"Seven specialised red-team agents, each grounded in a dedicated system prompt\n" +
		"and a scoped tool set:\n\n" +
		"**redteam-recon** · **redteam-injection** · **redteam-xss** · **redteam-auth** ·\n" +
		"**redteam-ssrf** · **redteam-authz** · **redteam-reporter**\n\n" +
		"---\n\n" +

		"## Advanced Capabilities\n\n" +
		"**Bee Colony Debate** — parallel multi-role analysis (**Hive.Debate**); independent\n" +
		"analyst bees reason simultaneously then a synthesizer produces a consolidated verdict.\n\n" +
		"**CCI (Contextual Context Injection)** — tiered document layers injected into the\n" +
		"system prompt based on task relevance (project rules, workspace context, user prefs).\n\n" +
		"**Task Scheduler** — cron-style background task runner with persistent job store.\n\n" +
		"**Discovery** — polls xAI and Gemini model APIs continuously; results surface in the\n" +
		"Cloud Brains tab with live capability metadata.\n\n" +
		"**Session Management** — named save / load / resume with full conversation replay.\n\n" +
		"**Skills** — slash-command macros that expand into full prompts; user-definable.\n\n" +
		"---\n\n" +

		"## Terminal UI\n\n" +
		"Built on the Charm Bracelet stack (Bubble Tea · Lip Gloss · Glamour · Bubbles).\n\n" +
		"**Keyboard Shortcuts**\n\n" +
		"| Shortcut | Action |\n" +
		"|----------|--------|\n" +
		"| Ctrl+T | Tools menu |\n" +
		"| Ctrl+S | Settings overlay (Models · Verbosity · Tools · Providers) |\n" +
		"| Ctrl+K | Omni-search — search across entire conversation history |\n" +
		"| Ctrl+O | Expand / collapse latest tool block |\n" +
		"| Ctrl+E | Expand / collapse all tool blocks |\n" +
		"| Ctrl+P | Context status |\n" +
		"| Ctrl+L | Clear & reset session |\n" +
		"| Ctrl+D | Duplicate session |\n" +
		"| Ctrl+R | Force refresh |\n" +
		"| Ctrl+C | Quit |\n" +
		"| Esc+J | Export session to JSON |\n\n" +
		"**Analytics Dashboard** — per-session token counts, tool call rates, provider usage,\n" +
		"and cost estimates.\n\n" +
		"**Bookmarks** — mark and navigate to any conversation position.\n\n" +
		"---\n\n" +

		"## Platform\n\n" +
		"Runs natively on Linux, macOS, Windows, and Android/Termux.\n" +
		"Single static binary, no runtime dependencies. API surface is OpenAI-compatible.\n\n" +
		"---\n\n" +
		"*Gorkbot v" + v + " — Todd Eddings / Velarium AI*", nil
}

func (r *Registry) handleChat(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /chat [save|load|list|delete] [name]", nil
	}

	subcommand := args[0]
	switch subcommand {
	case "save":
		if len(args) < 2 {
			return "Usage: /chat save <name>", nil
		}
		if r.Orch != nil && r.Orch.SaveSession != nil {
			return r.Orch.SaveSession(args[1]), nil
		}
		return fmt.Sprintf("SESSION_SAVE:%s", args[1]), nil

	case "load":
		if len(args) < 2 {
			return "Usage: /chat load <name>", nil
		}
		if r.Orch != nil && r.Orch.LoadSession != nil {
			return r.Orch.LoadSession(args[1]), nil
		}
		return fmt.Sprintf("SESSION_LOAD:%s", args[1]), nil

	case "list":
		if r.Orch != nil && r.Orch.ListSessions != nil {
			return r.Orch.ListSessions(), nil
		}
		return "Session persistence not wired.", nil

	case "delete":
		if len(args) < 2 {
			return "Usage: /chat delete <name>", nil
		}
		return fmt.Sprintf("Use /chat save to overwrite or delete the file manually from the sessions directory."), nil

	default:
		return fmt.Sprintf("Unknown subcommand: %s", subcommand), nil
	}
}

func (r *Registry) handleModel(args []string) (string, error) {
	// No args — show full status and available pool
	if len(args) == 0 {
		return r.modelStatus(), nil
	}

	// Parse: /model [primary|consultant|secondary|specialist] <model-id>
	//     or /model <model-id>  (role inferred from provider)
	role := ""
	modelID := ""

	switch strings.ToLower(args[0]) {
	case "primary":
		if len(args) < 2 {
			return r.modelStatus(), nil
		}
		role = "primary"
		modelID = args[1]
	case "consultant", "secondary", "specialist":
		if len(args) < 2 {
			return r.modelStatus(), nil
		}
		role = "consultant"
		modelID = args[1]
	default:
		// Bare model ID — infer role from provider
		modelID = args[0]
		role = r.inferRole(modelID)
	}

	// Validate against the known model pool
	found := false
	for _, m := range r.availableModels {
		if m.ID == modelID {
			found = true
			break
		}
	}
	if !found {
		// Soft fail — unknown but forward to TUI anyway with a warning note in output
		if len(r.availableModels) == 0 {
			// Registry not populated yet, fall through with signal
			found = true
		}
	}
	if !found {
		return fmt.Sprintf("Unknown model: `%s`\n\nUse `/model` to see available models.", modelID), nil
	}

	// Return role-scoped signal for the TUI to act on.
	// Include provider info when Orch adapter is available.
	provider := r.inferProvider(modelID)
	if role == "primary" {
		if r.Orch != nil && r.Orch.SetPrimary != nil {
			return r.Orch.SetPrimary(provider, modelID), nil
		}
		return fmt.Sprintf("MODEL_SWITCH_PRIMARY:%s:%s", provider, modelID), nil
	}
	// secondary/consultant
	if modelID == "auto" || modelID == "" {
		if r.Orch != nil && r.Orch.SetAutoSecondary != nil {
			return r.Orch.SetAutoSecondary(), nil
		}
		return "MODEL_SECONDARY_AUTO", nil
	}
	if r.Orch != nil && r.Orch.SetSecondary != nil {
		return r.Orch.SetSecondary(provider, modelID), nil
	}
	return fmt.Sprintf("MODEL_SWITCH_CONSULTANT:%s:%s", provider, modelID), nil
}

// modelStatus builds the full /model status display using live registry data.
func (r *Registry) modelStatus() string {
	var sb strings.Builder
	sb.WriteString("# Model Configuration\n\n")

	// Active models
	sb.WriteString("## Active Models\n\n")
	if r.currentPrimary.ID != "" {
		thinking := ""
		if r.currentPrimary.Thinking {
			thinking = " _(reasoning)_"
		}
		sb.WriteString(fmt.Sprintf("**Primary:** `%s` — %s%s\n",
			r.currentPrimary.ID, r.currentPrimary.Name, thinking))
	} else {
		sb.WriteString("**Primary:** _(not selected)_\n")
	}
	if r.currentConsultant.ID != "" {
		thinking := ""
		if r.currentConsultant.Thinking {
			thinking = " _(reasoning)_"
		}
		sb.WriteString(fmt.Sprintf("**Consultant:** `%s` — %s%s\n",
			r.currentConsultant.ID, r.currentConsultant.Name, thinking))
	} else {
		sb.WriteString("**Consultant:** _(disabled — no Gemini API key)_\n")
	}
	sb.WriteString("\n")

	// Available pool
	if len(r.availableModels) > 0 {
		sb.WriteString("## Available Models\n\n")

		providerOrder := []struct{ id, label, role string }{
			{"xai", "xAI (Grok)", "primary"},
			{"google", "Google (Gemini)", "consultant"},
		}

		for _, prov := range providerOrder {
			var group []ModelInfo
			for _, m := range r.availableModels {
				if m.Provider == prov.id {
					group = append(group, m)
				}
			}
			if len(group) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("**%s** — role: `%s`\n", prov.label, prov.role))
			for _, m := range group {
				thinking := ""
				if m.Thinking {
					thinking = " 🧠"
				}
				active := ""
				if m.ID == r.currentPrimary.ID || m.ID == r.currentConsultant.ID {
					active = " ✓"
				}
				sb.WriteString(fmt.Sprintf("  • `%s` — %s%s%s\n", m.ID, m.Name, thinking, active))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("_Model registry not yet populated. Start with defaults._\n\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString("**Usage:**\n")
	sb.WriteString("- `/model primary <id>` — switch primary (xAI/Grok) model\n")
	sb.WriteString("- `/model consultant <id>` — switch consultant (Gemini) model\n")
	sb.WriteString("- `/model <id>` — switch by ID (role auto-detected from provider)\n")

	return sb.String()
}

// inferRole returns "primary" for xAI models and "consultant" for Google models.
func (r *Registry) inferRole(modelID string) string {
	for _, m := range r.availableModels {
		if m.ID == modelID {
			if m.Provider == "google" {
				return "consultant"
			}
			return "primary"
		}
	}
	// Fallback heuristic: gemini → consultant, everything else → primary
	lower := strings.ToLower(modelID)
	if strings.Contains(lower, "gemini") || strings.Contains(lower, "palm") {
		return "consultant"
	}
	return "primary"
}

// inferProvider returns the provider ID for a model ID.
func (r *Registry) inferProvider(modelID string) string {
	for _, m := range r.availableModels {
		if m.ID == modelID {
			return m.Provider
		}
	}
	lower := strings.ToLower(modelID)
	if strings.Contains(lower, "gemini") || strings.Contains(lower, "palm") {
		return "google"
	}
	if strings.Contains(lower, "claude") {
		return "anthropic"
	}
	if strings.HasPrefix(lower, "gpt-") || strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") || strings.HasPrefix(lower, "o4") {
		return "openai"
	}
	if strings.Contains(lower, "minimax") {
		return "minimax"
	}
	return "xai"
}

func (r *Registry) handleTools(args []string) (string, error) {
	// /tools audit — all-time persistent audit log
	if len(args) > 0 && strings.ToLower(args[0]) == "audit" {
		if r.Orch != nil && r.Orch.GetToolCallStats != nil {
			return r.Orch.GetToolCallStats(), nil
		}
		return "Audit DB not available.", nil
	}

	// /tools errors [tool_name] — recent failures from audit DB
	if len(args) > 0 && strings.ToLower(args[0]) == "errors" {
		toolFilter := ""
		if len(args) > 1 {
			toolFilter = args[1]
		}
		if r.toolRegistry == nil {
			return "Tool registry not initialized.", nil
		}
		if adb := r.toolRegistry.GetAuditDB(); adb != nil {
			return adb.RecentErrors(20, toolFilter), nil
		}
		return "Audit DB not initialized.", nil
	}

	// /tools stats — session + persistent analytics dashboard
	if len(args) > 0 && strings.ToLower(args[0]) == "stats" {
		if r.toolRegistry == nil {
			return "Tool registry not initialized.", nil
		}
		analytics := r.toolRegistry.GetAnalytics()
		if analytics == nil {
			return "Analytics not available (not initialized).", nil
		}
		result := "# Tool Analytics Dashboard\n\n```\n" + analytics.GetSummary() + "\n```\n\n" +
			"_Data stored at: " + analytics.GetConfigPath() + "_"
		// Append persistent audit DB stats.
		if r.Orch != nil && r.Orch.GetToolCallStats != nil {
			sqliteStats := r.Orch.GetToolCallStats()
			if sqliteStats != "" {
				result += "\n\n" + sqliteStats
			}
		}
		return result, nil
	}

	var sb strings.Builder
	sb.WriteString("# 🔧 Tool System\n\n")

	// Check if tool registry is available
	if r.toolRegistry == nil {
		sb.WriteString("_Tool registry not initialized_\n\n")
		sb.WriteString("The tool system is not currently available.\n")
		return sb.String(), nil
	}

	toolsList := r.toolRegistry.List()
	if len(toolsList) == 0 {
		sb.WriteString("No tools registered.\n")
		return sb.String(), nil
	}

	// Group tools by category
	categories := make(map[string][]string)
	for _, tool := range toolsList {
		name := tool.Name()
		category := fmt.Sprintf("%v", tool.Category())
		desc := tool.Description()

		// Truncate description if too long
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}

		entry := fmt.Sprintf("**%s** - %s", name, desc)
		categories[category] = append(categories[category], entry)
	}

	sb.WriteString(fmt.Sprintf("**Total Tools:** %d\n\n", len(toolsList)))

	// Display by category
	categoryOrder := []string{"shell", "file", "git", "web", "system", "meta", "custom"}
	categoryIcons := map[string]string{
		"shell":  "🐚",
		"file":   "📁",
		"git":    "🔀",
		"web":    "🌐",
		"system": "💻",
		"meta":   "🔧",
		"custom": "🎨",
	}

	for _, cat := range categoryOrder {
		if toolList, exists := categories[cat]; exists && len(toolList) > 0 {
			icon := categoryIcons[cat]
			if icon == "" {
				icon = "•"
			}
			sb.WriteString(fmt.Sprintf("## %s %s (%d)\n\n", icon, strings.Title(cat), len(toolList)))
			for _, tool := range toolList {
				sb.WriteString(fmt.Sprintf("• %s\n", tool))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("💡 **Tip:** Tools are automatically available to the AI.\n")
	sb.WriteString("Permissions are managed interactively when tools are first used.\n")

	return sb.String(), nil
}

func (r *Registry) handleAuth(args []string) (string, error) {
	if len(args) == 0 {
		return r.authHelp(), nil
	}

	// /auth <service> <subcommand> [args…]
	service := strings.ToLower(args[0])
	sub := ""
	if len(args) > 1 {
		sub = strings.ToLower(args[1])
	}

	switch service {
	case "notebooklm":
		return r.handleAuthNotebookLM(sub, args[2:])
	case "status":
		// Legacy: /auth status
		return r.authStatusAll(), nil
	case "refresh":
		// Legacy: /auth refresh
		return "🔄 Refreshing OAuth tokens… use `/auth notebooklm login` for interactive flows.", nil
	default:
		return r.authHelp(), nil
	}
}

func (r *Registry) authHelp() string {
	return `**Authentication Commands**

` + "```" + `
/auth status                     — Show all auth states
/auth notebooklm status          — NotebookLM / Google auth status
/auth notebooklm setup <id> [secret] — Save Google OAuth client credentials
/auth notebooklm login           — Authenticate via Google Device Flow
/auth notebooklm logout          — Revoke stored Google OAuth token
` + "```"
}

func (r *Registry) authStatusAll() string {
	var sb strings.Builder
	sb.WriteString("**Authentication Status**\n\n")

	// AI providers: check env vars directly.
	if os.Getenv("XAI_API_KEY") != "" {
		sb.WriteString("✅ xAI (Grok): API key configured\n")
	} else {
		sb.WriteString("❌ xAI (Grok): no API key\n")
	}
	if os.Getenv("GEMINI_API_KEY") != "" {
		sb.WriteString("✅ Google Gemini: API key configured\n")
	} else {
		sb.WriteString("❌ Google Gemini: no API key  →  add GEMINI_API_KEY to .env\n")
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		sb.WriteString("✅ Anthropic: API key configured\n")
	}

	// Google OAuth.
	sb.WriteString("\n**Google OAuth (Drive / NotebookLM):**\n")
	if r.GoogleAuthStatus != nil {
		sb.WriteString(r.GoogleAuthStatus())
		sb.WriteString("\n")
	} else {
		sb.WriteString("⚠️  not configured\n")
	}

	return sb.String()
}

func (r *Registry) handleAuthNotebookLM(sub string, extraArgs []string) (string, error) {
	switch sub {
	case "status", "":
		if r.GoogleAuthStatus == nil {
			return "⚠️  NotebookLM auth is not configured in this session.", nil
		}
		return fmt.Sprintf("**NotebookLM / Google Auth**\n\n%s", r.GoogleAuthStatus()), nil

	case "setup":
		if r.GoogleAuthSetup == nil {
			return "❌ Auth setup is not available in this session.", nil
		}
		if len(extraArgs) == 0 {
			return ("Usage: `/auth notebooklm setup <client_id> [client_secret]`\n\n" +
				"Get your client_id from https://console.cloud.google.com/apis/credentials\n" +
				"Application type: **Desktop app** (Installed application)"), nil
		}
		clientID := extraArgs[0]
		clientSecret := ""
		if len(extraArgs) > 1 {
			clientSecret = extraArgs[1]
		}
		if err := r.GoogleAuthSetup(clientID, clientSecret); err != nil {
			return "", fmt.Errorf("setup failed: %w", err)
		}
		return ("✅ Google OAuth credentials saved.\n\n" +
			"Now run `/auth notebooklm login` to authenticate."), nil

	case "login":
		if r.GoogleAuthLogin == nil {
			return "❌ Auth login is not available in this session.", nil
		}
		instructions, deviceCode, err := r.GoogleAuthLogin()
		if err != nil {
			return "", fmt.Errorf("auth login failed: %w", err)
		}
		if deviceCode == "" {
			// Already authenticated, or setup required.
			return instructions, nil
		}
		// Start polling in background; return instructions immediately so the
		// TUI displays them to the user without blocking.
		if r.GoogleAuthPoll != nil {
			go func() {
				if err := r.GoogleAuthPoll(deviceCode); err != nil {
					// Non-fatal: user will re-run /auth notebooklm login if needed.
					_ = err
				}
			}()
		}
		return instructions, nil

	case "poll":
		// Manual re-poll: for use if the background goroutine missed the completion.
		return ("🔄 Auth polling runs automatically after `/auth notebooklm login`.\n" +
			"If the token was granted, the next NotebookLM tool call will succeed.\n" +
			"Run `/auth notebooklm status` to verify."), nil

	case "logout":
		if r.GoogleAuthLogout == nil {
			return "❌ Auth logout is not available in this session.", nil
		}
		if err := r.GoogleAuthLogout(); err != nil {
			return "", fmt.Errorf("logout failed: %w", err)
		}
		return "✅ Google OAuth token revoked. Run `/auth notebooklm login` to re-authenticate.", nil

	default:
		return fmt.Sprintf("Unknown NotebookLM auth subcommand: `%s`\n\n%s", sub, r.authHelp()), nil
	}
}

func (r *Registry) handleSettings(args []string) (string, error) {
	// Signal the TUI to open the interactive settings overlay.
	return "SETTINGS_MODAL", nil
}

func (r *Registry) handleVerbose(args []string) (string, error) {
	// Get current state
	if r.Orch == nil || r.Orch.GetVerboseMode == nil {
		return "Verbose mode is not available", nil
	}

	currentState := r.Orch.GetVerboseMode()

	// Parse argument
	var newState bool
	if len(args) == 0 {
		// Toggle
		newState = !currentState
	} else {
		switch strings.ToLower(args[0]) {
		case "on", "yes", "true", "1":
			newState = true
		case "off", "no", "false", "0":
			newState = false
		case "toggle":
			newState = !currentState
		default:
			return fmt.Sprintf("Invalid argument: %s. Use 'on', 'off', or 'toggle'", args[0]), nil
		}
	}

	// Set new state
	if r.Orch.SetVerboseMode != nil {
		if err := r.Orch.SetVerboseMode(newState); err != nil {
			return fmt.Sprintf("Error setting verbose mode: %v", err), err
		}
	}

	statusStr := "off (suppressing internal system messages)"
	if newState {
		statusStr = "on (showing all messages including system narration)"
	}
	return fmt.Sprintf("Verbose mode is now **%s**", statusStr), nil
}

// GetToolRegistry exposes the tool registry for use by the settings overlay.
func (r *Registry) GetToolRegistry() *tools.Registry {
	return r.toolRegistry
}

func (r *Registry) handleVersion(args []string) (string, error) {
	buildTime := time.Now().Format("2006-01-02")

	return fmt.Sprintf(`**Gorkbot v%s**

Build: %s
Go: %s
OS: %s/%s

🦙 Powered by Grok & Gemini`, platform.Version, buildTime, runtime.Version(), runtime.GOOS, runtime.GOARCH), nil
}

func (r *Registry) handleQuit(args []string) (string, error) {
	return "QUIT", nil // Special signal for TUI
}

func (r *Registry) handleBug(args []string) (string, error) {
	url := "https://github.com/velariumai/gorkbot/issues/new?template=bug_report.md"

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("termux-open-url"); err == nil {
			cmd = exec.Command("termux-open-url", url)
		} else {
			cmd = exec.Command("xdg-open", url)
		}
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Sprintf("Please open: %s", url), nil
	}

	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("Failed to open browser. Please visit:\n%s", url), nil
	}

	return "🐛 Opened GitHub issue template in browser", nil
}

func (r *Registry) handleTheme(args []string) (string, error) {
	// If theme manager is wired, use it.
	if r.ThemeList != nil {
		if len(args) == 0 {
			return r.ThemeList(), nil
		}
		name := strings.ToLower(args[0])
		if r.ThemeSet != nil {
			if err := r.ThemeSet(name); err != nil {
				return fmt.Sprintf("Theme error: %v", err), nil
			}
			return fmt.Sprintf("Theme set to **%s**. Changes apply on next TUI restart.", name), nil
		}
	}

	// Legacy fallback (no theme manager wired)
	if len(args) == 0 {
		active := "dark"
		if r.ThemeActive != nil {
			active = r.ThemeActive()
		}
		return fmt.Sprintf("Current theme: **%s**\n\nAvailable themes:\n• dracula (default)\n• nord\n• gruvbox\n• solarized\n• monokai\n\nUsage: `/theme <name>`", active), nil
	}
	return fmt.Sprintf("THEME:%s", args[0]), nil
}

func (r *Registry) handleMCP(args []string) (string, error) {
	subCmd := "status"
	if len(args) > 0 {
		subCmd = strings.ToLower(args[0])
	}

	switch subCmd {
	case "config":
		path := "~/.config/gorkbot/mcp.json"
		if r.MCPConfigPath != nil {
			path = r.MCPConfigPath()
		}
		return fmt.Sprintf("# MCP Configuration\n\nConfig file: `%s`\n\nEdit this file to add MCP servers.\n\n**Example:**\n```json\n{\n  \"servers\": [\n    {\n      \"name\": \"filesystem\",\n      \"transport\": \"stdio\",\n      \"command\": \"npx\",\n      \"args\": [\"-y\", \"@modelcontextprotocol/server-filesystem\", \"/tmp\"]\n    }\n  ]\n}\n```", path), nil
	case "reload":
		if r.MCPReload == nil {
			return "# MCP — Reload\n\nReload not available (MCP manager not wired).", nil
		}
		n, err := r.MCPReload()
		if err != nil {
			return fmt.Sprintf("# MCP — Reload Failed\n\n%v", err), nil
		}
		status := ""
		if r.MCPStatus != nil {
			status = "\n\n" + r.MCPStatus()
		}
		return fmt.Sprintf("# MCP — Reloaded\n\n%d server(s) connected.%s", n, status), nil
	default: // status
		if r.MCPStatus != nil {
			return "# MCP — Model Context Protocol\n\n" + r.MCPStatus(), nil
		}
		return "# MCP — Model Context Protocol\n\nMCP manager not initialized. Start the TUI to activate MCP servers.", nil
	}
}

func (r *Registry) handleCompress(args []string) (string, error) {
	if r.Orch == nil || r.Orch.CompactFocus == nil {
		return "⚠️ Orchestrator not connected", nil
	}

	focus := ""
	if len(args) > 0 {
		focus = strings.Join(args, " ")
	}

	result := r.Orch.CompactFocus(focus)
	return result, nil
}

func (r *Registry) handleMouse(args []string) (string, error) {
	return "MOUSE_TOGGLE", nil
}

func (r *Registry) handlePermissions(args []string) (string, error) {
	if r.toolRegistry == nil {
		return "⚠️ Tool registry not available", nil
	}

	// Default to list if no args
	action := "list"
	if len(args) > 0 {
		action = args[0]
	}

	switch action {
	case "list":
		return r.listPermissions()

	case "reset":
		if len(args) > 1 {
			// Reset specific tool
			toolName := args[1]
			return r.resetToolPermission(toolName)
		}
		// Reset all permissions
		return r.resetAllPermissions()

	default:
		return fmt.Sprintf("Unknown action: %s\n\nUsage: /permissions [list|reset|reset <tool>]", action), nil
	}
}

func (r *Registry) listPermissions() (string, error) {
	var sb strings.Builder
	sb.WriteString("# 🔐 Tool Permissions\n\n")

	// Get all tools
	toolsList := r.toolRegistry.List()
	if len(toolsList) == 0 {
		sb.WriteString("No tools registered.\n")
		return sb.String(), nil
	}

	// Get permission manager
	permMgr := r.getPermissionManager()
	if permMgr == nil {
		sb.WriteString("⚠️ Permission manager not available\n")
		return sb.String(), nil
	}

	// Count permissions by level
	counts := map[string]int{
		"always":  0,
		"session": 0,
		"once":    0,
		"never":   0,
	}

	// Categorize tools by permission
	byPermission := make(map[string][]string)

	for _, tool := range toolsList {
		toolName := tool.Name()
		perm := permMgr.GetPermission(toolName)

		permStr := "once" // default
		switch perm {
		case tools.PermissionAlways:
			permStr = "always"
			counts["always"]++
		case tools.PermissionSession:
			permStr = "session"
			counts["session"]++
		case tools.PermissionNever:
			permStr = "never"
			counts["never"]++
		default:
			counts["once"]++
		}

		byPermission[permStr] = append(byPermission[permStr], toolName)
	}

	// Summary
	sb.WriteString(fmt.Sprintf("**Total Tools:** %d\n", len(toolsList)))
	sb.WriteString(fmt.Sprintf("- ✅ Always: %d\n", counts["always"]))
	sb.WriteString(fmt.Sprintf("- 🔓 Session: %d\n", counts["session"]))
	sb.WriteString(fmt.Sprintf("- ❓ Once: %d\n", counts["once"]))
	sb.WriteString(fmt.Sprintf("- ❌ Never: %d\n\n", counts["never"]))

	// List by permission level
	if len(byPermission["always"]) > 0 {
		sb.WriteString("## ✅ Always Allowed\n")
		for _, tool := range byPermission["always"] {
			sb.WriteString(fmt.Sprintf("- `%s`\n", tool))
		}
		sb.WriteString("\n")
	}

	if len(byPermission["session"]) > 0 {
		sb.WriteString("## 🔓 Session (current session only)\n")
		for _, tool := range byPermission["session"] {
			sb.WriteString(fmt.Sprintf("- `%s`\n", tool))
		}
		sb.WriteString("\n")
	}

	if len(byPermission["never"]) > 0 {
		sb.WriteString("## ❌ Blocked\n")
		for _, tool := range byPermission["never"] {
			sb.WriteString(fmt.Sprintf("- `%s`\n", tool))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString("**Commands:**\n")
	sb.WriteString("- `/permissions reset` - Reset all permissions\n")
	sb.WriteString("- `/permissions reset <tool>` - Reset specific tool\n")
	sb.WriteString("\n💡 **Tip:** Session permissions are cleared when you exit Grokster.\n")

	return sb.String(), nil
}

func (r *Registry) resetAllPermissions() (string, error) {
	permMgr := r.getPermissionManager()
	if permMgr == nil {
		return "⚠️ Permission manager not available", nil
	}

	err := permMgr.ClearAll()
	if err != nil {
		return fmt.Sprintf("❌ Failed to reset permissions: %v", err), err
	}

	// Also clear session permissions
	r.toolRegistry.ClearSessionPermissions()

	return "✅ **All permissions reset**\n\nAll tools will require permission approval on next use.", nil
}

func (r *Registry) resetToolPermission(toolName string) (string, error) {
	permMgr := r.getPermissionManager()
	if permMgr == nil {
		return "⚠️ Permission manager not available", nil
	}

	// Check if tool exists
	if _, exists := r.toolRegistry.Get(toolName); !exists {
		return fmt.Sprintf("❌ Tool not found: `%s`\n\nUse `/tools` to see available tools.", toolName), nil
	}

	// Remove permission
	permMgr.RemovePermission(toolName)

	// Also clear from session
	r.toolRegistry.RevokeSessionPermission(toolName)

	return fmt.Sprintf("✅ Permission reset for `%s`\n\nYou will be asked for permission next time this tool is used.", toolName), nil
}

func (r *Registry) getPermissionManager() *tools.PermissionManager {
	if r.toolRegistry == nil {
		return nil
	}
	return r.toolRegistry.GetPermissionManager()
}

// ─── New P0/P1/P2 command handlers ──────────────────────────────────────────

func (r *Registry) handleContext(args []string) (string, error) {
	if r.Orch != nil && r.Orch.GetContextReport != nil {
		return r.Orch.GetContextReport(), nil
	}
	return "# Context Window\n\nContext tracking not yet available. Start a conversation to enable tracking.", nil
}

func (r *Registry) handleCost(args []string) (string, error) {
	var sb strings.Builder
	if r.Orch != nil && r.Orch.GetCostReport != nil {
		sb.WriteString(r.Orch.GetCostReport())
		sb.WriteString("\n\n")
	}
	if r.Orch != nil && r.Orch.BillingGet != nil {
		sb.WriteString(r.Orch.BillingGet())
		sb.WriteString("\n\n")
	}
	if r.Orch != nil && r.Orch.BillingGetAllTime != nil {
		sb.WriteString(r.Orch.BillingGetAllTime())
	}
	if sb.Len() == 0 {
		return "Cost tracking not available.", nil
	}
	return sb.String(), nil
}

func (r *Registry) handleRewind(args []string) (string, error) {
	if r.Orch == nil || r.Orch.RewindTo == nil {
		return "Rewind system not available.", nil
	}
	if len(args) == 0 {
		// Show checkpoint list
		if r.Orch.GetCheckpoints != nil {
			return r.Orch.GetCheckpoints(), nil
		}
		return "No checkpoints available.", nil
	}
	return r.Orch.RewindTo(args[0]), nil
}

func (r *Registry) handleMode(args []string) (string, error) {
	if r.Orch == nil {
		return "Mode management not available.", nil
	}
	if len(args) == 0 {
		mode := "NORMAL"
		if r.Orch.GetMode != nil {
			mode = r.Orch.GetMode()
		}
		return fmt.Sprintf("# Execution Mode\n\nCurrent mode: **%s**\n\nAvailable modes:\n- `normal` — All tools available (default)\n- `plan` — Read-only: write/execute tools blocked\n- `auto` — Auto-approve file edits\n\n**Usage:** `/mode plan` or `Ctrl+P` to cycle\n", mode), nil
	}
	if r.Orch.SetMode != nil {
		newMode := r.Orch.SetMode(args[0])
		return fmt.Sprintf("MODE_CHANGE:%s", newMode), nil // Special signal for TUI
	}
	return fmt.Sprintf("Set mode to: %s", args[0]), nil
}

func (r *Registry) handleExport(args []string) (string, error) {
	format := "md"
	path := ""
	if len(args) >= 1 {
		format = strings.ToLower(args[0])
	}
	if len(args) >= 2 {
		path = args[1]
	}
	ext := formatExt(format)
	if path == "" {
		path = fmt.Sprintf("%s/gorkbot_export_%s.%s",
			homeDir(),
			time.Now().Format("20060102_150405"),
			ext)
	}
	// Signal the TUI to export its full message history (includes tool calls,
	// results, system messages — not just the AI conversation buffer).
	return fmt.Sprintf("EXPORT_TUI:%s:%s", ext, path), nil
}

// homeDir returns the user's home directory, falling back to "~".
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "~"
}

func formatExt(format string) string {
	switch format {
	case "txt", "text", "plain":
		return "txt"
	case "pdf":
		return "pdf"
	default:
		return "md"
	}
}

func (r *Registry) handleCompactEnhanced(args []string) (string, error) {
	if r.Orch != nil && r.Orch.CompactFocus != nil {
		focus := strings.Join(args, " ")
		result := r.Orch.CompactFocus(focus)
		return result, nil
	}
	// Fall back to original handler
	return r.handleCompress(args)
}

func (r *Registry) handleSkills(args []string) (string, error) {
	if r.SkillsFormat != nil {
		return r.SkillsFormat(), nil
	}
	return "# Skills\n\nSkill system not yet configured.\n\nCreate skill files in `~/.config/gorkbot/skills/` or `.gorkbot/skills/`\n\nSee the documentation for the skill file format.", nil
}

func (r *Registry) handleRules(args []string) (string, error) {
	if len(args) == 0 || args[0] == "list" {
		if r.RulesFormat != nil {
			return r.RulesFormat(), nil
		}
		return "# Permission Rules\n\nNo rules configured.\n\n**Usage:**\n- `/rules add allow \"bash(git status)\"` — always allow\n- `/rules add deny \"bash(rm -rf*)\"` — always block\n- `/rules add ask \"bash(git push*)\"` — always ask\n- `/rules remove allow \"bash(git status)\"` — remove rule\n", nil
	}
	action := args[0]
	switch action {
	case "add":
		if len(args) < 3 {
			return "Usage: /rules add <allow|ask|deny> <pattern> [comment]", nil
		}
		decision := args[1]
		pattern := args[2]
		comment := ""
		if len(args) > 3 {
			comment = strings.Join(args[3:], " ")
		}
		if r.RulesAdd != nil {
			if err := r.RulesAdd(decision, pattern, comment); err != nil {
				return fmt.Sprintf("Failed to add rule: %v", err), nil
			}
			return fmt.Sprintf("Rule added: %s `%s`", decision, pattern), nil
		}
		return "Rule engine not configured.", nil
	case "remove":
		if len(args) < 3 {
			return "Usage: /rules remove <allow|ask|deny> <pattern>", nil
		}
		if r.RulesRemove != nil {
			if err := r.RulesRemove(args[1], args[2]); err != nil {
				return fmt.Sprintf("Failed to remove rule: %v", err), nil
			}
			return fmt.Sprintf("Rule removed: %s `%s`", args[1], args[2]), nil
		}
		return "Rule engine not configured.", nil
	default:
		return fmt.Sprintf("Unknown action: %s\n\nUsage: /rules [list|add|remove]", action), nil
	}
}

func (r *Registry) handleHooks(args []string) (string, error) {
	var sb strings.Builder
	sb.WriteString("# Lifecycle Hooks\n\n")
	sb.WriteString("Hook scripts in `~/.config/gorkbot/hooks/` run at lifecycle events.\n\n")
	sb.WriteString("## Available Events\n\n")
	events := [][]string{
		{"session_start", "Fires when Gorkbot starts"},
		{"session_end", "Fires when Gorkbot exits"},
		{"pre_tool_use", "Fires before a tool executes (exit 2 = block)"},
		{"post_tool_use", "Fires after a tool completes"},
		{"post_tool_failure", "Fires after a tool fails"},
		{"pre_compaction", "Fires before context compression"},
		{"on_notification", "Fires for AI notifications"},
		{"mode_change", "Fires when execution mode changes"},
	}
	for _, e := range events {
		sb.WriteString(fmt.Sprintf("- **%s** — %s\n", e[0], e[1]))
	}
	sb.WriteString("\n## Hook Protocol\n\n")
	sb.WriteString("Scripts receive a JSON payload on stdin.\n")
	sb.WriteString("- Exit 0: proceed\n- Exit 2: block action (stderr = reason)\n\n")
	sb.WriteString("**Example** (`~/.config/gorkbot/hooks/pre_tool_use.sh`):\n")
	sb.WriteString("```sh\n#!/bin/sh\npayload=$(cat)\n# exit 2 to block; echo reason >&2\nexit 0\n```\n")
	if len(args) > 0 && args[0] == "dir" {
		homeDir, _ := os.UserHomeDir()
		sb.WriteString(fmt.Sprintf("\nHooks directory: `%s/.config/gorkbot/hooks/`\n", homeDir))
	}
	return sb.String(), nil
}

func (r *Registry) handleRename(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /rename <session-name>", nil
	}
	name := strings.Join(args, "-")
	return fmt.Sprintf("SESSION_RENAME:%s", name), nil // Signal for TUI
}

func (r *Registry) handleSave(args []string) (string, error) {
	// Empty name → orchestrator auto-generates one from the conversation.
	name := strings.Join(args, "-")
	if r.Orch != nil && r.Orch.SaveSession != nil {
		return r.Orch.SaveSession(name), nil
	}
	return "Session save not available (orchestrator not wired).", nil
}

func (r *Registry) handleResume(args []string) (string, error) {
	if len(args) == 0 {
		// List sessions when called with no args.
		if r.Orch != nil && r.Orch.ListSessions != nil {
			return r.Orch.ListSessions(), nil
		}
		return "Usage: /resume <name>", nil
	}
	if args[0] == "list" {
		if r.Orch != nil && r.Orch.ListSessions != nil {
			return r.Orch.ListSessions(), nil
		}
		return "Session list not available.", nil
	}
	name := strings.Join(args, "-")
	if r.Orch != nil && r.Orch.LoadSession != nil {
		return r.Orch.LoadSession(name), nil
	}
	return "Session resume not available (orchestrator not wired).", nil
}

func (r *Registry) handleRate(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /rate <1-5>  — rates the last AI response for adaptive model learning.", nil
	}
	var score float64
	if _, err := fmt.Sscanf(args[0], "%f", &score); err != nil || score < 1 || score > 5 {
		return "Rating must be a number between 1 and 5.", nil
	}
	if r.Orch != nil && r.Orch.RateResponse != nil {
		return r.Orch.RateResponse(score), nil
	}
	return "Rating not available (adaptive router not wired).", nil
}

func (r *Registry) handleShare(args []string) (string, error) {
	action := "start"
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}
	if r.Orch == nil {
		return "Share not available (orchestrator not wired).", nil
	}
	switch action {
	case "stop":
		if r.Orch.StopRelay != nil {
			r.Orch.StopRelay()
			return "Session sharing stopped.", nil
		}
		return "Relay stop not available.", nil
	default: // "start"
		if r.Orch.StartRelay != nil {
			url := r.Orch.StartRelay()
			if url == "" {
				return "Failed to start relay (already active or error).", nil
			}
			return fmt.Sprintf("Session sharing active.\nObserver URL: %s\n\nConnect with: gorkbot --join <host:port>", url), nil
		}
		return "Relay start not available.", nil
	}
}

func (r *Registry) handleDebug(args []string) (string, error) {
	if r.Orch == nil || r.Orch.ToggleDebug == nil {
		return "Debug mode not available (orchestrator not wired).", nil
	}
	on := r.Orch.ToggleDebug()
	if on {
		return "DEBUG_ON", nil // Signal for TUI
	}
	return "DEBUG_OFF", nil // Signal for TUI
}

func (r *Registry) handleThink(args []string) (string, error) {
	if r.Orch == nil || r.Orch.SetThinkingBudget == nil {
		return "Extended thinking not available (orchestrator not wired).", nil
	}
	budget := 8000 // default budget when toggling on
	if len(args) > 0 {
		n := 0
		for _, ch := range args[0] {
			if ch >= '0' && ch <= '9' {
				n = n*10 + int(ch-'0')
			} else {
				return fmt.Sprintf("Invalid budget %q — must be a non-negative integer.", args[0]), nil
			}
		}
		budget = n
	}
	return r.Orch.SetThinkingBudget(budget), nil
}

func (r *Registry) handleSchedule(args []string) (string, error) {
	if r.Orch != nil && r.Orch.GetScheduledTasks != nil {
		return r.Orch.GetScheduledTasks(), nil
	}
	return "Scheduler not available. Start Gorkbot normally to enable scheduled tasks.", nil
}

func (r *Registry) handleTelegram(args []string) (string, error) {
	if r.Orch != nil && r.Orch.GetTelegramStatus != nil {
		return r.Orch.GetTelegramStatus(), nil
	}
	return "Telegram integration not configured. Add token to ~/.config/gorkbot/telegram.json.", nil
}

func (r *Registry) handleA2A(args []string) (string, error) {
	return "**A2A HTTP Gateway**\n\nStart with `--a2a` flag (default addr: 127.0.0.1:18890).\n\n" +
		"Endpoints:\n" +
		"- `POST /a2a/v1` — JSON-RPC 2.0 (methods: message/send, tasks/get, tasks/cancel)\n" +
		"- `GET /.well-known/agent.json` — Agent Card\n" +
		"- `GET /a2a/health` — Health check\n", nil
}

func (r *Registry) handleUserCommands(args []string) (string, error) {
	if r.UserCmdsGet == nil {
		return "User commands not available.", nil
	}
	result, ok := r.UserCmdsGet("__list__", "")
	if !ok || result == "" {
		return "No user-defined commands yet.\n\nUse the `define_command` tool to create one.\nExample: ask the AI to `/define_command summarize \"Summarize this: {{args}}\"`", nil
	}
	return result, nil
}

// handleKey implements /key <provider> <api-key> | /key status | /key validate <provider>
func (r *Registry) handleKey(args []string) (string, error) {
	if len(args) == 0 || (len(args) == 1 && strings.ToLower(args[0]) == "status") {
		if r.Orch != nil && r.Orch.GetProviderStatus != nil {
			return "# Provider API Key Status\n\n```\n" + r.Orch.GetProviderStatus() + "```\n", nil
		}
		return "Provider status not available.", nil
	}

	if strings.ToLower(args[0]) == "validate" {
		if len(args) < 2 {
			return "Usage: /key validate <provider>", nil
		}
		provider := strings.ToLower(args[1])
		if r.Orch != nil && r.Orch.SetProviderKey != nil {
			return r.Orch.SetProviderKey(provider, ""), nil
		}
		return "Key validation not available.", nil
	}

	// /key <provider> <api-key>
	if len(args) < 2 {
		return "Usage: /key <provider> <api-key>", nil
	}
	provider := strings.ToLower(args[0])
	apiKey := args[1]

	if r.Orch != nil && r.Orch.SetProviderKey != nil {
		return r.Orch.SetProviderKey(provider, apiKey), nil
	}
	return fmt.Sprintf("API key management not available. Set %s_API_KEY in .env instead.", strings.ToUpper(provider)), nil
}

// ── /self command ─────────────────────────────────────────────────────────────
//
// The /self command is the SENSE self-knowledge layer entry point.
// Subcommands:
//
//   /self schema              — Dump machine-readable JSON discovery document
//   /self check               — Run autonomous trace analyzer
//   /self evolve [--dry-run]  — Convert failure reports to SKILL.md files
//   /self fix    [--dry-run]  — Alias for evolve (targeted at top pattern)
//   /self sanitizer           — Show stabilization middleware statistics

func (r *Registry) handleSelf(args []string) (string, error) {
	if len(args) == 0 {
		return selfHelp(), nil
	}

	sub := strings.ToLower(args[0])
	rest := args[1:]

	switch sub {
	case "schema":
		return r.handleSelfSchema(rest)
	case "check":
		return r.handleSelfCheck(rest)
	case "evolve":
		return r.handleSelfEvolve(rest, false)
	case "fix":
		return r.handleSelfEvolve(rest, true) // fix = targeted evolve
	case "sanitizer":
		return r.handleSelfSanitizer(rest)
	default:
		return fmt.Sprintf("Unknown /self subcommand: %q\n\n%s", sub, selfHelp()), nil
	}
}

// handleSelfSchema builds and returns the machine-readable JSON Discovery Document.
func (r *Registry) handleSelfSchema(args []string) (string, error) {
	if r.toolRegistry == nil {
		return "Tool registry not available.", nil
	}

	// Convert tool definitions to sense.ToolDescriptor format.
	defs := r.toolRegistry.GetDefinitions()
	// Sort deterministically by name.
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })

	descs := make([]sense.ToolDescriptor, 0, len(defs))
	catCounts := make(map[string]int)
	for _, d := range defs {
		descs = append(descs, sense.ToolDescriptor{
			Name:               d.Name,
			Description:        d.Description,
			Category:           d.Category,
			Parameters:         d.Parameters,
			RequiresPermission: false, // GetDefinitions() doesn't carry this; use list for accuracy
			DefaultPermission:  "always",
			OutputFormat:       "text",
		})
		catCounts[d.Category]++
	}

	// Enrich RequiresPermission + DefaultPermission from live tool list.
	toolList := r.toolRegistry.List()
	permMap := make(map[string]struct {
		req  bool
		perm string
		fmt  string
	})
	for _, t := range toolList {
		permMap[t.Name()] = struct {
			req  bool
			perm string
			fmt  string
		}{
			req:  t.RequiresPermission(),
			perm: string(t.DefaultPermission()),
			fmt:  string(t.OutputFormat()),
		}
	}
	for i := range descs {
		if pm, ok := permMap[descs[i].Name]; ok {
			descs[i].RequiresPermission = pm.req
			descs[i].DefaultPermission = pm.perm
			descs[i].OutputFormat = pm.fmt
		}
	}

	doc := sense.DiscoveryDoc{
		SchemaVersion:  "1.0.0",
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		Application:    "Gorkbot",
		ToolCount:      len(descs),
		CategoryCounts: catCounts,
		Tools:          descs,
		Flags:          sense.KnownCLIFlags(),
		SENSEVersion:   "1.0.0",
	}

	// Check if caller wants compact or pretty output.
	pretty := true
	for _, a := range args {
		if a == "--compact" {
			pretty = false
		}
	}

	var b []byte
	var err error
	if pretty {
		b, err = json.MarshalIndent(doc, "", "  ")
	} else {
		b, err = json.Marshal(doc)
	}
	if err != nil {
		return "", fmt.Errorf("/self schema: marshal failed: %w", err)
	}
	return string(b), nil
}

// handleSelfCheck runs the autonomous trace analyzer on the SENSE trace directory.
func (r *Registry) handleSelfCheck(args []string) (string, error) {
	traceDir := r.senseTraceDir()
	if traceDir == "" {
		return "Config directory not set. Call SetConfigDir() before using /self check.", nil
	}

	// Check if trace dir exists — provide a useful message if it doesn't.
	if _, err := os.Stat(traceDir); os.IsNotExist(err) {
		return fmt.Sprintf(
			"No trace directory found at `%s`.\n\n"+
				"Traces are written automatically during tool execution.\n"+
				"Run some tool-using tasks first, then re-run `/self check`.",
			traceDir,
		), nil
	}

	analyzer := sense.NewTraceAnalyzer(traceDir)
	report, err := analyzer.Analyze()
	if err != nil {
		return fmt.Sprintf("Trace analysis failed: %v", err), nil
	}

	return report.Summary, nil
}

// handleSelfEvolve runs the SENSE evolutionary pipeline.
// targeted=true limits scope to the single highest-priority pattern.
func (r *Registry) handleSelfEvolve(args []string, targeted bool) (string, error) {
	traceDir := r.senseTraceDir()
	if traceDir == "" {
		return "Config directory not set. Call SetConfigDir() before using /self evolve.", nil
	}

	// Parse flags.
	dryRun := true // MANDATORY default
	minEvidence := 2
	for _, a := range args {
		switch {
		case a == "--dry-run=false" || a == "--no-dry-run" || a == "-x":
			dryRun = false
		case a == "--dry-run" || a == "--dry-run=true":
			dryRun = true
		case strings.HasPrefix(a, "--min-evidence="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--min-evidence=")); err == nil && n > 0 {
				minEvidence = n
			}
		}
	}

	// Run trace analysis first.
	analyzer := sense.NewTraceAnalyzer(traceDir)
	report, err := analyzer.Analyze()
	if err != nil {
		return fmt.Sprintf("Trace analysis failed: %v", err), nil
	}

	if len(report.FailureEvents) == 0 {
		return "No failure events found in traces — nothing to evolve.\n\n" + report.Summary, nil
	}

	// Optionally limit to top pattern (targeted mode / /self fix).
	if targeted && len(report.Patterns) > 1 {
		report.Patterns = report.Patterns[:1]
	}

	skillsDir := r.senseSkillsDir()
	cfg := sense.DefaultEvolverConfig(skillsDir)
	cfg.DryRun = dryRun
	cfg.MinEvidence = minEvidence

	evolver := sense.NewSkillEvolver(cfg)
	result, err := evolver.Evolve(report)
	if err != nil {
		return fmt.Sprintf("SENSE evolution failed: %v", err), nil
	}

	return result.Summary, nil
}

// handleSelfSanitizer displays SENSE stabilization middleware statistics.
func (r *Registry) handleSelfSanitizer(_ []string) (string, error) {
	var sb strings.Builder
	sb.WriteString("# SENSE Stabilization Middleware\n\n")
	sb.WriteString("The input sanitizer validates every tool call parameter against three invariants:\n\n")
	sb.WriteString("| Invariant | Description |\n")
	sb.WriteString("|-----------|-------------|\n")
	sb.WriteString("| Control-character rejection | Rejects any byte < 0x20 and Unicode control/format chars |\n")
	sb.WriteString("| Path sandboxing | File paths must resolve within the current working directory |\n")
	sb.WriteString("| Resource-name validation | Names must not contain `?`, `#`, or `%` |\n\n")
	sb.WriteString("The sanitizer is **always active** and cannot be disabled.\n\n")
	sb.WriteString(fmt.Sprintf("**Trace directory:** `%s`\n", r.senseTraceDir()))
	sb.WriteString(fmt.Sprintf("**Skills directory:** `%s`\n", r.senseSkillsDir()))
	return sb.String(), nil
}

// ── Path helpers ─────────────────────────────────────────────────────────────

// senseTraceDir returns the platform-agnostic path to the SENSE trace directory.
// Uses filepath.Join for cross-platform correctness (Windows + Termux + Linux).
func (r *Registry) senseTraceDir() string {
	if r.configDir == "" {
		return ""
	}
	return filepath.Join(r.configDir, "sense", "traces")
}

// senseSkillsDir returns the platform-agnostic path to the SENSE skills directory.
func (r *Registry) senseSkillsDir() string {
	if r.configDir == "" {
		return ""
	}
	return filepath.Join(r.configDir, "sense", "skills")
}

// selfHelp returns the /self command help text.
func selfHelp() string {
	return `## /self — SENSE Self-Knowledge Layer

**Subcommands:**

| Command | Description |
|---------|-------------|
| ` + "`/self schema`" + ` | Dump JSON discovery document of all tools + CLI flags |
| ` + "`/self check`" + ` | Analyse trace logs for Neural Hallucinations, Tool Failures, Context Overflows |
| ` + "`/self evolve [--dry-run]`" + ` | Generate SKILL.md invariant files from failure patterns |
| ` + "`/self fix [--dry-run]`" + ` | Apply top-priority fix (same as evolve, scoped to top pattern) |
| ` + "`/self sanitizer`" + ` | Show stabilization middleware policy summary |

**Safety:** ` + "`--dry-run`" + ` is the default for ` + "`evolve`" + ` and ` + "`fix`" + `.
Pass ` + "`--dry-run=false`" + ` to write SKILL.md files to disk.

**Options:**
- ` + "`--min-evidence=N`" + ` — minimum failure count to generate a SKILL file (default: 2)
- ` + "`--compact`" + ` — compact JSON output for ` + "`/self schema`" + `
`
}

func (r *Registry) handleCascade(args []string) (string, error) {
	if r.Orch == nil {
		return "Orchestrator not available.", nil
	}

	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	switch sub {
	case "reset":
		if r.Orch.ResetCascadeOrder != nil {
			return r.Orch.ResetCascadeOrder(), nil
		}
		return "ResetCascadeOrder not wired.", nil

	case "set":
		if len(args) < 2 {
			return "Usage: /cascade set <provider1,provider2,...>\nExample: /cascade set xai,anthropic,google,openai", nil
		}
		rawList := strings.Join(args[1:], " ")
		rawList = strings.ReplaceAll(rawList, " ", ",")
		parts := strings.Split(rawList, ",")
		order := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				order = append(order, p)
			}
		}
		if len(order) == 0 {
			return "No providers specified. Usage: /cascade set xai,anthropic,google", nil
		}
		if r.Orch.SetCascadeOrder != nil {
			return r.Orch.SetCascadeOrder(order), nil
		}
		return "SetCascadeOrder not wired.", nil

	default:
		// Show current cascade order.
		if r.Orch.GetCascadeOrder != nil {
			order := r.Orch.GetCascadeOrder()
			var sb strings.Builder
			sb.WriteString("## Provider Failover Cascade\n\n")
			if len(order) == 0 {
				sb.WriteString("(empty — default order will be used)\n")
			} else {
				for i, p := range order {
					sb.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, p))
				}
			}
			sb.WriteString("\n**Usage:**\n")
			sb.WriteString("- `/cascade set xai,anthropic,google` — set custom order\n")
			sb.WriteString("- `/cascade reset` — restore default order\n")
			return sb.String(), nil
		}
		return "GetCascadeOrder not wired.", nil
	}
}

func (r *Registry) handleSandbox(args []string) (string, error) {
	if r.Orch == nil || r.Orch.ToggleSandbox == nil || r.Orch.SandboxEnabled == nil {
		return "Sandbox toggle not available (orchestrator not wired).", nil
	}

	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	switch sub {
	case "on":
		if r.Orch.SandboxEnabled() {
			return "Sandbox is already **ON** — SENSE input sanitizer active.", nil
		}
		r.Orch.ToggleSandbox()
		return "Sandbox **enabled** — SENSE path sandboxing and injection checks active.", nil

	case "off":
		if !r.Orch.SandboxEnabled() {
			return "Sandbox is already **OFF** — SENSE input sanitizer bypassed.", nil
		}
		r.Orch.ToggleSandbox()
		return "⚠ Sandbox **disabled** — SENSE path sandboxing bypassed. Use `/sandbox on` to re-enable.", nil

	default:
		// Show status when no args.
		enabled := r.Orch.SandboxEnabled()
		var sb strings.Builder
		sb.WriteString("## SENSE Sandbox Status\n\n")
		if enabled {
			sb.WriteString("**Status: ON** ✅ — SENSE input sanitizer active\n\n")
			sb.WriteString("Controls:\n")
			sb.WriteString("- Control-char rejection (ASCII < 0x20, Unicode Cc/Cf)\n")
			sb.WriteString("- Path sandboxing (file paths must resolve within CWD)\n")
			sb.WriteString("- Resource-name validation (?, #, %)\n")
		} else {
			sb.WriteString("**Status: OFF** ⚠ — SENSE input sanitizer bypassed\n\n")
			sb.WriteString("Path sandboxing and injection checks are disabled.\n")
		}
		sb.WriteString("\n**Usage:** `/sandbox on` | `/sandbox off`\n")
		return sb.String(), nil
	}
}

func (r *Registry) handleEnv(args []string) (string, error) {
	// /env refresh — trigger a background re-probe and report
	if len(args) > 0 && strings.EqualFold(args[0], "refresh") {
		if r.EnvProbeRefresh != nil {
			r.EnvProbeRefresh()
			return "♻ Environment re-probe triggered in the background. Run `/env` again in a moment to see updated results.", nil
		}
		return "Environment probe not available.", nil
	}

	if r.EnvProbeSnapshot != nil {
		snap := r.EnvProbeSnapshot()
		if snap == "" {
			return "Environment probe has not run yet. Retrying at startup.", nil
		}
		return "```\n" + strings.TrimSpace(snap) + "\n```", nil
	}
	return "Environment probe not wired. Restart Gorkbot to enable.", nil
}
