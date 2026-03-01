#!/bin/bash
set -e

echo "Initializing Grokster setup..."

# Check Go
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go 1.21+."
    exit 1
fi

# Setup .env
if [ ! -f .env ]; then
    echo "Creating .env template..."
    cat <<EOF > .env
XAI_API_KEY=your_xai_key_here
GEMINI_API_KEY=your_gemini_key_here
EOF
    echo ".env created. Please edit it with your API keys."
else
    echo ".env already exists."
fi

# Build
echo "Building Grokster..."
if command -v make &> /dev/null; then
    make build
else
    echo "Make not found, using direct go build..."
    mkdir -p bin
    go build -o bin/grokster ./cmd/grokster
fi

echo "Setup complete! Run './bin/grokster' to start."
