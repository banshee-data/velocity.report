// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file implements replay mode support.
package visualiser

import (
	"context"
	"io"
	"log"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FrameReader is an interface for reading frames in sequence.
// This allows us to abstract over recorder.Replayer without creating a circular dependency.
type FrameReader interface {
	ReadFrame() (*FrameBundle, error)
	Seek(frameIdx uint64) error
	SeekToTimestamp(timestampNs int64) error
	CurrentFrame() uint64
	TotalFrames() uint64
	SetPaused(paused bool)
	SetRate(rate float32)
	Close() error
}

// ReplayServer wraps a Server with replay capabilities.
type ReplayServer struct {
	*Server
	reader       FrameReader
	mu           sync.RWMutex
	seekOccurred bool // Set by Seek(), cleared by streaming loop to reset timing
	sendOneFrame bool // Set by Seek() when paused, causes one frame to be sent
}

// NewReplayServer creates a server configured for replay mode.
func NewReplayServer(publisher *Publisher, reader FrameReader) *ReplayServer {
	return &ReplayServer{
		Server: NewServer(publisher),
		reader: reader,
	}
}

// StreamFrames implements the streaming RPC for frame data in replay mode.
func (rs *ReplayServer) StreamFrames(req *pb.StreamRequest, stream pb.VisualiserService_StreamFramesServer) error {
	log.Printf("[gRPC] *** NEW CLIENT CONNECTED (REPLAY MODE) ***")
	log.Printf("[gRPC] StreamFrames started: sensor=%s points=%v clusters=%v tracks=%v",
		req.SensorId, req.IncludePoints, req.IncludeClusters, req.IncludeTracks)

	ctx := stream.Context()
	return rs.streamFromReader(ctx, req, stream)
}

// streamFromReader streams frames from the reader.
func (rs *ReplayServer) streamFromReader(ctx context.Context, req *pb.StreamRequest, stream pb.VisualiserService_StreamFramesServer) error {
	rs.mu.RLock()
	reader := rs.reader
	rs.mu.RUnlock()

	if reader == nil {
		return status.Error(codes.Internal, "frame reader not initialized")
	}

	log.Printf("[gRPC] Starting replay: %d total frames", reader.TotalFrames())

	// Calculate frame interval based on playback rate
	var lastFrameTime int64
	var lastWallTime time.Time

	for {
		select {
		case <-ctx.Done():
			log.Printf("[gRPC] Replay cancelled")
			return ctx.Err()
		default:
		}

		// Check if paused and if a seek occurred
		rs.mu.Lock()
		isPaused := rs.paused
		rate := rs.playbackRate
		seeked := rs.seekOccurred
		rs.seekOccurred = false // Clear the flag
		sendOne := rs.sendOneFrame
		rs.sendOneFrame = false // Clear the flag
		rs.mu.Unlock()

		// Reset timing state after a seek to prevent long sleeps
		if seeked {
			lastFrameTime = 0
			lastWallTime = time.Time{}
		}

		// If paused and not stepping, just wait
		if isPaused && !sendOne {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Read next frame
		frame, err := reader.ReadFrame()
		if err != nil {
			if err == io.EOF {
				log.Printf("[gRPC] Replay complete")
				return nil
			}
			log.Printf("[gRPC] Replay error: %v", err)
			return status.Errorf(codes.Internal, "replay error: %v", err)
		}

		// Rate control: sleep to match playback rate
		if lastFrameTime > 0 && rate > 0 {
			frameDelta := time.Duration(float64(frame.TimestampNanos-lastFrameTime) / float64(rate))
			wallDelta := time.Since(lastWallTime)
			if frameDelta > wallDelta {
				time.Sleep(frameDelta - wallDelta)
			}
		}

		lastFrameTime = frame.TimestampNanos
		lastWallTime = time.Now()

		// Convert to proto and send
		pbFrame := frameBundleToProto(frame, req)
		if err := stream.Send(pbFrame); err != nil {
			log.Printf("[gRPC] Send error: %v", err)
			return err
		}
	}
}

// Pause pauses playback (replay mode).
func (rs *ReplayServer) Pause(ctx context.Context, req *pb.PauseRequest) (*pb.PlaybackStatus, error) {
	log.Printf("[gRPC] Pause called")
	rs.mu.Lock()
	rs.paused = true
	currentFrame := uint64(0)
	if rs.reader != nil {
		currentFrame = rs.reader.CurrentFrame()
		rs.reader.SetPaused(true)
	}
	rate := rs.playbackRate
	rs.mu.Unlock()

	log.Printf("[gRPC] Paused at frame %d", currentFrame)
	return &pb.PlaybackStatus{
		Paused:         true,
		Rate:           rate,
		CurrentFrameId: currentFrame,
	}, nil
}

// Play resumes playback (replay mode).
func (rs *ReplayServer) Play(ctx context.Context, req *pb.PlayRequest) (*pb.PlaybackStatus, error) {
	log.Printf("[gRPC] Play called")
	rs.mu.Lock()
	rs.paused = false
	currentFrame := uint64(0)
	if rs.reader != nil {
		currentFrame = rs.reader.CurrentFrame()
		rs.reader.SetPaused(false)
	}
	rate := rs.playbackRate
	rs.mu.Unlock()

	log.Printf("[gRPC] Playing from frame %d", currentFrame)
	return &pb.PlaybackStatus{
		Paused:         false,
		Rate:           rate,
		CurrentFrameId: currentFrame,
	}, nil
}

// Seek seeks to a specific timestamp or frame (replay mode).
func (rs *ReplayServer) Seek(ctx context.Context, req *pb.SeekRequest) (*pb.PlaybackStatus, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.reader == nil {
		return nil, status.Error(codes.FailedPrecondition, "replay mode not active")
	}

	var err error
	switch target := req.Target.(type) {
	case *pb.SeekRequest_TimestampNs:
		log.Printf("[gRPC] Seek to timestamp: %d ns", target.TimestampNs)
		err = rs.reader.SeekToTimestamp(target.TimestampNs)
	case *pb.SeekRequest_FrameId:
		log.Printf("[gRPC] Seek to frame: %d", target.FrameId)
		err = rs.reader.Seek(target.FrameId)
	default:
		return nil, status.Error(codes.InvalidArgument, "seek target not specified")
	}

	if err != nil {
		log.Printf("[gRPC] Seek error: %v", err)
		return nil, status.Errorf(codes.Internal, "seek failed: %v", err)
	}

	// Signal to streaming loop to reset timing
	rs.seekOccurred = true
	// If paused, send one frame so the UI updates
	if rs.paused {
		rs.sendOneFrame = true
	}

	currentFrame := rs.reader.CurrentFrame()
	log.Printf("[gRPC] Seek complete: now at frame %d, paused=%v, rate=%.2f", currentFrame, rs.paused, rs.playbackRate)
	return &pb.PlaybackStatus{
		Paused:         rs.paused,
		Rate:           rs.playbackRate,
		CurrentFrameId: currentFrame,
	}, nil
}

// SetRate sets the playback rate.
func (rs *ReplayServer) SetRate(ctx context.Context, req *pb.SetRateRequest) (*pb.PlaybackStatus, error) {
	log.Printf("[gRPC] SetRate called: rate=%.2f", req.Rate)
	rs.mu.Lock()
	rs.playbackRate = req.Rate
	if rs.reader != nil {
		rs.reader.SetRate(req.Rate)
	}
	currentFrame := uint64(0)
	if rs.reader != nil {
		currentFrame = rs.reader.CurrentFrame()
	}
	paused := rs.paused
	rs.mu.Unlock()

	log.Printf("[gRPC] SetRate complete: rate=%.2f, frame=%d", req.Rate, currentFrame)
	return &pb.PlaybackStatus{
		Paused:         paused,
		Rate:           req.Rate,
		CurrentFrameId: currentFrame,
	}, nil
}

// GetCapabilities returns server capabilities (replay mode).
func (rs *ReplayServer) GetCapabilities(ctx context.Context, req *pb.CapabilitiesRequest) (*pb.CapabilitiesResponse, error) {
	return &pb.CapabilitiesResponse{
		SupportsPoints:    true,
		SupportsClusters:  true,
		SupportsTracks:    true,
		SupportsDebug:     true,
		SupportsReplay:    true,
		SupportsRecording: false,
		AvailableSensors:  []string{"replay"},
	}, nil
}

// Close closes the replay server and its reader.
func (rs *ReplayServer) Close() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.reader != nil {
		return rs.reader.Close()
	}
	return nil
}
