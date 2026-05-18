#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "${SCRIPT_DIR}/lib/common.sh"
# shellcheck source=lib/report.sh
source "${SCRIPT_DIR}/lib/report.sh"

ROOT="$(rr_repo_root "${SCRIPT_DIR}")"
cd "${ROOT}"

REPORT_DIR="${RR003_REPORT_DIR:-${ROOT}/.local/release-readiness/rr003}"
RUN_DIR="${REPORT_DIR}/run"
mkdir -p "${REPORT_DIR}" "${RUN_DIR}/home" "${RUN_DIR}/gocache" "${RUN_DIR}/tmp"
REPORT_FILE="${REPORT_DIR}/rr003-var-spine-smoke.$(rr_timestamp).md"

TIMEOUT_SECONDS="${RR003_TIMEOUT_SECONDS:-120}"
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
BLOCKERS=""
FINAL_RECOMMENDATION="RR003_PASS"

rr003_mark_blocker() {
  rr_append_unique_line "$1" BLOCKERS
}

rr003_record_pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
}

rr003_record_fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  rr003_mark_blocker "$1"
}

rr003_record_skip() {
  SKIP_COUNT=$((SKIP_COUNT + 1))
}

rr003_truncate_output() {
  awk 'NR <= 140 { print; next } NR == 141 { print "... [truncated]"; exit }'
}

rr003_report_check() {
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
      printf '%s\n' "${output}" | rr003_truncate_output
    else
      printf '(no output)\n'
    fi
    printf '```\n\n'
  } >> "${REPORT_FILE}"
}

rr003_run_fixture_go_test() {
  local __out_var="$1"
  rr_run_go_capture "${__out_var}" "rr003" "${TIMEOUT_SECONDS}" "${RUN_DIR}" \
    go test "$(rr_go_test_args)" ./pkg/releasecheck -run TestRR003VARSpineFixture -count=1 -v
}

rr_report_begin "${REPORT_FILE}"
rr_report_section "${REPORT_FILE}" "RR-003 VAR Spine Fixture Smoke" "Local fixture-based release-readiness smoke for profile/config, evidence receipts, harness refs, replay refs, statelock posture, skillruntime facade lifecycle, and trace linkage. Provider calls, app tool execution, network calls, release workflows, tag mutation, and release publication are not performed."

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
rr003_record_pass

inventory_body="$(
  printf 'local APIs usable for fixture VAR checks:\n'
  printf 'profile.DefaultConfig, profile.EvaluateCapability, profile.ConfigRef\n'
  printf 'evidence.Assessment, evidence.Record, evidence.Receipt, evidence refs\n'
  printf 'harness.Registry, harness.Artifact, harness.Report.ValidationRef\n'
  printf 'replay.CaseFromTrajectory, replay.Runner, replay.Result\n'
  printf 'statelock.Evaluator.Check, statelock.ProposedStateFromReplayResult, statelock.ParadoxRef\n'
  printf 'skillruntime.Facade.Stage, skillruntime.Evaluate, skillruntime receipts\n'
  printf 'trace.Trajectory, trace.Ref, trace operator path\n'
  printf 'safe/deferred behavior:\n'
  printf 'no provider calls; no app tool execution; no network calls; no release/tag/workflow mutation\n'
  printf 'vector/semantic memory inspected only: yes\n'
  printf 'likely sensitive sinks in planned changes: shell invokes bounded local go test; report files under .local only\n'
)"
rr_report_list "${REPORT_FILE}" "Initial inventory findings" "${inventory_body}"
rr003_record_pass

safety_body="$(
  printf 'network calls: no\n'
  printf 'provider calls: no\n'
  printf 'app tool execution: no\n'
  printf 'release workflow execution: no\n'
  printf 'tag creation/movement/deletion: no\n'
  printf 'release publication: no\n'
  printf 'tracked file mutation by RR-003 runtime: no\n'
  printf 'fixture output confinement: %s\n' "${REPORT_DIR}"
)"
rr_report_list "${REPORT_FILE}" "Operational safety posture" "${safety_body}"
rr003_record_pass

rr_print_go_profile "rr003"

if ! command -v timeout >/dev/null 2>&1; then
  rr003_report_check "Timeout availability" "command -v timeout" "1" "BLOCKED" "timeout command is required for bounded RR-003 fixture execution"
  rr003_record_fail "timeout command unavailable"
fi

fixture_command="go test -p=1 ./pkg/releasecheck -run TestRR003VARSpineFixture -count=1 -v"
fixture_output=""
fixture_status=0
if [[ "${RR003_SKIP_GO_TEST:-0}" == "1" ]]; then
  skip_output="RR003_SKIP_GO_TEST=1; fixture package execution skipped by request."
  rr003_report_check "Fixture workflow" "${fixture_command}" "not-run" "SKIP" "${skip_output}"
  rr003_record_skip
else
  set +e
  rr003_run_fixture_go_test fixture_output
  fixture_status=$?
  set -e

  if [[ "${fixture_status}" == "0" ]]; then
    rr003_report_check "Fixture workflow" "${fixture_command}" "${fixture_status}" "PASS" "${fixture_output}"
    rr003_record_pass
  elif [[ "${fixture_status}" == "124" ]]; then
    rr003_report_check "Fixture workflow" "${fixture_command}" "${fixture_status}" "BLOCKED" "${fixture_output}"
    rr003_record_fail "RR-003 fixture workflow timed out"
  else
    rr003_report_check "Fixture workflow" "${fixture_command}" "${fixture_status}" "BLOCKED" "${fixture_output}"
    rr003_record_fail "RR-003 fixture workflow failed with exit code ${fixture_status}"
  fi
fi

scenario_body="$(
  printf 'happy path:\n'
  printf 'classification: PASS if fixture Go test stages candidate with coherent profile, evidence, harness, replay, statelock, skillruntime, and trace refs\n'
  printf 'negative path:\n'
  printf 'classification: PASS if missing evidence is surfaced and vector_assert_truth is denied as authority\n'
  printf 'operator-facing report quality:\n'
  printf 'scenario logs include name, command/test invoked, story, expected outcome, actual outcome, evidence/ref summary, classification, and weak seams\n'
  printf 'observed weak seams:\n'
  printf 'missing evidence currently resolves to approval_required rather than a more specific incomplete status; RR-004+ can sharpen policy absence/statelock reporting\n'
  printf 'scenario output lines:\n'
  if [[ -n "${fixture_output}" ]]; then
    printf '%s\n' "${fixture_output}" | grep 'RR003_' || true
  else
    printf '(fixture not run)\n'
  fi
)"
rr_report_list "${REPORT_FILE}" "Fixture scenario behavior" "${scenario_body}"
if [[ "${RR003_SKIP_GO_TEST:-0}" == "1" ]]; then
  rr003_record_skip
elif [[ "${fixture_status}" == "0" ]]; then
  rr003_record_pass
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
  rr003_record_fail "working tree status changed during RR-003"
else
  rr003_record_pass
fi

if [[ -n "${BLOCKERS}" ]]; then
  FINAL_RECOMMENDATION="BLOCKED"
elif [[ "${SKIP_COUNT}" != "0" ]]; then
  FINAL_RECOMMENDATION="RR003_INCOMPLETE"
else
  FINAL_RECOMMENDATION="RR003_PASS"
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

echo "[rr003] report path: ${REPORT_FILE}"
echo "[rr003] final recommendation: ${FINAL_RECOMMENDATION}"
echo "[rr003] checks passed=${PASS_COUNT} failed=${FAIL_COUNT} skipped=${SKIP_COUNT}"

if [[ "${FINAL_RECOMMENDATION}" == "BLOCKED" ]]; then
  exit 1
fi
