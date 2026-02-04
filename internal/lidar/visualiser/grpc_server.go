// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file implements the gRPC service methods.
package visualiser

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Ensure Server implements the gRPC interface.
var _ pb.VisualiserServiceServer = (*Server)(nil)

// Server implements the VisualiserService gRPC server.
type Server struct {
	pb.UnimplementedVisualiserServiceServer

	publisher *Publisher

	// Synthetic mode
	syntheticMode bool
	syntheticGen  *SyntheticGenerator

	// Playback state (for future replay support)
	paused       bool
	playbackRate float32
}

// NewServer creates a new gRPC server.
func NewServer(publisher *Publisher) *Server {
	return &Server{
		publisher:    publisher,
		playbackRate: 1.0,
	}
}

// EnableSyntheticMode enables synthetic data generation.
func (s *Server) EnableSyntheticMode(sensorID string) {
	s.syntheticMode = true
	s.syntheticGen = NewSyntheticGenerator(sensorID)
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
			if s.paused {
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
		frameCh: frameCh,
		doneCh:  make(chan struct{}),
	}
	s.publisher.clientsMu.Unlock()
	s.publisher.clientCount.Add(1)

	defer func() {
		s.publisher.removeClient(clientID)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case frame := <-frameCh:
			pbFrame := frameBundleToProto(frame, req)
			if err := stream.Send(pbFrame); err != nil {
				return err
			}
		}
	}
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
	s.paused = true
	return &pb.PlaybackStatus{
		Paused: true,
		Rate:   s.playbackRate,
	}, nil
}

// Play resumes playback (replay mode).
func (s *Server) Play(ctx context.Context, req *pb.PlayRequest) (*pb.PlaybackStatus, error) {
	s.paused = false
	return &pb.PlaybackStatus{
		Paused: false,
		Rate:   s.playbackRate,
	}, nil
}

// Seek seeks to a specific timestamp or frame (replay mode).
func (s *Server) Seek(ctx context.Context, req *pb.SeekRequest) (*pb.PlaybackStatus, error) {
	// TODO: Implement seek when replay is supported
	return nil, status.Error(codes.Unimplemented, "seek not yet supported")
}

// SetRate sets the playback rate.
func (s *Server) SetRate(ctx context.Context, req *pb.SetRateRequest) (*pb.PlaybackStatus, error) {
	s.playbackRate = req.Rate
	return &pb.PlaybackStatus{
		Paused: s.paused,
		Rate:   s.playbackRate,
	}, nil
}

// SetOverlayModes configures which overlays to emit.
func (s *Server) SetOverlayModes(ctx context.Context, req *pb.OverlayModeRequest) (*pb.OverlayModeResponse, error) {
	// TODO: Store overlay preferences
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
