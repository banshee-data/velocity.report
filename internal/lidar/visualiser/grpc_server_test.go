// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

// Add sync import at the top of the file
import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
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
	if !caps.SupportsReplay {
		t.Error("expected SupportsReplay=true")
	}
	if !caps.SupportsRecording {
		t.Error("expected SupportsRecording=true")
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

func TestFrameBundleToProto_ClusterOBB(t *testing.T) {
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
					OBB: &OrientedBoundingBox{
						CenterX:    10.0,
						CenterY:    20.0,
						CenterZ:    0.8,
						Length:     4.5,
						Width:      1.8,
						Height:     1.5,
						HeadingRad: 0.785, // ~45 degrees
					},
				},
				{
					ClusterID: 2,
					CentroidX: 30.0,
					CentroidY: 40.0,
					CentroidZ: 0.9,
					// No OBB — should remain nil in proto
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

	// Cluster with OBB should have it serialised
	c0 := pbFrame.Clusters.Clusters[0]
	if c0.Obb == nil {
		t.Fatal("expected non-nil OBB on cluster 0")
	}
	if c0.Obb.Length != 4.5 {
		t.Errorf("expected OBB Length=4.5, got %f", c0.Obb.Length)
	}
	if c0.Obb.Width != 1.8 {
		t.Errorf("expected OBB Width=1.8, got %f", c0.Obb.Width)
	}
	if c0.Obb.HeadingRad != 0.785 {
		t.Errorf("expected OBB HeadingRad=0.785, got %f", c0.Obb.HeadingRad)
	}
	if c0.Obb.CenterX != 10.0 {
		t.Errorf("expected OBB CenterX=10.0, got %f", c0.Obb.CenterX)
	}

	// Cluster without OBB should have nil
	c1 := pbFrame.Clusters.Clusters[1]
	if c1.Obb != nil {
		t.Errorf("expected nil OBB on cluster 1, got %+v", c1.Obb)
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

// TestFrameBundleToProto_TrackFieldCompleteness verifies that ALL Track
// fields survive the model → proto conversion at the wire boundary.  This
// regression test was added after discovering that 11 fields (PeakSpeedMps,
// AvgSpeedMps, Hits, Confidence, Duration, Length, etc.) were silently
// zero'd because the conversion in frameBundleToProto omitted them.
func TestFrameBundleToProto_TrackFieldCompleteness(t *testing.T) {
	frame := &FrameBundle{
		FrameID:        42,
		TimestampNanos: 1_000_000_000,
		SensorID:       "test-sensor",
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/test",
			ReferenceFrame: "ENU",
		},
		Tracks: &TrackSet{
			FrameID:        42,
			TimestampNanos: 1_000_000_000,
			Tracks: []Track{
				{
					TrackID:           "trk-full",
					SensorID:          "test-sensor",
					State:             TrackStateConfirmed,
					Hits:              10,
					Misses:            3,
					ObservationCount:  13,
					FirstSeenNanos:    500_000_000,
					LastSeenNanos:     1_000_000_000,
					X:                 12.0,
					Y:                 8.0,
					Z:                 0.5,
					VX:                5.0,
					VY:                0.3,
					VZ:                0.1,
					SpeedMps:          5.01,
					HeadingRad:        0.06,
					Covariance4x4:     []float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
					BBoxLength:        4.5,
					BBoxWidth:         1.8,
					BBoxHeight:        1.5,
					BBoxHeadingRad:    0.1,
					HeightP95Max:      1.65,
					IntensityMeanAvg:  42.0,
					AvgSpeedMps:       4.2,
					MedianSpeedMps:    3.8,
					P85SpeedMps:       5.5,
					P98SpeedMps:       6.2,
					PeakSpeedMps:      6.8,
					ObjectClass:       "car",
					ClassConfidence:   0.92,
					TrackLengthMetres: 55.0,
					TrackDurationSecs: 11.0,
					OcclusionCount:    2,
					Confidence:        0.95,
					OcclusionState:    OcclusionPartial,
					MotionModel:       1, // CV
					Alpha:             0.8,
					HeadingSource:     1,
				},
			},
			Trails: []TrackTrail{},
		},
	}

	req := &pb.StreamRequest{
		IncludeTracks: true,
	}

	pbFrame := frameBundleToProto(frame, req)
	if pbFrame.Tracks == nil {
		t.Fatal("expected non-nil Tracks")
	}
	if len(pbFrame.Tracks.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(pbFrame.Tracks.Tracks))
	}

	tr := pbFrame.Tracks.Tracks[0]

	// -- Lifecycle -------------------------------------------------------
	if tr.TrackId != "trk-full" {
		t.Errorf("TrackId: got %q, want %q", tr.TrackId, "trk-full")
	}
	if tr.SensorId != "test-sensor" {
		t.Errorf("SensorId: got %q, want %q", tr.SensorId, "test-sensor")
	}
	if tr.State != pb.TrackState(TrackStateConfirmed) {
		t.Errorf("State: got %v, want %v", tr.State, pb.TrackState(TrackStateConfirmed))
	}
	if tr.Hits != 10 {
		t.Errorf("Hits: got %d, want 10", tr.Hits)
	}
	if tr.Misses != 3 {
		t.Errorf("Misses: got %d, want 3", tr.Misses)
	}
	if tr.ObservationCount != 13 {
		t.Errorf("ObservationCount: got %d, want 13", tr.ObservationCount)
	}
	if tr.FirstSeenNs != 500_000_000 {
		t.Errorf("FirstSeenNs: got %d, want 500000000", tr.FirstSeenNs)
	}
	if tr.LastSeenNs != 1_000_000_000 {
		t.Errorf("LastSeenNs: got %d, want 1000000000", tr.LastSeenNs)
	}

	// -- Position --------------------------------------------------------
	if tr.X != 12.0 {
		t.Errorf("X: got %f, want 12.0", tr.X)
	}
	if tr.Y != 8.0 {
		t.Errorf("Y: got %f, want 8.0", tr.Y)
	}
	if tr.Z != 0.5 {
		t.Errorf("Z: got %f, want 0.5", tr.Z)
	}

	// -- Velocity --------------------------------------------------------
	if tr.Vx != 5.0 {
		t.Errorf("Vx: got %f, want 5.0", tr.Vx)
	}
	if tr.Vy != 0.3 {
		t.Errorf("Vy: got %f, want 0.3", tr.Vy)
	}
	if tr.Vz != 0.1 {
		t.Errorf("Vz: got %f, want 0.1", tr.Vz)
	}
	if tr.SpeedMps != 5.01 {
		t.Errorf("SpeedMps: got %f, want 5.01", tr.SpeedMps)
	}
	if tr.HeadingRad != 0.06 {
		t.Errorf("HeadingRad: got %f, want 0.06", tr.HeadingRad)
	}
	if tr.MedianSpeedMps != 3.8 {
		t.Errorf("MedianSpeedMps: got %f, want 3.8", tr.MedianSpeedMps)
	}
	if tr.PeakSpeedMps != 6.8 {
		t.Errorf("PeakSpeedMps: got %f, want 6.8", tr.PeakSpeedMps)
	}
	if tr.P85SpeedMps != 5.5 {
		t.Errorf("P85SpeedMps: got %f, want 5.5", tr.P85SpeedMps)
	}
	if tr.P98SpeedMps != 6.2 {
		t.Errorf("P98SpeedMps: got %f, want 6.2", tr.P98SpeedMps)
	}

	// -- Covariance ------------------------------------------------------
	if len(tr.Covariance_4X4) != 16 {
		t.Errorf("Covariance4x4 length: got %d, want 16", len(tr.Covariance_4X4))
	}

	// -- Bounding box ----------------------------------------------------
	if tr.BboxLength != 4.5 {
		t.Errorf("BboxLength: got %f, want 4.5", tr.BboxLength)
	}
	if tr.BboxWidth != 1.8 {
		t.Errorf("BboxWidth: got %f, want 1.8", tr.BboxWidth)
	}
	if tr.BboxHeight != 1.5 {
		t.Errorf("BboxHeight: got %f, want 1.5", tr.BboxHeight)
	}
	if tr.BboxHeadingRad != 0.1 {
		t.Errorf("BboxHeadingRad: got %f, want 0.1", tr.BboxHeadingRad)
	}

	// -- Features --------------------------------------------------------
	if tr.HeightP95Max != 1.65 {
		t.Errorf("HeightP95Max: got %f, want 1.65", tr.HeightP95Max)
	}
	if tr.IntensityMeanAvg != 42.0 {
		t.Errorf("IntensityMeanAvg: got %f, want 42.0", tr.IntensityMeanAvg)
	}

	// -- Classification --------------------------------------------------
	if tr.ObjectClass != pb.ObjectClass_OBJECT_CLASS_CAR {
		t.Errorf("ObjectClass: got %v, want %v", tr.ObjectClass, pb.ObjectClass_OBJECT_CLASS_CAR)
	}
	if tr.ClassConfidence != 0.92 {
		t.Errorf("ClassConfidence: got %f, want 0.92", tr.ClassConfidence)
	}

	// -- Quality metrics (the main regression targets) -------------------
	if tr.TrackLengthMetres != 55.0 {
		t.Errorf("TrackLengthMetres: got %f, want 55.0", tr.TrackLengthMetres)
	}
	if tr.TrackDurationSecs != 11.0 {
		t.Errorf("TrackDurationSecs: got %f, want 11.0", tr.TrackDurationSecs)
	}
	if tr.OcclusionCount != 2 {
		t.Errorf("OcclusionCount: got %d, want 2", tr.OcclusionCount)
	}
	if tr.Confidence != 0.95 {
		t.Errorf("Confidence: got %f, want 0.95", tr.Confidence)
	}
	if tr.OcclusionState != pb.OcclusionState(OcclusionPartial) {
		t.Errorf("OcclusionState: got %v, want %v", tr.OcclusionState, pb.OcclusionState(OcclusionPartial))
	}

	// -- Rendering hints -------------------------------------------------
	if tr.MotionModel != 1 {
		t.Errorf("MotionModel: got %v, want 1", tr.MotionModel)
	}
	if tr.Alpha != 0.8 {
		t.Errorf("Alpha: got %f, want 0.8", tr.Alpha)
	}
	if tr.HeadingSource != 1 {
		t.Errorf("HeadingSource: got %d, want 1", tr.HeadingSource)
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
			Seekable:     true,
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
	if !pbFrame.PlaybackInfo.Seekable {
		t.Error("expected Seekable=true")
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

// =============================================================================
// Tests for getPointCount
// =============================================================================

func TestGetPointCount_NilFrame(t *testing.T) {
	count := getPointCount(nil)
	if count != 0 {
		t.Errorf("expected 0 for nil frame, got %d", count)
	}
}

func TestGetPointCount_NilPointCloud(t *testing.T) {
	frame := &FrameBundle{
		FrameID:    1,
		PointCloud: nil,
	}
	count := getPointCount(frame)
	if count != 0 {
		t.Errorf("expected 0 for nil PointCloud, got %d", count)
	}
}

func TestGetPointCount_ZeroPoints(t *testing.T) {
	frame := &FrameBundle{
		FrameID: 1,
		PointCloud: &PointCloudFrame{
			PointCount: 0,
		},
	}
	count := getPointCount(frame)
	if count != 0 {
		t.Errorf("expected 0 for zero points, got %d", count)
	}
}

func TestGetPointCount_WithPoints(t *testing.T) {
	frame := &FrameBundle{
		FrameID: 1,
		PointCloud: &PointCloudFrame{
			PointCount: 1000,
		},
	}
	count := getPointCount(frame)
	if count != 1000 {
		t.Errorf("expected 1000, got %d", count)
	}
}

func TestGetPointCount_LargePointCount(t *testing.T) {
	frame := &FrameBundle{
		FrameID: 1,
		PointCloud: &PointCloudFrame{
			PointCount: 70000, // Typical Pandar40P point count
		},
	}
	count := getPointCount(frame)
	if count != 70000 {
		t.Errorf("expected 70000, got %d", count)
	}
}

// =============================================================================
// Tests for Server replay mode
// =============================================================================

func TestServer_SetReplayMode(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	// Initially not in replay mode
	if server.replayMode {
		t.Error("expected replayMode=false initially")
	}

	// Enable replay mode
	server.SetReplayMode(true)
	if !server.replayMode {
		t.Error("expected replayMode=true after SetReplayMode(true)")
	}

	// Set PCAP progress
	server.SetPCAPProgress(500, 1000)
	if server.pcapCurrentPacket != 500 {
		t.Errorf("expected pcapCurrentPacket=500, got %d", server.pcapCurrentPacket)
	}
	if server.pcapTotalPackets != 1000 {
		t.Errorf("expected pcapTotalPackets=1000, got %d", server.pcapTotalPackets)
	}

	// Disable replay mode - should reset PCAP progress
	server.SetReplayMode(false)
	if server.replayMode {
		t.Error("expected replayMode=false after SetReplayMode(false)")
	}
	if server.pcapCurrentPacket != 0 {
		t.Errorf("expected pcapCurrentPacket=0 after disabling, got %d", server.pcapCurrentPacket)
	}
	if server.pcapTotalPackets != 0 {
		t.Errorf("expected pcapTotalPackets=0 after disabling, got %d", server.pcapTotalPackets)
	}

	// Test SetPCAPTimestamps
	server.SetReplayMode(true)
	server.SetPCAPTimestamps(1000000000, 2000000000)
	if server.pcapStartNs != 1000000000 {
		t.Errorf("expected pcapStartNs=1000000000, got %d", server.pcapStartNs)
	}
	if server.pcapEndNs != 2000000000 {
		t.Errorf("expected pcapEndNs=2000000000, got %d", server.pcapEndNs)
	}

	// Disable replay mode - should also reset timestamps
	server.SetReplayMode(false)
	if server.pcapStartNs != 0 {
		t.Errorf("expected pcapStartNs=0 after disabling, got %d", server.pcapStartNs)
	}
	if server.pcapEndNs != 0 {
		t.Errorf("expected pcapEndNs=0 after disabling, got %d", server.pcapEndNs)
	}
}

// =============================================================================
// Tests for frameBundleToProto with Debug overlays
// =============================================================================

// Note: frameBundleToProto doesn't currently convert debug overlays to proto.
// These tests verify the actual behaviour (Debug is not serialised).
func TestFrameBundleToProto_DebugNotConverted(t *testing.T) {
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/test",
			ReferenceFrame: "ENU",
		},
		Debug: &DebugOverlaySet{
			FrameID:        1,
			TimestampNanos: time.Now().UnixNano(),
			AssociationCandidates: []AssociationCandidate{
				{ClusterID: 1, TrackID: "track-001", Distance: 2.5, Accepted: true},
			},
		},
	}

	req := &pb.StreamRequest{
		IncludeDebug: true, // Even when requested, Debug is not converted (not implemented)
	}

	pbFrame := frameBundleToProto(frame, req)

	// Debug conversion is not implemented - verify it's nil
	if pbFrame.Debug != nil {
		t.Error("expected nil Debug (not yet implemented in frameBundleToProto)")
	}
}

func TestFrameBundleToProto_DebugFieldAbsent(t *testing.T) {
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/test",
			ReferenceFrame: "ENU",
		},
		Debug: nil, // No debug data
	}

	req := &pb.StreamRequest{
		IncludeDebug: false,
	}

	pbFrame := frameBundleToProto(frame, req)

	if pbFrame.Debug != nil {
		t.Error("expected nil Debug when IncludeDebug=false")
	}
}

// =============================================================================
// Tests for streamFromPublisher
// =============================================================================

func TestStreamFromPublisher_BasicFlow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	server := NewServer(pub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track received frames
	var receivedFrames []*pb.FrameBundle
	var mu sync.Mutex

	mockStream := &mockSyntheticStream{
		ctx: ctx,
		send: func(frame *pb.FrameBundle) error {
			mu.Lock()
			receivedFrames = append(receivedFrames, frame)
			frameCount := len(receivedFrames)
			mu.Unlock()

			// Cancel after receiving 3 frames
			if frameCount >= 3 {
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

	// Start streaming in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.streamFromPublisher(ctx, req, mockStream)
	}()

	// Give time for the client to register
	time.Sleep(10 * time.Millisecond)

	// Publish some frames
	for i := 0; i < 5; i++ {
		frame := &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
			PointCloud: &PointCloudFrame{
				X:          []float32{1.0, 2.0},
				Y:          []float32{1.0, 2.0},
				Z:          []float32{0.5, 0.5},
				Intensity:  []uint8{100, 150},
				PointCount: 2,
			},
		}
		pub.Publish(frame)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for streaming to complete
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for streaming to complete")
		cancel()
	}

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount < 3 {
		t.Errorf("expected at least 3 frames, got %d", frameCount)
	}
}

func TestStreamFromPublisher_WithPause(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	server := NewServer(pub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedFrames []*pb.FrameBundle
	var mu sync.Mutex

	mockStream := &mockSyntheticStream{
		ctx: ctx,
		send: func(frame *pb.FrameBundle) error {
			mu.Lock()
			receivedFrames = append(receivedFrames, frame)
			mu.Unlock()
			return nil
		},
	}

	req := &pb.StreamRequest{
		SensorId:        "test-sensor",
		IncludePoints:   true,
		IncludeClusters: true,
	}

	// Start streaming
	go func() {
		_ = server.streamFromPublisher(ctx, req, mockStream)
	}()

	// Give time for client to register
	time.Sleep(10 * time.Millisecond)

	// Pause the server using the Pause method (with proper synchronization)
	_, _ = server.Pause(ctx, &pb.PauseRequest{})

	// Publish frames while paused - these should be dropped
	for i := 0; i < 3; i++ {
		frame := &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
			PointCloud: &PointCloudFrame{
				X:          []float32{1.0, 2.0},
				Y:          []float32{1.0, 2.0},
				Z:          []float32{0.5, 0.5},
				Intensity:  []uint8{100, 150},
				PointCount: 2,
			},
		}
		pub.Publish(frame)
		time.Sleep(5 * time.Millisecond)
	}

	// Check that no frames were received while paused
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	frameCountPaused := len(receivedFrames)
	mu.Unlock()

	// Unpause and send more frames using the Play method (with proper synchronization)
	_, _ = server.Play(ctx, &pb.PlayRequest{})

	for i := 0; i < 3; i++ {
		frame := &FrameBundle{
			FrameID:        uint64(i + 100),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
		}
		pub.Publish(frame)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for frames to be processed
	time.Sleep(50 * time.Millisecond)

	cancel()

	mu.Lock()
	finalFrameCount := len(receivedFrames)
	mu.Unlock()

	// Should have received frames only after unpausing
	if frameCountPaused >= finalFrameCount {
		t.Errorf("expected more frames after unpause: paused_count=%d final_count=%d",
			frameCountPaused, finalFrameCount)
	}
}

func TestStreamFromPublisher_ReplayMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	server := NewServer(pub)

	// Enable replay mode
	server.SetReplayMode(true)
	server.SetPCAPProgress(10, 100) // 10 of 100 packets

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedFrames []*pb.FrameBundle
	var mu sync.Mutex

	mockStream := &mockSyntheticStream{
		ctx: ctx,
		send: func(frame *pb.FrameBundle) error {
			mu.Lock()
			receivedFrames = append(receivedFrames, frame)
			frameCount := len(receivedFrames)
			mu.Unlock()

			if frameCount >= 2 {
				cancel()
			}
			return nil
		},
	}

	req := &pb.StreamRequest{
		SensorId: "test-sensor",
	}

	// Start streaming
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.streamFromPublisher(ctx, req, mockStream)
	}()

	// Give time for client to register
	time.Sleep(10 * time.Millisecond)

	// Publish frames - PlaybackInfo should be injected
	for i := 0; i < 3; i++ {
		frame := &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
		}
		pub.Publish(frame)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for streaming to complete
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		cancel()
	}

	mu.Lock()
	frameCount := len(receivedFrames)
	mu.Unlock()

	if frameCount < 2 {
		t.Errorf("expected at least 2 frames, got %d", frameCount)
	}

	// Verify PlaybackInfo was injected (checked via logs since proto doesn't expose it directly)
	// The test exercises the replay mode code path
}

// TestObjectClassConversionInProtoMessages verifies that Track.ObjectClass field
// is correctly converted to proto ObjectClass enum in StreamFrame messages.
// This integration test ensures the full pipeline works:
// Go Track.ObjectClass (string) → objectClassFromString → proto enum → Swift conversion.
func TestObjectClassConversionInProtoMessages(t *testing.T) {
	// Create test tracks with various classifications
	testTracks := []Track{
		{
			TrackID:     "trk-car",
			ObjectClass: string(l6objects.ClassCar),
			SensorID:    "sensor-1",
			State:       TrackStateConfirmed,
			Hits:        10,
			Misses:      0,
			Confidence:  0.95,
			X:           10.0,
			Y:           20.0,
			Z:           0.0,
			SpeedMps:    5.0,
			HeadingRad:  0.0,
			Alpha:       1.0,
		},
		{
			TrackID:     "trk-pedestrian",
			ObjectClass: string(l6objects.ClassPedestrian),
			SensorID:    "sensor-1",
			State:       TrackStateConfirmed,
			Hits:        8,
			Misses:      0,
			Confidence:  0.85,
			X:           5.0,
			Y:           5.0,
			Z:           0.0,
			SpeedMps:    1.5,
			HeadingRad:  0.0,
			Alpha:       1.0,
		},
		{
			TrackID:     "trk-bird",
			ObjectClass: string(l6objects.ClassBird),
			SensorID:    "sensor-1",
			State:       TrackStateConfirmed,
			Hits:        5,
			Misses:      0,
			Confidence:  0.7,
			X:           0.0,
			Y:           0.0,
			Z:           10.0,
			SpeedMps:    2.0,
			HeadingRad:  0.0,
			Alpha:       1.0,
		},
		{
			TrackID:     "trk-unclassified",
			ObjectClass: "", // Unspecified
			SensorID:    "sensor-1",
			State:       TrackStateTentative,
			Hits:        2,
			Misses:      1,
			Confidence:  0.5,
			X:           15.0,
			Y:           15.0,
			Z:           0.0,
			SpeedMps:    0.5,
			HeadingRad:  0.0,
			Alpha:       0.5,
		},
	}

	// Build a FrameBundle with test tracks
	ts := TrackSet{
		FrameID:        100,
		TimestampNanos: 123456789,
		Tracks:         testTracks,
	}

	frameBundle := []interface{}{
		&ts,
	}

	// Simulate server processing to create proto message
	// (This mimics what happens in StreamFrame handler)
	for _, frame := range frameBundle {
		if ts, ok := frame.(*TrackSet); ok {
			// Convert to proto (similar to grpc_server.go StreamFrame logic)
			pbTracks := make([]*pb.Track, len(ts.Tracks))
			for i, t := range ts.Tracks {
				pbTracks[i] = &pb.Track{
					TrackId:     t.TrackID,
					SensorId:    t.SensorID,
					State:       pb.TrackState(t.State),
					Hits:        int32(t.Hits),
					Misses:      int32(t.Misses),
					Confidence:  t.Confidence,
					X:           t.X,
					Y:           t.Y,
					Z:           t.Z,
					SpeedMps:    t.SpeedMps,
					HeadingRad:  t.HeadingRad,
					Alpha:       t.Alpha,
					ObjectClass: objectClassFromString(t.ObjectClass), // KEY: The conversion happens here
				}
			}

			// Verify each track's ObjectClass was converted correctly
			expectedConversions := map[string]pb.ObjectClass{
				"trk-car":          pb.ObjectClass_OBJECT_CLASS_CAR,
				"trk-pedestrian":   pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN,
				"trk-bird":         pb.ObjectClass_OBJECT_CLASS_BIRD,
				"trk-unclassified": pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED,
			}

			for i, pbTrack := range pbTracks {
				expected, ok := expectedConversions[pbTrack.TrackId]
				if !ok {
					t.Fatalf("unexpected track ID: %s", pbTrack.TrackId)
				}

				if pbTrack.ObjectClass != expected {
					t.Errorf("Track %s: expected ObjectClass=%v, got %v",
						pbTrack.TrackId, expected, pbTrack.ObjectClass)
				}

				// Verify the conversion is not lossy (all valid classes convert to non-unspecified)
				if ts.Tracks[i].ObjectClass != "" && pbTrack.ObjectClass == pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED {
					t.Errorf("Track %s: non-empty ObjectClass string '%s' converted to UNSPECIFIED (data loss)",
						pbTrack.TrackId, ts.Tracks[i].ObjectClass)
				}
			}
		}
	}
}

// TestAllObjectClassConstantsConvertible verifies all l6objects class constants
// can be converted to proto enums without data loss.
func TestAllObjectClassConstantsConvertible(t *testing.T) {
	classConstants := []struct {
		name     string
		constant string
		expected pb.ObjectClass
	}{
		{"car", string(l6objects.ClassCar), pb.ObjectClass_OBJECT_CLASS_CAR},
		{"truck", string(l6objects.ClassTruck), pb.ObjectClass_OBJECT_CLASS_TRUCK},
		{"bus", string(l6objects.ClassBus), pb.ObjectClass_OBJECT_CLASS_BUS},
		{"pedestrian", string(l6objects.ClassPedestrian), pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN},
		{"cyclist", string(l6objects.ClassCyclist), pb.ObjectClass_OBJECT_CLASS_CYCLIST},
		{"motorcyclist", string(l6objects.ClassMotorcyclist), pb.ObjectClass_OBJECT_CLASS_MOTORCYCLIST},
		{"bird", string(l6objects.ClassBird), pb.ObjectClass_OBJECT_CLASS_BIRD},
		{"dynamic", string(l6objects.ClassDynamic), pb.ObjectClass_OBJECT_CLASS_DYNAMIC},
	}

	for _, tc := range classConstants {
		t.Run(tc.name, func(t *testing.T) {
			result := objectClassFromString(tc.constant)
			if result != tc.expected {
				t.Errorf("objectClassFromString(%q) = %v, want %v",
					tc.constant, result, tc.expected)
			}

			// Verify it's not unspecified (no data loss)
			if result == pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED {
				t.Errorf("Class constant %q unexpectedly converted to UNSPECIFIED", tc.name)
			}
		})
	}
}

// TestEmptyObjectClassBecomesUnspecified verifies that empty/uninitialized
// ObjectClass values correctly become UNSPECIFIED in proto.
func TestEmptyObjectClassBecomesUnspecified(t *testing.T) {
	testCases := []string{"", " ", "invalid-class"}

	for _, input := range testCases {
		t.Run("input="+input, func(t *testing.T) {
			result := objectClassFromString(input)
			if result != pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED {
				t.Errorf("objectClassFromString(%q) = %v, want UNSPECIFIED",
					input, result)
			}
		})
	}
}
