package provider

import "context"

// SandboxProvider defines the interface for sandbox execution environments.
type SandboxProvider interface {
	// RunCommand executes a command with the given arguments, returning output and exit code.
	RunCommand(ctx context.Context, cmd string, args []string) (output string, exitCode int, err error)

	// ReadFile reads a file from the sandbox environment.
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// WriteFile writes content to a file in the sandbox environment.
	WriteFile(ctx context.Context, path string, content []byte) error

	// Name returns the name of the sandbox provider.
	Name() string

	// Close releases any resources held by the provider.
	Close() error
}

// GuardrailsProvider defines the interface for authorization and policy enforcement.
type GuardrailsProvider interface {
	// Authorize checks if a tool invocation is allowed.
	// Returns nil if authorized, or an error if denied.
	Authorize(ctx context.Context, toolName string, params map[string]interface{}) error

	// Name returns the name of the guardrails provider.
	Name() string
}
