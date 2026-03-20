package tools

// android_control.go — granular Android/Termux control tools.
// Integrated from Gorkbot's dynamic tool generation history.
// All tools use shellescape() for injection prevention; DB paths are
// resolved via gorkStateDB() so they work on any device.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ─── DB init helper ──────────────────────────────────────────────────────────

// ensureStateDB ensures the Gorkbot state DB and its schema exist.
func ensureStateDB(ctx context.Context) string {
	db := gorkStateDB()
	if err := os.MkdirAll(filepath.Dir(db), 0700); err != nil {
		return db
	}
	bash := NewBashTool()
	bash.Execute(ctx, map[string]interface{}{ //nolint
		"command": fmt.Sprintf("sqlite3 %s %s 2>/dev/null; true",
			shellescape(db), shellescape(gorkStateDBInit())),
	})
	return db
}

// ─── ADB ─────────────────────────────────────────────────────────────────────

// AdbScreenshotTool captures the screen via screencap (no Termux:API required).
type AdbScreenshotTool struct{ BaseTool }

func NewAdbScreenshotTool() *AdbScreenshotTool {
	return &AdbScreenshotTool{BaseTool: BaseTool{
		name:               "adb_screenshot",
		description:        "Capture device screen via screencap (works without Termux:API). Saves to /sdcard/gorkbot_vision.png.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *AdbScreenshotTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Output path (default: /sdcard/gorkbot_vision.png)",
			},
		},
	})
	return s
}

func (t *AdbScreenshotTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "/sdcard/gorkbot_vision.png"
	if p, ok := params["path"].(string); ok && p != "" {
		path = p
	}
	cmd := fmt.Sprintf("screencap -p %s && echo 'Captured: %s'", shellescape(path), shellescape(path))
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── AdbShellTool ────────────────────────────────────────────────────────────

// AdbShellTool executes arbitrary ADB shell commands via termux-adb.
type AdbShellTool struct{ BaseTool }

func NewAdbShellTool() *AdbShellTool {
	return &AdbShellTool{BaseTool: BaseTool{
		name:               "adb_shell",
		description:        "Execute ADB shell commands on connected Android devices via termux-adb. Provides full system-level access.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *AdbShellTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell command to run via ADB (e.g. 'dumpsys battery')",
			},
		},
		"required": []string{"command"},
	})
	return s
}

func (t *AdbShellTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	command, _ := params["command"].(string)
	if command == "" {
		return &ToolResult{Success: false, Error: "command is required"}, nil
	}
	cmd := fmt.Sprintf("termux-adb shell %s", shellescape(command))
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── App management ──────────────────────────────────────────────────────────

// AppCatalogTool lists installed Android apps with optional filter.
type AppCatalogTool struct{ BaseTool }

func NewAppCatalogTool() *AppCatalogTool {
	return &AppCatalogTool{BaseTool: BaseTool{
		name:               "app_catalog",
		description:        "List all installed Android apps. Optionally filter by name/package. Returns package names.",
		category:           CategoryAndroid,
		requiresPermission: false,
		defaultPermission:  PermissionSession,
	}}
}

func (t *AppCatalogTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "Optional substring to filter package names (e.g. 'google', 'chrome')",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max results to return (default 50)",
			},
		},
	})
	return s
}

func (t *AppCatalogTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	filter, _ := params["filter"].(string)
	limit := 50
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	var cmd string
	if filter != "" {
		cmd = fmt.Sprintf(
			"pm list packages | sed 's/package://g' | grep -i %s | head -%d | nl",
			shellescape(filter), limit,
		)
	} else {
		cmd = fmt.Sprintf("pm list packages | sed 's/package://g' | head -%d | nl", limit)
	}
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// AppControlTool launches, kills, or queries Android apps.
type AppControlTool struct{ BaseTool }

func NewAppControlTool() *AppControlTool {
	return &AppControlTool{BaseTool: BaseTool{
		name:               "app_control",
		description:        "Full Android app control: launch by name/package, force-stop, list matching apps, or show foreground app.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *AppControlTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"launch", "kill", "list", "current"},
				"description": "Action: launch (start app), kill (force-stop), list (find matching), current (foreground app)",
			},
			"app": map[string]interface{}{
				"type":        "string",
				"description": "App name or package (used for launch/kill/list)",
			},
			"activity": map[string]interface{}{
				"type":        "string",
				"description": "Activity class for launch (optional; omit to use MAIN launcher intent)",
			},
		},
		"required": []string{"action"},
	})
	return s
}

func (t *AppControlTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, _ := params["action"].(string)
	app, _ := params["app"].(string)
	activity, _ := params["activity"].(string)

	var cmd string
	switch action {
	case "launch":
		if app == "" {
			return &ToolResult{Success: false, Error: "app is required for launch"}, nil
		}
		if activity != "" {
			cmd = fmt.Sprintf("am start -n %s/%s",
				shellescape(app), shellescape(activity))
		} else {
			cmd = fmt.Sprintf(
				"PKG=$(pm list packages | grep -i %s | head -1 | cut -d: -f2); am start -a android.intent.action.MAIN -c android.intent.category.LAUNCHER -n $PKG",
				shellescape(app))
		}
	case "kill":
		if app == "" {
			return &ToolResult{Success: false, Error: "app is required for kill"}, nil
		}
		cmd = fmt.Sprintf(
			"PKG=$(pm list packages | grep -i %s | head -1 | cut -d: -f2); am force-stop $PKG && echo \"Stopped: $PKG\"",
			shellescape(app))
	case "list":
		if app == "" {
			return &ToolResult{Success: false, Error: "app is required for list"}, nil
		}
		cmd = fmt.Sprintf("pm list packages | grep -i %s | sed 's/package://'", shellescape(app))
	case "current":
		cmd = "dumpsys activity activities 2>/dev/null | grep mResumedActivity | head -3 || dumpsys window 2>/dev/null | grep mCurrentFocus | head -3"
	default:
		return &ToolResult{Success: false, Error: "action must be one of: launch, kill, list, current"}, nil
	}
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// AppStatusTool reports the current foreground app and basic device state.
type AppStatusTool struct{ BaseTool }

func NewAppStatusTool() *AppStatusTool {
	return &AppStatusTool{BaseTool: BaseTool{
		name:               "app_status",
		description:        "Show the current foreground app and recent task stack via dumpsys.",
		category:           CategoryAndroid,
		requiresPermission: false,
		defaultPermission:  PermissionSession,
	}}
}

func (t *AppStatusTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{"type": "object", "properties": map[string]interface{}{}})
	return s
}

func (t *AppStatusTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	cmd := "dumpsys activity activities 2>/dev/null | grep mResumedActivity | head -5 || echo 'No foreground activity detected'"
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── Screen capture ──────────────────────────────────────────────────────────

// ScreenCaptureTool takes a screenshot using the best available method.
type ScreenCaptureTool struct{ BaseTool }

func NewScreenCaptureTool() *ScreenCaptureTool {
	return &ScreenCaptureTool{BaseTool: BaseTool{
		name:               "screen_capture",
		description:        "Capture a screenshot using the best available method (termux-screenshot → screencap → keyevent fallback).",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *ScreenCaptureTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Output file path (default: /sdcard/gorkbot_screen.jpg)",
			},
		},
	})
	return s
}

func (t *ScreenCaptureTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path := "/sdcard/gorkbot_screen.jpg"
	if p, ok := params["path"].(string); ok && p != "" {
		path = p
	}
	cmd := fmt.Sprintf(
		"termux-screenshot -f %s 2>/dev/null && echo 'Captured via termux-screenshot' || "+
			"(screencap -p %s && echo 'Captured via screencap') || "+
			"(input keyevent 120 && echo 'Triggered hardware screenshot button')",
		shellescape(path), shellescape(path))
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ScreenshotTool captures a high-quality screenshot via Termux:API.
type ScreenshotTool struct{ BaseTool }

func NewScreenshotTool() *ScreenshotTool {
	return &ScreenshotTool{BaseTool: BaseTool{
		name: "screenshot",
		description: "Capture device screenshot via termux-screenshot. " +
			"Output is always PNG. Default path: $HOME/gorkbot_screen.png. " +
			"Requires Termux:API app + package installed.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *ScreenshotTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Output PNG file path (default: $HOME/gorkbot_screen.png)",
			},
		},
		"required": []string{},
	})
	return s
}

func (t *ScreenshotTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path, _ := params["path"].(string)
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, "gorkbot_screen.png")
	}
	cmd := fmt.Sprintf("termux-screenshot -f %s && echo 'Saved: %s'",
		shellescape(path), shellescape(path))
	result, err := NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
	if err != nil {
		return result, err
	}
	// Verify the file actually exists — termux-screenshot can exit 0 without writing
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return &ToolResult{
			Success: false,
			Error: fmt.Sprintf(
				"termux-screenshot ran but output file not found at %s — "+
					"check that Termux:API app is installed and permission is granted",
				path),
		}, nil
	}
	return result, nil
}

// ScreenrecordTool records the device screen to a video file.
type ScreenrecordTool struct{ BaseTool }

func NewScreenrecordTool() *ScreenrecordTool {
	return &ScreenrecordTool{BaseTool: BaseTool{
		name:               "screenrecord",
		description:        "Record the device screen to a video file via termux-screenrecord (up to 30s clips).",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *ScreenrecordTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Output file path (e.g. /sdcard/recording.mp4)",
			},
			"duration": map[string]interface{}{
				"type":        "integer",
				"description": "Duration in seconds (default 10, max 30)",
			},
		},
		"required": []string{"path"},
	})
	return s
}

func (t *ScreenrecordTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return &ToolResult{Success: false, Error: "path is required"}, nil
	}
	duration := 10
	if d, ok := params["duration"].(float64); ok && d > 0 {
		if int(d) > 30 {
			duration = 30
		} else {
			duration = int(d)
		}
	}
	cmd := fmt.Sprintf("termux-screenrecord -f %s %ds && echo 'Recorded: %s'",
		shellescape(path), duration, shellescape(path))
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// CaptureScreenHackTool tries every available screen capture method in sequence.
type CaptureScreenHackTool struct{ BaseTool }

func NewCaptureScreenHackTool() *CaptureScreenHackTool {
	return &CaptureScreenHackTool{BaseTool: BaseTool{
		name:               "capture_screen_hack",
		description:        "Multi-method screen capture: tries screencap, uiautomator, keyevent, wm size, and SurfaceFlinger in sequence. Use when standard screenshot tools fail.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *CaptureScreenHackTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{"type": "object", "properties": map[string]interface{}{}})
	return s
}

func (t *CaptureScreenHackTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	cmd := `
OUT=/sdcard/gorkbot_screen.png
echo "Attempting screencap..." && screencap -p "$OUT" 2>/dev/null && echo "SUCCESS: screencap -> $OUT" && exit 0
echo "Attempting input keyevent 120..." && input keyevent 120 2>/dev/null && echo "SUCCESS: hardware screenshot"
echo "Attempting uiautomator dump..." && uiautomator dump /sdcard/ui.xml 2>/dev/null && echo "SUCCESS: ui dump"
echo "Checking display size..." && wm size 2>/dev/null
echo "Checking SurfaceFlinger..." && dumpsys SurfaceFlinger 2>/dev/null | head -5
echo "Fallback: manual pull-down screenshot recommended. Path: /sdcard/"
`
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── UI inspection ───────────────────────────────────────────────────────────

// UiDumpTool dumps the full Android UI accessibility hierarchy.
type UiDumpTool struct{ BaseTool }

func NewUiDumpTool() *UiDumpTool {
	return &UiDumpTool{BaseTool: BaseTool{
		name:               "ui_dump",
		description:        "Dump the full Android UI hierarchy to XML via uiautomator. Returns text, content-desc, and bounds of visible elements.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *UiDumpTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"max_elements": map[string]interface{}{
				"type":        "integer",
				"description": "Max elements to return (default 30)",
			},
		},
	})
	return s
}

func (t *UiDumpTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	max := 30
	if m, ok := params["max_elements"].(float64); ok && m > 0 {
		max = int(m)
	}
	cmd := fmt.Sprintf(
		"uiautomator dump /sdcard/gorkbot_ui.xml 2>/dev/null && "+
			"cat /sdcard/gorkbot_ui.xml | grep -oE '(text|content-desc|bounds)=\"[^\"]+\"' | head -%d || "+
			"echo 'uiautomator unavailable — try adb_shell with uiautomator dump'",
		max)
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── Device info & state ─────────────────────────────────────────────────────

// DeviceInfoTool gathers comprehensive Android device diagnostics.
type DeviceInfoTool struct{ BaseTool }

func NewDeviceInfoTool() *DeviceInfoTool {
	return &DeviceInfoTool{BaseTool: BaseTool{
		name:               "device_info",
		description:        "Full Android device diagnostics: battery, storage, installed app count, screen size, build info, and sensor data.",
		category:           CategoryAndroid,
		requiresPermission: false,
		defaultPermission:  PermissionSession,
	}}
}

func (t *DeviceInfoTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{"type": "object", "properties": map[string]interface{}{}})
	return s
}

func (t *DeviceInfoTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	cmd := `
echo "=== Battery ==="
dumpsys battery 2>/dev/null | grep -E 'level|status|temperature|voltage' | head -8
echo "=== Storage ==="
df -h /sdcard /data 2>/dev/null | head -5
echo "=== Apps installed ==="
echo "$(pm list packages 2>/dev/null | wc -l) packages"
echo "=== Display ==="
wm size 2>/dev/null; wm density 2>/dev/null
echo "=== Build ==="
getprop ro.product.model 2>/dev/null; getprop ro.build.version.release 2>/dev/null
echo "=== Sensors (1 sample) ==="
termux-sensor -s all -n 1 2>/dev/null | head -20 || echo "termux-sensor unavailable"
`
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ContextStateTool reports current phone/app/screen state in a single call.
type ContextStateTool struct{ BaseTool }

func NewContextStateTool() *ContextStateTool {
	return &ContextStateTool{BaseTool: BaseTool{
		name:               "context_state",
		description:        "Snapshot of current phone state: foreground app, battery level, network, and screen status.",
		category:           CategoryAndroid,
		requiresPermission: false,
		defaultPermission:  PermissionSession,
	}}
}

func (t *ContextStateTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{"type": "object", "properties": map[string]interface{}{}})
	return s
}

func (t *ContextStateTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	cmd := `
echo "=== Foreground App ==="
dumpsys activity activities 2>/dev/null | grep mResumedActivity | head -2 || echo "unknown"
echo "=== Battery ==="
dumpsys battery 2>/dev/null | grep -E 'level|status' | head -3
echo "=== Network ==="
dumpsys connectivity 2>/dev/null | grep -E 'NetworkAgentInfo|type=' | head -3 || ip route 2>/dev/null | head -3
echo "=== Screen ==="
dumpsys power 2>/dev/null | grep -E 'mWakefulness|Display Power' | head -3
`
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── App lifecycle ───────────────────────────────────────────────────────────

// KillAppTool force-stops an app by name or package.
type KillAppTool struct{ BaseTool }

func NewKillAppTool() *KillAppTool {
	return &KillAppTool{BaseTool: BaseTool{
		name:               "kill_app",
		description:        "Force-stop an Android app by name or partial package name.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *KillAppTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"app_name": map[string]interface{}{
				"type":        "string",
				"description": "App name or partial package name (e.g. 'chrome', 'com.google.android.youtube')",
			},
		},
		"required": []string{"app_name"},
	})
	return s
}

func (t *KillAppTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	appName, _ := params["app_name"].(string)
	if appName == "" {
		return &ToolResult{Success: false, Error: "app_name is required"}, nil
	}
	cmd := fmt.Sprintf(
		`PKG=$(pm list packages | grep -i %s | head -1 | cut -d: -f2); `+
			`[ -n "$PKG" ] && am force-stop "$PKG" && echo "Stopped: $PKG" || echo "No package found matching: %s"`,
		shellescape(appName), appName)
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// LaunchAppTool launches an app by name or package.
type LaunchAppTool struct{ BaseTool }

func NewLaunchAppTool() *LaunchAppTool {
	return &LaunchAppTool{BaseTool: BaseTool{
		name:               "launch_app",
		description:        "Launch any Android app by name or partial package name via MAIN launcher intent.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *LaunchAppTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"app_name": map[string]interface{}{
				"type":        "string",
				"description": "App name or partial package name (e.g. 'maps', 'com.spotify')",
			},
		},
		"required": []string{"app_name"},
	})
	return s
}

func (t *LaunchAppTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	appName, _ := params["app_name"].(string)
	if appName == "" {
		return &ToolResult{Success: false, Error: "app_name is required"}, nil
	}
	cmd := fmt.Sprintf(
		`PKG=$(pm list packages | grep -i %s | head -1 | cut -d: -f2); `+
			`[ -n "$PKG" ] && am start -a android.intent.action.MAIN -c android.intent.category.LAUNCHER -n $PKG && echo "Launched: $PKG" || echo "No package found matching: %s"`,
		shellescape(appName), appName)
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── Vision pipeline ─────────────────────────────────────────────────────────

// VisionCaptureTool captures screen/camera for AI vision analysis.
type VisionCaptureTool struct{ BaseTool }

func NewVisionCaptureTool() *VisionCaptureTool {
	return &VisionCaptureTool{BaseTool: BaseTool{
		name:               "android_vision_capture",
		description:        "Capture screen or camera frame (termux-screenshot / termux-camera-photo) and save to standard path for AI vision analysis.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *VisionCaptureTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"source": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"screen", "camera"},
				"description": "Capture source: screen (default) or camera",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Output path (default: /sdcard/gorkbot_vision.jpg)",
			},
		},
	})
	return s
}

func (t *VisionCaptureTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	source, _ := params["source"].(string)
	if source == "" {
		source = "screen"
	}
	path := "/sdcard/gorkbot_vision.jpg"
	if p, ok := params["path"].(string); ok && p != "" {
		path = p
	}

	var cmd string
	switch source {
	case "camera":
		cmd = fmt.Sprintf("termux-camera-photo -c 0 %s 2>/dev/null && echo 'Camera captured: %s' || echo 'Camera unavailable'",
			shellescape(path), shellescape(path))
	default:
		cmd = fmt.Sprintf(
			"termux-screenshot -f %s 2>/dev/null && echo 'Screen captured: %s' || "+
				"(screencap -p %s && echo 'Screen captured via screencap: %s')",
			shellescape(path), shellescape(path), shellescape(path), shellescape(path))
	}
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// VisionAnalyzeTool captures the screen and sends it to the grok-2-vision API.
type VisionAnalyzeTool struct{ BaseTool }

func NewVisionAnalyzeTool() *VisionAnalyzeTool {
	return &VisionAnalyzeTool{BaseTool: BaseTool{
		name:               "vision_analyze",
		description:        "Capture device screen then analyze via grok-2-vision-1212. Returns AI description of what is on screen.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *VisionAnalyzeTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Question or instruction for the vision model (e.g. 'What app is open? What does it show?')",
			},
			"image_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to existing image (skips capture if provided)",
			},
		},
		"required": []string{"prompt"},
	})
	return s
}

func (t *VisionAnalyzeTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return &ToolResult{Success: false, Error: "prompt is required"}, nil
	}
	imagePath, _ := params["image_path"].(string)
	if imagePath == "" {
		imagePath = "/sdcard/gorkbot_vision.jpg"
	}

	payload := fmt.Sprintf(
		`{"model":"grok-2-vision-1212","messages":[{"role":"user","content":[{"type":"text","text":%s},{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,$(base64 -w0 %s)"}}]}],"max_tokens":1024}`,
		jsonStringEscape(prompt),
		shellescape(imagePath),
	)

	cmd := fmt.Sprintf(
		`IMG=%s
# Capture if file doesn't exist or is older than 5 seconds
if [ ! -f "$IMG" ] || [ $(( $(date +%%s) - $(stat -c %%Y "$IMG" 2>/dev/null || echo 0) )) -gt 5 ]; then
  termux-screenshot -f "$IMG" 2>/dev/null || screencap -p "$IMG"
  echo "Captured screen → $IMG"
fi
echo "Analyzing with grok-2-vision..."
curl -s -X POST https://api.x.ai/v1/chat/completions \
  -H "Authorization: Bearer $XAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d %s | jq -r '.choices[0].message.content // .error.message // "Vision API error"'`,
		shellescape(imagePath),
		shellescape(payload),
	)
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// LiveVisionTool runs the full capture → grok-2-vision pipeline in one step.
type LiveVisionTool struct{ BaseTool }

func NewLiveVisionTool() *LiveVisionTool {
	return &LiveVisionTool{BaseTool: BaseTool{
		name:               "live_vision",
		description:        "Full pipeline: capture device screen → send to grok-2-vision → return analysis. One-shot convenience wrapper around vision_capture + vision_analyze.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *LiveVisionTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "What to ask about the screen (e.g. 'Describe what you see', 'What errors are shown?')",
			},
		},
		"required": []string{"prompt"},
	})
	return s
}

func (t *LiveVisionTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return &ToolResult{Success: false, Error: "prompt is required"}, nil
	}
	imagePath := "/sdcard/gorkbot_vision.jpg"
	payload := fmt.Sprintf(
		`{"model":"grok-2-vision-1212","messages":[{"role":"user","content":[{"type":"text","text":%s},{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,$(base64 -w0 %s)"}}]}],"max_tokens":1024}`,
		jsonStringEscape(prompt),
		shellescape(imagePath),
	)
	cmd := fmt.Sprintf(
		`IMG=%s
termux-screenshot -f "$IMG" 2>/dev/null || screencap -p "$IMG"
echo "Captured → $IMG. Sending to grok-2-vision..."
curl -s -X POST https://api.x.ai/v1/chat/completions \
  -H "Authorization: Bearer $XAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d %s | jq -r '.choices[0].message.content // .error.message // "Vision API unavailable"'`,
		shellescape(imagePath),
		shellescape(payload),
	)
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ScreenAnalyzeTool detects screen-related queries, captures, and analyzes automatically.
type ScreenAnalyzeTool struct{ BaseTool }

func NewScreenAnalyzeTool() *ScreenAnalyzeTool {
	return &ScreenAnalyzeTool{BaseTool: BaseTool{
		name:               "screen_analyze",
		description:        "Auto-detects if the query is screen-related, captures the screen, and sends to vision AI for analysis.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *ScreenAnalyzeTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Natural language query about the screen (e.g. 'what is on my screen?', 'what app is open?')",
			},
		},
		"required": []string{"query"},
	})
	return s
}

func (t *ScreenAnalyzeTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	query, _ := params["query"].(string)

	// Use Go-side check to avoid shell injection via query string.
	isScreenQuery := containsAny(query, "screen", "what", "see", "looking", "display", "show", "app", "open")

	if !isScreenQuery {
		return &ToolResult{
			Success: true,
			Output:  "Query doesn't appear to be screen-related. For screen analysis, ask about what is on/shown on the screen.",
		}, nil
	}

	// Delegate to VisionAnalyzeTool with the query as the prompt.
	vt := NewVisionAnalyzeTool()
	return vt.Execute(ctx, map[string]interface{}{"prompt": query})
}

// ─── Package management ──────────────────────────────────────────────────────
// Note: PkgInstallTool is defined in advanced.go and registered from there.

// ManageDepsTool intelligently checks, installs, and logs package dependencies.
type ManageDepsTool struct{ BaseTool }

func NewManageDepsTool() *ManageDepsTool {
	return &ManageDepsTool{BaseTool: BaseTool{
		name:               "manage_deps",
		description:        "Smart dependency manager: checks if packages are installed, installs if missing, verifies, and logs to state DB. Extends pkg_install with automatic check-before-install logic.",
		category:           CategoryAndroid,
		requiresPermission: false,
		defaultPermission:  PermissionSession,
	}}
}

func (t *ManageDepsTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"packages": map[string]interface{}{
				"type":        "string",
				"description": "Package name(s) to ensure are installed (space-separated)",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"install", "remove", "check"},
				"description": "Action: install (default), remove, or check only",
			},
		},
		"required": []string{"packages"},
	})
	return s
}

func (t *ManageDepsTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	packages, _ := params["packages"].(string)
	if packages == "" {
		return &ToolResult{Success: false, Error: "packages is required"}, nil
	}
	action, _ := params["action"].(string)
	if action == "" {
		action = "install"
	}
	db := ensureStateDB(ctx)
	sqlLog := sqlEscapeSingleQuote(fmt.Sprintf("Deps %s %s ok", packages, action))

	cmd := fmt.Sprintf(
		`PKGS=%s ACTION=%s DB=%s
if pkg list-installed 2>/dev/null | grep -qi "$(echo $PKGS | cut -d' ' -f1)"; then
  echo "$PKGS: already installed"
elif [ "$ACTION" = "check" ]; then
  echo "$PKGS: not installed"
else
  pkg "$ACTION" -y $PKGS && \
  echo "Verified: $PKGS installed" && \
  sqlite3 "$DB" "INSERT INTO logs (level,message) VALUES ('info','%s')" 2>/dev/null || \
  echo "Failed: $PKGS $ACTION"
fi`,
		shellescape(packages), shellescape(action), shellescape(db), sqlLog,
	)
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── Termux master control ───────────────────────────────────────────────────

// TermuxControlTool provides master Termux environment control.
type TermuxControlTool struct{ BaseTool }

func NewTermuxControlTool() *TermuxControlTool {
	return &TermuxControlTool{BaseTool: BaseTool{
		name:               "termux_control",
		description:        "Master Termux environment control: install packages, manage aliases, set env vars, configure storage, update system.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *TermuxControlTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"install", "alias", "env", "setup", "storage", "update"},
				"description": "Action: install (pkg), alias (create shell alias), env (set variable), setup (pkg+storage), storage (termux-setup-storage), update (pkg update+upgrade)",
			},
			"packages": map[string]interface{}{
				"type":        "string",
				"description": "Package names for install/setup action",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Alias name or env var name",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "Alias command or env var value",
			},
		},
		"required": []string{"action"},
	})
	return s
}

func (t *TermuxControlTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, _ := params["action"].(string)
	packages, _ := params["packages"].(string)
	name, _ := params["name"].(string)
	value, _ := params["value"].(string)

	var cmd string
	switch action {
	case "install":
		if packages == "" {
			return &ToolResult{Success: false, Error: "packages required for install"}, nil
		}
		cmd = fmt.Sprintf("pkg install -y %s", shellescape(packages))
	case "alias":
		if name == "" || value == "" {
			return &ToolResult{Success: false, Error: "name and value required for alias"}, nil
		}
		aliasLine := fmt.Sprintf("alias %s=%s", name, shellescape(value))
		cmd = fmt.Sprintf("echo %s >> ~/.bashrc && echo 'Alias added. Run: source ~/.bashrc'", shellescape(aliasLine))
	case "env":
		if name == "" {
			return &ToolResult{Success: false, Error: "name required for env"}, nil
		}
		if value == "" {
			cmd = fmt.Sprintf("echo %s", shellescape(fmt.Sprintf("$%s", name)))
		} else {
			exportLine := fmt.Sprintf("export %s=%s", name, shellescape(value))
			cmd = fmt.Sprintf("echo %s >> ~/.bashrc && export %s=%s && echo 'Set %s'",
				shellescape(exportLine), shellescape(name), shellescape(value), name)
		}
	case "setup":
		if packages == "" {
			cmd = "termux-setup-storage"
		} else {
			cmd = fmt.Sprintf("pkg install -y %s && termux-setup-storage", shellescape(packages))
		}
	case "storage":
		cmd = "termux-setup-storage"
	case "update":
		cmd = "pkg update -y && pkg upgrade -y"
	default:
		return &ToolResult{Success: false, Error: "action must be one of: install, alias, env, setup, storage, update"}, nil
	}
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── Persistent state ────────────────────────────────────────────────────────

// SaveStateTool persists key-value data across sessions in the Gorkbot state DB.
type SaveStateTool struct{ BaseTool }

func NewSaveStateTool() *SaveStateTool {
	return &SaveStateTool{BaseTool: BaseTool{
		name:               "save_state",
		description:        "Persist a key-value pair to the Gorkbot state DB (~/.gorkbot/state.db). Data survives across sessions. Use for remembering preferences, context, or task state.",
		category:           CategoryAndroid,
		requiresPermission: false,
		defaultPermission:  PermissionAlways,
	}}
}

func (t *SaveStateTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "State key (e.g. 'monitor_status', 'last_project')",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "Value to store",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"set", "get", "delete", "list"},
				"description": "Action: set (default), get, delete, or list all keys",
			},
		},
	})
	return s
}

func (t *SaveStateTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	key, _ := params["key"].(string)
	value, _ := params["value"].(string)
	action, _ := params["action"].(string)
	if action == "" {
		action = "set"
	}
	db := ensureStateDB(ctx)

	var cmd string
	switch action {
	case "set":
		if key == "" {
			return &ToolResult{Success: false, Error: "key is required for set"}, nil
		}
		sql := fmt.Sprintf(
			"INSERT OR REPLACE INTO state (key,value,updated_at) VALUES ('%s','%s',datetime('now'));"+
				"INSERT INTO logs (level,message) VALUES ('info','State set: %s')",
			sqlEscapeSingleQuote(key), sqlEscapeSingleQuote(value), sqlEscapeSingleQuote(key),
		)
		cmd = fmt.Sprintf(`sqlite3 %s %s && echo "Saved: %s"`, shellescape(db), shellescape(sql), key)
	case "get":
		if key == "" {
			return &ToolResult{Success: false, Error: "key is required for get"}, nil
		}
		sql := fmt.Sprintf("SELECT value FROM state WHERE key='%s'", sqlEscapeSingleQuote(key))
		cmd = fmt.Sprintf(`sqlite3 %s %s`, shellescape(db), shellescape(sql))
	case "delete":
		if key == "" {
			return &ToolResult{Success: false, Error: "key is required for delete"}, nil
		}
		sql := fmt.Sprintf("DELETE FROM state WHERE key='%s'", sqlEscapeSingleQuote(key))
		cmd = fmt.Sprintf(`sqlite3 %s %s && echo "Deleted: %s"`, shellescape(db), shellescape(sql), key)
	case "list":
		cmd = fmt.Sprintf(`sqlite3 %s "SELECT key, substr(value,1,60) as val, updated_at FROM state ORDER BY updated_at DESC"`, shellescape(db))
	default:
		return &ToolResult{Success: false, Error: "action must be one of: set, get, delete, list"}, nil
	}
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── Health monitor ──────────────────────────────────────────────────────────

// StartHealthMonitorTool starts a background device health monitoring loop.
type StartHealthMonitorTool struct{ BaseTool }

func NewStartHealthMonitorTool() *StartHealthMonitorTool {
	return &StartHealthMonitorTool{BaseTool: BaseTool{
		name:               "start_health_monitor",
		description:        "Start a background device health monitoring loop that logs battery, storage, and app state every 60 minutes. Run in tmux/screen. Stop with Ctrl+C or kill the PID.",
		category:           CategoryAndroid,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *StartHealthMonitorTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"interval_minutes": map[string]interface{}{
				"type":        "integer",
				"description": "Check interval in minutes (default 60)",
			},
		},
	})
	return s
}

func (t *StartHealthMonitorTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	interval := 60
	if i, ok := params["interval_minutes"].(float64); ok && i > 0 {
		interval = int(i)
	}
	db := ensureStateDB(ctx)
	cmd := fmt.Sprintf(
		`DB=%s
sqlite3 "$DB" "INSERT OR REPLACE INTO state (key,value) VALUES ('monitor_status','running')" 2>/dev/null
echo "Health monitor started (interval: %dm). Ctrl+C to stop."
while true; do
  BATTERY=$(dumpsys battery 2>/dev/null | grep level | head -1 | tr -d ' ')
  STORAGE=$(df -h /sdcard 2>/dev/null | tail -1 | awk '{print $5}')
  APP=$(dumpsys activity activities 2>/dev/null | grep mResumedActivity | head -1)
  MSG="battery=$BATTERY storage=$STORAGE app=$APP"
  sqlite3 "$DB" "INSERT INTO logs (level,message) VALUES ('monitor','$(echo $MSG | head -c200)')" 2>/dev/null
  echo "[$(date)] $MSG"
  sleep %d
done`,
		shellescape(db), interval, interval*60,
	)
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}

// ─── Browser / headless scraping ─────────────────────────────────────────────

// BrowserScrapeTool scrapes dynamic (React/SPA) web pages via headless Chromium + Puppeteer.
type BrowserScrapeTool struct{ BaseTool }

func NewBrowserScrapeTool() *BrowserScrapeTool {
	return &BrowserScrapeTool{BaseTool: BaseTool{
		name:               "browser_scrape",
		description:        "Scrape JavaScript-rendered pages (React/SPA) via headless Chromium + Puppeteer. Returns headings and text content. Requires chromium and puppeteer-core via npm.",
		category:           CategoryWeb,
		requiresPermission: true,
		defaultPermission:  PermissionOnce,
	}}
}

func (t *BrowserScrapeTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to scrape (must include https://)",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector to extract (default: 'h1,h2,h3,p')",
			},
			"max_items": map[string]interface{}{
				"type":        "integer",
				"description": "Max elements to return (default 15)",
			},
		},
		"required": []string{"url"},
	})
	return s
}

func (t *BrowserScrapeTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, _ := params["url"].(string)
	if url == "" {
		return &ToolResult{Success: false, Error: "url is required"}, nil
	}
	selector, _ := params["selector"].(string)
	if selector == "" {
		selector = "h1,h2,h3,p"
	}
	maxItems := 15
	if m, ok := params["max_items"].(float64); ok && m > 0 {
		maxItems = int(m)
	}

	// Build the Puppeteer JS inline — URL and selector are embedded as JS template
	// literals so JS backtick quoting handles special characters naturally.
	// The entire node script is then shellescape()'d for the bash layer.
	jsScript := fmt.Sprintf(
		`const p=require('puppeteer-core');(async()=>{
const b=await p.launch({executablePath:process.env.CHROMIUM_PATH||'/data/data/com.termux/files/usr/bin/chromium',headless:'new',args:['--no-sandbox','--disable-setuid-sandbox','--single-process']});
const pg=await b.newPage();
await pg.goto(`+"`"+`%s`+"`"+`,{waitUntil:'domcontentloaded',timeout:30000});
const t=await pg.$$eval(%s,els=>els.map(e=>e.textContent.trim()).filter(Boolean));
console.log(JSON.stringify(t.slice(0,%d)));
await b.close();
})()`,
		url, jsonStringEscape(selector), maxItems,
	)
	cmd := fmt.Sprintf("node -e %s 2>/dev/null | jq -r '.[]' 2>/dev/null || echo 'Chromium/Puppeteer not installed. Run: pkg install chromium && npm install -g puppeteer-core'",
		shellescape(jsScript))
	return NewBashTool().Execute(ctx, map[string]interface{}{"command": cmd})
}
