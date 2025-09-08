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

	"github.com/banshee-data/velocity.report/internal/lidardb"
)

var (
	listen     = flag.String("listen", ":8080", "HTTP listen address")
	udpPort    = flag.Int("udp-port", 2369, "UDP port to listen for lidar packets")
	udpAddress = flag.String("udp-addr", "", "UDP bind address (default: listen on all interfaces)")
)

// Constants
const DB_FILE = "lidar_data.db"
const SCHEMA_VERSION = "0.1.0"

// Packet statistics tracking
type PacketStats struct {
	mu          sync.Mutex
	packetCount int64
	byteCount   int64
	lastReset   time.Time
}

func (ps *PacketStats) AddPacket(bytes int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.packetCount++
	ps.byteCount += int64(bytes)
}

func (ps *PacketStats) GetAndReset() (packets int64, bytes int64, duration time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	duration = now.Sub(ps.lastReset)
	packets = ps.packetCount
	bytes = ps.byteCount

	ps.packetCount = 0
	ps.byteCount = 0
	ps.lastReset = now

	return
}

func handleLidarPacket(stats *PacketStats, packet []byte, addr *net.UDPAddr) error {
	// Track packet statistics instead of logging each packet
	stats.AddPacket(len(packet))

	// Store the raw packet in the lidar database
	// Commented out - use wireshark/pcaps for raw packet capture instead
	// return ldb.RecordLidarPacket(packet, addr)

	return nil
}

// UDP listener function
func listenUDP(ctx context.Context, ldb *lidardb.LidarDB, address string) error {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %v", err)
	}
	defer conn.Close()

	log.Printf("Listening for lidar packets on %s", address)

	// Initialize packet statistics
	stats := &PacketStats{lastReset: time.Now()}

	// Start periodic logging goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				packets, bytes, duration := stats.GetAndReset()
				if packets > 0 {
					packetsPerSec := float64(packets) / duration.Seconds()
					bytesPerSec := float64(bytes) / duration.Seconds()
					log.Printf("Lidar stats: %.1f packets/sec, %.1f bytes/sec (%.1f KB/sec)",
						packetsPerSec, bytesPerSec, bytesPerSec/1024)
				}
			}
		}
	}()

	// Buffer for incoming packets - adjust size based on expected lidar packet size
	buffer := make([]byte, 65536) // 64KB buffer

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
					continue
				}
				log.Printf("Error reading UDP packet: %v", err)
				continue
			}

			// Handle the packet in a goroutine to avoid blocking
			go func(packet []byte, addr *net.UDPAddr) {
				if err := handleLidarPacket(stats, packet, addr); err != nil {
					log.Printf("Error handling lidar packet: %v", err)
				}
			}(buffer[:n], clientAddr)
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
	ldb, err := lidardb.NewLidarDB(DB_FILE)
	if err != nil {
		log.Fatalf("Failed to connect to lidar database: %v", err)
	}
	defer ldb.Close()

	// Create a wait group for the HTTP server and UDP listener routines
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start UDP listener routine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := listenUDP(ctx, ldb, udpListenAddr); err != nil && err != context.Canceled {
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
			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>Lidar UDP Listener</title></head>
<body>
	<h1>Lidar UDP Listener</h1>
	<p>Listening on UDP port %d</p>
	<p>HTTP server running on %s</p>
	<ul>
		<li><a href="/health">Health check</a></li>
	</ul>
</body>
</html>`, *udpPort, *listen)
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
