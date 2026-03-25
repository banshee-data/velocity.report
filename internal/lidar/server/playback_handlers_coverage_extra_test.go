package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestPlayback_HandlePCAPStart_FormParseError(t *testing.T) {
	ws := &Server{sensorID: "sensor-form-error"}
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-form-error", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Body = errReadCloser{}
	w := httptest.NewRecorder()

	ws.handlePCAPStart(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestPlayback_HandlePCAPStart_ResetErrorRestartFails(t *testing.T) {
	sensorID := "sensor-start-reset-restart-fails"
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})

	ws := &Server{sensorID: sensorID}
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id="+sensorID, strings.NewReader(`{"pcap_file":"ignored.pcap","analysis_mode":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ws.handlePCAPStart(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestPlayback_HandlePCAPStart_ResetErrorRestartSucceeds(t *testing.T) {
	sensorID := "sensor-start-reset-restart-succeeds"
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ws := &Server{
		sensorID:          sensorID,
		udpListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}
	ws.setBaseContext(baseCtx)

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id="+sensorID, strings.NewReader(`{"pcap_file":"ignored.pcap","analysis_mode":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ws.handlePCAPStart(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}

	ws.dataSourceMu.Lock()
	ws.stopLiveListenerLocked()
	ws.dataSourceMu.Unlock()
}

func TestPlayback_HandlePCAPStart_GenericStartError(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir = resolveSymlinks(t, tmpDir)
	pcapPath := filepath.Join(tmpDir, "generic-error.pcap")
	if err := os.WriteFile(pcapPath, testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()
	dbWrapped.DB.Close()

	ws := NewServer(Config{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    "sensor-generic-start-error",
		PCAPSafeDir: tmpDir,
		DB:          dbWrapped,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-generic-start-error", strings.NewReader(`{"pcap_file":"generic-error.pcap","analysis_mode":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ws.handlePCAPStart(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestPlayback_HandlePCAPStart_ReplayModeStartedCallback(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir = resolveSymlinks(t, tmpDir)
	pcapPath := filepath.Join(tmpDir, "replay-started.pcap")
	if err := os.WriteFile(pcapPath, testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	var started bool
	ws := NewServer(Config{
		Address:       ":0",
		Stats:         NewPacketStats(),
		SensorID:      "sensor-replay-started",
		PCAPSafeDir:   tmpDir,
		OnPCAPStarted: func() { started = true },
	})
	ws.setBaseContext(context.Background())

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-replay-started", strings.NewReader(`{"pcap_file":"replay-started.pcap","analysis_mode":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ws.handlePCAPStart(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !started {
		t.Fatal("expected OnPCAPStarted callback")
	}

	waitForPCAPDone(t, ws)
}

func TestPlayback_HandlePCAPStart_AnalysisModeResponse(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir = resolveSymlinks(t, tmpDir)
	pcapPath := filepath.Join(tmpDir, "analysis-started.pcap")
	if err := os.WriteFile(pcapPath, testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	ws := NewServer(Config{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    "sensor-analysis-started",
		PCAPSafeDir: tmpDir,
	})
	ws.setBaseContext(context.Background())

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-analysis-started", strings.NewReader(`{"pcap_file":"analysis-started.pcap","analysis_mode":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ws.handlePCAPStart(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"analysis_mode":true`) {
		t.Fatalf("expected analysis mode response, got %s", w.Body.String())
	}

	waitForPCAPDone(t, ws)
}

func TestPlayback_HandlePCAPStop_ResetAllStateError(t *testing.T) {
	sensorID := "sensor-stop-reset-error"
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})

	ws := &Server{sensorID: sensorID}
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()
	ws.pcapMu.Lock()
	ws.pcapInProgress = true
	ws.pcapDone = make(chan struct{})
	close(ws.pcapDone)
	ws.pcapMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/stop?sensor_id="+sensorID, nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestPlayback_HandlePCAPStop_StartLiveListenerError(t *testing.T) {
	ws := &Server{sensorID: "sensor-stop-start-listener-error"}
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()
	ws.pcapMu.Lock()
	ws.pcapInProgress = true
	ws.pcapDone = make(chan struct{})
	close(ws.pcapDone)
	ws.pcapMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/stop?sensor_id=sensor-stop-start-listener-error", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestPlayback_HandlePCAPStop_OnStoppedCallback(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stopped bool
	ws := &Server{
		sensorID:          "sensor-stop-callback",
		udpListenerConfig: network.UDPListenerConfig{Address: ":0"},
		onPCAPStopped:     func() { stopped = true },
	}
	ws.setBaseContext(baseCtx)
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()
	ws.pcapMu.Lock()
	ws.pcapInProgress = true
	ws.pcapDone = make(chan struct{})
	close(ws.pcapDone)
	ws.pcapMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/stop?sensor_id=sensor-stop-callback", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !stopped {
		t.Fatal("expected OnPCAPStopped callback")
	}

	ws.dataSourceMu.Lock()
	ws.stopLiveListenerLocked()
	ws.dataSourceMu.Unlock()
}

func TestPlayback_HandlePCAPResumeLive_StartListenerError(t *testing.T) {
	ws := &Server{sensorID: "sensor-resume-error"}
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAPAnalysis
	ws.dataSourceMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/resume_live?sensor_id=sensor-resume-error", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPResumeLive(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestPlayback_HandlePCAPResumeLive_OnStoppedCallback(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stopped bool
	ws := &Server{
		sensorID:          "sensor-resume-callback",
		udpListenerConfig: network.UDPListenerConfig{Address: ":0"},
		onPCAPStopped:     func() { stopped = true },
	}
	ws.setBaseContext(baseCtx)
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAPAnalysis
	ws.dataSourceMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/resume_live?sensor_id=sensor-resume-callback", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPResumeLive(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !stopped {
		t.Fatal("expected OnPCAPStopped callback")
	}

	ws.dataSourceMu.Lock()
	ws.stopLiveListenerLocked()
	ws.dataSourceMu.Unlock()
}

func TestPlayback_HandleVRLogLoad_DefaultSafeDirAndUnknownEncoding(t *testing.T) {
	var loadedPath string
	ws := &Server{
		onVRLogLoad: func(path string) (string, error) {
			loadedPath = path
			return "", nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/vrlog/load", strings.NewReader(`{"vrlog_path":"/var/lib/velocity-report/test.vrlog"}`))
	w := httptest.NewRecorder()
	ws.handleVRLogLoad(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if loadedPath != "/var/lib/velocity-report/test.vrlog" {
		t.Fatalf("loadedPath = %q, want default safe-dir path", loadedPath)
	}
	if !strings.Contains(w.Body.String(), `"frame_encoding":"unknown"`) {
		t.Fatalf("expected unknown frame encoding, got %s", w.Body.String())
	}
}
