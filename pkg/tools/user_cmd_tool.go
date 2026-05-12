package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/velariumai/gorkbot/pkg/governance"
	"github.com/velariumai/gorkbot/pkg/selfmod"
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
			"prompt": {"type": "string", "description": "Prompt template. Use {{args}} where the user's arguments should be inserted."},
			"manifest": {"description": "Self-modification manifest (required in non-off governance modes). Accepts object or JSON string."}
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
	if err := selfmod.ValidateSafeArtifactName(name); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("invalid command name: %v", err)}, nil
	}

	mode := governanceModeFromContext(ctx)
	if mode != governance.GOVERNANCE_OFF {
		validation := selfmod.ValidateDynamicProposal(selfmod.ValidateInput{
			OperationID: uuid.NewString(),
			ToolName:    "define_command",
			Mode:        string(mode),
			Parameters:  paramsToAny(params),
		})
		if !validation.Allowed || validation.HardBlock {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("dynamic proposal blocked: %s", validation.ReasonCode),
			}, nil
		}
		stagePath := filepath.Join(".gorkbot", "staging", "commands", name+".json")
		if _, blocked, reason, issue := selfmod.ValidateStagedTargetPath(filepath.ToSlash(stagePath)); blocked {
			return &ToolResult{Success: false, Error: fmt.Sprintf("stage path rejected: %s (%s)", reason, issue)}, nil
		}
		payload := map[string]string{"name": name, "description": desc, "prompt": prompt}
		b, _ := json.MarshalIndent(payload, "", "  ")
		if err := os.MkdirAll(filepath.Dir(stagePath), 0755); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to create staging dir: %v", err)}, nil
		}
		if err := os.WriteFile(stagePath, b, 0600); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to stage command: %v", err)}, nil
		}
		return &ToolResult{
			Success: true,
			Output:  fmt.Sprintf("Command /%s validated and staged at %s (not active yet).", name, stagePath),
		}, nil
	}

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
