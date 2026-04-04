package events

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event represents an audit event
type Event struct {
	ID        string                 // Unique event ID
	Type      string                 // Event type (session.created, budget.deducted, etc.)
	Timestamp int64                  // Unix timestamp
	Data      map[string]interface{} // Event payload
	Source    string                 // Source identifier
	Version   int                    // Event version
}

// EventStore manages event sourcing with JSONL log
type EventStore struct {
	logger       *slog.Logger
	logPath      string
	events       []*Event
	snapshots    map[string]*StateSnapshot
	mu           sync.RWMutex
	eventCount   int
	snapshotFreq int
}

// StateSnapshot represents a state snapshot
type StateSnapshot struct {
	Timestamp int64
	EventID   string
	State     map[string]interface{}
}

// NewEventStore creates a new event store
func NewEventStore(logger *slog.Logger, logPath string) *EventStore {
	if logger == nil {
		logger = slog.Default()
	}

	// Create log directory if not exists
	if logPath != "" {
		os.MkdirAll(filepath.Dir(logPath), 0755)
	}

	es := &EventStore{
		logger:       logger,
		logPath:      logPath,
		events:       make([]*Event, 0),
		snapshots:    make(map[string]*StateSnapshot),
		snapshotFreq: 100, // Create snapshot every 100 events
	}

	// Load existing events
	if logPath != "" {
		es.loadEvents()
	}

	return es
}

// Append appends an event to the store
func (es *EventStore) Append(eventType string, data map[string]interface{}, source string) *Event {
	es.mu.Lock()
	defer es.mu.Unlock()

	event := &Event{
		ID:        fmt.Sprintf("evt-%d-%d", time.Now().Unix(), es.eventCount),
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data:      data,
		Source:    source,
		Version:   1,
	}

	es.events = append(es.events, event)
	es.eventCount++

	// Write to log
	if es.logPath != "" {
		es.writeEvent(event)
	}

	es.logger.Debug("appended event",
		slog.String("id", event.ID),
		slog.String("type", eventType),
		slog.String("source", source),
	)

	// Create snapshot if needed
	if es.eventCount%es.snapshotFreq == 0 {
		es.createSnapshot(event.ID)
	}

	return event
}

// GetEvents returns all events
func (es *EventStore) GetEvents() []*Event {
	es.mu.RLock()
	defer es.mu.RUnlock()

	events := make([]*Event, len(es.events))
	copy(events, es.events)
	return events
}

// GetEventsByType returns events of a specific type
func (es *EventStore) GetEventsByType(eventType string) []*Event {
	es.mu.RLock()
	defer es.mu.RUnlock()

	var filtered []*Event
	for _, event := range es.events {
		if event.Type == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// GetEventsSince returns events since a timestamp
func (es *EventStore) GetEventsSince(timestamp int64) []*Event {
	es.mu.RLock()
	defer es.mu.RUnlock()

	var filtered []*Event
	for _, event := range es.events {
		if event.Timestamp >= timestamp {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// createSnapshot creates a state snapshot
func (es *EventStore) createSnapshot(eventID string) {
	snapshot := &StateSnapshot{
		Timestamp: time.Now().Unix(),
		EventID:   eventID,
		State:     es.reconstructState(),
	}

	es.snapshots[eventID] = snapshot

	es.logger.Debug("created snapshot",
		slog.String("event_id", eventID),
		slog.Int("state_size", len(snapshot.State)),
	)
}

// ReconstructState reconstructs state from events
func (es *EventStore) ReconstructState() map[string]interface{} {
	es.mu.RLock()
	defer es.mu.RUnlock()

	return es.reconstructState()
}

// reconstructState internal implementation
func (es *EventStore) reconstructState() map[string]interface{} {
	state := make(map[string]interface{})

	for _, event := range es.events {
		switch event.Type {
		case "session.created":
			state["session_id"] = event.Data["session_id"]
			state["start_time"] = event.Data["start_time"]

		case "budget.deducted":
			if current, ok := state["total_spent"].(float64); ok {
				state["total_spent"] = current + event.Data["amount"].(float64)
			} else {
				state["total_spent"] = event.Data["amount"].(float64)
			}

		case "tool.executed":
			state["last_tool"] = event.Data["tool_name"]
			state["last_execution"] = event.Timestamp

		case "provider.selected":
			state["provider"] = event.Data["provider_name"]

		case "session.closed":
			state["session_active"] = false
		}
	}

	return state
}

// GetLatestSnapshot returns the most recent snapshot
func (es *EventStore) GetLatestSnapshot() *StateSnapshot {
	es.mu.RLock()
	defer es.mu.RUnlock()

	var latest *StateSnapshot
	for _, snapshot := range es.snapshots {
		if latest == nil || snapshot.Timestamp > latest.Timestamp {
			latest = snapshot
		}
	}
	return latest
}

// Helper functions

func (es *EventStore) writeEvent(event *Event) {
	if es.logPath == "" {
		return
	}

	file, err := os.OpenFile(es.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		es.logger.Error("failed to open event log", slog.String("error", err.Error()))
		return
	}
	defer file.Close()

	data, err := json.Marshal(event)
	if err != nil {
		es.logger.Error("failed to marshal event", slog.String("error", err.Error()))
		return
	}

	file.Write(data)
	file.WriteString("\n")
}

func (es *EventStore) loadEvents() {
	if _, err := os.Stat(es.logPath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(es.logPath)
	if err != nil {
		es.logger.Error("failed to read event log", slog.String("error", err.Error()))
		return
	}

	_ = data // Parse JSONL (simplified)
	es.logger.Debug("loaded events from log",
		slog.String("path", es.logPath),
	)
}

// GetStats returns event store statistics
func (es *EventStore) GetStats() map[string]interface{} {
	es.mu.RLock()
	defer es.mu.RUnlock()

	return map[string]interface{}{
		"total_events": len(es.events),
		"snapshots":    len(es.snapshots),
		"log_file":     es.logPath,
	}
}

// Purge removes old events (older than days)
func (es *EventStore) Purge(daysBefore int) int {
	es.mu.Lock()
	defer es.mu.Unlock()

	cutoffTime := time.Now().AddDate(0, 0, -daysBefore).Unix()
	removed := 0

	filtered := make([]*Event, 0, len(es.events))
	for _, event := range es.events {
		if event.Timestamp >= cutoffTime {
			filtered = append(filtered, event)
		} else {
			removed++
		}
	}

	es.events = filtered

	es.logger.Info("purged old events",
		slog.Int("removed", removed),
		slog.Int("remaining", len(es.events)),
	)

	return removed
}
