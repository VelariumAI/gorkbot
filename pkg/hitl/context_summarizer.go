package hitl

import (
	"fmt"
	"regexp"
	"strings"
)

// ContextSummarizer generates concise explanations of why a tool execution
// is needed in the context of the user's conversation.
type ContextSummarizer struct {
	conversationTurns []ConversationTurn
}

// ConversationTurn represents a single exchange in the conversation
type ConversationTurn struct {
	UserMessage    string
	AIResponse     string
	ToolsRequested []string
	Timestamp      string
	TurnNumber     int
}

// NewContextSummarizer creates a new context summarizer
func NewContextSummarizer() *ContextSummarizer {
	return &ContextSummarizer{
		conversationTurns: []ConversationTurn{},
	}
}

// AddConversationTurn records a new turn in the conversation
func (cs *ContextSummarizer) AddConversationTurn(turn ConversationTurn) {
	turn.TurnNumber = len(cs.conversationTurns) + 1
	cs.conversationTurns = append(cs.conversationTurns, turn)
}

// SummarizeContext generates a concise context explanation for a tool execution.
// It examines recent conversation history to understand what the user asked for
// and how this tool execution fulfills that request.
func (cs *ContextSummarizer) SummarizeContext(
	toolName string,
	params map[string]interface{},
	aiExplanation string,
) string {
	// Strategy 1: Extract from AI explanation
	if summary := cs.extractFromExplanation(toolName, aiExplanation); summary != "" {
		return summary
	}

	// Strategy 2: Infer from recent user request
	if summary := cs.inferFromUserRequest(toolName); summary != "" {
		return summary
	}

	// Strategy 3: Describe what the tool does
	return cs.describeToolPurpose(toolName)
}

// extractFromExplanation tries to extract context directly from AI's explanation
func (cs *ContextSummarizer) extractFromExplanation(toolName, explanation string) string {
	if explanation == "" {
		return ""
	}

	// Look for explicit "to..." patterns
	patterns := []string{
		`(?i)to\s+([^.!?\n]{10,100})`,
		`(?i)in order to\s+([^.!?\n]{10,100})`,
		`(?i)for\s+([^.!?\n]{10,100})`,
		`(?i)so that\s+([^.!?\n]{10,100})`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(explanation); len(matches) > 1 {
			context := strings.TrimSpace(matches[1])
			// Clean up the extraction
			context = cleanContextString(context)
			if len(context) > 10 && len(context) < 200 {
				return fmt.Sprintf("The AI is running %s to %s.", toolName, context)
			}
		}
	}

	return ""
}

// inferFromUserRequest looks at the most recent user message to understand context
func (cs *ContextSummarizer) inferFromUserRequest(toolName string) string {
	if len(cs.conversationTurns) == 0 {
		return ""
	}

	// Get the most recent user request
	lastTurn := cs.conversationTurns[len(cs.conversationTurns)-1]
	userRequest := lastTurn.UserMessage

	if userRequest == "" {
		return ""
	}

	// Truncate if too long
	if len(userRequest) > 150 {
		userRequest = userRequest[:150] + "..."
	}

	// Infer intent from request
	intent := cs.inferIntent(userRequest, toolName)

	if intent != "" {
		return fmt.Sprintf("The AI is executing %s to fulfill your request: \"%s\" (%s)", toolName, userRequest, intent)
	}

	return fmt.Sprintf("The AI is running %s as part of your request: \"%s\"", toolName, userRequest)
}

// inferIntent analyzes a user request and tool name to infer what the user wants
func (cs *ContextSummarizer) inferIntent(userRequest, toolName string) string {
	lowerRequest := strings.ToLower(userRequest)
	lowerToolName := strings.ToLower(toolName)

	// Common intent patterns
	if strings.Contains(lowerRequest, "what") {
		return "answering your question"
	}
	if strings.Contains(lowerRequest, "show") || strings.Contains(lowerRequest, "display") {
		return "showing the information you requested"
	}
	if strings.Contains(lowerRequest, "tell") {
		return "providing the information you asked for"
	}
	if strings.Contains(lowerRequest, "help") || strings.Contains(lowerRequest, "assist") {
		return "assisting with your request"
	}
	if strings.Contains(lowerRequest, "create") || strings.Contains(lowerRequest, "make") {
		return "creating what you asked for"
	}
	if strings.Contains(lowerRequest, "delete") || strings.Contains(lowerRequest, "remove") {
		return "removing what you requested"
	}
	if strings.Contains(lowerRequest, "modify") || strings.Contains(lowerRequest, "change") {
		return "making the changes you requested"
	}
	if strings.Contains(lowerRequest, "check") || strings.Contains(lowerRequest, "verify") {
		return "verifying what you requested"
	}
	if strings.Contains(lowerRequest, "fix") {
		return "fixing the issue you mentioned"
	}
	if strings.Contains(lowerRequest, "how") {
		return "helping you understand how to do something"
	}

	// Tool-specific intent inference
	if strings.Contains(lowerToolName, "read") {
		return "retrieving the content you asked about"
	}
	if strings.Contains(lowerToolName, "write") || strings.Contains(lowerToolName, "create") {
		return "creating what you requested"
	}
	if strings.Contains(lowerToolName, "delete") || strings.Contains(lowerToolName, "remove") {
		return "removing something as requested"
	}
	if strings.Contains(lowerToolName, "list") || strings.Contains(lowerToolName, "search") {
		return "finding what you asked about"
	}
	if strings.Contains(lowerToolName, "git") {
		return "managing your git repository as requested"
	}
	if strings.Contains(lowerToolName, "bash") || strings.Contains(lowerToolName, "execute") {
		return "executing the command you requested"
	}

	return ""
}

// describeToolPurpose generates a generic description of what a tool does
func (cs *ContextSummarizer) describeToolPurpose(toolName string) string {
	lowerName := strings.ToLower(toolName)

	// Tool purpose mappings
	purposes := map[string]string{
		"read_file":      "to retrieve file contents",
		"write_file":     "to create or update a file",
		"delete_file":    "to remove a file",
		"list_directory": "to show directory contents",
		"search_files":   "to search for files matching a pattern",
		"grep_content":   "to search within files",
		"git_status":     "to check the status of the git repository",
		"git_diff":       "to show changes in the repository",
		"git_commit":     "to commit changes to git",
		"git_push":       "to push changes to the remote repository",
		"git_pull":       "to pull changes from the remote repository",
		"bash":           "to execute a shell command",
		"execute":        "to run a system command",
		"web_fetch":      "to retrieve content from a web URL",
		"http_request":   "to make an HTTP request",
		"system_info":    "to gather system information",
		"list_processes": "to show running processes",
		"spawn_agent":    "to initiate a specialized agent",
		"http_get":       "to fetch data from an API endpoint",
		"http_post":      "to send data to an API endpoint",
		"create_tool":    "to define a new custom tool",
	}

	// Exact match
	if purpose, ok := purposes[lowerName]; ok {
		return fmt.Sprintf("The AI is running %s %s.", toolName, purpose)
	}

	// Partial match
	for key, purpose := range purposes {
		if strings.Contains(lowerName, key) || strings.Contains(key, lowerName) {
			return fmt.Sprintf("The AI is running %s %s.", toolName, purpose)
		}
	}

	// Category-based inference
	if strings.Contains(lowerName, "read") || strings.Contains(lowerName, "get") {
		return fmt.Sprintf("The AI is running %s to retrieve information.", toolName)
	}
	if strings.Contains(lowerName, "write") || strings.Contains(lowerName, "create") || strings.Contains(lowerName, "put") {
		return fmt.Sprintf("The AI is running %s to create or modify data.", toolName)
	}
	if strings.Contains(lowerName, "delete") || strings.Contains(lowerName, "remove") {
		return fmt.Sprintf("The AI is running %s to remove data or resources.", toolName)
	}
	if strings.Contains(lowerName, "list") || strings.Contains(lowerName, "search") {
		return fmt.Sprintf("The AI is running %s to find or list items.", toolName)
	}
	if strings.Contains(lowerName, "git") {
		return fmt.Sprintf("The AI is running %s to manage version control.", toolName)
	}
	if strings.Contains(lowerName, "bash") || strings.Contains(lowerName, "shell") || strings.Contains(lowerName, "exec") {
		return fmt.Sprintf("The AI is running %s to execute a system command.", toolName)
	}
	if strings.Contains(lowerName, "http") || strings.Contains(lowerName, "web") || strings.Contains(lowerName, "fetch") {
		return fmt.Sprintf("The AI is running %s to interact with a web service.", toolName)
	}

	// Fallback
	return fmt.Sprintf("The AI is running %s to fulfill your request.", toolName)
}

// cleanContextString removes artifacts and improves readability
func cleanContextString(s string) string {
	// Remove trailing punctuation and artifacts
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	s = strings.TrimSuffix(s, "!")
	s = strings.TrimSuffix(s, "?")
	s = strings.TrimSuffix(s, ",")

	// Remove "the" at the beginning if it makes sense
	if strings.HasPrefix(strings.ToLower(s), "the ") {
		s = s[4:]
	}

	return s
}

// GetRecentHistory returns the last N conversation turns
func (cs *ContextSummarizer) GetRecentHistory(count int) []ConversationTurn {
	if count > len(cs.conversationTurns) {
		count = len(cs.conversationTurns)
	}

	if count == 0 {
		return []ConversationTurn{}
	}

	start := len(cs.conversationTurns) - count
	return cs.conversationTurns[start:]
}

// ClearHistory resets the conversation history
func (cs *ContextSummarizer) ClearHistory() {
	cs.conversationTurns = []ConversationTurn{}
}

// GetContextStats returns statistics about the conversation
func (cs *ContextSummarizer) GetContextStats() map[string]interface{} {
	totalTurns := len(cs.conversationTurns)
	totalTools := 0

	for _, turn := range cs.conversationTurns {
		totalTools += len(turn.ToolsRequested)
	}

	return map[string]interface{}{
		"total_turns":      totalTurns,
		"total_tools_used": totalTools,
		"avg_tools_per_turn": func() float64 {
			if totalTurns == 0 {
				return 0
			}
			return float64(totalTools) / float64(totalTurns)
		}(),
	}
}

// GenerateDetailedContext creates a more comprehensive explanation for complex operations
func (cs *ContextSummarizer) GenerateDetailedContext(
	toolName string,
	params map[string]interface{},
	aiExplanation string,
	stepNumber int,
	totalSteps int,
) string {
	basicContext := cs.SummarizeContext(toolName, params, aiExplanation)

	// Add step information if part of a multi-step process
	if totalSteps > 1 {
		basicContext = fmt.Sprintf("%s (Step %d of %d in the overall process)", basicContext, stepNumber, totalSteps)
	}

	// Add parameter summary for complex operations
	if len(params) > 2 {
		paramSummary := cs.summarizeParameters(toolName, params)
		if paramSummary != "" {
			basicContext = fmt.Sprintf("%s. Parameters: %s", basicContext, paramSummary)
		}
	}

	return basicContext
}

// summarizeParameters creates a human-readable summary of parameters
func (cs *ContextSummarizer) summarizeParameters(toolName string, params map[string]interface{}) string {
	if len(params) == 0 {
		return ""
	}

	var parts []string

	for key, val := range params {
		strVal := fmt.Sprintf("%v", val)
		if strVal == "" || strVal == "<nil>" {
			continue
		}

		// Truncate very long values
		if len(strVal) > 80 {
			strVal = strVal[:80] + "..."
		}

		// Format based on key type
		switch strings.ToLower(key) {
		case "path", "file", "directory":
			// Extract just the filename for readability
			if strings.Contains(strVal, "/") {
				parts := strings.Split(strVal, "/")
				strVal = parts[len(parts)-1]
			}
			parts = append(parts, fmt.Sprintf("%s='%s'", key, strVal))

		case "message", "content", "text":
			// Truncate messages
			if len(strVal) > 60 {
				strVal = strVal[:60] + "..."
			}
			parts = append(parts, fmt.Sprintf("%s: %q", key, strVal))

		case "command", "query", "pattern":
			if len(strVal) > 80 {
				strVal = strVal[:80] + "..."
			}
			parts = append(parts, fmt.Sprintf("%s: %s", key, strVal))

		default:
			// Generic formatting
			if len(strVal) > 50 {
				strVal = strVal[:50] + "..."
			}
			parts = append(parts, fmt.Sprintf("%s=%s", key, strVal))
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "; ")
}
