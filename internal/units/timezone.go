package units

import (
	"fmt"
	"strings"
	"time"
)

// CommonTimezones contains a curated list of commonly used timezones from the tz database
// These are verified to exist in the system's tz database
var CommonTimezones = []string{
	"UTC",
	"US/Eastern",
	"US/Central",
	"US/Mountain",
	"US/Pacific",
	"US/Alaska",
	"US/Hawaii",
	"America/New_York",
	"America/Chicago",
	"America/Denver",
	"America/Los_Angeles",
	"America/Anchorage",
	"America/Honolulu",
	"Europe/London",
	"Europe/Paris",
	"Europe/Berlin",
	"Europe/Rome",
	"Europe/Madrid",
	"Europe/Amsterdam",
	"Asia/Tokyo",
	"Asia/Shanghai",
	"Asia/Kolkata",
	"Asia/Dubai",
	"Australia/Sydney",
	"Australia/Melbourne",
	"Australia/Perth",
	"Pacific/Auckland",
	"America/Toronto",
	"America/Vancouver",
	"America/Mexico_City",
	"America/Sao_Paulo",
	"Africa/Cairo",
	"Africa/Johannesburg",
}

// IsTimezoneValid checks if the given timezone is valid by attempting to load it from the tz database
// This validates against the actual system tz database rather than a hardcoded list
func IsTimezoneValid(tz string) bool {
	if tz == "" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// IsCommonTimezone checks if the given timezone is in our curated list of common timezones
func IsCommonTimezone(tz string) bool {
	for _, commonTz := range CommonTimezones {
		if tz == commonTz {
			return true
		}
	}
	return false
}

// GetValidTimezonesString returns a comma-separated string of common timezones for error messages
func GetValidTimezonesString() string {
	return strings.Join(CommonTimezones, ", ")
}

// ConvertTime converts a UTC time to the specified timezone
// Database stores all times in UTC, this function converts them for display
func ConvertTime(utcTime time.Time, targetTimezone string) (time.Time, error) {
	if targetTimezone == "UTC" {
		return utcTime, nil // No conversion needed
	}

	// Load the target timezone location from the tz database
	loc, err := time.LoadLocation(targetTimezone)
	if err != nil {
		return utcTime, fmt.Errorf("failed to load timezone %s: %w", targetTimezone, err)
	}

	// Convert UTC time to the target timezone
	return utcTime.In(loc), nil
}

// GetTimezoneLabel returns a human-readable label for the timezone
func GetTimezoneLabel(tz string) string {
	// Map of timezone IDs to human-readable labels
	labels := map[string]string{
		"UTC":                 "UTC",
		"US/Eastern":          "US Eastern (EST/EDT)",
		"US/Central":          "US Central (CST/CDT)",
		"US/Mountain":         "US Mountain (MST/MDT)",
		"US/Pacific":          "US Pacific (PST/PDT)",
		"US/Alaska":           "US Alaska (AKST/AKDT)",
		"US/Hawaii":           "US Hawaii (HST)",
		"America/New_York":    "New York (EST/EDT)",
		"America/Chicago":     "Chicago (CST/CDT)",
		"America/Denver":      "Denver (MST/MDT)",
		"America/Los_Angeles": "Los Angeles (PST/PDT)",
		"America/Anchorage":   "Anchorage (AKST/AKDT)",
		"America/Honolulu":    "Honolulu (HST)",
		"Europe/London":       "London (GMT/BST)",
		"Europe/Paris":        "Paris (CET/CEST)",
		"Europe/Berlin":       "Berlin (CET/CEST)",
		"Europe/Rome":         "Rome (CET/CEST)",
		"Europe/Madrid":       "Madrid (CET/CEST)",
		"Europe/Amsterdam":    "Amsterdam (CET/CEST)",
		"Asia/Tokyo":          "Tokyo (JST)",
		"Asia/Shanghai":       "Shanghai (CST)",
		"Asia/Kolkata":        "Mumbai/Kolkata (IST)",
		"Asia/Dubai":          "Dubai (GST)",
		"Australia/Sydney":    "Sydney (AEST/AEDT)",
		"Australia/Melbourne": "Melbourne (AEST/AEDT)",
		"Australia/Perth":     "Perth (AWST)",
		"Pacific/Auckland":    "Auckland (NZST/NZDT)",
		"America/Toronto":     "Toronto (EST/EDT)",
		"America/Vancouver":   "Vancouver (PST/PDT)",
		"America/Mexico_City": "Mexico City (CST/CDT)",
		"America/Sao_Paulo":   "São Paulo (BRT)",
		"Africa/Cairo":        "Cairo (EET)",
		"Africa/Johannesburg": "Johannesburg (SAST)",
	}

	if label, exists := labels[tz]; exists {
		return label
	}
	return tz // Return the timezone ID if no label is defined
}
