package ai

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/registry"
)

// TokenBucket implements a token bucket rate limiter.
// Tokens are replenished at a fixed rate (tokens/second).
// Each Wait(ctx) call consumes one token, blocking until available or context cancels.
type TokenBucket struct {
	capacity float64       // max tokens
	tokens   float64       // current tokens
	rate     float64       // tokens/second
	lastFill time.Time
	mu       sync.Mutex
}

// NewTokenBucket creates a new token bucket with a capacity and refill rate.
// rpm is the requests-per-minute limit; it's converted to tokens/second internally.
func NewTokenBucket(rpm float64) *TokenBucket {
	tokensPerSecond := rpm / 60.0
	return &TokenBucket{
		capacity: rpm,      // at least 1 minute worth of capacity
		tokens:   rpm,      // start full
		rate:     tokensPerSecond,
		lastFill: time.Now(),
	}
}

// Wait blocks until one token is available, then consumes it.
// If the context is cancelled before a token becomes available, returns the context error.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	for {
		tb.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(tb.lastFill).Seconds()
		tb.tokens += elapsed * tb.rate
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastFill = now

		if tb.tokens >= 1.0 {
			tb.tokens -= 1.0
			tb.mu.Unlock()
			return nil
		}

		// Calculate how long to wait for the next token
		waitTime := time.Duration((1.0 - tb.tokens) / tb.rate * float64(time.Second))
		tb.mu.Unlock()

		// Wait and then retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Try again
		}
	}
}

// RateLimitedProvider wraps an AIProvider with token bucket rate limiting.
type RateLimitedProvider struct {
	inner  AIProvider
	bucket *TokenBucket
}

// NewRateLimitedProvider creates a rate-limited wrapper around an AIProvider.
// rpm is the requests-per-minute limit (use defaultRPM[providerID] for defaults).
func NewRateLimitedProvider(inner AIProvider, rpm float64) *RateLimitedProvider {
	return &RateLimitedProvider{
		inner:  inner,
		bucket: NewTokenBucket(rpm),
	}
}

// DefaultRPM provides conservative per-provider rate limits.
var DefaultRPM = map[string]float64{
	"grok":       60,
	"gemini":     60,
	"anthropic":  50,
	"minimax":    30,
	"moonshot":   20,
	"openrouter": 10,
	"openai":     60,
}

// Generate implements AIProvider, with rate limiting.
func (rp *RateLimitedProvider) Generate(ctx context.Context, prompt string) (string, error) {
	if err := rp.bucket.Wait(ctx); err != nil {
		return "", err
	}
	return rp.inner.Generate(ctx, prompt)
}

// GenerateWithHistory implements AIProvider, with rate limiting.
func (rp *RateLimitedProvider) GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error) {
	if err := rp.bucket.Wait(ctx); err != nil {
		return "", err
	}
	return rp.inner.GenerateWithHistory(ctx, history)
}

// Stream implements AIProvider, with rate limiting.
func (rp *RateLimitedProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	if err := rp.bucket.Wait(ctx); err != nil {
		return err
	}
	return rp.inner.Stream(ctx, prompt, out)
}

// StreamWithHistory implements AIProvider, with rate limiting.
func (rp *RateLimitedProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
	if err := rp.bucket.Wait(ctx); err != nil {
		return err
	}
	return rp.inner.StreamWithHistory(ctx, history, out)
}

// GetMetadata implements AIProvider.
func (rp *RateLimitedProvider) GetMetadata() ProviderMetadata {
	return rp.inner.GetMetadata()
}

// Name implements AIProvider.
func (rp *RateLimitedProvider) Name() string {
	return rp.inner.Name()
}

// ID implements AIProvider.
func (rp *RateLimitedProvider) ID() registry.ProviderID {
	return rp.inner.ID()
}

// Ping implements AIProvider.
func (rp *RateLimitedProvider) Ping(ctx context.Context) error {
	return rp.inner.Ping(ctx)
}

// FetchModels implements AIProvider.
func (rp *RateLimitedProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return rp.inner.FetchModels(ctx)
}

// WithModel implements AIProvider.
func (rp *RateLimitedProvider) WithModel(model string) AIProvider {
	return NewRateLimitedProvider(rp.inner.WithModel(model), 60.0)
}
