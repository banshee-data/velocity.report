# Velocity-Coherent Foreground Extraction

Active plan: [lidar-velocity-coherent-foreground-extraction-plan.md](../../plans/lidar-velocity-coherent-foreground-extraction-plan.md)

**Status:** Core phases 1–5 prototyped with simplifications; phases 0, 6–7
pending.

Mathematical model: [data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md](../../data/maths/proposals/20260220-velocity-coherent-foreground-extraction.md)

## Problem

Background-subtraction (`ProcessFramePolarWithMask`) produces foreground
trails — persistent false-positive points behind vehicles after they pass.
EMA-based background model reconverges too slowly after freeze expiry.
Additionally, DBSCAN `MinPts=12` discards valid objects with sparse returns.

Human observers can identify moving objects with as few as 3 points by
leveraging motion continuity, spatial coherence, and temporal persistence.

## Core Concept

1. **Estimate per-point velocities** from frame-to-frame correspondence.
2. **Cluster in 6D** (position + velocity) — group points moving together.
3. **Associate with tracks** via velocity matching.
4. **Extend track boundaries** — pre-entry and post-exit via prediction.

| Feature        | Background Subtraction  | Velocity-Coherent        |
| -------------- | ----------------------- | ------------------------ |
| Classification | Per-point, static       | Velocity-temporal        |
| Min cluster    | 12 points               | 3 points                 |
| Lifecycle      | Hits/misses counter     | Velocity prediction      |
| Pre-entry      | Warmup suppression      | Predicted from velocity  |
| Post-exit      | Deleted after MaxMisses | Continued via prediction |
| Fragmentation  | Multiple tracks         | Kinematic merge          |

## Algorithm Phases

### Phase 1: Point-Level Velocity Estimation

Nearest-neighbour correspondence with velocity constraint. For each point
in frame N, search previous frame within `SearchRadius` (2.0 m), score by
distance + velocity consistency with neighbours, compute velocity vector
and confidence.

Config: `SearchRadius` 2.0 m, `MaxVelocityMps` 50.0, `MinConfidence` 0.3.

### Phase 2: 6D DBSCAN Clustering

Extend standard 3D DBSCAN to 6D: $(x, y, z, v_x, v_y, v_z)$.

$$D_{6D}(p, q) = \sqrt{\alpha(\Delta x^2 + \Delta y^2 + \Delta z^2) + \beta(\Delta v_x^2 + \Delta v_y^2 + \Delta v_z^2)}$$

Where $\alpha = 1.0$ (position weight), $\beta = 2.0$ (velocity weight).
`MinPts=3` (reduced from 12).

### Phase 3: Long-Tail Track Management

Extended state machine:

```
PRE_TAIL (≥3 pts) → TENTATIVE → CONFIRMED → POST_TAIL → DELETED
         ↑                                    │
         └──────── recovery match ────────────┘
```

- **Pre-tail**: Velocity-predicted entry zone matching.
- **Post-tail**: Continue prediction up to 30 frames (3 s at 10 Hz)
  with growing uncertainty radius, max 10.0 m.

### Phase 4: Sparse Continuation

Adaptive tolerances by point count:

| Points | Velocity Tolerance | Spatial Tolerance |
| ------ | ------------------ | ----------------- |
| ≥12    | ±2.0 m/s           | ±1.0 m            |
| 6–11   | ±1.5 m/s           | ±0.8 m            |
| 3–5    | ±0.5 m/s           | ±0.5 m            |
| <3     | Prediction only    | —                 |

### Phase 5: Track Fragment Merging

Reconnect split tracks via kinematic trajectory matching. Score by
position error, velocity match, and trajectory alignment:

$$S_{\text{trajectory}} = \cos(\theta_{\text{exit}}, \theta_{\text{entry}}) \cdot \exp\!\left(\frac{-|v_{\text{exit}} - v_{\text{entry}}|}{\sigma_v}\right)$$

Merge config: `MaxTimeGap` 5.0 s, `MaxPositionError` 3.0 m,
`MaxVelocityDiff` 2.0 m/s, `MinAlignment` 0.7.

## Dual-Source Architecture

Parallels the radar pattern (`radar_objects` vs `radar_data_transits`):

- `lidar_tracks` — background-subtraction + DBSCAN (existing)
- `lidar_velocity_coherent_tracks` — velocity-coherent extraction (new)

Both stored independently, queryable via `?source=` parameter,
comparable in dashboards.

## Database Extensions

- `lidar_velocity_coherent_clusters` — 6D DBSCAN output per frame.
- `lidar_velocity_coherent_tracks` — parallel track table with velocity
  confidence, consistency scores, and sparse-tracking metrics.
- `lidar_track_merges` — auditable merge history.

## Prototype Simplifications (vs Original Design)

| Area           | Original      | Prototype                 |
| -------------- | ------------- | ------------------------- |
| Tracking model | 6D            | 2D + velocity (x,y,vx,vy) |
| 6D DBSCAN      | Full 6D index | 3D position + vel filter  |
| Heading        | Explicit      | Implicit from velocity    |
| Z-axis         | Tracked       | Stored as statistic       |
| Batch mode     | Supported     | Real-time only            |

## Acceptance Metrics (Hypotheses)

| Metric                           | Target       |
| -------------------------------- | ------------ |
| Sparse-object recall (3–11 pts)  | +20% to +40% |
| Track fragmentation rate         | −10% to −25% |
| Median track duration (boundary) | +10% to +30% |
| Additional false positives       | <+10%        |
| Throughput regression            | <20%         |
