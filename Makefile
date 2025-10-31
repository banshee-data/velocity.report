radar-linux:
	GOOS=linux GOARCH=arm64 go build -o app-radar-linux-arm64 ./cmd/radar

radar-linux-pcap:
	GOOS=linux GOARCH=arm64 go build -tags=pcap -o app-radar-linux-arm64 ./cmd/radar

radar-mac:
	GOOS=darwin GOARCH=arm64 go build -tags=pcap -o app-radar-mac-arm64 ./cmd/radar

radar-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -tags=pcap -o app-radar-mac-amd64 ./cmd/radar

radar-local:
	go build -tags=pcap -o app-radar-local ./cmd/radar


# Reusable script for starting the app in background. Call with extra flags
# using '$(call run_dev_go,<extra-flags>)'. Uses shell $$ variables so we
# escape $ to $$ inside the define so the resulting shell script receives
# single-dollar variables.
define run_dev_go
mkdir -p logs; \
ts=$$(date +%Y%m%d-%H%M%S); \
logfile=logs/velocity-$${ts}.log; \
piddir=logs/pids; \
pidfile=$${piddir}/velocity-$${ts}.pid; \
DB_PATH=$${DB_PATH:-./sensor_data.db}; \
echo "Stopping previously-launched app-radar-local processes (from $$piddir) ..."; \
if [ -d "$$piddir" ] && [ $$(ls -1 $$piddir/velocity-*.pid 2>/dev/null | wc -l) -gt 0 ]; then \
  for pidfile_k in $$(ls -1t $$piddir/velocity-*.pid 2>/dev/null | head -n3); do \
    pid_k=$$(cat "$$pidfile_k" 2>/dev/null || echo); \
    if [ -n "$$pid_k" ] && kill -0 $$pid_k 2>/dev/null; then \
      cmdline=$$(ps -p $$pid_k -o args= 2>/dev/null || true); \
      case "$$cmdline" in \
        *app-radar-local*) \
          echo "Stopping pid $$pid_k (from $$pidfile_k): $$cmdline"; \
          kill $$pid_k 2>/dev/null || true; \
          sleep 1; \
          kill -0 $$pid_k 2>/dev/null && kill -9 $$pid_k 2>/dev/null || true; \
          ;; \
        *) echo "Skipping pid $$pid_k (cmd does not match app-radar-local): $$cmdline"; ;; \
      esac; \
    fi; \
  done; \
fi; \
echo "Building app-radar-local..."; \
go build -tags=pcap -o app-radar-local ./cmd/radar; \
mkdir -p "$$piddir"; \
echo "Starting app-radar-local (background) with DB=$$DB_PATH -> $$logfile"; \
nohup ./app-radar-local --disable-radar --enable-lidar $(1) --db-path="$$DB_PATH" >> "$$logfile" 2>&1 & echo $$! > "$$pidfile"; \
echo "Started; PID $$(cat $$pidfile)"; \
echo "Log: $$logfile"
endef

.PHONY: dev-go dev-go-pcap
dev-go:
	@$(call run_dev_go,)

dev-go-pcap:
	@$(call run_dev_go,--lidar-pcap-mode --debug)

.PHONY: tail-log-go
tail-log-go:
	@# Tail the most recent velocity log file in logs/ without building or starting anything
	@if [ -d logs ] && [ $$(ls -1 logs/velocity-*.log 2>/dev/null | wc -l) -gt 0 ]; then \
		latest=$$(ls -1t logs/velocity-*.log 2>/dev/null | head -n1); \
		echo "Tailing $$latest"; \
		tail -F "$$latest"; \
	else \
		echo "No logs found in logs/ (try: make dev-go)"; exit 1; \
	fi

.PHONY: cat-log-go
cat-log-go:
	@# Cat the entire most recent velocity log file (can be piped to grep, etc.)
	@if [ -d logs ] && [ $$(ls -1 logs/velocity-*.log 2>/dev/null | wc -l) -gt 0 ]; then \
		latest=$$(ls -1t logs/velocity-*.log 2>/dev/null | head -n1); \
		cat "$$latest"; \
	else \
		echo "No logs found in logs/ (try: make dev-go)"; exit 1; \
	fi

tools-local:
	go build -o app-bg-sweep ./cmd/bg-sweep
	go build -o app-bg-multisweep ./cmd/bg-multisweep


# =============================================================================
# Python PDF Generator (PYTHONPATH approach - no package installation)
# =============================================================================

.PHONY: pdf-setup pdf-test pdf-test-cov pdf-report pdf-config pdf-demo pdf-clean

PDF_DIR = tools/pdf-generator
PDF_PYTHON = $(PDF_DIR)/.venv/bin/python
PDF_PYTEST = $(PDF_DIR)/.venv/bin/pytest

pdf-setup:
	@echo "Setting up PDF generator..."
	cd $(PDF_DIR) && python3 -m venv .venv
	cd $(PDF_DIR) && .venv/bin/pip install --upgrade pip
	cd $(PDF_DIR) && .venv/bin/pip install -r requirements.txt
	@echo "✓ PDF generator setup complete (no package installation needed)"

pdf-test:
	@echo "Running PDF generator tests..."
	cd $(PDF_DIR) && PYTHONPATH=. .venv/bin/pytest pdf_generator/tests/

pdf-test-cov:
	@echo "Running PDF generator tests with coverage..."
	cd $(PDF_DIR) && PYTHONPATH=. .venv/bin/pytest --cov=pdf_generator --cov-report=html pdf_generator/tests/
	@echo "Coverage report: $(PDF_DIR)/htmlcov/index.html"

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
	cd $(PDF_DIR) && PYTHONPATH=. .venv/bin/python -m pdf_generator.cli.main $$CONFIG_PATH

pdf-config:
	@echo "Creating example configuration..."
	cd $(PDF_DIR) && PYTHONPATH=. .venv/bin/python -m pdf_generator.cli.create_config

pdf-demo:
	@echo "Running configuration system demo..."
	cd $(PDF_DIR) && PYTHONPATH=. .venv/bin/python -m pdf_generator.cli.demo

pdf-clean:
	@echo "Cleaning PDF generator outputs..."
	rm -rf $(PDF_DIR)/output/*.pdf
	rm -rf $(PDF_DIR)/output/*.tex
	rm -rf $(PDF_DIR)/output/*.svg
	rm -rf $(PDF_DIR)/.pytest_cache
	rm -rf $(PDF_DIR)/htmlcov
	rm -rf $(PDF_DIR)/.coverage
	rm -rf $(PDF_DIR)/pdf_generator/**/__pycache__
	@echo "✓ Cleaned"

# Convenience alias
pdf: pdf-report

# =============================================================================
# Test targets
# =============================================================================

.PHONY: test test-all test-go test-web test-python

WEB_DIR = web

# Run Go unit tests for the whole repository
test-go:
	@echo "Running Go unit tests..."
	@go test ./...

# Run web test suite (Jest) using pnpm inside the web directory
test-web:
	@echo "Running web (Jest) tests..."
	@cd $(WEB_DIR) && pnpm run test:ci

# Run Python tests for the PDF generator. Ensures venv is setup first.
test-python:
	@echo "Running Python (PDF generator) tests..."
	@$(MAKE) pdf-setup
	@$(MAKE) pdf-test

# Aggregate test target: runs Go, web, and Python tests in sequence
test: test-go test-web test-python

# =============================================================================
# Formatting target: formats Go, Python and JS/TS (where tooling is available)
# =============================================================================

.PHONY: format-go format-python format-web fmt

format-go:
	@echo "Formatting Go source (gofmt)..."
	@gofmt -s -w . || true

format-python:
	@echo "Formatting Python (black, ruff) using venv at $(PDF_DIR)/.venv if present..."
	@if [ -x "$(PDF_DIR)/.venv/bin/black" ]; then \
		"$(PDF_DIR)/.venv/bin/black" . || true; \
	elif command -v black >/dev/null 2>&1; then \
		black . || true; \
	else \
		echo "black not found; to install into the PDF venv: cd $(PDF_DIR) && python3 -m venv .venv && .venv/bin/pip install -U black ruff"; \
	fi
	@if [ -x "$(PDF_DIR)/.venv/bin/ruff" ]; then \
		"$(PDF_DIR)/.venv/bin/ruff" check --fix . || true; \
	elif command -v ruff >/dev/null 2>&1; then \
		ruff check --fix . || true; \
	else \
		echo "ruff not found; to install into the PDF venv: cd $(PDF_DIR) && python3 -m venv .venv && .venv/bin/pip install -U ruff"; \
	fi

format-web:
	@echo "Formatting web JS/TS in $(WEB_DIR) (prettier via pnpm or npx)..."
	@if [ -d "$(WEB_DIR)" ]; then \
		if command -v pnpm >/dev/null 2>&1; then \
			cd $(WEB_DIR) && pnpm exec prettier --write . || echo "prettier run failed or not configured"; \
		elif command -v npx >/dev/null 2>&1; then \
			cd $(WEB_DIR) && npx prettier --write . || echo "prettier run failed or not configured"; \
		else \
			echo "pnpm/npx not found; skipping JS/TS formatting in $(WEB_DIR)"; \
		fi; \
	else \
		echo "$(WEB_DIR) does not exist; skipping web formatting"; \
	fi

fmt: format-go format-python format-web
	@echo "\nAll formatting targets complete."

## Lint (non-mutating) checks - fail if formatting is required
.PHONY:	lint lint-go lint-python lint-web

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
	@if [ -x "$(PDF_DIR)/.venv/bin/black" ]; then \
		"$(PDF_DIR)/.venv/bin/black" --check .; \
	elif command -v black >/dev/null 2>&1; then \
		black --check .; \
	else \
		echo "black not found; install it (e.g. cd $(PDF_DIR) && python3 -m venv .venv && .venv/bin/pip install -U black)"; \
		exit 2; \
	fi
	@if [ -x "$(PDF_DIR)/.venv/bin/ruff" ]; then \
		"$(PDF_DIR)/.venv/bin/ruff" check .; \
	elif command -v ruff >/dev/null 2>&1; then \
		ruff check .; \
	else \
		echo "ruff not found; install it (e.g. cd $(PDF_DIR) && python3 -m venv .venv && .venv/bin/pip install -U ruff)"; \
		exit 2; \
	fi

lint-web:
	@echo "Checking web formatting (prettier --check) in $(WEB_DIR)..."
	@if [ -d "$(WEB_DIR)" ]; then \
		if command -v pnpm >/dev/null 2>&1; then \
			cd $(WEB_DIR) && pnpm exec prettier --check . || exit 1; \
		elif command -v npx >/dev/null 2>&1; then \
			cd $(WEB_DIR) && npx prettier --check . || exit 1; \
		else \
			echo "pnpm/npx not found; cannot run prettier --check"; \
			exit 2; \
		fi; \
	else \
		echo "$(WEB_DIR) does not exist; skipping web format check"; \
	fi

lint: lint-go lint-python lint-web
	@echo "\nAll lint checks passed."
