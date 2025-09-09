package lidar

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidardb"
)

// UDPListener handles receiving and processing LiDAR packets from UDP
// It manages the UDP socket, packet forwarding, parsing, and statistics
type UDPListener struct {
	address        string
	rcvBuf         int
	logInterval    time.Duration
	buffer         []byte
	stats          *PacketStats
	forwarder      *PacketForwarder
	parser         *Pandar40PParser
	db             *lidardb.LidarDB
	disableParsing bool
}

// UDPListenerConfig contains configuration options for the UDP listener
type UDPListenerConfig struct {
	Address        string
	RcvBuf         int
	LogInterval    time.Duration
	Stats          *PacketStats
	Forwarder      *PacketForwarder
	Parser         *Pandar40PParser
	DB             *lidardb.LidarDB
	DisableParsing bool
}

// NewUDPListener creates a new UDP listener with the provided configuration
func NewUDPListener(config UDPListenerConfig) *UDPListener {
	return &UDPListener{
		address:        config.Address,
		rcvBuf:         config.RcvBuf,
		logInterval:    config.LogInterval,
		buffer:         make([]byte, 1500), // Buffer for typical lidar packet size
		stats:          config.Stats,
		forwarder:      config.Forwarder,
		parser:         config.Parser,
		db:             config.DB,
		disableParsing: config.DisableParsing,
	}
}

// Start begins listening for UDP packets and processing them
// Returns when the context is cancelled or an unrecoverable error occurs
func (l *UDPListener) Start(ctx context.Context) error {
	// Parse the address
	addr, err := net.ResolveUDPAddr("udp", l.address)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}

	// Create main UDP listener
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %v", err)
	}
	defer conn.Close()

	// Set socket receive buffer size
	if err := conn.SetReadBuffer(l.rcvBuf); err != nil {
		log.Printf("Warning: failed to set UDP receive buffer to %d bytes: %v (some OSes clamp buffer sizes)", l.rcvBuf, err)
	} else {
		log.Printf("Set UDP receive buffer to %d bytes", l.rcvBuf)
	}

	log.Printf("Listening for lidar packets on %s", l.address)

	// Start packet forwarding if forwarder is provided
	if l.forwarder != nil {
		l.forwarder.Start(ctx)
	}

	// Start periodic logging goroutine
	go l.startStatsLogging(ctx)

	log.Printf("Starting UDP packet receive loop...")
	timeoutCount := 0

	for {
		select {
		case <-ctx.Done():
			log.Println("UDP listener shutting down")
			return ctx.Err()
		default:
			// Set a read timeout to allow checking for context cancellation
			if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
				log.Printf("Error setting read deadline: %v", err)
				continue
			}

			n, clientAddr, err := conn.ReadFromUDP(l.buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Timeout is expected, continue the loop
					timeoutCount++
					if timeoutCount%10 == 0 {
						log.Printf("No packets received for %d seconds", timeoutCount)
					}
					continue
				}
				log.Printf("Error reading UDP packet: %v", err)
				continue
			}

			// Reset timeout counter when we receive a packet
			timeoutCount = 0

			// Handle the packet directly using the reused buffer (no allocation per packet).
			// Note: buffer[:n] creates a slice view without copying data.
			// Any function that needs to store the packet data beyond this call
			// must make its own copy (see forwarder.ForwardAsync for an example).
			if err := l.handlePacket(l.buffer[:n], clientAddr); err != nil {
				log.Printf("Error handling lidar packet: %v", err)
			}
		}
	}
}

// startStatsLogging starts a goroutine that logs statistics at regular intervals
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

		// @TODO: store points in database
	}

	return nil
}

// Close cleans up resources used by the UDP listener
func (l *UDPListener) Close() error {
	if l.forwarder != nil {
		return l.forwarder.Close()
	}
	return nil
}
