# Gorkbot Tool Reference

**Version:** 3.5.1

Gorkbot ships with 162+ built-in tools spanning 33+ categories. This document provides a complete reference for every tool: its name, description, parameters, permission level, and usage notes.

All tools are registered in `pkg/tools/registry.go → RegisterDefaultTools()` and routed through the unified permission and caching pipeline. Additional tools are registered from `cmd/gorkbot/main.go` (process management, subagents) and `pkg/cci` (CCI context tools).

---

## Permission Levels

| Level | Meaning |
|-------|---------|
| `always` | Pre-approved — no prompt shown |
| `session` | Approved for this session only |
| `once` | Prompts every call |
| `never` | Blocked permanently |

Default permissions shown below are the built-in defaults. Users can override any tool permission via the TUI or `/permissions`.

---

## Table of Contents

1. [Shell](#1-shell)
2. [File Operations](#2-file-operations)
3. [Hashline File Tools](#3-hashline-file-tools)
4. [Git](#4-git)
5. [Web & Network](#5-web--network)
6. [System](#6-system)
7. [Task Management](#7-task-management)
8. [AI / Skills](#8-ai--skills)
9. [Meta / Introspection](#9-meta--introspection)
10. [Background Agents](#10-background-agents)
11. [Process Management](#11-process-management)
12. [Database](#12-database)
13. [Security — Recon & Audit](#13-security--recon--audit)
14. [Security — Exploitation & Credentials](#14-security--exploitation--credentials)
15. [Security — Assessment Helpers](#15-security--assessment-helpers)
16. [DevOps & Cloud](#16-devops--cloud)
17. [Android / Termux — Device Control](#17-android--termux--device-control)
18. [Android / Termux — APIs & System](#18-android--termux--apis--system)
19. [Vision (Screen Capture & Analysis)](#19-vision-screen-capture--analysis)
20. [Worktree Management](#20-worktree-management)
21. [Media & Content](#21-media--content)
22. [Data Science & Knowledge](#22-data-science--knowledge)
23. [Personal & Productivity](#23-personal--productivity)
24. [Scheduling & Automation](#24-scheduling--automation)
25. [Web Scraping (Scrapling)](#25-web-scraping-scrapling)
26. [Jupyter Notebooks](#26-jupyter-notebooks)
27. [SENSE Memory](#27-sense-memory)
28. [Self-Introspection](#28-self-introspection)
29. [Goal Ledger](#29-goal-ledger)
30. [Security Context](#30-security-context)
31. [Agentic Pipeline](#31-agentic-pipeline)
32. [CCI Context Tools](#32-cci-context-tools)
33. [Dynamic Tool Creation](#33-dynamic-tool-creation)
34. [RAG Memory Plugin](#34-rag-memory-plugin)

---

## 1. Shell

### `bash`
Execute an arbitrary bash command.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | yes | Shell command to execute |
| `timeout` | int | no | Timeout in seconds (default 30) |
| `workdir` | string | no | Working directory |

**Permission:** `once` — prompts every call.
All parameters are shell-escaped before execution. Execution is wrapped in a timeout. Use `start_managed_process` for long-running background tasks.

---

## 2. File Operations

### `read_file`
Read the content of a file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path |
| `encoding` | string | `utf8` (default) or `base64` |

**Permission:** `session`

### `write_file`
Write content to a file (creates or overwrites).

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path |
| `content` | string | Content to write |
| `append` | bool | Append instead of overwrite (default false) |

**Permission:** `once`

### `edit_file`
Replace a specific string within a file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path |
| `old_string` | string | Exact string to replace |
| `new_string` | string | Replacement string |
| `replace_all` | bool | Replace all occurrences (default false) |

**Permission:** `once`

### `multi_edit_file`
Apply multiple find-and-replace edits to a file in a single operation.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path |
| `edits` | array | List of `{old_string, new_string}` objects |

**Permission:** `once`

### `list_directory`
List directory contents with metadata.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Directory path |
| `recursive` | bool | Recurse into subdirectories |
| `hidden` | bool | Include hidden files |

**Permission:** `session`
Returns: name, size, permissions, modification time, type for each entry.

### `search_files`
Search for files by name pattern using `find`.

| Parameter | Type | Description |
|-----------|------|-------------|
| `pattern` | string | Glob pattern (e.g., `*.go`, `*config*`) |
| `path` | string | Root search directory |
| `type` | string | `f` (file), `d` (directory), or omit for both |

**Permission:** `session`

### `grep_content`
Search for text patterns within files (ripgrep-style).

| Parameter | Type | Description |
|-----------|------|-------------|
| `pattern` | string | Regex or literal pattern |
| `path` | string | Directory or file to search |
| `recursive` | bool | Recurse (default true) |
| `ignore_case` | bool | Case-insensitive search |
| `line_numbers` | bool | Include line numbers in output |

**Permission:** `session`

### `file_info`
Get detailed metadata for a file or directory.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File or directory path |

**Permission:** `always`
Returns: size, permissions (octal), owner, group, timestamps, MIME type.

### `delete_file`
Delete a file or directory.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path to delete |
| `recursive` | bool | Required for directories |

**Permission:** `once` — always asks. Destructive and irreversible.

---

## 3. Hashline File Tools

Hash-validated file operations that prevent stale-line failures in multi-turn editing sessions.

### `read_file_hashed`
Read a file and return content with line-hash annotations. Subsequent `edit_file_hashed` calls verify hashes match before applying edits.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path |

**Permission:** `session`

### `edit_file_hashed`
Apply a line-hash-validated edit. The tool verifies that the line content matches the hash from a prior `read_file_hashed` before modifying it, preventing edits to stale content.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path |
| `line_hash` | string | Hash from prior read |
| `old_content` | string | Line content (validated against hash) |
| `new_content` | string | Replacement content |

**Permission:** `once`

### `ast_grep`
Structural code search using AST patterns (tree-sitter-based). Matches code structure rather than text.

| Parameter | Type | Description |
|-----------|------|-------------|
| `pattern` | string | AST pattern |
| `path` | string | Directory to search |
| `lang` | string | Language (`go`, `python`, `js`, `ts`, etc.) |

**Permission:** `session`

---

## 4. Git

### `git_status`
Show working tree status.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Repository path (default: cwd) |
| `short` | bool | Short format output |

**Permission:** `always`

### `git_diff`
Show changes between working tree, index, and commits.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Repository path |
| `cached` | bool | Show staged changes |
| `file` | string | Limit to specific file |
| `commit` | string | Compare to specific commit |

**Permission:** `always`

### `git_log`
Show commit history.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Repository path |
| `limit` | int | Number of commits (default 10) |
| `oneline` | bool | Compact one-line format |
| `graph` | bool | ASCII graph |

**Permission:** `always`

### `git_commit`
Record changes to the repository.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Repository path |
| `message` | string | Commit message |
| `all` | bool | Auto-stage all modified files |
| `files` | array | Specific files to stage |

**Permission:** `once`

### `git_push`
Push local commits to remote.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Repository path |
| `remote` | string | Remote name (default `origin`) |
| `branch` | string | Branch name |
| `force` | bool | Force push (use with caution) |
| `set_upstream` | bool | Set upstream tracking |

**Permission:** `once`

### `git_pull`
Fetch and merge from remote.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Repository path |
| `remote` | string | Remote (default `origin`) |
| `branch` | string | Branch name |
| `rebase` | bool | Rebase instead of merge |

**Permission:** `once`

---

## 5. Web & Network

### `web_fetch`
Fetch a URL using curl and return the response body.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | URL to fetch |
| `method` | string | HTTP method (default `GET`) |
| `headers` | object | Additional headers |
| `follow_redirects` | bool | Follow redirects (default true) |
| `timeout` | int | Timeout in seconds (default 30) |

**Permission:** `session`

### `http_request`
Make advanced HTTP requests with full control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Request URL |
| `method` | string | GET, POST, PUT, PATCH, DELETE |
| `headers` | object | Request headers |
| `body` | string | Raw request body |
| `json` | object | JSON body (sets Content-Type) |
| `auth` | string | Basic auth `user:pass` |
| `bearer` | string | Bearer token |

**Permission:** `session`

### `check_port`
Check if a TCP port is open.

| Parameter | Type | Description |
|-----------|------|-------------|
| `port` | int | Port number |
| `host` | string | Hostname (default `localhost`) |
| `timeout` | int | Timeout in seconds (default 5) |

**Permission:** `always`

### `download_file`
Download a file from URL to the local filesystem.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Download URL |
| `output` | string | Output file path |
| `resume` | bool | Resume interrupted download |
| `follow_redirects` | bool | Follow redirects |

**Permission:** `once`

### `x_pull`
Fetch content from X (Twitter) posts or threads using the xAI API.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | X post or thread URL |
| `include_replies` | bool | Include reply thread |

**Permission:** `session`

---

## 6. System

### `list_processes`
List running processes with resource usage.

| Parameter | Type | Description |
|-----------|------|-------------|
| `filter` | string | Filter by process name |
| `sort_by` | string | `cpu`, `memory`, or `pid` |
| `limit` | int | Max results (default 20) |

**Permission:** `always`

### `kill_process`
Terminate a process.

| Parameter | Type | Description |
|-----------|------|-------------|
| `pid` | int | Process ID (or use `name`) |
| `name` | string | Process name (kills all matching) |
| `signal` | string | `TERM` (default), `KILL`, `INT` |
| `force` | bool | Use SIGKILL regardless |

**Permission:** `once`

### `env_var`
Get, set, list, or unset environment variables.

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `get`, `set`, `list`, `unset` |
| `name` | string | Variable name |
| `value` | string | Value to set |

**Permission:** `session`

### `system_info`
Get system information.

| Parameter | Type | Description |
|-----------|------|-------------|
| `detail` | string | `all`, `os`, `cpu`, `memory`, `disk`, `uptime` |

**Permission:** `always`

### `disk_usage`
Analyze disk usage of a directory.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Directory to analyze |
| `depth` | int | Depth of output (default 1) |
| `sort` | bool | Sort by size |

**Permission:** `always`

---

## 7. Task Management

### `todo_write`
Write or update the session task list.

| Parameter | Type | Description |
|-----------|------|-------------|
| `tasks` | array | List of `{id, description, status}` objects |

**Permission:** `always`

### `todo_read`
Read the current session task list.

**Permission:** `always`

### `complete`
Mark a task as complete and return a summary.

| Parameter | Type | Description |
|-----------|------|-------------|
| `task_id` | string | Task ID to complete |
| `result` | string | Completion summary |

**Permission:** `always`

---

## 8. AI / Skills

### `consultation`
Consult the specialist AI (Gemini by default) for a second opinion or architectural advice.

| Parameter | Type | Description |
|-----------|------|-------------|
| `question` | string | Question or task for the specialist |
| `context` | string | Optional context to include |

**Permission:** `session`

### `web_search`
Search the web and return results.

| Parameter | Type | Description |
|-----------|------|-------------|
| `query` | string | Search query |
| `num_results` | int | Number of results (default 5) |

**Permission:** `session`

### `web_reader`
Fetch and parse a web page, returning clean readable text.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Page URL |
| `extract` | string | `text`, `markdown`, or `json` |

**Permission:** `session`

### `parse_docx`
Extract text and structure from a .docx file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path to .docx file |

**Permission:** `session`

### `parse_xlsx`
Extract data from an Excel .xlsx file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path to .xlsx file |
| `sheet` | string | Sheet name (default: first sheet) |

**Permission:** `session`

### `parse_pdf`
Extract text from a PDF file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path to PDF |
| `pages` | string | Page range (e.g., `1-5`) |

**Permission:** `session`

### `parse_pptx`
Extract content from a PowerPoint .pptx file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path to .pptx |

**Permission:** `session`

### `frontend_design`
Generate HTML/CSS/JS frontend code from a natural-language description.

| Parameter | Type | Description |
|-----------|------|-------------|
| `description` | string | UI description |
| `framework` | string | `vanilla`, `tailwind`, `bootstrap` |

**Permission:** `session`

### `ai_image_generate`
Generate an image from a text prompt via an AI image API.

| Parameter | Type | Description |
|-----------|------|-------------|
| `prompt` | string | Image description |
| `output` | string | Output file path |
| `size` | string | Image dimensions (e.g., `1024x1024`) |

**Permission:** `once`

### `ai_summarize_audio`
Transcribe and summarize an audio file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Audio file path |
| `language` | string | Language code (e.g., `en`) |

**Permission:** `session`

### `ml_model_run`
Run a local ML model (GGUF format via llama.cpp) on a prompt.

| Parameter | Type | Description |
|-----------|------|-------------|
| `model_path` | string | Path to GGUF model file |
| `prompt` | string | Input prompt |
| `max_tokens` | int | Max output tokens |

**Permission:** `once`

---

## 9. Meta / Introspection

### `create_tool`
Generate a new dynamic tool at runtime. The tool is immediately available; no restart required.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Tool name (snake_case) |
| `description` | string | Tool description |
| `category` | string | Tool category |
| `command` | string | Shell command with `{{param}}` placeholders |
| `parameters` | object | Parameter definitions `{name: type}` |
| `requires_permission` | bool | Whether permission is required |
| `default_permission` | string | Default permission level |

**Permission:** `once`
Generated tools are written to `dynamic_tools.json` and reloaded on next startup. Use `rebuild` to permanently compile them into the binary.

### `modify_tool`
Modify an existing dynamic tool's definition.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Tool to modify |
| `description` | string | New description |
| `command` | string | New command template |

**Permission:** `once`

### `list_tools`
List all registered tools.

| Parameter | Type | Description |
|-----------|------|-------------|
| `category` | string | Filter by category |
| `format` | string | `table`, `json`, `detailed` |

**Permission:** `always`

### `tool_info`
Get full details for a specific tool.

| Parameter | Type | Description |
|-----------|------|-------------|
| `tool_name` | string | Tool name |

**Permission:** `always`

### `audit_tool_call`
Record an audit log entry for a tool call (for compliance and tracing).

| Parameter | Type | Description |
|-----------|------|-------------|
| `tool_name` | string | Tool that was called |
| `params` | object | Parameters used |
| `result_summary` | string | Brief result description |

**Permission:** `always`

### `context_stats`
Return the current context window usage, cost estimate, and execution mode.

**Permission:** `always`

### `rebuild`
Recompile Gorkbot with all dynamically created tools permanently integrated into the binary.

**Permission:** `once`
Runs `go build -o bin/gorkbot ./cmd/gorkbot/`.

### `define_command`
Define a custom slash command that invokes a prompt template.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Command name (without `/`) |
| `description` | string | Command description |
| `template` | string | Prompt template with `{{args}}` placeholder |

**Permission:** `once`

---

## 10. Background Agents

### `spawn_agent`
Spawn an AI agent to run a task in the background.

| Parameter | Type | Description |
|-----------|------|-------------|
| `type` | string | Agent type |
| `task` | string | Task description |

**Permission:** `once`
Returns an agent ID.

### `collect_agent` / `check_agent_status`
Wait for or poll a background agent.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Agent ID from `spawn_agent` |

**Permission:** `session`

### `list_agents`
List all active background agents with status.

**Permission:** `always`

### `spawn_sub_agent`
Spawn a discovery-aware recursive sub-agent. Selects the best model for the requested capability class.

| Parameter | Type | Description |
|-----------|------|-------------|
| `task` | string | Task description |
| `agent_type` | string | `reasoning`, `speed`, `coding`, `general` |
| `isolated` | bool | Run in isolated git worktree |
| `verify_with` | string | Secondary model ID for verification pass |

**Permission:** `once`
Depth-limited (max 4) via context key to prevent runaway recursion.

### `colony_debate`
Run the same prompt through multiple AI personas and synthesize the responses.

| Parameter | Type | Description |
|-----------|------|-------------|
| `topic` | string | Discussion topic |
| `perspectives` | int | Number of perspectives (default 3) |

**Permission:** `once`

### `run_pipeline`
Execute a named multi-step agentic pipeline synchronously.

| Parameter | Type | Description |
|-----------|------|-------------|
| `agent_type` | string | Agent/pipeline type |
| `task` | string | Input task |

**Permission:** `once`

---

## 11. Process Management

Managed long-running background processes (distinct from background agents).

### `start_managed_process`
Start a background process.

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | Shell command |
| `name` | string | Human-readable name |
| `workdir` | string | Working directory |

**Permission:** `once`

### `list_managed_processes`
List all managed background processes.

**Permission:** `always`

### `stop_managed_process`
Stop a managed process by ID or name.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Process ID or name |
| `force` | bool | SIGKILL instead of SIGTERM |

**Permission:** `once`

### `read_managed_process_output`
Read buffered stdout/stderr from a managed process.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Process ID or name |
| `lines` | int | Number of lines to read (default 50) |

**Permission:** `session`

---

## 12. Database

### `db_query`
Execute a SQL query on a local SQLite database.

| Parameter | Type | Description |
|-----------|------|-------------|
| `db_path` | string | Path to SQLite database |
| `query` | string | SQL query |
| `params` | array | Query parameters (for prepared statements) |

**Permission:** `session`

### `db_migrate`
Apply SQL migration files to a database.

| Parameter | Type | Description |
|-----------|------|-------------|
| `db_path` | string | Path to SQLite database |
| `migration_dir` | string | Directory containing .sql migration files |
| `direction` | string | `up` or `down` |

**Permission:** `once`

---

## 13. Security — Recon & Audit

> **Note:** Security tools require explicit tool category enablement via `/settings → Tool Groups`. All tools are for authorized testing, defensive use, and CTF contexts only.

### `nmap_scan`
Port scan using nmap.

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | Host or IP range |
| `ports` | string | Port range (e.g., `1-1000`, `80,443`) |
| `flags` | string | Extra nmap flags |

**Permission:** `once`

### `masscan_run`
High-speed port scan using masscan.

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | IP range in CIDR notation |
| `ports` | string | Ports or ranges |
| `rate` | int | Packets per second |

**Permission:** `once`

### `dns_enum`
DNS enumeration (subdomains, records).

| Parameter | Type | Description |
|-----------|------|-------------|
| `domain` | string | Target domain |
| `wordlist` | string | Wordlist file path |

**Permission:** `once`

### `arp_scan_run`
ARP scan to discover hosts on local network.

| Parameter | Type | Description |
|-----------|------|-------------|
| `interface` | string | Network interface |
| `range` | string | IP range |

**Permission:** `once`

### `traceroute_run`
Trace network path to a host.

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | Hostname or IP |
| `protocol` | string | `icmp`, `udp`, `tcp` |

**Permission:** `always`

### `nikto_scan`
Web server vulnerability scanner.

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | URL or host |
| `port` | int | Port (default 80) |

**Permission:** `once`

### `gobuster_scan`
Directory and file brute-forcing.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Target URL |
| `wordlist` | string | Wordlist file path |
| `mode` | string | `dir`, `dns`, `vhost` |

**Permission:** `once`

### `ffuf_run`
Web fuzzer for directories, parameters, and virtual hosts.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | URL with `FUZZ` placeholder |
| `wordlist` | string | Wordlist file path |
| `filter_code` | string | Filter HTTP codes (e.g., `404`) |

**Permission:** `once`

### `subfinder_run`
Passive subdomain enumeration.

| Parameter | Type | Description |
|-----------|------|-------------|
| `domain` | string | Target domain |

**Permission:** `once`

### `nuclei_scan`
Vulnerability scanning using Nuclei templates.

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | Target URL or file |
| `templates` | string | Template path or tag |

**Permission:** `once`

### `shodan_query`
Query Shodan for exposed services.

| Parameter | Type | Description |
|-----------|------|-------------|
| `query` | string | Shodan search query |
| `limit` | int | Number of results |

**Permission:** `once`
Requires `SHODAN_API_KEY` environment variable.

### `ssl_validator`
Check SSL/TLS certificate and configuration.

| Parameter | Type | Description |
|-----------|------|-------------|
| `host` | string | Hostname |
| `port` | int | Port (default 443) |

**Permission:** `always`

### `http_header_audit`
Analyze HTTP security headers.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Target URL |

**Permission:** `session`

### `wafw00f_run`
Detect Web Application Firewalls.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Target URL |

**Permission:** `once`

### `sqlmap_scan`
SQL injection testing.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Target URL |
| `data` | string | POST data |
| `flags` | string | Extra sqlmap flags |

**Permission:** `once`

### `packet_capture`
Capture network packets (requires root/CAP_NET_RAW).

| Parameter | Type | Description |
|-----------|------|-------------|
| `interface` | string | Network interface |
| `filter` | string | BPF filter expression |
| `count` | int | Number of packets |
| `output` | string | Output .pcap file |

**Permission:** `once`

### `wifi_analyzer`
Analyze nearby WiFi networks.

**Permission:** `once`

### `network_scan`
Generic network reachability scan.

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | Target host/range |
| `timeout` | int | Per-host timeout |

**Permission:** `once`

---

## 14. Security — Exploitation & Credentials

### `hydra_run`
Online password brute-forcing (authorized testing only).

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | Target host |
| `service` | string | Service (ssh, ftp, http, etc.) |
| `userlist` | string | Path to user list file |
| `passlist` | string | Path to password list file |

**Permission:** `once`

### `hashcat_run`
Offline hash cracking with GPU acceleration.

| Parameter | Type | Description |
|-----------|------|-------------|
| `hash` | string | Hash value or file |
| `mode` | int | Hashcat attack mode |
| `wordlist` | string | Wordlist file |

**Permission:** `once`

### `john_run`
John the Ripper hash cracking.

| Parameter | Type | Description |
|-----------|------|-------------|
| `hash_file` | string | File containing hashes |
| `wordlist` | string | Wordlist file |
| `format` | string | Hash format |

**Permission:** `once`

### `hash_identify`
Identify hash type from a sample.

| Parameter | Type | Description |
|-----------|------|-------------|
| `hash` | string | Hash string |

**Permission:** `always`

### `jwt_decode`
Decode and optionally verify a JWT token.

| Parameter | Type | Description |
|-----------|------|-------------|
| `token` | string | JWT string |
| `secret` | string | Optional signing secret for verification |

**Permission:** `always`

### `metasploit_rpc`
Interact with Metasploit Framework via RPC.

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | MSF console command |
| `host` | string | MSF RPC host |
| `port` | int | MSF RPC port |

**Permission:** `once`

### `cve_lookup`
Look up CVE details from NVD.

| Parameter | Type | Description |
|-----------|------|-------------|
| `cve_id` | string | CVE identifier (e.g., `CVE-2024-1234`) |

**Permission:** `always`

### `searchsploit_query`
Search Exploit-DB via searchsploit.

| Parameter | Type | Description |
|-----------|------|-------------|
| `query` | string | Search terms |

**Permission:** `always`

---

## 15. Security — Assessment Helpers

### `enum4linux_run`
Enumerate Windows/Samba information.

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | Target host |

**Permission:** `once`

### `smbmap_run`
Enumerate SMB shares.

| Parameter | Type | Description |
|-----------|------|-------------|
| `target` | string | Target host |
| `user` | string | Username |
| `password` | string | Password |

**Permission:** `once`

### `suid_check`
Find SUID/SGID binaries on the local system.

**Permission:** `always`

### `sudo_check`
List sudo privileges for the current user.

**Permission:** `always`

### `linpeas_run`
Run LinPEAS privilege escalation script.

**Permission:** `once`

### `strings_analyze`
Extract and analyze strings from a binary.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Binary file path |
| `min_length` | int | Minimum string length |

**Permission:** `session`

### `hexdump_file`
Hex dump a file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path |
| `length` | int | Bytes to dump |

**Permission:** `session`

### `netstat_analysis`
Analyze active network connections.

**Permission:** `always`

### `totp_generate`
Generate a TOTP code from a base32 secret.

| Parameter | Type | Description |
|-----------|------|-------------|
| `secret` | string | Base32 TOTP secret |

**Permission:** `session`

---

## 16. DevOps & Cloud

### `docker_manager`
Manage Docker containers, images, and volumes.

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `run`, `stop`, `rm`, `ps`, `images`, `pull` |
| `image` | string | Docker image |
| `name` | string | Container name |
| `flags` | string | Additional docker flags |

**Permission:** `once`

### `k8s_kubectl`
Execute kubectl commands against a Kubernetes cluster.

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | kubectl command and arguments |
| `namespace` | string | Kubernetes namespace |

**Permission:** `once`

### `aws_s3_sync`
Sync files to/from an S3 bucket.

| Parameter | Type | Description |
|-----------|------|-------------|
| `source` | string | Source path or S3 URI |
| `destination` | string | Destination path or S3 URI |
| `delete` | bool | Delete files not in source |

**Permission:** `once`

### `git_blame_analyze`
Analyze `git blame` output to identify code ownership and change history.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path |
| `lines` | string | Line range (e.g., `10-50`) |

**Permission:** `always`

### `ngrok_tunnel`
Create a public tunnel to a local port using ngrok.

| Parameter | Type | Description |
|-----------|------|-------------|
| `port` | int | Local port to expose |
| `protocol` | string | `http` or `tcp` |

**Permission:** `once`

### `ci_trigger`
Trigger a CI/CD pipeline via webhook or API.

| Parameter | Type | Description |
|-----------|------|-------------|
| `provider` | string | `github`, `gitlab`, `jenkins` |
| `repo` | string | Repository identifier |
| `branch` | string | Branch to build |
| `token` | string | API token |

**Permission:** `once`

---

## 17. Android / Termux — Device Control

> These tools require Termux with ADB wireless debugging enabled or MediaProjection capture configured. Most are specific to Android/Termux environments.

### `adb_screenshot`
Take a screenshot via ADB and return the image path.

**Permission:** `session`

### `adb_shell`
Execute a shell command via ADB.

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | ADB shell command |

**Permission:** `once`

### `screen_capture`
Capture the current screen using MediaProjection companion.

**Permission:** `session`

### `screenshot` / `screenrecord`
Take a screenshot or record the screen using Termux APIs.

**Permission:** `session`

### `capture_screen_hack`
Alternative screen capture using Android accessibility services.

**Permission:** `session`

### `ui_dump`
Dump UI hierarchy (accessibility tree) for the current screen.

**Permission:** `session`

### `device_info`
Get Android device information (model, OS version, specs).

**Permission:** `always`

### `context_state`
Get the current foreground app and UI context state.

**Permission:** `always`

### `kill_app` / `launch_app`
Force-stop or launch an Android application by package name.

| Parameter | Type | Description |
|-----------|------|-------------|
| `package` | string | Android package name |

**Permission:** `once`

### `app_catalog`
List all installed applications.

**Permission:** `always`

### `app_control` / `app_status`
Send control commands or query the status of an app.

**Permission:** `once`

### `manage_deps`
Install or update Termux packages.

| Parameter | Type | Description |
|-----------|------|-------------|
| `packages` | array | Package names |
| `action` | string | `install`, `upgrade`, `remove` |

**Permission:** `once`

### `termux_control`
Execute Termux API control commands (battery, location, etc.).

| Parameter | Type | Description |
|-----------|------|-------------|
| `command` | string | Termux API command |

**Permission:** `once`

### `save_state`
Snapshot the current Termux session state.

**Permission:** `session`

### `start_health_monitor`
Start a background health monitoring daemon for the device.

**Permission:** `once`

### `browser_scrape` / `browser_control`
Scrape or control the Android browser.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Target URL |
| `action` | string | `open`, `back`, `screenshot` |

**Permission:** `once`

---

## 18. Android / Termux — APIs & System

### `sensor_read`
Read Android sensor data (accelerometer, GPS, etc.).

| Parameter | Type | Description |
|-----------|------|-------------|
| `sensor` | string | Sensor type |

**Permission:** `session`

### `notification_send`
Send an Android notification.

| Parameter | Type | Description |
|-----------|------|-------------|
| `title` | string | Notification title |
| `message` | string | Notification body |

**Permission:** `once`

### `intent_broadcast`
Send an Android broadcast intent.

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | Intent action |
| `extras` | object | Extra key-value pairs |

**Permission:** `once`

### `logcat_dump`
Dump recent Android logcat entries.

| Parameter | Type | Description |
|-----------|------|-------------|
| `filter` | string | Logcat filter expression |
| `lines` | int | Number of lines |

**Permission:** `session`

### `clipboard_manager`
Get or set clipboard content.

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `get` or `set` |
| `content` | string | Content to set |

**Permission:** `once`

### `notification_listener`
Read recent system notifications.

**Permission:** `session`

### `accessibility_query`
Query the accessibility tree for UI elements.

| Parameter | Type | Description |
|-----------|------|-------------|
| `query` | string | Element description or selector |

**Permission:** `session`

### `apk_decompile`
Decompile an APK using apktool.

| Parameter | Type | Description |
|-----------|------|-------------|
| `apk_path` | string | Path to APK file |
| `output_dir` | string | Output directory |

**Permission:** `once`

### `sqlite_explorer`
Browse SQLite databases on the device (including app databases with root).

| Parameter | Type | Description |
|-----------|------|-------------|
| `db_path` | string | Path to SQLite database |
| `query` | string | SQL query |

**Permission:** `once`

### `termux_api_bridge`
Execute arbitrary Termux API commands.

| Parameter | Type | Description |
|-----------|------|-------------|
| `api_command` | string | Termux API command name |
| `args` | array | Command arguments |

**Permission:** `once`

---

## 19. Vision (Screen Capture & Analysis)

### `vision_install`
Install the MediaProjection companion app needed for screen capture.

**Permission:** `once`

### `adb_setup`
Run ADB diagnostics and assist with wireless ADB setup.

**Permission:** `once`

### `vision_screen`
Capture screen and analyze it with Grok Vision API.

| Parameter | Type | Description |
|-----------|------|-------------|
| `prompt` | string | What to analyze in the screenshot |

**Permission:** `session`

### `vision_capture_only`
Capture screen without analysis.

**Permission:** `session`

### `vision_file`
Analyze a local image file with Grok Vision API.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Image file path |
| `prompt` | string | Analysis prompt |

**Permission:** `session`

### `vision_ocr`
Extract text from an image (OCR).

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Image path or URL |

**Permission:** `session`

### `vision_find`
Find a UI element in a screenshot by description.

| Parameter | Type | Description |
|-----------|------|-------------|
| `description` | string | Element description |

**Permission:** `session`

### `vision_watch`
Continuously monitor the screen until a condition is met.

| Parameter | Type | Description |
|-----------|------|-------------|
| `condition` | string | Condition description |
| `interval` | int | Check interval in seconds |
| `timeout` | int | Overall timeout in seconds |

**Permission:** `once`

---

## 20. Worktree Management

### `create_worktree`
Create a git worktree for isolated development.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Worktree name |
| `branch` | string | Branch to check out (created if new) |

**Permission:** `once`

### `list_worktrees`
List all active git worktrees.

**Permission:** `always`

### `remove_worktree`
Remove a git worktree.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Worktree name or path |
| `force` | bool | Remove even with unmerged changes |

**Permission:** `once`

### `integrate_worktree`
Merge a worktree's branch back into the main branch and remove the worktree.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Worktree name |
| `strategy` | string | `merge` or `rebase` |

**Permission:** `once`

---

## 21. Media & Content

### `ffmpeg_pro`
Run ffmpeg with full parameter control for video/audio processing.

| Parameter | Type | Description |
|-----------|------|-------------|
| `input` | string | Input file path or URL |
| `output` | string | Output file path |
| `options` | string | ffmpeg option string |

**Permission:** `once`

### `audio_transcribe`
Transcribe audio to text using Whisper.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Audio file path |
| `language` | string | Language code |

**Permission:** `session`

### `tts_generate`
Convert text to speech.

| Parameter | Type | Description |
|-----------|------|-------------|
| `text` | string | Text to speak |
| `voice` | string | Voice ID |
| `output` | string | Output audio file |

**Permission:** `once`

### `image_ocr_batch`
Run OCR on multiple images.

| Parameter | Type | Description |
|-----------|------|-------------|
| `directory` | string | Directory containing images |
| `output_format` | string | `txt`, `json`, `csv` |

**Permission:** `session`

### `video_summarize`
Summarize a video's content using vision analysis.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Video file path |
| `interval` | int | Frame sampling interval in seconds |

**Permission:** `session`

### `meme_generator`
Generate a meme image with top and bottom text.

| Parameter | Type | Description |
|-----------|------|-------------|
| `template` | string | Meme template name |
| `top_text` | string | Top text |
| `bottom_text` | string | Bottom text |
| `output` | string | Output file path |

**Permission:** `once`

### `image_process`
Resize, convert, or filter images.

| Parameter | Type | Description |
|-----------|------|-------------|
| `input` | string | Input file |
| `output` | string | Output file |
| `operation` | string | `resize`, `convert`, `grayscale`, `rotate` |
| `params` | object | Operation parameters |

**Permission:** `once`

### `media_convert`
Convert media files between formats.

| Parameter | Type | Description |
|-----------|------|-------------|
| `input` | string | Input file |
| `output` | string | Output file |

**Permission:** `once`

---

## 22. Data Science & Knowledge

### `csv_pivot`
Perform pivot table analysis on CSV data.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | CSV file path |
| `rows` | string | Row dimension column |
| `cols` | string | Column dimension |
| `values` | string | Value column |
| `aggfunc` | string | `sum`, `mean`, `count` |

**Permission:** `session`

### `plot_generate`
Generate charts and plots from data.

| Parameter | Type | Description |
|-----------|------|-------------|
| `data` | array | Data points |
| `chart_type` | string | `line`, `bar`, `scatter`, `pie` |
| `output` | string | Output image path |
| `title` | string | Chart title |

**Permission:** `once`

### `arxiv_search`
Search arXiv for research papers.

| Parameter | Type | Description |
|-----------|------|-------------|
| `query` | string | Search query |
| `max_results` | int | Number of results |
| `category` | string | arXiv category (e.g., `cs.AI`) |

**Permission:** `always`

### `web_archive`
Fetch a URL from the Wayback Machine.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | URL to look up |
| `date` | string | Specific date (YYYYMMDD) |

**Permission:** `session`

### `whois_lookup`
Perform a WHOIS lookup for a domain or IP.

| Parameter | Type | Description |
|-----------|------|-------------|
| `query` | string | Domain or IP |

**Permission:** `always`

---

## 23. Personal & Productivity

### `calendar_manage`
Manage calendar events via Termux API or system calendar.

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `list`, `add`, `delete` |
| `title` | string | Event title |
| `start` | string | Start datetime (ISO 8601) |
| `end` | string | End datetime |

**Permission:** `once`

### `email_client`
Send, read, or list emails.

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `send`, `list`, `read` |
| `to` | string | Recipient address |
| `subject` | string | Email subject |
| `body` | string | Email body |

**Permission:** `once`

### `contact_sync`
Sync or query device contacts.

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `list`, `search`, `add` |
| `query` | string | Search query |

**Permission:** `once`

### `smart_home_api`
Control smart home devices via local API.

| Parameter | Type | Description |
|-----------|------|-------------|
| `device` | string | Device name or ID |
| `action` | string | `on`, `off`, `status`, `dim` |
| `value` | int | Value (e.g., brightness 0-100) |

**Permission:** `once`

---

## 24. Scheduling & Automation

### `schedule_task`
Schedule a prompt or tool sequence to run at a future time.

| Parameter | Type | Description |
|-----------|------|-------------|
| `cron` | string | Cron expression (e.g., `0 9 * * *`) |
| `task` | string | Prompt or task to execute |
| `name` | string | Human-readable task name |

**Permission:** `once`

### `list_scheduled_tasks`
List all scheduled tasks.

**Permission:** `always`

### `cancel_scheduled_task`
Cancel a scheduled task.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Task ID or name |

**Permission:** `once`

### `pause_resume_scheduled_task`
Pause or resume a scheduled task.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Task ID or name |
| `action` | string | `pause` or `resume` |

**Permission:** `once`

### `cron_manager`
Advanced cron job management (list system crontab, add entries).

**Permission:** `once`

---

## 25. Web Scraping (Scrapling)

Scrapling tools provide robust web scraping with stealth and dynamic page support.

### `scraping_fetch`
Fetch and parse a web page.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Target URL |
| `format` | string | `html`, `markdown`, `text` |

**Permission:** `session`

### `scraping_stealth`
Fetch a page with stealth mode (rotated user agents, randomized delays).

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Target URL |
| `proxy` | string | Optional proxy URL |

**Permission:** `once`

### `scraping_dynamic`
Render and scrape JavaScript-heavy pages.

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | string | Target URL |
| `wait` | int | Wait time in seconds after load |

**Permission:** `once`

### `scraping_extract`
Extract structured data from HTML.

| Parameter | Type | Description |
|-----------|------|-------------|
| `html` | string | HTML content |
| `selector` | string | CSS selector |
| `attribute` | string | Attribute to extract |

**Permission:** `session`

### `scraping_search`
Search the web and return scraped result content.

| Parameter | Type | Description |
|-----------|------|-------------|
| `query` | string | Search query |
| `engine` | string | `google`, `bing`, `duckduckgo` |

**Permission:** `session`

---

## 26. Jupyter Notebooks

### `jupyter`
Read, write, or execute Jupyter notebook cells.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Notebook file path |
| `action` | string | `read`, `execute`, `add_cell`, `clear_outputs` |
| `cell_index` | int | Cell index for targeted operations |
| `source` | string | Cell source code |

**Permission:** `once`

---

## 27. SENSE Memory

### `code2world`
Translate code artifacts to semantic knowledge entries stored in AgeMem.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Source file path |
| `notes` | string | Additional context |

**Permission:** `session`

### `record_engram`
Record an experience or insight as an engram in the SENSE memory store.

| Parameter | Type | Description |
|-----------|------|-------------|
| `content` | string | Experience or insight to record |
| `tags` | array | Categorization tags |

**Permission:** `always`

---

## 28. Self-Introspection

### `query_routing_stats`
Get statistics from the ARC Router (classification counts, budget distributions).

**Permission:** `always`

### `query_heuristics`
List MEL heuristics matching a topic.

| Parameter | Type | Description |
|-----------|------|-------------|
| `topic` | string | Search topic |

**Permission:** `always`

### `query_memory_state`
Get a summary of all active memory systems (AgeMem, Engrams, GoalLedger, CCI).

**Permission:** `always`

### `query_system_state`
Get a full system state snapshot: providers, active tools, mode, context usage.

**Permission:** `always`

---

## 29. Goal Ledger

Cross-session prospective memory for tracking multi-session goals.

### `add_goal`
Add a goal to the cross-session ledger.

| Parameter | Type | Description |
|-----------|------|-------------|
| `description` | string | Goal description |
| `priority` | string | `high`, `medium`, `low` |
| `deadline` | string | Optional deadline (ISO 8601) |

**Permission:** `always`

### `close_goal`
Mark a goal as achieved.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Goal ID |
| `outcome` | string | Description of outcome |

**Permission:** `always`

### `list_goals`
List all open goals.

**Permission:** `always`

---

## 30. Security Context

### `report_finding`
Report a security finding to the shared security assessment context. Findings are visible to all redteam sub-agents in the same session via `securityBriefFn`.

| Parameter | Type | Description |
|-----------|------|-------------|
| `title` | string | Finding title |
| `severity` | string | `critical`, `high`, `medium`, `low`, `info` |
| `description` | string | Detailed description |
| `evidence` | string | Supporting evidence |
| `remediation` | string | Recommended fix |

**Permission:** `always`

---

## 31. Agentic Pipeline

### `run_pipeline`
Execute a named multi-step agentic pipeline synchronously.

| Parameter | Type | Description |
|-----------|------|-------------|
| `agent_type` | string | Pipeline/agent type name |
| `task` | string | Input task description |

**Permission:** `once`

---

## 32. CCI Context Tools

These tools interact with the Codified Context Infrastructure (CCI) memory tiers. See [ARCHITECTURE.md](ARCHITECTURE.md) for the full CCI description.

### `mcp_context_list_subsystems`
List all Tier 3 cold memory subsystem documents.

**Permission:** `always`

### `mcp_context_get_subsystem`
Retrieve a Tier 3 subsystem specification. An empty result triggers gap detection → Plan mode.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Subsystem name |

**Permission:** `session`

### `mcp_context_suggest_specialist`
Keyword-score a task and recommend the best Tier 2 specialist domain.

| Parameter | Type | Description |
|-----------|------|-------------|
| `task` | string | Task description |

**Permission:** `always`

### `mcp_context_update_subsystem`
Write or update a Tier 3 living document.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Subsystem name |
| `content` | string | Markdown content |

**Permission:** `session`

### `mcp_context_list_specialists`
List all Tier 2 specialist domains.

**Permission:** `always`

### `mcp_context_status`
Return full CCI status (Tier 1 loaded, Tier 2 active domain, Tier 3 doc count, drift warnings).

**Permission:** `always`

---

## 33. Dynamic Tool Creation

Tools created with `create_tool` are persisted to `~/.config/gorkbot/dynamic_tools.json` and loaded on startup. They work as first-class tools with the same permission system, analytics tracking, and caching as built-in tools.

To permanently compile dynamic tools into the binary:

```bash
# Option 1: Run rebuild tool in the session
/run rebuild

# Option 2: Set env var for auto-rebuild on exit
GORKBOT_AUTO_REBUILD=1 ./gorkbot.sh

# Option 3: Manual build
go build -o bin/gorkbot ./cmd/gorkbot/
```

### Tool Template Syntax

```json
{
  "name": "count_words",
  "description": "Count words in a file",
  "category": "file",
  "command": "wc -w {{path}}",
  "parameters": {
    "path": "string"
  },
  "requires_permission": false,
  "default_permission": "always"
}
```

Placeholders `{{param_name}}` are replaced with shell-escaped parameter values at execution time.

---

## 34. RAG Memory Plugin

**Tool name:** `rag_memory`
**Package:** `plugins/python/rag_memory/`
**Introduced:** v3.5.0

The RAG Memory plugin provides persistent semantic vector memory backed by ChromaDB and the `all-MiniLM-L6-v2` sentence embedding model. It enables the AI to store and retrieve information across sessions using cosine similarity search.

Dependencies are auto-installed on first use via pip (`chromadb`, `sentence-transformers`). Storage is at `~/.config/gorkbot/rag_memory/` (configurable via `GORKBOT_CONFIG_DIR`).

### Actions

The `rag_memory` tool takes an `action` parameter selecting one of four operations:

---

### `store`

Embed and store content as an engram in the vector database.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `"store"` |
| `content` | string | yes | Text content to embed and store |
| `metadata` | object | no | Key-value metadata attached to the engram |

**Permission:** `session`

```
# Example: store a code review finding
rag_memory {action: "store", content: "Auth module uses MD5 for password hashing — critical security issue", metadata: {tag: "security", file: "pkg/auth/hash.go"}}
```

---

### `search`

Perform a cosine similarity search and return the top N matching engrams.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `action` | string | yes | — | `"search"` |
| `query` | string | yes | — | Natural language search query |
| `n_results` | int | no | 5 | Number of results to return |
| `min_score` | float | no | 0.0 | Minimum similarity score threshold (0.0–1.0) |

**Permission:** `always`

```
# Example: find previous security findings
rag_memory {action: "search", query: "security vulnerabilities in auth module", n_results: 3, min_score: 0.6}
```

Results are returned ranked by cosine similarity score (highest first). Each result includes the stored content, metadata, and similarity score.

---

### `stats`

Return collection statistics.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `"stats"` |

**Permission:** `always`

Returns: total engram count, collection name, storage path, and embedding model in use.

---

### `purge`

Delete all stored engrams from the collection.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | `"purge"` |

**Permission:** `once` — prompts before deletion.

Use with care — this operation is irreversible. Useful for clearing stale memory at the start of a new project.

---

### Capacity

The rolling memory window holds up to **10,000 engrams**. When the limit is reached, the oldest engrams are evicted to make room for new ones.

### Setup

No manual setup is required. On first invocation, the plugin:

1. Auto-installs `chromadb` and `sentence-transformers` via pip
2. Downloads the `all-MiniLM-L6-v2` embedding model (~90 MB)
3. Initializes a persistent ChromaDB collection at `~/.config/gorkbot/rag_memory/`

Subsequent invocations use the cached model and collection — no re-download occurs.
