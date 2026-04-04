package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// FactExtractor extracts structured facts from unstructured text using LLM
type FactExtractor struct {
	provider ai.AIProvider
	logger   *slog.Logger

	// Model to use for extraction
	model string

	// Extraction prompt template
	promptTemplate string

	// Cache extracted facts (credentialID -> []Fact)
	cache map[string][]Fact
	mu    map[string]bool // Mutex flags for cache entries

	// Statistics
	extractedCount int64
	deduplicCount  int64
}

// ExtractedFact represents a single (subject, predicate, object) triple
type ExtractedFact struct {
	// Subject of the fact (e.g., "Alice", "Python", "AWS")
	Subject string

	// Predicate/relationship (e.g., "likes", "supports", "located_in")
	Predicate string

	// Object value (e.g., "coffee", "async/await", "us-east-1")
	Object string

	// Confidence score (0-1, higher = more confident)
	Confidence float64

	// Source text that led to this fact
	SourceText string

	// Extracted timestamp
	ExtractedAt time.Time

	// Additional metadata
	Metadata map[string]string
}

// ExtractionRequest represents a request to extract facts
type ExtractionRequest struct {
	Text     string
	Context  string // Additional context for better extraction
	Language string // "en", "es", "fr", etc. (default: "en")
}

// ExtractionResponse represents extracted facts from text
type ExtractionResponse struct {
	Facts     []ExtractedFact
	Summary   string
	Sentiment string // "positive", "negative", "neutral"
	Topics    []string
	ErrorMsg  string
}

// NewFactExtractor creates a new fact extractor
func NewFactExtractor(provider ai.AIProvider, model string, logger *slog.Logger) *FactExtractor {
	if logger == nil {
		logger = slog.Default()
	}

	return &FactExtractor{
		provider: provider,
		logger:   logger,
		model:    model,
		cache:    make(map[string][]Fact),
		mu:       make(map[string]bool),
		promptTemplate: `Extract structured facts from the following text. Return as JSON array of facts with: subject, predicate, object, confidence (0-1).

Text: {{TEXT}}

Rules:
1. Extract factual information only (not opinions)
2. Use specific, concrete terms
3. Confidence reflects how certain the fact is
4. Subject and Object should be entities (nouns)
5. Predicates should be relationships (verbs)

Example output:
[
  {"subject": "Alice", "predicate": "works_at", "object": "Acme Corp", "confidence": 0.95},
  {"subject": "Acme Corp", "predicate": "located_in", "object": "New York", "confidence": 0.90}
]`,
	}
}

// ExtractFacts extracts facts from text using LLM
func (fe *FactExtractor) ExtractFacts(ctx context.Context, req ExtractionRequest) (*ExtractionResponse, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}

	// Build prompt
	prompt := strings.ReplaceAll(fe.promptTemplate, "{{TEXT}}", req.Text)
	if req.Context != "" {
		prompt = fmt.Sprintf("Context: %s\n\n%s", req.Context, prompt)
	}

	fe.logger.Info("extracting facts from text",
		slog.String("model", fe.model),
		slog.Int("text_length", len(req.Text)),
	)

	// Call LLM via provider
	hist := &ai.ConversationHistory{}
	hist.AddMessage("user", prompt)

	response, err := fe.provider.GenerateWithHistory(ctx, hist)
	if err != nil {
		fe.logger.Error("fact extraction failed", slog.String("error", err.Error()))
		return &ExtractionResponse{
			ErrorMsg: fmt.Sprintf("extraction failed: %v", err),
		}, err
	}

	// Parse response
	extractedFacts, err := fe.parseFactResponse(response)
	if err != nil {
		fe.logger.Error("failed to parse facts", slog.String("error", err.Error()))
		return &ExtractionResponse{
			ErrorMsg: fmt.Sprintf("parse failed: %v", err),
		}, err
	}

	// Deduplicate facts
	deduped := fe.deduplicateFacts(extractedFacts)

	fe.extractedCount += int64(len(deduped))

	return &ExtractionResponse{
		Facts:     deduped,
		Summary:   fmt.Sprintf("Extracted %d unique facts", len(deduped)),
		Sentiment: extractSentiment(response),
		Topics:    extractTopics(response, deduped),
	}, nil
}

// parseFactResponse parses LLM response as JSON facts
func (fe *FactExtractor) parseFactResponse(response string) ([]ExtractedFact, error) {
	// Try to extract JSON array from response
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")

	if start == -1 || end == -1 || start >= end {
		// Try alternative format: extract key-value pairs
		return fe.parseAlternativeFormat(response)
	}

	jsonStr := response[start : end+1]

	var facts []ExtractedFact
	err := json.Unmarshal([]byte(jsonStr), &facts)
	if err != nil {
		fe.logger.Warn("failed to parse JSON facts", slog.String("error", err.Error()))
		return fe.parseAlternativeFormat(response)
	}

	// Validate and enrich facts
	for i := range facts {
		facts[i].ExtractedAt = time.Now()
		if facts[i].Confidence == 0 {
			facts[i].Confidence = 0.7 // Default confidence
		}
		if facts[i].Metadata == nil {
			facts[i].Metadata = make(map[string]string)
		}
	}

	return facts, nil
}

// parseAlternativeFormat parses facts from less structured text
func (fe *FactExtractor) parseAlternativeFormat(text string) ([]ExtractedFact, error) {
	// Simple parsing for text like "subject: X, predicate: Y, object: Z"
	var facts []ExtractedFact

	lines := strings.Split(text, "\n")
	var currentFact ExtractedFact
	var fieldCount int

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "subject:") {
			currentFact.Subject = strings.TrimSpace(strings.TrimPrefix(line, "subject:"))
			fieldCount++
		} else if strings.HasPrefix(line, "predicate:") {
			currentFact.Predicate = strings.TrimSpace(strings.TrimPrefix(line, "predicate:"))
			fieldCount++
		} else if strings.HasPrefix(line, "object:") {
			currentFact.Object = strings.TrimSpace(strings.TrimPrefix(line, "object:"))
			fieldCount++
		} else if strings.HasPrefix(line, "confidence:") {
			confStr := strings.TrimSpace(strings.TrimPrefix(line, "confidence:"))
			// Try to parse as float
			fmt.Sscanf(confStr, "%f", &currentFact.Confidence)
			fieldCount++

			// If we have all fields, add fact
			if fieldCount >= 4 && currentFact.Subject != "" && currentFact.Predicate != "" {
				if currentFact.Confidence == 0 {
					currentFact.Confidence = 0.7
				}
				currentFact.ExtractedAt = time.Now()
				currentFact.Metadata = make(map[string]string)
				facts = append(facts, currentFact)
				currentFact = ExtractedFact{}
				fieldCount = 0
			}
		}
	}

	return facts, nil
}

// deduplicateFacts removes duplicate or near-duplicate facts
func (fe *FactExtractor) deduplicateFacts(facts []ExtractedFact) []ExtractedFact {
	if len(facts) == 0 {
		return facts
	}

	// Simple deduplication: exact match on (subject, predicate, object)
	seen := make(map[string]bool)
	var deduped []ExtractedFact

	for _, fact := range facts {
		key := fmt.Sprintf("%s|%s|%s", fact.Subject, fact.Predicate, fact.Object)

		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, fact)
		} else {
			fe.deduplicCount++
		}
	}

	fe.logger.Info("facts deduplicated",
		slog.Int("original", len(facts)),
		slog.Int("unique", len(deduped)),
	)

	return deduped
}

// extractSentiment attempts to determine overall sentiment
func extractSentiment(text string) string {
	text = strings.ToLower(text)

	positiveWords := []string{"good", "great", "excellent", "love", "happy", "amazing", "best"}
	negativeWords := []string{"bad", "terrible", "hate", "sad", "awful", "worst", "poor"}

	positiveCount := 0
	negativeCount := 0

	for _, word := range positiveWords {
		positiveCount += strings.Count(text, word)
	}

	for _, word := range negativeWords {
		negativeCount += strings.Count(text, word)
	}

	if positiveCount > negativeCount {
		return "positive"
	} else if negativeCount > positiveCount {
		return "negative"
	}
	return "neutral"
}

// extractTopics attempts to extract main topics from text
func extractTopics(text string, facts []ExtractedFact) []string {
	topics := make(map[string]bool)

	// Extract subjects as topics
	for _, fact := range facts {
		if fact.Subject != "" {
			topics[fact.Subject] = true
		}
		if fact.Predicate != "" {
			// Convert predicate to topic (e.g., "works_at" -> "work")
			topic := strings.Split(fact.Predicate, "_")[0]
			topics[topic] = true
		}
	}

	// Extract common words
	words := strings.Fields(strings.ToLower(text))
	for _, word := range words {
		if len(word) > 5 {
			// Keep longer words as potential topics
			topics[word] = true
		}
	}

	// Convert to slice, limit to top 5
	var result []string
	for topic := range topics {
		if len(result) < 5 {
			result = append(result, topic)
		}
	}

	return result
}

// StoreFacts stores extracted facts in the memory system
// Note: Caller is responsible for converting facts to appropriate storage format
func (fe *FactExtractor) StoreFacts(ctx context.Context, db *SQLiteFactSearcher, facts []ExtractedFact) error {
	if db == nil {
		return fmt.Errorf("fact searcher is required")
	}

	for _, fact := range facts {
		// Generate fact ID and timestamp strings
		factID := fmt.Sprintf("fact_%d_%s_%s_%s",
			fact.ExtractedAt.UnixNano(),
			fact.Subject, fact.Predicate, fact.Object)
		timestamp := fact.ExtractedAt.Format(time.RFC3339)
		source := "extractor"

		// InsertFact requires: factID, subject, predicate, object, confidence, source, timestamp
		if err := db.InsertFact(factID, fact.Subject, fact.Predicate, fact.Object,
			fact.Confidence, source, timestamp); err != nil {
			fe.logger.Error("failed to store fact", slog.String("error", err.Error()))
		}
	}

	return nil
}

// GetStats returns extraction statistics
func (fe *FactExtractor) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"extracted":    fe.extractedCount,
		"deduplicated": fe.deduplicCount,
		"model":        fe.model,
	}
}

// Fact represents a stored fact in the memory system
type Fact struct {
	ID         string
	Subject    string
	Predicate  string
	Object     string
	Confidence float64
	CreatedAt  time.Time
}
