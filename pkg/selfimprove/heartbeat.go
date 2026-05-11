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
	resetCh    chan heartbeatReset
	tickCh     chan time.Time
}

type heartbeatReset struct {
	interval time.Duration
	done     chan struct{}
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
		resetCh:    make(chan heartbeatReset, 1),
		tickCh:     make(chan time.Time),
	}
	h.ticker = time.NewTicker(h.currentInt)
	// Start the relay loop.
	go h.relayTicks()
	return h
}

// relayTicks runs in a background goroutine and forwards ticker events.
func (h *AdaptiveHeartbeat) relayTicks() {
	tickC := h.ticker.C
	for {
		select {
		case <-h.stopCh:
			return
		case req := <-h.resetCh:
			h.mu.Lock()
			h.ticker.Stop()
			h.currentInt = req.interval
			h.ticker = time.NewTicker(req.interval)
			tickC = h.ticker.C
			h.mu.Unlock()
			close(req.done)
		case t := <-tickC:
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
	newInt, ok := modeIntervals[mode]
	if !ok {
		newInt = modeIntervals[ModeCalm]
	}

	h.mu.Lock()
	same := newInt == h.currentInt
	h.mu.Unlock()
	if same {
		return
	}
	req := heartbeatReset{interval: newInt, done: make(chan struct{})}
	select {
	case h.resetCh <- req:
	default:
		// Keep latest requested interval when updates arrive rapidly.
		select {
		case <-h.resetCh:
		default:
		}
		h.resetCh <- req
	}
	select {
	case <-req.done:
	case <-h.stopCh:
	}
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
