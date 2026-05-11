package providers

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSemanticCacheMkdirFailure(t *testing.T) {
	base := t.TempDir()
	bad := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(bad, []byte("x"), 0o600); err != nil {
		t.Fatalf("write marker file: %v", err)
	}
	if _, err := NewSemanticCache(&mockEmbedder{}, bad); err == nil {
		t.Fatalf("expected NewSemanticCache mkdir error")
	}
}

func TestSemanticCacheNilAndClosedBranches(t *testing.T) {
	var nilCache *SemanticCache
	if resp, ok := nilCache.GetCachedResponse(context.Background(), "p"); ok || resp != "" {
		t.Fatalf("nil cache should always miss")
	}
	// nil cache StoreResponse should be no-op
	nilCache.StoreResponse(context.Background(), "p", "r")

	c := &SemanticCache{embedder: nil, logger: slog.Default()}
	if resp, ok := c.GetCachedResponse(context.Background(), "p"); ok || resp != "" {
		t.Fatalf("nil embedder should always miss")
	}
	c.StoreResponse(context.Background(), "p", "r")
}

func TestSemanticCacheStoreDropOnFullChannel(t *testing.T) {
	c := &SemanticCache{
		embedder: &mockEmbedder{vec: map[string][]float32{"p": {1, 0, 0}}},
		logger:   slog.Default(),
		writeCh:  make(chan writeEntry, 1),
	}
	c.writeCh <- writeEntry{prompt: "existing", response: "existing", blob: []byte{1}}
	// Channel is full; this should take the default/drop branch without blocking.
	c.StoreResponse(context.Background(), "p", "r")
}
