#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
cd "$ROOT"

echo "[release-check] version consistency"
bash scripts/check_version_consistency.sh

echo "[release-check] tag/version sync policy (no tag = consistency-only)"
bash scripts/check_tag_version_sync.sh

echo "[release-check] allowlist integrity"
bash scripts/check_promotion_allowlist.sh

echo "[release-check] public hygiene"
bash scripts/check_public_hygiene.sh

echo "[release-check] promotion manifest parity"
bash scripts/check_promotion_manifest.sh

echo "[release-check] lint promotion script"
bash -n scripts/promote_to_public.sh

echo "[release-check] build static analysis"
go vet ./...

echo "[release-check] test suite"
go test ./... -count=1 -timeout=180s

echo "[release-check] OK"
