# Tool Reference Guide

Complete documentation of all 28+ Gorkbot tools.

---

## Quick Reference

```
/tools              List all tools
/tool-info <name>   Get details about specific tool
```

---

## Tool Categories

| Category | Tools | Use Case |
|----------|-------|----------|
| **Bash** | shell | Execute commands |
| **File** | read_file, write_file, list_directory, search_files, grep_content, file_info, delete_file | File operations |
| **Git** | git_status, git_diff, git_log, git_commit, git_push, git_pull | Version control |
| **Web** | web_fetch, http_request, check_port, download_file, browser_scrape, browser_control | Web operations |
| **System** | list_processes, kill_process, env_var, system_info, disk_usage | System information |
| **Security** | nmap_scan, sqlmap_scan, nuclei_scan, totp_generate, etc. | Security testing |
| **Meta** | list_tools, tool_info, create_tool | Tool management |

---

## Bash Tools

### bash

**Purpose**: Execute shell commands

**Parameters**:
- `command` (string, required): Shell command to execute

**Example**:
```
> Run: echo "Hello, World!"
Tool: bash
Params: {"command": "echo 'Hello, World!'"}
Result: Hello, World!
```

**Risk Level**: CRITICAL
**Requires HITL**: YES
**Permissions**: once (default)

**Notes**:
- All output captured and returned
- Timeout: 300 seconds
- Works on Linux, macOS, Windows, Termux
- Shell commands must be valid for your OS

---

## File Tools

### read_file

**Purpose**: Read file contents

**Parameters**:
- `path` (string, required): Path to file

**Example**:
```
> Read /home/user/config.json

Tool: read_file
Params: {"path": "/home/user/config.json"}
Result: {file contents}
```

**Risk Level**: LOW
**Requires HITL**: NO
**Permissions**: always (default)

---

### write_file

**Purpose**: Write content to file

**Parameters**:
- `path` (string, required): File path
- `content` (string, required): Content to write

**Example**:
```
> Create a Python script at /tmp/hello.py that prints "Hello"

Tool: write_file
Params: {"path": "/tmp/hello.py", "content": "print('Hello')"}
```

**Risk Level**: MEDIUM
**Requires HITL**: YES (if sensitive path)
**Permissions**: session (default)

---

### list_directory

**Purpose**: List directory contents

**Parameters**:
- `path` (string, required): Directory path

**Example**:
```
> List files in current directory

Tool: list_directory
Params: {"path": "."}
Result: [file1.txt, file2.py, subdir/, ...]
```

**Risk Level**: LOW
**Permissions**: always

---

### search_files

**Purpose**: Search for files matching pattern

**Parameters**:
- `directory` (string, required): Search directory
- `pattern` (string, required): Glob pattern (e.g., "*.py")

**Example**:
```
> Find all Python files in the project

Tool: search_files
Params: {"directory": ".", "pattern": "*.py"}
Result: [file1.py, src/file2.py, ...]
```

---

### grep_content

**Purpose**: Search file contents

**Parameters**:
- `pattern` (string, required): Search regex
- `path` (string, required): File or directory

**Example**:
```
> Find all TODO comments in code

Tool: grep_content
Params: {"pattern": "TODO.*", "path": "./src/"}
```

---

### file_info

**Purpose**: Get file metadata

**Parameters**:
- `path` (string, required): File path

**Example**:
```
> Check file size and permissions

Tool: file_info
Params: {"path": "/home/user/data.csv"}
Result: {size: 1024000, mode: 0644, modified: 2026-03-20...}
```

---

### delete_file

**Purpose**: Delete file (DESTRUCTIVE)

**Parameters**:
- `path` (string, required): File to delete

**Risk Level**: CRITICAL
**Requires HITL**: YES
**Permissions**: once (must confirm each time)

**Example**:
```
> Delete the temporary file

Tool: delete_file
Params: {"path": "/tmp/temp.txt"}
```

**WARNING**: Cannot be undone! User must explicitly approve.

---

## Git Tools

### git_status

**Purpose**: Show git status

**Example**:
```
> What's the git status?

Tool: git_status
Result: On branch main
        Changes not staged for commit: modified: file.txt
```

**Risk Level**: LOW
**Permissions**: always

---

### git_diff

**Purpose**: Show changes between commits

**Parameters**:
- `path` (string, optional): Specific file to diff

**Example**:
```
> Show what changed in main.go

Tool: git_diff
Params: {"path": "main.go"}
```

---

### git_log

**Purpose**: Show commit history

**Parameters**:
- `limit` (integer, optional): Number of commits to show

**Example**:
```
> Show last 10 commits

Tool: git_log
Params: {"limit": 10}
```

---

### git_commit

**Purpose**: Commit changes

**Parameters**:
- `message` (string, required): Commit message

**Risk Level**: MEDIUM
**Requires HITL**: YES
**Permissions**: once

**Example**:
```
> Commit with message "Fix auth bug"

Tool: git_commit
Params: {"message": "fix(auth): resolve session timeout issue"}
```

---

### git_push

**Purpose**: Push commits to remote

**Risk Level**: HIGH
**Requires HITL**: YES
**Permissions**: once

**Example**:
```
> Push changes to main

Tool: git_push
```

---

### git_pull

**Purpose**: Pull changes from remote

**Risk Level**: MEDIUM
**Permissions**: session

---

## Web Tools

### web_fetch

**Purpose**: Fetch URL content

**Parameters**:
- `url` (string, required): URL to fetch
- `timeout` (integer, optional): Timeout in seconds

**Example**:
```
> Fetch the latest news from HN

Tool: web_fetch
Params: {"url": "https://news.ycombinator.com", "timeout": 10}
```

---

### http_request

**Purpose**: Make HTTP request

**Parameters**:
- `url` (string, required)
- `method` (string): GET, POST, PUT, DELETE
- `headers` (object): HTTP headers
- `body` (string): Request body

---

### check_port

**Purpose**: Check if port is open

**Parameters**:
- `host` (string): Host to check
- `port` (integer): Port number

---

### download_file

**Purpose**: Download file from URL

**Parameters**:
- `url` (string): File URL
- `output_path` (string): Where to save

---

### browser_scrape

**Purpose**: Scrape web content with JavaScript

---

### browser_control

**Purpose**: Control browser automation

---

## System Tools

### list_processes

**Purpose**: List running processes

**Example**:
```
> What processes are running?

Tool: list_processes
Result: [
    {pid: 1234, name: "gorkbot", cpu: 2.5%, mem: 150MB},
    {pid: 5678, name: "python", cpu: 0.1%, mem: 50MB},
    ...
]
```

**Risk Level**: LOW

---

### kill_process

**Purpose**: Terminate process

**Parameters**:
- `pid` (integer): Process ID

**Risk Level**: MEDIUM
**Permissions**: once

---

### env_var

**Purpose**: Get environment variable

**Parameters**:
- `name` (string): Variable name

---

### system_info

**Purpose**: Get system information

**Returns**:
- OS, architecture, CPU, RAM, disk, uptime

---

### disk_usage

**Purpose**: Check disk usage

**Parameters**:
- `path` (string, optional): Directory to check

---

## Security Tools

### nmap_scan

**Purpose**: Network port scanning

**Parameters**:
- `target` (string): IP or hostname
- `ports` (string, optional): Port range (e.g., "80,443,1000-2000")

**Risk Level**: HIGH
**Requires Authorization**: YES

---

### sqlmap_scan

**Purpose**: SQL injection testing

**Parameters**:
- `url` (string): Target URL

---

### nuclei_scan

**Purpose**: Comprehensive vulnerability scanning

---

### totp_generate

**Purpose**: Generate TOTP token

**Parameters**:
- `secret` (string): Base32 encoded secret

---

## Meta Tools

### list_tools

**Purpose**: List all available tools

**Example**:
```
> Show all available tools

Tool: list_tools
Result: [
    {name: "bash", description: "Execute shell command", category: "bash"},
    {name: "read_file", description: "Read file", category: "file"},
    ...
]
```

**Risk Level**: LOW
**Permissions**: always

---

### tool_info

**Purpose**: Get details about a tool

**Parameters**:
- `tool_name` (string): Name of tool

**Example**:
```
> Tell me about the git_commit tool

Tool: tool_info
Params: {"tool_name": "git_commit"}
Result: {
    name: "git_commit",
    description: "Commit staged changes",
    parameters: {...}
}
```

---

### create_tool

**Purpose**: Create custom tool dynamically

**Parameters**:
- `name` (string): Tool name
- `description` (string): What it does
- `command_template` (string): Command with {{param}} placeholders
- `parameters` (object): Parameter definitions

**Example**:
```
> Create a tool that pings a host

Tool: create_tool
Params: {
    "name": "ping_host",
    "description": "Ping a host to check connectivity",
    "command_template": "ping -c 4 {{host}}",
    "parameters": {"host": "string"}
}
```

**Result**: New tool available immediately!

---

## Best Practices

1. **Use absolute paths**: `/home/user/file.txt` not `./file.txt`
2. **Escape special characters**: Use quotes around strings with spaces
3. **Check risk level**: Some tools require approval
4. **Review permissions**: Grant minimally needed permissions
5. **Handle failures**: Tool failures are logged, not fatal
6. **Monitor execution**: Check tool results carefully
7. **Audit trail**: All executions logged to `gorkbot.db`

---

## Tool Permissions

Set via `/settings` in TUI or `app_state.json`:

```json
{
  "bash": "once",
  "delete_file": "never",
  "read_file": "always",
  "git_push": "session"
}
```

---

**For more help, use `/tool-info <name>` or see [FAQ.md](FAQ.md).**

