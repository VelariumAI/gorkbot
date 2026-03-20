# Configuration Guide

Complete reference for all Gorkbot configuration options.

---

## Table of Contents

1. [Configuration Files](#configuration-files)
2. [Application State](#application-state)
3. [Environment Variables](#environment-variables)
4. [Advanced Settings](#advanced-settings)
5. [Project Instructions (GORKBOT.md)](#project-instructions)
6. [Tool Permissions](#tool-permissions)
7. [Theme Configuration](#theme-configuration)

---

## Configuration Files

Gorkbot stores configuration in `~/.config/gorkbot/`:

```
~/.config/gorkbot/
├── .env                          # API keys (gitignored)
├── app_state.json               # User preferences
├── tool_permissions.json        # Per-tool approval levels
├── gorkbot.db                   # SQLite database
├── gorkbot.json                 # Structured logs (slog)
├── mcp.json                     # MCP server configuration
├── theme_light.toml             # Light theme
├── theme_dark.toml              # Dark theme
├── xskill_kb/                   # XSKILL experience bank
├── spark/                       # SPARK daemon data
├── trace/                       # Daily SENSE traces (JSONL)
└── cci/                         # Codified Context Infrastructure
```

---

## Application State

### File: `app_state.json`

Controls user preferences (persistent across sessions).

```json
{
  "primary_provider": "xai",
  "primary_model": "grok-2-1212",
  "secondary_provider": "google",
  "secondary_model": "gemini-2-flash",
  "secondary_auto": false,
  
  "disabled_categories": ["security"],
  "disabled_providers": ["openai"],
  
  "cascade_order": ["xai", "google", "anthropic"],
  "compression_provider": "anthropic",
  
  "sandbox_enabled": true,
  "sre_enabled": true,
  "ensemble_enabled": true,
  "verbose_mode": false,
  
  "suppression_config": {
    "ToolNarration": true,
    "ToolStatus": true,
    "InternalReason": true,
    "DebugInfo": true,
    "SystemStatus": false,
    "CooldownNotice": false
  }
}
```

### Configuration Options

#### Provider Selection

- **`primary_provider`** (string)
  - Default: `"xai"`
  - Options: `xai`, `google`, `anthropic`, `openai`, `openrouter`, `minimax`, `moonshot`
  - Sets the main AI model used for responses

- **`primary_model`** (string)
  - Default: `"grok-2-1212"`
  - Examples: `"grok-2"`, `"gemini-2-flash"`, `"claude-opus-4-6"`
  - Specific model version to use

- **`secondary_provider`** (string)
  - Default: `"google"`
  - Consultant AI for complex reasoning
  - Set to `null` to disable consultant

- **`secondary_model`** (string)
  - Model to use for consultant

- **`secondary_auto`** (boolean)
  - Default: `false`
  - If true, automatically selects best secondary model
  - If false, uses manual selection

#### Model Cascade

- **`cascade_order`** (array of strings)
  - Order for provider fallback
  - Example: `["xai", "google", "anthropic"]`
  - If primary fails, tries next in order
  - Empty/null uses hardcoded default

- **`compression_provider`** (string)
  - Which provider to use for context compression
  - Default: `""` (use primary provider)
  - Set to `"anthropic"` for Claude's compression

#### Tool Configuration

- **`disabled_categories`** (array)
  - Tools to disable
  - Example: `["bash", "security"]`
  - Available: `bash`, `file`, `git`, `web`, `system`, `security`, `meta`

- **`disabled_providers`** (array)
  - Providers to disable
  - Example: `["openai"]`

#### Mode Toggles

- **`sandbox_enabled`** (boolean)
  - Default: `true`
  - Enables SENSE input sanitizer
  - Validates paths, ANSI codes, SQL patterns

- **`sre_enabled`** (boolean)
  - Default: `true`
  - Enables Step-wise Reasoning Engine
  - Hypothesis → test → validate workflow

- **`ensemble_enabled`** (boolean)
  - Default: `true`
  - Enables multi-trajectory ensemble reasoning
  - Provides diverse response options

- **`verbose_mode`** (boolean)
  - Default: `false`
  - If true, shows all internal messages
  - If false, suppresses system narration

#### Message Suppression

- **`suppression_config`** (object)
  - Per-category message filtering
  - Categories:
    - `ToolNarration`: "I will now execute..."
    - `ToolStatus`: "Tool completed"
    - `InternalReason`: "I chose bash because..."
    - `DebugInfo`: "Context tokens: 5000"
    - `SystemStatus`: "Context overflow detected"
    - `CooldownNotice`: "Rate limited..."

---

## Environment Variables

### API Keys

Set via `.env` file or shell environment:

```bash
# Grok (xAI)
export XAI_API_KEY=xai-xxxxxxxxxxxxx

# Gemini (Google)
export GEMINI_API_KEY=AIzaxxxxxxxxxxxxxxx

# Claude (Anthropic)
export ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxx

# OpenAI (GPT)
export OPENAI_API_KEY=sk-proj-xxxxxxxxxxxxx

# OpenRouter
export OPENROUTER_API_KEY=sk-or-xxx

# Minimax
export MINIMAX_API_KEY=xxx

# Moonshot
export MOONSHOT_API_KEY=xxx
```

### Feature Flags

```bash
# Enable native LLM (llamacpp)
export LLAMACPP_ENABLED=true
export LLAMACPP_MODEL=/path/to/model.gguf

# Enable webhook server
export WEBHOOK_PORT=8000
export WEBHOOK_SECRET=your_secret_here
export WEBHOOK_NOTIFY_DISCORD=webhook_url
export WEBHOOK_NOTIFY_TELEGRAM=bot_token

# Enable MCP server debugging
export DEBUG_MCP=true

# Set custom config directory
export GORKBOT_CONFIG=~/.gorkbot/custom

# Set trace directory
export GORKBOT_TRACE=/tmp/gorkbot_traces

# XSKILL settings
export XSKILL_KB=/custom/xskill/path
```

### Billing & Limits

```bash
# Daily budget limit (USD)
export BUDGET_DAILY_LIMIT=10.00

# Per-request budget limit (USD)
export BUDGET_PER_REQUEST=0.50

# Context token limit
export CONTEXT_TOKEN_LIMIT=100000

# Max tool execution time (seconds)
export TOOL_TIMEOUT=300
```

---

## Advanced Settings

### Tool Permissions

File: `tool_permissions.json`

```json
{
  "bash": "once",
  "delete_file": "never",
  "git_push": "session",
  "read_file": "always",
  "web_fetch": "session"
}
```

**Permission Levels**:
- `always`: Approved permanently, saved in config
- `session`: Approved for current session only
- `once`: Ask every time
- `never`: Permanently blocked

### MCP Configuration

File: `mcp.json`

```json
{
  "servers": [
    {
      "name": "gorkbot-introspect",
      "enabled": true,
      "command": "python",
      "args": ["./mcp_servers/gorkbot_introspect.py"]
    },
    {
      "name": "file-system",
      "enabled": true,
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home"]
    }
  ]
}
```

### XSKILL Configuration

Set via environment or in orchestrator:

```go
orch.XSkillKB = xskill.NewKnowledgeBase(
    "~/.gorkbot/xskill_kb",
    provider,
)
```

Data stored in:
- `experiences.json` - Experience bank
- `skills/*.md` - Domain skill documents

---

## Project Instructions (GORKBOT.md)

Create `GORKBOT.md` in your project root for project-specific instructions.

**Example**:

```markdown
# Project-Specific AI Instructions

## Technology Stack
- Language: Go 1.25.0
- Database: SQLite3 with WAL mode
- Framework: Charm Bubble Tea (TUI)

## Code Style
- Prefer Go conventions
- Use `gofmt` for formatting
- Keep functions <50 lines
- Comment exported functions

## Build Process
```bash
make build                  # Build for host OS
make build-all              # Build for all platforms
go test ./...               # Run tests
go vet ./...                # Run linter
```

## Deployment
- Deploy to Linux x86_64 primary
- macOS ARM64 for development
- Windows support optional

## Sensitive Data
- Never hardcode API keys
- Use .env file (gitignored)
- Encrypt long-term credentials

## Decision Log
- When to use SENSE compression
- When to invoke SRE
- Provider selection criteria
```

**Usage**:
- Automatically loaded on session start
- Injected into system prompt
- Watched for live changes (ConfigWatcher)

---

## Running Configuration Commands

### Via TUI

```
/settings              Open settings overlay
/model                 List available models
/theme <theme>         Change theme
/verbose <on|off>      Toggle verbose mode
/tools                 List available tools
/export <file>         Export conversation
/compress              Compress history
```

### Via CLI

```bash
# Run with specific model
gorkbot -model grok-2

# Enable extended thinking
gorkbot -thinking-budget 5000

# Enable watchdog debugging
gorkbot -watchdog

# One-shot with model
gorkbot -p "Question" -model claude-opus-4-6

# Web UI
gorkbot -web -port 8080
```

---

## Best Practices

1. **Security**:
   - Keep .env file gitignored
   - Use encrypted API key storage
   - Regularly rotate credentials
   - Review audit logs periodically

2. **Performance**:
   - Disable unused tool categories
   - Use compression when history grows
   - Enable SRE for complex tasks
   - Use XSKILL for repeated patterns

3. **Customization**:
   - Create GORKBOT.md for project context
   - Customize tool permissions per project
   - Set appropriate compression provider
   - Configure cascade order for preferences

4. **Monitoring**:
   - Check SENSE traces daily: `trace/<date>.jsonl`
   - Monitor SQLite database size
   - Review billing: `SELECT SUM(tokens_out * price) FROM tool_calls`
   - Analyze error patterns

---

**Next Steps**: See [GETTING_STARTED.md](GETTING_STARTED.md) for initial setup.

