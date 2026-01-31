package network

import (
	"context"
	"net"
	"testing"
	"time"
)

// MockPacketStats implements the PacketStats interface for testing
type MockPacketStats struct {
	droppedCount int
}

func (m *MockPacketStats) AddDropped() {
	m.droppedCount++
}

func TestPacketForwarder_NewPacketForwarder(t *testing.T) {
	stats := &MockPacketStats{}
	logInterval := 2 * time.Second

	forwarder, err := NewPacketForwarder("localhost", 12345, stats, logInterval)
	if err != nil {
		t.Fatalf("NewPacketForwarder failed: %v", err)
	}

	if forwarder == nil {
		t.Fatal("NewPacketForwarder returned nil")
	}

	if forwarder.address != "localhost:12345" {
		t.Errorf("Expected address 'localhost:12345', got '%s'", forwarder.address)
	}

	// Clean up
	forwarder.conn.Close()
}

func TestPacketForwarder_StartStop(t *testing.T) {
	// Start a test UDP server to receive forwarded packets
	serverAddr, err := net.ResolveUDPAddr("udp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to resolve server address: %v", err)
	}

	server, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer server.Close()

	serverPort := server.LocalAddr().(*net.UDPAddr).Port

	// Create forwarder pointing to test server
	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("localhost", serverPort, stats, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer forwarder.conn.Close()

	// Start forwarder with context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	forwarder.Start(ctx)

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Send a test packet
	testPacket := []byte("test packet data")
	forwarder.ForwardAsync(testPacket)

	// Read the packet from server
	if err := server.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		t.Fatalf("Failed to set read deadline: %v", err)
	}
	buffer := make([]byte, 1024)
	n, _, err := server.ReadFromUDP(buffer)
	if err != nil {
		t.Fatalf("Failed to read from test server: %v", err)
	}

	received := string(buffer[:n])
	if received != "test packet data" {
		t.Errorf("Expected 'test packet data', got '%s'", received)
	}

	// Cancel context to stop forwarder
	cancel()
	time.Sleep(10 * time.Millisecond)
}

func TestPacketForwarder_ForwardAsync_BufferFull(t *testing.T) {
	stats := &MockPacketStats{}

	// Create forwarder that will work but not start it (so packets pile up in buffer)
	forwarder, err := NewPacketForwarder("localhost", 12345, stats, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer forwarder.conn.Close()

	// Fill buffer without starting forwarder - use many packets to exceed buffer
	testPacket := []byte("test")

	// Fill the channel buffer (1000 packets) plus extra to cause drops
	for i := 0; i < 1001; i++ {
		forwarder.ForwardAsync(testPacket)
	}

	// This should complete quickly since ForwardAsync is non-blocking
	time.Sleep(10 * time.Millisecond)
}

func TestPacketForwarder_InvalidAddress(t *testing.T) {
	stats := &MockPacketStats{}

	// Try to create forwarder with invalid address
	_, err := NewPacketForwarder("invalid-address-12345", 12345, stats, 1*time.Second)
	if err == nil {
		t.Error("Expected error for invalid address, got nil")
	}
}

// TestPacketForwarder_Close tests the Close function
func TestPacketForwarder_Close(t *testing.T) {
	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("localhost", 12346, stats, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}

	err = forwarder.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Verify channel is closed
	_, ok := <-forwarder.channel
	if ok {
		t.Error("Expected channel to be closed")
	}
}

// TestPacketForwarder_Close_MultiplePackets tests sending multiple packets before close
func TestPacketForwarder_Close_MultiplePackets(t *testing.T) {
	// Start a test UDP server
	serverAddr, err := net.ResolveUDPAddr("udp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to resolve server address: %v", err)
	}

	server, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer server.Close()

	serverPort := server.LocalAddr().(*net.UDPAddr).Port

	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("localhost", serverPort, stats, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	forwarder.Start(ctx)

	// Send multiple packets
	for i := 0; i < 10; i++ {
		forwarder.ForwardAsync([]byte("test packet"))
	}

	// Give time for packets to be processed
	time.Sleep(50 * time.Millisecond)

	// Close forwarder
	err = forwarder.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

// TestPacketForwarder_StartWithDroppedPackets tests logging of dropped packets
func TestPacketForwarder_StartWithDroppedPackets(t *testing.T) {
	// Create a forwarder that points to a closed port (will cause write errors)
	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("localhost", 1, stats, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer forwarder.conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	forwarder.Start(ctx)

	// Queue packets - some may fail to send
	for i := 0; i < 10; i++ {
		forwarder.ForwardAsync([]byte("test"))
	}

	// Wait for the log interval to pass
	time.Sleep(100 * time.Millisecond)

	cancel()
}

// TestPacketForwarder_ForwardAsync_PacketCopy tests that packets are copied
func TestPacketForwarder_ForwardAsync_PacketCopy(t *testing.T) {
	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("localhost", 12347, stats, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer forwarder.conn.Close()

	originalPacket := []byte("original data")
	forwarder.ForwardAsync(originalPacket)

	// Modify original packet
	originalPacket[0] = 'X'

	// Check that the queued packet is unchanged
	select {
	case queuedPacket := <-forwarder.channel:
		if queuedPacket[0] == 'X' {
			t.Error("Queued packet should be a copy, but was affected by original modification")
		}
		if string(queuedPacket) != "original data" {
			t.Errorf("Expected 'original data', got '%s'", string(queuedPacket))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Packet was not queued")
	}
}

// TestPacketForwarder_ChannelClosedDuringStart tests channel closure while running
func TestPacketForwarder_ChannelClosedDuringStart(t *testing.T) {
	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("localhost", 12348, stats, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}

	ctx := context.Background()
	forwarder.Start(ctx)

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Close the forwarder (closes channel and connection)
	forwarder.Close()

	// Goroutine should exit cleanly
	time.Sleep(20 * time.Millisecond)
}

func BenchmarkPacketForwarder_ForwardAsync(b *testing.B) {
	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("localhost", 12345, stats, 1*time.Second)
	if err != nil {
		b.Fatalf("Failed to create forwarder: %v", err)
	}
	defer forwarder.conn.Close()

	testPacket := make([]byte, 1262) // Typical lidar packet size

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		forwarder.ForwardAsync(testPacket)
	}
}
