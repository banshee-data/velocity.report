package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
)

// PacketStats interface for packet statistics tracking
type PacketStats interface {
	AddDropped()
}

// PacketForwarder handles asynchronous forwarding of UDP packets to another address
// It provides non-blocking packet forwarding with error tracking and logging
type PacketForwarder struct {
	conn        *net.UDPConn
	channel     chan []byte
	stats       PacketStats
	logInterval time.Duration
	address     string
}

// NewPacketForwarder creates a new packet forwarder that sends packets to the specified address
func NewPacketForwarder(addr string, port int, stats PacketStats, logInterval time.Duration) (*PacketForwarder, error) {
	forwardAddress := fmt.Sprintf("%s:%d", addr, port)
	forwardUDPAddr, err := net.ResolveUDPAddr("udp", forwardAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve forward address: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, forwardUDPAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create forward connection: %v", err)
	}

	return &PacketForwarder{
		conn:        conn,
		channel:     make(chan []byte, 1000), // Buffer 1000 packets
		stats:       stats,
		logInterval: logInterval,
		address:     forwardAddress,
	}, nil
}

// Start begins the packet forwarding goroutine that processes packets from the channel
// It logs dropped packets at the specified interval and handles context cancellation
func (f *PacketForwarder) Start(ctx context.Context) {
	go func() {
		droppedCount := 0
		var lastError error
		ticker := time.NewTicker(f.logInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case packet := <-f.channel:
				_, err := f.conn.Write(packet)
				if err != nil {
					droppedCount++
					lastError = err
				}
			case <-ticker.C:
				// Only log if we have dropped packets in this interval
				if droppedCount > 0 && lastError != nil {
					log.Printf("\033[93mDropped %d forwarded packets due to errors (latest: %v)\033[0m", droppedCount, lastError)
					droppedCount = 0 // Reset counter after logging
					lastError = nil
				}
			}
		}
	}()

	log.Printf("Forwarding packets to %s", f.address)
}

// ForwardAsync sends a packet to the forwarding channel in a non-blocking manner
// If the channel is full, the packet is dropped and the drop counter is incremented
func (f *PacketForwarder) ForwardAsync(packet []byte) {
	// Make a copy of the packet data to avoid buffer sharing issues
	packetCopy := make([]byte, len(packet))
	copy(packetCopy, packet)

	// Send to forwarding channel (non-blocking)
	select {
	case f.channel <- packetCopy:
		// Packet successfully queued for forwarding
	default:
		// Drop packet if forwarding buffer is full (prevents blocking)
		f.stats.AddDropped()
	}
}

// Close closes the UDP connection and channel
func (f *PacketForwarder) Close() error {
	close(f.channel)
	return f.conn.Close()
}
