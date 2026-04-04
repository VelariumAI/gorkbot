package routing

import (
	"fmt"
	"log/slog"
	"sort"
)

// CostProfile describes the cost of using a provider
type CostProfile struct {
	Provider         string
	Model            string
	CostPer1MInput   float64
	CostPer1MOutput  float64
	EstimatedInputTokens  int
	EstimatedOutputTokens int
	EstimatedCost    float64
}

// CapabilityScore describes how well a provider matches a task
type CapabilityScore struct {
	Provider      string
	Model         string
	ReasoningScore float64
	CodingScore    float64
	VisionScore    float64
	CreativityScore float64
	OverallScore  float64
}

// CostAwareRouter routes requests to optimal provider based on cost and capability
type CostAwareRouter struct {
	logger      *slog.Logger
	costProfiles map[string]*CostProfile
	capabilities map[string]*CapabilityScore
	budgetLimit  float64
	preferCheap  bool
}

// NewCostAwareRouter creates a cost-aware router
func NewCostAwareRouter(logger *slog.Logger, budgetLimit float64) *CostAwareRouter {
	if logger == nil {
		logger = slog.Default()
	}

	return &CostAwareRouter{
		logger:       logger,
		costProfiles: make(map[string]*CostProfile),
		capabilities: make(map[string]*CapabilityScore),
		budgetLimit:  budgetLimit,
		preferCheap:  true,
	}
}

// RegisterCostProfile registers cost information for a provider
func (car *CostAwareRouter) RegisterCostProfile(profile *CostProfile) {
	car.costProfiles[profile.Provider+":"+profile.Model] = profile

	car.logger.Debug("registered cost profile",
		slog.String("provider", profile.Provider),
		slog.String("model", profile.Model),
		slog.Float64("input_cost_per_1m", profile.CostPer1MInput),
	)
}

// RegisterCapabilityScore registers capability information
func (car *CostAwareRouter) RegisterCapabilityScore(score *CapabilityScore) {
	car.capabilities[score.Provider+":"+score.Model] = score

	car.logger.Debug("registered capability score",
		slog.String("provider", score.Provider),
		slog.String("model", score.Model),
		slog.Float64("overall", score.OverallScore),
	)
}

// SelectProvider selects the best provider given constraints
func (car *CostAwareRouter) SelectProvider(
	taskType string,
	availableProviders []string,
	budgetRemaining float64,
) (string, string, error) {

	if len(availableProviders) == 0 {
		return "", "", fmt.Errorf("no providers available")
	}

	// Filter providers by budget
	affordable := make([]string, 0)
	for _, provider := range availableProviders {
		cost := car.estimateProviderCost(provider)
		if cost <= budgetRemaining {
			affordable = append(affordable, provider)
		}
	}

	if len(affordable) == 0 {
		car.logger.Warn("no providers within budget",
			slog.Float64("budget", budgetRemaining),
		)
		return "", "", fmt.Errorf("insufficient budget for any provider")
	}

	// Score by task type + cost
	var candidates []*ProviderCandidate
	for _, provider := range affordable {
		candidate := &ProviderCandidate{
			Provider: provider,
			Cost:     car.estimateProviderCost(provider),
		}

		// Score by capability match
		capability := car.getCapabilityScore(provider, taskType)
		candidate.CapabilityScore = capability

		// Combined score: capability 70%, cost-efficiency 30%
		cost := candidate.Cost
		if cost == 0 {
			cost = 0.001
		}
		costEfficiency := 1.0 / cost
		candidate.FinalScore = (capability * 0.7) + (costEfficiency * 0.3)

		candidates = append(candidates, candidate)
	}

	// Sort by final score (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].FinalScore > candidates[j].FinalScore
	})

	selected := candidates[0]

	car.logger.Info("selected provider",
		slog.String("provider", selected.Provider),
		slog.String("task", taskType),
		slog.Float64("score", selected.FinalScore),
		slog.Float64("cost", selected.Cost),
	)

	parts := parseProviderModel(selected.Provider)
	return parts[0], parts[1], nil
}

// PreferCheapWhenBudgeted prefers cheaper models when budget allows
func (car *CostAwareRouter) PreferCheapWhenBudgeted(budgetRemaining float64) bool {
	if budgetRemaining > 10.0 {
		return false // Budget is plentiful, prefer best quality
	}
	return true // Budget is tight, prefer cheap
}

// GetCheapestProvider returns the cheapest provider for a given task
func (car *CostAwareRouter) GetCheapestProvider(availableProviders []string) (string, string, float64) {
	cheapest := ""
	cheapestCost := float64(^uint(0) >> 1) // Max float

	for _, provider := range availableProviders {
		cost := car.estimateProviderCost(provider)
		if cost < cheapestCost {
			cheapestCost = cost
			cheapest = provider
		}
	}

	parts := parseProviderModel(cheapest)
	return parts[0], parts[1], cheapestCost
}

// EstimateCost estimates the cost of a request
func (car *CostAwareRouter) EstimateCost(provider string, inputTokens int, outputTokens int) float64 {
	key := provider
	profile, ok := car.costProfiles[key]
	if !ok {
		return 0.0
	}

	inputCost := float64(inputTokens) / 1e6 * profile.CostPer1MInput
	outputCost := float64(outputTokens) / 1e6 * profile.CostPer1MOutput

	return inputCost + outputCost
}

// Helper methods

func (car *CostAwareRouter) estimateProviderCost(provider string) float64 {
	profile, ok := car.costProfiles[provider]
	if !ok {
		return 0.1 // Default estimate
	}
	return profile.EstimatedCost
}

func (car *CostAwareRouter) getCapabilityScore(provider string, taskType string) float64 {
	capability, ok := car.capabilities[provider]
	if !ok {
		return 0.5 // Default
	}

	// Score based on task type
	switch taskType {
	case "reasoning":
		return capability.ReasoningScore
	case "coding":
		return capability.CodingScore
	case "vision":
		return capability.VisionScore
	case "creative":
		return capability.CreativityScore
	default:
		return capability.OverallScore
	}
}

// ProviderCandidate represents a provider being evaluated
type ProviderCandidate struct {
	Provider        string
	Cost            float64
	CapabilityScore float64
	FinalScore      float64
}

// parseProviderModel parses "provider:model" string
func parseProviderModel(s string) []string {
	// Simple parsing - in reality would be more robust
	parts := make([]string, 2)
	if len(s) > 0 {
		parts[0] = "provider"
		parts[1] = s
	}
	return parts
}

// BudgetManager manages per-user budgets
type BudgetManager struct {
	logger  *slog.Logger
	budgets map[string]float64
}

// NewBudgetManager creates a budget manager
func NewBudgetManager(logger *slog.Logger) *BudgetManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &BudgetManager{
		logger:  logger,
		budgets: make(map[string]float64),
	}
}

// SetBudget sets user budget
func (bm *BudgetManager) SetBudget(userID string, amount float64) {
	bm.budgets[userID] = amount

	bm.logger.Info("set budget",
		slog.String("user", userID),
		slog.Float64("amount", amount),
	)
}

// Deduct deducts cost from budget
func (bm *BudgetManager) Deduct(userID string, amount float64) error {
	remaining, ok := bm.budgets[userID]
	if !ok {
		return fmt.Errorf("no budget for user: %s", userID)
	}

	if remaining < amount {
		return fmt.Errorf("insufficient budget: need %.2f, have %.2f", amount, remaining)
	}

	bm.budgets[userID] = remaining - amount

	bm.logger.Debug("deducted cost",
		slog.String("user", userID),
		slog.Float64("amount", amount),
		slog.Float64("remaining", bm.budgets[userID]),
	)

	return nil
}

// GetRemaining returns remaining budget
func (bm *BudgetManager) GetRemaining(userID string) float64 {
	return bm.budgets[userID]
}
