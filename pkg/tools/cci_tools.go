package tools

// cci_tools.go — CCI Retrieval Tools
//
// Registers the native mcp_context_* tools that expose the Tier 3 Cold Memory
// store and Tier 2 specialist suggestion to the AI agent.
//
// Tool naming uses the mcp_context_ prefix to match the CCI spec's
// "dynamically wrapped retrieval functions" terminology, even though these are
// native Gorkbot tools (no external MCP server required).
//
// Tools:
//   mcp_context_list_subsystems  — list all documented subsystems (Tier 3)
//   mcp_context_get_subsystem    — retrieve a Tier 3 spec by name
//   mcp_context_suggest_specialist — suggest a Tier 2 domain for a task
//   mcp_context_update_subsystem   — write/update a Tier 3 living doc
//   mcp_context_list_specialists   — list available Tier 2 specialist domains
//   mcp_context_status             — full CCI system status

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/adaptive"
)

// CCIAccessor is the minimal interface the tools need from CCILayer.
// The concrete type is *adaptive.CCILayer; using an interface avoids a direct
// import of internal/engine from pkg/tools (which would be a cycle).
type CCIAccessor interface {
	GetStatus() string
}

// cciLayerKey is the context key used to store a *adaptive.CCILayer in ctx.
type cciLayerKey struct{}

// WithCCILayer stores the CCI layer in the context so tools can retrieve it.
func WithCCILayer(ctx context.Context, layer *adaptive.CCILayer) context.Context {
	return context.WithValue(ctx, cciLayerKey{}, layer)
}

// cciFromCtx retrieves the CCILayer from context. Returns nil if not set.
func cciFromCtx(ctx context.Context) *adaptive.CCILayer {
	v, _ := ctx.Value(cciLayerKey{}).(*adaptive.CCILayer)
	return v
}

// ── mcp_context_list_subsystems ──────────────────────────────────────────────

type cciListSubsystems struct{}

func (t *cciListSubsystems) Name() string                       { return "mcp_context_list_subsystems" }
func (t *cciListSubsystems) Category() ToolCategory             { return CategoryMeta }
func (t *cciListSubsystems) RequiresPermission() bool           { return false }
func (t *cciListSubsystems) DefaultPermission() PermissionLevel { return PermissionAlways }
func (t *cciListSubsystems) OutputFormat() OutputFormat         { return FormatText }
func (t *cciListSubsystems) Description() string {
	return "List all documented subsystems in the CCI Tier 3 cold memory knowledge base."
}
func (t *cciListSubsystems) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *cciListSubsystems) Execute(ctx context.Context, _ map[string]interface{}) (*ToolResult, error) {
	layer := cciFromCtx(ctx)
	if layer == nil {
		return &ToolResult{Success: false, Error: "CCI layer not initialized"}, nil
	}
	names := layer.ColdStore.ListSubsystems()
	if len(names) == 0 {
		return &ToolResult{Success: true, Output: "No subsystem specifications found in Tier 3 cold memory."}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## CCI Tier 3 Subsystems (%d documented)\n\n", len(names)))
	for _, n := range names {
		sb.WriteString("- `")
		sb.WriteString(n)
		sb.WriteString("`\n")
	}
	sb.WriteString("\nUse mcp_context_get_subsystem to retrieve a specific specification.")
	return &ToolResult{Success: true, Output: sb.String()}, nil
}

// ── mcp_context_get_subsystem ────────────────────────────────────────────────

type cciGetSubsystem struct{}

func (t *cciGetSubsystem) Name() string                       { return "mcp_context_get_subsystem" }
func (t *cciGetSubsystem) Category() ToolCategory             { return CategoryMeta }
func (t *cciGetSubsystem) RequiresPermission() bool           { return false }
func (t *cciGetSubsystem) DefaultPermission() PermissionLevel { return PermissionAlways }
func (t *cciGetSubsystem) OutputFormat() OutputFormat         { return FormatText }
func (t *cciGetSubsystem) Description() string {
	return "Retrieve the Tier 3 architectural specification for a specific subsystem from the CCI cold memory. Returns empty if undocumented (gap event — switch to PLAN mode)."
}
func (t *cciGetSubsystem) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Subsystem name (e.g. orchestrator, tui, tool-system, ai-providers, arc-mel, mcp, sense, cci, memory, subagents, session, security)"
			}
		},
		"required": ["name"]
	}`)
}

func (t *cciGetSubsystem) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	layer := cciFromCtx(ctx)
	if layer == nil {
		return &ToolResult{Success: false, Error: "CCI layer not initialized"}, nil
	}
	name, _ := params["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return &ToolResult{Success: false, Error: "name parameter is required"}, nil
	}

	content := layer.ColdStore.GetSubsystem(name)
	if content == "" {
		return &ToolResult{
			Success: false,
			Error: fmt.Sprintf("No Tier 3 specification found for subsystem %q.\n"+
				"CCI GAP DETECTED: Switch to PLAN mode and map the subsystem before modifying it.\n"+
				"After creating the spec, call mcp_context_update_subsystem to persist it.", name),
		}, nil
	}
	return &ToolResult{
		Success: true,
		Output:  adaptive.FormatSubsystemDoc(name, content),
	}, nil
}

// ── mcp_context_suggest_specialist ──────────────────────────────────────────

type cciSuggestSpecialist struct{}

func (t *cciSuggestSpecialist) Name() string                       { return "mcp_context_suggest_specialist" }
func (t *cciSuggestSpecialist) Category() ToolCategory             { return CategoryMeta }
func (t *cciSuggestSpecialist) RequiresPermission() bool           { return false }
func (t *cciSuggestSpecialist) DefaultPermission() PermissionLevel { return PermissionAlways }
func (t *cciSuggestSpecialist) OutputFormat() OutputFormat         { return FormatText }
func (t *cciSuggestSpecialist) Description() string {
	return "Suggest the appropriate CCI Tier 2 specialist domain for a given task description."
}
func (t *cciSuggestSpecialist) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "Description of the task or file paths involved"
			}
		},
		"required": ["task"]
	}`)
}

func (t *cciSuggestSpecialist) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	layer := cciFromCtx(ctx)
	if layer == nil {
		return &ToolResult{Success: false, Error: "CCI layer not initialized"}, nil
	}
	task, _ := params["task"].(string)
	if task == "" {
		return &ToolResult{Success: false, Error: "task parameter is required"}, nil
	}

	domain := layer.ColdStore.SuggestSpecialist(task)
	spec := layer.Specialists.Load(domain)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Suggested Tier 2 Specialist**: `%s`\n\n", domain))

	if spec != nil {
		sb.WriteString("Specialist persona is available and will be loaded automatically.\n")
		// Show first 400 chars of specialist content as preview.
		preview := spec.Content
		if len(preview) > 400 {
			preview = preview[:400] + "..."
		}
		sb.WriteString("\n**Preview:**\n```\n")
		sb.WriteString(preview)
		sb.WriteString("\n```\n")
	} else {
		sb.WriteString("⚠ No specialist file found for this domain yet.\n")
		sb.WriteString("The AI will rely on Tier 3 cold memory docs for this task.\n")
	}

	return &ToolResult{Success: true, Output: sb.String()}, nil
}

// ── mcp_context_update_subsystem ────────────────────────────────────────────

type cciUpdateSubsystem struct{}

func (t *cciUpdateSubsystem) Name() string                       { return "mcp_context_update_subsystem" }
func (t *cciUpdateSubsystem) Category() ToolCategory             { return CategoryMeta }
func (t *cciUpdateSubsystem) RequiresPermission() bool           { return true }
func (t *cciUpdateSubsystem) DefaultPermission() PermissionLevel { return PermissionSession }
func (t *cciUpdateSubsystem) OutputFormat() OutputFormat         { return FormatText }
func (t *cciUpdateSubsystem) Description() string {
	return "Write or update a Tier 3 CCI subsystem specification (living documentation). Used after code changes to keep specs synchronized."
}
func (t *cciUpdateSubsystem) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Subsystem name"
			},
			"content": {
				"type": "string",
				"description": "Full markdown content for the specification"
			}
		},
		"required": ["name", "content"]
	}`)
}

func (t *cciUpdateSubsystem) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	layer := cciFromCtx(ctx)
	if layer == nil {
		return &ToolResult{Success: false, Error: "CCI layer not initialized"}, nil
	}
	name, _ := params["name"].(string)
	content, _ := params["content"].(string)
	if name == "" || content == "" {
		return &ToolResult{Success: false, Error: "name and content parameters are required"}, nil
	}

	if err := layer.ColdStore.UpdateSubsystem(name, content); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to update spec: %v", err)}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Tier 3 spec for `%s` updated successfully.\nDrift detector will now recognize this subsystem as documented.", name),
	}, nil
}

// ── mcp_context_list_specialists ────────────────────────────────────────────

type cciListSpecialists struct{}

func (t *cciListSpecialists) Name() string                       { return "mcp_context_list_specialists" }
func (t *cciListSpecialists) Category() ToolCategory             { return CategoryMeta }
func (t *cciListSpecialists) RequiresPermission() bool           { return false }
func (t *cciListSpecialists) DefaultPermission() PermissionLevel { return PermissionAlways }
func (t *cciListSpecialists) OutputFormat() OutputFormat         { return FormatText }
func (t *cciListSpecialists) Description() string {
	return "List all available CCI Tier 2 specialist domains."
}
func (t *cciListSpecialists) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}
func (t *cciListSpecialists) Execute(ctx context.Context, _ map[string]interface{}) (*ToolResult, error) {
	layer := cciFromCtx(ctx)
	if layer == nil {
		return &ToolResult{Success: false, Error: "CCI layer not initialized"}, nil
	}
	domains := layer.Specialists.List()
	if len(domains) == 0 {
		return &ToolResult{Success: true, Output: "No Tier 2 specialists available yet."}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## CCI Tier 2 Specialists (%d available)\n\n", len(domains)))
	for _, d := range domains {
		sb.WriteString("- `")
		sb.WriteString(d)
		sb.WriteString("`\n")
	}
	return &ToolResult{Success: true, Output: sb.String()}, nil
}

// ── mcp_context_status ───────────────────────────────────────────────────────

type cciStatus struct{}

func (t *cciStatus) Name() string                       { return "mcp_context_status" }
func (t *cciStatus) Category() ToolCategory             { return CategoryMeta }
func (t *cciStatus) RequiresPermission() bool           { return false }
func (t *cciStatus) DefaultPermission() PermissionLevel { return PermissionAlways }
func (t *cciStatus) OutputFormat() OutputFormat         { return FormatText }
func (t *cciStatus) Description() string {
	return "Display the full CCI (Codified Context Infrastructure) system status: hot memory, specialists, cold docs."
}
func (t *cciStatus) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}
func (t *cciStatus) Execute(ctx context.Context, _ map[string]interface{}) (*ToolResult, error) {
	layer := cciFromCtx(ctx)
	if layer == nil {
		return &ToolResult{Success: false, Error: "CCI layer not initialized"}, nil
	}
	return &ToolResult{Success: true, Output: layer.GetStatus()}, nil
}

// ── Registration ─────────────────────────────────────────────────────────────

// RegisterCCITools registers all CCI retrieval tools in the given registry.
// Called from RegisterDefaultTools() in registry.go.
func RegisterCCITools(reg *Registry) {
	reg.Register(&cciListSubsystems{})
	reg.Register(&cciGetSubsystem{})
	reg.Register(&cciSuggestSpecialist{})
	reg.Register(&cciUpdateSubsystem{})
	reg.Register(&cciListSpecialists{})
	reg.Register(&cciStatus{})
}
