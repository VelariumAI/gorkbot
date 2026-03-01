package mel

import (
	"math"
	"strings"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

// tokenize splits text into lowercase tokens
func tokenize(text string) []string {
	text = strings.ToLower(text)
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	return fields
}

// buildDF builds document frequency map from a corpus
func buildDF(docs [][]string) map[string]int {
	df := make(map[string]int)
	for _, doc := range docs {
		seen := make(map[string]bool)
		for _, tok := range doc {
			if !seen[tok] {
				df[tok]++
				seen[tok] = true
			}
		}
	}
	return df
}

// avgLen computes average document length
func avgLen(docs [][]string) float64 {
	if len(docs) == 0 {
		return 0
	}
	total := 0
	for _, d := range docs {
		total += len(d)
	}
	return float64(total) / float64(len(docs))
}

// log1pUse returns math.Log(1+x), used to dampen UseCount weighting in Query.
func log1pUse(x float64) float64 {
	return math.Log1p(x)
}

// bm25Score computes BM25 score for a single document against a query
func bm25Score(queryTokens, docTokens []string, df map[string]int, numDocs int, avgDocLen float64) float64 {
	docLen := float64(len(docTokens))
	// build term frequency for this doc
	tf := make(map[string]int)
	for _, t := range docTokens {
		tf[t]++
	}

	score := 0.0
	for _, qt := range queryTokens {
		freq := float64(tf[qt])
		if freq == 0 {
			continue
		}
		// IDF with smoothing
		n := float64(numDocs)
		dfq := float64(df[qt]) + 0.5
		idf := math.Log((n-dfq+0.5)/(dfq) + 1)
		// normalized TF
		tfNorm := (freq * (bm25K1 + 1)) / (freq + bm25K1*(1-bm25B+bm25B*docLen/avgDocLen))
		score += idf * tfNorm
	}
	return score
}
