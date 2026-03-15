//go:build !llamacpp

package llm

import "github.com/velariumai/gorkbot/pkg/embeddings"

// BuildTag identifies which binary variant is running.
const BuildTag = "standard"

// NewLocalEmbedder returns ErrUnavailable when the llamacpp tag is absent.
// The caller should treat this as a graceful degradation signal and fall back
// to the heuristic classifier in internal/arc.
func NewLocalEmbedder(modelPath string) (embeddings.Embedder, error) {
	return nil, ErrUnavailable
}
