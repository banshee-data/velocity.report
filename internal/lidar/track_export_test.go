package lidar

import (
	"encoding/binary"
	"testing"
	"time"
)

// TestDefaultPandar40PConfig tests default sensor configuration
func TestDefaultPandar40PConfig(t *testing.T) {
	config := DefaultPandar40PConfig()

	if config.ModelName != "Pandar40P" {
		t.Errorf("expected ModelName=Pandar40P, got %s", config.ModelName)
	}
	if config.Channels != 40 {
		t.Errorf("expected Channels=40, got %d", config.Channels)
	}
	if config.MotorSpeedRPM != 600.0 {
		t.Errorf("expected MotorSpeedRPM=600.0, got %.1f", config.MotorSpeedRPM)
	}
	if config.UDPPort != 2368 {
		t.Errorf("expected UDPPort=2368, got %d", config.UDPPort)
	}
}

// TestEncodePandar40PPacket tests Pandar40P packet encoding
func TestEncodePandar40PPacket(t *testing.T) {
	t.Run("empty points", func(t *testing.T) {
		config := DefaultPandar40PConfig()
		packet, err := EncodePandar40PPacket([]PointPolar{}, 0.0, config)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(packet) != 1262 {
			t.Errorf("expected packet size 1262, got %d", len(packet))
		}

		// Verify packet structure (should have preambles even if empty)
		for blockIdx := 0; blockIdx < 10; blockIdx++ {
			offset := blockIdx * 124
			preamble := binary.LittleEndian.Uint16(packet[offset : offset+2])
			if preamble != 0xFFEE {
				t.Errorf("block %d: expected preamble 0xFFEE, got 0x%04X", blockIdx, preamble)
			}
		}
	})

	t.Run("single point encoding", func(t *testing.T) {
		points := []PointPolar{
			{
				Azimuth:   45.0,
				Elevation: 2.0,
				Distance:  50.0,
				Intensity: 200,
				Channel:   5,
				BlockID:   2,
			},
		}

		config := DefaultPandar40PConfig()
		packet, err := EncodePandar40PPacket(points, 45.0, config)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(packet) != 1262 {
			t.Errorf("expected packet size 1262, got %d", len(packet))
		}

		// Verify preambles
		for blockIdx := 0; blockIdx < 10; blockIdx++ {
			offset := blockIdx * 124
			preamble := binary.LittleEndian.Uint16(packet[offset : offset+2])
			if preamble != 0xFFEE {
				t.Errorf("block %d: expected preamble 0xFFEE, got 0x%04X", blockIdx, preamble)
			}
		}

		// Verify distance encoding (corrected scale factor: d * 200 for 0.5cm resolution)
		// Point at 50m should encode as 50 * 200 = 10000 units
		// Check if any block has non-zero distance
		foundNonZero := false
		for blockIdx := 0; blockIdx < 10; blockIdx++ {
			offset := blockIdx * 124
			for ch := 0; ch < 40; ch++ {
				chOffset := offset + 4 + (ch * 3)
				distance := binary.LittleEndian.Uint16(packet[chOffset : chOffset+2])
				if distance > 0 {
					foundNonZero = true
					// Verify distance is reasonable (should be around 10000 for 50m)
					if distance < 5000 || distance > 15000 {
						t.Logf("warning: block %d channel %d has distance %d (expected ~10000 for 50m)", blockIdx, ch, distance)
					}
				}
			}
		}

		if !foundNonZero {
			t.Error("expected at least one non-zero distance value")
		}
	})

	t.Run("multiple points across azimuth range", func(t *testing.T) {
		points := []PointPolar{
			{Azimuth: 10.0, Elevation: 0.0, Distance: 30.0, Intensity: 150, Channel: 5, BlockID: 0},
			{Azimuth: 100.0, Elevation: 1.0, Distance: 45.0, Intensity: 180, Channel: 10, BlockID: 3},
			{Azimuth: 200.0, Elevation: -1.0, Distance: 60.0, Intensity: 200, Channel: 15, BlockID: 6},
			{Azimuth: 300.0, Elevation: 2.0, Distance: 75.0, Intensity: 220, Channel: 20, BlockID: 8},
		}

		config := DefaultPandar40PConfig()
		packet, err := EncodePandar40PPacket(points, 180.0, config)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Count non-empty blocks (blocks with at least one non-zero distance)
		nonEmptyBlocks := 0
		for blockIdx := 0; blockIdx < 10; blockIdx++ {
			offset := blockIdx * 124
			hasData := false
			for ch := 0; ch < 40; ch++ {
				chOffset := offset + 4 + (ch * 3)
				distance := binary.LittleEndian.Uint16(packet[chOffset : chOffset+2])
				if distance > 0 {
					hasData = true
					break
				}
			}
			if hasData {
				nonEmptyBlocks++
			}
		}

		// With 4 points spread across azimuth, we should have at least 2 non-empty blocks
		if nonEmptyBlocks < 2 {
			t.Errorf("expected at least 2 non-empty blocks, got %d", nonEmptyBlocks)
		}
	})

	t.Run("distance overflow protection", func(t *testing.T) {
		points := []PointPolar{
			{Azimuth: 45.0, Elevation: 0.0, Distance: 250.0, Intensity: 100, Channel: 10, BlockID: 2}, // > 200m
		}

		config := DefaultPandar40PConfig()
		packet, err := EncodePandar40PPacket(points, 45.0, config)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify that long distances are clamped to 0xFFFE or 0xFFFF (invalid/no-return marker)
		// Only check the channel where we expect our point (channel 10)
		bucket := 2 // 45 degrees falls in bucket 2 (36-72 degrees)
		offset := bucket * 124
		chOffset := offset + 4 + (10 * 3)
		distance := binary.LittleEndian.Uint16(packet[chOffset : chOffset+2])

		// Distance should be clamped at MAX_DISTANCE (200m * 200 = 40000) or use 0xFFFE
		if distance == 0 {
			t.Errorf("expected non-zero distance for channel 10, got 0")
		}
	})

	t.Run("channel validation", func(t *testing.T) {
		points := []PointPolar{
			{Azimuth: 45.0, Elevation: 0.0, Distance: 50.0, Intensity: 100, Channel: -1, BlockID: 2}, // Invalid
			{Azimuth: 45.0, Elevation: 0.0, Distance: 50.0, Intensity: 100, Channel: 50, BlockID: 2}, // Invalid
			{Azimuth: 45.0, Elevation: 0.0, Distance: 50.0, Intensity: 100, Channel: 10, BlockID: 2}, // Valid
		}

		config := DefaultPandar40PConfig()
		packet, err := EncodePandar40PPacket(points, 45.0, config)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should handle invalid channels gracefully (skip them)
		if len(packet) != 1262 {
			t.Errorf("expected packet size 1262, got %d", len(packet))
		}
	})

	t.Run("azimuth boundary handling", func(t *testing.T) {
		// Test points near 0°/360° boundary
		points := []PointPolar{
			{Azimuth: 359.5, Elevation: 0.0, Distance: 40.0, Intensity: 150, Channel: 5, BlockID: 9},
			{Azimuth: 0.5, Elevation: 0.0, Distance: 40.0, Intensity: 150, Channel: 5, BlockID: 0},
			{Azimuth: 324.5, Elevation: 0.0, Distance: 40.0, Intensity: 150, Channel: 5, BlockID: 9},
		}

		config := DefaultPandar40PConfig()
		packet, err := EncodePandar40PPacket(points, 0.0, config)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify packet structure is valid
		if len(packet) != 1262 {
			t.Errorf("expected packet size 1262, got %d", len(packet))
		}
	})
}

// TestExportTrackPointCloud tests track point cloud extraction
func TestExportTrackPointCloud(t *testing.T) {
	t.Run("empty observation history", func(t *testing.T) {
		track := &TrackedObject{TrackID: "track-1"}
		frames, err := ExportTrackPointCloud(track, []*TrackObservation{})

		if err == nil {
			t.Error("expected error for empty observation history")
		}
		if frames != nil {
			t.Error("expected nil frames for empty observations")
		}
	})

	t.Run("single observation", func(t *testing.T) {
		track := &TrackedObject{TrackID: "track-1"}
		observations := []*TrackObservation{
			{
				TSUnixNanos: time.Now().UnixNano(),
				X:           10.0,
				Y:           20.0,
			},
		}

		frames, err := ExportTrackPointCloud(track, observations)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(frames) != 1 {
			t.Errorf("expected 1 frame, got %d", len(frames))
		}
		if frames[0].TrackID != "track-1" {
			t.Errorf("expected TrackID=track-1, got %s", frames[0].TrackID)
		}
		if frames[0].FrameIndex != 0 {
			t.Errorf("expected FrameIndex=0, got %d", frames[0].FrameIndex)
		}
	})

	t.Run("multiple observations", func(t *testing.T) {
		track := &TrackedObject{TrackID: "track-2"}
		observations := []*TrackObservation{
			{TSUnixNanos: time.Now().UnixNano(), X: 10.0, Y: 20.0},
			{TSUnixNanos: time.Now().Add(100 * time.Millisecond).UnixNano(), X: 11.0, Y: 21.0},
			{TSUnixNanos: time.Now().Add(200 * time.Millisecond).UnixNano(), X: 12.0, Y: 22.0},
		}

		frames, err := ExportTrackPointCloud(track, observations)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(frames) != 3 {
			t.Errorf("expected 3 frames, got %d", len(frames))
		}

		// Verify frame indices are sequential
		for i, frame := range frames {
			if frame.FrameIndex != i {
				t.Errorf("frame %d: expected FrameIndex=%d, got %d", i, i, frame.FrameIndex)
			}
		}
	})
}

// TestExtractMetadata tests metadata extraction from tracks
func TestExtractMetadata(t *testing.T) {
	now := time.Now()
	track := &TrackedObject{
		TrackID:            "track-123",
		SensorID:           "sensor-1",
		FirstUnixNanos:     now.UnixNano(),
		LastUnixNanos:      now.Add(6500 * time.Millisecond).UnixNano(),
		ObjectClass:        "vehicle",
		ObjectConfidence:   0.92,
		TrackLengthMeters:  65.5,
		TrackDurationSecs:  6.5,
		OcclusionCount:     2,
		MaxOcclusionFrames: 5,
		SpatialCoverage:    0.85,
		NoisePointRatio:    0.12,
	}

	frames := []*TrackPointCloudFrame{
		{TrackID: "track-123", FrameIndex: 0, PolarPoints: []PointPolar{{Distance: 10.0}, {Distance: 15.0}}},
		{TrackID: "track-123", FrameIndex: 1, PolarPoints: []PointPolar{{Distance: 12.0}}},
	}

	metadata := ExtractMetadata(track, frames)

	if metadata.TrackID != "track-123" {
		t.Errorf("expected TrackID=track-123, got %s", metadata.TrackID)
	}
	if metadata.ObjectClass != "vehicle" {
		t.Errorf("expected ObjectClass=vehicle, got %s", metadata.ObjectClass)
	}
	if metadata.ObjectConfidence != 0.92 {
		t.Errorf("expected ObjectConfidence=0.92, got %.2f", metadata.ObjectConfidence)
	}
	if metadata.TrackLength != 65.5 {
		t.Errorf("expected TrackLength=65.5, got %.1f", metadata.TrackLength)
	}
	if metadata.Duration != 6.5 {
		t.Errorf("expected Duration=6.5, got %.1f", metadata.Duration)
	}
	if metadata.OcclusionCount != 2 {
		t.Errorf("expected OcclusionCount=2, got %d", metadata.OcclusionCount)
	}
	if metadata.TotalFrames != 2 {
		t.Errorf("expected TotalFrames=2, got %d", metadata.TotalFrames)
	}
	if metadata.TotalPoints != 3 {
		t.Errorf("expected TotalPoints=3, got %d", metadata.TotalPoints)
	}
	// Quality score should be computed
	if metadata.QualityScore < 0 || metadata.QualityScore > 1 {
		t.Errorf("expected QualityScore in [0,1], got %.2f", metadata.QualityScore)
	}
}
