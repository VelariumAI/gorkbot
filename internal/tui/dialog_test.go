package tui

import (
	"strings"
	"testing"
)

func TestDialogModel_WrapAddsTitle(t *testing.T) {
	cfg := DialogConfig{
		Title:    "Test Dialog",
		MinWidth: 40,
	}
	d := NewDialogModel(cfg, nil)

	content := d.Wrap("Hello world", 80)

	if !strings.Contains(content, "Test Dialog") {
		t.Error("Wrap should include title in output")
	}

	if !strings.Contains(content, "Hello world") {
		t.Error("Wrap should include content in output")
	}

	// Check for borders
	if !strings.Contains(content, "┌") || !strings.Contains(content, "└") {
		t.Error("Wrap should include top and bottom borders")
	}
}

func TestDialogModel_CenterRespectsWidth(t *testing.T) {
	cfg := DialogConfig{
		Title: "Centered",
		Width: 50,
	}
	d := NewDialogModel(cfg, nil)

	content := d.Center("Test content", 120, 30)

	// Should contain the dialog
	if !strings.Contains(content, "Centered") {
		t.Error("Center should include dialog content")
	}

	// Should have newlines for vertical centering
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		t.Error("Center should add vertical padding")
	}
}

func TestDialogModel_EmptyContentOK(t *testing.T) {
	cfg := DialogConfig{
		Title: "Empty Dialog",
	}
	d := NewDialogModel(cfg, nil)

	// Should not panic
	content := d.Wrap("", 60)

	if !strings.Contains(content, "Empty Dialog") {
		t.Error("Dialog should render even with empty content")
	}
}

func TestDialogModel_WithFooter(t *testing.T) {
	cfg := DialogConfig{
		Title:  "Dialog",
		Footer: "Press Enter to confirm",
	}
	d := NewDialogModel(cfg, nil)

	content := d.Wrap("Content", 60)

	if !strings.Contains(content, "Press Enter to confirm") {
		t.Error("Dialog should include footer text")
	}
}

func TestRepeatStr(t *testing.T) {
	result := repeatStr("x", 5)
	if result != "xxxxx" {
		t.Errorf("repeatStr failed: got %q, want %q", result, "xxxxx")
	}

	result = repeatStr("x", 0)
	if result != "" {
		t.Errorf("repeatStr(0) should return empty string")
	}

	result = repeatStr("ab", 3)
	if result != "ababab" {
		t.Errorf("repeatStr with multi-char string: got %q", result)
	}
}

func TestPadStr(t *testing.T) {
	result := padStr("hello", 10)
	if len(result) != 10 {
		t.Errorf("padStr should pad to width; got len=%d, want 10", len(result))
	}

	if !strings.HasPrefix(result, "hello") {
		t.Errorf("padStr should preserve original string")
	}

	// Truncate if longer
	result = padStr("hello world", 5)
	if len(result) != 5 {
		t.Errorf("padStr should truncate if too long")
	}
}

func TestSplitLines(t *testing.T) {
	result := splitLines("line1\nline2\nline3")
	if len(result) != 3 {
		t.Errorf("splitLines: expected 3 lines, got %d", len(result))
	}

	if result[0] != "line1" || result[1] != "line2" {
		t.Errorf("splitLines failed: %v", result)
	}

	// Empty string
	result = splitLines("")
	if len(result) != 1 || result[0] != "" {
		t.Error("splitLines on empty string should return one empty line")
	}
}
