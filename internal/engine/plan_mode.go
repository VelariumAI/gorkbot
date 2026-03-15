package engine

import (
	"fmt"
	"strings"
	"sync"
)

// ExecutePlanMode is the entry point for ARC Router's Complex/Plan mode execution.
// It securely executes the provided planFunc while ensuring interim reasoning buffers
// are evaluated and token usage strictly incremented without polluting context.
func ExecutePlanMode(orch *Orchestrator, planningBuf *strings.Builder, planFunc func() error) (err error) {
	// Edge-Case Anticipation: handle nil dependencies gracefully
	if orch == nil {
		return fmt.Errorf("ExecutePlanMode: orchestrator is nil")
	}

	// State-Safe Execution: defer block guarantees token accrual and buffer reset
	// whether the function succeeds, fails via tool error/network, or panics.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("planning mode panicked: %v", r)
		}

		if planningBuf == nil || orch.ContextMgr == nil {
			return
		}

		// Explicit Token Tracking: strictly increment usage
		tokensUsed := orch.ContextMgr.TrackTokens(planningBuf)

		// Summarized History Commit: append system note, avoid context contamination
		if orch.ConversationHistory != nil {
			summaryMsg := fmt.Sprintf("[System: Planning phase completed - %d tokens used]", tokensUsed)
			orch.ConversationHistory.AddMessage("system", summaryMsg)
		}

		// Safely evaluate and wipe buffer
		planningBuf.Reset()
	}()

	if planFunc != nil {
		return planFunc()
	}
	return nil
}

// TrackTokens calculates and strictly increments token usage based on buffer size,
// without writing raw content to the history.
func (cm *ContextManager) TrackTokens(buf *strings.Builder) int {
	if cm == nil || buf == nil {
		return 0
	}
	// Approximate 4 bytes per token for buffer text
	tokens := buf.Len() / 4
	if tokens == 0 && buf.Len() > 0 {
		tokens = 1
	}

	cm.mu.Lock()
	cm.inputTokens += tokens
	cm.totalInputTokens += tokens
	cm.mu.Unlock()
	return tokens
}

// ExecutionMode controls which tools are allowed and how permissions are handled.
type ExecutionMode int

const (
	// ModeNormal is the default: all tools available, permissions as configured.
	ModeNormal ExecutionMode = iota
	// ModePlan blocks all write/execute/destructive tools.
	// The AI can only read, search, and reason. Useful for safe pre-flight analysis.
	ModePlan
	// ModeAutoEdit auto-approves non-destructive edit operations.
	ModeAutoEdit
)

// modeNames provides display labels for each mode.
var modeNames = map[ExecutionMode]string{
	ModeNormal:   "NORMAL",
	ModePlan:     "PLAN",
	ModeAutoEdit: "AUTO",
}

// modeColors for TUI display (ANSI colour names for Lip Gloss).
var modeDescriptions = map[ExecutionMode]string{
	ModeNormal:   "All tools available",
	ModePlan:     "Read-only: write/execute tools blocked",
	ModeAutoEdit: "Auto-approve edit operations",
}

// planModeBlockedTools lists tools that are blocked in ModePlan.
var planModeBlockedTools = map[string]bool{
	"bash":                     true,
	"write_file":               true,
	"edit_file":                true,
	"multi_edit_file":          true,
	"delete_file":              true,
	"git_commit":               true,
	"git_push":                 true,
	"git_pull":                 true,
	"kill_process":             true,
	"start_background_process": true,
	"stop_background_process":  true,
	"pkg_install":              true,
	"db_migrate":               true,
	"create_tool":              true,
	"modify_tool":              true,
}

// autoEditApprovedTools lists tools that are auto-approved in ModeAutoEdit.
var autoEditApprovedTools = map[string]bool{
	"write_file":      true,
	"edit_file":       true,
	"multi_edit_file": true,
	"read_file":       true,
	"list_directory":  true,
	"search_files":    true,
	"grep_content":    true,
	"file_info":       true,
}

// ModeManager manages the current execution mode.
type ModeManager struct {
	mu   sync.RWMutex
	mode ExecutionMode
	// onModeChange is called whenever the mode changes (nil = no-op).
	onModeChange func(from, to ExecutionMode)
}

// NewModeManager creates a ModeManager starting in ModeNormal.
func NewModeManager() *ModeManager {
	return &ModeManager{mode: ModeNormal}
}

// SetOnChange registers a callback for mode changes.
func (mm *ModeManager) SetOnChange(fn func(from, to ExecutionMode)) {
	mm.mu.Lock()
	mm.onModeChange = fn
	mm.mu.Unlock()
}

// Current returns the active execution mode.
func (mm *ModeManager) Current() ExecutionMode {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.mode
}

// Set changes the execution mode.
func (mm *ModeManager) Set(m ExecutionMode) {
	mm.mu.Lock()
	old := mm.mode
	mm.mode = m
	cb := mm.onModeChange
	mm.mu.Unlock()

	if cb != nil && old != m {
		cb(old, m)
	}
}

// Cycle advances through Normal → Plan → AutoEdit → Normal.
func (mm *ModeManager) Cycle() ExecutionMode {
	mm.mu.Lock()
	next := ExecutionMode((int(mm.mode) + 1) % 3)
	old := mm.mode
	mm.mode = next
	cb := mm.onModeChange
	mm.mu.Unlock()

	if cb != nil && old != next {
		cb(old, next)
	}
	return next
}

// Name returns the display name of the current mode.
func (mm *ModeManager) Name() string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return modeNames[mm.mode]
}

// SetMode switches the mode by name string ("NORMAL", "PLAN", "AUTO").
// Satisfies the adaptive.ModeManagerIface interface.
func (mm *ModeManager) SetMode(name string) {
	switch name {
	case "PLAN":
		mm.Set(ModePlan)
	case "AUTO", "AUTOEDIT":
		mm.Set(ModeAutoEdit)
	default:
		mm.Set(ModeNormal)
	}
}

// Description returns a human-readable description of the current mode.
func (mm *ModeManager) Description() string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return modeDescriptions[mm.mode]
}

// IsToolAllowed returns false if the current mode blocks a tool,
// and whether the tool should be auto-approved.
func (mm *ModeManager) IsToolAllowed(toolName string) (allowed bool, autoApprove bool) {
	mm.mu.RLock()
	mode := mm.mode
	mm.mu.RUnlock()

	switch mode {
	case ModePlan:
		if planModeBlockedTools[toolName] {
			return false, false
		}
		return true, false
	case ModeAutoEdit:
		return true, autoEditApprovedTools[toolName]
	default: // ModeNormal
		return true, false
	}
}

// SystemPromptInjection returns text to prepend to the system prompt
// based on the current mode.
func (mm *ModeManager) SystemPromptInjection() string {
	mm.mu.RLock()
	mode := mm.mode
	mm.mu.RUnlock()

	switch mode {
	case ModePlan:
		return fmt.Sprintf(`
### EXECUTION MODE: PLAN (READ-ONLY)
You are currently in PLAN MODE. This is a safe analysis mode.
STRICT RULES:
- You may ONLY use read/search/analysis tools: read_file, list_directory, search_files, grep_content, file_info, git_status, git_diff, git_log, web_fetch, system_info, disk_usage
- You MUST NOT use write_file, edit_file, bash, git_commit, git_push, or any destructive tool
- Your role is to research, analyze, and present a structured plan
- End your response with a clear "## Implementation Plan" section listing the exact steps you would take
- The user will review your plan and switch to NORMAL mode to execute it
`)
	case ModeAutoEdit:
		return `
### EXECUTION MODE: AUTO-EDIT
File read and edit operations are auto-approved. Bash commands still require approval.
`
	default:
		return ""
	}
}

// FormatModeChange returns a display string for a mode change notification.
func FormatModeChange(from, to ExecutionMode) string {
	fromName := modeNames[from]
	toName := modeNames[to]
	desc := modeDescriptions[to]
	return fmt.Sprintf("Mode: %s -> **%s** — %s", fromName, toName, desc)
}
