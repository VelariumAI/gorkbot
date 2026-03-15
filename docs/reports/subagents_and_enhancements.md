# Engineering Report: Parallel Sub-Agents & Architectural Enhancements

## Summary of Changes

1.  **Parallel Sub-Agents Architecture**
    *   **Async Execution**: Refactored `pkg/subagents` to launch agents in detached goroutines, returning an immediate ID for tracking.
    *   **New Tools**: Implemented `spawn_agent` (async) and `check_agent_status` tools, enabling the main agent to delegate tasks and monitor progress without blocking.
    *   **State Management**: Updated `Registry` to manage a persistent `subagents.Manager` instance, ensuring agent state survives across tool calls within a session.
    *   **API Consistency**: Sub-agents automatically inherit the primary or consultant AI provider configuration.

2.  **Engram Store (Full Implementation)**
    *   **Persistence**: Removed the confidence threshold (`< 0.85`) in `pkg/sense/engrams.go`. All explicit `record_engram` calls are now persisted to long-term memory, honoring the user's intent.

3.  **HITL Removal**
    *   **Logic Disabled**: Modified `internal/engine/sense_hitl.go` to disable the `IsHighStakes` check and make `GateToolExecution` a pass-through, effectively removing the human-in-the-loop requirement.

## Verification
*   **Build**: Successfully built `gorkbot` binary.
*   **Functional Test**: Confirmed `spawn_agent` creates a background task and returns an ID. Confirmed `check_agent_status` retrieves the task status.
*   **Note**: The test revealed a potential configuration issue with the default Gemini model ID (`gemini-2.5-flash-preview-09-2025` returned 404), which should be updated in your `.env` or `gorkbot` config if you encounter issues. The tool logic itself is correct.

## Next Steps
*   Update your `.env` or `gorkbot` configuration to use a valid model ID if you see 404 errors.
*   Explore creating more specialized agents in `pkg/subagents` now that the parallel infrastructure is in place.
