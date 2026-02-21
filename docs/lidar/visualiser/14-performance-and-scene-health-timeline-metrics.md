# Design: Performance and Scene Health Metrics in Timeline and VR Logs

**Status:** Proposed (February 2026)

## Objective

Add a first-class metrics channel to the visualiser so each scene frame can be inspected with:

- performance signals (CPU spikes, memory usage),
- scene health signals (for example points outside bounding boxes, drift over time),
- timeline rendering that shows these metrics beside scene data in real time and replay.

## Goals

- Make per-frame performance regressions visible during live use and replay.
- Keep metric data in `.vrlog` so analysis is reproducible and portable.
- Expose the same metric model to APIs and UI to avoid duplicate logic.
- Support additive metric families without schema churn for each new metric.

## Non-Goals

- Full system observability platform.
- Remote/cloud telemetry pipelines.
- Replacing existing frame or event timeline features.

## Comparative harness design

The harness compares two representations for the same frame/time window:

- **Centroid-vector scene**: track/cluster-centroid and vector-derived world model.
- **Grid-world scene**: original grid-based world representation used by the pipeline.

Both are aligned by `frame_id` and `timestamp_ns`, then evaluated against shared point-cloud and bounding-box evidence.

### Comparison outputs

- Resolution delta metrics (how much detail is retained/lost between vector and grid views).
- Drift metrics between boxes and points over time.
- Subregion quality metrics against ground truth.

## Metric Model

### Metric families

- `PERFORMANCE`: runtime/process metrics (CPU usage, memory, frame encode/decode time).
- `SCENE_HEALTH`: scene-quality metrics derived from frame content.
- `PIPELINE_HEALTH`: optional future family for transport/backpressure/drop rates.

### Core metric definitions

Initial required metrics:

- `perf.cpu_usage_pct` (`PERFORMANCE`, percent, warning >= 75, error >= 90)
- `perf.memory_rss_mb` (`PERFORMANCE`, MB, warning >= 1000, error >= 1500)
- `scene.points_outside_bbox_count` (`SCENE_HEALTH`, count, warning >= 50, error >= 200)
- `scene.track_drift_m` (`SCENE_HEALTH`, metres, warning >= 0.50, error >= 1.00)
- `scene.subregion_match_iou` (`SCENE_HEALTH`, ratio, warning < 0.70, error < 0.50)

### Harness metric formulas

- **Points outside boxes**: count points that do not fall within any expected box for the same object/subregion.
- **Drift (metres)**: distance between expected object centroid (ground truth or baseline world) and observed centroid/box centre.
- **Subregion performance**: per-region overlap score (IoU) and point-class agreement versus annotated truth.

## Ground truth annotation model

The harness requires a scene annotation layer that can be loaded for replay and live comparison.

Minimum label sets:

- Ground plane polygons/mesh regions.
- Wall/structure regions.
- Known static objects (for example poles, signs, kerb islands).
- Optional dynamic-object reference tracks for benchmark runs.

Proposed annotation payload:

```json
{
  "scene_id": "site-001-main-street",
  "regions": [
    {"region_id": "ground_a", "class": "ground_plane", "geometry": "..."},
    {"region_id": "wall_east", "class": "wall", "geometry": "..."}
  ],
  "known_objects": [
    {"object_id": "pole_07", "class": "static_object", "geometry": "..."}
  ]
}
```

## API Design

### 1) Stream API (real time + replay)

Extend `FrameBundle` and capability contracts in `visualiser.proto`.

```protobuf
enum MetricFamily {
  METRIC_FAMILY_UNSPECIFIED = 0;
  METRIC_FAMILY_PERFORMANCE = 1;
  METRIC_FAMILY_SCENE_HEALTH = 2;
  METRIC_FAMILY_PIPELINE_HEALTH = 3;
}

message MetricDefinition {
  string metric_key = 1;           // stable key, e.g. perf.cpu_usage_pct
  string display_name = 2;         // CPU usage
  MetricFamily family = 3;
  string unit = 4;                 // pct|mb|count|m|ms
  double warning_threshold = 5;    // NaN/empty if none
  double error_threshold = 6;      // NaN/empty if none
}

message MetricSample {
  string metric_key = 1;
  double value = 2;
  int64 timestamp_ns = 3;          // same frame timestamp
  uint64 frame_id = 4;
  string severity = 5;             // OK|WARN|ERROR
}

message FrameMetrics {
  uint64 frame_id = 1;
  int64 timestamp_ns = 2;
  repeated MetricSample samples = 3;
  repeated SubregionMetric subregion_metrics = 4;
  repeated TrackComparisonMetric track_metrics = 5;
}

message SubregionMetric {
  string region_id = 1;
  string metric_key = 2;           // e.g. scene.subregion_match_iou
  double value = 3;
  string severity = 4;
}

message TrackComparisonMetric {
  string track_id = 1;
  string metric_key = 2;           // e.g. scene.track_drift_m
  double value = 3;
  uint32 points_outside_bbox = 4;
  string severity = 5;
}

message FrameBundle {
  // existing fields...
  FrameMetrics metrics = 10;
}

message StreamRequest {
  // existing fields...
  bool include_metrics = 23;
}

message CapabilitiesResponse {
  // existing fields...
  bool supports_metrics = 8;
  repeated MetricDefinition metric_definitions = 9;
}
```

### 2) Query API (windowed fetch)

Add a metrics window endpoint for timeline panels and run browser workflows.

- `GET /api/lidar/runs/{run_id}/metrics?start_ns=&end_ns=&families=&metric_keys=&bucket=&limit=&cursor=`

Response:

- `definitions[]` (`MetricDefinition`)
- `frames[]` (`FrameMetrics`)
- `series[]` (optional bucketed points for zoomed-out timeline)
- `next_cursor`

This keeps scene data and metrics queryable side-by-side by shared `frame_id` and `timestamp_ns`.

### 3) Per-track/subregion debug comparison API

Allow front-end tools to request focused comparisons for debug workflows.

- `POST /api/lidar/runs/{run_id}/metrics/compare`

Request body:

```json
{
  "start_ns": 1700000000000000000,
  "end_ns": 1700000005000000000,
  "track_ids": ["track_42", "track_57"],
  "region_ids": ["ground_a", "wall_east"],
  "include_point_mismatch_samples": true
}
```

Response includes per-track drift, points-outside-box counts, and per-region match metrics so the UI can inspect specific offenders.

## Per-frame collection and logging

Metrics are generated once per frame in the same frame assembly path that builds `FrameBundle`.

1. Frame assembled (`frame_id`, `timestamp_ns` known).
2. Performance sampler captures process CPU/memory snapshot.
3. Scene health sampler computes frame-derived values (outside-box points, drift).
4. Severity computed against metric definition thresholds.
5. `FrameMetrics` attached to `FrameBundle`.
6. Publisher streams frame to clients.
7. Recorder writes the same `FrameBundle` into `.vrlog` chunk.

## VR log format changes

No new file type is required; keep `.vrlog` chunking/indexing unchanged and store metrics in each `FrameBundle`.

Header metadata additions:

```json
{
  "metrics_schema_version": "v1",
  "metric_definitions": [
    {
      "metric_key": "perf.cpu_usage_pct",
      "family": "PERFORMANCE",
      "unit": "pct",
      "warning_threshold": 75,
      "error_threshold": 90
    }
  ]
}
```

Benefits:

- replay determinism for metric visualisation,
- no secondary log synchronisation problem,
- forwards-compatible metric extension through additive `MetricSample` keys.

## Timeline UI design

### Layout

Use a multi-lane timeline with shared time cursor:

- Lane A: existing scene/event lane (tracks, labels, QC events)
- Lane B: performance lane (CPU, memory)
- Lane C: scene health lane (outside-box points, drift)

All lanes align by `timestamp_ns`; hovering or scrubbing in one lane updates all lanes.

### Visual encoding

- Continuous metrics (CPU, memory, drift): line charts.
- Count metrics (outside-box points): bars or step-line.
- Severity overlays:
  - warn range in amber,
  - error range in red,
  - spike markers for threshold crossings.

### Real-time behaviour

- Append new per-frame samples as frames arrive.
- Use fixed-size ring buffers per metric key for viewport efficiency.
- Downsample for zoomed-out views; keep raw values for tooltips and seek targets.

### Interaction model

- Legend-driven toggles per metric key/family.
- Crosshair tooltip shows scene summary and selected metric values for the same frame.
- Click spike marker to seek replay cursor to that frame.

## Extensibility strategy

The timeline should be definition-driven, not hard-coded:

- Metric renderers resolve from `MetricDefinition.family` + `unit`.
- Unknown keys fall back to generic scalar renderer.
- New metrics require only emitting new `metric_key` + `MetricDefinition`; no timeline schema migration.

## Compatibility and rollout

1. Add proto fields and backend emitters behind `include_metrics`.
2. Populate `metric_definitions` in capability response.
3. Add recorder/header support for metric metadata.
4. Add comparison harness execution path (vector scene vs grid world) with ground-truth loading.
5. Add timeline lanes in macOS visualiser.
6. Keep old logs readable (`metrics` field absent => no metrics lane data).

## Scenario profiling and optimisation prioritisation

After harness implementation, run comparisons across a scenario matrix:

- low/medium/high object density scenes,
- different road geometries and structure layouts,
- known edge cases (occlusion-heavy frames, fast crossings, sparse returns).

For each scenario, capture:

- drift distributions by track and region,
- points-outside-box rates,
- stage-level runtime costs (for example scene-flow calculations, DBSCAN iterations).

Prioritisation policy:

1. Rank by highest user impact (largest drift/error rates first).
2. Rank by largest runtime bottlenecks (slowest pipeline stages).
3. Optimise top offenders first (threshold tuning, maths optimisation, parallelisation) and re-run the same harness to confirm gain.

The workflow is explicitly **measure first, then optimise**.

## Acceptance criteria

- Each streamed/replayed frame can carry `FrameMetrics`.
- `.vrlog` replay reproduces identical metric traces for the same recording.
- Timeline renders scene and metrics in synchronised lanes with shared cursor.
- CPU and memory spikes, plus configured scene-health thresholds, are visibly marked.
- Adding a new metric key does not require timeline structural changes.

## Related documents

- [02-api-contracts.md](./02-api-contracts.md)
- [08-track-event-timeline-bar.md](./08-track-event-timeline-bar.md)
- [03-architecture.md](./03-architecture.md)
