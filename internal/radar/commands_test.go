package radar

import (
	"testing"
)

func TestIsValidAngleCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected bool
	}{
		// Valid commands
		{"valid inbound zero", "^/+0.0", true},
		{"valid outbound zero", "^/-0.0", true},
		{"valid inbound positive", "^/+5.2", true},
		{"valid outbound positive", "^/-10.5", true},
		{"valid without decimal", "^/+5", true},
		{"valid small angle", "^/+0.1", true},

		// Invalid commands
		{"too short", "^/+", false},
		{"wrong prefix", "R+0.0", false},
		{"no sign", "^/0.0", false},
		{"not a number", "^/+abc", false},
		{"missing angle", "^/+", false},
		{"empty", "", false},
		{"only prefix", "^/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidAngleCommand(tt.cmd)
			if result != tt.expected {
				t.Errorf("IsValidAngleCommand(%q) = %v, expected %v", tt.cmd, result, tt.expected)
			}
		})
	}
}

func TestIsAllowedCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected bool
	}{
		// Static commands
		{"static command ??", "??", true},
		{"static command R+", "R+", true},
		{"static command AX", "AX", true},

		// Dynamic angle commands
		{"dynamic inbound zero", "^/+0.0", true},
		{"dynamic outbound zero", "^/-0.0", true},
		{"dynamic angle", "^/+5.2", true},

		// Invalid commands
		{"invalid command", "XX", false},
		{"invalid angle", "^/+abc", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAllowedCommand(tt.cmd)
			if result != tt.expected {
				t.Errorf("IsAllowedCommand(%q) = %v, expected %v", tt.cmd, result, tt.expected)
			}
		})
	}
}
