package governance

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestApprovalRuntimeTimeoutDoesNotBlockCaller(t *testing.T) {
	runtime := NewApprovalRuntime(1)
	block := make(chan struct{})
	started := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := runtime.Run(ctx, "key-1", func() (ApprovalResult, error) {
		close(started)
		<-block
		return ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != APPROVAL_TIMEOUT {
		t.Fatalf("expected timeout, got %#v", res)
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatalf("runtime should return promptly on timeout")
	}
	close(block)
}

func TestApprovalRuntimeMaxInflightBoundsWorkers(t *testing.T) {
	runtime := NewApprovalRuntime(1)
	var started int32
	firstReady := make(chan struct{})
	firstBlock := make(chan struct{})
	firstDone := make(chan struct{})

	go func() {
		defer close(firstDone)
		_, _ = runtime.Run(context.Background(), "key-a", func() (ApprovalResult, error) {
			atomic.AddInt32(&started, 1)
			close(firstReady)
			<-firstBlock
			return ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
		})
	}()

	select {
	case <-firstReady:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("first approval callback did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	res, err := runtime.Run(ctx, "key-b", func() (ApprovalResult, error) {
		atomic.AddInt32(&started, 1)
		return ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != APPROVAL_TIMEOUT {
		t.Fatalf("expected timeout while waiting for inflight slot, got %#v", res)
	}
	if atomic.LoadInt32(&started) != 1 {
		t.Fatalf("expected only one started callback, got %d", started)
	}

	close(firstBlock)
	select {
	case <-firstDone:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("first approval call did not complete")
	}
}

func TestApprovalRuntimeDuplicateKeyJoinsInflight(t *testing.T) {
	runtime := NewApprovalRuntime(2)
	var started int32

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type out struct {
		res ApprovalResult
		err error
	}
	ch1 := make(chan out, 1)
	ch2 := make(chan out, 1)

	run := func(ch chan<- out) {
		res, err := runtime.Run(ctx, "dup-key", func() (ApprovalResult, error) {
			atomic.AddInt32(&started, 1)
			time.Sleep(40 * time.Millisecond)
			return ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_SESSION}, nil
		})
		ch <- out{res: res, err: err}
	}

	go run(ch1)
	go run(ch2)

	o1 := <-ch1
	o2 := <-ch2
	if o1.err != nil || o2.err != nil {
		t.Fatalf("unexpected errs: %v %v", o1.err, o2.err)
	}
	if o1.res.Decision != APPROVAL_GRANTED || o2.res.Decision != APPROVAL_GRANTED {
		t.Fatalf("expected both approvals granted: %#v %#v", o1.res, o2.res)
	}
	if atomic.LoadInt32(&started) != 1 {
		t.Fatalf("expected one callback invocation, got %d", started)
	}
}

func TestApprovalRuntimeShutdownCancelsWaiters(t *testing.T) {
	runtime := NewApprovalRuntime(1)
	firstStarted := make(chan struct{})
	firstBlock := make(chan struct{})
	firstDone := make(chan struct{})

	go func() {
		defer close(firstDone)
		_, _ = runtime.Run(context.Background(), "key-1", func() (ApprovalResult, error) {
			close(firstStarted)
			<-firstBlock
			return ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
		})
	}()

	select {
	case <-firstStarted:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("first callback did not start")
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	waiterDone := make(chan ApprovalResult, 1)
	go func() {
		res, _ := runtime.Run(waitCtx, "key-2", func() (ApprovalResult, error) {
			return ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
		})
		waiterDone <- res
	}()

	time.Sleep(20 * time.Millisecond)
	runtime.Shutdown()

	select {
	case res := <-waiterDone:
		if res.Decision != APPROVAL_CANCELLED {
			t.Fatalf("expected waiter cancelled on shutdown, got %#v", res)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("waiter did not return after shutdown")
	}

	close(firstBlock)
	select {
	case <-firstDone:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("first callback did not complete")
	}
}

func TestApprovalRuntimeShutdownIdempotent(t *testing.T) {
	runtime := NewApprovalRuntime(1)
	runtime.Shutdown()
	runtime.Shutdown()
	if !runtime.IsShutdown() {
		t.Fatal("expected runtime to report shutdown")
	}
}

func TestApprovalRuntimeCallerTimeoutStillReleasesCapacityOnCompletion(t *testing.T) {
	runtime := NewApprovalRuntime(1)
	unblock := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	res, err := runtime.Run(ctx, "key-timeout", func() (ApprovalResult, error) {
		<-unblock
		return ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Decision != APPROVAL_TIMEOUT {
		t.Fatalf("expected timeout, got %#v", res)
	}

	close(unblock)
	deadline := time.Now().Add(500 * time.Millisecond)
	for runtime.InflightCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("inflight callback did not clean up")
		}
		time.Sleep(10 * time.Millisecond)
	}

	res2, err := runtime.Run(context.Background(), "key-next", func() (ApprovalResult, error) {
		return ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
	})
	if err != nil {
		t.Fatalf("unexpected err on second run: %v", err)
	}
	if res2.Decision != APPROVAL_GRANTED {
		t.Fatalf("expected second run granted, got %#v", res2)
	}
}
