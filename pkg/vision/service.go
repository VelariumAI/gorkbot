package vision

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const companionBaseURL = "http://127.0.0.1:7777"

// companionClient is a short-timeout HTTP client for the local companion service.
var companionClient = &http.Client{Timeout: 25 * time.Second}

// CompanionRunning returns true if the companion service is reachable.
func CompanionRunning(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, companionBaseURL+"/status", nil)
	if err != nil {
		return false
	}
	resp, err := companionClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// CaptureViaCompanion asks the companion service for a PNG screenshot.
// Returns raw PNG bytes. Companion must already be running.
func CaptureViaCompanion(ctx context.Context) (*CaptureResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, companionBaseURL+"/screenshot", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := companionClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("companion not reachable at %s: %w\n\n"+
			"Start it with: adb_setup install  (or tap 'Gorkbot Vision' in your app drawer)", companionBaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("companion returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024)) // 64 MB max
	if err != nil {
		return nil, fmt.Errorf("read screenshot: %w", err)
	}

	if len(data) < 8 {
		return nil, fmt.Errorf("companion returned too few bytes (%d)", len(data))
	}

	// Validate PNG magic
	if data[0] != 0x89 || data[1] != 'P' || data[2] != 'N' || data[3] != 'G' {
		return nil, fmt.Errorf("companion did not return a valid PNG")
	}

	return &CaptureResult{
		Data:     data,
		Format:   "png",
		Strategy: "companion-mediaprojection",
	}, nil
}

// StopCompanion sends a stop signal to the companion service.
func StopCompanion(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, companionBaseURL+"/stop", nil)
	if err != nil {
		return err
	}
	resp, err := companionClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
