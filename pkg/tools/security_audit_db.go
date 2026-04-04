package tools

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

// SecurityAuditDB wraps a *sql.DB for security operation logging.
// All exported methods are nil-receiver safe.
type SecurityAuditDB struct {
	db      *sql.DB
	pruneMu sync.Mutex
}

const (
	securityAuditPruneInterval = 24 * time.Hour
	securityAuditMaxRecords    = 5000
	securityAuditRetryMax      = 5
	securityAuditRetryBackoff  = 10 // milliseconds
)

// InitSecurityAuditDB opens (or creates) <dataDir>/security_audit.db and applies the schema.
// Returns (nil, nil) if dataDir is empty — the caller can proceed without auditing.
func InitSecurityAuditDB(dataDir string) (*SecurityAuditDB, error) {
	if dataDir == "" {
		return nil, nil
	}

	// Ensure the directory exists with restricted permissions.
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("security_audit_db: mkdir %q: %w", dataDir, err)
	}

	dbPath := filepath.Join(dataDir, "security_audit.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("security_audit_db: open %q: %w", dbPath, err)
	}

	// Single writer for safety
	db.SetMaxOpenConns(1)

	// WAL mode for concurrent reads
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("security_audit_db: pragma: %w", err)
	}

	if err := securityAuditMigrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("security_audit_db: migrate: %w", err)
	}

	return &SecurityAuditDB{db: db}, nil
}

// securityAuditMigrate creates the security_operations table idempotently.
func securityAuditMigrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS security_operations (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp        DATETIME DEFAULT CURRENT_TIMESTAMP,
			tool_name        TEXT NOT NULL,
			target           TEXT,
			operation_type   TEXT,
			severity         TEXT,
			args_json        TEXT,
			success          BOOLEAN NOT NULL,
			duration_ms      INTEGER,
			result_summary   TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_sec_tool_name ON security_operations(tool_name);
		CREATE INDEX IF NOT EXISTS idx_sec_severity   ON security_operations(severity);
		CREATE INDEX IF NOT EXISTS idx_sec_timestamp  ON security_operations(timestamp);
	`)
	return err
}

// securityAuditExecWithRetry executes a query with exponential backoff on locking errors.
func (s *SecurityAuditDB) securityAuditExecWithRetry(ctx context.Context, query string, args ...interface{}) error {
	var lastErr error
	for i := 0; i < securityAuditRetryMax; i++ {
		_, lastErr = s.db.ExecContext(ctx, query, args...)
		if lastErr == nil {
			return nil
		}
		msg := lastErr.Error()
		if !strings.Contains(msg, "database is locked") &&
			!strings.Contains(msg, "SQLITE_BUSY") {
			return lastErr
		}
		// Exponential backoff
		delay := time.Duration(float64(securityAuditRetryBackoff)*math.Pow(2, float64(i))) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("security_audit_db: locked after %d retries: %w", securityAuditRetryMax, lastErr)
}

// LogSecurityOperation fires a background goroutine to log a security tool execution.
// It returns immediately and never blocks the caller.
// Safe to call with a nil receiver — becomes a no-op.
func (s *SecurityAuditDB) LogSecurityOperation(
	toolName, target, opType, severity, argsJSON string,
	success bool, durationMS int64, resultSummary string,
) {
	if s == nil || s.db == nil {
		return
	}

	// Snapshot values before handing off to the goroutine
	tn, tgt, ot, sev, aj, rs := toolName, target, opType, severity, argsJSON, resultSummary

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		successVal := 0
		if success {
			successVal = 1
		}

		const q = `INSERT INTO security_operations
			(tool_name, target, operation_type, severity, args_json, success, duration_ms, result_summary)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

		//nolint:errcheck  — best-effort; caller must not block on audit writes
		_ = s.securityAuditExecWithRetry(ctx, q, tn, tgt, ot, sev, aj, successVal, durationMS, rs)
	}()
}

// SecurityAuditSummary returns a markdown-formatted summary of recent security operations.
func (s *SecurityAuditDB) SecurityAuditSummary(limit int) string {
	if s == nil || s.db == nil {
		return "(no security audit data)"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT tool_name, COUNT(*) as count,
		       SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successful,
		       severity
		FROM security_operations
		GROUP BY tool_name, severity
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return fmt.Sprintf("(error: %v)", err)
	}
	defer rows.Close()

	var summary strings.Builder
	summary.WriteString("## Recent Security Operations\n\n")
	summary.WriteString("| Tool | Count | Successful | Severity |\n")
	summary.WriteString("|------|-------|------------|----------|\n")

	count := 0
	for rows.Next() {
		var toolName, severity string
		var totalCount, successCount int
		if err := rows.Scan(&toolName, &totalCount, &successCount, &severity); err != nil {
			continue
		}
		successStr := fmt.Sprintf("%d/%d", successCount, totalCount)
		summary.WriteString(fmt.Sprintf("| %s | %d | %s | %s |\n", toolName, totalCount, successStr, severity))
		count++
	}

	if count == 0 {
		summary.WriteString("| (no recent operations) | — | — | — |\n")
	}

	return summary.String()
}

// PruneSecurityAuditLogs deletes the oldest rows when the total exceeds maxRecords.
func (s *SecurityAuditDB) PruneSecurityAuditLogs(maxRecords int) error {
	if s == nil || s.db == nil {
		return nil
	}

	s.pruneMu.Lock()
	defer s.pruneMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM security_operations`).Scan(&count); err != nil {
		return fmt.Errorf("security_audit_db: prune count: %w", err)
	}

	if count <= maxRecords {
		return nil
	}

	excess := count - maxRecords
	return s.securityAuditExecWithRetry(ctx,
		`DELETE FROM security_operations
		 WHERE id IN (
		   SELECT id FROM security_operations ORDER BY id ASC LIMIT ?
		 )`,
		excess,
	)
}

// StartSecurityPruner starts a background goroutine that prunes old security audit logs.
func (s *SecurityAuditDB) StartSecurityPruner(ctx context.Context, maxRecords int) {
	if s == nil || s.db == nil {
		return
	}

	go func() {
		if err := s.PruneSecurityAuditLogs(maxRecords); err != nil {
			fmt.Fprintf(os.Stderr, "security_audit_db: startup prune: %v\n", err)
		}

		ticker := time.NewTicker(securityAuditPruneInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.PruneSecurityAuditLogs(maxRecords); err != nil {
					fmt.Fprintf(os.Stderr, "security_audit_db: scheduled prune: %v\n", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
