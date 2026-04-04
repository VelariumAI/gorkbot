package distributed

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteEventSource persists events to SQLite for durability.
type SQLiteEventSource struct {
	db     *sql.DB
	mu     sync.RWMutex
	logger *slog.Logger
	dbPath string
}

// NewSQLiteEventSource creates a new SQLite-backed event store.
func NewSQLiteEventSource(dbPath string, logger *slog.Logger) (*SQLiteEventSource, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(nil, nil))
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	s := &SQLiteEventSource{
		db:     db,
		logger: logger,
		dbPath: dbPath,
	}

	// Create table if not exists
	if err := s.createSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	logger.Info("SQLite event source initialized", "path", dbPath)
	return s, nil
}

// createSchema creates the events table.
func (s *SQLiteEventSource) createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		event_id TEXT PRIMARY KEY,
		sequence INTEGER UNIQUE NOT NULL,
		timestamp_ms INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		correlation_id TEXT NOT NULL,
		payload BLOB NOT NULL,
		source_node TEXT,
		hash TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_correlation_id ON events(correlation_id);
	CREATE INDEX IF NOT EXISTS idx_event_type ON events(event_type);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON events(timestamp_ms);
	CREATE INDEX IF NOT EXISTS idx_sequence ON events(sequence DESC);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Store persists an event to SQLite.
func (s *SQLiteEventSource) Store(ctx context.Context, entry *EventStoreEntry) (int64, error) {
	if entry == nil {
		return 0, fmt.Errorf("nil entry")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get next sequence
	var maxSeq int64
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence), 0) FROM events").Scan(&maxSeq)
	if err != nil {
		return 0, fmt.Errorf("get max sequence: %w", err)
	}

	sequence := maxSeq + 1
	entry.Sequence = sequence
	entry.Hash = s.computeHash(entry)

	query := `
	INSERT INTO events (event_id, sequence, timestamp_ms, event_type, correlation_id, payload, source_node, hash)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		entry.EventId, sequence, entry.TimestampMs, entry.EventType, entry.CorrelationId,
		entry.Payload, entry.SourceNode, entry.Hash,
	)
	if err != nil {
		return 0, fmt.Errorf("insert: %w", err)
	}

	return sequence, nil
}

// Load retrieves an event by ID.
func (s *SQLiteEventSource) Load(ctx context.Context, eventID string) (*EventStoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry := &EventStoreEntry{}
	query := `
	SELECT event_id, sequence, timestamp_ms, event_type, correlation_id, payload, source_node, hash
	FROM events WHERE event_id = ?
	`

	err := s.db.QueryRowContext(ctx, query, eventID).Scan(
		&entry.EventId, &entry.Sequence, &entry.TimestampMs, &entry.EventType, &entry.CorrelationId,
		&entry.Payload, &entry.SourceNode, &entry.Hash,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	return entry, nil
}

// Query returns events matching criteria.
func (s *SQLiteEventSource) Query(ctx context.Context, eventType, correlationID string, sinceMS int64) ([]*EventStoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := "SELECT event_id, sequence, timestamp_ms, event_type, correlation_id, payload, source_node, hash FROM events WHERE 1=1"
	var args []interface{}

	if eventType != "" {
		query += " AND event_type = ?"
		args = append(args, eventType)
	}
	if correlationID != "" {
		query += " AND correlation_id = ?"
		args = append(args, correlationID)
	}
	if sinceMS > 0 {
		query += " AND timestamp_ms >= ?"
		args = append(args, sinceMS)
	}

	query += " ORDER BY sequence ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	result := make([]*EventStoreEntry, 0)

	for rows.Next() {
		entry := &EventStoreEntry{}
		err := rows.Scan(
			&entry.EventId, &entry.Sequence, &entry.TimestampMs, &entry.EventType, &entry.CorrelationId,
			&entry.Payload, &entry.SourceNode, &entry.Hash,
		)
		if err != nil {
			s.logger.Error("scan error", "error", err)
			continue
		}
		result = append(result, entry)
	}

	return result, rows.Err()
}

// Tail returns the last N events.
func (s *SQLiteEventSource) Tail(ctx context.Context, limit int) ([]*EventStoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	query := `
	SELECT event_id, sequence, timestamp_ms, event_type, correlation_id, payload, source_node, hash
	FROM events ORDER BY sequence DESC LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	result := make([]*EventStoreEntry, 0, limit)

	for rows.Next() {
		entry := &EventStoreEntry{}
		err := rows.Scan(
			&entry.EventId, &entry.Sequence, &entry.TimestampMs, &entry.EventType, &entry.CorrelationId,
			&entry.Payload, &entry.SourceNode, &entry.Hash,
		)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result = append(result, entry)
	}

	// Reverse to get oldest first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, rows.Err()
}

// Size returns the total number of events.
func (s *SQLiteEventSource) Size(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count)
	return count, err
}

// Hash returns the hash of all events.
func (s *SQLiteEventSource) Hash(ctx context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := "SELECT event_id, event_type FROM events ORDER BY sequence ASC"

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	h := sha256.New()

	for rows.Next() {
		var eventID, eventType string
		if err := rows.Scan(&eventID, &eventType); err != nil {
			continue
		}
		h.Write([]byte(eventID))
		h.Write([]byte(eventType))
	}

	return hex.EncodeToString(h.Sum(nil)), rows.Err()
}

// Compact removes old events.
func (s *SQLiteEventSource) Compact(ctx context.Context, keepMS int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UnixMilli() - keepMS

	result, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE timestamp_ms < ?", cutoff)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	affected, _ := result.RowsAffected()
	s.logger.Info("compacted events", "removed", affected)

	// Vacuum to reclaim space
	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		s.logger.Warn("vacuum failed", "error", err)
	}

	return nil
}

// Close closes the database connection.
func (s *SQLiteEventSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// computeHash computes the hash of an event.
func (s *SQLiteEventSource) computeHash(entry *EventStoreEntry) string {
	h := sha256.New()
	h.Write([]byte(entry.EventId))
	h.Write([]byte(entry.EventType))
	h.Write(entry.Payload)
	h.Write([]byte(entry.CorrelationId))
	return hex.EncodeToString(h.Sum(nil))
}
