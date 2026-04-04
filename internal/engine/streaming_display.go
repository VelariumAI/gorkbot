package engine

import (
	"fmt"
	"sync"
	"time"
)

// ToolExecutionDisplay tracks tool execution state for real-time TUI display
type ToolExecutionDisplay struct {
	mu sync.RWMutex

	// Current execution
	ToolName  string
	StartTime time.Time
	Elapsed   time.Duration
	Status    string // "pending", "running", "completed", "failed"
	Output    string // Last output/result snippet

	// Metrics
	TokensUsed  int64
	Success     bool
	ErrorMsg    string
	ResultSize  int // Bytes of result

	// Real-time updates
	lastUpdate time.Time
	updateChan chan ToolUpdate
}

// ToolUpdate notifies display of tool execution changes
type ToolUpdate struct {
	ToolName   string
	Status     string
	Output     string
	Elapsed    time.Duration
	TokensUsed int64
	Success    bool
	ErrorMsg   string
}

// NewToolExecutionDisplay creates display tracker for tool execution
func NewToolExecutionDisplay() *ToolExecutionDisplay {
	return &ToolExecutionDisplay{
		updateChan: make(chan ToolUpdate, 100),
	}
}

// BeginToolExecution marks tool start for display
func (ted *ToolExecutionDisplay) BeginToolExecution(toolName string) {
	ted.mu.Lock()
	defer ted.mu.Unlock()

	ted.ToolName = toolName
	ted.StartTime = time.Now()
	ted.Elapsed = 0
	ted.Status = "pending"
	ted.Output = ""
	ted.TokensUsed = 0
	ted.Success = false
	ted.ErrorMsg = ""
	ted.ResultSize = 0
}

// UpdateToolOutput updates the tool's output display
func (ted *ToolExecutionDisplay) UpdateToolOutput(output string, truncateAt int) {
	ted.mu.Lock()
	defer ted.mu.Unlock()

	ted.Status = "running"
	if len(output) > truncateAt {
		ted.Output = output[:truncateAt] + "..."
	} else {
		ted.Output = output
	}

	ted.lastUpdate = time.Now()
}

// CompleteToolExecution marks tool completion with result
func (ted *ToolExecutionDisplay) CompleteToolExecution(success bool, output string, tokens int64) {
	ted.mu.Lock()
	defer ted.mu.Unlock()

	ted.Status = "completed"
	ted.Success = success
	ted.TokensUsed = tokens
	ted.Elapsed = time.Since(ted.StartTime)

	if len(output) > 500 {
		ted.Output = output[:500] + " [truncated]"
	} else {
		ted.Output = output
	}

	ted.ResultSize = len(output)
	ted.lastUpdate = time.Now()
}

// FailToolExecution marks tool failure
func (ted *ToolExecutionDisplay) FailToolExecution(err string) {
	ted.mu.Lock()
	defer ted.mu.Unlock()

	ted.Status = "failed"
	ted.Success = false
	ted.ErrorMsg = err
	ted.Elapsed = time.Since(ted.StartTime)
	ted.lastUpdate = time.Now()
}

// RenderDisplay returns formatted display string for TUI
func (ted *ToolExecutionDisplay) RenderDisplay() string {
	ted.mu.RLock()
	defer ted.mu.RUnlock()

	if ted.ToolName == "" {
		return ""
	}

	status := ted.statusGlyph() + " " + ted.ToolName

	if ted.StartTime.IsZero() {
		return status
	}

	elapsed := time.Since(ted.StartTime)
	statusStr := fmt.Sprintf("%s | %s", status, formatElapsed(elapsed))

	if ted.TokensUsed > 0 {
		statusStr += fmt.Sprintf(" | %d tokens", ted.TokensUsed)
	}

	if ted.ErrorMsg != "" {
		statusStr += fmt.Sprintf(" | ❌ %s", ted.ErrorMsg)
	}

	return statusStr
}

// RenderDetailedOutput returns full tool output for display
func (ted *ToolExecutionDisplay) RenderDetailedOutput() string {
	ted.mu.RLock()
	defer ted.mu.RUnlock()

	if ted.Output == "" {
		return fmt.Sprintf("## %s\n\n(No output yet)", ted.ToolName)
	}

	report := fmt.Sprintf("## %s Execution\n\n", ted.ToolName)
	report += fmt.Sprintf("**Status**: %s\n", ted.Status)
	report += fmt.Sprintf("**Duration**: %s\n", formatElapsed(ted.Elapsed))

	if ted.TokensUsed > 0 {
		report += fmt.Sprintf("**Tokens Used**: %d\n", ted.TokensUsed)
	}

	if ted.ResultSize > 0 {
		report += fmt.Sprintf("**Result Size**: %d bytes\n", ted.ResultSize)
	}

	report += fmt.Sprintf("\n### Output\n\n%s\n", ted.Output)

	return report
}

func (ted *ToolExecutionDisplay) statusGlyph() string {
	switch ted.Status {
	case "running":
		return "⟳"
	case "completed":
		if ted.Success {
			return "✓"
		}
		return "✗"
	case "failed":
		return "✗"
	case "pending":
		return "⧁"
	default:
		return "◦"
	}
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

