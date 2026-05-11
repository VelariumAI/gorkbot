package cache

import (
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

func TestStrategyForProvider(t *testing.T) {
	cases := map[string]Strategy{
		"anthropic":  StrategyAnthropicBreakpoints,
		"minimax":    StrategyAnthropicBreakpoints,
		"openrouter": StrategyAnthropicBreakpoints,
		"google":     StrategyGeminiContext,
		"gemini":     StrategyGeminiContext,
		"xai":        StrategyGrokAutomatic,
		"grok":       StrategyGrokAutomatic,
		"openai":     StrategyOpenAIAutomatic,
		"moonshot":   StrategyMoonshotBestEffort,
		"unknown":    StrategyApplicationLayer,
	}
	for in, want := range cases {
		if got := strategyForProvider(in); got != want {
			t.Fatalf("strategyForProvider(%q)=%v want %v", in, got, want)
		}
	}
}

func TestAnthropicMinTokensModelMapping(t *testing.T) {
	if got := anthropicMinTokens("claude-sonnet-4-6"); got != 2048 {
		t.Fatalf("unexpected floor for sonnet-4-6: %d", got)
	}
	if got := anthropicMinTokens("claude-opus-4-6"); got != 4096 {
		t.Fatalf("unexpected floor for opus-4-6: %d", got)
	}
	if got := anthropicMinTokens("claude-unknown"); got != 1024 {
		t.Fatalf("unexpected floor for unknown model: %d", got)
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := estimateTokens(""); got != 0 {
		t.Fatalf("estimateTokens(\"\")=%d want 0", got)
	}
	if got := estimateTokens("abcd"); got != 1 {
		t.Fatalf("estimateTokens(4 chars)=%d want 1", got)
	}
	if got := estimateTokens("abcde"); got != 2 {
		t.Fatalf("estimateTokens(5 chars)=%d want 2", got)
	}
}

func TestAnthropicBreakpoints(t *testing.T) {
	floorChars := 1024 * 4
	long := strings.Repeat("x", floorChars)
	short := "tiny"

	msgs := []ai.ConversationMessage{
		{Role: "assistant", Content: long},
		{Role: "user", Content: short},
		{Role: "user", Content: long},
		{Role: "tool", Content: long},
		{Role: "user", Content: long},
	}
	bp := anthropicBreakpoints("claude-3-sonnet", long, msgs)
	if len(bp) == 0 {
		t.Fatal("expected non-empty breakpoints")
	}
	if bp[0] != -1 {
		t.Fatalf("expected first breakpoint to be system sentinel -1, got %d", bp[0])
	}
	// Should include last two user messages that clear threshold.
	if len(bp) != 3 || bp[1] != 4 || bp[2] != 2 {
		t.Fatalf("unexpected user breakpoints: %#v", bp)
	}
}

func TestGrokCacheHeaders(t *testing.T) {
	if got := GrokCacheHeaders(""); got != nil {
		t.Fatalf("expected nil headers for empty conv id, got %#v", got)
	}
	got := GrokCacheHeaders("conv-123")
	if got["x-grok-conv-id"] != "conv-123" {
		t.Fatalf("missing/invalid x-grok-conv-id header: %#v", got)
	}
}

func TestOpenAICacheHelpers(t *testing.T) {
	msgs := []ai.ConversationMessage{
		{Role: "user", Content: "u1"},
		{Role: "system", Content: "s1"},
		{Role: "assistant", Content: "a1"},
		{Role: "system", Content: "s2"},
	}
	out := OptimiseForOpenAICache(msgs)
	if len(out) != len(msgs) {
		t.Fatalf("unexpected output length: %d", len(out))
	}
	if out[0].Role != "system" || out[1].Role != "system" {
		t.Fatalf("expected system messages first, got roles: %s, %s", out[0].Role, out[1].Role)
	}

	if !IsOpenAICacheWorthy(strings.Repeat("x", OpenAICacheMinTokens*4)) {
		t.Fatal("expected long prompt to be cache-worthy")
	}
	if IsOpenAICacheWorthy("short") {
		t.Fatal("expected short prompt to not be cache-worthy")
	}
}

func TestMoonshotClientNoop(t *testing.T) {
	m := &MoonshotCacheClient{}
	id, err := m.Create("system")
	if err != nil {
		t.Fatalf("expected no-op create to not error: %v", err)
	}
	if id != "" {
		t.Fatalf("expected empty cache id from stub, got %q", id)
	}
	if ref := m.Reference(); ref != "" {
		t.Fatalf("expected empty reference from stub, got %q", ref)
	}
	m.Delete()
}

func TestAppCacheSetGetAndTTL(t *testing.T) {
	c := NewAppCache(t.TempDir())
	c.ttl = 10 * time.Millisecond
	c.Set("k", "v")
	if got, ok := c.Get("k"); !ok || got != "v" {
		t.Fatalf("expected immediate cache hit, got %q ok=%v", got, ok)
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestAppCacheEvictionAtCapacity(t *testing.T) {
	c := NewAppCache(t.TempDir())
	for i := 0; i < appCacheMaxEntries+10; i++ {
		key := "k" + strings.Repeat("x", i%3) + string(rune('a'+(i%26)))
		c.Set(key+string(rune(i/26)), "v")
	}
	if got := c.Len(); got > appCacheMaxEntries {
		t.Fatalf("cache length exceeds max entries: %d", got)
	}
}

func TestSessionKeyAndContentHashDeterminism(t *testing.T) {
	sk1, err := NewSessionKey()
	if err != nil {
		t.Fatalf("NewSessionKey failed: %v", err)
	}
	sk2, err := NewSessionKey()
	if err != nil {
		t.Fatalf("NewSessionKey failed: %v", err)
	}
	data := []byte("hello")
	s1a := sk1.Sign(data)
	s1b := sk1.Sign(data)
	s2 := sk2.Sign(data)
	if s1a != s1b {
		t.Fatal("same session key/data must produce same signature")
	}
	if s1a == s2 {
		t.Fatal("different session keys should produce different signatures")
	}
	if len(s1a) != 64 {
		t.Fatalf("expected 64-char hex signature, got len=%d", len(s1a))
	}

	h1 := ContentHash([]byte("abc"))
	h2 := ContentHash([]byte("abc"))
	h3 := ContentHash([]byte("xyz"))
	if h1 != h2 || h1 == h3 {
		t.Fatalf("unexpected content hash behavior: %q %q %q", h1, h2, h3)
	}
}

func TestAdvisorAppCacheAndStrategyResolution(t *testing.T) {
	a, err := NewAdvisor("", "", t.TempDir())
	if err != nil {
		t.Fatalf("NewAdvisor failed: %v", err)
	}
	sys := "system-prompt"
	model := "m"
	provider := "custom-provider"

	h0 := a.Advise(provider, model, sys, nil)
	if h0.AppCacheHit {
		t.Fatal("expected initial app-cache miss")
	}

	a.StoreAppCacheResponse(provider, model, sys, "cached-response")
	h1 := a.Advise(provider, model, sys, nil)
	if !h1.AppCacheHit || h1.AppCachedResponse != "cached-response" {
		t.Fatalf("expected app cache hit with stored value, got %+v", h1)
	}
	if h1.SystemPromptChanged {
		t.Fatal("expected unchanged system prompt on second advise")
	}

	if got := a.resolveStrategy("xai"); got != StrategyGrokAutomatic {
		t.Fatalf("unexpected resolved strategy for xai: %v", got)
	}
}

func TestAdvisorGrokAndGeminiHints(t *testing.T) {
	a, err := NewAdvisor("gem-key", "gemini-2.0-flash", t.TempDir())
	if err != nil {
		t.Fatalf("NewAdvisor failed: %v", err)
	}

	// Grok strategy should include stable conversation ID.
	g1 := a.Advise("xai", "grok-3", "sys", nil)
	g2 := a.Advise("xai", "grok-3", "sys", nil)
	if g1.GrokConvID == "" || g2.GrokConvID == "" || g1.GrokConvID != g2.GrokConvID {
		t.Fatalf("expected stable non-empty grok conv id, got %q and %q", g1.GrokConvID, g2.GrokConvID)
	}

	// Gemini strategy should use client state for cached content name.
	a.RecordGeminiCacheName("cachedContents/123")
	gm := a.Advise("google", "gemini-2.0-flash", "sys", nil)
	if gm.GeminiCachedContentName != "cachedContents/123" {
		t.Fatalf("expected recorded gemini cached content name, got %q", gm.GeminiCachedContentName)
	}
	if a.GeminiCacheClient() == nil {
		t.Fatal("expected gemini cache client to exist when key/model provided")
	}
}
