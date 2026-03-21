package l2frames

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Frame detection constants for azimuth-based rotation detection
const (
	// MinAzimuthCoverage is the minimum azimuth coverage (degrees) required for a valid frame
	// Must cover at least 340° of a full 360° rotation to be considered complete
	MinAzimuthCoverage = 340.0

	// MinFramePointsForCompletion is the minimum number of points required for frame completion
	// Ensures substantial data before declaring a rotation complete (typical full rotation: ~70k points)
	MinFramePointsForCompletion = 10000
)

// LiDARFrame represents one complete 360° rotation of LiDAR data
type LiDARFrame struct {
	FrameID        string       // unique identifier for this frame
	SensorID       string       // which sensor generated this frame
	StartTimestamp time.Time    // timestamp of first point in frame
	EndTimestamp   time.Time    // timestamp of last point in frame
	StartWallTime  time.Time    // wall-clock time when frame started (ingest time)
	EndWallTime    time.Time    // wall-clock time when last point was ingested
	PolarPoints    []PointPolar // sensor-polar view: all points in polar coordinates
	Points         []Point      // sensor-Cartesian view: all points in Cartesian coordinates
	MinAzimuth     float64      // minimum azimuth angle observed
	MaxAzimuth     float64      // maximum azimuth angle observed
	PointCount     int          // total number of points in frame
	SpinComplete   bool         // true when full 360° rotation detected

	// Completeness tracking
	ExpectedPackets   map[uint32]bool // expected UDP sequence numbers
	ReceivedPackets   map[uint32]bool // received UDP sequence numbers
	MissingPackets    []uint32        // sequence numbers of missing packets
	PacketGaps        int             // count of missing packets
	CompletenessRatio float64         // ratio of received/expected packets
	AzimuthCoverage   float64         // degrees of azimuth covered (0-360)
}

// FrameBuilder accumulates points from multiple packets into complete rotational frames
// Uses azimuth-based rotation detection and UDP sequence tracking for completeness
type FrameBuilder struct {
	sensorID            string            // sensor identifier
	frameCallback       func(*LiDARFrame) // callback when frame is complete
	frameCh             chan *LiDARFrame  // serialises frame callback invocations
	frameDone           chan struct{}     // closed when frameCallbackWorker exits
	droppedFrames       atomic.Uint64     // count of frames dropped due to full channel (accessed atomically)
	blockOnFrameChannel bool              // when true, block instead of dropping frames (analysis mode)
	exportNextFrameASC  bool              // flag to export next completed frame
	exportBatchCount    int               // number of frames to export in batch
	exportBatchExported int               // number of frames already exported in current batch
	mu                  sync.Mutex        // protect concurrent access
	frameCounter        int64             // sequential frame number

	// Azimuth-based frame detection
	currentFrame     *LiDARFrame // frame currently being built
	lastAzimuth      float64     // previous azimuth to detect 360° wrap
	azimuthTolerance float64     // tolerance for azimuth wrap detection (default: 10°)
	minFramePoints   int         // minimum points required for valid frame

	// UDP sequence tracking for completeness
	lastSequence     uint32             // last processed UDP sequence
	sequenceGaps     map[uint32]bool    // detected sequence gaps
	pendingPackets   map[uint32][]Point // out-of-order packets waiting for backfill
	maxBackfillDelay time.Duration      // max time to wait for backfill packets

	// Frame buffering for late packets
	frameBuffer     map[string]*LiDARFrame // completed frames awaiting finalization
	frameBufferSize int                    // max frames to buffer
	bufferTimeout   time.Duration          // how long to wait before finalizing frame

	// Cleanup timer to finalize old frames
	cleanupTimer    *time.Timer
	cleanupInterval time.Duration // how often to check for frames to finalize
	closed          bool          // set by Close() to prevent cleanupFrames rescheduling
	closeCh         chan struct{} // closed by Close() to unblock in-flight blocking sends

	// Time-based frame detection for accurate motor speed handling
	expectedFrameDuration time.Duration // expected duration per frame based on motor speed
	enableTimeBased       bool          // true to use time-based detection with azimuth validation
	// debug toggles lightweight frame-completion logging when true
	lastArrivalWallTime time.Time
}

func frameAzimuthCoverage(frame *LiDARFrame) float64 {
	if frame == nil {
		return 0
	}
	cov := frame.MaxAzimuth - frame.MinAzimuth
	if cov < 0 {
		cov += 360.0
	}
	return cov
}

// FrameBuilderConfig contains configuration for the FrameBuilder
type FrameBuilderConfig struct {
	SensorID              string            // sensor identifier
	FrameCallback         func(*LiDARFrame) // callback when frame is complete
	AzimuthTolerance      float64           // tolerance for azimuth wrap detection (default: 10°)
	MinFramePoints        int               // minimum points required for valid frame (default: 1000)
	MaxBackfillDelay      time.Duration     // max time to wait for backfill packets (default: 100ms)
	FrameBufferSize       int               // max frames to buffer (default: 10)
	BufferTimeout         time.Duration     // how long to wait before finalizing frame (default: 1s)
	CleanupInterval       time.Duration     // how often to check for frames to finalize (default: 250ms)
	ExpectedFrameDuration time.Duration     // expected duration per frame based on motor speed (default: 0 = azimuth-only)
	EnableTimeBased       bool              // true to use time-based detection with azimuth validation

	// FrameChCapacity sets the buffered channel capacity for the frame
	// callback worker. Default 8 is adequate for live sensor input;
	// PCAP replay benefits from a larger buffer (e.g. 32) to absorb
	// short processing stalls without dropping frames.
	FrameChCapacity int
}

// NewFrameBuilder creates a new FrameBuilder with the specified configuration
func NewFrameBuilder(config FrameBuilderConfig) *FrameBuilder {
	// Set reasonable defaults
	if config.FrameBufferSize == 0 {
		config.FrameBufferSize = 10 // buffer 10 frames for out-of-order processing
	}
	if config.AzimuthTolerance == 0 {
		config.AzimuthTolerance = 10.0 // 10° tolerance for azimuth wrap detection
	}
	if config.MinFramePoints == 0 {
		config.MinFramePoints = 1000 // minimum 1000 points for valid frame
	}
	if config.MaxBackfillDelay == 0 {
		config.MaxBackfillDelay = 100 * time.Millisecond // wait 100ms for backfill
	}
	if config.BufferTimeout == 0 {
		config.BufferTimeout = 1000 * time.Millisecond // wait 1s before finalizing
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 250 * time.Millisecond // cleanup every 250ms
	}

	fb := &FrameBuilder{
		sensorID:              config.SensorID,
		frameCallback:         config.FrameCallback,
		lastAzimuth:           -1.0, // invalid initial value to detect first point
		azimuthTolerance:      config.AzimuthTolerance,
		minFramePoints:        config.MinFramePoints,
		sequenceGaps:          make(map[uint32]bool),
		pendingPackets:        make(map[uint32][]Point),
		maxBackfillDelay:      config.MaxBackfillDelay,
		frameBuffer:           make(map[string]*LiDARFrame),
		frameBufferSize:       config.FrameBufferSize,
		bufferTimeout:         config.BufferTimeout,
		cleanupInterval:       config.CleanupInterval,
		expectedFrameDuration: config.ExpectedFrameDuration,
		enableTimeBased:       config.EnableTimeBased,
		closeCh:               make(chan struct{}),
	}

	// Start cleanup timer (protect with mutex to avoid race with timer callback)
	fb.mu.Lock()
	fb.cleanupTimer = time.AfterFunc(fb.cleanupInterval, fb.cleanupFrames)
	fb.mu.Unlock()

	// Start serialised frame callback worker. The channel ensures that
	// only one frame callback runs at a time, preventing concurrent
	// tracker Update() and persistence operations that cause data races.
	if fb.frameCallback != nil {
		chCap := config.FrameChCapacity
		if chCap <= 0 {
			chCap = 8
		}
		fb.frameCh = make(chan *LiDARFrame, chCap)
		fb.frameDone = make(chan struct{})
		go fb.frameCallbackWorker()
	}

	// Register FrameBuilder instance
	RegisterFrameBuilder(config.SensorID, fb)

	return fb
}

// NewFrameBuilderDI creates a FrameBuilder without registering it in the
// global registry. Prefer this constructor when wiring dependencies
// explicitly via pipeline.SensorRuntime.
func NewFrameBuilderDI(config FrameBuilderConfig) *FrameBuilder {
	// Set reasonable defaults
	if config.FrameBufferSize == 0 {
		config.FrameBufferSize = 10 // buffer 10 frames for out-of-order processing
	}
	if config.AzimuthTolerance == 0 {
		config.AzimuthTolerance = 10.0 // 10° tolerance for azimuth wrap detection
	}
	if config.MinFramePoints == 0 {
		config.MinFramePoints = 1000 // minimum 1000 points for valid frame
	}
	if config.MaxBackfillDelay == 0 {
		config.MaxBackfillDelay = 100 * time.Millisecond // wait 100ms for backfill
	}
	if config.BufferTimeout == 0 {
		config.BufferTimeout = 1000 * time.Millisecond // wait 1s before finalizing
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 250 * time.Millisecond // cleanup every 250ms
	}

	fb := &FrameBuilder{
		sensorID:              config.SensorID,
		frameCallback:         config.FrameCallback,
		lastAzimuth:           -1.0, // invalid initial value to detect first point
		azimuthTolerance:      config.AzimuthTolerance,
		minFramePoints:        config.MinFramePoints,
		sequenceGaps:          make(map[uint32]bool),
		pendingPackets:        make(map[uint32][]Point),
		maxBackfillDelay:      config.MaxBackfillDelay,
		frameBuffer:           make(map[string]*LiDARFrame),
		frameBufferSize:       config.FrameBufferSize,
		bufferTimeout:         config.BufferTimeout,
		cleanupInterval:       config.CleanupInterval,
		expectedFrameDuration: config.ExpectedFrameDuration,
		enableTimeBased:       config.EnableTimeBased,
		closeCh:               make(chan struct{}),
	}

	// Start cleanup timer (protect with mutex to avoid race with timer callback)
	fb.mu.Lock()
	fb.cleanupTimer = time.AfterFunc(fb.cleanupInterval, fb.cleanupFrames)
	fb.mu.Unlock()

	// Start serialised frame callback worker. The channel ensures that
	// only one frame callback runs at a time, preventing concurrent
	// tracker Update() and persistence operations that cause data races.
	if fb.frameCallback != nil {
		chCap := config.FrameChCapacity
		if chCap <= 0 {
			chCap = 8
		}
		fb.frameCh = make(chan *LiDARFrame, chCap)
		fb.frameDone = make(chan struct{})
		go fb.frameCallbackWorker()
	}

	// Note: Skip RegisterFrameBuilder call for DI version

	return fb
}

// AddPointsPolar accepts polar points (sensor-frame) and converts them to cartesian Points
// before processing. Both polar and Cartesian representations are stored on the frame.
// This is used by network listeners that parse into polar form.
func (fb *FrameBuilder) AddPointsPolar(polar []PointPolar) {
	if len(polar) == 0 {
		return
	}

	pts := make([]Point, 0, len(polar))
	for _, p := range polar {
		x, y, z := SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)
		pts = append(pts, Point{
			X:               x,
			Y:               y,
			Z:               z,
			Intensity:       p.Intensity,
			Distance:        p.Distance,
			Azimuth:         p.Azimuth,
			Elevation:       p.Elevation,
			Channel:         p.Channel,
			Timestamp:       time.Unix(0, p.Timestamp),
			BlockID:         p.BlockID,
			UDPSequence:     p.UDPSequence,
			RawBlockAzimuth: p.RawBlockAzimuth,
		})
	}

	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.addPointsDualInternal(pts, polar)
}

// addPointsDualInternal processes paired Cartesian + polar points, storing both
// representations on each frame. Lock must be held by caller.
// Panics if len(points) != len(polar) — callers must guarantee alignment.
func (fb *FrameBuilder) addPointsDualInternal(points []Point, polar []PointPolar) {
	if len(points) == 0 {
		return
	}
	if len(points) != len(polar) {
		panic(fmt.Sprintf("addPointsDualInternal: slice length mismatch: points=%d, polar=%d", len(points), len(polar)))
	}

	arrivalNow := time.Now()
	fb.lastArrivalWallTime = arrivalNow

	// Debug: record previous frame point count when enabled
	var prevCount int
	if fb.currentFrame != nil {
		prevCount = fb.currentFrame.PointCount
	}

	// Process each point for azimuth-based frame detection
	for i, point := range points {
		// Check for UDP sequence gaps
		fb.checkSequenceGaps(point.UDPSequence)

		// Check if we need to start a new frame based on azimuth wrap and/or time
		shouldStart, reason := fb.shouldStartNewFrame(point.Azimuth, point.Timestamp)
		if shouldStart {
			if fb.currentFrame != nil {
				tracef("[FrameBuilder] Frame completion detected (%s): lastAz=%.2f currAz=%.2f, finalizing frame with %d points",
					reason, fb.lastAzimuth, point.Azimuth, fb.currentFrame.PointCount)
			}
			fb.finalizeCurrentFrame()
			fb.startNewFrame(point.Timestamp, arrivalNow)
		}

		// Ensure we have a current frame
		if fb.currentFrame == nil {
			fb.startNewFrame(point.Timestamp, arrivalNow)
		}

		// Add both representations to current frame
		fb.addPointToCurrentFrame(point)
		fb.currentFrame.PolarPoints = append(fb.currentFrame.PolarPoints, polar[i])
		fb.lastAzimuth = point.Azimuth
	}

	if fb.currentFrame != nil {
		fb.currentFrame.EndWallTime = arrivalNow
	}

	var newCount int
	if fb.currentFrame != nil {
		newCount = fb.currentFrame.PointCount
	}
	tracef("[FrameBuilder] Added %d points (dual); frame_count was=%d now=%d; lastAzimuth=%.2f",
		len(points), prevCount, newCount, fb.lastAzimuth)
}

// shouldStartNewFrame determines if we should start a new frame based on azimuth and/or time
func (fb *FrameBuilder) shouldStartNewFrame(azimuth float64, timestamp time.Time) (bool, string) {
	if fb.lastAzimuth < 0 {
		return false, "" // First point ever
	}

	if fb.currentFrame == nil {
		return true, "initialize" // No current frame
	}

	cov := frameAzimuthCoverage(fb.currentFrame)

	// Time-based frame detection (if enabled and duration is configured)
	if fb.enableTimeBased && fb.expectedFrameDuration > 0 {
		frameDuration := timestamp.Sub(fb.currentFrame.StartTimestamp)

		// If we've exceeded the expected frame duration, start a new frame
		// Add a small tolerance (10%) to account for timing variations
		maxDuration := fb.expectedFrameDuration + (fb.expectedFrameDuration / 10)
		if frameDuration >= maxDuration && cov >= MinAzimuthCoverage {
			return true, fmt.Sprintf("time_limit_exceeded (dur=%v, cov=%.1f)", frameDuration, cov)
		}

		// Even with time-based detection, respect azimuth wraps for precise timing
		// but with relaxed requirements since we're time-bounded
		if fb.lastAzimuth > 340.0 && azimuth < 20.0 && frameDuration >= (fb.expectedFrameDuration/2) && cov >= MinAzimuthCoverage {
			return true, "azimuth_wrap_time_aligned"
		}
	} else {
		// Traditional azimuth-based detection (original logic)
		// Detect azimuth wrap (360° → 0°) only when crossing from high to low
		// Require strict conditions to avoid false triggers from individual packets
		// Also detect large negative jumps in azimuth (e.g., 289° -> 61°) which
		// indicate a rotation wrap even if values don't cross the 350°->10° band.
		if fb.lastAzimuth-azimuth > 180.0 {
			if fb.currentFrame != nil && fb.currentFrame.PointCount > fb.minFramePoints && cov >= MinAzimuthCoverage {
				return true, "azimuth_wrap_large_jump"
			}
		}

		if fb.lastAzimuth > 350.0 && azimuth < 10.0 {
			// Additional checks to ensure this is a complete rotation:
			// 1. Frame must have substantial azimuth coverage (near 360°)
			// 2. Frame must have enough points (substantial data)
			// 3. Current frame azimuth range must indicate a near-complete rotation
			if fb.currentFrame != nil &&
				(fb.currentFrame.MaxAzimuth-fb.currentFrame.MinAzimuth) > MinAzimuthCoverage &&
				fb.currentFrame.PointCount > MinFramePointsForCompletion {
				return true, "azimuth_wrap_crossing"
			}
		}
	}

	return false, ""
}

// startNewFrame creates a new frame for accumulating points
func (fb *FrameBuilder) startNewFrame(timestamp time.Time, wallTime time.Time) {
	fb.frameCounter++
	fb.currentFrame = &LiDARFrame{
		FrameID:         fmt.Sprintf("%s-frame-%d", fb.sensorID, fb.frameCounter),
		SensorID:        fb.sensorID,
		StartTimestamp:  timestamp,
		EndTimestamp:    timestamp,
		StartWallTime:   wallTime,
		EndWallTime:     wallTime,
		PolarPoints:     make([]PointPolar, 0, 36000), // pre-allocate for full rotation
		Points:          make([]Point, 0, 36000),      // pre-allocate for full rotation
		MinAzimuth:      360.0,
		MaxAzimuth:      0.0,
		ExpectedPackets: make(map[uint32]bool),
		ReceivedPackets: make(map[uint32]bool),
		MissingPackets:  make([]uint32, 0),
		SpinComplete:    false,
	}
}

// addPointToCurrentFrame adds a point to the current frame being built
func (fb *FrameBuilder) addPointToCurrentFrame(point Point) {
	if fb.currentFrame == nil {
		return
	}

	frame := fb.currentFrame

	// Add point to frame
	frame.Points = append(frame.Points, point)
	frame.PointCount++

	// Track packet for completeness
	frame.ReceivedPackets[point.UDPSequence] = true

	// Update timestamp range
	if point.Timestamp.Before(frame.StartTimestamp) {
		frame.StartTimestamp = point.Timestamp
	}
	if point.Timestamp.After(frame.EndTimestamp) {
		frame.EndTimestamp = point.Timestamp
	}

	// Update azimuth range
	if point.Azimuth < frame.MinAzimuth {
		frame.MinAzimuth = point.Azimuth
	}
	if point.Azimuth > frame.MaxAzimuth {
		frame.MaxAzimuth = point.Azimuth
	}
}
