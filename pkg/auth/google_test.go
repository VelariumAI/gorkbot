package auth

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewGoogleClient(t.TempDir(), NotebookLMScopes(), nil)
	if err != nil {
		t.Fatalf("NewGoogleClient failed: %v", err)
	}
	return c
}

func TestTokenSetValidityHelpers(t *testing.T) {
	tok := &TokenSet{AccessToken: "a", ExpiresAt: time.Now().Add(2 * time.Minute)}
	if !tok.Valid() {
		t.Fatal("expected token to be valid")
	}
	if tok.Refreshable() {
		t.Fatal("expected token to not be refreshable without refresh token")
	}
	tok.RefreshToken = "r"
	if !tok.Refreshable() {
		t.Fatal("expected token to be refreshable with refresh token")
	}
}

func TestSaveLoadTokenRoundTrip(t *testing.T) {
	c := testClient(t)
	tok := &TokenSet{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		TokenType:    "Bearer",
		Scope:        strings.Join(NotebookLMScopes(), " "),
	}
	if err := c.saveToken(tok); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}
	got, err := c.loadToken()
	if err != nil {
		t.Fatalf("loadToken failed: %v", err)
	}
	if got == nil || got.AccessToken != "access" || got.RefreshToken != "refresh" {
		t.Fatalf("unexpected loaded token: %+v", got)
	}
}

func TestClientConfigRoundTrip(t *testing.T) {
	c := testClient(t)
	cfg := ClientConfig{ClientID: "client-id.apps.googleusercontent.com"}
	if err := c.SaveClientConfig(cfg); err != nil {
		t.Fatalf("SaveClientConfig failed: %v", err)
	}
	got, err := c.loadClientConfig()
	if err != nil {
		t.Fatalf("loadClientConfig failed: %v", err)
	}
	if got.ClientID != cfg.ClientID {
		t.Fatalf("unexpected client id: %q", got.ClientID)
	}
}

func TestEnsureTokenNoClientConfigReturnsAuthInstructions(t *testing.T) {
	c := testClient(t)
	res, err := c.EnsureToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureToken returned error: %v", err)
	}
	if !res.AuthRequired {
		t.Fatal("expected EnsureToken to require auth without client config")
	}
	if !strings.Contains(res.Instructions, "Google OAuth Setup Required") {
		t.Fatalf("unexpected setup instructions: %q", res.Instructions)
	}
}

func TestEnsureTokenWithValidStoredToken(t *testing.T) {
	c := testClient(t)
	tok := &TokenSet{
		AccessToken: "valid-access",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
		TokenType:   "Bearer",
	}
	if err := c.saveToken(tok); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}
	res, err := c.EnsureToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureToken failed: %v", err)
	}
	if res.Token == nil || res.Token.AccessToken != "valid-access" {
		t.Fatalf("expected valid cached token, got %+v", res.Token)
	}
}

func TestPollDeviceTokenPendingAndSuccess(t *testing.T) {
	c := testClient(t)
	state := 0
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := `{"error":"authorization_pending"}`
			if state == 1 {
				body = `{"access_token":"tok","refresh_token":"ref","expires_in":3600,"token_type":"Bearer","scope":"s"}`
			}
			state++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	cfg := &ClientConfig{ClientID: "cid"}
	tok, pending, err := c.pollDeviceToken(context.Background(), cfg, "dc")
	if err != nil || !pending || tok != nil {
		t.Fatalf("expected pending response first, got tok=%+v pending=%v err=%v", tok, pending, err)
	}

	tok, pending, err = c.pollDeviceToken(context.Background(), cfg, "dc")
	if err != nil || pending || tok == nil || tok.AccessToken != "tok" {
		t.Fatalf("expected successful token response, got tok=%+v pending=%v err=%v", tok, pending, err)
	}
}

func TestRefreshTokenKeepsExistingRefreshTokenWhenMissingInResponse(t *testing.T) {
	c := testClient(t)
	if err := c.SaveClientConfig(ClientConfig{ClientID: "cid"}); err != nil {
		t.Fatalf("SaveClientConfig failed: %v", err)
	}
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := `{"access_token":"new-access","expires_in":1800,"token_type":"Bearer"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	in := &TokenSet{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
		TokenType:    "Bearer",
		Scope:        "scope",
	}
	out, err := c.refreshToken(context.Background(), in)
	if err != nil {
		t.Fatalf("refreshToken failed: %v", err)
	}
	if out.RefreshToken != "old-refresh" {
		t.Fatalf("expected old refresh token to be preserved, got %q", out.RefreshToken)
	}
}

func TestSetupInstructionsAndScopes(t *testing.T) {
	c := testClient(t)
	s := c.setupInstructions()
	if !strings.Contains(s, "Google OAuth Setup Required") {
		t.Fatalf("unexpected setup instructions: %q", s)
	}
	scopes := NotebookLMScopes()
	if len(scopes) != 2 || scopes[0] != DriveReadOnlyScope || scopes[1] != DocsReadOnlyScope {
		t.Fatalf("unexpected NotebookLM scopes: %#v", scopes)
	}
}

func TestEnsureTokenRefreshAndDeviceFlowStart(t *testing.T) {
	c := testClient(t)
	if err := c.SaveClientConfig(ClientConfig{ClientID: "cid"}); err != nil {
		t.Fatalf("SaveClientConfig failed: %v", err)
	}

	// Seed expired refreshable token.
	if err := c.saveToken(&TokenSet{
		AccessToken:  "old",
		RefreshToken: "ref",
		ExpiresAt:    time.Now().Add(-time.Minute),
		TokenType:    "Bearer",
		Scope:        "scope",
	}); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	// First run: successful refresh path.
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"new","expires_in":3600,"token_type":"Bearer"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	res, err := c.EnsureToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureToken refresh path failed: %v", err)
	}
	if res.Token == nil || res.Token.AccessToken != "new" {
		t.Fatalf("expected refreshed token, got %+v", res.Token)
	}

	// Second run: refresh failure falls back to device flow start.
	if err := c.saveToken(&TokenSet{
		AccessToken:  "old2",
		RefreshToken: "ref2",
		ExpiresAt:    time.Now().Add(-time.Minute),
		TokenType:    "Bearer",
		Scope:        "scope",
	}); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}
	call := 0
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			call++
			if call == 1 {
				// refresh returns OAuth error
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_grant","error_description":"bad refresh"}`)),
					Header:     make(http.Header),
				}, nil
			}
			// requestDeviceCode response
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"device_code":"dc","user_code":"uc","verification_url":"https://verify","expires_in":1800,"interval":1}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
	}
	res2, err := c.EnsureToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureToken device flow start failed: %v", err)
	}
	if !res2.AuthRequired || res2.DeviceCode != "dc" || res2.PollInterval <= 0 {
		t.Fatalf("expected device flow response, got %+v", res2)
	}
}

func TestStatusBranches(t *testing.T) {
	c := testClient(t)

	// No token branch.
	if status := c.Status(); !strings.Contains(status, "not authenticated") {
		t.Fatalf("unexpected no-token status: %q", status)
	}

	// Expired refreshable branch.
	if err := c.saveToken(&TokenSet{
		AccessToken:  "expired",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
		TokenType:    "Bearer",
	}); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}
	if status := c.Status(); !strings.Contains(status, "auto-refresh") {
		t.Fatalf("unexpected refreshable status: %q", status)
	}

	// Expired non-refreshable branch.
	if err := c.saveToken(&TokenSet{
		AccessToken: "expired2",
		ExpiresAt:   time.Now().Add(-time.Minute),
		TokenType:   "Bearer",
	}); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}
	if status := c.Status(); !strings.Contains(status, "re-authentication required") {
		t.Fatalf("unexpected expired status: %q", status)
	}
}

func TestDeviceAndRefreshErrorBranches(t *testing.T) {
	c := testClient(t)
	cfg := &ClientConfig{ClientID: "cid"}

	// requestDeviceCode non-200 path.
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("bad request")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if _, err := c.requestDeviceCode(context.Background(), cfg); err == nil {
		t.Fatalf("expected requestDeviceCode error on non-200")
	}

	// pollDeviceToken error variants.
	for _, payload := range []string{
		`{"error":"access_denied"}`,
		`{"error":"expired_token"}`,
		`{"error":"mystery","error_description":"oops"}`,
	} {
		c.httpCl = &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(payload)),
					Header:     make(http.Header),
				}, nil
			}),
		}
		if _, pending, err := c.pollDeviceToken(context.Background(), cfg, "dc"); err == nil || pending {
			t.Fatalf("expected terminal error for payload %s, got pending=%v err=%v", payload, pending, err)
		}
	}

	// refreshToken parse and OAuth error branches.
	if err := c.SaveClientConfig(ClientConfig{ClientID: "cid"}); err != nil {
		t.Fatalf("SaveClientConfig failed: %v", err)
	}
	in := &TokenSet{RefreshToken: "r", Scope: "s"}
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_grant","error_description":"bad refresh"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if _, err := c.refreshToken(context.Background(), in); err == nil {
		t.Fatalf("expected refreshToken oauth error")
	}
}

func TestRequestDeviceCodeAndCompleteDeviceFlow(t *testing.T) {
	c := testClient(t)
	if err := c.SaveClientConfig(ClientConfig{ClientID: "cid"}); err != nil {
		t.Fatalf("SaveClientConfig failed: %v", err)
	}

	call := 0
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"device_code":"dc","user_code":"uc","verification_url":"https://verify","expires_in":1800,"interval":1}`,
					)),
					Header: make(http.Header),
				}, nil
			case 2:
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"error":"authorization_pending"}`)),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"access_token":"tok","refresh_token":"ref","expires_in":3600,"token_type":"Bearer","scope":"s"}`,
					)),
					Header: make(http.Header),
				}, nil
			}
		}),
	}

	dc, err := c.requestDeviceCode(context.Background(), &ClientConfig{ClientID: "cid"})
	if err != nil {
		t.Fatalf("requestDeviceCode failed: %v", err)
	}
	if dc.DeviceCode != "dc" || dc.UserCode != "uc" {
		t.Fatalf("unexpected device response: %+v", dc)
	}

	tok, err := c.CompleteDeviceFlow(context.Background(), "dc", time.Millisecond)
	if err != nil {
		t.Fatalf("CompleteDeviceFlow failed: %v", err)
	}
	if tok.AccessToken != "tok" || tok.RefreshToken != "ref" {
		t.Fatalf("unexpected completed token: %+v", tok)
	}
}

func TestRevokeStatusAndAccessToken(t *testing.T) {
	c := testClient(t)
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	valid := &TokenSet{
		AccessToken:  "valid-token",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		TokenType:    "Bearer",
		Scope:        "scope",
	}
	if err := c.saveToken(valid); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	if got := c.GetAccessToken(); got != "valid-token" {
		t.Fatalf("unexpected access token: %q", got)
	}
	if status := c.Status(); !strings.Contains(status, "authenticated") {
		t.Fatalf("expected authenticated status, got: %q", status)
	}

	if err := c.RevokeToken(context.Background()); err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}
	if _, err := os.Stat(c.tokenPath()); !os.IsNotExist(err) {
		t.Fatalf("expected token file to be removed, err=%v", err)
	}
	if got := c.GetAccessToken(); got != "" {
		t.Fatalf("expected empty access token after revoke, got %q", got)
	}
	if status := c.Status(); !strings.Contains(status, "not authenticated") {
		t.Fatalf("expected not authenticated status, got: %q", status)
	}
}

func TestLoadTokenCorruptionAndParseBranches(t *testing.T) {
	c := testClient(t)

	// Corrupted ciphertext should be treated as missing token and removed.
	if err := os.WriteFile(c.tokenPath(), []byte("not-valid-ciphertext"), 0o600); err != nil {
		t.Fatalf("write corrupted token: %v", err)
	}
	tok, err := c.loadToken()
	if err != nil {
		t.Fatalf("expected no hard error for corrupted token, got %v", err)
	}
	if tok != nil {
		t.Fatalf("expected nil token for corrupted payload")
	}
	if _, err := os.Stat(c.tokenPath()); !os.IsNotExist(err) {
		t.Fatalf("expected corrupted token file to be removed")
	}

	// Valid encryption but invalid JSON should return parse error.
	enc, err := c.keyMgr.Encrypt("not-json")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := os.WriteFile(c.tokenPath(), []byte(enc), 0o600); err != nil {
		t.Fatalf("write malformed json token: %v", err)
	}
	if _, err := c.loadToken(); err == nil {
		t.Fatalf("expected parse error for malformed token json")
	}
}

func TestClientConfigErrorBranches(t *testing.T) {
	c := testClient(t)

	// Invalid JSON config.
	if err := os.WriteFile(c.clientConfigPath(), []byte("{"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	if _, err := c.loadClientConfig(); err == nil {
		t.Fatalf("expected parse error for invalid client config")
	}

	// Empty client_id branch.
	if err := os.WriteFile(c.clientConfigPath(), []byte(`{"client_id":""}`), 0o600); err != nil {
		t.Fatalf("write empty config: %v", err)
	}
	if _, err := c.loadClientConfig(); err == nil {
		t.Fatalf("expected error for empty client_id")
	}
}

func TestSaveBranchesWithMkdirFailure(t *testing.T) {
	c := testClient(t)
	// Point configDir to a regular file path so MkdirAll fails.
	badPath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(badPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write marker file: %v", err)
	}
	c.configDir = badPath

	if err := c.saveToken(&TokenSet{AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)}); err == nil {
		t.Fatalf("expected saveToken mkdir error")
	}
	if err := c.SaveClientConfig(ClientConfig{ClientID: "cid"}); err == nil {
		t.Fatalf("expected SaveClientConfig mkdir error")
	}
}

func TestDeviceFlowAdditionalBranches(t *testing.T) {
	c := testClient(t)
	cfg := &ClientConfig{ClientID: "cid"}

	// requestDeviceCode parse-error branch.
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if _, err := c.requestDeviceCode(context.Background(), cfg); err == nil {
		t.Fatalf("expected parse error for malformed device code response")
	}

	// pollDeviceToken slow_down branch.
	c.httpCl = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"error":"slow_down"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if tok, pending, err := c.pollDeviceToken(context.Background(), cfg, "dc"); err != nil || !pending || tok != nil {
		t.Fatalf("expected slow_down pending branch, got tok=%v pending=%v err=%v", tok, pending, err)
	}
}

func TestCompleteDeviceFlowCancelledAndGetAccessTokenExpired(t *testing.T) {
	c := testClient(t)
	if err := c.SaveClientConfig(ClientConfig{ClientID: "cid"}); err != nil {
		t.Fatalf("SaveClientConfig failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.CompleteDeviceFlow(ctx, "dc", time.Millisecond); err == nil {
		t.Fatalf("expected context cancellation from CompleteDeviceFlow")
	}

	if err := c.saveToken(&TokenSet{
		AccessToken: "expired",
		ExpiresAt:   time.Now().Add(-time.Minute),
		TokenType:   "Bearer",
	}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	if got := c.GetAccessToken(); got != "" {
		t.Fatalf("expected empty access token for expired token, got %q", got)
	}
}
