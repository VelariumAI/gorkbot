package statelock

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

const (
	maxIDLen        = 128
	maxSubjectLen   = 256
	maxStateHashLen = 256
)

type Lock struct {
	ID             string            `json:"id"`
	Scope          Scope             `json:"scope"`
	Dimension      Dimension         `json:"dimension"`
	Subject        string            `json:"subject"`
	StateHash      string            `json:"state_hash"`
	Status         Status            `json:"status"`
	Source         Source            `json:"source"`
	EvidenceRefs   []trace.Ref       `json:"evidence_refs,omitempty"`
	ValidationRefs []trace.Ref       `json:"validation_refs,omitempty"`
	PolicyState    PolicyState       `json:"policy_state"`
	CreatedAt      time.Time         `json:"created_at"`
	ExpiresAt      time.Time         `json:"expires_at,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func (l Lock) Normalized() Lock {
	out := l
	out.ID = trace.RedactString(strings.TrimSpace(out.ID), maxIDLen)
	out.Scope = NormalizeScope(string(out.Scope))
	out.Dimension = NormalizeDimension(string(out.Dimension))
	out.Subject = trace.RedactString(strings.TrimSpace(out.Subject), maxSubjectLen)
	out.StateHash = trace.RedactString(strings.TrimSpace(out.StateHash), maxStateHashLen)
	out.Status = normalizeStatus(string(out.Status))
	out.Source = normalizeSource(string(out.Source))
	out.PolicyState = normalizePolicyState(string(out.PolicyState))
	out.EvidenceRefs = normalizeRefs(out.EvidenceRefs)
	out.ValidationRefs = normalizeRefs(out.ValidationRefs)
	out.Metadata = normalizeMetadata(out.Metadata)
	if out.CreatedAt.IsZero() {
		out.CreatedAt = time.Now().UTC()
	}
	if !out.ExpiresAt.IsZero() && out.ExpiresAt.Before(out.CreatedAt) {
		out.ExpiresAt = out.CreatedAt
	}
	if out.Status == StatusInvalid {
		out.Status = StatusActive
	}
	if out.ID == "" && out.Subject != "" && out.StateHash != "" {
		out.ID = "lock_" + trace.StableHash(string(out.Scope), string(out.Dimension), out.Subject, out.StateHash)
	}
	return out
}

func (l Lock) Validate() error {
	n := l.Normalized()
	if n.ID == "" || n.Subject == "" || n.StateHash == "" {
		return fmt.Errorf("%w: missing id/subject/state_hash", ErrInvalidLock)
	}
	if n.Scope == ScopeUnknown {
		return fmt.Errorf("%w: unknown scope", ErrInvalidLock)
	}
	if n.Dimension == DimensionUnknown {
		return fmt.Errorf("%w: unknown dimension", ErrInvalidLock)
	}
	if n.CreatedAt.IsZero() {
		return fmt.Errorf("%w: missing created_at", ErrInvalidLock)
	}
	return nil
}

func normalizeRefs(in []trace.Ref) []trace.Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]trace.Ref, 0, len(in))
	for i := range in {
		ref := trace.NewRef(in[i].Kind, in[i].Ref, in[i].Hash, in[i].SizeBytes)
		if strings.TrimSpace(ref.Ref) == "" {
			continue
		}
		if ref.SizeBytes < 0 {
			ref.SizeBytes = 0
		}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			if out[i].Ref == out[j].Ref {
				return out[i].Hash < out[j].Hash
			}
			return out[i].Ref < out[j].Ref
		}
		return out[i].Kind < out[j].Kind
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeMetadata(in map[string]string) map[string]string {
	return trace.BoundMetadata(in)
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRefs(in []trace.Ref) []trace.Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]trace.Ref, len(in))
	copy(out, in)
	return out
}

func cloneLock(in Lock) Lock {
	out := in
	out.EvidenceRefs = cloneRefs(in.EvidenceRefs)
	out.ValidationRefs = cloneRefs(in.ValidationRefs)
	out.Metadata = cloneStringMap(in.Metadata)
	return out
}
