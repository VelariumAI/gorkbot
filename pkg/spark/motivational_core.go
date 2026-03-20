package spark

import (
	"fmt"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
)

// MotivationalCore wraps *sense.LIEEvaluator and drives an EWMA quality score
// (DriveScore) calibrating response quality over the session.
type MotivationalCore struct {
	lie         *sense.LIEEvaluator
	alpha       float64
	driveScore  float64 // initialised at 0.5
	mu          sync.RWMutex
	history     []float64 // ring buffer, cap 20
	lastUpdated time.Time
}

// NewMotivationalCore creates a MotivationalCore backed by the given LIEEvaluator.
func NewMotivationalCore(lie *sense.LIEEvaluator, alpha float64) *MotivationalCore {
	return &MotivationalCore{
		lie:        lie,
		alpha:      alpha,
		driveScore: 0.5,
		history:    make([]float64, 0, 20),
	}
}

// Observe evaluates a response and updates the DriveScore EWMA.
func (mc *MotivationalCore) Observe(response string) sense.RewardMetrics {
	metrics, _ := mc.lie.Evaluate(response)

	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.driveScore = mc.alpha*metrics.FinalReward + (1-mc.alpha)*mc.driveScore
	// Clamp to [-1, 1] (FinalReward is already clamped, but be defensive).
	mc.driveScore = clamp(mc.driveScore, -1.0, 1.0)
	mc.history = append(mc.history, metrics.FinalReward)
	if len(mc.history) > 20 {
		mc.history = mc.history[len(mc.history)-20:]
	}
	mc.lastUpdated = time.Now()
	return metrics
}

// CalibrateWeights adjusts LIE weights based on recent reward history.
// No-op if fewer than 5 observations.
func (mc *MotivationalCore) CalibrateWeights() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if len(mc.history) < 5 {
		return
	}
	var sum float64
	for _, v := range mc.history {
		sum += v
	}
	mean := sum / float64(len(mc.history))

	if mean < 0.3 {
		mc.lie.LengthWeight = clamp01(mc.lie.LengthWeight + 0.05)
		mc.lie.StructureWeight = clamp01(mc.lie.StructureWeight + 0.05)
	} else if mean > 0.8 {
		mc.lie.LengthWeight = clamp01(mc.lie.LengthWeight - 0.02)
		mc.lie.StructureWeight = clamp01(mc.lie.StructureWeight - 0.02)
	}
}

// DriveScore returns the current EWMA quality score.
func (mc *MotivationalCore) DriveScore() float64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.driveScore
}

// FormatDriveBlock returns a formatted [SPARK: Motivational Core] prompt block.
func (mc *MotivationalCore) FormatDriveBlock() string {
	score := mc.DriveScore()
	var directive string
	switch {
	case score < 0.3:
		directive = "CRITICAL: Response quality is very low. Provide substantially longer, " +
			"better-structured answers with clear reasoning steps."
	case score < 0.6:
		directive = "Response quality is below target. Aim for more comprehensive answers " +
			"with explicit structure and diverse content."
	default:
		directive = "Response quality is good. Maintain depth and structural clarity."
	}
	return fmt.Sprintf("[SPARK: Motivational Core]\nDrive Score: %.3f\nDirective: %s", score, directive)
}

// ObserveEnsemble evaluates multiple trajectory outputs and returns per-output
// FinalReward scores. Updates DriveScore EWMA for each output.
func (mc *MotivationalCore) ObserveEnsemble(outputs []string) []float64 {
	scores := make([]float64, len(outputs))
	for i, out := range outputs {
		m := mc.Observe(out)
		scores[i] = m.FinalReward
	}
	return scores
}

// clamp01 clamps v to the [0, 1] range.
func clamp01(v float64) float64 {
	return clamp(v, 0.0, 1.0)
}

// clamp clamps v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
