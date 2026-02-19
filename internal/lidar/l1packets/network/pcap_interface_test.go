package network

import (
	"errors"
	"testing"
	"time"
)

func TestMockPCAPReader_Open(t *testing.T) {
	reader := NewMockPCAPReader(nil)

	err := reader.Open("/path/to/test.pcap")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if reader.OpenedFile != "/path/to/test.pcap" {
		t.Errorf("Expected OpenedFile '/path/to/test.pcap', got '%s'", reader.OpenedFile)
	}
}

func TestMockPCAPReader_Open_Error(t *testing.T) {
	reader := NewMockPCAPReader(nil)
	reader.OpenError = errors.New("file not found")

	err := reader.Open("/nonexistent.pcap")
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "file not found" {
		t.Errorf("Expected 'file not found', got: %v", err)
	}
}

func TestMockPCAPReader_SetBPFFilter(t *testing.T) {
	reader := NewMockPCAPReader(nil)

	err := reader.SetBPFFilter("udp port 2368")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if reader.AppliedFilter != "udp port 2368" {
		t.Errorf("Expected filter 'udp port 2368', got '%s'", reader.AppliedFilter)
	}
}

func TestMockPCAPReader_SetBPFFilter_Error(t *testing.T) {
	reader := NewMockPCAPReader(nil)
	reader.FilterError = errors.New("invalid filter")

	err := reader.SetBPFFilter("invalid")
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "invalid filter" {
		t.Errorf("Expected 'invalid filter', got: %v", err)
	}
}

func TestMockPCAPReader_NextPacket(t *testing.T) {
	now := time.Now()
	packets := []PCAPPacket{
		{Data: []byte("packet1"), Timestamp: now},
		{Data: []byte("packet2"), Timestamp: now.Add(time.Second)},
		{Data: []byte("packet3"), Timestamp: now.Add(2 * time.Second)},
	}
	reader := NewMockPCAPReader(packets)

	// Read first packet
	pkt, err := reader.NextPacket()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if pkt == nil {
		t.Fatal("Expected packet, got nil")
	}
	if string(pkt.Data) != "packet1" {
		t.Errorf("Expected 'packet1', got '%s'", string(pkt.Data))
	}

	// Read second packet
	pkt, err = reader.NextPacket()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if pkt == nil {
		t.Fatal("Expected packet, got nil")
	}
	if string(pkt.Data) != "packet2" {
		t.Errorf("Expected 'packet2', got '%s'", string(pkt.Data))
	}

	// Read third packet
	pkt, err = reader.NextPacket()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if string(pkt.Data) != "packet3" {
		t.Errorf("Expected 'packet3', got '%s'", string(pkt.Data))
	}

	// EOF
	pkt, err = reader.NextPacket()
	if err != nil {
		t.Errorf("Unexpected error on EOF: %v", err)
	}
	if pkt != nil {
		t.Errorf("Expected nil packet at EOF, got: %v", pkt)
	}
}

func TestMockPCAPReader_NextPacket_Empty(t *testing.T) {
	reader := NewMockPCAPReader(nil)

	pkt, err := reader.NextPacket()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if pkt != nil {
		t.Error("Expected nil packet for empty reader")
	}
}

func TestMockPCAPReader_NextPacket_AfterClose(t *testing.T) {
	packets := []PCAPPacket{{Data: []byte("test")}}
	reader := NewMockPCAPReader(packets)

	reader.Close()

	pkt, err := reader.NextPacket()
	if err == nil {
		t.Error("Expected error reading from closed reader")
	}
	if pkt != nil {
		t.Error("Expected nil packet from closed reader")
	}
}

func TestMockPCAPReader_Close(t *testing.T) {
	reader := NewMockPCAPReader(nil)

	reader.Close()

	if !reader.Closed {
		t.Error("Expected reader to be marked as closed")
	}
}

func TestMockPCAPReader_LinkType(t *testing.T) {
	reader := NewMockPCAPReader(nil)

	if reader.LinkType() != 1 {
		t.Errorf("Expected default link type 1 (Ethernet), got %d", reader.LinkType())
	}

	reader.MockLinkType = 113 // Linux cooked capture
	if reader.LinkType() != 113 {
		t.Errorf("Expected link type 113, got %d", reader.LinkType())
	}

	// Test link type > 255 (Linux Cooked Capture v2 = 276)
	reader.MockLinkType = 276
	if reader.LinkType() != 276 {
		t.Errorf("Expected link type 276, got %d", reader.LinkType())
	}
}

func TestMockPCAPReader_Reset(t *testing.T) {
	packets := []PCAPPacket{
		{Data: []byte("packet1")},
		{Data: []byte("packet2")},
	}
	reader := NewMockPCAPReader(packets)

	// Read all packets
	reader.NextPacket()
	reader.NextPacket()
	reader.Open("/test.pcap")
	reader.SetBPFFilter("udp")
	reader.Close()

	// Reset
	reader.Reset()

	// Verify reset state
	if reader.ReadIndex != 0 {
		t.Errorf("Expected ReadIndex 0, got %d", reader.ReadIndex)
	}
	if reader.Closed {
		t.Error("Expected reader not closed after reset")
	}
	if reader.OpenedFile != "" {
		t.Errorf("Expected empty OpenedFile, got '%s'", reader.OpenedFile)
	}
	if reader.AppliedFilter != "" {
		t.Errorf("Expected empty AppliedFilter, got '%s'", reader.AppliedFilter)
	}

	// Should be able to read packets again
	pkt, _ := reader.NextPacket()
	if pkt == nil || string(pkt.Data) != "packet1" {
		t.Error("Expected to read packet1 after reset")
	}
}

func TestMockPCAPReader_AddPacket(t *testing.T) {
	reader := NewMockPCAPReader(nil)
	now := time.Now()

	reader.AddPacket([]byte("dynamic1"), now)
	reader.AddPacket([]byte("dynamic2"), now.Add(time.Second))

	if len(reader.Packets) != 2 {
		t.Fatalf("Expected 2 packets, got %d", len(reader.Packets))
	}

	pkt, _ := reader.NextPacket()
	if string(pkt.Data) != "dynamic1" {
		t.Errorf("Expected 'dynamic1', got '%s'", string(pkt.Data))
	}

	pkt, _ = reader.NextPacket()
	if string(pkt.Data) != "dynamic2" {
		t.Errorf("Expected 'dynamic2', got '%s'", string(pkt.Data))
	}
}

func TestMockPCAPReaderFactory(t *testing.T) {
	mockReader := NewMockPCAPReader(nil)
	factory := NewMockPCAPReaderFactory(mockReader)

	reader := factory.NewReader()
	if reader != mockReader {
		t.Error("Expected factory to return configured reader")
	}
	if factory.CreateCalls != 1 {
		t.Errorf("Expected 1 CreateCalls, got %d", factory.CreateCalls)
	}

	// Call again
	factory.NewReader()
	if factory.CreateCalls != 2 {
		t.Errorf("Expected 2 CreateCalls, got %d", factory.CreateCalls)
	}
}

func TestPCAPPacket_Fields(t *testing.T) {
	now := time.Now()
	pkt := PCAPPacket{
		Data:      []byte("test data"),
		Timestamp: now,
	}

	if string(pkt.Data) != "test data" {
		t.Errorf("Expected Data 'test data', got '%s'", string(pkt.Data))
	}
	if !pkt.Timestamp.Equal(now) {
		t.Errorf("Expected Timestamp %v, got %v", now, pkt.Timestamp)
	}
}
