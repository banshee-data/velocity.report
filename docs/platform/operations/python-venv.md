# Python Virtual Environment — Single Shared `.venv`

Active plan: [tooling-python-venv-consolidation-plan.md](../../plans/tooling-python-venv-consolidation-plan.md)

**Status:** Complete

## Architecture

The repository uses a single shared Python virtual environment at `.venv/`
for all Python tools (PDF generator, data visualisation, analysis scripts).

```
velocity.report/
├── .venv/                    # Single shared environment
├── requirements.in            # Human-editable dependency list
├── requirements.txt           # Pinned versions (pip-compile)
├── tools/pdf-generator/       # Uses root .venv
├── tools/grid-heatmap/        # Uses root .venv
└── data/multisweep-graph/     # Uses root .venv
```

### Key Facts

- **One command:** `make install-python` sets up everything
- **Single source of truth:** `requirements.in` at repository root
- **Pinned dependencies:** `requirements.txt` generated with `pip-compile`
- All Makefile Python targets use `VENV_PYTHON = .venv/bin/python3`

## Dependency Management

Root `requirements.in` includes all packages for all Python tools:

- PDF generation: PyLaTeX, reportlab
- Data analysis: pandas, numpy, scipy
- Visualisation: matplotlib, seaborn
- Testing: pytest, pytest-cov
- Formatting: black, ruff

## Go Server Integration

The Go server finds the Python binary at:

```go
defaultPythonBin := filepath.Join(repoRoot, ".venv", "bin", "python")
```

## Makefile Variables

```makefile
VENV_DIR    = .venv
VENV_PYTHON = $(VENV_DIR)/bin/python3
VENV_PIP    = $(VENV_DIR)/bin/pip
VENV_PYTEST = $(VENV_DIR)/bin/pytest
```

## Consolidation Background

Previously the repo had two conflicting venv approaches: a root-level `.venv/`
for data visualisation and a `tools/pdf-generator/.venv/` for PDF generation.
This caused duplicate dependency management, confusing scripts, and wasted
disk space. The consolidation merged all requirements into the root `.venv/`.
