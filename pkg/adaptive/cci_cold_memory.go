package adaptive

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// subsystemFileMap maps subsystem names to the source-code paths they document.
// Used by the DriftDetector to cross-reference git changes against spec files.
var subsystemFileMap = map[string][]string{
	"orchestrator": {"internal/engine/orchestrator.go", "internal/engine/streaming.go", "internal/engine/brain.go"},
	"tui":          {"internal/tui/model.go", "internal/tui/update.go", "internal/tui/view.go", "internal/tui/style.go"},
	"tool-system":  {"pkg/tools/registry.go", "pkg/tools/tool.go", "pkg/tools/bash.go"},
	"ai-providers": {"pkg/ai/grok.go", "pkg/ai/gemini.go", "pkg/ai/anthropic.go"},
	"arc-mel":      {"internal/arc/router.go", "internal/arc/classifier.go", "internal/mel/store.go"},
	"mcp":          {"pkg/mcp/manager.go", "pkg/mcp/client.go"},
	"sense":        {"pkg/sense/compression.go", "pkg/sense/lie.go", "pkg/sense/agemem.go"},
	"cci":          {"pkg/cci/layer.go", "pkg/cci/hot_memory.go", "pkg/cci/specialist.go"},
	"memory":       {"pkg/memory/unified.go", "pkg/memory/goals.go"},
	"subagents":    {"pkg/subagents/delegate.go", "pkg/subagents/worktree.go"},
	"session":      {"pkg/session/checkpoint.go", "pkg/session/workspace.go"},
	"security":     {"pkg/tools/security_findings.go"},
	"commands":     {"pkg/commands/registry.go"},
	"providers":    {"pkg/providers/manager.go", "pkg/providers/keystore.go"},
}

// ColdMemoryStore is the Tier 3 on-demand subsystem knowledge base.
// Documents live in configDir/cci/docs/<subsystem>.md and are authored
// strictly for AI consumption (file paths, exact params, do/don't tables).
type ColdMemoryStore struct {
	docsDir string
}

// NewColdMemoryStore creates a ColdMemoryStore backed by configDir/cci/docs/.
func NewColdMemoryStore(configDir string) *ColdMemoryStore {
	docsDir := filepath.Join(configDir, "cci", "docs")
	_ = os.MkdirAll(docsDir, 0700)
	cs := &ColdMemoryStore{docsDir: docsDir}
	cs.seedDefaults()
	return cs
}

// GetSubsystem returns the Tier 3 spec for the given subsystem name.
// Returns empty string if the spec does not exist (gap event should follow).
func (cs *ColdMemoryStore) GetSubsystem(name string) string {
	name = sanitizeDomain(name)
	path := filepath.Join(cs.docsDir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// ListSubsystems returns all documented subsystem names, sorted alphabetically.
func (cs *ColdMemoryStore) ListSubsystems() []string {
	entries, err := os.ReadDir(cs.docsDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Strings(names)
	return names
}

// UpdateSubsystem writes or overwrites the Tier 3 spec for a subsystem.
// This implements "living documentation" — specs are updated in the same
// session as the code change.
func (cs *ColdMemoryStore) UpdateSubsystem(name, content string) error {
	name = sanitizeDomain(name)
	path := filepath.Join(cs.docsDir, name+".md")
	return os.WriteFile(path, []byte(content), 0600)
}

// SubsystemToFileMap returns a copy of the source-code path mapping.
// Used by DriftDetector to cross-reference git changes.
func (cs *ColdMemoryStore) SubsystemToFileMap() map[string][]string {
	result := make(map[string][]string, len(subsystemFileMap))
	for k, v := range subsystemFileMap {
		paths := make([]string, len(v))
		copy(paths, v)
		result[k] = paths
	}
	return result
}

// FormatSubsystemDoc wraps the raw doc content in a Tier 3 context block.
func FormatSubsystemDoc(name, content string) string {
	return fmt.Sprintf("<!-- CCI TIER 3 SPEC: %s (retrieved %s) -->\n\n%s",
		name, time.Now().Format("15:04:05"), content)
}

// SuggestSpecialist returns the most relevant Tier 2 specialist domain for a
// given task description using simple keyword overlap scoring.
func (cs *ColdMemoryStore) SuggestSpecialist(task string) string {
	task = strings.ToLower(task)
	scores := map[string]int{
		"orchestrator": score(task, []string{"orchestrat", "streaming", "brain", "turn loop", "system message", "consultat"}),
		"tui":          score(task, []string{"tui", "ui", "terminal", "bubble tea", "viewport", "render", "view", "input", "keyboard", "display"}),
		"tool-system":  score(task, []string{"tool", "registry", "permission", "execute", "bash", "file", "shell"}),
		"ai-providers": score(task, []string{"grok", "gemini", "anthropic", "provider", "model", "streaming", "api", "token"}),
		"arc-mel":      score(task, []string{"arc", "mel", "route", "classif", "heuristic", "budget", "workflow"}),
		"mcp":          score(task, []string{"mcp", "server", "stdio", "json-rpc", "protocol"}),
		"cci":          score(task, []string{"cci", "codified", "context", "tier", "hot memory", "specialist", "cold memory", "drift"}),
		"memory":       score(task, []string{"memory", "agemem", "engram", "goal", "unified", "ltm", "stm"}),
		"subagents":    score(task, []string{"subagent", "spawn", "worktree", "delegate", "background", "agent"}),
	}

	best := ""
	bestScore := 0
	for domain, s := range scores {
		if s > bestScore {
			bestScore = s
			best = domain
		}
	}
	if bestScore == 0 {
		return "orchestrator" // default to most central domain
	}
	return best
}

func score(task string, keywords []string) int {
	n := 0
	for _, kw := range keywords {
		if strings.Contains(task, kw) {
			n++
		}
	}
	return n
}

// seedDefaults writes initial Tier 3 specs if they don't exist.
func (cs *ColdMemoryStore) seedDefaults() {
	defaults := defaultColdDocs()
	for name, content := range defaults {
		path := filepath.Join(cs.docsDir, name+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			_ = os.WriteFile(path, []byte(content), 0600)
		}
	}
}

func defaultColdDocs() map[string]string {
	return map[string]string{
		"orchestrator": coldDocOrchestrator,
		"tui":          coldDocTUI,
		"tool-system":  coldDocTools,
		"arc-mel":      coldDocARCMEL,
		"cci":          coldDocCCI,
	}
}

// ── Built-in Tier 3 cold docs ────────────────────────────────────────────────

const coldDocOrchestrator = `# Tier 3 Spec: Orchestrator (internal/engine/)

## File Index
- orchestrator.go — Orchestrator struct (30+ fields), NewOrchestrator(), InitEnhancements(), InitIntelligence()
- streaming.go — ExecuteTaskWithStreaming() — primary execution path
- brain.go — GetDynamicBrainContext() — reads ~/.gorkbot/brain/
- intelligence.go — IntelligenceLayer: ARC+MEL integration
- plan_mode.go — ModeManager (Normal/Plan/AutoEdit)
- context_manager.go — ContextManager: token tracking, 90% compaction trigger
- sense_hitl.go — HITLGuard, HITLCallback, GateToolExecution()
- cci_integration.go — CCILayer hooks

## Struct Fields (Orchestrator)
Primary, Consultant ai.AIProvider — AI providers
Registry *tools.Registry — tool execution
ConversationHistory *ai.ConversationHistory — message history
LIE, Stabilizer, AgeMem, Engrams, Compressor — SENSE components
ModeManager *ModeManager — execution mode (NORMAL/PLAN/AUTOEDIT)
Intelligence *IntelligenceLayer — ARC router + MEL store
CCI *CCILayer — Codified Context Infrastructure
Workspace *session.WorkspaceManager — git checkpoints
BackgroundAgents *BackgroundAgentManager
GoalLedger *memory.GoalLedger
UnifiedMem *memory.UnifiedMemory

## DO
- Add new fields to Orchestrator with a comment block
- Initialize in NewOrchestrator() if no configDir dependency
- Initialize in InitEnhancements() if configDir-dependent
- Use nil guards at every call site

## DON'T
- Don't touch ConversationHistory from background goroutines (not thread-safe)
- Don't import tui from engine (import cycle)
- Don't use cancelFunc directly — always go through Interrupt()
`

const coldDocTUI = `# Tier 3 Spec: TUI (internal/tui/)

## File Index
- model.go — Model struct, Init(), NewModel()
- update.go — Update(msg) — Elm update function; CRITICAL: do not modify MouseMsg block:67-91
- view.go — View() — rendering; tab routing; overlay dispatch
- style.go — Lip Gloss styles, theme constants
- messages.go — typed message structs (TokenMsg, ErrorMsg, etc.)
- keys.go — keyboard bindings
- statusbar.go — context%, cost, mode, git branch display
- settings_overlay.go — /settings 3-tab modal
- model_select_view.go — dual-pane model selection
- analytics_view.go — tool analytics dashboard

## Elm Architecture Flow
1. User input → tea.KeyMsg → Update()
2. AI starts → send GenerateCmd → returns StreamStartMsg
3. Tokens → streamCallback → tea.Batch(tea.Cmd) → TokenMsg → append m.messages
4. Done → StreamCompleteMsg → handleStreamComplete()

## DO
- All new state: add field to Model, init in NewModel()
- New message type: add to messages.go
- New overlay: add case to renderView() and viewFn switch
- New keyboard binding: add to keys.go, handle in Update()

## DON'T
- Don't modify MouseMsg block in update.go
- Don't call viewport.SetContent() per-token (expensive)
- Don't block in Update() — return a tea.Cmd for async work
`

const coldDocTools = `# Tier 3 Spec: Tool System (pkg/tools/)

## File Index
- tool.go — Tool interface, ToolResult, ToolRequest, ToolCategory
- registry.go — Registry, RegisterDefaultTools(), Execute()
- bash.go — BashTool, shellescape() (package-level)
- file.go — File tools (read/write/list/search/delete)
- permissions.go — PermissionManager, PermissionLevel constants
- cache.go — ToolCache (TTL memoization, mutation invalidation)
- dispatcher.go — Dispatcher (parallel execution, max 8 workers)
- rules.go — RuleEngine (fine-grained glob-pattern rules)
- error_recovery.go — ErrorCode, ClassifyError, EnrichResult
- cci_tools.go — mcp_context_* retrieval tools

## Tool Interface
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

## Registration Pattern
func init() {
  // Or register directly in RegisterDefaultTools()
}
// In RegisterDefaultTools():
reg.Register(NewMyTool())

## DO
- Use shellescape() for ALL shell parameters
- Set exec.CommandContext with reasonable timeout
- Return ToolResult{Success: false, Error: "..."} on failure (not error)
- Use PermissionOnce for destructive ops, PermissionSession for read-only

## DON'T
- Don't put tools in pkg/tools/custom/ for permanent integration (fragment dir)
- Don't return ANSI codes in ToolResult.Output
- Don't store global mutable state in tool structs (concurrent execution)
`

const coldDocARCMEL = `# Tier 3 Spec: ARC Router + MEL Learning (internal/arc/, internal/mel/)

## ARC Files
- classifier.go — QueryClassifier, WorkflowType enum, keyword scoring table
- router.go — ARCRouter.Route(prompt) → RouteDecision{Classification, Budget}
- budget.go — ComputeBudget(platform, workflow) → ResourceBudget
- trigger_table.go — TriggerTable: file path patterns → Tier 2 specialist domains

## MEL Files
- store.go — VectorStore: JSON persistence, Jaccard similarity, max 500 entries
- analyzer.go — BifurcationAnalyzer: ObserveFailed/ObserveSuccess → auto-heuristic synthesis
- heuristic.go — Heuristic{Context, Constraint, ErrorToAvoid, Confidence, UseCount}

## Integration (internal/engine/intelligence.go)
type IntelligenceLayer struct {
  Router   *ARCRouter
  Store    *VectorStore
  Analyzer *BifurcationAnalyzer
  Evaluator *ReframedEvaluator
}
Call sites in orchestrator:
1. Before AI call: decision = il.Router.Route(prompt) → sets maxTurns
2. System prompt injection: il.Store.Query(prompt, 5) → prepend to first system message
3. After tool: il.Analyzer.ObserveFailed/Success(toolName, params)

## Trigger Table Integration
- TriggerTable.Match(filePath) returns specialist domain
- Called from CCILayer.LoadSpecialistForPrompt() which scans referenced files in prompt

## DO
- Add new workflow keywords with appropriate weights to classifier.go
- Store heuristics via Analyzer.ObserveFailed — they auto-deduplicate at 70% similarity

## DON'T
- Don't manually edit vector_store.json — use VectorStore API
- Don't add security keywords with weight < 3.0 (too easily false-positive)
`

const coldDocCCI = `# Tier 3 Spec: CCI System (pkg/cci/)

## File Index
- tier.go — TierType enum, CCIDoc, DriftWarning, GapEvent
- hot_memory.go — HotMemory: Tier 1, always-injected conventions + pointers
- specialist.go — SpecialistManager: Tier 2 on-demand personas
- cold_memory.go — ColdMemoryStore: Tier 3 subsystem specs
- drift_detector.go — DriftDetector: Truth Sentry cross-referencing git changes
- layer.go — CCILayer: top-level API used by Orchestrator

## Directory Structure
~/.config/gorkbot/cci/
  hot/
    CONVENTIONS.md      ← Tier 1 universal conventions (editable)
    SUBSYSTEM_POINTERS.md ← Tier 1 subsystem index (editable)
  specialists/
    orchestrator.md     ← Tier 2 specialist personas
    tui.md
    tool-system.md
    ...
  docs/
    orchestrator.md     ← Tier 3 subsystem specs
    tui.md
    arc-mel.md
    ...

## CCILayer API
layer.BuildSystemContext(prompt) string  — Tier 1 + optional Tier 2 injection
layer.RunDriftCheck(cwd) []DriftWarning  — Truth Sentry pre-flight
layer.HandleGap(subsystem, modeManager)  — null query → PLAN mode
layer.ColdStore.GetSubsystem(name) string
layer.ColdStore.UpdateSubsystem(name, content) error
layer.Specialists.Synthesize(domain, content) error

## Lifecycle (per session)
1. InitEnhancements() → CCILayer.Init(configDir, cwd, logger)
2. First system message → CCILayer.BuildSystemContext(prompt) prepended
3. Workspace checkpoint → CCILayer.RunDriftCheck(cwd) — inject warnings if any
4. Tool mcp_context_get_subsystem query → if "" → CCILayer.HandleGap()
5. AI updates doc → ColdStore.UpdateSubsystem() — living documentation

## DO
- Keep Tier 1 hot block under 2000 tokens
- Update Tier 3 docs in same session as code changes
- Synthesize new Tier 2 specialists when a domain causes 3+ bifurcation loops

## DON'T
- Don't embed large code blocks in Tier 1 — link to Tier 3 instead
- Don't gate EVERY subsystem — only those that repeatedly cause failure
- Don't let stale Tier 3 docs persist — the drift detector will flag them
`
