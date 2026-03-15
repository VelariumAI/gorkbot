package consultation

// tool.go — ConsultTool: the tools.Tool interface implementation
//
// ConsultTool is the bridge between the Primary model's tool-calling mechanism
// and the ConsultationMediator pipeline. The Primary invokes it like any other
// tool by providing a JSON payload conforming to the EntropyVoid schema.
//
// Retry ceiling enforcement:
//   ConsultTool tracks consecutive validation failures in a session-scoped
//   atomic counter. After MaxRetries failures, Execute returns a hard stop
//   error rather than allowing the Primary to loop indefinitely on broken JSON.
//   A successful invocation resets the counter.
//
// Universal Truth injection:
//   When TruthInjector is set (wired by the orchestrator), ConsultTool calls
//   it with the validated truth so it can be upserted into the Primary's
//   conversation history as a system message (not a tool_result message).
//   This makes the truth a persistent, authoritative context observation rather
//   than a transient conversational exchange.
//
// Bubble Tea integration:
//   All blocking work (embedding, search, API) runs inside the Mediator which
//   is called synchronously from Execute. Execute itself is called by the
//   orchestrator's tool dispatcher — which already runs in a goroutine separate
//   from the Bubble Tea main loop. Progress updates are forwarded to the TUI
//   via the MediatorConfig.SendMsg callback.
//
//   For use cases where ConsultTool is triggered directly from the TUI (e.g.,
//   a slash command), use NewConsultCmd to get a tea.Cmd that wraps Execute
//   into a Bubble Tea message pipeline.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// consultToolParams is the JSON schema for the Primary model's tool invocation.
// Two variants are accepted:
//  1. Flat: {"context_hash":"…","void_target":"…","expected_type":"…"}
//     The simplest form; the Primary marshals EntropyVoid directly.
//  2. Wrapped: {"payload": "{…json-string…}"}
//     Used by some models that wrap tool arguments in a single "payload" field.
var consultToolParams = json.RawMessage(`{
  "type": "object",
  "description": "Invoke the Secondary consultation model via the EntropyVoid protocol",
  "properties": {
    "context_hash": {
      "type": "string",
      "description": "SHA-256 hex digest (64 chars) of the current conversation slice. Call compute_context_hash to obtain."
    },
    "void_target": {
      "type": "string",
      "description": "The precise logic gap or question to resolve. Max 4096 chars. Be as specific as possible."
    },
    "expected_type": {
      "type": "string",
      "enum": ["code","json","analysis","plan","boolean","value"],
      "description": "The format you expect the Secondary to return. Constrains airlock sanitisation."
    }
  },
  "required": ["context_hash","void_target","expected_type"]
}`)

// consultHashHelperParams is the schema for the context-hash helper tool.
var consultHashHelperParams = json.RawMessage(`{
  "type": "object",
  "description": "Computes the SHA-256 context hash required by the consult_secondary tool",
  "properties": {},
  "required": []
}`)

// ── ConsultTool ───────────────────────────────────────────────────────────

// ConsultTool implements the tools.Tool interface. Register it in the tool
// registry via tools.RegisterDefaultTools or call Register directly.
type ConsultTool struct {
	tools.BaseTool
	mediator      *ConsultationMediator
	history       *ai.ConversationHistory
	TruthInjector func(tag, truth string) // optional; set by orchestrator
	failCount     atomic.Int32            // consecutive validation failures
	mu            sync.Mutex              // guards failCount reset on success
}

// NewConsultTool constructs a ConsultTool.
//
//	mediator — the wired ConsultationMediator (must be non-nil)
//	history  — the Primary's ConversationHistory (used for ComputeContextHash
//	            and for Universal Truth injection if TruthInjector is nil)
func NewConsultTool(mediator *ConsultationMediator, history *ai.ConversationHistory) *ConsultTool {
	return &ConsultTool{
		BaseTool: tools.NewBaseTool(
			"consult_secondary",
			"Invoke the Secondary AI consultant via the EntropyVoid protocol. "+
				"Provides a validated, sanitised answer for a precisely scoped logic gap. "+
				"Use this when you need an architectural assessment, a code correctness check, "+
				"a JSON schema, or a boolean decision that requires an independent evaluator.",
			tools.CategoryAI,
			false,                  // no interactive permission prompt needed
			tools.PermissionAlways, // auto-approved: Secondary is a trusted subsystem
		),
		mediator: mediator,
		history:  history,
	}
}

// Parameters returns the JSON schema for the Primary model.
func (t *ConsultTool) Parameters() json.RawMessage { return consultToolParams }

// OutputFormat signals structured JSON output so the orchestrator renders
// the result verbatim rather than wrapping it in markdown.
func (t *ConsultTool) OutputFormat() tools.OutputFormat { return tools.FormatJSON }

// Execute runs the consultation pipeline synchronously.
//
// This method MUST only be called from the orchestrator's tool dispatcher
// goroutine, never from the Bubble Tea Update function.
//
// Parameter extraction order:
//  1. Try flat: {"context_hash","void_target","expected_type"}
//  2. Try wrapped: {"payload": "<json-string>"}
func (t *ConsultTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	// Check retry ceiling BEFORE parsing payload so the Primary gets
	// the ceiling error even on a brand-new (possibly still malformed) attempt.
	if count := t.failCount.Load(); count > MaxRetries {
		t.failCount.Store(0) // reset so a genuine new question can proceed
		return &tools.ToolResult{
			Success: false,
			Error: fmt.Sprintf(
				"%s: %d consecutive validation failures on consult_secondary; "+
					"retry ceiling exceeded. Reformulate the VoidTarget or request as a different type.",
				ErrRetryLimitExceeded.Error(), count,
			),
		}, nil
	}

	// Extract the raw JSON payload from parameters.
	rawPayload, err := extractPayload(params)
	if err != nil {
		t.failCount.Add(1)
		return &tools.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Execute the five-stage pipeline.
	result, medErr := t.mediator.MediateRequest(ctx, rawPayload, t.history)
	if medErr != nil {
		t.failCount.Add(1)
		// Return a structured error the Primary can read and self-correct.
		// The raw error message from ParseAndValidate / Stage 5 is intentionally
		// verbose and human-readable.
		return &tools.ToolResult{
			Success: false,
			Error:   medErr.Error(),
		}, nil
	}

	// Success: reset the failure counter.
	t.failCount.Store(0)

	// ── Universal Truth injection ─────────────────────────────────────────
	// Inject the validated response as a system-level "Universal Truth" rather
	// than a conversational tool_result. This makes it a persistent authoritative
	// observation that the Primary always sees, independent of tool-call threading.
	t.injectTruth(result.Content)

	cacheNote := ""
	if result.FromCache {
		cacheNote = " (served from engram cache — no API call)"
	}

	return &tools.ToolResult{
		Success: true,
		Output:  result.Content,
		Data: map[string]interface{}{
			"from_cache":  result.FromCache,
			"retries":     result.Retries,
			"cache_note":  cacheNote,
			"void_target": extractVoidTargetPreview(rawPayload),
		},
	}, nil
}

// injectTruth calls TruthInjector if set, otherwise falls back to
// UpsertSystemMessage on the conversation history directly.
func (t *ConsultTool) injectTruth(truth string) {
	tag := "consultation-truth"
	content := tag + ":\n" + truth

	if t.TruthInjector != nil {
		t.TruthInjector(tag, content)
		return
	}
	// Fallback: inject directly into history (still authoritative as a
	// system message, but without the callback abstraction).
	if t.history != nil {
		t.history.UpsertSystemMessage(tag, content)
	}
}

// ── Context hash helper tool ──────────────────────────────────────────────

// ContextHashTool is a companion tool that computes the context_hash the
// Primary must supply when calling consult_secondary. Registering it alongside
// ConsultTool prevents the Primary from guessing or hallucinating hash values.
type ContextHashTool struct {
	tools.BaseTool
	history *ai.ConversationHistory
}

// NewContextHashTool constructs the hash helper tool.
func NewContextHashTool(history *ai.ConversationHistory) *ContextHashTool {
	return &ContextHashTool{
		BaseTool: tools.NewBaseTool(
			"compute_context_hash",
			"Computes the SHA-256 context_hash required by consult_secondary. "+
				"Call this immediately before constructing an EntropyVoid payload.",
			tools.CategoryAI,
			false,
			tools.PermissionAlways,
		),
		history: history,
	}
}

func (t *ContextHashTool) Parameters() json.RawMessage      { return consultHashHelperParams }
func (t *ContextHashTool) OutputFormat() tools.OutputFormat { return tools.FormatText }

// Execute computes and returns the current context hash.
func (t *ContextHashTool) Execute(_ context.Context, _ map[string]interface{}) (*tools.ToolResult, error) {
	hash := ComputeContextHash(t.history)
	return &tools.ToolResult{
		Success: true,
		Output:  hash,
		Data:    map[string]interface{}{"context_hash": hash},
	}, nil
}

// ── tea.Cmd factories ─────────────────────────────────────────────────────

// NewConsultCmd returns a tea.Cmd that invokes the ConsultTool from within
// the Bubble Tea event loop (e.g., triggered by a slash command or keybinding).
// The result is delivered as a ConsultationDoneMsg or ConsultationErrorMsg.
//
// ev must be a pre-validated EntropyVoid (use ParseAndValidate first if
// constructing from user input). history is the Primary's live history.
func NewConsultCmd(mediator *ConsultationMediator, ev EntropyVoid, history *ai.ConversationHistory) tea.Cmd {
	raw, _ := json.Marshal(ev)
	return RunConsultationCmd(mediator, raw, history)
}

// NewConsultFromParamsCmd returns a tea.Cmd that accepts raw tool params
// (as the orchestrator dispatcher would pass them). Suitable for wiring
// the tool invocation from a TUI slash command handler.
func NewConsultFromParamsCmd(
	mediator *ConsultationMediator,
	params map[string]interface{},
	history *ai.ConversationHistory,
) tea.Cmd {
	return func() tea.Msg {
		raw, err := extractPayload(params)
		if err != nil {
			return ConsultationErrorMsg{Err: err, Stage: StageValidating, Payload: fmt.Sprintf("%v", params)}
		}
		result, medErr := mediator.MediateRequest(context.Background(), raw, history)
		if medErr != nil {
			return ConsultationErrorMsg{Err: medErr, Payload: string(raw)}
		}
		return ConsultationDoneMsg{Content: result.Content, FromCache: result.FromCache, Retries: result.Retries}
	}
}

// ── Parameter extraction ──────────────────────────────────────────────────

// extractPayload extracts and returns a raw JSON []byte from tool params.
// It handles two call conventions:
//
//  1. Flat keys: {"context_hash":"…","void_target":"…","expected_type":"…"}
//     The Primary marshals the EntropyVoid fields directly as tool arguments.
//
//  2. Wrapped key: {"payload": "{…json-string…}"}
//     Some models wrap structured arguments in a single "payload" string.
func extractPayload(params map[string]interface{}) ([]byte, error) {
	// Convention 1: flat keys — check for the three required fields.
	if _, ok := params["void_target"]; ok {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("consult_secondary: failed to marshal flat params: %w", err)
		}
		return b, nil
	}

	// Convention 2: wrapped "payload" string.
	if raw, ok := params["payload"]; ok {
		switch v := raw.(type) {
		case string:
			return []byte(v), nil
		case []byte:
			return v, nil
		}
	}

	// Neither convention matched — construct a helpful error.
	return nil, fmt.Errorf(
		`%w: consult_secondary requires {"context_hash":"…","void_target":"…","expected_type":"…"} or {"payload":"{…}"}; got keys: %v`,
		ErrMalformedPayload, paramKeys(params),
	)
}

// paramKeys returns sorted keys for error messages.
func paramKeys(params map[string]interface{}) []string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	return keys
}

// extractVoidTargetPreview returns the first 80 chars of void_target for
// inclusion in ToolResult.Data without requiring a full unmarshal.
func extractVoidTargetPreview(rawPayload []byte) string {
	var partial struct {
		VoidTarget string `json:"void_target"`
	}
	if err := json.Unmarshal(rawPayload, &partial); err != nil {
		return ""
	}
	if len(partial.VoidTarget) > 80 {
		return partial.VoidTarget[:80] + "…"
	}
	return partial.VoidTarget
}
