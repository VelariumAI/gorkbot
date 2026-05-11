package governance

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/execution"
)

// ApprovalCache stores in-memory approval reuse state for a session.
type ApprovalCache struct {
	mu      sync.Mutex
	session map[string]ApprovalResult
	always  map[string]ApprovalResult
	never   map[string]ApprovalResult
}

// NewApprovalCache creates an empty cache.
func NewApprovalCache() *ApprovalCache {
	return &ApprovalCache{
		session: make(map[string]ApprovalResult),
		always:  make(map[string]ApprovalResult),
		never:   make(map[string]ApprovalResult),
	}
}

// Key creates a deterministic cache key for an action.
func (c *ApprovalCache) Key(action GovernedAction) string {
	return approvalKeyFromAction(action)
}

func approvalKeyFromAction(action GovernedAction) string {
	redacted := RedactParams(action.Parameters)
	return strings.ToLower(strings.TrimSpace(action.ToolName)) + "|" +
		strings.ToLower(strings.TrimSpace(action.Capability)) + "|" +
		string(action.RiskClass) + "|" + hashAnyMap(redacted)
}

// Get returns cached decision precedence: never > always > session.
func (c *ApprovalCache) Get(action GovernedAction) (ApprovalResult, bool) {
	if c == nil {
		return ApprovalResult{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.Key(action)
	if res, ok := c.never[key]; ok {
		return res, true
	}
	if res, ok := c.always[key]; ok {
		return res, true
	}
	if res, ok := c.session[key]; ok {
		return res, true
	}
	return ApprovalResult{}, false
}

// Put stores approval result according to its scope.
func (c *ApprovalCache) Put(action GovernedAction, result ApprovalResult) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.Key(action)
	switch result.Scope {
	case APPROVAL_SESSION:
		if ApprovalGranted(result) {
			c.session[key] = result
		}
	case APPROVAL_ALWAYS:
		if ApprovalGranted(result) {
			c.always[key] = result
		}
	case APPROVAL_NEVER:
		res := result
		if res.Decision == "" {
			res.Decision = APPROVAL_DENIED
		}
		c.never[key] = res
	}
}

// ClearSession clears session-scoped approvals.
func (c *ApprovalCache) ClearSession() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = make(map[string]ApprovalResult)
}

// ClearAll clears all in-memory cache state.
func (c *ApprovalCache) ClearAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = make(map[string]ApprovalResult)
	c.always = make(map[string]ApprovalResult)
	c.never = make(map[string]ApprovalResult)
}

func hashAnyMap(in map[string]any) string {
	if in == nil {
		return execution.HashString("null")
	}
	m := make(map[string]interface{}, len(in))
	for k, v := range in {
		m[k] = normalizeAny(v)
	}
	return execution.HashParams(m)
}

func normalizeAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]interface{}, len(t))
		for k, v2 := range t {
			out[k] = normalizeAny(v2)
		}
		return out
	case []any:
		out := make([]interface{}, len(t))
		for i := range t {
			out[i] = normalizeAny(t[i])
		}
		return out
	default:
		return t
	}
}

// RedactParams removes secret-like keys recursively.
func RedactParams(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		if isSecretKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			out[k] = RedactParams(t)
		case []any:
			arr := make([]any, len(t))
			for i := range t {
				arr[i] = redactAnyValue(t[i])
			}
			out[k] = arr
		default:
			out[k] = v
		}
	}
	return out
}

func redactAnyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return RedactParams(t)
	case []any:
		arr := make([]any, len(t))
		for i := range t {
			arr[i] = redactAnyValue(t[i])
		}
		return arr
	default:
		return v
	}
}

func isSecretKey(k string) bool {
	key := strings.ToLower(strings.TrimSpace(k))
	if key == "" {
		return false
	}
	if key == "authorization" || key == "cookie" || key == "api_key" || key == "token" || key == "password" || key == "secret" {
		return true
	}
	if strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") {
		return true
	}
	if strings.Contains(key, "api-key") || strings.Contains(key, "api_key") {
		return true
	}
	return false
}

// MarshalRedactedParams is useful for concise logging.
func MarshalRedactedParams(params map[string]any) string {
	redacted := RedactParams(params)
	b, err := json.Marshal(redacted)
	if err != nil {
		return "{}"
	}
	return string(b)
}
