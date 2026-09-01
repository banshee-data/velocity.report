package l9endpoints

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Ensure Server implements the gRPC interface.
var _ pb.VisualiserServiceServer = (*Server)(nil)

// overlayPreferences stores per-client overlay preferences.
type overlayPreferences struct {
	showPoints      bool
	showClusters    bool
	showTracks      bool
	showTrails      bool
	showVelocity    bool
	showGating      bool
	showAssociation bool
	showResiduals   bool
}

// Server implements the VisualiserService gRPC server.
type Server struct {
	pb.UnimplementedVisualiserServiceServer

	publisher *Publisher

	// Synthetic mode
	syntheticMode bool
	syntheticGen  *SyntheticGenerator

	// Playback state — used by PCAP and replay modes.
	// In PCAP mode, pause/play are honoured at the stream level
	// (frames are silently dropped while paused).
	// Protected by playbackMu for concurrent access.
	paused       bool
	playbackRate float32
	replayMode   bool // True when replaying a PCAP or log (not live sensor)
	vrlogMode    bool // True when replaying a VRLOG (seekable replay)
	seekPending  bool // True after a seek; allows one frame through even while paused
	playbackMu   sync.RWMutex

	// PCAP progress tracking (updated by WebServer progress callback)
	pcapCurrentPacket uint64
	pcapTotalPackets  uint64
	pcapStartNs       int64
	pcapEndNs         int64
	replayEpoch       uint64 // monotonically increasing; bumped on each new replay load

	// sourceModeProvider pulls the canonical source mode and recording flag
	// from whoever owns them — the monitor server. It is a pull rather than a
	// stored copy on purpose: mirroring the mode here is exactly what let this
	// layer and the monitor server disagree about what was playing.
	sourceModeProvider func() (mode string, recording bool)

	// settlingProvider pulls background settling progress. It comes from the
	// grid rather than the pipeline state, so it is wired separately.
	settlingProvider func() (settling bool, elapsedSecs float32)

	// sensorSilentProvider reports live input with no packets arriving. Frames
	// keep flowing while a sensor is silent, so this cannot be inferred from
	// frame arrival.
	sensorSilentProvider func() bool

	// Per-client overlay preferences (protected by preferenceMu)
	clientPreferences map[string]*overlayPreferences
	preferenceMu      sync.RWMutex
}

// SetSourceModeProvider wires the authoritative source-mode lookup. Without
// one, streamed frames carry SOURCE_MODE_UNSPECIFIED rather than guessing a
// source the monitor server has not reported.
func (s *Server) SetSourceModeProvider(fn func() (string, bool)) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	s.sourceModeProvider = fn
}

// currentSourceMode returns the canonical source mode and recording flag,
// or ("", false) when no provider is wired.
func (s *Server) currentSourceMode() (string, bool) {
	s.playbackMu.RLock()
	fn := s.sourceModeProvider
	s.playbackMu.RUnlock()
	if fn == nil {
		return "", false
	}
	return fn()
}

// SetSensorSilentProvider wires the live-input presence lookup.
func (s *Server) SetSensorSilentProvider(fn func() bool) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	s.sensorSilentProvider = fn
}

// currentSensorSilent reports whether live input has gone quiet.
func (s *Server) currentSensorSilent() bool {
	s.playbackMu.RLock()
	fn := s.sensorSilentProvider
	s.playbackMu.RUnlock()
	if fn == nil {
		return false
	}
	return fn()
}

// SetSettlingProvider wires the background settling lookup.
func (s *Server) SetSettlingProvider(fn func() (bool, float32)) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	s.settlingProvider = fn
}

// currentSettling reports whether the background grid is still settling.
func (s *Server) currentSettling() (bool, float32) {
	s.playbackMu.RLock()
	fn := s.settlingProvider
	s.playbackMu.RUnlock()
	if fn == nil {
		return false, 0
	}
	return fn()
}

// decoratePlaybackInfo adds the server-owned playback state to a frame before
// it is serialised. Keeping the composition in one testable step matters: the
// source, settling state and sensor-presence flag come from three providers,
// but they form one wire contract and must agree on which source they describe.
func (s *Server) decoratePlaybackInfo(frame *FrameBundle) {
	// Inject PlaybackInfo for replay mode (PCAP) if not already set.
	if s.replayMode && frame.PlaybackInfo == nil {
		s.playbackMu.RLock()
		frame.PlaybackInfo = &PlaybackInfo{
			LogStartNs:        s.pcapStartNs,
			LogEndNs:          s.pcapEndNs,
			PlaybackRate:      s.playbackRate,
			Paused:            s.paused,
			CurrentFrameIndex: s.pcapCurrentPacket,
			TotalFrames:       s.pcapTotalPackets,
			Seekable:          false,
			ReplayEpoch:       s.replayEpoch,
		}
		s.playbackMu.RUnlock()
	}

	// Stamp epoch on existing PlaybackInfo (for example, from a VRLOG recorder).
	if s.replayMode && frame.PlaybackInfo != nil && frame.PlaybackInfo.ReplayEpoch == 0 {
		s.playbackMu.RLock()
		frame.PlaybackInfo.ReplayEpoch = s.replayEpoch
		s.playbackMu.RUnlock()
	}

	// Stamp the source on every frame, live included. Live frames otherwise
	// carry no PlaybackInfo, so create one rather than make the client infer it.
	mode, recording := s.currentSourceMode()
	if mode != "" {
		if frame.PlaybackInfo == nil {
			frame.PlaybackInfo = &PlaybackInfo{PlaybackRate: 1.0}
		}
		frame.PlaybackInfo.SourceMode = mode
		frame.PlaybackInfo.Recording = recording
	}

	// Settling and silence describe current live input. Set both
	// deterministically rather than only setting their true cases: a VRLOG
	// frame may contain the values recorded while it was live, and replaying
	// that frame must not resurrect an old SETTLING or IDLE badge.
	if frame.PlaybackInfo != nil {
		if mode == "live" {
			settling, elapsedSecs := s.currentSettling()
			frame.PlaybackInfo.Settling = settling
			frame.PlaybackInfo.SettlingElapsedSecs = elapsedSecs
			frame.PlaybackInfo.SensorSilent = s.currentSensorSilent()
		} else if mode != "" {
			frame.PlaybackInfo.Settling = false
			frame.PlaybackInfo.SettlingElapsedSecs = 0
			frame.PlaybackInfo.SensorSilent = false
		}
	}
}

// NewServer creates a new gRPC server.
func NewServer(publisher *Publisher) *Server {
	return &Server{
		publisher:         publisher,
		playbackRate:      1.0,
		clientPreferences: make(map[string]*overlayPreferences),
	}
}

// EnableSyntheticMode enables synthetic data generation.
func (s *Server) EnableSyntheticMode(sensorID string) {
	s.syntheticMode = true
	s.syntheticGen = NewSyntheticGenerator(sensorID)
}

// SetReplayMode marks the server as replaying recorded data (PCAP or log).
// When in replay mode, PlaybackInfo is injected into streamed frames and
// the client UI shows "REPLAY" instead of "LIVE".
func (s *Server) SetReplayMode(enabled bool) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	wasEnabled := s.replayMode
	s.replayMode = enabled
	if enabled && !wasEnabled {
		s.replayEpoch++
	} else if !enabled {
		s.pcapCurrentPacket = 0
		s.pcapTotalPackets = 0
		s.pcapStartNs = 0
		s.pcapEndNs = 0
	}
}

// SetVRLogMode marks the server as replaying a VRLOG file (seekable replay).
// This also sets replayMode to true. In VRLOG mode, pause/play/seek/rate
// commands are delegated to the publisher's VRLOG replay loop.
func (s *Server) SetVRLogMode(enabled bool) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	wasReplay := s.replayMode
	s.vrlogMode = enabled
	if enabled {
		s.replayMode = true
		if !wasReplay {
			s.replayEpoch++
		}
		// Reset pause state so the new VRLOG replay starts playing
		// immediately.  Without this, a previous Pause() RPC leaves
		// s.paused=true and streamFromPublisher silently drops every
		// frame, resulting in frames_sent=0 for connected clients.
		s.paused = false
	}
}

// sendStallTimeout bounds a single stream.Send. It is deliberately far longer
// than any legitimate hiccup — at ten frames a second it is fifty frames of
// slack — because the cost of ending a stream early is a reconnect, while the
// cost of not bounding it is an indefinite stall that the send instrumentation
// cannot even report.
// sendStallTimeout is how long a single stream.Send may make no progress
// before it is reported. It is a reporting threshold, not a deadline: the send
// is still waited for. See the stall handling in streamFromPublisher.
//
// A var rather than a const so tests can shorten it; nothing else reassigns it.
var sendStallTimeout = 5 * time.Second

// currentReplayEpoch returns the epoch that identifies the current source.
// It is bumped whenever a replay is loaded, so a change means frame IDs from
// here on belong to a different sequence than the ones before it.
func (s *Server) currentReplayEpoch() uint64 {
	s.playbackMu.RLock()
	defer s.playbackMu.RUnlock()
	return s.replayEpoch
}

// SetPCAPProgress updates the current packet position for seek-bar display.
func (s *Server) SetPCAPProgress(currentPacket, totalPackets uint64) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	s.pcapCurrentPacket = currentPacket
	s.pcapTotalPackets = totalPackets
}

// SetPCAPTimestamps stores the first and last capture timestamps from pre-counting.
func (s *Server) SetPCAPTimestamps(startNs, endNs int64) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	s.pcapStartNs = startNs
	s.pcapEndNs = endNs
}

// SyntheticGenerator returns the synthetic generator (if enabled).
func (s *Server) SyntheticGenerator() *SyntheticGenerator {
	return s.syntheticGen
}

// StreamFrames implements the streaming RPC for frame data.
func (s *Server) StreamFrames(req *pb.StreamRequest, stream pb.VisualiserService_StreamFramesServer) error {
	diagf("[gRPC] *** NEW CLIENT CONNECTED ***")
	diagf("[gRPC] StreamFrames started: sensor=%s points=%v clusters=%v tracks=%v",
		req.SensorId, req.IncludePoints, req.IncludeClusters, req.IncludeTracks)

	ctx := stream.Context()

	// If synthetic mode, generate and stream synthetic data
	if s.syntheticMode {
		return s.streamSynthetic(ctx, req, stream)
	}

	// Otherwise, stream from publisher
	return s.streamFromPublisher(ctx, req, stream)
}

// streamSynthetic generates and streams synthetic data.
func (s *Server) streamSynthetic(ctx context.Context, req *pb.StreamRequest, stream pb.VisualiserService_StreamFramesServer) error {
	frameInterval := time.Duration(float64(time.Second) / s.syntheticGen.FrameRate)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			diagf("[gRPC] StreamFrames cancelled")
			return ctx.Err()
		case <-ticker.C:
			s.playbackMu.RLock()
			paused := s.paused
			s.playbackMu.RUnlock()
			if paused {
				continue
			}

			frame := s.syntheticGen.NextFrame()
			pbFrame := frameBundleToProto(frame, req)

			if err := stream.Send(pbFrame); err != nil {
				opsf("[gRPC] Send error: %v", err)
				return err
			}
		}
	}
}

// sendCooldown implements hysteresis-based frame skip control (§7.3).
//
// After entering skip mode (consecutive slow sends >= maxSlow), it requires
// minFast consecutive fast sends before exiting skip mode. This prevents
// oscillation when a client hovers around the slow send threshold.
type sendCooldown struct {
	maxSlow  int // Consecutive slow sends to enter skip mode
	minFast  int // Consecutive fast sends to exit skip mode
	slowRun  int // Current consecutive slow sends
	fastRun  int // Current consecutive fast sends
	skipping bool
}

// newSendCooldown creates a sendCooldown with the given thresholds.
func newSendCooldown(maxSlow, minFast int) *sendCooldown {
	return &sendCooldown{maxSlow: maxSlow, minFast: minFast}
}

// recordSlow records a slow send. Returns true if now in skip mode.
func (sc *sendCooldown) recordSlow() bool {
	sc.slowRun++
	sc.fastRun = 0
	if sc.slowRun >= sc.maxSlow {
		sc.skipping = true
	}
	return sc.skipping
}

// recordFast records a fast send. Returns true if still in skip mode.
func (sc *sendCooldown) recordFast() bool {
	sc.fastRun++
	if sc.skipping {
		if sc.fastRun >= sc.minFast {
			sc.slowRun = 0
			sc.fastRun = 0
			sc.skipping = false
		}
	} else {
		sc.slowRun = 0
	}
	return sc.skipping
}

// inSkipMode returns true if the cooldown is currently in skip mode.
func (sc *sendCooldown) inSkipMode() bool {
	return sc.skipping
}

// coalesceBufferedFrames drains queued frames and keeps only the newest one.
// This is used for replay catch-up so we stop serialising stale frames when
// the client is already behind. Any discarded point clouds are released.
//
// Background frames are never coalesced away. A foreground frame can be dropped
// because a later one supersedes it, but a background frame carries scene state
// no later frame reproduces: the client renders whatever background it last
// received until another arrives. Discarding one during a source change left the
// previous source's settled grid on screen underneath the new source's
// foreground, for as long as it took another background frame to survive — which
// is why coalescing has to stop at one rather than skip past it.
func coalesceBufferedFrames(frameCh chan *FrameBundle, frame *FrameBundle) (*FrameBundle, int) {
	if frame != nil && frame.FrameType == FrameTypeBackground {
		return frame, 0
	}

	skipped := 0
	for len(frameCh) > 0 {
		select {
		case newerFrame := <-frameCh:
			if frame != nil && frame.PointCloud != nil {
				frame.PointCloud.Release()
			}
			frame = newerFrame
			skipped++
			if frame != nil && frame.FrameType == FrameTypeBackground {
				return frame, skipped
			}
		default:
			return frame, skipped
		}
	}
	return frame, skipped
}

// streamFromPublisher streams frames from the publisher.
func (s *Server) streamFromPublisher(ctx context.Context, req *pb.StreamRequest, stream pb.VisualiserService_StreamFramesServer) error {
	// Create a unique client ID
	clientID := fmt.Sprintf("grpc-%d", time.Now().UnixNano())

	// Cumulative volume on this stream, for diagnosing a blocked send. An
	// HTTP/2 connection window opens at 65535 bytes and grows only by
	// WINDOW_UPDATE, so where a stall begins says whether it is flow control.
	var bytesSentOnStream int64
	var framesSentOnStream int64

	// Subscribe through the publisher rather than registering here. Doing it
	// inline duplicated addClient and so skipped what it does beyond
	// registration — handing the new client the current background. A replay
	// whose recording carries its background early looked fine; one carrying it
	// at frame 116 drew foreground over an empty grid until the next refresh.
	client := s.publisher.addClient(clientID, req)
	frameCh := client.frameCh

	lidar.Diagf("[gRPC] Client %s subscribed: points=%v clusters=%v tracks=%v",
		clientID, req.IncludePoints, req.IncludeClusters, req.IncludeTracks)

	defer func() {
		s.publisher.removeClient(clientID)
	}()

	// Tracking for performance logging
	var framesSent uint64
	var totalSendTimeNs int64
	var slowSends int
	var droppedFrames uint64
	lastLogTime := time.Now()
	const logInterval = 5 * time.Second
	const slowSendThresholdMs = 50    // Warn if Send() takes > 50ms
	const sendTimeoutMs = 100         // Log a send slower than this at trace level
	const maxConsecutiveSlowSends = 3 // After 3 slow sends, start skipping
	const minConsecutiveFastSends = 5 // Require 5 fast sends before exiting skip mode (hysteresis)

	// Track message sizes for bandwidth estimation
	var totalBytesSent int64
	cooldown := newSendCooldown(maxConsecutiveSlowSends, minConsecutiveFastSends)
	var lastFrameID uint64
	// Frame IDs are only comparable within one source. A live stream and a
	// recording number their frames independently, so the switch between them
	// is a discontinuity, not a gap. lastEpoch detects the switch; see the gap
	// accounting below.
	var lastEpoch uint64
	var totalFramesSent uint64

	for {
		select {
		case <-ctx.Done():
			// framesSent, slowSends and totalSendTimeNs are reset by the
			// periodic stats block below, so on their own they describe the
			// last partial interval, not the connection. droppedFrames is
			// cumulative. Reporting the two side by side read as a lifetime
			// total and made a healthy client look starved, so name the
			// interval ones and carry a genuine lifetime count alongside.
			lidar.Diagf("[gRPC] Client %s disconnected: frames_sent_total=%d frames_sent_last_interval=%d dropped_total=%d slow_sends_last_interval=%d avg_send_time_ms=%.2f",
				clientID, totalFramesSent+framesSent, framesSent, droppedFrames, slowSends, float64(totalSendTimeNs)/float64(max(framesSent, 1))/1e6)
			return ctx.Err()
		case frame := <-frameCh:
			// Respect pause state — drop frames silently while paused,
			// UNLESS a seek is pending (deliver the seeked frame).
			// M7: Release the retained reference since we won't process this frame.
			s.playbackMu.RLock()
			paused := s.paused
			seekPending := s.seekPending
			s.playbackMu.RUnlock()
			if paused && !seekPending {
				if frame.PointCloud != nil {
					frame.PointCloud.Release()
				}
				continue
			}
			// Clear seek pending after delivering the seeked frame
			if seekPending {
				s.playbackMu.Lock()
				s.seekPending = false
				s.playbackMu.Unlock()
			}

			// When replaying (VRLOG) or in cooldown skip mode, coalesce to
			// the newest buffered frame so the client catches up quickly and
			// the channel stays drained. In normal live mode, preserve smooth
			// delivery when the client can keep up.
			skipped := 0
			if s.vrlogMode || cooldown.inSkipMode() {
				if len(frameCh) > 0 {
					frame, skipped = coalesceBufferedFrames(frameCh, frame)
				}
			}
			if skipped > 0 {
				droppedFrames += uint64(skipped)
				lidar.Tracef("[gRPC] Client %s: skipped %d frames to catch up (skip_mode=%v)",
					clientID, skipped, cooldown.inSkipMode())
			}

			// Track frame ID gaps for detecting frames dropped before they reached
			// this stream (for example, in the publisher when the client queue is full).
			// Local catch-up skips are already counted above, so exclude them here.
			//
			// A change of replay epoch means the source changed, and the new
			// source numbers its frames from its own sequence: a live stream at
			// frame 500 followed by a recording starting at frame 6400 is not
			// 5900 losses. Restart the comparison instead of measuring across
			// the discontinuity.
			epoch := s.currentReplayEpoch()
			if epoch != lastEpoch {
				lastFrameID = 0
				lastEpoch = epoch
			}
			if lastFrameID > 0 && frame.FrameID > lastFrameID+1 {
				gap := frame.FrameID - lastFrameID - 1
				if skippedGap := uint64(skipped); gap > skippedGap {
					droppedFrames += gap - skippedGap
				}
			}
			lastFrameID = frame.FrameID

			s.decoratePlaybackInfo(frame)

			// Measure serialisation and send time
			sendStart := time.Now()
			pbFrame := frameBundleToProto(frame, req)

			// Measure serialised message size
			msgSize := proto.Size(pbFrame)
			totalBytesSent += int64(msgSize)

			// stream.Send blocks for as long as the client declines to read:
			// gRPC flow control stalls the write and the call carries no
			// deadline of its own. Every piece of send instrumentation below
			// runs after Send returns, so such a stall is not merely unbounded
			// but invisible — on 2026-08-26 a visualiser stopped reading for
			// four minutes and produced no slow-send log at all, only the
			// publisher discarding every frame into a queue nobody was
			// emptying.
			//
			// Bound it. Send runs on its own goroutine so this loop can give up
			// on it; the goroutine owns the PointCloud release because
			// frameBundleToProto aliases the point slices into pbFrame rather
			// than copying them, so they must stay alive until Send has
			// finished marshalling on every path, including the one where we
			// have already stopped waiting.
			sendResult := make(chan error, 1)
			go func(pc *PointCloudFrame) {
				sendErr := stream.Send(pbFrame)
				if pc != nil {
					pc.Release()
				}
				sendResult <- sendErr
			}(frame.PointCloud)

			// Wait for the send, reporting a stall rather than acting on it.
			//
			// Severing the stream here was tried and made things worse. A
			// client that has stopped reading is usually one whose UI thread is
			// blocked, and a client in that state cannot run its reconnect
			// logic either: on 2026-08-26 closing the stream after five seconds
			// left the replay streaming to nobody for the remaining two and a
			// half minutes, where previously the same client had recovered on
			// its own after four. A genuinely departed client is already
			// covered — its context is cancelled and the case below fires.
			//
			// So the timeout exists purely to make the stall visible, which is
			// what was missing: every other piece of send instrumentation runs
			// after Send returns, so a stalled send produced no diagnostic at
			// all.
			var sendErr error
			waitingSince := time.Now()
			reportedStall := false
		awaitSend:
			for {
				select {
				case sendErr = <-sendResult:
					if sendErr == nil {
						bytesSentOnStream += int64(msgSize)
						framesSentOnStream++
					}
					if reportedStall {
						opsf("[gRPC] Client %s resumed reading after %v",
							clientID, time.Since(waitingSince).Round(time.Millisecond))
					}
					break awaitSend
				case <-ctx.Done():
					// The stream is going away; the pending Send unblocks as it
					// is torn down and its goroutine releases the frame.
					return ctx.Err()
				case <-time.After(sendStallTimeout):
					reportedStall = true
					// Name the frame. A stall is nearly always one specific
					// message the client cannot digest, and without its
					// identity the log says only that something is stuck.
					// Cumulative bytes matter as much as this frame's size. An
					// HTTP/2 connection window opens at 65535 bytes and grows
					// only by WINDOW_UPDATE, so a stall that always begins near
					// a round multiple of that is flow control rather than a
					// slow client.
					opsf("[gRPC] Client %s has not read for %v: blocked sending frame %d (type=%v points=%d msg=%.1fKB, %.1fKB sent on this stream in %d frames), frames are being dropped for it",
						clientID, time.Since(waitingSince).Round(time.Second),
						frame.FrameID, frame.FrameType, framePointCount(frame), float64(msgSize)/1024,
						float64(bytesSentOnStream)/1024, framesSentOnStream)
				}
			}
			if sendErr != nil {
				opsf("[gRPC] Send error for client %s after %d frames: %v", clientID, framesSent, sendErr)
				return sendErr
			}
			sendDuration := time.Since(sendStart)
			totalSendTimeNs += sendDuration.Nanoseconds()
			framesSent++

			// Track slow sends with message size info — hysteresis cooldown (§7.3)
			// After entering skip mode, require minConsecutiveFastSends before exiting
			// to prevent oscillation between skip and normal modes.
			if sendDuration.Milliseconds() > slowSendThresholdMs {
				slowSends++
				wasSkipping := cooldown.inSkipMode()
				cooldown.recordSlow()
				if !wasSkipping && cooldown.inSkipMode() {
					lidar.Opsf("[gRPC] Client %s entering skip mode after %d slow sends (send=%v points=%d)",
						clientID, slowSends, sendDuration, getPointCount(frame))
				}
				if sendDuration.Milliseconds() > sendTimeoutMs {
					tracef("[gRPC] SLOW SEND: client=%s frame=%d duration=%v points=%d msg_size_kb=%.1f skip_mode=%v",
						clientID, frame.FrameID, sendDuration, getPointCount(frame), float64(msgSize)/1024, cooldown.inSkipMode())
				}
			} else {
				wasSkipping := cooldown.inSkipMode()
				cooldown.recordFast()
				if wasSkipping && !cooldown.inSkipMode() {
					lidar.Opsf("[gRPC] Client %s exiting skip mode after %d consecutive fast sends",
						clientID, minConsecutiveFastSends)
				}
			}

			// Periodic performance logging
			if time.Since(lastLogTime) >= logInterval {
				avgSendMs := float64(totalSendTimeNs) / float64(framesSent) / 1e6
				fps := float64(framesSent) / time.Since(lastLogTime).Seconds()
				queueDepth := len(frameCh)
				bandwidthMbps := float64(totalBytesSent) * 8 / time.Since(lastLogTime).Seconds() / 1e6
				avgMsgSizeKB := float64(totalBytesSent) / float64(max(framesSent, 1)) / 1024
				lidar.Tracef("[gRPC] Client %s stats: fps=%.1f frames=%d dropped=%d queue=%d/10 avg_send_ms=%.2f slow_sends=%d bandwidth_mbps=%.1f avg_msg_kb=%.1f",
					clientID, fps, framesSent, droppedFrames, queueDepth, avgSendMs, slowSends, bandwidthMbps, avgMsgSizeKB)

				// Check for queue backup
				if queueDepth > 5 {
					opsf("[gRPC] WARNING: Client %s queue backing up: %d/10 frames buffered", clientID, queueDepth)
				}

				// Reset counters for next interval, carrying the lifetime total
				// so the disconnect line can report both.
				totalFramesSent += framesSent
				framesSent = 0
				totalSendTimeNs = 0
				slowSends = 0
				totalBytesSent = 0
				lastLogTime = time.Now()
			}
		}
	}
}

// framePointCount reports how many points a frame carries, from wherever it
// keeps them. A background frame has no PointCloud — its points live in the
// snapshot — so counting only the cloud reports a large background as empty.
func framePointCount(frame *FrameBundle) int {
	if n := getPointCount(frame); n > 0 {
		return n
	}
	if frame != nil && frame.Background != nil {
		return len(frame.Background.X)
	}
	return 0
}

// getPointCount safely extracts point count from a frame bundle.
func getPointCount(frame *FrameBundle) int {
	if frame != nil && frame.PointCloud != nil {
		return frame.PointCloud.PointCount
	}
	return 0
}

// Pause pauses playback (replay mode).
func (s *Server) Pause(ctx context.Context, req *pb.PauseRequest) (*pb.PlaybackStatus, error) {
	s.playbackMu.Lock()
	isVRLog := s.vrlogMode
	s.paused = true
	rate := s.playbackRate
	s.playbackMu.Unlock()

	// In VRLOG mode, delegate to publisher
	if isVRLog && s.publisher != nil {
		s.publisher.SetVRLogPaused(true)
	}

	return &pb.PlaybackStatus{
		Paused: true,
		Rate:   rate,
	}, nil
}

// Play resumes playback (replay mode).
func (s *Server) Play(ctx context.Context, req *pb.PlayRequest) (*pb.PlaybackStatus, error) {
	s.playbackMu.Lock()
	isVRLog := s.vrlogMode
	s.paused = false
	rate := s.playbackRate
	s.playbackMu.Unlock()

	// In VRLOG mode, delegate to publisher
	if isVRLog && s.publisher != nil {
		s.publisher.SetVRLogPaused(false)
	}

	return &pb.PlaybackStatus{
		Paused: false,
		Rate:   rate,
	}, nil
}

// Seek seeks to a specific timestamp or frame (replay mode).
func (s *Server) Seek(ctx context.Context, req *pb.SeekRequest) (*pb.PlaybackStatus, error) {
	s.playbackMu.RLock()
	isVRLog := s.vrlogMode
	paused := s.paused
	rate := s.playbackRate
	s.playbackMu.RUnlock()

	// In VRLOG mode, delegate to publisher
	if isVRLog && s.publisher != nil {
		var err error
		var currentFrame uint64

		switch target := req.Target.(type) {
		case *pb.SeekRequest_TimestampNs:
			currentFrame, err = s.publisher.SeekVRLogTimestamp(target.TimestampNs)
		case *pb.SeekRequest_FrameId:
			currentFrame, err = s.publisher.SeekVRLog(target.FrameId)
		default:
			return nil, status.Error(codes.InvalidArgument, "seek target not specified")
		}

		if err != nil {
			return nil, status.Errorf(codes.Internal, "seek failed: %v", err)
		}

		// Mark seek pending so the frame is delivered even while paused
		s.playbackMu.Lock()
		s.seekPending = true
		s.playbackMu.Unlock()

		return &pb.PlaybackStatus{
			Paused:         paused,
			Rate:           rate,
			CurrentFrameId: currentFrame,
		}, nil
	}

	return nil, status.Error(codes.Unimplemented, "seek not supported in current mode")
}

// SetRate sets the playback rate.
func (s *Server) SetRate(ctx context.Context, req *pb.SetRateRequest) (*pb.PlaybackStatus, error) {
	s.playbackMu.Lock()
	isVRLog := s.vrlogMode
	s.playbackRate = req.Rate
	paused := s.paused
	rate := s.playbackRate
	s.playbackMu.Unlock()

	// In VRLOG mode, delegate to publisher
	if isVRLog && s.publisher != nil {
		s.publisher.SetVRLogRate(req.Rate)
	}

	return &pb.PlaybackStatus{
		Paused: paused,
		Rate:   rate,
	}, nil
}

// SetOverlayModes configures which overlays to emit for the requesting client.
func (s *Server) SetOverlayModes(ctx context.Context, req *pb.OverlayModeRequest) (*pb.OverlayModeResponse, error) {
	// Extract client ID from context (for future per-client preferences)
	// For now, store global preferences that apply to all clients
	// TODO: Extract client ID from gRPC metadata for per-client preferences

	prefs := &overlayPreferences{
		showPoints:      req.ShowPoints,
		showClusters:    req.ShowClusters,
		showTracks:      req.ShowTracks,
		showTrails:      req.ShowTrails,
		showVelocity:    req.ShowVelocity,
		showGating:      req.ShowGating,
		showAssociation: req.ShowAssociation,
		showResiduals:   req.ShowResiduals,
	}

	// Store preferences (use "default" as global key for now)
	s.preferenceMu.Lock()
	s.clientPreferences["default"] = prefs
	s.preferenceMu.Unlock()

	diagf("[gRPC] Overlay modes updated: points=%v clusters=%v tracks=%v trails=%v velocity=%v gating=%v association=%v residuals=%v",
		prefs.showPoints, prefs.showClusters, prefs.showTracks, prefs.showTrails,
		prefs.showVelocity, prefs.showGating, prefs.showAssociation, prefs.showResiduals)

	return &pb.OverlayModeResponse{Success: true}, nil
}

// GetCapabilities returns server capabilities.
func (s *Server) GetCapabilities(ctx context.Context, req *pb.CapabilitiesRequest) (*pb.CapabilitiesResponse, error) {
	return &pb.CapabilitiesResponse{
		SupportsPoints:    true,
		SupportsClusters:  true,
		SupportsTracks:    true,
		SupportsDebug:     true,
		SupportsReplay:    true,
		SupportsRecording: true,
		AvailableSensors:  []string{s.publisher.config.SensorID},
	}, nil
}

// RegisterService registers the gRPC service with the server.
func RegisterService(grpcServer *grpc.Server, server *Server) {
	pb.RegisterVisualiserServiceServer(grpcServer, server)
}

// StartRecording starts recording frames to disk.
func (s *Server) StartRecording(ctx context.Context, req *pb.RecordingRequest) (*pb.RecordingStatus, error) {
	return nil, status.Error(codes.Unimplemented, "recording not yet supported")
}

// StopRecording stops recording.
func (s *Server) StopRecording(ctx context.Context, req *pb.RecordingRequest) (*pb.RecordingStatus, error) {
	return nil, status.Error(codes.Unimplemented, "recording not yet supported")
}

// PlaybackPositionInfo is the replay position owned by this streaming layer.
//
// It mirrors server.PlaybackPosition structurally rather than importing it:
// internal/lidar/server already imports this package, so declaring the
// interface there and satisfying it here keeps the dependency edge one-way.
//
// It carries no mode field on purpose. Which source is driving the pipeline is
// owned by the monitor server, which initiates every transition; this layer is
// told, and duplicating the answer here is what previously let the two
// disagree.
type PlaybackPositionInfo struct {
	Paused       bool
	Rate         float32
	Seekable     bool
	CurrentFrame uint64
	TotalFrames  uint64
	TimestampNs  int64
	LogStartNs   int64
	LogEndNs     int64
	ReplayEpoch  uint64
}

// PlaybackPosition returns the current replay position. Live streaming has no
// meaningful position, so the zero value (with a unit rate) is returned.
func (s *Server) PlaybackPosition() PlaybackPositionInfo {
	s.playbackMu.RLock()
	info := PlaybackPositionInfo{
		Paused:       s.paused,
		Rate:         s.playbackRate,
		Seekable:     s.vrlogMode,
		CurrentFrame: s.pcapCurrentPacket,
		TotalFrames:  s.pcapTotalPackets,
		LogStartNs:   s.pcapStartNs,
		LogEndNs:     s.pcapEndNs,
		ReplayEpoch:  s.replayEpoch,
	}
	vrlogMode := s.vrlogMode
	s.playbackMu.RUnlock()

	// Frame counts come from the VRLOG reader, which tracks them per frame;
	// the packet counters above are the PCAP equivalent.
	if vrlogMode && s.publisher != nil {
		if reader := s.publisher.VRLogReader(); reader != nil {
			info.CurrentFrame = reader.CurrentFrame()
			info.TotalFrames = reader.TotalFrames()
		}
	}
	if info.Rate == 0 {
		info.Rate = 1.0
	}
	return info
}
