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

	if !caps.Radar {
		t.Error("Expected radar to be true")
	}
	if caps.Lidar.Enabled {
		t.Error("Expected lidar.enabled to be false when no provider set")
	}
	if caps.Lidar.State != "disabled" {
		t.Errorf("Expected lidar.state to be 'disabled', got %q", caps.Lidar.State)
	}
	if caps.LidarSweep {
		t.Error("Expected lidar_sweep to be false when no provider set")
	}
}

func TestShowCapabilities_WithLidarReady(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	server.SetCapabilitiesProvider(&mockCapabilitiesProvider{
		caps: Capabilities{
			Radar: true,
			Lidar: LidarCapability{
				Enabled: true,
				State:   "ready",
			},
			LidarSweep: true,
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

	if !caps.Radar {
		t.Error("Expected radar to be true")
	}
	if !caps.Lidar.Enabled {
		t.Error("Expected lidar.enabled to be true")
	}
	if caps.Lidar.State != "ready" {
		t.Errorf("Expected lidar.state to be 'ready', got %q", caps.Lidar.State)
	}
	if !caps.LidarSweep {
		t.Error("Expected lidar_sweep to be true")
	}
}

func TestShowCapabilities_LidarError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	server.SetCapabilitiesProvider(&mockCapabilitiesProvider{
		caps: Capabilities{
			Radar: true,
			Lidar: LidarCapability{
				Enabled: true,
				State:   "error",
			},
			LidarSweep: false,
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

	if caps.Lidar.State != "error" {
		t.Errorf("Expected lidar.state to be 'error', got %q", caps.Lidar.State)
	}
	if !caps.Lidar.Enabled {
		t.Error("Expected lidar.enabled to be true even in error state")
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
