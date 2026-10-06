package server

import (
	"context"
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
// Clears lidarSweep so stale capability is not advertised during transitions.
func (cp *capabilitiesProvider) SetLidarStarting() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lidarState = "starting"
	cp.lidarSweep = false
}

// SetLidarError marks the LiDAR subsystem as having encountered an error.
// Clears lidarSweep because the subsystem cannot serve sweep requests in
// an error state.
func (cp *capabilitiesProvider) SetLidarError() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lidarState = "error"
	cp.lidarSweep = false
}

// SetLidarDisabled marks the LiDAR subsystem as disabled.
func (cp *capabilitiesProvider) SetLidarDisabled() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lidarState = "disabled"
	cp.lidarSweep = false
}

// Capabilities returns the current sensor state snapshot.
// Radar is always present as "default". LiDAR appears as "default"
// when enabled (state != "disabled"), otherwise the map is empty.
func (cp *capabilitiesProvider) Capabilities() api.Capabilities {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	caps := api.Capabilities{
		Radar: map[string]api.SensorStatus{
			"default": {Enabled: true, Status: "receiving"},
		},
		Lidar: map[string]api.LidarSensorStatus{},
	}

	if cp.lidarState != "disabled" {
		caps.Lidar["default"] = api.LidarSensorStatus{
			SensorStatus: api.SensorStatus{
				Enabled: true,
				Status:  cp.lidarState,
			},
			Sweep: cp.lidarSweep,
		}
	}

	return caps
}

// lidarStarter is the part of the LiDAR server that runLidarServer needs:
// a hook for when it is serving, and Start, whose result says whether it
// started. The capabilities report that outcome.
type lidarStarter interface {
	SetOnReady(fn func())
	Start(ctx context.Context) error
}

// runLidarServer runs the LiDAR server until ctx ends and reports its
// startup outcome: ready once it is serving, error if it cannot start (a
// port already in use, say), so /api/capabilities never stays at
// "starting". Sweeps are advertised with ready because the sweep runner is
// wired whenever LiDAR is enabled.
func runLidarServer(ctx context.Context, srv lidarStarter, caps *capabilitiesProvider) error {
	srv.SetOnReady(func() { caps.SetLidarReady(true) })
	if err := srv.Start(ctx); err != nil {
		caps.SetLidarError()
		return err
	}
	return nil
}
