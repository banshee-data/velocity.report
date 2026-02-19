package network

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func TestNewForegroundForwarder_Success(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12350, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	if ff.conn == nil {
		t.Error("Expected conn to be set")
	}
	if ff.channel == nil {
		t.Error("Expected channel to be set")
	}
	if ff.sensorConfig == nil {
		t.Error("Expected default sensor config to be set")
	}
	if ff.sensorConfig.MotorSpeedRPM != 600.0 {
		t.Errorf("Expected default motor speed 600, got %f", ff.sensorConfig.MotorSpeedRPM)
	}
	if ff.sensorConfig.Channels != 40 {
		t.Errorf("Expected default channels 40, got %d", ff.sensorConfig.Channels)
	}
	if ff.address != "127.0.0.1:12350" {
		t.Errorf("Expected address '127.0.0.1:12350', got '%s'", ff.address)
	}
	if ff.port != 12350 {
		t.Errorf("Expected port 12350, got %d", ff.port)
	}
}

func TestNewForegroundForwarder_WithCustomConfig(t *testing.T) {
	config := &SensorConfig{
		MotorSpeedRPM: 1200.0,
		Channels:      64,
	}

	ff, err := NewForegroundForwarder("127.0.0.1", 12351, config)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	if ff.sensorConfig.MotorSpeedRPM != 1200.0 {
		t.Errorf("Expected motor speed 1200, got %f", ff.sensorConfig.MotorSpeedRPM)
	}
	if ff.sensorConfig.Channels != 64 {
		t.Errorf("Expected channels 64, got %d", ff.sensorConfig.Channels)
	}
}

func TestForegroundForwarder_Start_ContextCancellation(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12352, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	ctx, cancel := context.WithCancel(context.Background())

	ff.Start(ctx)

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Cancel and verify goroutine stops
	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestForegroundForwarder_Start_ChannelClosed(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12353, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}

	ctx := context.Background()
	ff.Start(ctx)

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Close the forwarder (closes channel)
	ff.Close()
	time.Sleep(20 * time.Millisecond)
}

func TestForegroundForwarder_ForwardForeground_EmptySlice(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12354, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	// Should return early without queueing
	ff.ForwardForeground(nil)
	ff.ForwardForeground([]l4perception.PointPolar{})

	// Channel should be empty
	select {
	case <-ff.channel:
		t.Error("Expected channel to be empty for empty input")
	default:
		// Success - nothing was queued
	}
}

func TestForegroundForwarder_ForwardForeground_QueuePoints(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12355, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	points := []l4perception.PointPolar{
		{Channel: 1, Distance: 10.0, Azimuth: 45.0, Intensity: 100},
		{Channel: 2, Distance: 15.0, Azimuth: 90.0, Intensity: 150},
	}

	ff.ForwardForeground(points)

	// Should have queued the points
	select {
	case received := <-ff.channel:
		if len(received) != 2 {
			t.Errorf("Expected 2 points, got %d", len(received))
		}
		// Verify it's a copy (not same slice)
		if &received[0] == &points[0] {
			t.Error("Expected a copy of points, not the same slice")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected points to be queued")
	}
}

func TestForegroundForwarder_ForwardForeground_BufferFull(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12356, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	// Fill the buffer (capacity is 100)
	points := []l4perception.PointPolar{{Channel: 1, Distance: 10.0}}
	for i := 0; i < 100; i++ {
		ff.ForwardForeground(points)
	}

	// Next one should be dropped (non-blocking)
	done := make(chan bool)
	go func() {
		ff.ForwardForeground(points)
		done <- true
	}()

	select {
	case <-done:
		// Success - non-blocking even when buffer is full
	case <-time.After(100 * time.Millisecond):
		t.Error("ForwardForeground blocked when buffer was full")
	}
}

func TestForegroundForwarder_StartAndForward(t *testing.T) {
	// Create a test UDP server to receive packets
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve server address: %v", err)
	}

	server, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer server.Close()

	serverPort := server.LocalAddr().(*net.UDPAddr).Port

	ff, err := NewForegroundForwarder("127.0.0.1", serverPort, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ff.Start(ctx)

	// Send points
	points := []l4perception.PointPolar{
		{Channel: 1, Distance: 10.0, Azimuth: 45.0, Intensity: 100, BlockID: 0},
	}
	ff.ForwardForeground(points)

	// Read from server
	server.SetReadDeadline(time.Now().Add(1 * time.Second))
	buffer := make([]byte, 2048)
	n, _, err := server.ReadFromUDP(buffer)
	if err != nil {
		t.Fatalf("Failed to read from test server: %v", err)
	}

	// Should receive a packet (1262 or 1266 bytes)
	if n != 1262 && n != 1266 {
		t.Errorf("Expected packet size 1262 or 1266, got %d", n)
	}
}

func TestForegroundForwarder_Close(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12357, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}

	err = ff.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Verify channel is closed
	_, ok := <-ff.channel
	if ok {
		t.Error("Expected channel to be closed")
	}
}

func TestEncodePointsAsPackets_Empty(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12358, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	packets, err := ff.encodePointsAsPackets(nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if packets != nil {
		t.Errorf("Expected nil packets for empty input, got %v", packets)
	}

	packets, err = ff.encodePointsAsPackets([]l4perception.PointPolar{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if packets != nil {
		t.Errorf("Expected nil packets for empty slice, got %v", packets)
	}
}

func TestEncodePointsAsPackets_SinglePacket(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12359, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	points := []l4perception.PointPolar{
		{Channel: 1, Distance: 10.0, Azimuth: 45.0, Intensity: 100, BlockID: 0, Timestamp: now, RawBlockAzimuth: 4500},
		{Channel: 2, Distance: 15.0, Azimuth: 45.0, Intensity: 150, BlockID: 0, Timestamp: now, RawBlockAzimuth: 4500},
		{Channel: 1, Distance: 12.0, Azimuth: 46.0, Intensity: 120, BlockID: 1, Timestamp: now + 1000, RawBlockAzimuth: 4600},
	}

	packets, err := ff.encodePointsAsPackets(points)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packets) != 1 {
		t.Errorf("Expected 1 packet, got %d", len(packets))
	}
	if len(packets[0]) != 1262 {
		t.Errorf("Expected packet size 1262, got %d", len(packets[0]))
	}
}

func TestEncodePointsAsPackets_WithSequence(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12360, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	points := []l4perception.PointPolar{
		{Channel: 1, Distance: 10.0, BlockID: 0, Timestamp: now, UDPSequence: 1},
		{Channel: 2, Distance: 15.0, BlockID: 0, Timestamp: now, UDPSequence: 1},
	}

	packets, err := ff.encodePointsAsPackets(points)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packets) != 1 {
		t.Errorf("Expected 1 packet, got %d", len(packets))
	}
	// Packet with sequence should be 1266 bytes
	if len(packets[0]) != 1266 {
		t.Errorf("Expected packet size 1266 (with sequence), got %d", len(packets[0]))
	}
}

func TestEncodePointsAsPackets_MultiplePackets(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12361, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	points := []l4perception.PointPolar{
		// First packet (sequence 1)
		{Channel: 1, Distance: 10.0, BlockID: 0, Timestamp: now, UDPSequence: 1},
		{Channel: 2, Distance: 15.0, BlockID: 1, Timestamp: now + 1000, UDPSequence: 1},
		// Second packet (sequence 2)
		{Channel: 1, Distance: 20.0, BlockID: 0, Timestamp: now + 200000, UDPSequence: 2},
		{Channel: 2, Distance: 25.0, BlockID: 1, Timestamp: now + 201000, UDPSequence: 2},
	}

	packets, err := ff.encodePointsAsPackets(points)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packets) != 2 {
		t.Errorf("Expected 2 packets, got %d", len(packets))
	}
}

func TestEncodePointsAsPackets_TimestampGap(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12362, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	// Points with large timestamp gap (>200us) should be in separate packets
	points := []l4perception.PointPolar{
		{Channel: 1, Distance: 10.0, BlockID: 0, Timestamp: now},
		{Channel: 1, Distance: 20.0, BlockID: 1, Timestamp: now + 300000}, // 300us gap
	}

	packets, err := ff.encodePointsAsPackets(points)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packets) != 2 {
		t.Errorf("Expected 2 packets due to timestamp gap, got %d", len(packets))
	}
}

func TestEncodePointsAsPackets_BlockIDReset(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12363, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	// BlockID reset (9 -> 0) should trigger new packet
	points := []l4perception.PointPolar{
		{Channel: 1, Distance: 10.0, BlockID: 9, Timestamp: now},
		{Channel: 1, Distance: 20.0, BlockID: 0, Timestamp: now + 1000}, // Reset to 0
	}

	packets, err := ff.encodePointsAsPackets(points)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packets) != 2 {
		t.Errorf("Expected 2 packets due to BlockID reset, got %d", len(packets))
	}
}

func TestBuildPacket_Basic(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12364, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	points := []l4perception.PointPolar{
		{Channel: 1, Distance: 10.0, Intensity: 100, BlockID: 0, Timestamp: now, RawBlockAzimuth: 4500},
	}

	packet, err := ff.buildPacket(points, false, 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packet) != 1262 {
		t.Errorf("Expected packet size 1262, got %d", len(packet))
	}

	// Verify preamble (0xFFEE at offset 0)
	preamble := uint16(packet[0]) | uint16(packet[1])<<8
	if preamble != 0xFFEE {
		t.Errorf("Expected preamble 0xFFEE, got 0x%04X", preamble)
	}
}

func TestBuildPacket_WithSequence(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12365, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	points := []l4perception.PointPolar{
		{Channel: 1, Distance: 10.0, BlockID: 0, Timestamp: now, UDPSequence: 42},
	}

	packet, err := ff.buildPacket(points, true, 42)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packet) != 1266 {
		t.Errorf("Expected packet size 1266 with sequence, got %d", len(packet))
	}
}

func TestBuildPacket_InvalidChannel(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12366, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	// Invalid channels (0, 41) should be skipped
	points := []l4perception.PointPolar{
		{Channel: 0, Distance: 10.0, BlockID: 0, Timestamp: now},
		{Channel: 41, Distance: 10.0, BlockID: 0, Timestamp: now},
		{Channel: 1, Distance: 10.0, BlockID: 0, Timestamp: now},
	}

	packet, err := ff.buildPacket(points, false, 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packet) != 1262 {
		t.Errorf("Expected packet size 1262, got %d", len(packet))
	}
}

func TestBuildPacket_DistanceEdgeCases(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12367, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	testCases := []struct {
		name     string
		distance float64
	}{
		{"zero distance", 0.0},
		{"negative distance", -1.0},
		{"normal distance", 50.0},
		{"max distance", 200.0},
		{"over max distance", 250.0},
	}

	now := time.Now().UnixNano()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			points := []l4perception.PointPolar{
				{Channel: 1, Distance: tc.distance, BlockID: 0, Timestamp: now},
			}

			packet, err := ff.buildPacket(points, false, 0)
			if err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.name, err)
			}
			if len(packet) != 1262 {
				t.Errorf("Expected packet size 1262, got %d", len(packet))
			}
		})
	}
}

func TestBuildPacket_AllBlocks(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12368, nil)
	if err != nil {
		t.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()

	// Create points for all 10 blocks
	var points []l4perception.PointPolar
	for blockID := 0; blockID < 10; blockID++ {
		points = append(points, l4perception.PointPolar{
			Channel:         1,
			Distance:        float64(10 + blockID),
			Intensity:       uint8(100 + blockID),
			BlockID:         blockID,
			Timestamp:       now + int64(blockID*1000),
			RawBlockAzimuth: uint16(blockID * 100),
		})
	}

	packet, err := ff.buildPacket(points, false, 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(packet) != 1262 {
		t.Errorf("Expected packet size 1262, got %d", len(packet))
	}

	// Verify each block has the correct preamble
	for blockIdx := 0; blockIdx < 10; blockIdx++ {
		blockOffset := blockIdx * 124
		preamble := uint16(packet[blockOffset]) | uint16(packet[blockOffset+1])<<8
		if preamble != 0xFFEE {
			t.Errorf("Block %d: Expected preamble 0xFFEE, got 0x%04X", blockIdx, preamble)
		}
	}
}

// Benchmark tests
func BenchmarkEncodePointsAsPackets(b *testing.B) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12369, nil)
	if err != nil {
		b.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	points := make([]l4perception.PointPolar, 400) // Typical foreground frame size
	for i := 0; i < len(points); i++ {
		points[i] = l4perception.PointPolar{
			Channel:   (i % 40) + 1,
			Distance:  float64(10 + i%100),
			Intensity: uint8(100 + i%100),
			BlockID:   i % 10,
			Timestamp: now + int64(i*1000),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ff.encodePointsAsPackets(points)
	}
}

func BenchmarkBuildPacket(b *testing.B) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12370, nil)
	if err != nil {
		b.Fatalf("Failed to create ForegroundForwarder: %v", err)
	}
	defer ff.Close()

	now := time.Now().UnixNano()
	points := make([]l4perception.PointPolar, 40)
	for i := 0; i < len(points); i++ {
		points[i] = l4perception.PointPolar{
			Channel:   i + 1,
			Distance:  float64(10 + i),
			Intensity: uint8(100 + i),
			BlockID:   0,
			Timestamp: now,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ff.buildPacket(points, false, 0)
	}
}
