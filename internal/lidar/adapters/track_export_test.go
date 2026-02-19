package adapters

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func TestDefaultPandar40PConfig(t *testing.T) {
	config := DefaultPandar40PConfig()

	if config == nil {
		t.Fatal("DefaultPandar40PConfig returned nil")
	}

	if config.ModelName != "Pandar40P" {
		t.Errorf("Expected ModelName 'Pandar40P', got %s", config.ModelName)
	}

	if config.Channels != 40 {
		t.Errorf("Expected 40 channels, got %d", config.Channels)
	}

	if config.MotorSpeedRPM != 600.0 {
		t.Errorf("Expected MotorSpeedRPM 600.0, got %f", config.MotorSpeedRPM)
	}

	if config.UDPPort != 2368 {
		t.Errorf("Expected UDPPort 2368, got %d", config.UDPPort)
	}
}

func TestExportTrackPointCloud(t *testing.T) {
	track := &l5tracks.TrackedObject{
		TrackID:  "track-001",
		SensorID: "test-sensor",
	}

	observations := []*sqlite.TrackObservation{
		{
			TSUnixNanos: time.Now().UnixNano(),
			X:           10.0,
			Y:           20.0,
			Z:           1.5,
			SpeedMps:    15.0,
		},
	}

	frames, err := ExportTrackPointCloud(track, observations)
	if err != nil {
		t.Fatalf("ExportTrackPointCloud failed: %v", err)
	}

	if len(frames) != 1 {
		t.Errorf("Expected 1 frame, got %d", len(frames))
	}

	if frames[0].TrackID != track.TrackID {
		t.Errorf("Expected TrackID %s, got %s", track.TrackID, frames[0].TrackID)
	}
}

func TestExportTrackPointCloudNoObservations(t *testing.T) {
	track := &l5tracks.TrackedObject{
		TrackID:  "track-001",
		SensorID: "test-sensor",
	}

	_, err := ExportTrackPointCloud(track, []*sqlite.TrackObservation{})
	if err == nil {
		t.Error("Expected error for empty observations")
	}
}

func TestEncodePandar40PPacketBasic(t *testing.T) {
	config := DefaultPandar40PConfig()

	points := []l2frames.PointPolar{
		{Azimuth: 0.0, Elevation: 0.0, Distance: 10.0, Intensity: 100, Channel: 1},
	}

	packet, err := EncodePandar40PPacket(points, 0.0, config)
	if err != nil {
		t.Fatalf("EncodePandar40PPacket failed: %v", err)
	}

	expectedSize := 1262
	if len(packet) != expectedSize {
		t.Errorf("Expected packet size %d, got %d", expectedSize, len(packet))
	}

	// Verify block preambles (0xFFEE)
	for blockIdx := 0; blockIdx < 10; blockIdx++ {
		offset := blockIdx * 124
		preamble := binary.LittleEndian.Uint16(packet[offset:])
		if preamble != 0xFFEE {
			t.Errorf("Block %d: Expected preamble 0xFFEE, got 0x%04X", blockIdx, preamble)
		}
	}
}

func TestEncodePandar40PPacketEmptyPoints(t *testing.T) {
	config := DefaultPandar40PConfig()

	packet, err := EncodePandar40PPacket([]l2frames.PointPolar{}, 0.0, config)
	if err != nil {
		t.Fatalf("EncodePandar40PPacket failed for empty points: %v", err)
	}

	if len(packet) != 1262 {
		t.Errorf("Expected packet size 1262 even for empty points, got %d", len(packet))
	}
}

func TestEncodePandar40PPacketDistanceEncoding(t *testing.T) {
	config := DefaultPandar40PConfig()

	tests := []struct {
		name     string
		distance float64
		expected uint16
	}{
		{"10 meters", 10.0, 2000},
		{"50 meters", 50.0, 10000},
		{"negative distance", -5.0, 0xFFFF},
		{"zero distance", 0.0, 0xFFFF},
		{"over max distance", 250.0, 0xFFFE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := []l2frames.PointPolar{
				{Azimuth: 0.0, Elevation: 0.0, Distance: tt.distance, Intensity: 100, Channel: 1},
			}

			packet, err := EncodePandar40PPacket(points, 0.0, config)
			if err != nil {
				t.Fatalf("EncodePandar40PPacket failed: %v", err)
			}

			blockOffset := 0
			channelOffset := blockOffset + 4
			idx := channelOffset + (0 * 3)
			distance := binary.LittleEndian.Uint16(packet[idx:])

			if distance != tt.expected {
				t.Errorf("Expected encoded distance 0x%04X, got 0x%04X",
					tt.expected, distance)
			}
		})
	}
}

func TestWritePCAPFile(t *testing.T) {
	config := DefaultPandar40PConfig()
	packets := [][]byte{
		make([]byte, 1262),
	}

	err := WritePCAPFile(packets, "/tmp/test.pcap", config)
	if err == nil {
		t.Error("Expected WritePCAPFile to return error (not implemented)")
	}
}

func TestWriteNetworkStream(t *testing.T) {
	config := DefaultPandar40PConfig()
	packets := [][]byte{
		make([]byte, 1262),
	}

	err := WriteNetworkStream(packets, "127.0.0.1:2368", config)
	if err == nil {
		t.Error("Expected WriteNetworkStream to return error (not implemented)")
	}
}

func TestExtractMetadata(t *testing.T) {
	now := time.Now()
	track := &l5tracks.TrackedObject{
		TrackID:           "track-001",
		SensorID:          "test-sensor",
		FirstUnixNanos:    now.UnixNano(),
		LastUnixNanos:     now.Add(5 * time.Second).UnixNano(),
		TrackLengthMeters: 50.0,
		TrackDurationSecs: 5.0,
		OcclusionCount:    2,
		ObjectClass:       "vehicle",
		ObjectConfidence:  0.85,
	}

	frames := []*TrackPointCloudFrame{
		{
			TrackID:    "track-001",
			FrameIndex: 0,
			Timestamp:  now,
			PolarPoints: []l2frames.PointPolar{
				{Azimuth: 0.0, Distance: 10.0, Channel: 1},
			},
		},
	}

	metadata := ExtractMetadata(track, frames)

	if metadata.TrackID != track.TrackID {
		t.Errorf("Expected TrackID %s, got %s", track.TrackID, metadata.TrackID)
	}

	if metadata.TotalFrames != 1 {
		t.Errorf("Expected TotalFrames 1, got %d", metadata.TotalFrames)
	}

	if metadata.TotalPoints != 1 {
		t.Errorf("Expected TotalPoints 1, got %d", metadata.TotalPoints)
	}
}
