package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/lipgloss"

	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/process"
	"github.com/velariumai/gorkbot/pkg/registry"
	"github.com/velariumai/gorkbot/pkg/theme"
	"github.com/velariumai/gorkbot/pkg/tools"
	tui_style "github.com/velariumai/gorkbot/pkg/tui"
)

const (
	// UI dimensions
	minWidth  = 60
	minHeight = 20
)

type sessionState int

const (
	chatView      sessionState = iota
	modelListView              // kept for backward compat
	toolsTableView
	discoveryView
	analyticsView     // session analytics dashboard (Ctrl+A)
	diagnosticsView   // system diagnostics (Ctrl+\)
	stateHITLApproval // SENSE HITL plan-and-execute approval overlay
	dagView           // DAG task-graph executor view
)

// hitlPendingItem holds a queued HITL request alongside the response channel
// that the requesting goroutine is blocking on.
type hitlPendingItem struct {
	req engine.HITLRequest
	ch  chan engine.HITLDecision
}

// modelSelectView is the dual-pane model selection view (alias for modelListView).
const modelSelectView = modelListView

// Model represents the TUI application state
type Model struct {
	// Bubble Tea components
	viewport          viewport.Model
	textarea          textarea.Model
	spinner           spinner.Model
	consultantSpinner spinner.Model
	help              help.Model
	keymap            KeyMap

	// Model Selection List
	modelList       list.Model
	availableModels []commands.ModelInfo
	state           sessionState

	// Tools Table
	toolsTable TableModel

	// Command registry
	commands *commands.Registry

	// Orchestrator
	orchestrator *engine.Orchestrator

	// Process Manager
	processManager *process.Manager

	// Active Overlay
	activeOverlay Overlay

	// Status Bar
	statusBar StatusBar

	// State
	ready         bool
	width         int
	height        int
	generating    bool
	isConsultant  bool
	mouseEnabled  bool
	currentPhrase string
	err           error

	// Permission prompts
	awaitingPermission bool
	permissionPrompt   *PermissionPrompt
	permissionChan     chan tools.PermissionLevel

	// Intervention prompts (Watchdog)
	interventionMode   bool
	interventionPrompt string
	interventionChan   chan engine.InterventionResponse

	// SENSE HITL approval state
	awaitingHITL  bool
	hitlRequest   *engine.HITLRequest
	hitlChan      chan engine.HITLDecision
	hitlQueue     []hitlPendingItem // FIFO queue of pending HITL requests
	hitlOverlay   *HITLOverlay      // Rich HITL approval dialog

	// Content
	messages        []Message
	currentResponse strings.Builder
	currentModel    string
	theme           string
	themeManager    *theme.Manager

	// Scroll state - track if user is viewing older messages
	userScrolledUp bool // true when user has scrolled up to read older content

	// Live tool execution panel — tracks running/completed tools with elapsed times
	livePanel *LiveToolsPanel

	// Discovery: live cloud model sidebar + agent tree.
	discoveredModels []discoveryModel      // latest snapshot from discovery manager
	discoverySub     chan []discoveryModel // receives updates from discovery bridge goroutine

	// Markdown renderer
	glamour *glamour.TermRenderer

	// Styles
	styles *Styles

	// Quit flag
	quitting bool

	// Program reference for sending messages
	program *tea.Program

	// Dynamic Auth Wizard state
	awaitingAuth bool
	authRequest  *AuthRequestMsg
	authInput    textinput.Model

	// Provider registry — used by /model to instantiate providers via WithModel.
	providerRegistry *registry.ModelRegistry

	currentColorIdx int

	// Performance: track renderer word-wrap width so we only recreate glamour
	// when the terminal width actually changes, not on every WindowSizeMsg.
	rendererWidth int

	// Performance: throttle live viewport re-renders during streaming.
	// updateViewportContent (full glamour re-render) runs every streamChunkInterval
	// tokens instead of every single token, eliminating O(messages×tokens) cost.
	streamChunkCount    int
	streamChunkInterval int // set once in NewModel

	// Tool-interleave tracking: after a tool result is inserted, post-tool tokens
	// must go into a NEW assistant message rather than updating the pre-tool one.
	// responseSegStart is the byte offset into currentResponse where the current
	// segment begins; streamAfterTool signals that the next token starts a new segment.
	streamAfterTool  bool
	responseSegStart int

	// Header logo — pre-rendered block-art lines from gorkbot.png.
	logoLines []string
	logoWidth int

	// Header animation — glisten wave + searchlight sweep positions (0.0–1.0).
	glistenPos   float64
	spotlightPos float64

	// debugMode shows raw AI output (incl. tool JSON blocks) when true.
	debugMode bool

	// ── Analytics Dashboard (analyticsView) ──────────────────────────────
	// analytics holds session metrics shown in the Analytics tab (Ctrl+A).
	analytics *AnalyticsData

	// ── Splash screen ─────────────────────────────────────────────────────
	// splashDone is false until the user presses Enter on the splash screen.
	splashDone bool

	// ── Toast notifications ───────────────────────────────────────────────
	// Queue is kept sorted: highest Priority first, then newest CreatedAt.
	// At most 5 items are stored; at most 3 are rendered at once.
	toasts []toastItem

	// ── Side panel ────────────────────────────────────────────────────────
	sidePanelOpen  bool
	sidePanelWidth int

	// ── ARC intent badge for latest user message ──────────────────────────
	lastIntentCategory string

	// ── Planning box: hides internal monologue during generation ──────────
	// planningActive is true while the AI is generating (hidden in box).
	// planningBuf is kept for the PlanningCommit path (content capture).
	// planningShowDots / planningIntent are no longer used for display
	// (superseded by the activity panel below).
	planningActive   bool
	planningBuf      strings.Builder
	planningIntent   string // unused; kept for zero-cost backward compat
	planningShowDots bool   // unused; kept for zero-cost backward compat

	// ── Extended thinking panel ────────────────────────────────────────────
	// thinkingBuf accumulates streamed thinking tokens from Anthropic's
	// extended thinking feature.  It is cleared at the start of each new
	// callOrchestrator round-trip and rendered as a collapsible box while
	// m.generating is true (and for a moment after, until StreamCompleteMsg).
	thinkingBuf    strings.Builder
	thinkingActive bool // true while the AI is mid-thinking block

	// ── Activity panel: live view of what the AI is actually doing ────────
	// genPhase tracks the current generation phase (thinking/tool/synthesizing).
	// thinkingTokens counts planning tokens received so far.
	// activityLog holds the last 4 completed tool actions with elapsed times.
	// currentActivity is the tool currently running (nil = none).
	genPhase        genPhase
	thinkingTokens  int
	activityLog     []activityEntry
	currentActivity *activityEntry

	liveTokenCount int // estimated live tokens in current response

	// ── Tool timing: ToolName → start time (for elapsed display) ──────────
	toolStartTimes map[string]time.Time

	// ── Single authoritative status line (G ▶ ...) ─────────────────────────
	// Updated via StatusUpdateMsg to show pipeline progress, SRE phases, and tokens.
	// statusPhase is "pipeline" / "grounding" / "hypothesis" / "prune" / "converge" / "thinking".
	// statusDescription is the full human-readable text (e.g. "Analyzing input...").
	// statusTokens is the token count (0 = not shown).
	// statusModel is the model ID suffix (empty = not shown).
	statusPhase       string
	statusDescription string
	statusTokens      int
	statusModel       string

	// ── Dual-pane model selection (modelSelectView) ───────────────────────
	modelSelect  modelSelectState
	apiKeyPrompt apiKeyPromptState

	// ── Cloud Brains interactive cursor (discoveryView) ───────────────────
	discCursor     int    // index into discoveredModels list
	discTestActive bool   // test-prompt input is open
	discTestInput  string // accumulated test prompt text
	discTestResult string // result of test prompt

	// ── Conversation bookmarks ────────────────────────────────────────────
	bookmarks           []Bookmark // in-memory bookmark list
	bookmarkOverlay     bool       // bookmark manager is open
	bookmarkInput       string     // new bookmark name input
	bookmarkInputActive bool       // creating new bookmark

	// ── Omni-search (Ctrl+K) ──────────────────────────────────────────────
	// searchMode is true while the search bar is active.
	// searchQuery is the current search string typed by the user.
	// searchMatches holds the message indices of conversations that contain the query.
	// searchMatchIdx is which match is currently highlighted / scrolled to.
	searchMode     bool
	searchQuery    string
	searchMatches  []int
	searchMatchIdx int

	// ── DAG Orchestrator (dagView) ────────────────────────────────────────
	// dagVM is the active DAG task-graph view model. Nil when no graph is running.
	dagVM *DAGViewModel

	// ── Integration settings callbacks (tabIntegrations in SettingsOverlay) ──
	// integrationGetter returns current effective values (env var or stored config).
	// integrationSetter persists a key/value pair and applies it live.
	integrationGetter func() map[string]string
	integrationSetter func(key, value string) error

	// ── Hook output system (Claude Code-style) ────────────────────────────
	// activeHooks holds top-level hook entries live during generation.
	// Hooks are appended/updated by HookStartMsg/HookUpdateMsg/HookDoneMsg.
	// hookSpinFrame advances on every HookTickMsg (150 ms).
	activeHooks    []HookEntry
	hookSpinFrame  int
	hookTickActive bool      // true while hookTick() loop is running
	lastTokenTime  time.Time // debouncer for token rendering

	// ── @ file path autocomplete ─────────────────────────────────────────
	atCompleteActive bool
	atCompleteQuery  string
	atCompleteItems  []string
	atCompleteIdx    int
	atCompleteAt     int    // textarea cursor offset of the triggering @
	atCompleteCWD    string // cached CWD; set once in NewModel, avoids os.Getwd() per keystroke

	// ── Input history search (Ctrl+R) ────────────────────────────────────
	inputHistory      []string
	inputHistoryLower []string // pre-lowercased copy for O(1) search per item
	histSearchMode    bool
	histSearchQuery   string
	histSearchMatches []int // indices into inputHistory
	histSearchIdx     int

	// ── Double-Esc rewind menu ────────────────────────────────────────────
	lastEscTime    time.Time
	rewindMenuOpen bool
	rewindItems    []rewindItem // []session.CheckpointSummary (alias in completions.go)
	rewindCursor   int

	// ── Compact tab bar ──────────────────────────────────────────────────
	compactTabs bool
}

// ── Activity panel types ───────────────────────────────────────────────────

// genPhase represents the current AI generation phase shown in the activity panel.
type genPhase int

const (
	phaseIdle         genPhase = iota // no generation in progress
	phaseThinking                     // AI is reasoning internally (planning tokens)
	phaseTool                         // a tool call is executing
	phaseSynthesizing                 // composing final response after tool(s)
)

// activityEntry records one discrete action taken during generation.
type activityEntry struct {
	Icon      string
	Label     string
	StartedAt time.Time
	Elapsed   time.Duration // zero while in progress
	Done      bool
	Success   bool
}

// ── Hook output entries (Claude Code-style) ───────────────────────────────
// HookEntry represents one discrete action in the live hook output tree.
// It may nest child actions up to depth maxHookDepth.
const maxHookDepth = 5

// HookEntry is a single node in the live action tree rendered below the
// loading indicator during generation and persisted in the viewport after.
type HookEntry struct {
	ID         string
	Icon       string // active glyph (e.g. "⚙"); replaced by ✓/✗ when done
	Label      string // short description
	Metadata   string // elapsed, progress, size, etc.
	Output     string // optional preview output (first 2 lines)
	Active     bool   // true while running
	IsFinal    bool   // true once completed
	IsError    bool   // true if failed
	Collapsed  bool   // fold children
	Depth      int    // nesting level (0 = top)
	CreatedAt  time.Time
	Elapsed    time.Duration // zero while active
	SubEntries []HookEntry   // child actions (max depth 5)
}

// findHook searches a flat or nested slice for the entry with the given ID.
// Returns a pointer into the slice for in-place mutation.
func findHook(hooks []HookEntry, id string) *HookEntry {
	for i := range hooks {
		if hooks[i].ID == id {
			return &hooks[i]
		}
		if ptr := findHook(hooks[i].SubEntries, id); ptr != nil {
			return ptr
		}
	}
	return nil
}

// hasActiveHooks recursively checks if any hook in the tree is still active.
func hasActiveHooks(hooks []HookEntry) bool {
	for i := range hooks {
		if hooks[i].Active {
			return true
		}
		if hasActiveHooks(hooks[i].SubEntries) {
			return true
		}
	}
	return false
}

// sealAllHooks forcibly marks every active hook as done (orphan protection).
func sealAllHooks(hooks []HookEntry) []HookEntry {
	for i := range hooks {
		if hooks[i].Active {
			hooks[i].Active = false
			hooks[i].IsFinal = true
			hooks[i].IsError = true
			if hooks[i].Metadata == "" {
				hooks[i].Metadata = "interrupted"
			}
		}
		hooks[i].SubEntries = sealAllHooks(hooks[i].SubEntries)
	}
	return hooks
}

// toastItem is the internal representation of a queued toast notification.
// Constructed only via pushToast() from a ToastMsg; never built directly outside model_extensions.go.
type toastItem struct {
	ID        string // empty for anonymous toasts
	Icon      string
	Text      string
	Color     string // resolved foreground hex (never empty after pushToast)
	Priority  ToastPriority
	Kind      ToastKind
	Progress  float64 // 0.0–1.0; only meaningful for KindProgress
	CreatedAt time.Time
	ExpiresAt time.Time // zero value = KindPersistent (never auto-dismissed)
}

// Bookmark marks a specific message index in the conversation.
type Bookmark struct {
	ID           string
	Name         string
	MessageIndex int
	CreatedAt    time.Time
}

// Message represents a single message in the conversation
type Message struct {
	Role           string // "user", "assistant", "consultant", "system", "tool", "tool_call", "internal", "a2a"
	Content        string
	IsConsultant   bool
	ToolName       string                 // For tool messages
	ToolResult     *tools.ToolResult      // For tool result messages
	ToolParams     map[string]interface{} // For tool_call (request) messages
	NestLevel      int                    // Nesting depth for tool calls (0 = top level)
	MessageType    string                 // "tool", "tool_call", "internal", "a2a", "normal"
	Collapsed      bool                   // true = show 1-line summary; toggled with Ctrl+R
	FullyExpanded  bool                   // true = show all lines without truncation
	Elapsed        time.Duration          // tool execution duration (for display)
	IntentCategory string                 // ARC category at time of user message
	HookEntries    []HookEntry            // Claude Code-style hooks for this message
	IsStructured   bool                   // true if the message is primarily structured/hook-driven
	IsEphemeral    bool                   // true if the message is a temporary UI banner/alert not part of core history
}

// modelSelectState holds the dual-pane model selection view state.
type modelSelectState struct {
	activePane     int // 0 = primary, 1 = secondary
	primaryList    list.Model
	secondaryList  list.Model
	providerFilter string           // "" = all; otherwise filter by provider ID
	providerKeys   []providerStatus // latest provider key statuses
	refreshing     map[string]bool
}

// apiKeyPromptState holds the API key entry modal state.
type apiKeyPromptState struct {
	active     bool
	provider   string
	inputVal   string
	validating bool // true while background Ping is in flight
	errMsg     string
	websiteURL string
}

// NewModel creates a new TUI model.
// modelName is the display name of the primary AI (e.g. "Grok-3").
// consultantName is the display name of the consultant AI (e.g. "Gemini 2.0 Flash"); pass "" if unavailable.
func NewModel(orch *engine.Orchestrator, pm *process.Manager, modelName, consultantName string, registry *commands.Registry) (*Model, error) {
	// Initialize components
	ta := textarea.New()
	ta.Placeholder = "Ask me anything... (Tap here to type, Enter to send)"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(true)         // Alt+Enter for newline
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // Keep cursor visible

	// Use existing registry if provided, otherwise create a new one
	if registry == nil {
		registry = commands.NewRegistry()
	}

	sp := spinner.New()
	sp.Spinner = BlockGSpinner()
	// Use GrokBlue for the spinner
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue))

	csp := spinner.New()
	csp.Spinner = ConsultantSpinner()
	csp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Blood Red

	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true // Enable mouse wheel scrolling
	vp.MouseWheelDelta = 3      // Scroll 3 lines per wheel tick

	// Initialize glamour for markdown rendering
	r, _ := glamour.NewTermRenderer(
		glamour.WithStyles(CustomGlamourStyle()),
		glamour.WithWordWrap(80),
	)

	// Initialize help bubble
	h := help.New()

	// Initialize model list
	l := initModelList(80, 20)

	// Initialize tools table
	toolColumns := []table.Column{
		{Title: "Name", Width: 20},
		{Title: "Description", Width: 50},
		{Title: "Category", Width: 15},
	}
	tt := NewTableModel(toolColumns, []table.Row{})

	// Create theme manager
	env, _ := platform.GetEnvConfig()
	configDir := ""
	if env != nil {
		configDir = env.ConfigDir
	}
	themeManager := theme.NewManager(configDir)
	activeTheme := themeManager.Active()

	// Create styles with the active theme
	styles := NewStyles(activeTheme)

	m := &Model{
		viewport:            vp,
		textarea:            ta,
		spinner:             sp,
		consultantSpinner:   csp,
		help:                h,
		keymap:              DefaultKeyMap(),
		modelList:           l,
		toolsTable:          tt,
		state:               chatView,
		commands:            registry,
		orchestrator:        orch,
		processManager:      pm,
		statusBar:           NewStatusBar(styles),
		ready:               false,
		generating:          false,
		isConsultant:        false,
		messages:            []Message{},
		currentModel:        modelName,
		theme:               activeTheme.Name,
		glamour:             r,
		styles:              styles,
		themeManager:        themeManager,
		currentColorIdx:     0,
		rendererWidth:       80,
		streamChunkInterval: 8,
		toolStartTimes:      make(map[string]time.Time),
		livePanel:           NewLiveToolsPanel(),
		authInput:           textinput.New(),
	}
	m.authInput.Placeholder = "Enter credential..."
	m.authInput.Focus()

	// Cache CWD once so @ autocomplete never calls os.Getwd() per keystroke.
	if cwd, err := os.Getwd(); err == nil {
		m.atCompleteCWD = cwd
	}

	// Load mascot logo (embedded PNG → block-art lines). Graceful on failure.
	m.logoLines, m.logoWidth = loadLogoLines()

	// Set git branch in status bar (fast, synchronous).
	if branch := currentGitBranch(); branch != "" {
		m.statusBar.SetGitBranch(branch)
	}

	// Connect tool registry if orchestrator has one
	if orch != nil && orch.Registry != nil {
		registry.SetToolRegistry(orch.Registry)

		// Set permission handler
		orch.Registry.SetPermissionHandler(func(toolName string, params map[string]interface{}) tools.PermissionLevel {
			description := "No description available"
			if tool, ok := orch.Registry.Get(toolName); ok {
				description = tool.Description()
			}
			// This will block the goroutine executing the tool, which is what we want
			return m.RequestPermission(toolName, description, params)
		})

		// Set SENSE HITL callback — surfaces plan-and-execute approval requests.
		orch.HITLCallback = func(req engine.HITLRequest) engine.HITLDecision {
			return m.RequestHITLApproval(req)
		}
	}

	// Initialise analytics dashboard.
	m.analytics = NewAnalyticsData()
	if orch != nil && orch.ContextMgr != nil && m.analytics != nil {
		m.analytics.ContextMaxToks = orch.ContextMgr.MaxTokens()
		m.analytics.ContextUsedToks = orch.ContextMgr.TokensUsed()
		m.analytics.ContextUsedPct = orch.ContextMgr.UsedPct()
		m.statusBar.UpdateContext(m.analytics.ContextUsedPct, orch.ContextMgr.TotalCostUSD())
	}

	// Build minimal welcome message — full details available via /about.
	consultantLine := ""
	if consultantName != "" {
		consultantLine = fmt.Sprintf(" · **%s** (Consultant)", consultantName)
	}
	m.addSystemMessage(fmt.Sprintf(
		"**Gorkbot v%s** — %s%s\n\n"+
			"Type `/help` for commands · `/about` for system overview",
		platform.Version, modelName, consultantLine,
	))

	return m, nil
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	m.mouseEnabled = true
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
		m.consultantSpinner.Tick,
		m.statusBar.Init(),
		tea.EnableMouseAllMotion,
		discoveryPollTick(),            // start cloud-brains polling ticker
		glistenTick(),                  // start header animation
		m.pollAllConfiguredProviders(), // populate model lists from all keyed providers on startup
		providerPollTick(),             // schedule periodic re-poll every 5 minutes
	)
}

// Helper methods

func (m *Model) addSystemMessage(content string) {
	m.messages = append(m.messages, Message{
		Role:        "system",
		Content:     content,
		IsEphemeral: true,
	})
}

func (m *Model) addUserMessage(content string) {
	m.messages = append(m.messages, Message{
		Role:           "user",
		Content:        content,
		IntentCategory: m.lastIntentCategory,
	})
}

func (m *Model) addAssistantMessage(content string, isConsultant bool) {
	role := "assistant"
	if isConsultant {
		role = "consultant"
	}
	m.messages = append(m.messages, Message{
		Role:         role,
		Content:      content,
		IsConsultant: isConsultant,
	})
}

func (m *Model) addToolMessage(toolName string, result *tools.ToolResult) {
	m.addToolMessageWithNesting(toolName, result, 0, 0)
}

func (m *Model) addToolMessageWithNesting(toolName string, result *tools.ToolResult, level int, elapsed time.Duration) {
	var content string
	var icon string

	if result.Success {
		icon = tui_style.Success
	} else {
		icon = tui_style.Failure
	}

	// Create a condensed summary line
	// Format: "Tool: [Name] ... [Result Summary]"
	summary := strings.TrimSpace(result.Output)
	if !result.Success {
		summary = strings.TrimSpace(result.Error)
	}

	// Truncate summary to a single line for the "collapsed" view
	if len(summary) > 60 {
		summary = summary[:57] + "..."
	}
	summary = strings.ReplaceAll(summary, "\n", " ")

	// Minimalist, tree-like structure
	content = fmt.Sprintf("%s %s %s", icon, toolName, summary)

	var hookOutput string
	if result.Success {
		hookOutput = result.Output
	} else {
		hookOutput = result.Error
	}

	entry := HookEntry{
		Label:    fmt.Sprintf("Tool: %s", toolName),
		Metadata: fmt.Sprintf("%.2fs", elapsed.Seconds()),
		Active:   false,
		IsFinal:  true,
		IsError:  !result.Success,
		Output:   hookOutput,
		Elapsed:  elapsed,
	}

	m.messages = append(m.messages, Message{
		Role:         "tool",
		Content:      content,
		ToolName:     toolName,
		ToolResult:   result,
		NestLevel:    level,
		MessageType:  "tool",
		Elapsed:      elapsed,
		HookEntries:  []HookEntry{entry},
		IsStructured: true,
		Collapsed:    true, // start compact; Ctrl+O expands
	})
}

// addToolCallMessage inserts a tool_call (request) message that renders as a
// cyan-bordered box showing the tool name and its parameters before the result.
func (m *Model) addToolCallMessage(toolName string, params map[string]interface{}) {
	var p []string
	for k, v := range params {
		p = append(p, fmt.Sprintf("%s: %v", k, v))
	}
	meta := strings.Join(p, ", ")
	if len(meta) > 50 {
		meta = meta[:47] + "..."
	}

	entry := HookEntry{
		Label:    fmt.Sprintf("Tool: %s", toolName),
		Metadata: meta,
		Active:   true,
		Output:   "Executing...",
	}

	m.messages = append(m.messages, Message{
		Role:         "tool_call",
		ToolName:     toolName,
		ToolParams:   params,
		MessageType:  "tool_call",
		HookEntries:  []HookEntry{entry},
		IsStructured: true,
		Collapsed:    true, // start compact; Ctrl+O expands
	})
}

func (m *Model) addInternalMessage(content string, level int) {
	m.messages = append(m.messages, Message{
		Role:        "internal",
		Content:     content,
		NestLevel:   level,
		MessageType: "internal",
		Collapsed:   true, // collapsed by default for tidy streaming
	})
}

func (m *Model) addA2AMessage(content string, level int) {
	m.messages = append(m.messages, Message{
		Role:        "a2a",
		Content:     content,
		NestLevel:   level,
		MessageType: "a2a",
		Collapsed:   true, // collapsed by default for tidy streaming
	})
}

// renderMessages renders all messages to markdown
// renderWithGorkyPrefix adds the Gorky glyph (𝗚 ▸) to the first line of content,
// and indents subsequent lines to align with the first line.
func (m *Model) renderWithGorkyPrefix(content string) string {
	glyph := m.styles.GorkyGlyphStyle.Render(GorkyGlyph)
	lines := strings.Split(content, "\n")

	if len(lines) == 0 {
		return glyph + "  " + content
	}

	// First line: glyph + content (2 spaces after glyph for padding)
	result := glyph + "  " + lines[0]

	// Subsequent lines: indent to align with content start.
	// The Gorky glyph (𝗚 ▸) renders as ~2 display columns + 2 spaces = 4 cols total.
	// Use exactly 4 spaces to align continuation lines properly.
	const glyphDisplayWidth = 4 // visual width: glyph (~2) + 2 spaces
	indent := strings.Repeat(" ", glyphDisplayWidth)

	for i := 1; i < len(lines); i++ {
		result += "\n" + indent + lines[i]
	}

	return result
}

func (m *Model) renderMessages() string {
	var output strings.Builder

	for i, msg := range m.messages {
		// Add nesting prefix for tool/internal/a2a messages
		prefix := m.getNestingPrefix(msg.NestLevel, msg.MessageType)

		switch msg.Role {
		case "user":
			// Wrap user message content to fit within viewport width
			// Reserve space for "You: " prefix and some margin
			wrapWidth := m.width - 9
			if wrapWidth < 20 {
				wrapWidth = 20
			}
			wrappedContent := wrapText(msg.Content, wrapWidth)
			userLine := m.styles.UserMessage.Render(fmt.Sprintf("You: %s", wrappedContent))
			if msg.IntentCategory != "" {
				label := adaptive.CategoryLabel(adaptive.IntentCategory(msg.IntentCategory))
				emoji := adaptive.CategoryEmoji(adaptive.IntentCategory(msg.IntentCategory))
				badge := lipgloss.NewStyle().
					Foreground(lipgloss.Color(TextGray)).
					Background(lipgloss.Color(BgDarkAlt)).
					Padding(0, 1).
					Render(emoji + " " + label)
				userLine = lipgloss.JoinHorizontal(lipgloss.Center, userLine, "  ", badge)
			}
			output.WriteString(userLine)
			output.WriteString("\n\n")

		case "assistant":
			var contentArea string
			if len(msg.HookEntries) > 0 || msg.IsStructured {
				boxWidth := m.width - msg.NestLevel*2 - 4
				if boxWidth < 20 {
					boxWidth = 20
				}
				contentArea = RenderHookTree(msg.HookEntries, boxWidth, m.hookSpinFrame, m.styles.Hook)
				// Apply AIMessage styling to keep the box if it has one, but mostly it's margin/padding
				contentArea = m.styles.AIMessage.Render(contentArea)
			} else {
				// Clean up the content before rendering to avoid excessive indentation
				cleanContent := m.cleanMarkdownContent(msg.Content)
				rendered, err := m.glamour.Render(cleanContent)
				if err != nil {
					rendered = cleanContent
				} else {
					// Trim excessive leading spaces from rendered output
					rendered = m.trimRenderedIndentation(rendered)
				}
				// Add Gorky identity glyph
				contentWithGlyph := m.renderWithGorkyPrefix(rendered)
				contentArea = m.styles.AIMessage.Render(contentWithGlyph)
			}
			output.WriteString(contentArea)
			output.WriteString("\n")

		case "consultant":
			cleanContent := m.cleanMarkdownContent(msg.Content)
			rendered, err := m.glamour.Render(cleanContent)
			if err != nil {
				output.WriteString(m.styles.ConsultantBox.Render(cleanContent))
			} else {
				rendered = m.trimRenderedIndentation(rendered)
				output.WriteString(m.styles.ConsultantBox.Render(rendered))
			}
			output.WriteString("\n")

		case "system":
			rendered, err := m.glamour.Render(msg.Content)
			if err != nil {
				output.WriteString(msg.Content)
			} else {
				output.WriteString(rendered)
			}
			output.WriteString("\n")

		case "tool_call":
			// Render the outgoing JSON tool call request as a cyan-bordered box.
			nameColor := toolCategoryColor(msg.ToolName)
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(nameColor)).Bold(true)
			header := "→ " + nameStyle.Render(msg.ToolName)

			var boxContent string
			boxWidth := m.width - msg.NestLevel*2 - 4
			if boxWidth < 20 {
				boxWidth = 20
			}

			if msg.Collapsed {
				// Compact 1-line summary: "▶ → tool_name ..."
				hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray)).Italic(true)
				boxContent = "▶ " + header + "  " + hintStyle.Render("ctrl+o to expand")
				callBoxStyle := lipgloss.NewStyle().
					Border(lipgloss.NormalBorder()).
					BorderForeground(lipgloss.Color(DraculaCyan)).
					Padding(0, 1).
					Width(boxWidth)
				output.WriteString(callBoxStyle.Render(boxContent))
				output.WriteString("\n")
				break
			}

			if len(msg.HookEntries) > 0 || msg.IsStructured {
				boxContent = RenderHookTree(msg.HookEntries, boxWidth, m.hookSpinFrame, m.styles.Hook)
			} else {
				// Format parameters compactly: key: value, one per line, truncated.
				paramStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray))
				var paramLines []string
				for k, v := range msg.ToolParams {
					val := fmt.Sprintf("%v", v)
					val = strings.ReplaceAll(val, "\n", " ")
					if len(val) > 72 {
						val = val[:69] + "…"
					}
					paramLines = append(paramLines,
						paramStyle.Render(fmt.Sprintf("  %s: %s", k, val)))
				}
				var body string
				if len(paramLines) > 0 {
					body = strings.Join(paramLines, "\n")
				}

				if body != "" {
					boxContent = lipgloss.JoinVertical(lipgloss.Left, header, body)
				} else {
					boxContent = header
				}
			}

			// A tool call is 'active' until the result arrives.
			callBoxStyle := m.styles.ToolBoxActive.Copy().Width(boxWidth)
			output.WriteString(callBoxStyle.Render(boxContent))
			output.WriteString("\n")

		case "tool":
			// Render as a styled bordered box with category color + elapsed time.
			success := msg.ToolResult == nil || msg.ToolResult.Success
			statusIcon := "✓"
			borderColor := SuccessGreen
			if !success {
				statusIcon = "✗"
				borderColor = ErrorRed
			}

			nameColor := toolCategoryColor(msg.ToolName)
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(nameColor))

			if msg.Collapsed {
				// Compact 1-line summary: "▶ 🔧 tool_name · 0.3s ✓  [ctrl+o]"
				elapsed := ""
				if msg.Elapsed > 0 {
					elapsed = fmt.Sprintf("  ·  %.2fs", msg.Elapsed.Seconds())
				}
				hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray)).Italic(true)
				summary := fmt.Sprintf("▶ 🔧 %s%s  %s  %s",
					nameStyle.Render(msg.ToolName), elapsed, statusIcon,
					hintStyle.Render("ctrl+o to expand"))
				boxWidth := m.width - msg.NestLevel*2 - 4
				if boxWidth < 20 {
					boxWidth = 20
				}
				boxStyle := lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color(borderColor)).
					Padding(0, 1).
					Width(boxWidth)
				output.WriteString(boxStyle.Render(summary))
				output.WriteString("\n")
				break
			}

			titleParts := "🔧 " + nameStyle.Render(msg.ToolName)
			if msg.Elapsed > 0 {
				titleParts += fmt.Sprintf("  ·  %.2fs", msg.Elapsed.Seconds())
			}
			titleParts += "  " + statusIcon

			var outputContent string

			if len(msg.HookEntries) > 0 || msg.IsStructured {
				boxWidth := m.width - msg.NestLevel*2 - 4
				if boxWidth < 20 {
					boxWidth = 20
				}
				outputContent = RenderHookTree(msg.HookEntries, boxWidth, m.hookSpinFrame, m.styles.Hook)
			} else {
				// Check for diff data (before/after from file write/edit operations)
				if msg.ToolResult != nil && msg.ToolResult.Data != nil {
					before, hasBefore := msg.ToolResult.Data["before"].(string)
					after, hasAfter := msg.ToolResult.Data["after"].(string)
					if hasBefore && hasAfter {
						diffLines := computeSimpleDiff(before, after)
						const maxDiffLines = 20
						addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(SuccessGreen))
						rmStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ErrorRed))
						ctxStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray))
						var rendered []string
						limit := len(diffLines)
						truncated := 0
						if limit > maxDiffLines && !msg.Collapsed && !msg.FullyExpanded {
							truncated = limit - maxDiffLines
							limit = maxDiffLines
						}
						for _, dl := range diffLines[:limit] {
							if strings.HasPrefix(dl, "+") {
								rendered = append(rendered, addStyle.Render(dl))
							} else if strings.HasPrefix(dl, "-") {
								rendered = append(rendered, rmStyle.Render(dl))
							} else {
								rendered = append(rendered, ctxStyle.Render(dl))
							}
						}
						if truncated > 0 {
							rendered = append(rendered, ctxStyle.Render(fmt.Sprintf("▶ %d more diff lines — ctrl+e to expand fully", truncated)))
						}
						outputContent = strings.Join(rendered, "\n")
					}
				}

				// Fallback to raw output when no diff data.
				if outputContent == "" {
					var rawOutput string
					if msg.ToolResult != nil {
						if msg.ToolResult.Success {
							rawOutput = msg.ToolResult.Output
						} else {
							rawOutput = msg.ToolResult.Error
						}
					}
					
					// Apply context-aware structured formatting (Markdown/JSON) before truncation
					trimmedRaw := strings.TrimSpace(rawOutput)
					if len(trimmedRaw) > 2 && (strings.HasPrefix(trimmedRaw, "{") || strings.HasPrefix(trimmedRaw, "[")) && json.Valid([]byte(trimmedRaw)) {
						mdFormatted := fmt.Sprintf("```json\n%s\n```", trimmedRaw)
						if rendered, err := m.glamour.Render(mdFormatted); err == nil {
							rawOutput = strings.TrimSpace(rendered)
						}
					} else if len(trimmedRaw) > 5 && msg.ToolName == "read_file" {
						// Heuristically assume it's code/config for read_file
						mdFormatted := fmt.Sprintf("```\n%s\n```", trimmedRaw)
						if rendered, err := m.glamour.Render(mdFormatted); err == nil {
							rawOutput = strings.TrimSpace(rendered)
						}
					}

					lines := strings.Split(rawOutput, "\n")
					const maxToolLines = 10
					var displayLines []string
					if len(lines) > maxToolLines && !msg.Collapsed && !msg.FullyExpanded {
						displayLines = lines[:maxToolLines]
						displayLines = append(displayLines,
							fmt.Sprintf("[↓ %d more lines — ctrl+e to expand fully]", len(lines)-maxToolLines))
					} else {
						displayLines = lines
					}
					outputContent = strings.Join(displayLines, "\n")
				}
			}

			boxWidth := m.width - msg.NestLevel*2 - 4
			if boxWidth < 20 {
				boxWidth = 20
			}
			boxContent := lipgloss.JoinVertical(lipgloss.Left, titleParts, outputContent)
			boxStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(borderColor)).
				Padding(0, 1).
				Width(boxWidth)
			output.WriteString(boxStyle.Render(boxContent))
			output.WriteString("\n")

		case "internal":
			style := m.getNestedStyle(msg.NestLevel, "internal")
			if len(msg.HookEntries) > 0 || msg.IsStructured {
				boxWidth := m.width - msg.NestLevel*2 - 4
				if boxWidth < 20 {
					boxWidth = 20
				}
				content := prefix + RenderHookTree(msg.HookEntries, boxWidth, m.hookSpinFrame, m.styles.Hook)
				output.WriteString(style.Render(content))
			} else if msg.Collapsed {
				hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
				n := len([]rune(msg.Content))
				line := fmt.Sprintf("%s💭 ▶ reasoning · %d chars  %s", prefix, n, hintStyle.Render("ctrl+f"))
				output.WriteString(style.Render(line))
			} else {
				content := prefix + msg.Content
				output.WriteString(style.Render(content))
			}
			output.WriteString("\n")

		case "a2a":
			style := m.getNestedStyle(msg.NestLevel, "a2a")
			if len(msg.HookEntries) > 0 || msg.IsStructured {
				boxWidth := m.width - msg.NestLevel*2 - 4
				if boxWidth < 20 {
					boxWidth = 20
				}
				content := prefix + RenderHookTree(msg.HookEntries, boxWidth, m.hookSpinFrame, m.styles.Hook)
				output.WriteString(style.Render(content))
			} else if msg.Collapsed {
				hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
				n := len([]rune(msg.Content))
				line := fmt.Sprintf("%s🔄 ▶ agent·comm · %d chars  %s", prefix, n, hintStyle.Render("ctrl+f"))
				output.WriteString(style.Render(line))
			} else {
				content := prefix + msg.Content
				output.WriteString(style.Render(content))
			}
			output.WriteString("\n")
		}

		// Add separator between messages (except last)
		if i < len(m.messages)-1 {
			output.WriteString("\n")
		}
	}

	// Context pressure banner
	rendered := output.String()
	pct := int(m.statusBar.contextPct * 100)
	if m.statusBar.contextPct >= 0.95 {
		rendered += "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color(ErrorRed)).
			Bold(true).
			Width(m.width-4).
			Render(fmt.Sprintf("⚠  Context at %d%% — critical, run /compress", pct))
	} else if m.statusBar.contextPct >= 0.80 {
		rendered += "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color(WarningYellow)).
			Width(m.width-4).
			Render(fmt.Sprintf("⚡ Context at %d%% — approaching limit", pct))
	}

	// Planning box — shown while AI is generating (hides internal monologue).
	if m.planningActive {
		rendered += "\n" + m.renderPlanningBox()
	}

	return rendered
}

// toolActivityLabel returns a human-readable (icon, label) pair for a tool call,
// extracting meaningful context from the params map where possible.
func toolActivityLabel(name string, params map[string]interface{}) (icon, label string) {
	str := func(key string) string {
		if v, ok := params[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	truncPath := func(p string, n int) string {
		if len(p) <= n {
			return p
		}
		return "…" + p[len(p)-n:]
	}

	switch name {
	case "read_file":
		if p := str("path"); p != "" {
			return "📖", "Reading " + truncPath(p, 42)
		}
	case "write_file":
		if p := str("path"); p != "" {
			return "✏️ ", "Writing " + truncPath(p, 42)
		}
	case "delete_file":
		if p := str("path"); p != "" {
			return "🗑️ ", "Deleting " + truncPath(p, 40)
		}
	case "list_directory":
		if p := str("path"); p != "" {
			return "📂", "Listing " + truncPath(p, 42)
		}
	case "search_files":
		if q := str("pattern"); q != "" {
			return "🔍", "Searching for " + q
		}
	case "grep_content":
		if q := str("pattern"); q != "" {
			return "🔎", "Grepping for " + q
		}
	case "bash":
		if cmd := str("command"); cmd != "" {
			// Show first 48 chars of command
			if len(cmd) > 48 {
				cmd = cmd[:45] + "..."
			}
			return "⚡", cmd
		}
	case "git_status":
		return "🌿", "git status"
	case "git_diff":
		return "📊", "git diff"
	case "git_log":
		return "📜", "git log"
	case "git_commit":
		if msg := str("message"); msg != "" {
			if len(msg) > 45 {
				msg = msg[:42] + "..."
			}
			return "📝", "git commit: " + msg
		}
		return "📝", "git commit"
	case "git_push":
		return "🚀", "git push"
	case "git_pull":
		return "⬇️ ", "git pull"
	case "web_fetch":
		if u := str("url"); u != "" {
			return "🌐", "Fetching " + truncPath(u, 42)
		}
	case "http_request":
		method := str("method")
		if method == "" {
			method = "GET"
		}
		if u := str("url"); u != "" {
			return "🌐", method + " " + truncPath(u, 38)
		}
	case "download_file":
		if u := str("url"); u != "" {
			return "⬇️ ", "Downloading " + truncPath(u, 38)
		}
	case "browser_scrape", "browser_control":
		if u := str("url"); u != "" {
			return "🖥️ ", "Browser → " + truncPath(u, 38)
		}
	case "nmap_scan":
		if t := str("target"); t != "" {
			return "🔬", "nmap " + t
		}
	case "spawn_agent":
		if t := str("type"); t != "" {
			return "🤖", "Spawning " + t + " agent"
		}
		return "🤖", "Spawning agent"
	case "collect_agent":
		return "📥", "Collecting agent result"
	case "create_tool":
		if n := str("name"); n != "" {
			return "🛠️ ", "Creating tool: " + n
		}
	}

	// Default: humanize snake_case name
	humanized := strings.ReplaceAll(name, "_", " ")
	// Capitalize first letter
	if len(humanized) > 0 {
		humanized = strings.ToUpper(humanized[:1]) + humanized[1:]
	}
	return "🔧", humanized
}

// SetTheme switches to a new theme and updates all TUI styles.
// Called when the user selects a theme via /theme command.
func (m *Model) SetTheme(themeName string) error {
	// Update theme manager
	if err := m.themeManager.Set(themeName); err != nil {
		return err
	}

	// Get the new active theme
	newTheme := m.themeManager.Active()

	// Regenerate styles with new theme
	m.styles.RemapForTheme(newTheme)

	// Update theme name
	m.theme = newTheme.Name

	// Regenerate glamour renderer to reflect new code block colors
	if newTheme != nil {
		colors := newTheme.Colors
		// Update glamour style based on new theme colors
		glamourStyle := ansi.StyleConfig{}
		glamourStyle.Document.BlockPrefix = "\n"
		glamourStyle.Document.BlockSuffix = "\n"
		glamourStyle.Document.Margin = uintPtr(0)
		glamourStyle.CodeBlock.Margin = uintPtr(0)
		glamourStyle.CodeBlock.StylePrimitive.BackgroundColor = stringPtr(colors.CodeBg)
		glamourStyle.CodeBlock.StylePrimitive.Color = stringPtr(colors.CodeFg)
		glamourStyle.Code.StylePrimitive.BackgroundColor = stringPtr(colors.InlineCodeBg)
		glamourStyle.Code.StylePrimitive.Color = stringPtr(colors.InlineCodeFg)
		glamourStyle.H1.Color = stringPtr(colors.Header1)
		glamourStyle.H1.Bold = boolPtr(true)
		glamourStyle.H1.BlockSuffix = "\n"
		glamourStyle.H2.Color = stringPtr(colors.Header2)
		glamourStyle.H2.Bold = boolPtr(true)
		glamourStyle.H2.BlockSuffix = "\n"
		glamourStyle.Text.Color = stringPtr(colors.Text)
		glamourStyle.Link.Color = stringPtr(colors.Link)
		glamourStyle.Link.Underline = boolPtr(true)

		r, _ := glamour.NewTermRenderer(
			glamour.WithStyles(glamourStyle),
			glamour.WithWordWrap(80),
		)
		m.glamour = r
	}

	return nil
}

// wrapText wraps text to fit within maxWidth characters.
// Respects word boundaries and handles multi-line input gracefully.
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var wrappedLines []string

	for _, line := range lines {
		wrapped := wrapLine(line, maxWidth)
		wrappedLines = append(wrappedLines, wrapped...)
	}

	return strings.Join(wrappedLines, "\n")
}

// wrapLine wraps a single line of text to maxWidth characters.
func wrapLine(line string, maxWidth int) []string {
	if len(line) <= maxWidth {
		return []string{line}
	}

	var result []string
	words := strings.Fields(line)
	var currentLine string

	for _, word := range words {
		if len(currentLine) == 0 {
			// First word on the line
			if len(word) > maxWidth {
				// Word is longer than maxWidth, split it
				for len(word) > maxWidth {
					result = append(result, word[:maxWidth])
					word = word[maxWidth:]
				}
				currentLine = word
			} else {
				currentLine = word
			}
		} else if len(currentLine)+1+len(word) <= maxWidth {
			// Word fits on current line
			currentLine += " " + word
		} else {
			// Word doesn't fit, start a new line
			result = append(result, currentLine)

			if len(word) > maxWidth {
				// Word is longer than maxWidth, split it
				for len(word) > maxWidth {
					result = append(result, word[:maxWidth])
					word = word[maxWidth:]
				}
				currentLine = word
			} else {
				currentLine = word
			}
		}
	}

	if len(currentLine) > 0 {
		result = append(result, currentLine)
	}

	return result
}

// renderPlanningBox renders the live activity panel shown while the AI is generating.
// Replaces the old "Planning..." box: shows actual actions and thinking progress.
func (m *Model) renderPlanningBox() string {
	boxWidth := m.viewport.Width - 4
	if boxWidth < 20 {
		boxWidth = 20
	}

	borderColor := "#6272a4" // default: thinking (indigo)
	switch m.genPhase {
	case phaseTool:
		borderColor = "#f1fa8c" // yellow: tool executing
	case phaseSynthesizing:
		borderColor = "#50fa7b" // green: composing answer
	}

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 1).
		Width(boxWidth)

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f8f8f2"))
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4"))

	var lines []string

	// ── Activity log (completed tool calls) ───────────────────────────────
	for _, e := range m.activityLog {
		var marker string
		if e.Success {
			marker = okStyle.Render("✓")
		} else {
			marker = errStyle.Render("✗")
		}
		elapsed := ""
		if e.Elapsed >= time.Millisecond {
			elapsed = timeStyle.Render(fmt.Sprintf(" %.1fs", e.Elapsed.Seconds()))
		}
		line := marker + " " + dimStyle.Render(e.Icon) + " " + dimStyle.Render(e.Label) + elapsed
		lines = append(lines, line)
	}

	// ── Current action ────────────────────────────────────────────────────
	// NOTE: Status for thinking/reasoning/grounding is now shown via the single
	// authoritative "G ▶ " status line in renderLoadingIndicator().
	// This activity panel only shows tool execution (phaseTool) and composition (phaseSynthesizing).
	switch m.genPhase {
	case phaseTool:
		if m.currentActivity != nil {
			elapsed := time.Since(m.currentActivity.StartedAt)
			lines = append(lines, "⚡ "+m.currentActivity.Icon+" "+labelStyle.Render(m.currentActivity.Label)+
				" "+timeStyle.Render(fmt.Sprintf("%.1fs", elapsed.Seconds())))
		} else {
			lines = append(lines, "⚡ "+labelStyle.Render("Executing..."))
		}
	case phaseSynthesizing:
		lines = append(lines, "✍️  "+labelStyle.Render("Composing response..."))
	}

	if len(lines) == 0 {
		lines = append(lines, "💡 "+labelStyle.Render("Ready"))
	}

	return panelStyle.Render(strings.Join(lines, "\n"))
}

// ── Session export ────────────────────────────────────────────────────────────

// exportTUISession formats m.messages and writes the result to path.
// ext is "md", "txt", or "pdf". PDF requires pandoc; if unavailable it falls
// back to markdown and reports the fallback.
func (m *Model) exportTUISession(ext, path string) string {
	var content string
	switch ext {
	case "txt":
		content = m.exportAsPlain()
	case "pdf":
		// Generate markdown first, then invoke pandoc to convert.
		md := m.exportAsMarkdown()
		if result := m.exportPDF(md, path); result != "" {
			return result // success or pandoc error message
		}
		// pandoc unavailable — fall back to markdown at a .md path.
		mdPath := strings.TrimSuffix(path, ".pdf") + ".md"
		if err := writeExportFile(mdPath, md); err != nil {
			return fmt.Sprintf("**Export failed**: %v", err)
		}
		return fmt.Sprintf("⚠ pandoc not found — exported as Markdown instead: `%s`", mdPath)
	default: // "md"
		content = m.exportAsMarkdown()
	}
	if err := writeExportFile(path, content); err != nil {
		return fmt.Sprintf("**Export failed**: %v", err)
	}
	return fmt.Sprintf("✅ Session exported to `%s` (%d messages).", path, len(m.messages))
}

// exportAsMarkdown renders m.messages as a Markdown document.
func (m *Model) exportAsMarkdown() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Gorkbot Session Export\n\n*Exported: %s*\n\n---\n\n",
		time.Now().Format("2006-01-02 15:04:05")))
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("## You\n\n%s\n\n---\n\n", msg.Content))
		case "assistant":
			sb.WriteString(fmt.Sprintf("## Gorkbot\n\n%s\n\n---\n\n", msg.Content))
		case "consultant":
			sb.WriteString(fmt.Sprintf("## Consultant\n\n%s\n\n---\n\n", msg.Content))
		case "system":
			// Skip internal UI/system messages to prevent bleed-through in exports
			continue
		case "tool_call":
			sb.WriteString(fmt.Sprintf("**→ Tool call: `%s`**\n\n", msg.ToolName))
		case "tool":
			status := "✓"
			if msg.ToolResult != nil && !msg.ToolResult.Success {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("**🔧 `%s` %s**\n\n", msg.ToolName, status))
			if msg.ToolResult != nil {
				out := msg.ToolResult.Output
				if !msg.ToolResult.Success {
					out = msg.ToolResult.Error
				}
				if out != "" {
					sb.WriteString("```\n" + out + "\n```\n\n")
				}
			}
		}
	}
	return sb.String()
}

// exportAsPlain renders m.messages as plain text.
func (m *Model) exportAsPlain() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Gorkbot Session Export — %s\n%s\n\n",
		time.Now().Format("2006-01-02 15:04:05"),
		strings.Repeat("=", 50)))
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("[You]\n%s\n\n", msg.Content))
		case "assistant":
			sb.WriteString(fmt.Sprintf("[Gorkbot]\n%s\n\n", msg.Content))
		case "consultant":
			sb.WriteString(fmt.Sprintf("[Consultant]\n%s\n\n", msg.Content))
		case "tool_call":
			sb.WriteString(fmt.Sprintf("[Tool → %s]\n", msg.ToolName))
		case "tool":
			status := "OK"
			if msg.ToolResult != nil && !msg.ToolResult.Success {
				status = "FAIL"
			}
			out := ""
			if msg.ToolResult != nil {
				out = msg.ToolResult.Output
				if !msg.ToolResult.Success {
					out = msg.ToolResult.Error
				}
			}
			sb.WriteString(fmt.Sprintf("[Tool %s: %s]\n%s\n\n", status, msg.ToolName, out))
		}
	}
	return sb.String()
}

// exportPDF converts markdown content to PDF at path using pandoc.
// Returns the success/error message, or "" if pandoc is not found.
func (m *Model) exportPDF(md, path string) string {
	// Write markdown to a temp file.
	tmpFile := path + ".tmp.md"
	if err := writeExportFile(tmpFile, md); err != nil {
		return fmt.Sprintf("**Export failed** (temp file): %v", err)
	}
	// Run pandoc: pandoc tmp.md -o output.pdf --pdf-engine=xelatex (or wkhtmltopdf)
	// Try pandoc with default engine first; many Termux installs use xelatex or wkhtmltopdf.
	args := []string{tmpFile, "-o", path, "--wrap=none"}
	cmd := exec.Command("pandoc", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		// pandoc failed or not found — remove temp, caller will fall back.
		os.Remove(tmpFile)
		if strings.Contains(err.Error(), "executable file not found") ||
			strings.Contains(err.Error(), "no such file") {
			return "" // signal: not available
		}
		return fmt.Sprintf("**pandoc error**: %s\n\nInstall pandoc: `pkg install pandoc`", string(out))
	}
	os.Remove(tmpFile)
	return fmt.Sprintf("✅ Session exported to `%s` (%d messages).", path, len(m.messages))
}

// exportAsJSON serialises m.messages to JSON and returns the string.
func (m *Model) exportAsJSON() string {
	type jsonMsg struct {
		Role     string `json:"role"`
		Content  string `json:"content,omitempty"`
		ToolName string `json:"tool,omitempty"`
		Success  *bool  `json:"success,omitempty"`
		Output   string `json:"output,omitempty"`
		Elapsed  string `json:"elapsed,omitempty"`
	}
	var items []jsonMsg
	for _, msg := range m.messages {
		jm := jsonMsg{Role: msg.Role, Content: msg.Content}
		if msg.ToolName != "" {
			jm.ToolName = msg.ToolName
		}
		if msg.ToolResult != nil {
			v := msg.ToolResult.Success
			jm.Success = &v
			if msg.ToolResult.Success {
				jm.Output = msg.ToolResult.Output
			} else {
				jm.Output = msg.ToolResult.Error
			}
		}
		if msg.Elapsed > 0 {
			jm.Elapsed = msg.Elapsed.String()
		}
		items = append(items, jm)
	}
	type sessionJSON struct {
		ExportedAt string    `json:"exported_at"`
		Model      string    `json:"model"`
		Messages   []jsonMsg `json:"messages"`
	}
	sess := sessionJSON{
		ExportedAt: time.Now().Format(time.RFC3339),
		Model:      m.currentModel,
		Messages:   items,
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

// ── Search helpers ────────────────────────────────────────────────────────────

// rebuildSearchMatches refreshes m.searchMatches based on m.searchQuery.
func (m *Model) rebuildSearchMatches() {
	m.searchMatches = m.searchMatches[:0]
	if m.searchQuery == "" {
		m.searchMatchIdx = 0
		return
	}
	lq := strings.ToLower(m.searchQuery)
	for i, msg := range m.messages {
		haystack := strings.ToLower(msg.Content + msg.ToolName)
		if msg.ToolResult != nil {
			haystack += strings.ToLower(msg.ToolResult.Output + msg.ToolResult.Error)
		}
		if strings.Contains(haystack, lq) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
	if m.searchMatchIdx >= len(m.searchMatches) {
		m.searchMatchIdx = 0
	}
}

// applySearchHighlight post-processes a rendered content string: it marks every
// line that contains the search query with a leading indicator and wraps the
// matched text in an ANSI highlight. Returns the highlighted content and a slice
// of line indices (0-based) where matches were found.
func applySearchHighlight(content, query string) (string, []int) {
	if query == "" {
		return content, nil
	}
	lq := strings.ToLower(query)
	hl := lipgloss.NewStyle().
		Background(lipgloss.Color("220")).
		Foreground(lipgloss.Color("0")).
		Bold(true)
	indicator := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("▶ ")

	lines := strings.Split(content, "\n")
	var matchLines []int
	for i, line := range lines {
		plain := strings.ToLower(stripANSICodes(line))
		if !strings.Contains(plain, lq) {
			continue
		}
		matchLines = append(matchLines, i)
		// Best-effort: replace first occurrence of query (case-insensitive) in
		// the raw line with a highlighted version. ANSI codes already present
		// in the line may cause minor visual artifacts but the text is readable.
		idx := strings.Index(strings.ToLower(line), lq)
		if idx >= 0 {
			lines[i] = indicator + line[:idx] + hl.Render(line[idx:idx+len(query)]) + line[idx+len(query):]
		} else {
			lines[i] = indicator + line
		}
	}
	return strings.Join(lines, "\n"), matchLines
}

// writeExportFile writes content to path, creating parent directories as needed.
func writeExportFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// toolCategoryColor returns a Lip Gloss color string for a tool name.
func toolCategoryColor(name string) string {
	switch {
	case name == "bash":
		return DraculaOrange
	case strings.HasPrefix(name, "read_") || strings.HasPrefix(name, "write_") ||
		strings.HasPrefix(name, "list_") || strings.HasPrefix(name, "grep_") ||
		strings.HasPrefix(name, "file_") || strings.HasPrefix(name, "delete_"):
		return DraculaCyan
	case strings.HasPrefix(name, "git_"):
		return DraculaPink
	case strings.HasPrefix(name, "web_") || strings.HasPrefix(name, "http_") ||
		strings.HasPrefix(name, "browser_"):
		return DraculaPurple
	case strings.HasPrefix(name, "nmap") || strings.HasPrefix(name, "sqlmap") ||
		strings.HasPrefix(name, "nuclei"):
		return ErrorRed
	case name == "spawn_agent" || name == "collect_agent" || name == "list_agents":
		return GrokBlue
	case strings.HasSuffix(name, "_hashed"):
		return DraculaYellow
	default:
		return SuccessGreen
	}
}

// getNestingPrefix creates a tree-like prefix for nested messages
func (m *Model) getNestingPrefix(level int, msgType string) string {
	if level == 0 {
		return ""
	}

	var prefix strings.Builder
	for i := 0; i < level; i++ {
		if i < level-1 {
			prefix.WriteString("  │ ") // Vertical line for continuation
		} else {
			prefix.WriteString("  └─ ") // Branch for the current item
		}
	}

	// Add type indicator
	switch msgType {
	case "tool":
		prefix.WriteString("🔧 ")
	case "internal":
		prefix.WriteString("💭 ")
	case "a2a":
		prefix.WriteString("🔄 ")
	}

	return prefix.String()
}

// getNestedStyle returns a style based on nesting level and message type
func (m *Model) getNestedStyle(level int, msgType string) lipgloss.Style {
	baseStyle := lipgloss.NewStyle()

	// Color based on message type
	switch msgType {
	case "tool":
		// Tools: Collapsed Grey look (Hierarchical tree structure)
		// We use shades of grey to indicate depth/subservience
		baseStyle = baseStyle.Foreground(lipgloss.Color("240"))

	case "internal":
		// Internal monologue: Dark yellow/gold
		baseStyle = baseStyle.Foreground(lipgloss.Color("136")) // Muted gold

	case "a2a":
		// A2A comms: Dark magenta
		baseStyle = baseStyle.Foreground(lipgloss.Color("127")) // Muted magenta
	}

	return baseStyle
}

// updateViewportContent updates the viewport with rendered messages
func (m *Model) updateViewportContent() {
	content := m.renderMessages()

	// Apply search highlighting and scroll to current match when search is active.
	if m.searchMode && m.searchQuery != "" {
		var matchLines []int
		content, matchLines = applySearchHighlight(content, m.searchQuery)
		m.viewport.SetContent(content)
		if len(matchLines) > 0 {
			idx := m.searchMatchIdx
			if idx < 0 || idx >= len(matchLines) {
				idx = 0
			}
			targetLine := matchLines[idx]
			if targetLine > m.viewport.Height/2 {
				targetLine -= m.viewport.Height / 2
			}
			m.viewport.SetYOffset(targetLine)
		}
		return
	}

	m.viewport.SetContent(content)

	// Auto-scroll to bottom unless the user has manually scrolled up.
	if !m.userScrolledUp {
		m.viewport.GotoBottom()
	}
	// When all content fits on screen there is nothing to scroll; allow
	// auto-scroll again on the next message.
	if m.viewport.TotalLineCount() <= m.viewport.Height {
		m.userScrolledUp = false
	}
}

// currentGitBranch returns the current git branch name, or "" if not in a repo.
func currentGitBranch() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// handleCommand processes a slash command
func (m *Model) handleCommand(input string) tea.Cmd {
	// Parse command and arguments
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	cmdName := strings.TrimPrefix(parts[0], "/")
	args := parts[1:]

	// Execute command
	result, err := m.commands.Execute(cmdName, args)
	if err != nil {
		m.err = err
		return nil
	}

	// Handle special signals
	switch {
	case result == "CLEAR_SCREEN":
		m.messages = []Message{}
		// Clear orchestrator conversation history
		if m.orchestrator != nil {
			m.orchestrator.ClearHistory()
		}
		m.addSystemMessage("# Screen Cleared\n\nConversation history has been reset.")
		m.updateViewportContent()
		return nil

	case result == "QUIT":
		m.quitting = true
		return tea.Quit

	case result == "MOUSE_TOGGLE":
		m.mouseEnabled = !m.mouseEnabled
		var cmd tea.Cmd
		status := "disabled"
		if m.mouseEnabled {
			cmd = tea.EnableMouseAllMotion
			status = "enabled (Keyboard hidden)"
		} else {
			cmd = tea.DisableMouse
			status = "disabled (Keyboard visible)"
		}
		m.addSystemMessage(fmt.Sprintf("**Mouse support %s**", status))
		m.updateViewportContent()
		return cmd

	case result == "DEBUG_ON":
		m.debugMode = true
		m.addSystemMessage("**Debug mode ON** — raw AI output (including tool JSON) is now visible.")
		m.updateViewportContent()
		return nil

	case result == "DEBUG_OFF":
		m.debugMode = false
		m.addSystemMessage("**Debug mode OFF** — tool JSON blocks are now hidden from output.")
		m.updateViewportContent()
		return nil

	case strings.HasPrefix(result, "MODEL_SWITCH_PRIMARY:"):
		modelID := strings.TrimPrefix(result, "MODEL_SWITCH_PRIMARY:")
		if err := m.switchPrimaryModel(modelID); err != nil {
			m.addSystemMessage(fmt.Sprintf("❌ Failed to switch primary model: %v", err))
		} else {
			m.addSystemMessage(fmt.Sprintf("✅ Primary model switched to **%s**", modelID))
		}
		m.updateViewportContent()
		return nil

	case strings.HasPrefix(result, "MODEL_SWITCH_CONSULTANT:"):
		modelID := strings.TrimPrefix(result, "MODEL_SWITCH_CONSULTANT:")
		if err := m.switchConsultantModel(modelID); err != nil {
			m.addSystemMessage(fmt.Sprintf("❌ Failed to switch consultant model: %v", err))
		} else {
			m.addSystemMessage(fmt.Sprintf("✅ Consultant model switched to **%s**", modelID))
		}
		m.updateViewportContent()
		return nil

	case strings.HasPrefix(result, "THEME:"):
		themeName := strings.TrimPrefix(result, "THEME:")
		if err := m.SetTheme(themeName); err != nil {
			m.addSystemMessage(fmt.Sprintf("❌ Failed to switch theme: %v", err))
		} else {
			m.addSystemMessage(fmt.Sprintf("✅ Switched to **%s** theme", m.theme))
		}
		m.updateViewportContent()
		return nil

	case strings.HasPrefix(result, "REWIND_COMPLETE:"):
		// Signal format: REWIND_COMPLETE:<checkpoint_id>:<msg_count>
		rest := strings.TrimPrefix(result, "REWIND_COMPLETE:")
		parts := strings.SplitN(rest, ":", 2)
		desc := parts[0]
		count := 0
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &count)
		}
		// Wipe the TUI's displayed history so the viewport reflects the restored state.
		m.messages = []Message{}
		m.addSystemMessage(fmt.Sprintf(
			"_Rewound to checkpoint **%s** — conversation restored to %d messages_",
			desc, count,
		))
		m.updateViewportContent()
		if !m.userScrolledUp {
			m.viewport.GotoBottom()
		}
		return nil

	case strings.HasPrefix(result, "SESSION_LOADED:"):
		// Signal format: SESSION_LOADED:<name>:<count>
		rest := strings.TrimPrefix(result, "SESSION_LOADED:")
		parts := strings.SplitN(rest, ":", 2)
		name := parts[0]
		count := 0
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &count)
		}
		// Replace TUI display; orchestrator history is already replaced.
		m.messages = []Message{}
		m.addSystemMessage(fmt.Sprintf(
			"_Session **%s** resumed — %d messages restored._\n\n"+
				"The conversation context has been loaded. Continue from where you left off.",
			name, count,
		))
		m.updateViewportContent()
		if !m.userScrolledUp {
			m.viewport.GotoBottom()
		}
		return nil

	case result == "SETTINGS_MODAL":
		var debugOn bool
		if m.commands != nil && m.commands.Orch != nil && m.commands.Orch.ToggleDebug != nil {
			// Read current debug state without flipping it — toggle it and immediately toggle back
			// is too noisy; instead read from model.debugMode which is synced with the orchestrator.
			debugOn = m.debugMode
		}
		var appStateSetter func(cats []string) error
		var providerSetter func(ids []string) error
		// Wire appState setters if available via the adapter
		if m.commands != nil && m.commands.Orch != nil {
			adapter := m.commands.Orch
			if adapter.PersistDisabledCategories != nil {
				appStateSetter = adapter.PersistDisabledCategories
			}
			if adapter.PersistDisabledProviders != nil {
				providerSetter = adapter.PersistDisabledProviders
			}
		}
		var toolReg *tools.Registry
		if m.commands != nil {
			toolReg = m.commands.GetToolRegistry()
		}
		var sreEnabledSetter func(bool) error
		var ensembleSetter func(bool) error
		sreEnabled, ensembleEnabled := false, false
		if m.commands != nil && m.commands.Orch != nil {
			if m.commands.Orch.SetSREEnabled != nil {
				sreEnabledSetter = m.commands.Orch.SetSREEnabled
			}
			if m.commands.Orch.SetEnsembleEnabled != nil {
				ensembleSetter = m.commands.Orch.SetEnsembleEnabled
			}
			if m.commands.Orch.GetSREEnabled != nil {
				sreEnabled = m.commands.Orch.GetSREEnabled()
			}
			if m.commands.Orch.GetEnsembleEnabled != nil {
				ensembleEnabled = m.commands.Orch.GetEnsembleEnabled()
			}
		}
		m.activeOverlay = NewSettingsOverlay(m.width, m.height, m.commands.Orch, toolReg, appStateSetter, debugOn, providerSetter, m.integrationGetter, m.integrationSetter, sreEnabled, ensembleEnabled, sreEnabledSetter, ensembleSetter)
		return nil

	case strings.HasPrefix(result, "SAVE_SESSION_OK:"):
		// /save completed: session was stored under an auto-generated or user-supplied name.
		name := strings.TrimPrefix(result, "SAVE_SESSION_OK:")
		m.addSystemMessage(fmt.Sprintf("✅ Session saved as **%s**.", name))
		m.updateViewportContent()
		return nil

	case strings.HasPrefix(result, "EXPORT_TUI:"):
		// /export: format TUI messages and write to file.
		// Signal format: EXPORT_TUI:<ext>:<path>
		rest := strings.TrimPrefix(result, "EXPORT_TUI:")
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			m.addSystemMessage("**Export error**: malformed signal (missing path).")
			m.updateViewportContent()
			return nil
		}
		ext := rest[:colonIdx]
		path := rest[colonIdx+1:]
		msg := m.exportTUISession(ext, path)
		m.addSystemMessage(msg)
		m.updateViewportContent()
		if !m.userScrolledUp {
			m.viewport.GotoBottom()
		}
		return nil

	case strings.HasPrefix(result, "SKILL_INVOKE:"):
		// Skill invocation — inject the rendered skill template as a normal user prompt.
		skillPrompt := strings.TrimPrefix(result, "SKILL_INVOKE:")
		if skillPrompt == "" {
			return nil
		}
		// Process like a regular user prompt (mirrors submitPrompt logic).
		m.addUserMessage(skillPrompt)
		m.updateViewportContent()
		m.userScrolledUp = false
		upperSkill := strings.ToUpper(skillPrompt)
		m.isConsultant = strings.Contains(upperSkill, "COMPLEX") ||
			strings.Contains(upperSkill, "REFRESH") ||
			len(skillPrompt) > 1000
		m.generating = true
		m.currentPhrase = GetRandomPhrase(m.isConsultant)
		m.currentResponse.Reset()
		return tea.Batch(
			m.spinner.Tick,
			phraseTick(),
			m.callOrchestrator(skillPrompt),
		)

	default:
		// Regular command output
		m.addSystemMessage(result)
		m.updateViewportContent()
		return nil
	}
}

// handleBangCommand processes a bang command (!cmd)
func (m *Model) handleBangCommand(input string) tea.Cmd {
	cmdStr := strings.TrimPrefix(input, "!")
	cmdStr = strings.TrimSpace(cmdStr)

	if cmdStr == "" {
		return nil
	}

	// Split command and args (simple split)
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil
	}

	cmdName := parts[0]
	args := parts[1:]

	id := fmt.Sprintf("cmd-%d", len(m.processManager.ListProcesses())+1)

	// Start process
	// Note: We use PTY by default for ! commands to support interactive apps
	proc, err := m.processManager.Start(id, cmdName, args, true)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("❌ Failed to start process: %v", err))
		m.updateViewportContent()
		return nil
	}

	// Set up streaming output to chat
	processID := id
	sendMsg := func(output string, isStderr, done bool, exitCode int) {
		if m.program != nil {
			m.program.Send(ProcessOutputMsg{
				ProcessID: processID,
				Output:    output,
				IsStderr:  isStderr,
				Done:      done,
				ExitCode:  exitCode,
			})
		}
	}

	// Create buffered writer for streaming
	stdoutWriter := &processStreamWriter{
		processID: id,
		sendMsg:   sendMsg,
		isStderr:  false,
		buffer:    make([]byte, 0, 4096),
	}
	stderrWriter := &processStreamWriter{
		processID: id,
		sendMsg:   sendMsg,
		isStderr:  true,
		buffer:    make([]byte, 0, 4096),
	}

	// Attach writers to process (goroutines already started, but they'll check these)
	proc.StdoutStream = stdoutWriter
	proc.StderrStream = stderrWriter

	// Set up completion callback to send final message
	proc.OnComplete = func(exitCode int, isError bool) {
		sendMsg("", false, true, exitCode)
	}

	m.addSystemMessage(fmt.Sprintf("🚀 Started process **%s** (%s)", id, cmdStr))
	m.updateViewportContent()

	// Update status bar immediately
	m.statusBar.SetStatus(fmt.Sprintf("Started %s", cmdName))

	return nil
}

// processStreamWriter buffers process output and sends it to the TUI as messages.
type processStreamWriter struct {
	processID string
	sendMsg   func(string, bool, bool, int)
	isStderr  bool
	buffer    []byte
}

func (w *processStreamWriter) Write(p []byte) (int, error) {
	w.buffer = append(w.buffer, p...)
	// Flush on newline or when buffer gets large
	flushed := false
	for i, b := range w.buffer {
		if b == '\n' {
			output := string(w.buffer[:i])
			w.buffer = w.buffer[i+1:]
			w.sendMsg(output, w.isStderr, false, 0)
			flushed = true
		}
	}
	// If buffer too large, flush what we have
	if len(w.buffer) > 1024 && !flushed {
		output := string(w.buffer)
		w.buffer = w.buffer[:0]
		w.sendMsg(output, w.isStderr, false, 0)
	}
	return len(p), nil
}

// stripANSICodes removes all ANSI escape sequences and control characters from a string
func stripANSICodes(s string) string {
	// First pass: Remove sequences starting with ESC
	s = stripEscapeSequences(s)

	// Second pass: Remove partial/corrupted escape sequences
	s = stripPartialSequences(s)

	return s
}

// stripEscapeSequences removes complete escape sequences
func stripEscapeSequences(s string) string {
	result := strings.Builder{}
	result.Grow(len(s)) // Pre-allocate

	i := 0
	for i < len(s) {
		ch := s[i]

		// Handle ESC sequences
		if ch == 0x1B && i+1 < len(s) { // ESC character
			i++
			next := s[i]

			switch next {
			case '[': // CSI - Control Sequence Introducer
				// Skip until we find a letter (A-Z, a-z) or tilde (~)
				i++
				for i < len(s) {
					if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') || s[i] == '~' {
						i++
						break
					}
					i++
				}
				continue

			case ']': // OSC - Operating System Command
				// Skip until BEL (0x07) or ESC \ (String Terminator)
				i++
				for i < len(s) {
					if s[i] == 0x07 { // BEL
						i++
						break
					}
					if s[i] == 0x1B && i+1 < len(s) && s[i+1] == '\\' { // ESC \
						i += 2
						break
					}
					i++
				}
				continue

			case 'P': // DCS - Device Control String
				fallthrough
			case '_': // APC - Application Program Command
				fallthrough
			case '^': // PM - Privacy Message
				// Skip until ESC \ (String Terminator)
				i++
				for i < len(s) {
					if s[i] == 0x1B && i+1 < len(s) && s[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
				continue

			case '(', ')', '*', '+': // Character set selection
				// Skip next character
				i++
				if i < len(s) {
					i++
				}
				continue

			default:
				// Other ESC sequences - skip the next character
				i++
				continue
			}
		}

		// Filter out other control characters except newline, tab, carriage return
		if ch < 32 && ch != '\n' && ch != '\t' && ch != '\r' {
			i++
			continue
		}

		// Keep printable characters
		result.WriteByte(ch)
		i++
	}

	return result.String()
}

// cleanMarkdownContent removes excessive indentation from markdown content and cleans up formatting
func (m *Model) cleanMarkdownContent(content string) string {
	// 1. Deduplicate LLM generation stutters (e.g. repeated sentences or chunks)
	content = deduplicateStutter(content)

	// 2. Un-wrap lines that were improperly broken by the terminal/output wrapping.
	// We want to join lines that don't look like intentional line breaks.
	lines := strings.Split(content, "\n")
	var cleaned []string

	// Generalized list fixes
	// Matches things like "1Text", "1.Text", "1)Text", "-Text", "*Text" and normalizes them
	listSquashRegex := regexp.MustCompile(`^(\s*)([0-9]+[.)]?|[-*+])([A-Za-z])`)

	// Detects start of any list, header, blockquote, or codeblock
	isNewBlockRegex := regexp.MustCompile(`^(\s*)([0-9]+[.)]\s+|[-*+]\s+|#+ |>|` + "```" + `)`)

	inCodeBlock := false
	var currentParagraph strings.Builder

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if currentParagraph.Len() > 0 {
				cleaned = append(cleaned, currentParagraph.String())
				currentParagraph.Reset()
			}
			cleaned = append(cleaned, line)
			inCodeBlock = !inCodeBlock
			continue
		}

		if inCodeBlock {
			cleaned = append(cleaned, line)
			continue
		}

		if listSquashRegex.MatchString(line) {
			matches := listSquashRegex.FindStringSubmatch(line)
			if len(matches) == 4 {
				indent := matches[1]
				bullet := matches[2]
				char := matches[3]

				if len(bullet) > 0 && unicode.IsDigit(rune(bullet[0])) && !strings.HasSuffix(bullet, ".") && !strings.HasSuffix(bullet, ")") {
					bullet += "."
				}

				line = indent + bullet + " " + char + line[len(matches[0]):]
				trimmed = strings.TrimSpace(line)
			}
		}

		if trimmed == "" {
			if currentParagraph.Len() > 0 {
				cleaned = append(cleaned, currentParagraph.String())
				currentParagraph.Reset()
			}
			cleaned = append(cleaned, "")
			continue
		}

		isNewBlock := isNewBlockRegex.MatchString(line)

		if isNewBlock && currentParagraph.Len() > 0 {
			cleaned = append(cleaned, currentParagraph.String())
			currentParagraph.Reset()
		}

		if currentParagraph.Len() > 0 {
			lastChar := currentParagraph.String()[currentParagraph.Len()-1]
			if lastChar != ' ' && lastChar != '-' {
				currentParagraph.WriteByte(' ')
			}
			currentParagraph.WriteString(trimmed)
		} else {
			leftTrimmed := strings.TrimLeft(line, " ")
			leadingSpaces := len(line) - len(leftTrimmed)
			if leadingSpaces > 6 {
				currentParagraph.WriteString("  " + leftTrimmed)
			} else {
				currentParagraph.WriteString(line)
			}
		}

		if i == len(lines)-1 && currentParagraph.Len() > 0 {
			cleaned = append(cleaned, currentParagraph.String())
		}
	}

	return strings.Join(cleaned, "\n")
}

// deduplicateStutter removes exact phrase repetitions generated by runaway LLMs.
func deduplicateStutter(s string) string {
	if len(s) < 10 {
		return s
	}

	// We look for patterns of length L repeating immediately
	// Try lengths from len(s)/2 down to 10
	for l := len(s) / 2; l >= 10; l-- {
		for i := 0; i <= len(s)-2*l; i++ {
			chunk1 := s[i : i+l]
			chunk2 := s[i+l : i+2*l]

			if chunk1 == chunk2 {
				// Found a stutter. Slice it out and recurse to catch multiple stutters.
				return deduplicateStutter(s[:i+l] + s[i+2*l:])
			}
		}
	}

	// Also check for partial phrase stutter like "Hello! How can I help you today?Hello!"
	// Where the end of a string repeats the beginning.
	for overlap := 5; overlap < len(s)/2; overlap++ {
		prefix := s[:overlap]
		if strings.Contains(s[overlap:], prefix) {
			idx := strings.Index(s[overlap:], prefix) + overlap
			// If the prefix is immediately following some punctuation, or at the end
			// It might be a restart stutter. Let's be careful.
			if idx > 0 && (s[idx-1] == ' ' || s[idx-1] == '\n' || s[idx-1] == '?' || s[idx-1] == '!') {
				// We found a repetition of the start of the message later in the message.
				// If it's a known greeting stutter, strip the first part.
				if strings.HasPrefix(strings.ToLower(prefix), "hello") {
					return deduplicateStutter(s[idx:])
				}
			}
		}
	}

	return s
}

// trimRenderedIndentation removes excessive indentation from rendered markdown
func (m *Model) trimRenderedIndentation(rendered string) string {
	lines := strings.Split(rendered, "\n")
	var result []string

	for _, line := range lines {
		// Trim lines that start with excessive whitespace
		trimmed := strings.TrimLeft(line, " ")
		leadingSpaces := len(line) - len(trimmed)

		// If there are more than 4 leading spaces, reduce them
		if leadingSpaces > 4 && trimmed != "" {
			result = append(result, "  "+trimmed)
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// stripPartialSequences removes partial/corrupted escape sequences that lack ESC prefix
func stripPartialSequences(s string) string {
	// Remove patterns like "11;rgb:0000/0000/0000\"
	// These are OSC sequences without the ESC ] prefix

	// Pattern 1: Digit followed by semicolon and "rgb:" pattern
	// Example: "11;rgb:0000/0000/0000\"
	result := strings.Builder{}
	result.Grow(len(s))

	i := 0
	for i < len(s) {
		// Check for patterns like "11;rgb:" or similar escape sequence patterns
		if i < len(s)-6 {
			// Check for "N;rgb:" pattern (where N is a digit or two)
			if s[i] >= '0' && s[i] <= '9' {
				// Look ahead for ;rgb: pattern
				lookahead := i
				digitCount := 0
				for lookahead < len(s) && s[lookahead] >= '0' && s[lookahead] <= '9' && digitCount < 3 {
					lookahead++
					digitCount++
				}

				// Check if followed by ";rgb:" or ";rgba:"
				if lookahead < len(s)-4 && s[lookahead] == ';' {
					remaining := s[lookahead+1:]
					if strings.HasPrefix(remaining, "rgb:") || strings.HasPrefix(remaining, "rgba:") {
						// This looks like a partial OSC color sequence, skip to the end
						// Find the terminator (backslash, space, or newline)
						for i < len(s) && s[i] != '\\' && s[i] != ' ' && s[i] != '\n' {
							i++
						}
						// Skip the terminator too if it's a backslash
						if i < len(s) && s[i] == '\\' {
							i++
						}
						continue
					}
				}
			}
		}

		// Not a partial escape sequence, keep the character
		result.WriteByte(s[i])
		i++
	}

	return result.String()
}

// submitPrompt handles user prompt submission
func (m *Model) submitPrompt() tea.Cmd {
	input := m.textarea.Value()

	// Strip ANSI escape codes and control characters
	input = stripANSICodes(input)
	input = strings.TrimSpace(input)

	if input == "" {
		m.textarea.Reset()
		return nil
	}

	// Clear textarea
	m.textarea.Reset()

	// Append to input history (keep last 200 entries).
	m.inputHistory = append(m.inputHistory, input)
	m.inputHistoryLower = append(m.inputHistoryLower, strings.ToLower(input))
	if len(m.inputHistory) > 200 {
		m.inputHistory = m.inputHistory[len(m.inputHistory)-200:]
		m.inputHistoryLower = m.inputHistoryLower[len(m.inputHistoryLower)-200:]
	}

	// Check if it's a command
	if strings.HasPrefix(input, "/") {
		// Handle HITL commands before routing to the general handler.
		if strings.HasPrefix(input, "/hitl") {
			return m.handleHITLCommand(input)
		}
		return m.handleCommand(input)
	}

	// Check if it's a bang command
	if strings.HasPrefix(input, "!") {
		return m.handleBangCommand(input)
	}

	// Classify intent for ARC badge
	if cat := adaptive.ClassifyIntent(input); cat != adaptive.CategoryAuto {
		m.lastIntentCategory = string(cat)
	} else {
		m.lastIntentCategory = ""
	}

	// Add user message
	m.addUserMessage(input)
	m.updateViewportContent()

	// Reset scroll state for new prompt - user wants to see the new response
	m.userScrolledUp = false

	// Pre-compute consultant flag using the same heuristics as callOrchestrator.
	upperInput := strings.ToUpper(input)
	m.isConsultant = strings.Contains(upperInput, "COMPLEX") ||
		strings.Contains(upperInput, "REFRESH") ||
		len(input) > 1000

	// Start generation
	m.generating = true
	m.currentPhrase = GetRandomPhrase(m.isConsultant)
	m.currentResponse.Reset()

	return tea.Batch(
		m.spinner.Tick,
		phraseTick(),
		m.callOrchestrator(input),
	)
}

// callOrchestrator calls the orchestrator to generate a response
func (m *Model) callOrchestrator(prompt string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Check if prompt needs consultation based on orchestrator logic
		upperPrompt := strings.ToUpper(prompt)
		isConsult := strings.Contains(upperPrompt, "COMPLEX") ||
			strings.Contains(upperPrompt, "REFRESH") ||
			len(prompt) > 1000

		// Send start generation message
		if prog := m.getProgram(); prog != nil {
			prog.Send(StartGenerationMsg{IsConsultant: isConsult, Prompt: prompt})
		}

		// Reset the extended-thinking buffer for this round-trip.
		if prog := m.getProgram(); prog != nil {
			prog.Send(thinkingResetMsg{})
		}

		// planBuf tracks the current segment in the goroutine; reset on each tool call.
		// At the end of the SENSE loop, planBuf holds the final answer text.
		var planBuf strings.Builder

		// Stream callback — tokens go to the planning box, NOT to the live chat stream.
		// inJSONBlock filters raw JSON tool-call markup that leaks from some models.
		inJSONBlock := false

		streamCallback := func(token string) {
			// Retry signal: the stream rewound; clear the local planning buffer.
			if token == "[__GORKBOT_STREAM_RETRY__]" {
				planBuf.Reset()
				if prog := m.getProgram(); prog != nil {
					prog.Send(PlanningTokenMsg{Content: token})
				}
				return
			}

			// Drop tokens inside JSON tool-call blocks.
			if inJSONBlock {
				if strings.Contains(token, "[/TOOL_CALL]") {
					inJSONBlock = false
				}
				return
			}
			if strings.Contains(token, "\"tool_calls\"") ||
				strings.Contains(token, "\"function\"") ||
				strings.Contains(token, "[TOOL_CALL]") {
				inJSONBlock = true
				return
			}

			planBuf.WriteString(token)
			if prog := m.getProgram(); prog != nil {
				prog.Send(PlanningTokenMsg{Content: token})
			}
		}

		// Tool callback — called after each tool completes.
		toolCallback := func(toolName string, result *tools.ToolResult) {
			if prog := m.getProgram(); prog != nil {
				// Deliver the result message for display.
				prog.Send(ToolExecutionMsg{ToolName: toolName, Result: result})
				// Mark the tool done in the live panel.
				success := result != nil && result.Success
				prog.Send(ToolProgressMsg{ToolName: toolName, Done: true, Success: success})
			}
		}

		// toolStartCallback — called just before each tool begins.
		// Clears the planning buffer (discards the inter-tool reasoning) and sends
		// a ToolCallMsg (request box) plus a ToolProgressMsg (live panel).
		toolStartCallback := func(toolName string, params map[string]interface{}) {
			planBuf.Reset() // discard planning reasoning; only the final segment is shown
			if prog := m.getProgram(); prog != nil {
				prog.Send(PlanningBoxClearMsg{})
				prog.Send(ToolCallMsg{ToolName: toolName, Params: params})
				prog.Send(ToolProgressMsg{ToolName: toolName, Done: false})
			}
		}

		// Intervention callback for Watchdog
		interventionCallback := func(severity engine.WatchdogSeverity, contextStr string) engine.InterventionResponse {
			if prog := m.getProgram(); prog != nil {
				responseChan := make(chan engine.InterventionResponse)
				prog.Send(InterventionRequestMsg{
					Severity:     severity,
					Context:      contextStr,
					ResponseChan: responseChan,
				})
				// Block until user responds
				return <-responseChan
			}
			return engine.InterventionStop // Default if no UI
		}

		// Wire extended-thinking callback so the TUI can show a thinking panel.
		// The closure captures prog to avoid races; the \x03 sentinel signals done.
		if m.orchestrator != nil {
			m.orchestrator.ThinkingCallback = func(token string) {
				if prog := m.getProgram(); prog != nil {
					if token == "\x03" {
						prog.Send(ThinkingDoneMsg{})
					} else {
						prog.Send(ThinkingTokenMsg{Content: token})
					}
				}
			}
		}

	// Wire status callback so the TUI can show the single authoritative status line.
	// The closure captures prog to avoid races; sends StatusUpdateMsg on every update.
	if m.orchestrator != nil {
		m.orchestrator.StatusCallback = func(phase string, description string, tokens int, model string) {
			if prog := m.getProgram(); prog != nil {
				prog.Send(StatusUpdateMsg{
					Phase:       phase,
					Description: description,
					Tokens:      tokens,
					Model:       model,
				})
			}
		}
	}
		// Call orchestrator with streaming support
		err := m.orchestrator.ExecuteTaskWithStreaming(ctx, prompt, streamCallback, toolCallback, toolStartCallback, interventionCallback, nil)
		if err != nil {
			return ErrorMsg{Err: err}
		}

		// Commit the final planning segment (the actual answer) to chat.
		// PlanningCommitMsg sets m.generating=false before StreamCompleteMsg arrives,
		// so handleStreamComplete skips the legacy currentResponse commit path.
		if prog := m.getProgram(); prog != nil {
			prog.Send(PlanningCommitMsg{Content: planBuf.String()})
		}

		// Signal stream completion (handles viewport recalc + textarea focus).
		return StreamCompleteMsg{}
	}
}

// RequestPermission requests permission for a tool
func (m *Model) RequestPermission(toolName, description string, params map[string]interface{}) tools.PermissionLevel {
	// We are running on a background goroutine (the tool execution).
	// We MUST communicate with the main TUI loop via messages to update UI state safely.

	respChan := make(chan tools.PermissionLevel, 1)

	if m.program != nil {
		m.program.Send(PermissionRequestMsg{
			ToolName:     toolName,
			Description:  description,
			Params:       params,
			ResponseChan: respChan,
		})
	} else {
		// Fallback if program not set (shouldn't happen in normal run)
		return tools.PermissionNever
	}

	// Wait for user response (blocking this background goroutine)
	level := <-respChan
	return level
}

// ApprovePermission approves the current permission request
func (m *Model) ApprovePermission() {
	if m.awaitingPermission && m.permissionPrompt != nil {
		level := m.permissionPrompt.GetPermissionLevel()
		m.permissionChan <- level
		m.awaitingPermission = false
		m.permissionPrompt = nil
		m.permissionChan = nil
	}
}

// DenyPermission denies the current permission request
func (m *Model) DenyPermission() {
	if m.awaitingPermission {
		m.permissionChan <- tools.PermissionNever
		m.awaitingPermission = false
		m.permissionPrompt = nil
		m.permissionChan = nil
	}
}

// RequestHITLApproval surfaces a SENSE HITL plan to the user and waits for
// their approval decision.  It runs on the tool-execution goroutine and blocks
// until the TUI sends a decision through the response channel.
//
// This function does NOT touch any Model fields directly — all state mutations
// happen on the TUI goroutine via handleHITLRequest. This eliminates the data
// race that the previous implementation had.
func (m *Model) RequestHITLApproval(req engine.HITLRequest) engine.HITLDecision {
	prog := m.getProgram()
	if prog == nil {
		// Non-interactive mode (e.g. --stdin) — auto-approve.
		return engine.HITLDecision{Approval: engine.HITLApproved}
	}
	// Create the response channel locally; pass it through the message so the
	// TUI goroutine owns the write end and we own the read end.
	ch := make(chan engine.HITLDecision, 1)
	prog.Send(HITLRequestMsg{Request: req, ResponseChan: ch})
	return <-ch // block until TUI sends decision
}

// resolveCurrentHITL sends the given decision to the waiting goroutine, pops
// the front of the queue, and either activates the next pending request or
// returns the TUI to chatView. Must be called from the TUI goroutine.
func (m *Model) resolveCurrentHITL(decision engine.HITLDecision) {
	if len(m.hitlQueue) == 0 {
		return
	}
	// Unblock the waiting goroutine.
	item := m.hitlQueue[0]
	m.hitlQueue = m.hitlQueue[1:]
	item.ch <- decision

	m.hitlRequest = nil
	m.hitlChan = nil

	if len(m.hitlQueue) > 0 {
		// Advance to the next queued request.
		next := m.hitlQueue[0]
		m.hitlRequest = &next.req
		m.hitlChan = next.ch
		m.awaitingHITL = true
		m.state = stateHITLApproval
		planDisplay := fmt.Sprintf(
			"## ⚡ SENSE HITL — Approval Required\n\n**Tool:** `%s`\n\n%s",
			next.req.ToolName, next.req.Plan,
		)
		m.addSystemMessage(planDisplay)
		m.updateViewportContent()
		if !m.userScrolledUp {
			m.viewport.GotoBottom()
		}
	} else {
		m.awaitingHITL = false
		m.state = chatView
	}
}

// ApproveHITL approves the current HITL request (optionally with amendment notes).
func (m *Model) ApproveHITL(notes string) {
	approval := engine.HITLApproved
	if notes != "" {
		approval = engine.HITLAmended
	}
	m.resolveCurrentHITL(engine.HITLDecision{Approval: approval, Notes: notes})
}

// RejectHITL rejects the current HITL request.
func (m *Model) RejectHITL(reason string) {
	m.resolveCurrentHITL(engine.HITLDecision{Approval: engine.HITLRejected, Notes: reason})
}

// handleHITLCommand processes /hitl sub-commands for the SENSE HITL approval flow.
//
// Supported forms:
//
//	/hitl approve            — approve with no notes        (HITLApproved)
//	/hitl approve <notes>    — approve with amendment notes (HITLAmended)
//	/hitl reject             — reject with no reason        (HITLRejected)
//	/hitl reject <reason>    — reject with a reason         (HITLRejected)
//	/hitl status             — show whether a decision is pending
func (m *Model) handleHITLCommand(input string) tea.Cmd {
	// Split into at most 3 parts: "/hitl", subcommand, rest-of-line
	parts := strings.SplitN(strings.TrimSpace(input), " ", 3)

	subcommand := ""
	rest := ""
	if len(parts) >= 2 {
		subcommand = strings.ToLower(parts[1])
	}
	if len(parts) >= 3 {
		rest = strings.TrimSpace(parts[2])
	}

	switch subcommand {
	case "approve", "yes", "y":
		if !m.awaitingHITL {
			m.addSystemMessage("_No HITL approval is currently pending._")
			m.updateViewportContent()
			return nil
		}
		label := "approved"
		if rest != "" {
			label = fmt.Sprintf("approved with notes: *%s*", rest)
		}
		m.ApproveHITL(rest) // resolveCurrentHITL handles state/queue cleanup
		m.addSystemMessage(fmt.Sprintf("✅ **HITL: %s** — tool execution will proceed.", label))
		m.updateViewportContent()
		m.textarea.Blur()
		m.textarea.Focus()
		return textarea.Blink

	case "reject", "no", "n", "deny":
		if !m.awaitingHITL {
			m.addSystemMessage("_No HITL approval is currently pending._")
			m.updateViewportContent()
			return nil
		}
		reason := rest
		if reason == "" {
			reason = "user rejected"
		}
		m.RejectHITL(reason) // resolveCurrentHITL handles state/queue cleanup
		m.addSystemMessage(fmt.Sprintf("❌ **HITL: rejected** — *%s*. Tool execution cancelled.", reason))
		m.updateViewportContent()
		m.textarea.Blur()
		m.textarea.Focus()
		return textarea.Blink

	case "status", "":
		if m.awaitingHITL && m.hitlRequest != nil {
			m.addSystemMessage(fmt.Sprintf(
				"⏳ **HITL pending** — awaiting decision for tool `%s`.\n\n"+
					"Type `/hitl approve` or `/hitl reject` to respond.",
				m.hitlRequest.ToolName,
			))
		} else {
			m.addSystemMessage("_No HITL approval is currently pending._")
		}
		m.updateViewportContent()
		return nil

	default:
		m.addSystemMessage(fmt.Sprintf(
			"**Unknown HITL sub-command:** `%s`\n\n"+
				"**Usage:**\n"+
				"- `/hitl approve` — approve the pending action\n"+
				"- `/hitl approve <notes>` — approve with amendment notes\n"+
				"- `/hitl reject` — cancel the pending action\n"+
				"- `/hitl reject <reason>` — cancel with a reason\n"+
				"- `/hitl status` — show current HITL state",
			subcommand,
		))
		m.updateViewportContent()
		return nil
	}
}

// SetProgram sets the program reference for sending messages during streaming
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// getProgram returns the program reference
func (m *Model) getProgram() *tea.Program {
	return m.program
}

// SetModelInfo wires up the live model pool so the /model command can display
// and switch real models. Call this from main.go after dynamic selection.
func (m *Model) SetModelInfo(reg *registry.ModelRegistry, available []commands.ModelInfo, primary, consultant commands.ModelInfo) {
	m.providerRegistry = reg
	m.availableModels = available
	m.commands.SetModelInfo(available, primary, consultant)

	// Populate list
	m.updateModelListItems()
}

// updateToolsTable populates the tools table from the registry
func (m *Model) updateToolsTable() {
	if m.commands == nil || m.orchestrator == nil || m.orchestrator.Registry == nil {
		return
	}

	toolsList := m.orchestrator.Registry.List()
	rows := make([]table.Row, len(toolsList))

	for i, t := range toolsList {
		rows[i] = table.Row{
			t.Name(),
			t.Description(),
			fmt.Sprintf("%v", t.Category()),
		}
	}

	m.toolsTable.table.SetRows(rows)
}

// switchPrimaryModel hot-swaps the orchestrator's primary provider.
func (m *Model) switchPrimaryModel(modelID string) error {
	if m.providerRegistry == nil {
		return fmt.Errorf("provider registry not available")
	}
	modelDef, ok := m.providerRegistry.GetModel(registry.ModelID(modelID))
	if !ok {
		return fmt.Errorf("model not found in registry: %s", modelID)
	}
	providerEntry, ok := m.providerRegistry.GetProvider(modelDef.Provider)
	if !ok {
		return fmt.Errorf("provider not found: %s", modelDef.Provider)
	}
	factory, ok := providerEntry.(ai.AIProvider)
	if !ok {
		return fmt.Errorf("provider does not implement AIProvider interface")
	}
	m.orchestrator.Primary = factory.WithModel(modelID)
	m.currentModel = modelID
	m.commands.UpdateCurrentPrimary(commands.ModelInfo{
		ID:       modelID,
		Name:     modelDef.Name,
		Provider: string(modelDef.Provider),
		Thinking: modelDef.Capabilities.SupportsThinking,
	})
	return nil
}

// switchConsultantModel hot-swaps the orchestrator's consultant provider.
func (m *Model) switchConsultantModel(modelID string) error {
	if m.providerRegistry == nil {
		return fmt.Errorf("provider registry not available")
	}
	modelDef, ok := m.providerRegistry.GetModel(registry.ModelID(modelID))
	if !ok {
		return fmt.Errorf("model not found in registry: %s", modelID)
	}
	providerEntry, ok := m.providerRegistry.GetProvider(modelDef.Provider)
	if !ok {
		return fmt.Errorf("provider not found: %s", modelDef.Provider)
	}
	factory, ok := providerEntry.(ai.AIProvider)
	if !ok {
		return fmt.Errorf("provider does not implement AIProvider interface")
	}
	m.orchestrator.Consultant = factory.WithModel(modelID)
	m.commands.UpdateCurrentConsultant(commands.ModelInfo{
		ID:       modelID,
		Name:     modelDef.Name,
		Provider: string(modelDef.Provider),
		Thinking: modelDef.Capabilities.SupportsThinking,
	})
	return nil
}

// computeSimpleDiff produces a unified-style diff between two strings.
// Returns lines prefixed with '+', '-', or ' ' (context).
func computeSimpleDiff(before, after string) []string {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	// Find common prefix length
	prefixLen := 0
	for prefixLen < len(beforeLines) && prefixLen < len(afterLines) &&
		beforeLines[prefixLen] == afterLines[prefixLen] {
		prefixLen++
	}

	// Find common suffix length
	suffixLen := 0
	maxSuffix := len(beforeLines) - prefixLen
	if s := len(afterLines) - prefixLen; s < maxSuffix {
		maxSuffix = s
	}
	for suffixLen < maxSuffix &&
		beforeLines[len(beforeLines)-1-suffixLen] == afterLines[len(afterLines)-1-suffixLen] {
		suffixLen++
	}

	const ctxLines = 2
	startCtx := prefixLen - ctxLines
	if startCtx < 0 {
		startCtx = 0
	}

	var result []string
	if startCtx > 0 {
		result = append(result, fmt.Sprintf("@@ -%d +%d @@", prefixLen+1, prefixLen+1))
	}
	for i := startCtx; i < prefixLen; i++ {
		result = append(result, " "+beforeLines[i])
	}
	for i := prefixLen; i < len(beforeLines)-suffixLen; i++ {
		result = append(result, "-"+beforeLines[i])
	}
	for i := prefixLen; i < len(afterLines)-suffixLen; i++ {
		result = append(result, "+"+afterLines[i])
	}
	endCtxBefore := len(beforeLines) - suffixLen + ctxLines
	if endCtxBefore > len(beforeLines) {
		endCtxBefore = len(beforeLines)
	}
	for i := len(beforeLines) - suffixLen; i < endCtxBefore; i++ {
		result = append(result, " "+beforeLines[i])
	}
	return result
}
