//go:build pcap
// +build pcap

package lidar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitMain_RunError(t *testing.T) {
	junk := filepath.Join(t.TempDir(), "junk.pcap")
	if err := os.WriteFile(junk, []byte("not a pcap"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := silence(t, func() int {
		return SplitMain([]string{"--pcap", junk, "--port", "2369", "--output", t.TempDir()})
	})
	if code != 1 {
		t.Errorf("SplitMain corrupt pcap = %d, want 1", code)
	}
}

func TestSettlingEvalMain_WriteReportError(t *testing.T) {
	pcapFile := truncatedCapture(t, 1000)
	f := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	badOut := filepath.Join(f, "sub", "report.json") // parent is a file -> write fails
	code := silence(t, func() int {
		return SettlingEvalMain([]string{"--port", "2369", "--output", badOut, pcapFile})
	})
	if code != 1 {
		t.Errorf("SettlingEvalMain bad output = %d, want 1", code)
	}
}
