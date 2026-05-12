package tools

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/researchgate"
)

func testGatewayForServer(t *testing.T, ts *httptest.Server, virtualHost string) (*researchgate.Gateway, string) {
	t.Helper()

	target, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	virtualURL := target.Scheme + "://" + virtualHost + "/"

	policy := researchgate.DefaultPolicy()
	gateway := researchgate.New(policy, nil)
	gateway.Client = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if strings.HasPrefix(addr, virtualHost) {
				addr = target.Host
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}

	return gateway, virtualURL
}

func gatewayCtx(mode string, g *researchgate.Gateway) context.Context {
	cfg := researchGatewayConfig{Gateway: g, Mode: mode}
	return context.WithValue(context.Background(), researchGatewayContextKey, cfg)
}

func TestHTTPRequestGetAllowedThroughResearchGateway(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	gateway, vurl := testGatewayForServer(t, ts, "public.test:80")
	tool := NewHttpRequestTool()

	res, err := tool.Execute(gatewayCtx("enforce", gateway), map[string]interface{}{
		"url":    vurl,
		"method": "GET",
	})
	if err != nil {
		t.Fatalf("http_request get failed: %v", err)
	}
	if !res.Success || !strings.Contains(res.Output, "ok") {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestHTTPRequestPostBlockedByResearchGateway(t *testing.T) {
	gateway := researchgate.New(researchgate.DefaultPolicy(), nil)
	tool := NewHttpRequestTool()

	res, err := tool.Execute(gatewayCtx("enforce", gateway), map[string]interface{}{
		"url":    "https://example.com/api",
		"method": "POST",
	})
	if err == nil {
		t.Fatal("expected post to be blocked")
	}
	if res == nil || res.Success || !strings.Contains(res.Error, researchgate.REASON_EXTERNAL_SIDE_EFFECT_REQUIRES_APPROVAL) {
		t.Fatalf("unexpected post block result: %#v err=%v", res, err)
	}
}

func TestHTTPRequestGetWithAuthorizationBlockedByResearchGateway(t *testing.T) {
	gateway := researchgate.New(researchgate.DefaultPolicy(), nil)
	tool := NewHttpRequestTool()

	res, err := tool.Execute(gatewayCtx("enforce", gateway), map[string]interface{}{
		"url":    "https://example.com",
		"method": "GET",
		"headers": map[string]interface{}{
			"Authorization": "Bearer secret",
		},
	})
	if err == nil {
		t.Fatal("expected credentialed get to be blocked")
	}
	if res == nil || res.Success || !strings.Contains(res.Error, researchgate.REASON_CREDENTIALS_FORBIDDEN) {
		t.Fatalf("unexpected credential block result: %#v err=%v", res, err)
	}
}

func TestWebFetchUsesResearchGatewayWhenEnforced(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("gateway-body"))
	}))
	defer ts.Close()

	gateway, vurl := testGatewayForServer(t, ts, "public.test:80")
	tool := NewWebFetchTool()

	res, err := tool.Execute(gatewayCtx("enforce", gateway), map[string]interface{}{
		"url": vurl,
	})
	if err != nil {
		t.Fatalf("web_fetch failed: %v", err)
	}
	if !res.Success || !strings.Contains(res.Output, "gateway-body") {
		t.Fatalf("unexpected web_fetch result: %#v", res)
	}
}

func TestHTTPRequestOffModeKeepsCompatibility(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("legacy"))
	}))
	defer ts.Close()

	tool := NewHttpRequestTool()
	ctx := gatewayCtx("off", researchgate.New(researchgate.DefaultPolicy(), nil))
	res, err := tool.Execute(ctx, map[string]interface{}{
		"url":    ts.URL,
		"method": "GET",
	})
	if err != nil {
		t.Fatalf("off mode should preserve legacy behavior, got err: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("off mode should preserve success path, got %#v", res)
	}
}

func TestHTTPGatewayEnforceTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer ts.Close()

	policy := researchgate.DefaultPolicy()
	policy.DefaultTimeout = 20 * time.Millisecond
	policy.MaxTimeout = 20 * time.Millisecond
	gateway, vurl := testGatewayForServer(t, ts, "public.test:80")
	gateway.Policy = policy

	tool := NewHttpRequestTool()
	res, err := tool.Execute(gatewayCtx("enforce", gateway), map[string]interface{}{
		"url":    vurl,
		"method": "GET",
	})
	if err == nil {
		t.Fatal("expected timeout block")
	}
	if res == nil || res.Success || !strings.Contains(res.Error, researchgate.REASON_TIMEOUT) {
		t.Fatalf("unexpected timeout result: %#v err=%v", res, err)
	}
}
