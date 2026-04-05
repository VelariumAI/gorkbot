#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
TAG_INPUT="${2:-${RELEASE_TAG:-}}"
POLICY="${3:-${TAG_POLICY:-public}}"

version_file="${ROOT}/VERSION"
check_version_script="${ROOT}/scripts/check_version_consistency.sh"

fail() {
  echo "[tag-sync] ERROR: $*" >&2
  exit 1
}

extract_after_header() {
  local file="$1"
  local header="$2"
  awk -v hdr="$header" '
    $0 == hdr {found=1; next}
    found && NF {gsub(/^ +| +$/, "", $0); print; exit}
  ' "$file"
}

[[ -f "$check_version_script" ]] || fail "missing $check_version_script"
bash "$check_version_script" "$ROOT"

[[ -f "$version_file" ]] || fail "missing VERSION file"
public_version="$(extract_after_header "$version_file" "## Public Release Version")"
internal_version="$(extract_after_header "$version_file" "## Internal Development Version")"
[[ -n "$public_version" ]] || fail "could not parse public version from VERSION"
[[ -n "$internal_version" ]] || fail "could not parse internal version from VERSION"

if [[ -z "$TAG_INPUT" ]]; then
  if [[ "${GITHUB_REF_TYPE:-}" == "tag" && -n "${GITHUB_REF_NAME:-}" ]]; then
    TAG_INPUT="${GITHUB_REF_NAME}"
  elif [[ -n "${GITHUB_REF:-}" && "${GITHUB_REF#refs/tags/}" != "${GITHUB_REF}" ]]; then
    TAG_INPUT="${GITHUB_REF#refs/tags/}"
  fi
fi

if [[ -z "$TAG_INPUT" ]]; then
  echo "[tag-sync] INFO: no tag provided; version consistency validated only"
  exit 0
fi

# Accept full refs (refs/tags/v1.6.0-rc) or short tags (v1.6.0-rc)
TAG="${TAG_INPUT#refs/tags/}"

case "$POLICY" in
  public)
    expected="v${public_version}"
    [[ "$TAG" == "$expected" ]] || fail "tag '$TAG' must match public version '$expected'"
    ;;
  internal)
    expected="v${internal_version}"
    [[ "$TAG" == "$expected" ]] || fail "tag '$TAG' must match internal version '$expected'"
    ;;
  either)
    expected_public="v${public_version}"
    expected_internal="v${internal_version}"
    if [[ "$TAG" != "$expected_public" && "$TAG" != "$expected_internal" ]]; then
      fail "tag '$TAG' must match either '$expected_public' or '$expected_internal'"
    fi
    ;;
  *)
    fail "unsupported policy '$POLICY' (expected: public|internal|either)"
    ;;
esac

echo "[tag-sync] OK: tag=${TAG}, policy=${POLICY}, public=${public_version}, internal=${internal_version}"
