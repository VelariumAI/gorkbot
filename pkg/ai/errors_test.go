package ai

import (
	"errors"
	"testing"
)

func TestMapStatusErrorSentinels(t *testing.T) {
	cases := []struct {
		code int
		want error
	}{
		{401, ErrUnauthorized},
		{403, ErrUnauthorized},
		{402, ErrNoCredits},
		{429, ErrRateLimit},
		{500, ErrProviderDown},
		{502, ErrBadGateway},
		{503, ErrBadGateway},
		{504, ErrBadGateway},
	}

	for _, tc := range cases {
		err := MapStatusError(tc.code, []byte("body"))
		if !errors.Is(err, tc.want) {
			t.Fatalf("code %d: expected %v, got %v", tc.code, tc.want, err)
		}
	}

	err := MapStatusError(418, []byte("teapot"))
	if err == nil {
		t.Fatalf("expected generic error for unknown status")
	}
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrNoCredits) || errors.Is(err, ErrRateLimit) ||
		errors.Is(err, ErrProviderDown) || errors.Is(err, ErrBadGateway) {
		t.Fatalf("unknown status should not wrap known sentinel: %v", err)
	}
}
