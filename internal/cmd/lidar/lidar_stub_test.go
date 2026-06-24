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
	devnull, _ := os.Open(os.DevNull)
	os.Stderr = devnull
	defer func() {
		os.Stderr = oldErr
		_ = devnull.Close()
	}()

	if code := Main([]string{"pcap-split"}); code != 1 {
		t.Errorf("stub Main = %d, want 1", code)
	}
}
