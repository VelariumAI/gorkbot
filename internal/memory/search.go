package memory

import (
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"
)

// SearchResult represents a memory search result
type SearchResult struct {
	Fact       *Fact
	Relevance  float64
	RecencyBoost float64
	ConfidenceScore float64
}

// AdvancedSearcher provides sophisticated memory search
type AdvancedSearcher struct {
	logger    *slog.Logger
	facts     map[string]*Fact
	index     map[string][]*Fact
}

// NewAdvancedSearcher creates a new advanced searcher
func NewAdvancedSearcher(logger *slog.Logger, facts map[string]*Fact, index map[string][]*Fact) *AdvancedSearcher {
	if logger == nil {
		logger = slog.Default()
	}

	return &AdvancedSearcher{
		logger: logger,
		facts:  facts,
		index:  index,
	}
}

// Search performs advanced search with ranking
func (as *AdvancedSearcher) Search(query string, limit int) []*SearchResult {
	var results []*SearchResult

	// Parse query
	terms := strings.Fields(strings.ToLower(query))

	// Score all facts
	for _, fact := range as.facts {
		relevance := as.computeRelevance(fact, terms)
		if relevance > 0 {
			recency := as.computeRecency(fact)
			result := &SearchResult{
				Fact:           fact,
				Relevance:      relevance,
				RecencyBoost:   recency,
				ConfidenceScore: fact.Confidence,
			}

			// Combined score
			combined := relevance * 0.5 + recency * 0.3 + fact.Confidence * 0.2
			if combined > 0 {
				results = append(results, result)
			}
		}
	}

	// Sort by combined score
	sort.Slice(results, func(i, j int) bool {
		scoreI := results[i].Relevance * 0.5 + results[i].RecencyBoost * 0.3 + results[i].ConfidenceScore * 0.2
		scoreJ := results[j].Relevance * 0.5 + results[j].RecencyBoost * 0.3 + results[j].ConfidenceScore * 0.2
		return scoreI > scoreJ
	})

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	as.logger.Debug("search completed",
		slog.String("query", query),
		slog.Int("results", len(results)),
	)

	return results
}

// RankedRetrieval performs ranked retrieval with RRF (Reciprocal Rank Fusion)
func (as *AdvancedSearcher) RankedRetrieval(query string, k int) []*SearchResult {
	var results []*SearchResult

	terms := strings.Fields(strings.ToLower(query))

	// Multiple ranking strategies (RRF combines them)
	for _, fact := range as.facts {
		relevanceRank := as.rankByRelevance(fact, terms)
		recencyRank := as.rankByRecency(fact)
		confidenceRank := as.rankByConfidence(fact)

		// RRF formula: 1/(k + rank1) + 1/(k + rank2) + ...
		rrf := 1.0/float64(k+relevanceRank) +
			1.0/float64(k+recencyRank) +
			1.0/float64(k+confidenceRank)

		if rrf > 0.01 {
			results = append(results, &SearchResult{
				Fact:            fact,
				Relevance:       float64(relevanceRank),
				RecencyBoost:    float64(recencyRank),
				ConfidenceScore: rrf,
			})
		}
	}

	// Sort by RRF score
	sort.Slice(results, func(i, j int) bool {
		return results[i].ConfidenceScore > results[j].ConfidenceScore
	})

	return results
}

// Helper ranking methods

func (as *AdvancedSearcher) computeRelevance(fact *Fact, terms []string) float64 {
	matches := 0
	for _, term := range terms {
		if strings.Contains(strings.ToLower(fact.Subject), term) ||
			strings.Contains(strings.ToLower(fact.Object), term) ||
			strings.Contains(strings.ToLower(fact.Predicate), term) {
			matches++
		}
	}

	if matches == 0 {
		return 0
	}

	return float64(matches) / float64(len(terms))
}

func (as *AdvancedSearcher) computeRecency(fact *Fact) float64 {
	// Exponential decay: e^(-0.5 * days)
	now := time.Now().Unix()
	age := now - fact.LastConfirmed
	ageInDays := float64(age) / (24 * 3600)

	return expDecay(-0.5 * ageInDays)
}

func (as *AdvancedSearcher) rankByRelevance(fact *Fact, terms []string) int {
	matches := 0
	for _, term := range terms {
		if strings.Contains(strings.ToLower(fact.Subject), term) {
			matches += 2
		}
		if strings.Contains(strings.ToLower(fact.Object), term) {
			matches += 1
		}
	}

	return len(terms) - matches // Invert for ranking
}

func (as *AdvancedSearcher) rankByRecency(fact *Fact) int {
	ageDays := float64(time.Now().Unix()-fact.LastConfirmed) / (24 * 3600)
	switch {
	case ageDays < 1:
		return 1
	case ageDays < 7:
		return 2
	case ageDays < 30:
		return 3
	default:
		return 4
	}
}

func (as *AdvancedSearcher) rankByConfidence(fact *Fact) int {
	// Higher confidence = better rank (lower rank number)
	if fact.Confidence > 0.9 {
		return 1
	} else if fact.Confidence > 0.7 {
		return 2
	} else if fact.Confidence > 0.5 {
		return 3
	}
	return 4
}

func expDecay(x float64) float64 {
	if x > 0 {
		return 1.0 / (1.0 + x)
	}
	return math.Exp(x)
}

// HybridSearch combines multiple search strategies
type HybridSearch struct {
	logger   *slog.Logger
	searcher *AdvancedSearcher
}

// NewHybridSearch creates a hybrid searcher
func NewHybridSearch(logger *slog.Logger, searcher *AdvancedSearcher) *HybridSearch {
	return &HybridSearch{
		logger:   logger,
		searcher: searcher,
	}
}

// Search performs hybrid search combining strategies
func (hs *HybridSearch) Search(query string, limit int) []*SearchResult {
	// Use RRF for best results
	return hs.searcher.RankedRetrieval(query, 10)
}
