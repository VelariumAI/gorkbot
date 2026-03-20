package spark

import (
	"os"
	"testing"
	"time"
)

func TestMetricsEmitNoBlock(t *testing.T) {
	dir, err := os.MkdirTemp("", "spark-metrics-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	m := NewMetrics(dir)
	defer m.Close()

	// Emit 600 events — more than the 512-slot buffer — must not deadlock.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 600; i++ {
			m.Emit(SPARKEvent{
				Time: time.Now(),
				Kind: EventTIIUpdate,
			})
		}
		close(done)
	}()

	select {
	case <-done:
		// pass
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked for >2s with 600 events")
	}
}

func TestMetricsClose(t *testing.T) {
	dir, err := os.MkdirTemp("", "spark-metrics-close-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	m := NewMetrics(dir)
	for i := 0; i < 10; i++ {
		m.Emit(SPARKEvent{Time: time.Now(), Kind: EventCycleStart})
	}

	done := make(chan struct{})
	go func() {
		m.Close()
		close(done)
	}()

	select {
	case <-done:
		// pass: closed + drained in under 1s
	case <-time.After(1 * time.Second):
		t.Fatal("Close took longer than 1s")
	}
}
