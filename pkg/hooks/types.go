// Package hooks provides a lifecycle event system for Gorkbot.
// Hook scripts in ~/.config/gorkbot/hooks/ receive JSON on stdin and
// communicate via exit codes (0=proceed, 2=block) and stderr.
//
// Event coverage was expanded (inspired by oh-my-opencode's 46-hook system)
// to provide fine-grained lifecycle observability across message flow,
// model routing, context management, and agent coordination.
package hooks

import "time"

// Event identifies a lifecycle point in Gorkbot's execution.
type Event string

const (
	// ── Session lifecycle ─────────────────────────────────────────────────────

	// EventSessionStart fires once when the TUI initialises.
	EventSessionStart Event = "session_start"
	// EventSessionEnd fires when the user quits gracefully.
	EventSessionEnd Event = "session_end"

	// ── Message flow ─────────────────────────────────────────────────────────

	// EventPreMessageSend fires before a user prompt is sent to the AI.
	// Exit code 2 blocks the send and shows the reason to the user.
	EventPreMessageSend Event = "pre_message_send"
	// EventPostMessageReceive fires after the AI completes a response.
	EventPostMessageReceive Event = "post_message_receive"

	// ── Tool execution ────────────────────────────────────────────────────────

	// EventPreToolUse fires before any tool executes.
	// Exit code 2 blocks the tool.
	EventPreToolUse Event = "pre_tool_use"
	// EventPostToolUse fires after a tool completes successfully.
	EventPostToolUse Event = "post_tool_use"
	// EventPostToolFailure fires after a tool errors or is blocked.
	EventPostToolFailure Event = "post_tool_failure"

	// ── Context & compaction ──────────────────────────────────────────────────

	// EventPreCompaction fires before SENSE compresses the context.
	EventPreCompaction Event = "pre_compaction"
	// EventPostCompaction fires after SENSE compresses the context.
	EventPostCompaction Event = "post_compaction"
	// EventContextWarning fires when the context window crosses a usage threshold.
	// Payload.Extra["level"] = "warn" (80%) or "critical" (95%).
	// Payload.Extra["used_pct"] = float64 usage percentage.
	EventContextWarning Event = "context_warning"

	// ── Routing & model selection ─────────────────────────────────────────────

	// EventIntentDetected fires after the ARC router classifies the user's intent.
	// Payload.Extra["category"] = IntentCategory string.
	// Payload.Extra["workflow"] = WorkflowType string.
	EventIntentDetected Event = "intent_detected"
	// EventModelFallback fires when the primary model fails and a fallback is used.
	// Payload.Extra["from_model"] = original model ID.
	// Payload.Extra["to_model"]   = fallback model ID.
	// Payload.Extra["reason"]     = error reason for fallback.
	EventModelFallback Event = "model_fallback"

	// ── Consultant (Gemini / secondary AI) ────────────────────────────────────

	// EventConsultantInvoked fires just before the consultant AI is queried.
	// Payload.Extra["trigger"] = reason string ("keyword" | "length" | "intent").
	EventConsultantInvoked Event = "consultant_invoked"
	// EventConsultantResponse fires after the consultant AI replies.
	// Payload.Extra["response_length"] = int length of response.
	EventConsultantResponse Event = "consultant_response"

	// ── Plan mode ────────────────────────────────────────────────────────────

	// EventPlanStarted fires when plan mode is activated.
	EventPlanStarted Event = "plan_started"
	// EventPlanComplete fires when plan mode finishes or is cancelled.
	EventPlanComplete Event = "plan_complete"

	// ── Background agents ─────────────────────────────────────────────────────

	// EventBackgroundAgentStarted fires when a background sub-agent is spawned.
	// Payload.Extra["agent_id"] = agent ID string.
	// Payload.Extra["label"]    = agent label string.
	EventBackgroundAgentStarted Event = "background_agent_started"
	// EventBackgroundAgentDone fires when a background sub-agent completes.
	// Payload.Extra["agent_id"] = agent ID.
	// Payload.Extra["status"]   = "done" | "failed" | "cancelled".
	// Payload.Extra["elapsed"]  = duration string.
	EventBackgroundAgentDone Event = "background_agent_done"

	// ── Ralph Loop ───────────────────────────────────────────────────────────

	// EventRalphLoopTriggered fires when the self-referential retry loop activates.
	// Payload.Extra["iteration"]         = current iteration number.
	// Payload.Extra["failures_this_run"] = number of tool failures this iteration.
	EventRalphLoopTriggered Event = "ralph_loop_triggered"

	// ── Subagents (existing, kept for compatibility) ──────────────────────────

	// EventSubagentStart fires when a subagent is spawned.
	EventSubagentStart Event = "subagent_start"
	// EventSubagentStop fires when a subagent completes.
	EventSubagentStop Event = "subagent_stop"

	// ── Notifications ────────────────────────────────────────────────────────

	// EventOnNotification fires for AI-generated notifications.
	EventOnNotification Event = "on_notification"

	// ── Mode changes ─────────────────────────────────────────────────────────

	// EventModeChange fires when the execution mode changes.
	EventModeChange Event = "mode_change"
)

// ContextWarningLevel indicates how severe a context window warning is.
type ContextWarningLevel string

const (
	ContextWarnLevelWarn     ContextWarningLevel = "warn"     // 80% usage
	ContextWarnLevelCritical ContextWarningLevel = "critical" // 95% usage
)

// Payload is the JSON object sent to hook scripts on stdin.
type Payload struct {
	Event     Event                  `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id,omitempty"`
	Tool      string                 `json:"tool,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
	Result    *ResultInfo            `json:"result,omitempty"`
	Mode      string                 `json:"mode,omitempty"`
	// Extra carries event-specific metadata. See individual event comments.
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// ResultInfo summarises a tool execution result in hook payloads.
type ResultInfo struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HookResult carries the outcome of running a hook script.
type HookResult struct {
	Blocked bool   // true if exit code was 2
	Reason  string // stderr output from script
	Err     error  // Process/IO error (not exit code 2)
}

// AllEvents returns every defined Event constant for documentation/tooling.
func AllEvents() []Event {
	return []Event{
		EventSessionStart,
		EventSessionEnd,
		EventPreMessageSend,
		EventPostMessageReceive,
		EventPreToolUse,
		EventPostToolUse,
		EventPostToolFailure,
		EventPreCompaction,
		EventPostCompaction,
		EventContextWarning,
		EventIntentDetected,
		EventModelFallback,
		EventConsultantInvoked,
		EventConsultantResponse,
		EventPlanStarted,
		EventPlanComplete,
		EventBackgroundAgentStarted,
		EventBackgroundAgentDone,
		EventRalphLoopTriggered,
		EventSubagentStart,
		EventSubagentStop,
		EventOnNotification,
		EventModeChange,
	}
}
