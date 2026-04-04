package memory

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Fact represents an extracted fact (subject-predicate-object triple)
type Fact struct {
	ID            string    // Unique fact ID
	Subject       string    // Entity being described
	Predicate     string    // Relationship/property
	Object        string    // Target entity/value
	Confidence    float64   // 0.0-1.0 confidence score
	Source        string    // Where the fact came from
	Timestamp     int64     // When learned
	LastConfirmed int64     // When last confirmed
	DecayFactor   float64   // Temporal decay multiplier
}

// FactExtractor extracts facts from text using LLM
type FactExtractor struct {
	logger    *slog.Logger
	facts     map[string]*Fact
	index     map[string][]*Fact // Subject index
	deduped   map[string]string   // Deduplication cache
}

// NewFactExtractor creates a new fact extractor
func NewFactExtractor(logger *slog.Logger) *FactExtractor {
	if logger == nil {
		logger = slog.Default()
	}

	return &FactExtractor{
		logger:  logger,
		facts:   make(map[string]*Fact),
		index:   make(map[string][]*Fact),
		deduped: make(map[string]string),
	}
}

// Extract extracts facts from text
func (fe *FactExtractor) Extract(text string, source string) []*Fact {
	if len(text) < 20 {
		return nil // Skip very short text
	}

	extracted := fe.parseText(text, source)

	for _, fact := range extracted {
		fe.AddFact(fact)
	}

	fe.logger.Debug("extracted facts",
		slog.Int("count", len(extracted)),
		slog.String("source", source),
	)

	return extracted
}

// parseText parses text and extracts facts (simplified NLP)
func (fe *FactExtractor) parseText(text string, source string) []*Fact {
	facts := make([]*Fact, 0)

	// Simple pattern-based extraction
	sentences := strings.Split(text, ".")
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) < 10 {
			continue
		}

		// Extract facts from sentence structure
		if fact := fe.parseSentence(sentence, source); fact != nil {
			facts = append(facts, fact)
		}
	}

	return facts
}

// parseSentence extracts fact from single sentence
func (fe *FactExtractor) parseSentence(sentence string, source string) *Fact {
	// Simple heuristics for fact extraction
	words := strings.Fields(sentence)
	if len(words) < 3 {
		return nil
	}

	// Pattern: "X is Y"
	for i, word := range words {
		if word == "is" && i > 0 && i < len(words)-1 {
			subject := strings.Join(words[:i], " ")
			object := strings.Join(words[i+1:], " ")
			object = strings.TrimRight(object, ",")

			return &Fact{
				ID:            fmt.Sprintf("fact-%d", time.Now().UnixNano()),
				Subject:       subject,
				Predicate:     "is",
				Object:        object,
				Confidence:    0.8,
				Source:        source,
				Timestamp:     time.Now().Unix(),
				LastConfirmed: time.Now().Unix(),
				DecayFactor:   1.0,
			}
		}
	}

	// Pattern: "X has Y"
	for i, word := range words {
		if word == "has" && i > 0 && i < len(words)-1 {
			subject := strings.Join(words[:i], " ")
			object := strings.Join(words[i+1:], " ")

			return &Fact{
				ID:            fmt.Sprintf("fact-%d", time.Now().UnixNano()),
				Subject:       subject,
				Predicate:     "has",
				Object:        object,
				Confidence:    0.75,
				Source:        source,
				Timestamp:     time.Now().Unix(),
				LastConfirmed: time.Now().Unix(),
				DecayFactor:   1.0,
			}
		}
	}

	return nil
}

// AddFact adds a fact to the knowledge base
func (fe *FactExtractor) AddFact(fact *Fact) {
	if fact == nil || fact.Subject == "" {
		return
	}

	// Check for duplicates
	dedupKey := fact.Subject + "|" + fact.Predicate + "|" + fact.Object
	if existingID, exists := fe.deduped[dedupKey]; exists {
		// Update existing fact
		if existing, ok := fe.facts[existingID]; ok {
			existing.LastConfirmed = time.Now().Unix()
			existing.Confidence = (existing.Confidence + fact.Confidence) / 2
			return
		}
	}

	// Add new fact
	fe.facts[fact.ID] = fact
	fe.deduped[dedupKey] = fact.ID

	// Index by subject
	fe.index[fact.Subject] = append(fe.index[fact.Subject], fact)

	fe.logger.Debug("added fact",
		slog.String("subject", fact.Subject),
		slog.String("predicate", fact.Predicate),
		slog.Float64("confidence", fact.Confidence),
	)
}

// GetFactsBySubject retrieves facts about a subject
func (fe *FactExtractor) GetFactsBySubject(subject string) []*Fact {
	return fe.index[subject]
}

// SearchFacts searches facts matching criteria
func (fe *FactExtractor) SearchFacts(query string) []*Fact {
	var results []*Fact

	for _, fact := range fe.facts {
		if strings.Contains(strings.ToLower(fact.Subject), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(fact.Object), strings.ToLower(query)) {
			results = append(results, fact)
		}
	}

	return results
}

// ApplyDecay applies temporal decay to facts
func (fe *FactExtractor) ApplyDecay(decayFunction string) {
	now := time.Now().Unix()

	for _, fact := range fe.facts {
		age := now - fact.Timestamp
		ageInDays := float64(age) / (24 * 3600)

		var decayedConfidence float64

		switch decayFunction {
		case "exponential":
			// e^(-0.1 * days)
			decayedConfidence = fact.Confidence * exp(-0.1 * ageInDays)
		case "linear":
			// Reduce by 1% per day
			decayedConfidence = fact.Confidence * (1.0 - (ageInDays * 0.01))
		case "logarithmic":
			// log(ageInDays + 1)
			decayedConfidence = fact.Confidence / (1.0 + log(ageInDays+1))
		default:
			decayedConfidence = fact.Confidence
		}

		if decayedConfidence < 0 {
			decayedConfidence = 0
		}

		fact.Confidence = decayedConfidence
	}

	fe.logger.Debug("applied temporal decay",
		slog.String("function", decayFunction),
		slog.Int("facts_affected", len(fe.facts)),
	)
}

// GetFacts returns all facts
func (fe *FactExtractor) GetFacts() map[string]*Fact {
	return fe.facts
}

// GetIndex returns the subject index
func (fe *FactExtractor) GetIndex() map[string][]*Fact {
	return fe.index
}

// GetStats returns fact extraction statistics
func (fe *FactExtractor) GetStats() map[string]interface{} {
	totalConfidence := 0.0
	for _, fact := range fe.facts {
		totalConfidence += fact.Confidence
	}

	avgConfidence := 0.0
	if len(fe.facts) > 0 {
		avgConfidence = totalConfidence / float64(len(fe.facts))
	}

	return map[string]interface{}{
		"total_facts":      len(fe.facts),
		"avg_confidence":   avgConfidence,
		"unique_subjects":  len(fe.index),
		"deduped_removed":  len(fe.deduped),
	}
}

// Helper math functions
func exp(x float64) float64 {
	result := 1.0
	term := 1.0
	for i := 1; i < 20; i++ {
		term *= x / float64(i)
		result += term
	}
	return result
}

func log(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Simple log approximation
	return logApprox(x)
}

func logApprox(x float64) float64 {
	if x < 0.5 {
		return 0
	}
	// Rough log approximation
	result := 0.0
	power := x
	for i := 1; i < 10; i++ {
		result += power / float64(i)
		power *= -((x - 1) / x)
	}
	return result
}
