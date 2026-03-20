package adaptive

import (
	"context"
	"math"

	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/embeddings"
)

// VectorProjector wraps an embeddings.Embedder and reduces the output vector
// to a RAM-appropriate number of dimensions via prefix truncation.
//
// Dimensionality tiers (chosen at construction from the HAL RAM profile):
//   - ≥ 4 GB free RAM : full dimensions  (768 / 1536 / 3072, model-dependent)
//   - 1–4 GB free RAM : 256 dimensions
//   - < 1 GB free RAM : 128 dimensions
//
// Prefix truncation is the simplest dimensionality reduction that preserves
// the angular structure of L2-normalised embeddings produced by standard
// embedding models (text-embedding-3-small, text-embedding-004, etc.).
// After truncation the vector is re-normalised to unit length so that
// CosineSimilarity still equals the dot product.
//
// The projector implements embeddings.Embedder transparently — the VectorStore
// and ARCRouter classifiers see no difference.
type VectorProjector struct {
	inner      embeddings.Embedder
	targetDims int
}

// NewVectorProjector creates a projector calibrated to the current HAL profile.
// Pass hal from the existing platform.ProbeHAL() call — no new probing.
func NewVectorProjector(inner embeddings.Embedder, hal platform.HALProfile) *VectorProjector {
	dims := targetDims(hal, inner.Dims())
	return &VectorProjector{inner: inner, targetDims: dims}
}

// Embed returns a unit-normalised embedding truncated to targetDims.
func (p *VectorProjector) Embed(ctx context.Context, text string) ([]float32, error) {
	full, err := p.inner.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	return project(full, p.targetDims), nil
}

// Dims returns the projected (output) dimensionality, not the inner model's.
func (p *VectorProjector) Dims() int { return p.targetDims }

// Name returns a descriptive name for logging.
func (p *VectorProjector) Name() string {
	return p.inner.Name() + "@" + dimStr(p.targetDims)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// targetDims picks the right dimensionality given free RAM and the embedder's
// native dimension count.
func targetDims(hal platform.HALProfile, nativeDims int) int {
	freeGB := float64(hal.FreeRAMMB) / 1024.0

	var target int
	switch {
	case freeGB >= 4.0:
		target = nativeDims // full — no projection
	case freeGB >= 1.0:
		target = 256
	default:
		target = 128
	}

	// Never project UP — if the model's native dims are smaller, use those.
	if nativeDims < target {
		return nativeDims
	}
	// Snap to the nearest multiple of 64 ≥ target for alignment.
	return target
}

// project truncates v to targetDims and re-normalises to unit length.
func project(v []float32, targetDims int) []float32 {
	if len(v) <= targetDims {
		return v // no projection needed
	}
	sub := make([]float32, targetDims)
	copy(sub, v[:targetDims])
	return embeddings.L2Normalize(sub)
}

// l2norm computes the L2 norm of v.
func l2norm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}

// dimStr returns a compact string like "256d" for logging.
func dimStr(d int) string {
	switch d {
	case 128:
		return "128d"
	case 256:
		return "256d"
	case 512:
		return "512d"
	case 768:
		return "768d"
	case 1536:
		return "1536d"
	case 3072:
		return "3072d"
	default:
		return "fulld"
	}
}

// ensure l2norm is used (suppress "declared and not used" if inlined away)
var _ = l2norm
