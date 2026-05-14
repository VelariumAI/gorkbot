package trace

import "context"

// NoopSink drops all events.
type NoopSink struct{}

func (NoopSink) Emit(context.Context, Event) error { return nil }
func (NoopSink) Close() error                      { return nil }
