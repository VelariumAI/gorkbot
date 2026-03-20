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

	// Providers disabled by the user (persist across sessions).
	DisabledProviders []string `json:"disabled_providers,omitempty"`

	// CascadeOrder controls the provider failover sequence.
	// nil or empty means use the hardcoded default order.
	CascadeOrder []string `json:"cascade_order,omitempty"`

	// CompressionProvider pins a specific provider ID for compression.
	// "" means use the primary provider (default, recommended).
	CompressionProvider string `json:"compression_provider,omitempty"`

	// SandboxEnabled controls the SENSE input sanitizer. nil = default (true/enabled).
	SandboxEnabled *bool `json:"sandbox_enabled,omitempty"`

	// SREEnabled controls the Step-wise Reasoning Engine. nil = default (true/enabled).
	SREEnabled *bool `json:"sre_enabled,omitempty"`

	// EnsembleEnabled controls the multi-trajectory ensemble reasoning.
	EnsembleEnabled *bool `json:"ensemble_enabled,omitempty"`

	// VerboseMode controls whether internal system messages are suppressed.
	// nil/false = silent mode (suppress internal messages)
	// true = verbose mode (show all messages including system narration)
	VerboseMode *bool `json:"verbose_mode,omitempty"`

	// SuppressionConfig stores output filtering preferences.
	// These control which categories of internal messages to suppress.
	SuppressionConfig map[string]bool `json:"suppression_config,omitempty"`
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

// SetCascadeOrder persists a custom provider failover order.
// Pass nil to restore the default hardcoded order.
func (m *AppStateManager) SetCascadeOrder(order []string) error {
	m.mu.Lock()
	m.st.CascadeOrder = order
	m.mu.Unlock()
	return m.save()
}

// SetCompressionProvider persists a pinned compression provider.
// Pass "" to use the primary provider (recommended default).
func (m *AppStateManager) SetCompressionProvider(providerID string) error {
	m.mu.Lock()
	m.st.CompressionProvider = providerID
	m.mu.Unlock()
	return m.save()
}

// IsSandboxEnabled returns true when the SENSE input sanitizer should be active.
// Returns true by default (nil means enabled).
func (m *AppStateManager) IsSandboxEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.SandboxEnabled == nil || *m.st.SandboxEnabled
}

// SetSandboxEnabled persists the sandbox enabled/disabled preference.
func (m *AppStateManager) SetSandboxEnabled(v bool) error {
	m.mu.Lock()
	m.st.SandboxEnabled = &v
	m.mu.Unlock()
	return m.save()
}

// SetDisabledProviders persists the list of session-disabled provider IDs.
func (m *AppStateManager) SetDisabledProviders(ids []string) error {
	m.mu.Lock()
	m.st.DisabledProviders = ids
	m.mu.Unlock()
	return m.save()
}

// SetSREEnabled persists the SRE enabled/disabled preference.
func (m *AppStateManager) SetSREEnabled(v bool) error {
	m.mu.Lock()
	m.st.SREEnabled = &v
	m.mu.Unlock()
	return m.save()
}

// SetEnsembleEnabled persists the ensemble enabled/disabled preference.
func (m *AppStateManager) SetEnsembleEnabled(v bool) error {
	m.mu.Lock()
	m.st.EnsembleEnabled = &v
	m.mu.Unlock()
	return m.save()
}

// IsVerboseMode returns true when verbose mode is enabled (show all messages).
// Returns false by default (nil means silent mode, suppression active).
func (m *AppStateManager) IsVerboseMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.VerboseMode != nil && *m.st.VerboseMode
}

// SetVerboseMode persists the verbose mode enabled/disabled preference.
func (m *AppStateManager) SetVerboseMode(v bool) error {
	m.mu.Lock()
	m.st.VerboseMode = &v
	m.mu.Unlock()
	return m.save()
}

// GetSuppressionConfig returns the output suppression configuration.
// Returns nil if not set (will use defaults).
func (m *AppStateManager) GetSuppressionConfig() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.st.SuppressionConfig == nil {
		return nil
	}
	// Return a copy to prevent external modification
	config := make(map[string]bool)
	for k, v := range m.st.SuppressionConfig {
		config[k] = v
	}
	return config
}

// SetSuppressionConfig persists the output suppression configuration.
func (m *AppStateManager) SetSuppressionConfig(config map[string]bool) error {
	m.mu.Lock()
	m.st.SuppressionConfig = config
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
