package cci

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Specialist represents a Tier 2 domain persona.
type Specialist struct {
	Domain    string
	Content   string
	LoadedAt  time.Time
}

// SpecialistManager loads and manages Tier 2 specialist personas.
// Specialists live in configDir/cci/specialists/<domain>.md
//
// Specialists are:
//   - Loaded on-demand when the Tier 1 trigger table routes a task to them.
//   - Rich with embedded domain knowledge (formulas, patterns, constraints).
//   - Pre-loaded with symptom-cause-fix tables for known failure modes.
//   - Auto-synthesized by MEL when a domain causes repeated bifurcation loops.
type SpecialistManager struct {
	baseDir string
}

// NewSpecialistManager creates a SpecialistManager backed by configDir/cci/specialists/.
func NewSpecialistManager(configDir string) *SpecialistManager {
	dir := filepath.Join(configDir, "cci", "specialists")
	_ = os.MkdirAll(dir, 0700)
	sm := &SpecialistManager{baseDir: dir}
	sm.seedDefaults()
	return sm
}

// Load returns the specialist persona for the given domain, or nil if not found.
func (sm *SpecialistManager) Load(domain string) *Specialist {
	domain = sanitizeDomain(domain)
	path := filepath.Join(sm.baseDir, domain+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return &Specialist{
		Domain:   domain,
		Content:  string(data),
		LoadedAt: time.Now(),
	}
}

// List returns all available specialist domain names.
func (sm *SpecialistManager) List() []string {
	entries, err := os.ReadDir(sm.baseDir)
	if err != nil {
		return nil
	}
	var domains []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			domains = append(domains, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Strings(domains)
	return domains
}

// Synthesize writes a new specialist file from generated content.
// This is called by the MEL bifurcation analyzer when it detects a new failure domain.
func (sm *SpecialistManager) Synthesize(domain, content string) error {
	domain = sanitizeDomain(domain)
	path := filepath.Join(sm.baseDir, domain+".md")
	return os.WriteFile(path, []byte(content), 0600)
}

// Update replaces an existing specialist file content.
func (sm *SpecialistManager) Update(domain, content string) error {
	return sm.Synthesize(domain, content) // same operation
}

// FormatBlock returns the Tier 2 injection block for a specialist.
func FormatSpecialistBlock(s *Specialist) string {
	return fmt.Sprintf("<!-- CCI TIER 2 SPECIALIST: %s (loaded %s) -->\n\n%s",
		s.Domain, s.LoadedAt.Format("15:04:05"), s.Content)
}

func sanitizeDomain(domain string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(domain) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else if r == ' ' || r == '/' {
			sb.WriteRune('-')
		}
	}
	return sb.String()
}

// seedDefaults writes the built-in specialist personas if they don't exist.
func (sm *SpecialistManager) seedDefaults() {
	defaults := map[string]string{
		"orchestrator": specialistOrchestrator,
		"tui":          specialistTUI,
		"tool-system":  specialistTools,
		"ai-providers": specialistAI,
		"arc-mel":      specialistARCMEL,
		"cci":          specialistCCI,
	}
	for domain, content := range defaults {
		path := filepath.Join(sm.baseDir, domain+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			_ = os.WriteFile(path, []byte(content), 0600)
		}
	}
}

// ── Built-in specialist personas ────────────────────────────────────────────

const specialistOrchestrator = `# Specialist: Orchestrator Engineer

## Domain Knowledge
The Orchestrator (internal/engine/orchestrator.go) is the central coordination hub.
It holds ~30 subsystem references; adding new fields requires adding init in NewOrchestrator() or InitEnhancements().

### Critical Invariants
- ConversationHistory is NOT thread-safe — only touch from the streaming goroutine.
- All subsystem init that needs configDir must happen in InitEnhancements(), not NewOrchestrator().
- cancelMu guards cancelFunc — never access cancelFunc without this lock.
- ExecuteTaskWithStreaming is the primary execution path for TUI sessions.

### Known Failure Modes
| Symptom | Cause | Fix |
|---------|-------|-----|
| "nil pointer dereference" in streaming | New Orchestrator field not initialized | Add init to NewOrchestrator() |
| System prompt injected on every turn | Count() check wrong | Wrap in if History.Count()==0 |
| Tool result never reaches AI | ConversationHistory.AddUserMessage used instead of AddToolResultMessage | Use correct method |
| Import cycle build error | engine→tui import | Use OrchestratorAdapter function refs |

### Patterns
New integration checklist:
1. Add field to Orchestrator struct with comment
2. Init in NewOrchestrator() if no configDir needed, else InitEnhancements()
3. Add nil guard in all call sites
4. Expose via OrchestratorAdapter if TUI needs it
`

const specialistTUI = `# Specialist: TUI Architect

## Domain Knowledge
Follows the Elm architecture (Model-View-Update). All state lives in model.go.
Bubble Tea message passing: never block in Update(); send commands/IOs as tea.Cmd.

### Critical Invariants
- NEVER modify the tea.MouseMsg block in update.go:67-91 — breaks Android touch-scroll.
- NEVER modify handleStreamComplete — complex state machine for streaming token receipt.
- viewport.SetContent() is expensive; batch updates, don't call per-token.
- All custom messages must be typed structs, not raw strings.

### Message Pipeline
Token arrives → streamCallback → tea.Cmd → TokenMsg → Update() → append to m.messages → SetContent

### Known Failure Modes
| Symptom | Cause | Fix |
|---------|-------|-----|
| Messages disappear after scroll | Viewport not resized on WindowSizeMsg | Recompute height in WindowSizeMsg handler |
| Keyboard stops working (Android) | Input not focused after modal closed | Call m.input.Focus() in modal close handler |
| ANSI codes in input field | Missing StripANSI on paste | Call sense.StripANSI before storing |
| New overlay not shown | viewFn switch not updated | Add case to renderView() switch |
`

const specialistTools = `# Specialist: Tool System Engineer

## Domain Knowledge
Tools implement pkg/tools/Tool interface: Name, Description, Category, Parameters, Execute.
All tools must be registered in RegisterDefaultTools() in pkg/tools/registry.go.

### Critical Invariants
- pkg/tools/custom/ is a fragment dir — never compile with go build ./...
- shellescape() is package-level in bash.go — use it for all shell params
- Execute() receives ctx — always respect cancellation
- ToolResult.Output must be plain text — no ANSI codes

### Permission Levels
- PermissionAlways: permanent approval → persisted to tool_permissions.json
- PermissionSession: current session only
- PermissionOnce: ask each time (default for destructive ops)
- PermissionNever: permanently blocked

### Known Failure Modes
| Symptom | Cause | Fix |
|---------|-------|-----|
| Tool not found by AI | Not registered in RegisterDefaultTools() | Add registration |
| Command injection risk | Params not escaped | Use shellescape() |
| Tool hangs | No timeout on exec.Command | Add exec.CommandContext with timeout |
| Category disabled but tool runs | Category check not in Execute() path | Category guard is in registry Execute() not tool |
`

const specialistAI = `# Specialist: AI Provider Engineer

## Domain Knowledge
All providers implement pkg/ai/AIProvider interface.
GrokProvider uses xAI OpenAI-compat API; GeminiProvider uses Google Generative AI.
Native function calling only available on GrokProvider (NativeToolCaller interface).

### Critical Invariants
- role:"tool" messages in history MUST have tool_call_id — xAI rejects otherwise
- Empty-content assistant messages with tool_calls must be included — don't strip
- StreamWithHistory writes tokens to io.Writer — never buffer entire response
- Always clone ConversationMessage before modifying (history is shared)

### Known Failure Modes
| Symptom | Cause | Fix |
|---------|-------|-----|
| 400 from xAI streaming | role:"tool" without tool_call_id | Use AddToolResultMessage() not AddUserMessage() |
| Context lost after rewind | History truncation removes tool messages | TruncateToTokenLimit uses continue not break |
| Gemini returning empty | OAuth scope missing | Use API key auth, not OAuth |
| Streaming cuts off | Writer not flushed | Provider must call Flush() after each chunk |
`

const specialistARCMEL = `# Specialist: ARC Router & MEL Learning Engineer

## Domain Knowledge
ARC Router (internal/arc/): classifies prompts into 6 WorkflowTypes, computes ResourceBudget.
MEL (internal/mel/): Jaccard-similarity vector store of learned heuristics (max 500, evict lowest-conf).

### ARC Integration Points (internal/engine/intelligence.go)
1. Route(prompt) before AI call → budget.MaxToolCalls overrides maxTurns
2. MEL heuristics injected into system prompt (first turn only)
3. ObserveFailed/ObserveSuccess after every tool execution

### Known Failure Modes
| Symptom | Cause | Fix |
|---------|-------|-----|
| Heuristics not appearing in prompt | Intelligence not initialized | Call InitIntelligence() after InitEnhancements() |
| All prompts route to Conversational | Low keyword scores on technical prompts | Add domain keywords to classifier.go |
| MEL store grows unbounded | Eviction threshold not set | Max 500 enforced in VectorStore.Add() |
`

const specialistCCI = `# Specialist: CCI System Engineer

## Domain Knowledge
CCI (pkg/cci/) implements the three-tier memory architecture.
Tier 1 (Hot): always-injected, built by HotMemory.BuildBlock()
Tier 2 (Specialist): loaded by SpecialistManager.Load(domain)
Tier 3 (Cold): queried via ColdMemoryStore.GetSubsystem(name)
DriftDetector: runs at session start via Workspace git checkpoint hook
GapHandler: null Tier 3 query → ModeManager.SetMode(PLAN)

### Integration Points
- CCILayer.Init() called from Orchestrator.InitEnhancements()
- CCILayer.BuildSystemContext(prompt) called in streaming.go before first system message
- CCILayer.RunDriftCheck(cwd) called right after workspace checkpoint
- Gap detection: if ColdStore.GetSubsystem() returns "" → emit GapEvent → shift mode

### Known Failure Modes
| Symptom | Cause | Fix |
|---------|-------|-----|
| Hot memory block too large | defaultHotConventions too verbose | Keep Tier 1 under 2000 tokens |
| Specialist not loaded | Domain mismatch in trigger table | Check sanitizeDomain() output vs file name |
| Drift detector false positives | go.sum changes flagged | Filter out non-source-code file patterns |
| Gap mode not resetting | ModeManager.SetMode() not called after doc created | CCILayer.NotifyGapResolved() resets mode |
`
