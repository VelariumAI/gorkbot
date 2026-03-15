package engine

import (
	"context"
	"log/slog"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/vectorstore"
)

// RAGInjector retrieves semantically similar past messages and injects them
// into ConversationHistory as a system message before the primary call.
type RAGInjector struct {
	store     *vectorstore.VectorStore
	maxTokens int
	logger    *slog.Logger
}

// NewRAGInjector creates a RAGInjector. maxTokens limits the injected block size.
func NewRAGInjector(store *vectorstore.VectorStore, maxTokens int, logger *slog.Logger) *RAGInjector {
	if store == nil {
		return nil
	}
	return &RAGInjector{store: store, maxTokens: maxTokens, logger: logger}
}

// InjectContext searches the vector store for content similar to prompt and
// injects a "rag-context" system message into history when results are found.
func (ri *RAGInjector) InjectContext(ctx context.Context, prompt string, history *ai.ConversationHistory) {
	if ri == nil {
		return
	}
	results, err := ri.store.Search(ctx, prompt, 5)
	if err != nil {
		ri.logger.Warn("RAG search failed", "err", err)
		return
	}
	block := vectorstore.FormatResults(results, ri.maxTokens)
	if block == "" {
		return
	}
	history.UpsertSystemMessage("rag-context", block)
	ri.logger.Info("RAG context injected", "results", len(results))
}
