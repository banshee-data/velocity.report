//go:build pcap
// +build pcap

package network

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectUDPPort(t *testing.T) {
	src := filepath.Join("..", "..", "perf", "pcap", "kirk0.pcapng")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("perf capture unavailable: %v", err)
	}
	port, err := DetectUDPPort(src)
	if err != nil {
		t.Fatalf("DetectUDPPort: %v", err)
	}
	if port != 2369 {
		t.Errorf("DetectUDPPort(kirk0) = %d, want 2369", port)
	}
}

func TestDetectUDPPort_Errors(t *testing.T) {
	if _, err := DetectUDPPort("/nonexistent/x.pcap"); err == nil {
		t.Error("expected error opening a missing pcap")
	}
}
