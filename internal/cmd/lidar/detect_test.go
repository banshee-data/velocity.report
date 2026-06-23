//go:build pcap
// +build pcap

package lidar

import "testing"

// TestMain_AutoDetectsPort confirms that omitting --port auto-detects the
// sensor's UDP port from the capture (kirk0 logs on 2369).
func TestMain_AutoDetectsPort(t *testing.T) {
	pcapFile := truncatedCapture(t, 1000)
	code := silence(t, func() int {
		return Main([]string{"pcap-split", "--pcap", pcapFile, "--dry-run", "--output", t.TempDir(), "--progress", "0"})
	})
	if code != 0 {
		t.Errorf("pcap-split without --port = %d, want 0 (should auto-detect 2369)", code)
	}
}

func TestResolveUDPPort(t *testing.T) {
	if got := resolveUDPPort(2370, "x.pcap"); got != 2370 {
		t.Errorf("explicit port = %d, want 2370 (unchanged)", got)
	}
	if got := resolveUDPPort(0, ""); got != 0 {
		t.Errorf("no pcap file = %d, want 0", got)
	}
	if got := resolveUDPPort(0, "/nonexistent/x.pcap"); got != -1 {
		t.Errorf("undetectable = %d, want -1", got)
	}
}
