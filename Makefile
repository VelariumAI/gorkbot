APP_NAME := gorkbot
VERSION  := v1.0.0
CMD_PATH := ./cmd/gorkbot
BUILD_DIR := ./bin
SHORT_NAME := gork
INSTALL_DIR := $(HOME)/bin
DIST_DIR := ./dist

.PHONY: all build clean install install-global build-windows build-android build-linux dist

all: build

build:
	@echo "Building $(APP_NAME) for host OS..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)

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
	@rm -rf $(BUILD_DIR) $(DIST_DIR)

install: build
	@echo "Installing to $(GOPATH)/bin..."
	@go install $(CMD_PATH)

# Install as 'gork' in ~/bin (works in Termux — no root needed).
# Installs the launcher script (which loads .env) rather than the raw binary,
# so API keys are always picked up automatically.
# Then add  export PATH="$HOME/bin:$PATH"  to ~/.bashrc if not already there.
install-global: build
	@mkdir -p $(INSTALL_DIR)
	@cp $(BUILD_DIR)/$(APP_NAME) $(INSTALL_DIR)/$(APP_NAME)
	@PROJ="$(CURDIR)" && printf '#!/bin/bash\n# Gorkbot global launcher — auto-sources .env\nset -e\nENV_FILE="%s/.env"\nBINARY="%s/$(BUILD_DIR)/$(APP_NAME)"\nif [ -f "$$ENV_FILE" ]; then\n  set -a; source <(grep -v '"'"'^#'"'"' "$$ENV_FILE" | grep -v '"'"'^$$'"'"' | sed '"'"'s/\\r//'"'"'); set +a\nfi\nexec "$$BINARY" "$$@"\n' "$$PROJ" "$$PROJ" > $(INSTALL_DIR)/$(SHORT_NAME)
	@chmod +x $(INSTALL_DIR)/$(SHORT_NAME)
	@echo ""
	@echo "Installed as: $(INSTALL_DIR)/$(SHORT_NAME)"
	@echo ""
	@echo "If ~/bin is not yet on your PATH, run:"
	@echo "  echo 'export PATH=\"\$$HOME/bin:\$$PATH\"' >> ~/.bashrc && source ~/.bashrc"
	@echo ""
	@echo "Then just type: gork"

# Create a release tarball: dist/gorkbot-v1.0.0.tar.gz
dist: build-linux build-windows build-android
	@echo "Creating release tarball..."
	@mkdir -p $(DIST_DIR)
	@tar --exclude='./bin' --exclude='./.git' --exclude='./.env' \
	     --exclude='./dist' --exclude='./vendor' \
	     -czf $(DIST_DIR)/$(APP_NAME)-$(VERSION).tar.gz .
	@cp $(BUILD_DIR)/$(APP_NAME)-linux   $(DIST_DIR)/$(APP_NAME)-$(VERSION)-linux-amd64
	@cp $(BUILD_DIR)/$(APP_NAME)-android $(DIST_DIR)/$(APP_NAME)-$(VERSION)-android-arm64
	@cp $(BUILD_DIR)/$(APP_NAME).exe     $(DIST_DIR)/$(APP_NAME)-$(VERSION)-windows-amd64.exe
	@echo "Release artifacts in $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/
