package providers

import (
	"context"
	"database/sql"
	"encoding/binary"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"github.com/velariumai/gorkbot/pkg/embeddings"
)

const (
	cacheSimThreshold = 0.94               // cosine similarity required for a cache hit
	cacheTTL          = 7 * 24 * time.Hour // entries older than 7 days are pruned
	cacheMaxRows      = 5_000              // hard cap; oldest rows evicted beyond this
	cacheRecentLimit  = 200                // rows scanned per ANN search
)

// SemanticCache provides prompt-level semantic deduplication backed by SQLite
// with embeddings-based approximate nearest-neighbour lookup.
//
// Concurrency: safe for concurrent use.  All writes are async (fire-and-forget)
// via a 256-slot buffered channel drained by a single background goroutine.
type SemanticCache struct {
	db       *sql.DB
	embedder embeddings.Embedder
	logger   *slog.Logger

	mu      sync.RWMutex // protects nothing currently, reserved for hot-path cache
	writeCh chan writeEntry
	wg      sync.WaitGroup
}

type writeEntry struct {
	prompt   string
	response string
	blob     []byte
}

// NewSemanticCache opens (or creates) the SQLite cache DB under configDir and
// starts the background write-drain goroutine.
func NewSemanticCache(embedder embeddings.Embedder, configDir string) (*SemanticCache, error) {
	dbPath := filepath.Join(configDir, "semantic_cache.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// WAL mode: concurrent readers never block the writer.
	// busy_timeout: prevents SQLITE_BUSY on contention.
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS cache_entries (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			prompt      TEXT    NOT NULL,
			response    TEXT    NOT NULL,
			embedding   BLOB    NOT NULL,
			created_at  INTEGER NOT NULL DEFAULT (unixepoch())
		);
		CREATE INDEX IF NOT EXISTS idx_cache_created ON cache_entries (created_at DESC);
	`)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	c := &SemanticCache{
		db:       db,
		embedder: embedder,
		logger:   slog.Default(),
		writeCh:  make(chan writeEntry, 256),
	}

	c.wg.Add(1)
	go c.drainWrites()

	// Prune on startup in the background so it never blocks the caller.
	go c.prune()

	return c, nil
}

// drainWrites is the sole writer goroutine — serialises all INSERT operations
// so the SQLite WAL never has two concurrent writers.
func (c *SemanticCache) drainWrites() {
	defer c.wg.Done()
	for e := range c.writeCh {
		_, err := c.db.Exec(
			`INSERT INTO cache_entries (prompt, response, embedding) VALUES (?, ?, ?)`,
			e.prompt, e.response, e.blob,
		)
		if err != nil {
			c.logger.Debug("semantic cache: write failed", "error", err)
		}
	}
}

// prune removes entries older than cacheTTL and keeps the table under cacheMaxRows.
func (c *SemanticCache) prune() {
	cutoff := time.Now().Add(-cacheTTL).Unix()
	c.db.Exec(`DELETE FROM cache_entries WHERE created_at < ?`, cutoff)

	// Evict oldest rows beyond the hard cap (single DELETE with LIMIT).
	c.db.Exec(`
		DELETE FROM cache_entries WHERE id IN (
			SELECT id FROM cache_entries ORDER BY created_at ASC
			LIMIT MAX(0, (SELECT COUNT(*) FROM cache_entries) - ?)
		)`, cacheMaxRows)
}

// GetCachedResponse returns a previously cached response whose prompt embedding
// is within cacheSimThreshold cosine distance of the given prompt.
// Returns ("", false) on any miss, embedder error, or DB error.
func (c *SemanticCache) GetCachedResponse(ctx context.Context, prompt string) (string, bool) {
	if c == nil || c.embedder == nil {
		return "", false
	}

	vec, err := c.embedder.Embed(ctx, prompt)
	if err != nil {
		c.logger.Debug("semantic cache: embedder unavailable for lookup", "error", err)
		return "", false
	}

	// Fetch the most recent cacheRecentLimit entries for ANN scan.
	rows, err := c.db.QueryContext(ctx,
		`SELECT response, embedding FROM cache_entries
		 ORDER BY created_at DESC LIMIT ?`, cacheRecentLimit)
	if err != nil {
		return "", false
	}
	defer rows.Close()

	var bestResponse string
	var bestScore float64 = -1.0

	for rows.Next() {
		var resp string
		var blob []byte
		if err := rows.Scan(&resp, &blob); err != nil {
			continue
		}
		cachedVec := blobToFloat32s(blob)
		if score := embeddings.CosineSimilarity(vec, cachedVec); score > bestScore {
			bestScore = score
			bestResponse = resp
		}
	}

	if bestScore >= cacheSimThreshold {
		c.logger.Debug("semantic cache: hit", "score", bestScore)
		return bestResponse, true
	}
	return "", false
}

// StoreResponse asynchronously persists a prompt→response pair.
// The embedding is computed synchronously (to avoid a data race on the prompt
// string), then the write is handed off to the drain goroutine.
func (c *SemanticCache) StoreResponse(ctx context.Context, prompt, response string) {
	if c == nil || c.embedder == nil {
		return
	}
	vec, err := c.embedder.Embed(ctx, prompt)
	if err != nil {
		c.logger.Debug("semantic cache: embedder unavailable for store", "error", err)
		return
	}
	blob := float32sToBlob(vec)
	select {
	case c.writeCh <- writeEntry{prompt: prompt, response: response, blob: blob}:
	default:
		// Write channel full — drop silently; prefer responsiveness over completeness.
		c.logger.Debug("semantic cache: write channel full, entry dropped")
	}
}

// Close flushes all pending writes and closes the DB.
func (c *SemanticCache) Close() {
	close(c.writeCh)
	c.wg.Wait()
	_ = c.db.Close()
}

// ── encoding helpers ─────────────────────────────────────────────────────────

func float32sToBlob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func blobToFloat32s(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
