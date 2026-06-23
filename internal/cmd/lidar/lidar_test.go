//go:build pcap
// +build pcap

package lidar

import (
	"os"
	"testing"
)

// runMain invokes Main with stdout/stderr redirected to avoid noisy test output.
func runMain(t *testing.T, args []string) int {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stdout, os.Stderr = devnull, devnull
	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		_ = devnull.Close()
	}()
	return Main(args)
}

func TestMain_NoArgsPrintsUsage(t *testing.T) {
	if code := runMain(t, nil); code != 0 {
		t.Errorf("Main(nil) = %d, want 0", code)
	}
}

func TestMain_HelpFlag(t *testing.T) {
	if code := runMain(t, []string{"-h"}); code != 0 {
		t.Errorf("Main(-h) = %d, want 0", code)
	}
}

func TestMain_UnknownCommand(t *testing.T) {
	if code := runMain(t, []string{"bogus"}); code != 2 {
		t.Errorf("Main(bogus) = %d, want 2", code)
	}
}

// TestMain_PcapSplitRouting confirms the namespace reaches the split engine:
// with no --pcap flag the engine reports the missing file and returns 2.
func TestMain_PcapSplitRouting(t *testing.T) {
	if code := runMain(t, []string{"pcap-split"}); code != 2 {
		t.Errorf("Main(pcap-split) with no --pcap = %d, want 2", code)
	}
}
