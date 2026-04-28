# PDF report design (canonical)

## 0. Status

Source of truth for visual, layout, and chart-rendering decisions in the
generated PDF report surface. Companion to [DESIGN.md](DESIGN.md) (which
governs the cross-platform contract across web, macOS, and PDF).

This document is **renderer-independent**: every decision below is recorded
with the rationale that motivated it, so a future migration away from
xelatex (Typst, ConTeXt, HTML→PDF, gofpdf, etc.) can replicate the design
without re-deriving the constraints from the existing `.tex` templates.

Operational details (deployment, fonts on disk, xelatex install,
rsvg-convert pipeline) live in
[platform/operations/pdf-reporting.md](../platform/operations/pdf-reporting.md).
Historical migration record:
[plans/pdf-go-chart-migration-plan.md](../plans/pdf-go-chart-migration-plan.md).

## 1. Pipeline shape (renderer-independent)

```
DB query ─► Go SVG charts (internal/report/chart) ─► chart.svg / chart.pdf
                                                          │
Go templates (internal/report/tex/templates) ─► report.tex┤
                                                          ▼
                                               compositor (xelatex today)
                                                          │
                                                          ▼
                                                       report.pdf
```

The pipeline has three independent stages:

| Stage              | Today                   | Inputs                | Outputs               |
| ------------------ | ----------------------- | --------------------- | --------------------- |
| **Chart render**   | Go SVG builder          | `TimeSeriesData` etc. | SVG bytes             |
| **SVG → image**    | `rsvg-convert -f pdf`   | SVG bytes             | per-chart PDF         |
| **Doc compositor** | xelatex on Go templates | `.tex` + chart PDFs   | final `.pdf` + `.zip` |

The chart-render stage is fully portable. The doc-compositor stage is the
only place a future migration touches: every visual decision below either
lives in the Go SVG code (carries over unchanged) or in the `.tex` templates
(must be re-expressed in the new compositor's language).

## 2. Document structure

### 2.1 Layout and section order

Two-column body (`\twocolumn`) on a centred title block (`@twocolumnfalse`).
The chart figures break out to single column (`\onecolumn`) so a wide
time-series fits at full text-width.

Section order is invariant:

1. Title block: site name, surveyor/contact, period range
2. **Velocity Overview** — site, period(s), total count, speed limit, key-metrics table, histogram thumbnail
3. **Site Information** — optional description and speed-limit note
4. **The Science** — methodology paragraph
5. **Hardware Configuration** + **Survey Parameters** — two stacked tables
6. **Statistics** — supertabular percentile breakdown(s)
7. **Chart Section** — full-width time-series figure(s)
8. **Map Section** — optional site-map figure on a final page

Single vs comparison reports share section identity; only the _content_ of
each section differs (one period vs two). This is enforced by the dispatch
template `period_report.tex`:

```
{{if .CompareStartDate}}{{template "period_report_comparison" .}}
{{else}}{{template "period_report_single" .}}{{end}}
```

with paired `_single` / `_comparison` definitions for every section.

### 2.2 Page geometry

| Margin | Size  | Reason                                                                                                  |
| ------ | ----- | ------------------------------------------------------------------------------------------------------- |
| Top    | 1.8cm | Header rule + site location                                                                             |
| Bottom | 1.0cm | Footer rule with period range and page number                                                           |
| Left   | 1.0cm | Tight side margin — column gutter (`\columnsep=14pt`) does the visual work between the two body columns |
| Right  | 1.0cm | "                                                                                                       |

US Letter is the default Python-standard report size; A4 remains an explicit
option. `paperTextWidthMM` (`chart/config.go`)
returns the matching textwidth so chart SVGs are emitted at the _exact_
physical dimensions the LaTeX `\includegraphics[width=\textwidth]` will
honour. Charts must never be re-scaled by the compositor — that is what
keeps font sizes and tick density legible.

## 3. Typography

- **Family:** Atkinson Hyperlegible (open-licence, optimised for reduced
  visual acuity). Embedded into both the SVG charts and the LaTeX
  document via `\setmainfont{...}[Path=<FontDir>/...]` (`fontspec`).
- **Sizes:** body 11pt; section headers `\Large` bold; subsection
  `\large` bold; tick labels in a separate `AxisTickFontPx` scale (see §4).
- **Single source for the font binary:** `internal/report/chart/assets/`
  via `//go:embed`, surfaced by `assets.AllFonts()`. The chart package and
  the workdir copy that xelatex reads both pull from the same place.

A future renderer must:

- embed Atkinson Hyperlegible the same way (or the report changes brand);
- preserve the LaTeX-equivalent font sizes converted to its own unit system.

## 4. Chart design

### 4.1 Sizing policy

Charts are rendered at _physical_ mm dimensions, then converted to PDF at
96 DPI by `rsvg-convert`. There is no scaling step inside the compositor.

| Chart                  | Width                  | Aspect | Reason                                                                                                               |
| ---------------------- | ---------------------- | ------ | -------------------------------------------------------------------------------------------------------------------- |
| Time-series            | `paperTextWidthMM`     | 2.7:1  | Full text width (single-column block); 2.7:1 leaves room for legend underneath without forcing labels into rotation. |
| Histogram (single)     | `paperTextWidthMM / 2` | 1:0.55 | Half-width so two histograms or a histogram + key-metrics fit side by side in the two-column body.                   |
| Histogram (comparison) | `paperTextWidthMM / 2` | 1:0.55 | Same; grouped bars share the slot.                                                                                   |

Font sizes in `ChartStyle` are tuned to the physical chart width so 9.5px
ticks render as readable type at print scale.

### 4.2 Time-series chart

The flagship chart: percentile lines, reference lines, count bars, and
gap dividers on a dual-axis canvas.

#### Y-axes

- **Left axis:** speed. Tick step is `niceStep(rawSpeedMax, 6)` with a
  floor of 5. So a 35 mph range yields step=5 (8 ticks); a 60 mph range
  yields step=10 (7 ticks). This avoids 13 cramped ticks at higher ranges.
- **Right axis:** observation count. Tick step is `niceStep(countAxisMax, 4)`
  — independent of the speed axis because counts can run from tens to
  millions and need their own magnitude-aware step.

Both ceilings are rounded _up_ to the next multiple of the step so the top
tick is a round number ("60", "1000"), never "57" or "950".

#### Series

| Series | Colour    | Marker   | Line style    | Why                                                          |
| ------ | --------- | -------- | ------------- | ------------------------------------------------------------ |
| `p50`  | `#fbd92f` | triangle | solid         | Median; warm yellow keeps it visually subordinate.           |
| `p85`  | `#f7b32b` | square   | solid         | Upper-typical; orange escalates from p50.                    |
| `p98`  | `#f25f5c` | circle   | solid         | The number stakeholders argue about; warning red.            |
| `max`  | `#2d1e2f` | (none)   | dashed `1 3`  | Single-event line, not population statistic; muted dark hue. |
| count  | `#2d1e2f` | bar      | filled, α=.25 | Volume context behind the percentile lines.                  |

Legend order is fixed: p50, p85, p98, max, then auxiliary signals
(reference lines, low-sample swatch). Same order as the web app — change
both at once.

#### Series segmentation (NaN gaps)

Polylines must remain **continuous** across day boundaries. They break
_only_ at NaN values. NaN is generated when:

- the bucket genuinely has no samples; or
- the bucket has fewer than `CountMissingThreshold` samples (default 5)
  — masked via `ApplyCountMask`, which copies the slice and NaN-ifies
  speeds while preserving `Count` so the count bar and tooltip still
  show the truth.

Each contiguous run of non-NaN samples emits its own `<polyline>`. Markers
are skipped at NaN positions so a missing point looks like a missing
point, not a connect-the-dots artefact.

#### Gap dividers (replaces day-boundary markers)

A single dashed vertical line is drawn at the _first_ index of every NaN
run. This is the only vertical divider on the chart — day boundaries do
**not** get their own marker, because crossing midnight inside a
contiguous run of data should not visually fragment the line.

```
inGap := false
for i := range n {
    isNaN := math.IsNaN(maskedPts[i].P50Speed)
    if !inGap && isNaN && i > 0 {
        x := leftPx + float64(i)/n * plotW
        line(x, topPx, x, bottomPx,
             stroke="#999" stroke-dasharray="3 3" opacity="0.6")
    }
    inGap = isNaN
}
```

Rationale: a 3-day report with eight hours of data per day used to render
as a continuous line jumping over the gaps with day-boundary verticals
that didn't help orient the reader. Now it renders as three visually
separate "blocks" with a dashed cut between them, which matches how
operators read the data.

#### Reference lines

Two horizontal dashed lines, drawn behind the series:

- `p98=NN` in `ColourP98` at `data.P98Reference` (the aggregate p98
  across the full range — the headline figure).
- `max=NN` in `ColourMax` at `data.MaxReference`.

Both labels are right-aligned at `leftPx-6` on the speed axis, beside (not
on top of) regular tick numbers. To prevent overlap when the reference
line lands close to a tick, the label is drawn with a tight white
background rect (no stroke):

```
labelWithBg(x, y, "p98=34", colour):
    estW = len(label) * 0.62 * fontPx
    rect(x - estW - 2, y - 0.8*fontPx - 2, estW + 4, fontPx + 4, fill=white)
    text(x, y, "p98=34", text-anchor=end, fill=colour, weight=bold)
```

This is needed because SVG has no native text-with-background primitive;
the compositor cannot help. A future renderer must reproduce the
manually-drawn background rect.

#### X-axis

Tick cadence is **span-aware** (`pickTickCadence`):

| Span   | Cadence      | Label format   |
| ------ | ------------ | -------------- |
| ≤12h   | 2-hourly     | `15:04`        |
| ≤48h   | 6-hourly     | `Jan 02 15:04` |
| ≤7d    | 12-hourly    | `Jan 02 15:04` |
| ≤14d   | daily        | `Jan 02`       |
| ≤90d   | weekly (ISO) | `Jan 02`       |
| longer | monthly      | `Jan 2006`     |

Target: 6–10 visible labels regardless of span (matching DESIGN.md §4.1).
Labels rotate -45° automatically when projected tick spacing falls below
the estimated label width.

### 4.3 Histogram chart

#### Y-axis is **percentage**, not count

Both `RenderHistogram` (single) and `RenderComparison` (grouped) display
percentage of period total on the Y-axis. Reasons:

- A two-period comparison must show comparable bars; raw counts disagree
  even when the _shape_ of the distribution is identical.
- A reader scanning the headline histogram cares about _what fraction of
  drivers exceeded 30 mph_, not how many absolute observations there were.
- The companion histogram **table** (built by `BuildHistogramTableTeX` /
  `BuildDualHistogramTableTeX`) shows both count and percent; the chart
  reflects the more interpretable axis.

#### Tick step

`pctStep := pctNiceStep(maxPct)` — wraps `niceStep(maxPct, 5)` with a
minimum of 5. So a peaked distribution at 60% (where one bucket holds
60% of the population) renders 0/10/20/30/40/50/60 (step=10), not
13 cramped ticks at step=5.

#### Bucket labels

`BucketLabel(lo, hi, maxBucket)` returns:

- `"20-25"` for an interior bucket, or
- `"70+"` for the saturating bucket at and above `maxBucket`.

Labels rotate -45° at the X axis to fit the narrow column width.

#### Single vs comparison

| Mode       | Bars                   | Colour                           | Width                             |
| ---------- | ---------------------- | -------------------------------- | --------------------------------- |
| Single     | One per bucket         | `ColourSteelBlue` (α=0.7)        | `BarWidthFraction` of slot        |
| Comparison | Two grouped per bucket | t1: `ColourP50`, t2: `ColourP98` | half-slot each, with internal gap |

Comparison legend lays out left and right halves: t1 in left quarter,
t2 in right quarter, so date-range labels (which can be 20+ chars)
do not overlap.

## 5. Tables

### 5.1 Table-styling helper

`withStyledTable(b, fontSize, body, afterReset)` (`tex/helpers.go`) is
the canonical way to emit any styled table. It opens a group, applies
typeface and spacing, calls `\rowcolors{...}`, runs the body, and
resets — the helper is the only place those styling decisions live:

```
{
  \AtkinsonMono\<fontSize>
  \renewcommand{\arraystretch}{1.04}
  \setlength{\tabcolsep}{2pt}
  \rowcolors{2}{black!2}{white}
  <body>
  \rowcolors{0}{}{}
  <afterReset>
}
```

All page-spanning data tables (stat table, histogram table, dual
histogram table) use this helper. New tables should use it too — do
not reach for `\rowcolors` directly.

### 5.2 Row-colour rules

| Table                              | Row 1              | Body alternation             | Why `\rowcolors` start row                   |
| ---------------------------------- | ------------------ | ---------------------------- | -------------------------------------------- |
| Stat table, histogram tables       | header (no colour) | `black!2` / white from row 2 | `\rowcolors{2}` skips the styled header      |
| Hardware Config, Survey Parameters | data row 1         | `black!2` / white from row 1 | `\rowcolors{1}` — there is no header to skip |
| Key Metrics                        | header             | `black!2` / white from row 2 | Header present                               |

**`black!2` is the canonical alternating tint** across all tables — match
it whenever a new table is added. The colour is deliberately light
(printer-safe, gray-only) and works on every output medium.

### 5.3 Column rules

The stat and histogram tables draw thin vertical column rules
(`!{\color{black!20}\vrule}`) between every column to keep aligned
numbers readable in a dense supertabular. Hardware/Survey parameters
do not — those are two-column key/value tables and rules add clutter.

### 5.4 Survey-parameters labels

The left key column is `p{0.44\linewidth}` and lives inside a two-column
body, so it is narrow. Labels were shortened deliberately to prevent
wrapping:

| Old label                   | New label                         |
| --------------------------- | --------------------------------- |
| `Minimum speed (cutoff):`   | `Min speed:`                      |
| `Cosine Error Angle:`       | `Cosine angle:`                   |
| `Cosine Error Factor:`      | `Cosine factor:`                  |
| `Data Source:` (single)     | `Data source:`                    |
| `Data Source:` (comparison) | `Source (t1):` and `Source (t2):` |

A new field that needs to fit here should aim for ≤16 characters
including punctuation, or it will wrap onto two lines.

## 6. Comparison-mode design

### 6.1 t1/t2 nomenclature

When a report shows two periods, the two periods are always referred to
as **t1** (primary) and **t2** (compared). This is the user-facing label
in:

- Survey Parameters table rows (`Start time (t1)`, `Source (t2)`, etc.)
- Statistics table headings (`t1 Count`, `t2 %`, `Delta`)
- Chart legends and figure captions
- The footer period range (`<t1 range> vs <t2 range>`)

Using `t1`/`t2` everywhere lets readers pivot between sections without
re-establishing which period is which.

### 6.2 Per-period fields

Each period carries its own:

- **Source** (`Source` and `CompareSource`) — t2's source can differ from
  t1 (e.g. t1 from `radar_data_transits`, t2 from `radar_objects` after
  a sensor swap). Both must be displayed.
- **Cosine angle / factor** (`CosineAngle` + `CompareCosineAngle`) — the
  sensor angle correction is a per-period property; if t1 and t2 used
  different mounting angles, both show.

When either `CosineAngle > 0` or `CompareCosineAngle > 0`, the survey
parameters section appends the line _"Speeds have been corrected for
sensor angle changes."_ — disclosure that values are not raw.

### 6.3 Deltas

Statistics show absolute and percentage deltas:

- `DeltaP50` etc. — signed difference `(t2 − t1)`, formatted `+1.50` /
  `-0.40`. Source: `FormatDelta(primary, compare)`.
- `DeltaP50Pct` etc. — signed percentage change `(t2 − t1) / t1 * 100`,
  formatted `+6.1%` / `-3.2%`. Source: `FormatDeltaPercent`.

NaN inputs produce `--`. Zero baseline produces `--` for the percent
delta (no division by zero implied).

## 7. Palette

Every colour used in the PDF chart engine resolves to a constant in
`internal/report/chart/palette.go`. The palette is deliberately small:

| Constant          | Hex       | Used in                                             |
| ----------------- | --------- | --------------------------------------------------- |
| `ColourP50`       | `#fbd92f` | p50 series, comparison-bar t1 fill                  |
| `ColourP85`       | `#f7b32b` | p85 series, low-sample background swatch            |
| `ColourP98`       | `#f25f5c` | p98 series, p98 reference line, comparison t2 fill  |
| `ColourMax`       | `#2d1e2f` | max series, max reference line, count bars          |
| `ColourCountBar`  | `#2d1e2f` | (alias for max — count bars share the dark hue)     |
| `ColourLowSample` | `#f7b32b` | (alias for p85 — low-sample swatch shares the warn) |
| `ColourSteelBlue` | `#4682b4` | Histogram bars (single mode)                        |

Cross-platform palette source of truth and rationale live in
[DESIGN.md §3.3](DESIGN.md#33-percentile-metric-colour-mapping-charts).
Web (`palette.ts`), Go PDF (this file), and macOS must stay in sync —
change all three in the same PR.

LaTeX-side colour definitions (`\definecolor{vrP50}{HTML}{fbd92f}` etc.)
in `preamble.tex` are decorative-only (footer / header). They duplicate
the palette into the typesetter's namespace; if the chart palette
changes, regenerate them.

## 8. Renderer-portability cheat sheet

If/when xelatex is replaced, the following responsibilities have to be
re-homed in the new compositor. Rows are sorted by migration risk.

| Responsibility                                   | xelatex implementation                                    | What an alternative needs                                                        |
| ------------------------------------------------ | --------------------------------------------------------- | -------------------------------------------------------------------------------- |
| Embed Atkinson Hyperlegible                      | `\setmainfont{...}[Path=...]` via `fontspec`              | Native font embedding (Typst `text(font:...)`, CSS `@font-face`, etc.)           |
| Two-column flow + single-column figure breakouts | `\twocolumn[...]` + `\onecolumn` per figure               | Multi-column page layout with break-out support                                  |
| Page-spanning, multi-page tables                 | `supertabular` + `\tablehead` / `\tabletail`              | Repeating table head, page-spanning rows                                         |
| Alternating row colours                          | `colortbl` `\rowcolors{n}{a}{b}`                          | Same effect; styled-row alternation by start-row index                           |
| Inline column rules                              | `!{\color{black!20}\vrule}` in `tabular` colspec          | Per-column-rule colour control                                                   |
| SVG embedding                                    | `rsvg-convert` to PDF, then `\includegraphics{chart.pdf}` | Native SVG support (Typst, HTML/CSS), or keep `rsvg-convert` and embed as image  |
| Header/footer with site location + period        | `fancyhdr` `\fancyhead/\fancyfoot`                        | Equivalent running-header support with template fields                           |
| Special-character escaping                       | `EscapeTeX` (Go) on every interpolated string             | Per-renderer escape function over the same character set (`& % $ # _ { } ~ ^ \`) |
| Hyperlinks                                       | `hyperref` package + `\href{}{}`                          | Native hyperlink support                                                         |

The chart-render stage produces SVG — it is independent of the doc
compositor. A migration only needs to:

1. Re-express the eight `templates/*.tex` files in the new template
   language, preserving the section order from §2.1.
2. Re-implement `BuildStatTableTeX`, `BuildHistogramTableTeX`, and
   `BuildDualHistogramTableTeX` (currently in `tex/helpers.go`) in the
   new compositor's table primitive.
3. Re-implement `EscapeTeX` for the new template language.
4. Embed Atkinson Hyperlegible per §3.

Everything in §4 (chart design) is preserved as-is — the SVGs do not
change.

## 9. Cross-references

- Cross-platform contract (palette, legend order, tick density,
  segmentation rules): [DESIGN.md](DESIGN.md)
- Operations and deployment (fonts, xelatex install,
  rsvg-convert, package layout):
  [platform/operations/pdf-reporting.md](../platform/operations/pdf-reporting.md)
- Migration history (Python → Go):
  [plans/pdf-go-chart-migration-plan.md](../plans/pdf-go-chart-migration-plan.md)

## 10. PR checklist (PDF report changes)

Before merging a PR that touches the PDF report:

- [ ] Palette changes propagated to web and macOS (DESIGN.md §3.3).
- [ ] New table uses `withStyledTable`; alternating tint is `black!2`.
- [ ] New survey-parameter rows fit the 0.44-linewidth left column.
- [ ] Comparison sections render _both_ t1 and t2 of every per-period
      field (source, cosine angle, cosine factor).
- [ ] Chart axis tick step is computed (`niceStep` / `pctNiceStep`),
      not hardcoded.
- [ ] NaN handling: polylines break only on NaN; markers skip NaN;
      gap divider drawn at the first NaN of each run.
- [ ] Reference-line labels use `labelWithBg` (white-fill rect behind
      the text).
- [ ] Golden tex files regenerated (`go test ./internal/report/tex/... -update`).
- [ ] `make lint-go && make test-go` green.
