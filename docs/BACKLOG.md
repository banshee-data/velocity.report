# Backlog

Single source of truth for project-wide work items in velocity.report. Where available, tasks link to a design document using syntax `[#$pr] (#$issue) $title - $task [design doc]($url)`; tasks without a design doc use just the backlog entry and effort tag. Individual docs in `plans/` describe single projects, not priority lists.

## v0.5 (Platform Hardening)

- Python venv consolidation — Makefile uses root .venv/; remove stale tools/pdf-generator/.venv — [design doc](plans/tooling-python-venv-consolidation-plan.md) `S`
- Documentation standardisation — metadata and validation gates for all docs — [design doc](plans/platform-documentation-standardization-plan.md) `S`
- Platform simplification Phase 1 — deprecation signalling and deploy retirement gate — [design doc](plans/platform-simplification-and-deprecation-plan.md) `S`
- SWEEP/HINT platform hardening (Phase 5–6) — transform pipeline, objective registry, explainability — [design doc](plans/lidar-sweep-hint-mode-plan.md) `M`
- HINT sweep polish — 11 remaining polish items — [design doc](lidar/operations/hint-sweep-mode.md) `M`

## v0.6 (Deployment & Packaging)

- Precompiled LaTeX — faster PDF report generation via vendored TeX tree — [design doc](plans/pdf-latex-precompiled-format-plan.md) `M`
- Single `velocity-report` binary + subcommands — unified CLI with radar/lidar/pdf subcommands — [design doc](plans/deploy-distribution-packaging-plan.md) `L`
- One-line install script — curl-based installer with automatic platform detection — [design doc](plans/deploy-distribution-packaging-plan.md) `S`
- (#210) Raspberry Pi imager pipeline — custom flashing UX, depends on packaging — [design doc](plans/deploy-rpi-imager-fork-plan.md) `L`
- Geometry-coherent tracking (P1 maths, D-04) — spatial consistency in track association — [proposal](maths/proposals/20260222-geometry-coherent-tracking.md) `M`
- Simplification and deprecation programme (Project A/B) — deprecation signalling, deploy retirement gate, and migration plan task list — [design doc](plans/platform-simplification-and-deprecation-plan.md) `M`
- LiDAR foundations fix-it — documentation truth alignment, implementation boundary stabilisation — [design doc](plans/lidar-architecture-foundations-fixit-plan.md) `M`

## v0.7 (Unified Frontend)

- Frontend consolidation (Phases 0–5) — migrate status/regions/sweep to Svelte, retire port 8081 — [design doc](plans/web-frontend-consolidation-plan.md) `L`
- ECharts → LayerChart rewrite (8 charts, D-11) — migrate all radar/lidar charts to Svelte LayerChart — [design doc](ui/DESIGN.md) `L`
- Frontend decomposition (Svelte stores) — item 13: tracksStore, runsStore, missedRegionStore — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md) `M`
- Retire Go-embedded dashboards — ~2,000 lines removed from monitor once Svelte dashboards replace ECharts — [review doc §7](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md) `L`
- Platform simplification complete — all deprecated surfaces retired, migration complete — [design doc](plans/platform-simplification-and-deprecation-plan.md) `M`
- GitHub Releases CI pipeline — automated binary builds and release packaging — [design doc](plans/deploy-distribution-packaging-plan.md) `M`
- Track labelling Phase 9 UI (Swift, D-07) — seekable replay, Swift-native labelling — [design doc](plans/lidar-track-labeling-auto-aware-tuning-plan.md) `M`
- Transit deduplication (D-03) — duplicate transit record prevention — [design doc](radar/architecture/transit-deduplication.md) `M`
- SQLite client standardisation — unify DB interfaces across internal/db, internal/api, and internal/lidar/storage; remove API-layer SQL — [design doc](plans/data-sqlite-client-standardization-plan.md) `M`
- Accessibility testing — add axe-core/playwright asserting no critical violations on each route — [design doc §7.2](ui/design-review-and-improvement.md) `S`
- Widescreen content containment (D-13) — add vr-page max-width centring at ≥3000px — [design doc §2.2](ui/design-review-and-improvement.md) `S`
- Profile comparison system — cross-run evaluation UI, scene evaluation APIs — [design doc](plans/lidar-track-labeling-auto-aware-tuning-plan.md) `M`
- Metrics/stats/frontend consolidation follow-through (Project C/D) — retire duplicate stats surfaces, simplify CLI flags, and prune Make wrappers after parity — [design doc](plans/platform-simplification-and-deprecation-plan.md) `M`
- Cosine error correction remaining items — delete endpoint, report angle annotation, speed limit field migration — [design doc](radar/architecture/site-config-cosine-correction-spec.md) `M`
- macOS palette constants — prepare shared palette definition when metric charts added to visualiser — [design doc §1.2](ui/design-review-and-improvement.md)
- LayerChart policy in LiDAR routes — enforce chart rendering policy (no ad-hoc SVG) when charts added to tracks/scenes/runs/sweeps — [design doc §4.2](ui/design-review-and-improvement.md)

## v0.8 (??)

- Speed limit schedules (D-16) — time-based speed limits for school zones and weekday/weekend variation — [design doc](radar/architecture/speed-limit-schedules.md) `L`

## v1.0 (Production-Ready)

- Test coverage ≥ 95.5% — raise every internal/, web, Python, and macOS package to ≥ 95.5% line coverage — [design doc](plans/platform-quality-coverage-improvement-plan.md) `L`
- Velocity-coherent foreground extraction (P2, D-05) — 6D DBSCAN alternative for moving object detection — [proposal](maths/proposals/20260220-velocity-coherent-foreground-extraction.md) `L`
- Unified settling (L3/L4 SettlementCore, P4, D-05) — consolidate L3 background and L4 drift into single settlement core — [proposal](maths/proposals/20260219-unify-l3-l4-settling.md) `L`
- Time-partitioned raw data tables — major storage architecture change — [design doc](radar/architecture/time-partitioned-data-tables.md) `M`
- Geometry-prior local file format (GeoJSON) — local scene geometry configuration via GeoJSON — [design doc](lidar/architecture/vector-scene-map.md) `M`
- Data export (CSV, GeoJSON) — export vehicle transits and scene geometry for external analysis — design doc not yet written `M`
- Stable public API with versioned endpoints — formal API versioning and stability guarantees — design doc not yet written `M`

## v2.0 (Advanced Perception & Connected)

- Visualiser QC programme (Features 1–10) — comprehensive quality control tooling for LiDAR data — [design doc](plans/lidar-visualiser-labelling-qc-enhancements-overview-plan.md) `XL`
- ML classifier training pipeline (Phase 4.1) — automated model training and evaluation framework — [plan](plans/lidar-ml-classifier-training-plan.md) `L`
- Parameter tuning optimisation (Phase 4.2) — automated hyperparameter search and optimisation — [plan](plans/lidar-parameter-tuning-optimisation-plan.md) `L`
- Ground-plane vector-scene maths (P3, D-05) — 3D scene reconstruction with ground-plane constraints — [proposal](maths/proposals/20260221-ground-plane-vector-scene-maths.md) `L`
- Dynamic algorithm selection — runtime algorithm switching based on scene conditions — [design doc](plans/lidar-architecture-dynamic-algorithm-selection-plan.md) `M`
- Online geometry-prior service — opt-in community-maintained geometry priors (local-only remains default) — [design doc](lidar/architecture/vector-scene-map.md) `L`
- Multi-location aggregate dashboard — cross-site analytics and comparative reporting `L`
- Threshold-based speed alerts — configurable alerting for speed threshold violations `M`
- Peak-hour and seasonal trend analysis — temporal pattern detection and analysis `M`
- Visualiser performance and scene health metrics — timeline and VR log metrics — [design doc](plans/lidar-visualiser-performance-and-scene-health-timeline-metrics-plan.md) `M`
- Frontend background debug surfaces — Swift visualiser debugging outputs for background settlement — [design doc](plans/web-frontend-background-debug-surfaces-plan.md) `M`

## v∞.0 (Deferred, waybacklog)

- AV dataset integration — 28-class taxonomy, Parquet ingestion — [design doc](plans/lidar-av-lidar-integration-plan.md)
- Motion capture architecture — moving sensor support — [design doc](plans/lidar-motion-capture-architecture-plan.md)
- Static pose alignment — 7-DOF tracking — [design doc](plans/lidar-static-pose-alignment-plan.md)
- AV range image format — dual-return support — [design doc](lidar/architecture/av-range-image-format-alignment.md)
- Visual regression testing — Playwright baseline screenshots — [design doc](ui/design-review-and-improvement.md)
- E2E test infrastructure — Playwright smoke tests — [design doc](ui/design-review-and-improvement.md)
- (#9) LAN authentication — add auth layer if deployment moves beyond private LAN — [design doc §10.1](ui/design-review-and-improvement.md)
- Coverage thresholds — raise codecov thresholds to meaningful levels after coverage improves — [design doc §7.5](ui/design-review-and-improvement.md)
- ECharts palette cross-reference — document palette alignment requirement for Phase 3 frontend consolidation migration — [design doc §3.3](ui/design-review-and-improvement.md) `S`

## Complete

- [#280] 501 stub replacement (evaluation and reprocess endpoints) — review doc item 4 — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#280] Arena.go deprecation — [design doc](lidar/architecture/arena-go-deprecation-and-layered-type-layout-design-20260217.md)
- [#280] L1–L6 layer alignment and code migration — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#280] Route table conversion and HTTP method prefixes — review doc items 3 and 11 — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] background.go split — background_persistence.go, background_export.go, background_drift.go extracted — [design doc §6.2](ui/design-review-and-improvement.md)
- [#284] CompareRuns extraction — comparison logic moved to l6objects/comparison.go — [design doc §6.3](ui/design-review-and-improvement.md)
- [#284] Cross-layer placement fixes — background.go split, webserver.go split, CompareRuns extraction — review doc item 14 — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] LiDAR monitor file splits — echarts_handlers.go + export_handlers.go — [review doc §1–2](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] Storage compareParams extraction — analysis_run_compare.go — [review doc §6](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] Sweep file splits — hint_progress/notifications.go, auto_narrowing.go, sweep_params.go — [review doc §3–5](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] Visualiser frame codec extraction — frame_codec.go extracted from adapter.go — [review doc §8](lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md)
- [#284] webserver.go split — datasource_handlers.go + playback_handlers.go extracted — [design doc §6.1](ui/design-review-and-improvement.md)
- [#286] Chart empty-state placeholder — "No chart data available" shown when chartData is empty — [design doc §3.1](ui/design-review-and-improvement.md)
- [#286] DESIGN.md references — Design Language section added to CONTRIBUTING.md; link added to README.md — [design doc §8.1](ui/design-review-and-improvement.md)
- [#286] Shared CSS standard classes — vr-page, vr-toolbar, vr-stat-grid, vr-chart-card in app.css — [design doc §2.1](ui/design-review-and-improvement.md)
- [#286] Shared palette module — palette.ts exports PERCENTILE_COLOURS, LEGEND_ORDER with tests — [design doc §1.3](ui/design-review-and-improvement.md)
- [#286] Web palette compliance — palette.ts created with canonical DESIGN.md §3.3 values; colorMap/cRange removed — [design doc §1.1](ui/design-review-and-improvement.md)
- [#287] LiDAR logging stream split — explicit Opsf/Diagf/Tracef call sites replacing Debugf/classifier — [design doc](lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md)
- [#287] LiDAR logging stream split — ops/debug/trace streams with routing rubric — [design doc](lidar/architecture/lidar-logging-stream-split-and-rubric-design-20260217.md)
- [#291] PR template design checklist — add DESIGN.md §9 UI/chart checklist to .github/PULL_REQUEST_TEMPLATE.md — [design doc §8.2](ui/design-review-and-improvement.md)
- [#319] Settling optimisation Phase 3 — convergence/evaluation tooling — [design doc](lidar/operations/settling-time-optimization.md)
