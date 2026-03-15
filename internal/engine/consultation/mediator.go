// Package consultation implements the Dual-Model Consultation Architecture —
// a five-stage "airlock" that enforces strict signal fidelity when the Primary
// AI model delegates a logic gap to a Secondary consultant model.
//
// Pipeline stages:
//
//	Stage 1 — Strict Bandwidth Protocol:  EntropyVoid schema validation + retry ceiling.
//	Stage 2 — Hybrid Context Pruning:     Nomic semantic search + concurrent lexical grep.
//	Stage 3 — Engram Intercept (cache):   VectorStore cosine ≥ 95% short-circuits the API.
//	Stage 4 — Synthetic Thinking Time:    Secondary called at temperature=0, timeout-guarded.
//	Stage 5 — Airlock Validation:         Type sanitisation + reverse semantic deviation check.
//
// All blocking I/O (embed, search, API call) runs inside goroutines.
// Use RunConsultationCmd to obtain the tea.Cmd that wraps MediateRequest —
// the Bubble Tea main thread must never block on this package.
package consultation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/embeddings"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/vectorstore"
)

// ── Error sentinels ────────────────────────────────────────────────────────

var (
	// ErrMalformedPayload is returned when the Primary model's JSON deviates
	// from the EntropyVoid schema. The message is verbose so the Primary can
	// self-correct without human intervention.
	ErrMalformedPayload = errors.New("consultation: malformed EntropyVoid payload")

	// ErrRetryLimitExceeded fires after MaxRetries consecutive validation
	// failures, preventing the Primary from looping forever on broken JSON.
	ErrRetryLimitExceeded = errors.New("consultation: retry ceiling exceeded")

	// ErrSignalDegradation fires in Stage 5 when the Secondary's response
	// drifts too far semantically from the original VoidTarget — indicating
	// the model produced conversational fluff instead of a focused answer.
	ErrSignalDegradation = errors.New("consultation: signal degradation — response deviates from VoidTarget")

	// ErrSecondaryUnavailable is returned when no Secondary provider is wired.
	ErrSecondaryUnavailable = errors.New("consultation: no secondary provider configured")
)

// ── Tuning constants ──────────────────────────────────────────────────────

// MaxRetries is the per-call ceiling on consecutive validation failures.
// Two retries balances LLM self-correction latency vs. infinite-loop risk:
// empirically, a well-configured Primary fixes malformed JSON in ≤ 2 attempts.
const MaxRetries = 2

// EngramSimilarityThreshold is the minimum cosine similarity (0–1) between a
// new VoidTarget and a stored consultation result to trigger a cache hit.
// 0.95 means near-identical semantic footprint — a rephrasing of the same
// underlying question rather than a genuinely new problem.
const EngramSimilarityThreshold = 0.95

// ConsultationDegradationThreshold is the maximum acceptable cosine DISTANCE
// (1 − similarity) between the Secondary's response and the VoidTarget.
// Responses above this threshold are considered semantically drifted and are
// discarded in Stage 5.
const ConsultationDegradationThreshold = 0.42

// consultationTimeoutDesktop is the Secondary API deadline on desktop/server.
const consultationTimeoutDesktop = 45 * time.Second

// consultationTimeoutMobile is the tighter deadline for Termux/SBC environments
// where cellular/battery governance can stall network calls mid-stream.
const consultationTimeoutMobile = 30 * time.Second

// ── Closed type enum ──────────────────────────────────────────────────────

// allowedExpectedTypes is the closed set of valid EntropyVoid.ExpectedType
// values. Keeping it closed prevents prompt injection from sneaking in
// arbitrary values that confuse the Stage 5 airlock sanitiser.
var allowedExpectedTypes = map[string]bool{
	"code":     true,
	"json":     true,
	"analysis": true,
	"plan":     true,
	"boolean":  true,
	"value":    true,
}

// ── ConsultationStage — TUI progress labelling ────────────────────────────

// ConsultationStage represents a named step in the consultation pipeline.
// These values are sent as ConsultationStageMsg tea.Msgs so the TUI can render
// a live "Reasoning…" status indicator without polling.
type ConsultationStage int

const (
	StageValidating         ConsultationStage = iota // Stage 1: schema + retry gate
	StageCacheCheck                                  // Stage 3: engram VectorStore lookup
	StageContextBuilding                             // Stage 2: hybrid semantic + lexical
	StageConsulting                                  // Stage 4: Secondary API call
	StageValidatingResponse                          // Stage 5: sanitise + semantic check
)

// stageLabel returns a short human-readable label for UI display.
func (s ConsultationStage) stageLabel() string {
	switch s {
	case StageValidating:
		return "Validating request"
	case StageCacheCheck:
		return "Checking engram cache"
	case StageContextBuilding:
		return "Building context (semantic + lexical)"
	case StageConsulting:
		return "Consulting Secondary model"
	case StageValidatingResponse:
		return "Airlock validation"
	default:
		return "Processing"
	}
}

// ── Tea.Msg types exported for TUI ────────────────────────────────────────

// ConsultationStageMsg signals a pipeline stage transition. The TUI renders
// this as a dynamic status chip while the generation spinner is active.
type ConsultationStageMsg struct {
	Stage  ConsultationStage
	Detail string // optional per-stage annotation (e.g. "cache miss")
}

// ConsultationDoneMsg carries the validated Secondary response. The orchestrator
// injects Content into the Primary's history as a Universal Truth system message.
type ConsultationDoneMsg struct {
	Content   string // sanitised, type-validated answer
	FromCache bool   // true = Stage 3 hit; no API call was made
	Retries   int    // validation retries consumed (0–MaxRetries)
}

// ConsultationErrorMsg signals that the pipeline failed without a valid result.
type ConsultationErrorMsg struct {
	Err     error
	Stage   ConsultationStage // where the failure occurred
	Payload string            // original raw payload for Primary-side diagnostics
}

// ── EntropyVoid ───────────────────────────────────────────────────────────

// EntropyVoid is the strict JSON contract the Primary model must honour when
// requesting a consultation. Every field is mandatory; any violation causes an
// immediate, structured rejection that forces the Primary to self-correct.
//
// Rationale for each field:
//   - ContextHash: SHA-256 of the recent conversation slice. Binds the request
//     to current world-state and serves as the primary key for the Stage 3
//     cache. Generated by ComputeContextHash().
//   - VoidTarget: the minimal, precise logic gap to resolve. Forces the Primary
//     to distil its question before asking, preventing the "telephone game"
//     where vague prompts produce vague responses.
//   - ExpectedType: pre-arms the Stage 5 airlock with the correct sanitiser
//     so the response is validated before it touches the Primary's context.
type EntropyVoid struct {
	// ContextHash is a SHA-256 hex digest of the active conversation slice.
	// Exactly 64 hex characters. Produce with ComputeContextHash().
	ContextHash string `json:"context_hash"`

	// VoidTarget is the precise question or logic gap to resolve.
	// Non-empty, ≤ 4096 characters.
	VoidTarget string `json:"void_target"`

	// ExpectedType constrains the Stage 5 sanitiser.
	// Must be one of: "code" | "json" | "analysis" | "plan" | "boolean" | "value"
	ExpectedType string `json:"expected_type"`
}

// validate enforces all structural and semantic constraints.
// Returns a verbose, human-readable error safe to send to the Primary model
// as a tool result so it can correct its JSON and retry.
func (ev EntropyVoid) validate() error {
	if ev.ContextHash == "" {
		return fmt.Errorf(
			"%w: context_hash is required — call ComputeContextHash on the current history",
			ErrMalformedPayload,
		)
	}
	if len(ev.ContextHash) != 64 {
		return fmt.Errorf(
			"%w: context_hash must be exactly 64 hex chars (SHA-256), got len=%d",
			ErrMalformedPayload, len(ev.ContextHash),
		)
	}
	if ev.VoidTarget == "" {
		return fmt.Errorf(
			"%w: void_target is required — state the precise logic gap",
			ErrMalformedPayload,
		)
	}
	if len(ev.VoidTarget) > 4096 {
		return fmt.Errorf(
			"%w: void_target exceeds 4096 chars (%d) — compress the query first",
			ErrMalformedPayload, len(ev.VoidTarget),
		)
	}
	if !allowedExpectedTypes[ev.ExpectedType] {
		return fmt.Errorf(
			"%w: expected_type %q is invalid; allowed values: code, json, analysis, plan, boolean, value",
			ErrMalformedPayload, ev.ExpectedType,
		)
	}
	return nil
}

// ParseAndValidate unmarshals raw JSON and validates the EntropyVoid schema.
// The returned error is intentionally verbose so the Primary can self-correct.
func ParseAndValidate(raw []byte) (EntropyVoid, error) {
	var ev EntropyVoid
	if err := json.Unmarshal(raw, &ev); err != nil {
		return ev, fmt.Errorf(
			`%w: JSON parse failed (%s) — expected: {"context_hash":"<64 hex>","void_target":"<query>","expected_type":"code|json|analysis|plan|boolean|value"}`,
			ErrMalformedPayload, err.Error(),
		)
	}
	return ev, ev.validate()
}

// ComputeContextHash produces a SHA-256 hex digest of the most recent 20
// conversation messages. The Primary model calls the ConsultTool's helper to
// obtain this value, binding each request to a snapshot of session state.
// A stale hash causes a Stage 3 cache miss (falling through to a live API
// call), which is a safe degradation rather than an error.
func ComputeContextHash(history *ai.ConversationHistory) string {
	msgs := history.GetRecentMessages(20)
	h := sha256.New()
	for _, m := range msgs {
		h.Write([]byte(m.Role))
		h.Write([]byte(m.Content))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ── MediatorConfig ────────────────────────────────────────────────────────

// MediatorConfig carries all injected dependencies. Optional fields (nil)
// degrade gracefully: the pipeline skips the unavailable stage rather than
// aborting. This keeps Gorkbot operational in offline / resource-constrained
// configurations (Termux on a mid-range Android device, no local embedder).
type MediatorConfig struct {
	// Secondary is the consultant AI provider (Gemini, Anthropic, etc.).
	// Required for Stage 4; nil returns ErrSecondaryUnavailable.
	Secondary ai.AIProvider

	// VectorStore powers Stage 2 semantic search and Stage 3 engram cache.
	// nil silently disables both semantic retrieval and cache lookups.
	VectorStore *vectorstore.VectorStore

	// Embedder is the local Nomic model used by Stage 3 + Stage 5.
	// nil disables the reverse semantic deviation check in Stage 5.
	Embedder embeddings.Embedder

	// AgeMem is the SENSE short-term memory, enriching Stage 2 context
	// with recent tool-preference engrams not yet indexed by VectorStore.
	AgeMem *sense.AgeMem

	// WorkDir is the project root for the Stage 2 lexical grep.
	// "" disables worktree scanning (lexical channel falls silent).
	WorkDir string

	// HAL is the hardware profile, used to select the appropriate API
	// timeout (30 s mobile vs. 45 s desktop) and cap grep parallelism.
	HAL platform.HALProfile

	// Logger is the structured logger. nil falls back to slog.Default().
	Logger *slog.Logger

	// SendMsg, when non-nil, is called with stage-progress tea.Msgs from
	// inside MediateRequest. The TUI sets this so it can render live status
	// without the consultation package importing internal/tui.
	// Safe to call from any goroutine.
	SendMsg func(tea.Msg)
}

// ── ConsultationResult ────────────────────────────────────────────────────

// ConsultationResult is the sanitised truth that exits the Stage 5 airlock.
// It is safe to inject into the Primary model's context as a Universal Truth
// system observation.
type ConsultationResult struct {
	Content   string // Sanitised, type-validated response
	FromCache bool   // Stage 3 hit — no Secondary API call was made
	Retries   int    // Validation retries consumed (0–MaxRetries)
}

// ── ConsultationMediator ──────────────────────────────────────────────────

// ConsultationMediator is the five-stage airlock. It is safe for concurrent
// use; each MediateRequest invocation is isolated with its own context.
//
// Concurrency design:
//   - mu is a RWMutex so provider hot-swaps during failover never race with
//     ongoing consultation calls. The lock covers only a pointer copy
//     (nanoseconds) and cannot cause priority inversion on a mobile CPU.
//   - All blocking I/O runs in the goroutine of the caller. The mediator
//     itself contains no long-lived goroutines.
//   - Use RunConsultationCmd to wrap MediateRequest in a tea.Cmd so the
//     Bubble Tea runtime manages the goroutine.
type ConsultationMediator struct {
	cfg     MediatorConfig
	mu      sync.RWMutex // guards cfg.Secondary hot-swap only
	hybrid  *HybridSearch
	cache   *EngramCache
	airlock *AirlockValidator
	log     *slog.Logger
}

// NewMediator constructs a fully-wired ConsultationMediator.
func NewMediator(cfg MediatorConfig) *ConsultationMediator {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &ConsultationMediator{
		cfg:     cfg,
		log:     log,
		hybrid:  newHybridSearch(cfg.VectorStore, cfg.AgeMem, cfg.WorkDir, cfg.Embedder, log, cfg.HAL),
		cache:   newEngramCache(cfg.VectorStore, log),
		airlock: newAirlockValidator(cfg.Embedder, log),
	}
}

// SetSecondary hot-swaps the Secondary provider atomically. Called by the
// provider failover cascade when the configured Secondary goes offline.
func (m *ConsultationMediator) SetSecondary(p ai.AIProvider) {
	m.mu.Lock()
	m.cfg.Secondary = p
	m.mu.Unlock()
}

// SetEmbedder wires (or replaces) the Embedder used by the Stage 5 airlock
// reverse semantic check. Called by initEmbedder after the local Nomic model
// loads — before this, the reverse check is silently skipped.
func (m *ConsultationMediator) SetEmbedder(emb embeddings.Embedder) {
	m.mu.Lock()
	m.cfg.Embedder = emb
	m.mu.Unlock()
	// airlock.setEmb uses its own RWMutex — safe to call outside m.mu.
	m.airlock.setEmb(emb)
}

// SetVectorStore wires (or replaces) the VectorStore across all sub-components
// that need it: the HybridSearch semantic channel and the EngramCache.
// Called by initEmbedder after the local Nomic model loads asynchronously —
// the mediator starts in degraded mode (no semantic search, no engram cache)
// and upgrades itself live once the embedder is ready.
func (m *ConsultationMediator) SetVectorStore(vs *vectorstore.VectorStore) {
	m.mu.Lock()
	m.cfg.VectorStore = vs
	m.mu.Unlock()
	// Propagate via atomic stores — safe to call outside m.mu; each sub-
	// component uses its own atomic.Pointer to eliminate the data race.
	m.hybrid.vs.Store(vs)
	m.cache.vs.Store(vs)
}

// secondary returns the current Secondary under a minimal read lock.
func (m *ConsultationMediator) secondary() ai.AIProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Secondary
}

// send dispatches a tea.Msg to the Bubble Tea runtime.
// Safe to call when SendMsg is nil (headless / CLI mode).
func (m *ConsultationMediator) send(msg tea.Msg) {
	if m.cfg.SendMsg != nil {
		m.cfg.SendMsg(msg)
	}
}

// ── Main pipeline ─────────────────────────────────────────────────────────

// consultationPipelineTimeout is the hard deadline for the ENTIRE five-stage
// pipeline (Stages 1–5 inclusive). It is separate from consultationTimeout*
// which only applies to the Stage 4 Secondary API call.
//
// Without a pipeline-level deadline, Stages 2 (hybrid search) and 3 (engram
// cache) run under the caller's context which may have no timeout at all —
// allowing a stalled VectorStore query or a slow worktree walk to hold up the
// pipeline indefinitely. On Termux / flaky WiFi this manifests as the ~3-minute
// "consultation hung" reports in the audit log.
const consultationPipelineMobile = 90 * time.Second   // mobile: 30 s × 3 stages
const consultationPipelineDesktop = 120 * time.Second // desktop: generous for large repos

// MediateRequest executes the full five-stage consultation pipeline and returns
// a validated, sanitised ConsultationResult ready for system-context injection.
//
// MUST be called from a goroutine — this function performs blocking network I/O.
// Use RunConsultationCmd to obtain the appropriate tea.Cmd wrapper.
//
// On validation failure (Stage 1) the error string is human-readable and is
// intended to be returned to the Primary model as a tool result, forcing it
// to correct its JSON and re-invoke the tool.
func (m *ConsultationMediator) MediateRequest(
	ctx context.Context,
	rawPayload []byte,
	history *ai.ConversationHistory,
) (ConsultationResult, error) {

	// ── Total pipeline deadline ───────────────────────────────────────────
	// Wrap ALL five stages in a single hard deadline that includes the hybrid
	// search (Stage 2) and engram cache lookup (Stage 3), not just the API call
	// (Stage 4). Without this, a stalled VectorStore query can hang the whole
	// pipeline for minutes on mobile networks.
	pipelineBudget := consultationPipelineDesktop
	if m.cfg.HAL.IsTermux || m.cfg.HAL.IsSBC {
		pipelineBudget = consultationPipelineMobile
	}
	pipelineCtx, pipelineCancel := context.WithTimeout(ctx, pipelineBudget)
	defer pipelineCancel()
	ctx = pipelineCtx // shadow: all stages below use the deadline-guarded ctx

	// ── Stage 1: Strict payload validation ───────────────────────────────
	// Parse and validate the EntropyVoid schema. On failure, return
	// immediately with a descriptive error — the retry ceiling is enforced
	// at the ConsultTool layer (each Execute call = one attempt). This keeps
	// MediateRequest idempotent and straightforward to unit-test.
	m.send(ConsultationStageMsg{Stage: StageValidating, Detail: "parsing EntropyVoid schema"})

	ev, err := ParseAndValidate(rawPayload)
	if err != nil {
		m.log.Warn("consultation/stage1: payload rejected", "error", err.Error())
		return ConsultationResult{}, err
	}
	m.log.Debug("consultation/stage1: payload valid",
		"expected_type", ev.ExpectedType,
		"void_len", len(ev.VoidTarget),
	)

	// ── Stage 3: Engram cache intercept (runs BEFORE Stage 2) ───────────
	// We probe the cache before building hybrid context because a cache hit
	// costs only one VectorStore scan, whereas a miss adds a hybrid search
	// on top. Ordering: cache → hybrid → API minimises wasted I/O.
	m.send(ConsultationStageMsg{Stage: StageCacheCheck, Detail: "querying engram store (threshold: 0.95)"})

	if cached, hit := m.cache.Lookup(ctx, ev); hit {
		m.log.Info("consultation/stage3: engram cache HIT — bypassing Secondary API",
			"void_prefix", ev.VoidTarget[:clamp(len(ev.VoidTarget), 60)],
		)
		result := ConsultationResult{Content: cached, FromCache: true}
		m.send(ConsultationDoneMsg{Content: result.Content, FromCache: true})
		return result, nil
	}
	m.log.Debug("consultation/stage3: cache miss")

	// ── Stage 2: Hybrid context pruning ──────────────────────────────────
	// Build a surgically compressed context window by merging:
	//   (a) Nomic semantic similarity hits from the VectorStore (topic-level)
	//   (b) Concurrent regex grep of the worktree (syntax-level exact matches)
	//   (c) AgeMem recent entries (tool preferences, ingested reasoning)
	// The merged block becomes the <CONTEXT> section in the Secondary prompt.
	m.send(ConsultationStageMsg{Stage: StageContextBuilding, Detail: "semantic + lexical hybrid search"})

	hybridCtx, err := m.hybrid.Build(ctx, ev.VoidTarget, history)
	if err != nil {
		// Non-fatal: degraded context still produces a useful Secondary answer.
		m.log.Warn("consultation/stage2: hybrid search degraded", "error", err.Error())
		hybridCtx = ""
	}

	// ── Stage 4: Synthetic thinking time — Secondary API, temp=0 ─────────
	// Build the suffocating directive and call the Secondary under a hard
	// deadline that adapts to the detected hardware profile.
	sec := m.secondary()
	if sec == nil {
		return ConsultationResult{}, ErrSecondaryUnavailable
	}

	// If the Secondary implements optional temperature control, force 0.
	if tc, ok := sec.(TemperatureConfigurable); ok {
		sec = tc.WithTemperature(0.0)
	}

	directive := buildSecondaryDirective(ev, hybridCtx)
	timeout := consultationDeadline(m.cfg.HAL)

	m.send(ConsultationStageMsg{
		Stage:  StageConsulting,
		Detail: fmt.Sprintf("calling %s (timeout=%s, temp=0)", sec.Name(), timeout),
	})

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, retries, callErr := m.callWithRetry(callCtx, sec, directive)
	if callErr != nil {
		m.send(ConsultationErrorMsg{Err: callErr, Stage: StageConsulting, Payload: string(rawPayload)})
		return ConsultationResult{Retries: retries},
			fmt.Errorf("consultation/stage4: %w", callErr)
	}

	// ── Stage 5: Airlock validation ───────────────────────────────────────
	// Sanitise the Secondary's response by type, then perform a reverse
	// semantic check: embed the response and compare it against the original
	// VoidTarget. Responses that drift too far (hallucinated fluff, off-topic
	// replies) are discarded and surface as ErrSignalDegradation.
	m.send(ConsultationStageMsg{Stage: StageValidatingResponse, Detail: "type sanitisation + semantic deviation check"})

	sanitised, err := m.airlock.Sanitise(ctx, ev, raw)
	if err != nil {
		m.log.Warn("consultation/stage5: airlock rejected response",
			"error", err.Error(), "raw_len", len(raw))
		m.send(ConsultationErrorMsg{Err: err, Stage: StageValidatingResponse, Payload: raw})
		return ConsultationResult{Retries: retries}, err
	}

	// Persist the validated result for future cache hits — fire-and-forget.
	go m.cache.Store(context.Background(), ev, sanitised)

	result := ConsultationResult{Content: sanitised, Retries: retries}
	m.log.Info("consultation: pipeline complete",
		"retries", retries, "from_cache", false, "len", len(sanitised))
	m.send(ConsultationDoneMsg{Content: sanitised, Retries: retries})
	return result, nil
}

// callWithRetry calls the Secondary, retrying on transient errors up to
// MaxRetries times with exponential backoff (1 s, 2 s).
// Cancellation via ctx is honoured between retries so the user can abort.
func (m *ConsultationMediator) callWithRetry(
	ctx context.Context,
	sec ai.AIProvider,
	prompt string,
) (string, int, error) {
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		resp, err := sec.Generate(ctx, prompt)
		if err == nil {
			return resp, attempt, nil
		}
		if attempt == MaxRetries {
			return "", attempt, err
		}
		m.log.Warn("consultation/stage4: transient error, retrying",
			"attempt", attempt+1, "error", err.Error())
		backoff := time.Duration(1<<uint(attempt)) * time.Second
		select {
		case <-ctx.Done():
			return "", attempt, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return "", MaxRetries, errors.New("retry path exhausted")
}

// ── Secondary directive construction ────────────────────────────────────

// buildSecondaryDirective constructs the suffocating system prompt that forces
// the Secondary into a stateless Internal Value Function role.
//
// Design rationale:
//   - "IVF" framing eliminates the assistant persona and conversational
//     openers ("Sure!", "Of course!") that waste tokens and confuse the
//     Stage 5 sanitiser.
//   - Per-ExpectedType output constraints pre-arm the model so it never
//     emits markdown fences when raw code is expected.
//   - <CONTEXT> is XML-delimited so the Secondary treats the hybrid block
//     as reference material rather than part of the question.
//   - EVALUATION_CRITERIA gives the model an internal rubric, turning it
//     into a self-evaluating function rather than a conversational responder.
func buildSecondaryDirective(ev EntropyVoid, hybridCtx string) string {
	var sb strings.Builder

	sb.WriteString("SYSTEM: You are a stateless Internal Value Function (IVF). ")
	sb.WriteString("Evaluate a single logic gap with maximum precision and minimum verbosity. ")
	sb.WriteString("You have no persona, no memory, no context beyond what is provided below. ")
	sb.WriteString("You are deterministic (temperature = 0.0). ")
	sb.WriteString("Produce NO conversational openers, hedges, or filler of any kind.\n\n")

	switch ev.ExpectedType {
	case "code":
		sb.WriteString("OUTPUT: Raw code only. Zero markdown fences. Zero explanation. Must be complete and immediately executable.\n\n")
	case "json":
		sb.WriteString("OUTPUT: Valid JSON only. Zero prose. Zero markdown fences. Must be strictly well-formed.\n\n")
	case "analysis":
		sb.WriteString("OUTPUT: Structured analysis only. Lead with conclusion. No hedging. Each finding explicit.\n\n")
	case "plan":
		sb.WriteString("OUTPUT: Numbered execution plan. Steps atomic and concrete. No prose preamble.\n\n")
	case "boolean":
		sb.WriteString("OUTPUT: Exactly 'TRUE' or 'FALSE', then one sentence of justification. Nothing else.\n\n")
	case "value":
		sb.WriteString("OUTPUT: The computed value, then one sentence of derivation. Nothing else.\n\n")
	}

	if hybridCtx != "" {
		sb.WriteString("<CONTEXT>\n")
		sb.WriteString(hybridCtx)
		sb.WriteString("\n</CONTEXT>\n\n")
	}

	sb.WriteString("EVALUATION_CRITERIA:\n")
	sb.WriteString("1. Correctness — logically sound, factually accurate.\n")
	sb.WriteString("2. Completeness — fully addresses void_target with zero gaps.\n")
	sb.WriteString("3. Precision    — every word carries signal; zero fluff.\n")
	sb.WriteString("4. Fidelity     — stays on-topic; zero subject drift.\n\n")

	sb.WriteString("LOGIC_GAP (void_target):\n")
	sb.WriteString(ev.VoidTarget)
	sb.WriteByte('\n')

	return sb.String()
}

// consultationDeadline returns the API call timeout adapted to the HAL profile.
func consultationDeadline(hal platform.HALProfile) time.Duration {
	if hal.IsTermux || hal.IsSBC {
		return consultationTimeoutMobile
	}
	return consultationTimeoutDesktop
}

// ── Optional temperature interface ────────────────────────────────────────

// TemperatureConfigurable is an optional interface providers may implement
// to allow per-call temperature override. The consultation package type-asserts
// the Secondary provider and forces temperature=0 when supported.
// Not implementing this interface is safe — the prompt-level directive still
// encourages deterministic behaviour on compliant models.
type TemperatureConfigurable interface {
	WithTemperature(temp float32) ai.AIProvider
}

// ── tea.Cmd factory ──────────────────────────────────────────────────────

// RunConsultationCmd returns a tea.Cmd that executes the full consultation
// pipeline in a goroutine managed by the Bubble Tea runtime, delivering the
// result as a ConsultationDoneMsg or ConsultationErrorMsg.
//
// This is the ONLY correct entry point from the TUI layer. Never call
// MediateRequest directly from a Bubble Tea Update function.
func RunConsultationCmd(
	m *ConsultationMediator,
	rawPayload []byte,
	history *ai.ConversationHistory,
) tea.Cmd {
	return func() tea.Msg {
		// Use a fresh context so the consultation's deadline is independent
		// of any parent context that might cancel early (e.g., user interrupt).
		result, err := m.MediateRequest(context.Background(), rawPayload, history)
		if err != nil {
			return ConsultationErrorMsg{Err: err, Payload: string(rawPayload)}
		}
		return ConsultationDoneMsg{Content: result.Content, FromCache: result.FromCache, Retries: result.Retries}
	}
}

// ConsultationResult.RetryCount is a named alias to avoid confusion with the
// unexported retries field in the internal call path.
func (r ConsultationResult) RetryCount() int { return r.Retries }

// ── Internal helpers ──────────────────────────────────────────────────────

// clamp returns min(n, max) — avoids importing math for a single trivial op.
func clamp(n, max int) int {
	if n > max {
		return max
	}
	return n
}
