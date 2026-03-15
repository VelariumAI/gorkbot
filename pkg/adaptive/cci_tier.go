// Package cci implements the Codified Context Infrastructure — a three-tier
// persistent project memory system for Gorkbot.
//
// Tier 1 (Hot): Universal conventions + trigger table, injected every session.
// Tier 2 (Specialist): Domain personas loaded on-demand by the trigger table.
// Tier 3 (Cold): On-demand subsystem specification docs queried by retrieval tools.
package adaptive

import "time"

// TierType identifies which memory tier a document belongs to.
type TierType int

const (
	// TierHot is always injected into every session (compact, universal).
	TierHot TierType = iota
	// TierSpecialist is loaded on-demand for a specific domain task.
	TierSpecialist
	// TierCold is retrieved on-demand from the subsystem knowledge base.
	TierCold
)

func (t TierType) String() string {
	switch t {
	case TierHot:
		return "Hot"
	case TierSpecialist:
		return "Specialist"
	case TierCold:
		return "Cold"
	default:
		return "Unknown"
	}
}

// CCIDoc is a single document managed by the CCI system.
type CCIDoc struct {
	Name      string
	Tier      TierType
	Domain    string // specialist domain or subsystem name
	Content   string
	UpdatedAt time.Time
}

// DriftWarning is emitted by the Truth Sentry when source files were modified
// but their corresponding Tier 3 spec was not updated in the same commit window.
type DriftWarning struct {
	SourceFile  string // modified Go/config file
	SpecFile    string // expected Tier 3 spec path (relative to docs dir)
	Subsystem   string // human-readable subsystem name
	LastModSpec time.Time
}

// GapEvent is emitted when a retrieval query returns no Tier 3 document for a subsystem.
type GapEvent struct {
	Subsystem string
	Query     string
}
