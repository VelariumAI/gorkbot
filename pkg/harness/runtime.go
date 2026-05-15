package harness

import (
	"context"
	"os"
	"strings"
	"time"
)

// Mode controls runtime harness behavior.
type Mode string

const (
	ModeOff   Mode = "off"
	ModeAudit Mode = "audit"
)

// ParseMode parses harness runtime mode values.
func ParseMode(raw string) Mode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "audit":
		return ModeAudit
	default:
		return ModeOff
	}
}

// Runtime wraps optional harness validation for runtime audit-only integration.
type Runtime struct {
	mode     Mode
	Registry *Registry
}

// NewRuntime creates a runtime harness wrapper.
func NewRuntime(mode Mode, registry *Registry) *Runtime {
	return &Runtime{
		mode:     ParseMode(string(mode)),
		Registry: registry,
	}
}

// NewRuntimeFromEnv builds runtime mode from GORKBOT_HARNESS_MODE.
func NewRuntimeFromEnv() *Runtime {
	return NewRuntime(ParseMode(os.Getenv("GORKBOT_HARNESS_MODE")), nil)
}

// Enabled reports whether audit mode is active.
func (r *Runtime) Enabled() bool {
	return r != nil && r.mode == ModeAudit
}

// Mode reports the normalized runtime mode.
func (r *Runtime) Mode() Mode {
	if r == nil {
		return ModeOff
	}
	return ParseMode(string(r.mode))
}

// Validate runs deterministic harness validation in audit mode.
// It never executes commands, network operations, or file mutations.
func (r *Runtime) Validate(ctx context.Context, artifact Artifact) (Report, error) {
	if !r.Enabled() {
		return Report{}, ErrHarnessRuntimeDisabled
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.Registry == nil {
		report := NewReport("harness.audit.unavailable", artifact.ID)
		report.Status = StatusUnsupported
		report.Results = []Result{{
			AssertionID: "runtime.registry",
			Status:      StatusUnsupported,
			Severity:    SeverityWarn,
			Message:     "harness audit registry unavailable",
			ReasonCode:  "harness_registry_unavailable",
			Evidence:    []Evidence{{Kind: "registry", Value: "missing"}},
		}}
		report.Metadata = map[string]string{"mode": string(ModeAudit)}
		report.FinishedAt = time.Now().UTC()
		report.Duration = report.FinishedAt.Sub(report.StartedAt)
		report = report.Normalized()
		return report, ErrHarnessAuditUnavailable
	}

	report := r.Registry.Validate(ctx, artifact).Normalized()
	if report.Metadata == nil {
		report.Metadata = map[string]string{}
	}
	report.Metadata["mode"] = string(ModeAudit)
	report = report.Normalized()

	if report.Status == StatusFail || report.Status == StatusInvalid {
		return report, ErrHarnessAuditFailed
	}
	return report, nil
}
