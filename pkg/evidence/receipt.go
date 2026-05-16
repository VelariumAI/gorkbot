package evidence

import (
	"fmt"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

// Receipt aggregates records and an assessment into a bounded audit artifact.
type Receipt struct {
	ID           string            `json:"id"`
	Records      []Record          `json:"records,omitempty"`
	Assessment   Assessment        `json:"assessment"`
	Status       Status            `json:"status"`
	Summary      string            `json:"summary"`
	EvidenceRefs []trace.Ref       `json:"evidence_refs,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   time.Time         `json:"finished_at"`
	DurationMS   int64             `json:"duration_ms"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func (r Receipt) Normalized() Receipt {
	out := r
	if len(out.Records) > maxRecordCount {
		out.Records = out.Records[:maxRecordCount]
	}
	for i := range out.Records {
		out.Records[i] = out.Records[i].Normalized()
	}
	out.Assessment = Evaluate(out.Assessment)
	out.Status = NormalizeStatus(string(out.Status))
	out.Summary = boundString(out.Summary, maxSummaryLen)
	out.EvidenceRefs = boundRefs(out.EvidenceRefs, maxRefCount)
	out.Metadata = boundMetadata(out.Metadata, maxMetadataCount)
	if out.StartedAt.IsZero() {
		out.StartedAt = zeroTime
	} else {
		out.StartedAt = out.StartedAt.UTC()
	}
	if out.FinishedAt.IsZero() || out.FinishedAt.Before(out.StartedAt) {
		out.FinishedAt = out.StartedAt
	} else {
		out.FinishedAt = out.FinishedAt.UTC()
	}
	if out.DurationMS < 0 {
		out.DurationMS = 0
	}
	if out.DurationMS == 0 {
		out.DurationMS = out.FinishedAt.Sub(out.StartedAt).Milliseconds()
		if out.DurationMS < 0 {
			out.DurationMS = 0
		}
	}

	if len(out.Records) > 0 && allRecordsIndeterminate(out.Records) && out.Status == StatusPass {
		out.Status = StatusInconclusive
	}
	if out.Status == StatusUnknown || out.Status == StatusInvalid {
		out.Status = deriveReceiptStatus(out.Records)
	}
	if out.ID == "" {
		recordIDs := make([]string, 0, len(out.Records))
		for i := range out.Records {
			recordIDs = append(recordIDs, out.Records[i].ID)
		}
		out.ID = "receipt_" + trace.StableHash(
			trace.StableHash(recordIDs...),
			out.Assessment.ID,
			string(out.Status),
			out.Summary,
			stableRefsHash(out.EvidenceRefs),
			stableMetadataHash(out.Metadata),
		)
	}
	return out
}

func (r Receipt) Validate() error {
	n := r.Normalized()
	if n.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidReceipt)
	}
	if n.DurationMS < 0 {
		return fmt.Errorf("%w: negative duration", ErrInvalidReceipt)
	}
	if err := n.Assessment.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidReceipt, err)
	}
	for i := range n.Records {
		if err := n.Records[i].Validate(); err != nil {
			return fmt.Errorf("%w: record[%d]: %v", ErrInvalidReceipt, i, err)
		}
	}
	// Assessment-only pass receipts are intentional. A pass can be valid with
	// zero records when the normalized assessment validates.
	if n.Status == StatusPass && allRecordsIndeterminate(n.Records) {
		return fmt.Errorf("%w: pass with only skipped/unavailable/inconclusive", ErrInvalidReceipt)
	}
	return nil
}

func allRecordsIndeterminate(records []Record) bool {
	if len(records) == 0 {
		return false
	}
	for i := range records {
		s := NormalizeStatus(string(records[i].Status))
		if s != StatusSkipped && s != StatusUnavailable && s != StatusInconclusive {
			return false
		}
	}
	return true
}

func deriveReceiptStatus(records []Record) Status {
	if len(records) == 0 {
		return StatusInconclusive
	}
	seenWarn := false
	seenPass := false
	for i := range records {
		s := NormalizeStatus(string(records[i].Status))
		switch s {
		case StatusFail:
			return StatusFail
		case StatusWarn:
			seenWarn = true
		case StatusPass:
			seenPass = true
		}
	}
	if seenWarn {
		return StatusWarn
	}
	if seenPass {
		return StatusPass
	}
	return StatusInconclusive
}
