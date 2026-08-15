//go:build !pcap

package server

import (
	"strings"
	"testing"
)

// TestRunSelfCheckFailsWhenLiveCaptureRequestedWithoutPcap pins the stub
// behaviour in selfcheck_nopcap.go: a binary built without the pcap tag cannot
// capture, so asking it to must fail loudly rather than quietly reporting
// success. Under -tags=pcap the real capture runs instead, which is why this
// case is confined to the untagged build.
func TestRunSelfCheckFailsWhenLiveCaptureRequestedWithoutPcap(t *testing.T) {
	var out strings.Builder

	code := runSelfCheck(&out, "eth0")

	if code != 1 {
		t.Errorf("runSelfCheck = %d, want 1 when live capture is unavailable", code)
	}
	if !strings.Contains(out.String(), "built without the pcap tag") {
		t.Errorf("output = %q, want the missing-pcap-tag diagnostic", out.String())
	}
}
