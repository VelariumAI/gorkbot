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

ask_secret() {
  local prompt="$1"
  local answer
  read -r -s -p "${prompt}: " answer
  printf '\n' >&2
  printf '%s' "${answer}"
}

ask_choice() {
  local prompt="$1"
  local def="$2"
  shift 2
  local answer
  while true; do
    read -r -p "${prompt} [${def}]: " answer
    answer="${answer:-$def}"
    for opt in "$@"; do
      if [[ "${answer}" == "${opt}" ]]; then
        printf '%s' "${answer}"
        return 0
      fi
    done
    say "Please choose one of: $*"
  done
}

extract_access_token_from_file() {
  local path="$1"
  [[ -f "${path}" ]] || return 1

  if command -v python3 >/dev/null 2>&1; then
    python3 - "${path}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1])
raw = p.read_text(encoding="utf-8", errors="ignore").strip()
if not raw:
    raise SystemExit(1)
if raw[:1] not in "{[":
    print(raw)
    raise SystemExit(0)
try:
    v = json.loads(raw)
except Exception:
    raise SystemExit(1)

def find(x):
    if isinstance(x, dict):
        for k in ("access_token", "token", "id_token"):
            val = x.get(k)
            if isinstance(val, str) and val.strip():
                return val.strip()
        for val in x.values():
            t = find(val)
            if t:
                return t
    elif isinstance(x, list):
        for val in x:
            t = find(val)
            if t:
                return t
    return ""

tok = find(v)
if tok:
    print(tok)
    raise SystemExit(0)
raise SystemExit(1)
PY
    return $?
  fi

  local raw token
  raw="$(tr -d '\r' < "${path}" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g')"
  [[ -n "${raw}" ]] || return 1
  if [[ "${raw}" != \{* && "${raw}" != \[* ]]; then
    printf '%s\n' "${raw}"
    return 0
  fi

  token="$(printf '%s' "${raw}" | sed -n 's/.*"access_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  [[ -z "${token}" ]] && token="$(printf '%s' "${raw}" | sed -n 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  [[ -z "${token}" ]] && token="$(printf '%s' "${raw}" | sed -n 's/.*"id_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  [[ -n "${token}" ]] || return 1
  printf '%s\n' "${token}"
}

discover_openai_access_token() {
  local cfg_dir token
  cfg_dir="$(dirname "${CONFIG_ENV_FILE}")"

  if [[ -n "${OPENAI_ACCESS_TOKEN:-}" ]]; then
    printf '%s\t%s\n' "${OPENAI_ACCESS_TOKEN}" "env:OPENAI_ACCESS_TOKEN"
    return 0
  fi

  local paths=(
    "${cfg_dir}/openai_auth.json"
    "${cfg_dir}/openai_oauth_token.json"
    "${cfg_dir}/openai_oauth_token.txt"
    "${HOME}/.config/openai/auth.json"
    "${HOME}/.openai/auth.json"
    "${HOME}/.config/codex/auth.json"
    "${HOME}/.codex/auth.json"
  )
  for p in "${paths[@]}"; do
    token="$(extract_access_token_from_file "${p}" 2>/dev/null || true)"
    if [[ -n "${token}" ]]; then
      printf '%s\t%s\n' "${token}" "${p}"
      return 0
    fi
  done
  return 1
}

discover_anthropic_access_token() {
  local cfg_dir token
  cfg_dir="$(dirname "${CONFIG_ENV_FILE}")"

  if [[ -n "${ANTHROPIC_ACCESS_TOKEN:-}" ]]; then
    printf '%s\t%s\n' "${ANTHROPIC_ACCESS_TOKEN}" "env:ANTHROPIC_ACCESS_TOKEN"
    return 0
  fi

  local paths=(
    "${cfg_dir}/anthropic_auth.json"
    "${cfg_dir}/anthropic_oauth_token.json"
    "${cfg_dir}/anthropic_oauth_token.txt"
    "${HOME}/.config/anthropic/auth.json"
    "${HOME}/.anthropic/auth.json"
    "${HOME}/.config/claude/auth.json"
    "${HOME}/.claude/auth.json"
  )
  for p in "${paths[@]}"; do
    token="$(extract_access_token_from_file "${p}" 2>/dev/null || true)"
    if [[ -n "${token}" ]]; then
      printf '%s\t%s\n' "${token}" "${p}"
      return 0
    fi
  done
  return 1
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
OPENAI_ACCESS_TOKEN=
ANTHROPIC_ACCESS_TOKEN=
MINIMAX_API_KEY=
OPENROUTER_API_KEY=
MOONSHOT_API_KEY=
GITHUB_PERSONAL_ACCESS_TOKEN=
BRAVE_API_KEY=
DISCORD_BOT_TOKEN=
TELEGRAM_BOT_TOKEN=
WEBHOOK_SECRET=
GORKBOT_JWT_SECRET=
GORKBOT_PREFER_OAUTH=1
GORKBOT_PREFER_OAUTH_OPENAI=1
GORKBOT_PREFER_OAUTH_ANTHROPIC=1
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
OPENAI_ACCESS_TOKEN=
ANTHROPIC_ACCESS_TOKEN=
MINIMAX_API_KEY=
OPENROUTER_API_KEY=
MOONSHOT_API_KEY=
GITHUB_PERSONAL_ACCESS_TOKEN=
BRAVE_API_KEY=
DISCORD_BOT_TOKEN=
TELEGRAM_BOT_TOKEN=
WEBHOOK_SECRET=
GORKBOT_JWT_SECRET=
GORKBOT_API_USER=
GORKBOT_API_PASSWORD=
GORKBOT_API_ALLOW_INSECURE_LOGIN=false
GORKBOT_PREFER_OAUTH=1
GORKBOT_PREFER_OAUTH_OPENAI=1
GORKBOT_PREFER_OAUTH_ANTHROPIC=1
GORKBOT_MEMORY_ENABLE_SEMANTIC=0
GORKBOT_MEMORY_ENABLE_RERANKER=0
GORKBOT_MEMORY_EMBEDDER=
GORKBOT_MEMORY_OLLAMA_URL=
GORKBOT_MEMORY_OLLAMA_MODEL=
GORKBOT_TOOL_PACKS=
GORKBOT_SAVE_TRAJECTORIES=0
GORKBOT_PROJECT_DIR=
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
    pkg install -y git curl make clang cmake pkg-config
    if command -v go >/dev/null 2>&1; then
      return
    fi
    if pkg install -y golang; then
      return
    fi
    if pkg install -y go; then
      return
    fi
    warn "Failed to install Go via Termux package manager. Install manually: pkg install golang"
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
  if command -v dnf >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      dnf install -y git curl make cmake pkgconf-pkg-config clang golang
    elif command -v sudo >/dev/null 2>&1; then
      sudo dnf install -y git curl make cmake pkgconf-pkg-config clang golang
    else
      warn "dnf found but no sudo/root; skipping auto install."
    fi
    return
  fi
  if command -v yum >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      yum install -y git curl make cmake pkgconfig clang golang
    elif command -v sudo >/dev/null 2>&1; then
      sudo yum install -y git curl make cmake pkgconfig clang golang
    else
      warn "yum found but no sudo/root; skipping auto install."
    fi
    return
  fi
  if command -v pacman >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      pacman -Sy --noconfirm git curl make cmake pkgconf clang go
    elif command -v sudo >/dev/null 2>&1; then
      sudo pacman -Sy --noconfirm git curl make cmake pkgconf clang go
    else
      warn "pacman found but no sudo/root; skipping auto install."
    fi
    return
  fi
  if command -v zypper >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      zypper --non-interactive install git curl make cmake pkg-config clang go
    elif command -v sudo >/dev/null 2>&1; then
      sudo zypper --non-interactive install git curl make cmake pkg-config clang go
    else
      warn "zypper found but no sudo/root; skipping auto install."
    fi
    return
  fi
  if command -v apk >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      apk add --no-cache git curl make cmake pkgconf clang go build-base
    elif command -v sudo >/dev/null 2>&1; then
      sudo apk add --no-cache git curl make cmake pkgconf clang go build-base
    else
      warn "apk found but no sudo/root; skipping auto install."
    fi
    return
  fi
  warn "No supported package manager found for auto dependency install."
}

install_mcp_runtime_deps() {
  if command -v npx >/dev/null 2>&1; then
    say "MCP runtime check: npx already available."
    return
  fi

  say "MCP runtime check: npx not found."
  if ! ask_yes_no "Install Node.js/npm now for npx-based MCP servers?" "y"; then
    warn "Skipping Node.js/npm install. MCP servers using npx may not run yet."
    return
  fi

  if [[ "${PREFIX:-}" == *"com.termux"* ]] && command -v pkg >/dev/null 2>&1; then
    pkg install -y nodejs-lts
    return
  fi
  if command -v apt-get >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      apt-get update && apt-get install -y nodejs npm
    elif command -v sudo >/dev/null 2>&1; then
      sudo apt-get update && sudo apt-get install -y nodejs npm
    else
      warn "apt-get found but no sudo/root; cannot install Node.js/npm automatically."
    fi
    return
  fi
  if command -v brew >/dev/null 2>&1; then
    brew install node || true
    return
  fi
  if command -v dnf >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      dnf install -y nodejs npm
    elif command -v sudo >/dev/null 2>&1; then
      sudo dnf install -y nodejs npm
    else
      warn "dnf found but no sudo/root; cannot install Node.js/npm automatically."
    fi
    return
  fi
  if command -v yum >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      yum install -y nodejs npm
    elif command -v sudo >/dev/null 2>&1; then
      sudo yum install -y nodejs npm
    else
      warn "yum found but no sudo/root; cannot install Node.js/npm automatically."
    fi
    return
  fi
  if command -v pacman >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      pacman -Sy --noconfirm nodejs npm
    elif command -v sudo >/dev/null 2>&1; then
      sudo pacman -Sy --noconfirm nodejs npm
    else
      warn "pacman found but no sudo/root; cannot install Node.js/npm automatically."
    fi
    return
  fi
  if command -v zypper >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      zypper --non-interactive install nodejs18 npm18 || zypper --non-interactive install nodejs npm
    elif command -v sudo >/dev/null 2>&1; then
      sudo zypper --non-interactive install nodejs18 npm18 || sudo zypper --non-interactive install nodejs npm
    else
      warn "zypper found but no sudo/root; cannot install Node.js/npm automatically."
    fi
    return
  fi
  if command -v apk >/dev/null 2>&1; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      apk add --no-cache nodejs npm
    elif command -v sudo >/dev/null 2>&1; then
      sudo apk add --no-cache nodejs npm
    else
      warn "apk found but no sudo/root; cannot install Node.js/npm automatically."
    fi
    return
  fi
  warn "No supported package manager found to install Node.js/npm automatically."
}

write_api_server_security() {
  local target="$1"
  local file_a="${CONFIG_ENV_FILE}"
  local file_b="${PROJECT_ENV_FILE}"
  local api_user api_pass allow_insecure

  api_user="$(ask_input 'GORKBOT_API_USER (for API login, leave blank to skip)' '')"
  if [[ -n "${api_user}" ]]; then
    api_pass="$(ask_secret 'GORKBOT_API_PASSWORD (hidden input)')"
    [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_API_USER" "${api_user}"
    [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_API_PASSWORD" "${api_pass}"
    [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_API_USER" "${api_user}"
    [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_API_PASSWORD" "${api_pass}"
  fi
  ask_yes_no "Allow insecure API login fallback (development only)?" "n" && allow_insecure="true" || allow_insecure="false"
  [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_API_ALLOW_INSECURE_LOGIN" "${allow_insecure}"
  [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_API_ALLOW_INSECURE_LOGIN" "${allow_insecure}"
}

write_memory_config() {
  local target="$1"
  local file_a="${CONFIG_ENV_FILE}"
  local file_b="${PROJECT_ENV_FILE}"
  local semantic reranker embedder ollama_url ollama_model

  ask_yes_no "Enable semantic memory retrieval?" "y" && semantic="1" || semantic="0"
  ask_yes_no "Enable reranker pass for semantic memory?" "y" && reranker="1" || reranker="0"
  embedder="$(ask_input 'Memory embedder backend (simple/ollama/openai/google)' 'simple')"
  ollama_url=""
  ollama_model=""
  if [[ "${embedder}" == "ollama" ]]; then
    ollama_url="$(ask_input 'GORKBOT_MEMORY_OLLAMA_URL' 'http://localhost:11434')"
    ollama_model="$(ask_input 'GORKBOT_MEMORY_OLLAMA_MODEL' 'nomic-embed-text')"
  fi

  [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_MEMORY_ENABLE_SEMANTIC" "${semantic}"
  [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_MEMORY_ENABLE_RERANKER" "${reranker}"
  [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_MEMORY_EMBEDDER" "${embedder}"
  [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_MEMORY_ENABLE_SEMANTIC" "${semantic}"
  [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_MEMORY_ENABLE_RERANKER" "${reranker}"
  [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_MEMORY_EMBEDDER" "${embedder}"

  if [[ -n "${ollama_url}" ]]; then
    [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_MEMORY_OLLAMA_URL" "${ollama_url}"
    [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_MEMORY_OLLAMA_URL" "${ollama_url}"
  fi
  if [[ -n "${ollama_model}" ]]; then
    [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_MEMORY_OLLAMA_MODEL" "${ollama_model}"
    [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_MEMORY_OLLAMA_MODEL" "${ollama_model}"
  fi
}

write_runtime_toggles() {
  local target="$1"
  local file_a="${CONFIG_ENV_FILE}"
  local file_b="${PROJECT_ENV_FILE}"
  local packs save_traj project_dir

  packs="$(ask_input 'GORKBOT_TOOL_PACKS (comma-separated, blank for default)' '')"
  ask_yes_no "Persist trajectories to disk (debug/audit)?" "n" && save_traj="1" || save_traj="0"
  project_dir="$(ask_input 'GORKBOT_PROJECT_DIR override (blank to skip)' '')"

  [[ -n "${packs}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_TOOL_PACKS" "${packs}"
  [[ -n "${packs}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_TOOL_PACKS" "${packs}"
  [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_SAVE_TRAJECTORIES" "${save_traj}"
  [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_SAVE_TRAJECTORIES" "${save_traj}"
  if [[ -n "${project_dir}" ]]; then
    [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PROJECT_DIR" "${project_dir}"
    [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PROJECT_DIR" "${project_dir}"
  fi
}

write_mcp_starter() {
  local mcp_dir="${HOME}/.config/gorkbot/mcp/filesystem"
  local mcp_cfg="${mcp_dir}/config.toml"
  if [[ -f "${mcp_cfg}" ]]; then
    return 0
  fi
  mkdir -p "${mcp_dir}"
  cat > "${mcp_cfg}" <<'EOF'
# Minimal MCP starter entry.
# Install a server package and update command/args for your environment.
name = "filesystem"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
enabled = false
EOF
}

write_google_oauth_client() {
  local client_id client_secret cfg_file
  cfg_file="${HOME}/.config/gorkbot/google_client.json"
  client_id="$(ask_input 'Google OAuth Client ID (Desktop app)' '')"
  if [[ -z "${client_id}" ]]; then
    warn "Google OAuth client ID not provided; skipping OAuth client config."
    return
  fi
  client_secret="$(ask_input 'Google OAuth Client Secret (optional for installed app flow)' '')"
  mkdir -p "$(dirname "${cfg_file}")"
  if [[ -n "${client_secret}" ]]; then
    cat > "${cfg_file}" <<EOF
{
  "client_id": "${client_id}",
  "client_secret": "${client_secret}"
}
EOF
  else
    cat > "${cfg_file}" <<EOF
{
  "client_id": "${client_id}"
}
EOF
  fi
  chmod 600 "${cfg_file}" 2>/dev/null || true
  say "Wrote Google OAuth client config: ${cfg_file}"
  say "Next step: run 'gorkbot', then '/auth notebooklm login' to complete sign-in."
}

write_keys() {
  local target="${1}"
  local file_a="${CONFIG_ENV_FILE}"
  local file_b="${PROJECT_ENV_FILE}"
  local key val gemini_mode discovered_token discovered_source

  [[ "${target}" != "project" ]] && backup_env_file "${file_a}"
  [[ "${target}" != "config" ]] && backup_env_file "${file_b}"

  say ""
  say "Credential setup (leave blank to skip optional values)."
  key="$(ask_input 'XAI_API_KEY (Grok)' '')"; val="${key}"; [[ -n "${val}" ]] && export XAI_API_KEY="${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "XAI_API_KEY" "${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "XAI_API_KEY" "${val}"

  say ""
  say "Google Gemini auth mode:"
  say "  - oauth: OAuth sign-in only"
  say "  - api_key: API key only"
  say "  - hybrid: both (with preference toggle)"
  say "  - skip: configure later"
  gemini_mode="$(ask_choice 'Select Gemini auth mode (oauth/api_key/hybrid/skip)' 'hybrid' oauth api_key hybrid skip)"

  case "${gemini_mode}" in
    oauth)
      [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH" "1"
      [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH" "1"
      write_google_oauth_client
      ;;
    api_key)
      key="$(ask_input 'GEMINI_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export GEMINI_API_KEY="${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GEMINI_API_KEY" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GEMINI_API_KEY" "${val}"
      [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH" "0"
      [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH" "0"
      ;;
    hybrid)
      key="$(ask_input 'GEMINI_API_KEY (fallback when OAuth unavailable)' '')"; val="${key}"; [[ -n "${val}" ]] && export GEMINI_API_KEY="${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GEMINI_API_KEY" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GEMINI_API_KEY" "${val}"
      write_google_oauth_client
      if ask_yes_no "Prefer OAuth over API key when both exist?" "y"; then
        [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH" "1"
        [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH" "1"
      else
        [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH" "0"
        [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH" "0"
      fi
      ;;
    skip)
      say "Skipping Gemini credential setup for now."
      ;;
  esac

  say ""
  say "Anthropic auth mode:"
  say "  - oauth: session token only (Pro/CLI login style)"
  say "  - api_key: API key only"
  say "  - hybrid: both (with preference toggle)"
  say "  - skip: configure later"
  local anthropic_mode
  anthropic_mode="$(ask_choice 'Select Anthropic auth mode (oauth/api_key/hybrid/skip)' 'hybrid' oauth api_key hybrid skip)"
  case "${anthropic_mode}" in
    oauth)
      discovered_token=""
      discovered_source=""
      if read -r discovered_token discovered_source < <(discover_anthropic_access_token 2>/dev/null); then
        if ask_yes_no "Use discovered Anthropic session token from ${discovered_source}?" "y"; then
          val="${discovered_token}"
        else
          key="$(ask_input 'ANTHROPIC_ACCESS_TOKEN' '')"; val="${key}"
        fi
      else
        key="$(ask_input 'ANTHROPIC_ACCESS_TOKEN' '')"; val="${key}"
      fi
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "ANTHROPIC_ACCESS_TOKEN" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "ANTHROPIC_ACCESS_TOKEN" "${val}"
      [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH_ANTHROPIC" "1"
      [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH_ANTHROPIC" "1"
      ;;
    api_key)
      key="$(ask_input 'ANTHROPIC_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export ANTHROPIC_API_KEY="${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "ANTHROPIC_API_KEY" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "ANTHROPIC_API_KEY" "${val}"
      [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH_ANTHROPIC" "0"
      [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH_ANTHROPIC" "0"
      ;;
    hybrid)
      key="$(ask_input 'ANTHROPIC_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export ANTHROPIC_API_KEY="${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "ANTHROPIC_API_KEY" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "ANTHROPIC_API_KEY" "${val}"
      discovered_token=""
      discovered_source=""
      if read -r discovered_token discovered_source < <(discover_anthropic_access_token 2>/dev/null); then
        if ask_yes_no "Use discovered Anthropic session token from ${discovered_source}?" "y"; then
          val="${discovered_token}"
        else
          key="$(ask_input 'ANTHROPIC_ACCESS_TOKEN (fallback/primary token)' '')"; val="${key}"
        fi
      else
        key="$(ask_input 'ANTHROPIC_ACCESS_TOKEN (fallback/primary token)' '')"; val="${key}"
      fi
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "ANTHROPIC_ACCESS_TOKEN" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "ANTHROPIC_ACCESS_TOKEN" "${val}"
      if ask_yes_no "Prefer OAuth/session over API key for Anthropic when both exist?" "y"; then
        [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH_ANTHROPIC" "1"
        [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH_ANTHROPIC" "1"
      else
        [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH_ANTHROPIC" "0"
        [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH_ANTHROPIC" "0"
      fi
      ;;
    skip)
      key="$(ask_input 'ANTHROPIC_API_KEY (optional)' '')"; val="${key}"; [[ -n "${val}" ]] && export ANTHROPIC_API_KEY="${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "ANTHROPIC_API_KEY" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "ANTHROPIC_API_KEY" "${val}"
      say "Skipping Anthropic OAuth/session setup for now."
      ;;
  esac

  say ""
  say "OpenAI auth mode:"
  say "  - oauth: session token only (Codex/Pro login style)"
  say "  - api_key: API key only"
  say "  - hybrid: both (with preference toggle)"
  say "  - skip: configure later"
  local openai_mode
  openai_mode="$(ask_choice 'Select OpenAI auth mode (oauth/api_key/hybrid/skip)' 'hybrid' oauth api_key hybrid skip)"
  case "${openai_mode}" in
    oauth)
      discovered_token=""
      discovered_source=""
      if read -r discovered_token discovered_source < <(discover_openai_access_token 2>/dev/null); then
        if ask_yes_no "Use discovered OpenAI session token from ${discovered_source}?" "y"; then
          val="${discovered_token}"
        else
          key="$(ask_input 'OPENAI_ACCESS_TOKEN' '')"; val="${key}"
        fi
      else
        key="$(ask_input 'OPENAI_ACCESS_TOKEN' '')"; val="${key}"
      fi
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "OPENAI_ACCESS_TOKEN" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "OPENAI_ACCESS_TOKEN" "${val}"
      [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH_OPENAI" "1"
      [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH_OPENAI" "1"
      ;;
    api_key)
      key="$(ask_input 'OPENAI_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export OPENAI_API_KEY="${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "OPENAI_API_KEY" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "OPENAI_API_KEY" "${val}"
      [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH_OPENAI" "0"
      [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH_OPENAI" "0"
      ;;
    hybrid)
      key="$(ask_input 'OPENAI_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export OPENAI_API_KEY="${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "OPENAI_API_KEY" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "OPENAI_API_KEY" "${val}"
      discovered_token=""
      discovered_source=""
      if read -r discovered_token discovered_source < <(discover_openai_access_token 2>/dev/null); then
        if ask_yes_no "Use discovered OpenAI session token from ${discovered_source}?" "y"; then
          val="${discovered_token}"
        else
          key="$(ask_input 'OPENAI_ACCESS_TOKEN (fallback/primary token)' '')"; val="${key}"
        fi
      else
        key="$(ask_input 'OPENAI_ACCESS_TOKEN (fallback/primary token)' '')"; val="${key}"
      fi
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "OPENAI_ACCESS_TOKEN" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "OPENAI_ACCESS_TOKEN" "${val}"
      if ask_yes_no "Prefer OAuth/session over API key for OpenAI when both exist?" "y"; then
        [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH_OPENAI" "1"
        [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH_OPENAI" "1"
      else
        [[ "${target}" != "project" ]] && env_upsert "${file_a}" "GORKBOT_PREFER_OAUTH_OPENAI" "0"
        [[ "${target}" != "config" ]] && env_upsert "${file_b}" "GORKBOT_PREFER_OAUTH_OPENAI" "0"
      fi
      ;;
    skip)
      key="$(ask_input 'OPENAI_API_KEY (optional)' '')"; val="${key}"; [[ -n "${val}" ]] && export OPENAI_API_KEY="${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "OPENAI_API_KEY" "${val}"
      [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "OPENAI_API_KEY" "${val}"
      ;;
  esac

  key="$(ask_input 'MINIMAX_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export MINIMAX_API_KEY="${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "MINIMAX_API_KEY" "${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "MINIMAX_API_KEY" "${val}"
  key="$(ask_input 'OPENROUTER_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export OPENROUTER_API_KEY="${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "OPENROUTER_API_KEY" "${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "OPENROUTER_API_KEY" "${val}"
  key="$(ask_input 'MOONSHOT_API_KEY' '')"; val="${key}"; [[ -n "${val}" ]] && export MOONSHOT_API_KEY="${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "project" ]] && env_upsert "${file_a}" "MOONSHOT_API_KEY" "${val}"
  [[ -n "${val}" ]] && [[ "${target}" != "config" ]] && env_upsert "${file_b}" "MOONSHOT_API_KEY" "${val}"

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
    if ask_yes_no "Configure API server authentication settings?" "y"; then
      write_api_server_security "${env_target}"
    fi
    if ask_yes_no "Configure semantic memory + reranker backend?" "y"; then
      write_memory_config "${env_target}"
    fi
    if ask_yes_no "Configure runtime toggles (tool packs/project/trajectory)?" "n"; then
      write_runtime_toggles "${env_target}"
    fi
    if ask_yes_no "Generate MCP starter config in ~/.config/gorkbot/mcp?" "y"; then
      write_mcp_starter
      install_mcp_runtime_deps
    fi
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
