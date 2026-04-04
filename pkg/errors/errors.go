package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// Severity levels for structured errors.
type Severity string

const (
	SeverityDebug       Severity = "debug"
	SeverityInfo        Severity = "info"
	SeverityWarning     Severity = "warning"
	SeverityError       Severity = "error"
	SeverityCritical    Severity = "critical"
)

// Domain categorizes which subsystem produced the error.
type Domain string

const (
	DomainProvider      Domain = "provider"
	DomainTool          Domain = "tool"
	DomainMemory        Domain = "memory"
	DomainSelfImprove   Domain = "selfimprove"
	DomainAuth          Domain = "auth"
	DomainIO            Domain = "io"
	DomainNetwork       Domain = "network"
	DomainValidation    Domain = "validation"
	DomainPermission    Domain = "permission"
	DomainUnknown       Domain = "unknown"
)

// Error is a structured error carrying context, domain, correlation ID, and stack trace.
// It implements the standard error interface and can be wrapped with additional context.
type Error struct {
	severity      Severity
	domain        Domain
	correlationID string
	message       string
	cause         error
	stack         []string
}

// New creates a new structured error at the Error severity level.
func New(domain Domain, message string) *Error {
	return &Error{
		severity:  SeverityError,
		domain:    domain,
		message:   message,
		stack:     captureStack(),
	}
}

// Newf creates a new structured error with formatted message.
func Newf(domain Domain, format string, args ...interface{}) *Error {
	return &Error{
		severity:  SeverityError,
		domain:    domain,
		message:   fmt.Sprintf(format, args...),
		stack:     captureStack(),
	}
}

// WithSeverity sets the severity level.
func (e *Error) WithSeverity(sev Severity) *Error {
	if e != nil {
		e.severity = sev
	}
	return e
}

// WithCorrelationID sets the correlation ID for tracing.
func (e *Error) WithCorrelationID(id string) *Error {
	if e != nil {
		e.correlationID = id
	}
	return e
}

// Wrap wraps an existing error with additional context.
func (e *Error) Wrap(cause error) *Error {
	if e != nil {
		e.cause = cause
	}
	return e
}

// Severity returns the error's severity level.
func (e *Error) Severity() Severity {
	if e == nil {
		return SeverityInfo
	}
	return e.severity
}

// Domain returns the subsystem that produced the error.
func (e *Error) Domain() Domain {
	if e == nil {
		return DomainUnknown
	}
	return e.domain
}

// CorrelationID returns the error's correlation ID (may be empty).
func (e *Error) CorrelationID() string {
	if e == nil {
		return ""
	}
	return e.correlationID
}

// Message returns the error message.
func (e *Error) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

// Cause returns the wrapped error (may be nil).
func (e *Error) Cause() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Stack returns the captured call stack.
func (e *Error) Stack() []string {
	if e == nil {
		return nil
	}
	return e.stack
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[")
	sb.WriteString(string(e.severity))
	sb.WriteString("] ")
	sb.WriteString(string(e.domain))
	sb.WriteString(": ")
	sb.WriteString(e.message)

	if e.correlationID != "" {
		sb.WriteString(" (correlation: ")
		sb.WriteString(e.correlationID)
		sb.WriteString(")")
	}

	if e.cause != nil {
		sb.WriteString(" → ")
		sb.WriteString(e.cause.Error())
	}

	return sb.String()
}

// String returns a detailed representation including stack trace.
func (e *Error) String() string {
	if e == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(e.Error())

	if len(e.stack) > 0 {
		sb.WriteString("\nStack trace:\n")
		for _, frame := range e.stack {
			sb.WriteString("  ")
			sb.WriteString(frame)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// captureStack captures the current call stack (skip first 3 frames).
func captureStack() []string {
	var frames []string
	pcs := make([]uintptr, 32)
	n := runtime.Callers(3, pcs) // Skip: captureStack → New/Newf → caller

	for i := 0; i < n; i++ {
		pc := pcs[i]
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		file, line := fn.FileLine(pc)
		frames = append(frames, fmt.Sprintf("%s:%d in %s", file, line, fn.Name()))
	}

	return frames
}

// ─── Convenience Constructors ──────────────────────────────────────────────

// ProviderError creates an error from a provider.
func ProviderError(providerID, message string) *Error {
	return Newf(DomainProvider, "[%s] %s", providerID, message)
}

// ToolError creates an error from tool execution.
func ToolError(toolName, message string) *Error {
	return Newf(DomainTool, "[%s] %s", toolName, message)
}

// MemoryError creates an error from the memory system.
func MemoryError(operation, message string) *Error {
	return Newf(DomainMemory, "[%s] %s", operation, message)
}

// ValidationError creates a validation error.
func ValidationError(field, reason string) *Error {
	return Newf(DomainValidation, "field %q: %s", field, reason)
}

// PermissionError creates a permission denied error.
func PermissionError(resource string) *Error {
	return Newf(DomainPermission, "permission denied: %s", resource)
}

// NetworkError creates a network error.
func NetworkError(operation string, cause error) *Error {
	return Newf(DomainNetwork, "%s", operation).Wrap(cause)
}
