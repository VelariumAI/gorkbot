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
	db      *sql.DB
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

// AuditToolStat holds per-tool summary statistics from the persistent audit log.
type AuditToolStat struct {
	ToolName       string
	ExecutionCount int
	SuccessCount   int
}

// TopTools returns the top n tools by total execution count from the persistent
// audit log. It is safe to call with a nil receiver (returns nil).
func (a *AuditDB) TopTools(n int) ([]AuditToolStat, error) {
	if a == nil || a.db == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := a.db.QueryContext(ctx, `
		SELECT tool_name,
		       COUNT(*)        AS total,
		       SUM(success)    AS successes
		FROM   tool_audit_log
		GROUP  BY tool_name
		ORDER  BY total DESC
		LIMIT  ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []AuditToolStat
	for rows.Next() {
		var s AuditToolStat
		if err := rows.Scan(&s.ToolName, &s.ExecutionCount, &s.SuccessCount); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// AuditSummary returns a markdown table of all tools ranked by execution count,
// including success rate, average duration, and dominant error category.
// Safe to call with a nil receiver.
func (a *AuditDB) AuditSummary(limit int) string {
	if a == nil || a.db == nil {
		return "Audit DB not available."
	}
	if limit <= 0 {
		limit = 25
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := a.db.QueryContext(ctx, `
		SELECT tool_name,
		       COUNT(*)              AS calls,
		       SUM(success)          AS successes,
		       AVG(execution_ms)     AS avg_ms,
		       (SELECT error_category
		          FROM tool_audit_log t2
		         WHERE t2.tool_name = t1.tool_name AND t2.success = 0
		         GROUP BY error_category
		         ORDER BY COUNT(*) DESC
		         LIMIT 1)            AS top_err
		FROM   tool_audit_log t1
		GROUP  BY tool_name
		ORDER  BY calls DESC
		LIMIT  ?`, limit)
	if err != nil {
		return fmt.Sprintf("audit summary error: %v", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("## Tool Audit Log (all-time)\n\n")
	sb.WriteString("| Tool | Calls | Success% | Avg ms | Top Error |\n")
	sb.WriteString("|------|-------|----------|--------|-----------|\n")

	total, totalFail := 0, 0
	for rows.Next() {
		var name string
		var calls, successes int
		var avgMs float64
		var topErr *string
		if err := rows.Scan(&name, &calls, &successes, &avgMs, &topErr); err != nil {
			continue
		}
		pct := 0.0
		if calls > 0 {
			pct = float64(successes) / float64(calls) * 100
		}
		errStr := "—"
		if topErr != nil && *topErr != "" {
			errStr = *topErr
		}
		total += calls
		totalFail += calls - successes
		sb.WriteString(fmt.Sprintf("| %s | %d | %.0f%% | %.0f | %s |\n",
			name, calls, pct, avgMs, errStr))
	}
	if rows.Err() != nil {
		return fmt.Sprintf("audit summary scan error: %v", rows.Err())
	}
	if total > 0 {
		overallPct := float64(total-totalFail) / float64(total) * 100
		sb.WriteString(fmt.Sprintf("\n**Total**: %d calls | **%.0f%% success** | %d failures\n",
			total, overallPct, totalFail))
	} else {
		sb.WriteString("\n_No tool executions recorded yet._\n")
	}
	return sb.String()
}

// RecentErrors returns a markdown table of the most recent failed tool executions.
// Pass toolFilter="" to include all tools.  Safe to call with a nil receiver.
func (a *AuditDB) RecentErrors(limit int, toolFilter string) string {
	if a == nil || a.db == nil {
		return "Audit DB not available."
	}
	if limit <= 0 {
		limit = 20
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rows *sql.Rows
	var err error
	if toolFilter == "" {
		rows, err = a.db.QueryContext(ctx, `
			SELECT timestamp, tool_name, error_category, raw_error, execution_ms
			FROM   tool_audit_log
			WHERE  success = 0
			ORDER  BY id DESC
			LIMIT  ?`, limit)
	} else {
		rows, err = a.db.QueryContext(ctx, `
			SELECT timestamp, tool_name, error_category, raw_error, execution_ms
			FROM   tool_audit_log
			WHERE  success = 0 AND tool_name = ?
			ORDER  BY id DESC
			LIMIT  ?`, toolFilter, limit)
	}
	if err != nil {
		return fmt.Sprintf("recent errors query error: %v", err)
	}
	defer rows.Close()

	var sb strings.Builder
	header := "## Recent Tool Failures"
	if toolFilter != "" {
		header += " — " + toolFilter
	}
	sb.WriteString(header + "\n\n")
	sb.WriteString("| Time | Tool | Category | Error | ms |\n")
	sb.WriteString("|------|------|----------|-------|----||\n")

	count := 0
	for rows.Next() {
		var ts, tool string
		var category, rawErr *string
		var ms int64
		if err := rows.Scan(&ts, &tool, &category, &rawErr, &ms); err != nil {
			continue
		}
		catStr, errStr := "—", "—"
		if category != nil && *category != "" {
			catStr = *category
		}
		if rawErr != nil && *rawErr != "" {
			e := *rawErr
			if len(e) > 60 {
				e = e[:57] + "…"
			}
			errStr = e
		}
		// Trim timestamp to HH:MM:SS
		if len(ts) >= 19 {
			ts = ts[11:19]
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d |\n",
			ts, tool, catStr, errStr, ms))
		count++
	}
	if rows.Err() != nil {
		return fmt.Sprintf("recent errors scan error: %v", rows.Err())
	}
	if count == 0 {
		sb.WriteString("\n_No failures recorded._\n")
	}
	return sb.String()
}

// ErrorRate returns the total call count, failure count, and failure rate for
// the last `hours` hours across all tools.  Safe to call with a nil receiver.
func (a *AuditDB) ErrorRate(hours int) (total, failed int, rate float64, err error) {
	if a == nil || a.db == nil {
		return 0, 0, 0, nil
	}
	if hours <= 0 {
		hours = 24
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = a.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END)
		FROM   tool_audit_log
		WHERE  timestamp >= datetime('now', ? || ' hours')`,
		fmt.Sprintf("-%d", hours),
	).Scan(&total, &failed)
	if err != nil {
		return 0, 0, 0, err
	}
	if total > 0 {
		rate = float64(failed) / float64(total) * 100
	}
	return total, failed, rate, nil
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
		strings.Contains(lower, "context deadline") ||
		// HTTP/2 header-wait timeout — the server connected but never sent HEADERS.
		// Seen on flaky WiFi / cellular with MiniMax and Anthropic secondaries.
		strings.Contains(lower, "awaiting response headers") ||
		strings.Contains(lower, "http2: timeout") ||
		strings.Contains(lower, "i/o timeout"):
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
		strings.Contains(lower, "eof") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "no route to host"):
		return "network_error"
	// signal: killed — OOM killer on Android (e.g., download_file on low RAM).
	case strings.Contains(lower, "signal: killed") ||
		strings.Contains(lower, "signal: oom"):
		return "resource_limit"
	// Missing executable: start_background_process / bash on Termux.
	case strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "exec: ") ||
		strings.Contains(lower, "no such file or directory") && strings.Contains(lower, "exec"):
		return "env_error"
	// Missing required params — caught by validateRequiredParams in registry.
	case strings.Contains(lower, "missing required parameter"):
		return "param_error"
	default:
		return "tool_error"
	}
}
