package governance

import (
	"testing"
	"time"
)

func cacheAction(params map[string]any) GovernedAction {
	return GovernedAction{
		ID:         "id",
		Actor:      "gorkbot",
		Capability: "tool.bash",
		ToolName:   "bash",
		Parameters: params,
		RiskClass:  RISK_PRIVILEGED_BRIDGE,
		CreatedAt:  time.Now(),
	}
}

func TestApprovalCacheOnceNotCached(t *testing.T) {
	c := NewApprovalCache()
	a := cacheAction(map[string]any{"command": "echo hi"})
	c.Put(a, ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE})
	if _, ok := c.Get(a); ok {
		t.Fatal("once approval should not be cached")
	}
}

func TestApprovalCacheSessionCached(t *testing.T) {
	c := NewApprovalCache()
	a := cacheAction(map[string]any{"command": "echo hi"})
	c.Put(a, ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_SESSION})
	if _, ok := c.Get(a); !ok {
		t.Fatal("session approval should be cached")
	}
}

func TestApprovalCacheAlwaysCached(t *testing.T) {
	c := NewApprovalCache()
	a := cacheAction(map[string]any{"command": "echo hi"})
	c.Put(a, ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ALWAYS})
	if _, ok := c.Get(a); !ok {
		t.Fatal("always approval should be cached")
	}
}

func TestApprovalCacheNeverCachedAsDeny(t *testing.T) {
	c := NewApprovalCache()
	a := cacheAction(map[string]any{"command": "echo hi"})
	c.Put(a, ApprovalResult{Decision: APPROVAL_DENIED, Scope: APPROVAL_NEVER})
	res, ok := c.Get(a)
	if !ok {
		t.Fatal("never decision should be cached")
	}
	if res.Decision != APPROVAL_DENIED {
		t.Fatalf("expected denied, got %#v", res)
	}
}

func TestApprovalCacheClearSession(t *testing.T) {
	c := NewApprovalCache()
	a := cacheAction(map[string]any{"command": "echo hi"})
	c.Put(a, ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_SESSION})
	c.ClearSession()
	if _, ok := c.Get(a); ok {
		t.Fatal("session cache should clear")
	}
}

func TestApprovalCacheKeyStable(t *testing.T) {
	c := NewApprovalCache()
	a1 := cacheAction(map[string]any{"b": "x", "a": float64(1)})
	a2 := cacheAction(map[string]any{"a": float64(1), "b": "x"})
	if c.Key(a1) != c.Key(a2) {
		t.Fatalf("expected stable key")
	}
}

func TestApprovalCacheKeyIgnoresActionID(t *testing.T) {
	c := NewApprovalCache()
	a1 := cacheAction(map[string]any{"command": "echo hi"})
	a2 := a1
	a1.ID = "action-1"
	a2.ID = "action-2"
	if c.Key(a1) != c.Key(a2) {
		t.Fatalf("expected key to ignore action id")
	}
}

func TestApprovalCacheKeyChangesWithParams(t *testing.T) {
	c := NewApprovalCache()
	a1 := cacheAction(map[string]any{"command": "echo hi"})
	a2 := cacheAction(map[string]any{"command": "echo bye"})
	if c.Key(a1) == c.Key(a2) {
		t.Fatalf("expected different keys for different params")
	}
}
