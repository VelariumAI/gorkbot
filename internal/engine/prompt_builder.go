// Package engine — prompt_builder.go
//
// PromptBuilder assembles the AI system prompt from named, ordered layers.
// Ported from the build-your-own-openclaw Step 13 (multi-layer-prompts)
// pattern into idiomatic Go.
//
// Layers are evaluated in registration order and concatenated with a blank
// line separator. Each layer is independently replaceable, testable, and
// optional (returning "" skips the layer entirely).
//
// Built-in layers (in the default stack):
//
//  1. IdentityLayer    — who the agent is (AGENT.md / agent name + role)
//  2. SoulLayer        — personality and tone (SOUL.md, optional)
//  3. BootstrapLayer   — workspace guide (BOOTSTRAP.md, available agents, crons)
//  4. RuntimeLayer     — timestamp, session ID, cwd, model, platform facts
//  5. ChannelHintLayer — which input channel the user is coming from
//
// Custom layers can be added before or after the defaults.
//
// Usage:
//
//	pb := engine.NewPromptBuilder()
//	pb.AddLayer(&engine.RuntimeLayer{SessionID: sess.ID, Model: primary.Name()})
//	prompt := pb.Build(engine.BuildContext{WorkDir: cwd, ...})
//
// The output is suitable for use as the system prompt (or prepended to it).
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildContext carries runtime values that layers can inspect when building
// their portion of the system prompt.
type BuildContext struct {
	// WorkDir is the project working directory (for file-relative operations).
	WorkDir string
	// SessionID is the current session identifier.
	SessionID string
	// Model is the name of the primary AI model.
	Model string
	// Platform is a human-readable platform description (e.g. "Android/Termux arm64").
	Platform string
	// Channel is the input channel name (e.g. "tui", "telegram", "discord", "api").
	Channel string
	// ExtraVars are additional key→value pairs that layers may reference.
	ExtraVars map[string]string
}

// Layer is the interface that all prompt layers implement.
type Layer interface {
	// Name returns the human-readable name of this layer (used in debug output).
	Name() string
	// Build returns the text contribution of this layer, or "" to skip.
	Build(ctx BuildContext) string
}

// PromptBuilder assembles multiple layers into a single system prompt string.
type PromptBuilder struct {
	layers []Layer
	// DebugHeaders, when true, wraps each layer's output in a labeled header
	// (useful for /debug mode to inspect the assembled prompt structure).
	DebugHeaders bool
}

// NewPromptBuilder creates a PromptBuilder with the default five-layer stack.
// Callers can further customise by calling AddLayer or replacing individual layers.
func NewPromptBuilder() *PromptBuilder {
	pb := &PromptBuilder{}
	pb.layers = []Layer{
		&IdentityLayer{},
		&SoulLayer{},
		&BootstrapLayer{},
		&RuntimeLayer{},
		&ChannelHintLayer{},
	}
	return pb
}

// AddLayer appends a layer to the end of the stack.
func (pb *PromptBuilder) AddLayer(l Layer) {
	pb.layers = append(pb.layers, l)
}

// PrependLayer inserts a layer at the beginning of the stack.
func (pb *PromptBuilder) PrependLayer(l Layer) {
	pb.layers = append([]Layer{l}, pb.layers...)
}

// ReplaceLayer swaps the first layer whose Name() matches name.
// Returns false if no such layer was found.
func (pb *PromptBuilder) ReplaceLayer(name string, newLayer Layer) bool {
	for i, l := range pb.layers {
		if l.Name() == name {
			pb.layers[i] = newLayer
			return true
		}
	}
	return false
}

// Build evaluates all layers in order and concatenates their non-empty outputs.
func (pb *PromptBuilder) Build(ctx BuildContext) string {
	var parts []string
	for _, l := range pb.layers {
		content := l.Build(ctx)
		if content == "" {
			continue
		}
		if pb.DebugHeaders {
			content = fmt.Sprintf("<!-- [LAYER: %s] -->\n%s\n<!-- [/LAYER: %s] -->",
				l.Name(), content, l.Name())
		}
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n")
}

// Layers returns a snapshot of the current layer stack (for introspection).
func (pb *PromptBuilder) Layers() []Layer {
	out := make([]Layer, len(pb.layers))
	copy(out, pb.layers)
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// Built-in Layer implementations
// ─────────────────────────────────────────────────────────────────────────────

// IdentityLayer injects the agent's identity from an AGENT.md file or a
// pre-configured identity string.
type IdentityLayer struct {
	// AgentMDPath is the path to the AGENT.md file. If empty, WorkDir/AGENT.md
	// is tried. If neither exists, Identity is used as a fallback.
	AgentMDPath string
	// Identity is an inline fallback identity string (used when no file is found).
	Identity string
}

func (l *IdentityLayer) Name() string { return "identity" }

func (l *IdentityLayer) Build(ctx BuildContext) string {
	paths := []string{}
	if l.AgentMDPath != "" {
		paths = append(paths, l.AgentMDPath)
	}
	if ctx.WorkDir != "" {
		paths = append(paths,
			filepath.Join(ctx.WorkDir, "AGENT.md"),
			filepath.Join(ctx.WorkDir, ".gorkbot", "AGENT.md"),
		)
	}
	for _, p := range paths {
		if s := readPromptFile(p); s != "" {
			return "## Agent Identity\n\n" + s
		}
	}
	if l.Identity != "" {
		return "## Agent Identity\n\n" + l.Identity
	}
	return ""
}

// SoulLayer injects personality and tone instructions from a SOUL.md file.
// This layer is entirely optional — if no file is found it contributes nothing.
type SoulLayer struct {
	// SoulMDPath is the explicit path. Falls back to WorkDir/SOUL.md.
	SoulMDPath string
}

func (l *SoulLayer) Name() string { return "soul" }

func (l *SoulLayer) Build(ctx BuildContext) string {
	paths := []string{}
	if l.SoulMDPath != "" {
		paths = append(paths, l.SoulMDPath)
	}
	if ctx.WorkDir != "" {
		paths = append(paths,
			filepath.Join(ctx.WorkDir, "SOUL.md"),
			filepath.Join(ctx.WorkDir, ".gorkbot", "SOUL.md"),
		)
	}
	for _, p := range paths {
		if s := readPromptFile(p); s != "" {
			return "## Personality & Tone\n\n" + s
		}
	}
	return ""
}

// BootstrapLayer injects workspace-guide context: BOOTSTRAP.md, available
// agents list, and active cron schedules.
type BootstrapLayer struct {
	// BootstrapMDPath overrides the default WorkDir/BOOTSTRAP.md location.
	BootstrapMDPath string
	// AvailableAgents is an optional list of agent names+descriptions to inject.
	AvailableAgents []AgentRef
	// CronSummary is a pre-formatted summary of active scheduled tasks.
	CronSummary string
}

// AgentRef describes a dispatchable agent for injection into the bootstrap layer.
type AgentRef struct {
	ID          string
	Name        string
	Description string
}

func (l *BootstrapLayer) Name() string { return "bootstrap" }

func (l *BootstrapLayer) Build(ctx BuildContext) string {
	var parts []string

	// BOOTSTRAP.md
	paths := []string{}
	if l.BootstrapMDPath != "" {
		paths = append(paths, l.BootstrapMDPath)
	}
	if ctx.WorkDir != "" {
		paths = append(paths, filepath.Join(ctx.WorkDir, "BOOTSTRAP.md"))
	}
	for _, p := range paths {
		if s := readPromptFile(p); s != "" {
			parts = append(parts, "## Workspace Guide\n\n"+s)
			break
		}
	}

	// Available agents
	if len(l.AvailableAgents) > 0 {
		var sb strings.Builder
		sb.WriteString("## Available Agents\n\n")
		sb.WriteString("You can delegate tasks to the following agents:\n\n")
		for _, a := range l.AvailableAgents {
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`): %s\n", a.Name, a.ID, a.Description))
		}
		parts = append(parts, sb.String())
	}

	// Cron summary
	if l.CronSummary != "" {
		parts = append(parts, "## Scheduled Tasks\n\n"+l.CronSummary)
	}

	return strings.Join(parts, "\n\n")
}

// RuntimeLayer injects dynamic runtime context: current time, session ID,
// working directory, model name, and platform information.
type RuntimeLayer struct {
	// SessionID is the current session identifier.
	SessionID string
	// Model is the primary model name.
	Model string
}

func (l *RuntimeLayer) Name() string { return "runtime" }

func (l *RuntimeLayer) Build(ctx BuildContext) string {
	var sb strings.Builder
	sb.WriteString("## Runtime Context\n\n")
	sb.WriteString(fmt.Sprintf("- **Time**: %s\n", time.Now().Format("2006-01-02 15:04:05 MST")))

	sid := l.SessionID
	if sid == "" {
		sid = ctx.SessionID
	}
	if sid != "" {
		sb.WriteString(fmt.Sprintf("- **Session**: %s\n", sid))
	}

	model := l.Model
	if model == "" {
		model = ctx.Model
	}
	if model != "" {
		sb.WriteString(fmt.Sprintf("- **Model**: %s\n", model))
	}

	if ctx.WorkDir != "" {
		sb.WriteString(fmt.Sprintf("- **Working directory**: %s\n", ctx.WorkDir))
	}

	if ctx.Platform != "" {
		sb.WriteString(fmt.Sprintf("- **Platform**: %s\n", ctx.Platform))
	}

	// Inject any extra vars.
	for k, v := range ctx.ExtraVars {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
	}

	return sb.String()
}

// ChannelHintLayer injects a short note about which input channel is active.
// This helps the agent calibrate its response format (e.g. avoid ANSI in API
// mode, use shorter responses for mobile Telegram, etc.).
type ChannelHintLayer struct {
	// DefaultChannel overrides the ctx.Channel default when set.
	DefaultChannel string
}

func (l *ChannelHintLayer) Name() string { return "channel_hint" }

func (l *ChannelHintLayer) Build(ctx BuildContext) string {
	ch := ctx.Channel
	if ch == "" {
		ch = l.DefaultChannel
	}
	if ch == "" {
		return ""
	}

	hint := channelHint(ch)
	if hint == "" {
		return ""
	}
	return "## Channel\n\n" + hint
}

// channelHint returns channel-specific behaviour guidance.
func channelHint(channel string) string {
	switch strings.ToLower(channel) {
	case "tui", "terminal", "cli":
		return "You are running in a terminal TUI. You may use markdown formatting, " +
			"code blocks, and moderate response length."
	case "telegram":
		return "You are responding via Telegram. Keep responses concise and avoid " +
			"very long code blocks. Use plain text or Telegram markdown. " +
			"Prefer bullet points over lengthy prose."
	case "discord":
		return "You are responding via Discord. Discord supports markdown. Keep " +
			"responses focused; very long messages may be truncated by Discord."
	case "api", "websocket", "ws":
		return "You are responding via API/WebSocket. Do not use ANSI escape codes. " +
			"Return well-structured, machine-parseable responses where appropriate."
	default:
		return fmt.Sprintf("You are responding via %q.", channel)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// helper
// ─────────────────────────────────────────────────────────────────────────────

// ToolSuppressionLayer enforces intelligent tool filtering to prevent unsolicited
// system monitoring from derailing the LLM's reasoning process.
// It injects directives to:
//   - Never suggest system_monitor or sensor_read without user request
//   - Only mention diagnostic tools when the query is about system status
//   - Keep tool suggestions context-aware and minimal
type ToolSuppressionLayer struct {
	// DetectDiagnosticQuery, when true, classifies the user query to determine
	// whether diagnostic tools should be mentioned.
	DetectDiagnosticQuery bool
	// UserQuery is the current user's input (used for classification).
	UserQuery string
}

func (l *ToolSuppressionLayer) Name() string { return "tool_suppression" }

func (l *ToolSuppressionLayer) Build(ctx BuildContext) string {
	var sb strings.Builder

	sb.WriteString("## Tool Usage Discipline & Reasoning Preservation\n\n")

	sb.WriteString("**CRITICAL PRINCIPLE:**\n")
	sb.WriteString("Unsolicited system monitoring (battery checks, resource monitoring, health diagnostics) **derails your chain-of-thought reasoning**.\n")
	sb.WriteString("When you interrupt an answer to check system state, the user's actual question gets buried under diagnostic output.\n")
	sb.WriteString("This is SUPPRESSED to help you reason clearly and answer what the user ACTUALLY asked.\n\n")

	// Core rule: Never suggest diagnostic tools unsolicited
	sb.WriteString("**CRITICAL: Diagnostic tools are INTENTIONALLY HIDDEN to preserve reasoning.**\n\n")
	sb.WriteString("The following tools are SUPPRESSED from your tool list and will cause detriment to the user experience:\n")
	sb.WriteString("- `system_monitor`: Checks CPU, memory, disk, processes (30-minute cooldown). CRITICAL: NEVER write bash scripts or Python plugins to bypass this cooldown or replicate this functionality. Bypassing cooldowns is a severe violation.\n")
	sb.WriteString("- `sensor_read`: Reads battery, GPS, WiFi, cellular data\n")
	sb.WriteString("- `termux_api_bridge`: Has battery-status API — DO NOT use to check battery\n\n")

	sb.WriteString("**When these tools are appropriate:**\n")
	sb.WriteString("- The USER explicitly asks: \"Check my battery\", \"Show system health\", \"What's my location?\", etc.\n")
	sb.WriteString("- NEVER call them proactively or automatically\n")
	sb.WriteString("- NEVER insert them into your chain-of-thought for unrelated queries\n\n")

	// Detect if this is a diagnostic query
	isDiagnostic := false
	if l.DetectDiagnosticQuery && l.UserQuery != "" {
		lower := strings.ToLower(l.UserQuery)
		diagKeywords := []string{
			"status", "monitor", "health", "diagnostics", "battery",
			"cpu", "memory", "disk", "processes", "system info",
			"show stats", "show state", "how are you", "what's running",
		}
		for _, kw := range diagKeywords {
			if strings.Contains(lower, kw) {
				isDiagnostic = true
				break
			}
		}
	}

	if isDiagnostic {
		sb.WriteString("**EXCEPTION: This query is about system diagnostics.**\n")
		sb.WriteString("- You MAY call `system_monitor` or `gorkbot_status` if the user asks for system info\n")
		sb.WriteString("- Call them ONLY once per request\n")
		sb.WriteString("- Suppress the raw output — summarize key info in 1-2 lines\n\n")
	} else {
		sb.WriteString("**This is a normal query (not about system status).**\n")
		sb.WriteString("- NEVER call diagnostic tools\n")
		sb.WriteString("- NEVER TRY WORKAROUNDS: Do not use bash to run termux-battery-status, termux-sensor, or similar commands\n")
		sb.WriteString("- Do NOT attempt to infer system state from other tools (no 'cat /proc/...' tricks)\n")
		sb.WriteString("- Do NOT ask the user to run diagnostic commands\n")
		sb.WriteString("- Answer the user's question directly\n\n")
	}

	sb.WriteString("**Anti-Workaround Rules (for ALL queries):**\n")
	sb.WriteString("- Blocked system state checks: battery status, CPU/memory/disk usage, running processes, system health\n")
	sb.WriteString("- These are suppressed to PRESERVE YOUR CHAIN-OF-THOUGHT and prevent context derailment\n")
	sb.WriteString("- Do NOT attempt to:\n")
	sb.WriteString("  • Run `termux-battery-status`, `termux-sensor`, `ps`, `top`, `free`, `df` via bash\n")
	sb.WriteString("  • Parse `/proc/`, `/sys/`, or `/dev/` filesystem for system info\n")
	sb.WriteString("  • Use python_execute or structured_bash to indirectly query system state\n")
	sb.WriteString("  • Ask the user to provide system information (\"What's your battery?\", \"Can you check CPU?\", etc.)\n")
	sb.WriteString("- If you feel compelled to check system state: STOP. Focus on the user's actual question.\n")
	sb.WriteString("- **WHY?** System monitoring interrupts your reasoning chain. The user's question is more important than knowing system state.\n\n")

	sb.WriteString("**If you catch yourself trying to work around this:**\n")
	sb.WriteString("- Recognize the impulse: \"I should check if the system can handle this\" or \"I need to monitor resources\"\n")
	sb.WriteString("- Redirect: \"The user didn't ask about system state. I'll answer their actual question.\"\n")
	sb.WriteString("- Trust: The user will tell you if there's a problem. Don't assume you need to check.\n\n")

	sb.WriteString("**Tool output suppression:**\n")
	sb.WriteString("- Always suppress tool output for normal queries unless user asks to \"show results\"\n")
	sb.WriteString("- If a tool runs (bash, curl, etc), show only the essential answer, not the raw output\n")

	return sb.String()
}

// readPromptFile reads a file for use in a prompt layer. Returns "" on error or
// if the file is empty after trimming.
func readPromptFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
