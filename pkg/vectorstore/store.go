// Package vectorstore provides a lightweight in-process vector search store
// backed by the existing SQLite database. It embeds conversation turns using
// the embeddings.Embedder interface and retrieves semantically similar past
// messages via cosine similarity computed in-process.
package vectorstore

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/velariumai/gorkbot/pkg/embeddings"
)

// RAGResult holds a single retrieval result from the vector store.
type RAGResult struct {
	Content   string
	Role      string
	Score     float64
	SessionID string
}

// VectorStore wraps a sql.DB and an embedder for semantic search.
type VectorStore struct {
	db       *sql.DB
	embedder embeddings.Embedder
	dims     int
}

// New creates a VectorStore. Call Init before use.
func New(db *sql.DB, embedder embeddings.Embedder) *VectorStore {
	if db == nil || embedder == nil {
		return nil
	}
	return &VectorStore{db: db, embedder: embedder, dims: embedder.Dims()}
}

// InitSchema creates the vector_embeddings table in db without requiring an
// embedder. Use this for early schema migration at startup before the embedder
// is available.
func InitSchema(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return initSchema(db)
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS vector_embeddings (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT    NOT NULL,
			turn_role  TEXT    NOT NULL,
			content    TEXT    NOT NULL,
			embedding  BLOB    NOT NULL,
			dims       INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_vec_session ON vector_embeddings(session_id);
		CREATE INDEX IF NOT EXISTS idx_vec_created ON vector_embeddings(created_at DESC);
	`)
	return err
}

// Init creates the vector_embeddings table and indices if they do not exist.
func (vs *VectorStore) Init(db *sql.DB) error {
	if db != nil {
		vs.db = db
	}
	return initSchema(vs.db)
}

// IndexTurn embeds content and stores it asynchronously. Errors are silently
// dropped — indexing is best-effort and must never block the hot path.
func (vs *VectorStore) IndexTurn(ctx context.Context, sessionID, role, content string) {
	if vs == nil || content == "" {
		return
	}
	vec, err := vs.embedder.Embed(ctx, content)
	if err != nil {
		return
	}
	blob := float32sToBlob(vec)
	vs.db.ExecContext(ctx, //nolint:errcheck
		`INSERT INTO vector_embeddings(session_id,turn_role,content,embedding,dims) VALUES(?,?,?,?,?)`,
		sessionID, role, content, blob, len(vec),
	)
}

// Search embeds query and returns the top-K most similar stored messages.
func (vs *VectorStore) Search(ctx context.Context, query string, topK int) ([]RAGResult, error) {
	if vs == nil {
		return nil, nil
	}
	queryVec, err := vs.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: embed query: %w", err)
	}
	queryNorm := embeddings.L2Normalize(queryVec)

	rows, err := vs.db.QueryContext(ctx,
		`SELECT session_id, turn_role, content, embedding, dims FROM vector_embeddings ORDER BY created_at DESC LIMIT 2000`)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: query: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		RAGResult
		vec []float32
	}
	var candidates []candidate

	for rows.Next() {
		var (
			sid     string
			role    string
			content string
			blob    []byte
			dims    int
		)
		if err := rows.Scan(&sid, &role, &content, &blob, &dims); err != nil {
			continue
		}
		vec := blobToFloat32s(blob, dims)
		candidates = append(candidates, candidate{
			RAGResult: RAGResult{Content: content, Role: role, SessionID: sid},
			vec:       vec,
		})
	}

	// Score each candidate.
	for i := range candidates {
		norm := embeddings.L2Normalize(candidates[i].vec)
		candidates[i].Score = embeddings.CosineSimilarity(queryNorm, norm)
	}

	// Sort descending by score.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if topK > len(candidates) {
		topK = len(candidates)
	}
	results := make([]RAGResult, topK)
	for i := 0; i < topK; i++ {
		results[i] = candidates[i].RAGResult
	}
	return results, nil
}

// FormatResults formats retrieval results into a system-message block, capping
// at approximately maxTokens tokens (4 chars ≈ 1 token).
func FormatResults(results []RAGResult, maxTokens int) string {
	if len(results) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Relevant Past Context\n")
	remaining := maxTokens * 4 // convert to chars
	for _, r := range results {
		if r.Score < 0.35 {
			continue // skip low-confidence matches
		}
		line := fmt.Sprintf("- [%s]: %s\n", r.Role, r.Content)
		if len(line) > remaining {
			break
		}
		sb.WriteString(line)
		remaining -= len(line)
	}
	result := sb.String()
	if result == "### Relevant Past Context\n" {
		return ""
	}
	return result
}

// ── binary encoding helpers ───────────────────────────────────────────────────

func float32sToBlob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(b[i*4:], bits)
	}
	return b
}

func blobToFloat32s(b []byte, dims int) []float32 {
	n := len(b) / 4
	if dims > 0 && dims < n {
		n = dims
	}
	v := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		v[i] = math.Float32frombits(bits)
	}
	return v
}
