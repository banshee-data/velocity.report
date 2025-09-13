package api

import (
	"math"
	"testing"
)

func TestConvertSpeed(t *testing.T) {
	tests := []struct {
		name     string
		speedMPS float64
		units    string
		expected float64
	}{
		{"10 m/s to mph", 10.0, "mph", 22.3694},
		{"10 m/s to kmph", 10.0, "kmph", 36.0},
		{"10 m/s to kph", 10.0, "kph", 36.0},
		{"10 m/s to mps", 10.0, "mps", 10.0},
		{"unknown units default to mps", 10.0, "unknown", 10.0},
		{"0 m/s to mph", 0.0, "mph", 0.0},
		{"highway speed 31.29 m/s to mph", 31.29, "mph", 70.0}, // ~70 mph
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertSpeed(tt.speedMPS, tt.units)
			if math.Abs(result-tt.expected) > 0.01 { // Allow small floating point differences
				t.Errorf("convertSpeed(%f, %s) = %f, want %f", tt.speedMPS, tt.units, result, tt.expected)
			}
		})
	}
}
