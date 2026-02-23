# Design: Clustering Observability Metrics and Performance Benchmark Harness

**Status:** Proposed (February 2026)
**Related:** [Clustering Maths](../maths/clustering-maths.md), [Performance and Scene Health Metrics](lidar-visualiser-performance-and-scene-health-timeline-metrics-plan.md), [Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md)

## Problem

Two classes of problem lack tooling:

1. **Trail glitches** — tracks that repeatedly exhibit heading flips, speed jitter, merge/split artefacts, or dropout gaps on the same stretch of road. We cannot diagnose _why_ without per-frame, per-track clustering diagnostics. The pipeline currently logs aggregate counts (`N confirmed tracks active`) but nothing about _which_ cluster was associated, how many points it had, what the innovation residual was, or whether the cluster was subsampled.

2. **Performance blind spot** — the `pcap-analyse -benchmark` harness captures wall-clock timing and throughput, but has no clustering-specific metrics (cluster count distribution, points-per-cluster, DBSCAN grid utilisation, subsampling frequency). When stepping down from a Mac M1 (8-core, 16 GB) to a Raspberry Pi 4 (4-core, 4 GB) we need fine-grained levers for trading detection quality against latency, and CI regression gates to catch regressions before deployment.

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

## Part A: Per-Track / Per-Frame Observability

### A.1 Per-Frame Pipeline Stage Timing

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

**Logging:** At `tracef` level, emit a single structured line per frame:

```
[Pipeline] frame=%s total_us=%d bg_us=%d cluster_us=%d track_us=%d fg=%d clusters=%d tracks=%d subsampled=%t
```

At `diagf` level, emit a summary every 100 frames:

```
[Pipeline] 100-frame avg: bg=%.1fms cluster=%.1fms track=%.1fms fg_pts=%.0f clusters=%.1f tracks=%.1f subsample_rate=%.1f%%
```

**VRLOG embedding:** Add `FrameStageTiming` to `FrameBundle.DebugOverlaySet` as an optional field. This makes timing data available during offline replay analysis without requiring the debug log.

### A.2 Per-Track Association Diagnostics

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

### A.3 Per-Track Lifecycle Summary

On track deletion, emit a `diagf`-level summary:

```
[Track] id=%s state=%s duration=%.1fs length=%.1fm obs=%d misses_max=%d occlusions=%d heading_jitter=%.1f° speed_jitter=%.1fm/s merge_events=%d split_events=%d alignment_mean=%.1f° class=%s conf=%.2f
```

All fields already exist on `TrackedObject`. This is a formatting change, not new data collection.

### A.4 Cluster Quality Metrics (new fields on WorldCluster)

Add to `WorldCluster` in `internal/lidar/l4perception/types.go`:

| Field                     | Type      | Description                                |
| ------------------------- | --------- | ------------------------------------------ |
| `DBSCANNeighboursVisited` | `int`     | Neighbour queries during cluster expansion |
| `PointDensity`            | `float32` | Points per m² of OBB area                  |
| `RangeMean`               | `float32` | Mean range of cluster points from sensor   |
| `RangeSpread`             | `float32` | Max − min range within cluster             |

`PointDensity` reveals sparse long-range clusters likely to produce jittery tracks. `RangeMean` allows stratifying benchmark metrics by distance band.

### A.5 VRLOG Diagnostic Channel

Current `DebugOverlaySet` fields in `FrameBundle`:

- `AssociationCandidate` — cluster↔track distances
- `GatingEllipse` — semi-major/minor/rotation
- `InnovationResidual` — Kalman innovation
- `StatePrediction` — predicted state

Proposed additions (all optional, populated only when debug logging is active):

- `FrameStageTiming` — per-frame pipeline timing (Section A.1)
- `ClusterDiagnostics []ClusterDiag` — per-cluster quality (Section A.4)
- `AssociationDecisions []AssociationDecision` — full per-track association log (Section A.2)

This keeps the VRLOG self-contained: when replaying a recording, all diagnostic context is available without needing the original text log.

---

## Part B: Clustering Performance Benchmark Harness

### B.1 New Metrics in `pcap-analyse`

Extend `PerformanceMetrics` in `cmd/tools/pcap-analyse/main.go`:

```go
type ClusteringMetrics struct {
    // Per-frame distributions (percentiles computed from all frames)
    ForegroundPointsStats  DistributionStats `json:"foreground_points_stats"`
    FilteredPointsStats    DistributionStats `json:"filtered_points_stats"`
    ClusterCountStats      DistributionStats `json:"cluster_count_stats"`
    PointsPerClusterStats  DistributionStats `json:"points_per_cluster_stats"`
    ClusterAreaStats       DistributionStats `json:"cluster_area_stats"`

    // DBSCAN-specific
    DBSCANCallCount        int64   `json:"dbscan_call_count"`
    DBSCANTotalUs          int64   `json:"dbscan_total_us"`
    DBSCANAvgUs            float64 `json:"dbscan_avg_us"`
    DBSCANP95Us            float64 `json:"dbscan_p95_us"`
    DBSCANP99Us            float64 `json:"dbscan_p99_us"`
    SubsampleCount         int64   `json:"subsample_count"`           // frames where MaxInputPoints was exceeded
    SubsampleRate          float64 `json:"subsample_rate_pct"`        // % of frames subsampled

    // Tracking-derived (quality proxies)
    ConfirmedTrackCount    int     `json:"confirmed_track_count"`
    TentativeTrackCount    int     `json:"tentative_track_count"`
    FragmentationRatio     float64 `json:"fragmentation_ratio"`       // tentative / (tentative + confirmed)
    MeanTrackDurationSecs  float64 `json:"mean_track_duration_secs"`
    MeanTrackLengthMeters  float64 `json:"mean_track_length_meters"`
    HeadingJitterRMSDeg    float64 `json:"heading_jitter_rms_deg"`
    SpeedJitterRMSMps      float64 `json:"speed_jitter_rms_mps"`

    // Memory (clustering-specific)
    SpatialIndexPeakCells  int64   `json:"spatial_index_peak_cells"`  // max grid cells in any frame
}

type DistributionStats struct {
    Min     float64 `json:"min"`
    Max     float64 `json:"max"`
    Avg     float64 `json:"avg"`
    P50     float64 `json:"p50"`
    P95     float64 `json:"p95"`
    P99     float64 `json:"p99"`
    Samples int64   `json:"samples"`
}
```

### B.2 Benchmark Baseline Format (v2)

Extend the existing `baseline-{name}.json` format with a `clustering` key. This is backward-compatible — the comparison logic skips missing keys.

```jsonc
{
  "version": "2.0",
  "timestamp": "2026-02-22T23:00:00Z",
  "pcap_file": "kirk0.pcapng",
  "system_info": {
    /* unchanged */
  },
  "metrics": {
    /* existing fields: wall_clock_ms, frame_time_stats, etc. */
    "clustering": {
      "foreground_points_stats": {
        "min": 0,
        "max": 8200,
        "avg": 1450,
        "p50": 1200,
        "p95": 3800,
        "p99": 6100,
        "samples": 2604,
      },
      "cluster_count_stats": {
        "min": 0,
        "max": 42,
        "avg": 18.3,
        "p50": 17,
        "p95": 35,
        "p99": 40,
        "samples": 2604,
      },
      "points_per_cluster_stats": {
        "min": 3,
        "max": 620,
        "avg": 78,
        "p50": 55,
        "p95": 280,
        "p99": 450,
        "samples": 47820,
      },
      "dbscan_call_count": 2604,
      "dbscan_total_us": 312000,
      "dbscan_avg_us": 119.8,
      "dbscan_p95_us": 450.0,
      "dbscan_p99_us": 1200.0,
      "subsample_count": 3,
      "subsample_rate_pct": 0.12,
      "confirmed_track_count": 312,
      "fragmentation_ratio": 0.23,
      "heading_jitter_rms_deg": 4.2,
      "speed_jitter_rms_mps": 0.8,
    },
  },
}
```

### B.3 CI Regression Gating

Add a `bench-clustering` step to `.github/workflows/go-ci.yml` (runs only on pushes to `main` and PRs that modify `internal/lidar/l4perception/`, `internal/lidar/l5tracks/`, or `internal/lidar/pipeline/`):

```yaml
bench-clustering:
  runs-on: ubuntu-latest
  needs: [build]
  if: >-
    contains(github.event.head_commit.modified, 'internal/lidar/l4perception') ||
    contains(github.event.head_commit.modified, 'internal/lidar/l5tracks') ||
    contains(github.event.head_commit.modified, 'internal/lidar/pipeline')
  steps:
    - uses: actions/checkout@v4
      with: { lfs: true }
    - uses: actions/setup-go@v5
      with: { go-version-file: go.mod }
    - run: sudo apt-get install -y libpcap-dev
    - run: make test-perf NAME=kirk0
    # test-perf exits 1 on regression (>15% wall-clock or >25% DBSCAN p99 regression)
```

**Regression thresholds** (configurable via `pcap-analyse` flags):

| Metric                  | Warning (exit 0) | Regression (exit 1) |
| ----------------------- | ---------------- | ------------------- |
| `wall_clock_ms`         | >10%             | >20%                |
| `dbscan_p99_us`         | >15%             | >30%                |
| `subsample_rate_pct`    | absolute >5%     | absolute >15%       |
| `fragmentation_ratio`   | >0.05            | >0.15               |
| `confirmed_track_count` | < -10%           | < -25%              |

### B.4 Checked-in Benchmark Log

Store baselines as checked-in JSON files (existing pattern):

```
internal/lidar/perf/baseline/
├── baseline-kirk0.json           # local Mac ARM64 baseline
├── baseline-kirk0-ci.json        # CI (Linux x86_64) baseline
├── baseline-kirk0-pi.json        # Raspberry Pi ARM64 baseline (manual)
└── baseline-lidar_20Hz.json      # second fixture
```

**Makefile targets:**

```makefile
bench-clustering:                  ## Run clustering benchmark (local)
    @make test-perf NAME=kirk0

bench-clustering-save:             ## Save new baseline (local)
    @make test-perf-save NAME=kirk0

bench-clustering-ci:               ## Run clustering benchmark (CI profile)
    @make test-perf NAME=kirk0 PROFILE=ci

bench-clustering-pi:               ## Run clustering benchmark (Pi profile)
    @make test-perf NAME=kirk0 PROFILE=pi
```

The `PROFILE` variable selects the baseline file suffix (`-ci`, `-pi`) for comparison.

### B.5 Go Micro-Benchmarks (new)

Add targeted benchmarks in `internal/lidar/l4perception/cluster_benchmark_test.go`:

```go
func BenchmarkDBSCAN_500pts(b *testing.B)  { benchDBSCAN(b, 500) }
func BenchmarkDBSCAN_2000pts(b *testing.B) { benchDBSCAN(b, 2000) }
func BenchmarkDBSCAN_5000pts(b *testing.B) { benchDBSCAN(b, 5000) }
func BenchmarkDBSCAN_8000pts(b *testing.B) { benchDBSCAN(b, 8000) }

func BenchmarkSpatialIndexBuild_5000pts(b *testing.B) { benchSIBuild(b, 5000) }
func BenchmarkSpatialIndexQuery_5000pts(b *testing.B) { benchSIQuery(b, 5000) }

func BenchmarkUniformSubsample_10000to8000(b *testing.B) { benchSubsample(b, 10000, 8000) }
```

These provide isolated signal for DBSCAN algorithmic performance independent of the full pipeline.

Add a Makefile target:

```makefile
bench-go:                          ## Run all Go micro-benchmarks
    go test -bench=. -benchmem -count=3 ./internal/lidar/l4perception/ \
        ./internal/lidar/l5tracks/ ./internal/lidar/l3grid/ | tee bench-results.txt
```

---

## Part C: Raspberry Pi Performance Tuning Levers

### C.1 Tuning Parameters with Pi Impact

These parameters in `config/tuning.defaults.json` directly affect computational cost. Documented here for operators deploying on Pi:

| Parameter                       | Default | Pi Recommendation | Impact                                           |
| ------------------------------- | ------- | ----------------- | ------------------------------------------------ |
| `foreground_max_input_points`   | 8000    | 2000–4000         | Caps DBSCAN input; O(n·k) improvement            |
| `foreground_dbscan_eps`         | 1.0     | 1.5–2.0           | Larger eps → fewer grid cells, faster queries    |
| `foreground_min_cluster_points` | 3       | 5                 | Fewer tiny clusters → less tracking overhead     |
| `max_frame_rate`                | 25      | 10–12             | Drop frames early; biggest single lever          |
| `max_tracks`                    | 64      | 32                | Caps association matrix size                     |
| `voxel_leaf_size`               | 0 (off) | 0.15              | 60–70% point reduction before DBSCAN             |
| `remove_ground`                 | true    | true              | Ground filter is cheap; keeps DBSCAN input small |

### C.2 Pi-Specific Benchmark Profile

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

### C.3 Monitoring on Pi

Since the Pi has no display, the benchmark harness serves as the primary performance monitoring tool. Additionally:

- The existing `/api/lidar/scene-health` HTTP endpoint can expose `FrameStageTiming` aggregates (p50/p95) for remote monitoring.
- A new `--perf-log-interval` flag (default: 0 = disabled) on the radar binary dumps `FrameStageTiming` summary every N seconds to opsf, providing lightweight production monitoring without full tracef overhead.

---

## Part D: Implementation Plan

### Phase 1: Pipeline Stage Timing (low risk, high value)

1. Add `FrameStageTiming` struct to `pipeline` package.
2. Instrument `NewFrameCallback()` with `time.Now()` checkpoints.
3. Emit `tracef` per-frame and `diagf` summary every 100 frames.
4. Wire timing into VRLOG `DebugOverlaySet`.

**Files:** `internal/lidar/pipeline/tracking_pipeline.go`, `internal/lidar/visualiser/model.go`

### Phase 2: Clustering Metrics in pcap-analyse (medium risk)

1. Add `ClusteringMetrics` to `PerformanceMetrics`.
2. Collect per-frame foreground counts, cluster counts, DBSCAN timings.
3. Extend baseline format to v2 with `clustering` key.
4. Add comparison logic with per-metric thresholds.
5. Update `baseline-kirk0.json` and `baseline-kirk0-ci.json`.

**Files:** `cmd/tools/pcap-analyse/main.go`, `internal/lidar/perf/baseline/*.json`

### Phase 3: Per-Track Association Logging (low risk)

1. Add `tracef` logging in `Tracker.Update()` for association decisions.
2. Add `diagf` track lifecycle summary on deletion.
3. Optionally populate `DebugOverlaySet.AssociationDecisions` in VRLOG.

**Files:** `internal/lidar/l5tracks/tracking.go`, `internal/lidar/visualiser/adapter.go`

### Phase 4: Go Micro-Benchmarks (low risk)

1. Create `cluster_benchmark_test.go` with DBSCAN point-count scaling benchmarks.
2. Add `bench-go` Makefile target.
3. Consider adding to nightly CI with `benchstat` comparison.

**Files:** `internal/lidar/l4perception/cluster_benchmark_test.go`, `Makefile`

### Phase 5: CI Integration (medium risk)

1. Add `bench-clustering` job to `go-ci.yml`.
2. Store PCAP fixtures in Git LFS (already done for `kirk0.pcapng`).
3. Set regression thresholds, tune after 2–3 weeks of data.

**Files:** `.github/workflows/go-ci.yml`, `Makefile`

### Phase 6: Pi Baseline (requires hardware)

1. Cross-compile and deploy to Pi 4.
2. Run `make test-perf NAME=kirk0 PROFILE=pi`.
3. Check in `baseline-kirk0-pi.json`.
4. Document tuning recommendations in `config/README.md`.

**Files:** `internal/lidar/perf/baseline/baseline-kirk0-pi.json`, `config/README.md`

---

## Open Questions

1. **VRLOG size growth** — Embedding `FrameStageTiming` per frame adds ~200 bytes/frame (~600 KB for a 5-minute, 10 Hz capture). Acceptable?
2. **Structured logging format** — Should we migrate tracef/diagf from plain-text `log.Logger` to JSON for machine-parseable diagnostics? Or keep human-readable and parse with grep?
3. **Benchmark fixture management** — `kirk0.pcapng` is 191 MB in LFS. Do we need a smaller synthetic fixture for faster CI runs?
4. **statistics_json column** — `RunStatistics` from `l6objects/quality.go` is computed but never written to the `lidar_analysis_runs` table. Should Phase 2 wire this up?
