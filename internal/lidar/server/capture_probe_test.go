//go:build pcap
// +build pcap

package server

import (
	"errors"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

// stubProbeSeams replaces the counter and the port sniffer for one test.
func stubProbeSeams(t *testing.T, counts map[int]network.PCAPCountResult, detected int, detectErr error) *int {
	t.Helper()
	origCount, origDetect := countPCAPPackets, detectCapturePort
	t.Cleanup(func() { countPCAPPackets, detectCapturePort = origCount, origDetect })

	reads := 0
	countPCAPPackets = func(_ string, port int) (network.PCAPCountResult, error) {
		reads++
		if r, ok := counts[port]; ok {
			return r, nil
		}
		return network.PCAPCountResult{}, nil
	}
	detectCapturePort = func(string) (int, error) { return detected, detectErr }
	return &reads
}

func TestProbeUsesTheConfiguredPortWhenItMatches(t *testing.T) {
	// On a live deployment the captures were written by this very sensor, so
	// the configured port is right and sniffing would be a wasted read.
	reads := stubProbeSeams(t, map[int]network.PCAPCountResult{
		2369: {Count: 540000, FirstTimestampNs: 100, LastTimestampNs: 200},
	}, 9999, nil)

	extent, err := probeCaptureExtent("a.pcap", 2369)
	if err != nil {
		t.Fatalf("probeCaptureExtent: %v", err)
	}
	if extent.PacketCount != 540000 || extent.UDPPort != 2369 {
		t.Errorf("extent = %+v, want the configured port's count", extent)
	}
	if *reads != 1 {
		t.Errorf("read the capture %d times, want 1", *reads)
	}
}

func TestProbeFallsBackToThePortTheCaptureCarries(t *testing.T) {
	// An index that could only read captures recorded on the port this process
	// listens on would be useless for anything copied in from another site.
	stubProbeSeams(t, map[int]network.PCAPCountResult{
		2369: {Count: 540000, FirstTimestampNs: 100, LastTimestampNs: 200},
	}, 2369, nil)

	extent, err := probeCaptureExtent("a.pcap", 12369)
	if err != nil {
		t.Fatalf("probeCaptureExtent: %v", err)
	}
	if extent.PacketCount != 540000 {
		t.Errorf("packet count = %d, want the sniffed port's count", extent.PacketCount)
	}
	if extent.UDPPort != 2369 {
		t.Errorf("UDPPort = %d, want the port the capture carries", extent.UDPPort)
	}
}

func TestProbeReportsAnEmptyCaptureWithoutReadingItTwice(t *testing.T) {
	// When sniffing lands on the port that just matched nothing, the capture
	// holds no LiDAR data; repeating the read would say the same thing slower.
	reads := stubProbeSeams(t, map[int]network.PCAPCountResult{}, 2369, nil)

	extent, err := probeCaptureExtent("a.pcap", 2369)
	if err != nil {
		t.Fatalf("probeCaptureExtent: %v", err)
	}
	if extent.PacketCount != 0 {
		t.Errorf("packet count = %d, want 0 for a capture with no LiDAR data", extent.PacketCount)
	}
	if *reads != 1 {
		t.Errorf("read the capture %d times, want 1", *reads)
	}
}

func TestProbeSurfacesASniffFailure(t *testing.T) {
	stubProbeSeams(t, map[int]network.PCAPCountResult{}, 0, errors.New("truncated capture"))
	if _, err := probeCaptureExtent("a.pcap", 12369); err == nil {
		t.Fatal("a sniff failure was swallowed")
	}
}

func TestProbeSniffsWhenNoPortIsConfigured(t *testing.T) {
	stubProbeSeams(t, map[int]network.PCAPCountResult{
		2369: {Count: 10, FirstTimestampNs: 1, LastTimestampNs: 2},
	}, 2369, nil)

	extent, err := probeCaptureExtent("a.pcap", 0)
	if err != nil {
		t.Fatalf("probeCaptureExtent: %v", err)
	}
	if extent.UDPPort != 2369 {
		t.Errorf("UDPPort = %d, want the sniffed port", extent.UDPPort)
	}
}
