# Release Readiness

RR-001 adds local report-only release-readiness tooling. It gathers release facts,
runs validation commands, and writes a human-readable markdown report. RR-002
adds local CLI smoke and config/profile matrix checks for safe operator-facing
entrypoints. RR-003 adds a fixture-based VAR spine smoke that exercises local
validation, audit, and replay-style readiness without live providers or release
mutation.

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

Run RR-003 directly from any directory:

```bash
bash scripts/release_readiness/rr003_var_spine_smoke.sh
```

RR-003 reports are written under:

```text
.local/release-readiness/rr003/
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
- It does not claim VAR spine fixture coverage unless the nested RR-003 section
  runs and records its own recommendation.
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
`NEEDS_RR_SUITE` because later release-readiness layers remain deferred. If
RR-002 finds a concrete blocker, the wrapper reports `BLOCKED`.

## RR-003 VAR Spine Fixture Smoke

### What RR-003 Checks

- A happy-path local fixture flow through profile/config posture, evidence
  assessment/record/receipt posture, harness report/ref posture, replay
  result/ref posture, statelock no-conflict posture, skillruntime staging, and
  trace/ref linkage.
- Negative-path fixture behavior for missing harness/replay/statelock evidence.
- Negative-path hard-invariant behavior for `vector_assert_truth` attempts as
  authority.
- Operator-facing report quality: scenario name, command invoked, operator
  story, expected outcome, actual outcome, evidence/ref summary, classification,
  and weak seams.
- Report generation and runtime output confined to `.local/release-readiness/rr003/`.
- Tracked-file mutation avoidance.

### What RR-003 Does Not Do

- It does not call live providers.
- It does not make network calls.
- It does not execute app tools.
- It does not create, move, or delete tags.
- It does not run release workflows.
- It does not publish releases.
- It does not claim final release readiness.
- It does not provide the full RR-004 policy absence/statelock/paradox suite.
- It does not provide RR-005 vector/RAG/engram preservation coverage.
- It does not provide RR-006 TUI/operator/session scripting.
- It does not provide RR-007 final release report generation.

The main wrapper runs RR-003 by default:

```bash
bash scripts/release_readiness/readiness.sh
```

If RR-003 passes or is incomplete without blockers, the wrapper still reports
`NEEDS_RR_SUITE` because RR-004 and later release-readiness layers remain
deferred. If RR-003 finds a concrete blocker, the wrapper reports `BLOCKED`.
RR-003 starts the transition from shallow smoke checks toward operator-like
fixture validation.

## Private Scanner Handling

The wrapper only reports whether the private scanner path exists. Scanner
execution remains a separate private preflight for RR-001.

## Future RR Track

- RR-002 CLI smoke + config matrix: implemented as local report-only smoke.
- RR-003 VAR spine fixture smoke: implemented as local fixture validation.
- RR-004 policy absence + statelock/paradox smoke: deferred.
- RR-005 vector/RAG/engram preservation smoke: deferred.
- RR-006 TUI/operator/session scripted smoke: deferred.
- RR-007 final release report generator: deferred.
