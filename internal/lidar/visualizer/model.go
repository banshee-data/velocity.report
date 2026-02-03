// Package visualizer provides gRPC streaming of LiDAR perception data.
// This file defines the canonical internal model that drives all outputs.
package visualizer

import (
	"time"
)

// FrameBundle is the canonical internal model for a single frame of LiDAR data.
// Both the LidarView adapter and gRPC publisher consume this model.
type FrameBundle struct {
	// Metadata
	FrameID         uint64
	TimestampNanos  int64
	SensorID        string
	CoordinateFrame CoordinateFrameInfo

	// Point cloud (optional)
	PointCloud *PointCloudFrame

	// Perception outputs
	Clusters *ClusterSet
	Tracks   *TrackSet

	// Debug overlays (optional)
	Debug *DebugOverlaySet

	// Playback info (for replay)
	PlaybackInfo *PlaybackInfo
}

// CoordinateFrameInfo describes the coordinate frame of the data.
type CoordinateFrameInfo struct {
	FrameID        string  // e.g., "site/hesai-01"
	ReferenceFrame string  // e.g., "ENU" or "sensor"
	OriginLat      float64 // optional, for georeferencing
	OriginLon      float64
	OriginAlt      float64
	RotationDeg    float32 // rotation of X-axis from East
}

// PointCloudFrame contains point cloud data for a frame.
type PointCloudFrame struct {
	FrameID        uint64
	TimestampNanos int64
	SensorID       string

	// Point data (parallel arrays)
	X              []float32
	Y              []float32
	Z              []float32
	Intensity      []uint8
	Classification []uint8 // 0=background, 1=foreground, 2=ground

	// Decimation info
	DecimationMode  DecimationMode
	DecimationRatio float32
	PointCount      int
}

// DecimationMode specifies how points were decimated.
type DecimationMode int

const (
	DecimationNone           DecimationMode = 0
	DecimationUniform        DecimationMode = 1
	DecimationVoxel          DecimationMode = 2
	DecimationForegroundOnly DecimationMode = 3
)

// ClusterSet contains all clusters detected in a frame.
type ClusterSet struct {
	FrameID        uint64
	TimestampNanos int64
	Clusters       []Cluster
	Method         ClusteringMethod
}

// ClusteringMethod specifies the clustering algorithm used.
type ClusteringMethod int

const (
	ClusteringDBSCAN              ClusteringMethod = 0
	ClusteringConnectedComponents ClusteringMethod = 1
)

// Cluster represents a detected foreground object.
type Cluster struct {
	ClusterID      int64
	SensorID       string
	TimestampNanos int64

	// Centroid in world frame
	CentroidX float32
	CentroidY float32
	CentroidZ float32

	// Axis-aligned bounding box
	AABBLength float32
	AABBWidth  float32
	AABBHeight float32

	// Oriented bounding box (if computed)
	OBB *OrientedBoundingBox

	// Features
	PointsCount   int
	HeightP95     float32
	IntensityMean float32

	// Sample points for debug rendering
	SamplePoints []float32 // xyz interleaved
}

// OrientedBoundingBox represents a 7-DOF (7 Degrees of Freedom) 3D bounding box.
// This format conforms to the AV industry standard specification.
// See: docs/lidar/future/av-lidar-integration-plan.md for BoundingBox7DOF spec.
//
// 7-DOF parameters:
//   - CenterX/Y/Z: Centre position (metres, world frame)
//   - Length: Box extent along heading direction (metres)
//   - Width: Box extent perpendicular to heading (metres)
//   - Height: Box extent along Z-axis (metres)
//   - HeadingRad: Yaw angle around Z-axis (radians, [-π, π])
type OrientedBoundingBox struct {
	CenterX    float32 // metres, world frame
	CenterY    float32 // metres, world frame
	CenterZ    float32 // metres, world frame
	Length     float32 // metres, along heading direction
	Width      float32 // metres, perpendicular to heading
	Height     float32 // metres, Z extent
	HeadingRad float32 // radians, rotation around Z-axis, [-π, π]
}

// TrackSet contains all tracks for a frame.
type TrackSet struct {
	FrameID        uint64
	TimestampNanos int64
	Tracks         []Track
	Trails         []TrackTrail
}

// Track represents a tracked object.
type Track struct {
	TrackID  string
	SensorID string

	// Lifecycle
	State            TrackState
	Hits             int
	Misses           int
	ObservationCount int

	// Timestamps
	FirstSeenNanos int64
	LastSeenNanos  int64

	// Current position (world frame)
	X float32
	Y float32
	Z float32

	// Current velocity (world frame)
	VX float32
	VY float32
	VZ float32

	// Derived kinematics
	SpeedMps   float32
	HeadingRad float32

	// Uncertainty (optional, row-major 4x4)
	Covariance4x4 []float32

	// Bounding box (running average)
	BBoxLengthAvg  float32
	BBoxWidthAvg   float32
	BBoxHeightAvg  float32
	BBoxHeadingRad float32

	// Features
	HeightP95Max     float32
	IntensityMeanAvg float32
	AvgSpeedMps      float32
	PeakSpeedMps     float32

	// Classification
	ClassLabel      string
	ClassConfidence float32

	// Quality metrics
	TrackLengthMetres  float32
	TrackDurationSecs  float32
	OcclusionCount     int
	Confidence         float32
	OcclusionState     OcclusionState
	MotionModel        MotionModel
}

// TrackState represents the lifecycle state of a track.
type TrackState int

const (
	TrackStateUnknown   TrackState = 0
	TrackStateTentative TrackState = 1
	TrackStateConfirmed TrackState = 2
	TrackStateDeleted   TrackState = 3
)

// OcclusionState represents the occlusion state of a track.
type OcclusionState int

const (
	OcclusionNone    OcclusionState = 0
	OcclusionPartial OcclusionState = 1
	OcclusionFull    OcclusionState = 2
)

// MotionModel represents the motion model used for tracking.
type MotionModel int

const (
	MotionModelCV MotionModel = 0 // constant velocity
	MotionModelCA MotionModel = 1 // constant acceleration
)

// TrackTrail contains historical positions for trail rendering.
type TrackTrail struct {
	TrackID string
	Points  []TrackPoint
}

// TrackPoint is a single point in a track trail.
type TrackPoint struct {
	X              float32
	Y              float32
	TimestampNanos int64
}

// DebugOverlaySet contains debug artifacts for visualisation.
type DebugOverlaySet struct {
	FrameID        uint64
	TimestampNanos int64

	AssociationCandidates []AssociationCandidate
	GatingEllipses        []GatingEllipse
	Residuals             []InnovationResidual
	Predictions           []StatePrediction
}

// AssociationCandidate represents a cluster-track association candidate.
type AssociationCandidate struct {
	ClusterID int64
	TrackID   string
	Distance  float32 // Mahalanobis distance
	Accepted  bool
}

// GatingEllipse represents a Mahalanobis gating threshold.
type GatingEllipse struct {
	TrackID     string
	CenterX     float32
	CenterY     float32
	SemiMajor   float32
	SemiMinor   float32
	RotationRad float32
}

// InnovationResidual represents a Kalman filter innovation.
type InnovationResidual struct {
	TrackID           string
	PredictedX        float32
	PredictedY        float32
	MeasuredX         float32
	MeasuredY         float32
	ResidualMagnitude float32
}

// StatePrediction represents a predicted track state.
type StatePrediction struct {
	TrackID string
	X       float32
	Y       float32
	VX      float32
	VY      float32
}

// PlaybackInfo contains playback metadata for replay mode.
type PlaybackInfo struct {
	IsLive       bool
	LogStartNs   int64
	LogEndNs     int64
	PlaybackRate float32
	Paused       bool
}

// NewFrameBundle creates a new FrameBundle with the given metadata.
func NewFrameBundle(frameID uint64, sensorID string, timestamp time.Time) *FrameBundle {
	return &FrameBundle{
		FrameID:        frameID,
		TimestampNanos: timestamp.UnixNano(),
		SensorID:       sensorID,
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/" + sensorID,
			ReferenceFrame: "ENU",
		},
	}
}
