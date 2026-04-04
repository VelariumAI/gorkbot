package selfimprove

import "time"

// EmotionalMode represents the current drive state.
type EmotionalMode int

const (
	// ModeCalm: idle, low drive (heartbeat 8 minutes).
	ModeCalm EmotionalMode = iota
	// ModeCurious: moderate drive, exploring (heartbeat 4 minutes).
	ModeCurious
	// ModeFocused: high drive, executing (heartbeat 90 seconds).
	ModeFocused
	// ModeUrgent: very high drive, failures detected (heartbeat 30 seconds).
	ModeUrgent
	// ModeRestrained: explicitly paused by user (heartbeat 30 minutes).
	ModeRestrained
)

func (m EmotionalMode) String() string {
	switch m {
	case ModeCalm:
		return "CALM"
	case ModeCurious:
		return "CURIOUS"
	case ModeFocused:
		return "FOCUSED"
	case ModeUrgent:
		return "URGENT"
	case ModeRestrained:
		return "RESTRAINED"
	default:
		return "UNKNOWN"
	}
}

// SignalSource indicates where a drive signal came from.
type SignalSource int

const (
	SourceSPARK SignalSource = iota
	SourceFreeWill
	SourceHarness
	SourceResearch
)

func (s SignalSource) String() string {
	switch s {
	case SourceSPARK:
		return "SPARK"
	case SourceFreeWill:
		return "FreeWill"
	case SourceHarness:
		return "Harness"
	case SourceResearch:
		return "Research"
	default:
		return "Unknown"
	}
}

// ImproveCycle tracks a single self-improvement work cycle.
type ImproveCycle struct {
	ID        string        // unique identifier
	StartedAt time.Time     // when the cycle started
	Source    SignalSource  // which facade triggered it
	Target    string        // what was targeted for improvement
	Mode      EmotionalMode // mode at cycle start
	Outcome   string        // "success" | "failed" | "skipped"
	Duration  time.Duration // how long it took
}

// CandidateInfo tracks the last selected improvement candidate.
type CandidateInfo struct {
	Source     SignalSource // which source triggered it
	Target     string       // what was selected for improvement
	BaseScore  float64      // raw score before selection
	FinalScore float64      // computed score after selection
}

// SISnapshot is a point-in-time snapshot of Self-Improve state.
type SISnapshot struct {
	Enabled        bool          // is SI currently enabled/running?
	Mode           EmotionalMode // current emotional mode
	DriveScore     float64       // current composite drive (0.0-1.0)
	LastCycle      *ImproveCycle // last completed cycle (or nil)
	NextHeartbeat  time.Time     // when the next heartbeat will fire
	PendingSignals int           // number of pending improvement signals

	// Extended fields for rich dashboard
	RawScore       float64        // pre-EWMA raw composite score
	CycleCount     int64          // total completed cycles
	Signals        SignalSnapshot // S1-S5 sub-score breakdown
	IsRunning      bool           // a cycle is currently executing
	ActivePhase    string         // "selecting" | "executing:<target>" | "verifying" | ""
	LastCandidate  *CandidateInfo // last selected candidate (nil before first cycle)
	CycleHistory   []ImproveCycle // ring buffer, last 5 completed cycles
}
