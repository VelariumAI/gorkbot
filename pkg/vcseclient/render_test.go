package vcseclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerifyRenderedAnswerSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/render/verify" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"answer_id":"ans-1",
			"final_status":"RENDER_VALID",
			"valid":true,
			"reason_code":"RENDER_GUARD_PASSED",
			"issues":[],
			"claim_count":1,
			"accepted_claim_ids":["c1"],
			"rejected_claim_ids":[],
			"render_mode":"CANONICAL_ONLY"
		}`))
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	res, err := c.VerifyRenderedAnswer(context.Background(), RenderVerifyRequest{
		Answer: map[string]any{"answer_id": "ans-1"},
		Claims: []any{},
	})
	if err != nil {
		t.Fatalf("verify rendered answer failed: %v", err)
	}
	if !res.Valid || res.ReasonCode != "RENDER_GUARD_PASSED" {
		t.Fatalf("unexpected response: %#v", res)
	}
}

func TestVerifyRenderedAnswerInvalidDecision(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"answer_id":"ans-2",
			"final_status":"RENDER_INVALID",
			"valid":false,
			"reason_code":"CLAIM_STATUS_NOT_ALLOWED",
			"issues":["claim rejected"],
			"claim_count":2,
			"accepted_claim_ids":["c1"],
			"rejected_claim_ids":["c2"],
			"render_mode":"CANONICAL_ONLY"
		}`))
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	res, err := c.VerifyRenderedAnswer(context.Background(), RenderVerifyRequest{
		Answer: map[string]any{"answer_id": "ans-2"},
		Claims: []any{},
	})
	if err != nil {
		t.Fatalf("verify rendered answer failed: %v", err)
	}
	if res.Valid {
		t.Fatalf("expected invalid decision: %#v", res)
	}
	if res.ReasonCode != "CLAIM_STATUS_NOT_ALLOWED" {
		t.Fatalf("unexpected reason code: %s", res.ReasonCode)
	}
}

func TestVerifyRenderedAnswerTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(120 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true}`))
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: 30 * time.Millisecond, Enabled: true})
	_, err := c.VerifyRenderedAnswer(context.Background(), RenderVerifyRequest{
		Answer: map[string]any{"answer_id": "ans-3"},
		Claims: []any{},
	})
	if err == nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestVerifyRenderedAnswerNon2xx(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad render", http.StatusBadRequest)
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	_, err := c.VerifyRenderedAnswer(context.Background(), RenderVerifyRequest{
		Answer: map[string]any{"answer_id": "ans-4"},
		Claims: []any{},
	})
	var hsErr *HTTPStatusError
	if !errors.As(err, &hsErr) {
		t.Fatalf("expected HTTPStatusError, got %T (%v)", err, err)
	}
	if hsErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", hsErr.StatusCode)
	}
}

func TestVerifyRenderedAnswerInvalidJSON(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{bad-json`))
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	_, err := c.VerifyRenderedAnswer(context.Background(), RenderVerifyRequest{
		Answer: map[string]any{"answer_id": "ans-5"},
		Claims: []any{},
	})
	if err == nil || !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected invalid response error, got %v", err)
	}
}
