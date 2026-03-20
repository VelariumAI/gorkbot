package hitl

import (
	"fmt"
	"strings"
)

// RiskLevel represents the risk classification of a tool execution
type RiskLevel int

const (
	RiskLow      RiskLevel = 1
	RiskMedium   RiskLevel = 2
	RiskHigh     RiskLevel = 3
	RiskCritical RiskLevel = 4
)

// String returns the string representation of a risk level
func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// RiskClassifier evaluates the risk level of tool executions based on
// tool type, parameters, and operation context.
type RiskClassifier struct {
	// Hardcoded tool risk mappings
	toolRiskMap map[string]RiskLevel
	// Dangerous keywords in bash commands
	dangerousKeywords []string
	// Sensitive path patterns
	sensitivePaths []string
	// Dangerous parameter patterns
	dangerousPatterns map[string][]string
}

// NewRiskClassifier creates a new risk classifier with comprehensive rules
func NewRiskClassifier() *RiskClassifier {
	rc := &RiskClassifier{
		toolRiskMap: make(map[string]RiskLevel),
		dangerousKeywords: []string{
			"rm", "dd", "mkfs", "fdisk", "parted",
			"chmod", "chown", "chgrp",
			"sudo", "su", "root",
			"fork", "exec", "trap",
			"iptables", "ufw", "firewall",
		},
		sensitivePaths: []string{
			"/root", "/etc", "/sys", "/proc", "/boot",
			"/.ssh", "/.aws", "/.gnupg", "/.config/",
			".env", "secret", "credential", "key",
			"password", "token", "api_key",
		},
		dangerousPatterns: make(map[string][]string),
	}

	// Initialize tool risk mappings
	rc.initializeToolRiskMap()
	rc.initializeDangerousPatterns()

	return rc
}

// initializeToolRiskMap sets up the default tool risk levels
func (rc *RiskClassifier) initializeToolRiskMap() {
	// CRITICAL RISK TOOLS
	criticalTools := []string{
		"bash", "shell", "execute", "exec", "run_command",
		"delete_file", "remove_file", "unlink",
		"git_push", "git_force_push", "git_reset_hard",
		"pkg_install", "package_install", "apt_install", "brew_install",
		"chmod", "chown", "setfacl", "setcap",
		"dd", "fdisk", "mkfs", "parted",
		"http_request", // Non-GET methods
		"create_tool", "modify_tool", "delete_tool",
		"system_reboot", "system_shutdown", "poweroff",
		"docker_exec", "container_exec",
		"database_migrate", "database_drop", "database_delete",
		"credential_store", "secret_set",
	}
	for _, tool := range criticalTools {
		rc.toolRiskMap[tool] = RiskCritical
	}

	// HIGH RISK TOOLS
	highTools := []string{
		"git_commit", "git_rebase", "git_reset",
		"write_file", "create_file", "append_file",
		"http_request_post", "http_request_put", "http_request_delete",
		"spawn_agent", // Especially redteam agents
		"system_monitor", // System introspection
		"process_kill", "process_terminate",
		"service_restart", "service_stop",
		"file_move", "file_rename",
		"symlink_create",
		"execute_python", "execute_ruby", "execute_node",
		"ssh_command", "ssh_exec",
		"curl_request", "wget_request",
		"notification_send", // Could leak info
	}
	for _, tool := range highTools {
		rc.toolRiskMap[tool] = RiskHigh
	}

	// MEDIUM RISK TOOLS
	mediumTools := []string{
		"git_pull", "git_merge", "git_fetch", "git_rebase_interactive",
		"git_config", "git_remote",
		"list_directory", "list_files",
		"find_files", "grep_content",
		"http_request_get", // External URLs
		"web_fetch", "web_scrape",
		"system_info", "list_processes", "disk_usage",
		"text_replace", "text_search",
		"archive_create", "archive_extract",
		"mail_send", "email_send",
		"calendar_event", "calendar_create",
	}
	for _, tool := range mediumTools {
		rc.toolRiskMap[tool] = RiskMedium
	}

	// LOW RISK TOOLS
	lowTools := []string{
		"read_file", "cat_file", "head_file", "tail_file",
		"git_log", "git_status", "git_diff", "git_show",
		"list_tools", "tool_info", "help",
		"version", "echo", "print",
		"get_env", "get_var",
		"time", "date", "uptime",
		"calculator", "math",
		"uuid_generate", "hash_generate",
	}
	for _, tool := range lowTools {
		rc.toolRiskMap[tool] = RiskLow
	}
}

// initializeDangerousPatterns sets up parameter-specific danger patterns
func (rc *RiskClassifier) initializeDangerousPatterns() {
	rc.dangerousPatterns["bash"] = []string{
		"rm -rf /",
		"dd if=/dev/zero",
		"mkfs",
		"fork();",
		": () { :|:& };:",
		"$()",
		"`",
		"&& rm",
		"| rm",
		"|| rm",
		"> /dev/sda",
		"chmod -R 777",
		"chmod -R 000",
	}

	rc.dangerousPatterns["write_file"] = []string{
		".env",
		"~/.ssh/",
		"/etc/",
		"/root/",
		"/sys/",
	}

	rc.dangerousPatterns["http_request"] = []string{
		"exfiltrate",
		"steal",
		"backdoor",
		"malware",
	}
}

// ClassifyTool evaluates the risk level of a tool execution.
// Returns the risk level and a human-readable reason.
func (rc *RiskClassifier) ClassifyTool(toolName string, params map[string]interface{}) (RiskLevel, string) {
	// Normalize tool name
	normalizedName := strings.ToLower(strings.TrimSpace(toolName))

	// Get base risk level
	baseRisk := rc.getBaseToolRisk(normalizedName)

	// Analyze parameters for risk escalation
	escalation := rc.analyzeParametersForRisk(normalizedName, params)

	finalRisk := baseRisk + escalation
	if finalRisk > RiskCritical {
		finalRisk = RiskCritical
	}

	reason := rc.buildRiskReason(normalizedName, baseRisk, escalation, params)

	return finalRisk, reason
}

// getBaseToolRisk returns the base risk level for a tool
func (rc *RiskClassifier) getBaseToolRisk(toolName string) RiskLevel {
	// Exact match first
	if risk, ok := rc.toolRiskMap[toolName]; ok {
		return risk
	}

	// Substring matching (allow flexibility in naming)
	for mappedTool, risk := range rc.toolRiskMap {
		if strings.Contains(toolName, mappedTool) || strings.Contains(mappedTool, toolName) {
			return risk
		}
	}

	// Default to medium if unknown
	return RiskMedium
}

// analyzeParametersForRisk evaluates parameters for risk escalation
func (rc *RiskClassifier) analyzeParametersForRisk(toolName string, params map[string]interface{}) RiskLevel {
	escalation := RiskLow

	for key, value := range params {
		strValue := fmt.Sprintf("%v", value)

		// Check for sensitive paths
		if rc.containsSensitivePattern(strValue) {
			escalation += RiskHigh
		}

		// Check tool-specific dangerous patterns
		if patterns, ok := rc.dangerousPatterns[toolName]; ok {
			for _, pattern := range patterns {
				if strings.Contains(strings.ToLower(strValue), strings.ToLower(pattern)) {
					escalation += RiskHigh
				}
			}
		}

		// Check for dangerous bash keywords
		if toolName == "bash" || toolName == "execute" || toolName == "shell" {
			if rc.containsDangerousKeyword(strValue) {
				escalation += RiskMedium
			}
		}

		// Check for credential/secret patterns
		if rc.containsCredentialPattern(key, strValue) {
			escalation += RiskMedium
		}

		// Check parameter key for hints
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "path") || strings.Contains(lowerKey, "file") {
			if rc.containsSensitivePattern(strValue) {
				escalation += RiskMedium
			}
		}
	}

	return escalation
}

// containsSensitivePattern checks if a value contains sensitive path patterns
func (rc *RiskClassifier) containsSensitivePattern(value string) bool {
	lowerValue := strings.ToLower(value)
	for _, pattern := range rc.sensitivePaths {
		if strings.Contains(lowerValue, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// containsDangerousKeyword checks if a value contains dangerous bash keywords
func (rc *RiskClassifier) containsDangerousKeyword(value string) bool {
	lowerValue := strings.ToLower(value)
	for _, keyword := range rc.dangerousKeywords {
		if strings.Contains(lowerValue, keyword) {
			return true
		}
	}
	return false
}

// containsCredentialPattern checks if a value looks like credentials
func (rc *RiskClassifier) containsCredentialPattern(key, value string) bool {
	lowerKey := strings.ToLower(key)
	credentialPatterns := []string{
		"password", "passwd", "pwd",
		"secret", "key", "token", "auth",
		"credential", "credentials",
		"api_key", "apikey", "api-key",
		"oauth", "bearer", "jwt",
		"private", "priv",
	}

	for _, pattern := range credentialPatterns {
		if strings.Contains(lowerKey, pattern) {
			return true
		}
	}

	// Check for common credential formats in value
	if len(value) > 20 {
		// Looks like a token/key
		if strings.Contains(lowerKey, "key") || strings.Contains(lowerKey, "token") {
			return true
		}
	}

	return false
}

// buildRiskReason constructs a human-readable explanation of the risk classification
func (rc *RiskClassifier) buildRiskReason(toolName string, baseRisk, escalation RiskLevel, params map[string]interface{}) string {
	var reasons []string

	switch baseRisk {
	case RiskCritical:
		reasons = append(reasons, fmt.Sprintf("Tool '%s' is inherently high-risk (destructive/privileged operation)", toolName))
	case RiskHigh:
		reasons = append(reasons, fmt.Sprintf("Tool '%s' can modify system state or access sensitive data", toolName))
	case RiskMedium:
		reasons = append(reasons, fmt.Sprintf("Tool '%s' performs external operations or system inspection", toolName))
	case RiskLow:
		reasons = append(reasons, fmt.Sprintf("Tool '%s' is read-only or informational", toolName))
	}

	if escalation > 0 {
		if escalation >= RiskHigh {
			reasons = append(reasons, "Parameters contain sensitive paths or dangerous patterns (+high escalation)")
		} else if escalation >= RiskMedium {
			reasons = append(reasons, "Parameters may contain credentials or unusual patterns (+medium escalation)")
		}
	}

	// Check for specific risky parameters
	for key, value := range params {
		strValue := fmt.Sprintf("%v", value)
		if rc.containsSensitivePattern(strValue) {
			reasons = append(reasons, fmt.Sprintf("Parameter '%s' targets sensitive system areas", key))
		}
	}

	if len(reasons) == 0 {
		return "Standard tool execution"
	}

	return strings.Join(reasons, "; ")
}

// GetRiskColor returns an ANSI color code for the risk level
func (r RiskLevel) Color() string {
	switch r {
	case RiskLow:
		return "\033[32m" // Green
	case RiskMedium:
		return "\033[33m" // Yellow
	case RiskHigh:
		return "\033[31m" // Red
	case RiskCritical:
		return "\033[35m" // Magenta
	default:
		return "\033[0m" // Reset
	}
}

// GetRiskSymbol returns a symbolic representation of the risk level
func (r RiskLevel) Symbol() string {
	switch r {
	case RiskLow:
		return "●"
	case RiskMedium:
		return "●●"
	case RiskHigh:
		return "●●●"
	case RiskCritical:
		return "●●●●"
	default:
		return "?"
	}
}
