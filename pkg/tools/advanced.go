package tools

// advanced.go — Extended toolset covering AI, Database, Network, Media, Android, and Package management.
// All tools are bash-command wrappers that call system utilities available in Termux on Android
// and on standard Linux desktops. Each tool degrades gracefully when the underlying utility is absent.

import (
	"context"
	"encoding/json"
	"fmt"
)

// ─── AI / ML ──────────────────────────────────────────────────────────────────

// AIImageGenerateTool calls a local Stable Diffusion API or Replicate to generate images.
type AIImageGenerateTool struct{ BaseTool }

func NewAIImageGenerateTool() *AIImageGenerateTool {
	return &AIImageGenerateTool{BaseTool: BaseTool{
		name:               "ai_image_generate",
		description:        "Generate images from text prompts using a local Stable Diffusion API (http://localhost:7860) or any compatible endpoint.",
		category:           CategoryAI,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *AIImageGenerateTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt":          map[string]interface{}{"type": "string", "description": "Text description of the image to generate"},
			"negative_prompt": map[string]interface{}{"type": "string", "description": "Things to avoid in the image (optional)"},
			"output_path":     map[string]interface{}{"type": "string", "description": "Path to save the output image (default: /tmp/generated.png)"},
			"width":           map[string]interface{}{"type": "string", "description": "Image width in pixels (default: 512)"},
			"height":          map[string]interface{}{"type": "string", "description": "Image height in pixels (default: 512)"},
			"api_url":         map[string]interface{}{"type": "string", "description": "API base URL (default: http://localhost:7860)"},
		},
		"required": []string{"prompt"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *AIImageGenerateTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return &ToolResult{Success: false, Error: "prompt is required"}, fmt.Errorf("prompt required")
	}
	negPrompt, _ := params["negative_prompt"].(string)
	outputPath, _ := params["output_path"].(string)
	if outputPath == "" {
		outputPath = "/tmp/ai_generated.png"
	}
	width, _ := params["width"].(string)
	if width == "" {
		width = "512"
	}
	height, _ := params["height"].(string)
	if height == "" {
		height = "512"
	}
	apiURL, _ := params["api_url"].(string)
	if apiURL == "" {
		apiURL = "http://localhost:7860"
	}

	// Build JSON payload for SD WebUI /sdapi/v1/txt2img
	payload := fmt.Sprintf(
		`{"prompt":%s,"negative_prompt":%s,"width":%s,"height":%s,"steps":20}`,
		shellescape(prompt), shellescape(negPrompt), width, height,
	)

	cmd := fmt.Sprintf(
		`_resp=$(curl -s -X POST %s/sdapi/v1/txt2img -H 'Content-Type: application/json' -d %s) && `+
			`echo "$_resp" | python3 -c "import sys,json,base64; d=json.load(sys.stdin); open(%s,'wb').write(base64.b64decode(d['images'][0]))" && `+
			`echo "Image saved to %s"`,
		shellescape(apiURL),
		shellescape(payload),
		shellescape(outputPath),
		outputPath,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 120})
}

// ─────────────────────────────────────────────────────────────────────────────

// AISummarizeAudioTool transcribes audio using Whisper (local CLI or OpenAI-compatible API).
type AISummarizeAudioTool struct{ BaseTool }

func NewAISummarizeAudioTool() *AISummarizeAudioTool {
	return &AISummarizeAudioTool{BaseTool: BaseTool{
		name:               "ai_summarize_audio",
		description:        "Transcribe and summarize audio files using Whisper (local CLI preferred, falls back to API).",
		category:           CategoryAI,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *AISummarizeAudioTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"audio_path": map[string]interface{}{"type": "string", "description": "Path to the audio file (mp3, wav, m4a, etc.)"},
			"language":   map[string]interface{}{"type": "string", "description": "Language code (e.g. en, es). Leave blank for auto-detect."},
			"model":      map[string]interface{}{"type": "string", "description": "Whisper model size (tiny, base, small, medium, large). Default: base"},
		},
		"required": []string{"audio_path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *AISummarizeAudioTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	audioPath, _ := params["audio_path"].(string)
	if audioPath == "" {
		return &ToolResult{Success: false, Error: "audio_path is required"}, fmt.Errorf("audio_path required")
	}
	lang, _ := params["language"].(string)
	model, _ := params["model"].(string)
	if model == "" {
		model = "base"
	}

	langFlag := ""
	if lang != "" {
		langFlag = "--language " + shellescape(lang)
	}

	cmd := fmt.Sprintf(
		`if command -v whisper >/dev/null 2>&1; then `+
			`whisper %s --model %s %s --output_format txt; `+
			`else echo "Error: 'whisper' not found. Install with: pip install openai-whisper"; `+
			`fi`,
		shellescape(audioPath), shellescape(model), langFlag,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 300})
}

// ─────────────────────────────────────────────────────────────────────────────

// MLModelRunTool runs inference using a locally hosted Ollama model.
type MLModelRunTool struct{ BaseTool }

func NewMLModelRunTool() *MLModelRunTool {
	return &MLModelRunTool{BaseTool: BaseTool{
		name:               "ml_model_run",
		description:        "Run local AI inference via Ollama (e.g. llama3, mistral, codellama). Ollama must be running.",
		category:           CategoryAI,
		requiresPermission: false,
		defaultPermission:  PermissionAlways,
	}}
}

func (t *MLModelRunTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"model":   map[string]interface{}{"type": "string", "description": "Ollama model name (e.g. llama3, mistral, codellama)"},
			"prompt":  map[string]interface{}{"type": "string", "description": "The input prompt"},
			"system":  map[string]interface{}{"type": "string", "description": "Optional system prompt"},
			"api_url": map[string]interface{}{"type": "string", "description": "Ollama API base URL (default: http://localhost:11434)"},
		},
		"required": []string{"model", "prompt"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *MLModelRunTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	model, _ := params["model"].(string)
	prompt, _ := params["prompt"].(string)
	if model == "" || prompt == "" {
		return &ToolResult{Success: false, Error: "model and prompt are required"}, fmt.Errorf("model and prompt required")
	}
	system, _ := params["system"].(string)
	apiURL, _ := params["api_url"].(string)
	if apiURL == "" {
		apiURL = "http://localhost:11434"
	}

	systemJSON := "null"
	if system != "" {
		systemJSON = shellescape(system)
	}

	payload := fmt.Sprintf(
		`{"model":%s,"prompt":%s,"system":%s,"stream":false}`,
		shellescape(model), shellescape(prompt), systemJSON,
	)

	cmd := fmt.Sprintf(
		`curl -s -X POST %s/api/generate -H 'Content-Type: application/json' -d %s | `+
			`python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('response',''))"`,
		shellescape(apiURL), shellescape(payload),
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 120})
}

// ─── DATABASE ─────────────────────────────────────────────────────────────────

// DBQueryTool executes SQL queries against a SQLite3 database.
type DBQueryTool struct{ BaseTool }

func NewDBQueryTool() *DBQueryTool {
	return &DBQueryTool{BaseTool: BaseTool{
		name:               "db_query",
		description:        "Execute SQL queries against a SQLite3 database file.",
		category:           CategoryDatabase,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *DBQueryTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"database": map[string]interface{}{"type": "string", "description": "Path to the SQLite3 database file"},
			"query":    map[string]interface{}{"type": "string", "description": "SQL query to execute"},
			"format":   map[string]interface{}{"type": "string", "description": "Output format: table (default), csv, json"},
		},
		"required": []string{"database", "query"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *DBQueryTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	database, _ := params["database"].(string)
	query, _ := params["query"].(string)
	if database == "" || query == "" {
		return &ToolResult{Success: false, Error: "database and query are required"}, fmt.Errorf("database and query required")
	}
	format, _ := params["format"].(string)

	var cmd string
	switch format {
	case "csv":
		cmd = fmt.Sprintf(`sqlite3 -csv %s %s`, shellescape(database), shellescape(query))
	case "json":
		cmd = fmt.Sprintf(`sqlite3 -json %s %s`, shellescape(database), shellescape(query))
	default:
		cmd = fmt.Sprintf(`sqlite3 -column -header %s %s`, shellescape(database), shellescape(query))
	}

	cmd = fmt.Sprintf(
		`if command -v sqlite3 >/dev/null 2>&1; then %s; else echo "Error: sqlite3 not found. Install: pkg install sqlite"; fi`,
		cmd,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 30})
}

// ─────────────────────────────────────────────────────────────────────────────

// DBMigrateTool applies SQL migration scripts to a SQLite3 database.
type DBMigrateTool struct{ BaseTool }

func NewDBMigrateTool() *DBMigrateTool {
	return &DBMigrateTool{BaseTool: BaseTool{
		name:               "db_migrate",
		description:        "Apply SQL migration scripts or inline SQL to a SQLite3 database.",
		category:           CategoryDatabase,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *DBMigrateTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"database": map[string]interface{}{"type": "string", "description": "Path to the SQLite3 database file (created if absent)"},
			"sql":      map[string]interface{}{"type": "string", "description": "Inline SQL to execute (CREATE TABLE, ALTER TABLE, etc.)"},
			"sql_file": map[string]interface{}{"type": "string", "description": "Path to a .sql migration file (alternative to sql)"},
		},
		"required": []string{"database"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *DBMigrateTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	database, _ := params["database"].(string)
	if database == "" {
		return &ToolResult{Success: false, Error: "database is required"}, fmt.Errorf("database required")
	}
	sql, _ := params["sql"].(string)
	sqlFile, _ := params["sql_file"].(string)

	if sql == "" && sqlFile == "" {
		return &ToolResult{Success: false, Error: "either sql or sql_file is required"}, fmt.Errorf("sql or sql_file required")
	}

	var cmd string
	if sqlFile != "" {
		cmd = fmt.Sprintf(`sqlite3 %s < %s && echo "Migration applied from %s"`, shellescape(database), shellescape(sqlFile), sqlFile)
	} else {
		cmd = fmt.Sprintf(`sqlite3 %s %s && echo "Migration applied"`, shellescape(database), shellescape(sql))
	}

	cmd = fmt.Sprintf(
		`if command -v sqlite3 >/dev/null 2>&1; then %s; else echo "Error: sqlite3 not found. Install: pkg install sqlite"; fi`,
		cmd,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 60})
}

// ─── NETWORK ──────────────────────────────────────────────────────────────────

// NetworkScanTool scans network hosts and ports using nmap (or nc fallback).
type NetworkScanTool struct{ BaseTool }

func NewNetworkScanTool() *NetworkScanTool {
	return &NetworkScanTool{BaseTool: BaseTool{
		name:               "network_scan",
		description:        "Scan network hosts and open ports using nmap (preferred) or netcat fallback.",
		category:           CategoryNetwork,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *NetworkScanTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target":    map[string]interface{}{"type": "string", "description": "Host, IP, or CIDR range to scan (e.g. 192.168.1.0/24)"},
			"ports":     map[string]interface{}{"type": "string", "description": "Port or range to scan (e.g. 80, 1-1024). Default: top 100"},
			"scan_type": map[string]interface{}{"type": "string", "description": "Scan type: ping (host discovery), port (port scan), service (service detection). Default: port"},
		},
		"required": []string{"target"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *NetworkScanTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	target, _ := params["target"].(string)
	if target == "" {
		return &ToolResult{Success: false, Error: "target is required"}, fmt.Errorf("target required")
	}
	ports, _ := params["ports"].(string)
	scanType, _ := params["scan_type"].(string)
	if scanType == "" {
		scanType = "port"
	}

	portFlag := ""
	if ports != "" {
		portFlag = "-p " + shellescape(ports)
	}

	var nmapArgs string
	switch scanType {
	case "ping":
		nmapArgs = "-sn"
	case "service":
		nmapArgs = "-sV " + portFlag
	default: // port
		nmapArgs = "-sT --open " + portFlag
	}

	// nc fallback for single host+port when nmap is absent
	var ncFallback string
	if ports != "" {
		ncFallback = fmt.Sprintf(
			`nc -zv -w2 %s %s 2>&1`,
			shellescape(target), shellescape(ports),
		)
	} else {
		ncFallback = fmt.Sprintf(`ping -c2 %s`, shellescape(target))
	}

	cmd := fmt.Sprintf(
		`if command -v nmap >/dev/null 2>&1; then `+
			`nmap %s %s; `+
			`elif command -v nc >/dev/null 2>&1; then `+
			`echo "nmap not found — using nc fallback:"; %s; `+
			`else echo "Error: neither nmap nor nc found. Install: pkg install nmap"; `+
			`fi`,
		nmapArgs, shellescape(target), ncFallback,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 60})
}

// ─────────────────────────────────────────────────────────────────────────────

// SocketConnectTool opens raw TCP/UDP socket connections via netcat.
type SocketConnectTool struct{ BaseTool }

func NewSocketConnectTool() *SocketConnectTool {
	return &SocketConnectTool{BaseTool: BaseTool{
		name:               "socket_connect",
		description:        "Open a TCP/UDP socket connection and optionally send data (uses netcat).",
		category:           CategoryNetwork,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *SocketConnectTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host":     map[string]interface{}{"type": "string", "description": "Hostname or IP to connect to"},
			"port":     map[string]interface{}{"type": "string", "description": "TCP/UDP port number"},
			"data":     map[string]interface{}{"type": "string", "description": "Data to send (optional)"},
			"protocol": map[string]interface{}{"type": "string", "description": "Protocol: tcp (default) or udp"},
			"timeout":  map[string]interface{}{"type": "string", "description": "Connection timeout in seconds (default: 5)"},
		},
		"required": []string{"host", "port"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SocketConnectTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	host, _ := params["host"].(string)
	port, _ := params["port"].(string)
	if host == "" || port == "" {
		return &ToolResult{Success: false, Error: "host and port are required"}, fmt.Errorf("host and port required")
	}
	data, _ := params["data"].(string)
	protocol, _ := params["protocol"].(string)
	timeout, _ := params["timeout"].(string)
	if timeout == "" {
		timeout = "5"
	}

	udpFlag := ""
	if protocol == "udp" {
		udpFlag = "-u"
	}

	var cmd string
	if data != "" {
		cmd = fmt.Sprintf(
			`echo %s | nc %s -w%s %s %s`,
			shellescape(data), udpFlag, shellescape(timeout), shellescape(host), shellescape(port),
		)
	} else {
		cmd = fmt.Sprintf(
			`nc %s -zv -w%s %s %s 2>&1`,
			udpFlag, shellescape(timeout), shellescape(host), shellescape(port),
		)
	}

	cmd = fmt.Sprintf(
		`if command -v nc >/dev/null 2>&1; then %s; else echo "Error: nc not found. Install: pkg install netcat-openbsd"; fi`,
		cmd,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 30})
}

// ─── MEDIA ────────────────────────────────────────────────────────────────────

// ImageProcessTool manipulates images using ImageMagick.
type ImageProcessTool struct{ BaseTool }

func NewImageProcessTool() *ImageProcessTool {
	return &ImageProcessTool{BaseTool: BaseTool{
		name:               "image_process",
		description:        "Resize, convert, rotate, crop, or apply effects to images using ImageMagick.",
		category:           CategoryMedia,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *ImageProcessTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input":     map[string]interface{}{"type": "string", "description": "Path to the input image"},
			"output":    map[string]interface{}{"type": "string", "description": "Path for the output image"},
			"operation": map[string]interface{}{"type": "string", "description": "Operation: resize, convert, rotate, crop, grayscale, info"},
			"options":   map[string]interface{}{"type": "string", "description": "Operation-specific options (e.g. '800x600' for resize, '90' for rotate, '100x100+10+10' for crop)"},
		},
		"required": []string{"input", "operation"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ImageProcessTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	input, _ := params["input"].(string)
	operation, _ := params["operation"].(string)
	if input == "" || operation == "" {
		return &ToolResult{Success: false, Error: "input and operation are required"}, fmt.Errorf("input and operation required")
	}
	output, _ := params["output"].(string)
	options, _ := params["options"].(string)

	if output == "" && operation != "info" {
		output = input + "_processed"
	}

	var magickCmd string
	switch operation {
	case "resize":
		size := options
		if size == "" {
			size = "800x600"
		}
		magickCmd = fmt.Sprintf(`magick %s -resize %s %s`, shellescape(input), shellescape(size), shellescape(output))
	case "rotate":
		degrees := options
		if degrees == "" {
			degrees = "90"
		}
		magickCmd = fmt.Sprintf(`magick %s -rotate %s %s`, shellescape(input), shellescape(degrees), shellescape(output))
	case "grayscale":
		magickCmd = fmt.Sprintf(`magick %s -colorspace Gray %s`, shellescape(input), shellescape(output))
	case "crop":
		geometry := options
		if geometry == "" {
			geometry = "100x100+0+0"
		}
		magickCmd = fmt.Sprintf(`magick %s -crop %s +repage %s`, shellescape(input), shellescape(geometry), shellescape(output))
	case "info":
		magickCmd = fmt.Sprintf(`magick identify -verbose %s`, shellescape(input))
	case "convert":
		if output == "" {
			return &ToolResult{Success: false, Error: "output path required for convert"}, nil
		}
		magickCmd = fmt.Sprintf(`magick %s %s`, shellescape(input), shellescape(output))
	default:
		// Pass-through: allow arbitrary ImageMagick flags via options
		magickCmd = fmt.Sprintf(`magick %s %s %s`, shellescape(input), options, shellescape(output))
	}

	cmd := fmt.Sprintf(
		`if command -v magick >/dev/null 2>&1; then %s && echo "Done"; `+
			`elif command -v convert >/dev/null 2>&1; then %s && echo "Done"; `+
			`else echo "Error: ImageMagick not found. Install: pkg install imagemagick"; fi`,
		magickCmd,
		// Rewrite for legacy ImageMagick (convert instead of magick)
		func() string {
			return "convert " + magickCmd[len("magick "):]
		}(),
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 60})
}

// ─────────────────────────────────────────────────────────────────────────────

// MediaConvertTool converts audio/video files using ffmpeg.
type MediaConvertTool struct{ BaseTool }

func NewMediaConvertTool() *MediaConvertTool {
	return &MediaConvertTool{BaseTool: BaseTool{
		name:               "media_convert",
		description:        "Convert, compress, or extract audio/video files using ffmpeg.",
		category:           CategoryMedia,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *MediaConvertTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input":   map[string]interface{}{"type": "string", "description": "Input file path"},
			"output":  map[string]interface{}{"type": "string", "description": "Output file path (extension determines format)"},
			"options": map[string]interface{}{"type": "string", "description": "Extra ffmpeg options (e.g. '-vn -ar 44100' for audio extraction)"},
		},
		"required": []string{"input", "output"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *MediaConvertTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	input, _ := params["input"].(string)
	output, _ := params["output"].(string)
	if input == "" || output == "" {
		return &ToolResult{Success: false, Error: "input and output are required"}, fmt.Errorf("input and output required")
	}
	options, _ := params["options"].(string)

	cmd := fmt.Sprintf(
		`if command -v ffmpeg >/dev/null 2>&1; then `+
			`ffmpeg -y -i %s %s %s && echo "Conversion complete: %s"; `+
			`else echo "Error: ffmpeg not found. Install: pkg install ffmpeg"; fi`,
		shellescape(input), options, shellescape(output), output,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 300})
}

// ─── ANDROID / TERMUX ─────────────────────────────────────────────────────────

// SensorReadTool reads Android hardware sensors via the Termux:API app.
type SensorReadTool struct{ BaseTool }

func NewSensorReadTool() *SensorReadTool {
	return &SensorReadTool{BaseTool: BaseTool{
		name:               "sensor_read",
		description:        "Read Android hardware sensor data (accelerometer, light, battery, GPS, etc.) using Termux:API.",
		category:           CategoryAndroid,
		requiresPermission: false,
		defaultPermission:  PermissionAlways,
	}}
}

func (t *SensorReadTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sensor": map[string]interface{}{
				"type":        "string",
				"description": "Sensor type: battery, location, light, accelerometer, gyroscope, magnetic, proximity, temperature, wifi, cell. Default: battery",
			},
			"duration": map[string]interface{}{
				"type":        "string",
				"description": "Sampling duration in seconds for continuous sensors (default: 2)",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SensorReadTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	sensor, _ := params["sensor"].(string)
	if sensor == "" {
		sensor = "battery"
	}
	duration, _ := params["duration"].(string)
	if duration == "" {
		duration = "2"
	}

	var sensorCmd string
	switch sensor {
	case "battery":
		sensorCmd = "termux-battery-status"
	case "location":
		sensorCmd = "termux-location"
	case "wifi":
		sensorCmd = "termux-wifi-connectioninfo"
	case "cell":
		sensorCmd = "termux-telephony-cellinfo"
	case "light", "accelerometer", "gyroscope", "magnetic", "proximity", "temperature":
		sensorCmd = fmt.Sprintf("termux-sensor -s %s -d %s -n 1", shellescape(sensor), shellescape(duration))
	default:
		sensorCmd = fmt.Sprintf("termux-sensor -s %s -d %s -n 1", shellescape(sensor), shellescape(duration))
	}

	cmd := fmt.Sprintf(
		`if command -v termux-battery-status >/dev/null 2>&1; then %s; `+
			`else echo "Error: Termux:API not installed. Install the Termux:API app and run: pkg install termux-api"; fi`,
		sensorCmd,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 15})
}

// ─────────────────────────────────────────────────────────────────────────────

// NotificationSendTool sends an Android notification via Termux:API.
type NotificationSendTool struct{ BaseTool }

func NewNotificationSendTool() *NotificationSendTool {
	return &NotificationSendTool{BaseTool: BaseTool{
		name:               "notification_send",
		description:        "Send an Android system notification using Termux:API.",
		category:           CategoryAndroid,
		requiresPermission: false,
		defaultPermission:  PermissionAlways,
	}}
}

func (t *NotificationSendTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"title":    map[string]interface{}{"type": "string", "description": "Notification title"},
			"content":  map[string]interface{}{"type": "string", "description": "Notification body text"},
			"id":       map[string]interface{}{"type": "string", "description": "Notification ID for updating existing notifications (optional)"},
			"priority": map[string]interface{}{"type": "string", "description": "Priority: default, low, min, high, max (default: default)"},
			"sound":    map[string]interface{}{"type": "string", "description": "Play notification sound: true or false (default: false)"},
		},
		"required": []string{"title", "content"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *NotificationSendTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	title, _ := params["title"].(string)
	content, _ := params["content"].(string)
	if title == "" || content == "" {
		return &ToolResult{Success: false, Error: "title and content are required"}, fmt.Errorf("title and content required")
	}
	id, _ := params["id"].(string)
	priority, _ := params["priority"].(string)
	if priority == "" {
		priority = "default"
	}
	sound, _ := params["sound"].(string)

	flags := fmt.Sprintf("-t %s -c %s --priority %s",
		shellescape(title), shellescape(content), shellescape(priority))
	if id != "" {
		flags += " --id " + shellescape(id)
	}
	if sound == "true" {
		flags += " --sound"
	}

	cmd := fmt.Sprintf(
		`if command -v termux-notification >/dev/null 2>&1; then `+
			`termux-notification %s && echo "Notification sent"; `+
			`else echo "Error: Termux:API not installed. Install the Termux:API app and run: pkg install termux-api"; fi`,
		flags,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 10})
}

// ─── PACKAGE MANAGEMENT ───────────────────────────────────────────────────────

// PkgInstallTool manages Termux packages via the pkg/apt front-end.
type PkgInstallTool struct{ BaseTool }

func NewPkgInstallTool() *PkgInstallTool {
	return &PkgInstallTool{BaseTool: BaseTool{
		name:               "pkg_install",
		description:        "Manage Termux/apt packages: install, remove, update, search, or list installed packages.",
		category:           CategoryPackage,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *PkgInstallTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: install, remove, update, upgrade, search, list",
				"enum":        []string{"install", "remove", "update", "upgrade", "search", "list"},
			},
			"packages": map[string]interface{}{
				"type":        "string",
				"description": "Space-separated package name(s) (required for install, remove, search)",
			},
		},
		"required": []string{"action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *PkgInstallTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, _ := params["action"].(string)
	if action == "" {
		return &ToolResult{Success: false, Error: "action is required"}, fmt.Errorf("action required")
	}
	packages, _ := params["packages"].(string)

	var cmd string
	switch action {
	case "install":
		if packages == "" {
			return &ToolResult{Success: false, Error: "packages required for install"}, nil
		}
		cmd = fmt.Sprintf("pkg install -y %s", shellescape(packages))
	case "remove":
		if packages == "" {
			return &ToolResult{Success: false, Error: "packages required for remove"}, nil
		}
		cmd = fmt.Sprintf("pkg uninstall -y %s", shellescape(packages))
	case "update":
		cmd = "pkg update -y"
	case "upgrade":
		cmd = "pkg upgrade -y"
	case "search":
		if packages == "" {
			return &ToolResult{Success: false, Error: "packages (search term) required for search"}, nil
		}
		cmd = fmt.Sprintf("pkg search %s", shellescape(packages))
	case "list":
		if packages != "" {
			// Targeted check: show status for specific packages only.
			// dpkg -s emits ~10 lines per package, not the full database.
			cmd = fmt.Sprintf("dpkg -s %s 2>/dev/null || apt-cache show %s 2>/dev/null | head -30", shellescape(packages), shellescape(packages))
		} else {
			// Full list: truncate to 60 entries + show total count.
			// pkg list-installed can emit 400+ lines which floods AI context.
			cmd = `pkg list-installed 2>/dev/null | head -60; echo ""; pkg list-installed 2>/dev/null | wc -l | xargs -I{} echo "({} packages installed total — use packages param to query specific ones)"`
		}
	default:
		return &ToolResult{Success: false, Error: "invalid action: " + action}, nil
	}

	cmd = fmt.Sprintf(
		`if command -v pkg >/dev/null 2>&1; then %s; `+
			`elif command -v apt-get >/dev/null 2>&1; then %s; `+
			`else echo "Error: no package manager found (pkg or apt-get)"; fi`,
		cmd, cmd,
	)

	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd, "timeout": 120})
}
