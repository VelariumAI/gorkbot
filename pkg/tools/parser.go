package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// reBracketCall matches [TOOL_CALL] ... [/TOOL_CALL] blocks, case-insensitive.
// Some AI models emit this format instead of the expected markdown JSON block.
var reBracketCall = regexp.MustCompile(`(?i)\[TOOL_CALL\]\s*([\s\S]*?)\s*\[/TOOL_CALL\]`)

// StripToolCalls removes markdown code blocks and [TOOL_CALL] blocks from
// a string, leaving only the conversational text.
func StripToolCalls(response string) string {
	// Remove ```json ... ``` and ``` ... ```
	reMarkdown := regexp.MustCompile("(?s)```[\\s\\S]*?```")
	result := reMarkdown.ReplaceAllString(response, "")

	// Remove [TOOL_CALL] ... [/TOOL_CALL]
	result = reBracketCall.ReplaceAllString(result, "")

	return strings.TrimSpace(result)
}

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

	// Strategy 3: Handle [TOOL_CALL] ... [/TOOL_CALL] delimited blocks.
	// Some models emit arrow-syntax (tool => "name") instead of JSON; we parse
	// those here so the orchestrator can still execute the intended tools.
	if len(requests) == 0 {
		bracketMatches := reBracketCall.FindAllStringSubmatch(response, -1)
		for _, match := range bracketMatches {
			if len(match) < 2 {
				continue
			}
			if req := parseBracketCall(match[1]); req != nil {
				requests = append(requests, *req)
			}
		}
	}

	return requests
}

// parseBracketCall parses the body of a [TOOL_CALL]...[/TOOL_CALL] block.
// Handles standard JSON as well as the Ruby/Perl-style arrow syntax
// (tool => "name", parameters => {...}) that some AI models generate.
func parseBracketCall(content string) *ToolRequest {
	content = strings.TrimSpace(content)

	// Fast path: if the block already contains valid JSON with a "tool" field, use it.
	if strings.Contains(content, `"tool"`) {
		reqs := tryParseJSON(content)
		if len(reqs) > 0 {
			return &reqs[0]
		}
	}

	// Arrow-syntax path: extract tool name from  tool => "name"  or  "tool" => "name"
	reToolName := regexp.MustCompile(`(?i)["']?tool["']?\s*=>\s*["']([^"'\s,}\]]+)["']`)
	nameMatch := reToolName.FindStringSubmatch(content)
	if len(nameMatch) < 2 {
		// Also handle colon syntax: tool: "name" (without quotes on the key)
		reToolColon := regexp.MustCompile(`(?i)tool\s*:\s*["']([^"'\s,}\]]+)["']`)
		nameMatch = reToolColon.FindStringSubmatch(content)
		if len(nameMatch) < 2 {
			return nil
		}
	}
	toolName := strings.TrimSpace(nameMatch[1])
	if toolName == "" {
		return nil
	}

	params := make(map[string]interface{})

	// Try to locate the parameters block using brace counting.
	lowerContent := strings.ToLower(content)
	paramIdx := strings.Index(lowerContent, "parameters")
	if paramIdx != -1 {
		rest := content[paramIdx:]
		braceStart := strings.Index(rest, "{")
		if braceStart != -1 {
			depth := 0
			braceEnd := -1
			for i, c := range rest[braceStart:] {
				switch c {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						braceEnd = braceStart + i + 1
					}
				}
				if braceEnd != -1 {
					break
				}
			}
			if braceEnd != -1 {
				paramsStr := strings.TrimSpace(rest[braceStart:braceEnd])
				// Skip CLI-style flag blocks (--flag "value") — treat as empty params.
				if !strings.Contains(paramsStr, " --") {
					var p map[string]interface{}
					if err := json.Unmarshal([]byte(paramsStr), &p); err == nil {
						params = p
					}
				}
			}
		}
	}

	return &ToolRequest{
		ToolName:   toolName,
		Parameters: params,
		RequestID:  fmt.Sprintf("req_%d_%s", time.Now().UnixNano(), toolName),
	}
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
	// Step 0: remove [TOOL_CALL] ... [/TOOL_CALL] blocks emitted by some AI models.
	result := reBracketCall.ReplaceAllString(response, "")

	// Step 1: remove markdown code blocks (```json … ``` and ``` … ```)
	reMarkdown := regexp.MustCompile("```(?:json)?[\\s\\S]*?```")
	result = reMarkdown.ReplaceAllString(result, "")

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
