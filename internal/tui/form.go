package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FieldInput wraps bubbles/textinput with validation and styling.
type FieldInput struct {
	input      textinput.Model
	Label      string
	Validator  func(string) error
	err        error
	Focused    bool
	LabelStyle lipgloss.Style
	InputStyle lipgloss.Style
	ErrorStyle lipgloss.Style
}

// NewFieldInput creates a new form field input.
func NewFieldInput(label string, styles *Styles) *FieldInput {
	input := textinput.New()
	input.CharLimit = 256
	input.Placeholder = ""

	var labelStyle, inputStyle, errorStyle lipgloss.Style
	if styles != nil {
		labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
		inputStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	}

	return &FieldInput{
		input:      input,
		Label:      label,
		Validator:  func(s string) error { return nil }, // No-op default
		LabelStyle: labelStyle,
		InputStyle: inputStyle,
		ErrorStyle: errorStyle,
	}
}

// Update handles key events and validates input.
func (f *FieldInput) Update(msg tea.Msg) (FieldInput, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyTab, tea.KeyShiftTab:
			f.Focused = !f.Focused
			if f.Focused {
				f.input.Focus()
			} else {
				f.input.Blur()
				// Validate on blur
				f.err = f.Validator(f.input.Value())
			}
		case tea.KeyEnter:
			f.err = f.Validator(f.input.Value())
		}
	}

	f.input, cmd = f.input.Update(msg)
	return *f, cmd
}

// View renders the field with label, input, and optional error.
func (f *FieldInput) View(width int) string {
	var result string

	// Label
	result += f.LabelStyle.Render(f.Label) + "\n"

	// Input
	inputView := f.input.View()
	if len(inputView) > width {
		inputView = inputView[:width]
	}
	result += f.InputStyle.Render(inputView) + "\n"

	// Error if present
	if f.err != nil {
		result += f.ErrorStyle.Render("✖ " + f.err.Error()) + "\n"
	}

	return result
}

// Valid returns true if the field is valid (no validation error).
func (f *FieldInput) Valid() bool {
	return f.err == nil
}

// Value returns the current field value.
func (f *FieldInput) Value() string {
	return f.input.Value()
}

// SetValue sets the field value programmatically.
func (f *FieldInput) SetValue(value string) {
	f.input.SetValue(value)
}

// SetFocus sets the focus state.
func (f *FieldInput) SetFocus(focused bool) {
	f.Focused = focused
	if focused {
		f.input.Focus()
	} else {
		f.input.Blur()
	}
}

// ─────────────────────────────────────────────────────────────────────────────

// Toggle is a boolean checkbox using Bubble Tea update pattern.
type Toggle struct {
	Label   string
	Value   bool
	Focused bool
	Style   lipgloss.Style
}

// NewToggle creates a new toggle field.
func NewToggle(label string, initialValue bool, styles *Styles) *Toggle {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	if styles != nil {
		// Use theme colors if available
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	}

	return &Toggle{
		Label: label,
		Value: initialValue,
		Style: style,
	}
}

// Update handles key events.
func (t *Toggle) Update(msg tea.Msg) (Toggle, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeySpace, tea.KeyEnter:
			t.Value = !t.Value
		case tea.KeyTab, tea.KeyShiftTab:
			t.Focused = !t.Focused
		}
	}
	return *t, nil
}

// View renders the toggle as a checkbox with label.
func (t *Toggle) View() string {
	checkbox := "☐"
	if t.Value {
		checkbox = "☑"
	}

	focusMarker := " "
	if t.Focused {
		focusMarker = "▸"
	}

	return t.Style.Render(focusMarker + " " + checkbox + " " + t.Label)
}

// SetValue updates the toggle value.
func (t *Toggle) SetValue(value bool) {
	t.Value = value
}

// SetFocus sets the focus state.
func (t *Toggle) SetFocus(focused bool) {
	t.Focused = focused
}
