package units

import (
	"math"
	"testing"
)

func TestConvertToMPS(t *testing.T) {
	// 10 mph -> ~4.4704 m/s
	mphVal := 10.0
	mps := ConvertToMPS(mphVal, MPH)
	if !(mps > 4.47 && mps < 4.48) {
		t.Fatalf("unexpected ConvertToMPS result: %v", mps)
	}

	// Round-trip: convert to mps then back to mph should be approximately the same
	back := ConvertSpeed(mps, MPH)
	if math.Abs(back-mphVal) > 1e-3 {
		t.Fatalf("round-trip mismatch: started %v mph, got %v mph", mphVal, back)
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
