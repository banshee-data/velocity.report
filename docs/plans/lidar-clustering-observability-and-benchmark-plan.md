# Design: clustering observability metrics and performance benchmark harness

- **Status:** Proposed (February 2026)
- **Layers:** L4 Perception, L8 Analytics
- **Related:** [Clustering Maths](../../data/maths/clustering-maths.md), [Performance and Scene Health Metrics](lidar-visualiser-performance-and-scene-health-timeline-metrics-plan.md), [Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md)
- **Canonical:** [clustering-diagnostics.md](../lidar/operations/clustering-diagnostics.md)

## Problem

Two classes of problem lack tooling:

1. **Trail glitches**: tracks that repeatedly exhibit heading flips, speed jitter, merge/split artefacts, or dropout gaps on the same stretch of road. We cannot diagnose _why_ without per-frame, per-track clustering diagnostics. The pipeline currently logs aggregate counts (`N confirmed tracks active`) but nothing about _which_ cluster was associated, how many points it had, what the innovation residual was, or whether the cluster was subsampled.

2. **Performance blind spot**: the `pcap-analyse -benchmark` harness captures wall-clock timing and throughput, but has no clustering-specific metrics (cluster count distribution, points-per-cluster, DBSCAN grid utilisation, subsampling frequency). When stepping down from a Mac M1 (8-core, 16 GB) to a Raspberry Pi 4 (4-core, 4 GB) we need fine-grained levers for trading detection quality against latency, and CI regression gates to catch regressions before deployment.

## Goals

- Define per-track/per-frame diagnostic metrics that can be logged at `diagf`/`tracef` level and optionally embedded in VRLOG frames.
- Identify which metrics to expose on the existing `/api/lidar/scene-health` and analysis-run statistics surfaces.
- Spec a checked-in benchmark log format for clustering performance, with CI regression gating.
- Provide tuning levers (documented) for Raspberry Pi deployment.

## Non-Goals

- Full distributed tracing / OpenTelemetry integration.
- Real-time UI rendering of per-track diagnostics (covered by the timeline metrics plan).
- Changing the VRLOG serialisation format from JSON to protobuf (separate task).
- Modifying the DBSCAN algorithm itself.

---

## Part a: per-track / per-frame observability

### A.1 per-frame pipeline stage timing

Instrument `NewFrameCallback()` in [internal/lidar/pipeline/tracking_pipeline.go](../../internal/lidar/pipeline/tracking_pipeline.go) with `time.Now()` checkpoints at each stage boundary. Collect as a `FrameStageTiming` struct:

| Field                | Type     | Description                         |
| -------------------- | -------- | ----------------------------------- |
| `FrameID`            | `string` | Frame identifier                    |
| `TimestampNanos`     | `int64`  | Frame start timestamp               |
| `PolarConvertUs`     | `int64`  | L1→polar conversion                 |
| `BackgroundUpdateUs` | `int64`  | L3 `ProcessFramePolarWithMask`      |
| `GroundFilterUs`     | `int64`  | Height-band ground removal          |
| `VoxelDownsampleUs`  | `int64`  | Optional voxel grid (0 if disabled) |
| `ClusteringUs`       | `int64`  | L4 DBSCAN + feature extraction      |
| `TrackingUs`         | `int64`  | L5 predict + associate + update     |
| `ClassificationUs`   | `int64`  | L6 object classification            |
| `SerialisationUs`    | `int64`  | Visualiser adapt + gRPC publish     |
| `TotalUs`            | `int64`  | Wall-clock frame-to-frame           |
| `ForegroundPoints`   | `int`    | Points after background subtraction |
| `FilteredPoints`     | `int`    | Points after ground filter          |
| `ClusterCount`       | `int`    | Clusters from DBSCAN                |
| `ConfirmedTracks`    | `int`    | Confirmed tracks this frame         |
| `Throttled`          | `bool`   | Frame was throttled                 |
| `Subsampled`         | `bool`   | DBSCAN input was subsampled         |
| `SubsampleInputN`    | `int`    | Pre-subsample point count           |
| `SubsampleOutputN`   | `int`    | Post-subsample point count          |

**Logging:** At `tracef` level, emit a single structured line per frame:

```
[Pipeline] frame=%s total_us=%d bg_us=%d cluster_us=%d track_us=%d fg=%d clusters=%d tracks=%d subsampled=%t
```

At `diagf` level, emit a summary every 100 frames:

```
[Pipeline] 100-frame avg: bg=%.1fms cluster=%.1fms track=%.1fms fg_pts=%.0f clusters=%.1f tracks=%.1f subsample_rate=%.1f%%
```

**VRLOG embedding:** Add `FrameStageTiming` to `FrameBundle.DebugOverlaySet` as an optional field. This makes timing data available during offline replay analysis without requiring the debug log.

### A.2 per-track association diagnostics

For each cluster→track association in `Tracker.Update()`, log (at `tracef` level) the decision context:

| Field             | Type      | Description                             |
| ----------------- | --------- | --------------------------------------- |
| `TrackID`         | `string`  | Target track                            |
| `ClusterID`       | `int`     | Associated cluster                      |
| `MahalDistance`   | `float32` | Mahalanobis distance (pre-gating)       |
| `GatingThreshold` | `float32` | Effective gating distance               |
| `Accepted`        | `bool`    | Whether association was accepted        |
| `InnovationX`     | `float32` | Measurement residual X (metres)         |
| `InnovationY`     | `float32` | Measurement residual Y (metres)         |
| `PredictedX`      | `float32` | Kalman-predicted position X             |
| `PredictedY`      | `float32` | Kalman-predicted position Y             |
| `ClusterPoints`   | `int`     | Points in the associated cluster        |
| `ClusterArea`     | `float32` | OBB area (length × width)               |
| `HistoricalArea`  | `float32` | Track's running average area            |
| `AreaRatio`       | `float32` | Current / historical (merge/split flag) |
| `HeadingDeltaDeg` | `float32` | Cluster heading − track heading         |
| `SpeedDeltaMps`   | `float32` | Implied speed − Kalman speed            |

These fields already partially exist in the `DebugOverlaySet` (`AssociationCandidate`, `InnovationResidual`). The proposal formalises them and ensures they are always populated when `tracef` logging is active.

**Glitch trail diagnosis:** A track that jitters typically exhibits:

- Oscillating `AreaRatio` (merge/split cycles),
- High `HeadingDeltaDeg` between consecutive frames,
- `MahalDistance` near the gating boundary (fragile association),
- Low `ClusterPoints` (sparse cluster = unstable OBB).

By logging these per-frame for all tracks, a post-hoc grep of the debug log by `TrackID` immediately reveals _which_ frames caused the glitch and _why_.

### A.3 per-track lifecycle summary

On track deletion, emit a `diagf`-level summary:

```
[Track] id=%s state=%s duration=%.1fs length=%.1fm obs=%d misses_max=%d occlusions=%d heading_jitter=%.1f° speed_jitter=%.1fm/s merge_events=%d split_events=%d alignment_mean=%.1f° class=%s conf=%.2f
```

All fields already exist on `TrackedObject`. This is a formatting change, not new data collection.

### A.4 cluster quality metrics (new fields on worldCluster)

Add to `WorldCluster` in [internal/lidar/l4perception/types.go](../../internal/lidar/l4perception/types.go):

| Field                     | Type      | Description                                |
| ------------------------- | --------- | ------------------------------------------ |
| `DBSCANNeighboursVisited` | `int`     | Neighbour queries during cluster expansion |
| `PointDensity`            | `float32` | Points per m² of OBB area                  |
| `RangeMean`               | `float32` | Mean range of cluster points from sensor   |
| `RangeSpread`             | `float32` | Max − min range within cluster             |

`PointDensity` reveals sparse long-range clusters likely to produce jittery tracks. `RangeMean` allows stratifying benchmark metrics by distance band.

### A.5 VRLOG diagnostic channel

Current `DebugOverlaySet` fields in `FrameBundle`:

- `AssociationCandidate`: cluster↔track distances
- `GatingEllipse`: semi-major/minor/rotation
- `InnovationResidual`: Kalman innovation
- `StatePrediction`: predicted state

Proposed additions (all optional, populated only when debug logging is active):

- `FrameStageTiming`: per-frame pipeline timing (Section A.1)
- `ClusterDiagnostics []ClusterDiag`: per-cluster quality (Section A.4)
- `AssociationDecisions []AssociationDecision`: full per-track association log (Section A.2)

This keeps the VRLOG self-contained: when replaying a recording, all diagnostic context is available without needing the original text log.

---

## Part b: clustering performance benchmark harness

### B.1 new metrics in `pcap-analyse`

Extend `PerformanceMetrics` in [cmd/tools/pcap-analyse/main.go](../../cmd/tools/pcap-analyse/main.go):

**`ClusteringMetrics` struct** — extends `PerformanceMetrics` in [cmd/tools/pcap-analyse/main.go](../../cmd/tools/pcap-analyse/main.go):

| Field                   | Type              | JSON key                   | Notes                                         |
| ----------------------- | ----------------- | -------------------------- | --------------------------------------------- |
| `ForegroundPointsStats` | DistributionStats | `foreground_points_stats`  | Per-frame foreground point count distribution |
| `FilteredPointsStats`   | DistributionStats | `filtered_points_stats`    | Per-frame filtered point count distribution   |
| `ClusterCountStats`     | DistributionStats | `cluster_count_stats`      | Per-frame cluster count distribution          |
| `PointsPerClusterStats` | DistributionStats | `points_per_cluster_stats` | Points per cluster distribution               |
| `ClusterAreaStats`      | DistributionStats | `cluster_area_stats`       | Cluster area distribution                     |
| `DBSCANCallCount`       | int64             | `dbscan_call_count`        | Total DBSCAN invocations                      |
| `DBSCANTotalUs`         | int64             | `dbscan_total_us`          | Cumulative DBSCAN wall time (µs)              |
| `DBSCANAvgUs`           | float64           | `dbscan_avg_us`            | Mean DBSCAN time per call                     |
| `DBSCANP95Us`           | float64           | `dbscan_p95_us`            | 95th percentile DBSCAN time                   |
| `DBSCANP99Us`           | float64           | `dbscan_p99_us`            | 99th percentile DBSCAN time                   |
| `SubsampleCount`        | int64             | `subsample_count`          | Frames where MaxInputPoints exceeded          |
| `SubsampleRate`         | float64           | `subsample_rate_pct`       | % of frames subsampled                        |
| `ConfirmedTrackCount`   | int               | `confirmed_track_count`    | Tracks reaching confirmed state               |
| `TentativeTrackCount`   | int               | `tentative_track_count`    | Tracks still tentative                        |
| `FragmentationRatio`    | float64           | `fragmentation_ratio`      | tentative / (tentative + confirmed)           |
| `MeanTrackDurationSecs` | float64           | `mean_track_duration_secs` | Average track lifetime                        |
| `MeanTrackLengthMeters` | float64           | `mean_track_length_meters` | Average track spatial length                  |
| `HeadingJitterRMSDeg`   | float64           | `heading_jitter_rms_deg`   | Heading stability (RMS degrees)               |
| `SpeedJitterRMSMps`     | float64           | `speed_jitter_rms_mps`     | Speed stability (RMS m/s)                     |
| `SpatialIndexPeakCells` | int64             | `spatial_index_peak_cells` | Max grid cells in any frame                   |

**`DistributionStats` struct** — used for per-frame histogram summaries:

| Field     | Type    | Notes                  |
| --------- | ------- | ---------------------- |
| `Min`     | float64 | Minimum value          |
| `Max`     | float64 | Maximum value          |
| `Avg`     | float64 | Mean                   |
| `P50`     | float64 | Median                 |
| `P95`     | float64 | 95th percentile        |
| `P99`     | float64 | 99th percentile        |
| `Samples` | int64   | Number of observations |

### B.2 benchmark baseline format (v2)

Extend the existing `baseline-{name}.json` format with a `clustering` key. This is backward-compatible: the comparison logic skips missing keys.

The `baseline-{name}.json` format gains a `clustering` key (v2, backward-compatible — comparison skips missing keys). Example entries under `metrics.clustering`:

| Key                            | Example | Notes                            |
| ------------------------------ | ------- | -------------------------------- |
| `foreground_points_stats.avg`  | 1450    | Mean foreground points per frame |
| `foreground_points_stats.p95`  | 3800    | 95th percentile                  |
| `cluster_count_stats.avg`      | 18.3    | Mean clusters per frame          |
| `points_per_cluster_stats.avg` | 78      | Mean points per cluster          |
| `dbscan_call_count`            | 2604    | Total DBSCAN calls               |
| `dbscan_avg_us`                | 119.8   | Mean DBSCAN time (µs)            |
| `dbscan_p95_us`                | 450.0   | 95th percentile (µs)             |
| `dbscan_p99_us`                | 1200.0  | 99th percentile (µs)             |
| `subsample_count`              | 3       | Frames subsampled                |
| `subsample_rate_pct`           | 0.12    | % frames subsampled              |
| `confirmed_track_count`        | 312     | Confirmed tracks                 |
| `fragmentation_ratio`          | 0.23    | Fragmentation metric             |
| `heading_jitter_rms_deg`       | 4.2     | Heading jitter (degrees)         |
| `speed_jitter_rms_mps`         | 0.8     | Speed jitter (m/s)               |

Each `DistributionStats` object contains `min`, `max`, `avg`, `p50`, `p95`, `p99`, and `samples`.

### B.3 CI regression gating

Add a `bench-clustering` step to [.github/workflows/go-ci.yml](../../.github/workflows/go-ci.yml) (runs only on pushes to `main` and PRs that modify [internal/lidar/l4perception/](../../internal/lidar/l4perception), [internal/lidar/l5tracks/](../../internal/lidar/l5tracks), or [internal/lidar/pipeline/](../../internal/lidar/pipeline)):

Add a `bench-clustering` job to [.github/workflows/go-ci.yml](../../.github/workflows/go-ci.yml). It runs on `ubuntu-latest` after the `build` job, gated on pushes to `main` and PRs modifying [internal/lidar/l4perception/](../../internal/lidar/l4perception), [internal/lidar/l5tracks/](../../internal/lidar/l5tracks), or [internal/lidar/pipeline/](../../internal/lidar/pipeline). Steps: checkout (with LFS), setup Go (from `go.mod`), install `libpcap-dev`, then run `make test-perf NAME=kirk0` (exits 1 on regression: >15% wall-clock or >25% DBSCAN p99).

**Regression thresholds** (configurable via `pcap-analyse` flags):

| Metric                  | Warning (exit 0) | Regression (exit 1) |
| ----------------------- | ---------------- | ------------------- |
| `wall_clock_ms`         | >10%             | >20%                |
| `dbscan_p99_us`         | >15%             | >30%                |
| `subsample_rate_pct`    | absolute >5%     | absolute >15%       |
| `fragmentation_ratio`   | >0.05            | >0.15               |
| `confirmed_track_count` | < -10%           | < -25%              |

### B.4 checked-in benchmark log

Store baselines as checked-in JSON files (existing pattern):

```
internal/lidar/perf/baseline/
├── baseline-kirk0.json           # local Mac ARM64 baseline
├── baseline-kirk0-ci.json        # CI (Linux x86_64) baseline
├── baseline-kirk0-pi.json        # Raspberry Pi ARM64 baseline (manual)
└── baseline-lidar_20Hz.json      # second fixture
```

**Makefile targets:**

| Target                  | Purpose                                                        |
| ----------------------- | -------------------------------------------------------------- |
| `bench-clustering`      | Run clustering benchmark locally (`make test-perf NAME=kirk0`) |
| `bench-clustering-save` | Save a new local baseline (`make test-perf-save NAME=kirk0`)   |
| `bench-clustering-ci`   | Run benchmark with CI profile (`PROFILE=ci`)                   |
| `bench-clustering-pi`   | Run benchmark with Raspberry Pi profile (`PROFILE=pi`)         |

The `PROFILE` variable selects the baseline file suffix (`-ci`, `-pi`) for comparison.

The `PROFILE` variable selects the baseline file suffix (`-ci`, `-pi`) for comparison.

### B.5 Go micro-benchmarks (new)

Add targeted benchmarks in `internal/lidar/l4perception/cluster_benchmark_test.go`:

Benchmark functions in `internal/lidar/l4perception/cluster_benchmark_test.go`:

| Benchmark                               | Input size   |
| --------------------------------------- | ------------ |
| `BenchmarkDBSCAN_500pts`                | 500 points   |
| `BenchmarkDBSCAN_2000pts`               | 2000 points  |
| `BenchmarkDBSCAN_5000pts`               | 5000 points  |
| `BenchmarkDBSCAN_8000pts`               | 8000 points  |
| `BenchmarkSpatialIndexBuild_5000pts`    | 5000 points  |
| `BenchmarkSpatialIndexQuery_5000pts`    | 5000 points  |
| `BenchmarkUniformSubsample_10000to8000` | 10000 → 8000 |

These provide isolated signal for DBSCAN algorithmic performance independent of the full pipeline.

These provide isolated signal for DBSCAN algorithmic performance independent of the full pipeline.

Add a Makefile target:

| Target     | Purpose                                                                                                                                                |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `bench-go` | Run all Go micro-benchmarks (`-bench=. -benchmem -count=3`) across `l4perception/`, `l5tracks/`, and `l3grid/`, writing results to `bench-results.txt` |

---

## Part c: Raspberry Pi performance tuning levers

### C.1 tuning parameters with Pi impact

These parameters in [config/tuning.defaults.json](../../config/tuning.defaults.json) directly affect computational cost. Documented here for operators deploying on Pi:

| Parameter                       | Default | Pi Recommendation | Impact                                           |
| ------------------------------- | ------- | ----------------- | ------------------------------------------------ |
| `foreground_max_input_points`   | 8000    | 2000–4000         | Caps DBSCAN input; O(n·k) improvement            |
| `foreground_dbscan_eps`         | 1.0     | 1.5–2.0           | Larger eps → fewer grid cells, faster queries    |
| `foreground_min_cluster_points` | 3       | 5                 | Fewer tiny clusters → less tracking overhead     |
| `max_frame_rate`                | 25      | 10–12             | Drop frames early; biggest single lever          |
| `max_tracks`                    | 64      | 32                | Caps association matrix size                     |
| `voxel_leaf_size`               | 0 (off) | 0.15              | 60–70% point reduction before DBSCAN             |
| `remove_ground`                 | true    | true              | Ground filter is cheap; keeps DBSCAN input small |

### C.2 Pi-specific benchmark profile

Create `baseline-kirk0-pi.json` by running `make test-perf NAME=kirk0` on a Pi 4 and checking in the result. This gives a Pi-calibrated frame budget:

**Target frame budget for Pi 4 at 10 Hz:**

| Stage             | Budget (ms) | Notes                                                      |
| ----------------- | ----------- | ---------------------------------------------------------- |
| Polar conversion  | 2           | Linear in points; already fast                             |
| Background update | 15          | 72K cell grid; dominated by memory bandwidth               |
| Ground filter     | 2           | Single-pass height cull                                    |
| DBSCAN            | 25          | Largest variable; depends on `foreground_max_input_points` |
| Tracking          | 8           | 32 tracks × Kalman update                                  |
| Serialisation     | 3           | gRPC publish (if enabled)                                  |
| **Total**         | **55**      | Must stay under 100 ms (10 fps)                            |
| **Headroom**      | **45**      | For GC, OS scheduling, SD card I/O                         |

### C.3 monitoring on Pi

Since the Pi has no display, the benchmark harness serves as the primary performance monitoring tool. Additionally:

- The existing `/api/lidar/scene-health` HTTP endpoint can expose `FrameStageTiming` aggregates (p50/p95) for remote monitoring.
- A new `--perf-log-interval` flag (default: 0 = disabled) on the radar binary dumps `FrameStageTiming` summary every N seconds to opsf, providing lightweight production monitoring without full tracef overhead.

---

## Part d: implementation plan

### Phase 1: pipeline stage timing (low risk, high value)

1. Add `FrameStageTiming` struct to `pipeline` package.
2. Instrument `NewFrameCallback()` with `time.Now()` checkpoints.
3. Emit `tracef` per-frame and `diagf` summary every 100 frames.
4. Wire timing into VRLOG `DebugOverlaySet`.

**Files:** [internal/lidar/pipeline/tracking_pipeline.go](../../internal/lidar/pipeline/tracking_pipeline.go), `internal/lidar/visualiser/model.go`

### Phase 2: clustering metrics in pcap-analyse (medium risk)

1. Add `ClusteringMetrics` to `PerformanceMetrics`.
2. Collect per-frame foreground counts, cluster counts, DBSCAN timings.
3. Extend baseline format to v2 with `clustering` key.
4. Add comparison logic with per-metric thresholds.
5. Update `baseline-kirk0.json` and `baseline-kirk0-ci.json`.

**Files:** [cmd/tools/pcap-analyse/main.go](../../cmd/tools/pcap-analyse/main.go), `internal/lidar/perf/baseline/*.json`

### Phase 3: per-track association logging (low risk)

1. Add `tracef` logging in `Tracker.Update()` for association decisions.
2. Add `diagf` track lifecycle summary on deletion.
3. Optionally populate `DebugOverlaySet.AssociationDecisions` in VRLOG.

**Files:** [internal/lidar/l5tracks/tracking.go](../../internal/lidar/l5tracks/tracking.go), `internal/lidar/visualiser/adapter.go`

### Phase 4: Go micro-benchmarks (low risk)

1. Create `cluster_benchmark_test.go` with DBSCAN point-count scaling benchmarks.
2. Add `bench-go` Makefile target.
3. Consider adding to nightly CI with `benchstat` comparison.

**Files:** `internal/lidar/l4perception/cluster_benchmark_test.go`, `Makefile`

### Phase 5: CI integration (medium risk)

1. Add `bench-clustering` job to `go-ci.yml`.
2. Store PCAP fixtures in Git LFS (already done for `kirk0.pcapng`).
3. Set regression thresholds, tune after 2–3 weeks of data.

**Files:** [.github/workflows/go-ci.yml](../../.github/workflows/go-ci.yml), `Makefile`

### Phase 6: Pi baseline (requires hardware)

1. Cross-compile and deploy to Pi 4.
2. Run `make test-perf NAME=kirk0 PROFILE=pi`.
3. Check in `baseline-kirk0-pi.json`.
4. Document tuning recommendations in [config/CONFIG.md](../../config/CONFIG.md).

**Files:** `internal/lidar/perf/baseline/baseline-kirk0-pi.json`, [config/CONFIG.md](../../config/CONFIG.md)

---

## Open questions

1. **VRLOG size growth**: Embedding `FrameStageTiming` per frame adds ~200 bytes/frame (~600 KB for a 5-minute, 10 Hz capture). Acceptable?
2. **Structured logging format**: Should we migrate tracef/diagf from plain-text `log.Logger` to JSON for machine-parseable diagnostics? Or keep human-readable and parse with grep?
3. **Benchmark fixture management**: `kirk0.pcapng` is 191 MB in LFS. Do we need a smaller synthetic fixture for faster CI runs?
4. **statistics_json column**: `RunStatistics` from `l6objects/quality.go` is computed but never written to the `lidar_analysis_runs` table. Should Phase 2 wire this up?
