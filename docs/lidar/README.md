# LiDAR Documentation

Documentation for the velocity.report LiDAR subsystem (Hesai Pandar40P integration).

## Folder Structure

### `architecture/`

Core system design and implementation specifications.

- **Technical overview** of the LiDAR processing pipeline
- **Foreground tracking pipeline** implementation (Phases 2.9–3.7)
- **Background grid standards** comparison with industry formats
- **AV range image format** design for dual-return support (future)

### `operations/`

Runtime operations and debugging.

- **Data source switching** between live UDP and PCAP replay
- **PCAP analysis mode** for background characterisation
- **Performance regression testing** for pipeline benchmarking
- **Auto-tuning** parameter sweep and iterative grid narrowing
- **Settling time optimization** for efficient parameter sweeps
- **Tracking status** and known issues

### `reference/`

Protocol specifications and data formats.

- **Packet structure** for Hesai Pandar40P UDP format
- **Database schema** for LiDAR tables

### `roadmap/`

Development progress and future planning.

- **ML pipeline roadmap** (Phases 4.0–4.3: labelling UI, training, tuning)
- **Development log** with implementation notes

### `future/`

Deferred features for specialised use cases.

- **AV integration plan** (28-class taxonomy, Parquet ingestion)
- **Motion capture architecture** (moving sensors)
- **Velocity-coherent extraction** (alternative algorithm)
- **PCAP split tool** for motion/static segmentation
- **Static pose alignment** (7DOF tracking)

These are **not required** for static roadside deployments.

### `troubleshooting/`

Resolved investigation notes for reference.

- **Warmup trails fix** (January 2026)

### `noise_investigation/`

Historical analysis artifacts from background parameter tuning.

### `visualiser/`

Design documentation for the macOS-native 3D visualiser.

- **Problem statement** and user workflows
- **API contracts** (protobuf schema, gRPC service)
- **Architecture** (Track A visualiser, Track B pipeline refactor)
- **Implementation plan** (incremental milestones)
- **Labelling + QC enhancements** (quality scoring, event timeline, repairs, queueing, dashboard)

### `refactor/`

Tracking pipeline refactor and upgrade proposals.

- **Tracking upgrades** (ground removal, OBB, association, debug overlays)

## Quick Links

| Topic                   | Document                                                                                         |
| ----------------------- | ------------------------------------------------------------------------------------------------ |
| System overview         | [architecture/lidar_sidecar_overview.md](architecture/lidar_sidecar_overview.md)                 |
| Tracking implementation | [architecture/foreground_tracking.md](architecture/foreground_tracking.md)                       |
| Current status          | [operations/lidar-foreground-tracking-status.md](operations/lidar-foreground-tracking-status.md) |
| ML pipeline             | [roadmap/ml_pipeline_roadmap.md](roadmap/ml_pipeline_roadmap.md)                                 |
| Packet format           | [reference/packet_analysis_results.md](reference/packet_analysis_results.md)                     |
| **macOS Visualiser**    | [visualiser/01-problem-and-user-workflows.md](visualiser/01-problem-and-user-workflows.md)       |
| **API Contracts**       | [visualiser/02-api-contracts.md](visualiser/02-api-contracts.md)                                 |
| **Labelling + QC Plan** | [visualiser/06-labelling-qc-enhancements-overview.md](visualiser/06-labelling-qc-enhancements-overview.md) |
| **Tracking Upgrades**   | [refactor/01-tracking-upgrades.md](refactor/01-tracking-upgrades.md)                             |
| **Auto-Tuning Plan**    | [operations/auto-tuning.md](operations/auto-tuning.md)                                           |
| **Track Labelling**     | [future/track-labeling-auto-aware-tuning.md](future/track-labeling-auto-aware-tuning.md)         |

## Implementation Status

### Completed Work

#### Phases 1–3.7: Core Pipeline (Sep 2025 – Jan 2026)

- ✅ UDP packet ingestion (Hesai Pandar40P)
- ✅ Frame assembly (360° rotations)
- ✅ Background learning (EMA-based polar grid)
- ✅ Foreground/background classification with warmup scaling
- ✅ DBSCAN clustering (world frame)
- ✅ Kalman tracking (constant velocity model)
- ✅ Rule-based classification (pedestrian, car, bird, other)
- ✅ REST API endpoints for tracks/clusters
- ✅ PCAP analysis tool for batch processing
- ✅ Analysis run infrastructure (params JSON, run comparison)
- ✅ Port 2370 foreground streaming
- ✅ Track visualisation UI (Svelte: MapPane, TimelinePane, TrackList)

#### Phase 3.8: Tracking Upgrades (Jan 2026)

- ✅ Hungarian (Kuhn-Munkres) optimal assignment (`internal/lidar/hungarian.go`)
- ✅ Height-based ground removal (`internal/lidar/ground.go`)
- ✅ PCA-oriented bounding boxes with temporal smoothing (`internal/lidar/obb.go`)
- ✅ Occlusion coasting — MaxMissesConfirmed=15 (`internal/lidar/tracking.go`)
- ✅ Debug overlay emission via gRPC (`internal/lidar/debug/collector.go`)

#### Phase 3.9: Adaptive Regions & Sweep System (Jan–Feb 2026)

- ✅ Adaptive region segmentation (stable/variable/volatile)
- ✅ Region persistence & restoration (scene hash-based, skips settling on subsequent runs)
- ✅ Parameter sweep runner with settle mode — once/per_combo (`internal/lidar/sweep/runner.go`)
- ✅ Auto-tuner with iterative grid narrowing (`internal/lidar/sweep/auto.go`)
- ✅ Multi-objective scoring — acceptance, alignment, tracks, cells (`internal/lidar/sweep/objective.go`)
- ✅ Sweep dashboard — ECharts: bar charts, heatmaps, results table (`sweep_dashboard.html`)
- ✅ PARAM_SCHEMA with sane defaults for all numeric parameters

#### Phase 4.0: Track Labelling & VRLOG Replay (Feb 2026)

- ✅ VRLOG recording format — binary frame bundles with index for seek (`internal/lidar/visualiser/recorder/`)
- ✅ VRLOG replay in Publisher — `StartVRLogReplay`, `StopVRLogReplay`, `SeekVRLog`, `SetVRLogRate`
- ✅ gRPC control delegation — Pause/Play/Seek/SetRate with VRLOG mode routing
- ✅ REST playback API — `/api/lidar/playback/*` (status, pause, play, seek, rate)
- ✅ VRLOG load API — `/api/lidar/vrlog/load` (by run_id or vrlog_path), `/api/lidar/vrlog/stop`
- ✅ Path traversal protection — validate vrlog_path within allowed directory
- ✅ Run-track label API — `PUT /api/lidar/runs/{run_id}/tracks/{track_id}/label`
- ✅ DB migration 000023 — `vrlog_path` column for analysis runs
- ✅ Swift run browser UI — `RunBrowserView`, `RunBrowserState` for loading analysis runs
- ✅ Swift label API client — `RunTrackLabelAPIClient` for track labelling

See: [`docs/plans/track-labeling-ui-plan.md`](../plans/track-labeling-ui-plan.md)

#### macOS Visualiser: M0–M7 Complete (Oct 2025 – Feb 2026)

- ✅ M0: Schema + Synthetic — gRPC streaming, synthetic data
- ✅ M1: Recorder/Replayer — Deterministic playback with seek/pause
- ✅ M2: Real Point Clouds — Live pipeline via gRPC
- ✅ M3: Canonical Model — LidarView + gRPC from same source
- ✅ M3.5: Split Streaming — 96% bandwidth reduction (BG/FG separation)
- ✅ M4: Tracking Interface — Golden replay tests, deterministic clustering
- ✅ M5: Algorithm Upgrades — OBB, Hungarian association, occlusion handling
- ✅ M6: Debug + Labelling — Full debug overlays, label export
- ✅ M7: Performance Hardening — Buffer pooling (7.1, 7.2, 7.3 complete)

**Test Coverage (February 2026):**

- `internal/lidar/visualiser`: 89.7%
- `internal/lidar/network`: 94.7%
- `internal/lidar/monitor`: 75.9%
- `internal/lidar`: 87.0%

**Resolved Issues (January 2026):**

- ✅ Warmup trails (sensitivity scaling fix)
- ✅ Port 2370 packet corruption (RawBlockAzimuth preservation)
- ✅ recFg accumulation during freeze (reset on thaw)

### Planned Work (Priority Order)

#### P0: Track Labelling Enhancements (Remaining Phase 4.0)

- Ground truth evaluation engine (temporal IoU, Hungarian matching)
- Label-aware auto-tuning (composite scoring vs reference)
- Web playback controls (optional parity with macOS visualiser)
- `lidar_transits` table for dashboard/report integration

#### P1: ML Classification Pipeline (Phase 4.1–4.3)

- Feature extraction (spatial, kinematic, temporal, intensity)
- Python training pipeline (scikit-learn / RandomForest)
- ONNX model export for Go inference
- Rule-based → ML classifier with fallback
- See: [`roadmap/ml_pipeline_roadmap.md`](roadmap/ml_pipeline_roadmap.md)

#### P2: Tracking Upgrades (Remaining)

- Voxel grid preprocessing
- Constant acceleration motion model
- Feature extraction for ML training
- See: [`refactor/01-tracking-upgrades.md`](refactor/01-tracking-upgrades.md)

#### P2: Settling Time Optimisation (Phases 3–4)

- Settling evaluation tool
- Adaptive convergence-based settling
- See: [`operations/settling-time-optimization.md`](operations/settling-time-optimization.md)

#### P3: Velocity-Coherent Clustering (Design Only)

- 6D DBSCAN (position + velocity space)
- Long-tail tracking, sparse continuation, fragment merging
- See: [`future/velocity-coherent-foreground-extraction.md`](future/velocity-coherent-foreground-extraction.md)

### Deferred (Research / AV Integration)

- AV dataset format alignment (7-DOF, 28-class taxonomy, Parquet)
- Motion capture / moving sensor support (7-DOF pose, ego-motion)
- Static pose alignment (3D Kalman, oriented boxes)
- Background grid standard exports (VTK, PCD)

See `roadmap/ml_pipeline_roadmap.md` for detailed Phase 4.0–4.3 planning.
