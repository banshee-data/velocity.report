package main

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/api"
)

func TestCapabilitiesProvider_DefaultState(t *testing.T) {
	cp := newCapabilitiesProvider()
	caps := cp.Capabilities()

	radarDefault, ok := caps.Radar["default"]
	if !ok {
		t.Fatal("Expected radar.default to exist")
	}
	if !radarDefault.Enabled {
		t.Error("Expected radar.default.enabled to be true")
	}

	if len(caps.Lidar) != 0 {
		t.Errorf("Expected empty lidar map by default, got %d entries", len(caps.Lidar))
	}
}

func TestCapabilitiesProvider_LidarReady(t *testing.T) {
	cp := newCapabilitiesProvider()
	cp.SetLidarReady(true)
	caps := cp.Capabilities()

	lidarDefault, ok := caps.Lidar["default"]
	if !ok {
		t.Fatal("Expected lidar.default to exist after SetLidarReady")
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

func TestCapabilitiesProvider_LidarStarting(t *testing.T) {
	cp := newCapabilitiesProvider()
	// Transition through ready first so lidarSweep is true, then verify
	// that SetLidarStarting clears it.
	cp.SetLidarReady(true)
	cp.SetLidarStarting()
	caps := cp.Capabilities()

	lidarDefault, ok := caps.Lidar["default"]
	if !ok {
		t.Fatal("Expected lidar.default to exist during starting")
	}
	if !lidarDefault.Enabled {
		t.Error("Expected lidar.default.enabled to be true during starting")
	}
	if lidarDefault.Status != "starting" {
		t.Errorf("Expected lidar.default.status 'starting', got %q", lidarDefault.Status)
	}
	if lidarDefault.Sweep {
		t.Error("Expected lidar.default.sweep to be false during starting")
	}
}

func TestCapabilitiesProvider_LidarError(t *testing.T) {
	cp := newCapabilitiesProvider()
	// Transition through ready so lidarSweep is true, then verify
	// that SetLidarError clears it.
	cp.SetLidarReady(true)
	cp.SetLidarError()
	caps := cp.Capabilities()

	lidarDefault, ok := caps.Lidar["default"]
	if !ok {
		t.Fatal("Expected lidar.default to exist in error state")
	}
	if !lidarDefault.Enabled {
		t.Error("Expected lidar.default.enabled to be true in error state")
	}
	if lidarDefault.Status != "error" {
		t.Errorf("Expected lidar.default.status 'error', got %q", lidarDefault.Status)
	}
	if lidarDefault.Sweep {
		t.Error("Expected lidar.default.sweep to be false in error state")
	}
}

func TestCapabilitiesProvider_LidarDisabled(t *testing.T) {
	cp := newCapabilitiesProvider()
	cp.SetLidarReady(true)
	cp.SetLidarDisabled()
	caps := cp.Capabilities()

	if len(caps.Lidar) != 0 {
		t.Errorf("Expected empty lidar map after SetLidarDisabled, got %d entries", len(caps.Lidar))
	}
}

func TestCapabilitiesProvider_ImplementsInterface(t *testing.T) {
	cp := newCapabilitiesProvider()
	// Compile-time check that *capabilitiesProvider satisfies api.CapabilitiesProvider
	var _ api.CapabilitiesProvider = cp
}
