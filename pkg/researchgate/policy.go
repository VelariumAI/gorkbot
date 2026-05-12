package researchgate

import (
	"net/url"
	"strings"
	"time"
)

type Policy struct {
	AllowPublicGET             bool
	AllowHEAD                  bool
	AllowSearch                bool
	AllowDownloads             bool
	RequireApprovalForDownload bool
	AllowCredentials           bool
	AllowPrivateNetworks       bool
	AllowMutatingMethods       bool
	AllowedSchemes             []string
	BlockedHosts               []string
	AllowedHosts               []string
	MaxResponseBytes           int64
	MaxDownloadBytes           int64
	DefaultTimeout             time.Duration
	MaxTimeout                 time.Duration
	RequestsPerMinute          int
}

func DefaultPolicy() Policy {
	return Policy{
		AllowPublicGET:             true,
		AllowHEAD:                  true,
		AllowSearch:                true,
		AllowDownloads:             true,
		RequireApprovalForDownload: false,
		AllowCredentials:           false,
		AllowPrivateNetworks:       false,
		AllowMutatingMethods:       false,
		AllowedSchemes:             []string{"http", "https"},
		MaxResponseBytes:           512 * 1024,
		MaxDownloadBytes:           25 * 1024 * 1024,
		DefaultTimeout:             8 * time.Second,
		MaxTimeout:                 20 * time.Second,
		RequestsPerMinute:          30,
	}
}

func (p Policy) Evaluate(req ResearchRequest) ResearchDecision {
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}

	decision := ResearchDecision{
		RequestID:   req.ID,
		Allowed:     false,
		FinalStatus: RESEARCH_BLOCKED,
		ReasonCode:  REASON_METHOD_NOT_ALLOWED,
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = inferredMethod(req.Kind)
	}

	timeout := boundedTimeout(p, req.TimeoutMS)
	decision.TimeoutMS = timeout.Milliseconds()

	switch req.Kind {
	case REQUEST_DOWNLOAD:
		decision.MaxBytes = boundedMax(req.MaxBytes, p.MaxDownloadBytes)
	default:
		decision.MaxBytes = boundedMax(req.MaxBytes, p.MaxResponseBytes)
	}

	if !p.AllowCredentials && hasCredentialMaterial(req.Headers, req.Metadata) {
		decision.ReasonCode = REASON_CREDENTIALS_FORBIDDEN
		return decision
	}

	if req.Kind == REQUEST_SEARCH {
		if !p.AllowSearch {
			decision.ReasonCode = REASON_METHOD_NOT_ALLOWED
			return decision
		}
		decision.Allowed = true
		decision.FinalStatus = RESEARCH_ALLOWED
		decision.ReasonCode = REASON_RESEARCH_READ_ALLOWED
		return decision
	}

	if method != string(METHOD_GET) && method != string(METHOD_HEAD) {
		if !p.AllowMutatingMethods {
			decision.ReasonCode = REASON_EXTERNAL_SIDE_EFFECT_REQUIRES_APPROVAL
			decision.RequiresHuman = true
			decision.FinalStatus = RESEARCH_REQUIRES_HUMAN
			return decision
		}
	}

	if method == string(METHOD_GET) && !p.AllowPublicGET {
		decision.ReasonCode = REASON_METHOD_NOT_ALLOWED
		return decision
	}
	if method == string(METHOD_HEAD) && !p.AllowHEAD {
		decision.ReasonCode = REASON_METHOD_NOT_ALLOWED
		return decision
	}

	normalized, parsed, ok := validateURL(req.URL, p)
	if !ok {
		decision.ReasonCode = REASON_URL_INVALID
		return decision
	}
	decision.NormalizedURL = normalized

	if !IsSupportedScheme(parsed, p.AllowedSchemes) {
		decision.ReasonCode = REASON_UNSUPPORTED_SCHEME
		return decision
	}
	if IsCredentialedURL(parsed) && !p.AllowCredentials {
		decision.ReasonCode = REASON_CREDENTIALS_FORBIDDEN
		return decision
	}

	host := normalizedHost(parsed.Host)
	if isHostBlocked(host, p.BlockedHosts) {
		decision.ReasonCode = REASON_DOMAIN_BLOCKED
		return decision
	}
	if len(p.AllowedHosts) > 0 && !isHostAllowed(host, p.AllowedHosts) {
		decision.ReasonCode = REASON_DOMAIN_BLOCKED
		return decision
	}

	if !p.AllowPrivateNetworks && (IsPrivateOrLocalHost(host) || HostLooksLikeCloudMetadata(host)) {
		decision.ReasonCode = REASON_PRIVATE_NETWORK_BLOCKED
		return decision
	}

	if req.Kind == REQUEST_DOWNLOAD {
		if !p.AllowDownloads {
			decision.ReasonCode = REASON_METHOD_NOT_ALLOWED
			return decision
		}
		if p.RequireApprovalForDownload {
			decision.RequiresHuman = true
			decision.FinalStatus = RESEARCH_REQUIRES_HUMAN
			decision.ReasonCode = REASON_DOWNLOAD_REQUIRES_QUEUE
			return decision
		}
	}

	decision.Allowed = true
	decision.FinalStatus = RESEARCH_ALLOWED
	decision.ReasonCode = REASON_RESEARCH_READ_ALLOWED
	return decision
}

func inferredMethod(kind RequestKind) string {
	switch kind {
	case REQUEST_HEAD:
		return string(METHOD_HEAD)
	case REQUEST_FETCH, REQUEST_DOWNLOAD:
		return string(METHOD_GET)
	default:
		return string(METHOD_GET)
	}
}

func validateURL(raw string, policy Policy) (string, *url.URL, bool) {
	normalized, err := NormalizeURL(raw)
	if err != nil {
		return "", nil, false
	}
	u, err := url.Parse(normalized)
	if err != nil || u == nil {
		return "", nil, false
	}
	return normalized, u, true
}

func hasCredentialMaterial(headers map[string]string, metadata map[string]any) bool {
	credentialHeaderKeys := []string{
		"authorization", "cookie", "x-api-key", "x-auth-token", "proxy-authorization", "api-key", "bearer",
	}
	credentialMetaKeys := []string{
		"api_key", "api-key", "token", "secret", "password", "credential", "auth", "cookie",
	}

	for k := range headers {
		low := strings.ToLower(strings.TrimSpace(k))
		for _, needle := range credentialHeaderKeys {
			if low == needle || strings.Contains(low, needle) {
				return true
			}
		}
	}

	for k := range metadata {
		low := strings.ToLower(strings.TrimSpace(k))
		for _, needle := range credentialMetaKeys {
			if low == needle || strings.Contains(low, needle) {
				return true
			}
		}
	}

	return false
}

func boundedTimeout(p Policy, timeoutMS int64) time.Duration {
	out := p.DefaultTimeout
	if out <= 0 {
		out = 8 * time.Second
	}
	if timeoutMS > 0 {
		out = time.Duration(timeoutMS) * time.Millisecond
	}
	max := p.MaxTimeout
	if max <= 0 {
		max = 20 * time.Second
	}
	if out > max {
		return max
	}
	if out <= 0 {
		return 8 * time.Second
	}
	return out
}

func boundedMax(requested, policyMax int64) int64 {
	max := policyMax
	if max <= 0 {
		max = 512 * 1024
	}
	if requested <= 0 {
		return max
	}
	if requested > max {
		return max
	}
	return requested
}

func isHostBlocked(host string, blocked []string) bool {
	h := normalizedHost(host)
	for _, b := range blocked {
		if h == normalizedHost(b) {
			return true
		}
	}
	return false
}

func isHostAllowed(host string, allowed []string) bool {
	h := normalizedHost(host)
	for _, a := range allowed {
		if h == normalizedHost(a) {
			return true
		}
	}
	return false
}
