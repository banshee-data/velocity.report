package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidardb"
)

var (
	listen         = flag.String("listen", ":8081", "HTTP listen address")
	udpPort        = flag.Int("udp-port", 2369, "UDP port to listen for lidar packets")
	udpAddress     = flag.String("udp-addr", "", "UDP bind address (default: listen on all interfaces)")
	parsePackets   = flag.Bool("parse", false, "Parse lidar packets into points using embedded sensor config")
	forwardPackets = flag.Bool("forward", false, "Forward received UDP packets to another port")
	forwardPort    = flag.Int("forward-port", 2368, "Port to forward UDP packets to (for LidarView monitoring)")
	forwardAddr    = flag.String("forward-addr", "localhost", "Address to forward UDP packets to")
	dbFile         = flag.String("db", "lidar_data.db", "Path to the SQLite database file")
	rcvBuf         = flag.Int("rcvbuf", 4<<20, "UDP receive buffer size in bytes (default 4MB)")
	logInterval    = flag.Int("log-interval", 2, "Statistics logging interval in seconds")
)

// Constants
const SCHEMA_VERSION = "0.1.0"

// formatWithCommas formats a number with thousands separators
func formatWithCommas(n int64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	result := ""
	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(char)
	}
	return result
}

// Packet statistics tracking
type PacketStats struct {
	mu           sync.Mutex
	packetCount  int64
	byteCount    int64
	droppedCount int64
	pointCount   int64
	lastReset    time.Time
}

func (ps *PacketStats) AddPacket(bytes int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.packetCount++
	ps.byteCount += int64(bytes)
}

func (ps *PacketStats) AddDropped() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.droppedCount++
}

func (ps *PacketStats) AddPoints(count int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.pointCount += int64(count)
}

func (ps *PacketStats) GetAndReset() (packets int64, bytes int64, dropped int64, points int64, duration time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	duration = now.Sub(ps.lastReset)
	packets = ps.packetCount
	bytes = ps.byteCount
	dropped = ps.droppedCount
	points = ps.pointCount

	ps.packetCount = 0
	ps.byteCount = 0
	ps.droppedCount = 0
	ps.pointCount = 0
	ps.lastReset = now

	return
}

// Fast packet forwarding without blocking main receive loop
func forwardPacketAsync(stats *PacketStats, forwardConn *net.UDPConn, packet []byte, forwardChan chan []byte) {
	if forwardConn != nil && *forwardPackets {
		// Make a copy of the packet data to avoid buffer sharing issues
		packetCopy := make([]byte, len(packet))
		copy(packetCopy, packet)

		// Send to forwarding channel (non-blocking)
		select {
		case forwardChan <- packetCopy:
		default:
			// Drop packet if forwarding buffer is full (prevents blocking)
			stats.AddDropped()
		}
	}
}

func handleLidarPacket(stats *PacketStats,
	ldb *lidardb.LidarDB,
	parser *lidar.Pandar40PParser,
	packet []byte, addr *net.UDPAddr,
	forwardConn *net.UDPConn,
	forwardChan chan []byte) error {
	// Track packet statistics
	stats.AddPacket(len(packet))

	// Forward packet asynchronously if forwarding is enabled
	forwardPacketAsync(stats, forwardConn, packet, forwardChan)

	// Parse packet if parser is available and parsing is enabled
	if parser != nil && *parsePackets {
		points, err := parser.ParsePacket(packet)
		if err != nil {
			log.Printf("Pandar40P parsing failed: %v", err)
			return nil // Don't fail on parse errors, just continue
		}

		// Track parsed points in statistics
		stats.AddPoints(len(points))

		// Store parsed points in database if requested
		// for _, point := range points {
		// 	err := ldb.RecordLidarPoint(0, point.X, point.Y, point.Z, int(point.Intensity),
		// 		point.Timestamp.UnixNano(), point.Azimuth, point.Distance)
		// 	if err != nil {
		// 		log.Printf("Failed to store point: %v", err)
		// 	}
		// }
	}

	return nil
}

// UDP listener function with optimized packet forwarding
func listenUDP(ctx context.Context, ldb *lidardb.LidarDB, parser *lidar.Pandar40PParser, address string) error {
	// Parse the address
	addr, err := net.ResolveUDPAddr("udp", address)
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
	if err := conn.SetReadBuffer(*rcvBuf); err != nil {
		log.Printf("Warning: failed to set UDP receive buffer to %d bytes: %v (some OSes clamp buffer sizes)", *rcvBuf, err)
	} else {
		log.Printf("Set UDP receive buffer to %d bytes", *rcvBuf)
	}

	log.Printf("Listening for lidar packets on %s", address)

	// Setup packet forwarding if enabled
	var forwardConn *net.UDPConn
	var forwardChan chan []byte
	if *forwardPackets {
		forwardAddress := fmt.Sprintf("%s:%d", *forwardAddr, *forwardPort)
		forwardUDPAddr, err := net.ResolveUDPAddr("udp", forwardAddress)
		if err != nil {
			return fmt.Errorf("failed to resolve forward address: %v", err)
		}

		forwardConn, err = net.DialUDP("udp", nil, forwardUDPAddr)
		if err != nil {
			return fmt.Errorf("failed to create forward connection: %v", err)
		}
		defer forwardConn.Close()

		// Create buffered channel for packet forwarding (buffer 1000 packets)
		forwardChan = make(chan []byte, 1000)

		// Start dedicated forwarding goroutine
		go func() {
			droppedCount := 0
			for {
				select {
				case <-ctx.Done():
					return
				case packet := <-forwardChan:
					_, err := forwardConn.Write(packet)
					if err != nil {
						droppedCount++
						// Only log every 100th error to avoid spamming logs
						if droppedCount%100 == 0 {
							log.Printf("Dropped %d forwarded packets due to errors (latest: %v)", droppedCount, err)
						}
					}
				}
			}
		}()

		log.Printf("Forwarding packets to %s", forwardAddress)
	}

	// Initialize packet statistics
	stats := &PacketStats{lastReset: time.Now()}

	// Start periodic logging goroutine
	go func() {
		ticker := time.NewTicker(time.Duration(*logInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				packets, bytes, dropped, points, duration := stats.GetAndReset()
				if packets > 0 || dropped > 0 {
					packetsPerSec := float64(packets) / duration.Seconds()
					mbPerSec := float64(bytes) / duration.Seconds() / (1024 * 1024)
					pointsPerSec := float64(points) / duration.Seconds()

					var logMsg string
					if *parsePackets && points > 0 {
						logMsg = fmt.Sprintf("Lidar stats (/sec): %.1f MB, %.1f packets, %s points",
							mbPerSec, packetsPerSec, formatWithCommas(int64(pointsPerSec)))
					} else {
						logMsg = fmt.Sprintf("Lidar stats (/sec): %.1f MB, %.1f packets",
							mbPerSec, packetsPerSec)
					}

					if dropped > 0 {
						logMsg += fmt.Sprintf(", %d dropped on forward", dropped)
					}

					log.Print(logMsg)
				}
			}
		}
	}()

	// Buffer for incoming packets - sized for typical lidar packet (~1200-1500 bytes)
	buffer := make([]byte, 1500)

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

			n, clientAddr, err := conn.ReadFromUDP(buffer)
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
			// must make its own copy (see forwardPacketAsync for an example).
			if err := handleLidarPacket(stats, ldb, parser, buffer[:n], clientAddr, forwardConn, forwardChan); err != nil {
				log.Printf("Error handling lidar packet: %v", err)
			}
		}
	}
}

// Main
func main() {
	flag.Parse()

	if *listen == "" {
		log.Fatal("HTTP listen address is required")
	}

	// Construct UDP listen address
	var udpListenAddr string
	if *udpAddress == "" {
		udpListenAddr = fmt.Sprintf(":%d", *udpPort)
	} else {
		udpListenAddr = fmt.Sprintf("%s:%d", *udpAddress, *udpPort)
	}

	// Initialize database
	ldb, err := lidardb.NewLidarDB(*dbFile)
	if err != nil {
		log.Fatalf("Failed to connect to lidar database: %v", err)
	}
	defer ldb.Close()

	// Initialize parser if parsing is enabled
	var parser *lidar.Pandar40PParser
	if *parsePackets {
		log.Printf("Loading embedded Pandar40P sensor configuration")
		config, err := lidar.LoadEmbeddedPandar40PConfig()
		if err != nil {
			log.Fatalf("Failed to load embedded lidar configuration: %v", err)
		}

		err = config.Validate()
		if err != nil {
			log.Fatalf("Invalid embedded lidar configuration: %v", err)
		}

		parser = lidar.NewPandar40PParser(*config)
		log.Println("Lidar packet parsing enabled")
	} else {
		log.Println("Lidar packet parsing disabled (use -parse flag to enable)")
	}

	// Create a wait group for the HTTP server and UDP listener routines
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start UDP listener routine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := listenUDP(ctx, ldb, parser, udpListenAddr); err != nil && err != context.Canceled {
			log.Printf("UDP listener error: %v", err)
		}
		log.Print("UDP listener routine terminated")
	}()

	// HTTP server goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Create a simple HTTP multiplexer for lidar API
		mux := http.NewServeMux()

		// Health check endpoint
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status": "ok", "service": "lidar", "timestamp": "%s"}`, time.Now().UTC().Format(time.RFC3339))
		})

		// Basic info endpoint
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html")

			forwardingStatus := "disabled"
			if *forwardPackets {
				forwardingStatus = fmt.Sprintf("enabled (%s:%d)", *forwardAddr, *forwardPort)
			}

			parsingStatus := "disabled"
			if *parsePackets {
				parsingStatus = "enabled"
			}

			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>Lidar UDP Listener</title></head>
<body>
	<h1>Lidar UDP Listener</h1>
	<p>Listening on UDP port %d</p>
	<p>HTTP server running on %s</p>
	<p>Packet forwarding: %s</p>
	<p>Packet parsing: %s</p>
	<ul>
		<li><a href="/health">Health check</a></li>
	</ul>
</body>
</html>`, *udpPort, *listen, forwardingStatus, parsingStatus)
		})

		server := &http.Server{
			Addr:    *listen,
			Handler: mux,
		}

		// Start server in a goroutine so it doesn't block
		go func() {
			log.Printf("Starting HTTP server on %s", *listen)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("failed to start server: %v", err)
			}
		}()

		// Wait for context cancellation to shut down server
		<-ctx.Done()
		log.Println("shutting down HTTP server...")

		// Create a shutdown context with a shorter timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
			// Force close the server if graceful shutdown fails
			if err := server.Close(); err != nil {
				log.Printf("HTTP server force close error: %v", err)
			}
		}

		log.Printf("HTTP server routine stopped")
	}()

	// Wait for all goroutines to finish
	wg.Wait()
	log.Printf("Graceful shutdown complete")
}
