# Gorkbot Tool Reference

**Version:** 4.7.0

Gorkbot ships with 196+ built-in tools spanning 20+ categories. This document provides a complete reference for every tool: its name, description, parameters, permission level, and usage notes.

Tools are organized into named **packs** and activated by `GORKBOT_TOOL_PACKS`. The default active packs are `core,dev,web,sys,agent,data,media,comm`. Set `GORKBOT_TOOL_PACKS=ALL` to enable everything including security and vision tools.

All tools are routed through the unified permission and SENSE sanitizer pipeline. MCP server tools are prefixed `mcp_<server>_<toolname>` and behave identically to built-in tools.

---

## Permission Levels

| Level | Meaning |
|-------|---------|
| `always` | Pre-approved — no prompt shown |
| `session` | Approved for this session only |
| `once` | Prompts before every call |
| `never` | Blocked permanently |

Default permissions shown below are the built-in defaults. Override any tool permission via `/permissions` or the permission prompt.

---

## Table of Contents

1. [Shell](#1-shell)
2. [File Operations](#2-file-operations)
3. [Hashline File Tools](#3-hashline-file-tools)
4. [Git](#4-git)
5. [Web and Network](#5-web-and-network)
6. [Web Scraping — Scrapling](#6-web-scraping--scrapling)
7. [System](#7-system)
8. [DevOps and Cloud](#8-devops-and-cloud)
9. [Android and Termux](#9-android-and-termux)
10. [Vision — Screen Capture and Analysis](#10-vision--screen-capture-and-analysis)
11. [Task Management](#11-task-management)
12. [AI and Skills](#12-ai-and-skills)
13. [Meta and Tool Creation](#13-meta-and-tool-creation)
14. [Background Agents and Pipeline](#14-background-agents-and-pipeline)
15. [Process Management](#15-process-management)
16. [Worktree Management](#16-worktree-management)
17. [SENSE Memory](#17-sense-memory)
18. [Self-Introspection](#18-self-introspection)
19. [Goal Ledger](#19-goal-ledger)
20. [Security Assessment](#20-security-assessment)
21. [Security Exploitation Tools](#21-security-exploitation-tools)
22. [Database](#22-database)
23. [Media and Content](#23-media-and-content)
24. [Data Science and Knowledge](#24-data-science-and-knowledge)
25. [Personal and Productivity](#25-personal-and-productivity)
26. [Scheduling and Automation](#26-scheduling-and-automation)
27. [CCI Context Tools](#27-cci-context-tools)
28. [Colony Debate](#28-colony-debate)
29. [Session Search](#29-session-search)
30. [MCP Tools](#30-mcp-tools)

---

## 1. Shell

### `bash`

Execute an arbitrary bash command. Returns stdout, stderr, and exit code.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | yes | Shell command to execute |
| `timeout` | number | no | Timeout in seconds (default 30) |
| `workdir` | string | no | Working directory; supports `~` and `$VAR` expansion |

**Permission:** `once` — prompts every call.
All parameters are validated. Use `start_managed_process` for long-running background tasks. Use `privileged_execute` instead of embedding `sudo` inside the command.

---

### `structured_bash`

Execute a bash command and return a structured JSON result. SENSE Module 4 — Universal Parsing Engine.

The output is automatically classified and parsed:

1. **JSON** — valid JSON output → `data_type: "json"`
2. **Tabular** — header row + aligned columns (`ps`, `ls -l`, `df`, `ip`) → `data_type: "tabular"`
3. **Key-Value** — `KEY=VALUE` or `Key: Value` patterns (`env`, `sysctl`, `/proc`) → `data_type: "keyvalue"`
4. **Raw** — safely truncated plain text → `data_type: "raw"`

Hard 5 MB stdout cap prevents OOM kills on verbose commands (`dumpsys`, `journalctl`).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | yes | Bash command to execute |
| `timeout` | number | no | Timeout in seconds (default 30) |
| `workdir` | string | no | Working directory |

**Permission:** `once`
Use instead of `bash` when you need to reason about or chain the output programmatically.

---

### `python_execute`

Run Python code in an isolated sandbox with optional access to safe Gorkbot tools via a Unix-domain RPC socket.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `code` | string | yes | Python code to execute |
| `timeout` | number | no | Timeout in seconds (default 30) |
| `packages` | array | no | pip packages to install before running |

**Permission:** `once`

The sandbox provides `import gorkbot_tools` which allows calling a restricted allowlist of tools: `read_file`, `write_file`, `list_directory`, `grep_content`, `search_files`, `web_fetch`, `http_request`, `session_search`.

---

### `privileged_execute`

Auto-escalation router for commands requiring elevated privileges. SENSE Module 2 — EAL (Escalation Abstraction Layer).

Tries escalation methods in order: direct (no escalation needed), `sudo`, `su`, `su -c`, ADB root. Never requires embedding `sudo` in `bash` commands.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | yes | Command to execute with escalated privileges |
| `method` | string | no | Force specific method: `sudo`, `su`, `adb` |
| `timeout` | number | no | Timeout in seconds |

**Permission:** `once`

---

## 2. File Operations

### `read_file`

Read the content of a file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path |
| `encoding` | string | no | `utf8` (default) or `base64` |

**Permission:** `session`

---

### `write_file`

Write content to a file (creates or overwrites).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path |
| `content` | string | yes | Content to write |
| `append` | bool | no | Append instead of overwrite (default false) |

**Permission:** `once`

---

### `edit_file`

Replace a specific string within a file. Safer than write_file for targeted edits.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path |
| `old_string` | string | yes | Exact string to replace |
| `new_string` | string | yes | Replacement string |
| `replace_all` | bool | no | Replace all occurrences (default false) |

**Permission:** `once`

---

### `multi_edit_file`

Apply multiple find-and-replace edits to a file in a single operation.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path |
| `edits` | array | yes | List of `{old_string, new_string}` objects |

**Permission:** `once`

---

### `list_directory`

List directory contents with metadata (size, permissions, modification time).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | no | Directory path (default: current directory) |
| `recursive` | bool | no | Recurse into subdirectories (default false) |
| `hidden` | bool | no | Include hidden files (default true) |

**Permission:** `session`

---

### `search_files`

Search for files by name pattern.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `pattern` | string | yes | Glob pattern (e.g., `*.go`, `*config*`) |
| `path` | string | no | Root search directory (default: cwd) |
| `type` | string | no | `f` (file), `d` (directory), or omit for both |

**Permission:** `session`

---

### `grep_content`

Search for text patterns within files.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `pattern` | string | yes | Regex or literal pattern |
| `path` | string | no | Directory or file to search |
| `recursive` | bool | no | Recurse (default true) |
| `ignore_case` | bool | no | Case-insensitive search |
| `line_numbers` | bool | no | Include line numbers in output |

**Permission:** `session`

---

### `file_info`

Get detailed metadata for a file or directory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File or directory path |

**Permission:** `always`
Returns: size, permissions (octal), owner, group, timestamps, MIME type.

---

### `delete_file`

Delete a file or directory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to delete |
| `recursive` | bool | no | Required for non-empty directories |

**Permission:** `once` — always asks. Destructive and irreversible.

---

## 3. Hashline File Tools

Hash-validated file operations that prevent stale-line failures in multi-turn editing sessions.

### `read_file_hashed`

Read a file and return content with per-line hash annotations. Subsequent `edit_file_hashed` calls verify hashes match before applying edits.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path |

**Permission:** `session`

---

### `edit_file_hashed`

Apply a line-hash-validated edit. Verifies that the line content matches the hash from a prior `read_file_hashed`, preventing edits to stale content.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path |
| `line_hash` | string | yes | Hash from prior `read_file_hashed` |
| `old_content` | string | yes | Line content (validated against hash) |
| `new_content` | string | yes | Replacement content |

**Permission:** `once`

---

### `ast_grep`

Structural code search using AST patterns (tree-sitter-based). Matches code structure rather than text.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `pattern` | string | yes | AST pattern |
| `path` | string | yes | Directory or file to search |
| `lang` | string | no | Language: `go`, `python`, `js`, `ts`, `rust`, etc. |

**Permission:** `session`
Requires `ast-grep` binary in PATH.

---

## 4. Git

### `git_status`

Show working tree status.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | no | Repository path (default: cwd) |
| `short` | bool | no | Short format output |

**Permission:** `always`

---

### `git_diff`

Show changes between working tree, index, and commits.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | no | Repository path |
| `cached` | bool | no | Show staged changes |
| `file` | string | no | Limit to specific file |
| `commit` | string | no | Compare to specific commit |

**Permission:** `always`

---

### `git_log`

Show commit history.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | no | Repository path |
| `limit` | number | no | Number of commits (default 10) |
| `oneline` | bool | no | Compact one-line format |
| `graph` | bool | no | Show ASCII branch graph |

**Permission:** `always`

---

### `git_commit`

Record changes to the repository.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | no | Repository path |
| `message` | string | yes | Commit message |
| `all` | bool | no | Auto-stage all modified files |
| `files` | array | no | Specific files to stage |

**Permission:** `once`

---

### `git_push`

Push local commits to remote.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | no | Repository path |
| `remote` | string | no | Remote name (default `origin`) |
| `branch` | string | no | Branch name |
| `force` | bool | no | Force push |
| `set_upstream` | bool | no | Set upstream tracking branch |

**Permission:** `once`

---

### `git_pull`

Fetch and merge from remote.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | no | Repository path |
| `remote` | string | no | Remote (default `origin`) |
| `branch` | string | no | Branch name |
| `rebase` | bool | no | Rebase instead of merge |

**Permission:** `once`

---

### `git_blame_analyze`

Analyze git blame output to find who last modified each line of a file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path to analyze |
| `line_start` | number | no | Starting line number |
| `line_end` | number | no | Ending line number |

**Permission:** `always`

---

## 5. Web and Network

### `web_fetch`

Fetch a URL and return the response body as clean text (not raw HTML).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | URL to fetch |
| `method` | string | no | HTTP method (default `GET`) |
| `headers` | object | no | Additional headers |
| `follow_redirects` | bool | no | Follow redirects (default true) |
| `timeout` | number | no | Timeout in seconds (default 30) |

**Permission:** `session`

---

### `http_request`

Make advanced HTTP requests with full control over headers and body.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Request URL |
| `method` | string | no | GET, POST, PUT, PATCH, DELETE (default GET) |
| `headers` | object | no | Request headers |
| `body` | string | no | Raw request body |
| `json` | object | no | JSON body (auto-sets Content-Type) |
| `auth` | string | no | Basic auth `user:password` |
| `bearer` | string | no | Bearer token for Authorization header |

**Permission:** `session`

---

### `check_port`

Check if a TCP port is open on a host.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `port` | number | yes | Port number |
| `host` | string | no | Hostname (default `localhost`) |
| `timeout` | number | no | Timeout in seconds (default 5) |

**Permission:** `always`

---

### `download_file`

Download a file from a URL to the local filesystem.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Download URL |
| `output` | string | yes | Output file path |
| `resume` | bool | no | Resume interrupted download |
| `follow_redirects` | bool | no | Follow redirects (default true) |

**Permission:** `once`

---

### `x_pull`

Fetch content from X (Twitter) posts or threads using the xAI API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | X post or thread URL |
| `include_replies` | bool | no | Include reply thread |

**Permission:** `session`

---

### `web_search`

Search the web and return results with titles, URLs, and snippets.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `num_results` | number | no | Number of results (default 5) |

**Permission:** `session`

---

### `web_reader`

Fetch and parse a web page, returning clean readable text stripped of navigation and boilerplate.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Page URL |
| `extract` | string | no | `text`, `markdown`, or `json` (default `text`) |

**Permission:** `session`

---

## 6. Web Scraping — Scrapling

Advanced web scraping tools backed by Scrapling (requires `scrapling` Python package).

### `scrapling_fetch`

Fetch a page and extract content using CSS or XPath selectors.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Target URL |
| `selector` | string | no | CSS or XPath selector |
| `output_format` | string | no | `text`, `html`, `json` |

**Permission:** `session`

---

### `scrapling_stealth`

Fetch a page using stealth mode to bypass bot detection.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Target URL |
| `selector` | string | no | Element selector to extract |

**Permission:** `session`

---

### `scrapling_dynamic`

Scrape a JavaScript-rendered page (uses a headless browser).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Target URL |
| `wait_for` | string | no | CSS selector to wait for before scraping |
| `selector` | string | no | Element selector to extract |

**Permission:** `session`

---

### `scrapling_extract`

Extract structured data from a page using a JSON schema.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Target URL |
| `schema` | object | yes | JSON schema describing fields to extract |

**Permission:** `session`

---

### `scrapling_search`

Search within a page's content for specific patterns.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Target URL |
| `pattern` | string | yes | Text pattern to find |

**Permission:** `session`

---

## 7. System

### `list_processes`

List running processes with resource usage.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `filter` | string | no | Filter by process name |
| `sort_by` | string | no | `cpu`, `memory`, or `pid` |
| `limit` | number | no | Max results (default 20) |

**Permission:** `always`

---

### `kill_process`

Terminate a process by PID or name.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `pid` | number | no | Process ID |
| `name` | string | no | Process name (kills all matching) |
| `signal` | string | no | `TERM` (default), `KILL`, `INT` |
| `force` | bool | no | Use SIGKILL regardless of signal setting |

**Permission:** `once`

---

### `env_var`

Get, set, list, or unset environment variables.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `get`, `set`, `list`, `unset` |
| `name` | string | no | Variable name (required for get/set/unset) |
| `value` | string | no | Value to set |

**Permission:** `session`

---

### `system_info`

Get system information.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `detail` | string | no | `all`, `os`, `cpu`, `memory`, `disk`, `uptime` |

**Permission:** `always`

---

### `disk_usage`

Analyze disk usage of a directory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Directory to analyze |
| `depth` | number | no | Output depth level (default 1) |
| `sort` | bool | no | Sort by size descending |

**Permission:** `always`

---

### `cron_manager`

Create, list, or remove cron jobs.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `list`, `add`, `remove` |
| `schedule` | string | no | Cron expression (for `add`) |
| `command` | string | no | Command to schedule (for `add`) |
| `id` | string | no | Job ID (for `remove`) |

**Permission:** `once`

---

### `backup_restore`

Create or restore file backups.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `backup` or `restore` |
| `source` | string | yes | Source path |
| `destination` | string | no | Destination path |

**Permission:** `once`

---

### `system_monitor`

Monitor system resources in real time and return a snapshot.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `duration` | number | no | Monitoring duration in seconds |
| `interval` | number | no | Sample interval in seconds |

**Permission:** `always`

---

### `pkg_install`

Install system packages via the native package manager (`apt`, `pkg`, `brew`, `choco`).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `packages` | array | yes | Package names to install |

**Permission:** `once`

---

## 8. DevOps and Cloud

### `docker_manager`

Manage Docker containers, images, and networks.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `ps`, `run`, `stop`, `rm`, `images`, `pull`, `logs`, `exec` |
| `container` | string | no | Container name or ID |
| `image` | string | no | Image name (for `run`/`pull`) |
| `command` | string | no | Command (for `run`/`exec`) |

**Permission:** `once`

---

### `k8s_kubectl`

Run kubectl commands against a Kubernetes cluster.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `args` | string | yes | kubectl arguments (e.g., `get pods -n default`) |
| `context` | string | no | kubectl context name |

**Permission:** `once`

---

### `aws_s3_sync`

Sync files to or from an S3 bucket.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source` | string | yes | Source path or `s3://bucket/prefix` |
| `destination` | string | yes | Destination path or `s3://bucket/prefix` |
| `delete` | bool | no | Delete files in destination not in source |

**Permission:** `once`

---

### `ngrok_tunnel`

Start or stop an ngrok tunnel to expose a local port publicly.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `start` or `stop` |
| `port` | number | no | Local port to expose |
| `proto` | string | no | `http` (default) or `tcp` |

**Permission:** `once`

---

### `ci_trigger`

Trigger a CI/CD pipeline (GitHub Actions, GitLab CI, etc.).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `platform` | string | yes | `github`, `gitlab`, `circleci` |
| `repo` | string | yes | Repository path (`owner/repo`) |
| `workflow` | string | no | Workflow file name |
| `ref` | string | no | Branch or tag name |

**Permission:** `once`

---

### `code_exec`

Execute code in a sandboxed environment with a specified language runtime.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `code` | string | yes | Code to execute |
| `language` | string | yes | `python`, `go`, `js`, `ruby`, `bash` |
| `timeout` | number | no | Timeout in seconds |

**Permission:** `once`

---

### `code2world`

Preview what a shell command or script will do before execution (action preview guard). SENSE guard.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | yes | Command to analyze |

**Permission:** `always`

---

### `rebuild`

Recompile Gorkbot with all dynamically created tools permanently integrated into the binary.

Runs `go build -o bin/gorkbot ./cmd/gorkbot/`.

**Permission:** `once`
**Parameters:** none required.

---

## 9. Android and Termux

These tools require Termux or ADB connectivity.

### `adb_screenshot`

Capture an Android screenshot via ADB.

**Permission:** `session`

---

### `adb_shell`

Execute a command in an ADB shell.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | yes | ADB shell command |

**Permission:** `once`

---

### `screen_capture`

Capture the current screen state.

**Permission:** `session`

---

### `device_info`

Return device hardware and software information.

**Permission:** `always`

---

### `launch_app`

Launch an Android app by package name.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `package` | string | yes | App package name |
| `activity` | string | no | Specific activity to start |

**Permission:** `once`

---

### `kill_app`

Force-stop an Android app.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `package` | string | yes | App package name |

**Permission:** `once`

---

### `notification_send`

Send a notification via Termux API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `title` | string | yes | Notification title |
| `content` | string | yes | Notification body |
| `id` | number | no | Notification ID |

**Permission:** `once`

---

### `intent_broadcast`

Broadcast an Android intent.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | Intent action |
| `data` | string | no | Intent data URI |
| `extras` | object | no | Intent extras |

**Permission:** `once`

---

### `logcat_dump`

Dump Android logcat output.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `lines` | number | no | Number of lines (default 100) |
| `filter` | string | no | Log tag filter |
| `level` | string | no | Minimum level: `V`, `D`, `I`, `W`, `E` |

**Permission:** `session`

---

### `termux_sensor`

Read sensor data via Termux API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sensor` | string | yes | Sensor name (e.g., `accelerometer`) |
| `duration` | number | no | Reading duration in seconds |

**Permission:** `session`

---

### `termux_location`

Get device GPS location via Termux API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `provider` | string | no | `gps`, `network`, or `passive` |

**Permission:** `once`

---

### `apk_decompile`

Decompile an APK file to inspect its source.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to the APK file |
| `output` | string | no | Output directory |

**Permission:** `once`
Requires `jadx` or `apktool` in PATH.

---

## 10. Vision — Screen Capture and Analysis

Vision tools use `grok-2-vision-1212` or the ADB screen capture pipeline.

### `vision_screen`

Capture the current screen and analyze it with Grok Vision.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | What to analyze or look for in the screenshot |

**Permission:** `session`

---

### `vision_file`

Analyze an image file with Grok Vision.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to image file |
| `prompt` | string | yes | Analysis question or instruction |

**Permission:** `session`

---

### `vision_ocr`

Extract text from an image using OCR (via Grok Vision).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | no | Path to image (uses screen capture if omitted) |
| `region` | object | no | `{x, y, width, height}` crop region |

**Permission:** `session`

---

### `vision_find`

Find a specific UI element or text on screen.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `target` | string | yes | Element description or text to find |

**Permission:** `session`

---

### `vision_watch`

Watch the screen for a change or condition, then return.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `condition` | string | yes | What to wait for |
| `timeout` | number | no | Timeout in seconds (default 30) |

**Permission:** `session`

---

### `frontend_design`

Generate HTML/CSS/JS frontend code from a natural-language description, optionally using a screenshot as reference.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `description` | string | yes | UI description |
| `framework` | string | no | `vanilla`, `tailwind`, `bootstrap` |
| `reference_image` | string | no | Path to reference image |

**Permission:** `session`

---

## 11. Task Management

### `todo_write`

Write or update the session task list.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tasks` | array | yes | List of `{id, description, status}` objects; status: `pending`, `in_progress`, `completed` |

**Permission:** `always`

---

### `todo_read`

Read the current session task list.

**Permission:** `always`
**Parameters:** none.

---

### `complete`

Mark a task as complete and return a summary.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | yes | Task ID to complete |
| `result` | string | no | Completion summary |

**Permission:** `always`

---

## 12. AI and Skills

### `consultation`

Consult the specialist AI (Gemini by default) for a second opinion or architectural advice.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `question` | string | yes | Question or task for the specialist |
| `context` | string | no | Optional context to include |

**Permission:** `session`

---

### `parse_docx`

Extract text and structure from a .docx Word document.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to .docx file |

**Permission:** `session`

---

### `parse_xlsx`

Extract data from an Excel .xlsx file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to .xlsx file |
| `sheet` | string | no | Sheet name (default: first sheet) |

**Permission:** `session`

---

### `parse_pdf`

Extract text from a PDF file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to PDF |
| `pages` | string | no | Page range (e.g., `1-5`) |

**Permission:** `session`

---

### `parse_pptx`

Extract content from a PowerPoint .pptx file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to .pptx |

**Permission:** `session`

---

### `ai_image_generate`

Generate an image from a text prompt via an AI image API.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | Image description |
| `output` | string | yes | Output file path |
| `size` | string | no | Image dimensions (e.g., `1024x1024`) |

**Permission:** `once`

---

### `ai_summarize_audio`

Transcribe and summarize an audio file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Audio file path |
| `language` | string | no | Language code (e.g., `en`) |

**Permission:** `session`

---

### `ml_model_run`

Run a local ML model (GGUF format via llama.cpp) on a prompt.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model_path` | string | yes | Path to GGUF model file |
| `prompt` | string | yes | Input prompt |
| `max_tokens` | number | no | Max output tokens |

**Permission:** `once`

---

## 13. Meta and Tool Creation

### `create_tool`

Generate a new dynamic tool at runtime. The tool is immediately hot-loaded and available without restart.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Tool name (snake_case) |
| `description` | string | yes | Tool description |
| `category` | string | yes | Tool category |
| `command` | string | yes | Shell command with `{{param}}` placeholders |
| `parameters` | object | yes | Parameter definitions `{name: {type, description}}` |
| `requires_permission` | bool | no | Whether permission is required (default true) |
| `default_permission` | string | no | Default permission level (default `once`) |

**Permission:** `once`
Generated tools are persisted to `dynamic_tools.json`. Use `rebuild` to permanently compile them into the binary.

---

### `modify_tool`

Modify an existing dynamic tool's definition.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Tool to modify |
| `description` | string | no | New description |
| `command` | string | no | New command template |

**Permission:** `once`

---

### `list_tools`

List all registered tools.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `category` | string | no | Filter by category |
| `format` | string | no | `table`, `json`, `detailed` |

**Permission:** `always`

---

### `tool_info`

Get full details for a specific tool including parameters and current permission level.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tool_name` | string | yes | Tool name |

**Permission:** `always`

---

### `context_stats`

Return the current context window usage, token count, cost estimate, and execution mode.

**Permission:** `always`
**Parameters:** none.

---

### `gorkbot_status`

Return a summary of the current Gorkbot system state (providers, tools loaded, mode, session).

**Permission:** `always`
**Parameters:** none.

---

### `define_command`

Define a custom slash command that invokes a prompt template.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Command name (without `/`) |
| `description` | string | yes | Command description |
| `template` | string | yes | Prompt template with `{{args}}` placeholder |

**Permission:** `once`

---

## 14. Background Agents and Pipeline

### `spawn_agent`

Spawn an AI sub-agent to run a task. Supports optional git worktree isolation.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task` | string | yes | Task description for the sub-agent |
| `model` | string | no | Model to use (default: primary) |
| `isolated` | bool | no | Run in isolated git worktree (auto-removed on completion) |
| `max_turns` | number | no | Maximum tool turns allowed |

**Permission:** `once`
Depth-limited to 4 levels to prevent infinite delegation chains.

---

### `run_pipeline`

Execute a named agent pipeline synchronously.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `agent_type` | string | yes | Pipeline agent type name |
| `task` | string | yes | Task to execute |

**Permission:** `once`

---

### `report_finding`

Report a security or quality finding from within a sub-agent.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `title` | string | yes | Finding title |
| `severity` | string | yes | `critical`, `high`, `medium`, `low`, `info` |
| `description` | string | yes | Detailed description |
| `evidence` | string | no | Supporting evidence |

**Permission:** `always`

---

## 15. Process Management

### `start_managed_process`

Start a long-running background process managed by Gorkbot.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Process identifier |
| `command` | string | yes | Command to run |
| `workdir` | string | no | Working directory |

**Permission:** `once`

---

### `list_managed_processes`

List all currently managed background processes and their status.

**Permission:** `always`
**Parameters:** none.

---

### `stop_managed_process`

Stop a managed background process.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Process identifier |

**Permission:** `once`

---

### `read_managed_process`

Read output from a managed background process.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Process identifier |
| `lines` | number | no | Number of recent output lines |

**Permission:** `session`

---

## 16. Worktree Management

### `create_worktree`

Create a new git worktree for isolated parallel development.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `branch` | string | yes | Branch name for the new worktree |
| `path` | string | no | Worktree directory (auto-generated if omitted) |
| `repo` | string | no | Repository path (default: cwd) |

**Permission:** `once`

---

### `list_worktrees`

List all git worktrees for the current repository.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | no | Repository path (default: cwd) |

**Permission:** `always`

---

### `remove_worktree`

Remove a git worktree.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Worktree path to remove |
| `force` | bool | no | Force removal even if branch has unmerged changes |

**Permission:** `once`

---

### `integrate_worktree`

Merge a worktree branch back into the main branch and clean up.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Worktree path |
| `target_branch` | string | no | Branch to merge into (default: `main`) |

**Permission:** `once`

---

## 17. SENSE Memory

### `record_engram`

Record a behaviour preference or long-term memory fact as an engram. Engrams are injected into every future system prompt.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | The preference or fact to remember |
| `category` | string | no | Category tag for organisation |

**Permission:** `always`

---

### `record_fact`

Record a short-term fact in AgeMem.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `fact` | string | yes | Fact to remember |
| `ttl` | number | no | Time-to-live in hours |

**Permission:** `always`

---

### `record_user_pref`

Record a user preference in AgeMem.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `preference` | string | yes | Preference description |

**Permission:** `always`

---

### `read_brain`

Read current brain state (AgeMem facts + engrams + GoalLedger summary).

**Permission:** `always`
**Parameters:** none.

---

### `forget_fact`

Remove a specific fact from AgeMem.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `fact` | string | yes | Fact text to forget |

**Permission:** `always`

---

### `sense_discovery`

Run SENSE system discovery: scan loaded tools, skills, and capabilities.

**Permission:** `always`
**Parameters:** none.

---

### `sense_check`

Run SENSE consistency check on the current system state.

**Permission:** `always`
**Parameters:** none.

---

### `sense_evolve`

Run SENSE skill evolver to analyse usage patterns and suggest improvements.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `min_evidence` | number | no | Minimum usage evidence required (default 3) |
| `dry_run` | bool | no | Preview changes without applying (default false) |

**Permission:** `once`

---

### `sense_sanitize`

Run the SENSE input sanitizer on provided text and return validation results.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | yes | Text to validate |

**Permission:** `always`

---

## 18. Self-Introspection

### `query_routing_stats`

Return ARC Router routing statistics (workflow type distribution, count by class).

**Permission:** `always`
**Parameters:** none.

---

### `query_heuristics`

Return MEL heuristics from the VectorStore relevant to a query.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Query to find relevant heuristics |
| `limit` | number | no | Maximum heuristics to return (default 5) |

**Permission:** `always`

---

### `query_memory_state`

Return a summary of all memory subsystems (AgeMem, Engrams, GoalLedger, UnifiedMemory).

**Permission:** `always`
**Parameters:** none.

---

### `query_system_state`

Return a full system diagnostic snapshot (providers, mode, context stats, tool count).

**Permission:** `always`
**Parameters:** none.

---

### `query_audit_log`

Query the SQLite tool audit log.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tool` | string | no | Filter by tool name |
| `limit` | number | no | Number of entries (default 20) |
| `success_only` | bool | no | Show only successful calls |
| `error_only` | bool | no | Show only failed calls |

**Permission:** `always`

---

## 19. Goal Ledger

### `add_goal`

Add a new goal to the cross-session Goal Ledger.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `title` | string | yes | Goal title |
| `description` | string | no | Detailed goal description |
| `priority` | string | no | `high`, `medium`, `low` |

**Permission:** `always`

---

### `list_goals`

List all open goals in the Goal Ledger.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `status` | string | no | `open`, `closed`, or `all` (default `open`) |

**Permission:** `always`

---

### `close_goal`

Mark a goal as completed.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `goal_id` | string | yes | Goal ID to close |
| `outcome` | string | no | Completion summary |

**Permission:** `always`

---

## 20. Security Assessment

These tools are in the `sec` pack, which is not loaded by default. Enable with `GORKBOT_TOOL_PACKS=ALL` or add `sec` to your pack list. Use only on systems you own or have explicit written permission to test.

### `nmap_scan`

Network port scanner.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `target` | string | yes | IP address or hostname |
| `ports` | string | no | Port specification (e.g., `22,80,443`, `1-1000`) |
| `flags` | string | no | Additional nmap flags |

**Permission:** `once`
Requires `nmap` binary.

---

### `nuclei_scan`

Vulnerability scanner using nuclei templates.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `target` | string | yes | Target URL |
| `templates` | string | no | Template path or category |
| `severity` | string | no | Filter by: `critical`, `high`, `medium`, `low` |

**Permission:** `once`
Requires `nuclei` binary.

---

### `sqlmap_scan`

SQL injection scanner.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Target URL with parameter |
| `data` | string | no | POST data |
| `level` | number | no | Detection level (1-5) |
| `risk` | number | no | Risk level (1-3) |

**Permission:** `once`
Requires `sqlmap` binary.

---

### `totp_generate`

Generate a TOTP code from a base32 secret.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `secret` | string | yes | Base32-encoded TOTP secret |

**Permission:** `once`

---

For a complete list of all 30+ security tools (nmap, masscan, nikto, gobuster, ffuf, hydra, hashcat, john, metasploit, burp, impacket, tshark, shodan, subfinder, enum4linux, smbmap, linpeas, and more), see the `sec` pack in `pkg/tools/packs.go`.

---

## 21. Security Exploitation Tools

The `sec` pack also includes:

- `hydra_run` — Brute-force login credentials
- `hashcat_run` — GPU-accelerated hash cracking
- `john_run` — Password hash cracking
- `hash_identify` — Identify hash type
- `searchsploit_query` — Search Exploit-DB
- `cve_lookup` — Look up CVE details
- `metasploit_rpc` — Metasploit Framework RPC client
- `burp_suite_scan` — Burp Suite scanner integration
- `impacket_attack` — Impacket protocol attacks (PtH, PtT, DCSync)
- `tshark_capture` — Network packet capture

All require their respective binaries in PATH. Use `context_stats` or the capability pre-flight system to verify availability before execution.

---

## 22. Database

### `sqlite_query`

Execute a SQL query against a SQLite database file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to .sqlite or .db file |
| `query` | string | yes | SQL query |

**Permission:** `session`

---

### `postgres_connect`

Execute a SQL query against a PostgreSQL database.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `connection_string` | string | yes | PostgreSQL connection string |
| `query` | string | yes | SQL query |

**Permission:** `once`

---

### `db_query`

Generic database query tool supporting multiple database backends.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `dsn` | string | yes | Data source name |
| `query` | string | yes | SQL query |
| `db_type` | string | no | `sqlite`, `postgres`, `mysql` |

**Permission:** `once`

---

### `db_migrate`

Run database migrations.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Path to migration directory |
| `dsn` | string | yes | Database DSN |
| `direction` | string | no | `up` (default) or `down` |

**Permission:** `once`

---

## 23. Media and Content

### `image_process`

Process images: resize, crop, convert format, apply filters.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Input image path |
| `output` | string | yes | Output path |
| `operation` | string | yes | `resize`, `crop`, `convert`, `rotate`, `grayscale` |
| `width` | number | no | Target width (for resize) |
| `height` | number | no | Target height (for resize) |
| `format` | string | no | Output format: `jpg`, `png`, `webp` |

**Permission:** `once`

---

### `media_convert`

Convert media files between formats using ffmpeg.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `input` | string | yes | Input file path |
| `output` | string | yes | Output file path |
| `options` | string | no | Additional ffmpeg options |

**Permission:** `once`
Requires `ffmpeg` binary.

---

### `ffmpeg_pro`

Advanced ffmpeg operations with full flag control.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `args` | string | yes | Full ffmpeg argument string |

**Permission:** `once`
Requires `ffmpeg` binary.

---

### `audio_transcribe`

Transcribe audio to text using Whisper or similar.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Audio file path |
| `language` | string | no | Language code |
| `model` | string | no | Model size: `tiny`, `base`, `small`, `medium`, `large` |

**Permission:** `session`

---

### `tts_generate`

Generate speech audio from text.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | yes | Text to speak |
| `output` | string | yes | Output audio file path |
| `voice` | string | no | Voice name or ID |

**Permission:** `once`

---

## 24. Data Science and Knowledge

### `csv_pivot`

Load a CSV file and return a pivot table or aggregation.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | CSV file path |
| `index` | string | no | Column to use as row index |
| `columns` | string | no | Column to pivot on |
| `values` | string | no | Column to aggregate |
| `aggfunc` | string | no | Aggregation: `sum`, `mean`, `count` |

**Permission:** `session`

---

### `plot_generate`

Generate a chart from data and save as an image.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `data` | array | yes | Data points (array of `{x, y}` objects) |
| `chart_type` | string | no | `line`, `bar`, `scatter`, `pie` |
| `output` | string | yes | Output image path |
| `title` | string | no | Chart title |

**Permission:** `once`
Requires Python + matplotlib.

---

### `arxiv_search`

Search arXiv for research papers.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `max_results` | number | no | Maximum results (default 5) |
| `sort_by` | string | no | `relevance`, `lastUpdatedDate`, `submittedDate` |

**Permission:** `session`

---

### `whois_lookup`

Look up WHOIS registration information for a domain.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `domain` | string | yes | Domain name |

**Permission:** `session`

---

### `jupyter`

Create or execute a Jupyter notebook cell.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `create`, `execute`, `read` |
| `path` | string | yes | Notebook file path |
| `code` | string | no | Code to add/execute (for `create`/`execute`) |
| `cell_index` | number | no | Cell index (for `execute`) |

**Permission:** `once`
Requires `jupyter` in PATH.

---

## 25. Personal and Productivity

### `send_email`

Send an email via SMTP.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `to` | string | yes | Recipient email address |
| `subject` | string | yes | Email subject |
| `body` | string | yes | Email body |
| `from` | string | no | Sender address |

**Permission:** `once`

---

### `slack_notify`

Send a message to a Slack channel.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `channel` | string | yes | Channel name or ID |
| `message` | string | yes | Message text |
| `webhook_url` | string | no | Slack webhook URL (uses env var if omitted) |

**Permission:** `once`

---

### `calendar_manage`

Manage calendar events (create, list, delete).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `create`, `list`, `delete` |
| `title` | string | no | Event title (for `create`) |
| `start` | string | no | Start time ISO8601 (for `create`) |
| `end` | string | no | End time ISO8601 (for `create`) |

**Permission:** `once`

---

## 26. Scheduling and Automation

### `schedule_task`

Schedule a task to run at a future time or on a cron schedule.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Task name |
| `prompt` | string | yes | Prompt to execute |
| `schedule` | string | yes | Cron expression or ISO8601 time |
| `once` | bool | no | Run only once (not recurring) |

**Permission:** `once`

---

### `list_scheduled_tasks`

List all scheduled tasks with their next run time and status.

**Permission:** `always`
**Parameters:** none.

---

### `cancel_scheduled_task`

Cancel a scheduled task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Task name to cancel |

**Permission:** `once`

---

### `pause_resume_scheduled_task`

Pause or resume a scheduled task without deleting it.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Task name |
| `action` | string | yes | `pause` or `resume` |

**Permission:** `once`

---

## 27. CCI Context Tools

These tools manage the CCI three-tier memory system. They are registered separately from tool packs via `RegisterCCITools()`.

### `mcp_context_get_subsystem`

Retrieve a Tier 3 (Cold) subsystem specification.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Subsystem name |

**Permission:** `always`

---

### `mcp_context_update_subsystem`

Create or update a Tier 3 subsystem specification.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Subsystem name |
| `content` | string | yes | Specification content (markdown) |

**Permission:** `once`

---

### `mcp_context_list_subsystems`

List all available Tier 3 subsystem specifications.

**Permission:** `always`
**Parameters:** none.

---

## 28. Colony Debate

### `colony_debate`

Run a multi-perspective colony debate on a question or problem. Multiple simulated agents argue different positions and a synthesis is returned.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `topic` | string | yes | Question or problem to debate |
| `perspectives` | number | no | Number of perspectives (default 3) |
| `rounds` | number | no | Debate rounds (default 2) |

**Permission:** `once`
The colony runner must be wired via `Registry.SetColonyRunner()` for this tool to function.

---

## 29. Session Search

### `session_search`

Search past conversation history across all SQLite-persisted sessions.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `limit` | number | no | Maximum results (default 10) |
| `session_id` | string | no | Restrict to a specific session |

**Permission:** `session`
Requires the SQLite persistence store to be wired via `Registry.SetPersistStore()`.

---

## 30. MCP Tools

MCP (Model Context Protocol) tools are registered automatically when `pkg/mcp.Manager.RegisterTools()` is called after a successful `LoadAndStart()`. They appear as `mcp_<server>_<toolname>` in the registry.

All MCP tools:
- Use `PermissionOnce` by default
- Belong to the `custom` category
- Forward parameters directly to the MCP server
- Return the server's text content as the result

Check active MCP tools with `/mcp status` or `/tools`.

Example MCP tools from the sample `configs/mcp.json`:

| Tool Name | Server | Description |
|-----------|--------|-------------|
| `mcp_sequential_thinking_*` | sequential-thinking | Structured reasoning chains |
| `mcp_filesystem_*` | filesystem | File I/O within allowed directories |
| `mcp_memory_*` | memory | Cross-session entity graph |
| `mcp_fetch_*` | fetch | Raw HTTP fetching |
| `mcp_time_*` | time | Current time and timezone |
| `mcp_puppeteer_*` | puppeteer | Headless browser with JavaScript |
| `mcp_notebooklm_*` | notebooklm | Google NotebookLM notebooks |
| `mcp_github_*` | github | GitHub API (requires token) |
| `mcp_brave_search_*` | brave-search | Brave web search API |
