#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
WF="${ROOT}/.github/workflows/release.yml"

fail() {
  echo "[release-artifacts] ERROR: $*" >&2
  exit 1
}

[[ -f "${WF}" ]] || fail "release workflow not found: ${WF}"

require_line() {
  local pattern="$1"
  local label="$2"
  if ! grep -Fq "${pattern}" "${WF}"; then
    fail "missing ${label} (${pattern})"
  fi
}

# Required platform/arch release matrix.
require_line "goos: linux" "linux goos entry"
require_line "goarch: amd64" "amd64 goarch entry"
require_line "goarch: arm64" "arm64 goarch entry"
require_line "goos: darwin" "darwin goos entry"
require_line "goos: windows" "windows goos entry"

# Required release artifacts.
require_line "dist/gorkbot-*" "compiled binary artifact glob"
require_line "dist/SHA256SUMS" "checksum artifact"
require_line "dist/SBOM.spdx.json" "sbom artifact"
require_line "sha256sum gorkbot-* > SHA256SUMS" "checksum generation step"
require_line "uses: anchore/sbom-action@v0" "SBOM generation action"

echo "[release-artifacts] OK: release matrix and artifact expectations are present"
