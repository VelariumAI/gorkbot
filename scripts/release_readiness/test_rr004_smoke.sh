#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RR004="${ROOT}/scripts/release_readiness/rr004_policy_statelock_smoke.sh"
READINESS="${ROOT}/scripts/release_readiness/readiness.sh"
DOCS="${ROOT}/docs/release/README.md"
GO_FIXTURE="${ROOT}/pkg/releasecheck/rr004_policy_statelock_test.go"

required_files=(
  "${RR004}"
  "${READINESS}"
  "${DOCS}"
  "${GO_FIXTURE}"
)

for file in "${required_files[@]}"; do
  [[ -f "${file}" ]] || {
    echo "[rr004-smoke] missing ${file}" >&2
    exit 1
  }
done

[[ -x "${RR004}" ]] || {
  echo "[rr004-smoke] rr004_policy_statelock_smoke.sh is not executable" >&2
  exit 1
}

bash -n "${RR004}"
bash -n "${READINESS}"

grep -Fq "RR-004 Policy Absence + Statelock / Paradox Smoke" "${RR004}"
grep -Fq ".local/release-readiness/rr004" "${RR004}"
grep -Fq "RR004_PASS" "${RR004}"
grep -Fq "RR004_INCOMPLETE" "${RR004}"
grep -Fq "BLOCKED" "${RR004}"
grep -Fq "go test ./pkg/releasecheck -run TestRR004PolicyStatelockSmoke -count=1 -v" "${RR004}"
grep -Fq "no policy is not permission" "${RR004}"
grep -Fq "RR-004 Policy Absence + Statelock / Paradox Smoke" "${READINESS}"
grep -Fq "bash scripts/release_readiness/rr004_policy_statelock_smoke.sh" "${READINESS}"
grep -Fq "RR-004 Policy Absence + Statelock / Paradox Smoke" "${DOCS}"
grep -Fq "bash scripts/release_readiness/rr004_policy_statelock_smoke.sh" "${DOCS}"
grep -Fq "TestRR004PolicyStatelockSmoke" "${GO_FIXTURE}"
grep -Fq "policy_absence_sensitive_action" "${GO_FIXTURE}"
grep -Fq "policy_absence_low_risk_action" "${GO_FIXTURE}"
grep -Fq "statelock_conflict" "${GO_FIXTURE}"
grep -Fq "paradox_report" "${GO_FIXTURE}"
grep -Fq "skillruntime_profile_linkage" "${GO_FIXTURE}"
grep -Fq "no policy is not permission" "${GO_FIXTURE}"

forbidden_patterns=(
  "rm -""rf"
  "git pu""sh"
  "gh rel""ease"
  "gh workflow ""run"
  "cu""rl"
  "wg""et"
  "s""sh"
  "s""cp"
  " n""c "
  "net""cat"
  "ev""al"
)

for pattern in "${forbidden_patterns[@]}"; do
  if grep -Fq "${pattern}" "${RR004}"; then
    echo "[rr004-smoke] forbidden shell pattern in rr004_policy_statelock_smoke.sh: ${pattern}" >&2
    exit 1
  fi
done

tag_pattern="git ""tag"
tag_list_pattern="git ""tag --list"
if grep -Fq "${tag_pattern}" "${RR004}" && ! grep -Fq "${tag_list_pattern}" "${RR004}"; then
  echo "[rr004-smoke] unexpected tag operation in rr004_policy_statelock_smoke.sh" >&2
  exit 1
fi

tmp_dir="${ROOT}/.local/release-readiness/rr004-smoke-test"
mkdir -p "${tmp_dir}"
tmp_root="${TMPDIR:-/tmp}"
out_file="$(mktemp "${tmp_root%/}/rr004-smoke.XXXXXX")"
trap 'rm -f "$out_file"' EXIT
(
  cd "${tmp_dir}"
  RR004_SKIP_GO_TEST=1 bash "${RR004}" >"${out_file}"
)
grep -Fq "[rr004] report path:" "${out_file}"
grep -Fq "[rr004] final recommendation:" "${out_file}"
grep -Fq "[rr004] checks passed=" "${out_file}"

echo "[rr004-smoke] OK"
