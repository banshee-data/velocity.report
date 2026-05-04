# AV standard LIDAR integration plan

- **Status:** DEFERRED - AV Integration Only
- **Layers:** L4 Perception, L5 Tracks, L6 Objects
- **Version:** 1.1
- **Canonical:** [av-range-image-format-alignment.md](../lidar/architecture/av-range-image-format-alignment.md)

---

> **Format alignment, current state analysis, gap analysis, and AV dataset format (7-DOF, NLZ, Parquet schema):** see [av-range-image-format-alignment.md](../lidar/architecture/av-range-image-format-alignment.md).
>
> **Taxonomy mapping (SemanticKITTI, Waymo, Panoptic nuScenes → v.r classes):** see §11 of [classification-maths.md](../../data/maths/classification-maths.md). The north star is Waymo `CameraSegmentation.Type` (28 classes from `camera_segmentation.proto`); see §11.4 for the full provenance.

---

## Current velocity.report capabilities

### Existing data structures

#### WorldCluster (clustering.go)

**WorldCluster** fields:

| Field             | Type      | Description |
| ----------------- | --------- | ----------- |
| ClusterID         | `int64`   |             |
| SensorID          | `string`  |             |
| TSUnixNanos       | `int64`   |             |
| CentroidX         | `float32` | World frame |
| CentroidY         | `float32` |             |
| CentroidZ         | `float32` |             |
| BoundingBoxLength | `float32` | X extent    |
| BoundingBoxWidth  | `float32` | Y extent    |
| BoundingBoxHeight | `float32` | Z extent    |
| PointsCount       | `int`     |             |
| HeightP95         | `float32` |             |
| IntensityMean     | `float32` |             |
| ClusterDensity    | `float32` |             |
| AspectRatio       | `float32` |             |
| NoisePointsCount  | `int`     |             |

**Gaps vs AV Standard:**

- ❌ No heading/yaw angle
- ❌ No tracking ID at cluster level
- ❌ No object class (added at track level)
- ✅ Bounding box dimensions (length, width, height)
- ✅ World frame coordinates

#### TrackedObject (tracking.go)

**TrackedObject** fields:

| Field                | Type         | Description           |
| -------------------- | ------------ | --------------------- |
| TrackID              | `string`     |                       |
| SensorID             | `string`     |                       |
| State                | `TrackState` |                       |
| X, Y                 | `float32`    | World frame           |
| VX, VY               | `float32`    | Velocity              |
| BoundingBoxLengthAvg | `float32`    |                       |
| BoundingBoxWidthAvg  | `float32`    |                       |
| BoundingBoxHeightAvg | `float32`    |                       |
| ObjectClass          | `string`     | Classification result |
| ObjectConfidence     | `float32`    |                       |

**Gaps vs AV Standard:**

- ❌ No heading/yaw angle
- ❌ No NLZ annotation
- ✅ Tracking ID (TrackID)
- ✅ Object class
- ✅ Bounding box dimensions

#### ForegroundFrame (training_data.go)

**ForegroundFrame** fields:

| Field            | Type           | Description       |
| ---------------- | -------------- | ----------------- |
| SensorID         | `string`       |                   |
| TSUnixNanos      | `int64`        |                   |
| SequenceID       | `string`       |                   |
| ForegroundPoints | `[]PointPolar` | Polar coordinates |
| TotalPoints      | `int`          |                   |
| BackgroundPoints | `int`          |                   |

**Gaps vs AV Standard:**

- ❌ No 3D bounding box labels
- ❌ No object class labels per frame
- ❌ No tracking ID associations
- ❌ No NLZ annotations

### Current ML pipeline status

| Component                   | Status      | Description                                                                       |
| --------------------------- | ----------- | --------------------------------------------------------------------------------- |
| Background subtraction      | ✅ Complete | EMA grid-based                                                                    |
| DBSCAN clustering           | ✅ Complete | Spatial indexing                                                                  |
| Kalman tracking             | ✅ Complete | Multi-object tracking                                                             |
| Rule-based classification   | ✅ Complete | All P0 classes: car, truck, bus, pedestrian, cyclist, motorcyclist, bird, dynamic |
| P0 ObjectClass enum         | ✅ Complete | Proto enum (10 values), Go/Swift/TS converters                                    |
| Analysis Run Infrastructure | ✅ Complete | Versioned runs with params                                                        |
| Training data export        | ✅ Complete | Compact binary encoding                                                           |
| Track labelling UI (Swift)  | ✅ Complete | Seekable replay, all 9 classes user-assignable                                    |
| Track labelling UI (Web)    | ✅ Complete | Run browser with label/quality assignment                                         |
| VRLOG replay classification | ✅ Complete | On-the-fly re-classification of legacy recordings                                 |
| ML classifier training      | 📋 Planned  | Phase 4.1                                                                         |

---

## Data structure gap analysis

### Required additions for AV standard compatibility

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

### Coordinate frame alignment

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

## Phase 1: core data structure alignment

### Objective

Extend velocity.report data structures to support industry-standard 7-DOF bounding boxes and ground truth labels.

### 1.1 BoundingBox7DOF type

**File:** `internal/lidar/av_types.go` (new)

**BoundingBox7DOF** fields:

| Field   | Type      | Description                                            |
| ------- | --------- | ------------------------------------------------------ |
| CenterX | `float64` | Center position (meters)                               |
| CenterY | `float64` |                                                        |
| CenterZ | `float64` |                                                        |
| Length  | `float64` | Extent along local X                                   |
| Width   | `float64` | Extent along local Y                                   |
| Height  | `float64` | Extent along local Z                                   |
| Heading | `float64` | Rotation around Z-axis to align +X with object forward |

### 1.2 Object label type

**ObjectLabel** fields:

| Field               | Type              | Description                      |
| ------------------- | ----------------- | -------------------------------- |
| ObjectID            | `string`          | Globally unique tracking ID      |
| FrameID             | `string`          | Frame context                    |
| SensorID            | `string`          | Source sensor                    |
| TimestampMicros     | `int64`           | Timestamp                        |
| Box                 | `BoundingBox7DOF` | Bounding box (7-DOF)             |
| ObjectType          | `AVObjectClass`   | Fine-grained class               |
| ObjectCategory      | `AVCategory`      | High-level category              |
| DifficultyLevel     | `int`             | 1=easy, 2=moderate, 3=hard       |
| NumLidarPointsInBox | `int`             | LiDAR metadata                   |
| InNoLabelZone       | `bool`            |                                  |
| OcclusionLevel      | `OcclusionLevel`  | NONE, PARTIAL, HEAVY             |
| TruncationLevel     | `float32`         | 0.0-1.0 (portion outside FOV)    |
| IsShapeCompleted    | `bool`            | True if box was estimated        |
| CompletionMethod    | `string`          | "observed", "symmetric", "model" |
| CompletionScore     | `float32`         | Confidence in completed shape    |

### 1.3 LabeledFrame type

**LabeledFrame** fields:

| Field            | Type            | Description                               |
| ---------------- | --------------- | ----------------------------------------- |
| ContextName      | `string`        | Dataset segment name                      |
| FrameTimestamp   | `int64`         | Microseconds                              |
| SequenceIndex    | `int`           | Frame index in sequence                   |
| VehiclePose      | `[16]float64`   | 4x4 vehicle-to-world matrix               |
| Labels           | `[]ObjectLabel` | Ground truth labels                       |
| NoLabelZones     | `[]NoLabelZone` | No Label Zones (polygons in global frame) |
| NumPoints        | `int`           | Point cloud metadata (not the full cloud) |
| NumPointsReturn1 | `int`           |                                           |
| NumPointsReturn2 | `int`           |                                           |
| NumPointsInNLZ   | `int`           |                                           |

### 1.4 Extend worldCluster with heading

**File:** `internal/lidar/clustering.go`

Add the following fields to the existing `WorldCluster` struct:

| Field             | Type      | Description               |
| ----------------- | --------- | ------------------------- |
| Heading           | `float32` |                           |
| HeadingSource     | `string`  | "pca", "velocity", "none" |
| HeadingConfidence | `float32` |                           |

These fields extend the existing WorldCluster (defined in `clustering.go`) to support orientation information needed for AV standard compatibility.

**Heading Computation Methods:**

1. **PCA**: Principal Component Analysis of point distribution
2. **Velocity**: From track velocity (VX, VY)
3. **Motion History**: Direction of travel from track history

### 1.5 Database schema extensions

**File:** `internal/db/migrations/000014_av_integration.up.sql`

**lidar_frame_metadata** columns:

| Column                 | Type      | Notes                     |
| ---------------------- | --------- | ------------------------- |
| id                     | `INTEGER` | PRIMARY KEY AUTOINCREMENT |
| context_name           | `TEXT`    | NOT NULL                  |
| frame_timestamp_micros | `INTEGER` | NOT NULL                  |
| object_id              | `TEXT`    | NOT NULL                  |
| center_x               | `REAL`    | NOT NULL                  |
| center_y               | `REAL`    | NOT NULL                  |
| center_z               | `REAL`    | NOT NULL                  |
| length                 | `REAL`    | NOT NULL                  |
| width                  | `REAL`    | NOT NULL                  |
| height                 | `REAL`    | NOT NULL                  |
| heading                | `REAL`    | NOT NULL                  |
| object_type            | `INTEGER` | NOT NULL                  |
| difficulty_level       | `INTEGER` | DEFAULT 1                 |
| num_lidar_points       | `INTEGER` |                           |
| in_no_label_zone       | `INTEGER` | DEFAULT 0                 |
| import_run_id          | `TEXT`    |                           |
| imported_at            | `INTEGER` | DEFAULT (UNIXEPOCH())     |
| id                     | `INTEGER` | PRIMARY KEY AUTOINCREMENT |
| context_name           | `TEXT`    | NOT NULL                  |
| frame_timestamp_micros | `INTEGER` | NOT NULL                  |
| zone_index             | `INTEGER` | NOT NULL                  |
| polygon_vertices_json  | `TEXT`    | NOT NULL                  |
| import_run_id          | `TEXT`    |                           |
| imported_at            | `INTEGER` | DEFAULT (UNIXEPOCH())     |
| id                     | `INTEGER` | PRIMARY KEY AUTOINCREMENT |
| context_name           | `TEXT`    | NOT NULL                  |
| frame_timestamp_micros | `INTEGER` | NOT NULL                  |
| sequence_index         | `INTEGER` |                           |
| vehicle_pose_json      | `TEXT`    |                           |
| num_points_total       | `INTEGER` |                           |
| num_points_return1     | `INTEGER` |                           |
| num_points_return2     | `INTEGER` |                           |
| num_points_in_nlz      | `INTEGER` |                           |
| num_labeled_objects    | `INTEGER` |                           |
| import_run_id          | `TEXT`    |                           |
| imported_at            | `INTEGER` | DEFAULT (UNIXEPOCH())     |

- Index `idx_gt_labels_context` on `lidar_ground_truth_labels` (context_name)
- Index `idx_gt_labels_timestamp` on `lidar_ground_truth_labels` (frame_timestamp_micros)
- Index `idx_gt_labels_object` on `lidar_ground_truth_labels` (object_id)
- Index `idx_gt_labels_type` on `lidar_ground_truth_labels` (object_type)
- Index `idx_nlz_context` on `lidar_no_label_zones` (context_name)
- Index `idx_frame_meta_context` on `lidar_frame_metadata` (context_name)

---

## Phase 2: parquet ingestion pipeline

### Objective

Create a robust Parquet reader for AV standard dataset files.

### 2.1 Parquet reader (required)

**File:** `internal/lidar/av/parquet_reader.go`

**Dependencies:**

- `github.com/parquet-go/parquet-go` (Apache Arrow Parquet for Go)
- Alternative: `github.com/xitongsys/parquet-go`

// AVParquetReader reads AV standard dataset Parquet files.
**AVParquetReader** holds `basePath` and wraps the parquet-go library.

| Method              | Parameters         | Returns               |
| ------------------- | ------------------ | --------------------- |
| `ReadLidarBoxes`    | contextName string | []ObjectLabel, error  |
| `ReadFrameMetadata` | contextName string | []LabeledFrame, error |
| `ReadNoLabelZones`  | contextName string | []NoLabelZone, error  |
| `ListContexts`      | (none)             | []string, error       |

### 2.2 Import command (required)

**File:** `cmd/tools/av-import/main.go`

CLI tool that imports AV standard Parquet data into the velocity.report SQLite database. Flags: `--input` (path to AV Parquet dataset directory), `--db` (SQLite database path, default: sensor_data.db), `--components` (comma-separated list of components to import).
// --context Import only specific context (segment) name
// --dry-run Validate data without importing
// --verbose Enable verbose logging

### 2.3 Component support matrix

| Component            | Required    | Description                 | Priority |
| -------------------- | ----------- | --------------------------- | -------- |
| `lidar_box`          | ✅ Yes      | 3D bounding box labels      | P0       |
| `lidar_calibration`  | ✅ Yes      | Sensor extrinsics           | P0       |
| `vehicle_pose`       | ✅ Yes      | Vehicle-to-world transforms | P0       |
| `lidar`              | ⚪ Optional | Raw point clouds            | P2       |
| `lidar_segmentation` | ⚪ Optional | Semantic segmentation       | P2       |

### 2.4 Data validation

**Validation Rules:**

1. Bounding box dimensions must be positive
2. Heading must be in [-π, π]
3. Object types must be known enums
4. Tracking IDs must be non-empty for tracked objects
5. Timestamps must be monotonically increasing within a context

---

## Phase 3: no label zone (NLZ) support

### Objective

Implement NLZ polygon handling and point annotation.

### 3.1 NoLabelZone type

**NoLabelZone** fields:

| Field   | Type           | Description                               |
| ------- | -------------- | ----------------------------------------- |
| ZoneID  | `string`       | Polygons are in the global (world) frame. |
| Polygon | `[][2]float64` | List of [x, y] vertices                   |

### 3.2 Point NLZ annotation

| Method                  | Parameters                                 | Returns        |
| ----------------------- | ------------------------------------------ | -------------- |
| `AnnotatePointsWithNLZ` | `points []WorldPoint, zones []NoLabelZone` | `[]bool`       |
| `FilterNLZPoints`       | `points []WorldPoint, nlzMask []bool`      | `[]WorldPoint` |

### 3.3 Prediction NLZ overlap

- CheckPredictionNLZOverlap checks if a prediction overlaps with NLZ points.
- This is required for AV standard metrics computation.

**CheckPredictionNLZOverlap** algorithm:

---

## Phase 4: ML training integration

### Objective

Connect AV standard ground truth labels to the ML training pipeline.

### 4.1 Training data generator

**File:** `internal/lidar/av/training_generator.go`

**TrainingExample** fields:

| Field          | Type             | Description                                                          |
| -------------- | ---------------- | -------------------------------------------------------------------- |
| db             | `*sql.DB`        | AVTrainingGenerator creates training examples from AV standard data. |
| config         | `TrainingConfig` |                                                                      |
| PointCloud     | `[]WorldPoint`   | Input features                                                       |
| Clusters       | `[]WorldCluster` |                                                                      |
| Labels         | `[]ObjectLabel`  | Ground truth labels                                                  |
| ContextName    | `string`         | Metadata                                                             |
| FrameTimestamp | `int64`          |                                                                      |
| NLZMask        | `[]bool`         | NLZ mask for points                                                  |

### 4.2 Label association

**ClusterLabelAssociation** fields:

| Field      | Type      | Description                    |
| ---------- | --------- | ------------------------------ |
| ClusterID  | `int64`   |                                |
| LabelID    | `string`  |                                |
| IoU        | `float64` |                                |
| IsMatch    | `bool`    | IoU >= threshold               |
| IsFalsePos | `bool`    | Cluster with no matching label |
| IsFalseNeg | `bool`    | Label with no matching cluster |

### 4.3 Metrics computation

**AVMetrics** fields:

| Field        | Type      | Description                        |
| ------------ | --------- | ---------------------------------- |
| VehicleAP    | `float64` | Per-class metrics                  |
| PedestrianAP | `float64` |                                    |
| CyclistAP    | `float64` |                                    |
| SignAP       | `float64` |                                    |
| MeanAP       | `float64` | Aggregate metrics                  |
| MOTA         | `float64` | Multiple Object Tracking Accuracy  |
| MOTP         | `float64` | Multiple Object Tracking Precision |

---

## Phase 5: frame analyzer tool

### Objective

Create a command-line tool for analysing LIDAR frames with AV standard-compatible output.

### 5.1 Frame analyzer command

**File:** `cmd/tools/frame-analyzer/main.go`

// frame-analyzer analyzes LIDAR frames for ML training and evaluation.
//
// Usage:
// frame-analyzer --input /path/to/pcap --output /path/to/analysis
// frame-analyzer --dataset /path/to/parquet --output /path/to/analysis
// frame-analyzer --compare pred.parquet gt.parquet
//
// Modes:
// analyse Process raw LIDAR data (PCAP or live)
// evaluate Compare predictions against AV standard ground truth
// export Export training data in various formats
//
// Flags:
// --input Input PCAP or LIDAR data source
// --dataset AV standard Parquet dataset path
// --output Output directory for analysis results
// --format Output format: json, parquet, tfrecord
// --viz Enable visualisation output
// --metrics Compute detection/tracking metrics

### 5.2 Analysis output format

## JSON object with fields: `metadata`, `analyzer_version`, `timestamp`, `source_type`, `context_name`, `frames`, `frame_index`, `timestamp_micros`, `detections`, `ground_truth`, `metrics`, `precision`, `recall`, `iou_mean`, `summary`.

## Common types and helper functions

This section defines shared types and helper functions used across the clustering and occlusion handling algorithms.

### Cluster type

**Cluster** fields:

| Field    | Type           | Description                      |
| -------- | -------------- | -------------------------------- |
| Points   | `[]WorldPoint` | Points belonging to this cluster |
| Centroid | `[3]float64`   | Cluster centroid [x, y, z]       |
| ID       | `int64`        | Unique cluster identifier        |

### Helper functions

The following helper functions are used by the algorithms in Phases 6 and 7. These are implemented in the clustering and geometry packages.

- computeCentroidZ computes the mean Z coordinate of a set of points.

**computeCentroidZ** algorithm:

- computeHeightRange computes the vertical extent (max Z - min Z) of a point set.

**computeHeightRange** algorithm:

- variance computes the sample variance of a slice of float64 values.

**variance** algorithm:

- computeCompletionConfidence estimates confidence in a completed bounding box
- based on observation quality, prior match, and occlusion level.

**computeCompletionConfidence** algorithm:

- Base confidence from visibility
- Bonus for close match to prior dimensions
- Clamp to valid range

---

## Phase 6: clustering algorithms for consistent object detection

### Objective

Implement robust clustering algorithms that identify consistent point clusters representing real-world objects, with handling for partial observations (occlusion) and shape estimation for non-illuminated surfaces.

### 6.1 Multi-Stage clustering pipeline

**Pipeline Overview:**

Raw Points → Ground Removal → DBSCAN Clustering → Cluster Merging →
Shape Estimation → Temporal Association → 7-DOF Box Fitting
**Stage 1: Ground Removal (Prerequisite)**

Ground points must be filtered before clustering to avoid merging ground with objects.

**GroundRemover** fields:

| Field            | Type      | Description                   |
| ---------------- | --------- | ----------------------------- |
| Method           | `string`  | "ransac", "grid", "hybrid"    |
| GridResolution   | `float64` | meters (for grid method)      |
| HeightThreshold  | `float64` | meters above estimated ground |
| RANSACIterations | `int`     | iterations for plane fitting  |
| RANSACThreshold  | `float64` | inlier distance threshold     |

**Stage 2: DBSCAN with Adaptive Parameters**

Standard DBSCAN with distance-adaptive epsilon for better clustering at varying ranges.

**AdaptiveDBSCANParams** fields:

| Field       | Type      | Description                                          |
| ----------- | --------- | ---------------------------------------------------- |
| EpsBase     | `float64` | Base neighborhood radius at 10m (default: 0.5m)      |
| EpsPerMeter | `float64` | Additional radius per meter of range (default: 0.02) |
| MinPts      | `int`     | Minimum points per cluster (default: 10)             |
| MaxEps      | `float64` | Maximum epsilon cap (default: 2.0m)                  |

**Stage 3: Cluster Merging for Split Objects**

Large vehicles may be split into multiple clusters due to discontinuities. This stage merges clusters that likely belong to the same object.

**MergeCriteria** fields:

| Field              | Type      | Description                                                |
| ------------------ | --------- | ---------------------------------------------------------- |
| MaxMergeDistance   | `float64` | Max centroid distance for merge candidates (default: 3.0m) |
| MinOverlapRatio    | `float64` | Min bounding box overlap for merge (default: 0.1)          |
| VelocityTolerance  | `float64` | Max velocity difference for merge (default: 2.0 m/s)       |
| HeadingTolerance   | `float64` | Max heading difference for merge (default: 0.3 rad)        |
| SpatialProximity   | `bool`    | Centroids within MaxMergeDistance                          |
| TemporalCoherence  | `bool`    | Similar velocity and heading                               |
| ShapeCompatibility | `bool`    | Combined shape is plausible (aspect ratio, size)           |
| ConnectedByPoints  | `bool`    | Edge points are within eps distance                        |

### 6.2 Euclidean cluster extraction algorithm

**Algorithm: Region Growing with Spatial Index**

More efficient than DBSCAN for structured point clouds with known density patterns.

**EuclideanClusterExtractor** fields:

| Field            | Type           | Description                                       |
| ---------------- | -------------- | ------------------------------------------------- |
| ClusterTolerance | `float64`      | Distance threshold for neighbours (default: 0.5m) |
| MinClusterSize   | `int`          | Minimum points per cluster (default: 10)          |
| MaxClusterSize   | `int`          | Maximum points per cluster (default: 25000)       |
| spatialIndex     | `*OctreeIndex` | Spatial index for efficient neighbour queries     |

### 6.3 Octree spatial index

**Implementation for efficient 3D neighbour queries:**

**OctreeIndex** fields:

| Field       | Type             | Description                                          |
| ----------- | ---------------- | ---------------------------------------------------- |
| Center      | `[3]float64`     | Center of this node's bounding box                   |
| HalfSize    | `float64`        | Half the side length of the bounding box             |
| Children    | `[8]*OctreeNode` | Child nodes (nil if leaf)                            |
| Points      | `[]int`          | Point indices (only in leaf nodes)                   |
| IsLeaf      | `bool`           |                                                      |
| Root        | `*OctreeNode`    |                                                      |
| Points      | `[]WorldPoint`   |                                                      |
| MaxLeafSize | `int`            | Max points per leaf before subdivision (default: 32) |
| MaxDepth    | `int`            | Max tree depth (default: 10)                         |

---

## Phase 7: occlusion handling and shape completion

### Objective

Handle partial observations where portions of an object are not "illuminated" by the LIDAR (e.g., the far side of a car) and estimate the complete 3D bounding box.

### 7.1 Occlusion detection

**Problem:** A LIDAR sensor only observes surfaces facing toward it. For a typical vehicle, only 1-3 sides are visible at any time.

**Occlusion Classification:**

**OcclusionInfo** fields:

| Field           | Type         | Description                                                  |
| --------------- | ------------ | ------------------------------------------------------------ |
| SensorPosition  | `[3]float64` | Sensor location in world frame                               |
| FrontVisible    | `bool`       | Front face illuminated                                       |
| BackVisible     | `bool`       | Back face illuminated (rare)                                 |
| LeftVisible     | `bool`       | Left side illuminated                                        |
| RightVisible    | `bool`       | Right side illuminated                                       |
| TopVisible      | `bool`       | Top surface illuminated                                      |
| VisibleFraction | `float32`    | Estimated fraction of surface visible [0, 1]                 |
| OcclusionAngle  | `float32`    | Angle from sensor to object centroid                         |
| PointCoverage   | `[6]int`     | Point count per face (front, back, left, right, top, bottom) |

### 7.2 Shape completion algorithms

**Algorithm 1: Symmetry-Based Completion**

Most vehicles and many objects exhibit bilateral symmetry. Use visible points to estimate the hidden half.

**SymmetricCompletion** fields:

| Field              | Type      | Description                                                   |
| ------------------ | --------- | ------------------------------------------------------------- |
| MinVisibleFraction | `float32` | Minimum visible fraction to attempt completion (default: 0.3) |
| SymmetryAxis       | `int`     | 0=X (along heading), 1=Y (lateral)                            |

**Algorithm 2: Model-Based Completion (Class Priors)**

Use learned class-specific shape priors to complete boxes when symmetry is insufficient.

**ShapePrior** fields:

| Field          | Type                           | Description                                                 |
| -------------- | ------------------------------ | ----------------------------------------------------------- |
| ClassPriors    | `map[AVObjectClass]ShapePrior` | ModelBasedCompletion uses class priors for shape estimation |
| MeanLength     | `float32`                      | Mean dimensions (meters)                                    |
| MeanWidth      | `float32`                      |                                                             |
| MeanHeight     | `float32`                      |                                                             |
| StdLength      | `float32`                      | Standard deviations (for confidence estimation)             |
| StdWidth       | `float32`                      |                                                             |
| StdHeight      | `float32`                      |                                                             |
| MinAspectRatio | `float32`                      | length/width                                                |
| MaxAspectRatio | `float32`                      |                                                             |

### 7.3 Temporal consistency for shape refinement

Use multi-frame observations to refine shape estimates as an object moves and reveals different surfaces.

**ShapeEstimate** fields:

| Field                   | Type                        | Description                                       |
| ----------------------- | --------------------------- | ------------------------------------------------- |
| ShapeEstimates          | `map[string]*ShapeEstimate` | trackID -> estimate                               |
| DecayFactor             | `float32`                   | Weight decay for old observations (default: 0.9)  |
| MinObservations         | `int`                       | Min frames before confident estimate (default: 3) |
| LengthSum, LengthWeight | `float64`                   | Weighted running averages                         |
| WidthSum, WidthWeight   | `float64`                   |                                                   |
| HeightSum, HeightWeight | `float64`                   |                                                   |
| BestVisibility          | `float32`                   | Best observation (highest visibility)             |
| BestBox                 | `BoundingBox7DOF`           |                                                   |
| ObservationCount        | `int`                       | Observation history                               |
| SurfacesCovered         | `map[string]bool`           | Which surfaces have been observed                 |

### 7.4 L-Shape fitting for vehicles

Classic algorithm for estimating vehicle bounding boxes from corner observations.

**LShapeFitter** fields:

| Field            | Type      | Description                                          |
| ---------------- | --------- | ---------------------------------------------------- |
| MinPointsPerEdge | `int`     | Min points to detect an edge (default: 5)            |
| AngleTolerance   | `float64` | Tolerance for perpendicular edges (default: 0.1 rad) |
| EdgeSearchAngles | `int`     | Number of angles to search (default: 180)            |

---

## Tool requirements matrix

### Required tools and libraries

| Tool/Library            | Purpose                        | Status    | Priority      |
| ----------------------- | ------------------------------ | --------- | ------------- |
| **SQLite**              | Store imported data            | ✅ Exists | -             |
| **Parquet Go library**  | Read AV standard Parquet files | 🆕 New    | P0 - Required |
| **7-DOF box math**      | IoU, containment, corners      | 🆕 New    | P0 - Required |
| **av-import CLI**       | Import AV standard data        | 🆕 New    | P0 - Required |
| **Polygon containment** | NLZ point checking             | 🆕 New    | P1 - High     |
| **frame-analyzer CLI**  | Analyse frames                 | 🆕 New    | P1 - High     |

### Optional tools and libraries

| Tool/Library                 | Purpose                  | Status      | Priority    |
| ---------------------------- | ------------------------ | ----------- | ----------- |
| **TFRecord writer**          | Export TensorFlow format | ⚪ Optional | P2 - Medium |
| **AV dataset Python SDK**    | Reference implementation | ⚪ Optional | P2 - Medium |
| **Visualisation**            | Point cloud rendering    | ⚪ Optional | P3 - Low    |
| **Point cloud registration** | Refined pose alignment   | ⚪ Optional | P3 - Low    |
| **Semantic segmentation**    | Per-point labels         | ⚪ Optional | P3 - Low    |

### Decision matrix: build vs external

| Capability       | Recommendation             | Rationale                           |
| ---------------- | -------------------------- | ----------------------------------- |
| Parquet reading  | Use existing Go library    | Standard format, good Go support    |
| 7-DOF box math   | Build in Go                | Simple math, no dependencies needed |
| NLZ polygon math | Build in Go                | Simple ray-casting algorithm        |
| TFRecord export  | Use TensorFlow Go bindings | Standard ML format                  |
| Visualisation    | Defer to external tools    | CloudCompare, Open3D exist          |

---

## Implementation timeline

### Phase 1: core data structures (week 1-2)

- [ ] Create `av_types.go` with BoundingBox7DOF, ObjectLabel, LabeledFrame
- [ ] Implement AV industry standard object taxonomy (see [classification-maths.md §11](../../data/maths/classification-maths.md))
- [ ] Extend WorldCluster with heading angle
- [ ] Add database migrations for ground truth tables
- [ ] Unit tests for box math (IoU, containment)

### Phase 2: parquet ingestion (week 2-3)

- [ ] Add Parquet library dependency
- [ ] Implement AVParquetReader
- [ ] Create av-import CLI tool
- [ ] Integration tests with sample AV dataset

### Phase 3: NLZ support (week 3-4)

- [ ] Implement NoLabelZone type and polygon math
- [ ] Add point NLZ annotation functions
- [ ] Update database schema for NLZ storage
- [ ] Tests for polygon containment edge cases

### Phase 4: ML integration (week 4-5)

- [ ] Implement cluster-to-label association
- [ ] Create AVTrainingGenerator
- [ ] Add metrics computation (AP, IoU)
- [ ] Export formats (JSON, Parquet)

### Phase 5: frame analyzer (week 5-6)

- [ ] Create frame-analyzer CLI tool
- [ ] Implement analysis pipeline
- [ ] Add evaluation mode
- [ ] Documentation and examples

### Phase 6: clustering algorithms (week 6-7)

- [ ] Implement ground removal (RANSAC/grid-based)
- [ ] Implement AdaptiveDBSCAN with range-dependent epsilon
- [ ] Implement cluster merging for over-segmented objects
- [ ] Build octree spatial index for efficient 3D queries
- [ ] L-shape fitting for vehicle bounding boxes
- [ ] Unit tests for clustering algorithms

### Phase 7: occlusion handling (week 7-8)

- [ ] Implement occlusion detection (surface visibility analysis)
- [ ] Symmetry-based shape completion
- [ ] Model-based completion with class priors
- [ ] Temporal shape refinement across frames
- [ ] Integration with tracker for consistent shape estimates
- [ ] Validation against AV industry standard ground truth

---

## Future considerations

### Potential extensions

1. **Radar Integration**: Integrate radar detections with LiDAR for velocity validation
2. **Sensor Fusion**: Combine LiDAR and radar detections for improved tracking
3. **Domain Adaptation**: Transfer AV dataset models to Hesai P40 sensor
4. **Active Learning**: Prioritise labelling based on model uncertainty

### Privacy alignment

**AV Dataset Compatibility with velocity.report Privacy Principles:**

| Principle         | AV Dataset Alignment                    |
| ----------------- | --------------------------------------- |
| No PII collection | ✅ AV datasets are anonymized           |
| No license plates | ✅ LiDAR-only, no visual identification |
| Local storage     | ✅ Data imported locally                |
| User ownership    | ✅ User downloads and owns copy         |

### Performance considerations

- **Large dataset**: Typical AV datasets have ~1000 segments with ~20s each at 10Hz = 200k frames
- **Storage**: Estimated 50GB+ for full Parquet import
- **Processing**: Batch processing recommended, not real-time

---

## Appendix a: AV dataset SDK reference

### Python SDK (reference, not required)

Example workflow for common AV dataset formats: install the dataset SDK (e.g., `pip install av-dataset-tools`), then use `av_dataset.dataframe_for_component()` to read lidar boxes by context name and component. Each row yields a `LiDARBoxComponent` with 7-DOF box centre, size, and heading attributes.

### Data access

- **Storage**: Cloud storage or local download
- **Download**: Via appropriate dataset tools
- **License**: Check specific dataset license agreement

---

## Appendix b: file structure after implementation

velocity.report/
├── internal/lidar/
│ ├── av/
│ │ ├── parquet_reader.go
│ │ ├── parquet_reader_test.go
│ │ ├── training_generator.go
│ │ └── training_generator_test.go
│ ├── av_types.go # BoundingBox7DOF, ObjectLabel
│ ├── av_types_test.go
│ ├── nlz.go # NoLabelZone support
│ ├── nlz_test.go
│ └── docs/
│ └── av-lidar-integration-plan.md # This document
├── cmd/tools/
│ ├── av-import/
│ │ └── main.go
│ └── frame-analyzer/
│ └── main.go
└── internal/db/migrations/
├── 000014_av_integration.up.sql
└── 000014_av_integration.down.sql
