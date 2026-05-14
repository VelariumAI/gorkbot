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

	"github.com/velariumai/gorkbot/pkg/trace"
)

const defaultCacheTTL = 5 * time.Minute

type Gateway struct {
	Policy    Policy
	Client    *http.Client
	Logger    *slog.Logger
	Cache     Cache
	TraceSink trace.Sink
	TraceMode trace.Mode
}

type evaluatedRequest struct {
	req      ResearchRequest
	decision ResearchDecision
	safeURL  validatedURL
}

type redirectBlockedError struct {
	reason string
}

func (e redirectBlockedError) Error() string {
	return "redirect blocked: " + e.reason
}

func New(policy Policy, logger *slog.Logger) *Gateway {
	if logger == nil {
		logger = slog.Default()
	}
	return &Gateway{
		Policy:    policy,
		Client:    &http.Client{},
		Logger:    logger,
		Cache:     NewMemoryCache(),
		TraceSink: trace.NoopSink{},
		TraceMode: trace.ModeOff,
	}
}

func (g *Gateway) SetTraceSink(sink trace.Sink, mode trace.Mode) {
	if sink == nil {
		sink = trace.NoopSink{}
	}
	g.TraceSink = sink
	g.TraceMode = mode
}

func (g *Gateway) Decide(ctx context.Context, req ResearchRequest) ResearchDecision {
	_ = ctx
	return g.Policy.Evaluate(req)
}

func (g *Gateway) Fetch(ctx context.Context, req ResearchRequest) (ResearchResult, ResearchDecision, error) {
	start := time.Now()
	evaluated := g.evaluateForExecution(ctx, req)
	decision := evaluated.decision

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
	cacheAllowed := !hasCredentialMaterial(req.Headers, req.Metadata) && !isCredentialedNormalizedURL(decision.NormalizedURL)
	if g.Cache != nil && cacheAllowed && (req.Kind == REQUEST_FETCH || req.Kind == REQUEST_HEAD) {
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

	resp, err := g.executeValidatedFetch(reqCtx, method, evaluated.safeURL, req.Headers, req.UserAgent)
	if err != nil {
		if errors.Is(err, errInvalidValidatedRequest) {
			decision.Allowed = false
			decision.FinalStatus = RESEARCH_BLOCKED
			decision.ReasonCode = REASON_URL_INVALID
			g.logDecision(req, decision, result, time.Since(start))
			return result, decision, err
		}
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
		if g.Cache != nil && cacheAllowed && (req.Kind == REQUEST_FETCH || req.Kind == REQUEST_HEAD) {
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

	if g.Cache != nil && cacheAllowed && (req.Kind == REQUEST_FETCH || req.Kind == REQUEST_HEAD) {
		g.Cache.Put(cacheKey, result, defaultCacheTTL)
	}

	g.logDecision(req, decision, result, time.Since(start))
	return result, decision, nil
}

func isCredentialedNormalizedURL(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return false
	}
	return IsCredentialedURL(u)
}

func (g *Gateway) httpClientForRequest() *http.Client {
	if g.Client == nil {
		return &http.Client{}
	}
	copy := *g.Client
	return &copy
}

// errInvalidValidatedRequest is returned when constructing the outbound HTTP
// request fails for reasons other than transport, deadline, or redirect
// blocking. It is used by the caller to classify a request as URL-invalid.
var errInvalidValidatedRequest = errors.New("invalid validated research request")

// executeValidatedFetch is the single centralized outbound HTTP sink for the
// research-egress gateway. It accepts only the validatedURL safe typed
// artifact; raw user input never reaches this function. Redirects are
// revalidated into fresh validatedURL artifacts before they are followed.
// All call sites in pkg/researchgate route their outbound HTTP through here.
func (g *Gateway) executeValidatedFetch(
	ctx context.Context,
	method string,
	safeURL validatedURL,
	headers map[string]string,
	userAgent string,
) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, method, safeURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidValidatedRequest, err)
	}

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	if ua := strings.TrimSpace(userAgent); ua != "" {
		httpReq.Header.Set("User-Agent", ua)
	}

	client := g.httpClientForRequest()
	client.CheckRedirect = func(redirReq *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		revalidated, reason, ok := validateResearchURL(redirReq.URL.String(), g.Policy)
		if !ok {
			return redirectBlockedError{reason: reason}
		}
		redirReq.URL = revalidated.URL()
		return nil
	}

	// Centralized validated research-egress sink. The URL is not consumed from
	// raw user input: Fetch first runs validateResearchURL(...) to produce a
	// validatedURL safe typed artifact, and only that artifact is carried to
	// this sink. Validation rejects unsupported schemes, embedded URL
	// credentials, private/loopback/link-local/cloud-metadata hosts, and
	// unsafe redirects (revalidated above). Credential headers and
	// credentialed normalized URLs are blocked earlier in the pipeline.
	//
	// This sink is intentional user-directed research egress, which CodeQL's
	// go/request-forgery query cannot model. Negative tests prove the
	// transport is not reached for invalid input:
	//   - TestGatewayFetchUsesValidatedURLInTransport
	//   - TestGatewayFetchBlockedURLNeverCallsTransport
	//   - TestGatewayFetchBlocksUnsupportedSchemeAndCredentialsBeforeTransport
	//   - TestGatewayBlocksRedirectToPrivateTarget
	//   - TestGatewayBlocksRedirectToIPv6Unspecified
	//   - TestGatewayBlocksRedirectToMetadataHost
	//   - TestGatewayBlocksCredentialHeaders
	//
	// codeql[go/request-forgery]
	return client.Do(httpReq)
}

func (g *Gateway) evaluateForExecution(ctx context.Context, req ResearchRequest) evaluatedRequest {
	_ = ctx
	decision := g.Decide(ctx, req)
	if !decision.Allowed {
		return evaluatedRequest{req: req, decision: decision}
	}

	safeURL, reason, ok := validateResearchURL(req.URL, g.Policy)
	if !ok {
		decision.Allowed = false
		decision.FinalStatus = RESEARCH_BLOCKED
		decision.ReasonCode = reason
		return evaluatedRequest{req: req, decision: decision}
	}
	decision.NormalizedURL = safeURL.String()
	return evaluatedRequest{req: req, decision: decision, safeURL: safeURL}
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

	if g.TraceSink != nil && g.TraceMode != trace.ModeOff {
		ev := trace.NewEvent("researchgate", "research_egress")
		ev.Operator = trace.OperatorRetrieve
		ev.Decision = trace.RedactString(decision.FinalStatus, 64)
		ev.ReasonCode = trace.RedactString(decision.ReasonCode, 128)
		ev.Duration = dur.Milliseconds()
		ev.Status = "ok"
		if !decision.Allowed {
			ev.Status = "blocked"
		}
		ev.RedactionState = trace.RedactionRedacted
		ev.ArtifactRefs = []trace.Ref{
			trace.NewRef("url", decision.NormalizedURL, result.SHA256, result.BytesRead),
		}
		ev.ReceiptRefs = []trace.Ref{
			trace.NewRef("request_id", req.ID, "", 0),
		}
		ev.Metadata = trace.NewMetadata(map[string]string{
			"kind":         string(req.Kind),
			"method":       strings.ToUpper(strings.TrimSpace(req.Method)),
			"status_code":  fmt.Sprintf("%d", result.StatusCode),
			"bytes_read":   fmt.Sprintf("%d", result.BytesRead),
			"from_cache":   fmt.Sprintf("%t", result.FromCache),
			"content_type": trace.RedactString(result.ContentType, 64),
		})
		_ = trace.Emit(context.Background(), g.TraceSink, g.TraceMode, ev)
	}
}
