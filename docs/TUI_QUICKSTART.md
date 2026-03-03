# Gorkbot TUI Quick Start

**Version:** 3.5.1

This guide covers the Gorkbot terminal interface in full — launching, navigation, all tabs, overlays, keyboard shortcuts, status bar, execution modes, and touch-scroll on Android.

---

## Table of Contents

1. [Launching Gorkbot](#1-launching-gorkbot)
2. [Interface Overview](#2-interface-overview)
3. [Tab Navigation](#3-tab-navigation)
4. [Chat Tab](#4-chat-tab)
5. [Models Tab (Ctrl+T)](#5-models-tab-ctrlt)
6. [Tools Tab (Ctrl+E)](#6-tools-tab-ctrle)
7. [Cloud Brains Tab (Ctrl+D)](#7-cloud-brains-tab-ctrld)
8. [Analytics Tab (Ctrl+A)](#8-analytics-tab-ctrla)
9. [Diagnostics Tab (Ctrl+\)](#9-diagnostics-tab-ctrl)
10. [Settings Overlay (Ctrl+G)](#10-settings-overlay-ctrlg)
11. [Bookmarks Overlay (Ctrl+B)](#11-bookmarks-overlay-ctrlb)
12. [Status Bar](#12-status-bar)
13. [Execution Modes](#13-execution-modes)
14. [Keyboard Shortcuts Reference](#14-keyboard-shortcuts-reference)
15. [Touch-Scroll on Android](#15-touch-scroll-on-android)
16. [One-Shot Mode](#16-one-shot-mode)

---

## 1. Launching Gorkbot

**Standard launch (recommended — loads .env):**
```bash
./gorkbot.sh
```

**Direct binary (requires API keys already in environment):**
```bash
./bin/gorkbot
```

**One-shot prompt (non-interactive):**
```bash
./gorkbot.sh -p "Explain this function"
```

**Shared session (SSE relay):**
```bash
./gorkbot.sh --share          # start relay, print observer URL
./gorkbot.sh --join host:9090 # join as read-only observer
```

**Debug flags:**
```bash
./gorkbot.sh --verbose-thoughts   # show consultant reasoning in chat
./gorkbot.sh --watchdog           # orchestrator state debug log
./gorkbot.sh --trace              # write JSONL trace to ~/.gorkbot/traces/
```

---

## 2. Interface Overview

The TUI is a full-screen application built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss), and [Glamour](https://github.com/charmbracelet/glamour).

```
┌─────────────────────────────────────────────────────────────────────┐
│  Chat │ Models (Ctrl+T) │ Tools (Ctrl+E) │ Cloud Brains (Ctrl+D)   │
│  Analytics (Ctrl+A) │ Diagnostics (Ctrl+\)                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│                      Conversation Area                              │
│                   (scrollable with PgUp/PgDn                        │
│                    or touch-scroll on Android)                      │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ You: What is the capital of France?                         │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ Gorkbot: Paris is the capital of France. Founded over 2,000 │   │
│  │ years ago on the Île de la Cité, it is today a metropolis  │   │
│  │ of approximately 2.2 million people in the city proper...  │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ > _                                                   [Multi-line] │
│   Type a message or /help for commands                              │
├─────────────────────────────────────────────────────────────────────┤
│ ctx: 4% │ $0.0002 │ Normal │ main                                   │
└─────────────────────────────────────────────────────────────────────┘
```

**Regions:**
| Region | Description |
|--------|-------------|
| **Tab bar** | Navigation tabs — click or use keyboard shortcuts |
| **Conversation area** | Scrollable message history with Markdown rendering |
| **Input field** | Multi-line text input for prompts and slash commands |
| **Status bar** | Context%, cost, execution mode, git branch |

---

## 3. Tab Navigation

| Tab | Shortcut | Description |
|-----|----------|-------------|
| **Chat** | (default) | Main conversation interface |
| **Models** | `Ctrl+T` | Dual-pane model selection, key management |
| **Tools** | `Ctrl+E` | Tool registry table with call stats |
| **Cloud Brains** | `Ctrl+D` | Discovered models and agent delegation tree |
| **Analytics** | `Ctrl+A` | Session analytics dashboard |
| **Diagnostics** | `Ctrl+\` | System and orchestrator diagnostics |

Press `Esc` from any tab to return to Chat.

---

## 4. Chat Tab

The Chat tab is the primary interface for interacting with Gorkbot.

### Sending Messages

Type your message in the input field at the bottom and press `Enter` to send. For multi-line input, use `Alt+Enter` to insert a newline.

### Message Rendering

All AI responses are rendered as full **CommonMark Markdown** with:
- Syntax-highlighted code blocks (all major languages)
- Tables, blockquotes, lists
- Bold, italic, inline code
- Horizontal rules

### Consultant Response Boxes

When the specialist consultant (secondary AI) contributes, its response appears in a bordered box:

```
╭─────────────────────────────────────────────────────────────╮
│  Specialist (Gemini)                                        │
│                                                             │
│  Architectural recommendation: For this use case, prefer   │
│  an event-driven pattern with CQRS. The write model can     │
│  be a simple append-only log; the read model can be a      │
│  pre-computed projection updated via async consumer...      │
╰─────────────────────────────────────────────────────────────╯
```

### Tool Execution Display

When the AI calls a tool, the TUI shows:
```
[Tool: read_file] path=main.go
```
After execution, the result is incorporated into the next response. Enable `/debug` to see the full raw tool JSON.

### Collapsible Frames

Reasoning blocks and A2A messages appear as collapsible frames. Toggle with `Ctrl+R`:

```
▶ [Reasoning] (click to expand)    ← collapsed

▼ [Reasoning]                      ← expanded
  The user is asking about...
  I should first check the file...
  My plan is to...
```

### Slash Commands

Type `/` in the input field to access all slash commands. Type `/help` for a full listing. Commands are processed immediately when sent (they do not go to the AI).

---

## 5. Models Tab (Ctrl+T)

The Models tab provides a dual-pane model selection interface that lets you view and switch both the primary AI and the specialist consultant without leaving the TUI.

```
┌──────────────────────────────────────────────────────────────┐
│  Models                                                      │
│                                                              │
│  PRIMARY                    │  SPECIALIST                    │
│  ─────────────              │  ─────────────                 │
│  ▶ xAI                      │    Google                      │
│    ✅ grok-3         [active]│    ✅ gemini-2.0-flash [active] │
│       grok-3-mini            │       gemini-1.5-pro           │
│       grok-3-fast            │       gemini-2.0-flash-think…  │
│  ─────────────              │  ─────────────                 │
│    Anthropic                │    Anthropic                   │
│       claude-opus-4-6        │       claude-sonnet-4-6        │
│       claude-sonnet-4-6      │       claude-haiku-4-5         │
│  ─────────────              │  ─────────────                 │
│    OpenAI                   │    OpenAI                      │
│       gpt-4o                 │       gpt-4o                   │
│       o4-mini  [thinking]    │       o3        [thinking]     │
│                                                              │
│  [Provider: All] [r=refresh] [k=add key] [Tab=switch pane]  │
└──────────────────────────────────────────────────────────────┘
```

### Controls

| Key | Action |
|-----|--------|
| `Tab` | Switch between Primary and Specialist panes |
| `↑` / `↓` or `k` / `j` | Navigate models |
| `Enter` | Select the highlighted model |
| `r` | Refresh model lists from all providers |
| `p` | Cycle provider filter (All / xAI / Google / Anthropic / OpenAI / MiniMax) |
| `k` | Open API key entry prompt for the selected provider |
| `Esc` | Return to Chat |

### API Key Entry

Press `k` on a provider to open the API key modal:

```
┌──────────────────────────────────────────────────┐
│  Set API Key — Anthropic                         │
│                                                  │
│  Get your key at: console.anthropic.com          │
│                                                  │
│  Key: sk-ant-_                                   │
│                                                  │
│  Enter  save     Esc  cancel                     │
└──────────────────────────────────────────────────┘
```

Keys entered here are saved to `api_keys.json` and immediately available for model selection.

### Auto Specialist Mode

Select `[Auto]` in the Specialist pane to enable auto-selection, where the ARC Router chooses the best specialist model for each task category based on capability class and feedback history.

---

## 6. Tools Tab (Ctrl+E)

The Tools tab shows the full tool registry as a sortable table.

```
┌──────────────────────────────────────────────────────────────┐
│  Tools (162 registered)                                      │
│                                                              │
│  Name                  Category   Permission  Calls  Status  │
│  ─────────────────     ─────────  ──────────  ─────  ──────  │
│  bash                  shell      once           47  enabled  │
│  read_file             file       always        312  enabled  │
│  write_file            file       once           89  enabled  │
│  edit_file             file       once           76  enabled  │
│  git_status            git        always         55  enabled  │
│  git_diff              git        always         38  enabled  │
│  web_fetch             web        once           14  enabled  │
│  ...                                                          │
│                                                              │
│  [/] filter   [Enter] details   [Esc] back                  │
└──────────────────────────────────────────────────────────────┘
```

For detailed tool usage analytics, use:
```
/tools stats
```

---

## 7. Cloud Brains Tab (Ctrl+D)

The Cloud Brains tab shows the dynamic model discovery system and the live agent delegation tree.

```
┌────────────────────────────────────────────────────────────────┐
│  Cloud Brains                                                  │
│                                                                │
│  DISCOVERED MODELS              AGENT TREE                     │
│  ─────────────────              ──────────                     │
│  xAI                            Root Agent (grok-3)            │
│    General                       ├── Sub-agent #1 (grok-3-mini)│
│      grok-3          ✅           │     Task: code review       │
│      grok-3-fast     ✅           └── Sub-agent #2 (gemini-2.0) │
│    Speed                               Task: docs generation   │
│      grok-3-mini     ✅                                         │
│  Google                                                        │
│    General                                                     │
│      gemini-2.0-flash ✅                                       │
│    Reasoning                                                   │
│      gemini-2.0-flash-think ✅                                  │
│  Anthropic                                                     │
│    Reasoning                                                   │
│      claude-opus-4-6  ✅                                       │
│    General                                                     │
│      claude-sonnet-4-6 ✅                                      │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

**Left panel:** All models discovered from all providers, grouped by provider and capability class (`General`, `Reasoning`, `Speed`, `Coding`). Availability indicator shows whether a valid API key is configured.

**Right panel:** The live hierarchical agent delegation tree. When `spawn_sub_agent` is used, sub-agents appear here with their assigned model and task description. Updated in real-time.

---

## 8. Analytics Tab (Ctrl+A)

The Analytics tab shows a real-time dashboard of session metrics.

```
┌──────────────────────────────────────────────────────────────┐
│  Session Analytics                                           │
│                                                              │
│  TOOL USAGE                      PERFORMANCE                  │
│  ──────────                      ───────────                  │
│  Total calls:        521          Avg latency:    1.2s        │
│  Successful:         498 (95.6%)  P95 latency:    3.8s        │
│  Failed:              23  (4.4%)  Fastest tool:   file_info   │
│                                   Slowest tool:   bash        │
│  TOP TOOLS                                                    │
│  read_file      312 calls  99% success                        │
│  bash            47 calls  87% success                        │
│  write_file      89 calls  98% success                        │
│  git_diff        38 calls  100% success                       │
│                                                              │
│  COST BREAKDOWN                                              │
│  xAI (primary):    $0.0312  (2.1M tokens)                    │
│  Google (consult): $0.0041  (380k tokens)                    │
│  Total:            $0.0353                                    │
└──────────────────────────────────────────────────────────────┘
```

---

## 9. Diagnostics Tab (Ctrl+\)

The Diagnostics tab shows system and orchestrator state for troubleshooting.

```
┌──────────────────────────────────────────────────────────────┐
│  System Diagnostics                                          │
│                                                              │
│  PLATFORM                                                    │
│  OS:           linux (android / Termux)                      │
│  Arch:         arm64                                         │
│  Go:           1.24.2                                        │
│  Gorkbot:      v3.5.1                                        │
│                                                              │
│  PROVIDERS                                                   │
│  Primary:      grok-3 (xAI) — 47 calls, 2.1M tokens         │
│  Specialist:   gemini-2.0-flash (Google) — 8 calls           │
│  Failover:     0 cascades triggered this session             │
│                                                              │
│  MEMORY                                                      │
│  CCI Hot:      loaded (2 files, 1,847 chars)                 │
│  MEL Heuristics: 47 loaded, 5 injected this turn            │
│  SENSE Engrams: 312 total                                    │
│  Goals:        3 open                                        │
│                                                              │
│  MCP SERVERS                                                 │
│  filesystem:   connected (18 tools)                          │
│  github:       connected (12 tools)                          │
└──────────────────────────────────────────────────────────────┘
```

---

## 10. Settings Overlay (Ctrl+G)

The Settings overlay is a four-tab modal that persists to `app_state.json`.

```
Ctrl+G to open   Esc to close
```

```
┌────────────────────────────────────────────────────────────────┐
│  Settings                                                      │
│                                                                │
│  [Model Routing] [Verbosity] [Tool Groups] [API Providers]     │
│  ─────────────────────────────────────────────────────         │
│                                                                │
│  Model Routing                                                 │
│                                                                │
│  Primary Provider:    xAI (Grok)     [change]                  │
│  Specialist:          Google (Gemini) [change]                  │
│  Auto Specialist:     [ ] Enable                               │
│                                                                │
│  ARC Routing:         WorkflowDirect (last task)               │
│  Compute Budget:      MaxTokens=4096  MaxTools=8               │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Tab 1: Model Routing

Configure primary and specialist providers, auto-specialist mode, and view the last ARC routing decision and compute budget.

### Tab 2: Verbosity

Toggle verbose consultant thoughts (show/hide the Gemini reasoning chain of thought in the TUI when `--verbose-thoughts` is active).

### Tab 3: Tool Groups

Enable or disable entire tool categories. Changes take effect immediately. High-risk categories (`security`, `pentest`) are disabled by default.

```
  [✅] shell         [✅] file          [✅] git
  [✅] web           [✅] system        [✅] ai
  [✅] devops        [✅] android       [✅] vision
  [✅] database      [✅] memory        [✅] scheduler
  [  ] security      [  ] pentest
```

### Tab 4: API Providers

Toggle individual providers on or off for this session. Disabled providers are excluded from the failover cascade and model selection. Persisted to `app_state.json` under `disabled_providers`.

```
  [✅] xAI          key: set   (xai-...xxxx)
  [✅] Google        key: set   (AIza...xxxx)
  [  ] Anthropic     key: unset
  [  ] OpenAI        key: unset
  [  ] MiniMax       key: unset
```

---

## 11. Bookmarks Overlay (Ctrl+B)

The Bookmarks overlay lets you mark important conversation points and jump back to them.

```
Ctrl+B to open   Esc to close
```

```
┌─────────────────────────────────────────────────────┐
│  Conversation Bookmarks                             │
│                                                     │
│  [+] Add bookmark at current position               │
│                                                     │
│  ● Turn 12 — Auth implementation plan               │
│  ● Turn 28 — Database schema design                 │
│  ● Turn 45 — Performance bottleneck found           │
│                                                     │
│  Enter=jump to   d=delete   Esc=close               │
└─────────────────────────────────────────────────────┘
```

---

## 12. Status Bar

The status bar runs across the bottom of the screen and provides real-time session state:

```
ctx: 14% │ $0.0042 │ Normal │ main
  │         │         │       └── Current git branch (from git rev-parse)
  │         │         └── Execution mode (Normal / Plan / Auto)
  │         └── Session cost estimate (all providers combined)
  └── Context window usage (estimated tokens / limit)
```

| Field | Description |
|-------|-------------|
| `ctx: X%` | Current history token usage as a percentage of the limit |
| `$X.XXXX` | Estimated API cost for this session |
| `Normal` / `Plan` / `Auto` | Current execution mode |
| `<branch>` | Active git branch in the working directory |

The context% colors from green (low) to yellow (medium) to red (high, approaching limit).

---

## 13. Execution Modes

Gorkbot has three execution modes that affect how the AI approaches tasks:

| Mode | Description |
|------|-------------|
| **Normal** | Standard response — balanced tool use and direct answers |
| **Plan** | The AI lays out a step-by-step plan before executing any actions |
| **Auto** | The AI operates autonomously with minimal confirmation prompts |

**Cycle through modes:**
- `Ctrl+P` — rotate Normal → Plan → Auto → Normal
- `/mode` — show current mode
- `/mode plan` — set to Plan mode
- `/mode normal` — set to Normal mode
- `/mode auto` — set to Auto mode

The current mode is always visible in the status bar.

---

## 14. Keyboard Shortcuts Reference

### Global

| Key | Action |
|-----|--------|
| `Ctrl+C` / `Ctrl+Q` | Quit Gorkbot |
| `Ctrl+H` | Show help |
| `Esc` | Back / close overlay / return to Chat |

### Chat

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Alt+Enter` | Insert newline (multi-line input) |
| `PgUp` | Scroll conversation up |
| `PgDn` | Scroll conversation down |
| `Ctrl+I` | Focus the input field |
| `Ctrl+L` | Clear screen (resets display, not history) |
| `Ctrl+X` | Interrupt / cancel in-progress AI generation |
| `Ctrl+R` | Fold / unfold collapsible reasoning frames |

### Navigation

| Key | Action |
|-----|--------|
| `Ctrl+T` | Open Models tab |
| `Ctrl+E` | Open Tools tab |
| `Ctrl+D` | Open Cloud Brains tab |
| `Ctrl+A` | Open Analytics tab |
| `Ctrl+\` | Open Diagnostics tab |
| `Ctrl+G` | Open Settings overlay |
| `Ctrl+B` | Open Bookmarks overlay |
| `Ctrl+P` | Cycle execution mode (Normal → Plan → Auto) |

### Model Selection (Ctrl+T)

| Key | Action |
|-----|--------|
| `Tab` | Switch between Primary and Specialist panes |
| `↑` / `k` | Move up in model list |
| `↓` / `j` | Move down in model list |
| `Enter` | Select highlighted model |
| `r` | Refresh model lists from all providers |
| `p` | Cycle provider filter |
| `k` | Open API key entry for selected provider |
| `Esc` | Return to Chat |

### Permission Prompt

| Key | Action |
|-----|--------|
| `↑` / `k` | Move selection up |
| `↓` / `j` | Move selection down |
| `Enter` | Confirm selected permission level |
| `Esc` | Deny this execution (no permission change) |

---

## 15. Touch-Scroll on Android

Gorkbot's conversation area supports native touch-scroll on Android / Termux with finger gestures.

### How It Works

Touch events are captured via Bubble Tea's `tea.MouseMsg` handler in `internal/tui/update.go`. The scroll implementation:

- **Swipe up** → scrolls the conversation view down (shows older messages)
- **Swipe down** → scrolls the conversation view up (shows newer messages)
- Touch events translate directly to scroll position updates on the viewport

### Requirements

- Termux running on Android
- A terminal emulator that forwards touch events (Termux's built-in terminal does this)
- No configuration needed — touch-scroll is always active

### Fallback

Keyboard scroll always works: `PgUp` / `PgDn` scroll by one screen at a time.

---

## 16. One-Shot Mode

Gorkbot can be used non-interactively for scripting, piping, and CI/CD.

### Basic One-Shot

```bash
./gorkbot.sh -p "Summarize this codebase in 3 bullet points"
```

### Reading from Stdin

```bash
cat myfile.txt | ./gorkbot.sh --stdin
echo "What is 2+2?" | ./gorkbot.sh --stdin
```

### Writing Output to File

```bash
./gorkbot.sh -p "Write a Go HTTP server" --output server.go
```

### Filtering Tools

```bash
# Allow only specific tools
./gorkbot.sh -p "List files" --allow-tools bash,list_directory

# Block specific tools
./gorkbot.sh -p "Review this code" --deny-tools bash,write_file,delete_file
```

### Timeout

```bash
./gorkbot.sh -p "Complex analysis" --timeout 120s
```

### Piping into Scripts

```bash
# Generate code and pipe through formatter
./gorkbot.sh -p "Write a Go function to parse JSON" | gofmt

# Use in Makefile
gen-code:
    ./gorkbot.sh -p "Generate boilerplate for $(MODULE)" --output $(MODULE)/gen.go
```
