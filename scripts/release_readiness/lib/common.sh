#!/usr/bin/env bash

rr_timestamp() {
  date -u +%Y%m%dT%H%M%SZ
}

rr_repo_root() {
  local start_dir="$1"
  local root
  root="$(git -C "${start_dir}" rev-parse --show-toplevel 2>/dev/null)" || return 1
  printf '%s\n' "${root}"
}

rr_report_dir() {
  local root="$1"
  printf '%s\n' "${READINESS_REPORT_DIR:-${root}/.local/release-readiness/reports}"
}

rr_bool() {
  if [[ "$1" == "0" ]]; then
    printf 'yes\n'
  else
    printf 'no\n'
  fi
}

rr_run_capture() {
  local __out_var="$1"
  shift
  local captured status
  local had_errexit=0

  if [[ "$-" == *e* ]]; then
    had_errexit=1
  fi

  set +e
  captured="$("$@" 2>&1)"
  status=$?
  if [[ "${had_errexit}" == "1" ]]; then
    set -e
  else
    set +e
  fi

  printf -v "${__out_var}" '%s' "${captured}"
  return "${status}"
}

rr_run_shell_capture() {
  local __out_var="$1"
  local command_text="$2"
  local captured status
  local had_errexit=0

  if [[ "$-" == *e* ]]; then
    had_errexit=1
  fi

  set +e
  captured="$(bash -c "${command_text}" 2>&1)"
  status=$?
  if [[ "${had_errexit}" == "1" ]]; then
    set -e
  else
    set +e
  fi

  printf -v "${__out_var}" '%s' "${captured}"
  return "${status}"
}

rr_append_unique_line() {
  local line="$1"
  local __list_var="$2"
  local current
  current="${!__list_var}"
  if ! printf '%s\n' "${current}" | grep -Fxq "${line}" 2>/dev/null; then
    if [[ -n "${current}" ]]; then
      printf -v "${__list_var}" '%s\n%s' "${current}" "${line}"
    else
      printf -v "${__list_var}" '%s' "${line}"
    fi
  fi
}

rr_release_base_ref() {
  local __out_var="$1"
  local base_ref=""

  if git show-ref --verify --quiet refs/heads/main 2>/dev/null; then
    base_ref="main"
  elif git show-ref --verify --quiet refs/remotes/origin/main 2>/dev/null; then
    base_ref="origin/main"
  else
    return 1
  fi

  printf -v "${__out_var}" '%s' "${base_ref}"
}

rr_release_merge_base() {
  local __out_var="$1"
  local resolved_base_ref=""
  local merge_base=""

  rr_release_base_ref resolved_base_ref || return 1
  merge_base="$(git merge-base "${resolved_base_ref}" HEAD 2>/dev/null)" || return 1
  printf -v "${__out_var}" '%s' "${merge_base}"
}

rr_release_branch_range() {
  local __out_var="$1"
  local resolved_base_ref=""

  rr_release_base_ref resolved_base_ref || return 1
  printf -v "${__out_var}" '%s' "${resolved_base_ref}...HEAD"
}

rr_changed_files() {
  local branch_range=""

  {
    if rr_release_branch_range branch_range; then
      git diff --name-only "${branch_range}" 2>/dev/null || true
    fi
    git diff --name-only 2>/dev/null || true
    git diff --cached --name-only 2>/dev/null || true
  } | awk 'NF' | sort -u
}

rr_is_termux_android() {
  local uname_o=""
  uname_o="$(uname -o 2>/dev/null || true)"

  if [[ -n "${TERMUX_VERSION:-}" ]]; then
    return 0
  fi
  if [[ "${PREFIX:-}" == */com.termux/files/usr ]]; then
    return 0
  fi
  if [[ "${uname_o}" == "Android" ]]; then
    return 0
  fi
  if [[ "$(go env GOOS 2>/dev/null || true)" == "android" ]]; then
    return 0
  fi

  return 1
}

rr_go_platform_value() {
  local key="$1"
  local value=""

  value="$(go env "${key}" 2>/dev/null || true)"
  if [[ -n "${value}" ]]; then
    printf '%s\n' "${value}"
  else
    printf 'unknown\n'
  fi
}

rr_go_platform_profile() {
  if rr_is_termux_android; then
    printf 'termux/android low-resource\n'
  else
    printf 'local readonly-modcache\n'
  fi
}

rr_go_cache_mode() {
  if rr_is_termux_android; then
    printf 'using existing Go toolchain cache; no local module-cache copy\n'
  else
    printf 'using project-local Go build cache and temp only; no local module-cache copy\n'
  fi
}

rr_go_test_args() {
  printf '%s\n' '-p=1'
}

rr_go_safety_summary() {
  printf '%s\n' 'GOPROXY=off GOSUMDB=off GOFLAGS=-mod=readonly GOMAXPROCS=1 -p=1'
}

rr_go_env_prefix() {
  local run_dir="$1"

  if rr_is_termux_android; then
    RR_GO_ENV_PREFIX=(
      env
      -u GOCACHE
      -u GOPATH
      -u GOMODCACHE
      TERM=dumb
      TMPDIR="${run_dir}/tmp"
      GOTMPDIR="${run_dir}/tmp"
      GOPROXY=off
      GOSUMDB=off
      GOFLAGS=-mod=readonly
      GOMAXPROCS=1
      GOMEMLIMIT=1024MiB
      HOME="${HOME:-${run_dir}/home}"
      PATH="${PATH}"
    )
  else
    RR_GO_ENV_PREFIX=(
      env -i
      HOME="${run_dir}/home"
      PATH="${PATH}"
      TERM=dumb
      TMPDIR="${run_dir}/tmp"
      GOTMPDIR="${run_dir}/tmp"
      GOCACHE="${run_dir}/gocache"
      GOPROXY=off
      GOSUMDB=off
      GOFLAGS=-mod=readonly
      GOMAXPROCS=1
      GOMEMLIMIT=1024MiB
    )
  fi
}

rr_print_go_profile() {
  local prefix="$1"

  printf '[%s] platform profile: %s\n' "${prefix}" "$(rr_go_platform_profile)"
  printf '[%s] go platform: GOOS=%s GOARCH=%s\n' "${prefix}" "$(rr_go_platform_value GOOS)" "$(rr_go_platform_value GOARCH)"
  printf '[%s] %s\n' "${prefix}" "$(rr_go_cache_mode)"
  printf '[%s] Go safety: %s\n' "${prefix}" "$(rr_go_safety_summary)"
}

rr_run_go_capture() {
  local __out_var="$1"
  local prefix="$2"
  local timeout_seconds="$3"
  local run_dir="$4"
  shift 4
  local captured status elapsed
  local had_errexit=0

  if [[ "$-" == *e* ]]; then
    had_errexit=1
  fi

  rr_go_env_prefix "${run_dir}"

  printf '[%s] go command: %s\n' "${prefix}" "$*"
  elapsed="${SECONDS}"
  set +e
  captured="$("${RR_GO_ENV_PREFIX[@]}" timeout "${timeout_seconds}" "$@" 2>&1)"
  status=$?
  if [[ "${had_errexit}" == "1" ]]; then
    set -e
  else
    set +e
  fi
  elapsed=$((SECONDS - elapsed))
  printf '[%s] go command elapsed: %ss\n' "${prefix}" "${elapsed}"

  printf -v "${__out_var}" '%s' "${captured}"
  return "${status}"
}
