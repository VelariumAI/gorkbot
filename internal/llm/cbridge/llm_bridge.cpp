/*
 * llm_bridge.cpp — C++ bridge between llama.cpp and Go/CGO.
 *
 * Two session modes: generation (step-based pull) and embedding (single-call).
 * Compiled by scripts/build_llm_bridge.sh into libgorkbot_llm.a.
 */

#include "llm_bridge.h"
#include <llama.h>

#include <cstdio>
#include <cstring>
#include <string>
#include <vector>

/* ── Internal session struct ───────────────────────────────────────────── */

struct GorkLLMSession {
    llama_model   *model           = nullptr;
    llama_context *ctx             = nullptr;
    llama_sampler *smpl            = nullptr;
    int            n_threads       = 4;
    int            n_threads_batch = 4;
    bool           embed_mode      = false; /* true → embedding context */

    /* per-inference state (generation only) */
    llama_token    eos_tok  = -1;
    llama_token    eot_tok  = -1;
    bool           inferring = false;
};

/* ── Log suppression ───────────────────────────────────────────────────── */

/* No-op log callback: swallows all llama.cpp/GGML output at the C level.
 * This is the only safe way to silence the library when the TUI is running —
 * redirecting OS file descriptors is process-wide and races with Go output. */
static void suppress_log(enum ggml_log_level, const char *, void *) {}

/* ── Backend lifecycle ─────────────────────────────────────────────────── */

void gork_backend_init(void) {
    llama_backend_init();
    llama_log_set(suppress_log, nullptr);
}
void gork_backend_free(void)  { llama_backend_free(); }

/* ── Session load / free ───────────────────────────────────────────────── */

GorkLLMSession *gork_session_load(
    const char *model_path,
    int         n_ctx,
    int         n_threads,
    int         n_threads_batch,
    int         n_gpu_layers,
    char       *error_buf)
{
    llama_model_params mparams = llama_model_default_params();
    mparams.n_gpu_layers = (n_gpu_layers >= 0) ? n_gpu_layers : 0;

    llama_model *model = llama_model_load_from_file(model_path, mparams);
    if (!model) {
        if (error_buf)
            snprintf(error_buf, 256, "failed to load model: %s", model_path);
        return nullptr;
    }

    llama_context_params cparams = llama_context_default_params();
    cparams.n_ctx           = (n_ctx > 0)           ? (uint32_t)n_ctx : 2048u;
    cparams.n_threads       = (n_threads > 0)       ? n_threads       : 4;
    cparams.n_threads_batch = (n_threads_batch > 0) ? n_threads_batch : 4;
    cparams.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_AUTO;

    llama_context *ctx = llama_init_from_model(model, cparams);
    if (!ctx) {
        llama_model_free(model);
        if (error_buf)
            snprintf(error_buf, 256, "failed to create llama context");
        return nullptr;
    }

    GorkLLMSession *sess  = new GorkLLMSession();
    sess->model           = model;
    sess->ctx             = ctx;
    sess->n_threads       = cparams.n_threads;
    sess->n_threads_batch = cparams.n_threads_batch;
    sess->embed_mode      = false;
    return sess;
}

GorkLLMSession *gork_embed_session_load(
    const char *model_path,
    int         n_threads,
    int         n_gpu_layers,
    char       *error_buf)
{
    llama_model_params mparams = llama_model_default_params();
    mparams.n_gpu_layers = (n_gpu_layers >= 0) ? n_gpu_layers : 0;

    llama_model *model = llama_model_load_from_file(model_path, mparams);
    if (!model) {
        if (error_buf)
            snprintf(error_buf, 256, "failed to load embed model: %s", model_path);
        return nullptr;
    }

    llama_context_params cparams = llama_context_default_params();
    /* Minimal context — embedding doesn't need a large KV cache. */
    cparams.n_ctx           = 512u;
    cparams.n_threads       = (n_threads > 0) ? n_threads : 4;
    cparams.n_threads_batch = cparams.n_threads;
    /* Enable embedding mode — let llama.cpp auto-detect pooling from GGUF. */
    cparams.embeddings      = true;
    cparams.pooling_type    = LLAMA_POOLING_TYPE_UNSPECIFIED;

    llama_context *ctx = llama_init_from_model(model, cparams);
    if (!ctx) {
        llama_model_free(model);
        if (error_buf)
            snprintf(error_buf, 256, "failed to create embed context");
        return nullptr;
    }

    GorkLLMSession *sess  = new GorkLLMSession();
    sess->model           = model;
    sess->ctx             = ctx;
    sess->n_threads       = cparams.n_threads;
    sess->n_threads_batch = cparams.n_threads_batch;
    sess->embed_mode      = true;
    return sess;
}

void gork_session_free(GorkLLMSession *sess) {
    if (!sess) return;
    if (!sess->embed_mode) gork_infer_end(sess);
    if (sess->ctx)   llama_free(sess->ctx);
    if (sess->model) llama_model_free(sess->model);
    delete sess;
}

/* ── Helpers ───────────────────────────────────────────────────────────── */

static std::vector<llama_token> do_tokenize(
    const llama_model *model,
    const std::string &text,
    bool add_special, bool parse_special)
{
    const llama_vocab *vocab = llama_model_get_vocab(model);
    int n = -llama_tokenize(vocab, text.c_str(), (int32_t)text.size(),
                            nullptr, 0, add_special, parse_special);
    if (n <= 0) return {};
    std::vector<llama_token> toks(n);
    llama_tokenize(vocab, text.c_str(), (int32_t)text.size(),
                   toks.data(), n, add_special, parse_special);
    return toks;
}

/* ── Embedding ─────────────────────────────────────────────────────────── */

int gork_embed(
    GorkLLMSession *sess,
    const char     *text,
    float          *out_vec,   /* Go-managed buffer — C writes directly here */
    int             out_len,
    char           *error_buf)
{
    if (!sess || !sess->embed_mode || !sess->ctx || !sess->model) {
        if (error_buf) snprintf(error_buf, 256, "invalid embed session");
        return -1;
    }
    if (!out_vec || out_len <= 0) {
        if (error_buf) snprintf(error_buf, 256, "invalid output buffer");
        return -1;
    }

    /* Clear KV / memory before each embed to avoid stale state. */
    llama_memory_t mem = llama_get_memory(sess->ctx);
    if (mem) llama_memory_clear(mem, false);

    /* Tokenise with special tokens (BOS/EOS) for embedding models. */
    std::vector<llama_token> toks = do_tokenize(sess->model, text ? text : "", true, false);
    if (toks.empty()) {
        if (error_buf) snprintf(error_buf, 256, "tokenisation returned 0 tokens");
        return -1;
    }

    /* Clamp to context window: oversized inputs cause llama_batch_init OOM
     * (batch.seq_id[i] inner arrays go null) and llama_decode overflow. */
    const uint32_t n_ctx = llama_n_ctx(sess->ctx);
    if ((uint32_t)toks.size() > n_ctx) {
        toks.resize(n_ctx);
    }

    /*
     * Build a batch with seq_id=0 for all tokens.
     * llama_batch_get_one sets logits=true only for the last token;
     * for pooled embeddings we need all positions, so build manually.
     */
    llama_batch batch = llama_batch_init((int32_t)toks.size(), 0, 1);
    if (!batch.token || !batch.pos || !batch.n_seq_id || !batch.seq_id || !batch.logits) {
        llama_batch_free(batch);
        if (error_buf) snprintf(error_buf, 256, "llama_batch_init allocation failed");
        return -1;
    }
    batch.n_tokens = (int32_t)toks.size();
    for (int32_t i = 0; i < batch.n_tokens; i++) {
        batch.token   [i] = toks[i];
        batch.pos     [i] = i;
        batch.n_seq_id[i] = 1;
        if (!batch.seq_id[i]) {
            llama_batch_free(batch);
            if (error_buf) snprintf(error_buf, 256, "llama_batch_init seq_id[%d] null", i);
            return -1;
        }
        batch.seq_id  [i][0] = 0;
        batch.logits  [i] = false; /* pooling layer handles aggregation */
    }

    if (llama_decode(sess->ctx, batch) != 0) {
        llama_batch_free(batch);
        if (error_buf) snprintf(error_buf, 256, "llama_decode (embed) failed");
        return -1;
    }
    llama_batch_free(batch);

    /*
     * Retrieve pooled sequence embedding.
     * llama_get_embeddings_seq is preferred for mean/cls-pooled models;
     * fall back to last-token embedding if seq pooling returns NULL.
     */
    const float *emb = llama_get_embeddings_seq(sess->ctx, 0);
    if (!emb) {
        emb = llama_get_embeddings_ith(sess->ctx, batch.n_tokens - 1);
    }
    if (!emb) {
        if (error_buf) snprintf(error_buf, 256, "llama_get_embeddings returned NULL");
        return -1;
    }

    /* Model embedding dimension. */
    int n_embd = llama_model_n_embd(sess->model);
    int copy_n = (n_embd < out_len) ? n_embd : out_len;

    /* Write directly into the Go-managed float32 slice backing array. */
    memcpy(out_vec, emb, (size_t)copy_n * sizeof(float));

    return copy_n;
}

/* ── Inference: start ──────────────────────────────────────────────────── */

int gork_infer_start(
    GorkLLMSession *sess,
    const char     *system_prompt,
    const char     *user_prompt,
    float           temperature,
    char           *error_buf)
{
    if (!sess || !sess->ctx || !sess->model) {
        if (error_buf) snprintf(error_buf, 256, "invalid session");
        return -1;
    }

    gork_infer_end(sess);

    llama_memory_t mem = llama_get_memory(sess->ctx);
    if (mem) llama_memory_clear(mem, false);

    std::string prompt;
    if (system_prompt && system_prompt[0]) {
        prompt  = "<|im_start|>system\n";
        prompt += system_prompt;
        prompt += "<|im_end|>\n";
    }
    prompt += "<|im_start|>user\n";
    prompt += (user_prompt ? user_prompt : "");
    prompt += "<|im_end|>\n<|im_start|>assistant\n";

    std::vector<llama_token> toks = do_tokenize(sess->model, prompt, true, true);
    if (toks.empty()) {
        if (error_buf) snprintf(error_buf, 256, "tokenisation returned 0 tokens");
        return -1;
    }
    llama_batch batch = llama_batch_get_one(toks.data(), (int32_t)toks.size());
    if (llama_decode(sess->ctx, batch) != 0) {
        if (error_buf) snprintf(error_buf, 256, "llama_decode (prompt) failed");
        return -1;
    }

    llama_sampler_chain_params sparams = llama_sampler_chain_default_params();
    sess->smpl = llama_sampler_chain_init(sparams);
    if (temperature <= 0.0f) {
        llama_sampler_chain_add(sess->smpl, llama_sampler_init_greedy());
    } else {
        llama_sampler_chain_add(sess->smpl, llama_sampler_init_top_p(0.9f, 1));
        llama_sampler_chain_add(sess->smpl, llama_sampler_init_temp(temperature));
        llama_sampler_chain_add(sess->smpl, llama_sampler_init_dist(LLAMA_DEFAULT_SEED));
    }

    const llama_vocab *vocab = llama_model_get_vocab(sess->model);
    sess->eos_tok   = llama_vocab_eos(vocab);
    sess->eot_tok   = llama_vocab_eot(vocab);
    sess->inferring = true;
    return 0;
}

/* ── Inference: step ───────────────────────────────────────────────────── */

int gork_infer_step(
    GorkLLMSession *sess,
    char           *token_buf,
    int             buf_size,
    char           *error_buf)
{
    if (!sess || !sess->inferring || !sess->smpl) {
        if (error_buf) snprintf(error_buf, 256, "no active inference");
        return -1;
    }

    llama_token tok = llama_sampler_sample(sess->smpl, sess->ctx, -1);
    llama_sampler_accept(sess->smpl, tok);

    if (tok == sess->eos_tok || tok == sess->eot_tok) return 0;

    const llama_vocab *vocab = llama_model_get_vocab(sess->model);
    int n = llama_token_to_piece(vocab, tok, token_buf, buf_size, 0, true);
    if (n <= 0) n = 0;

    llama_batch next = llama_batch_get_one(&tok, 1);
    if (llama_decode(sess->ctx, next) != 0) {
        if (error_buf) snprintf(error_buf, 256, "llama_decode (gen) failed");
        return -1;
    }

    return (n > 0) ? n : 1;
}

/* ── Inference: end ────────────────────────────────────────────────────── */

void gork_infer_end(GorkLLMSession *sess) {
    if (!sess) return;
    if (sess->smpl) {
        llama_sampler_free(sess->smpl);
        sess->smpl = nullptr;
    }
    sess->inferring = false;
}
