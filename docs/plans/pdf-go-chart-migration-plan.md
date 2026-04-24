# PDF generation migration to Go

- **Status:** Phases 1–5 are complete. The Python exec path is fully removed; `tools/pdf-generator/` is retained for reference and marked deprecated (removal in v0.6). Phase 8: 8.1 golden tests and `tex-compare` implemented; 8.9 `FirmwareVersion` implemented. Remaining open: 8.11 single-survey drift (deferred — no Python single-survey .tex available). Phases 6 (map overlay) and 7 (grid-heatmap) are deferred to later branches.
- **Layers:** Cross-cutting (reporting infrastructure)
- **Related:**
- **Canonical:** [pdf-reporting.md](../platform/operations/pdf-reporting.md)

- [Precompiled LaTeX plan](pdf-latex-precompiled-format-plan.md) (D-08)
- [Distribution packaging plan](deploy-distribution-packaging-plan.md) (D-09)
- [RPi imager plan](deploy-rpi-imager-fork-plan.md) (D-10)
- [Platform simplification plan](platform-simplification-and-deprecation-plan.md)

**Goal:** Replace the Python PDF-generation stack (matplotlib, PyLaTeX, numpy,
pandas, seaborn: 45 transitive packages) with native Go SVG charting and Go
`text/template`-based LaTeX assembly, retaining XeTeX for typesetting and
producing SVG charts equivalent to the current matplotlib output. The same Go
chart package also serves SVG to the web frontend, replacing ECharts for
report views and aligning the charting engine across both surfaces. The
`grid-heatmap` tool is also migrated in scope. The Python `pdf-generator`
subcommand is deprecated; report generation becomes the `pdf` subcommand of
the unified `velocity-report` binary.

---

> **Problem summary and key changes overview:** see [pdf-reporting.md](../platform/operations/pdf-reporting.md).

---

## Implementation status snapshot

- **Phases 1–3:** Landed in PR #455 and remain green.
- **Phase 4a:** Implemented in `internal/api/server_reports_generate.go`. The handler now branches on `VELOCITY_PDF_BACKEND=go`, builds `report.Config`, calls `report.Generate`, validates both PDF and ZIP paths, preserves `db.SiteReport` creation, and returns the same JSON response shape as the Python path. Current tests cover Go-path selection, Python fallback, config mapping, and relative-path hardening.
- **Phase 4b:** Implemented in `internal/api/server_charts.go` and registered in `internal/api/server.go`. Time-series, histogram, and comparison SVG handlers are present and have request/response tests.
- **Live verification on 2026-04-21:** Against the running dev server on `http://127.0.0.1:8080`, `GET /api/charts/timeseries`, `GET /api/charts/histogram`, and `GET /api/charts/comparison` each returned `200` with `Content-Type: image/svg+xml` and an `<svg` root for site `1` over `2025-05-02` to `2025-05-30`. `POST /api/generate_report` for the same site/range returned `200` with the expected `report_id`, `pdf_path`, and `zip_path` response fields.
- **Phase 4c:** A `velocity-report pdf` subcommand already exists in `cmd/radar/pdf.go`, and `cmd/radar/pdf_test.go` now covers flag validation, version output, config parsing, and output-dir override behaviour. This plan still describes a different file layout (`cmd/velocity-report/pdf/main.go`) and an unimplemented `build-pdf-tool` target, so the implementation exists but the plan and packaging details have drifted.
- **Frontend/report parity:** The dashboard and report generator now both render preview charts through `InlineSvgChart` backed by `/api/charts/*`, and `web/src/lib/reportRequests.ts` centralises report payload construction for both surfaces. `web/src/lib/reportRequests.test.ts` covers fresh-settings parity, stale-settings fallback, and comparison payload fields.

## Current architecture

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

### Python modules to replace

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

## Proposed architecture

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

### Package layout

Current code note: the shipped CLI entrypoint lives in `cmd/radar/pdf.go` and the SVG HTTP handlers live in `internal/api/server_charts.go`. The tree below remains the intended D-09 end-state, not the exact current file layout.

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
│   ├── heatmap.go      # RenderGridHeatmap(data, style) → []byte (SVG)
│   ├── heatmap_test.go
│   ├── palette.go      # Percentile colour constants, matching web palette
│   ├── palette_test.go
│   └── svg.go          # Low-level SVG element helpers (rect, polyline, text…)
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

cmd/velocity-report/
└── pdf/                # `velocity-report pdf` subcommand
    └── main.go
    └── grid_heatmap/   # `velocity-report grid-heatmap` subcommand

internal/api/
└── chart_handler.go    # HTTP SVG endpoints for web frontend consumption
                        # GET /api/charts/timeseries, /histogram, /comparison
```

---

## Chart-by-Chart migration

### 1. Time-Series chart (dual-axis percentile + count)

**Current (matplotlib):** 24.0 × 8.0 inch figure with:

- Left Y-axis: P50/P85/P98/Max speed lines with markers
- Right Y-axis: Count bars (translucent)
- Orange background bars for low-sample periods (< 50 count)
- Broken lines at day boundaries
- Custom X-axis: `HH:MM` with `Mon DD` at day starts
- Legend below chart

**Go approach: direct SVG generation** ✅

gonum/plot does not have built-in dual-axis support and would require fighting
its layout engine for this chart's dual-axis, day-boundary-gap, and
low-sample-shading requirements. Direct SVG generation via Go's
`encoding/xml` is chosen instead. This gives pixel-precise control and
produces an SVG that is both the PDF source and the web-embeddable artefact.

**Styling map:**

| matplotlib element             | Direct SVG equivalent                            |
| ------------------------------ | ------------------------------------------------ |
| `fig, ax = plt.subplots()`     | `<svg viewBox="...">` with computed layout rects |
| `ax.plot(x, y, marker, color)` | `<polyline>` + `<circle>` per data point         |
| `ax.bar(x, heights)`           | `<rect>` per bar, right-axis scale               |
| `ax.twinx()`                   | Two independent Y-scale functions, one viewBox   |
| `fig.legend()`                 | `<text>` + `<line>` legend group below chart     |
| `ax.axvline()`                 | `<line>` at computed X position                  |
| `Patch(facecolor, alpha)`      | `<rect fill-opacity="0.15">`                     |
| Masked arrays (NaN gaps)       | Multiple `<polyline>` segments, one per day      |
| `ticker.FixedLocator`          | Computed tick positions from time range          |

The chart package exposes a `ChartStyle` struct with explicit control knobs
for dimensions, colours, font size, bar opacity, marker size, and axis
padding. These same knobs are used by the web frontend when it renders the
same SVG endpoint, ensuring visual consistency across PDF and browser
surfaces. Dimension knobs are in millimetres (LaTeX-native, see Q3 below).

### 2. Histogram (single period)

**Current (matplotlib):** 3.0 × 2.0 inch figure with:

- Vertical bars: steelblue, α=0.7, black edge
- X-axis: speed bucket labels ("20–25", "70+")
- Y-axis: count

**Go approach: direct SVG generation** ✅

A straightforward bar chart rendered as `<rect>` elements with `<text>` tick
labels. Direct SVG is used for consistency with the time-series chart rather
than mixing gonum and hand-rolled SVG in the same package. Bucket label
formatting ("20–25", "70+") is a simple Go string function.

### 3. Comparison histogram

**Current (matplotlib):** 3.0 × 2.0 inch figure with:

- Side-by-side bars (primary vs comparison period)
- Y-axis: percentage (normalised from counts)
- Two colours from the percentile palette

**Go approach: direct SVG generation** ✅

Side-by-side grouped bars as offset `<rect>` pairs. Percentage normalisation
(counts → fractions) is computed in Go before rendering. Two colours come
from `palette.go` matching the web palette.

### 4. Map overlay

**Current (Python):** `SVGMarkerInjector` injects radar-coverage triangles
into site map SVGs, then converts SVG→PDF via `cairosvg`/`inkscape`/`rsvg`.

**Go equivalent:** The SVG manipulation (marker injection) can use Go's
`encoding/xml` to parse and modify SVG DOM. The SVG→PDF conversion shares
the same strategy as chart SVGs (see § SVG-to-PDF Strategy).

---

## SVG-to-PDF strategy

XeTeX's `\includegraphics` does not natively handle SVG files. Three options:

### ~~Option a: require `inkscape` on the system~~

Use the `svg` LaTeX package, which calls `inkscape --export-pdf` during
compilation. This adds a ~300 MB dependency: counterproductive.

### Option b: convert SVG→PDF in Go before `xelatex` ✅

Use `rsvg-convert` (from `librsvg`, ~2 MB) as a lightweight SVG→PDF
converter:

A single `exec.Command("rsvg-convert", "-f", "pdf", "-o", "chart.pdf", "chart.svg")` call converts each SVG chart to PDF.

`rsvg-convert` is:

- Already available on most Linux distributions (`librsvg2-bin`)
- ~2 MB installed footprint (vs ~300 MB for inkscape)
- The same tool already used by the Python `map_utils.py` as a fallback
- Available via `apt install librsvg2-bin` on Raspberry Pi

The `.tex` file then uses `\includegraphics{chart.pdf}` as before.

### ~~Option c: generate PDF charts directly from Go (bypass SVG)~~

Use `gonum/plot` with `vg/vgpdf` to produce PDF figures directly. This
avoids the SVG→PDF step entirely, but loses the SVG artefact for web reuse
and source archive inclusion. It also makes visual debugging harder: SVGs
can be opened in any browser.

### ~~Option d: embed SVG inline via LaTeX `\input` with PGF~~

Convert SVG paths to PGF/TikZ commands. Fragile, slow compilation, and
significant implementation effort.

**Recommendation: Option B.** `rsvg-convert` is tiny, battle-tested, and
already a known fallback in the current Python code. The SVG artefact is
preserved for the `.zip` source archive and potential web frontend reuse.

### Font embedding in SVG

SVG charts **embed the Atkinson Hyperlegible font as a base64 data URI**
(`<style>@font-face { src: url("data:font/...") }</style>`). This makes each
SVG self-contained for `rsvg-convert` and for direct web embedding without
requiring the font to be installed separately. The file-size cost is modest
(~100 KB per SVG for the subset used by chart labels), which is negligible
compared to the PDF output size. When the web frontend fetches chart SVGs
directly, the font is already embedded and renders correctly without network
requests for the font file.

---

## LaTeX template design

Replace PyLaTeX's programmatic document construction with Go `text/template`
files embedded via `go:embed`.

### Preamble template (`templates/preamble.tex`)

The preamble uses `\documentclass[11pt,a4paper]{article}` with packages: `geometry` (1.8 cm top, 1.0 cm bottom/left/right), `graphicx`, `tabularx`, `supertabular`, `xcolor`, `colortbl`, `fancyhdr`, `hyperref`, `fontspec`, `amsmath`, `titlesec`, `caption`, `multicol`, and `float`. Fonts are set via `\setmainfont{AtkinsonHyperlegible}` loaded from the `<<.FontDir>>` template variable with Regular/Bold/Italic/BoldItalic TTF variants. Colours match the web palette:

| Colour | Name    | Hex     |
| ------ | ------- | ------- |
| P50    | `vrP50` | #fbd92f |
| P85    | `vrP85` | #f7b32b |
| P98    | `vrP98` | #f25f5c |
| Max    | `vrMax` | #2d1e2f |

### Main template (`templates/report.tex`)

The main template includes the preamble, opens a two-column layout for the overview and site info sections, then switches to single-column for chart section, statistics, and science sections. Template composition uses `<<template "name" .>>` syntax (Go `text/template` with `<<>>` delimiters to avoid clashing with LaTeX braces).

### Template data structure

| Field             | Type        | Purpose                           |
| ----------------- | ----------- | --------------------------------- |
| `Location`        | string      | Site location name                |
| `Surveyor`        | string      | Surveyor name                     |
| `Contact`         | string      | Contact details                   |
| `SpeedLimit`      | int         | Posted speed limit                |
| `Description`     | string      | Site description                  |
| `StartDate`       | string      | Survey start (formatted)          |
| `EndDate`         | string      | Survey end (formatted)            |
| `Timezone`        | string      | IANA timezone                     |
| `Units`           | string      | Speed unit label                  |
| `P50`             | string      | 50th percentile (formatted)       |
| `P85`             | string      | 85th percentile (formatted)       |
| `P98`             | string      | 98th percentile (formatted)       |
| `MaxSpeed`        | string      | Maximum speed (formatted)         |
| `TotalCount`      | int         | Total vehicle count               |
| `HoursCount`      | int         | Total survey hours                |
| `TimeSeriesChart` | string      | Path to timeseries PDF            |
| `HistogramChart`  | string      | Path to histogram PDF             |
| `CompareChart`    | string      | Path to comparison PDF (optional) |
| `MapChart`        | string      | Path to map PDF (optional)        |
| `FontDir`         | string      | Font directory path               |
| `HourlyTable`     | []HourlyRow | Hourly breakdown rows             |
| `DailyTable`      | []DailyRow  | Daily breakdown rows              |
| `CosineAngle`     | float64     | Radar cosine angle                |
| `CosineFactor`    | float64     | Cosine correction factor          |
| `ModelVersion`    | string      | Radar model version               |

### Template advantages over PyLaTeX

1. **Readable:** Templates are plain `.tex` files, editable by anyone who
   knows LaTeX. No Python API knowledge needed.
2. **Cacheable:** Templates are `go:embed`-ed at compile time; zero disk I/O
   at runtime.
3. **Testable:** Template rendering produces deterministic `.tex` output that
   can be compared byte-for-byte in tests.
4. **Familiar:** Go's `text/template` is widely understood; LaTeX syntax
   highlighting works in any editor.

---

## Implementation Checklist

Each item is a discrete, testable unit of work. Items within a phase are
ordered; phases build on each other. Phases 1–3 landed in PR #455.
Phase 4 is broken into three independent sub-phases to derisk integration.

### Phase 1 — Chart Package (`internal/report/chart/`) `M` ✅ PR #455

#### 1.0 Pre-work: constants + Q6 resolution

- [x] Read `tools/pdf-generator/pdf_generator/core/config_manager.py` and record
      exact values for: all colour hex codes (p50/p85/p98/max/count_bar/low_sample),
      figure sizes in inches, and all layout constants (thresholds, fractions, widths).
- [x] Confirm histogram API shape: `server_radar.go` lines 230–241 confirm raw
      `map[string]int64` (bucket-start in display units → count). `NormaliseHistogram`
      helper needed in chart package.

#### 1.1 Font assets

- [x] Create `internal/report/chart/assets/` directory.
- [x] Copy `AtkinsonHyperlegible-Regular.ttf` from
      `tools/pdf-generator/pdf_generator/core/fonts/` into `assets/`.
- [x] Add `//go:embed assets/AtkinsonHyperlegible-Regular.ttf` in `svg.go`.
- [x] `AtkinsonRegularBase64() string` — returns base64-encoded TTF string.

#### 1.2 `palette.go`

- [x] Constants: `ColourP50`, `ColourP85`, `ColourP98`, `ColourMax`,
      `ColourCountBar`, `ColourLowSample` (hex strings, matching web palette and Python config).
- [x] Test: assert all constants are non-empty and start with `#`.

#### 1.3 `config.go` — ChartStyle

- [x] `type ChartStyle struct` with fields (all with Go zero values safe):
      `WidthMM`, `HeightMM float64`;
      `ColourP50`…`ColourLowSample string`;
      `CountMissingThreshold`, `LowSampleThreshold int`;
      `CountAxisScale`, `BarWidthFraction`, `BarWidthBGFraction float64`;
      `LineWidthPx`, `MarkerRadiusPx float64`;
      `AxisLabelFontPx`, `AxisTickFontPx`, `LegendFontPx float64`.
- [x] `DefaultTimeSeriesStyle() ChartStyle` — 609.6 × 203.2 mm, all Python defaults.
- [x] `DefaultHistogramStyle() ChartStyle` — 76.2 × 50.8 mm, histogram font sizes.

#### 1.4 `svg.go` — SVGCanvas

- [x] `type SVGCanvas` holding dims, `strings.Builder`, and pixel scale factor
      (mm → px: multiply by `96/25.4 ≈ 3.7795`).
- [x] `NewCanvas(widthMM, heightMM float64) *SVGCanvas`.
- [x] Methods: `Rect`, `Polyline`, `Circle`, `Line`, `Text`, `BeginGroup`/`EndGroup`.
- [x] `EmbedFont(family, base64Data string)` — emits `<defs><style>@font-face{…}</style></defs>`.
- [x] `Bytes() []byte` — close root `</svg>` and return.
- [x] Test: `TestNewCanvas` — parse output with `encoding/xml`, assert root is `<svg>`,
      `viewBox` contains correct pixel dimensions.

#### 1.5 `histogram.go`

- [x] `NormaliseHistogram(buckets map[float64]int64) (keys []float64, counts []int64, total int64)` — sorted keys.
- [x] `BucketLabel(lo, hi, maxBucket float64) string` — `"20-25"`, `"70+"`.
- [x] `type HistogramData struct` — `Buckets map[float64]int64`, `Units string`,
      `BucketSz float64`, `MaxBucket float64`, `Cutoff float64`.
- [x] `RenderHistogram(data HistogramData, style ChartStyle) ([]byte, error)` —
      single bar chart; bars: steelblue `#4682b4`, alpha 0.7, edge black 0.5 px.
      Rotate labels 45° when > 20 buckets. "No data" text if empty.
- [x] `RenderComparison(primary, compare HistogramData, primaryLabel, compareLabel string, style ChartStyle) ([]byte, error)` —
      grouped bars, `bar_width=0.4`, alpha 0.75, primary → `ColourP50`, compare → `ColourP98`.
      Percentages computed per-total separately. Y-label `"Percentage (%)"`.
- [x] Tests:
  - `TestNormaliseHistogram` — empty, single, multi bucket, total correct.
  - `TestBucketLabel` — `"20-25"`, `"70+"`, capped range.
  - `TestRenderHistogram_Structure` — parse SVG, `<rect>` count = bucket count.
  - `TestRenderHistogram_Empty` — contains "No data".
  - `TestRenderComparison_Structure` — `<rect>` count = 2 × bucket count.

#### 1.6 `timeseries.go`

- [x] `type TimeSeriesPoint struct` — `StartTime time.Time`, `P50/P85/P98/MaxSpeed float64` (NaN = missing), `Count int`.
- [x] `type TimeSeriesData struct` — `Points []TimeSeriesPoint`, `Units string`, `Title string`.
- [x] `DayBoundaries(pts []TimeSeriesPoint) []int` — always includes 0; adds idx where `.date()` changes.
- [x] `ApplyCountMask(pts []TimeSeriesPoint, threshold int) []TimeSeriesPoint` — NaN-ify speeds where Count < threshold.
- [x] `XTicks(pts []TimeSeriesPoint, boundaries []int) []XTick` — day-start + every 3rd interior; label `"Jan 02\n15:04"` / `"15:04"`.
- [x] `RenderTimeSeries(data TimeSeriesData, style ChartStyle) ([]byte, error)`:
  - Integer x-domain; two independent Y-scale functions (speed left, count right).
  - Count bars: alpha 0.25, `ColourCountBar`.
  - Low-sample bg bars: full-height, alpha 0.25, `ColourLowSample`, where `Count < LowSampleThreshold`.
  - Percentile polylines: p50 `▲`, p85 `▪`, p98 `●`, max `✕` + dashed; segment per day, no cross-day connection.
  - Day boundary lines: `stroke="gray"`, `stroke-dasharray="4 2"`, `stroke-width="0.5"`, `opacity="0.3"`.
  - Legend below chart; embed font via `EmbedFont`.
  - "No data" if empty.
- [x] Tests:
  - `TestDayBoundaries` — single day (boundary=[0]), two days.
  - `TestApplyCountMask` — below-threshold points have NaN.
  - `TestRenderTimeSeries_Structure` — parse SVG, `<polyline>` present.
  - `TestRenderTimeSeries_DayLines` — `<line>` elements with `stroke="gray"` present.
  - `TestRenderTimeSeries_Empty` — "No data" text.

**Phase 1 acceptance:** `go test ./internal/report/chart/... -v` all green. ✅
SVG output for all three chart types reviewed visually against matplotlib.

---

### Phase 2 — LaTeX Template Engine (`internal/report/tex/`) `S` ✅ PR #455

#### 2.1 `helpers.go`

- [x] `EscapeTeX(s string) string` — escape `& % $ # _ { } ~ ^ \` for LaTeX.
- [x] `FormatNumber(v float64) string` — `"--"` for NaN/Inf, else `"%.2f"`.
- [x] `FormatPercent(v float64) string` — `"--"` for NaN/Inf, else `"%.1f%%"`.
- [x] `FormatTime(t time.Time, loc *time.Location) string` — `"1/2 15:04"` (no zero-pad, matching Python `"%-m/%-d %H:%M"`).
- [x] `BuildHistogramTableTeX(buckets map[float64]int64, bucketSz, cutoff, maxBucket float64, units string) string` —
      produces a LaTeX `tabular` string with Bucket/Count/Percent columns;
      `<N` below-cutoff row (only if data exists below cutoff);
      `N+` final row (only if data exists ≥ maxBucket);
      percentage formatted to 1 decimal place.
- [x] Tests:
  - `TestEscapeTeX` table: `&` → `\&`, `%` → `\%`, `\` → `\textbackslash{}`, `~` → `\textasciitilde{}`.
  - `TestFormatNumber`: NaN → `"--"`, +Inf → `"--"`, `3.14159` → `"3.14"`.
  - `TestFormatTime`: known UTC time + Pacific timezone → expected display string.
  - `TestBuildHistogramTableTeX`: known buckets → contains `\hline`, `70+`, correct row count.

#### 2.2 `render.go` — TemplateData + RenderTeX

- [x] `type TemplateData struct` (full field list — see plan implementation notes).
- [x] `type StatRow struct { StartTime string; Count int; P50,P85,P98,MaxSpeed string }`.
- [x] `BuildStatRows(pts []TimeSeriesPoint, loc *time.Location) []StatRow`.
- [x] `RenderTeX(data TemplateData) ([]byte, error)` — parse embedded templates with
      `template.Delims("<<", ">>")`, execute, return `.tex` bytes.

#### 2.3 Templates in `tex/templates/` (all `//go:embed templates/*.tex`)

All templates use `<<` / `>>` delimiters throughout.

- [x] `preamble.tex` — `\documentclass[11pt,a4paper]{article}`, geometry, all packages
      (graphicx, tabularx, supertabular, xcolor, colortbl, fancyhdr, hyperref, fontspec,
      amsmath, titlesec, caption, multicol, float, array), colour defines
      (`vrP50=#fbd92f`, `vrP85=#f7b32b`, `vrP98=#f25f5c`, `vrMax=#2d1e2f`),
      `\setmainfont{AtkinsonHyperlegible}[Path=<<.FontDir>>/,…]`,
      fancyhdr header/footer (left: velocity.report link; right: location italic;
      footer: date range / page N).
- [x] `report.tex` — top-level driver: `\input{preamble}`, `\begin{document}`,
      `\twocolumn[…heading block…]`, `\input` each section template, `\end{document}`.
- [x] `overview.tex` — `\section*{Velocity Overview}`, itemize (location/period/count/speed limit),
      key metrics param table or comparison summary table.
- [x] `site_info.tex` — `\subsection*{Site Information}`, description + speed limit note.
- [x] `science.tex` — "Citizen Radar" subsection, KE formula block, aggregation/percentiles text.
- [x] `survey_parameters.tex` — hardware config table + survey params table (start/end/tz/
      group/units/min speed/cosine angle+factor).
- [x] `statistics.tex` — histogram table (conditional on `<<if .HistogramTableTeX>>`),
      `\subsection*{Detailed Data}`, supertabular stats table in two-column layout.
- [x] `chart_section.tex` — `\onecolumn`, time-series figure (`\includegraphics`),
      comparison figure (conditional), map figure (conditional).

#### 2.4 Tests

- [x] `TestRenderTeX_Valid` — render with minimal `TemplateData`, assert `\begin{document}` and `\end{document}` present.
- [x] `TestRenderTeX_EscapedStrings` — location `"Smith & Jones"` → `"Smith \& Jones"` in output.
- [x] `TestRenderTeX_ConditionalHistogram` — when `HistogramTableTeX == ""`, histogram section absent.
- [x] `TestRenderTeX_ConditionalComparison` — when `CompareStartDate == ""`, comparison table absent.
- [x] Golden file: `testdata/golden_report.tex` — generate with fixture `TemplateData`, compare
      (use `-update` flag to regenerate).

**Phase 2 acceptance:** `go test ./internal/report/tex/... -v` all green. ✅
`RenderTeX()` output compiles with `xelatex` on a machine with TeX installed.

---

### Phase 3 — Report Orchestrator (`internal/report/`) `S` ✅ PR #455

#### 3.1 `db.go` — DB interface

- [x] `type DB interface` with only the methods the report package needs:
      `RadarObjectRollupRange(…) (*db.RadarStatsResult, error)` and
      `GetSite(ctx, id int) (*db.Site, error)`.
      (Allows mocking in tests without the full `db.DB`.)

#### 3.2 `config.go`

- [x] `type Config struct` — mirrors `api.ReportRequest` plus site fields already
      resolved by the API handler (Location, Surveyor, Contact, SpeedLimit,
      SiteDescription, CosineAngle). Self-contained; no import of `internal/api`.
- [x] `type Result struct { PDFPath, ZIPPath, RunID string }`.

#### 3.3 `archive.go`

- [x] `BuildZip(files map[string][]byte) ([]byte, error)` — `archive/zip` stdlib,
      one entry per map key (key = zip-internal path).
- [x] Test: `TestBuildZip` — build zip with two files, re-read with `archive/zip`, assert both present.

#### 3.4 `report.go` — Generate()

- [x] `Generate(ctx context.Context, db DB, cfg Config) (Result, error)`:

  **A — DB queries (direct, no HTTP)**
  - [x] Parse dates using `cfg.Timezone` → Unix timestamps.
  - [x] Call `db.RadarObjectRollupRange` for primary period.
  - [x] If `cfg.CompareStart != ""` call again for compare period.
  - [x] Call with `histBucketSize > 0` for histogram (same or separate call depending on `cfg.Histogram`).
  - [x] Convert `RadarStatsResult.Rows` → `[]chart.TimeSeriesPoint`.
  - [x] Extract summary stats (p50/p85/p98/max/count) from overall result.

  **B — SVG chart rendering**
  - [x] `chart.RenderHistogram` → `histogram.svg`.
  - [x] `chart.RenderTimeSeries` → `timeseries.svg`.
  - [x] If comparison: `chart.RenderComparison` → `comparison.svg`.
  - [x] Write all SVGs to `os.MkdirTemp`.

  **C — SVG → PDF (`rsvg-convert`)**
  - [x] `convertSVGToPDF(ctx, svgPath, pdfPath string) error` — `exec.Command("rsvg-convert", "-f", "pdf", "--dpi-x", "150", "--dpi-y", "150", "-o", pdfPath, svgPath)`.
  - [x] `checkRsvgConvert() error` — `exec.LookPath`; on failure returns error with
        `"apt install librsvg2-bin"` / `"brew install librsvg"` hint.
  - [x] On missing binary: return descriptive error (do NOT silently skip SVG artefact).

  **D — LaTeX rendering**
  - [x] Resolve `FontDir` (absolute path to bundled font assets at `internal/report/chart/assets/`).
  - [x] Build `tex.TemplateData` from query results, chart PDF paths, cfg fields.
  - [x] `tex.RenderTeX(data)` → write `report.tex` to temp dir.

  **E — xelatex compilation**
  - [x] `runXeLatex(ctx context.Context, texPath string, envOverrides map[string]string) error`:
    - Detect `VELOCITY_TEX_ROOT` env var; if set, use vendored compiler path and build env vars
      (matching `tex_environment.py` logic for `TEXMFHOME`, `TEXMFDIST`, `TEXMFVAR`,
      `TEXMFCNF`, `TEXINPUTS`, `PATH`, optionally `TEXFORMATS`).
    - Run xelatex twice (two-pass for cross-refs and fancyhdr).
    - On failure: read `.log`, detect fatal signatures (missing font, nullfont),
      return error with log excerpt.
  - [x] `checkXeLatex(texRoot string) error` — verify compiler binary exists.

  **F — Package and return**
  - [x] `BuildZip` with `.tex` + all `.svg` files → `sources.zip`.
  - [x] Move PDF + ZIP to `output/{runID}/` mirroring current Python output dir layout.
  - [x] Return `Result{PDFPath, ZIPPath, RunID}`.

#### 3.5 Integration test (`report_test.go`)

- [x] `TestGenerate_EndToEnd`:
  - In-memory SQLite with fixture radar stats rows (use existing `testutil` pattern from `internal/`).
  - Mock xelatex: shell script writing a 1-byte `output.pdf` and exiting 0.
  - Mock rsvg-convert: shell script copying SVG to PDF and exiting 0.
  - Call `Generate(ctx, db, cfg)`.
  - Assert: `Result.PDFPath` exists; ZIP contains `.tex`, `timeseries.svg`, `histogram.svg`.
- [x] `TestConvertSVG_MissingBinary` — binary not on PATH → error contains `"librsvg"`.
- [x] `TestRunXeLatex_LogExcerpt` — xelatex exits non-zero → error contains `.log` excerpt.

**Phase 3 acceptance:** `go test ./internal/report/... -v` all green (mocked externals). ✅
Manual end-to-end: call `Generate()` with a real DB and real tools; open resulting PDF.

---

### Phase 4a — Feature-flag Go backend in HTTP handler `S` ✅

> **Risk level: low-medium.** Python remains default. Go path enabled by env var.

- [x] In `internal/api/server_reports_generate.go`: check `os.Getenv("VELOCITY_PDF_BACKEND") == "go"`.
- [x] When flag set: build `report.Config` from already-resolved `ReportRequest` + site fields.
- [x] Call `report.Generate(ctx, s.db, cfg)`.
- [x] Map `report.Result` → same JSON response shape and filename convention as Python path.
- [x] Keep all existing security checks (`security.ValidatePathWithinDirectory`, etc.).
- [x] Keep `db.SiteReport` record creation unchanged.
- [x] Keep `outputIndicatesReportFailure` on the Python path and direct `report.Generate` error propagation as the Go-path equivalent.
- [x] Tests currently in place: `TestGenerateReport_GoBackend_RequiresTools`,
      `TestGenerateReport_PythonPath_WhenFlagUnset`, `TestGenerateReport_GoBackend_ConfigMapping`,
      `TestRelativeReportPaths_Valid`, and `TestRelativeReportPaths_RejectEscape`.

**Phase 4a acceptance:** `POST /api/generate_report` now produces the expected JSON response shape on the live dev API, and the code path for explicit Go-backend selection is covered by tests. Python fallback remains covered when the flag is absent. ✅

---

### Phase 4b — `/api/charts/*` SVG endpoints `S` ✅

> **Risk level: very low.** Additive new endpoints; zero changes to existing handlers.

New file: `internal/api/server_charts.go`.

- [x] `GET /api/charts/timeseries?site_id=N&start=YYYY-MM-DD&end=YYYY-MM-DD&tz=...&units=mph&group=1h`
      → query DB → `chart.RenderTimeSeries` → `Content-Type: image/svg+xml`, `Cache-Control: max-age=300`.
- [x] `GET /api/charts/histogram?site_id=N&start=...&end=...&bucket_size=5&max=70`
      → query DB → `chart.RenderHistogram` → SVG.
- [x] `GET /api/charts/comparison?site_id=N&start=...&end=...&compare_start=...&compare_end=...`
      → two DB queries → `chart.RenderComparison` → SVG.
- [x] Register routes in `server.go`.
- [x] Tests: `TestChartEndpoints_TimeSeries`, `TestChartEndpoints_Histogram`, and
      `TestChartEndpoints_Comparison` assert `Content-Type: image/svg+xml` and `<svg` in body;
      additional request-validation tests cover method, group, unit, and comparison-param failures.

**Phase 4b acceptance:** Live `curl` requests to `/api/charts/timeseries`, `/api/charts/histogram`, and `/api/charts/comparison` returned valid SVG with `<svg` roots on the dev API on 2026-04-21. ✅

---

### Phase 4c — `velocity-report pdf` CLI subcommand `S`

> **Risk level: very low.** New binary entrypoint; zero changes to HTTP server.

Current code status: implemented as `cmd/radar/pdf.go`; this plan's original target path and `build-pdf-tool` packaging step have not been reconciled.

- [x] `velocity-report pdf --config report.json [--output ./out] [--db path/to/db.sqlite]`
- [x] Parse config JSON into `report.Config`.
- [x] Open DB directly (no HTTP server needed).
- [x] Call `report.Generate(ctx, db, cfg)`.
- [x] Print PDF path on success; exit `1` with error on failure.
- [x] Reads `VELOCITY_TEX_ROOT` via the same underlying `report.Generate` environment handling.
- [x] Plan aligned: CLI lives in `cmd/radar/pdf.go` wired from `cmd/radar/radar.go`; `cmd/velocity-report/` was never created and is not needed. No separate `build-pdf-tool` target is required.

**Phase 4c acceptance:** CLI exists, is unit-tested, and plan/code drift is resolved. ✅

---

### Phase 5 — Python Deprecation and Cleanup `S` ✅

- [x] Remove Python exec path from `server_reports_generate.go`. The handler calls `generateReportGo` directly; all Python subprocess code deleted (756 → 348 lines; functions `runPythonPDFGenerator`, `buildPythonReportConfig`, `writePythonConfigFile`, `resolvePythonBinary`, `outputIndicatesReportFailure`, `appendPythonComparisonTeX`, and zip helpers removed).
- [x] Mark `tools/pdf-generator/` deprecated in README (deprecation banner + title change).
- [x] Remove `python3`, `python3-pip`, `python3-venv` from `image/stage-velocity/00-install-packages/00-packages`. Retain `python3-serial` (RS-232 HAT, unrelated to PDF).
- [x] Gut `image/stage-velocity/02-velocity-python/00-run.sh` to only create the report output directory.
- [x] Remove PDF generator copy block from `image/scripts/build-image.sh`.
- [x] Remove `PDF_GENERATOR_DIR` / `PDF_GENERATOR_PYTHON` env vars from systemd service; add `VELOCITY_PDF_BACKEND=go`.
- [x] Delete `.github/workflows/python-ci.yml`.
- [x] Remove `test-python`, `format-python`, `lint-python` from Makefile aggregate targets; mark `install-python` deprecated.
- [x] Update `ARCHITECTURE.md`: component table, performance section, L10 layer, and inter-service diagram updated to reflect Go pipeline.
- [x] Retain `tools/pdf-generator/` in repo history; removal scheduled for v0.6.

**Acceptance:** `make test` passes with no Python deps for reports. ✅

---

### Future Opportunities (post-Phase 5)

The following items emerged during Phase 5 implementation. They are scoped for future branches.

| Item                                     | Notes                                                                                                                                                                              |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Output directory path**                | `getPDFGeneratorDir()` still defaults to `/opt/velocity-report/tools/pdf-generator/`. Rename to `/opt/velocity-report/reports/` and update stage 02 to match.                      |
| **`python3-serial` removal**             | The RS-232 HAT driver (`python3-serial`) is the last Python package in the image. Evaluate a Go serial library (e.g. `go.bug.st/serial`) to remove Python entirely from the image. |
| **8.11 single-survey drift**             | No Python single-survey `.tex` file captured for comparison. Run a single-survey Python build against the Clarendon fixture, save output, then diff against Go golden.             |
| **v0.6 — delete `tools/pdf-generator/`** | Remove the deprecated Python generator directory from the repo. Update all remaining references in docs.                                                                           |
| **Startup `rsvg-convert` check**         | Q4 resolution called for a startup check + mini-PDF test when `rsvg-convert` is missing. Not yet implemented.                                                                      |

---

### Phase 6 — Map Overlay Migration `S`

_(Later branch)_

- [ ] Port `SVGMarkerInjector` (Python `map_utils.py`) to Go `encoding/xml`.
- [ ] Read `map_svg_data` blob from DB via `db.GetSite`.
- [ ] Apply same `rsvg-convert` pipeline as chart SVGs.
- [ ] Test with production site SVG data.

---

### Phase 7 — `grid-heatmap` Migration `S`

_(Later branch)_

- [ ] Implement `cmd/grid-heatmap/` using `chart.RenderGridHeatmap`.
- [ ] Support polar and Cartesian projection modes.
- [ ] Wire as `velocity-report grid-heatmap` subcommand.
- [ ] Deprecate `tools/grid-heatmap/plot_grid_heatmap.py`.

---

## Phase dependency diagram

```
Phase 1 (charts) ──► Phase 2 (tex) ──► Phase 3 (orchestrator)
                                                │
                         ┌──────────────────────┼────────────────────┐
                         ▼                      ▼                    ▼
                    Phase 4a               Phase 4b             Phase 4c
                (flag in handler)        (SVG API)           (CLI binary)
                [medium risk]           [very low risk]     [very low risk]
                         │
                         ▼
                    Phase 5 (cleanup, after 4a validated in prod)
                         │
               ┌─────────┴──────────┐
               ▼                    ▼
          Phase 6 (map)      Phase 7 (grid-heatmap)
```

---

## Testing and validation

### Unit tests

- **Chart SVG structure:** Parse generated SVG, assert expected elements
  (lines, bars, text labels, colours)
- **Template rendering:** Compare `.tex` output against golden files
- **LaTeX escaping:** Edge cases (ampersands, percent signs, backslashes)
- **Number formatting:** Consistent decimal places, locale-independent
- **Histogram bucketing:** Match Go server's existing computation

### Visual regression

- Generate PDF from test database with both Python and Go pipelines
- Compare visually (manual review during development)
- Capture "golden" SVG snapshots for automated comparison after stabilisation

### Integration tests

- In-memory SQLite with known data → `Generate()` → assert PDF exists
- Mock `xelatex` binary for CI (assert `.tex` is well-formed without full
  TeX installation)
- Test `rsvg-convert` fallback (assert graceful error when not installed)

### Backward compatibility

- JSON API response format for `POST /api/generate_report` unchanged
- Report filenames follow existing pattern:
  `{endDate}_velocity.report_{location}_report.pdf`
- `.zip` archive structure preserved (`.tex` + chart assets)

---

## Risks and mitigations

| Risk                                                                            | Impact           | Mitigation                                                                                                                         |
| ------------------------------------------------------------------------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| SVG-to-PDF fidelity via `rsvg-convert`                                          | Text rendering   | Use `rsvg-convert --dpi 150` for consistent sizing; font is embedded in SVG so no system font dependency                           |
| Chart visual parity with matplotlib                                             | User expectation | Side-by-side comparison during development; accept minor styling differences if data accuracy is preserved                         |
| `rsvg-convert` not available on target                                          | Build failure    | Detected at startup with clear warning; graceful error message directs user to `apt install librsvg2-bin` / `brew install librsvg` |
| LaTeX template complexity                                                       | Maintenance      | Keep templates minimal; complex logic stays in Go helpers, templates only interpolate values                                       |
| Go `text/template` default delimiters `{{`/`}}` clash with LaTeX brace grouping | Template errors  | Use custom Go template delimiters `<<` and `>>` via `template.Delims("<<", ">>")` to avoid any ambiguity with LaTeX `{`/`}` syntax |
| Comparison chart normalisation (Q6)                                             | Incorrect chart  | Investigate `/api/radar_stats` response shape before Phase 1; add `NormaliseHistogram` helper if raw counts are returned           |

---

## Relationship to existing decisions

### D-08 (precompiled LaTeX)

This plan is **complementary**. D-08 reduces the TeX installation from
~800 MB to ~30–60 MB. This plan eliminates the Python stack (~450 MB).
Together they reduce the deployment footprint from ~1.25 GB to ~30–60 MB.

The precompiled `.fmt` file (D-08) still applies: the Go-generated `.tex`
compiles against the same minimal TeX tree with the same precompiled format.

### D-09 (single binary)

This plan **enables** D-09's `velocity-report pdf` subcommand without
requiring a bundled Python interpreter. The report generation logic lives
in `internal/report/`, callable from both the API handler and the CLI.
The `grid-heatmap` tool (Phase 7) becomes `velocity-report grid-heatmap`.

### D-10 (RPi image)

This plan **simplifies** the Raspberry Pi image by removing Python entirely
from the report-generation path. The image only needs the Go binary +
vendored TeX tree + `rsvg-convert`.

### D-11 (ECharts → LayerChart migration)

This plan **modifies D-11's scope** for report-view charts. Report charts
(time-series, histogram, comparison) will be served as SVGs via the Go API
and consumed directly by the Svelte frontend, rather than being rewritten in
LayerChart. Non-report charts (live dashboard, real-time stats) remain in
scope for the LayerChart migration. DECISIONS.md D-11 entry is updated to
reflect this split.

---

## Resolved Questions

These questions were open at plan-draft time and have since been answered.

### Q1 — gonum/plot vs direct SVG

**Decision: direct SVG generation via `encoding/xml`.**

gonum/plot's single-axis layout model, inability to share an X-axis between
two Y-scale plots, and lack of built-in day-boundary gap handling make it a
poor fit for the time-series chart. Direct SVG gives pixel-precise control
over the dual-axis layout, low-sample shading, and broken-line segments. It
is also used consistently for the histogram and comparison charts, so the
package has one rendering model throughout. gonum is not used.

### Q2 — Font embedding in SVG

**Decision: embed Atkinson Hyperlegible as a base64 data URI in each SVG.**

This makes each SVG self-contained — `rsvg-convert` requires no font
installed on the system, and the web frontend can embed the SVG directly
without a separate font request. File-size cost is modest and acceptable.

### Q3 — Chart dimensions: mm vs inches

**Decision: millimetres (mm).**

`ChartStyle.Width` and `ChartStyle.Height` are defined in millimetres. mm is
LaTeX-native (used directly in `\includegraphics[width=…mm]`) and avoids any
inches↔px conversion. SVG viewBox uses the mm values multiplied by a DPI
constant (96 px/mm) to produce pixel dimensions; `rsvg-convert` preserves the
physical size at the specified DPI. Frontend rendering reads `width`/`height`
attributes directly.

### Q4 — `rsvg-convert` on macOS / developer setup

**Decision: document in setup guide and add a startup check in v0.6.1.**

`brew install librsvg` is added to the macOS developer prerequisites in
`public_html/src/guides/setup.md`. A startup check added in v0.6.1 detects
whether `rsvg-convert` is available and, if not, logs a clear warning and
runs a one-page mini-PDF test (a single-page LaTeX "flyer" easter egg —
fun, harmless, immediately confirms the full pipeline works end to end).
The check runs when the server starts and also when `velocity-report pdf`
is invoked via CLI.

### Q5 — Web frontend reuse of Go chart SVGs

**Decision: yes, in scope. Go chart package serves SVG to the web frontend.**

The `internal/report/chart` package exposes an HTTP handler (via
`internal/api`) that returns SVG charts for the same data views currently
rendered by ECharts in the Svelte frontend. This is the consistent/DRY path:
one layout engine (direct SVG, same `ChartStyle` struct), one colour palette,
used by both the PDF pipeline and the web frontend.

`ChartStyle` control knobs (dimensions, colours, font size, opacity) are
designed to be consumed coherently by both surfaces. That frontend SVG
consumption has since landed for the dashboard and report-generator preview
surfaces via `InlineSvgChart` on both routes, backed by `/api/charts/*`.
Broader chart-library consolidation outside those report surfaces remains
separate work, but this plan no longer depends on a future PR for report-view
SVG reuse.

### Q6 — Comparison chart normalisation: Go API vs report package

**Decision: the API returns raw counts. Normalisation lives in the chart
rendering layer.**

Investigation confirmed that every layer below the chart renderer deals only
in raw `count` values:

- **DB** (`RadarObjectRollupRange`): returns `map[float64]int64` — bucket
  start in mps → raw count.
- **API** (`/api/radar_stats`): converts keys to display units, values remain
  raw counts (`map[string]int64` in JSON).
- **Report orchestrator** (`internal/report/report.go`): passes
  `HistogramData` with raw counts to the chart package.
- **Chart renderer** (`internal/report/chart/histogram.go`): computes
  percentages at render time, inside `RenderComparison()`.

This matches the Python architecture where `build_comparison()` normalises
inline during rendering. The single-bar `RenderHistogram()` uses raw counts
on the Y-axis (matching Python's `build_histogram()`). The existing
`NormaliseHistogram()` helper only sorts keys and totals counts — the name
is slightly misleading but the total it returns is consumed by
`RenderComparison` for the percentage calculation. No additional helper is
needed.

### Q7 — `grid-heatmap` tool: in scope

**Decision: yes. Migrate `tools/grid-heatmap/` to a Go subcommand.**

`tools/grid-heatmap/plot_grid_heatmap.py` uses matplotlib for polar and
Cartesian background-grid heatmap visualisations. It is migrated to Go as
part of this plan — using the same direct SVG engine — and becomes the
`grid-heatmap` subcommand of the unified `velocity-report` binary (aligning
with D-09). The standalone Python script is deprecated. This is added as
Phase 7 below.

---

## New Backlog Items Arising From This Plan

The following items are added to BACKLOG.md as a result of this plan's
decisions:

1. **v0.7 — Frontend SVG chart consumption:** Implemented for the dashboard
   and report-generator preview surfaces, which now consume Go-generated SVG
   charts from `internal/api` via `InlineSvgChart`. Remaining follow-up, if
   any, is broader chart-library consolidation outside these report views.

2. **v0.7 — `ChartStyle` frontend/backend coherence:**
   Design and ship a mechanism for the Svelte frontend and Go report package
   to consume `ChartStyle` control knobs consistently (e.g. theme tokens
   served via API, or a shared JSON config). Ensures palette, font size, and
   layout constants stay in sync across both rendering surfaces.

---

## Phase 8 — Tex Output Consistency `L`

**Goal:** 100% structural and content parity between the Go-generated and
Python-generated `.tex` output files. The baseline for this comparison is the
real build captured in `comparison/go/report.tex` and `comparison/python/report.tex`
from 2025-06-07 (the Clarendon Avenue San Francisco report).

**Status:** In progress. Build SHA metadata is implemented (8.0). Comparison-mode parity items 8.2–8.7 and 8.10 are substantially landed in code. Measurement tooling (8.1), firmware/version parity in 8.9, and single-survey drift work (8.11) remain open.

---

### 8.0 — Build SHA metadata ✅

Both `.tex` files must be traceable to the pipeline version that generated them.

- [x] `RenderTeX()` in `internal/report/tex/render.go` prepends a metadata comment block:
  ```latex
  % velocity.report tex output
  % Pipeline: go | Version: dev | SHA: unknown
  % Generated: 2026-04-21T12:00:00Z
  %
  ```
  Uses `internal/version.Version`, `internal/version.GitSHA`, and `time.Now().UTC()`.
- [ ] Python: add equivalent comment block to `pdf_generator.py` (or `document_builder.py`).

---

### 8.1 — Measurement methodology

To measure and track drift, we use structural diff rather than byte-level diff,
because Python's PyLaTeX emits `%` terminators on every line whereas Go's
templates do not, making a raw `diff` output noisy.

**Recommended approach:**

```bash
# Strip trailing % and normalise whitespace, then diff
sed 's/%$//' comparison/python/report.tex | sed 's/[[:space:]]*$//' > /tmp/py.tex
sed 's/[[:space:]]*$//' comparison/go/report.tex > /tmp/go.tex
diff -u /tmp/py.tex /tmp/go.tex
```

**Automated test target:** add a `make tex-compare` target that:

1. Generates a Go `.tex` from a known fixture config.
2. Compares against the Python golden file at `tools/pdf-generator/tests/fixtures/golden_comparison.tex`.
3. Runs `diff --unified` and fails if output differs beyond a known-acceptable set of lines
   (preamble font paths, generated timestamps, font block).

**Golden-file tests:** `internal/report/tex/render_test.go` should add `TestRenderTeX_GoldenComparison`
and `TestRenderTeX_GoldenSingle` that compare full `.tex` output against committed golden files
in `internal/report/tex/testdata/`. Use a `-update` flag to regenerate.

- [x] `make tex-compare` target in Makefile.
- [x] Golden files in `internal/report/tex/testdata/golden_{single,comparison}.tex`.
- [x] `TestRenderTeX_GoldenSingle` and `TestRenderTeX_GoldenComparison` in `render_test.go`. Volatile `% Generated:` timestamp line is normalised before comparison.

---

### 8.2 — Comparison mode: Key Metrics table `M` ✅

**Current code status:** Implemented in `internal/report/tex/templates/overview.tex` and `internal/report/tex/helpers.go`. `overview_comparison` now renders a single 4-column comparison table captioned "Table 1: Key Metrics", uses `Site:` in the overview list, and emits comma-formatted per-period and combined counts.

- [x] `FormatDeltaPercent(primary, compare float64) string` in `helpers.go`.
- [x] `FormatCount(n int) string` in `helpers.go`.
- [x] Comparison deltas and counts are precomputed in Go before template render; no template `FuncMap` wiring is needed in the current implementation.
- [x] Update `overview_comparison` in `overview.tex` to the single unified table.
- [x] Update overview itemize to use `\textbf{Site:}` label.
- [x] Update overview itemize to emit combined total count with `FormatCount`.

---

### 8.3 — Comparison mode: Speed Distribution table `M` ✅

**Current code status:** Implemented. Comparison reports now render a 6-column dual-period table via `DualHistogramTableTeX`, produced by `BuildDualHistogramTableTeX(...)` and inserted by `statistics_comparison`.

- [x] `BuildDualHistogramTableTeX(...)` in `helpers.go`.
- [x] `DualHistogramTableTeX string` in `TemplateData`.
- [x] `statistics_comparison` template renders dual table.
- [x] `report.go` builds `DualHistogramTableTeX` when comparison mode.

---

### 8.4 — Comparison mode: Compare timeseries chart `M` ✅

**Current code status:** Implemented. Comparison reports render both t1 and t2 timeseries charts in the one-column chart section.

- [x] `CompareTimeSeriesChart string` in `TemplateData`.
- [x] Second timeseries query + render + rsvg-convert in `report.go`.
- [x] `chart_section_comparison` template updated.

---

### 8.5 — Comparison mode: Daily + Granular data tables `L`

**Current code status:** Structurally implemented. Comparison reports now render three tables: the dual histogram table, a merged daily summary, and a merged granular breakdown. `report.go` queries daily and hourly comparison data, merges both periods, and sorts the merged rows chronologically before building the tables. Exact Python column-width parity was later superseded by the shared centred `BuildStatTableTeX(...)` style now used for both single and comparison reports.

- [x] `DailyStatRows []StatRow` in `TemplateData`; comparison hourly rows are merged into `StatRows` rather than kept in a separate `CompareStatRows` slice.
- [x] Daily-group query + merge in `report.go`.
- [x] Combined chronological sort of `StatRows` (t1 + t2 merged).
- [x] `statistics_comparison` template updated to three-table layout.
- [ ] Exact Python `>{\ttfamily\raggedright...}p{...}` column format: **won't match by design.** Current `BuildStatTableTeX` uses `l`/`r` columns with `\ttfamily\scriptsize` wrapper and grey column rules — this is more readable than Python's `p{...}` approach and is intentional. Closing this sub-item.

---

### 8.6 — Comparison mode: Cosine angle/factor per period `S` ✅

**Current code status:** Implemented. Comparison reports now carry and render per-period cosine angle and factor fields when present.

- [x] `CompareCosineAngle float64`, `CompareCosineFactor float64` in `TemplateData`.
- [x] Populate in `report.go`.
- [x] `survey_parameters_comparison` template updated.

---

### 8.7 — Section ordering and two-column structure `M` ✅

**Current code status:** Implemented. Comparison reports now keep overview, site information, science, survey parameters, and statistics in the two-column flow; the switch to one-column happens at the chart section. The comparison histogram lives in `overview_comparison`, while `statistics_comparison` now focuses on tables.

- [x] `period_report.tex` reordered.
- [x] `overview_comparison` includes histogram figure after Key Metrics.
- [x] `survey_parameters.tex` splits hardware + survey subsections.
- [x] `statistics_comparison` drops histogram figure.
- [x] `report.tex` restructured so site_info and science appear before chart_section.

---

### 8.8 — Velocity Overview list: labels, dates, counts `S`

**Current code status:** Mostly implemented. The overview list now uses `Site:` in both modes, ISO-format dates, combined comma-formatted comparison counts, and explicit compact itemize spacing. The one notable remaining difference is that single-survey mode still shows speed limit in the overview list; that is folded into the broader single-survey parity pass in 8.11.

- [x] Standardise on `Site:` in both modes.
- [x] Add combined count (t1+t2) for comparison mode using `FormatCount`.
- [x] Add inline `\setlength{\itemsep}{-1pt}` etc. to itemize.

---

### 8.9 — Hardware Configuration: split from Survey Parameters `S`

**Current code status:** Partially implemented. `survey_parameters_single` and `survey_parameters_comparison` are now split into `Hardware Configuration` and `Survey Parameters` subsections, but firmware/version fields are still omitted because `TemplateData` does not expose them.

- [x] Split survey_parameters templates into Hardware + Survey subsections.
- [x] `FirmwareVersion string` in `TemplateData` and `report.Config` (optional; row omitted from hardware table when empty).

---

### 8.10 — Science section: two subsections `S` ✅

**Current code status:** Implemented. `science.tex` now has separate `Citizen Radar` and `Aggregation and Percentiles` subsections and includes the expanded explanatory prose.

- [x] `science.tex` split into two subsections with Python's prose.

---

### 8.11 — Single-survey mode: Overview prose (lower priority) `S`

**Current:** itemize list only.

**Target (Python):** Not yet captured — single-survey Python output not in the comparison folder.
Defer until a single-survey Python build is available for comparison.

- [ ] Capture single-survey Python tex output.
- [ ] Identify and list drift items.
- [ ] Align Go `overview_single` and `statistics_single` templates.

---

### Phase 8 implementation order

Implement items in this sequence to avoid regressions:

```
8.0 ✅  Build SHA metadata (done)
8.1 ✅  Measurement tooling (make tex-compare, golden files) — done
8.6 ✅  Cosine per period (done)
8.2 ✅  Key Metrics table (done)
8.8 ✅  Overview list alignment (done; single-survey speed-limit review deferred to 8.11)
8.9 ✅  Hardware Config split + FirmwareVersion field (done)
8.3 ✅  Dual histogram table (done)
8.4 ✅  Compare timeseries chart (done)
8.5 ✅  Daily + Granular tables (done; column-format parity closed — won't match Python by design)
8.7 ✅  Section ordering (done)
8.10 ✅ Science section expansion (done)
8.11    Single-survey drift (deferred — no Python single-survey .tex available for comparison)
```

**Phase 8 acceptance:** `make tex-compare` exits 0; golden-file tests green;
a PDF compiled from Go tex is visually indistinguishable from a PDF compiled from Python tex
for the same data, in both comparison and single-survey modes.
