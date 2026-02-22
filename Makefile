# =============================================================================
# HELP TARGET (default)
# =============================================================================

.PHONY: help
help:
	@echo "velocity.report - Make Targets"
	@echo "=============================="
	@echo ""
	@echo "Pattern: <action>-<subsystem>[-<variant>]"
	@echo ""
	@echo "BUILD TARGETS (Go cross-compilation):"
	@echo "  build-radar-linux    Build for Linux ARM64 (no pcap)"
	@echo "  build-radar-linux-pcap Build for Linux ARM64 with pcap"
	@echo "  build-radar-mac      Build for macOS ARM64 with pcap"
	@echo "  build-radar-mac-intel Build for macOS AMD64 with pcap"
	@echo "  build-radar-local    Build for local development with pcap"
	@echo "  build-tools          Build sweep tool"
	@echo "  build-deploy         Build velocity-deploy deployment manager"
	@echo "  build-deploy-linux   Build velocity-deploy for Linux ARM64"
	@echo "  build-web            Build web frontend (SvelteKit)"
	@echo "  build-docs           Build documentation site (Eleventy)"
	@echo "  build-mac            Build macOS LiDAR visualiser (Xcode)"
	@echo "  clean-mac            Clean macOS visualiser build artifacts"
	@echo "  run-mac              Run macOS visualiser (requires build-mac)"
	@echo "  dev-mac              Kill, build, and run macOS visualiser"
	@echo ""
	@echo "PROTOBUF CODE GENERATION:"
	@echo "  proto-gen            Generate protobuf stubs for all languages"
	@echo "  proto-gen-go         Generate Go protobuf stubs"
	@echo "  proto-gen-swift      Generate Swift protobuf stubs (macOS visualiser)"
	@echo ""
	@echo "INSTALLATION:"
	@echo "  install-python       Set up Python PDF generator (venv + deps)"
	@echo "  build-texlive-minimal Build local minimal TeX tree for production mode"
	@echo "  build-tex-fmt        Rebuild velocity-report.fmt in local minimal TeX tree"
	@echo "  install-texlive-minimal Install local minimal TeX tree to /opt/velocity-report"
	@echo "  deploy-install-latex Install LaTeX on remote target (for PDF generation)"
	@echo "  deploy-install-latex-minimal Copy local minimal TeX tree to remote target"
	@echo "  validate-tex-minimal Compare report output between full and minimal TeX"
	@echo "  deploy-update-deps   Update source, LaTeX, and Python deps on remote target"
	@echo "  install-web          Install web dependencies (pnpm/npm)"
	@echo "  install-docs         Install docs dependencies (pnpm/npm)"
	@echo ""
	@echo "DEVELOPMENT SERVERS:"
	@echo "  dev-go               Start Go server (radar disabled, precompiled LaTeX)"
	@echo "  dev-go-latex-full    Start Go server (radar disabled, full system LaTeX)"
	@echo "  dev-go-lidar         Start Go server with LiDAR enabled (gRPC mode, precompiled LaTeX)"
	@echo "  dev-go-lidar-both    Start Go server with LiDAR (both gRPC and 2370 forward, precompiled LaTeX)"
	@echo "  dev-go-kill-server   Stop background Go server"
	@echo "  dev-web              Start web dev server"
	@echo "  dev-docs             Start docs dev server"
	@echo "  dev-vis-server       Start visualiser gRPC server (VIS_MODE=synthetic)"
	@echo "                       VIS_MODE: synthetic, replay (requires VIS_LOG), live"
	@echo ""
	@echo "VISUALISER TOOLS:"
	@echo "  record-sample        Generate sample .vrlog file for testing"
	@echo ""
	@echo "TESTING:"
	@echo "  test                 Run all tests (Go + Python + Web + macOS)"
	@echo "  test-go              Run Go unit tests"
	@echo "  test-go-cov          Run Go tests with coverage"
	@echo "  test-go-coverage-summary Show coverage summary for cmd/ and internal/"
	@echo "  test-python          Run Python PDF generator tests"
	@echo "  test-python-cov      Run Python tests with coverage"
	@echo "  test-web             Run web tests (Jest)"
	@echo "  test-web-cov         Run web tests with coverage"
	@echo "  test-mac             Run macOS visualiser tests (XCTest)"
	@echo "  test-mac-cov         Run macOS tests with coverage"
	@echo "  coverage             Generate coverage reports for all components"
	@echo ""
	@echo "DATABASE MIGRATIONS:"
	@echo "  migrate-up           Apply all pending migrations"
	@echo "  migrate-down         Rollback one migration"
	@echo "  migrate-status       Show current migration status"
	@echo "  migrate-detect       Detect schema version (for legacy databases)"
	@echo "  migrate-version      Migrate to specific version (VERSION=N)"
	@echo "  migrate-force        Force version (recovery, VERSION=N)"
	@echo "  migrate-baseline     Set baseline version (VERSION=N)"
	@echo "  schema-sync          Regenerate schema.sql from latest migrations"
	@echo "  schema-erd           Generate schema ERD (entity-relationship diagram) as SVG"
	@echo ""
	@echo "FORMATTING (mutating):"
	@echo "  format               Format all code (Go + Python + Web + macOS + SQL + Markdown)"
	@echo "  format-go            Format Go code (gofmt)"
	@echo "  format-python        Format Python code (black + ruff)"
	@echo "  format-web           Format web code (prettier)"
	@echo "  format-mac           Format macOS Swift code (swift-format)"
	@echo "  format-markdown      Format Markdown files (prettier)"
	@echo "  format-sql           Format SQL files (sql-formatter)"
	@echo ""
	@echo "LINTING (non-mutating, CI-friendly):"
	@echo "  lint                 Lint all code, fail if formatting needed"
	@echo "  lint-go              Check Go formatting"
	@echo "  lint-python          Check Python formatting"
	@echo "  lint-web             Check web formatting"
	@echo "  check-config-order   Validate tuning key order consistency"
	@echo "  sync-config-order    Rewrite tuning sources to canonical key order"
	@echo "  check-config-maths   Validate README.maths keys across docs, tuning JSON, and Go surfaces"
	@echo "  check-config-maths-strict Validate README.maths keys with strict webserver parity"
	@echo ""
	@echo "PDF GENERATOR:"
	@echo "  pdf-check-latex-parity Verify package parity between document builder and format ini"
	@echo "  pdf-report           Generate PDF from config (CONFIG=file.json)"
	@echo "  pdf-config           Create example configuration"
	@echo "  pdf-demo             Run configuration demo"
	@echo "  pdf-test             Run PDF tests (alias for test-python)"
	@echo "  pdf                  Alias for pdf-report"
	@echo "  clean-python         Clean PDF output files"
	@echo ""
	@echo "DEPLOYMENT:"
	@echo "  setup-radar          Install server on this host (requires sudo, legacy)"
	@echo "  deploy-install       Install using velocity-deploy (local)"
	@echo "  deploy-upgrade       Upgrade using velocity-deploy (local)"
	@echo "  deploy-status        Check service status using velocity-deploy"
	@echo "  deploy-health        Run health check using velocity-deploy"
	@echo ""
	@echo "UTILITIES:"
	@echo "  set-version          Update version across codebase (VER=0.4.0 TARGETS='--all')"
	@echo "  log-go-tail          Tail most recent Go server log"
	@echo "  log-go-cat           Cat most recent Go server log"
	@echo "  log-go-tail-all      Tail most recent Go server log plus debug log"
	@echo "  git-fs               Show the git files that differ from main"
	@echo ""
	@echo "DATA VISUALIZATION:"
	@echo "  plot-noise-sweep     Generate noise sweep line plot (FILE=data.csv)"
	@echo "  plot-multisweep      Generate multi-parameter grid (FILE=data.csv)"
	@echo "  plot-noise-buckets   Generate per-noise bar charts (FILE=data.csv)"
	@echo "  stats-live           Capture live LiDAR snapshots"
	@echo "  stats-pcap           Capture PCAP replay snapshots (PCAP=file.pcap)"
	@echo "  profile-macos-lidar  Poll Go/Swift process metrics, then auto-start PCAP replay"
	@echo "  run-pcap-stats       PCAP capture stats — frame rate, RPM (FILE=path)"
	@echo ""
	@echo "API SHORTCUTS (LiDAR HTTP API):"
	@echo "  api-grid-status      Get grid status"
	@echo "  api-grid-reset       Reset background grid"
	@echo "  api-grid-heatmap     Get grid heatmap"
	@echo "  api-snapshot         Get current snapshot"
	@echo "  api-snapshots        List all snapshots"
	@echo "  api-acceptance       Get acceptance metrics"
	@echo "  api-acceptance-reset Reset acceptance counters"
	@echo "  api-params           Get algorithm parameters"
	@echo "  api-params-set       Set parameters (PARAMS='{...}')"
	@echo "  api-persist          Trigger snapshot persistence"
	@echo "  api-export-snapshot  Export specific snapshot"
	@echo "  api-export-next-frame Export next LiDAR frame"
	@echo "  api-status           Get server status"
	@echo "  api-start-pcap       Start PCAP replay (PCAP=file.pcap)"
	@echo "  api-stop-pcap        Stop PCAP replay"
	@echo "  api-switch-data-source Switch live/pcap (SOURCE=live|pcap)"
	@echo ""
	@echo "For detailed information, see README.md or inspect the Makefile"

.DEFAULT_GOAL := help

# =============================================================================
# VERSION INFORMATION
# =============================================================================
VERSION := 0.5.0
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'github.com/banshee-data/velocity.report/internal/version.Version=$(VERSION)' -X 'github.com/banshee-data/velocity.report/internal/version.GitSHA=$(GIT_SHA)' -X 'github.com/banshee-data/velocity.report/internal/version.BuildTime=$(BUILD_TIME)'

# =============================================================================
# BUILD TARGETS (Go cross-compilation)
# =============================================================================

build-radar-linux:
	@./scripts/ensure-web-stub.sh
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o velocity-report-linux-arm64 ./cmd/radar

build-radar-linux-pcap:
	@./scripts/ensure-web-stub.sh
	GOOS=linux GOARCH=arm64 go build -tags=pcap -ldflags "$(LDFLAGS)" -o velocity-report-linux-arm64 ./cmd/radar

build-radar-mac:
	@./scripts/ensure-web-stub.sh
	GOOS=darwin GOARCH=arm64 go build -tags=pcap -ldflags "$(LDFLAGS)" -o velocity-report-mac-arm64 ./cmd/radar

build-radar-mac-intel:
	@./scripts/ensure-web-stub.sh
	GOOS=darwin GOARCH=amd64 go build -tags=pcap -ldflags "$(LDFLAGS)" -o velocity-report-mac-amd64 ./cmd/radar

build-radar-local:
	@./scripts/ensure-web-stub.sh
	go build -tags=pcap -ldflags "$(LDFLAGS)" -o velocity-report-local ./cmd/radar

build-tools:
	go build -o app-sweep ./cmd/sweep

# Build velocity-deploy deployment manager
build-deploy:
	go build -ldflags "$(LDFLAGS)" -o velocity-deploy ./cmd/deploy

build-deploy-linux:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o velocity-deploy-linux-arm64 ./cmd/deploy

.PHONY: build-web
build-web:
	@echo "Building web frontend..."
	@cd web && export PUBLIC_GIT_SHA="$(GIT_SHA)" && export PUBLIC_BUILD_TIME="$(BUILD_TIME)" && if command -v pnpm >/dev/null 2>&1; then \
		pnpm run build; \
	elif command -v npm >/dev/null 2>&1; then \
		npm run build; \
	else \
		echo "pnpm/npm not found; install pnpm (recommended) or npm and retry"; exit 1; \
	fi
	@echo "✓ Web build complete: web/build/"

.PHONY: build-docs
build-docs:
	@echo "Building documentation site..."
	@cd public_html && if command -v pnpm >/dev/null 2>&1; then \
		pnpm run build; \
	elif command -v npm >/dev/null 2>&1; then \
		npm run build; \
	else \
		echo "pnpm/npm not found; install pnpm (recommended) or npm and retry"; exit 1; \
	fi
	@echo "✓ Docs build complete: public_html/_site/"

# Build macOS LiDAR visualiser (requires macOS and Xcode)
VISUALISER_DIR = tools/visualiser-macos
VISUALISER_BUILD_DIR = $(VISUALISER_DIR)/build
VISUALISER_APP = $(VISUALISER_BUILD_DIR)/Build/Products/Release/VelocityVisualiser.app
VISUALISER_BIN = $(VISUALISER_APP)/Contents/MacOS/VelocityVisualiser

.PHONY: build-mac clean-mac run-mac dev-mac

build-mac:
	@echo "Building macOS LiDAR visualiser..."
	@if [ "$$(uname)" != "Darwin" ]; then \
		echo "Error: macOS required for building the visualiser"; \
		exit 1; \
	fi
	@if [ ! -d "$(VISUALISER_DIR)" ]; then \
		echo "Error: Visualiser directory not found: $(VISUALISER_DIR)"; \
		echo ""; \
		echo "The macOS visualiser project hasn't been created yet."; \
		echo "This is a planned feature currently in the design phase."; \
		echo ""; \
		echo "Documentation: docs/lidar/visualiser/"; \
		exit 1; \
	fi
	@if ! command -v xcodebuild >/dev/null 2>&1; then \
		echo "Error: xcodebuild not found. Install Xcode from the App Store."; \
		exit 1; \
	fi
	@dev_dir=$$(xcode-select -p 2>/dev/null); \
	if echo "$$dev_dir" | grep -q "CommandLineTools"; then \
		echo "Error: xcodebuild requires Xcode, but Command Line Tools are active."; \
		echo ""; \
		echo "Solutions:"; \
		echo "  1. Install Xcode from the App Store, then run:"; \
		echo "     sudo xcode-select --switch /Applications/Xcode.app/Contents/Developer"; \
		echo ""; \
		echo "  2. Or if Xcode is already installed but not active:"; \
		echo "     sudo xcode-select --switch /Applications/Xcode.app/Contents/Developer"; \
		echo "     sudo xcodebuild -runFirstLaunch"; \
		echo ""; \
		echo "Current developer directory: $$dev_dir"; \
		exit 1; \
	fi
	@cd $(VISUALISER_DIR) && xcodebuild \
		-project VelocityVisualiser.xcodeproj \
		-scheme VelocityVisualiser \
		-configuration Release \
		-derivedDataPath build \
		build
	@echo "✓ Visualiser build complete: $(VISUALISER_BUILD_DIR)/Build/Products/Release/"

clean-mac:
	@echo "Cleaning macOS visualiser build artifacts..."
	@rm -rf $(VISUALISER_BUILD_DIR)
	@echo "✓ Clean complete"

run-mac:
	@if [ ! -f "$(VISUALISER_BIN)" ]; then \
		echo "Error: Visualiser binary not found. Run 'make build-mac' first."; \
		exit 1; \
	fi
	@echo "Running macOS visualiser..."
	@$(VISUALISER_BIN)

dev-mac:
	@echo "Stopping any running visualiser instances..."
	@pkill -f "VelocityVisualiser" || true
	@sleep 0.5
	@$(MAKE) build-mac
	@echo "Starting visualiser..."
	@$(VISUALISER_BIN)

# =============================================================================
# PROTOBUF CODE GENERATION
# =============================================================================

PROTO_DIR = proto/velocity_visualiser/v1
PROTO_GO_OUT = internal/lidar/visualiser/pb
PROTO_SWIFT_OUT = tools/visualiser-macos/VelocityVisualiser/gRPC/Generated

.PHONY: proto-gen proto-gen-go proto-gen-swift

# Generate protobuf stubs for all languages
proto-gen: proto-gen-go proto-gen-swift
	@echo "✓ Protobuf generation complete"

# Generate Go protobuf stubs
proto-gen-go:
	@echo "Generating Go protobuf stubs..."
	@mkdir -p $(PROTO_GO_OUT)
	@if ! command -v protoc >/dev/null 2>&1; then \
		echo "ERROR: protoc not found; install Protocol Buffers compiler"; \
		echo "  macOS: brew install protobuf"; \
		exit 1; \
	fi
	@if ! command -v protoc-gen-go >/dev/null 2>&1; then \
		echo "ERROR: protoc-gen-go not found; install Go protobuf plugin"; \
		echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"; \
		exit 1; \
	fi
	@if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then \
		echo "ERROR: protoc-gen-go-grpc not found; install Go gRPC plugin"; \
		echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"; \
		exit 1; \
	fi
	@protoc --go_out=$(PROTO_GO_OUT) --go_opt=paths=source_relative \
	       --go-grpc_out=$(PROTO_GO_OUT) --go-grpc_opt=paths=source_relative \
	       -I $(PROTO_DIR) $(PROTO_DIR)/visualiser.proto
	@echo "✓ Go stubs generated in $(PROTO_GO_OUT)"

# Generate Swift protobuf stubs (for macOS visualiser)
proto-gen-swift:
	@echo "Generating Swift protobuf stubs..."
	@mkdir -p $(PROTO_SWIFT_OUT)
	@if ! command -v protoc >/dev/null 2>&1; then \
		echo "ERROR: protoc not found; install Protocol Buffers compiler"; \
		echo "  macOS: brew install protobuf"; \
		exit 1; \
	fi
	@if ! command -v protoc-gen-swift >/dev/null 2>&1; then \
		echo "ERROR: protoc-gen-swift not found; install Swift protobuf plugin"; \
		echo "  macOS: brew install swift-protobuf"; \
		exit 1; \
	fi
	@if ! command -v protoc-gen-grpc-swift-2 >/dev/null 2>&1; then \
		echo "ERROR: protoc-gen-grpc-swift-2 not found; install Swift gRPC v2 plugin"; \
		echo "  Build from source: git clone https://github.com/grpc/grpc-swift-protobuf && cd grpc-swift-protobuf && swift build -c release && sudo cp .build/release/protoc-gen-grpc-swift-2 /usr/local/bin/"; \
		exit 1; \
	fi
	@protoc --swift_out=$(PROTO_SWIFT_OUT) \
	       --plugin=protoc-gen-grpc-swift=`which protoc-gen-grpc-swift-2` \
	       --grpc-swift_out=$(PROTO_SWIFT_OUT) \
	       -I $(PROTO_DIR) $(PROTO_DIR)/visualiser.proto
	@echo "✓ Swift stubs generated in $(PROTO_SWIFT_OUT)"

# =============================================================================
# INSTALLATION
# =============================================================================

.PHONY: install-python install-web install-docs build-texlive-minimal build-tex-fmt install-texlive-minimal deploy-install-latex deploy-install-latex-minimal deploy-update-deps validate-tex-minimal

# Python environment variables (unified at repository root)
VENV_DIR = .venv
VENV_PYTHON = $(VENV_DIR)/bin/python3
VENV_PIP = $(VENV_DIR)/bin/pip
VENV_PYTEST = $(VENV_DIR)/bin/pytest
PDF_DIR = tools/pdf-generator
PDF_OUTPUT_DIR ?= $(PDF_DIR)/output
PYTHON_VERSION = 3.12
TEX_MINIMAL_DIR ?= build/texlive-minimal

# Build: Local minimal TeX tree
build-texlive-minimal:
	@echo "Building minimal TeX tree at $(TEX_MINIMAL_DIR)..."
	@OUTPUT_DIR="$(TEX_MINIMAL_DIR)" ./scripts/build-minimal-texlive.sh

# Build: Rebuild velocity-report.fmt in existing minimal tree
build-tex-fmt:
	@if [ ! -x "$(TEX_MINIMAL_DIR)/bin/xelatex" ]; then \
		echo "Error: $(TEX_MINIMAL_DIR)/bin/xelatex not found."; \
		echo "Run 'make build-texlive-minimal' first."; \
		exit 1; \
	fi
	@echo "Rebuilding velocity-report.fmt in $(TEX_MINIMAL_DIR)..."
	@OUTPUT_DIR="$(TEX_MINIMAL_DIR)" FMT_ONLY=1 ./scripts/build-minimal-texlive.sh

# Install: Copy local minimal TeX tree to /opt/velocity-report
install-texlive-minimal:
	@if [ ! -d "$(TEX_MINIMAL_DIR)" ]; then \
		echo "Error: Minimal TeX tree not found at $(TEX_MINIMAL_DIR)"; \
		echo "Run 'make build-texlive-minimal' first."; \
		exit 1; \
	fi
	@echo "Installing minimal TeX tree from $(TEX_MINIMAL_DIR) to /opt/velocity-report/texlive-minimal..."
	@SOURCE_DIR="$(abspath $(TEX_MINIMAL_DIR))" ./scripts/install-minimal-texlive.sh

# Deploy: Install local minimal TeX tree on remote target
deploy-install-latex-minimal:
	@if [ -z "$(TARGET)" ]; then \
		echo "Error: TARGET not set. Usage: make deploy-install-latex-minimal TARGET=radar-ts"; \
		exit 1; \
	fi
	@if [ ! -d "$(TEX_MINIMAL_DIR)" ]; then \
		echo "Error: Minimal TeX tree not found at $(TEX_MINIMAL_DIR)"; \
		echo "Run 'make build-texlive-minimal' first."; \
		exit 1; \
	fi
	@echo "Deploying minimal TeX tree to $(TARGET):/opt/velocity-report/texlive-minimal..."
	@scp -r "$(TEX_MINIMAL_DIR)" "$(TARGET):/tmp/velocity-report-texlive-minimal"
	@ssh "$(TARGET)" "sudo mkdir -p /opt/velocity-report && sudo rm -rf /opt/velocity-report/texlive-minimal && sudo mv /tmp/velocity-report-texlive-minimal /opt/velocity-report/texlive-minimal && sudo chmod -R a+rX /opt/velocity-report/texlive-minimal"
	@echo "✓ Minimal TeX tree deployed to $(TARGET)"

# Deploy: Install LaTeX on remote target
deploy-install-latex:
	@if [ -z "$(TARGET)" ]; then \
		echo "Error: TARGET not set. Usage: make deploy-install-latex TARGET=radar-ts"; \
		exit 1; \
	fi
	@if [ -d "$(TEX_MINIMAL_DIR)" ]; then \
		echo "Found local minimal TeX tree at $(TEX_MINIMAL_DIR); deploying vendored tree."; \
		$(MAKE) deploy-install-latex-minimal TARGET="$(TARGET)"; \
	else \
		echo "No local minimal TeX tree found; installing TeX Live via apt on $(TARGET)..."; \
		ssh "$(TARGET)" "if ! command -v pdflatex >/dev/null 2>&1; then \
			sudo apt-get update && sudo apt-get install -y texlive-xetex texlive-fonts-recommended texlive-latex-extra; \
		else \
			echo 'LaTeX already installed'; \
		fi"; \
	fi

# Deploy: Update dependencies on remote target
deploy-update-deps:
	@if [ -z "$(TARGET)" ]; then \
		echo "Error: TARGET not set. Usage: make deploy-update-deps TARGET=radar-ts"; \
		exit 1; \
	fi
	@echo "Updating dependencies on $(TARGET)..."
	@echo "  → Updating source code..."
	@ssh $(TARGET) "test -d /opt/velocity-report/.git && cd /opt/velocity-report && sudo git pull || echo 'No git repo found'"
	@echo "  → Ensuring LaTeX is installed..."
	@$(MAKE) --no-print-directory deploy-install-latex TARGET="$(TARGET)"
	@echo "  → Updating Python dependencies..."
	@ssh $(TARGET) "test -d /opt/velocity-report && cd /opt/velocity-report && sudo make install-python || echo 'Source not found'"
	@echo "  → Fixing ownership..."
	@ssh $(TARGET) "test -d /opt/velocity-report && sudo chown -R \$$(sudo systemctl show velocity-report.service -p User --value 2>/dev/null || echo 'velocity'):\$$(sudo systemctl show velocity-report.service -p User --value 2>/dev/null || echo 'velocity') /opt/velocity-report || echo 'Source not found'"
	@echo "✓ Dependencies updated on $(TARGET)"

install-python:
	@echo "Setting up Python environment..."
	@python_path=$$(command -v python$(PYTHON_VERSION) 2>/dev/null || true); \
	if [ -z "$$python_path" ]; then \
		echo "python$(PYTHON_VERSION) not found."; \
		if command -v brew >/dev/null 2>&1; then \
			echo "Attempting to install python@$(PYTHON_VERSION) via Homebrew..."; \
			brew install python@$(PYTHON_VERSION) >/dev/null 2>&1 || true; \
		fi; \
		if ! command -v python$(PYTHON_VERSION) >/dev/null 2>&1; then \
			echo "Please install python$(PYTHON_VERSION) (e.g. 'brew install python@$(PYTHON_VERSION)' or distro package)."; \
			echo "The venv will fall back to the default python3 interpreter."; \
		fi; \
	fi; \
	if command -v python$(PYTHON_VERSION) >/dev/null 2>&1; then \
		python_cmd=python$(PYTHON_VERSION); \
	elif command -v python3 >/dev/null 2>&1; then \
		python_cmd=python3; \
	else \
		python_cmd=python; \
	fi; \
	if ! command -v "$$python_cmd" >/dev/null 2>&1; then \
		echo "No usable Python interpreter found (python$(PYTHON_VERSION)/python3)."; \
		exit 1; \
	fi; \
	echo "Using: $$python_cmd"; \
	if [ -d "$(VENV_DIR)" ]; then \
		existing_version=$$($(VENV_DIR)/bin/python3 --version 2>&1 || true); \
		if echo "$$existing_version" | grep -q "Python $(PYTHON_VERSION)"; then \
			echo "Reusing existing venv at $(VENV_DIR) ($$existing_version)"; \
		else \
			echo "Recreating venv with $$python_cmd (was $$existing_version)"; \
			rm -rf $(VENV_DIR); \
			$$python_cmd -m venv $(VENV_DIR); \
		fi; \
	else \
		$$python_cmd -m venv $(VENV_DIR); \
	fi
	@$(VENV_PIP) install --upgrade pip
	@$(VENV_PIP) install -r requirements.txt
	@echo "✓ Python environment ready at $(VENV_DIR)"
	@echo ""
	@echo "Activate with: source $(VENV_DIR)/bin/activate"

install-web:
	@echo "Installing web dependencies..."
	@cd web && if command -v pnpm >/dev/null 2>&1; then \
		pnpm install --frozen-lockfile; \
		elif command -v npm >/dev/null 2>&1; then \
			npm install; \
		else \
			echo "pnpm/npm not found; install pnpm (recommended) or npm and retry"; exit 1; \
		fi

install-docs:
	@echo "Installing docs dependencies..."
	@cd public_html && if command -v pnpm >/dev/null 2>&1; then \
		pnpm install --frozen-lockfile; \
		elif command -v npm >/dev/null 2>&1; then \
			npm install; \
		else \
			echo "pnpm/npm not found; install pnpm (recommended) or npm and retry"; exit 1; \
		fi

.PHONY: ensure-python-tools
ensure-python-tools:
	@if [ ! -d "$(VENV_DIR)" ] || [ ! -x "$(VENV_DIR)/bin/black" ] || [ ! -x "$(VENV_DIR)/bin/ruff" ]; then \
		$(MAKE) install-python; \
	fi

# =============================================================================
# DEVELOPMENT SERVERS
# =============================================================================

.PHONY: dev-go dev-go-latex-full dev-go-lidar dev-go-lidar-both dev-go-kill-server dev-web dev-docs dev-vis-server record-sample

# Reusable script for starting the app in background. Call with extra flags
# using '$(call run_dev_go,<extra-flags>)'. Uses shell $$ variables so we
# escape $ to $$ inside the define so the resulting shell script receives
# single-dollar variables.
define run_dev_go
	mkdir -p logs; \
	ts=$$(date +%Y%m%d-%H%M%S); \
	logfile=logs/velocity-$${ts}.log; \
	debuglog=logs/velocity-debug-$${ts}.log; \
	piddir=logs/pids; \
	pidfile=$${piddir}/velocity-$${ts}.pid; \
	DB_PATH=$${DB_PATH:-./sensor_data.db}; \
	$(call run_dev_go_kill_server); \
	echo "Building velocity-report-local..."; \
	go build -tags=pcap -ldflags "$(LDFLAGS)" -o velocity-report-local ./cmd/radar; \
	mkdir -p "$$piddir"; \
	echo "Starting velocity-report-local (background) with DB=$$DB_PATH -> $$logfile (debug -> $$debuglog)"; \
	VELOCITY_REPORT_ENABLE_DESTRUCTIVE_LIDAR_API=1 VELOCITY_DEBUG_LOG="$$debuglog" nohup ./velocity-report-local --disable-radar $(1) --db-path="$$DB_PATH" >> "$$logfile" 2>&1 & echo $$! > "$$pidfile"; \
	echo "Started; PID $$(cat $$pidfile)"
endef

define run_dev_go_require_precompiled_root
	tex_root="$(abspath $(TEX_MINIMAL_DIR))"; \
	if [ ! -x "$$tex_root/bin/xelatex" ]; then \
		echo "Error: precompiled TeX flow requested but $$tex_root/bin/xelatex not found."; \
		echo "Run 'make build-texlive-minimal' first, or use 'make dev-go-latex-full'."; \
		exit 1; \
	fi; \
	if [ ! -f "$$tex_root/texmf-dist/web2c/xelatex/xelatex.fmt" ]; then \
		echo "Error: precompiled TeX flow requested but $$tex_root/texmf-dist/web2c/xelatex/xelatex.fmt not found."; \
		echo "Run 'make build-tex-fmt' (or rebuild via 'make build-texlive-minimal'), or use 'make dev-go-latex-full'."; \
		exit 1; \
	fi
endef

define run_dev_go_kill_server
	piddir=logs/pids; \
	echo "Stopping previously-launched velocity-report-local processes (from $$piddir) ..."; \
	if [ -d "$$piddir" ] && [ $$(ls -1 $$piddir/velocity-*.pid 2>/dev/null | wc -l) -gt 0 ]; then \
	  for pidfile_k in $$(ls -1t $$piddir/velocity-*.pid 2>/dev/null | head -n3); do \
	    pid_k=$$(cat "$$pidfile_k" 2>/dev/null || echo); \
	    if [ -n "$$pid_k" ] && kill -0 $$pid_k 2>/dev/null; then \
	      cmdline=$$(ps -p $$pid_k -o args= 2>/dev/null || true); \
	      case "$$cmdline" in \
	        *velocity-report-local*) \
	          echo "Stopping pid $$pid_k (from $$pidfile_k): $$cmdline"; \
	          kill $$pid_k 2>/dev/null || true; \
	          sleep 1; \
	          kill -0 $$pid_k 2>/dev/null && kill -9 $$pid_k 2>/dev/null || true; \
	          ;; \
	        *) echo "Skipping pid $$pid_k (cmd does not match velocity-report-local): $$cmdline"; ;; \
	      esac; \
	    fi; \
	  done; \
	fi
endef

DEV_GO_LATEX_PRECOMPILED_FLAGS := --pdf-latex-flow=precompiled --pdf-tex-root="$(abspath $(TEX_MINIMAL_DIR))"
DEV_GO_LATEX_FULL_FLAGS := --pdf-latex-flow=full

dev-go:
	@$(call run_dev_go_require_precompiled_root)
	@$(call run_dev_go,$(DEV_GO_LATEX_PRECOMPILED_FLAGS))

dev-go-latex-full:
	@$(call run_dev_go,$(DEV_GO_LATEX_FULL_FLAGS))

dev-go-lidar:
	@$(call run_dev_go_require_precompiled_root)
	@$(call run_dev_go,$(DEV_GO_LATEX_PRECOMPILED_FLAGS) --enable-transit-worker=false --enable-lidar --lidar-forward --lidar-forward-mode=grpc)

dev-go-lidar-both:
	@$(call run_dev_go_require_precompiled_root)
	@$(call run_dev_go,$(DEV_GO_LATEX_PRECOMPILED_FLAGS) --enable-transit-worker=false --enable-lidar --lidar-forward --lidar-foreground-forward --lidar-forward-mode=both)

dev-go-kill-server:
	@$(call run_dev_go_kill_server)

dev-web:
	@echo "Starting web dev server..."
	@cd web && if command -v pnpm >/dev/null 2>&1; then \
		pnpm run dev; \
		elif command -v npm >/dev/null 2>&1; then \
		npm run dev; \
		else \
			echo "pnpm/npm not found; install dependencies (pnpm install) and run 'pnpm run dev'"; exit 1; \
		fi

dev-docs:
	@echo "Starting docs dev server..."
	@cd public_html && if command -v pnpm >/dev/null 2>&1; then \
		pnpm run dev; \
		elif command -v npm >/dev/null 2>&1; then \
		npm run dev; \
		else \
			echo "pnpm/npm not found; install dependencies (pnpm install) and run 'pnpm run dev'"; exit 1; \
		fi

# Visualiser server mode: synthetic (default), replay, live
# Examples:
#   make dev-vis-server                                          # synthetic mode
#   make dev-vis-server VIS_MODE=replay VIS_LOG=/path/to/log    # replay mode
VIS_MODE ?= synthetic
VIS_LOG ?=

dev-vis-server:
	@echo "Starting visualiser gRPC server (mode: $(VIS_MODE))..."
	@if [ "$(VIS_MODE)" = "replay" ] && [ -z "$(VIS_LOG)" ]; then \
		echo "Error: VIS_LOG required for replay mode. Usage: make dev-vis-server VIS_MODE=replay VIS_LOG=/path/to/recording.vrlog"; \
		exit 1; \
	fi
	@if [ "$(VIS_MODE)" = "replay" ]; then \
		go run ./cmd/tools/visualiser-server -addr localhost:50051 -mode replay -log "$(VIS_LOG)"; \
	else \
		go run ./cmd/tools/visualiser-server -addr localhost:50051 -mode $(VIS_MODE); \
	fi

# Record a sample .vrlog file for testing replay mode
# Variables: RECORD_OUTPUT (default: sample.vrlog), RECORD_FRAMES (default: 100)
# Usage: make record-sample RECORD_FRAMES=50 RECORD_OUTPUT=test.vrlog
RECORD_OUTPUT ?= sample.vrlog
RECORD_FRAMES ?= 100

record-sample:
	@echo "Recording sample .vrlog file..."
	@echo "Output: $(RECORD_OUTPUT)"
	@echo "Frames: $(RECORD_FRAMES)"
	go run ./cmd/tools/gen-vrlog -o $(RECORD_OUTPUT) -n $(RECORD_FRAMES)

# =============================================================================
# TESTING
# =============================================================================

.PHONY: test test-go test-go-cov test-go-coverage-summary test-python test-python-cov test-web test-web-cov test-mac test-mac-cov coverage

WEB_DIR = web
MAC_DIR = tools/visualiser-macos

# Aggregate test target: runs Go, web, Python, and macOS tests in sequence
test: test-go test-web test-python test-mac

# Run Go unit tests for the whole repository
test-go:
	@echo "Running Go unit tests..."
	@go test ./...

# Run Go unit tests with coverage
test-go-cov:
	@echo "Running Go unit tests with coverage..."
	@go test ./... -coverprofile=coverage.out -covermode=atomic
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Show coverage summary for cmd/ and internal/ packages
test-go-coverage-summary:
	@echo "Computing Go coverage by directory..."
	@go test -cover ./cmd/... 2>/dev/null | awk '/^ok.*coverage:/ {gsub(/%/, "", $$5); sum+=$$5; count++} END {if (count>0) printf "cmd/      coverage: %.1f%%\n", sum/count; else print "cmd/      coverage: 0.0%"}'
	@go test -cover ./internal/... 2>/dev/null | awk '/^ok.*coverage:/ {gsub(/%/, "", $$5); sum+=$$5; count++} END {if (count>0) printf "internal/ coverage: %.1f%%\n", sum/count; else print "internal/ coverage: 0.0%"}'

# Run Python tests for the PDF generator. Ensures venv is setup first.
test-python:
	@echo "Running Python (PDF generator) tests..."
	@$(MAKE) install-python
	@$(MAKE) pdf-test

test-python-cov:
	@echo "Running PDF generator tests with coverage..."
	cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTEST) --cov=pdf_generator --cov-report=html --cov-report=xml pdf_generator/tests/
	@echo "Coverage report: $(PDF_DIR)/htmlcov/index.html"

# Run web test suite (Jest) using pnpm inside the web directory
test-web:
	@echo "Running web (Jest) tests..."
	@cd $(WEB_DIR) && pnpm run test:ci

# Run web tests with coverage
test-web-cov:
	@echo "Running web (Jest) tests with coverage..."
	@cd $(WEB_DIR) && pnpm run test:coverage
	@echo "Coverage report: $(WEB_DIR)/coverage/lcov-report/index.html"

# Run macOS visualiser tests (XCTest)
test-mac:
	@echo "Running macOS visualiser tests..."
	@if [ "$$(uname)" != "Darwin" ]; then \
		echo "Skipping macOS tests (not on macOS)"; \
		exit 0; \
	fi
	@if [ ! -d "$(MAC_DIR)" ]; then \
		echo "Skipping macOS tests (project not found)"; \
		exit 0; \
	fi
	@if ! command -v xcodebuild >/dev/null 2>&1; then \
		echo "Skipping macOS tests (xcodebuild not found)"; \
		exit 0; \
	fi
	@cd $(MAC_DIR) && xcodebuild test \
		-project VelocityVisualiser.xcodeproj \
		-scheme VelocityVisualiser \
		-destination 'platform=macOS'

# Run macOS visualiser tests with coverage
# Coverage results are written to $(MAC_DIR)/coverage/
test-mac-cov:
	@echo "Running macOS visualiser tests with coverage..."
	@if [ "$$(uname)" != "Darwin" ]; then \
		echo "Skipping macOS coverage (not on macOS)"; \
		exit 0; \
	fi
	@if [ ! -d "$(MAC_DIR)" ]; then \
		echo "Skipping macOS coverage (project not found)"; \
		exit 0; \
	fi
	@if ! command -v xcodebuild >/dev/null 2>&1; then \
		echo "Skipping macOS coverage (xcodebuild not found)"; \
		exit 0; \
	fi
	@mkdir -p $(MAC_DIR)/coverage
	@cd $(MAC_DIR) && xcodebuild test \
		-project VelocityVisualiser.xcodeproj \
		-scheme VelocityVisualiser \
		-destination 'platform=macOS' \
		-enableCodeCoverage YES \
		-derivedDataPath ./DerivedData \
		-resultBundlePath ./coverage/TestResults.xcresult
	@echo ""
	@echo "Coverage data generated in $(MAC_DIR)/coverage/TestResults.xcresult"
	@echo "View in Xcode: open $(MAC_DIR)/coverage/TestResults.xcresult"
	@echo ""
	@# Extract coverage summary using xcrun xccov
	@if command -v xcrun >/dev/null 2>&1; then \
		echo "Coverage Summary:"; \
		xcrun xccov view --report $(MAC_DIR)/coverage/TestResults.xcresult 2>/dev/null | head -20 || echo "(xcrun xccov not available)"; \
	fi

# Generate coverage reports for all components
coverage: test-go-cov test-python-cov test-web-cov test-mac-cov
	@echo ""
	@echo "✓ All coverage reports generated:"
	@echo "  - Go:     coverage.html"
	@echo "  - Python: $(PDF_DIR)/htmlcov/index.html"
	@echo "  - Web:    $(WEB_DIR)/coverage/lcov-report/index.html"
	@echo "  - macOS:  $(MAC_DIR)/coverage/TestResults.xcresult"

# Run performance regression test
test-perf:
	@NAME="$${NAME:-kirk0}"; \
	BASE_NAME="$${NAME%.*}"; \
	echo "Target: $$BASE_NAME"; \
	if [ -f "internal/lidar/perf/pcap/$$BASE_NAME.pcapng" ]; then \
		PCAP_FILE="internal/lidar/perf/pcap/$$BASE_NAME.pcapng"; \
	elif [ -f "internal/lidar/perf/pcap/$$BASE_NAME.pcap" ]; then \
		PCAP_FILE="internal/lidar/perf/pcap/$$BASE_NAME.pcap"; \
	else \
		echo "Error: PCAP file not found for $$BASE_NAME (.pcap or .pcapng)"; \
		exit 1; \
	fi; \
	if [ "$$CI" = "true" ]; then \
		BASELINE_FILE="internal/lidar/perf/baseline/baseline-$$BASE_NAME-ci.json"; \
	else \
		BASELINE_FILE="internal/lidar/perf/baseline/baseline-$$BASE_NAME.json"; \
	fi; \
	echo "Building pcap-analyse..."; \
	go build -tags=pcap -o pcap-analyse ./cmd/tools/pcap-analyse; \
	EXIT_CODE=0; \
	if [ ! -f "$$BASELINE_FILE" ]; then \
		echo "Baseline not found at $$BASELINE_FILE. Creating new baseline..."; \
		./pcap-analyse -pcap "$$PCAP_FILE" -benchmark -benchmark-output "$$BASELINE_FILE"; \
		echo "Created baseline: $$BASELINE_FILE"; \
	else \
		echo "Running performance comparison against $$BASELINE_FILE..."; \
		./pcap-analyse -pcap "$$PCAP_FILE" -benchmark -compare-baseline "$$BASELINE_FILE" -quiet || EXIT_CODE=$$?; \
	fi; \
	rm -f pcap-analyse *_analysis.json *_benchmark.json; \
	exit $$EXIT_CODE

# =============================================================================
# DATABASE MIGRATIONS
# =============================================================================

.PHONY: migrate-up migrate-down migrate-status migrate-detect migrate-version migrate-force migrate-baseline schema-sync schema-erd

# Apply all pending migrations
migrate-up:
	@echo "Applying all pending migrations..."
	@go run ./cmd/radar migrate up

# Rollback one migration
migrate-down:
	@echo "Rolling back one migration..."
	@go run ./cmd/radar migrate down

# Show current migration status
migrate-status:
	@go run ./cmd/radar migrate status

# Detect schema version (for legacy databases)
migrate-detect:
	@echo "Detecting schema version..."
	@go run ./cmd/radar migrate detect

# Migrate to a specific version (usage: make migrate-version VERSION=3)
migrate-version:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION not specified"; \
		echo "Usage: make migrate-version VERSION=3"; \
		exit 1; \
	fi
	@echo "Migrating to version $(VERSION)..."
	@go run ./cmd/radar migrate version $(VERSION)

# Force migration version (recovery only, usage: make migrate-force VERSION=2)
migrate-force:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION not specified"; \
		echo "Usage: make migrate-force VERSION=2"; \
		exit 1; \
	fi
	@echo "Forcing migration version to $(VERSION)..."
	@go run ./cmd/radar migrate force $(VERSION)

# Baseline database at version (usage: make migrate-baseline VERSION=6)
migrate-baseline:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION not specified"; \
		echo "Usage: make migrate-baseline VERSION=6"; \
		exit 1; \
	fi
	@echo "Baselining database at version $(VERSION)..."
	@go run ./cmd/radar migrate baseline $(VERSION)

# Regenerate schema.sql from latest migrations
schema-sync:
	@echo "Regenerating schema.sql from latest migrations..."
	@bash scripts/sync-schema.sh

# Generate schema ERD (Entity-Relationship Diagram) as SVG
schema-erd:
	@echo "Generating schema ERD (schema.svg)..."
	@bash data/sqlite-erd/graph.sh internal/db/schema.sql

# =============================================================================
# FORMATTING (mutating)
# =============================================================================

.PHONY: format format-go format-python format-web format-mac format-markdown format-sql

format: format-go format-python format-web format-mac format-markdown format-sql
	@echo "\nAll formatting targets complete."

format-go:
	@echo "Formatting Go source (gofmt)..."
	@gofmt -s -w . || true

format-python: ensure-python-tools
	@echo "Formatting Python (black, ruff) using venv at $(VENV_DIR)..."
	@$(VENV_PYTHON) -m black . || true
	@$(VENV_PYTHON) -m ruff check --fix . || true

format-web:
	@echo "Formatting web JS/TS in $(WEB_DIR) (prettier via pnpm or npx)..."
	@if [ -d "$(WEB_DIR)" ]; then \
		if command -v pnpm >/dev/null 2>&1; then \
			cd $(WEB_DIR) && pnpm run prettier:write || echo "prettier run failed or not configured"; \
		elif command -v npx >/dev/null 2>&1; then \
			cd $(WEB_DIR) && npx prettier --write . || echo "prettier run failed or not configured"; \
		else \
			echo "pnpm/npx not found; skipping JS/TS formatting in $(WEB_DIR)"; \
		fi; \
	else \
		echo "$(WEB_DIR) does not exist; skipping web formatting"; \
	fi
	@if [ -d "$(WEB_DIR)" ]; then \
		cd $(WEB_DIR) && pnpm exec prettier --write \
			../internal/lidar/monitor/assets/dashboard_common.js \
			../internal/lidar/monitor/assets/regions_dashboard.js \
			../internal/lidar/monitor/assets/sweep_dashboard.js \
			../internal/lidar/monitor/assets/common.css \
			../internal/lidar/monitor/assets/status_dashboard.css \
			../internal/lidar/monitor/assets/dashboard.css \
			../internal/lidar/monitor/assets/regions_dashboard.css \
			../internal/lidar/monitor/assets/sweep_dashboard.css \
			2>/dev/null \
			|| echo "monitor assets prettier skipped"; \
	fi

format-mac:
	@echo "Formatting macOS Swift code (swift-format)..."
	@if [ "$$(uname)" != "Darwin" ]; then \
		echo "Skipping macOS formatting (not on macOS)"; \
		exit 0; \
	fi
	@if [ ! -d "$(MAC_DIR)" ]; then \
		echo "Skipping macOS formatting (project not found)"; \
		exit 0; \
	fi
	@if command -v swift-format >/dev/null 2>&1; then \
		cd $(MAC_DIR) && find VelocityVisualiser -name '*.swift' -exec swift-format -i --configuration .swift-format {} \; ; \
		echo "✓ Swift formatting complete"; \
	else \
		echo "swift-format not found; install via: brew install swift-format"; \
		echo "Skipping macOS formatting"; \
	fi

format-markdown:
	@echo "Formatting Markdown files with prettier..."
	@if [ -d "$(WEB_DIR)" ]; then \
		if command -v pnpm >/dev/null 2>&1; then \
			cd $(WEB_DIR) && pnpm exec prettier --write "../**/*.md" --ignore-path ../.prettierignore || echo "prettier failed"; \
		elif command -v npx >/dev/null 2>&1; then \
			cd $(WEB_DIR) && npx prettier --write "../**/*.md" --ignore-path ../.prettierignore || echo "prettier failed"; \
		else \
			echo "pnpm/npx not found; skipping Markdown formatting"; \
		fi; \
	else \
		echo "$(WEB_DIR) does not exist; skipping Markdown formatting"; \
	fi

format-sql:
	@echo "Formatting SQL files with sql-formatter..."
	@bash scripts/format-sql.sh

# =============================================================================
# LINTING (non-mutating, CI-friendly)
# =============================================================================

.PHONY: lint lint-go lint-python lint-web

lint: lint-go lint-python lint-web
	@echo "\nAll lint checks passed."

.PHONY: check-config-order sync-config-order config-order-check config-order-sync

check-config-order:
	@./scripts/config-order-sync \
		--main-go-struct internal/config/tuning.go:TuningConfig \
		--discover \
		--md-target config/README.md \
		--check

sync-config-order:
	@./scripts/config-order-sync \
		--main-json config/tuning.defaults.json \
		--discover \
		--md-target config/README.md
	@./scripts/config-order-sync \
		--main-go-struct internal/config/tuning.go:TuningConfig \
		--discover \
		--md-target config/README.md

config-order-check: check-config-order

config-order-sync: sync-config-order

.PHONY: check-config-maths check-config-maths-strict readme-maths-check readme-maths-check-strict

check-config-maths:
	@./scripts/readme-maths-check

check-config-maths-strict:
	@./scripts/readme-maths-check --webserver-mode exact

readme-maths-check: check-config-maths

readme-maths-check-strict: check-config-maths-strict

lint-go:
	@echo "Checking Go formatting (gofmt -l)..."
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "The following Go files are not properly formatted:"; \
		echo "$$files"; \
		exit 1; \
	else \
		echo "OK"; \
	fi

lint-python:
	@echo "Checking Python formatting (black --check, ruff)..."
	@if [ -x "$(VENV_DIR)/bin/black" ]; then \
		"$(VENV_DIR)/bin/black" --check .; \
	elif command -v black >/dev/null 2>&1; then \
		black --check .; \
	else \
		echo "black not found; install it with 'make install-python'"; \
		exit 2; \
	fi
	@if [ -x "$(VENV_DIR)/bin/ruff" ]; then \
		"$(VENV_DIR)/bin/ruff" check .; \
	elif command -v ruff >/dev/null 2>&1; then \
		ruff check .; \
	else \
		echo "ruff not found; install it with 'make install-python'"; \
		exit 2; \
	fi

lint-web:
	@echo "Checking web formatting & lint (prettier + eslint) in $(WEB_DIR)..."
	@if [ -d "$(WEB_DIR)" ]; then \
		if command -v pnpm >/dev/null 2>&1; then \
			cd $(WEB_DIR) && pnpm run lint || exit 1; \
		elif command -v npx >/dev/null 2>&1; then \
			(cd $(WEB_DIR) && npx prettier --check . && npx eslint .) || exit 1; \
		else \
			echo "pnpm/npx not found; cannot run prettier --check"; \
			exit 2; \
		fi; \
	else \
		echo "$(WEB_DIR) does not exist; skipping web format check"; \
	fi

# =============================================================================
# PDF GENERATOR
# =============================================================================

.PHONY: pdf-check-latex-parity pdf-test validate-tex-minimal pdf-report pdf-config pdf-demo pdf clean-python

pdf-check-latex-parity:
	@echo "Checking LaTeX package parity..."
	cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTHON) scripts/check_latex_package_parity.py

pdf-test: pdf-check-latex-parity
	@echo "Running PDF generator tests..."
	cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTEST) pdf_generator/tests/

validate-tex-minimal:
	@config_value="$(CONFIG)"; \
	if [ -z "$$config_value" ]; then \
		config_value="$(PDF_DIR)/config.minimal.json"; \
		echo "CONFIG not set; defaulting to $$config_value"; \
	fi; \
	if [ -f "$$config_value" ]; then \
		config_path="$$(cd "$$(dirname "$$config_value")" && pwd)/$$(basename "$$config_value")"; \
	elif [ -f "$(PDF_DIR)/$$config_value" ]; then \
		config_path="$$(cd "$(PDF_DIR)" && pwd)/$$config_value"; \
	else \
		echo "Error: Config file not found: $$config_value"; \
		echo "Usage: make validate-tex-minimal CONFIG=config.json"; \
		exit 1; \
	fi; \
	if [ ! -d "$(TEX_MINIMAL_DIR)" ]; then \
		echo "Error: Minimal TeX tree not found at $(TEX_MINIMAL_DIR)"; \
		echo "Run 'make build-texlive-minimal' first."; \
		exit 1; \
	fi; \
	if [ ! -f "$(TEX_MINIMAL_DIR)/texmf-dist/web2c/xelatex/xelatex.fmt" ]; then \
		echo "Error: precompiled flow requested but $(TEX_MINIMAL_DIR)/texmf-dist/web2c/xelatex/xelatex.fmt not found."; \
		echo "Run 'make build-tex-fmt' (or rebuild via 'make build-texlive-minimal')."; \
		exit 1; \
	fi; \
	api_base="$${API_BASE_URL:-http://localhost:8080}"; \
	if command -v curl >/dev/null 2>&1; then \
		if ! curl -fsS --max-time 5 "$$api_base/health" >/dev/null 2>&1; then \
			echo "Error: API health check failed at $$api_base/health"; \
			echo "Start the backend first (precompiled: make dev-go, full TeX: make dev-go-latex-full)."; \
			echo "Or set API_BASE_URL to a reachable instance."; \
			exit 1; \
		fi; \
	else \
		echo "Warning: curl not found; skipping API health check"; \
	fi; \
	tmp_dir=$$(mktemp -d); \
	ref_pdf="$$tmp_dir/reference.pdf"; \
	min_pdf="$$tmp_dir/minimal.pdf"; \
	ref_stamp="$$tmp_dir/reference.stamp"; \
	min_stamp="$$tmp_dir/minimal.stamp"; \
	touch "$$ref_stamp"; \
	echo "Generating reference report (development mode)..."; \
	if ! $(MAKE) --no-print-directory pdf-report CONFIG="$$config_path"; then \
		echo "Error: reference report generation failed."; \
		echo "Artifacts kept in $$tmp_dir"; \
		exit 1; \
	fi; \
	ref_candidates="$$(find "$(PDF_DIR)" -type f -name '*_report.pdf' -newer "$$ref_stamp" 2>/dev/null)"; \
	if [ -z "$$ref_candidates" ]; then \
		echo "Error: No new *_report.pdf found under $(PDF_DIR) for the reference run."; \
		echo "Artifacts kept in $$tmp_dir"; \
		exit 1; \
	fi; \
	ref_latest=$$(printf '%s\n' "$$ref_candidates" | xargs ls -t | head -n1); \
	cp "$$ref_latest" "$$ref_pdf"; \
	touch "$$min_stamp"; \
	echo "Generating report using minimal TeX tree (production mode)..."; \
	if ! VELOCITY_TEX_ROOT="$$(cd "$(TEX_MINIMAL_DIR)" && pwd)" $(MAKE) --no-print-directory pdf-report CONFIG="$$config_path"; then \
		echo "Error: minimal-tree report generation failed."; \
		echo "Artifacts kept in $$tmp_dir"; \
		exit 1; \
	fi; \
	min_candidates="$$(find "$(PDF_DIR)" -type f -name '*_report.pdf' -newer "$$min_stamp" 2>/dev/null)"; \
	if [ -z "$$min_candidates" ]; then \
		echo "Error: No new *_report.pdf found under $(PDF_DIR) for the minimal run."; \
		echo "Artifacts kept in $$tmp_dir"; \
		exit 1; \
	fi; \
	min_latest=$$(printf '%s\n' "$$min_candidates" | xargs ls -t | head -n1); \
	cp "$$min_latest" "$$min_pdf"; \
	if command -v pdfinfo >/dev/null 2>&1; then \
		ref_pages=$$(pdfinfo "$$ref_pdf" | awk -F: '/^Pages:/ {gsub(/ /, "", $$2); print $$2}'); \
		min_pages=$$(pdfinfo "$$min_pdf" | awk -F: '/^Pages:/ {gsub(/ /, "", $$2); print $$2}'); \
		if [ "$$ref_pages" != "$$min_pages" ]; then \
			echo "Error: Page count mismatch (reference=$$ref_pages, minimal=$$min_pages)"; \
			echo "Artifacts kept in $$tmp_dir"; \
			exit 1; \
		fi; \
	else \
		echo "Warning: pdfinfo not found; skipping page count check"; \
	fi; \
	if command -v pdftotext >/dev/null 2>&1; then \
		pdftotext "$$ref_pdf" "$$tmp_dir/reference.txt"; \
		pdftotext "$$min_pdf" "$$tmp_dir/minimal.txt"; \
		if ! diff -u "$$tmp_dir/reference.txt" "$$tmp_dir/minimal.txt" > "$$tmp_dir/text.diff"; then \
			echo "Error: Extracted text differs (see $$tmp_dir/text.diff)"; \
			echo "Artifacts kept in $$tmp_dir"; \
			exit 1; \
		fi; \
	else \
		echo "Warning: pdftotext not found; skipping text diff"; \
	fi; \
	if command -v diff-pdf >/dev/null 2>&1; then \
		if ! diff-pdf "$$ref_pdf" "$$min_pdf" >/dev/null 2>&1; then \
			echo "Error: Visual PDF diff detected"; \
			diff-pdf --output-diff="$$tmp_dir/visual-diff.pdf" "$$ref_pdf" "$$min_pdf" >/dev/null 2>&1 || true; \
			echo "Artifacts kept in $$tmp_dir"; \
			exit 1; \
		fi; \
	else \
		echo "Warning: diff-pdf not found; skipping visual comparison"; \
	fi; \
	echo "✓ Minimal TeX validation passed"; \
	rm -rf "$$tmp_dir"

pdf-report:
	@if [ -z "$(CONFIG)" ]; then \
		echo "Error: CONFIG required. Usage: make pdf-report CONFIG=config.json"; \
		exit 1; \
	fi
	@if [ -f "$(CONFIG)" ]; then \
		CONFIG_PATH="$$(cd $$(dirname "$(CONFIG)") && pwd)/$$(basename "$(CONFIG)")"; \
	elif [ -f "$(PDF_DIR)/$(CONFIG)" ]; then \
		CONFIG_PATH="$(CONFIG)"; \
	else \
		echo "Error: Config file not found: $(CONFIG)"; \
		echo "Try: make pdf-report CONFIG=config.example.json"; \
		exit 1; \
	fi; \
	cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTHON) -m pdf_generator.cli.main $$CONFIG_PATH

pdf-config:
	@echo "Creating example configuration..."
	cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTHON) -m pdf_generator.cli.create_config

pdf-demo:
	@echo "Running configuration system demo..."
	cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTHON) -m pdf_generator.cli.demo

# Convenience alias
pdf: pdf-report

clean-python:
	@echo "Cleaning PDF generator outputs..."
	rm -rf $(PDF_DIR)/output/*.pdf
	rm -rf $(PDF_DIR)/output/*.tex
	rm -rf $(PDF_DIR)/output/*.svg
	rm -rf $(PDF_DIR)/.pytest_cache
	rm -rf $(PDF_DIR)/htmlcov
	rm -rf $(PDF_DIR)/.coverage
	rm -rf $(PDF_DIR)/pdf_generator/**/__pycache__
	@echo "✓ Cleaned"

# =============================================================================
# DEPLOYMENT
# =============================================================================

.PHONY: setup-radar deploy-install deploy-upgrade deploy-status deploy-health

# Legacy installation script (kept for backward compatibility)
setup-radar:
	@if [ ! -f "velocity-report-linux-arm64" ]; then \
		echo "Error: velocity-report-linux-arm64 not found!"; \
		echo "Run 'make build-radar-linux' first."; \
		exit 1; \
	fi
	@echo "Setting up velocity.report server on this host..."
	@echo "This will:"
	@echo "  1. Install binary to /usr/local/bin/velocity-report"
	@echo "  2. Create service user and working directory"
	@echo "  3. Install and start systemd service"
	@echo ""
	@sudo ./scripts/setup-radar-host.sh

# Modern deployment using velocity-deploy
deploy-install:
	@if [ ! -f "velocity-deploy" ]; then \
		echo "Building velocity-deploy..."; \
		make build-deploy; \
	fi
	@if [ ! -f "velocity-report-linux-arm64" ]; then \
		echo "Error: velocity-report-linux-arm64 not found!"; \
		echo "Run 'make build-radar-linux' first."; \
		exit 1; \
	fi
	@echo "Installing velocity.report using velocity-deploy..."
	./velocity-deploy install --binary ./velocity-report-linux-arm64

deploy-upgrade:
	@if [ ! -f "velocity-deploy" ]; then \
		echo "Building velocity-deploy..."; \
		make build-deploy; \
	fi
	@if [ ! -f "velocity-report-linux-arm64" ]; then \
		echo "Error: velocity-report-linux-arm64 not found!"; \
		echo "Run 'make build-radar-linux' first."; \
		exit 1; \
	fi
	@echo "Upgrading velocity.report using velocity-deploy..."
	./velocity-deploy upgrade --binary ./velocity-report-linux-arm64

deploy-status:
	@if [ ! -f "velocity-deploy" ]; then \
		echo "Building velocity-deploy..."; \
		make build-deploy; \
	fi
	./velocity-deploy status

deploy-health:
	@if [ ! -f "velocity-deploy" ]; then \
		echo "Building velocity-deploy..."; \
		make build-deploy; \
	fi
	./velocity-deploy health

# =============================================================================
# UTILITIES
# =============================================================================

.PHONY: set-version log-go-tail log-go-cat log-go-tail-all git-fs

set-version:
	@if [ -z "$(VER)" ]; then \
		echo "Usage: make set-version VER=<version> TARGETS='<targets>'"; \
		echo ""; \
		echo "Example: make set-version VER=0.4.0-pre2 TARGETS='--all'"; \
		echo "         make set-version VER=0.5.0 TARGETS='--makefile --deploy --pdf'"; \
		echo ""; \
		./scripts/set-version.sh; \
		exit 1; \
	fi
	@if [ -z "$(TARGETS)" ]; then \
		echo "Error: TARGETS not specified"; \
		echo "Usage: make set-version VER=<version> TARGETS='<targets>'"; \
		echo "Example: make set-version VER=0.4.0-pre2 TARGETS='--all'"; \
		exit 1; \
	fi
	@./scripts/set-version.sh $(VER) $(TARGETS)

log-go-tail:
	@# Tail the most recent velocity log file in logs/ without building or starting anything
	@if [ -d logs ] && [ $$(ls -1 logs/velocity-*.log 2>/dev/null | wc -l) -gt 0 ]; then \
		latest=$$(ls -1t logs/velocity-*.log 2>/dev/null | head -n1); \
		echo "Tailing $$latest"; \
		tail -F "$$latest"; \
	else \
		echo "No logs found in logs/ (try: make dev-go)"; exit 1; \
	fi

log-go-cat:
	@# Cat the entire most recent velocity log file (can be piped to grep, etc.)
	@if [ -d logs ] && [ $$(ls -1 logs/velocity-*.log 2>/dev/null | wc -l) -gt 0 ]; then \
		latest=$$(ls -1t logs/velocity-*.log 2>/dev/null | head -n1); \
		cat "$$latest"; \
	else \
		echo "No logs found in logs/ (try: make dev-go)"; exit 1; \
	fi

log-go-tail-all:
	@# Tail the most recent standard and debug velocity logs together (if present)
	@if [ -d logs ] && [ $$(ls -1 logs/velocity-*.log 2>/dev/null | wc -l) -gt 0 ]; then \
		main_log=$$(ls -1t logs/velocity-*.log 2>/dev/null | head -n1); \
		debug_log=$$(ls -1t logs/velocity-debug-*.log 2>/dev/null | head -n1); \
		if [ -n "$$debug_log" ] && [ -f "$$debug_log" ]; then \
			echo "Tailing $$main_log and $$debug_log"; \
			tail -F "$$main_log" "$$debug_log"; \
		else \
			echo "Tailing $$main_log (no debug log found yet)"; \
			tail -F "$$main_log"; \
		fi; \
	else \
		echo "No logs found in logs/ (try: make dev-go)"; exit 1; \
	fi

git-fs:
	@git fetch origin main >/dev/null 2>&1 || true; \
	git diff --name-only --diff-filter=ACMRTUXB origin/main...HEAD -- "$(or $(DIR),.)" | sort -u

# =============================================================================
# DATA VISUALIZATION
# =============================================================================

.PHONY: plot-noise-sweep plot-multisweep plot-noise-buckets stats-live stats-pcap profile-macos-lidar run-pcap-stats run-pcap-stats-10s

# Noise sweep line plot (neighbour=1, closeness=2.5 by default)
plot-noise-sweep:
	@[ -z "$(FILE)" ] && echo "Usage: make plot-noise-sweep FILE=data.csv [OUT=plot.png]" && exit 1 || true
	@[ ! -f "$(FILE)" ] && echo "File not found: $(FILE)" && exit 1 || true
	$(VENV_PYTHON) data/multisweep-graph/plot_noise_sweep.py --file "$(FILE)" \
		--out "$${OUT:-noise-sweep.png}" --neighbour $${NEIGHBOUR:-1} --closeness $${CLOSENESS:-2.5}

# Multi-sweep grid (neighbour=1 by default)
plot-multisweep:
	@[ -z "$(FILE)" ] && echo "Usage: make plot-multisweep FILE=data.csv [OUT=plot.png]" && exit 1 || true
	@[ ! -f "$(FILE)" ] && echo "File not found: $(FILE)" && exit 1 || true
	$(VENV_PYTHON) data/multisweep-graph/plot_multisweep.py --file "$(FILE)" \
		--out "$${OUT:-multisweep.png}" --neighbour $${NEIGHBOUR:-1}

# Per-noise bar charts (neighbour=1, closeness=2.5 by default)
plot-noise-buckets:
	@[ -z "$(FILE)" ] && echo "Usage: make plot-noise-buckets FILE=data.csv [OUT_DIR=plots/]" && exit 1 || true
	@[ ! -f "$(FILE)" ] && echo "File not found: $(FILE)" && exit 1 || true
	$(VENV_PYTHON) data/multisweep-graph/plot_noise_buckets.py --file "$(FILE)" \
		--out-dir "$${OUT_DIR:-noise-plots}" --neighbour $${NEIGHBOUR:-1} --closeness $${CLOSENESS:-2.5}

# Live grid stats - periodic snapshots from running lidar system
# Usage: make stats-live [INTERVAL=10] [DURATION=60]
stats-live:
	@echo "Starting live lidar server..."
	@$(MAKE) dev-go-lidar
	@sleep 2
	@echo "Capturing live grid snapshots..."
	$(VENV_PYTHON) tools/grid-heatmap/plot_grid_heatmap.py --interval $${INTERVAL:-30} $${DURATION:+--duration $$DURATION}

# PCAP replay grid stats - periodic snapshots during PCAP replay
# Usage: make stats-pcap PCAP=file.pcap [INTERVAL=5]
stats-pcap:
	@[ -z "$(PCAP)" ] && echo "Usage: make stats-pcap PCAP=file.pcap [INTERVAL=5]" && exit 1 || true
	@[ ! -f "$(PCAP)" ] && echo "PCAP file not found: $(PCAP)" && exit 1 || true
	@echo "Capturing PCAP replay snapshots via runtime data source switching..."
	$(VENV_PYTHON) tools/grid-heatmap/plot_grid_heatmap.py --pcap "$(PCAP)" --interval $${INTERVAL:-5}

# macOS process profiling at fixed cadence:
# 1) Poll idle metrics for IDLE seconds.
# 2) Trigger PCAP replay via API.
# 3) Continue polling until replay completes.
# Usage:
#   make profile-macos-lidar PCAP=/abs/path/file.pcapng [INTERVAL=5] [IDLE=120] [SENSOR=hesai-pandar40p] [BASE_URL=http://127.0.0.1:8081]
profile-macos-lidar:
	@[ -z "$(PCAP)" ] && echo "Usage: make profile-macos-lidar PCAP=/abs/path/file.pcapng [INTERVAL=5] [IDLE=120] [SENSOR=hesai-pandar40p] [BASE_URL=http://127.0.0.1:8081]" && exit 1 || true
	@[ ! -f "$(PCAP)" ] && echo "PCAP file not found: $(PCAP)" && exit 1 || true
	@./scripts/perf/macos_profile_lidar.sh \
		--pcap "$(PCAP)" \
		--interval $${INTERVAL:-5} \
		--idle-seconds $${IDLE:-120} \
		--sensor $${SENSOR:-hesai-pandar40p} \
		--base-url $${BASE_URL:-http://127.0.0.1:8081}

# PCAP capture statistics — frame rate, RPM, duration, track counts
# Accepts a single file or a directory (recursively finds .pcap/.pcapng)
# Usage:
#   make run-pcap-stats PCAP=capture.pcap
#   make run-pcap-stats PCAP=./data/captures/
run-pcap-stats:
	@if [ -z "$(PCAP)" ]; then \
		echo "Usage: make run-pcap-stats PCAP=<file-or-directory>"; \
		echo ""; \
		echo "  PCAP  Path to a .pcap/.pcapng file or a directory."; \
		echo "        If a directory, recursively finds all .pcap/.pcapng files."; \
		exit 1; \
	fi
	@if [ -d "$(PCAP)" ]; then \
		files=$$(find "$(PCAP)" -type f \( -name '*.pcap' -o -name '*.pcapng' \) | sort); \
		if [ -z "$$files" ]; then \
			echo "No .pcap/.pcapng files found in $(PCAP)"; \
			exit 1; \
		fi; \
		count=$$(echo "$$files" | wc -l | tr -d ' '); \
		echo "Found $$count PCAP file(s) in $(PCAP)"; \
		for f in $$files; do \
			go run -tags=pcap ./cmd/tools/pcap-analyse -pcap "$$f" -stats || true; \
		done; \
	elif [ -f "$(PCAP)" ]; then \
		go run -tags=pcap ./cmd/tools/pcap-analyse -pcap "$(PCAP)" -stats; \
	else \
		echo "Error: $(PCAP) not found"; \
		exit 1; \
	fi

# PCAP per-10s frame rate — grep-friendly one-line-per-bucket output
# Usage:
#   make run-pcap-stats-10s PCAP=capture.pcap
#   make run-pcap-stats-10s PCAP=./data/captures/
run-pcap-stats-10s:
	@if [ -z "$(PCAP)" ]; then \
		echo "Usage: make run-pcap-stats-10s PCAP=<file-or-directory>"; \
		echo ""; \
		echo "  PCAP  Path to a .pcap/.pcapng file or a directory."; \
		echo "        If a directory, recursively finds all .pcap/.pcapng files."; \
		exit 1; \
	fi
	@if [ -d "$(PCAP)" ]; then \
		files=$$(find "$(PCAP)" -type f \( -name '*.pcap' -o -name '*.pcapng' \) | sort); \
		if [ -z "$$files" ]; then \
			echo "No .pcap/.pcapng files found in $(PCAP)"; \
			exit 1; \
		fi; \
		count=$$(echo "$$files" | wc -l | tr -d ' '); \
		echo "Found $$count PCAP file(s) in $(PCAP)"; \
		for f in $$files; do \
			go run -tags=pcap ./cmd/tools/pcap-analyse -pcap "$$f" -stats-10s || true; \
		done; \
	elif [ -f "$(PCAP)" ]; then \
		go run -tags=pcap ./cmd/tools/pcap-analyse -pcap "$(PCAP)" -stats-10s; \
	else \
		echo "Error: $(PCAP) not found"; \
		exit 1; \
	fi

# =============================================================================
# API SHORTCUTS (LiDAR HTTP API)
# =============================================================================

.PHONY: api-grid-status api-grid-reset api-grid-heatmap \
        api-snapshot api-snapshots \
        api-acceptance api-acceptance-reset \
        api-params api-params-set \
        api-persist api-export-snapshot api-export-next-frame \
        api-status api-start-pcap api-stop-pcap api-switch-data-source

# Grid endpoints
api-grid-status:
	@./scripts/api/lidar/get_grid_status.sh $(SENSOR)

api-grid-reset:
	@./scripts/api/lidar/reset_grid.sh $(SENSOR)

api-grid-heatmap:
	@./scripts/api/lidar/get_grid_heatmap.sh $(SENSOR) $(AZIMUTH) $(THRESHOLD)

# Snapshot endpoints
api-snapshot:
	@./scripts/api/lidar/get_snapshot.sh $(SENSOR)

api-snapshots:
	@./scripts/api/lidar/get_snapshots.sh $(SENSOR)

# Acceptance endpoints
api-acceptance:
	@./scripts/api/lidar/get_acceptance.sh $(SENSOR)

api-acceptance-reset:
	@./scripts/api/lidar/reset_acceptance.sh $(SENSOR)

# Parameter endpoints
api-params:
	@./scripts/api/lidar/get_params.sh $(SENSOR)

api-params-set:
	@[ -z "$(PARAMS)" ] && echo "Usage: make api-params-set SENSOR=sensor-id PARAMS='{\"noise_relative\": 0.15}'" && exit 1 || true
	@./scripts/api/lidar/set_params.sh $(SENSOR) '$(PARAMS)'

# Persistence and export endpoints
api-persist:
	@./scripts/api/lidar/trigger_persist.sh $(SENSOR)

api-export-snapshot:
	@./scripts/api/lidar/export_snapshot.sh $(SENSOR) $(SNAPSHOT_ID) $(OUT)

api-export-next-frame:
	@./scripts/api/lidar/export_next_frame.sh $(SENSOR) $(OUT)

# Status & data source endpoints
api-status:
	@./scripts/api/lidar/get_status.sh $(BASE_URL)

# PCAP replay
api-start-pcap:
	@[ -z "$(PCAP)" ] && echo "Usage: make api-start-pcap PCAP=file.pcap [BASE_URL=http://127.0.0.1:8081]" && exit 1 || true
	@[ ! -f "$(PCAP)" ] && echo "PCAP file not found: $(PCAP)" && exit 1 || true
	@./scripts/api/lidar/start_pcap.sh "$(PCAP)" $(SENSOR) $(BASE_URL)

api-stop-pcap:
	@./scripts/api/lidar/stop_pcap.sh $(SENSOR) $(BASE_URL)

api-switch-data-source:
	@[ -z "$(SOURCE)" ] && echo "Usage: make api-switch-data-source SOURCE={live|pcap} [PCAP=file.pcap] [SENSOR=hesai-pandar40p] [BASE_URL=http://127.0.0.1:8081]" && exit 1 || true
	@if [ "$(SOURCE)" = "pcap" ]; then \
		[ -z "$(PCAP)" ] && echo "PCAP file required when SOURCE=pcap" && exit 1 || true; \
		[ ! -f "$(PCAP)" ] && echo "PCAP file not found: $(PCAP)" && exit 1 || true; \
		./scripts/api/lidar/switch_data_source.sh $(SOURCE) "$(PCAP)" $(SENSOR) $(BASE_URL); \
	else \
		./scripts/api/lidar/switch_data_source.sh $(SOURCE) $(SENSOR) $(BASE_URL); \
	fi
