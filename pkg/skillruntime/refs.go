package skillruntime

import (
	"github.com/velariumai/gorkbot/pkg/trace"
)

func CandidateRef(candidate Candidate) trace.Ref {
	n := candidate.Normalized()
	return trace.NewRef(
		"skill_candidate",
		"candidate:"+trace.RedactString(n.ID, 128),
		trace.StableHash(n.ID, n.Name, n.Version, string(n.Risk), string(n.OperationClass)),
		int64(len(n.ArtifactRefs)+len(n.EvidenceRefs)),
	)
}

func ResultRef(result Result) trace.Ref {
	n := result.Normalized()
	return trace.NewRef(
		"skill_result",
		"result:"+trace.RedactString(n.ID, 128),
		trace.StableHash(n.ID, string(n.Operation), n.CandidateID, string(n.Status), string(n.Decision)),
		int64(len(n.ValidationRefs)+len(n.ArtifactRefs)),
	)
}
