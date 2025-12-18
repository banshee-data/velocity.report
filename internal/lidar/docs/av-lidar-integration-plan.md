# AV Standard LIDAR Integration Plan

**Status:** Planning
**Date:** December 16, 2025
**Author:** Agent Ictinus (Product Architecture)
**Version:** 1.0

---

## Executive Summary

This document outlines a comprehensive plan to align velocity.report's LIDAR data structures with the industry-standard autonomous vehicle (AV) Perception format, enabling ingestion of AV dataset Parquet data for ML training. The goal is to create a robust LIDAR frame analyzer that can leverage world-class AV perception datasets for training object detection and classification models.

**Key Objective:** Ingest AV standard Perception LiDAR data (with 3D bounding box labels, tracking IDs, and No Label Zones) into velocity.report's planned LIDAR frame analyzer for ML training.

---

## Table of Contents

1. [AV Standard Dataset Overview](#av-standard-dataset-overview)
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

## AV Standard Dataset Overview

### AV Standard Perception Format - LiDAR Labels

The industry-standard AV perception format provides **3D bounding box labels** for LiDAR data with the following characteristics:

#### 7-DOF Bounding Box Format

Each labeled object is represented by a **7-Degree-of-Freedom (7-DOF) bounding box** in the **vehicle frame**:

| Parameter  | Description                   | Unit    | Notes           |
| ---------- | ----------------------------- | ------- | --------------- |
| `center_x` | Center X position             | meters  | Vehicle frame   |
| `center_y` | Center Y position             | meters  | Vehicle frame   |
| `center_z` | Center Z position             | meters  | Vehicle frame   |
| `length`   | Box extent along vehicle +X   | meters  | Forward axis    |
| `width`    | Box extent along vehicle +Y   | meters  | Left-right axis |
| `height`   | Box extent along vehicle +Z   | meters  | Up-down axis    |
| `heading`  | Yaw angle (rotation around Z) | radians | Range: [-π, π]  |

**Critical Properties:**

- **Zero pitch and zero roll**: Boxes are always parallel to the ground plane
- **Heading**: Angle to rotate vehicle frame +X to align with object's forward axis
- **Vehicle frame**: Not sensor frame - requires pose transformation

#### Labeled Object Classes (AV Industry Standard Compatible)

The velocity.report system aligns with **AV industry standard** labeling specifications, supporting the full 28 fine-grained semantic categories. Instance segmentation labels are provided for Vehicle, Pedestrian, and Cyclist classes, consistent across sensors and over time.

**Core Object Classes (Instance Segmented):**

| Class        | Fine-Grained Types                                         | Instance ID |
| ------------ | ---------------------------------------------------------- | ----------- |
| `VEHICLE`    | Car, Bus, Truck, Other Large Vehicle, Trailer, Ego Vehicle | Yes         |
| `PEDESTRIAN` | Pedestrian, Pedestrian Object                              | Yes         |
| `CYCLIST`    | Cyclist, Motorcyclist, Bicycle, Motorcycle                 | Yes         |

**28 Fine-Grained Semantic Categories:**

| Category ID | Category Name      | Instance Segmentation | Description                                   |
| ----------- | ------------------ | --------------------- | --------------------------------------------- |
| 1           | Car                | Yes                   | Passenger cars, SUVs, vans                    |
| 2           | Bus                | Yes                   | Buses and large passenger vehicles            |
| 3           | Truck              | Yes                   | Pickup trucks, box trucks, freight            |
| 4           | Other Large Vehicle| Yes                   | RVs, construction vehicles, farm equipment    |
| 5           | Trailer            | Yes                   | Attached trailers (separate from tow vehicle) |
| 6           | Ego Vehicle        | No                    | Self-reference (sensor platform)              |
| 7           | Motorcycle         | Yes                   | Motorcycles without rider visible             |
| 8           | Bicycle            | Yes                   | Bicycles without rider visible                |
| 9           | Pedestrian         | Yes                   | Walking, standing, or using mobility aids     |
| 10          | Cyclist            | Yes                   | Person actively riding a bicycle              |
| 11          | Motorcyclist       | Yes                   | Person actively riding a motorcycle           |
| 12          | Ground Animal      | No                    | Dogs, cats, deer, other ground animals        |
| 13          | Bird               | No                    | Flying or perched birds                       |
| 14          | Pole               | No                    | Utility poles, lamp posts, signposts          |
| 15          | Sign               | No                    | Traffic signs, street signs                   |
| 16          | Traffic Light      | No                    | Traffic signals                               |
| 17          | Construction Cone  | No                    | Traffic cones, barrels, barricades            |
| 18          | Pedestrian Object  | Yes                   | Strollers, wheelchairs, carts pushed by peds  |
| 19          | Building           | No                    | Structures, houses, commercial buildings      |
| 20          | Road               | No                    | Driveable road surface                        |
| 21          | Sidewalk           | No                    | Pedestrian walkways                           |
| 22          | Road Marker        | No                    | Painted road markings, crosswalks             |
| 23          | Lane Marker        | No                    | Lane dividers, center lines                   |
| 24          | Vegetation         | No                    | Trees, bushes, grass                          |
| 25          | Sky                | No                    | Sky region (camera only)                      |
| 26          | Ground             | No                    | Unlabeled ground surface                      |
| 27          | Static             | No                    | Other static objects not in above categories  |
| 28          | Dynamic            | No                    | Other dynamic objects not in above categories |

**velocity.report Implementation Priority:**

| Priority | Categories                                                    | Rationale                          |
| -------- | ------------------------------------------------------------- | ---------------------------------- |
| P0       | Car, Truck, Bus, Pedestrian, Cyclist, Motorcyclist            | Core traffic monitoring            |
| P1       | Bicycle, Motorcycle, Ground Animal, Bird                      | Safety-relevant moving objects     |
| P2       | Sign, Pole, Traffic Light, Construction Cone                  | Infrastructure detection           |
| P3       | Building, Road, Sidewalk, Vegetation                          | Scene understanding                |
| Deferred | Sky, Ground, Static, Dynamic, Ego Vehicle, Pedestrian Object  | Lower priority for roadside sensor |

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

### AV Standard Data Format

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

**Gaps vs AV Standard:**

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

**Gaps vs AV Standard:**

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

**Gaps vs AV Standard:**

- ❌ No 3D bounding box labels
- ❌ No object class labels per frame
- ❌ No tracking ID associations
- ❌ No NLZ annotations

### Current ML Pipeline Status

| Component                   | Status      | Description                  |
| --------------------------- | ----------- | ---------------------------- |
| Background subtraction      | ✅ Complete | EMA grid-based               |
| DBSCAN clustering           | ✅ Complete | Spatial indexing             |
| Kalman tracking             | ✅ Complete | Multi-object tracking        |
| Rule-based classification   | ✅ Complete | Pedestrian, car, bird, other |
| Analysis Run Infrastructure | ✅ Complete | Versioned runs with params   |
| Training data export        | ✅ Complete | Compact binary encoding      |
| Track labeling UI           | 📋 Planned  | Phase 4.0                    |
| ML classifier training      | 📋 Planned  | Phase 4.1                    |

---

## Data Structure Gap Analysis

### Required Additions for AV Standard Compatibility

| Gap                            | Priority      | Effort | Description                          |
| ------------------------------ | ------------- | ------ | ------------------------------------ |
| **Heading angle**              | P0 - Required | Low    | Add yaw angle to clusters and tracks |
| **7-DOF bounding box type**    | P0 - Required | Medium | New type matching AV standard format |
| **Ground truth labels**        | P0 - Required | Medium | Per-frame object labels with class   |
| **NLZ polygon support**        | P1 - High     | Medium | Polygon storage and point annotation |
| **Parquet reader**             | P0 - Required | High   | Read AV standard parquet files       |
| **Frame-to-vehicle transform** | P1 - High     | Medium | Coordinate frame handling            |
| **Global tracking ID**         | P1 - High     | Low    | AV standard-compatible track IDs     |
| **Object difficulty level**    | P2 - Medium   | Low    | AV standard difficulty annotation    |

### Coordinate Frame Alignment

**AV Standard Convention:**

- +X: Forward (vehicle direction)
- +Y: Left
- +Z: Up
- Labels in vehicle frame (requires pose transform for world frame)

**velocity.report Convention:**

- Sensor frame → World frame transformation via Pose
- Currently no heading angle computed

**Alignment Strategy:**

- Import AV labels in vehicle frame
- Store vehicle pose per frame
- Transform to world frame for analysis
- Compute heading from velocity or labeled annotation

---

## Phase 1: Core Data Structure Alignment

### Objective

Extend velocity.report data structures to support industry-standard 7-DOF bounding boxes and ground truth labels.

### 1.1 BoundingBox7DOF Type

**File:** `internal/lidar/av_types.go` (new)

```go
// BoundingBox7DOF represents a 7-DOF 3D bounding box in AV standard format.
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
// Matches AV industry standard LiDARBoxComponent structure.
type ObjectLabel struct {
    // Identity
    ObjectID   string `json:"object_id"`   // Globally unique tracking ID
    FrameID    string `json:"frame_id"`    // Frame context
    SensorID   string `json:"sensor_id"`   // Source sensor

    // Timestamp
    TimestampMicros int64 `json:"timestamp_micros"`

    // Bounding box (7-DOF)
    Box BoundingBox7DOF `json:"box"`

    // Classification (AV industry standard 28-class taxonomy)
    ObjectType      AVObjectClass `json:"object_type"`      // Fine-grained class
    ObjectCategory  AVCategory    `json:"object_category"`  // High-level category
    DifficultyLevel int              `json:"difficulty_level"` // 1=easy, 2=moderate, 3=hard

    // LiDAR metadata
    NumLidarPointsInBox int  `json:"num_lidar_points_in_box"`
    InNoLabelZone       bool `json:"in_no_label_zone"`
    
    // Occlusion and truncation (for training data quality)
    OcclusionLevel    OcclusionLevel `json:"occlusion_level"`    // NONE, PARTIAL, HEAVY
    TruncationLevel   float32        `json:"truncation_level"`   // 0.0-1.0 (portion outside FOV)
    
    // Shape completion metadata (see Phase 7: Occlusion Handling and Shape Completion)
    IsShapeCompleted  bool    `json:"is_shape_completed"`   // True if box was estimated
    CompletionMethod  string  `json:"completion_method"`    // "observed", "symmetric", "model"
    CompletionScore   float32 `json:"completion_score"`     // Confidence in completed shape
}

// AVObjectClass enum matching AV industry standard 28 fine-grained categories
type AVObjectClass int

const (
    // Instance-segmented classes (tracked across frames)
    AVTypeCar              AVObjectClass = 1
    AVTypeBus              AVObjectClass = 2
    AVTypeTruck            AVObjectClass = 3
    AVTypeOtherLargeVehicle AVObjectClass = 4
    AVTypeTrailer          AVObjectClass = 5
    AVTypeEgoVehicle       AVObjectClass = 6
    AVTypeMotorcycle       AVObjectClass = 7
    AVTypeBicycle          AVObjectClass = 8
    AVTypePedestrian       AVObjectClass = 9
    AVTypeCyclist          AVObjectClass = 10
    AVTypeMotorcyclist     AVObjectClass = 11
    
    // Non-instance classes (semantic only)
    AVTypeGroundAnimal     AVObjectClass = 12
    AVTypeBird             AVObjectClass = 13
    AVTypePole             AVObjectClass = 14
    AVTypeSign             AVObjectClass = 15
    AVTypeTrafficLight     AVObjectClass = 16
    AVTypeConstructionCone AVObjectClass = 17
    AVTypePedestrianObject AVObjectClass = 18
    AVTypeBuilding         AVObjectClass = 19
    AVTypeRoad             AVObjectClass = 20
    AVTypeSidewalk         AVObjectClass = 21
    AVTypeRoadMarker       AVObjectClass = 22
    AVTypeLaneMarker       AVObjectClass = 23
    AVTypeVegetation       AVObjectClass = 24
    AVTypeSky              AVObjectClass = 25
    AVTypeGround           AVObjectClass = 26
    AVTypeStatic           AVObjectClass = 27
    AVTypeDynamic          AVObjectClass = 28
    
    AVTypeUnknown          AVObjectClass = 0
)

// AVCategory represents high-level object categories for instance segmentation
type AVCategory int

const (
    AVCategoryUnknown    AVCategory = 0
    AVCategoryVehicle    AVCategory = 1  // Car, Bus, Truck, etc.
    AVCategoryPedestrian AVCategory = 2  // Pedestrian, Pedestrian Object
    AVCategoryCyclist    AVCategory = 3  // Cyclist, Motorcyclist
    AVCategorySign       AVCategory = 4  // Sign, Traffic Light
    AVCategoryAnimal     AVCategory = 5  // Ground Animal, Bird
    AVCategoryStatic     AVCategory = 6  // Infrastructure, vegetation
)

// OcclusionLevel indicates how much of an object is hidden from view
type OcclusionLevel int

const (
    OcclusionNone    OcclusionLevel = 0  // Fully visible (>90% of points expected)
    OcclusionPartial OcclusionLevel = 1  // Partially occluded (50-90% visible)
    OcclusionHeavy   OcclusionLevel = 2  // Heavily occluded (<50% visible)
)

// GetCategory returns the high-level category for a fine-grained object class
func (c AVObjectClass) GetCategory() AVCategory {
    switch c {
    case AVTypeCar, AVTypeBus, AVTypeTruck, AVTypeOtherLargeVehicle,
         AVTypeTrailer, AVTypeEgoVehicle, AVTypeMotorcycle, AVTypeBicycle:
        return AVCategoryVehicle
    case AVTypePedestrian, AVTypePedestrianObject:
        return AVCategoryPedestrian
    case AVTypeCyclist, AVTypeMotorcyclist:
        return AVCategoryCyclist
    case AVTypeSign, AVTypeTrafficLight:
        return AVCategorySign
    case AVTypeGroundAnimal, AVTypeBird:
        return AVCategoryAnimal
    case AVTypePole, AVTypeConstructionCone, AVTypeBuilding, AVTypeRoad,
         AVTypeSidewalk, AVTypeRoadMarker, AVTypeLaneMarker, AVTypeVegetation,
         AVTypeSky, AVTypeGround, AVTypeStatic:
        return AVCategoryStatic
    case AVTypeDynamic:
        return AVCategoryUnknown
    default:
        return AVCategoryUnknown
    }
}

// IsInstanceSegmented returns true if this class has instance-level tracking
func (c AVObjectClass) IsInstanceSegmented() bool {
    switch c {
    case AVTypeCar, AVTypeBus, AVTypeTruck, AVTypeOtherLargeVehicle,
         AVTypeTrailer, AVTypeMotorcycle, AVTypeBicycle,
         AVTypePedestrian, AVTypeCyclist, AVTypeMotorcyclist,
         AVTypePedestrianObject:
        return true
    default:
        return false
    }
}
```

### 1.3 LabeledFrame Type

```go
// LabeledFrame represents a single LIDAR frame with ground truth labels.
// This is the primary unit for ML training data.
type LabeledFrame struct {
    // Frame identity
    ContextName     string `json:"context_name"`     // Dataset segment name
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

These fields extend the existing WorldCluster (defined in `clustering.go`) to support orientation information needed for AV standard compatibility.

**Heading Computation Methods:**

1. **PCA**: Principal Component Analysis of point distribution
2. **Velocity**: From track velocity (VX, VY)
3. **Motion History**: Direction of travel from track history

### 1.5 Database Schema Extensions

**File:** `internal/db/migrations/000014_av_integration.up.sql`

```sql
-- Ground truth labels table (for AV dataset import)
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

Create a robust Parquet reader for AV standard dataset files.

### 2.1 Parquet Reader (Required)

**File:** `internal/lidar/av/parquet_reader.go`

**Dependencies:**

- `github.com/parquet-go/parquet-go` (Apache Arrow Parquet for Go)
- Alternative: `github.com/xitongsys/parquet-go`

```go
// AVParquetReader reads AV standard dataset Parquet files.
type AVParquetReader struct {
    basePath string
    // Internal reader from parquet-go library
}

// ReadLidarBoxes reads 3D bounding box labels from lidar_box component.
func (r *AVParquetReader) ReadLidarBoxes(contextName string) ([]ObjectLabel, error)

// ReadFrameMetadata reads frame-level metadata including poses.
func (r *AVParquetReader) ReadFrameMetadata(contextName string) ([]LabeledFrame, error)

// ReadNoLabelZones reads NLZ polygons for a context.
func (r *AVParquetReader) ReadNoLabelZones(contextName string) ([]NoLabelZone, error)

// ListContexts lists all available segment contexts.
func (r *AVParquetReader) ListContexts() ([]string, error)
```

### 2.2 Import Command (Required)

**File:** `cmd/tools/av-import/main.go`

```go
// av-import imports AV standard Parquet data into velocity.report database.
//
// Usage:
//   av-import --input /path/to/av/parquet --db sensor_data.db
//   av-import --input /path/to/av/parquet --components lidar_box,lidar_calibration
//
// Flags:
//   --input       Path to AV standard Parquet dataset directory
//   --db          Path to SQLite database (default: sensor_data.db)
//   --components  Comma-separated list of components to import
//   --context     Import only specific context (segment) name
//   --dry-run     Validate data without importing
//   --verbose     Enable verbose logging
```

### 2.3 Component Support Matrix

| Component            | Required    | Description                 | Priority |
| -------------------- | ----------- | --------------------------- | -------- |
| `lidar_box`          | ✅ Yes      | 3D bounding box labels      | P0       |
| `lidar_calibration`  | ✅ Yes      | Sensor extrinsics           | P0       |
| `vehicle_pose`       | ✅ Yes      | Vehicle-to-world transforms | P0       |
| `lidar`              | ⚪ Optional | Raw point clouds            | P2       |
| `lidar_segmentation` | ⚪ Optional | Semantic segmentation       | P2       |

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
// This is required for AV standard metrics computation.
func CheckPredictionNLZOverlap(
    prediction BoundingBox7DOF,
    points []WorldPoint,
    nlzMask []bool,
) bool
```

---

## Phase 4: ML Training Integration

### Objective

Connect AV standard ground truth labels to the ML training pipeline.

### 4.1 Training Data Generator

**File:** `internal/lidar/av/training_generator.go`

```go
// AVTrainingGenerator creates training examples from AV standard data.
type AVTrainingGenerator struct {
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

// GenerateExamples creates training examples from imported AV standard data.
func (g *AVTrainingGenerator) GenerateExamples(filter TrainingDataFilter) ([]TrainingExample, error)

// ExportTFRecord exports training examples in TFRecord format.
func (g *AVTrainingGenerator) ExportTFRecord(examples []TrainingExample, outputPath string) error

// ExportParquet exports training examples in Parquet format.
func (g *AVTrainingGenerator) ExportParquet(examples []TrainingExample, outputPath string) error
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
// AVMetrics computes AV standard detection metrics.
type AVMetrics struct{
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
func ComputeMetrics(predictions []BoundingBox7DOF, labels []ObjectLabel) AVMetrics
```

---

## Phase 5: Frame Analyzer Tool

### Objective

Create a command-line tool for analyzing LIDAR frames with AV standard-compatible output.

### 5.1 Frame Analyzer Command

**File:** `cmd/tools/frame-analyzer/main.go`

```go
// frame-analyzer analyzes LIDAR frames for ML training and evaluation.
//
// Usage:
//   frame-analyzer --input /path/to/pcap --output /path/to/analysis
//   frame-analyzer --dataset /path/to/parquet --output /path/to/analysis
//   frame-analyzer --compare pred.parquet gt.parquet
//
// Modes:
//   analyze   Process raw LIDAR data (PCAP or live)
//   evaluate  Compare predictions against AV standard ground truth
//   export    Export training data in various formats
//
// Flags:
//   --input        Input PCAP or LIDAR data source
//   --dataset      AV standard Parquet dataset path
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
    "source_type": "av_standard_parquet",
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

## Common Types and Helper Functions

This section defines shared types and helper functions used across the clustering and occlusion handling algorithms.

### Cluster Type

```go
// Cluster represents a logical grouping of LIDAR points into a candidate object.
// This type is used as input/output for clustering algorithms.
// NOTE: In production code, Cluster is defined in internal/lidar/clustering.go.
type Cluster struct {
    Points   []WorldPoint  // Points belonging to this cluster
    Centroid [3]float64    // Cluster centroid [x, y, z]
    ID       int64         // Unique cluster identifier
}
```

### Helper Functions

The following helper functions are used by the algorithms in Phases 6 and 7. These are implemented in the clustering and geometry packages.

```go
// computeCentroidZ computes the mean Z coordinate of a set of points.
func computeCentroidZ(points []WorldPoint) float64 {
    if len(points) == 0 {
        return 0
    }
    var sum float64
    for _, p := range points {
        sum += p.Z
    }
    return sum / float64(len(points))
}

// computeHeightRange computes the vertical extent (max Z - min Z) of a point set.
func computeHeightRange(points []WorldPoint) float64 {
    if len(points) == 0 {
        return 0
    }
    minZ, maxZ := points[0].Z, points[0].Z
    for _, p := range points {
        if p.Z < minZ {
            minZ = p.Z
        }
        if p.Z > maxZ {
            maxZ = p.Z
        }
    }
    return maxZ - minZ
}

// variance computes the sample variance of a slice of float64 values.
func variance(values []float64) float64 {
    if len(values) == 0 {
        return 0
    }
    var sum, sumSq float64
    n := float64(len(values))
    for _, v := range values {
        sum += v
        sumSq += v * v
    }
    mean := sum / n
    return (sumSq / n) - (mean * mean)
}

// computeCompletionConfidence estimates confidence in a completed bounding box
// based on observation quality, prior match, and occlusion level.
func computeCompletionConfidence(
    observed, completed BoundingBox7DOF,
    prior ShapePrior,
    occlusion OcclusionInfo,
) float32 {
    // Base confidence from visibility
    confidence := occlusion.VisibleFraction
    
    // Bonus for close match to prior dimensions
    lengthDiff := math.Abs(float64(completed.Length) - float64(prior.MeanLength))
    widthDiff := math.Abs(float64(completed.Width) - float64(prior.MeanWidth))
    
    if lengthDiff < float64(prior.StdLength) && widthDiff < float64(prior.StdWidth) {
        confidence += 0.1 // Good prior match bonus
    }
    
    // Clamp to valid range
    if confidence > 1.0 {
        confidence = 1.0
    }
    
    return confidence
}
```

---

## Phase 6: Clustering Algorithms for Consistent Object Detection

### Objective

Implement robust clustering algorithms that identify consistent point clusters representing real-world objects, with handling for partial observations (occlusion) and shape estimation for non-illuminated surfaces.

### 6.1 Multi-Stage Clustering Pipeline

**Pipeline Overview:**

```
Raw Points → Ground Removal → DBSCAN Clustering → Cluster Merging → 
Shape Estimation → Temporal Association → 7-DOF Box Fitting
```

**Stage 1: Ground Removal (Prerequisite)**

Ground points must be filtered before clustering to avoid merging ground with objects.

```go
// GroundRemoval uses RANSAC plane fitting or grid-based height filtering
type GroundRemover struct {
    Method            string  // "ransac", "grid", "hybrid"
    GridResolution    float64 // meters (for grid method)
    HeightThreshold   float64 // meters above estimated ground
    RANSACIterations  int     // iterations for plane fitting
    RANSACThreshold   float64 // inlier distance threshold
}

func (gr *GroundRemover) RemoveGround(points []WorldPoint) (nonGround, ground []WorldPoint)
```

**Stage 2: DBSCAN with Adaptive Parameters**

Standard DBSCAN with distance-adaptive epsilon for better clustering at varying ranges.

```go
// AdaptiveDBSCAN adjusts epsilon based on distance from sensor
type AdaptiveDBSCANParams struct {
    EpsBase     float64 // Base neighborhood radius at 10m (default: 0.5m)
    EpsPerMeter float64 // Additional radius per meter of range (default: 0.02)
    MinPts      int     // Minimum points per cluster (default: 10)
    MaxEps      float64 // Maximum epsilon cap (default: 2.0m)
}

func AdaptiveDBSCAN(points []WorldPoint, params AdaptiveDBSCANParams) []Cluster {
    // For each point, compute range-adaptive epsilon
    // eps(r) = min(EpsBase + EpsPerMeter * r, MaxEps)
    // where r = sqrt(x² + y² + z²)
}
```

**Stage 3: Cluster Merging for Split Objects**

Large vehicles may be split into multiple clusters due to discontinuities. This stage merges clusters that likely belong to the same object.

```go
// ClusterMerger identifies and merges over-segmented clusters
type ClusterMerger struct {
    MaxMergeDistance    float64 // Max centroid distance for merge candidates (default: 3.0m)
    MinOverlapRatio     float64 // Min bounding box overlap for merge (default: 0.1)
    VelocityTolerance   float64 // Max velocity difference for merge (default: 2.0 m/s)
    HeadingTolerance    float64 // Max heading difference for merge (default: 0.3 rad)
}

// MergeCriteria determines if two clusters should be merged
type MergeCriteria struct {
    SpatialProximity    bool    // Centroids within MaxMergeDistance
    TemporalCoherence   bool    // Similar velocity and heading
    ShapeCompatibility  bool    // Combined shape is plausible (aspect ratio, size)
    ConnectedByPoints   bool    // Edge points are within eps distance
}

func (cm *ClusterMerger) MergeClusters(clusters []Cluster) []Cluster
```

### 6.2 Euclidean Cluster Extraction Algorithm

**Algorithm: Region Growing with Spatial Index**

More efficient than DBSCAN for structured point clouds with known density patterns.

```go
// EuclideanClusterExtractor performs region-growing clustering
type EuclideanClusterExtractor struct {
    ClusterTolerance float64 // Distance threshold for neighbors (default: 0.5m)
    MinClusterSize   int     // Minimum points per cluster (default: 10)
    MaxClusterSize   int     // Maximum points per cluster (default: 25000)
    
    // Spatial index for efficient neighbor queries
    spatialIndex *OctreeIndex
}

func (ece *EuclideanClusterExtractor) Extract(points []WorldPoint) []Cluster {
    // 1. Build octree spatial index
    ece.spatialIndex = BuildOctree(points, ece.ClusterTolerance)
    
    // 2. Region growing from unprocessed seed points
    clusters := []Cluster{}
    processed := make([]bool, len(points))
    
    for i := range points {
        if processed[i] {
            continue
        }
        
        // Grow region from seed point
        cluster := ece.growRegion(points, i, processed)
        
        if len(cluster) >= ece.MinClusterSize && len(cluster) <= ece.MaxClusterSize {
            clusters = append(clusters, cluster)
        }
    }
    
    return clusters
}

func (ece *EuclideanClusterExtractor) growRegion(points []WorldPoint, seed int, processed []bool) []int {
    queue := []int{seed}
    region := []int{}
    
    for len(queue) > 0 {
        idx := queue[0]
        queue = queue[1:]
        
        if processed[idx] {
            continue
        }
        processed[idx] = true
        region = append(region, idx)
        
        // Find neighbors within tolerance
        neighbors := ece.spatialIndex.RadiusSearch(points[idx], ece.ClusterTolerance)
        queue = append(queue, neighbors...)
    }
    
    return region
}
```

### 6.3 Octree Spatial Index

**Implementation for efficient 3D neighbor queries:**

```go
// OctreeNode represents a node in the octree spatial index
type OctreeNode struct {
    Center      [3]float64    // Center of this node's bounding box
    HalfSize    float64       // Half the side length of the bounding box
    Children    [8]*OctreeNode // Child nodes (nil if leaf)
    Points      []int         // Point indices (only in leaf nodes)
    IsLeaf      bool
}

// OctreeIndex provides O(log n) neighbor queries for 3D point clouds
type OctreeIndex struct {
    Root         *OctreeNode
    Points       []WorldPoint
    MaxLeafSize  int     // Max points per leaf before subdivision (default: 32)
    MaxDepth     int     // Max tree depth (default: 10)
}

func BuildOctree(points []WorldPoint, resolution float64) *OctreeIndex {
    // 1. Compute bounding box of all points
    // 2. Create root node covering bounding box
    // 3. Recursively subdivide until MaxLeafSize or MaxDepth reached
}

func (oi *OctreeIndex) RadiusSearch(query WorldPoint, radius float64) []int {
    // 1. Start at root, check if node bounding box intersects query sphere
    // 2. If leaf, check each point against radius
    // 3. If internal, recurse into children that intersect query sphere
    // Time complexity: O(k log n) where k is number of neighbors
}
```

---

## Phase 7: Occlusion Handling and Shape Completion

### Objective

Handle partial observations where portions of an object are not "illuminated" by the LIDAR (e.g., the far side of a car) and estimate the complete 3D bounding box.

### 7.1 Occlusion Detection

**Problem:** A LIDAR sensor only observes surfaces facing toward it. For a typical vehicle, only 1-3 sides are visible at any time.

**Occlusion Classification:**

```go
// OcclusionAnalyzer determines which portions of an object are visible
type OcclusionAnalyzer struct {
    SensorPosition [3]float64 // Sensor location in world frame
}

// OcclusionInfo describes visibility of object surfaces
type OcclusionInfo struct {
    // Visible surfaces (relative to object heading)
    FrontVisible    bool    // Front face illuminated
    BackVisible     bool    // Back face illuminated (rare)
    LeftVisible     bool    // Left side illuminated
    RightVisible    bool    // Right side illuminated
    TopVisible      bool    // Top surface illuminated
    
    // Visibility metrics
    VisibleFraction float32 // Estimated fraction of surface visible [0, 1]
    OcclusionAngle  float32 // Angle from sensor to object centroid
    
    // Point distribution
    PointCoverage   [6]int  // Point count per face (front, back, left, right, top, bottom)
}

func (oa *OcclusionAnalyzer) AnalyzeOcclusion(cluster Cluster, heading float32) OcclusionInfo {
    // 1. Compute vector from sensor to cluster centroid
    toObjectX := cluster.Centroid[0] - oa.SensorPosition[0]
    toObjectY := cluster.Centroid[1] - oa.SensorPosition[1]
    
    // 2. Compute angle relative to object heading
    relativeAngle := float32(math.Atan2(float64(toObjectY), float64(toObjectX))) - heading
    
    // 3. Determine which faces are potentially visible
    // Front visible if |relativeAngle| < 90°
    // Left visible if relativeAngle in [0, 180°]
    // Right visible if relativeAngle in [-180°, 0]
    
    // 4. Analyze point distribution to confirm visibility
}
```

### 7.2 Shape Completion Algorithms

**Algorithm 1: Symmetry-Based Completion**

Most vehicles and many objects exhibit bilateral symmetry. Use visible points to estimate the hidden half.

```go
// SymmetricCompletion estimates full bounding box from partial observation
type SymmetricCompletion struct {
    MinVisibleFraction  float32 // Minimum visible fraction to attempt completion (default: 0.3)
    SymmetryAxis        int     // 0=X (along heading), 1=Y (lateral)
}

func (sc *SymmetricCompletion) CompleteBox(
    observedPoints []WorldPoint,
    observedBox BoundingBox7DOF,
    occlusion OcclusionInfo,
) (completedBox BoundingBox7DOF, confidence float32) {
    
    // Case 1: Only front/back visible (seeing length, need width)
    if (occlusion.FrontVisible || occlusion.BackVisible) && 
       !occlusion.LeftVisible && !occlusion.RightVisible {
        // Width is underestimated; double the observed half-width
        observedHalfWidth := observedBox.Width / 2
        completedBox.Width = observedHalfWidth * 2
        
        // Shift centroid to estimated true center
        // (centroid moves perpendicular to heading)
        confidence = 0.7 // Lower confidence for width estimation
    }
    
    // Case 2: Only side visible (seeing width, need length)
    if (occlusion.LeftVisible || occlusion.RightVisible) &&
       !occlusion.FrontVisible && !occlusion.BackVisible {
        // Length is underestimated; use class priors
        // (see Model-Based Completion below)
        confidence = 0.6
    }
    
    // Case 3: Corner view (front + side visible)
    if (occlusion.FrontVisible || occlusion.BackVisible) &&
       (occlusion.LeftVisible || occlusion.RightVisible) {
        // Good observation - use observed dimensions with minor correction
        completedBox = observedBox
        confidence = 0.85
    }
    
    return completedBox, confidence
}
```

**Algorithm 2: Model-Based Completion (Class Priors)**

Use learned class-specific shape priors to complete boxes when symmetry is insufficient.

```go
// ModelBasedCompletion uses class priors for shape estimation
type ModelBasedCompletion struct {
    ClassPriors map[AVObjectClass]ShapePrior
}

// ShapePrior encodes typical dimensions for an object class
type ShapePrior struct {
    // Mean dimensions (meters)
    MeanLength float32
    MeanWidth  float32
    MeanHeight float32
    
    // Standard deviations (for confidence estimation)
    StdLength  float32
    StdWidth   float32
    StdHeight  float32
    
    // Aspect ratio constraints
    MinAspectRatio float32 // length/width
    MaxAspectRatio float32
}

// DefaultClassPriors returns AV industry standard shape priors
func DefaultClassPriors() map[AVObjectClass]ShapePrior {
    return map[AVObjectClass]ShapePrior{
        AVTypeCar: {
            MeanLength: 4.5, MeanWidth: 1.8, MeanHeight: 1.5,
            StdLength: 0.5, StdWidth: 0.15, StdHeight: 0.2,
            MinAspectRatio: 1.8, MaxAspectRatio: 3.0,
        },
        AVTypeTruck: {
            MeanLength: 6.5, MeanWidth: 2.2, MeanHeight: 2.5,
            StdLength: 1.5, StdWidth: 0.3, StdHeight: 0.5,
            MinAspectRatio: 2.0, MaxAspectRatio: 4.0,
        },
        AVTypeBus: {
            MeanLength: 12.0, MeanWidth: 2.5, MeanHeight: 3.2,
            StdLength: 2.0, StdWidth: 0.2, StdHeight: 0.3,
            MinAspectRatio: 3.5, MaxAspectRatio: 6.0,
        },
        AVTypePedestrian: {
            MeanLength: 0.5, MeanWidth: 0.5, MeanHeight: 1.7,
            StdLength: 0.2, StdWidth: 0.2, StdHeight: 0.2,
            MinAspectRatio: 0.6, MaxAspectRatio: 1.5,
        },
        AVTypeCyclist: {
            MeanLength: 1.8, MeanWidth: 0.6, MeanHeight: 1.7,
            StdLength: 0.3, StdWidth: 0.1, StdHeight: 0.2,
            MinAspectRatio: 2.0, MaxAspectRatio: 4.0,
        },
        AVTypeBicycle: {
            MeanLength: 1.7, MeanWidth: 0.5, MeanHeight: 1.0,
            StdLength: 0.2, StdWidth: 0.1, StdHeight: 0.1,
            MinAspectRatio: 2.5, MaxAspectRatio: 4.5,
        },
        AVTypeMotorcycle: {
            MeanLength: 2.2, MeanWidth: 0.8, MeanHeight: 1.4,
            StdLength: 0.3, StdWidth: 0.15, StdHeight: 0.2,
            MinAspectRatio: 2.0, MaxAspectRatio: 3.5,
        },
    }
}

func (mbc *ModelBasedCompletion) CompleteBox(
    observedBox BoundingBox7DOF,
    objectClass AVObjectClass,
    occlusion OcclusionInfo,
) (completedBox BoundingBox7DOF, confidence float32) {
    
    prior, exists := mbc.ClassPriors[objectClass]
    if !exists {
        return observedBox, 0.3 // Low confidence fallback
    }
    
    completedBox = observedBox
    
    // Complete length if underobserved
    if occlusion.VisibleFraction < 0.5 {
        // Blend observed and prior based on visibility
        observedWeight := occlusion.VisibleFraction
        priorWeight := 1.0 - observedWeight
        
        completedBox.Length = float64(observedWeight)*observedBox.Length + 
                              float64(priorWeight)*float64(prior.MeanLength)
        completedBox.Width = float64(observedWeight)*observedBox.Width + 
                             float64(priorWeight)*float64(prior.MeanWidth)
    }
    
    // Enforce aspect ratio constraints
    aspectRatio := float32(completedBox.Length / completedBox.Width)
    if aspectRatio < prior.MinAspectRatio {
        completedBox.Length = completedBox.Width * float64(prior.MinAspectRatio)
    } else if aspectRatio > prior.MaxAspectRatio {
        completedBox.Width = completedBox.Length / float64(prior.MaxAspectRatio)
    }
    
    // Confidence based on observation quality and prior match
    confidence = computeCompletionConfidence(observedBox, completedBox, prior, occlusion)
    
    return completedBox, confidence
}
```

### 7.3 Temporal Consistency for Shape Refinement

Use multi-frame observations to refine shape estimates as an object moves and reveals different surfaces.

```go
// TemporalShapeRefiner maintains shape estimates across frames
type TemporalShapeRefiner struct {
    // Running estimates per tracked object
    ShapeEstimates map[string]*ShapeEstimate // trackID -> estimate
    
    // Configuration
    DecayFactor     float32 // Weight decay for old observations (default: 0.9)
    MinObservations int     // Min frames before confident estimate (default: 3)
}

// ShapeEstimate tracks accumulated shape evidence
type ShapeEstimate struct {
    // Weighted running averages
    LengthSum, LengthWeight float64
    WidthSum, WidthWeight   float64
    HeightSum, HeightWeight float64
    
    // Best observation (highest visibility)
    BestVisibility float32
    BestBox        BoundingBox7DOF
    
    // Observation history
    ObservationCount int
    SurfacesCovered  map[string]bool // Which surfaces have been observed
}

func (tsr *TemporalShapeRefiner) UpdateEstimate(
    trackID string,
    observedBox BoundingBox7DOF,
    occlusion OcclusionInfo,
    timestamp int64,
) BoundingBox7DOF {
    
    estimate, exists := tsr.ShapeEstimates[trackID]
    if !exists {
        estimate = &ShapeEstimate{
            SurfacesCovered: make(map[string]bool),
        }
        tsr.ShapeEstimates[trackID] = estimate
    }
    
    // Weight by visibility fraction
    weight := float64(occlusion.VisibleFraction)
    
    // Update running averages
    estimate.LengthSum += observedBox.Length * weight
    estimate.LengthWeight += weight
    estimate.WidthSum += observedBox.Width * weight
    estimate.WidthWeight += weight
    estimate.HeightSum += observedBox.Height * weight
    estimate.HeightWeight += weight
    
    // Track which surfaces have been observed
    if occlusion.FrontVisible { estimate.SurfacesCovered["front"] = true }
    if occlusion.BackVisible  { estimate.SurfacesCovered["back"] = true }
    if occlusion.LeftVisible  { estimate.SurfacesCovered["left"] = true }
    if occlusion.RightVisible { estimate.SurfacesCovered["right"] = true }
    
    estimate.ObservationCount++
    
    // Update best observation
    if occlusion.VisibleFraction > estimate.BestVisibility {
        estimate.BestVisibility = occlusion.VisibleFraction
        estimate.BestBox = observedBox
    }
    
    // Return refined estimate
    return tsr.computeRefinedBox(estimate)
}

func (tsr *TemporalShapeRefiner) computeRefinedBox(est *ShapeEstimate) BoundingBox7DOF {
    refined := BoundingBox7DOF{}
    
    if est.LengthWeight > 0 {
        refined.Length = est.LengthSum / est.LengthWeight
    }
    if est.WidthWeight > 0 {
        refined.Width = est.WidthSum / est.WidthWeight
    }
    if est.HeightWeight > 0 {
        refined.Height = est.HeightSum / est.HeightWeight
    }
    
    // Use best observation for center and heading
    refined.CenterX = est.BestBox.CenterX
    refined.CenterY = est.BestBox.CenterY
    refined.CenterZ = est.BestBox.CenterZ
    refined.Heading = est.BestBox.Heading
    
    return refined
}
```

### 7.4 L-Shape Fitting for Vehicles

Classic algorithm for estimating vehicle bounding boxes from corner observations.

```go
// LShapeFitter fits oriented bounding boxes to vehicle-like point patterns
type LShapeFitter struct {
    MinPointsPerEdge  int     // Min points to detect an edge (default: 5)
    AngleTolerance    float64 // Tolerance for perpendicular edges (default: 0.1 rad)
    EdgeSearchAngles  int     // Number of angles to search (default: 180)
}

func (lsf *LShapeFitter) FitLShape(points []WorldPoint) (BoundingBox7DOF, float32) {
    // 1. Search for optimal heading angle
    bestHeading := 0.0
    bestScore := 0.0
    
    for i := 0; i < lsf.EdgeSearchAngles; i++ {
        heading := float64(i) * math.Pi / float64(lsf.EdgeSearchAngles)
        score := lsf.evaluateHeading(points, heading)
        if score > bestScore {
            bestScore = score
            bestHeading = heading
        }
    }
    
    // 2. Project points onto best heading axes
    cos_h := math.Cos(bestHeading)
    sin_h := math.Sin(bestHeading)
    
    var minAlong, maxAlong, minPerp, maxPerp float64
    minAlong, minPerp = math.MaxFloat64, math.MaxFloat64
    maxAlong, maxPerp = -math.MaxFloat64, -math.MaxFloat64
    
    for _, p := range points {
        along := p.X*cos_h + p.Y*sin_h      // Along heading
        perp  := -p.X*sin_h + p.Y*cos_h     // Perpendicular
        
        minAlong = math.Min(minAlong, along)
        maxAlong = math.Max(maxAlong, along)
        minPerp = math.Min(minPerp, perp)
        maxPerp = math.Max(maxPerp, perp)
    }
    
    // 3. Construct bounding box
    length := maxAlong - minAlong
    width := maxPerp - minPerp
    centerAlong := (minAlong + maxAlong) / 2
    centerPerp := (minPerp + maxPerp) / 2
    
    box := BoundingBox7DOF{
        CenterX: centerAlong*cos_h - centerPerp*sin_h,
        CenterY: centerAlong*sin_h + centerPerp*cos_h,
        CenterZ: computeCentroidZ(points),
        Length:  length,
        Width:   width,
        Height:  computeHeightRange(points),
        Heading: bestHeading,
    }
    
    confidence := float32(bestScore)
    return box, confidence
}

func (lsf *LShapeFitter) evaluateHeading(points []WorldPoint, heading float64) float64 {
    // Score based on:
    // 1. Point distribution along edges (variance perpendicular to edges)
    // 2. L-shape detection (perpendicular edge detected)
    // 3. Edge point density
    
    cos_h := math.Cos(heading)
    sin_h := math.Sin(heading)
    
    // Project points
    var along, perp []float64
    for _, p := range points {
        along = append(along, p.X*cos_h + p.Y*sin_h)
        perp = append(perp, -p.X*sin_h + p.Y*cos_h)
    }
    
    // Compute variance ratio (good heading = low variance along edges)
    varAlong := variance(along)
    varPerp := variance(perp)
    
    // Score: higher variance ratio = better edge alignment
    if varAlong < 0.01 || varPerp < 0.01 {
        return 0
    }
    
    return math.Max(varAlong/varPerp, varPerp/varAlong)
}
```

---

## Tool Requirements Matrix

### Required Tools and Libraries

| Tool/Library            | Purpose                        | Status    | Priority      |
| ----------------------- | ------------------------------ | --------- | ------------- |
| **Parquet Go library**  | Read AV standard Parquet files | 🆕 New    | P0 - Required |
| **SQLite**              | Store imported data            | ✅ Exists | -             |
| **7-DOF box math**      | IoU, containment, corners      | 🆕 New    | P0 - Required |
| **Polygon containment** | NLZ point checking             | 🆕 New    | P1 - High     |
| **av-import CLI**       | Import AV standard data        | 🆕 New    | P0 - Required |
| **frame-analyzer CLI**  | Analyze frames                 | 🆕 New    | P1 - High     |

### Optional Tools and Libraries

| Tool/Library                 | Purpose                  | Status      | Priority    |
| ---------------------------- | ------------------------ | ----------- | ----------- |
| **TFRecord writer**          | Export TensorFlow format | ⚪ Optional | P2 - Medium |
| **Visualization**            | Point cloud rendering    | ⚪ Optional | P3 - Low    |
| **AV dataset Python SDK**    | Reference implementation | ⚪ Optional | P2 - Medium |
| **Point cloud registration** | Refined pose alignment   | ⚪ Optional | P3 - Low    |
| **Semantic segmentation**    | Per-point labels         | ⚪ Optional | P3 - Low    |

### Decision Matrix: Build vs External

| Capability       | Recommendation             | Rationale                           |
| ---------------- | -------------------------- | ----------------------------------- |
| Parquet reading  | Use existing Go library    | Standard format, good Go support    |
| 7-DOF box math   | Build in Go                | Simple math, no dependencies needed |
| NLZ polygon math | Build in Go                | Simple ray-casting algorithm        |
| TFRecord export  | Use TensorFlow Go bindings | Standard ML format                  |
| Visualization    | Defer to external tools    | CloudCompare, Open3D exist          |

---

## Implementation Timeline

### Phase 1: Core Data Structures (Week 1-2)

- [ ] Create `av_types.go` with BoundingBox7DOF, ObjectLabel, LabeledFrame
- [ ] Implement AV industry standard 28-class object taxonomy
- [ ] Extend WorldCluster with heading angle
- [ ] Add database migrations for ground truth tables
- [ ] Unit tests for box math (IoU, containment)

### Phase 2: Parquet Ingestion (Week 2-3)

- [ ] Add Parquet library dependency
- [ ] Implement AVParquetReader
- [ ] Create av-import CLI tool
- [ ] Integration tests with sample AV dataset

### Phase 3: NLZ Support (Week 3-4)

- [ ] Implement NoLabelZone type and polygon math
- [ ] Add point NLZ annotation functions
- [ ] Update database schema for NLZ storage
- [ ] Tests for polygon containment edge cases

### Phase 4: ML Integration (Week 4-5)

- [ ] Implement cluster-to-label association
- [ ] Create AVTrainingGenerator
- [ ] Add metrics computation (AP, IoU)
- [ ] Export formats (JSON, Parquet)

### Phase 5: Frame Analyzer (Week 5-6)

- [ ] Create frame-analyzer CLI tool
- [ ] Implement analysis pipeline
- [ ] Add evaluation mode
- [ ] Documentation and examples

### Phase 6: Clustering Algorithms (Week 6-7)

- [ ] Implement ground removal (RANSAC/grid-based)
- [ ] Implement AdaptiveDBSCAN with range-dependent epsilon
- [ ] Implement cluster merging for over-segmented objects
- [ ] Build octree spatial index for efficient 3D queries
- [ ] L-shape fitting for vehicle bounding boxes
- [ ] Unit tests for clustering algorithms

### Phase 7: Occlusion Handling (Week 7-8)

- [ ] Implement occlusion detection (surface visibility analysis)
- [ ] Symmetry-based shape completion
- [ ] Model-based completion with class priors
- [ ] Temporal shape refinement across frames
- [ ] Integration with tracker for consistent shape estimates
- [ ] Validation against AV industry standard ground truth

---

## Future Considerations

### Potential Extensions

1. **Radar Integration**: Integrate radar detections with LiDAR for velocity validation
2. **Sensor Fusion**: Combine LiDAR and radar detections for improved tracking
3. **Domain Adaptation**: Transfer AV dataset models to Hesai P40 sensor
4. **Active Learning**: Prioritize labeling based on model uncertainty

### Privacy Alignment

**AV Dataset Compatibility with velocity.report Privacy Principles:**

| Principle         | AV Dataset Alignment                    |
| ----------------- | --------------------------------------- |
| No PII collection | ✅ AV datasets are anonymized           |
| No license plates | ✅ LiDAR-only, no visual identification |
| Local storage     | ✅ Data imported locally                |
| User ownership    | ✅ User downloads and owns copy         |

### Performance Considerations

- **Large dataset**: Typical AV datasets have ~1000 segments with ~20s each at 10Hz = 200k frames
- **Storage**: Estimated 50GB+ for full Parquet import
- **Processing**: Batch processing recommended, not real-time

---

## Appendix A: AV Dataset SDK Reference

### Python SDK (reference, not required)

```python
# Example for common AV dataset formats
# Install appropriate dataset SDK (e.g., pip install av-dataset-tools)

import av_dataset
from av_dataset import perception

# Read lidar boxes
lidar_box_df = av_dataset.dataframe_for_component(
    context_name='segment_xxx',
    component='lidar_box'
)

# Access 7-DOF box
for _, row in lidar_box_df.iterrows():
    box = perception.LiDARBoxComponent.from_dict(row)
    print(box.box.center.x, box.box.size.x, box.box.heading)
```

### Data Access

- **Storage**: Cloud storage or local download
- **Download**: Via appropriate dataset tools
- **License**: Check specific dataset license agreement

---

## Appendix B: File Structure After Implementation

```
velocity.report/
├── internal/lidar/
│   ├── av/
│   │   ├── parquet_reader.go
│   │   ├── parquet_reader_test.go
│   │   ├── training_generator.go
│   │   └── training_generator_test.go
│   ├── av_types.go              # BoundingBox7DOF, ObjectLabel
│   ├── av_types_test.go
│   ├── nlz.go                   # NoLabelZone support
│   ├── nlz_test.go
│   └── docs/
│       └── av-lidar-integration-plan.md  # This document
├── cmd/tools/
│   ├── av-import/
│   │   └── main.go
│   └── frame-analyzer/
│       └── main.go
└── internal/db/migrations/
    ├── 000014_av_integration.up.sql
    └── 000014_av_integration.down.sql
```

---

**Document Status:** Planning Complete
**Next Action:** Begin Phase 1 Implementation
**Last Updated:** December 16, 2025
