# Release Operations

This repository uses a tag-driven public release workflow.

## Release Policy

- Public release tags must match `VERSION` public version: `v<public-version>`
- Current example: public `1.6.2-rc` -> tag `v1.6.2-rc`
- Internal version (`6.2.0`) is tracked separately and validated for consistency

## Pre-Release Checklist

Run from repo root:

```bash
bash scripts/release_checklist.sh
```

This validates:
- version consistency,
- tag/version policy logic,
- promotion allowlist + manifest parity,
- hygiene checks,
- `go vet ./...`,
- `go test ./...`.

## Create a Release

1. Confirm `VERSION`, `pkg/version/version.go`, and `internal/platform/env.go` are aligned.
2. Push `main`.
3. Create and push tag:

```bash
git tag -a v1.6.2-rc -m "release: v1.6.2-rc"
git push origin v1.6.2-rc
```

## What the Release Workflow Produces

- Multi-platform binaries for `gorkbot`
- `SHA256SUMS`
- SPDX SBOM (`SBOM.spdx.json`)
- Build provenance attestation
- GitHub Release notes (auto-generated)

Tags containing a hyphen (for example `-alpha`, `-beta`, `-rc`) publish as prereleases.

## Troubleshooting

- Tag/version mismatch:
  - run `TAG_POLICY=public RELEASE_TAG=vX.Y.Z[-status] bash scripts/check_tag_version_sync.sh`
- CI gate failures:
  - run `bash scripts/release_checklist.sh` locally and resolve first failing step
