# LIDAR Tracking Enhancements: Foreground Extraction & Training Data Preparation

## Executive Summary

This document outlines a comprehensive plan to enhance the LIDAR tracking system's foreground point cloud extraction, quality assessment, and training data preparation capabilities. The focus is on preparing high-quality datasets for ML training rather than immediate ML integration. The plan addresses track quality metrics, point cloud export for isolated tracks, and curation tools for building training datasets.

**Current Status:**
- ✅ Kalman filter-based tracker with lifecycle management (tentative → confirmed → deleted)
- ✅ Rule-based classification (v1.0): pedestrian, car, bird, other
- ✅ DBSCAN clustering with spatial indexing
- ✅ Database schema for tracks, observations, clusters, and analysis runs
- ✅ Phase 1: Track quality metrics (length, duration, occlusions, spatial coverage)
- ⚠️ No point cloud export for isolated tracks
- ⚠️ Limited training data curation tools

---

## Current State Analysis

### 1. Track Algorithm (Kalman Filter-Based)

**Implementation:** `internal/lidar/tracking.go`

**Architecture:**
- **State Vector:** `[x, y, vx, vy]` - position and velocity in world frame
- **Motion Model:** Constant velocity with process noise
- **Association:** Mahalanobis distance gating with nearest neighbor
- **Lifecycle:** Tentative (new) → Confirmed (3+ hits) → Deleted (3+ misses)

**Parameters (Tunable):**
```go
TrackerConfig {
    MaxTracks:               100    // Concurrent track limit
    MaxMisses:               3      // Misses before deletion
    HitsToConfirm:           3      // Hits for confirmation
    GatingDistanceSquared:   25.0   // 5m gating radius²
    ProcessNoisePos:         0.1    // Position process noise
    ProcessNoiseVel:         0.5    // Velocity process noise
    MeasurementNoise:        0.2    // Measurement noise
    DeletedTrackGracePeriod: 5s     // Cleanup delay
}
```

**Track Features (Aggregated):**
- Spatial: Bounding box (length, width, height avg), HeightP95Max
- Kinematic: Avg/Peak speed, speed history (p50/p85/p95)
- Appearance: Intensity mean average
- Temporal: Observation count, first/last timestamps

**Strengths:**
- Low computational overhead (suitable for Raspberry Pi 4)
- Explicit lifecycle management prevents track leaks
- Covariance-based gating prevents spurious associations
- Per-track speed percentiles enable vehicle characterization

**Limitations:**
- Constant velocity model struggles with acceleration/turning
- No explicit occlusion handling
- Limited multi-hypothesis tracking (single nearest neighbor)
- No track-to-track distance metrics for merge/split detection

---

### 2. Classification System (Rule-Based v1.0)

**Implementation:** `internal/lidar/classification.go`

**Current Approach:**
Rule-based thresholds using aggregated track features:

```go
// Bird: Small + slow + low altitude
Height < 0.5m, Speed < 1 m/s, Length < 1m, Width < 1m

// Vehicle: Large + fast
Length > 3m OR Width > 1.5m, Speed > 5 m/s OR Peak > 7.5 m/s

// Pedestrian: Human-sized + moderate speed
Height 1.0-2.2m, Speed < 3 m/s, Length < 3m, Width < 1.5m

// Other: Default fallback
```

**Features Used:**
- AvgHeight, AvgLength, AvgWidth, HeightP95
- AvgSpeed, PeakSpeed, P50/P85/P95 Speed
- ObservationCount, DurationSecs

**Confidence Computation:**
- Base confidence: 0.70 (MediumConfidence)
- Adjustments: +0.05 to +0.15 based on feature alignment
- Clamping: [0.0, 1.0] with minimum thresholds per class

**Strengths:**
- No training data required (bootstrap-friendly)
- Interpretable thresholds tuned for traffic monitoring
- Fast inference (<1ms per track)
- Confidence scoring provides uncertainty estimates

**Limitations:**
- Rigid thresholds fail for edge cases (e.g., cyclists, scooters)
- No learning from mislabeled tracks
- Cannot leverage raw point cloud geometry (shape priors)
- Limited to pre-defined classes (no open-set recognition)
- No temporal pattern recognition (gait, acceleration profiles)

---

### 3. Foreground Extraction Pipeline

**Implementation:** `internal/lidar/foreground.go`, `internal/lidar/background.go`

**Workflow:**
1. **Background Grid:** Polar grid (rings × azimuth bins) with EMA-updated range statistics
2. **Point Classification:** Per-point foreground/background mask via `ProcessFramePolarWithMask`
3. **Foreground Extraction:** `ExtractForegroundPoints` filters points where `mask[i] == true`
4. **World Transform:** Polar → Cartesian (sensor frame → world frame)
5. **Clustering:** DBSCAN (eps=0.6m, minPts=12) groups foreground points into clusters

**Key Metrics (Current):**
- Per-frame: `TotalPoints`, `ForegroundPoints`, `BackgroundPoints`, `ForegroundFraction`
- Grid-level: `ForegroundCount`, `BackgroundCount`, `nonzeroCellCount`

**Strengths:**
- Real-time classification (suitable for streaming LIDAR)
- Neighbor confirmation reduces false positives
- Frozen cells prevent background updates during transient objects
- Tunable sensitivity via `ClosenessSensitivityMultiplier`

**Limitations:**
- No per-cluster metrics (noise points per cluster, cluster stability over time)
- Missing track-level coverage analysis (% of track with unknown/noise classification)
- No track length/trajectory quality metrics
- Limited introspection for tuning (no histograms, no per-range acceptance rates)

---

### 4. Database Schema (Tracks & Analysis Runs)

**Tables:**
- `lidar_tracks`: Track lifecycle, aggregated features, classification
- `lidar_track_obs`: Per-frame observations (position, velocity, bounding box)
- `lidar_clusters`: Detected DBSCAN clusters
- `lidar_analysis_runs`: PCAP analysis sessions with versioned parameters
- `lidar_run_tracks`: Tracks per run with user labels, split/merge flags

**Analysis Run Capabilities:**
- Parameter versioning (JSON serialization of all configs)
- Run comparison (track diffs, split/merge candidates)
- User labeling for ML training (labels, confidence, labeler ID)
- Track quality flags (split/merge candidates, linked tracks)

**Strengths:**
- Full reproducibility via `params_json`
- Supports iterative tuning (parent_run_id for A/B testing)
- ML-ready with user labels and confidence scores
- Track lineage tracking (linked_track_ids)

**Gaps:**
- No track quality metrics (e.g., occlusion count, coverage %, noise ratio)
- Missing per-run aggregate statistics (avg track length, classification accuracy)
- No histogram/distribution storage for parameter tuning

---

## Enhancement Plan: Phased Approach

### **Phase 1: Enhanced Point Cloud Extraction & Track Quality Metrics**

**Goal:** Improve track quality visibility and enable data-driven parameter tuning.

#### 1.1 Per-Track Quality Metrics

**New Fields for `TrackedObject`:**
```go
type TrackedObject struct {
    // ... existing fields ...

    // Quality Metrics
    TrackLengthMeters   float32  // Total distance traveled
    TrackDurationSecs   float32  // Total lifetime in seconds
    OcclusionCount      int      // Number of missed frames (gaps)
    MaxOcclusionFrames  int      // Longest gap in observations
    SpatialCoverage     float32  // % of bounding box covered by observations
    NoisePointRatio     float32  // Ratio of noise points to cluster points
    ConfidenceHistory   []float32 // Per-observation confidence scores
}
```

**Computation Logic:**
- `TrackLengthMeters`: Sum of Euclidean distances between consecutive positions in `History`
- `TrackDurationSecs`: `(LastUnixNanos - FirstUnixNanos) / 1e9`
- `OcclusionCount`: Count of gaps in `History` timestamps > 200ms
- `MaxOcclusionFrames`: Longest gap detected (estimate frame count at 10Hz)
- `SpatialCoverage`: Ratio of observed bounding box volume to theoretical max
- `NoisePointRatio`: Track ratio of DBSCAN noise points to total foreground points

**Database Schema Update:**
```sql
ALTER TABLE lidar_tracks ADD COLUMN track_length_meters REAL;
ALTER TABLE lidar_tracks ADD COLUMN track_duration_secs REAL;
ALTER TABLE lidar_tracks ADD COLUMN occlusion_count INTEGER;
ALTER TABLE lidar_tracks ADD COLUMN max_occlusion_frames INTEGER;
ALTER TABLE lidar_tracks ADD COLUMN spatial_coverage REAL;
ALTER TABLE lidar_tracks ADD COLUMN noise_point_ratio REAL;
```

#### 1.2 Per-Cluster Noise Metrics

**Goal:** Quantify cluster quality for association confidence.

**New Fields for `WorldCluster`:**
```go
type WorldCluster struct {
    // ... existing fields ...

    // Quality Metrics
    NoisePointsCount    int     // DBSCAN noise points in cluster neighborhood
    ClusterDensity      float32 // Points per unit volume (points/m³)
    AspectRatio         float32 // Length/Width (shape elongation)
    Compactness         float32 // Ratio of volume to convex hull volume
}
```

**Computation:**
- `NoisePointsCount`: Count DBSCAN label=-1 points within cluster bounding box + margin
- `ClusterDensity`: `PointsCount / (BoundingBoxLength × Width × Height)`
- `AspectRatio`: `max(Length, Width) / min(Length, Width)`
- `Compactness`: Requires convex hull (defer to Phase 2 or use bounding box approximation)

**Database Schema:**
```sql
ALTER TABLE lidar_clusters ADD COLUMN noise_points_count INTEGER;
ALTER TABLE lidar_clusters ADD COLUMN cluster_density REAL;
ALTER TABLE lidar_clusters ADD COLUMN aspect_ratio REAL;
```

#### 1.3 Per-Run Aggregate Statistics

**New Function:** `ComputeRunStatistics(runID string) *RunStatistics`

```go
type RunStatistics struct {
    // Track Quality Distribution
    AvgTrackLength       float32  // Mean track length (meters)
    MedianTrackLength    float32  // p50 track length
    AvgTrackDuration     float32  // Mean track duration (seconds)
    AvgOcclusionCount    float32  // Mean occlusions per track

    // Classification Distribution
    ClassCounts          map[string]int // Tracks per class
    ClassConfidenceAvg   map[string]float32 // Avg confidence per class
    UnknownRatio         float32  // % of tracks classified as "other"

    // Noise & Coverage
    AvgNoiseRatio        float32  // Mean noise point ratio
    AvgSpatialCoverage   float32  // Mean spatial coverage

    // Track Lifecycle
    TentativeRatio       float32  // % tracks deleted while tentative
    ConfirmedRatio       float32  // % tracks reaching confirmed state
    AvgObservationsPerTrack int  // Mean observations per track
}
```

**Storage:** Add `statistics_json TEXT` to `lidar_analysis_runs` table.

---

### **Phase 2: Training Data Preparation & Point Cloud Export (REVISED)**

**Goal:** Extract and export isolated track point clouds for ML training data preparation and visual inspection in LidarView.

#### 2.1 Track Point Cloud Extraction

**New Module:** `internal/lidar/track_export.go`

**Goal:** Extract isolated point clouds for individual tracks in a format compatible with LidarView for visual inspection and ML training data preparation.

**Architecture:**
- Maintain polar frame representation (azimuth, elevation, distance) to minimize transformations
- Export track point clouds as Pandar40P-compatible UDP packets
- Support both PCAP file export and network streaming for real-time inspection

**Track Point Cloud Frame:**
```go
type TrackPointCloudFrame struct {
    TrackID     string        // Track identifier
    FrameIndex  int           // Frame sequence number within track
    Timestamp   time.Time     // Frame timestamp
    PolarPoints []PointPolar  // Points in polar coordinates (sensor frame)
}
```

**Export Workflow:**
1. Query track observations from database (`lidar_track_obs`)
2. For each observation, extract associated foreground point cloud (requires point cloud storage integration)
3. Filter points belonging to track's cluster/bounding box
4. Encode points into Pandar40P-compatible packets (maintaining polar coordinates)
5. Write packets to PCAP file or stream to UDP destination

#### 2.2 PCAP Packet Encoding

**Pandar40P Packet Format (1262 bytes):**
- **Data Blocks (1240 bytes):** 10 blocks × 124 bytes each
  - Block preamble: 0xFFEE (2 bytes)
  - Block azimuth: uint16, scaled by 100 (2 bytes)
  - Channel data: 40 channels × 3 bytes (distance + intensity)
    - Distance: uint16, 2mm resolution (2 bytes)
    - Intensity: uint8 (1 byte)
- **Tail (22 bytes):** Metadata including motor speed, timestamp, return mode

**Encoder Function:**
```go
func EncodePandar40PPacket(points []PointPolar, blockAzimuth float64, config *SensorConfig) ([]byte, error)
```

**Benefits:**
- **Parser compatibility:** Generated packets can be read by existing `parse.ParsePacket()` function
- **LidarView support:** PCAP files load directly in LidarView/ParaView for visual inspection
- **Minimal transformation:** Maintains polar coordinates from foreground extraction
- **Training ready:** Point clouds can be labeled and used for ML training

#### 2.3 Training Data Curation

**New Module:** `internal/lidar/quality.go` (revised focus)

**Goal:** Filter tracks suitable for ML training based on quality criteria.

**Training Data Filter:**
```go
type TrainingDataFilter struct {
    MinQualityScore  float32  // Minimum composite quality score (0-1)
    MinDuration      float32  // Minimum track duration (seconds)
    MinLength        float32  // Minimum track length (meters)
    MaxOcclusionRatio float32 // Maximum occlusion ratio
    MinObservations  int      // Minimum observation count
    RequireClass     bool     // Only include tracks with assigned class
    AllowedStates    []TrackState // e.g., only confirmed tracks
}
```

**Default Criteria (High-Quality Training Tracks):**
- Quality score ≥ 0.6 (good quality or better)
- Duration ≥ 2.0 seconds (sustained observation)
- Length ≥ 5.0 meters (sufficient movement)
- Occlusion ratio ≤ 0.3 (max 30% gaps)
- Observations ≥ 20 frames (2s @ 10Hz)
- State = confirmed (validated tracks only)

**Curation Functions:**
```go
// Filter tracks meeting quality criteria
func FilterTracksForTraining(tracks []*TrackedObject, filter *TrainingDataFilter) []*TrackedObject

// Generate dataset statistics
func SummarizeTrainingDataset(tracks []*TrackedObject) *TrainingDatasetSummary
```

**Dataset Summary:**
```go
type TrainingDatasetSummary struct {
    TotalTracks      int                // Count of curated tracks
    TotalFrames      int                // Total observations
    TotalPoints      int                // Total point cloud points
    ClassDistribution map[string]int    // Tracks per class (pedestrian, car, etc.)
    AvgQualityScore  float32            // Mean quality score
    AvgDuration      float32            // Mean track duration (seconds)
    AvgLength        float32            // Mean track length (meters)
}
```

#### 2.4 Export Formats & Workflow

**PCAP File Export:**
```
1. Select high-quality tracks: FilterTracksForTraining()
2. For each track:
   a. Extract point cloud frames
   b. Encode frames as Pandar40P packets
   c. Write packets to PCAP file: track_<trackID>.pcap
3. Generate metadata JSON: track_<trackID>.json
```

**Network Streaming (Real-Time Inspection):**
```
1. Start LidarView listening on UDP port 2368
2. Stream isolated track packets in real-time
3. Allows immediate visual verification of track quality
```

**Metadata Export (JSON):**
```json
{
  "track_id": "track_001",
  "sensor_id": "hesai-pandar40p",
  "start_time": "2024-12-10T12:00:00Z",
  "end_time": "2024-12-10T12:00:05Z",
  "total_frames": 50,
  "total_points": 7500,
  "object_class": "car",
  "object_confidence": 0.85,
  "track_length_meters": 25.5,
  "duration_secs": 5.0,
  "occlusion_count": 2,
  "quality_score": 0.78
}
```

#### 2.5 Integration Requirements

**Point Cloud Storage:**
- **Current gap:** Foreground points are not persisted beyond frame processing
- **Requirement:** Store or stream foreground points with track/cluster association
- **Options:**
  1. Store raw foreground point clouds in database (compressed BLOB)
  2. Stream foreground points to file during PCAP analysis
  3. Re-play PCAP with track IDs to reconstruct point clouds

**Recommended Approach:**
During PCAP analysis mode, simultaneously:
- Run foreground extraction and tracking (as current)
- Write foreground points to temporary storage with frame timestamps
- After tracking completes, extract point clouds for confirmed tracks
- Export track point clouds as individual PCAP files

**Implementation Phases:**
1. **Phase 2a (Current PR):** Packet encoding infrastructure (`track_export.go`)
2. **Phase 2b (Future PR):** Point cloud storage/streaming during PCAP analysis
3. **Phase 2c (Future PR):** End-to-end export pipeline with LidarView validation

---

### **Phase 3: Advanced Introspection & Charting**

**Goal:** Real-time and post-hoc analysis of track quality, parameter sensitivity, and noise characteristics.

#### 3.1 Track Quality Dashboard

**New API Endpoints:**

**Track Quality Summary:**
```
GET /api/lidar/tracks/quality?run_id=<run_id>

Response:
{
  "track_count": 150,
  "confirmed_count": 120,
  "tentative_count": 15,
  "deleted_count": 15,

  "length_distribution": {
    "p10": 5.2, "p50": 18.5, "p90": 45.3, "max": 120.5
  },
  "duration_distribution": {
    "p10": 1.2, "p50": 4.5, "p90": 12.8, "max": 35.2
  },
  "occlusion_distribution": {
    "mean": 2.3, "p50": 2, "p90": 5, "max": 12
  },
  "noise_ratio_distribution": {
    "mean": 0.15, "p50": 0.12, "p90": 0.28
  }
}
```

**Track Timeline (Per-Track Introspection):**
```
GET /api/lidar/tracks/<track_id>/timeline

Response:
{
  "track_id": "track_001",
  "observations": [
    {
      "timestamp": 1733875200000000000,
      "position": {"x": 10.5, "y": -3.2, "z": 1.5},
      "velocity": {"x": 5.0, "y": 0.0},
      "speed": 5.0,
      "cluster_points": 150,
      "noise_points": 12,
      "confidence": 0.85,
      "occlusion": false
    },
    // ... more observations ...
    {
      "timestamp": 1733875201500000000,
      "position": {"x": 18.5, "y": -3.5, "z": 1.5},
      "velocity": {"x": 5.2, "y": -0.2},
      "speed": 5.2,
      "cluster_points": 140,
      "noise_points": 15,
      "confidence": 0.82,
      "occlusion": true  // Gap detected
    }
  ],
  "quality_metrics": {
    "track_length_meters": 28.5,
    "duration_secs": 5.2,
    "occlusion_count": 3,
    "avg_confidence": 0.83
  }
}
```

#### 3.2 Parameter Sensitivity Analysis

**Goal:** Understand how parameter changes affect track quality.

**Tunable Parameters to Analyze:**
- Background: `ClosenessSensitivityMultiplier`, `NeighborConfirmationCount`
- Clustering: `DBSCAN.Eps`, `DBSCAN.MinPts`
- Tracking: `GatingDistanceSquared`, `MaxMisses`, `HitsToConfirm`

**Sensitivity Workflow:**
1. Run baseline PCAP analysis with default parameters → `run_baseline`
2. Run parameter sweeps (e.g., vary `Eps` from 0.4 to 1.0 in 0.1 steps)
3. For each run, compute:
   - Track count, confirmed ratio, avg length, avg duration
   - Classification distribution, confidence averages
   - Noise ratio, occlusion counts
4. Generate comparison charts (parameter value vs metric)

**API Endpoint:**
```
POST /api/lidar/analysis/sensitivity
{
  "pcap_file": "test_data.pcapng",
  "base_params": { ... },
  "sweep_config": {
    "parameter": "clustering.eps",
    "values": [0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0]
  }
}

Response:
{
  "sweep_id": "sweep_20241210_001",
  "runs": [
    {"run_id": "run_eps_0.4", "status": "completed"},
    {"run_id": "run_eps_0.5", "status": "completed"},
    ...
  ],
  "results": [
    {"eps": 0.4, "track_count": 95, "confirmed_ratio": 0.68, "avg_length": 15.2},
    {"eps": 0.5, "track_count": 110, "confirmed_ratio": 0.72, "avg_length": 18.5},
    {"eps": 0.6, "track_count": 120, "confirmed_ratio": 0.75, "avg_length": 20.1},
    ...
  ]
}
```

**Chart Output:**
- Line chart: Parameter value (x-axis) vs Track count / Avg length / Confirmed ratio (y-axes)
- Box plot: Track length distribution per parameter value
- Heatmap: Pairwise parameter interaction (e.g., `Eps` × `MinPts`)

#### 3.3 Noise & Unknown Classification Coverage

**Goal:** Quantify "how much we don't know" about detected objects.

**Metrics:**

**Per-Run Noise Coverage:**
```go
type NoiseCoverageMetrics struct {
    TotalTracks              int
    TracksWithHighNoise      int     // noise_ratio > 0.3
    TracksUnknownClass       int     // object_class == "other"
    TracksLowConfidence      int     // object_confidence < 0.6

    NoiseRatioDistribution   []float32  // Histogram bins
    UnknownRatioBySpeed      map[string]float32  // "slow"/"medium"/"fast" → % unknown
    UnknownRatioBySize       map[string]float32  // "small"/"medium"/"large" → % unknown
}
```

**API Endpoint:**
```
GET /api/lidar/runs/<run_id>/noise_coverage

Response:
{
  "total_tracks": 150,
  "high_noise_tracks": 18,  // 12%
  "unknown_class_tracks": 22,  // 14.7%
  "low_confidence_tracks": 30,  // 20%

  "noise_ratio_histogram": {
    "bins": [0.0, 0.1, 0.2, 0.3, 0.4, 0.5],
    "counts": [80, 45, 15, 7, 2, 1]
  },

  "unknown_by_speed": {
    "slow": 0.25,      // 25% of slow tracks are "other"
    "medium": 0.10,    // 10% of medium tracks
    "fast": 0.05       // 5% of fast tracks
  },

  "unknown_by_size": {
    "small": 0.35,     // Birds, debris
    "medium": 0.12,    // Bicycles, scooters
    "large": 0.03      // Most vehicles classified
  }
}
```

**Visualization:**
- Bar chart: Unknown ratio by speed/size category
- Pie chart: Classification distribution with "unknown" highlighted
- Time series: Unknown ratio over time (detect degradation)

#### 3.4 Track Trajectory Visualization

**Goal:** Render track paths for visual inspection of quality.

**New Export Format:** GeoJSON for web mapping tools.

**API Endpoint:**
```
GET /api/lidar/tracks/<track_id>/trajectory.geojson

Response:
{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "geometry": {
        "type": "LineString",
        "coordinates": [[10.5, -3.2], [12.3, -3.3], [15.1, -3.5], ...]
      },
      "properties": {
        "track_id": "track_001",
        "object_class": "car",
        "confidence": 0.85,
        "avg_speed": 5.2,
        "duration": 4.5,
        "occlusion_count": 2
      }
    }
  ]
}
```

**Web Rendering:**
- Use Leaflet.js or Mapbox GL for 2D trajectory overlay
- Color-code by: speed, confidence, classification, noise ratio
- Add markers at occlusion points
- Interactive: click track → show timeline chart

---

### **Phase 4: Track Split/Merge Detection & Correction**

**Goal:** Identify and optionally correct tracking errors where single objects are split or multiple objects are merged.

#### 4.1 Split Detection Heuristics

**Split Indicators:**
1. **Spatial Proximity:** Two tracks with overlapping bounding boxes at similar times
2. **Temporal Correlation:** Track A ends, Track B starts within <500ms at nearby location
3. **Kinematic Continuity:** Velocity vectors align (heading difference < 15°)
4. **Feature Similarity:** Similar size, speed, classification

**Algorithm:**
```go
func DetectSplitCandidates(tracks []*TrackedObject) []TrackSplit {
    splits := []TrackSplit{}

    for i, track1 := range tracks {
        for j, track2 := range tracks {
            if i >= j { continue }

            // Check temporal gap
            gap := track2.FirstUnixNanos - track1.LastUnixNanos
            if gap < 0 || gap > 500e6 { continue }  // 500ms threshold

            // Check spatial proximity at handoff
            endPos1 := track1.History[len(track1.History)-1]
            startPos2 := track2.History[0]
            dist := distance(endPos1, startPos2)
            if dist > 2.0 { continue }  // 2m threshold

            // Check velocity alignment
            v1 := track1.VX, track1.VY
            v2 := track2.VX, track2.VY
            headingDiff := angleDiff(v1, v2)
            if headingDiff > 15 * math.Pi / 180 { continue }

            // Compute confidence score
            conf := computeSplitConfidence(track1, track2, dist, gap, headingDiff)
            if conf > 0.6 {
                splits = append(splits, TrackSplit{
                    OriginalTrack: track1.TrackID,
                    SplitTracks: []string{track2.TrackID},
                    SplitX: endPos1.X,
                    SplitY: endPos1.Y,
                    Confidence: conf,
                })
            }
        }
    }

    return splits
}
```

#### 4.2 Merge Detection Heuristics

**Merge Indicators:**
1. **Spatial Convergence:** Two tracks approach same location
2. **Temporal Overlap:** Tracks exist simultaneously, one disappears
3. **Occlusion Pattern:** Track A temporarily disappears, Track B takes over

**Algorithm:** Similar to split detection but checks for converging trajectories.

#### 4.3 Split/Merge Correction (Semi-Automatic)

**Workflow:**
1. Run PCAP analysis → detect split/merge candidates
2. Store candidates in `lidar_run_tracks.is_split_candidate`, `linked_track_ids`
3. Present candidates to user for review (web UI)
4. User confirms/rejects corrections
5. Apply corrections: merge track histories, update statistics

**Database Updates:**
```sql
-- Mark split candidates
UPDATE lidar_run_tracks
SET is_split_candidate = 1, linked_track_ids = '["track_002"]'
WHERE run_id = 'run_001' AND track_id = 'track_001';

-- Merge tracks (create new corrected track)
INSERT INTO lidar_run_tracks (run_id, track_id, ...)
SELECT 'run_001', 'track_001_002_merged', ...
FROM lidar_run_tracks
WHERE run_id = 'run_001' AND track_id IN ('track_001', 'track_002');
```

---

## Implementation Roadmap

### **Priority 1: Foundation (Phase 1)**
**Effort:** 2-3 weeks
**Dependencies:** None

**Deliverables:**
1. Track quality metrics (length, duration, occlusions, noise ratio)
2. Per-run aggregate statistics
3. Database schema updates + migrations
4. Basic API endpoints for quality queries

**Validation:**
- Run PCAP analysis → verify metrics populate
- Compare baseline vs noisy PCAP → observe metric differences

---

### **Priority 2: Classification Enhancement (Phase 2)**
**Effort:** 3-4 weeks
**Dependencies:** Phase 1 complete

**Deliverables:**
1. Feature extraction module (`features.go`)
2. Static ML classifier interface + ONNX loader
3. Training data export (CSV/JSON with features + labels)
4. Model comparison framework (A/B testing API)

**Validation:**
- Train simple Random Forest on labeled data
- Compare rule-based vs ML classifier accuracy
- Export features for offline experimentation (scikit-learn, XGBoost)

---

### **Priority 3: Introspection & Tuning (Phase 3)**
**Effort:** 2-3 weeks
**Dependencies:** Phases 1-2 complete

**Deliverables:**
1. Track quality dashboard (API + web UI)
2. Parameter sensitivity analysis tool
3. Noise coverage metrics + visualization
4. Trajectory export (GeoJSON) + web rendering

**Validation:**
- Run parameter sweeps (vary `Eps`, `GatingDistanceSquared`)
- Generate comparison charts (line plots, heatmaps)
- Render track trajectories in browser (Leaflet.js)

---

### **Priority 4: Track Correction (Phase 4)**
**Effort:** 2-3 weeks
**Dependencies:** Phases 1-3 complete

**Deliverables:**
1. Split/merge detection algorithms
2. Candidate storage in database
3. Web UI for candidate review
4. Semi-automatic correction workflow

**Validation:**
- Identify known split/merge cases in test PCAPs
- Verify detection algorithm recalls 80%+ of cases
- Test correction workflow (merge track histories)

---

## Success Metrics

### **Phase 1 Success Criteria:**
- ✅ Track quality metrics populated for 100% of tracks
- ✅ Per-run statistics API returns in <1s for 500-track runs
- ✅ Noise ratio correlates with manual inspection (visual agreement)

### **Phase 2 Success Criteria:**
- ✅ Feature extraction completes in <10ms per track
- ✅ ML classifier accuracy exceeds rule-based by >10% on test set
- ✅ Training data export includes 30+ features with proper normalization

### **Phase 3 Success Criteria:**
- ✅ Parameter sensitivity analysis identifies optimal `Eps` value (max confirmed tracks)
- ✅ Noise coverage metrics highlight under-classified categories (e.g., bicycles)
- ✅ Track trajectory visualization loads in browser with <2s latency

### **Phase 4 Success Criteria:**
- ✅ Split/merge detection recall >80% on manually labeled test set
- ✅ False positive rate <20% for split/merge candidates
- ✅ Corrected tracks show improved length/duration metrics (>30% increase)

---

## Technical Considerations

### **Performance (Raspberry Pi 4 Constraints):**
- Feature extraction: Vectorized operations, avoid per-point loops
- ML inference: Use quantized models (INT8), batch predictions
- Database queries: Pre-compute aggregates, use indexes on `run_id`, `track_id`
- Web UI: Lazy-load trajectories, paginate track lists

### **Storage:**
- Track quality metrics: ~50 bytes per track (minimal overhead)
- Feature vectors: ~200 bytes per track (compress for long-term storage)
- Trajectory GeoJSON: ~10KB per track (serve on-demand, cache)

### **Privacy:**
- No PII in track data (position/speed only)
- User labels stored locally (no cloud transmission)
- GeoJSON exports use relative coordinates (site-local frame)

---

## Appendix: Example Use Cases

### **Use Case 1: Tuning for Bicycle Detection**

**Problem:** Bicycles often classified as "other" (unknown class).

**Workflow:**
1. Run PCAP analysis with default parameters
2. Query noise coverage: `GET /api/lidar/runs/<run_id>/noise_coverage`
3. Observe high "unknown" ratio for medium-speed, medium-size tracks
4. Export training data: Filter tracks with `user_label == "bicycle"`
5. Train ML classifier with bicycle-specific features (aspect ratio, speed profile)
6. Re-run analysis with new classifier
7. Compare classification accuracy (target: >90% bicycle recall)

### **Use Case 2: Detecting Track Splits at Intersections**

**Problem:** Vehicles turning cause tracking to split into separate tracks.

**Workflow:**
1. Run PCAP analysis on intersection data
2. Detect split candidates: `POST /api/lidar/tracks/detect_splits`
3. Review candidates in web UI (overlay trajectories)
4. Confirm splits where velocity vectors align
5. Merge tracks: Update database with corrected track IDs
6. Re-compute statistics: Verify avg track length increases

### **Use Case 3: Parameter Optimization for Noisy Environments**

**Problem:** Parking lot has high reflections, causing false detections.

**Workflow:**
1. Run baseline analysis with default `ClosenessSensitivityMultiplier = 3.0`
2. Launch sensitivity analysis: Sweep from 2.0 to 5.0 in 0.5 steps
3. Generate chart: Multiplier (x-axis) vs Noise ratio (y-axis)
4. Identify optimal value: Lowest noise ratio while maintaining track count
5. Update configuration, re-run analysis
6. Validate: Noise ratio decreases by >30%

---

## References

**Existing Documentation:**
- `docs/features/lidar-tracking-integration.md` - Integration status
- `docs/features/pcap-analysis-mode.md` - PCAP analysis workflow
- `internal/lidar/tracking.go` - Kalman filter implementation
- `internal/lidar/classification.go` - Rule-based classifier
- `internal/lidar/clustering.go` - DBSCAN algorithm

**External Standards:**
- KITTI Tracking Benchmark (evaluation metrics)
- MOT Challenge (multi-object tracking benchmarks)
- ONNX Runtime (cross-platform ML inference)
- GeoJSON Specification (trajectory export format)

---

## Conclusion

This enhancement plan transforms the LIDAR tracking system from a basic tracker into a comprehensive analysis platform with:
- **Quantifiable track quality** via metrics (length, occlusions, noise)
- **ML-ready classification** with feature extraction and static model support
- **Data-driven tuning** via parameter sensitivity analysis
- **Introspection tools** for coverage, noise, and trajectory visualization
- **Track correction** for split/merge error handling

The phased approach allows incremental delivery with each phase building on previous work. Priority 1 (quality metrics) provides immediate value for validation, while later phases enable advanced workflows (ML training, parameter optimization).

**Next Steps:**
1. Review and approve enhancement plan
2. Create GitHub issues for Phase 1 deliverables
3. Begin implementation: Track quality metrics + database schema
4. Iterate based on feedback from PCAP analysis testing
