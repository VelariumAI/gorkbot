package tools

import (
	"testing"
)

// TestToolNormalization tests parameter normalization
func TestToolNormalization(t *testing.T) {
	tests := []struct {
		name       string
		toolName   string
		params     map[string]interface{}
		expected   map[string]interface{}
	}{
		{
			name:     "query to pattern",
			toolName: "grep_content",
			params:   map[string]interface{}{"query": "test"},
			expected: map[string]interface{}{"pattern": "test"},
		},
		{
			name:     "cmd to command",
			toolName: "bash",
			params:   map[string]interface{}{"cmd": "ls"},
			expected: map[string]interface{}{"command": "ls"},
		},
		{
			name:     "file to path",
			toolName: "read_file",
			params:   map[string]interface{}{"file": "/test/path"},
			expected: map[string]interface{}{"path": "/test/path"},
		},
		{
			name:     "no change for correct param",
			toolName: "bash",
			params:   map[string]interface{}{"command": "ls"},
			expected: map[string]interface{}{"command": "ls"},
		},
		{
			name:     "nil params returns nil",
			toolName: "bash",
			params:   nil,
			expected: nil,
		},
		{
			name:     "empty params returns empty",
			toolName: "bash",
			params:   map[string]interface{}{},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := NormalizeToolParams(tt.toolName, tt.params)
			if tt.expected == nil {
				if normalized != nil {
					t.Errorf("Expected nil, got %v", normalized)
				}
				return
			}
			for key, expectedVal := range tt.expected {
				if normalized[key] != expectedVal {
					t.Errorf("Expected %s=%v, got %v", key, expectedVal, normalized[key])
				}
			}
		})
	}
}

// TestToolResult tests ToolResult structure
func TestToolResult(t *testing.T) {
	result := &ToolResult{
		Success: true,
		Output:  "test output",
	}

	if !result.Success {
		t.Error("Expected success to be true")
	}

	if result.Output != "test output" {
		t.Errorf("Expected output 'test output', got '%s'", result.Output)
	}
}
