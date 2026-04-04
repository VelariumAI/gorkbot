#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"

version_file="${ROOT}/VERSION"
pkg_version_file="${ROOT}/pkg/version/version.go"
platform_version_file="${ROOT}/internal/platform/env.go"
versioning_md="${ROOT}/VERSIONING.md"

fail() {
  echo "[version-check] ERROR: $*" >&2
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

[[ -f "$version_file" ]] || fail "missing VERSION file"
[[ -f "$pkg_version_file" ]] || fail "missing pkg/version/version.go"
[[ -f "$platform_version_file" ]] || fail "missing internal/platform/env.go"

public_version="$(extract_after_header "$version_file" "## Public Release Version")"
internal_version="$(extract_after_header "$version_file" "## Internal Development Version")"
[[ -n "$public_version" ]] || fail "could not parse public version from VERSION"
[[ -n "$internal_version" ]] || fail "could not parse internal version from VERSION"

pkg_internal="$(grep -E '^const InternalVersion = ' "$pkg_version_file" | sed -E 's/^const InternalVersion = "([^"]+)".*/\1/')"
platform_internal="$(grep -E '^const Version = ' "$platform_version_file" | sed -E 's/^const Version = "([^"]+)".*/\1/')"

major="$(grep -E '^const PublicMajor = ' "$pkg_version_file" | grep -oE '[0-9]+' | head -n1)"
minor="$(grep -E '^const PublicMinor = ' "$pkg_version_file" | grep -oE '[0-9]+' | head -n1)"
build="$(grep -E '^const PublicBuild = ' "$pkg_version_file" | grep -oE '[0-9]+' | head -n1)"
revision="$(grep -E '^const PublicRevision = ' "$pkg_version_file" | grep -oE '[0-9]+' | head -n1)"
status="$(grep -E '^const PublicStatus = ' "$pkg_version_file" | sed -E 's/^const PublicStatus = "([^"]*)".*/\1/')"

[[ -n "$pkg_internal" ]] || fail "could not parse InternalVersion"
[[ -n "$platform_internal" ]] || fail "could not parse platform Version"
[[ -n "$major" && -n "$minor" && -n "$build" && -n "$revision" ]] || fail "could not parse public version components"

computed_public="${major}.${minor}.${build}"
if [[ "$revision" != "0" ]]; then
  computed_public="${computed_public}.${revision}"
fi
if [[ -n "$status" ]]; then
  computed_public="${computed_public}-${status}"
fi

[[ "$internal_version" == "$pkg_internal" ]] || fail "VERSION internal ($internal_version) != pkg InternalVersion ($pkg_internal)"
[[ "$internal_version" == "$platform_internal" ]] || fail "VERSION internal ($internal_version) != platform Version ($platform_internal)"
[[ "$public_version" == "$computed_public" ]] || fail "VERSION public ($public_version) != computed public ($computed_public)"

if [[ -f "$versioning_md" ]]; then
  grep -Fq "| **Internal** | ${internal_version} |" "$versioning_md" || fail "VERSIONING.md missing Internal=${internal_version} mapping"
  grep -Fq "| **Public** | ${public_version} |" "$versioning_md" || fail "VERSIONING.md missing Public=${public_version} mapping"
fi

echo "[version-check] OK: public=${public_version}, internal=${internal_version}"
