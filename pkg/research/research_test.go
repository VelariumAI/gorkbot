package research

import (
	"encoding/json"
	"testing"
	"time"
)

// ── DocBuffer Tests ──────────────────────────────────────────────────────────

func TestDocBufferStoreAndGet(t *testing.T) {
	buf := NewDocBuffer(5)

	doc := &Document{
		URL:     "https://example.com",
		Title:   "Example",
		Content: "Hello World",
		Length:  11,
	}

	buf.Store(doc)

	got, ok := buf.Get("https://example.com")
	if !ok {
		t.Fatal("expected document to be found")
	}
	if got.Title != "Example" {
		t.Errorf("got %q, want %q", got.Title, "Example")
	}
	if got.Content != "Hello World" {
		t.Errorf("got %q, want %q", got.Content, "Hello World")
	}
}

func TestDocBufferEviction(t *testing.T) {
	buf := NewDocBuffer(3)

	for i := 0; i < 5; i++ {
		buf.Store(&Document{
			URL:     "https://example.com/" + string(rune('a'+i)),
			Title:   "Page " + string(rune('A'+i)),
			Content: "content",
			Length:  7,
		})
	}

	if buf.Count() != 3 {
		t.Errorf("got count %d, want 3", buf.Count())
	}

	// First two should be evicted
	if _, ok := buf.Get("https://example.com/a"); ok {
		t.Error("evicted document should not be found")
	}
	if _, ok := buf.Get("https://example.com/b"); ok {
		t.Error("evicted document should not be found")
	}

	// Last three should exist
	if _, ok := buf.Get("https://example.com/c"); !ok {
		t.Error("document c should exist")
	}
	if _, ok := buf.Get("https://example.com/d"); !ok {
		t.Error("document d should exist")
	}
	if _, ok := buf.Get("https://example.com/e"); !ok {
		t.Error("document e should exist")
	}
}

func TestDocBufferActive(t *testing.T) {
	buf := NewDocBuffer(5)

	if buf.Active() != nil {
		t.Error("active should be nil initially")
	}

	buf.Store(&Document{URL: "https://a.com", Content: "aaa"})
	buf.Store(&Document{URL: "https://b.com", Content: "bbb"})

	if !buf.SetActive("https://b.com") {
		t.Error("SetActive should return true for existing doc")
	}

	active := buf.Active()
	if active == nil || active.URL != "https://b.com" {
		t.Errorf("got %v, want doc with URL https://b.com", active)
	}

	if buf.SetActive("https://nonexistent.com") {
		t.Error("SetActive should return false for non-existent doc")
	}
}

func TestDocBufferList(t *testing.T) {
	buf := NewDocBuffer(5)

	buf.Store(&Document{URL: "https://a.com", Title: "A", Length: 10})
	buf.Store(&Document{URL: "https://b.com", Title: "B", Length: 20})
	buf.SetActive("https://a.com")

	list := buf.List()
	if len(list) != 2 {
		t.Fatalf("got %d, want 2", len(list))
	}

	if !list[0].IsActive {
		t.Error("first doc should be active")
	}
	if list[1].IsActive {
		t.Error("second doc should not be active")
	}
}

func TestDocBufferClear(t *testing.T) {
	buf := NewDocBuffer(5)
	buf.Store(&Document{URL: "https://a.com"})
	buf.Store(&Document{URL: "https://b.com"})

	buf.Clear()
	if buf.Count() != 0 {
		t.Errorf("got count %d after clear, want 0", buf.Count())
	}
	if buf.Active() != nil {
		t.Error("active should be nil after clear")
	}
}

func TestDocBufferUpdateExisting(t *testing.T) {
	buf := NewDocBuffer(5)

	buf.Store(&Document{URL: "https://a.com", Title: "V1", Content: "old"})
	buf.Store(&Document{URL: "https://a.com", Title: "V2", Content: "new"})

	if buf.Count() != 1 {
		t.Errorf("got count %d, want 1 (should update in place)", buf.Count())
	}

	doc, _ := buf.Get("https://a.com")
	if doc.Title != "V2" {
		t.Errorf("got %q, want %q", doc.Title, "V2")
	}
}

func TestDocBufferDefaultCapacity(t *testing.T) {
	buf := NewDocBuffer(0)
	if buf.maxDocs != 10 {
		t.Errorf("got maxDocs %d, want 10 (default)", buf.maxDocs)
	}
}

// ── Engine Tests ─────────────────────────────────────────────────────────────

func TestEngineFindNoActive(t *testing.T) {
	e := NewEngine(5, nil)

	_, err := e.Find("test", 2)
	if err == nil {
		t.Error("expected error when no active document")
	}
}

func TestEngineFindPatternMatch(t *testing.T) {
	e := NewEngine(5, nil)

	// Manually store a document in the buffer
	doc := &Document{
		URL:     "https://test.com",
		Title:   "Test Page",
		Content: "line one\nline two has the KEYWORD here\nline three\nline four\nline five",
		Length:  68,
	}
	e.buffer.Store(doc)
	e.buffer.SetActive("https://test.com")

	matches, err := e.Find("KEYWORD", 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}

	if matches[0].LineNumber != 2 {
		t.Errorf("got line %d, want 2", matches[0].LineNumber)
	}
	if matches[0].MatchText != "KEYWORD" {
		t.Errorf("got %q, want %q", matches[0].MatchText, "KEYWORD")
	}
}

func TestEngineFindRegex(t *testing.T) {
	e := NewEngine(5, nil)

	doc := &Document{
		URL:     "https://test.com",
		Title:   "Test",
		Content: "foo bar123 baz\nqux 456 quux",
	}
	e.buffer.Store(doc)
	e.buffer.SetActive("https://test.com")

	matches, err := e.Find(`\d+`, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
}

func TestEngineFindMaxMatches(t *testing.T) {
	e := NewEngine(5, nil)

	// Create content with 30 matching lines
	content := ""
	for i := 0; i < 30; i++ {
		content += "match here\n"
	}

	doc := &Document{URL: "https://test.com", Content: content}
	e.buffer.Store(doc)
	e.buffer.SetActive("https://test.com")

	matches, err := e.Find("match", 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(matches) > 20 {
		t.Errorf("got %d matches, should be capped at 20", len(matches))
	}
}

func TestEngineListBuffered(t *testing.T) {
	e := NewEngine(5, nil)

	e.buffer.Store(&Document{URL: "https://a.com", Title: "A"})
	e.buffer.Store(&Document{URL: "https://b.com", Title: "B"})

	list := e.ListBuffered()
	if len(list) != 2 {
		t.Errorf("got %d, want 2", len(list))
	}
}

// ── Type Tests ───────────────────────────────────────────────────────────────

func TestDocumentContentNotSerialized(t *testing.T) {
	doc := Document{
		URL:       "https://test.com",
		Title:     "Test",
		Content:   "secret content",
		Length:    14,
		FetchedAt: time.Now(),
	}

	// The json:"-" tag should exclude Content
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if _, ok := decoded["content"]; ok {
		t.Error("Content should not be serialized (json:\"-\")")
	}
}

func TestSearchResultSerialization(t *testing.T) {
	sr := SearchResult{Title: "Test", URL: "https://test.com", Snippet: "A snippet", Rank: 1}
	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatal(err)
	}

	var decoded SearchResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.URL != sr.URL {
		t.Errorf("got %q, want %q", decoded.URL, sr.URL)
	}
}
