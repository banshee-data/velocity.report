// Command visualiser-server runs a synthetic gRPC visualiser server.
//
// This is useful for testing the macOS visualiser without a real LiDAR sensor.
// It generates synthetic point clouds with 10 tracks moving in circles.
//
// Usage:
//
//	go run ./cmd/tools/visualiser-server [flags]
//
// Flags:
//
//	-addr    Listen address (default: localhost:50051)
//	-rate    Frame rate in Hz (default: 10)
//	-points  Points per frame (default: 10000)
//	-tracks  Number of synthetic tracks (default: 10)
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "Listen address")
	rate := flag.Float64("rate", 10, "Frame rate in Hz")
	points := flag.Int("points", 10000, "Points per frame")
	tracks := flag.Int("tracks", 10, "Number of synthetic tracks")
	flag.Parse()

	log.Printf("Starting synthetic visualiser server on %s", *addr)
	log.Printf("Configuration: %d points, %d tracks, %.1f Hz", *points, *tracks, *rate)

	// Create publisher
	cfg := visualiser.DefaultConfig()
	cfg.ListenAddr = *addr
	cfg.SensorID = "synthetic-01"
	publisher := visualiser.NewPublisher(cfg)

	// Create gRPC server with synthetic mode
	server := visualiser.NewServer(publisher)
	server.EnableSyntheticMode("synthetic-01")

	// Configure synthetic generator
	if gen := server.SyntheticGenerator(); gen != nil {
		gen.PointCount = *points
		gen.TrackCount = *tracks
		gen.FrameRate = *rate
	}

	// Start publisher (this starts the gRPC listener)
	if err := publisher.Start(); err != nil {
		log.Fatalf("Failed to start publisher: %v", err)
	}

	// Register the gRPC service
	pb.RegisterVisualiserServiceServer(publisher.GRPCServer(), server)

	log.Printf("Server ready, waiting for connections...")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("Shutting down...")
	publisher.Stop()
}
