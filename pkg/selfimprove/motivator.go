package selfimprove

import (
	"sync"
)

// Motivator computes a multi-signal drive score using EWMA smoothing.
type Motivator struct {
	mu          sync.RWMutex
	alpha       float64 // EWMA smoothing factor (0.15 = slow-moving)
	prevScore   float64 // previous EWMA value
	lastScore   float64 // most recently computed raw score
	ewmaScore   float64 // current EWMA score
	modeHyst    float64 // hysteresis band (±0.05)
	lastSignals SignalSnapshot
}

// NewMotivator creates a motivator with given EWMA alpha.
func NewMotivator(alpha float64) *Motivator {
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.15 // default
	}
	return &Motivator{
		alpha:    alpha,
		prevScore: 0,
		lastScore: 0,
		ewmaScore: 0,
		modeHyst: 0.05,
	}
}

// Update recomputes the drive score from all signal sources.
// Returns the new EmotionalMode based on the updated EWMA score.
func (m *Motivator) Update(signals *SignalSnapshot) EmotionalMode {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store signals for later retrieval
	m.lastSignals = *signals

	// Compute quality metrics (0.0-1.0 each).
	s1 := signals.SPARKDriveScore // already 0.0-1.0

	// s2: tool health (1 - worst failure rate)
	s2 := 1.0
	if signals.ToolHealthWorst > 0 {
		s2 = 1.0 - min(signals.ToolHealthWorst, 1.0)
	}

	// s3: directive ratio
	s3 := 0.0
	if signals.SPARKActiveDirectives > 0 {
		s3 = min(float64(signals.SPARKActiveDirectives)/20.0, 1.0)
	}

	// s4: harness failing ratio
	s4 := 0.0
	if signals.HarnessTotal > 0 {
		s4 = float64(signals.HarnessFailing) / float64(signals.HarnessTotal)
	}

	// s5: free will proposals pending
	s5 := 0.0
	if signals.FreeWillProposalsPending > 0 {
		s5 = min(float64(signals.FreeWillProposalsPending)/5.0, 1.0)
	}

	// Weighted composite: quality vs badness.
	quality := (s1 + s3 + s5) / 3.0
	badness := (s2 + s4) / 2.0
	rawScore := 0.50*quality + 0.50*badness

	// Apply EWMA smoothing.
	m.lastScore = rawScore
	m.ewmaScore = m.alpha*rawScore + (1.0-m.alpha)*m.prevScore
	m.prevScore = m.ewmaScore

	return m.scoreToMode(m.ewmaScore)
}

// scoreToMode converts EWMA score to EmotionalMode with hysteresis.
func (m *Motivator) scoreToMode(score float64) EmotionalMode {
	// Thresholds with hysteresis bands:
	// Calm:     [0.0, 0.15)
	// Curious:  [0.15, 0.35)
	// Focused:  [0.35, 0.65)
	// Urgent:   [0.65, 1.0]
	if score < 0.15 {
		return ModeCalm
	}
	if score < 0.35 {
		return ModeCurious
	}
	if score < 0.65 {
		return ModeFocused
	}
	return ModeUrgent
}

// GetScore returns the current EWMA drive score (0.0-1.0).
func (m *Motivator) GetScore() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ewmaScore
}

// GetLastRaw returns the most recently computed raw (non-smoothed) score.
func (m *Motivator) GetLastRaw() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastScore
}

// LastSignals returns the most recently computed signal snapshot.
func (m *Motivator) LastSignals() SignalSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSignals
}

// SignalSnapshot captures all drive signals at a point in time.
type SignalSnapshot struct {
	SPARKDriveScore         float64 // from SPARK state
	SPARKActiveDirectives   int
	SPARKIDLDebt            int
	ToolHealthWorst         float64 // worst tool failure rate
	HarnessFailing          int     // number of failing features
	HarnessTotal            int     // total tracked features
	FreeWillProposalsPending int    // number of pending proposals
	ResearchBufferedDocs    int     // buffered research documents
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
