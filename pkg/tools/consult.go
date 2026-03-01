package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// ConsultationTool allows the primary agent to consult with the configured secondary AI provider.
type ConsultationTool struct {
	BaseTool
}

func NewConsultationTool() *ConsultationTool {
	return &ConsultationTool{
		BaseTool: BaseTool{
			name:               "consultation",
			description:        "Consult with the configured secondary AI provider for complex tasks, planning, or a second opinion. Uses the secondary model selected via Ctrl+T, or automatically selects the 2nd best model if auto mode is enabled.",
			category:           CategoryMeta,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *ConsultationTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *ConsultationTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The question or task for the secondary AI to analyze.",
			},
			"context": map[string]interface{}{
				"type":        "string",
				"description": "Additional context to provide (optional).",
			},
			"model": map[string]interface{}{
				"type":        "string",
				"description": "Optional model override for the secondary provider (e.g. grok-3-mini, gemini-2.0-flash).",
			},
		},
		"required": []string{"query"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ConsultationTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	query, ok := params["query"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "query is required", OutputFormat: FormatError}, fmt.Errorf("query required")
	}

	additionalContext := ""
	if c, ok := params["context"].(string); ok {
		additionalContext = c
	}

	// Get Registry from context
	registry, ok := ctx.Value(registryContextKey).(*Registry)
	if !ok || registry == nil {
		return &ToolResult{Success: false, Error: "registry not available", OutputFormat: FormatError}, fmt.Errorf("registry not available")
	}

	// Get secondary provider - uses ResolveConsultantProvider to handle auto mode
	// If no model explicitly selected, it will use the 2nd best model dynamically
	consultant := registry.ResolveConsultantProvider(ctx, query)
	if consultant == nil {
		return &ToolResult{
			Success:      false,
			Error:        "No secondary AI provider available. Configure a secondary model via Ctrl+T or add API keys for multiple providers.",
			OutputFormat: FormatError,
		}, fmt.Errorf("consultant not available")
	}

	aiProvider, ok := consultant.(ai.AIProvider)
	if !ok {
		return &ToolResult{Success: false, Error: "invalid consultant provider type", OutputFormat: FormatError}, fmt.Errorf("invalid consultant provider type")
	}

	// Optional per-call model override
	if model, ok := params["model"].(string); ok && model != "" {
		aiProvider = aiProvider.WithModel(model)
	}

	// Construct the prompt
	fullPrompt := query
	if additionalContext != "" {
		fullPrompt = fmt.Sprintf("CONTEXT:\n%s\n\nQUERY:\n%s", additionalContext, query)
	}

	// Execute
	response, err := aiProvider.Generate(ctx, fullPrompt)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Consultation failed: %v", err), OutputFormat: FormatError}, err
	}

	meta := aiProvider.GetMetadata()
	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Secondary AI Advice (model: %s):\n\n%s", meta.ID, response),
		OutputFormat: FormatText,
		Data:         map[string]interface{}{"provider": aiProvider.Name(), "model": meta.ID},
	}, nil
}
