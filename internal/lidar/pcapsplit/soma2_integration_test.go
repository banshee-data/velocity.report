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
	var staticSecs, motionSecs float64
	for _, period := range periods {
		switch period.Type {
		case StaticLabel:
			staticSecs += period.DurationSecs
		case MotionLabel:
			motionSecs += period.DurationSecs
		}
	}
	if staticSecs == 0 || motionSecs == 0 {
		t.Fatalf("soma2 must contain both static and motion periods, got %+v", periods)
	}
	// soma2 is parked for ~70 s then drives for the remaining ~21 min, so motion
	// must dominate. Regression guard: a foreground-only classifier mislabelled
	// the long drive as static once the per-cell range spread saturated and the
	// foreground gate widened — leaving only a brief motion blip near the start.
	if motionSecs < staticSecs {
		t.Errorf("expected motion to dominate; static=%.0fs motion=%.0fs", staticSecs, motionSecs)
	}
	// The first raw motion sample should appear near the known frame-800 onset.
	// Allow a broad window for capture-frame assembly and the 5-second timeline
	// trigger, but reject foreground-only regressions that detect only a late
	// transient.
	firstMotion := -1
	for i, sample := range analysis.Samples {
		if sample.Moving {
			firstMotion = i
			break
		}
	}
	if firstMotion < 650 || firstMotion > 1000 {
		t.Errorf("first motion frame=%d, want roughly 800", firstMotion)
	}
}
