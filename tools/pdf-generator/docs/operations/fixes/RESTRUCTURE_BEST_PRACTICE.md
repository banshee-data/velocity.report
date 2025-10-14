# PDF Generator Restructure: Best Practice Implementation

**Date**: October 11, 2025
**Status**: Ready for Implementation
**Approach**: Move to `tools/` with proper Python package structure

---

## Executive Summary

This guide provides a **complete, step-by-step implementation** for moving the Python PDF generator from `internal/report/query_data/` to a proper tool structure in `tools/pdf-generator/`. This respects Go monorepo conventions while following Python best practices.

**Current State** (After recent cleanup):
- ✅ Removed deprecated `report_config.py` and `test_report_config.py`
- ✅ Removed `generate_report_api.py` and tests
- ✅ 18 production Python modules
- ✅ 451 passing tests
- ❌ Still buried in Go `internal/` directory
- ❌ No standard command interface

**Target State**:
```
velocity.report/                    # Go monorepo
├── cmd/                            # Go executables
├── internal/                       # Go packages ONLY
│   ├── api/
│   ├── db/
│   └── radar/
├── tools/                          # ✨ Non-Go utilities
│   └── pdf-generator/             # Python PDF generator (proper package)
│       ├── pyproject.toml
│       ├── requirements.txt
│       ├── pdf_generator/         # Python package
│       │   ├── cli/              # Entry points
│       │   ├── core/             # Internal modules
│       │   └── tests/            # All tests
│       └── output/               # Generated PDFs
├── web/                           # Frontend
└── Makefile                       # Includes pdf-* commands
```

---

## Benefits

### 1. Respects Monorepo Conventions
- ✅ `internal/` is for Go packages only
- ✅ `tools/` is standard for utility programs
- ✅ Clear separation of concerns

### 2. Professional Python Structure
- ✅ Proper package layout (`pdf_generator/`)
- ✅ CLI separated from core logic
- ✅ Tests co-located with package
- ✅ Modern `pyproject.toml` configuration

### 3. Developer Experience
- ✅ Simple commands: `make pdf-report CONFIG=config.json`
- ✅ Can install as package: `pip install -e tools/pdf-generator`
- ✅ Console entry points: `pdf-generator config.json`
- ✅ Clean imports: `from pdf_generator.core import ChartBuilder`

### 4. Go Integration
- ✅ Clear path: `tools/pdf-generator`
- ✅ Module execution: `python3 -m pdf_generator.cli.main`
- ✅ Isolated dependencies (venv in tool directory)

---

## File Inventory

### Current Location: `internal/report/query_data/`

**Core Modules** (18 files):
1. `api_client.py` - API communication
2. `chart_builder.py` - Chart generation
3. `chart_saver.py` - Chart persistence
4. `config_manager.py` - Configuration system
5. `data_transformers.py` - Data normalization
6. `date_parser.py` - Date parsing utilities
7. `dependency_checker.py` - Dependency validation
8. `document_builder.py` - LaTeX document assembly
9. `map_utils.py` - Map generation
10. `pdf_generator.py` - PDF orchestration
11. `report_sections.py` - Report sections
12. `stats_utils.py` - Statistical utilities
13. `table_builders.py` - LaTeX tables

**CLI Entry Points** (2 files):
14. `get_stats.py` - Main CLI
15. `create_config_example.py` - Config generator

**Demo/Utils** (3 files):
16. `demo_config_system.py` - Interactive demo
17. `__init__.py` - Package marker
18. `conftest.py` - Pytest configuration

**Test Files** (30 files):
- `test_*.py` - All test modules

**Resources**:
- `fonts/` - Font files
- `docs/` - Documentation
- `*.json` - Example configs
- Generated artifacts: `*.pdf`, `*.tex`, `*.svg`

---

## Implementation Steps

### Phase 1: Create New Structure (30 minutes)

```bash
cd /Users/david/code/velocity.report

# Create new directory structure
mkdir -p tools/pdf-generator/pdf_generator/{cli,core,tests}
mkdir -p tools/pdf-generator/output
mkdir -p tools/pdf-generator/fonts
mkdir -p tools/pdf-generator/docs

# Create __init__.py files
touch tools/pdf-generator/pdf_generator/__init__.py
touch tools/pdf-generator/pdf_generator/cli/__init__.py
touch tools/pdf-generator/pdf_generator/core/__init__.py
touch tools/pdf-generator/pdf_generator/tests/__init__.py
```

### Phase 2: Copy Files to New Structure (15 minutes)

```bash
# Set paths
SRC="internal/report/query_data"
DST="tools/pdf-generator"

# Copy CLI entry points
cp $SRC/get_stats.py $DST/pdf_generator/cli/main.py
cp $SRC/create_config_example.py $DST/pdf_generator/cli/create_config.py
cp $SRC/demo_config_system.py $DST/pdf_generator/cli/demo.py

# Copy core modules
for file in api_client.py chart_builder.py chart_saver.py config_manager.py \
            data_transformers.py date_parser.py dependency_checker.py \
            document_builder.py map_utils.py pdf_generator.py \
            report_sections.py stats_utils.py table_builders.py; do
    cp $SRC/$file $DST/pdf_generator/core/$file
done

# Copy all tests
cp $SRC/test_*.py $DST/pdf_generator/tests/
cp $SRC/conftest.py $DST/pdf_generator/tests/

# Copy resources
cp -r $SRC/fonts/ $DST/fonts/
cp -r $SRC/docs/ $DST/docs/
cp $SRC/*.json $DST/
cp $SRC/requirements.txt $DST/
cp $SRC/README.md $DST/
cp $SRC/REQUIRED_FIELDS.md $DST/
```

### Phase 3: Create Configuration Files (15 minutes)

**File**: `tools/pdf-generator/pyproject.toml`

```toml
[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"

[project]
name = "velocity-report-pdf-generator"
version = "1.0.0"
description = "PDF report generator for velocity.report Go application"
readme = "README.md"
requires-python = ">=3.11"
authors = [
    {name = "Banshee Data", email = "info@banshee.data"},
]
dependencies = [
    "matplotlib>=3.10.6",
    "numpy>=2.3.3",
    "pandas>=2.3.2",
    "pylatex>=1.4.2",
    "requests>=2.32.3",
    "seaborn>=0.13.3",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.4.2",
    "pytest-cov>=7.0.0",
]

[project.scripts]
pdf-generator = "pdf_generator.cli.main:main"
pdf-config = "pdf_generator.cli.create_config:main"
pdf-demo = "pdf_generator.cli.demo:main"

[tool.setuptools.packages.find]
where = ["."]
include = ["pdf_generator*"]

[tool.pytest.ini_options]
testpaths = ["pdf_generator/tests"]
python_files = ["test_*.py"]
python_classes = ["Test*"]
python_functions = ["test_*"]
addopts = "-v --strict-markers"

[tool.coverage.run]
source = ["pdf_generator"]
omit = ["pdf_generator/tests/*"]

[tool.coverage.report]
exclude_lines = [
    "pragma: no cover",
    "def __repr__",
    "raise AssertionError",
    "raise NotImplementedError",
    "if __name__ == .__main__.:",
    "if TYPE_CHECKING:",
]
```

**File**: `tools/pdf-generator/output/.gitignore`

```gitignore
# Ignore all generated files
*.pdf
*.tex
*.svg
*.png
*.log
*.aux
*.fdb_latexmk
*.fls
*.xdv

# Keep the directory
!.gitignore
```

**File**: `tools/pdf-generator/.gitignore`

```gitignore
# Python
__pycache__/
*.py[cod]
*$py.class
*.so
.Python

# Virtual environment
.venv/
venv/
ENV/
env/

# Testing
.pytest_cache/
.coverage
htmlcov/
.tox/

# IDE
.vscode/
.idea/
*.swp
*.swo
*~

# Output
output/*.pdf
output/*.tex
output/*.svg
output/*.png
!output/.gitignore

# OS
.DS_Store
Thumbs.db
```

### Phase 4: Update Import Statements (30-60 minutes)

This is the most tedious but critical step. Update all imports to use the new package structure.

**In `pdf_generator/cli/main.py`** (was `get_stats.py`):

```python
# OLD imports
from api_client import RadarStatsClient, SupportedGroups
from config_manager import load_config
from chart_builder import TimeSeriesChartBuilder, HistogramChartBuilder
# ... etc

# NEW imports
from pdf_generator.core.api_client import RadarStatsClient, SupportedGroups
from pdf_generator.core.config_manager import load_config
from pdf_generator.core.chart_builder import TimeSeriesChartBuilder, HistogramChartBuilder
# ... etc
```

**Create a helper script** to automate this:

**File**: `tools/pdf-generator/update_imports.py`

```python
#!/usr/bin/env python3
"""Update imports to use new package structure."""

import re
from pathlib import Path

# Modules in core/
CORE_MODULES = [
    "api_client", "chart_builder", "chart_saver", "config_manager",
    "data_transformers", "date_parser", "dependency_checker",
    "document_builder", "map_utils", "pdf_generator",
    "report_sections", "stats_utils", "table_builders",
]

def update_file(filepath: Path):
    """Update imports in a single file."""
    content = filepath.read_text()
    original = content

    for module in CORE_MODULES:
        # Update: from module import ...
        pattern = rf'^from {module} import'
        replacement = rf'from pdf_generator.core.{module} import'
        content = re.sub(pattern, replacement, content, flags=re.MULTILINE)

        # Update: import module
        pattern = rf'^import {module}$'
        replacement = rf'import pdf_generator.core.{module} as {module}'
        content = re.sub(pattern, replacement, content, flags=re.MULTILINE)

    if content != original:
        filepath.write_text(content)
        print(f"✓ Updated {filepath}")
    else:
        print(f"  No changes needed: {filepath}")

def main():
    base = Path("pdf_generator")

    # Update CLI files
    for file in (base / "cli").glob("*.py"):
        if file.name != "__init__.py":
            update_file(file)

    # Update core files (they import from each other)
    for file in (base / "core").glob("*.py"):
        if file.name != "__init__.py":
            update_file(file)

    # Update test files
    for file in (base / "tests").glob("test_*.py"):
        update_file(file)

    # Update conftest
    update_file(base / "tests" / "conftest.py")

if __name__ == "__main__":
    main()
```

Run it:
```bash
cd tools/pdf-generator
python3 update_imports.py
```

### Phase 5: Update __main__ Entry Points (15 minutes)

Make CLI modules executable as modules.

**File**: `tools/pdf-generator/pdf_generator/cli/main.py`

Add at the bottom:
```python
def main():
    """Entry point for console script."""
    import sys
    # Your existing main logic here
    # ...

if __name__ == "__main__":
    main()
```

**File**: `tools/pdf-generator/pdf_generator/cli/create_config.py`

```python
def main():
    """Entry point for console script."""
    # Your existing config generation logic
    # ...

if __name__ == "__main__":
    main()
```

### Phase 6: Setup Virtual Environment (5 minutes)

**Note**: We're using the PYTHONPATH approach (no package installation) for simpler deployment, especially to Raspberry Pi.

```bash
cd tools/pdf-generator

# Create virtual environment
python3 -m venv .venv

# Install dependencies only (no package installation)
.venv/bin/pip install --upgrade pip
.venv/bin/pip install -r requirements.txt
```

### Phase 7: Update Tests (5 minutes)

Ensure tests can find the package via PYTHONPATH.

**File**: `tools/pdf-generator/pdf_generator/tests/conftest.py`

Add at the top if not already present:
```python
"""Pytest configuration for pdf_generator tests."""

import sys
from pathlib import Path

# Add parent directory to Python path so imports work
pkg_root = Path(__file__).parent.parent.parent
sys.path.insert(0, str(pkg_root))

# ... rest of your conftest.py
```

### Phase 8: Run Tests (5 minutes)

```bash
cd tools/pdf-generator

# Set PYTHONPATH and run tests
PYTHONPATH=. .venv/bin/pytest pdf_generator/tests/

# Run with coverage
PYTHONPATH=. .venv/bin/pytest --cov=pdf_generator --cov-report=html pdf_generator/tests/

# Should see: 451 passed
```

### Phase 9: Add Makefile Commands (10 minutes)

**File**: `Makefile` (append to existing)

```makefile
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
	cd $(PDF_DIR) && PYTHONPATH=. $(PDF_PYTEST) pdf_generator/tests/

pdf-test-cov:
	@echo "Running PDF generator tests with coverage..."
	cd $(PDF_DIR) && PYTHONPATH=. $(PDF_PYTEST) --cov=pdf_generator --cov-report=html pdf_generator/tests/
	@echo "Coverage report: $(PDF_DIR)/htmlcov/index.html"

pdf-report:
	@if [ -z "$(CONFIG)" ]; then \
		echo "Error: CONFIG required. Usage: make pdf-report CONFIG=config.json"; \
		exit 1; \
	fi
	cd $(PDF_DIR) && PYTHONPATH=. $(PDF_PYTHON) -m pdf_generator.cli.main $(CONFIG)

pdf-config:
	@echo "Creating example configuration..."
	cd $(PDF_DIR) && PYTHONPATH=. $(PDF_PYTHON) -m pdf_generator.cli.create_config

pdf-demo:
	@echo "Running configuration system demo..."
	cd $(PDF_DIR) && PYTHONPATH=. $(PDF_PYTHON) -m pdf_generator.cli.demo

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
```

### Phase 10: Update Go Integration (30 minutes)

Find all places in your Go code that call the Python generator and update them.

**Before**:
```go
cmd := exec.Command(
    "python3",
    "internal/report/query_data/get_stats.py",
    configPath,
)
```

**After** (Option 1 - Using installed script):
```go
cmd := exec.Command(
    filepath.Join(rootDir, "tools", "pdf-generator", ".venv", "bin", "pdf-generator"),
    configPath,
)
```

**After** (Option 2 - Using module):
```go
pythonBin := filepath.Join(rootDir, "tools", "pdf-generator", ".venv", "bin", "python")
cmd := exec.Command(
    pythonBin,
    "-m", "pdf_generator.cli.main",
    configPath,
)
cmd.Dir = filepath.Join(rootDir, "tools", "pdf-generator")
```

**Example Go helper function**:

```go
package report

import (
    "os"
    "os/exec"
    "path/filepath"
)

// GeneratePDFReport calls the Python PDF generator with the given config.
func GeneratePDFReport(configPath string) error {
    // Get repository root
    rootDir, err := os.Getwd()
    if err != nil {
        return fmt.Errorf("failed to get working directory: %w", err)
    }

    // Path to PDF generator
    pdfDir := filepath.Join(rootDir, "tools", "pdf-generator")
    pythonBin := filepath.Join(pdfDir, ".venv", "bin", "python")

    // Execute PDF generator
    cmd := exec.Command(
        pythonBin,
        "-m", "pdf_generator.cli.main",
        configPath,
    )
    cmd.Dir = pdfDir
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("PDF generation failed: %w", err)
    }

    return nil
}
```

### Phase 11: Update Documentation (30 minutes)

Update the following files:

**File**: `tools/pdf-generator/README.md`

```markdown
# Velocity Report PDF Generator

Python tool for generating PDF reports from radar velocity data.

## Installation

```bash
# From repository root
make pdf-setup

# Or manually
cd tools/pdf-generator
python3 -m venv .venv
source .venv/bin/activate
pip install -e ".[dev]"
```

## Usage

### As Installed Package

```bash
# Generate report
pdf-generator config.json

# Create example config
pdf-config

# Run demo
pdf-demo
```

### As Python Module

```bash
cd tools/pdf-generator
python -m pdf_generator.cli.main config.json
python -m pdf_generator.cli.create_config
```

### Via Makefile (from repo root)

```bash
make pdf-config              # Create example config
make pdf-report CONFIG=config.json
make pdf-test                # Run tests
make pdf-test-cov            # Tests with coverage
```

## Development

```bash
# Install in development mode
make pdf-setup

# Run tests
make pdf-test

# Run specific test
cd tools/pdf-generator
pytest pdf_generator/tests/test_config_manager.py -v

# Check coverage
make pdf-test-cov
```

## Integration with Go Application

The Go application calls this tool via:

```go
import "os/exec"

cmd := exec.Command("python", "-m", "pdf_generator.cli.main", configPath)
cmd.Dir = "tools/pdf-generator"
```

## Package Structure

```
pdf_generator/
├── cli/                   # Command-line entry points
│   ├── main.py           # Main PDF generator CLI
│   ├── create_config.py  # Config template generator
│   └── demo.py           # Interactive demo
├── core/                  # Core functionality
│   ├── api_client.py     # API communication
│   ├── chart_builder.py  # Chart generation
│   ├── config_manager.py # Configuration system
│   ├── pdf_generator.py  # PDF orchestration
│   └── ...               # Other modules
└── tests/                 # Test suite
    ├── test_*.py         # Test modules
    └── conftest.py       # Pytest configuration
```

## Configuration

See `config.example.json` for a complete example configuration file.
```

### Phase 12: Verify Everything Works (30 minutes)

```bash
# From repository root

# 1. Install PDF generator
make pdf-setup

# 2. Run tests
make pdf-test

# 3. Create example config
make pdf-config

# 4. Generate a test report
make pdf-report CONFIG=tools/pdf-generator/config.example.json

# 5. Check that PDF was created
ls -lh tools/pdf-generator/output/*.pdf

# 6. Test from Go (if applicable)
# Run your Go application that generates PDFs
```

### Phase 13: Clean Up Old Location (15 minutes)

**Only after verifying everything works!**

```bash
# Backup first (just in case)
tar czf internal-report-query-data-backup.tar.gz internal/report/query_data/

# Remove old Python code
# Keep the directory structure if Go code needs it
rm -rf internal/report/query_data/*.py
rm -rf internal/report/query_data/test_*.py
rm -rf internal/report/query_data/__pycache__
rm -rf internal/report/query_data/.pytest_cache
rm -rf internal/report/query_data/htmlcov

# Or remove entirely if not needed
# rm -rf internal/report/query_data/
```

---

## Post-Migration Checklist

- [ ] PDF generator installed: `make pdf-setup` completes successfully
- [ ] All tests pass: `make pdf-test` shows 451 passed
- [ ] Can generate PDFs: `make pdf-report CONFIG=...` creates PDF
- [ ] Console scripts work: `pdf-generator --help` runs
- [ ] Go integration updated and tested
- [ ] Documentation updated
- [ ] CI/CD updated (if applicable)
- [ ] Old location cleaned up
- [ ] Team notified of new structure

---

## Usage Examples

### Developer Workflow

```bash
# Setup (one time)
make pdf-setup

# Daily usage
make pdf-config                           # Create config
vim tools/pdf-generator/config.json       # Edit as needed
make pdf-report CONFIG=tools/pdf-generator/config.json

# Testing
make pdf-test                             # Run all tests
make pdf-test-cov                         # With coverage
cd tools/pdf-generator
pytest pdf_generator/tests/test_config_manager.py::TestConfigManager::test_validation_success -v
```

### CI/CD Pipeline

```yaml
# .github/workflows/test.yml
jobs:
  test-pdf-generator:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.11'

      - name: Install PDF Generator
        run: make pdf-setup

      - name: Run Tests
        run: make pdf-test-cov

      - name: Upload Coverage
        uses: codecov/codecov-action@v3
        with:
          files: tools/pdf-generator/htmlcov/coverage.xml
```

---

## Troubleshooting

### Import Errors

If you see `ModuleNotFoundError: No module named 'pdf_generator'`:

```bash
cd tools/pdf-generator
pip install -e .
```

### Tests Failing

If tests can't find modules:

```bash
cd tools/pdf-generator
export PYTHONPATH="${PYTHONPATH}:$(pwd)"
pytest
```

Or use the Makefile:
```bash
make pdf-test
```

### Go Can't Find Python

Update your Go code to use absolute paths:

```go
rootDir, _ := os.Getwd()
pythonBin := filepath.Join(rootDir, "tools", "pdf-generator", ".venv", "bin", "python")
```

---

## Timeline

- **Phase 1-3**: Create structure and copy files (1 hour)
- **Phase 4-5**: Update imports and entry points (1.5 hours)
- **Phase 6-8**: Install, test, verify (30 minutes)
- **Phase 9**: Add Makefile commands (15 minutes)
- **Phase 10**: Update Go integration (30 minutes)
- **Phase 11**: Update documentation (30 minutes)
- **Phase 12**: Final verification (30 minutes)
- **Phase 13**: Clean up old location (15 minutes)

**Total**: ~5 hours for careful, thorough migration

---

## Benefits Realized

After completion, you'll have:

✅ **Proper separation**: Go code in `internal/`, Python tool in `tools/`
✅ **Professional structure**: Follows Python packaging best practices
✅ **Easy commands**: `make pdf-report CONFIG=config.json`
✅ **Console scripts**: `pdf-generator config.json`
✅ **Clean imports**: `from pdf_generator.core import ChartBuilder`
✅ **Isolated testing**: Tests don't interfere with Go tests
✅ **Better development**: Can install package in other projects
✅ **Clear documentation**: Single source of truth in `tools/pdf-generator/`

---

## Questions?

- **Do I have to do this all at once?** Yes, because of import changes. But you can test thoroughly between phases.
- **Can I keep the old location as backup?** Yes! Don't delete until everything is verified.
- **Will this break existing functionality?** Not if you update Go integration code.
- **Can I skip the package structure?** Not recommended. The import updates are the same effort either way.

---

## See Also

- `docs/REMOVAL_report_config.md` - Recent deprecation removal
- `docs/REMOVAL_generate_report_api.md` - Web API removal
- Original proposal: `PROPOSAL_USABILITY_IMPROVEMENTS.md`
