package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// CalendarManageTool interacts with Google Calendar (gcalcli).
type CalendarManageTool struct {
	BaseTool
}

func NewCalendarManageTool() *CalendarManageTool {
	return &CalendarManageTool{
		BaseTool: BaseTool{
			name:               "calendar_manage",
			description:        "Manage calendar events using gcalcli.",
			category:           CategoryCommunication,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *CalendarManageTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "add", "delete"},
				"description": "Action to perform.",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Event title (for add).",
			},
			"when": map[string]interface{}{
				"type":        "string",
				"description": "Date/Time (e.g., 'tomorrow 5pm').",
			},
		},
		"required": []string{"action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CalendarManageTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	action, _ := args["action"].(string)
	title, _ := args["title"].(string)
	when, _ := args["when"].(string)

	var cmd *exec.Cmd
	switch action {
	case "list":
		cmd = exec.CommandContext(ctx, "gcalcli", "agenda")
	case "add":
		cmd = exec.CommandContext(ctx, "gcalcli", "add", "--title", title, "--when", when, "--noprompt")
	case "delete":
		cmd = exec.CommandContext(ctx, "gcalcli", "delete", title, "--noprompt")
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: "gcalcli failed (install and auth gcalcli first)."}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// EmailClientTool sends/reads email (mutt or similar CLI).
type EmailClientTool struct {
	BaseTool
}

func NewEmailClientTool() *EmailClientTool {
	return &EmailClientTool{
		BaseTool: BaseTool{
			name:               "email_client",
			description:        "Read/Send email via CLI (mutt/neomutt).",
			category:           CategoryCommunication,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *EmailClientTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"send", "read"},
				"description": "Action.",
			},
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Recipient address.",
			},
			"subject": map[string]interface{}{
				"type":        "string",
				"description": "Subject line.",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Body text.",
			},
		},
		"required": []string{"action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *EmailClientTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	action, _ := args["action"].(string)
	to, _ := args["to"].(string)
	subj, _ := args["subject"].(string)
	body, _ := args["body"].(string)

	if action == "send" {
		script := fmt.Sprintf("echo \"%s\" | neomutt -s \"%s\" %s", body, subj, to)
		cmd := exec.CommandContext(ctx, "bash", "-c", script)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("Email send failed: %v\n%s", err, string(out))}, nil
		}
		return &ToolResult{Success: true, Output: "Email sent."}, nil
	}
	return &ToolResult{Success: false, Error: "Email reading not fully implemented for non-interactive mode."}, nil
}

// ContactSyncTool (uses termux-contact-list or similar).
type ContactSyncTool struct {
	BaseTool
}

func NewContactSyncTool() *ContactSyncTool {
	return &ContactSyncTool{
		BaseTool: BaseTool{
			name:               "contact_sync",
			description:        "Sync or list contacts.",
			category:           CategoryCommunication,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ContactSyncTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list"},
				"description": "Action (list).",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ContactSyncTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	cmd := exec.CommandContext(ctx, "termux-contact-list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to list contacts."}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// SmartHomeApiTool calls Home Assistant API.
type SmartHomeApiTool struct {
	BaseTool
}

func NewSmartHomeApiTool() *SmartHomeApiTool {
	return &SmartHomeApiTool{
		BaseTool: BaseTool{
			name:               "smart_home_api",
			description:        "Control smart home devices via Home Assistant API.",
			category:           CategoryNetwork,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SmartHomeApiTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "Home Assistant URL.",
			},
			"token": map[string]interface{}{
				"type":        "string",
				"description": "Long-lived access token.",
			},
			"entity_id": map[string]interface{}{
				"type":        "string",
				"description": "Entity ID (e.g., light.living_room).",
			},
			"service": map[string]interface{}{
				"type":        "string",
				"description": "Service (turn_on, turn_off).",
			},
		},
		"required": []string{"url", "token", "service"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SmartHomeApiTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	url, _ := args["url"].(string)
	token, _ := args["token"].(string)
	entity, _ := args["entity_id"].(string)
	service, _ := args["service"].(string)

	apiEndpoint := fmt.Sprintf("%s/api/services/homeassistant/%s", url, service)
	payload := fmt.Sprintf(`{"entity_id": "%s"}`, entity)

	cmd := exec.CommandContext(ctx, "curl", "-X", "POST", "-H", "Authorization: Bearer "+token, "-H", "Content-Type: application/json", "-d", payload, apiEndpoint)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: "Smart home call failed."}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}
