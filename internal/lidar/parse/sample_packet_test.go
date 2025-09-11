package parse

import (
	"testing"
)

// TestSamplePacketTailParsing tests our parser against the actual sample packet data
func TestSamplePacketTailParsing(t *testing.T) {
	// Sample packet 1 tail bytes (last 30 bytes from the actual packet)
	// Extracted from the hexdump: 02 2f ae 01 89 33 2e 09 79 77 00 0d 0e 00 38 42 11 09 06 0e 21 26 02 f7 00 00 8d 23 04 00
	tailData := []byte{
		0x02, 0x2f, 0xae, 0x01, 0x89, // Reserved1 (bytes 0-4)
		0x33,                         // HighTempFlag (byte 5)
		0x2e, 0x09,                   // Reserved2 (bytes 6-7)
		0x79, 0x77,                   // MotorSpeed (bytes 8-9) - little endian = 0x7779 = 30585 RPM
		0x00, 0x0d, 0x0e, 0x00,       // Timestamp (bytes 10-13) - little endian = 0x000e0d00 = 921856 μs
		0x38,                         // ReturnMode (byte 14)
		0x42,                         // FactoryInfo (byte 15)
		0x11, 0x09, 0x06, 0x0e, 0x21, 0x26, // DateTime (bytes 16-21)
		0x02, 0xf7, 0x00, 0x00,       // UDPSequence (bytes 22-25) - little endian = 0x0000f702 = 63234
		0x8d, 0x23, 0x04, 0x00,       // FCS (bytes 26-29)
	}
	
	// Create parser and parse tail
	config, err := LoadPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	parser := NewPandar40PParser(*config)

	tail, err := parser.parseTail(tailData)
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
	t.Logf("  FCS: %02x", tail.FCS)

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