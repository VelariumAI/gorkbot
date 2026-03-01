package engine

import (
	"fmt"
	"sync"
	"time"
	
	"github.com/velariumai/gorkbot/pkg/billing"
)

// TokenUsage carries token counts from an AI API response.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	CachedTokens int
	ProviderID   string
	ModelID      string
}

// ContextManager tracks context window usage and fires a callback when
// the window approaches capacity.
type ContextManager struct {
	mu           sync.RWMutex
	maxTokens    int     // Provider context limit
	inputTokens  int     // Current input token count (updated after each call)
	outputTokens int     // Cumulative output tokens for cost tracking
	compactPct   float64 // Trigger threshold (default 0.90)

	// Cost tracking (USD) via billing manager
	Billing      *billing.BillingManager
	sessionStart time.Time

	// Callbacks
	onNearFull func() // Called when usage exceeds compactPct

	// Session stats
	totalInputTokens  int
	totalOutputTokens int
	turnCount         int
}

// NewContextManager creates a ContextManager for a model with the given
// context window size (tokens).
func NewContextManager(maxTokens int, onNearFull func()) *ContextManager {
	if maxTokens <= 0 {
		maxTokens = 131072 // Grok-3 default
	}
	return &ContextManager{
		maxTokens:      maxTokens,
		compactPct:     0.90,
		onNearFull:     onNearFull,
		sessionStart:   time.Now(),
		Billing:        billing.NewBillingManager(),
	}
}

// UpdateFromUsage updates token counts from an API response.
// Call this after every AI response.
func (cm *ContextManager) UpdateFromUsage(usage TokenUsage) {
	cm.mu.Lock()
	cm.inputTokens = usage.InputTokens
	cm.outputTokens += usage.OutputTokens
	cm.totalInputTokens += usage.InputTokens
	cm.totalOutputTokens += usage.OutputTokens
	cm.turnCount++
	
	if cm.Billing != nil {
		cm.Billing.TrackTurn(usage.ProviderID, usage.ModelID, usage.InputTokens, usage.OutputTokens)
	}

	nearFull := cm.usedPctLocked() > cm.compactPct
	cb := cm.onNearFull
	cm.mu.Unlock()

	if nearFull && cb != nil {
		cb()
	}
}

// SetInputTokens directly sets the current input token count.
// Useful when the provider reports tokens differently.
func (cm *ContextManager) SetInputTokens(n int) {
	cm.mu.Lock()
	cm.inputTokens = n
	cm.mu.Unlock()
}

// UsedPct returns the fraction of the context window currently used (0.0–1.0).
func (cm *ContextManager) UsedPct() float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.usedPctLocked()
}

func (cm *ContextManager) usedPctLocked() float64 {
	if cm.maxTokens == 0 {
		return 0
	}
	return float64(cm.inputTokens) / float64(cm.maxTokens)
}

// InputTokens returns the current input token count.
func (cm *ContextManager) InputTokens() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.inputTokens
}

// MaxTokens returns the context window size.
func (cm *ContextManager) MaxTokens() int { return cm.maxTokens }

// TokensUsed returns the current input token count (satisfies ContextStatsReporter).
func (cm *ContextManager) TokensUsed() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.inputTokens
}

// TokenLimit returns the context window size (satisfies ContextStatsReporter).
func (cm *ContextManager) TokenLimit() int { return cm.maxTokens }

// TotalCostUSD returns the estimated session cost in US dollars.
func (cm *ContextManager) TotalCostUSD() float64 {
	if cm.Billing != nil {
		return cm.Billing.GetTotalSessionCost()
	}
	return 0.0
}

// SessionDuration returns how long this session has been running.
func (cm *ContextManager) SessionDuration() time.Duration {
	return time.Since(cm.sessionStart)
}

// StatusBar returns a compact status string for the status bar.
// Format: "●●●●●○○○ 62% | $0.04"
func (cm *ContextManager) StatusBar() string {
	cm.mu.RLock()
	pct := cm.usedPctLocked()
	cm.mu.RUnlock()

	cost := cm.TotalCostUSD()
	bar := contextBar(pct, 8)
	pctStr := fmt.Sprintf("%d%%", int(pct*100))
	costStr := fmt.Sprintf("$%.3f", cost)
	if cm.Billing != nil {
		costStr = cm.Billing.GetCostString()
	}
	return fmt.Sprintf("%s %s | %s", bar, pctStr, costStr)
}

// ContextBreakdown returns a detailed context usage report for /context command.
func (cm *ContextManager) ContextBreakdown(systemTokens, conversationTokens, toolResultTokens, lastResponseTokens int) string {
	cm.mu.RLock()
	maxT := cm.maxTokens
	inT := cm.inputTokens
	totalIn := cm.totalInputTokens
	totalOut := cm.totalOutputTokens
	turns := cm.turnCount
	dur := time.Since(cm.sessionStart)
	cm.mu.RUnlock()

	cost := cm.TotalCostUSD()

	pct := float64(inT) / float64(maxT) * 100
	bar := contextBar(pct/100, 20)

	var sb string
	sb += fmt.Sprintf("# Context Window\n\n")
	sb += fmt.Sprintf("%s %.1f%% used\n\n", bar, pct)
	sb += fmt.Sprintf("| Component          | Tokens  | %% of window |\n")
	sb += fmt.Sprintf("|--------------------|---------|-------------|\n")
	if systemTokens > 0 {
		sb += fmt.Sprintf("| System Prompt      | %7d | %10.1f%% |\n", systemTokens, pct100(systemTokens, maxT))
	}
	if conversationTokens > 0 {
		sb += fmt.Sprintf("| Conversation       | %7d | %10.1f%% |\n", conversationTokens, pct100(conversationTokens, maxT))
	}
	if toolResultTokens > 0 {
		sb += fmt.Sprintf("| Tool Results       | %7d | %10.1f%% |\n", toolResultTokens, pct100(toolResultTokens, maxT))
	}
	if lastResponseTokens > 0 {
		sb += fmt.Sprintf("| Last Response      | %7d | %10.1f%% |\n", lastResponseTokens, pct100(lastResponseTokens, maxT))
	}
	sb += fmt.Sprintf("| **Total**          | **%5d** | **%9.1f%%** |\n\n", inT, pct)

	sb += fmt.Sprintf("**Session Stats**\n")
	sb += fmt.Sprintf("- Turns: %d\n", turns)
	sb += fmt.Sprintf("- Total input: %s tokens\n", formatTokens(totalIn))
	sb += fmt.Sprintf("- Total output: %s tokens\n", formatTokens(totalOut))
	sb += fmt.Sprintf("- Est. cost: $%.4f\n", cost)
	sb += fmt.Sprintf("- Duration: %s\n\n", dur.Round(time.Second))

	if pct > 85 {
		sb += "> **Warning:** Context is nearly full. Use `/compact` to compress.\n"
	} else if pct > 70 {
		sb += "> Tip: Use `/compact [focus]` to reduce context when approaching 90%.\n"
	}

	return sb
}

// CostReport returns a formatted session cost report for /cost command.
func (cm *ContextManager) CostReport(primaryModel, consultantModel string) string {
	cm.mu.RLock()
	dur := time.Since(cm.sessionStart)
	cm.mu.RUnlock()

	var sb string
	sb += "# Session Cost\n\n"
	
	totalCost := cm.TotalCostUSD()

	if cm.Billing != nil {
		cm.Billing.Mu.RLock()
		defer cm.Billing.Mu.RUnlock()
		
		sb += fmt.Sprintf("| Model | Input Tokens | Output Tokens | Est. Cost |\n")
		sb += fmt.Sprintf("|-------|-------------|---------------|----------|\n")
		
		for model, usage := range cm.Billing.Session {
			sb += fmt.Sprintf("| %s | %s | %s | $%.4f |\n",
				model, formatTokens(usage.InputTokens), formatTokens(usage.OutputTokens), usage.TotalCost)
		}
	} else {
		// Fallback if billing manager is somehow nil
		cm.mu.RLock()
		totalIn := cm.totalInputTokens
		totalOut := cm.totalOutputTokens
		cm.mu.RUnlock()
		
		sb += fmt.Sprintf("| Provider | Input Tokens | Output Tokens | Est. Cost |\n")
		sb += fmt.Sprintf("|----------|-------------|---------------|----------|\n")
		sb += fmt.Sprintf("| %s | %s | %s | $%.4f |\n",
			primaryModel, formatTokens(totalIn), formatTokens(totalOut), totalCost)
	}

	hourly := 0.0
	if dur.Minutes() > 0 {
		hourly = totalCost / dur.Hours()
	}

	sb += fmt.Sprintf("\n**Total session cost:** $%.4f\n", totalCost)
	sb += fmt.Sprintf("**Session duration:** %s\n", dur.Round(time.Second))
	if hourly > 0 {
		sb += fmt.Sprintf("**Est. hourly rate:** $%.2f/hr\n", hourly)
	}
	sb += "\n_Note: Costs are estimates based on approximate token pricing._\n"
	return sb
}

// contextBar returns an ASCII progress bar of width w representing the fraction f.
func contextBar(f float64, w int) string {
	if f > 1 {
		f = 1
	}
	if f < 0 {
		f = 0
	}
	filled := int(f * float64(w))
	bar := make([]byte, w)
	for i := range bar {
		if i < filled {
			bar[i] = 0xe2 // UTF-8 ● (multi-byte, use rune)
		}
	}
	// Use simple ASCII for safety
	result := "["
	for i := 0; i < w; i++ {
		if i < filled {
			result += "#"
		} else {
			result += "-"
		}
	}
	result += "]"
	return result
}

func pct100(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

func formatTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
