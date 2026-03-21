package l9endpoints

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// StartVRLogReplay starts VRLOG replay from a FrameReader.
// Frames are published to all connected clients at the specified rate.
func (p *Publisher) StartVRLogReplay(reader FrameReader) error {
	p.vrlogMu.Lock()

	if p.vrlogActive {
		p.vrlogMu.Unlock()
		return fmt.Errorf("VRLOG replay already active")
	}

	p.vrlogReader = reader
	p.vrlogStopCh = make(chan struct{})
	p.vrlogSeekSignal = make(chan struct{}, 1)
	p.vrlogPaused = false
	p.vrlogRate = 1.0
	p.vrlogSendOneFrame = false
	p.vrlogActive = true

	p.vrlogWg.Add(1)

	p.vrlogMu.Unlock()

	// Emit the first background frame AFTER releasing vrlogMu.
	// Publish() → shouldSendBackground() → IsVRLogActive() acquires
	// vrlogMu.RLock(), which would deadlock if the write lock were
	// still held.
	if err := p.emitFirstBackground(reader); err != nil {
		diagf("[Visualiser] emitFirstBackground failed: %v", err)
		p.StopVRLogReplay()
		return fmt.Errorf("emit first background: %w", err)
	}

	go p.vrlogReplayLoop()

	diagf("[Visualiser] Started VRLOG replay: %d total frames", reader.TotalFrames())
	return nil
}

// emitFirstBackground scans the VRLOG for the first background frame and
// publishes it immediately so the client sees the background grid at the
// start of replay.  The reader is reset to frame 0 afterwards.
func (p *Publisher) emitFirstBackground(reader FrameReader) error {
	total := reader.TotalFrames()
	for i := uint64(0); i < total; i++ {
		frame, err := reader.ReadFrame()
		if err != nil {
			break
		}
		if frame.FrameType == FrameTypeBackground {
			if frame.PlaybackInfo == nil {
				frame.PlaybackInfo = &PlaybackInfo{}
			}
			frame.PlaybackInfo.IsLive = false
			frame.PlaybackInfo.Seekable = true
			p.Publish(frame)
			diagf("[Visualiser] Emitted first background frame at index %d", i)
			break
		}
	}
	// Reset to the start so the main replay loop reads from frame 0.
	if err := reader.Seek(0); err != nil {
		return fmt.Errorf("seek to frame 0 after background scan: %w", err)
	}
	return nil
}

// StopVRLogReplay stops the current VRLOG replay.
func (p *Publisher) StopVRLogReplay() {
	p.vrlogMu.Lock()
	if !p.vrlogActive {
		p.vrlogMu.Unlock()
		return
	}
	close(p.vrlogStopCh)
	p.vrlogActive = false
	p.vrlogMu.Unlock()

	p.vrlogWg.Wait()

	p.vrlogMu.Lock()
	if p.vrlogReader != nil {
		p.vrlogReader.Close()
		p.vrlogReader = nil
	}
	p.vrlogMu.Unlock()

	diagf("[Visualiser] Stopped VRLOG replay")
}

// IsVRLogActive returns true if VRLOG replay is currently active.
func (p *Publisher) IsVRLogActive() bool {
	p.vrlogMu.RLock()
	defer p.vrlogMu.RUnlock()
	return p.vrlogActive
}

// VRLogReader returns the current VRLOG reader (nil if not active).
func (p *Publisher) VRLogReader() FrameReader {
	p.vrlogMu.RLock()
	defer p.vrlogMu.RUnlock()
	return p.vrlogReader
}

// SetVRLogPaused sets the paused state for VRLOG replay.
func (p *Publisher) SetVRLogPaused(paused bool) {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()
	p.vrlogPaused = paused
	if p.vrlogReader != nil {
		p.vrlogReader.SetPaused(paused)
	}
}

// SetVRLogRate sets the playback rate for VRLOG replay.
func (p *Publisher) SetVRLogRate(rate float32) {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()
	p.vrlogRate = rate
	if p.vrlogReader != nil {
		p.vrlogReader.SetRate(rate)
	}
}

// SeekVRLog seeks to a specific frame index in VRLOG replay.
// Returns the current frame index after seeking (captured atomically under lock).
func (p *Publisher) SeekVRLog(frameIdx uint64) (uint64, error) {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()

	if p.vrlogReader == nil {
		return 0, fmt.Errorf("VRLOG replay not active")
	}

	if err := p.vrlogReader.Seek(frameIdx); err != nil {
		return 0, fmt.Errorf("seek failed: %w", err)
	}

	currentFrame := p.vrlogReader.CurrentFrame()
	diagf("[Visualiser] SeekVRLog: requested=%d, landed=%d", frameIdx, currentFrame)

	// Drain buffered frames so the client doesn't receive stale
	// pre-seek frames before the new position's data arrives.
	p.drainFrameBuffers()

	// If paused, send one frame so the UI updates to the seeked position
	if p.vrlogPaused {
		p.vrlogSendOneFrame = true
	}

	// Signal the replay loop to reset timing
	select {
	case p.vrlogSeekSignal <- struct{}{}:
	default:
	}

	return currentFrame, nil
}

// SeekVRLogTimestamp seeks to a specific timestamp in VRLOG replay.
// Returns the current frame index after seeking (captured atomically under lock).
func (p *Publisher) SeekVRLogTimestamp(timestampNs int64) (uint64, error) {
	p.vrlogMu.Lock()
	defer p.vrlogMu.Unlock()

	if p.vrlogReader == nil {
		return 0, fmt.Errorf("VRLOG replay not active")
	}

	if err := p.vrlogReader.SeekToTimestamp(timestampNs); err != nil {
		return 0, fmt.Errorf("seek failed: %w", err)
	}

	currentFrame := p.vrlogReader.CurrentFrame()
	diagf("[Visualiser] SeekVRLogTimestamp: requested=%d, landed=%d", timestampNs, currentFrame)

	// Drain buffered frames so the client doesn't receive stale
	// pre-seek frames before the new position's data arrives.
	p.drainFrameBuffers()

	// If paused, send one frame so the UI updates to the seeked position
	if p.vrlogPaused {
		p.vrlogSendOneFrame = true
	}

	// Signal the replay loop to reset timing
	select {
	case p.vrlogSeekSignal <- struct{}{}:
	default:
	}

	return currentFrame, nil
}

// drainFrameBuffers discards all buffered frames from the publisher's
// central frameChan and every per-client channel. Call after seeking to
// prevent stale pre-seek frames from reaching clients.
func (p *Publisher) drainFrameBuffers() {
	// Drain the central broadcast channel.
	for {
		select {
		case f := <-p.frameChan:
			if f.PointCloud != nil {
				f.PointCloud.Release()
			}
		default:
			goto clientDrain
		}
	}

clientDrain:
	// Drain each per-client channel.
	p.clientsMu.RLock()
	defer p.clientsMu.RUnlock()
	for _, client := range p.clients {
		p.drainClientCh(client)
	}
}

// drainClientCh drains a single client's frame channel.
func (p *Publisher) drainClientCh(client *clientStream) {
	for {
		select {
		case f := <-client.frameCh:
			if f.PointCloud != nil {
				f.PointCloud.Release()
			}
		default:
			return
		}
	}
}

// vrlogReplayLoop reads frames from the VRLOG reader and publishes them.
func (p *Publisher) vrlogReplayLoop() {
	defer p.vrlogWg.Done()

	var lastFrameTime int64
	var lastWallTime time.Time

	// Throttle background frames to avoid overwhelming the gRPC stream.
	// Recordings made at slow rates (e.g. 0.1×) may contain many background
	// snapshots that would otherwise replay back-to-back.
	const bgReplayInterval = 10 * time.Second
	var lastBgSentWall time.Time

	for {
		select {
		case <-p.vrlogStopCh:
			return
		case <-p.vrlogSeekSignal:
			// Reset timing after seek
			lastFrameTime = 0
			lastWallTime = time.Time{}
			lastBgSentWall = time.Time{} // Ensure first bg after seek is sent
			// Fall through to check sendOneFrame (don't continue)
		default:
		}

		p.vrlogMu.Lock()
		isPaused := p.vrlogPaused
		rate := p.vrlogRate
		reader := p.vrlogReader
		sendOne := p.vrlogSendOneFrame
		p.vrlogSendOneFrame = false
		p.vrlogMu.Unlock()

		if (isPaused && !sendOne) || reader == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		frame, err := reader.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Pause at EOF instead of stopping — keeps the VRLOG loaded
				// so the client can Seek(0) + Play() to restart.
				diagf("[Visualiser] VRLOG replay complete — pausing at end")
				p.vrlogMu.Lock()
				p.vrlogPaused = true
				if p.vrlogReader != nil {
					p.vrlogReader.SetPaused(true)
				}
				p.vrlogMu.Unlock()
				// Reset timing so restart plays at correct pace.
				lastFrameTime = 0
				lastWallTime = time.Time{}
				continue
			}
			opsf("[Visualiser] VRLOG replay error: %v", err)
			// Clean up replay state asynchronously to prevent deadlock.
			// StopVRLogReplay() waits on vrlogWg, which includes this goroutine.
			go p.StopVRLogReplay()
			return
		}

		// Throttle background frames during replay: send at most one
		// every bgReplayInterval of wall-clock time. Background
		// timestamps use wall-clock time that would corrupt the
		// foreground rate-control state, so handle them separately.
		if frame.FrameType == FrameTypeBackground {
			if !lastBgSentWall.IsZero() && time.Since(lastBgSentWall) < bgReplayInterval {
				continue
			}
			lastBgSentWall = time.Now()
		} else {
			// Rate control: sleep to match playback rate (foreground only)
			if lastFrameTime > 0 && rate > 0 {
				frameDelta := time.Duration(float64(frame.TimestampNanos-lastFrameTime) / float64(rate))
				wallDelta := time.Since(lastWallTime)
				if frameDelta > wallDelta {
					sleepTime := frameDelta - wallDelta
					// Cap sleep to avoid long waits
					if sleepTime > 500*time.Millisecond {
						sleepTime = 500 * time.Millisecond
					}
					time.Sleep(sleepTime)
				}
			}

			lastFrameTime = frame.TimestampNanos
			lastWallTime = time.Now()
		}

		// Mark frame as seekable replay
		if frame.PlaybackInfo == nil {
			frame.PlaybackInfo = &PlaybackInfo{}
		}
		frame.PlaybackInfo.IsLive = false
		frame.PlaybackInfo.Seekable = true

		// Publish to all clients
		p.Publish(frame)
	}
}
