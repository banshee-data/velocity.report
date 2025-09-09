package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"html/template"
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

//go:embed status.html
var statusHTML embed.FS

var (
	listen         = flag.String("listen", ":8081", "HTTP listen address")
	udpPort        = flag.Int("udp-port", 2369, "UDP port to listen for lidar packets")
	udpAddress     = flag.String("udp-addr", "", "UDP bind address (default: listen on all interfaces)")
	disableParsing = flag.Bool("no-parse", false, "Disable lidar packet parsing (parsing is enabled by default)")
	forwardPackets = flag.Bool("forward", false, "Forward received UDP packets to another port")
	forwardPort    = flag.Int("forward-port", 2368, "Port to forward UDP packets to (for LidarView monitoring)")
	forwardAddr    = flag.String("forward-addr", "localhost", "Address to forward UDP packets to")
	dbFile         = flag.String("db", "lidar_data.db", "Path to the SQLite database file")
	rcvBuf         = flag.Int("rcvbuf", 4<<20, "UDP receive buffer size in bytes (default 4MB)")
	logInterval    = flag.Int("log-interval", 2, "Statistics logging interval in seconds")
)

// Constants
const SCHEMA_VERSION = "0.1.0"

// Fast packet forwarding without blocking main receive loop
func forwardPacketAsync(stats *lidar.PacketStats, forwardConn *net.UDPConn, packet []byte, forwardChan chan []byte) {
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

func handleLidarPacket(stats *lidar.PacketStats,
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
	if parser != nil && !*disableParsing {
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
func listenUDP(ctx context.Context, ldb *lidardb.LidarDB, parser *lidar.Pandar40PParser, stats *lidar.PacketStats, address string) error {
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
			var lastError error
			ticker := time.NewTicker(time.Duration(*logInterval) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case packet := <-forwardChan:
					_, err := forwardConn.Write(packet)
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

		log.Printf("Forwarding packets to %s", forwardAddress)
	}

	// Initialize packet statistics (passed from main)
	// stats := lidar.NewPacketStats()

	// Start periodic logging goroutine
	go func() {
		ticker := time.NewTicker(time.Duration(*logInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats.LogStats(!*disableParsing)
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
	if !*disableParsing {
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
		log.Println("Lidar packet parsing disabled (--no-parse flag was specified)")
	}

	// Create a wait group for the HTTP server and UDP listener routines
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize packet statistics (shared between UDP listener and HTTP server)
	stats := lidar.NewPacketStats()

	// Start UDP listener routine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := listenUDP(ctx, ldb, parser, stats, udpListenAddr); err != nil && err != context.Canceled {
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

			parsingStatus := "enabled"
			if *disableParsing {
				parsingStatus = "disabled"
			}

			// Load and parse the HTML template from embedded filesystem
			tmpl, err := template.ParseFS(statusHTML, "status.html")
			if err != nil {
				http.Error(w, "Error loading template: "+err.Error(), http.StatusInternalServerError)
				return
			}

			// Template data
			data := struct {
				UDPPort          int
				HTTPAddress      string
				ForwardingStatus string
				ParsingStatus    string
				Uptime           string
				Stats            *lidar.StatsSnapshot
			}{
				UDPPort:          *udpPort,
				HTTPAddress:      *listen,
				ForwardingStatus: forwardingStatus,
				ParsingStatus:    parsingStatus,
				Uptime:           stats.GetUptime().Round(time.Second).String(),
				Stats:            stats.GetLatestSnapshot(),
			}

			if err := tmpl.Execute(w, data); err != nil {
				http.Error(w, "Error executing template: "+err.Error(), http.StatusInternalServerError)
				return
			}
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
