# Waymo Open Dataset Integration Plan

**Status:** Planning  
**Date:** December 16, 2025  
**Author:** Agent Ictinus (Product Architecture)  
**Version:** 1.0

---

## Executive Summary

This document outlines a comprehensive plan to align velocity.report's LIDAR data structures with the Waymo Open Dataset Perception format, enabling ingestion of Waymo Parquet data for ML training. The goal is to create a robust LIDAR frame analyzer that can leverage the world-class Waymo perception dataset for training object detection and classification models.

**Key Objective:** Ingest Waymo Perception LiDAR data (with 3D bounding box labels, tracking IDs, and No Label Zones) into velocity.report's planned LIDAR frame analyzer for ML training.

---

## Table of Contents

1. [Waymo Dataset Overview](#waymo-dataset-overview)
2. [Current velocity.report Capabilities](#current-velocityreport-capabilities)
3. [Data Structure Gap Analysis](#data-structure-gap-analysis)
4. [Phase 1: Core Data Structure Alignment](#phase-1-core-data-structure-alignment)
5. [Phase 2: Parquet Ingestion Pipeline](#phase-2-parquet-ingestion-pipeline)
6. [Phase 3: No Label Zone (NLZ) Support](#phase-3-no-label-zone-nlz-support)
7. [Phase 4: ML Training Integration](#phase-4-ml-training-integration)
8. [Phase 5: Frame Analyzer Tool](#phase-5-frame-analyzer-tool)
9. [Tool Requirements Matrix](#tool-requirements-matrix)
10. [Implementation Timeline](#implementation-timeline)
11. [Future Considerations](#future-considerations)

---

## Waymo Dataset Overview

### Waymo Open Dataset Perception - LiDAR Labels

The Waymo Open Dataset provides **3D bounding box labels** for LiDAR data with the following characteristics:

#### 7-DOF Bounding Box Format

Each labeled object is represented by a **7-Degree-of-Freedom (7-DOF) bounding box** in the **vehicle frame**:

| Parameter | Description | Unit | Notes |
|-----------|-------------|------|-------|
| `center_x` | Center X position | meters | Vehicle frame |
| `center_y` | Center Y position | meters | Vehicle frame |
| `center_z` | Center Z position | meters | Vehicle frame |
| `length` | Box extent along vehicle +X | meters | Forward axis |
| `width` | Box extent along vehicle +Y | meters | Left-right axis |
| `height` | Box extent along vehicle +Z | meters | Up-down axis |
| `heading` | Yaw angle (rotation around Z) | radians | Range: [-π, π] |

**Critical Properties:**
- **Zero pitch and zero roll**: Boxes are always parallel to the ground plane
- **Heading**: Angle to rotate vehicle frame +X to align with object's forward axis
- **Vehicle frame**: Not sensor frame - requires pose transformation

#### Labeled Object Classes

| Class | Description |
|-------|-------------|
| `VEHICLE` | Cars, trucks, motorcycles |
| `PEDESTRIAN` | Pedestrians, people |
| `CYCLIST` | Cyclists, people on bikes |
| `SIGN` | Traffic signs |

#### Tracking and Identity

- **Globally unique tracking IDs**: Objects have consistent IDs across frames
- **Scene-level tracking**: Same object maintains ID throughout a scene

#### No Label Zone (NLZ)

**Definition:** Areas in a scene that are not labeled (e.g., opposite side of highway)

**Representation:**
- Polygons in the global frame (not necessarily convex)
- Each LiDAR point annotated with a boolean (`in_nlz`)
- Both 1st and 2nd return points have NLZ annotation
- Predictions overlapping NLZ points should be flagged

### Waymo Data Format

**Storage Format:** Apache Parquet (modular v2 format)

**Key Components:**
- `lidar_box`: 3D bounding box labels with tracking IDs
- `lidar`: Raw LiDAR point clouds (range images)
- `vehicle_pose`: Vehicle-to-world transformation per frame
- `lidar_calibration`: Sensor extrinsics and intrinsics

**Example Parquet Schema (lidar_box):**
```
key.segment_context_name: string
key.frame_timestamp_micros: int64
key.laser_object_id: string
box.center.x: float64
box.center.y: float64
box.center.z: float64
box.size.x: float64  (length)
box.size.y: float64  (width)
box.size.z: float64  (height)
box.heading: float64
type: int32  (object class enum)
num_lidar_points_in_box: int32
difficulty_level: int32
```

---

## Current velocity.report Capabilities

### Existing Data Structures

#### WorldCluster (clustering.go)
```go
type WorldCluster struct {
    ClusterID         int64
    SensorID          string
    TSUnixNanos       int64
    CentroidX         float32  // World frame
    CentroidY         float32
    CentroidZ         float32
    BoundingBoxLength float32  // X extent
    BoundingBoxWidth  float32  // Y extent
    BoundingBoxHeight float32  // Z extent
    PointsCount       int
    HeightP95         float32
    IntensityMean     float32
    ClusterDensity    float32
    AspectRatio       float32
    NoisePointsCount  int
}
```

**Gaps vs Waymo:**
- ❌ No heading/yaw angle
- ❌ No tracking ID at cluster level
- ❌ No object class (added at track level)
- ✅ Bounding box dimensions (length, width, height)
- ✅ World frame coordinates

#### TrackedObject (tracking.go)
```go
type TrackedObject struct {
    TrackID                 string
    SensorID                string
    State                   TrackState
    X, Y                    float32  // World frame
    VX, VY                  float32  // Velocity
    BoundingBoxLengthAvg    float32
    BoundingBoxWidthAvg     float32
    BoundingBoxHeightAvg    float32
    ObjectClass             string   // Classification result
    ObjectConfidence        float32
    // ... other fields
}
```

**Gaps vs Waymo:**
- ❌ No heading/yaw angle
- ❌ No NLZ annotation
- ✅ Tracking ID (TrackID)
- ✅ Object class
- ✅ Bounding box dimensions

#### ForegroundFrame (training_data.go)
```go
type ForegroundFrame struct {
    SensorID         string
    TSUnixNanos      int64
    SequenceID       string
    ForegroundPoints []PointPolar  // Polar coordinates
    TotalPoints      int
    BackgroundPoints int
}
```

**Gaps vs Waymo:**
- ❌ No 3D bounding box labels
- ❌ No object class labels per frame
- ❌ No tracking ID associations
- ❌ No NLZ annotations

### Current ML Pipeline Status

| Component | Status | Description |
|-----------|--------|-------------|
| Background subtraction | ✅ Complete | EMA grid-based |
| DBSCAN clustering | ✅ Complete | Spatial indexing |
| Kalman tracking | ✅ Complete | Multi-object tracking |
| Rule-based classification | ✅ Complete | Pedestrian, car, bird, other |
| Analysis Run Infrastructure | ✅ Complete | Versioned runs with params |
| Training data export | ✅ Complete | Compact binary encoding |
| Track labeling UI | 📋 Planned | Phase 4.0 |
| ML classifier training | 📋 Planned | Phase 4.1 |

---

## Data Structure Gap Analysis

### Required Additions for Waymo Compatibility

| Gap | Priority | Effort | Description |
|-----|----------|--------|-------------|
| **Heading angle** | P0 - Required | Low | Add yaw angle to clusters and tracks |
| **7-DOF bounding box type** | P0 - Required | Medium | New type matching Waymo format |
| **Ground truth labels** | P0 - Required | Medium | Per-frame object labels with class |
| **NLZ polygon support** | P1 - High | Medium | Polygon storage and point annotation |
| **Parquet reader** | P0 - Required | High | Read Waymo parquet files |
| **Frame-to-vehicle transform** | P1 - High | Medium | Coordinate frame handling |
| **Global tracking ID** | P1 - High | Low | Waymo-compatible track IDs |
| **Object difficulty level** | P2 - Medium | Low | Waymo difficulty annotation |

### Coordinate Frame Alignment

**Waymo Convention:**
- +X: Forward (vehicle direction)
- +Y: Left
- +Z: Up
- Labels in vehicle frame (requires pose transform for world frame)

**velocity.report Convention:**
- Sensor frame → World frame transformation via Pose
- Currently no heading angle computed

**Alignment Strategy:**
- Import Waymo labels in vehicle frame
- Store vehicle pose per frame
- Transform to world frame for analysis
- Compute heading from velocity or Waymo annotation

---

## Phase 1: Core Data Structure Alignment

### Objective
Extend velocity.report data structures to support Waymo 7-DOF bounding boxes and ground truth labels.

### 1.1 BoundingBox7DOF Type

**File:** `internal/lidar/waymo_types.go` (new)

```go
// BoundingBox7DOF represents a 7-DOF 3D bounding box in Waymo format.
// Used for ground truth labels and predictions.
type BoundingBox7DOF struct {
    // Center position (meters)
    CenterX float64 `json:"center_x"`
    CenterY float64 `json:"center_y"`
    CenterZ float64 `json:"center_z"`
    
    // Dimensions (meters)
    Length float64 `json:"length"` // Extent along local X
    Width  float64 `json:"width"`  // Extent along local Y
    Height float64 `json:"height"` // Extent along local Z
    
    // Heading (radians, [-π, π])
    // Rotation around Z-axis to align +X with object forward
    Heading float64 `json:"heading"`
}

// Corners returns the 8 corner points of the bounding box in local frame.
func (b *BoundingBox7DOF) Corners() [8][3]float64

// ContainsPoint checks if a point (in the same frame) is inside the box.
func (b *BoundingBox7DOF) ContainsPoint(x, y, z float64) bool

// Volume returns the volume of the bounding box in cubic meters.
func (b *BoundingBox7DOF) Volume() float64

// IoU computes Intersection over Union with another box.
func (b *BoundingBox7DOF) IoU(other *BoundingBox7DOF) float64
```

### 1.2 Object Label Type

```go
// ObjectLabel represents a ground truth label for a detected object.
// Matches Waymo LiDARBoxComponent structure.
type ObjectLabel struct {
    // Identity
    ObjectID   string `json:"object_id"`   // Globally unique tracking ID
    FrameID    string `json:"frame_id"`    // Frame context
    SensorID   string `json:"sensor_id"`   // Source sensor
    
    // Timestamp
    TimestampMicros int64 `json:"timestamp_micros"`
    
    // Bounding box (7-DOF)
    Box BoundingBox7DOF `json:"box"`
    
    // Classification
    ObjectType      WaymoObjectClass `json:"object_type"` // VEHICLE, PEDESTRIAN, CYCLIST, SIGN
    DifficultyLevel int              `json:"difficulty_level"`
    
    // LiDAR metadata
    NumLidarPointsInBox int  `json:"num_lidar_points_in_box"`
    InNoLabelZone       bool `json:"in_no_label_zone"`
}

// ObjectClass enum matching Waymo types
type WaymoObjectClass int

const (
    WaymoTypeUnknown    WaymoObjectClass = 0
    WaymoTypeVehicle    WaymoObjectClass = 1
    WaymoTypePedestrian WaymoObjectClass = 2
    WaymoTypeCyclist    WaymoObjectClass = 3
    WaymoTypeSign       WaymoObjectClass = 4
)
```

### 1.3 LabeledFrame Type

```go
// LabeledFrame represents a single LIDAR frame with ground truth labels.
// This is the primary unit for ML training data.
type LabeledFrame struct {
    // Frame identity
    ContextName     string `json:"context_name"`     // Waymo segment name
    FrameTimestamp  int64  `json:"frame_timestamp"`  // Microseconds
    SequenceIndex   int    `json:"sequence_index"`   // Frame index in sequence
    
    // Coordinate transforms
    VehiclePose [16]float64 `json:"vehicle_pose"` // 4x4 vehicle-to-world matrix
    
    // Ground truth labels
    Labels []ObjectLabel `json:"labels"`
    
    // No Label Zones (polygons in global frame)
    NoLabelZones []NoLabelZone `json:"no_label_zones,omitempty"`
    
    // Point cloud metadata (not the full cloud)
    NumPoints       int `json:"num_points"`
    NumPointsReturn1 int `json:"num_points_return1"`
    NumPointsReturn2 int `json:"num_points_return2"`
    NumPointsInNLZ  int `json:"num_points_in_nlz"`
}
```

### 1.4 Extend WorldCluster with Heading

**File:** `internal/lidar/clustering.go`

Add the following fields to the existing `WorldCluster` struct:

```go
// Fields to add to existing WorldCluster struct:
    
    // Heading angle (radians, [-π, π])
    // Computed from principal component analysis or velocity
    Heading           float32 `json:"heading"`
    HeadingSource     string  `json:"heading_source"`     // "pca", "velocity", "none"
    HeadingConfidence float32 `json:"heading_confidence"`
```

These fields extend the existing WorldCluster (defined in `clustering.go`) to support orientation information needed for Waymo compatibility.

**Heading Computation Methods:**
1. **PCA**: Principal Component Analysis of point distribution
2. **Velocity**: From track velocity (VX, VY)
3. **Motion History**: Direction of travel from track history

### 1.5 Database Schema Extensions

**File:** `internal/db/migrations/000014_waymo_integration.up.sql`

```sql
-- Ground truth labels table (for Waymo import)
CREATE TABLE IF NOT EXISTS lidar_ground_truth_labels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    context_name TEXT NOT NULL,
    frame_timestamp_micros INTEGER NOT NULL,
    object_id TEXT NOT NULL,
    
    -- 7-DOF bounding box
    center_x REAL NOT NULL,
    center_y REAL NOT NULL,
    center_z REAL NOT NULL,
    length REAL NOT NULL,
    width REAL NOT NULL,
    height REAL NOT NULL,
    heading REAL NOT NULL,
    
    -- Classification
    object_type INTEGER NOT NULL,
    difficulty_level INTEGER DEFAULT 1,
    num_lidar_points INTEGER,
    in_no_label_zone INTEGER DEFAULT 0,
    
    -- Import metadata
    import_run_id TEXT,
    imported_at INTEGER DEFAULT (UNIXEPOCH()),
    
    UNIQUE(context_name, frame_timestamp_micros, object_id)
);

CREATE INDEX idx_gt_labels_context ON lidar_ground_truth_labels(context_name);
CREATE INDEX idx_gt_labels_timestamp ON lidar_ground_truth_labels(frame_timestamp_micros);
CREATE INDEX idx_gt_labels_object ON lidar_ground_truth_labels(object_id);
CREATE INDEX idx_gt_labels_type ON lidar_ground_truth_labels(object_type);

-- No Label Zones table
CREATE TABLE IF NOT EXISTS lidar_no_label_zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    context_name TEXT NOT NULL,
    frame_timestamp_micros INTEGER NOT NULL,
    zone_index INTEGER NOT NULL,
    
    -- Polygon vertices (JSON array of [x, y] pairs)
    polygon_vertices_json TEXT NOT NULL,
    
    -- Metadata
    import_run_id TEXT,
    imported_at INTEGER DEFAULT (UNIXEPOCH()),
    
    UNIQUE(context_name, frame_timestamp_micros, zone_index)
);

CREATE INDEX idx_nlz_context ON lidar_no_label_zones(context_name);

-- Frame metadata table
CREATE TABLE IF NOT EXISTS lidar_frame_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    context_name TEXT NOT NULL,
    frame_timestamp_micros INTEGER NOT NULL,
    sequence_index INTEGER,
    
    -- Vehicle pose (4x4 matrix as JSON array)
    vehicle_pose_json TEXT,
    
    -- Point cloud statistics
    num_points_total INTEGER,
    num_points_return1 INTEGER,
    num_points_return2 INTEGER,
    num_points_in_nlz INTEGER,
    num_labeled_objects INTEGER,
    
    -- Import metadata
    import_run_id TEXT,
    imported_at INTEGER DEFAULT (UNIXEPOCH()),
    
    UNIQUE(context_name, frame_timestamp_micros)
);

CREATE INDEX idx_frame_meta_context ON lidar_frame_metadata(context_name);
```

---

## Phase 2: Parquet Ingestion Pipeline

### Objective
Create a robust Parquet reader for Waymo Open Dataset files.

### 2.1 Parquet Reader (Required)

**File:** `internal/lidar/waymo/parquet_reader.go`

**Dependencies:**
- `github.com/parquet-go/parquet-go` (Apache Arrow Parquet for Go)
- Alternative: `github.com/xitongsys/parquet-go`

```go
// WaymoParquetReader reads Waymo Open Dataset Parquet files.
type WaymoParquetReader struct {
    basePath string
    // Internal reader from parquet-go library
}

// ReadLidarBoxes reads 3D bounding box labels from lidar_box component.
func (r *WaymoParquetReader) ReadLidarBoxes(contextName string) ([]ObjectLabel, error)

// ReadFrameMetadata reads frame-level metadata including poses.
func (r *WaymoParquetReader) ReadFrameMetadata(contextName string) ([]LabeledFrame, error)

// ReadNoLabelZones reads NLZ polygons for a context.
func (r *WaymoParquetReader) ReadNoLabelZones(contextName string) ([]NoLabelZone, error)

// ListContexts lists all available segment contexts.
func (r *WaymoParquetReader) ListContexts() ([]string, error)
```

### 2.2 Import Command (Required)

**File:** `cmd/tools/waymo-import/main.go`

```go
// waymo-import imports Waymo Parquet data into velocity.report database.
//
// Usage:
//   waymo-import --input /path/to/waymo/parquet --db sensor_data.db
//   waymo-import --input /path/to/waymo/parquet --components lidar_box,lidar_calibration
//
// Flags:
//   --input       Path to Waymo Parquet dataset directory
//   --db          Path to SQLite database (default: sensor_data.db)
//   --components  Comma-separated list of components to import
//   --context     Import only specific context (segment) name
//   --dry-run     Validate data without importing
//   --verbose     Enable verbose logging
```

### 2.3 Component Support Matrix

| Component | Required | Description | Priority |
|-----------|----------|-------------|----------|
| `lidar_box` | ✅ Yes | 3D bounding box labels | P0 |
| `lidar_calibration` | ✅ Yes | Sensor extrinsics | P0 |
| `vehicle_pose` | ✅ Yes | Vehicle-to-world transforms | P0 |
| `lidar` | ⚪ Optional | Raw point clouds | P2 |
| `lidar_segmentation` | ⚪ Optional | Semantic segmentation | P2 |
| `camera_box` | ⚪ Optional | 2D box labels | P3 |
| `projected_lidar_box` | ⚪ Optional | LiDAR boxes projected to cameras | P3 |

### 2.4 Data Validation

**Validation Rules:**
1. Bounding box dimensions must be positive
2. Heading must be in [-π, π]
3. Object types must be known enums
4. Tracking IDs must be non-empty for tracked objects
5. Timestamps must be monotonically increasing within a context

---

## Phase 3: No Label Zone (NLZ) Support

### Objective
Implement NLZ polygon handling and point annotation.

### 3.1 NoLabelZone Type

```go
// NoLabelZone represents an unlabeled area in a scene.
// Polygons are in the global (world) frame.
type NoLabelZone struct {
    ZoneID  string        `json:"zone_id"`
    Polygon [][2]float64  `json:"polygon"` // List of [x, y] vertices
}

// ContainsPoint checks if a point is inside the NLZ polygon.
// Uses ray-casting algorithm for arbitrary (non-convex) polygons.
func (z *NoLabelZone) ContainsPoint(x, y float64) bool

// Bounds returns the axis-aligned bounding box of the polygon.
func (z *NoLabelZone) Bounds() (minX, minY, maxX, maxY float64)
```

### 3.2 Point NLZ Annotation

```go
// AnnotatePointsWithNLZ annotates each point with NLZ membership.
// Returns a boolean mask where true = point is in an NLZ.
func AnnotatePointsWithNLZ(points []WorldPoint, zones []NoLabelZone) []bool

// FilterNLZPoints returns points that are NOT in any NLZ.
func FilterNLZPoints(points []WorldPoint, nlzMask []bool) []WorldPoint
```

### 3.3 Prediction NLZ Overlap

```go
// CheckPredictionNLZOverlap checks if a prediction overlaps with NLZ points.
// This is required for Waymo metrics computation.
func CheckPredictionNLZOverlap(
    prediction BoundingBox7DOF,
    points []WorldPoint,
    nlzMask []bool,
) bool
```

---

## Phase 4: ML Training Integration

### Objective
Connect Waymo ground truth labels to the ML training pipeline.

### 4.1 Training Data Generator

**File:** `internal/lidar/waymo/training_generator.go`

```go
// WaymoTrainingGenerator creates training examples from Waymo data.
type WaymoTrainingGenerator struct {
    db          *sql.DB
    config      TrainingConfig
}

// TrainingExample represents a single ML training sample.
type TrainingExample struct {
    // Input features
    PointCloud    []WorldPoint      `json:"point_cloud"`
    Clusters      []WorldCluster    `json:"clusters"`
    
    // Ground truth labels
    Labels        []ObjectLabel     `json:"labels"`
    
    // Metadata
    ContextName   string            `json:"context_name"`
    FrameTimestamp int64            `json:"frame_timestamp"`
    
    // NLZ mask for points
    NLZMask       []bool            `json:"nlz_mask,omitempty"`
}

// GenerateExamples creates training examples from imported Waymo data.
func (g *WaymoTrainingGenerator) GenerateExamples(filter TrainingDataFilter) ([]TrainingExample, error)

// ExportTFRecord exports training examples in TFRecord format.
func (g *WaymoTrainingGenerator) ExportTFRecord(examples []TrainingExample, outputPath string) error

// ExportParquet exports training examples in Parquet format.
func (g *WaymoTrainingGenerator) ExportParquet(examples []TrainingExample, outputPath string) error
```

### 4.2 Label Association

```go
// AssociateClustersWithLabels matches detected clusters to ground truth labels.
// Uses IoU (Intersection over Union) for matching.
func AssociateClustersWithLabels(
    clusters []WorldCluster,
    labels []ObjectLabel,
    iouThreshold float64,
) []ClusterLabelAssociation

type ClusterLabelAssociation struct {
    ClusterID  int64
    LabelID    string
    IoU        float64
    IsMatch    bool  // IoU >= threshold
    IsFalsePos bool  // Cluster with no matching label
    IsFalseNeg bool  // Label with no matching cluster
}
```

### 4.3 Metrics Computation

```go
// WaymoMetrics computes Waymo-compatible detection metrics.
type WaymoMetrics struct {
    // Per-class metrics
    VehicleAP        float64 `json:"vehicle_ap"`
    PedestrianAP     float64 `json:"pedestrian_ap"`
    CyclistAP        float64 `json:"cyclist_ap"`
    SignAP           float64 `json:"sign_ap"`
    
    // Aggregate metrics
    MeanAP           float64 `json:"mean_ap"`
    
    // Tracking metrics (if applicable)
    MOTA             float64 `json:"mota"` // Multiple Object Tracking Accuracy
    MOTP             float64 `json:"motp"` // Multiple Object Tracking Precision
}

// ComputeMetrics computes detection metrics for a set of predictions vs labels.
func ComputeMetrics(predictions []BoundingBox7DOF, labels []ObjectLabel) WaymoMetrics
```

---

## Phase 5: Frame Analyzer Tool

### Objective
Create a command-line tool for analyzing LIDAR frames with Waymo-compatible output.

### 5.1 Frame Analyzer Command

**File:** `cmd/tools/frame-analyzer/main.go`

```go
// frame-analyzer analyzes LIDAR frames for ML training and evaluation.
//
// Usage:
//   frame-analyzer --input /path/to/pcap --output /path/to/analysis
//   frame-analyzer --waymo /path/to/parquet --output /path/to/analysis
//   frame-analyzer --compare pred.parquet gt.parquet
//
// Modes:
//   analyze   Process raw LIDAR data (PCAP or live)
//   evaluate  Compare predictions against Waymo ground truth
//   export    Export training data in various formats
//
// Flags:
//   --input        Input PCAP or LIDAR data source
//   --waymo        Waymo Parquet dataset path
//   --output       Output directory for analysis results
//   --format       Output format: json, parquet, tfrecord
//   --viz          Enable visualization output
//   --metrics      Compute detection/tracking metrics
```

### 5.2 Analysis Output Format

```json
{
  "metadata": {
    "analyzer_version": "1.0",
    "timestamp": "2025-12-16T00:00:00Z",
    "source_type": "waymo_parquet",
    "context_name": "segment_xxx"
  },
  "frames": [
    {
      "frame_index": 0,
      "timestamp_micros": 1234567890,
      "detections": [...],
      "ground_truth": [...],
      "metrics": {
        "precision": 0.95,
        "recall": 0.92,
        "iou_mean": 0.78
      }
    }
  ],
  "summary": {
    "total_frames": 100,
    "total_detections": 1234,
    "total_ground_truth": 1200,
    "mean_ap": 0.85
  }
}
```

---

## Tool Requirements Matrix

### Required Tools and Libraries

| Tool/Library | Purpose | Status | Priority |
|--------------|---------|--------|----------|
| **Parquet Go library** | Read Waymo Parquet files | 🆕 New | P0 - Required |
| **SQLite** | Store imported data | ✅ Exists | - |
| **7-DOF box math** | IoU, containment, corners | 🆕 New | P0 - Required |
| **Polygon containment** | NLZ point checking | 🆕 New | P1 - High |
| **waymo-import CLI** | Import Waymo data | 🆕 New | P0 - Required |
| **frame-analyzer CLI** | Analyze frames | 🆕 New | P1 - High |

### Optional Tools and Libraries

| Tool/Library | Purpose | Status | Priority |
|--------------|---------|--------|----------|
| **TFRecord writer** | Export TensorFlow format | ⚪ Optional | P2 - Medium |
| **Visualization** | Point cloud rendering | ⚪ Optional | P3 - Low |
| **waymo-open-dataset Python** | Reference implementation | ⚪ Optional | P2 - Medium |
| **Point cloud registration** | Refined pose alignment | ⚪ Optional | P3 - Low |
| **Semantic segmentation** | Per-point labels | ⚪ Optional | P3 - Low |

### Decision Matrix: Build vs External

| Capability | Recommendation | Rationale |
|------------|----------------|-----------|
| Parquet reading | Use existing Go library | Standard format, good Go support |
| 7-DOF box math | Build in Go | Simple math, no dependencies needed |
| NLZ polygon math | Build in Go | Simple ray-casting algorithm |
| TFRecord export | Use TensorFlow Go bindings | Standard ML format |
| Visualization | Defer to external tools | CloudCompare, Open3D exist |

---

## Implementation Timeline

### Phase 1: Core Data Structures (Week 1-2)
- [ ] Create `waymo_types.go` with BoundingBox7DOF, ObjectLabel, LabeledFrame
- [ ] Extend WorldCluster with heading angle
- [ ] Add database migrations for ground truth tables
- [ ] Unit tests for box math (IoU, containment)

### Phase 2: Parquet Ingestion (Week 2-3)
- [ ] Add Parquet library dependency
- [ ] Implement WaymoParquetReader
- [ ] Create waymo-import CLI tool
- [ ] Integration tests with sample Waymo data

### Phase 3: NLZ Support (Week 3-4)
- [ ] Implement NoLabelZone type and polygon math
- [ ] Add point NLZ annotation functions
- [ ] Update database schema for NLZ storage
- [ ] Tests for polygon containment edge cases

### Phase 4: ML Integration (Week 4-5)
- [ ] Implement cluster-to-label association
- [ ] Create WaymoTrainingGenerator
- [ ] Add metrics computation (AP, IoU)
- [ ] Export formats (JSON, Parquet)

### Phase 5: Frame Analyzer (Week 5-6)
- [ ] Create frame-analyzer CLI tool
- [ ] Implement analysis pipeline
- [ ] Add evaluation mode
- [ ] Documentation and examples

---

## Future Considerations

### Potential Extensions

1. **Camera Integration**: Waymo provides synchronized camera data
2. **Sensor Fusion**: Combine LIDAR and camera detections
3. **Domain Adaptation**: Transfer Waymo models to Hesai P40 sensor
4. **Active Learning**: Prioritize labeling based on model uncertainty

### Privacy Alignment

**Waymo Compatibility with velocity.report Privacy Principles:**

| Principle | Waymo Alignment |
|-----------|-----------------|
| No PII collection | ✅ Waymo data is anonymized |
| No license plates | ✅ Waymo redacts plates in camera data |
| Local storage | ✅ Data imported locally |
| User ownership | ✅ User downloads and owns copy |

### Performance Considerations

- **Large dataset**: Waymo has ~1000 segments with ~20s each at 10Hz = 200k frames
- **Storage**: Estimated 50GB+ for full Parquet import
- **Processing**: Batch processing recommended, not real-time

---

## Appendix A: Waymo SDK Reference

### Python SDK (reference, not required)

```python
# pip install waymo-open-dataset-tf-2-12-0

from waymo_open_dataset import v2
from waymo_open_dataset.v2 import perception

# Read lidar boxes
lidar_box_df = v2.dataframe_for_component(
    context_name='segment_xxx',
    component='lidar_box'
)

# Access 7-DOF box
for _, row in lidar_box_df.iterrows():
    box = v2.LiDARBoxComponent.from_dict(row)
    print(box.box.center.x, box.box.size.x, box.box.heading)
```

### Data Access

- **GCS Bucket**: `gs://waymo_open_dataset_v_2_0_0/`
- **Download**: Via `gsutil` or web interface
- **License**: Waymo Open Dataset License Agreement

---

## Appendix B: File Structure After Implementation

```
velocity.report/
├── internal/lidar/
│   ├── waymo/
│   │   ├── parquet_reader.go
│   │   ├── parquet_reader_test.go
│   │   ├── training_generator.go
│   │   └── training_generator_test.go
│   ├── waymo_types.go           # BoundingBox7DOF, ObjectLabel
│   ├── waymo_types_test.go
│   ├── nlz.go                   # NoLabelZone support
│   ├── nlz_test.go
│   └── docs/
│       └── waymo-integration-plan.md  # This document
├── cmd/tools/
│   ├── waymo-import/
│   │   └── main.go
│   └── frame-analyzer/
│       └── main.go
└── internal/db/migrations/
    ├── 000014_waymo_integration.up.sql
    └── 000014_waymo_integration.down.sql
```

---

**Document Status:** Planning Complete  
**Next Action:** Begin Phase 1 Implementation  
**Last Updated:** December 16, 2025
