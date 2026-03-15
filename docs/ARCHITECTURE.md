# Gorkbot Architecture

**Version:** 4.7.0

This document describes the complete technical architecture of Gorkbot — from the entry point through the orchestration engine, intelligence layer, memory systems, tool execution pipeline, and terminal UI.

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Startup Sequence](#2-startup-sequence)
3. [Orchestration Engine](#3-orchestration-engine)
4. [AI Provider System](#4-ai-provider-system)
5. [pkg/adaptive — Intelligence Layer](#5-pkgadaptive--intelligence-layer)
6. [Tool Execution Pipeline](#6-tool-execution-pipeline)
7. [SENSE Module](#7-sense-module)
8. [Memory Systems](#8-memory-systems)
9. [DAG Execution Engine](#9-dag-execution-engine)
10. [Subagent System](#10-subagent-system)
11. [Terminal UI](#11-terminal-ui)
12. [Integrations](#12-integrations)
13. [Local LLM Embedding](#13-local-llm-embedding)
14. [Data Flow: Single Turn](#14-data-flow-single-turn)
15. [Package Dependency Graph](#15-package-dependency-graph)

---

## 1. System Overview

```
┌────────────────────────────────────────────────────────────────────────┐
│                          Gorkbot v4.7.0                                │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │                    Terminal UI (TUI)                             │  │
│  │         Bubble Tea · Lip Gloss · Glamour (Markdown)             │  │
│  └──────────────────────────┬──────────────────────────────────────┘  │
│                              │ user input / messages                  │
│  ┌───────────────────────────▼──────────────────────────────────────┐  │
│  │                  Orchestration Engine                            │  │
│  │   Primary AI ←→ Consultant AI    ARC Router    MEL Store        │  │
│  │   ConversationHistory             CCI Layer    GoalLedger        │  │
│  │   HITL Guard · RalphLoop · RAGInjector · BudgetGuard            │  │
│  └──────────┬──────────────────────────┬───────────────────────────┘  │
│             │                          │                              │
│  ┌──────────▼───────────┐  ┌───────────▼─────────────────────────┐   │
│  │   AI Provider System  │  │        Tool Registry                │   │
│  │  xAI · Gemini        │  │  196+ tools · Permissions · Cache   │   │
│  │  Anthropic · OpenAI  │  │  Dispatcher · Analytics · Rules     │   │
│  │  MiniMax · Moonshot  │  │  SENSE Sanitizer · Audit DB         │   │
│  │  OpenRouter          │  └─────────────────────────────────────┘   │
│  └──────────────────────┘                                            │
│                                                                       │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │                   Integration Layer                               │ │
│  │  MCP Client · A2A Gateway · Telegram · Discord · SSE Relay      │ │
│  │  Scheduler · SQLite Persist · Billing · Session Management      │ │
│  │  DAG Engine · Colony Debate · Background Agents                  │ │
│  └──────────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Startup Sequence

`cmd/gorkbot/main.go` orchestrates initialization in strict order:

```
1. Platform detection (GetEnvConfig)
   └─ OS, arch, Termux flag, config dir, log dir

2. .env loading (loadEnv)
   └─ AES-GCM decryption for ENC_-prefixed values (pkg/security)

3. CLI flag parsing (flag.Parse)
   └─ --share/--join handled first (observer-only mode)

4. Provider system
   ├─ KeyStore (api_keys.json, seeds from env vars)
   ├─ ProviderManager (one base instance per provider)
   ├─ ModelRegistry (dynamic model list)
   └─ Router.SelectSystemModels() → primary + specialist

5. Tool system
   ├─ PermissionManager (tool_permissions.json)
   ├─ Analytics (SQLite via pkg/persist)
   ├─ AuditDB (SQLite structured audit log)
   ├─ Registry.RegisterDefaultTools() (tool packs from GORKBOT_TOOL_PACKS)
   ├─ Process tools (start/list/stop/read managed processes)
   ├─ Subagent tools (spawn_agent, spawn_sub_agent, colony_debate)
   ├─ SENSE tracer + input sanitizer wired to registry
   ├─ EnvSnapshot wired for capability pre-flight checks
   └─ LoadDynamicTools (dynamic_tools.json — hot-loaded)

6. Discovery manager
   └─ Polls all providers; starts 30-min refresh loop

7. Scheduler + UserCmdLoader

8. Orchestrator construction
   ├─ NewOrchestrator(primary, consultant, registry, logger)
   ├─ SetConfigDir, InitSENSEMemory, InitEnhancements
   │   └─ hooks, GORKBOT.md config, rules, checkpoints, CCI, compression pipe
   ├─ InitIntelligence (ARC Router + MEL VectorStore + BifurcationAnalyzer)
   ├─ GoalLedger, UnifiedMemory, VectorStore (RAG), RAGInjector
   ├─ BudgetGuard, RalphLoop, ContextInjector, BackgroundAgents
   ├─ Crystallizer (Python tool auto-generation monitor)
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

The `Orchestrator` struct is the central coordination point. Key fields:

```go
type Orchestrator struct {
    // AI providers
    Primary     ai.AIProvider
    Consultant  ai.AIProvider

    // Core
    Registry    *tools.Registry
    ConversationHistory *ai.ConversationHistory

    // Intelligence (pkg/adaptive)
    Intelligence *IntelligenceLayer   // ARC Router + MEL VectorStore + Analyzer
    CCI          *adaptive.CCILayer   // CCI three-tier memory

    // SENSE components
    LIE          *sense.LIEEvaluator
    Stabilizer   *sense.Stabilizer
    AgeMem       *sense.AgeMem
    Engrams      *sense.EngramStore
    Compressor   *sense.Compressor
    HITLGuard    *HITLGuard
    SENSETracer  *sense.SENSETracer

    // Context & cost
    ContextMgr  *ContextManager
    Billing     *billing.BillingManager
    BudgetGuard *BudgetGuard

    // Session management
    Checkpoints *session.CheckpointManager
    Exporter    *session.Exporter
    Workspace   *session.WorkspaceManager
    PersistStore *persist.Store

    // Memory
    GoalLedger  *memory.GoalLedger
    UnifiedMem  *memory.UnifiedMemory
    VectorStore *vectorstore.VectorStore
    RAGInjector *RAGInjector

    // Execution
    ModeManager *ModeManager
    RuleEngine  *tools.RuleEngine
    ToolCache   *tools.ToolCache
    Dispatcher  *tools.Dispatcher
    Hooks       *hooks.Manager
    ConfigLoader *config.Loader
    ContextInjector *ContextInjector
    CompressionPipe *CompressionPipe

    // Integrations
    Relay       *collab.Relay
    Feedback    *router.FeedbackManager
    Discovery   *discovery.Manager
    Tracer      *TraceLogger
    Billing     *billing.BillingManager

    // Advanced
    RalphLoop   *RalphLoop
    BackgroundAgents *BackgroundAgentManager
    Crystallizer *Crystallizer
    SecurityCtx *subagents.SecurityContext
    ThinkingBudget int
}
```

### Task Execution Loop (`streaming.go → ExecuteTaskWithTools`)

```
1. Build system message:
   - EnvContext (host environment snapshot)
   - MCPContext (running MCP servers)
   - CCI.BuildSystemContext(prompt)  [Tier 1 always + Tier 2 on-demand]
   - MEL heuristics (first turn only)
   - Tool definitions (GetSystemPrompt / GetSystemPromptNative)
   - GORKBOT.md hierarchical instructions
   - Skill index
   - Brain facts (cross-session engrams)

2. ARC.Route(prompt) → RouteDecision{WorkflowType, Budget, Confidence}
   - Budget.MaxToolCalls overrides maxTurns
   - LowConfidence → escalate to WorkflowAnalytical

3. CCI drift check → inject warnings if documentation is stale

4. ConversationHistory.AddUserMessage(prompt)
   - RAGInjector retrieves semantically similar past turns → prepended

5. CheckpointManager.Save()

6. Pre-turn hook: hooks.Manager.RunHook("pre_turn", …)

7. Determine if consultant needed (complexity heuristic / COMPLEX keyword)
   If yes:
   a. Consultant.StreamWithHistory(history) → advice
   b. Append advice to user message context

8. Primary AI call:
   a. If NativeToolCaller (Grok): GenerateWithTools → structured tool_calls
   b. Else: StreamWithHistory → text stream → ParseToolRequests
   c. SENSE Stabilizer evaluates output quality

9. Post-AI turn: UsageReporter.GetLastUsage → ContextMgr.UpdateFromUsage → Billing.Record

10. Tool execution batch (if tools requested):
    a. SENSE InputSanitizer validates all parameters
    b. Capability pre-flight check (binary/package availability)
    c. HITL guard — prompts user for destructive operations
    d. RuleEngine + PermissionManager check
    e. TTL cache lookup
    f. Parallel execution: up to 4 goroutines (sync.WaitGroup)
    g. AuditDB.LogExecution (async, non-blocking)
    h. SENSETracer.LogToolSuccess/Failure (async)
    i. MEL BifurcationAnalyzer.ObserveFailed/ObserveSuccess
    j. ConversationHistory.AddToolResult / AddUserMessage

11. Ralph Loop — self-referential retry if AI gets stuck

12. Repeat from step 8 up to maxTurns

13. Post-turn hook

14. Return final response
```

### Context Manager (`context_manager.go`)

Tracks token consumption and estimated USD cost per provider:

```
TokenUsage{PromptTokens, CompletionTokens, TotalTokens}
  → ContextManager.UpdateFromUsage()
  → emits ContextUpdateMsg to TUI (live status bar update)
  → BillingManager.Record() → usage_history.jsonl
  → BudgetGuard checks per-session and per-day limits
```

### Plan Mode (`plan_mode.go`)

Three modes managed by `ModeManager`:

| Mode | Behaviour |
|------|-----------|
| `Normal` | Standard agentic execution |
| `Plan` | AI drafts and presents plan before acting; CCI gap detection auto-switches here |
| `Auto` | Autonomous execution, minimal confirmation |

Mode persists across turns until explicitly changed via `/mode` or `Ctrl+P`.

### Intelligence Layer (`intelligence.go`)

```go
type IntelligenceLayer struct {
    Router   *adaptive.ARCRouter
    Store    *adaptive.VectorStore     // MEL heuristic store
    Analyzer *adaptive.BifurcationAnalyzer
    Reframer *adaptive.ReframedEvaluator
}
```

The intelligence layer is the bridge between `internal/engine` and `pkg/adaptive`. It exposes `Route(prompt)` and `HeuristicContext(prompt)` to the orchestrator without the orchestrator needing to import `pkg/adaptive` directly.

---

## 4. AI Provider System

**Packages:** `pkg/ai`, `pkg/providers`, `pkg/registry`, `pkg/router`

### Provider Hierarchy

```
providers.KeyStore (api_keys.json + env vars)
    └─ providers.Manager
           ├─ GrokProvider        (pkg/ai/grok.go)          — xAI
           ├─ GeminiProvider      (pkg/ai/gemini.go)         — Google
           ├─ AnthropicProvider   (pkg/ai/anthropic.go)      — Anthropic
           ├─ OpenAIProvider      (pkg/ai/openai_provider.go)— OpenAI
           ├─ MiniMaxProvider     (pkg/ai/minimax.go)        — wraps Anthropic
           ├─ MoonshotProvider    (pkg/ai/moonshot.go)       — Moonshot AI
           └─ OpenRouterProvider  (pkg/ai/openrouter.go)     — 400+ models
```

### AIProvider Interface (`pkg/ai/interface.go`)

```go
type AIProvider interface {
    Generate(ctx, prompt) (string, error)
    GenerateWithHistory(ctx, history) (string, error)
    Stream(ctx, prompt, writer) error
    StreamWithHistory(ctx, history, writer) error
    GetMetadata() ProviderMetadata
    Name() string
    ID() registry.ProviderID
    Ping(ctx) error
    FetchModels(ctx) ([]registry.ModelDefinition, error)
    WithModel(id string) AIProvider
}

// Optional — native function calling (xAI only):
type NativeToolCaller interface {
    GenerateWithTools(ctx, history, schemas) (response, toolCalls, error)
}

// Optional — usage reporting for billing:
type UsageReporter interface {
    GetLastUsage() TokenUsage
}
```

### Dynamic Model Selection

```
Router.SelectSystemModels()
  → ProviderManager.ListAvailableModels(provider) [all registered providers]
  → Registry.RankModels(criteria: capability, latency, cost)
  → SystemConfiguration{PrimaryModel, SpecialistModel}
```

Environment variables `GORKBOT_PRIMARY_MODEL` and `GORKBOT_CONSULTANT_MODEL` bypass dynamic selection entirely.

### Provider Failover Cascade

When the primary provider returns an error, `internal/engine/fallback.go` cycles through the cascade:

```
xAI → Google → Anthropic → MiniMax → OpenAI → OpenRouter
```

Each step tries the same request with the next available provider. The cascade skips providers that are disabled in `app_state.json`.

### Hardened Network Layer

All providers use `NewRetryClient()` (`pkg/ai/retry_client.go`) which provides:

- 15-second TCP keepalive
- TLS handshake timeout
- Response-header deadline
- Automatic retry on transient EOF / TLS-MAC / RST errors with exponential backoff

---

## 5. pkg/adaptive — Intelligence Layer

The `pkg/adaptive` package consolidates three previously-separate subsystems (ARC, MEL, CCI) into a single importable package. This eliminates the original import cycles between `internal/arc`, `internal/mel`, and `pkg/cci`.

### File Organization

```
pkg/adaptive/
├── arc_router.go          ARCRouter — routes prompts, computes budgets
├── arc_classifier.go      SemanticClassifier — WorkflowType classification
├── arc_categories.go      IntentCategory — 9 semantic categories with signals
├── arc_budget.go          ComputeBudget — HALProfile → ResourceBudget
├── arc_consistency.go     ReframedEvaluator — IsHighRisk, CheckConsistency
├── arc_trigger_table.go   Trigger table for CCI specialist dispatch
├── mel_heuristic.go       Heuristic struct — 3-part template (when/verify/avoid)
├── mel_store.go           VectorStore — BM25+TF-IDF+cosine hybrid, 500-entry cap
├── mel_analyzer.go        BifurcationAnalyzer — observes failures → heuristics
├── mel_bm25.go            BM25 scoring implementation
├── mel_tfidf.go           TF-IDF scoring implementation
├── cci_layer.go           CCILayer — top-level CCI API (Hot + Specialist + Cold)
├── cci_hot_memory.go      HotMemory — Tier 1 conventions and subsystem index
├── cci_specialist.go      SpecialistManager — Tier 2 domain personas
├── cci_cold_memory.go     ColdMemoryStore — Tier 3 subsystem specifications
├── cci_drift_detector.go  DriftDetector — Truth Sentry file hash comparison
└── cci_tier.go            Tier type constants and formatting helpers
```

### ARC Router

`ARCRouter` classifies every incoming prompt into a `WorkflowType` and computes a `ResourceBudget` calibrated to the host's RAM profile via `HALProfile`:

```go
type RouteDecision struct {
    Classification WorkflowType    // WorkflowConversational, WorkflowFactual,
                                   // WorkflowAnalytical, WorkflowReasonVerify, etc.
    Budget         ResourceBudget  // MaxTokens, Temperature, MaxToolCalls, Timeout
    Confidence     float64         // 0.0 (tie) → 1.0 (unambiguous)
    LowConfidence  bool            // true → escalated to WorkflowAnalytical
}
```

`SemanticClassifier` can operate in two modes:
- **Keyword scoring** (default) — weighted phrase matching against `arc_categories.go` signal table
- **Semantic mode** — cosine similarity over dense embeddings when an `embeddings.Embedder` is wired via `ARCRouter.SetEmbedder()`

`IntentCategory` classification (9 categories: auto, deep, quick, visual, research, security, code, creative, data, plan) feeds `provider_routing.go` for fine-grained model selection.

**Entropy guard:** when `confidence < 0.25`, the workflow is conservatively escalated to at least `WorkflowAnalytical`. This prevents silent downgrading of ambiguous prompts.

### MEL (Meta-Experience Learning)

The MEL system converts tool failure→correction cycles into persistent heuristics that steer the model away from previously observed failure modes.

```
BifurcationAnalyzer
  ObserveFailed(tool, params, errMsg) → records attempt
  ObserveSuccess(tool, params)        → if attempt existed, crystallises heuristic
                                        → VectorStore.Add(heuristic)

VectorStore
  Query(prompt, k=5) → []*Heuristic (most relevant to current prompt)
  Scoring: 0.5×cosine + 0.5×(0.6×BM25 + 0.4×TF-IDF) × confidence × log(1+useCount)
  Dedup: TF-IDF cosine > 0.75 merges entries
  Eviction: lowest-confidence entry removed when > 500 heuristics
  Persistence: ~/.config/gorkbot/vector_store.json
```

Each heuristic follows the template: "When [context], verify [constraint], avoid [error]."

### CCI (Codified Context Infrastructure)

Three-tier persistent project memory managed by `CCILayer`:

```
Tier 1 — Hot Memory (~/.config/gorkbot/cci/hot/)
  CONVENTIONS.md          — always-loaded project conventions
  SUBSYSTEM_POINTERS.md   — index of subsystems → specialist domains
  BuildBlock()            → injected into every system prompt

Tier 2 — Specialist (~/.config/gorkbot/cci/specialists/)
  <domain>.md             — on-demand domain personas with failure-mode tables
  Triggered by ARC trigger table pattern matching on file path / task
  SynthesizeSpecialist()  — MEL can auto-create specialists from cold content

Tier 3 — Cold Store (~/.config/gorkbot/cci/docs/)
  <subsystem>.md          — on-demand subsystem specifications
  Queryable via mcp_context_* tools (mcp_context_get, mcp_context_update, etc.)
  SuggestSpecialist()     — keyword scoring to find the best cold doc

Truth Sentry (DriftDetector)
  Compares file hashes in hot memory against live files
  Injected as warning prefix when stale documentation is detected
  RunDriftCheck(cwd) → formatted warning string
```

**CCI gap handling:** when `mcp_context_get_subsystem` returns empty, `CCILayer.HandleGap()` auto-switches `ModeManager` to PLAN mode and notifies the user.

---

## 6. Tool Execution Pipeline

**Package:** `pkg/tools`

### Registry Architecture

```
Registry
  ├─ tools map[string]Tool
  ├─ permissionMgr *PermissionManager  (tool_permissions.json)
  ├─ analytics *Analytics              (SQLite)
  ├─ auditDB *AuditDB                  (SQLite structured log)
  ├─ senseTracer senseTracerIface      (daily JSONL trace files)
  ├─ inputSanitizer inputSanitizerIface(SENSE stabilization middleware)
  ├─ envSnapshot envSnapshotReader     (capability pre-flight)
  ├─ disabledCategories map[ToolCategory]bool
  ├─ schedulerInst *scheduler.Scheduler
  └─ persistStore *persist.Store
```

### Tool Interface (`tool.go`)

```go
type Tool interface {
    Name() string
    Description() string
    Category() ToolCategory
    Parameters() json.RawMessage  // JSON Schema
    RequiresPermission() bool
    DefaultPermission() PermissionLevel
    OutputFormat() OutputFormat
    Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
}

// Optional — pre-flight capability check:
type CapabilityRequirer interface {
    RequiredBinaries() []string
    RequiredPythonPackages() []string
}
```

### Execution Pipeline (per tool call)

```
Registry.Execute(ctx, ToolRequest)
  1. normalizeToolName(req.ToolName)          — handle aliases/case
  2. NormalizeToolParams(name, params)        — normalize parameter names
  3. Check disabledCategories                 — returns error if category off
  4. CapabilityRequirer pre-flight             — binary/package check via envSnapshot
  5. InputSanitizer.SanitizeParams(params)    — control-char, path, resource validation
  6. validateRequiredParams(schema, params)   — required fields present
  7. checkPermission(name, params)            — RuleEngine → PermissionManager → HITL
  8. Inject context values (registry, scheduler, userCmdLoader, contextStats, etc.)
  9. tool.Execute(ctxWithExtras, normalizedParams)
  10. AuditDB.LogExecution (async goroutine)
  11. SENSETracer.LogToolSuccess/Failure (non-blocking channel send)
  12. Analytics.RecordExecution(name, success, duration)
  13. Return ToolResult
```

### Tool Packs

Tools are organized into named packs and activated by `GORKBOT_TOOL_PACKS`:

| Pack | Contents |
|------|----------|
| `core` | bash, structured_bash, read/write/edit/list/search/grep/delete file, context_stats, python_execute |
| `dev` | git (6), worktrees (4), Docker, k8s, AWS, ngrok, CI, code_exec, rebuild, code2world, ast_grep, hashed files |
| `web` | web_fetch, http_request, check_port, download_file, x_pull, web_search, web_reader, scrapling (5) |
| `sec` | nmap, masscan, nikto, gobuster, ffuf, sqlmap, hydra, hashcat, john, nuclei, metasploit, burp, impacket, tshark + 20 more |
| `media` | image, video, audio, ffmpeg, OCR, TTS, meme, docx/xlsx/pdf/pptx |
| `data` | CSV, plot, arxiv, web archive, whois, jupyter, AI image gen, ML model run, SQLite, PostgreSQL |
| `sys` | privileged_exec, processes, kill, env, system_info, disk, cron, backup, monitor, pkg_install, ADB, Android tools |
| `vision` | vision_screen, vision_file, vision_ocr, vision_find, vision_watch, ADB setup, frontend_design |
| `agent` | create_tool, modify_tool, list_tools, consultation, todo, engrams, SENSE tools, goals, pipeline, schedule, colony |
| `comm` | email, slack, calendar, contact_sync, smart_home |

Default active packs: `core,dev,web,sys,agent,data,media,comm`

### Permission System

Four levels (in ascending trust):

| Level | Behaviour |
|-------|-----------|
| `never` | Permanently blocked |
| `once` | Prompts before every execution |
| `session` | Approved for current session only |
| `always` | Permanent approval (persisted to `tool_permissions.json`) |

`RuleEngine` evaluates glob-pattern rules before the permission manager. Rules can `allow`, `ask`, or `deny` specific tool/parameter patterns.

**HITL Guard** (`sense_hitl.go`): wraps destructive tool calls — any tool with `once` or lower permission that matches a danger pattern triggers an explicit user confirmation callback before execution proceeds.

---

## 7. SENSE Module

**Package:** `pkg/sense`

SENSE (Self-Evolving Neural System Engine) provides quality guards, memory, and observability:

| Component | File | Purpose |
|-----------|------|---------|
| `SENSETracer` | `tracer.go` | Daily-rotated JSONL trace files; 7 event kinds; async non-blocking writes via 512-entry buffer |
| `InputSanitizer` | `input_sanitizer.go` | Validates tool parameters: control-char rejection, path sandboxing, resource-name validation |
| `LIEEvaluator` | `lie.go` | Reasoning-depth controller; prevents overly shallow responses |
| `Stabilizer` | `stabilizer.go` | Output quality gate; evaluates AI response coherence |
| `Compressor` | `compression.go` | Context compression when token budget is exceeded |
| `AgeMem` | `agemem.go` | Age-stratified episodic memory; hot (in-session) / cold (cross-session) |
| `EngramStore` | `engrams.go` | Persistent behaviour preferences recorded by the agent |
| `SkillEvolver` | `skill_evolver.go` | Monitors skill usage patterns, synthesises improved skill variants |
| `TraceAnalyzer` | `trace_analyzer.go` | Analyses JSONL trace files; surfaces patterns, failure correlations |

SENSE event kinds written to trace files:
- `tool_success` — successful tool execution
- `tool_failure` — tool error or false result
- `hallucination` — detected model fabrication
- `context_overflow` — token limit exceeded
- `sanitizer_reject` — input sanitizer blocked a parameter
- `provider_error` — transient AI provider failure
- `param_error` — missing required parameter

---

## 8. Memory Systems

Multiple memory subsystems provide short-term, long-term, and cross-session recall:

### AgeMem (Episodic Memory)

`sense.AgeMem` implements a two-tier store:
- **Hot (in-session):** recent facts decay quickly; LRU eviction
- **Cold (cross-session):** persisted to `~/.config/gorkbot/`; recalled at session start

### Engrams (Preference Memory)

`sense.EngramStore` persists explicit behaviour preferences recorded by the AI via the `record_engram` tool. Engrams are injected into every system prompt at session start.

### Goal Ledger

`memory.GoalLedger` tracks open and closed goals across sessions. Goals survive restarts and can be queried by the AI via `list_goals` / `close_goal` tools.

### Unified Memory

`memory.UnifiedMemory` provides a single retrieval façade over AgeMem, EngramStore, and the MEL VectorStore. The orchestrator queries it at context construction time.

### Conversation VectorStore

`vectorstore.VectorStore` indexes full conversation turns for semantic retrieval. `RAGInjector` queries this store each turn and prepends the k most similar historical turns to the current user message.

### Session Checkpoints

`session.CheckpointManager` saves up to 20 snapshots of `ConversationHistory` per session. `/rewind [last|<id>]` restores any checkpoint.

### SQLite Persistence

`persist.Store` records full conversation history and tool call analytics in a per-session SQLite database. The `session_search` tool queries this store across past sessions.

---

## 9. DAG Execution Engine

**Package:** `pkg/dag`

The DAG engine provides dependency-resolved parallel task execution for multi-step agentic workflows:

```
dag.go          — Graph structure, topological sort (Kahn's algorithm)
executor.go     — Parallel execution (semaphore goroutine pool, default 4 workers)
                  Retry with exponential backoff; RCA provider call on exhaustion
rollback.go     — RollbackStore: snapshots files before modification
                  On failure: restores originals (temp→rename); removes new files
resolver.go     — Dependency resolution; cycle detection
pruner.go       — Compresses verbose tool output before LLM injection
state.go        — Gob-encoded persistent state; interrupted graphs resume from checkpoint
```

The TUI renders a live DAG view with per-task progress bars, elapsed timers, dependency graphs, and RCA panels.

---

## 10. Subagent System

**Package:** `pkg/subagents`

Gorkbot supports spawning isolated sub-agents for parallel or depth-isolated work:

- `spawn_agent` / `spawn_sub_agent` — depth-limited (max 4 levels) sub-agent delegation with discovery-aware model selection
- `worktree isolation` — `isolated=true` parameter creates a git worktree, prepends the path to the task, auto-removes after completion (15-minute timeout)
- `WorktreeManager` — creates, lists, and removes git worktrees
- `SecurityContext` — shared session state for red team agents (findings, scope, severity)
- `BackgroundAgentManager` — runs parallel sub-agents as goroutines with result channels

---

## 11. Terminal UI

**Package:** `internal/tui`

The TUI follows the Elm architecture (Model → View → Update):

```
model.go              — State (conversation, input buffer, viewport, overlays)
update.go             — Event handling (keyboard, tokens, commands, messages)
view.go               — Rendering (markdown, consultant boxes, status bar, overlays)
style.go              — Lip Gloss styles and theme application
messages.go           — Custom Bubble Tea message types
keys.go               — Key binding definitions
statusbar.go          — Context %, cost, mode, git branch
list.go               — Model selection list items
model_select_view.go  — Dual-pane model selection UI
api_key_prompt.go     — Modal overlay for key entry
settings_overlay.go   — 4-tab settings modal
discovery_view.go     — Cloud Brains tab (discovered models + agent tree)
model_extensions.go   — SetDiscoveryManager, SetExecutionMode, UpdateContextStats
```

### Tabs

| Tab | Key | Content |
|-----|-----|---------|
| Chat | (default) | Conversation, input, status bar |
| Models | `Ctrl+T` | Dual-pane model selection (primary/specialist) |
| Tools | `Ctrl+E` | Tool list and analytics |
| Cloud Brains | `Ctrl+D` | Live discovered model list + agent delegation tree |
| Diagnostics | `Ctrl+\` | System diagnostic snapshot |

### Settings Overlay (`Ctrl+G`)

Four tabs:

1. **Model Routing** — switch primary/specialist, view current selection
2. **Verbosity** — verbose thoughts, debug mode
3. **Tool Groups** — enable/disable tool categories by pack
4. **API Providers** — toggle individual providers for the session

---

## 12. Integrations

### MCP (Model Context Protocol)

`pkg/mcp` implements the full MCP stdio transport with JSON-RPC 2.0:

- `Manager.LoadAndStart(ctx)` — reads `mcp.json`, spawns subprocesses, performs capability handshake
- `Manager.RegisterTools(registry)` — wraps each server's tools as `mcp_<server>_<toolname>` Gorkbot tools
- `Manager.Reload(ctx, registry)` — live reload without restart
- `/mcp reload` command triggers reload

### A2A HTTP Gateway

`pkg/a2a` implements an HTTP task server for inter-agent delegation:

- Listens on `--a2a-addr` (default `127.0.0.1:18890`)
- Accepts JSON-RPC style `{task, context}` POST requests
- Routes tasks through the orchestrator and returns the result

### Telegram Bot

`pkg/channels/telegram` runs a Telegram bot configured via `~/.config/gorkbot/telegram.json`. Messages are routed through the orchestrator with full tool access.

### SSE Session Sharing

`pkg/collab` provides:
- `Relay` — SSE server that broadcasts tokens and tool events to observers
- `Observer` — connects to a relay and renders the session read-only

### Adaptive Router

`pkg/router` maintains a JSONL-persisted feedback history. `/rate 1-5` records satisfaction with the last response. `FeedbackManager.SuggestModel(category)` recommends the best-performing model for each task category based on accumulated ratings.

---

## 13. Local LLM Embedding

**Package:** `internal/llm`, `pkg/embeddings`

The `pkg/embeddings` package defines a provider-agnostic `Embedder` interface:

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Dims() int
    Name() string
}
```

Three backends:

| Backend | Package | Notes |
|---------|---------|-------|
| llamacpp (local) | `internal/llm` | C++ bridge to `ext/llama.cpp`; requires `-tags llamacpp` build |
| Ollama (local network) | `pkg/embeddings/ollama.go` | HTTP to local Ollama server |
| Cloud | `pkg/embeddings/cloud.go` | OpenAI `text-embedding-3-small` or Google `text-embedding-004` |

**Automatic fallback chain:** local llamacpp → cloud → pure keyword scoring.

The llamacpp build (`make build-llm`) compiles `internal/llm/cbridge/` against `ext/llama.cpp` into `internal/llm/libgorkbot_llm.a`. When the build tag is absent, `llm_stub.go` returns `ErrUnavailable` and the rest of the system degrades to BM25/TF-IDF.

The Nomic Embed Text v1.5 Q4_K_M model (`make download-nomic`) is stored at `~/.cache/llama.cpp/nomic-embed-text-v1.5.Q4_K_M.gguf` and produces 768-dimensional L2-normalized embeddings.

---

## 14. Data Flow: Single Turn

```
User types message → TUI input buffer
  → Enter key → TeaCmd → OrchestratorMsg
  → Orchestrator.ExecuteTaskWithTools(ctx, prompt)
       │
       ├─ CCI.BuildSystemContext(prompt)
       │   ├─ DriftDetector.Check()
       │   ├─ HotMemory.BuildBlock()
       │   └─ loadSpecialistForPrompt()
       │
       ├─ Intelligence.Route(prompt) → RouteDecision
       │
       ├─ Intelligence.HeuristicContext(prompt) → MEL heuristics
       │
       ├─ ConversationHistory.AddUserMessage(systemPrompt + userPrompt)
       │
       ├─ [optional] Consultant.StreamWithHistory() → advice → prepend to context
       │
       ├─ Primary.StreamWithHistory() or GenerateWithTools()
       │   → tokens stream → SENSETracer + Stabilizer
       │   → ParseToolRequests or structured tool_calls
       │
       ├─ [for each tool]:
       │   Sanitize → preflight → permission → cache → Execute → Audit → Trace → MEL
       │
       ├─ ConversationHistory updated with tool results
       │
       ├─ [repeat AI + tools up to maxTurns]
       │
       └─ Final response → TUI TokenMsg stream → Glamour markdown render
```

---

## 15. Package Dependency Graph

```
cmd/gorkbot/main.go
  ├─ internal/engine
  │   ├─ pkg/adaptive          (ARC Router + MEL + CCI)
  │   ├─ pkg/ai                (AIProvider implementations)
  │   ├─ pkg/tools             (tool registry + all tools)
  │   ├─ pkg/sense             (SENSE guards + tracer)
  │   ├─ pkg/memory            (AgeMem, Engrams, GoalLedger, UnifiedMemory)
  │   ├─ pkg/session           (checkpoints, exporter, workspace)
  │   ├─ pkg/billing           (cost tracking)
  │   ├─ pkg/collab            (SSE relay)
  │   ├─ pkg/router            (adaptive routing)
  │   ├─ pkg/discovery         (model discovery)
  │   ├─ pkg/hooks             (lifecycle hooks)
  │   ├─ pkg/config            (GORKBOT.md + AppState)
  │   ├─ pkg/skills            (skill loader)
  │   ├─ pkg/subagents         (sub-agent + worktree)
  │   ├─ pkg/dag               (DAG executor)
  │   ├─ pkg/persist           (SQLite)
  │   └─ pkg/vectorstore       (RAG)
  │
  ├─ internal/tui
  │   ├─ pkg/commands          (slash command registry)
  │   └─ pkg/tui               (Stylist — Lip Gloss helpers)
  │
  ├─ pkg/mcp                   (MCP client)
  ├─ pkg/a2a                   (A2A gateway)
  ├─ pkg/channels/telegram
  ├─ pkg/providers             (KeyStore + ProviderManager)
  ├─ pkg/registry              (ModelRegistry)
  ├─ pkg/scheduler             (cron scheduler)
  ├─ pkg/process               (managed processes)
  └─ internal/platform         (env detection, Version)
```

**Import cycle prevention:** `internal/engine` must not import `internal/tui`. Communication flows through `OrchestratorAdapter` (function references) in `pkg/commands`. The `internal/tui` package receives updates via typed Bubble Tea messages (`TokenMsg`, `ContextUpdateMsg`, `ToolProgressMsg`, etc.).

---

## 16. pkg/adaptive — Consolidated Intelligence Package

This section serves as an addendum documenting the consolidation introduced in v4.7.0, where the previously separate packages `internal/arc`, `internal/mel`, and `pkg/cci` were unified into `pkg/adaptive`.

### Motivation

Prior to this consolidation:
- `internal/arc` held ARC Router + budget + classifier + consistency
- `internal/mel` held MEL heuristic store + bifurcation analyzer
- `pkg/cci` held the CCI three-tier memory layer

These packages had growing interdependencies (MEL feeding into CCI specialists; ARC trigger table used by CCI; MEL embedder shared with ARC semantic mode) that created an increasingly tangled import graph. Moving all three into `pkg/adaptive` makes the dependencies explicit within a single package and provides a clean, versioned public API for the orchestrator.

### Public API

The engine accesses `pkg/adaptive` exclusively through the `IntelligenceLayer` struct in `internal/engine/intelligence.go`:

```go
// Intelligence layer construction (called from InitIntelligence):
NewIntelligenceLayer(hal platform.HALProfile, configDir string) (*IntelligenceLayer, error)

// Routing (called before each AI turn):
il.Route(prompt string) adaptive.RouteDecision

// Heuristics injection (first turn of each task):
il.HeuristicContext(prompt string) string

// MEL observation (after each tool execution):
il.Analyzer.ObserveFailed(tool, params, errMsg string)
il.Analyzer.ObserveSuccess(tool, params string)

// CCI context construction (at system message build time):
orch.CCI.BuildSystemContext(prompt string) string
orch.CCI.HandleGap(subsystem string, modeManager ModeManagerIface) string
orch.CCI.RunDriftCheckDefault() string
```

### Embedder Integration

When a local embedder (llamacpp) or cloud embedder (OpenAI/Google) is configured, main.go calls:

```go
il.Router.SetEmbedder(embedder)         // ARC semantic routing
il.Store.SetEmbedder(embedder)          // MEL semantic retrieval
```

This upgrades both the router's classification and the heuristic store's query from pure keyword scoring to cosine-similarity-dominant hybrid scoring without any change to the calling code.

### IntentCategory vs WorkflowType

The package exposes two distinct classification dimensions:

- **`WorkflowType`** (used by `ARCRouter.Route`) — controls execution path: `WorkflowConversational`, `WorkflowFactual`, `WorkflowAnalytical`, `WorkflowReasonVerify`
- **`IntentCategory`** (used by `ClassifyIntent`) — semantic domain: `deep`, `quick`, `visual`, `research`, `security`, `code`, `creative`, `data`, `plan`

`WorkflowType` drives the compute budget. `IntentCategory` drives model selection (routing specific task types to the best-suited model variant).
