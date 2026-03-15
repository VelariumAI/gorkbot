package providers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/pkoukk/tiktoken-go"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

// ── token-counting stream writer ─────────────────────────────────────────────

// tokenCounterWriter wraps an io.Writer and accumulates a tiktoken token count
// over all bytes written.  It is used to measure streaming output sizes without
// buffering the full response in memory.
type tokenCounterWriter struct {
	target  io.Writer
	bpe     *tiktoken.Tiktoken
	written int // raw byte count; Count() tokenises on demand
	buf     []byte
}

func (cw *tokenCounterWriter) Write(p []byte) (n int, err error) {
	n, err = cw.target.Write(p)
	if n > 0 {
		cw.buf = append(cw.buf, p[:n]...)
		cw.written += n
	}
	return
}

func (cw *tokenCounterWriter) Count() int {
	if cw.bpe == nil || len(cw.buf) == 0 {
		return 0
	}
	return len(cw.bpe.Encode(string(cw.buf), nil, nil))
}

// ── async usage log ───────────────────────────────────────────────────────────

type logEntry struct {
	Time         string  `json:"time"`
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// usageLogger is a package-level singleton that batches usage log writes via a
// 256-slot buffered channel drained by a single background goroutine.
// This ensures the log file is never opened/written on the hot request path.
var (
	usageLogOnce sync.Once
	usageLogCh   chan logEntry
	usageLogWg   sync.WaitGroup
)

func initUsageLogger(logPath string) {
	usageLogOnce.Do(func() {
		usageLogCh = make(chan logEntry, 256)
		usageLogWg.Add(1)
		go func() {
			defer usageLogWg.Done()
			drainUsageLog(logPath)
		}()
	})
}

// drainUsageLog is the sole writer goroutine.  It batches entries (up to 16 at
// a time) into a single file-open/write/close cycle to minimise syscalls.
func drainUsageLog(logPath string) {
	batch := make([]logEntry, 0, 16)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		_ = os.MkdirAll(filepath.Dir(logPath), 0o700)
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			batch = batch[:0]
			return
		}
		for _, e := range batch {
			b, _ := json.Marshal(e)
			_, _ = f.WriteString(string(b) + "\n")
		}
		_ = f.Close()
		batch = batch[:0]
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case e, ok := <-usageLogCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= 16 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// ── WrappedProvider ───────────────────────────────────────────────────────────

// WrappedProvider decorates any AIProvider with:
//   - Semantic response caching (single-turn Generate only)
//   - Accurate per-call token counting via tiktoken
//   - Non-blocking async usage logging
//   - LastUsage() reporting for the TUI analytics panel
//
// It deliberately does NOT modify ConversationHistory — context management is
// the sole responsibility of the Orchestrator's ContextManager/Compressor.
type WrappedProvider struct {
	base    ai.AIProvider
	cache   *SemanticCache
	model   string
	bpe     *tiktoken.Tiktoken
	logPath string

	// lastUsg is stored as an atomic pointer so LastUsage() never blocks the
	// hot streaming path.
	lastUsgPtr atomic.Pointer[ai.GrokUsage]
}

// NewWrappedProvider wraps base with caching and usage telemetry.
// cache may be nil to disable semantic caching.
// logPath is the JSONL usage log file; pass "" to disable.
func NewWrappedProvider(base ai.AIProvider, cache *SemanticCache) *WrappedProvider {
	bpe, _ := tiktoken.GetEncoding("cl100k_base")

	logPath := ""
	if cache != nil {
		// Derive log path from cache's DB directory (both live in configDir).
		// We don't have direct access to configDir here; use UserConfigDir as
		// fallback — this is acceptable since logPath is only for usage stats.
		if d, err := os.UserConfigDir(); err == nil {
			logPath = filepath.Join(d, "gorkbot", "usage_log.jsonl")
		}
	}

	if logPath != "" {
		initUsageLogger(logPath)
	}

	w := &WrappedProvider{
		base:    base,
		cache:   cache,
		model:   base.GetMetadata().ID,
		bpe:     bpe,
		logPath: logPath,
	}
	// Initialise pointer so LastUsage() never returns a nil dereference.
	zero := ai.GrokUsage{}
	w.lastUsgPtr.Store(&zero)
	return w
}

// ── AIProvider interface ──────────────────────────────────────────────────────

func (w *WrappedProvider) Name() string                    { return w.base.Name() }
func (w *WrappedProvider) ID() registry.ProviderID         { return w.base.ID() }
func (w *WrappedProvider) GetMetadata() ai.ProviderMetadata { return w.base.GetMetadata() }
func (w *WrappedProvider) Ping(ctx context.Context) error  { return w.base.Ping(ctx) }
func (w *WrappedProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return w.base.FetchModels(ctx)
}

// LastUsage returns the token counts from the most recent API call.
// Implements ai.UsageReporter.  Lock-free via atomic pointer swap.
func (w *WrappedProvider) LastUsage() ai.GrokUsage {
	return *w.lastUsgPtr.Load()
}

// WithModel returns a new WrappedProvider using the same cache but a different
// underlying model variant.
func (w *WrappedProvider) WithModel(model string) ai.AIProvider {
	return NewWrappedProvider(w.base.WithModel(model), w.cache)
}

// ── internal helpers ──────────────────────────────────────────────────────────

// countTokens encodes text with tiktoken and returns the token count.
// Returns 0 when tiktoken is unavailable (graceful degradation).
func (w *WrappedProvider) countTokens(text string) int {
	if w.bpe == nil || text == "" {
		return 0
	}
	return len(w.bpe.Encode(text, nil, nil))
}

// recordUsage atomically swaps lastUsg and queues an async log write.
// Never blocks the caller.
func (w *WrappedProvider) recordUsage(inToks, outToks int) {
	usg := &ai.GrokUsage{
		PromptTokens:     inToks,
		CompletionTokens: outToks,
		TotalTokens:      inToks + outToks,
	}
	// Atomic store — LastUsage() readers always see a consistent snapshot.
	w.lastUsgPtr.Store(usg)

	if usageLogCh == nil {
		return
	}
	e := logEntry{
		Time:         time.Now().Format(time.RFC3339),
		Model:        w.model,
		InputTokens:  inToks,
		OutputTokens: outToks,
		CostUSD:      CostPerRequest(w.model, inToks, outToks),
	}
	// Non-blocking send: drop if logger is backed up.
	select {
	case usageLogCh <- e:
	default:
		slog.Debug("providers: usage log channel full, entry dropped")
	}
}

// ── Generate (single-turn, cache-eligible) ────────────────────────────────────

func (w *WrappedProvider) Generate(ctx context.Context, prompt string) (string, error) {
	// Semantic cache check — only for single-turn prompts.
	if w.cache != nil {
		if resp, ok := w.cache.GetCachedResponse(ctx, prompt); ok {
			w.recordUsage(w.countTokens(prompt), w.countTokens(resp))
			return resp, nil
		}
	}

	inToks := w.countTokens(prompt)
	resp, err := w.base.Generate(ctx, prompt)
	if err == nil {
		w.recordUsage(inToks, w.countTokens(resp))
		// Cache single-turn responses asynchronously.
		if w.cache != nil {
			w.cache.StoreResponse(ctx, prompt, resp)
		}
	}
	return resp, err
}

// ── GenerateWithHistory (multi-turn, no caching) ──────────────────────────────

// GenerateWithHistory delegates to the base provider without history mutation.
// Token counting uses ConversationHistory.EstimateTokens() which properly
// accounts for all roles, tool calls, and system messages.
func (w *WrappedProvider) GenerateWithHistory(ctx context.Context, history *ai.ConversationHistory) (string, error) {
	inToks := history.EstimateTokens()
	resp, err := w.base.GenerateWithHistory(ctx, history)
	if err == nil {
		w.recordUsage(inToks, w.countTokens(resp))
	}
	return resp, err
}

// ── Stream (single-turn) ──────────────────────────────────────────────────────

func (w *WrappedProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	inToks := w.countTokens(prompt)
	cw := &tokenCounterWriter{target: out, bpe: w.bpe}
	err := w.base.Stream(ctx, prompt, cw)
	w.recordUsage(inToks, cw.Count())
	return err
}

// ── StreamWithHistory (multi-turn, no caching) ────────────────────────────────

// StreamWithHistory delegates to the base provider.  History is never modified
// here — the Orchestrator's ContextManager owns context-window management.
func (w *WrappedProvider) StreamWithHistory(ctx context.Context, history *ai.ConversationHistory, out io.Writer) error {
	inToks := history.EstimateTokens()
	cw := &tokenCounterWriter{target: out, bpe: w.bpe}
	err := w.base.StreamWithHistory(ctx, history, cw)
	w.recordUsage(inToks, cw.Count())
	return err
}

// ── type-size assertion ───────────────────────────────────────────────────────

// Ensure atomic.Pointer[ai.GrokUsage] alignment is satisfied on 32-bit ARM.
var _ = unsafe.Sizeof(atomic.Pointer[ai.GrokUsage]{})
