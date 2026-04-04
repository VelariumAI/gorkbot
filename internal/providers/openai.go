package providers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/velariumai/gorkbot/internal/config"
)

// OpenAIProvider implements AIProvider for OpenAI GPT models
type OpenAIProvider struct {
	name       string
	config     *config.ProviderConfig
	logger     *slog.Logger
	healthy    bool
	lastHealth time.Time
}

func NewOpenAIProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if err := config.ValidateProviderConfig("openai", cfg); err != nil {
		return nil, fmt.Errorf("invalid openai config: %w", err)
	}

	return &OpenAIProvider{
		name:    "openai",
		config:  cfg,
		logger:  logger,
		healthy: true,
	}, nil
}

func (op *OpenAIProvider) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	return nil, errExecutionDisabled(op.name)
}

func (op *OpenAIProvider) Stream(ctx context.Context, req *ExecuteRequest) (<-chan *StreamChunk, error) {
	return nil, errExecutionDisabled(op.name)
}

func (op *OpenAIProvider) HealthCheck(ctx context.Context) error {
	if time.Since(op.lastHealth) < 5*time.Minute && !op.healthy {
		return fmt.Errorf("openai: provider unhealthy")
	}

	if !hasConfiguredAPIKey(op.config.APIKey) {
		op.healthy = false
		op.lastHealth = time.Now()
		return fmt.Errorf("openai: API key not configured")
	}

	op.healthy = true
	op.lastHealth = time.Now()
	op.logger.Debug("openai health check passed")
	return nil
}

func (op *OpenAIProvider) Capabilities() *ProviderCapabilities {
	return &ProviderCapabilities{
		SupportsThinking:      false, // GPT-5 doesn't use thinking like Claude
		SupportsVision:        op.config.Vision.Enabled,
		SupportsPromptCaching: op.config.Caching.Enabled,
		SupportsNativeTools:   op.config.NativeTools.Enabled, // Response API
		SupportsStreaming:     true,
		MaxContextWindow:      128000, // GPT-5
		CostPer1MInputTokens:  op.config.CostPer1MInput,
		CostPer1MOutputTokens: op.config.CostPer1MOutput,
		RequestsPerMinute:     100,
		DefaultTemperature:    0.7,
		AverageLatencyMs:      300,
	}
}

func (op *OpenAIProvider) EstimateCost(req *ExecuteRequest) float64 {
	if req == nil {
		return 0
	}

	inputTokens := estimateTokens(concatenateMessages(req.Messages))
	outputTokens := req.MaxTokens
	if outputTokens == 0 {
		outputTokens = 2048
	}

	return (float64(inputTokens)/1e6)*op.config.CostPer1MInput +
		(float64(outputTokens)/1e6)*op.config.CostPer1MOutput
}

func (op *OpenAIProvider) GetRateLimit() *RateLimit {
	return &RateLimit{
		RequestsPerMinute: 100,
		RequestsPerDay:    10000,
		TokensPerMinute:   2000000,
		TokensPerDay:      100000000,
	}
}

func (op *OpenAIProvider) Name() string {
	return op.name
}

func (op *OpenAIProvider) Close() error {
	op.logger.Debug("openai provider closed")
	return nil
}
