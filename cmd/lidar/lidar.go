package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/network"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
	"github.com/banshee-data/velocity.report/internal/lidardb"
)

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
	debug          = flag.Bool("debug", false, "Enable debug logging (UDP sequences, frame completion details)")
)

// Constants
const SCHEMA_VERSION = "0.1.0"

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
	var parser *parse.Pandar40PParser
	var frameBuilder *lidar.FrameBuilder
	if !*disableParsing {
		log.Printf("Loading embedded Pandar40P sensor configuration")
		config, err := parse.LoadEmbeddedPandar40PConfig()
		if err != nil {
			log.Fatalf("Failed to load embedded lidar configuration: %v", err)
		}

		err = config.Validate()
		if err != nil {
			log.Fatalf("Invalid embedded lidar configuration: %v", err)
		}

		parser = parse.NewPandar40PParser(*config)

		// Configure debug mode
		parser.SetDebug(*debug)

		// Configure timestamp mode based on environment
		// Default to PTP free-run mode for best timing consistency
		timestampMode := os.Getenv("LIDAR_TIMESTAMP_MODE")
		switch timestampMode {
		case "system":
			parser.SetTimestampMode(parse.TimestampModeSystemTime)
			log.Println("LiDAR timestamp mode: System time")
		case "gps":
			parser.SetTimestampMode(parse.TimestampModeGPS)
			log.Println("LiDAR timestamp mode: GPS (requires GPS-synchronized LiDAR)")
		case "internal":
			parser.SetTimestampMode(parse.TimestampModeInternal)
			log.Println("LiDAR timestamp mode: Internal (device boot time)")
		default:
			// Default to SystemTime for stability until PTP hardware is available
			parser.SetTimestampMode(parse.TimestampModeSystemTime)
			log.Println("LiDAR timestamp mode: System time (default - stable until PTP hardware available)")
		}

		log.Println("Lidar packet parsing enabled")

		// Create FrameBuilder for accumulating points into complete rotations
		frameBuilder = lidar.NewFrameBuilderWithDebugLoggingAndInterval("hesai-pandar40p", *debug, time.Duration(*logInterval)*time.Second)
		if *debug {
			log.Println("FrameBuilder initialized for complete rotation detection (debug mode enabled)")
		} else {
			log.Println("FrameBuilder initialized for complete rotation detection")
		}
	} else {
		log.Println("Lidar packet parsing disabled (--no-parse flag was specified)")
	}

	// Create a wait group for the HTTP server and UDP listener routines
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize packet statistics (shared between UDP listener and HTTP server)
	stats := lidar.NewPacketStats()

	// Create packet forwarder if forwarding is enabled
	var forwarder *network.PacketForwarder
	if *forwardPackets {
		forwarder, err = network.NewPacketForwarder(*forwardAddr, *forwardPort, stats, time.Duration(*logInterval)*time.Second)
		if err != nil {
			log.Fatalf("Failed to create packet forwarder: %v", err)
		}
		defer forwarder.Close()
	}

	// Create and start UDP listener
	udpListener := network.NewUDPListener(network.UDPListenerConfig{
		Address:        udpListenAddr,
		RcvBuf:         *rcvBuf,
		LogInterval:    time.Duration(*logInterval) * time.Second,
		Stats:          stats,
		Forwarder:      forwarder,
		Parser:         parser,
		FrameBuilder:   frameBuilder,
		DB:             ldb,
		DisableParsing: *disableParsing,
	})

	// Start UDP listener routine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := udpListener.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("UDP listener error: %v", err)
		}
		log.Print("UDP listener routine terminated")
	}()

	// Create and start web server
	webServer := lidar.NewWebServer(lidar.WebServerConfig{
		Address:           *listen,
		Stats:             stats,
		ForwardingEnabled: *forwardPackets,
		ForwardAddr:       *forwardAddr,
		ForwardPort:       *forwardPort,
		ParsingEnabled:    !*disableParsing,
		UDPPort:           *udpPort,
	})

	// Start HTTP server routine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := webServer.Start(ctx); err != nil {
			log.Printf("Web server error: %v", err)
		}
	}()

	// Wait for all goroutines to finish
	wg.Wait()
	log.Printf("Graceful shutdown complete")
}
