package persist

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ConversationRecord stores a single conversation turn.
type ConversationRecord struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	Metadata  map[string]interface{}
	CreatedAt time.Time
}

// Store provides SQLite-backed persistence for conversation history and memories.
type Store struct {
	db        *sql.DB
	sessionID string
}

// NewStore opens (or creates) the SQLite database at configDir/gorkbot.db.
func NewStore(configDir, sessionID string) (*Store, error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(configDir, "gorkbot.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("persist: open db: %w", err)
	}
	s := &Store{db: db, sessionID: sessionID}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("persist: migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS conversations (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id  TEXT NOT NULL,
			role        TEXT NOT NULL,
			content     TEXT NOT NULL,
			metadata    TEXT DEFAULT '{}',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_conv_session ON conversations(session_id, created_at);

		CREATE TABLE IF NOT EXISTS memories (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			text        TEXT NOT NULL UNIQUE,
			confidence  REAL DEFAULT 0.5,
			use_count   INTEGER DEFAULT 0,
			tags        TEXT DEFAULT '[]',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_mem_conf ON memories(confidence DESC);

		CREATE TABLE IF NOT EXISTS tool_calls (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id  TEXT NOT NULL,
			tool_name   TEXT NOT NULL,
			params      TEXT DEFAULT '{}',
			result      TEXT,
			success     INTEGER DEFAULT 1,
			duration_ms INTEGER DEFAULT 0,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_tool_session ON tool_calls(session_id, created_at);
	`)
	return err
}

// SaveTurn persists a conversation turn to the database.
func (s *Store) SaveTurn(ctx context.Context, role, content string, metadata map[string]interface{}) error {
	meta := "{}"
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			meta = string(b)
		}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO conversations (session_id, role, content, metadata) VALUES (?, ?, ?, ?)`,
		s.sessionID, role, content, meta,
	)
	return err
}

// LoadSession retrieves all turns for the current session.
func (s *Store) LoadSession(ctx context.Context) ([]ConversationRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, metadata, created_at FROM conversations WHERE session_id = ? ORDER BY created_at`,
		s.sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ConversationRecord
	for rows.Next() {
		var r ConversationRecord
		var metaStr string
		var createdAt string
		if err := rows.Scan(&r.ID, &r.SessionID, &r.Role, &r.Content, &metaStr, &createdAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(metaStr), &r.Metadata)
		r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		records = append(records, r)
	}
	return records, rows.Err()
}

// SaveToolCall persists a tool execution record.
func (s *Store) SaveToolCall(ctx context.Context, toolName string, params map[string]interface{}, result string, success bool, durationMs int64) error {
	paramsJSON := "{}"
	if params != nil {
		if b, err := json.Marshal(params); err == nil {
			paramsJSON = string(b)
		}
	}
	successInt := 0
	if success {
		successInt = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tool_calls (session_id, tool_name, params, result, success, duration_ms) VALUES (?, ?, ?, ?, ?, ?)`,
		s.sessionID, toolName, paramsJSON, result, successInt, durationMs,
	)
	return err
}

// ToolCallStats returns per-tool call counts and success rates.
func (s *Store) ToolCallStats(ctx context.Context) (string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tool_name,
		       COUNT(*) as calls,
		       SUM(success) as successes,
		       AVG(duration_ms) as avg_ms
		FROM tool_calls
		GROUP BY tool_name
		ORDER BY calls DESC
	`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb []string
	sb = append(sb, "| Tool | Calls | Success Rate | Avg ms |")
	sb = append(sb, "|------|-------|-------------|--------|")
	for rows.Next() {
		var name string
		var calls, successes int
		var avgMs float64
		if err := rows.Scan(&name, &calls, &successes, &avgMs); err != nil {
			continue
		}
		rate := 0.0
		if calls > 0 {
			rate = float64(successes) / float64(calls) * 100
		}
		sb = append(sb, fmt.Sprintf("| %s | %d | %.0f%% | %.0f |", name, calls, rate, avgMs))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "## Tool Call Statistics (All-Time)\n\n" + strings.Join(sb, "\n") + "\n", nil
}

// SessionCount returns the total number of distinct sessions.
func (s *Store) SessionCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT session_id) FROM conversations`).Scan(&count)
	return count, err
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }
