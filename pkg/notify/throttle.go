package notify

import (
	"sync"
	"time"
)

// Throttle tracks notification rates to prevent spamming.
// Maps notification ID (tool name or type) to last emission time.
type Throttle struct {
	mu       sync.RWMutex
	lastSent map[string]time.Time
}

// NewThrottle creates a new notification throttle.
func NewThrottle() *Throttle {
	return &Throttle{
		lastSent: make(map[string]time.Time),
	}
}

// Allow checks if a notification should be sent based on cooldown.
// Returns true if enough time has elapsed since the last notification of this type.
func (t *Throttle) Allow(id string, cooldown time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	last, exists := t.lastSent[id]
	now := time.Now()

	// If no previous record or cooldown expired, allow
	if !exists || now.Sub(last) >= cooldown {
		t.lastSent[id] = now
		return true
	}

	return false
}

// Reset clears the throttle for a specific notification ID.
func (t *Throttle) Reset(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.lastSent, id)
}

// ResetAll clears all throttle records.
func (t *Throttle) ResetAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastSent = make(map[string]time.Time)
}

// GetLastSent returns when a notification was last sent.
func (t *Throttle) GetLastSent(id string) (time.Time, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	last, exists := t.lastSent[id]
	return last, exists
}

// GetNextAllowed returns the time when the next notification will be allowed.
func (t *Throttle) GetNextAllowed(id string, cooldown time.Duration) time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()

	last, exists := t.lastSent[id]
	if !exists {
		return time.Now()
	}

	return last.Add(cooldown)
}

// --- Predefined Cooldowns ---

// Common notification cooldowns to prevent spam
const (
	// System Monitor: Severe cooldown (user said "spamming notifications")
	// 30 minutes minimum between system monitor alerts
	SystemMonitorCooldown = 30 * time.Minute

	// HITL Approval: 2 minutes between repeated notifications
	// (user approval needed, but don't spam constantly)
	HITLNotificationCooldown = 2 * time.Minute

	// Low Resources (memory/disk): 15 minutes between alerts
	LowResourceCooldown = 15 * time.Minute

	// Battery Low: 30 minutes between alerts
	BatteryLowCooldown = 30 * time.Minute

	// General Tool Notifications: 1 minute cooldown
	GeneralToolCooldown = 1 * time.Minute
)
