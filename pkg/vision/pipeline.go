package vision

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// AnalysisResult is the output of any vision pipeline operation.
type AnalysisResult struct {
	Analysis        string
	CaptureStrategy string        // which capture method was used (empty for file/URL inputs)
	ImageSize       int           // encoded byte count sent to API
	Duration        time.Duration
}

// Pipeline orchestrates screen capture, encoding, and vision API analysis.
type Pipeline struct {
	client *VisionClient
	maxDim int
}

// NewPipeline creates a Pipeline using the given xAI API key.
// maxDim controls the maximum image dimension before downscaling (0 = 1280px default).
func NewPipeline(apiKey string) *Pipeline {
	return &Pipeline{
		client: NewVisionClient(apiKey, ""),
		maxDim: defaultMaxDimension,
	}
}

// CaptureAndAnalyze is the full pipeline: capture screen → encode → analyze.
func (p *Pipeline) CaptureAndAnalyze(ctx context.Context, prompt string) (*AnalysisResult, error) {
	start := time.Now()

	cap, err := CaptureScreen(ctx)
	if err != nil {
		return nil, err
	}

	dataURI, err := PrepareForAPI(cap.Data, p.maxDim)
	if err != nil {
		return nil, fmt.Errorf("encode capture: %w", err)
	}

	analysis, err := p.client.Analyze(ctx, dataURI, prompt)
	if err != nil {
		return nil, fmt.Errorf("vision analysis: %w", err)
	}

	return &AnalysisResult{
		Analysis:        analysis,
		CaptureStrategy: cap.Strategy,
		ImageSize:       len(dataURI),
		Duration:        time.Since(start),
	}, nil
}

// AnalyzeFile analyzes an existing image file on disk.
func (p *Pipeline) AnalyzeFile(ctx context.Context, imagePath, prompt string) (*AnalysisResult, error) {
	start := time.Now()

	dataURI, err := LoadAndPrepare(imagePath, p.maxDim)
	if err != nil {
		return nil, err
	}

	analysis, err := p.client.Analyze(ctx, dataURI, prompt)
	if err != nil {
		return nil, fmt.Errorf("vision analysis: %w", err)
	}

	return &AnalysisResult{
		Analysis:        analysis,
		CaptureStrategy: "file:" + imagePath,
		ImageSize:       len(dataURI),
		Duration:        time.Since(start),
	}, nil
}

// AnalyzeURL downloads an image from an HTTP/HTTPS URL and analyzes it.
func (p *Pipeline) AnalyzeURL(ctx context.Context, imageURL, prompt string) (*AnalysisResult, error) {
	start := time.Now()

	if !strings.HasPrefix(imageURL, "http://") && !strings.HasPrefix(imageURL, "https://") {
		return nil, fmt.Errorf("URL must start with http:// or https://")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024)) // 20MB max
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	dataURI, err := PrepareForAPI(data, p.maxDim)
	if err != nil {
		return nil, fmt.Errorf("encode image: %w", err)
	}

	analysis, err := p.client.Analyze(ctx, dataURI, prompt)
	if err != nil {
		return nil, fmt.Errorf("vision analysis: %w", err)
	}

	return &AnalysisResult{
		Analysis:        analysis,
		CaptureStrategy: "url:" + imageURL,
		ImageSize:       len(dataURI),
		Duration:        time.Since(start),
	}, nil
}

// CaptureOnly captures the screen and saves it to path (or a temp file if
// path is empty). Returns the saved path and raw image bytes.
func (p *Pipeline) CaptureOnly(ctx context.Context, path string) (string, []byte, error) {
	cap, err := CaptureScreen(ctx)
	if err != nil {
		return "", nil, err
	}

	if path == "" {
		f, err := os.CreateTemp("", "gorkbot_cap_*.png")
		if err != nil {
			return "", nil, fmt.Errorf("create temp file: %w", err)
		}
		f.Close()
		path = f.Name()
	}

	if err := SaveCapture(cap.Data, path); err != nil {
		return "", nil, fmt.Errorf("save capture: %w", err)
	}

	return path, cap.Data, nil
}
