package engine

import (
	"strings"

	"github.com/velariumai/gorkbot/pkg/sense"
)

// MessageSuppressionMiddleware applies output filtering to AI responses.
// It suppresses internal system messages based on verbosity settings while
// preserving user-facing content and actual tool results.
type MessageSuppressionMiddleware struct {
	filter  *sense.OutputFilter
	verbose bool
	logger  Logger
}

// Logger is a minimal logging interface for the middleware.
type Logger interface {
	Debug(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// NewMessageSuppressionMiddleware creates a new suppression middleware.
func NewMessageSuppressionMiddleware(verbose bool, logger Logger) *MessageSuppressionMiddleware {
	var config sense.OutputFilterConfig
	if verbose {
		config = sense.CreateVerboseConfig()
	} else {
		config = sense.CreateDefaultConfig()
	}

	filter, err := sense.NewOutputFilter(config)
	if err != nil {
		// Fall back to empty filter if creation fails
		if logger != nil {
			logger.Error("Failed to create output filter", "error", err)
		}
		filter, _ = sense.NewOutputFilter(sense.OutputFilterConfig{})
	}

	return &MessageSuppressionMiddleware{
		filter:  filter,
		verbose: verbose,
		logger:  logger,
	}
}

// ProcessResponse filters an AI response according to suppression rules.
// Returns the filtered content (with system messages removed if not verbose).
// If verbose mode is enabled, returns the original content unfiltered.
func (m *MessageSuppressionMiddleware) ProcessResponse(content string) string {
	if m.verbose {
		// Verbose mode: pass through everything unchanged
		return content
	}

	// Silent mode: apply output filter
	filtered := m.filter.Filter(content)
	return filtered
}

// ProcessStreamingToken filters individual tokens from a streaming response.
// This is called for each token as it arrives from the AI provider.
// Returns the token (possibly empty if it should be suppressed).
func (m *MessageSuppressionMiddleware) ProcessStreamingToken(token string) string {
	if m.verbose {
		return token
	}

	// For streaming, we apply heuristic filtering based on known patterns.
	// This is less perfect than full content filtering but works in real-time.
	token = m.suppressToolNarrationTokens(token)
	token = m.suppressSystemStatusTokens(token)

	return token
}

// suppressToolNarrationTokens removes tokens that are part of tool narration.
// Examples: "I'm executing the", "Tool has completed", "invoking tool", "calling tool"
func (m *MessageSuppressionMiddleware) suppressToolNarrationTokens(token string) string {
	narrationPatterns := []string{
		"I'm executing the",
		"Tool has completed",
		"invoking tool",
		"calling tool",
	}

	lowerToken := strings.ToLower(token)
	for _, pattern := range narrationPatterns {
		if strings.Contains(lowerToken, strings.ToLower(pattern)) {
			// If this token contains a narration pattern, suppress it
			return ""
		}
	}
	return token
}

// suppressSystemStatusTokens removes tokens related to system status section headers only.
func (m *MessageSuppressionMiddleware) suppressSystemStatusTokens(token string) string {
	statusPatterns := []string{
		"=== system status ===",
		"--- status update ---",
		"=== status update ===",
		"--- system status ---",
	}

	lowerToken := strings.ToLower(token)
	for _, pattern := range statusPatterns {
		if strings.Contains(lowerToken, strings.ToLower(pattern)) {
			return ""
		}
	}
	return token
}

// SetVerboseMode updates the verbosity setting and recreates the filter.
func (m *MessageSuppressionMiddleware) SetVerboseMode(verbose bool) {
	m.verbose = verbose
	var config sense.OutputFilterConfig
	if verbose {
		config = sense.CreateVerboseConfig()
	} else {
		config = sense.CreateDefaultConfig()
	}

	filter, err := sense.NewOutputFilter(config)
	if err != nil {
		if m.logger != nil {
			m.logger.Error("Failed to recreate output filter", "error", err)
		}
		return // Keep existing filter
	}
	m.filter = filter
}

// IsVerbose returns whether verbose mode is currently enabled.
func (m *MessageSuppressionMiddleware) IsVerbose() bool {
	return m.verbose
}
