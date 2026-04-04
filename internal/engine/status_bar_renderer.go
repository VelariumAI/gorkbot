package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// StatusBarRenderer creates professional status bars for TUI with real-time metrics
type StatusBarRenderer struct {
	mu sync.RWMutex

	// Context metrics
	contextUsagePercent atomic.Int64
	maxContextTokens    int64
	currentTokens       int64

	// Cost metrics
	sessionCost         atomic.Uint64
	costPerMinute       atomic.Uint64
	budgetRemaining     atomic.Int64

	// Performance metrics
	tokenThroughput     atomic.Uint64 // tokens/sec
	activeToolCount     atomic.Int32
	averageLatency      atomic.Uint64

	// Interrupt state
	isInterrupted atomic.Bool
	interruptReason string

	// Aesthetics
	barWidth     int
	showDetailed bool
}

// NewStatusBarRenderer creates a new professional status bar renderer
func NewStatusBarRenderer(maxTokens int64) *StatusBarRenderer {
	return &StatusBarRenderer{
		maxContextTokens: maxTokens,
		barWidth:         16,
		showDetailed:     true,
	}
}

// UpdateContext updates context usage for display
func (sbr *StatusBarRenderer) UpdateContext(current, max int64) {
	sbr.currentTokens = current
	sbr.maxContextTokens = max

	pct := int64(0)
	if max > 0 {
		pct = (current * 100) / max
	}
	sbr.contextUsagePercent.Store(pct)
}

// UpdateCost updates cost metrics
func (sbr *StatusBarRenderer) UpdateCost(costUSD float64, budgetRemaining float64, costPerMin float64) {
	sbr.sessionCost.Store(uint64(costUSD * 1000000)) // Store as microdollars
	sbr.budgetRemaining.Store(int64(budgetRemaining * 100))
	sbr.costPerMinute.Store(uint64(costPerMin * 1000000))
}

// UpdatePerformance updates performance metrics
func (sbr *StatusBarRenderer) UpdatePerformance(throughput uint64, activeTools int32, avgLatencyMs uint64) {
	sbr.tokenThroughput.Store(throughput)
	sbr.activeToolCount.Store(activeTools)
	sbr.averageLatency.Store(avgLatencyMs)
}

// MarkInterrupted marks that interrupt has been requested
func (sbr *StatusBarRenderer) MarkInterrupted(reason string) {
	sbr.isInterrupted.Store(true)
	sbr.mu.Lock()
	sbr.interruptReason = reason
	sbr.mu.Unlock()
}

// RenderCompact returns single-line status bar
func (sbr *StatusBarRenderer) RenderCompact() string {
	pct := float64(sbr.contextUsagePercent.Load()) / 100.0
	contextBar := sbr.renderBar(pct, sbr.barWidth)

	cost := float64(sbr.sessionCost.Load()) / 1000000
	costStr := fmt.Sprintf("$%.3f", cost)
	if cost >= 0.01 {
		costStr = fmt.Sprintf("$%.2f", cost)
	}

	throughput := sbr.tokenThroughput.Load()
	throughputStr := ""
	if throughput > 0 {
		throughputStr = fmt.Sprintf(" | %d tok/s", throughput)
	}

	pctStr := fmt.Sprintf("%3d%%", sbr.contextUsagePercent.Load())

	return fmt.Sprintf("%s %s | %s%s", contextBar, pctStr, costStr, throughputStr)
}

// RenderDetailed returns multi-line detailed status
func (sbr *StatusBarRenderer) RenderDetailed() string {
	status := "## Session Status\n\n"

	// Context Window
	pct := float64(sbr.contextUsagePercent.Load()) / 100.0
	bar := sbr.renderBar(pct, 20)
	status += fmt.Sprintf("**Context**: %s %d%%\n", bar, sbr.contextUsagePercent.Load())
	status += fmt.Sprintf("- Used: %d / %d tokens\n", sbr.currentTokens, sbr.maxContextTokens)

	// Cost
	cost := float64(sbr.sessionCost.Load()) / 1000000
	remaining := float64(sbr.budgetRemaining.Load()) / 100
	costPerMin := float64(sbr.costPerMinute.Load()) / 1000000

	status += fmt.Sprintf("\n**Cost**: %.4f (Remaining: $%.2f)\n", cost, remaining)
	status += fmt.Sprintf("- Rate: $%.4f/min\n", costPerMin)

	// Performance
	throughput := sbr.tokenThroughput.Load()
	latency := sbr.averageLatency.Load()
	tools := sbr.activeToolCount.Load()

	status += fmt.Sprintf("\n**Performance**:\n")
	status += fmt.Sprintf("- Throughput: %d tokens/sec\n", throughput)
	status += fmt.Sprintf("- Avg Latency: %dms\n", latency)
	status += fmt.Sprintf("- Active Tools: %d\n", tools)

	// Interrupt status
	if sbr.isInterrupted.Load() {
		sbr.mu.RLock()
		reason := sbr.interruptReason
		sbr.mu.RUnlock()
		status += fmt.Sprintf("\n⚠ **Interrupted**: %s\n", reason)
	}

	return status
}

// RenderMinimal returns ultra-compact status (3-4 chars)
func (sbr *StatusBarRenderer) RenderMinimal() string {
	pct := sbr.contextUsagePercent.Load()

	if pct >= 95 {
		return "🔴" // Critical
	}
	if pct >= 85 {
		return "🟠" // Warning
	}
	if pct >= 70 {
		return "🟡" // Caution
	}
	return "🟢" // Healthy
}

// RenderBar returns a visual progress bar
func (sbr *StatusBarRenderer) renderBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}

	filled := int(float64(width) * pct)
	empty := width - filled

	bar := "█"
	for i := 0; i < filled-1; i++ {
		bar += "█"
	}
	if filled > 0 {
		bar += "▓"
	}
	for i := 0; i < empty; i++ {
		bar += "░"
	}

	return "[" + bar + "]"
}

// HealthScore returns overall health (0-100)
func (sbr *StatusBarRenderer) HealthScore() int {
	score := 100

	// Context impact (40% weight)
	contextPct := sbr.contextUsagePercent.Load()
	if contextPct > 90 {
		score -= 40
	} else if contextPct > 75 {
		score -= 20
	} else if contextPct > 50 {
		score -= 5
	}

	// Cost impact (30% weight)
	remaining := sbr.budgetRemaining.Load()
	if remaining < 100 { // < $1.00
		score -= 30
	} else if remaining < 500 { // < $5.00
		score -= 15
	}

	// Performance impact (30% weight)
	latency := sbr.averageLatency.Load()
	if latency > 5000 { // > 5s
		score -= 30
	} else if latency > 2000 { // > 2s
		score -= 15
	}

	if score < 0 {
		score = 0
	}
	return int(score)
}

// HealthEmoji returns visual health indicator
func (sbr *StatusBarRenderer) HealthEmoji() string {
	score := sbr.HealthScore()
	if score >= 80 {
		return "✨"
	}
	if score >= 60 {
		return "⚙"
	}
	if score >= 40 {
		return "⚠"
	}
	return "🔴"
}

