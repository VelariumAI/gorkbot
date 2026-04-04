#!/usr/bin/env bash
# build_llm_bridge.sh — Compile internal/llm/llm_bridge.cpp into libgorkbot_llm.a
# Run this before `go build -tags llamacpp ...` or via `make build-llm-bridge`.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LLM_DIR="${PROJECT_ROOT}/internal/llm"
BRIDGE_DIR="${LLM_DIR}/cbridge"
LLAMA_ROOT="${PROJECT_ROOT}/ext/llama.cpp"

mkdir -p "${BRIDGE_DIR}"

# Backward-compat migration: older mirrors kept bridge files in internal/llm root.
if [[ -f "${LLM_DIR}/llm_bridge.cpp" && ! -f "${BRIDGE_DIR}/llm_bridge.cpp" ]]; then
    mv "${LLM_DIR}/llm_bridge.cpp" "${BRIDGE_DIR}/llm_bridge.cpp"
fi
if [[ -f "${LLM_DIR}/llm_bridge.h" && ! -f "${BRIDGE_DIR}/llm_bridge.h" ]]; then
    mv "${LLM_DIR}/llm_bridge.h" "${BRIDGE_DIR}/llm_bridge.h"
fi

BRIDGE_CPP="${BRIDGE_DIR}/llm_bridge.cpp"
if [[ ! -f "${BRIDGE_CPP}" ]]; then
    echo "ERROR: missing bridge source at ${BRIDGE_CPP}" >&2
    exit 1
fi

INCLUDE_FLAGS=(
    "-I${LLAMA_ROOT}/include"
    "-I${LLAMA_ROOT}/ggml/include"
    "-I${BRIDGE_DIR}"
    "-I${LLM_DIR}"
)

OBJ="${LLM_DIR}/llm_bridge.o"
OUT="${LLM_DIR}/libgorkbot_llm.a"

# Detect C++ compiler — prefer clang++, fall back to g++.
if command -v clang++ &>/dev/null; then
    CXX="clang++"
elif command -v g++ &>/dev/null; then
    CXX="g++"
else
    echo "ERROR: no C++ compiler found (need clang++ or g++)" >&2
    exit 1
fi

echo "→ Compiler : ${CXX}"
echo "→ Bridge   : ${BRIDGE_CPP}"
echo "→ Output   : ${OUT}"

# Compile the bridge object.
"${CXX}" -std=c++17 -O2 \
    "${INCLUDE_FLAGS[@]}" \
    -c "${BRIDGE_CPP}" \
    -o "${OBJ}"

# Archive into a static library.
AR="${AR:-ar}"
if command -v llvm-ar &>/dev/null; then
    AR="llvm-ar"
fi
"${AR}" rcs "${OUT}" "${OBJ}"
rm -f "${OBJ}"

echo "✓ Built ${OUT}"
echo ""
echo "Now run:  go build -tags llamacpp -o bin/gorkbot ./cmd/gorkbot"
