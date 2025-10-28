package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
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
	ParsePacket(packet []byte) ([]lidar.PointPolar, error)
	GetLastMotorSpeed() uint16 // Get motor speed from last parsed packet
}

// FrameBuilder interface for building LiDAR frames
type FrameBuilder interface {
	AddPointsPolar(points []lidar.PointPolar)
	SetMotorSpeed(rpm uint16) // Update expected frame duration based on motor speed
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
	db             *db.DB
	disableParsing bool
	udpPort        int
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
	DB             *db.DB
	DisableParsing bool
	UDPPort        int // UDP port for normal operation (also used for PCAP filtering)
}

// NewUDPListener creates a new UDP listener with the provided configuration
func NewUDPListener(config UDPListenerConfig) *UDPListener {
	// Provide a no-op stats implementation when none is supplied to avoid
	// nil pointer dereferences in the packet handling and logging paths.
	var stats PacketStatsInterface
	if config.Stats != nil {
		stats = config.Stats
	} else {
		stats = &noopStats{}
	}

	// Default a sensible log interval if not provided
	logInterval := config.LogInterval
	if logInterval == 0 {
		logInterval = time.Minute
	}

	return &UDPListener{
		address:        config.Address,
		rcvBuf:         config.RcvBuf,
		logInterval:    logInterval,
		stats:          stats,
		forwarder:      config.Forwarder,
		parser:         config.Parser,
		frameBuilder:   config.FrameBuilder,
		db:             config.DB,
		disableParsing: config.DisableParsing,
		udpPort:        config.UDPPort,
	}
}

// noopStats is a PacketStatsInterface implementation that does nothing.
// It is used as a safe default when no stats collector is provided.
type noopStats struct{}

func (n *noopStats) AddPacket(bytes int)        {}
func (n *noopStats) AddDropped()                {}
func (n *noopStats) AddPoints(count int)        {}
func (n *noopStats) LogStats(parsePackets bool) {}

// Start begins listening for UDP packets and processing them
func (l *UDPListener) Start(ctx context.Context) error {
	// Normal UDP socket listening
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
			if err := l.handlePacket(packet); err != nil {
				log.Printf("Error handling packet from %v: %v", addr, err)
			}
		}
	}
}

// startStatsLogging starts a goroutine that periodically logs packet statistics
func (l *UDPListener) startStatsLogging(ctx context.Context) {
	// Trigger an initial stats report shortly after startup to avoid a long
	// silence on first-run. Then continue on the configured interval.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
		l.stats.LogStats(!l.disableParsing)
	}

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
func (l *UDPListener) handlePacket(packet []byte) error {
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

		// Update frame builder with current motor speed for time-based frame detection
		if l.frameBuilder != nil {
			motorSpeed := l.parser.GetLastMotorSpeed()
			if motorSpeed > 0 {
				l.frameBuilder.SetMotorSpeed(motorSpeed)
			}
		}

		// Add points to FrameBuilder for complete rotation accumulation
		if l.frameBuilder != nil && len(points) > 0 {
			// Prefer polar-aware API if available
			if fbPolar, ok := l.frameBuilder.(interface{ AddPointsPolar([]lidar.PointPolar) }); ok {
				fbPolar.AddPointsPolar(points)
			} else {
				// Fallback: convert to cartesian Points and call legacy AddPoints
				pts := make([]lidar.Point, 0, len(points))
				for _, p := range points {
					x, y, z := lidar.SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)
					pts = append(pts, lidar.Point{
						X:           x,
						Y:           y,
						Z:           z,
						Intensity:   p.Intensity,
						Distance:    p.Distance,
						Azimuth:     p.Azimuth,
						Elevation:   p.Elevation,
						Channel:     p.Channel,
						Timestamp:   time.Unix(0, p.Timestamp),
						BlockID:     p.BlockID,
						UDPSequence: p.UDPSequence,
					})
				}
				// Convert cartesian points back to polar and use polar API
				polarPts := make([]lidar.PointPolar, 0, len(pts))
				for _, p := range pts {
					polarPts = append(polarPts, lidar.PointPolar{
						Channel:     p.Channel,
						Azimuth:     p.Azimuth,
						Elevation:   p.Elevation,
						Distance:    p.Distance,
						Intensity:   p.Intensity,
						Timestamp:   p.Timestamp.UnixNano(),
						BlockID:     p.BlockID,
						UDPSequence: p.UDPSequence,
					})
				}
				l.frameBuilder.AddPointsPolar(polarPts)
			}
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
