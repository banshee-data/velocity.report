package lidar

import (
	"encoding/binary"
	"fmt"
	"time"
)

// Phase 2 (Revised): Training Data Preparation
// This module focuses on exporting track point clouds for ML training data preparation.
// Goal: Extract isolated track point clouds in formats that can be inspected in LidarView.

// TrackPointCloudExporter handles exporting point clouds for individual tracks.
// Maintains polar frame representation with minimal transformations.
type TrackPointCloudExporter struct {
	sensorConfig *SensorConfig // Sensor configuration for packet formatting
}

// SensorConfig holds sensor-specific configuration for packet generation.
type SensorConfig struct {
	ModelName     string  // e.g., "Pandar40P"
	Channels      int     // Number of laser channels (40 for Pandar40P)
	MotorSpeedRPM float64 // Motor speed for packet metadata
	UDPPort       int     // Target UDP port (default 2368)
}

// DefaultPandar40PConfig returns default configuration for Pandar40P sensor.
func DefaultPandar40PConfig() *SensorConfig {
	return &SensorConfig{
		ModelName:     "Pandar40P",
		Channels:      40,
		MotorSpeedRPM: 600.0,
		UDPPort:       2368,
	}
}

// TrackPointCloudFrame represents a single frame of point cloud data for a track.
// Points are stored in polar coordinates (sensor frame) for compatibility with parsers.
type TrackPointCloudFrame struct {
	TrackID     string       // Track identifier
	FrameIndex  int          // Frame sequence number within track
	Timestamp   time.Time    // Frame timestamp
	PolarPoints []PointPolar // Points in polar coordinates
}

// ExportTrackPointCloud extracts point clouds for a specific track from observation history.
// Returns frames that can be encoded into PCAP format for LidarView inspection.
func ExportTrackPointCloud(track *TrackedObject, observationHistory []*TrackObservation) ([]*TrackPointCloudFrame, error) {
	if len(observationHistory) == 0 {
		return nil, fmt.Errorf("no observations available for track %s", track.TrackID)
	}

	frames := make([]*TrackPointCloudFrame, 0, len(observationHistory))

	// TODO: For each observation, extract the associated point cloud
	// This requires linking observations to their source foreground points
	// Current implementation returns placeholder - needs integration with point cloud storage

	for i, obs := range observationHistory {
		frame := &TrackPointCloudFrame{
			TrackID:    track.TrackID,
			FrameIndex: i,
			Timestamp:  time.Unix(0, obs.TSUnixNanos),
			// PolarPoints will be populated when point cloud storage is integrated
			PolarPoints: []PointPolar{}, // Placeholder
		}
		frames = append(frames, frame)
	}

	return frames, nil
}

// EncodePandar40PPacket encodes polar points into a Pandar40P-compatible UDP packet format.
// This allows exported tracks to be loaded in LidarView for visual inspection.
//
// Pandar40P packet structure (1262 bytes):
// - 10 data blocks × 124 bytes each
// - Each block: 2-byte preamble (0xFFEE) + 2-byte azimuth + 40 channels × 3 bytes
// - 22-byte tail with timestamp and metadata
func EncodePandar40PPacket(points []PointPolar, blockAzimuth float64, config *SensorConfig) ([]byte, error) {
	const (
		PACKET_SIZE        = 1262
		BLOCKS_PER_PACKET  = 10
		BLOCK_SIZE         = 124
		CHANNELS_PER_BLOCK = 40
		TAIL_SIZE          = 22
	)

	packet := make([]byte, PACKET_SIZE)

	// Encode data blocks (10 blocks)
	for blockIdx := 0; blockIdx < BLOCKS_PER_PACKET; blockIdx++ {
		blockOffset := blockIdx * BLOCK_SIZE

		// Block preamble (0xFFEE)
		binary.LittleEndian.PutUint16(packet[blockOffset:], 0xFFEE)

		// Block azimuth (2 bytes, little-endian, scaled by 100)
		azimuthScaled := uint16(blockAzimuth * 100)
		binary.LittleEndian.PutUint16(packet[blockOffset+2:], azimuthScaled)

		// Channel data (40 channels × 3 bytes)
		channelOffset := blockOffset + 4
		for ch := 0; ch < CHANNELS_PER_BLOCK; ch++ {
			// Find matching point for this channel
			var distance uint16 = 0
			var intensity uint8 = 0

			for _, p := range points {
				if p.Channel == ch && p.BlockID == blockIdx {
					// Distance in 2mm resolution
					distance = uint16(p.Distance * 500)
					intensity = p.Intensity
					break
				}
			}

			// Encode: 2 bytes distance + 1 byte intensity
			idx := channelOffset + (ch * 3)
			binary.LittleEndian.PutUint16(packet[idx:], distance)
			packet[idx+2] = intensity
		}
	}

	// Encode tail (22 bytes at offset 1240)
	tailOffset := BLOCKS_PER_PACKET * BLOCK_SIZE

	// Reserved fields (bytes 0-4, 6-7)
	for i := 0; i < 8; i++ {
		if i != 5 {
			packet[tailOffset+i] = 0
		}
	}

	// HighTempFlag (byte 5)
	packet[tailOffset+5] = 0

	// MotorSpeed (bytes 8-9) - RPM * 60
	motorSpeedEncoded := uint16(config.MotorSpeedRPM * 60)
	binary.LittleEndian.PutUint16(packet[tailOffset+8:], motorSpeedEncoded)

	// Timestamp (bytes 10-13) - microseconds
	now := time.Now()
	timestampMicros := uint32(now.UnixNano() / 1000)
	binary.LittleEndian.PutUint32(packet[tailOffset+10:], timestampMicros)

	// ReturnMode (byte 14) - 0x37 for strongest return
	packet[tailOffset+14] = 0x37

	// FactoryInfo (byte 15)
	packet[tailOffset+15] = 0

	// DateTime (bytes 16-21) - [year-2000, month, day, hour, minute, second]
	packet[tailOffset+16] = uint8(now.Year() - 2000)
	packet[tailOffset+17] = uint8(now.Month())
	packet[tailOffset+18] = uint8(now.Day())
	packet[tailOffset+19] = uint8(now.Hour())
	packet[tailOffset+20] = uint8(now.Minute())
	packet[tailOffset+21] = uint8(now.Second())

	return packet, nil
}

// WritePCAPFile writes a sequence of packets to a PCAP file for LidarView inspection.
// This is a stub - actual implementation requires pcap library integration.
func WritePCAPFile(packets [][]byte, outputPath string, config *SensorConfig) error {
	// TODO: Implement PCAP file writing
	// Requires: gopacket library for PCAP file generation
	// Format: Ethernet + IP + UDP headers + LiDAR payload
	return fmt.Errorf("PCAP file writing not yet implemented - requires pcap build tag")
}

// WriteNetworkStream sends packets to a UDP destination for real-time inspection.
// Allows streaming isolated track data to LidarView without intermediate files.
func WriteNetworkStream(packets [][]byte, destAddr string, config *SensorConfig) error {
	// TODO: Implement UDP network streaming
	// Allows real-time visualization of isolated tracks in LidarView
	return fmt.Errorf("network streaming not yet implemented")
}

// TrackPointCloudMetadata contains metadata for exported track point clouds.
type TrackPointCloudMetadata struct {
	TrackID          string    `json:"track_id"`
	SensorID         string    `json:"sensor_id"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	TotalFrames      int       `json:"total_frames"`
	TotalPoints      int       `json:"total_points"`
	ObjectClass      string    `json:"object_class,omitempty"`
	ObjectConfidence float32   `json:"object_confidence,omitempty"`
	// Phase 1 quality metrics
	TrackLength    float32 `json:"track_length_meters"`
	Duration       float32 `json:"duration_secs"`
	OcclusionCount int     `json:"occlusion_count"`
	QualityScore   float32 `json:"quality_score,omitempty"`
}

// ExtractMetadata generates metadata for an exported track point cloud.
func ExtractMetadata(track *TrackedObject, frames []*TrackPointCloudFrame) *TrackPointCloudMetadata {
	totalPoints := 0
	for _, frame := range frames {
		totalPoints += len(frame.PolarPoints)
	}

	// Compute quality score using Phase 1 metrics
	qualityMetrics := ComputeTrackQualityMetrics(track)

	return &TrackPointCloudMetadata{
		TrackID:          track.TrackID,
		SensorID:         track.SensorID,
		StartTime:        time.Unix(0, track.FirstUnixNanos),
		EndTime:          time.Unix(0, track.LastUnixNanos),
		TotalFrames:      len(frames),
		TotalPoints:      totalPoints,
		ObjectClass:      track.ObjectClass,
		ObjectConfidence: track.ObjectConfidence,
		TrackLength:      track.TrackLengthMeters,
		Duration:         track.TrackDurationSecs,
		OcclusionCount:   track.OcclusionCount,
		QualityScore:     qualityMetrics.QualityScore,
	}
}
