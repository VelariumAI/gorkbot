package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFuzzyMatch_Subsequence(t *testing.T) {
	tests := []struct {
		query    string
		label    string
		shouldMatch bool
	}{
		{"test", "test", true},
		{"tst", "test", true},
		{"abc", "aabbcc", true},
		{"xyz", "abc", false},
		{"", "anything", true}, // Empty query matches everything
		{"hello", "hxexlxlo", true},
		{"git", "git_status", true},
		{"status", "git_status", true},
	}

	for _, tt := range tests {
		_, matched := fuzzyMatch(tt.query, tt.label)
		if matched != tt.shouldMatch {
			t.Errorf("fuzzyMatch(%q, %q): got %v, want %v", tt.query, tt.label, matched, tt.shouldMatch)
		}
	}
}

func TestFuzzyMatch_Scoring(t *testing.T) {
	// Test that better matches get higher scores
	score1, _ := fuzzyMatch("test", "test")       // Exact match
	score2, _ := fuzzyMatch("test", "t_e_s_t")   // Spread out match

	if score1 <= score2 {
		t.Errorf("Exact match should score higher than spread match")
	}

	// Verify exact match has positive score
	if score1 <= 0 {
		t.Errorf("Exact match should have positive score, got %d", score1)
	}
}

func TestCmdPalette_FilterUpdatesOnInput(t *testing.T) {
	cp := NewCmdPalette(nil)

	// Add some items
	cp.AddItem("git_status", "Show git status", func() tea.Cmd { return nil })
	cp.AddItem("git_commit", "Commit changes", func() tea.Cmd { return nil })
	cp.AddItem("help", "Show help", func() tea.Cmd { return nil })

	// Set input
	cp.input.SetValue("git")
	cp.refilter()

	if len(cp.filtered) != 2 {
		t.Errorf("Filtering 'git' should return 2 items, got %d", len(cp.filtered))
	}

	// Filter to exact match
	cp.input.SetValue("help")
	cp.refilter()

	if len(cp.filtered) != 1 {
		t.Errorf("Filtering 'help' should return 1 item, got %d", len(cp.filtered))
	}
}

func TestCmdPalette_EmptyFilter(t *testing.T) {
	cp := NewCmdPalette(nil)
	cp.AddItem("test1", "", func() tea.Cmd { return nil })
	cp.AddItem("test2", "", func() tea.Cmd { return nil })

	cp.input.SetValue("")
	cp.refilter()

	// Empty input should match all items
	if len(cp.filtered) != 2 {
		t.Errorf("Empty filter should match all items, got %d", len(cp.filtered))
	}
}

func TestCmdPalette_Navigation(t *testing.T) {
	cp := NewCmdPalette(nil)
	cp.SetActive(true)

	cp.AddItem("first", "", func() tea.Cmd { return nil })
	cp.AddItem("second", "", func() tea.Cmd { return nil })
	cp.AddItem("third", "", func() tea.Cmd { return nil })

	cp.refilter()

	// Initial cursor at 0
	if cp.cursor != 0 {
		t.Errorf("Initial cursor should be 0, got %d", cp.cursor)
	}

	// Simulate down key
	cp.Update(tea.KeyMsg{Type: tea.KeyDown})

	if cp.cursor != 1 {
		t.Errorf("Down key should increment cursor to 1, got %d", cp.cursor)
	}

	// Simulate up key
	cp.Update(tea.KeyMsg{Type: tea.KeyUp})

	if cp.cursor != 0 {
		t.Errorf("Up key should decrement cursor to 0, got %d", cp.cursor)
	}
}

func TestCmdPalette_WrapAround(t *testing.T) {
	cp := NewCmdPalette(nil)
	cp.SetActive(true)

	cp.AddItem("item1", "", func() tea.Cmd { return nil })
	cp.AddItem("item2", "", func() tea.Cmd { return nil })

	cp.refilter()

	// Go to last item
	cp.cursor = 1

	// Down should wrap to 0
	cp.Update(tea.KeyMsg{Type: tea.KeyDown})

	if cp.cursor != 0 {
		t.Errorf("Down at end should wrap to 0, got %d", cp.cursor)
	}

	// Up from 0 should go to last
	cp.Update(tea.KeyMsg{Type: tea.KeyUp})

	if cp.cursor != 1 {
		t.Errorf("Up at start should wrap to end, got %d", cp.cursor)
	}
}

func TestCmdPalette_Escape(t *testing.T) {
	cp := NewCmdPalette(nil)
	cp.SetActive(true)

	if !cp.IsActive() {
		t.Error("SetActive(true) should make palette active")
	}

	// Simulate Esc key
	cp.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cp.IsActive() {
		t.Error("Esc should deactivate palette")
	}
}

func TestCmdPalette_EnterExecutesAction(t *testing.T) {
	cp := NewCmdPalette(nil)
	cp.SetActive(true)

	actionCalled := false
	cp.AddItem("test", "", func() tea.Cmd {
		actionCalled = true
		return nil
	})

	cp.refilter()
	cp.cursor = 0

	// Simulate Enter key
	cp.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !actionCalled {
		t.Error("Enter on selected item should call action")
	}

	if cp.IsActive() {
		t.Error("After action, palette should be deactivated")
	}
}

func TestCmdPalette_View(t *testing.T) {
	cp := NewCmdPalette(nil)

	// Inactive palette should render nothing
	view := cp.View(80)
	if view != "" {
		t.Error("Inactive palette should render empty string")
	}

	// Activate and add items
	cp.SetActive(true)
	cp.AddItem("help", "Show help", func() tea.Cmd { return nil })

	view = cp.View(80)

	// View should show search prompt or "Start typing" message
	if view == "" {
		t.Error("Active palette should render something")
	}

	if !strings.Contains(view, "help") && !strings.Contains(view, "Start typing") {
		t.Error("View should contain item or prompt")
	}
}

func TestCmdPalette_SelectedItem(t *testing.T) {
	cp := NewCmdPalette(nil)
	cp.AddItem("item1", "Desc 1", func() tea.Cmd { return nil })
	cp.AddItem("item2", "Desc 2", func() tea.Cmd { return nil })

	cp.refilter()
	cp.cursor = 0

	selected := cp.SelectedItem()

	if selected == nil {
		t.Error("SelectedItem should not return nil")
	}

	if selected.Label != "item1" {
		t.Errorf("SelectedItem should return item at cursor, got %q", selected.Label)
	}
}

func TestCmdPalette_TabNavigation(t *testing.T) {
	cp := NewCmdPalette(nil)
	cp.SetActive(true)

	cp.AddItem("a", "", func() tea.Cmd { return nil })
	cp.AddItem("b", "", func() tea.Cmd { return nil })
	cp.AddItem("c", "", func() tea.Cmd { return nil })

	cp.refilter()

	// Tab should move forward
	cp.Update(tea.KeyMsg{Type: tea.KeyTab})

	if cp.cursor != 1 {
		t.Errorf("Tab should move to next item, got cursor=%d", cp.cursor)
	}

	// Shift+Tab should move backward
	cp.Update(tea.KeyMsg{Type: tea.KeyShiftTab})

	if cp.cursor != 0 {
		t.Errorf("Shift+Tab should move to previous item, got cursor=%d", cp.cursor)
	}
}
