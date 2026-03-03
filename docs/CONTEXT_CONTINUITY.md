# Gorkbot Context & Memory Architecture

**Version:** 3.5.1

This document explains how Gorkbot manages conversational context, persistent memory, and cross-session knowledge. It covers the `ConversationHistory` manager, the CCI three-tier memory system, SENSE AgeMem, the Goal Ledger, MEL heuristic learning, session checkpoints, and context window tracking.

---

## Table of Contents

1. [Conversational Context (ConversationHistory)](#1-conversational-context-conversationhistory)
2. [Context Window Tracking](#2-context-window-tracking)
3. [Session Checkpoints & Rewind](#3-session-checkpoints--rewind)
4. [Conversation Export & Sessions](#4-conversation-export--sessions)
5. [CCI — Codified Context Infrastructure](#5-cci--codified-context-infrastructure)
6. [SENSE AgeMem & Engrams](#6-sense-agemem--engrams)
7. [Goal Ledger](#7-goal-ledger)
8. [MEL — Meta-Experience Learning](#8-mel--meta-experience-learning)
9. [RAG Memory Plugin](#9-rag-memory-plugin)
10. [Unified Memory API](#10-unified-memory-api)
11. [Slash Commands Reference](#11-slash-commands-reference)

---

## 1. Conversational Context (ConversationHistory)

### Architecture

`pkg/ai.ConversationHistory` is the single source of truth for all in-session messages. It is a thread-safe, ordered sequence of `ConversationMessage` values.

```go
type ConversationHistory struct {
    messages []ConversationMessage
    mu       sync.RWMutex
}

type ConversationMessage struct {
    Role       string          // "system" | "user" | "assistant"
    Content    string          // message body
    Timestamp  time.Time
    ToolCalls  []ToolCallEntry // populated by native function calling (xAI)
    ToolCallID string          // for tool result messages
    ToolName   string          // tool name for tool result messages
}
```

### Message Roles

| Role | Source | Purpose |
|------|--------|---------|
| `system` | Orchestrator | Tool schemas, CCI Hot memory, MEL heuristics, GORKBOT.md |
| `user` | User, tool results | Human input and tool execution results |
| `assistant` | Primary AI | AI responses and native tool_call requests |

### Full-Turn Message Flow

Every AI turn appends messages to the history and sends the **complete history** to the provider API:

```
Turn N begins
  ├── history.AddUserMessage(prompt)
  ├── [first turn only] history.AddSystemMessage(full system context)
  │
  ├── provider.GenerateWithHistory(history) ← full context sent
  │     └── returns streaming response
  │
  ├── history.AddAssistantMessage(response)
  │
  ├── [if tool calls]
  │   ├── history.AddToolCallMessage(id, name, args)    ← native path
  │   ├── executor.Run(tool, args)
  │   ├── history.AddToolResultMessage(id, name, result)
  │   └── loop back to provider.GenerateWithHistory(history)
  │
  └── Turn N complete — history retained for Turn N+1
```

This ensures Gork maintains full memory of the entire session without requiring users to repeat context.

### Token-Aware Truncation

When the history grows beyond the configured token limit, `TruncateToTokenLimit()` trims the oldest non-system messages:

```
Strategy:
  1. All system messages are always preserved
  2. Most-recent conversation messages are kept first
  3. Oldest user/assistant pairs are dropped to fit the limit

Default limit: 100,000 tokens (estimated at ~4 chars/token)
```

The truncation algorithm uses `continue` (not `break`) on oversized individual messages, ensuring that a single very long message does not cause all prior history to be discarded.

### Provider Message Mapping

Each provider maps `ConversationMessage` roles to its own API format at send time:

| Gorkbot Role | xAI (Grok) | Google (Gemini) | Anthropic (Claude) | OpenAI |
|-------------|-----------|----------------|-------------------|--------|
| `system` | `"system"` | Prepended to user turn | `"system"` block | `"system"` |
| `user` | `"user"` | `"user"` | `"user"` | `"user"` |
| `assistant` | `"assistant"` | `"model"` | `"assistant"` | `"assistant"` |
| `tool` (result) | `"tool"` (native) | User message with `[Tool: name]` prefix | `"tool_result"` block | `"tool"` |

### Native Function Calling (xAI)

When xAI (Grok) is the primary provider, tool calls use the structured native function calling path:

1. Tool schemas are sent in the `tools: []` field of the API request.
2. Grok responds with structured `tool_calls` JSON (not free-form text).
3. `ToolCalls`, `ToolCallID`, and `ToolName` fields in `ConversationMessage` record the structured call and result.
4. This path is significantly more reliable than text-parsing for tool invocation.

For all other providers, tool requests are extracted from the AI's text output via `ParseToolRequests()`.

---

## 2. Context Window Tracking

### Live Context Stats

The context manager (`internal/engine/context_manager.go`) tracks token usage and cost in real time. Stats are displayed in the status bar at the bottom of the TUI and updated after every AI turn.

**Status bar fields:**
```
[ctx: 12% | $0.0042 | Normal | main]
  │           │         │       └── git branch
  │           │         └── execution mode
  │           └── session cost estimate
  └── context window percentage
```

### Viewing Detailed Context

```
/context
```

Outputs a breakdown of the context window:
```
Context Window Usage

System messages:    3,847 tokens  (3.8%)
Conversation:      8,412 tokens  (8.4%)
Tools schemas:     2,156 tokens  (2.2%)
─────────────────────────────────────
Total used:       14,415 tokens (14.4%)
Remaining:        85,585 tokens (85.6%)
Limit:           100,000 tokens
```

### Cost Tracking

```
/cost
```

Displays the current session's estimated API cost across all providers and models.

### Context Compaction

When the context window fills up, use `/compact` to summarize and compress the conversation while retaining key information:

```
/compact                    # auto-summarize current conversation
/compact "focus on auth"    # guide the summarization with a hint
/compress                   # alias for /compact
```

The compaction produces a condensed representation that replaces the conversation history, freeing context for further work.

---

## 3. Session Checkpoints & Rewind

The checkpoint system (`pkg/session.CheckpointManager`) maintains up to 20 snapshots of the conversation state per session.

### How Checkpoints Work

Checkpoints are taken automatically:
- Before each tool execution (when the tool requires permission)
- After each successful AI turn
- Explicitly on request

Each checkpoint stores:
- Complete `ConversationHistory` snapshot
- Current turn number
- Timestamp
- Metadata (model, tool calls that led to this state)

### Rewinding

```
/rewind                 # rewind to the most recent checkpoint
/rewind last            # same as above
/rewind <checkpoint-id> # rewind to a specific checkpoint by ID
```

When a rewind completes:
- The TUI clears the conversation display
- A rewind notice is shown: `[Rewound to checkpoint 12 — 3 turns removed]`
- The conversation history is restored to the snapshot state
- The session continues from that point forward

### Use Cases

- **Undo a bad AI action** — rewind before a tool call that had unexpected results
- **Branch exploration** — try one approach, rewind, try another
- **Recovery from context overflow** — rewind to before a long analysis that consumed too much context

---

## 4. Conversation Export & Sessions

### Exporting the Current Conversation

```
/export markdown          # export to Markdown file
/export json              # export to JSON file
/export plain             # export to plain text file
/export markdown report   # export to 'report.md' in current directory
```

Export files are saved to the current working directory with a timestamp in the filename if no name is specified.

### Saving and Loading Named Sessions

```
/save my-project-session    # save current conversation to a named session file
/resume my-project-session  # restore a named session
/chat list                  # list all saved sessions
/chat load my-project-session
/chat delete old-session
/rename new-name            # rename the current session
```

Sessions are stored in `~/.config/gorkbot/sessions/` as JSON files containing the full conversation history and metadata.

### SQLite Conversation Persistence

All conversation history is also written to an SQLite database (`pkg/persist`) at `~/.config/gorkbot/conversations.db`. This provides:
- Full-text search across all historical conversations
- Per-session tool call analytics
- Audit trail of all AI turns

---

## 5. CCI — Codified Context Infrastructure

CCI (`pkg/cci`) is a three-tier persistent memory system that gives Gorkbot durable knowledge about your projects across sessions. Unlike the conversation history (which resets on `/clear`), CCI persists indefinitely.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                CCI Memory Hierarchy                     │
│                                                         │
│  Tier 1 — Hot Memory (ALWAYS loaded)                   │
│  ┌─────────────────────────────────────────────────┐   │
│  │  CONVENTIONS.md    ← coding standards, rules   │   │
│  │  SUBSYSTEM_POINTERS.md ← index of Tier 3 docs  │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         │ injected into system prompt   │
│                         │ at session start              │
│                                                         │
│  Tier 2 — Specialist Personas (on-demand)              │
│  ┌─────────────────────────────────────────────────┐   │
│  │  specialists/security.md ← security context    │   │
│  │  specialists/frontend.md ← frontend context    │   │
│  │  specialists/devops.md   ← DevOps context      │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         │ loaded when ARC trigger       │
│                         │ pattern matches the task      │
│                                                         │
│  Tier 3 — Cold Memory / Living Docs (on-demand)        │
│  ┌─────────────────────────────────────────────────┐   │
│  │  docs/auth_system.md                           │   │
│  │  docs/database_layer.md                        │   │
│  │  docs/api_routes.md                            │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         │ queried via CCI tools         │
│                         │ (mcp_context_get_subsystem)   │
└─────────────────────────────────────────────────────────┘
```

### Tier 1 — Hot Memory (Always Loaded)

**Path:** `~/.config/gorkbot/cci/hot/`

Hot memory files are loaded unconditionally at session start and injected into the system prompt before any user interaction. They should contain:

- **`CONVENTIONS.md`** — universal project conventions: naming conventions, code style, forbidden patterns, tool preferences, build commands.
- **`SUBSYSTEM_POINTERS.md`** — index of available Tier 3 docs, so the AI knows what deep documentation is available to query.

Edit these files to teach Gorkbot about your project once and have it "know" them in every future session:

```bash
# Example CONVENTIONS.md content:
cat ~/.config/gorkbot/cci/hot/CONVENTIONS.md
```
```markdown
# Project Conventions

## Language: Go 1.24
- Use standard library; avoid unnecessary dependencies
- All exported functions must have godoc comments
- Error returns always use fmt.Errorf with %w for wrapping

## Repository Layout
- Internal packages: internal/
- Public API packages: pkg/
- Entry point: cmd/gorkbot/main.go

## Forbidden
- Never use panic() in library code
- Never commit .env files
- Never use time.Sleep() for retry logic
```

### Tier 2 — Specialist Personas (On-Demand)

**Path:** `~/.config/gorkbot/cci/specialists/`

One markdown file per domain. The ARC Router loads the matching specialist file when the task classification triggers it. Each specialist file contains:
- Domain-specific failure modes and known pitfalls
- Preferred patterns and anti-patterns for the domain
- Tool preferences and constraints

Common specialist domains: `security`, `frontend`, `backend`, `devops`, `data_science`, `mobile`.

### Tier 3 — Cold Memory / Living Docs (On-Demand)

**Path:** `~/.config/gorkbot/cci/docs/`

One markdown file per subsystem of your project. These are "living documents" that evolve as the codebase changes. They are never automatically loaded — they are queried explicitly when needed.

**Reading a subsystem doc:**
```
mcp_context_get_subsystem {"subsystem": "auth_system"}
```

**Updating a subsystem doc (the AI can do this automatically):**
```
mcp_context_update_subsystem {"subsystem": "auth_system", "content": "..."}
```

### Drift Detection

`pkg/cci.DriftDetector` monitors Tier 3 documents for staleness. When a document has not been updated in a configurable window (default: 30 days), it is flagged as potentially stale and a warning is included when it is loaded.

### CCI Tools

| Tool | Description |
|------|-------------|
| `mcp_context_get_subsystem` | Retrieve a Tier 3 subsystem document |
| `mcp_context_update_subsystem` | Update or create a Tier 3 subsystem document |
| `mcp_context_list_subsystems` | List all available Tier 3 documents |
| `mcp_context_get_hot` | Retrieve current Tier 1 Hot Memory content |
| `mcp_context_update_hot` | Update Tier 1 Hot Memory |
| `mcp_context_get_specialist` | Retrieve a Tier 2 specialist document |
| `mcp_context_update_specialist` | Update a Tier 2 specialist document |

---

## 6. SENSE AgeMem & Engrams

SENSE (`pkg/sense`, `pkg/memory`) is an age-stratified episodic memory system that provides cross-session recall beyond what CCI stores.

### Components

**AgeMem** — A rolling window of timestamped memory entries organized into age strata:
- **Hot** — recent (last 24 hours)
- **Warm** — last 7 days
- **Cold** — last 30 days
- **Archive** — older entries (compressed)

Memory entries are automatically promoted/demoted between strata based on age and access frequency.

**Engrams** — Individual memory records with semantic tagging. Each engram stores:
- Content (the remembered information)
- Tags / metadata
- Timestamp and access count
- Embedding vector for similarity search

### Tools

| Tool | Description |
|------|-------------|
| `sense_remember` | Store a new engram |
| `sense_recall` | Retrieve recent memories or search by query |
| `sense_forget` | Delete a specific engram by ID |
| `sense_stats` | Show AgeMem statistics (total engrams, strata counts) |

### Example Usage

```
# Store important context
sense_remember {"content": "Auth uses JWT with 24h expiry. Refresh via /api/auth/refresh.", "tags": ["auth", "jwt"]}

# Retrieve on a future session
sense_recall {"query": "JWT expiry", "limit": 5}
```

---

## 7. Goal Ledger

The Goal Ledger (`pkg/memory.GoalLedger`) tracks open goals and intentions across session boundaries. Unlike conversation history (which is session-scoped), goals persist indefinitely until explicitly marked complete.

### How It Works

Goals are stored in `~/.config/gorkbot/goals.json`. Each goal has:
- A description of the intended outcome
- A status: `open`, `in_progress`, `blocked`, `completed`
- Creation timestamp and last-updated timestamp
- Optional notes/progress log

### Tools

| Tool | Description |
|------|-------------|
| `goal_add` | Add a new open goal |
| `goal_list` | List all goals (filtered by status) |
| `goal_update` | Update goal status or add notes |
| `goal_complete` | Mark a goal as completed |
| `goal_delete` | Remove a goal |

### Example

```
# Create a goal at the end of one session
goal_add {"description": "Implement OAuth2 PKCE flow for the mobile app", "status": "open"}

# Next session — check open goals
goal_list {"status": "open"}
→ [GOAL-001] Implement OAuth2 PKCE flow for the mobile app  [open]

# Mark progress
goal_update {"id": "GOAL-001", "status": "in_progress", "notes": "PKCE library selected: golang.org/x/oauth2"}
```

---

## 8. MEL — Meta-Experience Learning

MEL (`internal/mel`) is a meta-learning system that automatically generates heuristics from past tool successes and failures, then injects them into the system prompt to improve future performance.

### Components

**BifurcationAnalyzer** (`internal/mel/analyzer.go`) — Observes tool execution outcomes:
- `ObserveFailed(tool, params, error)` — called after a tool error
- `ObserveSuccess(tool, params, result)` — called after a tool success
- Compares parameter differences between failed and successful calls to the same tool
- Automatically generates heuristics from recurring failure patterns

**Heuristic Format** — Three-part template:
```
When [context/condition],
  verify [constraint/precondition],
  avoid [known failure mode].
```

**VectorStore** (`internal/mel/store.go`) — JSON-persisted heuristic store:
- **Path:** `~/.config/gorkbot/vector_store.json`
- **Similarity:** Jaccard similarity for deduplication (>70% similarity → merge instead of add)
- **Capacity:** Maximum 500 heuristics; lowest-confidence entries evicted when full
- **Confidence scoring:** Updated based on how many times a heuristic has been validated by subsequent outcomes

### Heuristic Injection

Before each AI turn, MEL retrieves the most relevant heuristics for the current task (up to 5) and injects them as a prefix to the system message. This teaches the AI from accumulated experience without requiring explicit re-training.

### Example Generated Heuristics

```
When calling bash with a path argument,
  verify the path exists with file_info first,
  avoid "no such file or directory" errors.

When calling git_push,
  verify the current branch is not 'main' or 'master',
  avoid accidental force-pushes to protected branches.

When using http_request with POST,
  verify Content-Type header is set to application/json,
  avoid "400 Bad Request" errors from malformed payloads.
```

---

## 9. RAG Memory Plugin

The RAG memory plugin (`plugins/python/rag_memory/`) provides semantic vector search over stored text using ChromaDB and MiniLM-L6-v2 embeddings.

### Capabilities

- **Semantic search** — finds relevant stored content by meaning, not just keyword
- **Persistent storage** — ChromaDB collection at `~/.config/gorkbot/rag_memory/`
- **Rolling window** — maximum 10,000 engrams; oldest are pruned when full
- **Similarity scoring** — cosine similarity with configurable minimum score filter
- **Auto-installed** — dependencies (`chromadb`, `sentence-transformers`) are pip-installed on first use

### Tool Parameters

| Action | Required Parameters | Description |
|--------|-------------------|-------------|
| `store` | `content` | Embed and store text; optionally pass `metadata` (JSON string) |
| `search` | `query` | Semantic search; optionally pass `n_results` (default 5), `min_score` (default 0.0) |
| `stats` | — | Show collection stats (total engrams, model info) |
| `purge` | — | Delete all stored engrams |

### Example Usage

```json
{"action": "store", "content": "The payment API requires HMAC-SHA256 signature in X-Signature header. Key is in PAYMENT_SECRET env var.", "metadata": "{\"topic\": \"payment-api\", \"added\": \"2026-03-01\"}"}

{"action": "search", "query": "payment signature authentication", "n_results": 3, "min_score": 0.7}
```

### Storage Location

```
~/.config/gorkbot/rag_memory/
├── chroma.sqlite3         ChromaDB database
└── <collection-uuid>/     Embedding vectors
```

---

## 10. Unified Memory API

`pkg/memory.UnifiedMemory` provides a single facade that wraps all persistent memory systems:

```go
type UnifiedMemory struct {
    AgeMem      *AgeMem           // SENSE episodic memory
    Engrams     *EngramStore      // Tagged memory records
    MEL         *mel.VectorStore  // Heuristic store
    GoalLedger  *GoalLedger       // Cross-session goals
}
```

The orchestrator accesses all memory subsystems through this unified interface, ensuring consistent initialization, loading, and flushing across the session lifecycle.

---

## 11. Slash Commands Reference

### Context & History

| Command | Description |
|---------|-------------|
| `/context` | Show context window usage breakdown (tokens, %) |
| `/cost` | Show session API cost estimate |
| `/compact [hint]` | Summarize and compress conversation history |
| `/compress` | Alias for `/compact` |
| `/clear` | Reset conversation history and TUI display |

### Checkpoints & Sessions

| Command | Description |
|---------|-------------|
| `/rewind [last\|<id>]` | Restore to a previous checkpoint |
| `/save <name>` | Save current session to a named file |
| `/resume <name>` | Load a previously saved session |
| `/rename <name>` | Rename the current session |
| `/chat list` | List all saved sessions |
| `/chat load <name>` | Load a saved session |
| `/chat delete <name>` | Delete a saved session |
| `/export [format] [file]` | Export conversation (markdown/json/plain) |

### Memory

| Command | Description |
|---------|-------------|
| `/skills list` | List available skill definitions (includes memory-related skills) |
| `/context` | View context and token breakdown |

Memory tools (`sense_remember`, `goal_add`, etc.) are invoked directly as tool calls by the AI or by typing them in natural language.
