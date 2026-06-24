//go:build pcap
// +build pcap

package pcapsplit

import (
	"context"
	"os"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// TestSoma3StaticThenMotion is an opt-in hardware-capture regression test for the
// background-drift motion classifier. soma3 is parked in busy urban traffic for
// its first ~14,300 frames (~1430 s; 22:16:38–22:40:30 in the capture) and then
// drives. The earlier foreground+noise-deviation classifier mislabelled the whole
// parked period as motion: heavy cross-traffic inflates per-cell range spread, so
// a scene-activity signal cannot tell a busy parked scene from driving. The
// background-drift ratio keys on ego-motion instead — most of the settled grid
// shifting from its locked baseline at once — which a parked sensor never produces
// however busy the scene.
//
// soma2 does NOT cover this: its parked period is quiet, so the deviation detector
// never false-fired there. Only soma3's busy parked scene exposes the bug.
//
// The capture is ~10 GB and external to the repo, so the test reads only the first
// ~1600 s — across the parked→driving transition — to stay within a couple of
// minutes. Set SOMA3_PCAP to run it.
func TestSoma3StaticThenMotion(t *testing.T) {
	pcapFile := os.Getenv("SOMA3_PCAP")
	if pcapFile == "" {
		t.Skip("set SOMA3_PCAP to run the static-then-motion capture regression")
	}

	parserCfg, err := parse.LoadPandar40PConfig()
	if err != nil {
		t.Fatal(err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	elevations := parse.ElevationsFromConfig(parserCfg)

	classifier, err := NewMotionClassifier("hesai-pandar40p", pcapFile, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := classifier.SetRingElevations(elevations); err != nil {
		t.Fatal(err)
	}

	var samples []FrameSample
	frameCallback := func(frame *l2frames.LiDARFrame) {
		if frame == nil || len(frame.PolarPoints) == 0 {
			return
		}
		ev, err := classifier.Observe(frame.StartTimestamp, frame.PolarPoints)
		if err != nil {
			return
		}
		samples = append(samples, FrameSample{T: ev.T, Moving: ev.Moving})
	}

	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID:        "hesai-pandar40p",
		FrameCallback:   frameCallback,
		FrameChCapacity: 32,
	})
	fb.SetBlockOnFrameChannel(true)
	// Read the first 1600 s: the whole parked period (~1430 s) plus the first
	// ~170 s of the drive. durationSeconds bounds the read on the ~10 GB file.
	if err := network.ReadPCAPFile(
		context.Background(), pcapFile, 2369, parser, fb, nil, nil,
		0, 1600, 0, 0, nil,
	); err != nil {
		t.Fatal(err)
	}
	fb.Close()

	if len(samples) < 15000 {
		t.Fatalf("expected ~16000 frames over the first 1600 s, got %d", len(samples))
	}

	// Assert on the post-hysteresis timeline, exactly as the tool builds it. Raw
	// per-frame motion has occasional single-frame spikes even while parked (a bus
	// passing close, a brief drift-ratio excursion); the 5 s motion trigger and
	// min-segment merge absorb them, so the parked stretch stays one static segment.
	periods := BuildTimeline(samples, DefaultSplitConfig().TimelineConfig())
	if len(periods) == 0 || periods[0].Type != StaticLabel {
		t.Fatalf("expected an initial static period, got %+v", periods)
	}

	// The drive begins ~1430 s in. The bug collapsed the long parked stretch to
	// nothing and reported the whole capture as one motion period; the first motion
	// period must instead start near 1430 s.
	firstMotionStart := -1.0
	for _, p := range periods {
		if p.Type == MotionLabel {
			firstMotionStart = p.StartSecs
			break
		}
	}
	if firstMotionStart < 0 {
		t.Fatal("no motion period found; the drive onset was missed")
	}
	if firstMotionStart < 1380 || firstMotionStart > 1480 {
		t.Errorf("first motion period starts at %.0fs, want ~1430s; "+
			"an early start means the busy parked scene was mislabelled motion", firstMotionStart)
	}
}
