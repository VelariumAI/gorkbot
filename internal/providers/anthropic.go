package providers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/internal/config"
)

// AnthropicProvider implements AIProvider for Anthropic Claude models
type AnthropicProvider struct {
	name       string
	config     *config.ProviderConfig
	logger     *slog.Logger
	client     *anthropicClient
	cache      *providerCache
	mu         sync.RWMutex
	lastHealth time.Time
	healthy    bool
}

// anthropicClient wraps HTTP calls to Anthropic API
type anthropicClient struct {
	apiKey  string
	baseURL string
	timeout time.Duration
}

// providerCache caches provider-specific data
type providerCache struct {
	capabilities    *ProviderCapabilities
	lastHealthCheck time.Time
	mu              sync.RWMutex
}

// NewAnthropicProvider creates a new Anthropic provider instance
func NewAnthropicProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if err := config.ValidateProviderConfig("anthropic", cfg); err != nil {
		return nil, fmt.Errorf("invalid anthropic config: %w", err)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	ap := &AnthropicProvider{
		name:   "anthropic",
		config: cfg,
		logger: logger,
		client: &anthropicClient{
			apiKey:  cfg.APIKey,
			baseURL: baseURL,
			timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		cache: &providerCache{
			capabilities: buildAnthropicCapabilities(cfg),
		},
		healthy: true,
	}

	return ap, nil
}

// Execute performs synchronous execution
func (ap *AnthropicProvider) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	return nil, errExecutionDisabled(ap.name)
}

// Stream performs streaming execution
func (ap *AnthropicProvider) Stream(ctx context.Context, req *ExecuteRequest) (<-chan *StreamChunk, error) {
	return nil, errExecutionDisabled(ap.name)
}

// HealthCheck verifies provider is operational
func (ap *AnthropicProvider) HealthCheck(ctx context.Context) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	// Check if we need to do a health check (cache for 5 minutes)
	if time.Since(ap.lastHealth) < 5*time.Minute {
		if ap.healthy {
			return nil
		}
		return fmt.Errorf("provider unhealthy (cached)")
	}

	// Perform actual health check
	// For now, stub: just check API key is configured
	if !hasConfiguredAPIKey(ap.client.apiKey) {
		ap.healthy = false
		ap.lastHealth = time.Now()
		return fmt.Errorf("anthropic: API key not configured")
	}

	ap.healthy = true
	ap.lastHealth = time.Now()
	ap.logger.Debug("anthropic health check passed")
	return nil
}

// Capabilities returns what this provider can do
func (ap *AnthropicProvider) Capabilities() *ProviderCapabilities {
	ap.cache.mu.RLock()
	defer ap.cache.mu.RUnlock()
	return ap.cache.capabilities
}

// EstimateCost estimates cost for a request
func (ap *AnthropicProvider) EstimateCost(req *ExecuteRequest) float64 {
	if req == nil {
		return 0
	}

	inputTokens := estimateTokens(concatenateMessages(req.Messages))
	outputTokens := req.MaxTokens
	if outputTokens == 0 {
		outputTokens = 2048
	}

	inputCost := (float64(inputTokens) / 1e6) * ap.config.CostPer1MInput
	outputCost := (float64(outputTokens) / 1e6) * ap.config.CostPer1MOutput

	return inputCost + outputCost
}

// GetRateLimit returns rate limiting information
func (ap *AnthropicProvider) GetRateLimit() *RateLimit {
	return &RateLimit{
		RequestsPerMinute: 60,
		RequestsPerDay:    10000,
		TokensPerMinute:   1000000,
		TokensPerDay:      100000000,
	}
}

// Name returns provider name
func (ap *AnthropicProvider) Name() string {
	return ap.name
}

// Close closes the provider (cleanup)
func (ap *AnthropicProvider) Close() error {
	// No resources to close for Anthropic
	ap.logger.Debug("anthropic provider closed")
	return nil
}

// Helper functions

func buildAnthropicCapabilities(cfg *config.ProviderConfig) *ProviderCapabilities {
	return &ProviderCapabilities{
		SupportsThinking:      cfg.Thinking.Enabled,
		ThinkingBudget:        cfg.Thinking.Budget,
		SupportsVision:        cfg.Vision.Enabled,
		SupportsPromptCaching: cfg.Caching.Enabled,
		SupportsNativeTools:   cfg.NativeTools.Enabled,
		SupportsStreaming:     true,
		MaxContextWindow:      200000, // Claude 3.5 Sonnet
		CostPer1MInputTokens:  cfg.CostPer1MInput,
		CostPer1MOutputTokens: cfg.CostPer1MOutput,
		RequestsPerMinute:     60,
		DefaultTemperature:    0.7,
		AverageLatencyMs:      500,
	}
}

func (ap *AnthropicProvider) buildRequest(model string, req *ExecuteRequest) map[string]interface{} {
	request := map[string]interface{}{
		"model":       model,
		"max_tokens":  req.MaxTokens,
		"temperature": req.Temperature,
	}

	// Add system prompt if provided
	if req.SystemPrompt != "" {
		request["system"] = req.SystemPrompt
	}

	// Add messages
	if len(req.Messages) > 0 {
		request["messages"] = req.Messages
	}

	// Add thinking if enabled and budget > 0
	if req.ThinkingBudget > 0 && ap.config.Thinking.Enabled {
		request["thinking"] = map[string]interface{}{
			"type":   "enabled",
			"budget": req.ThinkingBudget,
		}
	}

	// Add tools if provided
	if len(req.Tools) > 0 {
		request["tools"] = req.Tools
	}

	return request
}

func estimateTokens(text string) int {
	// Rough heuristic: ~4 characters per token
	return len(text) / 4
}

func concatenateMessages(messages []*Message) string {
	result := ""
	for _, msg := range messages {
		result += msg.Content + " "
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Response is a temporary struct for building responses
type Response struct {
	Content      string
	Thinking     string
	Model        string
	Provider     string
	InputTokens  int
	OutputTokens int
	Cost         float64
	StartTime    int64
	EndTime      int64
}

// Convert to ExecuteResponse
func (r *Response) toExecuteResponse() *ExecuteResponse {
	return &ExecuteResponse{
		Content:      r.Content,
		Thinking:     r.Thinking,
		Model:        r.Model,
		Provider:     r.Provider,
		InputTokens:  r.InputTokens,
		OutputTokens: r.OutputTokens,
		Cost:         r.Cost,
		StartTime:    r.StartTime,
		EndTime:      r.EndTime,
		Duration:     r.EndTime - r.StartTime,
	}
}
