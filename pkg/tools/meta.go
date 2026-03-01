package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreateToolTool is the "DIY tool" that allows creating new tools
type CreateToolTool struct {
	BaseTool
}

func NewCreateToolTool() *CreateToolTool {
	return &CreateToolTool{
		BaseTool: BaseTool{
			name:              "create_tool",
			description:       "Create a new custom tool by generating Go code (DIY tool creator)",
			category:          CategoryMeta,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

func (t *CreateToolTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Tool name in snake_case (e.g. my_custom_tool)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable description of what the tool does",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Tool category",
				"enum":        []string{"shell", "file", "git", "web", "system", "communication", "meta", "ai", "database", "network", "media", "android", "package", "custom"},
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Bash command template. Use {{param_name}} placeholders for parameters.",
			},
			"parameters": map[string]interface{}{
				"type":        "object",
				"description": "Parameter definitions. Each key is a param name. Value can be a type string (e.g. 'string') or a full spec object {type, description, required, default}.",
			},
			"requires_permission": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether the tool must ask the user for permission before running (default: true)",
			},
			"default_permission": map[string]interface{}{
				"type":        "string",
				"description": "Default permission policy applied when the tool is first encountered",
				"enum":        []string{"always", "session", "once", "never"},
			},
		},
		"required": []string{"name", "description", "command"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CreateToolTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, ok := params["name"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "name is required"}, fmt.Errorf("name required")
	}

	description, ok := params["description"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "description is required"}, fmt.Errorf("description required")
	}

	command, ok := params["command"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "command is required"}, fmt.Errorf("command required")
	}

	category := "custom"
	if c, ok := params["category"].(string); ok {
		category = c
	}

	requiresPermission := true
	if r, ok := params["requires_permission"].(bool); ok {
		requiresPermission = r
	}

	defaultPermission := "once"
	if p, ok := params["default_permission"].(string); ok {
		defaultPermission = p
	}

	// Build DynamicToolParam map from the parameters input.
	// The caller may provide either map[string]string (name→type) or
	// map[string]map[string]interface{} (full param spec).
	toolParams := make(map[string]DynamicToolParam)
	if p, ok := params["parameters"].(map[string]interface{}); ok {
		for paramName, paramVal := range p {
			switch v := paramVal.(type) {
			case string:
				// Simple shorthand: name → type string
				toolParams[paramName] = DynamicToolParam{
					Type:        v,
					Description: paramName + " parameter",
					Required:    true,
				}
			case map[string]interface{}:
				// Full spec: { type, description, required, default }
				tp := DynamicToolParam{
					Type:     "string",
					Required: true,
				}
				if t, ok := v["type"].(string); ok {
					tp.Type = t
				}
				if d, ok := v["description"].(string); ok {
					tp.Description = d
				} else {
					tp.Description = paramName + " parameter"
				}
				if r, ok := v["required"].(bool); ok {
					tp.Required = r
				}
				if def, ok := v["default"].(string); ok {
					tp.Default = def
				}
				toolParams[paramName] = tp
			}
		}
	}

	cfg := DynamicToolConfig{
		Name:               name,
		Description:        description,
		Category:           category,
		Command:            command,
		Parameters:         toolParams,
		RequiresPermission: requiresPermission,
		DefaultPermission:  defaultPermission,
	}

	// Get registry from context and register the tool live.
	reg, hasRegistry := ctx.Value(registryContextKey).(*Registry)

	var persistErr error
	if hasRegistry && reg != nil {
		persistErr = reg.SaveDynamicTool(cfg)
	}

	// Always also write the Go stub into pkg/tools/custom/ as a reference.
	paramDefs := make(map[string]string, len(toolParams))
	for k, v := range toolParams {
		paramDefs[k] = v.Type
	}
	code := generateToolCode(name, description, category, command, paramDefs, requiresPermission, defaultPermission)

	customDir := "pkg/tools/custom"
	filePath := filepath.Join(customDir, fmt.Sprintf("%s.go", name))
	if err := os.MkdirAll(customDir, 0755); err == nil {
		_ = os.WriteFile(filePath, []byte(code), 0644)
	}

	status := "registered live (no restart needed)"
	if !hasRegistry || reg == nil {
		status = "WARNING: registry unavailable — tool NOT registered live; restart required"
	} else if persistErr != nil {
		status = fmt.Sprintf("registered live but persistence failed (%v); will not survive restart", persistErr)
	}

	return &ToolResult{
		Success: true,
		Output: fmt.Sprintf(
			"Custom tool '%s' created successfully.\n\nStatus: %s\n\nYou can use it immediately with:\n<tool name=\"%s\">...</tool>",
			name, status, name,
		),
		Data: map[string]interface{}{
			"tool_name": name,
			"file_path": filePath,
			"live":      hasRegistry && reg != nil,
		},
	}, nil
}

// generateToolCode generates Go source code for a custom tool
func generateToolCode(name, description, category, command string, params map[string]string, requiresPermission bool, defaultPermission string) string {
	// Convert snake_case to PascalCase
	structName := toPascalCase(name) + "Tool"

	// Generate parameter schema
	paramSchema := ""
	requiredParams := []string{}
	for paramName, paramType := range params {
		paramSchema += fmt.Sprintf("\t\t\t\"%s\": map[string]interface{}{\n", paramName)
		paramSchema += fmt.Sprintf("\t\t\t\t\"type\":        \"%s\",\n", paramType)
		paramSchema += fmt.Sprintf("\t\t\t\t\"description\": \"%s parameter\",\n", paramName)
		paramSchema += "\t\t\t},\n"
		requiredParams = append(requiredParams, paramName)
	}

	requiredJSON := ""
	if len(requiredParams) > 0 {
		requiredJSON = fmt.Sprintf("\"required\": []string{%s},\n\t\t", strings.Join(wrapQuotes(requiredParams), ", "))
	}

	// Generate parameter extraction code
	paramExtraction := ""
	commandBuilder := strings.ReplaceAll(command, "{{", "${")
	commandBuilder = strings.ReplaceAll(commandBuilder, "}}", "}")

	for paramName := range params {
		paramExtraction += fmt.Sprintf("\t%s := \"\"\n", paramName)
		paramExtraction += fmt.Sprintf("\tif p, ok := params[\"%s\"].(string); ok {\n", paramName)
		paramExtraction += fmt.Sprintf("\t\t%s = p\n", paramName)
		paramExtraction += "\t}\n\n"

		// Replace in command
		commandBuilder = strings.ReplaceAll(commandBuilder, fmt.Sprintf("{{%s}}", paramName), fmt.Sprintf("\" + shellescape(%s) + \"", paramName))
	}

	permissionStr := "true"
	if !requiresPermission {
		permissionStr = "false"
	}

	categoryConst := "CategoryCustom"
	switch category {
	case "shell":
		categoryConst = "CategoryShell"
	case "file":
		categoryConst = "CategoryFile"
	case "git":
		categoryConst = "CategoryGit"
	case "web":
		categoryConst = "CategoryWeb"
	case "system":
		categoryConst = "CategorySystem"
	case "communication":
		categoryConst = "CategoryCommunication"
	}

	permLevel := "PermissionOnce"
	switch defaultPermission {
	case "always":
		permLevel = "PermissionAlways"
	case "session":
		permLevel = "PermissionSession"
	case "never":
		permLevel = "PermissionNever"
	}

	template := `package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// %s - Custom generated tool
type %s struct {
	BaseTool
}

func New%s() *%s {
	return &%s{
		BaseTool: BaseTool{
			name:              "%s",
			description:       "%s",
			category:          %s,
			requiresPermission: %s,
			defaultPermission: %s,
		},
	}
}

func (t *%s) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
%s		},
		%s}
	data, _ := json.Marshal(schema)
	return data
}

func (t *%s) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
%s
	command := "%s"

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
`

	return fmt.Sprintf(template,
		structName, structName,
		structName, structName, structName,
		name, description,
		categoryConst, permissionStr, permLevel,
		structName,
		paramSchema,
		requiredJSON,
		structName,
		paramExtraction,
		commandBuilder,
	)
}

// toPascalCase converts snake_case to PascalCase
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// wrapQuotes wraps each string in quotes
func wrapQuotes(strs []string) []string {
	result := make([]string, len(strs))
	for i, s := range strs {
		result[i] = fmt.Sprintf("\"%s\"", s)
	}
	return result
}

// ListToolsTool lists all registered tools
type ListToolsTool struct {
	BaseTool
}

func NewListToolsTool() *ListToolsTool {
	return &ListToolsTool{
		BaseTool: BaseTool{
			name:              "list_tools",
			description:       "List all available tools with their descriptions and permissions",
			category:          CategoryMeta,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *ListToolsTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Filter by category (optional)",
			},
			"format": map[string]interface{}{
				"type":        "string",
				"description": "Output format: table, json, detailed (default: table)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ListToolsTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	// Get registry from context (set by Registry.Execute)
	registry, ok := ctx.Value(registryContextKey).(*Registry)
	if !ok || registry == nil {
		// Fallback if registry not in context
		return &ToolResult{
			Success: false,
			Error:   "Tool registry not available in context",
		}, fmt.Errorf("registry not in context")
	}

	format := "table"
	if f, ok := params["format"].(string); ok {
		format = f
	}

	categoryFilter := ""
	if c, ok := params["category"].(string); ok {
		categoryFilter = c
	}

	// Get all tools from registry
	allTools := registry.List()

	// Filter by category if specified
	var tools []Tool
	if categoryFilter != "" {
		for _, tool := range allTools {
			if string(tool.Category()) == categoryFilter {
				tools = append(tools, tool)
			}
		}
	} else {
		tools = allTools
	}

	// Build output based on format
	var output string
	switch format {
	case "json":
		// JSON format
		toolList := make([]map[string]interface{}, 0, len(tools))
		for _, tool := range tools {
			toolList = append(toolList, map[string]interface{}{
				"name":        tool.Name(),
				"description": tool.Description(),
				"category":    string(tool.Category()),
			})
		}
		jsonData, _ := json.MarshalIndent(map[string]interface{}{
			"tools": toolList,
			"count": len(tools),
		}, "", "  ")
		output = string(jsonData)

	case "detailed":
		// Detailed format
		output = fmt.Sprintf("# Available Tools (%d)\n\n", len(tools))
		categories := make(map[string][]Tool)
		for _, tool := range tools {
			cat := string(tool.Category())
			categories[cat] = append(categories[cat], tool)
		}

		for cat, catTools := range categories {
			output += fmt.Sprintf("## %s (%d)\n\n", cat, len(catTools))
			for _, tool := range catTools {
				output += fmt.Sprintf("### %s\n", tool.Name())
				output += fmt.Sprintf("**Description:** %s\n\n", tool.Description())
			}
		}

	default:
		// Table format (default)
		output = fmt.Sprintf("Available Tools: %d\n\n", len(tools))
		output += "NAME | CATEGORY | DESCRIPTION\n"
		output += "---- | -------- | -----------\n"
		for _, tool := range tools {
			desc := tool.Description()
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			output += fmt.Sprintf("%s | %s | %s\n", tool.Name(), tool.Category(), desc)
		}
	}

	return &ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"format": format,
			"category": categoryFilter,
			"count": len(tools),
		},
	}, nil
}

// ToolInfoTool gets detailed information about a specific tool
type ToolInfoTool struct {
	BaseTool
}

func NewToolInfoTool() *ToolInfoTool {
	return &ToolInfoTool{
		BaseTool: BaseTool{
			name:              "tool_info",
			description:       "Get detailed information about a specific tool",
			category:          CategoryMeta,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *ToolInfoTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tool_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the tool to get info about",
			},
		},
		"required": []string{"tool_name"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ToolInfoTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	toolName, ok := params["tool_name"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "tool_name is required"}, fmt.Errorf("tool_name required")
	}

	registry, ok := ctx.Value(registryContextKey).(*Registry)
	if !ok || registry == nil {
		return &ToolResult{Success: false, Error: "registry not available"}, fmt.Errorf("registry not in context")
	}

	tool, exists := registry.Get(toolName)
	if !exists {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool %q not found; use list_tools to see available tools", toolName),
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Tool: %s\n\n", tool.Name()))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", tool.Description()))
	sb.WriteString(fmt.Sprintf("**Category:** %s\n\n", string(tool.Category())))
	sb.WriteString(fmt.Sprintf("**Requires Permission:** %v\n\n", tool.RequiresPermission()))
	sb.WriteString(fmt.Sprintf("**Default Permission:** %s\n\n", string(tool.DefaultPermission())))

	// Parse and display parameters from JSON schema
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schema); err == nil {
		if props, ok := schema["properties"].(map[string]interface{}); ok && len(props) > 0 {
			sb.WriteString("**Parameters:**\n")
			required := map[string]bool{}
			if req, ok := schema["required"].([]interface{}); ok {
				for _, r := range req {
					if s, ok := r.(string); ok {
						required[s] = true
					}
				}
			}
			for pName, pDef := range props {
				req := ""
				if required[pName] {
					req = " (required)"
				}
				sb.WriteString(fmt.Sprintf("- `%s`%s", pName, req))
				if pMap, ok := pDef.(map[string]interface{}); ok {
					if tp, ok := pMap["type"].(string); ok {
						sb.WriteString(fmt.Sprintf(" [%s]", tp))
					}
					if desc, ok := pMap["description"].(string); ok {
						sb.WriteString(": " + desc)
					}
				}
				sb.WriteString("\n")
			}
		} else {
			sb.WriteString("**Parameters:** none\n")
		}
	}

	return &ToolResult{
		Success: true,
		Output:  sb.String(),
		Data:    map[string]interface{}{"tool_name": toolName},
	}, nil
}

// ─── ModifyToolTool ───────────────────────────────────────────────────────────

// ModifyToolTool lets the agent update an existing tool's command, description,
// parameters, category, or permissions at runtime. Changes are hot-loaded into
// the live registry immediately. When a compiled-in (static) tool is modified,
// an override DynamicScriptTool is created and a rebuild is scheduled.
type ModifyToolTool struct {
	BaseTool
}

func NewModifyToolTool() *ModifyToolTool {
	return &ModifyToolTool{
		BaseTool: BaseTool{
			name:               "modify_tool",
			description:        "Modify an existing tool's command, description, parameters, category, or permissions. Changes are hot-loaded into the live registry immediately. Static (compiled-in) tools are overridden at runtime and queued for a rebuild.",
			category:           CategoryMeta,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *ModifyToolTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the tool to modify",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "New description (leave blank to keep current)",
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "New bash command template with {{param}} placeholders (leave blank to keep current)",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "New category (leave blank to keep current)",
				"enum":        []string{"shell", "file", "git", "web", "system", "communication", "meta", "ai", "database", "network", "media", "android", "package", "custom"},
			},
			"parameters": map[string]interface{}{
				"type":        "object",
				"description": "Parameter definitions to add or update. Each key is a param name; value can be a type string or full spec {type, description, required, default}. Existing params not mentioned are preserved unless replace_parameters is true.",
			},
			"replace_parameters": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, replaces all parameters with the provided set instead of merging (default: false)",
			},
			"requires_permission": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether the tool must ask the user for permission before running",
			},
			"default_permission": map[string]interface{}{
				"type":        "string",
				"description": "Default permission policy",
				"enum":        []string{"always", "session", "once", "never"},
			},
		},
		"required": []string{"name"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ModifyToolTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, ok := params["name"].(string)
	if !ok || name == "" {
		return &ToolResult{Success: false, Error: "name is required"}, fmt.Errorf("name required")
	}

	reg, ok := ctx.Value(registryContextKey).(*Registry)
	if !ok || reg == nil {
		return &ToolResult{Success: false, Error: "registry not available"}, fmt.Errorf("registry not available")
	}

	existing, exists := reg.Get(name)
	if !exists {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool %q not found; use list_tools to see available tools", name),
		}, nil
	}

	// ── Build the updated DynamicToolConfig ─────────────────────────────────
	var cfg DynamicToolConfig
	isStatic := false

	if ds, ok := existing.(*DynamicScriptTool); ok {
		// Dynamic tool — start from its existing config so we only overwrite
		// what the caller explicitly changed.
		cfg = ds.config
	} else {
		// Static compiled tool — synthesise a skeleton from its public interface.
		// The caller must supply at least a new command to make it functional.
		isStatic = true
		cfg = DynamicToolConfig{
			Name:               existing.Name(),
			Description:        existing.Description(),
			Category:           string(existing.Category()),
			Command:            "",
			Parameters:         make(map[string]DynamicToolParam),
			RequiresPermission: existing.RequiresPermission(),
			DefaultPermission:  string(existing.DefaultPermission()),
		}
	}

	// ── Apply caller-supplied overrides ─────────────────────────────────────
	if v, ok := params["description"].(string); ok && v != "" {
		cfg.Description = v
	}
	if v, ok := params["command"].(string); ok && v != "" {
		cfg.Command = v
	}
	if v, ok := params["category"].(string); ok && v != "" {
		cfg.Category = v
	}
	if v, ok := params["requires_permission"].(bool); ok {
		cfg.RequiresPermission = v
	}
	if v, ok := params["default_permission"].(string); ok && v != "" {
		cfg.DefaultPermission = v
	}

	// ── Merge or replace parameters ─────────────────────────────────────────
	replaceParams, _ := params["replace_parameters"].(bool)
	if newParams, ok := params["parameters"].(map[string]interface{}); ok {
		if replaceParams {
			cfg.Parameters = make(map[string]DynamicToolParam)
		}
		for paramName, paramVal := range newParams {
			switch v := paramVal.(type) {
			case string:
				cfg.Parameters[paramName] = DynamicToolParam{
					Type:        v,
					Description: paramName + " parameter",
					Required:    true,
				}
			case map[string]interface{}:
				// Merge onto existing definition (if any) so partial updates work.
				tp := cfg.Parameters[paramName]
				if tp.Type == "" {
					tp.Type = "string"
				}
				if t2, ok := v["type"].(string); ok {
					tp.Type = t2
				}
				if d, ok := v["description"].(string); ok {
					tp.Description = d
				}
				if tp.Description == "" {
					tp.Description = paramName + " parameter"
				}
				if r, ok := v["required"].(bool); ok {
					tp.Required = r
				}
				if def, ok := v["default"].(string); ok {
					tp.Default = def
				}
				cfg.Parameters[paramName] = tp
			}
		}
	}

	// ── Validate that a static override has a command ────────────────────────
	if isStatic && cfg.Command == "" {
		return &ToolResult{
			Success: false,
			Error: fmt.Sprintf(
				"%q is a compiled-in tool; you must provide a 'command' to override its behaviour",
				name,
			),
		}, nil
	}

	// ── Hot-load: register/replace in live registry + persist ────────────────
	var persistErr error
	persistErr = reg.SaveDynamicTool(cfg)

	// For static overrides: write Go stub for rebuild + mark pending
	if isStatic {
		paramDefs := make(map[string]string, len(cfg.Parameters))
		for k, v := range cfg.Parameters {
			paramDefs[k] = v.Type
		}
		code := generateToolCode(
			cfg.Name, cfg.Description, cfg.Category, cfg.Command,
			paramDefs, cfg.RequiresPermission, cfg.DefaultPermission,
		)
		customDir := "pkg/tools/custom"
		filePath := filepath.Join(customDir, fmt.Sprintf("%s.go", name))
		if err := os.MkdirAll(customDir, 0755); err == nil {
			_ = os.WriteFile(filePath, []byte(code), 0644)
		}
		reg.MarkPendingRebuild(name)
	}

	// ── Build result message ─────────────────────────────────────────────────
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool '%s' modified successfully.\n\n", name))

	if isStatic {
		sb.WriteString("Type: Static (compiled-in) → overridden by dynamic wrapper\n")
		sb.WriteString("Rebuild required for permanent integration.\n")
		sb.WriteString("  Run: go build -o grokster ./cmd/grokster/\n")
		sb.WriteString("  (Grokster will notify you on exit if a rebuild is pending.)\n")
	} else {
		sb.WriteString("Type: Dynamic — hot-loaded, no rebuild needed.\n")
	}

	if persistErr != nil {
		sb.WriteString(fmt.Sprintf("\nPersistence warning: %v\n(tool is live but may not survive restart)\n", persistErr))
	} else {
		sb.WriteString("\nTool is active immediately and will reload on next startup.\n")
	}

	return &ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"tool_name":       name,
			"is_static":       isStatic,
			"live":            true,
			"needs_rebuild":   isStatic,
		},
	}, nil
}
