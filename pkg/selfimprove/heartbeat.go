package selfimprove

import (
	"sync"
	"time"
)

// AdaptiveHeartbeat adjusts its tick interval based on EmotionalMode.
type AdaptiveHeartbeat struct {
	mu         sync.Mutex
	ticker     *time.Ticker
	currentInt time.Duration
	stopCh     chan struct{}
	tickCh     chan time.Time
}

// Mode-specific intervals.
var modeIntervals = map[EmotionalMode]time.Duration{
	ModeCalm:       8 * time.Minute,
	ModeCurious:    4 * time.Minute,
	ModeFocused:    90 * time.Second,
	ModeUrgent:     30 * time.Second,
	ModeRestrained: 30 * time.Minute,
}

// NewAdaptiveHeartbeat creates a new heartbeat starting in ModeCalm.
func NewAdaptiveHeartbeat() *AdaptiveHeartbeat {
	h := &AdaptiveHeartbeat{
		currentInt: modeIntervals[ModeCalm],
		stopCh:     make(chan struct{}),
		tickCh:     make(chan time.Time),
	}
	h.ticker = time.NewTicker(h.currentInt)
	// Start the relay loop.
	go h.relayTicks()
	return h
}

// relayTicks runs in a background goroutine and forwards ticker events.
func (h *AdaptiveHeartbeat) relayTicks() {
	for {
		select {
		case <-h.stopCh:
			return
		case t := <-h.ticker.C:
			select {
			case h.tickCh <- t:
			case <-h.stopCh:
				return
			}
		}
	}
}

// Adapt changes the heartbeat interval for a new mode.
// This hot-resets the ticker and clears any pending tick.
func (h *AdaptiveHeartbeat) Adapt(mode EmotionalMode) {
	h.mu.Lock()
	defer h.mu.Unlock()

	newInt, ok := modeIntervals[mode]
	if !ok {
		newInt = modeIntervals[ModeCalm]
	}

	// If no change, skip.
	if newInt == h.currentInt {
		return
	}

	h.currentInt = newInt
	h.ticker.Stop()
	h.ticker = time.NewTicker(newInt)
}

// C returns the ticker channel to listen for ticks.
func (h *AdaptiveHeartbeat) C() <-chan time.Time {
	return h.tickCh
}

// Stop halts the heartbeat and cleans up resources.
func (h *AdaptiveHeartbeat) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ticker.Stop()
	close(h.stopCh)
}

// NextTickTime returns the expected time of the next tick.
func (h *AdaptiveHeartbeat) NextTickTime() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Now().Add(h.currentInt)
}
