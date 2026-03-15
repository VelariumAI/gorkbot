# Gorkbot Enhancement Roadmap
## Strategic Technical Analysis — February 2026

> **Mission**: Learn from best-in-class AI CLI architectures to dramatically elevate Gorkbot's power, reliability, and user experience. This is not imitation — it's informed engineering.

---

## Executive Summary

After deep analysis of Gorkbot's codebase and studying advanced AI CLI tool architectures, **17 high-impact enhancement areas** have been identified. These are organized by priority (P0=critical, P1=high, P2=medium, P3=nice-to-have) and grouped into implementation phases.

Gorkbot already has a strong foundation: dual-AI orchestration, SENSE framework, HITL guardrails, vision pipeline, subagent system, and a professional TUI. The gaps are in **context management**, **tool execution model**, **extensibility infrastructure**, and **session control primitives**.

---

## Phase 1: Critical Foundation (P0)

### 1.1 Parallel Tool Execution Engine

**Current State**: Tools execute sequentially. When the AI requests multiple tools, they run one-by-one.

**Problem**: Major throughput bottleneck. Reading 5 files takes 5× as long as it should. A subagent that needs to read 10 files, check 3 ports, and fetch 2 URLs does all of this linearly.

**Solution**: Concurrent tool dispatcher with dependency analysis.

**Implementation**:
```
pkg/tools/dispatcher.go
```

```go
// ToolDispatcher runs independent tools concurrently
type ToolDispatcher struct {
    registry *Registry
    maxWorkers int
}

// DispatchBatch executes a batch of ToolRequests.
// Tools with no data dependencies run concurrently.
// Tools that declare dependencies run after their deps complete.
func (d *ToolDispatcher) DispatchBatch(ctx context.Context, reqs []ToolRequest) []ToolResult {
    // 1. Build dependency graph from request metadata
    // 2. Identify independent sets (topological layers)
    // 3. Execute each layer concurrently with semaphore
    // 4. Feed results to next layer
    // 5. Return ordered results matching input order
}
```

**AI Prompt Engineering**: The system prompt should instruct the AI to batch independent tool requests in a single response using a JSON array, signaling which are independent:
```json
[
  {"tool": "read_file", "parameters": {"path": "a.go"}, "id": "t1"},
  {"tool": "read_file", "parameters": {"path": "b.go"}, "id": "t2"},
  {"tool": "bash", "parameters": {"command": "git status"}, "id": "t3"}
]
```

**Impact**: 3-10× speedup for multi-file operations. Transforms agentic loops from linear to parallel pipelines.

**Files to modify**: `pkg/tools/parser.go`, `pkg/tools/registry.go`, `internal/engine/orchestrator.go`

---

### 1.2 Context Window Tracking & Auto-Compaction

**Current State**: SENSE Compressor exists (`pkg/sense/compression.go`) but there is no tracking of how full the context window is. The `/compress` command exists but is manual.

**Problem**: Silent context overflow causes AI to "forget" earlier instructions without warning. Users experience degraded responses without knowing why.

**Solution**: Live context token tracking with automatic compaction trigger and visible UI indicator.

**Implementation**:
```
internal/engine/context_manager.go
internal/tui/statusbar.go (update)
```

```go
type ContextManager struct {
    maxTokens    int     // Model's context limit (from provider metadata)
    inputTokens  int     // Current usage (updated after each API call)
    compactPct   float64 // Trigger threshold (default 0.90)
    onNearFull   func()  // Callback to trigger compaction
}

// UpdateFromResponse parses token usage from API response headers/body
func (cm *ContextManager) UpdateFromResponse(usage TokenUsage) {
    cm.inputTokens = usage.InputTokens
    if cm.UsedPct() > cm.compactPct {
        cm.onNearFull()
    }
}

// UsedPct returns 0.0-1.0 representing context fullness
func (cm *ContextManager) UsedPct() float64 {
    return float64(cm.inputTokens) / float64(cm.maxTokens)
}
```

**Status Bar Enhancement**: Display context % as a mini progress bar:
```
[●●●●●●●○○○] 73% ctx | grok-3 | 12.4k tokens
```

**Auto-compaction**: When context exceeds threshold, invoke SENSE Compressor automatically and notify user with status message: "Context compacted (73% → 24%)".

**Files to modify**: `pkg/ai/grok.go`, `internal/tui/statusbar.go`, `internal/engine/orchestrator.go`

---

### 1.3 Session Checkpoint & Rewind System

**Current State**: No checkpointing. Tool execution is irreversible from the session perspective. If the AI makes a bad decision, you can only start over.

**Problem**: Irreversible mistakes — wrong file edits, incorrect bash commands, bad git commits — require manual rollback. High-stakes agentic work is unnecessarily risky.

**Solution**: Automatic session checkpoints before each tool execution, with `/rewind` command to restore.

**Implementation**:
```
pkg/session/checkpoint.go
```

```go
type Checkpoint struct {
    ID          string
    Timestamp   time.Time
    Description string          // e.g., "Before: bash(rm -rf build/)"
    History     []ConversationMessage
    ToolState   map[string]interface{} // Serializable tool state snapshots
}

type CheckpointManager struct {
    checkpoints []Checkpoint
    maxHistory  int // Keep last N checkpoints (default 20)
    storePath   string
}

// SaveCheckpoint creates a snapshot before a tool executes
func (cm *CheckpointManager) SaveCheckpoint(desc string, history []ConversationMessage) string

// Rewind restores to checkpoint by ID (or "last" for most recent)
func (cm *CheckpointManager) Rewind(id string) (*Checkpoint, error)

// List returns all checkpoints with IDs and descriptions
func (cm *CheckpointManager) List() []CheckpointSummary
```

**TUI Integration**: Add `/rewind` command. On rewind:
1. Restore conversation history
2. Show "Rewound to: Before bash(rm -rf build/)" in status
3. Clear viewport, re-render history

**Files**: New `pkg/session/checkpoint.go`, update `pkg/commands/registry.go`, `internal/engine/orchestrator.go`

---

### 1.4 Plan Mode (Read-Only Gate)

**Current State**: HITL system gates individual high-risk tools reactively. No proactive planning mode.

**Problem**: No way to say "analyze the codebase and plan what you'll do, don't change anything yet." Users have to trust the AI blindly or use verbose HITL approvals.

**Solution**: A formal Plan Mode that blocks all write/execute tools, letting the AI research and propose a structured plan before the user approves execution.

**Implementation**:
```
internal/engine/plan_mode.go
internal/tui/update.go (mode cycling)
```

```go
type ExecutionMode int
const (
    ModeNormal    ExecutionMode = iota // All tools available
    ModeAutoEdit                        // Auto-approve non-destructive edits
    ModePlan                            // Read-only: block write/exec/git-write tools
)

// WriteDeniedInPlanMode: list of tools blocked in plan mode
var WriteDeniedInPlanMode = map[string]bool{
    "write_file": true, "edit_file": true, "delete_file": true,
    "bash": true, "git_commit": true, "git_push": true,
    "git_pull": true, "kill_process": true,
}
```

**Keybinding**: `Ctrl+P` or `Shift+Tab` cycles through modes: `[NORMAL] → [PLAN] → [AUTO] → [NORMAL]`

**Mode Indicator**: Status bar shows colored badge:
- `[PLAN]` — amber, means "research only"
- `[AUTO]` — green, means "auto-approve edits"
- `[NORMAL]` — dim, default

**System Prompt Injection**: In Plan Mode, prepend to system: "You are in PLAN MODE. You may only use read/search tools. Before using any write or execute tool, you MUST present a structured plan and await user approval."

**Files**: New `internal/engine/plan_mode.go`, update `internal/tui/keys.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/engine/orchestrator.go`

---

## Phase 2: Extensibility Infrastructure (P1)

### 2.1 Lifecycle Hooks / Event Pipeline

**Current State**: No extensibility hooks. Hard to add pre/post processing without modifying core code.

**Problem**: Power users can't add custom validation, logging, or automation without recompiling. The tool system is closed.

**Solution**: An event-driven lifecycle hook system similar to git hooks — shell scripts that run at defined lifecycle points.

**Architecture**:
```
~/.config/gorkbot/hooks/
├── pre_tool_use.sh        # Runs before any tool executes
├── post_tool_use.sh       # Runs after any tool completes
├── session_start.sh       # Runs on startup
├── session_end.sh         # Runs on graceful exit
├── pre_compaction.sh      # Runs before context compaction
└── on_notification.sh     # Runs when AI surfaces a notification
```

**Protocol**: Hook scripts receive JSON on stdin:
```json
{
  "event": "pre_tool_use",
  "tool": "bash",
  "parameters": {"command": "rm -rf /"},
  "session_id": "sess_abc123",
  "timestamp": "2026-02-22T10:00:00Z"
}
```

**Exit codes**:
- `0` = Proceed
- `2` = Block (with reason on stderr)
- Other = Log warning, proceed

**Implementation**:
```
pkg/hooks/manager.go
pkg/hooks/runner.go
pkg/hooks/types.go
```

```go
type HookEvent string
const (
    EventSessionStart    HookEvent = "session_start"
    EventSessionEnd      HookEvent = "session_end"
    EventPreToolUse      HookEvent = "pre_tool_use"
    EventPostToolUse     HookEvent = "post_tool_use"
    EventPreCompaction   HookEvent = "pre_compaction"
    EventOnNotification  HookEvent = "on_notification"
    EventSubagentStart   HookEvent = "subagent_start"
    EventSubagentStop    HookEvent = "subagent_stop"
)

type HookManager struct {
    hooksDir string
    timeout  time.Duration // Default 10s per hook
}

func (hm *HookManager) Fire(ctx context.Context, event HookEvent, data map[string]interface{}) HookResult
```

**Security**: Hooks respect system permissions. They cannot escalate privilege. Hook scripts must be user-owned and not world-writable (mode check before execution).

**Files**: New `pkg/hooks/`, update `internal/engine/orchestrator.go`, `pkg/tools/registry.go`

---

### 2.2 Fine-Grained Permission Rules

**Current State**: Permissions are per-tool-name (always/session/once/never). No argument-level control.

**Problem**: `bash` is effectively all-or-nothing. Users either approve all bash commands or approve each individually. No way to say "always allow `git status` but ask for `git push`."

**Solution**: Pattern-based permission rules supporting argument matching.

**Format** (stored in `~/.config/gorkbot/rules.json`):
```json
{
  "allow": [
    "read_file",
    "bash(git status)",
    "bash(git diff*)",
    "bash(npm run test*)",
    "web_fetch(domain:docs.anthropic.com)"
  ],
  "ask": [
    "bash(git push*)",
    "write_file(*.go)",
    "bash(docker*)"
  ],
  "deny": [
    "bash(rm -rf*)",
    "bash(sudo*)",
    "delete_file"
  ]
}
```

**Rule Evaluation Engine**:
```go
type RuleEngine struct {
    rules    []PermissionRule
    // Precedence: deny > ask > allow > default
}

func (re *RuleEngine) Evaluate(toolName, args string) PermissionDecision {
    // 1. Check deny rules first (deny > all)
    // 2. Check ask rules
    // 3. Check allow rules
    // 4. Return default for tool category
}

// Pattern matching supports:
// - Exact: "bash(git status)"
// - Glob: "bash(git *)"
// - Domain: "web_fetch(domain:github.com)"
// - Regex: "bash(regex:rm\s+-rf)"
```

**TUI Integration**: Add `/rules` command to view/add/remove rules interactively.

**Files**: New `pkg/tools/rules.go`, update `pkg/tools/permissions.go`, `pkg/commands/registry.go`

---

### 2.3 Project-Level Configuration (GORKBOT.md)

**Current State**: Only `.env` and `~/.config/gorkbot/` global config. No project-specific AI instructions.

**Problem**: No way to give the AI project-specific context that persists across sessions without repeating it every time. No team-shareable AI behavior configuration.

**Solution**: Hierarchical configuration files loaded in order:

**Discovery & Precedence** (lowest → highest priority):
1. `~/.config/gorkbot/GLOBAL.md` — user-global preferences
2. `~/.config/gorkbot/GLOBAL.local.md` — user-global personal overrides (gitignored)
3. `<project_root>/GORKBOT.md` — project-level (commit to repo)
4. `<project_root>/GORKBOT.local.md` — personal per-project (gitignored)
5. `<project_root>/.gorkbot/rules/*.md` — modular topic rules

**Format** (GORKBOT.md):
```markdown
# Project: Gorkbot

## Tech Stack
- Go 1.24.2, Bubble Tea TUI, xAI Grok, Google Gemini
- Build: `go build ./cmd/gorkbot/`
- Test: `go test ./...`

## Coding Standards
- Never use global variables
- Always handle errors explicitly
- Use structured slog logging

## Tool Behavior
Never auto-commit without explicit user confirmation.
Always ask before running `go build` (may be slow).

## Context
This project runs on Android Termux. Consider mobile constraints.
```

**Implementation**:
```go
// pkg/config/loader.go
type ConfigLoader struct {
    projectRoot string
    homeDir     string
}

func (cl *ConfigLoader) LoadProjectInstructions() string {
    // Walk up from cwd to find GORKBOT.md files
    // Load in precedence order, concatenate with separators
    // Cache result for session
}
```

**Auto-injection**: Loaded instructions prepended to every orchestrator system prompt.

**`/memory` command**: Show which config files are loaded and their sizes.

**Files**: New `pkg/config/loader.go`, update `internal/engine/orchestrator.go`, `pkg/commands/registry.go`

---

### 2.4 Skill Definitions as Markdown Files

**Current State**: Skills are hardcoded in `pkg/tools/skills.go`. Adding a skill requires recompiling.

**Problem**: Users can't create new slash-command skills without touching Go code. Power users should be able to define task templates as markdown files.

**Solution**: File-based skill definitions with YAML frontmatter.

**Directory**: `~/.config/gorkbot/skills/` and `./.gorkbot/skills/`

**Format** (`.gorkbot/skills/code-review.md`):
```markdown
---
name: code-review
description: Thorough code review with security focus
aliases: [cr, review]
tools: [read_file, grep_content, bash]
model: grok-3
---

# Code Review Skill

Review the code changes in {{target}} for:
1. Security vulnerabilities (OWASP Top 10)
2. Logic errors and edge cases
3. Performance bottlenecks
4. Test coverage gaps
5. Documentation completeness

Provide specific line references and actionable recommendations.
```

**Usage**: `/code-review internal/engine/orchestrator.go` or `/cr pkg/tools/`

**Template Variables**: `{{target}}`, `{{args}}`, `{{date}}`, `{{project}}`

**Implementation**:
```go
// pkg/skills/loader.go
type SkillDefinition struct {
    Name        string
    Description string
    Aliases     []string
    Tools       []string // Allowed tools (empty = all)
    Model       string   // Override model
    Template    string   // Prompt template body
}

func (sl *SkillLoader) LoadFromDir(dir string) ([]SkillDefinition, error)
func (sl *SkillLoader) Execute(ctx context.Context, skill SkillDefinition, args string) (string, error)
```

**Files**: New `pkg/skills/`, update `pkg/commands/registry.go`

---

## Phase 3: Session Control & UX (P1)

### 3.1 Interrupt & Stop System

**Current State**: No reliable way to interrupt an in-progress AI generation or tool execution chain mid-stream. `Ctrl+C` quits the entire app.

**Problem**: Users can't stop a runaway agentic loop without killing the whole process. Critical for safety and user control.

**Solution**: `Esc` key for graceful interrupt. Two-level interrupt:
- **Single Esc**: Stop current AI generation (cancel stream), show partial response
- **Double Esc**: Stop current generation AND cancel pending tool execution

**Implementation**:
```go
// internal/engine/orchestrator.go
type Orchestrator struct {
    // ...
    cancelFunc context.CancelFunc  // Cancels current generation
    stopChan   chan struct{}         // Signals tool loop to stop
}

// In TUI update.go - handle Esc key
case tea.KeyMsg:
    if msg.Type == tea.KeyEsc {
        if m.isGenerating {
            m.orchestrator.Interrupt()  // Cancel stream
            return m, nil
        }
    }
```

**Status Feedback**: Status bar shows "Interrupted — press Enter to continue" after interrupt.

**Files**: Update `internal/tui/update.go`, `internal/tui/keys.go`, `internal/engine/orchestrator.go`

---

### 3.2 New Power Commands

The following slash commands provide critical session control missing from Gorkbot:

#### `/context` — Context Window Inspector
Shows a breakdown of what's consuming the context window:
```
Context Window: 73,241 / 131,072 tokens (55.9%)

  System Prompt:      2,400  (1.8%)
  GORKBOT.md:         1,100  (0.8%)
  Conversation:      48,200 (36.8%)
  Tool Results:      18,000 (13.7%)
  Last Response:      3,541  (2.7%)
  ─────────────────────────────────
  Total:             73,241 (55.9%)

Tip: Use /compact to reduce context by ~60%
```

#### `/compact [focus]` — Manual Context Compaction
Force SENSE compressor with optional focus:
```
/compact authentication system    # Summarize keeping auth context
/compact                          # General compression
```

#### `/cost` — Session Cost Tracker
Show accumulated API usage for the session:
```
Session Cost Summary
─────────────────────
  Primary (Grok-3):    $0.043  (12,400 input / 3,200 output tokens)
  Consultant (Gemini): $0.002  (1,100 input / 400 output tokens)
  ─────────────────────────────────
  Total Session:       $0.045
  Est. hourly rate:    $0.27/hr
```

#### `/rewind [checkpoint_id]` — Session Rewind
Restore to a previous checkpoint:
```
/rewind          # Show checkpoint list
/rewind last     # Rewind to most recent checkpoint
/rewind cp_003   # Rewind to specific checkpoint
```

#### `/resume [session_name]` — Resume Previous Session
Load a named session from disk:
```
/resume                     # Show available sessions
/resume project-planning    # Load specific session
```

#### `/rename <new_name>` — Name Current Session
Save current session with a memorable name for later resumption.

#### `/mode [plan|auto|normal]` — Switch Execution Mode
Change execution mode without keyboard shortcut:
```
/mode plan     # Switch to plan mode
/mode auto     # Switch to auto-edit mode
/mode normal   # Switch to default mode
```

**Files**: Update `pkg/commands/registry.go`, `internal/tui/model.go`

---

### 3.3 Streaming Tool Display

**Current State**: Tool execution is opaque — the user sees "Running..." and then gets a wall of results. No visibility into what's happening during multi-step agentic execution.

**Problem**: Long-running agentic tasks feel like a black box. Users don't know if things are working or stuck.

**Solution**: Real-time tool call display in the chat view as tools execute:

```
╭─ Tool Execution ────────────────────────────────────╮
│ ● read_file("internal/engine/orchestrator.go")  ✓  │
│ ● read_file("pkg/tools/registry.go")            ✓  │
│ ◌ bash("go build ./cmd/gorkbot/")             [1.2s]│
│ ◌ bash("go test ./...")                      pending │
╰─────────────────────────────────────────────────────╯
```

**Implementation**: New `ToolProgressMsg` in `messages.go`:
```go
type ToolProgressMsg struct {
    RequestID string
    ToolName  string
    Status    ToolStatus // pending/running/done/failed
    Elapsed   time.Duration
    Result    string
}
```

**View rendering**: In `view.go`, maintain an "active tool panel" above the input that shows currently-executing tools, auto-dismisses when all complete.

**Files**: Update `internal/tui/messages.go`, `internal/tui/view.go`, `internal/tui/update.go`

---

### 3.4 Enhanced Status Bar

**Current State**: Status bar shows spinner, running process count, model, and tokens. Good start but limited.

**Enhancement**: Richer status bar with context %, session cost, mode indicator, and git info:

```
[●●●●●○○○] 62% | $0.04 | grok-3 • gemini | 3 tools | main ↑2 | [PLAN]
```

Left-to-right:
- **Context bar**: Mini ASCII bar + percentage
- **Cost**: Running session cost
- **Models**: Active primary + consultant name abbreviations
- **Tool queue**: Count of pending/active tools
- **Git status**: Current branch + ahead/behind indicator
- **Mode**: Current execution mode (only shown when not NORMAL)

**Debouncing**: Status bar updates debounced to 200ms to prevent flicker during streaming.

**Files**: Update `internal/tui/statusbar.go`

---

## Phase 4: AI Intelligence Upgrades (P1)

### 4.1 Structured Tool Calls (Native Format)

**Current State**: `pkg/tools/parser.go` extracts tool calls from AI text responses by searching for JSON in markdown blocks. This is fragile — the AI might format JSON differently.

**Problem**: Text-based tool call parsing is inherently fragile. The AI may include JSON in explanations, wrap it differently, or produce malformed JSON that partially works.

**Solution**: Use the xAI API's native function calling format when available. When not available (older models), keep the existing parser as fallback.

**Grok Function Calling Format**:
```go
// In pkg/ai/grok.go - add tools parameter to API request
type ChatRequest struct {
    Model    string        `json:"model"`
    Messages []Message     `json:"messages"`
    Tools    []ToolSchema  `json:"tools,omitempty"`  // Native function defs
    Stream   bool          `json:"stream"`
}

type ToolSchema struct {
    Type     string       `json:"type"`     // "function"
    Function FunctionDef  `json:"function"`
}

// Extract tool calls from native format
type ToolCall struct {
    ID       string          `json:"id"`
    Type     string          `json:"type"`  // "function"
    Function FunctionCallDef `json:"function"`
}
```

**Benefits**:
- Zero false positives (no JSON in explanations accidentally parsed)
- AI knows exactly which tools are available (schema provided)
- Parallel calls naturally supported via tool_call arrays
- More reliable parameter extraction (typed schemas)

**Files**: Update `pkg/ai/grok.go`, `pkg/tools/registry.go`, `pkg/tools/parser.go`

---

### 4.2 Improved Subagent Isolation

**Current State**: Subagents run with the same tool registry as the parent. No worktree isolation.

**Problem**: Subagents doing heavy exploration pollute the parent context. File-modifying subagents can conflict with parent's file state.

**Solution**: Subagent worktree isolation + context isolation.

**Worktree Isolation** (for file-modifying subagents):
```go
// pkg/subagents/worktree.go
type WorktreeManager struct {
    baseDir string
}

func (wm *WorktreeManager) Create(branchName string) (string, error) {
    // git worktree add <tempdir> -b <branchName>
    // Return tempdir path
}

func (wm *WorktreeManager) Remove(worktreePath string) error {
    // git worktree remove --force <path>
}
```

**Context Isolation**: Each subagent gets a fresh `ConversationHistory` — only the task description is passed, not the parent's full history. Results return as a summary.

**Tool Filtering**: Subagent tool access configurable:
```go
type SubagentConfig struct {
    Type        AgentType
    Task        string
    AllowedTools []string  // Empty = all; set to restrict
    DeniedTools  []string  // Tools to block
    UseWorktree  bool      // Create git worktree for isolation
    MaxTurns    int        // Override default 10
}
```

**Files**: New `pkg/subagents/worktree.go`, update `pkg/subagents/agent.go`, `pkg/subagents/agents.go`

---

### 4.3 Automatic Model Routing Intelligence

**Current State**: Router heuristics in `pkg/router/heuristics.go` do basic keyword matching to decide primary vs consultant.

**Problem**: Routing is static and doesn't learn from feedback. A complex coding question always goes to Grok even if Gemini performed better last time.

**Solution**: Adaptive routing with feedback learning.

**Signals collected after each response**:
- User continued conversation naturally (positive signal)
- User asked to "try again" / "that's wrong" (negative signal)
- Response was followed by a tool call that fixed the AI's mistake (negative signal)
- Response length vs task complexity score (calibration)
- Time-to-first-token (efficiency signal)

**Routing adjustments**:
```go
// pkg/router/adaptive.go
type AdaptiveRouter struct {
    base         *Router
    feedback     *FeedbackStore    // pkg/router/feedback.go (exists)
    scores       map[string]float64 // model → quality score
}

func (ar *AdaptiveRouter) Route(query string) ModelSelection {
    // 1. Get base recommendation from heuristics
    // 2. Adjust based on historical performance for this query type
    // 3. Consider query complexity score (token count, keywords)
    // 4. Return optimized model selection
}
```

**Files**: New `pkg/router/adaptive.go`, update `pkg/router/router.go`, `pkg/router/feedback.go`

---

## Phase 5: Tool System Enhancements (P2)

### 5.1 Tool Result Caching

**Current State**: Tools execute fresh every time, even for identical calls.

**Problem**: Repeated reads of the same file, duplicate web fetches, same git status checks all consume time and tokens unnecessarily.

**Solution**: Intelligent result caching with TTL and cache-busting.

```go
// pkg/tools/cache.go
type ToolCache struct {
    entries map[string]CacheEntry
    mu      sync.RWMutex
    defaultTTL time.Duration // 60 seconds
}

// Cacheable tools (and their TTLs):
var CacheableTool = map[string]time.Duration{
    "read_file":      5 * time.Minute,    // File content (busted on write)
    "web_fetch":      15 * time.Minute,   // Web content
    "git_status":     10 * time.Second,   // Git state
    "system_info":    5 * time.Minute,    // System info
    "list_directory": 30 * time.Second,   // Directory listings
}

// Cache is busted when write_file/edit_file executes on same path
```

**Files**: New `pkg/tools/cache.go`, update `pkg/tools/registry.go`

---

### 5.2 Tool Analytics Dashboard

**Current State**: Tool analytics are tracked to a JSON file but never surfaced in the UI.

**Problem**: Users can't see which tools are being used, which are slow, which are failing.

**Solution**: `/tools stats` command shows analytics dashboard in TUI:

```
Tool Analytics (this session)
──────────────────────────────────────────────────────────
Tool              │ Calls │ Avg Time │ Success │ Last Used
──────────────────┼───────┼──────────┼─────────┼──────────
read_file         │   23  │  0.02s   │  100%   │  0:32 ago
bash              │   11  │  1.24s   │   91%   │  1:05 ago
web_fetch         │    4  │  2.10s   │  100%   │  5:12 ago
git_status        │    8  │  0.08s   │  100%   │  2:30 ago
write_file        │    6  │  0.03s   │   83%   │  8:00 ago
```

**Files**: Update `pkg/tools/analytics.go`, `pkg/commands/registry.go`

---

### 5.3 Tool Error Recovery

**Current State**: When a tool fails, the error is returned to the AI as plain text. The AI may or may not try again.

**Problem**: Common tool failures (missing file, network timeout, permission denied) often have well-known recovery actions. The AI wastes turns figuring out what to do.

**Solution**: Structured error results with suggested recovery actions:

```go
type ToolError struct {
    Code       string   // "file_not_found", "timeout", "permission_denied"
    Message    string   // Human-readable message
    Recovery   []string // Suggested recovery steps for AI
    IsRetryable bool
    RetryAfter time.Duration
}
```

**Common recovery hints**:
- `file_not_found`: "Use list_directory to find the correct path"
- `timeout`: "The command timed out. Try with a smaller scope or use start_background_process"
- `permission_denied`: "Check file permissions with bash(ls -la)"
- `json_parse_error`: "The output was not valid JSON. Check the command output format"

**Files**: Update `pkg/tools/tool.go`, individual tool files

---

## Phase 6: Developer Experience (P2)

### 6.1 Debug / Trace Mode

**Current State**: `-watchdog` flag gives some orchestrator debug info. No detailed trace of AI decision making.

**Solution**: Comprehensive trace mode with structured logging:

```
gorkbot --trace    # Enable trace mode
```

Trace output includes:
- Every token received from AI
- Every tool call with timing
- Context window size at each turn
- SENSE component outputs (LIE score, stabilizer score)
- Routing decisions and why
- Permission checks and results

**Implementation**: Structured trace log to `~/.config/gorkbot/trace_<session>.jsonl` — one JSON line per event.

---

### 6.2 One-Shot / Scripting Mode Enhancements

**Current State**: `-p "prompt"` one-shot mode exists but is basic — no stdin support, no format control, no tool control.

**Enhancement**:
```bash
# Stdin support
echo "What's in this file?" | gorkbot --stdin input.go

# Output format control
gorkbot -p "list all Go files" --output json
gorkbot -p "summarize this" --output plain

# Tool restrictions for scripting
gorkbot -p "analyze code" --allow-tools "read_file,grep_content,bash"
gorkbot -p "document this" --deny-tools "write_file,bash"

# Model selection
gorkbot -p "quick question" --model grok-3-mini

# Context from file
gorkbot -p "review this" --context-file context.md
```

**Files**: Update `cmd/gorkbot/main.go`

---

### 6.3 Conversation Export

**Current State**: Sessions save to JSON in `~/.config/gorkbot/sessions/`. No export options.

**Solution**: `/export` command with format options:

```
/export markdown          # Export as markdown (tool calls as code blocks)
/export json              # Export full session JSON
/export plain             # Plain text (AI responses only, no tool calls)
/export <filename>        # Export to specific file
```

**Files**: Update `pkg/commands/registry.go`, new `pkg/session/exporter.go`

---

## Phase 7: Platform & Integration (P3)

### 7.1 MCP-Compatible Tool Protocol

**Current State**: Gorkbot has its own tool protocol (JSON in markdown). No external tool server support.

**Solution**: Support the Model Context Protocol (MCP) for connecting external tool servers. This allows Gorkbot to consume tools from any MCP server — databases, GitHub, Slack, etc.

**Config** (`.gorkbot/mcp.json`):
```json
{
  "servers": [
    {
      "name": "github",
      "command": "npx @modelcontextprotocol/server-github",
      "env": {"GITHUB_TOKEN": "${GITHUB_TOKEN}"}
    },
    {
      "name": "filesystem",
      "command": "npx @modelcontextprotocol/server-filesystem /workspace"
    }
  ]
}
```

**Implementation**: MCP stdio transport (shell out, JSON-RPC 2.0 over stdin/stdout).

**Files**: New `pkg/mcp/`, update `pkg/tools/registry.go`

---

### 7.2 Remote Session Sharing

**Current State**: Sessions are local only.

**Solution**: Optional WebSocket-based session relay for:
- Pair programming (share session with collaborator)
- Remote monitoring of long-running tasks
- Mobile handoff (phone → desktop, desktop → phone)

**Architecture**:
```
gorkbot --share           # Creates shareable session URL
gorkbot --join <session>  # Join shared session as observer
```

---

### 7.3 TUI Theme System

**Current State**: Single Dracula theme. `/theme` command exists but limited.

**Solution**: Full theme system with user-defined themes:

**Theme file** (`~/.config/gorkbot/themes/monokai.toml`):
```toml
[colors]
primary = "#F8F8F2"
accent = "#A6E22E"
error = "#F92672"
warning = "#FD971F"
info = "#66D9EF"
consultant_border = "#AE81FF"
background = "#272822"
muted = "#75715E"

[ui]
header_style = "bold"
code_theme = "monokai"
spinner_style = "dots"
```

**Built-in themes**: dracula, monokai, solarized, nord, catppuccin, gruvbox

**Files**: New `pkg/themes/`, update `internal/tui/style.go`

---

## Implementation Priority Matrix

| Enhancement | Impact | Effort | Priority | Phase |
|-------------|--------|--------|----------|-------|
| Parallel Tool Execution | Very High | High | P0 | 1 |
| Context Tracking + Auto-Compact | Very High | Medium | P0 | 1 |
| Session Checkpoint/Rewind | High | Medium | P0 | 1 |
| Plan Mode | High | Low | P0 | 1 |
| Lifecycle Hooks | High | Medium | P1 | 2 |
| Fine-Grained Permission Rules | High | Medium | P1 | 2 |
| GORKBOT.md Config Files | Medium | Low | P1 | 2 |
| Skill Markdown Files | Medium | Medium | P1 | 2 |
| Interrupt/Stop System | High | Low | P1 | 3 |
| New Power Commands | Medium | Low | P1 | 3 |
| Streaming Tool Display | High | Medium | P1 | 3 |
| Enhanced Status Bar | Medium | Low | P1 | 3 |
| Native Function Calling | High | Medium | P1 | 4 |
| Subagent Worktree Isolation | Medium | Medium | P1 | 4 |
| Adaptive Model Routing | Medium | High | P1 | 4 |
| Tool Result Caching | Medium | Low | P2 | 5 |
| Tool Analytics Dashboard | Low | Low | P2 | 5 |
| Tool Error Recovery | Medium | Low | P2 | 5 |
| Debug/Trace Mode | Medium | Low | P2 | 6 |
| One-Shot Enhancements | Medium | Low | P2 | 6 |
| Conversation Export | Low | Low | P2 | 6 |
| MCP Protocol Support | Very High | Very High | P3 | 7 |
| TUI Theme System | Low | Medium | P3 | 7 |

---

## Quick Wins (Implement First — High Impact, Low Effort)

1. **Plan Mode** (`Ctrl+P` cycles modes, system prompt injection) — 1-2 days
2. **Esc Interrupt** (cancel generation mid-stream) — half day
3. **`/context` command** (show token breakdown) — half day
4. **`/cost` command** (session cost tracking) — half day
5. **`/rewind` with simple checkpoint stack** — 1 day
6. **GORKBOT.md config file loading** — 1 day
7. **Enhanced Status Bar** (context %, mode indicator) — 1 day
8. **Streaming Tool Display** (show tool progress in chat) — 1-2 days

These 8 items represent ~1 week of work and would dramatically elevate the daily UX.

---

## Architecture Principles Adopted

These principles extracted from best-in-class AI CLI tools should guide all Gorkbot development going forward:

1. **Context is the primary constraint** — Every feature decision should consider context impact. Subagents, lazy loading, and compaction exist to serve this constraint.

2. **Streaming is non-negotiable** — Real-time token display is what makes AI feel fast. Never batch responses.

3. **User control at all times** — The user must be able to interrupt, rewind, or inspect at any point. AI work should never feel irreversible.

4. **Local-first execution** — Hooks, config files, theme scripts — run locally, no API calls. This keeps the system fast and private.

5. **Layered permissions** — Deny > Ask > Allow. Fine-grained rules prevent both over-blocking and under-blocking.

6. **Subagent isolation** — Long agentic work should never pollute main context. Send in clean context, receive summary.

7. **Explicit over implicit** — Show the user what's happening (streaming tool display, plan mode indicator, context %) rather than hiding complexity.

8. **Configuration hierarchies** — Global < project < local. Teams configure via project files; individuals override locally.

9. **Tools are first-class citizens** — Tool availability, timing, and results should be visible, cached, and analytically tracked.

10. **Extensibility via files, not code** — Skills, hooks, themes, rules should all be user-configurable via files without recompilation.

---

*Document generated: 2026-02-22 | Based on Gorkbot v1.6.0 analysis*
