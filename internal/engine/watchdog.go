package engine

import (
	"strings"
	"sync"
)

type WatchdogSeverity int

const (
	SeverityNone WatchdogSeverity = iota
	SeverityInfo
	SeverityWarning
	SeverityCritical
)

type InterventionResponse int

const (
	InterventionContinue     InterventionResponse = iota // One-time allow
	InterventionAllowSession                             // Whitelist repetitive behavior for session
	InterventionStop                                     // Kill the stream
)

// StreamMonitor watches the output stream for issues
type StreamMonitor struct {
	buffer      strings.Builder
	windowSize  int
	history     []string
	mu          sync.Mutex
	repetition  int
	lastSegment string

	// Intelligent State
	consecutiveLines int
	isCodeBlock      bool
}

// NewStreamMonitor creates a new monitor
func NewStreamMonitor() *StreamMonitor {
	return &StreamMonitor{
		windowSize: 50,
		history:    make([]string, 0),
	}
}

// WriteToken adds a token and checks for issues
// Returns the severity of any detected issue
func (sm *StreamMonitor) WriteToken(token string) WatchdogSeverity {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.buffer.WriteString(token)

	// Track code block state
	if strings.Contains(token, "```") {
		sm.isCodeBlock = !sm.isCodeBlock
	}

	currentLen := sm.buffer.Len()
	if currentLen < 100 {
		return SeverityNone
	}

	content := sm.buffer.String()

	// 1. Pathological Phrase Repetition (High Severity)
	// "I will I will I will" - identical repetition without structure
	if currentLen > 50 {
		tail := content[currentLen-50:]
		prevContent := content[:currentLen-50]

		if strings.HasSuffix(prevContent, tail) {
			sm.repetition++
		} else {
			sm.repetition = 0
		}
	}

	// 2. Structural Analysis (Mitigation)
	// If we are inside a code block or writing distinct lines (like a list), reduce severity
	isStructured := false
	if sm.isCodeBlock {
		isStructured = true // Repetitive code (loops, data arrays) is common
	} else {
		// Check if the repetitive part has varying digits or timestamps?
		// Simple check: does it contain newlines?
		if strings.Count(token, "\n") > 0 {
			sm.consecutiveLines++
		}
		if sm.consecutiveLines > 5 && sm.repetition < 5 {
			// It's repeating but it's listing things.
			isStructured = true
		}
	}

	// 3. Decision Matrix
	if sm.repetition >= 3 {
		if isStructured {
			// It's repeating a lot, but looks structured. Warn user.
			return SeverityWarning
		}
		// It's repeating identical unstructured text. Kill it.
		return SeverityCritical
	}

	if sm.repetition >= 1 && !isStructured {
		// Just started repeating
		return SeverityInfo
	}

	return SeverityNone
}

// GetDiagnostics returns why it triggered
func (sm *StreamMonitor) GetDiagnostics() string {
	if sm.isCodeBlock {
		return "High repetition detected inside code block."
	}
	return "Detected unstructured repetitive output loop."
}
