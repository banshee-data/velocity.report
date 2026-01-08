package radar

import (
	"testing"
)

func TestAllowedCommands(t *testing.T) {
	if len(AllowedCommands) == 0 {
		t.Fatal("AllowedCommands should not be empty")
	}

	// Verify all commands are exactly 2 characters
	for _, cmd := range AllowedCommands {
		if len(cmd) != 2 {
			t.Errorf("Command %q is not exactly 2 characters", cmd)
		}
	}
}

func TestAllowedCommands_ContainsExpectedCommands(t *testing.T) {
	expectedCommands := []string{
		"??", // Query overall module information
		"A!", // Save current configuration
		"AX", // Reset to factory defaults
		"U?", // Query current speed units
		"UC", // Set units to centimeters per second
		"UF", // Set units to feet per second
		"UK", // Set units to kilometers per hour
		"UM", // Set units to meters per second
		"US", // Set units to miles per hour
		"OS", // Enable speed reporting
		"OM", // Enable magnitude reporting (Doppler)
		"Om", // Disable magnitude reporting (Doppler)
		"O?", // Query output settings
	}

	commandSet := make(map[string]bool)
	for _, cmd := range AllowedCommands {
		commandSet[cmd] = true
	}

	for _, expected := range expectedCommands {
		if !commandSet[expected] {
			t.Errorf("Expected command %q not found in AllowedCommands", expected)
		}
	}
}

func TestAllowedCommands_NoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, cmd := range AllowedCommands {
		if seen[cmd] {
			t.Errorf("Duplicate command found: %q", cmd)
		}
		seen[cmd] = true
	}
}
