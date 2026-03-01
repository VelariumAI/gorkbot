package ai

import "strings"

// streamContextKey is a private key type for stream retry context values.
// It avoids collisions with keys from other packages.
type streamContextKey struct{ name string }

// streamRetriesKey is the context key used to track how many times
// StreamWithHistory has been retried for a given request.
var streamRetriesKey = &streamContextKey{"streamRetries"}

// StreamGuard wraps an SSE stream scan loop and detects incomplete streams.
// An "incomplete" stream is one that ends without seeing a terminal event:
//   - OpenAI/xAI: "data: [DONE]"
//   - Anthropic:  event type "message_stop"
//   - Generic:    finish_reason == "stop", "end_turn", or "length"
type StreamGuard struct {
	sawTerminal bool
	partial     strings.Builder
}

// NewStreamGuard returns an initialised StreamGuard ready for use.
func NewStreamGuard() *StreamGuard {
	return &StreamGuard{}
}

// ObserveLine should be called for each raw SSE line received from the server.
// It updates internal terminal-detection state without modifying normal data flow.
func (sg *StreamGuard) ObserveLine(line string) {
	// OpenAI / xAI terminal marker
	if line == "data: [DONE]" {
		sg.sawTerminal = true
		return
	}

	// finish_reason variants (may appear inside JSON chunks)
	if strings.Contains(line, `"finish_reason":"stop"`) ||
		strings.Contains(line, `"finish_reason": "stop"`) {
		sg.sawTerminal = true
		return
	}
	if strings.Contains(line, `"finish_reason":"end_turn"`) ||
		strings.Contains(line, `"finish_reason": "end_turn"`) {
		sg.sawTerminal = true
		return
	}
	// "length" means the model hit its token cap — still a valid terminal state.
	if strings.Contains(line, `"finish_reason":"length"`) ||
		strings.Contains(line, `"finish_reason": "length"`) {
		sg.sawTerminal = true
		return
	}

	// Anthropic message_stop event (comes as a JSON "type" field)
	if strings.Contains(line, `"type":"message_stop"`) ||
		strings.Contains(line, `"type": "message_stop"`) {
		sg.sawTerminal = true
		return
	}

	// Anthropic stop_reason field
	if strings.Contains(line, `"stop_reason":"end_turn"`) ||
		strings.Contains(line, `"stop_reason": "end_turn"`) {
		sg.sawTerminal = true
		return
	}
}

// ObserveContent appends decoded content text to the internal partial buffer.
// Call this whenever content is written to the output writer so that callers
// can inspect what was received if the stream turns out to be incomplete.
func (sg *StreamGuard) ObserveContent(content string) {
	sg.partial.WriteString(content)
}

// WasComplete returns true if the stream received a recognised terminal event
// before the scanner loop ended. A false result indicates a dropout.
func (sg *StreamGuard) WasComplete() bool {
	return sg.sawTerminal
}

// PartialContent returns whatever content was accumulated via ObserveContent.
// Useful for logging or displaying a truncated response on final failure.
func (sg *StreamGuard) PartialContent() string {
	return sg.partial.String()
}
