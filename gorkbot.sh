#!/bin/bash
# Gorkbot launcher — loads .env then runs the binary
# Lead Designer & Engineer: Todd Eddings / Velarium AI
# Works with any OpenAI-compatible API.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.env"
BINARY="${SCRIPT_DIR}/bin/gorkbot"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [ ! -f "$BINARY" ]; then
    echo -e "${RED}Error: Binary not found at ${BINARY}${NC}"
    echo "Run 'make build' first."
    exit 1
fi

if [ -f "$ENV_FILE" ]; then
    echo -e "${GREEN}Loading environment from .env...${NC}"
    set -a
    source <(grep -v '^#' "$ENV_FILE" | grep -v '^$' | sed 's/\r$//')
    set +a

    if [[ "$XAI_API_KEY" == "your_xai_key_here" ]] || [[ -z "$XAI_API_KEY" ]]; then
        echo -e "${YELLOW}Warning: XAI_API_KEY is not configured${NC}"
    fi

    if [[ "$GEMINI_API_KEY" == "your_gemini_key_here" ]] || [[ -z "$GEMINI_API_KEY" ]]; then
        echo -e "${YELLOW}Warning: GEMINI_API_KEY is not configured${NC}"
    fi
else
    echo -e "${YELLOW}Warning: .env file not found. Using existing environment variables.${NC}"
fi

exec "$BINARY" "$@"
