#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
ALLOWLIST_FILE="${2:-${ROOT}/configs/promotion-allowlist.txt}"
cd "$ROOT"

fail() {
  echo "[hygiene-check] ERROR: $*" >&2
  exit 1
}

# Build projected publish surface from allowlist.
[[ -f "$ALLOWLIST_FILE" ]] || fail "allowlist file not found: $ALLOWLIST_FILE"
mapfile -t allow_entries < <(grep -vE '^\s*(#|$)' "$ALLOWLIST_FILE")
[[ "${#allow_entries[@]}" -gt 0 ]] || fail "allowlist has no entries"

tmp_list="$(mktemp)"
for p in "${allow_entries[@]}"; do
  if [[ -d "$p" ]]; then
    find "$p" -type f -print >> "$tmp_list"
  elif [[ -f "$p" ]]; then
    printf '%s\n' "$p" >> "$tmp_list"
  fi
done
sort -u "$tmp_list" -o "$tmp_list"
mapfile -t projected_files < "$tmp_list"
rm -f "$tmp_list"
[[ "${#projected_files[@]}" -gt 0 ]] || fail "projected publish surface is empty"

# Exclude vendored third-party trees from policy checks.
policy_files="$(printf '%s\n' "${projected_files[@]}" | grep -vE '^ext/' || true)"
[[ -n "$policy_files" ]] || fail "policy file set is empty after exclusions"

# 1) Block internal/planning artifacts from projected publish files.
blocked_paths_regex='(^|/)(AGENTS\.md|CLAUDE\.md|for_claude\.md|for_claude\.txt|monitor_log\.txt|coverage\.out|docs/.*(remediation|checkpoint|handoff|internal|audit|report|summary).*)$'
blocked_files="$(echo "$policy_files" | grep -E "$blocked_paths_regex" || true)"
if [[ -n "$blocked_files" ]]; then
  echo "$blocked_files"
  fail "internal/planning artifacts are present in publish surface"
fi

# 2) Lightweight secret pattern check on projected publish files only.
private_key_marker='PRIVATE KEY'
private_key_pattern="-----BEGIN (RSA|EC|OPENSSH) ${private_key_marker}-----"
secret_hits="$(echo "$policy_files" | xargs grep -nE "(AKIA[0-9A-Z]{16}|${private_key_pattern})" 2>/dev/null || true)"
if [[ -n "$secret_hits" ]]; then
  echo "$secret_hits"
  fail "secret-like key material found in publish surface"
fi

# 3) Block hard-coded API key assignments in projected env/config artifacts (unless obvious placeholder).
config_files="$(echo "$policy_files" | grep -E '(\.env(\.|$)|\.yaml$|\.yml$|\.toml$|\.json$)' || true)"
if [[ -n "$config_files" ]]; then
  env_hits="$(echo "$config_files" | xargs grep -nE '^(XAI_API_KEY|GEMINI_API_KEY|ANTHROPIC_API_KEY|OPENAI_API_KEY|GORKBOT_JWT_SECRET)=' 2>/dev/null | grep -Ev '(=|=your_|=your-|=your_api_key|=placeholder|=changeme|=ENC_|=xai-xxx|=AIza-xxx|=sk-ant-xxxxxxxx|=sk-xxxxxxxx|=sk-proj-xxxxxxxx)' || true)"
  if [[ -n "$env_hits" ]]; then
    echo "$env_hits"
    fail "non-placeholder API keys/secrets found in publish surface config files"
  fi
fi

echo "[hygiene-check] OK: repository hygiene checks passed"
