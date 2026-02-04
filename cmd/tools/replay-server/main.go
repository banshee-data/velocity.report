// Command replay-server runs a gRPC visualiser server in replay mode.
//
// This tool loads a recorded .vrlog file and streams it via gRPC,
// allowing the macOS visualiser to connect and playback the recording.
//
// Usage:
//
//go run ./cmd/tools/replay-server [flags]
//
// Flags:
//
//-addr      Listen address (default: localhost:50051)
//-log       Path to .vrlog directory to replay
//-loop      Loop playback when reaching end (default: false)
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
addr := flag.String("addr", "localhost:50051", "Listen address")
logPath := flag.String("log", "", "Path to .vrlog directory (required)")
flag.Parse()

if *logPath == "" {
log.Fatal("Error: -log flag is required")
}

log.Printf("Starting replay server on %s", *addr)
log.Printf("Log file: %s", *logPath)

// Open replayer
replayer, err := recorder.NewReplayer(*logPath)
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
cfg.ListenAddr = *addr
cfg.SensorID = header.SensorID
publisher := visualiser.NewPublisher(cfg)

// Create replay server
replayServer := visualiser.NewReplayServer(publisher, replayer)

// Start publisher (this starts the gRPC listener)
if err := publisher.Start(); err != nil {
log.Fatalf("Failed to start publisher: %v", err)
}
defer publisher.Stop()

// Register the gRPC service
pb.RegisterVisualiserServiceServer(publisher.GRPCServer(), replayServer)

log.Printf("Server ready, waiting for connections...")
log.Printf("Connect visualiser to %s to start replay", *addr)

// Wait for shutdown signal
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
<-sigCh

log.Printf("Shutting down...")
}
