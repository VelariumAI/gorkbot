package researchgate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/harness"
)

func TestResearchHarnessAuditPassDoesNotChangeDecision(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "safe response")
	}))
	defer ts.Close()

	g, vurl := gatewayForServer(t, DefaultPolicy(), ts, "public.test:80")
	reg := harness.NewRegistry(harness.WithFailClosedUnsupported(false))
	if err := reg.Register(harness.Assertion{
		ID:        "research-required-surface",
		Scope:     "researchgate.egress_decision",
		Severity:  harness.SeverityHardFail,
		Type:      harness.AssertionTypeRequiredMetadata,
		Condition: "surface",
	}); err != nil {
		t.Fatalf("register assertion: %v", err)
	}
	g.SetHarnessRuntime(harness.NewRuntime(harness.ModeAudit, reg))

	result, decision, err := g.Fetch(context.Background(), ResearchRequest{ID: "ra-1", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("decision should remain allowed, got %#v", decision)
	}
	if result.AuditSummary == nil {
		t.Fatalf("expected audit summary")
	}
}

func TestResearchHarnessAuditFailDoesNotChangeDecision(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "safe response")
	}))
	defer ts.Close()

	g, vurl := gatewayForServer(t, DefaultPolicy(), ts, "public.test:80")
	reg := harness.NewRegistry(harness.WithFailClosedUnsupported(false))
	if err := reg.Register(harness.Assertion{
		ID:        "research-force-fail",
		Scope:     "researchgate.egress_decision",
		Severity:  harness.SeverityHardFail,
		Type:      harness.AssertionTypeForbiddenMetadataKey,
		Condition: "surface",
	}); err != nil {
		t.Fatalf("register assertion: %v", err)
	}
	g.SetHarnessRuntime(harness.NewRuntime(harness.ModeAudit, reg))

	result, decision, err := g.Fetch(context.Background(), ResearchRequest{ID: "ra-2", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("harness failure must not change decision, got %#v", decision)
	}
	if result.AuditSummary == nil || result.AuditSummary.FailedCount == 0 {
		t.Fatalf("expected failed harness audit summary, got %#v", result.AuditSummary)
	}
}

func TestResearchHarnessArtifactRedactsSensitiveData(t *testing.T) {
	req := ResearchRequest{
		ID:      "ra-redact",
		Kind:    REQUEST_FETCH,
		Method:  "GET",
		URL:     "https://example.com/resource",
		Headers: map[string]string{"Authorization": "Bearer super-secret", "X-Api-Key": "hidden"},
	}
	decision := ResearchDecision{
		Allowed:       true,
		FinalStatus:   RESEARCH_ALLOWED,
		ReasonCode:    REASON_RESEARCH_READ_ALLOWED,
		NormalizedURL: "https://example.com/resource",
	}
	result := ResearchResult{
		StatusCode:  200,
		BytesRead:   42,
		SHA256:      "abc123",
		BodyPreview: "TOP_SECRET_BODY",
	}

	artifact := buildResearchHarnessArtifact(req, decision, result, 5*time.Millisecond)
	if artifact.Content != "" {
		t.Fatalf("expected empty artifact content")
	}
	if strings.Contains(artifact.ContentHash, "TOP_SECRET_BODY") {
		t.Fatalf("body leaked into content hash")
	}
	for k, v := range artifact.Metadata {
		joined := strings.ToLower(k + "=" + v)
		if strings.Contains(joined, "authorization") || strings.Contains(joined, "bearer") || strings.Contains(joined, "api-key") || strings.Contains(joined, "top_secret_body") {
			t.Fatalf("sensitive data leaked in metadata: %q=%q", k, v)
		}
	}
}
