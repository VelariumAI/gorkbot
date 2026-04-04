#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${SRC:-${ROOT}}"
DST="${DST:-${ROOT}}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-${SRC}/configs/promotion-allowlist.txt}"

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
trap 'rm -f "$tmp_expected" "$tmp_actual" "$tmp_missing" "$tmp_unmanaged"' EXIT

# Expected projected file set from source allowlist (tracked files only).
for p in "${ALLOWLIST[@]}"; do
  if [[ "${p}" == */ ]]; then
    git -C "${SRC}" ls-files "${p}**" >> "$tmp_expected" || true
  else
    git -C "${SRC}" ls-files "${p}" >> "$tmp_expected" || true
  fi
  if [[ ! -e "${SRC}/${p}" ]]; then
    fail "allowlisted path missing in source: ${p}"
  else
    :
  fi
done
sort -u "$tmp_expected" -o "$tmp_expected"

# Actual file set in destination under managed allowlist paths.
for p in "${ALLOWLIST[@]}"; do
  if [[ "${p}" == */ ]]; then
    while IFS= read -r rel; do
      [[ -e "${DST}/${rel}" ]] && printf '%s\n' "${rel}" >> "$tmp_actual"
    done < <(git -C "${DST}" ls-files "${p}**" || true)
  else
    while IFS= read -r rel; do
      [[ -e "${DST}/${rel}" ]] && printf '%s\n' "${rel}" >> "$tmp_actual"
    done < <(git -C "${DST}" ls-files "${p}" || true)
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
