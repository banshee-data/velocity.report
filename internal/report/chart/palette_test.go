package chart
package chart

import (
	"strings"
	"testing"
)

func TestPalette(t *testing.T) {
	colours := []struct {
		name  string
		value string
	}{
		{"ColourP50", ColourP50},
		{"ColourP85", ColourP85},
		{"ColourP98", ColourP98},
		{"ColourMax", ColourMax},
		{"ColourCountBar", ColourCountBar},
		{"ColourLowSample", ColourLowSample},
		{"ColourSteelBlue", ColourSteelBlue},
	}
	for _, c := range colours {
		if c.value == "" {
			t.Errorf("%s is empty", c.name)
		}
		if !strings.HasPrefix(c.value, "#") {
			t.Errorf("%s = %q, want prefix '#'", c.name, c.value)
		}
	}
}
