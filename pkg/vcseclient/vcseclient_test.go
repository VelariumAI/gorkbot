package vcseclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("health failed: %v", err)
	}
}

func TestReadySuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	if err := c.Ready(context.Background()); err != nil {
		t.Fatalf("ready failed: %v", err)
	}
}

func TestValidateProposalSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proposal/validate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","valid":true,"accepted":true,"issues":[]}`))
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	res, err := c.ValidateProposal(context.Background(), map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if !res.Valid || !res.Accepted {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestValidateProposalTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","valid":true,"accepted":true,"issues":[]}`))
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: 20 * time.Millisecond, Enabled: true})
	_, err := c.ValidateProposal(context.Background(), map[string]any{"x": 1})
	if err == nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestValidateProposalNon2xx(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	_, err := c.ValidateProposal(context.Background(), map[string]any{"x": 1})
	var hsErr *HTTPStatusError
	if !errors.As(err, &hsErr) {
		t.Fatalf("expected HTTPStatusError, got %T (%v)", err, err)
	}
	if hsErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", hsErr.StatusCode)
	}
}

func TestValidateProposalInvalidJSON(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not-json`))
	}))
	defer s.Close()

	c := New(Config{BaseURL: s.URL, Timeout: time.Second, Enabled: true})
	_, err := c.ValidateProposal(context.Background(), map[string]any{"x": 1})
	if err == nil || !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected invalid response error, got %v", err)
	}
}
