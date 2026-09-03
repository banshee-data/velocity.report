package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// registerGridlessManager registers a background manager with no grid for the
// duration of the test. A manager exists in the registry from the moment the
// sensor is known, but its grid is only built once the first frame arrives, so
// this is the real window every status endpoint is exposed to at startup.
func registerGridlessManager(t *testing.T, sensorID string) {
	t.Helper()
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})
	t.Cleanup(func() { l3grid.RegisterBackgroundManager(sensorID, nil) })
}

// TestGridStatusReportsErrorBeforeGridExists covers the nil-status arm. Before
// the first frame the manager has no grid, and the endpoint has to say so
// rather than serialise a nil into a 200.
func TestGridStatusReportsErrorBeforeGridExists(t *testing.T) {
	sensorID := "test-status-gridless"
	registerGridlessManager(t, sensorID)

	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats(), SensorID: sensorID})

	rec := httptest.NewRecorder()
	ws.handleGridStatus(rec, httptest.NewRequest(http.MethodGet, "/api/lidar/grid?sensor_id=test-status-gridless", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "background manager") {
		t.Errorf("body %q should point at the uninitialised background manager", rec.Body.String())
	}
}

// TestDataSourceIncludesSettlingProgress covers the settling lookup on the
// data-source endpoint. Until the grid settles there is no foreground at all,
// so an operator watching an empty scene needs this field to tell "still
// settling" from "broken sensor".
func TestDataSourceIncludesSettlingProgress(t *testing.T) {
	sensorID := "test-status-settling"
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats(), SensorID: sensorID})
	ws.setBaseContext(t.Context())

	rec := httptest.NewRecorder()
	ws.handleDataSource(rec, httptest.NewRequest(http.MethodGet, "/api/lidar/datasource", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status body: %v", err)
	}
	if _, ok := body["settling"]; !ok {
		t.Errorf("data-source response has no settling field; keys: %v", keysOf(body))
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
