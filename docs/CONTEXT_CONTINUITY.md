# Context Continuity & Memory Systems

**Version:** 3.4.0

Gorkbot maintains context continuity across turns within a session, and across sessions via several persistent memory systems. This document explains all memory layers and how they interact.

---

## Table of Contents

1. [In-Session Conversation History](#1-in-session-conversation-history)
2. [CCI — Three-Tier Project Memory](#2-cci--three-tier-project-memory)
3. [SENSE AgeMem & Engrams](#3-sense-agemem--engrams)
4. [MEL Heuristic Store](#4-mel-heuristic-store)
5. [Goal Ledger](#5-goal-ledger)
6. [Unified Memory API](#6-unified-memory-api)
7. [Session Management](#7-session-management)
8. [Context Window Management](#8-context-window-management)
9. [Message Flow Example](#9-message-flow-example)

---

## 1. In-Session Conversation History

`pkg/ai.ConversationHistory` maintains the complete message chain for the current session. It is passed to every AI API call, giving the model full context of the conversation.

### Message Types

```go
type ConversationMessage struct {
    Role        string       // "system" | "user" | "assistant" | "tool"
    Content     string
    Timestamp   time.Time
    ToolCalls   []ToolCallEntry  // native function calling (xAI)
    ToolCallID  string           // for "tool" role messages
    ToolName    string           // for "tool" role messages
}
```

### Thread Safety

`ConversationHistory` uses a `sync.RWMutex` — safe for concurrent access from multiple goroutines (streaming + tool execution).

### API

```go
// Adding messages
history.AddSystemMessage("System context...")
history.AddUserMessage("User question")
history.AddAssistantMessage("AI response")
history.AddToolCallMessage(toolCalls)           // native function calling
history.AddToolResultMessage(id, name, result)  // tool results

// Reading messages
messages := history.GetMessages()
recent := history.GetRecentMessages(10)
count := history.Count()
tokens := history.EstimateTokens()    // rough: len(content)/4

// Management
history.Truncate(50)                  // keep last 50 messages
history.TruncateToTokenLimit(100000)  // fit within token budget
history.Clear()                       // reset (called by /clear)
```

### Clearing Context

```
/clear    # clears TUI messages + ConversationHistory + starts fresh
```

---

## 2. CCI — Three-Tier Project Memory

**Package:** `pkg/cci`

CCI (Codified Context Infrastructure) provides persistent project knowledge across sessions. It is injected into every system message, giving the AI awareness of the project's conventions, architecture, and subsystems without relying on in-session messages.

### Tier 1 — Hot Memory (always loaded)

**Path:** `~/.config/gorkbot/cci/hot/`

- `CONVENTIONS.md` — Universal coding conventions, naming rules, workflow patterns
- `SUBSYSTEM_POINTERS.md` — Index of all Tier 3 subsystem docs with one-line summaries

`HotMemory.BuildBlock()` returns a compact string (< 2000 tokens) prepended to every system message. Includes conventions + subsystem index summary + ARC trigger table summary + any active drift warnings.

**Edit directly** or via the `mcp_context_update_subsystem` tool to add project-specific conventions.

### Tier 2 — Specialist Memory (on-demand)

**Path:** `~/.config/gorkbot/cci/specialists/<domain>.md`

Loaded when the ARC trigger table matches a file path or task description to a domain. Contains domain-specific patterns, failure mode tables, and known pitfalls.

**Domains:** `security`, `frontend`, `backend`, `devops`, `data_science`, `mobile`, `database`, etc.

Activate manually: `mcp_context_suggest_specialist` tool returns the best matching domain; the orchestrator loads it before the AI turn.

### Tier 3 — Cold Memory (on-demand via tool)

**Path:** `~/.config/gorkbot/cci/docs/<subsystem>.md`

Queried by the AI via `mcp_context_get_subsystem("<subsystem>")`. On empty result, `HandleCCIGap()` triggers Plan mode — the AI stops and explicitly plans how to acquire the missing knowledge.

**Living documents** — updated by `mcp_context_update_subsystem` as the project evolves. Auto-populated with drift warnings when git history shows the subsystem's source files changed.

### CCI Tools

| Tool | Purpose |
|------|---------|
| `mcp_context_list_subsystems` | List all Tier 3 docs |
| `mcp_context_get_subsystem` | Retrieve a spec |
| `mcp_context_suggest_specialist` | Recommend a Tier 2 domain |
| `mcp_context_update_subsystem` | Write/update a Tier 3 doc |
| `mcp_context_list_specialists` | List Tier 2 domains |
| `mcp_context_status` | Full CCI status report |

### Drift Detection

`DriftDetector.Check()` runs `git log --since=<doc_updated_at> -- <subsystem_files>` to find files modified after the doc was last updated. Drift warnings appear in the Tier 1 block:

```
⚠️ DRIFT WARNING: pkg/auth/jwt.go modified 3 days after auth_system doc was last updated.
   CCI docs may be stale. Consider running: mcp_context_update_subsystem auth_system
```

---

## 3. SENSE AgeMem & Engrams

**Package:** `pkg/sense` (integrated via `internal/engine`)

SENSE (Semantic Experience Neural Storage Engine) provides episodic memory with age-stratification.

### AgeMem

Age-stratified memory that retains "memories" with decreasing fidelity over time, similar to human memory consolidation:

- **Hot memories** — recent experiences, high fidelity, quickly accessible
- **Warm memories** — consolidated summaries, moderate fidelity
- **Cold memories** — long-term abstractions, low fidelity, permanent

AgeMem is initialized by `Orchestrator.InitSENSEMemory(configDir)` and populated during each session. Relevant memories are retrieved and injected into the system prompt.

### Engrams

`EngramStore` records specific experiences as structured knowledge entries:

```
record_engram(content="Discovered that batch tool execution requires WaitGroup", tags=["concurrency", "tools"])
```

Engrams persist across sessions and are recalled when similar tasks arise.

### `code2world` Tool

Translates code artifacts into semantic knowledge:

```
code2world(path="pkg/auth/jwt.go", notes="JWT implementation with refresh token rotation")
```

Creates an AgeMem entry linking the file's purpose and patterns to the SENSE knowledge graph.

---

## 4. MEL Heuristic Store

**Package:** `internal/mel`

MEL (Meta-Experience Learning) derives heuristics from repeated tool failure/success patterns and injects them into the system prompt.

### How Heuristics Form

```
ExecuteTool("write_file", {path: "auth.go", content: "..."})
  → Failure: "file locked by another process"

MEL.ObserveFailed("write_file", {path: "auth.go"}, error)

# Later, same task succeeds differently:
ExecuteTool("bash", {command: "flock -x auth.go -c 'cat > auth.go'"})
  → Success

MEL.ObserveSuccess("write_file-via-flock", {path: "auth.go"})

# BifurcationAnalyzer detects divergence:
→ Generates heuristic: "When writing auth.go, verify file lock with flock to avoid EWOULDBLOCK"
→ VectorStore.Add(heuristic)
→ Persisted to ~/.config/gorkbot/vector_store.json
```

### Heuristic Structure

```go
type Heuristic struct {
    Context    string   // "When [ctx]"
    Constraint string   // "verify [constraint]"
    Avoid      string   // "avoid [error]"
    Confidence float64  // 0.0-1.0; evicted when lowest at 500-entry cap
    Tags       []string
}
```

### Injection

Before each AI turn, the top-N (default 3) heuristics most relevant to the current prompt are retrieved via Jaccard similarity and prepended to the system message.

### Vector Store

**Path:** `~/.config/gorkbot/vector_store.json`
- Max capacity: 500 entries
- Deduplication: entries > 70% similar are merged/updated
- Eviction: lowest-confidence entry removed when at capacity

---

## 5. Goal Ledger

**Package:** `pkg/memory`

Cross-session prospective memory for multi-session goals and tasks.

```
add_goal(description="Refactor auth module to JWT", priority="high")
list_goals()         # shows all open goals at session start
close_goal(id, outcome="Completed JWT migration in PR #42")
```

**Path:** `~/.config/gorkbot/goal_ledger.json`

Open goals are listed in the system prompt on each session start, giving the AI awareness of ongoing work across restarts.

---

## 6. Unified Memory API

`memory.UnifiedMemory` provides a single API wrapping all three memory systems:

```go
type UnifiedMemory struct {
    AgeMem  *sense.AgeMem
    Engrams *sense.EngramStore
    Store   *mel.VectorStore
}

// Query all systems at once
results := unifiedMem.Query(ctx, "auth module patterns")
// Returns: relevant AgeMem entries + Engrams + MEL heuristics
// ranked by relevance score
```

The orchestrator uses `UnifiedMemory.Query` when building the system prompt to efficiently pull context from all persistent stores.

---

## 7. Session Management

### Automatic Checkpoints

Gorkbot saves up to 20 conversation checkpoints automatically. Each checkpoint captures the full `ConversationHistory` at that moment.

```
/rewind last          # restore most recent checkpoint
/rewind <id>          # restore specific checkpoint
```

After a rewind, the TUI clears its message list and shows a rewind notice.

### Named Sessions

```
/save my-session      # serialize and write to session file
/resume my-session    # deserialize and restore
/resume list          # list all saved sessions
```

Session files are stored in `~/.config/gorkbot/sessions/` (JSON encoded with full message history).

### Export

```
/export markdown                    # export to timestamped .md file
/export json session.json           # export specific file
/export plain                       # plain text
```

---

## 8. Context Window Management

**Package:** `internal/engine/context_manager.go`

### Token Tracking

After each AI turn, `GrokProvider.GetLastUsage()` (or equivalent) returns token counts which are fed to `ContextMgr.UpdateFromUsage()`:

```
TokenUsage{PromptTokens: 12345, CompletionTokens: 456, TotalTokens: 12801}
  → ContextMgr.UpdateFromUsage()
  → StatusBar.SetContextStats(pct, costStr)
  → BillingManager.Record(model, usage)
  → Emits ContextUpdateMsg to TUI
```

The status bar shows: `[ 34% ctx ] [ $0.0087 ]`

### Truncation Strategy

`ConversationHistory.TruncateToTokenLimit(100000)` (called after each message addition):

1. Separate system messages from conversation messages
2. Calculate tokens used by system messages
3. Starting from the most recent message, greedily include conversation messages until token budget is exhausted
4. Rebuild: `systemMessages + keptConversationMessages`

**Key invariant:** A very large individual message is **skipped** (not dropped), preserving older history — this was a deliberate fix from the original `break`-based implementation that would discard all older history when encountering one large message.

### Context Commands

```
/context    # show current usage breakdown
/cost       # show session cost estimate by model
/compact    # intelligent compression (summarize + retain recent)
/compress   # alias for /compact
```

### Compression

`/compact [focus hint]` invokes `Orchestrator.CompactFocus(hint)`:
1. Sends the current conversation history to the AI with instructions to compress it
2. Replaces the history with a summary + the most recent N messages
3. Emits a notice in the TUI indicating compression occurred and how many tokens were saved

---

## 9. Message Flow Example

### Turn 1

```
State: history=[SYSTEM], goal_ledger=[Goal: Refactor auth], mel_heuristics=[1 match]

User: "Start refactoring the auth module"

→ CCI.BuildCCISystemContext() → "Tier 1: conventions... Tier 2: security specialist..."
→ MEL heuristic injected: "When touching auth, verify token expiry handling"
→ Goal context injected: "Open goal: Refactor auth module to JWT (high priority)"
→ ConvHistory.AddUserMessage("Start refactoring the auth module")
→ Primary.GenerateWithTools(history, schemas)
   → tool_calls: [{name: "read_file", params: {path: "pkg/auth/auth.go"}}]
→ Execute read_file → content
→ MEL.ObserveSuccess("read_file", {path: "pkg/auth/auth.go"})
→ ConvHistory.AddToolResult(…)
→ Primary.GenerateWithTools(history, schemas)
   → response: "I can see the auth module uses sessions. Here's my refactoring plan..."
→ ConvHistory.AddAssistantMessage("I can see...")
→ StreamCompleteMsg → TUI renders

State: history=[SYSTEM, USER, TOOL_RESULT, ASSISTANT], checkpoint saved
```

### Turn 2

```
User: "Apply the changes"

→ CCI context: same (always-loaded Tier 1)
→ MEL heuristics: same + new ones if Turn 1 generated bifurcations
→ ConvHistory.AddUserMessage("Apply the changes")
→ AI has full context of Turn 1 (file content, plan discussed)
→ Proceeds to write changes using write_file
```

The AI retains full context throughout — no re-explanation needed.
