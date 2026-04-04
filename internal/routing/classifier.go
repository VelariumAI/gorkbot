package routing

import (
	"regexp"
	"strings"
)

// IntentClassifier classifies the intent of a task from its content
type IntentClassifier struct {
	patterns map[string]*regexp.Regexp
}

// Intent types
const (
	IntentRefactoring  = "refactoring"
	IntentOptimization = "optimization"
	IntentDebugging    = "debugging"
	IntentTesting      = "testing"
	IntentArchitecture = "architecture"
	IntentDocumentation = "documentation"
	IntentImplementation = "implementation"
	IntentAnalysis     = "analysis"
	IntentCreative     = "creative"
	IntentBrainstorming = "brainstorming"
	IntentPlanning     = "planning"
	IntentResearch     = "research"
	IntentLogic        = "logic"
	IntentUnknown      = "unknown"
)

// NewIntentClassifier creates a new intent classifier
func NewIntentClassifier() *IntentClassifier {
	return &IntentClassifier{
		patterns: compilePatterns(),
	}
}

// Classify analyzes content and returns the detected intent
func (ic *IntentClassifier) Classify(content string) string {
	content = strings.ToLower(content)

	// Check patterns in priority order
	if matches(content, ic.patterns[IntentRefactoring]) {
		return IntentRefactoring
	}

	if matches(content, ic.patterns[IntentOptimization]) {
		return IntentOptimization
	}

	if matches(content, ic.patterns[IntentDebugging]) {
		return IntentDebugging
	}

	if matches(content, ic.patterns[IntentTesting]) {
		return IntentTesting
	}

	if matches(content, ic.patterns[IntentArchitecture]) {
		return IntentArchitecture
	}

	if matches(content, ic.patterns[IntentPlanning]) {
		return IntentPlanning
	}

	if matches(content, ic.patterns[IntentAnalysis]) {
		return IntentAnalysis
	}

	if matches(content, ic.patterns[IntentResearch]) {
		return IntentResearch
	}

	if matches(content, ic.patterns[IntentCreative]) {
		return IntentCreative
	}

	if matches(content, ic.patterns[IntentBrainstorming]) {
		return IntentBrainstorming
	}

	if matches(content, ic.patterns[IntentDocumentation]) {
		return IntentDocumentation
	}

	if matches(content, ic.patterns[IntentImplementation]) {
		return IntentImplementation
	}

	// Default
	return IntentUnknown
}

// ClassifyMulti classifies content and returns all matching intents
func (ic *IntentClassifier) ClassifyMulti(content string) []string {
	content = strings.ToLower(content)
	var intents []string

	if matches(content, ic.patterns[IntentRefactoring]) {
		intents = append(intents, IntentRefactoring)
	}
	if matches(content, ic.patterns[IntentOptimization]) {
		intents = append(intents, IntentOptimization)
	}
	if matches(content, ic.patterns[IntentDebugging]) {
		intents = append(intents, IntentDebugging)
	}
	if matches(content, ic.patterns[IntentTesting]) {
		intents = append(intents, IntentTesting)
	}
	if matches(content, ic.patterns[IntentArchitecture]) {
		intents = append(intents, IntentArchitecture)
	}
	if matches(content, ic.patterns[IntentPlanning]) {
		intents = append(intents, IntentPlanning)
	}
	if matches(content, ic.patterns[IntentAnalysis]) {
		intents = append(intents, IntentAnalysis)
	}
	if matches(content, ic.patterns[IntentResearch]) {
		intents = append(intents, IntentResearch)
	}

	if len(intents) == 0 {
		intents = append(intents, IntentUnknown)
	}

	return intents
}

// compilePatterns returns regex patterns for intent detection
func compilePatterns() map[string]*regexp.Regexp {
	return map[string]*regexp.Regexp{
		IntentRefactoring: regexp.MustCompile(
			`(?i)(refactor|rewrite|restructure|reorganize|rework|redo|overhaul|redesign|clean.?up|simplify code)`,
		),
		IntentOptimization: regexp.MustCompile(
			`(?i)(optim|speed.?up|improve.?perform|faster|efficiency|bottleneck|slow|lag|latency|throughput)`,
		),
		IntentDebugging: regexp.MustCompile(
			`(?i)(debug|fix|broken|error|bug|issue|crash|fail|not.?work|problem|trouble|exception)`,
		),
		IntentTesting: regexp.MustCompile(
			`(?i)(test|unit.?test|integration.?test|e2e|end.?to.?end|qa|quality|coverage|assertion)`,
		),
		IntentArchitecture: regexp.MustCompile(
			`(?i)(architecture|design.?pattern|structure|framework|layer|module|component|interface)`,
		),
		IntentPlanning: regexp.MustCompile(
			`(?i)(plan|strategy|roadmap|timeline|schedule|estimate|scope|requirement)`,
		),
		IntentAnalysis: regexp.MustCompile(
			`(?i)(analyz|evaluate|assess|examine|investigate|study|impact|metric)`,
		),
		IntentResearch: regexp.MustCompile(
			`(?i)(research|explore|investigate|discover|understand|learn|explain|how)`,
		),
		IntentCreative: regexp.MustCompile(
			`(?i)(create|write|generate|imagine|invent|design|build something new|novel idea)`,
		),
		IntentBrainstorming: regexp.MustCompile(
			`(?i)(brainstorm|ideate|think|suggest|propose|option|alternative|what.?if)`,
		),
		IntentDocumentation: regexp.MustCompile(
			`(?i)(document|comment|docstring|readme|guide|tutorial|example|explain)`,
		),
		IntentImplementation: regexp.MustCompile(
			`(?i)(implement|code|build|add.?feature|create|develop|start|begin)`,
		),
	}
}

// matches checks if pattern matches content
func matches(content string, pattern *regexp.Regexp) bool {
	if pattern == nil {
		return false
	}
	return pattern.MatchString(content)
}

// ConfidenceScore returns a confidence score (0-1) for the classification
func (ic *IntentClassifier) ConfidenceScore(content string, intent string) float32 {
	content = strings.ToLower(content)

	// Count keyword matches
	pattern, ok := ic.patterns[intent]
	if !ok {
		return 0.0
	}

	matches := pattern.FindAllString(content, -1)
	confidence := float32(len(matches)) * 0.2 // 0.2 per match

	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// SuggestIntent suggests the most likely intent based on multiple classifiers
func (ic *IntentClassifier) SuggestIntent(content string) (string, float32) {
	intents := ic.ClassifyMulti(content)
	if len(intents) == 0 {
		return IntentUnknown, 0.0
	}

	// Calculate confidence for primary intent
	primary := intents[0]
	confidence := ic.ConfidenceScore(content, primary)

	return primary, confidence
}

// KeywordExtractor extracts intent-relevant keywords from content
type KeywordExtractor struct {
	keywords map[string]int
}

// NewKeywordExtractor creates a new keyword extractor
func NewKeywordExtractor() *KeywordExtractor {
	return &KeywordExtractor{
		keywords: make(map[string]int),
	}
}

// Extract extracts keywords from content
func (ke *KeywordExtractor) Extract(content string) map[string]int {
	words := strings.Fields(strings.ToLower(content))

	intentKeywords := map[string]bool{
		"refactor":      true,
		"optimize":      true,
		"debug":         true,
		"test":          true,
		"architecture":  true,
		"design":        true,
		"implement":     true,
		"fix":           true,
		"improve":       true,
		"analyze":       true,
		"research":      true,
		"documentation": true,
		"performance":   true,
		"bug":           true,
		"error":         true,
		"feature":       true,
		"code":          true,
		"security":      true,
		"quality":       true,
	}

	keywords := make(map[string]int)
	for _, word := range words {
		word = strings.TrimSpace(word)
		// Remove trailing punctuation
		word = strings.TrimRight(word, ".,!?;:\"'")
		if intentKeywords[word] {
			keywords[word]++
		}
	}

	return keywords
}

// ScoreIntent scores how well an intent matches the content
func (ic *IntentClassifier) ScoreIntent(content string, intent string) float32 {
	if intent == IntentUnknown {
		return 0.0
	}

	pattern, ok := ic.patterns[intent]
	if !ok {
		return 0.0
	}

	matches := pattern.FindAllString(strings.ToLower(content), -1)
	score := float32(len(matches)) / float32(len(strings.Fields(content))) * 100

	if score > 100 {
		score = 100
	}

	return score
}

// GetAllIntents returns all known intent types
func GetAllIntents() []string {
	return []string{
		IntentRefactoring,
		IntentOptimization,
		IntentDebugging,
		IntentTesting,
		IntentArchitecture,
		IntentDocumentation,
		IntentImplementation,
		IntentAnalysis,
		IntentCreative,
		IntentBrainstorming,
		IntentPlanning,
		IntentResearch,
		IntentLogic,
	}
}
