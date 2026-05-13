#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"

src_supplied="${SRC:-}"
dst_supplied="${DST:-}"
if [[ -z "${src_supplied}" && -z "${dst_supplied}" ]]; then
  echo "[promotion-unmanaged] INFO: SRC/DST not set or same repo; unmanaged destination check skipped"
  exit 0
fi

SRC="${src_supplied:-${ROOT}}"
DST="${dst_supplied:-${ROOT}}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-${SRC}/configs/promotion-allowlist.txt}"
MANIFEST_FILE="${MANIFEST_FILE:-${SRC}/configs/promotion-manifest.txt}"
ALLOW_UNMANAGED_FILE="${ALLOW_UNMANAGED_FILE:-${SRC}/configs/promotion-unmanaged-allowlist.txt}"

fail() {
  echo "[promotion-unmanaged] ERROR: $*" >&2
  exit 1
}

[[ -d "${SRC}" ]] || fail "source path not found: ${SRC}"
[[ -d "${DST}" ]] || fail "destination path not found: ${DST}"
[[ -f "${ALLOWLIST_FILE}" ]] || fail "allowlist file not found: ${ALLOWLIST_FILE}"
[[ -f "${MANIFEST_FILE}" ]] || fail "manifest file not found: ${MANIFEST_FILE}"

src_real="$(cd "${SRC}" && pwd -P)"
dst_real="$(cd "${DST}" && pwd -P)"
if [[ "${src_real}" == "${dst_real}" ]]; then
  echo "[promotion-unmanaged] INFO: SRC/DST not set or same repo; unmanaged destination check skipped"
  exit 0
fi

tmp_manifest="$(mktemp)"
tmp_allowlist="$(mktemp)"
tmp_dst="$(mktemp)"
tmp_allow="$(mktemp)"
tmp_unmanaged="$(mktemp)"
tmp_filtered="$(mktemp)"
trap 'rm -f "$tmp_manifest" "$tmp_allowlist" "$tmp_dst" "$tmp_allow" "$tmp_unmanaged" "$tmp_filtered"' EXIT

grep -vE '^\s*(#|$)' "${MANIFEST_FILE}" | sort -u > "${tmp_manifest}"
grep -vE '^\s*(#|$)' "${ALLOWLIST_FILE}" > "${tmp_allowlist}"

: > "${tmp_dst}"
while IFS= read -r p; do
  [[ -z "${p}" ]] && continue
  if [[ "${p}" == */ ]]; then
    if [[ -d "${DST}/${p}" ]]; then
      (cd "${DST}" && find "${p}" -type f ! -path '*/.git/*' -print) >> "${tmp_dst}"
    fi
  else
    [[ -f "${DST}/${p}" ]] && printf '%s\n' "${p}" >> "${tmp_dst}"
  fi
done < "${tmp_allowlist}"
sort -u "${tmp_dst}" -o "${tmp_dst}"

comm -23 "${tmp_dst}" "${tmp_manifest}" > "${tmp_unmanaged}" || true

if [[ -f "${ALLOW_UNMANAGED_FILE}" ]]; then
  grep -vE '^\s*(#|$)' "${ALLOW_UNMANAGED_FILE}" | sort -u > "${tmp_allow}" || true
else
  : > "${tmp_allow}"
fi

if [[ -s "${tmp_allow}" ]]; then
  grep -vxFf "${tmp_allow}" "${tmp_unmanaged}" > "${tmp_filtered}" || true
  mv "${tmp_filtered}" "${tmp_unmanaged}"
fi

count="$(wc -l < "${tmp_unmanaged}" | tr -d ' ')"
if [[ "${count}" != "0" ]]; then
  echo "[promotion-unmanaged] unmanaged files present in destination managed scope: ${count}" >&2
  sed -n '1,120p' "${tmp_unmanaged}" >&2
  fail "destination contains files outside promotion manifest"
fi

echo "[promotion-unmanaged] OK: destination managed scope has no unmanaged files"
