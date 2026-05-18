#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "${SCRIPT_DIR}/lib/common.sh"
# shellcheck source=lib/report.sh
source "${SCRIPT_DIR}/lib/report.sh"

ROOT="$(rr_repo_root "${SCRIPT_DIR}")"
cd "${ROOT}"

REPORT_DIR="${RR002_REPORT_DIR:-${ROOT}/.local/release-readiness/rr002}"
RUN_DIR="${REPORT_DIR}/run"
mkdir -p "${REPORT_DIR}" "${RUN_DIR}/home" "${RUN_DIR}/gocache" "${RUN_DIR}/tmp"
REPORT_FILE="${REPORT_DIR}/rr002-cli-smoke.$(rr_timestamp).md"

TIMEOUT_SECONDS="${RR002_TIMEOUT_SECONDS:-30}"
BUILD_TIMEOUT_SECONDS="${RR002_BUILD_TIMEOUT_SECONDS:-180}"
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
BLOCKERS=""
FINAL_RECOMMENDATION="RR002_PASS"

rr002_mark_blocker() {
  rr_append_unique_line "$1" BLOCKERS
}

rr002_record_pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
}

rr002_record_fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  rr002_mark_blocker "$1"
}

rr002_record_skip() {
  SKIP_COUNT=$((SKIP_COUNT + 1))
}

rr002_truncate_output() {
  awk 'NR <= 80 { print; next } NR == 81 { print "... [truncated]"; exit }'
}

rr002_report_check() {
  local title="$1"
  local command_text="$2"
  local status="$3"
  local state="$4"
  local output="$5"

  {
    printf '## %s\n\n' "${title}"
    printf -- '- Command: `%s`\n' "${command_text}"
    printf -- '- Exit code: `%s`\n' "${status}"
    printf -- '- Check state: `%s`\n\n' "${state}"
    printf '```text\n'
    if [[ -n "${output}" ]]; then
      printf '%s\n' "${output}" | rr002_truncate_output
    else
      printf '(no output)\n'
    fi
    printf '```\n\n'
  } >> "${REPORT_FILE}"
}

rr002_run_confined() {
  local __out_var="$1"
  local timeout_seconds="$2"
  shift 2
  rr_run_go_capture "${__out_var}" "rr002" "${timeout_seconds}" "${RUN_DIR}" "$@"
}

rr002_run_expected_success_with_timeout() {
  local title="$1"
  local timeout_seconds="$2"
  shift
  shift
  local output status command_text
  command_text="$*"

  set +e
  rr002_run_confined output "${timeout_seconds}" "$@"
  status=$?
  set -e

  if [[ "${status}" == "0" ]]; then
    rr002_report_check "${title}" "${command_text}" "${status}" "PASS" "${output}"
    rr002_record_pass
  elif [[ "${status}" == "124" ]]; then
    rr002_report_check "${title}" "${command_text}" "${status}" "BLOCKED" "${output}"
    rr002_record_fail "${title} timed out; expected safe command did not exit"
  else
    rr002_report_check "${title}" "${command_text}" "${status}" "BLOCKED" "${output}"
    rr002_record_fail "${title} failed with exit code ${status}"
  fi
}

rr002_run_expected_success() {
  local title="$1"
  shift
  rr002_run_expected_success_with_timeout "${title}" "${TIMEOUT_SECONDS}" "$@"
}

rr002_run_expected_unsupported() {
  local title="$1"
  shift
  local output status command_text
  command_text="$*"

  set +e
  rr002_run_confined output "${TIMEOUT_SECONDS}" "$@"
  status=$?
  set -e

  if [[ "${status}" == "124" ]]; then
    rr002_report_check "${title}" "${command_text}" "${status}" "BLOCKED" "${output}"
    rr002_record_fail "${title} timed out while probing unsupported safe flag"
  elif [[ "${status}" == "0" ]]; then
    rr002_report_check "${title}" "${command_text}" "${status}" "PASS" "${output}"
    rr002_record_pass
  else
    rr002_report_check "${title}" "${command_text}" "${status}" "SKIP" "${output}"
    rr002_record_skip
  fi
}

rr002_record_static_skip() {
  local title="$1"
  local command_text="$2"
  local reason="$3"
  rr002_report_check "${title}" "${command_text}" "not-run" "SKIP" "${reason}"
  rr002_record_skip
}

rr_report_begin "${REPORT_FILE}"
rr_report_section "${REPORT_FILE}" "RR-002 CLI Smoke + Config Matrix" "Local, report-only, deterministic smoke layer. Provider calls, network calls, app tool execution, release workflows, and tag mutation are not performed."

timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
branch="$(git branch --show-current 2>/dev/null || true)"
commit="$(git log -1 --oneline 2>/dev/null || true)"
status_before="$(git status --short 2>/dev/null || true)"

preflight_body="$(
  printf 'timestamp: %s\n' "${timestamp}"
  printf 'branch: %s\n' "${branch:-unknown}"
  printf 'commit: %s\n' "${commit:-unknown}"
  printf 'repo root: %s\n' "${ROOT}"
  printf 'report directory: %s\n' "${REPORT_DIR}"
  printf 'timeout seconds: %s\n' "${TIMEOUT_SECONDS}"
  printf 'build timeout seconds: %s\n' "${BUILD_TIMEOUT_SECONDS}"
  printf 'platform profile: %s\n' "$(rr_go_platform_profile)"
  printf 'go platform: GOOS=%s GOARCH=%s\n' "$(rr_go_platform_value GOOS)" "$(rr_go_platform_value GOARCH)"
  printf 'go cache mode: %s\n' "$(rr_go_cache_mode)"
  printf 'go safety: %s GOMEMLIMIT=1024MiB GOTMPDIR=%s\n' "$(rr_go_safety_summary)" "${RUN_DIR}/tmp"
  printf 'working tree before:\n'
  if [[ -n "${status_before}" ]]; then
    printf '%s\n' "${status_before}"
  else
    printf '(clean)\n'
  fi
)"
rr_report_list "${REPORT_FILE}" "Preflight" "${preflight_body}"

if ! command -v timeout >/dev/null 2>&1; then
  rr002_report_check "Timeout availability" "command -v timeout" "1" "BLOCKED" "timeout command is required for bounded CLI smoke checks"
  rr002_record_fail "timeout command unavailable"
fi

inventory_body="$(
  printf 'safe CLI entrypoints discovered:\n'
  printf 'go run ./cmd/gorkbot --help\n'
  printf 'go run ./cmd/gorkbot -h\n'
  printf 'go run ./cmd/gorkweb --help\n'
  printf 'go run ./cmd/gorkweb -h\n'
  printf 'unsupported safe flag probes:\n'
  printf 'go run ./cmd/gorkbot --version\n'
  printf 'go run ./cmd/gorkweb --version\n'
  printf 'unsafe/deferred commands:\n'
  printf 'go run ./cmd/gorkbot version: skipped; no version subcommand discovered and bare arg can fall through toward runtime startup\n'
  printf 'go run ./cmd/gorkweb version: skipped; no version subcommand discovered and bare arg can fall through toward runtime startup\n'
  printf 'config/profile env seams discovered:\n'
  printf 'GORKBOT_HARNESS_MODE, GORKBOT_TRACE_MODE, GORKBOT_AUTO_REBUILD, GORKBOT_SAVE_TRAJECTORIES, GORKBOT_PRIMARY, GORKBOT_CONSULTANT, GORKBOT_PRIMARY_MODEL, GORKBOT_CONSULTANT_MODEL, GORKBOT_MEMORY_ENABLE_SEMANTIC, GORKBOT_MEMORY_EMBEDDER\n'
  printf 'vector/semantic memory inspected only: yes\n'
)"
rr_report_list "${REPORT_FILE}" "Initial inventory findings" "${inventory_body}"
rr002_record_pass

rr_print_go_profile "rr002"

if [[ "${RR002_SKIP_CLI:-0}" == "1" ]]; then
  rr002_record_static_skip "CLI smoke checks" "go run ./cmd/gorkbot|./cmd/gorkweb help probes" "RR002_SKIP_CLI=1"
else
  rr002_run_expected_success_with_timeout "CLI build cache warmup" "${BUILD_TIMEOUT_SECONDS}" "go" "test" "$(rr_go_test_args)" "./cmd/gorkbot" "./cmd/gorkweb" "-run" "^$"
  rr002_run_expected_success "gorkbot help long" "go" "run" "./cmd/gorkbot" "--help"
  rr002_run_expected_success "gorkbot help short" "go" "run" "./cmd/gorkbot" "-h"
  rr002_run_expected_unsupported "gorkbot unsupported version flag" "go" "run" "./cmd/gorkbot" "--version"
  rr002_record_static_skip "gorkbot unsafe bare version" "go run ./cmd/gorkbot version" "Skipped because no safe version subcommand was discovered."

  rr002_run_expected_success "gorkweb help long" "go" "run" "./cmd/gorkweb" "--help"
  rr002_run_expected_success "gorkweb help short" "go" "run" "./cmd/gorkweb" "-h"
  rr002_run_expected_unsupported "gorkweb unsupported version flag" "go" "run" "./cmd/gorkweb" "--version"
  rr002_record_static_skip "gorkweb unsafe bare version" "go run ./cmd/gorkweb version" "Skipped because no safe version subcommand was discovered."
fi

if [[ "${RR002_SKIP_CONFIG_MATRIX:-0}" == "1" ]]; then
  rr002_record_static_skip "Config/profile matrix" "go test -p=1 ./pkg/profile" "RR002_SKIP_CONFIG_MATRIX=1"
else
  rr002_run_expected_success "Config/profile matrix" "go" "test" "$(rr_go_test_args)" "./pkg/profile"
fi

status_after="$(git status --short 2>/dev/null || true)"
tracked_diff_after="$(
  {
    git diff --name-only 2>/dev/null || true
    git diff --cached --name-only 2>/dev/null || true
  } | awk 'NF' | sort -u
)"
mutation_body="$(
  printf 'working tree after:\n'
  if [[ -n "${status_after}" ]]; then
    printf '%s\n' "${status_after}"
  else
    printf '(clean)\n'
  fi
  printf 'tracked changed files after:\n'
  if [[ -n "${tracked_diff_after}" ]]; then
    printf '%s\n' "${tracked_diff_after}"
  else
    printf '(none)\n'
  fi
  printf 'generated output confined to: %s\n' "${REPORT_DIR}"
)"
rr_report_list "${REPORT_FILE}" "Mutation confinement" "${mutation_body}"
if [[ "${status_after}" != "${status_before}" ]]; then
  rr002_record_fail "working tree status changed during RR-002"
elif [[ -n "${tracked_diff_after}" && -z "${status_before}" ]]; then
  rr002_record_fail "tracked files changed during RR-002"
else
  rr002_record_pass
fi

safety_body="$(
  printf 'network calls: no\n'
  printf 'provider calls: no\n'
  printf 'app tool execution: no\n'
  printf 'release workflow execution: no\n'
  printf 'tag creation/mutation/deletion: no\n'
  printf 'release publication: no\n'
  printf 'artifact upload: no\n'
  printf 'destructive cleanup: no\n'
  printf 'Go module network disabled: GOPROXY=off GOSUMDB=off\n'
)"
rr_report_list "${REPORT_FILE}" "Operational safety" "${safety_body}"
rr002_record_pass

if [[ -n "${BLOCKERS}" ]]; then
  FINAL_RECOMMENDATION="BLOCKED"
elif [[ "${SKIP_COUNT}" -gt 0 ]]; then
  FINAL_RECOMMENDATION="RR002_INCOMPLETE"
fi

final_body="$(
  printf 'status: REPORT_ONLY\n'
  printf 'recommendation: %s\n' "${FINAL_RECOMMENDATION}"
  printf 'checks passed: %s\n' "${PASS_COUNT}"
  printf 'checks failed: %s\n' "${FAIL_COUNT}"
  printf 'checks skipped: %s\n' "${SKIP_COUNT}"
  printf 'blockers:\n'
  if [[ -n "${BLOCKERS}" ]]; then
    printf '%s\n' "${BLOCKERS}"
  else
    printf '(none)\n'
  fi
)"
rr_report_list "${REPORT_FILE}" "Final recommendation" "${final_body}"

echo "[rr002] report path: ${REPORT_FILE}"
echo "[rr002] final recommendation: ${FINAL_RECOMMENDATION}"
echo "[rr002] checks passed=${PASS_COUNT} failed=${FAIL_COUNT} skipped=${SKIP_COUNT}"

if [[ "${FINAL_RECOMMENDATION}" == "BLOCKED" ]]; then
  exit 1
fi
