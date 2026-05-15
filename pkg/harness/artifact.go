package harness

import (
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/trace"
)

const (
	maxArtifactIDLen       = 128
	maxArtifactNameLen     = 128
	maxArtifactContentSize = 64 * 1024
	maxArtifactRefs        = 64
)

// ArtifactKind identifies the source/shape of the artifact under validation.
type ArtifactKind string

const (
	ArtifactKindUnknown          ArtifactKind = "unknown"
	ArtifactKindText             ArtifactKind = "text"
	ArtifactKindCommand          ArtifactKind = "command"
	ArtifactKindFilePath         ArtifactKind = "file_path"
	ArtifactKindFilePatch        ArtifactKind = "file_patch"
	ArtifactKindToolCall         ArtifactKind = "tool_call"
	ArtifactKindResearchClaim    ArtifactKind = "research_claim"
	ArtifactKindProviderResponse ArtifactKind = "provider_response"
	ArtifactKindSelfmodManifest  ArtifactKind = "selfmod_manifest"
	ArtifactKindReplayResult     ArtifactKind = "replay_result"
)

var validArtifactKinds = map[ArtifactKind]struct{}{
	ArtifactKindUnknown:          {},
	ArtifactKindText:             {},
	ArtifactKindCommand:          {},
	ArtifactKindFilePath:         {},
	ArtifactKindFilePatch:        {},
	ArtifactKindToolCall:         {},
	ArtifactKindResearchClaim:    {},
	ArtifactKindProviderResponse: {},
	ArtifactKindSelfmodManifest:  {},
	ArtifactKindReplayResult:     {},
}

// Artifact is the bounded input object for harness validation.
type Artifact struct {
	ID          string            `json:"id"`
	Kind        ArtifactKind      `json:"kind"`
	Name        string            `json:"name,omitempty"`
	Content     string            `json:"content,omitempty"`
	ContentHash string            `json:"content_hash,omitempty"`
	Refs        []trace.Ref       `json:"refs,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (a Artifact) Normalized() Artifact {
	out := a
	out.ID = truncateString(strings.TrimSpace(out.ID), maxArtifactIDLen)
	out.Name = truncateString(strings.TrimSpace(out.Name), maxArtifactNameLen)
	out.ContentHash = truncateString(strings.TrimSpace(out.ContentHash), 128)

	kind := ArtifactKind(strings.ToLower(strings.TrimSpace(string(out.Kind))))
	if _, ok := validArtifactKinds[kind]; ok {
		out.Kind = kind
	} else {
		out.Kind = ArtifactKindUnknown
	}

	if len(out.Refs) > maxArtifactRefs {
		out.Refs = out.Refs[:maxArtifactRefs]
	}
	for i := range out.Refs {
		out.Refs[i] = trace.NewRef(out.Refs[i].Kind, out.Refs[i].Ref, out.Refs[i].Hash, out.Refs[i].SizeBytes)
	}

	out.Metadata = trace.BoundMetadata(out.Metadata)
	if out.ContentHash == "" && strings.TrimSpace(out.Content) != "" {
		out.ContentHash = trace.StableHash(out.Content)
	}
	return out
}

func (a Artifact) Validate() error {
	norm := a.Normalized()
	if norm.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidArtifact)
	}
	if _, ok := validArtifactKinds[norm.Kind]; !ok {
		return fmt.Errorf("%w: invalid kind %q", ErrInvalidArtifact, norm.Kind)
	}
	if len(norm.Content) > maxArtifactContentSize {
		return fmt.Errorf("%w: %d > %d", ErrArtifactTooLarge, len(norm.Content), maxArtifactContentSize)
	}
	return nil
}

// PrimaryScope resolves the default registry scope for this artifact.
func (a Artifact) PrimaryScope() string {
	norm := a.Normalized()
	if norm.Name != "" {
		return strings.ToLower(norm.Name)
	}
	return string(norm.Kind)
}
