# Gorkbot Versioning Strategy

This document defines the dual-versioning system for Gorkbot: **internal development versioning** and **public release versioning**.

## Overview

**Gorkbot uses a strict dual-versioning scheme:**

1. **Internal Version** (e.g., `5.3.0`): Development-focused, tracks architectural progress
2. **Public Version** (e.g., `1.2.0-beta`): User-facing, follows semantic versioning
3. **Subsystem Versions**: Independent tracks (SENSE `1.9.0`, SRE `1.0.0`, XSKILL `1.0.0`)

---

## Internal Versioning (MAJOR.MINOR.PATCH)

**Format**: `X.Y.Z` (e.g., `5.3.0`)

Used in `internal/platform/env.go` as `const Version`.

### Version Components

| Component | Role | Example |
|-----------|------|---------|
| **X (Major)** | Architectural paradigm shifts | v2→v3: TUI overhaul; v4→v5: Orchestrator + companions |
| **Y (Minor)** | Feature additions, subsystem improvements | v5.2: LiveToolsPanel + async; v5.3: Theme system |
| **Z (Patch)** | Bug fixes, optimizations | v5.3.1: Notification cooldown fix |

### Increment Policy

- **Major**: Only on foundational rewrites (rare, every 6+ months)
- **Minor**: For feature releases (monthly or as features complete)
- **Patch**: For bug fixes and optimizations (weekly or as needed)

**Example Progression**:
```
4.7.0 → 4.8.0 (New feature)
      → 4.8.1 (Bug fix)
      → 5.0.0 (Architectural shift: Companion systems)
      → 5.1.0 (Feature: Async tool execution)
      → 5.3.0 (Feature: Theme system + UX polish)
```

---

## Public Versioning (MAJOR.MINOR[.BUILD[.REVISION]]-(STATUS))

**Format**: `X.Y[.Z[.W]]-STATUS` (e.g., `1.2.0-beta`)

Displayed in user-facing contexts (TUI status, `--version`, API headers).

### Version Components

| Component | Role | Range | Notes |
|-----------|------|-------|-------|
| **X (Major)** | Public generation | 1, 2, 3... | Incremented for breaking changes, resets at each new public major |
| **Y (Minor)** | Feature maturity | 0-999 | Incremented with significant user-facing improvements |
| **Z (Build)** | Production build | 0-999 | Optional; incremented for patch releases |
| **W (Revision)** | Hotfix revision | 0-999 | Optional; only included if > 0 |
| **STATUS** | Release maturity | alpha, beta, rc | **Never omitted** |

### Increment Policy

- **Major**: Only on complete architectural overhauls visible to users (rare)
- **Minor**: For feature releases (every 4-8 weeks)
- **Build**: For patch releases with bugfixes (2-3 per minor)
- **Revision**: For critical hotfixes (ad-hoc)
- **Status**:
  - **alpha**: New features, unstable API, breaking changes possible
  - **beta**: Feature-complete, most bugs resolved, production-use possible
  - **rc**: Release candidate, final QA stage, ready for release
  - (no suffix): Stable release (production-ready, long-term support)

### Example Progression

```
1.0.0-alpha → 1.0.0-beta → 1.0.0-rc → 1.0.0 (stable)
                                       → 1.0.1-rc (patch RC)
                                       → 1.0.1 (patch stable)
                                       → 1.1.0-beta (next minor)
```

---

## Subsystem Versions (Independent)

Subsystems maintain their own version tracks:

| Subsystem | Location | Current | Notes |
|-----------|----------|---------|-------|
| **SENSE** | `pkg/sense/discovery.go` | 1.9.0 | Input sanitization, tracing, compression |
| **SRE** | `pkg/sre/types.go` | 1.0.0 | Streaming response execution |
| **XSKILL** | `internal/xskill/models.go` | 1.0.0 | Continual learning framework |

**Future**: Each subsystem will eventually separate into independent repositories with their own versioning, release cycles, and governance.

---

## Current Version Mapping

### As of April 5, 2026

| Version Type | Current | Notes |
|--------------|---------|-------|
| **Internal** | 6.2.0 | Latest development version |
| **Public** | 1.6.0-rc | Release candidate with promotion-grade hardening |
| **SENSE** | 1.9.0 | On independent track |
| **SRE** | 1.0.0 | On independent track |
| **XSKILL** | 1.0.0 | On independent track |

### Rationale for 1.6.0-rc

- **Internal 6.2.0 → Public 1.6.0**:
  - Major=1: First public generation ensures clear versioning boundary between dev and public
  - Minor=6: Reflects substantial cross-system maturity and production hardening
  - Build=0: Baseline build for this release candidate
  - Status=rc: Release candidate with full promotion/readiness focus before stable cut

---

## Version File Locations

### Code
- `internal/platform/env.go`: Internal version constant
- `pkg/version/version.go`: Public version helpers
- `pkg/sense/discovery.go`: SENSE version
- `pkg/sre/types.go`: SRE version
- `internal/xskill/models.go`: XSKILL version

### Documentation
- `VERSION`: Human-readable version file (repo root)
- `VERSIONING.md`: This file
- `CHANGELOG.md` (in memory): Historical version notes

### Metadata
- Git tags: Follow `v{public}` convention (e.g., `v1.6.0-rc`)
- Release notes: Tagged in commit messages or GitHub releases

---

## When to Bump Versions

### Internal Version

**Bump Minor (5.3 → 5.4)**:
- Major new features shipped
- Significant architectural improvements
- Monthly or quarterly milestone reached

**Bump Major (5.x → 6.0)**:
- Complete subsystem rewrites
- Breaking API changes
- Rare (every 6+ months)

**Bump Patch (5.3.0 → 5.3.1)**:
- Bug fixes, optimizations
- Security patches
- Every week or two

### Public Version

**Bump Minor (1.2 → 1.3)**:
- Significant new user-facing features
- Major improvements in stability/performance
- Every 4-8 weeks

**Bump Build (1.2.0 → 1.2.1)**:
- Bugfix releases
- Patch releases
- As needed, 2-3 per minor

**Bump Revision (1.2.1.0 → 1.2.1.1)**:
- Critical security/stability hotfixes
- Applied after release
- Rare, ad-hoc

**Change Status**:
- alpha → beta: Feature freeze + testing
- beta → rc: Final QA + documentation
- rc → stable: Release to public

---

## Communicating Versions

### To Users
- **TUI Status Line**: Shows public version (e.g., "Gorkbot 1.2.0-beta")
- **`/version` Command**: Shows public version + subsystem versions
- **Release Notes**: Reference public version in changelogs
- **Documentation**: Use public version in setup guides

### To Developers
- **Git History**: Internal version in commit messages when significant
- **Code Comments**: Reference internal version for architectural context
- **Internal Documentation**: Track internal version in memory files

### In APIs/Integrations
- **HTTP Headers**: Use public version (`X-Gorkbot-Version: 1.2.0-beta`)
- **MCP Servers**: Advertise public version
- **Plugin Compatibility**: Reference public major version

---

## Version Stability Guarantees

### Alpha (1.x.y-alpha)
- ❌ No API stability guarantees
- ❌ Features may be added/removed/changed
- ⚠️ Use only for testing and feedback

### Beta (1.x.y-beta)
- ⚠️ Core features stable, APIs may change
- ✅ Production-use viable for advanced users
- ✅ Backwards compatibility within minor version
- ⚠️ Edge cases may exist

### RC (1.x.y-rc)
- ✅ Production-ready, final QA in progress
- ✅ No breaking changes expected
- ⚠️ Last-minute critical fixes possible

### Stable (1.x.y, no suffix)
- ✅ Fully production-ready
- ✅ Long-term support guaranteed within major version
- ✅ Breaking changes only at major version bump

---

## Implementation Notes

- **Strict delineation**: Each version component has a single, well-defined meaning
- **Professional standard**: Follows semantic versioning + release maturity indicators
- **Sustainable**: Easy to increment, track, and communicate
- **Future-proof**: Subsystems can independently graduate to their own release cycles

---

## Related Documentation

- `VERSION`: Quick reference file
- `CHANGELOG.md`: Historical release notes (in memory)
- `CLAUDE.md`: Developer instructions
- `go.mod`: Dependency tracking (currently at Go 1.25.0)
