# Parameter Comparison: Current vs. Optimised

Status: Active

This document provides a quick reference showing the parameter changes between the current defaults and the optimised configuration for addressing jitter, fragmentation, misalignment, and empty boxes.

## Side-by-Side Comparison

| Parameter                            | Current (tuning.defaults.json) | Optimised (tuning.optimised.json) | Change | Impact                                                       |
| ------------------------------------ | ------------------------------ | --------------------------------- | ------ | ------------------------------------------------------------ |
| **Background/Foreground Extraction** |                                |                                   |        |                                                              |
| `noise_relative`                     | 0.04                           | 0.02                              | ↓ 50%  | Tighter background acceptance → more foreground capture      |
| `closeness_multiplier`               | 8.0                            | 3.0                               | ↓ 62%  | **Major:** Reduces false negatives, captures vehicle edges   |
| `neighbor_confirmation_count`        | 7                              | 3                                 | ↓ 57%  | Easier foreground voting → fewer suppressed points           |
| `safety_margin_meters`               | 0.4                            | 0.15                              | ↓ 62%  | **Major:** Reduces edge-point suppression                    |
| **Clustering**                       |                                |                                   |        |                                                              |
| `foreground_min_cluster_points`      | 2                              | 5                                 | ↑ 150% | Rejects noise fragments → cleaner clusters                   |
| `foreground_dbscan_eps`              | 0.3                            | 0.7                               | ↑ 133% | **Major:** Merges sub-clusters → reduces fragmentation       |
| **Tracking/Association**             |                                |                                   |        |                                                              |
| `gating_distance_squared`            | 4.0                            | 25.0                              | ↑ 525% | **Major:** Allows re-association across gaps (2m→5m radius)  |
| `process_noise_vel`                  | 0.5                            | 0.3                               | ↓ 40%  | Constrains velocity drift → reduces speed jitter             |
| `measurement_noise`                  | 0.3                            | 0.15                              | ↓ 50%  | **Major:** Trusts observations → reduces heading jitter      |
| `hits_to_confirm`                    | 3                              | 4                                 | ↑ 33%  | Delays confirmation → allows cluster merging                 |
| **Unchanged (Already Optimal)**      |                                |                                   |        |                                                              |
| `background_update_fraction`         | 0.02                           | 0.02                              | -      | Stable background learning rate                              |
| `process_noise_pos`                  | 0.1                            | 0.1                               | -      | Position uncertainty already well-calibrated                 |
| `occlusion_cov_inflation`            | 0.5                            | 0.5                               | -      | Covariance growth during occlusion balanced                  |
| `max_misses`                         | 3                              | 3                                 | -      | Tentative track lifetime appropriate                         |
| `max_misses_confirmed`               | 15                             | 15                                | -      | Confirmed track coasting (1.5s at 10Hz) reasonable           |
| `max_tracks`                         | 100                            | 100                               | -      | Capacity sufficient for typical scenes                       |
| `height_band_floor`                  | −2.8                           | −2.8                              | -      | Sensor-frame floor (≈ 0.2 m above road for 3 m mount)        |
| `height_band_ceiling`                | 1.5                            | 1.5                               | -      | Sensor-frame ceiling (tall trucks up to ~1.5 m above sensor) |

## Critical Changes (Highest Impact)

These four parameters have the most significant effect on the observed issues:

### 1. `closeness_multiplier`: 8.0 → 3.0

- **Problem:** 8.0 is extremely loose, allowing vehicle points to be misclassified as background
- **Effect:** Causes empty boxes (tracks without clusters)
- **Fix:** 3.0 is the standard value, tightens foreground classification

### 2. `foreground_dbscan_eps`: 0.3 → 0.7

- **Problem:** 0.3m is too small for distant vehicles (point spacing grows with range)
- **Effect:** Splits single vehicles into multiple clusters → fragmentation
- **Fix:** 0.7m is the recommended default, merges sub-clusters

### 3. `gating_distance_squared`: 4.0 → 25.0

- **Problem:** 4.0 (2m radius) is too tight for occlusion gaps and re-association
- **Effect:** Prevents track merging → fragmentation, prevents re-acquisition → empty boxes
- **Fix:** 25.0 (5m radius) allows association across occlusion without false merging

### 4. `measurement_noise`: 0.3 → 0.15

- **Problem:** 0.3 causes Kalman to distrust observations → ignores cluster measurements
- **Effect:** OBB heading noise amplified → heading jitter, position lags → misalignment
- **Fix:** 0.15 balances trust in observations with robustness to outliers

## Expected Metric Improvements

| Metric                      | Current     | Optimised   | Change |
| --------------------------- | ----------- | ----------- | ------ |
| **HeadingJitterDeg (mean)** | 45-60°      | 15-25°      | ↓ 60%  |
| **SpeedJitterMps (mean)**   | 2.0-3.0 m/s | 0.5-1.0 m/s | ↓ 70%  |
| **FragmentationRatio**      | 0.40-0.50   | 0.10-0.15   | ↓ 75%  |
| **MisalignmentRatio**       | 0.30-0.40   | 0.10-0.15   | ↓ 65%  |
| **EmptyBoxRatio**           | 0.15-0.25   | 0.05-0.10   | ↓ 60%  |
| **ForegroundCapture**       | 0.70-0.75   | 0.85-0.90   | ↑ 15%  |

## Deployment Recommendation

**Incremental Rollout:**

1. **Test Stage 1** (Foreground Only): Apply closeness_multiplier, safety_margin, neighbor_confirmation_count changes
   - Duration: 15 minutes
   - Measure: EmptyBoxRatio should drop to ~0.10
2. **Test Stage 2** (Add Clustering): Apply foreground_dbscan_eps, foreground_min_cluster_points changes
   - Duration: 15 minutes
   - Measure: FragmentationRatio should drop to ~0.15
3. **Test Stage 3** (Add Tracking): Apply gating_distance_squared, process_noise_vel, measurement_noise changes
   - Duration: 30 minutes
   - Measure: Jitter metrics should halve

**Full Deployment:**

If all three stages succeed, deploy the complete `tuning.optimised.json` configuration for overnight testing.

## Usage

To apply the optimised configuration:

```bash
# Use the optimised config with the velocity-report binary
velocity-report --config config/tuning.optimised.json

# Or trigger via the monitor API for runtime parameter updates
curl -X POST http://localhost:8080/api/lidar/params \
  -H 'Content-Type: application/json' \
  -d @config/tuning.optimised.json
```

## Reverting Changes

If the optimised parameters cause unexpected issues, revert to defaults:

```bash
# Restore defaults by using the default config
velocity-report --config config/tuning.defaults.json
```

## Further Tuning

If the optimised configuration still shows quality issues, use auto-tuning to refine:

```json
{
  "params": [
    {
      "name": "foreground_dbscan_eps",
      "type": "float64",
      "start": 0.5,
      "end": 0.9,
      "step": 0.1
    },
    {
      "name": "gating_distance_squared",
      "type": "float64",
      "start": 16.0,
      "end": 36.0,
      "step": 4.0
    },
    {
      "name": "measurement_noise",
      "type": "float64",
      "start": 0.1,
      "end": 0.25,
      "step": 0.05
    }
  ],
  "objective": "weighted",
  "weights": {
    "acceptance": 1.0,
    "fragmentation": -5.0,
    "empty_boxes": -3.0,
    "heading_jitter": -2.0,
    "speed_jitter": -2.0
  }
}
```

## References

- **Full Analysis:** See `docs/lidar/troubleshooting/pipeline-diagnosis.md` for detailed explanations
- **Code References:**
  - Background extraction: `internal/lidar/foreground.go`
  - Clustering: `internal/lidar/clustering.go`
  - Tracking: `internal/lidar/tracking.go`
  - Metrics: `internal/lidar/sweep/objective.go`
