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
    local termux_pkg="${pkg}"
    case "${pkg}" in
      pkg-config) termux_pkg="pkgconf" ;;
      g++) termux_pkg="clang" ;;
      binutils) termux_pkg="binutils" ;;
    esac
    pkg install -y "${termux_pkg}"
    return 0
  fi

  if command -v apt-get >/dev/null 2>&1; then
    local apt_pkg="${pkg}"
    case "${pkg}" in
      clang) apt_pkg="clang" ;;
      g++) apt_pkg="g++" ;;
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

  if command -v dnf >/dev/null 2>&1; then
    local dnf_pkg="${pkg}"
    case "${pkg}" in
      pkg-config) dnf_pkg="pkgconf-pkg-config" ;;
    esac
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      dnf install -y "${dnf_pkg}"
    elif command -v sudo >/dev/null 2>&1; then
      sudo dnf install -y "${dnf_pkg}"
    else
      die "cannot install '${dnf_pkg}' automatically (dnf requires root/sudo)"
    fi
    return 0
  fi

  if command -v yum >/dev/null 2>&1; then
    local yum_pkg="${pkg}"
    case "${pkg}" in
      pkg-config) yum_pkg="pkgconfig" ;;
    esac
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      yum install -y "${yum_pkg}"
    elif command -v sudo >/dev/null 2>&1; then
      sudo yum install -y "${yum_pkg}"
    else
      die "cannot install '${yum_pkg}' automatically (yum requires root/sudo)"
    fi
    return 0
  fi

  if command -v pacman >/dev/null 2>&1; then
    local pacman_pkg="${pkg}"
    case "${pkg}" in
      pkg-config) pacman_pkg="pkgconf" ;;
      g++) pacman_pkg="gcc" ;;
    esac
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      pacman -Sy --noconfirm "${pacman_pkg}"
    elif command -v sudo >/dev/null 2>&1; then
      sudo pacman -Sy --noconfirm "${pacman_pkg}"
    else
      die "cannot install '${pacman_pkg}' automatically (pacman requires root/sudo)"
    fi
    return 0
  fi

  if command -v zypper >/dev/null 2>&1; then
    local zypper_pkg="${pkg}"
    case "${pkg}" in
      pkg-config) zypper_pkg="pkg-config" ;;
    esac
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      zypper --non-interactive install "${zypper_pkg}"
    elif command -v sudo >/dev/null 2>&1; then
      sudo zypper --non-interactive install "${zypper_pkg}"
    else
      die "cannot install '${zypper_pkg}' automatically (zypper requires root/sudo)"
    fi
    return 0
  fi

  if command -v apk >/dev/null 2>&1; then
    local apk_pkg="${pkg}"
    case "${pkg}" in
      pkg-config) apk_pkg="pkgconf" ;;
      g++) apk_pkg="g++" ;;
      binutils) apk_pkg="binutils" ;;
    esac
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      apk add --no-cache "${apk_pkg}"
    elif command -v sudo >/dev/null 2>&1; then
      sudo apk add --no-cache "${apk_pkg}"
    else
      die "cannot install '${apk_pkg}' automatically (apk requires root/sudo)"
    fi
    return 0
  fi

  if command -v brew >/dev/null 2>&1; then
    log "Installing '${pkg}' via Homebrew..."
    local brew_pkg="${pkg}"
    case "${pkg}" in
      g++) brew_pkg="gcc" ;;
    esac
    brew install "${brew_pkg}" || true
    return 0
  fi

  die "no supported package manager found for auto-installing '${pkg}'"
}

ensure_cpp_compiler() {
  if command -v clang++ >/dev/null 2>&1 || command -v g++ >/dev/null 2>&1; then
    return 0
  fi
  if [[ "${AUTO_INSTALL_DEPS}" != "1" ]]; then
    die "missing required C++ compiler (clang++ or g++)"
  fi

  install_pkg clang || true
  if command -v clang++ >/dev/null 2>&1 || command -v g++ >/dev/null 2>&1; then
    return 0
  fi

  install_pkg g++ || true
  if command -v clang++ >/dev/null 2>&1 || command -v g++ >/dev/null 2>&1; then
    return 0
  fi
  die "unable to install a working C++ compiler (clang++ or g++)"
}

ensure_llama_checkout() {
  if [[ -d "${LLAMA_ROOT}" ]]; then
    return 0
  fi
  if [[ "${AUTO_INSTALL_DEPS}" != "1" ]]; then
    die "missing ${LLAMA_ROOT}. Initialize submodule first: git submodule update --init --recursive ext/llama.cpp"
  fi
  if [[ ! -d "${PROJECT_ROOT}/.git" ]]; then
    die "missing ${LLAMA_ROOT} and repository metadata unavailable for submodule init"
  fi
  log "llama.cpp submodule missing; initializing automatically..."
  (cd "${PROJECT_ROOT}" && git submodule update --init --recursive ext/llama.cpp) || \
    die "failed to initialize ext/llama.cpp submodule automatically"
  [[ -d "${LLAMA_ROOT}" ]] || die "submodule initialization completed but ${LLAMA_ROOT} is still missing"
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
  ensure_cmd go golang
  ensure_cmd cmake cmake
  ensure_cmd make make
  ensure_cpp_compiler
  ensure_cmd ar binutils
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
