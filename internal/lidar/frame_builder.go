package lidar

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
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
}

// FrameBuilder accumulates points from multiple packets into complete rotational frames
// Uses time-based buffering to allow late-arriving packets to backfill appropriate frames
type FrameBuilder struct {
	sensorID      string            // sensor identifier
	frameCallback func(*LiDARFrame) // callback when frame is complete
	mu            sync.Mutex        // protect concurrent access
	frameCounter  int64             // sequential frame number

	// Time-based buffering for late packets
	frameBuffer     map[int64]*LiDARFrame // buffered frames by time slot (100ms slots)
	frameBufferSize int                   // max frames to buffer (default: 50 = 5 seconds)
	frameDuration   time.Duration         // duration per frame (default: 1s based on observed timing)
	bufferTimeout   time.Duration         // how long to wait before finalizing frame (default: 1s)

	// Cleanup timer to finalize old frames
	cleanupTimer    *time.Timer
	cleanupInterval time.Duration // how often to check for frames to finalize
}

// FrameBuilderConfig contains configuration for the FrameBuilder
type FrameBuilderConfig struct {
	SensorID        string            // sensor identifier
	FrameCallback   func(*LiDARFrame) // callback when frame is complete
	FrameBufferSize int               // max frames to buffer (default: 50 = 5 seconds)
	FrameDuration   time.Duration     // duration per frame (default: 1s based on observed timing)
	BufferTimeout   time.Duration     // how long to wait before finalizing frame (default: 1s)
	CleanupInterval time.Duration     // how often to check for frames to finalize (default: 500ms)
}

// NewFrameBuilder creates a new FrameBuilder with the specified configuration
func NewFrameBuilder(config FrameBuilderConfig) *FrameBuilder {
	// Set reasonable defaults
	if config.FrameBufferSize == 0 {
		config.FrameBufferSize = 50 // buffer 50 frames = 5 seconds at 10 Hz
	}
	if config.FrameDuration == 0 {
		config.FrameDuration = 100 * time.Millisecond // 600 RPM = 10 Hz = 100ms per rotation
	}
	if config.BufferTimeout == 0 {
		config.BufferTimeout = 200 * time.Millisecond // wait 200ms for late packets (2x frame duration)
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 500 * time.Millisecond // check every 500ms
	}

	fb := &FrameBuilder{
		sensorID:        config.SensorID,
		frameCallback:   config.FrameCallback,
		frameBuffer:     make(map[int64]*LiDARFrame),
		frameBufferSize: config.FrameBufferSize,
		frameDuration:   config.FrameDuration,
		bufferTimeout:   config.BufferTimeout,
		cleanupInterval: config.CleanupInterval,
	}

	// Start cleanup timer
	fb.cleanupTimer = time.AfterFunc(fb.cleanupInterval, fb.cleanupFrames)

	return fb
}

// AddPoints adds a slice of points to the appropriate time-based frame slots
// Uses timestamp to determine which frame each point belongs to
func (fb *FrameBuilder) AddPoints(points []Point) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if len(points) == 0 {
		return
	}

	// Process each point and assign to appropriate time slot
	for _, point := range points {
		// Calculate which time slot this point belongs to
		timeSlot := fb.getTimeSlot(point.Timestamp)

		// Get or create frame for this time slot
		frame := fb.getOrCreateFrame(timeSlot, point.Timestamp)

		// Add point to frame
		fb.addPointToFrame(frame, point)
	}
}

// getTimeSlot calculates which time slot a timestamp belongs to
func (fb *FrameBuilder) getTimeSlot(timestamp time.Time) int64 {
	// Convert timestamp to nanoseconds and divide by frame duration
	return timestamp.UnixNano() / fb.frameDuration.Nanoseconds()
}

// getOrCreateFrame gets existing frame or creates new one for time slot
func (fb *FrameBuilder) getOrCreateFrame(timeSlot int64, timestamp time.Time) *LiDARFrame {
	if frame, exists := fb.frameBuffer[timeSlot]; exists {
		return frame
	}

	// Create new frame
	fb.frameCounter++
	frame := &LiDARFrame{
		FrameID:        fmt.Sprintf("%s-frame-%d", fb.sensorID, fb.frameCounter),
		SensorID:       fb.sensorID,
		StartTimestamp: timestamp,
		EndTimestamp:   timestamp,
		Points:         make([]Point, 0, 70000), // pre-allocate for typical frame size
		MinAzimuth:     360.0,                   // will be updated to actual minimum
		MaxAzimuth:     0.0,                     // will be updated to actual maximum
		SpinComplete:   false,
	}

	fb.frameBuffer[timeSlot] = frame

	// Enforce buffer size limit
	if len(fb.frameBuffer) > fb.frameBufferSize {
		fb.evictOldestFrame()
	}

	return frame
}

// addPointToFrame adds a single point to the specified frame
func (fb *FrameBuilder) addPointToFrame(frame *LiDARFrame, point Point) {
	frame.Points = append(frame.Points, point)
	frame.PointCount++

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

// evictOldestFrame removes the oldest frame from buffer and finalizes it
func (fb *FrameBuilder) evictOldestFrame() {
	var oldestSlot int64 = -1
	var oldestFrame *LiDARFrame

	for slot, frame := range fb.frameBuffer {
		if oldestSlot == -1 || slot < oldestSlot {
			oldestSlot = slot
			oldestFrame = frame
		}
	}

	if oldestSlot != -1 {
		delete(fb.frameBuffer, oldestSlot)
		fb.finalizeFrame(oldestFrame)
	}
}

// cleanupFrames periodically checks for frames that should be finalized
func (fb *FrameBuilder) cleanupFrames() {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	now := time.Now()
	var slotsToFinalize []int64

	// Find frames that are old enough to finalize
	for slot, frame := range fb.frameBuffer {
		frameAge := now.Sub(frame.EndTimestamp)
		if frameAge >= fb.bufferTimeout {
			slotsToFinalize = append(slotsToFinalize, slot)
		}
	}

	// Finalize old frames
	for _, slot := range slotsToFinalize {
		frame := fb.frameBuffer[slot]
		delete(fb.frameBuffer, slot)
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

	// Call callback if provided (in separate goroutine to avoid blocking)
	if fb.frameCallback != nil {
		go fb.frameCallback(frame)
	}
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
		FrameDuration:   100 * time.Millisecond, // 600 RPM = 10 Hz = 100ms per rotation
		BufferTimeout:   500 * time.Millisecond, // wait 500ms for late packets (5x frame duration)
		CleanupInterval: 250 * time.Millisecond, // check every 250ms for better responsiveness
	})
}

// exportFrameToASC exports a LiDARFrame to CloudCompare .asc ASCII format
func exportFrameToASC(frame *LiDARFrame) error {
	if frame == nil || len(frame.Points) == 0 {
		return fmt.Errorf("empty frame")
	}

	// Create filename with frame ID and timestamp
	filename := fmt.Sprintf("lidar_frame_%s_%d.asc",
		frame.SensorID,
		frame.StartTimestamp.Unix())
	filePath := filepath.Join("/tmp/lidar", filename)

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create ASC file: %w", err)
	}
	defer file.Close()

	// Write header comment (optional for CloudCompare)
	// fmt.Fprintf(file, "# CloudCompare ASC file\n")
	// fmt.Fprintf(file, "# Frame: %s\n", frame.FrameID)
	// fmt.Fprintf(file, "# Points: %d\n", frame.PointCount)
	// fmt.Fprintf(file, "# Azimuth: %.1f°-%.1f°\n", frame.MinAzimuth, frame.MaxAzimuth)
	// fmt.Fprintf(file, "# Duration: %v\n", frame.EndTimestamp.Sub(frame.StartTimestamp))
	// fmt.Fprintf(file, "# Format: X Y Z Intensity\n")

	// Write points in X Y Z Intensity format
	for _, point := range frame.Points {
		fmt.Fprintf(file, "%.6f %.6f %.6f %d\n",
			point.X, point.Y, point.Z, point.Intensity)
	}

	log.Printf("Exported frame %s to %s (%d points)", frame.FrameID, filePath, frame.PointCount)
	return nil
}
