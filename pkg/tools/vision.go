package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"os"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/vision"
)

// ─── vision_screen ────────────────────────────────────────────────────────────

// VisionScreenTool captures the screen and analyzes it with Grok Vision.
// Requires wireless ADB to be connected.
type VisionScreenTool struct{ BaseTool }

func NewVisionScreenTool() *VisionScreenTool {
	return &VisionScreenTool{BaseTool{
		name: "vision_screen",
		description: "Capture the device screen and analyze it with Grok Vision AI. " +
			"Returns a full description of what is on screen. " +
			"Requires wireless ADB: Settings → Developer Options → Wireless Debugging → Enable, " +
			"then run 'adb pair' + 'adb connect' once.",
		category:           CategoryCustom,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *VisionScreenTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "How to analyze the screen (e.g. 'describe everything on screen', 'what app is open')",
			},
		},
		"required": []string{"prompt"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *VisionScreenTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return &ToolResult{Success: false, Error: "prompt parameter required"}, fmt.Errorf("missing prompt")
	}
	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		return &ToolResult{Success: false, Error: "XAI_API_KEY not set"}, nil
	}
	result, err := vision.NewPipeline(apiKey).CaptureAndAnalyze(ctx, prompt)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	out := fmt.Sprintf("## Screen Analysis\n\n%s\n\n---\n_Capture: %s | %d bytes | %s_",
		result.Analysis, result.CaptureStrategy, result.ImageSize, result.Duration.Round(1e6))
	return &ToolResult{
		Success: true,
		Output:  out,
		Data: map[string]interface{}{
			"analysis":         result.Analysis,
			"capture_strategy": result.CaptureStrategy,
			"image_size":       result.ImageSize,
			"duration_ms":      result.Duration.Milliseconds(),
		},
	}, nil
}

// ─── vision_capture ───────────────────────────────────────────────────────────

// VisionCaptureOnlyTool captures the screen and saves to a file — no analysis.
type VisionCaptureOnlyTool struct{ BaseTool }

func NewVisionCaptureOnlyTool() *VisionCaptureOnlyTool {
	return &VisionCaptureOnlyTool{BaseTool{
		name:               "vision_capture",
		description:        "Capture the device screen and save to a file. Returns path and image dimensions. Requires wireless ADB.",
		category:           CategoryCustom,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *VisionCaptureOnlyTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Output file path (optional; defaults to a temp file)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *VisionCaptureOnlyTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := ""
	if p, ok := params["path"].(string); ok {
		path = p
	}
	p := vision.NewPipeline(os.Getenv("XAI_API_KEY"))
	savedPath, rawData, err := p.CaptureOnly(ctx, path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	width, height := 0, 0
	if img, _, decErr := image.Decode(bytes.NewReader(rawData)); decErr == nil {
		b := img.Bounds()
		width, height = b.Dx(), b.Dy()
	}
	out := fmt.Sprintf("Screen captured: %s (%dx%d, %d bytes)", savedPath, width, height, len(rawData))
	return &ToolResult{
		Success: true,
		Output:  out,
		Data:    map[string]interface{}{"path": savedPath, "width": width, "height": height, "bytes": len(rawData)},
	}, nil
}

// ─── vision_file ──────────────────────────────────────────────────────────────

// VisionFileTool analyzes an existing image file with Grok Vision.
type VisionFileTool struct{ BaseTool }

func NewVisionFileTool() *VisionFileTool {
	return &VisionFileTool{BaseTool{
		name:               "vision_file",
		description:        "Analyze an existing image file (PNG/JPEG) with Grok Vision AI.",
		category:           CategoryCustom,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *VisionFileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":   map[string]interface{}{"type": "string", "description": "Path to the image file"},
			"prompt": map[string]interface{}{"type": "string", "description": "What to look for or analyze"},
		},
		"required": []string{"path", "prompt"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *VisionFileTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path, _ := params["path"].(string)
	prompt, _ := params["prompt"].(string)
	if path == "" {
		return &ToolResult{Success: false, Error: "path required"}, nil
	}
	if prompt == "" {
		return &ToolResult{Success: false, Error: "prompt required"}, nil
	}
	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		return &ToolResult{Success: false, Error: "XAI_API_KEY not set"}, nil
	}
	result, err := vision.NewPipeline(apiKey).AnalyzeFile(ctx, path, prompt)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	out := fmt.Sprintf("## Image Analysis: %s\n\n%s\n\n---\n_%d bytes | %s_",
		path, result.Analysis, result.ImageSize, result.Duration.Round(1e6))
	return &ToolResult{
		Success: true,
		Output:  out,
		Data: map[string]interface{}{
			"analysis":    result.Analysis,
			"image_size":  result.ImageSize,
			"duration_ms": result.Duration.Milliseconds(),
		},
	}, nil
}

// ─── vision_ocr ───────────────────────────────────────────────────────────────

const ocrPrompt = `Extract ALL text visible in this image exactly as it appears.
Output only the extracted text, preserving layout where possible.
Include text from buttons, labels, notifications, status bar, and any UI elements.
If no text is visible, say "No text found."`

// VisionOCRTool extracts all visible text from the screen or an image file.
type VisionOCRTool struct{ BaseTool }

func NewVisionOCRTool() *VisionOCRTool {
	return &VisionOCRTool{BaseTool{
		name:               "vision_ocr",
		description:        "Extract all visible text from the screen or an image file using Grok Vision AI.",
		category:           CategoryCustom,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *VisionOCRTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Image file path (optional; if omitted, captures the screen)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *VisionOCRTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		return &ToolResult{Success: false, Error: "XAI_API_KEY not set"}, nil
	}
	p := vision.NewPipeline(apiKey)
	var result *vision.AnalysisResult
	var err error
	if path, ok := params["path"].(string); ok && path != "" {
		result, err = p.AnalyzeFile(ctx, path, ocrPrompt)
	} else {
		result, err = p.CaptureAndAnalyze(ctx, ocrPrompt)
	}
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	out := fmt.Sprintf("## OCR Result\n\n%s\n\n---\n_%s_", result.Analysis, result.Duration.Round(1e6))
	return &ToolResult{
		Success: true,
		Output:  out,
		Data:    map[string]interface{}{"text": result.Analysis, "duration_ms": result.Duration.Milliseconds()},
	}, nil
}

// ─── vision_find ──────────────────────────────────────────────────────────────

// VisionFindTool locates a specific UI element or text on screen.
type VisionFindTool struct{ BaseTool }

func NewVisionFindTool() *VisionFindTool {
	return &VisionFindTool{BaseTool{
		name:               "vision_find",
		description:        "Find a specific UI element, button, text, or object on the screen using Grok Vision AI.",
		category:           CategoryCustom,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *VisionFindTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "What to find on screen (e.g. 'send button', 'battery percentage', 'error message')",
			},
		},
		"required": []string{"target"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *VisionFindTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	target, _ := params["target"].(string)
	if target == "" {
		return &ToolResult{Success: false, Error: "target required"}, nil
	}
	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		return &ToolResult{Success: false, Error: "XAI_API_KEY not set"}, nil
	}
	findPrompt := fmt.Sprintf(`Look for the following on the screen: "%s"
Answer these questions:
1. Is it present? (yes/no)
2. If yes, where is it? (top/middle/bottom, left/center/right)
3. What does it look like? (color, size, state)
4. Confidence: high/medium/low
Be concise.`, target)
	result, err := vision.NewPipeline(apiKey).CaptureAndAnalyze(ctx, findPrompt)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	out := fmt.Sprintf("## Find: %s\n\n%s\n\n---\n_%s_", target, result.Analysis, result.Duration.Round(1e6))
	return &ToolResult{
		Success: true,
		Output:  out,
		Data:    map[string]interface{}{"target": target, "result": result.Analysis, "duration_ms": result.Duration.Milliseconds()},
	}, nil
}

// ─── vision_watch ─────────────────────────────────────────────────────────────

// VisionWatchTool captures multiple screenshots over time and analyzes changes.
type VisionWatchTool struct{ BaseTool }

func NewVisionWatchTool() *VisionWatchTool {
	return &VisionWatchTool{BaseTool{
		name:               "vision_watch",
		description:        "Watch the screen over time by capturing N screenshots at intervals and analyzing changes between them.",
		category:           CategoryCustom,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *VisionWatchTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt":           map[string]interface{}{"type": "string", "description": "What to watch for across frames"},
			"count":            map[string]interface{}{"type": "number", "description": "Screenshots to capture (default 3, max 10)"},
			"interval_seconds": map[string]interface{}{"type": "number", "description": "Seconds between captures (default 2)"},
		},
		"required": []string{"prompt"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *VisionWatchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return &ToolResult{Success: false, Error: "prompt required"}, nil
	}
	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		return &ToolResult{Success: false, Error: "XAI_API_KEY not set"}, nil
	}

	count := 3
	if c, ok := params["count"].(float64); ok && c > 0 {
		count = int(c)
		if count > 10 {
			count = 10
		}
	}
	intervalSec := 2.0
	if iv, ok := params["interval_seconds"].(float64); ok && iv >= 1 {
		intervalSec = iv
	}
	interval := time.Duration(intervalSec * float64(time.Second))
	start := time.Now()

	dataURIs := make([]string, 0, count)
	captureErrors := 0

	for i := 0; i < count; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return &ToolResult{Success: false, Error: "cancelled"}, nil
			case <-time.After(interval):
			}
		}
		cap, err := vision.CaptureScreen(ctx)
		if err != nil {
			captureErrors++
			continue
		}
		dataURI, err := vision.PrepareForAPI(cap.Data, 0)
		if err != nil {
			captureErrors++
			continue
		}
		dataURIs = append(dataURIs, dataURI)
	}

	var analysis string
	if len(dataURIs) > 0 {
		watchPrompt := fmt.Sprintf(
			"You are analyzing %d screenshots captured %.0fs apart.\n\n%s\n\n"+
				"Describe what changed between frames and answer the prompt for the overall sequence.",
			len(dataURIs), intervalSec, prompt)
		client := vision.NewVisionClient(apiKey, "")
		var err error
		analysis, err = client.AnalyzeMultiple(ctx, dataURIs, watchPrompt)
		if err != nil {
			return &ToolResult{Success: false, Error: "analysis: " + err.Error()}, nil
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Screen Watch (%d frames, %.0fs interval)\n\n", count, intervalSec))
	if captureErrors > 0 {
		sb.WriteString(fmt.Sprintf("⚠ %d/%d frames failed\n\n", captureErrors, count))
	}
	if analysis != "" {
		sb.WriteString(analysis)
	} else {
		sb.WriteString("No frames captured successfully.")
	}
	sb.WriteString(fmt.Sprintf("\n\n---\n_Total: %s_", time.Since(start).Round(1e6)))

	return &ToolResult{
		Success: len(dataURIs) > 0,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"frames_captured": len(dataURIs),
			"frames_failed":   captureErrors,
			"analysis":        analysis,
			"duration_ms":     time.Since(start).Milliseconds(),
		},
	}, nil
}
