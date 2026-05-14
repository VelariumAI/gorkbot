package trace

import (
	"context"
	"testing"
)

func BenchmarkNoopSinkEmit(b *testing.B) {
	var sink NoopSink
	e := NewEvent("sense", "tool_success")
	e.Operator = OperatorExecute
	ctx := context.Background()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Emit(ctx, sink, ModeReplay, e)
	}
}

func BenchmarkEventNormalized(b *testing.B) {
	e := Event{
		Component:    "researchgate",
		EventKind:    "research_egress",
		Operator:     OperatorRetrieve,
		Metadata:     map[string]string{"k": "v", "token": "secret"},
		TrajectoryID: "traj",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = e.Normalized()
	}
}
