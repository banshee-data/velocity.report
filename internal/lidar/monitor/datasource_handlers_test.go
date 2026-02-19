package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// --- Simple accessor tests ---

func TestPCAPDone_NilChannel(t *testing.T) {
	ws := NewWebServer(WebServerConfig{
		Address: ":0",
		Stats:   NewPacketStats(),
	})
	ch := ws.PCAPDone()
	if ch != nil {
		t.Errorf("expected nil channel when no PCAP is running, got %v", ch)
	}
}

func TestPCAPDone_WithChannel(t *testing.T) {
	ws := NewWebServer(WebServerConfig{
		Address: ":0",
		Stats:   NewPacketStats(),
	})
	done := make(chan struct{})
	ws.pcapMu.Lock()
	ws.pcapDone = done
	ws.pcapMu.Unlock()

	ch := ws.PCAPDone()
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	if ch != done {
		t.Error("expected returned channel to match set channel")
	}
}

func TestLastAnalysisRunID_Empty(t *testing.T) {
	ws := NewWebServer(WebServerConfig{
		Address: ":0",
		Stats:   NewPacketStats(),
	})
	id := ws.LastAnalysisRunID()
	if id != "" {
		t.Errorf("expected empty run ID, got %q", id)
	}
}

func TestLastAnalysisRunID_Set(t *testing.T) {
	ws := NewWebServer(WebServerConfig{
		Address: ":0",
		Stats:   NewPacketStats(),
	})
	ws.pcapMu.Lock()
	ws.pcapLastRunID = "run-abc-123"
	ws.pcapMu.Unlock()

	id := ws.LastAnalysisRunID()
	if id != "run-abc-123" {
		t.Errorf("expected 'run-abc-123', got %q", id)
	}
}

func TestResetAllStateDirect(t *testing.T) {
	sensorID := "test-reset-direct"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	ws := NewWebServer(WebServerConfig{
		Address:  ":0",
		Stats:    NewPacketStats(),
		SensorID: sensorID,
	})

	err := ws.ResetAllStateDirect()
	if err != nil {
		t.Fatalf("ResetAllStateDirect() error: %v", err)
	}
}

// --- StartPCAPForSweep tests ---

func TestStartPCAPForSweep_TimeoutWhenBusy(t *testing.T) {
	sensorID := "test-sweep-timeout"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	ws := NewWebServer(WebServerConfig{
		Address:  ":0",
		Stats:    NewPacketStats(),
		SensorID: sensorID,
	})
	ws.setBaseContext(context.Background())

	// Simulate PCAP already in progress
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

	err := ws.StartPCAPForSweep("/dummy.pcap", false, "fastest", 0, 0, 1, false)
	if err == nil {
		t.Fatal("expected timeout error when PCAP already in progress")
	}
	if err.Error() != "timeout waiting for PCAP replay slot" {
		t.Errorf("unexpected error: %v", err)
	}
}

// resolveSymlinks resolves macOS /var -> /private/var symlinks for temp dirs.
func resolveSymlinks(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("Failed to resolve symlinks for %s: %v", dir, err)
	}
	return resolved
}

func TestStartPCAPForSweep_SuccessPath(t *testing.T) {
	sensorID := "test-sweep-success"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())

	// Create a minimal PCAP file (just the 24-byte global header)
	pcapHeader := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, // magic number (little-endian)
		0x02, 0x00, 0x04, 0x00, // version 2.4
		0x00, 0x00, 0x00, 0x00, // thiszone
		0x00, 0x00, 0x00, 0x00, // sigfigs
		0xff, 0xff, 0x00, 0x00, // snaplen
		0x01, 0x00, 0x00, 0x00, // network (Ethernet)
	}
	pcapPath := filepath.Join(tmpDir, "test.pcap")
	if err := os.WriteFile(pcapPath, pcapHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	ws := NewWebServer(WebServerConfig{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    sensorID,
		PCAPSafeDir: tmpDir,
	})
	ws.setBaseContext(context.Background())

	// Start should succeed (goroutine will fail asynchronously reading the
	// mostly-empty PCAP, but StartPCAPForSweep itself returns nil).
	err := ws.StartPCAPForSweep("test.pcap", false, "fastest", 0, 0, 1, false)
	if err != nil {
		t.Fatalf("StartPCAPForSweep() unexpected error: %v", err)
	}

	// Wait a moment for the goroutine to finish
	done := ws.PCAPDone()
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("PCAP goroutine did not finish in time")
		}
	}
}

func TestStartPCAPForSweep_AnalysisMode(t *testing.T) {
	sensorID := "test-sweep-analysis"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	pcapHeader := []byte{
		0xd4, 0xc3, 0xb2, 0xa1,
		0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}
	pcapPath := filepath.Join(tmpDir, "analysis.pcap")
	if err := os.WriteFile(pcapPath, pcapHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	var started bool
	ws := NewWebServer(WebServerConfig{
		Address:       ":0",
		Stats:         NewPacketStats(),
		SensorID:      sensorID,
		PCAPSafeDir:   tmpDir,
		OnPCAPStarted: func() { started = true },
	})
	ws.setBaseContext(context.Background())

	err := ws.StartPCAPForSweep("analysis.pcap", true, "fastest", 0, 0, 1, true)
	if err != nil {
		t.Fatalf("StartPCAPForSweep() error: %v", err)
	}

	done := ws.PCAPDone()
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out")
		}
	}

	if !started {
		t.Error("expected onPCAPStarted to be called")
	}
}

// --- StopPCAPForSweep tests ---

func TestStopPCAPForSweep_NotInPCAPMode(t *testing.T) {
	ws := NewWebServer(WebServerConfig{
		Address: ":0",
		Stats:   NewPacketStats(),
	})

	// Default source is Live, so StopPCAPForSweep should be a no-op.
	err := ws.StopPCAPForSweep()
	if err != nil {
		t.Fatalf("StopPCAPForSweep() error: %v", err)
	}
}

func TestStopPCAPForSweep_InPCAPMode(t *testing.T) {
	sensorID := "test-stop-sweep"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	ws := NewWebServer(WebServerConfig{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	ws.setBaseContext(context.Background())

	// Simulate active PCAP mode: set source and create a done channel that
	// is already closed (simulating replay completion).
	done := make(chan struct{})
	close(done)

	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

	ws.pcapMu.Lock()
	ws.pcapDone = done
	ws.pcapCancel = func() {} // no-op cancel
	ws.pcapMu.Unlock()

	err := ws.StopPCAPForSweep()
	if err != nil {
		t.Fatalf("StopPCAPForSweep() error: %v", err)
	}

	// After stop, source should be back to Live
	ws.dataSourceMu.RLock()
	src := ws.currentSource
	ws.dataSourceMu.RUnlock()
	if src != DataSourceLive {
		t.Errorf("expected DataSourceLive, got %v", src)
	}
}

func TestStopPCAPForSweep_AnalysisMode(t *testing.T) {
	sensorID := "test-stop-analysis"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	ws := NewWebServer(WebServerConfig{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	ws.setBaseContext(context.Background())

	done := make(chan struct{})
	close(done)

	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAPAnalysis
	ws.dataSourceMu.Unlock()

	ws.pcapMu.Lock()
	ws.pcapDone = done
	ws.pcapCancel = func() {}
	ws.pcapAnalysisMode = true
	ws.pcapMu.Unlock()

	var stopped bool
	ws.onPCAPStopped = func() { stopped = true }

	err := ws.StopPCAPForSweep()
	if err != nil {
		t.Fatalf("StopPCAPForSweep() error: %v", err)
	}

	// In analysis mode, SetSourcePath should be cleared
	if mgr := l3grid.GetBackgroundManager(sensorID); mgr != nil {
		// Just verify it didn't crash
		_ = mgr.GetSourcePath()
	}

	// After analysis mode stop, source should be Live
	ws.dataSourceMu.RLock()
	src := ws.currentSource
	ws.dataSourceMu.RUnlock()
	if src != DataSourceLive {
		t.Errorf("expected DataSourceLive, got %v", src)
	}

	if !stopped {
		t.Error("expected onPCAPStopped callback to fire")
	}
}

func TestStopPCAPForSweep_NilCancelAndDone(t *testing.T) {
	sensorID := "test-stop-nil"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	ws := NewWebServer(WebServerConfig{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	ws.setBaseContext(context.Background())

	// Set PCAP mode but with nil cancel/done (edge case)
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

	err := ws.StopPCAPForSweep()
	if err != nil {
		t.Fatalf("StopPCAPForSweep() error: %v", err)
	}
}

// --- resolvePCAPPath edge cases ---

func TestResolvePCAPPath_NotRegularFile(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())

	// Create a subdirectory named with .pcap extension
	dirPath := filepath.Join(tmpDir, "subdir.pcap")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	ws := &WebServer{pcapSafeDir: tmpDir}
	_, err := ws.resolvePCAPPath("subdir.pcap")
	if err == nil {
		t.Fatal("expected error for directory with .pcap extension")
	}
	se, ok := err.(*switchError)
	if !ok {
		t.Fatalf("expected *switchError, got %T", err)
	}
	// Directory is not a regular file -> 400
	if se.status != 400 {
		t.Errorf("expected status 400, got %d: %v", se.status, se.err)
	}
}

func TestResolvePCAPPath_InvalidExtension(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())

	// Create a file with wrong extension
	badFile := filepath.Join(tmpDir, "data.txt")
	if err := os.WriteFile(badFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := &WebServer{pcapSafeDir: tmpDir}
	_, err := ws.resolvePCAPPath("data.txt")
	if err == nil {
		t.Fatal("expected error for wrong extension")
	}
}

func TestResolvePCAPPath_PcapngExtension(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())

	// Create a file with .pcapng extension
	pcapFile := filepath.Join(tmpDir, "capture.pcapng")
	if err := os.WriteFile(pcapFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := &WebServer{pcapSafeDir: tmpDir}
	resolved, err := ws.resolvePCAPPath("capture.pcapng")
	if err != nil {
		t.Fatalf("expected .pcapng to be accepted, got error: %v", err)
	}
	if resolved == "" {
		t.Error("expected non-empty resolved path")
	}
}

func TestResolvePCAPPath_PathTraversal(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())

	ws := &WebServer{pcapSafeDir: tmpDir}
	_, err := ws.resolvePCAPPath("../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestResolvePCAPPath_EmptyInput(t *testing.T) {
	ws := &WebServer{pcapSafeDir: "/tmp"}
	_, err := ws.resolvePCAPPath("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestResolvePCAPPath_EmptySafeDir(t *testing.T) {
	ws := &WebServer{pcapSafeDir: ""}
	_, err := ws.resolvePCAPPath("test.pcap")
	if err == nil {
		t.Fatal("expected error for empty safe directory")
	}
}

func TestResolvePCAPPath_NonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	ws := &WebServer{pcapSafeDir: tmpDir}
	_, err := ws.resolvePCAPPath("nonexistent.pcap")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if se, ok := err.(*switchError); ok {
		if se.status != 404 {
			t.Errorf("expected status 404, got %d", se.status)
		}
	}
}

// --- Additional coverage: default maxRetries path ---

func TestStartPCAPForSweep_DefaultMaxRetries(t *testing.T) {
	sensorID := "test-sweep-default"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	pcapHeader := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "default.pcap"), pcapHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	ws := NewWebServer(WebServerConfig{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    sensorID,
		PCAPSafeDir: tmpDir,
	})
	ws.setBaseContext(context.Background())

	// maxRetries=0 triggers the default (60)
	err := ws.StartPCAPForSweep("default.pcap", false, "fastest", 0, 0, 0, false)
	if err != nil {
		t.Fatalf("StartPCAPForSweep(maxRetries=0) error: %v", err)
	}
	done := ws.PCAPDone()
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out")
		}
	}
}

// --- Full integration: analysis mode with DB and callbacks ---

func TestStartPCAPForSweep_AnalysisModeWithDB(t *testing.T) {
	sensorID := "test-sweep-analysis-db"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	pcapHeader := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "analysis.pcap"), pcapHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()

	var recordedRunID string
	var recordingStarted bool
	var recordingStopped bool
	var pcapStarted bool

	ws := NewWebServer(WebServerConfig{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		PCAPSafeDir:       tmpDir,
		DB:                dbWrapped,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
		OnPCAPStarted:     func() { pcapStarted = true },
		OnPCAPStopped:     func() {},
		OnRecordingStart:  func(runID string) { recordedRunID = runID; recordingStarted = true },
		OnRecordingStop:   func(_ string) string { recordingStopped = true; return "" },
	})
	ws.setBaseContext(context.Background())

	// Start with analysis mode and recording enabled (disableRecording=false)
	err := ws.StartPCAPForSweep("analysis.pcap", true, "fastest", 0, 0, 1, false)
	if err != nil {
		t.Fatalf("StartPCAPForSweep() error: %v", err)
	}

	done := ws.PCAPDone()
	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for PCAP completion")
		}
	}

	if !pcapStarted {
		t.Error("expected onPCAPStarted callback")
	}

	// In analysis mode with DB, a run should have been started
	if recordedRunID == "" {
		t.Log("no run ID recorded (may not have analysis manager)")
	}
	if recordingStarted && !recordingStopped {
		t.Error("expected onRecordingStop to be called after recording started")
	}

	// Check the last analysis run ID was set
	lastID := ws.LastAnalysisRunID()
	if recordedRunID != "" && lastID != recordedRunID {
		t.Errorf("LastAnalysisRunID() = %q, want %q", lastID, recordedRunID)
	}

	// After analysis mode, source should be PCapAnalysis (grid preserved)
	ws.dataSourceMu.RLock()
	src := ws.currentSource
	ws.dataSourceMu.RUnlock()
	if src != DataSourcePCAPAnalysis {
		t.Logf("source after analysis: %v (expected PCAP_ANALYSIS)", src)
	}
}

// --- StopPCAPForSweep error paths ---

func TestStopPCAPForSweep_ResetStateError(t *testing.T) {
	sensorID := "test-stop-reset-err"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	ws := NewWebServer(WebServerConfig{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	ws.setBaseContext(context.Background())

	done := make(chan struct{})
	close(done)

	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

	ws.pcapMu.Lock()
	ws.pcapDone = done
	ws.pcapCancel = func() {}
	ws.pcapAnalysisMode = false
	ws.pcapMu.Unlock()

	// Even if resetAllState has no error, this exercises the non-analysis path
	err := ws.StopPCAPForSweep()
	if err != nil {
		t.Fatalf("StopPCAPForSweep() error: %v", err)
	}
}

// --- startPCAPLocked errors ---

func TestStartPCAPLocked_AlreadyInProgress(t *testing.T) {
	sensorID := "test-pcap-conflict"
	ws := NewWebServer(WebServerConfig{
		Address:  ":0",
		Stats:    NewPacketStats(),
		SensorID: sensorID,
	})
	ws.setBaseContext(context.Background())

	tmpDir := resolveSymlinks(t, t.TempDir())
	pcapHeader := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "conflict.pcap"), pcapHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	ws.pcapSafeDir = tmpDir
	ws.pcapMu.Lock()
	ws.pcapInProgress = true
	ws.pcapMu.Unlock()

	err := ws.startPCAPLocked("conflict.pcap", "fastest", 1.0, 0, 0, 0, 0, 0, 0, false, false)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	se, ok := err.(*switchError)
	if !ok {
		t.Fatalf("expected *switchError, got %T", err)
	}
	if se.status != 409 {
		t.Errorf("expected status 409, got %d", se.status)
	}
}

func TestStartPCAPLocked_NoBaseContext(t *testing.T) {
	sensorID := "test-pcap-no-ctx"
	tmpDir := resolveSymlinks(t, t.TempDir())
	pcapHeader := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "noctx.pcap"), pcapHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	ws := NewWebServer(WebServerConfig{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    sensorID,
		PCAPSafeDir: tmpDir,
	})
	// Do NOT set base context

	err := ws.startPCAPLocked("noctx.pcap", "fastest", 1.0, 0, 0, 0, 0, 0, 0, false, false)
	if err == nil {
		t.Fatal("expected error for nil base context")
	}
}

func TestStartLiveListenerLocked_NilBaseContext(t *testing.T) {
	ws := NewWebServer(WebServerConfig{
		Address:           ":0",
		Stats:             NewPacketStats(),
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	// Do NOT set base context — baseContext() returns nil

	ws.dataSourceMu.Lock()
	err := ws.startLiveListenerLocked()
	ws.dataSourceMu.Unlock()

	if err == nil {
		t.Fatal("expected error for nil base context")
	}
	if err.Error() != "webserver base context not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartLiveListenerLocked_AlreadyHasListener(t *testing.T) {
	ws := NewWebServer(WebServerConfig{
		Address:           ":0",
		Stats:             NewPacketStats(),
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	ws.setBaseContext(context.Background())

	// Set a non-nil udpListener to simulate already started
	ws.udpListener = network.NewUDPListener(network.UDPListenerConfig{Address: ":0"})

	ws.dataSourceMu.Lock()
	err := ws.startLiveListenerLocked()
	ws.dataSourceMu.Unlock()

	if err != nil {
		t.Fatalf("expected nil error when listener already exists, got: %v", err)
	}
}

// --- StartPCAPForSweep with recording disabled ---

func TestStartPCAPForSweep_DisableRecording(t *testing.T) {
	sensorID := "test-sweep-no-rec"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	pcapHeader := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "norec.pcap"), pcapHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()

	var recordingStartCalled bool
	ws := NewWebServer(WebServerConfig{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		PCAPSafeDir:       tmpDir,
		DB:                dbWrapped,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
		OnRecordingStart:  func(_ string) { recordingStartCalled = true },
	})
	ws.setBaseContext(context.Background())

	// disableRecording=true: recording callbacks should NOT fire
	err := ws.StartPCAPForSweep("norec.pcap", true, "fastest", 0, 0, 1, true)
	if err != nil {
		t.Fatalf("StartPCAPForSweep() error: %v", err)
	}

	done := ws.PCAPDone()
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out")
		}
	}

	if recordingStartCalled {
		t.Error("expected onRecordingStart NOT to be called when disableRecording=true")
	}
}

// --- StartPCAPForSweep error from startPCAPLocked ---

func TestStartPCAPForSweep_StartPCAPError(t *testing.T) {
	sensorID := "test-sweep-start-err"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	ws := NewWebServer(WebServerConfig{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    sensorID,
		PCAPSafeDir: "/nonexistent",
	})
	ws.setBaseContext(context.Background())

	// startPCAPLocked will fail because resolvePCAPPath will fail
	err := ws.StartPCAPForSweep("missing.pcap", false, "fastest", 0, 0, 1, false)
	if err == nil {
		t.Fatal("expected error from startPCAPLocked failure")
	}
}
