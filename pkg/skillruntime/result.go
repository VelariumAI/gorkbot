package skillruntime

import (
	"fmt"
	"sort"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/trace"
)

type Result struct {
	ID             string              `json:"id"`
	Operation      Operation           `json:"operation"`
	CandidateID    string              `json:"candidate_id"`
	Status         Status              `json:"status"`
	Decision       evidence.Decision   `json:"decision"`
	Assessment     evidence.Assessment `json:"assessment"`
	Receipt        evidence.Receipt    `json:"receipt"`
	ValidationRefs []trace.Ref         `json:"validation_refs,omitempty"`
	ArtifactRefs   []trace.Ref         `json:"artifact_refs,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
	Metadata       map[string]string   `json:"metadata,omitempty"`
}

func (r Result) Normalized() Result {
	out := r
	out.ID = boundID(out.ID)
	out.Operation = NormalizeOperation(string(out.Operation))
	out.CandidateID = boundID(out.CandidateID)
	out.Status = NormalizeStatus(string(out.Status))
	out.Decision = evidence.NormalizeDecision(string(out.Decision))
	out.Assessment = evidence.Evaluate(out.Assessment)
	if out.Receipt.Assessment.ID == "" {
		out.Receipt.Assessment = out.Assessment
	}
	out.Receipt = out.Receipt.Normalized()
	out.ValidationRefs = boundRefs(out.ValidationRefs, maxLifecycleRefCount)
	out.ArtifactRefs = boundRefs(out.ArtifactRefs, maxLifecycleRefCount)
	out.Warnings = boundWarnings(out.Warnings)
	out.Metadata = boundMetadata(out.Metadata, maxMetadataEntries)

	if out.ID == "" {
		warningHash := ""
		if len(out.Warnings) > 0 {
			warningHash = trace.StableHash(out.Warnings...)
		}
		out.ID = "result_" + trace.StableHash(
			string(out.Operation),
			out.CandidateID,
			string(out.Status),
			string(out.Decision),
			out.Assessment.ID,
			out.Receipt.ID,
			stableRefsHash(out.ValidationRefs),
			stableRefsHash(out.ArtifactRefs),
			warningHash,
			stableMetadataHash(out.Metadata),
		)
	}
	return out
}

func (r Result) Validate() error {
	n := r.Normalized()
	if n.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidResult)
	}
	if err := n.Operation.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResult, err)
	}
	if n.CandidateID == "" {
		return fmt.Errorf("%w: missing candidate id", ErrInvalidResult)
	}
	if err := n.Status.Validate(); err != nil {
		return err
	}
	if err := n.Assessment.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResult, err)
	}
	if err := n.Receipt.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResult, err)
	}
	if len(n.Warnings) > maxWarningCount {
		return fmt.Errorf("%w: too many warnings", ErrInvalidResult)
	}
	return nil
}

func mergeWarningLists(existing []string, more ...string) []string {
	merged := append([]string(nil), existing...)
	merged = append(merged, more...)
	if len(merged) == 0 {
		return nil
	}
	bounded := boundWarnings(merged)
	if len(bounded) == 0 {
		return nil
	}
	return bounded
}

func mergeRefs(parts ...[]trace.Ref) []trace.Ref {
	all := make([]trace.Ref, 0)
	for i := range parts {
		all = append(all, parts[i]...)
	}
	if len(all) == 0 {
		return nil
	}
	all = boundRefs(all, maxLifecycleRefCount)
	if len(all) == 0 {
		return nil
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Kind == all[j].Kind {
			if all[i].Ref == all[j].Ref {
				return all[i].Hash < all[j].Hash
			}
			return all[i].Ref < all[j].Ref
		}
		return all[i].Kind < all[j].Kind
	})
	return all
}
