package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

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

// effectiveCascade returns the cascade order to use for provider failover.
// If the orchestrator has a custom CascadeOrder set, it is used; otherwise
// the hardcoded providerPriority default is returned.
func (o *Orchestrator) effectiveCascade() []string {
	if len(o.CascadeOrder) > 0 {
		return o.CascadeOrder
	}
	return providerPriority
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

// raceProviderPings fires pings for all candidate providers in parallel and
// returns the (providerID, modelID) of the first one that responds successfully.
// Slower winners are cancelled via context. Providers that fail are disabled
// for the session. Returns ("", "") when every candidate is unreachable.
func (o *Orchestrator) raceProviderPings(ctx context.Context, pm *providers.Manager, probeOrder []string) (providerID, modelID string) {
	type result struct {
		id    string
		model string
	}

	winCh := make(chan result, 1)
	pingCtx, pingCancel := context.WithTimeout(ctx, 8*time.Second)
	defer pingCancel()

	var wg sync.WaitGroup
	// priority serialises winner selection so the highest-priority ready
	// provider wins when two respond within the same scheduler tick.
	var mu sync.Mutex
	bestPriority := len(probeOrder) + 1

	for idx, id := range probeOrder {
		if pm.IsSessionDisabled(id) {
			continue
		}
		base, err := pm.GetBase(id)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(id string, priority int, p ai.AIProvider) {
			defer wg.Done()
			if err := p.Ping(pingCtx); err != nil {
				pm.DisableForSession(id)
				if o.Logger != nil {
					o.Logger.Info("Provider ping failed (race)", "provider", id, "error", err)
				}
				return
			}
			mu.Lock()
			if priority < bestPriority {
				bestPriority = priority
				select {
				case winCh <- result{id, bestModelFor(id)}:
				default:
					// Replace winner if we are higher priority.
					// Drain and resend.
					select {
					case <-winCh:
					default:
					}
					winCh <- result{id, bestModelFor(id)}
				}
			}
			mu.Unlock()
		}(id, idx, base)
	}

	// Close winCh after all goroutines finish so we can range-receive.
	go func() { wg.Wait(); close(winCh) }()

	// Give the race up to the ping deadline.
	select {
	case w, ok := <-winCh:
		if !ok {
			return "", ""
		}
		// Drain remaining wins — we already have the best one.
		go func() {
			for range winCh {
			}
		}()
		return w.id, w.model
	case <-pingCtx.Done():
		return "", ""
	}
}

// RunProviderCascade pings providers in priority order, disables failures,
// then hot-swaps Primary + Consultant.
// Returns (retryable, statusMsg).
func (o *Orchestrator) RunProviderCascade(ctx context.Context, failedID string) (bool, string) {
	pm := globalProvMgr
	if pm == nil {
		return false, "Provider manager not available"
	}

	// 1. Disable the failed provider for this session.
	pm.DisableForSession(failedID)
	if o.Logger != nil {
		o.Logger.Info("Provider cascade started", "failed", failedID)
	}

	// 2. Build probe order: start one position after failedID, wrap around.
	cascade := o.effectiveCascade()
	startIdx := 0
	for i, id := range cascade {
		if id == failedID {
			startIdx = i + 1
			break
		}
	}
	probeOrder := make([]string, 0, len(cascade))
	for i := 0; i < len(cascade); i++ {
		probeOrder = append(probeOrder, cascade[(startIdx+i)%len(cascade)])
	}

	// 3. Race all candidate providers in parallel; first ping wins.
	newPrimary, newPrimaryModel := o.raceProviderPings(ctx, pm, probeOrder)

	if newPrimary == "" {
		return false, "All providers unreachable — check API keys/credits"
	}

	// 4. Switch primary provider.
	if err := o.SetPrimary(ctx, newPrimary, newPrimaryModel); err != nil {
		return false, fmt.Sprintf("Failed to switch to %s: %v", providers.ProviderName(newPrimary), err)
	}

	// 5. Find a secondary (next available after newPrimary in probe order).
	newPrimaryIdx := -1
	for i, id := range probeOrder {
		if id == newPrimary {
			newPrimaryIdx = i
			break
		}
	}

	newSecondary := ""
	newSecondaryModel := ""
	for i := newPrimaryIdx + 1; i < len(probeOrder); i++ {
		id := probeOrder[i]
		if pm.IsSessionDisabled(id) {
			continue
		}
		base, err := pm.GetBase(id)
		if err != nil {
			continue
		}
		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		pingErr := base.Ping(pingCtx)
		pingCancel()
		if pingErr != nil {
			pm.DisableForSession(id)
			continue
		}
		newSecondary = id
		newSecondaryModel = bestModelFor(id)
		break
	}

	// Edge case: only one provider works — use 2nd model of the same provider.
	if newSecondary == "" {
		newSecondary = newPrimary
		newSecondaryModel = secondModelFor(newPrimary)
	}

	_ = o.SetSecondary(ctx, newSecondary, newSecondaryModel)

	primaryName := providers.ProviderName(newPrimary) + "/" + newPrimaryModel
	secondaryName := providers.ProviderName(newSecondary) + "/" + newSecondaryModel
	failedName := providers.ProviderName(failedID)

	if o.Logger != nil {
		o.Logger.Info("Provider cascade complete",
			"new_primary", primaryName,
			"new_secondary", secondaryName,
			"disabled", failedName,
		)
	}

	return true, fmt.Sprintf("Switched to %s (secondary: %s). %s disabled for session.",
		primaryName, secondaryName, failedName)
}
