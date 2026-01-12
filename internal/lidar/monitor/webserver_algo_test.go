package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

func TestHandleAlgorithmConfig(t *testing.T) {
	// Setup
	sensorID := "test-algo-sensor"

	// Ensure background manager is registered (global state in lidar package)
	// NewBackgroundManager registers itself
	bgManager := lidar.NewBackgroundManager(sensorID, 16, 360, lidar.BackgroundParams{}, nil)

	cfg := &lidar.TrackingPipelineConfig{
		BackgroundManager: bgManager,
		SensorID:          sensorID,
		ExtractorMode:     "background",
		DebugMode:         true,
	}
	pipeline := lidar.NewTrackingPipeline(cfg)

	// minimal webserver struct
	ws := &WebServer{
		sensorID:         sensorID,
		trackingPipeline: pipeline,
	}

	// 1. Test GET (JSON)
	reqGet := httptest.NewRequest("GET", "/api/lidar/algorithm", nil)
	wGet := httptest.NewRecorder()
	ws.handleAlgorithmConfig(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Errorf("GET failed: %d", wGet.Code)
	}
	var respGet map[string]string
	json.NewDecoder(wGet.Body).Decode(&respGet)
	if respGet["mode"] != "background" {
		t.Errorf("Expected background, got %s", respGet["mode"])
	}

	// 2. Test POST JSON
	jsonBody := `{"mode": "velocity"}`
	reqPostJson := httptest.NewRequest("POST", "/api/lidar/algorithm", strings.NewReader(jsonBody))
	reqPostJson.Header.Set("Content-Type", "application/json")
	wPostJson := httptest.NewRecorder()
	ws.handleAlgorithmConfig(wPostJson, reqPostJson)

	if wPostJson.Code != http.StatusOK {
		t.Errorf("POST JSON failed: %d", wPostJson.Code)
	}
	if pipeline.GetExtractorMode() != "velocity" {
		t.Errorf("Pipeline not updated to velocity")
	}

	// 3. Test POST Form
	form := url.Values{}
	form.Add("mode", "hybrid")
	reqPostForm := httptest.NewRequest("POST", "/api/lidar/algorithm", strings.NewReader(form.Encode()))
	reqPostForm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wPostForm := httptest.NewRecorder()
	ws.handleAlgorithmConfig(wPostForm, reqPostForm)

	if wPostForm.Code != http.StatusSeeOther {
		t.Errorf("POST Form expected redirect (303), got %d", wPostForm.Code)
	}
	if pipeline.GetExtractorMode() != "hybrid" {
		t.Errorf("Pipeline not updated to hybrid")
	}
}
