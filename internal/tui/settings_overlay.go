package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/commands"
	pkgconfig "github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// settingsTab enumerates the settings sections.
type settingsTab int

const (
	tabModels          settingsTab = iota // 0 — Model routing summary
	tabVerbosity                          // 1 — Debug / logging toggles
	tabTools                              // 2 — Tool group enable/disable
	tabProviders                          // 3 — API provider enable/disable
	tabIntegrations                       // 4 — Integration env vars (budget, webhook, etc.)
	tabReasoningEngine                    // 5 — SRE and ensemble toggles
	tabHITL                               // 6 — HITL override settings for power users
	tabEvolution                          // 7 — Evolution & Free Will engine
	tabSystemMonitor                      // 8 — System monitor configuration
)

var tabLabels = []string{"Model Routing", "Verbosity", "Tool Groups", "API Providers", "Integrations", "Reasoning Engine", "HITL Control", "Evolution & Free Will", "System Monitor"}

type hitlWhitelistProfile struct {
	Name  string
	Tools []string
}

var hitlWhitelistProfiles = []hitlWhitelistProfile{
	{Name: "strict (none)", Tools: nil},
	{Name: "readonly", Tools: []string{"read_file", "list_directory", "grep_content"}},
	{Name: "editor", Tools: []string{"read_file", "list_directory", "grep_content", "write_file", "edit_file"}},
	{Name: "power", Tools: []string{"read_file", "list_directory", "grep_content", "write_file", "edit_file", "bash"}},
}

// SettingsOverlay is a four-tab modal for configuring runtime preferences.
// It implements the Overlay interface and persists changes immediately via
// the OrchestratorAdapter and tools.Registry.
type SettingsOverlay struct {
	BaseOverlay

	width  int
	height int

	activeTab settingsTab
	cursor    int

	orch           *commands.OrchestratorAdapter
	toolReg        *tools.Registry
	appStateSetter func(cats []string) error // writes disabled categories to disk

	// Verbosity section toggles (read from orch on construction)
	debugMode bool

	// Tool groups: sorted slice of category names + enabled state
	toolGroups []toolGroupRow

	// Provider rows: one row per known provider
	providerRows   []providerRow
	providerSetter func(ids []string) error // persists disabled provider list

	// Status line shown at the bottom of the modal after an action
	statusMsg   string
	statusIsErr bool

	// Integration settings (tabIntegrations)
	integrationGetter func() map[string]string      // returns current env var values
	integrationSetter func(key, value string) error // persists + os.Setenv

	// Inline text-editing state for tabIntegrations
	editingField bool
	editBuffer   string
	editOriginal string

	// Reasoning Engine settings (tabReasoningEngine)
	sreEnabled       bool
	ensembleEnabled  bool
	sreEnabledSetter func(bool) error
	ensembleSetter   func(bool) error

	// HITL settings (tabHITL)
	hitlEnabled             bool
	hitlMinRiskLevel        string // "low", "medium", "high", "critical", or ""
	hitlConfidenceThreshold int    // 0-100
	hitlWhitelistedTools    []string
	hitlDisableWarning      bool
	hitlSettingsGetter      func() interface{}      // config.HITLSettings
	hitlSettingsSetter      func(interface{}) error // config.HITLSettings

	// Evolution & Free Will settings (tabEvolution)
	evolutionEngineEnabled                  bool
	codeEvolutionEnabled                    bool
	codeEvolutionTripleConfirmationAttempts int
	codeEvolutionLastConfirmationTimestamp  time.Time
	evolutionLogRetentionDays               int
	freeWillEngineEnabled                   bool
	freeWillMaxRisk                         string // "low", "medium", "high", "none"
	freeWillConfidenceThreshold             int    // 0-100
	freeWillProposalFreq                    string // "per_command", "per_session", "continuous"
	loopGuardSensitivity                    float64
	rollbackWindowSize                      int

	tripleConfirmationModal *TripleConfirmationModal
	isShowingConfirmation   bool

	evolutionSettingsGetter func() interface{}
	evolutionSettingsSetter func(interface{}) error

	// System Monitor settings (tabSystemMonitor)
	systemMonitorEnabled        bool
	systemMonitorManualOnly     bool
	systemMonitorCooldownMins   int
	systemMonitorAlertThreshold int
	systemMonitorSettingsGetter func() interface{}
	systemMonitorSettingsSetter func(interface{}) error
}

// TripleConfirmationModal manages the 3-step code_evolution activation flow
type TripleConfirmationModal struct {
	step             int
	randomCode       string
	expectedPhrase   string
	userInput        string
	failedAttempts   int
	lastFailureTime  time.Time
	coolingDown      bool
	coolingDownUntil time.Time
}

const (
	confirmationCooldown = 30 * time.Second
	maxFailedAttempts    = 3
)

type toolGroupRow struct {
	name    string
	enabled bool
}

type providerRow struct {
	id      string
	name    string
	enabled bool
}

// NewSettingsOverlay constructs a SettingsOverlay. Pass nil for any callback
// to skip persistence of that section.
func NewSettingsOverlay(
	w, h int,
	orch *commands.OrchestratorAdapter,
	toolReg *tools.Registry,
	appStateSetter func(cats []string) error,
	initialDebug bool,
	providerSetter func(ids []string) error,
	integrationGetter func() map[string]string,
	integrationSetter func(key, value string) error,
	sreEnabled, ensembleEnabled bool,
	sreEnabledSetter func(bool) error,
	ensembleSetter func(bool) error,
	hitlSettingsGetter func() interface{},
	hitlSettingsSetter func(interface{}) error,
	evolutionSettingsGetter func() interface{},
	evolutionSettingsSetter func(interface{}) error,
	systemMonitorSettingsGetter func() interface{},
	systemMonitorSettingsSetter func(interface{}) error,
) *SettingsOverlay {
	s := &SettingsOverlay{
		width:                       w,
		height:                      h,
		orch:                        orch,
		toolReg:                     toolReg,
		appStateSetter:              appStateSetter,
		debugMode:                   initialDebug,
		providerSetter:              providerSetter,
		integrationGetter:           integrationGetter,
		integrationSetter:           integrationSetter,
		sreEnabled:                  sreEnabled,
		ensembleEnabled:             ensembleEnabled,
		sreEnabledSetter:            sreEnabledSetter,
		ensembleSetter:              ensembleSetter,
		hitlSettingsGetter:          hitlSettingsGetter,
		hitlSettingsSetter:          hitlSettingsSetter,
		evolutionSettingsGetter:     evolutionSettingsGetter,
		evolutionSettingsSetter:     evolutionSettingsSetter,
		freeWillMaxRisk:             "low",
		freeWillConfidenceThreshold: 85,
		freeWillProposalFreq:        "per_session",
		loopGuardSensitivity:        0.85,
		rollbackWindowSize:          20,
		evolutionLogRetentionDays:   60,
		systemMonitorSettingsGetter: systemMonitorSettingsGetter,
		systemMonitorSettingsSetter: systemMonitorSettingsSetter,
		systemMonitorEnabled:        true,
		systemMonitorManualOnly:     true, // DEFAULT: manual-only to prevent auto-spam
		systemMonitorCooldownMins:   30,
		systemMonitorAlertThreshold: 50,
	}
	s.refreshToolGroups()
	s.refreshProviderRows()
	s.refreshHITLSettings()
	s.refreshEvolutionSettings()
	s.refreshSystemMonitorSettings()
	return s
}

func (s *SettingsOverlay) refreshHITLSettings() {
	if s.hitlSettingsGetter == nil {
		return
	}
	iface := s.hitlSettingsGetter()
	if iface == nil {
		return
	}
	settings, ok := iface.(pkgconfig.HITLSettings)
	if !ok {
		return
	}
	s.hitlEnabled = settings.Enabled == nil || *settings.Enabled
	s.hitlMinRiskLevel = settings.MinRiskLevel
	s.hitlConfidenceThreshold = settings.ConfidenceThreshold
	s.hitlWhitelistedTools = settings.WhitelistedTools
	s.hitlDisableWarning = settings.DisableWarning
}

func (s *SettingsOverlay) refreshEvolutionSettings() {
	if s.evolutionSettingsGetter == nil {
		return
	}
	iface := s.evolutionSettingsGetter()
	if iface == nil {
		return
	}
	settings, ok := iface.(pkgconfig.EvolutionSettings)
	if !ok {
		return
	}
	if settings.CodeEvolutionEnabled != nil {
		s.codeEvolutionEnabled = *settings.CodeEvolutionEnabled
	}
	if settings.LogRetentionDays > 0 {
		s.evolutionLogRetentionDays = settings.LogRetentionDays
	} else {
		s.evolutionLogRetentionDays = 60 // default
	}
	if settings.FreeWillEngineEnabled != nil {
		s.freeWillEngineEnabled = *settings.FreeWillEngineEnabled
	}
	if settings.MaxAutonomousRisk != "" {
		s.freeWillMaxRisk = settings.MaxAutonomousRisk
	} else {
		s.freeWillMaxRisk = "low" // default
	}
	if settings.ConfidenceThreshold > 0 {
		s.freeWillConfidenceThreshold = settings.ConfidenceThreshold
	} else {
		s.freeWillConfidenceThreshold = 85 // default
	}
	if settings.ProposalFrequency != "" {
		s.freeWillProposalFreq = settings.ProposalFrequency
	} else {
		s.freeWillProposalFreq = "per_session" // default
	}
	if settings.LoopGuardSensitivity > 0 {
		s.loopGuardSensitivity = settings.LoopGuardSensitivity
	} else {
		s.loopGuardSensitivity = 0.85 // default
	}
	if settings.RollbackWindowSize > 0 {
		s.rollbackWindowSize = settings.RollbackWindowSize
	} else {
		s.rollbackWindowSize = 20 // default
	}
}

func (s *SettingsOverlay) refreshSystemMonitorSettings() {
	if s.systemMonitorSettingsGetter == nil {
		return
	}
	iface := s.systemMonitorSettingsGetter()
	if iface == nil {
		return
	}
	settings, ok := iface.(pkgconfig.SystemMonitorSettings)
	if !ok {
		return
	}
	if settings.Enabled != nil {
		s.systemMonitorEnabled = *settings.Enabled
	} else {
		s.systemMonitorEnabled = true // default
	}
	if settings.ManualOnly != nil {
		s.systemMonitorManualOnly = *settings.ManualOnly
	} else {
		s.systemMonitorManualOnly = true // default: manual-only to prevent auto-spam
	}
	if settings.CooldownMinutes > 0 {
		s.systemMonitorCooldownMins = settings.CooldownMinutes
	} else {
		s.systemMonitorCooldownMins = 30 // default
	}
	if settings.AlertThreshold > 0 && settings.AlertThreshold <= 100 {
		s.systemMonitorAlertThreshold = settings.AlertThreshold
	} else {
		s.systemMonitorAlertThreshold = 50 // default
	}
}

func (s *SettingsOverlay) refreshToolGroups() {
	if s.toolReg == nil {
		return
	}
	cats := s.toolReg.Categories()
	rows := make([]toolGroupRow, 0, len(cats))
	for _, c := range cats {
		rows = append(rows, toolGroupRow{
			name:    string(c),
			enabled: s.toolReg.IsCategoryEnabled(c),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	s.toolGroups = rows
}

func (s *SettingsOverlay) refreshProviderRows() {
	if s.orch == nil || s.orch.GetProviderEnabled == nil {
		return
	}
	enabled := s.orch.GetProviderEnabled()
	// Use a fixed display order matching providerPriority.
	ordered := []string{"xai", "google", "anthropic", "minimax", "openai", "openrouter"}
	names := map[string]string{
		"xai":        "xAI",
		"google":     "Google",
		"anthropic":  "Anthropic",
		"minimax":    "MiniMax",
		"openai":     "OpenAI",
		"openrouter": "OpenRouter",
	}
	rows := make([]providerRow, 0, len(ordered))
	for _, id := range ordered {
		e, ok := enabled[id]
		if !ok {
			e = true // default to enabled if not in map
		}
		rows = append(rows, providerRow{
			id:      id,
			name:    names[id],
			enabled: e,
		})
	}
	s.providerRows = rows
}

// Helper functions
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Triple Confirmation Modal methods

func (m *TripleConfirmationModal) generateRandomCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, 6)
	for i := range code {
		code[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(code)
}

func (s *SettingsOverlay) initiateTripleConfirmation() {
	now := time.Now()

	// Check cooldown
	if s.tripleConfirmationModal != nil && s.tripleConfirmationModal.coolingDown {
		if now.Before(s.tripleConfirmationModal.coolingDownUntil) {
			return
		}
		s.tripleConfirmationModal.coolingDown = false
	}

	// Initialize modal
	if s.tripleConfirmationModal == nil {
		s.tripleConfirmationModal = &TripleConfirmationModal{}
	}

	s.tripleConfirmationModal.step = 1
	s.tripleConfirmationModal.randomCode = s.tripleConfirmationModal.generateRandomCode()
	s.tripleConfirmationModal.expectedPhrase = "I accept full responsibility for code_evolution"
	s.tripleConfirmationModal.userInput = ""
	s.tripleConfirmationModal.failedAttempts = 0
	s.tripleConfirmationModal.lastFailureTime = now
	s.isShowingConfirmation = true
}

func (s *SettingsOverlay) handleConfirmationInput(input string) bool {
	if s.tripleConfirmationModal == nil {
		return false
	}

	m := s.tripleConfirmationModal
	now := time.Now()

	switch m.step {
	case 1:
		// Step 1: Verify random code
		if input == m.randomCode {
			m.step = 2
			m.userInput = ""
			return true
		}
		m.failedAttempts++
		m.lastFailureTime = now
		if m.failedAttempts >= maxFailedAttempts {
			m.coolingDown = true
			m.coolingDownUntil = now.Add(confirmationCooldown)
			s.isShowingConfirmation = false
		}
		return false

	case 2:
		// Step 2: Verify acceptance phrase
		if input == m.expectedPhrase {
			m.step = 3
			m.userInput = ""
			return true
		}
		m.failedAttempts++
		m.lastFailureTime = now
		if m.failedAttempts >= maxFailedAttempts {
			m.coolingDown = true
			m.coolingDownUntil = now.Add(confirmationCooldown)
			s.isShowingConfirmation = false
		}
		return false

	case 3:
		// Step 3: Verify final confirmation
		if input == "YES I UNDERSTAND THIS MAY BE IRREVERSIBLE" {
			m.step = 0
			s.isShowingConfirmation = false
			// Update counters
			s.codeEvolutionTripleConfirmationAttempts++
			s.codeEvolutionLastConfirmationTimestamp = now
			s.saveEvolutionSettings()
			// NOW actually toggle code_evolution
			s.codeEvolutionEnabled = !s.codeEvolutionEnabled
			s.saveEvolutionSettings()
			return true
		}
		m.failedAttempts++
		m.lastFailureTime = now
		if m.failedAttempts >= maxFailedAttempts {
			m.coolingDown = true
			m.coolingDownUntil = now.Add(confirmationCooldown)
			s.isShowingConfirmation = false
		}
		return false
	}

	return false
}

func (s *SettingsOverlay) saveEvolutionSettings() error {
	if s.evolutionSettingsSetter == nil {
		return nil
	}
	codeEvoEnabled := s.codeEvolutionEnabled
	freeWillEnabled := s.freeWillEngineEnabled
	settings := pkgconfig.EvolutionSettings{
		CodeEvolutionEnabled:  &codeEvoEnabled,
		LogRetentionDays:      s.evolutionLogRetentionDays,
		FreeWillEngineEnabled: &freeWillEnabled,
		MaxAutonomousRisk:     s.freeWillMaxRisk,
		ConfidenceThreshold:   s.freeWillConfidenceThreshold,
		ProposalFrequency:     s.freeWillProposalFreq,
		LoopGuardSensitivity:  s.loopGuardSensitivity,
		RollbackWindowSize:    s.rollbackWindowSize,
	}
	return s.evolutionSettingsSetter(settings)
}

// ── Overlay interface ─────────────────────────────────────────────────────────

func (s *SettingsOverlay) Init() tea.Cmd { return nil }

func (s *SettingsOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	// ── In-field text editing mode ────────────────────────────────────────────
	if s.editingField {
		switch keyMsg.String() {
		case "esc", "ctrl+c":
			// Cancel: restore original value.
			s.editBuffer = s.editOriginal
			s.editingField = false
			s.statusMsg = "Edit cancelled"
		case "enter":
			// Save value.
			s.saveIntegrationField()
			s.editingField = false
		case "backspace", "ctrl+h":
			if len(s.editBuffer) > 0 {
				runes := []rune(s.editBuffer)
				s.editBuffer = string(runes[:len(runes)-1])
			}
		case "ctrl+u":
			s.editBuffer = ""
		default:
			// Append printable characters.
			if len(keyMsg.Runes) > 0 && keyMsg.Runes[0] >= 32 {
				s.editBuffer += string(keyMsg.Runes)
			}
		}
		return s, nil
	}

	// ── Triple Confirmation Modal (code_evolution) ─────────────────────────────
	if s.isShowingConfirmation {
		switch keyMsg.String() {
		case "esc":
			s.isShowingConfirmation = false
			s.tripleConfirmationModal = nil
			s.statusMsg = "code_evolution activation cancelled"
		case "enter":
			input := strings.TrimSpace(s.tripleConfirmationModal.userInput)
			if s.handleConfirmationInput(input) {
				// Step successfully completed
				if s.tripleConfirmationModal.step == 0 {
					// All 3 steps complete
					s.statusMsg = "code_evolution activated successfully"
				}
			} else {
				// Step failed
				if s.isShowingConfirmation {
					s.statusMsg = fmt.Sprintf("Incorrect input (%d/%d attempts)",
						s.tripleConfirmationModal.failedAttempts, maxFailedAttempts)
				} else {
					s.statusMsg = fmt.Sprintf("Cooldown active (~%d seconds remaining)",
						int(s.tripleConfirmationModal.coolingDownUntil.Sub(time.Now()).Seconds())+1)
				}
			}
			s.tripleConfirmationModal.userInput = ""
		case "backspace":
			if len(s.tripleConfirmationModal.userInput) > 0 {
				runes := []rune(s.tripleConfirmationModal.userInput)
				s.tripleConfirmationModal.userInput = string(runes[:len(runes)-1])
			}
		default:
			// Append printable characters
			if len(keyMsg.Runes) > 0 && keyMsg.Runes[0] >= 32 {
				s.tripleConfirmationModal.userInput += string(keyMsg.Runes)
			}
		}
		return s, nil
	}

	s.statusMsg = "" // clear status on any key

	switch keyMsg.String() {
	case "esc":
		return nil, nil // signals Model.Update to close the overlay

	case "tab":
		s.activeTab = settingsTab((int(s.activeTab) + 1) % len(tabLabels))
		s.cursor = 0
	case "shift+tab":
		s.activeTab = settingsTab((int(s.activeTab) + len(tabLabels) - 1) % len(tabLabels))
		s.cursor = 0

	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		maxCursor := s.maxCursor()
		if s.cursor < maxCursor {
			s.cursor++
		}

	case "enter":
		if s.activeTab == tabIntegrations {
			s.startEditingField()
		} else {
			s.handleAction()
		}
	case " ":
		if s.activeTab != tabIntegrations {
			s.handleAction()
		}
	}

	return s, nil
}

// startEditingField enters inline-edit mode for the currently selected integration row.
func (s *SettingsOverlay) startEditingField() {
	if s.cursor >= len(pkgconfig.IntegrationKeys) {
		return
	}
	key := pkgconfig.IntegrationKeys[s.cursor].Key
	current := ""
	if s.integrationGetter != nil {
		current = s.integrationGetter()[key]
	}
	s.editBuffer = current
	s.editOriginal = current
	s.editingField = true
	s.statusMsg = ""
}

// saveIntegrationField persists the editBuffer for the currently selected row.
func (s *SettingsOverlay) saveIntegrationField() {
	if s.cursor >= len(pkgconfig.IntegrationKeys) {
		return
	}
	key := pkgconfig.IntegrationKeys[s.cursor].Key
	if s.integrationSetter != nil {
		if err := s.integrationSetter(key, s.editBuffer); err != nil {
			s.statusMsg = fmt.Sprintf("Save failed: %v", err)
			s.statusIsErr = true
			return
		}
	}
	s.statusMsg = fmt.Sprintf("%s updated", pkgconfig.IntegrationKeys[s.cursor].Label)
	s.statusIsErr = false
}

func (s *SettingsOverlay) maxCursor() int {
	switch s.activeTab {
	case tabVerbosity:
		return 0 // one toggle
	case tabTools:
		if len(s.toolGroups) == 0 {
			return 0
		}
		return len(s.toolGroups) - 1
	case tabProviders:
		if len(s.providerRows) == 0 {
			return 0
		}
		return len(s.providerRows) - 1
	case tabIntegrations:
		return len(pkgconfig.IntegrationKeys) - 1
	case tabReasoningEngine:
		return 1 // two toggles: SRE and Ensemble
	case tabHITL:
		return 4 // 5 items: master toggle, risk level, confidence, whitelist, warning
	case tabEvolution:
		return 8 // Section A: 3 items (engine, code_evolution, retention) + Section B: 6 items = 9 total, max cursor is 8
	case tabSystemMonitor:
		return 3 // 4 items: enabled, manual-only, cooldown, alert threshold
	default:
		return 0
	}
}

func (s *SettingsOverlay) handleAction() {
	switch s.activeTab {
	case tabVerbosity:
		if s.orch != nil && s.orch.ToggleDebug != nil {
			s.debugMode = s.orch.ToggleDebug()
			if s.debugMode {
				s.statusMsg = "Debug mode ON — raw tool JSON visible"
			} else {
				s.statusMsg = "Debug mode OFF"
			}
		}

	case tabTools:
		if s.toolReg == nil || s.cursor >= len(s.toolGroups) {
			return
		}
		row := &s.toolGroups[s.cursor]
		row.enabled = !row.enabled
		s.toolReg.SetCategoryEnabled(tools.ToolCategory(row.name), row.enabled)
		if row.enabled {
			s.statusMsg = fmt.Sprintf("Category %q enabled", row.name)
		} else {
			s.statusMsg = fmt.Sprintf("Category %q disabled", row.name)
		}
		// Persist disabled list
		if s.appStateSetter != nil {
			var disabled []string
			for _, r := range s.toolGroups {
				if !r.enabled {
					disabled = append(disabled, r.name)
				}
			}
			if err := s.appStateSetter(disabled); err != nil {
				s.statusMsg += " (save failed)"
				s.statusIsErr = true
			}
		}

	case tabProviders:
		if s.orch == nil || s.orch.ToggleProvider == nil || s.cursor >= len(s.providerRows) {
			return
		}
		row := &s.providerRows[s.cursor]
		enabled, msg := s.orch.ToggleProvider(row.id)
		row.enabled = enabled
		s.statusMsg = msg
		s.statusIsErr = false
		// Persist full disabled list via providerSetter.
		if s.providerSetter != nil {
			var disabled []string
			for _, r := range s.providerRows {
				if !r.enabled {
					disabled = append(disabled, r.id)
				}
			}
			if err := s.providerSetter(disabled); err != nil {
				s.statusMsg += " (save failed)"
				s.statusIsErr = true
			}
		}

	case tabReasoningEngine:
		if s.cursor == 0 {
			// Toggle SRE
			s.sreEnabled = !s.sreEnabled
			if s.sreEnabledSetter != nil {
				if err := s.sreEnabledSetter(s.sreEnabled); err != nil {
					s.statusMsg = "Failed to save SRE setting"
					s.statusIsErr = true
				} else {
					if s.sreEnabled {
						s.statusMsg = "Step-wise Reasoning enabled"
					} else {
						s.statusMsg = "Step-wise Reasoning disabled"
					}
					s.statusIsErr = false
				}
			}
		} else if s.cursor == 1 {
			// Toggle Ensemble
			s.ensembleEnabled = !s.ensembleEnabled
			if s.ensembleSetter != nil {
				if err := s.ensembleSetter(s.ensembleEnabled); err != nil {
					s.statusMsg = "Failed to save Ensemble setting"
					s.statusIsErr = true
				} else {
					if s.ensembleEnabled {
						s.statusMsg = "Ensemble enabled"
					} else {
						s.statusMsg = "Ensemble disabled"
					}
					s.statusIsErr = false
				}
			}
		}

	case tabHITL:
		if s.cursor == 0 {
			// Toggle HITL master
			s.hitlEnabled = !s.hitlEnabled
			s.saveHITLSettings()
			if s.hitlEnabled {
				s.statusMsg = "HITL enabled (safe mode)"
			} else if !s.hitlDisableWarning {
				s.statusMsg = "⚠ HITL disabled (power user mode) — Gorkbot can auto-execute dangerous operations"
			} else {
				s.statusMsg = "HITL disabled (power user mode)"
			}
		} else if s.cursor == 1 {
			// Cycle risk level
			levels := []string{"", "low", "medium", "high"}
			idx := 0
			for i, l := range levels {
				if l == s.hitlMinRiskLevel {
					idx = i
					break
				}
			}
			idx = (idx + 1) % len(levels)
			s.hitlMinRiskLevel = levels[idx]
			s.saveHITLSettings()
			if s.hitlMinRiskLevel == "" {
				s.statusMsg = "Risk-based override: disabled"
			} else {
				s.statusMsg = fmt.Sprintf("Auto-approve risks below: %s", s.hitlMinRiskLevel)
			}
		} else if s.cursor == 2 {
			// Adjust confidence threshold (0-100, step by 5)
			s.hitlConfidenceThreshold = (s.hitlConfidenceThreshold + 5) % 105
			s.saveHITLSettings()
			s.statusMsg = fmt.Sprintf("Confidence threshold: %d%%", s.hitlConfidenceThreshold)
		} else if s.cursor == 3 {
			profile := s.cycleHITLWhitelistProfile()
			s.statusMsg = fmt.Sprintf("Whitelist profile: %s", profile)
		} else if s.cursor == 4 {
			// Toggle disable-warning
			s.hitlDisableWarning = !s.hitlDisableWarning
			s.saveHITLSettings()
			if s.hitlDisableWarning {
				s.statusMsg = "Disabled warning on HITL toggle"
			} else {
				s.statusMsg = "Re-enabled warning on HITL toggle"
			}
		}

	case tabEvolution:
		switch s.cursor {
		case 0: // Toggle Evolution Engine Enabled
			s.evolutionEngineEnabled = !s.evolutionEngineEnabled
			s.saveEvolutionSettings()
			if s.evolutionEngineEnabled {
				s.statusMsg = "Evolution Engine enabled"
			} else {
				s.statusMsg = "Evolution Engine disabled"
			}
		case 1: // Toggle code_evolution Domain Enabled
			if !s.codeEvolutionEnabled {
				// Attempting to enable; trigger triple confirmation
				s.initiateTripleConfirmation()
				s.statusMsg = "code_evolution triple confirmation started"
			} else {
				// Disabling is allowed without confirmation
				s.codeEvolutionEnabled = false
				s.saveEvolutionSettings()
				s.statusMsg = "code_evolution disabled"
			}
		case 2: // Evolution Log Retention (cycle: 30, 60, 90, 180)
			retentionValues := []int{30, 60, 90, 180}
			for i, val := range retentionValues {
				if val == s.evolutionLogRetentionDays {
					s.evolutionLogRetentionDays = retentionValues[(i+1)%len(retentionValues)]
					break
				}
			}
			s.saveEvolutionSettings()
			s.statusMsg = fmt.Sprintf("Evolution Log Retention: %d days", s.evolutionLogRetentionDays)
		case 3: // Toggle Free Will Engine Enabled
			s.freeWillEngineEnabled = !s.freeWillEngineEnabled
			s.saveEvolutionSettings()
			if s.freeWillEngineEnabled {
				s.statusMsg = "Free Will Engine enabled"
			} else {
				s.statusMsg = "Free Will Engine disabled"
			}
		case 4: // Cycle Max Autonomous Risk
			risks := []string{"low", "medium", "high", "none"}
			for i, r := range risks {
				if r == s.freeWillMaxRisk {
					s.freeWillMaxRisk = risks[(i+1)%len(risks)]
					break
				}
			}
			s.saveEvolutionSettings()
			s.statusMsg = fmt.Sprintf("Max Autonomous Risk: %s", s.freeWillMaxRisk)
		case 5: // Confidence Threshold (slider: 0-100, step 5)
			s.freeWillConfidenceThreshold = (s.freeWillConfidenceThreshold + 5) % 105
			s.saveEvolutionSettings()
			s.statusMsg = fmt.Sprintf("Confidence Threshold: %d%%", s.freeWillConfidenceThreshold)
		case 6: // Proposal Frequency
			freqs := []string{"per_command", "per_session", "continuous"}
			for i, f := range freqs {
				if f == s.freeWillProposalFreq {
					s.freeWillProposalFreq = freqs[(i+1)%len(freqs)]
					break
				}
			}
			s.saveEvolutionSettings()
			s.statusMsg = fmt.Sprintf("Proposal Frequency: %s", s.freeWillProposalFreq)
		case 7: // Loop Guard Sensitivity (slider: 0.0-1.0, step 0.1)
			s.loopGuardSensitivity = math.Round((s.loopGuardSensitivity+0.1)*10) / 10
			if s.loopGuardSensitivity > 1.0 {
				s.loopGuardSensitivity = 0.0
			}
			s.saveEvolutionSettings()
			s.statusMsg = fmt.Sprintf("Loop Guard Sensitivity: %.2f", s.loopGuardSensitivity)
		case 8: // Rollback Window Size (cycle: 5, 10, 20, 50)
			windowSizes := []int{5, 10, 20, 50}
			for i, size := range windowSizes {
				if size == s.rollbackWindowSize {
					s.rollbackWindowSize = windowSizes[(i+1)%len(windowSizes)]
					break
				}
			}
			s.saveEvolutionSettings()
			s.statusMsg = fmt.Sprintf("Rollback Window Size: %d", s.rollbackWindowSize)
		}

	case tabSystemMonitor:
		switch s.cursor {
		case 0: // Toggle system_monitor enabled
			s.systemMonitorEnabled = !s.systemMonitorEnabled
			s.saveSystemMonitorSettings()
			if s.systemMonitorEnabled {
				s.statusMsg = "system_monitor enabled"
			} else {
				s.statusMsg = "system_monitor disabled"
			}
		case 1: // Toggle manual-only mode
			s.systemMonitorManualOnly = !s.systemMonitorManualOnly
			s.saveSystemMonitorSettings()
			if s.systemMonitorManualOnly {
				s.statusMsg = "system_monitor: manual-only mode enabled"
			} else {
				s.statusMsg = "system_monitor: auto-run enabled"
			}
		case 2: // Cycle cooldown minutes (30, 60, 90, 120)
			cooldowns := []int{30, 60, 90, 120}
			for i, val := range cooldowns {
				if val == s.systemMonitorCooldownMins {
					s.systemMonitorCooldownMins = cooldowns[(i+1)%len(cooldowns)]
					break
				}
			}
			s.saveSystemMonitorSettings()
			s.statusMsg = fmt.Sprintf("system_monitor cooldown: %d minutes", s.systemMonitorCooldownMins)
		case 3: // Cycle alert threshold (20, 30, 50, 70)
			thresholds := []int{20, 30, 50, 70}
			for i, val := range thresholds {
				if val == s.systemMonitorAlertThreshold {
					s.systemMonitorAlertThreshold = thresholds[(i+1)%len(thresholds)]
					break
				}
			}
			s.saveSystemMonitorSettings()
			s.statusMsg = fmt.Sprintf("system_monitor alert threshold: %d%%", s.systemMonitorAlertThreshold)
		}
	}
}

func (s *SettingsOverlay) saveHITLSettings() {
	if s.hitlSettingsSetter == nil {
		return
	}
	settings := pkgconfig.HITLSettings{
		Enabled:             &s.hitlEnabled,
		MinRiskLevel:        s.hitlMinRiskLevel,
		ConfidenceThreshold: s.hitlConfidenceThreshold,
		WhitelistedTools:    s.hitlWhitelistedTools,
		DisableWarning:      s.hitlDisableWarning,
	}
	if err := s.hitlSettingsSetter(interface{}(settings)); err != nil {
		s.statusMsg += " (save failed)"
		s.statusIsErr = true
	} else {
		s.statusIsErr = false
	}
}

func (s *SettingsOverlay) saveSystemMonitorSettings() {
	if s.systemMonitorSettingsSetter == nil {
		return
	}
	enabled := s.systemMonitorEnabled
	manualOnly := s.systemMonitorManualOnly
	settings := pkgconfig.SystemMonitorSettings{
		Enabled:         &enabled,
		ManualOnly:      &manualOnly,
		CooldownMinutes: s.systemMonitorCooldownMins,
		AlertThreshold:  s.systemMonitorAlertThreshold,
	}
	if err := s.systemMonitorSettingsSetter(interface{}(settings)); err != nil {
		s.statusMsg += " (save failed)"
		s.statusIsErr = true
	} else {
		s.statusIsErr = false
	}
}

// View renders the settings modal. Self-contained box using lipgloss.
func (s *SettingsOverlay) View() string {
	boxW := s.width - 8
	if boxW < 50 {
		boxW = 50
	}
	if boxW > 90 {
		boxW = 90
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).Bold(true)
	activeTabStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("99")).Foreground(lipgloss.Color("255")).Padding(0, 1)
	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).Padding(0, 1)
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	uncheckStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	if s.statusIsErr {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	}
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var lines []string

	// Title
	lines = append(lines, titleStyle.Render("⚙  Settings"))
	lines = append(lines, "")

	// Tab bar
	tabs := make([]string, len(tabLabels))
	for i, label := range tabLabels {
		if settingsTab(i) == s.activeTab {
			tabs[i] = activeTabStyle.Render(label)
		} else {
			tabs[i] = inactiveTabStyle.Render(label)
		}
	}
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	lines = append(lines, strings.Repeat("─", boxW-4))
	lines = append(lines, "")

	// Section content
	switch s.activeTab {
	case tabModels:
		lines = append(lines, s.renderModelsSection(dimStyle)...)
	case tabVerbosity:
		lines = append(lines, s.renderVerbositySection(cursorStyle, checkStyle, uncheckStyle)...)
	case tabTools:
		lines = append(lines, s.renderToolsSection(cursorStyle, checkStyle, uncheckStyle, dimStyle)...)
	case tabProviders:
		lines = append(lines, s.renderProvidersSection(cursorStyle, checkStyle, uncheckStyle, dimStyle)...)
	case tabIntegrations:
		lines = append(lines, s.renderIntegrationsSection(cursorStyle, dimStyle, boxW)...)
	case tabReasoningEngine:
		lines = append(lines, s.renderReasoningEngineSection(cursorStyle, checkStyle, uncheckStyle)...)
	case tabHITL:
		lines = append(lines, s.renderHITLSection(cursorStyle, checkStyle, uncheckStyle)...)
	case tabEvolution:
		lines = append(lines, s.renderEvolutionSection(cursorStyle, checkStyle, uncheckStyle, dimStyle)...)
	case tabSystemMonitor:
		lines = append(lines, s.renderSystemMonitorSection(cursorStyle, checkStyle, uncheckStyle, dimStyle)...)
	}

	// Status
	lines = append(lines, "")
	if s.statusMsg != "" {
		lines = append(lines, statusStyle.Render("  "+s.statusMsg))
	} else {
		lines = append(lines, "")
	}

	// Help line
	lines = append(lines, strings.Repeat("─", boxW-4))
	var helpText string
	if s.editingField {
		helpText = "  Type value  Enter=save  Esc=cancel  Ctrl+U=clear"
	} else if s.activeTab == tabIntegrations {
		helpText = "  Tab=switch section  ↑↓=navigate  Enter=edit field  Esc=close"
	} else {
		helpText = "  Tab=switch section  ↑↓=navigate  Enter/Space=toggle  Esc=close"
	}
	lines = append(lines, helpStyle.Render(helpText))

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("99")).
		Padding(1, 2).
		Width(boxW).
		Render(content)

	return lipgloss.Place(s.width, s.height,
		lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars("░"),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
}

func (s *SettingsOverlay) renderModelsSection(dim lipgloss.Style) []string {
	var lines []string
	if s.orch == nil {
		lines = append(lines, dim.Render("  Orchestrator not available."))
		return lines
	}
	// Show current primary/secondary from provider status
	status := ""
	if s.orch.GetProviderStatus != nil {
		status = s.orch.GetProviderStatus()
	}
	if status != "" {
		for _, l := range strings.Split(status, "\n") {
			lines = append(lines, "  "+l)
		}
	} else {
		lines = append(lines, dim.Render("  No provider status available."))
	}
	lines = append(lines, "")
	lines = append(lines, dim.Render("  Press Ctrl+T to open the Model Selector."))

	// ── Live System State (Phase 4.3) ──────────────────────────────────────
	if s.orch != nil && s.orch.GetDiagnosticReport != nil {
		report := s.orch.GetDiagnosticReport()
		if report != "" {
			lines = append(lines, "")
			lines = append(lines, lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("213")).
				Render("── System State ──────────────────────────────"))
			for _, l := range strings.Split(report, "\n") {
				if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "##") {
					lines = append(lines, "  "+l)
				}
			}
		}
	}
	return lines
}

func (s *SettingsOverlay) renderVerbositySection(cursor, check, uncheck lipgloss.Style) []string {
	var lines []string
	lines = append(lines, s.renderToggleRow(0, "Debug mode (show raw tool JSON)", s.debugMode, cursor, check, uncheck))
	return lines
}

func (s *SettingsOverlay) renderToolsSection(cur, check, uncheck, dim lipgloss.Style) []string {
	var lines []string
	if len(s.toolGroups) == 0 {
		lines = append(lines, dim.Render("  No tool categories registered."))
		return lines
	}
	hdr := fmt.Sprintf("  %-20s  %s", "Category", "Status")
	lines = append(lines, dim.Render(hdr))
	for i, row := range s.toolGroups {
		lines = append(lines, s.renderToggleRow(i, row.name, row.enabled, cur, check, uncheck))
	}
	return lines
}

func (s *SettingsOverlay) renderToggleRow(idx int, label string, enabled bool, cursor, check, uncheck lipgloss.Style) string {
	arrow := "  "
	if s.activeTab != tabModels && idx == s.cursor {
		arrow = cursor.Render("> ")
	}
	box := uncheck.Render("[ ]")
	if enabled {
		box = check.Render("[x]")
	}
	return fmt.Sprintf("%s%s %-22s", arrow, box, label)
}

func (s *SettingsOverlay) renderProvidersSection(cur, check, uncheck, dim lipgloss.Style) []string {
	var lines []string
	if len(s.providerRows) == 0 {
		lines = append(lines, dim.Render("  No providers registered."))
		return lines
	}
	hdr := fmt.Sprintf("  %-20s  %s", "Provider", "Status")
	lines = append(lines, dim.Render(hdr))
	for i, row := range s.providerRows {
		lines = append(lines, s.renderToggleRow(i, row.name, row.enabled, cur, check, uncheck))
	}
	lines = append(lines, "")
	lines = append(lines, dim.Render("  Disabled providers are skipped during failover cascade."))
	return lines
}

func (s *SettingsOverlay) renderIntegrationsSection(cur, dim lipgloss.Style, boxW int) []string {
	var lines []string

	// Gather current values.
	vals := map[string]string{}
	if s.integrationGetter != nil {
		vals = s.integrationGetter()
	}

	editStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true) // amber for edit mode
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	sensitiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	// Group header names (derived from key prefixes).
	lastGroup := ""
	groupOf := func(key string) string {
		switch {
		case strings.HasPrefix(key, "BUDGET_"):
			return "Budget Limits"
		case strings.HasPrefix(key, "WEBHOOK_"):
			return "Webhook Server"
		case strings.HasPrefix(key, "SCHEDULER_"):
			return "Scheduler Notifications"
		default:
			return "Other"
		}
	}

	for i, entry := range pkgconfig.IntegrationKeys {
		// Emit group header when group changes.
		if g := groupOf(entry.Key); g != lastGroup {
			lastGroup = g
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, lipgloss.NewStyle().
				Foreground(lipgloss.Color("99")).Bold(true).
				Render("  ── "+g+" ──"))
		}

		isCurrent := i == s.cursor
		arrow := "  "
		if isCurrent && !s.editingField {
			arrow = cur.Render("> ")
		} else if isCurrent && s.editingField {
			arrow = editStyle.Render("> ")
		}

		// Build value display.
		var displayVal string
		if isCurrent && s.editingField {
			// Show edit buffer with blinking cursor character.
			buf := s.editBuffer
			if entry.Sensitive {
				buf = strings.Repeat("*", len([]rune(s.editBuffer)))
			}
			displayVal = editStyle.Render("[" + buf + "█]")
		} else {
			v := vals[entry.Key]
			if v == "" {
				displayVal = dim.Render("(not set)")
			} else if entry.Sensitive {
				displayVal = sensitiveStyle.Render(strings.Repeat("*", minInt(len(v), 12)))
			} else {
				maxLen := boxW - 28
				if maxLen < 8 {
					maxLen = 8
				}
				if len(v) > maxLen {
					v = v[:maxLen-1] + "…"
				}
				displayVal = valueStyle.Render(v)
			}
		}

		label := entry.Label
		if len(label) > 22 {
			label = label[:21] + "…"
		}
		line := fmt.Sprintf("%s%-23s %s", arrow, label, displayVal)
		lines = append(lines, line)

		// Show description below selected (non-editing) row.
		if isCurrent && !s.editingField {
			lines = append(lines, descStyle.Render("     "+entry.Description))
		}
	}

	if s.integrationGetter == nil {
		lines = append(lines, "")
		lines = append(lines, dim.Render("  (settings not configured — restart required)"))
	} else {
		lines = append(lines, "")
		lines = append(lines, dim.Render("  Budget/notification changes take effect immediately."))
		lines = append(lines, dim.Render("  Webhook port changes require restart."))
	}
	return lines
}

func (s *SettingsOverlay) renderReasoningEngineSection(cursor, check, uncheck lipgloss.Style) []string {
	var lines []string
	lines = append(lines, s.renderToggleRow(0, "Step-wise Reasoning (SRE)", s.sreEnabled, cursor, check, uncheck))
	lines = append(lines, s.renderToggleRow(1, "Multi-trajectory Ensemble", s.ensembleEnabled, cursor, check, uncheck))
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("  SRE: semantic grounding, phase-aware reasoning, correction"))
	lines = append(lines, lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("  Ensemble: parallel traces with temperature variation"))
	return lines
}

func (s *SettingsOverlay) renderHITLSection(cursor, check, uncheck lipgloss.Style) []string {
	var lines []string
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// HITL master toggle
	lines = append(lines, s.renderToggleRow(0, "HITL Enabled (Safe Mode)", s.hitlEnabled, cursor, check, uncheck))

	// Risk-level bypass
	lines = append(lines, "")
	if s.cursor == 1 {
		lines = append(lines, cursor.Render("  ➜ Auto-approve risks below:"))
	} else {
		lines = append(lines, "    Auto-approve risks below:")
	}
	riskDisplay := s.hitlMinRiskLevel
	if riskDisplay == "" {
		riskDisplay = "(disabled)"
	}
	lines = append(lines, fmt.Sprintf("      %s  [low / medium / high / disabled]", riskDisplay))

	// Confidence threshold
	lines = append(lines, "")
	if s.cursor == 2 {
		lines = append(lines, cursor.Render("  ➜ Confidence threshold for auto-approval:"))
	} else {
		lines = append(lines, "    Confidence threshold for auto-approval:")
	}
	lines = append(lines, fmt.Sprintf("      %d%% (press Enter to cycle: 0→85→100)", s.hitlConfidenceThreshold))

	// Whitelisted tools
	lines = append(lines, "")
	if s.cursor == 3 {
		lines = append(lines, cursor.Render("  ➜ Whitelisted tools (bypass HITL):"))
	} else {
		lines = append(lines, "    Whitelisted tools (bypass HITL):")
	}
	if len(s.hitlWhitelistedTools) == 0 {
		lines = append(lines, dimStyle.Render("      (none — all high-stakes tools require approval)"))
	} else {
		for _, tool := range s.hitlWhitelistedTools {
			lines = append(lines, fmt.Sprintf("      • %s", tool))
		}
	}
	lines = append(lines, dimStyle.Render("      Enter cycles profiles: strict → readonly → editor → power"))

	// Disable warning toggle
	lines = append(lines, "")
	lines = append(lines, s.renderToggleRow(4, "Suppress HITL-disable warning", s.hitlDisableWarning, cursor, check, uncheck))

	// Explanatory text
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("  HITL (Human-in-the-Loop) gates destructive operations. Power users can disable"))
	lines = append(lines, dimStyle.Render("  for faster autonomous execution, or use fine-grained overrides per risk level."))

	return lines
}

func (s *SettingsOverlay) cycleHITLWhitelistProfile() string {
	idx := currentWhitelistProfileIndex(s.hitlWhitelistedTools)
	nextIdx := (idx + 1) % len(hitlWhitelistProfiles)
	next := hitlWhitelistProfiles[nextIdx]
	s.hitlWhitelistedTools = append([]string(nil), next.Tools...)
	s.saveHITLSettings()
	return next.Name
}

func currentWhitelistProfileIndex(tools []string) int {
	current := canonicalToolsKey(tools)
	for i, p := range hitlWhitelistProfiles {
		if canonicalToolsKey(p.Tools) == current {
			return i
		}
	}
	return 0
}

func canonicalToolsKey(tools []string) string {
	if len(tools) == 0 {
		return ""
	}
	sorted := append([]string(nil), tools...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

func (s *SettingsOverlay) renderEvolutionSection(cursor, check, uncheck, dim lipgloss.Style) []string {
	var lines []string

	// SECTION A: Code Evolution
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Render("▸ Code Evolution"))
	lines = append(lines, "")

	// Code Evolution Engine Toggle
	lines = append(lines, s.renderToggleRow(0, "Code Evolution Engine", s.codeEvolutionEnabled, cursor, check, uncheck))
	lines = append(lines, "")

	if s.codeEvolutionEnabled {
		// Warning banner when enabled
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Render("  ⚠  Code evolution is an experimental autonomy feature."))
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Render("      Gorkbot may modify files with appropriate human oversight."))
		lines = append(lines, "")

		// Triple Confirmation indicator
		if s.isShowingConfirmation && s.tripleConfirmationModal != nil {
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Render("  ⚡ Triple Confirmation Required"))
			lines = append(lines, s.renderTripleConfirmationModal()...)
		} else if s.cursor == 1 {
			lines = append(lines, cursor.Render("  ➜ Enable Code Evolution (requires triple confirmation)"))
		} else {
			lines = append(lines, "    Enable Code Evolution (requires triple confirmation)")
		}
		lines = append(lines, "")
	}

	// Log retention
	if s.cursor == 2 {
		lines = append(lines, cursor.Render("  ➜ Evolution log retention (days):"))
	} else {
		lines = append(lines, "    Evolution log retention (days):")
	}
	lines = append(lines, fmt.Sprintf("      %d", s.evolutionLogRetentionDays))

	// SECTION B: Free Will Engine
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Render("▸ Free Will Engine"))
	lines = append(lines, "")

	// Free Will Engine Toggle
	lines = append(lines, s.renderToggleRow(3, "Free Will Engine", s.freeWillEngineEnabled, cursor, check, uncheck))
	lines = append(lines, "")

	if s.freeWillEngineEnabled {
		// Max Risk Level
		if s.cursor == 4 {
			lines = append(lines, cursor.Render("  ➜ Max autonomous risk:"))
		} else {
			lines = append(lines, "    Max autonomous risk:")
		}
		lines = append(lines, fmt.Sprintf("      %s  [low / medium / high / none]", s.freeWillMaxRisk))
		lines = append(lines, "")

		// Confidence Threshold
		if s.cursor == 5 {
			lines = append(lines, cursor.Render("  ➜ Auto-approve confidence (0-100):"))
		} else {
			lines = append(lines, "    Auto-approve confidence (0-100):")
		}
		thresholdBar := s.renderProgressBar(s.freeWillConfidenceThreshold, 100)
		lines = append(lines, fmt.Sprintf("      %d%%  %s", s.freeWillConfidenceThreshold, thresholdBar))
		lines = append(lines, "")

		// Proposal Frequency
		if s.cursor == 6 {
			lines = append(lines, cursor.Render("  ➜ Proposal frequency:"))
		} else {
			lines = append(lines, "    Proposal frequency:")
		}
		lines = append(lines, fmt.Sprintf("      %s  [per_command / per_session / continuous]", s.freeWillProposalFreq))
		lines = append(lines, "")

		// Loop Guard Sensitivity
		if s.cursor == 7 {
			lines = append(lines, cursor.Render("  ➜ Loop guard sensitivity (0.0-1.0):"))
		} else {
			lines = append(lines, "    Loop guard sensitivity (0.0-1.0):")
		}
		sensitivityBar := s.renderProgressBar(int(s.loopGuardSensitivity*100), 100)
		lines = append(lines, fmt.Sprintf("      %.2f  %s", s.loopGuardSensitivity, sensitivityBar))
		lines = append(lines, "")

		// Rollback Window Size
		if s.cursor == 8 {
			lines = append(lines, cursor.Render("  ➜ Rollback window size (changes):"))
		} else {
			lines = append(lines, "    Rollback window size (changes):")
		}
		lines = append(lines, fmt.Sprintf("      %d  [5 / 10 / 20 / 50]", s.rollbackWindowSize))
	}

	// Explanatory text
	lines = append(lines, "")
	lines = append(lines, dim.Render("  Code Evolution allows Gorkbot to autonomously refine its own behavior within strict"))
	lines = append(lines, dim.Render("  bounds. Free Will Engine proposes improvements based on observed patterns, with loop"))
	lines = append(lines, dim.Render("  detection to prevent feedback cycles and full rollback capability."))

	return lines
}

func (s *SettingsOverlay) renderTripleConfirmationModal() []string {
	var lines []string
	if s.tripleConfirmationModal == nil {
		return lines
	}

	m := s.tripleConfirmationModal
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))

	switch m.step {
	case 1:
		lines = append(lines, warningStyle.Render("  Step 1 of 3: Copy this code:"))
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46")).Render(fmt.Sprintf("    %s", m.randomCode)))
		lines = append(lines, warningStyle.Render("  Enter it below to proceed:"))
		lines = append(lines, fmt.Sprintf("    > %s", m.userInput))

	case 2:
		lines = append(lines, warningStyle.Render("  Step 2 of 3: Type this phrase:"))
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46")).Render(fmt.Sprintf("    %s", m.expectedPhrase)))
		lines = append(lines, warningStyle.Render("  Enter it below:"))
		lines = append(lines, fmt.Sprintf("    > %s", m.userInput))

	case 3:
		lines = append(lines, warningStyle.Render("  Step 3 of 3: Final confirmation"))
		lines = append(lines, warningStyle.Render("  Type 'YES' (uppercase) to enable code evolution:"))
		lines = append(lines, fmt.Sprintf("    > %s", m.userInput))
	}

	if m.coolingDown {
		remaining := time.Until(m.coolingDownUntil).Seconds()
		lines = append(lines, fmt.Sprintf("  Cooldown: %.0f seconds remaining", remaining))
	}
	if m.failedAttempts > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).
			Render(fmt.Sprintf("  Failed attempts: %d/3 (3 strikes = 30s lockout)", m.failedAttempts)))
	}

	return lines
}

func (s *SettingsOverlay) renderSystemMonitorSection(cursor, check, uncheck, dim lipgloss.Style) []string {
	var lines []string

	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Render("▸ System Monitor Configuration"))
	lines = append(lines, "")

	// Master enable/disable
	lines = append(lines, s.renderToggleRow(0, "System Monitor Enabled", s.systemMonitorEnabled, cursor, check, uncheck))
	lines = append(lines, "")

	if s.systemMonitorEnabled {
		// Manual-only mode
		lines = append(lines, s.renderToggleRow(1, "Manual-Only Mode", s.systemMonitorManualOnly, cursor, check, uncheck))
		if s.systemMonitorManualOnly {
			lines = append(lines, dim.Render("  (only run when explicitly requested)"))
		} else {
			lines = append(lines, dim.Render("  (auto-run on heartbeat when idle)"))
		}
		lines = append(lines, "")

		// Cooldown minutes
		if s.cursor == 2 {
			lines = append(lines, cursor.Render("  ➜ Cooldown between auto-runs (minutes):"))
		} else {
			lines = append(lines, "    Cooldown between auto-runs (minutes):")
		}
		lines = append(lines, fmt.Sprintf("      %d  [30 / 60 / 90 / 120]", s.systemMonitorCooldownMins))
		lines = append(lines, "")

		// Alert threshold
		if s.cursor == 3 {
			lines = append(lines, cursor.Render("  ➜ Resource alert threshold (%):"))
		} else {
			lines = append(lines, "    Resource alert threshold (%):")
		}
		lines = append(lines, fmt.Sprintf("      %d%%  [20 / 30 / 50 / 70]", s.systemMonitorAlertThreshold))
	}

	// Explanatory text
	lines = append(lines, "")
	lines = append(lines, dim.Render("  System Monitor checks disk, memory, and CPU usage. Configure here to prevent"))
	lines = append(lines, dim.Render("  excessive resource polling, which can impact performance on mobile devices."))

	return lines
}

func (s *SettingsOverlay) renderProgressBar(value, max int) string {
	if max == 0 {
		return ""
	}
	pct := minInt(100, maxInt(0, (value*100)/max))
	filledBlocks := pct / 10
	emptyBlocks := 10 - filledBlocks
	return lipgloss.NewStyle().Foreground(lipgloss.Color("76")).Render(
		strings.Repeat("█", filledBlocks) + strings.Repeat("░", emptyBlocks))
}

// min is a helper for int minimum (Go 1.21 has built-in but keeping compat).
