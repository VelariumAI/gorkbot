package xskill

// vector.go — Pure Go float64 cosine similarity and top-k retrieval.
//
// Design constraints (from XSKILL spec):
//
//   - Zero CGO: no wrappers for FAISS, Chroma, or any C++ vector library.
//   - Uses only the standard math package.
//   - float64 vectors (distinct from pkg/embeddings which uses float32).
//   - Safe against zero-vectors, nil slices, and length mismatches.
//
// These functions are package-internal helpers called by KnowledgeBase and
// InferenceEngine; they are not part of any exported interface.

import (
	"math"
	"sort"
)

// CosineSimilarity64 returns the cosine similarity in [-1, 1] between two
// float64 vectors.  It is a pure Go implementation with zero CGO dependencies,
// suitable for Windows / Linux / macOS / ARM without any native libraries.
//
// Special cases — all return 0 without panicking:
//   - Either vector is nil or has length 0.
//   - The vectors have different lengths.
//   - Either vector has L2 norm == 0 (zero vector — avoids division by zero).
//
// The result is clamped to [-1, 1] to guard against floating-point rounding
// that can push the dot product slightly outside the valid range.
func CosineSimilarity64(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	// Guard: if either vector is all zeros, similarity is undefined → return 0.
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	v := dot / denom

	// Clamp to [-1, 1] to handle floating-point drift at the boundaries
	// (e.g. a perfectly identical pair can compute to 1.0000000000000002).
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

// scoredExp pairs an experience bank index with its cosine similarity score.
type scoredExp struct {
	idx   int
	score float64
}

// TopKExperiences returns the indices (into bank) of the top-k experiences
// ranked by cosine similarity to query, in descending score order.
//
// Experiences without a vector (len == 0) are silently skipped, so the result
// may contain fewer than k entries.  Duplicate indices are never returned.
//
// This function takes a read-only view of the experience slice; the caller is
// responsible for holding kb.mu.RLock() if needed.
func TopKExperiences(query []float64, bank []Experience, k int) []int {
	if len(query) == 0 || len(bank) == 0 || k <= 0 {
		return nil
	}

	// Score every experience that has an embedding vector.
	scored := make([]scoredExp, 0, len(bank))
	for i, exp := range bank {
		if len(exp.Vector) == 0 {
			continue // un-embedded entry — skip
		}
		s := CosineSimilarity64(query, exp.Vector)
		scored = append(scored, scoredExp{idx: i, score: s})
	}

	if len(scored) == 0 {
		return nil
	}

	// Sort descending by score.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Collect up to k unique indices.
	if k > len(scored) {
		k = len(scored)
	}
	result := make([]int, k)
	for i := 0; i < k; i++ {
		result[i] = scored[i].idx
	}
	return result
}

// deduplicateIndices removes duplicate values from a slice of ints while
// preserving the original order.  Used to deduplicate retrieval results
// gathered across multiple sub-task queries in Phase 2.
func deduplicateIndices(indices []int) []int {
	seen := make(map[int]bool, len(indices))
	out := make([]int, 0, len(indices))
	for _, idx := range indices {
		if !seen[idx] {
			seen[idx] = true
			out = append(out, idx)
		}
	}
	return out
}
