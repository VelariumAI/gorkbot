package engine

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/embeddings"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/vectorstore"
)

type ragEmbedder struct{}

func (e *ragEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "query" {
		return []float32{1, 0, 0}, nil
	}
	return []float32{1, 0, 0}, nil
}
func (e *ragEmbedder) Dims() int    { return 3 }
func (e *ragEmbedder) Name() string { return "rag-embedder" }

func TestRAGInjector(t *testing.T) {
	if got := NewRAGInjector(nil, 128, slog.Default()); got != nil {
		t.Fatalf("expected nil RAG injector when store is nil")
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	if err := vectorstore.InitSchema(db); err != nil {
		t.Fatalf("init vector schema failed: %v", err)
	}

	var emb embeddings.Embedder = &ragEmbedder{}
	vs := vectorstore.New(db, emb)
	if err := vs.Init(nil); err != nil {
		t.Fatalf("vector store init failed: %v", err)
	}

	ctx := context.Background()
	vs.IndexTurn(ctx, "s1", "assistant", "query related answer")
	vs.IndexTurn(ctx, "s1", "user", "other text")

	ri := NewRAGInjector(vs, 100, slog.Default())
	h := ai.NewConversationHistory()
	ri.InjectContext(ctx, "query", h)

	msgs := h.GetMessages()
	if len(msgs) == 0 {
		t.Fatalf("expected injected RAG context message")
	}
	found := false
	for _, m := range msgs {
		if m.Role == "system" && m.Content != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected system message from RAG injection")
	}

	var nilRI *RAGInjector
	nilRI.InjectContext(ctx, "query", h) // should no-op safely
}

func TestCompressionPipe_SetCompressor(t *testing.T) {
	cp := NewCompressionPipe(sense.NewCompressor(&compressorGenStub{}), nil, "s", slog.Default())
	if cp == nil {
		t.Fatalf("expected non-nil compression pipe")
	}

	replacement := sense.NewCompressor(&compressorGenStub{out: "changed"})
	cp.SetCompressor(replacement)
	if cp.compressor != replacement {
		t.Fatalf("expected compressor replacement to be applied")
	}
}
