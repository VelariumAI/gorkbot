package researchgate

import "time"

type Method string

const (
	METHOD_GET  Method = "GET"
	METHOD_HEAD Method = "HEAD"
)

type RequestKind string

const (
	REQUEST_SEARCH   RequestKind = "SEARCH"
	REQUEST_FETCH    RequestKind = "FETCH"
	REQUEST_HEAD     RequestKind = "HEAD"
	REQUEST_DOWNLOAD RequestKind = "DOWNLOAD"
)

type ResearchRequest struct {
	ID        string            `json:"id"`
	MissionID string            `json:"mission_id,omitempty"`
	Kind      RequestKind       `json:"kind"`
	Method    string            `json:"method"`
	URL       string            `json:"url,omitempty"`
	Query     string            `json:"query,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	MaxBytes  int64             `json:"max_bytes,omitempty"`
	TimeoutMS int64             `json:"timeout_ms,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
