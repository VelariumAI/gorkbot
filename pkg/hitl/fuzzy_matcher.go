package hitl

import (
	"encoding/json"
	"strings"
	"unicode"
)

// FuzzyMatcher computes similarity scores between parameter sets to detect
// "similar" operations that the user may have already approved or rejected.
type FuzzyMatcher struct {
	approvedCount int // Track if user has patterns of approvals
}

// NewFuzzyMatcher creates a new fuzzy matcher
func NewFuzzyMatcher() *FuzzyMatcher {
	return &FuzzyMatcher{}
}

// ComputeSimilarity evaluates how similar two parameter sets are.
// Returns a score from 0.0 (completely different) to 1.0 (identical).
// Uses multiple matching strategies in order of strength.
func (fm *FuzzyMatcher) ComputeSimilarity(paramsJSON1, paramsJSON2 string) float64 {
	// Strategy 1: Exact match (score: 1.0)
	if paramsJSON1 == paramsJSON2 {
		return 1.0
	}

	// Parse parameters
	var params1, params2 map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON1), &params1); err != nil {
		return 0.0
	}
	if err := json.Unmarshal([]byte(paramsJSON2), &params2); err != nil {
		return 0.0
	}

	// Strategy 2: Parameter Levenshtein distance (score: 0.7-0.95)
	if score := fm.levenshteinSimilarity(paramsJSON1, paramsJSON2, 0.85); score >= 0.85 {
		return score
	}

	// Strategy 3: Semantic parameter match (score: 0.6-0.8)
	if score := fm.semanticSimilarity(params1, params2); score > 0.6 {
		return score
	}

	// Strategy 4: Key structure similarity (score: 0.4-0.6)
	if score := fm.keyStructureSimilarity(params1, params2); score > 0.4 {
		return score
	}

	// Strategy 5: Category similarity (score: 0.3-0.5)
	if score := fm.categorySimilarity(params1, params2); score > 0.3 {
		return score
	}

	return 0.0
}

// levenshteinSimilarity uses Levenshtein distance to compute similarity.
// Returns normalized similarity (0.0-1.0) where 1.0 is identical.
func (fm *FuzzyMatcher) levenshteinSimilarity(str1, str2 string, threshold float64) float64 {
	distance := fm.levenshteinDistance(str1, str2)
	maxLen := max(len(str1), len(str2))

	if maxLen == 0 {
		return 1.0
	}

	similarity := 1.0 - (float64(distance) / float64(maxLen))

	if similarity >= threshold {
		return similarity
	}

	return 0.0
}

// levenshteinDistance computes the edit distance between two strings
func (fm *FuzzyMatcher) levenshteinDistance(s1, s2 string) int {
	len1, len2 := len(s1), len(s2)

	// Create matrix
	matrix := make([][]int, len1+1)
	for i := 0; i <= len1; i++ {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len1][len2]
}

// semanticSimilarity analyzes the actual parameter values for semantic similarity.
// Looks for matching intents rather than exact string matches.
func (fm *FuzzyMatcher) semanticSimilarity(params1, params2 map[string]interface{}) float64 {
	// Check if parameters have the same keys
	commonKeys := 0
	for key1 := range params1 {
		for key2 := range params2 {
			if key1 == key2 {
				commonKeys++
				break
			}
		}
	}

	totalKeys := max(len(params1), len(params2))
	if totalKeys == 0 {
		return 1.0
	}

	// Key overlap score
	keyOverlap := float64(commonKeys) / float64(totalKeys)
	if keyOverlap < 0.5 {
		return 0.0 // Different parameter structure
	}

	// Now check value similarity for matching keys
	valueSimilarityTotal := 0.0
	matchedCount := 0

	for key := range params1 {
		val1, ok1 := params1[key]
		val2, ok2 := params2[key]

		if !ok1 || !ok2 {
			continue
		}

		matchedCount++

		// Compare values
		similarity := fm.valueSemanticSimilarity(val1, val2)
		valueSimilarityTotal += similarity
	}

	if matchedCount == 0 {
		return 0.5 // Similar structure but no matched values
	}

	avgValueSimilarity := valueSimilarityTotal / float64(matchedCount)

	// Combine scores
	return (keyOverlap*0.3 + avgValueSimilarity*0.7)
}

// valueSemanticSimilarity compares two individual parameter values
func (fm *FuzzyMatcher) valueSemanticSimilarity(val1, val2 interface{}) float64 {
	str1 := normalizePath(stringify(val1))
	str2 := normalizePath(stringify(val2))

	// Exact match
	if str1 == str2 {
		return 1.0
	}

	// For paths/filenames, compare the base names
	base1 := extractBaseName(str1)
	base2 := extractBaseName(str2)

	if base1 == base2 && base1 != "" {
		return 0.85 // Same filename, different path
	}

	// Compute token overlap
	tokens1 := tokenize(str1)
	tokens2 := tokenize(str2)

	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}

	// Count matching tokens
	matchingTokens := 0
	for _, t1 := range tokens1 {
		for _, t2 := range tokens2 {
			if t1 == t2 {
				matchingTokens++
				break
			}
		}
	}

	overlapRatio := float64(matchingTokens) / float64(max(len(tokens1), len(tokens2)))

	return overlapRatio
}

// keyStructureSimilarity compares the parameter structure (keys only)
func (fm *FuzzyMatcher) keyStructureSimilarity(params1, params2 map[string]interface{}) float64 {
	keys1 := make(map[string]bool)
	keys2 := make(map[string]bool)

	for key := range params1 {
		keys1[key] = true
	}
	for key := range params2 {
		keys2[key] = true
	}

	// Count overlapping keys
	overlapCount := 0
	for key := range keys1 {
		if keys2[key] {
			overlapCount++
		}
	}

	totalKeys := max(len(keys1), len(keys2))
	if totalKeys == 0 {
		return 1.0
	}

	similarity := float64(overlapCount) / float64(totalKeys)
	// Only return if similarity is meaningful
	if similarity > 0.4 {
		return similarity * 0.8 // Reduce score since we're only checking structure
	}

	return 0.0
}

// categorySimilarity checks if parameters fall into the same category
// (e.g., all file operations, all network operations)
func (fm *FuzzyMatcher) categorySimilarity(params1, params2 map[string]interface{}) float64 {
	cat1 := fm.parameterCategory(params1)
	cat2 := fm.parameterCategory(params2)

	if cat1 != "" && cat1 == cat2 {
		return 0.5 // Same category but specific values differ
	}

	return 0.0
}

// parameterCategory determines the category of operation from parameters
func (fm *FuzzyMatcher) parameterCategory(params map[string]interface{}) string {
	for key, val := range params {
		keyLower := strings.ToLower(key)

		// File operations
		if strings.Contains(keyLower, "path") || strings.Contains(keyLower, "file") ||
			strings.Contains(keyLower, "dir") {
			return "file"
		}

		// Network operations
		if strings.Contains(keyLower, "url") || strings.Contains(keyLower, "uri") ||
			strings.Contains(keyLower, "host") || strings.Contains(keyLower, "port") {
			return "network"
		}

		// Git operations
		if strings.Contains(keyLower, "repo") || strings.Contains(keyLower, "branch") ||
			strings.Contains(keyLower, "commit") {
			return "git"
		}

		// Shell commands
		if strings.Contains(keyLower, "command") || strings.Contains(keyLower, "cmd") ||
			strings.Contains(keyLower, "script") {
			return "shell"
		}

		// Database operations
		if strings.Contains(keyLower, "query") || strings.Contains(keyLower, "table") ||
			strings.Contains(keyLower, "database") {
			return "database"
		}

		// Check value content
		strVal := strings.ToLower(stringify(val))

		if strings.Contains(strVal, "/") || strings.Contains(strVal, "\\") {
			return "file"
		}
		if strings.Contains(strVal, "http") || strings.Contains(strVal, "@") {
			return "network"
		}
		if strings.Contains(strVal, "git") {
			return "git"
		}
	}

	return ""
}

// Helper functions

func stringify(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}

// normalizePath normalizes a file path for comparison
func normalizePath(p string) string {
	// Replace backslashes with forward slashes
	p = strings.ReplaceAll(p, "\\", "/")

	// Remove trailing slashes
	p = strings.TrimSuffix(p, "/")

	// Remove duplicate slashes
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}

	return strings.ToLower(p)
}

// extractBaseName extracts the filename from a path
func extractBaseName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// tokenize splits a string into meaningful tokens
func tokenize(s string) []string {
	// Split on common delimiters and camelCase
	var tokens []string
	var currentToken strings.Builder

	for _, ch := range s {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
			// Delimiter
			if currentToken.Len() > 0 {
				tokens = append(tokens, strings.ToLower(currentToken.String()))
				currentToken.Reset()
			}
		} else if unicode.IsUpper(ch) && currentToken.Len() > 0 {
			// CamelCase boundary
			tokens = append(tokens, strings.ToLower(currentToken.String()))
			currentToken.Reset()
			currentToken.WriteRune(ch)
		} else {
			currentToken.WriteRune(ch)
		}
	}

	if currentToken.Len() > 0 {
		tokens = append(tokens, strings.ToLower(currentToken.String()))
	}

	return tokens
}

func min(vals ...int) int {
	result := vals[0]
	for _, v := range vals[1:] {
		if v < result {
			result = v
		}
	}
	return result
}

func max(vals ...int) int {
	result := vals[0]
	for _, v := range vals[1:] {
		if v > result {
			result = v
		}
	}
	return result
}
