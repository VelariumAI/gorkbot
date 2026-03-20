# Gorkbot Versioning System - Complete Guide

Gorkbot uses a **dual versioning system** to track both public releases and internal development progress independently, plus independent tracks for major subsystems.

---

## 📋 Table of Contents

1. [Current Versions](#current-versions)
2. [Versioning Strategy](#versioning-strategy)
3. [Public Versioning](#public-versioning)
4. [Internal Versioning](#internal-versioning)
5. [Subsystem Versioning](#subsystem-versioning)
6. [Release Process](#release-process)
7. [Version Stability](#version-stability)
8. [Migration Guides](#migration-guides)

---

## 📊 Current Versions

### As of March 20, 2026

| Version Type | Current | Status | Audience |
|--------------|---------|--------|----------|
| **Public** | 1.2.0-beta | Feature-complete, Beta | End users, early adopters |
| **Internal** | 5.3.0 | Production-ready | Development, architecture tracking |
| **SENSE** | 1.9.0 | Stable | Input/output awareness subsystem |
| **SRE** | 1.0.0 | Stable | Streaming response execution |
| **XSKILL** | 1.0.0 | Stable | Continual learning framework |
| **Go Version** | 1.25.0 | Current | Language runtime |

---

## 🎯 Versioning Strategy

### Philosophy

Gorkbot uses dual versioning to serve different audiences:

1. **Public Version (1.2.0-beta)** - For users
   - Semantic versioning with maturity stage
   - Communicates stability and feature completeness
   - Follows industry standards (semver.org)
   - Changes infrequently (every 4-8 weeks)

2. **Internal Version (5.3.0)** - For developers
   - Tracks architectural evolution
   - Independent from public releases
   - Incremented with features and refactoring
   - Changes more frequently (weekly)

3. **Subsystem Versions** - For component tracking
   - Independent evolution of major subsystems
   - May graduate to separate repositories
   - Allows decoupled release cycles

---

## 📦 Public Versioning (User-Facing)

### Format: MAJOR.MINOR[.BUILD[.REVISION]]-STATUS

```
1    .2    .0    .0        -beta
│    │     │     │         │
│    │     │     │         └─ Status (alpha, beta, rc, or none)
│    │     │     └─────────── Revision (hotfix, optional)
│    │     └──────────────── Build (patch release, optional)
│    └──────────────────── Minor (features, significant improvements)
└───────────────────────── Major (breaking changes, complete rewrites)
```

### Component Definitions

#### MAJOR Version (1, 2, 3...)
**When**: Complete architectural rewrite or breaking changes affecting users
**Examples**:
- v1 → v2: Complete TUI redesign
- v2 → v3: New execution model or plugin system

**Increment Policy**: Rare (every 6-12+ months)
**Backwards Compatibility**: ❌ No guarantees

#### MINOR Version (0, 1, 2...)
**When**: Significant new features, major improvements
**Examples**:
- 1.0 → 1.1: Add 20+ new tools
- 1.1 → 1.2: Add multi-provider support (✓ what we did)

**Increment Policy**: Every 4-8 weeks
**Backwards Compatibility**: ✅ Within major version

#### BUILD Version (0, 1, 2...) - Optional
**When**: Bug fixes, patches, minor improvements
**Examples**:
- 1.2.0 → 1.2.1: Fix connection retry logic
- 1.2.1 → 1.2.2: Performance improvement

**Increment Policy**: 2-3 per minor release
**Backwards Compatibility**: ✅ Full compatibility

#### REVISION Version (0, 1, 2...) - Optional
**When**: Critical hotfixes after release
**Examples**:
- 1.2.0 → 1.2.0.1: Security vulnerability
- 1.2.0.1 → 1.2.0.2: Critical stability fix

**Increment Policy**: As needed (rare)
**Backwards Compatibility**: ✅ Full compatibility

#### STATUS (Never Omitted)
**alpha**: Experimental, unstable, breaking changes possible
**beta**: Feature-complete, production-viable, some edge cases
**rc**: Release candidate, production-ready pending final QA
**(none)**: Stable release, long-term support

### Release Progression Example

```
1.0.0-alpha    (Initial experimental release)
  ↓
1.0.0-beta     (Feature complete, testing)
  ↓
1.0.0-rc1      (Ready for release, final QA)
  ↓
1.0.0-rc2      (Critical fix found, re-QA)
  ↓
1.0.0          (Stable release)
  ↓
1.0.1          (Patch with bugfixes)
  ↓
1.1.0-beta     (New features, back to beta)
  ↓
1.1.0          (Stable release)
  ↓
1.1.1          (Patch)
  ↓
2.0.0-alpha    (Major architectural change, back to alpha)
```

### Stable Release Criteria

**For RC → Stable transition**:
- ✅ All critical bugs resolved
- ✅ No known security vulnerabilities
- ✅ Documentation complete and accurate
- ✅ Successful real-world testing (2+ weeks minimum)
- ✅ Performance benchmarks acceptable
- ✅ All supported platforms tested (Linux, macOS, Windows, Android)

### Version Increment Decision Tree

```
Is it a breaking change for users?
├─ YES → Increment MAJOR
└─ NO
    Is it a new user-facing feature?
    ├─ YES → Increment MINOR
    └─ NO
        Is it a bug fix or optimization?
        ├─ YES → Increment BUILD
        └─ NO
            Is it a critical hotfix in released version?
            ├─ YES → Increment REVISION
            └─ NO → No version change needed
```

---

## 🏗️ Internal Versioning (Development)

### Format: MAJOR.MINOR.PATCH

```
5.3.0
│ │ │
│ │ └─ Patch (bug fixes, optimizations)
│ └─── Minor (features, architectural improvements)
└───── Major (paradigm shifts, complete rewrites)
```

### Component Definitions

#### MAJOR Version (4, 5, 6...)
**When**: Fundamental architectural paradigm shift
**Examples**:
- v4 → v5: Introduction of companion systems (SENSE, ARC, CCI, MEL)
- v5 → v6: (Hypothetical) Complete orchestrator redesign

**Increment Policy**: Very rare (every 6-12+ months)

**v5.x Timeline** (Current generation):
- v5.0: Initial orchestrator + companion systems
- v5.1: Async tool execution, parallel dispatch
- v5.2: Extended thinking integration
- v5.3: SENSE stability, documentation, professional release

#### MINOR Version (0, 1, 2...)
**When**: Significant features, subsystem improvements
**Examples**:
- v5.2 → v5.3: Comprehensive SENSE layer (input sanitization, compression, memory)
- v5.1 → v5.2: Extended thinking support

**Increment Policy**: Monthly or when features complete
**Typical cadence**: 1-2 per 6 months

#### PATCH Version (0, 1, 2...)
**When**: Bug fixes, performance optimizations, security patches
**Examples**:
- v5.3.0 → v5.3.1: Fix streaming edge case
- v5.3.1 → v5.3.2: Optimize token counting

**Increment Policy**: Weekly or as needed
**Typical cadence**: 1-3 per release

### Internal Version Progression

```
v5.3.0 (Public 1.2.0-beta release)
  ↓
v5.3.1 (Bug fixes)
v5.3.2 (More fixes)
  ↓
v5.4.0 (New features for 1.3.0-beta)
v5.4.1 (Patch)
  ↓
v6.0.0 (Major architectural shift)
```

---

## 🧩 Subsystem Versioning

### Overview

Major subsystems maintain independent version tracks, allowing:
- Decoupled release cycles
- Clear maturity signaling
- Future separation into independent repos
- Specialized versioning needs

### Subsystem Versions

| Subsystem | Location | Version | Type | Status |
|-----------|----------|---------|------|--------|
| **SENSE** | pkg/sense/ | 1.9.0 | Production | Stable |
| **SRE** | pkg/sre/ | 1.0.0 | Production | Stable |
| **XSKILL** | internal/xskill/ | 1.0.0 | Production | Stable |
| **ARC** | pkg/adaptive/arc/ | Embedded in 5.3.0 | Component | Stable |
| **CCI** | pkg/adaptive/cci/ | Embedded in 5.3.0 | Component | Stable |
| **MEL** | pkg/adaptive/mel/ | Embedded in 5.3.0 | Component | Stable |
| **TUI** | internal/tui/ | 3.5.1 | Component | Stable |
| **AI Providers** | pkg/ai/ | 2.7.0 | Component | Stable |

### SENSE Layer (v1.9.0)

**What**: Input/output awareness, safety, compression, quality criticism
**Features**:
- Input sanitization (19 injection patterns)
- Token streaming and tracing (JSONL)
- Quality criticism (4-dimensional stabilizer)
- Context compression (4-stage pipeline)
- Memory management (3-tier STM/LTM)
- Episodic memory (engrams)
- LIE reward model

**Status**: Stable, production-ready
**Release Schedule**: Independent track
**Future**: May become separate "gorkbot-sense" package

### SRE Layer (v1.0.0)

**What**: Streaming Response Execution engine
**Features**:
- Token-by-token streaming
- Efficient buffer management
- Response reconstruction
- Error recovery

**Status**: Stable, version 1.0.0
**Release Schedule**: Independent track
**Future**: May become separate "gorkbot-sre" package

### XSKILL Framework (v1.0.0)

**What**: Continual learning and skill evolution
**Features**:
- Skill definition and storage
- Execution trace analysis
- Skill evolution from patterns
- MCP server integration

**Status**: Stable, version 1.0.0
**Release Schedule**: Independent track
**Future**: May become separate "gorkbot-xskill" package

### Integrated Components

Components embedded in main version (5.3.0):

- **ARC** (Adaptive Response Classification)
  - Query classification, budgeting, consistency checking

- **CCI** (Codified Context Infrastructure)
  - Hot/cold memory, drift detection, specialist management

- **MEL** (Meta-Experience Learning)
  - Heuristic generation, vector storage, bifurcation analysis

- **TUI** (Terminal UI)
  - Full Elm MVC, 40+ files, Bubble Tea stack

- **AI Providers**
  - 5 providers, native function calling, extended thinking

---

## 🚀 Release Process

### Pre-Release Checklist

**Code Quality**:
- [ ] All tests pass: `go test ./...`
- [ ] No race conditions: `go test -race ./...`
- [ ] Code formatted: `go fmt ./...`
- [ ] No lint issues: `golangci-lint run ./...`

**Documentation**:
- [ ] README.md updated with features
- [ ] GETTING_STARTED.md reviewed
- [ ] CLAUDE.md updated with new APIs
- [ ] Example code tested
- [ ] CHANGELOG.md written
- [ ] Subsystem docs updated

**Testing**:
- [ ] Manual TUI testing on all platforms
- [ ] Tool execution verification
- [ ] Provider integration tests
- [ ] Extended thinking with budget
- [ ] Session checkpoint/rewind
- [ ] Mobile (Android) testing

**Security**:
- [ ] API keys not logged
- [ ] Permissions properly enforced
- [ ] Input sanitization verified
- [ ] Audit logging functional
- [ ] Encryption working

### Release Steps

1. **Prepare Release Branch**
   ```bash
   git checkout -b release/v1.2.0-beta
   ```

2. **Update Version Files**
   ```bash
   # internal/platform/env.go
   const Version = "5.3.0"
   const PublicVersion = "1.2.0-beta"

   # Update subsystem versions if changed
   ```

3. **Create Version Tag**
   ```bash
   git tag -a v1.2.0-beta -m "Public release v1.2.0-beta"
   git tag -a internal/v5.3.0 -m "Internal v5.3.0"
   ```

4. **Build Release Artifacts**
   ```bash
   make clean
   make build
   make build-linux
   make build-windows
   make build-android
   ```

5. **Publish Release**
   ```bash
   git push origin release/v1.2.0-beta
   git push origin v1.2.0-beta
   # Create GitHub release with artifacts
   ```

6. **Update Release Notes**
   - Link to CHANGELOG.md
   - Include upgrade instructions
   - Note breaking changes (if any)

### Version File Locations

**Source Code**:
```
internal/platform/env.go       # Internal version constant
pkg/sense/discovery.go         # SENSE version
pkg/sre/types.go              # SRE version
internal/xskill/models.go     # XSKILL version
internal/tui/view.go          # TUI version (in view)
pkg/ai/interface.go           # AI provider version info
```

**Documentation**:
```
README.md                      # Project overview
GETTING_STARTED.md             # User guide
CLAUDE.md                      # Architecture guide
VERSIONING.md                  # This file
VERSION                        # Quick reference file
```

**Build System**:
```
go.mod                         # Go version requirement
Makefile                       # Build version tagging
```

---

## 📈 Version Stability Guarantees

### Alpha (1.x.y-alpha)

**Stability**: ❌ Unstable
**Use Case**: Testing, feedback gathering, early adopters only

**Guarantees**:
- ❌ No API stability guarantees
- ❌ No backwards compatibility
- ❌ May have breaking changes
- ❌ Not production-ready
- ✅ All features documented
- ✅ Basic testing done

**Support**: Minimal (issues, discussion only)

### Beta (1.x.y-beta) - CURRENT

**Stability**: ⚠️ Generally stable with caveats
**Use Case**: Production use by experienced users, testing

**Guarantees**:
- ✅ Core features stable
- ✅ Backwards compatible within minor version
- ✅ No breaking changes in patch releases
- ⚠️ Edge cases may exist
- ⚠️ APIs may change in next minor release
- ✅ Production-viable for prepared users
- ✅ Comprehensive documentation
- ✅ Good test coverage

**Support**: Full support for reported issues

### RC (1.x.y-rc)

**Stability**: ✅ Production-ready
**Use Case**: Final testing before stable release

**Guarantees**:
- ✅ Feature-complete
- ✅ No breaking changes expected
- ✅ Ready for production
- ⚠️ Last-minute critical fixes possible
- ✅ Full backwards compatibility
- ✅ All platforms tested
- ✅ Full documentation

**Support**: Critical fixes only

### Stable (1.x.y, no suffix)

**Stability**: ✅ Full production-ready
**Use Case**: Production deployments

**Guarantees**:
- ✅ Feature-complete and fully tested
- ✅ Long-term support within major version
- ✅ Full backwards compatibility
- ✅ Security updates provided
- ✅ Bug fixes provided
- ❌ No breaking changes (until major version bump)

**Support**: Full support for all issues

---

## 🔄 Migration Guides

### Migrating from v1.1.x to v1.2.0

**What's New**:
- 5 AI providers (was 2)
- 75+ tools (was 30+)
- Extended thinking support
- Multi-provider routing

**Breaking Changes**: None

**Steps**:
1. Backup current `.env` and config
2. Install new version: `make build`
3. Run setup: `./gorkbot.sh setup`
4. Existing conversations work immediately
5. New providers available automatically

**Backwards Compatibility**: ✅ Full

### Migrating from v1.0.x to v1.1.x

**Breaking Changes**:
- Multi-provider support (requires additional API keys)
- Database schema may migrate automatically
- New tool permission system

**Steps** (Hypothetical):
1. Backup conversations
2. Install v1.1.0
3. Run migration: `./gorkbot.sh migrate-db`
4. Reconfigure providers: `./gorkbot.sh setup`
5. Update tool permissions as prompted

### Future: Migrating to v2.0.0

**Expected Breaking Changes**:
- Complete architecture redesign
- New execution model
- Tool system changes
- API changes

**Preparation**:
- Export conversations before upgrade
- Document custom tools
- Note all configuration
- Plan for downtime

---

## 📊 Version Comparisons

| Feature | v1.0.0-alpha | v1.1.0-beta | v1.2.0-beta |
|---------|-------------|------------|------------|
| AI Providers | 1 (Grok only) | 2 (Grok + Gemini) | 5 (all major) |
| Tools | ~30 | ~50 | 75+ |
| Extended Thinking | ❌ No | ❌ No | ✅ Yes |
| Session Relay | ❌ No | ❌ No | ✅ Yes |
| MCP Integration | ❌ No | ❌ No | ✅ Yes |
| SENSE Layer | ❌ No | Partial | ✅ Full v1.9.0 |
| Mobile Support | ❌ No | Basic | ✅ Full |
| Platforms | Linux | Linux, macOS | Linux, macOS, Windows, Android |
| Test Coverage | Low | Moderate | Good |
| Production Ready | ❌ No | ⚠️ Limited | ✅ Yes (advanced users) |

---

## 🔮 Future Versioning Plans

### Roadmap to v1.3.0-beta
**Internal**: v5.4.0
**Timeline**: Q2-Q3 2026
**Focus**: Performance, stability, new providers

**Expected Features**:
- Streaming video input
- Enhanced persistent storage
- Additional MCP servers
- Performance optimizations
- Improved memory search

### Roadmap to v2.0.0
**Internal**: v6.0.0
**Timeline**: 2027 (speculative)
**Breaking Changes**: Yes
**Focus**: New architecture

**Possible Changes**:
- New execution model
- Redesigned tool system
- Changed APIs
- Database schema redesign
- UI/UX overhaul

### Subsystem Independence Timeline

**v1.x - v2.x**: Integrated subsystems
**v3.x+**: Separate repositories?
- `gorkbot-sense` (v2.0.0+)
- `gorkbot-sre` (v2.0.0+)
- `gorkbot-xskill` (v2.0.0+)
- `gorkbot-core` (v3.0.0+)

---

## 💾 Version Tracking in Code

### How to Check Version

```bash
# Show version
./gorkbot.sh --version
# Output: Gorkbot version 1.2.0-beta (internal: 5.3.0)

# In TUI
/version
# Shows all versions

# Programmatically
grep "const Version" internal/platform/env.go
grep "const PublicVersion" internal/platform/env.go
```

### How to Update Version

**For Release**:
```bash
# 1. Edit internal/platform/env.go
const (
    Version = "5.4.0"           // Internal
    PublicVersion = "1.3.0-beta" // Public
)

# 2. Update subsystem versions if needed
# 3. Update VERSION file
# 4. Update VERSIONING.md
# 5. Commit with version bump message
# 6. Create git tag
```

**For Development**:
```bash
# During development, update VERSION file periodically:
echo "5.3.1-dev" > VERSION
```

---

## 📞 Version Support Policy

### Active Support Tracks

| Version | Status | Support Level | End Date |
|---------|--------|---------------|----------|
| 1.2.0-beta | CURRENT | Full | TBD (v1.3.0-beta release) |
| 1.1.0-beta | PREVIOUS | Limited | 2026-06-30 |
| 1.0.0-alpha | LEGACY | Security only | 2026-03-31 |

### Bug Fix Policy

- **Critical**: All supported versions
- **Security**: All supported versions
- **Major**: Current + previous minor
- **Minor**: Current version only

### Deprecation Policy

- **Announcement**: 1 release in advance
- **Transition**: Available for 2+ releases
- **Removal**: Only in major version bump

---

## 🔗 Related Documentation

- **README.md** - Project overview and quick start
- **GETTING_STARTED.md** - User guide and usage
- **CLAUDE.md** - Architecture and development guide
- **VERSION** - Quick reference file (plain text)
- **CHANGELOG.md** (in memory) - Historical release notes

---

## 📝 Document History

| Date | Version | Changes |
|------|---------|---------|
| 2026-03-20 | 1.2.0-beta | Initial public release, comprehensive versioning guide |
| 2026-02-26 | 1.1.0-beta | (Hypothetical previous) |
| 2026-01-15 | 1.0.0-alpha | (Hypothetical initial) |

---

**Last Updated**: March 20, 2026
**Next Update**: When v1.3.0-beta or v2.0.0 is released
**Maintained By**: Velarium AI Team
