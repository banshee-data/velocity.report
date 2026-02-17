package network

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
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
	ParsePacket(packet []byte) ([]l4perception.PointPolar, error)
	GetLastMotorSpeed() uint16 // Get motor speed from last parsed packet
}

// FrameBuilder interface for building LiDAR frames
type FrameBuilder interface {
	AddPointsPolar(points []l4perception.PointPolar)
	SetMotorSpeed(rpm uint16) // Update expected frame duration based on motor speed
}

// UDPListener handles receiving and processing LiDAR packets from UDP
// with configurable components for parsing, statistics, and forwarding
type UDPListener struct {
	address        string
	rcvBuf         int
	logInterval    time.Duration
	connMu         sync.RWMutex // Protects conn field
	conn           UDPSocket
	stats          PacketStatsInterface
	forwarder      *PacketForwarder
	parser         Parser
	frameBuilder   FrameBuilder
	db             *db.DB
	disableParsing bool
	udpPort        int
	socketFactory  UDPSocketFactory
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
	UDPPort        int              // UDP port for normal operation (also used for PCAP filtering)
	SocketFactory  UDPSocketFactory // Optional: factory for creating UDP sockets (for testing)
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

	// Default to real socket factory if not provided
	socketFactory := config.SocketFactory
	if socketFactory == nil {
		socketFactory = NewRealUDPSocketFactory()
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
		socketFactory:  socketFactory,
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

	conn, err := l.socketFactory.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP address: %w", err)
	}
	l.setConn(conn)
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
	var deadlineErrLogged bool

	for {
		select {
		case <-ctx.Done():
			log.Print("UDP listener stopping due to context cancellation")
			return ctx.Err()
		default:
			// Set read deadline to allow checking context cancellation
			if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
				if !deadlineErrLogged {
					log.Printf("failed to set read deadline: %v", err)
					deadlineErrLogged = true
				}
				// Continue anyway - this is non-fatal
			}

			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Continue on timeout to check context
				}
				// Connection closed or context cancelled — clean shutdown.
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					return nil
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
			l.frameBuilder.AddPointsPolar(points)
		}
	}

	return nil
}

// setConn sets the connection with mutex protection
func (l *UDPListener) setConn(conn UDPSocket) {
	l.connMu.Lock()
	defer l.connMu.Unlock()
	l.conn = conn
}

// GetConn returns the connection with mutex protection (for testing)
func (l *UDPListener) GetConn() UDPSocket {
	l.connMu.RLock()
	defer l.connMu.RUnlock()
	return l.conn
}

// Close closes the UDP listener and releases resources.
// It is safe to call Close multiple times.
func (l *UDPListener) Close() error {
	l.connMu.Lock()
	conn := l.conn
	l.conn = nil
	l.connMu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}
