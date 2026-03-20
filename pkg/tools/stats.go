package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ExecutionStats tracks historical execution times for tools to enable progress estimation.
type ExecutionStats struct {
	mu       sync.RWMutex
	data     map[string]*toolStat
	filePath string
}

// toolStat holds aggregated execution statistics for a single tool.
type toolStat struct {
	Runs       int     `json:"runs"`
	MedianMs   int64   `json:"median_ms"`
	P90Ms      int64   `json:"p90_ms"`
	samples    []int64 // in-memory ring buffer (last 20 samples)
	maxSamples int
}

// NewExecutionStats creates a new ExecutionStats instance.
func NewExecutionStats(filePath string) *ExecutionStats {
	return &ExecutionStats{
		data:     make(map[string]*toolStat),
		filePath: filePath,
	}
}

// RecordExecution records a tool execution time and updates statistics.
func (s *ExecutionStats) RecordExecution(toolName string, elapsed time.Duration) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[toolName]; !exists {
		s.data[toolName] = &toolStat{
			maxSamples: 20,
			samples:    make([]int64, 0, 20),
		}
	}

	stat := s.data[toolName]
	stat.Runs++

	// Add to ring buffer
	elapsedMs := elapsed.Milliseconds()
	if len(stat.samples) < stat.maxSamples {
		stat.samples = append(stat.samples, elapsedMs)
	} else {
		// Ring buffer: shift and append
		copy(stat.samples, stat.samples[1:])
		stat.samples[stat.maxSamples-1] = elapsedMs
	}

	// Recalculate median and P90
	sorted := make([]int64, len(stat.samples))
	copy(sorted, stat.samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	if len(sorted) > 0 {
		stat.MedianMs = sorted[len(sorted)/2]
		p90Idx := (len(sorted) * 9) / 10
		if p90Idx >= len(sorted) {
			p90Idx = len(sorted) - 1
		}
		stat.P90Ms = sorted[p90Idx]
	}
}

// EstimateProgress estimates the progress (0.0-1.0) of a tool based on elapsed time.
// Returns 0 if no history exists, otherwise uses median as the 100% target.
// Progress is capped at 0.95 while running to never show 100% before completion.
func (s *ExecutionStats) EstimateProgress(toolName string, elapsed time.Duration) float64 {
	if s == nil {
		return 0
	}

	s.mu.RLock()
	stat, exists := s.data[toolName]
	s.mu.RUnlock()

	if !exists || stat.MedianMs == 0 {
		return 0
	}

	progress := float64(elapsed.Milliseconds()) / float64(stat.MedianMs)
	if progress > 0.95 {
		progress = 0.95
	}
	if progress < 0 {
		progress = 0
	}
	return progress
}

// Save persists statistics to disk in JSON format.
func (s *ExecutionStats) Save() error {
	if s == nil || s.filePath == "" {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert internal format to exportable format (exclude samples)
	exportData := make(map[string]interface{})
	for toolName, stat := range s.data {
		exportData[toolName] = map[string]interface{}{
			"runs":      stat.Runs,
			"median_ms": stat.MedianMs,
			"p90_ms":    stat.P90Ms,
		}
	}

	data, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return err
	}

	// Write with secure permissions (0600)
	return os.WriteFile(s.filePath, data, 0600)
}

// Load loads statistics from disk if the file exists.
func (s *ExecutionStats) Load() error {
	if s == nil || s.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		// File doesn't exist yet, which is fine
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var loaded map[string]map[string]interface{}
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for toolName, data := range loaded {
		stat := &toolStat{
			maxSamples: 20,
			samples:    make([]int64, 0, 20),
		}

		if runs, ok := data["runs"].(float64); ok {
			stat.Runs = int(runs)
		}
		if median, ok := data["median_ms"].(float64); ok {
			stat.MedianMs = int64(median)
		}
		if p90, ok := data["p90_ms"].(float64); ok {
			stat.P90Ms = int64(p90)
		}

		s.data[toolName] = stat
	}

	return nil
}

// ExportMetrics exports a human-readable metrics summary to a file.
// Format: TSV with tool name, runs, median ms, p90 ms, and summary.
func (s *ExecutionStats) ExportMetrics(filePath string) error {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var lines []string
	lines = append(lines, "Tool Name\tRuns\tMedian (ms)\tP90 (ms)\tAvg Speed Category")

	// Sort tool names for consistent output
	var toolNames []string
	for toolName := range s.data {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)

	for _, toolName := range toolNames {
		stat := s.data[toolName]
		speedCategory := "fast"
		if stat.MedianMs >= 5000 {
			speedCategory = "slow"
		} else if stat.MedianMs >= 2000 {
			speedCategory = "medium"
		}

		line := fmt.Sprintf("%s\t%d\t%d\t%d\t%s",
			toolName, stat.Runs, stat.MedianMs, stat.P90Ms, speedCategory)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return os.WriteFile(filePath, []byte(content+"\n"), 0600)
}
