#pragma once
/*
 * llm_bridge.h — plain-C API wrapping llama.cpp for Go/CGO.
 *
 * Two session modes:
 *   Generation — step-based pull API (gork_infer_start/step/end).
 *   Embedding  — single-call API (gork_embed) for dense vector output.
 *
 * Both modes share the same GorkLLMSession handle but must be loaded via
 * the appropriate factory (gork_session_load vs gork_embed_session_load).
 */

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Opaque session handle. */
typedef struct GorkLLMSession GorkLLMSession;

/* One-time backend initialisation (call before any load). */
void gork_backend_init(void);

/* Tear-down (call on process exit). */
void gork_backend_free(void);

/*
 * Load a GGUF model for text generation.
 *   model_path         — absolute path to .gguf file.
 *   n_ctx              — context window size (tokens).
 *   n_threads          — generation threads.
 *   n_threads_batch    — prompt-processing threads.
 *   n_gpu_layers       — layers to offload to GPU (99 = all; 0 = CPU-only).
 *   error_buf          — caller-allocated 256-byte error buffer.
 * Returns NULL on failure (error_buf populated).
 */
GorkLLMSession *gork_session_load(
    const char *model_path,
    int         n_ctx,
    int         n_threads,
    int         n_threads_batch,
    int         n_gpu_layers,
    char       *error_buf);

/*
 * Load a GGUF model for embedding (sentence-vector) mode.
 * Context is created with embeddings=true and auto-detected pooling.
 *   model_path      — absolute path to embedding .gguf file.
 *   n_threads       — CPU threads for batch processing.
 *   n_gpu_layers    — GPU layers (99 = all; 0 = CPU-only).
 *   error_buf       — caller-allocated 256-byte error buffer.
 * Returns NULL on failure.
 */
GorkLLMSession *gork_embed_session_load(
    const char *model_path,
    int         n_threads,
    int         n_gpu_layers,
    char       *error_buf);

/* Free a session and all associated resources. */
void gork_session_free(GorkLLMSession *sess);

/*
 * Compute an embedding vector for text.
 * out_vec must point to a Go-managed float32 buffer of at least out_len elements.
 * C writes directly into the Go slice backing array — no malloc/free needed.
 * Returns the number of floats written, or -1 on error (error_buf populated).
 */
int gork_embed(
    GorkLLMSession *sess,
    const char     *text,
    float          *out_vec,
    int             out_len,
    char           *error_buf);

/*
 * Begin an inference pass (tokenise + decode the prompt).
 *   system_prompt — may be NULL.
 *   user_prompt   — required.
 *   temperature   — 0.0 → greedy; >0 → sampling.
 *   error_buf     — 256-byte error buffer.
 * Returns 0 on success, -1 on error.
 */
int gork_infer_start(
    GorkLLMSession *sess,
    const char     *system_prompt,
    const char     *user_prompt,
    float           temperature,
    char           *error_buf);

/*
 * Generate one token and write its UTF-8 text into token_buf.
 * Returns:
 *   > 0  — bytes written (token ready, continue loop)
 *     0  — EOS/EOT reached (done)
 *   < 0  — error (error_buf populated)
 */
int gork_infer_step(
    GorkLLMSession *sess,
    char           *token_buf,
    int             buf_size,
    char           *error_buf);

/* Release inference state (call after loop finishes, even on error). */
void gork_infer_end(GorkLLMSession *sess);

#ifdef __cplusplus
}
#endif
