package contracts

// ToolActionContract is a normalized governance-facing tool action shape.
type ToolActionContract struct {
	ActionID   string         `json:"action_id"`
	ToolName   string         `json:"tool_name"`
	Capability string         `json:"capability"`
	Actor      string         `json:"actor"`
	RiskClass  string         `json:"risk_class"`
	Parameters map[string]any `json:"parameters"`
	Workspace  string         `json:"workspace,omitempty"`
	CreatedAt  string         `json:"created_at"`
}
