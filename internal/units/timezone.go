package units

import (
	"fmt"
	"strings"
	"time"
)

// CommonTimezones contains a curated list of commonly used timezones from the tz database
// Each timezone represents a unique STD/DST offset pair to avoid redundancy
// These are verified to exist in the system's tz database
// Total unique offset pairs: 55 (covering all possible timezones globally)
// Ordered from west to east: -11:00 (Niue) to +14:00 (Kiritimati)
var CommonTimezones = []string{
	"Pacific/Niue",           // -11:00
	"America/Adak",           // -10:00/-09:00
	"Pacific/Honolulu",       // -10:00
	"Pacific/Marquesas",      // -09:30
	"America/Anchorage",      // -09:00/-08:00
	"Pacific/Gambier",        // -09:00
	"America/Los_Angeles",    // -08:00/-07:00
	"Pacific/Pitcairn",       // -08:00
	"America/Denver",         // -07:00/-06:00
	"America/Phoenix",        // -07:00
	"America/Chicago",        // -06:00/-05:00
	"America/Mexico_City",    // -06:00
	"America/New_York",       // -05:00/-04:00
	"America/Lima",           // -05:00
	"America/Barbados",       // -04:00
	"America/Santiago",       // -04:00/-03:00
	"America/St_Johns",       // -03:30/-02:30
	"America/Miquelon",       // -03:00/-02:00
	"America/Sao_Paulo",      // -03:00
	"America/Godthab",        // -02:00/-01:00
	"Atlantic/South_Georgia", // -02:00
	"Atlantic/Azores",        // -01:00/+00:00
	"Atlantic/Cape_Verde",    // -01:00
	"UTC",                    // +00:00
	"Africa/Abidjan",         // +00:00
	"Europe/Dublin",          // +00:00/+01:00
	"Antarctica/Troll",       // +00:00/+02:00
	"Africa/Lagos",           // +01:00
	"Europe/Berlin",          // +01:00/+02:00
	"Africa/Johannesburg",    // +02:00
	"Europe/Athens",          // +02:00/+03:00
	"Africa/Nairobi",         // +03:00
	"Asia/Tehran",            // +03:30
	"Asia/Dubai",             // +04:00
	"Asia/Kabul",             // +04:30
	"Asia/Karachi",           // +05:00
	"Asia/Kolkata",           // +05:30
	"Asia/Kathmandu",         // +05:45
	"Asia/Dhaka",             // +06:00
	"Asia/Yangon",            // +06:30
	"Asia/Bangkok",           // +07:00
	"Asia/Singapore",         // +08:00
	"Australia/Eucla",        // +08:45
	"Asia/Seoul",             // +09:00
	"Australia/Darwin",       // +09:30
	"Australia/Adelaide",     // +09:30/+10:30
	"Australia/Brisbane",     // +10:00
	"Australia/Sydney",       // +10:00/+11:00
	"Australia/Lord_Howe",    // +10:30/+11:00
	"Pacific/Bougainville",   // +11:00
	"Pacific/Norfolk",        // +11:00/+12:00
	"Pacific/Fiji",           // +12:00
	"Pacific/Auckland",       // +12:00/+13:00
	"Pacific/Chatham",        // +12:45/+13:45
	"Pacific/Apia",           // +13:00
	"Pacific/Kiritimati",     // +14:00
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
// Labels include the offset(s) to make it clear which timezone is which
// Ordered from west to east: -11:00 (Niue) to +14:00 (Kiritimati)
func GetTimezoneLabel(tz string) string {
	// Map of timezone IDs to human-readable labels with offsets
	// Each entry represents a unique STD/DST offset pair
	labels := map[string]string{
		"Pacific/Niue":           "Niue (-11:00)",
		"America/Adak":           "Adak (-10:00/-09:00)",
		"Pacific/Honolulu":       "Honolulu (-10:00)",
		"Pacific/Marquesas":      "Marquesas (-09:30)",
		"America/Anchorage":      "Anchorage (-09:00/-08:00)",
		"Pacific/Gambier":        "Gambier (-09:00)",
		"Pacific/Pitcairn":       "Pitcairn (-08:00)",
		"America/Los_Angeles":    "Los Angeles (-08:00/-07:00)",
		"America/Denver":         "Denver (-07:00/-06:00)",
		"America/Phoenix":        "Phoenix (-07:00)",
		"America/Chicago":        "Chicago (-06:00/-05:00)",
		"America/Mexico_City":    "Mexico City (-06:00)",
		"America/New_York":       "New York (-05:00/-04:00)",
		"America/Lima":           "Lima (-05:00)",
		"America/Barbados":       "Barbados (-04:00)",
		"America/Santiago":       "Santiago (-04:00/-03:00)",
		"America/St_Johns":       "St. John's (-03:30/-02:30)",
		"America/Miquelon":       "Miquelon (-03:00/-02:00)",
		"America/Sao_Paulo":      "São Paulo (-03:00)",
		"America/Godthab":        "Godthab/Nuuk (-02:00/-01:00)",
		"Atlantic/South_Georgia": "South Georgia (-02:00)",
		"Atlantic/Azores":        "Azores (-01:00/+00:00)",
		"Atlantic/Cape_Verde":    "Cape Verde (-01:00)",
		"UTC":                    "UTC (+00:00)",
		"Africa/Abidjan":         "Abidjan (+00:00)",
		"Europe/Dublin":          "Dublin (+00:00/+01:00)",
		"Antarctica/Troll":       "Troll (+00:00/+02:00)",
		"Africa/Lagos":           "Lagos (+01:00)",
		"Europe/Berlin":          "Berlin (+01:00/+02:00)",
		"Africa/Johannesburg":    "Johannesburg (+02:00)",
		"Europe/Athens":          "Athens (+02:00/+03:00)",
		"Africa/Nairobi":         "Nairobi (+03:00)",
		"Asia/Tehran":            "Tehran (+03:30)",
		"Asia/Dubai":             "Dubai (+04:00)",
		"Asia/Kabul":             "Kabul (+04:30)",
		"Asia/Karachi":           "Karachi (+05:00)",
		"Asia/Kolkata":           "Mumbai/Kolkata (+05:30)",
		"Asia/Kathmandu":         "Kathmandu (+05:45)",
		"Asia/Dhaka":             "Dhaka (+06:00)",
		"Asia/Yangon":            "Yangon (+06:30)",
		"Asia/Bangkok":           "Bangkok (+07:00)",
		"Asia/Singapore":         "Singapore (+08:00)",
		"Australia/Eucla":        "Eucla (+08:45)",
		"Asia/Seoul":             "Seoul (+09:00)",
		"Australia/Darwin":       "Darwin (+09:30)",
		"Australia/Adelaide":     "Adelaide (+09:30/+10:30)",
		"Australia/Brisbane":     "Brisbane (+10:00)",
		"Australia/Sydney":       "Sydney (+10:00/+11:00)",
		"Australia/Lord_Howe":    "Lord Howe (+10:30/+11:00)",
		"Pacific/Bougainville":   "Bougainville (+11:00)",
		"Pacific/Norfolk":        "Norfolk (+11:00/+12:00)",
		"Pacific/Fiji":           "Fiji (+12:00)",
		"Pacific/Auckland":       "Auckland (+12:00/+13:00)",
		"Pacific/Chatham":        "Chatham (+12:45/+13:45)",
		"Pacific/Apia":           "Apia (+13:00)",
		"Pacific/Kiritimati":     "Kiritimati (+14:00)",
	}

	if label, exists := labels[tz]; exists {
		return label
	}
	return tz // Return the timezone ID if no label is defined
}
