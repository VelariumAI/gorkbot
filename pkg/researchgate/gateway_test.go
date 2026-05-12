package researchgate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func gatewayForServer(t *testing.T, p Policy, ts *httptest.Server, virtualHost string) (*Gateway, string) {
	t.Helper()

	target, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	virtualURL := target.Scheme + "://" + virtualHost + "/"

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if strings.HasPrefix(addr, virtualHost) {
				addr = target.Host
			}
			d := &net.Dialer{}
			return d.DialContext(ctx, network, addr)
		},
	}

	g := New(p, slog.New(slog.NewTextHandler(io.Discard, nil)))
	g.Client = &http.Client{Transport: transport}
	return g, virtualURL
}

func TestGatewayFetchPublicGET(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "hello world")
	}))
	defer ts.Close()

	g, vurl := gatewayForServer(t, DefaultPolicy(), ts, "public.test:80")
	req := ResearchRequest{ID: "1", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()}
	result, decision, err := g.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed decision, got %#v", decision)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 status, got %d", result.StatusCode)
	}
	if result.SHA256 == "" || result.BytesRead == 0 {
		t.Fatalf("expected hash+bytes in result, got %#v", result)
	}
}

func TestGatewayFetchPublicHEAD(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	g, vurl := gatewayForServer(t, DefaultPolicy(), ts, "public.test:80")
	req := ResearchRequest{ID: "2", Kind: REQUEST_HEAD, Method: "HEAD", URL: vurl, CreatedAt: time.Now().UTC()}
	result, decision, err := g.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("head failed: %v", err)
	}
	if !decision.Allowed || result.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected head response: decision=%#v result=%#v", decision, result)
	}
}

func TestGatewayEnforcesTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(60 * time.Millisecond)
		_, _ = io.WriteString(w, "late")
	}))
	defer ts.Close()

	p := DefaultPolicy()
	p.DefaultTimeout = 20 * time.Millisecond
	p.MaxTimeout = 20 * time.Millisecond
	g, vurl := gatewayForServer(t, p, ts, "public.test:80")

	req := ResearchRequest{ID: "3", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()}
	_, decision, err := g.Fetch(context.Background(), req)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if decision.ReasonCode != REASON_TIMEOUT {
		t.Fatalf("expected timeout reason, got %#v", decision)
	}
}

func TestGatewayEnforcesMaxResponseBytes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("A", 2048))
	}))
	defer ts.Close()

	p := DefaultPolicy()
	p.MaxResponseBytes = 512
	g, vurl := gatewayForServer(t, p, ts, "public.test:80")

	req := ResearchRequest{ID: "4", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()}
	_, decision, err := g.Fetch(context.Background(), req)
	if err == nil {
		t.Fatal("expected size-limit error")
	}
	if decision.ReasonCode != REASON_RESPONSE_TOO_LARGE {
		t.Fatalf("expected response too large reason, got %#v", decision)
	}
}

func TestGatewayBlocksRedirectToPrivateTarget(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/private", http.StatusFound)
	}))
	defer ts.Close()

	g, vurl := gatewayForServer(t, DefaultPolicy(), ts, "public.test:80")
	req := ResearchRequest{ID: "5", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()}
	_, decision, err := g.Fetch(context.Background(), req)
	if err == nil {
		t.Fatal("expected redirect block error")
	}
	if decision.ReasonCode != REASON_PRIVATE_NETWORK_BLOCKED {
		t.Fatalf("expected private redirect block, got %#v", decision)
	}
}

func TestGatewayDoesNotLogResponseBody(t *testing.T) {
	const secretBody = "SUPER_SECRET_RESPONSE_PAYLOAD"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, secretBody)
	}))
	defer ts.Close()

	p := DefaultPolicy()
	g, vurl := gatewayForServer(t, p, ts, "public.test:80")
	var logs bytes.Buffer
	g.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	req := ResearchRequest{ID: "6", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()}
	_, _, err := g.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if strings.Contains(logs.String(), secretBody) {
		t.Fatalf("response body leaked to logs: %s", logs.String())
	}
}

func TestGatewayBlocksCredentialHeaders(t *testing.T) {
	g := New(DefaultPolicy(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := ResearchRequest{
		ID:        "7",
		Kind:      REQUEST_FETCH,
		Method:    "GET",
		URL:       "https://example.com",
		Headers:   map[string]string{"Authorization": "Bearer test"},
		CreatedAt: time.Now().UTC(),
	}
	_, decision, err := g.Fetch(context.Background(), req)
	if err == nil {
		t.Fatal("expected credentials blocked")
	}
	if decision.ReasonCode != REASON_CREDENTIALS_FORBIDDEN {
		t.Fatalf("expected credential block, got %#v", decision)
	}
}

func TestGatewayCacheHit(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = io.WriteString(w, "cache")
	}))
	defer ts.Close()

	g, vurl := gatewayForServer(t, DefaultPolicy(), ts, "public.test:80")
	req := ResearchRequest{ID: "8", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()}
	_, _, err := g.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	result, _, err := g.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if !result.FromCache {
		t.Fatalf("expected cache hit on second fetch")
	}
	if hits != 1 {
		t.Fatalf("expected single upstream hit, got %d", hits)
	}
}

func TestGatewayDownloadUnknownSizeRequiresHuman(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = io.WriteString(w, "payload")
	}))
	defer ts.Close()

	g, vurl := gatewayForServer(t, DefaultPolicy(), ts, "public.test:80")
	req := ResearchRequest{ID: "9", Kind: REQUEST_DOWNLOAD, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()}
	_, decision, err := g.Fetch(context.Background(), req)
	if err == nil {
		t.Fatal("expected approval-required error")
	}
	if decision.ReasonCode != REASON_DOWNLOAD_REQUIRES_QUEUE || !decision.RequiresHuman {
		t.Fatalf("expected download queue requirement, got %#v", decision)
	}
}

func TestGatewayReturnsTimeoutReasonOnContextDeadline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, "never")
	}))
	defer ts.Close()

	p := DefaultPolicy()
	p.DefaultTimeout = 50 * time.Millisecond
	p.MaxTimeout = 50 * time.Millisecond
	g, vurl := gatewayForServer(t, p, ts, "public.test:80")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := ResearchRequest{ID: "10", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()}
	_, decision, err := g.Fetch(ctx, req)
	if err == nil {
		t.Fatal("expected context timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) && decision.ReasonCode != REASON_TIMEOUT {
		t.Fatalf("expected timeout classification, got err=%v decision=%#v", err, decision)
	}
}
