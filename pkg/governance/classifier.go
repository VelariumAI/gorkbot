package governance

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ClassifyTool returns deterministic risk class for a tool invocation.
func ClassifyTool(toolName string, params map[string]any) RiskClass {
	name := strings.ToLower(strings.TrimSpace(toolName))

	if (name == "web_fetch" || name == "download_file") && isPrivateTarget(getString(params, "url")) {
		return RISK_PRIVILEGED_BRIDGE
	}

	switch name {
	case "read_file", "list_directory", "file_info", "git_status", "git_diff", "git_log", "system_info", "disk_usage", "list_tools", "tool_info", "context_stats", "puter.fs.read", "puter.kv.get", "puter.app.preview":
		return RISK_READ_ONLY
	case "write_file", "edit_file", "download_file", "git_commit", "puter.fs.write", "puter.kv.set":
		return RISK_LOCAL_MUTATION
	case "delete_file", "kill_process", "stop_managed_process", "puter.fs.delete", "puter.kv.delete":
		return RISK_DESTRUCTIVE
	case "bash", "structured_bash", "code_exec", "python_sandbox", "jupyter", "browser_control", "android_control", "android_system", "adb_setup", "spawn_agent", "spawn_subagent", "run_pipeline", "puter.bridge.host", "puter.auth.request":
		return RISK_PRIVILEGED_BRIDGE
	case "create_tool", "modify_tool", "define_command", "rebuild":
		return RISK_SELF_MODIFICATION
	case "sense_evolve":
		dryRun, _ := params["dry_run"].(bool)
		if dryRun {
			return RISK_READ_ONLY
		}
		return RISK_SELF_MODIFICATION
	case "git_push", "post_notify", "webhook", "puter.hosting.publish", "puter.network.fetch":
		return RISK_EXTERNAL_SIDE_EFFECT
	case "http_request":
		return classifyHTTPRequest(params)
	}

	if strings.Contains(name, "service_control") {
		action := strings.ToLower(getString(params, "action"))
		if action == "stop" || action == "restart" || action == "disable" {
			return RISK_DESTRUCTIVE
		}
	}

	if strings.Contains(name, "send") || strings.Contains(name, "email") || strings.Contains(name, "message") || strings.Contains(name, "publish") {
		return RISK_EXTERNAL_SIDE_EFFECT
	}

	if strings.Contains(name, "delete") || strings.Contains(name, "remove") || strings.Contains(name, "rm") {
		return RISK_DESTRUCTIVE
	}

	if strings.Contains(name, "dynamic") && (strings.Contains(name, "loader") || strings.Contains(name, "updater")) {
		return RISK_SELF_MODIFICATION
	}
	if strings.Contains(name, "worktree") && (strings.Contains(name, "create") || strings.Contains(name, "delete") || strings.Contains(name, "move")) {
		return RISK_SELF_MODIFICATION
	}

	return RISK_UNKNOWN
}

func classifyHTTPRequest(params map[string]any) RiskClass {
	method := strings.ToUpper(strings.TrimSpace(getString(params, "method")))
	if method == "" {
		method = "GET"
	}

	if method == "GET" || method == "HEAD" {
		if hasAny(params, "credentials", "credential", "auth", "authorization", "token", "body", "api_key", "password", "secret") {
			return RISK_PRIVILEGED_BRIDGE
		}
		if hasCredentialHeaders(params) {
			return RISK_PRIVILEGED_BRIDGE
		}
		if isPrivateTarget(getString(params, "url")) {
			return RISK_PRIVILEGED_BRIDGE
		}
		return RISK_READ_ONLY
	}

	if isPrivateTarget(getString(params, "url")) {
		return RISK_PRIVILEGED_BRIDGE
	}
	return RISK_EXTERNAL_SIDE_EFFECT
}

func hasCredentialHeaders(params map[string]any) bool {
	if params == nil {
		return false
	}
	headersRaw, ok := params["headers"]
	if !ok || headersRaw == nil {
		return false
	}
	headers := map[string]any{}
	switch t := headersRaw.(type) {
	case map[string]any:
		headers = t
	default:
		return false
	}
	for k := range headers {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "authorization" || key == "cookie" || strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "api-key") {
			return true
		}
	}
	return false
}

func getString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	if v, ok := params[key]; ok {
		switch t := v.(type) {
		case string:
			return t
		case fmt.Stringer:
			return t.String()
		}
	}
	return ""
}

func hasAny(params map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := params[k]; ok {
			return true
		}
	}
	return false
}

func isPrivateTarget(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return false
	}
	// Strip trailing dots (FQDN form: "localhost." == "localhost").
	host := strings.TrimRight(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	if host == "169.254.169.254" {
		return true
	}
	// Catch abbreviated loopback forms like "127.1" that net.ParseIP rejects.
	if strings.HasPrefix(host, "127.") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.To4() == nil {
		// Basic ULA check fc00::/7
		if len(ip) == net.IPv6len {
			first := ip[0]
			if first&0xfe == 0xfc {
				return true
			}
		}
	}
	if port := u.Port(); port != "" {
		if p, err := strconv.Atoi(port); err == nil && p == 0 {
			return true
		}
	}
	return false
}
