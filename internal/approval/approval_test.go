package approval

import (
	"log/slog"
	"regexp"
	"testing"
)

func TestPatternMatcher(t *testing.T) {
	logger := slog.Default()
	pm := NewPatternMatcher(logger)

	tests := []struct {
		target   string
		category string
		expected bool
	}{
		{"src/main.go", "file_read", true},
		{"internal/config.go", "file_read", true},
		{"vendor/pkg.go", "file_read", true},
		{"config.yaml", "file_read", true},
		{"settings.json", "file_read", true},
		{"secret.env", "file_read", false},
		{"list_tools", "tool_call", true},
		{"git_status", "tool_call", true},
		{"system_info", "tool_call", true},
		{"list_directory", "tool_call", true},
	}

	for _, tt := range tests {
		matched, patternName := pm.Match(tt.target, tt.category)
		if matched != tt.expected {
			t.Errorf("Match(%s, %s) = %v, want %v (pattern: %s)",
				tt.target, tt.category, matched, tt.expected, patternName)
		}
	}
}

func TestAddPattern(t *testing.T) {
	logger := slog.Default()
	pm := NewPatternMatcher(logger)

	pattern := &ApprovalPattern{
		Name:        "custom_pattern",
		Description: "Custom test pattern",
		Pattern:     "^custom_.*",
		Category:    "tool_call",
		Priority:    15,
	}

	err := pm.AddPattern(pattern)
	if err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	matched, _ := pm.Match("custom_tool", "tool_call")
	if !matched {
		t.Error("Custom pattern should match")
	}
}

func TestGetMatchingPatterns(t *testing.T) {
	logger := slog.Default()
	pm := NewPatternMatcher(logger)

	patterns := pm.GetMatchingPatterns("src/main.go", "file_read")
	if len(patterns) == 0 {
		t.Error("Should find matching patterns")
	}

	if patterns[0].Name != "read_src_files" {
		t.Errorf("Expected read_src_files, got %s", patterns[0].Name)
	}
}

func TestClearCache(t *testing.T) {
	logger := slog.Default()
	pm := NewPatternMatcher(logger)

	pm.Match("src/main.go", "file_read")
	if len(pm.approved) == 0 {
		t.Error("Cache should have entries")
	}

	pm.ClearCache()
	if len(pm.approved) != 0 {
		t.Error("Cache should be cleared")
	}
}

func TestGetStats(t *testing.T) {
	logger := slog.Default()
	pm := NewPatternMatcher(logger)

	pm.Match("src/main.go", "file_read")
	pm.Match("secret.env", "file_read")

	stats := pm.GetStats()

	if stats["approved"].(int) != 1 {
		t.Errorf("Expected 1 approved, got %d", stats["approved"].(int))
	}

	if stats["denied"].(int) != 1 {
		t.Errorf("Expected 1 denied, got %d", stats["denied"].(int))
	}
}

func TestIsSafeDirectory(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"src/main.go", true},
		{"internal/config.go", true},
		{"pkg/tools.go", true},
		{"test/test.go", true},
		{"vendor/dep.go", true},
		{"random/file.go", false},
		{"../../etc/passwd", false},
	}

	for _, tt := range tests {
		result := IsSafeDirectory(tt.path)
		if result != tt.expected {
			t.Errorf("IsSafeDirectory(%s) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestMatchMulti(t *testing.T) {
	logger := slog.Default()
	pm := NewPatternMatcher(logger)

	targets := map[string]string{
		"src/main.go": "file_read",
		"list_tools":  "tool_call",
	}

	if !pm.MatchMulti(targets) {
		t.Error("MatchMulti should approve all safe operations")
	}

	targets["secret.env"] = "file_read"
	if pm.MatchMulti(targets) {
		t.Error("MatchMulti should deny if any operation is unsafe")
	}
}

func TestGlobToRegex(t *testing.T) {
	tests := []struct {
		glob      string
		testStr   string
		expected  bool
	}{
		{"*.go", "main.go", true},
		{"src/*.go", "src/main.go", true},
		{"src/*.go", "src/subdir/main.go", false},
		{"*/main.go", "src/main.go", true},
		{"**", "anything", true},
	}

	for _, tt := range tests {
		regex := globToRegex(tt.glob)
		r, err := regexp.Compile(regex)
		if err != nil {
			t.Errorf("Failed to compile regex: %v", err)
			continue
		}
		matched := r.MatchString(tt.testStr)
		if matched != tt.expected {
			t.Errorf("globToRegex(%s) matching %s = %v, want %v",
				tt.glob, tt.testStr, matched, tt.expected)
		}
	}
}

func TestApprovalRate(t *testing.T) {
	logger := slog.Default()
	pm := NewPatternMatcher(logger)

	// Simulate operations
	operations := []struct {
		target   string
		category string
	}{
		{"src/main.go", "file_read"},
		{"internal/pkg.go", "file_read"},
		{"config.yaml", "file_read"},
		{"list_tools", "tool_call"},
		{"system_info", "tool_call"},
		{"git_status", "tool_call"},
	}

	for _, op := range operations {
		pm.Match(op.target, op.category)
	}

	stats := pm.GetStats()
	approved := stats["approved"].(int)

	if approved < 5 {
		t.Errorf("Expected at least 5 approved operations, got %d", approved)
	}
}
