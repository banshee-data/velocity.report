// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file contains tests for VRLOG recording and replay.
package visualiser

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
)

// mockRecorder implements FrameRecorder for testing.
type mockRecorder struct {
	frames []*FrameBundle
	mu     sync.Mutex
	closed bool
}

func newMockRecorder() *mockRecorder {
	return &mockRecorder{
		frames: make([]*FrameBundle, 0),
	}
}

func (m *mockRecorder) Record(frame *FrameBundle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return io.EOF
	}
	m.frames = append(m.frames, frame)
	return nil
}

func (m *mockRecorder) Frames() []*FrameBundle {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy to prevent race conditions with concurrent Record() calls
	copyFrames := make([]*FrameBundle, len(m.frames))
	copy(copyFrames, m.frames)
	return copyFrames
}

// TestPublisher_SetRecorder tests the FrameRecorder interface setup.
func TestPublisher_SetRecorder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	rec := newMockRecorder()

	// Set recorder
	pub.SetRecorder(rec)

	// Verify recorder was set (indirectly through recording behavior)
	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Publish a frame and verify it was recorded
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
	}

	pub.Publish(frame)

	// Give some time for the frame to be processed
	time.Sleep(100 * time.Millisecond)

	frames := rec.Frames()
	if len(frames) != 1 {
		t.Errorf("expected 1 recorded frame, got %d", len(frames))
	}
}

// TestPublisher_ClearRecorder tests removing the recorder.
func TestPublisher_ClearRecorder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	rec := newMockRecorder()
	pub.SetRecorder(rec)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Publish a frame with recorder
	frame1 := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
	}
	pub.Publish(frame1)
	time.Sleep(50 * time.Millisecond)

	// Clear recorder
	pub.ClearRecorder()

	// Publish another frame
	frame2 := &FrameBundle{
		FrameID:        2,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
	}
	pub.Publish(frame2)
	time.Sleep(50 * time.Millisecond)

	// Only the first frame should be recorded
	frames := rec.Frames()
	if len(frames) != 1 {
		t.Errorf("expected 1 recorded frame after clearing, got %d", len(frames))
	}
}

// TestPublisher_VRLogReplay_StartStop tests VRLOG replay lifecycle.
func TestPublisher_VRLogReplay_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Create mock reader with a few frames
	frames := make([]*FrameBundle, 3)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	// Initially not active
	if pub.IsVRLogActive() {
		t.Error("expected VRLOG not active initially")
	}

	// Start replay
	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}

	if !pub.IsVRLogActive() {
		t.Error("expected VRLOG active after start")
	}

	// Starting again should fail
	if err := pub.StartVRLogReplay(reader); err == nil {
		t.Error("expected error when starting already active replay")
	}

	// Stop replay
	pub.StopVRLogReplay()

	if pub.IsVRLogActive() {
		t.Error("expected VRLOG not active after stop")
	}

	// Stop again should be safe
	pub.StopVRLogReplay()
}

// TestPublisher_VRLogReplay_PauseResume tests VRLOG pause/resume.
func TestPublisher_VRLogReplay_PauseResume(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frames := make([]*FrameBundle, 10)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().Add(time.Duration(i) * 100 * time.Millisecond).UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// Pause
	pub.SetVRLogPaused(true)

	// Verify reader is paused
	if !reader.paused {
		t.Error("expected reader to be paused")
	}

	// Resume
	pub.SetVRLogPaused(false)

	if reader.paused {
		t.Error("expected reader to not be paused")
	}
}

// TestPublisher_VRLogReplay_SetRate tests VRLOG rate control.
func TestPublisher_VRLogReplay_SetRate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frames := make([]*FrameBundle, 10)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().Add(time.Duration(i) * 100 * time.Millisecond).UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// Set rate
	pub.SetVRLogRate(2.0)

	if reader.rate != 2.0 {
		t.Errorf("expected rate 2.0, got %v", reader.rate)
	}
}

// TestPublisher_VRLogReplay_Seek tests VRLOG seek.
func TestPublisher_VRLogReplay_Seek(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frames := make([]*FrameBundle, 10)
	baseTime := time.Now().UnixNano()
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: baseTime + int64(i)*int64(time.Second),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// Seek by frame index
	if _, err := pub.SeekVRLog(5); err != nil {
		t.Errorf("SeekVRLog failed: %v", err)
	}

	if reader.CurrentFrame() != 5 {
		t.Errorf("expected frame 5, got %d", reader.CurrentFrame())
	}

	// Seek by timestamp
	targetTime := baseTime + int64(3)*int64(time.Second)
	if _, err := pub.SeekVRLogTimestamp(targetTime); err != nil {
		t.Errorf("SeekVRLogTimestamp failed: %v", err)
	}

	if reader.CurrentFrame() != 3 {
		t.Errorf("expected frame 3, got %d", reader.CurrentFrame())
	}
}

// TestPublisher_VRLogReplay_SeekErrors tests VRLOG seek error cases.
func TestPublisher_VRLogReplay_SeekErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Seek without active replay should fail
	if _, err := pub.SeekVRLog(5); err == nil {
		t.Error("expected error when seeking without active replay")
	}

	if _, err := pub.SeekVRLogTimestamp(123456); err == nil {
		t.Error("expected error when seeking by timestamp without active replay")
	}
}

// TestPublisher_VRLogReplay_SeekWhilePaused verifies that seeking while paused
// delivers exactly one frame to connected subscribers (sendOneFrame semantics).
func TestPublisher_VRLogReplay_SeekWhilePaused(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	baseTime := time.Now().UnixNano()
	frames := make([]*FrameBundle, 10)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: baseTime + int64(i)*int64(time.Second),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// Wait for the replay loop to consume at least one frame before pausing.
	deadline := time.Now().Add(2 * time.Second)
	for reader.CurrentFrame() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for replay loop to start consuming frames")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Pause playback and wait for the loop to observe the pause by confirming
	// the frame pointer stops advancing.
	pub.SetVRLogPaused(true)
	time.Sleep(50 * time.Millisecond)
	snapshot := reader.CurrentFrame()
	time.Sleep(100 * time.Millisecond)
	if reader.CurrentFrame() != snapshot {
		// Loop hasn't settled into paused state yet; give it one more chance.
		time.Sleep(100 * time.Millisecond)
	}

	// Seek to frame 7 while paused
	currentFrame, err := pub.SeekVRLog(7)
	if err != nil {
		t.Fatalf("SeekVRLog failed: %v", err)
	}
	if currentFrame != 7 {
		t.Errorf("expected currentFrame=7, got %d", currentFrame)
	}

	// The replay loop should deliver one frame despite being paused.
	// Poll until the reader advances past the seeked frame, with a timeout.
	deadline = time.Now().Add(2 * time.Second)
	for reader.CurrentFrame() != 8 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for reader to advance to frame 8, got %d", reader.CurrentFrame())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestPublisher_VRLogReader tests VRLogReader accessor.
func TestPublisher_VRLogReader(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Initially nil
	if pub.VRLogReader() != nil {
		t.Error("expected nil reader initially")
	}

	frames := make([]*FrameBundle, 3)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}

	// Should return the reader
	if pub.VRLogReader() != reader {
		t.Error("expected to get the reader back")
	}

	pub.StopVRLogReplay()

	// Should be nil after stop
	if pub.VRLogReader() != nil {
		t.Error("expected nil reader after stop")
	}
}

// TestServer_SetVRLogMode tests the gRPC server VRLOG mode setting.
func TestServer_SetVRLogMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	// Initially not in VRLOG mode
	server.playbackMu.RLock()
	isVRLog := server.vrlogMode
	isReplay := server.replayMode
	server.playbackMu.RUnlock()

	if isVRLog {
		t.Error("expected vrlogMode=false initially")
	}
	if isReplay {
		t.Error("expected replayMode=false initially")
	}

	// Enable VRLOG mode
	server.SetVRLogMode(true)

	server.playbackMu.RLock()
	isVRLog = server.vrlogMode
	isReplay = server.replayMode
	server.playbackMu.RUnlock()

	if !isVRLog {
		t.Error("expected vrlogMode=true after SetVRLogMode(true)")
	}
	if !isReplay {
		t.Error("expected replayMode=true after SetVRLogMode(true)")
	}

	// Disable VRLOG mode
	server.SetVRLogMode(false)

	server.playbackMu.RLock()
	isVRLog = server.vrlogMode
	server.playbackMu.RUnlock()

	if isVRLog {
		t.Error("expected vrlogMode=false after SetVRLogMode(false)")
	}
}

// TestServer_Pause_VRLogMode tests pause delegation in VRLOG mode.
func TestServer_Pause_VRLogMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frames := make([]*FrameBundle, 10)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// Enable VRLOG mode on server
	server.SetVRLogMode(true)

	// Pause via gRPC
	resp, err := server.Pause(context.Background(), &pb.PauseRequest{})
	if err != nil {
		t.Fatalf("Pause failed: %v", err)
	}

	if !resp.Paused {
		t.Error("expected Paused=true in response")
	}

	// Verify reader was paused
	if !reader.paused {
		t.Error("expected reader to be paused via gRPC delegation")
	}
}

// TestServer_Play_VRLogMode tests play delegation in VRLOG mode.
func TestServer_Play_VRLogMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frames := make([]*FrameBundle, 10)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)
	reader.paused = true // Start paused

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// Enable VRLOG mode on server
	server.SetVRLogMode(true)

	// Play via gRPC
	resp, err := server.Play(context.Background(), &pb.PlayRequest{})
	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	if resp.Paused {
		t.Error("expected Paused=false in response")
	}

	// Verify reader was unpaused
	if reader.paused {
		t.Error("expected reader to be unpaused via gRPC delegation")
	}
}

// TestServer_SetRate_VRLogMode tests rate delegation in VRLOG mode.
func TestServer_SetRate_VRLogMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frames := make([]*FrameBundle, 10)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// Enable VRLOG mode on server
	server.SetVRLogMode(true)

	// Set rate via gRPC
	resp, err := server.SetRate(context.Background(), &pb.SetRateRequest{Rate: 2.5})
	if err != nil {
		t.Fatalf("SetRate failed: %v", err)
	}

	if resp.Rate != 2.5 {
		t.Errorf("expected rate 2.5, got %v", resp.Rate)
	}

	// Verify reader rate was set
	if reader.rate != 2.5 {
		t.Errorf("expected reader rate 2.5, got %v", reader.rate)
	}
}

// TestServer_Seek_VRLogMode tests seek delegation in VRLOG mode.
func TestServer_Seek_VRLogMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frames := make([]*FrameBundle, 10)
	baseTime := time.Now().UnixNano()
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: baseTime + int64(i)*int64(time.Second),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// Enable VRLOG mode on server
	server.SetVRLogMode(true)

	// Seek by frame ID via gRPC
	resp, err := server.Seek(context.Background(), &pb.SeekRequest{
		Target: &pb.SeekRequest_FrameId{FrameId: 5},
	})
	if err != nil {
		t.Fatalf("Seek failed: %v", err)
	}

	if resp.CurrentFrameId != 5 {
		t.Errorf("expected CurrentFrameId 5, got %d", resp.CurrentFrameId)
	}

	// Seek by timestamp via gRPC
	targetTime := baseTime + int64(3)*int64(time.Second)
	resp, err = server.Seek(context.Background(), &pb.SeekRequest{
		Target: &pb.SeekRequest_TimestampNs{TimestampNs: targetTime},
	})
	if err != nil {
		t.Fatalf("Seek by timestamp failed: %v", err)
	}

	if resp.CurrentFrameId != 3 {
		t.Errorf("expected CurrentFrameId 3, got %d", resp.CurrentFrameId)
	}
}

// TestServer_Seek_NoVRLogMode tests seek returns error when not in VRLOG mode.
func TestServer_Seek_NoVRLogMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	// Seek without VRLOG mode should return Unimplemented
	_, err := server.Seek(context.Background(), &pb.SeekRequest{
		Target: &pb.SeekRequest_FrameId{FrameId: 5},
	})
	if err == nil {
		t.Error("expected error when seeking without VRLOG mode")
	}
}

// TestServer_Seek_NoTarget tests seek returns error when no target specified.
func TestServer_Seek_NoTarget(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	frames := make([]*FrameBundle, 10)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	server.SetVRLogMode(true)

	// Seek without target
	_, err := server.Seek(context.Background(), &pb.SeekRequest{})
	if err == nil {
		t.Error("expected error when seeking without target")
	}
}

// TestServer_GetCapabilities_VRLog tests capabilities response includes replay/recording.
func TestServer_GetCapabilities_VRLog(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)
	server := NewServer(pub)

	resp, err := server.GetCapabilities(context.Background(), &pb.CapabilitiesRequest{})
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	if !resp.SupportsReplay {
		t.Error("expected SupportsReplay=true")
	}
	if !resp.SupportsRecording {
		t.Error("expected SupportsRecording=true")
	}
	if !resp.SupportsPoints {
		t.Error("expected SupportsPoints=true")
	}
	if !resp.SupportsClusters {
		t.Error("expected SupportsClusters=true")
	}
	if !resp.SupportsTracks {
		t.Error("expected SupportsTracks=true")
	}
}

// TestPublisher_ShouldSendBackground_VRLogMode tests background suppression during VRLOG replay.
func TestPublisher_ShouldSendBackground_VRLogMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddr = "localhost:0"
	pub := NewPublisher(cfg)

	if err := pub.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pub.Stop()

	// Set up a mock background manager
	pub.SetBackgroundManager(&testBackgroundManager{})

	// Initially, should send background (when not in VRLOG mode)
	// Note: shouldSendBackground also checks other conditions, so this tests
	// that VRLOG mode bypasses those checks
	frames := make([]*FrameBundle, 3)
	for i := range frames {
		frames[i] = &FrameBundle{
			FrameID:        uint64(i + 1),
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "test-sensor",
		}
	}
	reader := newMockFrameReader(frames)

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	defer pub.StopVRLogReplay()

	// In VRLOG mode, shouldSendBackground should return false
	if pub.shouldSendBackground() {
		t.Error("expected shouldSendBackground=false during VRLOG replay")
	}
}

// testBackgroundManager implements BackgroundManagerInterface for testing.
type testBackgroundManager struct{}

func (m *testBackgroundManager) GenerateBackgroundSnapshot() (interface{}, error) {
	return &BackgroundSnapshot{
		SequenceNumber: 1,
		TimestampNanos: time.Now().UnixNano(),
	}, nil
}

func (m *testBackgroundManager) GetBackgroundSequenceNumber() uint64 {
	return 1
}
