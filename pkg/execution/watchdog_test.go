package execution

import (
	"testing"
	"time"
)

func TestWatchdogEnterUpdatesEnteredAt(t *testing.T) {
	wd := NewWatchdog(DefaultBudget())
	first := wd.enteredAt
	time.Sleep(5 * time.Millisecond)
	wd.Enter(TURN_DECIDING)
	if !wd.enteredAt.After(first) {
		t.Fatalf("expected enteredAt to advance")
	}
	if got := wd.State(); got != TURN_DECIDING {
		t.Fatalf("expected state TURN_DECIDING, got %s", got)
	}
}

func TestWatchdogIdleTimeoutTriggers(t *testing.T) {
	b := DefaultBudget()
	b.MaxIdleDuration = 15 * time.Millisecond
	wd := NewWatchdog(b)
	time.Sleep(25 * time.Millisecond)
	if !wd.TimedOut() {
		t.Fatal("expected idle timeout")
	}
}

func TestWatchdogProgressResetsIdleTimer(t *testing.T) {
	b := DefaultBudget()
	b.MaxIdleDuration = 20 * time.Millisecond
	wd := NewWatchdog(b)
	time.Sleep(10 * time.Millisecond)
	wd.Progress("keepalive")
	time.Sleep(10 * time.Millisecond)
	if wd.TimedOut() {
		t.Fatal("did not expect timeout after progress reset")
	}
}

func TestWatchdogCancelReason(t *testing.T) {
	wd := NewWatchdog(DefaultBudget())
	wd.Cancel("manual")
	if !wd.Cancelled() {
		t.Fatal("expected cancelled=true")
	}
	if wd.TimeoutReason() != "manual" {
		t.Fatalf("unexpected cancel reason: %q", wd.TimeoutReason())
	}
}
