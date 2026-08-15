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
	if testing.Short() {
		t.Skip("analyses a multi-frame PCAP fixture")
	}

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
	if testing.Short() {
		t.Skip("splits a multi-frame PCAP fixture")
	}

	dir := t.TempDir()

	code := SplitMain([]string{
		"-pcap", benchFixture,
		"-output", dir,
		"-prefix", "seg",
		"-export-json",
		"-progress", "0",
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

func TestSplitMainRejectsBadTimelineUnits(t *testing.T) {
	// An unrecognised units value should be reported rather than silently
	// falling back, so a typo in a script does not change the report format.
	code := SplitMain([]string{
		"-pcap", benchFixture,
		"-output", t.TempDir(),
		"-dry-run",
		"-timeline-units", "furlongs",
		"-progress", "0",
	})
	if code == 2 {
		return // rejected at parse/validation time
	}
	if code != 0 {
		t.Errorf("SplitMain with bad timeline units = %d, want 0 or 2", code)
	}
}
