package execution

import (
	"sync"
	"time"
)

// BreakerState captures circuit-breaker state.
type BreakerState string

const (
	BREAKER_CLOSED    BreakerState = "BREAKER_CLOSED"
	BREAKER_OPEN      BreakerState = "BREAKER_OPEN"
	BREAKER_HALF_OPEN BreakerState = "BREAKER_HALF_OPEN"
)

// CircuitBreaker guards an unreliable dependency.
type CircuitBreaker struct {
	Name             string
	FailureThreshold int
	SuccessThreshold int
	Cooldown         time.Duration
	Window           time.Duration

	mu            sync.Mutex
	state         BreakerState
	failureCount  int
	successCount  int
	openedAt      time.Time
	lastReason    string
	lastFailureAt time.Time
}

// NewCircuitBreaker creates a breaker with CLOSED initial state.
func NewCircuitBreaker(name string, failureThreshold int, cooldown time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 1
	}
	return &CircuitBreaker{
		Name:             name,
		FailureThreshold: failureThreshold,
		SuccessThreshold: 1,
		Cooldown:         cooldown,
		state:            BREAKER_CLOSED,
	}
}

// Allow reports whether a call should be attempted.
func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == BREAKER_CLOSED {
		return true
	}
	if b.state == BREAKER_OPEN {
		if b.Cooldown <= 0 || time.Since(b.openedAt) >= b.Cooldown {
			b.state = BREAKER_HALF_OPEN
			b.successCount = 0
			return true
		}
		return false
	}
	return true
}

// RecordSuccess records a successful call.
func (b *CircuitBreaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case BREAKER_CLOSED:
		b.failureCount = 0
	case BREAKER_HALF_OPEN:
		b.successCount++
		if b.successCount >= max(1, b.SuccessThreshold) {
			b.state = BREAKER_CLOSED
			b.failureCount = 0
			b.successCount = 0
		}
	}
}

// RecordFailure records a failed call.
func (b *CircuitBreaker) RecordFailure(reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lastReason = reason
	b.lastFailureAt = time.Now()

	switch b.state {
	case BREAKER_CLOSED:
		b.failureCount++
		if b.failureCount >= max(1, b.FailureThreshold) {
			b.state = BREAKER_OPEN
			b.openedAt = time.Now()
			b.successCount = 0
		}
	case BREAKER_HALF_OPEN:
		b.state = BREAKER_OPEN
		b.openedAt = time.Now()
		b.successCount = 0
		b.failureCount = max(1, b.FailureThreshold)
	case BREAKER_OPEN:
		b.openedAt = time.Now()
	}
}

// State returns current breaker state.
func (b *CircuitBreaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// LastReason returns most recent failure reason.
func (b *CircuitBreaker) LastReason() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastReason
}

// BreakerSet groups breakers by subsystem.
type BreakerSet struct {
	VCSE     *CircuitBreaker
	Provider *CircuitBreaker
	Tool     *CircuitBreaker
	Puter    *CircuitBreaker
	Search   *CircuitBreaker
	Subagent *CircuitBreaker
}

// NewDefaultBreakerSet returns default subsystem breaker thresholds.
func NewDefaultBreakerSet() *BreakerSet {
	return &BreakerSet{
		VCSE:     NewCircuitBreaker("vcse", 3, 2*time.Minute),
		Provider: NewCircuitBreaker("provider", 3, 1*time.Minute),
		Tool:     NewCircuitBreaker("tool", 3, 1*time.Minute),
		Puter:    NewCircuitBreaker("puter", 3, 2*time.Minute),
		Search:   NewCircuitBreaker("search", 3, 1*time.Minute),
		Subagent: NewCircuitBreaker("subagent", 2, 2*time.Minute),
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
