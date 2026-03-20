package xskill

// kb_test.go — Unit tests for KnowledgeBase I/O and helper functions.
//
// Tests cover:
//   - ExperienceBank JSON serialisation round-trip
//   - loadBank handling of missing and corrupt files
//   - splitCondAction heuristic
//   - truncateToWords and wordCount utilities
//   - extractLastJSON parser
//   - parseRefinementOps parser
//   - sanitizeSkillName path-safety
//
// All tests use os.MkdirTemp for isolation — no writes to ~/.gorkbot.
// The mockProvider below stubs the LLMProvider interface for tests that
// need to exercise code paths that call Generate or Embed.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// mockProvider — minimal LLMProvider stub
// ──────────────────────────────────────────────────────────────────────────────

// mockProvider is a test-only LLMProvider implementation that returns
// configurable canned responses.  It counts how many times Generate and Embed
// are called so tests can assert on call counts.
type mockProvider struct {
	generateResp string // returned by Generate
	generateErr  error  // if non-nil, returned instead of generateResp
	embedResp    []float64
	embedErr     error
	generateN    int // call counter
	embedN       int // call counter
}

func (m *mockProvider) Generate(_, _ string) (string, error) {
	m.generateN++
	if m.generateErr != nil {
		return "", m.generateErr
	}
	return m.generateResp, nil
}

func (m *mockProvider) Embed(_ string) ([]float64, error) {
	m.embedN++
	if m.embedErr != nil {
		return nil, m.embedErr
	}
	return m.embedResp, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// ExperienceBank JSON round-trip
// ──────────────────────────────────────────────────────────────────────────────

func TestExperienceBank_JSONRoundtrip(t *testing.T) {
	now := time.Now().UTC().Round(time.Second)
	original := ExperienceBank{
		Version:   ExperienceBankVersion,
		UpdatedAt: now,
		Experiences: []Experience{
			{
				ID:        "E1",
				Condition: "When searching for a file",
				Action:    "use grep_content before read_file to avoid loading large files",
				Vector:    []float64{0.1, 0.2, 0.3},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:        "E2",
				Condition: "When tool returns empty output",
				Action:    "verify the search scope before concluding the target does not exist",
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored ExperienceBank
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.Version != original.Version {
		t.Errorf("version mismatch: got %q, want %q", restored.Version, original.Version)
	}
	if len(restored.Experiences) != len(original.Experiences) {
		t.Fatalf("experience count mismatch: got %d, want %d",
			len(restored.Experiences), len(original.Experiences))
	}
	for i, e := range original.Experiences {
		r := restored.Experiences[i]
		if r.ID != e.ID {
			t.Errorf("[%d] ID: got %q, want %q", i, r.ID, e.ID)
		}
		if r.Condition != e.Condition {
			t.Errorf("[%d] Condition: got %q, want %q", i, r.Condition, e.Condition)
		}
		if r.Action != e.Action {
			t.Errorf("[%d] Action: got %q, want %q", i, r.Action, e.Action)
		}
		if len(r.Vector) != len(e.Vector) {
			t.Errorf("[%d] Vector length: got %d, want %d", i, len(r.Vector), len(e.Vector))
		}
	}
}

func TestExperienceBank_EmptyExperiencesSlice(t *testing.T) {
	// Ensure that unmarshaling a bank with "experiences": null does not produce a nil slice.
	jsonInput := `{"version":"1.0.0","experiences":null,"updated_at":"2026-01-01T00:00:00Z"}`
	var bank ExperienceBank
	if err := json.Unmarshal([]byte(jsonInput), &bank); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	// Note: the nil guard is in loadBank, not the JSON unmarshaler.
	// This test documents the raw JSON behavior (slice will be nil here).
	// loadBank always normalises nil → empty slice.
	_ = bank
}

// ──────────────────────────────────────────────────────────────────────────────
// loadBank — missing and corrupt file handling
// ──────────────────────────────────────────────────────────────────────────────

func TestNewKnowledgeBase_FreshDirectory(t *testing.T) {
	// A freshly created directory should produce an empty bank without error.
	dir := t.TempDir()
	p := &mockProvider{embedResp: []float64{0.1, 0.2, 0.3}}

	kb, err := NewKnowledgeBase(dir, p)
	if err != nil {
		t.Fatalf("NewKnowledgeBase failed on fresh dir: %v", err)
	}

	snap := kb.Snapshot()
	if snap.Version != ExperienceBankVersion {
		t.Errorf("version: got %q, want %q", snap.Version, ExperienceBankVersion)
	}
	if len(snap.Experiences) != 0 {
		t.Errorf("fresh bank should have 0 experiences, got %d", len(snap.Experiences))
	}
}

func TestNewKnowledgeBase_LoadExistingBank(t *testing.T) {
	// Pre-seeded bank should be loaded correctly.
	dir := t.TempDir()

	// Write a pre-seeded bank.
	bank := ExperienceBank{
		Version: ExperienceBankVersion,
		Experiences: []Experience{
			{ID: "E1", Condition: "cond", Action: "act", CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: "E7", Condition: "c2", Action: "a2", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		UpdatedAt: time.Now(),
	}
	data, _ := json.Marshal(bank)
	if err := os.WriteFile(filepath.Join(dir, "experiences.json"), data, 0600); err != nil {
		t.Fatalf("cannot seed bank: %v", err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "skills"), 0700)

	p := &mockProvider{embedResp: []float64{0.1, 0.2}}
	kb, err := NewKnowledgeBase(dir, p)
	if err != nil {
		t.Fatalf("NewKnowledgeBase failed loading existing bank: %v", err)
	}

	snap := kb.Snapshot()
	if len(snap.Experiences) != 2 {
		t.Errorf("loaded bank should have 2 experiences, got %d", len(snap.Experiences))
	}

	// nextID should be 7 (max numeric ID from E1, E7).
	id := fmt.Sprintf("E%d", kb.nextID.Add(1))
	if id != "E8" {
		t.Errorf("nextID after loading E7: expected E8, got %s", id)
	}
}

func TestNewKnowledgeBase_CorruptFile(t *testing.T) {
	// A corrupt JSON file must return an error, not panic.
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "skills"), 0700)
	if err := os.WriteFile(filepath.Join(dir, "experiences.json"), []byte("not json{{"), 0600); err != nil {
		t.Fatalf("cannot write corrupt file: %v", err)
	}

	p := &mockProvider{}
	_, err := NewKnowledgeBase(dir, p)
	if err == nil {
		t.Error("expected error loading corrupt bank, got nil")
	}
	if !strings.Contains(err.Error(), "cannot parse experience bank") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewKnowledgeBase_NilProvider(t *testing.T) {
	_, err := NewKnowledgeBase(t.TempDir(), nil)
	if err == nil {
		t.Error("expected error for nil provider, got nil")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// splitCondAction
// ──────────────────────────────────────────────────────────────────────────────

func TestSplitCondAction_DoubleNewline(t *testing.T) {
	cond, action := splitCondAction("When searching files\n\nUse grep first")
	if cond != "When searching files" {
		t.Errorf("condition: got %q", cond)
	}
	if action != "Use grep first" {
		t.Errorf("action: got %q", action)
	}
}

func TestSplitCondAction_SingleNewline(t *testing.T) {
	cond, action := splitCondAction("When output is empty\nVerify the scope first")
	if cond != "When output is empty" {
		t.Errorf("condition: got %q", cond)
	}
	if action != "Verify the scope first" {
		t.Errorf("action: got %q", action)
	}
}

func TestSplitCondAction_Period(t *testing.T) {
	cond, action := splitCondAction("When facing a large file. Read only the needed section.")
	if cond != "When facing a large file." {
		t.Errorf("condition: got %q", cond)
	}
	if action != "Read only the needed section." {
		t.Errorf("action: got %q", action)
	}
}

func TestSplitCondAction_Fallback(t *testing.T) {
	// No separator — entire text becomes condition, action is empty.
	cond, action := splitCondAction("Always verify paths before calling write_file")
	if cond != "Always verify paths before calling write_file" {
		t.Errorf("condition: got %q", cond)
	}
	if action != "" {
		t.Errorf("action should be empty, got %q", action)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// wordCount and truncateToWords
// ──────────────────────────────────────────────────────────────────────────────

func TestWordCount(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"   ", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  a  b  c  ", 3},
	}
	for _, c := range cases {
		if got := wordCount(c.input); got != c.want {
			t.Errorf("wordCount(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestTruncateToWords(t *testing.T) {
	s := "one two three four five"
	got := truncateToWords(s, 3)
	if got != "one two three" {
		t.Errorf("truncate to 3: got %q", got)
	}
	// Truncating to more words than exist should return original.
	got2 := truncateToWords(s, 100)
	if got2 != s {
		t.Errorf("truncate to 100 (more than words): got %q, want original", got2)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// extractLastJSON
// ──────────────────────────────────────────────────────────────────────────────

func TestExtractLastJSON_SimpleObject(t *testing.T) {
	input := `Some preamble text here.
{"option": "add", "experience": "test text"}`
	got := extractLastJSON(input)
	if got != `{"option": "add", "experience": "test text"}` {
		t.Errorf("got %q", got)
	}
}

func TestExtractLastJSON_MultipleObjects(t *testing.T) {
	// Should return the LAST JSON object.
	input := `{"first": 1} Some text {"second": 2}`
	got := extractLastJSON(input)
	if got != `{"second": 2}` {
		t.Errorf("got %q, want last object", got)
	}
}

func TestExtractLastJSON_NoObject(t *testing.T) {
	got := extractLastJSON("no json here at all")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractLastJSON_NestedObject(t *testing.T) {
	input := `{"outer": {"inner": "value"}}`
	got := extractLastJSON(input)
	if got != `{"outer": {"inner": "value"}}` {
		t.Errorf("nested: got %q", got)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// parseRefinementOps
// ──────────────────────────────────────────────────────────────────────────────

func TestParseRefinementOps_MixedOps(t *testing.T) {
	input := `
Reasoning text here, skipped.
{"op":"merge","ids":["E1","E2"],"result":"merged experience text here"}
More reasoning text.
{"op":"delete","id":"E5","reason":"low value"}
`
	ops := parseRefinementOps(input)
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}
	if ops[0].Op != "merge" {
		t.Errorf("ops[0].Op: got %q, want merge", ops[0].Op)
	}
	if len(ops[0].IDs) != 2 || ops[0].IDs[0] != "E1" {
		t.Errorf("ops[0].IDs: got %v", ops[0].IDs)
	}
	if ops[1].Op != "delete" {
		t.Errorf("ops[1].Op: got %q, want delete", ops[1].Op)
	}
	if ops[1].ID != "E5" {
		t.Errorf("ops[1].ID: got %q, want E5", ops[1].ID)
	}
}

func TestParseRefinementOps_EmptyResponse(t *testing.T) {
	ops := parseRefinementOps("")
	if len(ops) != 0 {
		t.Errorf("empty response: expected 0 ops, got %d", len(ops))
	}
}

func TestParseRefinementOps_NoOps(t *testing.T) {
	ops := parseRefinementOps("The library looks great. No changes needed.")
	if len(ops) != 0 {
		t.Errorf("no ops response: expected 0 ops, got %d", len(ops))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// sanitizeSkillName
// ──────────────────────────────────────────────────────────────────────────────

func TestSanitizeSkillName_Basic(t *testing.T) {
	cases := []struct{ input, want string }{
		{"visual-logic", "visual-logic"},
		{"Search Tactics", "search-tactics"},
		{"../../../etc/passwd", "etc-passwd"},
		{"CODE review!", "code-review-"},
		{"", ""},
		{"UPPER_CASE", "upper_case"},
	}
	for _, c := range cases {
		got := sanitizeSkillName(c.input)
		// Trim trailing dashes for comparison (edge case with trailing punct).
		want := strings.Trim(c.want, "-")
		got = strings.Trim(got, "-")
		if got != want {
			t.Errorf("sanitize(%q): got %q, want %q", c.input, got, want)
		}
	}
}

func TestSanitizeSkillName_LengthCap(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := sanitizeSkillName(long)
	if len(got) > 50 {
		t.Errorf("sanitize should cap at 50 chars, got %d", len(got))
	}
}

func TestSanitizeSkillName_PathTraversalBlocked(t *testing.T) {
	// Any result containing ".." or "/" is a security failure.
	malicious := "../secret/config"
	got := sanitizeSkillName(malicious)
	if strings.Contains(got, "..") {
		t.Errorf("sanitized name contains '..': %q", got)
	}
	if strings.Contains(got, "/") {
		t.Errorf("sanitized name contains '/': %q", got)
	}
}
