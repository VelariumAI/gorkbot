package skillruntime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/profile"
	"github.com/velariumai/gorkbot/pkg/trace"
)

const (
	maxIDLen             = 128
	maxNameLen           = 128
	maxVersionLen        = 64
	maxSourceLen         = 160
	maxSummaryLen        = 256
	maxHashLen           = 128
	maxWarningCount      = 24
	maxWarningLen        = 160
	maxMetadataEntries   = 16
	maxLifecycleRefCount = 64
)

type Operation string

const (
	OperationRetrieve Operation = "retrieve"
	OperationPropose  Operation = "propose"
	OperationValidate Operation = "validate"
	OperationStage    Operation = "stage"
	OperationPromote  Operation = "promote"
	OperationDisable  Operation = "disable"
	OperationUnknown  Operation = "unknown"
)

func NormalizeOperation(raw string) Operation {
	op := Operation(strings.ToLower(strings.TrimSpace(raw)))
	switch op {
	case OperationRetrieve, OperationPropose, OperationValidate,
		OperationStage, OperationPromote, OperationDisable:
		return op
	default:
		return OperationUnknown
	}
}

func (o Operation) Validate() error {
	if NormalizeOperation(string(o)) == OperationUnknown {
		return fmt.Errorf("%w: %q", ErrInvalidOperation, o)
	}
	return nil
}

type Status string

const (
	StatusUnknown          Status = "unknown"
	StatusAllowed          Status = "allowed"
	StatusStaged           Status = "staged"
	StatusApprovalRequired Status = "approval_required"
	StatusConfigRequired   Status = "config_required"
	StatusDenied           Status = "denied"
	StatusDisabled         Status = "disabled"
	StatusNotFound         Status = "not_found"
	StatusInvalid          Status = "invalid"
)

func NormalizeStatus(raw string) Status {
	s := Status(strings.ToLower(strings.TrimSpace(raw)))
	switch s {
	case StatusAllowed, StatusStaged, StatusApprovalRequired,
		StatusConfigRequired, StatusDenied, StatusDisabled,
		StatusNotFound, StatusInvalid:
		return s
	case "":
		return StatusUnknown
	default:
		return StatusInvalid
	}
}

func (s Status) Validate() error {
	n := NormalizeStatus(string(s))
	if n == StatusInvalid || n == StatusUnknown {
		return fmt.Errorf("%w: invalid status %q", ErrInvalidResult, s)
	}
	return nil
}

type Candidate struct {
	ID             string                      `json:"id"`
	Name           string                      `json:"name"`
	Version        string                      `json:"version"`
	Source         string                      `json:"source"`
	Summary        string                      `json:"summary"`
	Risk           evidence.Risk               `json:"risk"`
	OperationClass evidence.SensitiveOperation `json:"operation_class"`
	Profile        profile.Profile             `json:"profile"`
	ArtifactRefs   []trace.Ref                 `json:"artifact_refs,omitempty"`
	EvidenceRefs   []trace.Ref                 `json:"evidence_refs,omitempty"`
	Metadata       map[string]string           `json:"metadata,omitempty"`
	Disabled       bool                        `json:"disabled,omitempty"`
}

func (c Candidate) Normalized() Candidate {
	out := c
	out.ID = boundID(out.ID)
	out.Name = boundStringSingleLine(out.Name, maxNameLen)
	out.Version = boundStringSingleLine(out.Version, maxVersionLen)
	out.Source = boundStringSingleLine(out.Source, maxSourceLen)
	out.Summary = boundStringSingleLine(out.Summary, maxSummaryLen)
	out.Risk = evidence.NormalizeRisk(string(out.Risk))
	out.OperationClass = evidence.NormalizeSensitiveOperation(string(out.OperationClass))
	if out.Risk == evidence.RiskUnknown {
		if out.OperationClass != evidence.SensitiveUnknown {
			out.Risk = evidence.RiskSensitive
		} else {
			out.Risk = evidence.RiskMedium
		}
	}
	out.Profile = profile.NormalizeProfile(string(out.Profile))
	if out.Profile == profile.ProfileUnknown {
		out.Profile = profile.ProfileBeginner
	}
	out.ArtifactRefs = boundRefs(out.ArtifactRefs, maxLifecycleRefCount)
	out.EvidenceRefs = boundRefs(out.EvidenceRefs, maxLifecycleRefCount)
	out.Metadata = boundMetadata(out.Metadata, maxMetadataEntries)

	if out.ID == "" {
		out.ID = "candidate_" + trace.StableHash(
			out.Name,
			out.Version,
			out.Source,
			out.Summary,
			string(out.Risk),
			string(out.OperationClass),
			string(out.Profile),
			stableRefsHash(out.ArtifactRefs),
			stableRefsHash(out.EvidenceRefs),
			stableMetadataHash(out.Metadata),
		)
	}
	return out
}

func (c Candidate) Validate() error {
	n := c.Normalized()
	if n.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidCandidate)
	}
	if n.Name == "" && n.Source == "" {
		return fmt.Errorf("%w: missing candidate identity", ErrInvalidCandidate)
	}
	return nil
}

func boundID(raw string) string {
	return trace.RedactString(strings.TrimSpace(raw), maxIDLen)
}

func boundHash(raw string) string {
	return trace.RedactString(strings.TrimSpace(raw), maxHashLen)
}

func boundStringSingleLine(raw string, maxLen int) string {
	clean := strings.TrimSpace(raw)
	if idx := strings.IndexByte(clean, '\n'); idx >= 0 {
		clean = clean[:idx]
	}
	if idx := strings.IndexByte(clean, '\r'); idx >= 0 {
		clean = clean[:idx]
	}
	return trace.RedactString(clean, maxLen)
}

func boundWarnings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	if len(in) > maxWarningCount {
		in = in[:maxWarningCount]
	}
	out := make([]string, 0, len(in))
	for i := range in {
		v := boundStringSingleLine(in[i], maxWarningLen)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func boundRefs(in []trace.Ref, limit int) []trace.Ref {
	if len(in) == 0 || limit <= 0 {
		return nil
	}
	if len(in) > limit {
		in = in[:limit]
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
	if len(out) == 0 {
		return nil
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
	return out
}

func boundMetadata(in map[string]string, limit int) map[string]string {
	if len(in) == 0 || limit <= 0 {
		return nil
	}
	base := trace.BoundMetadata(in)
	if len(base) == 0 {
		return nil
	}
	out := make(map[string]string, len(base))
	for k, v := range base {
		if shouldDropMetadataKey(k) {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	bounded := make(map[string]string, len(keys))
	for _, k := range keys {
		bounded[k] = out[k]
	}
	return bounded
}

func shouldDropMetadataKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	k = strings.ReplaceAll(k, "-", "_")
	dropNeedles := []string{
		"prompt",
		"model_output",
		"command_output",
		"file_body",
		"diff",
		"env_dump",
		"raw_output",
	}
	for _, needle := range dropNeedles {
		if k == needle || strings.Contains(k, needle) {
			return true
		}
	}
	return false
}

func stableMetadataHash(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		parts = append(parts, k, meta[k])
	}
	return trace.StableHash(parts...)
}

func stableRefsHash(refs []trace.Ref) string {
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(refs)*4)
	for i := range refs {
		parts = append(parts, refs[i].Kind, refs[i].Ref, refs[i].Hash, fmt.Sprintf("%d", refs[i].SizeBytes))
	}
	return trace.StableHash(parts...)
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
