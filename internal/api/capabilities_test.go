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

// TestShowCapabilities_EmptyMaps verifies that a provider returning empty
// radar and lidar maps serialises to {} (not null) for both keys.
func TestShowCapabilities_EmptyMaps(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	server.SetCapabilitiesProvider(&mockCapabilitiesProvider{
		caps: Capabilities{
			Radar: map[string]SensorStatus{},
			Lidar: map[string]LidarSensorStatus{},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	w := httptest.NewRecorder()

	server.showCapabilities(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Verify raw JSON uses {} not null for empty maps.
	body := w.Body.String()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("Failed to decode raw response: %v", err)
	}
	if string(raw["radar"]) != "{}" {
		t.Errorf("Expected radar to be {}, got %s", raw["radar"])
	}
	if string(raw["lidar"]) != "{}" {
		t.Errorf("Expected lidar to be {}, got %s", raw["lidar"])
	}

	// Also decode into typed struct.
	var caps Capabilities
	if err := json.Unmarshal([]byte(body), &caps); err != nil {
		t.Fatalf("Failed to decode capabilities: %v", err)
	}
	if len(caps.Radar) != 0 {
		t.Errorf("Expected empty radar map, got %d entries", len(caps.Radar))
	}
	if len(caps.Lidar) != 0 {
		t.Errorf("Expected empty lidar map, got %d entries", len(caps.Lidar))
	}
}

// TestShowCapabilities_MultiSensor verifies the handler round-trips
// multiple named sensors per class correctly.
func TestShowCapabilities_MultiSensor(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	server.SetCapabilitiesProvider(&mockCapabilitiesProvider{
		caps: Capabilities{
			Radar: map[string]SensorStatus{
				"ops243_front": {Enabled: true, Status: "receiving"},
				"ops243_rear":  {Enabled: true, Status: "stale"},
			},
			Lidar: map[string]LidarSensorStatus{
				"hesai": {
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

	if len(caps.Radar) != 2 {
		t.Fatalf("Expected 2 radar entries, got %d", len(caps.Radar))
	}
	if caps.Radar["ops243_front"].Status != "receiving" {
		t.Errorf("Expected ops243_front status 'receiving', got %q", caps.Radar["ops243_front"].Status)
	}
	if caps.Radar["ops243_rear"].Status != "stale" {
		t.Errorf("Expected ops243_rear status 'stale', got %q", caps.Radar["ops243_rear"].Status)
	}
	if !caps.Lidar["hesai"].Sweep {
		t.Error("Expected hesai lidar sweep to be true")
	}
}
