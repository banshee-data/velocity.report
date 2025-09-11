package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/lidardb"
)

// PacketStatsInterface provides packet statistics management
type PacketStatsInterface interface {
	AddPacket(bytes int)
	AddDropped()
	AddPoints(count int)
	LogStats(parsePackets bool)
}

// Parser interface for parsing LiDAR packets
type Parser interface {
	ParsePacket(packet []byte) ([]lidar.Point, error)
}

// FrameBuilder interface for building LiDAR frames
type FrameBuilder interface {
	AddPoints(points []lidar.Point)
}

// UDPListener handles receiving and processing LiDAR packets from UDP
// with configurable components for parsing, statistics, and forwarding
type UDPListener struct {
	address        string
	rcvBuf         int
	logInterval    time.Duration
	conn           *net.UDPConn
	stats          PacketStatsInterface
	forwarder      *PacketForwarder
	parser         Parser
	frameBuilder   FrameBuilder
	db             *lidardb.LidarDB
	disableParsing bool
}

// UDPListenerConfig contains configuration options for the UDP listener
type UDPListenerConfig struct {
	Address        string
	RcvBuf         int
	LogInterval    time.Duration
	Stats          PacketStatsInterface
	Forwarder      *PacketForwarder
	Parser         Parser
	FrameBuilder   FrameBuilder
	DB             *lidardb.LidarDB
	DisableParsing bool
}

// NewUDPListener creates a new UDP listener with the provided configuration
func NewUDPListener(config UDPListenerConfig) *UDPListener {
	return &UDPListener{
		address:        config.Address,
		rcvBuf:         config.RcvBuf,
		logInterval:    config.LogInterval,
		stats:          config.Stats,
		forwarder:      config.Forwarder,
		parser:         config.Parser,
		frameBuilder:   config.FrameBuilder,
		db:             config.DB,
		disableParsing: config.DisableParsing,
	}
}

// Start begins listening for UDP packets and processing them
func (l *UDPListener) Start(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", l.address)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP address: %w", err)
	}
	l.conn = conn
	defer conn.Close()

	// Set receive buffer size
	if err := conn.SetReadBuffer(l.rcvBuf); err != nil {
		log.Printf("Warning: Failed to set UDP receive buffer size to %d: %v", l.rcvBuf, err)
	}

	log.Printf("UDP listener started on %s with receive buffer %d bytes", l.address, l.rcvBuf)

	// Start forwarder if configured
	if l.forwarder != nil {
		l.forwarder.Start(ctx)
	}

	// Start statistics logging
	go l.startStatsLogging(ctx)

	// Prepare buffer for incoming packets
	buffer := make([]byte, 2048) // Pandar40P packets are 1262 bytes + some margin

	for {
		select {
		case <-ctx.Done():
			log.Print("UDP listener stopping due to context cancellation")
			return ctx.Err()
		default:
			// Set read deadline to allow checking context cancellation
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Continue on timeout to check context
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Printf("UDP read error: %v", err)
				continue
			}

			// Handle the received packet
			packet := buffer[:n]
			if err := l.handlePacket(packet, addr); err != nil {
				log.Printf("Error handling packet from %v: %v", addr, err)
			}
		}
	}
}

// startStatsLogging starts a goroutine that periodically logs packet statistics
func (l *UDPListener) startStatsLogging(ctx context.Context) {
	ticker := time.NewTicker(l.logInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.stats.LogStats(!l.disableParsing)
		}
	}
}

// handlePacket processes a single received UDP packet
func (l *UDPListener) handlePacket(packet []byte, addr *net.UDPAddr) error {
	// Track packet statistics
	l.stats.AddPacket(len(packet))

	// Forward packet asynchronously if forwarding is enabled
	if l.forwarder != nil {
		l.forwarder.ForwardAsync(packet)
	}

	// Parse packet if parser is available and parsing is enabled
	if l.parser != nil && !l.disableParsing {
		points, err := l.parser.ParsePacket(packet)
		if err != nil {
			log.Printf("Pandar40P parsing failed: %v", err)
			return nil // Don't fail on parse errors, just continue
		}

		// Track parsed points in statistics
		l.stats.AddPoints(len(points))

		// Add points to FrameBuilder for complete rotation accumulation
		if l.frameBuilder != nil && len(points) > 0 {
			l.frameBuilder.AddPoints(points)
		}
	}

	return nil
}

// Close closes the UDP listener and releases resources
func (l *UDPListener) Close() error {
	if l.conn != nil {
		return l.conn.Close()
	}
	return nil
}
