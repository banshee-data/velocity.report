package l2frames

import (
	"fmt"
	"sync"
	"time"
)

// frameCallbackWorker processes frames sequentially from the frameCh channel.
// This ensures that only one frame callback runs at a time, preventing
// concurrent tracker Update() and persistence operations.
// The worker exits when closeCh is closed, after draining any remaining
// frames from frameCh.
func (fb *FrameBuilder) frameCallbackWorker() {
	defer close(fb.frameDone)
	for {
		select {
		case frame := <-fb.frameCh:
			fb.frameCallback(frame)
		case <-fb.closeCh:
			// Drain remaining frames so no sends block.
			for {
				select {
				case frame := <-fb.frameCh:
					fb.frameCallback(frame)
				default:
					return
				}
			}
		}
	}
}

// Close shuts down the frame callback worker and waits for it to drain.
// Must be called when the FrameBuilder is no longer needed to avoid
// goroutine leaks. Close is idempotent — subsequent calls are no-ops.
func (fb *FrameBuilder) Close() {
	fb.mu.Lock()
	if fb.closed {
		fb.mu.Unlock()
		return
	}
	fb.closed = true
	if fb.cleanupTimer != nil {
		fb.cleanupTimer.Stop()
	}
	// Signal shutdown. Any in-flight blocking sends in finalizeFrame
	// will see closeCh and abort. The callback worker will drain
	// remaining frames and exit.
	close(fb.closeCh)
	fb.mu.Unlock()
	if fb.frameCh != nil {
		<-fb.frameDone
	}
}

// DroppedFrames returns the number of frames dropped due to a full
// callback channel. Useful for post-run diagnostics.
func (fb *FrameBuilder) DroppedFrames() uint64 {
	return fb.droppedFrames.Load()
}

// SetBlockOnFrameChannel enables or disables blocking mode for the frame
// callback channel. When true, finalizeFrame blocks until the pipeline
// accepts the frame (true back-pressure). When false (default), frames
// are dropped if the channel is full. Enable for analysis mode to
// ensure every frame is processed; disable for live mode where dropping
// is acceptable to maintain real-time throughput.
func (fb *FrameBuilder) SetBlockOnFrameChannel(block bool) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.blockOnFrameChannel = block
}

// Reset clears all buffered frame state. This should be called when switching
// data sources (e.g., live to PCAP) to prevent stale frames from contaminating
// the new data stream.
func (fb *FrameBuilder) Reset() {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	// Discard current frame in progress
	fb.currentFrame = nil
	fb.lastAzimuth = 0

	// Clear frame buffer
	for k := range fb.frameBuffer {
		delete(fb.frameBuffer, k)
	}

	// Reset sequence tracking
	fb.lastSequence = 0
	for k := range fb.sequenceGaps {
		delete(fb.sequenceGaps, k)
	}
	for k := range fb.pendingPackets {
		delete(fb.pendingPackets, k)
	}

	// Reset dropped frame counter so per-run diagnostics are accurate.
	fb.droppedFrames.Store(0)

	diagf("[FrameBuilder] Reset: cleared all buffered frames and state for sensor=%s", fb.sensorID)
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
	if fb.currentFrame == nil {
		return
	}

	if fb.currentFrame.PointCount < fb.minFramePoints {
		// Discard incomplete frame; log only in debug to reduce noise
		diagf("[FrameBuilder] Discarding incomplete frame %s: points=%d, min_required=%d",
			fb.currentFrame.FrameID, fb.currentFrame.PointCount, fb.minFramePoints)
		fb.currentFrame = nil // Discard incomplete frame
		return
	}

	frame := fb.currentFrame
	fb.currentFrame = nil

	// Calculate completeness metrics
	fb.calculateFrameCompleteness(frame)

	// In blocking mode (analysis replay), bypass the buffer entirely and
	// send the frame directly to the pipeline. The buffer exists for live
	// mode to handle out-of-order backfill packets, but analysis mode
	// processes packets sequentially. Without this shortcut the PCAP
	// reader fills the 10-frame buffer faster than the pipeline drains it,
	// silently overwriting intermediate frames and delivering only ~12%
	// of the rotations.
	if fb.blockOnFrameChannel {
		fb.finalizeFrame(frame, "direct_blocking")
		return
	}

	// Move to buffer for potential backfill
	fb.frameBuffer[frame.FrameID] = frame

	tracef("[FrameBuilder] Moved frame %s to buffer (points=%d); buffer_size=%d",
		frame.FrameID, frame.PointCount, len(fb.frameBuffer))

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
		diagf("[FrameBuilder] Evicting buffered frame: ID=%s, Points=%d, Sensor=%s", oldestFrame.FrameID, oldestFrame.PointCount, oldestFrame.SensorID)
		// Remove from buffer and finalize so the callback is invoked.
		delete(fb.frameBuffer, oldestID)
		// Finalize the frame so the registered callback receives it.
		fb.finalizeFrame(oldestFrame, "buffer_evict")
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

	tracef("[FrameBuilder] cleanupFrames invoked: buffer_size=%d, now=%v", len(fb.frameBuffer), now)

	// Find frames that are old enough to finalize
	for frameID, frame := range fb.frameBuffer {
		ageSource := frame.EndWallTime
		if ageSource.IsZero() {
			ageSource = frame.EndTimestamp
		}
		// Fall back to StartWallTime or StartTimestamp to prevent memory leaks
		// from frames with unset end timestamps (e.g., partially created frames).
		if ageSource.IsZero() {
			ageSource = frame.StartWallTime
		}
		if ageSource.IsZero() {
			ageSource = frame.StartTimestamp
		}
		// If still no valid timestamp, use a large age to force cleanup
		// after buffer timeout. This prevents indefinite memory growth.
		if ageSource.IsZero() {
			// Treat timestamp-less frames as extremely old to ensure cleanup
			frameIDsToFinalize = append(frameIDsToFinalize, frameID)
			continue
		}
		frameAge := now.Sub(ageSource)
		if frameAge >= fb.bufferTimeout {
			frameIDsToFinalize = append(frameIDsToFinalize, frameID)
		}
	}

	// Finalize old frames
	for _, frameID := range frameIDsToFinalize {
		frame := fb.frameBuffer[frameID]
		delete(fb.frameBuffer, frameID)
		fb.finalizeFrame(frame, "buffer_timeout")
	}

	// DEBUG: If a current frame exists but hasn't been moved to buffer (wrap not detected),
	// force-finalize it after a short age so callbacks and buffering can be exercised.
	if fb.currentFrame != nil {
		ageSource := fb.currentFrame.EndWallTime
		if ageSource.IsZero() {
			ageSource = fb.currentFrame.EndTimestamp
		}
		age := now.Sub(ageSource)
		// Use configured buffer timeout as the inactivity threshold to finalize
		// the current frame when no recent points have arrived.
		if age >= fb.bufferTimeout && fb.currentFrame.PointCount > 0 {
			tracef("[FrameBuilder] Finalizing idle current frame ID=%s age=%v points=%d (bufferTimeout=%v)",
				fb.currentFrame.FrameID, age, fb.currentFrame.PointCount, fb.bufferTimeout)
			fb.finalizeCurrentFrame()
		}
	}

	// Schedule next cleanup unless the builder has been closed.
	if !fb.closed {
		fb.cleanupTimer = time.AfterFunc(fb.cleanupInterval, fb.cleanupFrames)
	}
}

// finalizeFrame completes a frame and calls the callback
func (fb *FrameBuilder) finalizeFrame(frame *LiDARFrame, reason string) {
	if frame == nil {
		return
	}

	// lightweight frame-completion logging
	tracef("[FrameBuilder] Frame completed - ID: %s, Points: %d, Azimuth: %.1f°-%.1f°, Duration: %v, Sensor: %s, reason=%s",
		frame.FrameID,
		frame.PointCount,
		frame.MinAzimuth,
		frame.MaxAzimuth,
		frame.EndTimestamp.Sub(frame.StartTimestamp),
		frame.SensorID,
		reason)

	// Determine rotation completeness before export
	coverage := frameAzimuthCoverage(frame)
	spinComplete := coverage >= MinAzimuthCoverage && frame.PointCount >= MinFramePointsForCompletion
	frame.SpinComplete = spinComplete
	coverageGap := 360.0 - coverage

	if !spinComplete || frame.PacketGaps > 0 || coverageGap > 0.5 {
		tracef("[FrameBuilder] Incomplete or gappy frame: id=%s sensor=%s reason=%s cov=%.1f° gap=%.1f° min=%.1f° pts=%d/%d gaps=%d completeness=%.3f duration=%v range=[%.1f,%.1f] start=%s end=%s spin_complete=%v",
			frame.FrameID,
			frame.SensorID,
			reason,
			coverage,
			coverageGap,
			MinAzimuthCoverage,
			frame.PointCount,
			MinFramePointsForCompletion,
			frame.PacketGaps,
			frame.CompletenessRatio,
			frame.EndTimestamp.Sub(frame.StartTimestamp),
			frame.MinAzimuth,
			frame.MaxAzimuth,
			frame.StartTimestamp.UTC().Format(time.RFC3339Nano),
			frame.EndTimestamp.UTC().Format(time.RFC3339Nano),
			spinComplete,
		)
	}

	// Export to ASC if requested (single-shot)
	if fb.exportNextFrameASC {
		if !spinComplete {
			diagf("[FrameBuilder] Skipping export_next_frame: incomplete rotation frame=%s cov=%.1f° points=%d", frame.FrameID, coverage, frame.PointCount)
		} else {
			if err := exportFrameToASCInternal(frame); err != nil {
				opsf("[FrameBuilder] Failed to export next frame for sensor %s: %v", frame.SensorID, err)
			} else {
				diagf("[FrameBuilder] Exported next frame for sensor %s", frame.SensorID)
				fb.exportNextFrameASC = false
			}
		}
	}

	// Export batch of upcoming frames, if queued
	if fb.exportBatchExported < fb.exportBatchCount {
		if !spinComplete {
			diagf("[FrameBuilder] Skipping batch export (%d/%d) incomplete rotation frame=%s cov=%.1f° points=%d", fb.exportBatchExported+1, fb.exportBatchCount, frame.FrameID, coverage, frame.PointCount)
		} else {
			if err := exportFrameToASCInternal(frame); err != nil {
				opsf("[FrameBuilder] Failed to export batch frame %d/%d for sensor %s: %v", fb.exportBatchExported+1, fb.exportBatchCount, frame.SensorID, err)
			} else {
				diagf("[FrameBuilder] Exported batch frame %d/%d for sensor %s", fb.exportBatchExported+1, fb.exportBatchCount, frame.SensorID)
			}
			fb.exportBatchExported++
			if fb.exportBatchExported >= fb.exportBatchCount {
				fb.exportBatchCount = 0
				fb.exportBatchExported = 0
			}
		}
	}
	// Call callback if provided (via serialised channel to avoid concurrent pipeline runs)
	if fb.frameCallback != nil && fb.frameCh != nil && !fb.closed {
		tracef("[FrameBuilder] Invoking frame callback for ID=%s, Points=%d, Sensor=%s",
			frame.FrameID, frame.PointCount, frame.SensorID)
		if fb.blockOnFrameChannel {
			// Blocking mode (PCAP fast): wait for pipeline to accept the
			// frame, providing true back-pressure to the packet reader.
			// This prevents frame drops during analysis runs where every
			// frame must be processed.
			//
			// Release fb.mu before the blocking send to avoid holding the
			// lock while waiting. Without this, a full channel deadlocks
			// operations that need fb.mu (cleanup timer, Reset, source
			// switching) and prevents PCAP cancellation from progressing.
			// Safe because the callback worker does not acquire fb.mu, and
			// all finalizeFrame work is complete before this point.
			//
			// Use select with closeCh so that Close() can shut down
			// without racing against this send. Close() closes closeCh
			// (under fb.mu) before closing frameCh, so a concurrent
			// Close will unblock the select here instead of panicking
			// on a closed frameCh.
			closeCh := fb.closeCh
			fb.mu.Unlock()
			select {
			case fb.frameCh <- frame:
			case <-closeCh:
				// Shutdown in progress — drop the frame.
				count := fb.droppedFrames.Add(1)
				opsf("[FrameBuilder] Dropped frame %s during shutdown (total dropped: %d)", frame.FrameID, count)
			}
			fb.mu.Lock()
		} else {
			select {
			case fb.frameCh <- frame:
			default:
				// Channel full — drop frame to avoid blocking frame assembly.
				// This handles back-pressure when the tracking pipeline cannot
				// keep up with frame arrival rate.
				count := fb.droppedFrames.Add(1)
				opsf("[FrameBuilder] Dropped frame %s: callback queue full (total dropped: %d)", frame.FrameID, count)
			}
		}
	}
}

// RequestExportNextFrameASC schedules export of the next completed frame to ASC format.
// The export path is generated internally for security.
func (fb *FrameBuilder) RequestExportNextFrameASC() {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.exportNextFrameASC = true
}

// RequestExportFrameBatchASC schedules export of the next N completed frames.
// Export paths are generated internally for security.
func (fb *FrameBuilder) RequestExportFrameBatchASC(count int) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if count <= 0 {
		count = 5 // default to 5 frames
	}

	fb.exportBatchCount = count
	fb.exportBatchExported = 0
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
			tracef("Frame completed - ID: %s, Points: %d, Azimuth: %.1f°-%.1f°, Duration: %v, Sensor: %s",
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
					opsf("Failed to export frame %s: %v", frame.FrameID, err)
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
	return exportFrameToASCInternal(frame)
}

// exportFrameToASCInternal writes a LiDARFrame to ASC. The path is generated internally.
func exportFrameToASCInternal(frame *LiDARFrame) error {
	if frame == nil || len(frame.Points) == 0 {
		return fmt.Errorf("empty frame")
	}

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
		diagf("[FrameBuilder] all Z==0 for frame %s; recomputing XYZ from polar data before export", frame.FrameID)
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
	actualPath, err := ExportPointsToASC(ascPoints, extraHeader)
	if err != nil {
		return fmt.Errorf("failed to export ASC: %w", err)
	}
	diagf("Exported frame %s to %s (%d points)", frame.FrameID, actualPath, frame.PointCount)
	return nil
}
