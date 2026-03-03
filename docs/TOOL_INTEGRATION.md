# Gorkbot Tool System Integration

**Version:** 3.5.1

This document describes how the tool system is wired into the Gorkbot orchestration engine — initialization, registration, parallel dispatch, permission enforcement, caching, error recovery, and the full call flow from AI request to result.

---

## Table of Contents

1. [Initialization & Registration](#1-initialization--registration)
2. [Tool Interface](#2-tool-interface)
3. [Registry Architecture](#3-registry-architecture)
4. [Permission Pipeline](#4-permission-pipeline)
5. [Caching](#5-caching)
6. [Parallel Dispatch](#6-parallel-dispatch)
7. [Error Recovery & Classification](#7-error-recovery--classification)
8. [Analytics & Tracing](#8-analytics--tracing)
9. [Dynamic Tools](#9-dynamic-tools)
10. [MCP Tool Integration](#10-mcp-tool-integration)
11. [Python Plugin Tools](#11-python-plugin-tools)
12. [Tool Call Flow: End-to-End](#12-tool-call-flow-end-to-end)

---

## 1. Initialization & Registration

Tool system initialization happens in `cmd/gorkbot/main.go` in this order:

```go
// 1. Create permission manager
permMgr := tools.NewPermissionManager(configDir)
permMgr.Load()   // Load persisted always/never decisions from tool_permissions.json

// 2. Create registry
registry := tools.NewRegistry(permMgr)
registry.SetConfigDir(configDir)

// 3. Register all 162+ built-in tools
tools.RegisterDefaultTools(registry)

// 4. Wire dependencies into registry
registry.SetAIProvider(orch.Primary)           // consultation tool
registry.SetScheduler(sched)                   // schedule_task tool
registry.SetUserCmdLoader(cmdLoader)           // define_command tool
registry.SetGoalLedger(orch.Memory.GoalLedger) // goal ledger tools
registry.SetPipelineRunner(orch.RunPipeline)   // run_pipeline tool
registry.SetContextStatsReporter(orch)         // context_stats tool

// 5. Load dynamic tools (hot-loaded from dynamic_tools.json)
if err := registry.LoadDynamicTools(); err != nil {
    logger.Warn("dynamic tools load failed", "err", err)
}

// 6. Wire analytics
analytics := tools.NewAnalytics(configDir)
registry.SetAnalytics(analytics)

// 7. Wire registry into orchestrator
orch.Registry = registry
```

### Category State Restoration

Disabled tool categories are restored from `app_state.json` after registration:

```go
for _, cat := range appState.DisabledCategories {
    registry.SetCategoryEnabled(tools.ToolCategory(cat), false)
}
```

---

## 2. Tool Interface

Every tool implements the `Tool` interface (`pkg/tools/interface.go`):

```go
type Tool interface {
    Name() string
    Description() string
    Category() ToolCategory
    Parameters() []Parameter
    RequiresPermission() bool
    DefaultPermission() PermissionLevel
    OutputFormat() OutputFormat
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
    IsReadOnly() bool
    IsMutation() bool
}
```

| Method | Description |
|--------|-------------|
| `Name()` | Unique tool identifier (snake_case) |
| `Description()` | Human-readable description shown in permission prompts and `/tools` |
| `Category()` | `ToolCategory` string for grouping and enabling/disabling |
| `Parameters()` | Typed parameter definitions with names, descriptions, required flag, defaults |
| `RequiresPermission()` | Whether the tool must go through the permission manager |
| `DefaultPermission()` | Starting permission level before user sets it |
| `OutputFormat()` | `Text`, `JSON`, or `Markdown` — controls how results are rendered |
| `Execute()` | The actual implementation — receives resolved params, returns (result, error) |
| `IsReadOnly()` | True for tools that only read state (used for caching eligibility) |
| `IsMutation()` | True for tools that modify state (triggers cache invalidation) |

### Parameter Types

```go
type Parameter struct {
    Name        string
    Type        ParameterType  // "string", "int", "bool", "array", "object"
    Description string
    Required    bool
    Default     interface{}
}
```

Parameters are normalized via `NormalizeParameters()` before execution, which converts string representations of booleans and integers to their native types.

---

## 3. Registry Architecture

`pkg/tools.Registry` manages the complete tool set:

```go
type Registry struct {
    tools              map[string]Tool
    permissionMgr      *PermissionManager
    ruleEngine         *RuleEngine
    analytics          *Analytics
    disabledCategories map[ToolCategory]bool

    // Injected dependencies
    aiProvider       interface{}
    consultantProvider interface{}
    scheduler        *scheduler.Scheduler
    goalLedger       GoalLedgerAccessor
    // ... other dependencies
}
```

### Key Registry Methods

| Method | Description |
|--------|-------------|
| `Register(tool)` | Add a tool (returns error if name conflicts) |
| `RegisterOrReplace(tool)` | Add or replace (used for dynamic and hot-reloaded tools) |
| `Get(name)` | Retrieve a tool by name |
| `List()` | Return all registered tools |
| `Execute(ctx, name, params)` | Full execution path including permission, cache, dispatch |
| `SetCategoryEnabled(cat, bool)` | Enable or disable a category |
| `IsCategoryEnabled(cat)` | Check category state |
| `Categories()` | Return all unique category names |

### Tool Execution via Registry

```go
result, err := registry.Execute(ctx, "read_file", map[string]interface{}{
    "path": "/path/to/file",
})
```

`Execute()` runs the full pipeline: category guard → rule engine → permission store → cache lookup → dispatcher → tool.Execute() → cache store → analytics record.

---

## 4. Permission Pipeline

See `docs/PERMISSIONS_GUIDE.md` for the full permission system documentation. In the context of tool integration:

### Category Guard

```go
func (r *Registry) Execute(ctx, name, params) (string, error) {
    tool := r.tools[name]
    cat := tool.Category()
    if !r.IsCategoryEnabled(cat) {
        return "", fmt.Errorf("tool category %q is disabled", cat)
    }
    // ...
}
```

### Rule Engine

```go
decision := r.ruleEngine.Evaluate(name, params)
if decision == RuleDeny {
    return "", fmt.Errorf("rule denied tool %q", name)
}
if decision == RuleAllow {
    // skip permission store
    goto execute
}
```

### Permission Store

```go
level := r.permissionMgr.Get(name)
switch level {
case PermissionNever:
    return "", fmt.Errorf("tool %q is blocked (never)", name)
case PermissionAlways, PermissionSession:
    goto execute
case PermissionOnce:
    // show permission prompt; wait for user decision
    granted := r.showPermissionPrompt(tool, params)
    if !granted {
        return "", fmt.Errorf("tool %q denied by user", name)
    }
}
```

---

## 5. Caching

`pkg/tools.Cache` is an in-memory TTL cache that memoizes results of read-only tool calls.

### Cache Behavior

- **Eligible tools:** `tool.IsReadOnly() == true` — e.g., `read_file`, `git_status`, `system_info`
- **TTL:** Configurable per tool category; default 60 seconds for file reads, 5 seconds for system state
- **Mutation invalidation:** When a mutation tool (e.g., `write_file` with `path=foo.go`) executes successfully, the cache entry for `read_file{path=foo.go}` is invalidated
- **Cache key:** Composed from tool name + parameter fingerprint (deterministic hash)

### Cache Statistics

The tool analytics include cache hit/miss rates viewable via `/tools stats`.

---

## 6. Parallel Dispatch

`pkg/tools.Dispatcher` manages concurrent tool execution within a single AI turn.

```go
type Dispatcher struct {
    maxWorkers int           // default: 4
    workerPool chan struct{}  // semaphore
}
```

When the AI requests multiple tools in a single response (a tool batch), the dispatcher:
1. Creates a goroutine for each tool in the batch
2. Limits concurrent execution to `maxWorkers` (4)
3. Uses a `sync.WaitGroup` to wait for all tools in the batch
4. Collects results in input order (not completion order)
5. Mutation invalidation and analytics recording happen on the goroutine that completed the tool

This allows the AI to parallelize reads and other non-conflicting tool calls without user-visible latency stacking.

---

## 7. Error Recovery & Classification

`pkg/tools.ErrorRecovery` (`pkg/tools/error_recovery.go`) classifies tool errors and provides structured recovery guidance:

```go
type ErrorCode string

const (
    ErrCodePermissionDenied ErrorCode = "PERMISSION_DENIED"
    ErrCodeToolNotFound     ErrorCode = "TOOL_NOT_FOUND"
    ErrCodeTimeout          ErrorCode = "TIMEOUT"
    ErrCodeShellError       ErrorCode = "SHELL_ERROR"
    ErrCodeNetworkError     ErrorCode = "NETWORK_ERROR"
    ErrCodeFileNotFound     ErrorCode = "FILE_NOT_FOUND"
    ErrCodeInvalidParams    ErrorCode = "INVALID_PARAMS"
    ErrCodeCategoryDisabled ErrorCode = "CATEGORY_DISABLED"
)

type RecoveryAction string

const (
    RecoveryRetry     RecoveryAction = "RETRY"
    RecoveryFix       RecoveryAction = "FIX_PARAMS"
    RecoveryEscalate  RecoveryAction = "ESCALATE"
    RecoverySkip      RecoveryAction = "SKIP"
    RecoveryUserInput RecoveryAction = "AWAIT_USER"
)
```

`ClassifyError()` maps raw errors to `ErrorCode` + `RecoveryAction`. `EnrichResult()` wraps results with structured metadata for MEL bifurcation analysis.

---

## 8. Analytics & Tracing

### Tool Analytics

`pkg/tools.Analytics` records every tool execution in SQLite:

```go
type CallRecord struct {
    ToolName   string
    CalledAt   time.Time
    DurationMs int64
    Success    bool
    Error      string  // empty on success
    ParamHash  string  // for deduplication
}
```

Accessible via:
- `/tools stats` — summary dashboard in TUI
- The Analytics tab (`Ctrl+A`)
- SQL queries on `~/.config/gorkbot/analytics.db`

### Execution Traces

When `--trace` is enabled, every tool call produces a JSONL trace entry:

```json
{"type":"tool_call","tool":"read_file","params":{"path":"main.go"},"timestamp":"2026-03-01T10:30:06Z"}
{"type":"tool_result","tool":"read_file","success":true,"duration_ms":12,"cache_hit":false,"timestamp":"2026-03-01T10:30:06Z"}
```

Traces are written to `~/.config/gorkbot/traces/<session-timestamp>.jsonl`.

### MEL Integration

After every tool execution, the result (success or failure with parameters) is fed to the MEL bifurcation analyzer:

```go
// On success:
orch.Intel.BifurcationAnalyzer.ObserveSuccess(toolName, params, result)

// On failure:
orch.Intel.BifurcationAnalyzer.ObserveFailed(toolName, params, err)
```

This allows MEL to automatically generate heuristics from recurring failure patterns and inject them into future system prompts.

---

## 9. Dynamic Tools

Dynamic tools are shell-command wrappers created at runtime via the `create_tool` tool or by directly editing `dynamic_tools.json`.

### Dynamic Tool Structure

```json
{
  "name": "count_words",
  "description": "Count words in a file",
  "category": "file",
  "command": "wc -w {{path}}",
  "parameters": {
    "path": {
      "type": "string",
      "description": "File path",
      "required": true
    }
  },
  "requires_permission": false,
  "default_permission": "always",
  "created_at": "2026-03-01T10:00:00Z"
}
```

Template variables (`{{path}}`, `{{query}}`, etc.) are replaced with the resolved parameter values. All substituted values are shell-escaped before execution.

### Registration Flow

When `create_tool` executes:
1. Validates the tool definition
2. Appends to `dynamic_tools.json`
3. Calls `registry.RegisterOrReplace()` immediately (no restart required)
4. The tool is available for the next AI turn

### Compilation

To permanently bake dynamic tools into the binary:

```bash
# Option 1: Set environment variable before session
GORKBOT_AUTO_REBUILD=1 ./gorkbot.sh

# Option 2: Use the rebuild tool inside the session
rebuild {}

# Option 3: Manual build
go build -o bin/gorkbot ./cmd/gorkbot/
```

---

## 10. MCP Tool Integration

MCP (Model Context Protocol) tools from external servers are registered dynamically at startup.

### Startup Flow

```
main.go:
  mcpMgr := mcp.NewManager(configDir, logger)
  mcpMgr.LoadConfig()   // read ~/.config/gorkbot/mcp.json
  mcpMgr.StartAll(ctx)  // spawn server subprocesses

  for server, tools := range mcpMgr.AllTools() {
      for _, tool := range tools {
          prefix := "mcp_" + server.Name + "_"
          registry.Register(mcp.WrapTool(prefix, tool))
      }
  }
```

### MCP Tool Wrapper

`mcp.WrapTool()` creates a `Tool` implementation that:
- Uses `mcp_<server>_<toolname>` as the name
- Maps parameters from the MCP tool schema to Gorkbot's `Parameter` type
- Executes by sending a `tools/call` JSON-RPC request to the server subprocess
- Returns the string content of the `content[0].text` field from the response

All MCP tools default to `once` permission and `enabled` state.

---

## 11. Python Plugin Tools

The Python plugin bridge (`plugins/python/`) is managed by `pkg/python.Manager`.

### Discovery

On startup, `pkg/python.Manager` scans `plugins/python/*/manifest.json`:

```go
func (m *Manager) Discover(pluginsDir string) error {
    entries, _ := os.ReadDir(pluginsDir)
    for _, entry := range entries {
        manifestPath := filepath.Join(pluginsDir, entry.Name(), "manifest.json")
        if manifest, err := loadManifest(manifestPath); err == nil {
            m.plugins[manifest.Name] = &Plugin{manifest, pluginsDir}
        }
    }
    return nil
}
```

### Auto-Install

When a plugin is first invoked, `Manager.EnsureDeps()` runs:
```bash
pip install <requires from manifest>
```

Dependencies are installed once and cached; subsequent invocations skip the install step.

### Plugin Tool Wrapper

Each discovered plugin is wrapped as a `Tool` that:
1. Resolves the Python interpreter (Termux: `python3` from Termux packages)
2. Calls `python3 plugins/python/<name>/tool.py` with params as JSON stdin
3. Reads stdout as the result string
4. Reports stderr as the error on non-zero exit

### RAG Memory Plugin

The built-in `rag_memory` plugin (`plugins/python/rag_memory/`) provides semantic vector memory:

```
Tool name: rag_memory
Actions:   store | search | stats | purge

store: embed content into ChromaDB (auto-installs chromadb + sentence-transformers)
search: cosine similarity search with configurable min_score
stats: show total engrams and collection metadata
purge: delete all stored engrams
```

Storage: `~/.config/gorkbot/rag_memory/` (ChromaDB persistent directory, set via `GORKBOT_CONFIG_DIR`).

---

## 12. Tool Call Flow: End-to-End

This traces a complete tool call from AI response parsing through result delivery.

### Step 1: Parse Tool Request from AI Response

**Native path (xAI Grok):**
```go
// AI returns structured tool_calls in JSON response
for _, tc := range response.ToolCalls {
    call := ToolCall{Name: tc.Function.Name, Args: tc.Function.Arguments}
    pendingCalls = append(pendingCalls, call)
}
```

**Text path (all other providers):**
```go
// ParseToolRequests() scans the response text for tool call patterns
calls := ai.ParseToolRequests(responseText)
```

### Step 2: Batch Dispatch

```go
// Dispatcher groups all calls from a single turn into a batch
results := dispatcher.Batch(ctx, registry, pendingCalls)
```

### Step 3: Per-Tool Execution (runs concurrently)

```go
// For each call in the batch (up to 4 concurrent goroutines):
result, err := registry.Execute(ctx, call.Name, call.Args)
```

### Step 4: Permission Pipeline (inside Execute)

1. Category guard → returns error if disabled
2. Rule engine → allow/deny/passthrough
3. Permission store → allow/deny/show prompt

### Step 5: Cache Check

```go
cacheKey := buildCacheKey(name, params)
if cached, ok := cache.Get(cacheKey); ok && tool.IsReadOnly() {
    return cached, nil  // cache hit — no execution
}
```

### Step 6: Tool Execution

```go
result, err := tool.Execute(ctx, resolvedParams)
// - runs with 30s deadline (or tool-specific timeout)
// - captures stdout/stderr for shell tools
// - returns (string, error)
```

### Step 7: Post-Execution

```go
// Cache the result if the tool is read-only
if tool.IsReadOnly() && err == nil {
    cache.Set(cacheKey, result, tool.CacheTTL())
}

// Invalidate related cache entries if the tool is a mutation
if tool.IsMutation() && err == nil {
    cache.InvalidateRelated(name, params)
}

// Record analytics
analytics.Record(CallRecord{ToolName: name, Duration: elapsed, Success: err == nil})

// Feed MEL
if err != nil {
    bifurcation.ObserveFailed(name, params, err)
} else {
    bifurcation.ObserveSuccess(name, params, result)
}
```

### Step 8: Add Result to Conversation History

**Native path:**
```go
history.AddToolResultMessage(tc.ID, name, result)
// Role: "tool", ToolCallID: tc.ID, Content: result
```

**Text path:**
```go
history.AddUserMessage(fmt.Sprintf("[Tool result: %s]\n%s", name, result))
```

### Step 9: Next AI Turn

The updated history (including tool results) is sent to the AI provider for the next completion. The AI incorporates tool results and either responds to the user or requests additional tools.
