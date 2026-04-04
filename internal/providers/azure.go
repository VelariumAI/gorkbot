package providers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/velariumai/gorkbot/internal/config"
)

// AzureProvider implements AIProvider for Azure OpenAI
type AzureProvider struct {
	name       string
	config     *config.ProviderConfig
	logger     *slog.Logger
	healthy    bool
	lastHealth time.Time
}

func NewAzureProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if err := config.ValidateProviderConfig("azure", cfg); err != nil {
		return nil, fmt.Errorf("invalid azure config: %w", err)
	}

	return &AzureProvider{
		name:    "azure",
		config:  cfg,
		logger:  logger,
		healthy: true,
	}, nil
}

func (ap *AzureProvider) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	return nil, errExecutionDisabled(ap.name)
}

func (ap *AzureProvider) Stream(ctx context.Context, req *ExecuteRequest) (<-chan *StreamChunk, error) {
	return nil, errExecutionDisabled(ap.name)
}

func (ap *AzureProvider) HealthCheck(ctx context.Context) error {
	if time.Since(ap.lastHealth) < 5*time.Minute && !ap.healthy {
		return fmt.Errorf("azure: provider unhealthy")
	}

	if !hasConfiguredAPIKey(ap.config.APIKey) {
		ap.healthy = false
		ap.lastHealth = time.Now()
		return fmt.Errorf("azure: API key not configured")
	}

	ap.healthy = true
	ap.lastHealth = time.Now()
	ap.logger.Debug("azure health check passed")
	return nil
}

func (ap *AzureProvider) Capabilities() *ProviderCapabilities {
	return &ProviderCapabilities{
		SupportsThinking:      false,
		SupportsVision:        ap.config.Vision.Enabled,
		SupportsPromptCaching: ap.config.Caching.Enabled,
		SupportsNativeTools:   ap.config.NativeTools.Enabled,
		SupportsStreaming:     true,
		MaxContextWindow:      128000,
		CostPer1MInputTokens:  ap.config.CostPer1MInput,
		CostPer1MOutputTokens: ap.config.CostPer1MOutput,
		RequestsPerMinute:     100,
		DefaultTemperature:    0.7,
		AverageLatencyMs:      400,
	}
}

func (ap *AzureProvider) EstimateCost(req *ExecuteRequest) float64 {
	if req == nil {
		return 0
	}

	inputTokens := estimateTokens(concatenateMessages(req.Messages))
	outputTokens := req.MaxTokens
	if outputTokens == 0 {
		outputTokens = 2048
	}

	return (float64(inputTokens)/1e6)*ap.config.CostPer1MInput +
		(float64(outputTokens)/1e6)*ap.config.CostPer1MOutput
}

func (ap *AzureProvider) GetRateLimit() *RateLimit {
	return &RateLimit{
		RequestsPerMinute: 100,
		TokensPerMinute:   1000000,
	}
}

func (ap *AzureProvider) Name() string {
	return ap.name
}

func (ap *AzureProvider) Close() error {
	ap.logger.Debug("azure provider closed")
	return nil
}
