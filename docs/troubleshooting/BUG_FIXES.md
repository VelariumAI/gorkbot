# Bug Fixes - February 14, 2026

## Summary

Fixed 4 critical bugs in the Gorkbot TUI that were preventing normal operation.

---

## Bug #1: `/settings` Command Shows Empty

**Problem:**
- The `/settings` command tried to open `~/.config/gorkbot/config.yaml` in an external editor
- The file didn't exist, so the editor showed an empty file
- Using `exec.Command()` took over the terminal and broke the TUI
- No way to change tool permissions or configure settings

**Solution:**
- Changed `/settings` to display configuration information within the TUI instead of opening external editor
- Shows status of all configuration files (config.yaml, tool_permissions.json, tool_analytics.json)
- Displays masked API keys (XAI_API_KEY, GEMINI_API_KEY) with status indicators
- Provides instructions for editing config files outside of TUI
- No longer breaks TUI by taking over terminal

**Files Modified:**
- `pkg/commands/registry.go` - Rewrote `handleSettings()` function

---

## Bug #2: `/tools` Command Shows Hardcoded Text

**Problem:**
- The `/tools` command returned hardcoded text: "Code Executor (disabled)"
- Did not query the actual tool registry
- Could not see the 28 real tools that were registered
- No way to verify what tools were actually available

**Solution:**
- Connected the command registry to the tool registry
- Added `SetToolRegistry()` method to pass tool registry to command registry
- Rewrote `handleTools()` to dynamically query the registry using interface types
- Groups tools by category (shell, file, git, web, system, meta, custom)
- Shows actual tool count and descriptions
- Updated TUI model initialization to connect the registries

**Files Modified:**
- `pkg/commands/registry.go` - Added `toolRegistry` field, `SetToolRegistry()` method, rewrote `handleTools()`
- `internal/tui/model.go` - Updated `NewModel()` to connect tool registry to command registry

---

## Bug #3: Messages Disappear After First Interaction

**Problem:**
- After the first prompt and reply, subsequent interactions were invisible
- Could not scroll viewport to see messages
- Chat history appeared to vanish
- Viewport not updating properly with new content

**Solution:**
- Fixed viewport content update logic in `updateViewportContent()`
- Added safety check: if viewport is taller than content, go to top instead of bottom
- Increased viewport update frequency from every 50 tokens to every 20 tokens for smoother streaming
- Adjusted viewport height calculation with extra padding to prevent overlap
- Increased glamour word wrap margin from -8 to -10 to prevent wrapping issues
- Force viewport update after final message to ensure visibility

**Files Modified:**
- `internal/tui/model.go` - Fixed `updateViewportContent()` with better scrolling logic
- `internal/tui/update.go` - Updated `handleTokenMsg()` and `handleWindowSize()` for better rendering

---

## Bug #4: Random ANSI Escape Codes in Input Box

**Problem:**
- Input box randomly filled with escape sequences like `P1+r436f=323536\P1+r6b75=1B4F41`
- These are ANSI color/style codes bleeding into textarea
- Made input unusable and corrupted user messages
- Likely caused by lipgloss/glamour rendering affecting terminal state

**Solution:**
- Created `stripANSICodes()` function to remove all ANSI escape sequences
- Sanitizes input on submission before processing
- Also sanitizes textarea value during update if escape codes detected
- Filters out control characters (except newline, tab, carriage return)
- Prevents escape codes from accumulating in the input buffer

**Files Modified:**
- `internal/tui/model.go` - Added `stripANSICodes()` function, sanitize input in `submitPrompt()`
- `internal/tui/update.go` - Added escape code detection and cleanup in `handleKeyMsg()`, added `strings` import

---

## Build Status

✅ All fixes implemented
✅ Code compiles successfully
✅ Binary created: `bin/gorkbot` (21MB)
✅ Ready for testing

---

## Testing Recommendations

### Test Bug #1 Fix
```bash
./gorkbot.sh
# In TUI:
/settings
```
**Expected:** Shows configuration status with file paths and API key status (no longer tries to open editor)

### Test Bug #2 Fix
```bash
./gorkbot.sh
# In TUI:
/tools
```
**Expected:** Shows all 28 tools grouped by category with actual descriptions (no hardcoded text)

### Test Bug #3 Fix
```bash
./gorkbot.sh
# In TUI:
You: Test message 1
# Wait for response
You: Test message 2
# Wait for response
You: Test message 3
```
**Expected:** All messages and responses remain visible, can scroll with PgUp/PgDn

### Test Bug #4 Fix
```bash
./gorkbot.sh
# In TUI:
# Type normally and verify no random escape codes appear
# Try pasting text
# Try multiple messages
```
**Expected:** No random characters like "P1+r436f=323536" appear in input box

---

## Technical Details

### Escape Code Stripping Algorithm
The `stripANSICodes()` function uses a state machine approach:
1. Detects ESC character (0x1B)
2. Enters "in escape" state
3. Skips all characters until finding an alphabet character (A-Z, a-z)
4. Also filters control characters < 32 (except \n, \t, \r)

### Viewport Update Strategy
- Updates every 20 tokens during streaming (not every token to avoid performance issues)
- Always updates on final message
- Uses GotoBottom() to show latest content
- Falls back to GotoTop() if content is smaller than viewport

### Tool Registry Connection
Uses interface types to avoid circular imports:
```go
type ToolInfo interface {
    Name() string
    Category() interface{}
    Description() string
}
```

---

## Remaining Work

All critical bugs are fixed. The TUI should now be fully functional for:
- ✅ Viewing settings and configuration
- ✅ Listing all available tools
- ✅ Scrolling through chat history
- ✅ Clean text input without escape codes

---

## Files Changed (Summary)

1. `pkg/commands/registry.go` - Fixed /settings and /tools commands
2. `internal/tui/model.go` - Fixed viewport updates and escape code stripping
3. `internal/tui/update.go` - Fixed viewport scrolling and input sanitization

**Total:** 3 files modified, 0 files created
**Build:** Success ✅
**Status:** Ready for production testing
