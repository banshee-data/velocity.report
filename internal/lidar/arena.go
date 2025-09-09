package lidar

import "time"

//
// 0) Frames & poses
//

// FrameID is a human-readable name like "sensor/hesai-01" or "site/main-st-001".
type FrameID string

// Pose is a rigid transform (sensor -> world) with versioning.
// T is 4x4 row-major (m00..m03, m10..m13, m20..m23, m30..m33).
type Pose struct {
	PoseID         int64
	SensorID       string
	FromFrame      FrameID // e.g., "sensor/hesai-01"
	ToFrame        FrameID // e.g., "site/main-st-001"
	T              [16]float64
	ValidFromNanos int64 // unix ns
	ValidToNanos   *int64
	Method         string // "tape+square", "plane-fit", etc.
	RMSEm          float32
}

// PoseCache holds the current pose used for realtime transforms.
type PoseCache struct {
	BySensorID map[string]*Pose
	WorldFrame FrameID // canonical site frame (e.g., "site/main-st-001")
}

//
// 1) Background subtractor (SENSOR FRAME)
//    - one grid per LiDAR (indexed by ring × azimuth bin)
//

type BackgroundParams struct {
	BackgroundUpdateFraction       float32 // e.g., 0.02
	ClosenessSensitivityMultiplier float32 // e.g., 3.0
	SafetyMarginMeters             float32 // e.g., 0.5
	FreezeDurationNanos            int64   // e.g., 5e9 (5s)
	NeighborConfirmationCount      int     // e.g., 5 of 8 neighbors
}

type BackgroundCell struct {
	AverageRangeMeters   float32
	RangeSpreadMeters    float32
	TimesSeenCount       uint32
	LastUpdateUnixNanos  int64
	FrozenUntilUnixNanos int64
}

type BackgroundGrid struct {
	SensorID    string
	SensorFrame FrameID // e.g., "sensor/hesai-01"

	Rings       int // e.g., 40
	AzimuthBins int // e.g., 1800 for 0.2°

	Cells []BackgroundCell // len = Rings * AzimuthBins

	Params BackgroundParams
	// Optional telemetry:
	ForegroundCount int64
	BackgroundCount int64
}

// Helper to index Cells: idx = ring*AzimuthBins + azBin
func (g *BackgroundGrid) Idx(ring, azBin int) int { return ring*g.AzimuthBins + azBin }

//
// 2) Foreground extraction result (WORLD FRAME)
//    - clusters are already transformed into world/site coordinates
//

// WorldCluster is a single-frame foreground region after BG subtraction and clustering.
type WorldCluster struct {
	ClusterID     int64
	SensorID      string
	WorldFrame    FrameID
	PoseID        int64 // which pose was applied
	TSUnixNanos   int64 // frame time (sensor->host corrected)
	CentroidX     float32
	CentroidY     float32
	CentroidZ     float32
	BBoxL         float32 // along motion axis or PCA axis
	BBoxW         float32
	BBoxH         float32
	PointsCount   int
	HeightP95     float32
	IntensityMean float32
	// (Optional) a few representative points for thumbnails/debug (world frame):
	SamplePoints [][3]float32
	// (Optional) minimal sensor-frame debug hooks:
	SensorRingHint  int     // dominant ring
	SensorAzDegHint float32 // approximate azimuth of centroid in sensor frame
}

// Simple, serializable summary that the sidecar can publish (and your Go app can persist).
type TrackSummary struct {
	TrackID    string
	SensorID   string
	WorldFrame FrameID
	PoseID     int64
	UnixNanos  int64

	// Kinematics (world frame; road-plane oriented)
	X, Y, VX, VY float32
	SpeedMps     float32
	HeadingRad   float32

	// Shape/quality
	BBoxL, BBoxW, BBoxH float32
	PointsCount         int
	HeightP95           float32
	IntensityMean       float32

	// Classification
	ClassLabel      string // "", "car", "ped", "bird", "other"
	ClassConfidence float32

	// Optional flattened covariance of [x y vx vy] row-major (4x4 = 16 vals)
	Cov4x4 []float32
}

//
// 3) Tracking (WORLD FRAME)
//

type TrackState2D struct {
	// State vector in world frame: [x y vx vy]
	X, Y   float32
	VX, VY float32
	// Row-major covariance (4x4). float32 saves RAM; use float64 if you prefer.
	Cov [16]float32
}

type Track struct {
	TrackID    string
	SensorID   string
	WorldFrame FrameID
	PoseID     int64

	FirstUnixNanos int64
	LastUnixNanos  int64

	State TrackState2D
	// Smoothed shape (running means)
	BBoxLAvg, BBoxWAvg, BBoxHAvg float32

	// Rollups for features/training
	ObsCount         int
	AvgSpeedMps      float32
	PeakSpeedMps     float32
	HeightP95Max     float32
	IntensityMeanAvg float32

	// Label/model
	ClassLabel      string
	ClassConfidence float32

	// Life-cycle
	Misses int // consecutive misses for deletion
}

type TrackObs struct {
	TrackID    string
	UnixNanos  int64
	WorldFrame FrameID
	PoseID     int64

	X, Y, Z    float32
	VX, VY, VZ float32
	SpeedMps   float32
	HeadingRad float32

	BBoxL, BBoxW, BBoxH float32
	HeightP95           float32
	IntensityMean       float32
}

//
// 4) Fusion hooks (association scaffolding; WORLD FRAME)
//    These are useful if you later do association inside the sidecar or
//    want to carry the result alongside TrackSummary.
//

// RadarPingWorld is a radar detection already transformed to world.
type RadarPingWorld struct {
	RadarObsID int64
	SensorID   string
	WorldFrame FrameID
	UnixNanos  int64
	X, Y       float32 // projected to road plane
	RadialMps  float32 // original radial speed (keep for reference)
	SNR        float32
}

// Association result at a specific time.
type Association struct {
	UnixNanos       int64
	TrackID         string
	RadarObsID      int64
	CostMahalanobis float32
	// Fused state if you do the update here (else leave zero and fuse in API):
	FusedX, FusedY   float32
	FusedVX, FusedVY float32
	FusedSpeedMps    float32
	FusedCov4x4      [16]float32
	SourceMask       uint8 // bit0=lidar, bit1=radar
}

//
// 5) Supervisory containers
//

type SidecarState struct {
	Poses   *PoseCache
	BG      map[string]*BackgroundGrid // by SensorID
	Tracks  map[string]*Track          // by TrackID
	LastObs map[string][]TrackObs      // by TrackID (ring buffer)
	LastClu map[string][]*WorldCluster // recent clusters for UI
}

//
// 6) Small helper to tag events
//

type Event struct {
	When    time.Time
	Level   string // "info","warn","error","debug"
	Message string
	Context map[string]any
}
