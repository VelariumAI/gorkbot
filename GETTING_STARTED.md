# Getting Started

This guide gets Gorkbot running quickly with production-safe defaults.

## 1. Prerequisites

- OS: Linux, macOS, Windows, or Android (Termux)
- Go: 1.25+ (only required when building from source)
- Network access for provider APIs

## 2. Clone and Enter Repo

```bash
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
```

## 3. Recommended Setup (Interactive)

```bash
make setup
```

`make setup` is the primary onboarding path. It walks through:
- environment and dependency checks,
- API key configuration,
- optional native LLM bridge bootstrap,
- optional semantic ranking model download,
- build/install and validation.

## 4. Fast Default Setup (Non-Interactive)

```bash
make setup-auto
```

Use this for quick local bootstrap with default choices.

## 5. Manual Build and Run

```bash
make build
./bin/gorkbot --version
./bin/gorkbot
```

One-shot example:

```bash
./bin/gorkbot -p "Summarize this repository architecture."
```

## 6. Core Runtime Commands

```text
/help      list commands
/model     change model
/tools     inspect available tools
/settings  update runtime configuration
/version   show public/internal version info
```

## 7. Release and Quality Gates

```bash
bash scripts/release_checklist.sh
```

For release tagging and publish flow, see [docs/RELEASE_OPERATIONS.md](docs/RELEASE_OPERATIONS.md).

## 8. Next Reading

- [README.md](README.md)
- [VERSIONING.md](VERSIONING.md)
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/PERMISSIONS_GUIDE.md](docs/PERMISSIONS_GUIDE.md)
