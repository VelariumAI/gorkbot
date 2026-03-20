# Getting Started Guide

Complete step-by-step guide to set up and start using Gorkbot.

---

## Table of Contents

1. [System Requirements](#system-requirements)
2. [Installation](#installation)
3. [API Key Setup](#api-key-setup)
4. [First Run](#first-run)
5. [Basic Usage](#basic-usage)
6. [Understanding the Interface](#understanding-the-interface)
7. [Common Workflows](#common-workflows)
8. [Next Steps](#next-steps)

---

## System Requirements

### Minimum Requirements
- **OS**: Linux, macOS, Windows, or Android (Termux)
- **RAM**: 512MB minimum, 2GB recommended
- **Disk**: 100MB for application, depends on SQLite database size
- **Network**: Internet connection for API calls

### Recommended Setup
- **OS**: Ubuntu 20.04+, macOS 11+, or Windows 11
- **RAM**: 4GB or more
- **Go**: 1.25.0+ (if building from source)
- **Terminal**: bash, zsh, or PowerShell

### Optional Components
- **Docker**: For containerized deployment
- **Git**: For version control workflows
- **Python 3.8+**: For advanced Python tool integration
- **Node.js**: For certain MCP server integrations

---

## Installation

### Option 1: Docker (Recommended for Beginners)

```bash
docker pull ghcr.io/velariumai/gorkbot:latest

docker run -it \
  -e XAI_API_KEY=your_key_here \
  -v ~/.config/gorkbot:/root/.config/gorkbot \
  ghcr.io/velariumai/gorkbot:latest
```

### Option 2: Pre-built Binaries

Download from [GitHub Releases](https://github.com/velariumai/gorkbot/releases/latest):

```bash
# Linux
wget https://github.com/velariumai/gorkbot/releases/latest/download/gorkbot-linux-amd64
chmod +x gorkbot-linux-amd64
./gorkbot-linux-amd64 --version
```

### Option 3: Build from Source

```bash
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
make build
./bin/gorkbot --version
```

---

## API Key Setup

### Get Keys from These Providers

| Provider | Setup | Free Tier |
|----------|-------|-----------|
| **Grok (xAI)** | https://console.x.ai | $5 free |
| **Gemini (Google)** | https://aistudio.google.com/apikey | ✅ Yes |
| **Claude (Anthropic)** | https://console.anthropic.com | Free credits |
| **OpenAI (GPT)** | https://platform.openai.com/api-keys | $5 free |

### Configure Keys

**Option A: Interactive Setup**
```bash
./gorkbot setup
```

**Option B: Environment Variables**
```bash
export XAI_API_KEY=xai-xxx
export GEMINI_API_KEY=AIza-xxx
./gorkbot
```

**Option C: .env File**
```bash
echo "XAI_API_KEY=xai-xxx" > .env
echo "GEMINI_API_KEY=AIza-xxx" >> .env
./gorkbot
```

---

## First Run

```bash
./gorkbot

# You should see:
# ✨ Welcome to Gorkbot 1.2.0-beta
# 🤖 Primary: Grok | Secondary: Gemini
# > _
```

Type a message and press Enter to start!

---

## Common Commands

```
/help              Show all commands
/model             Change active model
/clear             Clear conversation
/settings          Configure preferences
/tools             List available tools
/export <file>     Export conversation
/compress          Compress history
/verbose <on|off>  Toggle verbose mode
/quit              Exit
```

---

## Next Steps

- 📖 Read full [GETTING_STARTED.md](GETTING_STARTED.md) for detailed setup
- 🏗️ Review [ARCHITECTURE.md](ARCHITECTURE.md) to understand the system
- 🔧 Explore [TOOL_REFERENCE.md](TOOL_REFERENCE.md) for all available tools
- ❓ Check [FAQ.md](FAQ.md) for common questions

**Need help?** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) or open an [issue](https://github.com/velariumai/gorkbot/issues).

