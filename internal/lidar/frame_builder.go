package lidar

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Global registry for FrameBuilder instances keyed by SensorID.
var (
	fbRegistry   = map[string]*FrameBuilder{}
	fbRegistryMu = &sync.RWMutex{}
)

// RegisterFrameBuilder registers a FrameBuilder for a sensor ID.
func RegisterFrameBuilder(sensorID string, fb *FrameBuilder) {
	if sensorID == "" || fb == nil {
		return
	}
	fbRegistryMu.Lock()
	defer fbRegistryMu.Unlock()
	fbRegistry[sensorID] = fb
}

// GetFrameBuilder returns a registered FrameBuilder or nil
func GetFrameBuilder(sensorID string) *FrameBuilder {
	fbRegistryMu.RLock()
	defer fbRegistryMu.RUnlock()
	return fbRegistry[sensorID]
}

// Frame detection constants for azimuth-based rotation detection
const (
	// MinAzimuthCoverage is the minimum azimuth coverage (degrees) required for a valid frame
	// Must cover at least 340° of a full 360° rotation to be considered complete
	MinAzimuthCoverage = 340.0

	// MinFramePointsForCompletion is the minimum number of points required for frame completion
	// Ensures substantial data before declaring a rotation complete (typical full rotation: ~70k points)
	MinFramePointsForCompletion = 10000
)

//
// FrameBuilder - accumulates points into complete rotational frames
//

// LiDARFrame represents one complete 360° rotation of LiDAR data
type LiDARFrame struct {
	FrameID        string    // unique identifier for this frame
	SensorID       string    // which sensor generated this frame
	StartTimestamp time.Time // timestamp of first point in frame
	EndTimestamp   time.Time // timestamp of last point in frame
	Points         []Point   // all points in this complete rotation
	MinAzimuth     float64   // minimum azimuth angle observed
	MaxAzimuth     float64   // maximum azimuth angle observed
	PointCount     int       // total number of points in frame
	SpinComplete   bool      // true when full 360° rotation detected

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
	exportNextFrameASC  bool              // flag to export next completed frame
	exportNextFramePath string            // output path for ASC export
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

	// Time-based frame detection for accurate motor speed handling
	expectedFrameDuration time.Duration // expected duration per frame based on motor speed
	enableTimeBased       bool          // true to use time-based detection with azimuth validation
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
	}

	// Start cleanup timer
	fb.cleanupTimer = time.AfterFunc(fb.cleanupInterval, fb.cleanupFrames)

	// Register FrameBuilder instance
	RegisterFrameBuilder(config.SensorID, fb)

	return fb
}

// SetMotorSpeed updates the expected frame duration based on motor speed (RPM)
// This enables time-based frame detection for accurate motor speed handling
func (fb *FrameBuilder) SetMotorSpeed(rpm uint16) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if rpm == 0 {
		// Disable time-based detection if RPM is unknown
		fb.enableTimeBased = false
		fb.expectedFrameDuration = 0
		return
	}

	// Calculate expected frame duration: 60,000ms / RPM
	fb.expectedFrameDuration = time.Duration(60000/rpm) * time.Millisecond
	fb.enableTimeBased = true
}

// EnableTimeBased enables or disables time-based frame detection
func (fb *FrameBuilder) EnableTimeBased(enable bool) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.enableTimeBased = enable
}

// NOTE: Legacy AddPoints removed in polar-first refactor. Use AddPointsPolar.

// AddPointsPolar accepts polar points (sensor-frame) and converts them to cartesian Points
// before processing. This is used by network listeners that parse into polar form.
func (fb *FrameBuilder) AddPointsPolar(polar []PointPolar) {
	if len(polar) == 0 {
		return
	}

	pts := make([]Point, 0, len(polar))
	for _, p := range polar {
		x, y, z := SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)
		pts = append(pts, Point{
			X:           x,
			Y:           y,
			Z:           z,
			Intensity:   p.Intensity,
			Distance:    p.Distance,
			Azimuth:     p.Azimuth,
			Elevation:   p.Elevation,
			Channel:     p.Channel,
			Timestamp:   time.Unix(0, p.Timestamp),
			BlockID:     p.BlockID,
			UDPSequence: p.UDPSequence,
		})
	}

	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.addPointsInternal(pts)
}

// addPointsInternal processes cartesian Points assuming lock is held by caller for safety
func (fb *FrameBuilder) addPointsInternal(points []Point) {
	if len(points) == 0 {
		return
	}

	// Process each point for azimuth-based frame detection
	for _, point := range points {
		// Check for UDP sequence gaps
		fb.checkSequenceGaps(point.UDPSequence)

		// Check if we need to start a new frame based on azimuth wrap and/or time
		if fb.shouldStartNewFrame(point.Azimuth, point.Timestamp) {
			fb.finalizeCurrentFrame()
			fb.startNewFrame(point.Timestamp)
		}

		// Ensure we have a current frame
		if fb.currentFrame == nil {
			fb.startNewFrame(point.Timestamp)
		}

		// Add point to current frame
		fb.addPointToCurrentFrame(point)
		fb.lastAzimuth = point.Azimuth
	}
}

// shouldStartNewFrame determines if we should start a new frame based on azimuth and/or time
func (fb *FrameBuilder) shouldStartNewFrame(azimuth float64, timestamp time.Time) bool {
	if fb.lastAzimuth < 0 {
		return false // First point ever
	}

	if fb.currentFrame == nil {
		return true // No current frame
	}

	// Time-based frame detection (if enabled and duration is configured)
	if fb.enableTimeBased && fb.expectedFrameDuration > 0 {
		frameDuration := timestamp.Sub(fb.currentFrame.StartTimestamp)

		// If we've exceeded the expected frame duration, start a new frame
		// Add a small tolerance (10%) to account for timing variations
		maxDuration := fb.expectedFrameDuration + (fb.expectedFrameDuration / 10)
		if frameDuration >= maxDuration {
			// Additional validation: ensure we have reasonable azimuth coverage
			// This prevents starting frames on timing anomalies without spatial coverage
			azimuthRange := fb.currentFrame.MaxAzimuth - fb.currentFrame.MinAzimuth
			if azimuthRange > 270.0 { // At least 3/4 rotation coverage
				return true
			}
		}

		// Even with time-based detection, respect azimuth wraps for precise timing
		// but with relaxed requirements since we're time-bounded
		if fb.lastAzimuth > 340.0 && azimuth < 20.0 && frameDuration >= (fb.expectedFrameDuration/2) {
			return true
		}
	} else {
		// Traditional azimuth-based detection (original logic)
		// Detect azimuth wrap (360° → 0°) only when crossing from high to low
		// Require strict conditions to avoid false triggers from individual packets
		if fb.lastAzimuth > 350.0 && azimuth < 10.0 {
			// Additional checks to ensure this is a complete rotation:
			// 1. Frame must have substantial azimuth coverage (near 360°)
			// 2. Frame must have enough points (substantial data)
			// 3. Current frame azimuth range must indicate a near-complete rotation
			if fb.currentFrame != nil &&
				(fb.currentFrame.MaxAzimuth-fb.currentFrame.MinAzimuth) > MinAzimuthCoverage &&
				fb.currentFrame.PointCount > MinFramePointsForCompletion {
				return true
			}
		}
	}

	return false
}

// startNewFrame creates a new frame for accumulating points
func (fb *FrameBuilder) startNewFrame(timestamp time.Time) {
	fb.frameCounter++
	fb.currentFrame = &LiDARFrame{
		FrameID:         fmt.Sprintf("%s-frame-%d", fb.sensorID, fb.frameCounter),
		SensorID:        fb.sensorID,
		StartTimestamp:  timestamp,
		EndTimestamp:    timestamp,
		Points:          make([]Point, 0, 36000), // pre-allocate for full rotation
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

// checkSequenceGaps detects missing UDP sequence numbers
func (fb *FrameBuilder) checkSequenceGaps(sequence uint32) {
	if fb.lastSequence == 0 {
		fb.lastSequence = sequence
		return
	}

	// Check for sequence gap
	expectedNext := fb.lastSequence + 1
	if sequence > expectedNext {
		// Mark missing sequences
		for missing := expectedNext; missing < sequence; missing++ {
			fb.sequenceGaps[missing] = true
		}
	}

	fb.lastSequence = sequence
}

// finalizeCurrentFrame completes the current frame and moves it to buffer
func (fb *FrameBuilder) finalizeCurrentFrame() {
	if fb.currentFrame == nil || fb.currentFrame.PointCount < fb.minFramePoints {
		fb.currentFrame = nil // Discard incomplete frame
		return
	}

	frame := fb.currentFrame
	fb.currentFrame = nil

	// Calculate completeness metrics
	fb.calculateFrameCompleteness(frame)

	// Move to buffer for potential backfill
	fb.frameBuffer[frame.FrameID] = frame

	// Enforce buffer size limit
	if len(fb.frameBuffer) > fb.frameBufferSize {
		fb.evictOldestBufferedFrame()
	}
}

// evictOldestBufferedFrame removes the oldest frame from buffer and finalizes it
func (fb *FrameBuilder) evictOldestBufferedFrame() {
	var oldestFrame *LiDARFrame
	var oldestID string

	for frameID, frame := range fb.frameBuffer {
		if oldestFrame == nil || frame.StartTimestamp.Before(oldestFrame.StartTimestamp) {
			oldestFrame = frame
			oldestID = frameID
		}
	}

	if oldestFrame != nil {
		delete(fb.frameBuffer, oldestID)
		// Send oldest frame to output channel
		// Handle finalized frame (for now just log completion)
		// TODO: Add output channel or callback for completed frames
	}
}

// calculateFrameCompleteness analyzes frame completeness based on sequence gaps
func (fb *FrameBuilder) calculateFrameCompleteness(frame *LiDARFrame) {
	if len(frame.ReceivedPackets) == 0 {
		return
	}

	// Find sequence range for this frame
	var minSeq, maxSeq uint32 = ^uint32(0), 0
	for seq := range frame.ReceivedPackets {
		if seq < minSeq {
			minSeq = seq
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}

	// Calculate expected packets in range
	expectedCount := maxSeq - minSeq + 1
	receivedCount := uint32(len(frame.ReceivedPackets))

	// Identify missing packets
	for seq := minSeq; seq <= maxSeq; seq++ {
		frame.ExpectedPackets[seq] = true
		if !frame.ReceivedPackets[seq] {
			frame.MissingPackets = append(frame.MissingPackets, seq)
		}
	}

	frame.PacketGaps = len(frame.MissingPackets)
	frame.CompletenessRatio = float64(receivedCount) / float64(expectedCount)
	frame.AzimuthCoverage = frame.MaxAzimuth - frame.MinAzimuth
	if frame.AzimuthCoverage < 0 {
		frame.AzimuthCoverage += 360.0 // Handle wrap-around
	}
}

// cleanupFrames periodically checks for frames that should be finalized
func (fb *FrameBuilder) cleanupFrames() {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	now := time.Now()
	var frameIDsToFinalize []string

	// Find frames that are old enough to finalize
	for frameID, frame := range fb.frameBuffer {
		frameAge := now.Sub(frame.EndTimestamp)
		if frameAge >= fb.bufferTimeout {
			frameIDsToFinalize = append(frameIDsToFinalize, frameID)
		}
	}

	// Finalize old frames
	for _, frameID := range frameIDsToFinalize {
		frame := fb.frameBuffer[frameID]
		delete(fb.frameBuffer, frameID)
		fb.finalizeFrame(frame)
	}

	// Schedule next cleanup
	fb.cleanupTimer = time.AfterFunc(fb.cleanupInterval, fb.cleanupFrames)
}

// finalizeFrame completes a frame and calls the callback
func (fb *FrameBuilder) finalizeFrame(frame *LiDARFrame) {
	if frame == nil {
		return
	}

	// Mark frame as complete
	frame.SpinComplete = true

	// Export to ASC if requested
	if fb.exportNextFrameASC {
		path := fb.exportNextFramePath
		if path == "" {
			path = filepath.Join(os.TempDir(), fmt.Sprintf("next_frame_%s_%d.asc", frame.SensorID, time.Now().Unix()))
		}
		if err := exportFrameToASC(frame); err != nil {
			log.Printf("[FrameBuilder] Failed to export next frame for sensor %s to %s: %v", frame.SensorID, path, err)
		} else {
			log.Printf("[FrameBuilder] Exported next frame for sensor %s to %s", frame.SensorID, path)
		}
		fb.exportNextFrameASC = false
		fb.exportNextFramePath = ""
	}
	// Call callback if provided (in separate goroutine to avoid blocking)
	if fb.frameCallback != nil {
		go fb.frameCallback(frame)
	}
}

// Request export of the next completed frame to ASC format
func (fb *FrameBuilder) RequestExportNextFrameASC(outPath string) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.exportNextFrameASC = true
	fb.exportNextFramePath = outPath
}

// GetCurrentFrameStats returns statistics about the frames currently being built
func (fb *FrameBuilder) GetCurrentFrameStats() (frameCount int, oldestAge time.Duration, newestAge time.Duration) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	frameCount = len(fb.frameBuffer)
	if frameCount == 0 {
		return 0, 0, 0
	}

	now := time.Now()
	var oldest, newest time.Time
	first := true

	for _, frame := range fb.frameBuffer {
		if first {
			oldest = frame.StartTimestamp
			newest = frame.StartTimestamp
			first = false
		} else {
			if frame.StartTimestamp.Before(oldest) {
				oldest = frame.StartTimestamp
			}
			if frame.StartTimestamp.After(newest) {
				newest = frame.StartTimestamp
			}
		}
	}

	return frameCount, now.Sub(oldest), now.Sub(newest)
}

// NewFrameBuilderWithLogging creates a FrameBuilder that logs completed frames
// This is a convenience function for common use cases where you want to log frame completion
func NewFrameBuilderWithLogging(sensorID string) *FrameBuilder {
	return NewFrameBuilderWithDebugLogging(sensorID, false)
}

// NewFrameBuilderWithDebugLogging creates a FrameBuilder with optional debug logging
func NewFrameBuilderWithDebugLogging(sensorID string, debug bool) *FrameBuilder {
	return NewFrameBuilderWithDebugLoggingAndInterval(sensorID, debug, 2*time.Second)
}

// NewFrameBuilderWithDebugLoggingAndInterval creates a FrameBuilder with optional debug logging and export interval
func NewFrameBuilderWithDebugLoggingAndInterval(sensorID string, debug bool, logInterval time.Duration) *FrameBuilder {
	var callback func(*LiDARFrame)

	if debug {
		var lastExportTime time.Time
		var exportMutex sync.Mutex

		callback = func(frame *LiDARFrame) {
			log.Printf("Frame completed - ID: %s, Points: %d, Azimuth: %.1f°-%.1f°, Duration: %v, Sensor: %s",
				frame.FrameID,
				frame.PointCount,
				frame.MinAzimuth,
				frame.MaxAzimuth,
				frame.EndTimestamp.Sub(frame.StartTimestamp),
				frame.SensorID)

			// Export frame to CloudCompare .asc format only once per log interval
			exportMutex.Lock()
			now := time.Now()
			if now.Sub(lastExportTime) >= logInterval {
				lastExportTime = now
				exportMutex.Unlock()

				if err := exportFrameToASC(frame); err != nil {
					log.Printf("Failed to export frame %s: %v", frame.FrameID, err)
				}
			} else {
				exportMutex.Unlock()
			}
		}
	} else {
		// No logging callback when debug is disabled
		callback = nil
	}

	return NewFrameBuilder(FrameBuilderConfig{
		SensorID:      sensorID,
		FrameCallback: callback,
		// Enhanced buffering for out-of-order packet handling
		FrameBufferSize: 100,                    // buffer 100 frames = 10 seconds at 10 Hz
		BufferTimeout:   500 * time.Millisecond, // wait 500ms for late packets (5x frame duration)
		CleanupInterval: 250 * time.Millisecond, // check every 250ms for better responsiveness
	})
}

// exportFrameToASC exports a LiDARFrame to CloudCompare .asc ASCII format
func exportFrameToASC(frame *LiDARFrame) error {
	if frame == nil || len(frame.Points) == 0 {
		return fmt.Errorf("empty frame")
	}

	filename := fmt.Sprintf("lidar_frame_%s_%d.asc", frame.SensorID, frame.StartTimestamp.Unix())
	filePath := filepath.Join(os.TempDir(), filename)

	// Convert LiDARFrame points to PointASC
	ascPoints := make([]PointASC, len(frame.Points))
	// Detect if Z values look invalid (all zero) and recompute from polar if needed
	zNonZero := 0
	for _, p := range frame.Points {
		if p.Z != 0 {
			zNonZero++
			break
		}
	}
	if zNonZero == 0 {
		// Recompute XYZ from Distance/Azimuth/Elevation
		log.Printf("[FrameBuilder] all Z==0 for frame %s; recomputing XYZ from polar data before export", frame.FrameID)
		for i, p := range frame.Points {
			x, y, z := SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)
			ascPoints[i] = PointASC{
				X:         x,
				Y:         y,
				Z:         z,
				Intensity: int(p.Intensity),
			}
		}
	} else {
		for i, p := range frame.Points {
			ascPoints[i] = PointASC{
				X:         p.X,
				Y:         p.Y,
				Z:         p.Z,
				Intensity: int(p.Intensity),
			}
		}
	}

	extraHeader := "" // No extra columns for now
	err := ExportPointsToASC(ascPoints, filePath, extraHeader)
	if err != nil {
		return fmt.Errorf("failed to export ASC: %w", err)
	}
	log.Printf("Exported frame %s to %s (%d points)", frame.FrameID, filePath, frame.PointCount)
	return nil
}
