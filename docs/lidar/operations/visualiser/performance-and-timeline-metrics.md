# Performance and Timeline Metrics

- **Source plan:** `docs/plans/lidar-visualiser-performance-and-scene-health-timeline-metrics-plan.md`

First-class metrics channel for the visualiser: per-frame performance and scene health signals with timeline rendering, comparison harness, and `.vrlog` persistence.

## Metric Families

| Family            | Purpose                                                   |
| ----------------- | --------------------------------------------------------- |
| `PERFORMANCE`     | Runtime/process metrics (CPU, memory, encode/decode time) |
| `SCENE_HEALTH`    | Scene-quality metrics derived from frame content          |
| `PIPELINE_HEALTH` | Future: transport/backpressure/drop rates                 |

## Core Metric Definitions

| Key                               | Family       | Unit    | Warn ≥ | Error ≥ |
| --------------------------------- | ------------ | ------- | ------ | ------- |
| `perf.cpu_usage_pct`              | PERFORMANCE  | percent | 75     | 90      |
| `perf.memory_rss_mb`              | PERFORMANCE  | MB      | 1000   | 1500    |
| `scene.points_outside_bbox_count` | SCENE_HEALTH | count   | 50     | 200     |
| `scene.track_drift_m`             | SCENE_HEALTH | metres  | 0.50   | 1.00    |
| `scene.subregion_match_iou`       | SCENE_HEALTH | ratio   | < 0.70 | < 0.50  |

## Comparative Harness

Compares two representations of the same frame aligned by `frame_id` and `timestamp_ns`:

- **Centroid-vector scene:** track/cluster centroid and vector-derived world model.
- **Grid-world scene:** original grid-based world representation.

### Comparison Outputs

- Resolution delta metrics (detail retained/lost between vector and grid).
- Drift metrics between boxes and points over time.
- Subregion quality metrics against ground truth.

### Ground Truth Annotation Model

Minimum label sets:

- Ground plane polygons/mesh regions
- Wall/structure regions
- Known static objects (poles, signs, kerb islands)
- Optional dynamic-object reference tracks for benchmark runs

## Per-Frame Collection Pipeline

1. Frame assembled (`frame_id`, `timestamp_ns` known)
2. Performance sampler captures process CPU/memory snapshot
3. Scene health sampler computes frame-derived values
4. Severity computed against metric definition thresholds
5. `FrameMetrics` attached to `FrameBundle`
6. Publisher streams frame to clients
7. Recorder writes the same `FrameBundle` into `.vrlog` chunk

## Proto Extensions

`FrameBundle` gains a `FrameMetrics metrics` field (field 13), gated by `StreamRequest.include_metrics` (field 8). Capabilities response adds `supports_metrics` and `metric_definitions[]`.

Key messages: `MetricFamily` enum, `MetricDefinition`, `MetricSample`, `FrameMetrics`, `SubregionMetric`, `TrackComparisonMetric`.

## API Endpoints

| Endpoint                                   | Method | Purpose                                    |
| ------------------------------------------ | ------ | ------------------------------------------ |
| Streaming (via `FrameBundle.metrics`)      | gRPC   | Real-time and replay metrics               |
| `/api/lidar/runs/{run_id}/metrics`         | GET    | Windowed metric query (timeline, bucketed) |
| `/api/lidar/runs/{run_id}/metrics/compare` | POST   | Per-track/subregion debug comparison       |

## VRLOG Format

No new file type. Metrics ride inside existing `FrameBundle` chunks. Header metadata additions:

- `metrics_schema_version` (e.g. `"v1"`)
- `metric_definitions[]` (keys, families, units, thresholds)

Old logs without metrics field remain readable (absent field = no metrics lane).

## Timeline UI Design

Multi-lane timeline with shared time cursor:

- **Lane A:** existing scene/event lane (tracks, labels, QC events)
- **Lane B:** performance lane (CPU, memory) — line charts
- **Lane C:** scene health lane (outside-box points, drift) — bars or step-line

Severity overlays: warn range in amber, error range in red, spike markers for threshold crossings.

### Interaction Model

- Legend-driven toggles per metric key/family
- Crosshair tooltip shows scene summary + selected metric values for same frame
- Click spike marker to seek replay cursor to that frame

## Extensibility

Timeline is definition-driven, not hard-coded. Metric renderers resolve from `MetricDefinition.family` + `unit`. Unknown keys fall back to generic scalar renderer. New metrics require only emitting new `metric_key` + `MetricDefinition`; no timeline schema migration.

## Scenario Profiling Strategy

After harness implementation, compare across a scenario matrix (low/medium/high density, different road geometries, known edge cases). Prioritise by highest user impact first, then largest runtime bottlenecks. Workflow: **measure first, then optimise**.
