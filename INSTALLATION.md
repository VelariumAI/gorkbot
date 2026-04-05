# Installation Guide

Detailed installation instructions for all platforms and deployment methods.

---

## Table of Contents

1. [System Requirements](#system-requirements)
2. [Installation Methods](#installation-methods)
3. [Platform-Specific Guides](#platform-specific-guides)
4. [Verification](#verification)
5. [Uninstallation](#uninstallation)
6. [Troubleshooting](#troubleshooting)

---

## System Requirements

### Mandatory
- **Go 1.25.0+** (if building from source)
- **At least 512MB RAM**
- **Internet connection** (for API calls)
- **One API key** from a supported provider

### Supported Platforms
- ✅ **Linux** (x86_64, ARM64)
- ✅ **macOS** (Intel & Apple Silicon)
- ✅ **Windows** (10+, PowerShell or Command Prompt)
- ✅ **Android** (via Termux)

---

## Installation Methods

### Method 1: Docker (Easiest)

**Prerequisites**: Docker installed

```bash
# Pull latest image
docker pull ghcr.io/velariumai/gorkbot:latest

# Run interactive
docker run -it \
  -e XAI_API_KEY=xai-xxx \
  -v ~/.config/gorkbot:/root/.config/gorkbot \
  ghcr.io/velariumai/gorkbot:latest

# Or with docker-compose
cat > docker-compose.yml << 'EOF'
version: '3'
services:
  gorkbot:
    image: ghcr.io/velariumai/gorkbot:latest
    environment:
      XAI_API_KEY: ${XAI_API_KEY}
      GEMINI_API_KEY: ${GEMINI_API_KEY}
    volumes:
      - ~/.config/gorkbot:/root/.config/gorkbot
    stdin_open: true
    tty: true
EOF

docker-compose up
```

### Method 2: Pre-built Binaries (Fast)

Visit [GitHub Releases](https://github.com/velariumai/gorkbot/releases/latest) and download:

- `gorkbot-linux-amd64` (Linux x86_64)
- `gorkbot-darwin-amd64` (macOS Intel)
- `gorkbot-darwin-arm64` (macOS Apple Silicon)
- `gorkbot-windows-amd64.exe` (Windows)

**Installation**:

```bash
# Linux/macOS
chmod +x gorkbot-linux-amd64
sudo mv gorkbot-linux-amd64 /usr/local/bin/gorkbot

# Verify
gorkbot --version

# Windows
# Download .exe and add to PATH
# Or run from Download folder directly
```

### Method 3: Build from Source (Flexible)

#### Prerequisites

```bash
# Check Go version
go version  # Requires 1.25.0+

# If not installed:
# Ubuntu/Debian
sudo apt install golang-1.25

# macOS (Homebrew)
brew install go@1.25

# Windows
# Download from https://golang.org/dl/
```

#### Build Steps

```bash
# Clone repository
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot

# Verify Go
go version

# Download dependencies
go mod download

# Build
make build

# Output: ./bin/gorkbot

# Verify
./bin/gorkbot --version

# Optional: Install globally
sudo cp bin/gorkbot /usr/local/bin/
# or
make install
```

#### Build with Custom Flags

```bash
# Optimized build (smaller binary)
CGO_ENABLED=0 go build \
  -ldflags="-s -w" \
  -o bin/gorkbot \
  ./cmd/gorkbot/

# With version info
go build \
  -ldflags="-X main.Version=1.2.0 -X main.BuildTime=$(date)" \
  -o bin/gorkbot \
  ./cmd/gorkbot/

# For specific OS/architecture
GOOS=windows GOARCH=amd64 go build \
  -o bin/gorkbot.exe \
  ./cmd/gorkbot/
```

---

## Platform-Specific Guides

### Linux (Ubuntu/Debian)

```bash
# Install dependencies
sudo apt update
sudo apt install golang-1.25 git sqlite3

# Clone and build
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
make build

# Make executable
chmod +x bin/gorkbot

# Add to PATH
sudo cp bin/gorkbot /usr/local/bin/

# Configure
gorkbot setup
```

### macOS (Intel & Apple Silicon)

```bash
# Install Go (Homebrew)
brew install go

# Clone and build
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
make build

# Install
make install

# Verify
gorkbot --version
```

**Note**: On Apple Silicon, the build system detects your architecture automatically.

### Windows (PowerShell)

```powershell
# Download Go from https://golang.org/dl/
# Add to PATH

# Clone repository
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot

# Build
go build -o bin\gorkbot.exe .\cmd\gorkbot\

# Run
.\bin\gorkbot.exe

# Add to PATH for global access
# System Properties > Environment Variables > PATH > Add folder containing .exe
```

### Android (Termux)

```bash
# Update Termux
apt update && apt upgrade

# Install Go
apt install golang

# Clone and build
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
make build-android

# Run
./bin/gorkbot

# Configure API keys
./gorkbot setup
```

---

## Verification

### Check Installation

```bash
gorkbot --version
# Output: Gorkbot 1.6.1-rc (internal: 6.2.0)

gorkbot status
# Output: ✅ All systems operational
```

### Test Connectivity

```bash
gorkbot -p "Hello, are you working?"
# Should receive response from AI provider
```

---

## Configuration

### Initial Setup

```bash
gorkbot setup
```

The wizard will:
1. Create config directory (~/.config/gorkbot)
2. Prompt for API keys
3. Validate each key
4. Save to .env file
5. Test provider connectivity

### API Keys

Create `.env` file in Gorkbot directory:

```bash
XAI_API_KEY=xai-xxxxxxxxxxxxx
GEMINI_API_KEY=AIzaxxxxxxxxxxxxxxx
ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxx
OPENAI_API_KEY=sk-proj-xxxxxxxxxxxxx
```

**Important**: Never commit .env to git!

---

## Uninstallation

### If installed via Homebrew (macOS)

```bash
brew uninstall gorkbot
```

### If installed via apt (Linux)

```bash
sudo apt remove gorkbot
```

### If installed manually

```bash
# Remove binary
sudo rm /usr/local/bin/gorkbot

# Remove configuration
rm -rf ~/.config/gorkbot

# Remove database
rm ~/.config/gorkbot/gorkbot.db
```

### If using Docker

```bash
docker rmi ghcr.io/velariumai/gorkbot:latest
```

---

## Troubleshooting

### "Go: command not found"

**Solution**: Install Go 1.25.0+

```bash
# Ubuntu/Debian
sudo apt install golang-1.25

# macOS
brew install go

# Check version
go version
```

### "Failed to build: missing dependencies"

**Solution**: Download dependencies

```bash
go mod download
go mod tidy
make build
```

### "Permission denied" when running binary

**Solution**: Make executable

```bash
chmod +x bin/gorkbot
```

### "Cannot find config directory"

**Solution**: Create manually and run setup

```bash
mkdir -p ~/.config/gorkbot
gorkbot setup
```

### "API key validation failed"

**Solution**: Verify key format and validity

```bash
# Check key format (should not have spaces or newlines)
echo "$XAI_API_KEY" | od -c

# Re-generate key from provider website
# Run setup again
gorkbot setup
```

---

## Next Steps

1. **Run Gorkbot**: `gorkbot`
2. **Read QUICK_START**: Get running in 5 minutes
3. **Read GETTING_STARTED**: Full onboarding guide
4. **Read CONFIGURATION**: All config options

---

**Installation complete! See [GETTING_STARTED.md](GETTING_STARTED.md) to begin using Gorkbot.**
