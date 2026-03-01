# Getting Started with Gorkbot

This guide takes you from a fresh clone to a fully operational Gorkbot session in about five minutes. It then walks through the TUI, all keyboard shortcuts, slash commands, key concepts, and common workflows.

---

## Table of Contents

1. [Requirements](#1-requirements)
2. [Installation](#2-installation)
3. [API Key Setup](#3-api-key-setup)
4. [Building](#4-building)
5. [First Run](#5-first-run)
6. [TUI Overview](#6-tui-overview)
7. [Keyboard Shortcuts](#7-keyboard-shortcuts)
8. [Slash Commands](#8-slash-commands)
9. [Execution Modes](#9-execution-modes)
10. [Tool Permissions](#10-tool-permissions)
11. [Session Management](#11-session-management)
12. [Themes](#12-themes)
13. [One-Shot Mode](#13-one-shot-mode)
14. [Global Install](#14-global-install)
15. [Troubleshooting](#15-troubleshooting)

---

## 1. Requirements

- **Go 1.24.2+** — `go version` to confirm
- **Git** — for cloning and git tools
- **xAI API key** — required for Grok (primary AI) — get one at [console.x.ai](https://console.x.ai)
- **Google Gemini API key** — required for Gemini (specialist AI) — get one at [aistudio.google.com](https://aistudio.google.com/apikey)
- Optional: Anthropic, OpenAI, and MiniMax keys for additional providers

Gorkbot works with only xAI or only Google keys; having both enables full dual-model orchestration.

---

## 2. Installation

```bash
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
```

No external dependencies need to be installed manually — `go mod tidy` (or the build step below) handles everything.

---

## 3. API Key Setup

### Option A — Interactive Setup Wizard (Recommended)

```bash
./gorkbot.sh setup
```

The wizard prompts for each API key one at a time:

```
╔════════════════════════════════════════════════════════════╗
║           Welcome to Gorkbot Setup Wizard                  ║
╚════════════════════════════════════════════════════════════╝

  Step 1: xAI API Key (for Grok)
──────────────────────────────────────────────────────────────
  📍 Get your key from: https://console.x.ai/
  📋 Paste your xAI API key: _
```

Keys are written to `.env` in the project root and `.env` is gitignored — they will never be committed.

### Option B — Manual `.env` File

Copy the example and fill in your keys:

```bash
cp .env.example .env
nano .env   # or your preferred editor
```

```env
# Required — at least one of these two
XAI_API_KEY=xai-xxxxxxxxxxxxxxxxxxxxxxxx
GEMINI_API_KEY=AIzaxxxxxxxxxxxxxxxxxxxxxxxx

# Optional additional providers
ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxxxxxxxxxx
OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
MINIMAX_API_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Optional model overrides (bypass dynamic selection)
# GORKBOT_PRIMARY_MODEL=grok-3-mini
# GORKBOT_CONSULTANT_MODEL=gemini-2.0-flash
```

### Option C — Environment Variables

If you prefer to manage keys externally (e.g., from a secrets manager or shell profile):

```bash
export XAI_API_KEY="xai-..."
export GEMINI_API_KEY="AIza..."
./bin/gorkbot   # runs without the .env wrapper
```

### Option D — Runtime `/key` Command

After starting, add or change keys without restarting:

```
/key xai xai-your-new-key
/key google AIza-your-new-key
/key anthropic sk-ant-your-key
/key status
```

Keys set this way are persisted to `~/.config/gorkbot/api_keys.json`.

### Getting API Keys

**xAI (Grok):**
1. Sign in at [console.x.ai](https://console.x.ai)
2. Navigate to **API Keys**
3. Click **Create API Key**
4. Copy the key — it starts with `xai-`

**Google Gemini:**
1. Sign in at [aistudio.google.com/apikey](https://aistudio.google.com/apikey)
2. Click **Create API Key**
3. Select or create a project
4. Copy the key — it starts with `AIza`

**Anthropic (Claude):**
1. Sign in at [console.anthropic.com](https://console.anthropic.com)
2. Navigate to **API Keys**
3. Create a key — it starts with `sk-ant-`

**OpenAI:**
1. Sign in at [platform.openai.com](https://platform.openai.com)
2. Navigate to **API Keys**
3. Create a key — it starts with `sk-`

---

## 4. Building

```bash
make build
```

This produces `bin/gorkbot`. The `gorkbot.sh` launcher script automatically runs `bin/gorkbot` after sourcing `.env`.

Other targets:

```bash
make build-linux     # Linux amd64
make build-android   # Android/Termux arm64
make build-windows   # Windows amd64
make dist            # all platforms + release tarball in dist/
make clean           # remove bin/ and dist/
```

---

## 5. First Run

```bash
./gorkbot.sh
```

On first launch Gorkbot:
1. Detects your OS/environment (Linux, macOS, Windows, or Termux)
2. Creates config directories (`~/.config/gorkbot/`, log dir)
3. Loads `.env` and decrypts any `ENC_`-prefixed values
4. Initializes the provider manager and polls live model lists from all configured providers
5. Selects the best primary and specialist models automatically
6. Seeds CCI memory tiers with project conventions
7. Opens the full-screen TUI

To verify your configuration before starting the TUI:

```bash
./gorkbot.sh status
```

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Gorkbot Configuration Status
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  xAI (Grok):
    ✓ Configured (xai-xxxx...xxxx)

  Google Gemini:
    ✓ Configured (AIza...xxxx)

  ✓ All systems ready — run: ./gorkbot.sh
```

---

## 6. TUI Overview

The TUI is divided into three sections:

```
┌─────────────────────────────────────────────────────────────┐
│                    Conversation Area                         │
│  Gorkbot v3.4.0  ·  Chat  Tools  Models  Cloud Brains       │
│                                                              │
│  You: Hello, what can you help me with today?               │
│                                                              │
│  Gorkbot: I can help you with code, system tasks, research, │
│  file operations, git workflows, web requests, security      │
│  assessment, media processing, and much more. I have 150+   │
│  tools at my disposal. Just ask!                            │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│  ▶ Type your message...                                      │
├─────────────────────────────────────────────────────────────┤
│  gork-3 │ gemini-2-flash │ Normal │ 12% ctx │ $0.0032 │ main│
└─────────────────────────────────────────────────────────────┘
```

**Conversation area** — Rendered markdown with syntax-highlighted code blocks, collapsible reasoning frames, and consultant response boxes (bordered for visual distinction).

**Input area** — Single or multi-line (Alt+Enter) with slash command autocomplete.

**Status bar** — Shows primary model, consultant model, execution mode, context usage %, session cost estimate, and current git branch.

### Tabs

| Key | Tab |
|-----|-----|
| Default | Chat |
| `Ctrl+E` | Tools — browsable list of all registered tools with categories |
| `Ctrl+T` | Models — dual-pane model selection (primary / specialist) |
| `Ctrl+D` | Cloud Brains — live discovered models + agent delegation tree |
| `Ctrl+\` | Diagnostics — system snapshot, route decisions, memory state |

### Overlays

| Key | Overlay |
|-----|---------|
| `Ctrl+G` | Settings — model routing, verbosity, tool group enable/disable |
| `Ctrl+B` | Bookmarks — jump to bookmarked conversation points |

---

## 7. Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Alt+Enter` | Insert newline (compose multi-line prompts) |
| `PgUp` | Scroll conversation up |
| `PgDn` | Scroll conversation down |
| `Ctrl+C` / `Ctrl+Q` | Quit |
| `Ctrl+H` | Show keybinding help |
| `Ctrl+L` | Clear screen |
| `Ctrl+I` | Focus input field |
| `Ctrl+X` | Interrupt — cancel in-progress AI generation |
| `Ctrl+T` | Open model selection (dual-pane primary + specialist) |
| `Ctrl+G` | Open settings overlay |
| `Ctrl+P` | Cycle execution mode: Normal → Plan → Auto → Normal |
| `Ctrl+E` | Show tools panel |
| `Ctrl+D` | Open Cloud Brains / discovery tab |
| `Ctrl+R` | Fold / unfold reasoning frames |
| `Ctrl+B` | Open conversation bookmarks |
| `Ctrl+\` | System diagnostics view |
| `Esc` | Back / close current overlay |

**In model selection (Ctrl+T):**

| Key | Action |
|-----|--------|
| `Tab` | Switch between primary / specialist pane |
| `↑` / `↓` or `k` / `j` | Navigate model list |
| `Enter` | Select highlighted model |
| `r` | Refresh model lists from all providers |
| `p` | Cycle provider filter |
| `k` | Add / update API key for selected provider |
| `Esc` | Close model selection |

---

## 8. Slash Commands

Type any command in the input field. Commands starting with `/` are intercepted before being sent to the AI.

### Conversation

| Command | Description |
|---------|-------------|
| `/clear` | Reset conversation context and clear screen |
| `/compress` | Smart context compression (keeps recent + important) |
| `/compact [hint]` | Targeted compression — e.g., `/compact focus on errors` |
| `/export [markdown\|json\|plain] [file]` | Export full conversation |
| `/save <name>` | Save current session to named file |
| `/resume <name>` | Load a previously saved session |
| `/resume list` | List all saved sessions |
| `/chat save <name>` | Alias for `/save` |
| `/chat load <name>` | Alias for `/resume` |
| `/chat list` | List saved sessions |
| `/rename <name>` | Rename current session |
| `/rewind [last\|<id>]` | Restore to a previous checkpoint |

### Models & Providers

| Command | Description |
|---------|-------------|
| `/model` | Show current primary and specialist models |
| `/model primary <id>` | Switch primary model |
| `/model consultant <id>` | Switch specialist model |
| `/key <provider> <key>` | Set API key for a provider |
| `/key status` | Show key status for all providers |
| `/key validate <provider>` | Validate a provider's key |
| `/auth status` | Check credential status |
| `/auth refresh` | Refresh credentials |

### Tools & Permissions

| Command | Description |
|---------|-------------|
| `/tools` | List all registered tools by category |
| `/tools stats` | Tool usage analytics (call count, success rate, latency) |
| `/permissions` | Show all tool permissions |
| `/permissions reset` | Reset all permissions to "ask each time" |
| `/permissions reset <tool>` | Reset permission for a specific tool |
| `/rules list` | List glob-pattern permission rules |
| `/rules add allow <pattern>` | Permanently allow tools matching pattern |
| `/rules add deny <pattern>` | Permanently block tools matching pattern |
| `/rules remove allow <pattern>` | Remove an allow rule |

### Context & Cost

| Command | Description |
|---------|-------------|
| `/context` | Show context window % breakdown |
| `/cost` | Show session cost estimate by provider |
| `/mode [normal\|plan\|auto]` | View or switch execution mode |

### Integrations

| Command | Description |
|---------|-------------|
| `/mcp [status\|config]` | MCP server status + tool list |
| `/a2a` | A2A HTTP gateway status |
| `/telegram` | Telegram bot status |
| `/schedule` | List scheduled tasks |
| `/share [start\|stop]` | Start/stop SSE session sharing |

### Skills & User Commands

| Command | Description |
|---------|-------------|
| `/skills [list\|help]` | List skill definitions and their triggers |
| `/commands` | List user-defined slash commands |
| `/<skill-name> [args]` | Invoke a skill by name |
| `/<user-command> [args]` | Invoke a user-defined command |

### UI & System

| Command | Description |
|---------|-------------|
| `/theme [light\|dark\|auto\|<name>]` | Change theme |
| `/debug` | Toggle debug mode (shows raw AI tool JSON) |
| `/hooks [list\|dir]` | List lifecycle hook scripts |
| `/rate <1-5>` | Rate last response (trains adaptive router) |
| `/mouse` | Toggle mouse support |
| `/settings` | Open settings overlay (also Ctrl+G) |
| `/version` | Show version, OS, model selection info |
| `/help` | Show all commands |
| `/bug` | Open GitHub issue template |
| `/quit` | Exit gracefully |

---

## 9. Execution Modes

Gorkbot has three execution modes, switchable with `/mode` or `Ctrl+P`:

### Normal (default)
The AI processes your request and executes tools as needed in a standard agentic loop. Best for most tasks.

### Plan
The AI drafts a structured step-by-step plan before taking any action. Use this when you want to review and approve the approach before execution begins. Triggered automatically when a CCI knowledge gap is detected.

### Auto
Fully autonomous execution — the AI chains tools and sub-tasks without pausing for confirmation on individual steps. Best for long-running, well-defined workflows.

The current mode is shown in the status bar. The ARC Router can also override the effective mode based on task complexity and compute budget.

---

## 10. Tool Permissions

Every tool call that modifies state, executes shell commands, or accesses the network requires a permission check. There are four permission levels:

| Level | Behavior | Storage |
|-------|---------|---------|
| `always` | Permanently approved — no prompt | `tool_permissions.json` |
| `session` | Approved for this session only | In-memory only |
| `once` | Prompt every time (default) | — |
| `never` | Permanently blocked — no execution | `tool_permissions.json` |

When a tool requires permission, a centered overlay appears:

```
┌──────────────────────────────────────┐
│  Permission Request                  │
│                                      │
│  Tool: bash                          │
│  Command: ls -la /home               │
│                                      │
│  Allow this tool to execute?         │
│                                      │
│  ▶ [Always]  Grant permanent         │
│    [Session] Allow this session      │
│    [Once]    Ask next time (default) │
│    [Never]   Block permanently       │
│                                      │
│  ↑/↓ to select  ·  Enter to confirm  │
│  Esc to deny this execution          │
└──────────────────────────────────────┘
```

**Fine-grained rules** override individual tool permissions:

```
/rules add allow "read_*"        # allow all read tools permanently
/rules add deny "delete_*"       # block all delete tools permanently
/rules add ask "git_push"        # always prompt for git_push
```

**Disable entire tool categories** via `/settings → Tool Groups`.

**Recommended defaults:**
- Grant `always` to: `read_file`, `list_directory`, `file_info`, `git_status`, `git_diff`, `git_log`, `system_info`
- Keep as `once`: `bash`, `write_file`, `delete_file`, `git_commit`, `git_push`, `http_request`, `kill_process`

---

## 11. Session Management

### Checkpoints (automatic)

Gorkbot saves up to 20 checkpoints automatically as the conversation progresses. Restore any checkpoint with:

```
/rewind last          # restore the most recent checkpoint
/rewind <id>          # restore a specific checkpoint by ID
```

### Named Sessions (manual)

```
/save project-review        # save current conversation
/resume project-review      # restore it later
/resume list                # list all saved sessions
```

### Export

```
/export markdown             # export to timestamped .md file
/export json session.json    # export to specific file as JSON
/export plain                # plain text export
```

---

## 12. Themes

Gorkbot ships with five built-in themes and supports custom JSON themes.

**Built-in themes:**

| Theme | Command |
|-------|---------|
| Dracula (default dark) | `/theme dracula` |
| Nord | `/theme nord` |
| Gruvbox | `/theme gruvbox` |
| Solarized | `/theme solarized` |
| Monokai | `/theme monokai` |
| Light mode | `/theme light` |
| Dark mode | `/theme dark` |

**Custom themes** — place a JSON file in `~/.config/gorkbot/themes/` and activate with `/theme <name>` (the filename without `.json`).

The active theme is persisted to `~/.config/gorkbot/active_theme` and restored on next launch.

---

## 13. One-Shot Mode

Run Gorkbot non-interactively for scripting, CI/CD, and automation:

```bash
# Simple prompt
./gorkbot.sh -p "What Go version does this project use?"

# Read from stdin
cat README.md | ./gorkbot.sh --stdin -p "Summarize this document"
echo "Explain error handling in Go" | ./gorkbot.sh --stdin

# Write output to file
./gorkbot.sh -p "Write a Dockerfile for a Go app" --output Dockerfile

# Custom timeout (default 60s)
./gorkbot.sh -p "Complex architecture review" --timeout 120s

# Allow only specific tools (comma-separated)
./gorkbot.sh -p "Audit this directory" --allow-tools bash,read_file,list_directory,grep_content

# Block specific tools
./gorkbot.sh -p "Code review only" --deny-tools bash,write_file,delete_file,git_push

# Enable execution trace for debugging
./gorkbot.sh -p "Run a deployment check" --trace
```

---

## 14. Global Install

Install `gork` as a system-wide command (no root required on Termux/Linux):

```bash
make install-global
```

This installs the `gork` launcher to `~/bin/gork`. If `~/bin` is not on your `PATH`:

```bash
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

Then use anywhere:

```bash
gork                          # start TUI
gork -p "Quick question"      # one-shot
gork setup                    # run setup wizard
```

---

## 15. Troubleshooting

### "XAI_API_KEY is not set" or "GEMINI_API_KEY is not set"

```bash
# Re-run setup wizard
./gorkbot.sh setup

# Or set manually in .env
echo 'XAI_API_KEY=xai-your-key' >> .env
```

### Keys not loading

Always use the wrapper script — it sources `.env` before running the binary:

```bash
./gorkbot.sh   # ✓ sources .env
./bin/gorkbot  # ✗ does not source .env automatically
```

Or export keys in your shell:

```bash
export XAI_API_KEY="xai-..."
./bin/gorkbot
```

### TUI doesn't start / display issues

```bash
# Ensure the binary is built
make build

# Check Go dependencies
go mod tidy

# Try a different theme if colours look wrong
/theme dark

# Minimum recommended terminal: 80 columns × 24 rows
```

### API errors

| Error | Resolution |
|-------|-----------|
| `401 Unauthorized` | Check API key is complete and correct (no trailing spaces) |
| `429 Too Many Requests` | Rate limit hit — wait or upgrade API plan |
| `context deadline exceeded` | Increase timeout: `./gorkbot.sh -p "..." --timeout 120s` |
| `model not found` | Run `/model` to see available models; some require API plan upgrades |

### Tool permission prompt doesn't appear

Check if the tool already has an `always` or `never` permission:

```
/permissions
/permissions reset <tool-name>
```

### Context filling up

```
/compact                        # smart compression
/compress                       # alias
/clear                          # full reset (new conversation)
/context                        # show current usage
```

### Check logs

```bash
# Linux/Termux
tail -f ~/.gorkbot/logs/gorkbot.json | jq

# macOS
tail -f ~/Library/Logs/gorkbot/gorkbot.json | jq
```

### Debug raw AI output

```
/debug         # toggle debug mode — shows tool JSON blocks inline
```

---

## Next Steps

- Explore the [Tool Reference](docs/TOOLS_REFERENCE.md) to see all 150+ available tools
- Read the [Architecture Guide](docs/ARCHITECTURE.md) for a deep dive into the orchestration engine
- See [Configuration Guide](docs/CONFIGURATION.md) for all settings and environment variables
- Review the [Providers Guide](docs/PROVIDERS.md) for model selection and hot-swapping
- Check [SECURITY.md](docs/SECURITY.md) for key management best practices
