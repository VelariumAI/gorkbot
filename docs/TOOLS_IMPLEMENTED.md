# Gorkbot Tools — Complete Implementation Reference

**Version:** 3.5.1

This document provides a comprehensive catalog of all tools implemented in Gorkbot, organized by category. All tools are registered in `pkg/tools/registry.go → RegisterDefaultTools()` and executed through the unified permission, caching, and parallel-dispatch pipeline.

---

## Summary

| Category | Tools | Default State |
|----------|-------|--------------|
| Shell | 1 | Enabled |
| File Operations | 9 | Enabled |
| Hashline File Tools | 3 | Enabled |
| Git | 6 | Enabled |
| Web & Network | 5 | Enabled |
| System | 5 | Enabled |
| Task Management | 3 | Enabled |
| AI / Skills | 9 | Enabled |
| Meta / Introspection | 6 | Enabled |
| Background Agents | 3 | Enabled |
| Process Management | (via main.go) | Enabled |
| Database | 2 | Enabled |
| DevOps & Cloud | 6 | Enabled |
| Android / Termux — Device Control | 14 | Enabled |
| Android / Termux — APIs & System | 8 | Enabled |
| Vision | 9 | Enabled |
| Worktree Management | 4 | Enabled |
| Security — Recon & Audit | 6 | **Disabled** |
| Security — Pentesting Suite | 30+ | **Disabled** |
| Media & Content | 6 | Enabled |
| Data Science & Knowledge | 5 | Enabled |
| Personal & Productivity | 4 | Enabled |
| Scheduling & Automation | 5 | Enabled |
| Web Scraping (Scrapling) | 5 | Enabled |
| Jupyter Notebooks | 1 | Enabled |
| SENSE Memory | 2 | Enabled |
| Self-Introspection | 4 | Enabled |
| Goal Ledger | 3 | Enabled |
| Security Context | 1 | Enabled |
| Agentic Pipeline | 1 | Enabled |
| CCI Context Tools | (via pkg/cci) | Enabled |
| Dynamic Tool Creation | 5 | Enabled |
| Python Plugin Bridge | varies | Enabled |
| MCP Server Tools | varies | Enabled |

**Total built-in tools: 162+** (plus MCP and dynamically created tools)

---

## Category Details

### Shell

| Tool | Description | Permission |
|------|-------------|-----------|
| `bash` | Execute arbitrary shell commands with timeout and exit-code-aware success detection | `once` |

The `bash` tool sanitizes all parameters with `shellescape()` before execution. Accepts `command` (required), `timeout` (seconds, default 30), and `workdir` (optional). Exit codes are captured and reported; non-zero exits surface stderr in the result.

---

### File Operations

| Tool | Description | Permission |
|------|-------------|-----------|
| `read_file` | Read file contents (UTF-8 or base64) | `session` |
| `write_file` | Write content to file (create or overwrite; supports append) | `once` |
| `edit_file` | Replace a specific string in a file | `once` |
| `multi_edit_file` | Apply multiple find-and-replace edits atomically | `once` |
| `list_directory` | List directory contents with metadata | `session` |
| `search_files` | Find files by glob pattern (`find`-style) | `session` |
| `grep_content` | Search file contents by regex pattern | `session` |
| `file_info` | Detailed file/directory metadata (size, perms, owner, MIME) | `always` |
| `delete_file` | Delete file or directory (requires `recursive` for directories) | `once` |

---

### Hashline File Tools

Hash-validated file operations that prevent stale-content edits in multi-turn sessions.

| Tool | Description | Permission |
|------|-------------|-----------|
| `read_file_hashed` | Read file with per-line hash annotations | `session` |
| `edit_file_hashed` | Edit file with line-hash validation before modification | `once` |
| `ast_grep` | AST-aware code search using `ast-grep` patterns | `session` |

`edit_file_hashed` verifies that the line content matches the hash from a prior `read_file_hashed` before applying any modification, preventing edits to stale content in long multi-turn editing sessions.

---

### Git

| Tool | Description | Permission |
|------|-------------|-----------|
| `git_status` | Show working tree status | `always` |
| `git_diff` | Show staged and unstaged changes | `always` |
| `git_log` | Show commit history with options | `always` |
| `git_commit` | Create a commit with a message | `once` |
| `git_push` | Push to remote | `once` |
| `git_pull` | Pull from remote | `once` |

---

### Web & Network

| Tool | Description | Permission |
|------|-------------|-----------|
| `web_fetch` | Fetch a URL and return page content | `once` |
| `http_request` | Make an HTTP request (GET/POST/PUT/DELETE) with headers | `once` |
| `check_port` | Check if a TCP port is open | `once` |
| `download_file` | Download a file from a URL to disk | `once` |
| `x_pull` | Fetch content from X (Twitter) posts via xAI API | `once` |

---

### System

| Tool | Description | Permission |
|------|-------------|-----------|
| `list_processes` | List running processes with PID, CPU, memory | `session` |
| `kill_process` | Terminate a process by PID or name | `once` |
| `env_var` | Read, set, or list environment variables | `session` |
| `system_info` | OS, architecture, memory, CPU, uptime | `always` |
| `disk_usage` | Disk usage by path (df / du style) | `always` |

---

### Task Management

| Tool | Description | Permission |
|------|-------------|-----------|
| `todo_write` | Create or update a TODO list item | `always` |
| `todo_read` | Read the current TODO list | `always` |
| `complete` | Mark a TODO item as complete | `always` |

---

### AI / Skills

| Tool | Description | Permission |
|------|-------------|-----------|
| `consultation` | Consult the specialist AI on a specific question | `always` |
| `web_search` | Search the web using the configured search engine | `once` |
| `web_reader` | Fetch and extract readable content from a webpage | `once` |
| `docx_reader` | Read and extract text from a .docx file | `session` |
| `xlsx_reader` | Read and extract data from an .xlsx spreadsheet | `session` |
| `pdf_reader` | Read and extract text from a PDF | `session` |
| `pptx_reader` | Read and extract content from a .pptx file | `session` |
| `frontend_design` | Generate frontend UI designs (HTML/CSS/components) | `once` |
| `ai_image_generate` | Generate images via AI image generation API | `once` |

---

### Meta / Introspection

| Tool | Description | Permission |
|------|-------------|-----------|
| `create_tool` | Create a new hot-loaded tool (saved to dynamic_tools.json) | `once` |
| `modify_tool` | Modify an existing dynamic tool definition | `once` |
| `list_tools` | List all registered tools with categories and permissions | `always` |
| `tool_info` | Get detailed information about a specific tool | `always` |
| `audit_tool_call` | Audit a tool call: verify parameters and predict permission | `always` |
| `context_stats` | Get current context window and token usage stats | `always` |

---

### Background Agents

| Tool | Description | Permission |
|------|-------------|-----------|
| `spawn_agent` | Spawn an asynchronous background agent task | `once` |
| `collect_agent` | Collect results from a background agent by ID | `always` |
| `list_agents` | List all running and completed background agents | `always` |

---

### Database

| Tool | Description | Permission |
|------|-------------|-----------|
| `db_query` | Execute a SQLite query (SELECT) | `once` |
| `db_migrate` | Apply a schema migration to a SQLite database | `once` |

---

### DevOps & Cloud

| Tool | Description | Permission |
|------|-------------|-----------|
| `docker_manager` | Docker container/image management (build, run, stop, logs) | `once` |
| `k8s_kubectl` | Execute kubectl commands | `once` |
| `aws_s3_sync` | Sync files to/from S3 buckets via AWS CLI | `once` |
| `git_blame_analyze` | Run `git blame` with analysis output | `session` |
| `ngrok_tunnel` | Start/stop ngrok tunnels | `once` |
| `ci_trigger` | Trigger CI/CD pipeline runs (GitHub Actions, etc.) | `once` |

---

### Android / Termux — Device Control

| Tool | Description | Permission |
|------|-------------|-----------|
| `adb_screenshot` | Take a screenshot via ADB | `once` |
| `adb_shell` | Execute a command via ADB shell | `once` |
| `app_catalog` | List installed apps on device | `always` |
| `app_control` | Start, stop, force-stop apps | `once` |
| `app_status` | Check app status (running, version, permissions) | `always` |
| `screen_capture` | Capture screen via MediaProjection API | `once` |
| `screenshot` | Take a screenshot (Termux-native) | `once` |
| `screenrecord` | Record screen to file | `once` |
| `capture_screen_hack` | Alternative screen capture via /proc or framebuffer | `once` |
| `ui_dump` | Dump the UI hierarchy (accessibility tree) | `once` |
| `device_info` | Device hardware and OS details | `always` |
| `context_state` | Read Android context state (foreground app, etc.) | `always` |
| `kill_app` | Force-kill an app by package name | `once` |
| `launch_app` | Launch an app by package name or intent | `once` |

---

### Android / Termux — APIs & System

| Tool | Description | Permission |
|------|-------------|-----------|
| `sensor_read` | Read device sensor values (accelerometer, GPS, etc.) | `once` |
| `notification_send` | Send a local Android notification | `once` |
| `intent_broadcast` | Send an Android intent broadcast | `once` |
| `logcat_dump` | Read Android logcat output | `once` |
| `clipboard_manager` | Read or write Android clipboard content | `once` |
| `notification_listener` | Listen for incoming notifications | `once` |
| `accessibility_query` | Query UI elements via accessibility APIs | `once` |
| `apk_decompile` | Decompile an APK to inspect its code | `once` |
| `sqlite_explorer` | Browse SQLite databases (app data, system DBs) | `once` |
| `termux_api_bridge` | Call Termux:API endpoints directly | `once` |

---

### Vision (Screen Capture & Analysis)

| Tool | Description | Permission |
|------|-------------|-----------|
| `vision_install` | Install/verify the vision capture companion service | `once` |
| `adb_setup` | Run ADB connection diagnostics | `always` |
| `vision_screen` | Capture screen and analyze with Grok Vision API | `once` |
| `vision_capture_only` | Capture screen without analysis | `once` |
| `vision_file` | Analyze a local image file with Grok Vision | `once` |
| `vision_ocr` | Extract text from screen/image via OCR | `once` |
| `vision_find` | Find a specific element in the screen by description | `once` |
| `vision_watch` | Monitor screen for changes matching a condition | `once` |
| `vision_capture` | Full capture pipeline: auto-connect ADB → capture → analyze | `once` |

Vision uses `grok-2-vision-1212` via the xAI API. Images are sent as base64 data URIs. The capture pipeline auto-connects to ADB over WiFi (no USB cable or root required on Android).

---

### Worktree Management

| Tool | Description | Permission |
|------|-------------|-----------|
| `create_worktree` | Create a git worktree for isolated work | `once` |
| `list_worktrees` | List all active git worktrees | `always` |
| `remove_worktree` | Remove a git worktree | `once` |
| `integrate_worktree` | Merge a worktree's changes back to the main branch | `once` |

---

### Security — Recon & Audit (Category: `security` — disabled by default)

| Tool | Description |
|------|-------------|
| `nmap_scan` | Network scan with Nmap (port discovery, OS detection) |
| `packet_capture` | Capture network packets via libpcap/tcpdump |
| `wifi_analyzer` | Scan and analyze WiFi networks |
| `shodan_query` | Query Shodan for internet-exposed host data (`SHODAN_API_KEY` required) |
| `metasploit_rpc` | Interface with Metasploit Framework via RPC |
| `ssl_validator` | Validate SSL/TLS certificates and configurations |

---

### Security — Comprehensive Pentesting Suite (Category: `pentest` — disabled by default)

| Tool | Description |
|------|-------------|
| `masscan_run` | High-speed port scanner |
| `dns_enum` | DNS enumeration and zone transfer |
| `arp_scan` | ARP scan for local network host discovery |
| `traceroute_run` | Network path tracing |
| `nikto_scan` | Web server vulnerability scanner |
| `gobuster_scan` | Directory and file brute-forcing |
| `ffuf_run` | Web fuzzing with custom wordlists |
| `sqlmap_scan` | SQL injection detection and exploitation |
| `wafw00f_run` | Web Application Firewall detection |
| `http_header_audit` | Audit HTTP security headers |
| `jwt_decode` | Decode and analyze JWT tokens |
| `hydra_run` | Network login brute-forcer |
| `hashcat_run` | GPU-accelerated hash cracking |
| `john_run` | Password cracking with John the Ripper |
| `hash_identify` | Identify hash type from a sample |
| `searchsploit_query` | Search ExploitDB for known exploits |
| `cve_lookup` | Look up CVE details |
| `enum4linux_run` | Enumerate Windows/Samba hosts |
| `smbmap_run` | SMB share enumeration |
| `suid_check` | Find SUID binaries for privilege escalation |
| `sudo_check` | Check sudo misconfigurations |
| `linpeas_run` | Linux privilege escalation script runner |
| `strings_analyze` | Extract strings from binaries |
| `hexdump_file` | Hex dump file contents |
| `netstat_analysis` | Analyze active network connections |
| `subfinder_run` | Subdomain enumeration |
| `nuclei_scan` | Template-based vulnerability scanner |
| `totp_generate` | Generate TOTP codes for 2FA |
| `network_escape_proxy` | Network escape proxy for restricted environments |

> Enable security/pentest tools via `Ctrl+G → Tool Groups` or by setting categories in `app_state.json`. These tools are intended for authorized security assessments and ethical penetration testing only.

---

### Media & Content

| Tool | Description | Permission |
|------|-------------|-----------|
| `ffmpeg_pro` | Full FFmpeg command wrapper (transcode, extract, convert) | `once` |
| `audio_transcribe` | Transcribe audio files to text | `once` |
| `tts_generate` | Generate text-to-speech audio | `once` |
| `image_ocr_batch` | Batch OCR processing on multiple images | `once` |
| `video_summarize` | Extract and summarize video content | `once` |
| `meme_generator` | Generate meme images with caption | `once` |

---

### Data Science & Knowledge

| Tool | Description | Permission |
|------|-------------|-----------|
| `csv_pivot` | Pivot and analyze CSV data | `session` |
| `plot_generate` | Generate data visualizations (PNG/SVG) | `once` |
| `arxiv_search` | Search ArXiv for academic papers | `once` |
| `web_archive` | Retrieve archived versions of webpages (Wayback Machine) | `once` |
| `whois_lookup` | WHOIS domain registration lookup | `once` |

---

### Personal & Productivity

| Tool | Description | Permission |
|------|-------------|-----------|
| `calendar_manage` | Read and create calendar events | `once` |
| `email_client` | Send and read emails | `once` |
| `contact_sync` | Read and sync contacts | `once` |
| `smart_home_api` | Interface with smart home APIs | `once` |

---

### Scheduling & Automation

| Tool | Description | Permission |
|------|-------------|-----------|
| `schedule_task` | Schedule a one-time or recurring task (cron syntax) | `once` |
| `list_scheduled_tasks` | List all scheduled tasks with status | `always` |
| `cancel_scheduled_task` | Cancel a scheduled task | `once` |
| `pause_resume_scheduled_task` | Pause or resume a scheduled task | `once` |
| `define_command` | Create a user-defined slash command at runtime | `once` |

---

### Web Scraping (Scrapling)

Scrapling tools use the Python `scrapling` library for robust, stealthy web scraping with JavaScript rendering support.

| Tool | Description | Permission |
|------|-------------|-----------|
| `scrapling_fetch` | Fetch a webpage with bot-detection bypass | `once` |
| `scrapling_stealth` | Stealth fetch with fingerprint rotation | `once` |
| `scrapling_dynamic` | Fetch JavaScript-rendered pages (headless browser) | `once` |
| `scrapling_extract` | Extract structured data from a page by CSS/XPath selectors | `once` |
| `scrapling_search` | Search-and-extract from multiple pages | `once` |

---

### Jupyter Notebooks

| Tool | Description | Permission |
|------|-------------|-----------|
| `jupyter` | Execute Python code in a Jupyter kernel, read/write notebook cells | `once` |

---

### SENSE Memory

| Tool | Description | Permission |
|------|-------------|-----------|
| `code2world` | Encode conceptual context into SENSE memory (AgeMem engram) | `always` |
| `record_engram` | Record a specific engram with content and tags | `always` |

---

### Self-Introspection

| Tool | Description | Permission |
|------|-------------|-----------|
| `query_routing_stats` | Query ARC Router statistics and last routing decision | `always` |
| `query_heuristics` | Query MEL VectorStore for relevant heuristics | `always` |
| `query_memory_state` | Query current memory system state (AgeMem, GoalLedger) | `always` |
| `query_system_state` | Query full orchestrator and system state | `always` |

---

### Goal Ledger

| Tool | Description | Permission |
|------|-------------|-----------|
| `add_goal` | Add a new cross-session goal | `always` |
| `close_goal` | Mark a goal as completed or cancelled | `always` |
| `list_goals` | List goals filtered by status | `always` |

---

### Security Context

| Tool | Description | Permission |
|------|-------------|-----------|
| `report_finding` | Record a security finding to the shared assessment context | `always` |

---

### Agentic Pipeline

| Tool | Description | Permission |
|------|-------------|-----------|
| `run_pipeline` | Execute a multi-step coordinated agent pipeline | `once` |

---

### Dynamic Tool Creation

| Tool | Description | Permission |
|------|-------------|-----------|
| `create_tool` | Create a hot-loaded tool (persisted to dynamic_tools.json) | `once` |
| `modify_tool` | Modify a dynamic tool definition | `once` |
| `list_tools` | List all registered tools | `always` |
| `tool_info` | Get detailed info on a tool | `always` |
| `rebuild` | Recompile the Gorkbot binary with all current dynamic tools | `once` |

Dynamic tools are shell-command wrappers with parameter definitions. They are loaded at startup from `dynamic_tools.json` and immediately after creation within the same session. Use the `rebuild` tool to permanently compile them into the binary.

---

### Python Plugin Bridge

The Python plugin bridge (`plugins/python/`) allows Gorkbot to load Python-based tools. Plugins are auto-discovered by scanning `plugins/python/*/manifest.json` at startup. Dependencies are auto-installed via `pip`.

**Built-in Python plugins:**

| Plugin | Description |
|--------|-------------|
| `rag_memory` | Semantic RAG memory via ChromaDB + MiniLM-L6-v2 embeddings |
| *(user-added)* | Drop a `manifest.json` and `tool.py` in `plugins/python/<name>/` |

See `GETTING_STARTED.md` for Python plugin setup instructions.

---

### MCP Server Tools

Tools from MCP servers are registered with the prefix `mcp_<server-name>_<toolname>`. They are available after configuring `~/.config/gorkbot/mcp.json` and restarting.

Example tool names from common MCP servers:
- `mcp_filesystem_read_file`
- `mcp_filesystem_write_file`
- `mcp_github_create_pr`
- `mcp_github_list_issues`

All MCP tools go through the standard permission pipeline. View connected servers: `/mcp status`

---

## Tool System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Tool Request (from AI)                        │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   PermissionManager                              │
│  1. Category guard (disabled categories → reject)               │
│  2. Rule engine (glob patterns → allow/deny)                    │
│  3. Permission store (always/never/session/once)                 │
└────────────────────────────┬────────────────────────────────────┘
                             │ allowed
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Cache.Get()                                 │
│  Hit → return cached result immediately                          │
│  Miss → continue to execution                                   │
└────────────────────────────┬────────────────────────────────────┘
                             │ cache miss
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Dispatcher (parallel)                         │
│  Up to 4 concurrent goroutines per turn                         │
│  Results collected in-order; mutations invalidate cache         │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                  Tool.Execute(ctx, params)                       │
│  Timeout: 30s default (configurable)                            │
│  Shell tools: all params shellescape()d                         │
│  Analytics: call count, latency, success rate recorded          │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                 Result → ConversationHistory                     │
│  Native path: role="tool" message with tool_call_id             │
│  Text path: user message with [Tool result: ...] format         │
└─────────────────────────────────────────────────────────────────┘
```
