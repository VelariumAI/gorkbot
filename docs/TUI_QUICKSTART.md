# Grokster TUI Quick Start

## ✨ What Changed

The old REPL has been replaced with a **beautiful, modern TUI** built with Charm Bracelet tools!

## 🚀 Running Grokster

```bash
# Interactive TUI mode (new default!)
./grokster.sh

# One-shot mode (unchanged)
./grokster.sh -p "What is the meaning of life?"

# OAuth commands
./grokster.sh login
./grokster.sh status
./grokster.sh logout
```

## 🎮 Controls

### Basic Input
- **Type your question** and press **Enter** to submit
- **Alt+Enter** - Add a new line (multi-line input)
- **Ctrl+C** or **Ctrl+D** - Quit

### Navigation
- **PgUp** / **PgDn** - Scroll through conversation history
- **Home** - Jump to top
- **End** - Jump to bottom
- **Esc** - Cancel ongoing AI generation

## 💻 Slash Commands

Type any of these commands in the TUI:

| Command | Description | Example |
|---------|-------------|---------|
| `/help` | Show all commands | `/help` |
| `/clear` | Clear conversation | `/clear` |
| `/model` | Switch AI model | `/model gemini-flash` |
| `/theme` | Change theme | `/theme light` |
| `/version` | Show version info | `/version` |
| `/quit` | Exit application | `/quit` |
| `/chat save <name>` | Save conversation | `/chat save my-session` |
| `/chat load <name>` | Load conversation | `/chat load my-session` |
| `/compress` | Compress context | `/compress` |
| `/auth status` | Check auth | `/auth status` |

## 🎨 Visual Features

### Consultant Mode
When Gemini is consulted (complex queries), responses appear in a **purple/pink bordered box**:

```
╭──────────────────────────────────────────╮
│ 🔮 Consultant (Gemini) Response         │
│                                          │
│ Architectural advice appears here...    │
╰──────────────────────────────────────────╯
```

### Status Bar
Bottom status bar shows current state:
```
[ Grokster v1.0 ] [ Model: Grok-3 ] [ Consult: Ready ]
```

- **Ready** - Consultant available
- **Active** - Gemini is responding
- **Standby** - Grok is responding

### Loading Phrases
During generation, you'll see rotating humorous phrases:

**Standard (Grok):**
- "Reticulating splines..."
- "Warming up the neurons..."
- "Shuffling bits..."

**Consultant (Gemini):**
- "Summoning the Architectural Spirits..."
- "Gemini is judging your code..."
- "Consulting the Sacred Texts (RFC specs)..."

## 🔧 How It Works

### Orchestrator Integration

The TUI is fully integrated with the orchestrator:

1. **You type a prompt** → TUI adds to history
2. **TUI calls orchestrator** → Routing logic determines if consultant is needed
3. **If complex** → Gemini provides advice (shown in purple box)
4. **Grok generates response** → Shown in standard format
5. **Markdown rendering** → Code blocks, lists, tables all formatted beautifully

### Complexity Triggers

Gemini consultation is triggered when:
- Prompt contains "COMPLEX" or "REFRESH" keywords
- Prompt is longer than 1000 characters
- (More triggers can be added in orchestrator)

## 📊 Example Session

```
You: What is dependency injection?

Grok-3: Dependency injection is a design pattern...
[Standard response]

---

You: COMPLEX: Design a microservices architecture for e-commerce

╭──────────────────────────────────────────╮
│ 🔮 Consultant (Gemini)                  │
│                                          │
│ Consider these architectural patterns:  │
│ - API Gateway for routing              │
│ - Event-driven communication           │
│ - CQRS for order processing            │
╰──────────────────────────────────────────╯

Grok-3: Based on Gemini's advice, here's
a comprehensive architecture...
```

## 🎯 Tips

1. **Multi-line prompts**: Use Alt+Enter to format complex questions
2. **Scroll back**: PgUp/PgDn to review conversation history
3. **Save important chats**: `/chat save architecture-discussion`
4. **Switch themes**: `/theme light` for bright environments
5. **Cancel anytime**: Press Esc if AI is taking too long

## 🐛 Troubleshooting

### TUI doesn't start
```bash
# Check if binary was built
make build

# Ensure dependencies are installed
go mod tidy
```

### API Keys not working
```bash
# Check authentication status
./grokster.sh status

# Try OAuth login
./grokster.sh login
```

### Display issues
```bash
# Try different theme
/theme dark  # or /theme light

# Resize terminal (minimum 60x20)
```

## 🔮 What's Next?

The TUI currently:
- ✅ Displays responses from orchestrator
- ✅ Shows consultant (Gemini) responses in special box
- ✅ Supports all slash commands
- ✅ Handles keyboard shortcuts
- ✅ Renders beautiful markdown

Future enhancements:
- [ ] Real-time token streaming (currently shows full response)
- [ ] Syntax highlighting for code blocks
- [ ] Image display support
- [ ] Conversation search
- [ ] Export to HTML/PDF

## 📚 More Info

- Full TUI documentation: `internal/tui/README.md`
- Command details: See `/help` in the TUI
- OAuth setup: Run `./grokster.sh login`

Enjoy the new Grokster experience! 🚀
