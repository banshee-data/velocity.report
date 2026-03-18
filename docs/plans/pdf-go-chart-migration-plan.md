# PDF Generation Migration to Go

- **Status:** Draft — awaiting review before implementation
- **Layers:** Cross-cutting (reporting infrastructure)
- **Related:**

- [Precompiled LaTeX plan](pdf-latex-precompiled-format-plan.md) (D-08)
- [Distribution packaging plan](deploy-distribution-packaging-plan.md) (D-09)
- [RPi imager plan](deploy-rpi-imager-fork-plan.md) (D-10)
- [Platform simplification plan](platform-simplification-and-deprecation-plan.md)

**Goal:** Replace the Python PDF-generation stack (matplotlib, PyLaTeX, numpy,
pandas, seaborn — 45 transitive packages) with native Go SVG charting and Go
`text/template`-based LaTeX assembly, retaining XeTeX for typesetting and
producing SVG charts equivalent to the current matplotlib output.

---

## Quick Reference

### 30-Second Pitch

**Problem:** The Python PDF stack adds ~45 packages, a virtual-environment
lifecycle, and a separate runtime to every deployment. It is the only reason
Raspberry Pi images ship Python, and it complicates the single-binary goal
(D-09).

**Solution:** Generate SVG charts in Go, emit `.tex` files from Go templates,
and invoke `xelatex` to produce the final PDF. No Python required.

**Result:** One fewer runtime, faster report generation, simpler deployment,
and SVG charts usable in both PDF reports and the web frontend.

### Key Changes Summary

| Component           | Before (Python)                         | After (Go)                                       |
| ------------------- | --------------------------------------- | ------------------------------------------------ |
| **Charts**          | matplotlib + seaborn → PDF figures      | `gonum/plot` (vgsvg) → SVG → PDF via `rsvg`      |
| **Doc assembly**    | PyLaTeX `Document` builder              | Go `text/template` → `.tex` file                 |
| **PDF compilation** | PyLaTeX shells out to `xelatex`         | Go `os/exec` shells out to `xelatex` (unchanged) |
| **Config**          | JSON → Python dataclasses               | JSON → Go structs (ReportRequest already exists) |
| **Data source**     | HTTP GET `/api/radar_stats` from Python | Direct DB query from same Go process             |
| **Runtime deps**    | Python 3.12 + .venv + 45 packages       | None (charts compiled into Go binary)            |
| **Report archive**  | `.zip` with `.tex` + chart PDFs         | `.zip` with `.tex` + chart SVGs                  |

---

## Table of Contents

1. [Problem Summary](#problem-summary)
2. [Current Architecture](#current-architecture)
3. [Proposed Architecture](#proposed-architecture)
4. [Chart-by-Chart Migration](#chart-by-chart-migration)
5. [SVG-to-PDF Strategy](#svg-to-pdf-strategy)
6. [LaTeX Template Design](#latex-template-design)
7. [Implementation Phases](#implementation-phases)
8. [Testing and Validation](#testing-and-validation)
9. [Risks and Mitigations](#risks-and-mitigations)
10. [Relationship to Existing Decisions](#relationship-to-existing-decisions)
11. [Open Questions](#open-questions)

---

## Problem Summary

The PDF report pipeline currently requires:

| Dependency            | Size / Impact                                   |
| --------------------- | ----------------------------------------------- |
| Python 3.12 runtime   | ~50 MB on Raspberry Pi image                    |
| `.venv/` with 45 pkgs | ~400 MB uncompressed (matplotlib, numpy, scipy) |
| `texlive-xetex`       | ~800 MB (addressed separately by D-08)          |
| PyLaTeX               | Thin wrapper; but ties doc structure to Python  |

Eliminating Python removes the first two rows entirely. Combined with the
precompiled LaTeX plan (D-08), the total footprint drops from ~1.25 GB to
~30–60 MB (vendored TeX tree only).

### Motivation Beyond Size

1. **Single binary (D-09):** The `velocity-report pdf` subcommand can generate
   reports without shelling out to Python — just to `xelatex`.
2. **Faster generation:** No Python interpreter startup, no IPC marshalling
   through JSON config files, direct access to the database.
3. **Unified language:** Chart styling constants (colours, fonts, layout) live
   in one place, shared with the Svelte frontend palette.
4. **SVG reuse:** SVG charts can serve double duty — embedded in PDF reports
   via `\includegraphics` and served directly to the web frontend.

---

## Current Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Web UI  →  POST /api/generate_report                       │
└───────────┬─────────────────────────────────────────────────┘
            │  Go writes config.json
            ▼
┌─────────────────────────────────────────────────────────────┐
│  Go: exec.Command("python", "-m", "pdf_generator.cli.main") │
└───────────┬─────────────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────┐
│  Python: pdf_generator                                      │
│  ├── api_client.py  → GET /api/radar_stats (HTTP to self)   │
│  ├── chart_builder.py  → matplotlib figures (PDF)           │
│  ├── chart_saver.py  → save to disk                         │
│  ├── document_builder.py  → PyLaTeX Document                │
│  ├── pdf_generator.py  → assemble sections, embed charts    │
│  ├── table_builders.py  → LaTeX tables                      │
│  ├── report_sections.py  → section content                  │
│  └── map_utils.py  → SVG marker injection + SVG→PDF         │
└───────────┬─────────────────────────────────────────────────┘
            │  PyLaTeX writes .tex, shells out to xelatex
            ▼
┌─────────────────────────────────────────────────────────────┐
│  xelatex  → compiles .tex → .pdf                            │
└─────────────────────────────────────────────────────────────┘
```

**Data path:** Go server → HTTP → Python → HTTP → Go server → SQLite. The
Python process makes an HTTP request back to the Go server that spawned it.
This round-trip is eliminated in the new design.

### Python Modules to Replace

| Module                 | Lines | Replacement strategy                           |
| ---------------------- | ----- | ---------------------------------------------- |
| `chart_builder.py`     | ~900  | Go SVG chart package (`internal/report/chart`) |
| `chart_saver.py`       | ~185  | Go SVG writer + optional SVG→PDF conversion    |
| `pdf_generator.py`     | ~730  | Go template engine (`internal/report/tex`)     |
| `document_builder.py`  | ~350  | Go LaTeX preamble template                     |
| `report_sections.py`   | ~250  | Go section templates                           |
| `table_builders.py`    | ~200  | Go LaTeX table templates                       |
| `config_manager.py`    | ~530  | Go config struct (extend `ReportRequest`)      |
| `api_client.py`        | ~150  | Direct DB query (no HTTP)                      |
| `stats_utils.py`       | ~300  | Go formatting functions                        |
| `data_transformers.py` | ~200  | Go normalisation within chart package          |
| `date_parser.py`       | ~100  | Go `time.Parse` (already in server)            |
| `map_utils.py`         | ~300  | Reuse existing SVG blob + Go SVG manipulation  |
| `zip_utils.py`         | ~80   | Go `archive/zip` (stdlib)                      |
| `tex_environment.py`   | ~100  | Go TeX detection (align with D-08)             |
| `cli/main.py`          | ~60   | Go `pdf` subcommand                            |

---

## Proposed Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Web UI  →  POST /api/generate_report                       │
│  (or CLI: velocity-report pdf --config report.json)         │
└───────────┬─────────────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────┐
│  Go: internal/report/                                       │
│  ├── report.go          ← orchestrator (Generate entry pt)  │
│  ├── config.go          ← report configuration structs      │
│  ├── chart/                                                 │
│  │   ├── timeseries.go  ← dual-axis percentile + count SVG  │
│  │   ├── histogram.go   ← velocity distribution SVG         │
│  │   ├── palette.go     ← colour constants (from DESIGN.md) │
│  │   └── svg.go         ← SVG rendering helpers             │
│  ├── tex/                                                   │
│  │   ├── render.go      ← template executor                 │
│  │   ├── preamble.tex   ← LaTeX preamble template           │
│  │   ├── report.tex     ← main document template            │
│  │   ├── sections/      ← per-section .tex templates        │
│  │   └── helpers.go     ← LaTeX escaping, table formatting  │
│  └── archive.go         ← .zip packaging                    │
└───────────┬─────────────────────────────────────────────────┘
            │  Go writes .tex + .svg to temp dir
            ▼
┌─────────────────────────────────────────────────────────────┐
│  xelatex  → compiles .tex → .pdf                            │
│  (SVGs included via \includegraphics after svg→pdf convert) │
└─────────────────────────────────────────────────────────────┘
```

### Package Layout

```
internal/report/
├── report.go           # Generate(ctx, db, cfg) → (ReportResult, error)
├── report_test.go
├── config.go           # ReportConfig, ChartStyle, FontConfig, LayoutConfig
├── config_test.go
├── chart/
│   ├── timeseries.go   # RenderTimeSeries(data, style) → []byte (SVG)
│   ├── timeseries_test.go
│   ├── histogram.go    # RenderHistogram(data, style) → []byte (SVG)
│   ├── histogram_test.go
│   ├── comparison.go   # RenderComparison(a, b, style) → []byte (SVG)
│   ├── comparison_test.go
│   ├── palette.go      # Percentile colour constants, matching web palette
│   ├── palette_test.go
│   └── svg.go          # Low-level SVG element helpers
├── tex/
│   ├── render.go       # RenderTeX(cfg, charts, tables) → []byte (.tex)
│   ├── render_test.go
│   ├── helpers.go      # EscapeTeX(), FormatTable(), FormatNumber()
│   ├── helpers_test.go
│   └── templates/      # Embedded via go:embed
│       ├── preamble.tex
│       ├── report.tex
│       ├── overview.tex
│       ├── site_info.tex
│       ├── chart_section.tex
│       ├── statistics.tex
│       └── science.tex
├── archive.go          # BuildZip(texDir) → []byte
└── archive_test.go
```

---

## Chart-by-Chart Migration

### 1. Time-Series Chart (Dual-Axis Percentile + Count)

**Current (matplotlib):** 24.0 × 8.0 inch figure with:

- Left Y-axis: P50/P85/P98/Max speed lines with markers
- Right Y-axis: Count bars (translucent)
- Orange background bars for low-sample periods (< 50 count)
- Broken lines at day boundaries
- Custom X-axis: `HH:MM` with `Mon DD` at day starts
- Legend below chart

**Go equivalent (gonum/plot):**

`gonum/plot` supports all required primitives:

- `plotter.Line` for percentile lines with custom `draw.LineStyle`
- `plotter.BarChart` for count bars
- Custom `plot.Ticker` for X-axis time formatting
- Dual Y-axes via two overlaid `plot.Plot` instances sharing an X-axis
- SVG output via `vg/vgsvg` backend

**Styling map:**

| matplotlib                     | gonum/plot equivalent                  |
| ------------------------------ | -------------------------------------- |
| `fig, ax = plt.subplots()`     | `p := plot.New()`                      |
| `ax.plot(x, y, marker, color)` | `plotter.NewLine(xy)` + `LineStyle`    |
| `ax.bar(x, heights)`           | `plotter.NewBarChart(vals, width)`     |
| `ax.twinx()`                   | Second `plot.Plot` with shared X-axis  |
| `fig.legend()`                 | `p.Legend` configuration               |
| `ax.axvline()`                 | `plotter.NewLine` (vertical segment)   |
| `fig.savefig(path)`            | `p.Save(w, h, "chart.svg")`            |
| `Patch(facecolor, alpha)`      | Custom `plotter.Function` or rectangle |
| `ticker.FixedLocator`          | Custom `plot.Ticker` implementation    |
| Masked arrays (NaN gaps)       | Separate line segments per day         |

**Key implementation detail:** gonum/plot does not have built-in dual-axis
support. The recommended approach is to render two plots to the same SVG
canvas, one for speed (left axis) and one for counts (right axis), sharing
the X dimension. This matches how matplotlib's `twinx()` works internally.

Alternatively, generate the SVG directly using Go's `encoding/xml` or the
`ajstarks/svgo` package (already an indirect dependency via gonum) for full
control over the dual-axis layout. This avoids gonum/plot's single-axis
limitation and gives pixel-precise control over element placement.

### 2. Histogram (Single Period)

**Current (matplotlib):** 3.0 × 2.0 inch figure with:

- Vertical bars: steelblue, α=0.7, black edge
- X-axis: speed bucket labels ("20–25", "70+")
- Y-axis: count

**Go equivalent:**

This is a straightforward bar chart. `gonum/plot` with `plotter.BarChart`
handles this directly. Custom tick labels for range formatting ("20–25")
require a `plot.Ticker` implementation.

### 3. Comparison Histogram

**Current (matplotlib):** 3.0 × 2.0 inch figure with:

- Side-by-side bars (primary vs comparison period)
- Y-axis: percentage (normalised from counts)
- Two colours from the percentile palette

**Go equivalent:**

`gonum/plot` supports grouped bar charts via `plotter.BarChart` with offset
positioning. Percentage normalisation is trivial in Go.

### 4. Map Overlay

**Current (Python):** `SVGMarkerInjector` injects radar-coverage triangles
into site map SVGs, then converts SVG→PDF via `cairosvg`/`inkscape`/`rsvg`.

**Go equivalent:** The SVG manipulation (marker injection) can use Go's
`encoding/xml` to parse and modify SVG DOM. The SVG→PDF conversion shares
the same strategy as chart SVGs (see § SVG-to-PDF Strategy).

---

## SVG-to-PDF Strategy

XeTeX's `\includegraphics` does not natively handle SVG files. Three options:

### ~~Option A: Require `inkscape` on the system~~

Use the `svg` LaTeX package, which calls `inkscape --export-pdf` during
compilation. This adds a ~300 MB dependency — counterproductive.

### Option B: Convert SVG→PDF in Go before `xelatex` ✅

Use `rsvg-convert` (from `librsvg`, ~2 MB) as a lightweight SVG→PDF
converter:

```go
cmd := exec.Command("rsvg-convert", "-f", "pdf", "-o", "chart.pdf", "chart.svg")
```

`rsvg-convert` is:

- Already available on most Linux distributions (`librsvg2-bin`)
- ~2 MB installed footprint (vs ~300 MB for inkscape)
- The same tool already used by the Python `map_utils.py` as a fallback
- Available via `apt install librsvg2-bin` on Raspberry Pi

The `.tex` file then uses `\includegraphics{chart.pdf}` as before.

### ~~Option C: Generate PDF charts directly from Go (bypass SVG)~~

Use `gonum/plot` with `vg/vgpdf` to produce PDF figures directly. This
avoids the SVG→PDF step entirely, but loses the SVG artefact for web reuse
and source archive inclusion. It also makes visual debugging harder — SVGs
can be opened in any browser.

### ~~Option D: Embed SVG inline via LaTeX `\input` with PGF~~

Convert SVG paths to PGF/TikZ commands. Fragile, slow compilation, and
significant implementation effort.

**Recommendation: Option B.** `rsvg-convert` is tiny, battle-tested, and
already a known fallback in the current Python code. The SVG artefact is
preserved for the `.zip` source archive and potential web frontend reuse.

---

## LaTeX Template Design

Replace PyLaTeX's programmatic document construction with Go `text/template`
files embedded via `go:embed`.

### Preamble Template (`templates/preamble.tex`)

```latex
\documentclass[11pt,a4paper]{article}

% Geometry
\usepackage[top=1.8cm, bottom=1.0cm, left=1.0cm, right=1.0cm]{geometry}

% Packages
\usepackage{graphicx}
\usepackage{tabularx}
\usepackage{supertabular}
\usepackage{xcolor}
\usepackage{colortbl}
\usepackage{fancyhdr}
\usepackage{hyperref}
\usepackage{fontspec}
\usepackage{amsmath}
\usepackage{titlesec}
\usepackage{caption}
\usepackage{multicol}
\usepackage{float}

% Fonts
\setmainfont{AtkinsonHyperlegible}[
  Path = <<.FontDir>>/,
  Extension = .ttf,
  UprightFont = *-Regular,
  BoldFont = *-Bold,
  ItalicFont = *-Italic,
  BoldItalicFont = *-BoldItalic
]

% Colours (matching web palette)
\definecolor{vrP50}{HTML}{fbd92f}
\definecolor{vrP85}{HTML}{f7b32b}
\definecolor{vrP98}{HTML}{f25f5c}
\definecolor{vrMax}{HTML}{2d1e2f}
```

### Main Template (`templates/report.tex`)

```latex
<<template "preamble" .>>

\begin{document}
\begin{multicols}{2}

<<template "overview" .>>
<<template "site_info" .>>

\end{multicols}

<<template "chart_section" .>>
<<template "statistics" .>>
<<template "science" .>>

\end{document}
```

### Template Data Structure

```go
type TemplateData struct {
    // Site information
    Location    string
    Surveyor    string
    Contact     string
    SpeedLimit  int
    Description string

    // Survey period
    StartDate   string
    EndDate     string
    Timezone    string
    Units       string

    // Statistics
    P50         string  // formatted
    P85         string
    P98         string
    MaxSpeed    string
    TotalCount  int
    HoursCount  int

    // Chart file paths (relative to .tex directory)
    TimeSeriesChart string  // "timeseries.pdf"
    HistogramChart  string  // "histogram.pdf"
    CompareChart    string  // "comparison.pdf" (optional)
    MapChart        string  // "map.pdf" (optional)

    // Font directory
    FontDir string

    // Table data
    HourlyTable  []HourlyRow
    DailyTable   []DailyRow

    // Radar configuration
    CosineAngle float64
    CosineFactor float64
    ModelVersion string
}
```

### Template Advantages Over PyLaTeX

1. **Readable:** Templates are plain `.tex` files, editable by anyone who
   knows LaTeX. No Python API knowledge needed.
2. **Cacheable:** Templates are `go:embed`-ed at compile time — zero disk I/O
   at runtime.
3. **Testable:** Template rendering produces deterministic `.tex` output that
   can be compared byte-for-byte in tests.
4. **Familiar:** Go's `text/template` is widely understood; LaTeX syntax
   highlighting works in any editor.

---

## Implementation Phases

### Phase 1: Chart Package Foundation (`internal/report/chart/`) — `M`

Build the SVG chart generation library:

1. Define `ChartStyle` struct with colour, font, layout constants
2. Implement `RenderHistogram(data, style) → []byte` (simplest chart first)
3. Implement `RenderTimeSeries(data, style) → []byte` (most complex chart)
4. Implement `RenderComparison(a, b, style) → []byte`
5. Write comprehensive tests comparing output SVG structure
6. Use `gonum/plot` with `vgsvg` backend, or direct SVG generation via
   `encoding/xml` if gonum's dual-axis support proves insufficient

**Acceptance:** SVG output for all three chart types passes visual review
against equivalent matplotlib output.

### Phase 2: LaTeX Template Engine (`internal/report/tex/`) — `S`

Build the template-based `.tex` generation:

1. Create `go:embed`-ed `.tex` templates for each report section
2. Implement `RenderTeX(data) → []byte` using `text/template`
3. Implement `EscapeTeX()` for safe string interpolation
4. Implement table formatting helpers (`FormatHourlyTable`, etc.)
5. Write tests asserting template output matches expected `.tex` fragments

**Acceptance:** `RenderTeX()` produces a `.tex` file that compiles with
`xelatex` to a visually equivalent PDF.

### Phase 3: Report Orchestrator (`internal/report/`) — `S`

Wire charts + templates + compilation together:

1. Implement `Generate(ctx, db, cfg) → (ReportResult, error)`
2. Query database directly (no HTTP round-trip)
3. Render SVG charts → convert to PDF via `rsvg-convert`
4. Render `.tex` from templates → invoke `xelatex`
5. Package `.zip` with `.tex` source + `.svg` charts
6. Write integration test with in-memory SQLite + mock xelatex

**Acceptance:** End-to-end report generation from test database produces
valid PDF.

### Phase 4: API and CLI Integration — `S`

Connect the new report pipeline to existing entry points:

1. Replace `exec.Command("python"...)` in `generateReport()` with direct
   call to `report.Generate()`
2. Add `pdf` subcommand to CLI (aligns with D-09 single binary plan)
3. Maintain backward-compatible JSON response format
4. Update web frontend if API response structure changes

**Acceptance:** `POST /api/generate_report` produces identical results using
Go-native pipeline.

### Phase 5: Python Deprecation and Cleanup — `S`

Remove the Python PDF stack:

1. Mark `tools/pdf-generator/` as deprecated (one release cycle)
2. Remove Python execution path from `server.go`
3. Remove `make install-python` dependency from report generation targets
4. Update `ARCHITECTURE.md`, component READMEs
5. Retain `tools/pdf-generator/` in repository history (do not delete
   immediately — keep for reference during the transition)

**Acceptance:** `make test` passes with no Python dependencies for report
generation. Python venv is only needed for `tools/grid-heatmap/` (if still
in use).

### Phase 6: Map Overlay Migration — `S`

Migrate the SVG marker injection:

1. Port `SVGMarkerInjector` logic to Go using `encoding/xml`
2. Reuse existing `map_svg_data` blob from database
3. Apply same `rsvg-convert` pipeline for map SVG→PDF
4. Test with production site SVG data

**Acceptance:** Map overlays in PDF reports match Python-generated output.

---

## Testing and Validation

### Unit Tests

- **Chart SVG structure:** Parse generated SVG, assert expected elements
  (lines, bars, text labels, colours)
- **Template rendering:** Compare `.tex` output against golden files
- **LaTeX escaping:** Edge cases (ampersands, percent signs, backslashes)
- **Number formatting:** Consistent decimal places, locale-independent
- **Histogram bucketing:** Match Go server's existing computation

### Visual Regression

- Generate PDF from test database with both Python and Go pipelines
- Compare visually (manual review during development)
- Capture "golden" SVG snapshots for automated comparison after stabilisation

### Integration Tests

- In-memory SQLite with known data → `Generate()` → assert PDF exists
- Mock `xelatex` binary for CI (assert `.tex` is well-formed without full
  TeX installation)
- Test `rsvg-convert` fallback (assert graceful error when not installed)

### Backward Compatibility

- JSON API response format for `POST /api/generate_report` unchanged
- Report filenames follow existing pattern:
  `{endDate}_velocity.report_{location}_report.pdf`
- `.zip` archive structure preserved (`.tex` + chart assets)

---

## Risks and Mitigations

| Risk                                                                            | Impact           | Mitigation                                                                                                                         |
| ------------------------------------------------------------------------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| gonum/plot dual-axis limitation                                                 | Chart quality    | Fall back to direct SVG generation via `encoding/xml` or `ajstarks/svgo`; prototype in Phase 1 before committing                   |
| SVG-to-PDF fidelity via `rsvg-convert`                                          | Text rendering   | Use `rsvg-convert --dpi 150` for consistent sizing; test with Atkinson Hyperlegible font embedded in SVG                           |
| Chart visual parity with matplotlib                                             | User expectation | Side-by-side comparison during development; accept minor styling differences if data accuracy is preserved                         |
| `rsvg-convert` not available on target                                          | Build failure    | Detect at startup, log warning; fall back to gonum/plot `vgpdf` direct PDF output (skip SVG artefact)                              |
| LaTeX template complexity                                                       | Maintenance      | Keep templates minimal; complex logic stays in Go helpers, templates only interpolate values                                       |
| Go `text/template` default delimiters `{{`/`}}` clash with LaTeX brace grouping | Template errors  | Use custom Go template delimiters `<<` and `>>` via `template.Delims("<<", ">>")` to avoid any ambiguity with LaTeX `{`/`}` syntax |

---

## Relationship to Existing Decisions

### D-08 (Precompiled LaTeX)

This plan is **complementary**. D-08 reduces the TeX installation from
~800 MB to ~30–60 MB. This plan eliminates the Python stack (~450 MB).
Together they reduce the deployment footprint from ~1.25 GB to ~30–60 MB.

The precompiled `.fmt` file (D-08) still applies — the Go-generated `.tex`
compiles against the same minimal TeX tree with the same precompiled format.

### D-09 (Single Binary)

This plan **enables** D-09's `velocity-report pdf` subcommand without
requiring a bundled Python interpreter. The report generation logic lives
in `internal/report/`, callable from both the API handler and the CLI.

### D-10 (RPi Image)

This plan **simplifies** the Raspberry Pi image by removing Python entirely
from the report-generation path. The image only needs the Go binary +
vendored TeX tree + `rsvg-convert`.

---

## Open Questions

1. **gonum/plot vs direct SVG:** Should Phase 1 prototype both approaches
   before committing? Direct SVG gives full control but requires more code.
   gonum/plot provides abstractions but may fight us on dual-axis layout.

2. **Font embedding in SVG:** Should chart SVGs embed the Atkinson
   Hyperlegible font (larger files, self-contained) or reference it by name
   (smaller files, requires font installed on system for `rsvg-convert`)?

3. **Chart dimensions in SVG:** Matplotlib uses inches as the native unit.
   SVG uses pixels (with configurable viewBox). Should we define chart sizes
   in mm (LaTeX-native) or inches (matplotlib-compatible)?

4. **`rsvg-convert` on macOS:** The development workflow needs `rsvg-convert`
   available. It is installable via `brew install librsvg`. Should we add
   this to the developer setup guide?

5. **Web frontend reuse:** Should the Go chart package also serve SVG charts
   via the API for the web frontend, replacing ECharts for static report
   views? This would align with D-11 (ECharts → LayerChart migration) but
   expands scope.

6. **Comparison chart in Go API:** The Python `build_comparison` method
   normalises counts to percentages. Does the Go API already return
   normalised histogram data, or must the report package compute percentages?

7. **`grid-heatmap` tool:** `tools/grid-heatmap/` also uses matplotlib. Is
   it in scope for this migration, or does it remain as a standalone Python
   tool?
