# 🧪 Testing the Complete Tool System

Quick guide to test all 4 enhancements!

## ✅ Build Status
```bash
go build ./cmd/grokster
# ✅ SUCCESS - All code compiles!
```

---

## 🎯 Test 1: Tool Context in AI Prompts

**What to test:** AI knows about tools

**Commands:**
```bash
./grokster.sh
```

**In TUI, try:**
```
You: "What tools do you have available?"
```

**Expected Result:**
- AI lists tools (bash, git_status, read_file, etc.)
- Mentions categories (shell, file, git, web, system, meta)
- Describes what tools can do

**Why this works:**
- Orchestrator adds 11KB tool context to every prompt
- AI sees complete tool definitions
- No special commands needed!

---

## 🎯 Test 2: Multi-Turn Tool Chaining

**What to test:** AI can use tools and chain them

**Test Case A - Single Tool:**
```
You: "What files are in my current directory?"
```

**Expected:**
1. AI uses `list_directory` tool
2. Green ToolBox appears with:
   ```
   ✅ Tool: list_directory
   [directory contents shown]
   ```
3. AI summarizes the results

**Test Case B - Tool Chain:**
```
You: "Show me the git status and if there are changes, show the diff"
```

**Expected:**
1. AI uses `git_status` tool
2. ToolBox shows status
3. AI sees "changes detected" in result
4. AI uses `git_diff` tool
5. ToolBox shows diff
6. AI provides analysis

**Test Case C - Complex Chain:**
```
You: "Find all Go files and count the lines in each"
```

**Expected:**
1. AI uses `search_files` with pattern "*.go"
2. For each file found, AI uses `bash` with `wc -l`
3. AI summarizes total lines

---

## 🎯 Test 3: Permission Prompt UI

**What to test:** Interactive permission prompts

**Setup:**
```bash
# Clear any existing permissions (optional)
rm ~/.config/grokster/tool_permissions.json
```

**Test Case A - First Time Bash:**
```
You: "Run the command 'echo hello world'"
```

**Expected:**
1. Permission prompt appears with yellow border:
   ```
   🔐 Permission Request
   Tool: bash
   Description: Execute bash commands...

   Parameters:
     • command: echo hello world

   ▶ [Once] Ask every time (recommended)
   ```
2. Use ↑/↓ to navigate options
3. Press Enter to approve
4. Tool executes

**Test Case B - Permission Persistence:**
```
# After approving once:
You: "Run 'pwd'"
```

**Expected:**
- If you selected "Always": No prompt, executes immediately
- If you selected "Session": No prompt this session
- If you selected "Once": Prompts again

**Test Case C - Permission Denial:**
```
You: "Delete all files"
```

**Expected:**
1. AI requests `delete_file` tool
2. Permission prompt appears
3. Select "Never" or press Esc
4. Tool execution denied
5. AI reports it cannot proceed

---

## 🎯 Test 4: Tool Usage Analytics

**What to test:** Usage tracking and statistics

**Generate Some Activity:**
```bash
# In TUI, use various tools:
You: "Show git status"        # Uses git_status
You: "List files"             # Uses list_directory
You: "What's in README.md"    # Uses read_file
You: "Check git log"          # Uses git_log
```

**Check Analytics:**
```bash
cat ~/.config/grokster/tool_analytics.json | jq
```

**Expected Output:**
```json
{
  "version": "1.0",
  "stats": {
    "git_status": {
      "tool_name": "git_status",
      "execution_count": 1,
      "success_count": 1,
      "failure_count": 0,
      "total_duration": 45123456,
      "last_used": "2024-02-14T23:45:12Z"
    },
    "list_directory": {
      "tool_name": "list_directory",
      "execution_count": 1,
      "success_count": 1,
      "failure_count": 0,
      "total_duration": 67234567,
      "last_used": "2024-02-14T23:45:15Z"
    }
  }
}
```

**Verify Analytics Data:**
- Each tool used shows execution_count
- success_count increments on success
- failure_count increments on error
- total_duration tracks cumulative time
- last_used updates with timestamp

---

## 🎯 Integration Test: Complete Workflow

**Scenario:** Full tool system demonstration

**Command:**
```
You: "I need you to check my git repository, see if there are changes, and if so, show me what changed in the main.go file"
```

**Expected Multi-Turn Flow:**

**Turn 1:**
```
AI: I'll check the git status first.
[Uses git_status tool]
```

**ToolBox appears:**
```
✅ Tool: git_status
On branch main
Changes not staged for commit:
  modified: cmd/grokster/main.go
```

**Turn 2:**
```
AI: I see main.go has changes. Let me show you the diff.
[Uses git_diff tool with file parameter]
```

**ToolBox appears:**
```
✅ Tool: git_diff
diff --git a/cmd/grokster/main.go b/cmd/grokster/main.go
...changes shown...
```

**Turn 3:**
```
AI: Here's what changed in main.go:
- Added analytics initialization
- Added registry.SetAnalytics() call
[Provides analysis]
```

**Verify:**
- ✅ Multiple tools used in sequence
- ✅ AI saw tool results and made decisions
- ✅ Each tool shown in green ToolBox
- ✅ Analytics recorded all executions

---

## 🔍 Detailed Verification

### Check Tool Context
```bash
go run test_tool_context.go
```

**Expected:**
```
🔧 Tool Context Preview

This is what the AI will see:
==================================================
<tools>
You have access to the following tools...

## bash
Category: shell
Description: Execute bash commands...
Parameters: {"type":"object"...}

[... 27 more tools ...]
==================================================

📊 Tool context size: 11406 characters
```

### Check Tool Registration
```bash
go run test_tools.go
```

**Expected:**
```
🔧 Grokster Tool System Test

📁 shell Tools (1):
   - bash [once] ...

📁 file Tools (7):
   - read_file [session] ...
   [... more tools ...]

✅ Total tools registered: 26-28

🧪 Testing bash tool execution...
✅ Tool executed successfully!
   Output: Hello from Grokster tools!
```

---

## 🐛 Troubleshooting

### Issue: AI doesn't use tools
**Check:**
```bash
# Verify tool context is added
grep -A 5 "GetToolContext" internal/engine/orchestrator.go
```
**Expected:** Method exists and is called in ExecuteTaskWithTools

### Issue: Permission prompt doesn't show
**Check:**
```bash
# Verify permission UI exists
ls -la internal/tui/permission.go
```
**Expected:** File exists with PermissionPrompt struct

### Issue: Analytics not recording
**Check:**
```bash
# Verify analytics file is created
ls -la ~/.config/grokster/tool_analytics.json
```
**Expected:** File exists after tool executions

### Issue: Tool execution fails
**Check logs:**
```bash
tail -f ~/.local/state/grokster/grokster.json | jq
```
**Look for:** "Executing tool" entries with error messages

---

## 📊 Success Criteria

All enhancements working if:

✅ **Tool Context**
- AI responds with tool awareness
- Mentions specific tool names
- Describes tool capabilities

✅ **Multi-Turn Chaining**
- Tools execute successfully
- Green ToolBoxes appear
- AI uses multiple tools in sequence
- AI references previous tool results

✅ **Permission Prompts**
- Yellow bordered prompt appears
- Keyboard navigation works (↑/↓)
- Enter confirms, Esc denies
- Selections persist correctly

✅ **Analytics**
- JSON file created in ~/.config/grokster/
- Execution counts increase
- Success/failure tracked
- Timestamps update

---

## 🎉 Full System Test

**Run this complete test sequence:**

```bash
# 1. Build
go build ./cmd/grokster
echo "✅ Build successful"

# 2. Check tools
go run test_tools.go | grep "Total tools"

# 3. Start TUI
./grokster.sh
```

**In TUI:**
```
# Test 1: Tool awareness
You: "What tools can you use?"

# Test 2: Simple tool
You: "List files in current directory"

# Test 3: Tool chain
You: "Check git status and show recent commits"

# Test 4: Permission (if prompt appears)
[Navigate with ↑/↓, press Enter]

# Exit
/quit
```

**After TUI:**
```bash
# 5. Check analytics
cat ~/.config/grokster/tool_analytics.json | jq '.stats | keys'

# 6. Verify permissions
cat ~/.config/grokster/tool_permissions.json | jq

echo "✅ All enhancements working!"
```

---

## 🚀 Ready to Launch!

If all tests pass:
- ✅ Tool system fully operational
- ✅ AI has full tool awareness
- ✅ Multi-turn workflows functional
- ✅ Permissions working securely
- ✅ Analytics tracking usage

**You're ready for production use!**

---

## 📝 Notes

- First tool use may take longer (AI learning)
- Permission prompts only appear for tools requiring approval
- Analytics accumulate over multiple sessions
- Tool context adds ~11KB to each prompt (minimal impact)
- Max 10 tool chaining turns prevents infinite loops

**Enjoy your powerful AI assistant with 28 tools! 🎊**
