package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendEmailToolMissingConfig(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("SMTP_FROM", "")

	tool := NewSendEmailTool()
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"to":      "user@example.com",
		"subject": "subject",
		"body":    "body",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure when SMTP is not configured")
	}
	if !strings.Contains(res.Error, "SMTP configuration is incomplete") {
		t.Fatalf("unexpected error: %q", res.Error)
	}
}

func TestSlackNotifyToolMissingWebhook(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "")

	tool := NewSlackNotifyTool()
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"channel": "#alerts",
		"message": "hello",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure when webhook is not configured")
	}
	if !strings.Contains(res.Error, "webhook URL is required") {
		t.Fatalf("unexpected error: %q", res.Error)
	}
}

func TestSlackNotifyToolWebhookSuccess(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tool := NewSlackNotifyTool()
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"channel":     "#alerts",
		"message":     "deploy complete",
		"webhook_url": srv.URL,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got error: %s", res.Error)
	}
	if got["text"] != "deploy complete" {
		t.Fatalf("unexpected text payload: %q", got["text"])
	}
	if got["channel"] != "#alerts" {
		t.Fatalf("unexpected channel payload: %q", got["channel"])
	}
}

func TestSlackNotifyToolWebhookFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad webhook", http.StatusBadRequest)
	}))
	defer srv.Close()

	tool := NewSlackNotifyTool()
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"message":     "deploy complete",
		"webhook_url": srv.URL,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure on non-2xx response")
	}
	if !strings.Contains(res.Error, "status 400") {
		t.Fatalf("unexpected error: %q", res.Error)
	}
}
