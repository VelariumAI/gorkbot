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

  set +e
  captured="$("$@" 2>&1)"
  status=$?
  set -e

  printf -v "${__out_var}" '%s' "${captured}"
  return "${status}"
}

rr_run_shell_capture() {
  local __out_var="$1"
  local command_text="$2"
  local captured status

  set +e
  captured="$(bash -c "${command_text}" 2>&1)"
  status=$?
  set -e

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
