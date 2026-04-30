# Python developer tooling

- **Production status:** Python is not installed on Raspberry Pi images (removed in v0.5). All production PDF generation runs in Go (`internal/report/`).
- **Dev status:** Python is used for CI linting scripts, data exploration, hardware documentation tools, and CAD rendering. All tooling is developer-only.

## Two tiers of Python usage

### Tier 1 — System `python3` (no setup required)

The doc-linting and CI scripts in `scripts/` are invoked with system `python3` directly. No venv or install step is needed.

```
scripts/check-mermaid-blocks.py       # make lint-docs
scripts/check-prose-line-width.py     # make lint-docs
scripts/check-plan-canonical-links.py # make lint-docs
scripts/check-relative-links.py       # make lint-docs
scripts/check-backtick-paths.py       # make lint-docs
scripts/check-doc-header-metadata.py  # make lint-docs / make format-docs
scripts/check-british-spelling.py     # make lint-docs
scripts/check-release-hashes.py       # make lint-docs
scripts/update-release-json.py        # make update-release-json
scripts/check-action-pins.py          # CI
```

These scripts have no third-party dependencies beyond the standard library or packages available on the CI runner.

### Tier 2 — Shared `.venv/` (run `make install-python`)

Everything else uses the single shared virtual environment at `.venv/` in the project root:

| Tool / directory                          | Purpose                                                                  |
| ----------------------------------------- | ------------------------------------------------------------------------ |
| `black`, `ruff`                           | Python formatting and linting (`make format-python`, `make lint-python`) |
| `tools/grid-heatmap/`                     | LiDAR grid visualisation for field analysis                              |
| `tools/rack-drawing/`                     | Hardware rack diagram generation                                         |
| `tools/connector-pinouts/`                | Connector pinout documentation SVGs                                      |
| `tools/_render/svg_to_png.py`             | SVG rasterisation helper                                                 |
| `tools/guide-overlays/`                   | Sensor positioning overlay drawings                                      |
| `data/explore/`                           | Research data analysis (matplotlib, scipy, pandas)                       |
| `build123d` (via `make install-diagrams`) | 3D CAD rendering for hardware docs                                       |

## Setup

```bash
make install-python       # Creates .venv/, installs requirements.txt
make install-diagrams     # Adds build123d to .venv/ (optional, for CAD rendering)
```

`make install-python` tries `python3.12`, falls back to `python3`. Reuses an existing venv if the Python version matches.

## Repository layout

```
velocity.report/
├── .venv/                    # Shared virtual environment (not committed)
├── requirements.txt          # Pinned deps (pip-compile output)
├── tools/grid-heatmap/       # Uses root .venv
├── tools/rack-drawing/       # Uses root .venv
├── tools/connector-pinouts/  # Uses root .venv
└── data/explore/             # Uses root .venv
```

`data/explore/align/` has its own `pyproject.toml` (requires Python 3.9+, different deps); it is not part of the shared venv.

## Makefile variables

| Variable         | Value                     |
| ---------------- | ------------------------- |
| `VENV_DIR`       | `.venv`                   |
| `VENV_PYTHON`    | `$(VENV_DIR)/bin/python3` |
| `VENV_PIP`       | `$(VENV_DIR)/bin/pip`     |
| `VENV_PYTEST`    | `$(VENV_DIR)/bin/pytest`  |
| `PYTHON_VERSION` | `3.12`                    |

## Makefile targets

| Target                  | What it does                                      |
| ----------------------- | ------------------------------------------------- |
| `make install-python`   | Create/reuse `.venv/`, install `requirements.txt` |
| `make install-diagrams` | Add `build123d` to `.venv/` for CAD rendering     |
| `make format-python`    | Run `black` + `ruff --fix` across all Python      |
| `make lint-python`      | Run `black --check` + `ruff` (non-mutating)       |
| `make test-python`      | Run Python script/tool tests                      |
| `make test-python-cov`  | Run Python script/tool tests with HTML coverage   |

`test-python` is **not** included in the `make test` aggregate. The pdf-generator it previously exercised has been deleted from the repository; the target now covers the remaining repo-native Python tooling.

## Dependency management

`requirements.txt` is pip-compile output. To add a dependency:

1. Edit `requirements.in` (human-editable source list)
2. Run `pip-compile requirements.in` to regenerate `requirements.txt`
3. Commit both files

Key dependency groups:

- **Data analysis:** pandas, numpy, scipy
- **Visualisation:** matplotlib, seaborn
- **Testing:** pytest, pytest-cov
- **Formatting:** black, ruff

## Removed pdf-generator

`tools/pdf-generator/` was the Python PDF pipeline, superseded by Go in v0.5 and deleted from the repository in v0.6. The Go pipeline (`internal/report/`) is the sole PDF generation path. `requirements.txt` may still list PyLaTeX/reportlab as historical artefacts; they are unused.
