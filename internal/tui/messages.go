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

// ThinkingTokenMsg carries a chunk of extended thinking text routed
// separately from the main response stream.
type ThinkingTokenMsg struct {
	Content string
}

// ThinkingDoneMsg signals that the thinking block has completed and the
// main response is now beginning.
type ThinkingDoneMsg struct{}

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
	Result    string
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

// ToolCallMsg carries the outgoing tool call request (name + params) for
// display as a cyan-bordered box immediately before the tool result box.
type ToolCallMsg struct {
	ToolName string
	Params   map[string]interface{}
}

// ToolElapsedMsg carries per-tool elapsed time updates from the engine
// heartbeat ticker. Called every ~250ms per live tool and once on completion.
type ToolElapsedMsg struct {
	ToolName string
	Elapsed  time.Duration
	Done     bool // true when tool completes
	Success  bool // only meaningful when Done=true
}

// LivePanelClearMsg signals that the live tools panel should be cleared
type LivePanelClearMsg struct{}

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
// ResponseChan is created by the requesting goroutine; the TUI sends the
// decision back through it so the goroutine can unblock without a data race.
type HITLRequestMsg struct {
	Request      engine.HITLRequest
	ResponseChan chan engine.HITLDecision
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

// ModeChangeMsg signals a change in the active execution mode.
type ModeChangeMsg struct {
	ModeName string // "NORMAL", "PLAN", "AUTO"
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

// PlanningTokenMsg carries a streaming token destined for the planning box,
// not the live chat stream. All AI tokens during generation go through this
// path so that internal reasoning is hidden behind the planning box UI.
type PlanningTokenMsg struct{ Content string }

// PlanningBoxClearMsg resets the planning buffer when a tool call fires.
// The planning reasoning for that segment is discarded; the box is cleared.
type PlanningBoxClearMsg struct{}

// PlanningCommitMsg finalises generation: the last planning segment (the
// actual answer) is committed to the chat as an assistant message.
type PlanningCommitMsg struct{ Content string }

// PlanningTickMsg fires every 2 s to cycle the planning box label between
// "Planning..." and the latest extracted intent sentence.
type PlanningTickMsg time.Time

func planningTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return PlanningTickMsg(t)
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

// ── Toast notification system ─────────────────────────────────────────────

// ToastPriority controls display order, TTL, and default colour.
// Higher values are more urgent and float to the top of the visible stack.
type ToastPriority int

const (
	PrioritySubtle   ToastPriority = 1 // background status, 2 s TTL, gray
	PrioritySuccess  ToastPriority = 2 // confirmations, 3 s TTL, green
	PriorityInfo     ToastPriority = 3 // general info, 4 s TTL, blue
	PriorityWarning  ToastPriority = 4 // warnings, 6 s TTL, amber
	PriorityCritical ToastPriority = 5 // errors / limits, 10 s TTL, red
)

// defaultTTL returns the canonical TTL for a priority level.
func (p ToastPriority) defaultTTL() time.Duration {
	switch p {
	case PriorityCritical:
		return 10 * time.Second
	case PriorityWarning:
		return 6 * time.Second
	case PriorityInfo:
		return 4 * time.Second
	case PrioritySuccess:
		return 3 * time.Second
	default: // PrioritySubtle
		return 2 * time.Second
	}
}

// defaultColor returns a foreground hex color for a priority level.
func (p ToastPriority) defaultColor() string {
	switch p {
	case PriorityCritical:
		return "#ff6b6b"
	case PriorityWarning:
		return "#fbbf24"
	case PriorityInfo:
		return "#60a5fa"
	case PrioritySuccess:
		return "#34d399"
	default: // PrioritySubtle
		return "#9ca3af"
	}
}

// ToastKind determines lifetime and rendering behaviour.
type ToastKind int

const (
	KindEphemeral  ToastKind = iota // auto-dismiss after TTL (default)
	KindPersistent                  // stays until replaced or cleared
	KindProgress                    // shows live progress %; updated in-place via ID
)

// ToastMsg triggers a notification in the TUI toast stack.
// Backward-compatible: existing code setting only Icon/Text/Color still works —
// unset fields default to PriorityInfo / KindEphemeral / priority-default TTL.
type ToastMsg struct {
	// ID, if non-empty, updates an existing toast with the same ID in-place
	// (supports progress toasts and replaceable persistent alerts).
	ID string

	Icon  string
	Text  string
	Color string // hex foreground; "" = Priority.defaultColor()

	Priority ToastPriority // 0 → PriorityInfo
	Kind     ToastKind     // 0 → KindEphemeral
	TTL      time.Duration // 0 → Priority.defaultTTL()
	Progress float64       // 0.0–1.0; only rendered for KindProgress
}

// ── Builder functions ──────────────────────────────────────────────────────

// SubtleToast creates a low-priority background status notification (2 s, gray).
func SubtleToast(icon, text string) ToastMsg {
	return ToastMsg{Icon: icon, Text: text, Priority: PrioritySubtle}
}

// SuccessToast creates a success confirmation toast (3 s, green).
func SuccessToast(icon, text string) ToastMsg {
	return ToastMsg{Icon: icon, Text: text, Priority: PrioritySuccess}
}

// InfoToast creates a general informational toast (4 s, blue).
func InfoToast(icon, text string) ToastMsg {
	return ToastMsg{Icon: icon, Text: text, Priority: PriorityInfo}
}

// WarnToast creates an amber warning toast (6 s) with the ⚠ icon.
func WarnToast(text string) ToastMsg {
	return ToastMsg{Icon: "⚠", Text: text, Priority: PriorityWarning}
}

// ErrorToast creates a red error toast (8 s) with the ✗ icon.
func ErrorToast(text string) ToastMsg {
	return ToastMsg{Icon: "✗", Text: text, Priority: PriorityCritical, TTL: 8 * time.Second}
}

// CriticalToast creates a 10 s critical alert (red, 🚨 icon).
func CriticalToast(text string) ToastMsg {
	return ToastMsg{Icon: "🚨", Text: text, Priority: PriorityCritical}
}

// ProgressToast creates or updates a progress toast identified by id.
// pct is 0.0–1.0.  When pct reaches 1.0 the toast auto-dismisses in 2 s.
func ProgressToast(id, icon, text string, pct float64) ToastMsg {
	return ToastMsg{
		ID:       id,
		Icon:     icon,
		Text:     text,
		Priority: PriorityInfo,
		Kind:     KindProgress,
		Progress: pct,
	}
}

// ── Timer infrastructure ───────────────────────────────────────────────────

// toastDismissTickMsg fires to expire old toast notifications.
type toastDismissTickMsg time.Time

// toastDismissTick schedules a single toast expiry check 500 ms from now.
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

// thinkingResetMsg clears the thinking panel at the start of a new generation round-trip.
type thinkingResetMsg struct{}

// ── Consultation pipeline messages ────────────────────────────────────────
// These mirror the types defined in internal/engine/consultation so the TUI
// can handle consultation progress without importing that package directly.
// If you add a stage, add a corresponding label to consultationStageLabel().

// ConsultationStage identifies which step of the five-stage consultation
// pipeline is currently active.
type ConsultationStage int

const (
	ConsultStageValidating         ConsultationStage = iota // Stage 1: EntropyVoid schema check
	ConsultStageCacheCheck                                  // Stage 3: engram cosine-similarity lookup
	ConsultStageContextBuilding                             // Stage 2: hybrid semantic + lexical search
	ConsultStageConsulting                                  // Stage 4: Secondary API call (temp=0)
	ConsultStageValidatingResponse                          // Stage 5: type sanitisation + semantic check
)

// consultationStageLabel returns the UI-facing label for a ConsultationStage.
func consultationStageLabel(s ConsultationStage) string {
	switch s {
	case ConsultStageValidating:
		return "Validating request"
	case ConsultStageCacheCheck:
		return "Checking engram cache"
	case ConsultStageContextBuilding:
		return "Hybrid context search"
	case ConsultStageConsulting:
		return "Consulting Secondary (temp=0)"
	case ConsultStageValidatingResponse:
		return "Airlock validation"
	default:
		return "Processing"
	}
}

// ConsultationStageMsg signals a stage transition in the consultation pipeline.
// The TUI renders this as a dynamic label beneath the generation spinner.
type ConsultationStageMsg struct {
	Stage  ConsultationStage
	Detail string // optional per-stage annotation
}

// ConsultationDoneMsg carries the validated Secondary response.
// The orchestrator injects Content into the Primary's context as a Universal
// Truth system observation.
type ConsultationDoneMsg struct {
	Content   string // sanitised, type-validated answer
	FromCache bool   // true = Stage 3 hit; no Secondary API call was made
	Retries   int    // validation retries consumed (0–2)
}

// ConsultationErrorMsg signals that the consultation pipeline failed.
// Err is intentionally verbose and can be displayed directly in the TUI
// or forwarded to the Primary model as a tool error for self-correction.
type ConsultationErrorMsg struct {
	Err     error
	Stage   ConsultationStage
	Payload string // raw payload for debugging (first 200 chars)
}

// ── Hook output system (Claude Code-style) ────────────────────────────────
// Hooks provide live tree-structured output indicators for tool calls,
// consultation stages, DAG tasks, and any other discrete AI actions.

// HookStartMsg fires when a hook-tracked action begins.
// ParentID "" = top-level hook; non-empty = nested child (max depth 5).
type HookStartMsg struct {
	ID       string // unique hook ID (use tool name + timestamp suffix)
	ParentID string // "" for top-level
	Icon     string // glyph shown while active, e.g. "⚙", "⚡", "◐"
	Label    string // short description, e.g. "bash: echo hello"
}

// HookUpdateMsg updates metadata/output on an existing hook.
// Appends to existing output if AppendOutput is true.
type HookUpdateMsg struct {
	ID           string
	Metadata     string // e.g. progress %, file size, step N/M
	Output       string // preview output text
	AppendOutput bool   // true = append to existing output; false = replace
}

// HookDoneMsg marks a hook as completed (success or failure).
type HookDoneMsg struct {
	ID      string
	Success bool
	Elapsed time.Duration
	Summary string // final one-line summary shown as metadata
}

// HookTickMsg fires periodically to advance the embedded hook spinner.
// Only emitted while m.generating is true.
type HookTickMsg time.Time

// hookTick schedules the next hook animation frame (150 ms).
// Called only while generation is active to avoid idle CPU use.
func hookTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return HookTickMsg(t)
	})
}

// HookCollapseMsg toggles the collapsed state of a hook entry.
type HookCollapseMsg struct {
	ID string
}

// AtCompleteResultMsg carries file path completion results for @ autocomplete.
type AtCompleteResultMsg struct {
	Items []string
	Query string
}

// AuthRequestMsg signals that a tool requires authentication
type AuthRequestMsg struct {
	ToolName     string
	AuthType     string
	Description  string
	ResponseChan chan string // Sends the credential back
}

// ── SRE — Step-wise Reasoning Engine messages ───────────────────────────

// SREPhaseMsg signals an SRE phase transition
type SREPhaseMsg struct {
	Phase string // "HYPOTHESIS" / "PRUNE" / "CONVERGE"
	Turn  int
}

// SREGroundingMsg signals SRE grounding extraction results
type SREGroundingMsg struct {
	EntityCount     int
	ConstraintCount int
	FactCount       int
	Confidence      float64
}

// SREEnsembleMsg signals SRE ensemble execution
type SREEnsembleMsg struct {
	ConflictCount int
	Confidence    float64
}

// SRECorrectionMsg signals SRE deviation detection and backtrack
type SRECorrectionMsg struct {
	Reason      string
	RevertPhase string
}

// SREAnchorMsg signals SRE anchor storage
type SREAnchorMsg struct {
	Key   string
	Phase string
}

// ── Status line system ────────────────────────────────────────────────────────

// StatusUpdateMsg updates the single authoritative status line.
// Used to show pipeline progress, SRE phases, and token counts.
// Updates display in-place using \r\x1b[K to avoid creating new lines.
type StatusUpdateMsg struct {
	Phase       string // "pipeline" / "grounding" / "hypothesis" / "prune" / "converge" / "thinking"
	Description string // full descriptive text: "Analyzing input..." / "Grounding task..." / etc.
	Tokens      int    // only set when > 0; for "Thinking... (1,247 tokens)"
	Model       string // model ID suffix; only set during actual generation
}
