package spark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// EventKind identifies the kind of SPARK trace event.
type EventKind string

const (
	EventCycleStart     EventKind = "cycle_start"
	EventCycleEnd       EventKind = "cycle_end"
	EventTIIUpdate      EventKind = "tii_update"
	EventIDLAdded       EventKind = "idl_added"
	EventDirectiveApply EventKind = "directive_apply"
	EventContextInject  EventKind = "context_inject"
	EventObjectiveAdd   EventKind = "objective_add"
)

// SPARKEvent is a single JSONL trace record.
type SPARKEvent struct {
	Time     time.Time              `json:"t"`
	Kind     EventKind              `json:"kind"`
	ToolName string                 `json:"tool,omitempty"`
	Payload  map[string]interface{} `json:"payload,omitempty"`
}

// Metrics tracks operational counters (atomics) + async JSONL trace writer.
// Mirrors the SENSETracer pattern: buffered channel + drain goroutine, never
// blocks the hot path.
type Metrics struct {
	CyclesTotal       atomic.Int64
	CyclesFailed      atomic.Int64
	InjectionsTotal   atomic.Int64
	DirectivesApplied atomic.Int64

	traceCh   chan SPARKEvent
	traceDir  string
	stopCh    chan struct{}
	drainDone chan struct{}
}

// NewMetrics creates a Metrics instance and starts the drain goroutine.
func NewMetrics(traceDir string) *Metrics {
	m := &Metrics{
		traceCh:   make(chan SPARKEvent, 512),
		traceDir:  traceDir,
		stopCh:    make(chan struct{}),
		drainDone: make(chan struct{}),
	}
	go m.drainLoop()
	return m
}

// Emit enqueues a JSONL event. Non-blocking — drops if channel full.
func (m *Metrics) Emit(e SPARKEvent) {
	select {
	case m.traceCh <- e:
	default:
	}
}

// Close flushes all pending events and stops the drain goroutine.
func (m *Metrics) Close() {
	close(m.stopCh)
	<-m.drainDone
}

func (m *Metrics) drainLoop() {
	defer close(m.drainDone)
	var f *os.File
	var currentDay string
	defer func() {
		if f != nil {
			_ = f.Sync()
			_ = f.Close()
		}
	}()

	writeEvent := func(e SPARKEvent) {
		day := e.Time.Format("2006-01-02")
		if day != currentDay {
			if f != nil {
				_ = f.Close()
			}
			_ = os.MkdirAll(m.traceDir, 0755)
			path := filepath.Join(m.traceDir, day+".jsonl")
			f, _ = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			currentDay = day
		}
		if f != nil {
			if b, err := json.Marshal(e); err == nil {
				_, _ = f.Write(append(b, '\n'))
			}
		}
	}

	for {
		select {
		case e := <-m.traceCh:
			writeEvent(e)
		case <-m.stopCh:
			// Drain remaining events before exiting.
			for {
				select {
				case e := <-m.traceCh:
					writeEvent(e)
				default:
					return
				}
			}
		}
	}
}
