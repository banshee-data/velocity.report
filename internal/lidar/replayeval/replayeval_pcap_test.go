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

// Two runs of the same capture with the same config must agree exactly, not
// approximately. Without back-pressure on the frame channel the PCAP reader
// outruns the pipeline and frames are dropped at a rate that depends on how
// busy the machine is, so an A/B cannot separate the change under test from
// the scheduler. This is the property that makes the harness usable at all.
func TestRunIsDeterministic(t *testing.T) {
	pcapPath := requireKirk0(t)
	dir := t.TempDir()

	type outcome struct {
		frames  int
		tracks  int
		summary analysis.TrackSummary
	}
	var got []outcome

	for i, name := range []string{"a", "b"} {
		out := filepath.Join(dir, name)
		res, err := Run(Config{
			PCAPFile:        pcapPath,
			OutDir:          out,
			SensorID:        "test-replay",
			UDPPort:         2369,
			DurationSeconds: 6,
		})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		rep, _, err := analysis.GenerateReport(out)
		if err != nil {
			t.Fatalf("run %d analyse: %v", i, err)
		}
		got = append(got, outcome{res.FramesRead, rep.TrackSummary.TotalTracks, rep.TrackSummary})
	}

	if got[0].frames != got[1].frames {
		t.Fatalf("frame counts differ across identical runs: %d vs %d", got[0].frames, got[1].frames)
	}
	if got[0].tracks != got[1].tracks {
		t.Fatalf("track counts differ across identical runs: %d vs %d", got[0].tracks, got[1].tracks)
	}
	if got[0].summary.FragmentationRatio != got[1].summary.FragmentationRatio {
		t.Fatalf("fragmentation differs: %v vs %v",
			got[0].summary.FragmentationRatio, got[1].summary.FragmentationRatio)
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
