package execution

import (
	"testing"
	"time"
)

func TestCircuitBreakerClosedAllows(t *testing.T) {
	b := NewCircuitBreaker("x", 2, 50*time.Millisecond)
	if !b.Allow() {
		t.Fatal("closed breaker should allow")
	}
}

func TestCircuitBreakerFailuresOpen(t *testing.T) {
	b := NewCircuitBreaker("x", 2, 50*time.Millisecond)
	b.RecordFailure("a")
	if b.State() != BREAKER_CLOSED {
		t.Fatalf("expected closed after first failure")
	}
	b.RecordFailure("b")
	if b.State() != BREAKER_OPEN {
		t.Fatalf("expected open after threshold")
	}
}

func TestCircuitBreakerOpenBlocks(t *testing.T) {
	b := NewCircuitBreaker("x", 1, 1*time.Second)
	b.RecordFailure("boom")
	if b.Allow() {
		t.Fatal("open breaker should block before cooldown")
	}
}

func TestCircuitBreakerCooldownHalfOpenProbe(t *testing.T) {
	b := NewCircuitBreaker("x", 1, 20*time.Millisecond)
	b.RecordFailure("boom")
	time.Sleep(25 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("expected cooldown to permit probe")
	}
	if b.State() != BREAKER_HALF_OPEN {
		t.Fatalf("expected HALF_OPEN, got %s", b.State())
	}
}

func TestCircuitBreakerHalfOpenSuccessCloses(t *testing.T) {
	b := NewCircuitBreaker("x", 1, 20*time.Millisecond)
	b.RecordFailure("boom")
	time.Sleep(25 * time.Millisecond)
	_ = b.Allow()
	b.RecordSuccess()
	if b.State() != BREAKER_CLOSED {
		t.Fatalf("expected CLOSED after half-open success, got %s", b.State())
	}
}

func TestCircuitBreakerHalfOpenFailureReopens(t *testing.T) {
	b := NewCircuitBreaker("x", 1, 20*time.Millisecond)
	b.RecordFailure("boom")
	time.Sleep(25 * time.Millisecond)
	_ = b.Allow()
	b.RecordFailure("again")
	if b.State() != BREAKER_OPEN {
		t.Fatalf("expected OPEN after half-open failure, got %s", b.State())
	}
}
