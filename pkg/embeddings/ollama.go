package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaEmbedder implements Embedder using a local Ollama server.
type OllamaEmbedder struct {
	BaseURL string // e.g. "http://localhost:11434"
	Model   string // e.g. "nomic-embed-text"
	client  *http.Client
}

// NewOllamaEmbedder creates a new OllamaEmbedder.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbedder{
		BaseURL: baseURL,
		Model:   model,
		client:  &http.Client{},
	}
}

func (e *OllamaEmbedder) Name() string {
	return "Ollama (" + e.Model + ")"
}

func (e *OllamaEmbedder) Dims() int {
	// nomic-embed-text returns 768 dimensions
	return 768
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float64 `json:"embedding"`
}

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model:  e.Model,
		Prompt: text,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", e.BaseURL+"/api/embeddings", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error (%d): %s", resp.StatusCode, string(b))
	}

	var res ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	out := make([]float32, len(res.Embedding))
	for i, v := range res.Embedding {
		out[i] = float32(v)
	}

	return L2Normalize(out), nil
}
