# Gorkbot

**Multi-model AI orchestration in your terminal.**

Gorkbot is an enterprise-grade, open-source AI agent CLI that combines five AI providers — xAI Grok, Google Gemini, Anthropic Claude, OpenAI, and MiniMax — in a unified full-screen terminal UI. A sophisticated orchestration engine, 150+ built-in tools, a three-tier persistent memory system, and an adaptive intelligence layer make Gorkbot capable of handling everything from quick queries to complex, multi-step autonomous agentic workflows.

> **Version:** 3.4.0
> **License:** MIT
> **Author:** Todd Eddings / Velarium AI
> **Module:** `github.com/velariumai/gorkbot`

---

## Feature Overview

### AI & Orchestration
- **Five AI providers** — xAI (Grok), Google (Gemini), Anthropic (Claude), OpenAI, MiniMax; all switchable at runtime
- **Dynamic model discovery** — live model lists polled from all providers every 30 minutes; best model auto-selected per task
- **Dual-model orchestration** — primary agent + specialist consultant with automatic routing for complex queries
- **Native xAI function calling** — structured `tool_calls` via xAI API; falls back to text parsing for other providers
- **ARC Router** — keyword-scored query classifier routes tasks to `WorkflowDirect` or `WorkflowReasonVerify`; compute budget (tokens, temperature, timeout) tuned per device profile
- **MEL (Meta-Experience Learning)** — persisted vector store of heuristics derived from bifurcation analysis of past tool failures; injected into system prompt
- **Agent-to-Agent (A2A)** — HTTP gateway for inter-agent task delegation (JSON-RPC style)
- **Adaptive model routing** — JSONL-persisted feedback loop; `/rate 1-5` teaches the router which model performs best per task category

### Tool System
- **150+ built-in tools** across 20+ categories: shell, file, git, web, system, security, Android/Termux, DevOps, media, data science, personal, AI/ML, database, vision, and more
- **Parallel tool execution** — up to 4 concurrent tool goroutines per turn; result collection preserves ordering
- **Dynamic tool creation** — `create_tool` generates hot-loaded tools at runtime (no restart required); `rebuild` permanently compiles them into the binary
- **Tool permission system** — four levels (always/session/once/never) with per-tool glob-pattern rules and persistent JSON storage
- **Category-level enable/disable** — disable entire tool groups via `/settings` (persisted across sessions)
- **MCP (Model Context Protocol)** — multi-server stdio client; tools prefixed `mcp_<server>_<toolname>` registered automatically
- **Tool analytics** — per-tool call counts, success rates, and latency stored in SQLite

### Memory & Context
- **CCI (Codified Context Infrastructure)** — three-tier persistent project memory:
  - **Tier 1 (Hot)** — always-loaded conventions and subsystem index injected as system prompt prefix
  - **Tier 2 (Specialist)** — on-demand domain personas with failure-mode tables loaded by ARC trigger table
  - **Tier 3 (Cold)** — on-demand subsystem specs queryable via `mcp_context_*` tools
- **SENSE AgeMem** — age-stratified episodic memory with engram store for cross-session recall
- **Goal Ledger** — prospective cross-session memory; tracks open goals across restarts
- **Unified Memory** — single API wrapping AgeMem + Engrams + MEL vector store
- **Session checkpoints** — up to 20 snapshots per session; `/rewind` restores any checkpoint
- **Conversation export** — `/export [markdown|json|plain]` exports full history to file
- **Context window tracking** — live token usage %, cost estimate, and mode display in status bar

### Terminal UI
- **Full-screen TUI** built with Bubble Tea, Lip Gloss, and Glamour
- **Markdown rendering** — full CommonMark with syntax-highlighted code blocks
- **Touch-scroll** — works on Android/Termux with finger scrolling
- **Tabs** — Chat, Tools (`Ctrl+E`), Models (`Ctrl+T`), Cloud Brains (`Ctrl+D`), Diagnostics (`Ctrl+\`)
- **Settings overlay** (`Ctrl+G`) — model routing, verbosity, tool group enable/disable
- **Bookmarks** (`Ctrl+B`) — bookmark and jump to important conversation points
- **Execution modes** — Normal, Plan, Auto (cycle with `Ctrl+P` or `/mode`)
- **Debug mode** (`/debug`) — reveals raw AI output including tool JSON blocks
- **30+ slash commands** — full list in [Commands Reference](#commands-reference)

### Integrations & Channels
- **Telegram bot** — configure via `~/.config/gorkbot/telegram.json`; routes messages through the orchestrator
- **Remote session sharing** — `--share` starts an SSE relay; observers join with `--join <host:port>`
- **Scheduler** — cron-style task scheduling persisted to disk; `/schedule` shows active tasks
- **User-defined commands** — `define_command` tool creates custom slash commands at runtime
- **Skills system** — YAML-frontmatter markdown skill definitions invokable as `/skill-name`
- **Lifecycle hooks** — shell scripts in `~/.config/gorkbot/hooks/` run on events (pre-tool, post-tool, session-start, etc.)
- **SQLite persistence** — conversation history and tool call analytics stored per session

### Security & Platform
- **Encrypted `.env`** — API key values prefixed `ENC_` are decrypted at startup using a local key manager
- **Shell escaping** — all bash tool parameters sanitized with `shellescape()`
- **Execution timeouts** — every tool call has a hard timeout
- **Cross-platform** — Linux (amd64), macOS (arm64/amd64), Android/Termux (arm64), Windows (amd64)
- **XDG compliance** — config, log, and data paths follow OS-appropriate conventions
- **Comprehensive pentesting suite** — 30+ security tools (requires explicit tool-category enable)

---

## Requirements

| Requirement | Version |
|-------------|---------|
| Go | 1.24.2+ |
| xAI API key | Required for Grok (primary) |
| Google Gemini API key | Required for Gemini (specialist) |
| Anthropic API key | Optional |
| OpenAI API key | Optional |
| MiniMax API key | Optional |

At least one of xAI or Google credentials is required. Gorkbot functions with either provider alone.

---

## Quick Start

```bash
# 1. Clone
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot

# 2. Interactive setup wizard
./gorkbot.sh setup

# 3. Build
make build

# 4. Run
./gorkbot.sh
```

For a complete walkthrough including all providers and configuration options, see [GETTING_STARTED.md](GETTING_STARTED.md).

---

## Usage

```bash
# Interactive TUI (default)
./gorkbot.sh

# One-shot prompt
./gorkbot.sh -p "Summarize this codebase"

# Read prompt from stdin
echo "Explain async/await" | ./gorkbot.sh --stdin

# One-shot with output file
./gorkbot.sh -p "Write a dockerfile" --output Dockerfile

# Share session over SSE relay
./gorkbot.sh --share

# Join a shared session as observer
./gorkbot.sh --join localhost:9090

# Enable A2A HTTP gateway
./gorkbot.sh --a2a --a2a-addr 127.0.0.1:18890

# Debug flags
./gorkbot.sh --verbose-thoughts   # show consultant (Gemini) reasoning
./gorkbot.sh --watchdog           # orchestrator state debug log
./gorkbot.sh --trace              # write JSONL execution trace to ~/.gorkbot/traces/

# Model overrides (bypass dynamic selection)
GORKBOT_PRIMARY_MODEL=grok-3-mini ./gorkbot.sh
GORKBOT_CONSULTANT_MODEL=gemini-2.0-flash ./gorkbot.sh

# Install global shorthand 'gork'
make install-global
gork
```

### One-Shot Tool Filtering

```bash
# Allow only specific tools
./gorkbot.sh -p "List this directory" --allow-tools bash,read_file,list_directory

# Block specific tools
./gorkbot.sh -p "Review this code" --deny-tools bash,write_file,delete_file
```

---

## Build Targets

```bash
make build           # host OS → bin/gorkbot
make build-linux     # Linux amd64 → bin/gorkbot-linux
make build-android   # Android arm64 → bin/gorkbot-android
make build-windows   # Windows amd64 → bin/gorkbot.exe
make install-global  # install 'gork' shorthand to ~/bin
make dist            # build all platforms + create release tarball in dist/
make clean           # remove bin/ and dist/
```

---

## Commands Reference

All commands are entered in the TUI input field prefixed with `/`.

| Command | Description |
|---------|-------------|
| `/help` | Show all commands |
| `/clear` | Reset conversation and screen |
| `/model [primary\|consultant] [id]` | View or switch model |
| `/key <provider> <key>` | Set API key for a provider at runtime |
| `/tools [stats]` | List tools; `stats` shows usage analytics |
| `/permissions [list\|reset\|reset <tool>]` | Manage tool permissions |
| `/rules [list\|add\|remove]` | Fine-grained glob-pattern permission rules |
| `/context` | Show context window usage breakdown |
| `/cost` | Show session API cost estimate |
| `/mode [normal\|plan\|auto]` | View or switch execution mode |
| `/compact [hint]` | Compress context to save tokens |
| `/compress` | Alias for `/compact` |
| `/rewind [last\|<id>]` | Restore to a session checkpoint |
| `/export [markdown\|json\|plain] [file]` | Export conversation to file |
| `/save <name>` | Save session to named file |
| `/resume <name>` | Restore a saved session |
| `/chat [save\|load\|list\|delete] [name]` | Manage conversation sessions |
| `/rename <name>` | Rename current session |
| `/skills [list\|help]` | List and invoke skill definitions |
| `/hooks [list\|dir]` | List lifecycle hook scripts |
| `/theme [light\|dark\|auto\|<name>]` | Change UI theme |
| `/mcp [status\|config]` | MCP server status |
| `/a2a` | A2A gateway status |
| `/telegram` | Telegram bot status |
| `/schedule` | List scheduled tasks |
| `/share [start\|stop]` | Start/stop SSE session sharing |
| `/rate <1-5>` | Rate last response (trains adaptive router) |
| `/debug` | Toggle debug mode (raw AI output) |
| `/auth [refresh\|status]` | API credential status |
| `/settings` | Open settings overlay |
| `/version` | Show build version and system info |
| `/commands` | List user-defined slash commands |
| `/bug` | Open GitHub issue template |
| `/quit` | Exit gracefully |

Skill definitions and user-defined commands are also invokable as slash commands (e.g., `/code-review`, `/auto_2fa`).

---

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Alt+Enter` | Insert newline (multi-line input) |
| `PgUp` / `PgDn` | Scroll conversation |
| `Ctrl+C` / `Ctrl+Q` | Quit |
| `Ctrl+H` | Help |
| `Ctrl+L` | Clear screen |
| `Ctrl+X` | Interrupt / cancel in-progress generation |
| `Ctrl+T` | Open model selection |
| `Ctrl+G` | Open settings overlay |
| `Ctrl+P` | Cycle execution mode (Normal → Plan → Auto) |
| `Ctrl+E` | Show tools panel |
| `Ctrl+D` | Open Cloud Brains (discovery) tab |
| `Ctrl+R` | Fold/unfold reasoning frames |
| `Ctrl+B` | Open conversation bookmarks |
| `Ctrl+\` | System diagnostics view |
| `Ctrl+I` | Focus input field |
| `Esc` | Back / close overlay |

---

## Project Structure

```
gorkbot/
├── cmd/gorkbot/          Entry point (main.go + helpers)
├── internal/
│   ├── arc/              ARC Router — query classifier, compute budget, consistency check
│   ├── engine/           Orchestration engine, streaming, intelligence, CCI integration
│   ├── mel/              MEL — heuristic store, bifurcation analyzer, VectorStore
│   ├── platform/         OS/environment abstraction, Version constant
│   └── tui/              Full-screen terminal UI (Bubble Tea)
├── pkg/
│   ├── a2a/              Agent-to-Agent HTTP gateway
│   ├── ai/               AI provider implementations (grok, gemini, anthropic, openai, minimax)
│   ├── billing/          Per-model cost tracking, usage_history.jsonl
│   ├── cci/              Codified Context Infrastructure (3-tier memory)
│   ├── channels/telegram Telegram bot integration
│   ├── collab/           SSE relay server + observer client
│   ├── colony/           Colony debate (multi-perspective reasoning)
│   ├── commands/         Slash command registry + OrchestratorAdapter
│   ├── config/           GORKBOT.md hierarchical config, AppStateManager
│   ├── discovery/        Live model discovery (all 5 providers)
│   ├── hooks/            Lifecycle hook script manager
│   ├── mcp/              MCP protocol client (stdio, JSON-RPC 2.0)
│   ├── memory/           MemoryManager, AgeMem, Engrams, GoalLedger, UnifiedMemory
│   ├── persist/          SQLite persistence (conversation + tool analytics)
│   ├── pipeline/         Agentic pipeline execution
│   ├── process/          Managed background process system
│   ├── providers/        KeyStore, ProviderManager (5 providers), global singleton
│   ├── registry/         Model registry (dynamic model list)
│   ├── router/           Adaptive router, FeedbackManager, SystemConfiguration
│   ├── scheduler/        Cron-style task scheduler with SQLite store
│   ├── security/         Key manager (AES-GCM encryption for .env values)
│   ├── sense/            SENSE AgeMem integration
│   ├── session/          Checkpoint, exporter, loader, workspace
│   ├── skills/           Skill definition loader (YAML frontmatter markdown)
│   ├── subagents/        Sub-agent system (spawn, delegate, worktree isolation)
│   ├── theme/            JSON-based theme system (5 built-ins + custom)
│   ├── tools/            Tool registry, 150+ tool implementations
│   ├── tui/              (alias — tui lives in internal/tui)
│   ├── usercommands/     User-defined slash command loader
│   └── vision/           Vision pipeline (ADB, MediaProjection, Grok Vision API)
├── plugins/python/       Python plugin bridge
├── scripts/              Setup and bridge scripts
├── docs/                 Reference documentation
├── gorkbot.sh            Launcher script (loads .env, passes all flags)
├── Makefile              Build targets
└── .env.example          API key template
```

---

## Configuration Files

| File | Location | Purpose |
|------|----------|---------|
| `.env` | Project root | API keys and overrides (gitignored) |
| `api_keys.json` | `~/.config/gorkbot/` | Encrypted API key store |
| `app_state.json` | `~/.config/gorkbot/` | Persisted model selection + tool prefs |
| `active_theme` | `~/.config/gorkbot/` | Active theme name |
| `themes/*.json` | `~/.config/gorkbot/` | Custom theme definitions |
| `mcp.json` | `~/.config/gorkbot/` | MCP server configuration |
| `tool_permissions.json` | `~/.config/gorkbot/` | Persisted tool permissions |
| `dynamic_tools.json` | `~/.config/gorkbot/` | Hot-loaded tools created via `create_tool` |
| `telegram.json` | `~/.config/gorkbot/` | Telegram bot token + config |
| `vector_store.json` | `~/.config/gorkbot/` | MEL heuristic vector store |
| `feedback.jsonl` | `~/.config/gorkbot/` | Adaptive router feedback history |
| `usage_history.jsonl` | `~/.config/gorkbot/` | Per-model billing history |
| `cci/` | `~/.config/gorkbot/` | CCI tier 1/2/3 memory files |
| `hooks/` | `~/.config/gorkbot/` | Lifecycle hook scripts |
| `gorkbot.json` | `~/.gorkbot/logs/` (Linux) | Structured JSON log |
| `traces/` | `~/.gorkbot/traces/` | JSONL execution traces (--trace) |
| `GORKBOT.md` | Project root (or any parent) | Hierarchical project config |

Config and log directory locations are platform-specific:

| Platform | Config Dir | Log Dir |
|----------|-----------|---------|
| Linux (XDG) | `~/.config/gorkbot/` | `~/.local/share/gorkbot/logs/` |
| macOS | `~/Library/Application Support/gorkbot/` | `~/Library/Logs/gorkbot/` |
| Windows | `%APPDATA%\gorkbot\` | `%LOCALAPPDATA%\gorkbot\logs\` |
| Android/Termux | `~/.config/gorkbot/` | `~/.gorkbot/logs/` |

---

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `XAI_API_KEY` | xAI (Grok) API key |
| `GEMINI_API_KEY` | Google Gemini API key |
| `ANTHROPIC_API_KEY` | Anthropic (Claude) API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `MINIMAX_API_KEY` | MiniMax API key |
| `GORKBOT_PRIMARY_MODEL` | Override primary model ID |
| `GORKBOT_CONSULTANT_MODEL` | Override consultant model ID |
| `GORKBOT_AUTO_REBUILD` | Set to `1` to auto-compile new dynamic tools on exit |
| `TERMUX_VERSION` | Detected automatically; enables Termux-specific paths |

---

## Documentation

| Document | Description |
|----------|-------------|
| [GETTING_STARTED.md](GETTING_STARTED.md) | Installation, API keys, first run, TUI walkthrough |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Full system architecture — engine, intelligence, memory, TUI |
| [docs/TOOLS_REFERENCE.md](docs/TOOLS_REFERENCE.md) | Complete tool reference with parameters and examples |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | All config files, env vars, flags, and paths |
| [docs/PROVIDERS.md](docs/PROVIDERS.md) | Provider guide — models, API keys, hot-swap, dynamic discovery |
| [docs/PERMISSIONS_GUIDE.md](docs/PERMISSIONS_GUIDE.md) | Tool permission system — levels, rules, best practices |
| [docs/SECURITY.md](docs/SECURITY.md) | Security practices — key management, encryption, sandboxing |
| [docs/CONTEXT_CONTINUITY.md](docs/CONTEXT_CONTINUITY.md) | Conversation context and CCI memory system |

---

## License

MIT — see [LICENSE](LICENSE) for details.

Gorkbot is an independent open-source project and is not affiliated with xAI, Google, Anthropic, OpenAI, or MiniMax.
