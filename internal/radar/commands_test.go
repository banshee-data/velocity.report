package radar

import (
	"testing"
)

func TestKnownCommands(t *testing.T) {
	if len(KnownCommands) == 0 {
		t.Fatal("KnownCommands should not be empty")
	}

	for _, cmd := range KnownCommands {
		if len(cmd.Code) != 2 {
			t.Errorf("Command code %q is not exactly 2 characters", cmd.Code)
		}
		if cmd.Description == "" {
			t.Errorf("Command %q has an empty description", cmd.Code)
		}
	}
}

func TestKnownCommands_ContainsExpectedCommands(t *testing.T) {
	expectedCommands := []string{
		"??", // Query overall module information
		"A!", // Save current configuration
		"AX", // Reset to factory defaults
		"U?", // Query current speed units
		"UC", // Set speed units to centimetres per second
		"UF", // Set speed units to feet per second
		"UK", // Set speed units to kilometres per hour
		"UM", // Set speed units to metres per second
		"US", // Set speed units to miles per hour
		"OS", // Enable speed reporting
		"OM", // Enable magnitude reporting (Doppler)
		"Om", // Disable magnitude reporting (Doppler)
		"O?", // Query speed output settings
		"OJ", // Enable JSON output mode (sent during init)
		"OH", // Enable human-readable timestamp (sent during init)
		"OC", // Enable processing-activity lights (sent during init)
		"oD", // Enable range reporting, FMCW lowercase form (sent during init)
		"C=", // Set sensor clock (sent during init as C=<epoch>)
		"CZ", // Set timezone (sent during init as CZ<name><offset>)
		"R>", // Set lower speed filter (sent during init as R>0.25)
	}

	commandSet := make(map[string]bool)
	for _, cmd := range KnownCommands {
		commandSet[cmd.Code] = true
	}

	for _, expected := range expectedCommands {
		if !commandSet[expected] {
			t.Errorf("Expected command %q not found in KnownCommands", expected)
		}
	}
}

func TestKnownCommands_NoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, cmd := range KnownCommands {
		if seen[cmd.Code] {
			t.Errorf("Duplicate command found: %q", cmd.Code)
		}
		seen[cmd.Code] = true
	}
}

func TestIsKnownCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"??", true},
		{"OJ", true}, // now catalogued (init command)
		{"AX", true},
		{"OS", true},
		{"A!", true},
		{"ZZ", false},
		{"", false},
		{"HACK", false},
		{"\n", false},
		{"rm", false},
	}
	for _, tt := range tests {
		if got := IsKnownCommand(tt.cmd); got != tt.want {
			t.Errorf("IsKnownCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestIsKnownCommandCode(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"OJ", true},           // exact two-character code
		{"R>0.25", true},       // parameterised filter (init path)
		{"C=1700000000", true}, // parameterised clock sync (init path)
		{"CZPST-8", true},      // parameterised timezone (init path)
		{"XQ", false},          // genuine unknown code
		{"ZZ99", false},        // unknown code with argument
		{"R", false},           // shorter than a two-character code
		{"", false},            // empty
	}
	for _, tt := range tests {
		if got := IsKnownCommandCode(tt.cmd); got != tt.want {
			t.Errorf("IsKnownCommandCode(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}
