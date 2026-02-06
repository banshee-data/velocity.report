// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

// Add sync import at the top of the file
import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
	"google.golang.org/grpc/metadata"
)

func TestNewServer(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	if server == nil {
		t.Fatal("expected non-nil Server")
	}
	if server.publisher != pub {
		t.Error("expected publisher to be set")
	}
	if server.playbackRate != 1.0 {
		t.Errorf("expected playbackRate=1.0, got %f", server.playbackRate)
	}
	if server.paused {
		t.Error("expected paused=false by default")
	}
	if server.syntheticMode {
		t.Error("expected syntheticMode=false by default")
	}
}

func TestServer_EnableSyntheticMode(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	server.EnableSyntheticMode("test-sensor-01")

	if !server.syntheticMode {
		t.Error("expected syntheticMode=true after EnableSyntheticMode")
	}
	if server.syntheticGen == nil {
		t.Error("expected syntheticGen to be non-nil")
	}
	if server.SyntheticGenerator() == nil {
		t.Error("expected SyntheticGenerator() to return non-nil")
	}
}

func TestServer_SyntheticGenerator_NotEnabled(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	if server.SyntheticGenerator() != nil {
		t.Error("expected SyntheticGenerator() to return nil when not enabled")
	}
}

func TestServer_Pause(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	ctx := context.Background()
	status, err := server.Pause(ctx, &pb.PauseRequest{})

	if err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	if !status.Paused {
		t.Error("expected Paused=true after Pause")
	}
	if !server.paused {
		t.Error("expected server.paused=true after Pause")
	}
}

func TestServer_Play(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	// First pause
	server.paused = true

	ctx := context.Background()
	status, err := server.Play(ctx, &pb.PlayRequest{})

	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}
	if status.Paused {
		t.Error("expected Paused=false after Play")
	}
	if server.paused {
		t.Error("expected server.paused=false after Play")
	}
}

func TestServer_SetRate(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	ctx := context.Background()

	tests := []float32{0.5, 1.0, 2.0, 0.25}
	for _, rate := range tests {
		status, err := server.SetRate(ctx, &pb.SetRateRequest{Rate: rate})
		if err != nil {
			t.Fatalf("SetRate(%f) failed: %v", rate, err)
		}
		if status.Rate != rate {
			t.Errorf("expected Rate=%f, got %f", rate, status.Rate)
		}
		if server.playbackRate != rate {
			t.Errorf("expected server.playbackRate=%f, got %f", rate, server.playbackRate)
		}
	}
}

func TestServer_Seek_Unimplemented(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	ctx := context.Background()
	_, err := server.Seek(ctx, &pb.SeekRequest{})

	if err == nil {
		t.Error("expected error for unimplemented Seek")
	}
}

func TestServer_SetOverlayModes(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	ctx := context.Background()
	resp, err := server.SetOverlayModes(ctx, &pb.OverlayModeRequest{})

	if err != nil {
		t.Fatalf("SetOverlayModes failed: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}
}

func TestServer_GetCapabilities(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SensorID = "hesai-test"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	ctx := context.Background()
	caps, err := server.GetCapabilities(ctx, &pb.CapabilitiesRequest{})

	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}
	if !caps.SupportsPoints {
		t.Error("expected SupportsPoints=true")
	}
	if !caps.SupportsClusters {
		t.Error("expected SupportsClusters=true")
	}
	if !caps.SupportsTracks {
		t.Error("expected SupportsTracks=true")
	}
	if !caps.SupportsDebug {
		t.Error("expected SupportsDebug=true")
	}
	if caps.SupportsReplay {
		t.Error("expected SupportsReplay=false (not yet implemented)")
	}
	if caps.SupportsRecording {
		t.Error("expected SupportsRecording=false (not yet implemented)")
	}
	if len(caps.AvailableSensors) != 1 || caps.AvailableSensors[0] != "hesai-test" {
		t.Errorf("expected AvailableSensors=[hesai-test], got %v", caps.AvailableSensors)
	}
}

func TestServer_StartRecording_Unimplemented(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	ctx := context.Background()
	_, err := server.StartRecording(ctx, &pb.RecordingRequest{})

	if err == nil {
		t.Error("expected error for unimplemented StartRecording")
	}
}

func TestServer_StopRecording_Unimplemented(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	ctx := context.Background()
	_, err := server.StopRecording(ctx, &pb.RecordingRequest{})

	if err == nil {
		t.Error("expected error for unimplemented StopRecording")
	}
}

func TestServer_ImplementsInterface(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	// Verify server implements the interface
	var _ pb.VisualiserServiceServer = server
}

// Test that streaming synthetic works (basic sanity check).
func TestServer_StreamSynthetic(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)
	server.EnableSyntheticMode("test-sensor")

	// Configure for faster test
	server.syntheticGen.FrameRate = 100 // 100 Hz for fast test
	server.syntheticGen.PointCount = 100
	server.syntheticGen.TrackCount = 2

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock stream that captures frames
	frames := make([]*pb.FrameBundle, 0)
	var mu sync.Mutex

	mockStream := &mockSyntheticStream{
		ctx: ctx,
		send: func(frame *pb.FrameBundle) error {
			mu.Lock()
			frames = append(frames, frame)
			mu.Unlock()
			// Cancel after 3 frames to end the test quickly
			if len(frames) >= 3 {
				cancel()
			}
			return nil
		},
	}

	req := &pb.StreamRequest{
		SensorId:        "test-sensor",
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
	}

	// This will stream until cancelled
	err := server.StreamFrames(req, mockStream)

	// Should get context cancelled error
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	mu.Lock()
	frameCount := len(frames)
	mu.Unlock()

	// Should have received 3 frames
	if frameCount != 3 {
		t.Errorf("expected 3 frames, got %d", frameCount)
	}
}

// mockSyntheticStream is a simplified mock for testing synthetic streaming.
type mockSyntheticStream struct {
	ctx  context.Context
	send func(*pb.FrameBundle) error
}

func (m *mockSyntheticStream) Send(frame *pb.FrameBundle) error {
	return m.send(frame)
}

func (m *mockSyntheticStream) Context() context.Context {
	return m.ctx
}

func (m *mockSyntheticStream) SetHeader(md metadata.MD) error  { return nil }
func (m *mockSyntheticStream) SendHeader(md metadata.MD) error { return nil }
func (m *mockSyntheticStream) SetTrailer(md metadata.MD)       {}
func (m *mockSyntheticStream) SendMsg(msg interface{}) error   { return nil }
func (m *mockSyntheticStream) RecvMsg(msg interface{}) error   { return nil }

func TestByteSliceToUint32(t *testing.T) {
	tests := []struct {
		name  string
		input []uint8
		want  []uint32
	}{
		{
			name:  "empty slice",
			input: []uint8{},
			want:  []uint32{},
		},
		{
			name:  "single element",
			input: []uint8{42},
			want:  []uint32{42},
		},
		{
			name:  "multiple elements",
			input: []uint8{0, 127, 255},
			want:  []uint32{0, 127, 255},
		},
		{
			name:  "max values",
			input: []uint8{255, 255, 255},
			want:  []uint32{255, 255, 255},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := byteSliceToUint32(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("length mismatch: got %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFrameBundleToProto_EmptyFrame(t *testing.T) {
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/test",
			ReferenceFrame: "ENU",
		},
	}

	req := &pb.StreamRequest{
		IncludePoints:   false,
		IncludeClusters: false,
		IncludeTracks:   false,
	}

	pbFrame := frameBundleToProto(frame, req)

	if pbFrame.FrameId != 1 {
		t.Errorf("expected FrameId=1, got %d", pbFrame.FrameId)
	}
	if pbFrame.SensorId != "test-sensor" {
		t.Errorf("expected SensorId=test-sensor, got %s", pbFrame.SensorId)
	}
	if pbFrame.PointCloud != nil {
		t.Error("expected nil PointCloud when IncludePoints=false")
	}
	if pbFrame.Clusters != nil {
		t.Error("expected nil Clusters when IncludeClusters=false")
	}
	if pbFrame.Tracks != nil {
		t.Error("expected nil Tracks when IncludeTracks=false")
	}
}

func TestFrameBundleToProto_WithPointCloud(t *testing.T) {
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/test",
			ReferenceFrame: "ENU",
		},
		PointCloud: &PointCloudFrame{
			FrameID:        1,
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
			X:              []float32{1.0, 2.0, 3.0},
			Y:              []float32{4.0, 5.0, 6.0},
			Z:              []float32{7.0, 8.0, 9.0},
			Intensity:      []uint8{100, 150, 200},
			Classification: []uint8{0, 1, 0},
			PointCount:     3,
		},
	}

	req := &pb.StreamRequest{
		IncludePoints:   true,
		IncludeClusters: false,
		IncludeTracks:   false,
	}

	pbFrame := frameBundleToProto(frame, req)

	if pbFrame.PointCloud == nil {
		t.Fatal("expected non-nil PointCloud")
	}
	if pbFrame.PointCloud.PointCount != 3 {
		t.Errorf("expected PointCount=3, got %d", pbFrame.PointCloud.PointCount)
	}
	if len(pbFrame.PointCloud.X) != 3 {
		t.Errorf("expected X length=3, got %d", len(pbFrame.PointCloud.X))
	}
}

func TestFrameBundleToProto_WithClusters(t *testing.T) {
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/test",
			ReferenceFrame: "ENU",
		},
		Clusters: &ClusterSet{
			FrameID:        1,
			TimestampNanos: time.Now().UnixNano(),
			Clusters: []Cluster{
				{
					ClusterID: 1,
					CentroidX: 10.0,
					CentroidY: 20.0,
					CentroidZ: 0.8,
				},
				{
					ClusterID: 2,
					CentroidX: 30.0,
					CentroidY: 40.0,
					CentroidZ: 0.9,
				},
			},
			Method: ClusteringDBSCAN,
		},
	}

	req := &pb.StreamRequest{
		IncludePoints:   false,
		IncludeClusters: true,
		IncludeTracks:   false,
	}

	pbFrame := frameBundleToProto(frame, req)

	if pbFrame.Clusters == nil {
		t.Fatal("expected non-nil Clusters")
	}
	if len(pbFrame.Clusters.Clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(pbFrame.Clusters.Clusters))
	}
	if pbFrame.Clusters.Clusters[0].ClusterId != 1 {
		t.Errorf("expected ClusterId=1, got %d", pbFrame.Clusters.Clusters[0].ClusterId)
	}
}

func TestFrameBundleToProto_WithTracks(t *testing.T) {
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/test",
			ReferenceFrame: "ENU",
		},
		Tracks: &TrackSet{
			FrameID:        1,
			TimestampNanos: time.Now().UnixNano(),
			Tracks: []Track{
				{
					TrackID:  "track-001",
					SensorID: "test-sensor",
					State:    TrackStateConfirmed,
					X:        15.0,
					Y:        25.0,
					SpeedMps: 5.0,
				},
			},
			Trails: []TrackTrail{
				{
					TrackID: "track-001",
					Points: []TrackPoint{
						{X: 14.0, Y: 24.0, TimestampNanos: 1000000000},
						{X: 15.0, Y: 25.0, TimestampNanos: 1100000000},
					},
				},
			},
		},
	}

	req := &pb.StreamRequest{
		IncludePoints:   false,
		IncludeClusters: false,
		IncludeTracks:   true,
	}

	pbFrame := frameBundleToProto(frame, req)

	if pbFrame.Tracks == nil {
		t.Fatal("expected non-nil Tracks")
	}
	if len(pbFrame.Tracks.Tracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(pbFrame.Tracks.Tracks))
	}
	if pbFrame.Tracks.Tracks[0].TrackId != "track-001" {
		t.Errorf("expected TrackId=track-001, got %s", pbFrame.Tracks.Tracks[0].TrackId)
	}
	if len(pbFrame.Tracks.Trails) != 1 {
		t.Errorf("expected 1 trail, got %d", len(pbFrame.Tracks.Trails))
	}
	if len(pbFrame.Tracks.Trails[0].Points) != 2 {
		t.Errorf("expected 2 trail points, got %d", len(pbFrame.Tracks.Trails[0].Points))
	}
}

func TestFrameBundleToProto_WithPlaybackInfo(t *testing.T) {
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/test",
			ReferenceFrame: "ENU",
		},
		PlaybackInfo: &PlaybackInfo{
			IsLive:       true,
			LogStartNs:   1000000000,
			LogEndNs:     2000000000,
			PlaybackRate: 1.5,
			Paused:       false,
		},
	}

	req := &pb.StreamRequest{}

	pbFrame := frameBundleToProto(frame, req)

	if pbFrame.PlaybackInfo == nil {
		t.Fatal("expected non-nil PlaybackInfo")
	}
	if !pbFrame.PlaybackInfo.IsLive {
		t.Error("expected IsLive=true")
	}
	if pbFrame.PlaybackInfo.PlaybackRate != 1.5 {
		t.Errorf("expected PlaybackRate=1.5, got %f", pbFrame.PlaybackInfo.PlaybackRate)
	}
}

func TestServer_RegisterService(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0" // Use dynamic port to avoid conflicts
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	// Start publisher to initialise grpc server
	if err := pub.Start(); err != nil {
		t.Fatalf("failed to start publisher: %v", err)
	}
	defer pub.Stop()

	// Register the service using the standalone function
	RegisterService(pub.GRPCServer(), server)

	// If we get here without panic, registration succeeded
}

func TestPublisher_GRPCServer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0" // Use dynamic port to avoid conflicts
	pub := NewPublisher(cfg)

	// Before start, GRPCServer should be nil
	if pub.GRPCServer() != nil {
		t.Error("expected nil GRPCServer before Start")
	}

	// Start publisher
	if err := pub.Start(); err != nil {
		t.Fatalf("failed to start publisher: %v", err)
	}
	defer pub.Stop()

	// After start, GRPCServer should be non-nil
	if pub.GRPCServer() == nil {
		t.Error("expected non-nil GRPCServer after Start")
	}
}

// --- sendCooldown tests (§7.3 hysteresis frame-skip control) ---

func TestSendCooldown_StartsInNormalMode(t *testing.T) {
	sc := newSendCooldown(3, 5)
	if sc.inSkipMode() {
		t.Error("expected normal mode initially")
	}
}

func TestSendCooldown_EntersSkipModeAfterMaxSlow(t *testing.T) {
	sc := newSendCooldown(3, 5)

	// 2 slow sends — not yet in skip mode
	sc.recordSlow()
	sc.recordSlow()
	if sc.inSkipMode() {
		t.Error("should not be in skip mode after 2 slow sends (threshold=3)")
	}

	// 3rd slow send — enters skip mode
	sc.recordSlow()
	if !sc.inSkipMode() {
		t.Error("expected skip mode after 3 slow sends")
	}
}

func TestSendCooldown_RequiresMinFastToExitSkipMode(t *testing.T) {
	sc := newSendCooldown(3, 5)

	// Enter skip mode
	for i := 0; i < 3; i++ {
		sc.recordSlow()
	}
	if !sc.inSkipMode() {
		t.Fatal("precondition: should be in skip mode")
	}

	// 4 fast sends — not enough to exit (need 5)
	for i := 0; i < 4; i++ {
		sc.recordFast()
		if !sc.inSkipMode() {
			t.Errorf("should still be in skip mode after %d fast sends (need 5)", i+1)
		}
	}

	// 5th fast send — exits skip mode
	sc.recordFast()
	if sc.inSkipMode() {
		t.Error("expected normal mode after 5 consecutive fast sends")
	}
}

func TestSendCooldown_SlowSendResetsInSkipFastCounter(t *testing.T) {
	sc := newSendCooldown(3, 5)

	// Enter skip mode
	for i := 0; i < 3; i++ {
		sc.recordSlow()
	}

	// 4 fast sends, then a slow send interrupts
	for i := 0; i < 4; i++ {
		sc.recordFast()
	}
	sc.recordSlow() // Resets fast counter

	// Need 5 more fast sends from scratch
	for i := 0; i < 4; i++ {
		sc.recordFast()
		if !sc.inSkipMode() {
			t.Errorf("should still be in skip mode after interrupted recovery (%d fast)", i+1)
		}
	}
	sc.recordFast()
	if sc.inSkipMode() {
		t.Error("expected normal mode after 5 consecutive fast sends (post-interrupt)")
	}
}

func TestSendCooldown_SingleFastSendDoesNotExitSkipMode(t *testing.T) {
	sc := newSendCooldown(3, 5)

	// Enter skip mode
	for i := 0; i < 3; i++ {
		sc.recordSlow()
	}

	// A single fast send should NOT exit skip mode (the old behaviour)
	sc.recordFast()
	if !sc.inSkipMode() {
		t.Error("a single fast send should not exit skip mode (hysteresis)")
	}
}

func TestSendCooldown_NormalMode_SlowResetByFast(t *testing.T) {
	sc := newSendCooldown(3, 5)

	// 2 slow sends (not yet skip mode), then a fast send
	sc.recordSlow()
	sc.recordSlow()
	sc.recordFast()

	// Should still be in normal mode, and slow counter should be reset
	if sc.inSkipMode() {
		t.Error("should be in normal mode")
	}

	// Need full 3 slow sends to enter skip mode again
	sc.recordSlow()
	sc.recordSlow()
	if sc.inSkipMode() {
		t.Error("should not yet be in skip mode (only 2 slow since reset)")
	}
	sc.recordSlow()
	if !sc.inSkipMode() {
		t.Error("expected skip mode after 3 consecutive slow sends")
	}
}

func TestSendCooldown_RecordSlowReturnValue(t *testing.T) {
	sc := newSendCooldown(2, 3)

	if sc.recordSlow() {
		t.Error("first slow send should not return skip=true")
	}
	if !sc.recordSlow() {
		t.Error("second slow send should return skip=true (threshold=2)")
	}
}

func TestSendCooldown_RecordFastReturnValue(t *testing.T) {
	sc := newSendCooldown(1, 2)

	// Enter skip mode
	sc.recordSlow()

	if !sc.recordFast() {
		t.Error("first fast in skip mode should return still-skipping=true")
	}
	if sc.recordFast() {
		t.Error("second fast in skip mode should return still-skipping=false (minFast=2)")
	}
}
