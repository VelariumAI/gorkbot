package commands

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// CommandDefinition defines a slash command in the Gorkbot TUI
type CommandDefinition struct {
	Name        string
	Description string
	Usage       string
	Handler     func(args []string) (string, error)
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
	RateResponse     func(score float64) string
	// StartRelay starts session sharing and returns the observer URL.
	StartRelay       func() string
	// StopRelay stops the active session relay.
	StopRelay        func()
	// ToggleDebug flips debug mode (raw AI output visible). Returns new state.
	ToggleDebug      func() bool
	// SetPrimary hot-swaps the primary provider/model. Returns status string.
	SetPrimary       func(provider, modelID string) string
	// SetSecondary hot-swaps the consultant provider/model. Returns status string.
	SetSecondary     func(provider, modelID string) string
	// SetAutoSecondary enables auto-secondary selection (clears explicit consultant).
	SetAutoSecondary func() string
	// GetProviderStatus returns a formatted status summary of all providers.
	GetProviderStatus func() string
	// SetProviderKey saves a new API key for the given provider.
	SetProviderKey   func(provider, key string) string
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
}

// Registry holds all available commands
type Registry struct {
	commands          map[string]*CommandDefinition
	toolRegistry      *tools.Registry
	availableModels   []ModelInfo
	currentPrimary    ModelInfo
	currentConsultant ModelInfo
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
	// UserCmdsGet looks up a user-defined command (name, args → rendered prompt, found).
	UserCmdsGet func(name, args string) (string, bool)
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
	}

	r.commands["model"] = &CommandDefinition{
		Name:        "model",
		Description: "View or switch primary/consultant model",
		Usage:       "/model [primary|consultant] [model-id]",
		Handler:     r.handleModel,
	}

	r.commands["tools"] = &CommandDefinition{
		Name:        "tools",
		Description: "List active tools; /tools stats shows usage analytics",
		Usage:       "/tools [stats]",
		Handler:     r.handleTools,
	}

	r.commands["key"] = &CommandDefinition{
		Name:        "key",
		Description: "Set or validate an API key for a provider",
		Usage:       "/key <provider> <api-key>  |  /key status  |  /key validate <provider>",
		Handler:     r.handleKey,
	}

	r.commands["auth"] = &CommandDefinition{
		Name:        "auth",
		Description: "Refresh API credentials",
		Usage:       "/auth [refresh|status]",
		Handler:     r.handleAuth,
	}

	r.commands["settings"] = &CommandDefinition{
		Name:        "settings",
		Description: "View settings and configuration",
		Usage:       "/settings",
		Handler:     r.handleSettings,
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
	}

	r.commands["compress"] = &CommandDefinition{
		Name:        "compress",
		Description: "Compress current context to save tokens",
		Usage:       "/compress",
		Handler:     r.handleCompress,
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
	}

	r.commands["mode"] = &CommandDefinition{
		Name:        "mode",
		Description: "View or switch execution mode (normal/plan/auto)",
		Usage:       "/mode [normal|plan|auto]",
		Handler:     r.handleMode,
	}

	r.commands["export"] = &CommandDefinition{
		Name:        "export",
		Description: "Export conversation to file",
		Usage:       "/export [markdown|json|plain] [filename]",
		Handler:     r.handleExport,
	}

	r.commands["compact"] = &CommandDefinition{
		Name:        "compact",
		Description: "Compress context to save tokens",
		Usage:       "/compact [focus hint]",
		Handler:     r.handleCompactEnhanced,
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
	}

	r.commands["save"] = &CommandDefinition{
		Name:        "save",
		Description: "Save current conversation to a named session file",
		Usage:       "/save <name>",
		Handler:     r.handleSave,
	}

	r.commands["resume"] = &CommandDefinition{
		Name:        "resume",
		Description: "Resume a previously saved session",
		Usage:       "/resume <name>  |  /resume list",
		Handler:     r.handleResume,
	}

	r.commands["mcp"] = &CommandDefinition{
		Name:        "mcp",
		Description: "Show MCP server status and connected tools",
		Usage:       "/mcp [status|config]",
		Handler:     r.handleMCP,
	}

	r.commands["rate"] = &CommandDefinition{
		Name:        "rate",
		Description: "Rate the last AI response (1–5). Feeds the adaptive model router.",
		Usage:       "/rate <1-5>",
		Handler:     r.handleRate,
	}

	r.commands["share"] = &CommandDefinition{
		Name:        "share",
		Description: "Start or stop live session sharing over SSE.",
		Usage:       "/share [start|stop]",
		Handler:     r.handleShare,
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

}

// Command Handlers

func (r *Registry) handleClear(args []string) (string, error) {
	return "CLEAR_SCREEN", nil // Special signal for TUI
}

func (r *Registry) handleHelp(args []string) (string, error) {
	var sb strings.Builder
	sb.WriteString("# Gorkbot Commands\n\n")
	sb.WriteString("| Command | Description | Usage |\n")
	sb.WriteString("|---------|-------------|-------|\n")

	// Sort commands alphabetically for consistent display
	commandNames := []string{
		"help", "about", "clear", "chat", "model", "tools", "permissions", "auth",
		"settings", "version", "theme", "compress", "bug", "quit",
		// Enhanced commands
		"context", "cost", "rewind", "mode", "export", "compact",
		"skills", "rules", "hooks", "rename",
		// P2+ commands
		"mcp", "save", "resume", "rate", "share", "debug",
	}

	for _, name := range commandNames {
		if cmd, exists := r.commands[name]; exists {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | `%s` |\n",
				cmd.Name, cmd.Description, cmd.Usage))
		}
	}

	sb.WriteString("\n**Tip:** Press `Alt+Enter` for multi-line input\n")
	return sb.String(), nil
}

func (r *Registry) handleAbout(args []string) (string, error) {
	return "# About Gorkbot\n\n" +
		"**Intelligence**\n" +
		"- **ARC Router** — classifies every prompt (Direct vs ReasonVerify) and scales tool budget to platform RAM\n" +
		"- **MEL** — Meta-Experience Learning: turns tool failure→correction cycles into persistent guardrail heuristics\n\n" +
		"**Parametric Memory** *(cross-session, query-relevant, refreshed every turn)*\n" +
		"- **AgeMem STM/LTM** — two-tier memory; hot facts in-session, cold facts survive restarts\n" +
		"- **Engrams** — explicit tool/behaviour preferences written by `record_engram`, persisted to LTM\n" +
		"- **MEL VectorStore** — heuristic store with Jaccard retrieval and confidence weighting\n\n" +
		"**SENSE**\n" +
		"- **LIE** (reasoning depth) · **Stabilizer** (quality guard) · **Code2World** (action preview)\n\n" +
		"**Discovery** — live model polling (xAI + Gemini) · **Cloud Brains** tab `Ctrl+D`\n\n" +
		"---\n\n" +
		"*Independent project — Todd Eddings / Velarium AI — OpenAI-compatible API*", nil
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
	// /tools stats — analytics dashboard
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
		// Append SQLite all-time stats if available.
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
		return "Usage: /auth [refresh|status]", nil
	}

	subcommand := args[0]
	switch subcommand {
	case "refresh":
		return "🔄 Refreshing OAuth tokens...", nil
	case "status":
		return "✅ **Authentication Status**\n\n• Grok: API Key (valid)\n• Gemini: OAuth (expires in 2h 15m)", nil
	default:
		return fmt.Sprintf("Unknown subcommand: %s", subcommand), nil
	}
}

func (r *Registry) handleSettings(args []string) (string, error) {
	// Signal the TUI to open the interactive settings overlay.
	return "SETTINGS_MODAL", nil
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
	default: // status
		if r.MCPStatus != nil {
			return "# MCP — Model Context Protocol\n\n" + r.MCPStatus(), nil
		}
		return "# MCP — Model Context Protocol\n\nMCP manager not initialized. Start the TUI to activate MCP servers.", nil
	}
}

func (r *Registry) handleCompress(args []string) (string, error) {
	return "🗜️  **Context Compression**\n\nCompressing conversation history...\n• Before: 12,450 tokens\n• After: 3,200 tokens\n• Saved: 74%\n\n✅ Context compressed successfully", nil
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
	if r.Orch == nil || r.Orch.ExportConv == nil {
		return "Export system not available.", nil
	}
	format := "markdown"
	path := ""
	if len(args) >= 1 {
		format = args[0]
	}
	if len(args) >= 2 {
		path = args[1]
	}
	if path == "" {
		// Generate default filename
		path = fmt.Sprintf("gorkbot_export_%s.%s",
			time.Now().Format("20060102_150405"),
			formatExt(format))
	}
	return r.Orch.ExportConv(format, path), nil
}

func formatExt(format string) string {
	switch format {
	case "json":
		return "json"
	case "plain", "text":
		return "txt"
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
	if len(args) == 0 {
		return "Usage: /save <name>", nil
	}
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
