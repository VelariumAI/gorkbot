package engine

// xskill_adapter.go — Standalone helpers for XSKILL integration.
//
// This file contains:
//   - mutableProvider: hot-swappable embedder wrapper satisfying xskill.LLMProvider
//   - pickXSkillEmbedder: Ollama → Google → OpenAI cascade builder
//   - classifySkillDomain: heuristic prompt → skill domain name

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/embeddings"
)

// ──────────────────────────────────────────────────────────────────────────────
// mutableProvider — hot-swappable embedder wrapper
// ──────────────────────────────────────────────────────────────────────────────

// mutableProvider implements xskill.LLMProvider.  The AI provider is fixed at
// construction; the embedder can be replaced at any time via UpgradeEmbedder,
// allowing the local llama.cpp model to displace the cloud fallback once it
// finishes loading in the background initEmbedder goroutine.
type mutableProvider struct {
	aiProvider ai.AIProvider

	mu      sync.RWMutex
	embedder embeddings.Embedder // may be swapped after construction
}

// Generate calls the fixed AI provider using strict system/user role separation.
// Satisfies xskill.LLMProvider.
func (m *mutableProvider) Generate(systemPrompt, userPrompt string) (string, error) {
	history := ai.NewConversationHistory()
	if systemPrompt != "" {
		history.AddSystemMessage(systemPrompt)
	}
	if userPrompt != "" {
		history.AddUserMessage(userPrompt)
	}
	return m.aiProvider.GenerateWithHistory(context.Background(), history)
}

// Embed converts text to a float64 vector using the hot-swappable embedder.
// Acquires an RLock so concurrent Generate calls are never blocked.
// Satisfies xskill.LLMProvider.
func (m *mutableProvider) Embed(text string) ([]float64, error) {
	m.mu.RLock()
	emb := m.embedder
	m.mu.RUnlock()
	if emb == nil {
		return nil, fmt.Errorf("xskill: embedder not yet available")
	}
	f32, err := emb.Embed(context.Background(), text)
	if err != nil {
		return nil, err
	}
	// Widen float32 → float64 (xskill uses float64 for pure-Go cosine).
	f64 := make([]float64, len(f32))
	for i, v := range f32 {
		f64[i] = float64(v)
	}
	return f64, nil
}

// UpgradeEmbedder swaps in a new embedder under a write lock.
// Safe to call from any goroutine (e.g. the background initEmbedder routine).
func (m *mutableProvider) UpgradeEmbedder(e embeddings.Embedder) {
	m.mu.Lock()
	m.embedder = e
	m.mu.Unlock()
}

// ──────────────────────────────────────────────────────────────────────────────
// pickXSkillEmbedder — cascade builder
// ──────────────────────────────────────────────────────────────────────────────

// pickXSkillEmbedder constructs an embedder chain and wraps it in a
// FallbackEmbedder.  Priority: Ollama (local, no API key) → Google → OpenAI.
// Returns nil when the chain is empty; caller must disable XSKILL in that case.
func pickXSkillEmbedder(logger *slog.Logger) embeddings.Embedder {
	var chain []embeddings.Embedder

	// 1. Ollama — local, Termux-friendly, requires no API key.
	//    Uses default endpoint (http://localhost:11434) and model (nomic-embed-text).
	chain = append(chain, embeddings.NewOllamaEmbedder("", ""))

	// 2. Google / Gemini
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		chain = append(chain, embeddings.NewGoogleEmbedder(key))
	}

	// 3. OpenAI
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		chain = append(chain, embeddings.NewOpenAIEmbedder(key))
	}

	if len(chain) == 0 {
		if logger != nil {
			logger.Warn("XSKILL disabled: no embedder available (need Ollama, GEMINI_API_KEY, or OPENAI_API_KEY)")
		}
		return nil
	}
	return embeddings.NewFallbackEmbedder(chain...)
}

// ──────────────────────────────────────────────────────────────────────────────
// classifySkillDomain — heuristic prompt → domain name
// ──────────────────────────────────────────────────────────────────────────────

// classifySkillDomain returns a short, sanitized skill-domain name from the
// prompt's dominant topic.  Used to select the per-domain Markdown skill file.
func classifySkillDomain(prompt string) string {
	p := strings.ToLower(strings.TrimSpace(prompt))
	switch {
	case strings.Contains(p, "git") || strings.Contains(p, "commit") || strings.Contains(p, "branch") || strings.Contains(p, "diff") || strings.Contains(p, "merge"):
		return "git-ops"
	case strings.Contains(p, "search") || strings.Contains(p, "grep") || strings.Contains(p, "find") || strings.Contains(p, "locate"):
		return "search-tactics"
	case strings.Contains(p, "web") || strings.Contains(p, "http") || strings.Contains(p, "url") || strings.Contains(p, "fetch") || strings.Contains(p, "download"):
		return "web-ops"
	case strings.Contains(p, "security") || strings.Contains(p, "scan") || strings.Contains(p, "vuln") || strings.Contains(p, "pentest") || strings.Contains(p, "exploit"):
		return "security-ops"
	case strings.Contains(p, "code") || strings.Contains(p, "debug") || strings.Contains(p, "test") || strings.Contains(p, "build") || strings.Contains(p, "compile"):
		return "code-reasoning"
	case strings.Contains(p, "file") || strings.Contains(p, "read") || strings.Contains(p, "write") || strings.Contains(p, "directory") || strings.Contains(p, "path"):
		return "file-ops"
	default:
		return "general"
	}
}
