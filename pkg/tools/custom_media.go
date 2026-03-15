package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type ImageResizeTool struct {
	BaseTool
}

func NewImageResizeTool() *ImageResizeTool {
	return &ImageResizeTool{
		BaseTool: BaseTool{
			name:               "image_resize",
			description:        "Resizes an image file.",
			category:           CategoryMedia,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ImageResizeTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input_path":  map[string]interface{}{"type": "string", "description": "Path to input image"},
			"output_path": map[string]interface{}{"type": "string", "description": "Path to output image"},
			"resolution":  map[string]interface{}{"type": "string", "description": "Target resolution (e.g. 800x600)"},
		},
		"required": []string{"input_path", "output_path", "resolution"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *ImageResizeTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	in, _ := params["input_path"].(string)
	res, _ := params["resolution"].(string)
	out, _ := params["output_path"].(string)

	cmd := fmt.Sprintf("ffmpeg -i %s -vf scale=%s %s", in, res, out)
	bash := NewBashTool()
	return bash.Execute(ctx, map[string]interface{}{"command": cmd})
}

type VideoConvertTool struct {
	BaseTool
}

func NewVideoConvertTool() *VideoConvertTool {
	return &VideoConvertTool{
		BaseTool: BaseTool{
			name:               "video_convert",
			description:        "Converts a video file format using ffmpeg.",
			category:           CategoryMedia,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *VideoConvertTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input_path":  map[string]interface{}{"type": "string", "description": "Path to input video"},
			"output_path": map[string]interface{}{"type": "string", "description": "Path to output video"},
		},
		"required": []string{"input_path", "output_path"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *VideoConvertTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	in, _ := params["input_path"].(string)
	out, _ := params["output_path"].(string)

	cmd := fmt.Sprintf("ffmpeg -i %s %s", in, out)
	bash := NewBashTool()
	return bash.Execute(ctx, map[string]interface{}{"command": cmd})
}
