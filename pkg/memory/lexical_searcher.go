package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// FTS5LexicalSearcher implements BM25-like search using SQLite FTS5
type FTS5LexicalSearcher struct {
	db            *sql.DB
	logger        *slog.Logger
	fts5Available bool
}

// NewFTS5LexicalSearcher creates a lexical searcher backed by SQLite FTS5
func NewFTS5LexicalSearcher(db *sql.DB, logger *slog.Logger) *FTS5LexicalSearcher {
	if logger == nil {
		logger = slog.Default()
	}

	ls := &FTS5LexicalSearcher{
		db:     db,
		logger: logger,
	}

	// Ensure FTS5 table exists
	if err := ls.ensureFTS5Table(); err != nil {
		logger.Warn("FTS5 table initialization failed", slog.String("error", err.Error()))
		ls.fts5Available = false
	} else {
		ls.fts5Available = true
	}

	return ls
}

// ensureFTS5Table creates the FTS5 virtual table if it doesn't exist
func (ls *FTS5LexicalSearcher) ensureFTS5Table() error {
	createStmt := `
	CREATE VIRTUAL TABLE IF NOT EXISTS facts_fts USING fts5(
		fact_id UNINDEXED,
		subject,
		predicate,
		object,
		confidence UNINDEXED,
		source UNINDEXED,
		timestamp UNINDEXED
	)`

	_, err := ls.db.Exec(createStmt)
	return err
}

// IndexFact adds or updates a fact in the FTS5 index
func (ls *FTS5LexicalSearcher) IndexFact(
	factID string,
	subject, predicate, object string,
	confidence float64,
	source, timestamp string,
) error {
	if !ls.fts5Available {
		return nil
	}
	insertStmt := `
	INSERT INTO facts_fts (fact_id, subject, predicate, object, confidence, source, timestamp)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(fact_id) DO UPDATE SET
		subject = excluded.subject,
		predicate = excluded.predicate,
		object = excluded.object,
		confidence = excluded.confidence,
		source = excluded.source,
		timestamp = excluded.timestamp
	`

	_, err := ls.db.ExecContext(context.Background(), insertStmt,
		factID, subject, predicate, object, confidence, source, timestamp)
	return err
}

// Search performs FTS5 full-text search
// Uses MATCH operator for BM25-style ranking
func (ls *FTS5LexicalSearcher) Search(ctx context.Context, query string, k int) ([]SearchResult, error) {
	if k <= 0 {
		k = 8
	}

	// FTS5 MATCH query: searches all indexed columns with BM25 scoring
	if !ls.fts5Available {
		return ls.searchLike(ctx, query, k)
	}
	searchStmt := `
	SELECT
		fact_id,
		subject,
		predicate,
		object,
		confidence,
		source,
		timestamp,
		RANK as relevance_score
	FROM facts_fts
	WHERE facts_fts MATCH ?
	ORDER BY RANK
	LIMIT ?
	`

	rows, err := ls.db.QueryContext(ctx, searchStmt, query, k)
	if err != nil {
		// If FTS5 MATCH fails, fall back to simple LIKE (less accurate but always works)
		return ls.searchLike(ctx, query, k)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var rankScore sql.NullFloat64

		err := rows.Scan(
			&r.FactID,
			&r.Subject,
			&r.Predicate,
			&r.Object,
			&r.Confidence,
			&r.Source,
			&r.Timestamp,
			&rankScore,
		)
		if err != nil {
			ls.logger.Warn("failed to scan FTS5 result", slog.String("error", err.Error()))
			continue
		}

		if rankScore.Valid {
			r.RelevanceScore = rankScore.Float64
		}
		r.Source_ = "lexical"

		results = append(results, r)
	}

	return results, rows.Err()
}

// searchLike is a fallback when FTS5 MATCH fails
// Uses simple LIKE operator (slower, but always available)
func (ls *FTS5LexicalSearcher) searchLike(ctx context.Context, query string, k int) ([]SearchResult, error) {
	// Build LIKE pattern: match query in subject, predicate, or object
	likePattern := "%" + query + "%"

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
	WHERE subject LIKE ? OR predicate LIKE ? OR object LIKE ?
	ORDER BY confidence DESC
	LIMIT ?
	`

	rows, err := ls.db.QueryContext(ctx, searchStmt, likePattern, likePattern, likePattern, k)
	if err != nil {
		return nil, fmt.Errorf("lexical search (LIKE fallback) failed: %w", err)
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
			ls.logger.Warn("failed to scan LIKE result", slog.String("error", err.Error()))
			continue
		}

		r.RelevanceScore = r.Confidence // Use confidence as relevance in fallback
		r.Source_ = "lexical_like"

		results = append(results, r)
	}

	return results, rows.Err()
}

// Close cleanup (no-op for FTS5 since DB is managed externally)
func (ls *FTS5LexicalSearcher) Close() error {
	return nil
}

// RemoveFact deletes a fact from the FTS5 index
func (ls *FTS5LexicalSearcher) RemoveFact(factID string) error {
	_, err := ls.db.Exec("DELETE FROM facts_fts WHERE fact_id = ?", factID)
	return err
}

// ClearIndex drops and recreates the FTS5 table
func (ls *FTS5LexicalSearcher) ClearIndex() error {
	_, err := ls.db.Exec("DROP TABLE IF EXISTS facts_fts")
	if err != nil {
		return err
	}
	return ls.ensureFTS5Table()
}

// GetIndexStats returns statistics about the FTS5 index
func (ls *FTS5LexicalSearcher) GetIndexStats(ctx context.Context) (map[string]interface{}, error) {
	var totalFacts int64

	err := ls.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM facts_fts").Scan(&totalFacts)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"total_facts":   totalFacts,
		"search_engine": "sqlite_fts5",
		"fallback_mode": "like", // If MATCH fails
	}

	return stats, nil
}
