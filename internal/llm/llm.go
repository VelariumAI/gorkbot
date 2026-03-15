// Package llm provides a local embedding engine backed by llama.cpp.
// Build with -tags llamacpp to enable the native engine; the default stub
// returns ErrUnavailable so the rest of the codebase degrades gracefully
// to the keyword heuristic in internal/arc.
package llm

import "errors"

// ErrUnavailable is returned when the engine was not compiled in (stub build).
var ErrUnavailable = errors.New("local embedding engine not available (build with -tags llamacpp)")

// ErrNoModel is returned when no embedding GGUF model file can be found.
var ErrNoModel = errors.New("no embedding model found — run: make download-nomic")
