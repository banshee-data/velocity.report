package units

import (
	"math"
	"testing"
)

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
		{"empty unit", "", false},
		{"uppercase MPS", "MPS", false}, // Case-sensitive
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
	result := GetValidUnitsString()
	expected := "mps, mph, kmph, kph"
	if result != expected {
		t.Errorf("GetValidUnitsString() = %s, want %s", result, expected)
	}
}

func TestConvertSpeed(t *testing.T) {
	tests := []struct {
		name     string
		speedMPS float64
		unit     string
		expected float64
	}{
		// Test MPS (no conversion)
		{"0 m/s to mps", 0.0, MPS, 0.0},
		{"1 m/s to mps", 1.0, MPS, 1.0},
		{"5 m/s to mps", 5.0, MPS, 5.0},

		// Test MPH conversion (1 m/s = 2.23694 mph)
		{"0 m/s to mph", 0.0, MPH, 0.0},
		{"1 m/s to mph", 1.0, MPH, 2.2369362920544},
		{"5 m/s to mph", 5.0, MPH, 11.184681460272},

		// Test KM/H conversion (1 m/s = 3.6 km/h)
		{"0 m/s to kmph", 0.0, KMPH, 0.0},
		{"1 m/s to kmph", 1.0, KMPH, 3.6},
		{"5 m/s to kmph", 5.0, KMPH, 18.0},
		{"1 m/s to kph", 1.0, KPH, 3.6},

		// Test unknown unit (falls back to MPS)
		{"1 m/s to unknown", 1.0, "unknown", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertSpeed(tt.speedMPS, tt.unit)
			if math.Abs(result-tt.expected) > 1e-10 {
				t.Errorf("ConvertSpeed(%f, %s) = %f, want %f", tt.speedMPS, tt.unit, result, tt.expected)
			}
		})
	}
}

func TestConvertToMPS(t *testing.T) {
	tests := []struct {
		name     string
		speed    float64
		fromUnit string
		expected float64
	}{
		// Test MPS (no conversion)
		{"0 mps to mps", 0.0, MPS, 0.0},
		{"5 mps to mps", 5.0, MPS, 5.0},

		// Test MPH to MPS
		{"0 mph to mps", 0.0, MPH, 0.0},
		{"10 mph to mps", 10.0, MPH, 10.0 / 2.2369362920544},
		{"22.3694 mph to mps", 22.369362920544, MPH, 10.0}, // ~10 m/s

		// Test KMPH to MPS
		{"0 kmph to mps", 0.0, KMPH, 0.0},
		{"3.6 kmph to mps", 3.6, KMPH, 1.0},
		{"36 kmph to mps", 36.0, KMPH, 10.0},

		// Test KPH to MPS
		{"3.6 kph to mps", 3.6, KPH, 1.0},

		// Test unknown unit (falls back to returning input)
		{"5 unknown to mps", 5.0, "unknown", 5.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToMPS(tt.speed, tt.fromUnit)
			if math.Abs(result-tt.expected) > 1e-10 {
				t.Errorf("ConvertToMPS(%f, %s) = %f, want %f", tt.speed, tt.fromUnit, result, tt.expected)
			}
		})
	}
}

// Test round-trip conversions
func TestRoundTripConversions(t *testing.T) {
	originalMPS := 15.5

	// Test MPH round-trip
	mph := ConvertSpeed(originalMPS, MPH)
	backToMPS := ConvertToMPS(mph, MPH)
	if math.Abs(backToMPS-originalMPS) > 1e-10 {
		t.Errorf("MPH round-trip: started %f m/s, got %f m/s", originalMPS, backToMPS)
	}

	// Test KMPH round-trip
	kmph := ConvertSpeed(originalMPS, KMPH)
	backToMPS = ConvertToMPS(kmph, KMPH)
	if math.Abs(backToMPS-originalMPS) > 1e-10 {
		t.Errorf("KMPH round-trip: started %f m/s, got %f m/s", originalMPS, backToMPS)
	}

	// Test KPH round-trip
	kph := ConvertSpeed(originalMPS, KPH)
	backToMPS = ConvertToMPS(kph, KPH)
	if math.Abs(backToMPS-originalMPS) > 1e-10 {
		t.Errorf("KPH round-trip: started %f m/s, got %f m/s", originalMPS, backToMPS)
	}
}
