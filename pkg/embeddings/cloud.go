package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ── OpenAI embeddings ─────────────────────────────────────────────────────────

const (
	openaiEmbedURL   = "https://api.openai.com/v1/embeddings"
	openaiEmbedModel = "text-embedding-3-small"
	openaiEmbedDims  = 1536

	googleEmbedURL  = "https://generativelanguage.googleapis.com/v1beta/models/text-embedding-004:embedContent"
	googleEmbedDims = 768
)

// OpenAIEmbedder calls text-embedding-3-small via the OpenAI API.
type OpenAIEmbedder struct {
	APIKey string
	client *http.Client
}

// NewOpenAIEmbedder creates an OpenAIEmbedder.
func NewOpenAIEmbedder(apiKey string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		APIKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *OpenAIEmbedder) Dims() int    { return openaiEmbedDims }
func (e *OpenAIEmbedder) Name() string { return "openai/text-embedding-3-small" }

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"input": text,
		"model": openaiEmbedModel,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", openaiEmbedURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embed %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Data) == 0 {
		return nil, fmt.Errorf("openai embed: malformed response")
	}
	return L2Normalize(result.Data[0].Embedding), nil
}

// ── Google embeddings ─────────────────────────────────────────────────────────

// GoogleEmbedder calls text-embedding-004 via the Google Generative Language API.
type GoogleEmbedder struct {
	APIKey string
	client *http.Client
}

// NewGoogleEmbedder creates a GoogleEmbedder.
func NewGoogleEmbedder(apiKey string) *GoogleEmbedder {
	return &GoogleEmbedder{
		APIKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *GoogleEmbedder) Dims() int    { return googleEmbedDims }
func (e *GoogleEmbedder) Name() string { return "google/text-embedding-004" }

func (e *GoogleEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"content": map[string]interface{}{
			"parts": []map[string]string{{"text": text}},
		},
	})
	url := googleEmbedURL + "?key=" + e.APIKey
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google embed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google embed %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Embedding.Values) == 0 {
		return nil, fmt.Errorf("google embed: malformed response")
	}
	return L2Normalize(result.Embedding.Values), nil
}

// ── Fallback chain ────────────────────────────────────────────────────────────

// FallbackEmbedder tries a list of embedders in order, returning the first
// successful result. Useful for local-first → cloud fallback patterns.
type FallbackEmbedder struct {
	chain []Embedder
}

// NewFallbackEmbedder builds a FallbackEmbedder from the provided chain.
// nil entries are silently skipped.
func NewFallbackEmbedder(chain ...Embedder) *FallbackEmbedder {
	var filtered []Embedder
	for _, e := range chain {
		if e != nil {
			filtered = append(filtered, e)
		}
	}
	return &FallbackEmbedder{chain: filtered}
}

func (f *FallbackEmbedder) Dims() int {
	if len(f.chain) == 0 {
		return 0
	}
	return f.chain[0].Dims()
}

func (f *FallbackEmbedder) Name() string {
	if len(f.chain) == 0 {
		return "none"
	}
	return f.chain[0].Name() + " (fallback)"
}

func (f *FallbackEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	var lastErr error
	for _, e := range f.chain {
		v, err := e.Embed(ctx, text)
		if err == nil {
			return v, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no embedders configured")
}
