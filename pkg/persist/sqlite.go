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

// SessionSearchResult contains search result metadata for session_search tool.
type SessionSearchResult struct {
	SessionID    string
	Title        string
	StartedAt    time.Time
	Source       string
	Snippet      string  // 200-char context around match
	Score        float64 // BM25 rank (negative = better)
	MessageCount int
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

	// Enable WAL mode for high concurrency; busy_timeout avoids SQLITE_BUSY.
	_, err = db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA cache_size = -64000;
		PRAGMA temp_store = MEMORY;
		PRAGMA busy_timeout = 5000;
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("persist: pragma: %w", err)
	}

	s := &Store{db: db, sessionID: sessionID}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("persist: migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	// v1–v4: baseline schema (idempotent via IF NOT EXISTS).
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

		CREATE TABLE IF NOT EXISTS cache_session_context (
			session_id TEXT PRIMARY KEY,
			summary_text TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			ttl_seconds INTEGER DEFAULT 3600
		);

		CREATE TABLE IF NOT EXISTS cache_tool_results (
			request_hash TEXT PRIMARY KEY,
			result_data TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			ttl_seconds INTEGER DEFAULT 300
		);
	`)
	if err != nil {
		return err
	}

	// Versioned migration: check current schema version.
	var version int
	err = s.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&version)
	if err != nil {
		// schema_version table doesn't exist yet — we are at v4 (baseline).
		version = 4
	}

	if version < 5 {
		if err := s.migrateV5(); err != nil {
			return fmt.Errorf("migrate v5: %w", err)
		}
	}

	if version < 6 {
		if err := s.migrateV6(); err != nil {
			return fmt.Errorf("migrate v6: %w", err)
		}
	}
	return nil
}

// migrateV5 adds schema_version tracking, sessions table, and FTS5 support.
func (s *Store) migrateV5() error {
	// Create schema_version table and seed with current version if missing.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);
		INSERT OR IGNORE INTO schema_version VALUES(0);
	`); err != nil {
		return err
	}

	// Sessions metadata table.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			session_id   TEXT PRIMARY KEY,
			title        TEXT DEFAULT '',
			source       TEXT DEFAULT 'cli',
			started_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		return err
	}

	// FTS5 virtual table for full-text search on conversation content.
	// FTS5 availability depends on the SQLite build; fall back gracefully.
	ftsErr := s.tryCreateFTS5()
	if ftsErr != nil {
		// FTS5 unavailable — log but do not fail startup.
		_ = ftsErr
	}

	// Mark schema as v5.
	_, err := s.db.Exec(`UPDATE schema_version SET version = 5`)
	return err
}

// migrateV6 adds HITL decision history table for intelligent tool approval tracking.
func (s *Store) migrateV6() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS hitl_decisions (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id      TEXT NOT NULL,
			tool_name       TEXT NOT NULL,
			params_hash     TEXT NOT NULL,
			params_json     TEXT DEFAULT '{}',
			approved        INTEGER DEFAULT 0,
			rejected        INTEGER DEFAULT 0,
			notes           TEXT DEFAULT '',
			risk_level      INTEGER DEFAULT 0,
			confidence      INTEGER DEFAULT 0,
			execution_result TEXT DEFAULT '',
			similar_count   INTEGER DEFAULT 0,
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_hitl_tool ON hitl_decisions(tool_name);
		CREATE INDEX IF NOT EXISTS idx_hitl_hash ON hitl_decisions(params_hash);
		CREATE INDEX IF NOT EXISTS idx_hitl_session ON hitl_decisions(session_id);
		CREATE INDEX IF NOT EXISTS idx_hitl_approval ON hitl_decisions(approved);
		CREATE INDEX IF NOT EXISTS idx_hitl_time ON hitl_decisions(created_at DESC);
	`); err != nil {
		return err
	}

	// Mark schema as v6.
	_, err := s.db.Exec(`UPDATE schema_version SET version = 6`)
	return err
}

// tryCreateFTS5 attempts to create the FTS5 virtual table and sync triggers.
// Returns an error if FTS5 is not available in this SQLite build.
func (s *Store) tryCreateFTS5() error {
	_, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS conversations_fts USING fts5(
			content,
			content='conversations',
			content_rowid='id'
		);

		CREATE TRIGGER IF NOT EXISTS conversations_fts_insert
		AFTER INSERT ON conversations BEGIN
			INSERT INTO conversations_fts(rowid, content) VALUES (new.id, new.content);
		END;

		CREATE TRIGGER IF NOT EXISTS conversations_fts_delete
		AFTER DELETE ON conversations BEGIN
			INSERT INTO conversations_fts(conversations_fts, rowid, content) VALUES('delete', old.id, old.content);
		END;

		CREATE TRIGGER IF NOT EXISTS conversations_fts_update
		AFTER UPDATE ON conversations BEGIN
			INSERT INTO conversations_fts(conversations_fts, rowid, content) VALUES('delete', old.id, old.content);
			INSERT INTO conversations_fts(rowid, content) VALUES (new.id, new.content);
		END;
	`)
	return err
}

// hasFTS5 returns true if the conversations_fts virtual table exists.
func (s *Store) hasFTS5() bool {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='conversations_fts' LIMIT 1`).Scan(&name)
	return err == nil && name == "conversations_fts"
}

// SaveSessionContext UPSERTs a session summary.
func (s *Store) SaveSessionContext(ctx context.Context, sessionID, summary string, ttlSeconds int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cache_session_context (session_id, summary_text, updated_at, ttl_seconds) 
		VALUES (?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(session_id) DO UPDATE SET 
			summary_text = excluded.summary_text,
			updated_at = CURRENT_TIMESTAMP,
			ttl_seconds = excluded.ttl_seconds;
	`, sessionID, summary, ttlSeconds)
	return err
}

// GetSessionContext returns the cached summary if it exists and hasn't expired.
func (s *Store) GetSessionContext(ctx context.Context, sessionID string) (string, bool, error) {
	var summary string
	var updatedAt time.Time
	var ttl int
	err := s.db.QueryRowContext(ctx, `
		SELECT summary_text, updated_at, ttl_seconds 
		FROM cache_session_context 
		WHERE session_id = ?
	`, sessionID).Scan(&summary, &updatedAt, &ttl)

	if err == sql.ErrNoRows {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}

	if time.Since(updatedAt).Seconds() > float64(ttl) {
		return "", false, nil // Expired
	}

	return summary, true, nil
}

// GetLatestContext returns the most recent non-expired session context across
// all session IDs.  This is used at startup to restore a prior compressed
// summary when the session ID rotates between runs.
func (s *Store) GetLatestContext(ctx context.Context) (string, bool, error) {
	var summary string
	var updatedAt time.Time
	var ttl int
	err := s.db.QueryRowContext(ctx, `
		SELECT summary_text, updated_at, ttl_seconds
		FROM cache_session_context
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(&summary, &updatedAt, &ttl)

	if err == sql.ErrNoRows {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	if time.Since(updatedAt).Seconds() > float64(ttl) {
		return "", false, nil // expired
	}
	return summary, true, nil
}

// PruneExpiredContexts deletes stale rows from cache_session_context.
// Safe to call from a background goroutine.
func (s *Store) PruneExpiredContexts(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM cache_session_context
		 WHERE datetime(updated_at, '+' || ttl_seconds || ' seconds') < datetime('now')`)
	return err
}

// UpsertSession ensures a row exists in the sessions table for the given session.
// source is e.g. "cli", "web", "api". Safe to call multiple times.
func (s *Store) UpsertSession(ctx context.Context, sessionID, source string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO sessions (session_id, source) VALUES (?, ?)
	`, sessionID, source)
	return err
}

// SetSessionTitle upserts a human-readable title for a session.
func (s *Store) SetSessionTitle(ctx context.Context, sessionID, title string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (session_id, title) VALUES (?, ?)
		ON CONFLICT(session_id) DO UPDATE SET title = excluded.title
	`, sessionID, title)
	return err
}

// GetSessionTitle returns the title for a session, or "" if not found.
func (s *Store) GetSessionTitle(ctx context.Context, sessionID string) (string, error) {
	var title string
	err := s.db.QueryRowContext(ctx, `SELECT title FROM sessions WHERE session_id = ?`, sessionID).Scan(&title)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return title, err
}

// SearchSessions performs full-text search over conversation history.
// It attempts FTS5 first and falls back to a LIKE query if unavailable.
// days=0 means no time filter. topK defaults to 5 if 0.
func (s *Store) SearchSessions(ctx context.Context, query string, days, topK int) ([]SessionSearchResult, error) {
	if topK == 0 {
		topK = 5
	}

	type rawResult struct {
		sessionID    string
		snippet      string
		score        float64
		messageCount int
	}

	var rows []rawResult

	if s.hasFTS5() {
		sqlQuery := `
			SELECT c.session_id,
			       snippet(conversations_fts, 0, '[', ']', '...', 20) as snip,
			       -rank as score,
			       COUNT(*) OVER (PARTITION BY c.session_id) as msg_count
			FROM conversations_fts
			JOIN conversations c ON conversations_fts.rowid = c.id
			WHERE conversations_fts MATCH ?
			AND (? = 0 OR c.created_at >= datetime('now', '-' || ? || ' days'))
			ORDER BY rank
			LIMIT 50
		`
		r, err := s.db.QueryContext(ctx, sqlQuery, query, days, days)
		if err != nil {
			// FTS5 query failed (e.g. bad query syntax) — fall through to LIKE
			goto likeSearch
		}
		defer r.Close()
		for r.Next() {
			var rr rawResult
			if err := r.Scan(&rr.sessionID, &rr.snippet, &rr.score, &rr.messageCount); err != nil {
				continue
			}
			rows = append(rows, rr)
		}
		if err := r.Err(); err != nil {
			return nil, err
		}
		goto deduplicate
	}

likeSearch:
	{
		likeParam := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"
		sqlQuery := `
			SELECT session_id, content as snip, 1.0 as score, 0 as msg_count
			FROM conversations
			WHERE content LIKE ?
			AND (? = 0 OR created_at >= datetime('now', '-' || ? || ' days'))
			ORDER BY created_at DESC
			LIMIT 50
		`
		r, err := s.db.QueryContext(ctx, sqlQuery, likeParam, days, days)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		for r.Next() {
			var rr rawResult
			if err := r.Scan(&rr.sessionID, &rr.snippet, &rr.score, &rr.messageCount); err != nil {
				continue
			}
			// Trim snippet to ~200 chars
			if len(rr.snippet) > 200 {
				rr.snippet = rr.snippet[:200] + "..."
			}
			rows = append(rows, rr)
		}
		if err := r.Err(); err != nil {
			return nil, err
		}
	}

deduplicate:
	// Keep only the best-ranked result per session_id.
	seen := make(map[string]rawResult)
	for _, rr := range rows {
		if prev, ok := seen[rr.sessionID]; !ok || rr.score > prev.score {
			seen[rr.sessionID] = rr
		}
	}

	// Sort deduped results by score descending.
	deduped := make([]rawResult, 0, len(seen))
	for _, rr := range seen {
		deduped = append(deduped, rr)
	}
	// Simple insertion sort (small N).
	for i := 1; i < len(deduped); i++ {
		for j := i; j > 0 && deduped[j].score > deduped[j-1].score; j-- {
			deduped[j], deduped[j-1] = deduped[j-1], deduped[j]
		}
	}
	if len(deduped) > topK {
		deduped = deduped[:topK]
	}

	// Enrich with session metadata.
	results := make([]SessionSearchResult, 0, len(deduped))
	for _, rr := range deduped {
		res := SessionSearchResult{
			SessionID:    rr.sessionID,
			Snippet:      rr.snippet,
			Score:        rr.score,
			MessageCount: rr.messageCount,
			Source:       "cli",
		}
		// LEFT JOIN against sessions table.
		var title, source string
		var startedAt string
		err := s.db.QueryRowContext(ctx, `
			SELECT COALESCE(title,''), COALESCE(source,'cli'), COALESCE(started_at,'')
			FROM sessions WHERE session_id = ?
		`, rr.sessionID).Scan(&title, &source, &startedAt)
		if err == nil {
			res.Title = title
			res.Source = source
			if startedAt != "" {
				t, _ := time.Parse("2006-01-02 15:04:05", startedAt)
				res.StartedAt = t
			}
		}
		results = append(results, res)
	}
	return results, nil
}

// SaveTurn persists a conversation turn to the database.
func (s *Store) SaveTurn(ctx context.Context, role, content string, metadata map[string]interface{}) error {
	meta := "{}"
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			meta = string(b)
		}
	}
	// Ensure session row exists so sessions table stays in sync.
	_ = s.UpsertSession(ctx, s.sessionID, "cli")
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

// SaveHITLDecision stores a HITL approval/rejection decision in the database.
func (s *Store) SaveHITLDecision(ctx context.Context, toolName, paramsHash, paramsJSON string, approved bool, riskLevel, confidence int, notes string) error {
	approvedInt := 0
	if approved {
		approvedInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO hitl_decisions (session_id, tool_name, params_hash, params_json, approved, risk_level, confidence, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, s.sessionID, toolName, paramsHash, paramsJSON, approvedInt, riskLevel, confidence, notes)
	return err
}

// CountApprovedExecutions returns count of similar previously approved operations.
func (s *Store) CountApprovedExecutions(ctx context.Context, toolName string, paramsHash string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM hitl_decisions
		WHERE tool_name = ? AND params_hash = ? AND approved = 1
	`, toolName, paramsHash).Scan(&count)
	return count, err
}

// WasRecentlyRejected checks if a tool was rejected in the last N hours.
func (s *Store) WasRecentlyRejected(ctx context.Context, toolName string, paramsHash string, hoursBack int) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM hitl_decisions
		WHERE tool_name = ? AND params_hash = ? AND rejected = 1
		AND created_at > datetime('now', '-' || ? || ' hours')
	`, toolName, paramsHash, hoursBack).Scan(&count)
	return count > 0, err
}

// GetHITLStats returns approval/rejection statistics for a tool.
func (s *Store) GetHITLStats(ctx context.Context, toolName string) (approved int, rejected int, totalRisk float64, avgConfidence float64, err error) {
	var approvedCount, rejectedCount int
	var totalRiskVal float64
	var avgConfVal float64

	err = s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(CASE WHEN approved = 1 THEN 1 END),
			COUNT(CASE WHEN rejected = 1 THEN 1 END),
			COALESCE(AVG(risk_level), 0),
			COALESCE(AVG(confidence), 0)
		FROM hitl_decisions
		WHERE tool_name = ?
	`, toolName).Scan(&approvedCount, &rejectedCount, &totalRiskVal, &avgConfVal)

	return approvedCount, rejectedCount, totalRiskVal, avgConfVal, err
}

// PruneOldHITLDecisions deletes HITL records older than the specified number of days.
func (s *Store) PruneOldHITLDecisions(ctx context.Context, daysOld int) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM hitl_decisions
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`, daysOld)
	return err
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying sql.DB instance for use in caches.
func (s *Store) DB() *sql.DB {
	return s.db
}
