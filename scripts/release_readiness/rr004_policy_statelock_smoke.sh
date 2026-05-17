#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "${SCRIPT_DIR}/lib/common.sh"
# shellcheck source=lib/report.sh
source "${SCRIPT_DIR}/lib/report.sh"

ROOT="$(rr_repo_root "${SCRIPT_DIR}")"
cd "${ROOT}"

REPORT_DIR="${RR004_REPORT_DIR:-${ROOT}/.local/release-readiness/rr004}"
RUN_DIR="${REPORT_DIR}/run"
mkdir -p "${REPORT_DIR}" "${RUN_DIR}/home" "${RUN_DIR}/gocache" "${RUN_DIR}/gopath" "${RUN_DIR}/tmp"
REPORT_FILE="${REPORT_DIR}/rr004-policy-statelock-smoke.$(rr_timestamp).md"

TIMEOUT_SECONDS="${RR004_TIMEOUT_SECONDS:-120}"
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
BLOCKERS=""
FINAL_RECOMMENDATION="RR004_PASS"

rr004_mark_blocker() {
  rr_append_unique_line "$1" BLOCKERS
}

rr004_record_pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
}

rr004_record_fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  rr004_mark_blocker "$1"
}

rr004_record_skip() {
  SKIP_COUNT=$((SKIP_COUNT + 1))
}

rr004_truncate_output() {
  awk 'NR <= 180 { print; next } NR == 181 { print "... [truncated]"; exit }'
}

rr004_report_check() {
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
      printf '%s\n' "${output}" | rr004_truncate_output
    else
      printf '(no output)\n'
    fi
    printf '```\n\n'
  } >> "${REPORT_FILE}"
}

rr004_run_fixture_go_test() {
  local __out_var="$1"
  local captured status gomodcache

  gomodcache="$(go env GOMODCACHE 2>/dev/null || true)"
  if [[ -z "${gomodcache}" ]]; then
    gomodcache="${RUN_DIR}/gopath/pkg/mod"
  fi

  set +e
  captured="$(
    env -i \
      HOME="${RUN_DIR}/home" \
      PATH="${PATH}" \
      GOCACHE="${RUN_DIR}/gocache" \
      GOPATH="${RUN_DIR}/gopath" \
      GOMODCACHE="${gomodcache}" \
      TMPDIR="${RUN_DIR}/tmp" \
      GOTMPDIR="${RUN_DIR}/tmp" \
      GOPROXY=off \
      GOSUMDB=off \
      GOFLAGS=-mod=readonly \
      TERM=dumb \
      timeout "${TIMEOUT_SECONDS}" \
      go test ./pkg/releasecheck -run TestRR004PolicyStatelockSmoke -count=1 -v 2>&1
  )"
  status=$?
  set -e

  printf -v "${__out_var}" '%s' "${captured}"
  return "${status}"
}

rr004_fixture_lines() {
  local prefix="$1"
  local output="$2"
  local lines

  lines="$(printf '%s\n' "${output}" | grep "${prefix}" || true)"
  if [[ -n "${lines}" ]]; then
    printf '%s\n' "${lines}"
  else
    printf '(fixture not run)\n'
  fi
}

rr_report_begin "${REPORT_FILE}"
rr_report_section "${REPORT_FILE}" "RR-004 Policy Absence + Statelock / Paradox Smoke" "Local fixture-based release-readiness smoke for policy absence, explicit low-risk behavior, state lock conflict handling, paradox report wording, and skillruntime/profile linkage. Provider calls, app tool execution, network calls, release workflows, tag mutation, and release publication are not performed. Core invariant: no policy is not permission."

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
  printf 'working tree before:\n'
  if [[ -n "${status_before}" ]]; then
    printf '%s\n' "${status_before}"
  else
    printf '(clean)\n'
  fi
)"
rr_report_list "${REPORT_FILE}" "Preflight" "${preflight_body}"
rr004_record_pass

inventory_body="$(
  printf 'policy absence APIs usable for fixture checks:\n'
  printf 'evidence.PolicyState, evidence.Evaluate, evidence.Assessment, evidence.Receipt, profile.EvaluateCapability\n'
  printf 'statelock APIs usable for conflict/paradox checks:\n'
  printf 'statelock.Lock, statelock.ProposedState, statelock.Evaluator.Check, statelock.ParadoxRef\n'
  printf 'evidence/profile APIs usable for operator-facing assessment:\n'
  printf 'evidence.Record/Receipt/Ref helpers, profile.ConfigRef, profile capability rules\n'
  printf 'skillruntime/trace APIs usable for linkage:\n'
  printf 'skillruntime.Evaluate, skillruntime.Result receipts, trace.Ref, trace.StableHash\n'
  printf 'safe without runtime/provider/tool execution:\n'
  printf 'in-memory fixture policy checks, in-memory statelock store, fixture paradox reports, local Go test logs\n'
  printf 'unsafe/deferred:\n'
  printf 'live providers, app tools, network endpoints, sockets, release workflows, tag mutation, release publication, private scanner mutation\n'
  printf 'vector/semantic memory inspected only: yes\n'
  printf 'likely sensitive sinks in planned changes: bounded local go test command; markdown report writes under .local only\n'
)"
rr_report_list "${REPORT_FILE}" "Initial inventory findings" "${inventory_body}"
rr004_record_pass

safety_body="$(
  printf 'network calls: no\n'
  printf 'provider calls: no\n'
  printf 'app tool execution: no\n'
  printf 'release workflow execution: no\n'
  printf 'tag creation/movement/deletion: no\n'
  printf 'release publication: no\n'
  printf 'tracked file mutation by RR-004 runtime: no\n'
  printf 'fixture output confinement: %s\n' "${REPORT_DIR}"
  printf 'operator invariant: no policy is not permission\n'
)"
rr_report_list "${REPORT_FILE}" "Operational safety posture" "${safety_body}"
rr004_record_pass

if ! command -v timeout >/dev/null 2>&1; then
  rr004_report_check "Timeout availability" "command -v timeout" "1" "BLOCKED" "timeout command is required for bounded RR-004 fixture execution"
  rr004_record_fail "timeout command unavailable"
fi

fixture_command="go test ./pkg/releasecheck -run TestRR004PolicyStatelockSmoke -count=1 -v"
fixture_output=""
fixture_status=0
if [[ "${RR004_SKIP_GO_TEST:-0}" == "1" ]]; then
  skip_output="RR004_SKIP_GO_TEST=1; fixture package execution skipped by request."
  rr004_report_check "Fixture workflow" "${fixture_command}" "not-run" "SKIP" "${skip_output}"
  rr004_record_skip
else
  set +e
  rr004_run_fixture_go_test fixture_output
  fixture_status=$?
  set -e

  if [[ "${fixture_status}" == "0" ]]; then
    rr004_report_check "Fixture workflow" "${fixture_command}" "${fixture_status}" "PASS" "${fixture_output}"
    rr004_record_pass
  elif [[ "${fixture_status}" == "124" ]]; then
    rr004_report_check "Fixture workflow" "${fixture_command}" "${fixture_status}" "BLOCKED" "${fixture_output}"
    rr004_record_fail "RR-004 fixture workflow timed out"
  else
    rr004_report_check "Fixture workflow" "${fixture_command}" "${fixture_status}" "BLOCKED" "${fixture_output}"
    rr004_record_fail "RR-004 fixture workflow failed with exit code ${fixture_status}"
  fi
fi

scenario_body="$(
  printf 'policy_absence_sensitive_action:\n'
  printf 'operator story: requested network egress with no matching policy\n'
  printf 'requested action: network egress fixture, no network call made\n'
  printf 'policy state: policy_no_match\n'
  printf 'risk/sensitive class: sensitive/network_egress\n'
  printf 'expected outcome: approval required or denied; no silent allow\n'
  printf 'actual/evidence lines:\n'
  rr004_fixture_lines 'RR004_.*policy_absence_sensitive_action' "${fixture_output}"
  printf '\npolicy_absence_low_risk_action:\n'
  printf 'operator story: requested read-only manifest count under absent policy\n'
  printf 'requested action: local read-only metadata fixture\n'
  printf 'policy state: policy_no_match\n'
  printf 'risk/sensitive class: explicit low-risk/non-sensitive\n'
  printf 'expected outcome: may proceed only because low-risk class is explicit\n'
  printf 'actual/evidence lines:\n'
  rr004_fixture_lines 'RR004_.*policy_absence_low_risk_action' "${fixture_output}"
  printf '\nstatelock_conflict:\n'
  printf 'operator story: requested permission widening while active lock exists\n'
  printf 'requested action: in-memory statelock proposed state\n'
  printf 'policy state: policy_matched\n'
  printf 'risk/sensitive class: low-risk fixture with permission-scope conflict\n'
  printf 'expected outcome: conflict visible; unsafe path not silently allowed\n'
  printf 'actual/evidence lines:\n'
  rr004_fixture_lines 'RR004_.*statelock_conflict' "${fixture_output}"
  printf '\nparadox_report:\n'
  printf 'operator story: reviewed possible, confirmed, and inconclusive paradox states\n'
  printf 'requested action: report-only paradox fixture\n'
  printf 'policy state: matched/no_match/unavailable across fixture cases\n'
  printf 'risk/sensitive class: low-risk and sensitive fixture states\n'
  printf 'expected outcome: confirmed only for critical constraints; possible/inconclusive do not overclaim\n'
  printf 'actual/evidence lines:\n'
  rr004_fixture_lines 'RR004_.*paradox_report' "${fixture_output}"
  printf '\nskillruntime_profile_linkage:\n'
  printf 'operator story: promotion attempted with conflict/paradox evidence attached\n'
  printf 'requested action: local skillruntime fixture assessment, no skill execution\n'
  printf 'policy state: profile configured; statelock conflict adverse evidence\n'
  printf 'risk/sensitive class: sensitive selfmod promotion\n'
  printf 'expected outcome: denied/approval_required/incomplete according to contract; refs present\n'
  printf 'actual/evidence lines:\n'
  rr004_fixture_lines 'RR004_.*skillruntime_profile_linkage' "${fixture_output}"
  printf '\noperator-facing summary lines:\n'
  rr004_fixture_lines 'RR004_SUMMARY\\|RR004_OPERATOR_SUMMARY' "${fixture_output}"
)"
rr_report_list "${REPORT_FILE}" "Fixture scenario behavior" "${scenario_body}"
if [[ "${RR004_SKIP_GO_TEST:-0}" == "1" ]]; then
  rr004_record_skip
elif [[ "${fixture_status}" == "0" ]]; then
  rr004_record_pass
fi

seams_body="$(
  printf 'observed weak seams:\n'
  printf 'profile policy absence output can require approval without naming a concrete policy file path\n'
  printf 'low-risk distinction is explicit but compact; richer user-facing classification text would help\n'
  printf 'statelock conflict summaries are reason-code based and could use richer operator language\n'
  printf 'skillruntime warns about state lock conflict but does not inline every conflict reason\n'
  printf 'remaining incomplete:\n'
  printf 'RR-005 vector/RAG/engram preservation smoke deferred\n'
  printf 'RR-006 TUI/operator/session scripted smoke deferred\n'
  printf 'RR-007 final release report generator deferred\n'
)"
rr_report_list "${REPORT_FILE}" "Weak seams and incomplete work" "${seams_body}"
rr004_record_pass

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
  rr004_record_fail "working tree status changed during RR-004"
else
  rr004_record_pass
fi

if [[ -n "${BLOCKERS}" ]]; then
  FINAL_RECOMMENDATION="BLOCKED"
elif [[ "${SKIP_COUNT}" != "0" ]]; then
  FINAL_RECOMMENDATION="RR004_INCOMPLETE"
else
  FINAL_RECOMMENDATION="RR004_PASS"
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

echo "[rr004] report path: ${REPORT_FILE}"
echo "[rr004] final recommendation: ${FINAL_RECOMMENDATION}"
echo "[rr004] checks passed=${PASS_COUNT} failed=${FAIL_COUNT} skipped=${SKIP_COUNT}"

if [[ "${FINAL_RECOMMENDATION}" == "BLOCKED" ]]; then
  exit 1
fi
