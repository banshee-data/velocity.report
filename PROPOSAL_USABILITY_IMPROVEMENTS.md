# Proposal: Usability & Organization Improvements

**Date**: October 11, 2025
**Status**: Proposed
**Scope**: Python PDF Generator Utility (within Go monorepo)

## Executive Summary

This proposal addresses ease of use and organization for the **Python PDF generation utility** within the velocity.report Go monorepo. The Python component is a called generator/utility, not the main project. Changes should be scoped to improve the Python tooling without taking over the repository structure.

**Key Principle**: This is a Go project first. Python is a utility that generates PDFs when called by the Go application.

---

## 1. Python Equivalent to `npm run` Commands

### Current State
- Python PDF generator called from Go backend
- Users must remember full paths for standalone usage: `python internal/report/query_data/get_stats.py config.json`
- No standard command interface for Python utilities
- Makefile exists but only has Go build commands

### Context: Go Monorepo Structure
```
velocity.report/                 # Go project root
├── cmd/                         # Go executables
│   ├── radar/                   # Main Go application
│   ├── bg-sweep/
│   └── bg-multisweep/
├── internal/                    # Go internal packages
│   ├── api/                     # Go API
│   ├── db/                      # Go database
│   ├── lidar/                   # Go lidar
│   ├── radar/                   # Go radar
│   └── report/
│       └── query_data/          # Python PDF generator (feels out of place)
├── web/                         # Frontend (Svelte)
└── Makefile                     # Go build commands
```

**Issue**: Python PDF generator is buried in `internal/report/query_data/` which is confusing because:
- `internal/` is a Go convention for internal packages
- Python is not an "internal Go package"
- It's a standalone utility that could be in any language

### Proposed Solutions

#### Option A: **Makefile Commands Only** (Recommended - Minimal Impact)

Keep Python as a utility, add Makefile commands for convenience.

**File**: `Makefile` (extend existing)

```makefile
# =============================================================================
# Python PDF Generator Utilities
# =============================================================================

.PHONY: pdf-setup pdf-test pdf-report pdf-config pdf-check pdf-clean

PDF_DIR = tools/pdf-generator

pdf-setup:
	@echo "Setting up PDF generator..."
	cd $(PDF_DIR) && python3 -m venv .venv
	cd $(PDF_DIR) && .venv/bin/pip install -r requirements.txt
	@echo "✓ PDF generator setup complete"

pdf-test:
	@echo "Testing PDF generator..."
	cd $(PDF_DIR) && .venv/bin/pytest -v

pdf-test-quick:
	cd $(PDF_DIR) && .venv/bin/pytest -q

pdf-report:
	@if [ -z "$(CONFIG)" ]; then \
		echo "Error: CONFIG required. Usage: make pdf-report CONFIG=config.json"; \
		exit 1; \
	fi
	cd $(PDF_DIR) && .venv/bin/python -m pdf_generator.cli.main $(CONFIG)

pdf-config:
	cd $(PDF_DIR) && .venv/bin/python -m pdf_generator.cli.create_config

pdf-check:
	cd $(PDF_DIR) && .venv/bin/python -m pdf_generator.cli.main --check

pdf-clean:
	rm -rf $(PDF_DIR)/output/*.pdf
	rm -rf $(PDF_DIR)/output/*.tex
	rm -rf $(PDF_DIR)/.pytest_cache
	rm -rf $(PDF_DIR)/htmlcov
```

**Usage**:
```bash
make pdf-setup              # One-time setup
make pdf-config             # Create example config
make pdf-report CONFIG=my-config.json
make pdf-test               # Run tests
```

#### Option B: **Standalone Python Package in tools/** (Cleaner Separation)

Move Python to its own directory as a proper tool.

**New Structure**:
```
velocity.report/                 # Go project root
├── cmd/                         # Go executables
├── internal/                    # Go internal packages (Go only!)
├── web/                         # Frontend
├── tools/                       # Non-Go utilities
│   └── pdf-generator/          # Python PDF generator
│       ├── pyproject.toml
│       ├── requirements.txt
│       ├── pdf_generator/      # Python package
│       │   ├── __init__.py
│       │   ├── cli/           # Entry points
│       │   ├── core/          # Internal modules
│       │   └── tests/         # Tests
│       └── output/            # Generated PDFs
└── Makefile
```

This creates a **proper Python package** but keeps it scoped as a tool:

**File**: `tools/pdf-generator/pyproject.toml`

```toml
[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"

[project]
name = "velocity-report-pdf-generator"
version = "1.0.0"
description = "PDF report generator for velocity.report Go application"
requires-python = ">=3.11"
dependencies = [
    "matplotlib>=3.10.6",
    "numpy>=2.3.3",
    "pandas>=2.3.2",
    "pylatex>=1.4.2",
    "requests>=2.32.3",
    "seaborn>=0.13.3",
]

# Console entry points (only for local development/testing)
[project.scripts]
pdf-generator = "pdf_generator.cli.main:main"
pdf-config = "pdf_generator.cli.create_config:main"

[tool.pytest.ini_options]
testpaths = ["pdf_generator/tests"]
```

**Integration with Go**:
```go
// In your Go application, call the Python generator:
cmd := exec.Command(
    "python3", "-m", "pdf_generator.cli.main",
    configPath,
)
cmd.Dir = "/path/to/tools/pdf-generator"
```

---

## 2. Reorganize Python as a Tool (Not Internal Go Package)

### Current Issues

**Problem**: Python PDF generator incorrectly placed in Go `internal/` namespace
```
internal/
├── api/          # Go API package
├── db/           # Go database package
├── lidar/        # Go lidar package
├── radar/        # Go radar package
└── report/
    └── query_data/    # ❌ Python code (doesn't belong in Go internal/)
```

**Issues**:
- `internal/` is a Go-specific convention for unexported packages
- Mixing Go namespaces with Python creates confusion
- Python generator is a utility/tool, not a Go package
- Generated PDFs pollute source directory
- 147 files in one flat directory (30+ modules + tests + generated files)

### Proposed Structure: Move to `tools/`

**Option A: Minimal Reorganization** (Quick, low risk)

```
velocity.report/                 # Go project root
├── cmd/                         # Go executables
├── internal/                    # Go packages only
│   ├── api/
│   ├── db/
│   ├── lidar/
│   └── radar/
├── web/                         # Frontend
├── tools/                       # Non-Go utilities
│   └── pdf-generator/          # Python PDF generator
│       ├── .venv/              # Python virtual env
│       ├── requirements.txt
│       ├── README.md
│       ├── get_stats.py        # Main CLI (keep name for now)
│       ├── generate_report_api.py  # API endpoint
│       ├── create_config_example.py
│       ├── *.py                # Other Python modules (30+ files)
│       ├── tests/              # Move all test_*.py here
│       ├── output/             # Generated PDFs go here
│       ├── fonts/
│       └── docs/
└── Makefile
```

**Migration Steps**:
```bash
# Create new structure
mkdir -p tools/pdf-generator/{tests,output,fonts,docs}

# Move Python code
mv internal/report/query_data/* tools/pdf-generator/
mv tools/pdf-generator/test_*.py tools/pdf-generator/tests/
mv tools/pdf-generator/conftest.py tools/pdf-generator/tests/

# Update references in Makefile
sed -i '' 's|internal/report/query_data|tools/pdf-generator|g' Makefile

# Create output directory gitignore
echo "*.pdf\n*.tex\n*.svg\n!.gitignore" > tools/pdf-generator/output/.gitignore
```

**Option B: Proper Python Package** (Cleaner, more work)

```
tools/
└── pdf-generator/
    ├── pyproject.toml          # Python package config
    ├── requirements.txt
    ├── README.md
    ├── pdf_generator/          # Actual Python package
    │   ├── __init__.py
    │   ├── cli/               # Entry points (separate from internal)
    │   │   ├── __init__.py
    │   │   ├── main.py        # Main CLI
    │   │   ├── create_config.py
    │   │   └── api_server.py  # API endpoint for Go
    │   ├── core/              # Internal modules
    │   │   ├── __init__.py
    │   │   ├── api_client.py
    │   │   ├── chart_builder.py
    │   │   ├── config_manager.py
    │   │   └── ... (30 modules)
    │   └── tests/             # Tests
    │       ├── __init__.py
    │       └── test_*.py
    ├── output/                # Generated PDFs
    └── fonts/                 # Resources
```

---

## 3. Go Integration Considerations

### Current Integration (Assumed)

The Go application likely calls the Python generator via:

```go
// Somewhere in your Go code
cmd := exec.Command(
    "python3",
    "internal/report/query_data/get_stats.py",
    configPath,
)
output, err := cmd.CombinedOutput()
```

### After Moving to `tools/pdf-generator/`

**Option A: Update Go to call new path**

```go
// Update Go code to point to new location
cmd := exec.Command(
    "python3",
    filepath.Join("tools", "pdf-generator", "get_stats.py"),
    configPath,
)
cmd.Dir = rootDir  // Set working directory
```

**Option B: Use Python module syntax** (cleaner)

```go
cmd := exec.Command(
    "python3", "-m", "pdf_generator.cli.main",
    configPath,
)
cmd.Dir = filepath.Join(rootDir, "tools", "pdf-generator")
```

**Option C: Create wrapper script** (backward compatible)

Keep a shim at the old location:

**File**: `internal/report/query_data/get_stats.py` (wrapper)
```python
#!/usr/bin/env python3
"""Legacy wrapper for backward compatibility."""
import sys
import subprocess
from pathlib import Path

# Call the real implementation
root = Path(__file__).parent.parent.parent.parent
script = root / "tools" / "pdf-generator" / "get_stats.py"
sys.exit(subprocess.call([sys.executable, str(script)] + sys.argv[1:]))
```

### API Server Integration

If your Go app calls the Python API server (`generate_report_api.py`):

**Before**:
```go
cmd := exec.Command(
    "python3",
    "internal/report/query_data/generate_report_api.py",
    configPath,
)
```

**After** (Option A - direct path):
```go
cmd := exec.Command(
    "python3",
    "tools/pdf-generator/generate_report_api.py",
    configPath,
)
```

**After** (Option B - module):
```go
cmd := exec.Command(
    "python3", "-m", "pdf_generator.cli.api_server",
    configPath,
)
cmd.Dir = "tools/pdf-generator"
```

### Environment Isolation

Since this is called from Go, ensure Python venv is used:

```go
// Use the venv Python directly
pythonBin := filepath.Join(
    rootDir,
    "tools",
    "pdf-generator",
    ".venv",
    "bin",
    "python3",
)

cmd := exec.Command(
    pythonBin,
    "-m", "pdf_generator.cli.main",
    configPath,
)
```

---

## 4. Output Management

### Current Issue
Generated PDFs, LaTeX files, and test artifacts pollute the source directory.

### Proposed Solution

Create dedicated output directory in the PDF generator tool:

**Structure**:
```
tools/pdf-generator/
├── output/                     # All generated files
│   ├── .gitignore
│   ├── *.pdf                   # Generated PDFs
│   ├── *.tex                   # LaTeX intermediate files
│   └── *.svg                   # Generated maps
├── get_stats.py
└── ...
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

# Keep the directory
!.gitignore
```

**Update config default**:

**File**: `tools/pdf-generator/config_manager.py`

```python
@dataclass
class OutputConfig:
    """Output configuration."""

    file_prefix: Optional[str] = None
    debug: bool = False
    map: bool = True
    output_dir: str = "output"  # Relative to pdf-generator directory
    clean_on_success: bool = False
```

---

## 5. Recommended Implementation Order

### Phase 1: Move to tools/ (2-3 hours) ⭐ **RECOMMENDED START**

1. ✅ Create `tools/pdf-generator/` directory structure
2. ✅ Move all Python code from `internal/report/query_data/` to `tools/pdf-generator/`
3. ✅ Update Go code to call new path
4. ✅ Add Makefile commands for PDF generator
5. ✅ Update documentation

**Benefits**:
- Proper separation of Go and Python
- Respects monorepo conventions
- Minimal code changes (just paths)

### Phase 2: Clean up output (1 hour)

1. Create `tools/pdf-generator/output/` directory
2. Update `OutputConfig` default to use `output/`
3. Move test files to `tools/pdf-generator/tests/`
4. Update pytest configuration

**Benefits**:
- Clean source directory
- Better organization
- Tests separated from source

### Phase 3: Optional - Proper Python Package (4-6 hours)

Only if you want to distribute the PDF generator separately or use it in other projects.

1. Create `tools/pdf-generator/pdf_generator/` package structure
2. Reorganize into `cli/` and `core/` subdirectories
3. Add `pyproject.toml`
4. Update imports
5. Install with `pip install -e tools/pdf-generator/`

**Benefits**:
- Can install as standalone tool
- Better Python practices
- Console entry points

---

## 6. Benefits Summary

### For Go Developers
- ✅ **Clear separation**: Go code in `cmd/` and `internal/`, Python in `tools/`
- ✅ **Monorepo best practice**: Tools in dedicated `tools/` directory
- ✅ **Simple integration**: Call Python generator via known path
- ✅ **No namespace pollution**: `internal/` is Go-only

### For Python Developers
- ✅ **Proper structure**: Tests, source, output separated
- ✅ **Standard commands**: `make pdf-test`, `make pdf-report`
- ✅ **Clean workspace**: Generated files in `output/` directory
- ✅ **Familiar patterns**: Follows Python packaging conventions

### For DevOps/CI
- ✅ **Predictable paths**: `tools/pdf-generator/` is self-contained
- ✅ **Easy automation**: Makefile targets for CI pipelines
- ✅ **Isolated dependencies**: Python venv in tool directory
- ✅ **Clear boundaries**: Testing PDF generator doesn't run Go tests

---

## 7. Example Usage (After Implementation)

### Go Application Calls PDF Generator

```go
package main

import (
    "os/exec"
    "path/filepath"
)

func GeneratePDFReport(configPath string) error {
    // Get repository root
    rootDir, _ := os.Getwd()

    // Path to Python generator
    pythonBin := filepath.Join(rootDir, "tools", "pdf-generator", ".venv", "bin", "python3")
    script := filepath.Join(rootDir, "tools", "pdf-generator", "get_stats.py")

    // Execute PDF generator
    cmd := exec.Command(pythonBin, script, configPath)
    cmd.Dir = filepath.Join(rootDir, "tools", "pdf-generator")

    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("PDF generation failed: %v\n%s", err, output)
    }

    return nil
}
```

### Developer Workflow

```bash
# One-time setup
make pdf-setup

# Generate PDFs via Makefile
make pdf-report CONFIG=config.json

# Or call Python directly (from tools/pdf-generator/)
cd tools/pdf-generator
.venv/bin/python get_stats.py config.json

# Run tests
make pdf-test

# Create config template
make pdf-config
```

### CI/CD Pipeline

```yaml
# .github/workflows/test.yml
jobs:
  test-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Test Go
        run: make test

  test-python-pdf-generator:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Setup PDF Generator
        run: make pdf-setup
      - name: Test PDF Generator
        run: make pdf-test
```

---

## 8. Questions for Discussion

1. **Move to tools/ now or later?**
   - **Recommended**: Do it now (2-3 hours, clean separation)
   - Alternative: Keep in `internal/` and just add Makefile commands
   - Consideration: Any hard-coded paths in Go that need updating?

2. **Naming preference for the tool?**
   - `tools/pdf-generator/` (generic, clear purpose)
   - `tools/velocity-pdf-generator/` (branded, more specific)
   - `tools/report-generator/` (might do more than PDFs later)

3. **Go integration method?**
   - Direct path: `python3 tools/pdf-generator/get_stats.py`
   - Module syntax: `python3 -m pdf_generator.cli.main` (requires pyproject.toml)
   - Wrapper script: Keep shim at old location for backward compat

4. **Output directory location?**
   - `tools/pdf-generator/output/` (co-located with tool) ⭐ **Recommended**
   - `./reports/` or `./output/` (repository root)
   - Configurable via config file

5. **Backward compatibility needed?**
   - Can we break existing Go code paths?
   - Any external scripts/CI that call Python directly?
   - Documentation updates needed?

---

## Appendix: Migration Script Template

```python
#!/usr/bin/env python3
"""Migrate to new directory structure."""

import shutil
from pathlib import Path

def migrate():
    base = Path("internal/report/query_data")

    # Create new directories
    for dir_name in ["cli", "api", "core", "tests", "output"]:
        (base / dir_name).mkdir(exist_ok=True)

    # Move CLI entry points
    moves = {
        "get_stats.py": "cli/main.py",
        "create_config_example.py": "cli/create_config.py",
        "dependency_checker.py": "cli/check_deps.py",
        "generate_report_api.py": "api/server.py",
    }

    for src, dst in moves.items():
        shutil.move(base / src, base / dst)

    # Move internal modules to core/
    internal_modules = [
        "api_client.py", "chart_builder.py", "chart_saver.py",
        "config_manager.py", "data_transformers.py", "date_parser.py",
        "document_builder.py", "map_utils.py", "pdf_generator.py",
        "report_sections.py", "stats_utils.py", "table_builders.py",
        "report_config.py",  # deprecated but keep for now
    ]

    for module in internal_modules:
        shutil.move(base / module, base / "core" / module)

    # Move tests
    for test_file in base.glob("test_*.py"):
        shutil.move(test_file, base / "tests" / test_file.name)

    shutil.move(base / "conftest.py", base / "tests" / "conftest.py")

    print("Migration complete!")

if __name__ == "__main__":
    migrate()
```
