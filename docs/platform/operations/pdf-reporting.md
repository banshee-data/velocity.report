# PDF reporting: Go + Typst pipeline

Current plan lineage:

- [pdf-go-chart-migration-plan.md](../../plans/pdf-go-chart-migration-plan.md)
  moved report data loading and chart generation into Go.
- [pdf-typst-migration-plan.md](../../plans/pdf-typst-migration-plan.md)
  replaced the old typesetting layer with Typst.

PDF report generation now runs as a Go-orchestrated Typst pipeline. The system
queries SQLite directly, renders charts as SVG, materialises Typst templates
plus JSON data into a temporary work directory, and invokes `typst compile` to
produce the final PDF.

## Problem solved

The historical report stack had two separate migration steps:

1. Remove the Python PDF generator and keep report generation inside the Go
   server.
2. Remove the former external typesetting layer and its SVG-to-PDF conversion step.

The current Typst path completes both goals: no Python runtime, no external
typesetting tree, no SVG-to-PDF converter, and no separate report service.

## Current pipeline

```
Web UI → POST /api/generate_report (or CLI: velocity report pdf)
  → Go: internal/report/typst_generate.go → direct DB query
  → Go: internal/report/chart/*.go → SVG charts
  → Go: internal/report/typst/*.go → data.json + fonts + templates
  → Go: os/exec → typst compile → .pdf
  → Go: packageTypstOutput() → .zip archive (.typ + SVG + fonts + PDF)
```

Generated report artefacts are stored under `VELOCITY_REPORT_OUTPUT_DIR` when
set. Deployed images default to `/var/lib/velocity-report/reports`; local
development defaults to `.tmp/reports` at the repository root.

## Key packages

```
internal/report/
├── typst_generate.go        # GeneratePDF()/GenerateTypst() orchestration
├── typst_generate_test.go   # End-to-end tests for Typst output + archive
├── chart/                   # SVG chart + site-map renderers
│   ├── timeseries.go
│   ├── histogram.go
│   ├── sitemap.go
│   ├── osmtiles.go
│   └── palette.go
├── typst/
│   ├── data.go              # ReportData JSON contract
│   ├── render.go            # typst compile wrapper
│   ├── templates/           # Embedded `.typ` templates
│   ├── testdata/            # Sample fixture
│   └── typstbin/            # Typst binary resolution / embedding
└── archive.go               # Shared packaging helpers
```

## Report artefacts

The pipeline produces:

- A PDF report compiled by Typst.
- A ZIP archive containing the recompilable Typst source bundle:
  `report.typ`, `preamble.typ`, `sections.typ`, `data.json`, `charts/*.svg`,
  and bundled report fonts.

That archive is intended for traceability and reproducibility: a reviewer can
re-run `typst compile --font-path fonts report.typ` on the extracted bundle and
obtain the same report content without the server present.

## Runtime dependencies

The only external report compiler dependency is `typst`.

- Distributed builds embed the Typst binary into the `velocity` executable.
- Development builds can resolve Typst via `VELOCITY_TYPST_PATH` or `PATH`.
- A development-only downloader can fetch the pinned Typst version into a
  per-user cache when neither embedded nor local binaries are available.

The Atkinson Hyperlegible font family is embedded and materialised at render
time so report generation does not depend on host-installed fonts.

## Charts and figures

Implemented chart surfaces:

1. **Time-series chart** — SVG with dual axes, low-sample shading, and
   percentile series.
2. **Histogram** — single-period SVG distribution chart.
3. **Comparison histogram** — grouped SVG comparison chart.
4. **Site map** — saved vector SVG from `site.map_svg_data`, generated in the
   web editor only after explicit external map-request confirmation.

Typst consumes these SVG artefacts directly via `#image()`, so no SVG-to-PDF
conversion pass is needed.

## Relationship to earlier plans

- **Python PDF generator**: removed. The old Python stack is historical only.
- **Precompiled external typesetter bundles**: superseded for the report path
  by Typst. Those plans remain historical context rather than the active
  runtime.
- **Single-binary deployment**: strengthened. The report engine can now ship
  inside the `velocity` binary with no separate typesetting tree.

## Operational notes

- `report.GeneratePDF()` is the shared entry point used by the CLI and HTTP API.
- Report generation remains local-first: charts, data loading, and final PDF
  compilation all happen on-device or on the development host.
- Report generation never downloads map data. The web editor can optionally
  request OpenStreetMap tiles after explicit per-session confirmation and saves
  that tile snapshot into `site.map_svg_data`; users who do not want the app to
  make external map requests should use the SVG upload path.
- The source ZIP is part of the public contract of the report pipeline; treat
  it as a first-class artefact, not a debug afterthought.
