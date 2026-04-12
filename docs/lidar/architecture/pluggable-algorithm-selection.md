# Pluggable algorithm selection

Active plan: [lidar-architecture-dynamic-algorithm-selection-plan.md](../../plans/lidar-architecture-dynamic-algorithm-selection-plan.md)

**Status:** Branch-history design specification (not active on `main` runtime)

`ForegroundExtractor` interface design and its implementations: background-subtraction, velocity-coherent, and hybrid; enabling runtime algorithm selection for the LiDAR foreground extraction stage.

## Problem

The background-subtraction algorithm (`ProcessFramePolarWithMask`) produces
"foreground trails": persistent false-positive foreground points behind
vehicles after they pass. Root cause: EMA-based background model takes time
to reconverge after freeze expiry.

## ForegroundExtractor interface

The `ForegroundExtractor` interface defines five methods:

| Method         | Signature                                                                      | Purpose                                          |
| -------------- | ------------------------------------------------------------------------------ | ------------------------------------------------ |
| `Name`         | `() string`                                                                    | Returns the extractor's display name             |
| `ProcessFrame` | `(points []PointPolar, timestamp time.Time) ([]bool, ExtractorMetrics, error)` | Returns a foreground mask (same length as input) |
| `GetParams`    | `() map[string]interface{}`                                                    | Returns current parameter values                 |
| `SetParams`    | `(params map[string]interface{}) error`                                        | Updates parameters at runtime                    |
| `Reset`        | `()`                                                                           | Resets internal state                            |

The `[]bool` foreground mask preserves index correspondence with the input points for downstream processing.

## Extractor implementations

### BackgroundSubtractorExtractor

Zero-copy adapter wrapping `BackgroundManager.ProcessFramePolarWithMask()`.

### VelocityCoherentExtractor

Motion-based foreground extraction using frame-to-frame point
correspondence:

1. Convert polar → world coordinates with velocity metadata
2. Estimate per-point velocities via spatial correspondence
3. DBSCAN with reduced `MinPts=3` (velocity coherence confirms identity)
4. Filter clusters by velocity coherence score
5. Mark points in coherent clusters as foreground

### HybridExtractor

Runs multiple extractors in parallel, merges results via configurable mode:

- `union`: OR merge (max detection coverage)
- `intersection`: AND merge (max precision)
- `primary`: use first extractor, collect metrics from others

## Pipeline integration

`TrackingPipeline` wraps `TrackingPipelineConfig` with dynamic algorithm
selection. New fields on config:

| Field                 | Type                | Default        | Purpose                                                  |
| --------------------- | ------------------- | -------------- | -------------------------------------------------------- |
| `ExtractorMode`       | string              | `"background"` | Algorithm mode: `"background"`, `"velocity"`, `"hybrid"` |
| `HybridMergeMode`     | string              | `"union"`      | Merge strategy: `"union"`, `"intersection"`, `"primary"` |
| `ForegroundExtractor` | ForegroundExtractor | nil            | Custom injection override                                |

Frame callback delegates to extractor when present; all downstream logic
(ground removal, voxel downsampling, visualiser) unchanged.

## Runtime API

- `GET /api/lidar/algorithm`: returns current mode
- `POST /api/lidar/algorithm`: switches algorithm at runtime

## Evaluation harness

Runs multiple extractors on the same frames, collects per-frame comparison
metrics. Optional `GroundTruthProvider` for precision/recall computation.
Results stored in `lidar_algorithm_runs` and
`lidar_algorithm_frame_results` tables.

## What landed on main vs pending

**Already on main:** `isNilInterface()`, thaw grace period,
locked baseline fields, `AnalysisRunManager`, track quality metrics,
grid plotter, version package, foreground freeze/thaw fixes.

**Needs re-implementation:** `ForegroundExtractor` interface, background
adapter, velocity-coherent extractor (+ frame history + velocity
estimation), hybrid extractor, evaluation harness, `TrackingPipeline`
wrapper, algorithm API, `algo-compare` CLI tool, migration for comparison
tables.

## Implementation phases

| Phase | Scope                               | Risk   |
| ----- | ----------------------------------- | ------ |
| 1     | Interface + background adapter      | Low    |
| 2     | Velocity estimation + frame history | Low    |
| 3     | Velocity-coherent extractor         | Medium |
| 4     | Hybrid + evaluation harness         | Low    |
| 5     | Pipeline integration                | High   |
| 6     | Webserver API                       | Medium |
| 7     | Migration + CLI tool                | Low    |

Phase 5 is highest risk: `TrackingPipelineConfig` has been significantly
expanded on `main` with `VisualiserPublisher`, `LidarViewAdapter`,
`MaxFrameRate`, `VoxelLeafSize`, `FeatureExportFunc`, and `TrackerInterface`.
