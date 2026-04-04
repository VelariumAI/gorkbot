#!/usr/bin/env bash
# bootstrap_native_llm.sh
# Fully autonomous native LLM bootstrap:
# - Installs required toolchain packages when possible
# - Builds llama.cpp static libraries
# - Builds internal CGO bridge static library
# - Optionally downloads local embedding model
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LLAMA_ROOT="${PROJECT_ROOT}/ext/llama.cpp"
LLAMA_BUILD_DIR="${LLAMA_ROOT}/build"
LLM_DIR="${PROJECT_ROOT}/internal/llm"
MODEL_CACHE_DIR="${HOME}/.cache/llama.cpp"
MODEL_FILE="${MODEL_CACHE_DIR}/nomic-embed-text-v1.5.Q4_K_M.gguf"
MODEL_URL="${MODEL_URL:-https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q4_K_M.gguf}"

AUTO_INSTALL_DEPS="${AUTO_INSTALL_DEPS:-1}"
DOWNLOAD_MODEL="${DOWNLOAD_MODEL:-1}"
LLM_WITH_VULKAN="${LLM_WITH_VULKAN:-0}"

log() { printf '%s\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

in_termux() {
  [[ "${PREFIX:-}" == *"com.termux"* ]]
}

ensure_cmd() {
  local cmd="$1"
  local pkg="$2"
  if command -v "$cmd" >/dev/null 2>&1; then
    return 0
  fi
  if [[ "${AUTO_INSTALL_DEPS}" != "1" ]]; then
    die "missing required command '${cmd}' (install package '${pkg}')"
  fi
  install_pkg "${pkg}"
  command -v "$cmd" >/dev/null 2>&1 || die "required command '${cmd}' still missing after install attempt"
}

install_pkg() {
  local pkg="$1"
  if in_termux && command -v pkg >/dev/null 2>&1; then
    log "Installing '${pkg}' via termux pkg..."
    pkg install -y "${pkg}"
    return 0
  fi

  if command -v apt-get >/dev/null 2>&1; then
    local apt_pkg="${pkg}"
    case "${pkg}" in
      clang) apt_pkg="clang" ;;
      cmake) apt_pkg="cmake" ;;
      ninja) apt_pkg="ninja-build" ;;
      make) apt_pkg="make" ;;
      curl) apt_pkg="curl" ;;
      git) apt_pkg="git" ;;
      pkg-config) apt_pkg="pkg-config" ;;
    esac
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      apt-get update
      apt-get install -y "${apt_pkg}"
    elif command -v sudo >/dev/null 2>&1; then
      sudo apt-get update
      sudo apt-get install -y "${apt_pkg}"
    else
      die "cannot install '${apt_pkg}' automatically (apt-get requires root/sudo)"
    fi
    return 0
  fi

  if command -v brew >/dev/null 2>&1; then
    log "Installing '${pkg}' via Homebrew..."
    brew install "${pkg}" || true
    return 0
  fi

  die "no supported package manager found for auto-installing '${pkg}'"
}

ensure_llama_checkout() {
  if [[ -d "${LLAMA_ROOT}" ]]; then
    return 0
  fi
  die "missing ${LLAMA_ROOT}. Initialize submodule first: git submodule update --init --recursive ext/llama.cpp"
}

canonicalize_llama_libs() {
  local build_dir="$1"
  local -a libs=("libllama.a" "libggml.a" "libggml-cpu.a" "libggml-base.a")
  local src
  for lib in "${libs[@]}"; do
    if [[ -f "${build_dir}/${lib}" ]]; then
      continue
    fi
    src="$(find "${build_dir}" -type f -name "${lib}" | head -n 1 || true)"
    if [[ -n "${src}" ]]; then
      ln -sfn "${src}" "${build_dir}/${lib}"
    fi
  done
}

build_llama_cpp() {
  ensure_llama_checkout
  mkdir -p "${LLAMA_BUILD_DIR}"

  local generator_opts=()
  local configured_generator=""
  if [[ -f "${LLAMA_BUILD_DIR}/CMakeCache.txt" ]]; then
    configured_generator="$(grep -E '^CMAKE_GENERATOR:INTERNAL=' "${LLAMA_BUILD_DIR}/CMakeCache.txt" | sed 's/^CMAKE_GENERATOR:INTERNAL=//')"
  fi

  if [[ -n "${configured_generator}" ]]; then
    generator_opts=(-G "${configured_generator}")
  elif command -v ninja >/dev/null 2>&1; then
    generator_opts=(-G Ninja)
  fi

  local vulkan_flag="OFF"
  if [[ "${LLM_WITH_VULKAN}" == "1" ]]; then
    vulkan_flag="ON"
  fi

  log "Configuring llama.cpp build..."
  cmake -S "${LLAMA_ROOT}" -B "${LLAMA_BUILD_DIR}" \
    "${generator_opts[@]}" \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=OFF \
    -DLLAMA_BUILD_TESTS=OFF \
    -DLLAMA_BUILD_EXAMPLES=OFF \
    -DGGML_OPENMP=OFF \
    -DGGML_VULKAN="${vulkan_flag}"

  log "Building llama.cpp static libraries..."
  cmake --build "${LLAMA_BUILD_DIR}" --parallel

  canonicalize_llama_libs "${LLAMA_BUILD_DIR}"
  [[ -f "${LLAMA_BUILD_DIR}/libllama.a" ]] || die "libllama.a not found after build"
}

download_model() {
  if [[ "${DOWNLOAD_MODEL}" != "1" ]]; then
    return 0
  fi
  ensure_cmd curl curl
  mkdir -p "${MODEL_CACHE_DIR}"
  if [[ -f "${MODEL_FILE}" ]]; then
    log "Model already present: ${MODEL_FILE}"
    return 0
  fi
  log "Downloading embedding model to ${MODEL_FILE} ..."
  curl -L --fail --progress-bar -C - -o "${MODEL_FILE}" "${MODEL_URL}"
  log "Model downloaded."
}

main() {
  log "Bootstrapping native LLM toolchain and bridge..."
  ensure_cmd cmake cmake
  ensure_cmd make make
  ensure_cmd clang++ clang || ensure_cmd g++ g++
  ensure_cmd ar binutils
  ensure_cmd git git
  ensure_cmd pkg-config pkg-config

  build_llama_cpp

  log "Building gorkbot LLM bridge static library..."
  bash "${SCRIPT_DIR}/build_llm_bridge.sh"

  download_model

  log "Verifying CGO bridge compiles..."
  (cd "${PROJECT_ROOT}" && CGO_ENABLED=1 go test ./internal/llm -tags llamacpp -count=1 >/dev/null)

  log "Native LLM bootstrap complete."
  log "Next build command: CGO_ENABLED=1 go build -tags \"llamacpp,with_security,with_plugins,with_headless,with_mcp\" -o bin/gorkbot ./cmd/gorkbot"
}

main "$@"
