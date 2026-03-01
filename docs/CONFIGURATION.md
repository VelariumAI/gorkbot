# Gorkbot Configuration Reference

**Version:** 3.4.0

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
16. [Logging](#16-logging)
17. [Execution Traces](#17-execution-traces)

---

## 1. Configuration File Locations

Gorkbot respects platform conventions for config, log, and data directories.

### Linux (XDG Compliant)

| Type | Path |
|------|------|
| Config | `~/.config/gorkbot/` |
| Logs | `~/.local/share/gorkbot/logs/` |
| Traces | `~/.local/share/gorkbot/traces/` |

If `$XDG_CONFIG_HOME` is set, config lives in `$XDG_CONFIG_HOME/gorkbot/`.
If `$XDG_DATA_HOME` is set, logs and traces live in `$XDG_DATA_HOME/gorkbot/`.

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
├── api_keys.json           Provider API key store (0600)
├── app_state.json          Persisted model + tool group preferences
├── active_theme            Active theme name (plain text)
├── mcp.json                MCP server configurations
├── telegram.json           Telegram bot configuration
├── tool_permissions.json   Persistent tool permission store
├── dynamic_tools.json      Hot-loaded tools created via create_tool
├── feedback.jsonl          Adaptive router feedback history
├── usage_history.jsonl     Per-model billing history (all-time)
├── vector_store.json       MEL heuristic vector store
├── themes/                 Custom JSON theme files
│   └── *.json
├── hooks/                  Lifecycle hook shell scripts
│   ├── pre_turn.sh
│   ├── post_turn.sh
│   ├── pre_tool.sh
│   ├── post_tool.sh
│   └── session_start.sh
├── skills/                 Skill definition markdown files
│   └── *.md
└── cci/                    CCI three-tier memory
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

The `.env` file lives in the project root (next to `gorkbot.sh`). It is loaded by the `gorkbot.sh` launcher and by `main.go` via `loadEnv()`. The file is gitignored — it will never be committed.

```bash
cp .env.example .env
```

### Format

```env
# Lines beginning with # are comments
# Empty lines are ignored
KEY=value

# Encrypted values (encrypted in-place by the security module)
KEY=ENC_base64-encoded-ciphertext
```

Values prefixed `ENC_` are decrypted at startup using the AES-GCM key stored in the OS keyring or a local key file. The key manager is initialized from `pkg/security`.

### Example .env

```env
# ─── Required — at least one AI provider ──────────────────────────────────────
XAI_API_KEY=xai-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
GEMINI_API_KEY=AIzaSyxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# ─── Optional additional providers ────────────────────────────────────────────
ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
MINIMAX_API_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# ─── Model overrides (bypass dynamic selection) ───────────────────────────────
# GORKBOT_PRIMARY_MODEL=grok-3-mini
# GORKBOT_CONSULTANT_MODEL=gemini-2.0-flash

# ─── Auto-rebuild on exit when new tools are created ─────────────────────────
# GORKBOT_AUTO_REBUILD=1

# ─── Optional: third-party integrations ──────────────────────────────────────
# SHODAN_API_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
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
| `XAI_API_KEY` | xAI (Grok) | Required for Grok as primary |
| `GEMINI_API_KEY` | Google Gemini | Required for Gemini specialist |
| `ANTHROPIC_API_KEY` | Anthropic | Optional |
| `OPENAI_API_KEY` | OpenAI | Optional |
| `MINIMAX_API_KEY` | MiniMax | Optional |

### Model Selection

| Variable | Purpose |
|----------|---------|
| `GORKBOT_PRIMARY_MODEL` | Override primary model ID (skips dynamic selection) |
| `GORKBOT_CONSULTANT_MODEL` | Override specialist model ID |

### Build & Runtime

| Variable | Purpose |
|----------|---------|
| `GORKBOT_AUTO_REBUILD` | Set to `1` to auto-compile dynamic tools on session exit |
| `TERMUX_VERSION` | Detected by Termux; triggers Termux-specific paths |
| `XDG_CONFIG_HOME` | Override XDG config directory (Linux) |
| `XDG_DATA_HOME` | Override XDG data directory (Linux) |
| `APPDATA` | Windows config directory |
| `LOCALAPPDATA` | Windows local data directory |

### Third-Party Tools

| Variable | Purpose |
|----------|---------|
| `SHODAN_API_KEY` | Required for `shodan_query` tool |

---

## 4. CLI Flags

All flags are passed after `./gorkbot.sh` (the script passes them through to the binary).

```
./gorkbot.sh [flags]
```

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
| `--trace` | bool | false | Write JSONL execution trace to traces dir |

### Session Sharing

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--share` | bool | false | Start SSE relay and print observer URL |
| `--join <host:port>` | string | — | Observe a shared session (observer-only mode) |

### A2A Gateway

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--a2a` | bool | false | Enable A2A HTTP gateway |
| `--a2a-addr <addr>` | string | `127.0.0.1:18890` | A2A listen address |

### Subcommands (handled before flags)

| Subcommand | Description |
|------------|-------------|
| `setup` / `config` / `configure` | Run interactive setup wizard |
| `status` | Show configuration status |
| `help` / `--help` / `-h` | Show help |

---

## 5. api_keys.json — Provider Key Store

**Path:** `~/.config/gorkbot/api_keys.json`
**Permissions:** 0600 (owner read/write only)

Managed by `pkg/providers.KeyStore`. Do not edit manually — use the `/key` command or setup wizard instead.

```json
{
  "keys": {
    "xai": "xai-xxxxxxxx",
    "google": "AIzaSyxx",
    "anthropic": "sk-ant-xx",
    "openai": "sk-xx",
    "minimax": "xx"
  },
  "status": {
    "xai": "valid",
    "google": "valid",
    "anthropic": "unset",
    "openai": "unset",
    "minimax": "unset"
  }
}
```

Provider name constants: `xai`, `google`, `anthropic`, `openai`, `minimax`.

The KeyStore seeds from environment variables on initialization. If an env var key is present, it takes priority over the stored value.

---

## 6. app_state.json — Persisted Application State

**Path:** `~/.config/gorkbot/app_state.json`
**Permissions:** 0600

Managed by `pkg/config.AppStateManager`. Automatically updated when you switch models via the TUI or `/model` command and when you enable/disable tool categories.

```json
{
  "primary_provider": "xai",
  "primary_model": "grok-3",
  "secondary_provider": "google",
  "secondary_model": "gemini-2.0-flash",
  "secondary_auto": false,
  "disabled_categories": ["security", "pentest"]
}
```

| Field | Description |
|-------|-------------|
| `primary_provider` | Provider ID for primary model |
| `primary_model` | Model ID for primary |
| `secondary_provider` | Provider ID for specialist |
| `secondary_model` | Model ID for specialist |
| `secondary_auto` | If true, specialist is auto-selected per task |
| `disabled_categories` | Tool categories disabled via settings |

---

## 7. mcp.json — MCP Server Configuration

**Path:** `~/.config/gorkbot/mcp.json`

Configures external MCP (Model Context Protocol) servers. Each server is started as a subprocess when Gorkbot launches.

```json
{
  "servers": [
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/user/projects"],
      "env": {}
    },
    {
      "name": "github",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_xxxx"
      }
    },
    {
      "name": "custom-server",
      "command": "/usr/local/bin/my-mcp-server",
      "args": ["--port", "3000"],
      "env": {
        "MY_API_KEY": "secret"
      }
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `name` | Server identifier (used as tool prefix: `mcp_<name>_<toolname>`) |
| `command` | Executable to start |
| `args` | Command line arguments |
| `env` | Additional environment variables for the subprocess |

MCP servers communicate via JSON-RPC 2.0 over stdin/stdout. Tools from each server are registered in the tool registry with the prefix `mcp_<server-name>_`.

View connected servers and their tools: `/mcp status`

---

## 8. telegram.json — Telegram Bot

**Path:** `~/.config/gorkbot/telegram.json`

```json
{
  "token": "1234567890:AAFxxxxxxxxxxxxxxxxxxxxx",
  "allowed_user_ids": [123456789],
  "allowed_usernames": ["yourusername"],
  "welcome_message": "Hello! I'm Gorkbot. Ask me anything."
}
```

| Field | Description |
|-------|-------------|
| `token` | Telegram bot token from @BotFather |
| `allowed_user_ids` | Numeric Telegram user IDs allowed to use the bot |
| `allowed_usernames` | Telegram usernames allowed to use the bot |
| `welcome_message` | Message sent on `/start` |

The bot is started automatically on Gorkbot launch if `token` is set. Stop it with `defer tgMgr.Stop()` on exit.

Check status: `/telegram`

---

## 9. tool_permissions.json — Tool Permissions

**Path:** `~/.config/gorkbot/tool_permissions.json`
**Permissions:** 0600

Managed by `pkg/tools.PermissionManager`. Never edit manually — use `/permissions` commands.

```json
{
  "permissions": {
    "read_file": "always",
    "list_directory": "always",
    "file_info": "always",
    "git_status": "always",
    "git_diff": "always",
    "git_log": "always",
    "bash": "once",
    "write_file": "once",
    "delete_file": "once",
    "git_push": "never"
  },
  "version": "1.0"
}
```

Only `always` and `never` permissions are persisted. `session` permissions live in memory only. Unset tools default to `once` (prompt every time).

**Commands:**

```
/permissions                    # show all
/permissions reset              # reset all to "once"
/permissions reset <tool>       # reset one tool to "once"
```

---

## 10. dynamic_tools.json — Hot-Loaded Tools

**Path:** `~/.config/gorkbot/dynamic_tools.json`

Written by the `create_tool` tool. Loaded on startup (and immediately after creation) as `DynamicTool` instances.

```json
{
  "tools": [
    {
      "name": "count_words",
      "description": "Count words in a file",
      "category": "file",
      "command": "wc -w {{path}}",
      "parameters": {
        "path": {
          "type": "string",
          "description": "File path",
          "required": true
        }
      },
      "requires_permission": false,
      "default_permission": "always",
      "created_at": "2025-11-15T10:30:00Z"
    }
  ]
}
```

To compile these permanently into the binary:

```bash
GORKBOT_AUTO_REBUILD=1 ./gorkbot.sh   # auto on exit
# or
go build -o bin/gorkbot ./cmd/gorkbot/
```

---

## 11. Theme Configuration

### Built-In Themes

Activated with `/theme <name>`:

| Name | Style |
|------|-------|
| `dracula` | Dark purple/pink (default) |
| `nord` | Dark arctic blue |
| `gruvbox` | Dark warm brown/orange |
| `solarized` | Dark cool teal |
| `monokai` | Dark vivid green/yellow |

Additionally, `/theme dark` selects the default dark theme and `/theme light` selects a light variant.

### Active Theme

**Path:** `~/.config/gorkbot/active_theme` (plain text, just the theme name)

### Custom Themes

Place a JSON file in `~/.config/gorkbot/themes/` and activate it by filename (without `.json`):

```json
{
  "name": "my-theme",
  "background": "#1a1a2e",
  "foreground": "#e0e0e0",
  "primary": "#7c3aed",
  "secondary": "#10b981",
  "accent": "#f59e0b",
  "error": "#ef4444",
  "warning": "#f97316",
  "success": "#22c55e",
  "muted": "#6b7280",
  "border": "#374151",
  "code_background": "#111827",
  "consultant_border": "#8b5cf6"
}
```

```
/theme my-theme
```

---

## 12. GORKBOT.md — Project Configuration

Gorkbot looks for a `GORKBOT.md` file starting from the current working directory and traversing upward. This file provides project-specific instructions loaded into the system prompt (similar to Claude Code's `CLAUDE.md`).

```markdown
# My Project — Gorkbot Configuration

## Always do
- Prefer functional programming patterns
- Use Go 1.24 standard library features

## Never do
- Modify production database credentials
- Push directly to main branch

## Project structure
- API routes: `internal/api/`
- Database models: `pkg/models/`
- Tests: `*_test.go` alongside source files

## Build command
`make build`
```

The loader (`pkg/config.Loader`) merges configurations from all `GORKBOT.md` files found in the hierarchy, with closer files taking precedence.

---

## 13. Hooks Configuration

Lifecycle hooks are shell scripts placed in `~/.config/gorkbot/hooks/`. They run at defined events during the orchestrator lifecycle.

### Available Hook Events

| Filename | Trigger |
|----------|---------|
| `session_start.sh` | When Gorkbot starts |
| `session_end.sh` | When Gorkbot exits |
| `pre_turn.sh` | Before each AI turn |
| `post_turn.sh` | After each AI turn |
| `pre_tool.sh` | Before each tool execution |
| `post_tool.sh` | After each tool execution |

### Hook Environment Variables

Hooks receive the following environment variables:

| Variable | Description |
|----------|-------------|
| `GORKBOT_TURN` | Current turn number |
| `GORKBOT_TOOL_NAME` | Tool name (for pre/post_tool) |
| `GORKBOT_TOOL_RESULT` | Tool result (for post_tool) |
| `GORKBOT_SESSION_ID` | Current session ID |

### Example Hook

```bash
#!/bin/bash
# ~/.config/gorkbot/hooks/pre_tool.sh
# Log all tool executions to a file

echo "$(date -Iseconds) TOOL: $GORKBOT_TOOL_NAME" >> ~/gorkbot-tool-log.txt
```

Make scripts executable:

```bash
chmod +x ~/.config/gorkbot/hooks/*.sh
```

View hooks: `/hooks list`

---

## 14. CCI Configuration Files

CCI (Codified Context Infrastructure) files live under `~/.config/gorkbot/cci/`. They are seeded on first startup and can be edited manually or via the `mcp_context_update_subsystem` tool.

### Tier 1 — Hot Memory

**`~/.config/gorkbot/cci/hot/CONVENTIONS.md`** — Universal conventions always injected into the system prompt. Edit this to add project-wide coding standards, naming conventions, or workflow rules.

**`~/.config/gorkbot/cci/hot/SUBSYSTEM_POINTERS.md`** — Index of available Tier 3 subsystem documents. Auto-updated when new subsystem docs are created.

### Tier 2 — Specialist Personas

**`~/.config/gorkbot/cci/specialists/<domain>.md`** — One file per specialist domain. Domains are matched by ARC trigger table patterns (file paths → domain labels). Pre-populated with failure mode tables and known pitfalls.

Common domain names: `security`, `frontend`, `backend`, `devops`, `data_science`, `mobile`.

### Tier 3 — Cold Memory (Living Docs)

**`~/.config/gorkbot/cci/docs/<subsystem>.md`** — One file per subsystem. Queried via `mcp_context_get_subsystem`. Updated via `mcp_context_update_subsystem`.

Example subsystem names: `auth_system`, `database_layer`, `api_routes`, `deployment_pipeline`.

---

## 15. Scheduler Storage

**Path:** `~/.config/gorkbot/scheduler.db` (SQLite)

Stores all scheduled tasks. Managed automatically — do not edit manually.

```sql
-- Schema
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    cron TEXT NOT NULL,
    task TEXT NOT NULL,
    status TEXT NOT NULL,   -- active, paused, completed
    last_run DATETIME,
    next_run DATETIME,
    created_at DATETIME
);
```

View tasks: `/schedule` or `list_scheduled_tasks` tool.

---

## 16. Logging

Gorkbot writes structured JSON logs via `log/slog`.

### Log File Location

| Platform | Path |
|----------|------|
| Linux | `~/.local/share/gorkbot/logs/gorkbot.json` |
| macOS | `~/Library/Logs/gorkbot/gorkbot.json` |
| Windows | `%LOCALAPPDATA%\gorkbot\logs\gorkbot.json` |
| Termux | `~/.gorkbot/logs/gorkbot.json` |

### Log Format

Each line is a JSON object:

```json
{"time":"2025-11-15T10:30:01Z","level":"INFO","msg":"Gorkbot initialized","os":"linux","arch":"arm64","termux":true}
{"time":"2025-11-15T10:30:01Z","level":"INFO","msg":"Tool system initialized","tool_count":162}
{"time":"2025-11-15T10:30:05Z","level":"INFO","msg":"Executing AI turn","turn":1,"history_messages":3}
{"time":"2025-11-15T10:30:06Z","level":"INFO","msg":"Tool executed","tool":"read_file","duration_ms":12,"success":true}
```

### Reading Logs

```bash
# Stream live logs
tail -f ~/.local/share/gorkbot/logs/gorkbot.json | jq

# Filter for errors
cat ~/.local/share/gorkbot/logs/gorkbot.json | jq 'select(.level == "ERROR")'

# Filter for tool calls
cat ~/.local/share/gorkbot/logs/gorkbot.json | jq 'select(.msg == "Tool executed")'
```

---

## 17. Execution Traces

When `--trace` is enabled, a JSONL trace file is written to the traces directory.

### Trace File Location

| Platform | Path |
|----------|------|
| Linux | `~/.local/share/gorkbot/traces/<timestamp>.jsonl` |
| macOS | `~/Library/Logs/gorkbot/traces/<timestamp>.jsonl` |
| Termux | `~/.gorkbot/traces/<timestamp>.jsonl` |

### Trace Entry Format

```json
{"type":"turn_start","turn":1,"prompt":"Explain this codebase","timestamp":"2025-11-15T10:30:05Z"}
{"type":"arc_route","classification":"WorkflowDirect","budget":{"max_tokens":4096,"max_tool_calls":4},"timestamp":"2025-11-15T10:30:05Z"}
{"type":"tool_call","tool":"read_file","params":{"path":"README.md"},"timestamp":"2025-11-15T10:30:06Z"}
{"type":"tool_result","tool":"read_file","success":true,"duration_ms":12,"timestamp":"2025-11-15T10:30:06Z"}
{"type":"turn_end","turn":1,"total_tokens":1234,"cost_usd":0.0012,"timestamp":"2025-11-15T10:30:08Z"}
```

### Analyzing Traces

```bash
# List all trace files
ls -lh ~/.local/share/gorkbot/traces/

# Parse a trace
cat ~/.local/share/gorkbot/traces/2025-11-15T103005.jsonl | jq 'select(.type == "tool_call")'

# Count tool calls per type
cat *.jsonl | jq -r '.tool // empty' | sort | uniq -c | sort -rn
```
