# Gorkbot 🚀

**Gorkbot** is an **AI-powered orchestration platform** that unifies multiple large language models (LLMs) into a single, intelligent terminal interface. It integrates Grok (xAI), Gemini (Google), Claude (Anthropic), and other OpenAI-compatible providers into a sophisticated reasoning engine with real-time streaming, autonomous tool execution, cross-session memory, and human-in-the-loop approval gates.

**Public Version:** 1.6.1-rc | **Internal Version:** 6.2.0 (Development)
**Build:** Go 1.25.0 | **Platform:** Linux, macOS, Windows, Android/Termux
**License:** MIT

---

## 🎯 What is Gorkbot?

Gorkbot is a conversational AI system that goes far beyond simple LLM wrappers. It implements:

- **Orchestrator Pattern**: Coordinates primary and consultant AI providers with automatic routing
- **Adaptive Routing**: Uses ARC (Adaptive Response Classification) + CCI (Codified Context Infrastructure) + MEL (Meta-Experience Learning) for intelligent model selection
- **SENSE Awareness Layer**: Input sanitization, quality criticism, compression, memory management, episodic storage
- **Comprehensive Tool System**: 75+ integrated tools for file operations, git, web, security, system management, and more
- **Extended Thinking Support**: Native support for models like Grok-3 and Claude with reasoning token budgets
- **Professional TUI**: Beautiful Bubble Tea-based terminal interface with markdown rendering, streaming output, and full keyboard control
- **Session Management**: Checkpoints, export, relay, observer mode, multi-user collaboration
- **Continual Learning**: XSKILL framework for evolving agent skills from execution traces
- **Security**: Fine-grained permission system, encrypted API key storage, audit logging, input sanitization

---

## ⚡ Quick Start (2 Minutes)

### 1. Clone the Repository
```bash
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
```

### 2. Run Guided Setup (Recommended)
```bash
make setup
```

This single command guides dependency checks, API key setup, optional local LLM bridge bootstrap, optional semantic model download, build/install, and post-install validation.

### 3. Fast Non-Interactive Setup (Optional)
```bash
make setup-auto
```

### 4. Manual Build (Advanced)
```bash
make build
./bin/gorkbot --version
```

The setup flow supports API keys from:
- **xAI Grok**: https://console.x.ai/
- **Google Gemini**: https://aistudio.google.com/apikey
- **Anthropic Claude**: https://console.anthropic.com/ (optional)
- **OpenAI**: https://platform.openai.com/api-keys (optional)

### 5. Start Chatting
```bash
# Interactive TUI mode
./bin/gorkbot

# Or one-shot mode
./bin/gorkbot -p "What is quantum computing?"
```

---

## 📚 Documentation

### User Guides
- **[GETTING_STARTED.md](GETTING_STARTED.md)** - Setup and basic usage
- **[VERSIONING.md](VERSIONING.md)** - Version information and release notes
- **[docs/RELEASE_OPERATIONS.md](docs/RELEASE_OPERATIONS.md)** - Tagging and release workflow

### Developer Resources
- **[CONTRIBUTING.md](CONTRIBUTING.md)** - Repository contribution and engineering standards
- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** - Architecture overview
- **[docs/TOOL_SYSTEM_DESIGN.md](docs/TOOL_SYSTEM_DESIGN.md)** - Tool system architecture
- **[docs/TUI_QUICKSTART.md](docs/TUI_QUICKSTART.md)** - Terminal UI development
- **[docs/PERMISSIONS_GUIDE.md](docs/PERMISSIONS_GUIDE.md)** - Tool permission system
- **[docs/CONTEXT_CONTINUITY.md](docs/CONTEXT_CONTINUITY.md)** - Context management

### Integration Guides
- **[docs/TOOL_INTEGRATION.md](docs/TOOL_INTEGRATION.md)** - Adding new tools
- **[docs/OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** - OAuth integration
- **[docs/SECURITY.md](docs/SECURITY.md)** - Security best practices

### Troubleshooting
- **[docs/troubleshooting/BUG_FIXES.md](docs/troubleshooting/BUG_FIXES.md)** - Known issues and fixes
- **[docs/troubleshooting/FIXES_V2.md](docs/troubleshooting/FIXES_V2.md)** - V2 fixes and improvements

---

## 🏗️ Architecture Overview

### Multi-Provider Intelligence Layer

Gorkbot abstracts away provider-specific implementations and presents a unified interface:

```
User Input
    ↓
┌─────────────────────────────────────┐
│  Prompt Builder + SENSE Injections  │
│  (Context, Compression, Heuristics) │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│  ARC Router (Intelligent Routing)   │
│  - Classification: Direct/Analytical│
│  - Budget: Token/Temp/Tool limits   │
│  - Consistency: Destructive ops OK? │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│  Provider Selection                 │
│  - Primary (Grok, Claude, etc)      │
│  - Consultant (if triggered)        │
│  - Fallback chain if needed         │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│  AI Generation                      │
│  - Native function calling          │
│  - Extended thinking support        │
│  - Token streaming                  │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│  Tool Execution Loop                │
│  - Parallel tool execution (8 max)  │
│  - Permission checks                │
│  - Error recovery                   │
│  - Audit logging                    │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│  SENSE Output Processing            │
│  - Quality criticism (stabilizer)   │
│  - LIE reward model                 │
│  - Memory crystallization           │
└─────────────────────────────────────┘
    ↓
Output to User
```

### Core Subsystems

#### 1. **Orchestrator** (`internal/engine/orchestrator.go` - 86KB)
Central coordination hub that:
- Manages conversation state and token tracking
- Orchestrates tool execution loops
- Handles provider routing and fallback
- Integrates native function calling
- Manages extended thinking budgets
- Coordinates with SENSE awareness layer
- Tracks resource budgets (ARC)
- Maintains context compression (CCI)
- Observes and learns from execution (MEL)

#### 2. **ARC Router** (Adaptive Response Classification)
Classifies queries and computes resource budgets:
- **Classification Types:**
  - `Direct`: Simple questions, fast execution
  - `Analytical`: Reasoning-heavy, uses thinking tokens
  - `Speculative`: Creative/exploratory, higher temperature
- **Budget Computation:** Adjusts token limits, temperature, tool call count based on hardware
- **Consistency Checker:** Validates destructive operations before execution

#### 3. **CCI Layer** (Codified Context Infrastructure)
Manages context across conversations:
- **Hot Memory**: Recent context (immediate relevance)
- **Cold Memory**: Historical context (semantic relevance via vector search)
- **Drift Detection**: Monitors context coherence
- **Specialist Management**: Delegates tasks to specialized models
- **Tier System**: 3-tier relevance scoring

#### 4. **MEL System** (Meta-Experience Learning)
Learns from execution traces:
- **Heuristics**: "When [context], verify [constraint], avoid [error]" templates
- **Vector Store**: Jaccard similarity (500 items max, auto-evict lowest-confidence)
- **Bifurcation Analysis**: Observes failures/successes, generates heuristics
- **BM25 Ranking**: Relevance ranking for heuristic retrieval

#### 5. **SENSE Awareness Layer** (v1.9.0)
Input/output awareness subsystem:
- **Input Sanitizer**: 19 injection patterns + context scanning
- **Tracer**: Async JSONL event logging for analysis
- **Stabilizer**: 4-dimensional quality critic
- **Compression Pipeline**: 4-stage context compression
- **AgeMem**: 3-tier STM/LTM (short/medium/long-term) memory
- **Engrams**: Episodic memory for high-value interactions
- **LIE**: Reward model for output evaluation

#### 6. **Tool System** (75+ tools, 24.8KB lines of code)
Comprehensive tool registry with categories:
- **File Operations**: read_file, write_file, list_directory, search_files, grep_content, file_info, delete_file
- **Git/VCS**: git_status, git_diff, git_log, git_commit, git_push, git_pull
- **Web/HTTP**: web_fetch, http_request, browser_control, download_file, check_port
- **System**: bash, system_info, process_list, kill_process, disk_usage, env_var
- **Security/Pentesting**: 32+ specialized tools (nmap_scan, sqlmap_scan, nuclei_scan, etc.)
- **Android-Specific**: adb_setup, android_apps, android_control, android_system, android_intents, android_accessibility
- **Code Execution**: python_sandbox, jupyter, code_exec, structured_bash
- **Vision/Media**: vision.go, media_ops, browser_scrape, screenshot capture
- **Data Science**: ML/stats operations
- **Meta/Admin**: list_tools, tool_info, create_tool (dynamic generation)
- **Advanced**: pentest.go (64KB), advanced.go (32KB), scrapling.go (20KB)
- **Specialized**: brain_tools, colony_tool, cci_tools, consult, skill_tools, spawn_agent, worktrees, task_mgmt, etc.

#### 7. **TUI** (Terminal User Interface - v3.5.1)
Professional Bubble Tea-based interface (40+ files):
- **Elm MVC Architecture**: Model (state), Update (events), View (rendering)
- **Views**: Chat, Model selection (Ctrl+T), Tools table (Ctrl+E), Cloud Brains (Ctrl+D), Analytics (Ctrl+A), Diagnostics (Ctrl+\)
- **Features**: Markdown rendering, code highlighting, streaming output, touch scroll (Android), keyboard shortcuts, slash commands
- **Status Bar**: Context%, cost tracking, execution mode, git branch
- **Overlays**: Settings (Ctrl+G), SENSE HITL approval, API key entry

#### 8. **Multi-Provider Support**
Seamless integration with 5 major AI providers:

| Provider | Models | Key Features | Config |
|----------|--------|--------------|--------|
| **xAI (Grok)** | grok-3-mini, grok-3-vision | Native function calling, extended thinking | XAI_API_KEY |
| **Google (Gemini)** | gemini-2.0-pro, gemini-1.5-flash | Streaming, vision-capable | GEMINI_API_KEY |
| **Anthropic (Claude)** | claude-opus-4-1, claude-sonnet-4 | Extended thinking, 200K tokens | ANTHROPIC_API_KEY |
| **OpenAI (GPT)** | gpt-4-turbo, gpt-4o | Function calling, vision | OPENAI_API_KEY |
| **MiniMax** | minimax-01 | OpenAI-compatible wrapper | MINIMAX_API_KEY |

---

## 🛠️ Building & Installation

### Prerequisites
- **Go 1.25.0+** (check: `go version`)
- **Make** (check: `make --version`)
- **Git** (check: `git --version`)
- **Platform**: Linux, macOS, Windows, or Android/Termux

### Build Targets

```bash
# Build for current OS (output: ./bin/gorkbot)
make build

# Build for specific platforms
make build-linux    # Linux amd64
make build-windows  # Windows amd64
make build-android  # Android arm64

# Clean build artifacts
make clean

# Install to GOPATH/bin
make install

# Build and run (one command)
make run
```

### Installation Options

#### Option 1: From Source
```bash
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
make build
sudo cp bin/gorkbot /usr/local/bin/
```

#### Option 2: Using Wrapper Script
```bash
./gorkbot.sh          # TUI mode
./gorkbot.sh         # Alias
./gorkbot.sh -p "..."  # One-shot mode
```

---

## 🚀 Usage

### Interactive TUI Mode
```bash
./gorkbot.sh
```

Features:
- Beautiful markdown rendering
- Multi-line input (Alt+Enter)
- Slash commands (/help, /model, /tools, etc.)
- Streaming token display
- History scrolling (PgUp/PgDn)
- Touch scroll support (Android)

### One-Shot Mode
```bash
./gorkbot.sh -p "Your question here"
```

Perfect for:
- Quick queries
- Scripting and automation
- CI/CD pipelines
- Integration with shell scripts

### Configuration
```bash
# Setup wizard (configure API keys)
./gorkbot.sh setup

# Check configuration status
./gorkbot.sh status

# Advanced debugging
./gorkbot.sh -watchdog          # Enable orchestrator state debugging
./gorkbot.sh -verbose-thoughts  # Show consultant thinking
./gorkbot.sh --trace            # Enable execution tracing
```

### Slash Commands

#### Core Commands
- `/help` - Show all available commands
- `/clear` - Clear conversation history
- `/quit` - Exit Gorkbot
- `/version` - Show version information

#### Model & Provider Commands
- `/model <name>` - Switch to a different AI model
- `/key <provider> <key>` - Add/update API key for a provider
- `/key status` - Show API key status for all providers
- `/model` alone opens dual-pane model selection (Ctrl+T)

#### Advanced Commands
- `/settings` - Open settings overlay (Ctrl+G) with tabs:
  - Model routing (primary/secondary/auto)
  - Verbosity (output level)
  - Tool groups (enable/disable categories)
  - API providers
- `/tools` - Show available tools table (Ctrl+E)
- `/theme <name>` - Change theme (dracula, nord, gruvbox, solarized, monokai)
- `/compress` - Compress conversation context
- `/context` - Show context window statistics
- `/cost` - Show token usage and cost tracking
- `/rewind <id>` - Rewind to previous checkpoint
- `/mode <normal|plan|auto>` - Switch execution mode
- `/export <format>` - Export conversation (markdown/json/plain)
- `/compact` - Compact conversation focus
- `/skills` - Manage dynamic skills
- `/rules` - Configure permission rules
- `/hooks` - Manage lifecycle hooks
- `/rename` - Rename a conversation
- `/sandbox` - Run code in isolated environment
- `/mcp [status|config]` - MCP server management
- `/rate <1-5>` - Rate response for adaptive routing
- `/share [start|stop]` - Enable/disable session sharing
- `/billing` - Show token cost breakdown
- `/diagnostic` - Show diagnostic information

#### Keyboard Shortcuts
| Binding | Action |
|---------|--------|
| Enter | Send message |
| Alt+Enter | New line (multi-line input) |
| Ctrl+C | Quit |
| Ctrl+T | Open model selection |
| Ctrl+G | Open settings overlay |
| Ctrl+E | Show tools table |
| Ctrl+D | Open Cloud Brains (model discovery) |
| Ctrl+A | Show analytics dashboard |
| Ctrl+\\ | Show diagnostics |
| Ctrl+X | Interrupt generation |
| Ctrl+P | Cycle execution mode |
| PgUp/PgDn | Scroll history |
| Esc | Cancel generation / close overlay |
| Tab | Switch pane (in model selection) |

---

## 🔧 Configuration

### API Keys
```bash
# Create .env file (from .env.example)
cp .env.example .env

# Edit and add your keys
nano .env
```

```env
# xAI API Key (for Grok)
XAI_API_KEY=xai-xxxxxxxxxxxxx

# Google API Key (for Gemini)
GEMINI_API_KEY=AIzaxxxxxxxxxxxxxxx

# Anthropic API Key (optional)
ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxx

# OpenAI API Key (optional)
OPENAI_API_KEY=sk-xxxxxxxxxxxxx

# MiniMax API Key (optional)
MINIMAX_API_KEY=xxxxxxxxxxxxx
```

### Configuration Files
Configuration is persisted in platform-specific directories:

| File | Purpose | Permissions |
|------|---------|-------------|
| `~/.config/gorkbot/app_state.json` | Model selection, disabled categories | 0600 |
| `~/.config/gorkbot/api_keys.json` | Encrypted API key storage | 0600 |
| `~/.config/gorkbot/tool_permissions.json` | Tool permission settings | 0600 |
| `~/.config/gorkbot/active_theme` | Active theme selection | 0644 |
| `~/.config/gorkbot/themes/*.json` | Custom theme definitions | 0644 |
| `~/.config/gorkbot/mcp.json` | MCP server configuration | 0644 |
| `~/.config/gorkbot/hooks/` | Lifecycle hook scripts | 0755 |

### Data Storage
| Location | Purpose |
|----------|---------|
| `~/.local/share/gorkbot/gorkbot.db` | SQLite conversation store |
| `~/.local/share/gorkbot/vector_store.json` | MEL heuristic storage |
| `~/.local/share/gorkbot/tool_analytics.json` | Tool execution statistics |
| `~/.local/share/gorkbot/usage_history.jsonl` | Token usage history |
| `~/.local/share/gorkbot/traces/` | SENSE execution traces |

### Logging
```bash
# View logs in JSON format
cat ~/.local/share/gorkbot/gorkbot.json | jq .

# Follow logs in real-time
tail -f ~/.local/share/gorkbot/gorkbot.json | jq .
```

---

## 🧠 Advanced Features

### Extended Thinking Support
For models like Grok-3-mini and Claude 3.7+:
```bash
# Enable thinking with budget of 10,000 tokens
/thinking-budget 10000

# Disable thinking
/thinking-budget 0
```

Thinking tokens are shown in a dedicated panel in the TUI.

### Plan Mode
Generate a plan first, then execute with approval:
```bash
/mode plan
# Ask a complex question
# Gorkbot generates a plan
# Review and approve each step
```

### Session Relay & Collaboration
Share your session with others:
```bash
# Start relay server
/share start
# Relay URL: ws://localhost:9876
# Give URL to collaborators

# Stop relay
/share stop
```

Collaborators can observe with:
```bash
./gorkbot.sh --join ws://localhost:9876
```

### Custom Tool Creation
AI agents can create new tools dynamically:
```
User: Create a tool that pings a host 5 times
Gorkbot: [Creates ping_host tool]
Tool saved to: pkg/tools/custom/ping_host.go
```

### Model Context Protocol (MCP)
Integrate external tools via MCP servers:
```bash
# Configure in ~/.config/gorkbot/mcp.json
{
  "servers": [
    {
      "name": "my-server",
      "command": "/path/to/server",
      "args": []
    }
  ]
}

# Verify MCP status
/mcp status
```

### Worktree Tools
Execute tasks in isolated git worktrees:
```
Tool: spawn_agent
Parameters: {
  task: "Implement feature X",
  isolated: true
}
Result: Creates worktree, runs task, auto-cleans up
```

---

## 📊 Tool System Details

### Tool Categories & Count

| Category | Count | Examples |
|----------|-------|----------|
| Shell | 1 | bash |
| File Operations | 7 | read_file, write_file, list_directory, search_files, grep_content, file_info, delete_file |
| Git/VCS | 6 | git_status, git_diff, git_log, git_commit, git_push, git_pull |
| Web/HTTP | 6 | web_fetch, http_request, browser_control, download_file, check_port, browser_scrape |
| System | 6 | process_list, kill_process, env_var, system_info, disk_usage, service_control |
| Security/Pentesting | 32+ | nmap_scan, sqlmap_scan, nuclei_scan, totp_generate, exploit_db, vulnerability_scan, etc. |
| Android-Specific | 6 | adb_setup, android_apps, android_control, android_system, android_intents, android_accessibility |
| Code Execution | 4 | python_sandbox, jupyter, code_exec, structured_bash |
| Vision/Media | 5+ | vision (screen capture), media_ops, browser_scrape, screenshot_capture |
| Data Science | 3+ | data_science operations (ML/stats) |
| Memory | 5+ | brain_tools (long-term memory operations) |
| Multi-Agent | 3+ | colony_tool (debate), spawn_agent (subagent spawning), orchestration |
| Meta | 3 | list_tools, tool_info, create_tool |
| **TOTAL** | **75+** | — |

### Permission System

**Levels:**
- `always` - Permanent approval (persisted to `~/.config/gorkbot/tool_permissions.json`)
- `session` - Approved for current session only
- `once` - Ask each time (default for destructive operations)
- `never` - Permanently blocked

**Categories:**
- Enable/disable entire tool categories
- Fine-grained rule-based control with glob patterns
- Per-tool granularity

**Security:**
- Shell command escaping via shellescape()
- Parameter validation
- Execution timeouts
- Audit logging to SQLite

---

## 🔐 Security

### Key Security Features
1. **Encrypted API Key Storage**: Keys stored at `~/.config/gorkbot/api_keys.json` with AES encryption
2. **Fine-Grained Permissions**: Per-tool and per-category permission system
3. **Input Sanitization**: SENSE layer with 19 injection patterns + context scanning
4. **Audit Logging**: SQLite audit database for all tool executions
5. **Shell Escaping**: Automatic shell command escaping for bash tool
6. **Timeouts**: Tool execution timeouts to prevent hanging
7. **Sandboxing**: Python sandbox and subprocess isolation

### Permission Management
```bash
# Check permission status
/key status

# Set tool permission
Permission prompt appears on first tool use

# Configure rules
/rules

# View audit log
~/.local/share/gorkbot/tool_audit.db
```

### API Key Safety
- Keys stored encrypted at rest
- Never logged or displayed in full
- Rotatable via `/key <provider> <new-key>`
- Platform-specific secure storage

---

## 📈 Token Usage & Costs

Track token usage and estimated costs:

```bash
# Show token statistics
/cost

# Show detailed context report
/context

# Enable billing tracking
Automatic (persisted to ~/.local/share/gorkbot/usage_history.jsonl)
```

Token counts by provider vary based on model and thinking tokens.

---

## 🐛 Troubleshooting

### Common Issues

**"API Key is not set"**
```bash
make setup
# Or edit .env manually
```

**Keys not loading**
```bash
# Use guided setup to persist provider keys and validate configuration
make setup
```

**Slow generation**
```bash
# Check token count
/context

# Try faster model
/model grok-3-mini
# or
/model gemini-2.0-flash
```

**Tools not executing**
```bash
# Check tool list
/tools

# Check permissions
/key status

# Enable diagnostics
./bin/gorkbot -watchdog
```

**TUI display issues**
```bash
# Try resizing terminal
# Or clear screen: Ctrl+L
# Check terminal width (should be 80+ chars)
```

### Debug Modes

```bash
# Enable orchestrator debugging
./bin/gorkbot -watchdog

# Enable verbose consultant thinking
./bin/gorkbot -verbose-thoughts

# Enable execution tracing
./bin/gorkbot --trace

# Enable all diagnostics
./bin/gorkbot -watchdog -verbose-thoughts --trace
```

### Getting Help

```bash
# Show all commands
/help

# Show version
/version

# Show diagnostics
/diagnostic
```

---

## 📦 Subsystems & Modules

### Core Engine (`internal/engine/`)
- **orchestrator.go** (86KB) - Main orchestration hub
- **streaming.go** (41KB) - Token streaming with relay
- **prompt_builder.go** (11KB) - System prompt construction
- **context_manager.go** - Token tracking
- Plus 26 more specialized modules

### TUI (`internal/tui/`)
- **model.go** - State management
- **update.go** - Event handling
- **view.go** - Rendering pipeline
- **keys.go** - Keybindings
- **messages.go** - Message types
- **style.go** - Theming
- **statusbar.go** - Status display
- Plus 35+ additional files

### AI Providers (`pkg/ai/`)
- **grok.go** (21KB) - xAI Grok provider
- **anthropic.go** (23KB) - Anthropic Claude
- **gemini.go** (16KB) - Google Gemini
- **openai_provider.go** - OpenAI GPT
- **minimax.go** - MiniMax wrapper
- **conversation.go** (12KB) - Message history
- Plus 10+ more

### Adaptive Intelligence (`pkg/adaptive/`)
- **arc/** - Adaptive Response Classification (router, classifier, budget, consistency)
- **cci/** - Codified Context Infrastructure (hot/cold memory, drift detection, tiers)
- **mel/** - Meta-Experience Learning (analyzer, heuristics, vectorstore)
- **routing_table.go** - Decision caching

### Tool System (`pkg/tools/`)
- **registry.go** (33KB) - Tool registry and management
- **permissions.go** - Permission system
- **cache.go** - TTL memoization
- **dispatcher.go** - Parallel execution
- **rules.go** - Fine-grained rules
- **error_recovery.go** - Error classification
- **analytics.go** - Execution analytics
- **audit_db.go** (16KB) - SQLite audit logging
- Plus 65+ tool implementation files

### SENSE Awareness (`pkg/sense/` - v1.9.0)
- **input_sanitizer.go** - 19 injection patterns
- **tracer.go** - Async JSONL logging
- **stabilizer.go** - 4D quality critic
- **compression.go** - 4-stage compression
- **agemem.go** - 3-tier STM/LTM memory
- **engrams.go** - Episodic memory
- **lie.go** - Reward model
- Plus 4+ more

### Channels (`pkg/channels/`)
- **discord/** - Discord bot integration
- **telegram/** - Telegram bot integration
- **bridge/** - Channel routing

---

## 🧬 Development

### Project Structure
```
gorkbot/
├── cmd/                     # Executable entry points
│   ├── gorkbot/            # Main TUI app
│   ├── gorkbot/           # Alias
│   └── ...                 # Other variants
├── internal/               # Private packages
│   ├── engine/             # Orchestrator (29 files)
│   ├── tui/                # Terminal UI (40+ files)
│   ├── platform/           # Platform abstraction
│   └── ...                 # Other subsystems
├── pkg/                    # Public packages (44 subsystems)
│   ├── ai/                 # AI providers (16 files)
│   ├── adaptive/           # Intelligence layer (38 files)
│   ├── tools/              # Tool system (75 files)
│   ├── sense/              # Awareness layer (11 files)
│   ├── channels/           # Channel integrations
│   ├── commands/           # Slash commands
│   ├── config/             # Configuration
│   ├── discovery/          # Model discovery
│   ├── mcp/                # Model Context Protocol
│   ├── skills/             # Dynamic skills
│   ├── memory/             # Memory management
│   ├── session/            # Session management
│   └── ...                 # 30+ more
├── docs/                   # Documentation
├── go.mod, go.sum         # Dependencies
├── Makefile               # Build system
├── gorkbot.sh             # Wrapper script
└── README.md              # This file
```

### Adding Features

#### Adding a New Tool
1. Create in `pkg/tools/tool_category.go`
2. Implement `Tool` interface
3. Register in `RegisterDefaultTools()` in `registry.go`
4. Test: `go test ./pkg/tools`

#### Adding a New Command
1. Register in `pkg/commands/registry.go`
2. Implement handler function in `OrchestratorAdapter`
3. Handle signal in `internal/tui/update.go`

#### Adding a New AI Provider
1. Implement `AIProvider` interface in `pkg/ai/`
2. Add provider to discovery in `pkg/discovery/`
3. Wire into main.go provider initialization

#### Modifying Orchestrator
1. Edit `internal/engine/orchestrator.go`
2. Maintain subsystem interfaces (ARC, CCI, MEL, SENSE)
3. Test: `go test ./internal/engine`

### Testing
```bash
# Run all tests
go test ./...

# Test specific package
go test ./pkg/ai
go test ./pkg/tools
go test ./internal/engine

# Verbose output
go test -v ./...

# With coverage
go test -cover ./...
```

### Code Conventions
- **Package structure**: Go standard layout (cmd/, internal/, pkg/)
- **Interfaces**: AI providers, tools, commands use interfaces
- **Messages**: TUI messages use typed structs (not strings)
- **Logging**: Structured logging with slog (JSON format)
- **Error handling**: Explicit error returns, no panic except initialization

---

## 📊 Statistics

| Metric | Value |
|--------|-------|
| **Total Go Files** | 261 |
| **Total Lines of Code** | ~150,000+ |
| **Tool System Code** | 24,823 lines (75 tools) |
| **Engine Code** | 347 KB (29 files) |
| **AI Provider Code** | ~120 KB (16 files) |
| **Intelligence Layer** | ~400 KB (38 files) |
| **SENSE Layer** | Latest version 1.9.0 |
| **TUI Code** | 40+ files (Elm MVC) |
| **Total Packages** | 44 subsystems |
| **Supported Providers** | 5 (xAI, Google, Anthropic, OpenAI, MiniMax) |
| **Built-in Tools** | 75+ |
| **Slash Commands** | 30+ |

---

## 🤝 Contributing

Contributions welcome! Areas for improvement:
- Additional AI providers
- New tool implementations
- UI/UX improvements
- Performance optimizations
- Documentation enhancements
- Bug fixes and stability improvements

---

## 📄 License

Proprietary (Velarium AI) - All rights reserved

---

## 🔗 Resources

### Official Links
- **xAI Console**: https://console.x.ai/
- **xAI Documentation**: https://docs.x.ai/
- **Google AI Studio**: https://aistudio.google.com/apikey
- **Anthropic Console**: https://console.anthropic.com/
- **OpenAI Platform**: https://platform.openai.com/

### Technology Stack
- **Language**: Go 1.25.0
- **TUI Framework**: Charm Bracelet (Bubble Tea, Lip Gloss, Glamour)
- **Database**: SQLite
- **Protocols**: JSON-RPC, HTTP/2, WebSocket, MCP

### Key Dependencies
- `bubbletea` - TUI framework
- `lipgloss` - Terminal styling
- `glamour` - Markdown rendering
- `anthropic-sdk-go` - Anthropic provider
- `google/generative-ai-go` - Google provider
- `openai-go` - OpenAI provider
- `mark3labs/mcp-go` - MCP protocol
- `robfig/cron` - Task scheduling
- `modernc.org/sqlite` - Database

---

## 🎯 Roadmap

Current focus areas:
- [ ] Streaming video input support
- [ ] Persistent conversation storage improvements
- [ ] Additional MCP server integrations
- [ ] Performance optimization (token streaming)
- [ ] Enhanced memory search
- [ ] Cross-platform testing improvements

---

## 📞 Support

For issues and bug reports:
1. Check `docs/troubleshooting/` for known issues
2. Review `/diagnostic` output
3. Check logs: `~/.local/share/gorkbot/gorkbot.json`
4. Report via GitHub issues

---

**Built with ❤️ by Velarium AI**

Made for developers, researchers, and AI enthusiasts who need powerful, flexible AI orchestration.
# Gorkbot Release
