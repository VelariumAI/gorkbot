// Package memory — factstore.go
//
// FactStore implements LLM-based fact extraction with SQLite persistence.
// Facts are queried using keyword search and returned as formatted context
// for the orchestrator.
//
// Schema: facts table with session_id, content, source, timestamps, occurrence count
// Deduplication: UNIQUE constraint on content ensures global uniqueness
// Extraction: background goroutine with 5-second debounce timer
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// StoredFact represents a single extracted fact persisted in FactStore.
type StoredFact struct {
	ID          int64
	SessionID   string
	Content     string
	Source      string
	CreatedAt   time.Time
	LastSeenAt  time.Time
	Occurrences int
}

// FactStore manages SQLite-based fact persistence with LLM extraction.
type FactStore struct {
	db        *sql.DB
	sessionID string
	provider  ai.AIProvider
	mu        sync.Mutex
	pending   []string    // queued messages for extraction
	timer     *time.Timer // debounce timer (5s)
	stopCh    chan struct{}
	stopped   bool
}

const (
	debounceInterval   = 5 * time.Second
	maxPendingMessages = 100
)

// NewFactStore opens or creates a facts database and returns a new FactStore.
// The database is located at <dataDir>/facts.db.
func NewFactStore(dataDir, sessionID string, provider ai.AIProvider) (*FactStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "facts.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create schema if needed
	if err := createFactsSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	fs := &FactStore{
		db:        db,
		sessionID: sessionID,
		provider:  provider,
		stopCh:    make(chan struct{}),
	}

	// Start background extraction goroutine
	go fs.extractionLoop()

	return fs, nil
}

func createFactsSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS facts (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id   TEXT NOT NULL,
		content      TEXT NOT NULL,
		source       TEXT NOT NULL DEFAULT 'conversation',
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		occurrences  INTEGER DEFAULT 1,
		UNIQUE(content)
	);
	CREATE INDEX IF NOT EXISTS idx_facts_session ON facts(session_id);
	CREATE INDEX IF NOT EXISTS idx_facts_seen    ON facts(last_seen_at DESC);
	`

	_, err := db.Exec(schema)
	return err
}

// QueueForExtraction appends a message to the pending extraction queue.
// If the queue reaches maxPendingMessages, it's flushed immediately.
// Safe to call from any goroutine.
func (fs *FactStore) QueueForExtraction(content string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.stopped {
		return
	}

	if content = strings.TrimSpace(content); content == "" {
		return
	}

	fs.pending = append(fs.pending, content)

	// If queue is full, flush immediately
	if len(fs.pending) >= maxPendingMessages {
		go func() {
			messages := make([]string, len(fs.pending))
			copy(messages, fs.pending)
			fs.pending = fs.pending[:0]
			if err := fs.flushPending(context.Background(), messages); err != nil {
				// Silent error (could log but keeping minimal for now)
			}
		}()
	} else if fs.timer == nil {
		// Start/reset the debounce timer
		fs.timer = time.AfterFunc(debounceInterval, func() {
			fs.mu.Lock()
			defer fs.mu.Unlock()

			if len(fs.pending) > 0 {
				messages := make([]string, len(fs.pending))
				copy(messages, fs.pending)
				fs.pending = fs.pending[:0]

				go func() {
					if err := fs.flushPending(context.Background(), messages); err != nil {
						// Silent error
					}
				}()
			}
			fs.timer = nil
		})
	}
}

// extractionLoop runs in a background goroutine.
// It drains the pending queue on demand or when stopped.
func (fs *FactStore) extractionLoop() {
	for {
		select {
		case <-fs.stopCh:
			// Drain pending on shutdown
			fs.mu.Lock()
			if len(fs.pending) > 0 {
				messages := make([]string, len(fs.pending))
				copy(messages, fs.pending)
				fs.pending = fs.pending[:0]
				fs.mu.Unlock()

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_ = fs.flushPending(ctx, messages)
				cancel()
			} else {
				fs.mu.Unlock()
			}
			return
		}
	}
}

// flushPending extracts facts from queued messages and upserts them.
func (fs *FactStore) flushPending(ctx context.Context, messages []string) error {
	if len(messages) == 0 {
		return nil
	}

	// Extract facts using LLM
	facts, err := fs.extractFacts(ctx, messages)
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Upsert facts
	for _, fact := range facts {
		if err := fs.upsert(ctx, fact, "conversation"); err != nil {
			// Continue on per-fact errors
			continue
		}
	}

	return nil
}

// extractFacts calls the LLM to extract facts from the given messages.
func (fs *FactStore) extractFacts(ctx context.Context, messages []string) ([]string, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	// Build prompt
	combined := strings.Join(messages, "\n")
	prompt := fmt.Sprintf(`Extract factual statements worth remembering from the following text.
Output one fact per line, plain text, no bullets or numbers.
Facts should be concise (under 100 chars) and factual.

Text:
---
%s
---

Facts:`, combined)

	// Call provider
	response, err := fs.provider.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Parse response: one fact per line
	var facts []string
	for _, line := range strings.Split(response, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "-") {
			facts = append(facts, line)
		}
	}

	return facts, nil
}

// upsert inserts a new fact or updates its last_seen_at and occurrences if it exists.
func (fs *FactStore) upsert(ctx context.Context, content, source string) error {
	query := `
	INSERT INTO facts (session_id, content, source, created_at, last_seen_at, occurrences)
	VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 1)
	ON CONFLICT(content) DO UPDATE SET
		last_seen_at = CURRENT_TIMESTAMP,
		occurrences = occurrences + 1
	`

	_, err := fs.db.ExecContext(ctx, query, fs.sessionID, content, source)
	return err
}

// QueryRelevant searches for facts related to the query using keyword matching.
// Returns up to 'limit' facts sorted by occurrence count and recency.
func (fs *FactStore) QueryRelevant(queryText string, limit int) ([]StoredFact, error) {
	// Extract longest word (>4 chars) as keyword
	keyword := ""
	for _, word := range strings.FieldsFunc(queryText, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n'
	}) {
		word = strings.ToLower(word)
		if len(word) > 4 && len(word) > len(keyword) {
			keyword = word
		}
	}

	if keyword == "" {
		// No good keyword, return top facts
		sqlQuery := `
		SELECT id, session_id, content, source, created_at, last_seen_at, occurrences
		FROM facts
		WHERE session_id = ?
		ORDER BY occurrences DESC, last_seen_at DESC
		LIMIT ?
		`
		return queryFacts(fs.db, sqlQuery, fs.sessionID, limit)
	}

	// Search by keyword
	sqlQuery := `
	SELECT id, session_id, content, source, created_at, last_seen_at, occurrences
	FROM facts
	WHERE session_id = ? AND content LIKE ?
	ORDER BY occurrences DESC, last_seen_at DESC
	LIMIT ?
	`
	return queryFacts(fs.db, sqlQuery, fs.sessionID, "%"+keyword+"%", limit)
}

// queryFacts is a helper to execute a fact query and return results.
func queryFacts(db *sql.DB, query string, args ...interface{}) ([]StoredFact, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []StoredFact
	for rows.Next() {
		var f StoredFact
		if err := rows.Scan(&f.ID, &f.SessionID, &f.Content, &f.Source, &f.CreatedAt, &f.LastSeenAt, &f.Occurrences); err != nil {
			continue
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

// FormatForContext formats facts as a bullet-list string suitable for injection into context.
// Truncates to maxChars to fit within context size limits.
func (fs *FactStore) FormatForContext(query string, maxChars int) string {
	facts, err := fs.QueryRelevant(query, 20)
	if err != nil || len(facts) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, fact := range facts {
		line := fmt.Sprintf("- %s\n", fact.Content)
		if sb.Len()+len(line) > maxChars {
			break
		}
		sb.WriteString(line)
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		return ""
	}
	return result
}

// Close stops the extraction loop and closes the database.
// Drains pending facts synchronously.
func (fs *FactStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.stopped {
		return nil
	}

	fs.stopped = true

	// Cancel timer if pending
	if fs.timer != nil {
		fs.timer.Stop()
		fs.timer = nil
	}

	// Signal extraction loop to stop
	close(fs.stopCh)

	// Wait for extraction loop to drain (best effort)
	time.Sleep(100 * time.Millisecond)

	// Close database
	return fs.db.Close()
}
