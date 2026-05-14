package replay

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type MemoryStore struct {
	mu      sync.RWMutex
	cases   map[string]Case
	results map[string]Result
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		cases:   make(map[string]Case),
		results: make(map[string]Result),
	}
}

func (s *MemoryStore) SaveCase(_ context.Context, c Case) error {
	norm := c.Normalized()
	if err := norm.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cases[norm.ID] = cloneCase(norm)
	return nil
}

func (s *MemoryStore) LoadCase(_ context.Context, id string) (Case, error) {
	key := strings.TrimSpace(id)
	if key == "" {
		return Case{}, fmt.Errorf("%w: case id required", ErrInvalidCase)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cases[key]
	if !ok {
		return Case{}, fmt.Errorf("%w: case %q not found", ErrInvalidCase, key)
	}
	return cloneCase(c), nil
}

func (s *MemoryStore) ListCases(_ context.Context) ([]CaseSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CaseSummary, 0, len(s.cases))
	for _, c := range s.cases {
		out = append(out, CaseSummary{
			ID:           c.ID,
			Name:         c.Name,
			TrajectoryID: c.TrajectoryID,
			CandidateID:  c.Candidate.ID,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *MemoryStore) SaveResult(_ context.Context, r Result) error {
	if strings.TrimSpace(r.CaseID) == "" {
		return fmt.Errorf("%w: result case id required", ErrInvalidCase)
	}
	norm := cloneResult(r)
	norm.BaselineOutcome = normalizeOutcome(norm.BaselineOutcome)
	norm.CandidateOutcome = normalizeOutcome(norm.CandidateOutcome)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[r.CaseID] = norm
	return nil
}

func (s *MemoryStore) LoadResult(_ context.Context, id string) (Result, error) {
	key := strings.TrimSpace(id)
	if key == "" {
		return Result{}, fmt.Errorf("%w: result id required", ErrInvalidCase)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.results[key]
	if !ok {
		return Result{}, fmt.Errorf("%w: result %q not found", ErrInvalidCase, key)
	}
	return cloneResult(r), nil
}

func cloneCase(in Case) Case {
	out := in
	out.Metadata = cloneStringMap(in.Metadata)
	out.Candidate.Metadata = cloneStringMap(in.Candidate.Metadata)
	out.Baseline.Metadata = cloneStringMap(in.Baseline.Metadata)
	out.Baseline.OperatorPath = append([]trace.Operator(nil), in.Baseline.OperatorPath...)
	out.Baseline.EventRefs = append([]string(nil), in.Baseline.EventRefs...)
	out.Baseline.ArtifactRefs = append([]trace.Ref(nil), in.Baseline.ArtifactRefs...)
	out.Baseline.ValidationRefs = append([]trace.Ref(nil), in.Baseline.ValidationRefs...)
	out.Baseline.DecisionRefs = append([]trace.Ref(nil), in.Baseline.DecisionRefs...)
	out.Baseline.ReceiptRefs = append([]trace.Ref(nil), in.Baseline.ReceiptRefs...)
	out.Baseline.Locks = append([]string(nil), in.Baseline.Locks...)
	out.BaselineEvents = append([]trace.Event(nil), in.BaselineEvents...)
	out.Expectations.RequiredOperators = append([]trace.Operator(nil), in.Expectations.RequiredOperators...)
	out.Expectations.ForbiddenOperators = append([]trace.Operator(nil), in.Expectations.ForbiddenOperators...)
	out.Expectations.RequiredEventKinds = append([]string(nil), in.Expectations.RequiredEventKinds...)
	out.Expectations.ForbiddenEventKinds = append([]string(nil), in.Expectations.ForbiddenEventKinds...)
	return out
}

func cloneResult(in Result) Result {
	out := in
	out.Regressions = append([]Regression(nil), in.Regressions...)
	out.Improvements = append([]Improvement(nil), in.Improvements...)
	out.BaselineOutcome.OperatorPath = append([]trace.Operator(nil), in.BaselineOutcome.OperatorPath...)
	out.BaselineOutcome.EventKinds = append([]string(nil), in.BaselineOutcome.EventKinds...)
	out.BaselineOutcome.ValidationRefs = append([]trace.Ref(nil), in.BaselineOutcome.ValidationRefs...)
	out.BaselineOutcome.ReceiptRefs = append([]trace.Ref(nil), in.BaselineOutcome.ReceiptRefs...)
	out.CandidateOutcome.OperatorPath = append([]trace.Operator(nil), in.CandidateOutcome.OperatorPath...)
	out.CandidateOutcome.EventKinds = append([]string(nil), in.CandidateOutcome.EventKinds...)
	out.CandidateOutcome.ValidationRefs = append([]trace.Ref(nil), in.CandidateOutcome.ValidationRefs...)
	out.CandidateOutcome.ReceiptRefs = append([]trace.Ref(nil), in.CandidateOutcome.ReceiptRefs...)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
