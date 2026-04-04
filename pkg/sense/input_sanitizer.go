package sense

// input_sanitizer.go — SENSE Stabilization Middleware
//
// Implements §3.1 of Poehnelt (2026): "Self-Evolving Neural Stabilization
// Engine — Input Hardening".  Every agent-supplied input is treated as
// adversarial from day one.  Three invariants are enforced on every tool call:
//
//  1. Control-character rejection: any byte < 0x20 triggers an immediate error.
//     Prevents terminal injection, ANSI escape attacks, and null-byte smuggling.
//
//  2. Path sandboxing: string parameters whose key names hint at file paths are
//     canonicalized with filepath.Abs + filepath.EvalSymlinks and then confined
//     to the process CWD.  Uses os + path/filepath exclusively — no shell.
//
//  3. Resource-name validation: name-like parameters are scanned for the three
//     primary adversarial URI characters (?, #, %) that can poison tool names,
//     resource identifiers, or URL template slots.
//
// The CWD anchor is resolved once at construction time (TOCTOU-safe).
// All methods are safe for concurrent use.

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

// SanitizerViolation describes a single policy violation detected during input
// sanitization.  It carries enough context for trace logging and error messages.
type SanitizerViolation struct {
	// Field is the parameter key that triggered the violation.
	Field string
	// Value is the raw (possibly truncated) value that was rejected.
	Value string
	// Policy identifies which invariant was triggered.
	Policy SanitizerPolicy
	// Detail is a human-readable explanation.
	Detail string
}

func (v SanitizerViolation) Error() string {
	return fmt.Sprintf("SENSE[stabilizer] policy=%s field=%q: %s", v.Policy, v.Field, v.Detail)
}

// SanitizerPolicy identifies which invariant triggered a violation.
type SanitizerPolicy string

const (
	PolicyControlChar    SanitizerPolicy = "control_char_rejection"
	PolicyPathSandbox    SanitizerPolicy = "path_sandbox"
	PolicyResourceName   SanitizerPolicy = "resource_name"
	PolicyNullByte       SanitizerPolicy = "null_byte"
	PolicyUnicodeControl SanitizerPolicy = "unicode_control"
)

// criticalToolsSanitize lists tools that must ALWAYS be sanitized regardless
// of any bypass request. These are the highest-risk tools where injection attacks
// are most catastrophic.
var criticalToolsSanitize = map[string]bool{
	"bash":           true,
	"write_file":     true,
	"create_tool":    true,
	"modify_tool":    true,
	"code_exec":      true,
	"privileged_exec": true,
}

// IsCriticalTool returns true if a tool must always have its parameters sanitized.
func IsCriticalTool(name string) bool {
	return criticalToolsSanitize[name]
}

// InputSanitizer is the SENSE stabilization middleware.  It validates the
// full parameter map of any tool call before execution is permitted.
//
// Create exactly one sanitizer per process (see NewInputSanitizer) and share
// it across all goroutines — it is immutable after construction.
type InputSanitizer struct {
	// cwd is the resolved, symlink-free working directory used as the sandbox
	// anchor for all path parameters.  Immutable after construction.
	cwd string
	// allowedPrefixes are additional directory prefixes permitted beyond CWD.
	// These cover standard system temp directories, the user home directory,
	// and Android/Termux-specific storage paths that AI tools legitimately
	// need to write to (screenshots, downloads, temp files).
	allowedPrefixes []string
	// counters uses atomic operations so the hot path (SanitizeParams) never
	// acquires a mutex.
	counters sanitizerCounters
	// mu is kept for potential future use but is no longer used in the hot path.
	mu sync.Mutex
}

// SanitizerStats tracks cumulative sanitization metrics.
// Fields are plain int64 for easy external consumption; the InputSanitizer
// uses atomic operations internally and copies values out in Stats().
type SanitizerStats struct {
	Accepted         int64
	Rejected         int64
	ControlCharHits  int64
	PathEscapeHits   int64
	ResourceNameHits int64
}

// sanitizerCounters holds the live atomic counters, separate from the exported
// snapshot struct so callers can use SanitizerStats without knowing about atomics.
type sanitizerCounters struct {
	accepted         atomic.Int64
	rejected         atomic.Int64
	controlCharHits  atomic.Int64
	pathEscapeHits   atomic.Int64
	resourceNameHits atomic.Int64
}

// NewInputSanitizer constructs an InputSanitizer anchored to the current
// working directory.  EvalSymlinks is applied to the CWD so symlink-chain
// attacks cannot escape the sandbox.
func NewInputSanitizer() (*InputSanitizer, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("SENSE: cannot determine CWD for path sandboxing: %w", err)
	}
	// Resolve any symlinks in the CWD itself for a stable, canonical anchor.
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}

	// Build the allowlist of standard system paths that AI tools legitimately
	// need to access outside the CWD.  All entries are resolved through
	// EvalSymlinks so symlink-based escapes are still blocked.
	allowed := []string{
		"/tmp",            // POSIX standard temp dir
		"/var/tmp",        // persistent temp dir
		"/sdcard",         // Android external storage (symlink to /storage/emulated/0)
		"/storage",        // Android storage root (all volumes)
		"/data/local/tmp", // Android ADB temp dir
	}

	// Add the user home directory — the most common legitimate write target.
	if home, err := os.UserHomeDir(); err == nil {
		// Resolve symlinks in home dir too.
		if resolved, err := filepath.EvalSymlinks(home); err == nil {
			allowed = append(allowed, resolved)
		} else {
			allowed = append(allowed, home)
		}
	}

	// Resolve each allowlist entry through EvalSymlinks where possible so that
	// path-prefix matching works on the canonical forms.
	resolved := make([]string, 0, len(allowed))
	for _, p := range allowed {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			resolved = append(resolved, r)
		} else {
			resolved = append(resolved, filepath.Clean(p))
		}
	}

	// Also include $TMPDIR if set (e.g. on macOS or some Termux builds).
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		if r, err := filepath.EvalSymlinks(tmpdir); err == nil {
			resolved = append(resolved, r)
		} else {
			resolved = append(resolved, filepath.Clean(tmpdir))
		}
	}

	return &InputSanitizer{cwd: cwd, allowedPrefixes: resolved}, nil
}

// SanitizeParams validates every value in params before tool execution.
// It traverses one level of array nesting ([]interface{}) to validate
// individual elements.  Returns the first SanitizerViolation encountered,
// typed as error, so callers can use errors.As for introspection.
func (s *InputSanitizer) SanitizeParams(params map[string]interface{}) error {
	for key, val := range params {
		if err := s.sanitizeValue(key, val); err != nil {
			s.counters.rejected.Add(1)
			return err
		}
	}
	s.counters.accepted.Add(1)
	return nil
}

// sanitizeValue dispatches on the concrete type of val.
func (s *InputSanitizer) sanitizeValue(key string, val interface{}) error {
	switch v := val.(type) {
	case string:
		return s.sanitizeString(key, v)
	case []interface{}:
		for i, elem := range v {
			if sv, ok := elem.(string); ok {
				if err := s.sanitizeString(fmt.Sprintf("%s[%d]", key, i), sv); err != nil {
					return err
				}
			}
		}
	case map[string]interface{}:
		// Recurse one level into nested objects (e.g. JSON bodies, header maps).
		for subKey, subVal := range v {
			qualKey := key + "." + subKey
			if err := s.sanitizeValue(qualKey, subVal); err != nil {
				return err
			}
		}
	}
	return nil
}

// sanitizeString applies all three stabilization invariants to a single
// string value.
func (s *InputSanitizer) sanitizeString(key, value string) error {
	// ── Invariant 1: Control character rejection ──────────────────────────────
	if err := s.rejectControlChars(key, value); err != nil {
		return err
	}

	// ── Invariant 2: Path sandboxing ─────────────────────────────────────────
	if isPathParam(key) && value != "" {
		if err := s.sandboxPath(key, value); err != nil {
			return err
		}
	}

	// ── Invariant 3: Resource name validation ─────────────────────────────────
	if isNameParam(key) {
		if err := s.validateResourceName(key, value); err != nil {
			return err
		}
	}

	return nil
}

// rejectControlChars scans for:
//   - ASCII control characters (0x00–0x1F) — with the exception of the three
//     standard text whitespace characters (\t=0x09, \n=0x0A, \r=0x0D) which
//     are legitimate in file content, multi-line strings, and shell scripts.
//     Blocking them would make write_file unusable for any file with > 1 line.
//   - Unicode format/control categories (Cc, Cf) beyond ASCII range
//
// What we ARE blocking: terminal injection sequences (ESC=0x1B, BEL=0x07,
// BS=0x08, VT=0x0B, FF=0x0C, SO=0x0E–US=0x1F), null bytes, and Unicode
// invisible-direction/format controls.
func (s *InputSanitizer) rejectControlChars(field, value string) error {
	for i, r := range value {
		// Null byte — separate policy for clarity in logs.
		if r == 0x00 {
			s.counters.controlCharHits.Add(1)
			return SanitizerViolation{
				Field:  field,
				Value:  safePreview(value),
				Policy: PolicyNullByte,
				Detail: fmt.Sprintf("null byte (0x00) at position %d — rejected", i),
			}
		}
		// Allow standard text whitespace: HT (\t=0x09), LF (\n=0x0A), CR (\r=0x0D).
		// These appear in every multi-line file and shell script; rejecting them
		// breaks write_file for any content with more than one line.
		if r == 0x09 || r == 0x0A || r == 0x0D {
			continue
		}
		// Reject the remaining ASCII control range (0x01–0x08, 0x0B–0x0C, 0x0E–0x1F).
		// This covers terminal injection sequences (ESC=0x1B, BEL=0x07, etc.)
		// while leaving printable text untouched.
		if r < 0x20 {
			s.counters.controlCharHits.Add(1)
			return SanitizerViolation{
				Field:  field,
				Value:  safePreview(value),
				Policy: PolicyControlChar,
				Detail: fmt.Sprintf("ASCII control character 0x%02X at position %d — rejected", r, i),
			}
		}
		// Unicode control/format characters outside ASCII range (U+007F–U+009F,
		// Cc/Cf categories) can be used to smuggle hidden instructions.
		if r > 0x7E && (unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)) {
			s.counters.controlCharHits.Add(1)
			return SanitizerViolation{
				Field:  field,
				Value:  safePreview(value),
				Policy: PolicyUnicodeControl,
				Detail: fmt.Sprintf("Unicode control/format character U+%04X at position %d — rejected", r, i),
			}
		}
	}
	return nil
}

// sandboxPath canonicalizes path and enforces confinement to the CWD sandbox.
// Uses filepath.Abs and filepath.EvalSymlinks exclusively — no shell, no glob.
func (s *InputSanitizer) sandboxPath(field, path string) error {
	// Resolve to absolute path using the process CWD as the base.
	abs, err := filepath.Abs(path)
	if err != nil {
		return SanitizerViolation{
			Field:  field,
			Value:  safePreview(path),
			Policy: PolicyPathSandbox,
			Detail: fmt.Sprintf("filepath.Abs failed: %v", err),
		}
	}

	// Resolve symlinks where the target exists; for new files use Clean.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		resolved = filepath.Clean(abs)
	}

	// Enforce sandbox: resolved path must start with the CWD anchor.
	// Append a separator to both sides to prevent prefix-attack (e.g.
	// CWD=/home/user/dir must not match /home/user/dir2/file).
	cwdAnchor := s.cwd
	if !strings.HasSuffix(cwdAnchor, string(filepath.Separator)) {
		cwdAnchor += string(filepath.Separator)
	}
	checkResolved := resolved
	if !strings.HasSuffix(checkResolved, string(filepath.Separator)) {
		checkResolved += string(filepath.Separator)
	}

	// Allow the CWD directory itself exactly, or anything inside it.
	inSandbox := checkResolved == cwdAnchor || strings.HasPrefix(checkResolved, cwdAnchor)

	// Also allow any path within the explicitly permitted prefixes (home dir,
	// /tmp, /sdcard, /storage, etc.).  Each prefix is checked with a trailing
	// separator to prevent prefix-confusion attacks (e.g. /tmp2 vs /tmp).
	if !inSandbox {
		for _, prefix := range s.allowedPrefixes {
			pfxAnchor := prefix
			if !strings.HasSuffix(pfxAnchor, string(filepath.Separator)) {
				pfxAnchor += string(filepath.Separator)
			}
			if checkResolved == pfxAnchor || strings.HasPrefix(checkResolved, pfxAnchor) {
				inSandbox = true
				break
			}
		}
	}

	if !inSandbox {
		s.counters.pathEscapeHits.Add(1)
		return SanitizerViolation{
			Field:  field,
			Value:  safePreview(path),
			Policy: PolicyPathSandbox,
			Detail: fmt.Sprintf("path %q resolves to %q which escapes CWD sandbox %q — rejected",
				path, resolved, s.cwd),
		}
	}
	return nil
}

// validateResourceName rejects resource names containing the three primary
// adversarial URI characters: ? (query injection), # (fragment injection),
// % (percent-encoding injection).
func (s *InputSanitizer) validateResourceName(field, name string) error {
	const adversarialChars = "?#%"
	for i, r := range name {
		if strings.ContainsRune(adversarialChars, r) {
			s.counters.resourceNameHits.Add(1)
			return SanitizerViolation{
				Field:  field,
				Value:  safePreview(name),
				Policy: PolicyResourceName,
				Detail: fmt.Sprintf(
					"adversarial URI character %q (0x%02X) at position %d — rejected",
					r, r, i,
				),
			}
		}
	}
	return nil
}

// Stats returns a snapshot of sanitization counters.  Safe for concurrent use.
func (s *InputSanitizer) Stats() SanitizerStats {
	return SanitizerStats{
		Accepted:         s.counters.accepted.Load(),
		Rejected:         s.counters.rejected.Load(),
		ControlCharHits:  s.counters.controlCharHits.Load(),
		PathEscapeHits:   s.counters.pathEscapeHits.Load(),
		ResourceNameHits: s.counters.resourceNameHits.Load(),
	}
}

// CWD returns the sandbox anchor directory.
func (s *InputSanitizer) CWD() string { return s.cwd }

// ── Classifier helpers ─────────────────────────────────────────────────────

// pathKeywords and nameKeywords are package-level to avoid a []string
// heap allocation on every isPathParam / isNameParam call.
var pathKeywords = []string{
	"path", "file", "dir", "directory", "dest", "destination",
	"src", "source", "target", "output", "input", "location",
	"filename", "filepath", "pathname",
}

var userInputKeywords = []string{
	"query", "prompt", "message", "input", "text", "content",
	"description", "title", "body", "markdown", "html", "json",
	"data", "value", "question",
}

var nameKeywords = []string{
	"name", "_id", "id", "tool_name", "resource", "handle",
	"slug", "identifier", "key", "label", "tag",
	"url", "uri", "link", "address",
}

// isPathParam returns true when the key conventionally contains a file path.
// Uses lowercase substring matching — no regex, no allocation.
func isPathParam(key string) bool {
	lower := strings.ToLower(key)
	for _, kw := range pathKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isNameParam returns true when the key conventionally holds a resource name
// or identifier that should be free of adversarial URI characters.
// User input fields (queries, messages, content, etc.) are EXCLUDED from this
// check since users naturally use ?, #, % in questions and text.
func isNameParam(key string) bool {
	lower := strings.ToLower(key)

	// Exclude user input fields from URI character validation.
	// These naturally contain punctuation like ?, #, % (e.g., "What is X?", "#hashtag", "50% off").
	for _, kw := range userInputKeywords {
		if lower == kw || strings.HasSuffix(lower, kw) {
			return false
		}
	}

	// Check if it's a resource/identifier name that should be strict.
	for _, kw := range nameKeywords {
		if lower == kw || strings.HasSuffix(lower, kw) {
			return true
		}
	}
	return false
}

// injectionPatterns is the package-level slice of prompt-injection keywords
// checked by ScanContextContent.  Defined at package scope to avoid a heap
// allocation on every call.
var injectionPatterns = []string{
	"ignore previous",
	"ignore all previous",
	"disregard previous",
	"forget previous",
	"you are now",
	"you must now",
	"new instructions:",
	"override:",
	"system prompt:",
	"curl ",
	"wget ",
	"$API_KEY",
	"$XAI_API_KEY",
	"$GEMINI_API_KEY",
	"ssh -R",
	"eval $(",
	"base64 -d",
	"| bash",
	"| sh",
}

// base64LineRe matches a standalone base64-encoded blob (40+ chars, optional padding).
// Compiled once at package init.
var base64LineRe = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)

// htmlEntityRe matches HTML hex-encoded character sequences (e.g. &#x69;).
var htmlEntityRe = regexp.MustCompile(`(?:&#x[0-9a-fA-F]+;)+`)

// ScanContextContent scans arbitrary content (GORKBOT.md, brain files, skill files)
// for prompt injection attempts. Unlike SanitizeParams (which blocks), this method
// uses "redact" mode: suspicious sections are replaced with [suspicious section redacted]
// markers rather than rejecting the entire content. A file that mentions "ignore whitespace"
// should still be usable.
//
// Detects:
//   - Known injection keywords (ignore previous instructions, disregard, you are now, etc.)
//   - Data exfiltration patterns (curl/wget/$API_KEY, ssh -R, eval $(base64 -d))
//   - Invisible Unicode characters (U+200B, U+202E, U+FEFF)
//   - Base64-encoded payloads (decode + re-scan)
//   - HTML entity encoded instructions (&#x69;&#x67;&#x6e;&#x6f;&#x72;&#x65; style)
//
// Parameters:
//   - content: the raw file content to scan
//   - source: human-readable name for logging (e.g. "GORKBOT.md", "brain/MEMORY.md")
//
// Returns:
//   - clean: content with suspicious sections redacted
//   - blocked: true if more than 25% of content was redacted (whole file is suspect)
//   - reason: human-readable explanation of what was found
func (s *InputSanitizer) ScanContextContent(content, source string) (clean string, blocked bool, reason string) {
	lines := strings.Split(content, "\n")
	redactedCount := 0
	var reasons []string

	for i, line := range lines {
		lower := strings.ToLower(line)

		// ── Pass 1: invisible Unicode characters ─────────────────────────────
		// Replace with visible markers in-place before any further checks.
		if strings.ContainsRune(line, '\u200B') ||
			strings.ContainsRune(line, '\u202E') ||
			strings.ContainsRune(line, '\uFEFF') {
			line = strings.ReplaceAll(line, "\u200B", "[ZERO-WIDTH-SPACE]")
			line = strings.ReplaceAll(line, "\u202E", "[RTL-OVERRIDE]")
			line = strings.ReplaceAll(line, "\uFEFF", "[BOM]")
			lines[i] = line
			lower = strings.ToLower(line)
		}

		suspicious := false
		var matchReason string

		// ── Pass 2: injection keyword check (strings.Contains, no alloc) ─────
		for _, pat := range injectionPatterns {
			if strings.Contains(lower, strings.ToLower(pat)) {
				suspicious = true
				matchReason = fmt.Sprintf("injection keyword %q", pat)
				break
			}
		}

		// ── Pass 3: base64 payload detection ─────────────────────────────────
		if !suspicious {
			if m := base64LineRe.FindString(line); m != "" {
				// Attempt to decode and re-scan the decoded text.
				decoded, err := base64.StdEncoding.DecodeString(m)
				if err != nil {
					// Try RawStdEncoding for unpadded blobs.
					decoded, err = base64.RawStdEncoding.DecodeString(m)
				}
				if err == nil {
					decodedLower := strings.ToLower(string(decoded))
					for _, pat := range injectionPatterns {
						if strings.Contains(decodedLower, strings.ToLower(pat)) {
							suspicious = true
							matchReason = fmt.Sprintf("base64-encoded injection keyword %q", pat)
							break
						}
					}
				}
			}
		}

		// ── Pass 4: HTML entity decoding ─────────────────────────────────────
		if !suspicious && htmlEntityRe.MatchString(line) {
			decoded := htmlEntityRe.ReplaceAllStringFunc(line, func(entities string) string {
				var sb strings.Builder
				// Split on ';' and process each &#xNN; fragment.
				parts := strings.Split(entities, ";")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if strings.HasPrefix(p, "&#x") || strings.HasPrefix(p, "&#X") {
						hex := p[3:]
						var r rune
						if n, err := fmt.Sscanf(hex, "%x", &r); n == 1 && err == nil {
							sb.WriteRune(r)
						}
					}
				}
				return sb.String()
			})
			decodedLower := strings.ToLower(decoded)
			for _, pat := range injectionPatterns {
				if strings.Contains(decodedLower, strings.ToLower(pat)) {
					suspicious = true
					matchReason = fmt.Sprintf("HTML-entity-encoded injection keyword %q in %s", pat, source)
					break
				}
			}
		}

		if suspicious {
			lines[i] = "[suspicious section redacted by SENSE]"
			redactedCount++
			if matchReason != "" {
				reasons = append(reasons, matchReason)
			}
		}
	}

	clean = strings.Join(lines, "\n")

	totalLines := len(lines)
	if totalLines > 0 && redactedCount*100/totalLines > 25 {
		blocked = true
	}

	if redactedCount > 0 {
		reason = fmt.Sprintf("source=%q redacted=%d/%d lines; matches: %s",
			source, redactedCount, totalLines,
			strings.Join(uniqueStrings(reasons), "; "))
	}

	return clean, blocked, reason
}

// uniqueStrings returns a deduplicated copy of ss preserving first-seen order.
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// safePreview returns up to 64 printable characters of s for error messages,
// replacing non-printable runes with their hex escapes to prevent log injection.
func safePreview(s string) string {
	var sb strings.Builder
	count := 0
	for _, r := range s {
		if count >= 64 {
			sb.WriteString("…")
			break
		}
		if r < 0x20 || !unicode.IsPrint(r) {
			sb.WriteString(fmt.Sprintf("\\x%02X", r))
		} else {
			sb.WriteRune(r)
		}
		count++
	}
	return sb.String()
}
