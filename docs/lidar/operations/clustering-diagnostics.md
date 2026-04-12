# Clustering diagnostics

Observability metrics and benchmark harness for the LiDAR clustering and tracking pipeline. Covers per-frame timing, per-track association diagnostics, cluster quality fields, benchmark regression gating, and Raspberry Pi tuning levers.

## Source

- Plan: `docs/plans/lidar-clustering-observability-and-benchmark-plan.md`
- Status: Proposed (February 2026)
- Layers: L4 Perception, L8 Analytics

## Problem

Two classes of problem lack tooling:

1. **Trail glitches**: tracks that repeatedly exhibit heading flips, speed jitter, merge/split artefacts, or dropout gaps. The pipeline currently logs aggregate counts (`N confirmed tracks active`) but nothing about _which_ cluster was associated, how many points it had, what the innovation residual was, or whether the cluster was subsampled.

2. **Performance blind spot**: the `pcap-analyse -benchmark` harness captures wall-clock timing and throughput, but has no clustering-specific metrics (cluster count distribution, points-per-cluster, DBSCAN grid utilisation, subsampling frequency). Stepping from Mac M1 to Raspberry Pi 4 requires fine-grained levers for trading detection quality against latency, plus CI regression gates.

## Part a: per-track / per-frame observability

### A.1 per-frame pipeline stage timing

Instrument `NewFrameCallback()` in `internal/lidar/pipeline/tracking_pipeline.go` with `time.Now()` checkpoints at each stage boundary. Collect as a `FrameStageTiming` struct:

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

**Logging levels:**

- `tracef`: single structured line per frame
- `diagf`: summary every 100 frames (averages)

**VRLOG embedding:** Add `FrameStageTiming` to `FrameBundle.DebugOverlaySet` as an optional field so timing data is available during offline replay analysis.

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
| `ClusterPoints`   | `int`     | Points in the associated cluster        |
| `ClusterArea`     | `float32` | OBB area (length × width)               |
| `HistoricalArea`  | `float32` | Track's running average area            |
| `AreaRatio`       | `float32` | Current / historical (merge/split flag) |
| `HeadingDeltaDeg` | `float32` | Cluster heading − track heading         |
| `SpeedDeltaMps`   | `float32` | Implied speed − Kalman speed            |

**Glitch trail diagnosis:** A jittering track typically exhibits oscillating `AreaRatio` (merge/split cycles), high `HeadingDeltaDeg`, `MahalDistance` near the gating boundary, and low `ClusterPoints` (sparse cluster = unstable OBB).

### A.3 per-track lifecycle summary

On track deletion, emit a `diagf`-level summary including duration, length, observation count, max misses, occlusions, heading jitter, speed jitter, merge/split events, alignment, class, and confidence. All fields already exist on `TrackedObject`.

### A.4 cluster quality metrics

Add to `WorldCluster` in `internal/lidar/l4perception/types.go`:

| Field                     | Type      | Description                                |
| ------------------------- | --------- | ------------------------------------------ |
| `DBSCANNeighboursVisited` | `int`     | Neighbour queries during cluster expansion |
| `PointDensity`            | `float32` | Points per m² of OBB area                  |
| `RangeMean`               | `float32` | Mean range of cluster points from sensor   |
| `RangeSpread`             | `float32` | Max − min range within cluster             |

`PointDensity` reveals sparse long-range clusters likely to produce jittery tracks. `RangeMean` allows stratifying benchmark metrics by distance band.

### A.5 VRLOG diagnostic channel

Proposed additions to `DebugOverlaySet` (all optional, populated only when debug logging is active):

- `FrameStageTiming`: per-frame pipeline timing (A.1)
- `ClusterDiagnostics []ClusterDiag`: per-cluster quality (A.4)
- `AssociationDecisions []AssociationDecision`: full per-track association log (A.2)

## Part b: clustering performance benchmark harness

### B.1 new metrics in pcap-analyse

Extend `PerformanceMetrics` in `cmd/tools/pcap-analyse/main.go` with `ClusteringMetrics`:

- Per-frame distributions (percentiles): foreground points, filtered points, cluster count, points-per-cluster, cluster area
- DBSCAN-specific: call count, total/avg/p95/p99 time, subsample count and rate
- Tracking-derived quality proxies: confirmed/tentative track count, fragmentation ratio, mean track duration/length, heading jitter RMS, speed jitter RMS
- Memory: spatial index peak cells

All distributions use a `DistributionStats` struct: min, max, avg, p50, p95, p99, samples.

### B.2 benchmark baseline format (v2)

Extend the existing `baseline-{name}.json` format with a `clustering` key. Backward-compatible: comparison logic skips missing keys.

### B.3 CI regression gating

Add a `bench-clustering` step to `.github/workflows/go-ci.yml` running only on pushes to `main` and PRs modifying `internal/lidar/l4perception/`, `l5tracks/`, or `pipeline/`.

**Regression thresholds:**

| Metric                  | Warning (exit 0) | Regression (exit 1) |
| ----------------------- | ---------------- | ------------------- |
| `wall_clock_ms`         | >10%             | >20%                |
| `dbscan_p99_us`         | >15%             | >30%                |
| `subsample_rate_pct`    | absolute >5%     | absolute >15%       |
| `fragmentation_ratio`   | >0.05            | >0.15               |
| `confirmed_track_count` | < -10%           | < -25%              |

### B.4 checked-in baselines

Store baselines as checked-in JSON files:

```
internal/lidar/perf/baseline/
├── baseline-kirk0.json           # local Mac ARM64 baseline
├── baseline-kirk0-ci.json        # CI (Linux x86_64) baseline
├── baseline-kirk0-pi.json        # Raspberry Pi ARM64 baseline (manual)
└── baseline-lidar_20Hz.json      # second fixture
```

### B.5 Go micro-benchmarks

Targeted benchmarks in `internal/lidar/l4perception/cluster_benchmark_test.go`:

- `BenchmarkDBSCAN_{500,2000,5000,8000}pts`: DBSCAN scaling by point count
- `BenchmarkSpatialIndex{Build,Query}_5000pts`: spatial index operations
- `BenchmarkUniformSubsample_10000to8000`: subsampling overhead

## Part c: Raspberry Pi performance tuning levers

### Tuning parameters with Pi impact

| Parameter                       | Default | Pi Recommendation | Impact                                           |
| ------------------------------- | ------- | ----------------- | ------------------------------------------------ |
| `foreground_max_input_points`   | 8000    | 2000–4000         | Caps DBSCAN input; O(n·k) improvement            |
| `foreground_dbscan_eps`         | 1.0     | 1.5–2.0           | Larger eps → fewer grid cells, faster queries    |
| `foreground_min_cluster_points` | 3       | 5                 | Fewer tiny clusters → less tracking overhead     |
| `max_frame_rate`                | 25      | 10–12             | Drop frames early; biggest single lever          |
| `max_tracks`                    | 64      | 32                | Caps association matrix size                     |
| `voxel_leaf_size`               | 0 (off) | 0.15              | 60–70% point reduction before DBSCAN             |
| `remove_ground`                 | true    | true              | Ground filter is cheap; keeps DBSCAN input small |

### Pi frame budget at 10 hz

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

### Pi monitoring

- `/api/lidar/scene-health` HTTP endpoint can expose `FrameStageTiming` aggregates (p50/p95)
- `--perf-log-interval` flag (default disabled) dumps timing summary every N seconds to `opsf`

## Open questions

1. **VRLOG size growth**: Embedding `FrameStageTiming` per frame adds ~200 bytes/frame (~600 KB for a 5-minute, 10 Hz capture). Acceptable?
2. **Structured logging format**: Migrate tracef/diagf to JSON for machine parsing, or keep human-readable and parse with grep?
3. **Benchmark fixture management**: `kirk0.pcapng` is 191 MB in LFS. Needed smaller synthetic fixture for faster CI runs?
4. **statistics_json column**: `RunStatistics` is computed but never written to `lidar_analysis_runs`. Wire up in Phase 2?
