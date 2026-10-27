//go:build pcap

package replayeval

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// A frame tap reads every published frame and changes nothing: kirk0's
// baseline is byte-equal with and without one, each tapped frame's mask
// covers its returns, and with box_ground the scored frames carry the
// fitted surface and the L3 grid's resolution.
func TestFrameTapReadsWithoutChanging(t *testing.T) {
	pcapPath := requireKirk0(t)
	dir := t.TempDir()
	run := func(label string, tap func(FrameTapFrame)) []byte {
		t.Helper()
		cfg := Config{PCAPFile: pcapPath, OutDir: filepath.Join(dir, label), SensorID: "tap", UDPPort: 2369,
			StartSeconds: 20, WarmupSeconds: 20, DurationSeconds: 20,
			TuningFile: "../../../config/tuning.defaults.json", Experiments: []string{ExperimentBoxGround}, FrameTap: tap}
		if _, err := Run(cfg); err != nil {
			t.Fatalf("%s: Run: %v", label, err)
		}
		b, err := os.ReadFile(filepath.Join(cfg.OutDir, "tracking_baseline.json"))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var frames, scored, withSurface, badMask int
	tapped := run("tap", func(f FrameTapFrame) {
		frames++
		if len(f.Mask) != len(f.Frame.Points) || len(f.L3Mask) != len(f.Mask) {
			badMask++
		}
		for i := range f.L3Mask {
			// The guard only ever adds foreground for clients.
			if f.L3Mask[i] && !f.Mask[i] {
				badMask++
				break
			}
		}
		if f.Scored {
			scored++
			if f.Surface != nil && f.AzimuthBins > 0 && f.HeightBandFloor < 0 {
				withSurface++
			}
		}
	})
	plain := run("plain", nil)
	if !bytes.Equal(tapped, plain) {
		t.Fatal("the frame tap changed the baseline")
	}
	if frames == 0 || scored == 0 || badMask != 0 || withSurface != scored {
		t.Fatalf("frames %d, scored %d, bad masks %d, scored with surface %d", frames, scored, badMask, withSurface)
	}
}
