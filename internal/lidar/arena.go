package lidar

import (
	"sync"
	"time"
)

//
// 0) Point - LiDAR measurement data structure
//

// Point represents a single 3D LiDAR measurement point in Cartesian coordinates
// Each point contains both the processed 3D coordinates and raw measurement data
type Point struct {
	// 3D Cartesian coordinates (computed from spherical measurements)
	X float64 `json:"x"` // X coordinate in meters (forward direction from sensor)
	Y float64 `json:"y"` // Y coordinate in meters (right direction from sensor)
	Z float64 `json:"z"` // Z coordinate in meters (upward direction from sensor)

	// Measurement metadata
	Intensity uint8     `json:"intensity"` // Laser return intensity/reflectivity (0-255)
	Distance  float64   `json:"distance"`  // Radial distance from sensor in meters
	Azimuth   float64   `json:"azimuth"`   // Horizontal angle in degrees (0-360, corrected)
	Elevation float64   `json:"elevation"` // Vertical angle in degrees (corrected for channel)
	Channel   int       `json:"channel"`   // Laser channel number (1-40)
	Timestamp time.Time `json:"timestamp"` // Point acquisition time (with firetime correction)
	BlockID   int       `json:"block_id"`  // Data block index within packet (0-9)
}

//
// 1) Frames & poses
//

// FrameID is a human-readable name like "sensor/hesai-01" or "site/main-st-001".
type FrameID string

// Pose is a rigid transform (sensor -> world) with versioning.
// T is 4x4 row-major (m00..m03, m10..m13, m20..m23, m30..m33).
// Updated to match schema.sql sensor_poses table exactly.
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

// PoseCache holds the current pose used for realtime transforms.
// Enhanced with thread-safety for concurrent access.
type PoseCache struct {
	BySensorID map[string]*Pose
	WorldFrame FrameID // canonical site frame (e.g., "site/main-st-001")
	// TODO: add mutex for thread-safe operations when implementing concurrent access
}

//
// 2) Background subtractor (SENSOR FRAME)
//    - one grid per LiDAR (indexed by ring × azimuth bin)
//    Enhanced for automatic persistence matching schema.sql
//

// BackgroundParams configuration matching the param storage approach in schema
type BackgroundParams struct {
	BackgroundUpdateFraction       float32 // e.g., 0.02
	ClosenessSensitivityMultiplier float32 // e.g., 3.0
	SafetyMarginMeters             float32 // e.g., 0.5
	FreezeDurationNanos            int64   // e.g., 5e9 (5s)
	NeighborConfirmationCount      int     // e.g., 5 of 8 neighbors

	// Additional params for persistence matching schema requirements
	SettlingPeriodNanos        int64 // 5 minutes before first snapshot
	SnapshotIntervalNanos      int64 // 2 hours between snapshots
	ChangeThresholdForSnapshot int   // min changed cells to trigger snapshot
}

// BackgroundCell matches the compressed storage format for schema persistence
type BackgroundCell struct {
	AverageRangeMeters   float32
	RangeSpreadMeters    float32
	TimesSeenCount       uint32
	LastUpdateUnixNanos  int64
	FrozenUntilUnixNanos int64
}

// BackgroundGrid enhanced for schema persistence and 100-track performance
type BackgroundGrid struct {
	SensorID    string
	SensorFrame FrameID // e.g., "sensor/hesai-01"

	Rings       int // e.g., 40 - matches schema rings INTEGER NOT NULL
	AzimuthBins int // e.g., 1800 for 0.2° - matches schema azimuth_bins INTEGER NOT NULL

	Cells []BackgroundCell // len = Rings * AzimuthBins

	Params BackgroundParams

	// Enhanced persistence tracking matching schema lidar_bg_snapshot table
	Manager              *BackgroundManager
	LastSnapshotTime     time.Time
	ChangesSinceSnapshot int
	SnapshotID           *int64 // tracks last persisted snapshot_id from schema

	// Performance tracking for system_events table integration
	LastProcessingTimeUs  int64
	WarmupFramesRemaining int
	SettlingComplete      bool

	// Telemetry for monitoring (feeds into system_events)
	ForegroundCount int64
	BackgroundCount int64

	// Thread safety for concurrent access during persistence
	// TODO: add mutex when implementing concurrent background updates
}

// Helper to index Cells: idx = ring*AzimuthBins + azBin
func (g *BackgroundGrid) Idx(ring, azBin int) int { return ring*g.AzimuthBins + azBin }

//
// Background persistence management matching schema design
//

// BackgroundManager handles automatic persistence following schema lidar_bg_snapshot pattern
type BackgroundManager struct {
	Grid            *BackgroundGrid
	SettlingTimer   *time.Timer
	PersistTimer    *time.Timer
	HasSettled      bool
	LastPersistTime time.Time
	StartTime       time.Time

	// Persistence callback to main app - should save to schema lidar_bg_snapshot table
	PersistCallback func(snapshot *BgSnapshot) error
}

// BgSnapshot exactly matches schema lidar_bg_snapshot table structure
type BgSnapshot struct {
	SnapshotID        *int64 // will be set by database after insert
	SensorID          string // matches sensor_id TEXT NOT NULL
	TakenUnixNanos    int64  // matches taken_unix_nanos INTEGER NOT NULL
	Rings             int    // matches rings INTEGER NOT NULL
	AzimuthBins       int    // matches azimuth_bins INTEGER NOT NULL
	ParamsJSON        string // matches params_json TEXT NOT NULL
	GridBlob          []byte // matches grid_blob BLOB NOT NULL (compressed BackgroundCell data)
	ChangedCellsCount int    // matches changed_cells_count INTEGER
	SnapshotReason    string // matches snapshot_reason TEXT ('settling_complete', 'periodic_update', 'manual')
}

// Ring buffer implementation for efficient memory management at 100-track scale
type RingBuffer[T any] struct {
	Items    []T
	Head     int
	Tail     int
	Size     int
	Capacity int
	mu       sync.RWMutex // Added thread safety for concurrent access
}

// Ring buffer methods for safe concurrent access
func (rb *RingBuffer[T]) Push(item T) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.Size == rb.Capacity {
		return false // Buffer full
	}

	rb.Items[rb.Tail] = item
	rb.Tail = (rb.Tail + 1) % rb.Capacity
	rb.Size++
	return true
}

func (rb *RingBuffer[T]) Pop() (T, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	var zero T
	if rb.Size == 0 {
		return zero, false
	}

	item := rb.Items[rb.Head]
	rb.Items[rb.Head] = zero // Clear reference
	rb.Head = (rb.Head + 1) % rb.Capacity
	rb.Size--
	return item, true
}

func (rb *RingBuffer[T]) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.Size
}

// Performance tracking for system_events table integration
type FrameStats struct {
	TSUnixNanos      int64
	PacketsReceived  int
	PointsTotal      int
	ForegroundPoints int
	ClustersFound    int
	TracksActive     int
	ProcessingTimeUs int64

	// Additional metrics for 100-track monitoring
	MemoryUsageMB   int64
	CPUUsagePercent float32
	DroppedPackets  int64
}

// SystemEvent represents entries for the schema system_events table
type SystemEvent struct {
	EventID     *int64                 // auto-generated by database
	SensorID    *string                // NULL for system-wide events
	TSUnixNanos int64                  // event timestamp
	EventType   string                 // 'performance', 'track_initiate', etc.
	EventData   map[string]interface{} // JSON data specific to event type
}

// Retention policies optimized for 100 concurrent tracks and schema constraints
type RetentionConfig struct {
	MaxConcurrentTracks          int           // 100 - matches design target
	MaxTrackObservationsPerTrack int           // 1000 observations per track - ring buffer size
	MaxRecentClusters            int           // 10,000 recent clusters - memory management
	MaxTrackAge                  time.Duration // 30 minutes for inactive tracks
	BgSnapshotInterval           time.Duration // 2 hours - matches schema automatic persistence
	BgSnapshotRetention          time.Duration // 48 hours - cleanup old snapshots
	BgSettlingPeriod             time.Duration // 5 minutes before first persist

	// Enhanced cleanup policies for schema maintenance
	MaxTrackFeatureAge   time.Duration // 7 days - cleanup old feature vectors
	MaxSystemEventAge    time.Duration // 30 days - cleanup old performance metrics
	ClusterPruneInterval time.Duration // 1 hour - memory cleanup frequency
}

//
// 2) Foreground extraction result (WORLD FRAME)
//    - clusters are already transformed into world/site coordinates
//    Enhanced to match schema lidar_clusters table exactly
//

// WorldCluster matches schema lidar_clusters table structure exactly
type WorldCluster struct {
	ClusterID         int64   // matches lidar_cluster_id INTEGER PRIMARY KEY
	SensorID          string  // matches sensor_id TEXT NOT NULL
	WorldFrame        FrameID // matches world_frame TEXT NOT NULL
	PoseID            int64   // matches pose_id INTEGER NOT NULL
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
	SamplePoints [][3]float32 // for debugging/thumbnails
}

// TrackSummary for HTTP API responses - streamlined view of track state
type TrackSummary struct {
	TrackID    string  // matches schema track_id TEXT PRIMARY KEY
	SensorID   string  // matches schema sensor_id TEXT NOT NULL
	WorldFrame FrameID // matches schema world_frame TEXT NOT NULL
	PoseID     int64   // matches schema pose_id INTEGER NOT NULL
	UnixNanos  int64   // current observation timestamp

	// Current kinematics (world frame; road-plane oriented)
	X, Y                 float32 // current position
	VelocityX, VelocityY float32 // current velocity
	SpeedMps             float32 // current speed magnitude
	HeadingRad           float32 // current heading

	// Current shape/quality
	BoundingBoxLength float32
	BoundingBoxWidth  float32
	BoundingBoxHeight float32
	PointsCount       int
	HeightP95         float32
	IntensityMean     float32

	// Classification from track summary
	ClassLabel      string  // matches schema class_label TEXT
	ClassConfidence float32 // matches schema class_conf REAL

	// Optional uncertainty (for advanced fusion)
	Covariance4x4 []float32 // flattened 4x4 covariance of [x y velocity_x velocity_y]
}

//
// 3) Tracking (WORLD FRAME)
//    Enhanced to match schema lidar_tracks and lidar_track_obs tables
//

// TrackState2D represents the core kinematic state for Kalman filtering
type TrackState2D struct {
	X, Y                 float32     // State vector in world frame: [x y velocity_x velocity_y]
	VelocityX, VelocityY float32     // Velocity components in world frame
	CovarianceMatrix     [16]float32 // Row-major covariance (4x4). float32 saves RAM for 100-track performance.
}

// Track enhanced to match schema lidar_tracks table structure
type Track struct {
	// Core identification matching schema exactly
	TrackID    string  // matches track_id TEXT PRIMARY KEY
	SensorID   string  // matches sensor_id TEXT NOT NULL
	WorldFrame FrameID // matches world_frame TEXT NOT NULL
	PoseID     int64   // matches pose_id INTEGER NOT NULL

	// Lifecycle timestamps matching schema
	FirstUnixNanos int64 // matches start_unix_nanos INTEGER NOT NULL
	LastUnixNanos  int64 // matches end_unix_nanos INTEGER (NULL if active)

	// Current state for real-time tracking
	State TrackState2D

	// Running averages matching schema summary fields
	BoundingBoxLengthAvg, BoundingBoxWidthAvg, BoundingBoxHeightAvg float32 // matches bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg REAL

	// Rollups for features/training matching schema fields
	ObservationCount int     // matches observation_count INTEGER
	AvgSpeedMps      float32 // matches avg_speed_mps REAL
	PeakSpeedMps     float32 // matches peak_speed_mps REAL
	HeightP95Max     float32 // matches height_p95_max REAL
	IntensityMeanAvg float32 // matches intensity_mean_avg REAL

	// Classification matching schema
	ClassLabel      string  // matches class_label TEXT
	ClassConfidence float32 // matches class_conf REAL

	// Source tracking matching schema (LiDAR-only implementation)
	SourceMask uint8 // matches source_mask INTEGER (bit0=lidar only for now)

	// Life-cycle management (in-memory only)
	Misses int // consecutive misses for deletion
}

// TrackObs exactly matches schema lidar_track_obs table structure
type TrackObs struct {
	TrackID    string  // matches track_id TEXT NOT NULL
	UnixNanos  int64   // matches ts_unix_nanos INTEGER NOT NULL
	WorldFrame FrameID // matches world_frame TEXT NOT NULL
	PoseID     int64   // matches pose_id INTEGER NOT NULL

	// Position matching schema
	X, Y, Z float32 // matches x, y, z REAL

	// Velocity matching schema
	VelocityX, VelocityY, VelocityZ float32 // matches velocity_x, velocity_y, velocity_z REAL

	// Derived kinematics matching schema
	SpeedMps   float32 // matches speed_mps REAL
	HeadingRad float32 // matches heading_rad REAL

	// Shape matching schema
	BoundingBoxLength, BoundingBoxWidth, BoundingBoxHeight float32 // matches bounding_box_length, bounding_box_width, bounding_box_height REAL

	// Quality metrics matching schema
	HeightP95     float32 // matches height_p95 REAL
	IntensityMean float32 // matches intensity_mean REAL
}

//
// 4) Future expansion hooks (deferred for Phase 2+)
//    Note: Radar and fusion data structures have been removed to match
//    the simplified LiDAR-only schema. These can be re-added when
//    radar/fusion tables are restored to the schema.
//

//
// 5) Supervisory containers
//    Enhanced for 100-track performance and schema integration
//

// SidecarState is the main state container optimized for 100 concurrent tracks
type SidecarState struct {
	Poses              *PoseCache                    // thread-safe pose management
	BackgroundManagers map[string]*BackgroundManager // enhanced with persistence
	Tracks             map[string]*Track             // up to 100 concurrent

	// Ring buffers sized for 100 tracks with thread safety
	RecentClusters   *RingBuffer[*WorldCluster]        // 10,000 capacity
	RecentTrackObs   map[string]*RingBuffer[*TrackObs] // 1000 per track
	RecentFrameStats *RingBuffer[*FrameStats]          // 1000 capacity

	// Performance monitoring for system_events integration
	TrackCount     int64
	DroppedPackets int64
	ActiveTracks   int64 // current number of active tracks
	TotalClusters  int64 // lifetime cluster count
	TotalFrames    int64 // lifetime frame count

	// Configuration
	Config *RetentionConfig

	// Schema integration hooks
	SystemEventCallback func(event *SystemEvent) error    // callback to persist system events
	ClusterCallback     func(cluster *WorldCluster) error // callback to persist clusters
	TrackObsCallback    func(obs *TrackObs) error         // callback to persist track observations

	// Thread safety for all operations
	mu sync.RWMutex
}

// Thread-safe methods for SidecarState
func (s *SidecarState) GetActiveTrackCount() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ActiveTracks
}

func (s *SidecarState) GetTrack(trackID string) (*Track, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	track, exists := s.Tracks[trackID]
	return track, exists
}

func (s *SidecarState) AddTrack(track *Track) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tracks[track.TrackID] = track
	s.ActiveTracks++
	s.TrackCount++
}

func (s *SidecarState) RemoveTrack(trackID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.Tracks[trackID]; exists {
		delete(s.Tracks, trackID)
		s.ActiveTracks--
	}
}

//
// 6) Event system for monitoring and debugging
//    Enhanced to integrate with schema system_events table
//

// Event represents a system event with structured context for debugging and monitoring
type Event struct {
	When    time.Time              // event timestamp
	Level   string                 // "info", "warn", "error", "debug"
	Message string                 // human-readable message
	Context map[string]interface{} // structured context data

	// Schema integration fields
	SensorID  *string // sensor that generated the event (if applicable)
	EventType string  // maps to system_events.event_type for persistence
}

// Helper constructors for common event types
func NewPerformanceEvent(sensorID *string, metricName string, metricValue float64) *Event {
	return &Event{
		When:      time.Now(),
		Level:     "info",
		Message:   "Performance metric recorded",
		SensorID:  sensorID,
		EventType: "performance",
		Context: map[string]interface{}{
			"metric_name":  metricName,
			"metric_value": metricValue,
		},
	}
}

func NewTrackInitiateEvent(trackID string, sensorID string, initialPos [2]float32) *Event {
	return &Event{
		When:      time.Now(),
		Level:     "info",
		Message:   "New track initiated",
		SensorID:  &sensorID,
		EventType: "track_initiate",
		Context: map[string]interface{}{
			"track_id": trackID,
			"initial_position": map[string]float32{
				"x": initialPos[0],
				"y": initialPos[1],
			},
		},
	}
}

func NewTrackTerminateEvent(trackID string, sensorID string, finalStats map[string]interface{}) *Event {
	return &Event{
		When:      time.Now(),
		Level:     "info",
		Message:   "Track terminated",
		SensorID:  &sensorID,
		EventType: "track_terminate",
		Context: map[string]interface{}{
			"track_id":    trackID,
			"final_stats": finalStats,
		},
	}
}
