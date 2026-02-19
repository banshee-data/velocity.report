// Package monitor provides mock implementations for testing.
package monitor

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// BackgroundManagerProvider abstracts background manager access for testing.
// Production code uses the global registry; tests can inject mocks.
type BackgroundManagerProvider interface {
	GetBackgroundManager(sensorID string) BackgroundManagerInterface
}

// BackgroundManagerInterface defines the subset of BackgroundManager methods
// used by the WebServer for chart data and configuration.
type BackgroundManagerInterface interface {
	// GetGridCells returns exported cells for polar chart rendering
	GetGridCells() []l3grid.ExportedCell
	// GetGridHeatmap returns aggregated heatmap buckets
	GetGridHeatmap(azimuthBucketDeg float64, settledThreshold uint32) *l3grid.GridHeatmap
	// GetParams returns the current background parameters
	GetParams() l3grid.BackgroundParams
	// SetParams updates background parameters
	SetParams(p l3grid.BackgroundParams) error
	// SetNoiseRelativeFraction updates the noise relative fraction
	SetNoiseRelativeFraction(v float32) error
	// GetGrid returns the underlying grid (nil check used)
	GetGrid() *l3grid.BackgroundGrid
	// IsDiagnosticsEnabled returns whether diagnostics are enabled
	IsDiagnosticsEnabled() bool
	// SetDiagnosticsEnabled enables or disables diagnostics
	SetDiagnosticsEnabled(enabled bool)
}

// DefaultBackgroundManagerProvider uses the global lidar registry.
type DefaultBackgroundManagerProvider struct{}

// GetBackgroundManager returns the manager from the global registry.
func (p *DefaultBackgroundManagerProvider) GetBackgroundManager(sensorID string) BackgroundManagerInterface {
	bm := l3grid.GetBackgroundManager(sensorID)
	if bm == nil {
		return nil
	}
	return &backgroundManagerWrapper{bm: bm}
}

// backgroundManagerWrapper wraps *l3grid.BackgroundManager to implement the interface.
type backgroundManagerWrapper struct {
	bm *l3grid.BackgroundManager
}

func (w *backgroundManagerWrapper) GetGridCells() []l3grid.ExportedCell {
	return w.bm.GetGridCells()
}

func (w *backgroundManagerWrapper) GetGridHeatmap(azimuthBucketDeg float64, settledThreshold uint32) *l3grid.GridHeatmap {
	return w.bm.GetGridHeatmap(azimuthBucketDeg, settledThreshold)
}

func (w *backgroundManagerWrapper) GetParams() l3grid.BackgroundParams {
	return w.bm.GetParams()
}

func (w *backgroundManagerWrapper) SetParams(p l3grid.BackgroundParams) error {
	return w.bm.SetParams(p)
}

func (w *backgroundManagerWrapper) SetNoiseRelativeFraction(v float32) error {
	return w.bm.SetNoiseRelativeFraction(v)
}

func (w *backgroundManagerWrapper) GetGrid() *l3grid.BackgroundGrid {
	return w.bm.Grid
}

func (w *backgroundManagerWrapper) IsDiagnosticsEnabled() bool {
	return w.bm.EnableDiagnostics
}

func (w *backgroundManagerWrapper) SetDiagnosticsEnabled(enabled bool) {
	w.bm.EnableDiagnostics = enabled
}

// MockBackgroundManager provides a testable implementation of BackgroundManagerInterface.
type MockBackgroundManager struct {
	Cells          []l3grid.ExportedCell
	Heatmap        *l3grid.GridHeatmap
	Params         l3grid.BackgroundParams
	Grid           *l3grid.BackgroundGrid
	Diagnostics    bool
	SetParamsErr   error
	SetNoiseErr    error
	SetParamsCalls []l3grid.BackgroundParams
	SetNoiseCalls  []float32
}

// GetGridCells returns the configured cells.
func (m *MockBackgroundManager) GetGridCells() []l3grid.ExportedCell {
	return m.Cells
}

// GetGridHeatmap returns the configured heatmap.
func (m *MockBackgroundManager) GetGridHeatmap(azimuthBucketDeg float64, settledThreshold uint32) *l3grid.GridHeatmap {
	return m.Heatmap
}

// GetParams returns the configured params.
func (m *MockBackgroundManager) GetParams() l3grid.BackgroundParams {
	return m.Params
}

// SetParams records the call and returns configured error.
func (m *MockBackgroundManager) SetParams(p l3grid.BackgroundParams) error {
	m.SetParamsCalls = append(m.SetParamsCalls, p)
	if m.SetParamsErr != nil {
		return m.SetParamsErr
	}
	m.Params = p
	return nil
}

// SetNoiseRelativeFraction records the call and returns configured error.
func (m *MockBackgroundManager) SetNoiseRelativeFraction(v float32) error {
	m.SetNoiseCalls = append(m.SetNoiseCalls, v)
	if m.SetNoiseErr != nil {
		return m.SetNoiseErr
	}
	m.Params.NoiseRelativeFraction = v
	return nil
}

// GetGrid returns the configured grid.
func (m *MockBackgroundManager) GetGrid() *l3grid.BackgroundGrid {
	return m.Grid
}

// IsDiagnosticsEnabled returns the diagnostics flag.
func (m *MockBackgroundManager) IsDiagnosticsEnabled() bool {
	return m.Diagnostics
}

// SetDiagnosticsEnabled sets the diagnostics flag.
func (m *MockBackgroundManager) SetDiagnosticsEnabled(enabled bool) {
	m.Diagnostics = enabled
}

// MockBackgroundManagerProvider provides mock managers for testing.
type MockBackgroundManagerProvider struct {
	Managers map[string]BackgroundManagerInterface
}

// GetBackgroundManager returns a mock manager for the sensor ID.
func (p *MockBackgroundManagerProvider) GetBackgroundManager(sensorID string) BackgroundManagerInterface {
	if p.Managers == nil {
		return nil
	}
	return p.Managers[sensorID]
}

// NewMockBackgroundManagerProvider creates a provider with the given managers.
func NewMockBackgroundManagerProvider(managers map[string]BackgroundManagerInterface) *MockBackgroundManagerProvider {
	return &MockBackgroundManagerProvider{Managers: managers}
}
