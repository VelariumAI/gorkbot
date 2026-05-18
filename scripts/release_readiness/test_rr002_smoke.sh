#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RR002="${ROOT}/scripts/release_readiness/rr002_cli_smoke.sh"
READINESS="${ROOT}/scripts/release_readiness/readiness.sh"
DOCS="${ROOT}/docs/release/README.md"
COMMON="${ROOT}/scripts/release_readiness/lib/common.sh"

required_files=(
  "${RR002}"
  "${READINESS}"
  "${DOCS}"
  "${COMMON}"
)

for file in "${required_files[@]}"; do
  [[ -f "${file}" ]] || {
    echo "[rr002-smoke] missing ${file}" >&2
    exit 1
  }
done

[[ -x "${RR002}" ]] || {
  echo "[rr002-smoke] rr002_cli_smoke.sh is not executable" >&2
  exit 1
}

bash -n "${RR002}"
bash -n "${READINESS}"

grep -Fq "RR-002 CLI Smoke + Config Matrix" "${RR002}"
grep -Fq ".local/release-readiness/rr002" "${RR002}"
grep -Fq "RR002_PASS" "${RR002}"
grep -Fq "RR002_INCOMPLETE" "${RR002}"
grep -Fq "BLOCKED" "${RR002}"
grep -Fq "go run ./cmd/gorkbot --help" "${RR002}"
grep -Fq "go run ./cmd/gorkweb --help" "${RR002}"
grep -Fq "go test -p=1 ./pkg/profile" "${RR002}"
grep -Fq "rr_print_go_profile" "${RR002}"
grep -Fq "rr_go_platform_profile" "${COMMON}"
grep -Fq "rr_go_test_args" "${COMMON}"
grep -Fq "rr_go_env_prefix" "${COMMON}"
grep -Fq "using existing Go toolchain cache; no local module-cache copy" "${COMMON}"
grep -Fq "GOMAXPROCS=1" "${COMMON}"
grep -Fq "GOMEMLIMIT=1024MiB" "${COMMON}"
grep -Fq "RR-002 CLI Smoke + Config Matrix" "${READINESS}"
grep -Fq "bash scripts/release_readiness/rr002_cli_smoke.sh" "${DOCS}"

if grep -Fq 'GOPATH="${RUN_DIR}/gopath"' "${RR002}"; then
  echo "[rr002-smoke] rr002_cli_smoke.sh still forces a project-local GOPATH" >&2
  exit 1
fi

if grep -Fq 'GOMODCACHE=' "${RR002}"; then
  echo "[rr002-smoke] rr002_cli_smoke.sh still manages GOMODCACHE directly" >&2
  exit 1
fi

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
  if grep -Fq "${pattern}" "${RR002}"; then
    echo "[rr002-smoke] forbidden shell pattern in rr002_cli_smoke.sh: ${pattern}" >&2
    exit 1
  fi
done

tmp_dir="${ROOT}/.local/release-readiness/rr002-smoke-test"
mkdir -p "${tmp_dir}"
tmp_root="${TMPDIR:-/tmp}"
out_file="$(mktemp "${tmp_root%/}/rr002-smoke.XXXXXX")"
trap 'rm -f "$out_file"' EXIT
(
  cd "${tmp_dir}"
  RR002_SKIP_CLI=1 RR002_SKIP_CONFIG_MATRIX=1 bash "${RR002}" >"${out_file}"
)
grep -Fq "[rr002] platform profile:" "${out_file}"
grep -Fq "[rr002] Go safety:" "${out_file}"
grep -Fq "[rr002] report path:" "${out_file}"
grep -Fq "[rr002] final recommendation:" "${out_file}"

echo "[rr002-smoke] OK"
