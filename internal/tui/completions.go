package tui

// completions.go — @ file autocomplete and input history search logic.

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/velariumai/gorkbot/pkg/session"
)

// atCompleteMinLen is the minimum query length before we fire a glob.
// This prevents firing two concurrent glob calls on a single-char prefix
// that could match thousands of files.
const atCompleteMinLen = 2

// fetchAtComplete returns a Cmd that globs for files matching query under cwd.
// Requires at least atCompleteMinLen chars in query; returns empty results otherwise.
func fetchAtComplete(cwd, query string) tea.Cmd {
	return func() tea.Msg {
		if len(query) < atCompleteMinLen {
			return AtCompleteResultMsg{Items: nil, Query: query}
		}

		pattern := filepath.Join(cwd, "*"+query+"*")
		matches, _ := filepath.Glob(pattern)

		// Also search one level deep.
		deepPattern := filepath.Join(cwd, "*", "*"+query+"*")
		deep, _ := filepath.Glob(deepPattern)
		matches = append(matches, deep...)

		// Strip cwd prefix for display.
		rel := make([]string, 0, len(matches))
		for _, m := range matches {
			if r, err := filepath.Rel(cwd, m); err == nil {
				rel = append(rel, r)
			} else {
				rel = append(rel, m)
			}
		}

		// Cap at 20.
		if len(rel) > 20 {
			rel = rel[:20]
		}

		return AtCompleteResultMsg{Items: rel, Query: query}
	}
}

// handleAtCompleteKey processes a keypress while @ autocomplete is active.
// Returns (handled bool, cmd tea.Cmd).
func (m *Model) handleAtCompleteKey(key string) (bool, tea.Cmd) {
	switch key {
	case "esc":
		m.atCompleteActive = false
		m.atCompleteItems = nil
		return true, nil

	case "up":
		if m.atCompleteIdx > 0 {
			m.atCompleteIdx--
		}
		return true, nil

	case "down":
		if m.atCompleteIdx < len(m.atCompleteItems)-1 {
			m.atCompleteIdx++
		}
		return true, nil

	case "enter", "tab":
		if len(m.atCompleteItems) > 0 {
			chosen := m.atCompleteItems[m.atCompleteIdx]
			cur := m.textarea.Value()
			// Replace from @ position to current cursor with chosen path.
			at := m.atCompleteAt
			if at < 0 || at >= len(cur) {
				at = 0
			}
			newVal := cur[:at] + "@" + chosen + " " + cur[len(cur):]
			// Remove any partial query after @.
			newVal = cur[:at] + "@" + chosen + " "
			m.textarea.SetValue(newVal)
		}
		m.atCompleteActive = false
		m.atCompleteItems = nil
		return true, nil

	case "backspace":
		if m.atCompleteQuery != "" {
			m.atCompleteQuery = m.atCompleteQuery[:len(m.atCompleteQuery)-1]
			return true, fetchAtComplete(m.atCompleteCWD, m.atCompleteQuery)
		}
		// Query is empty — dismiss autocomplete and fall through.
		m.atCompleteActive = false
		m.atCompleteItems = nil
		return false, nil
	}

	return false, nil
}

// handleHistSearchKey processes a keypress while history search is active.
func (m *Model) handleHistSearchKey(key string) (bool, tea.Cmd) {
	switch key {
	case "esc":
		m.histSearchMode = false
		m.histSearchQuery = ""
		m.histSearchMatches = nil
		return true, nil

	case "enter":
		if len(m.histSearchMatches) > 0 {
			idx := m.histSearchMatches[m.histSearchIdx]
			if idx >= 0 && idx < len(m.inputHistory) {
				m.textarea.SetValue(m.inputHistory[idx])
			}
		}
		m.histSearchMode = false
		m.histSearchQuery = ""
		m.histSearchMatches = nil
		return true, nil

	case "up":
		if m.histSearchIdx < len(m.histSearchMatches)-1 {
			m.histSearchIdx++
		}
		return true, nil

	case "down":
		if m.histSearchIdx > 0 {
			m.histSearchIdx--
		}
		return true, nil

	case "backspace":
		if len(m.histSearchQuery) > 0 {
			m.histSearchQuery = m.histSearchQuery[:len(m.histSearchQuery)-1]
			m.rebuildHistMatches()
		}
		return true, nil

	default:
		if len(key) == 1 {
			m.histSearchQuery += key
			m.rebuildHistMatches()
			return true, nil
		}
	}
	return false, nil
}

// rebuildHistMatches rebuilds the history search match index list.
// Uses m.inputHistoryLower (pre-lowercased on submitPrompt) to avoid
// allocating a new lowercase string for every history entry per keystroke.
func (m *Model) rebuildHistMatches() {
	q := strings.ToLower(m.histSearchQuery)
	m.histSearchMatches = nil
	for i := len(m.inputHistoryLower) - 1; i >= 0; i-- {
		if q == "" || strings.Contains(m.inputHistoryLower[i], q) {
			m.histSearchMatches = append(m.histSearchMatches, i)
		}
	}
	m.histSearchIdx = 0
}

// rewindItems is stored as []session.CheckpointSummary in model.go.
// We keep a local type alias to avoid importing session everywhere.
type rewindItem = session.CheckpointSummary
