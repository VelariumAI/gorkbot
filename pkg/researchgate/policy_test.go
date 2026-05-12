package researchgate

import (
	"testing"
	"time"
)

func mkReq(kind RequestKind, method, rawURL string) ResearchRequest {
	return ResearchRequest{
		ID:        "req-1",
		Kind:      kind,
		Method:    method,
		URL:       rawURL,
		CreatedAt: time.Now().UTC(),
	}
}

func TestPolicyAllowsPublicGET(t *testing.T) {
	p := DefaultPolicy()
	decision := p.Evaluate(mkReq(REQUEST_FETCH, "GET", "https://example.com/docs"))
	if !decision.Allowed || decision.FinalStatus != RESEARCH_ALLOWED {
		t.Fatalf("expected allowed get, got %#v", decision)
	}
}

func TestPolicyAllowsPublicHEAD(t *testing.T) {
	p := DefaultPolicy()
	decision := p.Evaluate(mkReq(REQUEST_HEAD, "HEAD", "https://example.com/docs"))
	if !decision.Allowed {
		t.Fatalf("expected allowed head, got %#v", decision)
	}
}

func TestPolicyBlocksMutatingMethods(t *testing.T) {
	p := DefaultPolicy()
	methods := []string{"POST", "PUT", "PATCH", "DELETE"}
	for _, method := range methods {
		decision := p.Evaluate(mkReq(REQUEST_FETCH, method, "https://example.com/api"))
		if decision.Allowed || decision.ReasonCode != REASON_EXTERNAL_SIDE_EFFECT_REQUIRES_APPROVAL {
			t.Fatalf("method %s expected blocked side effect, got %#v", method, decision)
		}
	}
}

func TestPolicyBlocksUnsupportedSchemes(t *testing.T) {
	p := DefaultPolicy()
	for _, raw := range []string{"ftp://example.com/file", "file:///tmp/a", "gopher://example.com", "ssh://example.com"} {
		decision := p.Evaluate(mkReq(REQUEST_FETCH, "GET", raw))
		if decision.Allowed {
			t.Fatalf("expected blocked scheme for %s", raw)
		}
	}
}

func TestPolicyBlocksPrivateAndMetadataHosts(t *testing.T) {
	p := DefaultPolicy()
	for _, raw := range []string{
		"http://localhost/a",
		"http://localhost./a",
		"http://127.0.0.1/a",
		"http://127.1/a",
		"http://0.0.0.0/a",
		"http://[::1]/a",
		"http://[::]/a",
		"http://10.0.0.5/a",
		"http://172.16.0.9/a",
		"http://192.168.1.9/a",
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.10.1/a",
		"http://[fc00::1]/a",
		"http://[fe80::1]/a",
		"http://metadata.google.internal/a",
		"http://imds.amazonaws.com/a",
	} {
		decision := p.Evaluate(mkReq(REQUEST_FETCH, "GET", raw))
		if decision.Allowed || decision.ReasonCode != REASON_PRIVATE_NETWORK_BLOCKED {
			t.Fatalf("expected private network blocked for %s, got %#v", raw, decision)
		}
	}
}

func TestPolicyBlocksCredentialsInHeadersAndURL(t *testing.T) {
	p := DefaultPolicy()

	req := mkReq(REQUEST_FETCH, "GET", "https://example.com")
	req.Headers = map[string]string{"Authorization": "Bearer secret"}
	decision := p.Evaluate(req)
	if decision.Allowed || decision.ReasonCode != REASON_CREDENTIALS_FORBIDDEN {
		t.Fatalf("expected blocked auth header, got %#v", decision)
	}

	req = mkReq(REQUEST_FETCH, "GET", "https://example.com")
	req.Headers = map[string]string{"Cookie": "a=b"}
	decision = p.Evaluate(req)
	if decision.Allowed || decision.ReasonCode != REASON_CREDENTIALS_FORBIDDEN {
		t.Fatalf("expected blocked cookie header, got %#v", decision)
	}

	req = mkReq(REQUEST_FETCH, "GET", "https://example.com")
	req.Headers = map[string]string{"x-api-key": "abc"}
	decision = p.Evaluate(req)
	if decision.Allowed || decision.ReasonCode != REASON_CREDENTIALS_FORBIDDEN {
		t.Fatalf("expected blocked api key header, got %#v", decision)
	}

	req = mkReq(REQUEST_FETCH, "GET", "https://user:pass@example.com")
	decision = p.Evaluate(req)
	if decision.Allowed || decision.ReasonCode != REASON_CREDENTIALS_FORBIDDEN {
		t.Fatalf("expected blocked url credentials, got %#v", decision)
	}

	req = mkReq(REQUEST_FETCH, "GET", "https://example.com")
	req.Metadata = map[string]any{"bearer": "secret-token"}
	decision = p.Evaluate(req)
	if decision.Allowed || decision.ReasonCode != REASON_CREDENTIALS_FORBIDDEN {
		t.Fatalf("expected blocked bearer metadata, got %#v", decision)
	}
}

func TestPolicyBoundsTimeoutAndSize(t *testing.T) {
	p := DefaultPolicy()
	req := mkReq(REQUEST_FETCH, "GET", "https://example.com")
	req.TimeoutMS = int64((35 * time.Second).Milliseconds())
	req.MaxBytes = 999_999_999
	decision := p.Evaluate(req)

	if decision.TimeoutMS != int64((20 * time.Second).Milliseconds()) {
		t.Fatalf("expected max timeout clamp, got %d", decision.TimeoutMS)
	}
	if decision.MaxBytes != p.MaxResponseBytes {
		t.Fatalf("expected max bytes clamp, got %d", decision.MaxBytes)
	}
}
