package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
)

// SQLiteFactSearcher implements triple-based search on SQLite
type SQLiteFactSearcher struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewSQLiteFactSearcher creates a fact searcher for triple matching
func NewSQLiteFactSearcher(db *sql.DB, logger *slog.Logger) *SQLiteFactSearcher {
	if logger == nil {
		logger = slog.Default()
	}

	fs := &SQLiteFactSearcher{
		db:     db,
		logger: logger,
	}

	// Ensure facts table exists
	if err := fs.ensureFactsTable(); err != nil {
		logger.Warn("facts table initialization failed", slog.String("error", err.Error()))
	}

	return fs
}

// ensureFactsTable creates the facts table if it doesn't exist
func (fs *SQLiteFactSearcher) ensureFactsTable() error {
	createStmt := `
	CREATE TABLE IF NOT EXISTS facts (
		fact_id TEXT PRIMARY KEY,
		subject TEXT NOT NULL,
		predicate TEXT NOT NULL,
		object TEXT NOT NULL,
		confidence REAL DEFAULT 1.0,
		source TEXT,
		timestamp TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_subject ON facts(subject);
	CREATE INDEX IF NOT EXISTS idx_predicate ON facts(predicate);
	CREATE INDEX IF NOT EXISTS idx_object ON facts(object);
	CREATE INDEX IF NOT EXISTS idx_confidence ON facts(confidence DESC);
	`

	_, err := fs.db.Exec(createStmt)
	return err
}

// InsertFact stores a fact in the database
func (fs *SQLiteFactSearcher) InsertFact(
	factID string,
	subject, predicate, object string,
	confidence float64,
	source, timestamp string,
) error {
	insertStmt := `
	INSERT INTO facts (fact_id, subject, predicate, object, confidence, source, timestamp)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(fact_id) DO UPDATE SET
		subject = excluded.subject,
		predicate = excluded.predicate,
		object = excluded.object,
		confidence = excluded.confidence,
		source = excluded.source,
		timestamp = excluded.timestamp,
		updated_at = CURRENT_TIMESTAMP
	`

	_, err := fs.db.ExecContext(context.Background(), insertStmt,
		factID, subject, predicate, object, confidence, source, timestamp)
	return err
}

// Search performs triple-pattern matching
// Supports wildcards: empty string = match any
func (fs *SQLiteFactSearcher) Search(
	ctx context.Context,
	subject, predicate, object string,
	k int,
) ([]SearchResult, error) {
	if k <= 0 {
		k = 8
	}

	// Build WHERE clause with only non-empty patterns
	var whereConditions []string
	var args []interface{}

	if subject != "" {
		whereConditions = append(whereConditions, "subject LIKE ?")
		args = append(args, "%"+subject+"%")
	}

	if predicate != "" {
		whereConditions = append(whereConditions, "predicate LIKE ?")
		args = append(args, "%"+predicate+"%")
	}

	if object != "" {
		whereConditions = append(whereConditions, "object LIKE ?")
		args = append(args, "%"+object+"%")
	}

	// If no patterns provided, return top facts by confidence
	if len(whereConditions) == 0 {
		return fs.topFacts(ctx, k)
	}

	whereClause := strings.Join(whereConditions, " AND ")
	searchStmt := fmt.Sprintf(`
	SELECT
		fact_id,
		subject,
		predicate,
		object,
		confidence,
		source,
		timestamp
	FROM facts
	WHERE %s
	ORDER BY confidence DESC, timestamp DESC
	LIMIT ?
	`, whereClause)

	args = append(args, k)

	rows, err := fs.db.QueryContext(ctx, searchStmt, args...)
	if err != nil {
		return nil, fmt.Errorf("fact search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult

		err := rows.Scan(
			&r.FactID,
			&r.Subject,
			&r.Predicate,
			&r.Object,
			&r.Confidence,
			&r.Source,
			&r.Timestamp,
		)
		if err != nil {
			fs.logger.Warn("failed to scan fact", slog.String("error", err.Error()))
			continue
		}

		r.RelevanceScore = r.Confidence
		r.Source_ = "fact"

		results = append(results, r)
	}

	return results, rows.Err()
}

// topFacts returns the top-K facts by confidence
func (fs *SQLiteFactSearcher) topFacts(ctx context.Context, k int) ([]SearchResult, error) {
	searchStmt := `
	SELECT
		fact_id,
		subject,
		predicate,
		object,
		confidence,
		source,
		timestamp
	FROM facts
	ORDER BY confidence DESC, timestamp DESC
	LIMIT ?
	`

	rows, err := fs.db.QueryContext(ctx, searchStmt, k)
	if err != nil {
		return nil, fmt.Errorf("topFacts query failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult

		err := rows.Scan(
			&r.FactID,
			&r.Subject,
			&r.Predicate,
			&r.Object,
			&r.Confidence,
			&r.Source,
			&r.Timestamp,
		)
		if err != nil {
			fs.logger.Warn("failed to scan top fact", slog.String("error", err.Error()))
			continue
		}

		r.RelevanceScore = r.Confidence
		r.Source_ = "fact"

		results = append(results, r)
	}

	return results, rows.Err()
}

// Close cleanup (no-op since DB is managed externally)
func (fs *SQLiteFactSearcher) Close() error {
	return nil
}

// RemoveFact deletes a fact from the database
func (fs *SQLiteFactSearcher) RemoveFact(factID string) error {
	_, err := fs.db.Exec("DELETE FROM facts WHERE fact_id = ?", factID)
	return err
}

// GetFactCount returns the total number of stored facts
func (fs *SQLiteFactSearcher) GetFactCount(ctx context.Context) (int64, error) {
	var count int64
	err := fs.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM facts").Scan(&count)
	return count, err
}

// FindDuplicates finds semantically similar facts (same subject+predicate)
// Used for deduplication
func (fs *SQLiteFactSearcher) FindDuplicates(
	ctx context.Context,
	subject, predicate string,
) ([]SearchResult, error) {
	searchStmt := `
	SELECT
		fact_id,
		subject,
		predicate,
		object,
		confidence,
		source,
		timestamp
	FROM facts
	WHERE subject = ? AND predicate = ?
	ORDER BY confidence DESC
	`

	rows, err := fs.db.QueryContext(ctx, searchStmt, subject, predicate)
	if err != nil {
		return nil, fmt.Errorf("duplicate search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult

		err := rows.Scan(
			&r.FactID,
			&r.Subject,
			&r.Predicate,
			&r.Object,
			&r.Confidence,
			&r.Source,
			&r.Timestamp,
		)
		if err != nil {
			fs.logger.Warn("failed to scan duplicate", slog.String("error", err.Error()))
			continue
		}

		r.Source_ = "fact"
		results = append(results, r)
	}

	return results, rows.Err()
}

// ClearFacts deletes all facts from the database
func (fs *SQLiteFactSearcher) ClearFacts() error {
	_, err := fs.db.Exec("DELETE FROM facts")
	return err
}

// GetStats returns statistics about the fact store
func (fs *SQLiteFactSearcher) GetStats(ctx context.Context) (map[string]interface{}, error) {
	var totalFacts int64

	countErr := fs.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM facts").Scan(&totalFacts)
	if countErr != nil {
		return nil, countErr
	}

	var avgConf sql.NullFloat64
	avgErr := fs.db.QueryRowContext(ctx, "SELECT AVG(confidence) FROM facts").Scan(&avgConf)
	if avgErr != nil {
		return nil, avgErr
	}

	stats := map[string]interface{}{
		"total_facts":     totalFacts,
		"avg_confidence":  0.0,
		"storage_engine":  "sqlite",
	}

	if avgConf.Valid {
		stats["avg_confidence"] = avgConf.Float64
	}

	return stats, nil
}
