package monitor

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// mockError implements error for testing
type mockError struct {
	msg string
}

func (e *mockError) Error() string { return e.msg }

func TestMockBackgroundManager_GetGridCells(t *testing.T) {
	cells := []lidar.ExportedCell{
		{AzimuthDeg: 0, Range: 10, TimesSeen: 5},
		{AzimuthDeg: 90, Range: 20, TimesSeen: 10},
	}

	mock := &MockBackgroundManager{Cells: cells}

	result := mock.GetGridCells()
	if len(result) != 2 {
		t.Errorf("got %d cells, want 2", len(result))
	}
	if result[0].TimesSeen != 5 {
		t.Errorf("got TimesSeen %d, want 5", result[0].TimesSeen)
	}
}

func TestMockBackgroundManager_GetGridHeatmap(t *testing.T) {
	heatmap := &lidar.GridHeatmap{
		SensorID: "test-sensor",
		Buckets: []lidar.CoarseBucket{
			{Ring: 0, FilledCells: 10},
		},
	}

	mock := &MockBackgroundManager{Heatmap: heatmap}

	result := mock.GetGridHeatmap(3.0, 5)
	if result == nil {
		t.Fatal("GetGridHeatmap returned nil")
	}
	if result.SensorID != "test-sensor" {
		t.Errorf("got SensorID %q, want %q", result.SensorID, "test-sensor")
	}
}

func TestMockBackgroundManager_GetParams(t *testing.T) {
	params := lidar.BackgroundParams{
		NoiseRelativeFraction: 0.5,
	}

	mock := &MockBackgroundManager{Params: params}

	result := mock.GetParams()
	if result.NoiseRelativeFraction != 0.5 {
		t.Errorf("got NoiseRelativeFraction %v, want 0.5", result.NoiseRelativeFraction)
	}
}

func TestMockBackgroundManager_SetParams(t *testing.T) {
	mock := &MockBackgroundManager{}

	newParams := lidar.BackgroundParams{NoiseRelativeFraction: 0.7}
	err := mock.SetParams(newParams)
	if err != nil {
		t.Errorf("SetParams returned error: %v", err)
	}

	if len(mock.SetParamsCalls) != 1 {
		t.Errorf("got %d SetParams calls, want 1", len(mock.SetParamsCalls))
	}
	if mock.Params.NoiseRelativeFraction != 0.7 {
		t.Errorf("params not updated")
	}
}

func TestMockBackgroundManager_SetParamsError(t *testing.T) {
	mock := &MockBackgroundManager{
		SetParamsErr: errMockError,
	}

	err := mock.SetParams(lidar.BackgroundParams{})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

var errMockError = &mockError{msg: "mock error"}

func TestMockBackgroundManager_SetNoiseRelativeFraction(t *testing.T) {
	mock := &MockBackgroundManager{}

	err := mock.SetNoiseRelativeFraction(0.3)
	if err != nil {
		t.Errorf("SetNoiseRelativeFraction returned error: %v", err)
	}

	if len(mock.SetNoiseCalls) != 1 {
		t.Errorf("got %d calls, want 1", len(mock.SetNoiseCalls))
	}
	if mock.SetNoiseCalls[0] != 0.3 {
		t.Errorf("got value %v, want 0.3", mock.SetNoiseCalls[0])
	}
}

func TestMockBackgroundManager_Diagnostics(t *testing.T) {
	mock := &MockBackgroundManager{Diagnostics: false}

	if mock.IsDiagnosticsEnabled() {
		t.Error("expected diagnostics to be disabled")
	}

	mock.SetDiagnosticsEnabled(true)
	if !mock.IsDiagnosticsEnabled() {
		t.Error("expected diagnostics to be enabled")
	}
}

func TestMockBackgroundManagerProvider(t *testing.T) {
	mock1 := &MockBackgroundManager{Diagnostics: true}
	mock2 := &MockBackgroundManager{Diagnostics: false}

	provider := NewMockBackgroundManagerProvider(map[string]BackgroundManagerInterface{
		"sensor-1": mock1,
		"sensor-2": mock2,
	})

	// Test getting existing sensors
	result1 := provider.GetBackgroundManager("sensor-1")
	if result1 == nil {
		t.Fatal("expected non-nil manager for sensor-1")
	}
	if !result1.IsDiagnosticsEnabled() {
		t.Error("expected diagnostics enabled for sensor-1")
	}

	result2 := provider.GetBackgroundManager("sensor-2")
	if result2 == nil {
		t.Fatal("expected non-nil manager for sensor-2")
	}
	if result2.IsDiagnosticsEnabled() {
		t.Error("expected diagnostics disabled for sensor-2")
	}

	// Test getting non-existent sensor
	result3 := provider.GetBackgroundManager("sensor-3")
	if result3 != nil {
		t.Error("expected nil for non-existent sensor")
	}
}

func TestMockBackgroundManagerProvider_NilManagers(t *testing.T) {
	provider := &MockBackgroundManagerProvider{Managers: nil}

	result := provider.GetBackgroundManager("any-sensor")
	if result != nil {
		t.Error("expected nil when Managers map is nil")
	}
}

func TestDefaultBackgroundManagerProvider_NilManager(t *testing.T) {
	provider := &DefaultBackgroundManagerProvider{}

	// Getting a non-existent sensor should return nil
	result := provider.GetBackgroundManager("non-existent-sensor-xyz")
	if result != nil {
		t.Error("expected nil for non-existent sensor")
	}
}
