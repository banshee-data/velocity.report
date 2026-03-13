// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockFrameReader is a mock implementation of FrameReader for testing.
type mockFrameReader struct {
	frames       []*FrameBundle
	currentFrame uint64
	paused       bool
	rate         float32
	closed       bool
	mu           sync.Mutex
}

func newMockFrameReader(frames []*FrameBundle) *mockFrameReader {
	return &mockFrameReader{
		frames: frames,
		rate:   1.0,
	}
}

func (m *mockFrameReader) ReadFrame() (*FrameBundle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, io.EOF
	}

	if m.currentFrame >= uint64(len(m.frames)) {
		return nil, io.EOF
	}

	frame := m.frames[m.currentFrame]
	m.currentFrame++
	return frame, nil
}

func (m *mockFrameReader) Seek(frameIdx uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if frameIdx >= uint64(len(m.frames)) {
		return io.EOF
	}

	m.currentFrame = frameIdx
	return nil
}

func (m *mockFrameReader) SeekToTimestamp(timestampNs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Binary search for closest frame
	for i, frame := range m.frames {
		if frame.TimestampNanos >= timestampNs {
			m.currentFrame = uint64(i)
			return nil
		}
	}

	m.currentFrame = uint64(len(m.frames) - 1)
	return nil
}

func (m *mockFrameReader) CurrentFrame() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentFrame
}

func (m *mockFrameReader) TotalFrames() uint64 {
	return uint64(len(m.frames))
}

func (m *mockFrameReader) SetPaused(paused bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused = paused
}

func (m *mockFrameReader) SetRate(rate float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rate = rate
}

func (m *mockFrameReader) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// mockStreamServer is a mock implementation of the gRPC stream server.
type mockStreamServer struct {
	frames []*pb.FrameBundle
	ctx    context.Context
	mu     sync.Mutex
}

func newMockStreamServer(ctx context.Context) *mockStreamServer {
	return &mockStreamServer{
		frames: make([]*pb.FrameBundle, 0),
		ctx:    ctx,
	}
}

func (m *mockStreamServer) Send(frame *pb.FrameBundle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frames = append(m.frames, frame)
	return nil
}

func (m *mockStreamServer) Context() context.Context {
	return m.ctx
}

func (m *mockStreamServer) SetHeader(md metadata.MD) error  { return nil }
func (m *mockStreamServer) SendHeader(md metadata.MD) error { return nil }
func (m *mockStreamServer) SetTrailer(md metadata.MD)       {}
func (m *mockStreamServer) SendMsg(msg interface{}) error   { return nil }
func (m *mockStreamServer) RecvMsg(msg interface{}) error   { return nil }

func TestNewReplayServer(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	reader := newMockFrameReader([]*FrameBundle{})

	rs := NewReplayServer(pub, reader)

	if rs == nil {
		t.Fatal("expected non-nil ReplayServer")
	}
	if rs.Server == nil {
		t.Error("expected Server to be set")
	}
	if rs.reader != reader {
		t.Error("expected reader to be set")
	}
}

func TestReplayServer_Pause(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	reader := newMockFrameReader([]*FrameBundle{})
	rs := NewReplayServer(pub, reader)

	ctx := context.Background()
	status, err := rs.Pause(ctx, &pb.PauseRequest{})

	if err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	if !status.Paused {
		t.Error("expected Paused=true after Pause")
	}
	if !rs.paused {
		t.Error("expected rs.paused=true after Pause")
	}
}

func TestReplayServer_Play(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	reader := newMockFrameReader([]*FrameBundle{})
	rs := NewReplayServer(pub, reader)

	// First pause
	rs.paused = true

	ctx := context.Background()
	status, err := rs.Play(ctx, &pb.PlayRequest{})

	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}
	if status.Paused {
		t.Error("expected Paused=false after Play")
	}
	if rs.paused {
		t.Error("expected rs.paused=false after Play")
	}
}

func TestReplayServer_SetRate(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	reader := newMockFrameReader([]*FrameBundle{})
	rs := NewReplayServer(pub, reader)

	ctx := context.Background()

	tests := []float32{0.5, 1.0, 2.0, 0.25}
	for _, rate := range tests {
		status, err := rs.SetRate(ctx, &pb.SetRateRequest{Rate: rate})
		if err != nil {
			t.Fatalf("SetRate(%f) failed: %v", rate, err)
		}
		if status.Rate != rate {
			t.Errorf("expected Rate=%f, got %f", rate, status.Rate)
		}
		if rs.playbackRate != rate {
			t.Errorf("expected rs.playbackRate=%f, got %f", rate, rs.playbackRate)
		}
	}
}

func TestReplayServer_Seek_ByFrameID(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	// Create test frames
	frames := []*FrameBundle{
		{FrameID: 0, TimestampNanos: 1000},
		{FrameID: 1, TimestampNanos: 2000},
		{FrameID: 2, TimestampNanos: 3000},
	}
	reader := newMockFrameReader(frames)
	rs := NewReplayServer(pub, reader)

	ctx := context.Background()

	// Seek to frame 1
	status, err := rs.Seek(ctx, &pb.SeekRequest{
		Target: &pb.SeekRequest_FrameId{FrameId: 1},
	})

	if err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	if status.CurrentFrameId != 1 {
		t.Errorf("expected CurrentFrameId=1, got %d", status.CurrentFrameId)
	}
	if reader.CurrentFrame() != 1 {
		t.Errorf("expected reader.CurrentFrame()=1, got %d", reader.CurrentFrame())
	}
}

func TestReplayServer_Seek_ByTimestamp(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	// Create test frames
	frames := []*FrameBundle{
		{FrameID: 0, TimestampNanos: 1000},
		{FrameID: 1, TimestampNanos: 2000},
		{FrameID: 2, TimestampNanos: 3000},
	}
	reader := newMockFrameReader(frames)
	rs := NewReplayServer(pub, reader)

	ctx := context.Background()

	// Seek to timestamp 2500 (should go to frame 2)
	seekStatus, err := rs.Seek(ctx, &pb.SeekRequest{
		Target: &pb.SeekRequest_TimestampNs{TimestampNs: 2500},
	})

	if err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	_ = seekStatus
	// Mock implementation goes to next frame with timestamp >= target
	if reader.CurrentFrame() != 2 {
		t.Errorf("expected reader.CurrentFrame()=2, got %d", reader.CurrentFrame())
	}
}

func TestReplayServer_Seek_NoReader(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	rs := &ReplayServer{
		Server: NewServer(pub),
		reader: nil,
	}

	ctx := context.Background()
	_, err := rs.Seek(ctx, &pb.SeekRequest{
		Target: &pb.SeekRequest_FrameId{FrameId: 0},
	})

	if err == nil {
		t.Error("expected error when reader is nil")
	}
	code := status.Code(err)
	if code != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition error, got code %v", code)
	}
}

func TestReplayServer_Seek_NoTarget(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	reader := newMockFrameReader([]*FrameBundle{})
	rs := NewReplayServer(pub, reader)

	ctx := context.Background()
	_, err := rs.Seek(ctx, &pb.SeekRequest{})

	if err == nil {
		t.Error("expected error when target is not specified")
	}
	code := status.Code(err)
	if code != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument error, got code %v", code)
	}
}

func TestReplayServer_GetCapabilities(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	reader := newMockFrameReader([]*FrameBundle{})
	rs := NewReplayServer(pub, reader)

	ctx := context.Background()
	resp, err := rs.GetCapabilities(ctx, &pb.CapabilitiesRequest{})

	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}
	if !resp.SupportsReplay {
		t.Error("expected SupportsReplay=true")
	}
	if !resp.SupportsPoints {
		t.Error("expected SupportsPoints=true")
	}
	if !resp.SupportsClusters {
		t.Error("expected SupportsClusters=true")
	}
	if !resp.SupportsTracks {
		t.Error("expected SupportsTracks=true")
	}
}

func TestReplayServer_Close(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	reader := newMockFrameReader([]*FrameBundle{})
	rs := NewReplayServer(pub, reader)

	err := rs.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !reader.closed {
		t.Error("expected reader to be closed")
	}
}

func TestReplayServer_StreamFrames_CancelContext(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	// Create test frames
	frames := []*FrameBundle{
		{FrameID: 0, TimestampNanos: 1000, SensorID: "test"},
		{FrameID: 1, TimestampNanos: 2000, SensorID: "test"},
	}
	reader := newMockFrameReader(frames)
	rs := NewReplayServer(pub, reader)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stream := newMockStreamServer(ctx)
	req := &pb.StreamRequest{
		SensorId:        "test",
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
	}

	err := rs.StreamFrames(req, stream)

	if err == nil {
		t.Error("expected error when context is cancelled")
	}
}

func TestReplayServer_StreamFrames_Success(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	// Create test frames with coordinate frame info
	frames := []*FrameBundle{
		{
			FrameID:        0,
			TimestampNanos: 1000000000,
			SensorID:       "test",
			CoordinateFrame: CoordinateFrameInfo{
				FrameID:        "test/sensor",
				ReferenceFrame: "ENU",
			},
		},
		{
			FrameID:        1,
			TimestampNanos: 2000000000,
			SensorID:       "test",
			CoordinateFrame: CoordinateFrameInfo{
				FrameID:        "test/sensor",
				ReferenceFrame: "ENU",
			},
		},
	}
	reader := newMockFrameReader(frames)
	rs := NewReplayServer(pub, reader)

	// Create a context with timeout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockStreamServer(ctx)
	req := &pb.StreamRequest{
		SensorId:        "test",
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
	}

	// Run streaming in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- rs.StreamFrames(req, stream)
	}()

	// Wait for all frames to be sent (the server pauses at EOF instead of
	// returning, so we poll until the expected frame count is reached and
	// then cancel the context to stop the stream).
	deadline := time.After(2 * time.Second)
	for {
		stream.mu.Lock()
		n := len(stream.frames)
		stream.mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for frames")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Server should now be paused at EOF
	rs.mu.RLock()
	paused := rs.paused
	rs.mu.RUnlock()
	if !paused {
		t.Error("expected server to be paused at EOF")
	}

	// Reader should also be paused (SetPaused(true) called at EOF)
	reader.mu.Lock()
	readerPaused := reader.paused
	reader.mu.Unlock()
	if !readerPaused {
		t.Error("expected reader to be paused at EOF")
	}

	// Cancel context to stop the stream
	cancel()
	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	// Check that frames were sent
	stream.mu.Lock()
	frameCount := len(stream.frames)
	stream.mu.Unlock()

	if frameCount != 2 {
		t.Errorf("expected 2 frames, got %d", frameCount)
	}
}

func TestMockFrameReader_ReadFrame(t *testing.T) {
	frames := []*FrameBundle{
		{FrameID: 0, TimestampNanos: 1000},
		{FrameID: 1, TimestampNanos: 2000},
	}
	reader := newMockFrameReader(frames)

	// Read first frame
	frame, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}
	if frame.FrameID != 0 {
		t.Errorf("expected FrameID=0, got %d", frame.FrameID)
	}

	// Read second frame
	frame, err = reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}
	if frame.FrameID != 1 {
		t.Errorf("expected FrameID=1, got %d", frame.FrameID)
	}

	// EOF on third read
	_, err = reader.ReadFrame()
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestMockFrameReader_Seek(t *testing.T) {
	frames := []*FrameBundle{
		{FrameID: 0, TimestampNanos: 1000},
		{FrameID: 1, TimestampNanos: 2000},
		{FrameID: 2, TimestampNanos: 3000},
	}
	reader := newMockFrameReader(frames)

	// Seek to frame 2
	err := reader.Seek(2)
	if err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	if reader.CurrentFrame() != 2 {
		t.Errorf("expected CurrentFrame=2, got %d", reader.CurrentFrame())
	}

	// Seek past end
	err = reader.Seek(10)
	if err != io.EOF {
		t.Errorf("expected EOF when seeking past end, got %v", err)
	}
}

func TestMockFrameReader_TotalFrames(t *testing.T) {
	frames := []*FrameBundle{
		{FrameID: 0},
		{FrameID: 1},
		{FrameID: 2},
	}
	reader := newMockFrameReader(frames)

	total := reader.TotalFrames()
	if total != 3 {
		t.Errorf("expected TotalFrames=3, got %d", total)
	}
}

func TestReplayServer_StreamFrames_PausedResume(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	// Create test frames
	frames := []*FrameBundle{
		{
			FrameID:        0,
			TimestampNanos: 1000000000,
			SensorID:       "test",
			CoordinateFrame: CoordinateFrameInfo{
				FrameID:        "test/sensor",
				ReferenceFrame: "ENU",
			},
		},
		{
			FrameID:        1,
			TimestampNanos: 2000000000,
			SensorID:       "test",
			CoordinateFrame: CoordinateFrameInfo{
				FrameID:        "test/sensor",
				ReferenceFrame: "ENU",
			},
		},
	}
	reader := newMockFrameReader(frames)
	rs := NewReplayServer(pub, reader)

	// Start paused
	rs.paused = true

	ctx, cancel := context.WithCancel(context.Background())

	stream := newMockStreamServer(ctx)
	req := &pb.StreamRequest{
		SensorId:        "test",
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
	}

	// Run streaming in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- rs.StreamFrames(req, stream)
	}()

	// Let it run for a bit while paused
	time.Sleep(100 * time.Millisecond)

	// Resume
	rs.mu.Lock()
	rs.paused = false
	rs.mu.Unlock()

	// Let it stream
	time.Sleep(100 * time.Millisecond)

	// Cancel and wait
	cancel()
	err := <-done

	// Should get context cancelled error
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestReplayServer_Close_NilReader(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	rs := &ReplayServer{
		Server: NewServer(pub),
		reader: nil,
	}

	err := rs.Close()
	if err != nil {
		t.Errorf("Close() with nil reader should not error, got: %v", err)
	}
}

func TestReplayServer_streamFromReader_NilReader(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	rs := &ReplayServer{
		Server: NewServer(pub),
		reader: nil,
	}

	ctx := context.Background()
	stream := newMockStreamServer(ctx)
	req := &pb.StreamRequest{SensorId: "test"}

	err := rs.streamFromReader(ctx, req, stream)

	if err == nil {
		t.Error("expected error when reader is nil")
	}
	code := status.Code(err)
	if code != codes.Internal {
		t.Errorf("expected Internal error, got code %v", code)
	}
}

func TestMockFrameReader_SetRate(t *testing.T) {
	reader := newMockFrameReader([]*FrameBundle{})

	reader.SetRate(2.0)

	reader.mu.Lock()
	rate := reader.rate
	reader.mu.Unlock()

	if rate != 2.0 {
		t.Errorf("expected rate=2.0, got %f", rate)
	}
}

func TestMockFrameReader_SetPaused(t *testing.T) {
	reader := newMockFrameReader([]*FrameBundle{})

	reader.SetPaused(true)

	reader.mu.Lock()
	paused := reader.paused
	reader.mu.Unlock()

	if !paused {
		t.Error("expected paused=true")
	}
}

func TestMockFrameReader_ReadFrame_Closed(t *testing.T) {
	frames := []*FrameBundle{
		{FrameID: 0, TimestampNanos: 1000},
	}
	reader := newMockFrameReader(frames)

	// Close the reader
	reader.Close()

	// Try to read
	_, err := reader.ReadFrame()
	if err != io.EOF {
		t.Errorf("expected EOF after close, got %v", err)
	}
}

// TestReplayServer_StreamFrames_SkipsBackgroundWithManager verifies that
// background frames are skipped when the publisher has a background manager
// (e.g. the main cmd/radar path where live backgrounds are injected).
func TestReplayServer_StreamFrames_SkipsBackgroundWithManager(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	pub.SetBackgroundManager(&testReplayBackgroundManager{})

	frames := []*FrameBundle{
		{FrameID: 0, TimestampNanos: 1000000000, SensorID: "test", FrameType: FrameTypeForeground},
		{FrameID: 1, TimestampNanos: 2000000000, SensorID: "test", FrameType: FrameTypeBackground,
			Background: &BackgroundSnapshot{TimestampNanos: 2000000000}},
		{FrameID: 2, TimestampNanos: 3000000000, SensorID: "test", FrameType: FrameTypeForeground},
	}
	reader := newMockFrameReader(frames)
	rs := NewReplayServer(pub, reader)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockStreamServer(ctx)
	req := &pb.StreamRequest{SensorId: "test"}

	done := make(chan error, 1)
	go func() { done <- rs.StreamFrames(req, stream) }()

	// Wait for 2 foreground frames (background should be skipped)
	deadline := time.After(2 * time.Second)
	for {
		stream.mu.Lock()
		n := len(stream.frames)
		stream.mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for frames")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Give a brief window to ensure no extra frames arrive
	time.Sleep(50 * time.Millisecond)
	cancel()

	stream.mu.Lock()
	defer stream.mu.Unlock()

	// Should have exactly 2 foreground frames (background was skipped)
	if len(stream.frames) != 2 {
		t.Errorf("expected 2 frames (background skipped), got %d", len(stream.frames))
	}
	if stream.frames[0].FrameId != 0 {
		t.Errorf("first frame ID = %d, want 0", stream.frames[0].FrameId)
	}
	if stream.frames[1].FrameId != 2 {
		t.Errorf("second frame ID = %d, want 2 (skipping background frame 1)", stream.frames[1].FrameId)
	}
}

// TestReplayServer_StreamFrames_ForwardsBackgroundWithoutManager verifies
// that background frames are forwarded when no background manager exists
// (e.g. the standalone visualiser-server tool).
func TestReplayServer_StreamFrames_ForwardsBackgroundWithoutManager(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	// No SetBackgroundManager — simulates visualiser-server

	// Use timestamps 10ms apart to avoid rate-control sleeps exceeding the
	// test deadline (playbackRate defaults to 1.0×).
	frames := []*FrameBundle{
		{FrameID: 0, TimestampNanos: 1000000000, SensorID: "test", FrameType: FrameTypeForeground},
		{FrameID: 1, TimestampNanos: 1010000000, SensorID: "test", FrameType: FrameTypeBackground,
			Background: &BackgroundSnapshot{TimestampNanos: 1010000000}},
		{FrameID: 2, TimestampNanos: 1020000000, SensorID: "test", FrameType: FrameTypeForeground},
	}
	reader := newMockFrameReader(frames)
	rs := NewReplayServer(pub, reader)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockStreamServer(ctx)
	req := &pb.StreamRequest{SensorId: "test"}

	done := make(chan error, 1)
	go func() { done <- rs.StreamFrames(req, stream) }()

	// Wait for all 3 frames (including background)
	deadline := time.After(2 * time.Second)
	for {
		stream.mu.Lock()
		n := len(stream.frames)
		stream.mu.Unlock()
		if n >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for frames")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()

	stream.mu.Lock()
	defer stream.mu.Unlock()

	if len(stream.frames) != 3 {
		t.Errorf("expected 3 frames (background forwarded), got %d", len(stream.frames))
	}
}

// testReplayBackgroundManager implements BackgroundManagerInterface for testing.
type testReplayBackgroundManager struct{}

func (m *testReplayBackgroundManager) GenerateBackgroundSnapshot() (interface{}, error) {
	return &BackgroundSnapshot{TimestampNanos: time.Now().UnixNano()}, nil
}

func (m *testReplayBackgroundManager) GetBackgroundSequenceNumber() uint64 {
	return 1
}
