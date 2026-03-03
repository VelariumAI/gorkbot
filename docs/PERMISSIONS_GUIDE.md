# Gorkbot Tool Permissions Guide

**Version:** 3.5.1

This document covers the complete tool permission system — permission levels, the interactive prompt UI, fine-grained glob rules, category-level control, persistent storage, and all management commands. Understanding this system is essential for safe and productive use of Gorkbot's extensive tool suite.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Permission Levels](#2-permission-levels)
3. [The Permission Prompt](#3-the-permission-prompt)
4. [Fine-Grained Rules](#4-fine-grained-rules)
5. [Category-Level Enable / Disable](#5-category-level-enable--disable)
6. [Persistent Storage](#6-persistent-storage)
7. [Management Commands](#7-management-commands)
8. [Recommended Configurations](#8-recommended-configurations)
9. [Troubleshooting](#9-troubleshooting)

---

## 1. Overview

Every tool execution in Gorkbot is gated by a three-layer permission pipeline implemented in `pkg/tools.PermissionManager`. No tool — whether built-in, MCP-sourced, or dynamically created — bypasses this pipeline.

### The Three-Layer Pipeline

```
AI requests tool call
        │
        ▼
┌──────────────────────────────────┐
│  Layer 1: Category Guard         │
│  Is the tool's category enabled? │
│  No  → BLOCK (silent reject)     │
│  Yes → continue                  │
└──────────────────┬───────────────┘
                   │
                   ▼
┌──────────────────────────────────┐
│  Layer 2: Rule Engine            │
│  Does a glob rule match?         │
│  deny match  → BLOCK             │
│  allow match → ALLOW (bypass L3) │
│  no match    → continue          │
└──────────────────┬───────────────┘
                   │
                   ▼
┌──────────────────────────────────┐
│  Layer 3: Permission Store       │
│  always  → execute silently      │
│  never   → BLOCK                 │
│  session → execute (cached)      │
│  once    → show prompt to user   │
└──────────────────────────────────┘
```

The first applicable decision in this hierarchy wins. This design allows broad category blocks, fine-grained pattern exceptions, and individual tool overrides to coexist cleanly.

---

## 2. Permission Levels

| Level | Symbol | Behavior | Persistence |
|-------|--------|---------|-------------|
| `always` | ✅ | Pre-approved — executes silently every time | Saved to `tool_permissions.json` |
| `session` | 🔓 | Approved for the current session only | In-memory; cleared on exit or `/clear` |
| `once` | ❓ | Prompts the user on every call (default for all tools) | Not saved |
| `never` | ❌ | Blocked permanently — tool cannot execute | Saved to `tool_permissions.json` |

### Choosing the Right Level

**Grant `always` to** safe, read-only tools that you trust completely and call frequently:
- `read_file`, `list_directory`, `file_info`
- `git_status`, `git_diff`, `git_log`
- `system_info`, `disk_usage`, `list_processes`
- `grep_content`, `search_files`

**Use `session` for** a bounded block of work where you want repeated access without a permanent grant. Session approvals are automatically revoked when Gorkbot exits, providing a natural clean-up boundary.

**Keep `once` for** any tool whose parameters you want to review before each call — especially tools that write, delete, network, or execute code. The prompt shows you the exact parameters the AI has chosen.

**Set `never` for** tools you want completely off-limits regardless of what the AI requests:
- `git_push` — force manual review before any remote push
- `kill_process` — prevent accidental process termination
- Any tool in a category that should remain globally disabled but with a per-tool exception

---

## 3. The Permission Prompt

When a tool with `once` permission is called — or when a tool is encountered for the first time — a centered overlay appears in the TUI, pausing all other activity until you respond.

### Prompt Layout

```
┌─────────────────────────────────────────────────────────────┐
│                    Permission Request                       │
│                                                             │
│  Tool:        write_file                                    │
│  Category:    file                                          │
│  Description: Write content to a file (creates or appends) │
│                                                             │
│  Parameters:                                                │
│    path:      /home/user/project/main.go                   │
│    content:   (2,847 chars — package main\n\nimport ...)   │
│    append:    false                                         │
│                                                             │
│  Allow this tool to execute?                                │
│                                                             │
│  ▶ [Always]   Grant permanent permission                    │
│    [Session]  Allow for this session only                   │
│    [Once]     Ask again next time (recommended)             │
│    [Never]    Block permanently                             │
│                                                             │
│  ↑ / ↓  select     Enter  confirm     Esc  deny             │
└─────────────────────────────────────────────────────────────┘
```

### Navigation

| Key | Action |
|-----|--------|
| `↑` or `k` | Move selection up |
| `↓` or `j` | Move selection down |
| `Enter` | Confirm the selected option |
| `Esc` | Deny this specific execution without changing the stored permission |

### What the Prompt Shows You

The prompt always displays:
- **Tool name** and **category** — what is being called and from which group
- **Description** — a human-readable summary of what the tool does
- **Resolved parameters** — the actual values the AI has chosen, truncated for readability if very long

This design ensures you can make an informed decision about every tool execution before it runs.

### Esc-Deny Behavior

Pressing `Esc` is a "deny this call, keep permission unchanged" action. The tool does not execute, the permission level remains at `once`, and the AI receives a rejection message. Use `Esc` when you want to block a specific call but continue being asked about future calls.

---

## 4. Fine-Grained Rules

The rule engine (`pkg/tools.RuleEngine`) sits between the category guard and the permission store. It allows pattern-based allow/deny decisions based on the tool's primary parameter value.

### Creating Rules

```
/rules add <tool-name> <allow|deny> "<glob-pattern>"
```

**Examples:**

```bash
# Shell — allow only safe read commands
/rules add bash allow "ls *"
/rules add bash allow "cat *"
/rules add bash allow "git status"
/rules add bash allow "git log*"
/rules add bash deny "*"              # deny everything else

# File — block writes to sensitive files
/rules add write_file deny "*.env"
/rules add write_file deny "*.key"
/rules add write_file deny "/etc/*"
/rules add write_file deny "/system/*"

# Network — restrict to a known API
/rules add http_request allow "https://api.my-service.com/*"
/rules add http_request deny "*"

# Git — block pushes to main
/rules add git_push deny "origin main"
/rules add git_push deny "origin master"
```

### Listing and Removing Rules

```
/rules list                    # show all active rules with their IDs
/rules remove <rule-id>        # remove a specific rule by ID
```

Example output of `/rules list`:

```
Active Tool Rules

ID    Tool          Action  Pattern
────  ────────────  ──────  ─────────────────────────────
r001  bash          allow   ls *
r002  bash          allow   git status
r003  bash          deny    *
r004  write_file    deny    *.env
r005  http_request  deny    *
```

### Rule Evaluation Order

Rules are evaluated **top-to-bottom**. The first matching rule wins immediately:

```
Rules for 'bash':
  1. allow  "git status"
  2. allow  "ls *"
  3. deny   "*"

Incoming: bash command="git push origin main"
  Rule 1: "git status" — no match
  Rule 2: "ls *"       — no match
  Rule 3: "*"          — MATCH → DENY (rejected without prompt)
```

If no rule matches, evaluation falls through to the permission store (Layer 3).

### Glob Pattern Syntax

Patterns use standard shell glob matching:
- `*` — matches any sequence of characters
- `?` — matches any single character
- `[abc]` — matches any character in the set

---

## 5. Category-Level Enable / Disable

Tool categories provide coarse-grained control over entire groups of tools. Disabling a category blocks every tool in that category regardless of individual permission levels or rules.

### Managing Categories

**Via Settings Overlay (recommended):**
```
Ctrl+G → Tool Groups tab
```
Toggle any category on or off. Changes take effect immediately and persist to `app_state.json`.

**Via `app_state.json` (manual):**
```json
{
  "disabled_categories": ["security", "pentest", "devops"]
}
```

### Default Category States

| Category | Default State | Description |
|----------|--------------|-------------|
| `shell` | Enabled | `bash`, process management |
| `file` | Enabled | Read, write, edit, delete, search |
| `hashline` | Enabled | Hash-validated file editing |
| `git` | Enabled | Status, diff, log, commit, push, pull |
| `web` | Enabled | Fetch, HTTP requests, download |
| `system` | Enabled | System info, disk, processes, network |
| `ai` | Enabled | Consultation, subagent spawning, skills |
| `meta` | Enabled | Self-introspection, tool stats, rebuild |
| `background_agents` | Enabled | Async agent tasks |
| `process` | Enabled | Managed background processes |
| `database` | Enabled | SQLite read/write |
| `devops` | Enabled | Docker, Kubernetes, cloud CLIs |
| `android` | Enabled | Termux/Android device, ADB, sensors |
| `vision` | Enabled | Screen capture, OCR, image analysis |
| `media` | Enabled | Audio, video, image processing |
| `data_science` | Enabled | Jupyter, Python data tools |
| `personal` | Enabled | Calendar, email, notes, productivity |
| `scheduler` | Enabled | Cron task management |
| `memory` | Enabled | SENSE AgeMem, GoalLedger, RAG memory |
| `worktree` | Enabled | Git worktree management |
| `pipeline` | Enabled | Agentic pipeline execution |
| `cci` | Enabled | Codified Context Infrastructure tools |
| `security` | **Disabled** | Recon, audit, vulnerability tools — enable intentionally |
| `pentest` | **Disabled** | Exploitation and credential tools — assessments only |

### Re-enabling Security Categories

Security and pentest categories require explicit opt-in:

```
Ctrl+G → Tool Groups → toggle "security" → ON
```

This activates tools like `nmap_scan`, `shodan_query`, `sqlmap_scan`, `nuclei_scan`, and 25+ other security assessment tools. Disable again when the assessment is complete.

---

## 6. Persistent Storage

### `tool_permissions.json`

**Path:** `~/.config/gorkbot/tool_permissions.json`
**File permissions:** `0600` (owner read/write only)

Only `always` and `never` decisions are persisted. `session` permissions live in memory only. `once` is the implicit default and requires no storage entry.

**Example file:**
```json
{
  "permissions": {
    "read_file":       "always",
    "list_directory":  "always",
    "file_info":       "always",
    "grep_content":    "always",
    "search_files":    "always",
    "git_status":      "always",
    "git_diff":        "always",
    "git_log":         "always",
    "system_info":     "always",
    "disk_usage":      "always",
    "bash":            "once",
    "write_file":      "once",
    "edit_file":       "once",
    "delete_file":     "once",
    "git_commit":      "once",
    "git_push":        "never",
    "kill_process":    "never"
  },
  "version": "1.0"
}
```

**Do not edit this file manually.** Use `/permissions` commands to ensure the in-memory state and the file stay consistent.

### `app_state.json`

**Path:** `~/.config/gorkbot/app_state.json`

Contains the `disabled_categories` list alongside model selection and other application state. Managed by the Settings overlay and `pkg/config.AppStateManager`.

---

## 7. Management Commands

### `/permissions`

Display a full summary of all tool permission levels, including counts per level and the list of tools in each:

```
> /permissions

Tool Permissions  (162 tools total)

✅ Always (12):
  read_file, list_directory, file_info, grep_content, search_files,
  git_status, git_diff, git_log, system_info, disk_usage,
  list_processes, file_hashes

❌ Never (3):
  git_push, kill_process, delete_file

🔓 Session (2):
  bash, write_file

❓ Once (145):
  (all remaining tools)
```

### `/permissions reset`

Reset **all** permissions to `once`. This clears `tool_permissions.json` on disk and all in-memory session permissions. Every tool will prompt on its next call:

```
> /permissions reset
✅ All permissions reset. All 162 tools will prompt on next use.
```

### `/permissions reset <tool>`

Reset a **single tool's** permission to `once`. Removes the entry from `tool_permissions.json` and clears any session approval for that tool:

```
> /permissions reset bash
✅ Permission reset for 'bash'. You will be prompted on next use.

> /permissions reset git_push
✅ Permission reset for 'git_push'. You will be prompted on next use.
```

### `/rules`

```
/rules list                          # show all active rules
/rules add <tool> <allow|deny> <pat> # add a new rule
/rules remove <rule-id>              # remove a rule by ID
```

### `/settings`

Opens the Settings overlay (`Ctrl+G`). Navigate to the **Tool Groups** tab to enable or disable entire categories.

### `/tools`

Lists all registered tools with their current permission level, category, and call count for the session. Append `stats` for full analytics:

```
/tools           # list all tools with status
/tools stats     # usage analytics dashboard
```

---

## 8. Recommended Configurations

### Baseline (Recommended for Most Users)

Start with all permissions at `once`. When each of these safe read-only tools prompts for the first time, select **Always**:

| Tool | Why Always is Safe |
|------|--------------------|
| `read_file` | Read-only; cannot modify state |
| `list_directory` | Directory listing only |
| `file_info` | File metadata only |
| `grep_content` | Text search only |
| `search_files` | File name search only |
| `git_status` | Read-only git state |
| `git_diff` | Read-only diff view |
| `git_log` | Read-only history |
| `system_info` | Read-only system data |
| `disk_usage` | Read-only disk data |

Keep the following at `once` permanently:

| Tool | Why Always Prompt |
|------|--------------------|
| `bash` | Executes arbitrary shell commands |
| `write_file` | Creates or overwrites files |
| `edit_file` | Modifies file content |
| `delete_file` | Permanent deletion |
| `git_commit` | Creates permanent history |
| `git_push` | Sends to remote — irreversible |
| `git_pull` | Modifies local branch state |
| `http_request` | Outbound network calls |
| `download_file` | Saves remote content locally |
| `kill_process` | Terminates running processes |

### High-Security / Air-Gapped

Block all network and shell execution by default:

```
/rules add bash deny "*"
/rules add http_request deny "*"
/rules add web_fetch deny "*"
/rules add download_file deny "*"
/rules add http_post deny "*"
```

Then add exceptions only for specific allowed commands:

```
/rules add bash allow "git status"
/rules add bash allow "go build *"
/rules add bash allow "ls *"
```

### Permissive Development Sprint

For intensive development sessions where you want minimal friction:

1. At the first prompt for `bash`, select **Session**.
2. At the first prompt for `write_file`, select **Session**.
3. At the first prompt for `edit_file`, select **Session**.
4. When your sprint is complete, exit Gorkbot — all session grants are automatically revoked.

Never use **Always** for `bash` or `write_file` unless you are in a fully controlled, sandboxed environment.

---

## 9. Troubleshooting

### Permission prompt not appearing for a tool

The tool already has a non-`once` permission stored:

```
/permissions
# Look for the tool name in "Always" or "Never" list

/permissions reset <tool-name>
# This restores it to "once" (will prompt again)
```

### A tool I need is being blocked silently

Check if:
1. Its category is disabled — open `Ctrl+G` → Tool Groups
2. A deny rule is matching — run `/rules list`
3. It has `never` permission — run `/permissions` and look in the Never list

Fix each as appropriate.

### I granted `always` to a dangerous tool by accident

Reset it immediately:

```
/permissions reset bash
```

The next call to that tool will prompt again.

### I want to see every tool the AI tried to call (including blocked ones)

Enable debug mode:

```
/debug
```

Debug mode shows the raw AI output including all tool request JSON and the permission decision made for each. Toggle off with `/debug` again.

### `tool_permissions.json` is missing or corrupted

If the file is missing, Gorkbot treats all tools as `once` (the default). If it is corrupted:

```bash
rm ~/.config/gorkbot/tool_permissions.json
```

Gorkbot will start fresh with all tools at `once` on the next launch.

### Categories are not persisting across restarts

Verify `app_state.json` is writable:

```bash
ls -la ~/.config/gorkbot/app_state.json
# Should show -rw------- (0600)
```

If the file is read-only, fix permissions:

```bash
chmod 600 ~/.config/gorkbot/app_state.json
```
