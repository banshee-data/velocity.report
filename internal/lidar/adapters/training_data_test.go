package adapters

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func TestExportForegroundFrame(t *testing.T) {
	polarPoints := []l4perception.PointPolar{
		{Distance: 10, Azimuth: 0, Elevation: 0},
		{Distance: 15, Azimuth: 90, Elevation: 5},
		{Distance: 20, Azimuth: 180, Elevation: -5},
	}
	mask := []bool{true, false, true} // Points 0 and 2 are foreground

	frame := ExportForegroundFrame(polarPoints, mask, "sensor1", time.Now())

	if frame.SensorID != "sensor1" {
		t.Errorf("expected sensor1, got %s", frame.SensorID)
	}
	if frame.ForegroundCount() != 2 {
		t.Errorf("expected 2 foreground points, got %d", frame.ForegroundCount())
	}
	if frame.TotalPoints != 3 {
		t.Errorf("expected 3 total points, got %d", frame.TotalPoints)
	}
	if frame.BackgroundPoints != 1 {
		t.Errorf("expected 1 background point, got %d", frame.BackgroundPoints)
	}
}

func TestForegroundFrame_ForegroundFraction(t *testing.T) {
	tests := []struct {
		total    int
		fg       int
		expected float64
	}{
		{100, 20, 0.2},
		{100, 0, 0.0},
		{0, 0, 0.0},
		{50, 50, 1.0},
	}

	for _, tt := range tests {
		frame := &ForegroundFrame{
			TotalPoints:      tt.total,
			ForegroundPoints: make([]l4perception.PointPolar, tt.fg),
		}
		fraction := frame.ForegroundFraction()
		if fraction != tt.expected {
			t.Errorf("total=%d, fg=%d: expected fraction %f, got %f", tt.total, tt.fg, tt.expected, fraction)
		}
	}
}

func TestEncodeForegroundBlob(t *testing.T) {
	points := []l4perception.PointPolar{
		{Distance: 10.5, Azimuth: 45.25, Elevation: -5.5, Intensity: 200, Channel: 5},
		{Distance: 100.0, Azimuth: 180.0, Elevation: 10.0, Intensity: 100, Channel: 20},
	}

	blob := EncodeForegroundBlob(points)

	if len(blob) != 16 { // 2 points * 8 bytes
		t.Errorf("expected blob size 16, got %d", len(blob))
	}

	// Decode and verify
	decoded := DecodeForegroundBlob(blob)
	if len(decoded) != 2 {
		t.Errorf("expected 2 decoded points, got %d", len(decoded))
	}

	// Check first point (allowing for centimeter precision)
	if abs(decoded[0].Distance-10.5) > 0.01 {
		t.Errorf("distance mismatch: expected 10.5, got %f", decoded[0].Distance)
	}
	if abs(decoded[0].Azimuth-45.25) > 0.01 {
		t.Errorf("azimuth mismatch: expected 45.25, got %f", decoded[0].Azimuth)
	}
	if abs(decoded[0].Elevation-(-5.5)) > 0.01 {
		t.Errorf("elevation mismatch: expected -5.5, got %f", decoded[0].Elevation)
	}
	if decoded[0].Intensity != 200 {
		t.Errorf("intensity mismatch: expected 200, got %d", decoded[0].Intensity)
	}
	if decoded[0].Channel != 5 {
		t.Errorf("channel mismatch: expected 5, got %d", decoded[0].Channel)
	}

	// Check second point
	if abs(decoded[1].Distance-100.0) > 0.01 {
		t.Errorf("distance mismatch: expected 100.0, got %f", decoded[1].Distance)
	}
	if abs(decoded[1].Azimuth-180.0) > 0.01 {
		t.Errorf("azimuth mismatch: expected 180.0, got %f", decoded[1].Azimuth)
	}
}

func TestDecodeForegroundBlob_InvalidSize(t *testing.T) {
	// Blob not divisible by 8
	blob := make([]byte, 15)
	decoded := DecodeForegroundBlob(blob)

	if decoded != nil {
		t.Error("expected nil for invalid blob size")
	}
}

func TestDecodeForegroundBlob_Empty(t *testing.T) {
	blob := []byte{}
	decoded := DecodeForegroundBlob(blob)

	if len(decoded) != 0 {
		t.Error("expected empty slice for empty blob")
	}
}

func TestEncodeForegroundBlob_NegativeElevation(t *testing.T) {
	// Test negative elevation handling
	points := []l4perception.PointPolar{
		{Distance: 5.0, Azimuth: 0, Elevation: -15.0, Intensity: 100, Channel: 1},
	}

	blob := EncodeForegroundBlob(points)
	decoded := DecodeForegroundBlob(blob)

	if abs(decoded[0].Elevation-(-15.0)) > 0.01 {
		t.Errorf("negative elevation lost: expected -15.0, got %f", decoded[0].Elevation)
	}
}

func TestDefaultTrainingDataFilter(t *testing.T) {
	filter := DefaultTrainingDataFilter()

	if filter.MinForeground != 10 {
		t.Errorf("expected MinForeground=10, got %d", filter.MinForeground)
	}
	if filter.AnnotationOnly {
		t.Error("expected AnnotationOnly=false")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
