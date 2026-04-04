#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
ALLOWLIST_FILE="${2:-${ROOT}/configs/promotion-allowlist.txt}"

fail() {
  echo "[allowlist-check] ERROR: $*" >&2
  exit 1
}

[[ -f "$ALLOWLIST_FILE" ]] || fail "allowlist file not found: $ALLOWLIST_FILE"

# Ensure non-empty, non-comment entries.
mapfile -t entries < <(grep -vE '^\s*(#|$)' "$ALLOWLIST_FILE")
[[ "${#entries[@]}" -gt 0 ]] || fail "allowlist has no entries"

# Ensure no duplicate entries.
dups="$(printf '%s\n' "${entries[@]}" | sort | uniq -d)"
if [[ -n "$dups" ]]; then
  echo "$dups"
  fail "allowlist contains duplicate entries"
fi

# Ensure each allowlisted path exists in source tree.
for p in "${entries[@]}"; do
  if [[ ! -e "${ROOT}/${p}" ]]; then
    fail "allowlisted path does not exist: ${p}"
  fi
done

# Ensure script critical artifacts remain allowlisted.
required=(
  "cmd/"
  "internal/"
  "pkg/"
  "Makefile"
  "go.mod"
  "go.sum"
  "VERSION"
  "scripts/setup_wizard.sh"
  "scripts/bootstrap_native_llm.sh"
  "scripts/build_llm_bridge.sh"
)
for r in "${required[@]}"; do
  printf '%s\n' "${entries[@]}" | grep -qx "$r" || fail "required allowlist entry missing: $r"
done

echo "[allowlist-check] OK: allowlist is valid (${#entries[@]} entries)"
