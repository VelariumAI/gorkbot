package governance

import "testing"

func TestClassifyReadFile(t *testing.T) {
	if got := ClassifyTool("read_file", nil); got != RISK_READ_ONLY {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyWriteFile(t *testing.T) {
	if got := ClassifyTool("write_file", nil); got != RISK_LOCAL_MUTATION {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyDeleteFile(t *testing.T) {
	if got := ClassifyTool("delete_file", nil); got != RISK_DESTRUCTIVE {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyBash(t *testing.T) {
	if got := ClassifyTool("bash", map[string]any{"command": "rm -rf /"}); got != RISK_PRIVILEGED_BRIDGE {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyGitPush(t *testing.T) {
	if got := ClassifyTool("git_push", nil); got != RISK_EXTERNAL_SIDE_EFFECT {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyHTTPRequestGetNoHeadersReadOnly(t *testing.T) {
	if got := ClassifyTool("http_request", map[string]any{"method": "GET", "url": "https://example.com"}); got != RISK_READ_ONLY {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyHTTPRequestGetWithAuthorizationNotReadOnly(t *testing.T) {
	if got := ClassifyTool("http_request", map[string]any{
		"method": "GET",
		"url":    "https://example.com",
		"headers": map[string]any{
			"Authorization": "Bearer x",
		},
	}); got == RISK_READ_ONLY {
		t.Fatalf("expected non-read-only risk, got %s", got)
	}
}

func TestClassifyHTTPRequestLocalhostPrivileged(t *testing.T) {
	if got := ClassifyTool("http_request", map[string]any{"method": "GET", "url": "http://127.0.0.1:8080/x"}); got != RISK_PRIVILEGED_BRIDGE {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyHTTPRequestPost(t *testing.T) {
	if got := ClassifyTool("http_request", map[string]any{"method": "POST", "url": "https://example.com"}); got != RISK_EXTERNAL_SIDE_EFFECT {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyDownloadFileLocalhostPrivileged(t *testing.T) {
	if got := ClassifyTool("download_file", map[string]any{"url": "http://localhost/a"}); got != RISK_PRIVILEGED_BRIDGE {
		t.Fatalf("got %s", got)
	}
}

func TestClassifyUnknown(t *testing.T) {
	if got := ClassifyTool("mystery_tool", nil); got != RISK_UNKNOWN {
		t.Fatalf("got %s", got)
	}
}
