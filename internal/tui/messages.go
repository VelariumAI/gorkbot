package tui

import (
        "math/rand"
        "time"

        tea "github.com/charmbracelet/bubbletea"
        "github.com/velariumai/gorkbot/internal/engine"
        "github.com/velariumai/gorkbot/pkg/commands"
        "github.com/velariumai/gorkbot/pkg/tools"
)

// TokenMsg represents a streaming token from the AI response
type TokenMsg struct {
	Content      string
	IsConsultant bool
	IsFinal      bool
}

// ErrorMsg represents an error message
type ErrorMsg struct {
	Err error
}

// SpinnerTickMsg is sent by the spinner ticker
type SpinnerTickMsg time.Time

// PhraseTickMsg is sent to rotate the loading phrase
type PhraseTickMsg time.Time

// CommandResultMsg represents the result of a command execution
type CommandResultMsg struct {
	Result string
	IsSpecial bool // For special signals like CLEAR_SCREEN, QUIT, etc.
}

// ModelSwitchMsg indicates the model has been switched
type ModelSwitchMsg struct {
	Model string
}

// ThemeSwitchMsg indicates the theme has been switched
type ThemeSwitchMsg struct {
	Theme string
}

// StartGenerationMsg signals the start of AI generation
type StartGenerationMsg struct {
	IsConsultant bool
	Prompt       string
}

// EndGenerationMsg signals the end of AI generation
type EndGenerationMsg struct{}

// ClearScreenMsg signals to clear the screen
type ClearScreenMsg struct{}

// QuitMsg signals to quit the application
type QuitMsg struct{}

// StreamCompleteMsg signals that streaming has completed
type StreamCompleteMsg struct{}

// ToolExecutionMsg represents a tool being executed
type ToolExecutionMsg struct {
	ToolName string
	Result   *tools.ToolResult
}

// ColorTickMsg is sent to change spinner colors
type ColorTickMsg time.Time

// LightGlistenTickMsg is sent to update the banner light glisten effect
type LightGlistenTickMsg time.Time

// InterventionRequestMsg signals a request for user intervention
type InterventionRequestMsg struct {
	Severity     engine.WatchdogSeverity
	Context      string
	ResponseChan chan engine.InterventionResponse
}

// HITLRequestMsg signals a SENSE HITL plan-and-execute approval request.
type HITLRequestMsg struct {
	Request engine.HITLRequest
}

// CompletionMsg represents a text completion suggestion
type CompletionMsg struct {
	Content string
}

// PermissionRequestMsg signals a request for tool execution permission
type PermissionRequestMsg struct {
	ToolName     string
	Description  string
	Params       map[string]interface{}
	ResponseChan chan tools.PermissionLevel
}

// ModeChangeMsg signals an execution mode change.
type ModeChangeMsg struct {
	ModeName string // "NORMAL", "PLAN", "AUTO"
}

// ContextUpdateMsg carries context window stats for the status bar.
type ContextUpdateMsg struct {
	UsedPct  float64
	UsedToks int
	MaxToks  int
	CostUSD  float64
}

// InterruptMsg signals the user wants to cancel current generation.
type InterruptMsg struct{}

// ToolProgressMsg signals a tool starting or completing (for status bar).
type ToolProgressMsg struct {
	ToolName string
	Done     bool
	Success  bool
	Elapsed  float64 // seconds
}

// ProcessOutputMsg carries streaming output from a background process to the chat.
type ProcessOutputMsg struct {
	ProcessID string
	Output    string
	IsStderr  bool
	Done      bool // true when process has completed
	ExitCode  int
}

// RewindCompleteMsg signals a successful session rewind.
type RewindCompleteMsg struct {
	Description string
	MsgCount    int
}

// DiscoveryUpdateMsg carries a fresh model list from the discovery manager.
type DiscoveryUpdateMsg struct {
	Models []discoveryModel // lightweight copy to avoid importing discovery in messages
}

// discoveryModel is a minimal copy of discovery.DiscoveredModel for TUI use.
type discoveryModel struct {
	ID       string
	Name     string
	Provider string
	BestCap  string // "reasoning", "speed", "coding", "general"
}

// DiscoveryPollTickMsg triggers a poll of the discovery subscription channel.
type DiscoveryPollTickMsg struct{}

// Helper functions to create tickers

func spinnerTick() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return SpinnerTickMsg(t)
	})
}

func phraseTick() tea.Cmd {
	return tea.Tick(time.Second*3, func(t time.Time) tea.Msg {
		return PhraseTickMsg(t)
	})
}

func colorTick() tea.Cmd {
	// Change color every 2-4 seconds (random interval for organic feel)
	interval := time.Duration(2000+rand.Intn(2000)) * time.Millisecond
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return ColorTickMsg(t)
	})
}

func glistenTick() tea.Cmd {
	// Smooth animation: update every 50ms for fluid motion
	return tea.Tick(time.Millisecond*50, func(t time.Time) tea.Msg {
		return LightGlistenTickMsg(t)
	})
}

func discoveryPollTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return DiscoveryPollTickMsg{}
	})
}

// ProviderPollTickMsg triggers a full re-poll of all configured provider model lists.
type ProviderPollTickMsg struct{}

// providerPollTick schedules the next provider model list refresh (every 5 minutes).
func providerPollTick() tea.Cmd {
	return tea.Tick(5*time.Minute, func(t time.Time) tea.Msg {
		return ProviderPollTickMsg{}
	})
}

// ModelRefreshMsg carries a fresh model list for one provider.
type ModelRefreshMsg struct {
	Provider string
	Models   []commands.ModelInfo
	Err      error
}

// ToastMsg triggers an ephemeral inline notification in the TUI.
type ToastMsg struct {
	Icon  string // "✓", "⚠", "↩", "🔵"
	Text  string
	Color string // hex or ANSI color name
}

// toastDismissTickMsg fires to expire old toast notifications.
type toastDismissTickMsg time.Time

// toastDismissTick schedules a single toast expiry check 500ms from now.
func toastDismissTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return toastDismissTickMsg(t)
	})
}

// sidePanelTickMsg fires every 500ms while the side panel is open to refresh
// its live agent/tool data.
type sidePanelTickMsg time.Time

// sidePanelTick schedules the next side-panel refresh.
func sidePanelTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return sidePanelTickMsg(t)
	})
}

// APIKeySavedMsg signals the result of an API key save+validate attempt.
type APIKeySavedMsg struct {
	Provider string
	Valid    bool
	ErrMsg   string
}

// ModelSwitchedMsg signals that primary or secondary was hot-swapped.
type ModelSwitchedMsg struct {
	Role     string // "primary" or "secondary"
	ModelID  string
	Provider string
}

// ProviderStatusMsg carries a fresh provider status list for the UI.
type ProviderStatusMsg struct {
	Statuses []providerStatus
}

// providerStatus is a TUI-local snapshot of a provider's key state.
type providerStatus struct {
	Provider string
	Status   int // 0=missing, 1=unverified, 2=valid, 3=invalid
}
