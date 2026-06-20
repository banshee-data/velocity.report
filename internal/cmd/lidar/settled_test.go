//go:build pcap
// +build pcap

package lidar

import "testing"

// TestSplitMain_SettledThresholdOverflow guards the uint32 range check: a value
// above math.MaxUint32 is rejected (exit 2) rather than silently truncated.
func TestSplitMain_SettledThresholdOverflow(t *testing.T) {
	code := silence(t, func() int {
		return SplitMain([]string{"--pcap", "x", "--settled-threshold", "5000000000"})
	})
	if code != 2 {
		t.Errorf("oversized --settled-threshold = %d, want 2", code)
	}
}
