package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/dag"
	"github.com/velariumai/gorkbot/pkg/tools"
	"github.com/velariumai/gorkbot/pkg/tui/hotkeys"
)

// Update handles all messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Global Quit handler
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(keyMsg, m.keymap.Quit) {
			m.quitting = true
			return m, tea.Quit
		}
	}

	// 1a. API Key Prompt — intercepts ALL key events when active, regardless of view state.
	// Non-key messages (window resize, ticks, etc.) fall through so the UI stays responsive.
	if m.apiKeyPrompt.active {
		if _, isKey := msg.(tea.KeyMsg); isKey {
			return m.updateAPIKeyPrompt(msg)
		}
	}

	// 1b. Dynamic Auth Wizard — intercepts key events when awaiting credentials
	if m.awaitingAuth {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.Type {
			case tea.KeyEnter:
				val := m.authInput.Value()
				if m.authRequest != nil && m.authRequest.ResponseChan != nil {
					m.authRequest.ResponseChan <- val
				}
				m.awaitingAuth = false
				return m, nil
			case tea.KeyEsc:
				if m.authRequest != nil && m.authRequest.ResponseChan != nil {
					m.authRequest.ResponseChan <- ""
				}
				m.awaitingAuth = false
				return m, nil
			}
			var cmd tea.Cmd
			m.authInput, cmd = m.authInput.Update(msg)
			return m, cmd
		}
	}

	// 1. Overlay Handling (Modal Priority)
	if m.activeOverlay != nil {
		newOverlay, cmd := m.activeOverlay.Update(msg)
		cmds = append(cmds, cmd)

		if newOverlay == nil {
			// Overlay requested close
			m.activeOverlay = nil
			// Refocus textarea when overlay closes
			m.textarea.Focus()
		} else {
			m.activeOverlay = newOverlay
			// Return early if overlay consumes input, except for some global signals
			// But we might want status bar updates to pass through?
			// For now, let's block interaction with main UI when overlay is active
			return m, tea.Batch(cmds...)
		}
	}

	// 1.5 Global Message Handling (Processed across all views)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		_, cmd := m.handleWindowSize(msg)
		cmds = append(cmds, cmd)
	case APIKeySavedMsg:
		_, cmd := m.handleAPIKeySaved(msg)
		cmds = append(cmds, cmd)
	case ModelRefreshMsg:
		if msg.Err == nil {
			var kept []commands.ModelInfo
			for _, am := range m.availableModels {
				if am.Provider != msg.Provider {
					kept = append(kept, am)
				}
			}
			m.availableModels = append(kept, msg.Models...)
			m.refreshModelSelectLists()
		}
		if m.modelSelect.refreshing != nil {
			m.modelSelect.refreshing[msg.Provider] = false
		}
	case ModelSwitchedMsg:
		if msg.Role == "primary" {
			m.addSystemMessage(fmt.Sprintf("Primary switched to **%s** (%s)", msg.ModelID, msg.Provider))
			m.currentModel = msg.ModelID
		} else {
			m.addSystemMessage(fmt.Sprintf("Secondary switched to **%s** (%s)", msg.ModelID, msg.Provider))
		}
		m.statusBar.UpdateState(msg.ModelID, m.analytics.TotalTokens, nil)
		m.refreshModelSelectLists()
	case ProviderStatusMsg:
		m.modelSelect.providerKeys = msg.Statuses
	case ProviderPollTickMsg:
		cmds = append(cmds, m.pollAllConfiguredProviders())
		cmds = append(cmds, providerPollTick())
	case DiscoveryPollTickMsg:
		if m.discoverySub != nil {
			select {
			case models := <-m.discoverySub:
				m.discoveredModels = models
			default:
			}
		}
		cmds = append(cmds, discoveryPollTick())
	case DiscoveryUpdateMsg:
		m.discoveredModels = msg.Models
	case LightGlistenTickMsg:
		m.glistenPos += 0.025
		if m.glistenPos >= 1.0 {
			m.glistenPos = 0.0
		}
		m.spotlightPos += 0.05
		if m.spotlightPos > 1.2 {
			m.spotlightPos = -0.2
		}
		if !m.splashDone {
			cmds = append(cmds, glistenTick()) // keep animating splash
		}
		// After splash is dismissed, stop scheduling ticks — zero wasted cycles.

	case sidePanelTickMsg:
		if m.sidePanelOpen {
			cmds = append(cmds, sidePanelTick()) // keep polling while open
		}

	case ToastMsg:
		// All queue management (priority sort, dedup, ID-based update) is in pushToast.
		if cmd := m.pushToast(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case toastDismissTickMsg:
		now := time.Now()
		kept := m.toasts[:0]
		for _, t := range m.toasts {
			// KindPersistent toasts have zero ExpiresAt and are never auto-dismissed.
			if t.ExpiresAt.IsZero() || t.ExpiresAt.After(now) {
				kept = append(kept, t)
			}
		}
		prevLen := len(m.toasts)
		m.toasts = kept
		if len(m.toasts) > 0 {
			cmds = append(cmds, toastDismissTick())
		}
		// Recalc on any count change — each visible toast occupies 1 line (max 3).
		if len(m.toasts) != prevLen {
			m.recalcViewportHeight()
		}
	}

	// 2. Main UI State Handling
	// Bookmark overlay intercepts key events when open
	if m.bookmarkOverlay {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			return m.updateBookmarkOverlay(keyMsg)
		}
	}

	// DAG view messages are handled regardless of current state so events keep
	// flowing even if the user briefly navigates away.
	switch msg := msg.(type) {
	case DAGEventMsg, DAGTickMsg:
		// Intercept DAGEventMsg to feed the unified hook output tree
		if evMsg, ok := msg.(DAGEventMsg); ok && m.dagVM != nil && m.dagVM.graph != nil {
			task := m.dagVM.graph.Get(evMsg.TaskID)
			if task != nil {
				hookID := "dag_" + evMsg.TaskID
				switch evMsg.Status {
				case dag.StatusRunning:
					if findHook(m.activeHooks, hookID) == nil {
						cmds = append(cmds, func() tea.Msg {
							return HookStartMsg{
								ID:    hookID,
								Icon:  "⚡",
								Label: task.Description,
							}
						})
					} else if evMsg.Progress > 0 {
						cmds = append(cmds, func() tea.Msg {
							return HookUpdateMsg{
								ID:       hookID,
								Metadata: fmt.Sprintf("%.0f%%", evMsg.Progress*100),
							}
						})
					}
				case dag.StatusCompleted:
					cmds = append(cmds, func() tea.Msg {
						return HookDoneMsg{
							ID:      hookID,
							Success: true,
							Elapsed: time.Since(task.StartedAt),
						}
					})
				case dag.StatusFailed, dag.StatusSkipped:
					cmds = append(cmds, func() tea.Msg {
						return HookDoneMsg{
							ID:      hookID,
							Success: false,
							Elapsed: time.Since(task.StartedAt),
							Summary: evMsg.ErrMsg,
						}
					})
				}
			}
		}

		model, cmd := m.updateDAGView(msg)
		cmds = append(cmds, cmd)
		return model, tea.Batch(cmds...)
	}

	// ── Extended thinking panel messages ───────────────────────────────────
	switch msg := msg.(type) {
	case thinkingResetMsg:
		m.thinkingBuf.Reset()
		m.thinkingActive = false
		return m, nil
	case ThinkingTokenMsg:
		m.thinkingBuf.WriteString(msg.Content)
		m.thinkingActive = true
		m.genPhase = phaseThinking
		return m, nil
	case ThinkingDoneMsg:
		m.thinkingActive = false
		// Keep buffer visible until StreamCompleteMsg clears it.
		return m, nil
	}

	switch m.state {
	case analyticsView:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc", "ctrl+a", "ctrl+h":
				m.state = chatView
				return m, nil
			}
		}
	case dagView:
		model, cmd := m.updateDAGView(msg)
		cmds = append(cmds, cmd)
		return model, tea.Batch(cmds...)
	case modelSelectView: // also handles modelListView (same const)
		model, cmd := m.updateModelSelectView(msg)
		cmds = append(cmds, cmd)
		return model, tea.Batch(cmds...)
	case toolsTableView:
		model, cmd := m.updateToolsTableState(msg)
		cmds = append(cmds, cmd)
		return model, tea.Batch(cmds...)
	case discoveryView:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if m.discTestActive {
				// Test-prompt input mode
				switch keyMsg.String() {
				case "esc":
					m.discTestActive = false
					m.discTestInput = ""
					m.discTestResult = ""
				case "enter":
					// Submit test prompt
					if m.discTestInput != "" && m.orchestrator != nil {
						m.discTestResult = "Sending test prompt..."
						// We'll signal a one-shot query via the command pipeline
						m.handleCommand("DISC_TEST_PROMPT:" + m.discTestInput)
					}
				case "backspace":
					if len(m.discTestInput) > 0 {
						m.discTestInput = m.discTestInput[:len(m.discTestInput)-1]
					}
				default:
					if len(keyMsg.Runes) > 0 {
						m.discTestInput += string(keyMsg.Runes)
					}
				}
				return m, nil
			}
			switch keyMsg.String() {
			case "esc":
				m.state = chatView
			case "up", "k":
				if m.discCursor > 0 {
					m.discCursor--
				}
			case "down", "j":
				if m.discCursor < len(m.discoveredModels)-1 {
					m.discCursor++
				}
			case "enter":
				if m.discCursor < len(m.discoveredModels) {
					mod := m.discoveredModels[m.discCursor]
					signal := fmt.Sprintf("MODEL_SWITCH_PRIMARY:%s:%s", mod.Provider, mod.ID)
					m.handleCommand(signal)
					m.state = chatView
				}
			case "s":
				if m.discCursor < len(m.discoveredModels) {
					mod := m.discoveredModels[m.discCursor]
					signal := fmt.Sprintf("MODEL_SWITCH_SECONDARY:%s:%s", mod.Provider, mod.ID)
					m.handleCommand(signal)
					m.state = chatView
				}
			case "t":
				m.discTestActive = true
				m.discTestInput = ""
				m.discTestResult = ""
			}
			return m, nil
		}
	}

	// 3. Chat View Handling (Default)
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseMsg:
		// Handle mouse clicks to restore focus/keyboard (always handle this)
		if msg.Type == tea.MouseLeft {
			m.textarea.Focus()
			// Also let viewport handle the click
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}

		// Let viewport handle mouse events (wheel, drag, etc.)
		// This enables both mouse wheel and touch drag scrolling
		var cmd tea.Cmd
		oldYOffset := m.viewport.YOffset
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

		// Track scroll state based on position change
		if m.viewport.YOffset < oldYOffset {
			// Scrolled up
			m.userScrolledUp = true
		} else if m.viewport.AtBottom() {
			// At bottom
			m.userScrolledUp = false
		}

	case tea.KeyMsg:
		_, cmd = m.handleKeyMsg(msg)
		cmds = append(cmds, cmd)

	case hotkeys.Command:
		switch msg {
		case hotkeys.CmdToolsMenu:
			m.state = toolsTableView
			m.updateToolsTable()
			return m, nil
		case hotkeys.CmdSettings:
			var debugOn bool
			if m.commands != nil && m.commands.Orch != nil {
				debugOn = m.debugMode
			}
			var appStateSetter func(cats []string) error
			var providerSetter func(ids []string) error
			if m.commands != nil && m.commands.Orch != nil {
				if m.commands.Orch.PersistDisabledCategories != nil {
					appStateSetter = m.commands.Orch.PersistDisabledCategories
				}
				if m.commands.Orch.PersistDisabledProviders != nil {
					providerSetter = m.commands.Orch.PersistDisabledProviders
				}
			}
			var toolReg *tools.Registry
			if m.commands != nil {
				toolReg = m.commands.GetToolRegistry()
			}
			var orchAdapter *commands.OrchestratorAdapter
			if m.commands != nil {
				orchAdapter = m.commands.Orch
			}
			m.activeOverlay = NewSettingsOverlay(m.width, m.height, orchAdapter, toolReg, appStateSetter, debugOn, providerSetter, m.integrationGetter, m.integrationSetter)
			return m, nil
		case hotkeys.CmdModelsSelect:
			m.state = modelListView
			m.initModelSelectState()
			m.updateProviderKeyStatuses()
			return m, m.updateModelListItems()
		case hotkeys.CmdContextStatus:
			if m.state == diagnosticsView {
				m.state = chatView
			} else {
				m.state = diagnosticsView
			}
			return m, nil
		case hotkeys.CmdDebugToggle:
			if m.orchestrator != nil {
				active := m.orchestrator.ToggleDebug()
				m.addSystemMessage(fmt.Sprintf("Developer Debug Mode: %v", active))
				m.updateViewportContent()
			}
			return m, nil
		case hotkeys.CmdHistorySearch:
			if !m.generating {
				m.histSearchMode = true
				m.histSearchQuery = ""
				m.rebuildHistMatches()
			}
			return m, nil

		case hotkeys.CmdForceRefresh:
			hotkeys.ClearTerminal()
			m.updateViewportContent()
			return m, tea.ClearScreen
		case hotkeys.CmdClearReset:
			m.orchestrator.ClearHistory()
			m.addSystemMessage("Conversation history cleared.")
			m.updateViewportContent()
			return m, nil
		case hotkeys.CmdDuplicateSess:
			if m.orchestrator != nil {
				res := m.orchestrator.SaveSession("")
				m.addSystemMessage(fmt.Sprintf("Session duplicated/saved: %s", res))
				m.updateViewportContent()
			}
			return m, nil

		case hotkeys.CmdExpandDetails:
			// Toggle collapse state of the most recent tool/tool_call/internal/a2a message.
			for i := len(m.messages) - 1; i >= 0; i-- {
				role := m.messages[i].Role
				if role == "tool" || role == "tool_call" || role == "internal" || role == "a2a" {
					m.messages[i].Collapsed = !m.messages[i].Collapsed
					m.updateViewportContent()
					// Scroll to show the newly expanded content.
					if !m.messages[i].Collapsed {
						m.userScrolledUp = false
						m.viewport.GotoBottom()
					}
					break
				}
			}
			return m, nil

		case hotkeys.CmdExpandAll:
			// Toggle ALL collapsible messages: if any are expanded → collapse all;
			// if all are collapsed → expand all.
			hasExpanded := false
			for _, msg := range m.messages {
				role := msg.Role
				if (role == "tool" || role == "tool_call" || role == "internal" || role == "a2a") && !msg.Collapsed {
					hasExpanded = true
					break
				}
			}
			for i, msg := range m.messages {
				role := msg.Role
				if role == "tool" || role == "tool_call" || role == "internal" || role == "a2a" {
					m.messages[i].Collapsed = hasExpanded // collapse if any open, else expand
				}
			}
			m.updateViewportContent()
			return m, nil

		case hotkeys.CmdOmniSearch:
			// Toggle search mode. When opening, reset query and matches.
			if m.searchMode {
				m.searchMode = false
				m.searchQuery = ""
				m.searchMatches = nil
				m.searchMatchIdx = 0
				m.updateViewportContent()
			} else {
				m.searchMode = true
				m.searchQuery = ""
				m.searchMatches = nil
				m.searchMatchIdx = 0
				// Unfocus textarea so key events reach handleSearchInput.
				m.textarea.Blur()
			}
			return m, nil

		case hotkeys.CmdExportJSON:
			// Export full session as a JSON file with auto-generated name.
			jsonContent := m.exportAsJSON()
			if jsonContent == "" {
				m.addSystemMessage("**Export failed**: could not serialise session.")
				m.updateViewportContent()
				return m, nil
			}
			home, _ := os.UserHomeDir()
			path := fmt.Sprintf("%s/gorkbot_export_%s.json", home, time.Now().Format("20060102_150405"))
			if err := writeExportFile(path, jsonContent); err != nil {
				m.addSystemMessage(fmt.Sprintf("**Export failed**: %v", err))
			} else {
				m.addSystemMessage(fmt.Sprintf("Session exported to `%s` (%d messages).", path, len(m.messages)))
			}
			m.updateViewportContent()
			return m, nil

		case hotkeys.CmdHardQuit:
			m.quitting = true
			return m, tea.Quit
		}

	case TokenMsg:
		_, cmd = m.handleTokenMsg(msg)
		cmds = append(cmds, cmd)

	case PlanningTokenMsg:
		_, cmd = m.handlePlanningTokenMsg(msg)
		cmds = append(cmds, cmd)

	case PlanningBoxClearMsg:
		_, cmd = m.handlePlanningBoxClear(msg)
		cmds = append(cmds, cmd)

	case PlanningCommitMsg:
		_, cmd = m.handlePlanningCommit(msg)
		cmds = append(cmds, cmd)

	case PlanningTickMsg:
		_, cmd = m.handlePlanningTick(msg)
		cmds = append(cmds, cmd)

	case spinner.TickMsg:
		_, cmd = m.handleSpinnerTick(msg)
		cmds = append(cmds, cmd)

	case PhraseTickMsg:
		_, cmd = m.handlePhraseTick(msg)
		cmds = append(cmds, cmd)

	case ErrorMsg:
		_, cmd = m.handleErrorMsg(msg)
		cmds = append(cmds, cmd)

	case StartGenerationMsg:
		_, cmd = m.handleStartGeneration(msg)
		cmds = append(cmds, cmd)

	case EndGenerationMsg:
		_, cmd = m.handleEndGeneration(msg)
		cmds = append(cmds, cmd)

	case ClearScreenMsg:
		_, cmd = m.handleClearScreen(msg)
		cmds = append(cmds, cmd)

	case QuitMsg:
		_, cmd = m.handleQuit(msg)
		cmds = append(cmds, cmd)

	case StreamCompleteMsg:
		_, cmd = m.handleStreamComplete(msg)
		cmds = append(cmds, cmd)

	case ToolCallMsg:
		// Render the outgoing tool call request box before the result arrives.
		m.addToolCallMessage(msg.ToolName, msg.Params)
		// Start tracking this tool in the activity panel.
		icon, label := toolActivityLabel(msg.ToolName, msg.Params)
		m.currentActivity = &activityEntry{Icon: icon, Label: label, StartedAt: time.Now()}
		m.genPhase = phaseTool
		// ── Hook: emit HookStartMsg inline ───────────────────────────────────
		hookMsg := HookStartMsg{
			ID:    msg.ToolName + "_" + fmt.Sprintf("%d", time.Now().UnixNano()),
			Icon:  icon,
			Label: label,
		}
		// Store the hook ID so ToolProgressMsg can finalise it.
		if m.toolStartTimes == nil {
			m.toolStartTimes = make(map[string]time.Time)
		}
		m.toolStartTimes[msg.ToolName] = time.Now()
		// Process the hook start directly (avoids tea.Cmd round-trip).
		entry := HookEntry{
			ID:        hookMsg.ID,
			Icon:      hookMsg.Icon,
			Label:     hookMsg.Label,
			Active:    true,
			CreatedAt: time.Now(),
		}
		if len(m.activeHooks) >= 32 {
			m.activeHooks = m.activeHooks[1:]
		}
		m.activeHooks = append(m.activeHooks, entry)
		if !m.hookTickActive {
			m.hookTickActive = true
			cmds = append(cmds, hookTick())
		}
		m.recalcViewportHeight()
		m.updateViewportContent()
		if !m.userScrolledUp {
			m.viewport.GotoBottom()
		}

	case ToolExecutionMsg:
		_, cmd = m.handleToolExecution(msg)
		cmds = append(cmds, cmd)

	case InterventionRequestMsg:
		_, cmd = m.handleInterventionRequest(msg)
		cmds = append(cmds, cmd)

	case HITLRequestMsg:
		_, cmd = m.handleHITLRequest(msg)
		cmds = append(cmds, cmd)

	case PermissionRequestMsg:
		m.permissionPrompt = NewPermissionPrompt(msg.ToolName, msg.Description, msg.Params)
		m.awaitingPermission = true
		m.permissionChan = msg.ResponseChan
		// No cmd

	case AuthRequestMsg:
		m.authRequest = &msg
		m.awaitingAuth = true
		m.authInput.Placeholder = "Enter credential for " + msg.ToolName
		m.authInput.Focus()
		m.authInput.SetValue("")
		// No cmd

	case CompletionMsg:
		// Append completion to textarea
		current := m.textarea.Value()
		m.textarea.SetValue(current + msg.Content)
		// Move cursor to end
		m.textarea.CursorEnd()

	// ── Enhanced message handlers ──────────────────────────────────────────

	case ModeChangeMsg:
		// Reflect mode change from orchestrator (e.g., via /mode command)
		m.statusBar.SetMode(msg.ModeName)

	case InterruptMsg:
		// Programmatic interrupt (e.g., from hook or orchestrator)
		if m.generating {
			if m.orchestrator != nil {
				m.orchestrator.Interrupt()
			}
			m.generating = false
			m.planningActive = false
			m.planningBuf.Reset()
			m.planningIntent = ""
			m.currentResponse.Reset()
			m.streamChunkCount = 0
			m.recalcViewportHeight()
			m.addSystemMessage("_Generation interrupted_")
			m.updateViewportContent()
			if !m.userScrolledUp {
				m.viewport.GotoBottom()
			}
			m.textarea.Blur()
			m.textarea.Focus()
		}

	case ToolProgressMsg:
		// Track tool usage in analytics.
		if msg.Done && m.analytics != nil {
			m.analytics.RecordToolUse(msg.ToolName)
		}
		if msg.Done {
			// Remove the tool from the live panel.
			for i, t := range m.activeTools {
				if t == msg.ToolName {
					m.activeTools = append(m.activeTools[:i], m.activeTools[i+1:]...)
					break
				}
			}
			// ── Hook: seal the most-recent active hook for this tool ──────────
			elapsedD := time.Duration(msg.Elapsed * float64(time.Second))
			// Walk backwards through activeHooks to find the matching entry.
			for i := len(m.activeHooks) - 1; i >= 0; i-- {
				if m.activeHooks[i].Active && strings.HasPrefix(m.activeHooks[i].ID, msg.ToolName+"_") {
					m.activeHooks[i].Active = false
					m.activeHooks[i].IsFinal = true
					m.activeHooks[i].IsError = !msg.Success
					m.activeHooks[i].Elapsed = elapsedD
					m.activeHooks[i].Metadata = formatElapsed(elapsedD)
					break
				}
			}
		} else {
			// Add the tool to the live panel and record start time.
			m.activeTools = append(m.activeTools, msg.ToolName)
			if m.toolStartTimes != nil {
				m.toolStartTimes[msg.ToolName] = time.Now()
			}
		}
		m.statusBar.SetActiveTools(len(m.activeTools))

	// ── Consultation pipeline → hook integration ─────────────────────────────
	case ConsultationStageMsg:
		stageLabel := consultationStageLabel(msg.Stage)
		if msg.Detail != "" {
			stageLabel += ": " + msg.Detail
		}
		hookID := fmt.Sprintf("consult_stage_%d", int(msg.Stage))
		// Seal previous consultation stage hook if any.
		for i := range m.activeHooks {
			if strings.HasPrefix(m.activeHooks[i].ID, "consult_stage_") && m.activeHooks[i].Active {
				m.activeHooks[i].Active = false
				m.activeHooks[i].IsFinal = true
				elapsed := time.Since(m.activeHooks[i].CreatedAt)
				m.activeHooks[i].Elapsed = elapsed
				m.activeHooks[i].Metadata = formatElapsed(elapsed)
			}
		}
		entry := HookEntry{
			ID:        hookID,
			Icon:      "◐",
			Label:     stageLabel,
			Active:    true,
			CreatedAt: time.Now(),
		}
		if len(m.activeHooks) >= 32 {
			m.activeHooks = m.activeHooks[1:]
		}
		m.activeHooks = append(m.activeHooks, entry)
		if !m.hookTickActive {
			m.hookTickActive = true
			cmds = append(cmds, hookTick())
		}

	case ConsultationDoneMsg:
		// Seal the last active consultation hook.
		for i := len(m.activeHooks) - 1; i >= 0; i-- {
			if strings.HasPrefix(m.activeHooks[i].ID, "consult_stage_") && m.activeHooks[i].Active {
				m.activeHooks[i].Active = false
				m.activeHooks[i].IsFinal = true
				elapsed := time.Since(m.activeHooks[i].CreatedAt)
				m.activeHooks[i].Elapsed = elapsed
				suffix := ""
				if msg.FromCache {
					suffix = " (cache hit)"
				} else if msg.Retries > 0 {
					suffix = fmt.Sprintf(" (%d retries)", msg.Retries)
				}
				m.activeHooks[i].Metadata = formatElapsed(elapsed) + suffix
				break
			}
		}

	case ConsultationErrorMsg:
		// Seal as error.
		for i := len(m.activeHooks) - 1; i >= 0; i-- {
			if strings.HasPrefix(m.activeHooks[i].ID, "consult_stage_") && m.activeHooks[i].Active {
				m.activeHooks[i].Active = false
				m.activeHooks[i].IsFinal = true
				m.activeHooks[i].IsError = true
				m.activeHooks[i].Metadata = "failed"
				break
			}
		}

	case ProcessOutputMsg:
		// Stream process output to the chat
		if msg.Done {
			// Process completed - show final status
			prefix := "✅"
			if msg.ExitCode != 0 {
				prefix = "❌"
			}
			m.addSystemMessage(fmt.Sprintf("%s Process **%s** completed (exit code: %d)", prefix, msg.ProcessID, msg.ExitCode))
		} else if msg.Output != "" {
			// Stream output to chat
			prefix := ""
			if msg.IsStderr {
				prefix = "⚠️ "
			}
			m.addSystemMessage(fmt.Sprintf("%s`%s`: %s", prefix, msg.ProcessID, msg.Output))
		}
		m.updateViewportContent()
		if !m.userScrolledUp {
			m.viewport.GotoBottom()
		}

	case RewindCompleteMsg:
		m.addSystemMessage(fmt.Sprintf(
			"_Rewound to: **%s** — conversation restored to %d messages_",
			msg.Description, msg.MsgCount,
		))
		m.updateViewportContent()
		if !m.userScrolledUp {
			m.viewport.GotoBottom()
		}

	// ── Hook output messages (Claude Code-style) ───────────────────────────
	case HookStartMsg:
		entry := HookEntry{
			ID:        msg.ID,
			Icon:      msg.Icon,
			Label:     msg.Label,
			Active:    true,
			CreatedAt: time.Now(),
		}
		if entry.Icon == "" {
			entry.Icon = "⚙"
		}
		if msg.ParentID == "" {
			// Top-level: cap the list at 32 entries to prevent memory leak.
			if len(m.activeHooks) >= 32 {
				m.activeHooks = m.activeHooks[1:]
			}
			m.activeHooks = append(m.activeHooks, entry)
		} else {
			// Nested: find parent and append child (depth already checked by sender).
			if ptr := findHook(m.activeHooks, msg.ParentID); ptr != nil {
				if ptr.Depth < maxHookDepth-1 {
					entry.Depth = ptr.Depth + 1
					ptr.SubEntries = append(ptr.SubEntries, entry)
				}
			} else {
				// Parent not found: add as top-level fallback.
				m.activeHooks = append(m.activeHooks, entry)
			}
		}
		// Start tick loop if not already running.
		if !m.hookTickActive {
			m.hookTickActive = true
			cmds = append(cmds, hookTick())
		}
		m.recalcViewportHeight()

	case HookUpdateMsg:
		if ptr := findHook(m.activeHooks, msg.ID); ptr != nil {
			if msg.Metadata != "" {
				ptr.Metadata = msg.Metadata
			}
			if msg.Output != "" {
				if msg.AppendOutput {
					// Keep only first 2 lines worth of output.
					combined := ptr.Output + msg.Output
					lines := strings.SplitN(combined, "\n", 4)
					if len(lines) > 2 {
						lines = lines[:2]
					}
					ptr.Output = strings.Join(lines, "\n")
				} else {
					ptr.Output = msg.Output
				}
			}
		}

	case HookDoneMsg:
		if ptr := findHook(m.activeHooks, msg.ID); ptr != nil {
			ptr.Active = false
			ptr.IsFinal = true
			ptr.IsError = !msg.Success
			ptr.Elapsed = msg.Elapsed
			if msg.Summary != "" {
				ptr.Metadata = msg.Summary
			}
		}

	case HookCollapseMsg:
		if ptr := findHook(m.activeHooks, msg.ID); ptr != nil {
			ptr.Collapsed = !ptr.Collapsed
		}

	case AtCompleteResultMsg:
		if msg.Query == m.atCompleteQuery {
			m.atCompleteItems = msg.Items
			m.atCompleteIdx = 0
		}

	case HookTickMsg:
		// Advance spinner frame.
		m.hookSpinFrame++
		// Re-schedule only while generating (avoids idle CPU cost).
		if m.generating || hasActiveHooks(m.activeHooks) {
			cmds = append(cmds, hookTick())
			// Debouncer: avoid tearing view if tokens are actively streaming in
			if time.Since(m.lastTokenTime) > 100*time.Millisecond {
				m.updateViewportContent()
			}
		} else {
			m.hookTickActive = false
		}
	}

	// Update child components if not generating
	// During intervention mode, we block textarea updates to focus on the prompt
	if !m.generating && !m.interventionMode && !m.awaitingHITL && m.activeOverlay == nil {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	var cCmd tea.Cmd
	m.consultantSpinner, cCmd = m.consultantSpinner.Update(msg)
	cmds = append(cmds, cCmd)

	// Update Status Bar
	// Sync active processes
	if m.processManager != nil {
		liveToks := m.liveTokenCount
		if m.analytics != nil {
			// Include live tokens in the analytics view display as well
			// This ensures the Context Window gauge moves in real-time
			actualUsed := m.analytics.ContextUsedToks + liveToks
			if m.analytics.ContextMaxToks > 0 {
				m.analytics.ContextUsedPct = float64(actualUsed) / float64(m.analytics.ContextMaxToks)
			}
			
			// Update session total and rate sparkline in real-time
			// We use the cumulative session total (ContextUsedToks is per-session historically)
			// totalTokensSum := m.analytics.TotalTokens (already includes previous turns)
			// But RecordTokens expects the ABSOLUTE total.
			// m.analytics.TotalTokens is updated at end of turn.
			// Let's just update the analytics fields directly.
			if liveToks > 0 {
				m.analytics.RecordTokens(m.analytics.TotalTokens+liveToks, m.analytics.SessionCostUS)
			}
			
			liveToks += m.analytics.ContextUsedToks
		}
		m.statusBar.UpdateState(
			m.currentModel,
			liveToks,
			m.processManager.ListProcesses(),
		)
	}
	// Update status bar model
	var sbCmd tea.Cmd
	m.statusBar, sbCmd = m.statusBar.Update(msg)
	cmds = append(cmds, sbCmd)

	return m, tea.Batch(cmds...)
}

// updateModelList handles updates when in model selection mode
func (m *Model) updateModelList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.Back) {
			// If filtering, let the list handle the key to clear filter
			if m.modelList.FilterState() == list.Filtering {
				break
			}
			m.state = chatView
			return m, nil
		}
		if key.Matches(msg, m.keymap.Select) {
			selected := m.getSelectedModel()
			if selected != nil {
				// Switch primary model by default
				if err := m.switchPrimaryModel(selected.id); err != nil {
					m.addSystemMessage(fmt.Sprintf("❌ Failed to switch model: %v", err))
				} else {
					m.addSystemMessage(fmt.Sprintf("✅ Switched to **%s**", selected.name))
				}
				m.state = chatView
				m.updateViewportContent()
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.handleWindowSize(msg)
	}

	m.modelList, cmd = m.modelList.Update(msg)
	return m, cmd
}

// updateToolsTableState handles updates when in tools table mode
func (m *Model) updateToolsTableState(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.Back) {
			m.state = chatView
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.handleWindowSize(msg)
	}

	var tableModel tea.Model
	tableModel, cmd = m.toolsTable.Update(msg)
	m.toolsTable = tableModel.(TableModel)
	return m, cmd
}

// handleWindowSize handles window resize events
func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	if m.width < minWidth {
		m.width = minWidth
	}
	if m.height < minHeight {
		m.height = minHeight
	}

	// Update list size
	m.modelList.SetSize(msg.Width, msg.Height)

	// Update table height - fill available space but leave room for header/footer if any
	// bubbles/table doesn't auto-resize width well, we might need to manually set column widths
	// For now, just set height
	tableHeight := msg.Height - 4 // Leave some margin
	if tableHeight < 5 {
		tableHeight = 5
	}
	m.toolsTable.table.SetHeight(tableHeight)

	// Fixed layout calculation (like Claude Code / Gemini CLI)
	// Components from top to bottom:
	// 0. Branding Text (1 line)
	// 1. Tab Bar (3 lines: tabs + border)
	// 2. Viewport (chat history) - takes most space
	// 3. Separator line - 1 line
	// 4. Loading indicator (if generating) - 4 lines (multi-line spinner)
	// 5. Input area (textarea + help) - 5 lines
	// 6. Status bar - 1 line

	const (
		brandingHeight  = 0 // header moved to splash screen
		tabsHeight      = 2 // Text + Border
		separatorHeight = 1
		loadingHeight   = 4 // 4-line Block G spinner
		inputHeight     = 5 // textarea (3 lines) + help (1 line) + spacing
		statusBarHeight = 1
	)

	// Each visible toast occupies 1 line; at most 3 are rendered at once.
	notifyHeight := min(len(m.toasts), 3)
	fixedComponentsHeight := brandingHeight + tabsHeight + separatorHeight + inputHeight + statusBarHeight + notifyHeight
	if m.generating {
		fixedComponentsHeight += loadingHeight
	}

	viewportHeight := m.height - fixedComponentsHeight
	if viewportHeight < 10 {
		viewportHeight = 10
	}

	// Update viewport dimensions (account for side panel).
	if m.sidePanelOpen {
		m.viewport.Width = m.width - m.sidePanelWidth - 1
	} else {
		m.viewport.Width = m.width
	}
	m.viewport.Height = viewportHeight
	// recalcViewportHeight() uses the same logic; keep in sync via that helper
	// for any call sites outside of resize (e.g., stream completion, ESC cancel).

	// Update textarea width
	m.textarea.SetWidth(m.width - 2)

	// Update help width
	m.help.Width = m.width

	// Update status bar width
	m.statusBar.SetDimensions(m.width, statusBarHeight)

	// Only recreate the glamour renderer when the terminal width actually changes.
	// glamour.NewTermRenderer is an expensive allocation; firing it on every
	// WindowSizeMsg (Bubble Tea sends these frequently) caused the primary
	// performance regression. The renderer word-wrap is the only property that
	// depends on width, so skip creation when width is unchanged.
	if m.width != m.rendererWidth || !m.ready {
		var renderer *glamour.TermRenderer
		var err error
		if m.theme == "light" {
			renderer, err = glamour.NewTermRenderer(
				glamour.WithStandardStyle("light"),
				glamour.WithWordWrap(m.width-4),
			)
		} else {
			// dracula / dark: use CustomGlamourStyle() — NOT WithStandardStyle("dark").
			// The standard dark palette injects orange/amber code-block backgrounds
			// that bleed via ANSI reset sequences into surrounding unstyled text.
			renderer, err = glamour.NewTermRenderer(
				glamour.WithStyles(CustomGlamourStyle()),
				glamour.WithWordWrap(m.width-4),
			)
		}
		if err == nil {
			m.glamour = renderer
			m.rendererWidth = m.width
		}
	}

	// Re-render content at new dimensions — but skip during active streaming.
	// The streaming token handler updates the viewport on its own throttle
	// schedule; triggering a full re-render here on every resize event during
	// streaming would double the work on every WindowSizeMsg.
	if m.ready && !m.generating {
		m.updateViewportContent()
	}

	// Force focus on resize (helps keep keyboard open on mobile)
	m.textarea.Blur()
	m.textarea.Focus()

	m.ready = true

	return m, textarea.Blink
}

// handleKeyMsg handles keyboard input
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Splash screen: consume all keys; Enter dismisses when ready.
	if !m.splashDone {
		if msg.Type == tea.KeyEnter && m.ready {
			m.splashDone = true
		}
		return m, nil
	}

	// Search mode: route all input to the search handler.
	if m.searchMode {
		return m.handleSearchKeyMsg(msg)
	}

	// History search mode (Ctrl+R): route all input to history handler.
	if m.histSearchMode {
		handled, cmd := m.handleHistSearchKey(msg.String())
		if handled {
			return m, cmd
		}
	}

	// Rewind menu navigation.
	if m.rewindMenuOpen {
		switch msg.String() {
		case "up", "k":
			if m.rewindCursor > 0 {
				m.rewindCursor--
			}
			return m, nil
		case "down", "j":
			if m.rewindCursor < len(m.rewindItems)-1 {
				m.rewindCursor++
			}
			return m, nil
		case "enter":
			if len(m.rewindItems) > 0 && m.orchestrator != nil {
				item := m.rewindItems[m.rewindCursor]
				if _, err := m.orchestrator.Checkpoints.Rewind(item.ID, m.orchestrator.ConversationHistory); err == nil {
					m.addSystemMessage(fmt.Sprintf("_⏮ Rewound to checkpoint: %s_", item.Description))
					m.updateViewportContent()
				}
			}
			m.rewindMenuOpen = false
			return m, nil
		case "esc":
			m.rewindMenuOpen = false
			return m, nil
		}
		return m, nil
	}

	// @ autocomplete: check active before normal key routing.
	if m.atCompleteActive {
		handled, cmd := m.handleAtCompleteKey(msg.String())
		if handled {
			return m, cmd
		}
		// Not handled — let the key fall through to normal routing,
		// but append char to query if it's a regular character.
		if len(msg.String()) == 1 {
			m.atCompleteQuery += msg.String()
			return m, fetchAtComplete(m.atCompleteCWD, m.atCompleteQuery)
		}
	}

	// Ctrl+X — interrupt in-progress generation
	if key.Matches(msg, m.keymap.Interrupt) {
		if m.generating && m.orchestrator != nil {
			m.orchestrator.Interrupt()
			m.generating = false
			m.planningActive = false
			m.planningBuf.Reset()
			m.planningIntent = ""
			m.currentResponse.Reset()
			m.streamChunkCount = 0
			m.recalcViewportHeight()
			m.addSystemMessage("_Generation interrupted (Ctrl+X)_")
			m.updateViewportContent()
			if !m.userScrolledUp {
				m.viewport.GotoBottom()
			}
			m.textarea.Blur()
			m.textarea.Focus()
			return m, nil
		}
		return m, nil
	}

	// Shift+Tab — cycle execution mode (alternative to Ctrl+N)
	if key.Matches(msg, m.keymap.CycleModeTab) {
		if m.orchestrator != nil {
			modeName := m.orchestrator.CycleMode()
			m.statusBar.SetMode(modeName)
			return m, func() tea.Msg { return SuccessToast("⚡", "Mode: "+modeName) }
		}
		return m, nil
	}

	// Ctrl+P — cycle execution mode (Normal → Plan → AutoEdit → Normal)
	if key.Matches(msg, m.keymap.CycleMode) {
		if m.orchestrator != nil {
			modeName := m.orchestrator.CycleMode()
			m.statusBar.SetMode(modeName)
			m.addSystemMessage(fmt.Sprintf("_Mode: **%s**_", modeName))
			m.updateViewportContent()
		}
		return m, nil
	}

	// Ctrl+R — fold / unfold all collapsible frames (internal reasoning, a2a).
	// Rule: if any frame is expanded → collapse all; if all are collapsed → expand all.
	if key.Matches(msg, m.keymap.FoldFrames) {
		anyExpanded := false
		for _, fm := range m.messages {
			if (fm.Role == "internal" || fm.Role == "a2a") && !fm.Collapsed {
				anyExpanded = true
				break
			}
		}
		for i := range m.messages {
			if m.messages[i].Role == "internal" || m.messages[i].Role == "a2a" {
				m.messages[i].Collapsed = anyExpanded // collapse if any open; expand if all closed
			}
		}
		m.updateViewportContent()
		return m, nil
	}

	// Chat-specific keybindings
	switch {
	case key.Matches(msg, m.keymap.SelectModel):
		m.state = modelSelectView
		m.initModelSelectState()
		m.updateProviderKeyStatuses()
		return m, m.updateModelListItems()

	case key.Matches(msg, m.keymap.ShowTools):
		m.state = toolsTableView
		m.updateToolsTable()
		return m, nil

	case key.Matches(msg, m.keymap.ShowSettings):
		var debugOn bool
		if m.commands != nil && m.commands.Orch != nil {
			debugOn = m.debugMode
		}
		var appStateSetter func(cats []string) error
		var providerSetter func(ids []string) error
		if m.commands != nil && m.commands.Orch != nil {
			if m.commands.Orch.PersistDisabledCategories != nil {
				appStateSetter = m.commands.Orch.PersistDisabledCategories
			}
			if m.commands.Orch.PersistDisabledProviders != nil {
				providerSetter = m.commands.Orch.PersistDisabledProviders
			}
		}
		var toolReg *tools.Registry
		if m.commands != nil {
			toolReg = m.commands.GetToolRegistry()
		}
		m.activeOverlay = NewSettingsOverlay(m.width, m.height, m.commands.Orch, toolReg, appStateSetter, debugOn, providerSetter, m.integrationGetter, m.integrationSetter)
		return m, nil

	// ctrl+d (0x04) is intercepted by the hotkey manager (CmdDuplicateSess).
	// Discovery view is reachable via the tab bar or /disc command instead.

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "analytics"))):
		m.state = analyticsView
		return m, nil

	case key.Matches(msg, m.keymap.ShowDiagnostics):
		if m.state == diagnosticsView {
			m.state = chatView
		} else {
			m.state = diagnosticsView
		}
		return m, nil

	case key.Matches(msg, m.keymap.ShowBookmarks):
		m.bookmarkOverlay = !m.bookmarkOverlay
		m.discCursor = 0
		return m, nil

	case key.Matches(msg, m.keymap.ToggleTabs):
		m.compactTabs = !m.compactTabs
		m.recalcViewportHeight()
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+|"), key.WithHelp("ctrl+|", "side panel"))):
		m.sidePanelOpen = !m.sidePanelOpen
		if m.sidePanelOpen {
			m.sidePanelWidth = m.width * 28 / 100
			if m.sidePanelWidth < 24 {
				m.sidePanelWidth = 24
			}
			// V.2: Do NOT change viewport.Width or rendererWidth — keeps glamour
			// render width fixed so text doesn't reflow when panel opens.
			m.recalcViewportHeight()
			return m, sidePanelTick()
		} else {
			m.sidePanelWidth = 0
			m.recalcViewportHeight()
		}
		return m, nil

	case key.Matches(msg, m.keymap.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		// Handle HITL rejection via Esc
		if m.awaitingHITL {
			m.resolveCurrentHITL(engine.HITLDecision{Approval: engine.HITLRejected, Notes: "Rejected (Esc)"})
			m.addSystemMessage("🚫 **HITL: rejected** — tool execution cancelled.")
			m.updateViewportContent()
			if !m.userScrolledUp {
				m.viewport.GotoBottom()
			}
			return m, nil
		}
		// Handle permission prompt denial
		if m.awaitingPermission {
			m.DenyPermission()
			return m, nil
		}
		// Handle intervention prompt denial (Stop)
		if m.interventionMode {
			m.RespondToIntervention(engine.InterventionStop)
			return m, nil
		}
		if m.generating {
			// Cancel generation. Clear generating flag first so recalcViewportHeight
			// uses the correct height (loading bar gone).
			m.generating = false
			m.planningActive = false
			m.planningBuf.Reset()
			m.planningIntent = ""
			m.currentResponse.Reset()
			m.streamChunkCount = 0
			// Expand viewport now loading bar is gone, then sync content.
			m.recalcViewportHeight()
			m.addSystemMessage("_Generation cancelled_")
			m.updateViewportContent()
			if !m.userScrolledUp {
				m.viewport.GotoBottom()
			}
			m.textarea.Blur()
			m.textarea.Focus()
			return m, textarea.Blink
		}

		// Double-Esc rewind menu: open when two Esc presses are within 500ms at idle.
		if !m.generating && !m.searchMode && !m.histSearchMode &&
			!m.awaitingHITL && !m.awaitingPermission &&
			!m.interventionMode && m.activeOverlay == nil {
			if time.Since(m.lastEscTime) < 500*time.Millisecond {
				if m.orchestrator != nil && m.orchestrator.Checkpoints != nil {
					items := m.orchestrator.Checkpoints.List()
					if len(items) > 0 {
						m.rewindItems = items
						m.rewindMenuOpen = true
						m.rewindCursor = 0
					}
				}
			}
			m.lastEscTime = time.Now()
		}
	}

	// Handle intervention prompt navigation
	if m.interventionMode {
		switch msg.String() {
		case "y", "c": // Continue
			m.RespondToIntervention(engine.InterventionContinue)
			return m, nil
		case "s", "a": // Allow Session
			m.RespondToIntervention(engine.InterventionAllowSession)
			return m, nil
		case "k", "x", "n": // Kill/Stop
			m.RespondToIntervention(engine.InterventionStop)
			return m, nil
		}
		return m, nil // Ignore other keys
	}

	// Handle HITL approval prompt — intercept y/n/enter (esc handled in switch above).
	if m.awaitingHITL {
		switch msg.String() {
		case "y", "Y", "enter":
			m.resolveCurrentHITL(engine.HITLDecision{Approval: engine.HITLApproved})
			m.addSystemMessage("✅ **HITL: approved** — tool execution will proceed.")
			m.updateViewportContent()
			if !m.userScrolledUp {
				m.viewport.GotoBottom()
			}
		case "n", "N":
			m.resolveCurrentHITL(engine.HITLDecision{Approval: engine.HITLRejected, Notes: "Rejected by user"})
			m.addSystemMessage("🚫 **HITL: rejected** — tool execution cancelled.")
			m.updateViewportContent()
			if !m.userScrolledUp {
				m.viewport.GotoBottom()
			}
		}
		return m, nil // Drop all other keys while HITL prompt is active
	}

	// Viewport navigation shortcuts - always available for scrolling
	// Use ctrl+u/d and pgup/pgdown for reliable viewport scrolling
	switch msg.String() {
	case "ctrl+u":
		m.viewport.HalfViewUp()
		m.userScrolledUp = true
		return m, nil
	case "ctrl+d":
		m.viewport.HalfViewDown()
		if m.viewport.AtBottom() {
			m.userScrolledUp = false
		}
		return m, nil
	case "ctrl+up", "ctrl+k":
		// Additional scroll up shortcuts
		m.viewport.LineUp(1)
		m.userScrolledUp = true
		return m, nil
	case "ctrl+down", "ctrl+j":
		// Additional scroll down shortcuts
		m.viewport.LineDown(1)
		if m.viewport.AtBottom() {
			m.userScrolledUp = false
		}
		return m, nil
	case "alt+up", "shift+up":
		m.viewport.LineUp(1)
		m.userScrolledUp = true
		return m, nil
	case "alt+down", "shift+down":
		m.viewport.LineDown(1)
		if m.viewport.AtBottom() {
			m.userScrolledUp = false
		}
		return m, nil
	}

	// Handle permission prompt navigation
	if m.awaitingPermission && m.permissionPrompt != nil {
		switch msg.String() {
		case "up", "k":
			m.permissionPrompt.MoveUp()
			return m, nil
		case "down", "j":
			m.permissionPrompt.MoveDown()
			return m, nil
		case "enter":
			m.ApprovePermission()
			return m, nil
		}
		return m, nil // Ignore other keys during permission prompt
	}

	// If generating, allow viewport scrolling with arrow keys and j/k
	if m.generating {
		switch msg.String() {
		case "up", "k":
			m.viewport.LineUp(1)
			m.userScrolledUp = true
			return m, nil
		case "down", "j":
			m.viewport.LineDown(1)
			if m.viewport.AtBottom() {
				m.userScrolledUp = false
			}
			return m, nil
		}
		return m, nil
	}

	// Force focus on textarea if it's not focused
	// This helps with mobile keyboards in Termux
	if !m.textarea.Focused() {
		m.textarea.Focus()
	}

	// Handle Enter key for submission
	// Detect @ for file autocomplete trigger.
	if msg.String() == "@" && !m.generating {
		cur := m.textarea.Value()
		m.atCompleteAt = len(cur)
		m.atCompleteActive = true
		m.atCompleteQuery = ""
		m.atCompleteIdx = 0
		// Use cached CWD — no syscall per keystroke.
		return m, fetchAtComplete(m.atCompleteCWD, "")
	}

	if msg.Type == tea.KeyEnter {
		// Check if Alt is pressed (for multi-line)
		if !msg.Alt {
			return m, m.submitPrompt()
		}
	}

	// Special viewport navigation when textarea is not focused
	switch msg.String() {
	case "pgup":
		m.viewport.ViewUp()
		m.userScrolledUp = true
		return m, nil
	case "pgdown":
		m.viewport.ViewDown()
		if m.viewport.AtBottom() {
			m.userScrolledUp = false
		}
		return m, nil
	case "home":
		m.viewport.GotoTop()
		m.userScrolledUp = true
		return m, nil
	case "end":
		m.viewport.GotoBottom()
		m.userScrolledUp = false
		return m, nil
	case "tab":
		// Input completion using Gemini
		input := m.textarea.Value()
		if input != "" && m.orchestrator != nil && m.orchestrator.Consultant != nil {
			// Trigger completion in background
			return m, func() tea.Msg {
				ctx := context.Background()
				prompt := fmt.Sprintf("Complete the following user input for a chat interface. Provide ONLY the completion suffix, nothing else. If no clear completion is possible, return empty string.\n\nInput: %s", input)

				completion, err := m.orchestrator.Consultant.Generate(ctx, prompt)
				if err == nil && completion != "" {
					return CompletionMsg{Content: completion}
				}
				return nil
			}
		}
		// Fallback if empty or no consultant
		m.textarea.Focus()
		return m, nil
	}

	return m, nil
}

// handleSearchKeyMsg handles all keyboard input while search mode is active.
// It owns the full key event and returns before normal handleKeyMsg processing.
func (m *Model) handleSearchKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Exit search, restore normal view.
		m.searchMode = false
		m.searchQuery = ""
		m.searchMatches = nil
		m.searchMatchIdx = 0
		m.textarea.Focus()
		m.updateViewportContent()
		return m, nil

	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.searchQuery) > 0 {
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
			m.rebuildSearchMatches()
			m.updateViewportContent()
		}
		return m, nil

	case tea.KeyEnter:
		// Advance to next match (wrap around).
		if len(m.searchMatches) > 0 {
			m.searchMatchIdx = (m.searchMatchIdx + 1) % len(m.searchMatches)
			m.updateViewportContent()
		}
		return m, nil

	case tea.KeyUp:
		// Previous match.
		if len(m.searchMatches) > 0 {
			m.searchMatchIdx--
			if m.searchMatchIdx < 0 {
				m.searchMatchIdx = len(m.searchMatches) - 1
			}
			m.updateViewportContent()
		}
		return m, nil

	case tea.KeyDown:
		// Next match.
		if len(m.searchMatches) > 0 {
			m.searchMatchIdx = (m.searchMatchIdx + 1) % len(m.searchMatches)
			m.updateViewportContent()
		}
		return m, nil

	case tea.KeyRunes:
		// Append typed characters to the search query.
		m.searchQuery += string(msg.Runes)
		m.rebuildSearchMatches()
		m.searchMatchIdx = 0
		m.updateViewportContent()
		return m, nil
	}
	return m, nil
}

// handleTokenMsg handles incoming tokens from AI response
func (m *Model) handleTokenMsg(msg TokenMsg) (tea.Model, tea.Cmd) {
	if !m.generating {
		return m, nil
	}

	m.lastTokenTime = time.Now()

	if msg.Content == "[__GORKBOT_STREAM_RETRY__]" {
		if m.currentResponse.Len() > 0 {
			role := "assistant"
			if m.isConsultant {
				role = "consultant"
			}
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].Role == role {
					m.messages[i].Role = "internal"
					m.messages[i].MessageType = "internal"
					m.messages[i].Collapsed = true
					m.messages[i].Content = "**Incomplete Stream Attempt (Retrying...)**\n\n" + m.currentResponse.String()
					break
				}
			}
			m.currentResponse.Reset()
			m.updateViewportContent()
		}
		return m, nil
	}

	// Append token to current response
	m.currentResponse.WriteString(msg.Content)
	m.liveTokenCount++

	// Update the streaming message in real-time for live display
	m.updateStreamingMessage()

	// Continue receiving tokens
	return m, nil
}

// handleStreamComplete handles the completion of streaming.
// This fires on clean finish AND after ESC cancel (goroutine still runs to
// completion). All teardown runs unconditionally so every exit path leaves
// the viewport fully synced and touch scroll responsive.
func (m *Model) handleStreamComplete(msg StreamCompleteMsg) (tea.Model, tea.Cmd) {
	// Commit any remaining buffered content that the throttle skipped.
	// (No-op when ESC already cleared currentResponse.)
	if m.generating {
		finalContent := m.currentResponse.String()
		if finalContent != "" {
			role := "assistant"
			if m.isConsultant {
				role = "consultant"
			}
			foundIndex := -1
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].Role == role {
					foundIndex = i
					break
				}
			}
			if foundIndex != -1 {
				m.messages[foundIndex].Content = finalContent
			}
		}
		m.currentResponse.Reset()
		m.generating = false
	}
	m.streamChunkCount = 0
	// Clear extended-thinking panel now that generation is done.
	m.thinkingBuf.Reset()
	m.thinkingActive = false

	// ── Orphan protection: seal any hooks still marked active ────────────────
	m.activeHooks = sealAllHooks(m.activeHooks)
	m.hookTickActive = false

	// Expand viewport — the loading indicator line is now gone.
	// handleWindowSize reserved 1 line for it; without this the scroll area
	// stays 1 line short and touch scroll appears frozen on Termux.
	m.recalcViewportHeight()

	// Final guaranteed full re-render at correct dimensions, then pin to bottom.
	m.updateViewportContent()
	if !m.userScrolledUp {
		m.viewport.GotoBottom()
	}

	// Re-focus textarea so the mobile keyboard stays active.
	m.textarea.Blur()
	m.textarea.Focus()

	return m, textarea.Blink
}

// handleHITLRequest handles a SENSE HITL plan-and-execute approval request.
// It pushes the request onto the FIFO queue and, if no HITL is currently
// active, immediately activates it and opens the approval overlay.
func (m *Model) handleHITLRequest(msg HITLRequestMsg) (tea.Model, tea.Cmd) {
	req := msg.Request
	item := hitlPendingItem{req: req, ch: msg.ResponseChan}
	m.hitlQueue = append(m.hitlQueue, item)

	if !m.awaitingHITL {
		// Activate the front-of-queue immediately.
		front := m.hitlQueue[0]
		m.hitlRequest = &front.req
		m.hitlChan = front.ch
		m.awaitingHITL = true
		m.state = stateHITLApproval

		planDisplay := fmt.Sprintf(
			"## ⚡ SENSE HITL — v2.0 Validation Required\n\n"+
				"**Tool:** `%s`\n\n%s",
			req.ToolName, req.Plan,
		)
		m.addSystemMessage(planDisplay)
		m.updateViewportContent()
		if !m.userScrolledUp {
			m.viewport.GotoBottom()
		}
	}
	return m, nil
}

// handleToolExecution handles tool execution notifications
func (m *Model) handleToolExecution(msg ToolExecutionMsg) (tea.Model, tea.Cmd) {
	// ── P2 fix: flush the current streaming segment before inserting the tool box.
	// Without this, the viewport throttle (every 8 tokens) can leave the last
	// partial tokens invisible when the tool box renders.
	if m.generating && m.currentResponse.Len() > m.responseSegStart {
		role := "assistant"
		if m.isConsultant {
			role = "consultant"
		}
		seg := m.currentResponse.String()[m.responseSegStart:]
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Role == "user" {
				break
			}
			if m.messages[i].Role == role {
				m.messages[i].Content = seg
				break
			}
		}
	}

	// Record where the post-tool segment begins in currentResponse.
	m.responseSegStart = m.currentResponse.Len()
	// Signal that the next streaming token must open a new assistant message.
	m.streamAfterTool = true
	// Reset throttle so the first post-tool token triggers an immediate render.
	m.streamChunkCount = 0

	// Compute elapsed time from start time recorded in ToolProgressMsg.
	elapsed := time.Duration(0)
	if m.toolStartTimes != nil {
		if start, ok := m.toolStartTimes[msg.ToolName]; ok {
			elapsed = time.Since(start)
			delete(m.toolStartTimes, msg.ToolName)
		}
	}

	// Commit current activity to the log and transition to synthesizing phase.
	if m.currentActivity != nil {
		m.currentActivity.Done = true
		m.currentActivity.Success = msg.Result == nil || msg.Result.Error == ""
		m.currentActivity.Elapsed = elapsed
		m.activityLog = append(m.activityLog, *m.currentActivity)
		if len(m.activityLog) > 4 {
			m.activityLog = m.activityLog[len(m.activityLog)-4:]
		}
		m.currentActivity = nil
	}
	m.genPhase = phaseSynthesizing

	// If auth is required, trigger the dynamic wizard
	if msg.Result != nil && msg.Result.AuthRequired {
		if prog := m.getProgram(); prog != nil {
			go func() {
				respChan := make(chan string)
				prog.Send(AuthRequestMsg{
					ToolName:     msg.ToolName,
					AuthType:     msg.Result.AuthType,
					Description:  msg.Result.Output,
					ResponseChan: respChan,
				})
				
				// Block until credential received
				credential := <-respChan
				if credential != "" {
					// Persist and signal recovery
					var status string
					if m.orchestrator != nil {
						status = m.orchestrator.SetProviderKey(context.Background(), "google", credential)
					}
					
					success := !strings.Contains(strings.ToLower(status), "failed")
					errMsg := ""
					if !success {
						errMsg = status
					}

					prog.Send(APIKeySavedMsg{
						Provider: "google",
						Valid:    success,
						ErrMsg:   errMsg,
					})

					if success {
						prog.Send(InfoToast("🔐", "Credential saved. You can now retry your request."))
					}
				}
			}()
		}
	}

	// Level 1 nesting for tools called by the main assistant.
	m.addToolMessageWithNesting(msg.ToolName, msg.Result, 1, elapsed)
	m.updateViewportContent()

	return m, nil
}

// updateStreamingMessage updates the in-memory message and throttles viewport renders.
//
// Performance contract: glamour re-renders the ENTIRE message list on every
// updateViewportContent() call. At token rates of 20-80 tokens/sec this means
// hundreds of full glamour passes per second, which causes severe TUI lag.
// We batch the visual update to once every streamChunkInterval tokens; the final
// flush is done unconditionally by handleStreamComplete.
func (m *Model) updateStreamingMessage() {
	if !m.generating {
		return
	}

	role := "assistant"
	if m.isConsultant {
		role = "consultant"
	}

	// Only show content from the current segment (post-tool content goes into a
	// separate message so the display order is: pre-tool text → tool box → post-tool text).
	seg := m.currentResponse.String()[m.responseSegStart:]

	if m.streamAfterTool {
		// A tool result was just inserted.  Start a fresh assistant message so
		// post-tool tokens appear AFTER the tool box, not merged into the pre-tool message.
		m.streamAfterTool = false
		m.messages = append(m.messages, Message{
			Role:         role,
			Content:      seg,
			IsConsultant: m.isConsultant,
		})
	} else {
		// Normal case: update the latest assistant message in the current turn.
		foundIndex := -1
		for i := len(m.messages) - 1; i >= 0; i-- {
			// Never look past a user message into a previous turn.
			if m.messages[i].Role == "user" {
				break
			}
			if m.messages[i].Role == role {
				foundIndex = i
				break
			}
		}
		if foundIndex != -1 {
			m.messages[foundIndex].Content = seg
		} else {
			m.messages = append(m.messages, Message{
				Role:         role,
				Content:      seg,
				IsConsultant: m.isConsultant,
			})
		}
	}

	// Only re-render viewport every streamChunkInterval tokens to avoid
	// O(messages × tokens) glamour rendering cost.
	m.streamChunkCount++
	if m.streamChunkCount%m.streamChunkInterval == 0 {
		m.updateViewportContent()
	}
}

// handleSpinnerTick handles spinner animation updates
func (m *Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if !m.generating {
		return m, nil
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)

	return m, cmd
}

// handlePhraseTick handles loading phrase rotation
func (m *Model) handlePhraseTick(msg PhraseTickMsg) (tea.Model, tea.Cmd) {
	if !m.generating {
		return m, nil
	}

	// Rotate to a new phrase
	m.currentPhrase = RotatePhrase(m.currentPhrase, m.isConsultant)

	return m, phraseTick()
}

// handlePlanningTokenMsg receives a streaming token destined for the planning box.
// Tokens are counted for the "Reasoning... N tokens" display in the activity panel.
// currentResponse is NOT updated here; the final answer arrives via PlanningCommitMsg.
func (m *Model) handlePlanningTokenMsg(msg PlanningTokenMsg) (tea.Model, tea.Cmd) {
	if !m.generating {
		return m, nil
	}

	m.lastTokenTime = time.Now()

	// Retry signal: the stream rewound; reset token count.
	if msg.Content == "[__GORKBOT_STREAM_RETRY__]" {
		m.planningBuf.Reset()
		m.thinkingTokens = 0
		m.updateViewportContent()
		return m, nil
	}

	m.planningBuf.WriteString(msg.Content)
	m.thinkingTokens++
	m.liveTokenCount++

	// Throttle viewport re-renders (same interval as streaming).
	m.streamChunkCount++
	if m.streamChunkCount%m.streamChunkInterval == 0 {
		m.updateViewportContent()
	}
	return m, nil
}

// handlePlanningBoxClear resets the planning buffer when a tool call fires.
// The previous reasoning segment is discarded; only the final segment is committed.
func (m *Model) handlePlanningBoxClear(_ PlanningBoxClearMsg) (tea.Model, tea.Cmd) {
	m.planningBuf.Reset()
	m.thinkingTokens = 0
	m.streamChunkCount = 0
	m.updateViewportContent()
	return m, nil
}

// handlePlanningCommit finalises generation by adding the last planning segment
// (the actual answer) as an assistant message and deactivating the planning box.
// It sets m.generating=false so that the subsequent StreamCompleteMsg skips the
// legacy currentResponse commit path (but still recalcs the viewport).
func (m *Model) handlePlanningCommit(msg PlanningCommitMsg) (tea.Model, tea.Cmd) {
	m.planningActive = false
	m.planningBuf.Reset()
	m.genPhase = phaseIdle
	m.thinkingTokens = 0
	m.activityLog = nil
	m.currentActivity = nil
	m.generating = false
	m.streamChunkCount = 0

	if msg.Content != "" {
		m.addAssistantMessage(msg.Content, m.isConsultant)
	}
	m.updateViewportContent()
	if !m.userScrolledUp {
		m.viewport.GotoBottom()
	}
	return m, nil
}

// handlePlanningTick refreshes the activity panel every 2s while generating.
// This keeps the elapsed timer and token count up-to-date in the UI.
func (m *Model) handlePlanningTick(_ PlanningTickMsg) (tea.Model, tea.Cmd) {
	if !m.planningActive {
		return m, nil
	}

	// Sync context stats into analytics while we wait for long generations
	if m.orchestrator != nil {
		if cm := m.orchestrator.ContextMgr; cm != nil {
			totalIn, totalOut := cm.TotalUsage()
			usedPct := cm.UsedPct()
			usedToks := cm.TokensUsed()
			maxToks := cm.MaxTokens()
			totalToks := totalIn + totalOut
			costUSD := cm.TotalCostUSD()

			if m.analytics != nil {
				m.analytics.ContextUsedPct = usedPct
				m.analytics.ContextUsedToks = usedToks
				m.analytics.ContextMaxToks = maxToks
				m.analytics.RecordTokens(totalToks, costUSD)
			}
			m.statusBar.UpdateContext(usedPct, costUSD)
		}
	}

	m.updateViewportContent()
	return m, planningTick()
}

// handleErrorMsg handles error messages
func (m *Model) handleErrorMsg(msg ErrorMsg) (tea.Model, tea.Cmd) {
	m.err = msg.Err
	m.generating = false
	m.planningActive = false
	m.planningBuf.Reset()
	m.genPhase = phaseIdle
	m.thinkingTokens = 0
	m.currentActivity = nil

	// ── Orphan protection: seal any hooks still marked active ────────────────
	m.activeHooks = sealAllHooks(m.activeHooks)
	m.hookTickActive = false

	// Add error message to chat
	m.addSystemMessage("**Error:** " + msg.Err.Error())
	m.updateViewportContent()

	return m, nil
}

// handleStartGeneration handles the start of AI generation
func (m *Model) handleStartGeneration(msg StartGenerationMsg) (tea.Model, tea.Cmd) {
	m.generating = true
	m.isConsultant = msg.IsConsultant
	m.currentPhrase = GetRandomPhrase(msg.IsConsultant)
	m.currentResponse.Reset()
	m.streamChunkCount = 0    // Reset throttle counter for new generation
	m.responseSegStart = 0    // New turn starts a fresh segment
	m.streamAfterTool = false // No pending tool transition
	m.liveTokenCount = 0      // Reset live token count

	// Activate the planning box / activity panel for this generation turn.
	m.planningActive = true
	m.planningBuf.Reset()
	m.genPhase = phaseThinking
	m.thinkingTokens = 0
	m.activityLog = nil
	m.currentActivity = nil

	// Clear hooks from the previous turn (keep only recently-completed ones
	// for at most 8 entries; seal any still-active ones first).
	m.activeHooks = sealAllHooks(m.activeHooks)
	if len(m.activeHooks) > 8 {
		m.activeHooks = m.activeHooks[len(m.activeHooks)-8:]
	}

	// Start hook tick loop.
	if !m.hookTickActive {
		m.hookTickActive = true
	}

	return m, tea.Batch(
		phraseTick(),
		planningTick(),
		hookTick(),
	)
}

// handleEndGeneration handles the end of AI generation
func (m *Model) handleEndGeneration(msg EndGenerationMsg) (tea.Model, tea.Cmd) {
	m.generating = false

	// Flush the final segment into the streaming message already in m.messages.
	// Use responseSegStart so we only show content for the current segment
	// (post-tool tokens are never merged back into the pre-tool message).
	if m.currentResponse.Len() > m.responseSegStart || m.streamAfterTool {
		role := "assistant"
		if m.isConsultant {
			role = "consultant"
		}
		seg := m.currentResponse.String()[m.responseSegStart:]
		if m.streamAfterTool {
			// Tool fired but no post-tool tokens came — skip the empty message.
			m.streamAfterTool = false
		} else {
			foundIndex := -1
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].Role == "user" {
					break
				}
				if m.messages[i].Role == role {
					foundIndex = i
					break
				}
			}
			if foundIndex != -1 {
				m.messages[foundIndex].Content = seg
			} else {
				m.addAssistantMessage(seg, m.isConsultant)
			}
		}
		m.currentResponse.Reset()
		m.responseSegStart = 0
		m.updateViewportContent()
	}

	// Sync context stats + mode from orchestrator into the status bar.
	var toastCmd tea.Cmd
	if m.orchestrator != nil {
		if cm := m.orchestrator.ContextMgr; cm != nil {
			prevPct := m.statusBar.ContextPct()
			totalIn, totalOut := cm.TotalUsage()
			usedPct := cm.UsedPct()
			usedToks := cm.TokensUsed()
			maxToks := cm.MaxTokens()
			totalToks := totalIn + totalOut
			costUSD := cm.TotalCostUSD()

			m.liveTokenCount = 0
			m.statusBar.UpdateContext(usedPct, costUSD)

			if m.analytics != nil {
				m.analytics.ContextUsedPct = usedPct
				m.analytics.ContextUsedToks = usedToks
				m.analytics.ContextMaxToks = maxToks
				m.analytics.RecordTokens(totalToks, costUSD)
				m.statusBar.SetTokenRateHistory(m.analytics.TokenRateHistory)
			}

			// Emit threshold-crossing toasts with appropriate priority levels.
			if usedPct >= 0.95 && prevPct < 0.95 {
				toastCmd = func() tea.Msg {
					return CriticalToast("Context critical (95%+) — /compress now")
				}
			} else if usedPct >= 0.80 && prevPct < 0.80 {
				toastCmd = func() tea.Msg {
					return WarnToast("Context at 80% — approaching limit")
				}
			}
		}
		if mm := m.orchestrator.ModeManager; mm != nil {
			m.statusBar.SetMode(mm.Name())
		}
	}

	// ── Orphan protection: seal any hooks still marked active ────────────────
	m.activeHooks = sealAllHooks(m.activeHooks)
	m.hookTickActive = false

	// Re-focus textarea after generation ends
	// This ensures keyboard appears on mobile after AI responds
	m.textarea.Blur()
	m.textarea.Focus()

	if toastCmd != nil {
		return m, tea.Batch(textarea.Blink, toastCmd)
	}
	return m, textarea.Blink
}

// handleClearScreen handles screen clearing
func (m *Model) handleClearScreen(msg ClearScreenMsg) (tea.Model, tea.Cmd) {
	m.messages = []Message{}
	m.activeHooks = nil // clear hook history on screen clear
	m.hookTickActive = false
	m.addSystemMessage("# Screen Cleared\n\nConversation history has been reset.")
	m.updateViewportContent()
	return m, nil
}

// handleQuit handles quit message
func (m *Model) handleQuit(msg QuitMsg) (tea.Model, tea.Cmd) {
	m.quitting = true
	return m, tea.Quit
}

// recalcViewportHeight recomputes the viewport height from current m.height and
// m.generating, so the viewport always fills the space not occupied by fixed UI
// elements. Call this any time generating state changes outside of a resize event.
func (m *Model) recalcViewportHeight() {
	const (
		brandingHeight  = 0 // header moved to splash screen
		separatorHeight = 1
		loadingHeight   = 1 // single-line degraded spinner
		inputHeight     = 5
		statusBarHeight = 1
	)

	tabsHeight := 2
	if m.compactTabs {
		tabsHeight = 1
	}

	// Each visible toast occupies 1 line; at most 3 are rendered at once.
	notifyHeight := min(len(m.toasts), 3)

	// Hook section: 1 header line + maxHookSectionLines tree lines.
	// Only reserve space when hooks are active (generating or still alive).
	hookHeight := 0
	if len(m.activeHooks) > 0 && (m.generating || hasActiveHooks(m.activeHooks)) {
		hookHeight = 1 + maxHookSectionLines // 1 header + 8 = 9
	}

	fixed := brandingHeight + tabsHeight + separatorHeight + inputHeight + statusBarHeight + notifyHeight + hookHeight
	if m.generating {
		fixed += loadingHeight
	}

	h := m.height - fixed
	if h < 10 {
		h = 10
	}
	m.viewport.Height = h
}

// Helper function to check if a key binding matches
func keyMatches(msg tea.KeyMsg, keys ...string) bool {
	for _, k := range keys {
		if msg.String() == k {
			return true
		}
	}
	return false
}

// handleInterventionRequest handles requests for user intervention
func (m *Model) handleInterventionRequest(msg InterventionRequestMsg) (tea.Model, tea.Cmd) {
	m.interventionMode = true
	m.interventionPrompt = msg.Context
	m.interventionChan = msg.ResponseChan
	return m, nil
}

// RespondToIntervention sends the user's response back to the orchestrator
// and resets all intervention state so normal key handling resumes.
func (m *Model) RespondToIntervention(response engine.InterventionResponse) {
	if m.interventionChan != nil {
		m.interventionChan <- response
		close(m.interventionChan)
		m.interventionChan = nil
	}
	m.interventionMode = false
	m.interventionPrompt = ""
}
