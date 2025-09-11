package parse

import (
	"encoding/binary"
	"testing"
)

func TestLoadPandar40PConfig(t *testing.T) {
	// Test loading embedded configuration
	config, err := LoadPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load embedded config: %v", err)
	}

	// Validate configuration
	err = config.Validate()
	if err != nil {
		t.Fatalf("Configuration validation failed: %v", err)
	}

	// Test that we have all channels
	if len(config.AngleCorrections) != CHANNELS_PER_BLOCK {
		t.Errorf("Expected %d angle corrections, got %d", CHANNELS_PER_BLOCK, len(config.AngleCorrections))
	}

	if len(config.FiretimeCorrections) != CHANNELS_PER_BLOCK {
		t.Errorf("Expected %d firetime corrections, got %d", CHANNELS_PER_BLOCK, len(config.FiretimeCorrections))
	}

	// Test that channels are properly numbered (1-40)
	for i := 0; i < CHANNELS_PER_BLOCK; i++ {
		if config.AngleCorrections[i].Channel != i+1 {
			t.Errorf("Angle correction channel mismatch at index %d: expected %d, got %d",
				i, i+1, config.AngleCorrections[i].Channel)
		}
		if config.FiretimeCorrections[i].Channel != i+1 {
			t.Errorf("Firetime correction channel mismatch at index %d: expected %d, got %d",
				i, i+1, config.FiretimeCorrections[i].Channel)
		}
	}

	t.Logf("Successfully loaded embedded configuration for %d channels", CHANNELS_PER_BLOCK)
}

func TestPacketParsing(t *testing.T) {
	// Create a mock Pandar40P packet for testing
	packet := createMockPacket()

	// Create a basic config for testing
	config := createMockConfig()

	parser := NewPandar40PParser(*config)

	points, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	t.Logf("Parsed %d points from packet", len(points))

	// Verify we have some points
	if len(points) == 0 {
		t.Error("Expected some points, got none")
	}

	// Test first point properties
	if len(points) > 0 {
		point := points[0]

		if point.Channel < 1 || point.Channel > CHANNELS_PER_BLOCK {
			t.Errorf("Invalid channel number: %d", point.Channel)
		}

		if point.Distance < 0 {
			t.Errorf("Invalid distance: %f", point.Distance)
		}

		if point.Azimuth < 0 || point.Azimuth >= 360 {
			t.Errorf("Invalid azimuth: %f", point.Azimuth)
		}

		t.Logf("First point: X=%.3f, Y=%.3f, Z=%.3f, Distance=%.3f, Azimuth=%.1f, Channel=%d",
			point.X, point.Y, point.Z, point.Distance, point.Azimuth, point.Channel)
	}
}

func createMockPacket() []byte {
	packet := make([]byte, PACKET_SIZE_STANDARD)

	// Data blocks start immediately at offset 0 (no header)
	offset := 0
	for block := 0; block < BLOCKS_PER_PACKET; block++ {
		// Block preamble (0xFFEE) - appears as 0xEEFF in little-endian
		binary.LittleEndian.PutUint16(packet[offset:offset+2], 0xEEFF)
		offset += 2

		// Block azimuth
		binary.LittleEndian.PutUint16(packet[offset:offset+2], uint16(block*1000)) // Azimuth in 0.01-degree units
		offset += 2

		// Channel data (40 channels per block)
		for channel := 0; channel < CHANNELS_PER_BLOCK; channel++ {
			// Distance (simulate 10 meter measurement)
			distance := uint16(10000 / 4) // 10m in 4mm units
			binary.LittleEndian.PutUint16(packet[offset:offset+2], distance)
			offset += 2

			// Reflectivity
			packet[offset] = 100 // Simulate 100 intensity
			offset += 1
		}
	}

	// Tail (30 bytes) - fill with realistic values based on official documentation
	tailOffset := PACKET_SIZE_STANDARD - TAIL_SIZE

	// Reserved fields (bytes 0-4)
	copy(packet[tailOffset:tailOffset+5], []byte{0x02, 0x2f, 0xae, 0x01, 0x89})

	packet[tailOffset+5] = 0x00 // HighTempFlag (byte 5)

	// Reserved field (bytes 6-7)
	copy(packet[tailOffset+6:tailOffset+8], []byte{0x33, 0x2e})

	// Motor speed (bytes 8-9)
	binary.LittleEndian.PutUint16(packet[tailOffset+8:tailOffset+10], 1200) // MotorSpeed

	// Timestamp (bytes 10-13)
	binary.LittleEndian.PutUint32(packet[tailOffset+10:tailOffset+14], 1000000) // Timestamp (1 second)

	packet[tailOffset+14] = 0x38 // ReturnMode
	packet[tailOffset+15] = 0x42 // FactoryInfo

	// Date & Time (bytes 16-21)
	copy(packet[tailOffset+16:tailOffset+22], []byte{0x11, 0x09, 0x06, 0x0e, 0x21, 0x26})

	// UDP sequence and FCS (bytes 22-29)
	binary.LittleEndian.PutUint32(packet[tailOffset+22:tailOffset+26], 4131)  // UDPSequence
	copy(packet[tailOffset+26:tailOffset+30], []byte{0x8b, 0x00, 0x00, 0x00}) // FCS

	return packet
}

func createMockConfig() *Pandar40PConfig {
	config := &Pandar40PConfig{}

	// Create mock angle corrections (simplified)
	for i := 0; i < CHANNELS_PER_BLOCK; i++ {
		config.AngleCorrections[i] = AngleCorrection{
			Channel:   i + 1,
			Elevation: float64(i-20) * 0.5, // Simple elevation progression
			Azimuth:   -1.042,              // Common azimuth offset
		}

		config.FiretimeCorrections[i] = FiretimeCorrection{
			Channel:  i + 1,
			FireTime: float64(i) * -1.0, // Simple time progression
		}
	}

	return config
}

func BenchmarkPacketParsing(b *testing.B) {
	packet := createMockPacket()
	config := createMockConfig()
	parser := NewPandar40PParser(*config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.ParsePacket(packet)
		if err != nil {
			b.Fatalf("Failed to parse packet: %v", err)
		}
	}
}
