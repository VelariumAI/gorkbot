package tui

// bookmark_overlay.go — Conversation Bookmark Manager
//
// Activated by Ctrl+B: lists saved message bookmarks, allows creation/deletion/jump.
//
// Controls:
//   Enter — jump viewport to bookmarked message
//   n     — create new bookmark at current message (prompts for name)
//   d     — delete selected bookmark
//   Esc   — close overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// renderBookmarkOverlay renders the bookmark manager modal.
func (m *Model) renderBookmarkOverlay() string {
	boxW := m.width - 8
	if boxW < 40 {
		boxW = 40
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("57")).
		Foreground(lipgloss.Color("255")).
		Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	var lines []string
	lines = append(lines, headerStyle.Render("◆ Conversation Bookmarks"))
	lines = append(lines, "")

	if m.bookmarkInputActive {
		lines = append(lines, "New bookmark name:")
		lines = append(lines, "> "+m.bookmarkInput+"█")
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Enter to save, Esc to cancel"))
	} else if len(m.bookmarks) == 0 {
		lines = append(lines, dimStyle.Render("No bookmarks yet. Press 'n' to add one."))
	} else {
		for i, bm := range m.bookmarks {
			age := time.Since(bm.CreatedAt).Round(time.Second)
			label := fmt.Sprintf("[msg #%d] %s (%s ago)", bm.MessageIndex, bm.Name, formatDuration(age))
			if i == m.discCursor { // reuse discCursor for bookmark cursor
				label = selectedStyle.Render(label)
			}
			lines = append(lines, "  "+label)
		}
	}

	lines = append(lines, "")
	lines = append(lines, strings.Repeat("─", boxW-4))
	lines = append(lines, helpStyle.Render("  Enter=jump  n=new  d=delete  ↑↓=navigate  Esc=close"))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(1, 2).
		Width(boxW).
		Render(content)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars("░"),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
}

// updateBookmarkOverlay handles key events for the bookmark overlay.
func (m *Model) updateBookmarkOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.bookmarkInputActive {
		switch msg.String() {
		case "esc":
			m.bookmarkInputActive = false
			m.bookmarkInput = ""
		case "enter":
			if m.bookmarkInput != "" {
				m.addBookmark(m.bookmarkInput)
			}
			m.bookmarkInputActive = false
			m.bookmarkInput = ""
		case "backspace":
			if len(m.bookmarkInput) > 0 {
				m.bookmarkInput = m.bookmarkInput[:len(m.bookmarkInput)-1]
			}
		default:
			if len(msg.Runes) > 0 {
				m.bookmarkInput += string(msg.Runes)
			}
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.bookmarkOverlay = false
		m.discCursor = 0
	case "up", "k":
		if m.discCursor > 0 {
			m.discCursor--
		}
	case "down", "j":
		if m.discCursor < len(m.bookmarks)-1 {
			m.discCursor++
		}
	case "enter":
		if m.discCursor < len(m.bookmarks) {
			bm := m.bookmarks[m.discCursor]
			// Jump to bookmarked message in viewport
			m.jumpToMessage(bm.MessageIndex)
			m.bookmarkOverlay = false
		}
	case "n":
		m.bookmarkInputActive = true
		m.bookmarkInput = ""
	case "d":
		if m.discCursor < len(m.bookmarks) {
			m.bookmarks = append(m.bookmarks[:m.discCursor], m.bookmarks[m.discCursor+1:]...)
			if m.discCursor > 0 {
				m.discCursor--
			}
		}
	}
	return m, nil
}

// addBookmark creates a bookmark at the current last message.
func (m *Model) addBookmark(name string) {
	idx := len(m.messages) - 1
	if idx < 0 {
		idx = 0
	}
	bm := Bookmark{
		ID:           fmt.Sprintf("bm%d", len(m.bookmarks)+1),
		Name:         name,
		MessageIndex: idx,
		CreatedAt:    time.Now(),
	}
	m.bookmarks = append(m.bookmarks, bm)
}

// jumpToMessage scrolls the viewport to show the message at the given index.
// This is a best-effort estimate since we don't track per-message byte offsets.
func (m *Model) jumpToMessage(idx int) {
	if len(m.messages) == 0 || idx < 0 {
		return
	}
	if idx >= len(m.messages) {
		idx = len(m.messages) - 1
	}
	// Estimate scroll position: assume roughly equal height per message
	total := m.viewport.TotalLineCount()
	pct := float64(idx) / float64(len(m.messages))
	target := int(float64(total) * pct)
	m.viewport.SetYOffset(target)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
