package xskill

// vector_test.go — Unit tests for CosineSimilarity64 and TopKExperiences.
//
// These tests validate the pure Go cosine similarity function against known
// mathematical identities, and verify that TopKExperiences correctly ranks
// and deduplicates retrieval results.
//
// Run with: go test ./internal/xskill/ -run TestCosine -v

import (
	"math"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// CosineSimilarity64 tests
// ──────────────────────────────────────────────────────────────────────────────

func TestCosineSimilarity64_IdenticalVectors(t *testing.T) {
	// Two identical non-zero vectors must have similarity == 1.0.
	a := []float64{1, 2, 3, 4}
	got := CosineSimilarity64(a, a)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("identical vectors: got %v, want 1.0", got)
	}
}

func TestCosineSimilarity64_OrthogonalVectors(t *testing.T) {
	// Two orthogonal vectors must have similarity == 0.0.
	a := []float64{1, 0, 0}
	b := []float64{0, 1, 0}
	got := CosineSimilarity64(a, b)
	if math.Abs(got) > 1e-9 {
		t.Errorf("orthogonal vectors: got %v, want 0.0", got)
	}
}

func TestCosineSimilarity64_OppositeVectors(t *testing.T) {
	// Two exactly opposite vectors must have similarity == -1.0.
	a := []float64{1, 2, 3}
	b := []float64{-1, -2, -3}
	got := CosineSimilarity64(a, b)
	if math.Abs(got-(-1.0)) > 1e-9 {
		t.Errorf("opposite vectors: got %v, want -1.0", got)
	}
}

func TestCosineSimilarity64_ZeroVector(t *testing.T) {
	// A zero vector must return 0 without panicking (avoid division by zero).
	zero := []float64{0, 0, 0}
	nonZero := []float64{1, 2, 3}

	if got := CosineSimilarity64(zero, nonZero); got != 0 {
		t.Errorf("zero vector (left): got %v, want 0", got)
	}
	if got := CosineSimilarity64(nonZero, zero); got != 0 {
		t.Errorf("zero vector (right): got %v, want 0", got)
	}
	if got := CosineSimilarity64(zero, zero); got != 0 {
		t.Errorf("zero vector (both): got %v, want 0", got)
	}
}

func TestCosineSimilarity64_NilSlice(t *testing.T) {
	// Nil slices must return 0 without panicking.
	var nilVec []float64
	nonZero := []float64{1, 2, 3}

	if got := CosineSimilarity64(nil, nonZero); got != 0 {
		t.Errorf("nil left: got %v, want 0", got)
	}
	if got := CosineSimilarity64(nonZero, nil); got != 0 {
		t.Errorf("nil right: got %v, want 0", got)
	}
	if got := CosineSimilarity64(nil, nil); got != 0 {
		t.Errorf("nil both: got %v, want 0", got)
	}
	if got := CosineSimilarity64(nilVec, nilVec); got != 0 {
		t.Errorf("empty both: got %v, want 0", got)
	}
}

func TestCosineSimilarity64_LengthMismatch(t *testing.T) {
	// Vectors of different lengths must return 0 without panicking.
	a := []float64{1, 2, 3}
	b := []float64{1, 2}
	if got := CosineSimilarity64(a, b); got != 0 {
		t.Errorf("length mismatch: got %v, want 0", got)
	}
}

func TestCosineSimilarity64_ClampedResult(t *testing.T) {
	// Floating-point rounding can produce values slightly outside [-1, 1].
	// Verify the clamp guard works by constructing a near-identical unit pair.
	a := []float64{0.5773502691896258, 0.5773502691896258, 0.5773502691896258}
	// a is already L2-normalised (norm ≈ 1.0); using a with itself should
	// yield exactly 1.0 after clamping.
	got := CosineSimilarity64(a, a)
	if got > 1.0 || got < -1.0 {
		t.Errorf("result outside [-1,1]: got %v", got)
	}
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("unit vector self-similarity: got %v, want ~1.0", got)
	}
}

func TestCosineSimilarity64_KnownAngle(t *testing.T) {
	// 45-degree angle between (1,0) and (1,1)/sqrt(2) → cosine = 1/sqrt(2) ≈ 0.7071.
	a := []float64{1, 0}
	b := []float64{1.0 / math.Sqrt2, 1.0 / math.Sqrt2}
	want := 1.0 / math.Sqrt2
	got := CosineSimilarity64(a, b)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("45-degree angle: got %v, want %v", got, want)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TopKExperiences tests
// ──────────────────────────────────────────────────────────────────────────────

func TestTopKExperiences_Basic(t *testing.T) {
	// Three experiences; query should rank them by cosine similarity.
	bank := []Experience{
		{ID: "E1", Vector: []float64{1, 0, 0}},  // sim with query = 1.0
		{ID: "E2", Vector: []float64{0, 1, 0}},  // sim with query = 0.0
		{ID: "E3", Vector: []float64{-1, 0, 0}}, // sim with query = -1.0
	}
	query := []float64{1, 0, 0}

	got := TopKExperiences(query, bank, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	// Best match should be E1 (index 0).
	if got[0] != 0 {
		t.Errorf("top-1 should be index 0 (E1), got %d", got[0])
	}
	// Worst match should be E3 (index 2).
	if got[2] != 2 {
		t.Errorf("top-3 should be index 2 (E3), got %d", got[2])
	}
}

func TestTopKExperiences_KLessThanBank(t *testing.T) {
	bank := []Experience{
		{ID: "E1", Vector: []float64{1, 0}},
		{ID: "E2", Vector: []float64{0, 1}},
		{ID: "E3", Vector: []float64{-1, 0}},
	}
	query := []float64{1, 0}

	got := TopKExperiences(query, bank, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
}

func TestTopKExperiences_EmptyBank(t *testing.T) {
	// Empty bank must return nil without panicking.
	got := TopKExperiences([]float64{1, 0}, nil, 3)
	if got != nil {
		t.Errorf("empty bank: expected nil, got %v", got)
	}
}

func TestTopKExperiences_NoVectors(t *testing.T) {
	// All experiences have empty vectors — should return nil.
	bank := []Experience{
		{ID: "E1"}, // no Vector field
		{ID: "E2"},
	}
	got := TopKExperiences([]float64{1, 0}, bank, 3)
	if got != nil {
		t.Errorf("no vectors: expected nil, got %v", got)
	}
}

func TestTopKExperiences_KGreaterThanBank(t *testing.T) {
	// k larger than the bank should return all available results.
	bank := []Experience{
		{ID: "E1", Vector: []float64{1, 0}},
	}
	got := TopKExperiences([]float64{1, 0}, bank, 10)
	if len(got) != 1 {
		t.Errorf("k>bank: expected 1 result, got %d", len(got))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// deduplicateIndices tests
// ──────────────────────────────────────────────────────────────────────────────

func TestDeduplicateIndices(t *testing.T) {
	input := []int{3, 1, 2, 1, 3, 4}
	got := deduplicateIndices(input)
	want := []int{3, 1, 2, 4}

	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("index %d: got %d, want %d", i, got[i], v)
		}
	}
}

func TestDeduplicateIndices_Empty(t *testing.T) {
	if got := deduplicateIndices(nil); len(got) != 0 {
		t.Errorf("nil input: expected empty, got %v", got)
	}
	if got := deduplicateIndices([]int{}); len(got) != 0 {
		t.Errorf("empty input: expected empty, got %v", got)
	}
}
