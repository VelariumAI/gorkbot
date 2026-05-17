# Release Readiness

RR-001 adds local report-only release-readiness tooling. It gathers release facts,
runs validation commands, and writes a human-readable markdown report.

Run from any directory:

```bash
bash scripts/release_readiness/readiness.sh
```

Reports are written under:

```text
.local/release-readiness/reports/
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

## Private Scanner Handling

The wrapper only reports whether the private scanner path exists. Scanner
execution remains a separate private preflight for RR-001.

## Future RR Track

- RR-002 CLI smoke + config matrix
- RR-003 VAR spine fixture smoke
- RR-004 policy absence + statelock/paradox smoke
- RR-005 vector/RAG/engram preservation smoke
- RR-006 TUI/operator/session scripted smoke
- RR-007 final release report generator
