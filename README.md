# Gorkbot

**Multi-model AI orchestration in your terminal.**

Gorkbot is an enterprise-grade, open-source AI agent CLI that combines seven AI providers — xAI Grok, Google Gemini, Anthropic Claude, OpenAI, MiniMax, Moonshot, and OpenRouter — in a unified full-screen terminal UI. A sophisticated orchestration engine, 196+ built-in tools, a three-tier persistent memory system (CCI), an adaptive intelligence layer (ARC + MEL), and automatic provider failover make Gorkbot capable of handling everything from quick queries to complex, multi-step autonomous agentic workflows.

> **Version:** 4.7.0
> **License:** MIT
> **Author:** Todd Eddings / Velarium AI
> **Module:** `github.com/velariumai/gorkbot`

---

## Feature Overview

### AI & Orchestration

- **Seven AI providers** — xAI (Grok), Google (Gemini), Anthropic (Claude), OpenAI, MiniMax, Moonshot, OpenRouter; all switchable at runtime
- **Provider failover cascade** — automatic failover through `xAI → Google → Anthropic → MiniMax → OpenAI → OpenRouter` on outage, quota exhaustion, or credential failure
- **Manual provider toggles** — Settings overlay (`Ctrl+G`) enables/disables providers for the session; state persists across restarts
- **Dynamic model discovery** — live model lists polled from all providers every 30 minutes; best model auto-selected per task
- **Dual-model orchestration** — primary agent + specialist consultant with automatic routing for complex queries
- **Native xAI function calling** — structured `tool_calls` via xAI API; falls back to text parsing for other providers
- **Extended thinking** — Anthropic Claude and xAI reasoning models support extended thinking with configurable token budgets (`/think <budget>`)
- **ARC Router** (`pkg/adaptive`) — semantic intent classifier routes tasks to `WorkflowDirect` or `WorkflowReasonVerify`; compute budget (tokens, temperature, timeout) tuned per device RAM profile via HALProfile
- **MEL (Meta-Experience Learning)** (`pkg/adaptive`) — persisted BM25/TF-IDF + embedding vector store of heuristics derived from bifurcation analysis of past tool failures; injected into system prompt
- **Agent-to-Agent (A2A)** — HTTP gateway for inter-agent task delegation
- **Adaptive model routing** — JSONL-persisted feedback loop; `/rate 1-5` teaches the router which model performs best per task category
- **Colony debate** — multi-perspective reasoning via parallel agent perspectives
- **DAG execution engine** (`pkg/dag`) — dependency-resolved parallel task execution with rollback, retry, and RCA analysis

### Tool System

- **196+ built-in tools** across 20+ categories: shell, file, git, web, system, security, Android/Termux, DevOps, media, data science, vision, and more
- **Tool packs** — activated via `GORKBOT_TOOL_PACKS` env var; defaults load `core,dev,web,sys,agent,data,media,comm`; `ALL` loads all packs including security and vision
- **Parallel tool execution** — up to 4 concurrent tool goroutines per turn; result collection preserves ordering
- **Dynamic tool creation** — `create_tool` generates hot-loaded tools at runtime (no restart required); `rebuild` permanently compiles them into the binary
- **Python plugin sandbox** — `python_execute` runs Python code in an isolated sandbox with RPC access to safe built-in tools
- **Tool permission system** — four levels (always/session/once/never) with per-tool glob-pattern rules and persistent JSON storage
- **Category-level enable/disable** — disable entire tool groups via `/settings` (persisted across sessions)
- **HITL security overlay** — Human-in-the-Loop guard blocks destructive tool calls pending explicit user confirmation
- **MCP (Model Context Protocol)** — multi-server stdio client; tools prefixed `mcp_<server>_<toolname>` registered automatically
- **Tool analytics** — per-tool call counts, success rates, and latency stored in SQLite
- **Structured SQLite audit log** — every tool execution logged with category classification

### Memory & Context

- **CCI (Codified Context Infrastructure)** (`pkg/adaptive`) — three-tier persistent project memory:
  - **Tier 1 (Hot)** — always-loaded conventions and subsystem index injected as system prompt prefix
  - **Tier 2 (Specialist)** — on-demand domain personas with failure-mode tables loaded by ARC trigger table
  - **Tier 3 (Cold)** — on-demand subsystem specs queryable via `mcp_context_*` tools
- **Truth Sentry** — drift detector compares CCI hot memory against live file hashes; injects warnings when documentation is stale
- **SENSE AgeMem** — age-stratified episodic memory with engram store for cross-session recall
- **Goal Ledger** — prospective cross-session memory; tracks open goals across restarts
- **Unified Memory** — single API wrapping AgeMem + Engrams + MEL vector store
- **Session checkpoints** — up to 20 snapshots per session; `/rewind` restores any checkpoint
- **Conversation export** — `/export [md|txt|pdf]` exports full history to file
- **Context window tracking** — live token usage %, cost estimate, and mode display in status bar
- **Auto-compression** — `CompressionPipe` compresses history when token count exceeds threshold
- **SQLite conversation persistence** — full session history queryable via `session_search`

### Terminal UI

- **Full-screen TUI** built with Bubble Tea, Lip Gloss, and Glamour
- **Markdown rendering** — full CommonMark with syntax-highlighted code blocks
- **Touch-scroll** — works on Android/Termux with finger scrolling
- **Tabs** — Chat, Models (`Ctrl+T`), Tools (`Ctrl+E`), Cloud Brains (`Ctrl+D`), Analytics (`Ctrl+A`), Diagnostics (`Ctrl+\`)
- **Settings overlay** (`Ctrl+G`) — 4 tabs: model routing, verbosity, tool group enable/disable, API provider toggles
- **Bookmarks** (`Ctrl+B`) — bookmark and jump to important conversation points
- **Execution modes** — Normal, Plan, Auto (cycle with `Ctrl+P` or `/mode`)
- **Debug mode** (`/debug`) — reveals raw AI output including tool JSON blocks
- **Live DAG view** — per-task progress bars, elapsed timers, dependency display, RCA panels
- **Web UI** — `bin/gorkweb` serves a browser-based interface (`make build-web`)
- **35+ slash commands** — full list in [Commands Reference](#commands-reference)

### Integrations & Channels

- **Telegram bot** — configure via `~/.config/gorkbot/telegram.json`; routes messages through the orchestrator
- **Discord channel** — optional Discord notification tool
- **Remote session sharing** — `--share` starts an SSE relay; observers join with `--join <host:port>`
- **Scheduler** — cron-style task scheduling persisted to disk; `/schedule` shows active tasks
- **User-defined commands** — `define_command` tool creates custom slash commands at runtime
- **Skills system** — YAML-frontmatter markdown skill definitions invokable as `/skill-name`
- **Lifecycle hooks** — shell scripts in `~/.config/gorkbot/hooks/` run on events (pre-tool, post-tool, session-start, etc.)
- **Billing manager** — per-model token cost tracking with all-time history (`/billing`)

### Local LLM Embedding (llamacpp build)

- **`make build-llm`** compiles a C++ bridge (`internal/llm/libgorkbot_llm.a`) against `ext/llama.cpp` and builds with `-tags llamacpp`
- **Nomic Embed Text v1.5** — 274 MB GGUF model provides on-device dense-vector embeddings without any cloud API calls
- **Semantic retrieval** — when an embedder is available, MEL vector store and ARC router use cosine similarity instead of pure BM25/TF-IDF
- **`make download-nomic`** — downloads the model to `~/.cache/llama.cpp/` (required for llamacpp build)
- **Cloud fallback** — when no local embedder is compiled, the system automatically falls back to keyword scoring

---

## Requirements

| Requirement | Version |
|-------------|---------|
| Go | 1.24.2+ |
| xAI API key | Recommended primary provider |
| Google Gemini API key | Recommended specialist provider |
| Anthropic API key | Optional |
| OpenAI API key | Optional |
| MiniMax API key | Optional |
| Moonshot API key | Optional |
| OpenRouter API key | Optional (accesses 400+ models) |

At least one provider API key is required. Gorkbot functions with any single provider.

**For `make build-llm` (local embedding):**

| Requirement | Notes |
|-------------|-------|
| C++ compiler (clang/gcc) | For building the llama.cpp bridge |
| `ext/llama.cpp` submodule | `git submodule update --init ext/llama.cpp` |
| curl | For `make download-nomic` |

---

## Quick Start

```bash
# 1. Clone
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot

# 2. Build (standard — no C++ dependencies)
make build

# 3. Run interactive setup wizard
./gorkbot.sh setup

# 4. Start
./gorkbot.sh
```

For local embedding support (semantic MEL retrieval):

```bash
# Clone with submodule
git clone --recurse-submodules https://github.com/velariumai/gorkbot.git
cd gorkbot

# Build with llamacpp embedding engine
make build-llm

# Download Nomic embedding model (~274 MB)
make download-nomic

./gorkbot.sh
```

For a complete walkthrough, see [GETTING_STARTED.md](GETTING_STARTED.md).

---

## Build Targets

```bash
make build           # host OS → bin/gorkbot (standard, no CGO)
make build-web       # web UI → bin/gorkweb
make build-linux     # Linux amd64 → bin/gorkbot-linux
make build-android   # Android arm64 → bin/gorkbot-android
make build-windows   # Windows amd64 → bin/gorkbot.exe
make build-llm       # with llamacpp embedding → bin/gorkbot (requires CGO)
make build-llm-bridge # compile C++ bridge only (internal/llm/libgorkbot_llm.a)
make download-nomic  # download Nomic embed model to ~/.cache/llama.cpp/
make install-global  # build-llm + download-nomic + install 'gork' to ~/bin
make clean           # remove bin/
make clean-llm       # remove llm bridge artifacts
```

The `install-global` target builds the full stack (C++ bridge + llamacpp binary + Nomic model) and installs a `gork` shorthand launcher to `~/bin`. No root required. Works in Termux and standard Linux.

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
./gorkbot.sh --trace              # write JSONL execution trace to traces dir

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

## Commands Reference

All commands are entered in the TUI input field prefixed with `/`.

| Command | Description |
|---------|-------------|
| `/help` | Show all commands |
| `/about` | System overview, intelligence stack, platform info |
| `/clear` | Reset conversation and screen |
| `/model [primary\|consultant] [id]` | View or switch model |
| `/key <provider> <key>` | Set API key for a provider at runtime |
| `/tools [stats\|audit\|errors]` | List tools; `stats` shows analytics; `audit` shows log |
| `/permissions [list\|reset\|reset <tool>]` | Manage tool permissions |
| `/rules [list\|add\|remove]` | Fine-grained glob-pattern permission rules |
| `/context` | Show context window usage breakdown |
| `/cost` | Show session API cost estimate |
| `/mode [normal\|plan\|auto]` | View or switch execution mode |
| `/compact [hint]` | Compress context to save tokens |
| `/rewind [last\|<id>]` | Restore to a session checkpoint |
| `/export [md\|txt\|pdf] [file]` | Export conversation to file |
| `/save [name]` | Save session (auto-named if no name given) |
| `/resume <name>\|list` | Restore a saved session |
| `/chat [save\|load\|list\|delete] [name]` | Manage conversation sessions |
| `/rename <name>` | Rename current session |
| `/skills [list\|help]` | List and invoke skill definitions |
| `/hooks [list\|dir]` | List lifecycle hook scripts |
| `/theme [light\|dark\|auto\|<name>]` | Change UI theme |
| `/mcp [status\|config\|reload]` | MCP server status and management |
| `/a2a` | A2A gateway status |
| `/telegram` | Telegram bot status |
| `/schedule` | List scheduled tasks |
| `/commands` | List user-defined slash commands |
| `/share [start\|stop]` | Start/stop SSE session sharing |
| `/rate <1-5>` | Rate last response (trains adaptive router) |
| `/debug` | Toggle debug mode (raw AI output) |
| `/think [budget]` | Toggle extended thinking; set token budget |
| `/auth [refresh\|status]` | API credential status |
| `/settings` | Open settings overlay (`Ctrl+G`) |
| `/self <schema\|check\|evolve\|fix>` | SENSE self-knowledge layer commands |
| `/env [refresh]` | Show host environment snapshot |
| `/version` | Show build version and system info |
| `/bug` | Open GitHub issue template |
| `/quit` | Exit gracefully |

Skill definitions and user-defined commands are also invokable as slash commands (e.g., `/code-review`).

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
├── cmd/
│   ├── gorkbot/          Entry point (main.go + helpers)
│   └── gorkweb/          Web UI server entry point
├── internal/
│   ├── engine/           Orchestration engine, streaming, intelligence, CCI integration
│   ├── llm/              Local LLM embedding bridge (llamacpp build tag)
│   ├── platform/         OS/environment abstraction, Version constant (4.7.0)
│   ├── tui/              Full-screen terminal UI (Bubble Tea)
│   └── webui/            Browser-based UI server
├── pkg/
│   ├── a2a/              Agent-to-Agent HTTP gateway
│   ├── adaptive/         ARC Router + MEL + CCI (consolidated intelligence layer)
│   ├── ai/               AI provider implementations (grok, gemini, anthropic, openai, minimax, moonshot, openrouter)
│   ├── billing/          Per-model cost tracking, usage_history.jsonl
│   ├── channels/         Integrations (telegram, discord, bridge)
│   ├── collab/           SSE relay server + observer client
│   ├── colony/           Colony debate (multi-perspective reasoning)
│   ├── commands/         Slash command registry + OrchestratorAdapter
│   ├── config/           GORKBOT.md hierarchical config, AppStateManager
│   ├── dag/              DAG execution engine (parallel tasks + rollback)
│   ├── discovery/        Live model discovery (all providers)
│   ├── embeddings/       Embedder interface (local llamacpp + cloud backends)
│   ├── hooks/            Lifecycle hook script manager
│   ├── mcp/              MCP protocol client (stdio, JSON-RPC 2.0)
│   ├── memory/           MemoryManager, AgeMem, Engrams, GoalLedger, UnifiedMemory
│   ├── persist/          SQLite persistence (conversation + tool analytics)
│   ├── pipeline/         Agentic pipeline execution
│   ├── process/          Managed background process system
│   ├── providers/        KeyStore, ProviderManager (all providers), global singleton
│   ├── registry/         Model registry (dynamic model list)
│   ├── router/           Adaptive router, FeedbackManager
│   ├── scheduler/        Cron-style task scheduler with SQLite store
│   ├── security/         Key manager (AES-GCM encryption for .env values)
│   ├── sense/            SENSE module (tracer, sanitizer, AgeMem, engrams, stabilizer, LIE)
│   ├── session/          Checkpoint, exporter, loader, workspace
│   ├── skills/           Skill definition loader (YAML frontmatter markdown)
│   ├── subagents/        Sub-agent system (spawn, delegate, worktree isolation)
│   ├── theme/            JSON-based theme system (5 built-ins + custom)
│   ├── tools/            Tool registry + 196+ tool implementations
│   ├── tui/              Stylist (Lip Gloss style helpers used by engine)
│   ├── usercommands/     User-defined slash command loader
│   ├── vectorstore/      Conversation-level semantic vector store (RAG)
│   └── vision/           Vision pipeline (ADB, MediaProjection, Grok Vision API)
├── ext/llama.cpp         C++ LLM bridge (submodule, used by make build-llm)
├── plugins/python/       Python plugin bridge (custom plugins)
├── scripts/              Build and setup scripts
├── configs/              Example configuration files (mcp.json, etc.)
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
| `cci/` | `~/.config/gorkbot/` | CCI three-tier memory files |
| `hooks/` | `~/.config/gorkbot/` | Lifecycle hook scripts |
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
| `MOONSHOT_API_KEY` | Moonshot API key |
| `OPENROUTER_API_KEY` | OpenRouter API key (accesses 400+ models) |
| `GORKBOT_PRIMARY_MODEL` | Override primary model ID |
| `GORKBOT_CONSULTANT_MODEL` | Override consultant model ID |
| `GORKBOT_TOOL_PACKS` | Comma-separated list of tool packs to activate (`ALL` loads all) |
| `GORKBOT_AUTO_REBUILD` | Set to `1` to auto-compile new dynamic tools on exit |
| `SHODAN_API_KEY` | Required for `shodan_query` security tool |
| `TERMUX_VERSION` | Detected automatically; enables Termux-specific paths |

---

## Documentation

| Document | Description |
|----------|-------------|
| [GETTING_STARTED.md](GETTING_STARTED.md) | Installation, API keys, first run, TUI walkthrough |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Full system architecture — engine, intelligence, memory, TUI |
| [docs/TOOLS_REFERENCE.md](docs/TOOLS_REFERENCE.md) | Complete tool reference with parameters |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | All config files, env vars, flags, and paths |
| [docs/PROVIDERS.md](docs/PROVIDERS.md) | Provider guide — models, API keys, hot-swap, dynamic discovery |

---

## License

MIT — see [LICENSE](LICENSE) for details.

Gorkbot is an independent open-source project and is not affiliated with xAI, Google, Anthropic, OpenAI, MiniMax, Moonshot, or OpenRouter.
