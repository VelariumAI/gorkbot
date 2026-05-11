package providers

import (
	"context"
	"os"
	"testing"
	"time"
)

type mockEmbedder struct {
	vec map[string][]float32
}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if v, ok := m.vec[text]; ok {
		return v, nil
	}
	return []float32{0, 0, 1}, nil
}

func (m *mockEmbedder) Dims() int { return 3 }
func (m *mockEmbedder) Name() string {
	return "mock"
}

func TestSemanticCacheStoreAndLookup(t *testing.T) {
	dir, err := os.MkdirTemp("", "gorkbot-cache-store-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	emb := &mockEmbedder{
		vec: map[string][]float32{
			"prompt-a": {1, 0, 0},
			"prompt-b": {0, 1, 0},
			"query-a":  {1, 0, 0},
		},
	}
	c, err := NewSemanticCache(emb, dir)
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}
	defer c.Close()

	c.StoreResponse(context.Background(), "prompt-a", "response-a")
	c.StoreResponse(context.Background(), "prompt-b", "response-b")
	// Allow async writer to flush at least once.
	time.Sleep(100 * time.Millisecond)

	resp, ok := c.GetCachedResponse(context.Background(), "query-a")
	if !ok {
		t.Fatalf("expected semantic cache hit")
	}
	if resp != "response-a" {
		t.Fatalf("unexpected cached response: %q", resp)
	}
}

func TestSemanticCachePruneOldEntries(t *testing.T) {
	dir, err := os.MkdirTemp("", "gorkbot-cache-prune-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	emb := &mockEmbedder{vec: map[string][]float32{"p": {1, 0, 0}}}
	c, err := NewSemanticCache(emb, dir)
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}
	defer c.Close()

	blob := float32sToBlob([]float32{1, 0, 0})
	old := time.Now().Add(-cacheTTL - time.Hour).Unix()
	if _, err := c.db.Exec(
		`INSERT INTO cache_entries (prompt, response, embedding, created_at) VALUES (?, ?, ?, ?)`,
		"old", "old-resp", blob, old,
	); err != nil {
		t.Fatalf("insert old row: %v", err)
	}
	if _, err := c.db.Exec(
		`INSERT INTO cache_entries (prompt, response, embedding, created_at) VALUES (?, ?, ?, ?)`,
		"new", "new-resp", blob, time.Now().Unix(),
	); err != nil {
		t.Fatalf("insert new row: %v", err)
	}

	c.prune()

	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM cache_entries WHERE prompt='old'`).Scan(&count); err != nil {
		t.Fatalf("count old row: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected old rows pruned, found %d", count)
	}
}
