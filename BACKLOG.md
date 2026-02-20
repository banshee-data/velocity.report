# Backlog

Prioritised list of upcoming work for velocity.report. Each task links to its own design document with syntax `[#$pr] (#$issue) $title - $task [design doc]($url)` . This is the single source of truth for project-wide work items — individual docs in `docs/plans/` describe single projects, not priority lists.

## P1 — Next

- PR template design checklist — add DESIGN.md §9 UI/chart checklist to .github/PULL_REQUEST_TEMPLATE.md — [design doc §8.2](docs/plans/design-review-and-improvement-plan.md)
- Sweep/HINT platform hardening — transform pipeline, objective registry, explainability — [design doc](docs/plans/industry-standard-ml-solver-expansion-plan.md)
- Settling optimisation Phase 3 — convergence/evaluation tooling — [design doc](docs/lidar/operations/settling-time-optimization.md)
- Profile comparison system — cross-run evaluation UI, scene evaluation APIs — [design doc](docs/lidar/future/track-labeling-auto-aware-tuning.md)
- (#252) Frontend consolidation Phases 1–3 — migrate status/regions/sweep to Svelte — [design doc](docs/plans/frontend-consolidation.md)
- Frontend decomposition (Svelte stores) — item 13: tracksStore, runsStore, missedRegionStore — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)

## P2 — Later

- Distribution and packaging — Debian packaging, update mechanism — [design doc](docs/plans/distribution-packaging-plan.md)
- (#210) Raspberry Pi imager pipeline — custom flashing UX, depends on packaging — [design doc](docs/plans/rpi-imager-fork-design.md)
- Time-partitioned raw data tables — major storage architecture change — [design doc](docs/features/time-partitioned-data-tables.md)
- Visualiser QC programme — features 1/2/3/5/7/8/10 — [design doc](docs/lidar/visualiser/06-labelling-qc-enhancements-overview.md)
- Transit deduplication — duplicate transit record prevention — [design doc](docs/plans/transit-deduplication-plan.md)
- Track labelling UI enhancements — seekable replay, Swift-native labelling — [design doc](docs/plans/track-labeling-ui-plan.md)
- HINT sweep polish — 11 remaining polish items — [design doc](docs/plans/hint-sweep-mode.md)
- Precompiled LaTeX — faster PDF report generation — [design doc](docs/plans/precompiled-latex-plan.md)
- Python venv consolidation — Makefile uses root .venv/; remove stale tools/pdf-generator/.venv — [design doc](docs/plans/python-venv-consolidation-plan.md)
- Accessibility testing — add axe-core/playwright asserting no critical violations on each route — [design doc §7.2](docs/plans/design-review-and-improvement-plan.md)
- Widescreen content containment — add vr-page max-width centering at ≥3000px — [design doc §2.2](docs/plans/design-review-and-improvement-plan.md)
- ECharts palette cross-reference — document palette alignment requirement for Phase 3 frontend consolidation migration — [design doc §3.3](docs/plans/design-review-and-improvement-plan.md)

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
- macOS palette constants — prepare shared palette definition when metric charts added to visualiser — [design doc §1.2](docs/plans/design-review-and-improvement-plan.md)
- LayerChart policy in LiDAR routes — enforce chart rendering policy (no ad-hoc SVG) when charts added to tracks/scenes/runs/sweeps — [design doc §4.2](docs/plans/design-review-and-improvement-plan.md)
- (#9) LAN authentication — add auth layer if deployment moves beyond private LAN — [design doc §10.1](docs/plans/design-review-and-improvement-plan.md)
- Coverage thresholds — raise codecov thresholds to meaningful levels after coverage improves — [design doc §7.5](docs/plans/design-review-and-improvement-plan.md)

## Complete

- [#280] L1–L6 layer alignment and code migration — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#280] Route table conversion and HTTP method prefixes — review doc items 3 and 11 — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#280] 501 stub replacement (evaluation and reprocess endpoints) — review doc item 4 — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#280] Arena.go deprecation — [design doc](docs/lidar/architecture/arena-go-deprecation-and-layered-type-layout-design-20260217.md)
- [#284] Cross-layer placement fixes — background.go split, webserver.go split, CompareRuns extraction — review doc item 14 — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] LiDAR monitor file splits — echarts_handlers.go + export_handlers.go — [review doc §1–2](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] Sweep file splits — hint_progress/notifications.go, auto_narrowing.go, sweep_params.go — [review doc §3–5](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] Storage compareParams extraction — analysis_run_compare.go — [review doc §6](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] Visualiser frame codec extraction — frame_codec.go extracted from adapter.go — [review doc §8](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] webserver.go split — datasource_handlers.go + playback_handlers.go extracted — [design doc §6.1](docs/plans/design-review-and-improvement-plan.md)
- [#284] background.go split — background_persistence.go, background_export.go, background_drift.go extracted — [design doc §6.2](docs/plans/design-review-and-improvement-plan.md)
- [#284] CompareRuns extraction — comparison logic moved to l6objects/comparison.go — [design doc §6.3](docs/plans/design-review-and-improvement-plan.md)
- [#286] Web palette compliance — palette.ts created with canonical DESIGN.md §3.3 values; colorMap/cRange removed — [design doc §1.1](docs/plans/design-review-and-improvement-plan.md)
- [#286] Chart empty-state placeholder — "No chart data available" shown when chartData is empty — [design doc §3.1](docs/plans/design-review-and-improvement-plan.md)
- [#286] DESIGN.md references — Design Language section added to CONTRIBUTING.md; link added to README.md — [design doc §8.1](docs/plans/design-review-and-improvement-plan.md)
- [#286] Shared palette module — palette.ts exports PERCENTILE_COLOURS, LEGEND_ORDER with tests — [design doc §1.3](docs/plans/design-review-and-improvement-plan.md)
- [#286] Shared CSS standard classes — vr-page, vr-toolbar, vr-stat-grid, vr-chart-card in app.css — [design doc §2.1](docs/plans/design-review-and-improvement-plan.md)
- LiDAR logging stream split — explicit Opsf/Diagf/Tracef call sites replacing Debugf/classifier — [design doc](docs/lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md)
