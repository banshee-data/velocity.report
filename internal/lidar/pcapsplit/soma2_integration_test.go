//go:build pcap
// +build pcap

package pcapsplit

import (
	"os"
	"testing"
)

// TestSoma2MixedMotion is an opt-in hardware-capture regression test. soma2
// begins parked and contains sustained sensor motion from roughly frame 800.
// It is intentionally external to the repository because the source capture
// is several gigabytes.
func TestSoma2MixedMotion(t *testing.T) {
	pcapFile := os.Getenv("SOMA2_PCAP")
	if pcapFile == "" {
		t.Skip("set SOMA2_PCAP to run the mixed-motion capture regression")
	}

	cfg := DefaultSplitConfig()
	cfg.PCAPFile = pcapFile
	cfg.UDPPort = 2369
	analysis, err := Analyse(cfg)
	if err != nil {
		t.Fatal(err)
	}
	periods := BuildTimeline(analysis.Samples, cfg.TimelineConfig())
	var static, motion bool
	for _, period := range periods {
		switch period.Type {
		case StaticLabel:
			static = true
		case MotionLabel:
			motion = true
		}
	}
	if !static || !motion {
		t.Fatalf("soma2 must contain both static and motion periods, got %+v", periods)
	}
}
