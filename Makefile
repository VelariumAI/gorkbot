APP_NAME := gorkbot
WEB_APP_NAME := gorkweb
CMD_PATH := ./cmd/gorkbot
WEB_CMD_PATH := ./cmd/gorkweb
BUILD_DIR := ./bin
SHORT_NAME := gork
INSTALL_DIR := $(HOME)/bin
LLM_DIR := ./internal/llm
LLAMA_ROOT := ./ext/llama.cpp

.PHONY: all build clean install install-global install-dev \
        build-windows build-android build-linux build-security \
        build-llm-bridge build-llm bootstrap-llm clean-llm download-nomic build-web \
        build-all build-lite build-sec build-plugins build-gorkweb unleash setup setup-auto \
        release-check

all: build build-web

# ── Unified Setup Wizard ─────────────────────────────────────────────────────
# One-command onboarding for beginners through power users.
# Includes dependency install, API key setup, optional native LLM bridge/model,
# build/install, launcher setup, and optional validation.
setup:
	@bash scripts/setup_wizard.sh

# Non-interactive default path for quick onboarding.
setup-auto:
	@bash scripts/setup_wizard.sh --auto

# One-command public release readiness checklist.
release-check:
	@bash scripts/release_checklist.sh

# ── Specialized Binaries (Task 5.5) ──────────────────────────────────────────

build-all: build-lite build-sec build-plugins build-gorkweb

build-lite:
	@echo "Building $(APP_NAME)-lite (core only)..."
	@mkdir -p $(BUILD_DIR)
	go build -tags "lite" -o $(BUILD_DIR)/$(APP_NAME)-lite $(CMD_PATH)

build-sec:
	@echo "Building $(APP_NAME)-sec (with security tools)..."
	@mkdir -p $(BUILD_DIR)
	go build -tags "with_security" -o $(BUILD_DIR)/$(APP_NAME)-sec $(CMD_PATH)

build-plugins:
	@echo "Building $(APP_NAME)-plugins (with plugins and headless)..."
	@mkdir -p $(BUILD_DIR)
	go build -tags "with_plugins,with_headless,with_mcp" -o $(BUILD_DIR)/$(APP_NAME)-plugins $(CMD_PATH)

build-gorkweb:
	@echo "Building $(WEB_APP_NAME) (headless only)..."
	@mkdir -p $(BUILD_DIR)
	go build -tags "with_headless,with_mcp" -o $(BUILD_DIR)/$(WEB_APP_NAME) $(WEB_CMD_PATH)

# ── SBOM Generation (Task 5.5) ──────────────────────────────────────────────

.PHONY: sbom
sbom: build-all
	@echo "Generating SBOMs..."
	@mkdir -p sbom
	@syft bin/gorkbot-lite -o spdx-json > sbom/gorkbot-lite.spdx.json || echo "Warning: syft not found, skipping SBOM"
	@syft bin/gorkbot-sec -o spdx-json > sbom/gorkbot-sec.spdx.json || true
	@syft bin/gorkbot-plugins -o spdx-json > sbom/gorkbot-plugins.spdx.json || true
	@syft bin/gorkbot -o spdx-json > sbom/gorkbot.spdx.json || true

build:
	@echo "Building $(APP_NAME) for host OS (full)..."
	@mkdir -p $(BUILD_DIR)
	go build -tags "with_security,with_plugins,with_headless,with_mcp" -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)
build-web:
	@echo "Building $(WEB_APP_NAME) for host OS (full)..."
	@mkdir -p $(BUILD_DIR)
	go build -tags "with_security,with_plugins,with_headless,with_mcp" -o $(BUILD_DIR)/$(WEB_APP_NAME) $(WEB_CMD_PATH)
build-windows:
	@echo "Building for Windows (amd64)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME).exe $(CMD_PATH)

build-android:
	@echo "Building for Android (arm64)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=android GOARCH=arm64 go build -o $(BUILD_DIR)/$(APP_NAME)-android $(CMD_PATH)

build-linux:
	@echo "Building for Linux (amd64)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME)-linux $(CMD_PATH)

build-security:
	@echo "Building with security tools enabled (-tags with_security)..."
	@mkdir -p $(BUILD_DIR)
	go build -tags with_security -o $(BUILD_DIR)/$(APP_NAME)-security $(CMD_PATH)

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

install: build build-web
	@echo "Installing to $(GOPATH)/bin..."
	@go install $(CMD_PATH)
	@go install $(WEB_CMD_PATH)

# ── Global installation ────────────────────────────────────────────────────
# Full unified install: builds the C++ LLM bridge, compiles with llamacpp
# tag, downloads nomic embedding model if absent, installs binary to
# ~/bin/gorkbot, and writes a self-contained 'gork' launcher.
# No root needed — works in Termux and standard Linux.
#
# Add ~/bin to PATH if not already there:
#   echo 'export PATH="$HOME/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc

install-global: build-llm download-nomic build-web
	@install -m 755 $(BUILD_DIR)/$(APP_NAME) $(INSTALL_DIR)/$(APP_NAME)
	@install -m 755 $(BUILD_DIR)/$(WEB_APP_NAME) $(INSTALL_DIR)/$(WEB_APP_NAME)
	@bash scripts/write_launcher.sh $(INSTALL_DIR) $(APP_NAME) $(SHORT_NAME)

# ── Developer Installation (Easter Egg: Full Feature Build) ─────────────────────
# install-dev: Global installation with ALL features enabled (security, plugins, headless, MCP, LLM).
# Creates a single 'gork' command with absolute maximum capability.
# Command: make install-dev
# Result: ~/bin/gork with every feature unlocked
#
# For the curious: This target exists to give developers (and intrepid users) access to
# the full power of Gorkbot without needing to rebuild. It's an easter egg because who
# doesn't like discovering hidden capabilities? 🧠✨

install-dev: build-llm download-nomic
	@echo "🔓 Unleashing the full power of Gorkbot..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=1 go build -tags "with_security,with_plugins,with_headless,with_mcp,llamacpp" \
		-o $(BUILD_DIR)/$(APP_NAME)-dev $(CMD_PATH)
	@install -m 755 $(BUILD_DIR)/$(APP_NAME)-dev $(INSTALL_DIR)/$(SHORT_NAME)
	@echo "✨ $(SHORT_NAME) installed to $(INSTALL_DIR) with ALL features enabled"
	@echo "   Security Tools    → enabled (pentesting, redteaming)"
	@echo "   Plugin SDK v2     → enabled (custom plugin loading)"
	@echo "   Headless API      → enabled (REST endpoints)"
	@echo "   Model Context Pro → enabled (MCP protocol)"
	@echo "   Local LLM Engine  → enabled (Llama.cpp integration)"
	@echo ""
	@echo "🚀 Try it: gork --help"

# ── Easter Egg Build Alias ────────────────────────────────────────────────────
# unleash: Alias for install-dev. Because sometimes you just want to unleash the beast.

unleash: install-dev

# ── Local LLM engine (llamacpp build tag) ─────────────────────────────────────

# Full autonomous bootstrap: deps + llama.cpp libs + bridge + model.
bootstrap-llm:
	@echo "Bootstrapping native LLM stack (autonomous setup)..."
	@AUTO_INSTALL_DEPS=1 DOWNLOAD_MODEL=1 bash scripts/bootstrap_native_llm.sh

# Compile the C++ bridge → internal/llm/libgorkbot_llm.a
build-llm-bridge:
	@echo "Preparing llama.cpp + bridge (deps auto-install enabled)..."
	@AUTO_INSTALL_DEPS=1 DOWNLOAD_MODEL=0 bash scripts/bootstrap_native_llm.sh

# Build gorkbot with the native LLM engine enabled.
build-llm: build-llm-bridge
	@echo "Building $(APP_NAME) with local LLM engine..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -tags "llamacpp,with_security,with_plugins,with_headless,with_mcp" -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)
	@echo "✓ Built $(BUILD_DIR)/$(APP_NAME) (with llamacpp)"

# Clean LLM bridge artifacts (does not touch ext/llama.cpp).
clean-llm:
	@echo "Cleaning LLM bridge artifacts..."
	@rm -f $(LLM_DIR)/libgorkbot_llm.a $(LLM_DIR)/llm_bridge.o

# Download the nomic-embed-text-v1.5 embedding model (~274 MB, Q4_K_M).
# Used by the semantic MEL heuristic store when no cloud embedder is configured.
download-nomic:
	@echo "Downloading nomic-embed-text-v1.5.Q4_K_M.gguf..."
	@mkdir -p $(HOME)/.cache/llama.cpp
	@MODEL_FILE="$(HOME)/.cache/llama.cpp/nomic-embed-text-v1.5.Q4_K_M.gguf"; \
	 URL="https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q4_K_M.gguf"; \
	 if [ -f "$$MODEL_FILE" ]; then \
	   echo "Already present: $$MODEL_FILE"; \
	 else \
	   echo "Downloading from HuggingFace (~274 MB)..."; \
	   curl -L --progress-bar -C - -o "$$MODEL_FILE" "$$URL"; \
	   echo "✓ Downloaded $$MODEL_FILE"; \
	 fi
