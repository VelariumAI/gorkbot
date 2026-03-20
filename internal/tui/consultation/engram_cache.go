package consultation

// engram_cache.go — Stage 3: Short-Circuit Engram Intercept
//
// The EngramCache gates every consultation request against a persistent
// store of previously validated Secondary responses.
//
// Unlike a naive SHA-256 key cache (which would miss semantically identical
// questions phrased differently), this cache uses the VectorStore's Nomic
// embeddings to compute cosine similarity between the incoming VoidTarget and
// stored consultation results. A similarity ≥ 0.95 means the semantic
// footprint is near-identical — the same underlying question asked in
// different words — and the cached answer is returned directly without
// spending a Secondary API call.
//
// Storage encoding:
//   The VoidTarget and its validated response are stored as a single document:
//     "<VoidTarget>\n---ENGRAM_RESPONSE---\n<Response>"
//   The embedding is computed from the combined text. Since the VoidTarget
//   dominates the semantic content (it is the question; the response reinforces
//   the same topic), the embedding accurately reflects the VoidTarget's meaning.
//   On retrieval the separator is used to extract the response.
//
// Session-scope deduplication:
//   An in-memory mutex-protected map prevents the same VoidTarget from being
//   indexed twice in one session (e.g., from rapid successive calls). LTM
//   persistence is handled by the VectorStore's SQLite backend.

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/velariumai/gorkbot/pkg/vectorstore"
)

const (
	// consultationSessionID is the VectorStore session namespace for all
	// engram entries. Isolating them from conversation history prevents
	// consultation documents from polluting the RAG turn-retrieval results.
	consultationSessionID = "consultation:engrams"

	// engramSeparator delimits the VoidTarget from the Response inside a
	// stored engram document. Must be distinctive enough not to occur in
	// typical responses; the triple-dash prefix makes it visually obvious
	// when inspecting the SQLite store directly.
	engramSeparator = "\n---ENGRAM_RESPONSE---\n"
)

// EngramCache implements the Stage 3 short-circuit gate.
type EngramCache struct {
	// vs is written once from the initEmbedder goroutine via SetVectorStore
	// and read concurrently from Lookup/Store goroutines. Use atomic.Pointer
	// to eliminate the data race without a mutex in the retrieval hot path.
	vs   atomic.Pointer[vectorstore.VectorStore]
	mu   sync.Mutex
	seen map[string]bool // session-scope dedup: VoidTarget → already indexed
	log  *slog.Logger
}

func newEngramCache(vs *vectorstore.VectorStore, log *slog.Logger) *EngramCache {
	if log == nil {
		log = slog.Default()
	}
	ec := &EngramCache{
		seen: make(map[string]bool),
		log:  log,
	}
	if vs != nil {
		ec.vs.Store(vs)
	}
	return ec
}

// Lookup checks whether a semantically near-identical VoidTarget was
// previously answered, returning the cached response and true on a hit.
//
// Algorithm:
//  1. Call vs.Search(ctx, ev.VoidTarget, 10) — VectorStore embeds the query
//     internally using the same Nomic model and returns the top-10 candidates
//     with cosine similarity scores.
//  2. Filter for consultationSessionID entries only (ignoring conversation RAG).
//  3. If any candidate has Score ≥ EngramSimilarityThreshold, extract the
//     response from the combined document (split on engramSeparator) and return.
//
// Returns ("", false) when:
//   - VectorStore is not configured.
//   - No candidates exist in the consultation namespace.
//   - No candidate meets the 0.95 threshold.
func (ec *EngramCache) Lookup(ctx context.Context, ev EntropyVoid) (string, bool) {
	vs := ec.vs.Load() // atomic load: safe from any goroutine
	if vs == nil {
		return "", false
	}

	results, err := vs.Search(ctx, ev.VoidTarget, 10)
	if err != nil {
		ec.log.Warn("engram_cache: search failed", "error", err)
		return "", false
	}

	for _, r := range results {
		// Only consider entries stored in the consultation namespace.
		if r.SessionID != consultationSessionID {
			continue
		}
		if r.Score < EngramSimilarityThreshold {
			continue
		}

		// Extract the response from the combined document.
		response := extractResponse(r.Content)
		if response == "" {
			// Document was stored without a response half — skip.
			continue
		}

		ec.log.Debug("engram_cache: hit",
			"score", r.Score,
			"void_prefix", ev.VoidTarget[:clamp(len(ev.VoidTarget), 50)],
		)
		return response, true
	}
	return "", false
}

// Store indexes a validated consultation result for future cache lookups.
// The operation is fire-and-forget (caller should run this in a goroutine);
// IndexTurn is already async inside VectorStore.
//
// Encoding: the document content is "<VoidTarget><separator><Response>".
// The VectorStore embeds this combined text. Because the VoidTarget appears
// first and typically accounts for the majority of the character budget, it
// dominates the embedding's semantic direction. The response reinforces the
// same topic, so the combined embedding remains faithful to the question's
// meaning. At the 0.95 threshold this produces negligible false-positive risk.
func (ec *EngramCache) Store(ctx context.Context, ev EntropyVoid, response string) {
	vs := ec.vs.Load() // atomic load
	if vs == nil || response == "" {
		return
	}

	ec.mu.Lock()
	if ec.seen[ev.VoidTarget] {
		ec.mu.Unlock()
		return // already indexed this session
	}
	ec.seen[ev.VoidTarget] = true
	ec.mu.Unlock()

	content := ev.VoidTarget + engramSeparator + response
	// IndexTurn is non-blocking (internally fire-and-forget).
	vs.IndexTurn(ctx, consultationSessionID, "consultation", content)

	ec.log.Debug("engram_cache: stored",
		"void_len", len(ev.VoidTarget),
		"resp_len", len(response),
	)
}

// extractResponse splits a stored engram document and returns the response half.
// Returns "" if the separator is absent (legacy or malformed entry).
func extractResponse(content string) string {
	idx := strings.Index(content, engramSeparator)
	if idx < 0 {
		return ""
	}
	response := content[idx+len(engramSeparator):]
	return strings.TrimSpace(response)
}
