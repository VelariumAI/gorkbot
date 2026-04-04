package engine

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/collab"
	"github.com/velariumai/gorkbot/pkg/hooks"
	"github.com/velariumai/gorkbot/pkg/router"
	"github.com/velariumai/gorkbot/pkg/sre"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// StreamCallback is called for each token as it arrives
type StreamCallback func(token string)

// AdviceCallback is called when expert consultant advice is received
type AdviceCallback func(advice string)

// InterventionCallback is called when the watchdog needs human input
type InterventionCallback func(severity WatchdogSeverity, context string) InterventionResponse

// shouldSuppressToolOutput checks if tool outputs should be hidden based on the original prompt.
// Returns true for simple queries that shouldn't include diagnostic/status output.
func shouldSuppressToolOutput(prompt string) bool {
	// Keywords that signal a user wants diagnostic/status info
	diagnosticKeywords := []string{
		"status", "monitor", "diagnostic", "health", "stats", "cpu", "memory",
		"system info", "show state", "gorkbot status", "what are you", "how are you",
		"list tools", "list all", "capabilities", "what can", "brain", "rules",
	}

	lowerPrompt := strings.ToLower(prompt)
	for _, keyword := range diagnosticKeywords {
		if strings.Contains(lowerPrompt, keyword) {
			return false // User asked for diagnostics; show the output
		}
	}

	// Default: suppress tool output for normal queries
	return true
}

// getThinkingCallbackFromAtomic retrieves the thinking callback from atomic storage
func getThinkingCallbackFromAtomic(o *Orchestrator) StreamCallback {
	if cb, ok := o.thinkingCallback.Load().(func(string)); ok {
		return cb
	}
	return nil
}

// stripToolOutputPatterns removes raw tool output blocks from AI responses.
// Strips patterns like "Verified Status:", "Actionable Commands:", system tables, etc.
func stripToolOutputPatterns(response string) string {
	lines := strings.Split(response, "\n")
	var result []string
	skipUntilEmptyLine := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip tool output blocks
		if strings.HasPrefix(trimmed, "Verified Status:") ||
			strings.HasPrefix(trimmed, "Actionable Commands:") ||
			strings.HasPrefix(trimmed, "**Verified Status") ||
			strings.HasPrefix(trimmed, "**Actionable Commands") ||
			strings.HasPrefix(trimmed, "System Status:") ||
			strings.HasPrefix(trimmed, "Tool Execution") {
			skipUntilEmptyLine = true
			continue
		}

		// Stop skipping at blank lines
		if skipUntilEmptyLine && trimmed == "" {
			skipUntilEmptyLine = false
			continue
		}

		// Skip lines while we're in a suppressed block
		if skipUntilEmptyLine {
			continue
		}

		result = append(result, line)
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// ExecuteTaskWithStreaming handles a user prompt with real-time streaming output.
// toolStartCallback (may be nil) is invoked just before each tool execution begins,
// enabling the TUI to show a live "in-progress" panel and a tool call request box.
func (o *Orchestrator) ExecuteTaskWithStreaming(ctx context.Context, prompt string, streamCallback StreamCallback, toolCallback func(string, *tools.ToolResult), toolStartCallback func(string, map[string]interface{}), interventionCallback InterventionCallback, adviceCallback AdviceCallback) error {
	o.Logger.Info("Analyzing task complexity...", "prompt_length", len(prompt))

	// ── Git Workspace Checkpoint ─────────────────────────────────────────────
	workspaceHash := ""
	if o.Workspace != nil {
		if hash, err := o.Workspace.CreateCheckpoint(); err == nil {
			workspaceHash = hash
			o.Logger.Info("Workspace checkpoint created", "hash", hash)
		} else {
			o.Logger.Warn("Failed to create workspace checkpoint", "error", err)
		}
	}

	// Intelligent Trigger Logic
	needsConsult := false
	upperPrompt := strings.ToUpper(prompt)

	// Check keywords
	if strings.Contains(upperPrompt, "COMPLEX") || strings.Contains(upperPrompt, "REFRESH") {
		needsConsult = true
		o.Logger.Info("Complexity trigger detected", "trigger", "keyword_match")
	}

	// Check length threshold
	if len(prompt) > 1000 {
		needsConsult = true
		o.Logger.Info("Complexity trigger detected", "trigger", "length_threshold")
	}

	// Consult the adaptive router for a model suggestion and log it.
	if o.Feedback != nil {
		category := router.ClassifyQuery(prompt)
		if suggested := o.Feedback.SuggestModel(category); suggested != "" {
			currentModel := ""
			if primary := o.Primary(); primary != nil {
				currentModel = primary.GetMetadata().ID
			}
			if suggested != currentModel {
				o.Logger.Info("Adaptive router suggests different model",
					"category", string(category),
					"current", currentModel,
					"suggested", suggested,
				)
			}
		}
	}

	var consultationAdvice string
	if consultant := o.Consultant(); needsConsult && consultant != nil {
		o.Logger.Info("Triggering Specialty Consult...", "consultant", consultant.Name())
		// Fire consultant_invoked hook.
		if o.Hooks != nil {
			o.Hooks.FireAsync(ctx, hooks.EventConsultantInvoked, hooks.Payload{
				Extra: map[string]interface{}{
					"trigger":    "complexity_or_keyword",
					"prompt_len": len(prompt),
				},
			})
		}

		if o.EnableWatchdog {
			o.printWatchdogState("Consultation", prompt)
		}

		// For consultation, we still use non-streaming since it's quick
		advice, err := consultant.Generate(ctx, prompt)
		if err != nil {
			o.Logger.Error("Consultation failed", "error", err)
			consultationAdvice = ""
		} else if advice == "" {
			o.Logger.Warn("Consultant returned empty response")
			consultationAdvice = ""
		} else {
			consultationAdvice = advice
			o.Logger.Info("Consultation received", "length", len(advice))
			if adviceCallback != nil {
				adviceCallback(advice)
			}
			// Fire consultant_response hook.
			if o.Hooks != nil {
				o.Hooks.FireAsync(ctx, hooks.EventConsultantResponse, hooks.Payload{
					Extra: map[string]interface{}{
						"response_length": len(advice),
					},
				})
			}
		}
	}

	// Add system message with tool context (only on first message if history is empty)
	if o.ConversationHistory.Count() == 0 {
		// Layer 0: PromptBuilder preamble (identity, soul, bootstrap, runtime, channel hint, tool suppression).
		var pbPreamble string
		if o.PromptBuilder != nil {
			// Update ToolSuppressionLayer with current user query for intelligent classification
			for _, layer := range o.PromptBuilder.Layers() {
				if tsl, ok := layer.(*ToolSuppressionLayer); ok {
					tsl.UserQuery = prompt // Pass user query for diagnostic classification
				}
			}

			cwd, _ := os.Getwd()
			pbPreamble = o.PromptBuilder.Build(BuildContext{
				WorkDir:   cwd,
				SessionID: o.SessionID,
				Model:     o.primaryModelName,
				Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
				Channel:   "tui",
			})
		}

		toolContext := ""
		if o.Registry != nil {
			toolContext = o.GetToolContext()
		}

		// ── CCI Tier 1 + Tier 2 + Truth Sentry (always prepended) ──────────
		// CCI hot memory, optional specialist, and drift warnings are injected
		// at the head of every new session's system prompt.
		if cciPrefix := o.cciPrefixForSystemMessage(prompt); cciPrefix != "" {
			if toolContext != "" {
				toolContext = cciPrefix + "\n\n" + toolContext
			} else {
				toolContext = cciPrefix
			}
			o.Logger.Info("CCI context injected",
				"tier1_len", len(cciPrefix))
		}

		// ── Three-tier context injection (oh-my-opencode inspired) ──────────
		// Auto-inject GORKBOT.md hierarchy + README.md + .gorkbot/rules/*.md
		// from the current working directory and its parents.
		if o.ContextInjector != nil {
			injected := o.ContextInjector.Collect("")
			if injected.SystemPromptPrefix != "" {
				o.Logger.Info("Context injection active",
					"sources", len(injected.Sources),
					"bytes", injected.TotalBytes,
				)
				if toolContext != "" {
					toolContext = injected.SystemPromptPrefix + "\n\n" + toolContext
				} else {
					toolContext = injected.SystemPromptPrefix
				}
				// Fire hook for observability.
				if o.Hooks != nil {
					o.Hooks.FireAsync(ctx, hooks.EventPreMessageSend, hooks.Payload{
						Extra: map[string]interface{}{
							"type":    "context_injection",
							"sources": injected.Sources,
							"bytes":   injected.TotalBytes,
						},
					})
				}
			}
		}

		// Prepend PromptBuilder preamble as layer-0 of the system prompt.
		if pbPreamble != "" {
			if toolContext != "" {
				toolContext = pbPreamble + "\n\n" + toolContext
			} else {
				toolContext = pbPreamble
			}
		}

		if toolContext != "" {
			o.ConversationHistory.AddSystemMessage(toolContext)
		}
	}

	// ── RAG: inject semantically similar past context ─────────────────────────
	if o.RAGInjector != nil {
		o.RAGInjector.InjectContext(ctx, prompt, o.ConversationHistory)
	}

	// ── LLM-Driven Intent Gate & Swarm Routing ────────────────────────────────
	var intentCat adaptive.IntentCategory
	if o.Hooks != nil {
		gateResult := o.RunIntentGate(ctx, prompt)
		if gateResult != nil {
			intentCat = adaptive.IntentCategory(gateResult.Category)

			// Autonomously spawn background agents if the Intent Gate suggested them.
			if len(gateResult.SpawnAgents) > 0 && o.BackgroundAgents != nil {
				spawnedIds := []string{}
				for _, sa := range gateResult.SpawnAgents {
					// Use Consultant or Primary for background agent
					bgProv := o.Consultant()
					if bgProv == nil {
						bgProv = o.Primary()
					}
					id := o.BackgroundAgents.Spawn(ctx, BackgroundAgentSpec{
						Label:  sa.Label,
						Prompt: sa.Prompt,
					}, bgProv)
					spawnedIds = append(spawnedIds, id)
				}
				// Inform the primary AI that agents were spawned
				if len(spawnedIds) > 0 {
					msg := fmt.Sprintf("System Note: The Intent Gate has autonomously spawned %d background agents to assist you. Use the 'collect_agent' tool with these IDs to get their findings: %s", len(spawnedIds), strings.Join(spawnedIds, ", "))
					o.ConversationHistory.AddSystemMessage(msg)
					o.Logger.Info("Intent Gate spawned background agents", "count", len(spawnedIds), "ids", spawnedIds)
				}
			}
		} else {
			intentCat = adaptive.ClassifyIntent(prompt)
		}

		o.Hooks.FireAsync(ctx, hooks.EventIntentDetected, hooks.Payload{
			Extra: map[string]interface{}{
				"category": string(intentCat),
				"label":    adaptive.CategoryLabel(intentCat),
			},
		})
		o.Logger.Debug("Intent classified",
			"category", string(intentCat),
			"label", adaptive.CategoryLabel(intentCat),
		)
	}

	// ── Ralph Loop — begin tracking this execution attempt ──────────────────
	if o.RalphLoop != nil {
		o.RalphLoop.Begin()
	}

	// Build user message with consultation advice if provided
	userMessage := prompt
	if consultationAdvice != "" {
		userMessage = fmt.Sprintf("EXPERT CONSULTANT ADVICE:\n%s\n\nUSER REQUEST:\n%s", consultationAdvice, prompt)
	}

	// ── XSKILL Phase 2: inject task-relevant experiences and skill ──────────
	o.prepareXSkillContext(prompt)
	// ── SPARK Phase 2: inject TII/IDL/MotivationalCore context ─────────────
	o.prepareSPARKContext()

	// Add user message to history
	o.ConversationHistory.AddUserMessage(userMessage)

	// ── CacheAdvisor: compute and apply provider-specific caching hints ──────
	if o.CacheAdvisor != nil {
		if primary := o.Primary(); primary != nil {
			systemPrompt := o.buildSystemPrompt()
			msgs := o.ConversationHistory.GetMessages()
			providerID := string(primary.ID())
			model := primary.GetMetadata().ID
			hints := o.CacheAdvisor.Advise(providerID, model, systemPrompt, msgs)

			// Apply Gemini cached content name to the GeminiProvider.
			if hints.GeminiCachedContentName != "" {
				if gp, ok := primary.(interface{ SetCachedContent(string) }); ok {
					gp.SetCachedContent(hints.GeminiCachedContentName)
				}
			} else if hints.SystemPromptChanged {
				// System prompt changed — create a new Gemini cache entry async.
				gc := o.CacheAdvisor.GeminiCacheClient()
				if gc != nil {
					// Capture primary before spawning goroutine to avoid race condition
					prov := primary
					o.eg.Go(func() error {
						name, err := gc.Create(o.rootCtx, systemPrompt)
						if err != nil {
							o.Logger.Warn("Gemini cache create failed", "error", err)
							return err
						}
						if name != "" {
							o.CacheAdvisor.RecordGeminiCacheName(name)
							if gp, ok := prov.(interface{ SetCachedContent(string) }); ok {
								gp.SetCachedContent(name)
							}
						}
						return nil
					})
				}
			}
		}

		// Refresh Gemini TTL proactively.
		if gc := o.CacheAdvisor.GeminiCacheClient(); gc != nil {
			gc.RefreshIfNeeded(context.Background())
		}
	}

	// Persist user turn (fire-and-forget — never blocks the streaming path).
	if o.PersistStore != nil {
		go func(content string) { //nolint:errcheck
			_ = o.PersistStore.SaveTurn(context.Background(), "user", content, nil)
		}(prompt) // persist original prompt, not the consultant-augmented form
	}

	// ── Context enforcement: compress first, then safety-truncate ───────────
	// Correct order: compaction reduces tokens before we hard-truncate, which
	// avoids dropping messages that could have been summarised.
	maxContextTokens := o.safeContextLimit()
	if o.TieredCompactor != nil {
		_ = o.TieredCompactor.Check(ctx, o.ConversationHistory) // trim + SENSE-compress
	}
	o.ConversationHistory.TruncateToTokenLimit(maxContextTokens) // safety brake
	o.ConversationHistory.RepairOrphanedPairs()                  // fix dangling tool calls

	// Capture current provider ID for cascade detection.
	currentProviderID := ""
	if primary := o.Primary(); primary != nil {
		currentProviderID = string(primary.ID())
	}

	// ── SRE Phase 0: Grounding — must run before any LLM call ────────────────
	// Extracts semantic facts from the prompt before any planning or tool use begins.
	if o.SRE != nil && o.SRE.Enabled() {
		// Emit status: grounding extraction
		o.invokeStatus("grounding", "Grounding task and extracting anchors...", 0, "")
		_ = o.prepareGrounding(ctx, prompt)
	} else {
		// Emit status: pipeline analysis
		o.invokeStatus("pipeline", "Analyzing input...", 0, "")
	}

	// ── IngressFilter: prune low-information content from the prompt ─────────
	// The pruned prompt goes to the LLM; the guard decides which version ARC sees.
	arcPrompt := prompt // ARC sees this; may be raw if guard raises evasion risk
	if o.IngressFilter != nil {
		pruned, stats := o.IngressFilter.Prune(prompt)
		if stats.SavedRunes > 0 {
			o.Logger.Debug("IngressFilter savings",
				"saved_runes", stats.SavedRunes,
				"saved_pct", fmt.Sprintf("%.1f%%", stats.SavedPct),
			)
		}
		// IngressGuard: if pruning strips too much semantic signal, use raw
		// prompt for ARC routing to prevent classifier evasion.
		if o.IngressGuard != nil {
			gr := o.IngressGuard.Validate(prompt, pruned)
			if gr.EvasionRisk {
				o.Logger.Warn("IngressGuard: evasion risk detected — routing ARC with raw prompt",
					"similarity", fmt.Sprintf("%.2f", gr.Similarity))
				// ARC gets raw; LLM still gets pruned (saves tokens, keeps routing safe).
			} else {
				arcPrompt = pruned
			}
		} else {
			arcPrompt = pruned
		}
		// The user message added to history uses the pruned version.
		prompt = pruned
	}

	// ── ARC: classify prompt and compute platform-aware resource budget ──────
	maxTurns := 15 // Increased from 10 to allow more complex research
	var arcRoute adaptive.RouteDecision
	if o.Intelligence != nil {
		arcRoute = o.Intelligence.Route(arcPrompt)
		o.Logger.Info("ARC routing decision (streaming)",
			"workflow", arcRoute.Classification.String(),
			"max_tool_calls", arcRoute.Budget.MaxToolCalls,
			"temperature", arcRoute.Budget.Temperature,
		)
		if arcRoute.Budget.MaxToolCalls > 0 {
			maxTurns = arcRoute.Budget.MaxToolCalls
		}
	}

	// ── SRE: Reset phase engine + anchor state for this task ────────────────
	// Run ensemble for Analytical/Agentic workflows.
	if o.SRE != nil {
		o.SRE.Reset()
		// Multi-trajectory ensemble (Analytical/Agentic only)
		o.runEnsembleIfNeeded(ctx, arcRoute)
	}

	// Multi-turn tool execution loop
	const maxSENSEInjections = 3 // Increased from 2
	senseInjections := 0
	turnsWithOnlyTools := 0
	var fullResponse strings.Builder
	taskCompleted := false // set to true on any normal break; false = hit maxTurns

	for turn := 0; turn < maxTurns; turn++ {
		o.Logger.Info("Executing AI turn", "turn", turn+1, "max_turns", maxTurns)

		if o.EnableWatchdog {
			o.printWatchdogState(fmt.Sprintf("Turn %d", turn+1), fmt.Sprintf("History messages: %d", o.ConversationHistory.Count()))
		}

		// Inject progress enforcement if the AI has been silent for too long
		if turnsWithOnlyTools >= 3 {
			o.Logger.Warn("Silent tool loop detected — injecting progress request")
			o.ConversationHistory.AddSystemMessage("SYSTEM: You have been executing tools for 3 turns without providing any text output to the user. Please provide a brief status update on your progress before continuing with more tools.")
			turnsWithOnlyTools = 0 // reset after injection
		}

		// Create a writer that calls the stream callback
		monitor := NewStreamMonitor()
		ctxWithCancel, cancel := context.WithCancel(ctx)

		streamWriter := &streamCallbackWriter{
			ctx:                  ctxWithCancel,
			callback:             streamCallback,
			buffer:               &fullResponse,
			monitor:              monitor,
			cancel:               cancel,
			orchestrator:         o,
			interventionCallback: interventionCallback,
			relay:                o.Relay,
			thinkingCallback:     getThinkingCallbackFromAtomic(o),
			statusUpdateFreq:     10, // emit status every 10 tokens
			suppressor:           o.MessageSuppressor, // Apply message suppression if configured
		}

		// Apply thinking budget to provider if supported.
		primaryForTurn := o.Primary()
		if o.ThinkingBudget > 0 {
			// Use interface method if provider supports thinking budget
			if tbp, ok := primaryForTurn.(ai.ThinkingBudgetProvider); ok {
				tbp.SetThinkingBudget(o.ThinkingBudget)
			}
		}

		// ── SRE: inject CoS phase role + working memory block ────────────────
		if o.SRE != nil {
			o.prepareSREContext(turn)

			// Emit status update for current SRE phase
			phase := o.SRE.CurrentPhase()
			phaseStr := phase.String() // "HYPOTHESIS" / "PRUNE" / "CONVERGE"
			label := fmt.Sprintf("[SRE: %s]", phaseStr)
			description := o.SRE.CurrentDescriptiveLabel()

			if description != "" {
				// Emit status with SRE phase label and descriptive text
				o.invokeStatus("sre_"+strings.ToLower(phaseStr), label+" "+description, 0, "")
			}
		}

		// ── Budget Guard: estimate cost and block/warn before the API call ─────
		if o.BudgetGuard != nil {
			provID := ""
			modelID := ""
			if primary := o.Primary(); primary != nil {
				provID = string(primary.ID())
				modelID = primary.GetMetadata().ID
			}
			histToks := o.ConversationHistory.EstimateTokens()
			dec := o.BudgetGuard.CheckAndTrack(provID, modelID, histToks, len(prompt)/4)
			if dec.Action == BudgetBlock {
				cancel()
				return fmt.Errorf("budget exceeded: %s", dec.Message)
			}
			if dec.Action == BudgetWarn && streamCallback != nil {
				streamCallback("\n⚠ " + dec.Message + "\n")
			}
		}

		// Call AI with streaming
		startTime := time.Now()
		streamErr := primaryForTurn.StreamWithHistory(ctxWithCancel, o.ConversationHistory, streamWriter)
		latency := time.Since(startTime)
		cancel() // Release context resources for this turn immediately

		// ── Context window + billing tracking ────────────────────────────────
		// Mirror the updateContextMgr() logic from ExecuteTaskWithTools so the
		// streaming path keeps the ContextManager and Billing in sync.
		if primary := o.Primary(); primary != nil {
			if ur, ok := primary.(ai.UsageReporter); ok {
				u := ur.LastUsage()
				provID := string(primary.ID())
				modelID := primary.GetMetadata().ID

			// Record provider latency metric
			if o.Observability != nil {
				o.Observability.RecordProviderLatency(provID, modelID, latency)
			}

			if o.ContextMgr != nil {
				o.ContextMgr.UpdateFromUsage(TokenUsage{
					InputTokens:  u.PromptTokens,
					OutputTokens: u.CompletionTokens,
					ProviderID:   provID,
					ModelID:      modelID,
				})
			}
			if o.Billing != nil {
				cost := o.Billing.CalculateCost(provID, modelID, u.PromptTokens, u.CompletionTokens)
				o.Billing.TrackTurn(provID, modelID, u.PromptTokens, u.CompletionTokens)

				// Record cost metric
				if o.Observability != nil {
					o.Observability.RecordCost(provID, modelID, u.PromptTokens, u.CompletionTokens, cost)
				}
			}
		}
		}
		// ALWAYS commit whatever was streamed to history, even on partial / error.
		// Without this, an interrupted response is silently dropped and the model
		// starts the next turn with no recollection of what it was doing.
		response := fullResponse.String()
		// Apply output filter: strip raw tool outputs on normal queries
		if shouldSuppressToolOutput(userMessage) {
			response = stripToolOutputPatterns(response)
		}

		if response != "" {
			marker := ""
			if streamErr != nil {
				// Check if it was our watchdog
				if ctxWithCancel.Err() == context.Canceled {
					marker = fmt.Sprintf("\n\n[SYSTEM INTERVENTION: %s]", monitor.GetDiagnostics())
					// Reset error if we caused it intentionally
					streamErr = nil
				} else {
					marker = "\n\n[Response interrupted — resuming from here on next turn]"
				}
			}
			o.ConversationHistory.AddAssistantMessage(response + marker)
			o.ConversationHistory.RepairOrphanedPairs() // fix dangling tool calls after interruption
			// SPARK: observe completed response for MotivationalCore EWMA.
			if o.SPARK != nil && response != "" {
				o.SPARK.ObserveResponse(response)
			}

			// Persist assistant turn (fire-and-forget).
			if o.PersistStore != nil {
				go func(content string) { //nolint:errcheck
					_ = o.PersistStore.SaveTurn(context.Background(), "assistant", content, nil)
				}(response)
			}
		}

		if streamErr != nil {
			// Emit SENSE trace events for provider-level failures.
			if o.SENSETracer != nil {
				errMsg := streamErr.Error()
				if isContextOverflowErr(errMsg) {
					ctxTokens := 0
					if o.ContextMgr != nil {
						ctxTokens = o.ContextMgr.InputTokens()
					}
					o.SENSETracer.LogContextOverflow(currentProviderID, ctxTokens, errMsg)
				} else {
					o.SENSETracer.LogProviderError(currentProviderID, "", errMsg)
				}
			}
			// On first turn, try provider cascade for outage-type errors.
			if isProviderOutage(streamErr) && turn == 0 && currentProviderID != "" {
				retryable, msg := o.RunProviderCascade(ctx, currentProviderID)
				if streamCallback != nil {
					streamCallback("[__GORKBOT_STREAM_RETRY__]")
					streamCallback(fmt.Sprintf("\n\n[%s]\n\n", msg))
				}
				if retryable {
					// Update provider ID in case this turn also fails.
					if primary := o.Primary(); primary != nil {
						currentProviderID = string(primary.ID())
					}
					fullResponse.Reset()
					continue
				}
			}
			o.Logger.Error("Primary streaming failed", "error", streamErr, "turn", turn+1)
			return streamErr
		}

		// Parse tool requests
		toolRequests := tools.ParseToolRequests(response)

		// ── SRE: correction check (Phase 2+ only) ─────────────────────────────
		// Detect response deviations from working memory anchors and backtrack if needed.
		if o.SRE != nil && o.SRE.CurrentPhase() > sre.SREPhaseHypothesis {
			o.appendSRECorrectionCheck(response)
		}

		if len(toolRequests) == 0 {
			// Safety net: if the response contained [TOOL_CALL] blocks but the
			// parser extracted nothing from them (malformed names etc.), inject a
			// one-time format correction instead of letting SENSE retry blindly.
			if senseInjections == 0 && containsMalformedToolCall(response) {
				o.Logger.Warn("Detected unsupported tool call format — injecting correction", "turn", turn+1)
				formatMsg := "Your previous response used an unsupported tool call format " +
					"(e.g. [TOOL_CALL] tags or arrow syntax). " +
					"You MUST use markdown JSON code blocks only:\n" +
					"```json\n{\"tool\": \"tool_name\", \"parameters\": {\"key\": \"value\"}}\n```\n" +
					"Please retry with the correct format."
				o.ConversationHistory.AddUserMessage(formatMsg)
				if o.TieredCompactor != nil {
					_ = o.TieredCompactor.Check(ctx, o.ConversationHistory)
				}
				o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)
				o.ConversationHistory.RepairOrphanedPairs()
				if streamCallback != nil {
					streamCallback("[__GORKBOT_STREAM_RETRY__]")
					streamCallback("\n\n_[System: Detected unsupported tool call format, retrying...]_ \n\n")
				}
				fullResponse.Reset()
				continue
			}

			// No tools requested, we're done
			o.Logger.Info("No tool requests found, task complete", "turn", turn+1)

			// Signal completion to remote observers.
			if o.Relay != nil {
				o.Relay.SendComplete()
			}

			// Record adaptive routing outcome (success).
			if o.Feedback != nil {
				category := router.ClassifyQuery(prompt)
				modelID := ""
				if primary := o.Primary(); primary != nil {
					modelID = primary.GetMetadata().ID
				}
				o.Feedback.RecordOutcome(category, modelID, 1.0, true)
			}

			// Trigger Subconscious Reflection
			// If the agent thinks it's done, we check if it actually is.
			injected, err := o.AnalyzeAgency(ctx, response)
			if err != nil {
				o.Logger.Warn("Agency analysis error", "error", err)
			}
			if injected {
				senseInjections++
				if senseInjections > maxSENSEInjections {
					o.Logger.Info("SENSE injection cap reached, stopping early", "cap", maxSENSEInjections)
					taskCompleted = true
					break
				}
				o.Logger.Info("SENSE intervention triggered, continuing streaming execution loop", "injection", senseInjections)
				if streamCallback != nil {
					streamCallback("[__GORKBOT_STREAM_RETRY__]")
				}
				fullResponse.Reset()
				continue
			}

			// ── Vector store: index completed turn for future RAG retrieval ────
			if o.VectorStore != nil && o.SessionID != "" {
				go o.VectorStore.IndexTurn(context.Background(), o.SessionID, "user", prompt)
				go o.VectorStore.IndexTurn(context.Background(), o.SessionID, "assistant", response)
			}

			taskCompleted = true
			break
		}

		o.Logger.Info("Found tool requests", "count", len(toolRequests), "turn", turn+1)

		// Update silent turn counter
		if len(strings.TrimSpace(tools.StripToolCalls(response))) < 5 {
			turnsWithOnlyTools++
		} else {
			turnsWithOnlyTools = 0
		}

		// Execute tools — in parallel when more than one is requested.
		// Results are collected into a slice pre-sized to match request order
		// so the AI always receives results in the same order it requested them.
		toolResults := make([]string, len(toolRequests))

		if len(toolRequests) == 1 {
			// Fast path: single tool, no goroutine overhead.
			req := toolRequests[0]
			o.Logger.Info("Executing tool", "tool", req.ToolName, "index", 1, "total", 1)
			if toolStartCallback != nil {
				toolStartCallback(req.ToolName, req.Parameters)
			}
			if o.Relay != nil {
				o.Relay.SendToolStart(req.ToolName)
			}
			xskillStart := time.Now()
			result, err := o.ExecuteTool(ctx, req)
			o.appendXSkillStep(req, result, err, xskillStart)
			o.appendSPARKToolEvent(req, result, err, xskillStart)
			o.anchorToolResult(req.ToolName, result.Output, result.Success)
			if err != nil {
				o.Logger.Error("Tool execution error", "tool", req.ToolName, "error", err)
				result = &tools.ToolResult{Success: false, Error: err.Error()}
				if o.RalphLoop != nil {
					o.RalphLoop.RecordFailure(req.ToolName, err.Error())
				}
			} else if !result.Success && o.RalphLoop != nil && result.Error != "" {
				o.RalphLoop.RecordFailure(req.ToolName, result.Error)
			}
			if toolCallback != nil {
				toolCallback(req.ToolName, result)
			}
			if o.Relay != nil {
				o.Relay.SendToolDone(req.ToolName)
			}
			if result.Success {
				toolResults[0] = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: true\nOutput:\n%s\n</tool_result>",
					req.ToolName, result.Output)
			} else {
				toolResults[0] = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: false\nError: %s\n</tool_result>",
					req.ToolName, result.Error)
			}
		} else {
			// Parallel path: dispatch all tools concurrently, preserve result order.
			var wg sync.WaitGroup
			var ralphMu sync.Mutex // guards RalphLoop calls
			for i, req := range toolRequests {
				wg.Add(1)
				go func(idx int, req tools.ToolRequest) {
					defer wg.Done()
					o.Logger.Info("Executing tool (parallel)", "tool", req.ToolName, "index", idx+1, "total", len(toolRequests))
					if toolStartCallback != nil {
						toolStartCallback(req.ToolName, req.Parameters)
					}
					if o.Relay != nil {
						o.Relay.SendToolStart(req.ToolName)
					}
					xskillStart := time.Now()
					result, err := o.ExecuteTool(ctx, req)
					o.appendXSkillStep(req, result, err, xskillStart)
					o.appendSPARKToolEvent(req, result, err, xskillStart)
					o.anchorToolResult(req.ToolName, result.Output, result.Success)
					if err != nil {
						o.Logger.Error("Tool execution error", "tool", req.ToolName, "error", err)
						result = &tools.ToolResult{Success: false, Error: err.Error()}
						ralphMu.Lock()
						if o.RalphLoop != nil {
							o.RalphLoop.RecordFailure(req.ToolName, err.Error())
						}
						ralphMu.Unlock()
					} else if !result.Success && result.Error != "" {
						ralphMu.Lock()
						if o.RalphLoop != nil {
							o.RalphLoop.RecordFailure(req.ToolName, result.Error)
						}
						ralphMu.Unlock()
					}
					if toolCallback != nil {
						toolCallback(req.ToolName, result)
					}
					if o.Relay != nil {
						o.Relay.SendToolDone(req.ToolName)
					}
					if result.Success {
						toolResults[idx] = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: true\nOutput:\n%s\n</tool_result>",
							req.ToolName, result.Output)
					} else {
						toolResults[idx] = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: false\nError: %s\n</tool_result>",
							req.ToolName, result.Error)
					}
				}(i, req)
			}
			wg.Wait()
		}

		// Build tool results message and add to history
		toolResultsMessage := "Here are the results from the tools you requested:\n\n"
		toolResultsMessage += strings.Join(toolResults, "\n\n")
		toolResultsMessage += "\n\nPlease continue with the task based on these results. If you need more tools, request them. Otherwise, provide your final response to the user."

		// Add tool results as user message
		o.ConversationHistory.AddUserMessage(toolResultsMessage)

		// Enforce context limits after adding tool results: compress first, then truncate.
		if o.TieredCompactor != nil {
			_ = o.TieredCompactor.Check(ctx, o.ConversationHistory)
		}
		o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)
		o.ConversationHistory.RepairOrphanedPairs()

		// ── Ralph Loop: check if we should trigger a retry ─────────────────
		// After each tool-execution turn, evaluate whether failure thresholds
		// have been crossed and it's time for a self-referential retry.
		if o.RalphLoop != nil && o.RalphLoop.ShouldTrigger() {
			o.Logger.Info("Ralph Loop triggered",
				"iteration", o.RalphLoop.IterationsUsed()+1,
				"max", o.RalphLoop.MaxIterations(),
				"summary", o.RalphLoop.Summary(),
			)

			// Restore git workspace before retrying, if we have a checkpoint.
			if o.Workspace != nil && workspaceHash != "" {
				o.Logger.Info("Ralph Loop restoring workspace checkpoint", "hash", workspaceHash)
				if err := o.Workspace.RestoreCheckpoint(workspaceHash); err != nil {
					o.Logger.Warn("Workspace rollback failed", "error", err)
				} else {
					o.Logger.Info("Workspace successfully rolled back for fresh attempt")
				}
			}

			// Fire the ralph_loop_triggered hook.
			if o.Hooks != nil {
				o.Hooks.FireAsync(ctx, hooks.EventRalphLoopTriggered, hooks.Payload{
					Extra: map[string]interface{}{
						"iteration": o.RalphLoop.IterationsUsed() + 1,
						"summary":   o.RalphLoop.Summary(),
					},
				})
			}

			// Commit the current attempt and build a retry meta-prompt.
			o.RalphLoop.Commit()
			retryPrompt := o.RalphLoop.BuildRetryPrompt(prompt)

			// Reset response builder and inject the meta-prompt as a new
			// user message so the AI gets fresh guidance on the next turn.
			fullResponse.Reset()
			o.ConversationHistory.AddUserMessage(retryPrompt)
			if o.TieredCompactor != nil {
				_ = o.TieredCompactor.Check(ctx, o.ConversationHistory)
			}
			o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)
			o.ConversationHistory.RepairOrphanedPairs()

			// Stream a visible indicator to the user.
			if streamCallback != nil {
				streamCallback("[__GORKBOT_STREAM_RETRY__]")
				streamCallback(fmt.Sprintf("\n\n_[Ralph Loop: retrying with fresh strategy (attempt %d/%d)]_\n\n",
					o.RalphLoop.IterationsUsed(), o.RalphLoop.MaxIterations()))
			}
			continue
		}

		// Reset for next turn
		fullResponse.Reset()
	}

	// ── Final Summary Enforcement ───────────────────────────────────────────
	// Only runs when we exhausted maxTurns without a successful task break.
	// Normal task completion sets taskCompleted=true and skips this block.
	if !taskCompleted {
		o.Logger.Warn("Max turns reached without task completion — forcing final summary")
		summaryPrompt := "SYSTEM: You have reached the maximum turn limit for this task. Please provide a concise final summary of everything you have accomplished so far and the current status of the user's request. Do not attempt to use any more tools."
		o.ConversationHistory.AddUserMessage(summaryPrompt)

		// Create a non-looping final generation attempt
		if primary := o.Primary(); primary != nil {
			finalResponse, finalErr := primary.GenerateWithHistory(ctx, o.ConversationHistory)
			if finalErr == nil && finalResponse != "" {
				if streamCallback != nil {
					streamCallback("\n\n**[Max Turns Reached — Final Summary]**\n\n")
					streamCallback(finalResponse)
				}
				o.ConversationHistory.AddAssistantMessage(finalResponse)
			}
		}
	}

	// Commit Ralph Loop state at end of execution.
	if o.RalphLoop != nil {
		o.RalphLoop.Commit()
	}

	// ── XSKILL Phase 1: learn from this task's trajectory asynchronously ────
	o.launchXSkillAccumulation()
	// ── SPARK Phase 1: trigger improvement cycle ──────────────────────────
	o.launchSPARKIntrospection()
	// ── SI Phase 1: feed autonomous self-improvement with task outcome ─────
	// Get last assistant response from history (populated in both normal and max-turns paths)
	lastResponse := ""
	if msgs := o.ConversationHistory.GetMessages(); len(msgs) > 0 {
		if lastMsg := msgs[len(msgs)-1]; lastMsg.Role == "assistant" {
			lastResponse = lastMsg.Content
		}
	}
	o.launchSIPostTask(lastResponse)

	// Trigger ToolForge checks asynchronously
	if o.Crystallizer != nil {
		go o.Crystallizer.CheckAndForge(context.Background())
	}

	return nil
}

// safeContextLimit returns 90% of the orchestrator's context window so that
// TruncateToTokenLimit has a small safety margin before hitting the hard limit.
// Falls back to 115200 (90% of 128k) when ContextMgr is not wired.
func (o *Orchestrator) safeContextLimit() int {
	if o.ContextMgr != nil && o.ContextMgr.MaxTokens() > 0 {
		return (o.ContextMgr.MaxTokens() * 9) / 10
	}
	return 115200
}

// containsMalformedToolCall returns true when a response contains tool-call
// attempts in a format that ParseToolRequests cannot handle, such as [TOOL_CALL]
// tags, <tool_call> XML, or <tool_use> tags.
func containsMalformedToolCall(response string) bool {
	return strings.Contains(response, "[TOOL_CALL]") ||
		strings.Contains(response, "<tool_call>") ||
		strings.Contains(response, "<tool_use>")
}

// streamCallbackWriter implements io.Writer and calls the callback for each write
type streamCallbackWriter struct {
	ctx                  context.Context // turn-scoped context (for consultant checks)
	callback             StreamCallback
	buffer               *strings.Builder
	monitor              *StreamMonitor
	cancel               context.CancelFunc
	orchestrator         *Orchestrator
	interventionCallback InterventionCallback
	relay                *collab.Relay // nil when session sharing is inactive

	// thinking-block sentinel routing (Anthropic extended thinking)
	thinkingCallback StreamCallback  // nil = no thinking panel wired
	inThinking       bool            // true while between \x02 and \x03
	thinkingBuf      strings.Builder // accumulates partial thinking text within one Write call

	// token counting and status updates for the TUI status line
	tokenCount        int       // tracks tokens received from LLM
	firstTokenTime    time.Time // tracks when first token arrived
	modelID           string    // model being used (e.g. "grok-4-fast-")
	statusUpdateFreq  int       // throttle status updates: emit every N tokens (default 10)
	lastStatusTokens  int       // tokens at last status update

	// Message suppression for filtering internal system messages
	suppressor *MessageSuppressionMiddleware // nil = no suppression (all messages passed through)
}

func (w *streamCallbackWriter) Write(p []byte) (n int, err error) {
	s := string(p)

	if s == "[__GORKBOT_STREAM_RETRY__]" {
		w.buffer.Reset()
		if w.callback != nil {
			w.callback(s)
		}
		return len(p), nil
	}

	// ── Extended thinking sentinel routing ─────────────────────────────────
	// anthropic.go emits \x02 before thinking text and \x03 after it.
	// We parse them out here so thinking tokens never reach the main stream
	// or the response buffer (they're for the separate thinking panel only).
	if w.thinkingCallback != nil || w.inThinking {
		// Walk the token character by character to split on sentinel boundaries.
		// Most tokens won't contain sentinels so we fast-path the common case.
		const startSentinel = '\x02'
		const endSentinel = '\x03'
		hasStart := strings.ContainsRune(s, startSentinel)
		hasEnd := strings.ContainsRune(s, endSentinel)
		if w.inThinking || hasStart || hasEnd {
			var mainText strings.Builder
			for _, ch := range s {
				switch {
				case ch == startSentinel:
					w.inThinking = true
				case ch == endSentinel:
					// Flush anything buffered in thinking mode, then signal done.
					if w.thinkingCallback != nil && w.thinkingBuf.Len() > 0 {
						w.thinkingCallback(w.thinkingBuf.String())
						w.thinkingBuf.Reset()
					}
					// Signal end of thinking block.
					if w.thinkingCallback != nil {
						w.thinkingCallback("\x03") // ThinkingDone sentinel forwarded to TUI
					}
					w.inThinking = false
				case w.inThinking:
					// Accumulate in thinking buffer; flush in batches for smooth streaming.
					w.thinkingBuf.WriteRune(ch)
				default:
					mainText.WriteRune(ch)
				}
			}
			// Flush partial thinking buffer eagerly (each Write is already batched).
			if w.inThinking && w.thinkingCallback != nil && w.thinkingBuf.Len() > 0 {
				w.thinkingCallback(w.thinkingBuf.String())
				w.thinkingBuf.Reset()
			}
			// Replace s with the non-thinking remainder.
			s = mainText.String()
			if s == "" {
				return len(p), nil // nothing left for the main stream
			}
		}
	}
	// ── end sentinel routing ────────────────────────────────────────────────

	// Monitor the stream for issues
	if w.monitor != nil {
		severity := w.monitor.WriteToken(s)

		// Handle different severity levels
		if severity == SeverityCritical {
			if w.cancel != nil {
				w.cancel()
			}
			return len(p), fmt.Errorf("stream halted by watchdog (Critical Loop)")
		} else if severity == SeverityWarning {
			// INTELLIGENT CHECK
			// 1. Ask Consultant if this is valid
			if w.orchestrator != nil {
				if consultant := w.orchestrator.Consultant(); consultant != nil {
					// We take a snapshot of recent buffer
					content := w.buffer.String()
					sample := content
					if len(content) > 500 {
						sample = content[len(content)-500:]
					}

					prompt := fmt.Sprintf("The following AI output triggered a repetition warning. Is this valid structured output (like a list, log, or code) or a pathological loop? Reply ONLY with 'VALID' or 'LOOP'.\n\nOUTPUT SAMPLE:\n%s", sample)

					// Use the turn's context so the check is cancelled if the user aborts.
					verdict, _ := consultant.Generate(w.ctx, prompt)

				if strings.Contains(strings.ToUpper(verdict), "LOOP") {
					// Confirmed loop. Escalate to User.
					if w.interventionCallback != nil {
						response := w.interventionCallback(severity, sample)

						switch response {
						case InterventionStop:
							if w.cancel != nil {
								w.cancel()
							}
							return len(p), fmt.Errorf("stream halted by user intervention")
						case InterventionAllowSession:
							// Disable monitor for this writer
							w.monitor = nil
						case InterventionContinue:
							// Just continue, maybe reset monitor stats
							// We essentially ignore this warning instance
						}
					}
				}
			}
			}
		}
	}

	// Apply message suppression if configured, then call the callback
	if w.suppressor != nil {
		s = w.suppressor.ProcessStreamingToken(s)
	}

	if w.callback != nil && s != "" {
		w.callback(s)
	}

	// ── Token counting and status updates ──────────────────────────────────────
	// Track token count and emit status updates to the TUI's authoritative status line.
	// On first token, emit "Thinking..." and set the modelID.
	// On subsequent tokens, emit status updates every N tokens with token count.
	w.tokenCount++

	if w.tokenCount == 1 && w.firstTokenTime.IsZero() {
		// First token received — emit "Thinking..." status
		w.firstTokenTime = time.Now()

		// Get model ID from orchestrator if available
		if w.orchestrator != nil {
			if primary := w.orchestrator.Primary(); primary != nil {
				w.modelID = primary.GetMetadata().ID
			}
		}

		if w.orchestrator != nil {
			w.orchestrator.invokeStatus("thinking", "Thinking...", 0, "")
		}
	} else if w.tokenCount > w.lastStatusTokens+w.statusUpdateFreq {
		// Emit status update with token count every N tokens
		if w.orchestrator != nil {
			w.orchestrator.invokeStatus("thinking", "Thinking...", w.tokenCount, w.modelID)
			w.lastStatusTokens = w.tokenCount
		}
	}

	// Broadcast to remote observers if session sharing is active.
	if w.relay != nil {
		w.relay.SendToken(s)
	}

	// Also buffer for full response
	if w.buffer != nil {
		w.buffer.WriteString(s)
	}

	return len(p), nil
}
