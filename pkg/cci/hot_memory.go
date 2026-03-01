package cci

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultHotConventions is the immutable universal conventions block injected
// into every session. It defines non-negotiable code quality standards.
const defaultHotConventions = `## CCI Tier 1: Universal Conventions (IMMUTABLE — do not contradict)

### Build
- Binary: bin/gorkbot | Build: go build -o bin/gorkbot ./cmd/gorkbot/
- Run: ./gorkbot.sh (loads .env) | Module: github.com/velariumai/gorkbot
- Tests: go test ./... (exclude pkg/tools/custom/ — fragment dir)

### Code Standards
- Package layout: cmd/ internal/ pkg/ — standard Go layout
- New AI provider → implement pkg/ai/AIProvider interface
- New tool → add to pkg/tools/*.go, register in RegisterDefaultTools()
- New slash command → register in pkg/commands/registry.go
- No import cycle: engine→tui forbidden; use OrchestratorAdapter

### Critical Constraints
- pkg/tools/custom/ is a code-fragment dir — never run go build ./... from root targeting it
- shellescape() is package-level in bash.go — available to all pkg/tools/ files
- Touch-scroll on Android: never modify tea.MouseMsg block in update.go:67-91
- All destructive actions (delete, push, overwrite) require HITL approval

### Orchestration Trigger Protocol
When a task targets a known subsystem, the ARC Trigger Table MUST be consulted.
The trigger table maps file paths → Tier 2 specialist domain.
If no specialist exists for a domain, query mcp_context_get_subsystem before modifying.
If mcp_context_get_subsystem returns empty → PLAN mode is mandatory before coding.

### SENSE LIE Enforcement
- Modifying a core system without consulting the trigger table triggers ActionAdvise from the Stabilizer.
- When exploring unfamiliar code, ALWAYS use mcp_context_get_subsystem first.
- Confidence < 60% → ask user before proceeding.
- Repeated identical tool call → STOP and query query_routing_stats for loop detection.
`

// defaultSubsystemPointers is the high-level architecture overview with links to
// Tier 3 specs. Updated as architecture evolves.
const defaultSubsystemPointers = `## CCI Tier 1: Subsystem Pointers (links to Tier 3 docs)

| Subsystem       | Key Files                              | Tier 3 Doc              |
|-----------------|----------------------------------------|-------------------------|
| orchestrator    | internal/engine/orchestrator.go        | orchestrator.md         |
| tui             | internal/tui/model.go, update.go       | tui.md                  |
| tool-system     | pkg/tools/registry.go, tool.go         | tool_system.md          |
| ai-providers    | pkg/ai/grok.go, gemini.go              | ai_providers.md         |
| arc-router      | internal/arc/router.go, classifier.go  | arc_mel.md              |
| mel-learning    | internal/mel/store.go, analyzer.go     | arc_mel.md              |
| mcp-integration | pkg/mcp/manager.go, client.go          | mcp.md                  |
| sense           | pkg/sense/compression.go, lie.go       | sense.md                |
| cci             | pkg/cci/layer.go                       | cci.md                  |
| memory          | pkg/memory/unified.go, goals.go        | memory.md               |
| subagents       | pkg/subagents/delegate.go, worktree.go | subagents.md            |
| session         | pkg/session/checkpoint.go, workspace.go| session.md              |
| security        | pkg/tools/security*.go                 | security.md             |

Query a doc: mcp_context_get_subsystem {"name": "<subsystem>"}
List all:    mcp_context_list_subsystems {}
Get specialist suggestion: mcp_context_suggest_specialist {"task": "<description>"}
`

// HotMemory manages the Tier 1 "always-loaded" memory block.
// It composes: universal conventions + trigger table summary + subsystem pointers.
type HotMemory struct {
	configDir   string
	conventionsPath string
	pointerPath     string
}

// NewHotMemory creates a HotMemory backed by configDir/cci/hot/.
// Default files are written if they do not yet exist.
func NewHotMemory(configDir string) *HotMemory {
	hotDir := filepath.Join(configDir, "cci", "hot")
	_ = os.MkdirAll(hotDir, 0700)

	hm := &HotMemory{
		configDir:       configDir,
		conventionsPath: filepath.Join(hotDir, "CONVENTIONS.md"),
		pointerPath:     filepath.Join(hotDir, "SUBSYSTEM_POINTERS.md"),
	}
	hm.ensureDefaults()
	return hm
}

func (hm *HotMemory) ensureDefaults() {
	if _, err := os.Stat(hm.conventionsPath); os.IsNotExist(err) {
		_ = os.WriteFile(hm.conventionsPath, []byte(defaultHotConventions), 0600)
	}
	if _, err := os.Stat(hm.pointerPath); os.IsNotExist(err) {
		_ = os.WriteFile(hm.pointerPath, []byte(defaultSubsystemPointers), 0600)
	}
}

// BuildBlock constructs the full Tier 1 injection string.
// This is prepended to the system prompt on every session.
func (hm *HotMemory) BuildBlock(triggerTableSummary string) string {
	var sb strings.Builder

	sb.WriteString("<!-- CCI HOT MEMORY — ALWAYS LOADED —\n")
	sb.WriteString(fmt.Sprintf("   Generated: %s\n-->\n\n", time.Now().Format(time.RFC3339)))

	// Universal conventions (from file; falls back to default)
	conventions := hm.readFile(hm.conventionsPath, defaultHotConventions)
	sb.WriteString(conventions)
	sb.WriteString("\n\n")

	// Trigger table summary (provided by ARC)
	if triggerTableSummary != "" {
		sb.WriteString("## CCI Tier 1: Orchestration Trigger Table\n\n")
		sb.WriteString(triggerTableSummary)
		sb.WriteString("\n\n")
	}

	// Subsystem pointers
	pointers := hm.readFile(hm.pointerPath, defaultSubsystemPointers)
	sb.WriteString(pointers)

	return sb.String()
}

// UpdateConventions overwrites the CONVENTIONS.md file with new content.
// This allows the AI to update hot memory at the user's direction.
func (hm *HotMemory) UpdateConventions(content string) error {
	return os.WriteFile(hm.conventionsPath, []byte(content), 0600)
}

func (hm *HotMemory) readFile(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return fallback
	}
	return string(data)
}
