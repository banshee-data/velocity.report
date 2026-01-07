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
	@echo ""
	@echo "INSTALLATION:"
	@echo "  install-python       Set up Python PDF generator (venv + deps)"
	@echo "  deploy-install-latex Install LaTeX on remote target (for PDF generation)"
	@echo "  deploy-update-deps   Update source, LaTeX, and Python deps on remote target"
	@echo "  install-web          Install web dependencies (pnpm/npm)"
	@echo "  install-docs         Install docs dependencies (pnpm/npm)"
	@echo ""
	@echo "DEVELOPMENT SERVERS:"
	@echo "  dev-go               Start Go server (radar disabled)"
	@echo "  dev-go-lidar         Start Go server with LiDAR enabled"
	@echo "  dev-go-kill-server   Stop background Go server"
	@echo "  dev-web              Start web dev server"
	@echo "  dev-docs             Start docs dev server"
	@echo ""
	@echo "TESTING:"
	@echo "  test                 Run all tests (Go + Python + Web)"
	@echo "  test-go              Run Go unit tests"
	@echo "  test-python          Run Python PDF generator tests"
	@echo "  test-python-cov      Run Python tests with coverage"
	@echo "  test-web             Run web tests (Jest)"
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
	@echo ""
	@echo "FORMATTING (mutating):"
	@echo "  format               Format all code (Go + Python + Web + SQL)"
	@echo "  format-go            Format Go code (gofmt)"
	@echo "  format-python        Format Python code (black + ruff)"
	@echo "  format-web           Format web code (prettier)"
	@echo "  format-sql           Format SQL files (sql-formatter)"
	@echo ""
	@echo "LINTING (non-mutating, CI-friendly):"
	@echo "  lint                 Lint all code, fail if formatting needed"
	@echo "  lint-go              Check Go formatting"
	@echo "  lint-python          Check Python formatting"
	@echo "  lint-web             Check web formatting"
	@echo ""
	@echo "PDF GENERATOR:"
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
	@echo ""
	@echo "DATA VISUALIZATION:"
	@echo "  plot-noise-sweep     Generate noise sweep line plot (FILE=data.csv)"
	@echo "  plot-multisweep      Generate multi-parameter grid (FILE=data.csv)"
	@echo "  plot-noise-buckets   Generate per-noise bar charts (FILE=data.csv)"
	@echo "  stats-live           Capture live LiDAR snapshots"
	@echo "  stats-pcap           Capture PCAP replay snapshots (PCAP=file.pcap)"
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
VERSION := 0.4.0-pre3
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X 'main.version=$(VERSION)' -X 'main.gitSHA=$(GIT_SHA)'

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
	go build -o velocity-deploy ./cmd/deploy

build-deploy-linux:
	GOOS=linux GOARCH=arm64 go build -o velocity-deploy-linux-arm64 ./cmd/deploy

.PHONY: build-web
build-web:
	@echo "Building web frontend..."
	@cd web && if command -v pnpm >/dev/null 2>&1; then \
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
	@cd docs && if command -v pnpm >/dev/null 2>&1; then \
		pnpm run build; \
	elif command -v npm >/dev/null 2>&1; then \
		npm run build; \
	else \
		echo "pnpm/npm not found; install pnpm (recommended) or npm and retry"; exit 1; \
	fi
	@echo "✓ Docs build complete: docs/_site/"

# =============================================================================
# INSTALLATION
# =============================================================================

.PHONY: install-python install-web install-docs deploy-install-latex deploy-update-deps

# Python environment variables (unified at repository root)
VENV_DIR = .venv
VENV_PYTHON = $(VENV_DIR)/bin/python3
VENV_PIP = $(VENV_DIR)/bin/pip
VENV_PYTEST = $(VENV_DIR)/bin/pytest
PDF_DIR = tools/pdf-generator
PYTHON_VERSION = 3.12

# Deploy: Install LaTeX on remote target
deploy-install-latex:
	@if [ -z "$(TARGET)" ]; then \
		echo "Error: TARGET not set. Usage: make deploy-install-latex TARGET=radar-ts"; \
		exit 1; \
	fi
	@echo "Installing LaTeX on $(TARGET)..."
	@ssh $(TARGET) "if ! command -v pdflatex >/dev/null 2>&1; then \
		sudo apt-get update && sudo apt-get install -y texlive-xetex texlive-fonts-recommended texlive-latex-extra; \
	else \
		echo 'LaTeX already installed'; \
	fi"

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
	@ssh $(TARGET) "if ! command -v pdflatex >/dev/null 2>&1; then \
		sudo apt-get update && sudo apt-get install -y texlive-xetex texlive-fonts-recommended texlive-latex-extra; \
	fi"
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
	@cd docs && if command -v pnpm >/dev/null 2>&1; then \
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

.PHONY: dev-go dev-go-lidar dev-go-kill-server dev-web dev-docs

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
	go build -tags=pcap -o velocity-report-local ./cmd/radar; \
	mkdir -p "$$piddir"; \
	echo "Starting velocity-report-local (background) with DB=$$DB_PATH -> $$logfile (debug -> $$debuglog)"; \
	VELOCITY_DEBUG_LOG="$$debuglog" nohup ./velocity-report-local --disable-radar $(1) --db-path="$$DB_PATH" >> "$$logfile" 2>&1 & echo $$! > "$$pidfile"; \
	echo "Started; PID $$(cat $$pidfile)"; \
	echo "Log: $$logfile"
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

dev-go:
	@$(call run_dev_go)

dev-go-lidar:
	@$(call run_dev_go,--enable-lidar --lidar-bg-flush-interval=60s --lidar-seed-from-first=true --lidar-forward)

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
	@cd docs && if command -v pnpm >/dev/null 2>&1; then \
		pnpm run dev; \
		elif command -v npm >/dev/null 2>&1; then \
		npm run dev; \
		else \
			echo "pnpm/npm not found; install dependencies (pnpm install) and run 'pnpm run dev'"; exit 1; \
		fi

# =============================================================================
# TESTING
# =============================================================================

.PHONY: test test-go test-python test-python-cov test-web

WEB_DIR = web

# Aggregate test target: runs Go, web, and Python tests in sequence
test: test-go test-web test-python

# Run Go unit tests for the whole repository
test-go:
	@echo "Running Go unit tests..."
	@go test ./...

# Run Python tests for the PDF generator. Ensures venv is setup first.
test-python:
	@echo "Running Python (PDF generator) tests..."
	@$(MAKE) install-python
	@$(MAKE) pdf-test

test-python-cov:
	@echo "Running PDF generator tests with coverage..."
	cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTEST) --cov=pdf_generator --cov-report=html pdf_generator/tests/
	@echo "Coverage report: $(PDF_DIR)/htmlcov/index.html"

# Run web test suite (Jest) using pnpm inside the web directory
test-web:
	@echo "Running web (Jest) tests..."
	@cd $(WEB_DIR) && pnpm run test:ci

# =============================================================================
# DATABASE MIGRATIONS
# =============================================================================

.PHONY: migrate-up migrate-down migrate-status migrate-detect migrate-version migrate-force migrate-baseline schema-sync

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

# =============================================================================
# FORMATTING (mutating)
# =============================================================================

.PHONY: format format-go format-python format-web format-sql

format: format-go format-python format-web format-sql
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

format-sql:
	@echo "Formatting SQL files with sql-formatter..."
	@bash scripts/format-sql.sh

# =============================================================================
# LINTING (non-mutating, CI-friendly)
# =============================================================================

.PHONY: lint lint-go lint-python lint-web

lint: lint-go lint-python lint-web
	@echo "\nAll lint checks passed."

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

.PHONY: pdf-test pdf-report pdf-config pdf-demo pdf clean-python

pdf-test:
	@echo "Running PDF generator tests..."
	cd $(PDF_DIR) && PYTHONPATH=. ../../$(VENV_PYTEST) pdf_generator/tests/

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

.PHONY: set-version log-go-tail log-go-cat log-go-tail-all

set-version:
	@if [ -z "$(VER)" ]; then \
		echo "Usage: make set-version VER=<version> TARGETS='<targets>'"; \
		echo ""; \
		echo "Example: make set-version VER=0.4.0-pre2 TARGETS='--all'"; \
		echo "         make set-version VER=0.5.0 TARGETS='--makefile --deploy'"; \
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

# =============================================================================
# DATA VISUALIZATION
# =============================================================================

.PHONY: plot-noise-sweep plot-multisweep plot-noise-buckets stats-live stats-pcap

# Noise sweep line plot (neighbor=1, closeness=2.5 by default)
plot-noise-sweep:
	@[ -z "$(FILE)" ] && echo "Usage: make plot-noise-sweep FILE=data.csv [OUT=plot.png]" && exit 1 || true
	@[ ! -f "$(FILE)" ] && echo "File not found: $(FILE)" && exit 1 || true
	$(VENV_PYTHON) data/multisweep-graph/plot_noise_sweep.py --file "$(FILE)" \
		--out "$${OUT:-noise-sweep.png}" --neighbor $${NEIGHBOR:-1} --closeness $${CLOSENESS:-2.5}

# Multi-sweep grid (neighbor=1 by default)
plot-multisweep:
	@[ -z "$(FILE)" ] && echo "Usage: make plot-multisweep FILE=data.csv [OUT=plot.png]" && exit 1 || true
	@[ ! -f "$(FILE)" ] && echo "File not found: $(FILE)" && exit 1 || true
	$(VENV_PYTHON) data/multisweep-graph/plot_multisweep.py --file "$(FILE)" \
		--out "$${OUT:-multisweep.png}" --neighbor $${NEIGHBOR:-1}

# Per-noise bar charts (neighbor=1, closeness=2.5 by default)
plot-noise-buckets:
	@[ -z "$(FILE)" ] && echo "Usage: make plot-noise-buckets FILE=data.csv [OUT_DIR=plots/]" && exit 1 || true
	@[ ! -f "$(FILE)" ] && echo "File not found: $(FILE)" && exit 1 || true
	$(VENV_PYTHON) data/multisweep-graph/plot_noise_buckets.py --file "$(FILE)" \
		--out-dir "$${OUT_DIR:-noise-plots}" --neighbor $${NEIGHBOR:-1} --closeness $${CLOSENESS:-2.5}

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
