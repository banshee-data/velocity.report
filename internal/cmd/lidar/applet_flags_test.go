//go:build pcap
// +build pcap

package lidar

import (
	"os"
	"testing"
)

// silence redirects both stdout and stderr while fn runs (flag usage/errors go
// to stderr).
func silence(t *testing.T, fn func() int) int {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stdout, os.Stderr = devnull, devnull
	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		_ = devnull.Close()
	}()
	return fn()
}

func TestApplets_Help(t *testing.T) {
	for _, args := range [][]string{
		{"pcap-analyse", "-h"}, {"pcap-split", "-h"}, {"settling-eval", "-h"},
	} {
		if code := silence(t, func() int { return Main(args) }); code != 0 {
			t.Errorf("Main(%v) = %d, want 0", args, code)
		}
	}
}

func TestApplets_BadFlag(t *testing.T) {
	for _, args := range [][]string{
		{"pcap-analyse", "-nope"}, {"pcap-split", "-nope"}, {"settling-eval", "-nope"},
	} {
		if code := silence(t, func() int { return Main(args) }); code != 2 {
			t.Errorf("Main(%v) = %d, want 2", args, code)
		}
	}
}

func TestMain_NoArgsAndHelp(t *testing.T) {
	if code := silence(t, func() int { return Main(nil) }); code != 0 {
		t.Errorf("Main(nil) = %d, want 0", code)
	}
	if code := silence(t, func() int { return Main([]string{"help"}) }); code != 0 {
		t.Errorf("Main(help) = %d, want 0", code)
	}
	if code := silence(t, func() int { return Main([]string{"bogus"}) }); code != 2 {
		t.Errorf("Main(bogus) = %d, want 2", code)
	}
}
