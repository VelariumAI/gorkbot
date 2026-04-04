// Package api provides audit logging for the API server.
package api

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// AuditLogger provides audit trail functionality via SQLite.
type AuditLogger struct {
	db     *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
}

// AuditEntry represents a logged API/connector action.
type AuditEntry struct {
	ID            int64
	Timestamp     time.Time
	CorrelationID string
	UserID        string
	Action        string // "api_request", "connector_message", "connector_response"
	Resource      string // "/api/v1/message", "telegram", "discord", etc.
	Method        string // "POST", "GET", etc. (empty for connector actions)
	Status        string // "success", "error", etc.
	Details       string // Additional JSON context
	Duration      int64  // Milliseconds
}

// NewAuditLogger creates a new audit logger with SQLite backend.
func NewAuditLogger(dbPath string, logger *slog.Logger) (*AuditLogger, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Open or create SQLite database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit database: %w", err)
	}

	// Create audit table if it doesn't exist
	schema := `
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		correlation_id TEXT NOT NULL,
		user_id TEXT,
		action TEXT NOT NULL,
		resource TEXT NOT NULL,
		method TEXT,
		status TEXT,
		details TEXT,
		duration_ms INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_correlation_id ON audit_log(correlation_id);
	CREATE INDEX IF NOT EXISTS idx_user_id ON audit_log(user_id);
	CREATE INDEX IF NOT EXISTS idx_action ON audit_log(action);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create audit schema: %w", err)
	}

	al := &AuditLogger{
		db:     db,
		logger: logger,
	}

	logger.Info("audit logger initialized", "db_path", dbPath)
	return al, nil
}

// LogAPIRequest logs an API request.
func (al *AuditLogger) LogAPIRequest(correlationID, userID, method, resource, status string, durationMs int64, details string) error {
	return al.log(&AuditEntry{
		CorrelationID: correlationID,
		UserID:        userID,
		Action:        "api_request",
		Resource:      resource,
		Method:        method,
		Status:        status,
		Duration:      durationMs,
		Details:       details,
	})
}

// LogConnectorMessage logs a message sent via a connector.
func (al *AuditLogger) LogConnectorMessage(correlationID, userID, connectorName, targetID, status string, details string) error {
	return al.log(&AuditEntry{
		CorrelationID: correlationID,
		UserID:        userID,
		Action:        "connector_message",
		Resource:      connectorName,
		Status:        status,
		Details:       fmt.Sprintf(`{"target_id": "%s", "details": "%s"}`, targetID, details),
	})
}

// LogConnectorEvent logs a connector event (e.g., message received).
func (al *AuditLogger) LogConnectorEvent(correlationID, userID, connectorName, eventType, status string, details string) error {
	return al.log(&AuditEntry{
		CorrelationID: correlationID,
		UserID:        userID,
		Action:        fmt.Sprintf("connector_%s", eventType),
		Resource:      connectorName,
		Status:        status,
		Details:       details,
	})
}

// log inserts an audit entry into the database.
func (al *AuditLogger) log(entry *AuditEntry) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	query := `
	INSERT INTO audit_log (correlation_id, user_id, action, resource, method, status, details, duration_ms)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := al.db.Exec(query,
		entry.CorrelationID,
		entry.UserID,
		entry.Action,
		entry.Resource,
		entry.Method,
		entry.Status,
		entry.Details,
		entry.Duration,
	)

	if err != nil {
		al.logger.Error("failed to log audit entry", "error", err, "action", entry.Action)
		return err
	}

	return nil
}

// QueryByCorrelationID retrieves all audit entries for a correlation ID.
func (al *AuditLogger) QueryByCorrelationID(correlationID string) ([]AuditEntry, error) {
	al.mu.Lock()
	defer al.mu.Unlock()

	query := `SELECT id, timestamp, correlation_id, user_id, action, resource, method, status, details, duration_ms FROM audit_log WHERE correlation_id = ? ORDER BY timestamp DESC`

	rows, err := al.db.Query(query, correlationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		var ts string
		if err := rows.Scan(&entry.ID, &ts, &entry.CorrelationID, &entry.UserID, &entry.Action, &entry.Resource, &entry.Method, &entry.Status, &entry.Details, &entry.Duration); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, ts)
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// QueryByUserID retrieves recent audit entries for a user.
func (al *AuditLogger) QueryByUserID(userID string, limit int) ([]AuditEntry, error) {
	al.mu.Lock()
	defer al.mu.Unlock()

	query := `SELECT id, timestamp, correlation_id, user_id, action, resource, method, status, details, duration_ms FROM audit_log WHERE user_id = ? ORDER BY timestamp DESC LIMIT ?`

	rows, err := al.db.Query(query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		var ts string
		if err := rows.Scan(&entry.ID, &ts, &entry.CorrelationID, &entry.UserID, &entry.Action, &entry.Resource, &entry.Method, &entry.Status, &entry.Details, &entry.Duration); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, ts)
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// QueryByAction retrieves audit entries by action type.
func (al *AuditLogger) QueryByAction(action string, limit int) ([]AuditEntry, error) {
	al.mu.Lock()
	defer al.mu.Unlock()

	query := `SELECT id, timestamp, correlation_id, user_id, action, resource, method, status, details, duration_ms FROM audit_log WHERE action = ? ORDER BY timestamp DESC LIMIT ?`

	rows, err := al.db.Query(query, action, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		var ts string
		if err := rows.Scan(&entry.ID, &ts, &entry.CorrelationID, &entry.UserID, &entry.Action, &entry.Resource, &entry.Method, &entry.Status, &entry.Details, &entry.Duration); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, ts)
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// Close closes the audit database connection.
func (al *AuditLogger) Close() error {
	return al.db.Close()
}
