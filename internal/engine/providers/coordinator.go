package providers

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/internal/events"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/discovery"
	"github.com/velariumai/gorkbot/pkg/providers"
)

// ProviderCoordinator manages provider selection, failover, and health monitoring.
// It replaces scattered provider logic throughout the Orchestrator with a focused
// coordinator that communicates via the event bus.
type ProviderCoordinator struct {
	// Provider instances
	primary    ai.AIProvider
	consultant ai.AIProvider

	// Lifecycle management
	provMgr   *providers.Manager
	discovery *discovery.Manager

	// Configuration
	cascadeOrder     []string
	primaryModelName string

	// Utilities
	eventBus events.BusPublisher
	logger   *slog.Logger

	// Callbacks for coordinated updates to dependent systems
	// Called synchronously when provider changes to ensure atomicity
	onContextChange    func(maxTokens int)      // Notify ContextMgr
	onProviderSwitch   func(prov ai.AIProvider) // Notify Registry
	onCompressorChange func(gen ai.AIProvider)  // Notify Compressor
	onStabilizerReset  func()                   // Reset Stabilizer

	// Discovery polling
	discoveryTicker *time.Ticker
	discoveryMu     sync.RWMutex

	// Synchronization
	mu sync.RWMutex
}

// Default provider failover cascade order
var defaultCascadeOrder = []string{
	providers.ProviderXAI,
	providers.ProviderGoogle,
	providers.ProviderAnthropic,
	providers.ProviderMiniMax,
	providers.ProviderOpenAI,
	providers.ProviderOpenRouter,
	providers.ProviderMoonshot,
}

// NewProviderCoordinator creates a new provider coordinator.
// If any callbacks are nil, they are treated as no-ops.
func NewProviderCoordinator(
	provMgr *providers.Manager,
	primary, consultant ai.AIProvider,
	discovery *discovery.Manager,
	bus events.BusPublisher,
	logger *slog.Logger,
) *ProviderCoordinator {
	return &ProviderCoordinator{
		primary:            primary,
		consultant:         consultant,
		provMgr:            provMgr,
		discovery:          discovery,
		cascadeOrder:       nil, // Use default until overridden
		eventBus:           bus,
		logger:             logger,
		onContextChange:    func(int) {},
		onProviderSwitch:   func(ai.AIProvider) {},
		onCompressorChange: func(ai.AIProvider) {},
		onStabilizerReset:  func() {},
	}
}

// Primary returns the current primary provider (read-only access).
func (pc *ProviderCoordinator) Primary() ai.AIProvider {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.primary
}

// Consultant returns the current consultant (secondary) provider (read-only access).
func (pc *ProviderCoordinator) Consultant() ai.AIProvider {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.consultant
}

// Discovery returns the discovery manager (read-only access).
func (pc *ProviderCoordinator) Discovery() *discovery.Manager {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.discovery
}

// SetCallbacks registers callbacks for coordinated updates.
// Called by Orchestrator after wiring dependent systems.
func (pc *ProviderCoordinator) SetCallbacks(
	onContextChange func(int),
	onProviderSwitch func(ai.AIProvider),
	onCompressorChange func(ai.AIProvider),
	onStabilizerReset func(),
) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if onContextChange != nil {
		pc.onContextChange = onContextChange
	}
	if onProviderSwitch != nil {
		pc.onProviderSwitch = onProviderSwitch
	}
	if onCompressorChange != nil {
		pc.onCompressorChange = onCompressorChange
	}
	if onStabilizerReset != nil {
		pc.onStabilizerReset = onStabilizerReset
	}
}

// SetPrimary hot-swaps the primary AI provider.
// Calls all registered callbacks to update dependent systems atomically.
func (pc *ProviderCoordinator) SetPrimary(ctx context.Context, providerName, modelID string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.provMgr == nil {
		// Fallback: if no ProvMgr, update model on existing provider
		if pc.primary == nil {
			return fmt.Errorf("no provider manager and no current primary")
		}
		if modelID != "" {
			pc.primary = pc.primary.WithModel(modelID)
		}
		return nil
	}

	prov, err := pc.provMgr.GetProviderForModel(providerName, modelID)
	if err != nil {
		return fmt.Errorf("SetPrimary: %w", err)
	}

	pc.primary = prov
	pc.primaryModelName = prov.Name() + "/" + modelID

	// Call all callbacks to update dependent systems
	meta := prov.GetMetadata()
	if meta.ContextSize > 0 {
		pc.onContextChange(meta.ContextSize)
	}
	pc.onProviderSwitch(prov)
	pc.onCompressorChange(prov)

	if pc.logger != nil {
		pc.logger.Info("Primary provider switched",
			"provider", providerName,
			"model", modelID,
		)
	}

	// Publish event for observers
	if pc.eventBus != nil {
		evt := &events.ProviderFailoverEvent{
			BaseEvent:    events.NewBaseEvent(),
			FromProvider: "",
			ToProvider:   providerName,
			Reason:       "explicit_switch",
		}
		pc.eventBus.Publish(ctx, evt)
	}

	return nil
}

// SetSecondary hot-swaps the consultant (secondary) AI provider.
func (pc *ProviderCoordinator) SetSecondary(ctx context.Context, providerName, modelID string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.provMgr == nil {
		if pc.consultant == nil {
			return fmt.Errorf("no provider manager and no current consultant")
		}
		if modelID != "" {
			pc.consultant = pc.consultant.WithModel(modelID)
		}
		return nil
	}

	prov, err := pc.provMgr.GetProviderForModel(providerName, modelID)
	if err != nil {
		return fmt.Errorf("SetSecondary: %w", err)
	}

	pc.consultant = prov
	pc.onStabilizerReset()

	if pc.logger != nil {
		pc.logger.Info("Secondary provider switched",
			"provider", providerName,
			"model", modelID,
		)
	}

	return nil
}

// SelectConsultant returns the consultant provider for a given task.
// If consultant is statically set, returns it.
// Otherwise uses Discovery + ARC routing to intelligently select one.
func (pc *ProviderCoordinator) SelectConsultant(ctx context.Context, task string) ai.AIProvider {
	pc.mu.RLock()
	if pc.consultant != nil {
		pc.mu.RUnlock()
		return pc.consultant
	}
	pc.mu.RUnlock()

	return pc.selectConsultantIntelligent(ctx, task)
}

// selectConsultantIntelligent uses Discovery + ARC routing to pick the best
// secondary model that is different from the primary and has a valid key.
func (pc *ProviderCoordinator) selectConsultantIntelligent(ctx context.Context, task string) ai.AIProvider {
	if pc.discovery == nil || pc.provMgr == nil {
		return nil
	}

	// Classify the task capability requirement (default to reasoning)
	cap := discovery.CapReasoning

	// Assume Intelligence layer is available (hook if needed later)
	// For now, use reasoning as default
	preferCheap := false

	best := pc.discovery.BestForCap(cap, "")
	if best == nil {
		best = pc.discovery.BestForCap(discovery.CapGeneral, "")
	}
	if best == nil {
		return nil
	}

	// For cheap tier, prefer a cheap model if available
	if preferCheap && !providers.IsCheapModel(best.ID) {
		allModels := pc.discovery.Models()
		for i, m := range allModels {
			if providers.IsCheapModel(m.ID) && m.ID != best.ID {
				best = &allModels[i]
				break
			}
		}
	}

	// Avoid using the same model as primary
	primaryID := ""
	pc.mu.RLock()
	if pc.primary != nil {
		primaryID = pc.primary.GetMetadata().ID
	}
	pc.mu.RUnlock()

	if best.ID == primaryID {
		return nil
	}

	prov, err := pc.provMgr.GetProviderForModel(best.Provider, best.ID)
	if err != nil {
		if pc.logger != nil {
			pc.logger.Warn("selectConsultantIntelligent: provider unavailable",
				"provider", best.Provider,
				"model", best.ID,
				"error", err,
			)
		}
		return nil
	}
	return prov
}

// RunCascade initiates provider failover when the current provider fails.
// Returns (retryable, statusMsg, error).
func (pc *ProviderCoordinator) RunCascade(ctx context.Context, failedID string) (bool, string, error) {
	if pc.provMgr == nil {
		return false, "Provider manager not available", fmt.Errorf("no provider manager")
	}

	// 1. Disable the failed provider for this session
	pc.provMgr.DisableForSession(failedID)
	if pc.logger != nil {
		pc.logger.Info("Provider cascade started", "failed", failedID)
	}

	// 2. Build probe order: start one position after failedID, wrap around
	cascade := pc.effectiveCascade()
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

	// 3. Race all candidate providers in parallel; first ping wins
	newPrimary, newPrimaryModel := pc.raceProviderPings(ctx, probeOrder)

	if newPrimary == "" {
		return false, "All providers unreachable — check API keys/credits",
			fmt.Errorf("cascade: all providers down")
	}

	// 4. Switch primary provider
	if err := pc.SetPrimary(ctx, newPrimary, newPrimaryModel); err != nil {
		return false, fmt.Sprintf("Failed to switch to %s: %v",
			providers.ProviderName(newPrimary), err), err
	}

	// 5. Find a secondary (next available after newPrimary in probe order)
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
		if pc.provMgr.IsSessionDisabled(id) {
			continue
		}
		base, err := pc.provMgr.GetBase(id)
		if err != nil {
			continue
		}
		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		pingErr := pingWithRetry(pingCtx, base, 3, 200*time.Millisecond)
		pingCancel()
		if pingErr != nil {
			pc.provMgr.DisableForSession(id)
			continue
		}
		newSecondary = id
		newSecondaryModel = bestModelFor(id)
		break
	}

	// Edge case: only one provider works — use 2nd model of the same provider
	if newSecondary == "" {
		newSecondary = newPrimary
		newSecondaryModel = secondModelFor(newPrimary)
	}

	_ = pc.SetSecondary(ctx, newSecondary, newSecondaryModel)

	primaryName := providers.ProviderName(newPrimary) + "/" + newPrimaryModel
	secondaryName := providers.ProviderName(newSecondary) + "/" + newSecondaryModel
	failedName := providers.ProviderName(failedID)

	if pc.logger != nil {
		pc.logger.Info("Provider cascade complete",
			"new_primary", primaryName,
			"new_secondary", secondaryName,
			"disabled", failedName,
		)
	}

	return true, fmt.Sprintf("Switched to %s (secondary: %s). %s disabled for session.",
		primaryName, secondaryName, failedName), nil
}

// raceProviderPings fires pings for all candidate providers in parallel and
// returns the (providerID, modelID) of the first one that responds successfully.
func (pc *ProviderCoordinator) raceProviderPings(ctx context.Context, probeOrder []string) (string, string) {
	type result struct {
		id    string
		model string
	}

	winCh := make(chan result, 1)
	pingCtx, pingCancel := context.WithTimeout(ctx, 8*time.Second)
	defer pingCancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	bestPriority := len(probeOrder) + 1

	for idx, id := range probeOrder {
		if pc.provMgr.IsSessionDisabled(id) {
			continue
		}
		base, err := pc.provMgr.GetBase(id)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(id string, priority int, p ai.AIProvider) {
			defer wg.Done()
			if err := pingWithRetry(pingCtx, p, 3, 200*time.Millisecond); err != nil {
				pc.provMgr.DisableForSession(id)
				if pc.logger != nil {
					pc.logger.Info("Provider ping failed (race)",
						"provider", id,
						"error", err,
					)
				}
				return
			}
			mu.Lock()
			if priority < bestPriority {
				bestPriority = priority
				select {
				case winCh <- result{id, bestModelFor(id)}:
				default:
					// Replace winner if we are higher priority
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

	// Close winCh after all goroutines finish
	go func() { wg.Wait(); close(winCh) }()

	// Give the race up to the ping deadline
	select {
	case w, ok := <-winCh:
		if !ok {
			return "", ""
		}
		// Drain remaining wins
		go func() {
			for range winCh {
			}
		}()
		return w.id, w.model
	case <-pingCtx.Done():
		return "", ""
	}
}

// SetProviderKey stores a new API key for the given provider.
func (pc *ProviderCoordinator) SetProviderKey(ctx context.Context, providerName, key string) error {
	if pc.provMgr == nil {
		return fmt.Errorf("provider manager not initialized")
	}
	if err := pc.provMgr.SetKey(ctx, providerName, key, true); err != nil {
		return fmt.Errorf("key validation failed for %s: %w",
			providers.ProviderName(providerName), err)
	}
	// Re-poll discovery with the new key
	pc.discoveryMu.Lock()
	if pc.discovery != nil {
		pc.discovery.Start(ctx)
	}
	pc.discoveryMu.Unlock()
	return nil
}

// GetProviderStatus returns a formatted status summary of all providers.
func (pc *ProviderCoordinator) GetProviderStatus() string {
	if pc.provMgr == nil {
		return "Provider manager not initialized"
	}
	return pc.provMgr.KeyStore().FormatStatus()
}

// SetCascadeOrder updates the provider failover order at runtime.
func (pc *ProviderCoordinator) SetCascadeOrder(order []string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.cascadeOrder = order
}

// GetCascadeOrder returns the current effective cascade order.
func (pc *ProviderCoordinator) GetCascadeOrder() []string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.effectiveCascade()
}

// effectiveCascade returns the cascade order to use for provider failover.
// If custom CascadeOrder is set, it is used; otherwise the default is returned.
func (pc *ProviderCoordinator) effectiveCascade() []string {
	if len(pc.cascadeOrder) > 0 {
		return pc.cascadeOrder
	}
	return defaultCascadeOrder
}

// StartDiscovery begins polling providers for live model lists.
// Call once at session start.
func (pc *ProviderCoordinator) StartDiscovery(ctx context.Context) {
	pc.discoveryMu.Lock()
	if pc.discovery == nil {
		pc.discoveryMu.Unlock()
		return
	}
	pc.discovery.Start(ctx)
	pc.discoveryMu.Unlock()
}

// StopDiscovery stops polling providers.
func (pc *ProviderCoordinator) StopDiscovery() {
	pc.discoveryMu.Lock()
	if pc.discovery != nil && pc.discoveryTicker != nil {
		pc.discoveryTicker.Stop()
	}
	pc.discoveryMu.Unlock()
}

// ─── Utility Functions ──────────────────────────────────────────────────────

// bestModelFor returns the top safe-fallback model ID for a provider.
func bestModelFor(providerID string) string {
	models := ai.SafeModelDefs(providerID)
	if len(models) == 0 {
		return ""
	}
	return string(models[0].ID)
}

// secondModelFor returns the 2nd safe model for a provider.
func secondModelFor(providerID string) string {
	models := ai.SafeModelDefs(providerID)
	if len(models) < 2 {
		return bestModelFor(providerID)
	}
	return string(models[1].ID)
}

// isProviderOutage returns true for errors that warrant trying another provider.
func isProviderOutage(err error) bool {
	if err == nil {
		return false
	}
	if isAIError(err) {
		return true
	}

	// Network-level timeout
	var netErr net.Error
	if asNetErr(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Message pattern matching
	msg := strings.ToLower(err.Error())
	outageKeywords := []string{
		"credit", "billing", "payment", "quota", "insufficient_quota",
		"tls: bad record mac", "connection reset by peer",
		"broken pipe", "use of closed network connection",
	}
	for _, kw := range outageKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}

	// HTTP status codes
	statusCodes := []string{"401", "402", "403", "429", "500", "502", "503", "504"}
	for _, code := range statusCodes {
		if strings.Contains(msg, "status "+code) ||
			strings.Contains(msg, "("+code+")") {
			return true
		}
	}

	return false
}

// isAIError checks if err is a known AI provider error type.
func isAIError(err error) bool {
	return err != nil &&
		(err == ai.ErrUnauthorized ||
			err == ai.ErrProviderDown ||
			err == ai.ErrBadGateway ||
			err == ai.ErrRateLimit ||
			err == ai.ErrNoCredits)
}

// asNetErr unwraps err into a net.Error.
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

func pingWithRetry(ctx context.Context, p ai.AIProvider, maxAttempts int, baseDelay time.Duration) error {
	if p == nil {
		return fmt.Errorf("nil provider")
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := p.Ping(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if attempt == maxAttempts-1 || !isRetryableProviderErr(lastErr) {
			break
		}

		delay := baseDelay * time.Duration(1<<attempt)
		if delay > 2*time.Second {
			delay = 2 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func isRetryableProviderErr(err error) bool {
	if err == nil {
		return false
	}
	// Fast-path known retryable provider-layer errors.
	if err == ai.ErrProviderDown || err == ai.ErrBadGateway || err == ai.ErrRateLimit {
		return true
	}

	// Network-level timeout/temporary.
	var netErr net.Error
	if asNetErr(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	msg := strings.ToLower(err.Error())
	retryableKeywords := []string{
		"status 429", "status 500", "status 502", "status 503", "status 504",
		"(429)", "(500)", "(502)", "(503)", "(504)",
		"timeout", "temporarily unavailable",
	}
	for _, kw := range retryableKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
