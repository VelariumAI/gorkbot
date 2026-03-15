package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/billing"
)

// BudgetAction describes the guard's recommendation for the current turn.
type BudgetAction int

const (
	BudgetAllow BudgetAction = iota
	BudgetWarn
	BudgetBlock
)

// BudgetDecision is returned by CheckAndTrack.
type BudgetDecision struct {
	Action        BudgetAction
	EstimatedCost float64
	Message       string
}

// BudgetGuard enforces per-session and per-day USD spending limits.
// Limits of 0 mean unlimited for that scope.
type BudgetGuard struct {
	billing      *billing.BillingManager
	SessionLimit float64 // USD limit for this session (0 = unlimited)
	DailyLimit   float64 // USD rolling-24h limit (0 = unlimited)
	WarnAt       float64 // fraction of limit to warn at (default 0.8)

	mu          sync.RWMutex
	dailySpend  float64
	lastReset   time.Time
	sessionBase float64 // BillingManager.TotalSession value at guard creation
}

// NewBudgetGuard creates a BudgetGuard. Pass 0 for limits you don't need.
func NewBudgetGuard(bm *billing.BillingManager, sessionLimit, dailyLimit float64) *BudgetGuard {
	bg := &BudgetGuard{
		billing:      bm,
		SessionLimit: sessionLimit,
		DailyLimit:   dailyLimit,
		WarnAt:       0.8,
		lastReset:    time.Now(),
	}
	if bm != nil {
		bg.sessionBase = bm.GetTotalSessionCost()
	}
	return bg
}

// EstimateCost computes the expected USD cost for a single turn.
func (bg *BudgetGuard) EstimateCost(providerID, modelID string, inputToks, outputToks int) float64 {
	if bg.billing == nil {
		return 0
	}
	bg.billing.Mu.RLock()
	defer bg.billing.Mu.RUnlock()

	provLower := strings.ToLower(providerID)
	prov, ok := bg.billing.Config.Providers[provLower]
	if !ok {
		return 0
	}
	lowerModel := strings.ToLower(modelID)
	for _, entry := range prov.Entries {
		if strings.Contains(lowerModel, strings.ToLower(entry.Prefix)) {
			in := float64(inputToks) / 1_000_000.0 * entry.InputPerM
			out := float64(outputToks) / 1_000_000.0 * entry.OutputPerM
			return in + out
		}
	}
	return 0
}

// CheckAndTrack evaluates budget state before a turn and returns a decision.
// It does NOT deduct spend — the BillingManager does that after the actual call.
func (bg *BudgetGuard) CheckAndTrack(providerID, modelID string, historyToks, promptToks int) BudgetDecision {
	if bg == nil {
		return BudgetDecision{Action: BudgetAllow}
	}

	bg.mu.Lock()
	// Reset daily spend every 24 hours.
	if time.Since(bg.lastReset) >= 24*time.Hour {
		bg.dailySpend = 0
		bg.lastReset = time.Now()
	}
	bg.mu.Unlock()

	estimated := bg.EstimateCost(providerID, modelID, historyToks+promptToks, 2048)

	// Session spend = total billed so far this session.
	sessionSpend := 0.0
	if bg.billing != nil {
		sessionSpend = bg.billing.GetTotalSessionCost() - bg.sessionBase
	}

	bg.mu.RLock()
	dailySpend := bg.dailySpend
	bg.mu.RUnlock()

	// Check daily limit.
	if bg.DailyLimit > 0 {
		projectedDaily := dailySpend + estimated
		switch {
		case projectedDaily > bg.DailyLimit:
			return BudgetDecision{
				Action:        BudgetBlock,
				EstimatedCost: estimated,
				Message:       fmt.Sprintf("daily budget exceeded: $%.4f of $%.2f limit used", dailySpend, bg.DailyLimit),
			}
		case projectedDaily > bg.DailyLimit*bg.WarnAt:
			return BudgetDecision{
				Action:        BudgetWarn,
				EstimatedCost: estimated,
				Message:       fmt.Sprintf("approaching daily limit: $%.4f of $%.2f used (est. +$%.4f)", dailySpend, bg.DailyLimit, estimated),
			}
		}
	}

	// Check session limit.
	if bg.SessionLimit > 0 {
		projectedSession := sessionSpend + estimated
		switch {
		case projectedSession > bg.SessionLimit:
			return BudgetDecision{
				Action:        BudgetBlock,
				EstimatedCost: estimated,
				Message:       fmt.Sprintf("session budget exceeded: $%.4f of $%.2f limit", sessionSpend, bg.SessionLimit),
			}
		case projectedSession > bg.SessionLimit*bg.WarnAt:
			return BudgetDecision{
				Action:        BudgetWarn,
				EstimatedCost: estimated,
				Message:       fmt.Sprintf("approaching session limit: $%.4f of $%.2f used (est. +$%.4f)", sessionSpend, bg.SessionLimit, estimated),
			}
		}
	}

	// Accrue to daily tracker.
	bg.mu.Lock()
	bg.dailySpend += estimated
	bg.mu.Unlock()

	return BudgetDecision{Action: BudgetAllow, EstimatedCost: estimated}
}

// SessionCost returns the session spend tracked by the billing manager.
func (bg *BudgetGuard) SessionCost() float64 {
	if bg == nil || bg.billing == nil {
		return 0
	}
	return bg.billing.GetTotalSessionCost() - bg.sessionBase
}
