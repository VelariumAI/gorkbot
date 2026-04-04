package distributed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// InMemoryEventSource stores events in memory (for testing and small deployments).
type InMemoryEventSource struct {
	mu       sync.RWMutex
	events   []*EventStoreEntry
	sequence int64
	logger   *slog.Logger
}

// NewInMemoryEventSource creates a new in-memory event store.
func NewInMemoryEventSource(logger *slog.Logger) *InMemoryEventSource {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(nil, nil))
	}
	return &InMemoryEventSource{
		events:   make([]*EventStoreEntry, 0),
		sequence: 0,
		logger:   logger,
	}
}

// Store persists an event in memory.
func (s *InMemoryEventSource) Store(ctx context.Context, entry *EventStoreEntry) (int64, error) {
	if entry == nil {
		return 0, fmt.Errorf("nil entry")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequence++
	entry.Sequence = s.sequence
	entry.Hash = s.computeHash(entry)

	s.events = append(s.events, entry)
	return s.sequence, nil
}

// Load retrieves an event by ID.
func (s *InMemoryEventSource) Load(ctx context.Context, eventID string) (*EventStoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, e := range s.events {
		if e.EventId == eventID {
			return e, nil
		}
	}

	return nil, fmt.Errorf("event not found: %s", eventID)
}

// Query returns events matching criteria.
func (s *InMemoryEventSource) Query(ctx context.Context, eventType, correlationID string, sinceMS int64) ([]*EventStoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*EventStoreEntry, 0)

	for _, e := range s.events {
		match := true

		if eventType != "" && e.EventType != eventType {
			match = false
		}
		if correlationID != "" && e.CorrelationId != correlationID {
			match = false
		}
		if sinceMS > 0 && e.TimestampMs < sinceMS {
			match = false
		}

		if match {
			result = append(result, e)
		}
	}

	return result, nil
}

// Tail returns the last N events.
func (s *InMemoryEventSource) Tail(ctx context.Context, limit int) ([]*EventStoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	start := len(s.events) - limit
	if start < 0 {
		start = 0
	}

	result := make([]*EventStoreEntry, len(s.events[start:]))
	copy(result, s.events[start:])
	return result, nil
}

// Size returns the total number of events.
func (s *InMemoryEventSource) Size(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.events)), nil
}

// Hash returns the hash of all events.
func (s *InMemoryEventSource) Hash(ctx context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h := sha256.New()

	for _, e := range s.events {
		h.Write([]byte(e.EventId))
		h.Write([]byte(e.EventType))
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// Compact removes old events.
func (s *InMemoryEventSource) Compact(ctx context.Context, keepMS int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UnixMilli() - keepMS

	kept := make([]*EventStoreEntry, 0)
	for _, e := range s.events {
		if e.TimestampMs >= cutoff {
			kept = append(kept, e)
		}
	}

	removed := len(s.events) - len(kept)
	s.events = kept
	s.logger.Info("compacted events", "removed", removed, "remaining", len(s.events))

	return nil
}

// Close releases resources (no-op for in-memory).
func (s *InMemoryEventSource) Close() error {
	return nil
}

// computeHash computes the hash of an event for integrity.
func (s *InMemoryEventSource) computeHash(entry *EventStoreEntry) string {
	h := sha256.New()
	h.Write([]byte(entry.EventId))
	h.Write([]byte(entry.EventType))
	h.Write(entry.Payload)
	h.Write([]byte(entry.CorrelationId))
	return hex.EncodeToString(h.Sum(nil))
}
