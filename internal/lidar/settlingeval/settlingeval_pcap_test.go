//go:build pcap

package settlingeval

import (
	"os"
	"path/filepath"
	"testing"
)

// fixturePCAP is the reference capture for frame-level LiDAR tooling. It
// carries many frames on UDP port 2369; the 20Hz capture in the same tree is
// degenerate (a single frame) and cannot exercise settling convergence.
const (
	fixturePCAP = "../perf/pcap/kirk0.pcapng"
	fixturePort = 2369
)

// TestRunReplaysFixtureAndReportsSettling drives the whole offline evaluation:
// PCAP decode, frame assembly, background-grid updates and per-frame settling
// metrics. It is the only path that reaches the frame callback, which is the
// core the rest of the package exists to serve.
//
// Requires -tags=pcap; without it network.ReadPCAPFile is a stub that refuses.
func TestRunReplaysFixtureAndReportsSettling(t *testing.T) {
	if testing.Short() {
		t.Skip("replays a multi-frame PCAP fixture")
	}

	report, err := Run(filepath.Clean(fixturePCAP), "", "test-sensor", fixturePort)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report == nil {
		t.Fatal("Run returned a nil report")
	}

	if report.TotalFrames <= 1 {
		t.Fatalf("TotalFrames = %d, want more than 1 — the fixture should carry many frames",
			report.TotalFrames)
	}
	// One metrics sample is recorded per frame, so the history must track the
	// frame count rather than lagging behind it.
	if len(report.MetricsHistory) != report.TotalFrames {
		t.Errorf("history entries = %d, want %d (one per frame)",
			len(report.MetricsHistory), report.TotalFrames)
	}

	// Coverage is a rate in [0,1] and should climb as the grid fills: the last
	// frame must not be worse covered than the first.
	first := report.MetricsHistory[0]
	last := report.MetricsHistory[len(report.MetricsHistory)-1]
	for i, m := range []struct {
		name string
		val  float64
	}{{"first", first.CoverageRate}, {"last", last.CoverageRate}} {
		if m.val < 0 || m.val > 1 {
			t.Errorf("%s CoverageRate = %v, want a rate within [0,1] (sample %d)", m.name, m.val, i)
		}
	}
	if last.CoverageRate < first.CoverageRate {
		t.Errorf("CoverageRate fell from %v to %v across the replay, want it non-decreasing",
			first.CoverageRate, last.CoverageRate)
	}
}

func TestRunRejectsMissingPCAP(t *testing.T) {
	if _, err := Run("does-not-exist.pcapng", "", "test-sensor", fixturePort); err == nil {
		t.Fatal("Run on a missing PCAP succeeded, want an error")
	}
}

func TestRunRejectsUnreadableTuningConfig(t *testing.T) {
	// A path that exists but is not valid JSON must fail loudly rather than
	// silently falling back to the embedded defaults.
	bad := filepath.Join(t.TempDir(), "tuning.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("writing bad config: %v", err)
	}

	if _, err := Run(filepath.Clean(fixturePCAP), bad, "test-sensor", fixturePort); err == nil {
		t.Fatal("Run with a malformed tuning config succeeded, want an error")
	}
}
