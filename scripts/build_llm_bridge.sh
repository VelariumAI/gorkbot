#!/usr/bin/env bash
# build_llm_bridge.sh — Compile internal/llm/llm_bridge.cpp into libgorkbot_llm.a
# Run this before `go build -tags llamacpp ...` or via `make build-llm-bridge`.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LLM_DIR="${PROJECT_ROOT}/internal/llm"
BRIDGE_DIR="${LLM_DIR}/cbridge"
LLAMA_ROOT="${PROJECT_ROOT}/ext/llama.cpp"

INCLUDE_FLAGS=(
    "-I${LLAMA_ROOT}/include"
    "-I${LLAMA_ROOT}/ggml/include"
    "-I${BRIDGE_DIR}"
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
echo "→ Bridge   : ${BRIDGE_DIR}/llm_bridge.cpp"
echo "→ Output   : ${OUT}"

# Compile the bridge object.
"${CXX}" -std=c++17 -O2 \
    "${INCLUDE_FLAGS[@]}" \
    -c "${BRIDGE_DIR}/llm_bridge.cpp" \
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
