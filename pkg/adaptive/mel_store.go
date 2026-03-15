package adaptive

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/embeddings"
)

const (
	maxHeuristics    = 500  // Evict lowest-confidence when exceeded
	dedupThreshold   = 0.70 // Jaccard similarity above this merges entries (legacy)
	tfidfDedupThresh = 0.75 // TF-IDF cosine similarity threshold for deduplication
)

// VectorStore is a JSON-backed persistent store of heuristics. When an
// Embedder is configured via SetEmbedder, queries use cosine similarity over
// dense vectors for semantic retrieval. Without an embedder it falls back to
// the original BM25 + TF-IDF hybrid scoring.
type VectorStore struct {
	mu         sync.RWMutex
	Heuristics []*Heuristic `json:"heuristics"`
	path       string
	embedder   embeddings.Embedder // nil → keyword scoring only
}

// NewVectorStore opens (or creates) the JSON store at path.
func NewVectorStore(path string) (*VectorStore, error) {
	vs := &VectorStore{path: path}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, vs)
	}
	if vs.Heuristics == nil {
		vs.Heuristics = make([]*Heuristic, 0)
	}
	return vs, nil
}

// Add inserts a new heuristic, merging with an existing one when Jaccard
// similarity exceeds dedupThreshold.
func (vs *VectorStore) Add(h *Heuristic) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Check for near-duplicates using TF-IDF cosine similarity on full text.
	newTokens := tokenize(h.Text())
	for _, existing := range vs.Heuristics {
		existingTokens := tokenize(existing.Text())
		sim := tfidfSimilarity(newTokens, existingTokens)
		if sim >= tfidfDedupThresh {
			// Merge: adopt the higher-confidence version, bump use count.
			if h.Confidence > existing.Confidence {
				existing.Confidence = h.Confidence
				existing.Context = h.Context
				existing.Constraint = h.Constraint
				existing.Error = h.Error
			}
			existing.UseCount++
			existing.UpdatedAt = time.Now()
			vs.persist()
			return
		}
	}

	// Evict lowest-confidence entry when over capacity.
	if len(vs.Heuristics) >= maxHeuristics {
		sort.Slice(vs.Heuristics, func(i, j int) bool {
			return vs.Heuristics[i].Confidence < vs.Heuristics[j].Confidence
		})
		vs.Heuristics = vs.Heuristics[1:]
	}

	h.CreatedAt = time.Now()
	h.UpdatedAt = time.Now()
	vs.Heuristics = append(vs.Heuristics, h)
	vs.persist()
}

// Query returns up to k heuristics most relevant to the query string.
//
// Scoring strategy:
//   - When an embedder is available: 0.5×cosine-similarity + 0.3×BM25 + 0.2×TF-IDF,
//     weighted by confidence × log(1+useCount).
//   - Without an embedder: original 0.6×BM25 + 0.4×TF-IDF hybrid.
func (vs *VectorStore) Query(query string, k int) []*Heuristic {
	vs.mu.RLock()
	embedder := vs.embedder
	heuristics := vs.Heuristics
	vs.mu.RUnlock()

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 || len(heuristics) == 0 {
		return nil
	}

	// Optionally obtain a query embedding (non-blocking; 2 s deadline).
	var queryVec []float32
	if embedder != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		v, err := embedder.Embed(ctx, query)
		cancel()
		if err == nil {
			queryVec = v
		}
	}

	// Build corpus for keyword scoring.
	corpus := make([][]string, len(heuristics))
	for i, h := range heuristics {
		corpus[i] = tokenize(h.Text())
	}
	df := buildDF(corpus)
	al := avgLen(corpus)
	numDocs := len(heuristics)
	idf := buildIDF(corpus)

	type scored struct {
		h     *Heuristic
		score float64
	}
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	var results []scored
	for i, h := range heuristics {
		bm25 := bm25Score(queryTokens, corpus[i], df, numDocs, al)
		kw := hybridScore(bm25, queryTokens, corpus[i], idf) // 0.6×BM25 + 0.4×TF-IDF

		var raw float64
		if queryVec != nil && len(h.Embedding) == len(queryVec) {
			// Semantic-dominant blend when embeddings are available.
			cos := embeddings.CosineSimilarity(queryVec, h.Embedding)
			if cos < 0 {
				cos = 0 // treat anti-correlated as zero relevance
			}
			raw = 0.5*cos + 0.5*kw
		} else {
			raw = kw
		}

		if raw <= 0 {
			continue
		}
		useWeight := 1.0 + log1pUse(float64(h.UseCount))
		score := raw * h.Confidence * useWeight
		results = append(results, scored{h, score})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

	out := make([]*Heuristic, 0, k)
	for i, r := range results {
		if i >= k {
			break
		}
		out = append(out, r.h)
	}
	return out
}

// SetEmbedder wires a dense-vector embedder into the store. After this call
// Query will prefer cosine similarity over BM25/TF-IDF when the embedder is
// able to produce vectors. Thread-safe; safe to call at any time.
func (vs *VectorStore) SetEmbedder(e embeddings.Embedder) {
	vs.mu.Lock()
	vs.embedder = e
	vs.mu.Unlock()
}

// Len returns the number of stored heuristics.
func (vs *VectorStore) Len() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return len(vs.Heuristics)
}

// persist writes the store to disk (must be called with write lock held).
func (vs *VectorStore) persist() {
	data, err := json.MarshalIndent(vs, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(vs.path, data, 0600)
}

// jaccard computes |intersection| / |union| for two token slices.
func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	setA := make(map[string]struct{}, len(a))
	for _, t := range a {
		setA[t] = struct{}{}
	}
	intersection := 0
	setB := make(map[string]struct{}, len(b))
	for _, t := range b {
		setB[t] = struct{}{}
		if _, ok := setA[t]; ok {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
