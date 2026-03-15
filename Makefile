APP_NAME := gorkbot
WEB_APP_NAME := gorkweb
CMD_PATH := ./cmd/gorkbot
WEB_CMD_PATH := ./cmd/gorkweb
BUILD_DIR := ./bin
SHORT_NAME := gork
INSTALL_DIR := $(HOME)/bin
LLM_DIR := ./internal/llm
LLAMA_ROOT := ./ext/llama.cpp

.PHONY: all build clean install install-global \
        build-windows build-android build-linux \
        build-llm-bridge build-llm clean-llm download-nomic build-web \
        init-submodules

all: build build-web

build:
	@echo "Building $(APP_NAME) for host OS..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)

build-web:
	@echo "Building $(WEB_APP_NAME) for host OS..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(WEB_APP_NAME) $(WEB_CMD_PATH)

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

# ── Local LLM engine (llamacpp build tag) ─────────────────────────────────────

# Ensure ext/llama.cpp submodule is checked out before any C++ build step.
init-submodules:
	@if [ ! -f "$(LLAMA_ROOT)/include/llama.h" ]; then \
	  echo "Initialising ext/llama.cpp submodule..."; \
	  git submodule update --init --checkout --force ext/llama.cpp; \
	  echo "✓ ext/llama.cpp ready"; \
	else \
	  echo "✓ ext/llama.cpp already present"; \
	fi

# Compile the C++ bridge → internal/llm/libgorkbot_llm.a
build-llm-bridge: init-submodules
	@echo "Compiling LLM bridge (C++ → static lib)..."
	@bash scripts/build_llm_bridge.sh

# Build gorkbot with the native LLM engine enabled.
build-llm: build-llm-bridge
	@echo "Building $(APP_NAME) with local LLM engine..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -tags llamacpp -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)
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

