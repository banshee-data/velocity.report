//go:build pcap

package replayeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// On kirk0 the shipped defaults' track guard publishes none of a confirmed
// track's returns as background, where the guard off publishes many; the
// replay writes the report it returns. Thirty seconds of capture holds
// enough traffic for both.
func TestBackgroundLeakOnKirk0(t *testing.T) {
	pcapPath := requireKirk0(t)
	dir := t.TempDir()
	run := func(label, tuning string) *BackgroundLeakReport {
		t.Helper()
		cfg := Config{PCAPFile: pcapPath, OutDir: filepath.Join(dir, label), SensorID: "leak",
			UDPPort: 2369, DurationSeconds: 30, TuningFile: tuning, BackgroundLeak: true}
		result, err := Run(cfg)
		if err != nil {
			t.Fatalf("%s: Run: %v", label, err)
		}
		if result.BackgroundLeak == nil {
			t.Fatalf("%s: no background leak report", label)
		}
		b, err := os.ReadFile(filepath.Join(cfg.OutDir, backgroundLeakFile))
		if err != nil {
			t.Fatal(err)
		}
		var written BackgroundLeakReport
		if err := json.Unmarshal(b, &written); err != nil || written.BackgroundInTrackBoxes != result.BackgroundLeak.BackgroundInTrackBoxes {
			t.Fatalf("%s: written report %+v (%v) differs from the result's", label, written, err)
		}
		t.Logf("%s: %d of %d background returns in %d box-frames; settled under moving boxes: %d learned nearer, %d held",
			label, result.BackgroundLeak.BackgroundInTrackBoxes, result.BackgroundLeak.BackgroundPoints, result.BackgroundLeak.TrackBoxes,
			result.BackgroundLeak.SnapshotLearnedUnderMovingBoxes, result.BackgroundLeak.SnapshotHeldUnderMovingBoxes)
		return result.BackgroundLeak
	}
	off := run("off", "../../../config/experiments/track-guard-off.json")
	on := run("defaults", "../../../config/tuning.defaults.json")
	if off.TrackBoxes == 0 || off.BackgroundInTrackBoxes == 0 {
		t.Fatalf("guard off: %d box-frames, %d background returns in them; the capture shows no leak to stop", off.TrackBoxes, off.BackgroundInTrackBoxes)
	}
	if on.TrackBoxes == 0 || on.BackgroundInTrackBoxes != 0 {
		t.Fatalf("defaults: %d background returns in %d box-frames, want none", on.BackgroundInTrackBoxes, on.TrackBoxes)
	}
}
