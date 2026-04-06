#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${SRC:-${ROOT}}"
DST="${DST:-${ROOT}}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-${SRC}/configs/promotion-allowlist.txt}"
MANIFEST_FILE="${MANIFEST_FILE:-${SRC}/configs/promotion-manifest.txt}"

fail() {
  echo "[promotion-manifest] ERROR: $*" >&2
  exit 1
}

[[ -d "${SRC}/.git" ]] || fail "source repo not found: ${SRC}"
[[ -d "${DST}/.git" ]] || fail "destination repo not found: ${DST}"
[[ -f "${ALLOWLIST_FILE}" ]] || fail "allowlist file not found: ${ALLOWLIST_FILE}"

mapfile -t ALLOWLIST < <(grep -vE '^\s*(#|$)' "${ALLOWLIST_FILE}")
[[ "${#ALLOWLIST[@]}" -gt 0 ]] || fail "allowlist is empty: ${ALLOWLIST_FILE}"

tmp_expected="$(mktemp)"
tmp_actual="$(mktemp)"
tmp_missing="$(mktemp)"
tmp_unmanaged="$(mktemp)"
tmp_manifest="$(mktemp)"
tmp_generated="$(mktemp)"
trap 'rm -f "$tmp_expected" "$tmp_actual" "$tmp_missing" "$tmp_unmanaged" "$tmp_manifest" "$tmp_generated"' EXIT

# Expected projected file set from source allowlist (tracked files only).
ALLOWLIST_FILE="${ALLOWLIST_FILE}" OUT_FILE="${tmp_generated}" \
  bash "${ROOT}/scripts/generate_promotion_manifest.sh" "${SRC}" >/dev/null
if [[ -f "${MANIFEST_FILE}" ]]; then
  grep -vE '^\s*(#|$)' "${MANIFEST_FILE}" | sort -u > "$tmp_manifest"
else
  fail "manifest file not found: ${MANIFEST_FILE}"
fi
grep -vE '^\s*(#|$)' "${tmp_generated}" | sort -u > "$tmp_expected"

# Ensure checked-in manifest matches generated projection.
manifest_diff="$(
  comm -3 "$tmp_expected" "$tmp_manifest" || true
)"
if [[ -n "${manifest_diff}" ]]; then
  echo "${manifest_diff}" | sed -n '1,120p' >&2
  fail "manifest drift detected. Regenerate with: bash scripts/generate_promotion_manifest.sh"
fi

# Actual file set in destination under managed allowlist paths.
# Use filesystem state (not only tracked git files) so verification works
# immediately after sync, before destination commit.
for p in "${ALLOWLIST[@]}"; do
  if [[ "${p}" == */ ]]; then
    if [[ -d "${DST}/${p}" ]]; then
      (cd "${DST}" && find "${p}" -type f -print) >> "$tmp_actual"
    fi
  else
    [[ -f "${DST}/${p}" ]] && printf '%s\n' "${p}" >> "$tmp_actual"
  fi
done
sort -u "$tmp_actual" -o "$tmp_actual"

# Missing: expected in source but absent from destination managed scope.
comm -23 "$tmp_expected" "$tmp_actual" > "$tmp_missing" || true
# Unmanaged: present in destination managed scope but absent from source projection.
comm -13 "$tmp_expected" "$tmp_actual" > "$tmp_unmanaged" || true

missing_count=$(wc -l < "$tmp_missing" | tr -d ' ')
unmanaged_count=$(wc -l < "$tmp_unmanaged" | tr -d ' ')
expected_count=$(wc -l < "$tmp_expected" | tr -d ' ')
actual_count=$(wc -l < "$tmp_actual" | tr -d ' ')

echo "[promotion-manifest] expected files: ${expected_count}"
echo "[promotion-manifest] actual managed files in destination: ${actual_count}"

if [[ "$missing_count" != "0" ]]; then
  echo "[promotion-manifest] missing managed files in destination: ${missing_count}" >&2
  sed -n '1,120p' "$tmp_missing" >&2
fi

if [[ "$unmanaged_count" != "0" ]]; then
  echo "[promotion-manifest] unmanaged files present in destination managed scope: ${unmanaged_count}" >&2
  sed -n '1,120p' "$tmp_unmanaged" >&2
fi

if [[ "$missing_count" != "0" || "$unmanaged_count" != "0" ]]; then
  fail "promotion manifest mismatch detected"
fi

echo "[promotion-manifest] OK: destination managed scope matches source projection"
