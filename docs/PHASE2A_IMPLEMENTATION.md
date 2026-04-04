# Phase 2A: Pure Native Go Implementation

**Status**: Complete | **Target**: Go 1.25+ | **Database**: SQLite (modernc.org/sqlite, no CGO)

## Overview

Phase 2A implements a complete, self-contained AI orchestration platform in pure Go with zero external service dependencies. All DeerFlow patterns (config-driven, reflection-based, middleware chain, skills system) are extracted and reimplemented natively in Go, enabling Gorkbot to function as a standalone, extensible AI agent framework.

### Architecture Principles

✅ **No External Services** — Pure Go, SQLite, no Python subprocess, no gRPC bridge
✅ **DeerFlow Patterns** — Config-driven system prompt, skills registry, middleware chain, reflection-based instantiation
✅ **Native Concurrency** — Goroutines for subagents, background memory updates, async hooks
✅ **Persistent Storage** — SQLite with atomic transactions, ACID guarantees
✅ **Backward Compatible** — All existing Gorkbot features continue working unmodified
✅ **Highly Extensible** — New providers, skills, middleware via configuration without code recompilation

---

## Configuration System (`pkg/config/yaml.go`)

### File Location
```
~/.config/gorkbot/gorkbot.yaml
```

### YAML Format
```yaml
# AI Model Configuration
model:
  use: "pkg.ai:AnthropicProvider"         # Factory key format
  api_key: $ANTHROPIC_API_KEY             # Env var expansion
  model: "claude-opus-4-20250514"
  max_tokens: 4096
  temperature: 0.7
  custom_fields:
    thinking_budget: 8000                 # Extended thinking token budget

# Sandbox Configuration
sandbox:
  enabled: true
  use: "pkg.sandbox:LocalSandbox"
  custom_fields:
    work_dir: "/tmp/gorkbot-work"

# Guardrails Configuration
guardrails:
  enabled: true
  use: "pkg.guardrails:Default"
```

### Environment Variable Expansion
All string values support `${VAR}` and `$VAR` syntax:
```yaml
api_key: ${ANTHROPIC_KEY}
model: $LLM_MODEL
```

### Hot Reload
Changes to `gorkbot.yaml` are detected automatically (2s poll interval) and applied without restarting.

---

## Skills System (`pkg/skills/`)

### File Format: `SKILL.md`
Located in `~/.config/gorkbot/skills/` or `.gorkbot/skills/`

```markdown
---
name: code-review
description: Perform thorough code review with security focus
aliases: [cr, review]
tools: [read_file, grep_content, bash]
model: claude-opus-4
tags: [security, code-review]
platforms: [cli, discord, slack]
---

Review {{target}} for:
1. Security vulnerabilities (OWASP top 10)
2. Logic errors and edge cases
3. Code style and maintainability
4. Performance issues

Provide structured feedback with:
- Severity levels (critical, high, medium, low)
- Exact line numbers
- Remediation suggestions
```

### Fields
| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | Yes | Canonical `/skillname` command |
| `description` | string | Yes | Single-line UI description |
| `aliases` | array | No | Alternative command names (`/alias` shortcuts) |
| `tools` | array | No | Allowed tool set; empty = all |
| `model` | string | No | Override model for this skill |
| `tags` | array | No | Metadata for organization (e.g., "security", "refactoring") |
| `platforms` | array | No | Target platforms (e.g., "cli", "discord") |

### Template Variables
| Variable | Expansion |
|----------|-----------|
| `{{target}}` | Argument passed to skill (e.g., `/skill filename`) |
| `{{args}}` | Same as `{{target}}` |

### Registry Access
```go
if reg := orch.SkillRegistry; reg != nil {
    def, ok := reg.Get("code-review")   // by name or alias
    allSkills := reg.List()              // sorted by name
    count := reg.Count()
    reg.Reload()                         // hot-reload on SKILL.md change
}
```

---

## Memory System

### Unified Memory Interface
Three complementary memory systems accessible via `orchestrator.UnifiedMem`:

#### 1. FactStore (SQLite-Backed)
**Purpose**: Persistent, deduplicated facts extracted by LLM from conversation.

**Schema**:
```sql
CREATE TABLE facts (
    id INTEGER PRIMARY KEY,
    session_id TEXT,
    content TEXT UNIQUE,        -- Global dedup
    source TEXT DEFAULT 'conversation',
    created_at DATETIME,
    last_seen_at DATETIME,
    occurrences INTEGER         -- Bump on repeat
);
```

**Usage**:
```go
// Queue a message for LLM fact extraction (debounced 5s)
fs.QueueForExtraction("User mentioned they work on Go projects")

// Query relevant facts (keyword-based)
facts, _ := fs.QueryRelevant("golang", 20)
formatted := fs.FormatForContext("golang tips", 800)  // Truncate to maxChars
```

#### 2. AgeMem (In-Memory with LTM Persistence)
**Purpose**: Time-ranked episodic facts with cross-session persistence.

**Priority Scoring**: `recency × priority × keyword_overlap`

#### 3. Engrams (Learned Tool Preferences)
**Purpose**: Persistent tool/behavior preferences written by `record_engram`.

#### 4. MEL (Multi-Stage Executor Learning)
**Purpose**: Bifurcation-generated heuristics (failure→correction patterns).

### Usage Pattern
```go
// All three systems queried, deduped, merged, truncated
ctx := orch.UnifiedMem.Query(prompt, maxTokens)
```

---

## Middleware Chain (`internal/engine/middleware.go`)

### Architecture
Ordered composition of 11 middlewares, each a pure function:
```go
type ToolRequest struct {
    Name   string
    Params map[string]interface{}
    TraceID string
}

type MiddlewareFunc func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult
```

### Middleware Order (First = Outermost)
1. **PlanModeMiddleware** — Block tool execution in plan mode
2. **RuleEngineMiddleware** — Fine-grained permission rules
3. **ToolCacheMiddleware** — TTL-based read-only tool memoization
4. **PreHookMiddleware** — Fire `pre_tool_use` event
5. **CheckpointMiddleware** — Save workspace state before mutating tools
6. **HITLMiddleware** — Human-in-the-loop approval gates
7. **SanitizerMiddleware** — Path/command sanitization
8. **GuardrailsMiddleware** — Safety guardrails evaluation
9. **SandboxMiddleware** — Sandbox routing (placeholder for now)
10. **PostHookMiddleware** — Fire `post_tool_use`/`post_tool_failure` events
11. **AgeMemMiddleware** — Store outputs in AgeMem
12. **TracingMiddleware** — Log to SENSE trace files

### Extending the Chain
```go
// In orchestrator.go BuildToolChain():
o.ToolChain = NewChain(
    // ... existing middlewares ...
    MyCustomMiddleware(customProvider),
)
```

### Example Custom Middleware
```go
func MySecurityAuditMiddleware(auditLog *AuditLog) MiddlewareFunc {
    return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
        auditLog.LogAttempt(req.Name, req.Params)

        result := next()  // Continue chain

        if result.Err != nil {
            auditLog.LogFailure(req.Name, result.Err)
        }
        return result
    }
}
```

---

## Provider Configuration

### Available Factory Keys
| Key | Provider | Notes |
|-----|----------|-------|
| `pkg.ai:AnthropicProvider` | Claude API | ThinkingBudget field supported |
| `pkg.ai:GoogleProvider` | Gemini API | Extended thinking via CustomFields |
| `pkg.ai:XAIProvider` | xAI Grok | Default primary provider |
| `pkg.sandbox:LocalSandbox` | Local filesystem | Work directory with path confinement |

### Custom Fields
```yaml
custom_fields:
  thinking_budget: 8000          # int, Anthropic only
  stream_enabled: true           # bool, provider-specific
  max_parallel_tools: 4          # int, affects dispatcher
```

---

## Sandbox System

### Local Sandbox (`pkg/provider/sandbox_local.go`)

**Purpose**: Confine file operations and shell commands to a work directory.

**Configuration**:
```yaml
sandbox:
  enabled: true
  use: "pkg.sandbox:LocalSandbox"
  custom_fields:
    work_dir: "/tmp/gorkbot-work"    # Default: ~/.gorkbot/work
    allowed_prefixes:               # Additional allowed paths
      - "/home/user/projects"
      - "/tmp"
```

**Confinement Rules**:
1. All file paths must be within `work_dir` (symlinks resolved)
2. Additionally allowed prefixes (temp, home, system dirs) are permitted
3. Out-of-bounds paths are rejected before execution

**API**:
```go
sp := orch.SandboxProv
output, err := sp.RunCommand(ctx, "ls -la")
data, err := sp.ReadFile(ctx, "output.txt")
err := sp.WriteFile(ctx, "config.json", []byte{...})
err := sp.Close()
```

### Docker Sandbox (Deferred)
Future: Task #9B will implement container-based sandbox with image pull, volume mount, cleanup.

---

## Migration Guide

### For Existing Gorkbot Users
✅ **No Breaking Changes** — Phase 2A is fully backward compatible.

1. **YAML config is optional** — Existing environment variables still work
2. **Skills system** — New SKILL.md format complements existing skill loading
3. **Memory** — New FactStore supplements (doesn't replace) AgeMem
4. **Middleware** — New chain co-exists with legacy ExecuteTool logic
5. **Providers** — New factory system extends (doesn't replace) old selection

### Gradual Adoption
```go
// Start simple
orch := engine.NewOrchestrator(primary, consultant)
orch.InitEnhancements(configDir, cwd)  // Initializes all systems

// Systems are opt-in (nil = disabled)
// - No SKILL.md? SkillLoader is nil → no registry
// - No gorkbot.yaml? Configs default to zero values
// - No FactStore? Just creates empty DB (no extraction happens)
```

---

## Configuration Reference

### System Prompt Injection
Phase 2A injects in this order:
1. **Model Override** (if `model.use` specified in YAML)
2. **Sandbox Context** (if enabled, displays work_dir + allowed prefixes)
3. **Skills Index** (all available /skillname commands + descriptions)
4. **Memory Context** (GoalLedger + FactStore + AgeMem + Engrams + MEL)
5. **Project Instructions** (from GORKBOT.md hierarchy, hot-reloaded)

### Performance Tuning
```yaml
custom_fields:
  # AgeMem: TTL for hot/warm memory (default: 24h)
  agemem_ttl_hours: 24

  # FactStore: debounce extraction (default: 5s)
  factstore_debounce_ms: 5000

  # Middleware: disable caching for sensitive tools
  tool_cache_ttl_minutes: 30
```

---

## Testing

### Unit Tests
```bash
go test ./pkg/config/...          # YAML loading, env expansion
go test ./pkg/skills/...          # Registry, hot-reload
go test ./pkg/memory/...          # FactStore, unified interface
go test ./internal/engine/...     # Middleware chain, orchestrator
```

### Integration Tests
```bash
# Full orchestrator initialization with all systems
go test ./internal/engine -run Integration

# Hot-reload scenarios (YAML + SKILL.md)
go test ./pkg/config -run Watcher
```

---

## Troubleshooting

### "Provider not found"
**Cause**: YAML `use` field references unknown factory key.
**Solution**: Check available keys in `pkg/provider/registry.go` and use exact format (e.g., `pkg.ai:AnthropicProvider`).

### "Path not allowed by sandbox"
**Cause**: Tool tried to access file outside work_dir.
**Solution**: Add path to `allowed_prefixes` in gorkbot.yaml, or use relative paths within work_dir.

### "Skill not found"
**Cause**: SKILL.md not in `~/.config/gorkbot/skills/` or `.gorkbot/skills/`.
**Solution**: Verify file location, reload with `/skills reload`, or check SkillRegistry.List().

### "Memory context injection fails"
**Cause**: FactStore/AgeMem DB corrupted or locked.
**Solution**: Delete DB file (e.g., `~/.gorkbot/data/facts.db`), orchestrator will recreate it.

---

## API Reference

### Key Types
```go
// Configuration
type GorkbotConfig struct {
    Model      ModelConfig
    Sandbox    SandboxConfig
    Guardrails GuardrailsConfig
}

// Skills
type SkillRegistry struct { ... }
func (r *SkillRegistry) Get(nameOrAlias string) (*Definition, bool)
func (r *SkillRegistry) List() []*Definition
func (r *SkillRegistry) Reload()

// Memory
type FactStore struct { ... }
func (fs *FactStore) QueueForExtraction(content string)
func (fs *FactStore) QueryRelevant(query string, limit int) ([]Fact, error)
func (fs *FactStore) FormatForContext(query string, maxChars int) string

// Middleware
type Chain struct { ... }
func (c *Chain) Execute(ctx context.Context, req ToolRequest, final func(...) ToolResult) ToolResult

// Orchestrator
type Orchestrator struct {
    SkillRegistry *skills.SkillRegistry
    SandboxProv   provider.SandboxProvider
    GuardrailsProv provider.GuardrailsProvider
    ToolChain     *Chain
}
func (o *Orchestrator) BuildToolChain()
func (o *Orchestrator) ApplyConfig(cfg *config.GorkbotConfig)
```

---

## Version History

| Date | Version | Changes |
|------|---------|---------|
| 2026-03-23 | 2.0.0 | Phase 2A complete: YAML config, Skills registry, FactStore, Middleware chain, Orchestrator integration |

