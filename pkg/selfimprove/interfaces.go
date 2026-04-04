package selfimprove

import (
	"context"
	"time"
)

// SPARKFacade provides read-only access to SPARK state.
type SPARKFacade interface {
	// GetLastState returns a snapshot of the last SPARK state.
	GetLastState() *SPARKStateSnapshot
}

// SPARKStateSnapshot is a read-only copy of SPARK's diagnostic state.
type SPARKStateSnapshot struct {
	DriveScore       float64 // 0.0-1.0
	IDLDebt          int     // outstanding improvement items
	ActiveDirectives int     // number of active SPARK directives
}

// FreeWillFacade provides read-only and write access to Free Will state.
type FreeWillFacade interface {
	// AddObservation feeds a pattern observation to Free Will Engine.
	AddObservation(ctx context.Context, input FreeWillObsInput) error
	// GetPendingProposals returns current evolution proposals.
	GetPendingProposals() []FreeWillProposalSummary
}

// FreeWillObsInput describes an observation for the Free Will Engine.
type FreeWillObsInput struct {
	Context   string // domain/pattern description
	ToolName  string // tool executed
	Outcome   string // "success", "error", "timeout"
	Latency   int64  // milliseconds
	Confidence float64 // 0.0-1.0
}

// FreeWillProposalSummary is a summary of a proposed evolution.
type FreeWillProposalSummary struct {
	Target     string  // proposed change target
	Confidence float64 // 0-100
	Risk       float64 // 0.0-1.0
}

// HarnessFacade provides read-only access to harness state.
type HarnessFacade interface {
	// FailingCount returns the number of failing features.
	FailingCount() int
	// TotalCount returns the total number of tracked features.
	TotalCount() int
	// ActiveFeatureID returns the currently active feature being worked on.
	ActiveFeatureID() string
}

// ResearchFacade provides read-only access to research state.
type ResearchFacade interface {
	// BufferedCount returns the number of currently buffered research documents.
	BufferedCount() int
}

// ToolRegistryFacade allows tool execution.
type ToolRegistryFacade interface {
	// ExecuteTool runs a named tool with parameters.
	ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (string, error)
}

// NotifyFacade sends notifications to the user.
type NotifyFacade interface {
	// Notify sends a message to the TUI.
	Notify(msg string)
}

// ObservabilityFacade exposes SI metric recording.
// Implemented by *observability.ObservabilityHub in production.
// Implemented by mocks in tests.
type ObservabilityFacade interface {
	// Core metrics (Task 4.3A)
	RecordSICycleStart()
	RecordSIProposal()
	RecordSIAccepted()
	RecordSIRolledBack()
	RecordSIFailed()

	// Extended metrics (Task 4.3B)
	RecordSIExecutionLatency(latency time.Duration)
	RecordSIScoreDelta(delta float64)
	RecordSIGateRejectionReason(reason string)
	RecordSIToolError(toolName string)
	RecordSIRollbackLatency(latency time.Duration)
}

// RollbackStoreFacade provides atomic rollback capabilities for the SI pipeline.
// Implemented by *dag.RollbackStore in production.
// Implemented by test mocks.
type RollbackStoreFacade interface {
	Snapshot(id string, metadata []string) error
	Discard(id string) error
	Rollback(ctx context.Context, id string) error
}
