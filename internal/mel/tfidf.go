package mel

// tfidf.go — Pure Go TF-IDF cosine similarity for MEL VectorStore.
//
// Provides more accurate semantic similarity than Jaccard for:
//  - Deduplication in Add() (replaces Jaccard threshold check)
//  - Hybrid scoring in Query() (0.6×BM25 + 0.4×TF-IDF cosine)

import (
	"math"
)

// buildIDF computes the Inverse Document Frequency for each term across all docs.
// Returns map[term]→idf_weight.
func buildIDF(corpus [][]string) map[string]float64 {
	N := len(corpus)
	if N == 0 {
		return nil
	}
	// Count documents containing each term
	df := make(map[string]int, 256)
	for _, doc := range corpus {
		seen := make(map[string]bool, len(doc))
		for _, t := range doc {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}
	idf := make(map[string]float64, len(df))
	for term, count := range df {
		// Smoothed IDF: log((N+1)/(df+1)) + 1
		idf[term] = math.Log(float64(N+1)/float64(count+1)) + 1.0
	}
	return idf
}

// vectorizeDoc converts a token list to a TF-IDF weighted vector.
func vectorizeDoc(tokens []string, idf map[string]float64) map[string]float64 {
	if len(tokens) == 0 || len(idf) == 0 {
		return nil
	}
	// Compute TF (term frequency)
	tf := make(map[string]int, len(tokens))
	for _, t := range tokens {
		tf[t]++
	}
	total := len(tokens)
	vec := make(map[string]float64, len(tf))
	for term, count := range tf {
		if w, ok := idf[term]; ok {
			vec[term] = (float64(count) / float64(total)) * w
		}
	}
	return vec
}

// cosineSim computes cosine similarity between two TF-IDF vectors.
func cosineSim(a, b map[string]float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	dot := 0.0
	for term, wa := range a {
		if wb, ok := b[term]; ok {
			dot += wa * wb
		}
	}
	if dot == 0 {
		return 0
	}
	normA := 0.0
	for _, w := range a {
		normA += w * w
	}
	normB := 0.0
	for _, w := range b {
		normB += w * w
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// tfidfSimilarity computes cosine similarity between two token slices
// using a shared IDF built from the two docs combined.
func tfidfSimilarity(docA, docB []string) float64 {
	corpus := [][]string{docA, docB}
	idf := buildIDF(corpus)
	vecA := vectorizeDoc(docA, idf)
	vecB := vectorizeDoc(docB, idf)
	return cosineSim(vecA, vecB)
}

// hybridScore combines BM25 and TF-IDF cosine similarity.
// weight: 0.6 × BM25 + 0.4 × TF-IDF.
func hybridScore(bm25 float64, queryTokens, docTokens []string, idf map[string]float64) float64 {
	if idf == nil {
		return bm25
	}
	qVec := vectorizeDoc(queryTokens, idf)
	dVec := vectorizeDoc(docTokens, idf)
	tfidf := cosineSim(qVec, dVec)
	return 0.6*bm25 + 0.4*tfidf
}
