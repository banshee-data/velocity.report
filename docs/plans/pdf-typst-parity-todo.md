# Typst prototype vs reference PDF — remaining differences

Reference: <https://banshee-data.com/velocity.reports/2026-01-19_velocity.report_Clarendon-Avenue-San-Francisco.pdf>
Current output: `build/report/typst-prototype.pdf`

## Page 1 (two-column body)

- [ ] **Key Metrics column too narrow** — values like "30.54 mph" wrap to two
      lines ("30.54" / "mph"). Reference renders them on one line. Solutions:
      drop the unit suffix from cell values and put `(mph)` in the column
      header (matches Tables 3/4 pattern), or widen the value columns with
      explicit `fr` ratios.
- [ ] **Histogram caption uses Figure 1** — already correct, but caption is
      not bold; reference uses bold "Figure 1: ...".

## Page 2 (two-column continuation)

- [ ] **Table 2 header cell line-break alignment** — "Bucket / (mph)" stacks
      correctly, but the t1/t2 header cells appear visually merged in the
      raster preview. Verify in actual PDF viewer; pdftotext confirms
      separate cells, so this may be a Read-tool image artefact.
- [ ] **Tables 3 & 4 column 1/2 visual collapse** — pdftotext shows correct
      cell separation; image preview shows date/count overlapping. Likely a
      Read-tool rasterisation artefact, not a real issue. **Verify in a real
      PDF viewer.**
- [ ] **Table 3 (Daily Summary) is not paginating sensibly** — when t1+t2
      merge, all rows appear in column 2 of page 2 with Table 4 spilling
      across page break. Reference keeps each table together.

## Page 3 (table continuation)

- [ ] Currently shows only Table 4 continuation. Reference fits everything
      on pages 1–2 because LaTeX has tighter table inset.

## Page 4 (time-series figures)

- [ ] **X-axis tick labels too small / overlap** — date row "6/2 6/3 6/4"
      and time row mix; some labels show "98:88" / "98:99" font glitches in
      raster. Likely Atkinson Hyperlegible Mono digit shapes; verify on real
      PDF viewer.
- [ ] **Bold figure captions** — reference uses `\textbf{Figure N: ...}`.
      Current captions are regular weight.
- [ ] **Y-axis label "Velocity (mph)"** — the label shows but is quite
      small; reference uses larger axis labels.

## Page 5 (site map)

- [ ] **Schematic placeholder, not real OSM map** — reference shows actual
      satellite/street tiles. Phase 6 work; intentionally deferred.

## Header & footer

- [x] Header & footer now appear on every page (fixed by re-passing
      `header` and `footer` in subsequent `set page` calls in `report.typ`).
- [ ] **Header logo / wordmark** — reference uses bold "velocity.report"
      with the leading slash; current matches.
- [ ] **Title page header/footer** — reference suppresses header/footer on
      page 1. Current shows them. Probably keep as-is (cleaner).

## Charts

- [ ] **Histogram bar colours** — match reference (yellow t1, red t2). ✅
- [ ] **Histogram percent values** — show correct "28.0%" padding. ✅
- [ ] **Time-series count-bar opacity** — reference bars are very translucent
      grey; current bars look opaque. Bump alpha down further.
- [ ] **Time-series day-break dashed verticals** — present but very faint.

## Tables

- [ ] **Table 1 (Key Metrics) "Vehicle Count" row** — Δ column intentionally
      blank to avoid misleading comparison across unequal periods. ✅ matches
      reference.
- [ ] **Tables 3/4 merged t1+t2 with single header** — current matches
      reference. ✅

## Tooling / process

- [ ] PDF preview via Read tool produces RASTER artefacts where cells appear
      merged. To validate parity, view the PDF in a real viewer (Evince,
      Preview, browser) — pdftotext output confirms cell structure is
      correct.
