package monitor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// --- SetSweepStore ---

func TestCov_SetSweepStore(t *testing.T) {
	ws := &WebServer{}

	if ws.sweepStore != nil {
		t.Fatal("expected nil sweepStore initially")
	}

	store := &lidar.SweepStore{}
	ws.SetSweepStore(store)

	if ws.sweepStore != store {
		t.Error("SetSweepStore did not set the store")
	}
}

// --- handleSweepDashboard ---

func TestCov_HandleSweepDashboard(t *testing.T) {
	ws := &WebServer{sensorID: "test-sensor"}

	req := httptest.NewRequest(http.MethodGet, "/sweep-dashboard", nil)
	w := httptest.NewRecorder()
	ws.handleSweepDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !containsString(w.Body.String(), "test-sensor") {
		t.Error("response should contain sensor ID")
	}
}

func TestCov_HandleSweepDashboard_WithQuerySensorID(t *testing.T) {
	ws := &WebServer{sensorID: "default-sensor"}

	req := httptest.NewRequest(http.MethodGet, "/sweep-dashboard?sensor_id=custom-sensor", nil)
	w := httptest.NewRecorder()
	ws.handleSweepDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !containsString(w.Body.String(), "custom-sensor") {
		t.Error("response should contain custom sensor ID")
	}
}

// --- handleBackgroundGridPolar ---

func TestCov_HandleBackgroundGridPolar_NoManager(t *testing.T) {
	ws := &WebServer{sensorID: "no-such-sensor-bg"}

	req := httptest.NewRequest(http.MethodGet, "/background-grid-polar", nil)
	w := httptest.NewRecorder()
	ws.handleBackgroundGridPolar(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleBackgroundGridPolar_WithQuerySensorID(t *testing.T) {
	ws := &WebServer{sensorID: "default-sensor-bg"}

	req := httptest.NewRequest(http.MethodGet, "/background-grid-polar?sensor_id=nonexistent-sensor", nil)
	w := httptest.NewRecorder()
	ws.handleBackgroundGridPolar(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// --- handleForegroundFrameChart ---

func TestCov_HandleForegroundFrameChart_NoSnapshot(t *testing.T) {
	ws := &WebServer{sensorID: "no-such-sensor-fg"}

	req := httptest.NewRequest(http.MethodGet, "/foreground-frame-chart", nil)
	w := httptest.NewRecorder()
	ws.handleForegroundFrameChart(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandleForegroundFrameChart_QuerySensor(t *testing.T) {
	ws := &WebServer{sensorID: "default-sensor-fg"}

	req := httptest.NewRequest(http.MethodGet, "/foreground-frame-chart?sensor_id=nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleForegroundFrameChart(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// --- handleLidarSnapshot ---

func TestCov_HandleLidarSnapshot_WrongMethod(t *testing.T) {
	ws := &WebServer{}

	req := httptest.NewRequest(http.MethodPost, "/lidar-snapshot", nil)
	w := httptest.NewRecorder()
	ws.handleLidarSnapshot(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandleLidarSnapshot_MissingSensorID(t *testing.T) {
	ws := &WebServer{}

	req := httptest.NewRequest(http.MethodGet, "/lidar-snapshot", nil)
	w := httptest.NewRecorder()
	ws.handleLidarSnapshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleLidarSnapshot_NilDB(t *testing.T) {
	ws := &WebServer{db: nil}

	req := httptest.NewRequest(http.MethodGet, "/lidar-snapshot?sensor_id=test", nil)
	w := httptest.NewRecorder()
	ws.handleLidarSnapshot(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestCov_HandleLidarSnapshot_NoSnapshot(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()
	ws := &WebServer{db: &db.DB{DB: sqlDB}}

	req := httptest.NewRequest(http.MethodGet, "/lidar-snapshot?sensor_id=nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleLidarSnapshot(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// --- handlePCAPStop ---

func TestCov_HandlePCAPStop_WrongMethod(t *testing.T) {
	ws := &WebServer{sensorID: "sensor-1"}

	req := httptest.NewRequest(http.MethodGet, "/pcap/stop", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestCov_HandlePCAPStop_MissingSensorID(t *testing.T) {
	ws := &WebServer{sensorID: "sensor-1"}

	req := httptest.NewRequest(http.MethodPost, "/pcap/stop", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandlePCAPStop_WrongSensorID(t *testing.T) {
	ws := &WebServer{sensorID: "sensor-1"}

	req := httptest.NewRequest(http.MethodPost, "/pcap/stop?sensor_id=wrong-sensor", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCov_HandlePCAPStop_NotInPCAPMode(t *testing.T) {
	ws := &WebServer{
		sensorID:      "sensor-1",
		currentSource: DataSourceLive,
	}

	req := httptest.NewRequest(http.MethodPost, "/pcap/stop?sensor_id=sensor-1", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestCov_HandlePCAPStop_NoPCAPInProgress(t *testing.T) {
	ws := &WebServer{
		sensorID:       "sensor-1",
		currentSource:  DataSourcePCAP,
		pcapInProgress: false,
	}

	req := httptest.NewRequest(http.MethodPost, "/pcap/stop?sensor_id=sensor-1", nil)
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestCov_HandlePCAPStop_SensorIDFromFormValue(t *testing.T) {
	ws := &WebServer{
		sensorID:      "sensor-1",
		currentSource: DataSourceLive,
	}

	req := httptest.NewRequest(http.MethodPost, "/pcap/stop", nil)
	req.Form = map[string][]string{"sensor_id": {"sensor-1"}}
	w := httptest.NewRecorder()
	ws.handlePCAPStop(w, req)

	// Should hit the "not in PCAP mode" branch (not form parsing error)
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

// --- handleExportFrameSequenceASC ---

func TestCov_HandleExportFrameSequenceASC_MissingSensorID(t *testing.T) {
	ws := &WebServer{}

	req := httptest.NewRequest(http.MethodPost, "/export-frame-sequence", nil)
	w := httptest.NewRecorder()
	ws.handleExportFrameSequenceASC(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCov_HandleExportFrameSequenceASC_NoFrameBuilder(t *testing.T) {
	ws := &WebServer{}

	req := httptest.NewRequest(http.MethodPost, "/export-frame-sequence?sensor_id=nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleExportFrameSequenceASC(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// --- exportForegroundSequenceInternal ---

func TestCov_ExportForegroundSequenceInternal_ZeroCount(t *testing.T) {
	ws := &WebServer{}
	// count <= 0 should return immediately without panic
	ws.exportForegroundSequenceInternal("sensor-1", 0)
	ws.exportForegroundSequenceInternal("sensor-1", -1)
}

// helper to avoid importing strings for a simple contains check
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
