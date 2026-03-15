package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// RuleDecision is the outcome of a rule evaluation.
type RuleDecision string

const (
	RuleAllow RuleDecision = "allow"
	RuleAsk   RuleDecision = "ask"
	RuleDeny  RuleDecision = "deny"
)

// PermissionRule matches a tool call and assigns a decision.
// Pattern examples:
//   - "bash"               → matches any bash call
//   - "bash(git status)"   → exact argument match
//   - "bash(git *)"        → glob on first argument
//   - "web_fetch(domain:github.com)" → domain match
type PermissionRule struct {
	Pattern  string       `json:"pattern"`
	Decision RuleDecision `json:"decision"`
	Comment  string       `json:"comment,omitempty"`
}

// RuleSet holds ordered deny/ask/allow rule lists.
// Evaluation order: deny > ask > allow > default.
type RuleSet struct {
	Deny  []PermissionRule `json:"deny"`
	Ask   []PermissionRule `json:"ask"`
	Allow []PermissionRule `json:"allow"`
}

// RuleEngine evaluates tool calls against a RuleSet.
type RuleEngine struct {
	mu        sync.RWMutex
	rules     RuleSet
	rulesPath string
}

// NewRuleEngine creates a RuleEngine that loads/saves from rulesPath.
func NewRuleEngine(configDir string) *RuleEngine {
	re := &RuleEngine{
		rulesPath: filepath.Join(configDir, "rules.json"),
	}
	_ = re.Load() // Ignore error — file may not exist yet
	return re
}

// Load reads the rules file from disk.
func (re *RuleEngine) Load() error {
	data, err := os.ReadFile(re.rulesPath)
	if err != nil {
		return err
	}
	re.mu.Lock()
	defer re.mu.Unlock()
	return json.Unmarshal(data, &re.rules)
}

// Save writes the current rules to disk (0600).
func (re *RuleEngine) Save() error {
	re.mu.RLock()
	data, err := json.MarshalIndent(re.rules, "", "  ")
	re.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(re.rulesPath, data, 0600)
}

// Evaluate checks the rule sets in priority order (deny > ask > allow).
// Returns (decision, matched_rule) where decision is empty string if no rule
// matched (caller should use default logic).
func (re *RuleEngine) Evaluate(toolName string, params map[string]interface{}) (RuleDecision, PermissionRule) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	// Priority: deny first
	for _, rule := range re.rules.Deny {
		if matchRule(rule.Pattern, toolName, params) {
			return RuleDeny, rule
		}
	}
	for _, rule := range re.rules.Ask {
		if matchRule(rule.Pattern, toolName, params) {
			return RuleAsk, rule
		}
	}
	for _, rule := range re.rules.Allow {
		if matchRule(rule.Pattern, toolName, params) {
			return RuleAllow, rule
		}
	}
	return "", PermissionRule{}
}

// AddRule adds a rule to the appropriate list and saves.
func (re *RuleEngine) AddRule(decision RuleDecision, pattern, comment string) error {
	rule := PermissionRule{Pattern: pattern, Decision: decision, Comment: comment}
	re.mu.Lock()
	switch decision {
	case RuleDeny:
		re.rules.Deny = append(re.rules.Deny, rule)
	case RuleAsk:
		re.rules.Ask = append(re.rules.Ask, rule)
	case RuleAllow:
		re.rules.Allow = append(re.rules.Allow, rule)
	default:
		re.mu.Unlock()
		return fmt.Errorf("unknown decision: %s", decision)
	}
	re.mu.Unlock()
	return re.Save()
}

// RemoveRule removes a rule matching the pattern+decision and saves.
func (re *RuleEngine) RemoveRule(decision RuleDecision, pattern string) error {
	re.mu.Lock()
	switch decision {
	case RuleDeny:
		re.rules.Deny = filterRules(re.rules.Deny, pattern)
	case RuleAsk:
		re.rules.Ask = filterRules(re.rules.Ask, pattern)
	case RuleAllow:
		re.rules.Allow = filterRules(re.rules.Allow, pattern)
	}
	re.mu.Unlock()
	return re.Save()
}

// ListAll returns all rules grouped by decision.
func (re *RuleEngine) ListAll() RuleSet {
	re.mu.RLock()
	defer re.mu.RUnlock()
	return re.rules
}

// Format returns a human-readable rule listing.
func (re *RuleEngine) Format() string {
	rs := re.ListAll()
	var sb strings.Builder
	sb.WriteString("# Permission Rules\n\n")
	sb.WriteString("## Deny (highest priority)\n")
	for _, r := range rs.Deny {
		sb.WriteString(fmt.Sprintf("  - `%s`", r.Pattern))
		if r.Comment != "" {
			sb.WriteString(fmt.Sprintf("  _%s_", r.Comment))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Ask\n")
	for _, r := range rs.Ask {
		sb.WriteString(fmt.Sprintf("  - `%s`", r.Pattern))
		if r.Comment != "" {
			sb.WriteString(fmt.Sprintf("  _%s_", r.Comment))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Allow\n")
	for _, r := range rs.Allow {
		sb.WriteString(fmt.Sprintf("  - `%s`", r.Pattern))
		if r.Comment != "" {
			sb.WriteString(fmt.Sprintf("  _%s_", r.Comment))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n**Usage:** `/rules add allow \"bash(git status)\"` or `/rules add deny \"bash(rm -rf*)\"` or `/rules remove allow \"bash(git status)\"`\n")
	return sb.String()
}

// matchRule returns true if the pattern matches the given tool call.
// Pattern forms:
//
//	"tool_name"             → tool name match only
//	"tool_name(arg_glob)"   → tool name + argument glob
//	"tool_name(domain:x)"   → tool name + domain match on url param
func matchRule(pattern, toolName string, params map[string]interface{}) bool {
	// Split pattern into tool part and argument part
	pTool := pattern
	pArg := ""

	if idx := strings.Index(pattern, "("); idx != -1 {
		pTool = pattern[:idx]
		pArg = strings.TrimRight(pattern[idx+1:], ")")
	}

	// Tool name must match (exact)
	if pTool != toolName {
		return false
	}

	// No argument constraint — matches any call to this tool
	if pArg == "" {
		return true
	}

	// Domain match: "web_fetch(domain:github.com)"
	if strings.HasPrefix(pArg, "domain:") {
		domain := strings.TrimPrefix(pArg, "domain:")
		if url, ok := params["url"].(string); ok {
			return strings.Contains(url, domain)
		}
		return false
	}

	// Build a flat "command string" from all string params for glob matching
	cmdStr := flattenParams(params)
	return globMatch(pArg, cmdStr)
}

// flattenParams concatenates all string parameter values separated by spaces.
func flattenParams(params map[string]interface{}) string {
	parts := []string{}
	for _, v := range params {
		if s, ok := v.(string); ok {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// globMatch is a simple glob matcher supporting * as wildcard.
func globMatch(pattern, text string) bool {
	// Exact match
	if pattern == text {
		return true
	}
	// Prefix glob: "git *" matches "git status"
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(text, prefix)
	}
	// Suffix glob: "*.go"
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(text, suffix)
	}
	// Contains check: "*rm*"
	if strings.Contains(pattern, "*") {
		parts := strings.SplitN(pattern, "*", 2)
		if len(parts) == 2 {
			return strings.Contains(text, parts[0]) || strings.Contains(text, parts[1])
		}
	}
	// Substring match for simple patterns
	return strings.Contains(text, pattern)
}

func filterRules(rules []PermissionRule, pattern string) []PermissionRule {
	out := make([]PermissionRule, 0, len(rules))
	for _, r := range rules {
		if r.Pattern != pattern {
			out = append(out, r)
		}
	}
	return out
}
