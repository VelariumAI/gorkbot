package memory

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}

	// Create basic schema
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS facts (
		fact_id TEXT PRIMARY KEY,
		subject TEXT NOT NULL,
		predicate TEXT NOT NULL,
		object TEXT NOT NULL,
		confidence REAL DEFAULT 1.0,
		source TEXT,
		timestamp TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE VIRTUAL TABLE IF NOT EXISTS facts_fts USING fts5(
		fact_id UNINDEXED,
		subject,
		predicate,
		object,
		confidence UNINDEXED,
		source UNINDEXED,
		timestamp UNINDEXED
	);
	`)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("FTS5 not available in sqlite build")
		}
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestNewHybridSearcher_Degraded(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	lexical := NewFTS5LexicalSearcher(db, logger)
	facts := NewSQLiteFactSearcher(db, logger)

	config := SearchConfig{
		Logger:         logger,
		LexicalWeight:  1.0,
		FactWeight:     1.0,
		SemanticWeight: 1.5,
		TopK:           8,
		FusionK:        60,
	}

	hs := NewHybridSearcher(db, lexical, facts, config)

	// In Phase 2 MVP, should degrade due to missing semantic
	if !hs.isDegraded {
		t.Errorf("expected degraded mode in Phase 2 MVP")
	}

	if hs.memoryModeGauge == nil {
		t.Errorf("memoryModeGauge not initialized")
	}

	mode := hs.GetMode()
	if mode == "" {
		t.Errorf("GetMode returned empty string")
	}

	t.Logf("Mode: %s", mode)
}

func TestHybridSearcher_SearchDegraded(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	lexical := NewFTS5LexicalSearcher(db, logger)
	facts := NewSQLiteFactSearcher(db, logger)

	// Insert test facts
	testFacts := []struct {
		id, subject, pred, obj string
		conf                   float64
	}{
		{"f1", "alice", "prefers", "piña coladas", 0.95},
		{"f2", "alice", "hates", "YAML", 0.88},
		{"f3", "bob", "likes", "coffee", 0.92},
		{"f4", "alice", "prefers", "rain", 0.90},
	}

	for _, f := range testFacts {
		if err := facts.InsertFact(f.id, f.subject, f.pred, f.obj, f.conf, "test", "2026-03-24"); err != nil {
			t.Fatalf("failed to insert fact: %v", err)
		}
		if err := lexical.IndexFact(f.id, f.subject, f.pred, f.obj, f.conf, "test", "2026-03-24"); err != nil {
			t.Fatalf("failed to index fact: %v", err)
		}
	}

	config := SearchConfig{
		Logger:         logger,
		LexicalWeight:  1.0,
		FactWeight:     1.0,
		SemanticWeight: 1.5,
		TopK:           8,
		FusionK:        60,
	}

	hs := NewHybridSearcher(db, lexical, facts, config)

	ctx := context.Background()

	// Test 1: Search for "alice preferences"
	results, err := hs.Search(ctx, "alice preferences", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Errorf("expected results for alice preferences, got none")
	}

	t.Logf("Search for 'alice preferences': found %d results", len(results))
	for _, r := range results {
		t.Logf("  - %s | %s | %s (score: %.2f)", r.Subject, r.Predicate, r.Object, r.RelevanceScore)
	}

	// Test 2: Search for "YAML"
	results, err = hs.Search(ctx, "YAML", 5)
	if err != nil {
		t.Fatalf("search for YAML failed: %v", err)
	}

	if len(results) == 0 {
		t.Logf("WARNING: search for 'YAML' returned no results (may be FTS5 MATCH fallback)")
	} else {
		t.Logf("Search for 'YAML': found %d results", len(results))
		for _, r := range results {
			t.Logf("  - %s | %s | %s (score: %.2f)", r.Subject, r.Predicate, r.Object, r.RelevanceScore)
		}
	}

	// Test 3: Search for empty query (should return top facts)
	results, err = hs.Search(ctx, "", 3)
	if err != nil {
		t.Fatalf("search for empty query failed: %v", err)
	}

	t.Logf("Search for top-3 facts: found %d results", len(results))
	for _, r := range results {
		t.Logf("  - %s | %s | %s (confidence: %.2f)", r.Subject, r.Predicate, r.Object, r.Confidence)
	}
}

func TestHybridSearcher_GetCapabilities(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	lexical := NewFTS5LexicalSearcher(db, logger)
	facts := NewSQLiteFactSearcher(db, logger)

	config := SearchConfig{
		Logger: logger,
		TopK:   8,
	}

	hs := NewHybridSearcher(db, lexical, facts, config)

	caps := hs.GetCapabilities()

	if !caps["lexical"] {
		t.Errorf("expected lexical capability to be available")
	}

	if !caps["fact"] {
		t.Errorf("expected fact capability to be available")
	}

	if caps["degraded"] != hs.isDegraded {
		t.Errorf("degraded flag mismatch: %v vs %v", caps["degraded"], hs.isDegraded)
	}

	t.Logf("Capabilities: %v", caps)
}

func TestFTS5LexicalSearcher_IndexAndSearch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ls := NewFTS5LexicalSearcher(db, logger)

	// Index test facts
	facts := []struct {
		id, s, p, o string
		conf        float64
	}{
		{"f1", "alice", "prefers", "piña coladas", 0.95},
		{"f2", "alice", "hates", "YAML configs", 0.88},
		{"f3", "bob", "likes", "Go programming", 0.92},
	}

	for _, f := range facts {
		if err := ls.IndexFact(f.id, f.s, f.p, f.o, f.conf, "test", "2026-03-24"); err != nil {
			t.Fatalf("IndexFact failed: %v", err)
		}
	}

	ctx := context.Background()

	// Search
	results, err := ls.Search(ctx, "YAML", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	t.Logf("FTS5 search for 'YAML': found %d results", len(results))
	for _, r := range results {
		t.Logf("  - %s | %s | %s", r.Subject, r.Predicate, r.Object)
	}
}

func TestSQLiteFactSearcher_InsertAndSearch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	fs := NewSQLiteFactSearcher(db, logger)

	// Insert facts
	testFacts := []struct {
		id, s, p, o string
		conf        float64
	}{
		{"f1", "alice", "prefers", "piña coladas", 0.95},
		{"f2", "alice", "hates", "YAML", 0.88},
		{"f3", "bob", "likes", "coffee", 0.92},
		{"f4", "alice", "prefers", "rain", 0.90},
	}

	for _, f := range testFacts {
		if err := fs.InsertFact(f.id, f.s, f.p, f.o, f.conf, "test", "2026-03-24"); err != nil {
			t.Fatalf("InsertFact failed: %v", err)
		}
	}

	ctx := context.Background()

	// Test 1: Search by subject
	results, err := fs.Search(ctx, "alice", "", "", 10)
	if err != nil {
		t.Fatalf("Search by subject failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 facts for alice, got %d", len(results))
	}

	t.Logf("Facts for alice: %d", len(results))

	// Test 2: Search by predicate
	results, err = fs.Search(ctx, "", "prefers", "", 10)
	if err != nil {
		t.Fatalf("Search by predicate failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 facts for prefers, got %d", len(results))
	}

	t.Logf("Facts with predicate 'prefers': %d", len(results))

	// Test 3: Search by object
	results, err = fs.Search(ctx, "", "", "coffee", 10)
	if err != nil {
		t.Fatalf("Search by object failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 fact for coffee, got %d", len(results))
	}

	// Test 4: Find duplicates
	duplicates, err := fs.FindDuplicates(ctx, "alice", "prefers")
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	if len(duplicates) != 2 {
		t.Errorf("expected 2 duplicate entries, got %d", len(duplicates))
	}

	t.Logf("Duplicates for (alice, prefers): %d", len(duplicates))
}

func TestRRFFusion(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	lexical := NewFTS5LexicalSearcher(db, logger)
	facts := NewSQLiteFactSearcher(db, logger)

	// Insert test data
	for i := 1; i <= 5; i++ {
		subject := "user"
		predicate := "prefers"
		object := "item_" + string(rune('0'+i))
		if err := facts.InsertFact("f"+string(rune('0'+i)), subject, predicate, object, 0.90, "test", "2026-03-24"); err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
		if err := lexical.IndexFact("f"+string(rune('0'+i)), subject, predicate, object, 0.90, "test", "2026-03-24"); err != nil {
			t.Fatalf("failed to index: %v", err)
		}
	}

	config := SearchConfig{
		Logger:         logger,
		LexicalWeight:  1.0,
		FactWeight:     1.0,
		SemanticWeight: 1.5,
		TopK:           3,
		FusionK:        60,
	}

	hs := NewHybridSearcher(db, lexical, facts, config)

	// Test RRF fusion manually
	sources := map[string][]SearchResult{
		"lexical": {
			{FactID: "f1", Subject: "user", Predicate: "prefers", Object: "item_1", RelevanceScore: 1.0},
			{FactID: "f2", Subject: "user", Predicate: "prefers", Object: "item_2", RelevanceScore: 0.9},
			{FactID: "f3", Subject: "user", Predicate: "prefers", Object: "item_3", RelevanceScore: 0.8},
		},
		"fact": {
			{FactID: "f2", Subject: "user", Predicate: "prefers", Object: "item_2", RelevanceScore: 0.95},
			{FactID: "f1", Subject: "user", Predicate: "prefers", Object: "item_1", RelevanceScore: 0.85},
			{FactID: "f4", Subject: "user", Predicate: "prefers", Object: "item_4", RelevanceScore: 0.75},
		},
	}

	fused := hs.fuseRRF("alice prefs", sources, 3)

	if len(fused) == 0 {
		t.Errorf("RRF fusion returned no results")
	}

	t.Logf("RRF fusion result (top 3):")
	for i, r := range fused {
		t.Logf("  %d. %s (score: %.4f)", i+1, r.FactID, r.RelevanceScore)
	}

	// Verify f1 and f2 appear (they were in both sources)
	factIDs := make(map[string]bool)
	for _, r := range fused {
		factIDs[r.FactID] = true
	}

	if !factIDs["f1"] || !factIDs["f2"] {
		t.Errorf("expected f1 and f2 in fused results, got %v", factIDs)
	}
}

func TestHybridSearcher_SearchHybridSimpleEmbedder(t *testing.T) {
	t.Setenv("GORKBOT_MEMORY_ENABLE_SEMANTIC", "1")
	t.Setenv("GORKBOT_MEMORY_EMBEDDER", "simple")

	db := setupTestDB(t)
	defer db.Close()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	lexical := NewFTS5LexicalSearcher(db, logger)
	facts := NewSQLiteFactSearcher(db, logger)
	if err := facts.InsertFact("h1", "alice", "likes", "coffee", 0.9, "test", "2026-03-24"); err != nil {
		t.Fatalf("insert fact: %v", err)
	}
	if err := lexical.IndexFact("h1", "alice", "likes", "coffee", 0.9, "test", "2026-03-24"); err != nil {
		t.Fatalf("index fact: %v", err)
	}

	hs := NewHybridSearcher(db, lexical, facts, SearchConfig{Logger: logger})
	if hs.isDegraded {
		t.Fatalf("expected hybrid mode when semantic enabled")
	}
	results, err := hs.Search(context.Background(), "alice coffee", 3)
	if err != nil {
		t.Fatalf("hybrid search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}
}

func TestHybridSearcher_Close(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	lexical := NewFTS5LexicalSearcher(db, logger)
	facts := NewSQLiteFactSearcher(db, logger)

	config := SearchConfig{Logger: logger}

	hs := NewHybridSearcher(db, lexical, facts, config)

	// Should not panic
	if err := hs.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
