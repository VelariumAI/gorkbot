package sense

import (
	"regexp"
	"strings"
)

// OutputFilter provides intelligent suppression of internal system messages
// while preserving user-facing content. It supports multiple suppression
// categories with configurable verbosity levels.
type OutputFilter struct {
	config OutputFilterConfig
	rules  []*suppressionRule
}

// OutputFilterConfig controls which message categories are suppressed
type OutputFilterConfig struct {
	Verbose                bool // Show all internal narration (overrides all suppressions)
	SuppressToolNarration  bool // "I'm running...", "Executing...", etc.
	SuppressToolStatus     bool // Tool status updates, progress, loading messages
	SuppressInternalReason bool // "Here's my thought process", step-by-step reasoning without code
	SuppressDebugInfo      bool // Parameter validation, cache hits/misses
	SuppressSystemStatus   bool // System info updates, health checks
	SuppressCooldownNotice bool // Rate limit and cooldown notices
	MinimumOutputLength    int  // Collapse responses shorter than N characters (0 = disabled)
}

// SuppressCategory identifies which category a line belongs to
type SuppressCategory int

const (
	CategoryToolNarration SuppressCategory = iota
	CategoryToolStatus
	CategoryInternalReason
	CategoryDebugInfo
	CategorySystemStatus
	CategoryCooldownNotice
	CategoryUnknown
)

// suppressionRule defines a pattern-based suppression rule
type suppressionRule struct {
	Name              string
	Regex             *regexp.Regexp
	Category          SuppressCategory
	RequireOwnLine    bool // Must be the only content on its line
	RequireNewline    bool // Must be followed by newline
	MultilineMatch    bool // Can match across lines
	PreserveIfVerbose bool // Preserve even in filtered mode (for critical info)
}

// NewOutputFilter creates a new output filter with the given configuration
func NewOutputFilter(config OutputFilterConfig) (*OutputFilter, error) {
	f := &OutputFilter{
		config: config,
		rules:  []*suppressionRule{},
	}

	// Initialize all suppression rules
	if err := f.initializeRules(); err != nil {
		return nil, err
	}

	return f, nil
}

// initializeRules sets up all pattern-based suppression rules
func (f *OutputFilter) initializeRules() error {
	rules := []*suppressionRule{
		// TOOL NARRATION RULES
		{
			Name:           "tool_narration_intro",
			Regex:          regexp.MustCompile(`(?i)I'?m\s+(running|executing|calling|using|invoking|spawning)\s+the?\s+(\w+)\s+tool`),
			Category:       CategoryToolNarration,
			RequireOwnLine: false,
		},
		{
			Name:           "tool_narration_generic",
			Regex:          regexp.MustCompile(`(?i)(running|executing|using|invoking|calling|launching)\s+.*?\s+(tool|command|operation)`),
			Category:       CategoryToolNarration,
			RequireOwnLine: false,
		},
		{
			Name:           "tool_exec_msg",
			Regex:          regexp.MustCompile(`(?i)tool\s+execution:|executing.*?tool`),
			Category:       CategoryToolNarration,
			RequireOwnLine: false,
		},
		{
			Name:           "to_address_request",
			Regex:          regexp.MustCompile(`(?i)to\s+(address|handle|complete|answer|respond|check)\s+your\s+(request|question|query)`),
			Category:       CategoryToolNarration,
			RequireOwnLine: false,
		},
		{
			Name:           "comprehensive_check",
			Regex:          regexp.MustCompile(`(?i)I'?ll\s+run.*?to\s+(get|fetch|retrieve|check|analyze)`),
			Category:       CategoryToolNarration,
			RequireOwnLine: false,
		},
		{
			Name:           "let_me_phrase",
			Regex:          regexp.MustCompile(`(?i)(let me|I'?ll|I will|I want to).{1,50}(run|use|invoke|call|execute).*?(tool|command)`),
			Category:       CategoryToolNarration,
			RequireOwnLine: false,
		},

		// TOOL STATUS RULES
		{
			Name:           "cooldown_notice",
			Regex:          regexp.MustCompile(`(?i)note:\s+this\s+tool\s+has\s+a\s+.*?cooldown`),
			Category:       CategoryToolStatus,
			RequireOwnLine: false,
		},
		{
			Name:           "tool_loading",
			Regex:          regexp.MustCompile(`(?i)tool\s+\w+\s+is\s+(loading|executing|running|waiting)`),
			Category:       CategoryToolStatus,
			RequireOwnLine: false,
		},
		{
			Name:           "tool_waiting",
			Regex:          regexp.MustCompile(`(?i)(waiting\s+for|checking\s+if|monitoring)\s+tool`),
			Category:       CategoryToolStatus,
			RequireOwnLine: false,
		},
		{
			Name:           "tool_status_generic",
			Regex:          regexp.MustCompile(`(?i)tool\s+status:|tool\s+is\s+(ready|busy|idle|active)`),
			Category:       CategoryToolStatus,
			RequireOwnLine: false,
		},
		{
			Name:           "cache_notice",
			Regex:          regexp.MustCompile(`(?i)(caching|cached|cache\s+hit|using\s+cached)`),
			Category:       CategoryDebugInfo,
			RequireOwnLine: false,
		},

		// INTERNAL REASONING RULES
		{
			Name:           "thought_process",
			Regex:          regexp.MustCompile(`(?i)(here'?s my thought|let me think|my reasoning|let me break this down|my approach)`),
			Category:       CategoryInternalReason,
			RequireOwnLine: false,
		},
		{
			Name:           "step_by_step",
			Regex:          regexp.MustCompile(`(?i)(first,|second,|third,|next,|finally,|then|step\s+\d+:|proceed\s+as\s+follows)`),
			Category:       CategoryInternalReason,
			RequireOwnLine: false,
		},
		{
			Name:           "numbered_steps",
			Regex:          regexp.MustCompile(`^\d+\.\s+[^`+"`"+`]+$`),
			Category:       CategoryInternalReason,
			RequireOwnLine: true,
		},
		{
			Name:           "approach_msg",
			Regex:          regexp.MustCompile(`(?i)(my approach|this approach|my strategy|here'?s what|let me explain)`),
			Category:       CategoryInternalReason,
			RequireOwnLine: false,
		},

		// DEBUG INFO RULES
		{
			Name:           "param_validation",
			Regex:          regexp.MustCompile(`(?i)(parameter|param|argument).*?(validation|check|verified|valid)`),
			Category:       CategoryDebugInfo,
			RequireOwnLine: false,
		},
		{
			Name:           "schema_validation",
			Regex:          regexp.MustCompile(`(?i)(schema|type|format).*?(valid|match|correct)`),
			Category:       CategoryDebugInfo,
			RequireOwnLine: false,
		},

		// SYSTEM STATUS RULES
		{
			Name:           "system_health",
			Regex:          regexp.MustCompile(`(?i)(system status|system.*?healthy|all systems|system check)`),
			Category:       CategorySystemStatus,
			RequireOwnLine: false,
		},
		{
			Name:           "resources_available",
			Regex:          regexp.MustCompile(`(?i)(no errors|all.*?ok|everything.*?fine|ready to)`),
			Category:       CategorySystemStatus,
			RequireOwnLine: false,
		},
		{
			Name:           "tools_available",
			Regex:          regexp.MustCompile(`(?i)tools?\s+(available|ready|enabled):`),
			Category:       CategorySystemStatus,
			RequireOwnLine: false,
		},

		// COOLDOWN/RATE LIMIT NOTICES
		{
			Name:           "rate_limited",
			Regex:          regexp.MustCompile(`(?i)(rate limited|rate-limited|throttled|waiting.*?cooldown|retry after)`),
			Category:       CategoryCooldownNotice,
			RequireOwnLine: false,
		},
		{
			Name:           "cooldown_active",
			Regex:          regexp.MustCompile(`(?i)cooldown\s+(active|enforced|in effect)`),
			Category:       CategoryCooldownNotice,
			RequireOwnLine: false,
		},
		{
			Name:           "next_available",
			Regex:          regexp.MustCompile(`(?i)(next execution|next check|available in).*?minute`),
			Category:       CategoryCooldownNotice,
			RequireOwnLine: false,
		},
	}

	f.rules = rules
	return nil
}

// Filter applies suppression rules to the input text and returns filtered output
func (f *OutputFilter) Filter(text string) string {
	if f.config.Verbose {
		return text
	}

	lines := strings.Split(text, "\n")
	var filtered []string

	for _, line := range lines {
		if f.shouldSuppress(line) {
			continue
		}
		filtered = append(filtered, line)
	}

	result := strings.Join(filtered, "\n")

	// Clean up multiple consecutive blank lines
	result = regexp.MustCompile(`\n\n\n+`).ReplaceAllString(result, "\n\n")

	// Trim leading/trailing whitespace per line but preserve structure
	var cleanLines []string
	for _, line := range strings.Split(result, "\n") {
		// Don't trim internal whitespace (for code blocks)
		trimmed := strings.TrimRight(line, " \t")
		cleanLines = append(cleanLines, trimmed)
	}
	result = strings.Join(cleanLines, "\n")

	result = strings.TrimSpace(result)

	// Check minimum output length
	if f.config.MinimumOutputLength > 0 && len(result) < f.config.MinimumOutputLength {
		return ""
	}

	return result
}

// shouldSuppress checks if a line should be suppressed based on active rules
func (f *OutputFilter) shouldSuppress(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Never suppress empty lines (they're structural)
	if trimmed == "" {
		return false
	}

	for _, rule := range f.rules {
		if !f.isCategoryActive(rule.Category) {
			continue
		}

		if rule.Regex.MatchString(line) {
			if rule.RequireOwnLine {
				// Check if this is the only content on the line
				if trimmed != line {
					continue // Line has other content
				}
			}
			return true
		}
	}

	return false
}

// isCategoryActive checks if a suppression category is enabled
func (f *OutputFilter) isCategoryActive(category SuppressCategory) bool {
	switch category {
	case CategoryToolNarration:
		return f.config.SuppressToolNarration
	case CategoryToolStatus:
		return f.config.SuppressToolStatus
	case CategoryInternalReason:
		return f.config.SuppressInternalReason
	case CategoryDebugInfo:
		return f.config.SuppressDebugInfo
	case CategorySystemStatus:
		return f.config.SuppressSystemStatus
	case CategoryCooldownNotice:
		return f.config.SuppressCooldownNotice
	default:
		return false
	}
}

// ClassifyLine identifies which category a line belongs to
func (f *OutputFilter) ClassifyLine(line string) SuppressCategory {
	for _, rule := range f.rules {
		if rule.Regex.MatchString(line) {
			return rule.Category
		}
	}
	return CategoryUnknown
}

// FilteredConfig returns the current filter configuration
func (f *OutputFilter) FilteredConfig() OutputFilterConfig {
	return f.config
}

// SetVerbose updates the verbose flag
func (f *OutputFilter) SetVerbose(verbose bool) {
	f.config.Verbose = verbose
}

// CreateDefaultConfig returns a sensible default filter configuration
func CreateDefaultConfig() OutputFilterConfig {
	return OutputFilterConfig{
		Verbose:                false,
		SuppressToolNarration:  true,
		SuppressToolStatus:     true,
		SuppressInternalReason: true,
		SuppressDebugInfo:      true,
		SuppressSystemStatus:   true,
		SuppressCooldownNotice: true,
		MinimumOutputLength:    0,
	}
}

// CreateVerboseConfig returns a config that shows everything
func CreateVerboseConfig() OutputFilterConfig {
	return OutputFilterConfig{
		Verbose:                true,
		SuppressToolNarration:  false,
		SuppressToolStatus:     false,
		SuppressInternalReason: false,
		SuppressDebugInfo:      false,
		SuppressSystemStatus:   false,
		SuppressCooldownNotice: false,
		MinimumOutputLength:    0,
	}
}
