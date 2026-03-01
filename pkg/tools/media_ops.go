package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// FfmpegProTool wraps ffmpeg for media conversion.
type FfmpegProTool struct {
	BaseTool
}

func NewFfmpegProTool() *FfmpegProTool {
	return &FfmpegProTool{
		BaseTool: BaseTool{
			name:               "ffmpeg_pro",
			description:        "Advanced video/audio manipulation using ffmpeg.",
			category:           CategoryMedia,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *FfmpegProTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{
				"type":        "string",
				"description": "Input file path.",
			},
			"output": map[string]interface{}{
				"type":        "string",
				"description": "Output file path.",
			},
			"args": map[string]interface{}{
				"type":        "string",
				"description": "Additional ffmpeg arguments (e.g., '-c:v libx264 -crf 23').",
			},
		},
		"required": []string{"input", "output"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *FfmpegProTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	input, _ := args["input"].(string)
	output, _ := args["output"].(string)
	ffmpegArgs, _ := args["args"].(string)

	cmdArgs := []string{"-i", input}
	if ffmpegArgs != "" {
		cmdArgs = append(cmdArgs, strings.Fields(ffmpegArgs)...)
	}
	cmdArgs = append(cmdArgs, output)

	cmd := exec.CommandContext(ctx, "ffmpeg", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("FFmpeg failed: %v\n%s", err, string(out))}, nil
	}
	return &ToolResult{Success: true, Output: "Conversion complete: " + output}, nil
}

// AudioTranscribeTool uses whisper.cpp or similar CLI.
type AudioTranscribeTool struct {
	BaseTool
}

func NewAudioTranscribeTool() *AudioTranscribeTool {
	return &AudioTranscribeTool{
		BaseTool: BaseTool{
			name:               "audio_transcribe",
			description:        "Transcribe audio to text using local Whisper CLI.",
			category:           CategoryAI,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *AudioTranscribeTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file": map[string]interface{}{
				"type":        "string",
				"description": "Audio file path.",
			},
			"model": map[string]interface{}{
				"type":        "string",
				"description": "Model path or size (tiny, base, small).",
			},
		},
		"required": []string{"file"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *AudioTranscribeTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	file, _ := args["file"].(string)
	model, _ := args["model"].(string)

	cmdArgs := []string{file}
	if model != "" {
		cmdArgs = append(cmdArgs, "-m", model)
	}

	cmd := exec.CommandContext(ctx, "whisper", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Transcribe failed: %v", err)}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// TtsGenerateTool generates speech.
type TtsGenerateTool struct {
	BaseTool
}

func NewTtsGenerateTool() *TtsGenerateTool {
	return &TtsGenerateTool{
		BaseTool: BaseTool{
			name:               "tts_generate",
			description:        "Generate speech from text using local TTS engine (piper or espeak).",
			category:           CategoryMedia,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *TtsGenerateTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Text to speak.",
			},
			"output": map[string]interface{}{
				"type":        "string",
				"description": "Output audio file (e.g., out.wav).",
			},
		},
		"required": []string{"text"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *TtsGenerateTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	text, _ := args["text"].(string)
	output, _ := args["output"].(string)

	if output == "" {
		output = "output.wav"
	}

	// Try piper first, fallback to espeak-ng -w
	cmd := exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("echo %s | piper --output_file %s", shellescape(text), output))
	if err := cmd.Run(); err != nil {
		// Fallback
		cmd = exec.CommandContext(ctx, "espeak-ng", "-w", output, text)
		if err := cmd.Run(); err != nil {
			return &ToolResult{Success: false, Error: "TTS failed (needs piper or espeak-ng)."}, nil
		}
	}
	return &ToolResult{Success: true, Output: "Generated " + output}, nil
}

// ImageOcrBatchTool runs OCR on directory.
type ImageOcrBatchTool struct {
	BaseTool
}

func NewImageOcrBatchTool() *ImageOcrBatchTool {
	return &ImageOcrBatchTool{
		BaseTool: BaseTool{
			name:               "image_ocr_batch",
			description:        "Run OCR on all images in a directory using tesseract.",
			category:           CategoryAI,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ImageOcrBatchTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"dir": map[string]interface{}{
				"type":        "string",
				"description": "Directory containing images.",
			},
			"lang": map[string]interface{}{
				"type":        "string",
				"description": "Language code (default: eng).",
			},
		},
		"required": []string{"dir"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ImageOcrBatchTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	dir, _ := args["dir"].(string)
	lang, _ := args["lang"].(string)
	if lang == "" {
		lang = "eng"
	}

	// Double backslash for escaping
	script := fmt.Sprintf("find %s -type f \\( -iname '*.jpg' -o -iname '*.png' \\) -exec tesseract {} {}.txt -l %s \\;", shellescape(dir), lang)
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	if err := cmd.Run(); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("OCR failed: %v", err)}, nil
	}
	return &ToolResult{Success: true, Output: "OCR complete. Text files created alongside images."}, nil
}

// VideoSummarizeTool extracts frames and audio for summary.
// This is a complex pipeline, for this tool we'll just extract keyframes and audio.
type VideoSummarizeTool struct {
	BaseTool
}

func NewVideoSummarizeTool() *VideoSummarizeTool {
	return &VideoSummarizeTool{
		BaseTool: BaseTool{
			name:               "video_summarize",
			description:        "Extract keyframes and audio from video for AI summary.",
			category:           CategoryMedia,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *VideoSummarizeTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{
				"type":        "string",
				"description": "Input video file.",
			},
			"interval": map[string]interface{}{
				"type":        "string",
				"description": "Keyframe interval (e.g., 10s).",
			},
		},
		"required": []string{"input"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *VideoSummarizeTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	input, _ := args["input"].(string)
	interval, _ := args["interval"].(string)
	if interval == "" {
		interval = "10" // seconds
	}

	// Extract audio
	cmdAudio := exec.CommandContext(ctx, "ffmpeg", "-i", input, "-vn", "-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1", input+".wav")
	cmdAudio.Run()

	// Extract frames
	cmdFrames := exec.CommandContext(ctx, "ffmpeg", "-i", input, "-vf", "fps=1/"+interval, input+"_frame_%03d.jpg")
	out, err := cmdFrames.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Frame extraction failed: %v\n%s", err, string(out))}, nil
	}
	return &ToolResult{Success: true, Output: "Extracted audio and frames to directory."}, nil
}

// MemeGeneratorTool overlays text on images.
type MemeGeneratorTool struct {
	BaseTool
}

func NewMemeGeneratorTool() *MemeGeneratorTool {
	return &MemeGeneratorTool{
		BaseTool: BaseTool{
			name:               "meme_generator",
			description:        "Add top/bottom text to an image (imagemagick).",
			category:           CategoryMedia,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *MemeGeneratorTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"image": map[string]interface{}{
				"type":        "string",
				"description": "Base image path.",
			},
			"top": map[string]interface{}{
				"type":        "string",
				"description": "Top text.",
			},
			"bottom": map[string]interface{}{
				"type":        "string",
				"description": "Bottom text.",
			},
			"output": map[string]interface{}{
				"type":        "string",
				"description": "Output filename.",
			},
		},
		"required": []string{"image", "output"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *MemeGeneratorTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	img, _ := args["image"].(string)
	top, _ := args["top"].(string)
	bot, _ := args["bottom"].(string)
	out, _ := args["output"].(string)

	// Requires imagemagick 'convert'
	cmd := exec.CommandContext(ctx, "convert", img, 
		"-font", "Impact", "-pointsize", "40", "-fill", "white", "-stroke", "black", "-strokewidth", "2", "-gravity", "North", "-annotate", "+0+10", top,
		"-gravity", "South", "-annotate", "+0+10", bot,
		out)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Meme gen failed: %v\n%s", err, string(output))}, nil
	}
	return &ToolResult{Success: true, Output: "Meme created: " + out}, nil
}
