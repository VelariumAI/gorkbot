package replay

import (
	"fmt"
	"sort"
	"strings"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type CandidateKind string

const (
	CandidateKindPolicy  CandidateKind = "policy"
	CandidateKindSkill   CandidateKind = "skill"
	CandidateKindHarness CandidateKind = "harness"
	CandidateKindRoute   CandidateKind = "route"
	CandidateKindProfile CandidateKind = "profile"
	CandidateKindUnknown CandidateKind = "unknown"
)

var validCandidateKinds = map[CandidateKind]struct{}{
	CandidateKindPolicy:  {},
	CandidateKindSkill:   {},
	CandidateKindHarness: {},
	CandidateKindRoute:   {},
	CandidateKindProfile: {},
	CandidateKindUnknown: {},
}

type CandidateSpec struct {
	ID          string            `json:"id"`
	Kind        CandidateKind     `json:"kind"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (c CandidateSpec) Normalized() CandidateSpec {
	out := c
	out.ID = truncate(strings.TrimSpace(out.ID), 128)
	out.Version = truncate(strings.TrimSpace(out.Version), 128)
	out.Description = truncate(strings.TrimSpace(out.Description), 256)
	if _, ok := validCandidateKinds[out.Kind]; !ok {
		out.Kind = CandidateKindUnknown
	}
	out.Metadata = trace.BoundMetadata(out.Metadata)
	return out
}

func (c CandidateSpec) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: candidate id required", ErrInvalidCase)
	}
	if _, ok := validCandidateKinds[c.Kind]; !ok {
		return fmt.Errorf("%w: candidate kind %q", ErrUnsupportedCandidate, c.Kind)
	}
	return nil
}

type Expectations struct {
	RequiredOperators     []trace.Operator `json:"required_operators,omitempty"`
	ForbiddenOperators    []trace.Operator `json:"forbidden_operators,omitempty"`
	RequiredEventKinds    []string         `json:"required_event_kinds,omitempty"`
	ForbiddenEventKinds   []string         `json:"forbidden_event_kinds,omitempty"`
	MaxCostIncreaseMicros int64            `json:"max_cost_increase_micros,omitempty"`
	MaxDurationIncreaseMS int64            `json:"max_duration_increase_ms,omitempty"`
}

func (e Expectations) Normalized() Expectations {
	out := e
	out.RequiredOperators = normalizeOperators(out.RequiredOperators)
	out.ForbiddenOperators = normalizeOperators(out.ForbiddenOperators)
	out.RequiredEventKinds = normalizeKinds(out.RequiredEventKinds)
	out.ForbiddenEventKinds = normalizeKinds(out.ForbiddenEventKinds)
	if out.MaxCostIncreaseMicros < 0 {
		out.MaxCostIncreaseMicros = 0
	}
	if out.MaxDurationIncreaseMS < 0 {
		out.MaxDurationIncreaseMS = 0
	}
	return out
}

type Case struct {
	ID             string            `json:"id"`
	Name           string            `json:"name,omitempty"`
	TrajectoryID   string            `json:"trajectory_id"`
	Baseline       trace.Trajectory  `json:"baseline"`
	BaselineEvents []trace.Event     `json:"baseline_events,omitempty"`
	Candidate      CandidateSpec     `json:"candidate"`
	Expectations   Expectations      `json:"expectations,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func (c Case) Normalized() Case {
	out := c
	out.ID = truncate(strings.TrimSpace(out.ID), 128)
	out.Name = truncate(strings.TrimSpace(out.Name), 256)
	out.TrajectoryID = truncate(strings.TrimSpace(out.TrajectoryID), 256)
	out.Baseline = sanitizeTrajectory(out.Baseline)
	out.Candidate = out.Candidate.Normalized()
	out.Expectations = out.Expectations.Normalized()
	out.Metadata = trace.BoundMetadata(out.Metadata)
	out.BaselineEvents = normalizeEvents(out.BaselineEvents)
	if out.TrajectoryID == "" {
		out.TrajectoryID = out.Baseline.TrajectoryID
	}
	if out.ID == "" && out.TrajectoryID != "" {
		out.ID = "case_" + truncate(trace.StableHash(out.TrajectoryID, out.Candidate.ID), 32)
	}
	return out
}

func (c Case) Validate() error {
	if strings.TrimSpace(c.Baseline.TrajectoryID) == "" {
		return ErrMissingTrajectory
	}
	if strings.TrimSpace(c.TrajectoryID) == "" {
		return ErrMissingTrajectory
	}
	if err := c.Candidate.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: case id required", ErrInvalidCase)
	}
	if c.TrajectoryID != c.Baseline.TrajectoryID {
		return fmt.Errorf("%w: trajectory id mismatch", ErrInvalidCase)
	}
	if err := c.Baseline.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCase, err)
	}
	return nil
}

func NewCaseFromTrajectory(name string, baseline trace.Trajectory, candidate CandidateSpec, expectations Expectations) (Case, error) {
	return CaseFromTrajectory("", name, baseline, candidate, expectations, nil)
}

func CaseFromTrajectory(id, name string, baseline trace.Trajectory, candidate CandidateSpec, expectations Expectations, metadata map[string]string) (Case, error) {
	c := Case{
		ID:           id,
		Name:         name,
		TrajectoryID: baseline.TrajectoryID,
		Baseline:     baseline,
		Candidate:    candidate,
		Expectations: expectations,
		Metadata:     metadata,
	}
	c = c.Normalized()
	if err := c.Validate(); err != nil {
		return Case{}, err
	}
	return c, nil
}

func TrajectorySummaryToCase(id string, baseline trace.Trajectory, candidate CandidateSpec) (Case, error) {
	return CaseFromTrajectory(id, "", baseline, candidate, Expectations{}, nil)
}

func normalizeOperators(in []trace.Operator) []trace.Operator {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[trace.Operator]struct{}, len(in))
	out := make([]trace.Operator, 0, len(in))
	for i := range in {
		op := in[i]
		if !op.Valid() {
			op = trace.OperatorUnknown
		}
		if _, ok := seen[op]; ok {
			continue
		}
		seen[op] = struct{}{}
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

func normalizeKinds(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for i := range in {
		k := strings.ToLower(strings.TrimSpace(in[i]))
		if k == "" {
			continue
		}
		k = truncate(k, 64)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func normalizeEvents(in []trace.Event) []trace.Event {
	if len(in) == 0 {
		return nil
	}
	out := make([]trace.Event, 0, len(in))
	for i := range in {
		ev := in[i].Normalized()
		if ev.EventID == "" {
			continue
		}
		out = append(out, ev)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp.Equal(out[j].Timestamp) {
			return out[i].EventID < out[j].EventID
		}
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

func sanitizeTrajectory(in trace.Trajectory) trace.Trajectory {
	out := in
	out.TrajectoryID = truncate(strings.TrimSpace(out.TrajectoryID), 256)
	out.SessionID = trace.RedactString(out.SessionID, 256)
	out.ParentTrajectoryID = trace.RedactString(out.ParentTrajectoryID, 256)
	out.ObjectiveHash = trace.RedactString(out.ObjectiveHash, 128)
	out.ObjectiveSummary = trace.RedactString(out.ObjectiveSummary, 256)
	out.ModeProfile = trace.RedactString(out.ModeProfile, 256)
	out.StartStateHash = trace.RedactString(out.StartStateHash, 128)
	out.EndStateHash = trace.RedactString(out.EndStateHash, 128)
	out.Outcome = trace.RedactString(out.Outcome, 256)
	out.Status = trace.RedactString(out.Status, 64)
	out.Metadata = trace.BoundMetadata(out.Metadata)
	out.OperatorPath = normalizeOperators(out.OperatorPath)
	if len(out.EventRefs) > 256 {
		out.EventRefs = out.EventRefs[:256]
	}
	for i := range out.EventRefs {
		out.EventRefs[i] = trace.RedactString(out.EventRefs[i], 256)
	}
	if out.Duration < 0 {
		out.Duration = 0
	}
	return out
}

func truncate(in string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(in) <= n {
		return in
	}
	return in[:n]
}
