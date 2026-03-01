package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/usercommands"
)

type userCmdLoaderKey struct{}

// UserCommandLoaderKey is the context key for the usercommands.Loader
var UserCommandLoaderKey = userCmdLoaderKey{}

// WithUserCommandLoader injects a usercommands.Loader into ctx
func WithUserCommandLoader(ctx context.Context, l *usercommands.Loader) context.Context {
	return context.WithValue(ctx, UserCommandLoaderKey, l)
}

// DefineCommandTool allows the AI to define new user commands
type DefineCommandTool struct {
	BaseTool
}

func NewDefineCommandTool() *DefineCommandTool {
	return &DefineCommandTool{
		BaseTool: NewBaseTool(
			"define_command",
			"Define or update a user slash command that executes a prompt template. Use {{args}} as a placeholder for arguments passed when the command is invoked.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *DefineCommandTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Command name without the leading slash (e.g. 'summarize')"},
			"description": {"type": "string", "description": "Short description of what the command does"},
			"prompt": {"type": "string", "description": "Prompt template. Use {{args}} where the user's arguments should be inserted."}
		},
		"required": ["name", "description", "prompt"]
	}`)
}

func (t *DefineCommandTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, _ := params["name"].(string)
	desc, _ := params["description"].(string)
	prompt, _ := params["prompt"].(string)

	if name == "" || prompt == "" {
		return &ToolResult{Success: false, Error: "name and prompt are required"}, nil
	}
	name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "/")

	loader, ok := ctx.Value(UserCommandLoaderKey).(*usercommands.Loader)
	if !ok || loader == nil {
		return &ToolResult{Success: false, Error: "user command loader not available"}, nil
	}

	cmd := usercommands.UserCommand{Name: name, Description: desc, Prompt: prompt}
	if err := loader.Define(cmd); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to save command: %v", err)}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Command /%s defined. Use it as a slash command: /%s <args>", name, name),
	}, nil
}
