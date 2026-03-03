# Getting Started with Gorkbot

**Version:** 3.5.1

This guide walks you through installation, API key configuration, your first conversation, and a tour of Gorkbot's most important features.

---

## Table of Contents

1. [Requirements](#1-requirements)
2. [Installation](#2-installation)
3. [API Key Setup](#3-api-key-setup)
4. [First Run](#4-first-run)
5. [TUI Overview](#5-tui-overview)
6. [Essential Commands](#6-essential-commands)
7. [Switching Models](#7-switching-models)
8. [Tool Execution](#8-tool-execution)
9. [Memory & Context](#9-memory--context)
10. [Skills](#10-skills)
11. [MCP Servers](#11-mcp-servers)
12. [Python Plugins](#12-python-plugins)
13. [Configuration Files](#13-configuration-files)
14. [Troubleshooting](#14-troubleshooting)

---

## 1. Requirements

| Requirement | Notes |
|-------------|-------|
| Go 1.24.2+ | Required to build from source |
| xAI API key | Required for Grok (default primary AI) |
| Google Gemini API key | Recommended — specialist consultant |
| Anthropic, OpenAI, MiniMax keys | Optional — additional providers |
| Python 3.8+ | Optional — required for Python plugins (e.g., RAG memory) |

At least one AI provider key (xAI or Google) is required. Gorkbot runs in single-provider mode if only one is configured.

**Platform support:** Linux (amd64/arm64), macOS (arm64/amd64), Android/Termux (arm64), Windows (amd64)

---

## 2. Installation

### Clone and Build

```bash
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot

# Build for your platform
make build

# Verify
./bin/gorkbot --version
```

### Build Targets

```bash
make build           # current platform → bin/gorkbot
make build-linux     # Linux amd64 → bin/gorkbot-linux
make build-android   # Android arm64 → bin/gorkbot-android
make build-windows   # Windows amd64 → bin/gorkbot.exe
make dist            # all platforms + release tarball in dist/
make install-global  # install 'gork' shorthand to ~/bin
```

### Android / Termux

```bash
# Install Go in Termux
pkg install golang git

# Clone and build
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
go build -o bin/gorkbot ./cmd/gorkbot/
```

---

## 3. API Key Setup

### Option A: Interactive Wizard (Recommended)

```bash
./gorkbot.sh setup
```

The wizard prompts for each provider key and writes them to `.env`.

### Option B: Manual .env Configuration

```bash
cp .env.example .env
nano .env
```

```env
# Required — at least one:
XAI_API_KEY=xai-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
GEMINI_API_KEY=AIzaSyxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Optional additional providers:
ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
MINIMAX_API_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Option C: Set Keys at Runtime

Inside the TUI, add or update keys without restarting:

```
/key xai xai-your-key
/key google AIzaSy-your-key
/key anthropic sk-ant-your-key
/key status         # show all keys (masked)
/key validate xai   # verify key is accepted by the provider
```

Or open the model selection UI (`Ctrl+T`) and press `k` on any provider to open the key entry modal.

### Getting API Keys

| Provider | URL |
|----------|-----|
| xAI (Grok) | [console.x.ai](https://console.x.ai) → API Keys |
| Google (Gemini) | [aistudio.google.com/apikey](https://aistudio.google.com/apikey) |
| Anthropic | [console.anthropic.com](https://console.anthropic.com) → API Keys |
| OpenAI | [platform.openai.com](https://platform.openai.com) → API Keys |
| MiniMax | [minimax.io](https://minimax.io) → Developer Console |

### Verify Configuration

```bash
./gorkbot.sh status
```

```
Gorkbot Configuration Status (v3.5.1)

xAI API Key:      ✅ Set (xai-...xxxx)
Gemini API Key:   ✅ Set (AIza...xxxx)
Anthropic Key:    ❌ Not set
OpenAI Key:       ❌ Not set
MiniMax Key:      ❌ Not set

Primary model:    grok-3 (xAI)
Specialist:       gemini-2.0-flash (Google)

✅ Ready to run: ./gorkbot.sh
```

---

## 4. First Run

```bash
./gorkbot.sh
```

**Important:** Always use `./gorkbot.sh` rather than `./bin/gorkbot` directly. The script loads your `.env` file and passes all flags through to the binary.

You will see a splash screen followed by the full-screen TUI. Type any message and press `Enter` to send.

### One-Shot Mode (Non-Interactive)

```bash
# Single prompt, then exit
./gorkbot.sh -p "What is the time complexity of quicksort?"

# Read from stdin
echo "Explain this code" | ./gorkbot.sh --stdin

# Write output to file
./gorkbot.sh -p "Write a Go HTTP server" --output server.go

# With tool filtering
./gorkbot.sh -p "List this directory" --allow-tools bash,list_directory
./gorkbot.sh -p "Review this code" --deny-tools bash,write_file,delete_file
```

---

## 5. TUI Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Chat │ Models (Ctrl+T) │ Tools (Ctrl+E) │ Cloud Brains (Ctrl+D)   │
│  Analytics (Ctrl+A) │ Diagnostics (Ctrl+\)                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│                      Conversation (scrollable)                       │
│   Markdown rendered with syntax highlighting, tables, code blocks   │
│                                                                     │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ > Type here...                                    [Alt+Enter=newline]│
├─────────────────────────────────────────────────────────────────────┤
│ ctx: 4% │ $0.0002 │ Normal │ main                                   │
└─────────────────────────────────────────────────────────────────────┘
```

### Status Bar

| Field | Description |
|-------|-------------|
| `ctx: X%` | Context window usage (estimated tokens / limit) |
| `$X.XXXX` | Session cost estimate across all providers |
| `Normal/Plan/Auto` | Current execution mode |
| `<branch>` | Current git branch |

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Alt+Enter` | Insert newline in message |
| `PgUp` / `PgDn` | Scroll conversation |
| `Ctrl+X` | Interrupt in-progress generation |
| `Ctrl+T` | Open model selection |
| `Ctrl+G` | Open settings overlay |
| `Ctrl+P` | Cycle execution mode (Normal → Plan → Auto) |
| `Ctrl+E` | Open tool registry table |
| `Ctrl+D` | Open Cloud Brains (model discovery + agent tree) |
| `Ctrl+A` | Open Analytics dashboard |
| `Ctrl+\` | Open System Diagnostics |
| `Ctrl+B` | Open conversation bookmarks |
| `Ctrl+R` | Toggle reasoning frame fold/unfold |
| `Ctrl+H` | Help |
| `Ctrl+I` | Focus input field |
| `Ctrl+L` | Clear screen (display only — history preserved) |
| `Ctrl+C` / `Ctrl+Q` | Quit |

---

## 6. Essential Commands

All slash commands are entered in the input field. Type `/help` for a complete listing.

### Conversation Management

| Command | Description |
|---------|-------------|
| `/clear` | Reset conversation history and display |
| `/compact [hint]` | Summarize conversation to free context window space |
| `/compress` | Alias for `/compact` |
| `/context` | Show context window usage breakdown |
| `/cost` | Show estimated session API cost |
| `/rename <name>` | Rename the current session |

### Export & Sessions

| Command | Description |
|---------|-------------|
| `/export markdown` | Export conversation to Markdown file |
| `/export json` | Export conversation to JSON |
| `/export plain` | Export conversation to plain text |
| `/save <name>` | Save current session to a named file |
| `/resume <name>` | Restore a previously saved session |
| `/chat list` | List all saved sessions |
| `/chat load <name>` | Load a saved session |
| `/chat delete <name>` | Delete a saved session |
| `/rewind [last\|id]` | Restore to a session checkpoint |

### Models & Keys

| Command | Description |
|---------|-------------|
| `/model` | Show current primary and specialist models |
| `/model primary grok-3-mini` | Switch primary model |
| `/model consultant claude-opus-4-6` | Switch specialist |
| `/model consultant auto` | Enable auto-specialist selection |
| `/key <provider> <key>` | Set a provider API key at runtime |
| `/key status` | Show all provider key statuses (masked) |
| `/key validate <provider>` | Validate a provider's API key |

### Tools & Permissions

| Command | Description |
|---------|-------------|
| `/tools` | List all tools with permission and category |
| `/tools stats` | Tool usage analytics dashboard |
| `/permissions` | Show all tool permission levels |
| `/permissions reset` | Reset all permissions to "once" |
| `/permissions reset <tool>` | Reset a single tool's permission |
| `/rules list` | List all glob permission rules |
| `/rules add bash deny "*"` | Add a permission rule |
| `/rules remove <id>` | Remove a permission rule |

### Customization & Mode

| Command | Description |
|---------|-------------|
| `/theme dracula` | Switch TUI theme (dracula/nord/gruvbox/solarized/monokai) |
| `/mode plan` | Switch to plan mode |
| `/mode auto` | Switch to auto mode |
| `/mode normal` | Switch to normal mode |
| `/mode` | Show current execution mode |
| `/settings` | Open settings overlay (Ctrl+G) |
| `/debug` | Toggle debug mode (shows raw AI output and tool JSON) |

### Memory & Skills

| Command | Description |
|---------|-------------|
| `/skills list` | List all available skills |
| `/skills help <name>` | Show a skill's description |
| `/<skill-name>` | Invoke a built-in or custom skill |
| `/hooks list` | List configured lifecycle hooks |
| `/hooks dir` | Show hooks directory path |

### Integrations

| Command | Description |
|---------|-------------|
| `/mcp status` | MCP server connection status and tool counts |
| `/mcp config` | Show MCP configuration |
| `/schedule` | List scheduled tasks |
| `/share start` | Start SSE session sharing relay |
| `/share stop` | Stop the SSE relay |
| `/rate 1-5` | Rate last response (trains adaptive router) |
| `/a2a` | Show A2A gateway status |
| `/telegram` | Telegram bot status |
| `/commands` | List user-defined slash commands |
| `/bug` | Open GitHub issue template |
| `/version` | Show version and system info |
| `/quit` | Exit gracefully |

---

## 7. Switching Models

### Via TUI (Ctrl+T)

1. Press `Ctrl+T` to open the dual-pane model selection
2. Use `Tab` to switch between Primary and Specialist panes
3. Navigate with `↑`/`↓` or `k`/`j`
4. Press `Enter` to select the highlighted model
5. Press `r` to refresh model lists from all providers
6. Press `p` to cycle provider filter
7. Press `k` to open the API key entry modal for the selected provider

Models are hot-swapped immediately — no restart required.

### Via Command

```
/model primary grok-3-mini              # use faster, cheaper primary
/model consultant claude-opus-4-6       # use Claude Opus as specialist
/model consultant gemini-2.0-flash      # use Gemini as specialist
/model consultant auto                  # auto-select specialist per task
/model                                  # show current selection
```

### Via Environment Variable (before launch)

```bash
GORKBOT_PRIMARY_MODEL=grok-3-mini ./gorkbot.sh
GORKBOT_CONSULTANT_MODEL=gemini-2.0-flash ./gorkbot.sh
```

### Provider Failover

If a provider becomes unreachable (rate limit, billing issue, outage), Gorkbot automatically fails over:

```
Failover order: xAI → Google → Anthropic → MiniMax → OpenAI → OpenRouter
```

The TUI shows `[Switched to Google — retrying]` when failover occurs. The current turn is retried inline with the new provider.

To manually disable a provider: `Ctrl+G` → **API Providers** tab → toggle off.

### Dual-Model Orchestration

By default, Gorkbot uses two models together:
- **Primary (Grok)** — handles most requests
- **Specialist (Gemini)** — consulted for complex architectural, analytical, or lengthy tasks

When the specialist is consulted, its response appears in a bordered box in the conversation:

```
╭─────────────────────────────────────────────────────────────╮
│  Specialist (Gemini)                                        │
│                                                             │
│  Recommendation: Use an event-driven architecture with...   │
╰─────────────────────────────────────────────────────────────╯
```

---

## 8. Tool Execution

Gorkbot has 162+ built-in tools that the AI calls automatically. Tools span shell execution, file operations, git, web, system, security, Android/Termux device control, vision, data science, media, scheduling, and more.

### How Tool Permission Works

When a tool needs permission, a centered overlay appears before execution:

```
┌──────────────────────────────────────────────────────┐
│  Permission Request                                  │
│                                                      │
│  Tool:     bash                                      │
│  Command:  ls -la /home/user/project                 │
│                                                      │
│  ▶ [Always]   Grant permanent permission             │
│    [Session]  Allow for this session                 │
│    [Once]     Ask again next time (recommended)      │
│    [Never]    Block permanently                      │
│                                                      │
│  ↑/↓ select   Enter confirm   Esc deny               │
└──────────────────────────────────────────────────────┘
```

| Level | Use When |
|-------|---------|
| **Always** | Safe read-only tools you trust completely |
| **Session** | Batch work — auto-revokes when you exit |
| **Once** | Safe default — always ask |
| **Never** | Tools you never want the AI to use |

### Quick Permission Configuration

When first prompted, grant **Always** to these safe read-only tools:
- `read_file`, `list_directory`, `file_info`, `grep_content`, `search_files`
- `git_status`, `git_diff`, `git_log`
- `system_info`, `disk_usage`

Keep **Once** for anything that writes, deletes, commits, pushes, or makes network requests.

### Enabling Security Tools

The `security` and `pentest` tool categories are disabled by default:

```
Ctrl+G → Tool Groups → toggle "security" → ON
```

This enables Nmap, Shodan, Nikto, SQLMap, Hydra, Nuclei, and 25+ other security tools. Only enable for authorized security assessments.

---

## 9. Memory & Context

### Within-Session Context

Gorkbot maintains your full conversation history within a session. The AI sees everything discussed since the last `/clear`.

When the context window fills up:
```
/compact                                  # auto-summarize
/compact "focus on the database design"   # guided summarization
/context                                  # check current usage %
```

### Session Checkpoints

Checkpoints are taken automatically before tool executions. Rewind to undo recent changes:
```
/rewind              # rewind to most recent checkpoint
/rewind 12           # rewind to checkpoint #12
```

### Cross-Session Project Memory (CCI)

CCI (Codified Context Infrastructure) gives Gorkbot persistent knowledge about your project across all sessions. Default files are created at `~/.config/gorkbot/cci/` on first run.

**Edit Tier 1 — always loaded into every session:**
```bash
nano ~/.config/gorkbot/cci/hot/CONVENTIONS.md
```

Add your project conventions, build commands, coding standards, and forbidden patterns here. They will be injected into the system prompt in every future session automatically.

**Create Tier 3 subsystem docs (on-demand):**
```
"Please document the authentication system for the CCI"
→ AI creates ~/.config/gorkbot/cci/docs/auth_system.md
→ Retrieved on future sessions via: mcp_context_get_subsystem
```

### GORKBOT.md — Per-Project Configuration

Create a `GORKBOT.md` in your project root for project-specific instructions:

```markdown
# My API Service — Gorkbot Configuration

## Build
`make build` — builds to bin/
`make test` — runs all tests with coverage

## Conventions
- Go 1.24, stdlib-first, minimal external dependencies
- All exported identifiers need godoc comments
- Error messages in lowercase, no trailing periods

## Never
- Modify database schema without a migration
- Push directly to main — PRs required
```

Gorkbot finds `GORKBOT.md` by searching up from the current directory.

### RAG Memory Plugin (Semantic Cross-Session Search)

Store and search important context semantically across sessions:

```
# Store (ChromaDB + MiniLM-L6-v2 embeddings)
rag_memory {"action": "store", "content": "Payment API uses HMAC-SHA256. Key stored in PAYMENT_SECRET env var.", "metadata": "{\"topic\": \"payments\"}"}

# Search on a future session
rag_memory {"action": "search", "query": "payment authentication", "n_results": 5, "min_score": 0.7}
```

First use auto-installs `chromadb` and `sentence-transformers` via pip (~100MB, one-time).

---

## 10. Skills

Skills are YAML-frontmatter markdown templates that define reusable reasoning workflows. They are invokable as slash commands.

### Using Built-in Skills

```
/skills list               # list all 25+ built-in skills
/code-review               # invoke the code review skill
/deep_reason               # deep chain-of-thought analysis
/web_vuln_scanner          # security vulnerability assessment
/knowledge_gardener        # synthesize and organize knowledge
/research_scholar          # academic research workflow
/morning_briefing          # daily status briefing
/full_autonomy             # autonomous task execution
/dependency_updater        # project dependency management
```

### Adding Custom Skills

Create a `.md` file in `~/.config/gorkbot/skills/`:

```markdown
---
name: my-review
description: Code review for my project's conventions
version: 1.0
---

Review the following code against our project conventions:
- Go 1.24 stdlib-first
- All exported functions must have godoc comments
- Errors must be wrapped with fmt.Errorf and %w

Code to review:
{{input}}

Provide specific line-level feedback with suggested fixes.
```

Invoke with `/my-review`.

---

## 11. MCP Servers

MCP (Model Context Protocol) servers give Gorkbot access to external capabilities — filesystem operations on remote servers, GitHub API, databases, and more.

### Configuration

Create `~/.config/gorkbot/mcp.json`:

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
    }
  ]
}
```

Restart Gorkbot. Tools from each server are registered as `mcp_<server>_<toolname>`.

### Status

```
/mcp status
→ filesystem: connected (18 tools)
→ github: connected (12 tools)
```

All MCP tools appear in `/tools` and go through the standard permission pipeline.

---

## 12. Python Plugins

Python plugins extend Gorkbot with Python-based tools. They are auto-discovered from `plugins/python/*/manifest.json` at startup.

### Built-in: RAG Memory

The RAG memory plugin is included and auto-configured:

```
plugins/python/rag_memory/
├── manifest.json
└── tool.py
```

On first invocation, `chromadb` and `sentence-transformers` are auto-installed via pip.

### Adding a Custom Plugin

1. Create the plugin directory:
```bash
mkdir -p plugins/python/my_tool
```

2. Create `manifest.json`:
```json
{
  "name": "my_tool",
  "version": "1.0.0",
  "description": "My custom Python tool — does X with Y",
  "entry_point": "tool.py",
  "category": "custom",
  "requires": ["requests"],
  "parameters": {
    "input": {
      "type": "string",
      "description": "Input text to process",
      "required": true
    }
  }
}
```

3. Create `tool.py` (reads params from stdin, writes result to stdout):
```python
#!/usr/bin/env python3
import sys
import json

params = json.load(sys.stdin)
result = f"Processed: {params['input']}"
print(result)
```

4. Restart Gorkbot. The plugin is auto-discovered, dependencies are installed, and the tool is registered.

---

## 13. Configuration Files

All configuration lives in `~/.config/gorkbot/` (Linux/Android) or the platform-appropriate location.

| File | Description |
|------|-------------|
| `.env` | API keys and overrides (project root, gitignored) |
| `api_keys.json` | Runtime key store (0600 permissions) |
| `app_state.json` | Persisted model selection, disabled categories/providers |
| `active_theme` | Active theme name (plain text) |
| `themes/*.json` | Custom theme definitions |
| `mcp.json` | MCP server configurations |
| `tool_permissions.json` | Persisted always/never tool permissions |
| `dynamic_tools.json` | Hot-loaded dynamic tools |
| `feedback.jsonl` | Adaptive router feedback history |
| `usage_history.jsonl` | Per-model billing history |
| `vector_store.json` | MEL heuristic vector store |
| `cci/hot/` | Always-loaded project conventions (Tier 1 CCI) |
| `cci/specialists/` | On-demand specialist personas (Tier 2 CCI) |
| `cci/docs/` | On-demand subsystem documentation (Tier 3 CCI) |
| `hooks/` | Lifecycle hook shell scripts |
| `skills/` | Custom skill markdown files |
| `rag_memory/` | RAG memory ChromaDB store |
| `sessions/` | Saved named sessions |

For full documentation on each file, see [docs/CONFIGURATION.md](docs/CONFIGURATION.md).

---

## 14. Troubleshooting

### "XAI_API_KEY is not set"

Run the setup wizard:
```bash
./gorkbot.sh setup
```

Or add to `.env`:
```bash
echo "XAI_API_KEY=xai-your-key" >> .env
```

### Always use the wrapper script

```bash
./gorkbot.sh    # ✅ loads .env, passes all CLI flags
./bin/gorkbot   # ⚠️  skips .env (only safe if env vars already exported)
```

### Provider failover is triggering frequently

Check your account credits at the provider console. Validate keys:
```
/key validate xai
/key validate google
```

If a provider keeps timing out, disable it for the session:
```
Ctrl+G → API Providers → toggle provider off
```

### Context window full

```
/compact                              # auto-summarize
/compact "focus on the design"        # guided summarization
/rewind                               # undo recent turns
/clear                                # start fresh (loses history)
```

### Tool permission prompt not appearing

The tool may have a stored `always` or `never` permission:
```
/permissions
/permissions reset <tool-name>
```

Or a deny rule is blocking it:
```
/rules list
/rules remove <rule-id>
```

### TUI display looks wrong

- Resize your terminal — the TUI auto-adjusts to `WindowSizeMsg`
- Ensure your terminal supports 256 colors or true color
- On Android/Termux: use the Termux terminal app directly (not SSH from another terminal)

### Slow responses

Switch to a faster model:
```
/model primary grok-3-mini
```

For time-sensitive one-shot queries:
```bash
./gorkbot.sh -p "Quick question" --timeout 30s
```

### Debug mode

Enable to see raw AI output including all tool JSON:
```
/debug
```

Toggle off with `/debug` again.

### Log access

```bash
# Structured JSON logs
cat ~/.config/gorkbot/logs/gorkbot.json | python3 -m json.tool | head -100

# Live log stream (Linux)
tail -f ~/.config/gorkbot/logs/gorkbot.json

# Enable JSONL execution tracing
./gorkbot.sh --trace
# Traces written to ~/.config/gorkbot/traces/<timestamp>.jsonl
```

### Getting help

```
/help           # all commands
/version        # build info and system details
/bug            # open GitHub issue template (pre-filled)
```
