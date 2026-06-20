//go:build !pcap
// +build !pcap

package lidar

import (
	"os"
	"testing"
)

// TestStubMain covers the !pcap stub: it reports the missing pcap support and
// returns a non-zero code. Runs in the default (tag-free) test suite.
func TestStubMain(t *testing.T) {
	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldErr }()

	if code := Main([]string{"pcap-analyse"}); code != 1 {
		t.Errorf("stub Main = %d, want 1", code)
	}
}
