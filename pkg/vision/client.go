package vision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultVisionModel   = "grok-2-vision-1212"
	visionAPIURL         = "https://api.x.ai/v1/chat/completions"
	visionRequestTimeout = 60 * time.Second
)

// VisionClient sends image analysis requests to the Grok Vision API.
type VisionClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewVisionClient creates a client for the Grok Vision API.
// model may be empty to use the default (grok-2-vision-1212).
func NewVisionClient(apiKey, model string) *VisionClient {
	if model == "" {
		model = defaultVisionModel
	}
	return &VisionClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: visionRequestTimeout,
		},
	}
}

// --- request/response types ---

type visionImageURL struct {
	URL string `json:"url"`
}

type visionContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

type visionMessage struct {
	Role    string          `json:"role"`
	Content []visionContent `json:"content"`
}

type visionRequest struct {
	Model     string          `json:"model"`
	Messages  []visionMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}

type visionChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

type visionResponse struct {
	Choices []visionChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Analyze sends a single image (as a data URI) plus a text prompt to the
// Vision API and returns the model's analysis as a plain string.
func (c *VisionClient) Analyze(ctx context.Context, imageDataURI, prompt string) (string, error) {
	return c.AnalyzeMultiple(ctx, []string{imageDataURI}, prompt)
}

// AnalyzeMultiple sends multiple images in a single request.
func (c *VisionClient) AnalyzeMultiple(ctx context.Context, images []string, prompt string) (string, error) {
	contents := make([]visionContent, 0, len(images)+1)

	// Text prompt first
	contents = append(contents, visionContent{
		Type: "text",
		Text: prompt,
	})

	// Then all images
	for _, img := range images {
		contents = append(contents, visionContent{
			Type:     "image_url",
			ImageURL: &visionImageURL{URL: img},
		})
	}

	reqBody := visionRequest{
		Model: c.model,
		Messages: []visionMessage{
			{Role: "user", Content: contents},
		},
		MaxTokens: 4096,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, visionAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("vision API request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vision API HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var vResp visionResponse
	if err := json.Unmarshal(respBytes, &vResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if vResp.Error != nil {
		return "", fmt.Errorf("vision API error [%s]: %s", vResp.Error.Code, vResp.Error.Message)
	}

	if len(vResp.Choices) == 0 {
		return "", fmt.Errorf("vision API returned no choices")
	}

	return vResp.Choices[0].Message.Content, nil
}
