# PDF reporting: Go migration

Active plan: [pdf-go-chart-migration-plan.md](../../plans/pdf-go-chart-migration-plan.md)

Migrating PDF report generation from the Python stack to native Go, eliminating the Python runtime dependency and enabling the single-binary deployment goal.

## Problem

The Python PDF stack adds ~45 packages, a virtual-environment lifecycle, and
a separate runtime to every deployment. It is the only reason Raspberry Pi
images ship Python, and it complicates the single-binary goal (D-09).

## Solution

Generate SVG charts in Go, emit `.tex` files from Go `text/template`, and
invoke `xelatex` to produce the final PDF. No Python required.

### Key changes

| Component           | Before (Python)                         | After (Go)                                       |
| ------------------- | --------------------------------------- | ------------------------------------------------ |
| **Charts**          | matplotlib + seaborn → PDF figures      | `gonum/plot` (vgsvg) → SVG → PDF via `rsvg`      |
| **Doc assembly**    | PyLaTeX `Document` builder              | Go `text/template` → `.tex` file                 |
| **PDF compilation** | PyLaTeX shells out to `xelatex`         | Go `os/exec` shells out to `xelatex` (unchanged) |
| **Config**          | JSON → Python dataclasses               | JSON → Go structs (ReportRequest already exists) |
| **Data source**     | HTTP GET `/api/radar_stats` from Python | Direct DB query from same Go process             |
| **Runtime deps**    | Python 3.12 + .venv + 45 packages       | None (charts compiled into Go binary)            |
| **Report archive**  | `.zip` with `.tex` + chart PDFs         | `.zip` with `.tex` + chart SVGs                  |

## Current vs proposed architecture

### Current data path

```
Web UI → POST /api/generate_report → Go writes config.json
  → exec.Command("python", "-m", "pdf_generator.cli.main")
  → Python: api_client.py → GET /api/radar_stats (HTTP to self)
  → Python: chart_builder.py → matplotlib figures (PDF)
  → Python: document_builder.py → PyLaTeX Document
  → PyLaTeX writes .tex, shells out to xelatex → .pdf
```

The Python process makes an HTTP request back to the Go server that spawned
it. This round-trip is eliminated in the new design.

### Proposed data path

```
Web UI → POST /api/generate_report (or CLI: velocity-report pdf)
  → Go: internal/report/report.go → direct DB query
  → Go: chart/*.go → SVG charts → rsvg-convert → PDF charts
  → Go: tex/render.go → text/template → .tex file
  → Go: os/exec → xelatex → .pdf
```

## Package layout

```
internal/report/
├── report.go           # Generate(ctx, db, cfg) → (ReportResult, error)
├── config.go           # ReportConfig, ChartStyle, FontConfig, LayoutConfig
├── chart/
│   ├── timeseries.go   # Dual-axis percentile + count SVG
│   ├── histogram.go    # Velocity distribution SVG
│   ├── comparison.go   # Side-by-side comparison SVG
│   ├── palette.go      # Colour constants (matching web palette)
│   └── svg.go          # Low-level SVG element helpers
├── tex/
│   ├── render.go       # Template executor
│   ├── helpers.go      # EscapeTeX(), FormatTable(), FormatNumber()
│   └── templates/      # Embedded via go:embed
│       ├── preamble.tex
│       ├── report.tex
│       ├── overview.tex, site_info.tex, chart_section.tex
│       ├── statistics.tex, science.tex
└── archive.go          # .zip packaging
```

## Chart-by-Chart migration

### 1. Time-Series chart (dual-axis percentile + count)

Current: 24.0 × 8.0 inch matplotlib figure with left Y-axis (P50/P85/P98/Max
speed lines with markers), right Y-axis (count bars, translucent), orange
background for low-sample periods (< 50 count), broken lines at day
boundaries, custom X-axis (`HH:MM` with `Mon DD` at day starts).

Go approach: `gonum/plot` with `vgsvg` backend, or direct SVG via
`encoding/xml` / `ajstarks/svgo` for full control over dual-axis layout.
gonum/plot does not have built-in dual-axis support; render two plots to
the same SVG canvas sharing the X dimension.

### 2. Histogram (single period)

Straightforward bar chart: steelblue bars (α=0.7, black edge), speed bucket
labels ("20–25", "70+"). `gonum/plot` with `plotter.BarChart` handles
directly.

### 3. Comparison histogram

Side-by-side grouped bars (primary vs comparison, normalised to percentage).
`gonum/plot` supports grouped bar charts with offset positioning.

### 4. Map overlay

SVG marker injection (radar-coverage triangles into site map SVGs) via Go
`encoding/xml`. Same `rsvg-convert` pipeline for SVG→PDF.

## SVG-to-PDF strategy (chosen: rsvg-convert)

XeTeX's `\includegraphics` does not natively handle SVG. Use `rsvg-convert`
(from `librsvg`, ~2 MB) as a lightweight converter:

The conversion is performed by calling `rsvg-convert -f pdf -o chart.pdf chart.svg`.

- Already available on most Linux distributions (`librsvg2-bin`)
- ~2 MB installed (vs ~300 MB for inkscape)
- Already used as fallback in current Python `map_utils.py`
- SVG artefact preserved for `.zip` archive and web frontend reuse

Fallback: gonum/plot `vgpdf` for direct PDF output (skips SVG artefact).

## LaTeX template design

Replace PyLaTeX's programmatic construction with Go `text/template` files
embedded via `go:embed`. Use custom delimiters `<<` and `>>` (via
`template.Delims`) to avoid clashing with LaTeX `{`/`}`.

### Template data structure

| Field              | Type          | Purpose                             |
| ------------------ | ------------- | ----------------------------------- |
| `Location`         | `string`      | Site name                           |
| `Surveyor`         | `string`      | Surveyor name                       |
| `Contact`          | `string`      | Contact information                 |
| `Description`      | `string`      | Site description                    |
| `SpeedLimit`       | `int`         | Posted speed limit                  |
| `StartDate`        | `string`      | Report period start                 |
| `EndDate`          | `string`      | Report period end                   |
| `Timezone`         | `string`      | Display timezone                    |
| `Units`            | `string`      | Speed units                         |
| `P50`, `P85`, etc. | `string`      | Formatted speed percentiles and max |
| `TotalCount`       | `int`         | Total vehicle count                 |
| `HoursCount`       | `int`         | Number of hours with data           |
| `TimeSeriesChart`  | `string`      | Relative path to time-series chart  |
| `HistogramChart`   | `string`      | Relative path to histogram chart    |
| `CompareChart`     | `string`      | Optional comparison chart path      |
| `MapChart`         | `string`      | Optional map chart path             |
| `FontDir`          | `string`      | Font directory path                 |
| `HourlyTable`      | `[]HourlyRow` | Hourly breakdown rows               |
| `DailyTable`       | `[]DailyRow`  | Daily breakdown rows                |
| `CosineAngle`      | `float64`     | Angle correction value              |
| `CosineFactor`     | `float64`     | Cosine correction factor            |
| `ModelVersion`     | `string`      | Software version                    |

### Advantages over PyLaTeX

- Templates are plain `.tex` files, editable by anyone who knows LaTeX
- `go:embed` at compile time: zero disk I/O at runtime
- Deterministic output: byte-for-byte comparison in tests
- Go `text/template` is widely understood

## Colour palette (shared with web)

| Colour Name | Hex       | Usage           |
| ----------- | --------- | --------------- |
| `vrP50`     | `#fbd92f` | 50th percentile |
| `vrP85`     | `#f7b32b` | 85th percentile |
| `vrP98`     | `#f25f5c` | 98th percentile |
| `vrMax`     | `#2d1e2f` | Maximum speed   |

Font: Atkinson Hyperlegible (XeTeX `fontspec`).

## Python modules to replace

| Module                 | Lines | Replacement                                    |
| ---------------------- | ----- | ---------------------------------------------- |
| `chart_builder.py`     | ~900  | Go SVG chart package (`internal/report/chart`) |
| `chart_saver.py`       | ~185  | Go SVG writer + optional SVG→PDF conversion    |
| `pdf_generator.py`     | ~730  | Go template engine (`internal/report/tex`)     |
| `document_builder.py`  | ~350  | Go LaTeX preamble template                     |
| `report_sections.py`   | ~250  | Go section templates                           |
| `table_builders.py`    | ~200  | Go LaTeX table templates                       |
| `config_manager.py`    | ~530  | Go config struct (extend `ReportRequest`)      |
| `api_client.py`        | ~150  | Direct DB query (no HTTP)                      |
| `data_transformers.py` | ~200  | Go normalisation within chart package          |
| `map_utils.py`         | ~300  | Go `encoding/xml` SVG manipulation             |
| `zip_utils.py`         | ~80   | Go `archive/zip` (stdlib)                      |
| `cli/main.py`          | ~60   | Go `pdf` subcommand                            |

## Relationship to other decisions

- **D-08 (Precompiled LaTeX):** Complementary. D-08 reduces TeX from ~800 MB
  to ~30–60 MB. This plan eliminates Python (~450 MB). Together: ~1.25 GB →
  ~30–60 MB.
- **D-09 (Single Binary):** Enables `velocity-report pdf` without bundled
  Python.
- **D-10 (RPi Image):** Simplifies image by removing Python from report path.

## Risks

| Risk                              | Mitigation                                                           |
| --------------------------------- | -------------------------------------------------------------------- |
| gonum/plot dual-axis limitation   | Fall back to direct SVG via `encoding/xml`; prototype in Phase 1     |
| SVG→PDF fidelity via rsvg-convert | Use `--dpi 150`; test with Atkinson Hyperlegible embedded in SVG     |
| Chart visual parity               | Side-by-side comparison; accept minor styling diffs if data accurate |
| rsvg-convert not available        | Detect at startup; fall back to gonum `vgpdf` direct PDF output      |
| `text/template` delimiter clashes | Custom delimiters `<<`/`>>` via `template.Delims()`                  |
