package memory

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/embeddings"
)

type embedderAwareSemantic interface {
	Embedder() embeddings.Embedder
}

type semanticFactSearcher struct {
	db       *sql.DB
	logger   *slog.Logger
	embedder embeddings.Embedder

	mu    sync.RWMutex
	cache map[string][]float32
}

func (s *semanticFactSearcher) Embedder() embeddings.Embedder {
	return s.embedder
}

func semanticBackendName(s SemanticSearcher) string {
	if e, ok := s.(embedderAwareSemantic); ok && e.Embedder() != nil {
		return e.Embedder().Name()
	}
	return "unknown"
}

func newSemanticSearcherFromEnv(db *sql.DB, logger *slog.Logger) (SemanticSearcher, error) {
	if db == nil {
		return nil, fmt.Errorf("nil database")
	}
	if logger == nil {
		logger = slog.Default()
	}

	embedder, err := selectEmbedderFromEnv()
	if err != nil {
		return nil, err
	}
	return &semanticFactSearcher{
		db:       db,
		logger:   logger,
		embedder: embedder,
		cache:    make(map[string][]float32),
	}, nil
}

func selectEmbedderFromEnv() (embeddings.Embedder, error) {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("GORKBOT_MEMORY_EMBEDDER")))
	switch backend {
	case "", "simple":
		return newSimpleEmbedder(64), nil
	case "ollama":
		return embeddings.NewOllamaEmbedder(
			os.Getenv("GORKBOT_MEMORY_OLLAMA_URL"),
			os.Getenv("GORKBOT_MEMORY_OLLAMA_MODEL"),
		), nil
	case "openai":
		apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required for openai embedder")
		}
		return embeddings.NewOpenAIEmbedder(apiKey), nil
	case "google":
		apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
		}
		if apiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY or GOOGLE_API_KEY is required for google embedder")
		}
		return embeddings.NewGoogleEmbedder(apiKey), nil
	default:
		return nil, fmt.Errorf("unsupported semantic embedder backend: %s", backend)
	}
}

func (s *semanticFactSearcher) Search(ctx context.Context, query string, k int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return []SearchResult{}, nil
	}
	if k <= 0 {
		k = 8
	}

	qv, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query embedding failed: %w", err)
	}

	limit := k * 8
	if limit < 50 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT fact_id, subject, predicate, object, confidence, source, timestamp
		FROM facts
		ORDER BY confidence DESC, timestamp DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("semantic facts query failed: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0, limit)
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.FactID, &r.Subject, &r.Predicate, &r.Object, &r.Confidence, &r.Source, &r.Timestamp); err != nil {
			continue
		}
		content := r.Subject + " " + r.Predicate + " " + r.Object
		vec, err := s.embeddingForFact(ctx, r.FactID, content)
		if err != nil {
			continue
		}
		similarity := embeddings.CosineSimilarity(qv, vec)
		r.RelevanceScore = ((similarity + 1.0) / 2.0 * 0.8) + (r.Confidence * 0.2)
		r.Source_ = "semantic"
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})
	if len(results) > k {
		results = results[:k]
	}
	return results, nil
}

func (s *semanticFactSearcher) embeddingForFact(ctx context.Context, factID, text string) ([]float32, error) {
	s.mu.RLock()
	if vec, ok := s.cache[factID]; ok {
		s.mu.RUnlock()
		return vec, nil
	}
	s.mu.RUnlock()

	vec, err := s.embedder.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[factID] = vec
	s.mu.Unlock()
	return vec, nil
}

func (s *semanticFactSearcher) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string][]float32)
	return nil
}

type embeddingReranker struct {
	embedder embeddings.Embedder
}

func newEmbeddingReranker(e embeddings.Embedder) Reranker {
	return &embeddingReranker{embedder: e}
}

func (r *embeddingReranker) Rerank(ctx context.Context, query string, candidates []SearchResult) ([]SearchResult, error) {
	if r.embedder == nil || len(candidates) == 0 || strings.TrimSpace(query) == "" {
		return candidates, nil
	}

	qv, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, len(candidates))
	copy(out, candidates)
	for i := range out {
		text := out[i].Subject + " " + out[i].Predicate + " " + out[i].Object
		vec, err := r.embedder.Embed(ctx, text)
		if err != nil {
			continue
		}
		sim := embeddings.CosineSimilarity(qv, vec)
		out[i].RelevanceScore = (out[i].RelevanceScore * 0.65) + (((sim + 1) / 2) * 0.35)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RelevanceScore > out[j].RelevanceScore
	})
	return out, nil
}

func (r *embeddingReranker) Close() error { return nil }

type simpleEmbedder struct {
	dims int
}

func newSimpleEmbedder(dims int) embeddings.Embedder {
	if dims <= 0 {
		dims = 64
	}
	return &simpleEmbedder{dims: dims}
}

func (e *simpleEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	vec := make([]float32, e.dims)
	parts := strings.Fields(strings.ToLower(text))
	for _, p := range parts {
		h := fnv.New64a()
		_, _ = h.Write([]byte(p))
		idx := int(h.Sum64() % uint64(e.dims))
		vec[idx] += 1.0
	}
	return embeddings.L2Normalize(vec), nil
}

func (e *simpleEmbedder) Dims() int    { return e.dims }
func (e *simpleEmbedder) Name() string { return "simple-hash-embedder" }
