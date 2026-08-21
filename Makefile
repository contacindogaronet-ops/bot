# ==============================================================================
# JARGO MTProto Userbot Makefile
# Target: Termux Android ARM64 & Linux x86_64
# Zero CGO, Zero Shared Library Bloat
# ==============================================================================

APP_NAME = jargo-userbot
BUILD_DIR = ./bin
SRC = ./main.go

# Compiler flags for small, stripped, zero-alloc binary
LDFLAGS = -s -w -X 'main.Version=1.0.0' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'

.PHONY: all build build-termux-arm64 clean run deps

all: build

deps:
	@echo "[+] Downloading Go dependencies..."
	go mod download
	go mod tidy

# Native build (for current system)
build:
	@echo "[+] Building native binary for $(shell go env GOOS)/$(shell go env GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/$(APP_NAME) $(SRC)
	@echo "[✓] Build complete: $(BUILD_DIR)/$(APP_NAME)"

# Cross-compile for Termux ARM64 (Android 64-bit ARM)
build-termux-arm64:
	@echo "[+] Cross-compiling for Android/Linux ARM64 (Termux)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/$(APP_NAME)-arm64 $(SRC)
	@echo "[✓] Termux ARM64 binary built: $(BUILD_DIR)/$(APP_NAME)-arm64"

# Run locally
run:
	@echo "[+] Running JARGO Userbot..."
	go run $(SRC)

clean:
	@echo "[+] Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) downloads session.json
