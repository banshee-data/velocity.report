# Development Log

Status: Active
Purpose/Summary: DEVLOG.

## February 19, 2026 - LiDAR Layer Alignment & Architecture Review

- Implemented LiDAR 6-layer alignment refactor: split `l3grid/background.go` into persistence, export, and drift files.
- Split `monitor/webserver.go` into data-source and playback handler files.
- Extracted domain comparison logic from `storage/sqlite` into `l6objects`.
- Added HTTP method prefixes and middleware wrappers to route tables.
- Fixed route conflict panic from duplicate route registrations.
- Consolidated architecture docs, created `BACKLOG.md` for deferred work items.
- Documented further opportunities to reduce library size and complexity.
- Completed review items 11–14 and P1–P3 from LiDAR layer alignment review.
- Removed redundant method-not-allowed tests (mux handles 405 via method-prefixed routes).

## February 18, 2026 - Design System & TeX/Chart Updates

- Created `DESIGN.md` with project-wide design principles and frontend design language.
- Conducted comprehensive design review against `DESIGN.md`, producing improvement plan.
- Updated Python PDF generator TeX configuration for minimal precompiled install.
- Enhanced Svelte chart components (RadarOverviewChart).
- Updated Go CI integration tests to use minimal TeX tree (`build-texlive-minimal`) instead of full TeX Live install.
- Trimmed CI TeX Live packages: dropped `texlive-fonts-extra` (~500 MB) and `latexmk`; added `--no-install-recommends`.

## February 15-16, 2026 - HINT Tuning System & LiDAR Track Improvements

- Implemented Human-Involved Numerical Tuning (HINT) system (renamed from RLHF).
- Replaced HTTP client with in-process `DirectBackend` for sweep runner, eliminating HTTP overhead.
- Added long-polling for HINT status and PCAP completion.
- Refactored label taxonomy: `good_vehicle`/`good_pedestrian` → `car`/`ped`; added `impossible` label.
- Implemented suspend and resume functionality for auto-tune sweeps with checkpoint persistence.
- Added per-frame OBB dimensions and improved heading handling in LiDAR tracking.
- Enhanced cluster size filtering, track pruning, and classification updates.
- Serialised frame callback processing in `frame_builder.go` to prevent data races.
- Added gap detection in `MapPane.svelte` to prevent spaghetti lines in track visualisations.
- Added height band filtering parameters to tuning configuration.
- macOS visualiser: added ground reference grid toggle, background grid points toggle, track filtering with dual-handle range slider.
- Refactored all default parameters to load from `tuning.defaults.json` instead of hardcoded values.
- Bumped version to 0.5.0-pre8 and 0.5.0-pre9.
- Created LiDAR 6-layer data model documentation (OSI-style).
- Added LiDAR labelling QC enhancements plan.
- Created LiDAR refactor plans for package restructuring.

## February 14, 2026 - Sweep Schema Fixes & Documentation Updates

- Fixed sweep parameter schema and config compatibility issues from PR review.
- Updated documentation to reflect current code state: corrected Makefile target count (59→101), Go version (1.21+→1.25+), SQLite version (3.51.2), Python version (3.11+).
- Fixed broken doc links and wrong paths in `TROUBLESHOOTING.md`.
- Expanded repo structure in `copilot-instructions.md` (5→15 internal packages).
- Fixed setup guide frontmatter cost and typos.
- Dependency update: bumped `markdown-it` 14.1.0→14.1.1 in docs site.

## February 12, 2026 - Precompiled LaTeX Plan & Test Expansion

- Created design document for precompiled LaTeX `.fmt` support in PDF generator.
- Expanded test coverage across Go, Python, and macOS components.

## February 11, 2026 - RLHF Score Explainability & Label Provenance

- Implemented `RLHFTuner` engine with RLHF API endpoints and handler tests.
- Added RLHF mode to sweep dashboard UI and Svelte sweeps page.
- Implemented score component breakdown: `ScoreComponents`, `ScoreExplanation`, and `/explain` API endpoint.
- Added class/time coverage gates for RLHF continue validation.
- Implemented `label_source` provenance tracking with IoU-based confidence.
- Added schema/version stamp fields in sweep persistence (migration 9.1).
- Fixed VRLog seek race condition; fixed int64 overflow in temporal spread calculation.
- Boosted RLHF test coverage to 91.9%; web test coverage to 97%.
- Created RLHF expansion plan with ML solver-inspired optimisation approach.
- Designed `velocity.report-imager` (RPi Imager fork) for simplified deployment.
- Added comprehensive Swift tests for macOS visualiser.
- Expanded Go test coverage: Runner, sweep, tracking API, UDP listener, tracking pipeline.
- Simplified packet handling by directly using polar points in `FrameBuilder`.
- Bumped version to 0.5.0-pre7.

## February 10-11, 2026 - VRLog Replay & Track Labelling

- Implemented `.vrlog` recording and replay for track labelling workflow.
- Added `FrameRecorder` interface, `vrlog_path` field, and playback API endpoints.
- Implemented VRLOG replay in visualiser publisher and gRPC control delegation.
- macOS visualiser: run browser, run-track labelling support, side panel for track selection.
- Added VRLOG safe directory configuration with absolute path validation.
- Implemented background snapshot sending during VRLOG replay.
- Enhanced tuning parameters and acceptance metrics tracking in background processing.
- Added label taxonomy and classification details for detection and quality in LiDAR terminology.
- Bumped version to 0.5.0-pre6.

## February 9-10, 2026 - Label-Aware Auto-Tuning & LiDAR Refinement

- Implemented track label-aware auto-tuning: scoring incorporates human labels.
- Enhanced LiDAR tuning refinement with updated parameters and thresholds.
- Added PCAP analysis improvements for tune benchmarks.
- Expanded Go unit test coverage across multiple packages.
- Created frontend consolidation design document.
- Designed Swift label plan for macOS visualiser track labelling workflow.

## February 8, 2026 - Documentation Audit & Roadmap

- Comprehensive audit of all 29 LiDAR documentation files against actual codebase.
- Identified 12 discrepancies between docs and implementation status.
- Updated README, devlog, and 9 other documentation files with current status.
- Produced consolidated LiDAR roadmap with prioritised future work (P0–P3).
- Cross-referenced approved track labelling design document across relevant docs.
- Implemented auto-tuning system (`sweep/auto.go`): iterative grid narrowing, multi-objective scoring.
- Enhanced sweep dashboard: two new heatmaps (tracks, alignment), PARAM_SCHEMA with sane defaults.
- Increased chart height from 300px to 450px, changed grid layout from 6 to 3 columns.
- Fixed PCAP replay methods to use full file path for `pcap_file` parameter.
- Created design document for track labelling, ground truth evaluation, and label-aware auto-tuning (8 phases).
- Designed `lidar_transits` table schema for dashboard/report integration.
- Identified label API route gap: CRUD handlers exist but routes not registered.

## February 7, 2026 - Param Sweep Dashboard & Auto-Tune Mode

- Consolidated LiDAR configuration into single config struct with fluent setters.
- Implemented parameter sweep dashboard with ECharts bar charts and results table.
- Added auto-tuning mode toggle and recommendation card to sweep dashboard.
- Implemented settle mode support in sweep runner: `once` and `per_combo`.
- Added iteration validation and clamping in `Sample` method.
- Created sweep sampler, scoring, and chart data preparation utilities.

## February 5-6, 2026 - macOS Visualiser M3.5–M7

- M3.5: Split streaming — background snapshots every 30s + foreground-only per-frame (78→3 Mbps).
- M4: Tracking interface refactor — `TrackerInterface`, `ClustererInterface`, golden replay tests.
- M5: Algorithm upgrades — Hungarian association, ground removal, OBB estimation, occlusion coasting.
- M6: Debug overlays — gating ellipses, association lines, residuals via gRPC; label API handlers.
- M7: Performance hardening — Swift buffer pooling, PointCloudFrame reference counting, frame skip cooldown.
- Added LiDAR settling time optimisation design document.

## February 3-4, 2026 - macOS Visualiser M0–M3

- Designed macOS visualiser architecture (SwiftUI + Metal + gRPC).
- M0: Schema + synthetic — protobuf schema, gRPC server, synthetic data generator, SwiftUI+Metal renderer.
- M1: Recorder/replayer — `.vrlog` format, seek/pause/rate control, deterministic playback.
- M2: Real point clouds — `FrameAdapter`, decimation modes, 70k+ points at 30fps.
- M3: Canonical model — `FrameBundle` as single source of truth, LidarView + gRPC from same model.
- Added app icon assets.

## February 2, 2026 - Map Feature & CI Refactor

- Added interactive map component for site visualisation.
- Refactored CI pipeline with end-to-end test support.
- Dependency updates across application group.

## February 1, 2026 - Documentation Homepage

- Updated homepage spacing and added hamburger menu.
- Refined documentation site styling.

## January 31, 2026 - Dependency Injection & Test Coverage

- Implemented dependency injection interfaces: `CommandExecutor`, `UDPSocket`, `PCAPReader`, `DataSourceManager`.
- Refactored `UDPListener` to use `SocketFactory` for testability.
- Created `RealDataSourceManager` wrapping WebServer operations.
- Added sweep sampler for parameter sweep iterations with CSV output.
- Added `BackgroundFlusher` for periodic persistence with mock implementations.
- Added `BackgroundConfig` with validation and fluent setters.
- Added chart data preparation utilities for LiDAR visualisation.
- Increased Go test coverage to 85.9% on critical packages.
- Restructured documentation paths to use `public_html`.
- Added JavaScript unit tests for `TRACK_COLORS` in LiDAR types.
- Extended WebServer and BackgroundManager test coverage.
- Bumped version to 0.4.2.

## January 30, 2026 - Test Coverage Expansion

- Increased Go test coverage from ~38% to 73.1% on testable packages.
- Added comprehensive tests: monitoring, serialmux factory/mock, LiDAR arena, quality scoring.
- Added admin routes integration tests and serialmux extended tests.
- Enhanced tracking tests: `GetAllTracks`, speed history, quality metrics, spatial coverage.
- Added CONTRIBUTING guide.
- Fixed CI version check scripts and changelog links.

## January 29, 2026 - Release v0.4.0 & Setup Guide

- Released version 0.4.0 with comparison reports, site config, and transit worker.
- Created comprehensive setup guide for Citizen Radar deployment.
- Added code coverage badges (Go, Python, Web) with Codecov integration.
- Added coverage documentation and CI workflow updates.
- Enhanced documentation site: dark mode, Tailwind v4 CSS, KaTeX math rendering.
- Fixed documentation CI to use pnpm instead of yarn.

## January 25, 2026 - Radar Config SCD & Comparison Reports

- Implemented site config periods with cosine correction (Type 6 SCD pattern).
- Added boundary hour filtering to `RadarObjectRollupRange` for improved data accuracy.
- Added histogram aggregation with configurable `max_bucket`.
- Enhanced PDF report generation: detailed data tables, velocity unit support, percentile lines.
- Added `compare_source` to `ReportRequest` for dual-source comparison.
- Added `min_speed_used` to `RadarStatsResult` API responses.
- Implemented save/load report settings from local storage.
- Schema ordering fix: foreign key dependency-aware table creation.
- Added security validation for PDF path (path traversal prevention).
- Bumped version to 0.4.0-pre9.

## January 20, 2026 - Transit Worker Inspector

- Added full-history run capability to transit worker and API.
- Implemented transit CLI: `analyse`, `delete`, `migrate`, `rebuild` commands.
- Added transit deduplication plan and tests.
- Enhanced transit worker UI with run management features.
- Updated model version default from `rebuild-full` to `hourly-cron`.
- Bumped version to 0.0.4-pre8.

## January 19, 2026 - Comparison Report Generator

- Added comparison-period report generation with dual-period outputs (T1/T2).
- Enhanced PDF report with comparison metrics and improved labelling.
- Refactored `chart_builder` to use integer indices for x-axis.
- Added PDF generator version tracking in `set-version` script.
- Bumped version to 0.0.4-pre7.

## January 17, 2026 - Track Visualisation UI Fixes

- Fixed click detection to check track history, not just head position.
- Filtered (0,0) noise points from rendering (backend and frontend).
- Added timestamp sorting for coherent track history lines.
- Progressive track reveal during playback (point-by-point as timeline advances).
- Added pagination (50 tracks/page) with navigation controls to TrackList component.
- Added "Min Observations" filter (1+/5+/10+/20+/50+) to filter noise tracks.
- Fixed timeline sync with pagination (TimelinePane shows paginated subset via `onPaginatedTracksChange` callback).
- Fixed label truncation (removed `.slice(-6)`, increased sidebar width to 500px, left margin to 120px).
- Increased `HitsToConfirm` from 3 to 5 (tracks require 5 consecutive observations before confirmation).
- Added physical plausibility checks: `MaxReasonableSpeedMps=30.0`, `MaxPositionJumpMeters=5.0` in Mahalanobis gating.
- Increased API limit from 100 to 1000 tracks (`getTrackHistory` default=500).

## January 14-16, 2026 - Adaptive Region Segmentation

- Implemented adaptive region segmentation in BackgroundGrid for distance-aware thresholds.
- Added `RegionParams` struct with configurable `ThresholdMultiplier` and `WarmupFrames` per region.
- Default regions: near (0-30m, 1.0x), mid (30-60m, 1.2x), far (60-100m, 1.5x), extended (100m+, 2.0x).
- Fixed `WarmupFramesRemaining` initialisation logic (reset on grid clear, decrement per frame).
- Added region identification to `ProcessFramePolarWithMask` for per-point threshold scaling.
- Added region dashboard HTML template embedded in Go webserver via `html/template`.
- Fixed JSON serialisation for region parameters in API responses.
- Aggressively tuned thresholds to achieve <30 false positives target (from 150+).
- Fixed potential XSS vulnerability (code scanning alert #33) with HTML escaping.

## January 13, 2026 - Warmup Trails Fix

- Fixed false positive foreground classifications during grid warmup (~100 frames).
- Implemented dynamic threshold multiplier: 4x at count=0, tapering to 1x at count=100.
- Fixed `recFg` accumulation during cell freeze (reset to 0 on thaw).
- Added track quality metrics for investigation.
- Enhanced grid plotter visualisation for debugging.

## January 11, 2026 - PCAP Analyse Benchmarking

- Extended pcap-analyse tool with performance benchmarking capabilities.
- Added CI pipeline integration for automated benchmark runs.
- Improved frame processing performance metrics.

## January 9, 2026 - Test Coverage Expansion

- Added comprehensive unit tests across LiDAR pipeline (~3,363 lines of new test code).
- Improved coverage for background processing, clustering, and tracking modules.
- Added edge case testing for frame builder and parser.

## January 6, 2026 - Foreground Forwarding & Debugging

- Implemented foreground point forwarding to UDP port 2370 for downstream processing.
- Added PCAP realtime replay mode for development/testing.
- Created parameter tuning UI for runtime adjustment of background model settings.
- Added AV Range Image Format Alignment architecture document (dual return handling).
- Fixed build failure with stub generation for pcap-disabled builds.
- Applied security fixes for path validation.

## January 3, 2026 - DB Stats API

- Added `GET /api/db/stats` endpoint for database size and table statistics.
- Implemented path traversal security hardening in file operations.
- Added duplicate snapshot cleanup utility.

## January 2, 2026 - LiDAR Documentation Updates

- Restructured LiDAR documentation for improved navigation.
- Added velocity-coherent foreground extraction design document (1,456 lines).
- Merged in upstream dependency updates for documentation and application packages.

## December 28, 2025 - LiDAR Alignment Planning

- Created comprehensive alignment planning document for multi-sensor fusion.
- Documented clustering algorithms comparison (DBSCAN vs HDBSCAN).
- Added occlusion handling strategies and static pose alignment procedures.
- Designed motion capture architecture for dynamic calibration.

## December 16, 2025 - AV-LIDAR Integration Plan

- Added comprehensive integration plan for LIDAR frame analyser (913 lines).
- Documented frame data flow from sensor to classification pipeline.
- Specified integration points with AV perception stack.

## December 10, 2025 - LiDAR Track Visualisation UI

- Implemented `MapPane.svelte` canvas component for real-time track rendering.
- Added `TimelinePane.svelte` with D3-based SVG timeline and playback controls.
- Created `TrackList.svelte` sidebar with track selection and filtering.
- Added background grid overlay API for spatial visualisation.
- Integrated WebSocket updates for live track streaming.

## December 9, 2025 - Background Grid Standards & PCAP Split

- Documented LiDAR background grid export standards and format options.
- Added pcap-split tool design specification (1,094 lines) for PCAP file segmentation.
- Defined training data export formats for ML pipeline.

## December 2, 2025 - CLI Architecture Guide

- Created CLI comprehensive guide with long-term architecture vision (1,715 lines).
- Documented command structure, flag conventions, and extension patterns.
- Added LiDAR foreground tracking API implementation notes (Phases 2.9-3.6).

## December 1, 2025 - Release 0.3.0 & Infrastructure

- Implemented `velocity-deploy` deployment manager (install, upgrade, rollback, backup, health commands).
- Added hourly transit worker job with UI toggle for background processing.
- Created `scan_transits` backfill tool for historical data processing.
- Baselined all components to version 0.2.0, then bumped to 0.3.0.
- Reorganised SQL migrations, inserted original schema as 000001.
- Added `set-version.sh` utility for cross-codebase version updates.
- Created time-partitioned data tables specification (2,980 lines).
- Added speed limit schedules feature spec (school zones, time-based limits).
- Completed security review of data partitioning plan (CVE fixes for path traversal, SQL injection, race conditions).

## December 1, 2025 - Phase 3.7 Analysis Run Infrastructure

- Implemented `AnalysisRun` type for versioned parameter configurations with `params_json` storage.
- Added `RunParams` type capturing all configurable parameters (background, clustering, tracking, classification).
- Created `RunTrack` type extending track data with user labels and quality flags for ML training.
- Implemented `AnalysisRunStore` with database operations: `InsertRun()`, `CompleteRun()`, `GetRun()`, `ListRuns()`.
- Added track management: `InsertRunTrack()`, `GetRunTracks()`, `UpdateTrackLabel()`.
- Created labeling progress API: `GetLabelingProgress()`, `GetUnlabeledTracks()`.
- Added split/merge detection types: `RunComparison`, `TrackSplit`, `TrackMerge` for run comparison.
- Renumbered phases: 4.0→3.7 (Analysis Run), 4.1→4.0 (Labeling UI), 4.2→4.1 (ML Training).

## December 1, 2025 - ML Pipeline Roadmap

- Created comprehensive ML Pipeline Roadmap documentation (now tracked in `docs/plans/lidar-track-labeling-ui-implementation-plan.md` and related LiDAR plan docs).
- Planned Phase 4.0 Track Labeling UI: SvelteKit routes, track browser, trajectory viewer, labeling panel.
- Planned Phase 4.1 ML Classifier Training: feature extraction, Python training pipeline, Go model deployment.
- Planned Phase 4.2 Parameter Tuning: grid search, quality metrics, objective function optimisation.
- Recommended implementation order: ✅3.7 → 4.0 → 4.2 (parallel) → 4.1 → 4.3.

## December 1, 2025 - Phase 3.6 PCAP Analysis Tool

- Implemented `cmd/tools/pcap-analyze/main.go` for batch PCAP processing through full tracking pipeline.
- Full pipeline: parse UDP → build frames → background subtraction → clustering → tracking → classification.
- Added output formats: JSON (complete results), CSV (track table), binary (training data blobs).
- Computed speed percentiles (P50/P85/P95) per track.
- Added `SpeedHistory()` getter to `TrackedObject` for external percentile computation.
- Added `GetAllTracks()` method to `Tracker` for retrieving all tracks including deleted.
- Added build tag `pcap` to `integration_test.go` for conditional test execution.

## November 30, 2025 - Pose Simplification

- Removed `internal/lidar/pose.go` and `internal/lidar/pose_test.go` (deferred to future phase).
- Removed pose-related fields from `ForegroundFrame`, `TrainingFrameMetadata`, `TrainingDataFilter`, `TrackObservation`, `WorldCluster`, `TrackSummary`.
- Updated SQL schemas to remove `pose_id` columns from `lidar_clusters`, `lidar_tracks`, `lidar_track_obs`.
- Updated `track_store.go` and `track_api.go` to use simplified signatures.
- Training data stored in polar (sensor) frame, which is pose-independent.

## November 30, 2025 - Phase 3.5 Track/Cluster REST API

- Implemented `TrackAPI` struct in `internal/lidar/monitor/track_api.go`.
- Added endpoints: `GET /api/lidar/tracks`, `GET /api/lidar/tracks/active`, `GET /api/lidar/tracks/{id}`.
- Added endpoints: `PUT /api/lidar/tracks/{id}`, `GET /api/lidar/tracks/{id}/observations`.
- Added endpoints: `GET /api/lidar/tracks/summary`, `GET /api/lidar/clusters`.
- Created JSON response structures: `TrackResponse`, `ClusterResponse`, `TracksListResponse`, `TrackSummaryResponse`.
- Supports both in-memory tracker (real-time) and database queries.

## November 30, 2025 - Phases 3.3-3.4 SQL Schema & Classification

- Created migration `000009_create_lidar_tracks.up.sql` with `lidar_clusters`, `lidar_tracks`, `lidar_track_obs` tables.
- Implemented persistence in `track_store.go`: `InsertCluster()`, `InsertTrack()`, `UpdateTrack()`, `InsertTrackObservation()`.
- Added queries: `GetActiveTracks()`, `GetTrackObservations()`, `GetRecentClusters()`.
- Implemented rule-based classification in `classification.go` with object classes: pedestrian, car, bird, other.
- Added `ComputeSpeedPercentiles()` for P50/P85/P95 from speed history.
- Added `ObjectClass`, `ObjectConfidence`, `ClassificationModel` fields to `TrackedObject`.

## November 30, 2025 - Phases 2.9-3.2 Foreground Tracking Pipeline

- Phase 2.9: Implemented `ProcessFramePolarWithMask()` for per-point foreground/background classification.
- Phase 3.0: Added `WorldPoint` struct and `TransformToWorld()` with pose support.
- Phase 3.1: Implemented `SpatialIndex` with Szudzik pairing, `DBSCAN()` with eps=0.6m, minPts=12.
- Phase 3.2: Implemented Kalman tracking with `TrackedObject`, `Tracker`, Mahalanobis gating.
- Added track lifecycle: Tentative → Confirmed → Deleted with hits/misses counting.
- Added ML training data support: `ForegroundFrame`, `EncodeForegroundBlob()`/`DecodeForegroundBlob()`.

## November 29, 2025 - Distribution & Tracking Plans

- Created distribution and packaging plan document (1,636 lines).
- Created LIDAR foreground extraction and tracking implementation plan v2 with polar/world frame separation.
- Documented Szudzik pairing function for spatial indexing.

## November 19, 2025 - Database Migration System

- Implemented database migration system using golang-migrate.
- Created 12 migration files covering schema evolution.
- Integrated migration CLI commands into main binary.

## November 14, 2025 - Migration System Design

- Added database migration system design document.
- Evaluated golang-migrate vs custom solution.
- Documented migration file conventions and versioning strategy.

## November 7, 2025 - LaTeX Security Fix

- Fixed LaTeX injection vulnerability (CVE-9.8 severity) with `escape_latex()` function.
- Sanitised all user inputs before PDF generation.
- Fixed JavaScript download bug in report generation.

## November 6, 2025 - Python Venv Consolidation & AI Agents

- Consolidated Python virtual environments to single repository root `.venv/`.
- Added Malory (security) and Thompson (communications) custom AI agents.
- Merged multiple dependabot dependency updates.

## November 5, 2025 - Docs Restructure & Path Security

- Restructured Eleventy documentation site with syntax highlighting, typography plugin, breadcrumbs.
- Added community page to documentation.
- Implemented path validation to prevent traversal attacks in file operations.

## November 4, 2025 - Build Metadata & PCAP API

- Added GIT SHA and build time to HTML meta tags via `set-build-env.js`.
- Moved PCAP/live data source flag from CLI to runtime API (`POST /api/lidar/source`).
- Updated Makefile naming conventions (162+ line changes).

## November 1, 2025 - PCAP Security & Grid Visualisation

- Implemented path traversal protection with `--lidar-pcap-dir` flag using `filepath.Join()` + `filepath.Abs()` + prefix checking.
- Added file validation: regular files only, `.pcap`/`.pcapng` extensions required, 403 Forbidden for path escape.
- Added systemd integration: service auto-creates PCAP directory via `ExecStartPre`.
- Enhanced 4K-optimised dashboard (25.6×14.4" @ 150 DPI): 3 polar/spatial charts + 4 stacked metric panels.
- Added PCAP snapshot mode with configurable interval/duration, auto-numbered directories, metadata JSON.
- Created API helper scripts: grid reset, PCAP replay, background status fetching.
- Added Makefile targets for noise sweep/multisweep plotting.
- Added Python plotting tools: polar/cartesian heatmaps with live and PCAP replay modes.
- Consolidated DEBUG-LOGGING-PLAN, GRID-ANALYSIS-PLAN, GRID-HEATMAP-API, LIDAR-PCAP-Debug docs into sidecar overview.

## October 31, 2025 - Grid Analysis API & Debug Logging

- Added `GET /api/lidar/grid_heatmap` endpoint for spatial bucket aggregation (40 rings × 120 azimuth buckets).
- Implemented `GetGridHeatmap()` with configurable bucket size and settled threshold.
- Response includes summary stats and per-bucket metrics: fill/settle rates, mean range/times seen, frozen cells.
- Added Python plotting tools: polar (ring vs azimuth) and cartesian (X-Y) heatmaps.
- Created noise analysis scripts: `plot_noise_sweep.py`, `plot_noise_buckets.py`.
- Added comprehensive logging: grid reset timing, API call logs, rate-limited population tracking.
- Enhanced FrameBuilder diagnostics: eviction logging, frame callback, improved azimuth wrap detection.
- Re-enabled `SeedFromFirstObservation` with `--lidar-seed-from-first` flag.
- Added settle time flag, configurable background flush interval and frame buffer timeout.
- Added Makefile targets: dev-go, log-go-tail, log-go-cat, dev-go-pcap.
- Fixed frame eviction callback delivery bug.

## October 30, 2025 - PCAP Debugging & Development Tools

- Enhanced frame eviction logging and finalised frame callback delivery path.
- Added diagnostics for non-zero channel counts in ParsePacket.
- Improved azimuth wrap detection for large negative jumps (>180°).
- Added --debug flag for frame completion and PCAP parsing logs.
- Created local API helper scripts for PCAP replay and background status.
- Consolidated dev-go logic into reusable run_dev_go function in Makefile.
- Added log-go-cat and log-go-tail targets.
- Corrected log directory name in .gitignore.

## October 29, 2025 - Configuration & Documentation Cleanup

- Updated lidar configuration flags for clarity and consistency.
- Enhanced documentation for database path and command flags.
- Added `SeedFromFirstObservation` parameter for PCAP mode background initialisation.
- Removed outdated Frontend Units Override Feature documentation.

## October 28, 2025 - PCAP Support Foundation

- Added PCAP file replay support with BPF filtering for multi-sensor files.
- Integrated with existing parser and frame builder for seamless replay.
- Added background persistence during PCAP replay with configurable flush intervals.
- Added `--lidar-pcap-mode` flag to disable UDP listening for replay-only mode.
- Added `POST /api/lidar/pcap/start` endpoint for triggering PCAP replay via API.
- Updated LiDAR sidecar overview with classification, filtering, and metrics implementation details.

## October 27, 2025 - Formatting & Linting

- Added formatting and linting commands to Makefile.

## October 23, 2025 - Sites, Timezones & JavaScript Tests

- Updated file prefix conventions for reports.
- Added timezone handling for sites.
- Added JavaScript test suite with CI integration.

## October 21, 2025 - Vite Security Update

- Bumped vite from 7.1.5 to 7.1.11 for security fix.

## October 14, 2025 - PDF Generator Cleanup

- Cleaned up PDF generator code and structure.

## October 13, 2025 - Report Templates & Tests

- Tweaked LaTeX report templates.
- Added report generation tests.

## October 3, 2025 - PDF Report Initialisation

- Initialised PDF report generation with LaTeX templates (report.pdf).

## September 27, 2025 - Background Parameters & Multisweep

- Added configurable parameters: `closeness_multiplier`, `neighbor_confirmation_count`.
- Created multisweep tool for parameter exploration.
- Fixed export destination to use `os.TempDir()`.

## September 23, 2025 - Background Diagnostics & Monitor APIs

- Centralised runtime diagnostics with `internal/monitoring` logger and per-manager `EnableDiagnostics` flag.
- Added BackgroundManager helpers: `SetNoiseRelativeFraction`, `SetEnableDiagnostics`, `GetAcceptanceMetrics`, `ResetAcceptanceMetrics`, `GridStatus`, `ResetGrid`.
- Added monitor API endpoints: `GET/POST /api/lidar/params`, `GET /api/lidar/acceptance`, `POST /api/lidar/acceptance/reset`, `GET /api/lidar/grid_status`, `POST /api/lidar/grid_reset`.
- Created `cmd/bg-sweep` CLI: incremental & settle modes, per-noise grid reset, live bucket discovery, CSV output.

## September 22, 2025 - Background Model Fixes & Snapshot Export

- Wired BackgroundManager into LiDAR pipeline with self-contained snapshots.
- Persisted per-ring elevation angles (`ring_elevations_json`) with each `lidar_bg_snapshot`.
- Centralised snapshot-to-ASC export with elevation fallbacks.
- Added backfill tool for populating `ring_elevations_json` in existing snapshots.
- Improved `ProcessFramePolar`: restrict neighbor confirmation to same-ring, update spread EMA relative to previous mean.
- Fixed concurrent SQLite update pattern to avoid SQLITE_BUSY.

## September 21, 2025 - Server & SerialMux Consolidation

- Centralised HTTP server and UI paths into `internal/api`.
- Standardised on single SQLite DB (`sensor_data.db`) in `internal/db`.
- Added LiDAR background snapshot persistence with manual HTTP trigger.
- Added `--disable-radar` flag and robust `DisabledSerialMux`.
- Merged duplicate LiDAR webservers; canonical monitor accepts injected `*db.DB` and `SensorID`.
- Moved radar event handlers to `internal/serialmux/handlers.go`, classification to `parse.go`.
- Added unit tests for serialmux (DisabledSerialMux, classification, config parsing, event handlers).

## September 20, 2025 - Snapshot & Persistence Improvements

- Hardened BackgroundGrid persistence with RW-mutexes and copy-under-read.
- Added DB access for snapshots via GetLatestBgSnapshot helper.
- Added monitor endpoint to fetch, gunzip/gob-decode and summarise stored snapshots.
- Moved manual persist endpoint into lidar monitor webserver.

## September 19, 2025 - BackgroundManager & Polar Processing

- Introduced BackgroundManager registry with `NewBackgroundManager` constructor.
- Added managers discoverable via `GetBackgroundManager`/`RegisterBackgroundManager`.
- Implemented snapshot serialisation (gob + gzip) with `Persist` method.
- Added `InsertBgSnapshot` in lidar DB layer.
- Implemented `ProcessFramePolar`: bin by ring/azimuth, EMA updates, neighbor-confirmation, freezing heuristics.

## September 18, 2025 - Polar-First Refactor

- Centralised spherical→Cartesian math into `transform.go` helper.
- Introduced `PointPolar` type; parser now emits polar-first.
- Added `FrameBuilder.AddPointsPolar([]PointPolar)`, removed legacy `AddPoints([]Point)`.
- UDP listener forwards polar points directly.

## September 17, 2025 - Background Model & Transform Design

- Designed sensor-frame background model (ring × azimuth) for foreground masking.
- Two-level settling per cell: fast noise settling, slow parked-object settling.
- Designed BackgroundGrid snapshot persistence and warm-start on load.
- Planned spherical→Cartesian refactor with polar/cartesian point type split.
- Planned world-grid (height-map / ground estimate) on masked Cartesian points.

## September 15, 2025 - Velocity Graph

- Added velocity graph component to web frontend.

## September 13, 2025 - LiDAR Frame Parsing & Test Improvements

- Implemented LiDAR packet parsing into complete 360° frames.
- Added units, velocity, and timezone configuration support.
- Eliminated implementation dependencies in parse tests with local test constants.
- Fixed boundary conditions in PCAP extraction loop bounds.
- Streamlined extractUDPPayloads by removing redundant conditional checks.

## September 12, 2025 - Frame Builder Tests & Time-Based Detection

- Fixed 3 previously failing frame builder tests with realistic production data patterns.
- Moved PCAP integration test to `internal/lidar/integration_test.go`.
- Created `internal/lidar/testdata/` directory following Go conventions.
- Increased test point counts to 60,000 points matching production.
- Implemented hybrid frame detection: time-based primary with azimuth validation.
- Integrated motor speed extraction from packet tail (bytes 8-9).
- Added dynamic frame duration based on actual RPM (50ms at 1200 RPM, 100ms at 600 RPM).
- Added --sensor-name flag for flexible deployment.
- Enhanced code documentation in extract.go with packet structure details.

## September 11, 2025 - Memory Optimisation & Frame Rate Fixes

- Analysed Hesai Pandar40P UDP packet structure via Wireshark.
- Discovered Ethernet tail issue: extra 4 bytes appended to UDP packets.
- Fixed tail offset from last 6 bytes to last 10 bytes.
- Validated correct UDP sequence extraction and point parsing.
- Confirmed proper frame characteristics: ~69,000 points per frame, ~100ms duration.

## September 8, 2025 - LiDAR Parser Initialisation

- Initialised LiDAR parser for Hesai Pandar40P protocol.
- Added UDP listener for sensor data ingestion.
- Created basic packet parsing structure.

## September 5, 2025 - Telraam Integration

- Added Python tool to fetch Telraam traffic counting data.

## September 2, 2025 - UniFi Protect Integration

- Added Python tool to fetch UniFi Protect camera data.

## August 27, 2025 - Production Web Assets

- Fixed and bundled production web assets in Go binary.

## August 26, 2025 - Frontend Integration & Middleware

- Integrated Svelte frontend with Go backend server.
- Added Flush method to loggingResponseWriter for proper streaming.
- Moved DB schema declaration to `.sql` file.
- Fixed RadarObjects query and migration.
- Added VSCode settings for development.

## August 25, 2025 - Svelte Dashboard

- Created first dashboard slices with Svelte, svelte-ux, layerchart.
- Fixed Svelte theme configuration.
- Added Tailwind CSS styling.

## August 21, 2025 - Favicon & Logging Middleware

- Added favicon serving.
- Implemented LoggingMiddleware with colour-coded HTTP status codes.

## August 20, 2025 - Radar Stats API

- Added `/api/radar_stats` endpoint for aggregated radar statistics.

## July 23, 2025 - Documentation Merge

- Merged gh-pages branch into main for unified documentation.

## July 10, 2025 - README Enhancement

- Added ASCII art logo to README.

## June 28, 2025 - Code of Conduct

- Added CODE_OF_CONDUCT.md for community guidelines.

## June 27, 2025 - RadarObject Parsing & Project Structure

- Rebased and implemented RadarObject parsing.
- Restructured into `cmd/radar/`, `internal/*` packages.
- Renamed project to velocity.report.
- Added Apache 2.0 license.
- Implemented JSON parsing for radar data.
- Added unit tests for parsing.

## June 3, 2025 - Radar Objects Table

- Created `radar_objects` SQL table.
- Added RadarObjects database functions.

## May 30, 2025 - Live Tail & Command Fixes

- Fixed live tail WebSocket functionality.
- Fixed command sending to serial port.

## May 26, 2025 - Serial Port Configuration

- Updated serial port configuration handling.
- Updated event list SQL queries.

## May 22, 2025 - SerialMux Abstraction

- Initialised SerialMux for multiple subscriber support.
- Fixed graceful shutdown handling.
- Fixed x/net dependabot warning.

## May 16, 2025 - Dev Server & Web Skeleton

- Created working dev server configuration.
- Began web app skeleton with package namespace.
- Added systemd unit file for deployment.
- Fixed nested `/api/` route handling.
- Added `/debug/tailsql` and `/debug/backup` routes.

## May 12, 2025 - SQLite Migration & Serial Tests

- Replaced DuckDB with SQLite (modernc.org/sqlite for pure Go).
- Added serialReader unit tests.
- Implemented bufio for serial port buffering.

## April 11, 2025 - Command Logging & Handler Extraction

- Added command logging to database.
- Extracted serialPortHandler into separate function.

## March 21, 2025 - Serial Reader & API Improvements

- Implemented uptime parsing and validation.
- Added readline counter for serial data.
- Improved serial reader reliability.
- Updated backup filenames and intervals.
- Renamed execute endpoint.
- Set commandID from database.
- Added JSON response logging.
- Fixed timestamp casting in SQL.
- Added API verb support for commands.

## March 20, 2025 - Baud Rate & Schema Updates

- Set serial baud rate to 19.2k.
- Updated schema to v0.0.2 with uptime field.
- Added POST /execute endpoint.

## March 17, 2025 - Project Initialisation

- Initialised repository with Go 1.24.
- Created initial server structure with Gin HTTP framework.
- Set up serial port reader for radar sensor.
- Added gocron for backup scheduling.
- Configured DuckDB for initial data storage.
- Added .gitignore for database files and backup directory.
