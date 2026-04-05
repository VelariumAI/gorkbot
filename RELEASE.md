# Release Guide

This repository uses a **tag-driven** release process.

## Version Source of Truth

- `VERSION` defines:
  - Public release version
  - Internal development version
- `pkg/version/version.go` and `internal/platform/env.go` must remain aligned with `VERSION`.

## Tag Policy

- Public releases use `v<public-version>`.
- Example:
  - `VERSION` public: `1.6.2-rc`
  - Release tag: `v1.6.2-rc`

## Preflight

Run before tagging:

```bash
bash scripts/release_checklist.sh
```

This runs consistency, hygiene, promotion-parity, vet, and full tests.

## Publish

1. Push `main`.
2. Create and push annotated tag:

```bash
git tag -a v1.6.2-rc -m "release: v1.6.2-rc"
git push origin v1.6.2-rc
```

GitHub Actions `Release` workflow then builds artifacts and publishes:
- multi-platform binaries,
- `SHA256SUMS`,
- `SBOM.spdx.json`,
- provenance attestation.

For workflow details, see [docs/RELEASE_OPERATIONS.md](docs/RELEASE_OPERATIONS.md).
