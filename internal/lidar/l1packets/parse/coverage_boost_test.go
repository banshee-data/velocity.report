package parse

import (
	"encoding/binary"
	"testing"
)

// TestParseAngleCorrections_DataRowWrongFieldCount covers the branch in
// parseAngleCorrections where a data row has the wrong number of fields.
func TestParseAngleCorrections_DataRowWrongFieldCount(t *testing.T) {
	config := &Pandar40PConfig{}
	records := [][]string{
		{"Channel", "Elevation", "Azimuth"},
		{"1", "2.0", "3.0"},
		{"2", "2.0"}, // Only 2 fields instead of 3
	}
	err := parseAngleCorrections(records, config)
	if err == nil {
		t.Error("expected error for data row with wrong field count")
	}
}

// TestParseFiretimeCorrections_DataRowWrongFieldCount covers the branch in
// parseFiretimeCorrections where a data row has the wrong number of fields.
func TestParseFiretimeCorrections_DataRowWrongFieldCount(t *testing.T) {
	config := &Pandar40PConfig{}
	records := [][]string{
		{"Channel", "fire time(μs)"},
		{"1", "2.0"},
		{"2", "2.0", "extra"}, // 3 fields instead of 2
	}
	err := parseFiretimeCorrections(records, config)
	if err == nil {
		t.Error("expected error for data row with wrong field count")
	}
}

// TestParseTail_InvalidSize covers the tail-size guard in parseTail.
func TestParseTail_InvalidSize(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	_, err := parser.parseTail(make([]byte, 10), 0)
	if err == nil {
		t.Error("expected error for short tail")
	}
}

// TestParseDataBlock_InsufficientData covers the block-length guard.
func TestParseDataBlock_InsufficientData(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)

	_, err := parser.parseDataBlock(make([]byte, 10))
	if err == nil {
		t.Error("expected error for insufficient block data")
	}
}

// TestParsePacket_DebugLoggingBranches covers debug log paths including
// both the standard and sequence-number packet formats.
func TestParsePacket_DebugLoggingBranches(t *testing.T) {
	config := createTestMockConfig()

	// Standard packet with debug
	parser := NewPandar40PParser(*config)
	parser.SetDebug(true)
	parser.SetDebugPackets(5)

	stdPacket := createTestMockPacket()
	if _, err := parser.ParsePacket(stdPacket); err != nil {
		t.Fatalf("debug+standard parse failed: %v", err)
	}

	// Sequence packet with debug (separate parser to reset packetCount)
	parser2 := NewPandar40PParser(*config)
	parser2.SetDebug(true)
	parser2.SetDebugPackets(5)

	seqPacket := createTestMockPacketWithSequence()
	if _, err := parser2.ParsePacket(seqPacket); err != nil {
		t.Fatalf("debug+sequence parse failed: %v", err)
	}
}

// TestParsePacket_DebugZeroPoints covers the diagnostic log when all distances
// are zero and no points are produced.
func TestParsePacket_DebugZeroPoints(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)
	parser.SetDebug(true)
	parser.SetDebugPackets(5)

	// Build packet with valid preambles but all-zero distances
	packet := make([]byte, testPacketSizeStandard)
	for i := 0; i < testBlocksPerPacket; i++ {
		offset := i * testBlockSize
		binary.LittleEndian.PutUint16(packet[offset:], 0xEEFF)
		binary.LittleEndian.PutUint16(packet[offset+2:], uint16(i*1000))
		// Distance fields remain zero → no points
	}

	points, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket failed: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("expected 0 points, got %d", len(points))
	}
}

// TestResolvePacketTime_PTPStaticFallback covers the static timestamp
// detection branch in PTP mode where consecutive identical timestamps
// cause fallback to system time.
func TestResolvePacketTime_PTPStaticFallback(t *testing.T) {
	config := createTestMockConfig()
	parser := NewPandar40PParser(*config)
	parser.SetTimestampMode(TimestampModePTP)

	packet := createTestMockPacket()

	// Send STATIC_TIMESTAMP_THRESHOLD+3 packets with the same timestamp
	// to trigger the static detection fallback.
	for i := 0; i < STATIC_TIMESTAMP_THRESHOLD+3; i++ {
		_, err := parser.ParsePacket(packet)
		if err != nil {
			t.Fatalf("ParsePacket iteration %d failed: %v", i, err)
		}
	}
}
