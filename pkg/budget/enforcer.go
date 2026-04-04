package budget

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// BudgetEnforcer manages cost enforcement across sessions and users
type BudgetEnforcer struct {
	policy *BudgetPolicy

	// Track remaining budget per session
	sessionBudgets map[string]float64
	// Track initial budget per session (for warning threshold calculations)
	sessionInitial map[string]float64
	sessionMu      sync.RWMutex

	// Track cumulative spending per user (for daily limits)
	userDailySpend map[string]float64
	userMu         sync.RWMutex

	// Track warnings sent to avoid spam
	warningsSent map[string]map[float64]bool // sessionID -> threshold -> sent
	warningMu    sync.RWMutex

	// Historical cost tracking (for better estimates)
	costHistory map[string][]float64 // modelName -> costs
	historyMu   sync.RWMutex

	logger *slog.Logger
}

// NewBudgetEnforcer creates a new budget enforcer
func NewBudgetEnforcer(policy *BudgetPolicy, logger *slog.Logger) *BudgetEnforcer {
	if logger == nil {
		logger = slog.Default()
	}

	if policy == nil {
		policy = NewBudgetPolicy()
	}

	return &BudgetEnforcer{
		policy:         policy,
		sessionBudgets: make(map[string]float64),
		sessionInitial: make(map[string]float64),
		userDailySpend: make(map[string]float64),
		warningsSent:   make(map[string]map[float64]bool),
		costHistory:    make(map[string][]float64),
		logger:         logger,
	}
}

// InitializeSession sets initial budget for a session
func (be *BudgetEnforcer) InitializeSession(sessionID string, initialBudget float64) {
	be.sessionMu.Lock()
	defer be.sessionMu.Unlock()

	be.sessionBudgets[sessionID] = initialBudget
	be.sessionInitial[sessionID] = initialBudget
	be.warningsSent[sessionID] = make(map[float64]bool)

	be.logger.Debug("budget initialized",
		slog.String("session_id", sessionID),
		slog.Float64("budget", initialBudget),
	)
}

// EstimateCost estimates the cost of using a model
func (be *BudgetEnforcer) EstimateCost(ctx context.Context, model string, inputTokens int, outputTokens int) *CostEstimate {
	meta := GetEstimateForModel(model)

	// Use provided tokens or typical values
	actualInput := inputTokens
	if actualInput == 0 {
		actualInput = meta.TypicalInputTokens
	}

	actualOutput := outputTokens
	if actualOutput == 0 {
		actualOutput = meta.TypicalOutputTokens
	}

	// Calculate base cost
	// Most models price input and output the same, but some (like GPT-4) charge differently
	baseCost := (float64(actualInput) / 1_000_000) * meta.PricePerMToken
	if meta.PricePerOutputToken > 0 {
		// Some models have separate output pricing (per 1M tokens)
		baseCost += (float64(actualOutput) / 1_000_000) * meta.PricePerOutputToken
	} else {
		baseCost += (float64(actualOutput) / 1_000_000) * meta.PricePerMToken
	}

	// Add variance buffer
	estimatedCost := baseCost * (1 + be.policy.CostEstimateVariance)

	// Determine estimation confidence
	confidence := 0.7 // Historical data would improve this
	if inputTokens > 0 && outputTokens > 0 {
		confidence = 0.95 // Specific token counts = high confidence
	}

	return &CostEstimate{
		Model:            model,
		EstimatedTokens:  actualInput + actualOutput,
		EstimatedCost:    estimatedCost,
		ConfidenceLevel:  confidence,
		EstimationMethod: "metadata",
	}
}

// CanUseModel checks if a model can be used under current budget constraints
func (be *BudgetEnforcer) CanUseModel(
	ctx context.Context,
	sessionID string,
	userID string,
	model string,
	estimatedCost float64,
) *BudgetDecision {

	decision := &BudgetDecision{
		Status:          Approved,
		EstimatedCost:   estimatedCost,
		RemainingBudget: estimatedCost,
	}

	// Check session budget
	be.sessionMu.RLock()
	sessionBudget, sessionExists := be.sessionBudgets[sessionID]
	be.sessionMu.RUnlock()

	if !sessionExists {
		decision.Status = Denied
		decision.DenialReason = "session not initialized"
		return decision
	}

	// Check if enough budget for this model
	if sessionBudget < estimatedCost {
		decision.Status = Denied
		decision.DenialReason = fmt.Sprintf("insufficient session budget: %.4f < %.4f",
			sessionBudget, estimatedCost)

		// Find fallback
		if len(be.policy.FallbackModels) > 0 {
			fallbackModel := be.policy.FallbackModels[0]
			fallbackEstimate := be.EstimateCost(ctx, fallbackModel, 0, 0)
			if sessionBudget >= fallbackEstimate.EstimatedCost {
				decision.FallbackModel = fallbackModel
				decision.FallbackCost = fallbackEstimate.EstimatedCost
				decision.DenialReason += fmt.Sprintf("; fallback available: %s (%.4f)",
					fallbackModel, fallbackEstimate.EstimatedCost)
			}
		}

		return decision
	}

	// Check per-model limit
	if limit, ok := be.policy.PerModelLimits[model]; ok {
		if estimatedCost > limit {
			decision.Status = Denied
			decision.DenialReason = fmt.Sprintf("per-model limit exceeded: %.4f > %.4f",
				estimatedCost, limit)
			return decision
		}
	}

	// Check per-user daily limit
	be.userMu.RLock()
	userSpend := be.userDailySpend[userID]
	lastReset := be.policy.LastResetTime[userID]
	be.userMu.RUnlock()

	// Reset daily limit if needed
	if time.Since(lastReset) > 24*time.Hour {
		be.userMu.Lock()
		be.userDailySpend[userID] = 0
		be.policy.LastResetTime[userID] = time.Now()
		be.userMu.Unlock()
		userSpend = 0
	}

	if userSpend+estimatedCost > be.policy.PerUserLimit {
		decision.Status = Denied
		decision.DenialReason = fmt.Sprintf("per-user daily limit exceeded: %.4f + %.4f > %.4f",
			userSpend, estimatedCost, be.policy.PerUserLimit)
		return decision
	}

	// Check warning thresholds
	remainingAfter := sessionBudget - estimatedCost

	// Get initial budget for percentage calculation
	be.sessionMu.RLock()
	initialBudget := be.sessionInitial[sessionID]
	be.sessionMu.RUnlock()

	// Calculate percentage spent based on initial budget
	spentTotal := initialBudget - remainingAfter
	var percentSpent float64
	if initialBudget > 0 {
		percentSpent = (spentTotal / initialBudget) * 100
	}

	be.warningMu.RLock()
	warningsSent := be.warningsSent[sessionID]
	be.warningMu.RUnlock()

	for _, threshold := range be.policy.WarnThresholds {
		if percentSpent >= threshold && !warningsSent[threshold] {
			if decision.Status == Approved {
				decision.Status = ApprovedWarn
			}
			decision.WarningThreshold = threshold
			decision.WarningMessage = fmt.Sprintf(
				"Budget usage at %.0f%%: %.4f / %.4f remaining",
				threshold, remainingAfter, initialBudget)

			// Mark warning as sent
			be.warningMu.Lock()
			be.warningsSent[sessionID][threshold] = true
			be.warningMu.Unlock()

			break // Only one warning at a time
		}
	}

	decision.RemainingBudget = remainingAfter

	be.logger.Info("budget decision",
		slog.String("session_id", sessionID),
		slog.String("user_id", userID),
		slog.String("model", model),
		slog.String("status", string(decision.Status)),
		slog.Float64("cost", estimatedCost),
		slog.Float64("remaining", remainingAfter),
	)

	return decision
}

// DeductCost subtracts cost from session and user budgets
func (be *BudgetEnforcer) DeductCost(ctx context.Context, sessionID string, userID string, actualCost float64) error {
	be.sessionMu.Lock()
	if budget, exists := be.sessionBudgets[sessionID]; exists {
		be.sessionBudgets[sessionID] = budget - actualCost
	}
	be.sessionMu.Unlock()

	be.userMu.Lock()
	be.userDailySpend[userID] += actualCost
	be.userMu.Unlock()

	return nil
}

// RecordModelCost records actual cost for a model (for better future estimates)
func (be *BudgetEnforcer) RecordModelCost(model string, actualCost float64) {
	be.historyMu.Lock()
	defer be.historyMu.Unlock()

	be.costHistory[model] = append(be.costHistory[model], actualCost)

	// Keep only last 100 costs per model (for memory efficiency)
	if len(be.costHistory[model]) > 100 {
		be.costHistory[model] = be.costHistory[model][len(be.costHistory[model])-100:]
	}
}

// GetSessionBudget returns remaining budget for a session
func (be *BudgetEnforcer) GetSessionBudget(sessionID string) float64 {
	be.sessionMu.RLock()
	defer be.sessionMu.RUnlock()
	return be.sessionBudgets[sessionID]
}

// GetUserDailySpend returns total spend for user today
func (be *BudgetEnforcer) GetUserDailySpend(userID string) float64 {
	be.userMu.RLock()
	defer be.userMu.RUnlock()
	return be.userDailySpend[userID]
}

// GetUserDailyRemaining returns remaining user daily budget
func (be *BudgetEnforcer) GetUserDailyRemaining(userID string) float64 {
	spend := be.GetUserDailySpend(userID)
	return be.policy.PerUserLimit - spend
}

// RefundCost refunds a deducted cost (for failed operations)
func (be *BudgetEnforcer) RefundCost(sessionID string, userID string, amount float64) {
	be.sessionMu.Lock()
	if budget, exists := be.sessionBudgets[sessionID]; exists {
		be.sessionBudgets[sessionID] = budget + amount
	}
	be.sessionMu.Unlock()

	be.userMu.Lock()
	be.userDailySpend[userID] -= amount
	be.userMu.Unlock()

	be.logger.Debug("cost refunded",
		slog.String("session_id", sessionID),
		slog.String("user_id", userID),
		slog.Float64("amount", amount),
	)
}

// CloseSession cleans up session budget tracking
func (be *BudgetEnforcer) CloseSession(sessionID string) {
	be.sessionMu.Lock()
	delete(be.sessionBudgets, sessionID)
	delete(be.sessionInitial, sessionID)
	be.sessionMu.Unlock()

	be.warningMu.Lock()
	delete(be.warningsSent, sessionID)
	be.warningMu.Unlock()
}

// GetStats returns budget enforcement statistics
func (be *BudgetEnforcer) GetStats() map[string]interface{} {
	be.sessionMu.RLock()
	sessionCount := len(be.sessionBudgets)
	totalSessionBudget := 0.0
	for _, b := range be.sessionBudgets {
		totalSessionBudget += b
	}
	be.sessionMu.RUnlock()

	be.userMu.RLock()
	userCount := len(be.userDailySpend)
	totalDailySpend := 0.0
	for _, s := range be.userDailySpend {
		totalDailySpend += s
	}
	be.userMu.RUnlock()

	be.historyMu.RLock()
	historySize := 0
	for _, costs := range be.costHistory {
		historySize += len(costs)
	}
	be.historyMu.RUnlock()

	return map[string]interface{}{
		"active_sessions":      sessionCount,
		"total_session_budget": totalSessionBudget,
		"active_users":         userCount,
		"total_daily_spend":    totalDailySpend,
		"daily_budget_limit":   be.policy.PerUserLimit,
		"cost_history_samples": historySize,
		"per_user_limit":       be.policy.PerUserLimit,
		"warn_thresholds":      be.policy.WarnThresholds,
	}
}
