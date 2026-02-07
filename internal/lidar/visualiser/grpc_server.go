// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file implements the gRPC service methods.
package visualiser

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
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
	playbackMu   sync.RWMutex

	// PCAP progress tracking (updated by WebServer progress callback)
	pcapCurrentPacket uint64
	pcapTotalPackets  uint64
	pcapStartNs       int64
	pcapEndNs         int64

	// Per-client overlay preferences (protected by preferenceMu)
	clientPreferences map[string]*overlayPreferences
	preferenceMu      sync.RWMutex
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
	s.replayMode = enabled
	if !enabled {
		s.pcapCurrentPacket = 0
		s.pcapTotalPackets = 0
		s.pcapStartNs = 0
		s.pcapEndNs = 0
	}
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
	log.Printf("[gRPC] *** NEW CLIENT CONNECTED ***")
	log.Printf("[gRPC] StreamFrames started: sensor=%s points=%v clusters=%v tracks=%v",
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
			log.Printf("[gRPC] StreamFrames cancelled")
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
				log.Printf("[gRPC] Send error: %v", err)
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

// streamFromPublisher streams frames from the publisher.
func (s *Server) streamFromPublisher(ctx context.Context, req *pb.StreamRequest, stream pb.VisualiserService_StreamFramesServer) error {
	// Create a unique client ID
	clientID := fmt.Sprintf("grpc-%d", time.Now().UnixNano())

	// Subscribe to frames
	frameCh := make(chan *FrameBundle, 10)

	// Register with publisher
	s.publisher.clientsMu.Lock()
	s.publisher.clients[clientID] = &clientStream{
		id:      clientID,
		request: req,
		frameCh: frameCh,
		doneCh:  make(chan struct{}),
	}
	s.publisher.clientsMu.Unlock()
	s.publisher.clientCount.Add(1)

	lidar.Debugf("[gRPC] Client %s subscribed: points=%v clusters=%v tracks=%v",
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
	const sendTimeoutMs = 100         // Skip frame if send would take > 100ms
	const maxConsecutiveSlowSends = 3 // After 3 slow sends, start skipping
	const minConsecutiveFastSends = 5 // Require 5 fast sends before exiting skip mode (hysteresis)

	// Track message sizes for bandwidth estimation
	var totalBytesSent int64
	cooldown := newSendCooldown(maxConsecutiveSlowSends, minConsecutiveFastSends)
	var lastFrameID uint64

	for {
		select {
		case <-ctx.Done():
			lidar.Debugf("[gRPC] Client %s disconnected: frames_sent=%d dropped=%d slow_sends=%d avg_send_time_ms=%.2f",
				clientID, framesSent, droppedFrames, slowSends, float64(totalSendTimeNs)/float64(max(framesSent, 1))/1e6)
			return ctx.Err()
		case frame := <-frameCh:
			// Respect pause state — drop frames silently while paused.
			// M7: Release the retained reference since we won't process this frame.
			s.playbackMu.RLock()
			paused := s.paused
			s.playbackMu.RUnlock()
			if paused {
				if frame.PointCloud != nil {
					frame.PointCloud.Release()
				}
				continue
			}

			// Skip frames if we're falling behind (keep only latest)
			// Drain any additional frames in the channel to catch up
			skipped := 0
			for len(frameCh) > 0 && cooldown.inSkipMode() {
				select {
				case newerFrame := <-frameCh:
					// M7: Release the old frame we're discarding
					if frame.PointCloud != nil {
						frame.PointCloud.Release()
					}
					frame = newerFrame // Use the newer frame
					skipped++
					droppedFrames++
				default:
					break
				}
			}
			if skipped > 0 {
				lidar.Debugf("[gRPC] Client %s: skipped %d frames to catch up (skip_mode=%v)",
					clientID, skipped, cooldown.inSkipMode())
			}

			// Track frame ID gaps for detecting skipped frames
			if lastFrameID > 0 && frame.FrameID > lastFrameID+1 {
				gap := frame.FrameID - lastFrameID - 1
				if gap > 0 {
					droppedFrames += gap
				}
			}
			lastFrameID = frame.FrameID

			// Inject PlaybackInfo for replay mode (PCAP) if not already set.
			// This allows the client to show "REPLAY" instead of "LIVE".
			if s.replayMode && frame.PlaybackInfo == nil {
				s.playbackMu.RLock()
				frame.PlaybackInfo = &PlaybackInfo{
					IsLive:            false,
					LogStartNs:        s.pcapStartNs,
					LogEndNs:          s.pcapEndNs,
					PlaybackRate:      s.playbackRate,
					Paused:            s.paused,
					CurrentFrameIndex: s.pcapCurrentPacket,
					TotalFrames:       s.pcapTotalPackets,
					Seekable:          false,
				}
				s.playbackMu.RUnlock()
			}

			// Measure serialisation and send time
			sendStart := time.Now()
			pbFrame := frameBundleToProto(frame, req)

			// Measure serialised message size
			msgSize := proto.Size(pbFrame)
			totalBytesSent += int64(msgSize)

			if err := stream.Send(pbFrame); err != nil {
				// M7: Release on error path - protobuf data has been marshalled
				// by Send(), so it's safe to release the source slices now.
				if frame.PointCloud != nil {
					frame.PointCloud.Release()
				}
				log.Printf("[gRPC] Send error for client %s after %d frames: %v", clientID, framesSent, err)
				return err
			}

			// M7: Release PointCloud reference after stream.Send() completes.
			// The protobuf message has been marshalled and sent, so we can
			// safely decrement the reference count. When all clients have
			// released, the slices are returned to the pool.
			// NOTE: frameBundleToProto assigns X/Y/Z slices directly into the
			// protobuf (no deep copy), so Release must happen AFTER Send.
			if frame.PointCloud != nil {
				frame.PointCloud.Release()
			}
			sendDuration := time.Since(sendStart)
			totalSendTimeNs += sendDuration.Nanoseconds()
			framesSent++

			// Track slow sends with message size info — hysteresis cooldown (§7.3)
			// After entering skip mode, require minConsecutiveFastSends before exiting
			// to prevent oscillation between skip and normal modes.
			if sendDuration.Milliseconds() > slowSendThresholdMs {
				slowSends++
				cooldown.recordSlow()
				if sendDuration.Milliseconds() > sendTimeoutMs {
					log.Printf("[gRPC] SLOW SEND: client=%s frame=%d duration=%v points=%d msg_size_kb=%.1f skip_mode=%v",
						clientID, frame.FrameID, sendDuration, getPointCount(frame), float64(msgSize)/1024, cooldown.inSkipMode())
				}
			} else {
				cooldown.recordFast()
			}

			// Periodic performance logging
			if time.Since(lastLogTime) >= logInterval {
				avgSendMs := float64(totalSendTimeNs) / float64(framesSent) / 1e6
				fps := float64(framesSent) / time.Since(lastLogTime).Seconds()
				queueDepth := len(frameCh)
				bandwidthMbps := float64(totalBytesSent) * 8 / time.Since(lastLogTime).Seconds() / 1e6
				avgMsgSizeKB := float64(totalBytesSent) / float64(max(framesSent, 1)) / 1024
				lidar.Debugf("[gRPC] Client %s stats: fps=%.1f frames=%d dropped=%d queue=%d/10 avg_send_ms=%.2f slow_sends=%d bandwidth_mbps=%.1f avg_msg_kb=%.1f",
					clientID, fps, framesSent, droppedFrames, queueDepth, avgSendMs, slowSends, bandwidthMbps, avgMsgSizeKB)

				// Check for queue backup
				if queueDepth > 5 {
					log.Printf("[gRPC] WARNING: Client %s queue backing up: %d/10 frames buffered", clientID, queueDepth)
				}

				// Reset counters for next interval
				framesSent = 0
				totalSendTimeNs = 0
				slowSends = 0
				totalBytesSent = 0
				lastLogTime = time.Now()
			}
		}
	}
}

// getPointCount safely extracts point count from a frame bundle.
func getPointCount(frame *FrameBundle) int {
	if frame != nil && frame.PointCloud != nil {
		return frame.PointCloud.PointCount
	}
	return 0
}

// frameBundleToProto converts internal FrameBundle to protobuf.
func frameBundleToProto(frame *FrameBundle, req *pb.StreamRequest) *pb.FrameBundle {
	pbFrame := &pb.FrameBundle{
		FrameId:     frame.FrameID,
		TimestampNs: frame.TimestampNanos,
		SensorId:    frame.SensorID,
		CoordinateFrame: &pb.CoordinateFrameInfo{
			FrameId:        frame.CoordinateFrame.FrameID,
			ReferenceFrame: frame.CoordinateFrame.ReferenceFrame,
			OriginLat:      frame.CoordinateFrame.OriginLat,
			OriginLon:      frame.CoordinateFrame.OriginLon,
			OriginAlt:      frame.CoordinateFrame.OriginAlt,
			RotationDeg:    frame.CoordinateFrame.RotationDeg,
		},
	}

	// Include point cloud if requested
	if req.IncludePoints && frame.PointCloud != nil {
		pc := frame.PointCloud
		pbFrame.PointCloud = &pb.PointCloudFrame{
			FrameId:         pc.FrameID,
			TimestampNs:     pc.TimestampNanos,
			SensorId:        pc.SensorID,
			X:               pc.X,
			Y:               pc.Y,
			Z:               pc.Z,
			Intensity:       byteSliceToUint32(pc.Intensity),
			Classification:  byteSliceToUint32(pc.Classification),
			DecimationMode:  pb.DecimationMode(pc.DecimationMode),
			DecimationRatio: pc.DecimationRatio,
			PointCount:      int32(pc.PointCount),
		}
	}

	// Include clusters if requested
	if req.IncludeClusters && frame.Clusters != nil {
		cs := frame.Clusters
		pbClusters := make([]*pb.Cluster, len(cs.Clusters))
		for i, c := range cs.Clusters {
			pbClusters[i] = &pb.Cluster{
				ClusterId:   c.ClusterID,
				SensorId:    c.SensorID,
				TimestampNs: c.TimestampNanos,
				CentroidX:   c.CentroidX,
				CentroidY:   c.CentroidY,
				CentroidZ:   c.CentroidZ,
				AabbLength:  c.AABBLength,
				AabbWidth:   c.AABBWidth,
				AabbHeight:  c.AABBHeight,
				PointsCount: int32(c.PointsCount),
			}
		}
		pbFrame.Clusters = &pb.ClusterSet{
			FrameId:     cs.FrameID,
			TimestampNs: cs.TimestampNanos,
			Clusters:    pbClusters,
			Method:      pb.ClusteringMethod(cs.Method),
		}
	}

	// Include tracks if requested
	if req.IncludeTracks && frame.Tracks != nil {
		ts := frame.Tracks
		pbTracks := make([]*pb.Track, len(ts.Tracks))
		for i, t := range ts.Tracks {
			pbTracks[i] = &pb.Track{
				TrackId:          t.TrackID,
				SensorId:         t.SensorID,
				State:            pb.TrackState(t.State),
				Hits:             int32(t.Hits),
				Misses:           int32(t.Misses),
				ObservationCount: int32(t.ObservationCount),
				FirstSeenNs:      t.FirstSeenNanos,
				LastSeenNs:       t.LastSeenNanos,
				X:                t.X,
				Y:                t.Y,
				Z:                t.Z,
				Vx:               t.VX,
				Vy:               t.VY,
				Vz:               t.VZ,
				SpeedMps:         t.SpeedMps,
				HeadingRad:       t.HeadingRad,
				BboxLengthAvg:    t.BBoxLengthAvg,
				BboxWidthAvg:     t.BBoxWidthAvg,
				BboxHeightAvg:    t.BBoxHeightAvg,
				BboxHeadingRad:   t.BBoxHeadingRad,
				Confidence:       t.Confidence,
				MotionModel:      pb.MotionModel(t.MotionModel),
				Alpha:            t.Alpha,
			}
		}

		pbTrails := make([]*pb.TrackTrail, len(ts.Trails))
		for i, trail := range ts.Trails {
			pbPoints := make([]*pb.TrackPoint, len(trail.Points))
			for j, p := range trail.Points {
				pbPoints[j] = &pb.TrackPoint{
					X:           p.X,
					Y:           p.Y,
					TimestampNs: p.TimestampNanos,
				}
			}
			pbTrails[i] = &pb.TrackTrail{
				TrackId: trail.TrackID,
				Points:  pbPoints,
			}
		}

		pbFrame.Tracks = &pb.TrackSet{
			FrameId:     ts.FrameID,
			TimestampNs: ts.TimestampNanos,
			Tracks:      pbTracks,
			Trails:      pbTrails,
		}
	}

	// Include playback info
	if frame.PlaybackInfo != nil {
		pbFrame.PlaybackInfo = &pb.PlaybackInfo{
			IsLive:            frame.PlaybackInfo.IsLive,
			LogStartNs:        frame.PlaybackInfo.LogStartNs,
			LogEndNs:          frame.PlaybackInfo.LogEndNs,
			PlaybackRate:      frame.PlaybackInfo.PlaybackRate,
			Paused:            frame.PlaybackInfo.Paused,
			CurrentFrameIndex: frame.PlaybackInfo.CurrentFrameIndex,
			TotalFrames:       frame.PlaybackInfo.TotalFrames,
			Seekable:          frame.PlaybackInfo.Seekable,
		}
	}

	// M3.5: Include frame type and background snapshot
	pbFrame.FrameType = pb.FrameType(frame.FrameType)
	pbFrame.BackgroundSeq = frame.BackgroundSeq

	if frame.Background != nil {
		bg := frame.Background
		pbFrame.Background = &pb.BackgroundSnapshot{
			SequenceNumber: bg.SequenceNumber,
			TimestampNanos: bg.TimestampNanos,
			X:              bg.X,
			Y:              bg.Y,
			Z:              bg.Z,
			Confidence:     bg.Confidence,
			GridMetadata: &pb.GridMetadata{
				Rings:            int32(bg.GridMetadata.Rings),
				AzimuthBins:      int32(bg.GridMetadata.AzimuthBins),
				RingElevations:   bg.GridMetadata.RingElevations,
				SettlingComplete: bg.GridMetadata.SettlingComplete,
			},
		}
	}

	return pbFrame
}

// byteSliceToUint32 converts []uint8 to []uint32.
func byteSliceToUint32(b []uint8) []uint32 {
	result := make([]uint32, len(b))
	for i, v := range b {
		result[i] = uint32(v)
	}
	return result
}

// Pause pauses playback (replay mode).
func (s *Server) Pause(ctx context.Context, req *pb.PauseRequest) (*pb.PlaybackStatus, error) {
	s.playbackMu.Lock()
	s.paused = true
	rate := s.playbackRate
	s.playbackMu.Unlock()
	return &pb.PlaybackStatus{
		Paused: true,
		Rate:   rate,
	}, nil
}

// Play resumes playback (replay mode).
func (s *Server) Play(ctx context.Context, req *pb.PlayRequest) (*pb.PlaybackStatus, error) {
	s.playbackMu.Lock()
	s.paused = false
	rate := s.playbackRate
	s.playbackMu.Unlock()
	return &pb.PlaybackStatus{
		Paused: false,
		Rate:   rate,
	}, nil
}

// Seek seeks to a specific timestamp or frame (replay mode).
func (s *Server) Seek(ctx context.Context, req *pb.SeekRequest) (*pb.PlaybackStatus, error) {
	// TODO: Implement seek when replay is supported
	return nil, status.Error(codes.Unimplemented, "seek not yet supported")
}

// SetRate sets the playback rate.
func (s *Server) SetRate(ctx context.Context, req *pb.SetRateRequest) (*pb.PlaybackStatus, error) {
	s.playbackMu.Lock()
	s.playbackRate = req.Rate
	paused := s.paused
	rate := s.playbackRate
	s.playbackMu.Unlock()
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

	log.Printf("[gRPC] Overlay modes updated: points=%v clusters=%v tracks=%v trails=%v velocity=%v gating=%v association=%v residuals=%v",
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
		SupportsReplay:    false, // TODO: Enable when replay is implemented
		SupportsRecording: false, // TODO: Enable when recording is implemented
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
