# Gorkbot Architecture

**Version:** 3.4.0

This document describes the complete technical architecture of Gorkbot — from the entry point through the orchestration engine, intelligence layer, memory systems, tool execution pipeline, and terminal UI.

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Startup Sequence](#2-startup-sequence)
3. [Orchestration Engine](#3-orchestration-engine)
4. [AI Provider System](#4-ai-provider-system)
5. [Intelligence Layer (ARC + MEL)](#5-intelligence-layer-arc--mel)
6. [CCI Memory System](#6-cci-memory-system)
7. [Tool Execution Pipeline](#7-tool-execution-pipeline)
8. [Subagent System](#8-subagent-system)
9. [Terminal UI](#9-terminal-ui)
10. [Integrations](#10-integrations)
11. [Data Flow: Single Turn](#11-data-flow-single-turn)
12. [Package Dependency Graph](#12-package-dependency-graph)

---

## 1. System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Gorkbot v3.4.0                              │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                    Terminal UI (TUI)                          │  │
│  │         Bubble Tea · Lip Gloss · Glamour (Markdown)          │  │
│  └─────────────────────────────┬────────────────────────────────┘  │
│                                 │ user input / messages             │
│  ┌──────────────────────────────▼────────────────────────────────┐  │
│  │                  Orchestration Engine                         │  │
│  │   Primary AI ←→ Consultant AI   ARC Router   MEL Store       │  │
│  │   ConversationHistory           CCI Layer    GoalLedger       │  │
│  └───────────┬────────────────────────┬──────────────────────────┘  │
│              │                        │                             │
│  ┌───────────▼──────────┐  ┌──────────▼──────────────────────────┐  │
│  │   AI Provider System  │  │        Tool Registry                │  │
│  │  xAI · Gemini        │  │  150+ tools · Permissions · Cache   │  │
│  │  Anthropic · OpenAI  │  │  Dispatcher · Analytics · Rules     │  │
│  │  MiniMax             │  └─────────────────────────────────────┘  │
│  └───────────────────────┘                                          │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                   Integration Layer                          │   │
│  │  MCP Client · A2A Gateway · Telegram · SSE Relay            │   │
│  │  Scheduler · SQLite Persist · Billing · Session Management  │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. Startup Sequence

`cmd/gorkbot/main.go` orchestrates initialization in strict order:

```
1. Platform detection (GetEnvConfig)
   └─ OS, arch, Termux, config dir, log dir

2. .env loading (loadEnv)
   └─ AES-GCM decryption for ENC_-prefixed values

3. CLI flag parsing (flag.Parse)
   └─ --share/--join handled first (observer-only mode)

4. Provider system
   ├─ KeyStore (api_keys.json, seeds from env vars)
   ├─ ProviderManager (one base instance per provider)
   ├─ ModelRegistry (dynamic model list)
   └─ Router.SelectSystemModels() → primary + specialist

5. Tool system
   ├─ PermissionManager (tool_permissions.json)
   ├─ Analytics (SQLite)
   ├─ Registry.RegisterDefaultTools() (~150 tools)
   ├─ Process tools (start/list/stop/read managed processes)
   ├─ Subagent tools (spawn_agent, spawn_sub_agent, …)
   └─ LoadDynamicTools (dynamic_tools.json — hot-loaded)

6. Discovery manager
   └─ Polls all 5 providers; starts 30-min refresh loop

7. Scheduler + UserCmdLoader

8. Orchestrator construction
   ├─ SetConfigDir, Billing, InitSENSEMemory
   ├─ InitEnhancements (hooks, config, rules, checkpoints, CCI)
   ├─ InitIntelligence (ARC Router + MEL)
   ├─ GoalLedger, UnifiedMemory
   └─ Colony runner, Pipeline runner wired in

9. Trace logger (--trace flag)

10. MCP manager (stdio clients per mcp.json)

11. Theme manager + AppStateManager (restores saved model/tool prefs)

12. A2A gateway, Telegram bot, Scheduler start

13. SSE relay (--share) + Feedback manager

14. SQLite session store (persist.NewStore)

15. Dispatch
    ├─ One-shot: runOneShotTask(ctx, orch, prompt, outputFile)
    └─ Interactive: runTUI(orch, …)

16. Post-session: handlePendingRebuild (if create_tool was used)
```

---

## 3. Orchestration Engine

**Package:** `internal/engine`

The `Orchestrator` struct is the central coordination point:

```go
type Orchestrator struct {
    Primary     ai.AIProvider           // Primary AI (usually Grok)
    Consultant  ai.AIProvider           // Specialist AI (usually Gemini)
    Registry    *tools.Registry         // All registered tools
    Logger      *slog.Logger

    // Memory
    ConvHistory  *ai.ConversationHistory // Full message history
    CheckpointMgr *session.CheckpointManager
    AgeMem        *sense.AgeMem
    Engrams        *sense.EngramStore
    GoalLedger     *memory.GoalLedger
    UnifiedMem     *memory.UnifiedMemory
    CCI            *cci.CCILayer

    // Intelligence
    Intelligence  *IntelligenceLayer   // ARC + MEL

    // Context
    ContextMgr    *ContextManager       // Token tracking + cost

    // Enhancements
    Rules          *tools.RuleEngine
    Hooks          *hooks.Manager
    Config         *config.Loader
    ModeManager    *plan_mode.ModeManager
    Tracer         *TraceLogger

    // Integrations
    Relay          *collab.Relay
    Feedback       *router.FeedbackManager
    Discovery      *discovery.Manager
    Billing        *billing.BillingManager
    SecurityCtx    *subagents.SecurityContext
}
```

### Task Execution Loop (`ExecuteTaskWithTools`)

The core loop runs inside `ExecuteTaskWithTools` in `streaming.go`:

```
1. Build system message (tool definitions + CCI prefix)
2. ARC.Route(prompt) → RouteDecision{Budget, Classification}
3. Inject MEL heuristics into system prompt (first turn)
4. ConversationHistory.AddUserMessage(prompt)
5. CheckpointManager.Save()
6. Pre-turn hook: hooks.Manager.RunHook("pre_turn", …)
7. Determine if consultant needed (isConsultant heuristic)
8. If consultant needed:
   a. Consultant.StreamWithHistory(history) → advice string
   b. Append consultant advice to user message context
9. Primary AI call:
   a. If NativeToolCaller: GenerateWithTools(history, schemas) → structured tool_calls
   b. Else: StreamWithHistory(history) → text stream → ParseToolRequests
10. Post-AI turn: update token usage → ContextMgr.UpdateFromUsage
11. Tool execution batch (if tools requested):
    a. Parallel: up to 4 concurrent goroutines (sync.WaitGroup)
    b. Permission check per tool (RuleEngine → PermissionManager)
    c. TTL cache lookup (cache.go)
    d. Execute tool → result
    e. MEL: ObserveFailed/ObserveSuccess
    f. ConversationHistory.AddToolResult / AddUserMessage
12. Repeat from step 9 up to maxTurns (overridden by ARC budget)
13. Post-turn hook
14. Return final response
```

### Context Manager (`context_manager.go`)

Tracks token consumption and estimated USD cost per provider:

```
TokenUsage{PromptTokens, CompletionTokens, TotalTokens}
  → ContextManager.UpdateFromUsage()
  → emits ContextUpdateMsg to TUI (live status bar update)
  → ContextMgr.GetReport() → "/context" command output
  → BillingManager.Record() → usage_history.jsonl
```

### Plan Mode (`plan_mode.go`)

Three modes managed by `ModeManager`:

| Mode | Behaviour |
|------|-----------|
| `Normal` | Standard agentic execution |
| `Plan` | AI drafts and presents plan before acting |
| `Auto` | Autonomous execution, minimal confirmation |

Mode persists across turns until explicitly changed via `/mode` or `Ctrl+P`. CCI gap detection auto-switches to `Plan` mode.

### Execution Trace (`trace.go`)

When `--trace` is set, every tool call, AI turn, and routing decision is written as a JSONL line to `~/.gorkbot/traces/<timestamp>.jsonl`. Useful for debugging orchestration loops.

---

## 4. AI Provider System

**Packages:** `pkg/ai`, `pkg/providers`, `pkg/registry`, `pkg/router`

### Provider Hierarchy

```
providers.KeyStore (api_keys.json + env vars)
    └─ providers.Manager
           ├─ GrokProvider (pkg/ai/grok.go)
           ├─ GeminiProvider (pkg/ai/gemini.go)
           ├─ AnthropicProvider (pkg/ai/anthropic.go)
           ├─ OpenAIProvider (pkg/ai/openai_provider.go)
           └─ MiniMaxProvider (pkg/ai/minimax.go)
```

### AIProvider Interface

```go
type AIProvider interface {
    Generate(ctx, prompt) (string, error)
    GenerateWithHistory(ctx, history) (string, error)
    StreamWithHistory(ctx, history, writer) error
    WithModel(id string) AIProvider
    GetMetadata() ProviderMetadata
}

// Optional extension for native function calling:
type NativeToolCaller interface {
    GenerateWithTools(ctx, history, schemas) (response, toolCalls, error)
}

// Optional extension for usage reporting:
type UsageReporter interface {
    GetLastUsage() TokenUsage
}
```

### Dynamic Model Selection

On startup, `router.Router.SelectSystemModels()` queries all registered providers for their live model lists and ranks them:

```
ModelRegistry
  → ProviderManager.ListAvailableModels(provider) [for each provider]
  → Registry.RankModels(criteria: latency, capability, cost)
  → SystemConfiguration{PrimaryModel, SpecialistModel, Reasoning}
```

Env vars `GORKBOT_PRIMARY_MODEL` and `GORKBOT_CONSULTANT_MODEL` bypass this and force specific model IDs.

### Native xAI Function Calling

`GrokProvider` implements `NativeToolCaller`. When the primary provider is Grok, the orchestrator:

1. Converts tool definitions to `GrokToolSchema` structs
2. Sends `tools: [...]` in the API request body
3. Receives structured `tool_calls` (no text parsing required)
4. Appends `role:"assistant"` messages with `ToolCalls` field and `role:"tool"` result messages to history

Other providers fall back to the text-parsing path using `ParseToolRequests`.

### Conversation History

`ConversationHistory` (thread-safe via `sync.RWMutex`) maintains the full message chain:

```go
type ConversationMessage struct {
    Role        string       // "system" | "user" | "assistant" | "tool"
    Content     string
    Timestamp   time.Time
    ToolCalls   []ToolCallEntry  // populated for native function calling
    ToolCallID  string
    ToolName    string
}
```

**Token management:** `TruncateToTokenLimit(maxTokens)` preserves all system messages and the most recent conversation that fits. The truncation algorithm starts from the most recent message and works backward — a large individual message is skipped (not dropped), ensuring older context is preserved.

---

## 5. Intelligence Layer (ARC + MEL)

**Packages:** `internal/arc`, `internal/mel`

### ARC Router

```
internal/arc/
├── classifier.go     — QueryClassifier (keyword scoring + sigmoid → WorkflowDirect | WorkflowReasonVerify)
├── budget.go         — SystemDetector (HALProfile.TotalRAMMB → PlatformClass)
│                       ComputeBudget() → ResourceBudget{MaxTokens, Temperature, MaxToolCalls, Timeout}
├── router.go         — ARCRouter.Route(prompt) → RouteDecision{Classification, Budget, Timestamp}
│                       RouterStats (cumulative counts per classification)
├── trigger_table.go  — TriggerTable (file path patterns → domain label for CCI Tier 2)
└── consistency.go    — ReframedEvaluator: IsHighRisk() + CheckConsistency() for destructive ops
```

**Classification path:**

```
prompt
  → QueryClassifier.Classify(prompt)
  → keyword scoring (weighted feature vectors) + sigmoid normalization
  → WorkflowDirect (simple queries) | WorkflowReasonVerify (complex, multi-step)
  → ComputeBudget(classification, platformClass)
  → ResourceBudget{MaxTokens, MaxToolCalls, Temperature, Timeout}
```

The `MaxToolCalls` from `ResourceBudget` overrides `maxTurns` in the orchestrator loop, ensuring compute-constrained devices don't spin indefinitely.

### MEL (Meta-Experience Learning)

```
internal/mel/
├── heuristic.go  — Heuristic struct: "When [ctx], verify [constraint], avoid [error]"
├── store.go      — VectorStore: JSON at ~/.config/gorkbot/vector_store.json
│                   Jaccard similarity for retrieval
│                   Deduplication at > 70% similarity
│                   Evicts lowest-confidence heuristic at 500-entry capacity
└── analyzer.go   — BifurcationAnalyzer: observes (task, params, success/failure)
                    ObserveFailed / ObserveSuccess cycle
                    Auto-generates heuristics from parameter diffs between fail/success pairs
```

**Integration in orchestrator:**
1. On session start: retrieve top-N heuristics relevant to current task → inject into system prompt
2. After every `ExecuteTool`: `ObserveFailed(task, params, error)` or `ObserveSuccess(task, params)`
3. On bifurcation (same task, different outcome) → `VectorStore.Add(newHeuristic)` → persisted to disk

### IntelligenceLayer

`internal/engine/intelligence.go` bundles the two systems:

```go
type IntelligenceLayer struct {
    ARC      *arc.ARCRouter
    Store    *mel.VectorStore
    Analyzer *mel.BifurcationAnalyzer
    Evaluator *arc.ReframedEvaluator
}
```

Initialized by `Orchestrator.InitIntelligence(configDir)` — failures are non-fatal (logged and skipped).

---

## 6. CCI Memory System

**Package:** `pkg/cci`

Three-tier persistent project memory injected at session start. See also: [CONTEXT_CONTINUITY.md](CONTEXT_CONTINUITY.md).

### Tier 1 — Hot Memory (always loaded)

```
~/.config/gorkbot/cci/hot/
├── CONVENTIONS.md          — Universal project conventions (editable)
└── SUBSYSTEM_POINTERS.md   — Index of Tier 3 subsystems + short descriptions
```

`HotMemory.BuildBlock()` returns a compact string prepended to every system message. Includes conventions, subsystem index summary, and ARC trigger table summary.

### Tier 2 — Specialist Memory (on-demand)

```
~/.config/gorkbot/cci/specialists/<domain>.md
```

Loaded by `SpecialistManager.Load(domain)` when `TriggerTable.MatchTrigger(filePath)` returns a domain label. Contains failure mode tables, domain-specific patterns, and known pitfalls. Synthesized from task context and existing Tier 2 content.

### Tier 3 — Cold Memory (on-demand via tool)

```
~/.config/gorkbot/cci/docs/<subsystem>.md
```

Queried via `mcp_context_get_subsystem` tool. On cache miss (empty result), `HandleCCIGap()` triggers a switch to Plan mode so the AI stops and explicitly plans how to fill the knowledge gap before proceeding.

### CCI Tools

All registered via `RegisterCCITools()` in `pkg/tools/cci_tools.go`:

| Tool | Description |
|------|-------------|
| `mcp_context_list_subsystems` | List all Tier 3 subsystem docs |
| `mcp_context_get_subsystem` | Retrieve a Tier 3 spec (gap → Plan mode) |
| `mcp_context_suggest_specialist` | Keyword-score task → recommend Tier 2 domain |
| `mcp_context_update_subsystem` | Write/update a Tier 3 living doc |
| `mcp_context_list_specialists` | List all Tier 2 specialist domains |
| `mcp_context_status` | Full CCI status report |

### Drift Detection

`DriftDetector.Check()` cross-references Tier 3 docs with `git log` to identify files that have changed since a doc was last updated. Drift warnings are prepended to the Tier 1 block so the AI is aware that its knowledge may be stale.

---

## 7. Tool Execution Pipeline

**Package:** `pkg/tools`

### Registry

`Registry` is the central tool registry:

```go
type Registry struct {
    tools              map[string]Tool
    permissionMgr      *PermissionManager
    analytics          *Analytics           // SQLite
    sessionPerms       map[string]bool
    disabledCategories map[ToolCategory]bool
    schedulerInst      *scheduler.Scheduler
    userCmdLoader      *usercommands.Loader
    contextStats       ContextStatsReporter
    introspectionRep   IntrospectionReporter
    goalLedger         GoalLedgerAccessor
    colonyRunner       func(ctx, sys, prompt string) (string, error)
    pipelineRunner     func(ctx, agentType, task string) (string, error)
    securityBriefFn    func() string
}
```

### Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    Category() ToolCategory
    Parameters() map[string]ParameterSchema
    RequiresPermission() bool
    DefaultPermission() PermissionLevel
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}
```

### Execution Path

```
Registry.Execute(ctx, toolName, params)
  1. Category enabled? (disabledCategories guard)
  2. Tool exists? (map lookup)
  3. Permission check:
     a. RuleEngine.Evaluate(toolName, params) → RuleDecision (allow/ask/deny)
     b. If ask: PermissionManager.Check(toolName) → PermissionLevel
     c. If "never": return error immediately
  4. Cache lookup (TTL cache, keyed by name+params hash)
     → Hit: return cached result
     → Miss: proceed
  5. Inject dependencies into ctx:
     (CCI layer, scheduler, user cmd loader, context stats, goal ledger, security ctx)
  6. tool.Execute(ctx, params)
  7. Analytics.Record(toolName, duration, success)
  8. Cache.Set(result, TTL) if not a mutation tool
  9. Return result
```

### Parallel Dispatch

When the orchestrator receives multiple tool requests in one AI turn:

```
batchRequests := ParseToolRequests(response)   // or native tool_calls
results := make([]string, len(batchRequests))
sem := make(chan struct{}, 4)                   // cap = 4 concurrent
var wg sync.WaitGroup

for i, req := range batchRequests {
    wg.Add(1)
    go func(i int, req ToolRequest) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        results[i] = Registry.Execute(ctx, req.Name, req.Params)
    }(i, req)
}
wg.Wait()
// results ordered by original tool request index
```

### Permission Manager

Permissions persisted in `~/.config/gorkbot/tool_permissions.json`:

```json
{
  "permissions": {
    "read_file": "always",
    "bash": "once",
    "delete_file": "never"
  },
  "version": "1.0"
}
```

Session-only permissions live in `Registry.sessionPerms` (in-memory, cleared on exit).

### Rule Engine (`rules.go`)

Glob-pattern rules evaluated before the permission manager:

```go
type Rule struct {
    Decision  RuleDecision    // Allow | Ask | Deny
    Pattern   string          // Glob, e.g. "read_*" or "git_push"
    Comment   string
}
```

Rules are evaluated in order; first match wins. If no rule matches, falls through to the standard permission check.

### Tool Cache (`cache.go`)

TTL-based memoization. Mutation tools (write_file, bash, git_commit, etc.) bypass the cache. Non-mutation reads (read_file, list_directory, git_status, etc.) are cached for the TTL duration (default 60s). Cache is invalidated when a mutation tool modifies the same resource scope.

### Error Recovery (`error_recovery.go`)

`ClassifyError(err)` returns a structured `ToolError{Code, RecoveryAction, UserMessage}`:

| Code | Recovery Action |
|------|----------------|
| `ErrPermissionDenied` | Re-prompt with lower-privilege approach |
| `ErrTimeout` | Retry with shorter operation |
| `ErrNetworkFailure` | Retry with backoff |
| `ErrNotFound` | Suggest alternative path |
| `ErrInvalidParam` | Return parameter validation message |

`EnrichResult(result, err)` wraps raw errors with context and suggested next steps.

### Dynamic Tools

`create_tool` generates a JSON tool definition written to `~/.config/gorkbot/dynamic_tools.json`. On next startup (or immediately via `LoadDynamicTools`), these are instantiated as `DynamicTool` objects wrapping a template command string with `{{param}}` substitutions. `rebuild` compiles them permanently into the binary via `go build`.

---

## 8. Subagent System

**Package:** `pkg/subagents`

### Agent Types

`subagents.AgentRegistry` maps `AgentType` strings to `Agent` implementations. Each `Agent` receives an isolated tool registry slice and can call tools independently.

### SpawnAgentTool

Spawns a background goroutine running an agent. The spawning AI can:
- `spawn_agent(type, task)` — fire-and-forget background execution
- `check_agent_status(id)` — poll for completion
- `collect_agent(id)` — block until done and retrieve result
- `list_agents()` — list all active agents

### SpawnSubAgentTool (Recursive Hive)

Discovery-aware recursive delegation:

```go
type SpawnSubAgentParams struct {
    Task        string          // task description
    AgentType   string          // "reasoning" | "speed" | "coding" | "general"
    Isolated    bool            // if true, creates a git worktree
    VerifyWith  string          // optional secondary model for verification pass
    MaxDepth    int             // default 4 — prevents runaway recursion
}
```

Depth is tracked via a context key. At `MaxDepth`, the tool returns an error rather than spawning deeper. `discovery.Manager.BestForCap(CapabilityClass)` selects the best available model for the requested capability class.

If `Isolated=true`, a git worktree is created in `.claude/worktrees/<id>`, the agent task runs in that directory, and the worktree is automatically removed after a 15-minute timeout.

### Worktree Manager

`subagents.WorktreeManager` wraps `git worktree add/remove/list`. `pkg/tools/worktrees.go` exposes `create_worktree`, `list_worktrees`, `remove_worktree`, and `integrate_worktree` tools.

### Agentic Pipeline

`pkg/pipeline` provides multi-step coordinated agent execution. `run_pipeline(agentType, task)` delegates to `pipelineRunner` (wired from orchestrator), which looks up the agent type, instantiates it, and runs it synchronously, returning the result.

### Colony Debate

`colony_debate(topic, perspectives)` invokes `colonyRunner` multiple times with different system personas and synthesizes the responses. Useful for getting multiple viewpoints on a design decision.

---

## 9. Terminal UI

**Package:** `internal/tui`

The TUI is a [Bubble Tea](https://github.com/charmbracelet/bubbletea) application following the Elm Architecture (Model-Update-View).

### Model (`model.go`)

```go
type Model struct {
    // Core state
    messages       []Message
    input          textinput.Model
    viewport       viewport.Model

    // Overlay states
    modelSelectState  modelSelectState    // Ctrl+T
    apiKeyPromptState apiKeyPromptState
    settingsOverlay   settingsOverlay     // Ctrl+G

    // Views
    toolsView         toolsView           // Ctrl+E
    diagnosticsView   diagnosticsView     // Ctrl+\
    discoveryView     discoveryView       // Ctrl+D
    bookmarksOverlay  bookmarksOverlay    // Ctrl+B

    // Live data
    statusBar         StatusBar
    keys              KeyMap
    streaming         bool
    currentGitBranch  string

    // Orchestrator reference
    orch    *engine.Orchestrator
    cmdReg  *commands.Registry
}
```

### Message Types (`messages.go`)

Bubble Tea messages used for async communication:

| Message | Purpose |
|---------|---------|
| `StreamChunkMsg` | One streaming token chunk from AI |
| `StreamCompleteMsg` | AI turn finished |
| `ToolResultMsg` | Tool execution result ready |
| `ContextUpdateMsg` | Token usage / cost updated |
| `ModeChangeMsg` | Execution mode changed |
| `InterruptMsg` | User cancelled generation |
| `ToolProgressMsg` | Tool start/progress notification |
| `RewindCompleteMsg` | Checkpoint rewind done |
| `DiscoveryUpdateMsg` | New model list from discovery |
| `ModelRefreshMsg` | Model list refreshed |
| `APIKeySavedMsg` | API key saved successfully |
| `ModelSwitchedMsg` | Primary or specialist model changed |
| `ProviderStatusMsg` | Provider key validation result |

### Streaming Architecture

AI responses stream via a goroutine that writes `StreamChunkMsg` messages to the Bubble Tea program channel. The `handleStreamComplete` function in `update.go` assembles the final response, strips tool JSON blocks (unless debug mode is on), and renders markdown.

### Touch Scroll (Android)

The `tea.MouseMsg` handler in `update.go` (lines 67–91) translates finger scroll events to viewport scroll commands. This block must not be modified — it is carefully tuned for Android/Termux where mouse events encode scroll direction differently.

### Status Bar (`statusbar.go`)

```
[ primary-model ] [ consultant-model ] [ mode ] [ ctx% ] [ $cost ] [ git-branch ]
```

Updated via `StatusBar.SetContextStats()`, `SetGitBranch()`, `SetMode()`, `SetCost()`.

### Settings Overlay (`settings_overlay.go`)

Three-tab modal:

| Tab | Content |
|-----|---------|
| Model Routing | Primary and specialist model info |
| Verbosity | Verbose thoughts toggle |
| Tool Groups | Enable/disable tool categories |

---

## 10. Integrations

### MCP (Model Context Protocol) — `pkg/mcp`

```
mcp.json (config)
  → Manager.LoadAndStart() → spawns one stdio subprocess per server
  → JSON-RPC 2.0 over stdin/stdout
  → List tools from each server → wrap as mcp_<server>_<toolname>
  → Register all wrapped tools in tool registry
```

### A2A (Agent-to-Agent) — `pkg/a2a`

HTTP server listening on `--a2a-addr` (default `127.0.0.1:18890`). Exposes a `TaskRunnerFunc` that routes HTTP POST requests to `Orchestrator.ExecuteTask`. Enables external agents to delegate tasks to this Gorkbot instance.

### Telegram — `pkg/channels/telegram`

Bot configured via `~/.config/gorkbot/telegram.json`. Each incoming message is passed to `Orchestrator.ExecuteTask` and the response is sent back to the user. The bot runs in a goroutine started during `main()` and stopped via `defer tgMgr.Stop()`.

### SSE Relay — `pkg/collab`

`--share` starts `collab.Relay` on a random port and prints the observer URL. The relay broadcasts:
- Streaming tokens (`OnToken`)
- Tool start/done events
- Turn complete notifications

Observers join with `--join localhost:<port>` and run in a lightweight mode (no orchestrator, no TUI) that just prints events.

### Scheduler — `pkg/scheduler`

Cron-style tasks stored in SQLite. `schedule_task(cron_expr, task)` adds a job. The scheduler goroutine polls for due tasks and dispatches them to `Orchestrator.ExecuteTask`. `/schedule` lists active tasks.

### Billing — `pkg/billing`

`BillingManager` records per-turn token usage by provider and model to `usage_history.jsonl`. Costs are estimated using per-model price tables. `/cost` shows the session estimate; `BillingGetAllTime()` reads the JSONL log for a lifetime summary.

### SQLite Persistence — `pkg/persist`

Per-session SQLite database (one file per session) stores:
- Full conversation history (all messages)
- Tool call analytics (name, duration, success, timestamp)

---

## 11. Data Flow: Single Turn

```
User: "Refactor the auth module to use JWT"
  │
  ▼
TUI.handleCommand()
  │  not a slash command → route to orchestrator
  ▼
Orchestrator.ExecuteTaskWithTools(ctx, prompt)
  │
  ├─► ARC.Route(prompt)
  │     Classification: WorkflowReasonVerify
  │     Budget: MaxToolCalls=8, Temperature=0.3
  │
  ├─► MEL.Retrieve(prompt) → top 3 heuristics
  │     "When touching auth, verify token expiry handling"
  │
  ├─► CCI.BuildCCISystemContext(prompt)
  │     Tier 1: conventions + subsystem index
  │     Tier 2: security specialist persona (trigger table matched "auth")
  │
  ├─► ConvHistory.AddUserMessage(prompt)
  │
  ├─► Consultant needed? (isConsultant heuristic)
  │   Yes → Gemini.StreamWithHistory(history)
  │            → architectural advice appended to context
  │
  ├─► Primary (Grok) NativeToolCaller:
  │     GenerateWithTools(history, toolSchemas)
  │     Response: tool_calls: [
  │       {name: "read_file", params: {path: "pkg/auth/auth.go"}},
  │       {name: "grep_content", params: {pattern: "jwt|session", path: "pkg/auth/"}}
  │     ]
  │
  ├─► Parallel tool execution (2 concurrent):
  │     Thread A: read_file(auth.go) → file content
  │     Thread B: grep_content(…) → matches
  │
  ├─► ConvHistory.AddToolResult(…) × 2
  │
  ├─► Primary: GenerateWithTools(history, toolSchemas) [turn 2]
  │     Response: tool_calls: [
  │       {name: "write_file", params: {path: "pkg/auth/auth.go", content: "…"}}
  │     ]
  │
  ├─► Permission check: write_file → "once" → TUI shows prompt → user: "session"
  │
  ├─► write_file executes → success
  ├─► MEL.ObserveSuccess("refactor", {path: "pkg/auth/auth.go"})
  ├─► Billing.Record(promptTokens, completionTokens)
  │
  ├─► Primary: final summary turn
  │     "Refactored auth.go to use JWT. Changes: …"
  │
  └─► StreamCompleteMsg → TUI renders markdown response
```

---

## 12. Package Dependency Graph

Key allowed import relationships (cycles strictly forbidden):

```
cmd/gorkbot
  → internal/engine
  → internal/tui
  → internal/arc
  → internal/mel
  → internal/platform
  → pkg/ai
  → pkg/providers
  → pkg/tools
  → pkg/commands        (commands ← tools, NOT commands ← engine)
  → pkg/mcp
  → pkg/a2a
  → pkg/collab
  → pkg/discovery
  → pkg/billing
  → pkg/persist
  → pkg/memory
  → pkg/cci
  → pkg/session
  → pkg/subagents
  → pkg/scheduler
  → pkg/skills
  → pkg/theme
  → pkg/hooks
  → pkg/config
  → pkg/registry
  → pkg/router

Forbidden cycles:
  ✗ internal/engine → internal/tui   (use OrchestratorAdapter in pkg/commands)
  ✗ pkg/tools → pkg/subagents        (use FindingRecorder interface for duck typing)
  ✗ pkg/subagents → pkg/tools        (subagents receive *tools.Registry as parameter)
```

`OrchestratorAdapter` in `pkg/commands/registry.go` is the bridge between the command layer and the orchestrator — it holds function references rather than a direct import, preventing the `engine→tui` cycle.
