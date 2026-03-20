# CLAUDE.md - Gorkbot Architecture & Development Guide

This file provides comprehensive guidance for Claude Code and developers working on Gorkbot.

**Quick Info:**
- **Project**: Gorkbot (github.com/velariumai/gorkbot)
- **Language**: Go 1.25.0
- **Public Version**: 1.2.0-beta
- **Internal Version**: 5.3.0
- **Architecture**: Multi-provider orchestration with adaptive routing
- **Main Files**: cmd/gorkbot/main.go, internal/engine/orchestrator.go, internal/tui/, pkg/tools/registry.go

---

## 📋 Table of Contents

1. [Project Overview](#project-overview)
2. [Architecture](#architecture)
3. [Directory Structure](#directory-structure)
4. [Building & Testing](#building--testing)
5. [Code Conventions](#code-conventions)
6. [Integration Points](#integration-points)
7. [Debugging & Troubleshooting](#debugging--troubleshooting)
8. [Important Concepts](#important-concepts)

---

## 🎯 Project Overview

### What is Gorkbot?

Gorkbot is a sophisticated AI orchestration platform that:

1. **Integrates 5 AI Providers**: xAI Grok, Google Gemini, Anthropic Claude, OpenAI GPT, MiniMax
2. **Uses Orchestrator Pattern**: Coordinates primary and consultant AIs
3. **Implements Adaptive Routing**: ARC/CCI/MEL intelligence layers
4. **Provides Awareness Layer**: SENSE system for safety and quality
5. **Offers 75+ Tools**: File, git, web, system, security, Android, code execution, vision
6. **Features Professional TUI**: Bubble Tea-based terminal interface
7. **Supports Extended Thinking**: Token budgets for reasoning models
8. **Manages Sessions**: Checkpoints, export, relay, collaborative modes
9. **Enables Continual Learning**: XSKILL framework for skill evolution
10. **Provides Extensibility**: Dynamic tools, custom commands, MCP integration

### Key Design Patterns

1. **Orchestrator Pattern**: Central `orchestrator.go` coordinates all subsystems
2. **Adapter Pattern**: `OrchestratorAdapter` in commands for TUI decoupling
3. **Plugin System**: Tools implement `Tool` interface, providers implement `AIProvider`
4. **Elm MVC**: TUI follows Model-View-Update pattern
5. **Subsystem Architecture**: Modular, independent subsystems (ARC, CCI, MEL, SENSE)

---

## 🏗️ Architecture

### High-Level System Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    User Input (TUI)                         │
├─────────────────────────────────────────────────────────────┤
│
├─→ Prompt Builder
│    - User message
│    - System prompt
│    - SENSE injections (heuristics, compression, engrams)
│    - Context injection
│
├─→ ARC Router (Adaptive Response Classification)
│    - Classification: Direct/Analytical/Speculative
│    - Budget computation: tokens, temperature, tool limits
│    - Consistency checker (destructive ops)
│
├─→ Provider Selection + Native Function Calling
│    - Primary provider (Grok, Claude, GPT)
│    - Native function calling (if supported)
│    - Extended thinking (if model supports)
│
├─→ Streaming Token Generation
│    - Token-by-token streaming to TUI
│    - Tool calls detection
│    - Stop sequences
│
├─→ Tool Execution Loop (Parallel - max 8)
│    - Permission checks
│    - Parameter validation
│    - Tool execution
│    - Error recovery
│    - Audit logging
│    - Analytics tracking
│
├─→ SENSE Processing
│    - Input sanitization
│    - Quality criticism (stabilizer)
│    - Memory update (AgeMem, engrams)
│    - Compression
│
├─→ Context Management
│    - Token counting
│    - History truncation
│    - CCI layer updates
│
├─→ MEL Meta-Learning
│    - Observation of success/failure
│    - Heuristic generation
│    - VectorStore update
│
└─→ Output to User (TUI + Database)
```

### Orchestrator (`internal/engine/orchestrator.go` - 86KB)

The heart of Gorkbot. Responsibilities:

1. **Conversation Management**
   - Maintains `ConversationHistory` with full context
   - Tracks token usage and costs
   - Manages checkpoints (up to 20)
   - Handles multi-turn conversations

2. **Provider Orchestration**
   - Selects primary provider based on query
   - Manages fallback chain
   - Handles provider key rotation
   - Discovers available models

3. **Tool Execution**
   - Executes tool calls from AI
   - Runs tools in parallel (up to 8 workers)
   - Validates permissions
   - Recovers from errors
   - Tracks analytics

4. **Advanced Features**
   - Native function calling (xAI)
   - Extended thinking support
   - Background agent spawning
   - Worktree management
   - Brain file persistence

5. **Subsystem Coordination**
   - SENSE layer integration (input sanitization, quality criticism, memory)
   - ARC router (classification, budgeting)
   - CCI layer (context management, drift detection)
   - MEL system (heuristic generation, vector store)

### Key Subsystems

#### 1. ARC Router (pkg/adaptive/arc/)
**Adaptive Response Classification** - Intelligent routing and budgeting

```go
type RouteDecision struct {
    Classification WorkflowType  // Direct, Analytical, Speculative
    Budget ResourceBudget         // Token, temp, tool limits
    Timestamp time.Time
}

type ResourceBudget struct {
    MaxTokens int
    Temperature float32
    MaxToolCalls int
    Timeout time.Duration
}
```

**Behavior:**
- Classifies queries by semantic keywords and complexity
- Computes platform-aware budgets (HALProfile)
- Validates destructive operations
- Caches routing decisions

#### 2. CCI Layer (pkg/adaptive/cci/)
**Codified Context Infrastructure** - Context continuity across conversations

```go
type ColdMemory struct {
    Vector []float32
    Score float32
    Tier int  // 1=hot, 2=warm, 3=cold
    Confidence float32
}

type SpecialistModel struct {
    TaskType string
    Model string
    Confidence float32
}
```

**Features:**
- Hot memory: Recent context (immediate relevance)
- Cold memory: Historical context (vector search)
- Drift detection: Monitors context coherence
- Specialist delegation: Task-specific model selection

#### 3. MEL System (pkg/adaptive/mel/)
**Meta-Experience Learning** - Learns from execution traces

```go
type Heuristic struct {
    Template string  // "When [ctx], verify [constraint], avoid [error]"
    Context string
    Constraint string
    Avoidance string
    Confidence float32
}

type VectorStore struct {
    Heuristics []Heuristic
    BM25 map[string]float32  // Ranking
    Embeddings map[string][]float32
}
```

**Capabilities:**
- Generates heuristics from failed/successful executions
- Stores 500 items max with Jaccard similarity dedup (>70%)
- BM25 relevance ranking
- Bifurcation analysis (param diff comparison)

#### 4. SENSE Layer (pkg/sense/ - v1.9.0)
**Awareness & Safety System** - Input/output awareness

```go
type InputSanitizer struct {
    patterns map[string]string  // 19 injection patterns
}

type Stabilizer struct {
    // 4-dimensional quality critic
    CoherentQuality float32
    ContextualConsistency float32
    TokenEfficacy float32
    SafetyScore float32
}

type Memory interface {
    Short() []Event      // STM - immediate recall
    Medium() []Event     // MTM - recent context
    Long() []Event       // LTM - episodic memory
}

type LIEReward struct {
    // Output evaluation model
    Helpfulness float32
    Truthfulness float32
    Safety float32
}
```

**Functions:**
- **Input Sanitizer**: Detects 19 attack patterns + context scanning
- **Tracer**: Async JSONL event logging
- **Stabilizer**: 4-dimensional quality criticism
- **Compression**: 4-stage context reduction
- **AgeMem**: 3-tier STM/LTM memory with time decay
- **Engrams**: Episodic memory for high-value interactions
- **LIE Reward Model**: Evaluates output quality

#### 5. Tool System (pkg/tools/ - 75+ tools)
**Comprehensive Tool Registry**

```go
type Tool interface {
    Name() string
    Description() string
    Category() string
    Parameters() []ParameterDef
    Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

type PermissionLevel int
const (
    Always PermissionLevel = iota  // Persistent approval
    Session                        // Current session only
    Once                           // Ask each time
    Never                          // Blocked
)
```

**Categories:**
- Shell (1): bash
- File (7): read_file, write_file, list_directory, search_files, grep_content, file_info, delete_file
- Git (6): git_status, git_diff, git_log, git_commit, git_push, git_pull
- Web (6): web_fetch, http_request, browser_control, download_file, check_port
- System (6): process_list, kill_process, env_var, system_info, disk_usage
- Security (32+): nmap_scan, sqlmap_scan, nuclei_scan, etc.
- Android (6): adb_setup, android_apps, android_control, etc.
- Code (4): python_sandbox, jupyter, code_exec, structured_bash
- Vision (5+): vision, media_ops, browser_scrape, screenshot_capture
- Data Science (3+): ML/stats operations
- Memory (5+): brain_tools (long-term memory)
- Multi-Agent (3+): colony_tool, spawn_agent
- Meta (3): list_tools, tool_info, create_tool

**Infrastructure:**
- `registry.go` (33KB): Registry, permissions, analytics
- `dispatcher.go`: Parallel execution (8 workers max)
- `cache.go`: TTL-based memoization
- `permissions.go`: Permission manager
- `rules.go`: Fine-grained rule engine
- `error_recovery.go`: Error classification
- `analytics.go`: Execution analytics
- `audit_db.go`: SQLite audit logging

#### 6. TUI (internal/tui/ - 40+ files)
**Terminal User Interface** - Elm MVC architecture

```go
type Model struct {
    // State
    messages []Message
    inputBuffer string
    viewport viewport.Model
    conversation ConversationHistory

    // Subsystem refs
    orchestrator *engine.Orchestrator
    themeManager *theme.Manager

    // Internal state
    streaming bool
    focusedInput bool
    selectedModel string
}

type Msg interface{}  // Custom message types

// Standard messages
type StreamChunkMsg string
type ErrorMsg error
type ToolStartMsg struct { ToolName string }
type ToolCompleteMsg struct { ToolName, Result string }
```

**Core Files:**
- `model.go`: State management and struct definitions
- `update.go`: Event handling (keyboard, streaming, commands)
- `view.go`: Rendering pipeline
- `keys.go`: Keybinding definitions
- `messages.go`: Custom message types
- `style.go`: Lip Gloss styling and theme system
- `statusbar.go`: Status bar rendering
- `model_select_view.go`: Dual-pane model selector (Ctrl+T)
- `settings_overlay.go`: 4-tab settings modal (Ctrl+G)
- `list.go`: Model list rendering
- `README.md`: TUI documentation

**Features:**
- Markdown rendering with syntax highlighting
- Token streaming with progress indicators
- Keyboard shortcuts (30+)
- Multiple views (chat, model select, tools, cloud brains, analytics, diagnostics)
- Touch scroll support (Android)
- Overlays (settings, SENSE HITL, API key entry)

#### 7. AI Providers (pkg/ai/ - 16 files, ~120KB)

```go
type AIProvider interface {
    // Core generation
    Generate(ctx context.Context, history ConversationHistory) (string, error)
    GenerateWithHistory(ctx context.Context, ...) (string, error)
    Stream(ctx context.Context, ...) (<-chan string, error)

    // Function calling (if supported)
    GenerateWithTools(ctx context.Context, ...) (CallsAndText, error)

    // Model info
    ListModels(ctx context.Context) ([]Model, error)
    GetCapabilities(modelID string) Capabilities
}
```

**Providers Implemented:**
1. **xAI Grok** (grok.go - 21KB)
   - Native function calling support
   - Extended thinking (reasoning tokens)
   - Models: grok-3-mini, grok-3-vision, grok-3-thinking
   - API: https://api.x.ai/v1/chat/completions

2. **Google Gemini** (gemini.go - 16KB)
   - Streaming support
   - Vision capability
   - Models: gemini-2.0-pro, gemini-2.0-flash, gemini-1.5-*
   - API: https://generativelanguage.googleapis.com/v1beta/models

3. **Anthropic Claude** (anthropic.go - 23KB)
   - Extended thinking support
   - Native function calling
   - Models: claude-opus-4-1, claude-sonnet-4, claude-3.7+
   - API: https://api.anthropic.com/v1/messages

4. **OpenAI GPT** (openai_provider.go)
   - Chat completions
   - Function calling
   - Vision support (GPT-4)
   - Models: gpt-4-turbo, gpt-4o, gpt-3.5-turbo
   - API: https://api.openai.com/v1/chat/completions

5. **MiniMax** (minimax.go)
   - OpenAI-compatible wrapper
   - Models: minimax-01
   - API: https://api.minimax.io/anthropic/v1

**Message Types:**
```go
type ConversationMessage struct {
    Role string        // "user", "assistant", "tool"
    Content string
    ToolName string
    ToolCallID string
    ToolCalls []ToolCallEntry
}

type ToolCallEntry struct {
    ID string
    Name string
    Input map[string]interface{}
}
```

---

## 📁 Directory Structure

### Entry Points (cmd/)
```
cmd/
├── gorkbot/           # Main TUI application (350+ lines)
│   ├── main.go       # Entry point, provider init, TUI bootstrap
│   ├── setup.go      # Setup wizard
│   ├── flags.go      # CLI flag definitions
│   └── ...
└── ...               # grokster, gorkweb, debug-mcp, etc.
```

### Core Engine (internal/engine/ - 29 files, 347KB)
```
internal/engine/
├── orchestrator.go          # (86KB) Main orchestrator
├── streaming.go             # (41KB) Token streaming + relay
├── prompt_builder.go        # (11KB) System prompt construction
├── context_manager.go       # Token tracking + context window
├── consultation/            # Gemini consultation subsystem
├── compression_pipe.go      # Compression pipeline
├── tiered_compaction.go     # Multi-tier compaction
├── intelligence.go          # Intelligence layer init
├── provider_routing.go      # Dynamic provider selection
├── plan_mode.go             # Plan/execute execution modes
├── sense_hitl.go            # SENSE human-in-the-loop approval
├── budget_guard.go          # Resource budget enforcement
├── background_agents.go     # Background agent spawning
├── brain.go                 # Long-term brain file operations
├── crystallizer.go          # Memory crystallization
├── fallback.go              # Fallback handling
├── intent_gate.go           # Intent classification
├── introspection.go         # System introspection
├── reasoning_hooks.go       # Reasoning phase hooks
├── trace.go                 # Execution trace logging
├── watchdog.go              # State debugging
├── ralph.go                 # Ralph memory system
├── rag_injector.go          # RAG injection
├── xskill_adapter.go        # XSKILL integration
├── xskill_hooks.go          # XSKILL lifecycle
├── spark_hooks.go           # SPARK motivation
└── ...                      # Other specialized modules
```

### Terminal UI (internal/tui/ - 40+ files)
```
internal/tui/
├── model.go                 # State management (Model struct)
├── update.go                # Event handling (Update function)
├── view.go                  # Rendering (View function)
├── keys.go                  # Keybindings
├── messages.go              # Custom message types
├── style.go                 # Theming with Lip Gloss
├── statusbar.go             # Status bar rendering
├── model_extensions.go      # Extended methods
├── model_select_view.go     # Model selection UI
├── api_key_prompt.go        # API key entry modal
├── settings_overlay.go      # Settings modal
├── discovery_view.go        # Cloud Brains view
├── list.go                  # Model list items
├── README.md                # TUI documentation
└── ...
```

### Platform Abstraction (internal/platform/)
```
internal/platform/
├── env.go                   # OS detection, paths, HALProfile
└── ...
```

### AI Providers (pkg/ai/ - 16 files, ~120KB)
```
pkg/ai/
├── interface.go             # AIProvider interface
├── grok.go                  # (21KB) xAI Grok provider
├── gemini.go                # (16KB) Google Gemini
├── anthropic.go             # (23KB) Anthropic Claude
├── openai_provider.go       # OpenAI GPT
├── minimax.go               # MiniMax wrapper
├── moonshot.go              # Moonshot integration
├── openrouter.go            # OpenRouter gateway
├── conversation.go          # (12KB) Message history management
├── discovery.go             # Model discovery
├── transport.go             # HTTP transport with retries
├── retry_client.go          # Retry logic with backoff
├── stream_guard.go          # Stream safety validation
├── safe_models.go           # Model allowlisting
└── ...
```

### Adaptive Intelligence (pkg/adaptive/ - 38 files, ~400KB)
```
pkg/adaptive/
├── arc/                     # Adaptive Response Classification
│   ├── router.go           # Main router
│   ├── classifier.go       # Query classifier
│   ├── budget.go           # Resource budgeting
│   ├── categories.go       # Workflow categories
│   ├── consistency.go      # Consistency checker
│   └── trigger_tables.go   # Decision caching
├── cci/                     # Codified Context Infrastructure
│   ├── hot_memory.go       # Recent context
│   ├── cold_memory.go      # Historical context + vector search
│   ├── drift_detector.go   # Drift monitoring
│   ├── specialist.go       # Task-specific model delegation
│   └── tier_system.go      # 3-tier relevance scoring
├── mel/                     # Meta-Experience Learning
│   ├── analyzer.go         # Bifurcation analyzer
│   ├── heuristic.go        # Heuristic definitions
│   ├── store.go            # VectorStore with Jaccard similarity
│   ├── bm25.go             # BM25 ranking
│   ├── projection.go       # Vector projection methods
│   ├── validator.go        # Heuristic validation
│   └── ...
└── routing_table.go        # Decision caching + stats
```

### SENSE Layer (pkg/sense/ - v1.9.0)
```
pkg/sense/
├── input_sanitizer.go      # 19 injection patterns + context scanning
├── tracer.go               # Async JSONL event logging
├── stabilizer.go           # 4-dimensional quality critic
├── compression.go          # 4-stage context compression
├── agemem.go               # 3-tier STM/LTM memory
├── engrams.go              # Episodic memory store
├── lie.go                  # LIE reward model
├── discovery.go            # Tool registry self-description
├── skill_evolver.go        # Skill evolution from traces
├── trace_analyzer.go       # JSONL analysis pipeline
└── ...
```

### Tool System (pkg/tools/ - 75+ tools)
```
pkg/tools/
├── registry.go             # (33KB) Tool registry + registry operations
├── tool.go                 # Tool interface definition
├── permissions.go          # Permission manager
├── cache.go                # TTL-based memoization
├── dispatcher.go           # Parallel execution (8 workers)
├── rules.go                # Fine-grained permission rules
├── error_recovery.go       # Error classification + recovery
├── analytics.go            # Execution analytics
├── audit_db.go             # (16KB) SQLite audit logging
├── helpers.go              # Common utilities
├── normalizer.go           # Parameter normalization
├── parser.go               # Parameter parsing
├──
├── # File operations
├── file_ops.go             # read_file, write_file, delete_file
├── directory_ops.go        # list_directory
├── file_search.go          # search_files, grep_content, file_info
├──
├── # VCS/Git
├── git_ops.go              # git_status, git_diff, git_log, git_commit, git_push, git_pull
├──
├── # Web/HTTP
├── web_ops.go              # web_fetch, http_request, browser_control
├── download.go             # download_file, check_port
├── browser.go              # browser_scrape
├──
├── # System
├── system_ops.go           # bash, system_info, process_list, kill_process, disk_usage, env_var
├──
├── # Security/Pentesting (32+ tools)
├── pentest.go              # (64KB) Comprehensive security suite
│                            # nmap_scan, sqlmap_scan, nuclei_scan,
│                            # exploit_db, vulnerability_scan, etc.
├── advanced.go             # (32KB) Advanced security operations
├── scrapling.go            # (20KB) Web scraping operations
├──
├── # Android-specific
├── android_ops.go          # adb_setup, android_apps, android_control
├── android_system.go       # android_system, android_intents
├── android_accessibility.go # Android accessibility features
├──
├── # Code execution
├── python_sandbox.go       # python_sandbox
├── jupyter.go              # jupyter integration
├── code_exec.go            # code_exec, structured_bash
├──
├── # Vision/Media
├── vision.go               # Screen capture, image analysis
├── media_ops.go            # Media operations
├── screenshot.go           # Screenshot capture
├──
├── # Data science
├── data_science.go         # ML/stats operations
├──
├── # Memory
├── brain_tools.go          # Long-term memory operations
├──
├── # Multi-agent
├── colony_tool.go          # Multi-agent debate
├── spawn_agent.go          # Subagent spawning
├──
├── # CCI integration
├── cci_tools.go            # CCI layer introspection
├──
├── # Meta/admin
├── consult.go              # Consultant tool
├── skill_tools.go          # Skill management
├── sense_self_tools.go     # SENSE self-reflection
├── worktrees.go            # Worktree management
├── task_mgmt.go            # Task scheduling
├── task_schedule.go        # Task execution
├──
├── custom/                 # Dynamically generated tools (code fragments)
│   └── *.go                # Generated by create_tool meta-tool
└── ...
```

### Commands (pkg/commands/)
```
pkg/commands/
├── registry.go             # (100+ line) Command dispatcher + OrchestratorAdapter
│                            # 30+ slash commands registered here
└── ...
```

### Configuration & Persistence
```
pkg/config/
├── appstate.go            # App state manager (models, disabled categories)
├── loader.go              # GORKBOT.md hierarchical config
└── watcher.go             # Config file watcher

pkg/persist/
├── database.go            # SQLite conversation store
├── checkpoint.go          # Conversation checkpoints
└── search.go              # Full-text search
```

### Advanced Features
```
pkg/adaptive/              # Intelligence layer (ARC, CCI, MEL)
pkg/sense/                 # Awareness layer (v1.9.0)
pkg/ai/                    # AI provider abstraction
pkg/discovery/             # Model discovery from 5 providers
pkg/session/               # Session checkpoints, export, load
pkg/memory/                # Memory management and retrieval
pkg/vectorstore/           # Vector search implementation
pkg/embeddings/            # Embedding generation
pkg/skills/                # Dynamic skill system
pkg/theme/                 # Theme management (5 built-ins + custom)
pkg/security/              # Encryption, key management
pkg/channels/              # Discord, Telegram, bridge
pkg/mcp/                   # Model Context Protocol client
pkg/subagents/             # Subagent spawning, worktrees
pkg/pipeline/              # Multi-step agent pipelines
pkg/scheduler/             # Cron-based scheduling
pkg/spark/                 # SPARK motivation module
pkg/sre/                   # Streaming Response Execution
pkg/xskill/                # Continual learning framework
pkg/colony/                # Multi-agent debate
pkg/vision/                # Computer vision integration
pkg/billing/               # Token cost tracking
pkg/notify/                # Desktop notifications
pkg/webhook/               # Event triggers
pkg/router/                # Feedback-based routing
pkg/hooks/                 # Lifecycle hooks
pkg/providers/             # Provider key management
pkg/auth/                  # OAuth (mostly deprecated)
pkg/process/               # Process management
pkg/python/                # Python integration
pkg/dag/                   # DAG execution
pkg/version/               # Version management
pkg/schema/                # Schema validation
pkg/usercommands/          # Custom commands
```

---

## 🛠️ Building & Testing

### Build Commands

```bash
# Build for current OS (outputs to ./bin/gorkbot)
make build

# Cross-platform builds
make build-linux    # Linux amd64
make build-windows  # Windows amd64
make build-android  # Android arm64

# Clean artifacts
make clean

# Install to GOPATH/bin
make install

# Run (build + execute)
make run

# View Makefile targets
make help
```

### Testing

```bash
# Run all tests
go test ./...

# Test specific package
go test ./pkg/ai
go test ./pkg/tools
go test ./internal/engine
go test ./pkg/adaptive

# Verbose output
go test -v ./...

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -run TestName ./pkg/ai

# With race detector
go test -race ./...

# Integration tests (if available)
go test -tags=integration ./...
```

### Linting & Code Quality

```bash
# Lint (if configured)
golangci-lint run ./...

# Format code
go fmt ./...

# Vet for potential issues
go vet ./...

# Simplify code
gosimple ./...

# Find inefficiencies
ineffassign ./...
```

### Building with Submodules

```bash
# Initialize submodules (if any)
git submodule update --init --recursive

# Build with submodules included
make build
```

---

## 💾 Code Conventions

### Package Structure
- Follows Go standard layout: `cmd/`, `internal/`, `pkg/`
- `cmd/` - Executable binaries
- `internal/` - Private packages (not importable outside project)
- `pkg/` - Public packages (importable by external projects)

### Naming Conventions
- **Files**: snake_case (e.g., `orchestrator.go`)
- **Types**: PascalCase (e.g., `ConversationHistory`)
- **Functions**: PascalCase for exported, camelCase for unexported
- **Constants**: PascalCase for exported, UPPER_CASE for constants
- **Variables**: camelCase (e.g., `inputBuffer`)

### Interface Design
```go
// AI Providers must implement AIProvider
type AIProvider interface {
    Generate(ctx context.Context, history ConversationHistory) (string, error)
    GenerateWithHistory(...) (string, error)
    Stream(ctx context.Context, ...) (<-chan string, error)
    ListModels(ctx context.Context) ([]Model, error)
}

// Tools must implement Tool
type Tool interface {
    Name() string
    Description() string
    Category() string
    Parameters() []ParameterDef
    Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// Commands use function handlers with OrchestratorAdapter
type OrchestratorAdapter struct {
    // Function references (not struct embedding)
    GetContextReport func() string
    GetCostReport func() string
    GetCheckpoints func() []Checkpoint
    // ... 20+ more functions
}
```

### Message Types (TUI)
Use typed structs instead of raw strings:

```go
// ✓ Correct
type StreamChunkMsg string
type ErrorMsg error
type ToolStartMsg struct {
    ToolName string
    Timestamp time.Time
}

// ✗ Incorrect
m := "just a string"  // Don't use raw strings for messages
```

### Error Handling
- Explicit error returns (no panics except at init)
- Error wrapping with context: `fmt.Errorf("context: %w", err)`
- Type assertions for error recovery
- Timeouts on external operations

```go
// ✓ Correct
result, err := tool.Execute(ctx, params)
if err != nil {
    return fmt.Errorf("tool execution failed: %w", err)
}

// ✗ Incorrect
result := tool.Execute(ctx, params) // Ignores errors
panic(err)  // Never panic in production code
```

### Logging
Use structured logging with slog (JSON format):

```go
// ✓ Correct
slog.Info("tool executed", "tool", toolName, "duration_ms", elapsed)
slog.Error("API error", "provider", provider, "status", status, "error", err)

// ✗ Incorrect
log.Println("Tool: " + toolName)  // Unstructured
fmt.Printf("Error: %v\n", err)    // Not logged to file
```

### Concurrency
- Use `context.Context` for cancellation and timeouts
- Use `sync.WaitGroup` for goroutine coordination
- Use channels for communication
- Protect shared state with `sync.Mutex`

```go
// ✓ Correct
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    // Work
}()
wg.Wait()

// ✗ Incorrect
for {
    // No timeout = potential hang
}
```

### File Organization
- One type per file (if significant size)
- Related functions in same file
- Test files in same directory with `_test.go` suffix
- Internal helpers in unexported functions

```
pkg/tools/
├── tool.go              # Tool interface
├── registry.go          # Registry implementation (33KB - large, OK)
├── file_ops.go          # File operation tools
├── git_ops.go           # Git operation tools
└── registry_test.go     # Registry tests
```

---

## 🔌 Integration Points

### Adding a New AI Provider

1. **Create provider file** (`pkg/ai/newprovider.go`):
```go
type NewProviderConfig struct {
    APIKey string
    BaseURL string
}

type NewProvider struct {
    config NewProviderConfig
}

func (p *NewProvider) Generate(ctx context.Context, history ConversationHistory) (string, error) {
    // Implementation
}

func (p *NewProvider) ListModels(ctx context.Context) ([]Model, error) {
    // Implementation
}
```

2. **Register in discovery** (`pkg/discovery/manager.go`):
```go
func (m *Manager) fetchNewProviderModels() error {
    // Fetch models from API
    m.models = append(m.models, discoveredModels...)
}
```

3. **Add to provider manager** (`pkg/providers/manager.go`):
```go
func (pm *Manager) GetProviderForModel(modelID string) AIProvider {
    if strings.HasPrefix(modelID, "newprovider-") {
        return pm.newProvider
    }
}
```

4. **Wire in main.go**:
```go
newProvider := ai.NewProvider(config)
providerManager.RegisterProvider("newprovider", newProvider)
```

### Adding a New Tool

1. **Create tool file** (`pkg/tools/new_tool.go`):
```go
type NewToolImpl struct {}

func (t *NewToolImpl) Name() string {
    return "new_tool"
}

func (t *NewToolImpl) Description() string {
    return "Description of what tool does"
}

func (t *NewToolImpl) Category() string {
    return "file"  // or other category
}

func (t *NewToolImpl) Parameters() []ParameterDef {
    return []ParameterDef{
        {Name: "param1", Type: "string", Required: true},
    }
}

func (t *NewToolImpl) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // Implementation
}
```

2. **Register in registry** (`pkg/tools/registry.go`):
```go
func RegisterDefaultTools(reg *Registry) error {
    // ... existing tools ...
    reg.Register(&NewToolImpl{})
    return nil
}
```

3. **Test**:
```bash
go test ./pkg/tools
```

### Adding a New Slash Command

1. **Define handler** in `pkg/commands/registry.go`:
```go
type OrchestratorAdapter struct {
    // ... existing ...
    MyCommand func(args string) string
}
```

2. **Register in registry**:
```go
func (r *Registry) Execute(command string, args string) (string, bool) {
    switch command {
    // ... existing ...
    case "mycommand":
        if r.adapter.MyCommand != nil {
            return r.adapter.MyCommand(args), true
        }
    }
    return "", false
}
```

3. **Implement in main.go**:
```go
cmdReg.adapter.MyCommand = func(args string) string {
    // Implementation
    return result
}
```

4. **Handle in TUI** (`internal/tui/update.go`):
```go
case "MYCOMMAND_SIGNAL":
    // Handle the result
```

### Modifying Orchestrator

1. **Edit** `internal/engine/orchestrator.go`
2. **Key methods to modify**:
   - `ExecuteWithTools()` - Main loop
   - `ParseToolRequests()` - Tool call parsing
   - `ExecuteTool()` - Individual tool execution
   - `BuildSystemPrompt()` - System prompt construction
3. **Maintain subsystem interfaces** (ARC, CCI, MEL, SENSE)
4. **Test**: `go test ./internal/engine`

### Adding TUI Features

1. **Add custom message type** (`internal/tui/messages.go`):
```go
type MyFeatureMsg struct {
    Data string
}
```

2. **Update model** (`internal/tui/model.go`):
```go
type Model struct {
    // Add state
    myFeatureState bool
}
```

3. **Handle update** (`internal/tui/update.go`):
```go
case MyFeatureMsg:
    m.myFeatureState = msg.Data
    return m, nil
```

4. **Add rendering** (`internal/tui/view.go`):
```go
if m.myFeatureState {
    // Render custom UI
}
```

---

## 🐛 Debugging & Troubleshooting

### Debug Modes

```bash
# Enable orchestrator debugging
./gorkbot.sh -watchdog
# Shows: routing decisions, tool calls, context usage

# Enable verbose consultant thinking
./gorkbot.sh -verbose-thoughts
# Shows: Gemini consultation responses

# Enable execution tracing
./gorkbot.sh --trace
# Creates: ~/.local/share/gorkbot/traces/*.jsonl
```

### Checking Logs

```bash
# View all logs
tail -f ~/.local/share/gorkbot/gorkbot.json | jq .

# Filter by level
tail -f ~/.local/share/gorkbot/gorkbot.json | jq 'select(.level=="ERROR")'

# Filter by component
tail -f ~/.local/share/gorkbot/gorkbot.json | jq 'select(.component=="orchestrator")'
```

### Debugging Tool Execution

```bash
# Check tool registry
./gorkbot.sh -p "/tools"

# Check tool permissions
cat ~/.config/gorkbot/tool_permissions.json | jq .

# Check tool audit log
sqlite3 ~/.local/share/gorkbot/tool_audit.db "SELECT * FROM executions LIMIT 10;"
```

### Common Issues

**Build failures:**
```bash
go clean -cache
go mod download
make clean
make build
```

**Import cycles:**
- `internal/engine` cannot import `internal/tui`
- Use `OrchestratorAdapter` (function refs) instead of struct embedding

**Missing dependencies:**
```bash
go mod tidy
go mod download
```

**API connection issues:**
```bash
# Test connectivity
curl https://api.x.ai/health
curl https://generativelanguage.googleapis.com/

# Check logs for errors
tail -f ~/.local/share/gorkbot/gorkbot.json | jq '.errors'
```

---

## 💡 Important Concepts

### Conversation History Management

```go
type ConversationHistory struct {
    Messages []ConversationMessage
    TokenCount int
    MaxTokens int
}

func (h *ConversationHistory) TruncateToTokenLimit(ctx context.Context) error {
    // Removes oldest messages until within token budget
    // Preserves system message and recent context
}
```

**Important**: When messages are oversized (>token limit), use `continue` (not `break`) so they're skipped but don't drop all history.

### Native Function Calling

For providers like xAI Grok that support native function calling:

```go
type GrokNativeRequest struct {
    Tools []GrokToolCall
    ToolChoice "auto"  // "auto" | "required" | specific tool name
}

// Orchestrator checks if provider implements NativeToolCaller
if nativeCaller, ok := provider.(ai.NativeToolCaller); ok {
    // Use native path (tools in API request)
    result, _ := nativeCaller.GenerateWithTools(ctx, history, tools)
} else {
    // Fall back to text parsing
    text, _ := provider.Generate(ctx, history)
    // Parse tool calls from text
}
```

### Tool Message Handling

Tool results are returned as messages with `role:"tool"`:

```go
history.AddToolResultMessage(toolCallID, toolName, result)
// Becomes: ConversationMessage{Role: "tool", Content: result, ...}
```

**Note**: Non-native providers need text fallback:
```go
// Instead of role:"tool", use user message with formatted result
history.AddMessage("user", fmt.Sprintf("Tool %s result: %v", toolName, result))
```

### Extended Thinking Support

For models with extended thinking (reasoning tokens):

```go
// Set thinking budget
req.ThinkingBudget = 15000  // tokens for reasoning

// Response includes:
response.ThinkingContent  // Explicit thinking block
response.Content          // Final answer
response.ThinkingTokens   // Tokens used for thinking
```

### SENSE Integration

The SENSE layer is called at key points:

1. **Before API call**: Input sanitization, heuristic injection
2. **During streaming**: Quality criticism (stabilizer)
3. **After response**: Memory update (AgeMem, engrams), compression

```go
// Inject SENSE
sanityCheck := senseLayer.InputSanitizer.Validate(userMessage)
heuristics := senseLayer.VectorStore.Retrieve(query)
systemPrompt += heuristics  // Injected into prompt

// Process response
criticism := senseLayer.Stabilizer.Evaluate(response)
if criticism.SafetyScore < 0.5 {
    // Flag for HITL approval
}
```

### CCI Context Management

CCI manages two types of memory:

1. **Hot Memory**: Recent context (immediate relevance)
2. **Cold Memory**: Historical context (semantic search)

```go
// Hot memory is automatically maintained
// Cold memory requires semantic search

embedding := embedder.Embed(query)
relevantHistory := cci.ColdMemory.Search(embedding, topK=5)
```

### MEL Learning Cycle

MEL observes outcomes and generates heuristics:

```go
// After tool execution
mel.ObserveFailed(toolName, params, error)
// Analyzes param diff with similar past failures
// Generates heuristic: "When [context], verify [constraint]"

mel.ObserveSuccess(toolName, params, result)
// Strengthens heuristics that led to success
```

### Session Checkpointing

Save conversation state for rollback:

```go
checkpoint := orchestrator.CreateCheckpoint()
// Do something risky...
if error {
    orchestrator.RewindTo(checkpoint.ID)
    // Conversation reverted
}
```

### Token Tracking

Track token usage across providers:

```go
// After each AI call
orchestrator.contextManager.UpdateFromUsage(TokenUsage{
    InputTokens: 150,
    OutputTokens: 80,
    Provider: "xai",
    Model: "grok-3-mini",
})

// Query stats
report := orchestrator.contextManager.GetContextReport()
// Returns: % used, costs, token counts
```

---

## 📊 Project Statistics

| Metric | Value |
|--------|-------|
| Total Go Files | 261 |
| Total Lines of Code | ~150,000+ |
| Tool System | 24,823 lines (75 tools) |
| Engine | 347 KB (29 files) |
| AI Providers | ~120 KB (16 files) |
| Intelligence Layer | ~400 KB (38 files) |
| SENSE Layer | v1.9.0 (11 files) |
| TUI | 40+ files (Elm MVC) |
| Public Packages | 44 subsystems |
| Supported Providers | 5 (xAI, Google, Anthropic, OpenAI, MiniMax) |
| Built-in Tools | 75+ |
| Slash Commands | 30+ |

---

## 🚀 Development Workflow

### Making Changes

1. **Create feature branch**:
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Make changes** following code conventions

3. **Test**:
   ```bash
   go test ./...
   go test -race ./...  # Check for races
   ```

4. **Lint**:
   ```bash
   go fmt ./...
   go vet ./...
   ```

5. **Build**:
   ```bash
   make clean
   make build
   ```

6. **Commit**:
   ```bash
   git add .
   git commit -m "feat: describe your change"
   ```

7. **Push & create PR**:
   ```bash
   git push origin feature/my-feature
   # Create PR on GitHub
   ```

### Versioning
- **Public**: Semantic (1.2.0-beta)
- **Internal**: Development track (5.3.0)
- **Subsystems**: Independent (SENSE 1.9.0, SRE 1.0.0, XSKILL 1.0.0)

---

## 📖 Additional Resources

- **README.md** - Project overview and quick start
- **GETTING_STARTED.md** - User guide with examples
- **VERSIONING.md** - Version information
- **docs/** directory - Detailed documentation
- **go.mod** - Dependency list with versions

---

**Built with ❤️ by Velarium AI**

For questions or contributions, refer to the documentation or check GitHub issues.
