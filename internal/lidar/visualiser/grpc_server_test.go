// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

import (
	"context"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
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
