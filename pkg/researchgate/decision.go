package researchgate

import "time"

import "github.com/velariumai/gorkbot/pkg/harness"

const (
	RESEARCH_ALLOWED        = "RESEARCH_ALLOWED"
	RESEARCH_BLOCKED        = "RESEARCH_BLOCKED"
	RESEARCH_REQUIRES_HUMAN = "RESEARCH_REQUIRES_HUMAN"
	RESEARCH_DEGRADED       = "RESEARCH_DEGRADED"
)

const (
	REASON_RESEARCH_READ_ALLOWED                  = "REASON_RESEARCH_READ_ALLOWED"
	REASON_METHOD_NOT_ALLOWED                     = "REASON_METHOD_NOT_ALLOWED"
	REASON_PRIVATE_NETWORK_BLOCKED                = "REASON_PRIVATE_NETWORK_BLOCKED"
	REASON_CREDENTIALS_FORBIDDEN                  = "REASON_CREDENTIALS_FORBIDDEN"
	REASON_UNSUPPORTED_SCHEME                     = "REASON_UNSUPPORTED_SCHEME"
	REASON_URL_INVALID                            = "REASON_URL_INVALID"
	REASON_BODY_FORBIDDEN                         = "REASON_BODY_FORBIDDEN"
	REASON_UPLOAD_FORBIDDEN                       = "REASON_UPLOAD_FORBIDDEN"
	REASON_RESPONSE_TOO_LARGE                     = "REASON_RESPONSE_TOO_LARGE"
	REASON_TIMEOUT                                = "REASON_TIMEOUT"
	REASON_RATE_LIMITED                           = "REASON_RATE_LIMITED"
	REASON_DOMAIN_BLOCKED                         = "REASON_DOMAIN_BLOCKED"
	REASON_DOWNLOAD_REQUIRES_QUEUE                = "REASON_DOWNLOAD_REQUIRES_QUEUE"
	REASON_EXTERNAL_SIDE_EFFECT_REQUIRES_APPROVAL = "REASON_EXTERNAL_SIDE_EFFECT_REQUIRES_APPROVAL"
)

type ResearchDecision struct {
	RequestID     string   `json:"request_id"`
	Allowed       bool     `json:"allowed"`
	RequiresHuman bool     `json:"requires_human"`
	FinalStatus   string   `json:"final_status"`
	ReasonCode    string   `json:"reason_code"`
	Issues        []string `json:"issues,omitempty"`
	RiskClass     string   `json:"risk_class,omitempty"`
	NormalizedURL string   `json:"normalized_url,omitempty"`
	TimeoutMS     int64    `json:"timeout_ms"`
	MaxBytes      int64    `json:"max_bytes"`
}

type ResearchResult struct {
	RequestID    string                `json:"request_id"`
	URL          string                `json:"url,omitempty"`
	Query        string                `json:"query,omitempty"`
	StatusCode   int                   `json:"status_code,omitempty"`
	ContentType  string                `json:"content_type,omitempty"`
	BytesRead    int64                 `json:"bytes_read,omitempty"`
	SHA256       string                `json:"sha256,omitempty"`
	StoredAt     string                `json:"stored_at,omitempty"`
	FetchedAt    time.Time             `json:"fetched_at"`
	FromCache    bool                  `json:"from_cache"`
	Metadata     map[string]any        `json:"metadata,omitempty"`
	BodyPreview  string                `json:"body_preview,omitempty"`
	AuditSummary *harness.AuditSummary `json:"audit_summary,omitempty"`
}
