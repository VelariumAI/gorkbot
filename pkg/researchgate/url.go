package researchgate

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

var errInvalidURL = errors.New("invalid url")

func NormalizeURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errInvalidURL
	}

	u, err := url.Parse(trimmed)
	if err != nil || u == nil {
		return "", errInvalidURL
	}
	if u.Scheme == "" || u.Host == "" {
		return "", errInvalidURL
	}

	u.Scheme = strings.ToLower(strings.TrimSpace(u.Scheme))
	host := normalizedHost(u.Host)
	if host == "" {
		return "", errInvalidURL
	}
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	if u.Path == "" {
		u.Path = "/"
	}
	u.Fragment = ""

	return u.String(), nil
}

func IsSupportedScheme(u *url.URL, allowed []string) bool {
	if u == nil {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	for _, s := range allowed {
		if scheme == strings.ToLower(strings.TrimSpace(s)) {
			return true
		}
	}
	return false
}

func IsCredentialedURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	return u.User != nil
}

func IsPrivateOrLocalHost(host string) bool {
	h := normalizedHost(host)
	if h == "" {
		return true
	}
	if h == "localhost" || strings.HasPrefix(h, "localhost") {
		return true
	}
	if strings.HasPrefix(h, "127.") {
		return true
	}

	addr, err := netip.ParseAddr(h)
	if err != nil {
		return false
	}

	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return true
	}

	if addr.Is6() {
		// Explicitly guard ULA + link-local networks.
		if addr.IsPrivate() || addr.IsLinkLocalUnicast() {
			return true
		}
	}

	return false
}

func HostLooksLikeCloudMetadata(host string) bool {
	h := normalizedHost(host)
	if h == "" {
		return false
	}

	if h == "metadata" || h == "instance-data" || h == "metadata.google.internal" || h == "imds.amazonaws.com" {
		return true
	}

	if strings.HasSuffix(h, ".metadata.google.internal") {
		return true
	}

	return false
}

func normalizedHost(host string) string {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "" {
		return ""
	}

	if parsed, _, err := net.SplitHostPort(h); err == nil {
		h = parsed
	} else if strings.HasPrefix(h, "[") {
		if end := strings.Index(h, "]"); end > 0 {
			h = h[1:end]
		}
	}

	h = strings.TrimSuffix(h, ".")
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		h = strings.TrimPrefix(strings.TrimSuffix(h, "]"), "[")
	}
	return h
}
