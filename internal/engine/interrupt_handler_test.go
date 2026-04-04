package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestInterruptHandler_NoInterrupt(t *testing.T) {
	ih := NewInterruptHandler(1 * time.Second)
	defer ih.Shutdown()

	if ih.IsInterrupted() {
		t.Error("expected no interrupt initially")
	}
}

func TestInterruptHandler_UserInterrupt(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)
	defer ih.Shutdown()

	ih.RequestInterrupt(ReasonUserInterrupt, "user pressed stop")

	if !ih.IsInterrupted() {
		t.Error("expected interrupt to be set")
	}

	reason, msg := func() (InterruptReason, string) {
		select {
		case <-ih.Context().Done():
			ih.mu.RLock()
			r, m := ih.reason, ih.message
			ih.mu.RUnlock()
			return r, m
		case <-time.After(1 * time.Second):
			return ReasonNone, ""
		}
	}()

	if reason != ReasonUserInterrupt {
		t.Errorf("expected ReasonUserInterrupt, got %v", reason)
	}
	if msg != "user pressed stop" {
		t.Errorf("expected 'user pressed stop', got %s", msg)
	}
}

func TestInterruptHandler_Timeout(t *testing.T) {
	ih := NewInterruptHandler(100 * time.Millisecond)
	defer ih.Shutdown()

	// Wait for timeout
	<-ih.Context().Done()

	// Give monitoring goroutine time to set interrupt flag
	time.Sleep(10 * time.Millisecond)

	if !ih.IsInterrupted() {
		t.Error("expected timeout to cause interrupt")
	}
}

func TestInterruptHandler_CleanupHandlers(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)

	cleanupOrder := []string{}
	cleanupMu := &sync.Mutex{}

	// Register cleanup handlers
	ih.RegisterCleanup("first", func() error {
		cleanupMu.Lock()
		cleanupOrder = append(cleanupOrder, "first")
		cleanupMu.Unlock()
		return nil
	})

	ih.RegisterCleanup("second", func() error {
		cleanupMu.Lock()
		cleanupOrder = append(cleanupOrder, "second")
		cleanupMu.Unlock()
		return nil
	})

	// Shutdown should execute in reverse order
	errs := ih.Shutdown()

	if len(errs) != 0 {
		t.Errorf("expected no errors, got %d", len(errs))
	}

	if len(cleanupOrder) != 2 {
		t.Errorf("expected 2 cleanups, got %d", len(cleanupOrder))
	}

	// Check LIFO order
	if cleanupOrder[0] != "second" || cleanupOrder[1] != "first" {
		t.Errorf("expected LIFO order, got %v", cleanupOrder)
	}
}

func TestInterruptHandler_CleanupErrors(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)

	errMsg := "cleanup failed"
	ih.RegisterCleanup("failing", func() error {
		return errors.New(errMsg)
	})

	ih.RegisterCleanup("passing", func() error {
		return nil
	})

	errs := ih.Shutdown()

	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}

	if errs[0].Error() != errMsg {
		t.Errorf("expected '%s', got '%s'", errMsg, errs[0])
	}
}

func TestInterruptHandler_StreamingInterruptAware(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)

	tokenCount := 0
	wrapped := ih.StreamingInterruptAware(func(token string) error {
		tokenCount++
		return nil
	})

	// Should process tokens normally
	if err := wrapped("hello"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if tokenCount != 1 {
		t.Errorf("expected 1 token processed, got %d", tokenCount)
	}

	// Request interrupt
	ih.RequestInterrupt(ReasonUserInterrupt, "stop")

	// Should return context.Canceled
	err := wrapped("world")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestInterruptHandler_InterruptHook(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)
	defer ih.Shutdown()

	hookCalled := false
	var hookReason InterruptReason
	var hookMsg string

	ih.SetInterruptCallback(func(reason InterruptReason, msg string) {
		hookCalled = true
		hookReason = reason
		hookMsg = msg
	})

	ih.RequestInterrupt(ReasonResourceLimit, "budget exceeded")

	// Give hook time to execute
	time.Sleep(10 * time.Millisecond)

	if !hookCalled {
		t.Error("expected interrupt hook to be called")
	}

	if hookReason != ReasonResourceLimit || hookMsg != "budget exceeded" {
		t.Errorf("hook received unexpected values: %v, %s", hookReason, hookMsg)
	}
}

func TestInterruptHandler_StatusReport(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)
	defer ih.Shutdown()

	// Before interrupt
	status := ih.StatusReport()
	if status != "No interrupt requested" {
		t.Errorf("unexpected status: %s", status)
	}

	// After interrupt
	ih.RequestInterrupt(ReasonTimeout, "query timeout")
	status = ih.StatusReport()

	if !contains(status, "timeout") {
		t.Errorf("status missing timeout: %s", status)
	}
}

func TestInterruptHandler_Concurrent(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			if ih.IsInterrupted() {
				done <- false
				return
			}
			time.Sleep(10 * time.Millisecond)
			done <- true
		}(i)
	}

	// Request interrupt after brief delay
	go func() {
		time.Sleep(5 * time.Millisecond)
		ih.RequestInterrupt(ReasonUserInterrupt, "concurrent test")
	}()

	// Collect results
	successCount := 0
	for i := 0; i < 5; i++ {
		if <-done {
			successCount++
		}
	}

	if !ih.IsInterrupted() {
		t.Error("expected interrupt to be set")
	}
}

func TestInterruptHandler_AwaitInterrupt(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)
	defer ih.Shutdown()

	done := make(chan struct{})
	var reason InterruptReason
	var msg string
	go func() {
		reason, msg = ih.AwaitInterrupt()
		close(done)
	}()

	ih.RequestInterrupt(ReasonUserInterrupt, "await-test")

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("AwaitInterrupt did not return in time")
	}

	if reason != ReasonUserInterrupt || msg != "await-test" {
		t.Fatalf("unexpected await interrupt payload: reason=%v msg=%q", reason, msg)
	}
}

func TestInterruptHandler_SetCleanupCallback(t *testing.T) {
	ih := NewInterruptHandler(5 * time.Second)

	called := false
	var seenName string
	var seenErr error
	ih.SetCleanupCallback(func(handler string, err error) {
		called = true
		seenName = handler
		seenErr = err
	})

	ih.RegisterCleanup("ok-handler", func() error { return nil })
	errs := ih.Shutdown()
	if len(errs) != 0 {
		t.Fatalf("unexpected cleanup errors: %v", errs)
	}

	if !called || seenName != "ok-handler" || seenErr != nil {
		t.Fatalf("unexpected cleanup callback payload called=%v name=%q err=%v", called, seenName, seenErr)
	}
}
