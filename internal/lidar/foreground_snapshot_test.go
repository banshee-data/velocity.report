package lidar

import (
	"math"
	"testing"
	"time"
)

func TestStoreForegroundSnapshot(t *testing.T) {
	// Clear the global map for test isolation
	fgMu.Lock()
	latestForegrounds = make(map[string]*ForegroundSnapshot)
	fgMu.Unlock()

	sensorID := "test-sensor"
	ts := time.Now()

	foreground := []PointPolar{
		{Azimuth: 0.0, Elevation: 0.0, Distance: 10.0, Intensity: 100, Channel: 1},
		{Azimuth: 90.0, Elevation: 5.0, Distance: 15.0, Intensity: 150, Channel: 2},
	}

	background := []PointPolar{
		{Azimuth: 45.0, Elevation: 0.0, Distance: 50.0, Intensity: 50, Channel: 4},
	}

	StoreForegroundSnapshot(sensorID, ts, foreground, background, 100, 2)

	fgMu.RLock()
	snap, ok := latestForegrounds[sensorID]
	fgMu.RUnlock()

	if !ok {
		t.Fatal("Expected snapshot to be stored")
	}

	if len(snap.ForegroundPoints) != 2 {
		t.Errorf("Expected 2 foreground points, got %d", len(snap.ForegroundPoints))
	}
}

func TestGetForegroundSnapshot(t *testing.T) {
	fgMu.Lock()
	latestForegrounds = make(map[string]*ForegroundSnapshot)
	fgMu.Unlock()

	sensorID := "test-sensor"
	foreground := []PointPolar{
		{Azimuth: 0.0, Elevation: 0.0, Distance: 10.0, Intensity: 100, Channel: 1},
	}

	StoreForegroundSnapshot(sensorID, time.Now(), foreground, nil, 10, 1)

	snap := GetForegroundSnapshot(sensorID)
	if snap == nil {
		t.Fatal("Expected non-nil snapshot")
	}

	if len(snap.ForegroundPoints) != 1 {
		t.Errorf("Expected 1 foreground point, got %d", len(snap.ForegroundPoints))
	}
}

func TestProjectPolars(t *testing.T) {
	tests := []struct {
		name     string
		input    []PointPolar
		expected int
	}{
		{
			name:     "empty input",
			input:    []PointPolar{},
			expected: 0,
		},
		{
			name: "single point",
			input: []PointPolar{
				{Azimuth: 0.0, Elevation: 0.0, Distance: 10.0, Intensity: 100, Channel: 1},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := projectPolars(tt.input)
			if tt.expected == 0 {
				if result != nil {
					t.Error("Expected nil result for empty input")
				}
				return
			}
			if len(result) != tt.expected {
				t.Errorf("Expected %d points, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestExportForegroundSnapshotToASC(t *testing.T) {
	snap := &ForegroundSnapshot{
		SensorID:  "test-sensor",
		Timestamp: time.Now(),
		ForegroundPoints: []ProjectedPoint{
			{X: 1.0, Y: 2.0, Z: 3.0, Intensity: 100},
		},
		TotalPoints:     10,
		ForegroundCount: 1,
	}

	path, err := ExportForegroundSnapshotToASC(snap)
	if err != nil {
		t.Fatalf("ExportForegroundSnapshotToASC failed: %v", err)
	}

	if path == "" {
		t.Error("Expected non-empty path")
	}
}

func TestExportForegroundSnapshotToASCNil(t *testing.T) {
	_, err := ExportForegroundSnapshotToASC(nil)
	if err == nil {
		t.Error("Expected error for nil snapshot")
	}
}

func TestProjectPolarsAngles(t *testing.T) {
	// Test 0° azimuth (North)
	points := []PointPolar{
		{Azimuth: 0.0, Elevation: 0.0, Distance: 10.0, Intensity: 100, Channel: 1},
	}
	result := projectPolars(points)
	if math.Abs(result[0].Y-10.0) > 0.001 {
		t.Errorf("Expected Y=10.0 for 0° azimuth, got %f", result[0].Y)
	}

	// Test 90° azimuth (East)
	points = []PointPolar{
		{Azimuth: 90.0, Elevation: 0.0, Distance: 10.0, Intensity: 100, Channel: 1},
	}
	result = projectPolars(points)
	if math.Abs(result[0].X-10.0) > 0.001 {
		t.Errorf("Expected X=10.0 for 90° azimuth, got %f", result[0].X)
	}
}
