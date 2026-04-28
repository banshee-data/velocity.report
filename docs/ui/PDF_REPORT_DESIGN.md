# PDF report design (canonical)

## 0. Status

Source of truth for visual, layout, and chart-rendering decisions in the
generated PDF report surface. Companion to [DESIGN.md](DESIGN.md), which
records the cross-platform intent across web, macOS, and PDF.

This document is renderer-independent where possible, but it records the
current implementation faithfully. If the PDF renderer diverges from an older
cross-platform assumption, this file describes the code that ships today.

Operational details such as XeLaTeX installation, vendored TeX runtime, and
`rsvg-convert` setup live in
[platform/operations/pdf-reporting.md](../platform/operations/pdf-reporting.md).
Historical migration record:
[plans/pdf-go-chart-migration-plan.md](../plans/pdf-go-chart-migration-plan.md).

## 1. Pipeline shape (renderer-independent)

```text
DB query -> Go SVG charts (internal/report/chart) -> chart.svg / chart.pdf
                                                       |
Go templates (internal/report/tex/templates) -> report.tex
                                                       |
                                                       v
                                            compositor (xelatex today)
                                                       |
                                                       v
                                             report.pdf + source.zip
```

The pipeline has three independent stages:

| Stage              | Today                   | Inputs                                          | Outputs                                                 |
| ------------------ | ----------------------- | ----------------------------------------------- | ------------------------------------------------------- |
| **Chart render**   | Go SVG builder          | `TimeSeriesData`, `HistogramData`, chart styles | SVG bytes                                               |
| **SVG -> image**   | `rsvg-convert -f pdf`   | SVG bytes                                       | per-chart PDF, with the SVG retained for the source ZIP |
| **Doc compositor** | xelatex on Go templates | `report.tex`, chart PDFs, packaged fonts        | final `.pdf` and source `.zip`                          |

The chart-render stage is portable. The compositor stage is the only place a
future migration has to re-express layout semantics.

The source ZIP is part of the product surface. It currently contains:

- `report.tex`
- chart SVG sources (`timeseries.svg`, `histogram.svg`, `comparison.svg`,
  `map.svg` when present)
- the embedded Atkinson font files under `fonts/`
- a `README.md` explaining how to rebuild the PDF locally

### 1.1 Inputs that change the rendered output

The PDF layout is driven by a small set of runtime knobs in
`internal/api/server_reports_generate.go` and `internal/report/config.go`:

| Input             | Current behaviour                                                                                                                                                                                  |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `paper_size`      | Defaults to US Letter. `a4` remains an explicit option. This controls both LaTeX paper geometry and chart physical dimensions.                                                                     |
| `expanded_chart`  | Default `false` keeps sparse time-series charts consolidated. `true` inserts explicit missing buckets across the full requested range, which changes spacing, gap rendering, and SVG tooltip text. |
| `histogram`       | Enables the overview histogram and the histogram table(s).                                                                                                                                         |
| `include_map`     | Enables the final one-column map page when map SVG bytes are present.                                                                                                                              |
| comparison period | Adds the grouped comparison histogram, comparison time-series figure, and comparison tables.                                                                                                       |
| `compare_source`  | Changes which dataset is queried for t2. If omitted, t2 falls back to the primary `source`. This affects the plotted and tabulated data but is not currently printed in the PDF text.              |

## 2. Document structure

### 2.1 Layout and section order

The report uses a centred title block above a two-column body:

- `report.tex` opens with `\twocolumn[...]` and a `@twocolumnfalse` title block.
- Full-width chart figures switch the document to `\onecolumn`.
- The optional map page forces `\clearpage` and then `\onecolumn`.

Section order is invariant:

1. Title block: site name, then optional surveyor/contact line. The contact is
   rendered as a `mailto:` hyperlink when present. The date range is **not** in
   the title block; it lives in the footer.
2. **Velocity Overview**:
   Single mode shows site, period, total count, speed limit, key metrics, and
   the optional single histogram.
   Comparison mode shows site, primary period (t1), comparison period (t2),
   combined count, key metrics, and the optional grouped comparison histogram.
3. **Site Information**: optional description and optional speed-limit note.
4. **Citizen Radar** and **Aggregation and Percentiles**: the science copy is
   split across these two subsections, not a single generic "science" heading.
5. **Hardware Configuration** and **Survey Parameters**: two stacked key/value
   tables.
6. **Statistics**:
   Single mode shows the optional histogram table and a granular percentile
   breakdown.
   Comparison mode shows the dual histogram table, a daily percentile summary,
   and a granular merged comparison breakdown.
7. **Chart Section**:
   Single mode renders one full-width time-series chart.
   Comparison mode renders up to two full-width time-series charts, first t1
   and then t2.
8. **Map Section**: optional final-page site map.

Single vs comparison dispatch is controlled by `period_report.tex`:

```text
<<if .CompareStartDate>>
<<template "period_report_comparison" .>>
<<else>>
<<template "period_report_single" .>>
<<end>>
```

### 2.2 Page geometry and paper

The preamble uses `\documentclass[10pt,<paper option>]{article}`.

| Margin | Size  | Reason                                                                    |
| ------ | ----- | ------------------------------------------------------------------------- |
| Top    | 1.8cm | Header rule plus site location                                            |
| Bottom | 1.0cm | Footer rule plus period range and page number                             |
| Left   | 1.0cm | Tight side margin; the 14pt column gutter does the visual separation work |
| Right  | 1.0cm | Symmetric with left                                                       |

Other fixed layout values:

- `\columnsep = 14pt`
- `\headrulewidth = 0.8pt`
- `\footrulewidth = 0.8pt`
- `\headheight = 12pt`
- `\headsep = 10pt`

`paperTextWidthMM()` in `internal/report/chart/config.go` is the canonical
bridge between page geometry and SVG chart dimensions:

- Letter text width: `215.9mm - 20mm = 195.9mm`
- A4 text width: `210.0mm - 20mm = 190.0mm`

Charts are emitted at those physical sizes and then included into LaTeX at
`\textwidth` or `\linewidth`. The compositor should not add another scaling
decision on top.

### 2.3 Header and footer contract

The running header/footer is part of the visual design:

- Header left: bold `velocity.report` hyperlink
- Header right: italic site/location text
- Footer left: period range in `YYYY-MM-DD to YYYY-MM-DD`, with `vs` between t1
  and t2 in comparison mode
- Footer right: page number

## 3. Typography

The report uses a deliberate split between narrative text and data tables.

- **Narrative family:** Atkinson Hyperlegible, embedded as the document's sans
  family through `\setsansfont[...]` plus `\renewcommand{\familydefault}{\sfdefault}`.
- **Data-table family:** Atkinson Hyperlegible Mono, embedded separately as
  `\AtkinsonMono` and used only inside the reusable table helper.
- **Body size:** 10pt article class.
- **Section heads:** `\Large` bold for sections, `\large` bold for subsections.
- **Captions:** `\captionsetup{font=small,labelfont=bf,textfont=bf}`. Captions are
  bold sans, not monospace.

The packaged font set is currently:

- `AtkinsonHyperlegible-Regular.ttf`
- `AtkinsonHyperlegible-Bold.ttf`
- `AtkinsonHyperlegible-Italic.ttf`
- `AtkinsonHyperlegible-BoldItalic.ttf`
- `AtkinsonHyperlegibleMono-VariableFont_wght.ttf`
- `AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf`

All six files come from `internal/report/chart/assets/` via `//go:embed` and
`assets.AllFonts()`. The chart renderer, the temporary workdir, and the source
ZIP all consume the same bytes.

A future renderer must preserve both the sans/mono split and the same bundled
font provenance, or the report's tone and density will drift.

## 4. Chart design

### 4.1 Sizing policy

Charts are rendered at physical millimetre dimensions and converted at 96 DPI.
There is no compositor-side relayout.

| Chart                  | Width                  | Aspect | Current rationale                                                                                                     |
| ---------------------- | ---------------------- | ------ | --------------------------------------------------------------------------------------------------------------------- |
| Time-series            | `paperTextWidthMM`     | 2.7:1  | Full-width chart section with room for dual Y axes, horizontal X labels, and bottom legend                            |
| Histogram (single)     | `paperTextWidthMM / 2` | 1:0.55 | One text-column chart in the two-column overview                                                                      |
| Histogram (comparison) | `paperTextWidthMM / 2` | 1:0.70 | Same column width, but taller to fit the internal chart title, rotated bucket labels, axis labels, and in-plot legend |

The single histogram and grouped comparison histogram are both included at
`\linewidth` inside the overview's two-column flow. The grouped comparison
version is taller, not wider.

### 4.2 Time-series chart

The flagship chart combines percentile lines, count bars, reference lines,
gap dividers, dual Y axes, and a bottom legend.

#### Y axes

- **Left axis:** speed. The renderer uses `niceStep(rawSpeedMax, 6)` with a
  floor of 5, where `rawSpeedMax` is the highest observed speed series value.
  The axis ceiling is rounded up to the next multiple of the chosen step.
- **Right axis:** count. The renderer starts from
  `countAxisMax = maxCount * CountAxisScale`, where `CountAxisScale = 1.6`, and
  chooses `niceStep(countAxisMax, 4)`. Tick marks are then emitted at multiples
  of that step up to the largest multiple not exceeding the scaled ceiling.

The two axes are intentionally independent. The speed axis optimises for clean
headline values; the count axis optimises for readable order-of-magnitude
context.

#### Series

| Series     | Colour    | Marker   | Line style         | Notes                                      |
| ---------- | --------- | -------- | ------------------ | ------------------------------------------ |
| `p50`      | `#fbd92f` | triangle | solid              | Median                                     |
| `p85`      | `#f7b32b` | square   | solid              | Upper-typical speed                        |
| `p98`      | `#f25f5c` | circle   | solid              | Fastest 2 percent                          |
| `max`      | `#2d1e2f` | none     | dashed `1 3`       | Single-event outlier line                  |
| count bars | `#2d1e2f` | bar      | filled, alpha 0.25 | Volume context behind the percentile lines |

Legend order is fixed: `p50`, `p85`, `p98`, `Max`, then any auxiliary legend
items (`p98 overall`, `max overall`, `low sample`). Count bars are visible in
the plot but are not given a dedicated legend item.

#### Low-sample masking and gap segmentation

The renderer uses two thresholds from `ChartStyle`:

- `CountMissingThreshold = 5`: values below this are masked to `NaN` by
  `ApplyCountMask()`.
- `LowSampleThreshold = 50`: values from 5 up to 49 keep their count bars but
  gain a translucent orange background swatch.

Masked buckets break the percentile polylines into separate segments. Markers
are skipped at masked points.

The chart also detects real temporal coverage gaps with `detectTimeGaps()`.
That function marks any step larger than `1.5 * minStep` across the observed
series. Those gaps also break the polylines.

#### Consolidated vs expanded spacing

This branch supports two spacing modes:

- **Default / consolidated (`expanded_chart = false`)**: only observed buckets
  are drawn. Long coverage gaps are visually compressed, but a dashed divider is
  drawn where the gap begins.
- **Expanded (`expanded_chart = true`)**: `ExpandTimeSeriesGapsInRange()` inserts
  explicit zero-count, `NaN` placeholders from the requested start time through
  the requested end time. Missing periods then occupy real horizontal space in
  the chart.

This switch affects X-axis spacing, divider placement, and SVG tooltip text.

#### Gap dividers

There are two divider cases, both rendered as the same dashed grey vertical
stroke:

1. the first masked bucket in a `NaN` run, or
2. the midpoint between two observed buckets separated by a detected time gap

Day boundaries by themselves do **not** create a divider.

#### Reference lines

Two optional horizontal reference lines can be drawn behind the percentile
series:

- `p98=NN` at `data.P98Reference`, using `ColourP98` and dash `6 3`
- `max=NN` at `data.MaxReference`, using `ColourMax` and dash `1 3`

In the current report pipeline, `renderCharts()` populates those values for
both `timeseries.svg` and `timeseries_compare.svg` from the period-wide summary
metrics for the primary and comparison ranges respectively. That means both the
single-period and comparison report time-series charts now carry the aggregate
`p98 overall` and `max overall` guides whenever summary data exists.

Both labels are rendered by the local `labelWithBg()` helper. It draws a white
rectangle behind the text so the label can sit on the left speed axis without
colliding with ordinary tick labels.

Those same references appear in the legend as `p98 overall` and `max overall`
when present.

#### X axis

The current PDF renderer does **not** use the older span-aware
`pickTickCadence()` table. It now emits horizontal, one-line labels and then
culls overlaps.

Candidate ticks are generated as follows:

- if the plotted span is at least 24 hours, emit ticks at day boundaries with
  labels `Jan 02`
- otherwise, emit the first point, the first point of each new day, and every
  third bucket within a day with labels `Jan 02 15:04`

After that, the renderer estimates label width and keeps only ticks that are at
least one estimated label width apart.

This is the actual PDF behaviour today. If the cross-platform contract moves
back to span-aware cadence, update both this file and `docs/ui/DESIGN.md` in the
same PR.

#### Legend and empty state

The legend sits in a bordered box along the bottom of the chart. Empty datasets
still render a correctly sized chart canvas with a centred grey `No data`
message.

### 4.3 Histogram charts

#### Y axis is percentage, not raw count

Both histogram renderers use percentage of period total on the vertical axis.
That makes period shapes comparable even when the absolute sample count differs.

`pctNiceStep(maxPct)` wraps `niceStep(maxPct, 5)` with a minimum of 5.

#### Bucket labels

`BucketLabel(lo, hi, maxBucket)` returns:

- `20-25` for an interior bucket
- `70+` for the saturating bucket at and above `maxBucket`

All bucket labels rotate `-45deg`.

#### Single histogram

`RenderHistogram()` produces:

- one steel-blue bar per bucket (`ColourSteelBlue`, alpha 0.7)
- black X-axis tick marks below each bucket label
- Y tick labels with the percent sign included (`0%`, `10%`, ...)
- X-axis label `Speed (<units>)`

There is no legend and no internal chart title in the single histogram.

#### Comparison histogram

`RenderComparison()` produces:

- paired bars that touch inside each bucket (`groupGap = 0`)
- t1 in `ColourP50`, t2 in `ColourP98`
- an internal SVG title: `Velocity Distribution Comparison`
- numeric Y tick labels plus a rotated Y-axis title `Percentage (%)`
- X-axis label `Velocity (<units>)`
- an in-plot legend box near the top-left, with the full `t1: ...` and
  `t2: ...` date-range labels supplied by the report builder

This chart is still overview-column width, but the taller 0.70 aspect keeps the
rotated labels and legend from colliding.

#### Empty state

Like the time-series renderer, histogram renderers keep the fixed chart size and
fall back to a centred grey `No data` label when there is nothing to plot.

## 5. Tables

### 5.1 Table renderer stack

The current table system is two-layered:

1. `withStyledTable()` applies the shared visual treatment.
2. `renderReportTable()` chooses either `tabular` or `supertabular` from a
   small `reportTable` descriptor (`columns`, `rows`, `caption`, `pageBreak`).

The shared table-style wrapper is:

```text
{
  \AtkinsonMono\<fontSize>
  \renewcommand{\arraystretch}{1.00}
  \setlength{\tabcolsep}{2pt}
  \rowcolors{2}{black!2}{white}
  <body>
  \rowcolors{0}{}{}
  <afterReset>
}
```

The narrative key/value tables in `survey_parameters.tex` are the exception.
They are handwritten in the template because they have no header row and stay
within a single column.

### 5.2 Captions, colour, and page breaks

Current shared rules:

- alternating tint: `black!2`
- page-spanning tables: `supertabular`
- first-page header only: `renderReportTable()` currently uses
  `\tablefirsthead{...}` with an empty `\tablehead{}`; later pages do **not**
  repeat the header row
- caption helper: `tableCaptionTeX()` renders `\normalfont\bfseries\small`

The bold caption style is deliberate. It matches the current LaTeX preamble and
the checked-in TeX golden files.

### 5.3 Column layout

The branch-tip table system no longer uses vertical rules between columns.
Instead it uses fixed-width `p{...}` columns with explicit left/right ragging.
All reusable data tables trim outer padding with `@{}` at both edges.

Current width allocations:

| Table                  | Current column widths                          |
| ---------------------- | ---------------------------------------------- |
| single key metrics     | `0.55`, `0.42`                                 |
| comparison key metrics | `0.31`, `0.22`, `0.22`, `0.19`                 |
| stat table             | `0.24`, `0.12`, `0.14`, `0.14`, `0.14`, `0.14` |
| histogram table        | `0.35`, `0.29`, `0.32`                         |
| dual histogram table   | `0.15`, `0.14`, `0.14`, `0.14`, `0.14`, `0.21` |

Headers are bold sans. Numeric columns are ragged-left in TeX terms
(`\raggedleft`) so they line up visually against the right edge of their fixed
column.

### 5.4 Dense-value formatting details

Several formatting helpers exist purely to keep dense tables aligned:

- `FormatTime()` emits compact timestamps as `M/D HH:MM`, not ISO `YYYY-MM-DD`.
- `statStartTimeTeX()` pads one-digit month, day, and hour values with
  `\phantom{0}` so rows stay visually aligned in the stat table.
- histogram bucket labels pad one-digit starts with `\phantom{0}` for the same
  reason.
- the comparison key-metrics `Vehicle Count` row adds `\phantom{ <units>}` to
  both count cells so the counts align with the speed rows above them.

These helpers are not incidental formatting trivia; they are part of the
designed table density.

### 5.5 Current table builders and captions

| Builder                             | Output                                | Current caption                            |
| ----------------------------------- | ------------------------------------- | ------------------------------------------ |
| `BuildSingleKeyMetricsTableTeX`     | 2-column key-metrics table            | `Table 1: Key Metrics`                     |
| `BuildComparisonKeyMetricsTableTeX` | 4-column key-metrics comparison table | `Table 1: Key Metrics`                     |
| `BuildHistogramTableTeX`            | single-period histogram table         | `Table 2: Velocity Distribution (<units>)` |
| `BuildDualHistogramTableTeX`        | t1/t2 histogram comparison table      | `Table 2: Velocity Distribution (<units>)` |
| `BuildStatTableTeX`                 | granular or daily percentile table    | supplied by caller (`Table 3` / `Table 4`) |

### 5.6 Hardware and survey-parameter tables

The two template-owned key/value tables use a slightly different treatment from
the generic data-table stack:

- font size: `\small`
- `\arraystretch = 1.04`
- `\tabcolsep = 2pt`
- alternating rows start at row 1 with `\rowcolors{1}{black!2}{white}`
- key/value widths: `p{0.44\linewidth}` and `p{0.52\linewidth}`

Current hardware rows are:

- Radar Sensor
- Firmware version (optional)
- Transmit Frequency
- Sample Rate
- Velocity Resolution
- Azimuth Field of View
- Elevation Field of View

Current survey rows are:

- Units
- Minimum speed (cutoff)
- Roll-up Period
- Timezone
- Start time / End time (RFC3339 in single mode)
- Start time (t1/t2) and End time (t1/t2) in comparison mode
- cosine correction label, or cosine angle/factor rows when available

`Source` and `CompareSource` still influence the report data path, but they are
not currently rendered as visible rows in the PDF.

When any cosine-correction field is present, the section appends the italic note
`Note: speeds have been corrected to account for sensor angle.`

## 6. Comparison-mode design

### 6.1 t1 / t2 naming

The comparison report consistently refers to the two periods as t1 and t2 on
the surfaces that are actually rendered today:

- overview bullets: `Primary period (t1)` and `Comparison period (t2)`
- grouped histogram legend labels: `t1: <range>` and `t2: <range>`
- comparison time-series captions
- histogram-table column heads (`t1 Count`, `t1 %`, `t2 Count`, `t2 %`)
- footer period range (`<t1 range> vs <t2 range>`)

Survey parameters use t1/t2 only where they need a per-period distinction:
start/end times and cosine correction rows.

### 6.2 Visible comparison outputs

When a comparison period is present, the PDF can surface up to four comparison
artifacts:

1. grouped comparison histogram in the overview (when histograms are enabled)
2. daily percentile summary table
3. merged granular percentile breakdown table
4. second full-width time-series figure for t2

The comparison chart section is sequential, not overlaid: one figure for t1 and
one figure for t2.

### 6.3 Deltas

The rendered comparison key-metrics table currently shows **percentage deltas
only**.

`FormatDeltaPercent(primary, compare)` computes:

```text
(compare - primary) / primary * 100
```

That means positive values indicate t2 is above t1. `NaN`, `Inf`, or a zero
primary baseline render as `--`.

Absolute delta helpers still exist in template data (`DeltaP50`, `DeltaP85`,
and so on), but the current `.tex` templates do not surface them.

### 6.4 Comparison inputs that change the rendered result

Per-period differences that matter today:

- t2 can query a different source (`compare_source`), though the chosen source is
  not printed
- t2 can use a different cosine correction angle or multi-period cosine label
- t1 and t2 date ranges are shown everywhere the reader needs to distinguish the
  periods

## 7. Palette

Every colour used in the PDF chart engine resolves to a constant in
`internal/report/chart/palette.go`. The palette is deliberately small:

| Constant          | Hex       | Used in                                                |
| ----------------- | --------- | ------------------------------------------------------ |
| `ColourP50`       | `#fbd92f` | p50 series, comparison-bar t1 fill                     |
| `ColourP85`       | `#f7b32b` | p85 series, low-sample background swatch               |
| `ColourP98`       | `#f25f5c` | p98 series, p98 reference line, comparison-bar t2 fill |
| `ColourMax`       | `#2d1e2f` | max series, max reference line, count bars             |
| `ColourCountBar`  | `#2d1e2f` | alias used by count bars                               |
| `ColourLowSample` | `#f7b32b` | alias used by low-sample swatch                        |
| `ColourSteelBlue` | `#4682b4` | single-histogram bars                                  |

Cross-platform palette rationale lives in
[DESIGN.md §3.3](DESIGN.md#33-percentile-metric-colour-mapping-charts). If a
palette constant changes, update the web palette and any macOS visualiser use in
the same PR.

The LaTeX-side `vrP50`, `vrP85`, `vrP98`, and `vrMax` colour declarations in
`preamble.tex` are a parallel typesetter namespace. Keep them aligned with the
chart constants.

## 8. Renderer-portability cheat sheet

If XeLaTeX is replaced, the following responsibilities must move with it.

| Responsibility                         | Current xelatex implementation                                                      | What an alternative needs                                                                                                |
| -------------------------------------- | ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Narrative sans font + mono data font   | `\setsansfont` plus `\newfontfamily\AtkinsonMono`                                   | Native embedding of both Atkinson Hyperlegible and Atkinson Hyperlegible Mono                                            |
| Two-column flow + full-width breakouts | `\twocolumn[...]`, then `\onecolumn` for charts                                     | Equivalent multi-column layout with break-out figures                                                                    |
| Optional map final page                | `\clearpage` plus `\onecolumn`                                                      | Equivalent page break and full-width final figure                                                                        |
| Page-spanning tables                   | `supertabular` with `\tablefirsthead`, empty `\tablehead`, and `\tabletail{\hline}` | Equivalent multi-page table support, including the current first-page-only header behaviour unless deliberately improved |
| Fixed-width column layout              | explicit `p{...}` widths plus ragged left/right alignment                           | Per-column width control and ragged alignment                                                                            |
| Alternating row colours                | `colortbl` `\rowcolors{n}{a}{b}`                                                    | Same row-striping semantics                                                                                              |
| SVG embedding                          | `rsvg-convert` to PDF, then `\includegraphics{...}`                                 | Native SVG support or the same SVG-to-image bridge                                                                       |
| Running header/footer                  | `fancyhdr`                                                                          | Equivalent template-driven running heads and feet                                                                        |
| Escaping                               | `EscapeTeX()`                                                                       | Renderer-specific escaping for `& % $ # _ { } ~ ^ \`                                                                     |
| Hyperlinks                             | `hyperref` plus `\href{}{}`                                                         | Native link support for site URL, contact email, and science links                                                       |

The chart stage remains the easiest part to port because the SVG is already the
final chart specification.

A renderer migration needs to re-home:

1. the ten current templates: `report.tex`, `preamble.tex`, `period_report.tex`,
   `overview.tex`, `site_info.tex`, `science.tex`, `survey_parameters.tex`,
   `statistics.tex`, `chart_section.tex`, and `map_section.tex`
2. the reusable table stack in `tex/helpers.go`, especially `withStyledTable()`,
   `renderReportTable()`, `tableCaptionTeX()`, and the `Build*TableTeX`
   functions
3. the compact timestamp and padding helpers (`FormatTime`, `statStartTimeTeX`,
   `paddedDecimalTeX`, `paddedClockTeX`)
4. the same bundled font set from `assets.AllFonts()`

## 9. Cross-references

- Cross-platform UI design contract: [DESIGN.md](DESIGN.md)
- PDF reporting operations and runtime setup:
  [platform/operations/pdf-reporting.md](../platform/operations/pdf-reporting.md)
- Migration history (Python -> Go):
  [plans/pdf-go-chart-migration-plan.md](../plans/pdf-go-chart-migration-plan.md)
- Layout inspection tool for rendered PDFs:
  [../../scripts/compare_report_layout.py](../../scripts/compare_report_layout.py)

## 10. PR checklist (PDF report changes)

Before merging a PR that touches the PDF report:

- [ ] Palette changes propagated to the other renderers that consume the shared
      contract.
- [ ] New reusable data tables go through `renderReportTable()` and
      `withStyledTable()` unless they are intentionally template-owned key/value
      tables.
- [ ] Table striping remains `black!2`.
- [ ] The sans/mono font split remains intact and any new packaged font bytes are
      added through `assets.AllFonts()`.
- [ ] Comparison histogram sizing still reflects the taller `1:0.70` aspect.
- [ ] Time-series gap behaviour is still correct for both consolidated and
      expanded-chart modes.
- [ ] X-axis changes are reflected here and, if they affect the intended shared
      chart contract, also in [DESIGN.md](DESIGN.md).
- [ ] New survey-parameter rows fit the `0.44\linewidth` key column.
- [ ] If layout changed, inspect a rendered PDF directly and use
      `scripts/compare_report_layout.py` when a before/after diff is useful.
- [ ] Golden TeX files regenerated when the template output changed:
      `go test ./internal/report/tex/... -update`.
- [ ] `make lint-go && make test-go` green when the change touches report code.
