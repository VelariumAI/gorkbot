package tools

import (
	"context"
	"encoding/json"
)

type SendEmailTool struct {
	BaseTool
}

func NewSendEmailTool() *SendEmailTool {
	return &SendEmailTool{
		BaseTool: BaseTool{
			name:               "send_email",
			description:        "Sends an email using standard SMTP (if configured).",
			category:           CategoryCommunication,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SendEmailTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"to":      map[string]interface{}{"type": "string", "description": "Recipient address"},
			"subject": map[string]interface{}{"type": "string", "description": "Email subject"},
			"body":    map[string]interface{}{"type": "string", "description": "Email body"},
		},
		"required": []string{"to", "subject", "body"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *SendEmailTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{Error: "Not implemented: SMTP credentials not configured."}, nil
}

type SlackNotifyTool struct {
	BaseTool
}

func NewSlackNotifyTool() *SlackNotifyTool {
	return &SlackNotifyTool{
		BaseTool: BaseTool{
			name:               "slack_post",
			description:        "Posts a message to Slack.",
			category:           CategoryCommunication,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SlackNotifyTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"channel": map[string]interface{}{"type": "string", "description": "Channel name"},
			"message": map[string]interface{}{"type": "string", "description": "Message content"},
		},
		"required": []string{"channel", "message"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *SlackNotifyTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{Error: "Not implemented: Webhook missing."}, nil
}
