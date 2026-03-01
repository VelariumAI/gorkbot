package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AppState persists user preferences across sessions.
type AppState struct {
	// Model selection
	PrimaryProvider   string `json:"primary_provider,omitempty"`
	PrimaryModel      string `json:"primary_model,omitempty"`
	SecondaryProvider string `json:"secondary_provider,omitempty"`
	SecondaryModel    string `json:"secondary_model,omitempty"`
	SecondaryAuto     bool   `json:"secondary_auto"`

	// Tool groups disabled by the user via /settings
	DisabledCategories []string `json:"disabled_categories,omitempty"`
}

// AppStateManager loads and saves AppState to a JSON file in the config directory.
// Pattern mirrors pkg/theme/theme.go Manager — safe for concurrent use.
type AppStateManager struct {
	mu   sync.RWMutex
	path string
	st   AppState
}

// NewAppStateManager creates a manager, loading existing state from configDir.
// Missing or corrupt files are silently ignored — callers receive a zero-value AppState.
func NewAppStateManager(configDir string) *AppStateManager {
	m := &AppStateManager{
		path: filepath.Join(configDir, "app_state.json"),
	}
	m.load()
	return m
}

// Get returns a copy of the current state.
func (m *AppStateManager) Get() AppState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st
}

// HasSavedModel returns true if a primary model was previously persisted.
func (m *AppStateManager) HasSavedModel() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.PrimaryProvider != ""
}

// SetPrimary persists a primary model selection.
func (m *AppStateManager) SetPrimary(provider, model string) error {
	m.mu.Lock()
	m.st.PrimaryProvider = provider
	m.st.PrimaryModel = model
	m.mu.Unlock()
	return m.save()
}

// SetSecondary persists an explicit secondary model selection.
func (m *AppStateManager) SetSecondary(provider, model string) error {
	m.mu.Lock()
	m.st.SecondaryProvider = provider
	m.st.SecondaryModel = model
	m.st.SecondaryAuto = false
	m.mu.Unlock()
	return m.save()
}

// SetSecondaryAuto persists the auto-secondary preference.
func (m *AppStateManager) SetSecondaryAuto() error {
	m.mu.Lock()
	m.st.SecondaryAuto = true
	m.st.SecondaryProvider = ""
	m.st.SecondaryModel = ""
	m.mu.Unlock()
	return m.save()
}

// SetDisabledCategories persists the list of disabled tool categories.
func (m *AppStateManager) SetDisabledCategories(cats []string) error {
	m.mu.Lock()
	m.st.DisabledCategories = cats
	m.mu.Unlock()
	return m.save()
}

// save writes state to disk atomically (0600 permissions).
func (m *AppStateManager) save() error {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.st, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0600)
}

// load reads state from disk. Silently returns zero-value on missing or invalid file.
func (m *AppStateManager) load() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return // file missing — use defaults
	}
	var st AppState
	if err := json.Unmarshal(data, &st); err != nil {
		return // corrupt file — use defaults
	}
	m.mu.Lock()
	m.st = st
	m.mu.Unlock()
}
