package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockCapabilitiesProvider implements CapabilitiesProvider for tests.
type mockCapabilitiesProvider struct {
	caps Capabilities
}

func (m *mockCapabilitiesProvider) Capabilities() Capabilities {
	return m.caps
}

func TestShowCapabilities_DefaultRadarOnly(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// No capabilities provider set — should return radar-only defaults.
	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	w := httptest.NewRecorder()

	server.showCapabilities(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var caps Capabilities
	if err := json.NewDecoder(w.Body).Decode(&caps); err != nil {
		t.Fatalf("Failed to decode capabilities: %v", err)
	}

	// Radar should have a "default" entry that is enabled.
	radarDefault, ok := caps.Radar["default"]
	if !ok {
		t.Fatal("Expected radar.default to exist")
	}
	if !radarDefault.Enabled {
		t.Error("Expected radar.default.enabled to be true")
	}
	if radarDefault.Status != "receiving" {
		t.Errorf("Expected radar.default.status 'receiving', got %q", radarDefault.Status)
	}

	// Lidar should be an empty map when no provider is set.
	if len(caps.Lidar) != 0 {
		t.Errorf("Expected empty lidar map, got %d entries", len(caps.Lidar))
	}
}

func TestShowCapabilities_WithLidarReady(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	server.SetCapabilitiesProvider(&mockCapabilitiesProvider{
		caps: Capabilities{
			Radar: map[string]SensorStatus{
				"default": {Enabled: true, Status: "receiving"},
			},
			Lidar: map[string]LidarSensorStatus{
				"default": {
					SensorStatus: SensorStatus{Enabled: true, Status: "ready"},
					Sweep:        true,
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	w := httptest.NewRecorder()

	server.showCapabilities(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var caps Capabilities
	if err := json.NewDecoder(w.Body).Decode(&caps); err != nil {
		t.Fatalf("Failed to decode capabilities: %v", err)
	}

	radarDefault, ok := caps.Radar["default"]
	if !ok {
		t.Fatal("Expected radar.default to exist")
	}
	if !radarDefault.Enabled {
		t.Error("Expected radar.default.enabled to be true")
	}

	lidarDefault, ok := caps.Lidar["default"]
	if !ok {
		t.Fatal("Expected lidar.default to exist")
	}
	if !lidarDefault.Enabled {
		t.Error("Expected lidar.default.enabled to be true")
	}
	if lidarDefault.Status != "ready" {
		t.Errorf("Expected lidar.default.status 'ready', got %q", lidarDefault.Status)
	}
	if !lidarDefault.Sweep {
		t.Error("Expected lidar.default.sweep to be true")
	}
}

func TestShowCapabilities_LidarError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	server.SetCapabilitiesProvider(&mockCapabilitiesProvider{
		caps: Capabilities{
			Radar: map[string]SensorStatus{
				"default": {Enabled: true, Status: "receiving"},
			},
			Lidar: map[string]LidarSensorStatus{
				"default": {
					SensorStatus: SensorStatus{Enabled: true, Status: "error"},
					Sweep:        false,
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	w := httptest.NewRecorder()

	server.showCapabilities(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var caps Capabilities
	if err := json.NewDecoder(w.Body).Decode(&caps); err != nil {
		t.Fatalf("Failed to decode capabilities: %v", err)
	}

	lidarDefault, ok := caps.Lidar["default"]
	if !ok {
		t.Fatal("Expected lidar.default to exist")
	}
	if lidarDefault.Status != "error" {
		t.Errorf("Expected lidar.default.status 'error', got %q", lidarDefault.Status)
	}
	if !lidarDefault.Enabled {
		t.Error("Expected lidar.default.enabled to be true even in error state")
	}
}

func TestShowCapabilities_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(http.MethodPost, "/api/capabilities", nil)
	w := httptest.NewRecorder()

	server.showCapabilities(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}
