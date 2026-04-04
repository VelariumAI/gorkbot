package approval

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
)

// ApprovalPattern defines a pattern that can be auto-approved
type ApprovalPattern struct {
	Name        string
	Description string
	Pattern     string                   // glob or regex pattern
	Category    string                   // file_read, tool_call, system_command, etc.
	Priority    int                      // Higher = evaluate first
	Condition   func(target string) bool // Custom condition
}

// ApprovalRule represents a matching rule
type ApprovalRule struct {
	Pattern   *regexp.Regexp
	IsGlob    bool
	Condition func(target string) bool
}

// PatternMatcher evaluates if operations match approval patterns
type PatternMatcher struct {
	logger   *slog.Logger
	patterns map[string]*ApprovalPattern
	rules    map[string]*ApprovalRule
	approved map[string]bool // Cache of approved operations
}

// NewPatternMatcher creates a new pattern matcher
func NewPatternMatcher(logger *slog.Logger) *PatternMatcher {
	if logger == nil {
		logger = slog.Default()
	}

	pm := &PatternMatcher{
		logger:   logger,
		patterns: make(map[string]*ApprovalPattern),
		rules:    make(map[string]*ApprovalRule),
		approved: make(map[string]bool),
	}

	// Register default patterns
	pm.registerDefaultPatterns()

	return pm
}

// registerDefaultPatterns registers built-in approval patterns
func (pm *PatternMatcher) registerDefaultPatterns() {
	patterns := []*ApprovalPattern{
		// File reads from safe directories
		{
			Name:        "read_src_files",
			Description: "Read from source directories",
			Pattern:     "^(src|internal|pkg|lib|vendor|test|tests)/.+\\.(go|py|js|ts|jsx|tsx|rb|java|cpp|c|h|hpp|rs)$",
			Category:    "file_read",
			Priority:    10,
			Condition: func(target string) bool {
				return IsSafeDirectory(target)
			},
		},
		// Configuration file reads
		{
			Name:        "read_config",
			Description: "Read configuration files",
			Pattern:     "^.*\\.(yaml|yml|json|toml|ini|conf|config)$",
			Category:    "file_read",
			Priority:    9,
		},
		// Standard tool calls
		{
			Name:        "tool_list_tools",
			Description: "List available tools",
			Pattern:     "^list_tools$",
			Category:    "tool_call",
			Priority:    10,
		},
		// Git operations in repo
		{
			Name:        "git_status",
			Description: "Check git status",
			Pattern:     "^git_(status|log|diff)$",
			Category:    "tool_call",
			Priority:    9,
		},
		// System information (read-only)
		{
			Name:        "system_info",
			Description: "Get system information",
			Pattern:     "^system_info$",
			Category:    "tool_call",
			Priority:    10,
		},
		// Directory listings
		{
			Name:        "list_directory",
			Description: "List directory contents",
			Pattern:     "^list_directory$",
			Category:    "tool_call",
			Priority:    8,
		},
		// Search operations
		{
			Name:        "search_files",
			Description: "Search files",
			Pattern:     "^(search_files|grep_content)$",
			Category:    "tool_call",
			Priority:    8,
		},
	}

	for _, p := range patterns {
		pm.AddPattern(p)
	}

	pm.logger.Debug("registered default approval patterns", slog.Int("count", len(patterns)))
}

// AddPattern adds a new approval pattern
func (pm *PatternMatcher) AddPattern(pattern *ApprovalPattern) error {
	if pattern.Name == "" {
		return fmt.Errorf("pattern name required")
	}

	// Try to compile as regex
	var regex *regexp.Regexp
	var err error

	// Check if it looks like a glob pattern
	if !isLikelyRegex(pattern.Pattern) && (strings.Contains(pattern.Pattern, "*") || strings.Contains(pattern.Pattern, "?")) {
		// Convert glob to regex
		globRegex := globToRegex(pattern.Pattern)
		regex, err = regexp.Compile(globRegex)
	} else {
		// Treat as regex
		regex, err = regexp.Compile(pattern.Pattern)
	}

	if err != nil {
		return fmt.Errorf("invalid pattern %s: %w", pattern.Name, err)
	}

	pm.patterns[pattern.Name] = pattern
	pm.rules[pattern.Name] = &ApprovalRule{
		Pattern:   regex,
		IsGlob:    strings.Contains(pattern.Pattern, "*"),
		Condition: pattern.Condition,
	}

	pm.logger.Debug("added approval pattern",
		slog.String("name", pattern.Name),
		slog.String("category", pattern.Category),
	)

	return nil
}

// Match checks if target matches any approval patterns
func (pm *PatternMatcher) Match(target string, category string) (bool, string) {
	// Check cache first
	cacheKey := category + ":" + target
	if cached, ok := pm.approved[cacheKey]; ok {
		return cached, ""
	}

	// Find matching patterns
	var matches []*ApprovalPattern
	for name, pattern := range pm.patterns {
		if category != "" && pattern.Category != category {
			continue
		}

		rule := pm.rules[name]
		if rule.Pattern.MatchString(target) {
			// Check custom condition
			if rule.Condition != nil && !rule.Condition(target) {
				continue
			}

			matches = append(matches, pattern)
		}
	}

	// Sort by priority
	sortByPriority(matches)

	if len(matches) > 0 {
		pm.approved[cacheKey] = true
		pm.logger.Debug("operation auto-approved",
			slog.String("target", target),
			slog.String("category", category),
			slog.String("pattern", matches[0].Name),
		)
		return true, matches[0].Name
	}

	pm.approved[cacheKey] = false
	return false, ""
}

// MatchMulti checks if all targets match patterns in their category
func (pm *PatternMatcher) MatchMulti(targets map[string]string) bool {
	for target, category := range targets {
		if matched, _ := pm.Match(target, category); !matched {
			return false
		}
	}
	return true
}

// GetMatchingPatterns returns all patterns matching the target
func (pm *PatternMatcher) GetMatchingPatterns(target string, category string) []*ApprovalPattern {
	var matches []*ApprovalPattern

	for _, pattern := range pm.patterns {
		if category != "" && pattern.Category != category {
			continue
		}

		rule := pm.rules[pattern.Name]
		if rule.Pattern.MatchString(target) {
			if rule.Condition != nil && !rule.Condition(target) {
				continue
			}

			matches = append(matches, pattern)
		}
	}

	sortByPriority(matches)
	return matches
}

// ClearCache clears the approval cache
func (pm *PatternMatcher) ClearCache() {
	pm.approved = make(map[string]bool)
	pm.logger.Debug("cleared approval cache")
}

// GetStats returns matching statistics
func (pm *PatternMatcher) GetStats() map[string]interface{} {
	approved := 0
	denied := 0

	for _, v := range pm.approved {
		if v {
			approved++
		} else {
			denied++
		}
	}

	return map[string]interface{}{
		"patterns": len(pm.patterns),
		"cached":   len(pm.approved),
		"approved": approved,
		"denied":   denied,
	}
}

// SafeDirectories contains safe directories for auto-approval
var SafeDirectories = []string{
	"src",
	"internal",
	"pkg",
	"lib",
	"test",
	"tests",
	"vendor",
	"node_modules",
	"docs",
	"examples",
	".github",
}

// IsSafeDirectory checks if path is in a safe directory
func IsSafeDirectory(path string) bool {
	for _, safeDir := range SafeDirectories {
		if strings.HasPrefix(path, safeDir+"/") || strings.HasPrefix(path, safeDir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// globToRegex converts glob pattern to regex
func globToRegex(glob string) string {
	regex := ""
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				regex += ".*"
				i++
			} else {
				regex += "[^/]*"
			}
		case '?':
			regex += "[^/]"
		case '.':
			regex += "\\."
		case '+', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			regex += "\\" + string(c)
		default:
			regex += string(c)
		}
	}
	return "^" + regex + "$"
}

func isLikelyRegex(pattern string) bool {
	if strings.HasPrefix(pattern, "^") || strings.HasSuffix(pattern, "$") {
		return true
	}
	return strings.ContainsAny(pattern, "[]()|+\\")
}

// sortByPriority sorts patterns by priority (descending)
func sortByPriority(patterns []*ApprovalPattern) {
	for i := 0; i < len(patterns); i++ {
		for j := i + 1; j < len(patterns); j++ {
			if patterns[j].Priority > patterns[i].Priority {
				patterns[i], patterns[j] = patterns[j], patterns[i]
			}
		}
	}
}
