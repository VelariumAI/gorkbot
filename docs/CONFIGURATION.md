# Gorkbot Configuration Reference

**Version:** 4.7.0

This document covers every configuration file, environment variable, CLI flag, and platform-specific path used by Gorkbot.

---

## Table of Contents

1. [Configuration File Locations](#1-configuration-file-locations)
2. [.env File](#2-env-file)
3. [Environment Variables](#3-environment-variables)
4. [CLI Flags](#4-cli-flags)
5. [api_keys.json — Provider Key Store](#5-api_keysjson--provider-key-store)
6. [app_state.json — Persisted Application State](#6-app_statejson--persisted-application-state)
7. [mcp.json — MCP Server Configuration](#7-mcpjson--mcp-server-configuration)
8. [telegram.json — Telegram Bot](#8-telegramjson--telegram-bot)
9. [tool_permissions.json — Tool Permissions](#9-tool_permissionsjson--tool-permissions)
10. [dynamic_tools.json — Hot-Loaded Tools](#10-dynamic_toolsjson--hot-loaded-tools)
11. [Theme Configuration](#11-theme-configuration)
12. [GORKBOT.md — Project Configuration](#12-gorkbotmd--project-configuration)
13. [Hooks Configuration](#13-hooks-configuration)
14. [CCI Configuration Files](#14-cci-configuration-files)
15. [Scheduler Storage](#15-scheduler-storage)
16. [Logging and Traces](#16-logging-and-traces)
17. [Tool Pack Selection](#17-tool-pack-selection)

---

## 1. Configuration File Locations

Gorkbot respects platform conventions for config, log, and data directories. Paths are resolved by `internal/platform/env.go`.

### Linux (XDG Compliant)

| Type | Path |
|------|------|
| Config | `~/.config/gorkbot/` |
| Logs | `~/.local/share/gorkbot/logs/` |

If `$XDG_CONFIG_HOME` is set, config lives in `$XDG_CONFIG_HOME/gorkbot/`.
If `$XDG_DATA_HOME` is set, logs live in `$XDG_DATA_HOME/gorkbot/logs/`.

### macOS

| Type | Path |
|------|------|
| Config | `~/Library/Application Support/gorkbot/` |
| Logs | `~/Library/Logs/gorkbot/` |

### Windows

| Type | Path |
|------|------|
| Config | `%APPDATA%\gorkbot\` |
| Logs | `%LOCALAPPDATA%\gorkbot\logs\` |

### Android / Termux

| Type | Path |
|------|------|
| Config | `~/.config/gorkbot/` |
| Logs | `~/.gorkbot/logs/` |

Detection: `TERMUX_VERSION` env var is set, or `/data/data/com.termux/files/usr/bin/login` exists.

### Full Config Directory Tree

```
~/.config/gorkbot/
├── api_keys.json              Provider API key store (0600)
├── app_state.json             Persisted model + tool group preferences (0600)
├── active_theme               Active theme name (plain text)
├── mcp.json                   MCP server configurations
├── telegram.json              Telegram bot configuration
├── tool_permissions.json      Persistent tool permission store
├── dynamic_tools.json         Hot-loaded tools created via create_tool
├── feedback.jsonl             Adaptive router feedback history
├── usage_history.jsonl        Per-model billing history (all-time)
├── vector_store.json          MEL heuristic vector store (0600)
├── traces/                    SENSE trace files (daily JSONL rotation)
│   └── YYYY-MM-DD.jsonl
├── themes/                    Custom JSON theme files
│   └── *.json
├── hooks/                     Lifecycle hook shell scripts
│   ├── pre_turn.sh
│   ├── post_turn.sh
│   ├── pre_tool.sh
│   ├── post_tool.sh
│   └── session_start.sh
├── skills/                    Skill definition markdown files
│   └── *.md
└── cci/                       CCI three-tier memory
    ├── hot/
    │   ├── CONVENTIONS.md
    │   └── SUBSYSTEM_POINTERS.md
    ├── specialists/
    │   └── <domain>.md
    └── docs/
        └── <subsystem>.md
```

---

## 2. .env File

The `.env` file lives in the project root (next to `gorkbot.sh`). It is loaded by `gorkbot.sh` and by `main.go` via `loadEnv()`. The file is gitignored — it will never be committed.

```bash
cp .env.example .env
```

### Format

```env
# Lines beginning with # are comments
# Empty lines are ignored
KEY=value

# Encrypted values (AES-GCM encrypted in-place by the security module)
KEY=ENC_base64-encoded-ciphertext
```

Values prefixed `ENC_` are decrypted at startup using the AES-GCM key stored in the OS keyring or a local key file (`pkg/security`). The key manager is initialized before any provider is constructed.

### Example .env

```env
# ─── Required — at least one AI provider ─────────────────────────────────────
XAI_API_KEY=xai-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
GEMINI_API_KEY=AIzaSyxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# ─── Optional additional providers ───────────────────────────────────────────
ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
MINIMAX_API_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
MOONSHOT_API_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
OPENROUTER_API_KEY=sk-or-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# ─── Model overrides (bypass dynamic selection) ──────────────────────────────
# GORKBOT_PRIMARY_MODEL=grok-3-mini
# GORKBOT_CONSULTANT_MODEL=gemini-2.0-flash

# ─── Tool packs (default: core,dev,web,sys,agent,data,media,comm) ─────────────
# GORKBOT_TOOL_PACKS=ALL

# ─── Auto-rebuild on exit when new tools are created ─────────────────────────
# GORKBOT_AUTO_REBUILD=1

# ─── Optional: third-party integrations ──────────────────────────────────────
# SHODAN_API_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
# GITHUB_PERSONAL_ACCESS_TOKEN=ghp_xxxx
```

### Priority Order

When the same key is set in multiple places, the priority is:

1. Shell environment (`export KEY=val`) — highest priority
2. `.env` file — loaded only if key not already in environment
3. `api_keys.json` (KeyStore) — used as fallback for provider keys

---

## 3. Environment Variables

### AI Provider Keys

| Variable | Provider | Notes |
|----------|---------|-------|
| `XAI_API_KEY` | xAI (Grok) | Recommended primary; native function calling |
| `GEMINI_API_KEY` | Google Gemini | Recommended specialist |
| `ANTHROPIC_API_KEY` | Anthropic | Optional; extended thinking supported |
| `OPENAI_API_KEY` | OpenAI | Optional |
| `MINIMAX_API_KEY` | MiniMax | Optional |
| `MOONSHOT_API_KEY` | Moonshot | Optional |
| `OPENROUTER_API_KEY` | OpenRouter | Optional; access to 400+ models via single key |

### Model Selection

| Variable | Purpose |
|----------|---------|
| `GORKBOT_PRIMARY_MODEL` | Override primary model ID (skips dynamic selection) |
| `GORKBOT_CONSULTANT_MODEL` | Override specialist model ID |

### Build & Runtime

| Variable | Purpose |
|----------|---------|
| `GORKBOT_TOOL_PACKS` | Comma-separated tool packs to load; `ALL` loads every pack |
| `GORKBOT_AUTO_REBUILD` | Set to `1` to auto-compile dynamic tools on session exit |
| `TERMUX_VERSION` | Detected by Termux; triggers Termux-specific config paths |
| `XDG_CONFIG_HOME` | Override XDG config directory (Linux) |
| `XDG_DATA_HOME` | Override XDG data directory (Linux) |
| `APPDATA` | Windows config directory |
| `LOCALAPPDATA` | Windows local data directory |

### Third-Party Tools

| Variable | Purpose |
|----------|---------|
| `SHODAN_API_KEY` | Required for `shodan_query` security tool |
| `GITHUB_PERSONAL_ACCESS_TOKEN` | Optional; used by MCP GitHub server |
| `BRAVE_API_KEY` | Optional; used by MCP Brave Search server |

---

## 4. CLI Flags

All flags are passed after `./gorkbot.sh` (the script passes them through to the binary).

### Subcommands (handled before flags)

| Subcommand | Description |
|------------|-------------|
| `setup` / `config` / `configure` | Run interactive setup wizard |
| `status` | Show configuration status |
| `help` / `--help` / `-h` | Show help |

### One-Shot Mode

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-p <prompt>` | string | — | Execute a single prompt and exit |
| `--stdin` | bool | false | Read prompt from stdin |
| `--output <file>` | string | — | Write one-shot response to file |
| `--allow-tools <list>` | string | — | Comma-separated tool allow list |
| `--deny-tools <list>` | string | — | Comma-separated tool deny list |
| `--timeout <duration>` | duration | `60s` | Timeout for one-shot execution |

### Debug & Diagnostics

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--verbose-thoughts` | bool | false | Show consultant (Gemini) reasoning |
| `--watchdog` | bool | false | Enable orchestrator state debug log |
| `--trace` | bool | false | Write SENSE JSONL trace to `<configDir>/traces/` |

### Session Sharing

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--share` | bool | false | Start SSE relay and print observer URL |
| `--join <host:port>` | string | — | Observe a shared session (observer-only mode) |

### A2A Gateway

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--a2a` | bool | false | Enable A2A HTTP task gateway |
| `--a2a-addr <addr>` | string | `127.0.0.1:18890` | A2A listen address |

---

## 5. api_keys.json — Provider Key Store

**Path:** `~/.config/gorkbot/api_keys.json`
**Permissions:** 0600 (owner read/write only)

Managed by `pkg/providers.KeyStore`. Do not edit manually — use the `/key` command, setup wizard, or `/settings` API Providers tab instead.

```json
{
  "keys": {
    "xai": "xai-xxxxxxxx",
    "google": "AIzaSyxx",
    "anthropic": "sk-ant-xx",
    "openai": "sk-xx",
    "minimax": "xx",
    "moonshot": "xx",
    "openrouter": "sk-or-xx"
  },
  "status": {
    "xai": "valid",
    "google": "valid",
    "anthropic": "unset",
    "openai": "unset",
    "minimax": "unset",
    "moonshot": "unset",
    "openrouter": "unset"
  }
}
```

Provider ID constants: `xai`, `google`, `anthropic`, `openai`, `minimax`, `moonshot`, `openrouter`.

The KeyStore seeds from environment variables on initialization. Environment variable keys take priority over stored values.

### Key Management Commands

```
/key xai xai-your-new-key        # set xAI key
/key google AIza-your-new-key    # set Google key
/key anthropic sk-ant-your-key   # set Anthropic key
/key openai sk-your-key          # set OpenAI key
/key status                       # show all provider key statuses
/key validate xai                 # validate a specific key with live ping
```

---

## 6. app_state.json — Persisted Application State

**Path:** `~/.config/gorkbot/app_state.json`
**Permissions:** 0600

Managed by `pkg/config.AppStateManager`. Automatically updated when you switch models via the TUI (`Ctrl+T`) or `/model` command, toggle tool categories, or toggle providers.

```json
{
  "primary_provider": "xai",
  "primary_model": "grok-3",
  "secondary_provider": "google",
  "secondary_model": "gemini-2.0-flash",
  "secondary_auto": false,
  "disabled_categories": ["security"],
  "disabled_providers": []
}
```

| Field | Description |
|-------|-------------|
| `primary_provider` | Provider ID for the primary model |
| `primary_model` | Model ID for the primary |
| `secondary_provider` | Provider ID for the specialist |
| `secondary_model` | Model ID for the specialist |
| `secondary_auto` | If `true`, specialist is auto-selected per task (ignores secondary_model) |
| `disabled_categories` | Tool pack categories disabled via `/settings` |
| `disabled_providers` | Provider IDs disabled for the session (via Settings → API Providers) |

---

## 7. mcp.json — MCP Server Configuration

**Path:** `~/.config/gorkbot/mcp.json`

Configures external MCP (Model Context Protocol) servers. Each server is started as a subprocess when Gorkbot launches. A sample configuration file is provided in `configs/mcp.json` in the repository.

```json
{
  "servers": [
    {
      "name": "sequential-thinking",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"],
      "description": "Structured multi-step reasoning chains.",
      "disabled": false
    },
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/user/projects"],
      "description": "File I/O within allowed directories.",
      "disabled": false
    },
    {
      "name": "memory",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-memory"],
      "description": "Persistent cross-session key-value entity graph.",
      "disabled": false
    },
    {
      "name": "github",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_PERSONAL_ACCESS_TOKEN}"
      },
      "description": "GitHub API — issues, PRs, repos, code search.",
      "disabled": true
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Server identifier; tools are prefixed `mcp_<name>_<toolname>` |
| `command` | string | Executable to launch (e.g., `npx`, `python3`, absolute path) |
| `args` | array | Arguments passed to the command |
| `env` | object | Extra environment variables for the server process |
| `description` | string | Optional; displayed in `/mcp status` |
| `disabled` | bool | Skip this server on startup |
| `transport` | string | Currently only `stdio` is supported |

### MCP Commands

```
/mcp status          # list connected servers and their tools
/mcp config          # show config file path
/mcp reload          # stop all servers, re-read mcp.json, reconnect
```

### Tool Naming

Each MCP tool appears in the registry as `mcp_<server>_<toolname>`. For example, a tool named `read_file` on the `filesystem` server becomes `mcp_filesystem_read_file`.

---

## 8. telegram.json — Telegram Bot

**Path:** `~/.config/gorkbot/telegram.json`

```json
{
  "token": "1234567890:AAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "allowed_users": [123456789],
  "max_message_length": 4096
}
```

| Field | Description |
|-------|-------------|
| `token` | Telegram Bot API token (from @BotFather) |
| `allowed_users` | Array of Telegram user IDs allowed to interact |
| `max_message_length` | Maximum response chunk length (Telegram limit is 4096) |

The bot routes messages through the orchestrator with full tool access. Check status with `/telegram` in the TUI.

---

## 9. tool_permissions.json — Tool Permissions

**Path:** `~/.config/gorkbot/tool_permissions.json`
**Permissions:** 0600

Managed automatically by the permission manager. Each key is a tool name; each value is a permission level string.

```json
{
  "bash": "session",
  "write_file": "session",
  "delete_file": "never",
  "git_commit": "always",
  "nmap_scan": "never"
}
```

| Level | Meaning |
|-------|---------|
| `always` | Pre-approved permanently — no prompt shown |
| `session` | Approved for this session only (not persisted) |
| `once` | Prompts before every execution (default for destructive tools) |
| `never` | Blocked permanently |

**TUI management:**
```
/permissions list              # show all stored permissions
/permissions reset             # reset all to defaults
/permissions reset bash        # reset a single tool
```

---

## 10. dynamic_tools.json — Hot-Loaded Tools

**Path:** `~/.config/gorkbot/dynamic_tools.json`

Created and managed by the `create_tool` meta-tool. Contains tool definitions generated at runtime that are hot-loaded on startup. Each entry includes the tool name, description, bash command template, parameters, and category.

Do not edit manually — use `create_tool` or `modify_tool` instead.

If `GORKBOT_AUTO_REBUILD=1` is set, tools created this way are permanently compiled into the binary on session exit via `go build`.

---

## 11. Theme Configuration

### Built-In Themes

Five built-in themes: `dracula`, `nord`, `gruvbox`, `solarized`, `monokai`.

```
/theme dark           # switch to dark mode
/theme light          # switch to light mode
/theme dracula        # switch to specific built-in
/theme                # toggle between dark and light
```

The active theme name is stored in `~/.config/gorkbot/active_theme`.

### Custom Themes

Create a JSON file in `~/.config/gorkbot/themes/`:

```json
{
  "name": "my-theme",
  "background": "#1e1e2e",
  "foreground": "#cdd6f4",
  "primary": "#89b4fa",
  "secondary": "#cba6f7",
  "accent": "#a6e3a1",
  "error": "#f38ba8",
  "warning": "#fab387",
  "info": "#89dceb"
}
```

Apply with `/theme my-theme`.

---

## 12. GORKBOT.md — Project Configuration

Gorkbot loads hierarchical project instructions from `GORKBOT.md` files. These are plain markdown files that set project-specific conventions, rules, and context for the AI.

### Discovery Order (lowest → highest priority)

```
1. ~/.config/gorkbot/GLOBAL.md          — user-global preferences (all projects)
2. ~/.config/gorkbot/GLOBAL.local.md    — personal overrides (gitignored)
3. <project_root>/GORKBOT.md            — project-level (commit to repo)
4. <project_root>/GORKBOT.local.md      — personal per-project (gitignored)
5. <project_root>/.gorkbot/rules/*.md   — modular topic rules
```

The project root is found by walking up from the current working directory until a `GORKBOT.md` file (or `.git` directory) is found.

### Example GORKBOT.md

```markdown
# My Project

## Tech Stack
- Language: Go 1.24+
- Build: `make build` → `bin/gorkbot`
- Test: `go test ./...`

## Code Conventions
- All exported functions must have godoc comments
- Use `slog` for structured logging, not `fmt.Println`
- Error messages must be lowercase

## Never
- Commit `.env` files
- Use `panic()` in library code
- Leave TODO comments without an associated issue

## Important Files
- `cmd/gorkbot/main.go` — entry point, do not add business logic here
- `internal/engine/orchestrator.go` — central coordination, surgical edits only
```

---

## 13. Hooks Configuration

**Path:** `~/.config/gorkbot/hooks/`

Hooks are shell scripts that run at lifecycle events. They receive context via environment variables.

### Hook Events

| Hook File | Trigger | Environment Variables |
|-----------|---------|----------------------|
| `session_start.sh` | Session begins | `GORKBOT_SESSION_ID` |
| `pre_turn.sh` | Before each AI turn | `GORKBOT_PROMPT`, `GORKBOT_SESSION_ID` |
| `post_turn.sh` | After each AI turn | `GORKBOT_RESPONSE`, `GORKBOT_SESSION_ID` |
| `pre_tool.sh` | Before tool execution | `GORKBOT_TOOL_NAME`, `GORKBOT_TOOL_PARAMS` |
| `post_tool.sh` | After tool execution | `GORKBOT_TOOL_NAME`, `GORKBOT_TOOL_RESULT` |

### Example Hook

```bash
#!/bin/bash
# ~/.config/gorkbot/hooks/post_tool.sh
# Log all tool executions to a custom file
echo "$(date): $GORKBOT_TOOL_NAME" >> ~/gorkbot_tool_log.txt
```

Make hook scripts executable: `chmod +x ~/.config/gorkbot/hooks/*.sh`

Check installed hooks with `/hooks list` or `/hooks dir`.

---

## 14. CCI Configuration Files

**Path:** `~/.config/gorkbot/cci/`

The CCI (Codified Context Infrastructure) system uses plain markdown files for its three tiers.

### Tier 1 — Hot Memory

**Path:** `~/.config/gorkbot/cci/hot/`

Always-loaded into every system prompt. Edit these to set project-wide conventions:

- `CONVENTIONS.md` — coding style, naming conventions, patterns to follow/avoid
- `SUBSYSTEM_POINTERS.md` — maps subsystem names to specialist domains

### Tier 2 — Specialists

**Path:** `~/.config/gorkbot/cci/specialists/`

On-demand domain-specific personas, loaded when the ARC trigger table matches the current prompt to a domain. Each file is named `<domain>.md` (e.g., `tui.md`, `orchestrator.md`, `tool-system.md`).

Specialists are auto-synthesized by MEL's BifurcationAnalyzer when a domain repeatedly causes tool failure loops.

### Tier 3 — Cold Store

**Path:** `~/.config/gorkbot/cci/docs/`

On-demand subsystem specifications, queryable via the `mcp_context_*` tool suite. Each file is named `<subsystem>.md`.

Manage via tools (available when MCP gorkbot-context server is configured):
- `mcp_context_get_subsystem` — retrieve a subsystem spec
- `mcp_context_update_subsystem` — create or update a spec
- `mcp_context_list_subsystems` — list all available specs

---

## 15. Scheduler Storage

The task scheduler (`pkg/scheduler`) persists scheduled tasks to a SQLite database in the config directory. Scheduled tasks survive restarts.

```
/schedule             # list all active scheduled tasks
```

Tasks created via the `schedule_task` tool appear in `/schedule` with their next run time, cron expression, and status.

---

## 16. Logging and Traces

### Application Log

Structured JSON logging with `slog` to the platform log directory:

| Platform | Log Path |
|----------|----------|
| Linux (XDG) | `~/.local/share/gorkbot/logs/gorkbot.json` |
| macOS | `~/Library/Logs/gorkbot/gorkbot.json` |
| Windows | `%LOCALAPPDATA%\gorkbot\logs\gorkbot.json` |
| Android/Termux | `~/.gorkbot/logs/gorkbot.json` |

Falls back to stderr if the log file cannot be created.

### SENSE Trace Files

When SENSE tracing is active (always, unless the config directory is unavailable), daily-rotated JSONL files are written to `~/.config/gorkbot/traces/YYYY-MM-DD.jsonl`.

Enable the execution trace (full tool + AI turn log) with `--trace`:

```bash
./gorkbot.sh --trace
```

This writes a second JSONL trace to `~/.gorkbot/traces/<timestamp>.jsonl` (or platform-equivalent).

### Trace File Format

Each line in a SENSE trace file is a self-contained JSON object:

```json
{
  "ts": "2026-03-15T10:30:00.123456789Z",
  "sid": "abc123",
  "kind": "tool_success",
  "tool": "bash",
  "input": "{\"command\":\"ls -la\"}",
  "output": "total 48\\n...",
  "duration_ms": 45,
  "labels": ["success"]
}
```

| Field | Description |
|-------|-------------|
| `ts` | RFC3339Nano UTC timestamp |
| `sid` | Session ID (correlates events from the same run) |
| `kind` | Event type: `tool_success`, `tool_failure`, `hallucination`, `context_overflow`, `sanitizer_reject`, `provider_error`, `param_error` |
| `tool` | Tool name (for tool events) |
| `provider` | Provider ID (for provider events) |
| `input` | Truncated JSON of parameters (max 512 bytes) |
| `output` | Truncated result (max 512 bytes) |
| `error` | Error message (max 1024 bytes) |
| `duration_ms` | Wall-clock execution time |
| `labels` | Semantic tags for fast filtering |

---

## 17. Tool Pack Selection

Control which tools are loaded via the `GORKBOT_TOOL_PACKS` environment variable.

### Available Packs

| Pack | Tools | When to enable |
|------|-------|----------------|
| `core` | bash, structured_bash, file ops, python_execute | Always loaded |
| `dev` | git, worktrees, Docker, k8s, CI, code tools | Software development |
| `web` | web_fetch, HTTP, download, scrapling, search | Web research and APIs |
| `sec` | nmap, masscan, sqlmap, hydra, nuclei, burp, metasploit + 30 more | Security testing only |
| `media` | image, video, audio, OCR, TTS, Office docs | Media processing |
| `data` | CSV, plot, arxiv, jupyter, SQL, ML | Data analysis |
| `sys` | processes, system info, Android/Termux tools | System administration |
| `vision` | screen capture, visual analysis, OCR | Vision-based tasks |
| `agent` | create_tool, consultation, goals, scheduling, pipeline | Agent capabilities |
| `comm` | email, Slack, calendar, contact sync | Communication |

### Examples

```bash
# Default (most common tools, no security):
GORKBOT_TOOL_PACKS=core,dev,web,sys,agent,data,media,comm ./gorkbot.sh

# All tools including security (use with caution):
GORKBOT_TOOL_PACKS=ALL ./gorkbot.sh

# Minimal — core only:
GORKBOT_TOOL_PACKS=core ./gorkbot.sh

# Custom selection:
GORKBOT_TOOL_PACKS=core,dev,web,agent ./gorkbot.sh
```

You can also disable tool categories at runtime without restarting, using `/settings` → Tool Groups tab, or `/tools stats` to see what's loaded.
