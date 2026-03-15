package tools

import (
	"fmt"
	"strings"
)

// ErrorCode identifies the category of a tool failure.
type ErrorCode string

const (
	ErrCodePermission     ErrorCode = "PERMISSION_DENIED"
	ErrCodeNotFound       ErrorCode = "NOT_FOUND"
	ErrCodeTimeout        ErrorCode = "TIMEOUT"
	ErrCodeNetworkFail    ErrorCode = "NETWORK_ERROR"
	ErrCodeInvalidParam   ErrorCode = "INVALID_PARAMETER"
	ErrCodeRateLimit      ErrorCode = "RATE_LIMITED"
	ErrCodeAuthFail       ErrorCode = "AUTH_FAILED"
	ErrCodeOutputTooLarge ErrorCode = "OUTPUT_TOO_LARGE"
	ErrCodeCommandFail    ErrorCode = "COMMAND_FAILED"
	ErrCodeUnknown        ErrorCode = "UNKNOWN"
)

// RecoveryAction describes a concrete next step the AI should take.
type RecoveryAction string

const (
	ActionRetry         RecoveryAction = "retry"
	ActionRetryWithSudo RecoveryAction = "retry_with_sudo"
	ActionCheckPath     RecoveryAction = "check_path"
	ActionCheckNetwork  RecoveryAction = "check_network"
	ActionReduceScope   RecoveryAction = "reduce_scope"
	ActionWaitRetry     RecoveryAction = "wait_and_retry"
	ActionFixParam      RecoveryAction = "fix_parameter"
	ActionCheckAuth     RecoveryAction = "check_auth"
	ActionNone          RecoveryAction = "none"
)

// ToolError is a structured error with recovery guidance returned in tool results.
type ToolError struct {
	Code        ErrorCode      `json:"code"`
	Message     string         `json:"message"`
	Recoverable bool           `json:"recoverable"`
	Action      RecoveryAction `json:"suggested_action"`
	Hint        string         `json:"hint"`
}

// Error implements the error interface.
func (e *ToolError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// ToResultText formats the error as text suitable for inclusion in a ToolResult.
func (e *ToolError) ToResultText() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Error [%s]: %s\n", e.Code, e.Message))
	if e.Recoverable {
		sb.WriteString(fmt.Sprintf("Recovery hint: %s\n", e.Hint))
		sb.WriteString(fmt.Sprintf("Suggested action: %s\n", e.Action))
	} else {
		sb.WriteString("This error is not recoverable. Try a different approach.\n")
	}
	return sb.String()
}

// ClassifyError inspects a raw error message and returns a structured ToolError
// with appropriate recovery guidance.
func ClassifyError(toolName string, rawErr string) *ToolError {
	lower := strings.ToLower(rawErr)

	switch {
	case containsAny(lower, "permission denied", "access denied", "operation not permitted", "eperm", "eacces"):
		return &ToolError{
			Code:        ErrCodePermission,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionRetryWithSudo,
			Hint:        fmt.Sprintf("The %s tool lacks permission. Try running as a privileged user, or check file/directory permissions with list_directory.", toolName),
		}

	case containsAny(lower, "no such file", "not found", "does not exist", "enoent"):
		return &ToolError{
			Code:        ErrCodeNotFound,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionCheckPath,
			Hint:        "The file or path does not exist. Use list_directory to verify the path before retrying.",
		}

	case containsAny(lower, "timeout", "deadline exceeded", "context deadline", "timed out"):
		return &ToolError{
			Code:        ErrCodeTimeout,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionRetry,
			Hint:        "The operation timed out. Retry with a shorter command, or split the task into smaller steps.",
		}

	case containsAny(lower, "connection refused", "no route to host", "network unreachable", "dial tcp", "connection reset"):
		return &ToolError{
			Code:        ErrCodeNetworkFail,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionCheckNetwork,
			Hint:        "Network connectivity failed. Verify the target host is reachable before retrying.",
		}

	case containsAny(lower, "rate limit", "too many requests", "429", "quota exceeded"):
		return &ToolError{
			Code:        ErrCodeRateLimit,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionWaitRetry,
			Hint:        "Rate limited by the remote service. Wait 10-30 seconds before retrying.",
		}

	case containsAny(lower, "unauthorized", "forbidden", "401", "403", "invalid token", "authentication"):
		return &ToolError{
			Code:        ErrCodeAuthFail,
			Message:     rawErr,
			Recoverable: false,
			Action:      ActionCheckAuth,
			Hint:        "Authentication failed. Verify API keys or credentials with /auth status.",
		}

	case containsAny(lower, "output too large", "truncated", "too much output", "buffer overflow"):
		return &ToolError{
			Code:        ErrCodeOutputTooLarge,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionReduceScope,
			Hint:        "Command produced too much output. Use grep_content or pipe through head/tail to filter results.",
		}

	case containsAny(lower, "invalid", "bad parameter", "missing required", "malformed"):
		return &ToolError{
			Code:        ErrCodeInvalidParam,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionFixParam,
			Hint:        fmt.Sprintf("Invalid parameter for %s. Check the tool's parameter requirements with list_tools or tool_info.", toolName),
		}

	case containsAny(lower, "exit status", "command failed", "non-zero exit"):
		return &ToolError{
			Code:        ErrCodeCommandFail,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionRetry,
			Hint:        "The shell command returned a non-zero exit code. Check the error output and adjust the command.",
		}

	default:
		return &ToolError{
			Code:        ErrCodeUnknown,
			Message:     rawErr,
			Recoverable: true,
			Action:      ActionRetry,
			Hint:        fmt.Sprintf("Unexpected error in %s. Review the error message and try a different approach.", toolName),
		}
	}
}

// EnrichResult takes an existing failed ToolResult and enriches its Error field
// with structured recovery guidance. Returns the enriched result.
func EnrichResult(result *ToolResult, toolName string) *ToolResult {
	if result == nil || result.Success {
		return result
	}
	if result.Error == "" {
		return result
	}
	te := ClassifyError(toolName, result.Error)
	result.Error = te.ToResultText()
	return result
}

func containsAny(s string, patterns ...string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
