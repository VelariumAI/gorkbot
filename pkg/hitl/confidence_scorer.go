package hitl

import (
	"fmt"
	"strings"
)

// ConfidenceScorer evaluates how confident the AI is in its tool execution
// decisions, returning a score from 0-100.
type ConfidenceScorer struct {
	toolSuccessRates map[string]ToolSuccessStats
	sessionToolUse   map[string]int // Track tool use in current session
}

// ToolSuccessStats tracks success metrics for a tool
type ToolSuccessStats struct {
	ExecutionCount    int
	SuccessCount      int
	FailureCount      int
	ErrorCount        int
	LastExecutionTime string
	AverageErrorRate  float64
}

// SuccessRate returns the success rate as a percentage (0-100)
func (t ToolSuccessStats) SuccessRate() int {
	if t.ExecutionCount == 0 {
		return 0
	}
	rate := (float64(t.SuccessCount) / float64(t.ExecutionCount)) * 100
	return int(rate)
}

// NewConfidenceScorer creates a new confidence scorer
func NewConfidenceScorer() *ConfidenceScorer {
	return &ConfidenceScorer{
		toolSuccessRates: make(map[string]ToolSuccessStats),
		sessionToolUse:   make(map[string]int),
	}
}

// ScoreAIConfidence evaluates the AI's confidence in a tool execution decision.
// Factors include:
//   - Tool registry validation (known vs unknown tools)
//   - Parameter completeness and validation
//   - AI reasoning quality (certainty language, safety mentions)
//   - Historical success rates
//   - Session context (familiar tools, repeated patterns)
//   - Parameter validation against schema
func (cs *ConfidenceScorer) ScoreAIConfidence(
	toolName string,
	params map[string]interface{},
	requiredParams []string,
	aiReasoning string,
	toolExists bool,
) int {
	score := 50 // Base score

	// ========== POSITIVE FACTORS ==========

	// +5: Tool is in registered registry
	if toolExists {
		score += 5
	}

	// +15: All required parameters present and non-empty
	score += cs.scoreParamCompleteness(requiredParams, params)

	// +10: Parameter format and types appear correct
	score += cs.scoreParamValidation(params)

	// +10: AI reasoning mentions safety, verification, or confidence
	score += cs.scoreAIReasoningQuality(aiReasoning)

	// +10: Tool was recently used in session (precedent)
	score += cs.scoreSessionPrecedent(toolName)

	// +10: Tool has high historical success rate (>90%)
	score += cs.scoreHistoricalSuccess(toolName)

	// +15: Explicit approval patterns in reasoning
	score += cs.scoreApprovalPatterns(aiReasoning)

	// ========== NEGATIVE FACTORS ==========

	// -10: Unusual parameter combinations
	score -= cs.scoreUnusualCombinations(toolName, params)

	// -15: Tool has low success rate (<50%)
	score -= cs.scoreHistoricalFailure(toolName)

	// -20: Destructive operation without verification reasoning
	if strings.Contains(strings.ToLower(toolName), "delete") ||
		strings.Contains(strings.ToLower(toolName), "remove") ||
		strings.Contains(strings.ToLower(toolName), "destroy") {
		if !strings.Contains(strings.ToLower(aiReasoning), "verify") &&
			!strings.Contains(strings.ToLower(aiReasoning), "check") {
			score -= 20
		}
	}

	// -10: Parameter contains user secrets or paths
	score -= cs.scoreParameterSensitivity(params)

	// -5: First use of tool in session (no precedent)
	if _, exists := cs.sessionToolUse[toolName]; !exists {
		score -= 5
	}

	// -15: Tool has low success rate or many failures
	if stats, ok := cs.toolSuccessRates[toolName]; ok {
		if stats.SuccessRate() < 50 && stats.ExecutionCount > 3 {
			score -= 15
		}
	}

	// -10: AI uncertain language ("might", "possibly", "maybe", "perhaps")
	score -= cs.scoreUncertainLanguage(aiReasoning)

	// -5: Overly complex parameter structure
	score -= cs.scoreParameterComplexity(params)

	// Cap score at 0-100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// scoreParamCompleteness evaluates if all required parameters are provided
func (cs *ConfidenceScorer) scoreParamCompleteness(required []string, provided map[string]interface{}) int {
	if len(required) == 0 {
		return 10 // No params required, full points
	}

	providedCount := 0
	for _, req := range required {
		if val, ok := provided[req]; ok {
			// Check if value is non-empty
			strVal := fmt.Sprintf("%v", val)
			if strings.TrimSpace(strVal) != "" && strVal != "<nil>" {
				providedCount++
			}
		}
	}

	ratio := float64(providedCount) / float64(len(required))
	if ratio >= 0.95 {
		return 15
	} else if ratio >= 0.8 {
		return 10
	} else if ratio >= 0.5 {
		return 5
	}
	return 0
}

// scoreParamValidation checks if parameters appear to have correct types/formats
func (cs *ConfidenceScorer) scoreParamValidation(params map[string]interface{}) int {
	validatedCount := 0
	totalCount := 0

	for key, val := range params {
		totalCount++
		strVal := fmt.Sprintf("%v", val)
		if strVal == "" || strVal == "<nil>" {
			continue
		}

		switch strings.ToLower(key) {
		case "path", "file", "directory", "dir":
			// Check if looks like a path
			if strings.Contains(strVal, "/") || strings.Contains(strVal, "\\") {
				validatedCount++
			}
		case "url", "uri", "link", "address":
			// Check if looks like a URL
			if strings.HasPrefix(strVal, "http://") || strings.HasPrefix(strVal, "https://") {
				validatedCount++
			}
		case "port":
			// Check if numeric and in valid range
			var port int
			if _, err := fmt.Sscanf(strVal, "%d", &port); err == nil && port > 0 && port < 65536 {
				validatedCount++
			}
		case "name", "id", "identifier":
			// Check if non-empty string
			if len(strings.TrimSpace(strVal)) > 0 {
				validatedCount++
			}
		case "command", "cmd", "script":
			// Check for command-like syntax
			if strings.Contains(strVal, " ") || strings.Contains(strVal, "-") {
				validatedCount++
			}
		default:
			// Generic: non-empty is valid
			if len(strings.TrimSpace(strVal)) > 0 {
				validatedCount++
			}
		}
	}

	if totalCount == 0 {
		return 10
	}

	ratio := float64(validatedCount) / float64(totalCount)
	if ratio >= 0.9 {
		return 10
	} else if ratio >= 0.7 {
		return 5
	}
	return 0
}

// scoreAIReasoningQuality evaluates the quality of AI's stated reasoning
func (cs *ConfidenceScorer) scoreAIReasoningQuality(reasoning string) int {
	score := 0
	lowerReasoning := strings.ToLower(reasoning)

	// Look for safety-conscious language
	safetyKeywords := []string{
		"safe", "safety", "verify", "verify", "check", "validated",
		"confirmed", "cross-check", "ensure", "guarantee",
		"correctly", "properly", "appropriately",
	}

	for _, kw := range safetyKeywords {
		if strings.Contains(lowerReasoning, kw) {
			score += 2
			break // Only count once
		}
	}

	// Check for specific reasoning patterns
	if strings.Contains(lowerReasoning, "because") || strings.Contains(lowerReasoning, "since") {
		score += 3 // Shows causal reasoning
	}

	if strings.Contains(lowerReasoning, "user") || strings.Contains(lowerReasoning, "request") {
		score += 2 // References user context
	}

	if strings.Contains(lowerReasoning, "error") && strings.Contains(lowerReasoning, "handle") {
		score += 2 // Shows error handling awareness
	}

	if score > 10 {
		score = 10
	}
	return score
}

// scoreSessionPrecedent adds points for tools used earlier in session
func (cs *ConfidenceScorer) scoreSessionPrecedent(toolName string) int {
	if count, ok := cs.sessionToolUse[toolName]; ok && count > 1 {
		return 10 // Tool used before in this session
	}
	return 0
}

// scoreHistoricalSuccess adds points for tools with high success rates
func (cs *ConfidenceScorer) scoreHistoricalSuccess(toolName string) int {
	if stats, ok := cs.toolSuccessRates[toolName]; ok {
		successRate := stats.SuccessRate()
		if successRate >= 90 {
			return 10
		} else if successRate >= 75 {
			return 5
		}
	}
	return 0
}

// scoreHistoricalFailure subtracts points for tools with poor track records
func (cs *ConfidenceScorer) scoreHistoricalFailure(toolName string) int {
	if stats, ok := cs.toolSuccessRates[toolName]; ok {
		successRate := stats.SuccessRate()
		if successRate < 50 && stats.ExecutionCount > 3 {
			return 15
		} else if successRate < 70 && stats.ExecutionCount > 2 {
			return 8
		}
	}
	return 0
}

// scoreApprovalPatterns looks for explicit approval indicators in reasoning
func (cs *ConfidenceScorer) scoreApprovalPatterns(reasoning string) int {
	lowerReasoning := strings.ToLower(reasoning)

	approvalPatterns := []string{
		"user requested", "you asked", "you wanted", "per your request",
		"this is straightforward", "this is clear", "this is simple",
		"safe to proceed", "ready to execute", "ready to proceed",
	}

	for _, pattern := range approvalPatterns {
		if strings.Contains(lowerReasoning, pattern) {
			return 15
		}
	}

	return 0
}

// scoreUnusualCombinations detects unusual parameter combinations
func (cs *ConfidenceScorer) scoreUnusualCombinations(toolName string, params map[string]interface{}) int {
	// Some tools have expected parameter combinations
	// Deviations reduce confidence

	lowerToolName := strings.ToLower(toolName)

	// For example: read operations shouldn't have write flags
	if strings.Contains(lowerToolName, "read") {
		for key := range params {
			if strings.Contains(strings.ToLower(key), "write") ||
				strings.Contains(strings.ToLower(key), "append") {
				return 10
			}
		}
	}

	// Bash with conflicting parameters
	if strings.Contains(lowerToolName, "bash") || strings.Contains(lowerToolName, "shell") {
		_, hasCommand := params["command"]
		_, hasScript := params["script"]
		_, hasFile := params["file"]

		conflictCount := 0
		if hasCommand {
			conflictCount++
		}
		if hasScript {
			conflictCount++
		}
		if hasFile {
			conflictCount++
		}

		if conflictCount > 1 {
			return 10 // Multiple execution methods specified
		}
	}

	return 0
}

// scoreParameterSensitivity checks if parameters contain sensitive information
func (cs *ConfidenceScorer) scoreParameterSensitivity(params map[string]interface{}) int {
	sensitivePatterns := []string{
		"password", "passwd", "pwd", "secret", "key", "token",
		"credential", "oauth", "api_key", "private", "ssh",
		".env", ".aws", ".gnupg", ".ssh", "/root", "/etc",
	}

	for _, val := range params {
		strVal := strings.ToLower(fmt.Sprintf("%v", val))
		for _, pattern := range sensitivePatterns {
			if strings.Contains(strVal, pattern) {
				return 10
			}
		}
	}

	return 0
}

// scoreUncertainLanguage detects uncertain phrasing in AI reasoning
func (cs *ConfidenceScorer) scoreUncertainLanguage(reasoning string) int {
	lowerReasoning := strings.ToLower(reasoning)

	uncertainPatterns := []string{
		"might", "possibly", "maybe", "perhaps", "could be",
		"seemingly", "apparently", "arguably", "possibly",
		"uncertain", "unsure", "not sure", "probably",
		"tentatively", "allegedly",
	}

	count := 0
	for _, pattern := range uncertainPatterns {
		if strings.Contains(lowerReasoning, pattern) {
			count++
		}
	}

	// Penalize based on how many uncertain phrases appear
	if count >= 3 {
		return 10
	} else if count == 2 {
		return 7
	} else if count == 1 {
		return 3
	}

	return 0
}

// scoreParameterComplexity evaluates how complex the parameter structure is
func (cs *ConfidenceScorer) scoreParameterComplexity(params map[string]interface{}) int {
	// High complexity = more potential for error
	complexityScore := 0

	for _, val := range params {
		// Nested parameters add complexity
		switch v := val.(type) {
		case map[string]interface{}:
			complexityScore += 2 // Nested object
		case []interface{}:
			complexityScore += 1 // Array parameter
		default:
			// Evaluate string length
			strVal := fmt.Sprintf("%v", v)
			if len(strVal) > 500 {
				complexityScore += 2
			} else if len(strVal) > 200 {
				complexityScore += 1
			}
		}
	}

	// High complexity reduces confidence
	if complexityScore > 5 {
		return 5
	} else if complexityScore > 3 {
		return 3
	}

	return 0
}

// RecordToolExecution updates historical success stats for a tool
func (cs *ConfidenceScorer) RecordToolExecution(toolName string, success bool, errorMessage string) {
	stats, exists := cs.toolSuccessRates[toolName]
	if !exists {
		stats = ToolSuccessStats{}
	}

	stats.ExecutionCount++
	if success {
		stats.SuccessCount++
	} else {
		stats.FailureCount++
		if errorMessage != "" {
			stats.ErrorCount++
		}
	}

	cs.toolSuccessRates[toolName] = stats
}

// RecordToolUseInSession tracks tool use within current session
func (cs *ConfidenceScorer) RecordToolUseInSession(toolName string) {
	cs.sessionToolUse[toolName]++
}

// GetToolStats returns execution statistics for a tool
func (cs *ConfidenceScorer) GetToolStats(toolName string) (ToolSuccessStats, bool) {
	stats, ok := cs.toolSuccessRates[toolName]
	return stats, ok
}

// ConfidenceBand represents a confidence range and its properties
type ConfidenceBand int

const (
	BandVeryHigh ConfidenceBand = iota
	BandHigh
	BandMedium
	BandLow
	BandVeryLow
)

// GetConfidenceBand converts a numeric score to a confidence band
func GetConfidenceBand(score int) ConfidenceBand {
	switch {
	case score >= 90:
		return BandVeryHigh
	case score >= 70:
		return BandHigh
	case score >= 50:
		return BandMedium
	case score >= 30:
		return BandLow
	default:
		return BandVeryLow
	}
}

// BandName returns a human-readable name for a confidence band
func (b ConfidenceBand) Name() string {
	switch b {
	case BandVeryHigh:
		return "Very High"
	case BandHigh:
		return "High"
	case BandMedium:
		return "Medium"
	case BandLow:
		return "Low"
	case BandVeryLow:
		return "Very Low"
	default:
		return "Unknown"
	}
}

// BandColor returns an ANSI color code for the confidence band
func (b ConfidenceBand) Color() string {
	switch b {
	case BandVeryHigh:
		return "\033[32m" // Green
	case BandHigh:
		return "\033[36m" // Cyan
	case BandMedium:
		return "\033[33m" // Yellow
	case BandLow:
		return "\033[31m" // Red
	case BandVeryLow:
		return "\033[35m" // Magenta
	default:
		return "\033[0m" // Reset
	}
}

// ConfidenceBarLength renders a confidence bar at the given length
func (b ConfidenceBand) ConfidenceBar(score int, length int) string {
	filled := (score * length) / 100
	if filled > length {
		filled = length
	}

	bar := ""
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := filled; i < length; i++ {
		bar += "░"
	}

	return bar
}
