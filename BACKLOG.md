# Backlog

Prioritised list of upcoming work for velocity.report. Each task links to its own design document. This is the single source of truth for project-wide work items — individual docs in `docs/plans/` describe single projects, not priority lists.

Last updated: 2026-02-19

## P1 — Next

- Sweep/HINT platform hardening — transform pipeline, objective registry, explainability — [design doc](docs/plans/industry-standard-ml-solver-expansion-plan.md)
- Settling optimisation Phase 3 — convergence/evaluation tooling — [design doc](docs/lidar/operations/settling-time-optimization.md)
- Profile comparison system — cross-run evaluation UI, scene evaluation APIs — [design doc](docs/lidar/future/track-labeling-auto-aware-tuning.md)
- Frontend consolidation Phases 1–3 — migrate status/regions/sweep to Svelte — [design doc](docs/plans/frontend-consolidation.md)
- LiDAR logging stream split — ops/debug/trace streams with routing rubric — [design doc](docs/lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md)
- Design review fixes (palette, CSS DRY, chart empty states) — critical and high severity items from DESIGN.md audit — [design doc](docs/plans/design-review-and-improvement-plan.md)
- Frontend decomposition (Svelte stores) — item 13: tracksStore, runsStore, missedRegionStore — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)

## P2 — Later

- Distribution and packaging — Debian packaging, update mechanism — [design doc](docs/plans/distribution-packaging-plan.md)
- Raspberry Pi imager pipeline — custom flashing UX, depends on packaging — [design doc](docs/plans/rpi-imager-fork-design.md)
- Time-partitioned raw data tables — major storage architecture change — [design doc](docs/features/time-partitioned-data-tables.md)
- Visualiser QC programme — features 1/2/3/5/7/8/10 — [design doc](docs/lidar/visualiser/06-labelling-qc-enhancements-overview.md)
- Transit deduplication — duplicate transit record prevention — [design doc](docs/plans/transit-deduplication-plan.md)
- Track labelling UI enhancements — seekable replay, Swift-native labelling — [design doc](docs/plans/track-labeling-ui-plan.md)
- HINT sweep polish — 11 remaining polish items — [design doc](docs/plans/hint-sweep-mode.md)
- Precompiled LaTeX — faster PDF report generation — [design doc](docs/plans/precompiled-latex-plan.md)
- Python venv consolidation — unify to single .venv/ at root — [design doc](docs/plans/python-venv-consolidation-plan.md)

## P3 — Deferred / Research

- AV dataset integration — 28-class taxonomy, Parquet ingestion — [design doc](docs/lidar/future/av-lidar-integration-plan.md)
- Motion capture architecture — moving sensor support — [design doc](docs/lidar/future/motion-capture-architecture.md)
- Static pose alignment — 7-DOF tracking — [design doc](docs/lidar/future/static-pose-alignment-plan.md)
- AV range image format — dual-return support — [design doc](docs/lidar/architecture/av-range-image-format-alignment.md)
- Dynamic algorithm selection — runtime algorithm switching — [design doc](docs/lidar/architecture/dynamic-algorithm-selection.md)
- Velocity-coherent extraction — 6D DBSCAN alternative — [design doc](docs/lidar/future/velocity-coherent-foreground-extraction.md)
- Visual regression testing — Playwright baseline screenshots — [design doc](docs/plans/design-review-and-improvement-plan.md)
- E2E test infrastructure — Playwright smoke tests — [design doc](docs/plans/design-review-and-improvement-plan.md)
- Retire Go-embedded dashboards — ~2,000 lines removed from monitor once Svelte dashboards replace ECharts — [review doc §7](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)

## Complete

- L1–L6 layer alignment and code migration — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- Route table conversion and HTTP method prefixes — review doc items 3 and 11 — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- 501 stub replacement (evaluation and reprocess endpoints) — review doc item 4 — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- Arena.go deprecation — [design doc](docs/lidar/architecture/arena-go-deprecation-and-layered-type-layout-design-20260217.md)
- Documentation consistency sweep — reconciled status vs checklists across docs/
- Cross-layer placement fixes — background.go split, webserver.go split, CompareRuns extraction — review doc item 14 — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- LiDAR monitor file splits — echarts_handlers.go + export_handlers.go — [review doc §1–2](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- Sweep file splits — hint_progress/notifications.go, auto_narrowing.go, sweep_params.go — [review doc §3–5](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- Storage compareParams extraction — analysis_run_compare.go — [review doc §6](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- Visualiser frame codec extraction — frame_codec.go extracted from adapter.go — [review doc §8](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
