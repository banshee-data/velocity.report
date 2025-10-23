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
	expected := []string{"UTC", "America/New_York", "Europe/Dublin", "Asia/Seoul", "Asia/Singapore", "Europe/Athens", "Pacific/Bougainville"}
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
	tests := []struct {
		tz       string
		expected string
	}{
		{"UTC", "UTC (+00:00)"},
		{"America/New_York", "New York (-05:00/-04:00)"},
		{"Pacific/Chatham", "Chatham (+12:45/+13:45)"},
		{"Pacific/Niue", "Niue (-11:00)"},
		{"InvalidTimezone", "InvalidTimezone"}, // Should return the input if not found
	}

	for _, tt := range tests {
		result := GetTimezoneLabel(tt.tz)
		if result != tt.expected {
			t.Errorf("GetTimezoneLabel(%s) = %s, want %s", tt.tz, result, tt.expected)
		}
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
