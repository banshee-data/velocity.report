radar-linux:
	GOOS=linux GOARCH=arm64 go build -o app-radar-linux-arm64 ./cmd/radar

radar-mac:
	GOOS=darwin GOARCH=arm64 go build -o app-radar-mac-arm64 ./cmd/radar

radar-mac-intel:
	GOOS=darwin GOARCH=amd64 go build -o app-radar-mac-amd64 ./cmd/radar

radar-local:
	go build -o app-radar-local ./cmd/radar

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
