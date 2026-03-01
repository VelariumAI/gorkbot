// Package theme provides a JSON-based theme system for Gorkbot.
//
// Themes control the colors used in the TUI: borders, text, status bar, code blocks,
// and assistant box borders. Themes are loaded from ~/.config/gorkbot/themes/*.json.
// The active theme name is persisted in ~/.config/gorkbot/active_theme.
//
// Built-in themes: "dracula" (default), "nord", "gruvbox", "solarized", "monokai"
package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Colors holds all color definitions for a theme.
// All values are hex color strings (#RRGGBB or #RGB).
type Colors struct {
	// Primary accent — used for primary AI name/border
	Primary string `json:"primary"`
	// Secondary accent — used for consultant AI name/border
	Secondary string `json:"secondary"`
	// Background — main terminal background (usually transparent/"")
	Background string `json:"background,omitempty"`
	// Surface — panels, boxes
	Surface string `json:"surface"`
	// Border — default border color
	Border string `json:"border"`
	// Text — primary text color
	Text string `json:"text"`
	// TextDim — dimmed/secondary text
	TextDim string `json:"text_dim"`
	// Success — confirmation/success messages
	Success string `json:"success"`
	// Warning — caution messages
	Warning string `json:"warning"`
	// Error — error messages
	Error string `json:"error"`
	// CodeBg — code block background
	CodeBg string `json:"code_bg"`
	// CodeFg — code block text
	CodeFg string `json:"code_fg"`
	// InlineCodeBg — inline code background
	InlineCodeBg string `json:"inline_code_bg"`
	// InlineCodeFg — inline code text
	InlineCodeFg string `json:"inline_code_fg"`
	// Header1 — h1 heading color
	Header1 string `json:"header1"`
	// Header2 — h2 heading color
	Header2 string `json:"header2"`
	// Link — hyperlink color
	Link string `json:"link"`
	// StatusBg — status bar background
	StatusBg string `json:"status_bg"`
	// StatusFg — status bar text
	StatusFg string `json:"status_fg"`
}

// Theme is a named color scheme.
type Theme struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Author      string `json:"author,omitempty"`
	Colors      Colors `json:"colors"`
}

// ── Built-in themes ──────────────────────────────────────────────────────────

var builtins = map[string]*Theme{
	"dracula": {
		Name:        "dracula",
		Description: "Dracula color scheme — dark background with vivid accents",
		Colors: Colors{
			Primary:      "#BD93F9",
			Secondary:    "#FF79C6",
			Surface:      "#282A36",
			Border:       "#6272A4",
			Text:         "#F8F8F2",
			TextDim:      "#6272A4",
			Success:      "#50FA7B",
			Warning:      "#FFB86C",
			Error:        "#FF5555",
			CodeBg:       "#1E0A0A",
			CodeFg:       "#E6E6E6",
			InlineCodeBg: "#2D1212",
			InlineCodeFg: "#FF8080",
			Header1:      "#FF5555",
			Header2:      "#BD93F9",
			Link:         "#8BE9FD",
			StatusBg:     "#282A36",
			StatusFg:     "#F8F8F2",
		},
	},
	"nord": {
		Name:        "nord",
		Description: "Nord — arctic, north-bluish color palette",
		Colors: Colors{
			Primary:      "#81A1C1",
			Secondary:    "#B48EAD",
			Surface:      "#2E3440",
			Border:       "#4C566A",
			Text:         "#ECEFF4",
			TextDim:      "#4C566A",
			Success:      "#A3BE8C",
			Warning:      "#EBCB8B",
			Error:        "#BF616A",
			CodeBg:       "#2E3440",
			CodeFg:       "#D8DEE9",
			InlineCodeBg: "#3B4252",
			InlineCodeFg: "#88C0D0",
			Header1:      "#88C0D0",
			Header2:      "#81A1C1",
			Link:         "#8FBCBB",
			StatusBg:     "#3B4252",
			StatusFg:     "#ECEFF4",
		},
	},
	"gruvbox": {
		Name:        "gruvbox",
		Description: "Gruvbox dark — retro groove color scheme",
		Colors: Colors{
			Primary:      "#83A598",
			Secondary:    "#D3869B",
			Surface:      "#282828",
			Border:       "#504945",
			Text:         "#EBDBB2",
			TextDim:      "#928374",
			Success:      "#B8BB26",
			Warning:      "#FABD2F",
			Error:        "#FB4934",
			CodeBg:       "#1D2021",
			CodeFg:       "#EBDBB2",
			InlineCodeBg: "#3C3836",
			InlineCodeFg: "#FE8019",
			Header1:      "#FB4934",
			Header2:      "#FABD2F",
			Link:         "#83A598",
			StatusBg:     "#3C3836",
			StatusFg:     "#EBDBB2",
		},
	},
	"solarized": {
		Name:        "solarized",
		Description: "Solarized dark — precision colors for machines and people",
		Colors: Colors{
			Primary:      "#268BD2",
			Secondary:    "#D33682",
			Surface:      "#002B36",
			Border:       "#073642",
			Text:         "#839496",
			TextDim:      "#657B83",
			Success:      "#859900",
			Warning:      "#CB4B16",
			Error:        "#DC322F",
			CodeBg:       "#073642",
			CodeFg:       "#839496",
			InlineCodeBg: "#073642",
			InlineCodeFg: "#2AA198",
			Header1:      "#DC322F",
			Header2:      "#268BD2",
			Link:         "#2AA198",
			StatusBg:     "#073642",
			StatusFg:     "#839496",
		},
	},
	"monokai": {
		Name:        "monokai",
		Description: "Monokai — classic syntax highlighting palette",
		Colors: Colors{
			Primary:      "#66D9E8",
			Secondary:    "#AE81FF",
			Surface:      "#272822",
			Border:       "#49483E",
			Text:         "#F8F8F2",
			TextDim:      "#75715E",
			Success:      "#A6E22E",
			Warning:      "#E6DB74",
			Error:        "#F92672",
			CodeBg:       "#1E1F1C",
			CodeFg:       "#F8F8F2",
			InlineCodeBg: "#3E3D32",
			InlineCodeFg: "#E6DB74",
			Header1:      "#F92672",
			Header2:      "#66D9E8",
			Link:         "#66D9E8",
			StatusBg:     "#3E3D32",
			StatusFg:     "#F8F8F2",
		},
	},
}

// ── Manager ──────────────────────────────────────────────────────────────────

// Manager loads, stores, and activates themes.
type Manager struct {
	mu          sync.RWMutex
	themesDir   string
	activeFile  string
	active      *Theme
	userThemes  map[string]*Theme
}

// NewManager creates a theme manager. themesDir is where user theme JSON files live.
func NewManager(configDir string) *Manager {
	m := &Manager{
		themesDir:  filepath.Join(configDir, "themes"),
		activeFile: filepath.Join(configDir, "active_theme"),
		userThemes: make(map[string]*Theme),
	}
	m.active = builtins["dracula"] // Default

	// Load persisted active theme name
	if data, err := os.ReadFile(m.activeFile); err == nil {
		name := strings.TrimSpace(string(data))
		if t := m.get(name); t != nil {
			m.active = t
		}
	}

	// Load any user-defined themes
	m.loadUserThemes()
	return m
}

// Active returns the currently active theme.
func (m *Manager) Active() *Theme {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// Set activates a theme by name. Returns error if not found.
func (m *Manager) Set(name string) error {
	t := m.get(name)
	if t == nil {
		return fmt.Errorf("theme %q not found (built-ins: %s)", name, m.BuiltinNames())
	}
	m.mu.Lock()
	m.active = t
	m.mu.Unlock()

	// Persist
	_ = os.WriteFile(m.activeFile, []byte(name), 0644)
	return nil
}

// List returns names of all available themes (built-ins + user themes).
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(builtins)+len(m.userThemes))
	for n := range builtins {
		names = append(names, n)
	}
	for n := range m.userThemes {
		names = append(names, n)
	}
	return names
}

// BuiltinNames returns a comma-separated list of built-in theme names.
func (m *Manager) BuiltinNames() string {
	names := make([]string, 0, len(builtins))
	for n := range builtins {
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

// Format returns a human-readable summary of all themes.
func (m *Manager) Format() string {
	m.mu.RLock()
	active := m.active
	m.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("# Theme System\n\n")
	sb.WriteString(fmt.Sprintf("**Active:** `%s`\n\n", active.Name))
	sb.WriteString("## Built-in Themes\n\n")
	for _, t := range builtins {
		marker := ""
		if t.Name == active.Name {
			marker = " ← active"
		}
		sb.WriteString(fmt.Sprintf("• **`%s`**%s — %s\n", t.Name, marker, t.Description))
	}

	if len(m.userThemes) > 0 {
		sb.WriteString("\n## Custom Themes\n\n")
		for _, t := range m.userThemes {
			marker := ""
			if t.Name == active.Name {
				marker = " ← active"
			}
			sb.WriteString(fmt.Sprintf("• **`%s`**%s — %s\n", t.Name, marker, t.Description))
		}
	}

	sb.WriteString(fmt.Sprintf("\n**Themes directory:** `%s`\n", m.themesDir))
	sb.WriteString("\n**Usage:** `/theme <name>` to switch, or place `<name>.json` in the themes directory.\n")
	return sb.String()
}

// ThemesDir returns the path where user theme files are stored.
func (m *Manager) ThemesDir() string { return m.themesDir }

// get returns a theme by name (checks built-ins first, then user themes). Not thread-safe.
func (m *Manager) get(name string) *Theme {
	if t, ok := builtins[name]; ok {
		return t
	}
	m.mu.RLock()
	t := m.userThemes[name]
	m.mu.RUnlock()
	return t
}

// loadUserThemes reads all *.json files from themesDir.
func (m *Manager) loadUserThemes() {
	entries, err := os.ReadDir(m.themesDir)
	if err != nil {
		return // Directory may not exist yet
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(m.themesDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var t Theme
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		if t.Name == "" {
			t.Name = strings.TrimSuffix(e.Name(), ".json")
		}
		m.mu.Lock()
		m.userThemes[t.Name] = &t
		m.mu.Unlock()
	}
}
