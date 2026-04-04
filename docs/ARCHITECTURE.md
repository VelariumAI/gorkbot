# Gorkbot Architecture

## Phase 2A: Pure Native Go Implementation

The current implementation (v2.0+) is a pure Go orchestration platform with zero external service dependencies.

### Core Systems

#### Configuration (`pkg/config/`)
- **YAML Configuration**: Hot-reloadable `gorkbot.yaml` with env var expansion
- **Config Watcher**: Polls for changes on 2s interval, applies updates without restart
- **Schema Types**: `ModelConfig`, `SandboxConfig`, `GuardrailsConfig`

#### Skills System (`pkg/skills/`)
- **Skill Loader**: Discovers SKILL.md files in `~/.config/gorkbot/skills/` and `.gorkbot/skills/`
- **Skill Registry**: Thread-safe access with Get/List/Reload operations
- **Hot Reload**: Watches for `.md` changes and updates in-memory index

#### Memory Systems (`pkg/memory/`)
1. **FactStore** (SQLite): LLM-extracted facts with deduplication
2. **AgeMem** (In-Memory + LTM): Time-ranked episodic facts
3. **Engrams**: Learned tool preferences
4. **MEL** (Multi-Stage Executor Learning): Bifurcation-generated heuristics
5. **Unified Interface**: Single query over all four systems

#### Middleware Chain (`internal/engine/middleware.go`)
12-layer ordered composition for tool execution:
1. Plan mode blocker
2. Rule engine evaluator
3. Tool cache (read-only TTL)
4. Pre-execution hooks
5. Workspace checkpoint
6. Human-in-the-loop guard
7. Input sanitization
8. Safety guardrails
9. Sandbox routing
10. Post-execution hooks
11. AgeMem storage
12. SENSE tracing

#### Provider System (`pkg/provider/` + `pkg/providers/`)
- **Factory Registry**: Dynamic provider instantiation from `Use` key
- **Manager**: Caches provider instances, applies custom fields
- **Supported**: Anthropic, Google Gemini, xAI Grok, custom implementations
- **Sandboxing**: Local sandbox with path confinement (Docker deferred)

#### Orchestrator (`internal/engine/orchestrator.go`)
Central coordinator managing:
- Primary + Consultant AI providers
- Tool registry + execution pipeline
- Memory systems (AgeMem, Engrams, FactStore, MEL)
- Skill registry + hot-reload
- Middleware chain construction
- Configuration application + hot updates
- Session management + persistence

### Data Flow

```
User Input
    ↓
[Config Injection]
  ├─ Project Instructions (GORKBOT.md)
  ├─ Skills Index (SKILL.md)
  └─ Memory Context (GoalLedger + FactStore + AgeMem + Engrams + MEL)
    ↓
[Primary Model]
  └─ Generate response/tool calls
    ↓
[Tool Execution Pipeline]
  ├─ PlanMode check
  ├─ RuleEngine evaluation
  ├─ ToolCache lookup
  ├─ PreHooks fire
  ├─ Checkpoint save
  ├─ HITL approval
  ├─ InputSanitizer
  ├─ Guardrails check
  ├─ Sandbox routing
  ├─ Execute tool
  ├─ PostHooks fire
  ├─ AgeMem storage
  └─ SENSE tracing
    ↓
[Memory Updates]
  ├─ FactStore.QueueForExtraction (debounced 5s)
  └─ AgeMem.Store (immediate)
    ↓
User Output + Tool Results
```

### Key Architectural Decisions

#### Pure Go
- ✅ No Python subprocess, no gRPC bridge
- ✅ Native goroutines for concurrent subagents
- ✅ SQLite for persistence (modernc.org/sqlite, no CGO)
- ✅ Compiles to single binary, runs anywhere Go runs

#### DeerFlow Patterns Reimplemented
- ✅ Config-driven system prompt injection (YAML)
- ✅ Reflection-based provider factory
- ✅ Middleware chain for cross-cutting concerns
- ✅ Skills registry with YAML frontmatter
- ✅ Hot-reload via ConfigWatcher

#### Backward Compatible
- ✅ Existing features continue working
- ✅ Config files optional (defaults to zero values)
- ✅ New systems are opt-in
- ✅ No breaking changes to existing APIs

#### Extensible
- ✅ Custom providers via factory registration
- ✅ Custom middleware via Chain.Use()
- ✅ Custom skills via SKILL.md format
- ✅ Plugin points: Hooks, Guardrails, Sandbox, Memory

### Configuration Flow

```yaml
gorkbot.yaml
    ├─ model.use → ResolveFromConfig → Primary provider selected
    ├─ model.custom_fields → applyCustomFields → ThinkingBudget applied
    ├─ sandbox → ResolveSandboxFromConfig → SandboxProv initialized
    ├─ guardrails → ResolveGuardrailsFromConfig → GuardrailsProv initialized
    └─ ConfigWatcher polls → ApplyConfig → ToolChain rebuilt on change
```

### Memory Architecture

```
Unified Query (maxTokens):
    ├─ GoalLedger (prospective)
    ├─ FactStore.QueryRelevant (keyword-ranked facts)
    ├─ AgeMem.FormatRelevant (recency × priority)
    ├─ Engrams.FormatAsContext (tool preferences)
    └─ MEL.Query (heuristics)
         ↓
    [Dedup by MD5]
    [Truncate to maxTokens]
         ↓
    Formatted context string
```

### Deployment

**Single Binary**: `./gorkbot`
- Contains all systems (no external services)
- Config at `~/.config/gorkbot/gorkbot.yaml` (optional)
- Data at `~/.gorkbot/data/` (skills, memory, checkpoints)
- Runs on Linux, macOS, Windows (any platform Go supports)

---

## See Also
- [Phase 2A Implementation Guide](./PHASE2A_IMPLEMENTATION.md) — Detailed configuration, API reference, troubleshooting
- [Skills Authoring Guide](./PHASE2A_IMPLEMENTATION.md#skills-system) — How to write SKILL.md files
- [Provider Configuration](./PHASE2A_IMPLEMENTATION.md#provider-configuration) — Available providers and custom fields
- [Middleware Extension](./PHASE2A_IMPLEMENTATION.md#extending-the-chain) — How to add custom middleware

