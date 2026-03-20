package xskill

// provider.go — LLM provider interface and Gorkbot-native adapter.
//
// The LLMProvider interface is the ONLY dependency that KnowledgeBase and
// InferenceEngine have on any AI backend.  This makes the xskill package
// 100% provider-agnostic: swap Grok → Gemini → Anthropic → local llama.cpp
// by passing a different implementation.
//
// AIProviderAdapter is the standard bridge between this interface and the
// existing pkg/ai + pkg/embeddings infrastructure.  It strictly separates
// system and user roles using ai.ConversationHistory and GenerateWithHistory —
// no prompt concatenation is ever performed.

import (
	"context"
	"fmt"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/embeddings"
)

// ──────────────────────────────────────────────────────────────────────────────
// LLMProvider interface
// ──────────────────────────────────────────────────────────────────────────────

// LLMProvider is the single interface through which XSKILL interacts with any
// AI backend.  Dependency-inject a concrete implementation into KnowledgeBase
// and InferenceEngine at construction time.
//
// Implementations must be safe for concurrent use: KnowledgeBase.Accumulate
// runs in a background goroutine while InferenceEngine.PrepareContext may run
// on the request path.
type LLMProvider interface {
	// Generate sends a two-role (system + user) prompt to the LLM and returns
	// the complete text response.
	//
	// systemPrompt carries the standing instructions (the XSKILL prompt
	// template).  userPrompt carries the variable data (trajectory text, task
	// description, etc.).  Both strings may be non-empty; the underlying
	// implementation MUST keep them in separate message roles — concatenation
	// degrades instruction-following in modern LLMs and breaks role definitions.
	Generate(systemPrompt, userPrompt string) (string, error)

	// Embed returns a float64 embedding vector for the given text.
	// The vector is used for cosine-similarity retrieval in the Experience Bank.
	// Returned vectors must be non-nil and have consistent dimensionality for a
	// given provider instance.
	Embed(text string) ([]float64, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// AIProviderAdapter — bridges ai.AIProvider + embeddings.Embedder → LLMProvider
// ──────────────────────────────────────────────────────────────────────────────

// AIProviderAdapter adapts the existing Gorkbot ai.AIProvider and
// embeddings.Embedder types to the LLMProvider interface.
//
// Construction:
//
//	adapter := xskill.NewAIProviderAdapter(ctx, myAIProvider, myEmbedder)
//	kb, _ := xskill.NewKnowledgeBase("", adapter)
//
// The adapter is safe for concurrent use provided the underlying provider and
// embedder are also safe for concurrent use (all Gorkbot providers are).
type AIProviderAdapter struct {
	provider ai.AIProvider
	embedder embeddings.Embedder
	ctx      context.Context
}

// NewAIProviderAdapter creates an adapter that delegates LLM generation to
// provider and vector embedding to embedder.
//
// Pass context.Background() for long-running background operations, or the
// current request context for foreground inference calls so that cancellation
// is correctly propagated.
func NewAIProviderAdapter(ctx context.Context, provider ai.AIProvider, embedder embeddings.Embedder) *AIProviderAdapter {
	if ctx == nil {
		ctx = context.Background()
	}
	return &AIProviderAdapter{
		provider: provider,
		embedder: embedder,
		ctx:      ctx,
	}
}

// Generate implements LLMProvider.
//
// It builds an ai.ConversationHistory with systemPrompt in the "system" role
// and userPrompt in the "user" role, then calls provider.GenerateWithHistory.
// This strictly preserves the role boundary — no concatenation is ever done.
//
// If systemPrompt is empty the history contains only the user message; if
// userPrompt is empty the history contains only the system message.  At least
// one must be non-empty.
func (a *AIProviderAdapter) Generate(systemPrompt, userPrompt string) (string, error) {
	history := ai.NewConversationHistory()

	// Add system message first (must come before user in every major LLM API).
	if systemPrompt != "" {
		history.AddSystemMessage(systemPrompt)
	}

	// Add user message.
	if userPrompt != "" {
		history.AddUserMessage(userPrompt)
	}

	// GenerateWithHistory sends all messages in their correct roles to the
	// underlying provider (Grok, Gemini, Anthropic, etc.).
	result, err := a.provider.GenerateWithHistory(a.ctx, history)
	if err != nil {
		return "", fmt.Errorf("xskill: provider.GenerateWithHistory failed: %w", err)
	}
	return result, nil
}

// Embed implements LLMProvider.
//
// It calls the underlying embeddings.Embedder and performs a lossless widening
// conversion from float32 to float64 to match the Experience Bank's vector type.
//
// The float32→float64 widening is required because the existing Embedder
// interface returns []float32 (aligned with model APIs), while XSKILL's
// CosineSimilarity64 operates on []float64 for higher precision during
// similarity accumulation across large experience libraries.
func (a *AIProviderAdapter) Embed(text string) ([]float64, error) {
	f32, err := a.embedder.Embed(a.ctx, text)
	if err != nil {
		return nil, fmt.Errorf("xskill: embedder.Embed failed: %w", err)
	}
	if len(f32) == 0 {
		return nil, fmt.Errorf("xskill: embedder returned empty vector for text (len=%d)", len(text))
	}

	// Widen float32 → float64.  This is exact for all representable float32
	// values: float64 has a strictly larger mantissa (52 bits vs 23 bits).
	f64 := make([]float64, len(f32))
	for i, v := range f32 {
		f64[i] = float64(v)
	}
	return f64, nil
}
