# PDF generation migration to Go

- **Status:** Ready for implementation — all open questions resolved except Q6 (pre-Phase 1 research)
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

## Implementation phases

### Phase 1: chart package foundation (`internal/report/chart/`); `M`

Build the SVG chart generation library:

1. Define `ChartStyle` struct with colour, font, layout constants (dimensions in mm)
2. Implement low-level SVG helpers in `svg.go` (`rect`, `polyline`, `text`, `line`)
3. Implement `RenderHistogram(data, style) → []byte` (simplest chart first)
4. Implement `RenderTimeSeries(data, style) → []byte` (most complex chart)
5. Implement `RenderComparison(a, b, style) → []byte`
6. Embed Atkinson Hyperlegible font as base64 data URI in SVG `<style>` block
7. Write comprehensive tests comparing output SVG structure against golden files

**Acceptance:** SVG output for all three chart types passes visual review
against equivalent matplotlib output. All tests pass.

### Phase 2: LaTeX template engine (`internal/report/tex/`); `S`

Build the template-based `.tex` generation:

1. Create `go:embed`-ed `.tex` templates for each report section
2. Implement `RenderTeX(data) → []byte` using `text/template`
3. Implement `EscapeTeX()` for safe string interpolation
4. Implement table formatting helpers (`FormatHourlyTable`, etc.)
5. Write tests asserting template output matches expected `.tex` fragments

**Acceptance:** `RenderTeX()` produces a `.tex` file that compiles with
`xelatex` to a visually equivalent PDF.

### Phase 3: report orchestrator (`internal/report/`); `S`

Wire charts + templates + compilation together:

1. Implement `Generate(ctx, db, cfg) → (ReportResult, error)`
2. Query database directly (no HTTP round-trip)
3. Render SVG charts → convert to PDF via `rsvg-convert`
4. Render `.tex` from templates → invoke `xelatex`
5. Package `.zip` with `.tex` source + `.svg` charts
6. Write integration test with in-memory SQLite + mock xelatex

**Acceptance:** End-to-end report generation from test database produces
valid PDF.

### Phase 4: API and CLI integration; `S`

Connect the new report pipeline to existing entry points:

1. Replace `exec.Command("python"...)` in `generateReport()` with direct
   call to `report.Generate()`
2. Add `pdf` subcommand to `velocity-report` CLI (aligns with D-09)
3. Add HTTP SVG endpoints to `internal/api/`: `GET /api/charts/timeseries`,
   `/api/charts/histogram`, `/api/charts/comparison` — these serve the same
   SVG artefacts for web frontend consumption (see Q5)
4. Maintain backward-compatible JSON response format for `POST /api/generate_report`
5. Update web frontend if API response structure changes

**Acceptance:** `POST /api/generate_report` produces identical results using
Go-native pipeline. SVG chart endpoints return valid SVG with embedded font.

### Phase 5: Python deprecation and cleanup; `S`

Remove the Python PDF stack:

1. Mark [tools/pdf-generator/](../../tools/pdf-generator) as deprecated (one release cycle)
2. Remove Python execution path from `server.go`
3. Remove `make install-python` dependency from report generation targets
4. Update [ARCHITECTURE.md](../../ARCHITECTURE.md), component READMEs
5. Retain [tools/pdf-generator/](../../tools/pdf-generator) in repository history (do not delete
   immediately: keep for reference during the transition)

**Acceptance:** `make test` passes with no Python dependencies for report
generation. Python venv is no longer required for any report or chart tooling.

### Phase 6: map overlay migration; `S`

Migrate the SVG marker injection:

1. Port `SVGMarkerInjector` logic to Go using `encoding/xml`
2. Reuse existing `map_svg_data` blob from database
3. Apply same `rsvg-convert` pipeline for map SVG→PDF
4. Test with production site SVG data

**Acceptance:** Map overlays in PDF reports match Python-generated output.

### Phase 7: `grid-heatmap` Migration — `S`

Migrate `tools/grid-heatmap/plot_grid_heatmap.py` to Go:

1. Implement `cmd/grid-heatmap/` using the same direct SVG chart package
2. Support polar (ring × azimuth) and Cartesian (X–Y) projection modes
3. Support single-snapshot, live-periodic, and PCAP-replay modes (matching
   the current Python CLI flags)
4. Wire as `velocity-report grid-heatmap` subcommand (D-09 alignment)
5. Deprecate and remove `tools/grid-heatmap/` Python script
6. Update `make` targets to point at the new subcommand

**Acceptance:** `velocity-report grid-heatmap` produces equivalent SVG output
for both projection modes. Python script is removed from the repository.

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
designed to be consumed coherently by both surfaces. A future PR will ship
the frontend SVG consumption and retire ECharts for these report views;
that work is tracked as a new backlog item under v0.7 (see Backlog item added
below). This plan does not block on the frontend change — the HTTP SVG
endpoint ships as part of Phase 4 and the frontend continues to use ECharts
until the v0.7 work lands.

### Q6 — Comparison chart normalisation: Go API vs report package

**Research needed — tracked for pre-implementation investigation.**

The Python `build_comparison` method normalises raw histogram bucket counts
to percentages before rendering. It is not yet confirmed whether:

(a) The existing Go `/api/radar_stats` response already returns normalised
histogram data (fractions or percentages), or

(b) The report package must compute the normalisation itself from raw counts.

**Investigation required before Phase 1:** read `internal/api/` and the
`radar_stats` handler to determine what histogram shape the API returns.
If raw counts, add a `NormaliseHistogram(counts []int) []float64` helper in
`internal/report/chart/` and test it. If already normalised, use directly.
This does not gate Phase 1 (histogram rendering) but must be resolved before
the comparison chart is implemented.

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

1. **v0.7 — Frontend SVG chart consumption:** Wire Svelte frontend to consume
   Go-generated SVG charts from `internal/api` for report views; retire
   ECharts for those views. `ChartStyle` control knobs already designed for
   dual-surface use. New `M` item under v0.7 (frontend consolidation theme).

2. **v0.7 — `ChartStyle` frontend/backend coherence:**
   Design and ship a mechanism for the Svelte frontend and Go report package
   to consume `ChartStyle` control knobs consistently (e.g. theme tokens
   served via API, or a shared JSON config). Ensures palette, font size, and
   layout constants stay in sync across both rendering surfaces.
