# Tool Permissions Guide

**Version:** 3.4.0

Gorkbot includes a comprehensive four-level permission system for controlling tool access. Permissions can be managed via slash commands, the permission prompt overlay, and the rule engine.

---

## Permission Levels

### `always`
- **Permanently approved** — no confirmation prompt shown
- Stored in `~/.config/gorkbot/tool_permissions.json`
- Use for: safe read-only tools you use frequently

### `session`
- **Approved for the current session only**
- Cleared when Gorkbot exits
- Not stored to disk
- Use for: tools you want to approve in bulk for a specific task

### `once` (Default)
- **Prompts every single call**
- Most secure option for tools that modify state
- Use for: `bash`, `write_file`, `git_push`, `delete_file`

### `never`
- **Permanently blocked** — will not prompt or execute
- Stored in `~/.config/gorkbot/tool_permissions.json`
- Use for: tools you want to disable entirely

---

## Permission Prompt Overlay

When a tool requires a permission check, a centered overlay appears:

```
┌──────────────────────────────────────────────┐
│  Permission Request                          │
│                                              │
│  Tool: bash                                  │
│  Description: Execute shell commands         │
│                                              │
│  Parameters:                                 │
│    • command: ls -la /home/user              │
│                                              │
│  Allow this tool to execute?                 │
│                                              │
│  ▶ [Always]   Grant permanent permission     │
│    [Session]  Allow for this session         │
│    [Once]     Ask next time (recommended)    │
│    [Never]    Block permanently              │
│                                              │
│  ↑/↓ to select · Enter to confirm           │
│  Esc to deny this execution                  │
└──────────────────────────────────────────────┘
```

**Navigation:**
- `↑` / `↓` or `k` / `j` — move selection
- `Enter` — confirm selected level
- `Esc` — deny this specific execution (does not change stored permission)

---

## `/permissions` Commands

### View All Permissions

```
/permissions
/permissions list
```

**Output:**
```
Tool Permissions

Total Tools: 162
  Always:  14
  Session:  2
  Once:   144
  Never:    2

Always Allowed
  read_file, list_directory, file_info, git_status,
  git_diff, git_log, system_info, disk_usage, …

Never Allowed
  metasploit_rpc, linpeas_run
```

### Reset All Permissions

```
/permissions reset
```

Clears ALL stored permissions (both `always` and `never`). All tools return to the `once` default. Deletes `tool_permissions.json`.

### Reset One Tool

```
/permissions reset bash
/permissions reset git_push
/permissions reset web_fetch
```

Removes only that tool's stored permission. The tool will prompt on next use.

---

## Rule Engine

The rule engine provides glob-pattern-based permission rules that are evaluated **before** the standard permission check. First matching rule wins.

### View Rules

```
/rules list
```

### Add Rules

```
/rules add allow "read_*"         # permanently allow all read tools
/rules add allow "git_status"     # permanently allow git_status
/rules add ask "git_push"         # always prompt for git_push
/rules add deny "delete_*"        # block all delete tools
/rules add deny "metasploit_*"    # block all metasploit tools
```

### Remove Rules

```
/rules remove allow "read_*"
/rules remove deny "delete_*"
```

### Rule Priority

1. Rule engine (glob patterns) — evaluated first
2. Standard permission store (`tool_permissions.json`) — evaluated if no rule matches
3. Default permission level (per-tool) — used if neither applies

---

## Storage

### Persistent Permissions

**File:** `~/.config/gorkbot/tool_permissions.json`
**Permissions:** 0600 (owner read/write only)

```json
{
  "permissions": {
    "read_file": "always",
    "list_directory": "always",
    "file_info": "always",
    "git_status": "always",
    "git_diff": "always",
    "git_log": "always",
    "system_info": "always",
    "bash": "once",
    "write_file": "once",
    "delete_file": "once",
    "git_push": "once",
    "metasploit_rpc": "never"
  },
  "version": "1.0"
}
```

Only `always` and `never` are persisted. `session` lives in memory only.

### Session Permissions

Held in `Registry.sessionPerms` (in-memory `map[string]bool`). Cleared when Gorkbot exits or when `/permissions reset` is run.

---

## Category-Level Enable/Disable

Disable entire tool categories via `/settings → Tool Groups`. Disabled categories block all tools in that category regardless of individual permissions.

Categories disabled in `app_state.json` (`disabled_categories` field) are restored on next startup.

**Categories:**
- `shell` — bash
- `file` — read/write/delete/search
- `git` — all git operations
- `web` — HTTP requests, fetch, download
- `system` — processes, env vars, system info
- `android` — Android/Termux device tools
- `security` — recon tools (disabled by default)
- `pentest` — exploitation tools (disabled by default)
- `devops` — Docker, Kubernetes, CI/CD
- `media` — ffmpeg, audio, video
- `ai` — consultation, image generation, ML
- `data_science` — CSV, plotting, arxiv
- `vision` — screen capture and vision analysis

---

## Recommended Permission Settings

### Grant `always` (safe read-only)

```
read_file             list_directory        file_info
grep_content          search_files          git_status
git_diff              git_log               system_info
disk_usage            check_port            list_processes
list_tools            tool_info             git_blame_analyze
arxiv_search          whois_lookup          cve_lookup
hash_identify         jwt_decode            netstat_analysis
```

### Keep as `once` (prompt each time — modify state)

```
bash                  write_file            edit_file
delete_file           git_commit            git_push
git_pull              http_request          download_file
kill_process          docker_manager        k8s_kubectl
create_tool           schedule_task         intent_broadcast
notification_send
```

### Consider `never` (high-risk or unused)

```
metasploit_rpc        linpeas_run           hydra_run
hashcat_run           sqlmap_scan           packet_capture
```

---

## Troubleshooting

### Permission prompt not appearing

The tool has a stored `always` or `never` permission:
```
/permissions         # check current state
/permissions reset <tool>
```

### Accidentally granted `always` to a dangerous tool

```
/permissions reset bash
/permissions reset write_file
```

### Tool silently blocked

Check for `never` permission or a deny rule:
```
/permissions
/rules list
```

### Want to start completely fresh

```
/permissions reset       # clears tool_permissions.json
/rules list              # check and manually remove any deny rules
```

### Related Commands

```
/tools                   # list all tools and their categories
/settings                # open settings overlay (tool group enable/disable)
/rules list              # show glob-pattern rules
/permissions             # show all tool permissions
```
