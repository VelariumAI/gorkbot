package providers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/velariumai/gorkbot/internal/config"
)

// BedrockProvider implements AIProvider for AWS Bedrock models
type BedrockProvider struct {
	name       string
	config     *config.ProviderConfig
	logger     *slog.Logger
	healthy    bool
	lastHealth time.Time
}

func NewBedrockProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if err := config.ValidateProviderConfig("bedrock", cfg); err != nil {
		return nil, fmt.Errorf("invalid bedrock config: %w", err)
	}

	return &BedrockProvider{
		name:    "bedrock",
		config:  cfg,
		logger:  logger,
		healthy: true,
	}, nil
}

func (bp *BedrockProvider) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	return nil, errExecutionDisabled(bp.name)
}

func (bp *BedrockProvider) Stream(ctx context.Context, req *ExecuteRequest) (<-chan *StreamChunk, error) {
	return nil, errExecutionDisabled(bp.name)
}

func (bp *BedrockProvider) HealthCheck(ctx context.Context) error {
	if time.Since(bp.lastHealth) < 5*time.Minute && !bp.healthy {
		return fmt.Errorf("bedrock: provider unhealthy")
	}

	if !hasConfiguredAPIKey(bp.config.APIKey) {
		bp.healthy = false
		bp.lastHealth = time.Now()
		return fmt.Errorf("bedrock: AWS credentials not configured")
	}

	bp.healthy = true
	bp.lastHealth = time.Now()
	bp.logger.Debug("bedrock health check passed")
	return nil
}

func (bp *BedrockProvider) Capabilities() *ProviderCapabilities {
	return &ProviderCapabilities{
		SupportsThinking:      bp.config.Thinking.Enabled,
		ThinkingBudget:        bp.config.Thinking.Budget,
		SupportsVision:        bp.config.Vision.Enabled,
		SupportsPromptCaching: bp.config.Caching.Enabled,
		SupportsNativeTools:   bp.config.NativeTools.Enabled,
		SupportsStreaming:     true,
		MaxContextWindow:      200000,
		CostPer1MInputTokens:  bp.config.CostPer1MInput,
		CostPer1MOutputTokens: bp.config.CostPer1MOutput,
		RequestsPerMinute:     50,
		DefaultTemperature:    0.7,
		AverageLatencyMs:      600,
	}
}

func (bp *BedrockProvider) EstimateCost(req *ExecuteRequest) float64 {
	if req == nil {
		return 0
	}

	inputTokens := estimateTokens(concatenateMessages(req.Messages))
	outputTokens := req.MaxTokens
	if outputTokens == 0 {
		outputTokens = 2048
	}

	return (float64(inputTokens)/1e6)*bp.config.CostPer1MInput +
		(float64(outputTokens)/1e6)*bp.config.CostPer1MOutput
}

func (bp *BedrockProvider) GetRateLimit() *RateLimit {
	return &RateLimit{
		RequestsPerMinute: 50,
		TokensPerMinute:   500000,
	}
}

func (bp *BedrockProvider) Name() string {
	return bp.name
}

func (bp *BedrockProvider) Close() error {
	bp.logger.Debug("bedrock provider closed")
	return nil
}
