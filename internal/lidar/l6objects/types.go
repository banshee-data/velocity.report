package l6objects

import "time"

// Type definitions needed by classification, quality, and features modules.
// These are native definitions to avoid import cycles with the parent package.

// TrackState represents the lifecycle state of a track.
type TrackState string

const (
	TrackTentative TrackState = "tentative" // New track, needs confirmation
	TrackConfirmed TrackState = "confirmed" // Stable track with sufficient history
	TrackDeleted   TrackState = "deleted"   // Track marked for removal
)

// TrackPoint represents a single point in a track's history.
type TrackPoint struct {
	X         float32
	Y         float32
	Timestamp int64 // Unix nanos
}

// TrackedObject represents a single tracked object in the tracker.
// This is a subset of the fields from internal/lidar.TrackedObject
// containing only the fields needed by classification, quality, and features.
type TrackedObject struct {
	// Identity
	TrackID  string
	SensorID string
	State    TrackState

	// Lifecycle counters
	Hits   int // Consecutive successful associations
	Misses int // Consecutive missed associations

	// Timestamps
	FirstUnixNanos int64
	LastUnixNanos  int64

	// Kalman state (world frame): [x, y, vx, vy]
	X  float32 // Position X
	Y  float32 // Position Y
	VX float32 // Velocity X
	VY float32 // Velocity Y

	// Kalman covariance (4x4, row-major)
	P [16]float32

	// Aggregated features
	ObservationCount     int
	BoundingBoxLengthAvg float32
	BoundingBoxWidthAvg  float32
	BoundingBoxHeightAvg float32
	HeightP95Max         float32
	IntensityMeanAvg     float32
	AvgSpeedMps          float32
	PeakSpeedMps         float32

	// History of positions
	History []TrackPoint

	// Speed history for percentile computation.
	// This field is intentionally unexported to maintain encapsulation while
	// allowing access within the l6objects package for classification and features.
	speedHistory []float32

	// OBB heading (smoothed via exponential moving average)
	OBBHeadingRad float32 // Smoothed heading from oriented bounding box

	// Latest per-frame OBB dimensions (instantaneous, for real-time rendering)
	OBBLength float32 // Latest frame bounding box length (metres)
	OBBWidth  float32 // Latest frame bounding box width (metres)
	OBBHeight float32 // Latest frame bounding box height (metres)

	// Latest Z from the associated cluster OBB (ground-level, used for rendering)
	LatestZ float32

	// Classification
	ObjectClass         string  // Classification result: "pedestrian", "car", "bird", "other"
	ObjectConfidence    float32 // Classification confidence [0, 1]
	ClassificationModel string  // Model version used for classification

	// Track quality metrics
	TrackLengthMeters  float32 // Total distance traveled (meters)
	TrackDurationSecs  float32 // Total lifetime (seconds)
	OcclusionCount     int     // Number of missed frames (gaps)
	MaxOcclusionFrames int     // Longest gap in observations
	SpatialCoverage    float32 // % of bounding box covered by observations
	NoisePointRatio    float32 // Ratio of noise points to cluster points
}

// WorldPoint represents a point in Cartesian world coordinates (site frame).
type WorldPoint struct {
	X, Y, Z   float64   // World frame position (meters)
	Intensity uint8     // Laser return intensity
	Timestamp time.Time // Acquisition time
	SensorID  string    // Source sensor
}

// OrientedBoundingBox represents a 7-DOF (7 Degrees of Freedom) 3D bounding box.
// This format conforms to the AV industry standard specification.
type OrientedBoundingBox struct {
	CenterX    float32
	CenterY    float32
	CenterZ    float32
	Length     float32 // Extent along principal axis
	Width      float32 // Extent perpendicular to principal axis
	Height     float32 // Extent along Z
	HeadingRad float32 // Rotation around Z-axis
}

// WorldCluster represents a cluster of world points.
// This matches the schema lidar_clusters table structure.
type WorldCluster struct {
	ClusterID         int64   // matches lidar_cluster_id INTEGER PRIMARY KEY
	SensorID          string  // matches sensor_id TEXT NOT NULL
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
