package parse

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Test constants - defined locally to avoid dependency on implementation details
// This makes tests more maintainable and independent of internal constant changes
const (
	testChannelsPerBlock   = 40   // Number of laser channels per data block (Pandar40P)
	testPacketSizeStandard = 1262 // Standard UDP packet size in bytes
	testPacketSizeSequence = 1266 // UDP packet size with 4-byte sequence number
	testBlocksPerPacket    = 10   // Number of data blocks per packet
	testTailSize           = 22   // LiDAR data tail size in bytes
	testBlockSize          = 124  // Size of each data block in bytes
)

// TestPandar40PConfigWrapper tests loading the configuration via the wrapper function
func TestPandar40PConfigWrapper(t *testing.T) {
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
	if len(config.AngleCorrections) != testChannelsPerBlock {
		t.Errorf("Expected %d angle corrections, got %d", testChannelsPerBlock, len(config.AngleCorrections))
	}

	if len(config.FiretimeCorrections) != testChannelsPerBlock {
		t.Errorf("Expected %d firetime corrections, got %d", testChannelsPerBlock, len(config.FiretimeCorrections))
	}

	// Test that channels are properly numbered (1-40)
	for i := 0; i < testChannelsPerBlock; i++ {
		if config.AngleCorrections[i].Channel != i+1 {
			t.Errorf("Angle correction channel mismatch at index %d: expected %d, got %d",
				i, i+1, config.AngleCorrections[i].Channel)
		}
		if config.FiretimeCorrections[i].Channel != i+1 {
			t.Errorf("Firetime correction channel mismatch at index %d: expected %d, got %d",
				i, i+1, config.FiretimeCorrections[i].Channel)
		}
	}

	t.Logf("Successfully loaded embedded configuration for %d channels", testChannelsPerBlock)
}

// TestPacketParsingWithMockData tests basic packet parsing with generated data
func TestPacketParsingWithMockData(t *testing.T) {
	// Create a mock Pandar40P packet for testing
	packet := createTestMockPacket()

	// Create a basic config for testing
	config := createTestMockConfig()

	parser := NewPandar40PParser(*config)

	points, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	t.Logf("Parsed %d points from mock packet", len(points))

	// Verify we have some points
	if len(points) == 0 {
		t.Error("Expected some points, got none")
	}

	// Test first point properties
	if len(points) > 0 {
		p := points[0]

		if p.Channel < 1 || p.Channel > testChannelsPerBlock {
			t.Errorf("Invalid channel number: %d", p.Channel)
		}

		if p.Distance < 0 {
			t.Errorf("Invalid distance: %f", p.Distance)
		}

		if p.Azimuth < 0 || p.Azimuth >= 360 {
			t.Errorf("Invalid azimuth: %f", p.Azimuth)
		}

		x, y, z := lidar.SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)
		t.Logf("First point: X=%.3f, Y=%.3f, Z=%.3f, Distance=%.3f, Azimuth=%.1f, Channel=%d",
			x, y, z, p.Distance, p.Azimuth, p.Channel)
	}
}

// TestTailStructureParsing tests the PacketTail structure with realistic data
func TestTailStructureParsing(t *testing.T) {
	// Create a tail with data patterns from real sample packets (22-byte structure)
	tailData := make([]byte, 22)

	// Reserved fields (bytes 0-4)
	copy(tailData[0:5], []byte{0x02, 0x2f, 0xae, 0x01, 0x89})

	tailData[5] = 0x00 // HighTempFlag (byte 5)

	// Reserved field (bytes 6-7)
	copy(tailData[6:8], []byte{0x33, 0x2e})

	// Motor speed (bytes 8-9)
	binary.LittleEndian.PutUint16(tailData[8:10], 1200) // Motor speed RPM

	// Timestamp (bytes 10-13)
	binary.LittleEndian.PutUint32(tailData[10:14], 271005) // Timestamp microseconds

	tailData[14] = 0x38 // ReturnMode (byte 14)
	tailData[15] = 0x42 // FactoryInfo (byte 15)

	// Date & Time (bytes 16-21)
	copy(tailData[16:22], []byte{0x11, 0x09, 0x06, 0x0e, 0x21, 0x26})

	// Create parser and parse tail
	config, err := LoadPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	parser := NewPandar40PParser(*config)

	tail, err := parser.parseTail(tailData, 63234) // Use the expected UDP sequence from test data
	if err != nil {
		t.Fatalf("Failed to parse tail: %v", err)
	}

	// Verify parsed fields
	if tail.HighTempFlag != 0x00 {
		t.Errorf("Expected HighTempFlag 0x00, got 0x%02x", tail.HighTempFlag)
	}

	if tail.MotorSpeed != 1200 {
		t.Errorf("Expected MotorSpeed 1200, got %d", tail.MotorSpeed)
	}

	if tail.UDPSequence != 63234 {
		t.Errorf("Expected UDPSequence 63234, got %d", tail.UDPSequence)
	}

	if tail.ReturnMode != 0x38 {
		t.Errorf("Expected ReturnMode 0x38, got 0x%02x", tail.ReturnMode)
	}

	if tail.FactoryInfo != 0x42 {
		t.Errorf("Expected FactoryInfo 0x42, got 0x%02x", tail.FactoryInfo)
	}

	if tail.Timestamp != 271005 {
		t.Errorf("Expected Timestamp 271005, got %d", tail.Timestamp)
	}

	// Verify reserved fields
	expectedReserved1 := []byte{0x02, 0x2f, 0xae, 0x01, 0x89}
	for i, expected := range expectedReserved1 {
		if tail.Reserved1[i] != expected {
			t.Errorf("Reserved1[%d]: expected 0x%02x, got 0x%02x", i, expected, tail.Reserved1[i])
		}
	}

	expectedDateTime := []byte{0x11, 0x09, 0x06, 0x0e, 0x21, 0x26}
	for i, expected := range expectedDateTime {
		if tail.DateTime[i] != expected {
			t.Errorf("DateTime[%d]: expected 0x%02x, got 0x%02x", i, expected, tail.DateTime[i])
		}
	}

	t.Logf("Successfully parsed tail: HighTemp=0x%02x, MotorSpeed=%d, UDPSeq=%d, ReturnMode=0x%02x, Factory=0x%02x, Timestamp=%d",
		tail.HighTempFlag, tail.MotorSpeed, tail.UDPSequence, tail.ReturnMode, tail.FactoryInfo, tail.Timestamp)
}

// TestRealSamplePacketTailParsing tests our parser against the actual sample packet data
func TestRealSamplePacketTailParsing(t *testing.T) {
	// Sample packet 1 tail bytes (last 22 bytes from the actual packet tail)
	// Note: UDP sequence is handled separately, not part of the 22-byte tail
	tailData := []byte{
		0x02, 0x2f, 0xae, 0x01, 0x89, // Reserved1 (bytes 0-4)
		0x33,       // HighTempFlag (byte 5)
		0x2e, 0x09, // Reserved2 (bytes 6-7)
		0x79, 0x77, // MotorSpeed (bytes 8-9) - little endian = 0x7779 = 30585 RPM
		0x00, 0x0d, 0x0e, 0x00, // Timestamp (bytes 10-13) - little endian = 0x000e0d00 = 921856 μs
		0x38,                               // ReturnMode (byte 14)
		0x42,                               // FactoryInfo (byte 15)
		0x11, 0x09, 0x06, 0x0e, 0x21, 0x26, // DateTime (bytes 16-21)
	}

	// Create parser and parse tail
	config, err := LoadPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	parser := NewPandar40PParser(*config)

	// UDP sequence from the original sample packet data
	udpSequence := uint32(63234) // 0x0000f702 from bytes 22-25 of original 30-byte structure

	tail, err := parser.parseTail(tailData, udpSequence)
	if err != nil {
		t.Fatalf("Failed to parse tail: %v", err)
	}

	// Log the parsed tail information
	t.Logf("Parsed sample packet 1 tail:")
	t.Logf("  Reserved1: %02x", tail.Reserved1)
	t.Logf("  HighTempFlag: 0x%02x", tail.HighTempFlag)
	t.Logf("  Reserved2: %02x", tail.Reserved2)
	t.Logf("  MotorSpeed: %d RPM", tail.MotorSpeed)
	t.Logf("  Timestamp: %d μs", tail.Timestamp)
	t.Logf("  ReturnMode: 0x%02x", tail.ReturnMode)
	t.Logf("  FactoryInfo: 0x%02x", tail.FactoryInfo)
	t.Logf("  DateTime: %02x", tail.DateTime)
	t.Logf("  UDPSequence: %d", tail.UDPSequence)

	// Basic validation based on expected patterns from documentation
	if tail.ReturnMode != 0x38 && tail.ReturnMode != 0x37 && tail.ReturnMode != 0x39 {
		t.Errorf("Unexpected ReturnMode: 0x%02x (expected 0x37, 0x38, or 0x39)", tail.ReturnMode)
	}

	if tail.FactoryInfo != 0x42 && tail.FactoryInfo != 0x43 {
		t.Errorf("Unexpected FactoryInfo: 0x%02x (expected 0x42 or 0x43)", tail.FactoryInfo)
	}

	// Validate specific expected values from sample packet 1
	if tail.HighTempFlag != 0x33 {
		t.Logf("Note: HighTempFlag is 0x%02x (not 0x00 as expected for normal operation)", tail.HighTempFlag)
	}

	if tail.ReturnMode != 0x38 {
		t.Errorf("Expected ReturnMode 0x38, got 0x%02x", tail.ReturnMode)
	}

	if tail.FactoryInfo != 0x42 {
		t.Errorf("Expected FactoryInfo 0x42, got 0x%02x", tail.FactoryInfo)
	}

	// Validate calculated values
	expectedMotorSpeed := uint16(0x7779) // 30585 RPM
	if tail.MotorSpeed != expectedMotorSpeed {
		t.Errorf("Expected MotorSpeed %d, got %d", expectedMotorSpeed, tail.MotorSpeed)
	}

	expectedTimestamp := uint32(0x000e0d00) // 921856 μs
	if tail.Timestamp != expectedTimestamp {
		t.Errorf("Expected Timestamp %d, got %d", expectedTimestamp, tail.Timestamp)
	}

	expectedUDPSequence := uint32(0x0000f702) // 63234
	if tail.UDPSequence != expectedUDPSequence {
		t.Errorf("Expected UDPSequence %d, got %d", expectedUDPSequence, tail.UDPSequence)
	}

	t.Logf("Sample packet tail parsing validation completed successfully")
}

// TestPacketSequenceAnalysis tests the sequence pattern observed in real packets
func TestPacketSequenceAnalysis(t *testing.T) {
	// Test sequence values from real sample packets
	sequences := []struct {
		name      string
		sequence  uint32
		timestamp uint32
	}{
		{"packet_1", 63234, 271005}, // Anomalous first value
		{"packet_2", 4131, 271006},  // Start of increment pattern
		{"packet_3", 4687, 271007},  // +556
		{"packet_4", 5243, 271008},  // +556
	}

	config, err := LoadPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	parser := NewPandar40PParser(*config)

	var lastSeq uint32
	for i, seq := range sequences {
		tailData := make([]byte, 22)

		// Set relevant fields for 22-byte structure
		binary.LittleEndian.PutUint32(tailData[10:14], seq.timestamp) // Timestamp
		tailData[14] = 0x38                                           // ReturnMode
		tailData[15] = 0x42                                           // FactoryInfo

		tail, err := parser.parseTail(tailData, seq.sequence)
		if err != nil {
			t.Fatalf("Failed to parse tail for %s: %v", seq.name, err)
		}

		if tail.UDPSequence != seq.sequence {
			t.Errorf("%s: expected sequence %d, got %d", seq.name, seq.sequence, tail.UDPSequence)
		}

		if tail.Timestamp != seq.timestamp {
			t.Errorf("%s: expected timestamp %d, got %d", seq.name, seq.timestamp, tail.Timestamp)
		}

		// Check increment pattern (skip first packet as it's anomalous)
		if i > 1 {
			diff := tail.UDPSequence - lastSeq
			expectedDiff := uint32(556)
			if diff != expectedDiff {
				t.Errorf("%s: expected sequence increment %d, got %d", seq.name, expectedDiff, diff)
			}
		}

		lastSeq = tail.UDPSequence

		t.Logf("%s: seq=%d, timestamp=%d", seq.name, tail.UDPSequence, tail.Timestamp)
	}

	t.Log("Sequence pattern analysis completed - 556 increment confirmed for packets 2-4")
}

// TestPcapngPacketExtraction tests reading real packets from the pcapng file
func TestPcapngPacketExtraction(t *testing.T) {
	// Check if pcapng file exists
	pcapPath := filepath.Join(".", "sample_packet.pcapng")
	if _, err := os.Stat(pcapPath); os.IsNotExist(err) {
		t.Skipf("Pcapng file not found at %s, skipping test", pcapPath)
		return
	}

	// For now, we'll read the file as binary and extract known packet positions
	// In a production environment, you'd use a library like gopacket
	data, err := os.ReadFile(pcapPath)
	if err != nil {
		t.Fatalf("Failed to read pcapng file: %v", err)
	}

	t.Logf("Successfully read pcapng file: %d bytes", len(data))

	// Pcapng files have a complex structure, but for testing we can look for
	// the UDP payload patterns we know (0xFFEE preambles)
	packets := extractUDPPayloads(data)

	if len(packets) == 0 {
		t.Skip("No valid UDP packets found in pcapng file")
		return
	}

	t.Logf("Extracted %d potential UDP packets from pcapng", len(packets))

	// Test parsing the first few packets
	config, err := LoadPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	parser := NewPandar40PParser(*config)

	for i, packet := range packets {
		if i >= 5 { // Test first 5 packets only
			break
		}

		// Validate packet size
		if len(packet) != testPacketSizeStandard && len(packet) != testPacketSizeSequence {
			t.Logf("Packet %d: unexpected size %d bytes, skipping", i+1, len(packet))
			continue
		}

		points, err := parser.ParsePacket(packet)
		if err != nil {
			t.Logf("Packet %d: failed to parse: %v", i+1, err)
			continue
		}

		t.Logf("Packet %d: successfully parsed %d points", i+1, len(points))

		// Validate first point if available
		if len(points) > 0 {
			point := points[0]
			t.Logf("  First point: Channel=%d, Azimuth=%.1f°, Distance=%.2fm, Intensity=%d",
				point.Channel, point.Azimuth, point.Distance, point.Intensity)
		}
	}
}

// extractUDPPayloads is a simple function to find potential LiDAR UDP payloads in pcapng data
// This is a simplified approach - in production, use a proper pcapng library
func extractUDPPayloads(data []byte) [][]byte {
	var packets [][]byte

	// Look for patterns that might be LiDAR packets
	// LiDAR packets start with block preambles (0xFFEE in little-endian = 0xEEFF)
	for i := 0; i <= len(data)-testPacketSizeStandard; i++ {
		// Look for the characteristic pattern of multiple 0xEEFF preambles
		// spaced 124 bytes apart (block size)
		if binary.LittleEndian.Uint16(data[i:i+2]) == 0xEEFF {
			// Check if we have more preambles at expected intervals
			validPattern := true
			for block := 1; block < 3; block++ { // Check first 3 blocks
				offset := i + block*testBlockSize
				if offset+2 > len(data) || binary.LittleEndian.Uint16(data[offset:offset+2]) != 0xEEFF {
					validPattern = false
					break
				}
			}

			if validPattern {
				// Extract the full packet - bounds already checked by loop condition
				packet := make([]byte, testPacketSizeStandard)
				copy(packet, data[i:i+testPacketSizeStandard])
				packets = append(packets, packet)

				// Skip ahead to avoid overlapping extractions
				i += testPacketSizeStandard - 1
			}
		}
	}

	return packets
}

// createTestMockPacket creates a mock packet for testing
func createTestMockPacket() []byte {
	packet := make([]byte, testPacketSizeStandard)

	// Data blocks start immediately at offset 0 (no header)
	offset := 0
	for block := 0; block < testBlocksPerPacket; block++ {
		// Block preamble (0xFFEE) - appears as 0xEEFF in little-endian
		binary.LittleEndian.PutUint16(packet[offset:offset+2], 0xEEFF)
		offset += 2

		// Block azimuth
		binary.LittleEndian.PutUint16(packet[offset:offset+2], uint16(block*1000)) // Azimuth in 0.01-degree units
		offset += 2

		// Channel data (40 channels per block)
		for channel := 0; channel < testChannelsPerBlock; channel++ {
			// Distance (simulate 10 meter measurement)
			distance := uint16(10000 / 4) // 10m in 4mm units
			binary.LittleEndian.PutUint16(packet[offset:offset+2], distance)
			offset += 2

			// Reflectivity
			packet[offset] = 100 // Simulate 100 intensity
			offset += 1
		}
	}

	// Tail (22 bytes) - fill with realistic values based on official documentation
	tailOffset := testPacketSizeStandard - testTailSize

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

	return packet
}

// createTestMockConfig creates a mock configuration for testing
func createTestMockConfig() *Pandar40PConfig {
	config := &Pandar40PConfig{}

	// Create mock angle corrections (simplified)
	for i := 0; i < testChannelsPerBlock; i++ {
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

// BenchmarkParserPacketParsing benchmarks packet parsing performance
func BenchmarkParserPacketParsing(b *testing.B) {
	packet := createTestMockPacket()
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.ParsePacket(packet)
		if err != nil {
			b.Fatalf("Failed to parse packet: %v", err)
		}
	}
}

// BenchmarkPcapngExtraction benchmarks pcapng packet extraction
func BenchmarkPcapngExtraction(b *testing.B) {
	pcapPath := filepath.Join(".", "sample_packet.pcapng")
	data, err := os.ReadFile(pcapPath)
	if err != nil {
		b.Skipf("Pcapng file not found, skipping benchmark")
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packets := extractUDPPayloads(data)
		if len(packets) == 0 {
			b.Fatal("No packets extracted")
		}
	}
}
