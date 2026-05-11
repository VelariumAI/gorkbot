#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-${ROOT}/configs/promotion-allowlist.txt}"
OUT_FILE="${OUT_FILE:-${ROOT}/configs/promotion-manifest.txt}"
ROOT_REAL="$(cd "${ROOT}" && pwd -P)"
ALLOWLIST_REAL="$(cd "$(dirname "${ALLOWLIST_FILE}")" && pwd -P)/$(basename "${ALLOWLIST_FILE}")"
if [[ "${ALLOWLIST_REAL}" == "${ROOT_REAL}/"* ]]; then
  REL_ALLOWLIST_PATH="${ALLOWLIST_REAL#${ROOT_REAL}/}"
else
  REL_ALLOWLIST_PATH="${ALLOWLIST_FILE}"
fi

fail() {
  echo "[promotion-manifest-gen] ERROR: $*" >&2
  exit 1
}

[[ -d "${ROOT}/.git" ]] || fail "repo not found: ${ROOT}"
[[ -f "${ALLOWLIST_FILE}" ]] || fail "allowlist file not found: ${ALLOWLIST_FILE}"

mapfile -t ALLOWLIST < <(grep -vE '^\s*(#|$)' "${ALLOWLIST_FILE}")
[[ "${#ALLOWLIST[@]}" -gt 0 ]] || fail "allowlist is empty: ${ALLOWLIST_FILE}"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

for p in "${ALLOWLIST[@]}"; do
  if [[ "${p}" == */ ]]; then
    git -C "${ROOT}" ls-files "${p}" >> "$tmp" || true
  else
    git -C "${ROOT}" ls-files "${p}" >> "$tmp" || true
  fi
done

sort -u "$tmp" -o "$tmp"
if [[ ! -s "$tmp" ]]; then
  fail "generated manifest is empty"
fi

mkdir -p "$(dirname "${OUT_FILE}")"
{
  echo "# Auto-generated promotion manifest."
  echo "# Source allowlist: ${REL_ALLOWLIST_PATH}"
  echo "# Regenerate with: bash scripts/generate_promotion_manifest.sh"
  echo "# Generated at (UTC): $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo ""
  cat "$tmp"
} > "${OUT_FILE}"

echo "[promotion-manifest-gen] wrote ${OUT_FILE} ($(wc -l < "$tmp" | tr -d ' ') files)"
