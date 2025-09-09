package lidar

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestPacketForwarder_NewPacketForwarder(t *testing.T) {
	stats := NewPacketStats()
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
	stats := NewPacketStats()
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
	server.SetReadDeadline(time.Now().Add(1 * time.Second))
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
	stats := NewPacketStats()

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
	stats := NewPacketStats()

	// Try to create forwarder with invalid address
	_, err := NewPacketForwarder("invalid-address-12345", 12345, stats, 1*time.Second)
	if err == nil {
		t.Error("Expected error for invalid address, got nil")
	}
}

func BenchmarkPacketForwarder_ForwardAsync(b *testing.B) {
	stats := NewPacketStats()
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
