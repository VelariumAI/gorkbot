package execution

import "testing"

func TestTruncateOutput(t *testing.T) {
	if got := TruncateOutput("abc", 0); got != "" {
		t.Fatalf("expected empty for max<=0, got %q", got)
	}
	if got := TruncateOutput("abc", 5); got != "abc" {
		t.Fatalf("expected unchanged string, got %q", got)
	}
	got := TruncateOutput("abcdef", 3)
	want := "abc\n...[truncated 3 bytes]"
	if got != want {
		t.Fatalf("unexpected truncation: got %q want %q", got, want)
	}
}
