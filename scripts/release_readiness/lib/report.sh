#!/usr/bin/env bash

rr_report_begin() {
  local report_file="$1"
  {
    printf '# Gorkbot Release Readiness Report\n\n'
  } > "${report_file}"
}

rr_report_section() {
  local report_file="$1"
  local title="$2"
  local body="$3"
  {
    printf '## %s\n\n' "${title}"
    if [[ -n "${body}" ]]; then
      printf '%s\n\n' "${body}"
    else
      printf '_No output._\n\n'
    fi
  } >> "${report_file}"
}

rr_report_command() {
  local report_file="$1"
  local title="$2"
  local command_text="$3"
  local status="$4"
  local output="$5"
  local state="passed"

  if [[ "${status}" != "0" ]]; then
    state="failed"
  fi

  {
    printf '## %s\n\n' "${title}"
    printf -- '- Command: `%s`\n' "${command_text}"
    printf -- '- Exit code: `%s`\n' "${status}"
    printf -- '- Check state: `%s`\n\n' "${state}"
    printf '```text\n'
    if [[ -n "${output}" ]]; then
      printf '%s\n' "${output}"
    else
      printf '(no output)\n'
    fi
    printf '```\n\n'
  } >> "${report_file}"
}

rr_report_list() {
  local report_file="$1"
  local title="$2"
  local body="$3"
  {
    printf '## %s\n\n' "${title}"
    if [[ -n "${body}" ]]; then
      printf '```text\n%s\n```\n\n' "${body}"
    else
      printf '```text\n(none)\n```\n\n'
    fi
  } >> "${report_file}"
}
