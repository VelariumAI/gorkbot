package puteradapter

import (
	"fmt"
	"slices"
	"strings"
)

// PuterWorkspaceManifest is a validated immutable workspace policy model.
type PuterWorkspaceManifest struct {
	root          string
	readPrefixes  []string
	writePrefixes []string
	protected     []string
	puterRepo     string
	puterRef      string
	defaultBranch string
	inspectedDocs []string
}

// DefaultWorkspaceManifest returns the baseline scoped workspace model for PR-005.
func DefaultWorkspaceManifest() PuterWorkspaceManifest {
	cfg := DefaultConfig()
	m, _ := NewWorkspaceManifest(cfg, []string{
		"README.md",
		"src/docs/src/FS.md",
		"src/docs/src/FS/write.md",
		"src/docs/src/KV.md",
		"src/docs/src/KV/set.md",
		"src/docs/src/Hosting.md",
		"src/docs/src/Networking/fetch.md",
	})
	return m
}

// NewWorkspaceManifest builds a validated manifest from runtime configuration.
func NewWorkspaceManifest(cfg Config, inspectedDocs []string) (PuterWorkspaceManifest, error) {
	if err := cfg.Validate(); err != nil {
		return PuterWorkspaceManifest{}, err
	}
	root := cfg.Root
	writes := []string{
		root + "/scratch",
		root + "/experiments",
		root + "/apps",
		root + "/artifacts",
		root + "/state",
		root + "/missions",
	}
	protected := []string{
		root + "/logs",
		root + "/receipts",
		root + "/rollback",
		root + "/ledger",
	}
	cleanDocs := make([]string, 0, len(inspectedDocs))
	for _, doc := range inspectedDocs {
		d := strings.TrimSpace(doc)
		if d == "" {
			continue
		}
		cleanDocs = append(cleanDocs, d)
	}
	if len(cleanDocs) == 0 {
		return PuterWorkspaceManifest{}, fmt.Errorf("manifest requires at least one inspected doc path")
	}
	return PuterWorkspaceManifest{
		root:          root,
		readPrefixes:  []string{root},
		writePrefixes: slices.Clone(writes),
		protected:     slices.Clone(protected),
		puterRepo:     strings.TrimSpace(cfg.PuterRepo),
		puterRef:      strings.TrimSpace(cfg.PuterRef),
		defaultBranch: strings.TrimSpace(cfg.PuterDefaultBranch),
		inspectedDocs: slices.Clone(cleanDocs),
	}, nil
}

func (m PuterWorkspaceManifest) Root() string {
	return m.root
}

func (m PuterWorkspaceManifest) PuterRepo() string {
	return m.puterRepo
}

func (m PuterWorkspaceManifest) PuterRef() string {
	return m.puterRef
}

func (m PuterWorkspaceManifest) PuterDefaultBranch() string {
	return m.defaultBranch
}

func (m PuterWorkspaceManifest) InspectedDocs() []string {
	return slices.Clone(m.inspectedDocs)
}

func (m PuterWorkspaceManifest) readPrefixesCopy() []string {
	return slices.Clone(m.readPrefixes)
}

func (m PuterWorkspaceManifest) writePrefixesCopy() []string {
	return slices.Clone(m.writePrefixes)
}

func (m PuterWorkspaceManifest) protectedCopy() []string {
	return slices.Clone(m.protected)
}
