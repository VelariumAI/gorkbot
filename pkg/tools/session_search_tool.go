package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/persist"
)

// SessionSearchTool searches past conversation sessions using FTS5 (or LIKE fallback).
type SessionSearchTool struct {
	BaseTool
	store *persist.Store
}

// NewSessionSearchTool creates a session_search tool backed by the given Store.
// If store is nil the tool still registers successfully but returns an error on execution.
func NewSessionSearchTool(store *persist.Store) Tool {
	return &SessionSearchTool{
		BaseTool: NewBaseTool(
			"session_search",
			"Search past conversation sessions by keyword. Returns session summaries with relevant snippets.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
		store: store,
	}
}

func (t *SessionSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Search keywords or phrase to look for in past conversations."
			},
			"days": {
				"type": "integer",
				"description": "Limit search to conversations within the last N days. 0 means no time limit.",
				"default": 0
			},
			"top_k": {
				"type": "integer",
				"description": "Maximum number of sessions to return. Defaults to 5.",
				"default": 5
			}
		},
		"required": ["query"]
	}`)
}

func (t *SessionSearchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	if t.store == nil {
		return &ToolResult{
			Success: false,
			Error:   "session_search: persistence store not available",
		}, nil
	}

	query, _ := params["query"].(string)
	if strings.TrimSpace(query) == "" {
		return &ToolResult{
			Success: false,
			Error:   "session_search: 'query' parameter is required and must not be empty",
		}, nil
	}

	days := 0
	if v, ok := params["days"]; ok {
		switch n := v.(type) {
		case float64:
			days = int(n)
		case int:
			days = n
		case int64:
			days = int(n)
		}
	}

	topK := 5
	if v, ok := params["top_k"]; ok {
		switch n := v.(type) {
		case float64:
			topK = int(n)
		case int:
			topK = n
		case int64:
			topK = int(n)
		}
	}
	if topK <= 0 {
		topK = 5
	}

	results, err := t.store.SearchSessions(ctx, query, days, topK)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("session_search: query failed: %v", err),
		}, nil
	}

	if len(results) == 0 {
		return &ToolResult{
			Success:      true,
			Output:       fmt.Sprintf("No sessions found matching '%s'", query),
			OutputFormat: FormatText,
		}, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Session Search: \"%s\"\n\n", query)

	for i, r := range results {
		// Use first 8 chars of session ID as short label if no title.
		label := r.Title
		if label == "" {
			if len(r.SessionID) >= 8 {
				label = r.SessionID[:8]
			} else {
				label = r.SessionID
			}
		}

		dateStr := ""
		if !r.StartedAt.IsZero() {
			dateStr = r.StartedAt.Format("2006-01-02 15:04")
		}

		fmt.Fprintf(&sb, "### Session %d: %s\n", i+1, label)
		if dateStr != "" {
			fmt.Fprintf(&sb, "Date: %s\n", dateStr)
		}
		fmt.Fprintf(&sb, "Score: %.4f\n", r.Score)
		if r.Snippet != "" {
			fmt.Fprintf(&sb, "Snippet: %s\n", r.Snippet)
		}
		if r.MessageCount > 0 {
			fmt.Fprintf(&sb, "Messages: %d\n", r.MessageCount)
		}
		fmt.Fprintln(&sb)
	}

	return &ToolResult{
		Success:      true,
		Output:       sb.String(),
		OutputFormat: FormatText,
	}, nil
}

// RegisterSessionSearchTool registers the session_search tool with the registry.
// It is safe to call with a nil store — the tool will register but return an
// error result when executed without persistence.
func RegisterSessionSearchTool(reg *Registry, store *persist.Store) {
	reg.RegisterOrReplace(NewSessionSearchTool(store))
}
