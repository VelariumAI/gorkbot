package researchgate

import "time"

type ProvenanceRecord struct {
	RequestID   string    `json:"request_id"`
	URL         string    `json:"url,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
	StatusCode  int       `json:"status_code,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	BytesRead   int64     `json:"bytes_read,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
}
