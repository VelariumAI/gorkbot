package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/velariumai/gorkbot/internal/arc"
	"github.com/velariumai/gorkbot/internal/engine"
	        "github.com/velariumai/gorkbot/internal/platform"
	        "github.com/velariumai/gorkbot/pkg/ai"
	                        "github.com/velariumai/gorkbot/pkg/commands"
	                "github.com/velariumai/gorkbot/pkg/process"
	"github.com/velariumai/gorkbot/pkg/registry"
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
	chatView sessionState = iota
	modelListView           // kept for backward compat
	toolsTableView
	discoveryView
	analyticsView    // session analytics dashboard (Ctrl+A)
	diagnosticsView  // system diagnostics (Ctrl+\)
	stateHITLApproval // SENSE HITL plan-and-execute approval overlay
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
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	consultantSpinner spinner.Model
	help     help.Model
	keymap   KeyMap
	
	// Model Selection List
	modelList       list.Model
	availableModels []commands.ModelInfo
	state           sessionState

	// Tools Table
	toolsTable      TableModel

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
	ready            bool
	width            int
	height           int
	generating       bool
	isConsultant     bool
	mouseEnabled     bool
	currentPhrase    string
	err              error

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

	// Content
	messages         []Message
	currentResponse  strings.Builder
	currentModel     string
	theme            string

	// Scroll state - track if user is viewing older messages
	userScrolledUp   bool  // true when user has scrolled up to read older content

	// Live tool execution panel — names of tools currently running.
	activeTools []string

	// Discovery: live cloud model sidebar + agent tree.
	discoveredModels []discoveryModel // latest snapshot from discovery manager
	discoverySub     chan []discoveryModel // receives updates from discovery bridge goroutine

	// Markdown renderer
	glamour          *glamour.TermRenderer

	// Styles
	styles           *Styles

	// Quit flag
	quitting         bool

	// Program reference for sending messages
	program          *tea.Program

	// Provider registry — used by /model to instantiate providers via WithModel.
	providerRegistry *registry.ModelRegistry

	currentColorIdx  int

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
	toasts []activeToast

	// ── Side panel ────────────────────────────────────────────────────────
	sidePanelOpen  bool
	sidePanelWidth int

	// ── ARC intent badge for latest user message ──────────────────────────
	lastIntentCategory string

	// ── Tool timing: ToolName → start time (for elapsed display) ──────────
	toolStartTimes map[string]time.Time

	// ── Dual-pane model selection (modelSelectView) ───────────────────────
	modelSelect  modelSelectState
	apiKeyPrompt apiKeyPromptState

	// ── Cloud Brains interactive cursor (discoveryView) ───────────────────
	discCursor        int    // index into discoveredModels list
	discTestActive    bool   // test-prompt input is open
	discTestInput     string // accumulated test prompt text
	discTestResult    string // result of test prompt

	// ── Conversation bookmarks ────────────────────────────────────────────
	bookmarks            []Bookmark   // in-memory bookmark list
	bookmarkOverlay      bool         // bookmark manager is open
	bookmarkInput        string       // new bookmark name input
	bookmarkInputActive  bool         // creating new bookmark
}

// activeToast holds one pending toast notification.
type activeToast struct {
	Icon      string
	Text      string
	Color     string
	ExpiresAt time.Time
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
	Role         string // "user", "assistant", "consultant", "system", "tool", "tool_call", "internal", "a2a"
	Content      string
	IsConsultant bool
	ToolName     string             // For tool messages
	ToolResult   *tools.ToolResult  // For tool result messages
	ToolParams   map[string]interface{} // For tool_call (request) messages
	NestLevel    int                // Nesting depth for tool calls (0 = top level)
	MessageType  string             // "tool", "tool_call", "internal", "a2a", "normal"
	Collapsed    bool               // true = show 1-line summary; toggled with Ctrl+R
	Elapsed        time.Duration    // tool execution duration (for display)
	IntentCategory string           // ARC category at time of user message
}

// modelSelectState holds the dual-pane model selection view state.
type modelSelectState struct {
	activePane     int          // 0 = primary, 1 = secondary
	primaryList    list.Model
	secondaryList  list.Model
	providerFilter string       // "" = all; otherwise filter by provider ID
	providerKeys   []providerStatus // latest provider key statuses
	refreshing     map[string]bool
}

// apiKeyPromptState holds the API key entry modal state.
type apiKeyPromptState struct {
	active     bool
	provider   string
	inputVal   string
	validating bool   // true while background Ping is in flight
	errMsg     string
	websiteURL string
}

// NewModel creates a new TUI model.
// modelName is the display name of the primary AI (e.g. "Grok-3").
// consultantName is the display name of the consultant AI (e.g. "Gemini 2.0 Flash"); pass "" if unavailable.
func NewModel(orch *engine.Orchestrator, pm *process.Manager, modelName, consultantName string) (*Model, error) {
	// Initialize components
	ta := textarea.New()
	ta.Placeholder = "Ask me anything... (Tap here to type, Enter to send)"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(true) // Alt+Enter for newline
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // Keep cursor visible

	sp := spinner.New()
	sp.Spinner = BlockGSpinner()
	// Use GrokBlue for the spinner
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue))

	csp := spinner.New()
	csp.Spinner = ConsultantSpinner()
	csp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Blood Red

	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true  // Enable mouse wheel scrolling
	vp.MouseWheelDelta = 3       // Scroll 3 lines per wheel tick

	// Initialize glamour for markdown rendering
	r, _ := glamour.NewTermRenderer(
		glamour.WithStyles(CustomGlamourStyle()),
		glamour.WithWordWrap(80),
	)

	// Initialize command registry
	registry := commands.NewRegistry()

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

	styles := NewStyles()
	// Apply Dracula theme immediately
	styles.UpdateForDraculaTheme()

	m := &Model{
		viewport:        vp,
		textarea:        ta,
		spinner:         sp,
		consultantSpinner: csp,
		help:            h,
		keymap:          DefaultKeyMap(),
		modelList:       l,
		toolsTable:      tt,
		state:           chatView,
		commands:        registry,
		orchestrator:    orch,
		processManager:  pm,
		statusBar:       NewStatusBar(styles),
		ready:           false,
		generating:      false,
		isConsultant:    false,
		messages:        []Message{},
		currentModel:    modelName,
		theme:           "dracula", // Default to Dracula
		glamour:         r,
		styles:          styles,
		currentColorIdx:     0,
		rendererWidth:       80,
		streamChunkInterval: 8,
		toolStartTimes:      make(map[string]time.Time),
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

	// Build welcome message with accurate system inventory.
	consultantLine := ""
	if consultantName != "" {
		consultantLine = fmt.Sprintf(" · **%s** (Consultant)", consultantName)
	}
	m.addSystemMessage(fmt.Sprintf(
		"# Gorkbot v%s\n\n"+
			"**%s**%s\n\n"+
			"---\n\n"+
			"**Intelligence**\n"+
			"- **ARC Router** — classifies every prompt (Direct vs ReasonVerify) and scales tool budget to platform RAM\n"+
			"- **MEL** — Meta-Experience Learning: turns tool failure→correction cycles into persistent guardrail heuristics\n\n"+
			"**Parametric Memory** *(cross-session, query-relevant, refreshed every turn)*\n"+
			"- **AgeMem STM/LTM** — two-tier memory; hot facts in-session, cold facts survive restarts\n"+
			"- **Engrams** — explicit tool/behaviour preferences written by `record_engram`, persisted to LTM\n"+
			"- **MEL VectorStore** — heuristic store with Jaccard retrieval and confidence weighting\n\n"+
			"**SENSE**\n"+
			"- **LIE** (reasoning depth) · **Stabilizer** (quality guard) · **Code2World** (action preview)\n\n"+
			"**Discovery** — live model polling (xAI + Gemini) · **Cloud Brains** tab `Ctrl+D`\n\n"+
			"---\n\n"+
			"Type `/help` for commands · `/tools` to browse tools · `/mode` to switch execution mode\n\n"+
			"*Independent project — Todd Eddings / Velarium AI — OpenAI-compatible API*",
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
		discoveryPollTick(),           // start cloud-brains polling ticker
		glistenTick(),                 // start header animation
		m.pollAllConfiguredProviders(), // populate model lists from all keyed providers on startup
		providerPollTick(),             // schedule periodic re-poll every 5 minutes
	)
}

// Helper methods

func (m *Model) addSystemMessage(content string) {
	m.messages = append(m.messages, Message{
		Role:    "system",
		Content: content,
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

	m.messages = append(m.messages, Message{
		Role:        "tool",
		Content:     content,
		ToolName:    toolName,
		ToolResult:  result,
		NestLevel:   level,
		MessageType: "tool",
		Elapsed:     elapsed,
	})
}

// addToolCallMessage inserts a tool_call (request) message that renders as a
// cyan-bordered box showing the tool name and its parameters before the result.
func (m *Model) addToolCallMessage(toolName string, params map[string]interface{}) {
	m.messages = append(m.messages, Message{
		Role:        "tool_call",
		ToolName:    toolName,
		ToolParams:  params,
		MessageType: "tool_call",
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
func (m *Model) renderMessages() string {
	var output strings.Builder

	for i, msg := range m.messages {
		// Add nesting prefix for tool/internal/a2a messages
		prefix := m.getNestingPrefix(msg.NestLevel, msg.MessageType)

		switch msg.Role {
		case "user":
			userLine := m.styles.UserMessage.Render(fmt.Sprintf("You: %s", msg.Content))
			if msg.IntentCategory != "" {
				label := arc.CategoryLabel(arc.IntentCategory(msg.IntentCategory))
				emoji := arc.CategoryEmoji(arc.IntentCategory(msg.IntentCategory))
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
			// Clean up the content before rendering to avoid excessive indentation
			cleanContent := m.cleanMarkdownContent(msg.Content)
			rendered, err := m.glamour.Render(cleanContent)
			if err != nil {
				output.WriteString(m.styles.AIMessage.Render(cleanContent))
			} else {
				// Trim excessive leading spaces from rendered output
				rendered = m.trimRenderedIndentation(rendered)
				output.WriteString(m.styles.AIMessage.Render(rendered))
			}
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

			boxWidth := m.width - msg.NestLevel*2 - 4
			if boxWidth < 20 {
				boxWidth = 20
			}
			var boxContent string
			if body != "" {
				boxContent = lipgloss.JoinVertical(lipgloss.Left, header, body)
			} else {
				boxContent = header
			}
			callBoxStyle := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color(DraculaCyan)).
				Padding(0, 1).
				Width(boxWidth)
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

			titleParts := "⚙ " + nameStyle.Render(msg.ToolName)
			if msg.Elapsed > 0 {
				titleParts += fmt.Sprintf("  ·  %.2fs", msg.Elapsed.Seconds())
			}
			titleParts += "  " + statusIcon

			// Check for diff data (before/after from file write/edit operations)
			var outputContent string
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
					if limit > maxDiffLines && !msg.Collapsed {
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
						rendered = append(rendered, ctxStyle.Render(fmt.Sprintf("▶ %d more diff lines — Ctrl+R", truncated)))
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
				lines := strings.Split(rawOutput, "\n")
				const maxToolLines = 15
				var displayLines []string
				if len(lines) > maxToolLines && !msg.Collapsed {
					displayLines = lines[:maxToolLines]
					displayLines = append(displayLines,
						fmt.Sprintf("[↓ %d more lines — Ctrl+R]", len(lines)-maxToolLines))
				} else {
					displayLines = lines
				}
				outputContent = strings.Join(displayLines, "\n")
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
			if msg.Collapsed {
				hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
				n := len([]rune(msg.Content))
				line := fmt.Sprintf("%s💭 ▶ reasoning · %d chars  %s", prefix, n, hintStyle.Render("ctrl+r"))
				output.WriteString(style.Render(line))
			} else {
				content := prefix + msg.Content
				output.WriteString(style.Render(content))
			}
			output.WriteString("\n")

		case "a2a":
			style := m.getNestedStyle(msg.NestLevel, "a2a")
			if msg.Collapsed {
				hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
				n := len([]rune(msg.Content))
				line := fmt.Sprintf("%s🔄 ▶ agent·comm · %d chars  %s", prefix, n, hintStyle.Render("ctrl+r"))
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

	return rendered
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
	m.viewport.SetContent(content)

	// Auto-scroll to bottom unless the user has manually scrolled up.
	// When content fits entirely in the viewport the GotoBottom call is a no-op
	// so we always keep the newest messages anchored at the bottom.
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
		theme := strings.TrimPrefix(result, "THEME:")
		m.theme = theme
		if theme == "light" {
			m.styles.UpdateForLightTheme()
		} else {
			m.styles.UpdateForDarkTheme()
		}

		// Update markdown renderer style
		style := "dark"
		if theme == "light" {
			style = "light"
		}
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(style),
			glamour.WithWordWrap(m.width-10),
		)
		if err == nil {
			m.glamour = renderer
		}

		m.addSystemMessage(fmt.Sprintf("Switched to **%s** theme", theme))
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
		m.activeOverlay = NewSettingsOverlay(m.width, m.height, m.commands.Orch, toolReg, appStateSetter, debugOn, providerSetter)
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

// cleanMarkdownContent removes excessive indentation from markdown content
func (m *Model) cleanMarkdownContent(content string) string {
	lines := strings.Split(content, "\n")
	var cleaned []string

	for _, line := range lines {
		// Don't modify code blocks
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			cleaned = append(cleaned, line)
			continue
		}

		// For regular lines, normalize excessive leading spaces (but keep intentional indentation)
		// If a line has more than 6 leading spaces, it's likely a rendering artifact
		trimmed := strings.TrimLeft(line, " ")
		leadingSpaces := len(line) - len(trimmed)

		if leadingSpaces > 6 {
			// Reduce to max 2 spaces for normal indentation
			cleaned = append(cleaned, "  "+trimmed)
		} else {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, "\n")
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
			if (s[i] >= '0' && s[i] <= '9') {
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
	if cat := arc.ClassifyIntent(input); cat != arc.CategoryAuto {
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

	// Call orchestrator
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

		// Stream callback for real-time token updates
		// We implement a simple filter to prevent raw JSON tool calls from leaking into the UI.
		// Since we handle tool executions explicitly via toolCallback, we don't want the raw JSON request.
		inJSONBlock := false

		streamCallback := func(token string) {
			// 1. If we are inside a tool call block, drop tokens.
			// Check for the closing marker so we exit the blocked state after it.
			if inJSONBlock {
				if strings.Contains(token, "[/TOOL_CALL]") {
					inJSONBlock = false
				}
				return
			}

			// 2. Heuristic detection: enter blocked state on known tool call markers.
			// Covers native xAI format ("tool_calls"/"function") and the arrow-syntax
			// [TOOL_CALL] format emitted by some other AI models.
			if strings.Contains(token, "\"tool_calls\"") ||
				strings.Contains(token, "\"function\"") ||
				strings.Contains(token, "[TOOL_CALL]") {
				inJSONBlock = true
				return
			}

			// 3. Edge case: "{" at start of line might be start of JSON.
			// We buffer slightly to check, but for responsiveness we generally pass through.
			// If we accumulated a buffer and it turns out to be text, we'd release it.
			// For this fix, we focus on the most common leak pattern which contains the keys.
			
			// Check against buffer if we were suspicious (omitted for simplicity/stability, relying on key detection)

			if prog := m.getProgram(); prog != nil {
				prog.Send(TokenMsg{
					Content:      token,
					IsConsultant: isConsult,
					IsFinal:      false,
				})
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
		// Sends both a ToolCallMsg (request box) and a ToolProgressMsg (live panel).
		toolStartCallback := func(toolName string, params map[string]interface{}) {
			if prog := m.getProgram(); prog != nil {
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

		// Call orchestrator with streaming support
		err := m.orchestrator.ExecuteTaskWithStreaming(ctx, prompt, streamCallback, toolCallback, toolStartCallback, interventionCallback)
		if err != nil {
			return ErrorMsg{Err: err}
		}

		// Signal stream completion
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
