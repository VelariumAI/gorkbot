# Gorkbot TUI

**Package:** `internal/tui`
**Version:** 3.5.1

The Gorkbot terminal UI is a full-screen, multi-tab interactive interface built on the [Charm Bracelet](https://charm.sh) stack: [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the event loop, [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling, and [Glamour](https://github.com/charmbracelet/glamour) for Markdown rendering.

---

## Architecture

The TUI follows the [Elm Architecture](https://guide.elm-lang.org/architecture/) pattern via Bubble Tea:

```
Model (state) → View (render) → Update (handle events)
     ↑                                    │
     └────────────────────────────────────┘
                   Msg (events)
```

### Key Files

| File | Responsibility |
|------|---------------|
| `model.go` | `Model` struct definition; all state fields; `InitialModel()` constructor |
| `update.go` | `Update(msg)` — handles all `tea.Msg` events and returns (model, cmd) |
| `view.go` | `View()` — renders the full TUI from current model state |
| `keys.go` | `KeyMap` struct and `DefaultKeyMap()` — all keybindings |
| `statusbar.go` | Status bar rendering (context%, cost, mode, git branch) |
| `messages.go` | All custom `tea.Msg` types used for async communication |
| `model_extensions.go` | Extensions: `SetDiscoveryManager()`, `SetExecutionMode()`, `UpdateContextStats()` |
| `model_select_view.go` | Dual-pane model selection UI |
| `api_key_prompt.go` | Modal overlay for API key entry |
| `settings_overlay.go` | 4-tab settings modal (Model Routing, Verbosity, Tool Groups, API Providers) |
| `discovery_view.go` | Cloud Brains tab: discovered models + agent tree |
| `list.go` | Model list items for the model selection panes |

---

## State Machine

### Session States

```go
type sessionState int

const (
    chatView         sessionState = iota // default: conversation view
    modelSelectView                       // Ctrl+T: model selection
    toolsTableView                        // Ctrl+E: tool registry table
    discoveryView                         // Ctrl+D: Cloud Brains
    analyticsView                         // Ctrl+A: session analytics
    diagnosticsView                       // Ctrl+\: system diagnostics
)
```

State transitions are handled in `Update()` on key events. Only one state is active at a time; `Esc` always returns to `chatView`.

### Model Struct (selected fields)

```go
type Model struct {
    // Core
    keys        KeyMap
    styles      Styles
    width       int
    height      int
    state       sessionState

    // Chat
    messages    []ChatMessage      // rendered conversation items
    viewport    viewport.Model     // scrollable message area
    textarea    textarea.Model     // multi-line input
    spinner     spinner.Model      // in-progress indicator

    // Orchestrator interface
    cmdReg      *commands.Registry    // slash command dispatcher
    orch        OrchestratorRef       // orchestrator function refs

    // Status bar
    statusBar   StatusBar             // context%, cost, mode, git branch
    gitBranch   string                // from git rev-parse on startup

    // Overlays
    modelSelectState  ModelSelectState   // dual-pane model selection state
    apiKeyPromptState APIKeyPromptState  // key entry modal state

    // Settings
    settingsTab     int                 // active settings tab (0-3)
    settingsActive  bool

    // Bookmarks
    bookmarks   []ConversationBookmark
    bookmarksOpen bool

    // Discovery
    discoveryMgr  interface{}         // *discovery.Manager (optional)
    agentTree     []discoveryModel

    // Analytics
    analytics   *AnalyticsData

    // Execution modes
    execMode    ExecutionMode       // Normal | Plan | Auto
}
```

---

## Message Types

All async operations communicate back to the `Update()` loop via typed messages (`pkg/tui/messages.go`):

| Message | Source | Description |
|---------|--------|-------------|
| `StreamChunkMsg` | Orchestrator goroutine | Incremental AI token for streaming display |
| `StreamCompleteMsg` | Orchestrator goroutine | AI turn completed; includes full response |
| `ToolStartMsg` | Orchestrator goroutine | Tool execution beginning |
| `ToolDoneMsg` | Orchestrator goroutine | Tool execution completed |
| `ToolProgressMsg` | Orchestrator goroutine | Tool progress update |
| `PermissionRequestMsg` | Tool executor | Permission prompt data |
| `PermissionResponseMsg` | Permission UI | User's permission decision |
| `ModeChangeMsg` | Orchestrator / command | Execution mode changed |
| `ContextUpdateMsg` | Context manager | Updated token/cost stats |
| `InterruptMsg` | Ctrl+X handler | Cancel in-progress generation |
| `RewindCompleteMsg` | Checkpoint manager | Rewind completed |
| `DiscoveryUpdateMsg` | Discovery manager | New model list from provider |
| `DiscoveryPollTickMsg` | Ticker goroutine | Trigger next discovery poll |
| `ModelRefreshMsg` | Model selection UI | Refresh model list |
| `ModelSwitchedMsg` | Model selection UI | Model was changed by user |
| `APIKeySavedMsg` | API key prompt | Key saved to KeyStore |
| `ProviderStatusMsg` | Key status checker | Provider key validation result |

---

## Streaming Display

AI responses are streamed token-by-token:

1. Orchestrator goroutine sends `StreamChunkMsg{Chunk: "next token"}` for each token.
2. `Update()` appends the chunk to the in-progress `assistantMsg`.
3. `View()` renders the partial message with a trailing cursor indicator.
4. When streaming completes, `StreamCompleteMsg` is sent.
5. `handleStreamComplete()` finalizes the message, triggers a viewport scroll, and renders the full Glamour-rendered Markdown.

**Critical invariant:** `handleStreamComplete()` must not be modified without understanding its role in finalizing the conversation history and triggering the next tool-call loop.

---

## Touch Scroll (Android)

Touch-scroll is implemented in `Update()` via `tea.MouseMsg`:

```go
case tea.MouseMsg:
    switch msg.Type {
    case tea.MouseWheelUp:
        m.viewport.LineUp(3)
    case tea.MouseWheelDown:
        m.viewport.LineDown(3)
    }
```

This block handles native touch gestures on Android/Termux. **Do not modify this block** without testing on-device — it is the primary scroll mechanism on mobile.

---

## Model Selection (Ctrl+T)

The model selection UI (`model_select_view.go`) manages a dual-pane interface:

```go
type ModelSelectState struct {
    activePane      int                  // 0=primary, 1=specialist
    primaryList     []modelItem          // primary models
    secondaryList   []modelItem          // specialist models
    providerFilter  string               // "all" | provider ID
    providerKeys    map[string]string    // masked key statuses
    refreshing      bool                 // model list refresh in progress
}
```

Model list items include `isAuto bool` (for the [Auto] specialist option) and `active bool` (marks currently selected model).

When a model is selected, `Update()` signals the orchestrator via `OrchestratorAdapter.SetPrimary()` or `SetSecondary()`, which hot-swaps the provider without restarting.

---

## API Key Prompt

`api_key_prompt.go` renders a modal overlay for key entry:

```go
type APIKeyPromptState struct {
    active     bool
    provider   string     // "xai" | "google" | "anthropic" | ...
    inputVal   string
    websiteURL string     // shown for guidance
    errMsg     string     // validation failure message
}
```

On `Enter`, the key is passed to `OrchestratorAdapter.SetProviderKey()`, which saves it to the KeyStore and attempts validation.

---

## Settings Overlay

`settings_overlay.go` renders a 4-tab modal (`Ctrl+G`):

```go
const (
    tabModelRouting  = 0
    tabVerbosity     = 1
    tabToolGroups    = 2
    tabProviders     = 3  // API provider enable/disable
)

var tabLabels = []string{
    "Model Routing",
    "Verbosity",
    "Tool Groups",
    "API Providers",
}
```

**Tab 3 — API Providers** (`tabProviders`): Shows per-provider toggles. Toggling a provider calls `pm.DisableForSession(id)` / `pm.EnableForSession(id)` and persists the state to `app_state.json` via `appState.SetDisabledProviders()`.

---

## Status Bar

`statusbar.go` implements `StatusBar`:

```go
type StatusBar struct {
    contextPct  float64    // 0.0–1.0
    costUSD     float64
    mode        string     // "Normal" | "Plan" | "Auto"
    gitBranch   string
}
```

The git branch is captured once at startup in `model.go`:

```go
func currentGitBranch() string {
    out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(out))
}
```

`SetGitBranch()` on `StatusBar` stores the result for rendering.

---

## OrchestratorAdapter

The TUI cannot import `internal/engine` (import cycle prevention). Instead, `cmd/gorkbot/main.go` builds a `commands.OrchestratorAdapter` — a struct of function references — and passes it into the TUI via `model.SetCommandRegistry()`.

All TUI-to-orchestrator communication goes through this adapter:

```go
type OrchestratorAdapter struct {
    GetContextReport    func() string
    GetCostReport       func() string
    SetPrimary          func(ctx, provider, model string) error
    SetSecondary        func(ctx, provider, model string) error
    SetProviderKey      func(ctx, provider, key string) error
    GetProviderStatus   func() string
    SetAutoSecondary    func(bool)
    RewindTo            func(id string) (string, error)
    ExportConv          func(format, path string) error
    ToggleDebug         func()
    // ... 20+ total function refs
}
```

This pattern keeps `internal/tui` free of direct dependencies on the engine package.

---

## Keybindings

All keybindings are defined in `DefaultKeyMap()` in `keys.go`. To add a new keybinding:

1. Add a `key.Binding` field to `KeyMap`
2. Initialize it in `DefaultKeyMap()` with `key.NewBinding(...)`
3. Handle it in `Update()` with `key.Matches(msg, m.keys.YourNewKey)`

Do not hardcode key strings anywhere else — always use the `KeyMap` binding.

---

## Rendering Pipeline

`View()` is called on every state change and must be fast (it runs on every event):

```go
func (m Model) View() string {
    if m.width == 0 {
        return "Loading..."
    }

    // Route to active tab view
    switch m.state {
    case modelSelectView:
        return renderModelSelectView(m)
    case toolsTableView:
        return m.toolsTable.View()
    case discoveryView:
        return renderDiscoveryView(m)
    case analyticsView:
        return renderAnalyticsView(m)
    case diagnosticsView:
        return renderDiagnosticsView(m)
    }

    // Default: chat view
    var sections []string
    sections = append(sections, m.renderTabs())
    sections = append(sections, m.viewport.View())
    sections = append(sections, m.renderInputArea())
    sections = append(sections, m.statusBar.View())

    content := lipgloss.JoinVertical(lipgloss.Left, sections...)

    // Overlay rendering (settings, bookmarks, permissions, api key prompt)
    if m.settingsActive {
        content = renderSettingsOverlay(m, content)
    }
    if m.apiKeyPromptState.active {
        content = renderAPIKeyPrompt(m, content)
    }
    // ... other overlays

    return content
}
```

---

## Building

```bash
# Standard build (includes TUI)
go build -o bin/gorkbot ./cmd/gorkbot/

# Run
./bin/gorkbot
# Or with .env loading:
./gorkbot.sh
```

The TUI package is `internal/` — it is not intended for import by external packages.
