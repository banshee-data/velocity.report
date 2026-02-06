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

func TestHandleTuningParams_FormSubmission(t *testing.T) {
	// Setup mock background manager
	sensorID := "test-sensor"
	params := lidar.BackgroundParams{
		NoiseRelativeFraction: 0.01,
	}

	// We need a real background manager for this test since the handler looks it up
	// This is tricky without mocking the global registry or creating a full manager
	// Let's try to create a minimal one if possible, or skip if too complex

	// Create a temporary DB for the manager
	// For unit testing webserver handlers that depend on global state,
	// we might need to refactor or use a more integration-test approach.
	// However, we can try to register a dummy manager.

	// Create a dummy manager
	bm := lidar.NewBackgroundManager(sensorID, 1, 1, params, nil)
	if bm == nil {
		t.Skip("Skipping test: could not create background manager")
	}

	// Create webserver instance
	ws := &WebServer{}

	// Create form data
	newParams := map[string]interface{}{
		"noise_relative": 0.05,
	}
	jsonBytes, _ := json.Marshal(newParams)

	form := url.Values{}
	form.Add("config_json", string(jsonBytes))

	// Create request
	req := httptest.NewRequest("POST", "/api/lidar/params?sensor_id="+sensorID, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()

	// Call handler
	ws.handleTuningParams(w, req)

	// Check response
	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Expected status 303 See Other, got %d", resp.StatusCode)
	}

	// Verify params were updated
	currentParams := bm.GetParams()
	if currentParams.NoiseRelativeFraction != 0.05 {
		t.Errorf("Expected NoiseRelativeFraction 0.05, got %f", currentParams.NoiseRelativeFraction)
	}
}
