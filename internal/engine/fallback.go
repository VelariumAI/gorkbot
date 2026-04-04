package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/providers"
)

// providerPriority is the default failover order used when CascadeOrder is not set.
var providerPriority = []string{
	providers.ProviderXAI,
	providers.ProviderGoogle,
	providers.ProviderAnthropic,
	providers.ProviderMiniMax,
	providers.ProviderOpenAI,
	providers.ProviderOpenRouter,
	providers.ProviderMoonshot,
}

// isProviderOutage returns true for errors that warrant trying another provider.
// Does NOT match ErrContextExceeded (model limit, not outage).
//
// Extended to handle:
//   - Network-level timeouts (TLS handshake, response-header, dial) — these
//     indicate the provider endpoint is unreachable, not the user's context.
//   - Transient transport errors (EOF, TLS MAC, RST) that survived all
//     RetryTransport retries, meaning the provider is persistently unreachable.
func isProviderOutage(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ai.ErrUnauthorized) ||
		errors.Is(err, ai.ErrProviderDown) ||
		errors.Is(err, ai.ErrBadGateway) ||
		errors.Is(err, ai.ErrRateLimit) ||
		errors.Is(err, ai.ErrNoCredits) {
		return true
	}

	// Network-level timeout: covers TLSHandshakeTimeout, ResponseHeaderTimeout,
	// and DialContext timeout — all controlled by our hardened transport, not
	// by the user's context. This distinguishes provider unreachability from
	// a user-cancelled request.
	var netErr net.Error
	if asNetErr(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Also detect raw error message patterns from providers that haven't been
	// updated to use sentinel errors yet.
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{
		"credit", "billing", "payment", "quota", "insufficient_quota",
		// Transient transport errors that survived all RetryTransport retries.
		"tls: bad record mac", "connection reset by peer",
		"broken pipe", "use of closed network connection",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	// Detect raw HTTP status codes in error messages.
	for _, code := range []string{"status 401", "status 402", "status 403", "status 429",
		"status 500", "status 502", "status 503", "status 504",
		"(401)", "(402)", "(403)", "(429)", "(500)", "(502)", "(503)", "(504)"} {
		if strings.Contains(msg, code) {
			return true
		}
	}
	return false
}

// isContextOverflowErr returns true when the error message indicates a context
// token limit was exceeded.  Used by the SENSE tracer to classify streaming errors.
func isContextOverflowErr(msg string) bool {
	lower := strings.ToLower(msg)
	signals := []string{
		"context length", "context window", "context_length_exceeded",
		"maximum context", "max tokens", "token limit", "too many tokens",
		"input is too long", "tokens exceeds", "reduce your prompt",
		"context_too_long", "request too large",
	}
	for _, s := range signals {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// asNetErr unwraps err into a net.Error, traversing one level of wrapping.
func asNetErr(err error, target *net.Error) bool {
	if ne, ok := err.(net.Error); ok {
		*target = ne
		return true
	}
	type unwrapper interface{ Unwrap() error }
	if uw, ok := err.(unwrapper); ok {
		inner := uw.Unwrap()
		if inner != nil {
			return asNetErr(inner, target)
		}
	}
	return false
}

// bestModelFor returns the top safe-fallback model ID for a provider.
func bestModelFor(providerID string) string {
	models := ai.SafeModelDefs(providerID)
	if len(models) == 0 {
		return ""
	}
	return string(models[0].ID)
}

// secondModelFor returns the 2nd safe model for a provider.
// Used when primary and secondary must share the same API key.
func secondModelFor(providerID string) string {
	models := ai.SafeModelDefs(providerID)
	if len(models) < 2 {
		return bestModelFor(providerID)
	}
	return string(models[1].ID)
}

// RunProviderCascade delegates to the provider coordinator's RunCascade method.
// Returns (retryable, statusMsg).
func (o *Orchestrator) RunProviderCascade(ctx context.Context, failedID string) (bool, string) {
	if o.ProviderCoord == nil {
		return false, "Provider coordinator not available"
	}
	retryable, msg, err := o.ProviderCoord.RunCascade(ctx, failedID)
	if err != nil {
		return retryable, fmt.Sprintf("%s (error: %v)", msg, err)
	}
	return retryable, msg
}
