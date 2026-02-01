package units

import (
	"testing"
	"time"
)

func TestIsTimezoneValid(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

	t.Run("UTC to America/New_York", func(t *testing.T) {
		out, err := ConvertTime(utcTime, "America/New_York")
		if err != nil {
			t.Fatalf("ConvertTime error: %v", err)
		}
		// Sept 13 is during daylight saving time, so EDT (-4 hours)
		expected := utcTime.Add(-4 * time.Hour)
		if out.Hour() != expected.Hour() {
			t.Errorf("ConvertTime hour = %d, want %d", out.Hour(), expected.Hour())
		}
	})

	t.Run("UTC to Europe/London", func(t *testing.T) {
		out, err := ConvertTime(utcTime, "Europe/London")
		if err != nil {
			t.Fatalf("ConvertTime error: %v", err)
		}
		// Sept 13 is during BST (+1 hour from UTC)
		if out.Hour() != 13 {
			t.Errorf("ConvertTime hour = %d, want 13", out.Hour())
		}
	})

	t.Run("Invalid timezone", func(t *testing.T) {
		_, err := ConvertTime(utcTime, "Invalid/Timezone")
		if err == nil {
			t.Error("ConvertTime should return error for invalid timezone")
		}
	})
}

func TestIsCommonTimezone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		timezone string
		expected bool
	}{
		{"UTC is common", "UTC", true},
		{"America/New_York is common", "America/New_York", true},
		{"Europe/Berlin is common", "Europe/Berlin", true},
		{"Asia/Seoul is common", "Asia/Seoul", true},
		{"Pacific/Auckland is common", "Pacific/Auckland", true},
		{"Asia/Singapore is common", "Asia/Singapore", true},
		{"Invalid timezone not common", "Invalid/Timezone", false},
		{"Empty not common", "", false},
		{"Valid but not common", "Africa/Ouagadougou", false}, // Valid IANA but not in common list
		{"Europe/London not in list", "Europe/London", false}, // Valid but not in curated list
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCommonTimezone(tt.timezone)
			if result != tt.expected {
				t.Errorf("IsCommonTimezone(%s) = %v, want %v", tt.timezone, result, tt.expected)
			}
		})
	}
}

func TestGetTimezoneLabel(t *testing.T) {
	t.Parallel()
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
