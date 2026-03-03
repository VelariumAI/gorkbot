package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/internal/arc"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/collab"
	"github.com/velariumai/gorkbot/pkg/hooks"
	"github.com/velariumai/gorkbot/pkg/router"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// StreamCallback is called for each token as it arrives
type StreamCallback func(token string)

// InterventionCallback is called when the watchdog needs human input
type InterventionCallback func(severity WatchdogSeverity, context string) InterventionResponse

// ExecuteTaskWithStreaming handles a user prompt with real-time streaming output.
// toolStartCallback (may be nil) is invoked just before each tool execution begins,
// enabling the TUI to show a live "in-progress" panel before results arrive.
func (o *Orchestrator) ExecuteTaskWithStreaming(ctx context.Context, prompt string, streamCallback StreamCallback, toolCallback func(string, *tools.ToolResult), toolStartCallback func(string), interventionCallback InterventionCallback) error {
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
			if o.Primary != nil {
				currentModel = o.Primary.GetMetadata().ID
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
	if needsConsult && o.Consultant != nil {
		o.Logger.Info("Triggering Specialty Consult...", "consultant", o.Consultant.Name())
		// Fire consultant_invoked hook.
		if o.Hooks != nil {
			o.Hooks.FireAsync(ctx, hooks.EventConsultantInvoked, hooks.Payload{
				Extra: map[string]interface{}{
					"trigger":   "complexity_or_keyword",
					"prompt_len": len(prompt),
				},
			})
		}

		if o.EnableWatchdog {
			o.printWatchdogState("Consultation", prompt)
		}

		// For consultation, we still use non-streaming since it's quick
		advice, err := o.Consultant.Generate(ctx, prompt)
		if err != nil {
			o.Logger.Error("Consultation failed", "error", err)
			consultationAdvice = ""
		} else if advice == "" {
			o.Logger.Warn("Consultant returned empty response")
			consultationAdvice = ""
		} else {
			consultationAdvice = advice
			o.Logger.Info("Consultation received", "length", len(advice))
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

		if toolContext != "" {
			o.ConversationHistory.AddSystemMessage(toolContext)
		}
	}

	// ── LLM-Driven Intent Gate & Swarm Routing ────────────────────────────────
	var intentCat arc.IntentCategory
	if o.Hooks != nil {
		gateResult := o.RunIntentGate(ctx, prompt)
		if gateResult != nil {
			intentCat = arc.IntentCategory(gateResult.Category)
			
			// Autonomously spawn background agents if the Intent Gate suggested them.
			if len(gateResult.SpawnAgents) > 0 && o.BackgroundAgents != nil {
				spawnedIds := []string{}
				for _, sa := range gateResult.SpawnAgents {
					// Use Consultant or Primary for background agent
					bgProv := o.Consultant
					if bgProv == nil {
						bgProv = o.Primary
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
			intentCat = arc.ClassifyIntent(prompt)
		}

		o.Hooks.FireAsync(ctx, hooks.EventIntentDetected, hooks.Payload{
			Extra: map[string]interface{}{
				"category": string(intentCat),
				"label":    arc.CategoryLabel(intentCat),
			},
		})
		o.Logger.Debug("Intent classified",
			"category", string(intentCat),
			"label", arc.CategoryLabel(intentCat),
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

	// Add user message to history
	o.ConversationHistory.AddUserMessage(userMessage)

	// Ensure history doesn't exceed context limit
	maxContextTokens := 100000
	o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)

	// Capture current provider ID for cascade detection.
	currentProviderID := ""
	if o.Primary != nil {
		currentProviderID = string(o.Primary.ID())
	}

	// ── ARC: classify prompt and compute platform-aware resource budget ──────
	maxTurns := 10 // default: prevent infinite loops
	if o.Intelligence != nil {
		arcDecision := o.Intelligence.Route(prompt)
		o.Logger.Info("ARC routing decision (streaming)",
			"workflow", arcDecision.Classification.String(),
			"max_tool_calls", arcDecision.Budget.MaxToolCalls,
			"temperature", arcDecision.Budget.Temperature,
		)
		if arcDecision.Budget.MaxToolCalls > 0 {
			maxTurns = arcDecision.Budget.MaxToolCalls
		}
	}

	// Multi-turn tool execution loop
	const maxSENSEInjections = 2 // cap runaway SENSE chains per query
	senseInjections := 0
	var fullResponse strings.Builder

	for turn := 0; turn < maxTurns; turn++ {
		o.Logger.Info("Executing AI turn", "turn", turn+1, "max_turns", maxTurns)

		if o.EnableWatchdog {
			o.printWatchdogState(fmt.Sprintf("Turn %d", turn+1), fmt.Sprintf("History messages: %d", o.ConversationHistory.Count()))
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
		}

		// Call AI with streaming
		streamErr := o.Primary.StreamWithHistory(ctxWithCancel, o.ConversationHistory, streamWriter)
		cancel() // Release context resources for this turn immediately

		// ── Context window + billing tracking ────────────────────────────────
		// Mirror the updateContextMgr() logic from ExecuteTaskWithTools so the
		// streaming path keeps the ContextManager and Billing in sync.
		if ur, ok := o.Primary.(ai.UsageReporter); ok {
			u := ur.LastUsage()
			provID := string(o.Primary.ID())
			modelID := o.Primary.GetMetadata().ID
			if o.ContextMgr != nil {
				o.ContextMgr.UpdateFromUsage(TokenUsage{
					InputTokens:  u.PromptTokens,
					OutputTokens: u.CompletionTokens,
					ProviderID:   provID,
					ModelID:      modelID,
				})
			}
			if o.Billing != nil {
				o.Billing.TrackTurn(provID, modelID, u.PromptTokens, u.CompletionTokens)
			}
		}

		// ALWAYS commit whatever was streamed to history, even on partial / error.
		// Without this, an interrupted response is silently dropped and the model
		// starts the next turn with no recollection of what it was doing.
		response := fullResponse.String()
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
		}

		if streamErr != nil {
			// On first turn, try provider cascade for outage-type errors.
			if isProviderOutage(streamErr) && turn == 0 && currentProviderID != "" {
				retryable, msg := o.RunProviderCascade(ctx, currentProviderID)
				if streamCallback != nil {
					streamCallback(fmt.Sprintf("\n\n[%s]\n\n", msg))
				}
				if retryable {
					// Update provider ID in case this turn also fails.
					if o.Primary != nil {
						currentProviderID = string(o.Primary.ID())
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
				o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)
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
				if o.Primary != nil {
					modelID = o.Primary.GetMetadata().ID
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
					break
				}
				o.Logger.Info("SENSE intervention triggered, continuing streaming execution loop", "injection", senseInjections)
				fullResponse.Reset()
				continue
			}

			break
		}

		o.Logger.Info("Found tool requests", "count", len(toolRequests), "turn", turn+1)

		// Execute all requested tools
		toolResults := []string{}
		for i, req := range toolRequests {
			o.Logger.Info("Executing tool", "tool", req.ToolName, "index", i+1, "total", len(toolRequests))

			// Notify TUI that this tool is starting (shows live panel before result arrives).
			if toolStartCallback != nil {
				toolStartCallback(req.ToolName)
			}
			if o.Relay != nil {
				o.Relay.SendToolStart(req.ToolName)
			}

			result, err := o.ExecuteTool(ctx, req)
			if err != nil {
				o.Logger.Error("Tool execution error", "tool", req.ToolName, "error", err)
				result = &tools.ToolResult{
					Success: false,
					Error:   err.Error(),
				}
				// Record failure in Ralph Loop tracker.
				if o.RalphLoop != nil {
					o.RalphLoop.RecordFailure(req.ToolName, err.Error())
				}
			} else if !result.Success {
				// Tool returned a failure result (not an error).
				if o.RalphLoop != nil && result.Error != "" {
					o.RalphLoop.RecordFailure(req.ToolName, result.Error)
				}
			}

			// Notify callback if provided
			if toolCallback != nil {
				toolCallback(req.ToolName, result)
			}
			if o.Relay != nil {
				o.Relay.SendToolDone(req.ToolName)
			}

			// Format tool result for AI
			var resultStr string
			if result.Success {
				resultStr = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: true\nOutput:\n%s\n</tool_result>",
					req.ToolName, result.Output)
			} else {
				resultStr = fmt.Sprintf("<tool_result tool=\"%s\">\nSuccess: false\nError: %s\n</tool_result>",
					req.ToolName, result.Error)
			}
			toolResults = append(toolResults, resultStr)
		}

		// Build tool results message and add to history
		toolResultsMessage := "Here are the results from the tools you requested:\n\n"
		toolResultsMessage += strings.Join(toolResults, "\n\n")
		toolResultsMessage += "\n\nPlease continue with the task based on these results. If you need more tools, request them. Otherwise, provide your final response to the user."

		// Add tool results as user message
		o.ConversationHistory.AddUserMessage(toolResultsMessage)

		// Ensure we don't exceed context limits after adding tool results
		o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)

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
			o.ConversationHistory.TruncateToTokenLimit(maxContextTokens)

			// Stream a visible indicator to the user.
			if streamCallback != nil {
				streamCallback(fmt.Sprintf("\n\n_[Ralph Loop: retrying with fresh strategy (attempt %d/%d)]_\n\n",
					o.RalphLoop.IterationsUsed(), o.RalphLoop.MaxIterations()))
			}
			continue
		}

		// Reset for next turn
		fullResponse.Reset()
	}

	// Commit Ralph Loop state at end of execution.
	if o.RalphLoop != nil {
		o.RalphLoop.Commit()
	}

	// Trigger ToolForge checks asynchronously
	if o.Crystallizer != nil {
		go o.Crystallizer.CheckAndForge(context.Background())
	}

	return nil
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
			if w.orchestrator != nil && w.orchestrator.Consultant != nil {
				// We take a snapshot of recent buffer
				content := w.buffer.String()
				sample := content
				if len(content) > 500 {
					sample = content[len(content)-500:]
				}

				prompt := fmt.Sprintf("The following AI output triggered a repetition warning. Is this valid structured output (like a list, log, or code) or a pathological loop? Reply ONLY with 'VALID' or 'LOOP'.\n\nOUTPUT SAMPLE:\n%s", sample)

				// Use the turn's context so the check is cancelled if the user aborts.
				verdict, _ := w.orchestrator.Consultant.Generate(w.ctx, prompt)

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

	// Call the stream callback with the token
	if w.callback != nil {
		w.callback(s)
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
