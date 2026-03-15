package engine

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// IntentGateResult represents the structured output of the intent gate.
type IntentGateResult struct {
	Category    string `json:"category"`
	SpawnAgents []struct {
		Label  string `json:"label"`
		Prompt string `json:"prompt"`
	} `json:"spawn_agents"`
}

var (
	// Highly sophisticated rules for intent classification
	codePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(refactor|debug|implement|function|struct|interface|class|method|api)\b`),
		regexp.MustCompile(`(?i)\b(golang|python|rust|javascript|typescript|js|ts|go|cpp|c\+\+)\b`),
		regexp.MustCompile(`(?i)(how to code|write a script|fix this error|panic:|exception|stacktrace)`),
	}
	researchPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(search|find|look up|investigate|research|explore|summarize)\b`),
		regexp.MustCompile(`(?i)\b(documentation|docs|readme|repo|repository)\b`),
		regexp.MustCompile(`(?i)(what is the|who is|tell me about)`),
	}
	visualPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(ui|ux|frontend|css|html|tailwind|react|vue|component|styling|layout)\b`),
		regexp.MustCompile(`(?i)(make it look|design a|animate|color|pixel)`),
	}
	securityPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(hack|exploit|vulnerability|cve|nmap|scan|pentest|payload|injection|xss|sqli|auth)\b`),
		regexp.MustCompile(`(?i)(bypass|crack|secure|encryption|decrypt)`),
	}
	creativePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(write a poem|story|brainstorm|ideate|creative|imagine|joke)\b`),
	}
	quickPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^(?i)(hi|hello|hey|ping|test|ok|thanks|yes|no)$`),
		regexp.MustCompile(`(?i)\b(what time is it|date today)\b`),
	}
)

// evaluateRules Based Intent Classifier
func evaluateRules(prompt string) *IntentGateResult {
	lowerPrompt := strings.ToLower(strings.TrimSpace(prompt))

	result := &IntentGateResult{
		Category: "auto",
	}

	// 1. Quick conversational checks
	for _, p := range quickPatterns {
		if p.MatchString(lowerPrompt) {
			result.Category = "quick"
			return result
		}
	}

	// 2. Deep/Code checks
	codeMatches := 0
	for _, p := range codePatterns {
		if p.MatchString(lowerPrompt) {
			codeMatches++
		}
	}

	// 3. Security checks
	for _, p := range securityPatterns {
		if p.MatchString(lowerPrompt) {
			result.Category = "security"
			// Spawn a recon agent if it looks like an active pentest request
			if strings.Contains(lowerPrompt, "scan") || strings.Contains(lowerPrompt, "nmap") {
				result.SpawnAgents = append(result.SpawnAgents, struct {
					Label  string `json:"label"`
					Prompt string `json:"prompt"`
				}{
					Label:  "security-recon",
					Prompt: "Perform initial reconnaissance and gather context on the target based on: " + prompt,
				})
			}
			return result
		}
	}

	// 4. Visual/Frontend checks
	for _, p := range visualPatterns {
		if p.MatchString(lowerPrompt) {
			result.Category = "visual"
			return result
		}
	}

	// 5. Research/Docs checks
	researchMatches := 0
	for _, p := range researchPatterns {
		if p.MatchString(lowerPrompt) {
			researchMatches++
		}
	}

	if researchMatches > 0 {
		result.Category = "research"
		if strings.Contains(lowerPrompt, "docs") || strings.Contains(lowerPrompt, "documentation") {
			result.SpawnAgents = append(result.SpawnAgents, struct {
				Label  string `json:"label"`
				Prompt string `json:"prompt"`
			}{
				Label:  "doc-search",
				Prompt: "Search the local or web documentation for concepts related to: " + prompt,
			})
		}
		return result
	}

	if codeMatches > 1 || len(lowerPrompt) > 500 {
		result.Category = "deep"
		if strings.Contains(lowerPrompt, "refactor") || strings.Contains(lowerPrompt, "architect") {
			result.SpawnAgents = append(result.SpawnAgents, struct {
				Label  string `json:"label"`
				Prompt string `json:"prompt"`
			}{
				Label:  "code-architect",
				Prompt: "Analyze the current codebase architecture and prepare context for the requested refactoring: " + prompt,
			})
		}
		return result
	} else if codeMatches > 0 {
		result.Category = "code"
		return result
	}

	// 6. Creative checks
	for _, p := range creativePatterns {
		if p.MatchString(lowerPrompt) {
			result.Category = "creative"
			return result
		}
	}

	return result
}

// RunIntentGate uses a highly sophisticated rules-based intent classifier if the native local LLM is off,
// otherwise it bypasses the rules and uses an API provider (or the native engine handles it elsewhere).
func (o *Orchestrator) RunIntentGate(ctx context.Context, prompt string) *IntentGateResult {
	if !o.NativeLLMEnabled {
		// EXTRAORDINARILY INTELLIGENT AND EXTREMELY SOPHISTICATED RULES-BASED INTENT CLASSIFIER
		return evaluateRules(prompt)
	}

	// Bypass rules-based when Native LLM is enabled, and try to use API provider as fallback
	// or return nil to let native engine handle it.
	provider := o.Consultant
	if provider == nil {
		provider = o.Primary
	}
	if provider == nil {
		return nil
	}

	sysPrompt := `You are the Intent Gate. Your job is to classify the user's prompt into a category and decide if background sub-agents should be spawned to gather information BEFORE the main agent answers.

Valid Categories: auto, deep, quick, visual, research, security, code, creative, data, plan

If the task requires exploring a codebase, reading docs, or doing web research, you should spawn agents. Keep agents focused and single-purpose.

Output ONLY valid JSON in this format:
{
  "category": "research",
  "spawn_agents": [
    {
      "label": "grep-docs",
      "prompt": "Find documentation related to X"
    }
  ]
}

If no background agents are needed, leave "spawn_agents" empty.`

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	history := ai.NewConversationHistory()
	history.AddSystemMessage(sysPrompt)
	history.AddUserMessage(prompt)

	resp, err := provider.GenerateWithHistory(ctx, history)
	if err != nil {
		o.Logger.Warn("Intent Gate failed", "error", err)
		return nil
	}

	start := strings.Index(resp, "{")
	end := strings.LastIndex(resp, "}")
	if start >= 0 && end > start {
		resp = resp[start : end+1]
	}

	var result IntentGateResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		o.Logger.Warn("Intent Gate JSON parse failed", "error", err)
		return nil
	}

	return &result
}
