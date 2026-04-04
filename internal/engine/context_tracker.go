package engine

import (
	"fmt"
	"sync"
	"time"
)

// ContextBreakdownMetrics tracks token usage by component for precise compaction decisions
type ContextBreakdownMetrics struct {
	SystemTokens       int64 // System prompt and instructions
	ConversationTokens int64 // User/assistant messages
	ToolResultTokens   int64 // Tool execution outputs
	CacheTokens        int64 // Cached tokens (less valuable for context)
	TotalTokens        int64 // Sum of above
}

// CompactionTrigger defines reasons why compaction was triggered
type CompactionTrigger int

const (
	TriggerManual CompactionTrigger = iota
	TriggerThreshold
	TriggerPredictive
	TriggerCritical
)

func (ct CompactionTrigger) String() string {
	switch ct {
	case TriggerManual:
		return "manual"
	case TriggerThreshold:
		return "threshold"
	case TriggerPredictive:
		return "predictive"
	case TriggerCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// CompactionEvent records when and why compaction occurred
type CompactionEvent struct {
	Timestamp       time.Time
	TokensBefore    int
	TokensAfter     int
	Trigger         CompactionTrigger
	TokensReclaimed int64
}

// ContextTracker provides advanced context window tracking with predictive auto-compaction
type ContextTracker struct {
	mu sync.RWMutex

	// Current state
	maxTokens      int64
	currentTokens  int64
	breakdown      ContextBreakdownMetrics
	compactPct     float64 // Threshold percentage (e.g., 0.85)
	criticalPct    float64 // Critical threshold (e.g., 0.95)

	// Historical metrics
	peakUsage      int64                // Highest token count seen
	avgTurnsPerMin float64              // Moving average for predictive compaction
	turnCount      int64                // Total turns
	compactionCount int64               // Number of times compacted
	lastCompaction time.Time            // Last compaction timestamp

	// Compaction events
	events            []CompactionEvent
	maxEventHistory   int
	lastCompactionMsg string

	// Predictive auto-compaction
	turnsUntilFull    int64 // Estimated turns until critical (updated continuously)
	predictiveEnabled bool

	// Observability hook
	onCompaction func(event CompactionEvent)
}

// NewContextTracker creates an advanced context tracker with predictive compaction
func NewContextTracker(maxTokens int64, compactThreshold, criticalThreshold float64) *ContextTracker {
	if maxTokens <= 0 {
		maxTokens = 131072 // Conservative default
	}
	return &ContextTracker{
		maxTokens:       maxTokens,
		compactPct:      compactThreshold,
		criticalPct:     criticalThreshold,
		maxEventHistory: 50,
		events:          make([]CompactionEvent, 0, 50),
		predictiveEnabled: true,
		avgTurnsPerMin:  1.0,
		turnsUntilFull:  estimateTurnsToFull(0, maxTokens, int64(float64(maxTokens)*criticalThreshold)),
	}
}

// UpdateBreakdown updates component-level token tracking for surgical compaction
func (ct *ContextTracker) UpdateBreakdown(systemTokens, conversationTokens, toolResultTokens, cacheTokens int64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.breakdown.SystemTokens = systemTokens
	ct.breakdown.ConversationTokens = conversationTokens
	ct.breakdown.ToolResultTokens = toolResultTokens
	ct.breakdown.CacheTokens = cacheTokens
	ct.breakdown.TotalTokens = systemTokens + conversationTokens + toolResultTokens + cacheTokens

	ct.currentTokens = ct.breakdown.TotalTokens

	// Update peak usage
	if ct.currentTokens > ct.peakUsage {
		ct.peakUsage = ct.currentTokens
	}

	// Update predictive tracking
	ct.turnCount++
	if ct.predictiveEnabled {
		ct.updatePredictiveCompaction()
	}
}

// RecordTurn logs a turn for predictive compaction calculations
func (ct *ContextTracker) RecordTurn(tokensAdded int64) {
	ct.mu.Lock()
	ct.currentTokens += tokensAdded
	ct.turnCount++
	ct.mu.Unlock()
}

// updatePredictiveCompaction estimates turns until critical threshold
func (ct *ContextTracker) updatePredictiveCompaction() {
	// Based on current growth rate, estimate how many turns until critical
	pct := float64(ct.currentTokens) / float64(ct.maxTokens)
	if pct < 0.1 {
		ct.turnsUntilFull = 100 // Early stage, very conservative estimate
		return
	}

	// Growth rate calculation: tokens growing per turn
	if ct.turnCount > 0 {
		avgTokensPerTurn := ct.currentTokens / ct.turnCount
		if avgTokensPerTurn > 0 {
			criticalTokens := int64(float64(ct.maxTokens) * ct.criticalPct)
			turnsRemaining := (criticalTokens - ct.currentTokens) / avgTokensPerTurn
			if turnsRemaining < 0 {
				turnsRemaining = 0
			}
			ct.turnsUntilFull = turnsRemaining
		}
	}
}

// ShouldCompactNow returns true if compaction should happen immediately + reason
func (ct *ContextTracker) ShouldCompactNow() (bool, CompactionTrigger) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	pct := float64(ct.currentTokens) / float64(ct.maxTokens)

	// Critical threshold - MUST compact
	if pct >= ct.criticalPct {
		return true, TriggerCritical
	}

	// Predictive compaction - compact early if trajectory is steep
	if ct.predictiveEnabled && ct.turnsUntilFull < 5 && pct > ct.compactPct*0.8 {
		return true, TriggerPredictive
	}

	// Standard threshold - compact when approaching limit
	if pct >= ct.compactPct {
		return true, TriggerThreshold
	}

	return false, -1
}

// RecordCompaction logs a compaction event with tokens reclaimed
func (ct *ContextTracker) RecordCompaction(trigger CompactionTrigger, tokensReclaimed int64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	before := ct.currentTokens
	after := before - tokensReclaimed

	event := CompactionEvent{
		Timestamp:       time.Now(),
		TokensBefore:    int(before),
		TokensAfter:     int(after),
		Trigger:         trigger,
		TokensReclaimed: tokensReclaimed,
	}

	ct.events = append(ct.events, event)
	if len(ct.events) > ct.maxEventHistory {
		ct.events = ct.events[len(ct.events)-ct.maxEventHistory:]
	}

	ct.currentTokens = after
	ct.compactionCount++
	ct.lastCompaction = time.Now()
	ct.lastCompactionMsg = fmt.Sprintf(
		"Compacted %d tokens via %s | %d%% → %d%%",
		tokensReclaimed,
		trigger,
		int(float64(before)*100/float64(ct.maxTokens)),
		int(float64(after)*100/float64(ct.maxTokens)),
	)

	// Fire observability hook
	if ct.onCompaction != nil {
		ct.onCompaction(event)
	}
}

// UsagePercent returns current usage as percentage
func (ct *ContextTracker) UsagePercent() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return float64(ct.currentTokens) / float64(ct.maxTokens) * 100
}

// BreakdownReport returns detailed component usage for diagnostics
func (ct *ContextTracker) BreakdownReport() string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	pct := float64(ct.currentTokens) / float64(ct.maxTokens) * 100
	bar := contextBar(pct/100, 16)

	report := fmt.Sprintf("# Context Window Breakdown\n\n")
	report += fmt.Sprintf("%s **%.1f%% (%d/%d)**\n\n", bar, pct, ct.currentTokens, ct.maxTokens)
	report += fmt.Sprintf("| Component | Tokens | %% |\n")
	report += fmt.Sprintf("|-----------|--------|-----|\n")

	if ct.currentTokens > 0 {
		report += fmt.Sprintf("| System | %d | %.1f%% |\n",
			ct.breakdown.SystemTokens,
			float64(ct.breakdown.SystemTokens)*100/float64(ct.currentTokens))
		report += fmt.Sprintf("| Conversation | %d | %.1f%% |\n",
			ct.breakdown.ConversationTokens,
			float64(ct.breakdown.ConversationTokens)*100/float64(ct.currentTokens))
		report += fmt.Sprintf("| Tool Results | %d | %.1f%% |\n",
			ct.breakdown.ToolResultTokens,
			float64(ct.breakdown.ToolResultTokens)*100/float64(ct.currentTokens))
		report += fmt.Sprintf("| Cached | %d | %.1f%% |\n",
			ct.breakdown.CacheTokens,
			float64(ct.breakdown.CacheTokens)*100/float64(ct.currentTokens))
	}

	report += fmt.Sprintf("\n**Peak Usage**: %d tokens\n", ct.peakUsage)
	report += fmt.Sprintf("**Turns to Critical**: %d\n", ct.turnsUntilFull)
	report += fmt.Sprintf("**Compactions**: %d\n", ct.compactionCount)

	if !ct.lastCompaction.IsZero() {
		report += fmt.Sprintf("**Last Compaction**: %s\n", formatTimeSince(time.Since(ct.lastCompaction)))
	}

	return report
}

// CompactionHistory returns recent compaction events for metrics/logging
func (ct *ContextTracker) CompactionHistory() []CompactionEvent {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	
	events := make([]CompactionEvent, len(ct.events))
	copy(events, ct.events)
	return events
}

// SetCompactionHook sets the callback for when compaction occurs
func (ct *ContextTracker) SetCompactionHook(fn func(event CompactionEvent)) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.onCompaction = fn
}

// estimateTurnsToFull calculates how many turns until critical
func estimateTurnsToFull(current, max, critical int64) int64 {
	if current >= critical {
		return 0
	}
	// Conservative estimate: 500 tokens per turn average
	remaining := critical - current
	return remaining / 500
}

// formatTimeSince formats duration nicely (e.g., "2m ago")
func formatTimeSince(d time.Duration) string {
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

