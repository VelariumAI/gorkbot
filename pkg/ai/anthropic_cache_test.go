package ai

import "testing"

func TestInjectCacheControlMarksSystemAndRecentUser(t *testing.T) {
	long := ""
	for i := 0; i < 20000; i++ {
		long += "a"
	}
	msgs := []anthropicMessage{
		{Role: "user", Content: long},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: long},
	}

	sys, out := injectCacheControl("claude-opus-4", long, msgs)
	if _, ok := sys.([]anthropicBlockWithCache); !ok {
		t.Fatalf("expected cached system block")
	}
	if len(out) != len(msgs) {
		t.Fatalf("expected same number of messages")
	}
	// Most-recent user should be cached.
	if _, ok := out[2].Content.([]anthropicBlockWithCache); !ok {
		t.Fatalf("expected most recent user content to be cache-marked")
	}
}
