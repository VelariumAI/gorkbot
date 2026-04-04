//go:build llamacpp

package llm

/*
#cgo CFLAGS: -I${SRCDIR}/cbridge -I${SRCDIR}/../../ext/llama.cpp/include -I${SRCDIR}/../../ext/llama.cpp/ggml/include
#cgo CXXFLAGS: -std=c++17
#cgo LDFLAGS: ${SRCDIR}/libgorkbot_llm.a ${SRCDIR}/../../ext/llama.cpp/build/libllama.a ${SRCDIR}/../../ext/llama.cpp/build/libggml.a ${SRCDIR}/../../ext/llama.cpp/build/libggml-cpu.a ${SRCDIR}/../../ext/llama.cpp/build/libggml-base.a -lm
#cgo linux LDFLAGS: -lstdc++
#cgo android LDFLAGS: -lc++_shared
#cgo darwin LDFLAGS: -lc++
#include "llm_bridge.h"
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/velariumai/gorkbot/pkg/embeddings"
)

// BuildTag identifies which binary variant is running.
const BuildTag = "llamacpp"

// localEmbedder implements embeddings.Embedder via CGO → llama.cpp.
// A single mutex serialises calls since llama.cpp contexts are not thread-safe.
type localEmbedder struct {
	mu   sync.Mutex
	sess *C.GorkLLMSession
	dim  int
	name string
}

// NewLocalEmbedder loads modelPath as an embedding session and returns an
// embeddings.Embedder.  The backend is initialised once; subsequent calls to
// Close() release the model and backend.
func NewLocalEmbedder(modelPath string) (embeddings.Embedder, error) {
	// llama.cpp logging is suppressed via llama_log_set(no-op) in gork_backend_init().
	C.gork_backend_init()

	nThreads := runtime.NumCPU() - 2
	if nThreads < 2 {
		nThreads = 2
	}

	cpath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cpath))

	var errBuf [256]C.char
	sess := C.gork_embed_session_load(
		cpath,
		C.int(nThreads),
		C.int(99), // full GPU offload; llama.cpp falls back to CPU silently
		&errBuf[0],
	)
	if sess == nil {
		C.gork_backend_free()
		return nil, fmt.Errorf("embed load: %s", C.GoString(&errBuf[0]))
	}

	return &localEmbedder{
		sess: sess,
		dim:  EmbedDimension,
		name: "nomic-embed-text-v1.5 (local)",
	}, nil
}

// Embed computes the embedding for text.
// The output buffer is pre-allocated as a Go []float32; C writes directly
// into its backing array — no C.malloc, no manual C.free.
func (e *localEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.sess == nil {
		return nil, ErrUnavailable
	}

	// Pre-allocate output in Go-managed heap — C writes directly here.
	out := make([]float32, e.dim)

	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))

	var errBuf [256]C.char
	n := C.gork_embed(
		e.sess,
		ctext,
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(e.dim),
		&errBuf[0],
	)
	if n < 0 {
		return nil, fmt.Errorf("gork_embed: %s", C.GoString(&errBuf[0]))
	}

	// Truncate to actual dimension returned (usually equals e.dim).
	out = out[:int(n)]

	// L2-normalise so downstream cosine similarity is just a dot product.
	return embeddings.L2Normalize(out), nil
}

// Dims returns the embedding dimension (768 for nomic-embed-text-v1.5).
func (e *localEmbedder) Dims() int { return e.dim }

// Name identifies this embedder in logs and toasts.
func (e *localEmbedder) Name() string { return e.name }

// Close releases the session and llama.cpp backend.
func (e *localEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sess != nil {
		C.gork_session_free(e.sess)
		e.sess = nil
		C.gork_backend_free()
	}
	return nil
}
