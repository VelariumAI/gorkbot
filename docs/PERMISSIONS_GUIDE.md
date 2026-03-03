# Tool Permissions Management Guide

## Overview

Grokster includes a comprehensive permission system for managing tool access. The `/permissions` command allows you to view and reset tool permissions at any time.

---

## Permission Levels

### ✅ Always
- Tool is permanently approved
- No prompts on future executions
- Stored in `~/.config/grokster/tool_permissions.json`
- **Use for:** Trusted, frequently-used tools

### 🔓 Session
- Approved for current session only
- Cleared when you exit Grokster
- No persistent storage
- **Use for:** Temporary access during this session

### ❓ Once (Default)
- Ask every time (recommended for new/unknown tools)
- Most secure option
- **Use for:** Untrusted or rarely-used tools

### ❌ Never
- Tool is permanently blocked
- Will not prompt or execute
- Stored persistently
- **Use for:** Tools you want to disable

---

## Commands

### View All Permissions
```
/permissions
```
or
```
/permissions list
```

**Shows:**
- Summary of permissions by level
- List of tools in each category
- Quick reset instructions

**Example Output:**
```
🔐 Tool Permissions

Total Tools: 44
- ✅ Always: 8
- 🔓 Session: 2
- ❓ Once: 32
- ❌ Never: 2

## ✅ Always Allowed
- read_file
- list_directory
- git_status
...
```

---

### Reset All Permissions
```
/permissions reset
```

**Effect:**
- Clears ALL persistent permissions (always/never)
- Clears session permissions
- All tools return to default "once" (ask every time)
- Deletes `~/.config/grokster/tool_permissions.json`

**Use when:**
- You want a fresh start
- You've granted too many "always" permissions
- Testing permission flows

---

### Reset Specific Tool
```
/permissions reset <tool_name>
```

**Examples:**
```
/permissions reset bash
/permissions reset git_push
/permissions reset web_fetch
```

**Effect:**
- Removes permission for that specific tool
- Clears both persistent and session permissions
- Tool will ask for permission on next use

**Use when:**
- You want to re-evaluate a specific tool's access
- You accidentally granted "always" to a dangerous tool
- Testing a single tool's permission flow

---

## Permission Prompt UI

When a tool requires permission, you'll see a **centered overlay** with:

```
┌──────────────────────────────────┐
│ 🔐 Permission Request            │
│                                  │
│ Tool: bash                       │
│ Description: Execute shell cmds  │
│                                  │
│ Parameters:                      │
│   • command: ls -la              │
│                                  │
│ Allow this tool to execute?      │
│                                  │
│ ▶ [Always] Grant permanent perm  │
│   [Session] Allow for session    │
│   [Once] Ask every time (rec.)   │
│   [Never] Block permanently      │
│                                  │
│ Use ↑/↓ to select, Enter to     │
│ confirm, Esc to deny             │
└──────────────────────────────────┘
```

**Navigation:**
- `↑/↓` or `k/j` - Select option
- `Enter` - Confirm selection
- `Esc` - Deny permission (blocks this execution)

---

## Storage

### Persistent Permissions
- **File:** `~/.config/grokster/tool_permissions.json`
- **Permissions:** 0600 (owner read/write only)
- **Format:** JSON
- **Contains:** always/never permissions

**Example:**
```json
{
  "permissions": {
    "read_file": "always",
    "write_file": "once",
    "git_push": "never"
  },
  "version": "1.0"
}
```

### Session Permissions
- **Storage:** In-memory only
- **Lifetime:** Current Grokster session
- **Reset:** On exit or with `/permissions reset`

---

## Best Practices

### 🛡️ Security
1. **Start restrictive:** Default to "once" for new tools
2. **Review regularly:** Use `/permissions` to audit granted permissions
3. **Reset periodically:** Clear unused permissions with `/permissions reset`
4. **Block dangerous tools:** Use "never" for tools you don't want available

### 🚀 Productivity
1. **Grant "always" to safe tools:** `read_file`, `list_directory`, `git_status`
2. **Use "session" for batch operations:** Temporary access for current task
3. **Keep "once" for destructive tools:** `delete_file`, `git_push`, `kill_process`

### 📋 Recommended Settings

**Safe to grant "always":**
- `read_file` - Read-only file access
- `list_directory` - List directory contents
- `file_info` - File metadata
- `git_status` - View git status
- `git_diff` - View changes
- `git_log` - View commit history
- `list_processes` - View running processes
- `system_info` - System information
- `disk_usage` - Disk space information

**Keep as "once" (ask each time):**
- `bash` - Execute arbitrary shell commands
- `write_file` - Create/modify files
- `edit_file` - Edit files
- `delete_file` - Delete files/directories
- `git_commit` - Create commits
- `git_push` - Push to remote
- `git_pull` - Pull from remote
- `kill_process` - Terminate processes
- `http_request` - Make HTTP requests
- `download_file` - Download files

**Consider "session" for:**
- Temporary bulk operations
- Development sessions with frequent tool use
- Testing and debugging

---

## Troubleshooting

### Permission prompt not showing
- Check if tool has "always" or "never" permission
- Use `/permissions` to view current permissions
- Reset with `/permissions reset <tool>`

### Permission prompt cut off
- ✅ Fixed in latest version - prompt now appears as centered overlay
- If issue persists, resize terminal or use `/bug` to report

### Accidentally granted "always"
```
/permissions reset <tool_name>
```

### Want to start fresh
```
/permissions reset
```

### Can't find permissions file
```
/settings
```
Shows the path to `tool_permissions.json`

---

## Related Commands

- `/tools` - List all available tools
- `/settings` - View configuration and file paths
- `/help` - Show all commands

---

## Examples

### Example 1: Audit Permissions
```
> /permissions

🔐 Tool Permissions

Total Tools: 44
- ✅ Always: 12
- 🔓 Session: 0
- ❓ Once: 30
- ❌ Never: 2
...
```

### Example 2: Reset Dangerous Tool
```
> /permissions reset bash

✅ Permission reset for `bash`

You will be asked for permission next time this tool is used.
```

### Example 3: Clean Slate
```
> /permissions reset

✅ All permissions reset

All tools will require permission approval on next use.
```

---

## Summary

The `/permissions` command gives you full control over tool access:
- 📋 **List** permissions with `/permissions`
- 🔄 **Reset all** with `/permissions reset`
- 🎯 **Reset specific** with `/permissions reset <tool>`

Combined with the improved **centered permission prompt UI**, you now have complete visibility and control over tool permissions! 🎉
