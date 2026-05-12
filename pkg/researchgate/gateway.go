package researchgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultCacheTTL = 5 * time.Minute

type Gateway struct {
	Policy Policy
	Client *http.Client
	Logger *slog.Logger
	Cache  Cache
}

type redirectBlockedError struct {
	reason string
}

func (e redirectBlockedError) Error() string {
	return "redirect blocked"
}

func New(policy Policy, logger *slog.Logger) *Gateway {
	if logger == nil {
		logger = slog.Default()
	}
	return &Gateway{
		Policy: policy,
		Client: &http.Client{},
		Logger: logger,
		Cache:  NewMemoryCache(),
	}
}

func (g *Gateway) Decide(ctx context.Context, req ResearchRequest) ResearchDecision {
	_ = ctx
	return g.Policy.Evaluate(req)
}

func (g *Gateway) Fetch(ctx context.Context, req ResearchRequest) (ResearchResult, ResearchDecision, error) {
	start := time.Now()
	decision := g.Decide(ctx, req)

	result := ResearchResult{
		RequestID: req.ID,
		URL:       req.URL,
		Query:     req.Query,
		FetchedAt: time.Now().UTC(),
	}

	if !decision.Allowed {
		g.logDecision(req, decision, result, time.Since(start))
		return result, decision, fmt.Errorf("research request blocked: %s", decision.ReasonCode)
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = inferredMethod(req.Kind)
	}

	cacheKey := method + " " + decision.NormalizedURL
	if g.Cache != nil && (req.Kind == REQUEST_FETCH || req.Kind == REQUEST_HEAD) {
		if cached, ok := g.Cache.Get(cacheKey); ok {
			cached.RequestID = req.ID
			cached.FromCache = true
			g.logDecision(req, decision, cached, time.Since(start))
			return cached, decision, nil
		}
	}

	timeout := time.Duration(decision.TimeoutMS) * time.Millisecond
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, method, decision.NormalizedURL, nil)
	if err != nil {
		decision.Allowed = false
		decision.FinalStatus = RESEARCH_BLOCKED
		decision.ReasonCode = REASON_URL_INVALID
		g.logDecision(req, decision, result, time.Since(start))
		return result, decision, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if ua := strings.TrimSpace(req.UserAgent); ua != "" {
		httpReq.Header.Set("User-Agent", ua)
	}

	client := g.httpClientForRequest()
	client.CheckRedirect = func(redirReq *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		normalized, err := NormalizeURL(redirReq.URL.String())
		if err != nil {
			return redirectBlockedError{reason: REASON_URL_INVALID}
		}
		u, err := url.Parse(normalized)
		if err != nil {
			return redirectBlockedError{reason: REASON_URL_INVALID}
		}
		host := normalizedHost(u.Host)
		if !g.Policy.AllowPrivateNetworks && (IsPrivateOrLocalHost(host) || HostLooksLikeCloudMetadata(host)) {
			return redirectBlockedError{reason: REASON_PRIVATE_NETWORK_BLOCKED}
		}
		if !IsSupportedScheme(u, g.Policy.AllowedSchemes) {
			return redirectBlockedError{reason: REASON_UNSUPPORTED_SCHEME}
		}
		return nil
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			decision.Allowed = false
			decision.FinalStatus = RESEARCH_BLOCKED
			decision.ReasonCode = REASON_TIMEOUT
			g.logDecision(req, decision, result, time.Since(start))
			return result, decision, err
		}
		var redirectErr redirectBlockedError
		if errors.As(err, &redirectErr) {
			decision.Allowed = false
			decision.FinalStatus = RESEARCH_BLOCKED
			decision.ReasonCode = redirectErr.reason
			g.logDecision(req, decision, result, time.Since(start))
			return result, decision, err
		}
		g.logDecision(req, decision, result, time.Since(start))
		return result, decision, err
	}
	defer resp.Body.Close()

	result.URL = decision.NormalizedURL
	result.StatusCode = resp.StatusCode
	result.ContentType = strings.TrimSpace(resp.Header.Get("Content-Type"))
	result.Metadata = map[string]any{}
	if cl := strings.TrimSpace(resp.Header.Get("Content-Length")); cl != "" {
		result.Metadata["content_length"] = cl
	}

	if req.Kind == REQUEST_DOWNLOAD {
		if resp.ContentLength < 0 || resp.ContentLength > decision.MaxBytes {
			decision.Allowed = false
			decision.RequiresHuman = true
			decision.FinalStatus = RESEARCH_REQUIRES_HUMAN
			decision.ReasonCode = REASON_DOWNLOAD_REQUIRES_QUEUE
			g.logDecision(req, decision, result, time.Since(start))
			return result, decision, fmt.Errorf("download requires approval")
		}
	}

	if method == string(METHOD_HEAD) {
		if g.Cache != nil && (req.Kind == REQUEST_FETCH || req.Kind == REQUEST_HEAD) {
			g.Cache.Put(cacheKey, result, defaultCacheTTL)
		}
		g.logDecision(req, decision, result, time.Since(start))
		return result, decision, nil
	}

	maxBytes := decision.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 512 * 1024
	}
	buf, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if readErr != nil {
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			decision.Allowed = false
			decision.FinalStatus = RESEARCH_BLOCKED
			decision.ReasonCode = REASON_TIMEOUT
			g.logDecision(req, decision, result, time.Since(start))
			return result, decision, readErr
		}
		g.logDecision(req, decision, result, time.Since(start))
		return result, decision, readErr
	}

	if int64(len(buf)) > maxBytes {
		decision.Allowed = false
		decision.FinalStatus = RESEARCH_BLOCKED
		decision.ReasonCode = REASON_RESPONSE_TOO_LARGE
		result.BytesRead = maxBytes
		result.BodyPreview = string(buf[:maxBytes])
		g.logDecision(req, decision, result, time.Since(start))
		return result, decision, fmt.Errorf("response too large")
	}

	sum := sha256.Sum256(buf)
	result.BytesRead = int64(len(buf))
	result.SHA256 = hex.EncodeToString(sum[:])
	result.BodyPreview = string(buf)

	if g.Cache != nil && (req.Kind == REQUEST_FETCH || req.Kind == REQUEST_HEAD) {
		g.Cache.Put(cacheKey, result, defaultCacheTTL)
	}

	g.logDecision(req, decision, result, time.Since(start))
	return result, decision, nil
}

func (g *Gateway) httpClientForRequest() *http.Client {
	if g.Client == nil {
		return &http.Client{}
	}
	copy := *g.Client
	return &copy
}

func (g *Gateway) logDecision(req ResearchRequest, decision ResearchDecision, result ResearchResult, dur time.Duration) {
	host := ""
	if decision.NormalizedURL != "" {
		if u, err := url.Parse(decision.NormalizedURL); err == nil {
			host = normalizedHost(u.Host)
		}
	}
	if host == "" && req.URL != "" {
		if u, err := url.Parse(req.URL); err == nil {
			host = normalizedHost(u.Host)
		}
	}

	g.Logger.Info("research_egress",
		"request_id", req.ID,
		"kind", req.Kind,
		"method", strings.ToUpper(strings.TrimSpace(req.Method)),
		"host", host,
		"allowed", decision.Allowed,
		"final_status", decision.FinalStatus,
		"reason_code", decision.ReasonCode,
		"status_code", result.StatusCode,
		"content_type", result.ContentType,
		"bytes_read", result.BytesRead,
		"sha256", result.SHA256,
		"duration_ms", dur.Milliseconds(),
		"from_cache", result.FromCache,
	)
}
