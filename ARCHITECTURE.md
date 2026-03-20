# System Architecture Reference

Comprehensive guide to Gorkbot's architecture, components, and design patterns.

---

## Table of Contents

1. [High-Level Architecture](#high-level-architecture)
2. [Core Components](#core-components)
3. [Intelligence Layers](#intelligence-layers)
4. [Tool System](#tool-system)
5. [Data Flow](#data-flow)
6. [Subsystems](#subsystems)
7. [Integration Points](#integration-points)

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────┐
│              User Interface Layer                │
│  TUI (Bubble Tea) | Web UI (Gin) | CLI          │
└────────────┬────────────────────────────────────┘
             │
┌────────────▼────────────────────────────────────┐
│          Orchestrator (Core Routing)             │
│  Manages conversation, providers, permissions   │
└────────────┬────────────────────────────────────┘
             │
     ┌───────┴───────────────────────────────┐
     │                                       │
┌────▼──────────────────┐  ┌────────────────▼──┐
│ Intelligence Layers   │  │ Supporting Systems│
│ - SENSE (Stability)   │  │ - Cache System    │
│ - SPARK (Reasoning)   │  │ - Persistence     │
│ - SRE (Validation)    │  │ - Permissions     │
│ - XSKILL (Learning)   │  │ - Audit Logging   │
│ - ARC (Routing)       │  │ - HITL Approval   │
│ - MEL (Memory)        │  │                   │
└────┬──────────────────┘  └────────┬──────────┘
     │                              │
┌────▼──────────────────────────────▼──────────┐
│         Tool Execution Engine                │
│  Registry | Dispatcher | Permission Manager  │
└────┬────────────────────────────────────────┘
     │
┌────▼──────────────────────────────────────────┐
│     AI Provider Layer (Multi-Provider)        │
│  Grok | Gemini | Claude | OpenAI | Custom    │
└────────────────────────────────────────────────┘
```

---

## Core Components

### 1. Orchestrator (`internal/engine/orchestrator.go`)

**Purpose**: Central coordinator for all AI interactions

**Key Responsibilities**:
- Manage primary + secondary AI providers
- Maintain conversation history with full-text search
- Coordinate tool execution with permission validation
- Track context/token usage with auto-compaction
- Route between reasoning systems (SPARK, SRE, etc.)

**Key Methods**:
- `ExecuteTask()`: Synchronous task execution
- `ExecuteTaskWithStreaming()`: Streaming token delivery
- `SetProvider()`: Switch active provider
- `ObserveSuccess/Failure()`: Feedback for learning
- `Interrupt()`: Context cancellation

**State Management**:
- ConversationHistory: Full message history
- SessionID: Current session identifier
- PersistStore: SQLite database handle
- CancelFunc: Context cancellation (sync.Mutex-protected)

### 2. Conversation History (`pkg/ai/conversation.go`)

**Purpose**: In-memory conversation management with full-text search

**Structure**:
```go
type ConversationHistory struct {
    messages []*Message
    mu sync.RWMutex
}

type Message struct {
    Role string              // "user" | "assistant" | "system"
    Content string           // Message text
    Metadata map[string]interface{}  // Attachments, tool data, etc.
}
```

**Operations**:
- Append messages (thread-safe)
- Query by role or content
- Full-text search capability
- Compression (via SENSE Compressor)
- Export/import for persistence

### 3. Tool Registry (`pkg/tools/registry.go`)

**Purpose**: Manage all available tools and their execution

**Key Components**:
- **Tool Map**: map[string]Tool indexed by name
- **Permission Manager**: Per-tool approval levels
- **Analytics**: Usage statistics + cost tracking
- **Audit Database**: SQLite log of all executions
- **Dispatcher**: Parallel tool execution with semaphore

**Tool Interface**:
```go
type Tool interface {
    Name() string
    Description() string
    Category() ToolCategory
    Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
}
```

**Execution Flow**:
1. Parse tool invocation from AI response
2. Validate tool exists in registry
3. InputSanitizer validates parameters
4. PermissionManager checks approval level
5. AuditDB logs request
6. Tool.Execute() runs with context
7. SENSETracer logs result
8. MEL observer records pattern

---

## Intelligence Layers

### 4. SENSE (Stability & Enlightened Stabilization)

**Components**:

| Component | Purpose |
|-----------|---------|
| **InputSanitizer** | Validate paths, ANSI codes, SQL patterns |
| **LIE** | Detect neural hallucinations |
| **Stabilizer** | Exponential moving average of metrics |
| **Compressor** | SENSE-aware context compression |
| **AgeMem** | Decay older memories by frequency |
| **EngramStore** | Persistent episodic memories |
| **TraceAnalyzer** | Real-time JSONL trace parsing |
| **SENSETracer** | Daily-rotated event logging |
| **OutputFilter** | Category-based message suppression |

**Key Features**:
- Prevents prompt injection
- Detects factual hallucinations
- Auto-compresses when 85% context filled
- 63+ event types traced
- 7 message categories filterable

### 5. SPARK (Self-Propelling Autonomous Reasoning Kernel)

**8-Step Autonomous Cycle**:

1. **TII** (Task Introspection): Analyze just-completed task
2. **IDL** (Improvement Debt): Track improvements needed
3. **LIE Integration**: Hallucination feedback
4. **MotivationalCore**: Compute drive metric (0-100)
5. **ResearchModule**: Formulate study objectives
6. **DiagnosisKernel**: Analyze trace patterns
7. **Introspector**: Self-reflection on limitations
8. **Phase Output**: Generate directives for next session

**Wiring**: Triggered post-task via `ObserveSuccess()`, non-blocking

### 6. SRE (Step-wise Reasoning Engine)

**4-Phase Hypothesis Testing**:

1. **Ground**: Extract world model from prompt
2. **Hypothesis**: Generate N candidate hypotheses
3. **Test**: Formulate test for each hypothesis
4. **Validate**: Confirm best hypothesis via sampling

**Components**:
- GroundingExtractor: Structure world model
- AnchorLayer: Pin facts in memory
- CoSEngine (Chain of Schedules): Phase transitions
- CorrectionEngine: Detect deviation from grounding
- EnsembleManager: Multi-trajectory voting

### 7. XSKILL (Continual Learning)

**3-Phase System**:

**Phase 1 - Accumulation** (post-task, async):
- Capture tactical patterns
- Store in Experience Bank (JSON)
- Generate skill documents (markdown)
- Location: `~/.gorkbot/xskill_kb/`

**Phase 2 - Inference** (pre-generation):
- Retrieve similar experiences
- Embed via LLMProvider
- Inject context: "Previously, for X tasks..."

**Phase 3 - Hot-Swap**:
- Upgrade embedder at runtime
- Support native LLM (llamacpp)

### 8. ARC Router (Adaptive Reasoning Classifier)

**Classification Flow**:

```
Input prompt
  ↓
IngressFilter (prune low-info)
  ↓
IngressGuard (verify semantic preservation)
  ↓
ARC Classifier
  - TF-IDF vectorization
  - Historical feature scoring
  - Confidence computation
  - Action classification (NORMAL/PLAN/AUTOEDIT/SECURITY)
  ↓
If confidence < 0.25: Use RoutingTable fallback
  ↓
Dispatch to appropriate handler
```

**Confidence Guard**: Only routes if confidence ≥ 0.25

### 9. MEL (Multi-Evidential Learning)

**Learning Flow**:

```
Success observed
  ↓
MELValidator (semantic sanity check)
  ↓
VectorStore (SQLite + embedding)
  - Embed: "(tool=bash, params={...}, result=success)"
  - BM25 + TFIDF ranking
  ↓
MEL Heuristic
  - Pre-compute success patterns
  - "bash + git_push + small_diff → high_success"
  ↓
MELProjector
  - Project prompts into vector space
  - Find similar past successes
  - Inject as "reasoning examples"
```

### 10. CCI (Codified Context Infrastructure)

**3-Tier Memory**:

| Tier | Scope | Priority | Persistence |
|------|-------|----------|-------------|
| **Hot** | Current session | Highest | In-memory |
| **Specialist** | Domain-specific | Medium | Disk (per-domain) |
| **Cold** | Historical patterns | Lower | Disk (historical) |

**Truth Sentry**: Monitors semantic drift between tiers

---

## Tool System

### 28+ Integrated Tools

**Categories**:

1. **Bash** (1): shell command execution
2. **File** (7): read, write, list, search, grep, info, delete
3. **Git** (6): status, diff, log, commit, push, pull
4. **Web** (6): fetch, http_request, check_port, download, scrape, control
5. **System** (6): list_processes, kill, env_var, info, disk_usage
6. **Security** (32+): nmap, sqlmap, nuclei, totp, etc.
7. **Meta** (3): list_tools, tool_info, create_tool

### Tool Execution Pipeline

```
AI response with tool JSON
  ↓
Registry.Parse() - Extract tool name & params
  ↓
InputSanitizer.Validate() - Check paths, SQL, etc.
  ↓
PermissionManager.Check() - Permission level (once/session/always/never)
  ↓
HITL Gate - High-stakes approval if needed
  ↓
AuditDB.Log() - Record request
  ↓
Tool.Execute(ctx) - Run tool
  ↓
SENSETracer.Log() - Record result
  ↓
MEL.Observe() - Learning record
  ↓
StreamCallback - Token delivery to TUI
```

---

## Data Flow

### Request-Response Cycle

```
User Input
  ↓
InputSanitizer validates
  ↓
IngressFilter prunes context
  ↓
Build system prompt (CCI Hot+Specialist+Cold)
  ↓
XSKILL context enrichment
  ↓
Cache Advisor computes caching hints
  ↓
Call provider with streaming
  ↓
Tokens streamed to TUI
  ↓
MessageSuppressor filters if not verbose
  ↓
Tool detection from response
  ↓
HITL approval gate (if high-stakes)
  ↓
Tool execution
  ↓
MEL learning record
  ↓
SRE validation
  ↓
SPARK post-task cycle
  ↓
Save to ConversationHistory
  ↓
Save to SQLite (gorkbot.db)
```

---

## Subsystems

### Caching (`pkg/cache/`)

**Per-Provider Strategies**:

| Provider | Floor | TTL | Format |
|----------|-------|-----|--------|
| Anthropic | 1024 tokens | 5 mins | prompt_cache_control |
| Gemini | 512 tokens | 1 hour | cachedContents API |
| Grok | 2048 tokens | Session | X-Grok-Cache-ID |
| OpenAI | 1024 tokens | 24 hours | cache_control |

**Advisor Flow**: Runs AFTER XSKILL injection (correct ordering)

### Persistence (`pkg/persist/`)

**SQLite Schema** (migrations v1–v6):
- conversations: FTS5 indexed
- tool_calls: Audit log
- memories: Fact storage
- sessions: Full-text search
- hitl_decisions: Approval history
- sessions_fts: FTS5 virtual table

### Permissions System (`pkg/tools/permissions.go`)

**Levels**:
- `always`: Permanent, saved to disk
- `session`: Until session ends
- `once`: Ask every time
- `never`: Permanently blocked

**Storage**: `~/.config/gorkbot/tool_permissions.json`

### HITL Approval (`pkg/hitl/`)

**4-Level Risk Classification**:
- Low: File reading
- Medium: File modification
- High: File deletion, git ops
- Critical: Bash, package installation

**Auto-Approval Criteria**:
- Confidence ≥ 85% AND precedent ≥ 2
- Critical never auto-approved

---

## Integration Points

### MCP Servers (`pkg/mcp/`)

12+ supported servers:
- gorkbot-introspect
- gorkbot-android
- gorkbot-termux
- file-system
- postgres
- git
- NotebookLM (OAuth)
- etc.

**Bus**: Unix domain socket `~/.config/gorkbot/mcp_bus.sock`

### Channel Bridges (`pkg/channels/`)

- **Discord**: Bot token auth, streaming edits
- **Telegram**: Polling + webhook, message splitting
- **Webhooks**: HMAC validation, router dispatch

### Configuration Injection (`GORKBOT.md`)

Auto-discovered from project root:
- Loaded on session start
- Injected into system prompt
- Watched for live changes
- Persisted but not committed

---

## Design Patterns

### Interfaces Over Concretions

```go
// AIProvider interface - any LLM can implement
type AIProvider interface {
    Generate(ctx context.Context, prompt string) (string, error)
    StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error
    // ...
}

// Tool interface - any tool can implement
type Tool interface {
    Name() string
    Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
}
```

### Nil-Safe Optional Components

```go
// SPARK is optional - nil-safe
if orch.SPARK != nil {
    orch.SPARK.Trigger()  // Only if enabled
}

// SRE is optional
if orch.SRE != nil {
    orch.SRE.Ground(ctx, prompt)  // Only if enabled
}
```

### Middleware/Callback Pattern

```go
// Streaming middleware chain
suppressor.ProcessStreamingToken(token)
  → ThinkingCallback.Extract()
    → TUICallback.Update()
      → SENSETracer.LogToken()
```

---

## Performance Characteristics

| Operation | Typical Time |
|-----------|-------------|
| Token generation (streaming) | 50-100 tokens/sec |
| Bash execution | <100ms |
| File read | <50ms |
| Git operation | <500ms |
| Web fetch | 1-5s |
| Context compression | 100-500ms |
| Tool dispatch | <10ms |

---

**For implementation details, see [DEVELOPMENT.md](DEVELOPMENT.md).**

