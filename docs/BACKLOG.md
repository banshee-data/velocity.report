# Backlog

Single source of truth for project-wide work items in velocity.report. Where available, tasks link to a design document using syntax `[#$pr] (#$issue) $title - $task [design doc]($url)`; tasks without a design doc use just the backlog entry and effort tag. Individual docs in `plans/` describe single projects, not priority lists.

- **Governance:** Never delete agreed backlog items — split, consolidate, or complete them. Outstanding agreed work stays tracked here until delivered. When consolidating overlapping items, create distinct non-overlapping work units and move completed sub-tasks to the Complete section. Design documents may retire scope by marking phases complete or out-of-scope and linking to the PR where the scope change landed.
- **Formatting:** Backlog items describe outstanding work only. When sub-tasks complete, move them to the Complete section and simplify the parent item to show only what remains. Do not use strikethrough to track done sub-tasks inline — the Complete section is the record of delivered work.

## v0.5.0 (Platform Hardening)

- v0.5.0 backward compatibility shim removal — report download URL migration, server-side sweep legacy request/result fields plus dashboard legacy field names, `BackgroundCell` legacy TS fields, bare-array stats cache fallback, Python legacy stats format / config dict helpers / pylatex stubs, macOS legacy playback mode — [design doc](plans/v050-backward-compatibility-shim-removal-plan.md) `M`
- Legacy `.vrlog` speed-key compatibility cleanup — remove pre-max speed-key fallback readers from the visualiser replay path before v1.0 platform cleanup, after the current migration window closes — [design doc](plans/v050-backward-compatibility-shim-removal-plan.md) `S`
- Schema simplification (migration 000030) — drop dead per-track percentile columns (`p50/p85/p95_speed_mps`), drop always-NULL quality columns from `lidar_tracks`, rename `peak_speed_mps` → `max_speed_mps` on both track tables — [design doc](plans/schema-simplification-migration-030-plan.md) `M`
- Visualiser track proto parity — back out branch-local percentile fields, regenerate proto bindings (Go + Swift); SQL column rename deferred to migration 000030 — [design doc](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md) `S`
- v0.5.0 breaking changes — `transit-backfill` soft-deprecation notice, breaking-changes release notes — [design doc](plans/platform-simplification-and-deprecation-plan.md) `S`
- Config restructure Phase 1 — flat-to-nested realignment with versioned schema, engine selection, and strict validation — [design doc](../config/CONFIG-RESTRUCTURE.md) `M`
- Documentation standardisation — reconcile docs-only branch drift against `main`, standardise opening metadata/summary structure, and enforce validation gates across `docs/` — [design doc](plans/platform-documentation-standardization-plan.md) `S`

## v0.5.1 (Product)

- [#290] (#11) Serial port configuration UI — configure and test radar serial ports via web interface at `/settings/serial`; database-backed, replaces manual systemd service file edits; CLI flag fallback maintained — [design doc](radar/serial-config-quickref.md) `M`
- L8/L9/L10 layer refactor Phases 1–3 — update docs to ten-layer model, create `l8analytics/` package, move comparison/summary types from L6 and storage into L8, slim monitor handlers — [design doc](plans/lidar-l8-analytics-l9-endpoints-l10-client-plan.md) `L`
- SQLite client standardisation — unify DB interfaces across internal/db, internal/api, and internal/lidar/storage; remove API-layer SQL — [design doc](plans/data-sqlite-client-standardization-plan.md) `M`
- Track speed metric redesign + aggregate-only percentiles — reserve `p50/p85/p98` for report/group aggregates, keep `p98` over historical `p95`, and define replacement non-percentile track-level speed metrics — [design doc](plans/speed-percentile-aggregation-alignment-plan.md) `L`
- Metric registry + naming enforcement — establish canonical metric ids/definitions, cross-strata consistency checks, and Prometheus export/tagging stubs with user-defined prefix support — [design doc](plans/metrics-registry-and-observability-plan.md) `M`
- LiDAR tracks table consolidation — extract shared `TrackMeasurement` struct from `TrackedObject`/`RunTrack`, shared SQL column list and scan helpers, optional `lidar_all_tracks` VIEW; requires migration 030 first — [design doc](plans/lidar-tracks-table-consolidation-plan.md) `S`
- Light mode theme compliance — fix hardcoded white colours in TrackList (hex ID invisible), MapPane (canvas legend, grid labels), TimelinePane (SVG labels/strokes), and MapPane overlay panels; replace with theme-aware CSS variables — [design doc §12](ui/design-review-and-improvement.md) `S`
- Mac APP Release signing readiness — prepare code-signing/notarisation prerequisites and release-signing checks for packaged artifacts `S`

## v0.5.2 (Debug)

- Frontend background debug surfaces — Swift visualiser debugging outputs for background settlement — [design doc](plans/web-frontend-background-debug-surfaces-plan.md) `M`
- Visualiser performance and scene health metrics — timeline and VR log metrics; macOS: 30fps frame throttle, per-frame perf logging, scene name/hex ID in RunBrowser, replay epoch tracking — [design doc](plans/lidar-visualiser-performance-and-scene-health-timeline-metrics-plan.md) `M`
- Visualiser debug overlay + cluster proto follow-through — finish `FrameBundle.debug` streaming, cluster field serialisation, and positive serialiser tests — [design doc](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md) `M`

## v0.6 (Deployment & Packaging)

- (#210) Raspberry Pi imager pipeline, downloadable `vr_v0.6.0.img.gz` asset with precompiled binary + LaTeX — [design doc](plans/deploy-rpi-imager-fork-plan.md) `L`
- Precompiled LaTeX — faster PDF report generation via vendored TeX tree — [design doc](plans/pdf-latex-precompiled-format-plan.md) `M`
- Single `velocity-report` binary + subcommands — unified CLI with radar/lidar/pdf subcommands — [design doc](plans/deploy-distribution-packaging-plan.md) `L`
- One-line install script — curl-based installer with automatic platform detection — [design doc](plans/deploy-distribution-packaging-plan.md) `S`
- Geometry-coherent tracking (P1 maths, D-04) — spatial consistency in track association — [proposal](maths/proposals/20260222-geometry-coherent-tracking.md) `M`
- Simplification and deprecation programme (Project B execution) — remove deploy surfaces after #210 gate + migration window; doc/Make cleanup only (Project A complete, Phase 1 signalling done #344) — [design doc](plans/platform-simplification-and-deprecation-plan.md) `M`
- LiDAR foundations fix-it — documentation truth alignment, implementation boundary stabilisation — [design doc](plans/lidar-architecture-foundations-fixit-plan.md) `M`
- Typed UUID prefixes — migrate all UUID generation to 4-char prefixed format (`trak_`, `runa_`, `runy_`, `runs_`, `scne_`, `eval_`, `regn_`, `labl_`, `swep_`); create `internal/id` package; accept mixed formats in SQLite — [design doc](plans/platform-typed-uuid-prefixes-plan.md) `M`
- Cosine error correction remaining items — delete endpoint, report angle annotation, speed limit field migration — [design doc](radar/architecture/site-config-cosine-correction-spec.md) `M`
- Config restructure Phase 2 — expose L1 sensor/network and L3 background/foreground constants as tuning params; deprecate CLI flags — [design doc](../config/CONFIG-RESTRUCTURE.md) `M`
- L8/L9/L10 layer refactor Phases 4–5 — rename `visualiser/` → `l9endpoints/`, absorb chart/dashboard code from `monitor/`, decompose `monitor/` into `server/` + layered packages — [design doc](plans/lidar-l8-analytics-l9-endpoints-l10-client-plan.md) `L`
- `transit-backfill` removal — remove `cmd/transit-backfill` after confirming zero active usage; `velocity-report transits rebuild` is the replacement — [design doc](plans/platform-simplification-and-deprecation-plan.md) `S`

## v0.7 (Unified Frontend)

- (#252) Frontend consolidation (Phases 0–5) — migrate status/regions/sweep to Svelte, retire port 8081 — [design doc](plans/web-frontend-consolidation-plan.md) `L`
- Metrics/stats/frontend consolidation follow-through (Project C/D) — retire duplicate stats surfaces, simplify CLI flags, and prune Make wrappers after parity — [design doc](plans/platform-simplification-and-deprecation-plan.md) `M`
- ECharts → LayerChart rewrite (8 charts, D-11) — migrate all radar/lidar charts to Svelte LayerChart — [design doc](ui/DESIGN.md) `L`
- Frontend decomposition (Svelte stores) — item 13: tracksStore, runsStore, missedRegionStore — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review.md) `M`
- Retire Go-embedded dashboards — ~2,000 lines removed from monitor once Svelte dashboards replace ECharts — [review doc §7](lidar/architecture/lidar-layer-alignment-refactor-review.md) `L`
- Track labelling Phase 9 UI (Swift, D-07) — seekable replay, Swift-native labelling — [design doc](plans/lidar-track-labeling-auto-aware-tuning-plan.md) `M`
- Accessibility testing — add axe-core/playwright asserting no critical violations on each route — [design doc §7.2](ui/design-review-and-improvement.md) `S`
- Widescreen content containment (D-13) — add vr-page max-width centring at ≥3000px — [design doc §2.2](ui/design-review-and-improvement.md) `S`
- macOS palette constants — prepare shared palette definition when metric charts added to visualiser — [design doc §1.2](ui/design-review-and-improvement.md) `S`
- LayerChart policy in LiDAR routes — enforce chart rendering policy (no ad-hoc SVG) when charts added to tracks/scenes/runs/sweeps — [design doc §4.2](ui/design-review-and-improvement.md) `S`
- Platform simplification complete — all deprecated surfaces retired, migration complete — [design doc](plans/platform-simplification-and-deprecation-plan.md) `M`

## v0.8 (Radar Polish & Automation)

- (#4) Radar configuration via UI — read and send radar config commands through the web interface `M`
- (#323) Speed limit schedules (D-16) — time-based speed limits for school zones and weekday/weekend variation — [design doc](radar/architecture/speed-limit-schedules.md) `L`
- Profile comparison data layer hardening — analysis-run compare APIs + contract stabilization — [design doc](plans/lidar-analysis-run-infrastructure-plan.md) `M`
- Profile comparison UI delivery — cross-run compare workflow + scene evaluation UX — [design doc](plans/lidar-track-labeling-auto-aware-tuning-plan.md) `M`
- PDF generation migration to Go — replace Python matplotlib/PyLaTeX with Go SVG charts + Go `text/template` LaTeX assembly; retain XeTeX for typesetting — [design doc](plans/pdf-go-chart-migration-plan.md) `L`
- Transit deduplication (D-03) — duplicate transit record prevention — [design doc](radar/architecture/transit-deduplication.md) `M`
- GitHub Releases CI pipeline — automated binary builds and release packaging — [design doc](plans/deploy-distribution-packaging-plan.md) `M`
- TicTacTail Phase 1 incubation (D-23) — in-repo pkg/tictactail engine + bounded cache + VRLOG thin adapter extraction — [design doc](plans/tictactail-platform-plan.md) `M`

## v0.9.0 (Production-Ready)

- (#8) Data management (backup/archiving) — define backup destinations, read historical archives and rollup SQLite files from remote HTTP location `M`
- (#122) Database monitoring UI — daily table-size snapshots, available disk, growth-rate trends, projected fill-date dashboard `M`
- (#148) Report management UI — view, filter, and download old reports and zip files; paginated table with site/date filters `M`
- (#324) Time-partitioned raw data tables — major storage architecture change — [design doc](radar/architecture/time-partitioned-data-tables.md) `M`
- Threshold-based speed alerts — configurable alerting for speed threshold violations `M`
- Test coverage ≥ 95.5% — raise every internal/, web, Python, and macOS package to ≥ 95.5% line coverage — [design doc](plans/platform-quality-coverage-improvement-plan.md) `L`
- Stable public API with versioned endpoints — formal API versioning and stability guarantees — design doc not yet written `M`
- Visual regression testing — Playwright baseline screenshots — [design doc](ui/design-review-and-improvement.md) `M`
- E2E test infrastructure — Playwright smoke tests — [design doc](ui/design-review-and-improvement.md) `M`

## v1.0 (Vector Scene + VC)

- L7 Scene layer — persistent evidence-accumulated world model, static geometry, canonical objects, OSM priors, multi-sensor fusion architecture — [design doc](plans/lidar-l7-scene-plan.md) `XL`
- Velocity-coherent foreground extraction (P2, D-05) — 6D DBSCAN alternative for moving object detection — [proposal](maths/proposals/20260220-velocity-coherent-foreground-extraction.md) `L`
- Unified settling (L3/L4 SettlementCore, P4, D-05) — consolidate L3 background and L4 drift into single settlement core — [proposal](maths/proposals/20260219-unify-l3-l4-settling.md) `L`
- Geometry-prior local file format (GeoJSON) — local scene geometry configuration via GeoJSON — [design doc](lidar/architecture/vector-scene-map.md) `M`
- Data export (CSV, GeoJSON) — export vehicle transits and scene geometry for external analysis — design doc not yet written `M`

## v2.0 (Advanced Perception & Connected)

- (#103) Python OpenCV angle extraction — compute radar cosine-correction angle from checkerboard image; Python tool callable from webserver `M`
- (#325) Ground-plane vector-scene maths (P3, D-05) — 3D scene reconstruction with ground-plane constraints — [proposal](maths/proposals/20260221-ground-plane-vector-scene-maths.md) `L`
- Visualiser QC programme (Features 1–10) — comprehensive quality control tooling for LiDAR data — [design doc](plans/lidar-visualiser-labelling-qc-enhancements-overview-plan.md) `XL`
- Metrics-first data science programme — benchmark packs, scorecards, explicit specs, and reproducible experiment bundles — [plan](plans/platform-data-science-metrics-first-plan.md) `M`
- Optional classification benchmarking lane (Phase 4.1) — transparent feature-based models compared against the rule-based baseline; not on the critical path — [plan](plans/lidar-ml-classifier-training-plan.md) `L`
- Config restructure Phase 3 — expose L2/L5/pipeline constants and L6 classification thresholds once classifier strategy is settled — [design doc](../config/CONFIG-RESTRUCTURE.md) `S`
- Parameter tuning optimisation (Phase 4.2) — automated hyperparameter search and optimisation — [plan](plans/lidar-parameter-tuning-optimisation-plan.md) `L`
- Dynamic algorithm selection — runtime algorithm switching based on scene conditions — [design doc](plans/lidar-architecture-dynamic-algorithm-selection-plan.md) `M`
- Bodies in motion — L5 IMM kinematic extensions (CV/CA/CTRV), L7 scene-constrained path prediction, sparse-cluster track linking at range, and scene-graph geometric relations — [design doc](plans/lidar-bodies-in-motion-plan.md) `L`
- Peak-hour and seasonal trend analysis — temporal pattern detection and analysis `M`

## v∞.0 (Deferred, waybacklog)

- (#7) Live SQL query view — browser-based SQL query tool; low priority while TailSQL suffices `S`
- (#9) LAN authentication — add auth layer if deployment moves beyond private LAN — [design doc §10.1](ui/design-review-and-improvement.md) `M`
- (#322) Motion capture architecture — moving sensor support — [design doc](plans/lidar-motion-capture-architecture-plan.md) `XL`
- (#326) AV dataset integration — 28-class taxonomy, Parquet ingestion — [design doc](plans/lidar-av-lidar-integration-plan.md) `XL`
- (#327) AV range image format — dual-return support — [design doc](lidar/architecture/av-range-image-format-alignment.md) `L`
- Static pose alignment — 7-DOF tracking — [design doc](plans/lidar-static-pose-alignment-plan.md) `L`
- Online geometry-prior service — opt-in community-maintained geometry priors (local-only remains default) — [design doc](lidar/architecture/vector-scene-map.md) `L`
- Coverage thresholds — raise codecov thresholds to meaningful levels after coverage improves — [design doc §7.5](ui/design-review-and-improvement.md) `S`
- Multi-location aggregate dashboard — cross-site analytics and comparative reporting `L`
- ECharts palette cross-reference — document palette alignment requirement for Phase 3 frontend consolidation migration — [design doc §3.3](ui/design-review-and-improvement.md) `S`

## Complete

- [#144] LiDAR analysis-run infrastructure (Phase 3.7) — versioned run storage + comparison/split/merge scaffolding implemented — [design doc](plans/lidar-analysis-run-infrastructure-plan.md)
- [#240] Visualiser background snapshot serialisation — `frameBundleToProto` serialises `FrameBundle.background`, `frame_type`, `background_seq` — [design doc](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)
- [#280] 501 stub replacement (evaluation and reprocess endpoints) — review doc item 4 — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review.md)
- [#280] Arena.go deprecation — [design doc](lidar/architecture/arena-go-deprecation-and-layered-type-layout-design.md)
- [#280] L1–L6 layer alignment and code migration — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review.md)
- [#280] Route table conversion and HTTP method prefixes — review doc items 3 and 11 — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review.md)
- [#284] background.go split — background_persistence.go, background_export.go, background_drift.go extracted — [design doc §6.2](ui/design-review-and-improvement.md)
- [#284] CompareRuns extraction — comparison logic moved to l6objects/comparison.go — [design doc §6.3](ui/design-review-and-improvement.md)
- [#284] Cross-layer placement fixes — background.go split, webserver.go split, CompareRuns extraction — review doc item 14 — [review doc](lidar/architecture/lidar-layer-alignment-refactor-review.md)
- [#284] LiDAR monitor file splits — echarts_handlers.go + export_handlers.go — [review doc §1–2](lidar/architecture/lidar-layer-alignment-refactor-review.md)
- [#284] Storage compareParams extraction — analysis_run_compare.go — [review doc §6](lidar/architecture/lidar-layer-alignment-refactor-review.md)
- [#284] Sweep file splits — hint_progress/notifications.go, auto_narrowing.go, sweep_params.go — [review doc §3–5](lidar/architecture/lidar-layer-alignment-refactor-review.md)
- [#284] Visualiser frame codec extraction — frame_codec.go extracted from adapter.go — [review doc §8](lidar/architecture/lidar-layer-alignment-refactor-review.md)
- [#284] webserver.go split — datasource_handlers.go + playback_handlers.go extracted — [design doc §6.1](ui/design-review-and-improvement.md)
- [#286] Chart empty-state placeholder — "No chart data available" shown when chartData is empty — [design doc §3.1](ui/design-review-and-improvement.md)
- [#286] DESIGN.md references — Design Language section added to CONTRIBUTING.md; link added to README.md — [design doc §8.1](ui/design-review-and-improvement.md)
- [#286] Shared CSS standard classes — vr-page, vr-toolbar, vr-stat-grid, vr-chart-card in app.css — [design doc §2.1](ui/design-review-and-improvement.md)
- [#286] Shared palette module — palette.ts exports PERCENTILE_COLOURS, LEGEND_ORDER with tests — [design doc §1.3](ui/design-review-and-improvement.md)
- [#286] Web palette compliance — palette.ts created with canonical DESIGN.md §3.3 values; colorMap/cRange removed — [design doc §1.1](ui/design-review-and-improvement.md)
- [#287] LiDAR logging stream split — Opsf/Diagf/Tracef streams replacing Debugf/classifier, with ops/debug/trace routing rubric — [design doc](lidar/architecture/lidar-logging-stream-split-and-rubric-design.md)
- [#291] PR template design checklist — add DESIGN.md §9 UI/chart checklist to .github/PULL_REQUEST_TEMPLATE.md — [design doc §8.2](ui/design-review-and-improvement.md)
- [#298] macOS versioned DMG export — `create-dmg.sh` with Finder window layout for drag-to-install DMG, CI DMG job scoped to tag pushes — [design doc](plans/deploy-distribution-packaging-plan.md)
- [#319] Settling optimisation Phase 3 — convergence/evaluation tooling — [design doc](lidar/operations/settling-time-optimization.md)
- [#320] Python venv consolidation — Makefile uses root .venv/; remove stale tools/pdf-generator/.venv — [design doc](plans/tooling-python-venv-consolidation-plan.md)
- [#328] SWEEP/HINT platform hardening (Phase 5–6) — transform pipeline, objective registry, explainability — [design doc](plans/lidar-sweep-hint-mode-plan.md)
- [#328] HINT sweep polish — 11 remaining polish items — [design doc](lidar/operations/hint-sweep-mode.md)
- [#328] (#326) P0 ObjectClass schema alignment, label vocabulary consolidation Phases 1–3.1 — [design doc](plans/label-vocabulary-consolidation-plan.md) [AV plan §P0](plans/lidar-av-lidar-integration-plan.md)
- [#328] Visualiser track field serialisation — all Track fields now round-trip in `frameBundleToProto`, `TestFrameBundleToProto_TrackFieldCompleteness` added — [design doc](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)
- [#328] Visualiser `ObjectClass` enum — 9-class enum on proto field 26, `objectClassFromString` / `classifyOrConvert` conversion, Go + Swift tests — [design doc](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)
- [#330] Platform simplification Phase 1 — deprecation signalling and deploy retirement gate — [design doc](plans/platform-simplification-and-deprecation-plan.md)
- [#336] Visualiser proto contract parity + debug overlays — `FrameBundle.debug` streaming, cluster/track field serialisation, `avg_speed_mps` → `median_speed_mps` + p85/p98 — [design doc](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)
- [#336] v0.5.0 breaking changes — proto field 24 rename, AvgSpeedMps removal from visualiser model, deployment deprecation warnings — [design doc](plans/platform-simplification-and-deprecation-plan.md)
- [#352] PCAP analysis replay hardening — blocking frame channel for zero-drop analysis, speed-mode rename (fastest→analysis, fixed→scaled), SpeedRatio API, per-phase backoff logging with SubLogger, batched track DB writes, disable persistence during analysis runs, replay epoch tracking, FrameBuilder deadlock fix — [design doc](lidar/operations/pcap-analysis-mode.md)
- [#352] Benchmark mode and runtime profiling — BenchmarkMode toggle for performance tracing, pprof HTTP routes, heap-allocation tracking in health summary — [design doc](plans/lidar-clustering-observability-and-benchmark-plan.md)
- [#352] Occlusion aggregate metrics — per-frame occlusion stats in TrackingMetrics and sweep pipeline; speed_ratio sweep variable; dashboard exposure — [design doc](plans/lidar-visualiser-performance-and-scene-health-timeline-metrics-plan.md)
- [#352] Proto `peak_speed_mps` → `max_speed_mps` rename (D-19) — proto field 25, Go/Swift/TS model rename, regenerated bindings; SQL column deferred to migration 000030 — [design doc](plans/lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)
- [#356] VRLOG analysis §12.1 metrics — `analyse-vrlog` command with `GenerateReport` and `CompareReports` for implementable-now track quality metrics and distribution statistics — [design doc](data/VRLOG_ANALYSIS.md)
- [#364] Layer dependency hygiene — moved `PointPolar`, `Point`, `SphericalToCartesian`, `ApplyPose` from L4 to L2; fixed L1→L4 and L3→L4 import violations across ~30 files — [design doc](plans/lidar-layer-dependency-hygiene-plan.md)
- [#364] LiDAR L2 dual representation — `LiDARFrame` stores both `PolarPoints` and `Points`; pipeline consumes frame-owned polar data directly; per-frame polar rebuild eliminated — [design doc](plans/lidar-l2-dual-representation-plan.md)
