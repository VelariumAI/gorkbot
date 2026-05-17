#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RR003="${ROOT}/scripts/release_readiness/rr003_var_spine_smoke.sh"
READINESS="${ROOT}/scripts/release_readiness/readiness.sh"
DOCS="${ROOT}/docs/release/README.md"
GO_FIXTURE="${ROOT}/pkg/releasecheck/rr003_var_spine_test.go"

required_files=(
  "${RR003}"
  "${READINESS}"
  "${DOCS}"
  "${GO_FIXTURE}"
)

for file in "${required_files[@]}"; do
  [[ -f "${file}" ]] || {
    echo "[rr003-smoke] missing ${file}" >&2
    exit 1
  }
done

[[ -x "${RR003}" ]] || {
  echo "[rr003-smoke] rr003_var_spine_smoke.sh is not executable" >&2
  exit 1
}

bash -n "${RR003}"
bash -n "${READINESS}"

grep -Fq "RR-003 VAR Spine Fixture Smoke" "${RR003}"
grep -Fq ".local/release-readiness/rr003" "${RR003}"
grep -Fq "RR003_PASS" "${RR003}"
grep -Fq "RR003_INCOMPLETE" "${RR003}"
grep -Fq "BLOCKED" "${RR003}"
grep -Fq "go test ./pkg/releasecheck -run TestRR003VARSpineFixture -count=1 -v" "${RR003}"
grep -Fq "RR-003 VAR Spine Fixture Smoke" "${READINESS}"
grep -Fq "bash scripts/release_readiness/rr003_var_spine_smoke.sh" "${READINESS}"
grep -Fq "RR-003 VAR Spine Fixture Smoke" "${DOCS}"
grep -Fq "bash scripts/release_readiness/rr003_var_spine_smoke.sh" "${DOCS}"
grep -Fq "TestRR003VARSpineFixture" "${GO_FIXTURE}"
grep -Fq "vector_assert_truth" "${GO_FIXTURE}"

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
  if grep -Fq "${pattern}" "${RR003}"; then
    echo "[rr003-smoke] forbidden shell pattern in rr003_var_spine_smoke.sh: ${pattern}" >&2
    exit 1
  fi
done

if grep -Fq "git tag" "${RR003}" && ! grep -Fq "git tag --list" "${RR003}"; then
  echo "[rr003-smoke] unexpected git tag operation in rr003_var_spine_smoke.sh" >&2
  exit 1
fi

tmp_dir="${ROOT}/.local/release-readiness/rr003-smoke-test"
mkdir -p "${tmp_dir}"
tmp_root="${TMPDIR:-/tmp}"
out_file="$(mktemp "${tmp_root%/}/rr003-smoke.XXXXXX")"
trap 'rm -f "$out_file"' EXIT
(
  cd "${tmp_dir}"
  RR003_SKIP_GO_TEST=1 bash "${RR003}" >"${out_file}"
)
grep -Fq "[rr003] report path:" "${out_file}"
grep -Fq "[rr003] final recommendation:" "${out_file}"
grep -Fq "[rr003] checks passed=" "${out_file}"

echo "[rr003-smoke] OK"
