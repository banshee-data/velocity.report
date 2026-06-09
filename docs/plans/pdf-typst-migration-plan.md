# PDF generation via Typst

- **Status:** Complete on this branch
- **Branch:** `patrickod/go-typst`
- **Layers:** Reporting, packaging, image build
- **Supersedes on this branch:** `pdf-typst-phase-0-results.md`,
  `pdf-typst-parity-todo.md`
- **Canonical runtime docs:**
  [pdf-reporting.md](../platform/operations/pdf-reporting.md),
  [PDF_REPORT_DESIGN.md](../ui/PDF_REPORT_DESIGN.md)

## Summary

This branch completed the report-engine cutover from the former LaTeX/XeLaTeX
path to a Go-orchestrated Typst pipeline.

The resulting product contract is:

- `report.GeneratePDF()` is the public entry point and now renders through
  `GenerateTypst()`.
- The HTTP API and CLI keep the same call shape while producing Typst-backed
  PDFs.
- Charts remain Go-generated SVGs.
- PDF composition is handled by Typst templates plus `data.json`.
- The source ZIP remains a first-class artefact, now as a recompilable Typst
  bundle instead of a LaTeX bundle.

This document is the single branch-local implementation record. Prototype notes,
parity punch lists, and the migration proposal are folded here so there is one
place to understand what was decided and what landed.

## Completed Checklist

- [x] Keep report generation in Go and remove the Python report runtime from the
      production path.
- [x] Replace the public PDF engine behind `report.GeneratePDF()` with the
      Typst-backed `GenerateTypst()` flow.
- [x] Keep `/api/generate_report` and `velocity report pdf` on the same public
      surface while swapping the internal typesetting engine.
- [x] Introduce a dedicated Typst renderer in
      [internal/report/typst/](/Users/david/code/velocity.report/internal/report/typst)
      with embedded templates, structured data marshalling, and test fixtures.
- [x] Implement Typst templates for the report layout in
      [report.typ](/Users/david/code/velocity.report/internal/report/typst/templates/report.typ),
      [preamble.typ](/Users/david/code/velocity.report/internal/report/typst/templates/preamble.typ),
      and
      [sections.typ](/Users/david/code/velocity.report/internal/report/typst/templates/sections.typ).
- [x] Render both single-period and comparison reports through the Typst path.
- [x] Keep charts as SVG and hand them directly to Typst with no
      SVG-to-PDF conversion pass.
- [x] Preserve the editable source ZIP contract, now shipping `report.typ`,
      `preamble.typ`, `sections.typ`, `data.json`, `charts/*.svg`, `fonts/`,
      and a ZIP-local `README.md`.
- [x] Ensure the ZIP no longer exposes `report.tex` as the active source
      contract.
- [x] Embed the Atkinson Hyperlegible report fonts into the runtime and source
      ZIP so report generation does not depend on host-installed fonts.
- [x] Add Typst binary resolution and embedding support under
      [internal/report/typst/typstbin/](/Users/david/code/velocity.report/internal/report/typst/typstbin).
- [x] Make embedded Typst the default deployment model via `typst_embed` build
      tags and the image/release build scripts.
- [x] Keep `PATH` and the development-only downloader as local-development
      fallbacks when Typst is not embedded.
- [x] Replace TeX-specific runtime/image dependencies in the Pi image flow with
      the Typst binary path.
- [x] Add PDF metadata post-processing in Go so generated PDFs carry
      velocity.report creator/keyword metadata even though Typst does not expose
      every required PDF field directly.
- [x] Add end-to-end and low-level tests covering Typst rendering, metadata
      stamping, binary resolution, and API-level report generation.
- [x] Add regression coverage for optional/missing histogram data so absent
      buckets omit the affected section instead of crashing Typst rendering.
- [x] Fold the earlier prototype results and parity follow-up notes into this
      single completed plan.

## Design Decisions

1. **Typst is the only active report engine on this branch.**
   The branch accepts a clean engine cutover rather than a long-lived
   dual-engine runtime. LaTeX is no longer the live path behind API or CLI
   report generation.

2. **Equivalent output is the acceptance target, not pixel identity with
   XeLaTeX.**
   The branch keeps the established report structure and information contract,
   but it does not preserve a permanent "parity window" document for
   pixel-for-pixel review. Layout follow-ups belong in the Typst templates and
   the canonical PDF design docs, not in a separate migration todo.

3. **Go owns data loading and chart rendering; Typst owns page composition.**
   Data assembly stays in `internal/report/`, charts stay in
   `internal/report/chart/`, and Typst consumes those artefacts as structured
   JSON plus SVG inputs.

4. **Typst is invoked as an external CLI, not as an in-process Go library.**
   The runtime uses a thin Go wrapper around the Typst executable. That keeps
   the integration small and matches the packaging model we can actually ship.

5. **Embedded Typst is the deployment default.**
   Release and image builds embed a pinned Typst binary into the `velocity`
   binary. Development may still use `PATH` or the downloader, but deployed
   systems should not depend on host package state.

6. **The source ZIP remains a product feature.**
   The branch deliberately keeps an editable, recompilable source bundle. The
   cutover changes the archive format from LaTeX to Typst instead of deleting
   source export.

7. **SVG is the canonical chart interchange format across web and PDF.**
   Go renders charts once as SVG; Typst embeds them directly. This removes the
   extra rasterisation/conversion surface from the report pipeline.

8. **PDF metadata is patched after Typst compile.**
   The branch sets creator/keyword metadata in Go after the compile step so the
   final PDF carries velocity.report build provenance consistently.

9. **Map data is an input artefact, not a runtime fetch in PDF generation.**
   The report consumes saved `site.map_svg_data` content. PDF generation itself
   does not fetch map data.

## What Landed On This Branch

The branch-level code and packaging diff shows these concrete outcomes:

- Typst runtime and template code under
  [internal/report/typst/](/Users/david/code/velocity.report/internal/report/typst)
- Typst report orchestration under
  [internal/report/typst_generate.go](/Users/david/code/velocity.report/internal/report/typst_generate.go)
- API report generation wired through the Typst path in
  [internal/api/server_reports_generate.go](/Users/david/code/velocity.report/internal/api/server_reports_generate.go)
- Typst binary embedding/download support in
  [Makefile](/Users/david/code/velocity.report/Makefile),
  [scripts/download-typst.sh](/Users/david/code/velocity.report/scripts/download-typst.sh),
  and
  [image/scripts/build-image.sh](/Users/david/code/velocity.report/image/scripts/build-image.sh)
- Image package changes in
  [image/stage-velocity/00-install-packages/00-packages](/Users/david/code/velocity.report/image/stage-velocity/00-install-packages/00-packages)
  and related stage scripts
- Updated operational and design docs in
  [pdf-reporting.md](../platform/operations/pdf-reporting.md),
  [DESIGN.md](../ui/DESIGN.md), and
  [PDF_REPORT_DESIGN.md](../ui/PDF_REPORT_DESIGN.md)

## Prototype And Parity Conclusions Folded In

The removed side-docs contributed these decisions, now captured here:

- The early Typst prototype proved the core structure: embedded templates,
  rooted includes, JSON-backed report data, Atkinson font loading, tables,
  columns, headers/footers, and a Go `Render` wrapper.
- The temporary prototype-only driver/CLI was removed after the main report
  pipeline graduated.
- The branch accepted native Typst layout iteration inside the main templates
  instead of maintaining a separate parity todo document forever.
- Remaining report-layout refinements are now ordinary template/design work, not
  migration-phase work.

## Relationship To Other PDF Docs

- [pdf-go-chart-migration-plan.md](pdf-go-chart-migration-plan.md) remains the
  record for moving report data loading and chart generation into Go.
- [pdf-latex-precompiled-format-plan.md](pdf-latex-precompiled-format-plan.md)
  remains historical context for the old TeX footprint problem.
- [pdf-reporting.md](../platform/operations/pdf-reporting.md) is the runtime and
  operator-facing description of the current pipeline.
- [PDF_REPORT_DESIGN.md](../ui/PDF_REPORT_DESIGN.md) is the visual/layout source
  of truth for the report output.
