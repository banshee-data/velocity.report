package settlingeval

import (
	"path/filepath"
	"testing"
)

// TestRun_BadTuningConfig covers the tuning-load error branch (no pcap I/O, so
// it runs in the default tag-free suite).
func TestRun_BadTuningConfig(t *testing.T) {
	_, err := Run(Config{PCAPFile: "x.pcap", TuningFile: filepath.Join(t.TempDir(), "nonexistent.json"), SensorID: "s", UDPPort: 2369})
	if err == nil {
		t.Error("expected tuning-load error")
	}
}
