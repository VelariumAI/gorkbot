package providers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/velariumai/gorkbot/internal/config"
)

// GoogleProvider implements AIProvider for Google Gemini models
type GoogleProvider struct {
	name       string
	config     *config.ProviderConfig
	logger     *slog.Logger
	healthy    bool
	lastHealth time.Time
}

func NewGoogleProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if err := config.ValidateProviderConfig("google", cfg); err != nil {
		return nil, fmt.Errorf("invalid google config: %w", err)
	}

	return &GoogleProvider{
		name:    "google",
		config:  cfg,
		logger:  logger,
		healthy: true,
	}, nil
}

func (gp *GoogleProvider) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	return nil, errExecutionDisabled(gp.name)
}

func (gp *GoogleProvider) Stream(ctx context.Context, req *ExecuteRequest) (<-chan *StreamChunk, error) {
	return nil, errExecutionDisabled(gp.name)
}

func (gp *GoogleProvider) HealthCheck(ctx context.Context) error {
	if time.Since(gp.lastHealth) < 5*time.Minute && !gp.healthy {
		return fmt.Errorf("google: provider unhealthy")
	}

	if !hasConfiguredAPIKey(gp.config.APIKey) {
		gp.healthy = false
		gp.lastHealth = time.Now()
		return fmt.Errorf("google: API key not configured")
	}

	gp.healthy = true
	gp.lastHealth = time.Now()
	gp.logger.Debug("google health check passed")
	return nil
}

func (gp *GoogleProvider) Capabilities() *ProviderCapabilities {
	return &ProviderCapabilities{
		SupportsThinking:      false,
		SupportsVision:        gp.config.Vision.Enabled,
		SupportsPromptCaching: gp.config.Caching.Enabled,
		SupportsNativeTools:   gp.config.NativeTools.Enabled,
		SupportsStreaming:     true,
		MaxContextWindow:      1000000, // Gemini 2.0 Flash
		CostPer1MInputTokens:  gp.config.CostPer1MInput,
		CostPer1MOutputTokens: gp.config.CostPer1MOutput,
		RequestsPerMinute:     100,
		DefaultTemperature:    0.7,
		AverageLatencyMs:      200,
	}
}

func (gp *GoogleProvider) EstimateCost(req *ExecuteRequest) float64 {
	if req == nil {
		return 0
	}

	inputTokens := estimateTokens(concatenateMessages(req.Messages))
	outputTokens := req.MaxTokens
	if outputTokens == 0 {
		outputTokens = 2048
	}

	return (float64(inputTokens)/1e6)*gp.config.CostPer1MInput +
		(float64(outputTokens)/1e6)*gp.config.CostPer1MOutput
}

func (gp *GoogleProvider) GetRateLimit() *RateLimit {
	return &RateLimit{
		RequestsPerMinute: 100,
		TokensPerMinute:   4000000,
	}
}

func (gp *GoogleProvider) Name() string {
	return gp.name
}

func (gp *GoogleProvider) Close() error {
	gp.logger.Debug("google provider closed")
	return nil
}
