package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

func TestNewAlgorithmAPI(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	if api == nil {
		t.Fatal("NewAlgorithmAPI returned nil")
	}

	if api.sensorID != "test-sensor" {
		t.Errorf("Expected sensorID 'test-sensor', got '%s'", api.sensorID)
	}
}

func TestAlgorithmAPI_SetPipeline(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	config := lidar.DefaultDualPipelineConfig()
	pipeline := lidar.NewDualExtractionPipeline(config)

	api.SetPipeline(pipeline)

	// Verify pipeline is set by checking we can get config
	if api.pipeline == nil {
		t.Error("Pipeline was not set")
	}
}

func TestAlgorithmAPI_SetVCTracker(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	tracker := lidar.NewVelocityCoherentTracker(lidar.DefaultVelocityCoherentTrackerConfig())

	api.SetVCTracker(tracker)

	if api.vcTracker == nil {
		t.Error("VC Tracker was not set")
	}
}

func TestAlgorithmAPI_RegisterRoutes(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")
	mux := http.NewServeMux()

	api.RegisterRoutes(mux)

	// The routes should be registered without panic
	// We can't easily verify the routes are there without reflection
}

func TestAlgorithmAPI_HandleAlgorithmConfig_GET(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	// Set up a pipeline
	config := lidar.DefaultDualPipelineConfig()
	config.ActiveAlgorithm = lidar.AlgorithmBackgroundSubtraction
	pipeline := lidar.NewDualExtractionPipeline(config)
	api.SetPipeline(pipeline)

	req := httptest.NewRequest("GET", "/api/lidar/tracking/algorithm", nil)
	w := httptest.NewRecorder()

	api.handleAlgorithmConfig(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var respBody AlgorithmConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if respBody.Active != "background_subtraction" {
		t.Errorf("Expected active algorithm 'background_subtraction', got '%s'", respBody.Active)
	}
}

func TestAlgorithmAPI_HandleAlgorithmConfig_POST(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	// Set up a pipeline
	config := lidar.DefaultDualPipelineConfig()
	config.ActiveAlgorithm = lidar.AlgorithmBackgroundSubtraction
	pipeline := lidar.NewDualExtractionPipeline(config)
	api.SetPipeline(pipeline)

	// Switch to velocity_coherent
	reqBody := `{"active": "velocity_coherent"}`
	req := httptest.NewRequest("POST", "/api/lidar/tracking/algorithm", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleAlgorithmConfig(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify the algorithm was changed
	newConfig := pipeline.GetConfig()
	if newConfig.ActiveAlgorithm != lidar.AlgorithmVelocityCoherent {
		t.Errorf("Expected algorithm to be velocity_coherent, got %s", newConfig.ActiveAlgorithm)
	}
}

func TestAlgorithmAPI_HandleAlgorithmConfig_InvalidMethod(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	req := httptest.NewRequest("PUT", "/api/lidar/tracking/algorithm", nil)
	w := httptest.NewRecorder()

	api.handleAlgorithmConfig(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestAlgorithmAPI_HandleAlgorithmConfig_NoPipeline_GET(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	// No pipeline set - should return defaults
	req := httptest.NewRequest("GET", "/api/lidar/tracking/algorithm", nil)
	w := httptest.NewRecorder()

	api.handleAlgorithmConfig(w, req)

	resp := w.Result()
	// GET returns defaults even without a pipeline
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var respBody AlgorithmConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return default algorithm
	if respBody.Active != "background_subtraction" {
		t.Errorf("Expected default algorithm 'background_subtraction', got '%s'", respBody.Active)
	}
}

func TestAlgorithmAPI_HandleAlgorithmConfig_NoPipeline_POST(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	// No pipeline set - POST should return error
	reqBody := `{"active": "velocity_coherent"}`
	req := httptest.NewRequest("POST", "/api/lidar/tracking/algorithm", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleAlgorithmConfig(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", resp.StatusCode)
	}
}

func TestAlgorithmAPI_HandleTrackingStats(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	// Set up a pipeline
	config := lidar.DefaultDualPipelineConfig()
	pipeline := lidar.NewDualExtractionPipeline(config)
	api.SetPipeline(pipeline)

	req := httptest.NewRequest("GET", "/api/lidar/tracking/stats", nil)
	w := httptest.NewRecorder()

	api.handleTrackingStats(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestAlgorithmAPI_HandleVCTracks(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	// Set up a VC tracker
	tracker := lidar.NewVelocityCoherentTracker(lidar.DefaultVelocityCoherentTrackerConfig())
	api.SetVCTracker(tracker)

	req := httptest.NewRequest("GET", "/api/lidar/tracking/vc/tracks", nil)
	w := httptest.NewRecorder()

	api.handleVCTracks(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestAlgorithmAPI_HandleVCConfig_GET(t *testing.T) {
	api := NewAlgorithmAPI(nil, "test-sensor")

	// Set up a VC tracker
	tracker := lidar.NewVelocityCoherentTracker(lidar.DefaultVelocityCoherentTrackerConfig())
	api.SetVCTracker(tracker)

	req := httptest.NewRequest("GET", "/api/lidar/tracking/vc/config", nil)
	w := httptest.NewRecorder()

	api.handleVCConfig(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
