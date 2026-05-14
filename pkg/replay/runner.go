package replay

import (
	"context"
	"fmt"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type CandidateEvaluator interface {
	Evaluate(ctx context.Context, c Case) (CandidateOutcome, error)
}

type Runner struct {
	Comparator Comparator
	Evaluator  CandidateEvaluator
}

func (r Runner) Run(ctx context.Context, c Case) (Result, error) {
	started := time.Now().UTC()

	norm := c.Normalized()
	if err := norm.Validate(); err != nil {
		finished := time.Now().UTC()
		res := Result{
			CaseID:       norm.ID,
			TrajectoryID: norm.TrajectoryID,
			CandidateID:  norm.Candidate.ID,
			Verdict:      VerdictInvalid,
			Regressions: []Regression{{
				Code:    "invalid_case",
				Message: err.Error(),
			}},
			StartedAt:  started,
			FinishedAt: finished,
			Duration:   finished.Sub(started),
		}
		return res, fmt.Errorf("%w: %v", ErrInvalidCase, err)
	}

	evaluator := r.Evaluator
	if evaluator == nil {
		evaluator = BaselineEvaluator{}
	}
	candidate, err := evaluator.Evaluate(ctx, norm)
	if err != nil {
		finished := time.Now().UTC()
		res := Result{
			CaseID:       norm.ID,
			TrajectoryID: norm.TrajectoryID,
			CandidateID:  norm.Candidate.ID,
			Verdict:      VerdictInconclusive,
			Regressions: []Regression{{
				Code:    "candidate_evaluation_failed",
				Message: err.Error(),
			}},
			StartedAt:  started,
			FinishedAt: finished,
			Duration:   finished.Sub(started),
		}
		return res, err
	}
	if candidate.Trajectory.TrajectoryID == "" {
		candidate.Trajectory = norm.Baseline
	}

	cmpImpl := r.Comparator
	if cmpImpl == nil {
		cmpImpl = DeterministicComparator{}
	}
	cmp := cmpImpl.Compare(norm, candidate)
	finished := time.Now().UTC()

	res := Result{
		CaseID:           norm.ID,
		TrajectoryID:     norm.TrajectoryID,
		CandidateID:      norm.Candidate.ID,
		BaselineOutcome:  cmp.BaselineOutcome,
		CandidateOutcome: cmp.CandidateOutcome,
		Regressions:      cmp.Regressions,
		Improvements:     cmp.Improvements,
		Verdict:          cmp.Verdict,
		StartedAt:        started,
		FinishedAt:       finished,
		Duration:         finished.Sub(started),
	}
	return res, nil
}

type BaselineEvaluator struct{}

func (BaselineEvaluator) Evaluate(_ context.Context, c Case) (CandidateOutcome, error) {
	return CandidateOutcome{
		Trajectory: c.Baseline,
		Events:     append([]trace.Event(nil), c.BaselineEvents...),
	}, nil
}

type NoopEvaluator struct{}

func (NoopEvaluator) Evaluate(_ context.Context, c Case) (CandidateOutcome, error) {
	return CandidateOutcome{
		Trajectory: c.Baseline,
	}, nil
}

type StaticEvaluator struct {
	Outcome CandidateOutcome
	Err     error
}

func (s StaticEvaluator) Evaluate(_ context.Context, _ Case) (CandidateOutcome, error) {
	if s.Err != nil {
		return CandidateOutcome{}, s.Err
	}
	return s.Outcome, nil
}

type FixtureEvaluator struct {
	Fixtures map[string]CandidateOutcome
}

func (f FixtureEvaluator) Evaluate(_ context.Context, c Case) (CandidateOutcome, error) {
	if out, ok := f.Fixtures[c.ID]; ok {
		return out, nil
	}
	return CandidateOutcome{}, ErrInconclusive
}
