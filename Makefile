.PHONY: all build build-cli build-gateway gateway-dev dev-gateway gateway-restart backend-restart \
	dev-electron electron-restart test clean install fmt vet lint coverage bridge \
	bridge-install bridge-build bridge-run webui-install webui-build webui-dev \
	webfetch-install up up-daemon down-daemon restart-daemon docker-build docker-run \
	run dev mod electron-ensure-deps electron-install electron-dev electron-build \
	electron-start electron-dist electron-dist-mac electron-dist-win electron-dist-linux help

# Core binaries used by CLI users and the desktop app.
BINARY_NAME=maxclaw
GATEWAY_BINARY_NAME=maxclaw-gateway
BUILD_DIR=build
MAIN_FILE=cmd/maxclaw/main.go
GATEWAY_MAIN_FILE=cmd/maxclaw-gateway/main.go

# Runtime helper directories and default ports.
BRIDGE_DIR=bridge
BRIDGE_PORT?=3001
WEBFETCH_DIR=webfetcher

# Default target keeps local builds simple.
all: build

# Build both Go binaries used by CLI and desktop packaging.
build:
	@echo "Building..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	go build -o $(BUILD_DIR)/$(GATEWAY_BINARY_NAME) $(GATEWAY_MAIN_FILE)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "Build complete: $(BUILD_DIR)/$(GATEWAY_BINARY_NAME)"

# Build only the full CLI binary.
build-cli:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)

# Build only the standalone gateway binary.
build-gateway:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(GATEWAY_BINARY_NAME) $(GATEWAY_MAIN_FILE)

# Rebuild and run the standalone gateway in the foreground.
gateway-dev: build-gateway
	./$(BUILD_DIR)/$(GATEWAY_BINARY_NAME)

# Alias used in docs for backend-only development.
dev-gateway: gateway-dev

# Rebuild and restart background daemon services after backend changes.
gateway-restart:
	FORCE_GATEWAY_KILL=1 ./scripts/start_daemon.sh

# Alias used in docs for backend restart after Go changes.
backend-restart: gateway-restart

# Install both Go binaries to GOPATH/bin.
install:
	go install ./cmd/maxclaw
	go install ./cmd/maxclaw-gateway

# Run all Go tests.
test:
	go test -v ./...

# WhatsApp Bridge（Node.js）
bridge-install:
	cd $(BRIDGE_DIR) && npm install

bridge-build:
	cd $(BRIDGE_DIR) && npm run build

bridge-run:
	cd $(BRIDGE_DIR) && BRIDGE_PORT=$(BRIDGE_PORT) npm start

# Convenience target for local bridge bring-up.
bridge: bridge-install bridge-build bridge-run

# Web UI asset build helpers.
webui-install:
	cd webui && npm install

webui-build:
	cd webui && npm run build

webui-dev:
	cd webui && npm run dev

webfetch-install:
	cd $(WEBFETCH_DIR) && npm install

# Start bridge + gateway in the current terminal.
up:
	FORCE_BRIDGE_KILL=1 FORCE_GATEWAY_KILL=1 ./scripts/start_all.sh

# Start bridge + gateway in the background with PID/log management.
up-daemon:
	FORCE_BRIDGE_KILL=1 FORCE_GATEWAY_KILL=1 ./scripts/start_daemon.sh

# Stop background services started by up-daemon / gateway-restart.
down-daemon:
	./scripts/stop_daemon.sh

# Restart background services after config or binary changes.
restart-daemon:
	./scripts/restart_daemon.sh

docker-build:
	docker build -t maxclaw .

docker-run:
	docker run --rm -v ~/.maxclaw:/home/maxclaw/.maxclaw -p 18890:18890 maxclaw gateway

# Generate HTML coverage report.
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Format Go code.
fmt:
	go fmt ./...

# Run go vet.
vet:
	go vet ./...

# Clean build artifacts.
clean:
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Cleaned"

# Build and run the CLI entrypoint.
run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# Go hot-reload helper (requires air).
dev:
	@which air > /dev/null || (echo "Installing air..." && go install github.com/cosmtrek/air@latest)
	air

# Tidy and verify Go modules.
mod:
	go mod tidy
	go mod verify

# Run formatting and static checks.
lint: fmt vet
	@echo "Linting complete"

# Electron Desktop App
electron-ensure-deps:
	@if [ ! -x electron/node_modules/.bin/vite ]; then \
		echo "Electron dependencies missing (vite). Installing..."; \
		cd electron && if [ -f package-lock.json ]; then npm ci; else npm install; fi; \
	else \
		echo "Electron dependencies OK"; \
	fi

electron-install:
	cd electron && npm install

electron-dev:
	cd electron && npm run dev

electron-build:
	cd electron && npm run build

# Rebuild binaries and relaunch the packaged-mode Electron app.
electron-start: build electron-ensure-deps
	cd electron && npm start

# Rebuild the desktop app after frontend/main-process changes.
electron-restart: build electron-build
	cd electron && npm start

# Alias used in docs for Electron development mode.
dev-electron: electron-dev

electron-dist: build
	cd electron && npm run dist

electron-dist-mac: build
	cd electron && npm run dist:mac

electron-dist-win: build
	cd electron && npm run dist:win

electron-dist-linux: build
	cd electron && npm run dist:linux

# 帮助
help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  build-cli  - Build the CLI binary"
	@echo "  build-gateway - Build the standalone gateway binary"
	@echo "  gateway-dev - Rebuild and run the standalone gateway in foreground"
	@echo "  dev-gateway - Alias of gateway-dev"
	@echo "  gateway-restart - Rebuild/restart background gateway daemon"
	@echo "  backend-restart - Alias of gateway-restart"
	@echo "  test       - Run tests"
	@echo "  coverage   - Run tests with coverage"
	@echo "  install    - Install to GOPATH/bin"
	@echo "  fmt        - Format code"
	@echo "  vet        - Run go vet"
	@echo "  lint       - Run fmt and vet"
	@echo "  clean      - Clean build artifacts"
	@echo "  run        - Build and run"
	@echo "  dev        - Run with hot reload (requires air)"
	@echo "  mod        - Tidy and verify modules"
	@echo "  bridge     - Install/build/run WhatsApp bridge"
	@echo "  bridge-install - Install bridge dependencies"
	@echo "  bridge-build   - Build bridge"
	@echo "  bridge-run     - Run bridge (BRIDGE_PORT=$(BRIDGE_PORT))"
	@echo "  webui-install  - Install web UI dependencies"
	@echo "  webui-build    - Build web UI"
	@echo "  webui-dev      - Run web UI dev server"
	@echo "  webfetch-install - Install Playwright web fetcher"
	@echo "  electron-install - Install Electron app dependencies"
	@echo "  electron-ensure-deps - Ensure Electron runtime deps (vite/electron) are installed"
	@echo "  electron-dev     - Run Electron app in dev mode (with hot reload)"
	@echo "  dev-electron     - Alias of electron-dev"
	@echo "  electron-start   - Build and run Electron app (production mode)"
	@echo "  electron-restart - Rebuild and relaunch Electron after frontend changes"
	@echo "  electron-build   - Build Electron app"
	@echo "  electron-dist    - Create Electron distributable"
	@echo "  electron-dist-mac - Create macOS distributable"
	@echo "  electron-dist-win - Create Windows distributable"
	@echo "  electron-dist-linux - Create Linux distributable"
	@echo "  up         - Start bridge + gateway"
	@echo "  up-daemon  - Start bridge + gateway in background"
	@echo "  down-daemon - Stop background bridge + gateway"
	@echo "  restart-daemon - Restart background bridge + gateway"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run - Run gateway in Docker"
	@echo "  help       - Show this help"
