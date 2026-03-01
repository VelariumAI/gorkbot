package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ParseToolRequests extracts tool requests from AI response
func ParseToolRequests(response string) []ToolRequest {
	var requests []ToolRequest

	// Strategy 1: Look for explicit markdown JSON blocks
	reMarkdown := regexp.MustCompile("```(?:json)?\\s*\\n?([\\s\\S]*?)\\n?```")
	matches := reMarkdown.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		jsonStr := match[1]
		// Clean up potential whitespace or prefixes in the block
		jsonStr = strings.TrimSpace(jsonStr)

		reqs := tryParseJSON(jsonStr)
		requests = append(requests, reqs...)
	}

	// Strategy 2: If no requests found, look for raw JSON objects in the text
	if len(requests) == 0 {
		// Look for strings starting with { and containing "tool"
		// This is a heuristic to find JSON when the AI forgets markdown blocks
		candidates := extractJSONCandidates(response)
		for _, jsonStr := range candidates {
			reqs := tryParseJSON(jsonStr)
			requests = append(requests, reqs...)
		}
	}

	return requests
}

// extractJSONCandidates finds substrings that look like JSON objects containing a "tool" field
func extractJSONCandidates(text string) []string {
	var candidates []string

	// Simple scanner that looks for '{' and counts braces to find the matching '}'
	// only if the content mentions "tool"

	start := -1
	braceCount := 0

	for i, r := range text {
		if r == '{' {
			if start == -1 {
				start = i
			}
			braceCount++
		} else if r == '}' {
			if start != -1 {
				braceCount--
				if braceCount == 0 {
					// Found a complete object
					candidate := text[start : i+1]
					if strings.Contains(candidate, "\"tool\"") {
						candidates = append(candidates, candidate)
					}
					start = -1
				}
			}
		}
	}

	return candidates
}

// tryParseJSON attempts to unmarshal a string into one or more ToolRequests
func tryParseJSON(jsonStr string) []ToolRequest {
	var requests []ToolRequest

	// Attempt 1: Single object
	var singleReq map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &singleReq); err == nil {
		if req := mapToToolRequest(singleReq); req != nil {
			requests = append(requests, *req)
			return requests
		}
	}

	// Attempt 2: List of objects
	var multiReq []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &multiReq); err == nil {
		for _, r := range multiReq {
			if req := mapToToolRequest(r); req != nil {
				requests = append(requests, *req)
			}
		}
	}

	return requests
}

// StripToolBlocks removes tool-call JSON blocks from an AI response, leaving only
// reasoning prose. Used so users see clean text instead of raw JSON tool calls.
// Always pass the original (unstripped) response to ParseToolRequests.
func StripToolBlocks(response string) string {
	// Step 1: remove markdown code blocks (```json … ``` and ``` … ```)
	reMarkdown := regexp.MustCompile("```(?:json)?[\\s\\S]*?```")
	result := reMarkdown.ReplaceAllString(response, "")

	// Step 2: remove bare JSON objects that contain a "tool" field.
	// Uses the same brace-counting approach as extractJSONCandidates.
	runes := []rune(result)
	var out strings.Builder
	start := -1
	braceCount := 0
	lastEnd := 0

	for i, r := range runes {
		if r == '{' {
			if start == -1 {
				start = i
			}
			braceCount++
		} else if r == '}' {
			if start != -1 {
				braceCount--
				if braceCount == 0 {
					candidate := string(runes[start : i+1])
					if strings.Contains(candidate, `"tool"`) {
						// Write text before this JSON block; skip the block itself.
						out.WriteString(string(runes[lastEnd:start]))
						lastEnd = i + 1
					}
					start = -1
				}
			}
		}
	}
	// Write any remaining text after the last JSON block.
	out.WriteString(string(runes[lastEnd:]))
	result = out.String()

	// Step 3: collapse 3+ consecutive blank lines into 2.
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

// mapToToolRequest converts a raw map to a ToolRequest if valid
func mapToToolRequest(raw map[string]interface{}) *ToolRequest {
	toolName, ok := raw["tool"].(string)
	if !ok {
		return nil
	}

	params, _ := raw["parameters"].(map[string]interface{})
	if params == nil {
		params = make(map[string]interface{})
	}

	return &ToolRequest{
		ToolName:   toolName,
		Parameters: params,
		RequestID:  fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), len(toolName)),
	}
}
