# PDF report design (canonical)

## 0. Status

This is the source of truth for visual, layout, and chart-rendering decisions in
the generated PDF report surface. Companion design guidance lives in
[DESIGN.md](DESIGN.md). Operational details live in
[platform/operations/pdf-reporting.md](../platform/operations/pdf-reporting.md).

The current implementation is Go + Typst only:

```text
DB query
  -> Go report data assembly
  -> Go SVG charts
  -> Typst templates + data.json + fonts
  -> typst compile
  -> report.pdf + source.zip
```

## 1. Pipeline Contract

The report pipeline has three independent stages:

| Stage           | Implementation           | Inputs                                          | Outputs                        |
| --------------- | ------------------------ | ----------------------------------------------- | ------------------------------ |
| Data assembly   | `internal/report/`       | `report.Config`, SQLite data                    | Typst-ready report data        |
| Chart render    | `internal/report/chart/` | `TimeSeriesData`, `HistogramData`, chart styles | SVG chart sources              |
| PDF composition | `internal/report/typst/` | `.typ` templates, `data.json`, SVGs, fonts      | final `.pdf` and source `.zip` |

The source ZIP is part of the product surface. It contains the recompilable
Typst bundle: `report.typ`, shared template files, `data.json`, chart SVGs, and
the embedded Atkinson font files.

## 2. Inputs That Change Output

| Input             | Behaviour                                                                                                                         |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `paper_size`      | Defaults to US Letter. `a4` is an explicit option and controls page geometry plus chart physical dimensions.                      |
| `expanded_chart`  | Default `false` keeps sparse time-series charts consolidated. `true` inserts explicit missing buckets across the requested range. |
| `histogram`       | Enables the overview histogram and distribution tables.                                                                           |
| `include_map`     | Enables the final map figure when map SVG bytes are present.                                                                      |
| comparison period | Adds grouped comparison histogram, comparison time-series figure, and comparison tables.                                          |
| `compare_source`  | Selects the dataset queried for comparison period data.                                                                           |

## 3. Layout

The report keeps the established visual contract:

1. Title block: site name, optional surveyor/contact line, and footer date
   range. Contact text becomes a link when an email is present.
2. Velocity Overview: site, period, total count, key metrics, and optional
   histogram.
3. Survey Parameters: survey metadata and sensor configuration.
4. Distribution and detail tables.
5. Full-width media section: time-series chart, optional comparison chart, and
   optional map.
6. Science footer: local-first measurement note and project links.

The body uses a balanced two-column composition for narrative and tables, with
natural full-width media blocks after the table section. Long tables may break
across pages, but headings should stay attached to the first table or chart they
introduce.

## 4. Charts

Charts are SVG-first. PDF composition consumes the same SVG semantics that the
web design system uses, with physical page dimensions supplied by
`paperTextWidthMM()` in `internal/report/chart/config.go`.

Canonical chart colours:

| Purpose          | Colour    |
| ---------------- | --------- |
| P50              | `#4a9eff` |
| P85              | `#ff6b35` |
| P98              | `#d63447` |
| Max speed        | `#1a1a1a` |
| Count bars       | `#6c757d` |
| Low-sample shade | `#f7b32b` |
| Histogram bars   | `#4682b4` |

Cross-platform palette rationale lives in
[DESIGN.md §3.3](DESIGN.md#33-percentile-metric-colour-mapping-charts). If a
palette constant changes, update the web palette and any macOS visualiser use in
the same PR.

## 5. Tables

Tables must favour scanability over density:

- Right-align numeric columns.
- Keep speed, count, and percentile columns in stable positions across single
  and comparison reports.
- Use alternating row backgrounds only where they materially improve long-table
  reading.
- Do not force page breaks simply to keep every detail table on a fresh page.

## 6. Typography

Reports use the embedded Atkinson Hyperlegible family for readable narrative and
data presentation. The font bundle is materialised into the source ZIP so a
reviewer can rebuild the report without host-installed fonts.

## 7. Change Checklist

- Render at least one single-period and one comparison report.
- Inspect the PDF directly when layout, table, chart, or typography code
  changes.
- Confirm the source ZIP contains the Typst sources, `data.json`, chart SVGs,
  fonts, and the generated PDF.
- Run `make lint-go && make test-go` when the change touches report code.
