// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

import (
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ListenAddr != "localhost:50051" {
		t.Errorf("expected ListenAddr=localhost:50051, got %s", cfg.ListenAddr)
	}
	if cfg.SensorID != "hesai-01" {
		t.Errorf("expected SensorID=hesai-01, got %s", cfg.SensorID)
	}
	if cfg.EnableDebug {
		t.Error("expected EnableDebug=false")
	}
	if cfg.MaxClients != 5 {
		t.Errorf("expected MaxClients=5, got %d", cfg.MaxClients)
	}
}

func TestNewPublisher(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	if pub == nil {
		t.Fatal("expected non-nil Publisher")
	}
	if pub.config.ListenAddr != cfg.ListenAddr {
		t.Errorf("expected ListenAddr=%s, got %s", cfg.ListenAddr, pub.config.ListenAddr)
	}
	if pub.frameChan == nil {
		t.Error("expected non-nil frameChan")
	}
	if pub.clients == nil {
		t.Error("expected non-nil clients map")
	}
	if pub.stopCh == nil {
		t.Error("expected non-nil stopCh")
	}
}

func TestPublisher_Stats_NotRunning(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	stats := pub.Stats()

	if stats.Running {
		t.Error("expected Running=false before Start")
	}
	if stats.FrameCount != 0 {
		t.Errorf("expected FrameCount=0, got %d", stats.FrameCount)
	}
	if stats.ClientCount != 0 {
		t.Errorf("expected ClientCount=0, got %d", stats.ClientCount)
	}
}

func TestPublisher_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0" // Use random available port
	pub := NewPublisher(cfg)

	// Start
	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	stats := pub.Stats()
	if !stats.Running {
		t.Error("expected Running=true after Start")
	}

	// Start again should fail
	if err := pub.Start(); err == nil {
		t.Error("expected error when starting already running publisher")
	}

	// Stop
	pub.Stop()

	stats = pub.Stats()
	if stats.Running {
		t.Error("expected Running=false after Stop")
	}

	// Stop again should be safe
	pub.Stop()
}

func TestPublisher_Publish_NotRunning(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	frame := NewFrameBundle(1, "test", time.Now())

	// Publish should be safe even when not running
	pub.Publish(frame)

	stats := pub.Stats()
	if stats.FrameCount != 0 {
		t.Errorf("expected FrameCount=0 when not running, got %d", stats.FrameCount)
	}
}

func TestPublisher_Publish_Running(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frame := NewFrameBundle(1, "test", time.Now())
	pub.Publish(frame)

	// Give the broadcast loop time to process
	time.Sleep(10 * time.Millisecond)

	stats := pub.Stats()
	if stats.FrameCount != 1 {
		t.Errorf("expected FrameCount=1, got %d", stats.FrameCount)
	}
}

func TestPublisher_AddRemoveClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	req := &pb.StreamRequest{
		SensorId:        "test",
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
		IncludeDebug:    false,
	}

	// Add client
	client := pub.addClient("client-1", req)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.id != "client-1" {
		t.Errorf("expected id=client-1, got %s", client.id)
	}

	stats := pub.Stats()
	if stats.ClientCount != 1 {
		t.Errorf("expected ClientCount=1, got %d", stats.ClientCount)
	}

	// Remove client
	pub.removeClient("client-1")

	stats = pub.Stats()
	if stats.ClientCount != 0 {
		t.Errorf("expected ClientCount=0 after remove, got %d", stats.ClientCount)
	}
}

func TestPublisher_MultipleClients(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	req := &pb.StreamRequest{SensorId: "test"}

	// Add multiple clients
	pub.addClient("client-1", req)
	pub.addClient("client-2", req)
	pub.addClient("client-3", req)

	stats := pub.Stats()
	if stats.ClientCount != 3 {
		t.Errorf("expected ClientCount=3, got %d", stats.ClientCount)
	}

	// Remove one
	pub.removeClient("client-2")

	stats = pub.Stats()
	if stats.ClientCount != 2 {
		t.Errorf("expected ClientCount=2, got %d", stats.ClientCount)
	}

	// Remove non-existent client should be safe
	pub.removeClient("client-99")

	stats = pub.Stats()
	if stats.ClientCount != 2 {
		t.Errorf("expected ClientCount=2, got %d", stats.ClientCount)
	}
}

func TestPublisher_BroadcastToClients(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	req := &pb.StreamRequest{SensorId: "test"}
	client := pub.addClient("client-1", req)

	// Publish a frame
	frame := NewFrameBundle(1, "test", time.Now())
	pub.Publish(frame)

	// Client should receive the frame
	select {
	case received := <-client.frameCh:
		if received.FrameID != 1 {
			t.Errorf("expected FrameID=1, got %d", received.FrameID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for frame")
	}
}

func TestPublisher_FrameDropOnSlowClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	req := &pb.StreamRequest{SensorId: "test"}
	client := pub.addClient("client-1", req)

	// Fill up client's buffer (10 frames)
	for i := 0; i < 15; i++ {
		frame := NewFrameBundle(uint64(i+1), "test", time.Now())
		pub.Publish(frame)
		time.Sleep(1 * time.Millisecond)
	}

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Drain client buffer
	count := 0
	for {
		select {
		case <-client.frameCh:
			count++
		default:
			goto done
		}
	}
done:

	// Should have received up to buffer size (10)
	if count > 10 {
		t.Errorf("expected at most 10 frames (buffer size), got %d", count)
	}
}

func TestPublisher_ConcurrentPublish(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	framesPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < framesPerGoroutine; j++ {
				frame := NewFrameBundle(uint64(id*100+j), "test", time.Now())
				pub.Publish(frame)
			}
		}(i)
	}

	wg.Wait()

	// Give broadcast loop time to process
	time.Sleep(50 * time.Millisecond)

	stats := pub.Stats()
	expectedFrames := uint64(numGoroutines * framesPerGoroutine)
	if stats.FrameCount != expectedFrames {
		t.Errorf("expected FrameCount=%d, got %d", expectedFrames, stats.FrameCount)
	}
}

func TestStreamRequest_Fields(t *testing.T) {
	req := &pb.StreamRequest{
		SensorId:        "hesai-01",
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
		IncludeDebug:    true,
		PointDecimation: 2, // DecimationVoxel
		DecimationRatio: 0.5,
	}

	if req.SensorId != "hesai-01" {
		t.Errorf("expected SensorID=hesai-01, got %s", req.SensorId)
	}
	if !req.IncludePoints {
		t.Error("expected IncludePoints=true")
	}
	if !req.IncludeClusters {
		t.Error("expected IncludeClusters=true")
	}
	if !req.IncludeTracks {
		t.Error("expected IncludeTracks=true")
	}
	if !req.IncludeDebug {
		t.Error("expected IncludeDebug=true")
	}
	if req.PointDecimation != 2 {
		t.Errorf("expected PointDecimation=2, got %d", req.PointDecimation)
	}
	if req.DecimationRatio != 0.5 {
		t.Errorf("expected DecimationRatio=0.5, got %f", req.DecimationRatio)
	}
}

func TestPublisherStats_Fields(t *testing.T) {
	stats := PublisherStats{
		FrameCount:  100,
		ClientCount: 5,
		Running:     true,
	}

	if stats.FrameCount != 100 {
		t.Errorf("expected FrameCount=100, got %d", stats.FrameCount)
	}
	if stats.ClientCount != 5 {
		t.Errorf("expected ClientCount=5, got %d", stats.ClientCount)
	}
	if !stats.Running {
		t.Error("expected Running=true")
	}
}

func TestServer_SyntheticMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	server := NewServer(pub)

	// Initially not in synthetic mode
	if server.SyntheticGenerator() != nil {
		t.Error("expected nil generator before synthetic mode enabled")
	}

	// Enable synthetic mode
	server.EnableSyntheticMode("test-sensor")

	gen := server.SyntheticGenerator()
	if gen == nil {
		t.Fatal("expected non-nil generator after synthetic mode enabled")
	}

	// Configure and generate a frame
	gen.PointCount = 100
	gen.TrackCount = 3
	frame := gen.NextFrame()

	if frame == nil {
		t.Fatal("expected non-nil frame")
	}
	if frame.PointCloud == nil {
		t.Error("expected non-nil point cloud")
	}
	if frame.Tracks == nil {
		t.Error("expected non-nil tracks")
	}
}
