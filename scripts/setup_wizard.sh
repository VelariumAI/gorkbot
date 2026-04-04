#!/usr/bin/env bash
# One-command interactive setup for beginners through power users.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN_DIR="${PROJECT_ROOT}/bin"
INSTALL_DIR="${HOME}/bin"
CONFIG_ENV_FILE="${HOME}/.config/gorkbot/.env"
PROJECT_ENV_FILE="${PROJECT_ROOT}/.env"
STATE_DIR="${HOME}/.config/gorkbot"
STATE_FILE="${STATE_DIR}/setup.profile"

MODE="${1:-interactive}"

say() { printf '%s\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }

ask_yes_no() {
  local prompt="$1"
  local def="${2:-y}"
  local answer
  local suffix="[Y/n]"
  [[ "${def}" == "n" ]] && suffix="[y/N]"
  while true; do
    read -r -p "${prompt} ${suffix}: " answer
    answer="${answer:-$def}"
    case "${answer}" in
      y|Y|yes|YES) return 0 ;;
      n|N|no|NO) return 1 ;;
      *) say "Please answer y or n." ;;
    esac
  done
}

ask_input() {
  local prompt="$1"
  local def="${2:-}"
  local answer
  if [[ -n "${def}" ]]; then
    read -r -p "${prompt} [${def}]: " answer
    answer="${answer:-$def}"
  else
    read -r -p "${prompt}: " answer
  fi
  printf '%s' "${answer}"
}

env_upsert() {
  local file="$1"
  local key="$2"
  local value="$3"
  mkdir -p "$(dirname "${file}")"
  touch "${file}"
  chmod 600 "${file}" 2>/dev/null || true
  local esc="${value//\\/\\\\}"
  esc="${esc//&/\\&}"
  esc="${esc//|/\\|}"
  if grep -q "^${key}=" "${file}"; then
    sed -i "s|^${key}=.*|${key}=${esc}|" "${file}"
  else
    printf '%s=%s\n' "${key}" "${value}" >> "${file}"
  fi
}

backup_env_file() {
  local file="$1"
  if [[ -f "${file}" ]]; then
    cp "${file}" "${file}.bak.$(date +%Y%m%d%H%M%S)"
  fi
}

ensure_blank_canvas_env_templates() {
  mkdir -p "$(dirname "${CONFIG_ENV_FILE}")"
  if [[ ! -f "${CONFIG_ENV_FILE}" ]]; then
    cat > "${CONFIG_ENV_FILE}" <<'EOF'
# Gorkbot runtime secrets/config (local only, never commit)
XAI_API_KEY=
GEMINI_API_KEY=
ANTHROPIC_API_KEY=
OPENAI_API_KEY=
GORKBOT_JWT_SECRET=
EOF
    chmod 600 "${CONFIG_ENV_FILE}" 2>/dev/null || true
  fi

  if [[ ! -f "${PROJECT_ENV_FILE}" ]]; then
    cat > "${PROJECT_ENV_FILE}" <<'EOF'
# Local developer overrides (optional; avoid committing secrets)
XAI_API_KEY=
GEMINI_API_KEY=
ANTHROPIC_API_KEY=
OPENAI_API_KEY=
GORKBOT_JWT_SECRET=
EOF
    chmod 600 "${PROJECT_ENV_FILE}" 2>/dev/null || true
  fi
}

gen_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    date +%s | sha256sum | cut -d' ' -f1
  fi
}

install_deps() {
  say "Installing base dependencies..."
  if [[ "${PREFIX:-}" == *"com.termux"* ]] && command -v pkg >/dev/null 2>&1; then
    pkg install -y git curl make go clang cmake pkg-config
    return
  fi
  if command -v apt-get >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      apt-get update
      apt-get install -y git curl make cmake pkg-config clang golang-go
    elif command -v sudo >/dev/null 2>&1; then
      sudo apt-get update
      sudo apt-get install -y git curl make cmake pkg-config clang golang-go
    else
      warn "apt-get found but no sudo/root; skipping auto install."
    fi
    return
  fi
  if command -v brew >/dev/null 2>&1; then
    brew install git curl make cmake pkg-config go llvm || true
    return
  fi
  warn "No supported package manager found for auto dependency install."
}

write_keys() {
  local target="${1}"
  local file_a="${CONFIG_ENV_FILE}"
  local file_b="${PROJECT_ENV_FILE}"
  local key val

  [[ "${target}" != "project" ]] && backup_env_file "${file_a}"
  [[ "${target}" != "config" ]] && backup_env_file "${file_b}"

  say ""
  say "API key setup (leave blank to skip any key)."
  key="$(ask_input 'XAI_API_KEY (Grok)' '')"; val="${key}"; [[ -n "${val}" ]] && export XAI_API_KEY="${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "XAI_API_KEY" "${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "XAI_API_KEY" "${val}"

  key="$(ask_input 'GEMINI_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export GEMINI_API_KEY="${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GEMINI_API_KEY" "${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GEMINI_API_KEY" "${val}"

  key="$(ask_input 'ANTHROPIC_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export ANTHROPIC_API_KEY="${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "ANTHROPIC_API_KEY" "${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "ANTHROPIC_API_KEY" "${val}"

  key="$(ask_input 'OPENAI_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export OPENAI_API_KEY="${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "OPENAI_API_KEY" "${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "OPENAI_API_KEY" "${val}"

  if ask_yes_no "Configure optional integration keys (GitHub/Brave/Discord/Telegram/Webhook)?" "n"; then
    key="$(ask_input 'GITHUB_PERSONAL_ACCESS_TOKEN' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GITHUB_PERSONAL_ACCESS_TOKEN" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GITHUB_PERSONAL_ACCESS_TOKEN" "${key}"
    key="$(ask_input 'BRAVE_API_KEY' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "BRAVE_API_KEY" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "BRAVE_API_KEY" "${key}"
    key="$(ask_input 'DISCORD_BOT_TOKEN' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "DISCORD_BOT_TOKEN" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "DISCORD_BOT_TOKEN" "${key}"
    key="$(ask_input 'TELEGRAM_BOT_TOKEN' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "TELEGRAM_BOT_TOKEN" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "TELEGRAM_BOT_TOKEN" "${key}"
    key="$(ask_input 'WEBHOOK_SECRET' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "WEBHOOK_SECRET" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "WEBHOOK_SECRET" "${key}"
  fi

  if ask_yes_no "Configure optional OAuth/client integration keys (Google/Slack/Feishu)?" "n"; then
    key="$(ask_input 'GOOGLE_CLIENT_ID' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GOOGLE_CLIENT_ID" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GOOGLE_CLIENT_ID" "${key}"
    key="$(ask_input 'GOOGLE_CLIENT_SECRET' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GOOGLE_CLIENT_SECRET" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GOOGLE_CLIENT_SECRET" "${key}"
    key="$(ask_input 'SLACK_BOT_TOKEN' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "SLACK_BOT_TOKEN" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "SLACK_BOT_TOKEN" "${key}"
    key="$(ask_input 'SLACK_APP_TOKEN' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "SLACK_APP_TOKEN" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "SLACK_APP_TOKEN" "${key}"
    key="$(ask_input 'FEISHU_APP_ID' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "FEISHU_APP_ID" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "FEISHU_APP_ID" "${key}"
    key="$(ask_input 'FEISHU_APP_SECRET' '')"; [[ -n "${key}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "FEISHU_APP_SECRET" "${key}"; [[ -n "${key}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "FEISHU_APP_SECRET" "${key}"
  fi

  if ask_yes_no "Generate/set GORKBOT_JWT_SECRET automatically?" "y"; then
    local jwt
    jwt="$(gen_secret)"
    [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_JWT_SECRET" "${jwt}"
    [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_JWT_SECRET" "${jwt}"
  fi

  if ask_yes_no "Set provider/model overrides (GORKBOT_PRIMARY etc.)?" "n"; then
    local primary consultant pmodel cmodel
    primary="$(ask_input 'GORKBOT_PRIMARY provider (xai/google/anthropic/openai/minimax)' 'xai')"
    consultant="$(ask_input 'GORKBOT_CONSULTANT provider' 'google')"
    pmodel="$(ask_input 'GORKBOT_PRIMARY_MODEL (optional)' '')"
    cmodel="$(ask_input 'GORKBOT_CONSULTANT_MODEL (optional)' '')"
    [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PRIMARY" "${primary}"
    [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_CONSULTANT" "${consultant}"
    [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PRIMARY" "${primary}"
    [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_CONSULTANT" "${consultant}"
    [[ -n "${pmodel}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PRIMARY_MODEL" "${pmodel}"
    [[ -n "${pmodel}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PRIMARY_MODEL" "${pmodel}"
    [[ -n "${cmodel}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_CONSULTANT_MODEL" "${cmodel}"
    [[ -n "${cmodel}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_CONSULTANT_MODEL" "${cmodel}"
  fi
}

run_setup() {
  local install_base="$1"
  local setup_keys="$2"
  local enable_llm="$3"
  local dl_model="$4"
  local build_web="$5"
  local do_install="$6"
  local add_path="$7"
  local run_checks="$8"

  if [[ "${install_base}" == "1" ]]; then
    install_deps
  fi

  ensure_blank_canvas_env_templates

  if [[ "${setup_keys}" == "1" ]]; then
    local env_target
    env_target="$(ask_input 'Store API keys in (config/project/both)' 'config')"
    case "${env_target}" in
      config|project|both) ;;
      *) env_target="both" ;;
    esac
    write_keys "${env_target}"
  fi

  if [[ "${enable_llm}" == "1" ]]; then
    say "Bootstrapping native LLM bridge/toolchain..."
    (cd "${PROJECT_ROOT}" && AUTO_INSTALL_DEPS=1 DOWNLOAD_MODEL="${dl_model}" bash scripts/bootstrap_native_llm.sh)
    (cd "${PROJECT_ROOT}" && make build-llm)
  else
    (cd "${PROJECT_ROOT}" && make build)
  fi

  if [[ "${build_web}" == "1" ]]; then
    (cd "${PROJECT_ROOT}" && make build-web)
  fi

  if [[ "${do_install}" == "1" ]]; then
    mkdir -p "${INSTALL_DIR}"
    if [[ -f "${BIN_DIR}/gorkbot" ]]; then
      install -m 755 "${BIN_DIR}/gorkbot" "${INSTALL_DIR}/gorkbot"
    fi
    if [[ -f "${BIN_DIR}/gorkweb" ]]; then
      install -m 755 "${BIN_DIR}/gorkweb" "${INSTALL_DIR}/gorkweb"
    fi
    bash "${PROJECT_ROOT}/scripts/write_launcher.sh" "${INSTALL_DIR}" "gorkbot" "gork"
  fi

  if [[ "${add_path}" == "1" ]]; then
    local rc
    rc="${HOME}/.bashrc"
    if [[ "${SHELL:-}" == */zsh ]]; then
      rc="${HOME}/.zshrc"
    fi
    if ! grep -q 'export PATH="$HOME/bin:$PATH"' "${rc}" 2>/dev/null; then
      printf '\nexport PATH="$HOME/bin:$PATH"\n' >> "${rc}"
      say "Added ~/bin PATH export to ${rc}"
    fi
  fi

  if [[ "${run_checks}" == "1" ]]; then
    (cd "${PROJECT_ROOT}" && go test ./... -count=1 -timeout=180s)
  fi

  mkdir -p "${STATE_DIR}"
  cat > "${STATE_FILE}" <<EOF
INSTALL_BASE_DEPS=${install_base}
SETUP_KEYS=${setup_keys}
ENABLE_LLM=${enable_llm}
DOWNLOAD_MODEL=${dl_model}
BUILD_WEB=${build_web}
INSTALL_GLOBAL=${do_install}
ADD_PATH=${add_path}
RUN_CHECKS=${run_checks}
LAST_RUN_UTC=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF
}

say "Gorkbot One-Command Setup Wizard"
say "Project: ${PROJECT_ROOT}"
say ""

if [[ "${MODE}" == "--auto" ]]; then
  run_setup 1 0 0 0 1 1 1 0
  say ""
  say "Setup complete (auto mode)."
  exit 0
fi

say "Select setup mode:"
say "  1) Auto Default (recommended for beginners)"
say "  2) Guided Standard"
say "  3) Guided Pro (power-user full setup)"
choice="$(ask_input 'Enter choice' '1')"

case "${choice}" in
  1)
    run_setup 1 1 0 0 1 1 1 0
    ;;
  2)
    install_base=0; setup_keys=1; enable_llm=0; dl_model=0; build_web=1; do_install=1; add_path=1; run_checks=0
    ask_yes_no "Auto-install base dependencies?" "y" && install_base=1
    ask_yes_no "Configure API keys now?" "y" || setup_keys=0
    ask_yes_no "Enable/build native LLM bridge (llamacpp)?" "n" && enable_llm=1
    if [[ "${enable_llm}" == "1" ]]; then
      ask_yes_no "Download semantic ranker model (nomic)?" "y" && dl_model=1 || dl_model=0
    fi
    ask_yes_no "Build gorkweb too?" "y" && build_web=1 || build_web=0
    ask_yes_no "Install binaries to ~/bin and create 'gork' launcher?" "y" && do_install=1 || do_install=0
    ask_yes_no "Add ~/bin to PATH in shell rc?" "y" && add_path=1 || add_path=0
    ask_yes_no "Run full test validation now?" "n" && run_checks=1 || run_checks=0
    run_setup "${install_base}" "${setup_keys}" "${enable_llm}" "${dl_model}" "${build_web}" "${do_install}" "${add_path}" "${run_checks}"
    ;;
  3)
    install_base=1; setup_keys=1; enable_llm=1; dl_model=1; build_web=1; do_install=1; add_path=1; run_checks=0
    ask_yes_no "Auto-install base dependencies?" "y" && install_base=1 || install_base=0
    ask_yes_no "Configure API keys now?" "y" || setup_keys=0
    ask_yes_no "Enable/build native LLM bridge (llamacpp)?" "y" && enable_llm=1 || enable_llm=0
    if [[ "${enable_llm}" == "1" ]]; then
      ask_yes_no "Download semantic ranker model (nomic)?" "y" && dl_model=1 || dl_model=0
    else
      dl_model=0
    fi
    ask_yes_no "Build gorkweb too?" "y" && build_web=1 || build_web=0
    ask_yes_no "Install binaries to ~/bin and create 'gork' launcher?" "y" && do_install=1 || do_install=0
    ask_yes_no "Add ~/bin to PATH in shell rc?" "y" && add_path=1 || add_path=0
    ask_yes_no "Run full test validation now?" "n" && run_checks=1 || run_checks=0
    run_setup "${install_base}" "${setup_keys}" "${enable_llm}" "${dl_model}" "${build_web}" "${do_install}" "${add_path}" "${run_checks}"
    ;;
  *)
    warn "Invalid choice, using Auto Default."
    run_setup 1 1 0 0 1 1 1 0
    ;;
esac

say ""
say "Setup complete."
say "Next: run 'gork' (if installed) or './bin/gorkbot'."
