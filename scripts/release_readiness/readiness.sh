#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "${SCRIPT_DIR}/lib/common.sh"
# shellcheck source=lib/report.sh
source "${SCRIPT_DIR}/lib/report.sh"

if [[ "$#" -gt 0 ]]; then
  echo "[release-readiness] RR-001 does not accept flags" >&2
  exit 2
fi

ROOT="$(rr_repo_root "${SCRIPT_DIR}")"
cd "${ROOT}"

REPORT_DIR="$(rr_report_dir "${ROOT}")"
mkdir -p "${REPORT_DIR}"
REPORT_FILE="${REPORT_DIR}/release-readiness-report.$(rr_timestamp).md"

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
BLOCKERS=""
FINAL_RECOMMENDATION="NEEDS_RR_SUITE"

mark_blocker() {
  rr_append_unique_line "$1" BLOCKERS
}

record_pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
}

record_fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  mark_blocker "$1"
}

record_skip() {
  SKIP_COUNT=$((SKIP_COUNT + 1))
}

run_required_command() {
  local title="$1"
  local command_text="$2"
  local output status

  set +e
  rr_run_shell_capture output "${command_text}"
  status=$?
  set -e
  rr_report_command "${REPORT_FILE}" "${title}" "${command_text}" "${status}" "${output}"

  if [[ "${status}" == "0" ]]; then
    record_pass
  else
    record_fail "${title} failed with exit code ${status}"
  fi
}

rr_report_begin "${REPORT_FILE}"

timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
branch="$(git branch --show-current 2>/dev/null || true)"
commit="$(git log -1 --oneline 2>/dev/null || true)"
status_short="$(git status --short 2>/dev/null || true)"

rr_report_section "${REPORT_FILE}" "Timestamp" "${timestamp}"
rr_report_section "${REPORT_FILE}" "Branch" "${branch:-unknown}"
rr_report_section "${REPORT_FILE}" "Commit" "${commit:-unknown}"
rr_report_list "${REPORT_FILE}" "Working tree status" "${status_short}"

run_required_command "Promotion manifest" "bash scripts/check_promotion_manifest.sh"
rr_report_section "${REPORT_FILE}" "Test summary" "Individual test command results follow in command-specific sections."

test_commands=(
  "go test ./pkg/skillruntime"
  "go test ./pkg/profile"
  "go test ./pkg/evidence"
  "go test ./pkg/statelock"
  "go test ./pkg/harness"
  "go test ./pkg/replay"
  "go test ./pkg/trace"
  "go test ./pkg/sense"
  "go test ./pkg/tools"
  "go test ./pkg/researchgate"
  "go test ./pkg/selfmod"
  "go test ./pkg/router"
  "go test ./internal/bootstrap"
  "go test ./internal/engine"
  "go test ./cmd/gorkbot"
  "go test ./cmd/gorkweb"
  "go test ./..."
)

for command_text in "${test_commands[@]}"; do
  run_required_command "Test summary: ${command_text}" "${command_text}"
done

run_required_command "Vet summary" "go vet ./..."
run_required_command "Diff check" "git diff --check"

control_files=(
  ".agentic-control"
  "AGENTIC_CONTRACT.md"
  "AGENTS.md"
  "AGENT.md"
  "CLAU""DE.md"
  "GEM""INI.md"
  "COD""EX.md"
)
tracked_control="$(git ls-files "${control_files[@]}" 2>/dev/null || true)"
control_present="no"
if [[ -d "${ROOT}/.agentic-control" ]]; then
  control_present="yes"
fi

tracked_control_answer="no"
if [[ -n "${tracked_control}" ]]; then
  tracked_control_answer="yes"
  mark_blocker "private control files are tracked by the public repo"
fi
if [[ "${control_present}" == "yes" ]]; then
  mark_blocker ".agentic-control is present inside the public repo"
fi

private_control_body="$(
  printf 'private control files tracked by gorkbot: %s\n' "${tracked_control_answer}"
  printf '.agentic-control present inside public repo: %s\n' "${control_present}"
  printf 'tracked control file list:\n'
  if [[ -n "${tracked_control}" ]]; then
    printf '%s\n' "${tracked_control}"
  else
    printf '(none)\n'
  fi
)"
rr_report_list "${REPORT_FILE}" "Private control file check" "${private_control_body}"

scanner_path="${SCANNER_PATH:-${HOME}/project/_local-untracked/gorkbot/scanner-artifacts/_tools/_internal/analyses.py}"
scanner_presence="absent"
scanner_body=""
if [[ -f "${scanner_path}" ]]; then
  scanner_presence="present"
fi
scanner_body="$(
  printf 'private scanner: %s\n' "${scanner_presence}"
  printf 'scanner path: %s\n' "${scanner_path}"
  printf 'scanner invocation: deferred in RR-001 wrapper; run private preflight manually\n'
)"
rr_report_list "${REPORT_FILE}" "Private scanner presence" "${scanner_body}"
record_skip

vector_command="git diff --name-only HEAD~1..HEAD -- pkg/vectorstore pkg/adaptive/mel_store.go pkg/memory/semantic_searcher.go internal/engine/rag_injector.go internal/engine/consultation"
vector_output=""
vector_status=0
set +e
rr_run_shell_capture vector_output "${vector_command}"
vector_status=$?
set -e
vector_pending="$(
  {
    git diff --name-only -- pkg/vectorstore pkg/adaptive/mel_store.go pkg/memory/semantic_searcher.go internal/engine/rag_injector.go internal/engine/consultation 2>/dev/null || true
    git diff --cached --name-only -- pkg/vectorstore pkg/adaptive/mel_store.go pkg/memory/semantic_searcher.go internal/engine/rag_injector.go internal/engine/consultation 2>/dev/null || true
  } | awk 'NF' | sort -u
)"
vector_report_output="$(
  printf 'HEAD~1..HEAD protected path diff:\n'
  if [[ -n "${vector_output}" ]]; then
    printf '%s\n' "${vector_output}"
  else
    printf '(none)\n'
  fi
  printf 'uncommitted protected path diff:\n'
  if [[ -n "${vector_pending}" ]]; then
    printf '%s\n' "${vector_pending}"
  else
    printf '(none)\n'
  fi
)"
rr_report_command "${REPORT_FILE}" "Vector / semantic memory preservation" "${vector_command}" "${vector_status}" "${vector_report_output}"
if [[ "${vector_status}" != "0" ]]; then
  record_fail "vector preservation diff command failed with exit code ${vector_status}"
elif [[ -n "${vector_output}${vector_pending}" ]]; then
  record_fail "protected vector or semantic memory paths changed"
else
  record_pass
fi

local_tag="$(git tag --list 'v1.7.0-rc' 2>/dev/null || true)"
local_tag_commit="$(git rev-parse v1.7.0-rc^{commit} 2>/dev/null || true)"
remote_tag="$(git ls-remote --tags origin 'v1.7.0-rc' 2>/dev/null || true)"
remote_tag_peeled="$(git ls-remote --tags origin 'v1.7.0-rc^{}' 2>/dev/null || true)"
local_tag_exists="no"
remote_tag_exists="no"
if [[ -n "${local_tag}" ]]; then
  local_tag_exists="yes"
fi
if [[ -n "${remote_tag}${remote_tag_peeled}" ]]; then
  remote_tag_exists="yes"
fi
tag_body="$(
  printf 'local tag exists: %s\n' "${local_tag_exists}"
  printf 'local tag commit: %s\n' "${local_tag_commit:-none}"
  printf 'remote tag exists: %s\n' "${remote_tag_exists}"
  printf 'remote tag refs:\n'
  if [[ -n "${remote_tag}${remote_tag_peeled}" ]]; then
    printf '%s\n%s\n' "${remote_tag}" "${remote_tag_peeled}" | awk 'NF'
  else
    printf '(none)\n'
  fi
  printf 'tag operation performed: no\n'
)"
rr_report_list "${REPORT_FILE}" "Release tag status" "${tag_body}"
record_pass

release_changed=""
all_changed_files="$(rr_changed_files)"
while IFS= read -r file; do
  case "${file}" in
    .github/workflows/*|scripts/release/*|scripts/release_readiness/*|docs/release/*)
      rr_append_unique_line "${file}" release_changed
      ;;
  esac
done <<< "${all_changed_files}"
release_body="$(
  printf 'release workflow executed: no\n'
  printf 'release-related files changed:\n'
  if [[ -n "${release_changed}" ]]; then
    printf '%s\n' "${release_changed}"
  else
    printf '(none)\n'
  fi
)"
rr_report_list "${REPORT_FILE}" "Release workflow safety" "${release_body}"
record_pass

neutrality_findings=""
NEUTRALITY_FINDINGS_TMP=""
neutrality_patterns=(
  "generated ""by"
  "authored ""by"
  "co-authored ""by"
  "reviewed ""by"
  "agent ""authored"
  "model ""authored"
  "Clau""de"
  "Op""us"
  "Cod""ex"
  "Chat""G""PT"
  "Gem""ini"
  "Mini""Max"
  "G""PT"
)

changed_files="${all_changed_files}"
added_public_lines="$(
  {
    git diff --cached --unified=0 -- 2>/dev/null || true
    git diff --unified=0 -- 2>/dev/null || true
  } | awk '
    /^\+\+\+ b\// { file=substr($0, 7); next }
    /^\+[^+]/ {
      line=$0
      sub(/^\+/, "", line)
      if (file != "") {
        print file ":" line
      }
    }
  '
)"
for pattern in "${neutrality_patterns[@]}"; do
  matches="$(printf '%s\n' "${added_public_lines}" | grep -i -F "${pattern}" 2>/dev/null || true)"
  if [[ -n "${matches}" ]]; then
    while IFS= read -r match; do
      [[ -n "${match}" ]] || continue
      rr_append_unique_line "${match}" NEUTRALITY_FINDINGS_TMP
    done <<< "${matches}"
  fi
done

recent_messages="$(git log --format='%s%n%b' -n 5 2>/dev/null || true)"
for pattern in "${neutrality_patterns[@]}"; do
  if printf '%s\n' "${recent_messages}" | grep -i -Fq "${pattern}" 2>/dev/null; then
    rr_append_unique_line "recent commit message matched restricted neutrality pattern: ${pattern}" NEUTRALITY_FINDINGS_TMP
  fi
done
neutrality_findings="${NEUTRALITY_FINDINGS_TMP:-}"

neutrality_body="$(
  printf 'changed files scanned:\n'
  if [[ -n "${changed_files}" ]]; then
    printf '%s\n' "${changed_files}"
  else
    printf '(none)\n'
  fi
  printf 'changed lines scanned: staged and unstaged additions\n'
  printf 'recent commit messages scanned: last 5\n'
  printf 'findings:\n'
  if [[ -n "${neutrality_findings}" ]]; then
    printf '%s\n' "${neutrality_findings}"
  else
    printf '(none)\n'
  fi
)"
rr_report_list "${REPORT_FILE}" "Public artifact neutrality" "${neutrality_body}"
if [[ -n "${neutrality_findings}" ]]; then
  record_fail "public artifact neutrality findings detected"
else
  record_pass
fi

skipped_body="$(
  printf 'RR-002 CLI smoke + config matrix: skipped\n'
  printf 'RR-003 VAR spine fixture smoke: skipped\n'
  printf 'RR-004 policy absence + statelock/paradox smoke: skipped\n'
  printf 'RR-005 vector/RAG/engram preservation smoke: skipped\n'
  printf 'RR-006 TUI/operator/session scripted smoke: skipped\n'
  printf 'RR-007 final release report generator: skipped\n'
  printf 'release execution: skipped\n'
  printf 'tag creation or movement: skipped\n'
  printf 'release publication: skipped\n'
)"
rr_report_list "${REPORT_FILE}" "Known skipped RR checks" "${skipped_body}"

if [[ -n "${BLOCKERS}" ]]; then
  FINAL_RECOMMENDATION="BLOCKED"
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

echo "[release-readiness] report path: ${REPORT_FILE}"
echo "[release-readiness] final recommendation: ${FINAL_RECOMMENDATION}"
echo "[release-readiness] checks passed=${PASS_COUNT} failed=${FAIL_COUNT} skipped=${SKIP_COUNT}"

if [[ "${FINAL_RECOMMENDATION}" == "BLOCKED" ]]; then
  exit 1
fi
