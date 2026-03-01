# Grokster TUI

A high-fidelity, interactive Text User Interface for Grokster built with the Charm Bracelet stack.

## Architecture

### Components

- **Bubble Tea**: Core TUI framework providing the Elm architecture (Model-View-Update)
- **Lip Gloss**: Styling and layout engine for beautiful terminal UIs
- **Glamour**: Markdown rendering for rich text display
- **Bubbles**: Reusable TUI components (viewport, textarea, spinner)

### File Structure

```
internal/tui/
├── model.go      - Main TUI model and state management
├── update.go     - Update loop handling all events
├── view.go       - View rendering logic
├── style.go      - Lip Gloss styles and themes
├── messages.go   - Custom message types
├── phrases.go    - Loading phrases for personality
└── README.md     - This file
```

## Features

### Command System (`pkg/commands/`)

The TUI includes a comprehensive slash command system:

- `/clear` - Reset conversation context
- `/help` - Display command help
- `/chat` - Manage conversation history (save/load/list/delete)
- `/model` - Switch active AI model
- `/tools` - List active tools and MCPs
- `/auth` - Refresh API credentials
- `/settings` - Open config in editor
- `/version` - Show version info
- `/quit` - Exit gracefully
- `/bug` - Open GitHub issue template
- `/theme` - Toggle light/dark mode
- `/compress` - Compress context to save tokens

### Personality Engine

The TUI masks latency with humor through rotating loading phrases:

**Standard Mode** (Grok):
- "Reticulating splines..."
- "Consulting the oracle..."
- "Warming up the neurons..."
- And 17 more!

**Consultant Mode** (Gemini):
- "Summoning the Architectural Spirits..."
- "Gemini is judging your code..."
- "Deep Deep Thought engaged..."
- And 21 more!

Phrases rotate every 3 seconds during generation to keep the UX engaging.

### Responsive Design

- **Dynamic Sizing**: Viewport and glamour renderer adjust to terminal size
- **Mobile-Friendly**: Code blocks wrap properly on narrow screens
- **Minimum Dimensions**: 60x20 characters ensures usability

### Styling

#### Consultant Box
Gemini responses are rendered in a distinctive purple/pink bordered box to visually distinguish architectural advice from standard Grok responses.

#### Status Bar
Bottom status bar shows:
```
[ Grokster v1.0 ] [ Model: Grok-3 ] [ Consult: Ready ]
```

Status updates based on generation state:
- **Ready**: Consultant available
- **Active**: Gemini is currently responding
- **Standby**: Grok is responding, Gemini on standby

### Keyboard Shortcuts

- `Enter` - Submit prompt
- `Alt+Enter` - Multi-line input (new line)
- `Ctrl+C` / `Ctrl+D` - Quit
- `Esc` - Cancel generation
- `PgUp` / `PgDn` - Scroll viewport
- `Home` / `End` - Jump to top/bottom

## Usage

### Running the TUI

```bash
# Build the TUI
go build -o bin/grokster-tui ./cmd/grokster-tui

# Run it
./bin/grokster-tui
```

### Integration with Orchestrator

To integrate the TUI with the existing orchestrator:

1. **Message Bridge**: Create a channel to receive `TokenMsg` from orchestrator
2. **Streaming**: Orchestrator should emit tokens via `tea.Cmd`
3. **Routing**: Determine `isConsultant` flag based on routing logic
4. **Context**: Pass conversation history to orchestrator

Example integration:

```go
// In update.go
func (m *Model) callOrchestrator(prompt string) tea.Cmd {
    return func() tea.Msg {
        ctx := context.Background()

        // Stream tokens back to TUI
        tokenChan := make(chan string)
        go func() {
            for token := range tokenChan {
                // Send to TUI
                tea.Send(TokenMsg{
                    Content: token,
                    IsConsultant: false,
                    IsFinal: false,
                })
            }
        }()

        // Call orchestrator
        resp, err := orchestrator.ExecuteTask(ctx, prompt)
        if err != nil {
            return ErrorMsg{Err: err}
        }

        // Send final message
        return TokenMsg{
            Content: resp,
            IsConsultant: false,
            IsFinal: true,
        }
    }
}
```

## Themes

The TUI supports two themes:

### Dark Theme (Default)
- Background: `#0A0A0A`
- Text: `#FFFFFF`
- Consultant Box: Purple border (`#9945FF`)
- Primary Accent: Grok Blue (`#00D9FF`)

### Light Theme
- Background: White
- Text: `#000000`
- Consultant Box: Purple border (maintained for consistency)
- Primary Accent: Blue (`#0066CC`)

Switch themes with: `/theme light` or `/theme dark`

## Performance Optimizations

### Efficient Rendering
- **Throttled Updates**: Viewport only updates every 50 tokens (configurable)
- **Incremental Markdown**: Glamour renders delta, not entire history
- **Lazy Scrolling**: Viewport uses virtual scrolling for large conversations

### Token Streaming
- **Non-Blocking**: Token reception doesn't block UI updates
- **Buffered**: Uses `strings.Builder` for efficient string concatenation
- **Batched**: UI updates batched with `tea.Batch()`

## Customization

### Adding New Commands

1. Register in `pkg/commands/registry.go`:
```go
r.commands["mycommand"] = &CommandDefinition{
    Name: "mycommand",
    Description: "Does something cool",
    Usage: "/mycommand [arg]",
    Handler: r.handleMyCommand,
}
```

2. Implement handler:
```go
func (r *Registry) handleMyCommand(args []string) (string, error) {
    return "Result markdown", nil
}
```

### Adding New Loading Phrases

Edit `internal/tui/phrases.go`:
```go
standardPhrases = append(standardPhrases, "Your new phrase...")
consultantPhrases = append(consultantPhrases, "Your Gemini phrase...")
```

### Custom Styles

Modify `internal/tui/style.go`:
```go
// Add new style
s.MyStyle = lipgloss.NewStyle().
    Foreground(lipgloss.Color("#FF0000")).
    Bold(true)
```

## Future Enhancements

- [ ] Syntax highlighting for code blocks
- [ ] Image rendering (via kitty/iterm protocols)
- [ ] Multi-pane layout (chat + docs)
- [ ] Conversation search
- [ ] Export to HTML/PDF
- [ ] Custom keybindings
- [ ] Plugin system for commands
- [ ] Real-time collaboration

## Dependencies

```
github.com/charmbracelet/bubbletea v1.3.10
github.com/charmbracelet/lipgloss v1.1.1
github.com/charmbracelet/glamour v0.10.0
github.com/charmbracelet/bubbles v1.0.0
```

## License

Part of the Grokster project. See main repository LICENSE.
