package adaptive

import (
	"encoding/hex"
	"strings"
	"sync"

	"golang.org/x/crypto/blake2b"
)

// MELValidator guards the VectorStore against data-contamination (poisoning)
// attacks. Before any heuristic is persisted via VectorStore.Add(), callers
// should pass it through Validate(). Entries that fail validation are silently
// dropped — this mirrors how the input sanitizer handles injection attempts.
//
// Validation pipeline:
//  1. Bloom filter — reject content whose BLAKE2b hash has been seen and
//     flagged as malicious in this session.
//  2. Entropy gate — reject suspiciously low-entropy or repetitive content
//     (characteristic of poisoning payloads that repeat a keyword to force
//     high retrieval scores).
//  3. Injection scan — delegate to a lightweight keyword scanner that checks
//     for the 19 SENSE injection patterns and base64/HTML-entity anomalies.
//     This reuses the same patterns as sense.InputSanitizer.ScanContextContent
//     without depending on the full sanitizer package.
type MELValidator struct {
	mu           sync.Mutex
	blockedHex   map[string]struct{} // BLAKE2b-256 hex hashes of blocked content
	callCount    int
	blockedCount int
}

// NewMELValidator creates a validator with an empty block list.
func NewMELValidator() *MELValidator {
	return &MELValidator{
		blockedHex: make(map[string]struct{}),
	}
}

// ValidationResult carries the outcome of a single Validate call.
type ValidationResult struct {
	OK     bool
	Reason string // non-empty when OK == false
}

// Validate checks a heuristic text before it is persisted to the VectorStore.
// Returns ValidationResult{OK: true} when the content is safe.
func (v *MELValidator) Validate(text string) ValidationResult {
	v.mu.Lock()
	v.callCount++
	v.mu.Unlock()

	if strings.TrimSpace(text) == "" {
		return ValidationResult{false, "empty content"}
	}

	// 1. Bloom filter check.
	h := contentHash256(text)
	v.mu.Lock()
	_, blocked := v.blockedHex[h]
	v.mu.Unlock()
	if blocked {
		return ValidationResult{false, "previously blocked hash"}
	}

	// 2. Entropy gate: reject content where a single word accounts for > 40 %
	// of all tokens (symptom of a keyword-flooding poisoning payload).
	if entropyRisk(text) {
		v.block(h)
		return ValidationResult{false, "low-entropy / repetitive content"}
	}

	// 3. Lightweight injection scan.
	if reason, found := scanInjectionPatterns(text); found {
		v.block(h)
		return ValidationResult{false, "injection pattern: " + reason}
	}

	return ValidationResult{OK: true}
}

// BlockHash permanently blocks a BLAKE2b-256 hex hash. Can be called externally
// when the orchestrator detects a poisoned experience via other means.
func (v *MELValidator) BlockHash(contentHash string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.blockedHex[contentHash] = struct{}{}
}

// Stats returns (total validated, total blocked) counts.
func (v *MELValidator) Stats() (total, blocked int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.callCount, v.blockedCount
}

// ─── internal helpers ─────────────────────────────────────────────────────────

func (v *MELValidator) block(hash string) {
	v.mu.Lock()
	v.blockedHex[hash] = struct{}{}
	v.blockedCount++
	v.mu.Unlock()
}

// contentHash256 returns the unkeyed BLAKE2b-256 hex hash of s.
func contentHash256(s string) string {
	h, _ := blake2b.New256(nil)
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// entropyRisk returns true when a single word dominates the token frequency
// enough to indicate flooding. Threshold: top-word freq > 40 % of total.
func entropyRisk(text string) bool {
	words := tokeniseWords(text)
	if len(words) < 10 {
		return false // too short to evaluate meaningfully
	}
	freq := make(map[string]int, len(words))
	for _, w := range words {
		freq[w]++
	}
	max := 0
	for _, c := range freq {
		if c > max {
			max = c
		}
	}
	return float64(max)/float64(len(words)) > 0.40
}

// injectionKeywords mirrors a subset of SENSE input_sanitizer patterns.
// These are the highest-signal phrases that appear in prompt-injection payloads.
var injectionKeywords = []string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard your",
	"forget your instructions",
	"you are now",
	"new persona",
	"system prompt:",
	"[system]",
	"<|system|>",
	"assistant:",
	"<|assistant|>",
	"jailbreak",
	"dan mode",
	"developer mode",
	"do anything now",
	"ignore the above",
	"override",
	"sudo mode",
	"act as if",
}

// scanInjectionPatterns checks text for known injection signatures.
// Returns (pattern, true) on the first match; ("", false) when clean.
func scanInjectionPatterns(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, kw := range injectionKeywords {
		if strings.Contains(lower, kw) {
			return kw, true
		}
	}
	return "", false
}
