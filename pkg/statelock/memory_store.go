package statelock

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type MemoryStore struct {
	mu        sync.RWMutex
	locks     map[string]Lock
	paradoxes map[string]ParadoxReport
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		locks:     make(map[string]Lock),
		paradoxes: make(map[string]ParadoxReport),
	}
}

func (s *MemoryStore) SaveLock(_ context.Context, lock Lock) error {
	if s == nil {
		return ErrStoreUnavailable
	}
	n := lock.Normalized()
	if err := n.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locks[n.ID] = cloneLock(n)
	return nil
}

func (s *MemoryStore) LoadLock(_ context.Context, id string) (Lock, error) {
	if s == nil {
		return Lock{}, ErrStoreUnavailable
	}
	cleanID := strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.locks[cleanID]
	if !ok {
		return Lock{}, fmt.Errorf("%w: lock %q", ErrNotFound, cleanID)
	}
	return cloneLock(item), nil
}

func (s *MemoryStore) ListLocks(_ context.Context, filter Filter) ([]Lock, error) {
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Lock, 0, len(s.locks))
	for _, item := range s.locks {
		n := item.Normalized()
		if filter.Scope != "" && n.Scope != NormalizeScope(string(filter.Scope)) {
			continue
		}
		if filter.Dimension != "" && n.Dimension != NormalizeDimension(string(filter.Dimension)) {
			continue
		}
		if filter.Subject != "" && n.Subject != strings.TrimSpace(filter.Subject) {
			continue
		}
		if filter.Status != "" && n.Status != normalizeStatus(string(filter.Status)) {
			continue
		}
		items = append(items, cloneLock(n))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Scope == items[j].Scope {
			if items[i].Dimension == items[j].Dimension {
				if items[i].Subject == items[j].Subject {
					return items[i].ID < items[j].ID
				}
				return items[i].Subject < items[j].Subject
			}
			return items[i].Dimension < items[j].Dimension
		}
		return items[i].Scope < items[j].Scope
	})
	return items, nil
}

func (s *MemoryStore) SaveParadox(_ context.Context, report ParadoxReport) error {
	if s == nil {
		return ErrStoreUnavailable
	}
	n := report.Normalized()
	if err := n.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paradoxes[n.ID] = cloneParadoxReport(n)
	return nil
}

func (s *MemoryStore) LoadParadox(_ context.Context, id string) (ParadoxReport, error) {
	if s == nil {
		return ParadoxReport{}, ErrStoreUnavailable
	}
	cleanID := strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.paradoxes[cleanID]
	if !ok {
		return ParadoxReport{}, fmt.Errorf("%w: paradox %q", ErrNotFound, cleanID)
	}
	return cloneParadoxReport(item), nil
}

func cloneParadoxReport(in ParadoxReport) ParadoxReport {
	out := in
	out.Conflicts = append([]Conflict(nil), in.Conflicts...)
	out.Constraints = append([]Constraint(nil), in.Constraints...)
	out.EvidenceRefs = cloneRefs(in.EvidenceRefs)
	out.Remediation = append([]Remediation(nil), in.Remediation...)
	out.Metadata = cloneStringMap(in.Metadata)
	return out
}
