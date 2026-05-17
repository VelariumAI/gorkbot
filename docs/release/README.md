# Release Readiness

RR-001 adds local report-only release-readiness tooling. It gathers release facts,
runs validation commands, and writes a human-readable markdown report. RR-002
adds local CLI smoke and config/profile matrix checks for safe operator-facing
entrypoints.

Run from any directory:

```bash
bash scripts/release_readiness/readiness.sh
```

Reports are written under:

```text
.local/release-readiness/reports/
```

Run RR-002 directly from any directory:

```bash
bash scripts/release_readiness/rr002_cli_smoke.sh
```

RR-002 reports are written under:

```text
.local/release-readiness/rr002/
```

## What RR-001 Checks

- Current branch, commit, and working tree status
- Promotion manifest health
- Focused package tests plus full test suite
- Static analysis with `go vet ./...`
- Whitespace and conflict-marker check with `git diff --check`
- Private control file tracking and public-repo presence
- Private scanner presence without automatic scanner execution
- Protected vector and semantic memory paths
- Local and remote release tag status for `v1.7.0-rc`
- Release-related file changes in this branch
- Public artifact neutrality in changed files and recent commit messages

## What RR-001 Does Not Do

- It does not create, move, or delete tags.
- It does not run release workflows.
- It does not publish releases.
- It does not upload artifacts.
- It does not run CLI/TUI interaction smoke tests.
- It does not run VAR spine fixture smoke tests.
- It does not run vector/RAG/engram smoke tests.
- It does not run Workbench/operator/session smoke tests.
- It does not claim final release readiness.

RR-001 normally reports `NEEDS_RR_SUITE` unless a concrete blocker is found.
Reports also include `status: REPORT_ONLY`.

## What RR-002 Checks

- Safe `gorkbot` and `gorkweb` help startup modes with bounded timeouts.
- Unsupported version flags as skipped checks instead of failures.
- Unsafe bare `version` arguments as deferred because no safe subcommand exists.
- Conservative profile defaults across beginner, standard, power-user, expert,
  lab, enterprise, custom, and unknown profiles.
- Trace, harness, release authority, automatic promotion, policy absence,
  vector candidate-only, and vector truth-invariant postures through local tests.
- Report generation and temporary runtime output confined to `.local/`.
- Tracked-file mutation avoidance.

## What RR-002 Does Not Do

- It does not call live providers.
- It does not make network calls.
- It does not execute app tools.
- It does not create, move, or delete tags.
- It does not run release workflows.
- It does not publish releases.
- It does not claim final release readiness.
- It does not cover RR-003+ smoke layers.

The main wrapper runs RR-002 by default:

```bash
bash scripts/release_readiness/readiness.sh
```

If RR-002 passes or is incomplete without blockers, the wrapper still reports
`NEEDS_RR_SUITE` because RR-003 and later release-readiness layers remain
deferred. If RR-002 finds a concrete blocker, the wrapper reports `BLOCKED`.

## Private Scanner Handling

The wrapper only reports whether the private scanner path exists. Scanner
execution remains a separate private preflight for RR-001.

## Future RR Track

- RR-002 CLI smoke + config matrix: implemented as local report-only smoke.
- RR-003 VAR spine fixture smoke: deferred.
- RR-004 policy absence + statelock/paradox smoke: deferred.
- RR-005 vector/RAG/engram preservation smoke: deferred.
- RR-006 TUI/operator/session scripted smoke: deferred.
- RR-007 final release report generator: deferred.
