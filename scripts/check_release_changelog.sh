#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
TAG_INPUT="${2:-${RELEASE_TAG:-}}"
CHANGELOG_FILE="${ROOT}/CHANGELOG.md"
RELEASE_WORKFLOW="${ROOT}/.github/workflows/release.yml"

fail() {
  echo "[release-changelog] ERROR: $*" >&2
  exit 1
}

if [[ -z "${TAG_INPUT}" ]]; then
  if [[ "${GITHUB_REF_TYPE:-}" == "tag" && -n "${GITHUB_REF_NAME:-}" ]]; then
    TAG_INPUT="${GITHUB_REF_NAME}"
  elif [[ -n "${GITHUB_REF:-}" && "${GITHUB_REF#refs/tags/}" != "${GITHUB_REF}" ]]; then
    TAG_INPUT="${GITHUB_REF#refs/tags/}"
  fi
fi

TAG="${TAG_INPUT#refs/tags/}"

if [[ -f "${CHANGELOG_FILE}" ]]; then
  if [[ -z "${TAG}" ]]; then
    echo "[release-changelog] INFO: CHANGELOG.md present; no tag provided, skipping tag section validation"
    exit 0
  fi
  if grep -Eq "^##[[:space:]]+${TAG}([[:space:]]|$)" "${CHANGELOG_FILE}" || \
    grep -Eq "^##[[:space:]]+v?${TAG#v}([[:space:]]|$)" "${CHANGELOG_FILE}"; then
    echo "[release-changelog] OK: changelog contains section for ${TAG}"
    exit 0
  fi
  fail "CHANGELOG.md exists but does not contain a section for tag '${TAG}'"
fi

[[ -f "${RELEASE_WORKFLOW}" ]] || fail "release workflow not found: ${RELEASE_WORKFLOW}"
if grep -Eq "generate_release_notes:[[:space:]]*true" "${RELEASE_WORKFLOW}"; then
  echo "[release-changelog] OK: CHANGELOG.md not present; release workflow uses generate_release_notes=true"
  exit 0
fi

fail "Neither CHANGELOG.md nor generate_release_notes=true found; release notes source is undefined"
