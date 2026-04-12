# Dynamic algorithm selection for LiDAR foreground extraction

- **Status:** Branch-History Design Specification (Not Active on `main` Runtime)
- **Layers:** L3 Grid, L4 Perception
- **Source Branch:** `copilot/summarize-changes-and-spec` (34 commits, 54 files, ~7,300 lines)
- **Related:**
- **Canonical:** [pluggable-algorithm-selection.md](../lidar/architecture/pluggable-algorithm-selection.md)

- [`docs/plans/lidar-velocity-coherent-foreground-extraction-plan.md`](../plans/lidar-velocity-coherent-foreground-extraction-plan.md): Original design vision
- [Backlog](../BACKLOG.md): milestone placement
- [LiDAR Pipeline Reference](../lidar/architecture/lidar-pipeline-reference.md): metrics-first pipeline phases

---

## 1. Executive summary

This document specifies the design for **pluggable foreground extraction algorithms** in the LiDAR tracking pipeline. The work enables runtime switching between background subtraction (existing), velocity-coherent extraction (new), and hybrid approaches: supporting A/B evaluation and gradual algorithm migration.

Current runtime note (2026-02-21): the production pipeline on `main` still uses `ProcessFramePolarWithMask` in [internal/lidar/l3grid/foreground.go](../../internal/lidar/l3grid/foreground.go); this document should be treated as implementation guidance, not implemented-state documentation.

### Motivation

The current background-subtraction algorithm (`ProcessFramePolarWithMask`) produces "foreground trails": persistent false-positive foreground points behind vehicles after they pass. Root cause: the EMA-based background model takes time to reconverge after freeze expiry. A velocity-coherent approach eliminates trails by detecting motion rather than background deviation, but needs comparison infrastructure before replacing the proven algorithm.

### What was built (branch summary)

The branch implemented four phases of work:

| Phase | Description                                          | Files                                                                          | Status   |
| ----- | ---------------------------------------------------- | ------------------------------------------------------------------------------ | -------- |
| **A** | `ForegroundExtractor` interface + background adapter | `extractor.go`, `extractor_background.go`                                      | Complete |
| **B** | Velocity-coherent extractor + frame history          | `extractor_velocity_coherent.go`, `frame_history.go`, `velocity_estimation.go` | Complete |
| **C** | Hybrid extractor + evaluation harness                | `extractor_hybrid.go`, `evaluation_harness.go`                                 | Complete |
| **D** | Pipeline integration + API + CLI tool                | `tracking_pipeline.go`, `webserver.go`, `algo-compare/main.go`                 | Complete |

Additionally: bug fixes to foreground extraction (recFg accumulation, thaw reset, locked baseline), PCAP debug tooling (grid plotter, debug range filtering), track quality metrics, analysis run manager, and database migrations for algorithm comparison results.

### What already landed on `main`

Several bug fixes and foundational changes from this branch were separately cherry-picked or independently reimplemented on `main`:

- ✅ `isNilInterface()` utility function
- ✅ Thaw grace period constant (`ThawGracePeriodNanos`)
- ✅ Locked baseline parameters (`LockedBaselineThreshold`, `LockedBaselineMultiplier`)
- ✅ Locked baseline fields on `BackgroundCell`
- ✅ `AnalysisRunManager` and registry
- ✅ Track quality metrics on `TrackedObject`
- ✅ `quality.go` (RunStatistics)
- ✅ `track_export.go` (TrackPointCloudExporter)
- ✅ `analysis_run_manager.go`
- ✅ Grid plotter (`monitor/gridplotter.go`)
- ✅ Version package ([internal/version/version.go](../../internal/version/version.go))
- ✅ Foreground freeze/thaw fixes in `foreground.go`

### What needs to be applied to `main`

The following features are **NOT** on `main` and need to be re-implemented:

1. **`ForegroundExtractor` interface** (`extractor.go`)
2. **Background subtraction adapter** (`extractor_background.go`)
3. **Velocity-coherent extractor** (`extractor_velocity_coherent.go`, `frame_history.go`, `velocity_estimation.go`)
4. **Hybrid extractor** (`extractor_hybrid.go`)
5. **Evaluation harness** (`evaluation_harness.go`)
6. **`TrackingPipeline` wrapper** with dynamic algorithm switching (additions to `tracking_pipeline.go`)
7. **Algorithm selection API** (`/api/lidar/algorithm` endpoint in `webserver.go`)
8. **Algorithm comparison CLI** (`cmd/tools/algo-compare/main.go`)
9. **Migration 013**: `lidar_algorithm_runs` and `lidar_algorithm_frame_results` tables
10. **Tests** (`extractor_test.go`, `tracking_pipeline_logic_test.go`, `webserver_algo_test.go`)

---

## 2. Architecture

### 2.1 ForegroundExtractor interface

**`ForegroundExtractor` interface** (in `internal/lidar/extractor.go`):

| Method           | Signature                                                                                                 | Notes                                                                         |
| ---------------- | --------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| `Name()`         | `string`                                                                                                  | Algorithm identifier                                                          |
| `ProcessFrame()` | `(points []PointPolar, timestamp time.Time) (foregroundMask []bool, metrics ExtractorMetrics, err error)` | Returns `[]bool` mask (same length as input), preserving index correspondence |
| `GetParams()`    | `map[string]interface{}`                                                                                  | Current parameter snapshot                                                    |
| `SetParams()`    | `(params map[string]interface{}) error`                                                                   | Runtime parameter tuning via API                                              |
| `Reset()`        |                                                                                                           | Enables PCAP replay restart without recreating extractors                     |

**Design decisions:**

- Returns `[]bool` foreground mask (same length as input points), not filtered point slices: preserves index correspondence for downstream processing
- `ExtractorMetrics` carries algorithm-agnostic counts plus `AlgorithmSpecific` map for algorithm-specific data
- `Reset()` enables PCAP replay restart without recreating extractors
- `SetParams()` enables runtime parameter tuning via API

### 2.2 Extractor implementations

#### BackgroundSubtractorExtractor (`extractor_background.go`, ~200 lines)

Wraps existing `BackgroundManager.ProcessFramePolarWithMask()` to conform to the `ForegroundExtractor` interface. Zero-copy adapter: delegates entirely to the existing code path.

**`BackgroundSubtractorExtractor` struct** — wraps existing `BackgroundManager.ProcessFramePolarWithMask()`. Fields: `Manager` (`*BackgroundManager`), `SensorID` (string). Zero-copy adapter.

#### VelocityCoherentExtractor (`extractor_velocity_coherent.go`, ~240 lines)

Implements motion-based foreground extraction using frame-to-frame point correspondence:

1. Convert polar points to world coordinates with velocity metadata (`PointWithVelocity`)
2. Estimate per-point velocities via spatial correspondence with previous frame
3. Run DBSCAN with reduced `MinPts=3` (velocity coherence confirms cluster identity)
4. Filter clusters by velocity coherence score
5. Mark points in coherent clusters as foreground

**`VelocityCoherentConfig` struct:**

| Field                  | Type                     | Default | Purpose                                                |
| ---------------------- | ------------------------ | ------- | ------------------------------------------------------ |
| `VelocityEstimation`   | VelocityEstimationConfig |         | Nested config                                          |
| `DBSCANEps`            | float64                  | 0.6 m   | Clustering radius                                      |
| `DBSCANMinPts`         | int                      | 3       | Reduced from 12 (velocity coherence confirms identity) |
| `MinVelocityCoherence` | float64                  | 0.3     | Minimum coherence score                                |
| `MinVelocityPoints`    | int                      | 2       | Minimum velocity-confirmed points                      |
| `FrameHistoryCapacity` | int                      | 10      | Circular buffer size                                   |

**Dependencies:**

- `frame_history.go`: circular buffer of `VelocityFrame` with spatial index
- `velocity_estimation.go`: point correspondence and velocity estimation using `SpatialIndex`

#### HybridExtractor (`extractor_hybrid.go`, ~250 lines)

Runs multiple extractors in parallel and merges results:

**`HybridExtractor` struct** — fields: `Config` (HybridExtractorConfig), `Extractors` ([]ForegroundExtractor), `SensorID` (string). Runs multiple extractors in parallel and merges results.

**Merge modes:**

- `union`: OR merge (maximum detection coverage, may increase false positives)
- `intersection`: AND merge (maximum precision, may miss sparse objects)
- `primary`: Use first extractor, ignore others (for metrics collection without affecting output)

### 2.3 Mask merge utilities (`extractor.go`)

Utility functions: `MergeForegroundMasks(masks [][]bool, mode MergeMode) []bool`, `CountForeground(mask []bool) int`, `ComputeMaskAgreement(mask1, mask2 []bool) float64`, `ComputePrecisionRecall(predicted, groundTruth []bool) (precision, recall float64)`.

### 2.4 Velocity estimation (`velocity_estimation.go`, ~420 lines)

Per-point velocity estimation via frame-to-frame correspondence:

**`VelocityEstimationConfig` struct:**

| Field                  | Type    | Default | Purpose                                    |
| ---------------------- | ------- | ------- | ------------------------------------------ |
| `SearchRadius`         | float64 | 2.0 m   | Max correspondence search distance         |
| `MaxVelocityMps`       | float64 | 50.0    | ~180 km/h limit                            |
| `VelocityWeight`       | float64 | 2.0     | Weight for velocity consistency in scoring |
| `MinConfidence`        | float32 | 0.3     | Minimum confidence threshold               |
| `SpatialIndexCellSize` | float64 | 0.6 m   | Matches DBSCAN eps                         |

**Algorithm:**

1. Build spatial index for previous frame
2. For each current point, find candidates within `SearchRadius`
3. Score candidates by distance + velocity consistency with neighbours
4. Select best correspondence, compute velocity vector
5. Assign confidence based on match quality and neighbour consistency

### 2.5 Frame history (`frame_history.go`, ~190 lines)

Circular buffer of processed frames for multi-frame correspondence:

**`FrameHistory` struct** — circular buffer of `*VelocityFrame` with `capacity`, `head`, and `size` fields.

**`VelocityFrame` struct** — fields: `Points` ([]PointWithVelocity), `SpatialIndex` (\*SpatialIndex), `Timestamp` (time.Time), `FrameID` (string).

### 2.6 Evaluation harness (`evaluation_harness.go`, ~310 lines)

Runs multiple extractors on the same frames and collects comparison metrics:

**`EvaluationHarness` struct** — fields: `Config` (EvaluationHarnessConfig), `Extractors` ([]ForegroundExtractor), `PerExtractorStats` (map[string]*ExtractorStats), `ComparisonBuffer` ([]*FrameComparison, ring buffer). Supports optional `GroundTruthProvider` interface for precision/recall computation.

Supports optional `GroundTruthProvider` interface for precision/recall computation.

---

## 3. Pipeline integration

### 3.1 TrackingPipeline wrapper

**On `main`:** `TrackingPipelineConfig.NewFrameCallback()` returns a closure.
**This design adds:** A `TrackingPipeline` struct that wraps `TrackingPipelineConfig` with dynamic algorithm selection.

> **Important (main compatibility):** On `main`, `TrackingPipelineConfig` has been significantly expanded with `VisualiserPublisher`, `VisualiserAdapter`, `LidarViewAdapter`, `MaxFrameRate`, `VoxelLeafSize`, `FeatureExportFunc`, and uses `TrackerInterface` (not `*Tracker`). The re-implementation must integrate with these additions rather than replacing them.

**`TrackingPipeline` struct** — wraps `*TrackingPipelineConfig` with a `ForegroundExtractor` and `sync.RWMutex`. Methods: `NewTrackingPipeline(config)`, `SetExtractorMode(mode string)`, `GetExtractorMode() string`, `FrameCallback() func(*LiDARFrame)`.

**New fields on `TrackingPipelineConfig`:**

New fields on `TrackingPipelineConfig`:

| Field                 | Type                | Default        | Purpose                                                       |
| --------------------- | ------------------- | -------------- | ------------------------------------------------------------- |
| `ExtractorMode`       | string              | `"background"` | Active algorithm: `"background"`, `"velocity"`, or `"hybrid"` |
| `HybridMergeMode`     | string              | `"union"`      | Merge strategy: `"union"`, `"intersection"`, or `"primary"`   |
| `ForegroundExtractor` | ForegroundExtractor | nil            | Custom injected extractor (overrides `ExtractorMode`)         |

**Frame callback changes:**

The existing `NewFrameCallback()` closure is refactored to:

1. Acquire read lock on `tp.mu`
2. Check for custom extractor first, then fall back to `BackgroundManager`
3. Call `extractor.ProcessFrame()` which returns mask + metrics
4. Pass mask to existing clustering/tracking pipeline (unchanged)
5. Record per-frame metrics if analysis run is active

The deprecated `NewFrameCallback()` method delegates to `NewTrackingPipeline(cfg).FrameCallback()`.

### 3.2 DBSCAN signature change

The branch changed `DBSCAN()` to return `([]WorldCluster, []int)` (clusters + labels array). On `main`, the signature is still `[]WorldCluster`. This change enables noise point analysis but requires updating all call sites.

**Recommendation:** Add the labels return value on `main` as a separate preparatory PR since it affects tests.

### 3.3 Main program integration ([cmd/radar/radar.go](../../cmd/radar/radar.go))

In [cmd/radar/radar.go](../../cmd/radar/radar.go), create `pipeline = lidar.NewTrackingPipeline(pipelineConfig)` and use `pipeline.FrameCallback()` as the callback. Pass `pipeline` to `monitor.NewWebServer` via a new `TrackingPipeline` field for dynamic algorithm selection.

---

## 4. API endpoint

### `GET /api/lidar/algorithm`

Returns current algorithm mode:

`GET /api/lidar/algorithm` returns the current mode (e.g. `{ "mode": "background" }`).

`POST /api/lidar/algorithm` switches the algorithm at runtime. Accepts a JSON body with a `mode` field. Valid modes: `background`, `velocity`, `hybrid`. Also accepts form-encoded POST (redirects to monitor page).

Valid modes: `background`, `velocity`, `hybrid`

**Implementation:** `handleAlgorithmConfig` in `webserver.go`. Calls `TrackingPipeline.SetExtractorMode()`.

Accepts both `application/json` and form-encoded POST. Form POST redirects to monitor page.

---

## 5. Database schema (migration 013)

**`lidar_algorithm_runs` table:**

| Column                     | Type    | Constraint  | Notes                         |
| -------------------------- | ------- | ----------- | ----------------------------- |
| `run_id`                   | TEXT    | PRIMARY KEY |                               |
| `start_unix_nanos`         | INTEGER | NOT NULL    | Run start time                |
| `end_unix_nanos`           | INTEGER |             | Run end time                  |
| `algorithms_json`          | TEXT    |             | JSON array of algorithm names |
| `params_json`              | TEXT    |             | JSON config snapshot          |
| `pcap_file`                | TEXT    |             | Source PCAP (if replay)       |
| `total_frames`             | INTEGER | DEFAULT 0   | Frames processed              |
| `total_processing_time_us` | INTEGER | DEFAULT 0   | Cumulative time               |
| `summary_json`             | TEXT    |             | Final summary                 |

**`lidar_algorithm_frame_results` table:**

| Column               | Type    | Constraint                                              | Notes                     |
| -------------------- | ------- | ------------------------------------------------------- | ------------------------- |
| `run_id`             | TEXT    | NOT NULL, FK → `lidar_algorithm_runs` ON DELETE CASCADE |                           |
| `frame_unix_nanos`   | INTEGER | NOT NULL                                                | Frame timestamp           |
| `algorithm_name`     | TEXT    | NOT NULL                                                | Algorithm identifier      |
| `foreground_count`   | INTEGER |                                                         | Foreground points         |
| `background_count`   | INTEGER |                                                         | Background points         |
| `cluster_count`      | INTEGER |                                                         | Clusters found            |
| `processing_time_us` | INTEGER |                                                         | Per-frame time            |
| `precision`          | REAL    |                                                         | If ground truth available |
| `recall`             | REAL    |                                                         | If ground truth available |
| `extra_json`         | TEXT    |                                                         | Algorithm-specific data   |

Primary key: `(run_id, frame_unix_nanos, algorithm_name)`.

**Note:** On `main`, migration numbering may have advanced. Check current migration count before applying.

**Note:** On `main`, migration numbering may have advanced. Check current migration count before applying.

---

## 6. CLI tool: `algo-compare`

```
cmd/tools/algo-compare/main.go (build tag: pcap)
```

Processes a PCAP file through multiple foreground extraction algorithms simultaneously and generates comparison statistics:

```bash
algo-compare -pcap transit.pcap -output-dir results/ -merge-mode union -verbose
```

**Output:** JSON with per-algorithm foreground counts, processing times, and inter-algorithm agreement statistics.

**Build:** Requires `pcap` build tag (same as `pcap-analyze`).

---

## 7. Implementation plan for `main`

### Prerequisites (already on main)

- [x] `isNilInterface()` utility
- [x] `AnalysisRunManager` and registry pattern
- [x] Locked baseline parameters on `BackgroundParams` and `BackgroundCell`
- [x] Foreground thaw/freeze fixes
- [x] Track quality metrics on `TrackedObject`
- [x] Grid plotter
- [x] Version package

### Phase 1: interface + background adapter (low risk)

**New files:**

- `internal/lidar/extractor.go`: `ForegroundExtractor` interface, `MergeMode` constants, mask utilities
- `internal/lidar/extractor_background.go`: `BackgroundSubtractorExtractor` adapter
- `internal/lidar/extractor_test.go`: Unit tests for mask merge, agreement, precision/recall

**No existing file changes required.** Pure additive: can be merged independently.

### Phase 2: velocity estimation + frame history (low risk)

**New files:**

- `internal/lidar/frame_history.go`: `FrameHistory`, `VelocityFrame`, `PointWithVelocity`
- `internal/lidar/velocity_estimation.go`: `EstimatePointVelocities`, spatial correspondence

**No existing file changes required.** Uses existing `SpatialIndex` and `PointPolar` types.

### Phase 3: velocity-coherent extractor (medium risk)

**New files:**

- `internal/lidar/extractor_velocity_coherent.go`: `VelocityCoherentExtractor`

**Dependencies:** Phase 1 + Phase 2. Uses existing DBSCAN for clustering.

**Risk:** The DBSCAN signature on `main` returns `[]WorldCluster` (no labels). The velocity-coherent extractor currently calls `DBSCAN()` expecting `([]WorldCluster, []int)`. Either:

- Option A: Change `DBSCAN()` signature on `main` first (separate PR, touches tests)
- Option B: Ignore the labels return in the velocity-coherent extractor (use only clusters)

**Recommendation:** Option A; the labels array is independently useful for noise analysis.

### Phase 4: hybrid extractor + evaluation harness (low risk)

**New files:**

- `internal/lidar/extractor_hybrid.go`: `HybridExtractor`
- `internal/lidar/evaluation_harness.go`: `EvaluationHarness`

**Dependencies:** Phase 1.

### Phase 5: pipeline integration (high risk; most conflicts expected)

**Modified files:**

- `internal/lidar/tracking_pipeline.go`: Add `TrackingPipeline` struct, modify `NewFrameCallback()`

**Key conflict areas:**

- `TrackingPipelineConfig` struct has been significantly expanded on `main` with:
  - `VisualiserPublisher VisualiserPublisher`
  - `VisualiserAdapter VisualiserAdapter`
  - `LidarViewAdapter LidarViewAdapter`
  - `MaxFrameRate float64`
  - `VoxelLeafSize float64`
  - `FeatureExportFunc func(...)`
  - `Tracker` changed to `TrackerInterface`
- The `NewFrameCallback()` closure on `main` is ~300 lines and includes:
  - Ground removal (`removeGroundPoints`)
  - Voxel downsampling
  - Frame rate limiting via `atomic.Int64`
  - Visualiser publishing
  - LidarView adapter forwarding
  - Feature export hook

**Strategy:** Add `ExtractorMode`, `HybridMergeMode`, `ForegroundExtractor` fields to the existing `TrackingPipelineConfig`. Create `TrackingPipeline` wrapper. Modify the foreground extraction section of `NewFrameCallback()` to delegate to the extractor when present, keeping all downstream logic (ground removal, voxel downsampling, visualiser, etc.) intact.

### Phase 6: webserver API (medium risk)

**Modified files:**

- `internal/lidar/monitor/webserver.go`: Add `trackingPipeline` field, `handleAlgorithmConfig` handler, register route

**Key conflict areas:**

- `WebServer` struct has many new fields on `main` (sweep runner, auto-tuner, tuning config, etc.)
- Route registration in `RegisterRoutes()` / `setupRoutes()` has been refactored
- Need to integrate with `main`'s `WebServerConfig` pattern

**Strategy:** Add `TrackingPipeline *lidar.TrackingPipeline` to `WebServerConfig`, store in `WebServer`, add handler and route registration. Minimal touch.

### Phase 7: migration + CLI tool (low risk)

**New files:**

- `internal/db/migrations/000013_create_algorithm_comparison.{up,down}.sql`: Check migration numbering on `main`
- `cmd/tools/algo-compare/main.go`: Standalone CLI tool

**Modified files:**

- [internal/db/schema.sql](../../internal/db/schema.sql): Add table definitions (sync with migration)

---

## 8. Testing strategy

### New test files

| File                                             | Tests                                                                                                                                                                                                                     | Lines |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| `internal/lidar/extractor_test.go`               | MergeMasks (union/intersection/primary/majority), CountForeground, ComputeMaskAgreement, ComputePrecisionRecall, BackgroundSubtractorExtractor.Name, VelocityCoherentExtractor.Name, HybridExtractor interface compliance | 264   |
| `internal/lidar/tracking_pipeline_logic_test.go` | initializeExtractor (all modes), isNilInterface edge cases                                                                                                                                                                | 136   |
| `internal/lidar/tracking_pipeline_test.go`       | Pipeline callback with nil frame, pipeline with nil extractor, FrameCallback invocation, SetExtractorMode switching                                                                                                       | 149   |
| `internal/lidar/monitor/webserver_algo_test.go`  | handleAlgorithmConfig GET/POST, invalid mode rejection                                                                                                                                                                    | 78    |

### Integration with main's tests

On `main`, `tracking_pipeline_test.go` is 1,248 lines with extensive tests for the expanded pipeline. New tests must:

- Use `TrackerInterface` (not `*Tracker`)
- Account for `VisualiserPublisher` and `LidarViewAdapter` nil handling
- Follow the dependency injection patterns established in the refactoring PRs (#224, #229)

---

## 9. Risk assessment

| Risk                       | Likelihood | Impact | Mitigation                                                    |
| -------------------------- | ---------- | ------ | ------------------------------------------------------------- |
| Pipeline struct conflicts  | High       | High   | Phase 5 last; surgical additions to existing struct           |
| DBSCAN signature change    | Medium     | Medium | Separate preparatory PR for signature change                  |
| Webserver route conflicts  | Medium     | Low    | Additive handler + route; minimal touching existing code      |
| Migration number collision | Low        | Low    | Check `ls internal/db/migrations/` before applying            |
| Test conflicts             | Medium     | Medium | Write new tests; don't modify existing pipeline tests         |
| Performance regression     | Low        | Medium | Hybrid mode is opt-in; default remains background subtraction |

---

## 10. Appendix: file-by-file reference

### New files (pure additions)

| File                                             | Lines | Description                                                          |
| ------------------------------------------------ | ----- | -------------------------------------------------------------------- |
| `internal/lidar/extractor.go`                    | 200   | `ForegroundExtractor` interface, `MergeMode`, mask utilities         |
| `internal/lidar/extractor_background.go`         | 204   | `BackgroundSubtractorExtractor`: wraps `BackgroundManager`           |
| `internal/lidar/extractor_hybrid.go`             | 247   | `HybridExtractor`: multi-algorithm merge                             |
| `internal/lidar/extractor_velocity_coherent.go`  | 243   | `VelocityCoherentExtractor`: motion-based extraction                 |
| `internal/lidar/evaluation_harness.go`           | 314   | A/B comparison framework                                             |
| `internal/lidar/frame_history.go`                | 191   | `FrameHistory` circular buffer, `PointWithVelocity`, `VelocityFrame` |
| `internal/lidar/velocity_estimation.go`          | 418   | Point correspondence and velocity estimation                         |
| `internal/lidar/extractor_test.go`               | 264   | Unit tests for extractors and utilities                              |
| `internal/lidar/tracking_pipeline_logic_test.go` | 136   | Pipeline initialisation tests                                        |
| `internal/lidar/monitor/webserver_algo_test.go`  | 78    | Algorithm API endpoint tests                                         |
| `cmd/tools/algo-compare/main.go`                 | 340   | Algorithm comparison CLI (build tag: pcap)                           |
| `internal/db/migrations/000013_*.sql`            | ~40   | Algorithm comparison tables                                          |

### Modified files (require conflict resolution)

| File                                                   | Branch Changes                                                        | Main Changes                                                                                                                             | Conflict Risk      |
| ------------------------------------------------------ | --------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------ |
| `internal/lidar/tracking_pipeline.go`                  | Add `TrackingPipeline`, `initializeExtractor`, `ExtractorMode` fields | Add `VisualiserPublisher`, `VisualiserAdapter`, `LidarViewAdapter`, `MaxFrameRate`, `VoxelLeafSize`, ground removal, frame rate limiting | **HIGH**           |
| `internal/lidar/monitor/webserver.go`                  | Add `handleAlgorithmConfig`, `trackingPipeline` field                 | Add sweep dashboard, auto-tuner, tuning config, single config refactor                                                                   | **MEDIUM**         |
| `internal/lidar/clustering.go`                         | Return `([]WorldCluster, []int)` from `DBSCAN`                        | Unchanged signature `[]WorldCluster`                                                                                                     | **MEDIUM**         |
| [cmd/radar/radar.go](../../cmd/radar/radar.go)         | Add `NewTrackingPipeline`, pass to webserver                          | Extensive refactoring (config loading, tuning params, dependency injection)                                                              | **MEDIUM**         |
| [internal/db/schema.sql](../../internal/db/schema.sql) | Add algorithm tables                                                  | Schema has evolved                                                                                                                       | **LOW**            |
| `internal/lidar/tracking_pipeline_test.go`             | New tests (149 lines)                                                 | Existing 1,248 lines of tests                                                                                                            | **LOW** (additive) |
