package ai

import (
	"net/http"
	"net/http/httptest"
	"net/url"
)

type rewriteHostTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (r *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u := *clone.URL
	u.Scheme = r.target.Scheme
	u.Host = r.target.Host
	clone.URL = &u
	return r.base.RoundTrip(clone)
}

func rewrittenClientForServer(srv *httptest.Server) *http.Client {
	target, _ := url.Parse(srv.URL)
	return &http.Client{
		Transport: &rewriteHostTransport{
			target: target,
			base:   srv.Client().Transport,
		},
	}
}
