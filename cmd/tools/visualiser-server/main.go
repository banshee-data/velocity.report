// Command visualiser-server runs a gRPC visualiser server.
//
// This tool supports three modes:
//   - synthetic: Generate synthetic point clouds for testing (default)
//   - replay:    Play back a recorded .vrlog file
//   - live:      Stream live data from the pipeline (future)
//
// Usage:
//
//	go run ./cmd/tools/visualiser-server [flags]
//
// Flags:
//
//	-addr    Listen address (default: localhost:50051)
//	-mode    Server mode: synthetic, replay, live (default: synthetic)
//
// Synthetic Mode Flags:
//
//	-rate    Frame rate in Hz (default: 10)
//	-points  Points per frame (default: 10000)
//	-tracks  Number of synthetic tracks (default: 10)
//
// Replay Mode Flags:
//
//	-log     Path to .vrlog directory (required for replay mode)
//	-loop    Loop playback when reaching end (default: false)
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
)

func main() {
	// Common flags
	addr := flag.String("addr", "localhost:50051", "Listen address")
	mode := flag.String("mode", "synthetic", "Server mode: synthetic, replay, live")

	// Synthetic mode flags
	rate := flag.Float64("rate", 10, "Frame rate in Hz (synthetic mode)")
	points := flag.Int("points", 10000, "Points per frame (synthetic mode)")
	tracks := flag.Int("tracks", 10, "Number of synthetic tracks (synthetic mode)")

	// Replay mode flags
	logPath := flag.String("log", "", "Path to .vrlog directory (replay mode)")
	loop := flag.Bool("loop", false, "Loop playback when reaching end (replay mode)")
	_ = loop // TODO: implement loop functionality

	flag.Parse()

	switch *mode {
	case "synthetic":
		runSyntheticMode(*addr, *rate, *points, *tracks)
	case "replay":
		if *logPath == "" {
			log.Fatal("Error: -log flag is required for replay mode")
		}
		runReplayMode(*addr, *logPath)
	case "live":
		log.Fatal("Live mode not yet implemented")
	default:
		log.Fatalf("Unknown mode: %s (expected: synthetic, replay, live)", *mode)
	}
}

func runSyntheticMode(addr string, rate float64, points, tracks int) {
	log.Printf("Starting visualiser server in SYNTHETIC mode on %s", addr)
	log.Printf("Configuration: %d points, %d tracks, %.1f Hz", points, tracks, rate)

	// Create publisher
	cfg := visualiser.DefaultConfig()
	cfg.ListenAddr = addr
	cfg.SensorID = "synthetic-01"
	publisher := visualiser.NewPublisher(cfg)

	// Create gRPC server with synthetic mode
	server := visualiser.NewServer(publisher)
	server.EnableSyntheticMode("synthetic-01")

	// Configure synthetic generator
	if gen := server.SyntheticGenerator(); gen != nil {
		gen.PointCount = points
		gen.TrackCount = tracks
		gen.FrameRate = rate
	}

	// Start publisher (this starts the gRPC listener)
	if err := publisher.Start(); err != nil {
		log.Fatalf("Failed to start publisher: %v", err)
	}

	// Register the gRPC service
	pb.RegisterVisualiserServiceServer(publisher.GRPCServer(), server)

	log.Printf("Server ready, waiting for connections...")

	waitForShutdown(func() { publisher.Stop() })
}

func runReplayMode(addr, logPath string) {
	log.Printf("Starting visualiser server in REPLAY mode on %s", addr)
	log.Printf("Log file: %s", logPath)

	// Open replayer
	replayer, err := recorder.NewReplayer(logPath)
	if err != nil {
		log.Fatalf("Failed to open log: %v", err)
	}
	defer replayer.Close()

	header := replayer.Header()
	log.Printf("Log info: %d frames, %.2f seconds, sensor=%s",
		header.TotalFrames,
		float64(header.EndNs-header.StartNs)/1e9,
		header.SensorID)

	// Create publisher (for gRPC server infrastructure)
	cfg := visualiser.DefaultConfig()
	cfg.ListenAddr = addr
	cfg.SensorID = header.SensorID
	publisher := visualiser.NewPublisher(cfg)

	// Create replay server
	replayServer := visualiser.NewReplayServer(publisher, replayer)

	// Start publisher (this starts the gRPC listener)
	if err := publisher.Start(); err != nil {
		log.Fatalf("Failed to start publisher: %v", err)
	}

	// Register the gRPC service
	pb.RegisterVisualiserServiceServer(publisher.GRPCServer(), replayServer)

	log.Printf("Server ready, waiting for connections...")
	log.Printf("Connect visualiser to %s to start replay", addr)

	waitForShutdown(func() { publisher.Stop() })
}

func waitForShutdown(cleanup func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("Shutting down...")
	if cleanup != nil {
		cleanup()
	}
}
