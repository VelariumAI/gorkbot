package trace

import (
	"errors"
	"strings"
	"time"
)

const (
	maxOperatorPath = 64
	maxEventRefs    = 256
)

// CostSummary stores aggregated resource usage for a trajectory.
type CostSummary struct {
	TokenUsage
	CostEstimate
}

// Trajectory captures an ordered task path for replay/drift analysis.
type Trajectory struct {
	TrajectoryID       string            `json:"trajectory_id"`
	SessionID          string            `json:"session_id,omitempty"`
	ParentTrajectoryID string            `json:"parent_trajectory_id,omitempty"`
	ObjectiveHash      string            `json:"objective_hash,omitempty"`
	ObjectiveSummary   string            `json:"objective_summary,omitempty"`
	ModeProfile        string            `json:"mode_profile,omitempty"`
	OperatorPath       []Operator        `json:"operator_path,omitempty"`
	StartStateHash     string            `json:"start_state_hash,omitempty"`
	EndStateHash       string            `json:"end_state_hash,omitempty"`
	EventRefs          []string          `json:"event_refs,omitempty"`
	ArtifactRefs       []Ref             `json:"artifact_refs,omitempty"`
	ValidationRefs     []Ref             `json:"validation_refs,omitempty"`
	DecisionRefs       []Ref             `json:"decision_refs,omitempty"`
	ReceiptRefs        []Ref             `json:"receipt_refs,omitempty"`
	Locks              []string          `json:"locks,omitempty"`
	Outcome            string            `json:"outcome,omitempty"`
	CostSummary        CostSummary       `json:"cost_summary,omitempty"`
	StartedAt          time.Time         `json:"started_at"`
	FinishedAt         time.Time         `json:"finished_at,omitempty"`
	Duration           int64             `json:"duration_ms,omitempty"`
	Status             string            `json:"status,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

func NewTrajectory(sessionID, objectiveSummary, modeProfile string) Trajectory {
	now := time.Now().UTC()
	return Trajectory{
		TrajectoryID:     NewTrajectoryID(),
		SessionID:        RedactString(sessionID, maxTrajectoryRefSize),
		ObjectiveSummary: RedactString(objectiveSummary, maxMetadataValueLen),
		ObjectiveHash:    StableHash(objectiveSummary),
		ModeProfile:      RedactString(modeProfile, maxMetadataValueLen),
		StartedAt:        now,
	}
}

func (t Trajectory) Normalized() Trajectory {
	out := t
	if strings.TrimSpace(out.TrajectoryID) == "" {
		out.TrajectoryID = NewTrajectoryID()
	}
	if out.StartedAt.IsZero() {
		out.StartedAt = time.Now().UTC()
	}
	out.SessionID = RedactString(out.SessionID, maxTrajectoryRefSize)
	out.ParentTrajectoryID = RedactString(out.ParentTrajectoryID, maxTrajectoryRefSize)
	out.ObjectiveHash = RedactString(out.ObjectiveHash, 128)
	out.ObjectiveSummary = RedactString(out.ObjectiveSummary, maxMetadataValueLen)
	out.ModeProfile = RedactString(out.ModeProfile, maxMetadataValueLen)
	out.StartStateHash = RedactString(out.StartStateHash, 128)
	out.EndStateHash = RedactString(out.EndStateHash, 128)
	out.Outcome = RedactString(out.Outcome, maxMetadataValueLen)
	out.Status = RedactString(out.Status, maxStatusLen)
	out.Metadata = BoundMetadata(out.Metadata)
	out.ArtifactRefs = boundRefs(out.ArtifactRefs)
	out.ValidationRefs = boundRefs(out.ValidationRefs)
	out.DecisionRefs = boundRefs(out.DecisionRefs)
	out.ReceiptRefs = boundRefs(out.ReceiptRefs)

	if len(out.EventRefs) > maxEventRefs {
		out.EventRefs = out.EventRefs[:maxEventRefs]
	}
	for i := range out.EventRefs {
		out.EventRefs[i] = RedactString(out.EventRefs[i], maxTrajectoryRefSize)
	}
	if len(out.OperatorPath) > maxOperatorPath {
		out.OperatorPath = out.OperatorPath[:maxOperatorPath]
	}
	for i := range out.OperatorPath {
		if !out.OperatorPath[i].Valid() {
			out.OperatorPath[i] = OperatorUnknown
		}
	}
	if out.Duration < 0 {
		out.Duration = 0
	}
	return out
}

func (t Trajectory) Validate() error {
	if strings.TrimSpace(t.TrajectoryID) == "" {
		return errors.New("trajectory requires trajectory_id")
	}
	if t.StartedAt.IsZero() {
		return errors.New("trajectory requires started_at")
	}
	if len(t.EventRefs) > maxEventRefs {
		return errors.New("trajectory event refs exceed bound")
	}
	if len(t.OperatorPath) > maxOperatorPath {
		return errors.New("trajectory operator path exceeds bound")
	}
	for i := range t.OperatorPath {
		if !t.OperatorPath[i].Valid() {
			return errors.New("trajectory contains invalid operator")
		}
	}
	return nil
}

func LinkEventToTrajectory(e Event, trajectoryID, parentEventID string, op Operator) Event {
	out := e
	out.TrajectoryID = RedactString(trajectoryID, maxTrajectoryRefSize)
	out.ParentEventID = RedactString(parentEventID, maxTrajectoryRefSize)
	if op.Valid() {
		out.Operator = op
	} else if !out.Operator.Valid() {
		out.Operator = OperatorUnknown
	}
	return out
}

func AppendOperatorPath(path []Operator, op Operator) []Operator {
	if !op.Valid() {
		op = OperatorUnknown
	}
	if len(path) >= maxOperatorPath {
		return path
	}
	return append(path, op)
}

func FinalizeTrajectory(t Trajectory, endStateHash, outcome, status string, finishedAt time.Time, summary CostSummary) Trajectory {
	out := t
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	out.FinishedAt = finishedAt
	if out.StartedAt.IsZero() {
		out.StartedAt = finishedAt
	}
	if out.FinishedAt.Before(out.StartedAt) {
		out.FinishedAt = out.StartedAt
	}
	out.Duration = out.FinishedAt.Sub(out.StartedAt).Milliseconds()
	out.EndStateHash = RedactString(endStateHash, 128)
	out.Outcome = RedactString(outcome, maxMetadataValueLen)
	out.Status = RedactString(status, maxStatusLen)
	out.CostSummary = summary
	return out.Normalized()
}
