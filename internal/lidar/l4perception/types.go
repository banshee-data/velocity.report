package l4perception

import (
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// WorldPoint represents a point in Cartesian world coordinates (site frame).
// This is the canonical definition; internal/lidar aliases it for backward compatibility.
type WorldPoint struct {
	X, Y, Z   float64   // World frame position (meters)
	Intensity uint8     // Laser return intensity
	Timestamp time.Time // Acquisition time
	SensorID  string    // Source sensor
}

// FrameID is a human-readable name like "sensor/hesai-01" or "site/main-st-001".
type FrameID string

// OrientedBoundingBox represents a 7-DOF (7 Degrees of Freedom) 3D bounding box.
// This format conforms to the AV industry standard specification.
//
// 7-DOF parameters:
//   - CenterX/Y/Z: Centre position (metres, world frame)
//   - Length: Box extent along heading direction (metres)
//   - Width: Box extent perpendicular to heading (metres)
//   - Height: Box extent along Z-axis (metres)
//   - HeadingRad: Yaw angle around Z-axis (radians, [-π, π])
type OrientedBoundingBox struct {
	CenterX    float32
	CenterY    float32
	CenterZ    float32
	Length     float32 // Extent along principal axis
	Width      float32 // Extent perpendicular to principal axis
	Height     float32 // Extent along Z
	HeadingRad float32 // Rotation around Z-axis
}

// WorldCluster represents a detected object cluster in world coordinates.
type WorldCluster struct {
	ClusterID         int64   // matches lidar_cluster_id INTEGER PRIMARY KEY
	SensorID          string  // matches sensor_id TEXT NOT NULL
	WorldFrame        FrameID // matches world_frame TEXT NOT NULL
	TSUnixNanos       int64   // matches ts_unix_nanos INTEGER NOT NULL
	CentroidX         float32 // matches centroid_x REAL
	CentroidY         float32 // matches centroid_y REAL
	CentroidZ         float32 // matches centroid_z REAL
	BoundingBoxLength float32 // matches bounding_box_length REAL
	BoundingBoxWidth  float32 // matches bounding_box_width REAL
	BoundingBoxHeight float32 // matches bounding_box_height REAL
	PointsCount       int     // matches points_count INTEGER
	HeightP95         float32 // matches height_p95 REAL
	IntensityMean     float32 // matches intensity_mean REAL

	// Debug hints matching schema optional fields
	SensorRingHint  *int     // matches sensor_ring_hint INTEGER
	SensorAzDegHint *float32 // matches sensor_azimuth_deg_hint REAL

	// Optional in-memory only fields (not persisted to schema)
	SamplePoints [][3]float32         // for debugging/thumbnails
	OBB          *OrientedBoundingBox // Oriented bounding box (computed via PCA)
}

// PointPolar is a backward-compatible alias for the canonical definition in l2frames.
type PointPolar = l2frames.PointPolar

// Point is a backward-compatible alias for the canonical definition in l2frames.
type Point = l2frames.Point

// Pose represents a spatial transform between coordinate frames.
type Pose struct {
	PoseID                    int64       // matches pose_id INTEGER PRIMARY KEY
	SensorID                  string      // matches sensor_id TEXT NOT NULL
	FromFrame                 FrameID     // matches from_frame TEXT NOT NULL
	ToFrame                   FrameID     // matches to_frame TEXT NOT NULL
	T                         [16]float64 // matches T_rowmajor_4x4 BLOB (16 floats)
	ValidFromNanos            int64       // matches valid_from_ns INTEGER NOT NULL
	ValidToNanos              *int64      // matches valid_to_ns INTEGER (NULL = current)
	Method                    string      // matches method TEXT
	RootMeanSquareErrorMeters float32     // matches root_mean_square_error_meters REAL
}
