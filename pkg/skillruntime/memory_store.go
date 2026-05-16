package skillruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/trace"
)

type MemoryStore struct {
	mu         sync.RWMutex
	candidates map[string]Candidate
	results    map[string]Result
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		candidates: make(map[string]Candidate),
		results:    make(map[string]Result),
	}
}

func (s *MemoryStore) SaveCandidate(_ context.Context, candidate Candidate) error {
	if s == nil {
		return fmt.Errorf("%w: store unavailable", ErrInvalidRequest)
	}
	n := candidate.Normalized()
	if err := n.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.candidates[n.ID] = cloneCandidate(n)
	return nil
}

func (s *MemoryStore) LoadCandidate(_ context.Context, id string) (Candidate, error) {
	if s == nil {
		return Candidate{}, fmt.Errorf("%w: store unavailable", ErrInvalidRequest)
	}
	key := boundID(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.candidates[key]
	if !ok {
		return Candidate{}, fmt.Errorf("%w: candidate %q", ErrNotFound, key)
	}
	return cloneCandidate(c), nil
}

func (s *MemoryStore) ListCandidates(_ context.Context) ([]Candidate, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: store unavailable", ErrInvalidRequest)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Candidate, 0, len(s.candidates))
	for _, c := range s.candidates {
		out = append(out, cloneCandidate(c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *MemoryStore) SaveResult(_ context.Context, result Result) error {
	if s == nil {
		return fmt.Errorf("%w: store unavailable", ErrInvalidRequest)
	}
	n := result.Normalized()
	if err := n.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[n.ID] = cloneResult(n)
	return nil
}

func (s *MemoryStore) LoadResult(_ context.Context, id string) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("%w: store unavailable", ErrInvalidRequest)
	}
	key := boundID(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.results[key]
	if !ok {
		return Result{}, fmt.Errorf("%w: result %q", ErrNotFound, key)
	}
	return cloneResult(r), nil
}

func (s *MemoryStore) DisableCandidate(_ context.Context, id string) error {
	if s == nil {
		return fmt.Errorf("%w: store unavailable", ErrInvalidRequest)
	}
	key := strings.TrimSpace(id)
	if key == "" {
		return fmt.Errorf("%w: candidate id required", ErrInvalidCandidate)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.candidates[key]
	if !ok {
		return fmt.Errorf("%w: candidate %q", ErrNotFound, key)
	}
	c.Disabled = true
	s.candidates[key] = cloneCandidate(c)
	return nil
}

func cloneCandidate(in Candidate) Candidate {
	out := in
	out.ArtifactRefs = append([]trace.Ref(nil), in.ArtifactRefs...)
	out.EvidenceRefs = append([]trace.Ref(nil), in.EvidenceRefs...)
	out.Metadata = cloneMetadata(in.Metadata)
	return out
}

func cloneResult(in Result) Result {
	out := in
	out.Assessment.EvidenceRefs = append([]trace.Ref(nil), in.Assessment.EvidenceRefs...)
	out.Assessment.Metadata = cloneMetadata(in.Assessment.Metadata)
	out.Receipt = in.Receipt
	out.Receipt.Records = append([]evidence.Record(nil), in.Receipt.Records...)
	out.Receipt.EvidenceRefs = append([]trace.Ref(nil), in.Receipt.EvidenceRefs...)
	out.Receipt.Metadata = cloneMetadata(in.Receipt.Metadata)
	out.Receipt.Assessment.EvidenceRefs = append([]trace.Ref(nil), in.Receipt.Assessment.EvidenceRefs...)
	out.Receipt.Assessment.Metadata = cloneMetadata(in.Receipt.Assessment.Metadata)
	out.ValidationRefs = append([]trace.Ref(nil), in.ValidationRefs...)
	out.ArtifactRefs = append([]trace.Ref(nil), in.ArtifactRefs...)
	out.Warnings = append([]string(nil), in.Warnings...)
	out.Metadata = cloneMetadata(in.Metadata)
	return out
}
