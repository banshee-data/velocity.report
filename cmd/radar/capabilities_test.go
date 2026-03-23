package main

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/api"
)

func TestCapabilitiesProvider_DefaultState(t *testing.T) {
	cp := newCapabilitiesProvider()
	caps := cp.Capabilities()

	if !caps.Radar {
		t.Error("Expected radar to be true")
	}
	if caps.Lidar.Enabled {
		t.Error("Expected lidar.enabled to be false by default")
	}
	if caps.Lidar.State != "disabled" {
		t.Errorf("Expected lidar.state 'disabled', got %q", caps.Lidar.State)
	}
	if caps.LidarSweep {
		t.Error("Expected lidar_sweep to be false by default")
	}
}

func TestCapabilitiesProvider_LidarReady(t *testing.T) {
	cp := newCapabilitiesProvider()
	cp.SetLidarReady(true)
	caps := cp.Capabilities()

	if !caps.Lidar.Enabled {
		t.Error("Expected lidar.enabled to be true after SetLidarReady")
	}
	if caps.Lidar.State != "ready" {
		t.Errorf("Expected lidar.state 'ready', got %q", caps.Lidar.State)
	}
	if !caps.LidarSweep {
		t.Error("Expected lidar_sweep to be true")
	}
}

func TestCapabilitiesProvider_LidarStarting(t *testing.T) {
	cp := newCapabilitiesProvider()
	cp.SetLidarStarting()
	caps := cp.Capabilities()

	if !caps.Lidar.Enabled {
		t.Error("Expected lidar.enabled to be true during starting")
	}
	if caps.Lidar.State != "starting" {
		t.Errorf("Expected lidar.state 'starting', got %q", caps.Lidar.State)
	}
}

func TestCapabilitiesProvider_LidarError(t *testing.T) {
	cp := newCapabilitiesProvider()
	cp.SetLidarStarting()
	cp.SetLidarError()
	caps := cp.Capabilities()

	if !caps.Lidar.Enabled {
		t.Error("Expected lidar.enabled to be true in error state")
	}
	if caps.Lidar.State != "error" {
		t.Errorf("Expected lidar.state 'error', got %q", caps.Lidar.State)
	}
}

func TestCapabilitiesProvider_LidarDisabled(t *testing.T) {
	cp := newCapabilitiesProvider()
	cp.SetLidarReady(true)
	cp.SetLidarDisabled()
	caps := cp.Capabilities()

	if caps.Lidar.Enabled {
		t.Error("Expected lidar.enabled to be false after SetLidarDisabled")
	}
	if caps.Lidar.State != "disabled" {
		t.Errorf("Expected lidar.state 'disabled', got %q", caps.Lidar.State)
	}
	if caps.LidarSweep {
		t.Error("Expected lidar_sweep to be false after disable")
	}
}

func TestCapabilitiesProvider_ImplementsInterface(t *testing.T) {
	cp := newCapabilitiesProvider()
	// Compile-time check that *capabilitiesProvider satisfies api.CapabilitiesProvider
	var _ api.CapabilitiesProvider = cp
}
