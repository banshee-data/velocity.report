# Foreground Trails Deep Investigation - January 8, 2026

**Status:** Design Document & Investigation Roadmap
**Previous Investigations:** 
- [foreground-trails-investigation-20260107.md](foreground-trails-investigation-20260107.md)
- [foreground-corruption-investigation-status.md](foreground-corruption-investigation-status.md)
**Related Design:** [velocity-coherent-foreground-extraction.md](../../internal/lidar/docs/future/velocity-coherent-foreground-extraction.md)

---

## Executive Summary

This document consolidates findings from previous investigations into foreground "trails" visible in the port 2370 feed and proposes a path forward for algorithm evolution. The key insight is that the current **background-subtraction approach** has fundamental limitations that may require evolution to a **velocity-coherent foreground extraction** algorithm.

**Key Findings:**

1. The current background model (EMA-based) is fundamentally reactive—it cannot distinguish between "static background returning after vehicle passes" and "slow-moving object that should remain foreground"
2. The freeze mechanism, while protecting against background corruption, creates 5-second windows where cells are unconditionally classified as foreground
3. The problem manifests as "trails" because cells take time to reconverge to background after freeze expires
4. An alternative approach using velocity coherence could eliminate these issues by tracking motion patterns rather than static range expectations

**Recommended Path:**

1. **Short-term (Phase A):** Implement algorithm harness for A/B comparison
2. **Medium-term (Phase B):** Implement simplified velocity-coherent extraction
3. **Long-term (Phase C):** Dual-pipeline production deployment

---

## Table of Contents

1. [Root Cause Analysis](#root-cause-analysis)
2. [Current Algorithm Deep Dive](#current-algorithm-deep-dive)
3. [Algorithm Comparison](#algorithm-comparison)
4. [Velocity-Coherent Algorithm Design](#velocity-coherent-algorithm-design)
5. [Algorithm Harness Architecture](#algorithm-harness-architecture)
6. [Evaluation Metrics & Methodology](#evaluation-metrics--methodology)
7. [Implementation Phases](#implementation-phases)
8. [Risk Assessment](#risk-assessment)

---

## Root Cause Analysis

### Symptom Recap

When a vehicle passes through the LIDAR sensor field of view:
1. Cells along the vehicle's path correctly classify vehicle points as foreground
2. After the vehicle leaves, the cells behind it should return to classifying the true background
3. **Problem:** Instead, these cells continue to emit foreground points for several seconds, creating "trails"

### Primary Root Causes

#### Cause 1: EMA-Based Background Model Hysteresis

The current algorithm uses Exponential Moving Average (EMA) to track background:

```go
// foreground.go line 305
newAvg := (1.0-updateAlpha)*oldAvg + updateAlpha*p.Distance
```

**Problem:** When a vehicle occludes a cell:
- The cell's confidence (`TimesSeenCount`) decreases because vehicle points don't match background
- When the vehicle leaves, the first background observation may still exceed the closeness threshold due to EMA drift during the occlusion
- The cell must "re-learn" the background through multiple observations

**Evidence from debug logs:**
```
[FG_DEBUG] r=35 az=324.4 dist=10.152 avg=10.159 diff=0.007 seen=26 recFg=0 isBg=true
[FG_DEBUG] r=35 az=324.4 dist=8.776  avg=10.159 diff=1.383 seen=25 recFg=1 isBg=false  ← Vehicle
[FG_DEBUG] r=35 az=324.4 dist=10.124 avg=10.155 diff=0.035 seen=26 recFg=0 isBg=true   ← Returns
```

In this case, the cell recovered correctly. However, when the vehicle lingers or the occlusion is longer, the EMA can drift significantly.

#### Cause 2: Freeze Mechanism Creates 5-Second Foreground Windows

The freeze mechanism prevents background corruption by locking cells when divergence is extreme:

```go
// foreground.go line 357-363
if cell.TimesSeenCount < 100 && cellDiff > FreezeThresholdMultiplier*closenessThreshold {
    cell.FrozenUntilUnixNanos = nowNanos + freezeDur
}
```

**Problem:** Frozen cells are unconditionally classified as foreground:

```go
// foreground.go line 177-180
if cell.FrozenUntilUnixNanos > nowNanos {
    foregroundMask[i] = true
    foregroundCount++
    continue
}
```

**Impact:** If `FreezeDurationNanos` is set to 5 seconds (default), any cell that triggers a freeze will emit foreground points for 5 full seconds, even if the vehicle left after 0.5 seconds.

#### Cause 3: recFg Accumulation During Freeze (Previously Fixed)

Investigation on 2026-01-07 identified that `RecentForegroundCount` was accumulating during freeze periods, reaching values of 70+. This was fixed by not incrementing recFg during freeze and resetting it on thaw.

However, **the fix only addresses the symptom**, not the root cause. The fundamental issue remains: the background model cannot distinguish between:
- "Vehicle left, I should reconverge to background quickly" 
- "Vehicle is still here, I should stay foreground"

#### Cause 4: No Motion Context in Classification

The current algorithm classifies each point **independently** based solely on its range vs. the cell's expected background range:

```go
// foreground.go line 255-257
isBackgroundLike := isWithinLockedRange ||
    cellDiff <= closenessThreshold ||
    (neighConfirm > 0 && neighborConfirmCount >= neighConfirm)
```

**Missing Context:**
- No knowledge of whether nearby points are also foreground (cluster coherence)
- No knowledge of whether the point is moving with a tracked object (velocity coherence)
- No temporal history beyond recFg counter

This is fundamentally different from how humans perceive moving objects in point clouds—we track motion patterns, not individual range deviations.

---

## Current Algorithm Deep Dive

### Data Flow Pipeline (Current)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     CURRENT: BACKGROUND SUBTRACTION                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  UDP Packets                                                                │
│      ↓                                                                      │
│  FrameBuilder (360° rotation assembly)                                      │
│      ↓                                                                      │
│  ProcessFramePolarWithMask()                                                │
│      │                                                                      │
│      ├──→ Per-cell classification (polar space)                             │
│      │    ├── EMA background model lookup                                   │
│      │    ├── Distance threshold check                                      │
│      │    ├── Neighbor confirmation voting                                  │
│      │    ├── Locked baseline comparison                                    │
│      │    └── Freeze state check                                            │
│      │                                                                      │
│      └──→ foregroundMask[] (boolean array)                                  │
│                ↓                                                            │
│  ExtractForegroundPoints()                                                  │
│      ↓                                                                      │
│  TransformToWorld() (polar → cartesian)                                     │
│      ↓                                                                      │
│  DBSCAN() clustering (eps=0.6m, minPts=12)                                  │
│      ↓                                                                      │
│  Tracker.Update() (Kalman filter, Mahalanobis gating)                       │
│      ↓                                                                      │
│  Classification & Persistence                                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Parameters (BackgroundParams)

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `BackgroundUpdateFraction` | 0.02 | EMA alpha for background learning |
| `ClosenessSensitivityMultiplier` | 3.0 | Multiplier for threshold calculation |
| `SafetyMarginMeters` | 0.5 | Fixed margin added to threshold |
| `FreezeDurationNanos` | 5e9 (5s) | Duration to freeze corrupted cells |
| `NeighborConfirmationCount` | 3 | Neighbors needed for background confirmation |
| `NoiseRelativeFraction` | 0.01 | Distance-proportional noise allowance |
| `LockedBaselineThreshold` | 50 | Observations before locking baseline |
| `LockedBaselineMultiplier` | 4.0 | Acceptance window for locked baseline |
| `ReacquisitionBoostMultiplier` | 5.0 | Alpha multiplier for fast re-acquisition |
| `MinConfidenceFloor` | 3 | Minimum TimesSeenCount to preserve |

### Classification Logic Flow

```
For each point p at (ring, azimuth, distance):

1. Check if cell is frozen → if yes, classify as FOREGROUND, skip
2. Check if freeze just expired → reset recFg counter
3. Calculate closeness threshold:
   threshold = closenessMultiplier * (spread + noiseRel*distance + 0.01) + safety
4. Calculate cell difference:
   cellDiff = |cell.AverageRangeMeters - p.Distance|
5. Check locked baseline (if available):
   isWithinLockedRange = |cell.LockedBaseline - p.Distance| <= lockedWindow
6. Count same-ring neighbors that agree this is background
7. Classification decision:
   isBackground = isWithinLockedRange || cellDiff <= threshold || neighborConfirm >= N
8. If classifying as foreground:
   - Decrement TimesSeenCount (but not below floor)
   - Increment RecentForegroundCount
   - Check if divergence extreme → trigger freeze
9. If classifying as background:
   - Update EMA (with reacquisition boost if recFg > 0)
   - Decay recFg
   - Update locked baseline if appropriate
```

### Strengths of Current Approach

1. **Memory efficient:** Only stores per-cell statistics, not per-point history
2. **Real-time capable:** O(n) per frame where n = points
3. **Stable for static scenes:** Quickly converges to stable background
4. **Parameter-tunable:** Many knobs for different environments

### Weaknesses of Current Approach

1. **No motion context:** Cannot distinguish stationary vs. moving foreground
2. **Hysteresis:** EMA takes time to reconverge after occlusion
3. **Freeze artifacts:** 5-second unconditional foreground windows
4. **No track continuity:** Each frame is independent
5. **MinPts=12 too high:** Discards sparse distant objects

---

## Algorithm Comparison

### Background Subtraction vs. Velocity-Coherent

| Aspect | Background Subtraction (Current) | Velocity-Coherent (Proposed) |
|--------|----------------------------------|------------------------------|
| **Core principle** | Compare range to learned static background | Track motion patterns across frames |
| **Frame independence** | Fully independent per frame | Requires multi-frame history |
| **Foreground detection** | Deviation from expected range | Consistent motion with tracked velocity |
| **Minimum cluster size** | 12 points (DBSCAN minPts) | 3 points (velocity-confirmed) |
| **Sparse object handling** | Poor (below minPts = noise) | Good (velocity confirms identity) |
| **Trail artifacts** | Yes (EMA hysteresis, freeze) | Eliminated (no background model needed) |
| **Pre-entry detection** | No (needs warmup) | Yes (velocity prediction) |
| **Post-exit handling** | Depends on reconvergence | Velocity prediction continues |
| **Computational cost** | O(n) per frame | O(n) + O(k²) for correspondence |
| **Memory requirement** | Per-cell statistics only | Multi-frame point history |
| **Static scene handling** | Excellent | Requires explicit static classification |

### When Each Algorithm Excels

**Background Subtraction is better for:**
- Static camera with stable background
- High-density point clouds (>100 points per object)
- Scenes where all motion is transient (cars passing, not stopping)
- Memory-constrained environments

**Velocity-Coherent is better for:**
- Sparse point clouds (distant objects with <12 points)
- Objects that stop and restart (pedestrians at crosswalks)
- Long-tail tracking (capture entire trajectory including entry/exit)
- Scenes with slow-moving objects that might be absorbed as background
- Avoiding trail artifacts completely

### Hybrid Approach Recommendation

Rather than replacing background subtraction entirely, **run both algorithms in parallel** and merge results:

```
Frame N points
    │
    ├──→ Background Subtraction → foregroundMask1
    │
    └──→ Velocity-Coherent Extraction → foregroundMask2
                │
                └──→ Union(mask1, mask2) → mergedMask
                         │
                         └──→ DBSCAN → Tracking
```

This provides:
- **Redundancy:** If one algorithm misses an object, the other may catch it
- **Comparison:** Quantitative metrics to evaluate each algorithm's contribution
- **Gradual migration:** Can tune the balance over time

---

## Velocity-Coherent Algorithm Design

### Overview

The velocity-coherent algorithm has 5 phases as described in [velocity-coherent-foreground-extraction.md](../../internal/lidar/docs/future/velocity-coherent-foreground-extraction.md):

1. **Phase 1: Point-Level Velocity Estimation** - Find frame-to-frame correspondences
2. **Phase 2: 6D DBSCAN Clustering** - Cluster by position AND velocity
3. **Phase 3: Long-Tail Track Management** - Pre-tail and post-tail prediction
4. **Phase 4: Sparse Continuation** - Maintain tracks with only 3 points
5. **Phase 5: Track Fragment Merging** - Reconnect split tracks

### Simplified Implementation for Traffic Monitoring

For our use case (roadside LIDAR, vehicles on ground plane), we can simplify:

**Simplification 1: 2D Position + 2D Velocity (4D instead of 6D)**
- Ground-plane assumption valid
- Reduces computational cost

**Simplification 2: Frame Correspondence via Predicted Position**
- For each point in frame N, search for correspondence in frame N-1 at predicted position based on median local velocity
- O(k*m) where k = foreground points, m = search candidates

**Simplification 3: Velocity Confidence as Cluster Filter**
- Instead of full 6D DBSCAN, run standard 2D DBSCAN then filter clusters by velocity coherence
- Clusters with inconsistent internal velocities are rejected

### Key Data Structures

```go
// PointWithVelocity extends a point with estimated velocity
type PointWithVelocity struct {
    X, Y, Z          float64
    VX, VY           float64  // Estimated velocity (m/s)
    Confidence       float32  // Velocity confidence [0, 1]
    CorrespondenceIdx int     // Index in previous frame (-1 if none)
    Timestamp        int64
}

// FrameHistory maintains sliding window for correspondence
type FrameHistory struct {
    Frames   []*VelocityFrame
    Capacity int
    Head     int
}

// VelocityFrame is a processed frame with spatial index
type VelocityFrame struct {
    Points        []PointWithVelocity
    SpatialIndex  *SpatialIndex
    Timestamp     time.Time
}
```

### Algorithm Pseudocode

```
function ProcessFrameVelocityCoherent(currentFrame):
    // Step 1: Get previous frame from history
    prevFrame = frameHistory.Previous(1)
    
    // Step 2: Estimate per-point velocities
    for each point p in currentFrame:
        if prevFrame == nil:
            p.Confidence = 0
            continue
        
        // Find correspondence in previous frame
        candidates = prevFrame.SpatialIndex.Query(p.X, p.Y, searchRadius)
        bestCorr = nil
        bestScore = infinity
        
        for each candidate c in candidates:
            // Score = spatial distance + velocity consistency with neighbors
            dist = euclidean(p, c)
            velDiff = |estimatedVelocity(p,c) - neighborMedianVelocity(p)|
            score = dist + velocityWeight * velDiff
            
            if score < bestScore:
                bestCorr = c
                bestScore = score
        
        if bestCorr != nil:
            p.VX = (p.X - bestCorr.X) / dt
            p.VY = (p.Y - bestCorr.Y) / dt
            p.Confidence = computeConfidence(bestScore, dist, velocityMagnitude)
            p.CorrespondenceIdx = bestCorr.Index
    
    // Step 3: Run 2D DBSCAN clustering
    clusters = DBSCAN(currentFrame, eps=0.6, minPts=3)  // Note: minPts reduced to 3
    
    // Step 4: Filter clusters by velocity coherence
    validClusters = []
    for each cluster in clusters:
        velocities = [p.VX, p.VY for p in cluster.points where p.Confidence > 0.3]
        if len(velocities) >= 2:
            variance = computeVelocityVariance(velocities)
            if variance < maxVelocityVariance:
                cluster.AvgVelocity = mean(velocities)
                cluster.VelocityConfidence = mean([p.Confidence for p in cluster.points])
                validClusters.append(cluster)
    
    // Step 5: Add current frame to history
    frameHistory.Add(currentFrame)
    
    return validClusters
```

### Key Parameters

| Parameter | Recommended Value | Purpose |
|-----------|-------------------|---------|
| `SearchRadius` | 2.0 m | Correspondence search radius |
| `MaxVelocityMps` | 50.0 m/s | Reject implausible velocities |
| `VelocityVarianceThreshold` | 2.0 m/s | Max variance within cluster |
| `MinConfidence` | 0.3 | Minimum velocity confidence |
| `MinPts` | 3 | Reduced from 12 (velocity confirms) |
| `PositionEps` | 0.6 m | DBSCAN neighborhood radius |
| `VelocityWeight` | 2.0 | Weight for velocity in 6D distance |
| `FrameHistoryCapacity` | 10 | Frames to keep for correspondence |

---

## Algorithm Harness Architecture

### Goals

1. **Run multiple algorithms simultaneously** on the same input data
2. **Record per-algorithm outputs** for comparison
3. **Compute comparative metrics** (precision, recall, F1)
4. **Support PCAP replay** for reproducible evaluation
5. **Enable parameter sweeps** without code changes

### Proposed Design

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ALGORITHM EVALUATION HARNESS                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Input Source                                                               │
│  (Live UDP / PCAP Replay)                                                   │
│          │                                                                  │
│          ▼                                                                  │
│  ┌──────────────────┐                                                       │
│  │  FrameBuilder    │                                                       │
│  │  (shared)        │                                                       │
│  └────────┬─────────┘                                                       │
│           │                                                                 │
│           ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐        │
│  │             ForegroundExtractor (interface)                      │        │
│  │                                                                  │        │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌────────────┐   │        │
│  │  │ BackgroundSub   │    │ VelocityCoherent│    │  Hybrid    │   │        │
│  │  │ Extractor       │    │ Extractor       │    │  Extractor │   │        │
│  │  │ (current)       │    │ (new)           │    │  (merged)  │   │        │
│  │  └───────┬─────────┘    └───────┬─────────┘    └─────┬──────┘   │        │
│  │          │                      │                    │          │        │
│  │          └──────────────────────┼────────────────────┘          │        │
│  │                                 │                               │        │
│  └─────────────────────────────────┼───────────────────────────────┘        │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────┐        │
│  │               Algorithm Comparison Logger                        │        │
│  │                                                                  │        │
│  │   Per-frame metrics:                                             │        │
│  │   - foreground_count_bs, foreground_count_vc, foreground_merged │        │
│  │   - clusters_bs, clusters_vc, clusters_merged                    │        │
│  │   - tracks_bs, tracks_vc, tracks_merged                          │        │
│  │   - processing_time_us per algorithm                             │        │
│  │                                                                  │        │
│  └───────────────────────────────────┬─────────────────────────────┘        │
│                                      │                                      │
│                                      ▼                                      │
│  ┌─────────────────────────────────────────────────────────────────┐        │
│  │                    Output Destinations                           │        │
│  │                                                                  │        │
│  │   - Port 2370: Merged foreground points                          │        │
│  │   - Port 2371: Background-subtraction only                       │        │
│  │   - Port 2372: Velocity-coherent only                            │        │
│  │   - SQLite: Analysis run records                                 │        │
│  │   - JSON: Per-frame comparison logs                              │        │
│  │                                                                  │        │
│  └─────────────────────────────────────────────────────────────────┘        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Interface Design

```go
// ForegroundExtractor is the interface for all foreground extraction algorithms
type ForegroundExtractor interface {
    // Name returns the algorithm name for logging/metrics
    Name() string
    
    // ProcessFrame extracts foreground points from a frame
    // Returns foreground mask and processing metrics
    ProcessFrame(points []PointPolar, timestamp time.Time) (
        foregroundMask []bool, 
        metrics ExtractorMetrics,
        err error,
    )
    
    // GetParams returns current algorithm parameters (for serialization)
    GetParams() map[string]interface{}
    
    // SetParams updates algorithm parameters at runtime
    SetParams(params map[string]interface{}) error
    
    // Reset clears internal state (for PCAP replay restart)
    Reset()
}

// ExtractorMetrics contains per-frame metrics from an extractor
type ExtractorMetrics struct {
    ForegroundCount   int
    BackgroundCount   int
    ProcessingTimeUs  int64
    AlgorithmSpecific map[string]interface{} // Algorithm-specific metrics
}

// BackgroundSubtractorExtractor wraps the existing BackgroundManager
type BackgroundSubtractorExtractor struct {
    Manager *BackgroundManager
}

// VelocityCoherentExtractor implements the new algorithm
type VelocityCoherentExtractor struct {
    FrameHistory *FrameHistory
    Config       VelocityCoherentConfig
    SensorID     string
}

// HybridExtractor runs multiple extractors and merges results
type HybridExtractor struct {
    Extractors []ForegroundExtractor
    MergeMode  MergeMode // "union", "intersection", "weighted"
}
```

### Evaluation Harness Implementation

```go
// EvaluationHarness runs multiple extractors and compares results
type EvaluationHarness struct {
    Extractors      []ForegroundExtractor
    ComparisonLogger *ComparisonLogger
    GroundTruth     *GroundTruthProvider // Optional: for labeled data
}

// ProcessFrame runs all extractors and records comparison
func (h *EvaluationHarness) ProcessFrame(points []PointPolar, timestamp time.Time) []FrameResult {
    results := make([]FrameResult, len(h.Extractors))
    
    for i, extractor := range h.Extractors {
        start := time.Now()
        mask, metrics, err := extractor.ProcessFrame(points, timestamp)
        elapsed := time.Since(start)
        
        results[i] = FrameResult{
            AlgorithmName:   extractor.Name(),
            ForegroundMask:  mask,
            Metrics:         metrics,
            ProcessingTime:  elapsed,
            Error:           err,
        }
    }
    
    // Log comparison
    if h.ComparisonLogger != nil {
        h.ComparisonLogger.LogComparison(timestamp, results)
    }
    
    // Compute against ground truth if available
    if h.GroundTruth != nil {
        for i, result := range results {
            results[i].Precision, results[i].Recall = h.computePrecisionRecall(
                result.ForegroundMask, 
                h.GroundTruth.GetMask(timestamp),
            )
        }
    }
    
    return results
}
```

### Database Schema for Algorithm Comparison

```sql
-- Algorithm comparison runs
CREATE TABLE IF NOT EXISTS lidar_algorithm_runs (
    run_id TEXT PRIMARY KEY,
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    
    -- Configuration
    algorithms_json TEXT,         -- JSON array of algorithm names
    params_json TEXT,             -- JSON object of per-algorithm params
    pcap_file TEXT,               -- Source PCAP if replay
    
    -- Aggregate metrics
    total_frames INTEGER,
    total_processing_time_us INTEGER
);

-- Per-frame comparison results
CREATE TABLE IF NOT EXISTS lidar_algorithm_frame_results (
    run_id TEXT NOT NULL,
    frame_unix_nanos INTEGER NOT NULL,
    algorithm_name TEXT NOT NULL,
    
    -- Metrics
    foreground_count INTEGER,
    background_count INTEGER,
    cluster_count INTEGER,
    processing_time_us INTEGER,
    
    -- Optional: precision/recall if ground truth available
    precision REAL,
    recall REAL,
    
    PRIMARY KEY (run_id, frame_unix_nanos, algorithm_name),
    FOREIGN KEY (run_id) REFERENCES lidar_algorithm_runs(run_id) ON DELETE CASCADE
);

CREATE INDEX idx_frame_results_run ON lidar_algorithm_frame_results(run_id);
CREATE INDEX idx_frame_results_algo ON lidar_algorithm_frame_results(algorithm_name);
```

---

## Evaluation Metrics & Methodology

### Quantitative Metrics

#### 1. Foreground Detection Metrics

| Metric | Definition | Target |
|--------|------------|--------|
| **Foreground Ratio** | foreground_points / total_points | 5-40% (scene-dependent) |
| **Trail Duration** | Time from vehicle exit to background reconvergence | <0.5s |
| **False Positive Rate** | foreground_points in static scene / total_points | <1% |
| **False Negative Rate** | missed_vehicle_points / vehicle_points | <5% |

#### 2. Clustering Metrics

| Metric | Definition | Target |
|--------|------------|--------|
| **Cluster Count Stability** | σ(cluster_count) over time for static scene | <0.5 |
| **Object Detection Rate** | detected_objects / true_objects | >95% |
| **Fragmentation Rate** | extra_clusters / true_objects | <10% |
| **Merge Rate** | merged_objects / true_objects | <5% |

#### 3. Tracking Metrics

| Metric | Definition | Target |
|--------|------------|--------|
| **MOTA** | Multi-Object Tracking Accuracy | >90% |
| **IDF1** | ID F1 Score (track identity preservation) | >85% |
| **Track Completeness** | observed_duration / true_duration | >90% |
| **Track Purity** | frames_with_correct_object / total_frames | >95% |

### Qualitative Evaluation

1. **Visual inspection in LidarView:**
   - Port 2370 (merged): Should show clean foreground without trails
   - Port 2371 (BS only): May show trails for comparison
   - Port 2372 (VC only): Should show velocity-confirmed points

2. **A/B testing with operators:**
   - Which feed appears more accurate?
   - Which feed has fewer artifacts?
   - Response time to object entry/exit

### Test Scenarios

| Scenario | Purpose | Expected Outcome |
|----------|---------|------------------|
| **Static scene** | Measure false positive rate | <1% foreground after warmup |
| **Single vehicle pass** | Measure trail duration | <0.5s after vehicle exits |
| **Multiple vehicles** | Measure fragmentation | Each vehicle = 1 track |
| **Pedestrian crossing** | Measure sparse object detection | Track maintained with 3+ points |
| **Vehicle stopping** | Measure background absorption | Vehicle remains foreground |
| **Occlusion event** | Measure track continuity | Track resumes after occlusion |

### Ground Truth Creation

For precision/recall evaluation, we need labeled data:

1. **Manual labeling tool:** Export frames to CloudCompare, label foreground manually
2. **Synthetic data:** Generate PCAP with known moving objects
3. **Cross-validation:** Use radar transit data as approximate ground truth

---

## Implementation Phases

### Phase A: Algorithm Harness (Week 1-2)

**Goal:** Enable parallel algorithm execution and comparison

**Tasks:**
1. [ ] Define `ForegroundExtractor` interface
2. [ ] Wrap existing `BackgroundManager` as `BackgroundSubtractorExtractor`
3. [ ] Implement `EvaluationHarness` with comparison logging
4. [ ] Add database schema for algorithm comparison runs
5. [ ] Add multi-port forwarding (2370/2371/2372)
6. [ ] Create CLI tool for algorithm comparison runs

**Deliverables:**
- `internal/lidar/extractor.go` - Interface and base types
- `internal/lidar/extractor_background.go` - BS wrapper
- `internal/lidar/evaluation_harness.go` - Comparison harness
- `cmd/tools/algo-compare/main.go` - CLI tool

**Validation:**
- [ ] Run PCAP with single algorithm, verify identical output to current
- [ ] Run PCAP with multiple algorithms, verify independent execution
- [ ] Verify comparison logs are written correctly

### Phase B: Velocity-Coherent Extractor (Week 3-4)

**Goal:** Implement simplified velocity-coherent algorithm

**Tasks:**
1. [ ] Implement `FrameHistory` ring buffer with spatial index
2. [ ] Implement point correspondence algorithm
3. [ ] Implement velocity confidence scoring
4. [ ] Implement velocity-filtered DBSCAN (minPts=3)
5. [ ] Create `VelocityCoherentExtractor` implementing interface
6. [ ] Add unit tests for velocity estimation
7. [ ] Add unit tests for correspondence matching

**Deliverables:**
- `internal/lidar/velocity_estimation.go` - Velocity computation
- `internal/lidar/frame_history.go` - Multi-frame buffer
- `internal/lidar/extractor_velocity_coherent.go` - VC extractor
- `internal/lidar/velocity_estimation_test.go` - Unit tests

**Validation:**
- [ ] Unit tests pass for known motion patterns
- [ ] PCAP replay shows velocity-coherent clusters
- [ ] Port 2372 shows velocity-confirmed foreground

### Phase C: Hybrid Extractor & Tuning (Week 5-6)

**Goal:** Merge algorithms and tune parameters

**Tasks:**
1. [ ] Implement `HybridExtractor` with configurable merge modes
2. [ ] Add parameter sweep infrastructure
3. [ ] Run comparison on multiple PCAP files
4. [ ] Analyze metrics to determine optimal configuration
5. [ ] Document recommended parameters

**Deliverables:**
- `internal/lidar/extractor_hybrid.go` - Merged extractor
- `internal/lidar/param_sweep.go` - Parameter sweep runner
- `docs/analysis/algorithm-comparison-results.md` - Analysis report

**Validation:**
- [ ] Trail duration reduced to <0.5s
- [ ] False positive rate remains <1%
- [ ] Object detection rate maintained at >95%

### Phase D: Production Deployment (Week 7-8)

**Goal:** Deploy hybrid algorithm to production

**Tasks:**
1. [ ] Add CLI flags for algorithm selection
2. [ ] Update web UI to show algorithm comparison
3. [ ] Add runtime algorithm switching
4. [ ] Document operational procedures
5. [ ] Create runbook for troubleshooting

**Deliverables:**
- Updated `cmd/radar/radar.go` with algorithm flags
- Updated web UI with algorithm comparison view
- `docs/operations/algorithm-selection.md` - Operations guide

**Validation:**
- [ ] Production deployment on test sensor
- [ ] 24h monitoring with no regressions
- [ ] Operator feedback incorporated

---

## Risk Assessment

### Technical Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Velocity estimation fails for slow objects | Medium | High | Fall back to background subtraction for low-confidence velocities |
| Frame history memory usage | Low | Medium | Ring buffer with configurable capacity |
| Correspondence matching CPU cost | Medium | Medium | Spatial indexing, limit search radius |
| Velocity-coherent misses stationary objects | High | High | Hybrid approach unions both algorithms |

### Operational Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Algorithm switch causes service disruption | Low | High | Runtime switching with fallback |
| Parameter changes break tracking | Medium | Medium | Parameter validation, bounded ranges |
| Ground truth labeling is labor-intensive | High | Low | Start with synthetic/radar validation |

### Mitigation Strategy

1. **Parallel operation:** Always run both algorithms, never fully disable background subtraction
2. **Feature flags:** All new code behind feature flags for easy rollback
3. **Incremental rollout:** Test on one sensor before full deployment
4. **Monitoring:** Add alerts for anomalous foreground ratios

---

## Appendix A: Files to Modify/Create

### Existing Files to Modify

| File | Modification |
|------|--------------|
| `internal/lidar/foreground.go` | Extract interface, no logic changes |
| `internal/lidar/tracking_pipeline.go` | Add extractor selection |
| `cmd/radar/radar.go` | Add CLI flags for algorithm selection |

### New Files to Create

| File | Purpose |
|------|---------|
| `internal/lidar/extractor.go` | ForegroundExtractor interface |
| `internal/lidar/extractor_background.go` | BackgroundSubtractorExtractor |
| `internal/lidar/extractor_velocity_coherent.go` | VelocityCoherentExtractor |
| `internal/lidar/extractor_hybrid.go` | HybridExtractor |
| `internal/lidar/velocity_estimation.go` | Point velocity estimation |
| `internal/lidar/frame_history.go` | Multi-frame ring buffer |
| `internal/lidar/evaluation_harness.go` | Algorithm comparison |
| `internal/lidar/extractor_test.go` | Interface tests |
| `internal/lidar/velocity_estimation_test.go` | Velocity tests |
| `cmd/tools/algo-compare/main.go` | Algorithm comparison CLI |

---

## Appendix B: Related Documentation

- [foreground-trails-investigation-20260107.md](foreground-trails-investigation-20260107.md) - Previous investigation
- [foreground-corruption-investigation-status.md](foreground-corruption-investigation-status.md) - Initial investigation
- [velocity-coherent-foreground-extraction.md](../../internal/lidar/docs/future/velocity-coherent-foreground-extraction.md) - Algorithm design
- [foreground_tracking_plan.md](../../internal/lidar/docs/architecture/foreground_tracking_plan.md) - Implementation plan
- [lidar-foreground-tracking-status.md](../../internal/lidar/docs/operations/lidar-foreground-tracking-status.md) - Operations status

---

## Appendix C: Glossary

| Term | Definition |
|------|------------|
| **Background Subtraction (BS)** | Algorithm that learns static scene and classifies deviations as foreground |
| **Velocity-Coherent (VC)** | Algorithm that tracks motion patterns to identify foreground |
| **EMA** | Exponential Moving Average, used for background model learning |
| **Freeze** | Mechanism to lock cells when extreme divergence detected |
| **recFg** | RecentForegroundCount, counter for fast re-acquisition |
| **MinPts** | DBSCAN parameter for minimum cluster size |
| **Trail** | Artifact where foreground lingers after object exits |
| **Correspondence** | Matching points between consecutive frames |
| **Gating** | Rejecting unlikely associations based on distance threshold |

---

**Document Status:** Complete
**Next Action:** Begin Phase A implementation (Algorithm Harness)
**Last Updated:** January 8, 2026
**Author:** Ictinus (Product Architecture Agent)
