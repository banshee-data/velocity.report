package units

import (
	"testing"
	"time"
)

func TestIsTimezoneValid(t *testing.T) {
	tests := []struct {
		name     string
		timezone string
		expected bool
	}{
		{"valid UTC", "UTC", true},
		{"valid US Eastern", "US/Eastern", true},
		{"invalid", "Invalid/Timezone", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := IsTimezoneValid(tt.timezone)
			if res != tt.expected {
				t.Errorf("IsTimezoneValid(%s) = %v, want %v", tt.timezone, res, tt.expected)
			}
		})
	}
}

func TestGetValidTimezonesString(t *testing.T) {
	res := GetValidTimezonesString()
	if res == "" {
		t.Fatal("GetValidTimezonesString returned empty string")
	}
	expected := []string{"UTC", "US/Eastern", "Europe/London"}
	for _, s := range expected {
		if !contains(res, s) {
			t.Fatalf("GetValidTimezonesString missing %s", s)
		}
	}
}

func TestConvertTime(t *testing.T) {
	utcTime := time.Date(2025, 9, 13, 12, 0, 0, 0, time.UTC)
	t.Run("UTC to UTC", func(t *testing.T) {
		out, err := ConvertTime(utcTime, "UTC")
		if err != nil {
			t.Fatalf("ConvertTime error: %v", err)
		}
		if !out.Equal(utcTime) {
			t.Fatalf("ConvertTime returned %v, want %v", out, utcTime)
		}
	})
}

func TestGetTimezoneLabel(t *testing.T) {
	if GetTimezoneLabel("UTC") != "UTC" {
		t.Fatalf("GetTimezoneLabel UTC mismatch")
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > len(substr) && (indexInString(s, substr) >= 0)))
}

func indexInString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
