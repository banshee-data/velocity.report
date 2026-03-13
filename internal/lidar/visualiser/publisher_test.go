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

// =============================================================================
// Tests for SetBackgroundManager and background snapshot handling
// (mockBackgroundManager is defined in publisher_m35_test.go)
// =============================================================================

func TestPublisher_SetBackgroundManager(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	// Initially nil
	if pub.backgroundMgr != nil {
		t.Error("expected nil backgroundMgr initially")
	}

	// Set mock manager
	mgr := &mockBackgroundManager{sequenceNumber: 42}
	pub.SetBackgroundManager(mgr)

	if pub.backgroundMgr == nil {
		t.Fatal("expected non-nil backgroundMgr after SetBackgroundManager")
	}
}

func TestPublisher_Publish_WithBackgroundManager(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	cfg.BackgroundInterval = 24 * time.Hour // Long interval to prevent auto-send
	pub := NewPublisher(cfg)

	// Set up mock background manager
	mgr := &mockBackgroundManager{
		sequenceNumber: 1,
		snapshot: &BackgroundSnapshot{
			SequenceNumber: 1,
			TimestampNanos: time.Now().UnixNano(),
			X:              []float32{1.0, 2.0, 3.0},
			Y:              []float32{1.0, 2.0, 3.0},
			Z:              []float32{0.5, 0.5, 0.5},
			Confidence:     []uint32{1, 1, 1},
			GridMetadata: GridMetadata{
				Rings:       40,
				AzimuthBins: 1800,
			},
		},
	}
	pub.SetBackgroundManager(mgr)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Create and publish a frame
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test",
		PointCloud: &PointCloudFrame{
			X:              []float32{5.0, 6.0},
			Y:              []float32{5.0, 6.0},
			Z:              []float32{1.0, 1.0},
			Intensity:      []uint8{100, 150},
			Classification: []uint8{1, 0}, // One foreground, one background
			PointCount:     2,
		},
	}

	pub.Publish(frame)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	stats := pub.Stats()
	// Should have processed the frame (possibly 2 if background was sent first)
	if stats.FrameCount == 0 {
		t.Error("expected FrameCount > 0")
	}
}

func TestPublisher_Publish_WrongType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Publish with wrong type (string instead of *FrameBundle)
	pub.Publish("not a frame bundle")

	// Give time for processing
	time.Sleep(10 * time.Millisecond)

	stats := pub.Stats()
	// Should not have processed the invalid frame
	if stats.FrameCount != 0 {
		t.Errorf("expected FrameCount=0 for wrong type, got %d", stats.FrameCount)
	}
}

func TestPublisher_Publish_NilFrame(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Publish nil
	pub.Publish(nil)

	// Give time for processing
	time.Sleep(10 * time.Millisecond)

	stats := pub.Stats()
	// Should not have processed nil
	if stats.FrameCount != 0 {
		t.Errorf("expected FrameCount=0 for nil, got %d", stats.FrameCount)
	}
}

// NOTE: shouldSendBackground tests are in publisher_m35_test.go

func TestPublisher_SendBackgroundSnapshot_NoManager(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	// Without manager, should be no-op
	err := pub.sendBackgroundSnapshot()
	if err != nil {
		t.Errorf("expected nil error without manager, got: %v", err)
	}
}

func TestPublisher_SendBackgroundSnapshot_GenerateError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	mgr := &mockBackgroundManager{
		generateError: &testError{"generate failed"},
	}
	pub.SetBackgroundManager(mgr)

	err := pub.sendBackgroundSnapshot()
	if err == nil {
		t.Error("expected error from GenerateBackgroundSnapshot")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestPublisher_SendBackgroundSnapshot_EmptySnapshot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Empty snapshot (zero points) should still work
	mgr := &mockBackgroundManager{
		sequenceNumber: 1,
		snapshot: &BackgroundSnapshot{
			SequenceNumber: 1,
			TimestampNanos: time.Now().UnixNano(),
			X:              []float32{},
			Y:              []float32{},
			Z:              []float32{},
			Confidence:     []uint32{},
		},
	}
	pub.SetBackgroundManager(mgr)

	err := pub.sendBackgroundSnapshot()
	if err != nil {
		t.Errorf("unexpected error for empty snapshot: %v", err)
	}
}

func TestPublisher_SendBackgroundSnapshot_WrongType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	// Mock that returns wrong type
	mgr := &mockBackgroundManagerWrongType{}
	pub.SetBackgroundManager(mgr)

	err := pub.sendBackgroundSnapshot()
	if err == nil {
		t.Error("expected error for wrong type")
	}
}

type mockBackgroundManagerWrongType struct{}

func (m *mockBackgroundManagerWrongType) GenerateBackgroundSnapshot() (interface{}, error) {
	return "wrong type", nil // Returns string instead of *BackgroundSnapshot
}

func (m *mockBackgroundManagerWrongType) GetBackgroundSequenceNumber() uint64 {
	return 1
}

func TestPublisher_SendBackgroundSnapshot_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	mgr := &mockBackgroundManager{
		sequenceNumber: 42,
		snapshot: &BackgroundSnapshot{
			SequenceNumber: 42,
			TimestampNanos: time.Now().UnixNano(),
			X:              []float32{1.0, 2.0},
			Y:              []float32{1.0, 2.0},
			Z:              []float32{0.5, 0.5},
			Confidence:     []uint32{1, 1},
		},
	}
	pub.SetBackgroundManager(mgr)

	// Set a foreground timestamp so the background snapshot is not deferred.
	pub.lastForegroundTimestamp.Store(time.Now().UnixNano())

	err := pub.sendBackgroundSnapshot()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check that state was updated
	if pub.lastBackgroundSeq != 42 {
		t.Errorf("expected lastBackgroundSeq=42, got %d", pub.lastBackgroundSeq)
	}
	if pub.lastBackgroundSent.IsZero() {
		t.Error("expected lastBackgroundSent to be set")
	}
}

// =============================================================================
// Tests for Publish edge cases
// =============================================================================

func TestPublisher_Publish_WithPointCloudNoBackgroundMgr(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Frame with point cloud but no background manager
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test",
		PointCloud: &PointCloudFrame{
			X:              []float32{1.0, 2.0},
			Y:              []float32{1.0, 2.0},
			Z:              []float32{0.5, 0.5},
			Intensity:      []uint8{100, 150},
			Classification: []uint8{1, 0},
			PointCount:     2,
		},
	}

	pub.Publish(frame)

	// Give time for processing
	time.Sleep(20 * time.Millisecond)

	stats := pub.Stats()
	if stats.FrameCount != 1 {
		t.Errorf("expected FrameCount=1, got %d", stats.FrameCount)
	}
}

func TestPublisher_Publish_WithTracksAndClusters(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Frame with tracks and clusters
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test",
		Tracks: &TrackSet{
			FrameID: 1,
			Tracks: []Track{
				{TrackID: "track-1", X: 5.0, Y: 10.0},
				{TrackID: "track-2", X: 15.0, Y: 20.0},
			},
		},
		Clusters: &ClusterSet{
			FrameID: 1,
			Clusters: []Cluster{
				{ClusterID: 1, CentroidX: 5.0, CentroidY: 10.0},
			},
		},
	}

	pub.Publish(frame)

	// Give time for processing
	time.Sleep(20 * time.Millisecond)

	stats := pub.Stats()
	if stats.FrameCount != 1 {
		t.Errorf("expected FrameCount=1, got %d", stats.FrameCount)
	}
}

func TestPublisher_Publish_ChannelFull(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Fill up the channel beyond capacity (100)
	// We can't directly fill it without consuming, but we can verify the channel is used
	for i := 0; i < 100; i++ {
		frame := NewFrameBundle(uint64(i+1), "test", time.Now())
		pub.Publish(frame)
	}

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	stats := pub.Stats()
	// Should have processed most frames
	if stats.FrameCount == 0 {
		t.Error("expected FrameCount > 0")
	}
}

func TestPublisher_LogPeriodicStats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	// First call initialises the stats (lastStatsTime is zero)
	pub.logPeriodicStats(0, 100, 5, 2, 10)

	// Second call within 5 seconds should not log
	pub.logPeriodicStats(1, 100, 5, 2, 10)

	// Simulate time passage by directly manipulating lastStatsTime
	pub.lastStatsMu.Lock()
	pub.lastStatsTime = time.Now().Add(-6 * time.Second) // 6 seconds ago
	pub.lastStatsMu.Unlock()

	// Now call again - should log stats since > 5 seconds elapsed
	pub.logPeriodicStats(100, 500, 10, 5, 20)

	// Verify state was updated
	pub.lastStatsMu.Lock()
	lastTime := pub.lastStatsTime
	lastFrameCount := pub.lastFrameCount
	pub.lastStatsMu.Unlock()

	if lastTime.IsZero() {
		t.Error("expected lastStatsTime to be set")
	}
	if lastFrameCount != 100 {
		t.Errorf("expected lastFrameCount=100, got %d", lastFrameCount)
	}
}

// TestPublisher_LogPeriodicStats_DroppedTracking verifies that the periodic
// stats logger tracks dropped frame counts per interval and updates
// lastDroppedCount correctly.
func TestPublisher_LogPeriodicStats_DroppedTracking(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	// Initialise stats baseline
	pub.logPeriodicStats(0, 100, 5, 2, 10)

	// Simulate some dropped frames
	pub.droppedFrames.Store(20)

	// Advance time past the logging interval
	pub.lastStatsMu.Lock()
	pub.lastStatsTime = time.Now().Add(-6 * time.Second)
	pub.lastStatsMu.Unlock()

	// Log stats — should record the dropped count
	pub.logPeriodicStats(100, 500, 10, 5, 20)

	pub.lastStatsMu.Lock()
	lastDropped := pub.lastDroppedCount
	pub.lastStatsMu.Unlock()

	if lastDropped != 20 {
		t.Errorf("expected lastDroppedCount=20, got %d", lastDropped)
	}
}

// TestPublisher_VRLogReplay_PlaybackInfoPreserved verifies that the VRLOG
// replay loop preserves PlaybackInfo fields (LogStartNs, LogEndNs,
// TotalFrames, CurrentFrameIndex, Seekable) set by the FrameReader.
func TestPublisher_VRLogReplay_PlaybackInfoPreserved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	if err := pub.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer pub.Stop()

	// Build frames with PlaybackInfo pre-populated (simulating a recorder replayer)
	startNs := int64(1_700_000_000_000_000_000)
	endNs := int64(1_700_000_000_500_000_000)
	frames := make([]*FrameBundle, 3)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i),
			TimestampNanos: startNs + int64(i)*100_000_000,
			PlaybackInfo: &PlaybackInfo{
				IsLive:            false,
				LogStartNs:        startNs,
				LogEndNs:          endNs,
				PlaybackRate:      1.0,
				CurrentFrameIndex: uint64(i),
				TotalFrames:       3,
				Seekable:          true,
			},
		}
	}

	reader := newMockFrameReader(frames)

	// Add a client BEFORE starting replay so it receives the published frames
	req := &pb.StreamRequest{}
	client := pub.addClient("test-client-playbackinfo", req)
	defer pub.removeClient("test-client-playbackinfo")

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay() error = %v", err)
	}
	defer pub.StopVRLogReplay()

	var received []*FrameBundle
	timeout := time.After(5 * time.Second)
	for len(received) < 3 {
		select {
		case frame := <-client.frameCh:
			received = append(received, frame)
		case <-timeout:
			t.Fatalf("timed out waiting for frames (got %d/3)", len(received))
		}
	}

	for i, frame := range received {
		pi := frame.PlaybackInfo
		if pi == nil {
			t.Fatalf("frame %d PlaybackInfo is nil", i)
		}
		if pi.LogStartNs != startNs {
			t.Errorf("frame %d LogStartNs = %d, want %d", i, pi.LogStartNs, startNs)
		}
		if pi.LogEndNs != endNs {
			t.Errorf("frame %d LogEndNs = %d, want %d", i, pi.LogEndNs, endNs)
		}
		if pi.TotalFrames != 3 {
			t.Errorf("frame %d TotalFrames = %d, want 3", i, pi.TotalFrames)
		}
		if pi.IsLive {
			t.Errorf("frame %d IsLive = true, want false", i)
		}
		if !pi.Seekable {
			t.Errorf("frame %d Seekable = false, want true", i)
		}
	}
}

// TestPublisher_DrainFrameBuffers verifies that drainFrameBuffers clears
// both the central frameChan and per-client channels.
func TestPublisher_DrainFrameBuffers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Enqueue frames into the central frameChan
	for i := 0; i < 3; i++ {
		select {
		case pub.frameChan <- &FrameBundle{FrameID: uint64(i)}:
		default:
			t.Fatalf("failed to enqueue frame %d", i)
		}
	}

	if len(pub.frameChan) != 3 {
		t.Fatalf("expected 3 buffered frames, got %d", len(pub.frameChan))
	}

	pub.drainFrameBuffers()

	if len(pub.frameChan) != 0 {
		t.Errorf("expected 0 buffered frames after drain, got %d", len(pub.frameChan))
	}
}

// TestPublisher_DrainFrameBuffersReleasesPointClouds verifies that
// drainFrameBuffers releases point cloud references on discarded frames.
func TestPublisher_DrainFrameBuffersReleasesPointClouds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	pc := &PointCloudFrame{PointCount: 10, X: make([]float32, 10)}
	pc.Retain() // refCount=1 from Retain; drain will Release once
	frame := &FrameBundle{FrameID: 1, PointCloud: pc}

	select {
	case pub.frameChan <- frame:
	default:
		t.Fatal("failed to enqueue frame")
	}

	pub.drainFrameBuffers()

	// After drain, the Retain is the only remaining reference.
	// Release it — this should not panic (proving drain called Release).
	pc.Release()
}

// TestPublisher_DrainClientCh verifies that drainClientCh clears a
// single client's frame channel.
func TestPublisher_DrainClientCh(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	client := &clientStream{
		id:      "test-client",
		frameCh: make(chan *FrameBundle, 10),
	}

	// Fill the client channel
	for i := 0; i < 5; i++ {
		client.frameCh <- &FrameBundle{FrameID: uint64(i)}
	}

	if len(client.frameCh) != 5 {
		t.Fatalf("expected 5 buffered frames, got %d", len(client.frameCh))
	}

	pub.drainClientCh(client)

	if len(client.frameCh) != 0 {
		t.Errorf("expected 0 buffered frames after drain, got %d", len(client.frameCh))
	}
}

// TestPublisher_DrainClientChReleasesPointClouds verifies that drainClientCh
// releases point cloud references on discarded frames.
func TestPublisher_DrainClientChReleasesPointClouds(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	pc := &PointCloudFrame{PointCount: 10, X: make([]float32, 10)}
	pc.Retain() // refCount=1 from Retain; drain will Release once

	client := &clientStream{
		id:      "test-client",
		frameCh: make(chan *FrameBundle, 10),
	}
	client.frameCh <- &FrameBundle{FrameID: 1, PointCloud: pc}

	pub.drainClientCh(client)

	// Should not panic — drain released the first ref
	pc.Release()
}

// TestPublisher_DrainFrameBuffersWithClients verifies that drainFrameBuffers
// also drains per-client channels.
func TestPublisher_DrainFrameBuffersWithClients(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Manually add a client
	client := &clientStream{
		id:      "test-client",
		frameCh: make(chan *FrameBundle, 10),
		doneCh:  make(chan struct{}),
	}
	pub.clientsMu.Lock()
	pub.clients[client.id] = client
	pub.clientsMu.Unlock()

	// Fill client channel
	for i := 0; i < 3; i++ {
		client.frameCh <- &FrameBundle{FrameID: uint64(i)}
	}

	pub.drainFrameBuffers()

	if len(client.frameCh) != 0 {
		t.Errorf("expected client channel drained, got %d", len(client.frameCh))
	}
}

// TestPublisher_ForegroundTimestampTracking verifies that Publish tracks
// the most recent foreground frame timestamp and that background snapshots
// inherit this timestamp.
func TestPublisher_ForegroundTimestampTracking(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Publish a foreground frame
	pub.Publish(&FrameBundle{
		FrameID:        1,
		TimestampNanos: 5000000000,
		SensorID:       "test",
		FrameType:      FrameTypeForeground,
	})

	if pub.lastForegroundTimestamp.Load() != 5000000000 {
		t.Errorf("lastForegroundTimestamp = %d, want 5000000000", pub.lastForegroundTimestamp.Load())
	}

	// Publish a background frame — should NOT update the tracker
	pub.Publish(&FrameBundle{
		FrameID:        2,
		TimestampNanos: 9999999999,
		SensorID:       "test",
		FrameType:      FrameTypeBackground,
		Background:     &BackgroundSnapshot{TimestampNanos: 9999999999},
	})

	if pub.lastForegroundTimestamp.Load() != 5000000000 {
		t.Errorf("lastForegroundTimestamp changed after background frame: %d, want 5000000000",
			pub.lastForegroundTimestamp.Load())
	}
}

// TestPublisher_BackgroundTimestampInheritance verifies that
// sendBackgroundSnapshot uses the foreground timestamp when available.
func TestPublisher_BackgroundTimestampInheritance(t *testing.T) {
	cfg := DefaultConfig()
	pub := NewPublisher(cfg)

	mgr := &mockBackgroundManager{
		snapshot: &BackgroundSnapshot{
			TimestampNanos: 99999, // Wall-clock timestamp
			SequenceNumber: 1,
		},
		sequenceNumber: 1,
	}
	pub.SetBackgroundManager(mgr)

	// Don't call Start — the broadcastLoop would consume frameChan
	// before we can inspect it.  sendBackgroundSnapshot only needs
	// the channel to be present (which NewPublisher creates).

	// Set a foreground timestamp
	pub.lastForegroundTimestamp.Store(5000000000)

	err := pub.sendBackgroundSnapshot()
	if err != nil {
		t.Fatalf("sendBackgroundSnapshot failed: %v", err)
	}

	// The frame should have been sent to frameChan with the foreground timestamp
	select {
	case frame := <-pub.frameChan:
		if frame.TimestampNanos != 5000000000 {
			t.Errorf("background frame timestamp = %d, want 5000000000 (inherited from foreground)",
				frame.TimestampNanos)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for background frame")
	}
}

// TestPublisher_SendBackgroundSnapshot_DeferredBeforeForeground verifies that
// background snapshots are silently deferred when no foreground frame has
// been published.  This prevents wall-clock timestamps from contaminating
// VRLOG recordings of PCAP replays.
func TestPublisher_SendBackgroundSnapshot_DeferredBeforeForeground(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	mgr := &mockBackgroundManager{
		sequenceNumber: 1,
		snapshot: &BackgroundSnapshot{
			SequenceNumber: 1,
			TimestampNanos: time.Now().UnixNano(),
			X:              []float32{1.0},
			Y:              []float32{1.0},
			Z:              []float32{0.5},
			Confidence:     []uint32{1},
		},
	}
	pub.SetBackgroundManager(mgr)

	// Do NOT set lastForegroundTimestamp — background should be deferred.
	err := pub.sendBackgroundSnapshot()
	if err != nil {
		t.Fatalf("sendBackgroundSnapshot returned unexpected error: %v", err)
	}

	// frameChan should be empty — snapshot was deferred, not sent.
	select {
	case <-pub.frameChan:
		t.Error("background snapshot should have been deferred, but a frame was sent")
	default:
		// expected: nothing in the channel
	}

	if pub.lastBackgroundSeq != 0 {
		t.Errorf("lastBackgroundSeq = %d, want 0 (deferred)", pub.lastBackgroundSeq)
	}
}
