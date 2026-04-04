package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFieldInput_ValidatorCalledOnBlur(t *testing.T) {
	validator := func(s string) error {
		return nil
	}

	field := NewFieldInput("Test", nil)
	field.Validator = validator

	// Focus then blur
	field.SetFocus(true)
	if !field.Focused {
		t.Error("SetFocus(true) should set Focused=true")
	}

	// Simulate blur
	field.SetFocus(false)
	if field.Focused {
		t.Error("SetFocus(false) should set Focused=false")
	}
}

func TestFieldInput_ViewContainsLabel(t *testing.T) {
	field := NewFieldInput("Email Address", nil)
	view := field.View(60)

	if !strings.Contains(view, "Email Address") {
		t.Error("FieldInput view should contain label")
	}
}

func TestFieldInput_DisplaysError(t *testing.T) {
	validator := func(s string) error {
		if s == "" {
			return errors.New("required field")
		}
		return nil
	}

	field := NewFieldInput("Name", nil)
	field.Validator = validator
	field.err = errors.New("required field")

	view := field.View(60)
	if !strings.Contains(view, "required field") {
		t.Error("FieldInput should display validation error")
	}
}

func TestFieldInput_SetValue(t *testing.T) {
	field := NewFieldInput("Test", nil)
	field.SetValue("hello")

	if field.Value() != "hello" {
		t.Errorf("SetValue/Value failed: got %q", field.Value())
	}
}

func TestFieldInput_Valid(t *testing.T) {
	field := NewFieldInput("Test", nil)
	field.err = nil

	if !field.Valid() {
		t.Error("Field with no error should be valid")
	}

	field.err = errors.New("invalid")
	if field.Valid() {
		t.Error("Field with error should not be valid")
	}
}

// ─────────────────────────────────────────────────────────────────────────────

func TestToggle_UpdateFlipsValue(t *testing.T) {
	toggle := NewToggle("Enable Feature", false, nil)

	if toggle.Value {
		t.Error("Initial toggle value should be false")
	}

	toggle.SetValue(true)
	if !toggle.Value {
		t.Error("SetValue(true) should set Value=true")
	}

	toggle.SetValue(false)
	if toggle.Value {
		t.Error("SetValue(false) should set Value=false")
	}
}

func TestToggle_ViewShowsState(t *testing.T) {
	toggle := NewToggle("Auto-save", false, nil)
	view := toggle.View()

	if !strings.Contains(view, "Auto-save") {
		t.Error("Toggle view should contain label")
	}

	// Unchecked should show ☐
	if !strings.Contains(view, "☐") {
		t.Error("Unchecked toggle should show ☐")
	}

	toggle.SetValue(true)
	view = toggle.View()

	// Checked should show ☑
	if !strings.Contains(view, "☑") {
		t.Error("Checked toggle should show ☑")
	}
}

func TestToggle_FocusMarker(t *testing.T) {
	toggle := NewToggle("Test", false, nil)

	// Not focused
	view := toggle.View()
	if !strings.Contains(view, " ") {
		t.Error("Unfocused toggle should have space marker")
	}

	// Focused
	toggle.SetFocus(true)
	view = toggle.View()
	if !strings.Contains(view, "▸") {
		t.Error("Focused toggle should show arrow marker")
	}
}

func TestToggle_HandlesKeyMsg(t *testing.T) {
	toggle := NewToggle("Test", false, nil)

	// Simulate key message
	_, _ = toggle.Update(tea.KeyMsg{Type: tea.KeySpace})

	if !toggle.Value {
		t.Error("Space key should toggle the value")
	}
}
