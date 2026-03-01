package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ToolStats tracks statistics for a single tool
type ToolStats struct {
	ToolName       string        `json:"tool_name"`
	ExecutionCount int           `json:"execution_count"`
	SuccessCount   int           `json:"success_count"`
	FailureCount   int           `json:"failure_count"`
	TotalDuration  time.Duration `json:"total_duration"`
	LastUsed       time.Time     `json:"last_used"`
}

// Analytics tracks tool usage statistics
type Analytics struct {
	stats      map[string]*ToolStats
	configPath string
	mu         sync.RWMutex
}

// AnalyticsConfig is the persistent storage format
type AnalyticsConfig struct {
	Stats   map[string]*ToolStats `json:"stats"`
	Version string                `json:"version"`
}

// NewAnalytics creates a new analytics tracker
func NewAnalytics(configDir string) (*Analytics, error) {
	configPath := filepath.Join(configDir, "tool_analytics.json")

	a := &Analytics{
		stats:      make(map[string]*ToolStats),
		configPath: configPath,
	}

	// Load existing analytics
	if err := a.load(); err != nil {
		// If file doesn't exist, that's okay
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load analytics: %w", err)
		}
	}

	return a, nil
}

// RecordExecution records a tool execution
func (a *Analytics) RecordExecution(toolName string, success bool, duration time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	stats, exists := a.stats[toolName]
	if !exists {
		stats = &ToolStats{
			ToolName: toolName,
		}
		a.stats[toolName] = stats
	}

	stats.ExecutionCount++
	if success {
		stats.SuccessCount++
	} else {
		stats.FailureCount++
	}
	stats.TotalDuration += duration
	stats.LastUsed = time.Now()

	return a.save()
}

// GetStats returns statistics for a specific tool
func (a *Analytics) GetStats(toolName string) *ToolStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if stats, exists := a.stats[toolName]; exists {
		// Return a copy
		statsCopy := *stats
		return &statsCopy
	}
	return nil
}

// GetAllStats returns all tool statistics
func (a *Analytics) GetAllStats() map[string]*ToolStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Return copies
	result := make(map[string]*ToolStats, len(a.stats))
	for name, stats := range a.stats {
		statsCopy := *stats
		result[name] = &statsCopy
	}
	return result
}

// GetTopTools returns the N most used tools
func (a *Analytics) GetTopTools(n int) []*ToolStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.getTopToolsLocked(n)
}

// getTopToolsLocked is the lock-free inner implementation.
// Must be called with a.mu held (read or write).
func (a *Analytics) getTopToolsLocked(n int) []*ToolStats {
	statsList := make([]*ToolStats, 0, len(a.stats))
	for _, stats := range a.stats {
		statsCopy := *stats
		statsList = append(statsList, &statsCopy)
	}

	// Sort by execution count (bubble sort for simplicity)
	for i := 0; i < len(statsList); i++ {
		for j := i + 1; j < len(statsList); j++ {
			if statsList[i].ExecutionCount < statsList[j].ExecutionCount {
				statsList[i], statsList[j] = statsList[j], statsList[i]
			}
		}
	}

	if n > len(statsList) {
		n = len(statsList)
	}
	return statsList[:n]
}

// GetSuccessRate returns the success rate for a tool (0-1)
func (a *Analytics) GetSuccessRate(toolName string) float64 {
	stats := a.GetStats(toolName)
	if stats == nil || stats.ExecutionCount == 0 {
		return 0.0
	}
	return float64(stats.SuccessCount) / float64(stats.ExecutionCount)
}

// GetAverageDuration returns the average execution duration for a tool
func (a *Analytics) GetAverageDuration(toolName string) time.Duration {
	stats := a.GetStats(toolName)
	if stats == nil || stats.ExecutionCount == 0 {
		return 0
	}
	return stats.TotalDuration / time.Duration(stats.ExecutionCount)
}

// Reset resets all analytics
func (a *Analytics) Reset() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.stats = make(map[string]*ToolStats)
	return a.save()
}

// GetSummary returns a formatted summary of all analytics
func (a *Analytics) GetSummary() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.stats) == 0 {
		return "No tool usage data available."
	}

	var summary string
	summary += fmt.Sprintf("Tool Usage Analytics\n")
	summary += fmt.Sprintf("===================\n\n")

	// Total executions
	totalExec := 0
	totalSuccess := 0
	totalFailure := 0
	for _, stats := range a.stats {
		totalExec += stats.ExecutionCount
		totalSuccess += stats.SuccessCount
		totalFailure += stats.FailureCount
	}

	summary += fmt.Sprintf("Total Executions: %d\n", totalExec)
	summary += fmt.Sprintf("  Success: %d (%.1f%%)\n", totalSuccess, float64(totalSuccess)/float64(totalExec)*100)
	summary += fmt.Sprintf("  Failure: %d (%.1f%%)\n\n", totalFailure, float64(totalFailure)/float64(totalExec)*100)

	// Top tools (call the unlocked helper — we already hold a.mu.RLock)
	topTools := a.getTopToolsLocked(10)
	summary += "Top 10 Most Used Tools:\n"
	summary += "-----------------------\n"
	for i, stats := range topTools {
		avgDuration := stats.TotalDuration / time.Duration(stats.ExecutionCount)
		successRate := float64(stats.SuccessCount) / float64(stats.ExecutionCount) * 100
		summary += fmt.Sprintf("%2d. %-20s  Executions: %4d  Success: %5.1f%%  Avg Time: %v\n",
			i+1, stats.ToolName, stats.ExecutionCount, successRate, avgDuration.Round(time.Millisecond))
	}

	return summary
}

// load reads analytics from disk
func (a *Analytics) load() error {
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		return err
	}

	var config AnalyticsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse analytics: %w", err)
	}

	a.stats = config.Stats
	return nil
}

// save writes analytics to disk
func (a *Analytics) save() error {
	config := AnalyticsConfig{
		Stats:   a.stats,
		Version: "1.0",
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal analytics: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(a.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write with secure permissions
	if err := os.WriteFile(a.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write analytics: %w", err)
	}

	return nil
}

// GetConfigPath returns the path to the analytics file
func (a *Analytics) GetConfigPath() string {
	return a.configPath
}
