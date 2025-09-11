package parse

import (
	"encoding/binary"
	"testing"
)

// TestPacketTailParsing tests the new PacketTail structure with realistic data
func TestPacketTailParsing(t *testing.T) {
	// Create a tail with data patterns from real sample packets (30-byte structure)
	tailData := make([]byte, 30)

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

	// UDP sequence (bytes 22-25)
	binary.LittleEndian.PutUint32(tailData[22:26], 63234) // UDPSequence

	// FCS (bytes 26-29)
	copy(tailData[26:30], []byte{0x8b, 0x00, 0x00, 0x00}) // FCS

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

	if tail.FCS[0] != 0x8b {
		t.Errorf("Expected FCS[0] 0x8b, got 0x%02x", tail.FCS[0])
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

	t.Logf("Successfully parsed tail: HighTemp=0x%02x, MotorSpeed=%d, UDPSeq=%d, ReturnMode=0x%02x, Factory=0x%02x, Timestamp=%d, FCS=0x%02x",
		tail.HighTempFlag, tail.MotorSpeed, tail.UDPSequence, tail.ReturnMode, tail.FactoryInfo, tail.Timestamp, tail.FCS[0])
}

// TestPacketTailSequenceAnalysis tests the sequence pattern observed in real packets
func TestPacketTailSequenceAnalysis(t *testing.T) {
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
		tailData := make([]byte, 30)

		// Set relevant fields for 30-byte structure
		binary.LittleEndian.PutUint32(tailData[10:14], seq.timestamp) // Timestamp
		tailData[14] = 0x38                                           // ReturnMode
		tailData[15] = 0x42                                           // FactoryInfo
		binary.LittleEndian.PutUint32(tailData[22:26], seq.sequence)  // UDPSequence
		copy(tailData[26:30], []byte{0x8b, 0x00, 0x00, 0x00})         // FCS

		tail, err := parser.parseTail(tailData)
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
