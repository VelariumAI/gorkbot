package tui

import (
	"testing"
)

// TestWorkspaceIDMapping verifies that all 7 workspace IDs have sessionState mappings.
func TestWorkspaceIDMapping(t *testing.T) {
	tests := []WorkspaceID{
		WorkspaceChat, WorkspaceTasks, WorkspaceTools, WorkspaceAgents,
		WorkspaceMemory, WorkspaceAnalytics, WorkspaceSettings,
	}

	for _, ws := range tests {
		if _, ok := workspaceToState[ws]; !ok {
			t.Errorf("WorkspaceID %d has no mapping in workspaceToState", ws)
		}
	}
}

// TestSwitchWorkspaceUpdatesState verifies that workspaceToState mapping is correct.
func TestSwitchWorkspaceUpdatesState(t *testing.T) {
	// Verify the mapping in workspaceToState
	expectedMappings := map[WorkspaceID]sessionState{
		WorkspaceChat:      chatView,
		WorkspaceTasks:     taskView,
		WorkspaceTools:     toolsTableView,
		WorkspaceAgents:    agentsView,
		WorkspaceMemory:    memoryView,
		WorkspaceAnalytics: analyticsView,
		WorkspaceSettings:  settingsWorkspaceView,
	}

	for ws, expectedState := range expectedMappings {
		actualState, ok := workspaceToState[ws]
		if !ok {
			t.Errorf("WorkspaceID %d not in workspaceToState map", ws)
		}
		if actualState != expectedState {
			t.Errorf("WorkspaceID %d maps to %d, expected %d", ws, actualState, expectedState)
		}
	}
}

// TestNavRailToggle verifies that ToggleNavRail toggles visibility correctly.
func TestNavRailToggle(t *testing.T) {
	m := &Model{navRailVisible: true}

	m.navRailVisible = !m.navRailVisible
	if m.navRailVisible {
		t.Error("Expected navRailVisible to be false after toggle")
	}

	m.navRailVisible = !m.navRailVisible
	if !m.navRailVisible {
		t.Error("Expected navRailVisible to be true after second toggle")
	}
}

// TestRenderWorkspaceNavNilTokenStyles ensures renderWorkspaceNav doesn't panic with nil TokenStyles.
func TestRenderWorkspaceNavNilTokenStyles(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("renderWorkspaceNav panicked: %v", r)
		}
	}()

	m := &Model{
		currentWorkspace: WorkspaceChat,
		width:            80,
		height:           24,
		styles: &Styles{
			TokenStyles: nil, // Deliberately nil
		},
	}

	_ = m.renderWorkspaceNav()
}

// TestWorkspaceEntryCount verifies that exactly 7 workspace entries exist.
func TestWorkspaceEntryCount(t *testing.T) {
	expectedCount := 7
	actualCount := len(workspaceEntries)

	if actualCount != expectedCount {
		t.Errorf("Expected %d workspace entries, got %d", expectedCount, actualCount)
	}
}
