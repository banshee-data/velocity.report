//go:build pcap

package lidar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitMainRequiresPCAP(t *testing.T) {
	if code := SplitMain(nil); code != 2 {
		t.Errorf("SplitMain(nil) = %d, want 2", code)
	}
}

func TestSplitMainHelpExitsZero(t *testing.T) {
	if code := SplitMain([]string{"-h"}); code != 0 {
		t.Errorf("SplitMain(-h) = %d, want 0", code)
	}
}

func TestSplitMainRejectsUnknownFlag(t *testing.T) {
	if code := SplitMain([]string{"-nope"}); code != 2 {
		t.Errorf("SplitMain(-nope) = %d, want 2", code)
	}
}

func TestSplitMainRejectsMissingPCAP(t *testing.T) {
	code := SplitMain([]string{
		"-pcap", filepath.Join(t.TempDir(), "absent.pcapng"),
		"-output", t.TempDir(), "-dry-run",
	})
	if code != 1 {
		t.Errorf("SplitMain with a missing capture = %d, want 1", code)
	}
}

// TestSplitMainDryRunAnalysesWithoutWriting drives the analysis half of the
// splitter — motion/static segmentation and the reporting paths — without
// producing segment PCAPs.
func TestSplitMainDryRunAnalysesWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	motionJSON := filepath.Join(dir, "motion.json")

	code := SplitMain([]string{
		"-pcap", benchFixture,
		"-output", dir,
		"-dry-run",
		"-export-json",
		"-export-metrics",
		"-stats-10s",
		"-motion-json", motionJSON,
		"-timeline-units", "timestamp",
		"-progress", "0",
		"-duration-seconds", "10",
	})
	if code != 0 {
		t.Fatalf("SplitMain = %d, want 0", code)
	}

	if _, err := os.Stat(motionJSON); err != nil {
		t.Errorf("expected the motion timeline JSON at %s: %v", motionJSON, err)
	}
	// A dry run must not emit segment captures.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading output dir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".pcap" || filepath.Ext(e.Name()) == ".pcapng" {
			t.Errorf("dry run wrote a segment capture: %s", e.Name())
		}
	}
}

// TestSplitMainWritesSegments exercises the writer: real segment captures plus
// the metadata sidecars.
func TestSplitMainWritesSegments(t *testing.T) {
	dir := t.TempDir()

	code := SplitMain([]string{
		"-pcap", benchFixture,
		"-output", dir,
		"-prefix", "seg",
		"-export-json",
		"-progress", "0",
		"-duration-seconds", "10",
	})
	if code != 0 {
		t.Fatalf("SplitMain = %d, want 0", code)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading output dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("split produced no output files")
	}
}

func TestSplitMainUnknownTimelineUnitsFallsBackToFrames(t *testing.T) {
	// -timeline-units is not validated: FormatMotionTimeline switches on
	// "seconds" and "timestamp" and renders frame numbers for anything else.
	// An unrecognised value is therefore a successful run with the frames
	// rendering, not an error — worth pinning so the fallback is deliberate.
	code := SplitMain([]string{
		"-pcap", benchFixture,
		"-output", t.TempDir(),
		"-dry-run",
		"-timeline-units", "furlongs",
		"-progress", "0",
		"-duration-seconds", "10",
	})
	if code != 0 {
		t.Errorf("SplitMain with unknown timeline units = %d, want 0", code)
	}
}
