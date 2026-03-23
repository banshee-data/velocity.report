package main

import (
	"sync"

	"github.com/banshee-data/velocity.report/internal/api"
)

// capabilitiesProvider reports sensor availability to the API layer.
// It holds a snapshot of which subsystems were enabled at startup and
// allows runtime state transitions for LiDAR (disabled → starting →
// ready → error) without restarting the radar process.
type capabilitiesProvider struct {
	mu         sync.RWMutex
	lidarState string // "disabled", "starting", "ready", "error"
	lidarSweep bool
}

// newCapabilitiesProvider returns a provider with LiDAR disabled.
func newCapabilitiesProvider() *capabilitiesProvider {
	return &capabilitiesProvider{
		lidarState: "disabled",
	}
}

// SetLidarReady marks the LiDAR subsystem as ready for traffic.
func (cp *capabilitiesProvider) SetLidarReady(sweepEnabled bool) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lidarState = "ready"
	cp.lidarSweep = sweepEnabled
}

// SetLidarStarting marks the LiDAR subsystem as starting up.
func (cp *capabilitiesProvider) SetLidarStarting() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lidarState = "starting"
}

// SetLidarError marks the LiDAR subsystem as having encountered an error.
func (cp *capabilitiesProvider) SetLidarError() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lidarState = "error"
}

// SetLidarDisabled marks the LiDAR subsystem as disabled.
func (cp *capabilitiesProvider) SetLidarDisabled() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lidarState = "disabled"
	cp.lidarSweep = false
}

// Capabilities returns the current sensor state snapshot.
func (cp *capabilitiesProvider) Capabilities() api.Capabilities {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	enabled := cp.lidarState != "disabled"
	return api.Capabilities{
		Radar: true,
		Lidar: api.LidarCapability{
			Enabled: enabled,
			State:   cp.lidarState,
		},
		LidarSweep: cp.lidarSweep,
	}
}
