#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

required_files=(
  "scripts/release_readiness/readiness.sh"
  "scripts/release_readiness/lib/common.sh"
  "scripts/release_readiness/lib/report.sh"
  "docs/release/README.md"
)

for file in "${required_files[@]}"; do
  [[ -f "${ROOT}/${file}" ]] || {
    echo "[readiness-smoke] missing ${file}" >&2
    exit 1
  }
done

[[ -x "${ROOT}/scripts/release_readiness/readiness.sh" ]] || {
  echo "[readiness-smoke] readiness.sh is not executable" >&2
  exit 1
}

bash -n "${ROOT}/scripts/release_readiness/readiness.sh"
bash -n "${ROOT}/scripts/release_readiness/lib/common.sh"
bash -n "${ROOT}/scripts/release_readiness/lib/report.sh"

grep -Fq "# Gorkbot Release Readiness Report" "${ROOT}/scripts/release_readiness/lib/report.sh"
grep -Fq "NEEDS_RR_SUITE" "${ROOT}/scripts/release_readiness/readiness.sh"
grep -Fq "REPORT_ONLY" "${ROOT}/scripts/release_readiness/readiness.sh"
grep -Fq "go test ./pkg/governance" "${ROOT}/scripts/release_readiness/readiness.sh"
grep -Fq "main...HEAD" "${ROOT}/scripts/release_readiness/readiness.sh"

set +u
source "${ROOT}/scripts/release_readiness/lib/common.sh"
source "${ROOT}/scripts/release_readiness/lib/report.sh"
set -u

output=""
rr_run_shell_capture output "printf helper-ok"
[[ "${output}" == "helper-ok" ]] || {
  echo "[readiness-smoke] shell capture helper did not assign caller variable" >&2
  exit 1
}

main_ref=""
rr_release_base_ref main_ref
[[ "${main_ref}" == "main" ]] || {
  echo "[readiness-smoke] expected release base ref to resolve to main" >&2
  exit 1
}

main_range=""
rr_release_branch_range main_range
[[ "${main_range}" == "main...HEAD" ]] || {
  echo "[readiness-smoke] expected release branch range main...HEAD" >&2
  exit 1
}

smoke_report="${ROOT}/.local/release-readiness/smoke-report.md"
mkdir -p "$(dirname "${smoke_report}")"
rr_report_begin "${smoke_report}"
rr_report_command "${smoke_report}" "Smoke command" "printf ok" "0" "ok"
grep -Fq -- "- Command: \`printf ok\`" "${smoke_report}"

echo "[readiness-smoke] OK"
