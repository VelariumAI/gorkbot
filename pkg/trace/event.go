package trace

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxComponentLen      = 64
	maxEventKindLen      = 64
	maxReasonCodeLen     = 128
	maxDecisionLen       = 64
	maxStatusLen         = 64
	maxErrorClassLen     = 64
	maxRefCount          = 64
	maxRefFieldLen       = 256
	maxMetadataEntries   = 24
	maxMetadataKeyLen    = 64
	maxMetadataValueLen  = 256
	maxTrajectoryRefSize = 256
)

// Operator identifies the planning/execution stage that produced an event.
type Operator string

const (
	OperatorClassify  Operator = "classify"
	OperatorRetrieve  Operator = "retrieve"
	OperatorPlan      Operator = "plan"
	OperatorExecute   Operator = "execute"
	OperatorVerify    Operator = "verify"
	OperatorRepair    Operator = "repair"
	OperatorEscalate  Operator = "escalate"
	OperatorSummarize Operator = "summarize"
	OperatorStage     Operator = "stage"
	OperatorPromote   Operator = "promote"
	OperatorReject    Operator = "reject"
	OperatorUnknown   Operator = "unknown"
)

var validOperators = map[Operator]struct{}{
	OperatorClassify:  {},
	OperatorRetrieve:  {},
	OperatorPlan:      {},
	OperatorExecute:   {},
	OperatorVerify:    {},
	OperatorRepair:    {},
	OperatorEscalate:  {},
	OperatorSummarize: {},
	OperatorStage:     {},
	OperatorPromote:   {},
	OperatorReject:    {},
	OperatorUnknown:   {},
}

func (o Operator) Valid() bool {
	_, ok := validOperators[o]
	return ok
}

func NormalizeOperator(raw string) Operator {
	o := Operator(strings.ToLower(strings.TrimSpace(raw)))
	if !o.Valid() {
		return OperatorUnknown
	}
	return o
}

// RedactionState records whether payload-bearing fields were redacted.
type RedactionState string

const (
	RedactionUnknown  RedactionState = "unknown"
	RedactionRedacted RedactionState = "redacted"
	RedactionClean    RedactionState = "clean"
)

// Ref is a bounded reference to an artifact, receipt, or validation object.
type Ref struct {
	Kind      string `json:"kind,omitempty"`
	Ref       string `json:"ref"`
	Hash      string `json:"hash,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// TokenUsage records model token consumption without storing raw content.
type TokenUsage struct {
	InputTokens      int64 `json:"input_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	CachedInputToken int64 `json:"cached_input_tokens,omitempty"`
}

// CostEstimate stores estimated spend at micro precision.
type CostEstimate struct {
	Currency     string `json:"currency,omitempty"`
	InputMicros  int64  `json:"input_micros,omitempty"`
	OutputMicros int64  `json:"output_micros,omitempty"`
	TotalMicros  int64  `json:"total_micros,omitempty"`
}

// Event is the canonical replay-oriented trace atom.
type Event struct {
	EventID        string            `json:"event_id"`
	TrajectoryID   string            `json:"trajectory_id,omitempty"`
	ParentEventID  string            `json:"parent_event_id,omitempty"`
	Timestamp      time.Time         `json:"timestamp"`
	Component      string            `json:"component"`
	Operator       Operator          `json:"operator"`
	EventKind      string            `json:"event_kind"`
	Decision       string            `json:"decision,omitempty"`
	ReasonCode     string            `json:"reason_code,omitempty"`
	ArtifactRefs   []Ref             `json:"artifact_refs,omitempty"`
	ValidationRefs []Ref             `json:"validation_refs,omitempty"`
	ReceiptRefs    []Ref             `json:"receipt_refs,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Model          string            `json:"model,omitempty"`
	TokenUsage     TokenUsage        `json:"token_usage,omitempty"`
	CostEstimate   CostEstimate      `json:"cost_estimate,omitempty"`
	Duration       int64             `json:"duration_ms,omitempty"`
	Status         string            `json:"status,omitempty"`
	ErrorClass     string            `json:"error_class,omitempty"`
	RedactionState RedactionState    `json:"redaction_state,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func NewEvent(component, eventKind string) Event {
	return Event{
		EventID:        NewEventID(),
		Timestamp:      time.Now().UTC(),
		Component:      strings.TrimSpace(component),
		Operator:       OperatorUnknown,
		EventKind:      strings.TrimSpace(eventKind),
		RedactionState: RedactionUnknown,
	}
}

func (e Event) Normalized() Event {
	out := e
	if strings.TrimSpace(out.EventID) == "" {
		out.EventID = NewEventID()
	}
	if out.Timestamp.IsZero() {
		out.Timestamp = time.Now().UTC()
	}
	if !out.Operator.Valid() {
		out.Operator = OperatorUnknown
	}
	if out.RedactionState == "" {
		out.RedactionState = RedactionUnknown
	}
	out.Component = truncateString(strings.TrimSpace(out.Component), maxComponentLen)
	out.EventKind = truncateString(strings.TrimSpace(out.EventKind), maxEventKindLen)
	out.Decision = truncateString(strings.TrimSpace(out.Decision), maxDecisionLen)
	out.ReasonCode = truncateString(strings.TrimSpace(out.ReasonCode), maxReasonCodeLen)
	out.Status = truncateString(strings.TrimSpace(out.Status), maxStatusLen)
	out.ErrorClass = truncateString(strings.TrimSpace(out.ErrorClass), maxErrorClassLen)
	out.TrajectoryID = truncateString(strings.TrimSpace(out.TrajectoryID), maxTrajectoryRefSize)
	out.ParentEventID = truncateString(strings.TrimSpace(out.ParentEventID), maxTrajectoryRefSize)
	out.Metadata = BoundMetadata(out.Metadata)
	out.ArtifactRefs = boundRefs(out.ArtifactRefs)
	out.ValidationRefs = boundRefs(out.ValidationRefs)
	out.ReceiptRefs = boundRefs(out.ReceiptRefs)
	if out.Duration < 0 {
		out.Duration = 0
	}
	if out.TokenUsage.InputTokens < 0 {
		out.TokenUsage.InputTokens = 0
	}
	if out.TokenUsage.OutputTokens < 0 {
		out.TokenUsage.OutputTokens = 0
	}
	if out.TokenUsage.CachedInputToken < 0 {
		out.TokenUsage.CachedInputToken = 0
	}
	if out.CostEstimate.InputMicros < 0 {
		out.CostEstimate.InputMicros = 0
	}
	if out.CostEstimate.OutputMicros < 0 {
		out.CostEstimate.OutputMicros = 0
	}
	if out.CostEstimate.TotalMicros < 0 {
		out.CostEstimate.TotalMicros = 0
	}
	out.CostEstimate.Currency = truncateString(strings.TrimSpace(out.CostEstimate.Currency), 8)
	out.Provider = truncateString(strings.TrimSpace(out.Provider), maxComponentLen)
	out.Model = truncateString(strings.TrimSpace(out.Model), maxComponentLen)
	return out
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.EventID) == "" {
		return errors.New("trace event requires event_id")
	}
	if e.Timestamp.IsZero() {
		return errors.New("trace event requires timestamp")
	}
	if strings.TrimSpace(e.Component) == "" {
		return errors.New("trace event requires component")
	}
	if strings.TrimSpace(e.EventKind) == "" {
		return errors.New("trace event requires event_kind")
	}
	if !e.Operator.Valid() {
		return fmt.Errorf("trace event has invalid operator %q", e.Operator)
	}
	if len(e.ArtifactRefs) > maxRefCount {
		return fmt.Errorf("trace event artifact refs exceed %d", maxRefCount)
	}
	if len(e.ValidationRefs) > maxRefCount {
		return fmt.Errorf("trace event validation refs exceed %d", maxRefCount)
	}
	if len(e.ReceiptRefs) > maxRefCount {
		return fmt.Errorf("trace event receipt refs exceed %d", maxRefCount)
	}
	if len(e.Metadata) > maxMetadataEntries {
		return fmt.Errorf("trace event metadata entries exceed %d", maxMetadataEntries)
	}
	return nil
}

func boundRefs(in []Ref) []Ref {
	if len(in) == 0 {
		return nil
	}
	if len(in) > maxRefCount {
		in = in[:maxRefCount]
	}
	out := make([]Ref, 0, len(in))
	for i := range in {
		ref := Ref{
			Kind:      truncateString(strings.TrimSpace(in[i].Kind), 32),
			Ref:       truncateString(strings.TrimSpace(in[i].Ref), maxRefFieldLen),
			Hash:      truncateString(strings.TrimSpace(in[i].Hash), 128),
			SizeBytes: in[i].SizeBytes,
		}
		if ref.SizeBytes < 0 {
			ref.SizeBytes = 0
		}
		if ref.Ref == "" {
			continue
		}
		out = append(out, ref)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncateString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// BoundMetadata applies the canonical metadata bounds and key/value redaction.
func BoundMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, minInt(len(in), maxMetadataEntries))
	for k, v := range in {
		if len(out) >= maxMetadataEntries {
			break
		}
		key := truncateString(strings.TrimSpace(k), maxMetadataKeyLen)
		if key == "" {
			continue
		}
		if IsSensitiveKey(key) {
			out[key] = "[REDACTED]"
			continue
		}
		clean := truncateString(strings.TrimSpace(v), maxMetadataValueLen)
		if clean == "" {
			continue
		}
		out[key] = clean
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
