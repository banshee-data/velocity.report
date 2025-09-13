package api

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
)

func TestConvertEventAPISpeed(t *testing.T) {
	tests := []struct {
		name     string
		speedMPS *float64
		units    string
		expected *float64
	}{
		{"nil speed", nil, "mph", nil},
		{"10 m/s to mph", floatPtr(10.0), "mph", floatPtr(22.3694)},
		{"10 m/s to kmph", floatPtr(10.0), "kmph", floatPtr(36.0)},
		{"0 m/s to mph", floatPtr(0.0), "mph", floatPtr(0.0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := db.EventAPI{Speed: tt.speedMPS}
			result := convertEventAPISpeed(event, tt.units)

			if tt.expected == nil {
				if result.Speed != nil {
					t.Errorf("convertEventAPISpeed() speed = %v, want nil", result.Speed)
				}
			} else {
				if result.Speed == nil {
					t.Errorf("convertEventAPISpeed() speed = nil, want %f", *tt.expected)
				} else if math.Abs(*result.Speed-*tt.expected) > 0.01 {
					t.Errorf("convertEventAPISpeed() speed = %f, want %f", *result.Speed, *tt.expected)
				}
			}
		})
	}
}

// Helper function to create float64 pointers
func floatPtr(f float64) *float64 {
	return &f
}
