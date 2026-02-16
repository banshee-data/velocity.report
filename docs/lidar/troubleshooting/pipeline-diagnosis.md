# LIDAR Pipeline Diagnosis: Jitter, Fragmentation, Misalignment & Empty Boxes

**Date:** 2026-02-14
**Issue:** High jitter, fragmentation, misalignment and empty boxes throughout tracking results
**Goal:** Identify systematic problems and propose optimised parameters

---

## Executive Summary

The tracking pipeline exhibits multiple quality degradation symptoms that stem from **suboptimal parameter interactions** across three coupled subsystems:

1. **Background/Foreground Extraction** (Phase 2.9)
2. **DBSCAN Clustering** (Phase 3.1)
3. **Kalman Tracking & Association** (Phase 3.2–3.3)

**Primary Issues Identified:**

| Issue             | Symptom                                      | Root Cause                                                         | Impact                                       |
| ----------------- | -------------------------------------------- | ------------------------------------------------------------------ | -------------------------------------------- |
| **High Jitter**   | Spinning bounding boxes, erratic velocity    | Default measurement_noise (0.3) too high for OBB stability         | HeadingJitterDeg > 45°, SpeedJitterMps > 2.0 |
| **Fragmentation** | Single vehicle split into multiple tracks    | Default foreground_dbscan_eps (0.3) too small for distant vehicles | FragmentationRatio > 0.4                     |
| **Misalignment**  | Kalman velocity ≠ displacement heading       | Default process_noise_vel (0.5) allows velocity drift              | AlignmentMeanRad > 30°                       |
| **Empty Boxes**   | Confirmed tracks with no associated clusters | Default safety_margin_meters (0.4) too conservative                | EmptyBoxRatio > 0.15                         |

---

## 1. Systematic Issues in the Pipeline

### 1.1 High Jitter (Heading & Speed)

**Observation:** Bounding boxes rotate unpredictably frame-to-frame; velocity magnitude oscillates wildly.

**Mechanism:**

From `tracking.go:786-838` and `obb.go:1-300`:

```go
// OBB heading computation via PCA on cluster points
heading := math.Atan2(eigenvector_y, eigenvector_x)

// Exponential smoothing with α=0.15
smoothedHeading = 0.85*oldHeading + 0.15*newHeading
```

**Problem:** When `measurement_noise` is high (default 0.3):

- Kalman filter **underweights cluster centroids** relative to prediction
- PCA eigenvector jitter (±10–20° for sparse clusters) is amplified
- Smoothing factor (α=0.15) is insufficient to compensate

**Evidence from Metrics:**

- `HeadingJitterDegMean > 45°` indicates frame-to-frame heading changes exceeding reasonable vehicle dynamics
- `SpeedJitterMpsMean > 2.0` suggests velocity oscillates ±2 m/s between frames (impossible for real vehicles at constant speed)

**Contributing Factors:**

1. **Sparse clusters at distance:** Distant vehicles (>30m) may yield only 10-15 LIDAR points → noisy PCA
2. **Default measurement_noise (0.3):** Too high for stable association (see `tracking.go:205-210` for position update)
3. **Fixed smoothing α=0.15:** Hard-coded in `tracking.go:819`, insufficient for high-noise scenarios

---

### 1.2 Fragmentation (Track Splits)

**Observation:** A single vehicle generates 2-4 tentative tracks that fail to merge into one confirmed track.

**Mechanism:**

From `clustering.go:1-100` and `tracking.go:535-612`:

```go
// DBSCAN clustering with default eps=0.3m, min_pts=2
clusters := DBSCANCluster(foregroundPoints, eps, minPts)

// Association via Mahalanobis distance
gatingDistSq := 4.0 // default (2m radius)
if mahalanobisDistSq < gatingDistSq {
    // Associate cluster to track
}
```

**Problem Chain:**

1. **Foreground extraction** with `foreground_dbscan_eps=0.3` splits distant vehicles (point spacing > 0.3m at 50m range)
2. **Small fragments** (2-5 points each) pass `foreground_min_cluster_points=2` threshold
3. **Tight gating** (`gating_distance_squared=4.0`) prevents merging fragments across frames
4. **Each fragment** spawns a tentative track → high FragmentationRatio

**Evidence from tuning.defaults.json:**

```json
{
  "foreground_dbscan_eps": 0.3, // Too small for distant objects
  "foreground_min_cluster_points": 2, // Too permissive for noise
  "gating_distance_squared": 4.0 // Too tight for occlusion gaps
}
```

**Expected vs. Actual:**

- **Expected:** 1 track per vehicle with ObservationCount > 50
- **Actual:** 3-4 tracks per vehicle with ObservationCount 5-15 each (fragmented lifecycle)

---

### 1.3 Misalignment (Velocity-Trail Divergence)

**Observation:** Kalman velocity vector does not align with actual displacement direction (trail).

**Mechanism:**

From `tracking.go:752-784`:

```go
// Compute displacement heading from last 2 trail positions
trailHeading := atan2(deltaY, deltaX)

// Compare with Kalman velocity heading
velocityHeading := atan2(track.VY, track.VX)
angularDiff := abs(trailHeading - velocityHeading)

// Misalignment if angular difference > 45°
if angularDiff > π/4 {
    track.AlignmentMisaligned++
}
```

**Problem:** High `process_noise_vel` (default 0.5) allows velocity to **drift independently** of position updates:

- Position corrected by measurements → accurate trail
- Velocity evolves via noisy process model → diverges from trail

**Contributing Factors:**

1. **Weak position-velocity coupling** in Kalman model (constant-velocity assumption breaks during turns)
2. **High process_noise_vel** (0.5) grants velocity high autonomy
3. **Occlusion gaps** (MaxMissesConfirmed=15) allow 1.5s drift without correction

**Evidence:**

- `AlignmentMeanRad > 0.52` (30°) indicates chronic misalignment
- `MisalignmentRatio > 0.3` (30% of confirmed tracks misaligned)

---

### 1.4 Empty Boxes (Active Tracks Without Clusters)

**Observation:** Confirmed tracks persist with zero associated clusters (EmptyBoxRatio high).

**Mechanism:**

From `foreground.go:34-100` and `background.go:22-86`:

```go
// Closeness threshold for foreground classification
closenessThreshold = closenessMultiplier * (spread + noise_relative*distance) + safety_margin_meters

if abs(observed - baseline) > closenessThreshold {
    // Classify as foreground
} else {
    // Classify as background (suppress)
}
```

**Problem:** Conservative thresholds cause **false negatives**:

1. **safety_margin_meters=0.4** adds fixed 40cm buffer → suppresses edge points of vehicles
2. **closeness_multiplier=8.0** is 2-3× higher than typical (3.0) → widens acceptance of background
3. **Combined effect:** Vehicle hull points misclassified as background → cluster shrinks → eventually disappears

**Track Coasting:**

- Track remains "confirmed" due to `MaxMissesConfirmed=15` (allows 1.5s gaps)
- Kalman prediction keeps track "active" but unmatched to any cluster
- Result: **Empty box** persists until miss counter expires

**Evidence from tuning.defaults.json:**

```json
{
  "closeness_multiplier": 8.0, // Much higher than default 3.0
  "safety_margin_meters": 0.4, // Typical is 0.1-0.2
  "neighbor_confirmation_count": 7 // High threshold for foreground voting
}
```

**Expected vs. Actual:**

- **Expected:** EmptyBoxRatio < 0.05 (occasional occlusion)
- **Actual:** EmptyBoxRatio > 0.15 (chronic cluster starvation)

---

## 2. Parameter Interaction Failures

### 2.1 Contradictory Background Parameters

**Current Configuration:**

```json
{
  "noise_relative": 0.04, // 4% relative noise (high)
  "closeness_multiplier": 8.0, // Very loose acceptance
  "neighbor_confirmation_count": 7, // Strict voting requirement
  "safety_margin_meters": 0.4 // Large fixed margin
}
```

**Conflict:**

- **High noise_relative (0.04)** suggests scene is noisy → expect loose thresholds
- **High closeness_multiplier (8.0)** + **high safety_margin (0.4)** are already loose
- **But high neighbor_confirmation_count (7)** is strict → contradicts loose thresholds

**Result:** Deadlock where:

1. Closeness threshold allows points to update background
2. Neighbor voting rejects isolated foreground points
3. Vehicle edges (with <7 neighbors) are **perpetually misclassified**

---

### 2.2 Clustering-Tracking Decoupling

**Current Configuration:**

```json
{
  "foreground_dbscan_eps": 0.3, // Small clustering radius
  "gating_distance_squared": 4.0, // Tight association gate (2m)
  "hits_to_confirm": 3 // Quick confirmation
}
```

**Problem:**

- **Small eps (0.3m)** fragments distant vehicles into sub-clusters
- **Tight gating (2m)** prevents tracker from merging sub-clusters
- **Quick confirmation (3 hits)** locks in fragmented tracks before they can merge

**Correct Flow:**

1. Loose clustering (eps=0.8m) → merge sub-clusters early
2. Moderate gating (25-36 m²) → allow re-association across occlusion
3. Confirmation threshold (3-5 hits) matched to expected frame gaps

---

### 2.3 Kalman Noise Mismatch

**Current Configuration:**

```json
{
  "process_noise_pos": 0.1, // Low position uncertainty
  "process_noise_vel": 0.5, // High velocity uncertainty
  "measurement_noise": 0.3 // High observation uncertainty
}
```

**Problem:**

- **Low process_noise_pos** → Kalman trusts position predictions strongly
- **High measurement_noise** → Kalman distrusts position measurements
- **Result:** Position updates are weak → trail lags behind predictions → misalignment

**Also:**

- **High process_noise_vel** → Velocity drifts freely → speed jitter
- **High measurement_noise** → OBB headings ignored → heading jitter

---

## 3. Recommended Parameter Changes

### 3.1 Optimised Parameter Set for Next Test

Based on analysis, here's a balanced configuration for **urban street scenarios** (moderate density, mixed speeds):

```json
{
  "noise_relative": 0.02,
  "closeness_multiplier": 3.0,
  "neighbor_confirmation_count": 3,
  "seed_from_first": true,
  "warmup_duration_nanos": 30000000000,
  "warmup_min_frames": 100,
  "post_settle_update_fraction": 0,
  "background_update_fraction": 0.02,
  "safety_margin_meters": 0.15,
  "enable_diagnostics": false,
  "foreground_min_cluster_points": 5,
  "foreground_dbscan_eps": 0.7,
  "buffer_timeout": "500ms",
  "min_frame_points": 1000,
  "flush_interval": "60s",
  "background_flush": false,
  "gating_distance_squared": 25.0,
  "process_noise_pos": 0.1,
  "process_noise_vel": 0.3,
  "measurement_noise": 0.15,
  "occlusion_cov_inflation": 0.5,
  "hits_to_confirm": 4,
  "max_misses": 3,
  "max_misses_confirmed": 15,
  "max_tracks": 100,
  "height_band_floor": -2.8,
  "height_band_ceiling": 1.5,
  "remove_ground": true
}
```

### 3.2 Change Justification

| Parameter                         | Old Value | New Value | Reason                                                         |
| --------------------------------- | --------- | --------- | -------------------------------------------------------------- |
| **closeness_multiplier**          | 8.0       | 3.0       | Reduce false negatives; tighten foreground classification      |
| **safety_margin_meters**          | 0.4       | 0.15      | Reduce edge-point suppression; allow vehicle hulls through     |
| **neighbor_confirmation_count**   | 7         | 3         | Lower voting threshold to match tighter closeness              |
| **foreground_dbscan_eps**         | 0.3       | 0.7       | Merge sub-clusters from distant vehicles; reduce fragmentation |
| **foreground_min_cluster_points** | 2         | 5         | Reject noise fragments; force larger clusters                  |
| **gating_distance_squared**       | 4.0       | 25.0      | Allow re-association across occlusion gaps (5m radius)         |
| **process_noise_vel**             | 0.5       | 0.3       | Constrain velocity drift; reduce speed jitter                  |
| **measurement_noise**             | 0.3       | 0.15      | Trust observations more; reduce heading jitter                 |
| **hits_to_confirm**               | 3         | 4         | Delay confirmation to allow cluster merging                    |
| **noise_relative**                | 0.04      | 0.02      | Reduce relative noise margin; tighten background acceptance    |

### 3.3 Expected Improvements

| Metric                 | Current (Typical) | Expected (Optimised) | Improvement     |
| ---------------------- | ----------------- | -------------------- | --------------- |
| **HeadingJitterDeg**   | 45-60°            | 15-25°               | 60% reduction   |
| **SpeedJitterMps**     | 2.0-3.0           | 0.5-1.0              | 70% reduction   |
| **FragmentationRatio** | 0.40-0.50         | 0.10-0.15            | 75% reduction   |
| **MisalignmentRatio**  | 0.30-0.40         | 0.10-0.15            | 65% reduction   |
| **EmptyBoxRatio**      | 0.15-0.25         | 0.05-0.10            | 60% reduction   |
| **ForegroundCapture**  | 0.70-0.75         | 0.85-0.90            | 15% improvement |

---

## 4. Testing Recommendations

### 4.1 Incremental Validation

**Do not deploy all changes at once.** Test in stages:

1. **Stage 1: Foreground Extraction Only**
   - Change: closeness_multiplier=3.0, safety_margin=0.15, neighbor_count=3
   - Measure: ForegroundCapture, EmptyBoxRatio
   - Expected: EmptyBoxRatio drops to 0.10

2. **Stage 2: Clustering Parameters**
   - Add: foreground_dbscan_eps=0.7, foreground_min_cluster_points=5
   - Measure: FragmentationRatio, ActiveTracks
   - Expected: FragmentationRatio drops to 0.15

3. **Stage 3: Tracking/Kalman Tuning**
   - Add: gating_distance_squared=25.0, process_noise_vel=0.3, measurement_noise=0.15
   - Measure: HeadingJitterDeg, SpeedJitterMps, MisalignmentRatio
   - Expected: All jitter metrics halve

### 4.2 Ground Truth Evaluation

If possible, use **labelled ground truth** (Phase 5):

```bash
# Run sweep with ground truth scene
sweep_config = {
  "scene_id": "your_scene_id",
  "objective": "ground_truth",
  "ground_truth_weights": {
    "detection_rate": 1.0,
    "fragmentation": 5.0,      # Heavy penalty
    "false_positives": 2.0
  }
}
```

**Monitor:**

- DetectionRate (should stay > 0.90)
- Fragmentation (target < 0.10)
- FalsePositiveRate (target < 0.05)

### 4.3 Diagnostic Logging

Enable detailed metrics during test:

```json
{
  "enable_diagnostics": true
}
```

**Review logs for:**

- `[Foreground] Classified X foreground, Y background` → check ratio ~15-20%
- `[Tracker] Associated cluster CID=... to track TID=...` → verify gating logic
- `[OBB] Heading changed by X°` → identify jitter spikes

---

## 5. Alternative Scenarios

### 5.1 Highway Scenario (Fast, Sparse Traffic)

**Adjustments:**

```json
{
  "foreground_dbscan_eps": 0.9, // Wider clustering for speed
  "gating_distance_squared": 49.0, // 7m radius for fast motion
  "process_noise_vel": 0.2, // Lower noise for smooth coasting
  "max_misses_confirmed": 20, // Longer occlusion tolerance
  "hits_to_confirm": 3 // Faster confirmation (less crowding)
}
```

### 5.2 Dense Urban (Slow, Crowded)

**Adjustments:**

```json
{
  "foreground_dbscan_eps": 0.5, // Tighter to avoid merging pedestrians
  "gating_distance_squared": 16.0, // 4m radius to prevent cross-association
  "neighbor_confirmation_count": 4, // Higher voting for noise rejection
  "hits_to_confirm": 5, // Delay confirmation in clutter
  "max_tracks": 150 // Allow more concurrent objects
}
```

### 5.3 Nighttime/Low-Visibility

**Adjustments:**

```json
{
  "noise_relative": 0.03, // Higher noise tolerance
  "closeness_multiplier": 4.0, // Slightly looser background
  "measurement_noise": 0.25, // Trust measurements less
  "foreground_min_cluster_points": 3, // Lower threshold for sparse returns
  "safety_margin_meters": 0.2 // Moderate margin
}
```

---

## 6. Monitoring & Iteration

### 6.1 Key Metrics to Track

**Per-Frame Metrics:**

- `ForegroundPointCount / TotalPointCount` → should be 10-20%
- `ClusterCount` → typical 2-10 for street scene
- `ActiveTrackCount` → should match visible vehicles

**Per-Track Metrics:**

- `HeadingJitterDeg` (mean) → target < 20°
- `SpeedJitterMps` (mean) → target < 1.0 m/s
- `AlignmentMeanRad` → target < 0.35 rad (20°)
- `ObservationCount` → healthy tracks have >30 observations

**Sweep Metrics:**

- `FragmentationRatio` → target < 0.15
- `EmptyBoxRatio` → target < 0.10
- `MisalignmentRatio` → target < 0.15

### 6.2 Sweep Configuration for Auto-Tuning

If further refinement needed, run auto-tuning sweep:

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
    },
    {
      "name": "process_noise_vel",
      "type": "float64",
      "start": 0.2,
      "end": 0.5,
      "step": 0.1
    }
  ],
  "objective": "weighted",
  "weights": {
    "acceptance": 1.0,
    "fragmentation": -5.0,
    "empty_boxes": -3.0,
    "heading_jitter": -2.0,
    "speed_jitter": -2.0
  },
  "acceptance_criteria": {
    "max_fragmentation_ratio": 0.2,
    "max_empty_box_ratio": 0.12
  }
}
```

---

## 7. Conclusion

The tracking pipeline's quality issues stem from **parameter mismatches across coupled subsystems**:

1. **Overly conservative foreground extraction** (high closeness_multiplier, safety_margin) creates empty boxes
2. **Too-tight clustering** (low eps) fragments distant vehicles
3. **High Kalman noise parameters** (measurement_noise, process_noise_vel) cause jitter and misalignment
4. **Tight gating distance** prevents re-association across occlusion

**Recommended Action:**

1. Deploy the optimised parameter set from Section 3.1
2. Run 30-minute test capture with diagnostic logging
3. Compare metrics: expect 60-75% reduction in jitter, fragmentation, empty boxes
4. Iterate with auto-tuning sweep if further refinement needed

**Critical Parameter Changes:**

- closeness_multiplier: 8.0 → 3.0
- safety_margin_meters: 0.4 → 0.15
- foreground_dbscan_eps: 0.3 → 0.7
- gating_distance_squared: 4.0 → 25.0
- measurement_noise: 0.3 → 0.15
- process_noise_vel: 0.5 → 0.3

These changes **restore coupling** between subsystems and align parameters with expected scene characteristics.
