package tools

// audit_db.go — Structured SQLite audit log for every tool execution.
//
// Design goals:
//   - Zero-overhead for the caller: LogExecution() fires a goroutine and
//     returns immediately; the caller never blocks on a DB write.
//   - Resilient to SQLite locking: auditExecWithRetry() backs off
//     exponentially on SQLITE_BUSY / "database is locked" errors.
//   - Self-maintaining: StartPruner() trims the oldest rows every 12 h so
//     the database never degrades on constrained Android / Termux hardware.
//   - Platform-agnostic: only path/filepath + os for filesystem operations.
//
// Schema (tool_audit_log):
//
//	id             INTEGER  PK AUTOINCREMENT
//	timestamp      DATETIME DEFAULT CURRENT_TIMESTAMP
//	tool_name      TEXT NOT NULL
//	args_json      TEXT          — truncated at 4 KiB to cap DB growth
//	success        BOOLEAN NOT NULL
//	error_category TEXT          — 'rate_limit' | 'timeout' | 'tls_error' | ...
//	raw_error      TEXT
//	execution_ms   INTEGER

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver; no cgo required
)

// DefaultAuditMaxRecords is the row-count threshold above which PruneAuditLogs
// deletes the oldest entries.  Exported so main.go can reference it.
const DefaultAuditMaxRecords = 10_000

// auditPruneInterval controls how often the background pruner wakes up.
const auditPruneInterval = 12 * time.Hour

// auditMaxArgBytes caps the args_json column to prevent multi-MB file contents
// from bloating the audit database.
const auditMaxArgBytes = 4096

// auditRetryMax / auditRetryBackoffMs control the exponential backoff for
// SQLite "database is locked" / SQLITE_BUSY errors.
const (
	auditRetryMax       = 5
	auditRetryBackoffMs = 10
)

// AuditDB wraps a *sql.DB and provides the async, retry-safe audit API.
// All exported methods are nil-receiver safe.
type AuditDB struct {
	db    *sql.DB
	pruneMu sync.Mutex // serialises concurrent prune calls
}

// InitAuditDB opens (or creates) <dataDir>/audit.db, applies the schema,
// and enables WAL mode for concurrent-write performance.
//
// Returns (nil, nil) if dataDir is empty — the caller can proceed without
// auditing rather than crashing.
func InitAuditDB(dataDir string) (*AuditDB, error) {
	if dataDir == "" {
		return nil, nil
	}

	// Ensure the directory exists with restricted permissions.
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("audit_db: mkdir %q: %w", dataDir, err)
	}

	dbPath := filepath.Join(dataDir, "audit.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("audit_db: open %q: %w", dbPath, err)
	}

	// Limit to a single writer connection — SQLite is not concurrency-safe
	// with multiple writers; serialisation here prevents SQLITE_BUSY storms.
	db.SetMaxOpenConns(1)

	// WAL mode allows concurrent readers while a writer is active.
	// busy_timeout gives any competing writer up to 5 s before returning BUSY.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("audit_db: pragma: %w", err)
	}

	if err := auditMigrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("audit_db: migrate: %w", err)
	}

	return &AuditDB{db: db}, nil
}

// auditMigrate creates the tool_audit_log table and its indexes idempotently.
func auditMigrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tool_audit_log (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp      DATETIME DEFAULT CURRENT_TIMESTAMP,
			tool_name      TEXT NOT NULL,
			args_json      TEXT,
			success        BOOLEAN NOT NULL,
			error_category TEXT,
			raw_error      TEXT,
			execution_ms   INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_tool_name ON tool_audit_log(tool_name);
		CREATE INDEX IF NOT EXISTS idx_success   ON tool_audit_log(success);
	`)
	return err
}

// auditExecWithRetry executes a parameterised SQL statement and retries with
// exponential backoff when SQLite returns SQLITE_BUSY / "database is locked".
// It does NOT retry non-locking errors (schema errors, constraint violations).
func (a *AuditDB) auditExecWithRetry(ctx context.Context, query string, args ...interface{}) error {
	var lastErr error
	for i := 0; i < auditRetryMax; i++ {
		_, lastErr = a.db.ExecContext(ctx, query, args...)
		if lastErr == nil {
			return nil
		}
		msg := lastErr.Error()
		if !strings.Contains(msg, "database is locked") &&
			!strings.Contains(msg, "SQLITE_BUSY") {
			// Non-locking error — don't retry.
			return lastErr
		}
		// Exponential backoff: 10 ms, 20 ms, 40 ms, 80 ms, 160 ms
		delay := time.Duration(float64(auditRetryBackoffMs)*math.Pow(2, float64(i))) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("audit_db: db locked after %d retries: %w", auditRetryMax, lastErr)
}

// LogExecution fires a background goroutine to insert one row into
// tool_audit_log.  It returns immediately and never blocks the caller.
// Safe to call with a nil receiver — becomes a no-op.
func (a *AuditDB) LogExecution(
	toolName string,
	argsJSON string,
	success bool,
	errCategory string,
	rawErr string,
	executionMs int64,
) {
	if a == nil || a.db == nil {
		return
	}

	// Truncate large arg blobs so file-content tools don't fill the DB.
	if len(argsJSON) > auditMaxArgBytes {
		argsJSON = argsJSON[:auditMaxArgBytes] + "…(truncated)"
	}

	// Snapshot values before handing off to the goroutine.
	tn, aj, s, ec, re, ms :=
		toolName, argsJSON, success, errCategory, rawErr, executionMs

	go func() {
		// 10-second budget covers all retries (max ~310 ms) with room to spare.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		successVal := 0
		if s {
			successVal = 1
		}

		const q = `INSERT INTO tool_audit_log
			(tool_name, args_json, success, error_category, raw_error, execution_ms)
			VALUES (?, ?, ?, ?, ?, ?)`

		//nolint:errcheck  — best-effort; caller must not block on audit writes
		_ = a.auditExecWithRetry(ctx, q, tn, aj, successVal, ec, re, ms)
	}()
}

// PruneAuditLogs deletes the oldest rows when the total row count exceeds
// maxRecords.  It is safe to call concurrently — the internal mutex ensures
// only one prune runs at a time.
func (a *AuditDB) PruneAuditLogs(maxRecords int) error {
	if a == nil || a.db == nil {
		return nil
	}

	a.pruneMu.Lock()
	defer a.pruneMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	if err := a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tool_audit_log`).Scan(&count); err != nil {
		return fmt.Errorf("audit_db: prune count: %w", err)
	}

	if count <= maxRecords {
		return nil // Nothing to prune.
	}

	excess := count - maxRecords
	return a.auditExecWithRetry(ctx,
		`DELETE FROM tool_audit_log
		 WHERE id IN (
		   SELECT id FROM tool_audit_log ORDER BY id ASC LIMIT ?
		 )`,
		excess,
	)
}

// StartPruner starts a long-lived goroutine that prunes audit logs on startup
// and then every auditPruneInterval (12 h).  The goroutine exits when ctx is
// cancelled.  Safe to call with a nil receiver — becomes a no-op.
func (a *AuditDB) StartPruner(ctx context.Context, maxRecords int) {
	if a == nil || a.db == nil {
		return
	}

	go func() {
		// Prune on startup to reclaim space from a previous long session.
		if err := a.PruneAuditLogs(maxRecords); err != nil {
			// Non-fatal — just log the error to stderr and continue.
			fmt.Fprintf(os.Stderr, "audit_db: startup prune: %v\n", err)
		}

		ticker := time.NewTicker(auditPruneInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := a.PruneAuditLogs(maxRecords); err != nil {
					fmt.Fprintf(os.Stderr, "audit_db: scheduled prune: %v\n", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Close releases the underlying database connection.
// Safe to call with a nil receiver.
func (a *AuditDB) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	return a.db.Close()
}

// classifyToolError maps a tool execution error to one of a small set of
// canonical category strings for the error_category column.
// Returns "" when the execution succeeded.
func classifyToolError(err error, result *ToolResult) string {
	if err == nil {
		if result == nil || result.Success {
			return ""
		}
		// Tool ran but reported failure in its result.
		if result.Error != "" {
			return classifyErrString(result.Error)
		}
		return "tool_error"
	}
	return classifyErrString(err.Error())
}

func classifyErrString(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "429") ||
		strings.Contains(lower, "too many requests"):
		return "rate_limit"
	case strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded") ||
		strings.Contains(lower, "context deadline"):
		return "timeout"
	case strings.Contains(lower, "tls") ||
		strings.Contains(lower, "bad record mac") ||
		strings.Contains(lower, "certificate") ||
		strings.Contains(lower, "x509"):
		return "tls_error"
	case strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "401") || strings.Contains(lower, "403"):
		return "permission_denied"
	case strings.Contains(lower, "no credits") ||
		strings.Contains(lower, "402") ||
		strings.Contains(lower, "billing"):
		return "no_credits"
	case strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "broken pipe") ||
		strings.Contains(lower, "eof"):
		return "network_error"
	default:
		return "tool_error"
	}
}
