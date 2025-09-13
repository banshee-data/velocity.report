package units

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
		{"10 m/s to mph", 10.0, MPH, 22.3694},
		{"10 m/s to kmph", 10.0, KMPH, 36.0},
		{"10 m/s to kph", 10.0, KPH, 36.0},
		{"10 m/s to mps", 10.0, MPS, 10.0},
		{"unknown units default to mps", 10.0, "unknown", 10.0},
		{"0 m/s to mph", 0.0, MPH, 0.0},
		{"highway speed 31.29 m/s to mph", 31.29, MPH, 70.0},  // ~70 mph
		{"city speed 13.89 m/s to kmph", 13.89, KMPH, 50.004}, // ~50 km/h
		{"walking speed 1.4 m/s to mph", 1.4, MPH, 3.13172},   // ~3.1 mph
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertSpeed(tt.speedMPS, tt.units)
			if math.Abs(result-tt.expected) > 0.01 { // Allow small floating point differences
				t.Errorf("ConvertSpeed(%f, %s) = %f, want %f", tt.speedMPS, tt.units, result, tt.expected)
			}
		})
	}
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		name     string
		unit     string
		expected bool
	}{
		{"valid mps", MPS, true},
		{"valid mph", MPH, true},
		{"valid kmph", KMPH, true},
		{"valid kph", KPH, true},
		{"invalid unit", "invalid", false},
		{"empty string", "", false},
		{"case sensitive", "MPH", false},
		{"case sensitive", "Mph", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValid(tt.unit)
			if result != tt.expected {
				t.Errorf("IsValid(%s) = %v, want %v", tt.unit, result, tt.expected)
			}
		})
	}
}

func TestGetValidUnitsString(t *testing.T) {
	expected := "mps, mph, kmph, kph"
	result := GetValidUnitsString()
	if result != expected {
		t.Errorf("GetValidUnitsString() = %s, want %s", result, expected)
	}
}

// Test conversion accuracy with known values
func TestConversionAccuracy(t *testing.T) {
	// Test exact conversions
	tests := []struct {
		name     string
		speedMPS float64
		unit     string
		expected float64
	}{
		// Test MPH conversion (1 m/s = 2.23694 mph)
		{"1 m/s to mph", 1.0, MPH, 2.23694},
		{"5 m/s to mph", 5.0, MPH, 11.1847},

		// Test KM/H conversion (1 m/s = 3.6 km/h)
		{"1 m/s to kmph", 1.0, KMPH, 3.6},
		{"5 m/s to kmph", 5.0, KMPH, 18.0},
		{"1 m/s to kph", 1.0, KPH, 3.6},

		// Test MPS (no conversion)
		{"5 m/s to mps", 5.0, MPS, 5.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertSpeed(tt.speedMPS, tt.unit)
			if math.Abs(result-tt.expected) > 0.0001 { // Very precise check
				t.Errorf("ConvertSpeed(%f, %s) = %f, want %f", tt.speedMPS, tt.unit, result, tt.expected)
			}
		})
	}
}
