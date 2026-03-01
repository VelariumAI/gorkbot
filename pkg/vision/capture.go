// Package vision provides screen capture and AI-powered image analysis.
//
// Capture strategy (in order):
//  1. Companion service (pkg/vision/service.go) — MediaProjection over localhost HTTP.
//  2. Latest screenshot from ~/storage/pictures/Screenshots/ (user takes manually).
package vision

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CaptureResult holds raw image bytes from a screen capture.
type CaptureResult struct {
	Data     []byte
	Format   string // "png" or "jpeg"
	Strategy string // which method succeeded
}

// adbBin — kept for ADBSetup tool diagnostics only; not used for capture.
const adbBin = "/data/data/com.termux/files/usr/bin/adb"

// screenshotDirs are the paths searched (in order) for manually-taken screenshots.
var screenshotDirs = []string{
	filepath.Join(os.Getenv("HOME"), "storage", "pictures", "Screenshots"),
	filepath.Join(os.Getenv("HOME"), "storage", "dcim", "Screenshots"),
	filepath.Join(os.Getenv("HOME"), "storage", "pictures"),
}

// CaptureScreen captures the device screen.
//
// Strategy 1: companion MediaProjection service on 127.0.0.1:7777 (automatic).
// Strategy 2: latest screenshot file from ~/storage/pictures/Screenshots/.
func CaptureScreen(ctx context.Context) (*CaptureResult, error) {
	captureCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	// Strategy 1: companion service
	if CompanionRunning(captureCtx) {
		return CaptureViaCompanion(captureCtx)
	}

	// Strategy 2: latest manually-taken screenshot
	return captureLatestScreenshot()
}

// captureLatestScreenshot reads the most recently modified image file from the
// device's Screenshots folder (populated by the Android built-in screenshot,
// triggered with power+volume-down or palm swipe).
func captureLatestScreenshot() (*CaptureResult, error) {
	home, _ := os.UserHomeDir()

	dirs := []string{
		filepath.Join(home, "storage", "pictures", "Screenshots"),
		filepath.Join(home, "storage", "dcim", "Screenshots"),
		filepath.Join(home, "storage", "pictures"),
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		// Collect image files with their modification times
		type imgFile struct {
			path    string
			modTime int64
		}
		var imgs []imgFile
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if !strings.HasSuffix(name, ".png") && !strings.HasSuffix(name, ".jpg") && !strings.HasSuffix(name, ".jpeg") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			imgs = append(imgs, imgFile{
				path:    filepath.Join(dir, e.Name()),
				modTime: info.ModTime().UnixNano(),
			})
		}
		if len(imgs) == 0 {
			continue
		}

		// Most recent first
		sort.Slice(imgs, func(i, j int) bool {
			return imgs[i].modTime > imgs[j].modTime
		})

		data, err := os.ReadFile(imgs[0].path)
		if err != nil {
			continue
		}

		format := "png"
		name := strings.ToLower(imgs[0].path)
		if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") {
			format = "jpeg"
		}

		return &CaptureResult{
			Data:     data,
			Format:   format,
			Strategy: "screenshot-file:" + filepath.Base(imgs[0].path),
		}, nil
	}

	return nil, fmt.Errorf(
		"no screenshot found.\n\n" +
			"Take a screenshot first:\n" +
			"  • Press  power + volume-down  simultaneously, OR\n" +
			"  • Swipe your palm across the screen (if enabled in Settings)\n\n" +
			"Then ask again — gorkbot will read the latest screenshot automatically.",
	)
}

// ADBStatus — kept for the adb_setup diagnostic tool. Not used for capture.
func ADBStatus(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, adbBin, "devices", "-l").Output()
	if err != nil {
		return fmt.Sprintf("ADB error: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// validatePNG checks the PNG magic bytes of raw image data.
func validatePNG(data []byte) bool {
	return len(data) >= 8 &&
		data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G'
}

// adbCapture is a fallback for manual use only — not called from CaptureScreen.
// Kept so the adb_setup tool can still do a one-off test capture.
func adbCapture(ctx context.Context) (*CaptureResult, error) {
	if !isADBReady(ctx) {
		result, _ := AutoConnect(ctx)
		if result == nil || !result.Connected {
			return nil, fmt.Errorf("no ADB device connected")
		}
	}

	cmd := exec.CommandContext(ctx, adbBin, "exec-out", "/system/bin/screencap", "-p")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("adb screencap: %w — %s", err, strings.TrimSpace(stderr.String()))
	}

	data := stdout.Bytes()
	if !validatePNG(data) {
		return nil, fmt.Errorf("adb screencap did not return valid PNG")
	}

	return &CaptureResult{
		Data:     data,
		Format:   "png",
		Strategy: "adb-fallback",
	}, nil
}
