package trace

import "time"

func NewRef(kind, ref, hash string, sizeBytes int64) Ref {
	return Ref{
		Kind:      RedactString(kind, 32),
		Ref:       RedactString(ref, maxRefFieldLen),
		Hash:      RedactString(hash, 128),
		SizeBytes: sizeBytes,
	}
}

func NewMetadata(pairs map[string]string) map[string]string {
	return BoundMetadata(pairs)
}

func NewEventWithTiming(component, kind string, startedAt time.Time, finishedAt time.Time) Event {
	e := NewEvent(component, kind)
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	e.Timestamp = startedAt.UTC()
	if !finishedAt.IsZero() && finishedAt.After(startedAt) {
		e.Duration = finishedAt.Sub(startedAt).Milliseconds()
	}
	return e
}
