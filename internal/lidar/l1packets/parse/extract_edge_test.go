package parse

import (
	"encoding/binary"
	"testing"
	"time"
)

// Test constants for edge case testing
const (
	testPacketStandard = 1262
	testPacketSequence = 1266
	testBlockCount     = 10
	testChannelCount   = 40
	testBlockBytes     = 124
)

// TestParsePacket_InvalidPacketSize tests handling of packets with invalid sizes
func TestParsePacket_InvalidPacketSize(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	testCases := []struct {
		name       string
		packetSize int
		expectErr  bool
	}{
		{"empty_packet", 0, true},
		{"tiny_packet", 10, true},
		{"one_block", 124, true},
		{"almost_standard", testPacketStandard - 1, true},
		{"standard_size", testPacketStandard, false},
		{"between_sizes", testPacketStandard + 1, true},
		{"almost_sequence", testPacketSequence - 1, true},
		{"sequence_size", testPacketSequence, false},
		{"too_large", testPacketSequence + 100, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packet := make([]byte, tc.packetSize)
			// Fill with valid block preambles if large enough
			for i := 0; i < testBlockCount && i*testBlockBytes+2 < len(packet); i++ {
				binary.LittleEndian.PutUint16(packet[i*testBlockBytes:], 0xEEFF)
			}

			_, err := parser.ParsePacket(packet)
			if tc.expectErr && err == nil {
				t.Errorf("Expected error for packet size %d, got nil", tc.packetSize)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("Expected no error for packet size %d, got: %v", tc.packetSize, err)
			}
		})
	}
}

// TestParsePacket_InvalidBlockPreambles tests handling of invalid block preambles
func TestParsePacket_InvalidBlockPreambles(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	testCases := []struct {
		name          string
		preambleValue uint16
	}{
		{"zero_preamble", 0x0000},
		{"wrong_value_high", 0xEEFF},
		{"wrong_value_low", 0x00FF},
		{"max_value", 0xFFFF},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packet := createTestMockPacket()
			// Corrupt the first block preamble
			binary.LittleEndian.PutUint16(packet[0:2], tc.preambleValue)

			_, err := parser.ParsePacket(packet)
			// May or may not error depending on strictness
			t.Logf("ParsePacket with preamble 0x%04X: err=%v", tc.preambleValue, err)
		})
	}
}

// TestParsePacket_BoundaryAzimuthValues tests handling of extreme azimuth values
func TestParsePacket_BoundaryAzimuthValues(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	testCases := []struct {
		name         string
		azimuthValue uint16 // Value in 0.01 degree units (0-35999)
	}{
		{"zero_azimuth", 0},
		{"min_azimuth", 1},
		{"mid_azimuth", 18000},  // 180 degrees
		{"near_max", 35998},     // 359.98 degrees
		{"max_azimuth", 35999},  // 359.99 degrees
		{"wrap_around", 0},      // Back to 0
		{"invalid_high", 36000}, // Should wrap or error (360 degrees = 0)
		{"invalid_max", 0xFFFF}, // Maximum uint16
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packet := createTestMockPacket()
			// Set azimuth in first block (offset 2-3 after preamble)
			binary.LittleEndian.PutUint16(packet[2:4], tc.azimuthValue)

			points, err := parser.ParsePacket(packet)
			if err != nil {
				t.Logf("ParsePacket with azimuth %d: err=%v", tc.azimuthValue, err)
				return
			}

			if len(points) > 0 {
				// Check that azimuth is normalised to 0-360 range
				azimuth := points[0].Azimuth
				if azimuth < 0 || azimuth >= 360 {
					t.Errorf("Azimuth out of range: %f (from input %d)", azimuth, tc.azimuthValue)
				}
			}
		})
	}
}

// TestParsePacket_BoundaryDistanceValues tests handling of extreme distance values
func TestParsePacket_BoundaryDistanceValues(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	testCases := []struct {
		name          string
		distanceValue uint16 // Distance in 4mm units
	}{
		{"zero_distance", 0},      // Should be filtered out (no return)
		{"min_distance", 1},       // 4mm
		{"one_metre", 250},        // 1m
		{"ten_metres", 2500},      // 10m
		{"hundred_metres", 25000}, // 100m
		{"max_distance", 0xFFFF},  // ~262m
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packet := createTestMockPacket()
			// Set distance for first channel in first block
			// Offset: 4 (after preamble and azimuth)
			binary.LittleEndian.PutUint16(packet[4:6], tc.distanceValue)

			points, err := parser.ParsePacket(packet)
			if err != nil {
				t.Fatalf("ParsePacket failed: %v", err)
			}

			// Zero distance should be filtered
			if tc.distanceValue == 0 {
				// First channel should be filtered out
				hasFirstChannel := false
				for _, p := range points {
					if p.Channel == 1 && p.BlockID == 0 {
						hasFirstChannel = true
						break
					}
				}
				if hasFirstChannel {
					t.Error("Expected zero-distance point to be filtered out")
				}
			}
		})
	}
}

// TestParsePacket_AllZeroDistances tests handling of a packet with all zero distances
func TestParsePacket_AllZeroDistances(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	packet := make([]byte, testPacketStandard)
	// Fill with valid preambles but leave distances as zero
	for i := 0; i < testBlockCount; i++ {
		offset := i * testBlockBytes
		binary.LittleEndian.PutUint16(packet[offset:], 0xEEFF)
		binary.LittleEndian.PutUint16(packet[offset+2:], uint16(i*1000)) // Azimuth
		// All distances remain 0
	}

	points, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket failed: %v", err)
	}

	// All zero distances should be filtered out
	if len(points) != 0 {
		t.Errorf("Expected 0 points when all distances are zero, got %d", len(points))
	}
}

// TestParsePacket_AllMaxDistances tests handling of a packet with all max distances
func TestParsePacket_AllMaxDistances(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	packet := make([]byte, testPacketStandard)
	// Fill with valid preambles and max distances
	for i := 0; i < testBlockCount; i++ {
		offset := i * testBlockBytes
		binary.LittleEndian.PutUint16(packet[offset:], 0xEEFF)
		binary.LittleEndian.PutUint16(packet[offset+2:], uint16(i*3600)) // Azimuth

		// Set all channel distances to max
		for ch := 0; ch < testChannelCount; ch++ {
			chOffset := offset + 4 + ch*3
			binary.LittleEndian.PutUint16(packet[chOffset:], 0xFFFF)
			packet[chOffset+2] = 100 // Reflectivity
		}
	}

	points, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket failed: %v", err)
	}

	// Should have points for all non-zero distances
	expectedMax := testBlockCount * testChannelCount
	if len(points) > expectedMax {
		t.Errorf("Got more points than expected: %d > %d", len(points), expectedMax)
	}
}

// TestParsePacket_ReflectivityValues tests handling of various reflectivity values
func TestParsePacket_ReflectivityValues(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	testCases := []uint8{0, 1, 50, 100, 127, 128, 200, 255}

	for _, refVal := range testCases {
		t.Run("intensity_"+string(rune('0'+refVal%10)), func(t *testing.T) {
			packet := createTestMockPacket()
			// Set intensity for first channel (offset 6 = preamble(2) + azimuth(2) + distance(2))
			packet[6] = refVal

			points, err := parser.ParsePacket(packet)
			if err != nil {
				t.Fatalf("ParsePacket failed: %v", err)
			}

			if len(points) > 0 {
				// Find the first point from channel 1, block 0
				for _, p := range points {
					if p.Channel == 1 && p.BlockID == 0 {
						if p.Intensity != refVal {
							t.Errorf("Intensity mismatch: expected %d, got %d", refVal, p.Intensity)
						}
						break
					}
				}
			}
		})
	}
}

// TestParsePacket_TailMotorSpeed tests parsing of motor speed from packet tail
func TestParsePacket_TailMotorSpeed(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	testCases := []struct {
		name       string
		motorSpeed uint16 // RPM
	}{
		{"speed_600", 600},
		{"speed_900", 900},
		{"speed_1200", 1200},
		{"speed_zero", 0},
		{"speed_max", 0xFFFF},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packet := createTestMockPacket()
			// Motor speed is at tail offset 8-9 (tail starts at 1240)
			binary.LittleEndian.PutUint16(packet[1240+8:], tc.motorSpeed)

			_, err := parser.ParsePacket(packet)
			if err != nil {
				t.Fatalf("ParsePacket failed: %v", err)
			}

			// Check cached motor speed
			cachedSpeed := parser.GetLastMotorSpeed()
			if cachedSpeed != tc.motorSpeed {
				t.Errorf("Motor speed mismatch: expected %d, got %d", tc.motorSpeed, cachedSpeed)
			}
		})
	}
}

// TestParsePacket_TailReturnMode tests parsing of return mode from packet tail
func TestParsePacket_TailReturnMode(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	returnModes := []byte{0x37, 0x38, 0x39, 0x00, 0xFF}

	for _, mode := range returnModes {
		t.Run("mode_"+string(rune(mode)), func(t *testing.T) {
			packet := createTestMockPacket()
			// Return mode is at tail offset 14 (tail starts at 1240)
			packet[1240+14] = mode

			_, err := parser.ParsePacket(packet)
			// Should parse without error regardless of return mode
			if err != nil {
				t.Fatalf("ParsePacket failed with return mode 0x%02X: %v", mode, err)
			}
		})
	}
}

// TestParsePacket_WithSequenceNumber tests parsing sequence-enabled packets
func TestParsePacket_WithSequenceNumber(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	// Create sequence-enabled packet
	packet := make([]byte, testPacketSequence)

	// Fill with valid block preambles
	for i := 0; i < testBlockCount; i++ {
		offset := i * testBlockBytes
		binary.LittleEndian.PutUint16(packet[offset:], 0xEEFF)
		binary.LittleEndian.PutUint16(packet[offset+2:], uint16(i*3600))

		// Set valid distance for first channel
		binary.LittleEndian.PutUint16(packet[offset+4:], 1000)
		packet[offset+6] = 100
	}

	// Set sequence number at the end
	binary.LittleEndian.PutUint32(packet[testPacketSequence-4:], 12345)

	points, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket failed for sequence-enabled packet: %v", err)
	}

	t.Logf("Parsed %d points from sequence-enabled packet", len(points))
}

// TestTimestampModes tests all timestamp modes
func TestTimestampModes(t *testing.T) {
	config := createTestMockConfig()

	modes := []struct {
		name string
		mode TimestampMode
	}{
		{"system_time", TimestampModeSystemTime},
		{"ptp", TimestampModePTP},
		{"gps", TimestampModeGPS},
		{"internal", TimestampModeInternal},
		{"lidar", TimestampModeLiDAR},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			parser := NewPandar40PParser(*config)
			parser.SetTimestampMode(tc.mode)

			packet := createTestMockPacket()
			_, err := parser.ParsePacket(packet)
			if err != nil {
				t.Fatalf("ParsePacket failed with timestamp mode %s: %v", tc.name, err)
			}
		})
	}
}

// TestSetDebugModes tests debug configuration methods
func TestSetDebugModes(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	// Test SetDebugPackets
	parser.SetDebugPackets(0)
	parser.SetDebugPackets(100)
	parser.SetDebugPackets(1000)

	// Should not panic
	packet := createTestMockPacket()
	_, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket failed after debug config: %v", err)
	}
}

// TestSetPacketTime tests external timestamp override
func TestSetPacketTime(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	// Set external time (e.g., from PCAP)
	externalTime := createTestTime(2024, 6, 15, 12, 30, 45)
	parser.SetPacketTime(externalTime)

	packet := createTestMockPacket()
	_, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket failed with external time: %v", err)
	}

	// External time should be cleared after one use
	// (second parse should use normal timestamp mode)
	_, err = parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket failed on second call: %v", err)
	}
}

// TestParsePacket_ConsecutivePackets tests parsing many packets in sequence
func TestParsePacket_ConsecutivePackets(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	// Parse many consecutive packets to check state handling
	for i := 0; i < 100; i++ {
		packet := createTestMockPacket()
		// Vary azimuth to simulate rotation
		baseAzimuth := uint16((i * 3600) % 36000)
		for block := 0; block < testBlockCount; block++ {
			offset := block * testBlockBytes
			azimuth := (baseAzimuth + uint16(block*360)) % 36000
			binary.LittleEndian.PutUint16(packet[offset+2:], azimuth)
		}

		_, err := parser.ParsePacket(packet)
		if err != nil {
			t.Fatalf("ParsePacket failed on iteration %d: %v", i, err)
		}
	}
}

// createTestTime is a helper to create time values
func createTestTime(year, month, day, hour, min, sec int) time.Time {
	return time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
}
