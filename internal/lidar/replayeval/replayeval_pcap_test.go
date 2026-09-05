//go:build pcap
// +build pcap

package replayeval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/analysis"
)

// kirk0 is the in-repo reference capture. It is on UDP port 2369.
const kirk0 = "../perf/pcap/kirk0.pcapng"

func requireKirk0(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(kirk0)
	if err != nil {
		t.Fatalf("resolve fixture: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("reference capture not available: %v", err)
	}
	return p
}

// The harness must produce a VRLOG that the existing analysis path can read
// without any special casing. That compatibility is the whole point of
// recording a VRLOG rather than inventing a new output format.
func TestRunProducesAnalysableVRLOG(t *testing.T) {
	pcapPath := requireKirk0(t)
	out := filepath.Join(t.TempDir(), "run")

	result, err := Run(Config{
		PCAPFile:        pcapPath,
		OutDir:          out,
		SensorID:        "test-replay",
		UDPPort:         2369,
		DurationSeconds: 4,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FramesRead == 0 {
		t.Fatal("no frames read")
	}

	for _, name := range []string{"header.json", "index.bin", "frames/chunk_0000.pb"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("expected %s in the recording: %v", name, err)
		}
	}

	report, _, err := analysis.GenerateReport(out)
	if err != nil {
		t.Fatalf("GenerateReport on the harness output: %v", err)
	}
	if report.Recording.SensorID != "test-replay" {
		t.Fatalf("sensor id = %q, want \"test-replay\"", report.Recording.SensorID)
	}
	if report.FrameSummary.TotalFrames == 0 {
		t.Fatal("analysis reports zero frames")
	}
}

// Two runs of the same capture with the same config must agree on frame count.
// A harness whose output depends on wall-clock timing cannot support an A/B,
// because the difference between arms would be indistinguishable from noise.
func TestRunFrameCountIsStableAcrossRuns(t *testing.T) {
	pcapPath := requireKirk0(t)
	dir := t.TempDir()

	var counts []int
	for i, name := range []string{"a", "b"} {
		res, err := Run(Config{
			PCAPFile:        pcapPath,
			OutDir:          filepath.Join(dir, name),
			SensorID:        "test-replay",
			UDPPort:         2369,
			DurationSeconds: 4,
		})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		counts = append(counts, res.FramesRead)
	}
	if counts[0] != counts[1] {
		t.Fatalf("frame counts differ across identical runs: %d vs %d", counts[0], counts[1])
	}
}

// IncludePoints is covered by TestPublisherDropsPointCloudWhenNotRequested,
// which tests the same decision directly and runs in the default suite. An
// end-to-end version would need a capture window past the 30 s background
// warmup before any frame carries a cloud, costing about twenty seconds to
// re-prove the same line. Verified manually instead: 45 s of the SoMa capture
// records 347 MB with points against 9.1 MB without.

// A port filter that matches nothing must be reported, not returned as a
// successful empty run.
func TestRunRejectsEmptyResult(t *testing.T) {
	pcapPath := requireKirk0(t)
	_, err := Run(Config{
		PCAPFile:        pcapPath,
		OutDir:          filepath.Join(t.TempDir(), "empty"),
		UDPPort:         9999, // nothing in the capture uses this
		DurationSeconds: 2,
	})
	if err == nil {
		t.Fatal("a run that produced no frames returned success")
	}
}
