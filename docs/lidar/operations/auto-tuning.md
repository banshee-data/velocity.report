# LiDAR Auto-Tuning Guide

Status: Active

**Status**: Phases 1–2 implemented, Phase 5 (ground truth) implemented

This guide covers the complete auto-tuning system: tunable parameters, collected metrics, scoring objectives, operational workflows, and decision-making strategies. It is intended as a reference for both human operators and AI agents refining LiDAR tracking parameters.

**Implementation files:**

| Component               | File                                               |
| ----------------------- | -------------------------------------------------- |
| AutoTuner               | `internal/lidar/sweep/auto.go`                     |
| Runner + Settle Mode    | `internal/lidar/sweep/runner.go`                   |
| Multi-objective Scoring | `internal/lidar/sweep/objective.go`                |
| Sampler                 | `internal/lidar/sweep/sampler.go`                  |
| Parameter Schema (JS)   | `internal/lidar/monitor/assets/sweep_dashboard.js` |
| Sweep Dashboard         | `internal/lidar/monitor/html/sweep_dashboard.html` |

**Related documentation:**

- [Settling Time Optimisation](settling-time-optimization.md) — Region persistence and settle modes
- [Adaptive Region Parameters](adaptive-region-parameters.md) — Per-region parameter scaling
- [LiDAR Terminology](../terminology.md) — Core terms (point, cluster, track, scene, run, sweep)

---

## Overview

Auto-tuning iteratively searches the parameter space to find values that maximise tracking quality. It runs multiple **rounds** of grid search, narrowing the search bounds after each round based on the best results.

**When to use auto-tuning:**

- After deploying a sensor at a new location
- When environmental conditions change (seasonal foliage, construction)
- When acceptance rates or track quality degrade
- When adding new parameters to the tuning configuration

**Key terms:**

| Term       | Meaning                                                                                   |
| ---------- | ----------------------------------------------------------------------------------------- |
| **Sweep**  | A batch run that evaluates every combination in a parameter grid                          |
| **Combo**  | One specific set of parameter values being evaluated                                      |
| **Round**  | One complete sweep; auto-tuning runs multiple rounds with progressively narrower bounds   |
| **Top K**  | The best-scoring combinations from a round, used to narrow bounds for the next round      |
| **Settle** | The warmup period where the background model stabilises before sampling begins            |
| **Scene**  | A named evaluation environment with labelled reference tracks for ground truth scoring    |
| **Run**    | A single processing pass over data with fixed parameters, producing tracks for evaluation |

---

## Tunable Parameters

All parameters are exposed via `/api/lidar/params` and defined in `PARAM_SCHEMA`. They are grouped by subsystem below.

### Background Model

These control how the background grid distinguishes stationary environment from foreground objects.

| Parameter                     | Type    | Default Range           | Description                                                                                                                                                                                        |
| ----------------------------- | ------- | ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `noise_relative`              | float64 | 0.01–0.2 (step 0.001)   | Fraction of measured range treated as noise threshold. Higher = more tolerant of range variation, fewer false foreground detections.                                                               |
| `closeness_multiplier`        | float64 | 1.0–20.0 (step 0.5)     | Multiplier for the closeness threshold. Higher = wider band for background acceptance, fewer false foreground detections but may miss small objects.                                               |
| `neighbor_confirmation_count` | int     | 0–8 (step 1)            | Number of neighbouring cells (0–8) that must agree before marking a cell as foreground. Higher = fewer false positives but may miss isolated foreground points.                                    |
| `seed_from_first`             | bool    | —                       | If true, initialise the background model from the very first observation rather than accumulating over many frames. Useful for static scenes where the first frame is representative.              |
| `post_settle_update_fraction` | float64 | 0–0.5 (step 0.01)       | Background update alpha after settling completes. 0 = freeze the background model entirely. Higher values allow the background to adapt to gradual changes but risk absorbing slow-moving objects. |
| `warmup_duration_nanos`       | int64   | 5–120 seconds (step 1s) | Duration of the warmup phase before classification begins. Longer warmup = more stable background model but more data lost during settling.                                                        |
| `warmup_min_frames`           | int     | 10–500 (step 10)        | Minimum frames required before warmup can complete. Works alongside `warmup_duration_nanos`; both conditions must be met.                                                                          |

**Operational guidance:**

- `noise_relative` and `closeness_multiplier` are the two most impactful background parameters. Start tuning here.
- In high-variance environments (trees, glass), increase `noise_relative` and `neighbor_confirmation_count`.
- In stable environments (open road, clear sightlines), decrease `noise_relative` for tighter foreground detection.
- `post_settle_update_fraction` at 0 is safest for PCAP analysis; non-zero values allow drift during live operation.

### Foreground Detection

These control how foreground points are grouped into clusters.

| Parameter                       | Type    | Default Range       | Description                                                                                                                                                                     |
| ------------------------------- | ------- | ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `foreground_min_cluster_points` | int     | 0–20 (step 1)       | Minimum points required for a foreground cluster to be reported. 0 = disabled (all clusters pass). Higher values suppress noise clusters but may miss small or distant objects. |
| `foreground_dbscan_eps`         | float64 | 0–2.0 (step 0.1)    | DBSCAN epsilon (maximum distance between points in a cluster). 0 = disabled. Higher values merge nearby points into larger clusters; too high merges distinct objects.          |
| `min_frame_points`              | int     | 100–5000 (step 100) | Minimum total points in a frame before processing. Frames with fewer points are dropped entirely. Protects against processing partial or corrupt frames.                        |

**Operational guidance:**

- `foreground_dbscan_eps` strongly affects how vehicles are segmented. Too low = one vehicle becomes multiple clusters (fragmentation). Too high = two adjacent vehicles merge.
- `foreground_min_cluster_points` is a simple noise filter. Start at 3-5 for a 40-channel sensor.
- `min_frame_points` should be set well below the expected point count for a valid frame (typically 5000-20000 for a 40-channel sensor).

### Tracker (Kalman Filter)

These control how clusters are associated with tracks and how the Kalman filter behaves.

| Parameter                 | Type    | Default Range        | Description                                                                                                                                              |
| ------------------------- | ------- | -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `gating_distance_squared` | float64 | 4.0–100.0 (step 1.0) | Squared Mahalanobis distance threshold for track-to-cluster association. Lower = stricter matching (fewer false associations but more missed updates).   |
| `process_noise_pos`       | float64 | 0.01–1.0 (step 0.01) | Kalman process noise for position. Higher = the filter expects more position uncertainty between frames.                                                 |
| `process_noise_vel`       | float64 | 0.05–2.0 (step 0.01) | Kalman process noise for velocity. Higher = the filter expects more velocity changes (acceleration/deceleration).                                        |
| `measurement_noise`       | float64 | 0.01–2.0 (step 0.01) | Kalman measurement noise. Higher = less trust in observations, smoother but slower-responding tracks.                                                    |
| `occlusion_cov_inflation` | float64 | 0.1–5.0 (step 0.1)   | Covariance inflation factor during occlusion (missed observations). Higher = the filter tolerates longer occlusions before the track becomes unreliable. |
| `hits_to_confirm`         | int     | 1–10 (step 1)        | Consecutive successful associations needed to promote a tentative track to confirmed. Higher = fewer false tracks but slower track initialisation.       |
| `max_misses`              | int     | 1–10 (step 1)        | Consecutive misses before a tentative (unconfirmed) track is deleted.                                                                                    |
| `max_misses_confirmed`    | int     | 3–30 (step 1)        | Consecutive misses before a confirmed track is deleted. Higher = tracks persist longer through occlusions.                                               |

**Operational guidance:**

- `hits_to_confirm` and `max_misses` together control the fragmentation/false-positive trade-off. More hits required = fewer false tracks but higher fragmentation. More misses allowed = fewer fragments but more ghost tracks.
- `gating_distance_squared` should be tuned based on vehicle speed and frame rate. Fast vehicles at low frame rates need larger gates.
- `process_noise_vel` should be higher for locations with frequent acceleration/braking (intersections, hills).
- `measurement_noise` should reflect the actual sensor noise at the deployment range. Closer deployments can use lower values.

### Frame Handling

These control frame assembly and persistence.

| Parameter          | Type   | Default Range | Description                                                           |
| ------------------ | ------ | ------------- | --------------------------------------------------------------------- |
| `buffer_timeout`   | string | —             | Maximum wait time for a complete frame (Go duration, e.g. `"500ms"`). |
| `flush_interval`   | string | —             | How often the background grid is flushed to disk (e.g. `"60s"`).      |
| `background_flush` | bool   | —             | If true, enables periodic background grid flush to disk.              |

**Operational guidance:**

- These are typically left at defaults during tuning. Only adjust if frame assembly or persistence behaviour needs changing.

---

## Metrics

The sampler collects these metrics per iteration for each parameter combination. They are aggregated as means and standard deviations across all iterations for scoring.

### Core Metrics

| Metric            | Range | Meaning                                                                                      | Good Direction                                        |
| ----------------- | ----- | -------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| `acceptance_rate` | 0–1   | Fraction of background cells where the current observation falls within the noise threshold. | Higher = background model fits well                   |
| `nonzero_cells`   | 0–N   | Number of background cells with at least one observation.                                    | Higher = better coverage                              |
| `active_tracks`   | 0–N   | Number of non-deleted tracks at sample time.                                                 | Higher = more detections (but watch for false tracks) |

### Alignment Metrics

| Metric               | Range | Meaning                                                                                      | Good Direction                        |
| -------------------- | ----- | -------------------------------------------------------------------------------------------- | ------------------------------------- |
| `alignment_deg`      | 0–180 | Mean angular difference between velocity heading and displacement heading across all tracks. | Lower = velocity aligns with movement |
| `misalignment_ratio` | 0–1   | Fraction of track updates where angular difference exceeds 45 degrees.                       | Lower = fewer misaligned tracks       |

### Track Health Metrics

| Metric                | Range | Meaning                                                                              | Good Direction              |
| --------------------- | ----- | ------------------------------------------------------------------------------------ | --------------------------- |
| `heading_jitter_deg`  | 0+    | RMS of frame-to-frame heading changes across all tracks.                             | Lower = smoother headings   |
| `fragmentation_ratio` | 0–1   | `1 - (confirmed / created)`. Fraction of tentative tracks that were never confirmed. | Lower = fewer wasted tracks |

### Scene-Level Metrics

| Metric                     | Range | Meaning                                                                           | Good Direction                      |
| -------------------------- | ----- | --------------------------------------------------------------------------------- | ----------------------------------- |
| `foreground_capture_ratio` | 0–1   | Fraction of foreground points captured by DBSCAN clusters.                        | Higher = fewer orphaned points      |
| `unbounded_point_ratio`    | 0–1   | `1 - foreground_capture_ratio`. Fraction of foreground points not in any cluster. | Lower                               |
| `empty_box_ratio`          | 0–1   | Fraction of active-track frames where no cluster was associated with the track.   | Lower = tracks consistently matched |

---

## Scoring

### Objective Modes

The auto-tuner supports three objective modes:

| Mode           | When to Use                                                                                   |
| -------------- | --------------------------------------------------------------------------------------------- |
| `acceptance`   | Quick tuning focused on background model fit. Maximises acceptance rate only.                 |
| `weighted`     | Full tuning with multiple metrics. Uses configurable weights to balance competing objectives. |
| `ground_truth` | Precision tuning against labelled reference tracks. Requires a scene with reference run.      |

### Weighted Multi-Objective Scoring

The weighted scoring function computes:

```
score = acceptance × w_acceptance
      + misalignment_ratio × w_misalignment
      + alignment_deg × w_alignment
      + log(nonzero_cells) × w_nonzero
      + log(active_tracks) × w_tracks
      + foreground_capture × w_capture
      + empty_box_ratio × w_empty
      + fragmentation_ratio × w_frag
      + heading_jitter × w_jitter
```

**Sign convention:** Positive weights maximise the metric; negative weights minimise it. Metrics where "lower is better" (misalignment, alignment, empty boxes, fragmentation, jitter) should have negative weights.

**Default weights:**

| Weight               | Field                    | Default | Effect                                     |
| -------------------- | ------------------------ | ------- | ------------------------------------------ |
| `acceptance`         | Acceptance rate          | 1.0     | Maximise                                   |
| `misalignment`       | Misalignment ratio       | -0.5    | Minimise                                   |
| `alignment`          | Alignment degrees        | -0.01   | Minimise (small weight — degrees vs ratio) |
| `nonzero_cells`      | Nonzero cells (log)      | 0.1     | Maximise (log scale dampens large values)  |
| `active_tracks`      | Active tracks (log)      | 0.3     | Maximise (log scale dampens large values)  |
| `foreground_capture` | Foreground capture ratio | 0       | Off by default; set positive to enable     |
| `empty_boxes`        | Empty box ratio          | 0       | Off by default; set negative to enable     |
| `fragmentation`      | Fragmentation ratio      | 0       | Off by default; set negative to enable     |
| `heading_jitter`     | Heading jitter degrees   | 0       | Off by default; set negative to enable     |

The scene-level weights (`foreground_capture`, `empty_boxes`, `fragmentation`, `heading_jitter`) are opt-in. Enable them when you care about track quality beyond simple acceptance rate.

**Recommended weight sets:**

For general vehicle tracking:

```json
{
  "acceptance": 1.0,
  "misalignment": -0.5,
  "alignment": -0.01,
  "nonzero_cells": 0.1,
  "active_tracks": 0.3,
  "fragmentation": -0.3,
  "heading_jitter": -0.1
}
```

For high-quality speed measurement:

```json
{
  "acceptance": 0.5,
  "misalignment": -1.0,
  "alignment": -0.05,
  "active_tracks": 0.3,
  "fragmentation": -0.5,
  "heading_jitter": -0.3,
  "empty_boxes": -0.2
}
```

### Acceptance Criteria

Acceptance criteria are **hard thresholds** that reject parameter combinations before scoring. A combination that fails any criterion receives a score of negative infinity and sorts to the bottom. Use these to exclude clearly unacceptable configurations.

| Criterion                   | Field                                 | Effect                                                            |
| --------------------------- | ------------------------------------- | ----------------------------------------------------------------- |
| `max_fragmentation_ratio`   | Maximum allowed fragmentation ratio   | Rejects combos where too many tracks are never confirmed          |
| `max_unbounded_point_ratio` | Maximum allowed unbounded point ratio | Rejects combos where too many foreground points escape clustering |
| `max_empty_box_ratio`       | Maximum allowed empty box ratio       | Rejects combos where tracks frequently lack cluster associations  |

All fields are optional (nil = no constraint). Example:

```json
{
  "max_fragmentation_ratio": 0.5,
  "max_unbounded_point_ratio": 0.3,
  "max_empty_box_ratio": 0.4
}
```

### Ground Truth Scoring

When `objective` is `"ground_truth"`, scoring uses labelled reference tracks from a scene instead of live metrics. The composite score formula:

```
composite = w1 × detection_rate
          + w4 × velocity_coverage
          + w5 × quality_premium
          + w8 × stopped_recovery
          - w2 × fragmentation
          - w3 × false_positives
          - w6 × truncation_rate
          - w7 × velocity_noise_rate
```

**Default ground truth weights:**

| Weight | Field                 | Default | Meaning                                                     |
| ------ | --------------------- | ------- | ----------------------------------------------------------- |
| w1     | `detection_rate`      | 1.0     | Fraction of "good\_\*" reference tracks matched             |
| w2     | `fragmentation`       | 5.0     | Penalty for reference tracks split into multiple candidates |
| w3     | `false_positives`     | 2.0     | Penalty for unmatched candidate tracks                      |
| w4     | `velocity_coverage`   | 0.5     | Bonus for matched tracks with velocity data                 |
| w5     | `quality_premium`     | 0.3     | Bonus for matched tracks with "perfect" quality             |
| w6     | `truncation_rate`     | 0.4     | Penalty for truncated tracks                                |
| w7     | `velocity_noise_rate` | 0.4     | Penalty for noisy velocity tracks                           |
| w8     | `stopped_recovery`    | 0.2     | Bonus for stopped vehicle recovery                          |

Note: `fragmentation` carries the highest default weight (5.0) because track fragmentation is the most common and impactful tracking failure mode.

---

## Auto-Tune Configuration

### Core Settings

| Setting            | Default | Description                                                                                |
| ------------------ | ------- | ------------------------------------------------------------------------------------------ |
| `max_rounds`       | 3       | Number of refinement rounds. More rounds = finer convergence but longer runtime.           |
| `values_per_param` | 5       | Grid density per parameter per round. Total combos = values_per_param^(number of params).  |
| `top_k`            | 5       | Number of best results used to narrow bounds for the next round.                           |
| `iterations`       | 30      | Samples collected per combination. More iterations = lower variance in metrics but slower. |
| `interval`         | "2s"    | Time between samples within a combination.                                                 |
| `settle_time`      | "5s"    | Time to wait for the background model to stabilise before sampling begins.                 |

### Settle Modes

| Mode                  | Behaviour                                                                         | When to Use                                                                                                   |
| --------------------- | --------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `per_combo` (default) | Full grid reset and re-settle for each combination.                               | Most accurate results. Use when you have time and want reliable comparisons.                                  |
| `once`                | Full settle on the first combination only; subsequent combos do a short ~2s wait. | Faster iteration (~10× fewer settle seconds). Use for initial exploration when settle time dominates runtime. |

The `once` mode leverages region persistence to restore the background model quickly. See [Settling Time Optimisation](settling-time-optimization.md) for details.

### Data Sources

| Source | Setting                 | Description                                                                   |
| ------ | ----------------------- | ----------------------------------------------------------------------------- |
| Live   | `"data_source": "live"` | Uses real-time sensor data. Each combination sees different traffic.          |
| PCAP   | `"data_source": "pcap"` | Replays a PCAP file. Each combination sees the same data — more reproducible. |

When using PCAP, specify `pcap_file` (basename only), `pcap_start_secs`, and `pcap_duration_secs`.

### Narrowing Algorithm

After each round, bounds for the next round are computed from the top K results:

1. Find the minimum and maximum value of each parameter across the top K combinations
2. If all top K share one value: add a margin of `max(10% of value, 0.001)` on each side
3. If a range exists: compute the step size as `(max - min) / (values_per_param - 1)`, then add 1 step margin on each side
4. Clamp to the original request bounds

This ensures each round zooms in on the most promising region while retaining enough margin to avoid missing nearby optima.

### Scaling Considerations

Total combinations per round = `values_per_param ^ number_of_params`. Safety limit: 1000 combinations per sweep.

| Params | Values/Param | Combos/Round | Rounds | Total Combos  |
| ------ | ------------ | ------------ | ------ | ------------- |
| 2      | 5            | 25           | 3      | 75            |
| 3      | 5            | 125          | 3      | 375           |
| 4      | 4            | 256          | 3      | 768           |
| 5      | 4            | 1024         | —      | Exceeds limit |

For 4+ parameters, reduce `values_per_param` to 3-4, or split into two tuning runs (background params first, then tracker params).

---

## Operational Decision-Making

### Which Parameters to Tune First

**Tier 1 — Background Model (tune first):**

- `noise_relative` — Most impactful single parameter. Directly controls foreground sensitivity.
- `closeness_multiplier` — Second most impactful. Controls position tolerance.
- `neighbor_confirmation_count` — Low-cost noise filter. 0 for clean environments, 3-5 for noisy ones.

**Tier 2 — Foreground Detection (tune second):**

- `foreground_dbscan_eps` — Controls cluster merging. Critical for correct vehicle segmentation.
- `foreground_min_cluster_points` — Simple noise gate. Tune after DBSCAN epsilon.

**Tier 3 — Tracker (tune after background/foreground are stable):**

- `hits_to_confirm` and `max_misses` — Fragmentation vs false-positive trade-off.
- `gating_distance_squared` — Association strictness. Tune if tracks swap between nearby vehicles.
- `process_noise_vel` — Tune if tracks don't follow acceleration/braking well.

**Tier 4 — Fine-tuning (rarely needed):**

- `process_noise_pos`, `measurement_noise`, `occlusion_cov_inflation` — These have reasonable defaults. Only adjust when tier 1-3 tuning is complete and specific quality issues remain.

### Diagnosing Problems from Metrics

| Symptom                        | Likely Cause                                     | Parameters to Adjust                                                                     |
| ------------------------------ | ------------------------------------------------ | ---------------------------------------------------------------------------------------- |
| Low acceptance rate (<0.7)     | Background model too tight                       | Increase `noise_relative`, increase `closeness_multiplier`                               |
| High acceptance rate (>0.98)   | Background model too loose (foreground absorbed) | Decrease `noise_relative`, decrease `closeness_multiplier`                               |
| High misalignment ratio (>0.3) | Tracks not following real objects                | Increase `hits_to_confirm`, decrease `gating_distance_squared`, check DBSCAN eps         |
| High fragmentation (>0.5)      | Tracks dying prematurely                         | Increase `max_misses`, increase `max_misses_confirmed`, decrease `hits_to_confirm`       |
| High heading jitter            | Noisy cluster centroids                          | Increase `foreground_min_cluster_points`, increase `measurement_noise`                   |
| Low foreground capture         | Foreground points not clustering                 | Increase `foreground_dbscan_eps`, decrease `foreground_min_cluster_points`               |
| High empty box ratio           | Tracks exist but lack cluster matches            | Increase `gating_distance_squared`, check sensor alignment                               |
| High unbounded point ratio     | Many foreground points outside clusters          | Increase `foreground_dbscan_eps`, decrease `noise_relative` (may create more foreground) |

### Choosing Objective Mode

1. **Start with `acceptance`** to quickly validate that background parameters produce a reasonable foreground/background split
2. **Switch to `weighted`** once acceptance rate is reasonable (0.8-0.95) and you want to optimise track quality
3. **Use `ground_truth`** when you have a labelled scene and need precision tuning for a specific deployment

### Setting Acceptance Criteria

Use acceptance criteria to pre-filter clearly bad configurations:

- `max_fragmentation_ratio: 0.5` — Reject if more than half of created tracks never confirm. Safe default for most deployments.
- `max_unbounded_point_ratio: 0.3` — Reject if more than 30% of foreground points aren't in clusters. Indicates DBSCAN is too restrictive.
- `max_empty_box_ratio: 0.4` — Reject if confirmed tracks lack cluster associations 40% of the time. Indicates tracking/clustering mismatch.

These are conservative thresholds. Tighten them once you have a baseline understanding of your deployment's typical metric ranges.

---

## Workflows

### Quick Tune (Background Parameters Only)

Tune the two most impactful parameters with acceptance-only scoring:

```json
POST /api/lidar/sweep/auto
{
  "params": [
    { "name": "noise_relative", "type": "float64", "start": 0.01, "end": 0.15 },
    { "name": "closeness_multiplier", "type": "float64", "start": 2.0, "end": 16.0 }
  ],
  "max_rounds": 3,
  "values_per_param": 5,
  "top_k": 5,
  "objective": "acceptance",
  "iterations": 10,
  "settle_time": "5s",
  "interval": "2s",
  "data_source": "pcap",
  "pcap_file": "capture.pcap",
  "settle_mode": "once"
}
```

75 total combinations. With `settle_mode: "once"` and 10 iterations at 2s intervals, each combo takes ~20s. Total: ~25 minutes.

### Full Tune (Weighted Multi-Objective)

Tune background + foreground + key tracker params with weighted scoring and acceptance criteria:

```json
POST /api/lidar/sweep/auto
{
  "params": [
    { "name": "noise_relative", "type": "float64", "start": 0.01, "end": 0.1 },
    { "name": "closeness_multiplier", "type": "float64", "start": 2.0, "end": 12.0 },
    { "name": "foreground_dbscan_eps", "type": "float64", "start": 0.3, "end": 1.5 },
    { "name": "hits_to_confirm", "type": "int", "start": 2, "end": 6 }
  ],
  "max_rounds": 3,
  "values_per_param": 4,
  "top_k": 5,
  "objective": "weighted",
  "weights": {
    "acceptance": 1.0,
    "misalignment": -0.5,
    "active_tracks": 0.3,
    "fragmentation": -0.3,
    "heading_jitter": -0.1
  },
  "acceptance_criteria": {
    "max_fragmentation_ratio": 0.5,
    "max_unbounded_point_ratio": 0.3
  },
  "iterations": 15,
  "settle_time": "10s",
  "interval": "2s",
  "data_source": "pcap",
  "pcap_file": "capture.pcap"
}
```

256 combinations per round, 768 total. This is a long run; use `settle_mode: "once"` to reduce runtime.

### Ground Truth Tune

Requires a scene with labelled reference tracks:

```json
POST /api/lidar/sweep/auto
{
  "params": [
    { "name": "noise_relative", "type": "float64", "start": 0.01, "end": 0.1 },
    { "name": "closeness_multiplier", "type": "float64", "start": 2.0, "end": 10.0 }
  ],
  "max_rounds": 3,
  "values_per_param": 5,
  "top_k": 5,
  "objective": "ground_truth",
  "scene_id": "scene-123",
  "ground_truth_weights": {
    "detection_rate": 1.0,
    "fragmentation": 5.0,
    "false_positives": 2.0,
    "velocity_coverage": 0.5,
    "quality_premium": 0.3,
    "truncation_rate": 0.4,
    "velocity_noise_rate": 0.4,
    "stopped_recovery": 0.2
  },
  "iterations": 10,
  "settle_time": "5s",
  "interval": "2s",
  "data_source": "pcap",
  "pcap_file": "urban-intersection.pcap"
}
```

**Ground truth workflow:**

1. Create a scene for the sensor location
2. Replay the PCAP with current parameters to create an initial run
3. Label tracks in the run (user_label + quality_label)
4. Set the run as the scene's reference
5. Run auto-tuning with `objective: "ground_truth"`
6. Optimal parameters are automatically saved to the scene

### Applying Results

After auto-tuning completes, the status response includes a `recommendation` object with the best parameter values. Apply them to the live system:

```json
POST /api/lidar/params
{
  "noise_relative": 0.035,
  "closeness_multiplier": 6.2,
  "foreground_dbscan_eps": 0.8,
  "hits_to_confirm": 3
}
```

Only the parameters included in the request are updated; others retain their current values.

### Checking Status

Poll sweep status during a run:

```
GET /api/lidar/sweep/status
```

The response includes `round`, `total_rounds`, `completed_combos`, `total_combos`, `round_results`, and (on completion) `recommendation`.

---

## Parameter Interaction Guide

This section documents known parameter interactions to help operators and AI agents make informed tuning decisions.

### Parameters That Should Be Tuned Together

- **`noise_relative` + `closeness_multiplier`**: Both control background sensitivity. Tuning one without the other can leave the overall sensitivity suboptimal. Always include both in a background tuning run.
- **`hits_to_confirm` + `max_misses` + `max_misses_confirmed`**: These form the track lifecycle policy. Changing one shifts the fragmentation/false-positive balance; the others should be considered as a group.
- **`process_noise_pos` + `process_noise_vel` + `measurement_noise`**: These are the Kalman filter noise parameters. They set the balance between trusting predictions vs observations. Tune as a group.
- **`foreground_dbscan_eps` + `foreground_min_cluster_points`**: Both affect cluster formation. A small epsilon with a high min-points threshold will suppress most clusters.

### Parameters to Hold Fixed During Initial Tuning

- **Warmup parameters** (`warmup_duration_nanos`, `warmup_min_frames`): Set these to known-good values before tuning other parameters. Changing warmup during a sweep makes results incomparable.
- **Frame handling** (`buffer_timeout`, `flush_interval`, `background_flush`): These affect data collection, not tracking quality. Hold fixed.
- **`seed_from_first`**: Choose true or false based on deployment type and hold fixed. Toggling per-combo adds confounding variation.
- **`post_settle_update_fraction`**: Set to 0 for PCAP analysis (frozen background). Only tune for live operation.

### Recommended Starting Ranges

**Urban intersection (mixed traffic, moderate clutter):**

- `noise_relative`: 0.02–0.08
- `closeness_multiplier`: 3.0–10.0
- `neighbor_confirmation_count`: 2–5
- `foreground_dbscan_eps`: 0.4–1.2
- `hits_to_confirm`: 2–5
- `max_misses_confirmed`: 5–15

**Residential street (light traffic, low clutter):**

- `noise_relative`: 0.01–0.05
- `closeness_multiplier`: 2.0–8.0
- `neighbor_confirmation_count`: 0–3
- `foreground_dbscan_eps`: 0.3–0.8
- `hits_to_confirm`: 2–4
- `max_misses_confirmed`: 3–10

**High-clutter environment (trees, glass, vegetation):**

- `noise_relative`: 0.05–0.2
- `closeness_multiplier`: 5.0–20.0
- `neighbor_confirmation_count`: 4–8
- `foreground_dbscan_eps`: 0.5–1.5
- `hits_to_confirm`: 3–6
- `max_misses_confirmed`: 5–20

### Convergence Tips

- If all top K results cluster at one edge of the range, the optimal value may lie outside the initial bounds. Expand the range in that direction and re-run.
- If scores plateau after round 2, the grid has likely converged. Additional rounds add precision but diminishing returns.
- If no combinations pass acceptance criteria, the criteria may be too strict. Relax thresholds and re-run, then tighten once a viable region is found.
- For n > 4 parameters, consider splitting into two sequential tuning runs: background parameters first (freeze foreground/tracker at defaults), then foreground + tracker (freeze background at the values found in run 1).

---

## Adaptive Region Parameters

After settling, the background grid automatically segments the sensor's field of view into regions based on variance characteristics. Each region receives scaled parameters:

| Region Type              | Noise Scale | Neighbour Bonus | Settle Scale | Typical Cells   |
| ------------------------ | ----------- | --------------- | ------------ | --------------- |
| Stable (low variance)    | 0.8× base   | 0               | 1.5× base    | Walls, pavement |
| Variable (medium)        | 1.0× base   | 0               | 1.0× base    | Distant foliage |
| Volatile (high variance) | 2.0× base   | +2              | 0.5× base    | Trees, glass    |

Auto-tuning optimises the **base** parameter values. Region scaling is applied on top. This means tuning `noise_relative` to 0.04 results in 0.032 for stable regions and 0.08 for volatile regions.

When interpreting auto-tuning results, keep in mind that the recommended values are base values subject to per-region scaling. See [Adaptive Region Parameters](adaptive-region-parameters.md) for details.

---

## Changelog

- **2026-02-10**: Consolidated from `auto-tuning.md`, `label-aware-auto-tuning-implementation.md`, and `label-aware-auto-tuning-usage.md` into unified guide
- **2026-02**: Phase 5 (ground truth evaluation) implemented
- **2026-02**: Phases 1–2 (iterative grid narrowing, multi-objective scoring) implemented
