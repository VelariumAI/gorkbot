package budget

import "time"

// BudgetPolicy defines cost limits and fallback behavior
type BudgetPolicy struct {
	// Per-model cost limits (USD per session)
	PerModelLimits map[string]float64

	// Per-session total limit (USD)
	PerSessionLimit float64

	// Per-user total limit (USD per day)
	PerUserLimit float64

	// PerUserDailyReset resets per-user limit daily
	PerUserDailyReset bool

	// FallbackModels ordered by preference when budget exhausted
	// e.g., ["grok-3", "haiku", "claude-haiku"]
	FallbackModels []string

	// WarnThresholds trigger warnings at % budget remaining
	// e.g., [75, 90, 100] warns at 75%, 90%, and 100% spent
	WarnThresholds []float64

	// BlockOnExhausted blocks queries if budget exceeded (vs allowing fallback)
	BlockOnExhausted bool

	// CostEstimateVariance expected variance in estimates (for buffer)
	// e.g., 0.1 = expect 10% variance, add 10% buffer to estimates
	CostEstimateVariance float64

	// LastResetTime for per-user limit reset tracking
	LastResetTime map[string]time.Time
}

// NewBudgetPolicy creates a default policy
func NewBudgetPolicy() *BudgetPolicy {
	return &BudgetPolicy{
		PerModelLimits: map[string]float64{
			"grok-3":        100.0,
			"grok-2":        100.0,
			"haiku":         50.0,
			"sonnet":        100.0,
			"claude-haiku":  50.0,
			"gemini-pro":    100.0,
			"gpt-4":         150.0,
		},
		PerSessionLimit:      10.0,  // $10 per session default
		PerUserLimit:         100.0, // $100 per user per day
		PerUserDailyReset:    true,
		FallbackModels:       []string{"haiku", "grok-2"},
		WarnThresholds:       []float64{75, 90, 100}, // Warn at 75%, 90%, 100%
		BlockOnExhausted:     false,                  // Allow fallback by default
		CostEstimateVariance: 0.15,                   // 15% variance buffer
		LastResetTime:        make(map[string]time.Time),
	}
}

// BudgetDecision represents a decision about whether to allow a model
type BudgetDecision struct {
	Status             BudgetStatus // Approved, ApprovedWarn, Denied
	RemainingBudget    float64      // Budget left after this operation
	WarningMessage     string       // Why it's approved with warning
	DenialReason       string       // Why it's denied
	FallbackModel      string       // Alternative model if denied
	FallbackCost       float64      // Cost of fallback
	EstimatedCost      float64      // Estimated cost of requested model
	WarningThreshold   float64      // Which threshold triggered warning (75, 90, 100)
}

// BudgetStatus represents the decision outcome
type BudgetStatus string

const (
	Approved     BudgetStatus = "approved"      // Use requested model
	ApprovedWarn BudgetStatus = "approved_warn" // Use requested but warn user
	Denied       BudgetStatus = "denied"        // Blocked, suggest fallback
)

// CostEstimate represents estimated cost for an operation
type CostEstimate struct {
	Model            string
	EstimatedTokens  int
	EstimatedCost    float64 // USD
	ConfidenceLevel  float64 // 0-1, higher = more accurate
	EstimationMethod string  // "metadata", "historical", "default"
}

// ModelMetadata contains per-model pricing and capabilities
type ModelMetadata struct {
	Name           string
	Provider       string // "xai", "google", "anthropic", "openai"
	PricePerMToken float64
	// PricePerOutputToken: use this for output-specific pricing (some models charge differently)
	PricePerOutputToken float64
	// MaxTokens per request
	MaxTokens int
	// Capabilities for routing decisions
	HasReasoning    bool
	HasVision       bool
	HasFunctionCall bool
	// TypicalInputTokens for cost estimation
	TypicalInputTokens int
	// TypicalOutputTokens for cost estimation
	TypicalOutputTokens int
	// Availability can be false if rate-limited or down
	Available bool
}

// GetEstimateForModel returns standard metadata for a model
func GetEstimateForModel(model string) *ModelMetadata {
	estimates := map[string]*ModelMetadata{
		"grok-3": {
			Name:                 "grok-3",
			Provider:             "xai",
			PricePerMToken:       0.005, // $0.005 per 1M tokens
			PricePerOutputToken:  0.015,
			MaxTokens:            2000000,
			HasReasoning:         true,
			HasVision:            false,
			HasFunctionCall:      true,
			TypicalInputTokens:   2048,
			TypicalOutputTokens:  1024,
			Available:            true,
		},
		"grok-2": {
			Name:                 "grok-2",
			Provider:             "xai",
			PricePerMToken:       0.002,
			PricePerOutputToken:  0.010,
			MaxTokens:            2000000,
			HasReasoning:         true,
			HasVision:            false,
			HasFunctionCall:      true,
			TypicalInputTokens:   2048,
			TypicalOutputTokens:  1024,
			Available:            true,
		},
		"haiku": {
			Name:                 "haiku",
			Provider:             "anthropic",
			PricePerMToken:       0.0008,
			PricePerOutputToken:  0.004,
			MaxTokens:            200000,
			HasReasoning:         false,
			HasVision:            true,
			HasFunctionCall:      true,
			TypicalInputTokens:   1024,
			TypicalOutputTokens:  512,
			Available:            true,
		},
		"sonnet": {
			Name:                 "sonnet",
			Provider:             "anthropic",
			PricePerMToken:       0.003,
			PricePerOutputToken:  0.015,
			MaxTokens:            200000,
			HasReasoning:         true,
			HasVision:            true,
			HasFunctionCall:      true,
			TypicalInputTokens:   2048,
			TypicalOutputTokens:  1024,
			Available:            true,
		},
		"claude-haiku": {
			Name:                 "claude-haiku",
			Provider:             "anthropic",
			PricePerMToken:       0.0008,
			PricePerOutputToken:  0.004,
			MaxTokens:            200000,
			HasReasoning:         false,
			HasVision:            true,
			HasFunctionCall:      true,
			TypicalInputTokens:   1024,
			TypicalOutputTokens:  512,
			Available:            true,
		},
		"gemini-pro": {
			Name:                 "gemini-pro",
			Provider:             "google",
			PricePerMToken:       0.0005,
			PricePerOutputToken:  0.0015,
			MaxTokens:            2000000,
			HasReasoning:         true,
			HasVision:            true,
			HasFunctionCall:      true,
			TypicalInputTokens:   2048,
			TypicalOutputTokens:  1024,
			Available:            true,
		},
		"gpt-4": {
			Name:                 "gpt-4",
			Provider:             "openai",
			PricePerMToken:       0.03,
			PricePerOutputToken:  0.06,
			MaxTokens:            8192,
			HasReasoning:         true,
			HasVision:            true,
			HasFunctionCall:      true,
			TypicalInputTokens:   2048,
			TypicalOutputTokens:  1024,
			Available:            true,
		},
	}

	if meta, ok := estimates[model]; ok {
		return meta
	}

	// Default fallback for unknown models
	return &ModelMetadata{
		Name:                model,
		Provider:            "unknown",
		PricePerMToken:      0.001,
		PricePerOutputToken: 0.005,
		MaxTokens:           100000,
		HasReasoning:        false,
		HasVision:           false,
		HasFunctionCall:     false,
		TypicalInputTokens:  1024,
		TypicalOutputTokens: 512,
		Available:           true,
	}
}
