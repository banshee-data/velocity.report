# Velocity-Coherent Foreground Extraction

**Status:** Implementation In Progress (Core Phases 1–5 Simplified; Phases 0, 6–7 Pending)
**Original Design Date:** December 15, 2025
**Plan Version:** 2.0 (February 21, 2026)
**Author:** Ictinus (Product Architecture Agent)

> **Note:** This is the living design document and implementation checklist. The active foreground extractor is `ProcessFramePolarWithMask` in `internal/lidar/l3grid/foreground.go`; the active clustering is DBSCAN in `internal/lidar/l4perception/cluster.go`. No `VelocityCoherentTracker` exists yet in the codebase. Core phases 1–5 have prototype implementations with simplifications; see [Implementation Notes](#implementation-notes-january-2026) for detail.
>
> The mathematical model and parameter tradeoffs are also documented in:
> [`docs/maths/proposals/20260220-velocity-coherent-foreground-extraction.md`](../maths/proposals/20260220-velocity-coherent-foreground-extraction.md)

---

## Executive Summary

This document proposes an alternative algorithm for isolating foreground points from LIDAR data that addresses the limitations of the current background-subtraction approach. The key innovation is **velocity-coherent point association**, which tracks clusters of points moving at consistent velocities through 3D space, even when reduced to as few as ~3 points.

**Key Features:**

- **Velocity-based clustering**: Associate points by kinematic coherence, not just spatial proximity
- **Long-tail tracking**: Capture the complete trajectory including pre-entry and post-exit phases
- **Sparse continuation**: Maintain track identity with minimal point counts (~3 points)
- **Track merging**: Connect fragmented observations into unified object tracks

**Problem Statement:** The current background-subtraction approach fails to yield valuable foreground points that correspond to visible objects in frames. Human observers can identify objects with as few as 3 points based on motion continuity. In the LIDAR data a human eye can identify a point's continued motion, position and speed when it is down to just a handful of points (~3). This algorithm includes the long tail ability to track these points before they come into frame (pre-tail) and after they have transited (post-tail), plus a mechanism to merge fragmented tracks.

**Delivery Structure:** 8 phases (Phase 0: instrumentation, Phases 1–5: algorithm core, Phase 6: integration, Phase 7: validation and rollout). Phases 1–5 have simplified prototype implementations; Phases 0, 6, and 7 are not yet started.

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Scope](#scope)
3. [Baseline: Current System](#baseline-current-system)
4. [Current Limitations](#current-limitations)
5. [Proposed Solution: Velocity-Coherent Extraction](#proposed-solution-velocity-coherent-extraction)
6. [Algorithm Architecture](#algorithm-architecture)
7. [Phase 0: Instrumentation and Fixtures](#phase-0-instrumentation-and-fixtures)
8. [Phase 1: Point-Level Velocity Estimation](#phase-1-point-level-velocity-estimation)
9. [Phase 2: Velocity-Coherent Clustering](#phase-2-velocity-coherent-clustering)
10. [Phase 3: Long-Tail Track Management](#phase-3-long-tail-track-management)
11. [Phase 4: Sparse Continuation Logic](#phase-4-sparse-continuation-logic)
12. [Phase 5: Track Fragment Merging](#phase-5-track-fragment-merging)
13. [Phase 6: Pipeline, Storage, and API Integration](#phase-6-pipeline-storage-and-api-integration)
14. [Phase 7: Validation and Rollout](#phase-7-validation-and-rollout)
15. [Data Structures](#data-structures)
16. [Integration with Existing System](#integration-with-existing-system)
17. [Database Schema Extensions](#database-schema-extensions)
18. [Implementation Roadmap](#implementation-roadmap)
19. [Acceptance Metrics](#acceptance-metrics)
20. [Risks and Mitigations](#risks-and-mitigations)
21. [Dependencies](#dependencies)
22. [Milestones](#milestones)
23. [Appendix: Mathematical Formulation](#appendix-mathematical-formulation)
24. [Implementation Notes](#implementation-notes-january-2026)
25. [Related Documentation](#related-documentation)

---

## Scope

### In Scope

- Per-point velocity estimation across frames
- Position+velocity clustering (6D metric behaviour)
- Long-tail lifecycle states (pre-tail, post-tail)
- Sparse continuation down to 3 points with stricter velocity checks
- Fragment merge heuristics for split tracks
- Dual-source storage/API for side-by-side evaluation

### Out of Scope (for this plan)

- Replacing the existing background-subtraction pipeline
- New ML model training
- New sensor calibration procedures

---

## Baseline: Current System

- **Active foreground extractor:** `ProcessFramePolarWithMask` in `internal/lidar/l3grid/foreground.go`
- **Active clustering:** DBSCAN in `internal/lidar/l4perception/cluster.go`
- **No `VelocityCoherentTracker`** implementation currently present in the codebase
- **Workstream separation reference:** [`docs/lidar/architecture/20260221-vector-vs-velocity-workstreams.md`](../lidar/architecture/20260221-vector-vs-velocity-workstreams.md)

---

## Current Limitations

### Background Subtraction Issues

The existing foreground extraction (`ProcessFramePolarWithMask`) classifies points as foreground/background based on deviation from learned background ranges:

```go
// Current approach: Per-point polar-space classification
cellDiff := math.Abs(float64(cell.AverageRangeMeters) - p.Distance)
closenessThreshold := closenessMultiplier * (spread + noiseRel*distance + safety)
isBackground := cellDiff <= closenessThreshold
```

**Identified Problems:**

1. **Static threshold sensitivity**: Background parameters (closeness multiplier, noise fraction) are tuned globally, causing:
   - Distant objects to be absorbed into background (noise grows with distance)
   - Close objects to saturate foreground (overwhelming true objects)

2. **No velocity context**: Points are classified independently without considering:
   - Motion coherence with nearby points
   - Consistency with established track velocities
   - Temporal continuity of observations

3. **Aggressive warmup suppression**: PCAP replay shows ~1.2% foreground ratio vs. expected 15-40%

4. **Lost track continuity**: Objects entering/exiting sensor field are:
   - Delayed in detection (warmup frames with no points)
   - Prematurely terminated (post-exit points absorbed as background)
   - Fragmented into multiple tracks (occlusion gaps > MaxMisses)

5. **Minimum point threshold too high**: DBSCAN `MinPts=12` discards valid objects with sparse returns

### Human Vision Baseline

Human observers can identify moving objects in LIDAR point clouds with as few as **3 points** by leveraging:

- **Motion continuity**: Points move together at consistent velocity
- **Spatial coherence**: Points form a connected cluster in 3D
- **Temporal persistence**: Pattern repeats across frames

The proposed algorithm aims to match this capability.

---

## Proposed Solution: Velocity-Coherent Extraction

### Core Concept

Instead of classifying points as foreground/background in isolation, we:

1. **Track point velocities**: Estimate per-point velocity vectors from frame-to-frame correspondences
2. **Cluster by velocity**: Group points with similar velocity vectors (moving together)
3. **Associate with tracks**: Match velocity clusters to existing tracked objects
4. **Extend track boundaries**: Include points before entry and after exit

### Key Innovations

| Feature              | Current System              | Proposed System               |
| -------------------- | --------------------------- | ----------------------------- |
| Point classification | Per-point, polar, static    | Velocity-coherent, temporal   |
| Minimum cluster size | 12 points (DBSCAN MinPts)   | 3 points (velocity-confirmed) |
| Track lifecycle      | Hits/misses counter         | Velocity prediction window    |
| Pre-entry handling   | Missed (warmup suppression) | Predicted from velocity       |
| Post-exit handling   | Deleted after MaxMisses     | Continued via prediction      |
| Fragmentation        | Multiple tracks             | Merged via kinematic matching |

---

## Algorithm Architecture

### Data Flow Pipeline

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    VELOCITY-COHERENT FOREGROUND EXTRACTION                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Frame N-1     Frame N      Frame N+1                                       │
│     │            │             │                                            │
│     ▼            ▼             ▼                                            │
│  ┌──────┐    ┌──────┐     ┌──────┐                                          │
│  │Points│    │Points│     │Points│                                          │
│  └───┬──┘    └───┬──┘     └───┬──┘                                          │
│      │           │            │                                             │
│      └─────────┐ │ ┌──────────┘                                             │
│                │ │ │                                                        │
│                ▼ ▼ ▼                                                        │
│  ┌────────────────────────────────────────┐                                 │
│  │   PHASE 1: Point-Level Velocity        │                                 │
│  │                                        │                                 │
│  │   • Frame-to-frame correspondence      │                                 │
│  │   • Per-point velocity estimation      │                                 │
│  │   • Velocity confidence scoring        │                                 │
│  └─────────────────┬──────────────────────┘                                 │
│                    │                                                        │
│                    ▼                                                        │
│  ┌────────────────────────────────────────┐                                 │
│  │   PHASE 2: Velocity-Coherent Clusters  │                                 │
│  │                                        │                                 │
│  │   • Group by (position + velocity)     │                                 │
│  │   • 6D distance metric                 │                                 │
│  │   • Minimum 3 points per cluster       │                                 │
│  └─────────────────┬──────────────────────┘                                 │
│                    │                                                        │
│                    ▼                                                        │
│  ┌────────────────────────────────────────┐                                 │
│  │   PHASE 3: Long-Tail Track Management  │                                 │
│  │                                        │                                 │
│  │   • Pre-tail: Velocity-predicted entry │                                 │
│  │   • Post-tail: Prediction continuation │                                 │
│  │   • Extended track boundaries          │                                 │
│  └─────────────────┬──────────────────────┘                                 │
│                    │                                                        │
│                    ▼                                                        │
│  ┌────────────────────────────────────────┐                                 │
│  │   PHASE 4: Sparse Continuation         │                                 │
│  │                                        │                                 │
│  │   • 3-point minimum threshold          │                                 │
│  │   • Velocity-confirmed association     │                                 │
│  │   • Graceful degradation               │                                 │
│  └─────────────────┬──────────────────────┘                                 │
│                    │                                                        │
│                    ▼                                                        │
│  ┌────────────────────────────────────────┐                                 │
│  │   PHASE 5: Track Fragment Merging      │                                 │
│  │                                        │                                 │
│  │   • Kinematic trajectory matching      │                                 │
│  │   • Temporal gap bridging              │                                 │
│  │   • Unified track assembly             │                                 │
│  └─────────────────┬──────────────────────┘                                 │
│                    │                                                        │
│                    ▼                                                        │
│  ┌────────────────────────────────────────┐                                 │
│  │   Output: Complete Object Tracks       │                                 │
│  │                                        │                                 │
│  │   • Full pre-to-post trajectory        │                                 │
│  │   • Sparse-point tolerant              │                                 │
│  │   • Fragment-merged                    │                                 │
│  └────────────────────────────────────────┘                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Processing Modes

The algorithm supports two processing modes:

1. **Real-time mode**: Process frames as they arrive with sliding window history
2. **Batch mode**: Process complete PCAP with full backward/forward context

---

## Phase 0: Instrumentation and Fixtures

**Goal:** Establish repeatable evaluation before algorithm changes so that improvements can be measured objectively.

### Checklist

- [ ] Define evaluation datasets (PCAP segments: dense traffic, sparse/distant traffic, occlusion-heavy)
- [ ] Add baseline metrics capture for current pipeline
- [ ] Add benchmark harness for frame throughput and memory
- [ ] Lock acceptance report format for side-by-side comparisons

### Exit Criteria

- [ ] Reproducible baseline report generated from one command
- [ ] Baseline includes precision/recall proxy, track duration, fragmentation rate, and throughput

---

## Phase 1: Point-Level Velocity Estimation

**Goal:** Compute velocity vectors and confidence for points with stable frame-to-frame correspondence.

### Objective by finding correspondences between consecutive frames.

### Algorithm: Nearest-Neighbor with Velocity Constraint

```go
// PointVelocity represents a point with estimated velocity
type PointVelocity struct {
    // Position (world frame)
    X, Y, Z float64

    // Estimated velocity (m/s)
    VX, VY, VZ float64

    // Confidence [0, 1]
    VelocityConfidence float32

    // Correspondence metadata
    CorrespondingPointIdx int   // Index in previous frame (-1 if none)
    TimestampNanos        int64
}

// EstimatePointVelocities computes per-point velocities from frame correspondence
func EstimatePointVelocities(
    currentFrame []WorldPoint,
    previousFrame []WorldPoint,
    prevVelocities []PointVelocity,
    dtSeconds float64,
    config VelocityEstimationConfig,
) []PointVelocity {

    result := make([]PointVelocity, len(currentFrame))

    // Build 3D spatial index for previous frame (position-only for correspondence search)
    prevIndex := NewSpatialIndex(config.SearchRadius)
    prevIndex.Build(previousFrame)

    for i, curr := range currentFrame {
        result[i] = PointVelocity{
            X: curr.X, Y: curr.Y, Z: curr.Z,
            TimestampNanos: curr.Timestamp.UnixNano(),
        }

        // Search for correspondence in previous frame
        // Use predicted position if previous velocity known
        searchX, searchY := curr.X, curr.Y
        if len(prevVelocities) > 0 {
            // Back-project using median velocity from nearby points
            medianVX, medianVY := estimateLocalVelocity(curr, prevVelocities, config)
            searchX -= medianVX * dtSeconds
            searchY -= medianVY * dtSeconds
        }

        // Find nearest neighbor in previous frame (3D position search)
        candidates := prevIndex.RegionQuery(previousFrame, i, config.SearchRadius)

        if len(candidates) == 0 {
            result[i].VelocityConfidence = 0
            result[i].CorrespondingPointIdx = -1
            continue
        }

        // Select best correspondence by combined position + velocity consistency
        bestIdx, bestScore := selectBestCorrespondence(
            curr, previousFrame, prevVelocities, candidates, dtSeconds, config,
        )

        if bestIdx >= 0 {
            prev := previousFrame[bestIdx]
            result[i].VX = (curr.X - prev.X) / dtSeconds
            result[i].VY = (curr.Y - prev.Y) / dtSeconds
            result[i].VZ = (curr.Z - prev.Z) / dtSeconds
            result[i].VelocityConfidence = bestScore
            result[i].CorrespondingPointIdx = bestIdx
        }
    }

    return result
}
```

### Velocity Confidence Scoring

Velocity confidence is computed based on:

1. **Spatial distance**: Closer correspondences are more confident
2. **Velocity consistency**: Similar velocity to neighbors increases confidence
3. **Magnitude plausibility**: Reject physically impossible velocities (>50 m/s for vehicles)

```go
func computeVelocityConfidence(
    spatialDist float64,
    velocityMagnitude float64,
    neighborVelocityVariance float64,
    config VelocityEstimationConfig,
) float32 {

    // Spatial proximity score [0, 1]
    spatialScore := math.Exp(-spatialDist / config.SearchRadius)

    // Velocity plausibility score [0, 1]
    if velocityMagnitude > config.MaxVelocityMps {
        return 0 // Reject implausible velocities
    }
    plausibilityScore := 1.0 - (velocityMagnitude / config.MaxVelocityMps)

    // Consistency with neighbors [0, 1]
    consistencyScore := math.Exp(-neighborVelocityVariance / config.VelocityVarianceThreshold)

    // Combined confidence
    confidence := float32(spatialScore * plausibilityScore * consistencyScore)

    return confidence
}
```

### Configuration Parameters

```go
type VelocityEstimationConfig struct {
    // Search parameters
    SearchRadius            float64 // meters, default 2.0
    MaxVelocityMps          float64 // m/s, default 50.0 (vehicle max)
    VelocityVarianceThreshold float64 // m/s, default 2.0

    // Minimum confidence to accept velocity
    MinConfidence           float32 // default 0.3

    // Neighbor context for local velocity estimation
    NeighborRadius          float64 // meters, default 1.0
    MinNeighborsForContext  int     // default 3
}
```

### Checklist

- [ ] Create `internal/lidar/velocity_estimation.go`
- [ ] Implement correspondence search with configurable radius and plausibility gates
- [ ] Implement velocity confidence scoring
- [ ] Add `internal/lidar/velocity_estimation_test.go` with synthetic and replayed edge cases
- [ ] Add config wiring for velocity estimation parameters

### Exit Criteria

- [ ] Velocity output generated for >95% of matchable points on validation segments
- [ ] Implausible velocity rates bounded by configured threshold
- [ ] Unit tests cover no-match, ambiguous-match, and high-noise cases

---

## Phase 2: Velocity-Coherent Clustering

**Goal:** Cluster points using position+velocity coherence and support `MinPts=3` mode.

### Objective

Group points that are both **spatially close** and **moving together** (similar velocity vectors).

### Algorithm: 6D DBSCAN

Standard DBSCAN operates in 3D (x, y, z). We extend to 6D: (x, y, z, vx, vy, vz).

The key insight is that **two points belong to the same object if they are close in position AND have similar velocities**.

```go
// VelocityCoherentCluster represents a group of points moving together
type VelocityCoherentCluster struct {
    ClusterID int64

    // Centroid and velocity (world frame)
    CentroidX, CentroidY, CentroidZ float64
    VelocityX, VelocityY, VelocityZ float64

    // Cluster statistics
    PointCount          int
    VelocityConfidence  float32 // Average confidence across points

    // Point indices
    PointIndices []int

    // Bounding box and features (same as WorldCluster)
    BoundingBoxLength float32
    BoundingBoxWidth  float32
    BoundingBoxHeight float32
    HeightP95         float32
    IntensityMean     float32

    // Timestamp
    TSUnixNanos int64
}

// DBSCAN6D clusters points in position-velocity space
func DBSCAN6D(
    points []PointVelocity,
    config Clustering6DConfig,
) []VelocityCoherentCluster {

    n := len(points)
    labels := make([]int, n) // 0=unvisited, -1=noise, >0=clusterID
    clusterID := 0

    // Build 6D spatial index
    spatialIndex := NewSpatialIndex6D(config.PositionEps, config.VelocityEps)
    spatialIndex.Build(points)

    for i := 0; i < n; i++ {
        if labels[i] != 0 {
            continue
        }

        // Region query in 6D space
        neighbors := spatialIndex.RegionQuery6D(
            points[i].X, points[i].Y, points[i].Z,
            points[i].VX, points[i].VY, points[i].VZ,
            config.PositionEps,
            config.VelocityEps,
        )

        // LOWER minimum points threshold (3 instead of 12)
        if len(neighbors) < config.MinPts {
            labels[i] = -1 // Noise
            continue
        }

        clusterID++
        expandCluster6D(points, spatialIndex, labels, i, neighbors, clusterID, config)
    }

    return buildVelocityClusters(points, labels, clusterID)
}
```

### 6D Distance Metric

```go
// Distance6D computes weighted distance in position-velocity space
func Distance6D(
    p1, p2 PointVelocity,
    positionWeight float64, // typically 1.0
    velocityWeight float64, // typically 2.0 (velocity more important)
) float64 {

    // Position distance (Euclidean 3D)
    dx := p1.X - p2.X
    dy := p1.Y - p2.Y
    dz := p1.Z - p2.Z
    positionDist := math.Sqrt(dx*dx + dy*dy + dz*dz)

    // Velocity distance (Euclidean 3D)
    dvx := p1.VX - p2.VX
    dvy := p1.VY - p2.VY
    dvz := p1.VZ - p2.VZ
    velocityDist := math.Sqrt(dvx*dvx + dvy*dvy + dvz*dvz)

    // Weighted combination
    return positionWeight*positionDist + velocityWeight*velocityDist
}
```

### Minimum Cluster Size Reduction

**Critical change**: Reduce `MinPts` from 12 to **3** for velocity-coherent clustering.

Justification:

- Velocity coherence provides strong confirmation (points moving together)
- Human eye can identify objects with 3 consistent points
- Distant/sparse objects produce fewer returns but still have coherent motion

```go
type Clustering6DConfig struct {
    // Spatial clustering parameters
    PositionEps float64 // meters, default 0.6
    VelocityEps float64 // m/s, default 1.0

    // REDUCED minimum points (from 12 to 3)
    MinPts int // default 3

    // Weights for distance metric
    PositionWeight float64 // default 1.0
    VelocityWeight float64 // default 2.0

    // Velocity confidence filter
    MinVelocityConfidence float32 // default 0.3
}
```

### Checklist

- [ ] Create `internal/lidar/clustering_6d.go`
- [ ] Implement 6D neighbourhood metric (position + velocity weighting)
- [ ] Implement minimum-point behaviour with sparse guardrails
- [ ] Add `internal/lidar/clustering_6d_test.go`
- [ ] Validate cluster stability versus existing DBSCAN on replay data

### Exit Criteria

- [ ] 3-point sparse clusters are accepted only with velocity coherence
- [ ] False-positive growth stays within agreed threshold versus baseline
- [ ] Runtime impact is measured and documented

---

## Phase 3: Long-Tail Track Management

**Goal:** Extend track continuity at object entry and exit boundaries.

### Objective

Extend track lifetimes to capture:

- **Pre-tail**: Objects entering the sensor field (before full clustering)
- **Post-tail**: Objects exiting the sensor field (after point density drops)

### Pre-Tail Detection: Velocity-Predicted Entry

When a new cluster appears, we check if it matches the predicted position of an object that should be entering the field of view based on its extrapolated trajectory from previous sparse observations.

```go
// PredictedEntryZone represents an area where objects are expected to appear
type PredictedEntryZone struct {
    // Expected position (extrapolated from velocity)
    PredictedX, PredictedY float64

    // Velocity vector
    VelocityX, VelocityY float64

    // Uncertainty radius (grows with time since last observation)
    UncertaintyRadius float64

    // Source track (tentative or previous observation)
    SourceTrackID string

    // Time of prediction
    PredictionTimeNanos int64

    // Frames since last observation
    FramesSinceObservation int
}

// PreTailDetector watches for objects entering the sensor field
type PreTailDetector struct {
    // Predicted entry zones based on sparse pre-observations
    EntryZones []PredictedEntryZone

    // Field of view boundary (for entry point prediction)
    FieldOfViewBoundary PolygonBoundary

    // Configuration
    Config PreTailConfig
}

func (d *PreTailDetector) Update(
    newClusters []VelocityCoherentCluster,
    timestamp time.Time,
) []TrackAssociation {

    associations := []TrackAssociation{}

    for _, zone := range d.EntryZones {
        // Find clusters near predicted entry point
        for _, cluster := range newClusters {
            distance := math.Sqrt(
                math.Pow(cluster.CentroidX-zone.PredictedX, 2) +
                math.Pow(cluster.CentroidY-zone.PredictedY, 2),
            )

            if distance < zone.UncertaintyRadius {
                // Check velocity consistency
                velMatch := d.velocityMatches(cluster, zone)
                if velMatch > d.Config.MinVelocityMatchScore {
                    associations = append(associations, TrackAssociation{
                        ClusterID:  cluster.ClusterID,
                        TrackID:    zone.SourceTrackID,
                        Type:       AssociationPreTail,
                        Confidence: velMatch,
                    })
                }
            }
        }
    }

    return associations
}
```

### Post-Tail Continuation: Prediction Window

Instead of deleting tracks after `MaxMisses` frames, we continue predicting their position and attempt to recover them when points reappear.

```go
// PostTailConfig controls post-exit track continuation
type PostTailConfig struct {
    // Maximum frames to continue prediction after last observation
    MaxPredictionFrames int // default 30 (3 seconds at 10 Hz)

    // Maximum uncertainty growth before abandoning track
    MaxUncertaintyRadius float64 // meters, default 10.0

    // Minimum confidence to recover a predicted track
    MinRecoveryConfidence float32 // default 0.5
}

// ContinuePostTail extends track lifetime via prediction
func (t *VelocityCoherentTracker) ContinuePostTail(
    track *TrackedObject,
    currentTime time.Time,
) *PredictedPosition {

    if track.State != TrackDeleted {
        return nil // Only for deleted/missing tracks
    }

    framesSinceLast := int((currentTime.UnixNano() - track.LastUnixNanos) / 100_000_000)
    if framesSinceLast > t.PostTailConfig.MaxPredictionFrames {
        return nil // Too long since last observation
    }

    // Predict current position using velocity
    dt := float64(currentTime.UnixNano()-track.LastUnixNanos) / 1e9
    predictedX := track.X + track.VX*float32(dt)
    predictedY := track.Y + track.VY*float32(dt)

    // Grow uncertainty with time
    uncertaintyRadius := t.Config.BaseUncertainty +
        float64(framesSinceLast)*t.Config.UncertaintyGrowthPerFrame

    if uncertaintyRadius > t.PostTailConfig.MaxUncertaintyRadius {
        return nil
    }

    return &PredictedPosition{
        TrackID:           track.TrackID,
        PredictedX:        predictedX,
        PredictedY:        predictedY,
        VelocityX:         track.VX,
        VelocityY:         track.VY,
        UncertaintyRadius: float32(uncertaintyRadius),
        FramesSinceLast:   framesSinceLast,
    }
}
```

### Extended Track State Machine

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                                                         │
                    │  ┌─────────────┐                                        │
    New sparse      │  │             │                                        │
    observation ───►│  │  PRE_TAIL   │◄───────────────────────────────────┐   │
                    │  │  (≥3 pts)   │                                    │   │
                    │  │             │                                    │   │
                    │  └──────┬──────┘                                    │   │
                    │         │                                           │   │
                    │         │ Velocity confirmed                        │   │
                    │         │ over 3+ frames                            │   │
                    │         ▼                                           │   │
                    │  ┌─────────────┐                                    │   │
                    │  │             │                                    │   │
                    │  │ TENTATIVE   │                                    │   │
                    │  │             │                                    │   │
                    │  └──────┬──────┘                                    │   │
                    │         │                                           │   │
                    │         │ HitsToConfirm                             │   │
                    │         │ (3 consecutive)                           │   │
                    │         ▼                                           │   │
                    │  ┌─────────────┐                                    │   │
                    │  │             │                                    │   │
                    │  │ CONFIRMED   │◄───────────────────────────────┐   │   │
                    │  │             │                                │   │   │
                    │  └──────┬──────┘                                │   │   │
                    │         │                                       │   │   │
                    │         │ MaxMisses                             │   │   │
                    │         │ (but velocity known)                  │   │   │
                    │         ▼                                       │   │   │
                    │  ┌─────────────┐                                │   │   │
                    │  │             │  Recovery with     ────────────┘   │   │
                    │  │  POST_TAIL  │  velocity match                    │   │
                    │  │ (predicted) │                                    │   │
                    │  │             │  Pre-entry match   ────────────────┘   │
                    │  └──────┬──────┘                                        │
                    │         │                                               │
                    │         │ MaxPredictionFrames                           │
                    │         │ or MaxUncertainty                             │
                    │         ▼                                               │
                    │  ┌─────────────┐                                        │
                    │  │             │                                        │
                    │  │  DELETED    │                                        │
                    │  │             │                                        │
                    │  └─────────────┘                                        │
                    │                                                         │
                    └─────────────────────────────────────────────────────────┘
```

### Checklist

- [ ] Create `internal/lidar/long_tail.go`
- [ ] Add pre-tail predicted entry association logic
- [ ] Add post-tail prediction window and uncertainty growth logic
- [ ] Extend track states and transitions
- [ ] Add `internal/lidar/long_tail_test.go`

### Exit Criteria

- [ ] Mean track duration increases on boundary-entry/exit scenarios
- [ ] Recovery after brief occlusions improves without large precision drop
- [ ] State machine transitions are fully test-covered

---

## Phase 4: Sparse Continuation Logic

**Goal:** Preserve track identity through low-point-count observations.

### Objective

Maintain track identity even when point count drops to ~3 points, using velocity coherence as the primary confirmation signal.

### Sparse Track Criteria

```go
type SparseTrackConfig struct {
    // Absolute minimum points to maintain a track
    MinPointsAbsolute int // default 3

    // Minimum velocity confidence for sparse tracks
    MinVelocityConfidenceForSparse float32 // default 0.6

    // Maximum velocity variance for sparse tracks
    MaxVelocityVarianceForSparse float64 // m/s, default 0.5

    // Spatial coherence threshold
    MaxSpatialSpreadForSparse float64 // meters, default 2.0
}

// IsSparseTrackValid checks if a sparse cluster can maintain track identity
func IsSparseTrackValid(
    cluster VelocityCoherentCluster,
    existingTrack *TrackedObject,
    config SparseTrackConfig,
) (bool, float32) {

    // Minimum point count
    if cluster.PointCount < config.MinPointsAbsolute {
        return false, 0
    }

    // Velocity confidence threshold
    if cluster.VelocityConfidence < config.MinVelocityConfidenceForSparse {
        return false, 0
    }

    // Velocity must match existing track
    velDiff := math.Sqrt(
        math.Pow(float64(cluster.VelocityX)-float64(existingTrack.VX), 2) +
        math.Pow(float64(cluster.VelocityY)-float64(existingTrack.VY), 2),
    )

    if velDiff > config.MaxVelocityVarianceForSparse {
        return false, 0
    }

    // Spatial spread must be reasonable
    maxDim := cluster.BoundingBoxLength
    if cluster.BoundingBoxWidth > maxDim {
        maxDim = cluster.BoundingBoxWidth
    }
    if float64(maxDim) > config.MaxSpatialSpreadForSparse {
        return false, 0
    }

    // Compute confidence score
    velocityMatchScore := 1.0 - velDiff/config.MaxVelocityVarianceForSparse
    pointScore := float64(cluster.PointCount) / 10.0 // Scale 3-10 points to 0.3-1.0
    if pointScore > 1.0 {
        pointScore = 1.0
    }

    confidence := float32(velocityMatchScore * pointScore * float64(cluster.VelocityConfidence))

    return true, confidence
}
```

### Graceful Degradation Strategy

As point count decreases, we progressively tighten velocity constraints:

| Point Count | Velocity Tolerance | Spatial Tolerance | Notes                          |
| ----------- | ------------------ | ----------------- | ------------------------------ |
| ≥12         | ±2.0 m/s           | ±1.0 m            | Standard DBSCAN clustering     |
| 6-11        | ±1.5 m/s           | ±0.8 m            | Reduced tolerance              |
| 3-5         | ±0.5 m/s           | ±0.5 m            | Strict velocity match required |
| <3          | N/A                | N/A               | Rely on prediction only        |

```go
func (t *VelocityCoherentTracker) adaptiveTolerances(pointCount int) (velTol, spatialTol float64) {
    switch {
    case pointCount >= 12:
        return 2.0, 1.0
    case pointCount >= 6:
        return 1.5, 0.8
    case pointCount >= 3:
        return 0.5, 0.5
    default:
        return 0, 0 // Cannot track with <3 points
    }
}
```

### Checklist

- [ ] Create `internal/lidar/sparse_continuation.go`
- [ ] Implement adaptive tolerances by point count
- [ ] Enforce confidence and variance gates for 3–5 point frames
- [ ] Integrate sparse continuation decisions into tracker updates
- [ ] Add targeted tests for 3-point continuation and failure boundaries

### Exit Criteria

- [ ] Sparse tracks are maintained when motion is coherent
- [ ] No significant increase in ID switches in sparse scenes
- [ ] Parameter sensitivity documented for tuning

---

## Phase 5: Track Fragment Merging

**Goal:** Merge split track fragments when kinematics are consistent.

### Objective

Reconnect track fragments that were split due to:

- Occlusion gaps exceeding MaxMisses
- Sensor noise causing temporary point loss
- Objects passing through blind spots

### Fragment Detection

```go
// TrackFragment represents a potentially incomplete track segment
type TrackFragment struct {
    Track     *TrackedObject

    // Entry/exit characteristics
    EntryPoint    [2]float32 // Position where track first appeared
    ExitPoint     [2]float32 // Position where track last appeared
    EntryVelocity [2]float32 // Velocity at entry
    ExitVelocity  [2]float32 // Velocity at exit

    // Temporal bounds
    StartNanos int64
    EndNanos   int64

    // Flags
    HasNaturalEntry bool // Started from sensor boundary (vs. appeared mid-field)
    HasNaturalExit  bool // Ended at sensor boundary (vs. disappeared mid-field)
}

// DetectFragments identifies tracks that may be fragments of longer trajectories
func DetectFragments(tracks []*TrackedObject, sensorBoundary PolygonBoundary) []TrackFragment {
    fragments := []TrackFragment{}

    for _, track := range tracks {
        if len(track.History) < 2 {
            continue
        }

        entry := track.History[0]
        exit := track.History[len(track.History)-1]

        // Compute velocities
        if len(track.History) >= 2 {
            dt := float64(track.History[1].Timestamp-track.History[0].Timestamp) / 1e9
            entryVX := (track.History[1].X - track.History[0].X) / float32(dt)
            entryVY := (track.History[1].Y - track.History[0].Y) / float32(dt)

            dtExit := float64(exit.Timestamp-track.History[len(track.History)-2].Timestamp) / 1e9
            exitVX := (exit.X - track.History[len(track.History)-2].X) / float32(dtExit)
            exitVY := (exit.Y - track.History[len(track.History)-2].Y) / float32(dtExit)

            fragment := TrackFragment{
                Track:         track,
                EntryPoint:    [2]float32{entry.X, entry.Y},
                ExitPoint:     [2]float32{exit.X, exit.Y},
                EntryVelocity: [2]float32{entryVX, entryVY},
                ExitVelocity:  [2]float32{exitVX, exitVY},
                StartNanos:    track.FirstUnixNanos,
                EndNanos:      track.LastUnixNanos,
            }

            // Check if entry/exit are at sensor boundary
            fragment.HasNaturalEntry = sensorBoundary.IsNearBoundary(entry.X, entry.Y, 2.0)
            fragment.HasNaturalExit = sensorBoundary.IsNearBoundary(exit.X, exit.Y, 2.0)

            fragments = append(fragments, fragment)
        }
    }

    return fragments
}
```

### Fragment Matching Algorithm

```go
// MergeConfig controls fragment matching sensitivity
type MergeConfig struct {
    // Maximum time gap between fragments to consider merging
    MaxTimeGapSeconds float64 // default 5.0

    // Maximum position error (predicted vs actual entry point)
    MaxPositionErrorMeters float64 // default 3.0

    // Maximum velocity difference at junction
    MaxVelocityDifferenceMs float64 // default 2.0

    // Minimum trajectory alignment score
    MinAlignmentScore float32 // default 0.7
}

// MergeCandidatePair represents two fragments that might be the same object
type MergeCandidatePair struct {
    Earlier     *TrackFragment
    Later       *TrackFragment

    // Matching scores
    PositionScore   float32 // How well predicted position matches
    VelocityScore   float32 // How well velocities align
    TrajectoryScore float32 // How well overall trajectory matches
    OverallScore    float32
}

// FindMergeCandidates identifies fragment pairs that may belong together
func FindMergeCandidates(
    fragments []TrackFragment,
    config MergeConfig,
) []MergeCandidatePair {

    candidates := []MergeCandidatePair{}

    // Sort by start time
    sort.Slice(fragments, func(i, j int) bool {
        return fragments[i].StartNanos < fragments[j].StartNanos
    })

    for i, earlier := range fragments {
        // Skip if natural exit (went to boundary)
        if earlier.HasNaturalExit {
            continue
        }

        for j := i + 1; j < len(fragments); j++ {
            later := fragments[j]

            // Skip if natural entry (came from boundary)
            if later.HasNaturalEntry {
                continue
            }

            // Check time gap
            gapSeconds := float64(later.StartNanos-earlier.EndNanos) / 1e9
            if gapSeconds < 0 || gapSeconds > config.MaxTimeGapSeconds {
                continue
            }

            // Predict where earlier track would be at later.StartNanos
            predictedX := earlier.ExitPoint[0] + earlier.ExitVelocity[0]*float32(gapSeconds)
            predictedY := earlier.ExitPoint[1] + earlier.ExitVelocity[1]*float32(gapSeconds)

            // Position error
            posError := math.Sqrt(
                math.Pow(float64(predictedX-later.EntryPoint[0]), 2) +
                math.Pow(float64(predictedY-later.EntryPoint[1]), 2),
            )

            if posError > config.MaxPositionErrorMeters {
                continue
            }

            // Velocity difference
            velDiff := math.Sqrt(
                math.Pow(float64(earlier.ExitVelocity[0]-later.EntryVelocity[0]), 2) +
                math.Pow(float64(earlier.ExitVelocity[1]-later.EntryVelocity[1]), 2),
            )

            if velDiff > config.MaxVelocityDifferenceMs {
                continue
            }

            // Compute scores
            posScore := float32(1.0 - posError/config.MaxPositionErrorMeters)
            velScore := float32(1.0 - velDiff/config.MaxVelocityDifferenceMs)
            trajectoryScore := computeTrajectoryAlignment(earlier, later)

            overallScore := (posScore + velScore + trajectoryScore) / 3.0

            if overallScore >= config.MinAlignmentScore {
                candidates = append(candidates, MergeCandidatePair{
                    Earlier:         &fragments[i],
                    Later:           &fragments[j],
                    PositionScore:   posScore,
                    VelocityScore:   velScore,
                    TrajectoryScore: trajectoryScore,
                    OverallScore:    overallScore,
                })
            }
        }
    }

    return candidates
}
```

### Merge Execution

```go
// MergeTrackFragments combines two fragments into a unified track
func MergeTrackFragments(
    earlier *TrackedObject,
    later *TrackedObject,
    gapSeconds float64,
) *TrackedObject {

    merged := &TrackedObject{
        TrackID:        earlier.TrackID, // Keep earlier ID
        SensorID:       earlier.SensorID,
        State:          later.State,

        // Lifecycle spans both fragments
        FirstUnixNanos: earlier.FirstUnixNanos,
        LastUnixNanos:  later.LastUnixNanos,

        // Kalman state from later track (most recent)
        X:  later.X,
        Y:  later.Y,
        VX: later.VX,
        VY: later.VY,
        P:  later.P,

        // Aggregate statistics
        Hits:             earlier.Hits + later.Hits,
        Misses:           later.Misses,
        ObservationCount: earlier.ObservationCount + later.ObservationCount,
    }

    // Merge histories
    merged.History = make([]TrackPoint, 0, len(earlier.History)+len(later.History))
    merged.History = append(merged.History, earlier.History...)

    // Interpolate gap if needed
    if gapSeconds > 0 && gapSeconds < 5.0 {
        merged.History = append(merged.History, interpolateGap(
            earlier.History[len(earlier.History)-1],
            later.History[0],
            gapSeconds,
        )...)
    }

    merged.History = append(merged.History, later.History...)

    // Recompute aggregated features
    merged.ComputeQualityMetrics()
    recomputeAggregatedFeatures(merged)

    return merged
}
```

### Checklist

- [ ] Create `internal/lidar/track_merge.go`
- [ ] Implement candidate generation using time/position/velocity gates
- [ ] Implement merge scoring and deterministic tie-breaking
- [ ] Add `internal/lidar/track_merge_test.go`
- [ ] Record merge decisions for audit/debug

### Exit Criteria

- [ ] Fragmentation rate decreases on occlusion-heavy validation runs
- [ ] Incorrect merge rate remains below agreed threshold
- [ ] Merge audit trail is queryable

---

## Phase 6: Pipeline, Storage, and API Integration

**Goal:** Run current and velocity-coherent paths in parallel and expose both results via API and storage.

### Checklist

- [ ] Create `internal/lidar/velocity_coherent_tracker.go`
- [ ] Create dual extraction orchestration path (parallel source processing)
- [ ] Add storage schema for velocity-coherent clusters/tracks (see [Database Schema Extensions](#database-schema-extensions))
- [ ] Add API source selector (`background_subtraction`, `velocity_coherent`, `all`)
- [ ] Add migration and rollback notes

### Exit Criteria

- [ ] Both sources can be queried independently and jointly
- [ ] Dashboard comparison can be generated from stored results
- [ ] No regression in existing source behaviour

---

## Phase 7: Validation and Rollout

**Goal:** Decide production readiness from measured outcomes.

### Checklist

- [ ] Run full replay evaluation across selected PCAP suites
- [ ] Compare against baseline using agreed metrics
- [ ] Document default parameter set and safe bounds
- [ ] Add ops runbook (alerts, fallbacks, troubleshooting)
- [ ] Stage rollout behind feature flag

### Exit Criteria

- [ ] Acceptance thresholds met on continuity and sparse-object capture
- [ ] Throughput and memory remain within service budget
- [ ] Rollback path verified in staging

---

## Data Structures

### Core Types

```go
// VelocityCoherentTrackerConfig holds all configuration for the tracker
type VelocityCoherentTrackerConfig struct {
    // Phase 1: Velocity estimation
    VelocityEstimation VelocityEstimationConfig

    // Phase 2: Clustering
    Clustering Clustering6DConfig

    // Phase 3: Long-tail management
    PreTail  PreTailConfig
    PostTail PostTailConfig

    // Phase 4: Sparse continuation
    SparseContinuation SparseTrackConfig

    // Phase 5: Fragment merging
    Merge MergeConfig

    // Standard tracking (from existing system)
    Tracking TrackerConfig
}

// DefaultVelocityCoherentConfig returns sensible defaults
func DefaultVelocityCoherentConfig() VelocityCoherentTrackerConfig {
    return VelocityCoherentTrackerConfig{
        VelocityEstimation: VelocityEstimationConfig{
            SearchRadius:              2.0,
            MaxVelocityMps:            50.0,
            VelocityVarianceThreshold: 2.0,
            MinConfidence:             0.3,
            NeighborRadius:            1.0,
            MinNeighborsForContext:    3,
        },
        Clustering: Clustering6DConfig{
            PositionEps:               0.6,
            VelocityEps:               1.0,
            MinPts:                    3, // REDUCED from 12
            PositionWeight:            1.0,
            VelocityWeight:            2.0,
            MinVelocityConfidence:     0.3,
        },
        PreTail: PreTailConfig{
            EntryPredictionWindow:     30, // frames
            MinVelocityMatchScore:     0.6,
            BoundaryMarginMeters:      2.0,
        },
        PostTail: PostTailConfig{
            MaxPredictionFrames:       30,
            MaxUncertaintyRadius:      10.0,
            MinRecoveryConfidence:     0.5,
        },
        SparseContinuation: SparseTrackConfig{
            MinPointsAbsolute:                3,
            MinVelocityConfidenceForSparse:   0.6,
            MaxVelocityVarianceForSparse:     0.5,
            MaxSpatialSpreadForSparse:        2.0,
        },
        Merge: MergeConfig{
            MaxTimeGapSeconds:         5.0,
            MaxPositionErrorMeters:    3.0,
            MaxVelocityDifferenceMs:   2.0,
            MinAlignmentScore:         0.7,
        },
        Tracking: DefaultTrackerConfig(),
    }
}
```

### Point History Ring Buffer

For efficient frame-to-frame correspondence:

```go
// FrameHistory maintains a sliding window of recent frames
type FrameHistory struct {
    Frames     []PointVelocityFrame
    Capacity   int
    WriteIndex int
}

type PointVelocityFrame struct {
    Points        []PointVelocity
    Timestamp     time.Time
    SpatialIndex  *SpatialIndex6D
}

func (h *FrameHistory) Add(frame PointVelocityFrame) {
    h.Frames[h.WriteIndex] = frame
    h.WriteIndex = (h.WriteIndex + 1) % h.Capacity
}

func (h *FrameHistory) Previous(offset int) *PointVelocityFrame {
    if offset >= h.Capacity {
        return nil
    }
    idx := (h.WriteIndex - 1 - offset + h.Capacity) % h.Capacity
    return &h.Frames[idx]
}
```

---

## Integration with Existing System

### Dual-Source Architecture (Matching Radar Pattern)

Just as the radar system maintains two independent transit sources:

- **`radar_objects`**: Hardware classifier from OPS243 sensor
- **`radar_data_transits`**: Software sessionisation algorithm

The LIDAR system will maintain two independent track sources:

- **`lidar_tracks`**: Existing background-subtraction + DBSCAN clustering (MinPts=12)
- **`lidar_velocity_coherent_tracks`**: New velocity-coherent extraction (MinPts=3)

Both track sources are:

1. **Stored independently** in separate database tables
2. **Queryable via API** with a `source` parameter to select which algorithm
3. **Comparable in dashboards** for performance evaluation
4. **Compatible with the same downstream analysis** (speed percentiles, classification, etc.)

```go
// GET /api/lidar/tracks?source=background_subtraction
// GET /api/lidar/tracks?source=velocity_coherent
// GET /api/lidar/tracks?source=all  // returns both with source label

type TrackSource string

const (
    TrackSourceBackgroundSubtraction TrackSource = "background_subtraction"
    TrackSourceVelocityCoherent      TrackSource = "velocity_coherent"
)

type TrackWithSource struct {
    Track  TrackedObject `json:"track"`
    Source TrackSource   `json:"source"`
}
```

### Parallel Processing Path

The velocity-coherent extraction runs **alongside** the existing background-subtraction system, not replacing it:

```go
// ProcessFrame runs both extraction methods in parallel
func (p *DualExtractionPipeline) ProcessFrame(
    polarPoints []PointPolar,
    pose *Pose,
    timestamp time.Time,
) (*FrameResult, error) {

    // Path 1: Existing background subtraction
    bgMask, err := p.BackgroundManager.ProcessFramePolarWithMask(polarPoints)
    if err != nil {
        return nil, err
    }
    bgForeground := ExtractForegroundPoints(polarPoints, bgMask)
    bgWorld := TransformToWorld(bgForeground, pose, p.SensorID)
    bgClusters := DBSCAN(bgWorld, p.DBSCANParams)

    // Path 2: Velocity-coherent extraction (uses ALL points, not just background-filtered)
    worldPoints := TransformToWorld(polarPoints, pose, p.SensorID)
    vcPoints := p.VelocityEstimator.EstimateVelocities(worldPoints, timestamp)
    vcClusters := DBSCAN6D(vcPoints, p.Clustering6DConfig)

    // Path 3: Merge results (take union of detected objects)
    mergedClusters := mergeClusterSets(bgClusters, vcClusters, p.MergeThreshold)

    // Update tracker with merged clusters
    p.Tracker.Update(mergedClusters, timestamp)

    return &FrameResult{
        BackgroundClusters:        bgClusters,
        VelocityCoherentClusters:  vcClusters,
        MergedClusters:            mergedClusters,
        ActiveTracks:              p.Tracker.GetActiveTracks(),
    }, nil
}
```

### REST API Extensions

```go
// Additional endpoints for velocity-coherent tracking

// GET /api/lidar/velocity-tracks/active
// Returns tracks with velocity confidence scores

// GET /api/lidar/tracks/{track_id}/velocity-profile
// Returns velocity history for a track

// GET /api/lidar/merge-candidates
// Returns detected fragment merge opportunities

// POST /api/lidar/merge-tracks
// Manually merge two track fragments
type MergeRequest struct {
    EarlierTrackID string `json:"earlier_track_id"`
    LaterTrackID   string `json:"later_track_id"`
}
```

---

## Database Schema Extensions

```sql
-- Velocity-coherent clustering results (6D DBSCAN output)
CREATE TABLE IF NOT EXISTS lidar_velocity_coherent_clusters (
    cluster_id INTEGER PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    ts_unix_nanos INTEGER NOT NULL,

    -- Position (world frame)
    centroid_x REAL,
    centroid_y REAL,
    centroid_z REAL,

    -- Velocity (world frame, m/s)
    velocity_x REAL,
    velocity_y REAL,
    velocity_z REAL,
    velocity_confidence REAL,

    -- Cluster metrics
    points_count INTEGER,
    bounding_box_length REAL,
    bounding_box_width REAL,
    bounding_box_height REAL
);

-- Velocity-coherent tracks (parallel to lidar_tracks, like radar_objects vs radar_data_transits)
-- This table stores tracks from the velocity-coherent algorithm for comparison with
-- background-subtraction tracks in lidar_tracks
CREATE TABLE IF NOT EXISTS lidar_velocity_coherent_tracks (
    track_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    track_state TEXT NOT NULL,           -- 'pre_tail', 'tentative', 'confirmed', 'post_tail', 'deleted'

    -- Lifecycle
    start_unix_nanos INTEGER NOT NULL,
    end_unix_nanos INTEGER,
    observation_count INTEGER,

    -- Kinematics (world frame)
    avg_speed_mps REAL,
    peak_speed_mps REAL,
    p50_speed_mps REAL,
    p85_speed_mps REAL,
    p95_speed_mps REAL,

    -- Velocity estimation quality
    avg_velocity_confidence REAL,
    velocity_consistency_score REAL,     -- How stable velocity was across observations

    -- Shape features
    bounding_box_length_avg REAL,
    bounding_box_width_avg REAL,
    bounding_box_height_avg REAL,
    height_p95_max REAL,

    -- Sparse tracking metrics
    min_points_observed INTEGER,         -- Minimum point count in any frame
    sparse_frame_count INTEGER,          -- Frames with <12 points

    -- Classification
    object_class TEXT,
    object_confidence REAL,
    classification_model TEXT
);

CREATE INDEX idx_vc_tracks_sensor ON lidar_velocity_coherent_tracks(sensor_id);
CREATE INDEX idx_vc_tracks_state ON lidar_velocity_coherent_tracks(track_state);
CREATE INDEX idx_vc_tracks_time ON lidar_velocity_coherent_tracks(start_unix_nanos, end_unix_nanos);

-- Track merge history
CREATE TABLE IF NOT EXISTS lidar_track_merges (
    merge_id INTEGER PRIMARY KEY,
    merged_at INTEGER NOT NULL,

    -- Source tracks
    earlier_track_id TEXT NOT NULL,
    later_track_id TEXT NOT NULL,

    -- Resulting track
    result_track_id TEXT NOT NULL,

    -- Merge scores
    position_score REAL,
    velocity_score REAL,
    trajectory_score REAL,
    overall_score REAL,

    -- Gap info
    gap_seconds REAL,
    interpolated_points INTEGER
);

CREATE INDEX idx_velocity_coherent_clusters_time ON lidar_velocity_coherent_clusters(ts_unix_nanos);
CREATE INDEX idx_track_merges_result ON lidar_track_merges(result_track_id);
```

---

## Implementation Roadmap

### Phase Timeline

| Phase | Description                     | Duration  | Priority | Dependencies |
| ----- | ------------------------------- | --------- | -------- | ------------ |
| 0     | Instrumentation and Fixtures    | 1 week    | P0       | None         |
| 1     | Point-Level Velocity Estimation | 1–2 weeks | P0       | Phase 0      |
| 2     | 6D DBSCAN Clustering            | 1 week    | P0       | Phase 1      |
| 3     | Long-Tail Track Management      | 1–2 weeks | P1       | Phase 2      |
| 4     | Sparse Continuation Logic       | 1 week    | P1       | Phase 2      |
| 5     | Track Fragment Merging          | 1–2 weeks | P2       | Phases 3, 4  |
| 6     | Pipeline, Storage, and API      | 1 week    | P1       | All phases   |
| 7     | Validation and Rollout          | 1–2 weeks | P1       | Phase 6      |

### Implementation Files

| Phase | File                                          | Description                    |
| ----- | --------------------------------------------- | ------------------------------ |
| 1     | `internal/lidar/velocity_estimation.go`       | Per-point velocity computation |
| 1     | `internal/lidar/velocity_estimation_test.go`  | Unit tests                     |
| 2     | `internal/lidar/clustering_6d.go`             | 6D DBSCAN implementation       |
| 2     | `internal/lidar/clustering_6d_test.go`        | Unit tests                     |
| 3     | `internal/lidar/long_tail.go`                 | Pre-tail and post-tail logic   |
| 3     | `internal/lidar/long_tail_test.go`            | Unit tests                     |
| 4     | `internal/lidar/sparse_continuation.go`       | Sparse track validation        |
| 5     | `internal/lidar/track_merge.go`               | Fragment detection and merging |
| 5     | `internal/lidar/track_merge_test.go`          | Unit tests                     |
| 6     | `internal/lidar/velocity_coherent_tracker.go` | Combined pipeline              |
| 6     | `internal/lidar/monitor/velocity_api.go`      | REST endpoints                 |

---

## Acceptance Metrics

These targets are hypotheses to validate against measured outcomes, not committed production guarantees.

| Metric                                              | Target                |
| --------------------------------------------------- | --------------------- |
| Sparse-object recall (3–11 points)                  | +20% to +40% relative |
| Track fragmentation rate                            | −10% to −25% relative |
| Median track duration for boundary-crossing objects | +10% to +30% relative |
| Additional false positives vs baseline              | <+10%                 |
| Throughput regression at target frame rate          | <20%                  |

---

## Risks and Mitigations

| Risk                                                     | Mitigation                                                                  |
| -------------------------------------------------------- | --------------------------------------------------------------------------- |
| Low `MinPts` increases noise clusters                    | Strict velocity-confidence and variance gates in sparse mode                |
| Over-aggressive post-tail prediction causes ghost tracks | Uncertainty growth caps and hard prediction timeout                         |
| Incorrect fragment merges corrupt track statistics       | Conservative merge threshold + auditable merge logs in `lidar_track_merges` |
| Runtime overhead from added matching/clustering          | Bounded neighbourhood queries and benchmark gates in CI                     |

---

## Dependencies

- Stable world-frame transform quality from existing pose pipeline
- Representative replay datasets with known corner cases (dense traffic, sparse/distant traffic, occlusion-heavy)
- Dashboard/query support for dual-source comparison

---

## Milestones

| Milestone | Completion Criteria                                                          |
| --------- | ---------------------------------------------------------------------------- |
| M0        | Reproducible baseline report generated from one command                      |
| M1        | Phase 1 complete — per-point velocities with confidence scores validated     |
| M2        | Phase 2 complete — stable sparse clustering with MinPts=3                    |
| M3        | Phases 3–4 complete — long-tail states and sparse continuation working       |
| M4        | Phase 5 complete — audited fragment merging with queryable merge trail       |
| M5        | Phases 6–7 complete — dual-source API, storage, validation, rollout decision |

---

## Appendix: Mathematical Formulation

### A. Point Correspondence Optimisation

Given frames $F_{n-1}$ and $F_n$, find optimal point correspondences $C: F_{n-1} \to F_n$ that minimise:

$$L(C) = \sum_{i \in F_n} \left[ w_{\text{pos}} \cdot d_{\text{pos}}(i, C(i)) + w_{\text{vel}} \cdot d_{\text{vel}}(i, C(i)) \right]$$

Where:

- $d_{\text{pos}}(i, j)$ = Euclidean distance between points $i$ and $j$
- $d_{\text{vel}}(i, j)$ = Velocity consistency with local neighbourhood
- $w_{\text{pos}}, w_{\text{vel}}$ = Weighting factors

### B. 6D Distance Metric

For points $p = (x, y, z, v_x, v_y, v_z)$ and $q = (x', y', z', v_x', v_y', v_z')$:

$$D_{6D}(p, q) = \sqrt{ \alpha(\Delta x^2 + \Delta y^2 + \Delta z^2) + \beta(\Delta v_x^2 + \Delta v_y^2 + \Delta v_z^2) }$$

Where:

- $\alpha$ = position weight (default 1.0)
- $\beta$ = velocity weight (default 2.0)
- Higher $\beta$ emphasises velocity coherence over spatial proximity

### C. Merge Trajectory Alignment Score

For fragments $A$ (ending at $t_1$) and $B$ (starting at $t_2$), compute trajectory alignment:

$$S_{\text{trajectory}} = \cos(\theta_{\text{exit}}, \theta_{\text{entry}}) \cdot \exp\!\left(\frac{-|v_{\text{exit}} - v_{\text{entry}}|}{\sigma_v}\right)$$

Where:

- $\theta_{\text{exit}}$ = heading angle at $A$'s exit
- $\theta_{\text{entry}}$ = heading angle at $B$'s entry
- $v_{\text{exit}}, v_{\text{entry}}$ = speed magnitudes
- $\sigma_v$ = velocity tolerance parameter

---

## Implementation Notes (January 2026)

### Simplifications Applied vs. Original Design

The prototype implementation applies practical simplifications for the traffic monitoring use case:

| Design Section          | Original Spec                          | Implementation                           | Rationale                      |
| ----------------------- | -------------------------------------- | ---------------------------------------- | ------------------------------ |
| **Tracking Model**      | 6D (x,y,z,vx,vy,vz)                    | 2D+velocity (x,y,vx,vy)                  | Ground-plane assumption valid  |
| **6D DBSCAN**           | Full 6D spatial index                  | Sequential 3D position + velocity filter | Simpler, reuses existing index |
| **MinPts**              | 3                                      | 3 ✅                                     | Implemented as designed        |
| **Velocity Estimation** | Frame correspondence + back-projection | Frame correspondence ✅                  | Implemented as designed        |
| **Long-Tail Tracking**  | Pre-tail + post-tail states            | Implemented ✅                           | Working as designed            |
| **Fragment Merging**    | Full kinematic matching                | Basic implementation ✅                  | Simplified merge criteria      |
| **Sparse Continuation** | Adaptive tolerances by point count     | Implemented ✅                           | Working as designed            |

### Key Implementation Files (Prototype)

| Phase       | Design Section | Implementation File                                |
| ----------- | -------------- | -------------------------------------------------- |
| Phase 1     | §8 above       | `velocity_estimation.go`                           |
| Phase 2     | §9 above       | `velocity_coherent_clustering.go`                  |
| Phase 3     | §10 above      | `velocity_coherent_tracking.go`                    |
| Phase 4     | §11 above      | `velocity_coherent_tracking.go`                    |
| Phase 5     | §12 above      | `velocity_coherent_merging.go`                     |
| Integration | §13–14 above   | `dual_pipeline.go`, `velocity_coherent_tracker.go` |

### What Was NOT Implemented (Deferred)

These features from the original design are deferred to future work:

1. **Full 6D Spatial Index** — Simpler approach of 3D clustering + velocity validation used instead
2. **Heading Estimation** — Not needed; velocity vector provides implicit heading
3. **Z-axis Tracking** — Height stored as statistic, not tracked position
4. **Track Quality Scoring** — Basic quality metrics only
5. **Batch Mode Processing** — Real-time mode only

### Performance Observations (from PCAP Replay Testing)

- **MinPts=3** successfully captures sparse distant objects missed by MinPts=12
- **Velocity coherence** significantly reduces false positives from background noise
- **Fragment merging** recovers ~10–15% of tracks split by occlusion gaps
- **Post-tail prediction** extends track duration by 1–3 seconds on average

### Recommended Next Steps

1. **Add truck/cyclist classes** — Currently only car/pedestrian/bird/other
2. **Tune velocity tolerances** — May need per-class velocity limits
3. **Evaluate sparse track quality** — 3-point tracks may have elevated position noise
4. **Complete Phase 0** — Formal baseline metrics are needed to confirm improvement claims

---

## Related Documentation

- [`docs/maths/proposals/20260220-velocity-coherent-foreground-extraction.md`](../maths/proposals/20260220-velocity-coherent-foreground-extraction.md) — Mathematical model and parameter tradeoffs
- [`docs/lidar/architecture/20260221-vector-vs-velocity-workstreams.md`](../lidar/architecture/20260221-vector-vs-velocity-workstreams.md) — Workstream separation rationale
- [`docs/plans/lidar-static-pose-alignment-plan.md`](./lidar-static-pose-alignment-plan.md) — Pose pipeline dependency
- [`docs/plans/lidar-motion-capture-architecture-plan.md`](./lidar-motion-capture-architecture-plan.md) — Motion capture architecture

---

**Document Status:** Living design document and implementation checklist
**Implementation Status:** Core phases 1–5 prototyped with simplifications; Phases 0, 6–7 not started
**Last Updated:** February 21, 2026
