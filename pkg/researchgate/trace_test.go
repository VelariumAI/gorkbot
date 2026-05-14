package researchgate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type researchCaptureSink struct {
	events []trace.Event
}

func (c *researchCaptureSink) Emit(_ context.Context, e trace.Event) error {
	c.events = append(c.events, e)
	return nil
}

func (c *researchCaptureSink) Close() error { return nil }

func TestGatewayEmitsTraceEvent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok")
	}))
	defer ts.Close()

	g, vurl := gatewayForServer(t, DefaultPolicy(), ts, "public.test:80")
	sink := &researchCaptureSink{}
	g.SetTraceSink(sink, trace.ModeAudit)

	_, _, err := g.Fetch(context.Background(), ResearchRequest{ID: "trace-1", Kind: REQUEST_FETCH, Method: "GET", URL: vurl, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(sink.events) == 0 {
		t.Fatalf("expected trace event")
	}
	if sink.events[0].EventKind != "research_egress" {
		t.Fatalf("unexpected event kind: %s", sink.events[0].EventKind)
	}
}
