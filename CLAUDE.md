# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Grokster is a Go-based AI chatbot that integrates xAI's Grok and Google's Gemini models in an **orchestrator pattern** where Grok serves as the primary AI and Gemini acts as an architectural consultant. The application features a terminal UI built with the Charm Bracelet stack (Bubble Tea, Lip Gloss, Glamour, Bubbles) and includes a comprehensive tool system enabling AI agents to perform file operations, git commands, web requests, and system management tasks.

## Build and Development

### Building

```bash
# Build for host OS (outputs to ./bin/grokster)
make build

# Cross-platform builds
make build-windows    # Windows amd64
make build-android    # Android arm64
make build-linux      # Linux amd64

# Clean build artifacts
make clean

# Install to GOPATH/bin
make install
```

### Running

```bash
# Interactive TUI mode (requires .env with API keys)
./grokster.sh

# One-shot mode
./grokster.sh -p "Your question here"

# Setup wizard (configure API keys)
./grokster.sh setup

# Check configuration status
./grokster.sh status

# Enable verbose consultant thinking
./grokster.sh -verbose-thoughts

# Enable orchestrator state debugging
./grokster.sh -watchdog
```

**Important**: Always use `./grokster.sh` wrapper script (not `./bin/grokster` directly) to ensure `.env` file is loaded.

### Testing

```bash
# Run all tests
go test ./...

# Test specific package
go test ./pkg/tools

# Test with verbose output
go test -v ./pkg/ai
```

## Architecture

### Core Pattern: Orchestrator + A2A Communication

The **orchestrator** (`internal/engine/orchestrator.go`) coordinates interactions between:
- **Grok** (primary AI): Main conversational agent
- **Gemini** (consultant): Architectural advisor for complex queries

When users include keywords like "COMPLEX" or "REFRESH", or when Grok detects architectural questions, it uses the **A2A (Agent-to-Agent) protocol** (`pkg/a2a/`) to consult Gemini. The A2A channel supports:
- Query/Response messages
- Notifications
- Tool execution requests between agents
- Message threading with ReplyTo references

### Tool System Architecture

The tool system (`pkg/tools/`) provides **28 comprehensive tools** across 7 categories:

**Categories**:
- Shell (1): `bash`
- File (7): `read_file`, `write_file`, `list_directory`, `search_files`, `grep_content`, `file_info`, `delete_file`
- Git (6): `git_status`, `git_diff`, `git_log`, `git_commit`, `git_push`, `git_pull`
- Web (6): `web_fetch`, `http_request`, `check_port`, `download_file`, `browser_scrape`, `browser_control`
- System (6): `list_processes`, `kill_process`, `env_var`, `system_info`, `disk_usage`
- Security (32): `nmap_scan`, `sqlmap_scan`, `nuclei_scan`, `totp_generate`, and 28 other specialized pentesting tools
- Meta (3): `list_tools`, `tool_info`, `create_tool`
- Custom: User-generated tools via `create_tool` (DIY tool creator)

### Specialized Security Subagents

Gorkbot features an augmented suite of security-focused subagents for advanced assessments:
- **`redteam-recon`**: Gorkbot Security Recon (attack surface mapping)
- **`redteam-injection`**: Gorkbot Injection Analyst (SQLi, Command Injection)
- **`redteam-xss`**: Gorkbot XSS Specialist (DOM-based and Reflected XSS)
- **`redteam-auth`**: Gorkbot Auth Specialist (Authentication & Session flaws)
- **`redteam-ssrf`**: Gorkbot SSRF Specialist (Server-Side Request Forgery)
- **`redteam-authz`**: Gorkbot Authz Specialist (IDOR & Privilege Escalation)
- **`redteam-reporter`**: Gorkbot Security Reporter (Consolidated assessment reports)

**Permission System** (`pkg/tools/permissions.go`):
- **always**: Permanent approval (persisted to `~/.config/grokster/tool_permissions.json`)
- **session**: Approved for current session only
- **once**: Ask each time (default for destructive operations)
- **never**: Permanently blocked

The permission manager ensures secure tool execution with:
- Persistent storage (0600 file permissions)
- Shell command escaping via `shellescape()`
- Execution timeouts
- Parameter validation

### TUI Architecture

The TUI (`internal/tui/`) follows the **Elm architecture** (Model-View-Update):
- `model.go`: State management (conversation history, input buffer, viewport)
- `update.go`: Event handling (keyboard input, token streaming, commands)
- `view.go`: Rendering (markdown, consultant boxes, status bar)
- `style.go`: Lip Gloss styles and themes
- `messages.go`: Custom message types (`TokenMsg`, `ErrorMsg`, etc.)
- `phrases.go`: Loading phrases that rotate every 3 seconds during generation

**Slash Commands** (`pkg/commands/`):
- `/clear`, `/help`, `/model`, `/tools`, `/auth`, `/settings`, `/version`, `/quit`, `/theme`, `/compress`

**Consultant UI**: Gemini responses render in distinctive purple-bordered boxes to visually distinguish architectural advice from Grok responses.

## Key Directories

- `cmd/grokster/` - Main entry point, setup wizard, CLI flags
- `internal/engine/` - Orchestrator coordinating Grok/Gemini interactions
- `internal/platform/` - Environment abstraction (OS, Termux, paths)
- `internal/tui/` - Terminal UI (Bubble Tea app)
- `pkg/ai/` - AI provider interfaces and implementations (Grok, Gemini)
- `pkg/a2a/` - Agent-to-Agent communication protocol
- `pkg/auth/` - OAuth authentication and token storage
- `pkg/commands/` - TUI slash command registry
- `pkg/tools/` - Tool system (registry, permissions, 28 tools)
- `pkg/tools/custom/` - Dynamically created custom tools

## Important Concepts

### Environment Configuration

The platform package (`internal/platform/env.go`) abstracts environment differences:
- Detects OS, architecture, Termux environment
- Provides standard paths: ConfigDir, DataDir, CacheDir, LogDir
- Handles cross-platform differences (Android/Termux vs standard Linux/macOS/Windows)

### API Key Management

API keys stored in `.env` file (gitignored):
```
XAI_API_KEY=xai-xxxxxxxxxxxxx
GEMINI_API_KEY=AIzaxxxxxxxxxxxxxxx
```

Use `./grokster.sh setup` wizard to configure keys.

### Logging

Structured logging with `slog` (JSON format) to:
- `~/.config/grokster/grokster.json` (or equivalent platform path)
- Falls back to stderr if log file creation fails

### DIY Tool Creation

The **`create_tool`** meta-tool allows AI agents to dynamically generate new tools:
- Accepts tool name, description, command template, parameters, category
- Generates complete Go code in `pkg/tools/custom/`
- Supports parameter templating with `{{param}}` syntax
- Integrates with full permission system
- Example: Create a `ping_host` tool that runs `ping -c 4 {{host}}`

### OAuth Support

OAuth authentication (`pkg/auth/`) supports:
- Google Sign-In integration
- Token storage with encryption
- Automatic token refresh
- See `OAUTH_SETUP.md` for detailed configuration

## Code Conventions

- Package structure follows Go standard layout: `cmd/`, `internal/`, `pkg/`
- AI providers implement `AIProvider` interface (`pkg/ai/interface.go`)
- Tools implement `Tool` interface (`pkg/tools/tool.go`)
- Commands implement handler functions in `pkg/commands/registry.go`
- TUI messages use typed structs (e.g., `TokenMsg`, `ErrorMsg`) not raw strings

## Important Files

- `go.mod` - Dependency management (Go 1.24.2)
- `Makefile` - Build targets and cross-compilation
- `.env` - API keys (never commit!)
- `grokster.sh` - Wrapper script that loads .env
- `TOOL_SYSTEM_DESIGN.md` - Comprehensive tool architecture documentation
- `TOOLS_IMPLEMENTED.md` - Complete list of 28 tools with examples
- `GETTING_STARTED.md` - User guide for setup and usage

## Integration Points

When adding new features:

1. **New AI Provider**: Implement `AIProvider` interface in `pkg/ai/`
2. **New Tool**: Add to appropriate category file in `pkg/tools/` and register in `RegisterDefaultTools()`
3. **New TUI Command**: Register in `pkg/commands/registry.go` and implement handler
4. **Orchestrator Changes**: Modify `internal/engine/orchestrator.go` for routing logic
5. **A2A Messages**: Use `pkg/a2a/channel.go` for inter-agent communication

## Debugging

- Enable orchestrator watchdog: `./grokster.sh -watchdog`
- Enable verbose consultant thinking: `./grokster.sh -verbose-thoughts`
- Check logs: `cat ~/.config/grokster/grokster.json` (or platform-specific path)
- Tool analytics: `~/.config/grokster/tool_analytics.json`
- Permission storage: `~/.config/grokster/tool_permissions.json`

## Recent Bug Fixes

See detailed documentation: `BUG_FIXES.md`, `FIXES_V2.md`, `MOBILE_KEYBOARD_FIX.md`

### Feb 14, 2026 - Initial Fixes (6 bugs)
1. **`/settings` command** - Display config in TUI instead of external editor
2. **`/tools` command** - Query actual tool registry dynamically
3. **Viewport scrolling** - Fix disappearing messages, improve update frequency
4. **Input sanitization** - Comprehensive ANSI escape code stripping (complete + partial sequences)
5. **`list_tools` tool** - Return actual tool list to AI instead of stub message
6. **Escape codes v2** - Enhanced filtering for partial OSC sequences like `11;rgb:0000/0000/0000\`

### Feb 15, 2026 - Mobile & Layout Fixes
7. **Mobile keyboard focus** - Multiple strategies to restore keyboard on Android Termux:
   - Auto-focus on key events
   - Focus on touch/click
   - Auto-focus after AI response
   - Tab key shortcut
   - Visual focus indicator

8. **Proper TUI layout** - Professional full-screen layout like Claude Code/Gemini CLI:
   - Fixed viewport dimensions with clear height calculations
   - Full screen utilization without wasted space
   - Clean appearance with minimal borders
   - Smooth scrolling with proper boundaries
   - Perfect resize handling
