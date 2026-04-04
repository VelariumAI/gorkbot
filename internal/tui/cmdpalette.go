package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// paletteItem represents a single command palette item.
type paletteItem struct {
	Label  string
	Desc   string
	Action func() tea.Cmd
	Score  int // fuzzy match score
}

// CmdPaletteModel is a fuzzy-search command palette (Ctrl+K style).
type CmdPaletteModel struct {
	input      textinput.Model
	items      []paletteItem
	filtered   []paletteItem
	cursor     int
	active     bool
	styles     *Styles
	MaxVisible int // Maximum items to display
}

// NewCmdPalette creates a new command palette.
func NewCmdPalette(styles *Styles) *CmdPaletteModel {
	input := textinput.New()
	input.Placeholder = "Search commands..."
	input.CharLimit = 128

	if styles == nil {
		styles = &Styles{}
	}

	return &CmdPaletteModel{
		input:       input,
		items:       []paletteItem{},
		filtered:    []paletteItem{},
		cursor:      0,
		active:      false,
		styles:      styles,
		MaxVisible:  10,
	}
}

// AddItem adds a command to the palette.
func (cp *CmdPaletteModel) AddItem(label, desc string, action func() tea.Cmd) {
	cp.items = append(cp.items, paletteItem{
		Label:  label,
		Desc:   desc,
		Action: action,
		Score:  0,
	})
}

// SetActive toggles the palette visibility.
func (cp *CmdPaletteModel) SetActive(active bool) {
	cp.active = active
	if active {
		cp.input.Focus()
		cp.input.SetValue("")
		cp.cursor = 0
		cp.refilter()
	} else {
		cp.input.Blur()
		cp.filtered = []paletteItem{}
	}
}

// IsActive returns true if the palette is currently displayed.
func (cp *CmdPaletteModel) IsActive() bool {
	return cp.active
}

// Update handles input and navigation.
func (cp *CmdPaletteModel) Update(msg tea.Msg) (CmdPaletteModel, tea.Cmd) {
	if !cp.active {
		return *cp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			cp.SetActive(false)
			return *cp, nil

		case tea.KeyEnter:
			if len(cp.filtered) > 0 && cp.cursor < len(cp.filtered) {
				action := cp.filtered[cp.cursor].Action
				cp.SetActive(false)
				if action != nil {
					return *cp, action()
				}
			}
			return *cp, nil

		case tea.KeyUp:
			if cp.cursor > 0 {
				cp.cursor--
			} else if len(cp.filtered) > 0 {
				cp.cursor = len(cp.filtered) - 1
			}
			return *cp, nil

		case tea.KeyDown:
			if cp.cursor < len(cp.filtered)-1 {
				cp.cursor++
			} else {
				cp.cursor = 0
			}
			return *cp, nil

		case tea.KeyTab:
			// Tab → next item
			if cp.cursor < len(cp.filtered)-1 {
				cp.cursor++
			} else {
				cp.cursor = 0
			}
			return *cp, nil

		case tea.KeyShiftTab:
			// Shift+Tab → previous item
			if cp.cursor > 0 {
				cp.cursor--
			} else if len(cp.filtered) > 0 {
				cp.cursor = len(cp.filtered) - 1
			}
			return *cp, nil
		}
	}

	var cmd tea.Cmd
	cp.input, cmd = cp.input.Update(msg)

	// Re-filter on input change
	cp.refilter()

	return *cp, cmd
}

// View renders the command palette.
func (cp *CmdPaletteModel) View(termW int) string {
	if !cp.active {
		return ""
	}

	width := termW - 4
	if width < 40 {
		width = 40
	}

	var result string

	// Search box
	result += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("▸") + " "
	inputView := cp.input.View()
	if len(inputView) > width-4 {
		inputView = inputView[:width-4]
	}
	result += inputView + "\n\n"

	// Results
	for i, item := range cp.filtered {
		if i >= cp.MaxVisible {
			result += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("... and more") + "\n"
			break
		}

		marker := "  "
		if i == cp.cursor {
			marker = "▸ "
		}

		line := marker + item.Label
		if item.Desc != "" {
			line += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("("+item.Desc+")")
		}

		if i == cp.cursor {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(line)
		}

		result += line + "\n"
	}

	// Empty state
	if len(cp.filtered) == 0 && cp.input.Value() != "" {
		result += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("No results") + "\n"
	} else if len(cp.filtered) == 0 {
		result += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Start typing to search") + "\n"
	}

	return result
}

// refilter updates filtered items based on current input.
func (cp *CmdPaletteModel) refilter() {
	query := strings.ToLower(cp.input.Value())
	cp.filtered = []paletteItem{}

	for _, item := range cp.items {
		score, ok := fuzzyMatch(query, strings.ToLower(item.Label))
		if ok {
			item.Score = score
			cp.filtered = append(cp.filtered, item)
		}
	}

	// Reset cursor
	if cp.cursor >= len(cp.filtered) {
		cp.cursor = 0
	}
}

// fuzzyMatch scores a query against a label using subsequence matching.
// Returns (score, matched). Higher score = better match.
func fuzzyMatch(query, label string) (int, bool) {
	if query == "" {
		return 0, true // Empty query matches everything with score 0
	}

	// Check if all characters of query appear in label in order
	qi, li := 0, 0
	score := 0
	consecutive := 0

	for qi < len(query) && li < len(label) {
		if query[qi] == label[li] {
			qi++
			consecutive++
			// Bonus for consecutive matches
			score += 10 + consecutive
		} else {
			consecutive = 0
			score -= 1 // Small penalty for gap
		}
		li++
	}

	if qi == len(query) {
		// All characters matched; earlier matches are better
		return score - li, true
	}

	return 0, false // Query not found
}

// SelectedItem returns the currently selected item, or nil if none selected.
func (cp *CmdPaletteModel) SelectedItem() *paletteItem {
	if cp.cursor >= 0 && cp.cursor < len(cp.filtered) {
		return &cp.filtered[cp.cursor]
	}
	return nil
}
