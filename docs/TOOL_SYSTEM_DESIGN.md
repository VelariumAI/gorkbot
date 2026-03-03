# Gorkbot Tool System Design

**Version:** 3.5.1

This document describes the architectural design principles, data models, extension points, and design decisions behind Gorkbot's tool system.

---

## Table of Contents

1. [Design Goals](#1-design-goals)
2. [Core Abstractions](#2-core-abstractions)
3. [Execution Pipeline Layers](#3-execution-pipeline-layers)
4. [Permission System Design](#4-permission-system-design)
5. [Caching Strategy](#5-caching-strategy)
6. [Parallel Execution Model](#6-parallel-execution-model)
7. [Error Classification Design](#7-error-classification-design)
8. [Dynamic Tool Design](#8-dynamic-tool-design)
9. [MCP Integration Design](#9-mcp-integration-design)
10. [Category System](#10-category-system)
11. [A2A Gateway Integration](#11-a2a-gateway-integration)
12. [Extension Guide: Adding a New Tool](#12-extension-guide-adding-a-new-tool)

---

## 1. Design Goals

The Gorkbot tool system was designed with six core principles:

1. **Safety first** — no tool bypasses permission checks; shell inputs are always escaped; timeouts are always enforced
2. **Composability** — tools are orthogonal; combining simple tools is preferred to complex monolithic tools
3. **Transparency** — every tool call is visible to the user before execution; parameters are shown in the permission prompt
4. **Extensibility** — new tools can be added without modifying the core registry; dynamic tools can be created at runtime without recompiling
5. **Performance** — read-only results are cached; multiple tools execute concurrently; the parallel cap prevents resource exhaustion
6. **Observability** — every execution is recorded in SQLite analytics; execution traces are optionally written as JSONL; MEL learns from failures

---

## 2. Core Abstractions

### `Tool` Interface

The central abstraction. Every capability the AI can invoke implements this interface:

```go
type Tool interface {
    Name() string                                                  // snake_case identifier
    Description() string                                           // shown in prompts and /tools
    Category() ToolCategory                                        // for group enable/disable
    Parameters() []Parameter                                       // typed param schema
    RequiresPermission() bool                                      // false: always runs silently
    DefaultPermission() PermissionLevel                            // initial permission level
    OutputFormat() OutputFormat                                    // Text | JSON | Markdown
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
    IsReadOnly() bool                                              // eligible for caching
    IsMutation() bool                                             // invalidates cache on success
}
```

### `Parameter` Schema

```go
type Parameter struct {
    Name        string
    Type        ParameterType   // string | int | bool | array | object
    Description string
    Required    bool
    Default     interface{}
    Enum        []string        // optional allowed values
}
```

Parameters flow through `NormalizeParameters()` before execution, which coerces types and applies defaults. This ensures `Execute()` always receives the correct Go types regardless of how the AI serialized the call.

### `PermissionLevel`

```go
type PermissionLevel string

const (
    PermissionAlways  PermissionLevel = "always"
    PermissionSession PermissionLevel = "session"
    PermissionOnce    PermissionLevel = "once"
    PermissionNever   PermissionLevel = "never"
)
```

### `ToolCategory`

```go
type ToolCategory string
// Examples: "shell", "file", "git", "web", "system", "security", "pentest", ...
```

Categories are strings, not an enum, allowing new categories to be introduced without modifying the core type system.

---

## 3. Execution Pipeline Layers

The pipeline is implemented as a series of guards in `Registry.Execute()`. Each layer either passes through or short-circuits with an error.

```
Registry.Execute(ctx, name, params)
    │
    ├─ [1] Category Guard
    │       registry.IsCategoryEnabled(tool.Category())
    │       → false: return ErrCategoryDisabled
    │
    ├─ [2] Rule Engine
    │       ruleEngine.Evaluate(name, params)
    │       → RuleDeny:  return ErrRuleDenied
    │       → RuleAllow: skip [3], go to [4]
    │       → RulePass:  continue
    │
    ├─ [3] Permission Store
    │       permMgr.Get(name)
    │       → never:   return ErrPermissionNever
    │       → always:  go to [4]
    │       → session: go to [4]
    │       → once:    show prompt → denied: return ErrDenied
    │                              → granted: go to [4]
    │
    ├─ [4] Cache Lookup (read-only tools only)
    │       cache.Get(key)
    │       → hit:  return cached, nil
    │       → miss: continue
    │
    ├─ [5] Dispatcher
    │       dispatcher.Run(goroutine pool, cap=4)
    │       → tool.Execute(ctx, normalizedParams)
    │       → timeout enforced via context deadline
    │
    ├─ [6] Post-Execution
    │       cache.Set(key, result)         (read-only tools)
    │       cache.InvalidateRelated(...)   (mutation tools)
    │       analytics.Record(...)
    │       bifurcation.Observe(...)
    │
    └─ return result, err
```

This layered design means each concern is isolated. Adding a new layer (e.g., a rate limiter or quota enforcer) does not require changes to existing layers.

---

## 4. Permission System Design

### Design Decisions

**Why four levels instead of binary allow/deny?**
- `session` provides a middle ground: temporary approval that auto-revokes on exit, useful for batch workflows
- `once` is the safe default that preserves user oversight without permanent commitment
- `always` / `never` provide efficiency for tools with well-established trust

**Why are rules evaluated before the permission store?**
Rules are more specific than the stored level. A deny rule for `write_file "*.env"` should override even a stored `always` permission. Rules are the most fine-grained layer and must take precedence.

**Why is only always/never persisted?**
- `once` is the default and adds no value when stored
- `session` is explicitly ephemeral by design
- Storing only `always` and `never` keeps `tool_permissions.json` small and clean

**Path:** `~/.config/gorkbot/tool_permissions.json` — 0600 permissions enforced by `PermissionManager` on every write.

### PermissionManager Thread Safety

The permission manager uses a `sync.RWMutex`:
- Reads (Get, check) use read locks — multiple concurrent goroutines can check permissions simultaneously
- Writes (Set, persist) use exclusive locks — no other operation can proceed during a write

---

## 5. Caching Strategy

### Cache Design

`pkg/tools.Cache` is an in-memory LRU-with-TTL store:

```go
type Cache struct {
    entries    map[string]*CacheEntry
    mu         sync.RWMutex
    defaultTTL time.Duration
}

type CacheEntry struct {
    Value     string
    ExpiresAt time.Time
    Tags      []string   // for targeted invalidation
}
```

### Cache Key Generation

```go
func buildCacheKey(name string, params map[string]interface{}) string {
    // Sort params for deterministic ordering
    sorted := sortedParams(params)
    // Hash name + params JSON for compact, collision-resistant key
    return fmt.Sprintf("%s:%x", name, sha256(sorted))
}
```

### Mutation Invalidation

When a mutation tool succeeds, `cache.InvalidateRelated()` removes cache entries that reference the same resource. Resource matching uses the primary parameter (e.g., `path` for file tools):

```go
// write_file{path: "main.go"} invalidates:
// - read_file{path: "main.go"}
// - read_file_hashed{path: "main.go"}
// - file_info{path: "main.go"}
```

This prevents the AI from reading stale cached content after a write.

### Why Not Disk Cache?

The cache is intentionally in-memory only:
1. Tool results can be large and varied; disk caching adds I/O overhead
2. Session scope is natural — stale cache from a prior session would cause confusion
3. The MEL vector store handles cross-session learning; the cache handles within-session performance

---

## 6. Parallel Execution Model

### Dispatcher Design

```go
type Dispatcher struct {
    workerPool chan struct{}  // buffered channel as semaphore
}

func NewDispatcher(maxWorkers int) *Dispatcher {
    return &Dispatcher{
        workerPool: make(chan struct{}, maxWorkers),
    }
}
```

### Batch Execution

```go
func (d *Dispatcher) Batch(ctx context.Context, reg *Registry, calls []ToolCall) []ToolResult {
    results := make([]ToolResult, len(calls))
    var wg sync.WaitGroup

    for i, call := range calls {
        wg.Add(1)
        go func(i int, call ToolCall) {
            defer wg.Done()

            // Acquire semaphore slot
            d.workerPool <- struct{}{}
            defer func() { <-d.workerPool }()

            result, err := reg.Execute(ctx, call.Name, call.Args)
            results[i] = ToolResult{Value: result, Error: err}
        }(i, call)
    }

    wg.Wait()
    return results  // In input order, not completion order
}
```

### Why Cap at 4?

- Android/Termux devices (the primary deployment target) have limited CPU cores and memory
- Too many concurrent shell executions create resource contention and noisy results
- 4 workers provides meaningful parallelism for typical AI batch sizes (2-6 tools)
- The cap is not configurable at runtime to prevent accidental denial-of-service

---

## 7. Error Classification Design

`pkg/tools/error_recovery.go` implements a taxonomy of tool errors with structured recovery actions.

### Error Codes

| Code | Meaning | Default Recovery |
|------|---------|-----------------|
| `PERMISSION_DENIED` | Category/rule/store blocked the call | `AWAIT_USER` — user must change permission |
| `TOOL_NOT_FOUND` | Tool name not in registry | `SKIP` — AI should not retry |
| `TIMEOUT` | Execution exceeded deadline | `RETRY` — retry once with longer timeout |
| `SHELL_ERROR` | Non-zero exit from bash/shell tool | `FIX_PARAMS` — AI should adjust command |
| `NETWORK_ERROR` | HTTP or socket failure | `RETRY` — transient; retry after brief wait |
| `FILE_NOT_FOUND` | Path does not exist | `FIX_PARAMS` — AI should fix the path |
| `INVALID_PARAMS` | Parameter validation failure | `FIX_PARAMS` — AI should correct params |
| `CATEGORY_DISABLED` | Tool's category is off | `AWAIT_USER` — user must enable category |

### Recovery Action Contract

Recovery actions are **advisory** — the orchestrator may choose to ignore them. They are primarily used by MEL's `BifurcationAnalyzer` to understand why a tool failed and what change in parameters would likely make it succeed.

---

## 8. Dynamic Tool Design

Dynamic tools are a first-class mechanism for extending the tool set without recompiling.

### Design Principle

A dynamic tool is a **parameterized shell command template** with a typed schema. This covers the most common use case (automating CLI tools) without requiring Go code or a plugin system.

### Template Engine

```
command: "wc -w {{path}}"
params:  {path: "/home/user/file.txt"}

→ shellescape("path", "/home/user/file.txt") → "'/home/user/file.txt'"
→ replace "{{path}}" → "wc -w '/home/user/file.txt'"
→ execute via bash
```

All template substitutions are shell-escaped. The template engine does not support nested templates, conditionals, or loops — intentionally simple.

### Lifecycle

```
create_tool invoked
    │
    ├── validate definition
    ├── append to dynamic_tools.json
    ├── registry.RegisterOrReplace(wrappedTool)
    └── available immediately for next AI turn

session exit (with GORKBOT_AUTO_REBUILD=1)
    │
    └── os.Exec("go build -o bin/gorkbot ./cmd/gorkbot/")
        └── binary now contains the tool permanently
```

---

## 9. MCP Integration Design

### Protocol

MCP (Model Context Protocol) uses JSON-RPC 2.0 over stdin/stdout. Gorkbot implements the client side (`pkg/mcp/client.go`) for the `stdio` transport.

### Message Flow

```
Gorkbot → subprocess stdin:
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}

← subprocess stdout:
{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}}}}

Gorkbot → subprocess stdin:
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}

← subprocess stdout:
{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"read_file","description":"...","inputSchema":{...}}]}}
```

Tool calls follow the same pattern with `tools/call` method.

### Error Handling

MCP errors (JSON-RPC error objects) are mapped to Gorkbot `ErrCodeNetworkError`. If an MCP server subprocess dies, its tools are removed from the registry and subsequent calls return `ErrCodeToolNotFound`.

### Tool Naming Convention

The `mcp_<server>_<toolname>` prefix convention prevents name collisions and makes the source of each tool clear in `/tools` output and the permission prompt.

---

## 10. Category System

### Design

Categories are plain strings with no compile-time enforcement. This allows:
- New categories to be added without modifying the core type
- Plugin tools to define their own categories
- MCP tools to use category names from their server manifests

### Default High-Risk Categories

`security` and `pentest` are **disabled by default** because:
1. They contain tools that make network requests to external hosts or execute potentially harmful operations
2. Enabling them should be an explicit, conscious decision
3. The AI should not be able to call `nmap_scan` or `hydra_run` without the user deliberately opting in

### Category State Persistence

Category state persists to `app_state.json` as a list of **disabled** categories (not enabled ones). This means new tools added in future versions are enabled by default unless they are added to a default-disabled category.

---

## 11. A2A Gateway Integration

The A2A (Agent-to-Agent) HTTP gateway (`pkg/a2a/`) exposes Gorkbot's orchestrator as an HTTP service that other agents can call.

### Endpoint

```
POST http://127.0.0.1:18890/execute
Content-Type: application/json

{
    "task": "Summarize the function in main.go:42",
    "tools": ["read_file", "grep_content"],
    "timeout": 30
}
```

### Tool Access

A2A task executions go through the same Registry.Execute() pipeline as normal tool calls. The `tools` field in the request acts as an allow-list (`--allow-tools` equivalent). All permission and safety checks apply.

### Sub-Agent Tool

The `spawn_sub_agent` tool (registered from `cmd/gorkbot/main.go`) uses the discovery manager to select the best model for the task, creates an isolated context (optionally in a git worktree), and delegates the task to a sub-orchestrator. Results are collected asynchronously via `collect_agent`.

---

## 12. Extension Guide: Adding a New Tool

### Step 1: Create the Tool File

Create `pkg/tools/mytool.go`:

```go
package tools

import (
    "context"
    "fmt"
)

// MyTool implements Tool
type MyTool struct{}

func NewMyTool() *MyTool { return &MyTool{} }

func (t *MyTool) Name() string        { return "my_tool" }
func (t *MyTool) Description() string { return "Does something useful" }
func (t *MyTool) Category() ToolCategory { return "system" }

func (t *MyTool) Parameters() []Parameter {
    return []Parameter{
        {
            Name:        "input",
            Type:        ParameterTypeString,
            Description: "The input string",
            Required:    true,
        },
        {
            Name:        "count",
            Type:        ParameterTypeInt,
            Description: "How many times to repeat",
            Required:    false,
            Default:     1,
        },
    }
}

func (t *MyTool) RequiresPermission() bool    { return true }
func (t *MyTool) DefaultPermission() PermissionLevel { return PermissionOnce }
func (t *MyTool) OutputFormat() OutputFormat  { return OutputFormatText }
func (t *MyTool) IsReadOnly() bool            { return true }   // eligible for caching
func (t *MyTool) IsMutation() bool            { return false }

func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    input, _ := params["input"].(string)
    count, _ := params["count"].(int)

    if input == "" {
        return "", fmt.Errorf("input is required")
    }
    if count <= 0 {
        count = 1
    }

    result := ""
    for i := 0; i < count; i++ {
        result += input + "\n"
    }
    return result, nil
}
```

### Step 2: Register the Tool

In `pkg/tools/registry.go`, add to the `RegisterDefaultTools()` tools slice:

```go
// Step 2: Register
// ... in the appropriate category section:
NewMyTool(),
```

### Step 3: Build and Test

```bash
go build -o bin/gorkbot ./cmd/gorkbot/
./bin/gorkbot -p "Use my_tool with input=hello and count=3"
```

### Step 4: Verify Registration

```
/tools
# Look for 'my_tool' in the System category

tool_info {"name": "my_tool"}
# Should show full parameter schema and description
```

### Guidelines

- **Names:** `snake_case`, globally unique, descriptive
- **Descriptions:** Start with a verb; be specific about what the tool does and does not do
- **Parameters:** Provide clear descriptions and sensible defaults; mark truly required params as `Required: true`
- **IsReadOnly/IsMutation:** Set these correctly — they affect caching and cache invalidation
- **Error messages:** Return errors that help the AI fix its parameter choices (e.g., `"path does not exist: /foo"` not `"error"`)
- **Shell tools:** Always use `shellescape()` for any shell parameter; never interpolate user input directly into shell strings
- **Timeouts:** For shell or network tools, respect the context deadline:
  ```go
  cmd := exec.CommandContext(ctx, "mycommand", shellescape(arg))
  ```

### Storage Paths

If your tool needs to store persistent data, use the configured data directory:

```go
type MyTool struct {
    dataDir string  // injected via registry.SetConfigDir
}
// Access via: filepath.Join(t.dataDir, "mytool_data.json")
```

Wire the data directory via a setter on the Registry if needed, following the pattern of `SetScheduler()`, `SetGoalLedger()`, etc.
