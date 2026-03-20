# Getting Started with Gorkbot - Complete Guide

This comprehensive guide covers installation, configuration, and usage of Gorkbot in detail.

---

## 📋 Table of Contents

1. [System Requirements](#system-requirements)
2. [Installation](#installation)
3. [API Key Setup](#api-key-setup)
4. [Running Gorkbot](#running-gorkbot)
5. [TUI Features & Usage](#tui-features--usage)
6. [Slash Commands Reference](#slash-commands-reference)
7. [Configuration](#configuration)
8. [Troubleshooting](#troubleshooting)
9. [Advanced Topics](#advanced-topics)

---

## 🖥️ System Requirements

### Minimum Requirements
- **OS**: Linux, macOS, Windows, or Android (Termux)
- **Go**: 1.25.0 or newer (for building from source)
- **RAM**: 256 MB minimum (512 MB recommended)
- **Disk**: 100 MB free space (for binary + caches)
- **Network**: Internet connection required (for API access)

### Platform-Specific Notes

#### Linux
- Tested on Ubuntu 20.04+, Debian 11+, Fedora 35+
- Requires: build-essential (or equivalent)

#### macOS
- Tested on macOS 11.0+
- Xcode Command Line Tools required (`xcode-select --install`)

#### Windows
- Tested on Windows 10/11
- No additional requirements beyond Go

#### Android (Termux)
- Requires Termux app (from F-Droid)
- Install: `pkg install go git make`
- Storage access recommended for file operations

### Dependencies (Auto-Installed)
All Go dependencies are automatically fetched via `go mod download`. See `go.mod` for complete list.

---

## 📦 Installation

### Method 1: Build from Source (Recommended)

#### Step 1: Clone Repository
```bash
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
```

#### Step 2: Build the Application
```bash
# Standard build (outputs to ./bin/gorkbot)
make build

# Or use Go directly
go build -o bin/gorkbot ./cmd/gorkbot/

# Cross-platform builds available:
make build-linux      # Linux amd64
make build-windows    # Windows amd64
make build-android    # Android arm64
```

Verify build:
```bash
./bin/gorkbot --version
# Output: Gorkbot version 5.3.0 (public: 1.2.0-beta)
```

#### Step 3: Install (Optional)
```bash
# Option A: Install to GOPATH/bin (default: ~/go/bin)
make install

# Option B: Copy to system location
sudo cp bin/gorkbot /usr/local/bin/

# Option C: Keep in project directory (use ./gorkbot.sh)
# Already set up, just use: ./gorkbot.sh
```

### Method 2: Docker (If Available)
```bash
# Build image (if Dockerfile available)
docker build -t gorkbot .

# Run in container
docker run -it -e XAI_API_KEY=$XAI_API_KEY gorkbot
```

### Method 3: Download Binary
Check releases page for pre-built binaries (if available):
```bash
# Example (check actual URLs)
wget https://github.com/velariumai/gorkbot/releases/download/v1.2.0-beta/gorkbot-linux-amd64
chmod +x gorkbot-linux-amd64
./gorkbot-linux-amd64
```

---

## 🔑 API Key Setup

### Quick Setup (2 Minutes)

#### Automated Setup Wizard
```bash
./gorkbot.sh setup
```

The wizard will:
1. Detect available providers
2. Prompt for API keys one by one
3. Validate keys by making test API calls
4. Store keys in `.env` file
5. Confirm successful configuration

#### Example Wizard Session
```
╔═══════════════════════════════════════════════════════════════╗
║             Welcome to Gorkbot Setup Wizard! 🚀              ║
╚═══════════════════════════════════════════════════════════════╝

This wizard will help you configure your API keys.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Step 1: xAI API Key (for Grok) [REQUIRED]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📍 Get your xAI API key from:
   https://console.x.ai/

📋 Paste your xAI API key (press Enter when done):
   xai-xxxxxxxxxxxxxxxxxxxxxxxxxxxxx

✓ Key validated successfully!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Step 2: Google Gemini API Key (for Gemini) [REQUIRED]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📍 Get your Gemini API key from:
   https://aistudio.google.com/apikey

📋 Paste your Gemini API key:
   AIzaxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

✓ Key validated successfully!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Step 3: Anthropic Claude API Key (for Claude) [OPTIONAL]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📍 Get your Anthropic API key from:
   https://console.anthropic.com/

📋 Paste your Anthropic API key (or press Enter to skip):
   sk-ant-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

✓ Key validated successfully!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✓ xAI (Grok):          Configured
✓ Google (Gemini):     Configured
✓ Anthropic (Claude):  Configured
✗ OpenAI (GPT):        Not configured (optional)
✗ MiniMax:             Not configured (optional)

📁 Keys saved to: .env

✅ Setup complete! Run: ./gorkbot.sh
```

### Getting API Keys

#### xAI (Grok) - REQUIRED
1. Go to **https://console.x.ai/**
2. Sign in with your account (create one if needed)
3. Navigate to **"API Keys"** section
4. Click **"Create API Key"**
5. Copy the key (starts with `xai-`)
6. Paste into setup wizard or `.env` file

**Pricing**: Pay-as-you-go, $0.05 per 1M input tokens, $0.15 per 1M output tokens (approximate)

**Models Available**:
- `grok-3-mini` (fast, efficient)
- `grok-3-vision-1212` (vision-capable)
- `grok-3-thinking-1212` (extended reasoning)

#### Google Gemini - REQUIRED
1. Go to **https://aistudio.google.com/apikey**
2. Sign in with your Google account
3. Click **"Create API Key"**
4. Select "Create API key in new project" (or use existing)
5. Copy the key (starts with `AIza`)
6. Paste into setup wizard or `.env` file

**Pricing**: Free tier includes 15,000 requests/day for Gemini 1.5 Flash. Upgrade for more.

**Models Available**:
- `gemini-2.0-pro` (latest, most capable)
- `gemini-2.0-flash` (faster, lower cost)
- `gemini-1.5-pro` (previous generation)
- `gemini-1.5-flash` (fast, efficient)

#### Anthropic Claude - OPTIONAL (Recommended)
1. Go to **https://console.anthropic.com/**
2. Sign in or create account
3. Go to **"API Keys"** (left sidebar)
4. Click **"Create Key"**
5. Copy the key (starts with `sk-ant-`)
6. Paste into setup wizard or `.env` file

**Pricing**: $3 per 1M input tokens, $15 per 1M output tokens (approximate)

**Models Available**:
- `claude-opus-4-1` (most capable)
- `claude-sonnet-4` (balanced)
- `claude-3.7` (with extended thinking)

#### OpenAI GPT - OPTIONAL
1. Go to **https://platform.openai.com/api-keys**
2. Sign in or create account
3. Click **"Create new secret key"**
4. Copy the key (starts with `sk-`)
5. Paste into setup wizard or `.env` file

**Pricing**: Varies by model. GPT-4 is more expensive than GPT-3.5.

**Models Available**:
- `gpt-4-turbo` (most capable)
- `gpt-4o` (faster GPT-4)
- `gpt-4-1106-preview` (with vision)
- `gpt-3.5-turbo` (fast, low cost)

#### MiniMax - OPTIONAL
1. Go to **https://www.minimaxi.com/**
2. Sign up and verify email
3. Create API key in account settings
4. Copy the key
5. Paste into setup wizard or `.env` file

**Pricing**: Similar to GPT rates

**Models Available**:
- `minimax-01` (MiniMax's flagship)

### Manual Configuration

Create `.env` file in project root:
```bash
# Copy from example
cp .env.example .env

# Edit with your favorite editor
nano .env
# or
vim .env
```

Edit `.env`:
```env
# xAI API Key (for Grok) - REQUIRED
XAI_API_KEY=xai-your-actual-key-here

# Google API Key (for Gemini) - REQUIRED
GEMINI_API_KEY=AIza-your-actual-key-here

# Anthropic API Key (for Claude) - OPTIONAL
ANTHROPIC_API_KEY=sk-ant-your-actual-key-here

# OpenAI API Key (for GPT) - OPTIONAL
OPENAI_API_KEY=sk-your-actual-key-here

# MiniMax API Key - OPTIONAL
MINIMAX_API_KEY=your-actual-key-here
```

**Important Security Notes:**
- ❌ **NEVER** commit `.env` to git (it's in `.gitignore`)
- ❌ **NEVER** share your API keys
- ✅ Keep `.env` file permissions private (0600)
- ✅ Rotate keys regularly
- ✅ Use environment variables for sensitive deployments

### Verify Configuration

```bash
# Check configuration status
./gorkbot.sh status
```

Output:
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Gorkbot Configuration Status
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🤖 Grok (xAI):
   ✓ Configured (xai-****...****) [v5.3.0]
   Models available: grok-3-mini, grok-3-vision-1212

💎 Gemini (Google):
   ✓ Configured (AIza****...****)
   Models available: gemini-2.0-pro, gemini-2.0-flash, gemini-1.5-pro

🧠 Claude (Anthropic):
   ✓ Configured (sk-ant****...****)
   Models available: claude-opus-4-1, claude-sonnet-4

🚀 GPT (OpenAI):
   ✗ Not configured
   To use: Add OPENAI_API_KEY to .env

📦 MiniMax:
   ✗ Not configured
   To use: Add MINIMAX_API_KEY to .env

────────────────────────────────────────────────────────────────
  Features Available
────────────────────────────────────────────────────────────────

✓ Multi-provider support (3 providers)
✓ Tool system (75+ tools)
✓ Session management
✓ Extended thinking (Claude 3.7+)
✓ Vision/image analysis
✓ Web browsing
✓ File operations
✓ Git integration
✓ Code execution
✓ Security tools
✓ Android-specific tools

✓ All systems ready!

────────────────────────────────────────────────────────────────
Run: ./gorkbot.sh
────────────────────────────────────────────────────────────────
```

---

## 🚀 Running Gorkbot

### Interactive TUI Mode (Primary Usage)

#### Basic Startup
```bash
./gorkbot.sh
```

**What You'll See:**
- Terminal UI with conversation area (top)
- Status bar (bottom) showing: context%, cost, mode, git branch
- Input field for typing messages
- Markdown-rendered responses with syntax highlighting

#### Available Modes
```bash
# Interactive TUI (default)
./gorkbot.sh

# One-shot mode (quick query, no interactive UI)
./gorkbot.sh -p "What is quantum computing?"

# Setup/configuration
./gorkbot.sh setup

# Check configuration
./gorkbot.sh status

# Advanced debugging
./gorkbot.sh -watchdog              # Show orchestrator state
./gorkbot.sh -verbose-thoughts      # Show consultant thinking
./gorkbot.sh --trace                # Enable execution tracing
./gorkbot.sh -timeout 120s          # Set timeout for query
```

### One-Shot Mode (Scripting)

Perfect for scripts, cron jobs, and CI/CD pipelines:

```bash
# Simple query
./gorkbot.sh -p "Hello, how are you?"

# With output to file
./gorkbot.sh -p "Generate a Python script" > script.py

# With timeout
timeout 30s ./gorkbot.sh -p "Quick question"

# In bash script
RESPONSE=$(./gorkbot.sh -p "What time is it?")
echo "AI said: $RESPONSE"
```

### Using with Environment Variables

Instead of `.env` file:
```bash
export XAI_API_KEY="your-key"
export GEMINI_API_KEY="your-key"
./bin/gorkbot  # Direct execution
```

### Using Systemd (Linux)

Create systemd service:
```bash
sudo nano /etc/systemd/system/gorkbot.service
```

```ini
[Unit]
Description=Gorkbot AI Chatbot
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=/home/$USER/gorkbot
ExecStart=/home/$USER/gorkbot/gorkbot.sh
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable gorkbot
sudo systemctl start gorkbot
sudo systemctl status gorkbot
```

---

## 💬 TUI Features & Usage

### Main Interface Layout

```
┌─────────────────────────────────────────────────────────────────┐
│ Gorkbot AI Chatbot                                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ You: What is the capital of France?                            │
│                                                                 │
│ Assistant: The capital of France is Paris. It is the largest   │
│ city in France and has a population of about 2.2 million...    │
│                                                                 │
│ [Scrollable conversation history - PgUp/PgDn to navigate]      │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ > _                                                              │
│ ┌─────────────────────────────────────────────────────────────┐
│ │ [Input field - Type your message here, Alt+Enter for lines] │
│ └─────────────────────────────────────────────────────────────┘
├─────────────────────────────────────────────────────────────────┤
│ [45%] Grok (grok-3-mini) | Cost: $0.02 | Normal | main        │
└─────────────────────────────────────────────────────────────────┘
```

### Keyboard Shortcuts Reference

| Key | Action | When Used |
|-----|--------|-----------|
| **Enter** | Send message | In input field |
| **Alt+Enter** | New line | In input field (multi-line) |
| **Ctrl+C** | Quit Gorkbot | Anytime |
| **Ctrl+T** | Model selection | Anytime |
| **Ctrl+G** | Settings overlay | Anytime |
| **Ctrl+E** | Tools table | Anytime |
| **Ctrl+D** | Cloud Brains (discovery) | Anytime |
| **Ctrl+A** | Analytics dashboard | Anytime |
| **Ctrl+\\** | Diagnostics view | Anytime |
| **Ctrl+X** | Interrupt generation | During AI response |
| **Ctrl+P** | Cycle execution mode | Anytime |
| **PgUp** | Scroll up (history) | In conversation |
| **PgDn** | Scroll down (history) | In conversation |
| **Home** | Jump to top | In conversation |
| **End** | Jump to bottom | In conversation |
| **Esc** | Cancel/close overlay | In overlay/modal |
| **Tab** | Switch pane | In model selection |

### Text Input Features

#### Single-Line Input
```bash
> How do I learn Go programming?
[Press Enter to send]
```

#### Multi-Line Input
```bash
> [First line of message
> Second line of message
> Press Alt+Enter for new lines]
[Press Enter to send when done]
```

#### Markdown Support in Input
- **Bold**: `**text**`
- *Italic*: `*text*`
- `Code`: `` `code` ``
- Lists work too with `-` or `*`

### Model Selection Interface

Press **Ctrl+T** to open dual-pane model selector:

```
┌──────────────────────────────────────────────────────────────┐
│                    Model Selection                            │
├────────────────────────┬────────────────────────────────────┤
│ PRIMARY (Current)      │ SECONDARY (Consultant)             │
├────────────────────────┼────────────────────────────────────┤
│ ► grok-3-mini          │   gemini-2.0-pro                   │
│   grok-3-vision        │   gemini-2.0-flash                 │
│   grok-3-thinking      │   claude-opus-4-1                  │
│   gemini-2.0-pro       │   claude-sonnet-4                  │
│   claude-opus-4-1      │   gpt-4-turbo                      │
│   gpt-4-turbo          │   gpt-4o                           │
│ [More...]             │ [More...]                          │
├────────────────────────┴────────────────────────────────────┤
│ Tab=Switch Pane | ↑↓=Navigate | Enter=Select | p=Provider   │
│ r=Refresh | k=Add Key | q=Quit                              │
└───────────────────────────────────────────────────────────────┘
```

**Controls:**
- **Tab** - Switch between primary/secondary pane
- **↑/↓** - Navigate list
- **Enter** - Select model
- **p** - Filter by provider
- **r** - Refresh model list
- **k** - Add API key
- **q** or **Esc** - Close selector

### Settings Overlay

Press **Ctrl+G** to open settings:

```
┌─────────────────────────────────────────────────────────────┐
│                        SETTINGS                              │
├─────────────────────────────────────────────────────────────┤
│ ☑ Model Routing    □ Verbosity    □ Tool Groups  □ Providers│
├─────────────────────────────────────────────────────────────┤
│                                                               │
│ MODEL ROUTING SETTINGS                                       │
│ ────────────────────────────────────────────────────────────│
│                                                               │
│ Primary Model:         grok-3-mini                           │
│   [Change]                                                    │
│                                                               │
│ Secondary Model:       gemini-2.0-pro                        │
│   [Change]  [Auto-Select]                                    │
│                                                               │
│ Auto-Consultant:       Enabled                               │
│   Triggers: COMPLEX keyword, complex queries                │
│                                                               │
│ Fallback Chain:        grok-3-mini → gemini-2.0-pro         │
│                                                               │
├─────────────────────────────────────────────────────────────┤
│ [Save]  [Reset]  [Help]  [Close - Ctrl+G]                   │
└─────────────────────────────────────────────────────────────┘
```

Tabs:
1. **Model Routing** - Primary/secondary/fallback selection
2. **Verbosity** - Output detail level, thinking visibility
3. **Tool Groups** - Enable/disable tool categories
4. **Providers** - API key management and provider status

### Tools Table View

Press **Ctrl+E** to see available tools:

```
┌────────────────────────────────────────────────────────────┐
│                      TOOLS (75+)                            │
├────────────────────────────────────────────────────────────┤
│ Category        Tool Name           Status       Cache      │
├────────────────────────────────────────────────────────────┤
│ FILE            read_file            ✓ Allow      No        │
│ FILE            write_file           ✓ Allow      No        │
│ FILE            list_directory       ✓ Allow      Yes (5m)  │
│ GIT             git_status           ✓ Allow      Yes (10m) │
│ GIT             git_commit           ⚠ Ask        No        │
│ WEB             web_fetch            ✓ Allow      Yes (1h)  │
│ SHELL           bash                 ✗ Block      No        │
│ SYSTEM          system_info          ✓ Allow      Yes       │
│ SECURITY        nmap_scan            ⚠ Ask        No        │
│ [More tools...]                                              │
├────────────────────────────────────────────────────────────┤
│ Legend: ✓=Auto, ⚠=Ask, ✗=Blocked | q=Quit | /=Search      │
└────────────────────────────────────────────────────────────┘
```

---

## ⌨️ Slash Commands Reference

### Command Categories

#### Information Commands
```bash
/help          # Show all available commands and keyboard shortcuts
/version       # Show Gorkbot version and subsystem versions
/diagnostic    # Show system diagnostics and health status
/models        # List all available models
/tools         # Show tools table (same as Ctrl+E)
```

#### Model & Provider Commands
```bash
/model <name>           # Switch primary model
                        # Examples: /model grok-3-mini
                        #           /model claude-opus-4-1

/model                  # Open model selection view (Ctrl+T)

/key <provider> <key>   # Add/update API key
                        # Example: /key xai xai-xxxxx...

/key status             # Show API key status for all providers

/key validate <provider> # Validate a provider's API key

/thinking-budget <tokens> # Set extended thinking token budget
                          # Example: /thinking-budget 10000
                          # Use 0 to disable
```

#### Conversation & Session Commands
```bash
/clear                  # Clear conversation history (fresh start)

/quit                   # Exit Gorkbot (same as Ctrl+C)

/checkpoint             # Save conversation checkpoint

/rewind <id>            # Rewind to previous checkpoint
                        # List available: /checkpoint

/export <format>        # Export conversation
                        # Formats: markdown, json, plain
                        # Example: /export markdown

/compact                # Compact conversation (remove redundancy)

/rename <new-name>      # Rename current conversation

/save                   # Save conversation to database

/load <name>            # Load previous conversation
```

#### Execution & Routing Commands
```bash
/mode <type>            # Switch execution mode
                        # Types: normal, plan, auto
                        # normal: Interactive (default)
                        # plan: Generate plan first
                        # auto: Execute without pauses

/context                # Show context window statistics
                        # Displays: Used/Total tokens, % full

/cost                   # Show token usage and estimated costs
                        # By provider and model

/rate <1-5>             # Rate last response (for adaptive routing)
                        # Used to improve future model selection
```

#### Configuration Commands
```bash
/settings               # Open settings overlay (Ctrl+G)

/theme <name>           # Change theme
                        # Available: dracula, nord, gruvbox,
                        #            solarized, monokai, auto

/rules                  # Configure permission rules
                        # Fine-grained tool access control

/hooks                  # Manage lifecycle hooks
                        # Customize behavior at key events

/compress               # Compress conversation context
                        # Reduces token count by removing redundancy
```

#### Advanced/Experimental Commands
```bash
/skills                 # Manage dynamic skills
                        # List/enable/disable custom skills

/sandbox <code>         # Run code in isolated sandbox
                        # Python, Go, Node.js, etc.

/mcp [status|config]    # MCP server management
                        # status: Show connected servers
                        # config: Edit MCP config

/share [start|stop]     # Session relay for collaboration
                        # start: Enable relay server
                        # stop: Disable relay

/billing                # Show detailed billing breakdown
                        # Tokens, costs by provider

/spawn-agent <task>     # Spawn background agent
                        # Execute task in background
```

### Example Command Workflows

#### Switching Models
```bash
User: /model grok-3-vision
Gorkbot: Model switched to grok-3-vision
```

#### Using Extended Thinking
```bash
User: /thinking-budget 15000
Gorkbot: Extended thinking enabled (15000 tokens)

User: Explain quantum entanglement in detail
[Gorkbot shows thinking progress, then full response]
```

#### Exporting Conversation
```bash
User: /export markdown
Gorkbot: Conversation exported to conversation_20260320.md (2.5 KB)
```

#### Setting Up Relay
```bash
User: /share start
Gorkbot: Relay server started at ws://localhost:9876
        Share this URL with others to collaborate

User: /share stop
Gorkbot: Relay server stopped
```

---

## ⚙️ Configuration

### Configuration Files & Locations

| File | Location | Purpose | Editable |
|------|----------|---------|----------|
| `.env` | Project root | API keys | ✓ Yes |
| `app_state.json` | ~/.config/gorkbot/ | Model selection, disabled categories | Via commands |
| `api_keys.json` | ~/.config/gorkbot/ | Encrypted API key storage | Via `/key` command |
| `tool_permissions.json` | ~/.config/gorkbot/ | Tool permission levels | Via permission prompts |
| `active_theme` | ~/.config/gorkbot/ | Current theme | Via `/theme` command |
| `themes/*.json` | ~/.config/gorkbot/themes/ | Custom theme definitions | ✓ Yes |
| `mcp.json` | ~/.config/gorkbot/ | MCP server config | ✓ Yes |
| `hooks/` | ~/.config/gorkbot/hooks/ | Lifecycle hook scripts | ✓ Yes |

### Platform-Specific Paths

Gorkbot automatically detects platform and uses appropriate paths:

**Linux/macOS:**
```
~/.config/gorkbot/              # Config directory
~/.local/share/gorkbot/         # Data directory
~/.local/share/gorkbot/traces/  # Trace logs (if enabled)
```

**Windows:**
```
%APPDATA%\gorkbot\              # Config directory (usually C:\Users\...\AppData\Roaming\gorkbot)
%LOCALAPPDATA%\gorkbot\         # Data directory (usually C:\Users\...\AppData\Local\gorkbot)
```

**Android (Termux):**
```
~/.config/gorkbot/              # Config (in Termux home)
~/.local/share/gorkbot/         # Data
/data/data/com.termux/...       # App private storage (if integrated)
```

### Environment Variables

Override configuration via environment:
```bash
# API Keys
export XAI_API_KEY="your-key"
export GEMINI_API_KEY="your-key"
export ANTHROPIC_API_KEY="your-key"

# Logging
export GORKBOT_LOG_LEVEL="debug"    # Set log level
export GORKBOT_LOG_FILE="/tmp/gorkbot.log"

# Behavior
export GORKBOT_TIMEOUT="60s"        # Default timeout
export GORKBOT_MAX_RETRIES="3"      # API retry count
```

### Theme Configuration

#### Built-In Themes
```bash
/theme dracula      # Dark with blue accent
/theme nord         # Arctic colors, minimalist
/theme gruvbox      # Retro, warm colors
/theme solarized    # High contrast, scientific
/theme monokai       # Dark, vibrant
/theme auto         # Auto-detect (light/dark based on terminal)
```

#### Custom Themes
Create theme file at `~/.config/gorkbot/themes/my-theme.json`:

```json
{
  "name": "My Custom Theme",
  "background": "#1e1e1e",
  "foreground": "#d4d4d4",
  "primary": "#569cd6",
  "accent": "#ce9178",
  "error": "#f48771",
  "success": "#6a9955"
}
```

Apply:
```bash
/theme my-theme
```

---

## 🐛 Troubleshooting

### Common Issues & Solutions

#### "API Key is not set"
**Problem**: Gorkbot can't find API keys

**Solutions**:
1. Run setup wizard:
   ```bash
   ./gorkbot.sh setup
   ```

2. Or manually create `.env`:
   ```bash
   cp .env.example .env
   nano .env  # Add your keys
   ```

3. Verify `.env` exists and is readable:
   ```bash
   cat .env | grep XAI_API_KEY
   ```

4. Ensure you're using wrapper script:
   ```bash
   ./gorkbot.sh    # ✓ Loads .env
   ./bin/gorkbot   # ✗ Doesn't load .env
   ```

#### "Connection refused" or "Network error"
**Problem**: Can't reach API endpoints

**Solutions**:
1. Check internet connection:
   ```bash
   ping 8.8.8.8
   ```

2. Check API status pages:
   - xAI: https://status.x.ai/
   - Google: https://status.cloud.google.com/
   - Anthropic: https://status.anthropic.com/

3. Try different provider:
   ```bash
   /model gemini-2.0-pro  # Try Gemini instead of Grok
   ```

4. Check firewall/proxy settings:
   ```bash
   curl https://api.x.ai/health  # Test connectivity
   ```

#### "Invalid API Key"
**Problem**: API rejects your key

**Solutions**:
1. Verify key is correct:
   ```bash
   grep XAI_API_KEY .env
   ```

2. Check for extra spaces or quotes:
   ```env
   XAI_API_KEY=xai-xxx... # ✓ Correct
   XAI_API_KEY="xai-xxx..." # ✗ Has quotes
   ```

3. Regenerate key from provider:
   - xAI: https://console.x.ai/api-keys
   - Gemini: https://aistudio.google.com/apikey

4. Update key:
   ```bash
   ./gorkbot.sh setup
   # Or
   /key xai your-new-key
   ```

#### "Rate limit exceeded"
**Problem**: Too many API requests

**Solutions**:
1. Wait a few minutes before retrying
2. Upgrade your API plan
3. Use a faster/cheaper model:
   ```bash
   /model grok-3-mini    # Cheaper than vision
   /model gemini-2.0-flash  # Cheaper than pro
   ```

4. Reduce context:
   ```bash
   /compress           # Compress conversation
   /clear              # Clear and start fresh
   ```

#### "TUI display broken" or "Garbled text"
**Problem**: Terminal UI rendering issues

**Solutions**:
1. Resize terminal to wider width (80+ characters minimum)
2. Clear screen:
   ```bash
   Ctrl+L  # Or clear command
   ```

3. Check terminal encoding:
   ```bash
   echo $LANG
   # Should be: en_US.UTF-8 or similar
   ```

4. Try different terminal:
   - Linux: Try GNOME Terminal, Konsole, or Alacritty
   - macOS: Try iTerm2
   - Windows: Try Windows Terminal

5. Disable complex rendering:
   ```bash
   /theme mono  # Simple monochrome theme
   ```

#### "Tools not executing" or "Permission denied"
**Problem**: Tool execution fails

**Solutions**:
1. Check tool availability:
   ```bash
   /tools                # Show tools table
   ```

2. Check permissions:
   - If tool shows `⚠ Ask`: You'll be prompted on first use
   - If tool shows `✗ Block`: Tool is disabled, enable via `/rules`

3. Check tool permissions file:
   ```bash
   cat ~/.config/gorkbot/tool_permissions.json
   ```

4. Temporarily allow tool:
   ```bash
   /rules              # Configure permissions
   # Or answer "always" when prompted
   ```

5. Check tool logs:
   ```bash
   tail -f ~/.local/share/gorkbot/gorkbot.json | jq '.tool_executions'
   ```

#### "Out of memory" or "Slow responses"
**Problem**: System running slow

**Solutions**:
1. Compress conversation context:
   ```bash
   /compress
   ```

2. Clear old conversations:
   ```bash
   /clear              # Start fresh
   ```

3. Reduce model complexity:
   ```bash
   /model grok-3-mini  # Use faster/smaller model
   ```

4. Check system resources:
   ```bash
   free -h             # Linux
   top                 # Monitor processes
   ```

#### "On Android/Termux: Keyboard disappears"
**Problem**: Virtual keyboard closes during response

**Solutions**:
1. Tap input field to restore keyboard
2. Press Tab key to focus input
3. Try different keyboard app (Hacker's Keyboard, Termux Keyboard)
4. Disable Termux extra keys: Settings → Keyboard → Extra Keys (off)

---

## 📚 Advanced Topics

### Extended Thinking Support

For models supporting extended thinking (Grok-3, Claude 3.7+):

```bash
# Enable with token budget
/thinking-budget 15000

# Ask a complex question
User: Explain quantum computing, quantum entanglement, and quantum gates in detail

# Response includes:
# [Thinking: <detailed reasoning process>]
# Final Answer: <comprehensive response>
```

**Use Cases:**
- Complex problem-solving
- Mathematical proofs
- Algorithm design
- Technical analysis

**Cost:** Thinking tokens cost same as input tokens

### Plan Mode Execution

Generate a plan first, then execute step-by-step:

```bash
/mode plan

User: Build a web scraper for news articles

Gorkbot generates:
  PLAN:
  1. Choose web scraping library
  2. Design data model
  3. Write scraper code
  4. Add error handling
  5. Test with sample site

User: [Reviews plan]
> Looks good, proceed

Gorkbot executes each step with reasoning
```

### Session Relay & Collaboration

Share your session for real-time collaboration:

```bash
# In session 1 (primary)
/share start
# Output: ws://localhost:9876/session/abc123

# In session 2 (observer)
./gorkbot.sh --join ws://localhost:9876/session/abc123
# Observer sees all messages in real-time
```

### Custom Tool Creation

Create tools dynamically (AI or manual):

```bash
# Via AI
User: Create a tool that counts lines of code in a file

# Or manually:
User: /create-tool
Tool Name: count_lines
Description: Count lines of code
Command: wc -l {{file}}
Parameters: {file: string}

# Tool saved to: pkg/tools/custom/count_lines.go
# Available immediately in next query
```

### MCP Integration

Integrate Model Context Protocol servers:

```bash
# Configure in ~/.config/gorkbot/mcp.json
{
  "servers": [
    {
      "name": "my-server",
      "command": "python",
      "args": ["/path/to/server.py"]
    }
  ]
}

# Restart Gorkbot to load new servers
/mcp status  # Verify servers loaded

# Tools from MCP servers available automatically
# Named as: mcp_<server>_<toolname>
```

### Offline Usage

Gorkbot requires API access for AI responses, but can still:
- Browse local files
- Run git commands
- Execute shell commands
- Manage sessions and tools

```bash
# These work offline:
User: Read my project's README.md
User: Show git log for last 10 commits
User: List files in directory
```

### Batch Processing

Use one-shot mode for batch processing:

```bash
#!/bin/bash
# Batch process multiple queries

for query in "query1" "query2" "query3"; do
  echo "Processing: $query"
  ./gorkbot.sh -p "$query" >> results.txt
  sleep 2  # Rate limiting
done
```

---

## 🎓 Learning Resources

### Recommended Workflow for Beginners

1. **Day 1**: Setup and basic chat
   - Run setup wizard
   - Chat in TUI mode
   - Try a few slash commands (`/help`, `/model`)

2. **Day 2**: Explore features
   - Try different models (`/model`)
   - Use tools for file operations
   - Check `/tools` table

3. **Day 3**: Advanced usage
   - Try extended thinking (`/thinking-budget`)
   - Use plan mode (`/mode plan`)
   - Export conversations (`/export`)

4. **Week 2+**: Integration
   - Use in scripts (one-shot mode)
   - Create custom tools
   - Integrate MCP servers

### Tips for Effective Usage

1. **Be specific in queries**: More detail = better answers
2. **Use slash commands**: `/help` for full list
3. **Check context**: `/context` to see token usage
4. **Try different models**: Some are better for certain tasks
5. **Use extended thinking**: For complex problems
6. **Save checkpoints**: `/checkpoint` before risky operations
7. **Check tools available**: `/tools` before asking
8. **Rate responses**: `/rate 5` helps improve future answers

---

## 📞 Getting Help

1. **Built-in help**:
   ```bash
   /help              # Show all commands
   /version           # Version info
   /diagnostic        # System diagnostics
   ```

2. **Documentation**:
   - README.md - Overview and features
   - CLAUDE.md - Architecture and development
   - docs/ directory - Detailed guides

3. **Logs**:
   ```bash
   tail -f ~/.local/share/gorkbot/gorkbot.json
   ```

4. **Debugging**:
   ```bash
   ./gorkbot.sh -watchdog         # Orchestrator state
   ./gorkbot.sh -verbose-thoughts # Thinking output
   ./gorkbot.sh --trace           # Execution trace
   ```

---

**Happy chatting with Gorkbot! 🚀**
