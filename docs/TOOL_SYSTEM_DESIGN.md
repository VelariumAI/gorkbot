> **This document is superseded.**
>
> See [ARCHITECTURE.md](ARCHITECTURE.md) for the current system design.

# Grokster Tool & A2A System Design

## 🎯 Overview

Grokster now features a comprehensive tool system with:
1. **Robust Tool Framework** - Extensible tool architecture
2. **Permission System** - Multi-level permission control with persistence
3. **Built-in Tools** - Bash, file operations, git, web, etc.
4. **A2A Communication** - Agent-to-Agent communication protocol
5. **TUI Integration** - Permission prompts and tool execution feedback

## 🏗️ Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     User (TUI)                           │
└────────────────────┬─────────────────────────────────────┘
                     │ Permission Prompts
                     │ Tool Execution Results
                     ▼
┌──────────────────────────────────────────────────────────┐
│                  Orchestrator                            │
│  ┌────────────┐         ┌─────────────┐                 │
│  │  Grok AI   │◄───────►│  Gemini AI  │                 │
│  └──────┬─────┘  A2A    └──────┬──────┘                 │
│         │      Channel          │                        │
│         │                       │                        │
│         └───────┬───────────────┘                        │
│                 │ Tool Requests                          │
│                 ▼                                        │
│         ┌───────────────┐                                │
│         │ Tool Registry │                                │
│         └───────┬───────┘                                │
│                 │                                        │
└─────────────────┼────────────────────────────────────────┘
                  │
      ┌───────────┼───────────┬──────────────┐
      ▼           ▼           ▼              ▼
┌──────────┐ ┌────────┐ ┌─────────┐ ┌──────────────┐
│ Bash Tool│ │ File   │ │ Git     │ │ Permission   │
│          │ │ Tools  │ │ Tools   │ │ Manager      │
└──────────┘ └────────┘ └─────────┘ └──────┬───────┘
                                            │
                                            ▼
                                    ┌───────────────┐
                                    │  Persistent   │
                                    │  Storage      │
                                    │ permissions.  │
                                    │    json       │
                                    └───────────────┘
```

## 📦 Package Structure

```
pkg/
├── tools/                  # Tool system
│   ├── tool.go            # Tool interface & types
│   ├── registry.go        # Tool registry
│   ├── permissions.go     # Permission manager
│   ├── bash.go            # Shell execution tools
│   ├── file.go            # File operation tools (TODO)
│   ├── git.go             # Git tools (TODO)
│   └── web.go             # Web tools (TODO)
│
├── a2a/                    # Agent-to-Agent communication
│   ├── protocol.go        # A2A message protocol
│   └── channel.go         # Communication channel
│
└── ai/                     # AI providers
    ├── interface.go       # AIProvider interface
    ├── grok.go            # Grok implementation
    └── gemini.go          # Gemini implementation
```

## 🔧 Tool System

### Tool Interface

Every tool implements the `Tool` interface:

```go
type Tool interface {
    Name() string              // Unique identifier
    Description() string       // Human-readable description
    Category() ToolCategory    // Categorization
    Parameters() json.RawMessage  // JSON schema for parameters
    Execute(ctx, params) (*ToolResult, error)  // Execute the tool
    RequiresPermission() bool  // Permission needed?
    DefaultPermission() PermissionLevel  // Default permission
}
```

### Built-in Tools

#### 1. Bash Tool (`bash`)
- **Category:** Shell
- **Permission:** `once` (ask each time by default)
- **Parameters:**
  - `command` (string, required) - Bash command to execute
  - `timeout` (number, optional) - Timeout in seconds (default: 30)
  - `workdir` (string, optional) - Working directory
- **Example:**
  ```json
  {
    "tool": "bash",
    "parameters": {
      "command": "ls -la",
      "timeout": 10
    }
  }
  ```

#### 2. Read File Tool (`read_file`)
- **Category:** File
- **Permission:** `session` (allowed for session after first approval)
- **Parameters:**
  - `path` (string, required) - File path to read
  - `encoding` (string, optional) - File encoding
- **Example:**
  ```json
  {
    "tool": "read_file",
    "parameters": {
      "path": "/path/to/file.txt"
    }
  }
  ```

#### 3. Write File Tool (`write_file`)
- **Category:** File
- **Permission:** `once` (ask each time)
- **Parameters:**
  - `path` (string, required) - File path to write
  - `content` (string, required) - Content to write
  - `append` (boolean, optional) - Append vs overwrite (default: false)
- **Example:**
  ```json
  {
    "tool": "write_file",
    "parameters": {
      "path": "/path/to/file.txt",
      "content": "Hello, world!",
      "append": false
    }
  }
  ```

## 🔐 Permission System

### Permission Levels

| Level | Description | Persistence | User Action |
|-------|-------------|-------------|-------------|
| **always** | Always allowed | Permanent | Set once, applies forever |
| **session** | Allowed for session | Current session only | Applies until app restarts |
| **once** | Ask each time | None | Prompt every execution |
| **never** | Always denied | Permanent | Blocked forever |

### Permission Flow

```
Tool Execution Request
         ↓
┌────────────────────┐
│ Check Persistent   │
│ Permissions        │
└────────┬───────────┘
         │
    ┌────┴─────┐
    │ Level?   │
    └┬────┬────┬───┬─┘
     │    │    │   │
  always session once never
     │    │    │   │
     ↓    ↓    ↓   ↓
   Allow  Check Ask Deny
          Session User
          Cache
```

### Permission Storage

Permissions are stored in:
```
~/.config/grokster/tool_permissions.json
```

Format:
```json
{
  "version": "1.0",
  "permissions": {
    "bash": "once",
    "read_file": "session",
    "write_file": "once",
    "git_commit": "always"
  }
}
```

### Managing Permissions

Via TUI commands:
```
/tools                    # List all tools and permissions
/tools allow bash         # Grant permanent permission
/tools session read_file  # Grant session permission
/tools deny write_file    # Permanently deny
/tools reset bash         # Reset to default
/tools reset-all          # Clear all permissions
```

## 🤝 Agent-to-Agent (A2A) Communication

### Message Types

1. **Query** - Request information/advice (expects response)
2. **Response** - Reply to a query
3. **Notification** - One-way message (no response expected)
4. **Tool Request** - Request tool execution from another agent

### Message Structure

```go
type Message struct {
    ID        string                 // Unique message ID
    Type      MessageType            // query, response, notification
    From      string                 // "grok" or "gemini"
    To        string                 // "grok" or "gemini"
    Content   string                 // Message content
    Context   map[string]interface{} // Additional context
    ReplyTo   string                 // ID of message being replied to
    Timestamp time.Time              // When message was sent
}
```

### Usage Example

```go
// Grok asks Gemini for architectural advice
a2a := NewChannel(grokProvider, geminiProvider)

response, err := a2a.SendQuery(
    ctx,
    "grok",      // from
    "gemini",    // to
    "What's the best way to structure this microservices architecture?",
    map[string]interface{}{
        "domain": "e-commerce",
        "scale": "medium",
    },
)

// Gemini's response is returned
fmt.Println(response)
```

### A2A in Orchestrator

The orchestrator can:
1. **Automatic Consultation** - Grok asks Gemini for advice on complex queries
2. **Collaborative Problem Solving** - Agents discuss approaches
3. **Verification** - One agent verifies the other's output
4. **Specialization** - Route specific questions to specialist agents

Example flow:
```
User: "Design a scalable microservices architecture"
  ↓
Grok receives query
  ↓
Grok detects: Complex architecture question
  ↓
Grok → Gemini (A2A): "What architectural patterns should we use?"
  ↓
Gemini responds with patterns
  ↓
Grok synthesizes final answer combining:
  - Gemini's architectural advice
  - Grok's implementation knowledge
  - Specific recommendations
  ↓
User receives comprehensive answer
```

## 🔌 Integration Points

### 1. TUI Integration

The TUI will:
- Display permission prompts when tools need approval
- Show tool execution progress
- Render tool results in the chat
- Provide `/tools` commands for permission management

### 2. Orchestrator Integration

The orchestrator will:
- Maintain A2A channel between Grok and Gemini
- Route tool requests through the registry
- Handle permission prompts (blocking until user responds)
- Log tool executions

### 3. AI Provider Integration

AI providers will:
- Receive tool definitions in their context
- Use tools in their responses
- Format tool requests according to schema
- Handle tool results in subsequent reasoning

## 📝 Tool Execution Flow

```
1. AI generates tool request
   {
     "tool": "bash",
     "parameters": {
       "command": "git status"
     }
   }

2. Orchestrator receives request
   ↓
3. Registry checks permission
   ↓
4. If permission needed:
   → TUI prompts user
   → User approves/denies
   ↓
5. Tool executes
   ↓
6. Result returned to AI
   {
     "success": true,
     "output": "On branch main...",
     "data": {
       "stdout": "...",
       "exit_code": 0
     }
   }

7. AI incorporates result
   ↓
8. Final response to user
```

## 🚀 Future Tools

Planned additions:

### Git Tools
- `git_status` - Check repository status
- `git_diff` - View changes
- `git_commit` - Create commits
- `git_push` - Push to remote
- `git_pull` - Pull from remote

### Web Tools
- `web_fetch` - Fetch URL content
- `web_search` - Search the web
- `api_call` - Make API requests

### System Tools
- `list_processes` - Show running processes
- `check_port` - Check if port is open
- `env_var` - Read/set environment variables

### Advanced Tools
- `code_analysis` - Analyze code structure
- `test_runner` - Run tests
- `docker` - Docker operations
- `database` - Database queries

## 🔒 Security Considerations

1. **Permission Persistence** - Users must explicitly grant "always" permission
2. **Secure Storage** - Permissions stored with 0600 file permissions
3. **Command Escaping** - All bash commands properly escaped
4. **Timeout Protection** - All tools have execution timeouts
5. **Audit Trail** - Tool executions can be logged
6. **Revocability** - Users can revoke permissions at any time

## 📊 Status

### Completed ✅
- [x] Tool interface & base types
- [x] Tool registry
- [x] Permission manager with persistence
- [x] Bash execution tool
- [x] File read/write tools
- [x] A2A communication protocol
- [x] A2A message channel

### In Progress 🚧
- [ ] TUI integration for permission prompts
- [ ] Orchestrator integration
- [ ] Git tools
- [ ] Web tools

### Planned 📋
- [ ] System tools
- [ ] Advanced tools (docker, testing, etc.)
- [ ] Tool usage analytics
- [ ] Tool documentation generation

## 🎯 Next Steps

1. **Integrate with TUI** - Add permission prompt UI
2. **Update Orchestrator** - Add tool registry and A2A channel
3. **Build More Tools** - Git, web, system tools
4. **Add Tool Parsing** - Parse tool requests from AI responses
5. **Test End-to-End** - Full workflow testing

---

The tool system is designed to be:
- **Extensible** - Easy to add new tools
- **Secure** - Multiple permission levels with persistence
- **User-Friendly** - Clear prompts and easy management
- **Powerful** - Full terminal access when permitted
- **Collaborative** - A2A enables agent cooperation
