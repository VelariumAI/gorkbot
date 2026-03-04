package ai

// transport.go — Hardened HTTP transport for mobile / roaming environments.
//
// Designed to survive:
//   - IPv6 cellular handoffs (new source address mid-stream)
//   - TLS MAC errors on reused TCP connections after a roam
//   - Connection resets / ECONNRESET during long-running AI streams
//   - Aggressive NAT timeouts on Android Termux
//
// Every AI provider client (Grok, Gemini, Anthropic, OpenAI, MiniMax,
// OpenRouter) calls NewHardenedTransport() as its base *http.Transport so
// keep-alive and TLS settings are uniform across the board.

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// hardenedDialer returns a net.Dialer tuned for mobile networks:
//   - 30 s TCP-connect timeout (generous for congested mobile links)
//   - 15 s TCP keep-alive interval (aggressive — detects stale connections
//     before a stream hangs for minutes after a cell handoff)
func hardenedDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 15 * time.Second,
	}
}

// NewHardenedTransport returns an *http.Transport with:
//
//	TCP keep-alive      15 s  — drops dead connections quickly after roam
//	TLS handshake       15 s  — avoids hanging forever on a bad cell tower
//	Response header     30 s  — provider must send headers within 30 s
//	Idle conn timeout   90 s  — recycles connections before NAT kills them
//	MaxIdleConnsPerHost 10    — enough for parallel streaming requests
//	TLS min version     1.2   — required by all current AI providers
//
// This replaces http.DefaultTransport everywhere in the AI layer.
func NewHardenedTransport() *http.Transport {
	d := hardenedDialer()
	return &http.Transport{
		DialContext:             d.DialContext,
		TLSHandshakeTimeout:     15 * time.Second,
		ResponseHeaderTimeout:   30 * time.Second,
		ExpectContinueTimeout:    1 * time.Second,
		IdleConnTimeout:         90 * time.Second,
		MaxIdleConns:            100,
		MaxIdleConnsPerHost:      10,
		DisableKeepAlives:       false,
		ForceAttemptHTTP2:       true,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
}

// isTransientNetworkError returns true for low-level errors that indicate a
// dropped or corrupted connection rather than an application-level failure.
// These warrant a fresh-connection retry without cascading to another provider.
//
// Covered conditions:
//   - io.EOF / io.ErrUnexpectedEOF  — server closed connection mid-stream
//   - "tls: bad record mac"         — TLS session corrupted after IP change
//   - "connection reset by peer"    — RST after NAT eviction / cell handoff
//   - "broken pipe"                 — write to dead connection
//   - "use of closed network connection" — local socket already closed
//   - net.Error with Timeout()      — dialer / TLS / header read timeout
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// io sentinel values
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return true
	}

	// net.Error timeout (covers TLSHandshakeTimeout, ResponseHeaderTimeout,
	// DialContext timeout — all of which indicate the network, not the app)
	var netErr net.Error
	if asNetError(err, &netErr) && netErr.Timeout() {
		return true
	}

	// String-pattern fallback for errors wrapped behind url.Error or similar.
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{
		"connection reset by peer",
		"broken pipe",
		"use of closed network connection",
		"tls: bad record mac",
		"network is unreachable",
		"no route to host",
		"i/o timeout",
		"eof",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// asNetError is a helper that unwraps err into a *net.Error without importing
// the errors package (which is already imported via errors.go in this package).
func asNetError(err error, target *net.Error) bool {
	if ne, ok := err.(net.Error); ok {
		*target = ne
		return true
	}
	// Unwrap one level (covers url.Error wrapping a net.Error).
	type unwrapper interface{ Unwrap() error }
	if uw, ok := err.(unwrapper); ok {
		return asNetError(uw.Unwrap(), target)
	}
	return false
}
