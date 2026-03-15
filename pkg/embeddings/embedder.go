// Package embeddings provides a provider-agnostic interface for generating
// dense vector embeddings from text, plus helpers for cosine similarity and
// vector arithmetic. It supports both local (llama.cpp GGUF) and cloud
// (OpenAI text-embedding-3-small, Google text-embedding-004) backends with
// automatic fallback: local → cloud → error.
package embeddings

import (
	"context"
	"math"
)

// Embedder converts a text string into a dense float32 vector.
// All implementations must return L2-normalised vectors so that cosine
// similarity reduces to a simple dot product.
type Embedder interface {
	// Embed returns an L2-normalised embedding vector for text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dims returns the output vector dimensionality.
	Dims() int

	// Name returns a human-readable identifier for logging.
	Name() string
}

// CosineSimilarity returns the cosine similarity in [-1, 1] between two
// L2-normalised vectors.  When both vectors are unit-norm this is just the
// dot product and avoids the square-root division.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	v := dot / denom
	// Clamp to [-1, 1] to guard against floating-point drift.
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

// L2Normalize returns a copy of v scaled to unit length.
// Returns a zero vector when the input norm is zero.
func L2Normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		out := make([]float32, len(v))
		return out
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}
