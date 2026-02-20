# Backlog

Prioritised list of upcoming work for velocity.report. Each task links to its own design document. This is the single source of truth for project-wide work items — individual docs in `docs/plans/` describe single projects, not priority lists.

Last updated: 2026-02-19

## P1 — Next

| #   | Task                                                       | Design doc                                                                                                                                                           | Notes                                                  |
| --- | ---------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| 1   | Sweep/HINT platform hardening                              | [docs/plans/industry-standard-ml-solver-expansion-plan.md](docs/plans/industry-standard-ml-solver-expansion-plan.md)                                                 | Transform pipeline, objective registry, explainability |
| 2   | Settling optimisation Phase 3                              | [docs/lidar/operations/settling-time-optimization.md](docs/lidar/operations/settling-time-optimization.md)                                                           | Convergence/evaluation tooling                         |
| 3   | Profile comparison system                                  | [docs/lidar/future/track-labeling-auto-aware-tuning.md](docs/lidar/future/track-labeling-auto-aware-tuning.md)                                                       | Cross-run evaluation UI, scene evaluation APIs         |
| 4   | Frontend consolidation Phases 1–3                          | [docs/plans/frontend-consolidation.md](docs/plans/frontend-consolidation.md)                                                                                         | Migrate status/regions/sweep to Svelte                 |
| 5   | LiDAR logging stream split                                 | [docs/lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md](docs/lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md) | ops/debug/trace streams with routing rubric            |
| 6   | Design review fixes (palette, CSS DRY, chart empty states) | [docs/plans/design-review-and-improvement-plan.md](docs/plans/design-review-and-improvement-plan.md)                                                                 | Critical and high severity items from DESIGN.md audit  |
| 7   | Frontend decomposition (Svelte stores)                     | [docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)               | Item 13: tracksStore, runsStore, missedRegionStore     |

## P2 — Later

| #   | Task                             | Design doc                                                                                                                       | Notes                                              |
| --- | -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| 8   | Distribution and packaging       | [docs/plans/distribution-packaging-plan.md](docs/plans/distribution-packaging-plan.md)                                           | Debian packaging, update mechanism                 |
| 9   | Raspberry Pi imager pipeline     | [docs/plans/rpi-imager-fork-design.md](docs/plans/rpi-imager-fork-design.md)                                                     | Custom flashing UX, depends on packaging           |
| 10  | Time-partitioned raw data tables | [docs/features/time-partitioned-data-tables.md](docs/features/time-partitioned-data-tables.md)                                   | Major storage architecture change                  |
| 11  | Visualiser QC programme          | [docs/lidar/visualiser/06-labelling-qc-enhancements-overview.md](docs/lidar/visualiser/06-labelling-qc-enhancements-overview.md) | Features 1/2/3/5/7/8/10                            |
| 12  | Transit deduplication            | [docs/plans/transit-deduplication-plan.md](docs/plans/transit-deduplication-plan.md)                                             | Duplicate transit record prevention                |
| 13  | Track labelling UI enhancements  | [docs/plans/track-labeling-ui-plan.md](docs/plans/track-labeling-ui-plan.md)                                                     | Seekable replay, Swift-native labelling            |
| 14  | HINT sweep polish                | [docs/plans/hint-sweep-mode.md](docs/plans/hint-sweep-mode.md)                                                                   | 11 remaining polish items                          |
| 15  | Precompiled LaTeX                | [docs/plans/precompiled-latex-plan.md](docs/plans/precompiled-latex-plan.md)                                                     | Faster PDF report generation                       |
| 16  | Python venv consolidation        | [docs/plans/python-venv-consolidation-plan.md](docs/plans/python-venv-consolidation-plan.md)                                     | Unify to single .venv/ at root                     |
| 17  | ~~LiDAR monitor file splits~~    | [review doc §Further Opportunities 1–2](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)               | ✅ Done — echarts_handlers.go + export_handlers.go |
| 18  | ~~Sweep file splits~~            | [review doc §Further Opportunities 3–5](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)               | ✅ Done — hint/auto/runner splits                  |

## P3 — Deferred / Research

| #   | Task                            | Design doc                                                                                                                   | Notes                                                                    |
| --- | ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| 19  | AV dataset integration          | [docs/lidar/future/av-lidar-integration-plan.md](docs/lidar/future/av-lidar-integration-plan.md)                             | 28-class taxonomy, Parquet ingestion                                     |
| 20  | Motion capture architecture     | [docs/lidar/future/motion-capture-architecture.md](docs/lidar/future/motion-capture-architecture.md)                         | Moving sensor support                                                    |
| 21  | Static pose alignment           | [docs/lidar/future/static-pose-alignment-plan.md](docs/lidar/future/static-pose-alignment-plan.md)                           | 7-DOF tracking                                                           |
| 22  | AV range image format           | [docs/lidar/architecture/av-range-image-format-alignment.md](docs/lidar/architecture/av-range-image-format-alignment.md)     | Dual-return support                                                      |
| 23  | Dynamic algorithm selection     | [docs/lidar/architecture/dynamic-algorithm-selection.md](docs/lidar/architecture/dynamic-algorithm-selection.md)             | Runtime algorithm switching                                              |
| 24  | Velocity-coherent extraction    | [docs/lidar/future/velocity-coherent-foreground-extraction.md](docs/lidar/future/velocity-coherent-foreground-extraction.md) | 6D DBSCAN alternative                                                    |
| 25  | Visual regression testing       | [docs/plans/design-review-and-improvement-plan.md](docs/plans/design-review-and-improvement-plan.md)                         | Playwright baseline screenshots                                          |
| 26  | E2E test infrastructure         | [docs/plans/design-review-and-improvement-plan.md](docs/plans/design-review-and-improvement-plan.md)                         | Playwright smoke tests                                                   |
| 27  | Retire Go-embedded dashboards   | [review doc §Further Opportunity 7](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)               | ~2,000 lines removed from monitor once Svelte dashboards replace ECharts |
| 28  | ~~Visualiser codec extraction~~ | [review doc §Further Opportunity 8](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)               | ✅ Done — frame_codec.go extracted from adapter.go                       |

## Complete

1. L1–L6 layer alignment and code migration — [review doc](docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
2. Route table conversion and HTTP method prefixes — same review doc, items 3 and 11
3. 501 stub replacement (evaluation and reprocess endpoints) — same review doc, item 4
4. Arena.go deprecation — [design doc](docs/lidar/architecture/arena-go-deprecation-and-layered-type-layout-design-20260217.md)
5. Documentation consistency sweep — reconciled status vs checklists across docs/
6. Cross-layer placement fixes (background.go split, webserver.go split, CompareRuns extraction) — review doc, item 14
7. LiDAR monitor file splits — review doc §Further Opportunities 1–2 (echarts_handlers.go, export_handlers.go)
8. Sweep file splits — review doc §Further Opportunities 3–5 (hint_progress/notifications.go, auto_narrowing.go, sweep_params.go)
9. Storage compareParams extraction — review doc §Further Opportunity 6 (analysis_run_compare.go)
10. Visualiser frame codec extraction — review doc §Further Opportunity 8 (frame_codec.go)
