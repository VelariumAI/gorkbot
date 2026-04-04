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

	// SecurityMode enables/disables access to security/penetration testing tools.
	// nil = disabled (default, secure). true = enabled. false = explicitly disabled.
	SecurityMode *bool `json:"security_mode,omitempty"`

	// VerboseMode controls whether internal system messages are suppressed.
	// nil/false = silent mode (suppress internal messages)
	// true = verbose mode (show all messages including system narration)
	VerboseMode *bool `json:"verbose_mode,omitempty"`

	// SuppressionConfig stores output filtering preferences.
	// These control which categories of internal messages to suppress.
	SuppressionConfig map[string]bool `json:"suppression_config,omitempty"`

	// HITLSettings controls Human-in-the-Loop approval for tool execution.
	HITL HITLSettings `json:"hitl,omitempty"`

	// EvolutionSettings controls Self-Evolution and Free Will Engine behavior.
	Evolution EvolutionSettings `json:"evolution,omitempty"`

	// SystemMonitorSettings controls resource monitoring behavior.
	SystemMonitor SystemMonitorSettings `json:"system_monitor,omitempty"`
}

// HITLSettings configures HITL override mechanisms for power users.
type HITLSettings struct {
	// Enabled master toggle. nil/true = HITL active (default, safe).
	Enabled *bool `json:"enabled,omitempty"`

	// MinRiskLevel: bypass HITL for risks below this level.
	// Values: "low", "medium", "high", "critical". "" = no bypass (default).
	MinRiskLevel string `json:"min_risk_level,omitempty"`

	// ConfidenceThreshold: auto-approve if AI confidence >= this (0-100).
	// Default 85. Set to 0 to disable confidence-based override.
	ConfidenceThreshold int `json:"confidence_threshold,omitempty"`

	// WhitelistedTools: list of tools that bypass HITL even if high-stakes.
	// Examples: "git_commit", "bash", "write_file"
	WhitelistedTools []string `json:"whitelisted_tools,omitempty"`

	// DisableWarning: skip confirmation when disabling HITL entirely.
	DisableWarning bool `json:"disable_warning,omitempty"`
}

// EvolutionSettings controls Self-Evolution and Free Will Engine behavior.
type EvolutionSettings struct {
	// Code Evolution master toggle
	CodeEvolutionEnabled *bool `json:"code_evolution_enabled,omitempty"`

	// Log retention in days
	LogRetentionDays int `json:"log_retention_days,omitempty"`

	// Free Will Engine master toggle
	FreeWillEngineEnabled *bool `json:"free_will_engine_enabled,omitempty"`

	// Max autonomous risk level: "low", "medium", "high", "none"
	MaxAutonomousRisk string `json:"max_autonomous_risk,omitempty"`

	// Auto-approve confidence threshold (0-100)
	ConfidenceThreshold int `json:"confidence_threshold,omitempty"`

	// Proposal frequency: "per_command", "per_session", "continuous"
	ProposalFrequency string `json:"proposal_frequency,omitempty"`

	// Loop guard sensitivity (0.0-1.0)
	LoopGuardSensitivity float64 `json:"loop_guard_sensitivity,omitempty"`

	// Rollback window size (number of changes to track)
	RollbackWindowSize int `json:"rollback_window_size,omitempty"`

	// SelfImproveEnabled controls the autonomous self-improvement drive.
	// nil/false = disabled (default), true = enabled
	SelfImproveEnabled *bool `json:"self_improve_enabled,omitempty"`
}

// SystemMonitorSettings controls resource monitoring behavior.
type SystemMonitorSettings struct {
	// Enabled master toggle for system_monitor tool
	Enabled *bool `json:"enabled,omitempty"`

	// ManualOnly: if true, only run system_monitor when explicitly requested
	ManualOnly *bool `json:"manual_only,omitempty"`

	// CooldownMinutes: cooldown period between auto-runs (default 30)
	CooldownMinutes int `json:"cooldown_minutes,omitempty"`

	// AlertThreshold: alert if resource below this % (default 50)
	AlertThreshold int `json:"alert_threshold,omitempty"`
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

// IsSecurityModeEnabled returns true when security tools are allowed.
// Returns false by default (nil means disabled, secure).
func (m *AppStateManager) IsSecurityModeEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.SecurityMode != nil && *m.st.SecurityMode
}

// SetSecurityMode persists the security mode enabled/disabled preference.
func (m *AppStateManager) SetSecurityMode(v bool) error {
	m.mu.Lock()
	m.st.SecurityMode = &v
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

// IsHITLEnabled returns true when HITL is active (default true).
// nil or true = HITL enabled, false = disabled (power user mode).
func (m *AppStateManager) IsHITLEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.HITL.Enabled == nil || *m.st.HITL.Enabled
}

// SetHITLEnabled persists the master HITL toggle.
func (m *AppStateManager) SetHITLEnabled(v bool) error {
	m.mu.Lock()
	m.st.HITL.Enabled = &v
	m.mu.Unlock()
	return m.save()
}

// GetHITLSettings returns a copy of the current HITL configuration.
func (m *AppStateManager) GetHITLSettings() HITLSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	settings := m.st.HITL
	if settings.WhitelistedTools != nil {
		tools := make([]string, len(settings.WhitelistedTools))
		copy(tools, settings.WhitelistedTools)
		settings.WhitelistedTools = tools
	}
	return settings
}

// SetHITLSettings persists the complete HITL configuration.
func (m *AppStateManager) SetHITLSettings(settings HITLSettings) error {
	m.mu.Lock()
	m.st.HITL = settings
	m.mu.Unlock()
	return m.save()
}

// SetHITLMinRiskLevel persists the minimum risk level for HITL bypass.
func (m *AppStateManager) SetHITLMinRiskLevel(level string) error {
	m.mu.Lock()
	m.st.HITL.MinRiskLevel = level
	m.mu.Unlock()
	return m.save()
}

// SetHITLConfidenceThreshold persists the confidence-based auto-approval threshold (0-100).
func (m *AppStateManager) SetHITLConfidenceThreshold(threshold int) error {
	if threshold < 0 {
		threshold = 0
	} else if threshold > 100 {
		threshold = 100
	}
	m.mu.Lock()
	m.st.HITL.ConfidenceThreshold = threshold
	m.mu.Unlock()
	return m.save()
}

// SetHITLWhitelistedTools persists the list of tools that bypass HITL.
func (m *AppStateManager) SetHITLWhitelistedTools(tools []string) error {
	m.mu.Lock()
	m.st.HITL.WhitelistedTools = tools
	m.mu.Unlock()
	return m.save()
}

// SetHITLDisableWarning persists the disable-warning preference.
func (m *AppStateManager) SetHITLDisableWarning(v bool) error {
	m.mu.Lock()
	m.st.HITL.DisableWarning = v
	m.mu.Unlock()
	return m.save()
}

// GetEvolutionSettings retrieves the current evolution settings.
func (m *AppStateManager) GetEvolutionSettings() EvolutionSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.Evolution
}

// SetEvolutionSettings persists the complete evolution configuration.
func (m *AppStateManager) SetEvolutionSettings(settings EvolutionSettings) error {
	m.mu.Lock()
	m.st.Evolution = settings
	m.mu.Unlock()
	return m.save()
}

// IsSelfImproveEnabled returns true when self-improvement drive is enabled.
// Returns false by default (nil means disabled).
func (m *AppStateManager) IsSelfImproveEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.Evolution.SelfImproveEnabled != nil && *m.st.Evolution.SelfImproveEnabled
}

// SetSelfImproveEnabled persists the self-improve enabled/disabled preference.
func (m *AppStateManager) SetSelfImproveEnabled(v bool) error {
	m.mu.Lock()
	m.st.Evolution.SelfImproveEnabled = &v
	m.mu.Unlock()
	return m.save()
}

// GetSystemMonitorSettings retrieves the current system monitor settings.
func (m *AppStateManager) GetSystemMonitorSettings() SystemMonitorSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.st.SystemMonitor
}

// SetSystemMonitorSettings persists the complete system monitor configuration.
func (m *AppStateManager) SetSystemMonitorSettings(settings SystemMonitorSettings) error {
	m.mu.Lock()
	m.st.SystemMonitor = settings
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
