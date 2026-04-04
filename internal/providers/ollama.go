package providers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/velariumai/gorkbot/internal/config"
)

// OllamaProvider implements AIProvider for local Ollama models
type OllamaProvider struct {
	name       string
	config     *config.ProviderConfig
	logger     *slog.Logger
	healthy    bool
	lastHealth time.Time
}

func NewOllamaProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if err := config.ValidateProviderConfig("ollama", cfg); err != nil {
		return nil, fmt.Errorf("invalid ollama config: %w", err)
	}

	return &OllamaProvider{
		name:    "ollama",
		config:  cfg,
		logger:  logger,
		healthy: true,
	}, nil
}

func (op *OllamaProvider) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	return nil, errExecutionDisabled(op.name)
}

func (op *OllamaProvider) Stream(ctx context.Context, req *ExecuteRequest) (<-chan *StreamChunk, error) {
	return nil, errExecutionDisabled(op.name)
}

func (op *OllamaProvider) HealthCheck(ctx context.Context) error {
	if time.Since(op.lastHealth) < 5*time.Minute && !op.healthy {
		return fmt.Errorf("ollama: server unreachable")
	}

	// For Ollama, we check if the base URL is accessible
	if op.config.BaseURL == "" {
		op.healthy = false
		op.lastHealth = time.Now()
		return fmt.Errorf("ollama: base URL not configured (default: http://localhost:11434)")
	}

	// In a real implementation, we'd do a test request to verify connectivity
	op.healthy = true
	op.lastHealth = time.Now()
	op.logger.Debug("ollama health check passed", slog.String("url", op.config.BaseURL))
	return nil
}

func (op *OllamaProvider) Capabilities() *ProviderCapabilities {
	return &ProviderCapabilities{
		SupportsThinking:      false,
		SupportsVision:        op.config.Vision.Enabled,
		SupportsPromptCaching: false,
		SupportsNativeTools:   op.config.NativeTools.Enabled,
		SupportsStreaming:     true,
		MaxContextWindow:      8000, // Depends on model
		CostPer1MInputTokens:  0.0,  // Local
		CostPer1MOutputTokens: 0.0,  // Local
		RequestsPerMinute:     30,   // Limited by local hardware
		DefaultTemperature:    0.7,
		AverageLatencyMs:      1000, // Slower, local hardware
	}
}

func (op *OllamaProvider) EstimateCost(req *ExecuteRequest) float64 {
	// Ollama is free (local execution)
	return 0.0
}

func (op *OllamaProvider) GetRateLimit() *RateLimit {
	return &RateLimit{
		RequestsPerMinute: 30,
		TokensPerMinute:   100000,
	}
}

func (op *OllamaProvider) Name() string {
	return op.name
}

func (op *OllamaProvider) Close() error {
	op.logger.Debug("ollama provider closed")
	return nil
}
