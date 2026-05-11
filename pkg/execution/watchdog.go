package execution

import (
	"sync"
	"time"
)

// TurnState tracks coarse orchestrator phase for watchdog timeouts.
type TurnState string

const (
	TURN_IDLE             TurnState = "TURN_IDLE"
	TURN_PLANNING         TurnState = "TURN_PLANNING"
	TURN_DECIDING         TurnState = "TURN_DECIDING"
	TURN_WAITING_FOR_TOOL TurnState = "TURN_WAITING_FOR_TOOL"
	TURN_EXECUTING_TOOL   TurnState = "TURN_EXECUTING_TOOL"
	TURN_VERIFYING        TurnState = "TURN_VERIFYING"
	TURN_RENDERING        TurnState = "TURN_RENDERING"
	TURN_COMPLETE         TurnState = "TURN_COMPLETE"
	TURN_FAILED           TurnState = "TURN_FAILED"
	TURN_INTERRUPTED      TurnState = "TURN_INTERRUPTED"
)

// Watchdog tracks turn state and detects idle / total timeout breaches.
type Watchdog struct {
	mu             sync.Mutex
	state          TurnState
	enteredAt      time.Time
	turnStartedAt  time.Time
	budget         ExecutionBudget
	lastProgressAt time.Time
	cancelled      bool
	reason         string
}

// NewWatchdog creates a watchdog with TURN_IDLE state.
func NewWatchdog(budget ExecutionBudget) *Watchdog {
	now := time.Now()
	return &Watchdog{
		state:          TURN_IDLE,
		enteredAt:      now,
		turnStartedAt:  now,
		budget:         budget,
		lastProgressAt: now,
	}
}

// Enter moves watchdog to a new state and resets state-idle timer.
func (w *Watchdog) Enter(state TurnState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	w.state = state
	w.enteredAt = now
	w.lastProgressAt = now
}

// Progress marks forward progress and resets idle timer.
func (w *Watchdog) Progress(note string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastProgressAt = time.Now()
}

// State returns the current turn state.
func (w *Watchdog) State() TurnState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// TimedOut reports whether total or idle timeout has elapsed.
func (w *Watchdog) TimedOut() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	if w.budget.TurnTimeout > 0 && now.Sub(w.turnStartedAt) > w.budget.TurnTimeout {
		w.reason = "turn timeout exceeded"
		return true
	}
	if w.budget.MaxIdleDuration > 0 && now.Sub(w.lastProgressAt) > w.budget.MaxIdleDuration {
		w.reason = "state idle timeout exceeded"
		return true
	}
	return false
}

// TimeoutReason returns the most recent timeout/cancel reason.
func (w *Watchdog) TimeoutReason() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reason
}

// Cancel marks watchdog cancelled with reason.
func (w *Watchdog) Cancel(reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cancelled = true
	w.reason = reason
}

// Cancelled reports whether cancellation has been requested.
func (w *Watchdog) Cancelled() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cancelled
}
